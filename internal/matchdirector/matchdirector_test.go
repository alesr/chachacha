package matchdirector

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/alesr/chachacha/internal/sessionrepo"
	pubevts "github.com/alesr/chachacha/pkg/events"
	"github.com/alesr/chachacha/pkg/logutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		logger        *slog.Logger
		repo          repository
		publisher     publisher
		matchInterval time.Duration
		expectErr     bool
	}{
		{
			name:          "valid initialization",
			logger:        logutils.NewNoop(),
			repo:          &repoMock{},
			publisher:     &publisherMock{},
			matchInterval: time.Second,
			expectErr:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			md, err := New(tc.logger, tc.repo, tc.publisher, tc.matchInterval)
			if tc.expectErr {
				assert.Error(t, err)
				assert.Nil(t, md)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, md)
				assert.Equal(t, tc.repo, md.repo)
				assert.Equal(t, tc.publisher, md.publisher)
				assert.NotNil(t, md.matchTicker)
				assert.NotNil(t, md.stopChan)
			}
		})
	}
}

func TestMatchDirector_StartStop(t *testing.T) {
	t.Parallel()

	repo := &repoMock{
		getHostsFunc: func(ctx context.Context) ([]pubevts.HostRegistrationEvent, error) {
			return []pubevts.HostRegistrationEvent{}, nil
		},
		getPlayersFunc: func(ctx context.Context) ([]pubevts.PlayerMatchRequestEvent, error) {
			return []pubevts.PlayerMatchRequestEvent{}, nil
		},
	}

	publisher := &publisherMock{}
	md, err := New(logutils.NewNoop(), repo, publisher, 50*time.Millisecond)
	require.NoError(t, err)

	md.Start()

	// wait for at least one cycle
	time.Sleep(100 * time.Millisecond)
	assert.True(t, repo.wasGetHostsCalled())
	assert.True(t, repo.wasGetPlayersCalled())

	repo.resetCalls()
	md.Stop()

	// wait for the goroutine to actually stop
	time.Sleep(100 * time.Millisecond)

	// make sure no more calls after stopping
	repo.resetCalls()
	time.Sleep(100 * time.Millisecond)
	assert.False(t, repo.wasGetHostsCalled())
	assert.False(t, repo.wasGetPlayersCalled())
}

func TestMatchDirector_HandleHostDisconnection(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		hostID        string
		setupMocks    func(*repoMock, *publisherMock)
		expectErr     bool
		checkBehavior func(*testing.T, *repoMock, *publisherMock)
	}{
		{
			name:   "host not in session",
			hostID: "host1",
			setupMocks: func(r *repoMock, p *publisherMock) {
				r.getHostInActiveSessionFunc = func(ctx context.Context, hostID string) (*sessionrepo.Session, error) {
					return nil, nil
				}
			},
			expectErr: false,
			checkBehavior: func(t *testing.T, r *repoMock, p *publisherMock) {
				assert.True(t, r.wasGetHostInActiveSessionCalled())
				assert.True(t, r.wasRemoveHostCalled())
			},
		},
		{
			name:   "host in active session",
			hostID: "host1",
			setupMocks: func(r *repoMock, p *publisherMock) {
				r.getHostInActiveSessionFunc = func(ctx context.Context, hostID string) (*sessionrepo.Session, error) {
					return &sessionrepo.Session{
						ID:      "session1",
						HostID:  hostID,
						Mode:    "deathmatch",
						Players: []string{"player1", "player2"},
					}, nil
				}
			},
			expectErr: false,
			checkBehavior: func(t *testing.T, r *repoMock, p *publisherMock) {
				assert.True(t, r.wasGetHostInActiveSessionCalled())
				assert.True(t, r.wasStorePlayerCalled())
				assert.True(t, r.wasStoreGameSessionCalled())
				assert.True(t, r.wasRemoveHostCalled())
				assert.True(t, p.wasPublishPlayerJoinRequestedCalled())
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repo := &repoMock{}
			publisher := &publisherMock{}
			tc.setupMocks(repo, publisher)

			md, err := New(logutils.NewNoop(), repo, publisher, time.Second)
			require.NoError(t, err)

			err = md.HandleHostDisconnection(context.Background(), tc.hostID)
			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			tc.checkBehavior(t, repo, publisher)
		})
	}
}

func TestMatchDirector_HandlePlayerDisconnection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		playerID      string
		setupMocks    func(*repoMock, *publisherMock)
		expectErr     bool
		checkBehavior func(*testing.T, *repoMock)
	}{
		{
			name:     "player not in session",
			playerID: "player1",
			setupMocks: func(r *repoMock, p *publisherMock) {
				r.getPlayerInActiveSessionFunc = func(ctx context.Context, playerID string) (*sessionrepo.Session, error) {
					return nil, nil
				}
			},
			expectErr: false,
			checkBehavior: func(t *testing.T, r *repoMock) {
				assert.True(t, r.wasGetPlayerInActiveSessionCalled())
				assert.True(t, r.wasRemovePlayerCalled())
			},
		},
		{
			name:     "player in session",
			playerID: "player1",
			setupMocks: func(r *repoMock, p *publisherMock) {
				r.getPlayerInActiveSessionFunc = func(ctx context.Context, playerID string) (*sessionrepo.Session, error) {
					return &sessionrepo.Session{
						ID:             "session1",
						HostID:         "host1",
						Players:        []string{"player1", "player2"},
						AvailableSlots: 0,
						State:          sessionrepo.SessionStateComplete,
					}, nil
				}
			},
			expectErr: false,
			checkBehavior: func(t *testing.T, r *repoMock) {
				assert.True(t, r.wasGetPlayerInActiveSessionCalled())
				assert.True(t, r.wasStoreGameSessionCalled())
				assert.True(t, r.wasUpdateHostAvailableSlotsCalled())
				assert.True(t, r.wasRemovePlayerCalled())
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := &repoMock{}
			publisher := &publisherMock{}
			tt.setupMocks(repo, publisher)

			md, err := New(logutils.NewNoop(), repo, publisher, time.Second)
			require.NoError(t, err)

			err = md.HandlePlayerDisconnection(context.Background(), tt.playerID)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			tt.checkBehavior(t, repo)
		})
	}
}
