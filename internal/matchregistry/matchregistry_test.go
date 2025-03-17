package matchregistry

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	pubevents "github.com/alesr/chachacha/pkg/events"
	"github.com/alesr/chachacha/pkg/game"
	"github.com/alesr/chachacha/pkg/logutils"
	"github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Parallel()

	givenLogger := logutils.NewNoop()
	givenRepo := repoMock{}
	givenConsumerQueueName := "foo"
	givenConsumer := consumerMock{}
	givenPublisher := publisherMock{}

	got := New(givenLogger, &givenRepo, givenConsumerQueueName, &givenConsumer, &givenPublisher)

	require.NotNil(t, got)

	assert.Equal(t, givenLogger.WithGroup("match_registry"), got.logger)
	assert.Equal(t, &givenRepo, got.repo)
	assert.Equal(t, givenConsumerQueueName, got.consumerQueueName)
	assert.Equal(t, &givenConsumer, got.consumer)
	assert.Equal(t, &givenPublisher, got.publisher)
}

func TestMatchRegistry_Start(t *testing.T) {
	logger := logutils.NewNoop()

	t.Run("Process host registration message", func(t *testing.T) {
		repo := repoMock{
			storeHostFunc:   func(ctx context.Context, host game.HostRegistratioMessage) error { return nil },
			storePlayerFunc: func(ctx context.Context, player game.MatchRequestMessage) error { return nil },
		}

		// use controlled delivery channel to send values to consumer
		deliveryCh := make(chan amqp091.Delivery)
		consumer := consumerMock{
			consumeFunc: func(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp091.Table) (<-chan amqp091.Delivery, error) {
				return deliveryCh, nil
			},
		}

		publisher := publisherMock{
			publishGameCreatedFunc:         func(ctx context.Context, event pubevents.GameCreatedEvent) error { return nil },
			publishPlayerJoinRequestedFunc: func(ctx context.Context, event pubevents.PlayerJoinRequestedEvent) error { return nil },
		}

		mr := New(logger, &repo, "foo-consumer-queue-name", &consumer, &publisher)

		// run Start in a goroutine, as it contains an infinite loop
		errCh := make(chan error)
		go func() {
			errCh <- mr.Start()
		}()

		// give it time to init
		time.Sleep(100 * time.Millisecond)

		hostMsg := game.HostRegistratioMessage{
			HostID:         "test-host-123",
			Mode:           game.GameMode("foo-mode"),
			AvailableSlots: 2,
		}

		msgBody, err := json.Marshal(hostMsg)
		require.NoError(t, err)

		// send the message through the mock channel
		deliveryCh <- amqp091.Delivery{
			Type: pubevents.MsgTypeHostRegistration,
			Body: msgBody,
		}

		// give it time to process the msg
		time.Sleep(100 * time.Millisecond)

		assert.True(t, repo.wasStoreHostCalled())
		assert.False(t, repo.wasStorePlayerCalled())
		assert.True(t, publisher.wasPublishGameCreatedCalled())
		assert.False(t, publisher.wasPublishPlayerJoinRequestedCalled())

		mr.Shutdown()
	})

	t.Run("Process match request message", func(t *testing.T) {

		repo := repoMock{
			storeHostFunc:   func(ctx context.Context, host game.HostRegistratioMessage) error { return nil },
			storePlayerFunc: func(ctx context.Context, player game.MatchRequestMessage) error { return nil },
		}

		// use controlled delivery channel to send values to consumer
		deliveryCh := make(chan amqp091.Delivery)
		consumer := consumerMock{
			consumeFunc: func(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp091.Table) (<-chan amqp091.Delivery, error) {
				return deliveryCh, nil
			},
		}

		publisher := publisherMock{
			publishGameCreatedFunc:         func(ctx context.Context, event pubevents.GameCreatedEvent) error { return nil },
			publishPlayerJoinRequestedFunc: func(ctx context.Context, event pubevents.PlayerJoinRequestedEvent) error { return nil },
		}

		mr := New(logger, &repo, "foo-consumer-queue-name", &consumer, &publisher)

		// run Start in a goroutine, as it contains an infinite loop
		errCh := make(chan error)
		go func() {
			errCh <- mr.Start()
		}()

		// give it time to init
		time.Sleep(100 * time.Millisecond)

		gameMode := game.GameMode("foo-mode")

		hostMsg := game.MatchRequestMessage{
			PlayerID: "foo",
			HostID:   stringToPtr("test-host-123"),
			Mode:     &gameMode,
		}

		msgBody, err := json.Marshal(hostMsg)
		require.NoError(t, err)

		// send the message through the mock channel
		deliveryCh <- amqp091.Delivery{
			Type: pubevents.MsgTypeMatchRequest,
			Body: msgBody,
		}

		// give it time to process the msg
		time.Sleep(100 * time.Millisecond)

		assert.False(t, repo.wasStoreHostCalled())
		assert.True(t, repo.wasStorePlayerCalled())
		assert.False(t, publisher.wasPublishGameCreatedCalled())
		assert.True(t, publisher.wasPublishPlayerJoinRequestedCalled())

		mr.Shutdown()
	})
}

func stringToPtr(s string) *string {
	return &s
}
