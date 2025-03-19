package main

import (
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/alesr/chachacha/internal/config"
	"github.com/alesr/chachacha/internal/events"
	"github.com/alesr/chachacha/internal/matchregistry"
	"github.com/alesr/chachacha/internal/sessionrepo"
	"github.com/rabbitmq/amqp091-go"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	logLevel := slog.LevelInfo
	if cfg.LogLevel == "debug" {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))

	// Connect to RabbitMQ

	conn, err := amqp091.Dial(cfg.RabbitMQURL)
	if err != nil {
		logger.Error("Failed to connect to RabbitMQ", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer conn.Close()

	logger.Info("Connected to RabbitMQ", slog.String("url", cfg.RabbitMQURL))

	ch, err := conn.Channel()
	if err != nil {
		logger.Error("Failed to open the queue channel", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer ch.Close()

	logger.Debug("Channel opened")

	q, err := ch.QueueDeclare(
		cfg.QueueName, // queue name
		false,         // durable
		false,         // delete when unused
		false,         // exclusive
		false,         // no-wait
		nil,           // arguments
	)
	if err != nil {
		logger.Error("Failed to declare the queue", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger.Debug("Queue declared successfully", slog.String("queue_name", q.Name))

	if err := events.SetupInputExchangeBindings(ch, cfg.QueueName); err != nil {
		logger.Error("Failed to setup input exchange bindings", slog.String("error", err.Error()))
		os.Exit(1)
	}

	publisher, err := events.NewPublisher(ch)
	if err != nil {
		logger.Error("Failed to create event publisher", slog.String("error", err.Error()))
		os.Exit(1)
	} else {
		if err := events.SetupOutputExchangeQueueBindings(ch); err != nil {
			logger.Error("Failed to set up monitoring queues", slog.String("error", err.Error()))
			os.Exit(1)
		} else {
			logger.Debug("Monitoring queues set up successfully")
		}
	}

	// Initialize redis repo and match registry

	redisCli, err := sessionrepo.NewRedisClient(cfg.RedisAddr)
	if err != nil {
		logger.Error("Failed to init Redis client", slog.String("error", err.Error()))
		os.Exit(1)
	}

	repo, err := sessionrepo.NewRedisRepo(redisCli)
	if err != nil {
		logger.Error("Failed to connect to Redis", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger.Info("Connected to Redis", slog.String("address", cfg.RedisAddr))

	registry := matchregistry.New(logger, repo, cfg.QueueName, ch, publisher)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		logger.Info("Starting match registry service...")
		if err := registry.Start(); err != nil {
			logger.Error("Match registry failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	sig := <-sigChan
	logger.Info("Received signal, shutting down...", slog.String("signal", sig.String()))
	logger.Info("Service shutdown complete")
}
