package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/alesr/chachacha/internal/config"
	"github.com/alesr/chachacha/internal/events"
	"github.com/alesr/chachacha/internal/sessionrepo"
	pubevts "github.com/alesr/chachacha/pkg/events"
	"github.com/rabbitmq/amqp091-go"
)

// Event holds parsed event information for display.
type Event struct {
	Type    string
	Message string
	Data    interface{}
}

var (
	eventsChannel = make(chan Event, 100)
	stopWaiting   = make(chan bool)
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	rabbitMQURL := flag.String("rabbitmq", cfg.RabbitMQURL, "RabbitMQ connection URL")
	redisAddr := flag.String("redis", cfg.RedisAddr, "Redis address")
	queueName := flag.String("queue", cfg.QueueName, "RabbitMQ queue name")
	mode := flag.String("mode", "", "Operation mode: 'host' for registering a host, 'player' for registering a player, 'remove-host' for removing a host, 'remove-player' for removing a player, 'status' for showing current state")
	verbose := flag.Bool("verbose", false, "Enable verbose output for events")
	flag.Parse()

	validModes := map[string]bool{
		"host":          true,
		"player":        true,
		"remove-host":   true,
		"remove-player": true,
		"status":        true,
	}

	if !validModes[*mode] {
		fmt.Println("Available modes:")
		fmt.Println("  host          - Register a new game host")
		fmt.Println("  player        - Register a player looking for a match")
		fmt.Println("  remove-host   - Remove a host from matchmaking")
		fmt.Println("  remove-player - Remove a player from matchmaking")
		fmt.Println("  status        - Show current matchmaking status")
		fmt.Println("")
		fmt.Println("Options:")
		fmt.Println("  -verbose      - Show detailed event information")
		os.Exit(1)
	}

	redisCli, err := sessionrepo.NewRedisClient(*redisAddr)
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	repo, err := sessionrepo.NewRedisRepo(redisCli)
	if err != nil {
		log.Fatalf("Failed to create Redis repo: %v", err)
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

	if err := events.SetupInputExchangeBindings(ch, *queueName); err != nil {
		log.Fatalf("Failed to set up input exchange queue bindings: %v", err)
	}

	if err := events.SetupOutputExchangeQueueBindings(ch); err != nil {
		log.Fatalf("Failed to set up monitoring binding queues: %v", err)
	}

	if *mode != "status" {
		go displayEvents(*verbose)
		setupEventObservers(ch)
	}

	if *mode != "status" {
		fmt.Println("\n=== Current Matchmaking State (Before Operation) ===")
		displayCurrentState(repo)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	switch *mode {
	case "host":
		registerHost(ctx, ch)
	case "player":
		registerPlayer(ctx, ch)
	case "remove-host":
		removeHost(ctx, ch)
	case "remove-player":
		removePlayer(ctx, ch)
	case "status":
		showStatus(ctx, repo)
	}

	// only wait for events and display final state for non-status operations
	if *mode != "status" {
		waitForEvents()

		fmt.Println("\n=== Current Matchmaking State (After Operation) ===")
		displayCurrentState(repo)
	}
}

func displayEvents(verbose bool) {
	for {
		select {
		case evt := <-eventsChannel:
			switch evt.Type {
			case "game_created":
				fmt.Printf("\n🎮 GAME CREATED: %s\n", evt.Message)
			case "player_join_requested":
				fmt.Printf("\n👤 PLAYER JOIN REQUESTED: %s\n", evt.Message)
			case "player_joined":
				fmt.Printf("\n✅ PLAYER JOINED: %s\n", evt.Message)
			default:
				fmt.Printf("\n🔔 EVENT (%s): %s\n", evt.Type, evt.Message)
			}

			if verbose {
				data, _ := json.MarshalIndent(evt.Data, "    ", "  ")
				fmt.Printf("    Details: %s\n", string(data))
			}
		case <-stopWaiting:
			return
		}
	}
}

func setupEventObservers(ch *amqp091.Channel) {
	monitorQueues := []string{
		"monitor." + pubevts.ExchangeGameCreated,
		"monitor." + pubevts.ExchangePlayerJoinRequested,
		"monitor." + pubevts.ExchangePlayerJoined,
	}

	typeMapping := map[string]string{
		"monitor." + pubevts.ExchangeGameCreated:         "game_created",
		"monitor." + pubevts.ExchangePlayerJoinRequested: "player_join_requested",
		"monitor." + pubevts.ExchangePlayerJoined:        "player_joined",
	}

	for _, queueName := range monitorQueues {
		msgs, err := ch.Consume(
			queueName,
			"",    // consumer
			true,  // auto-ack
			false, // exclusive
			false, // no-local
			false, // no-wait
			nil,   // args
		)
		if err != nil {
			log.Printf("Warning: Failed to set up consumer for %s: %v", queueName, err)
			continue
		}

		go func(queue string, eventType string) {
			for msg := range msgs {
				switch eventType {
				case "game_created":
					var data pubevts.GameCreatedEvent
					if err := json.Unmarshal(msg.Body, &data); err == nil {
						eventsChannel <- Event{
							Type: eventType,
							Message: fmt.Sprintf("Host '%s' created game with %d slots, mode: %s",
								data.HostID, data.MaxPlayers, data.GameMode),
							Data: data,
						}
					}
				case "player_join_requested":
					var data pubevts.PlayerJoinRequestedEvent
					if err := json.Unmarshal(msg.Body, &data); err == nil {
						hostInfo := "any available host"
						if data.HostID != nil && *data.HostID != "" {
							hostInfo = "host '" + *data.HostID + "'"
						}

						modeInfo := "any game mode"
						if data.GameMode != nil && *data.GameMode != "" {
							modeInfo = string(*data.GameMode)
						}

						eventsChannel <- Event{
							Type: eventType,
							Message: fmt.Sprintf("Player '%s' looking to join %s, mode: %s",
								data.PlayerID, hostInfo, modeInfo),
							Data: data,
						}
					}
				case "player_joined":
					var data pubevts.PlayerJoinedEvent
					if err := json.Unmarshal(msg.Body, &data); err == nil {
						eventsChannel <- Event{
							Type: eventType,
							Message: fmt.Sprintf("Player '%s' joined host '%s' (%d/%d players)",
								data.PlayerID, data.HostID, data.CurrentPlayerCount, data.MaxPlayers),
							Data: data,
						}
					}
				default:
					eventsChannel <- Event{
						Type:    "unknown",
						Message: string(msg.Body),
						Data:    nil,
					}
				}
			}
		}(queueName, typeMapping[queueName])
	}
}

func displayCurrentState(repo *sessionrepo.Redis) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()

	hosts, err := repo.GetHosts(ctx)
	if err != nil {
		fmt.Printf("Error getting hosts: %v\n", err)
		return
	}

	players, err := repo.GetPlayers(ctx)
	if err != nil {
		fmt.Printf("Error getting players: %v\n", err)
		return
	}

	sessions, err := repo.GetActiveGameSessions(ctx)
	if err != nil {
		fmt.Printf("Error getting active sessions: %v\n", err)
		return
	}

	fmt.Printf("🎮 Hosts: %d  |  ", len(hosts))
	fmt.Printf("👤 Players waiting: %d  |  ", len(players))
	fmt.Printf("🎲 Active game sessions: %d\n", len(sessions))
}

func registerHost(ctx context.Context, ch *amqp091.Channel) {
	fmt.Println("\n=== 🎮 Host Registration ===")
	fmt.Println("Create a new game host that players can join")

	hostID := readInput("Host ID (or leave empty for random): ")
	if hostID == "" {
		hostID = fmt.Sprintf("host-%d", time.Now().UnixNano())
		fmt.Printf("Using generated host ID: %s\n", hostID)
	}

	gameMode := readInput("Game mode (e.g., 1v1, 2v2, battle-royale): ")
	slotsStr := readInput("Available player slots: ")

	slots, err := strconv.ParseInt(slotsStr, 10, 16)
	if err != nil || slots <= 0 {
		fmt.Println("Invalid number of slots. Using default value of 4.")
		slots = 4
	}

	hostMsg := pubevts.HostRegistrationEvent{
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
		pubevts.ExchangeMatchRequest,
		pubevts.RoutingKeyHostRegistration,
		false,
		false,
		amqp091.Publishing{
			ContentType: pubevts.ContentType,
			Type:        pubevts.MsgTypeHostRegistration,
			Body:        msgBody,
		},
	); err != nil {
		log.Fatalf("Failed to publish message: %v", err)
	}

	fmt.Printf("\n✅ Host registration sent successfully!\n")
	fmt.Printf("   Host ID: %s\n", hostID)
	fmt.Printf("   Game Mode: %s\n", gameMode)
	fmt.Printf("   Available Slots: %d\n", slots)
	fmt.Println("\nWaiting for events... (This will take a few seconds)")
	fmt.Println("You should see GameCreated events appear shortly.")
}

func registerPlayer(ctx context.Context, ch *amqp091.Channel) {
	fmt.Println("\n=== 👤 Player Registration ===")
	fmt.Println("Register a player looking for a match")

	playerID := readInput("Player ID (or leave empty for random ID): ")
	if playerID == "" {
		playerID = fmt.Sprintf("player-%d", time.Now().UnixNano())
		fmt.Printf("Using generated player ID: %s\n", playerID)
	}

	fmt.Println("\nThe following parameters are optional. Press Enter to skip.")

	hostID := readInput("Specific host ID to join (optional): ")
	gameMode := readInput("Preferred game mode (optional): ")

	playerMsg := pubevts.PlayerMatchRequestEvent{
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
		pubevts.ExchangeMatchRequest,
		pubevts.RoutingKeyPlayerMatchRequest,
		false,
		false,
		amqp091.Publishing{
			ContentType: pubevts.ContentType,
			Type:        pubevts.MsgTypePlayerMatchRequest,
			Body:        msgBody,
		},
	); err != nil {
		log.Fatalf("Failed to publish message: %v", err)
	}

	fmt.Printf("\n✅ Player registration sent successfully!\n")
	fmt.Printf("   Player ID: %s\n", playerID)

	if playerMsg.HostID != nil && *playerMsg.HostID != "" {
		fmt.Printf("   Requested Host: %s\n", *playerMsg.HostID)
	} else {
		fmt.Printf("   No specific host requested (will match with any compatible host)\n")
	}

	if playerMsg.Mode != nil && *playerMsg.Mode != "" {
		fmt.Printf("   Preferred Game Mode: %s\n", string(*playerMsg.Mode))
	} else {
		fmt.Printf("   No game mode preference (will match with any game mode)\n")
	}

	fmt.Println("\nWaiting for events... (This will take a few seconds)")
	fmt.Println("You should see PlayerJoinRequested events appear shortly.")
	fmt.Println("If there's a matching host, you may see additional events as the player is matched.")
}

func removeHost(ctx context.Context, ch *amqp091.Channel) {
	fmt.Println("\n=== 🔄 Host Removal ===")
	fmt.Println("Remove a host from matchmaking")

	hostID := readInput("Host ID to remove: ")

	hostMsg := pubevts.HostRegistrationRemovalEvent{
		HostID: hostID,
	}

	msgBody, err := json.Marshal(hostMsg)
	if err != nil {
		log.Fatalf("Failed to marshal host removal message: %v", err)
	}

	if err := ch.PublishWithContext(
		ctx,
		pubevts.ExchangeMatchRequest,
		pubevts.RoutingKeyHostRegistrationRemoval,
		false,
		false,
		amqp091.Publishing{
			ContentType: pubevts.ContentType,
			Type:        pubevts.MsgTypeHostRegistrationRemoval,
			Body:        msgBody,
		},
	); err != nil {
		log.Fatalf("Failed to publish message: %v", err)
	}

	fmt.Printf("\n✅ Host removal request sent successfully!\n")
	fmt.Printf("   Host ID: %s\n", hostID)
	fmt.Println("\nWaiting for events... (This will take a few seconds)")
	fmt.Println("If the host had active players, you should see PlayerJoinRequested events")
	fmt.Println("as those players are placed back into matchmaking.")
}

func removePlayer(ctx context.Context, ch *amqp091.Channel) {
	fmt.Println("\n=== 🔄 Player Removal ===")
	fmt.Println("Remove a player from matchmaking")

	playerID := readInput("Player ID to remove: ")

	playerMsg := pubevts.PlayerMatchRequestRemovalEvent{
		PlayerID: playerID,
	}

	msgBody, err := json.Marshal(playerMsg)
	if err != nil {
		log.Fatalf("Failed to marshal player removal message: %v", err)
	}

	if err := ch.PublishWithContext(
		ctx,
		pubevts.ExchangeMatchRequest,
		pubevts.RoutingKeyPlayerMatchRequestRemoval,
		false,
		false,
		amqp091.Publishing{
			ContentType: pubevts.ContentType,
			Type:        pubevts.MsgTypePlayerMatchRequestRemoval,
			Body:        msgBody,
		},
	); err != nil {
		log.Fatalf("Failed to publish message: %v", err)
	}

	fmt.Printf("\n✅ Player removal request sent successfully!\n")
	fmt.Printf("   Player ID: %s\n", playerID)
	fmt.Println("\nWaiting for events... (This will take a few seconds)")
	fmt.Println("If the player was in an active session, you may see events related to slot updates.")
}

func showStatus(ctx context.Context, repo *sessionrepo.Redis) {
	fmt.Println("\n=== 📊 Matchmaking Status ===")

	hosts, err := repo.GetHosts(ctx)
	if err != nil {
		log.Fatalf("Failed to get hosts: %v", err)
	}

	players, err := repo.GetPlayers(ctx)
	if err != nil {
		log.Fatalf("Failed to get players: %v", err)
	}

	sessions, err := repo.GetActiveGameSessions(ctx)
	if err != nil {
		log.Fatalf("Failed to get active sessions: %v", err)
	}

	fmt.Printf("\n🎮 Available Hosts (%d):\n", len(hosts))
	if len(hosts) == 0 {
		fmt.Println("   No hosts available")
	} else {
		for i, host := range hosts {
			fmt.Printf("   %d. Host ID: %s\n", i+1, host.HostID)
			fmt.Printf("      Game Mode: %s\n", host.Mode)
			fmt.Printf("      Available Slots: %d\n", host.AvailableSlots)
			fmt.Println()
		}
	}

	fmt.Printf("\n👤 Players Looking for Matches (%d):\n", len(players))
	if len(players) == 0 {
		fmt.Println("   No players waiting")
	} else {
		for i, player := range players {
			fmt.Printf("   %d. Player ID: %s\n", i+1, player.PlayerID)
			if player.HostID != nil && *player.HostID != "" {
				fmt.Printf("      Preferred Host: %s\n", *player.HostID)
			} else {
				fmt.Printf("      Preferred Host: Any\n")
			}
			if player.Mode != nil && *player.Mode != "" {
				fmt.Printf("      Preferred Game Mode: %s\n", *player.Mode)
			} else {
				fmt.Printf("      Preferred Game Mode: Any\n")
			}
			fmt.Println()
		}
	}

	fmt.Printf("\n🎲 Active Game Sessions (%d):\n", len(sessions))
	if len(sessions) == 0 {
		fmt.Println("   No active sessions")
	} else {
		for i, session := range sessions {
			fmt.Printf("   %d. Session ID: %s\n", i+1, session.ID)
			fmt.Printf("      Host ID: %s\n", session.HostID)
			fmt.Printf("      Game Mode: %s\n", session.Mode)
			fmt.Printf("      State: %s\n", session.State)
			fmt.Printf("      Available Slots: %d\n", session.AvailableSlots)
			fmt.Printf("      Players (%d): %s\n", len(session.Players), strings.Join(session.Players, ", "))
			fmt.Printf("      Created: %s\n", session.CreatedAt.Format(time.RFC3339))
			fmt.Println()
		}
	}

	fmt.Println("\nTip: Run other commands to modify the matchmaking state:")
	fmt.Println("  ./test/manual/run-test.sh -mode host          # Register a new host")
	fmt.Println("  ./test/manual/run-test.sh -mode player        # Register a new player")
	fmt.Println("  ./test/manual/run-test.sh -mode remove-host   # Remove a host")
	fmt.Println("  ./test/manual/run-test.sh -mode remove-player # Remove a player")
}

func readInput(prompt string) string {
	fmt.Print(prompt)
	var input string
	fmt.Fscanln(os.Stdin, &input)
	return strings.TrimSpace(input)
}

func waitForEvents() {
	waitTimeout := 10 * time.Second
	fmt.Printf("Watching for events (timeout: %s)...\n", waitTimeout)

	timer := time.NewTimer(waitTimeout)
	<-timer.C

	stopWaiting <- true
}
