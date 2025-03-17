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
	t.Parallel()

	logger := logutils.NewNoop()

	hostMsg := game.HostRegistratioMessage{
		HostID:         "test-host-123",
		Mode:           game.GameMode("foo-mode"),
		AvailableSlots: 2,
	}

	hostMsgBody, err := json.Marshal(hostMsg)
	require.NoError(t, err)

	gameMode := game.GameMode("foo-mode")

	matchMsg := game.MatchRequestMessage{
		PlayerID: "foo",
		HostID:   stringToPtr("test-host-123"),
		Mode:     &gameMode,
	}

	matchMsgBody, err := json.Marshal(matchMsg)
	require.NoError(t, err)

	testCases := []struct {
		name                               string
		givenDelivery                      amqp091.Delivery
		expectStoreHost                    bool
		expectStorePlayer                  bool
		expectPublishGameCreated           bool
		expectedPublishPlayerJoinRequested bool
	}{
		{
			name: "host registration",
			givenDelivery: amqp091.Delivery{
				Type: pubevents.MsgTypeHostRegistration,
				Body: hostMsgBody,
			},
			expectStoreHost:          true,
			expectPublishGameCreated: true,
		},
		{
			name: "match request",
			givenDelivery: amqp091.Delivery{
				Type: pubevents.MsgTypeMatchRequest,
				Body: matchMsgBody,
			},
			expectStorePlayer:                  true,
			expectedPublishPlayerJoinRequested: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

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

			// send the message through the mock channel
			deliveryCh <- tc.givenDelivery

			// give it time to process the msg
			time.Sleep(100 * time.Millisecond)

			assert.Equal(t, tc.expectStoreHost, repo.wasStoreHostCalled())
			assert.Equal(t, tc.expectStorePlayer, repo.wasStorePlayerCalled())
			assert.Equal(t, tc.expectPublishGameCreated, publisher.wasPublishGameCreatedCalled())
			assert.Equal(t, tc.expectedPublishPlayerJoinRequested, publisher.wasPublishPlayerJoinRequestedCalled())

			mr.Shutdown()
		})
	}
}

func TestMatchRegistry_tryDetectAndProcessMessage(t *testing.T) {
	t.Parallel()

	logger := logutils.NewNoop()

	hostMsg := game.HostRegistratioMessage{
		HostID:         "test-host-123",
		Mode:           game.GameMode("foo-mode"),
		AvailableSlots: 2,
	}

	hostMsgBody, err := json.Marshal(hostMsg)
	require.NoError(t, err)

	gameMode := game.GameMode("foo-mode")

	matchMsg := game.MatchRequestMessage{
		PlayerID: "foo",
		HostID:   stringToPtr("test-host-123"),
		Mode:     &gameMode,
	}

	matchMsgBody, err := json.Marshal(matchMsg)
	require.NoError(t, err)

	testCases := []struct {
		name                               string
		givenMsgBody                       []byte
		expectError                        bool
		expectStoreHost                    bool
		expectStorePlayer                  bool
		expectPublishGameCreated           bool
		expectedPublishPlayerJoinRequested bool
	}{
		{
			name:                     "host registration",
			givenMsgBody:             hostMsgBody,
			expectStoreHost:          true,
			expectPublishGameCreated: true,
		},
		{
			name:                               "match request",
			givenMsgBody:                       matchMsgBody,
			expectStorePlayer:                  true,
			expectedPublishPlayerJoinRequested: true,
		},
		{
			name:         "invalid message format",
			givenMsgBody: []byte(`{"unknown_field": "value"}`),
			expectError:  true,
		},
		{
			name:         "completely invalid JSON",
			givenMsgBody: []byte(`this is not json`),
			expectError:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repo := repoMock{
				storeHostFunc:   func(ctx context.Context, host game.HostRegistratioMessage) error { return nil },
				storePlayerFunc: func(ctx context.Context, player game.MatchRequestMessage) error { return nil },
			}

			publisher := publisherMock{
				publishGameCreatedFunc:         func(ctx context.Context, event pubevents.GameCreatedEvent) error { return nil },
				publishPlayerJoinRequestedFunc: func(ctx context.Context, event pubevents.PlayerJoinRequestedEvent) error { return nil },
			}

			mr := MatchRegistry{
				logger:    logger,
				repo:      &repo,
				publisher: &publisher,
			}

			err := mr.tryDetectAndProcessMessage(tc.givenMsgBody)
			if tc.expectError {
				require.Error(t, err)
			} else {
				assert.Equal(t, tc.expectStoreHost, repo.wasStoreHostCalled())
				assert.Equal(t, tc.expectStorePlayer, repo.wasStorePlayerCalled())
				assert.Equal(t, tc.expectPublishGameCreated, publisher.wasPublishGameCreatedCalled())
				assert.Equal(t, tc.expectedPublishPlayerJoinRequested, publisher.wasPublishPlayerJoinRequestedCalled())
			}
		})
	}

}

func stringToPtr(s string) *string { return &s }
