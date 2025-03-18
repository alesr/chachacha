package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/alesr/chachacha/internal/events"
	"github.com/alesr/chachacha/internal/matchdirector"
	"github.com/alesr/chachacha/internal/matchregistry"
	"github.com/alesr/chachacha/internal/sessionrepo"
	pubevts "github.com/alesr/chachacha/pkg/events"
	"github.com/alesr/chachacha/pkg/logutils"
	"github.com/oklog/ulid/v2"
	"github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatchmaking(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e tests")
	}

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

	redisCli, err := sessionrepo.NewRedisClient(redisAddr)
	require.NoError(t, err)

	repo, err := sessionrepo.NewRedisRepo(redisCli)
	require.NoError(t, err)

	logger := logutils.NewNoop()

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

	hostID := "111"
	gameMode := pubevts.GameMode("1v1")

	hostMsg := pubevts.HostRegistratioEvent{
		HostID:         hostID,
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
			ContentType: pubevts.ContentType,
			Type:        pubevts.MsgTypeHostRegistration,
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

	playerMsg := pubevts.MatchRequestEvent{
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
			ContentType: pubevts.ContentType,
			Type:        pubevts.MsgTypePlayerMatchRequest,
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

	hostID := "222"
	gameMode := pubevts.GameMode("2v2")

	hostMsg := pubevts.HostRegistratioEvent{
		HostID:         hostID,
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
			ContentType: pubevts.ContentType,
			Type:        pubevts.MsgTypeHostRegistration,
			Body:        msgBody,
		},
	)
	require.NoError(t, err)

	// Wait for host registration
	time.Sleep(1 * time.Second)

	// Register several players for this game mode
	for i := 0; i < 3; i++ {
		playerID := fmt.Sprintf("player-%s-%d", ulid.Make().String(), i)

		playerMsg := pubevts.MatchRequestEvent{
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
				ContentType: pubevts.ContentType,
				Type:        pubevts.MsgTypePlayerMatchRequest,
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
			assert.Equal(t, hostID, session.HostID)
		} else {
			t.Log("No 2v2 session found, which is unexpected")
			t.Fail()
		}
	}
}

func testSpecificHostRequests(t *testing.T, ch *amqp091.Channel, queueName string, repo *sessionrepo.Redis) {
	// Register a new host

	hostID := "123"
	gameMode := pubevts.GameMode("free-for-all")

	hostMsg := pubevts.HostRegistratioEvent{
		HostID:         hostID,
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
			ContentType: pubevts.ContentType,
			Type:        pubevts.MsgTypeHostRegistration,
			Body:        msgBody,
		},
	)
	require.NoError(t, err)

	time.Sleep(1 * time.Second)

	// Register players that specifically request this host
	for i := 0; i < 4; i++ {
		playerID := fmt.Sprintf("specific-player-%s-%d", ulid.Make().String(), i)

		playerMsg := pubevts.MatchRequestEvent{
			PlayerID: playerID,
			HostID:   &hostID,
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
				ContentType: pubevts.ContentType,
				Type:        pubevts.MsgTypePlayerMatchRequest,
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
		if s.HostID == hostID {
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

	hostID1 := "321"
	gameMode1 := pubevts.GameMode("duel")

	hostMsg1 := pubevts.HostRegistratioEvent{
		HostID:         hostID1,
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
			ContentType: pubevts.ContentType,
			Type:        pubevts.MsgTypeHostRegistration,
			Body:        msgBody,
		},
	)
	require.NoError(t, err)

	hostID2 := "456"
	gameMode2 := pubevts.GameMode("survival")

	hostMsg2 := pubevts.HostRegistratioEvent{
		HostID:         hostID2,
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
			ContentType: pubevts.ContentType,
			Type:        pubevts.MsgTypeHostRegistration,
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

		playerMsg := pubevts.MatchRequestEvent{
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
				ContentType: pubevts.ContentType,
				Type:        pubevts.MsgTypePlayerMatchRequest,
				Body:        msgBody,
			},
		)
		require.NoError(t, err)
	}

	// Players wanting Survival
	for i := 0; i < 3; i++ {
		playerID := fmt.Sprintf("survival-player-%s-%d", ulid.Make().String(), i)

		playerMsg := pubevts.MatchRequestEvent{
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
				ContentType: pubevts.ContentType,
				Type:        pubevts.MsgTypePlayerMatchRequest,
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
		assert.Equal(t, hostID1, duelsSession.HostID)
		assert.GreaterOrEqual(t, len(duelsSession.Players), 1)
	}

	// Verify the survival session
	assert.NotNil(t, survivalSession)
	if survivalSession != nil {
		assert.Equal(t, hostID2, survivalSession.HostID)
		assert.GreaterOrEqual(t, len(survivalSession.Players), 1)
	}
}
