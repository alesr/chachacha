package main

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/alesr/chachacha/internal/config"
	"github.com/alesr/chachacha/internal/events"
	"github.com/alesr/chachacha/internal/matchdirector"
	"github.com/alesr/chachacha/internal/sessionrepo"
	"github.com/rabbitmq/amqp091-go"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("Failed to load configuration", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logLevel := slog.LevelInfo
	if cfg.LogLevel == "debug" {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))

	// Initialize redis client and match director
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

	// Initialize RabbitMQ connection
	conn, err := amqp091.Dial(cfg.RabbitMQURL)
	if err != nil {
		logger.Error("Failed to connect to RabbitMQ", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		logger.Error("Failed to open channel", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer ch.Close()

	// Initialize publisher
	publisher, err := events.NewPublisher(ch)
	if err != nil {
		logger.Error("Failed to create publisher", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger.Info("Connected to Redis", slog.String("address", cfg.RedisAddr))

	director, err := matchdirector.New(logger, repo, publisher, cfg.MatchInterval)
	if err != nil {
		logger.Error("Failed to create match director", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger.Info("Starting match director service...", slog.Duration("match_interval", cfg.MatchInterval))
	director.Start()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	logger.Info("Received signal, shutting down...", slog.String("signal", sig.String()))

	director.Stop()
	logger.Info("Match director stopped")
	logger.Info("Service shutdown complete")
}
