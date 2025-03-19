package matchregistry

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alesr/chachacha/internal/sessionrepo"
	pubevts "github.com/alesr/chachacha/pkg/events"
	"github.com/alesr/chachacha/pkg/logutils"
	"github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Parallel()

	givenLogger := logutils.NewNoop()
	givenRepo := &repoMock{}
	givenConsumer := &consumerMock{}
	givenPublisher := &publisherMock{}
	givenQueueName := "test_queue"

	mr := New(givenLogger, givenRepo, givenQueueName, givenConsumer, givenPublisher)

	require.NotNil(t, mr)
	assert.Equal(t, givenRepo, mr.repo)
	assert.Equal(t, givenConsumer, mr.consumer)
	assert.Equal(t, givenPublisher, mr.publisher)
	assert.Equal(t, givenQueueName, mr.matchmakingQueueName)
}

func TestMatchRegistry_Start(t *testing.T) {
	t.Parallel()

	gameMode := pubevts.GameMode("deathmatch")
	hostID := "host1"

	testCases := []struct {
		name          string
		messageType   string
		message       any
		setupMocks    func(*repoMock, *publisherMock)
		checkBehavior func(*testing.T, *repoMock, *publisherMock)
	}{
		{
			name:        "host registration",
			messageType: pubevts.MsgTypeHostRegistration,
			message: pubevts.HostRegistrationEvent{
				HostID:         hostID,
				AvailableSlots: 4,
				Mode:           gameMode,
			},
			setupMocks: func(r *repoMock, p *publisherMock) {
				r.storeHostFunc = func(ctx context.Context, host pubevts.HostRegistrationEvent) error {
					return nil
				}
			},
			checkBehavior: func(t *testing.T, r *repoMock, p *publisherMock) {
				assert.True(t, r.wasStoreHostCalled())
				assert.True(t, p.wasPublishGameCreatedCalled())
			},
		},
		{
			name:        "player match request",
			messageType: pubevts.MsgTypePlayerMatchRequest,
			message: pubevts.PlayerMatchRequestEvent{
				PlayerID: "player1",
				Mode:     &gameMode,
				HostID:   &hostID,
			},
			setupMocks: func(r *repoMock, p *publisherMock) {
				r.storePlayerFunc = func(ctx context.Context, player pubevts.PlayerMatchRequestEvent) error {
					return nil
				}
				r.getHostsFunc = func(ctx context.Context) ([]pubevts.HostRegistrationEvent, error) {
					return []pubevts.HostRegistrationEvent{
						{
							HostID: hostID,
							Mode:   gameMode,
						},
					}, nil
				}
			},
			checkBehavior: func(t *testing.T, r *repoMock, p *publisherMock) {
				assert.True(t, r.wasStorePlayerCalled())
				assert.True(t, p.wasPublishPlayerJoinRequestedCalled())
			},
		},
		{
			name:        "host removal",
			messageType: pubevts.MsgTypeHostRegistrationRemoval,
			message: pubevts.HostRegistrationRemovalEvent{
				HostID: hostID,
			},
			setupMocks: func(r *repoMock, p *publisherMock) {
				r.getHostInActiveSessionFunc = func(ctx context.Context, hostID string) (*sessionrepo.Session, error) {
					return &sessionrepo.Session{
						ID:      "session1",
						HostID:  hostID,
						Mode:    gameMode,
						Players: []string{"player1"},
						State:   sessionrepo.SessionStateAwaiting,
					}, nil
				}
				r.storePlayerFunc = func(ctx context.Context, player pubevts.PlayerMatchRequestEvent) error {
					return nil
				}
				r.storeGameSessionFunc = func(ctx context.Context, session *sessionrepo.Session) error {
					return nil
				}
				r.removeHostFunc = func(ctx context.Context, hostID string) error {
					return nil
				}
			},
			checkBehavior: func(t *testing.T, r *repoMock, p *publisherMock) {
				assert.True(t, r.wasGetHostInActiveSessionCalled())
				assert.True(t, r.wasStorePlayerCalled())
				assert.True(t, r.wasStoreGameSessionCalled())
				assert.True(t, r.wasRemoveHostCalled())
			},
		},
		{
			name:        "unknown message type with auto-detection",
			messageType: "",
			message: pubevts.PlayerMatchRequestEvent{
				PlayerID: "player1",
				Mode:     &gameMode,
			},
			setupMocks: func(r *repoMock, p *publisherMock) {
				r.storePlayerFunc = func(ctx context.Context, player pubevts.PlayerMatchRequestEvent) error {
					return nil
				}
			},
			checkBehavior: func(t *testing.T, r *repoMock, p *publisherMock) {
				assert.True(t, r.wasStorePlayerCalled())
				assert.True(t, p.wasPublishPlayerJoinRequestedCalled())
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			delivery := make(chan amqp091.Delivery, 1) // buffer of 1 to prevent blocking
			consumer := &consumerMock{
				consumeFunc: func(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp091.Table) (<-chan amqp091.Delivery, error) {
					return delivery, nil
				},
			}

			repo := &repoMock{}
			publisher := &publisherMock{}
			tc.setupMocks(repo, publisher)

			mr := New(logutils.NewNoop(), repo, "test_queue", consumer, publisher)

			go func() {
				err := mr.Start()
				require.NoError(t, err)
			}()

			time.Sleep(200 * time.Millisecond)

			msgBody, err := json.Marshal(tc.message)
			require.NoError(t, err)

			delivery <- amqp091.Delivery{
				Type: tc.messageType,
				Body: msgBody,
			}

			// allow message to be processed
			time.Sleep(300 * time.Millisecond)

			tc.checkBehavior(t, repo, publisher)

			mr.Shutdown()
		})
	}
}

func TestMatchRegistry_MessageTypeDetection(t *testing.T) {
	t.Parallel()

	gameMode := pubevts.GameMode("deathmatch")

	testCases := []struct {
		name          string
		message       any
		setupMocks    func(*repoMock, *publisherMock)
		checkBehavior func(*testing.T, *repoMock, *publisherMock)
	}{
		{
			name: "detect player message",
			message: pubevts.PlayerMatchRequestEvent{
				PlayerID: "player1",
				Mode:     &gameMode,
			},
			setupMocks: func(r *repoMock, p *publisherMock) {
				r.storePlayerFunc = func(ctx context.Context, player pubevts.PlayerMatchRequestEvent) error {
					return nil
				}
			},
			checkBehavior: func(t *testing.T, r *repoMock, p *publisherMock) {
				assert.True(t, r.wasStorePlayerCalled())
			},
		},
		{
			name: "detect host message",
			message: pubevts.HostRegistrationEvent{
				HostID: "host1",
				Mode:   gameMode,
			},
			setupMocks: func(r *repoMock, p *publisherMock) {
				r.storeHostFunc = func(ctx context.Context, host pubevts.HostRegistrationEvent) error {
					return nil
				}
			},
			checkBehavior: func(t *testing.T, r *repoMock, p *publisherMock) {
				assert.True(t, r.wasStoreHostCalled())
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repo := &repoMock{}
			publisher := &publisherMock{}
			tc.setupMocks(repo, publisher)

			mr := New(logutils.NewNoop(), repo, "test_queue", &consumerMock{}, publisher)

			msgBody, err := json.Marshal(tc.message)
			require.NoError(t, err)

			err = mr.tryDetectAndProcessMessage(msgBody)
			require.NoError(t, err)

			tc.checkBehavior(t, repo, publisher)
		})
	}
}

func TestMatchRegistry_PlayerRequestNonExistingHost(t *testing.T) {
	t.Parallel()

	gameMode := pubevts.GameMode("deathmatch")
	nonExistentHostID := "non_existent_host"

	delivery := make(chan amqp091.Delivery, 1)
	consumer := &consumerMock{
		consumeFunc: func(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp091.Table) (<-chan amqp091.Delivery, error) {
			return delivery, nil
		},
	}

	repo := &repoMock{}

	repo.getHostsFunc = func(ctx context.Context) ([]pubevts.HostRegistrationEvent, error) {
		return []pubevts.HostRegistrationEvent{}, nil
	}

	publisher := &publisherMock{}

	mr := New(logutils.NewNoop(), repo, "test_queue", consumer, publisher)

	go func() {
		err := mr.Start()
		require.NoError(t, err)
	}()

	time.Sleep(200 * time.Millisecond)

	// player match request with non-existent host
	playerRequest := pubevts.PlayerMatchRequestEvent{
		PlayerID: "player1",
		Mode:     &gameMode,
		HostID:   &nonExistentHostID,
	}

	msgBody, err := json.Marshal(playerRequest)
	require.NoError(t, err)

	delivery <- amqp091.Delivery{
		Type: pubevts.MsgTypePlayerMatchRequest,
		Body: msgBody,
	}

	time.Sleep(300 * time.Millisecond)

	assert.True(t, repo.wasGetHostsCalled(), "GetHosts should be called")
	assert.True(t, publisher.wasPublishPlayerMatchErrorCalled(), "PublishPlayerMatchError should be called")
	assert.False(t, repo.wasStorePlayerCalled(), "StorePlayer should not be called for non-existent host")

	mr.Shutdown()
}
