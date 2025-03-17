package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/alesr/chachacha/internal/events"
	"github.com/alesr/chachacha/internal/matchdirector"
	"github.com/alesr/chachacha/internal/matchregistry"
	"github.com/alesr/chachacha/internal/sessionrepo"
	pubevents "github.com/alesr/chachacha/pkg/events"
	"github.com/alesr/chachacha/pkg/game"
	"github.com/oklog/ulid/v2"
	"github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatchmaking(t *testing.T) {
	ctx := context.TODO()

	redisContainer, redisAddr := startRedisContainer(t, ctx)
	defer redisContainer.Terminate(ctx)

	rabbitmqContainer, rabbitmqAddr := startRabbitMQContainer(t, ctx)
	defer rabbitmqContainer.Terminate(ctx)

	conn, ch := setupRabbitMQChannel(t, rabbitmqAddr)
	defer conn.Close()
	defer ch.Close()

	queueName := "test_matchmaking_queue"
	q, err := ch.QueueDeclare(
		queueName, // queue name
		false,     // durable
		true,      // delete when unused
		false,     // exclusive
		false,     // no-wait
		nil,       // arguments
	)
	require.NoError(t, err)

	repo, err := sessionrepo.NewRedisRepo(redisAddr)
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(os.Stdout,
		&slog.HandlerOptions{
			Level: slog.LevelDebug,
		}),
	)

	publisher, err := events.NewPublisher(ch)
	if err != nil {
		logger.Error("Failed to create event publisher", slog.String("error", err.Error()))
		// Continue without event publishing capability
	}

	registry := matchregistry.New(logger, repo, queueName, ch, publisher)

	matchInterval := 1 * time.Second
	director, err := matchdirector.New(logger, repo, matchInterval)
	require.NoError(t, err)

	go func() {
		if err := registry.Start(); err != nil {
			t.Logf("Match registry failed: %s", err)
		}
	}()

	director.Start()
	defer director.Stop()

	// Wait a moment for services to be fully initialized
	time.Sleep(2 * time.Second)

	t.Run("TestHostRegistration", func(t *testing.T) {
		testHostRegistration(t, ch, q.Name)
	})

	t.Run("TestPlayerRegistration", func(t *testing.T) {
		testPlayerRegistration(t, ch, q.Name)
	})

	t.Run("TestMatchmaking", func(t *testing.T) {
		testMatchmakingProcess(t, ch, q.Name, repo)
	})

	t.Run("TestSpecificHostRequests", func(t *testing.T) {
		testSpecificHostRequests(t, ch, q.Name, repo)
	})

	t.Run("TestGameModeFiltering", func(t *testing.T) {
		testGameModeFiltering(t, ch, q.Name, repo)
	})
}

func testHostRegistration(t *testing.T, ch *amqp091.Channel, queueName string) {
	// Create a host registration message

	hostIP := "192.168.1.100"
	gameMode := game.GameMode("1v1")

	hostMsg := game.HostRegistratioMessage{
		HostID:         hostIP,
		Mode:           gameMode,
		AvailableSlots: 2,
	}

	// Publish the host registration

	msgBody, err := json.Marshal(hostMsg)
	require.NoError(t, err)

	err = ch.PublishWithContext(
		context.Background(),
		"",        // exchange
		queueName, // routing key
		false,     // mandatory
		false,     // immediate
		amqp091.Publishing{
			ContentType: pubevents.ContentType,
			Type:        pubevents.MsgTypeHostRegistration,
			Body:        msgBody,
		},
	)
	require.NoError(t, err)

	// Wait for message processing
	time.Sleep(2 * time.Second)

	// Verify the host was registered (in the next test)
}

func testPlayerRegistration(t *testing.T, ch *amqp091.Channel, queueName string) {
	// Create a match request message

	playerID := ulid.Make().String()

	playerMsg := game.MatchRequestMessage{
		PlayerID: playerID,
	}

	// Publish the match request

	msgBody, err := json.Marshal(playerMsg)
	require.NoError(t, err)

	err = ch.PublishWithContext(
		context.Background(),
		"",        // exchange
		queueName, // routing key
		false,     // mandatory
		false,     // immediate
		amqp091.Publishing{
			ContentType: pubevents.ContentType,
			Type:        pubevents.MsgTypeMatchRequest,
			Body:        msgBody,
		},
	)
	require.NoError(t, err)

	// Wait for message processing
	time.Sleep(2 * time.Second)

	// Verification of player registration is implicit in the next test
}

func testMatchmakingProcess(t *testing.T, ch *amqp091.Channel, queueName string, repo *sessionrepo.Redis) {
	// Register another host

	hostIP := "192.168.1.101"
	gameMode := game.GameMode("2v2")

	hostMsg := game.HostRegistratioMessage{
		HostID:         hostIP,
		Mode:           gameMode,
		AvailableSlots: 4,
	}

	msgBody, err := json.Marshal(hostMsg)
	require.NoError(t, err)

	err = ch.PublishWithContext(
		context.Background(),
		"",        // exchange
		queueName, // routing key
		false,     // mandatory
		false,     // immediate
		amqp091.Publishing{
			ContentType: pubevents.ContentType,
			Type:        pubevents.MsgTypeHostRegistration,
			Body:        msgBody,
		},
	)
	require.NoError(t, err)

	// Wait for host registration
	time.Sleep(1 * time.Second)

	// Register several players for this game mode
	for i := 0; i < 3; i++ {
		playerID := fmt.Sprintf("player-%s-%d", ulid.Make().String(), i)

		playerMsg := game.MatchRequestMessage{
			PlayerID: playerID,
			Mode:     &gameMode,
		}

		msgBody, err := json.Marshal(playerMsg)
		require.NoError(t, err)

		err = ch.PublishWithContext(
			context.Background(),
			"",        // exchange
			queueName, // routing key
			false,     // mandatory
			false,     // immediate
			amqp091.Publishing{
				ContentType: pubevents.ContentType,
				Type:        pubevents.MsgTypeMatchRequest,
				Body:        msgBody,
			},
		)
		require.NoError(t, err)
	}

	// Wait for match processing to complete
	time.Sleep(3 * time.Second)

	// Check if a game session was created
	sessions, err := repo.GetActiveGameSessions(context.TODO())
	require.NoError(t, err)

	// At least one session should exist
	assert.GreaterOrEqual(t, len(sessions), 1)

	// Check if the session contains players
	if len(sessions) > 0 {
		// Find the 2v2 session
		var session *sessionrepo.Session
		for _, s := range sessions {
			if s.Mode == gameMode {
				session = s
				break
			}
		}

		if session != nil {
			assert.GreaterOrEqual(t, len(session.Players), 1)
			assert.Equal(t, gameMode, session.Mode)
			assert.Equal(t, hostIP, session.HostIP)
		} else {
			t.Log("No 2v2 session found, which is unexpected")
			t.Fail()
		}
	}
}

func testSpecificHostRequests(t *testing.T, ch *amqp091.Channel, queueName string, repo *sessionrepo.Redis) {
	// Register a new host

	hostIP := "192.168.1.102"
	gameMode := game.GameMode("free-for-all")

	hostMsg := game.HostRegistratioMessage{
		HostID:         hostIP,
		Mode:           gameMode,
		AvailableSlots: 8,
	}

	msgBody, err := json.Marshal(hostMsg)
	require.NoError(t, err)

	err = ch.PublishWithContext(
		context.Background(),
		"",        // exchange
		queueName, // routing key
		false,     // mandatory
		false,     // immediate
		amqp091.Publishing{
			ContentType: pubevents.ContentType,
			Type:        pubevents.MsgTypeHostRegistration,
			Body:        msgBody,
		},
	)
	require.NoError(t, err)

	time.Sleep(1 * time.Second)

	// Register players that specifically request this host
	for i := 0; i < 4; i++ {
		playerID := fmt.Sprintf("specific-player-%s-%d", ulid.Make().String(), i)

		playerMsg := game.MatchRequestMessage{
			PlayerID: playerID,
			HostID:   &hostIP,
		}

		msgBody, err := json.Marshal(playerMsg)
		require.NoError(t, err)

		err = ch.PublishWithContext(
			context.Background(),
			"",        // exchange
			queueName, // routing key
			false,     // mandatory
			false,     // immediate
			amqp091.Publishing{
				ContentType: pubevents.ContentType,
				Type:        pubevents.MsgTypeMatchRequest,
				Body:        msgBody,
			},
		)
		require.NoError(t, err)
	}

	time.Sleep(3 * time.Second)

	// Check if a game session was created with the specific host

	sessions, err := repo.GetActiveGameSessions(context.TODO())
	require.NoError(t, err)

	var specificHostSession *sessionrepo.Session
	for _, s := range sessions {
		if s.HostIP == hostIP {
			specificHostSession = s
			break
		}
	}

	assert.NotNil(t, specificHostSession)
	if specificHostSession != nil {
		assert.Equal(t, gameMode, specificHostSession.Mode)
		assert.GreaterOrEqual(t, len(specificHostSession.Players), 1)
	}
}

func testGameModeFiltering(t *testing.T, ch *amqp091.Channel, queueName string, repo *sessionrepo.Redis) {
	// Register two hosts with different game modes

	hostIP1 := "192.168.1.103"
	gameMode1 := game.GameMode("duel")

	hostMsg1 := game.HostRegistratioMessage{
		HostID:         hostIP1,
		Mode:           gameMode1,
		AvailableSlots: 2,
	}

	msgBody, err := json.Marshal(hostMsg1)
	require.NoError(t, err)

	err = ch.PublishWithContext(
		context.Background(),
		"",        // exchange
		queueName, // routing key
		false,     // mandatory
		false,     // immediate
		amqp091.Publishing{
			ContentType: pubevents.ContentType,
			Type:        pubevents.MsgTypeHostRegistration,
			Body:        msgBody,
		},
	)
	require.NoError(t, err)

	hostIP2 := "192.168.1.104"
	gameMode2 := game.GameMode("survival")

	hostMsg2 := game.HostRegistratioMessage{
		HostID:         hostIP2,
		Mode:           gameMode2,
		AvailableSlots: 10,
	}

	msgBody, err = json.Marshal(hostMsg2)
	require.NoError(t, err)

	err = ch.PublishWithContext(
		context.Background(),
		"",        // exchange
		queueName, // routing key
		false,     // mandatory
		false,     // immediate
		amqp091.Publishing{
			ContentType: pubevents.ContentType,
			Type:        pubevents.MsgTypeHostRegistration,
			Body:        msgBody,
		},
	)
	require.NoError(t, err)

	// Wait for host registrations
	time.Sleep(1 * time.Second)

	// Register players with specific game mode requests
	// Players wanting Duels
	for i := 0; i < 2; i++ {
		playerID := fmt.Sprintf("duel-player-%s-%d", ulid.Make().String(), i)

		playerMsg := game.MatchRequestMessage{
			PlayerID: playerID,
			Mode:     &gameMode1,
		}

		msgBody, err := json.Marshal(playerMsg)
		require.NoError(t, err)

		err = ch.PublishWithContext(
			context.Background(),
			"",        // exchange
			queueName, // routing key
			false,     // mandatory
			false,     // immediate
			amqp091.Publishing{
				ContentType: pubevents.ContentType,
				Type:        pubevents.MsgTypeMatchRequest,
				Body:        msgBody,
			},
		)
		require.NoError(t, err)
	}

	// Players wanting Survival
	for i := 0; i < 3; i++ {
		playerID := fmt.Sprintf("survival-player-%s-%d", ulid.Make().String(), i)

		playerMsg := game.MatchRequestMessage{
			PlayerID: playerID,
			Mode:     &gameMode2,
		}

		msgBody, err := json.Marshal(playerMsg)
		require.NoError(t, err)

		err = ch.PublishWithContext(
			context.Background(),
			"",        // exchange
			queueName, // routing key
			false,     // mandatory
			false,     // immediate
			amqp091.Publishing{
				ContentType: pubevents.ContentType,
				Type:        pubevents.MsgTypeMatchRequest,
				Body:        msgBody,
			},
		)
		require.NoError(t, err)
	}

	time.Sleep(3 * time.Second)

	// Check if game sessions were created with the correct game modes

	sessions, err := repo.GetActiveGameSessions(context.TODO())
	require.NoError(t, err)

	var duelsSession, survivalSession *sessionrepo.Session
	for _, s := range sessions {
		if s.Mode == gameMode1 {
			duelsSession = s
		}
		if s.Mode == gameMode2 {
			survivalSession = s
		}
	}

	// Verify the duel session
	assert.NotNil(t, duelsSession)
	if duelsSession != nil {
		assert.Equal(t, hostIP1, duelsSession.HostIP)
		assert.GreaterOrEqual(t, len(duelsSession.Players), 1)
	}

	// Verify the survival session
	assert.NotNil(t, survivalSession)
	if survivalSession != nil {
		assert.Equal(t, hostIP2, survivalSession.HostIP)
		assert.GreaterOrEqual(t, len(survivalSession.Players), 1)
	}
}
