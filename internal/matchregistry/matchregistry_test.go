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

func TestMatchRegistry_registerHost(t *testing.T) {
	t.Parallel()

	logger := logutils.NewNoop()

	givenHost := game.HostRegistratioMessage{
		HostID:         "test-host-123",
		Mode:           game.GameMode("foo-mode"),
		AvailableSlots: 2,
	}

	testCases := []struct {
		name                           string
		givenRepoMock                  func(ctx context.Context, host game.HostRegistratioMessage) error
		givenPublisherMock             func(ctx context.Context, event pubevents.GameCreatedEvent) error
		expectErr                      error
		expectStoreHostCalled          bool
		expectPublishGameCreatedCalled bool
	}{
		{
			name: "register host successfully",
			givenRepoMock: func(ctx context.Context, host game.HostRegistratioMessage) error {
				_, ok := ctx.Deadline()
				assert.True(t, ok)

				assert.Equal(t, givenHost, host)
				return nil
			},
			givenPublisherMock: func(ctx context.Context, event pubevents.GameCreatedEvent) error {
				_, ok := ctx.Deadline()
				assert.True(t, ok)

				assert.Equal(t, givenHost.HostID, event.GameID)
				assert.Equal(t, givenHost.HostID, event.HostID)
				assert.Equal(t, uint16(givenHost.AvailableSlots), event.MaxPlayers)
				assert.Equal(t, string(givenHost.Mode), event.GameMode)
				assert.NotZero(t, event.CreatedAt)

				return nil
			},
			expectErr:                      nil,
			expectStoreHostCalled:          true,
			expectPublishGameCreatedCalled: true,
		},
		{
			name: "fail to store host returns error",
			givenRepoMock: func(ctx context.Context, host game.HostRegistratioMessage) error {
				return assert.AnError
			},
			givenPublisherMock: func(ctx context.Context, event pubevents.GameCreatedEvent) error {
				return nil
			},
			expectErr:                      assert.AnError,
			expectStoreHostCalled:          true,
			expectPublishGameCreatedCalled: false,
		},
		{
			name: "fail to publish game created does not return error",
			givenRepoMock: func(ctx context.Context, host game.HostRegistratioMessage) error {
				return nil
			},
			givenPublisherMock: func(ctx context.Context, event pubevents.GameCreatedEvent) error {
				return assert.AnError
			},
			expectErr:                      nil,
			expectStoreHostCalled:          true,
			expectPublishGameCreatedCalled: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repo := repoMock{}
			repo.storeHostFunc = tc.givenRepoMock

			publisher := publisherMock{}
			publisher.publishGameCreatedFunc = tc.givenPublisherMock

			mr := MatchRegistry{
				logger:    logger,
				repo:      &repo,
				publisher: &publisher,
			}

			err := mr.registerHost(givenHost)
			assert.ErrorIs(t, err, tc.expectErr)

			assert.Equal(t, tc.expectStoreHostCalled, repo.storeHostCalled)
			assert.Equal(t, tc.expectPublishGameCreatedCalled, publisher.publishGameCreatedCalled)
		})
	}
}

func TestMatchRegistry_registerPlayer(t *testing.T) {
	t.Parallel()

	logger := logutils.NewNoop()

	gameMode := game.GameMode("foo-mode")

	givenMatchMsg := game.MatchRequestMessage{
		PlayerID: "foo",
		HostID:   stringToPtr("test-host-123"),
		Mode:     &gameMode,
	}

	testCases := []struct {
		name                             string
		givenMsg                         game.MatchRequestMessage
		givenRepoMock                    func(ctx context.Context, player game.MatchRequestMessage) error
		givenPublisherMock               func(ctx context.Context, event pubevents.PlayerJoinRequestedEvent) error
		expectErr                        error
		expectStorePlayerCalled          bool
		expectPublishJoinRequestedCalled bool
	}{
		{
			name:     "register player with all data successfully",
			givenMsg: givenMatchMsg,
			givenRepoMock: func(ctx context.Context, player game.MatchRequestMessage) error {
				_, ok := ctx.Deadline()
				assert.True(t, ok)

				assert.Equal(t, givenMatchMsg, player)
				return nil
			},
			givenPublisherMock: func(ctx context.Context, event pubevents.PlayerJoinRequestedEvent) error {
				_, ok := ctx.Deadline()
				assert.True(t, ok)

				assert.Equal(t, givenMatchMsg.PlayerID, event.PlayerID)
				assert.Equal(t, givenMatchMsg.HostID, event.HostID)
				assert.Equal(t, string(*givenMatchMsg.Mode), *event.GameMode)
				assert.NotZero(t, event.CreatedAt)

				return nil
			},
			expectErr:                        nil,
			expectStorePlayerCalled:          true,
			expectPublishJoinRequestedCalled: true,
		},
		{
			name: "register player without host ID successfully",
			givenMsg: game.MatchRequestMessage{
				PlayerID: "foo",
				// HostID:   stringToPtr("test-host-123"),
				Mode: &gameMode,
			},
			givenRepoMock: func(ctx context.Context, player game.MatchRequestMessage) error {
				_, ok := ctx.Deadline()
				assert.True(t, ok)

				assert.Equal(t, givenMatchMsg, player)
				return nil
			},
			givenPublisherMock: func(ctx context.Context, event pubevents.PlayerJoinRequestedEvent) error {
				_, ok := ctx.Deadline()
				assert.True(t, ok)

				assert.Equal(t, givenMatchMsg.PlayerID, event.PlayerID)
				assert.Equal(t, givenMatchMsg.HostID, event.HostID)
				assert.Equal(t, string(*givenMatchMsg.Mode), *event.GameMode)
				assert.NotZero(t, event.CreatedAt)

				return nil
			},
			expectErr:                        nil,
			expectStorePlayerCalled:          true,
			expectPublishJoinRequestedCalled: true,
		},
		{
			name: "register player without host ID and game mode successfully",
			givenMsg: game.MatchRequestMessage{
				PlayerID: "foo",
				// HostID:   stringToPtr("test-host-123"),
				// Mode: &gameMode,
			},
			givenRepoMock: func(ctx context.Context, player game.MatchRequestMessage) error {
				_, ok := ctx.Deadline()
				assert.True(t, ok)

				assert.Equal(t, givenMatchMsg, player)
				return nil
			},
			givenPublisherMock: func(ctx context.Context, event pubevents.PlayerJoinRequestedEvent) error {
				_, ok := ctx.Deadline()
				assert.True(t, ok)

				assert.Equal(t, givenMatchMsg.PlayerID, event.PlayerID)
				assert.Equal(t, givenMatchMsg.HostID, event.HostID)
				assert.Equal(t, string(*givenMatchMsg.Mode), *event.GameMode)
				assert.NotZero(t, event.CreatedAt)

				return nil
			},
			expectErr:                        nil,
			expectStorePlayerCalled:          true,
			expectPublishJoinRequestedCalled: true,
		},
		{
			name:     "fail to store player returns error",
			givenMsg: givenMatchMsg,
			givenRepoMock: func(ctx context.Context, player game.MatchRequestMessage) error {
				return assert.AnError
			},
			givenPublisherMock: func(ctx context.Context, event pubevents.PlayerJoinRequestedEvent) error {
				return nil
			},
			expectErr:                        assert.AnError,
			expectStorePlayerCalled:          true,
			expectPublishJoinRequestedCalled: false,
		},
		{
			name:     "fail to publish player join requested does not return error",
			givenMsg: givenMatchMsg,
			givenRepoMock: func(ctx context.Context, player game.MatchRequestMessage) error {
				return nil
			},
			givenPublisherMock: func(ctx context.Context, event pubevents.PlayerJoinRequestedEvent) error {
				return assert.AnError
			},
			expectErr:                        nil,
			expectStorePlayerCalled:          true,
			expectPublishJoinRequestedCalled: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repo := repoMock{}
			repo.storePlayerFunc = tc.givenRepoMock

			publisher := publisherMock{}
			publisher.publishPlayerJoinRequestedFunc = tc.givenPublisherMock

			mr := MatchRegistry{
				logger:    logger,
				repo:      &repo,
				publisher: &publisher,
			}

			err := mr.registerPlayer(givenMatchMsg)
			assert.ErrorIs(t, err, tc.expectErr)

			assert.Equal(t, tc.expectStorePlayerCalled, repo.storePlayerCalled)
			assert.Equal(t, tc.expectPublishJoinRequestedCalled, publisher.publishPlayerJoinRequestedCalled)
		})
	}
}
func stringToPtr(s string) *string { return &s }
