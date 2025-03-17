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
	"github.com/alesr/chachacha/pkg/game"
	"github.com/rabbitmq/amqp091-go"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Define command line flags for custom configuration
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

	// Ensure the queue exists
	_, err = ch.QueueDeclare(
		*queueName, // queue name
		false,      // durable
		false,      // delete when unused
		false,      // exclusive
		false,      // no-wait
		nil,        // arguments
	)
	if err != nil {
		log.Fatalf("Failed to declare a queue: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	switch *mode {
	case "host":
		registerHost(ctx, ch, *queueName)
	case "player":
		registerPlayer(ctx, ch, *queueName)
	}
}

func registerHost(ctx context.Context, ch *amqp091.Channel, queueName string) {
	fmt.Println("=== Host Registration ===")

	// Get host info
	hostIP := readInput("Enter host IP (e.g., 192.168.1.100): ")
	gameMode := readInput("Enter game mode (e.g., 1v1, 2v2, free-for-all): ")
	slotsStr := readInput("Enter available slots (e.g., 2, 4, 8): ")

	slots, err := strconv.ParseInt(slotsStr, 10, 8)
	if err != nil {
		log.Fatalf("Invalid number of slots: %v", err)
	}

	// Create host registration message
	hostMsg := game.HostRegistratioMessage{
		HostIP:         hostIP,
		Mode:           game.GameMode(gameMode),
		AvailableSlots: int8(slots),
	}

	// Publish the message
	msgBody, err := json.Marshal(hostMsg)
	if err != nil {
		log.Fatalf("Failed to marshal host message: %v", err)
	}

	err = ch.PublishWithContext(
		ctx,
		"",        // exchange
		queueName, // routing key
		false,     // mandatory
		false,     // immediate
		amqp091.Publishing{
			ContentType: "application/json",
			Type:        "host_registration",
			Body:        msgBody,
		},
	)
	if err != nil {
		log.Fatalf("Failed to publish message: %v", err)
	}

	fmt.Printf("\nHost registration sent successfully!\n")
	fmt.Printf("Host IP: %s\n", hostIP)
	fmt.Printf("Game Mode: %s\n", gameMode)
	fmt.Printf("Available Slots: %d\n", slots)
}

func registerPlayer(ctx context.Context, ch *amqp091.Channel, queueName string) {
	fmt.Println("=== Player Registration ===")

	// Get player info
	playerID := readInput("Enter player ID (or leave empty for random ID): ")
	if playerID == "" {
		playerID = fmt.Sprintf("player-%d", time.Now().UnixNano())
		fmt.Printf("Using generated player ID: %s\n", playerID)
	}

	// Optional parameters
	fmt.Println("The following parameters are optional. Press Enter to skip.")

	hostIP := readInput("Enter specific host IP (optional): ")
	gameMode := readInput("Enter preferred game mode (optional): ")

	// Create player registration message
	playerMsg := game.MatchRequestMessage{
		PlayerID: playerID,
	}

	// Set optional fields
	if hostIP != "" {
		playerMsg.HostIP = &hostIP
	}

	if gameMode != "" {
		mode := game.GameMode(gameMode)
		playerMsg.Mode = &mode
	}

	// Publish the message
	msgBody, err := json.Marshal(playerMsg)
	if err != nil {
		log.Fatalf("Failed to marshal player message: %v", err)
	}

	err = ch.PublishWithContext(
		ctx,
		"",        // exchange
		queueName, // routing key
		false,     // mandatory
		false,     // immediate
		amqp091.Publishing{
			ContentType: "application/json",
			Type:        "match_request",
			Body:        msgBody,
		},
	)
	if err != nil {
		log.Fatalf("Failed to publish message: %v", err)
	}

	fmt.Printf("\nPlayer registration sent successfully!\n")
	fmt.Printf("Player ID: %s\n", playerID)
	if playerMsg.HostIP != nil {
		fmt.Printf("Requested Host: %s\n", *playerMsg.HostIP)
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
