package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/alesr/chachacha/internal/config"
	"github.com/alesr/chachacha/internal/events"
	pubevts "github.com/alesr/chachacha/pkg/events"
	"github.com/rabbitmq/amqp091-go"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	rabbitMQURL := flag.String("rabbitmq", cfg.RabbitMQURL, "RabbitMQ connection URL")
	queueName := flag.String("queue", cfg.QueueName, "RabbitMQ queue name")
	mode := flag.String("mode", "", "Operation mode: 'host' for registering a host or 'player' for registering a player")
	flag.Parse()

	if *mode != "host" && *mode != "player" {
		log.Fatal("Mode must be either 'host' or 'player'")
	}

	conn, err := amqp091.Dial(*rabbitMQURL)
	if err != nil {
		log.Fatalf("Failed to connect to RabbitMQ: %v", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("Failed to open a channel: %v", err)
	}
	defer ch.Close()

	if _, err := ch.QueueDeclare(
		*queueName, // queue name
		false,      // durable
		false,      // delete when unused
		false,      // exclusive
		false,      // no-wait
		nil,        // arguments
	); err != nil {
		log.Fatalf("Failed to declare a queue: %v", err)
	}

	if _, err := events.NewPublisher(ch); err != nil {
		log.Fatalf("Failed to create event publisher: %v", err)
	}

	if err := events.SetupInputExchangeBindings(ch, cfg.QueueName); err != nil {
		log.Fatalf("Failed to set up input exchange queue bindings: %v", err)
	}

	if err := events.SetupMonitoringQueues(ch); err != nil {
		log.Fatalf("Failed to set up monitoring binding queues: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	switch *mode {
	case "host":
		registerHost(ctx, ch)
	case "player":
		registerPlayer(ctx, ch)
	}
}

func registerHost(ctx context.Context, ch *amqp091.Channel) {
	fmt.Println("=== Host Registration ===")

	hostID := readInput("Enter host ID (e.g., 123): ")
	gameMode := readInput("Enter game mode (e.g., 1v1, 2v2, free-for-all): ")
	slotsStr := readInput("Enter available slots (e.g., 2, 4, 8): ")

	slots, err := strconv.ParseInt(slotsStr, 10, 8)
	if err != nil {
		log.Fatalf("Invalid number of slots: %v", err)
	}

	hostMsg := pubevts.HostRegistratioEvent{
		HostID:         hostID,
		Mode:           pubevts.GameMode(gameMode),
		AvailableSlots: uint16(slots),
	}

	msgBody, err := json.Marshal(hostMsg)
	if err != nil {
		log.Fatalf("Failed to marshal host message: %v", err)
	}

	if err := ch.PublishWithContext(
		ctx,
		pubevts.ExchangeMatchRequest,   // exchange (reusing the same direct exchange)
		pubevts.RoutingKeyMatchRequest, // routing key
		false,                          // mandatory
		false,                          // immediate
		amqp091.Publishing{
			ContentType: pubevts.ContentType,
			Type:        pubevts.MsgTypeHostRegistration,
			Body:        msgBody,
		},
	); err != nil {
		log.Fatalf("Failed to publish message: %v", err)
	}

	fmt.Printf("\nHost registration sent successfully!\n")
	fmt.Printf("Host ID: %s\n", hostID)
	fmt.Printf("Game Mode: %s\n", gameMode)
	fmt.Printf("Available Slots: %d\n", slots)
}

func registerPlayer(ctx context.Context, ch *amqp091.Channel) {
	fmt.Println("=== Player Registration ===")

	playerID := readInput("Enter player ID (or leave empty for random ID): ")
	if playerID == "" {
		playerID = fmt.Sprintf("player-%d", time.Now().UnixNano())
		fmt.Printf("Using generated player ID: %s\n", playerID)
	}

	fmt.Println("The following parameters are optional. Press Enter to skip.")

	hostID := readInput("Enter specific host ID (optional): ")
	gameMode := readInput("Enter preferred game mode (optional): ")

	playerMsg := pubevts.MatchRequestEvent{
		PlayerID: playerID,
	}

	if hostID != "" {
		playerMsg.HostID = &hostID
	}

	if gameMode != "" {
		mode := pubevts.GameMode(gameMode)
		playerMsg.Mode = &mode
	}

	msgBody, err := json.Marshal(playerMsg)
	if err != nil {
		log.Fatalf("Failed to marshal player message: %v", err)
	}

	if err := ch.PublishWithContext(
		ctx,
		pubevts.ExchangeMatchRequest,   // exchange (reusing the same direct exchange)
		pubevts.RoutingKeyMatchRequest, // routing key
		false,                          // mandatory
		false,                          // immediate
		amqp091.Publishing{
			ContentType: pubevts.ContentType,
			Type:        pubevts.MsgTypePlayerMatchRequest,
			Body:        msgBody,
		},
	); err != nil {
		log.Fatalf("Failed to publish message: %v", err)
	}

	fmt.Printf("\nPlayer registration sent successfully!\n")
	fmt.Printf("Player ID: %s\n", playerID)

	if playerMsg.HostID != nil {
		fmt.Printf("Requested Host: %s\n", *playerMsg.HostID)
	}

	if playerMsg.Mode != nil {
		fmt.Printf("Preferred Game Mode: %s\n", string(*playerMsg.Mode))
	}
}

func readInput(prompt string) string {
	fmt.Print(prompt)
	var input string
	fmt.Fscanln(os.Stdin, &input)
	return input
}
