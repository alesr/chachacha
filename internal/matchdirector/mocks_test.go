package matchdirector

import (
	"context"
	"sync"

	"github.com/alesr/chachacha/internal/sessionrepo"
	pubevts "github.com/alesr/chachacha/pkg/events"
)

var _ repository = &repoMock{}

type repoMock struct {
	mu                             sync.Mutex
	getHostsCalled                 bool
	getPlayersCalled               bool
	storePlayerCalled              bool
	storeGameSessionCalled         bool
	updateHostAvailableSlotsCalled bool
	removePlayerCalled             bool
	removeHostCalled               bool
	getGameSessionCalled           bool
	getActiveGameSessionsCalled    bool
	getHostInActiveSessionCalled   bool
	getPlayerInActiveSessionCalled bool

	getHostsFunc                 func(ctx context.Context) ([]pubevts.HostRegistrationEvent, error)
	getPlayersFunc               func(ctx context.Context) ([]pubevts.PlayerMatchRequestEvent, error)
	storePlayerFunc              func(ctx context.Context, player pubevts.PlayerMatchRequestEvent) error
	storeGameSessionFunc         func(ctx context.Context, session *sessionrepo.Session) error
	updateHostAvailableSlotsFunc func(ctx context.Context, hostID string, slots uint16) error
	removePlayerFunc             func(ctx context.Context, playerID string) error
	removeHostFunc               func(ctx context.Context, hostID string) error
	getGameSessionFunc           func(ctx context.Context, sessionID string) (*sessionrepo.Session, error)
	getActiveGameSessionsFunc    func(ctx context.Context) ([]*sessionrepo.Session, error)
	getHostInActiveSessionFunc   func(ctx context.Context, hostID string) (*sessionrepo.Session, error)
	getPlayerInActiveSessionFunc func(ctx context.Context, playerID string) (*sessionrepo.Session, error)
}

func (m *repoMock) GetHosts(ctx context.Context) ([]pubevts.HostRegistrationEvent, error) {
	m.mu.Lock()
	m.getHostsCalled = true
	m.mu.Unlock()

	if m.getHostsFunc != nil {
		return m.getHostsFunc(ctx)
	}
	return []pubevts.HostRegistrationEvent{}, nil
}

func (m *repoMock) GetPlayers(ctx context.Context) ([]pubevts.PlayerMatchRequestEvent, error) {
	m.mu.Lock()
	m.getPlayersCalled = true
	m.mu.Unlock()

	if m.getPlayersFunc != nil {
		return m.getPlayersFunc(ctx)
	}
	return []pubevts.PlayerMatchRequestEvent{}, nil
}

func (m *repoMock) StorePlayer(ctx context.Context, player pubevts.PlayerMatchRequestEvent) error {
	m.mu.Lock()
	m.storePlayerCalled = true
	m.mu.Unlock()

	if m.storePlayerFunc != nil {
		return m.storePlayerFunc(ctx, player)
	}
	return nil
}

func (m *repoMock) StoreGameSession(ctx context.Context, session *sessionrepo.Session) error {
	m.mu.Lock()
	m.storeGameSessionCalled = true
	m.mu.Unlock()

	if m.storeGameSessionFunc != nil {
		return m.storeGameSessionFunc(ctx, session)
	}
	return nil
}

func (m *repoMock) UpdateHostAvailableSlots(ctx context.Context, hostID string, slots uint16) error {
	m.mu.Lock()
	m.updateHostAvailableSlotsCalled = true
	m.mu.Unlock()

	if m.updateHostAvailableSlotsFunc != nil {
		return m.updateHostAvailableSlotsFunc(ctx, hostID, slots)
	}
	return nil
}

func (m *repoMock) RemovePlayer(ctx context.Context, playerID string) error {
	m.mu.Lock()
	m.removePlayerCalled = true
	m.mu.Unlock()

	if m.removePlayerFunc != nil {
		return m.removePlayerFunc(ctx, playerID)
	}
	return nil
}

func (m *repoMock) RemoveHost(ctx context.Context, hostID string) error {
	m.mu.Lock()
	m.removeHostCalled = true
	m.mu.Unlock()

	if m.removeHostFunc != nil {
		return m.removeHostFunc(ctx, hostID)
	}
	return nil
}

func (m *repoMock) GetGameSession(ctx context.Context, sessionID string) (*sessionrepo.Session, error) {
	m.mu.Lock()
	m.getGameSessionCalled = true
	m.mu.Unlock()

	if m.getGameSessionFunc != nil {
		return m.getGameSessionFunc(ctx, sessionID)
	}
	return nil, nil
}

func (m *repoMock) GetActiveGameSessions(ctx context.Context) ([]*sessionrepo.Session, error) {
	m.mu.Lock()
	m.getActiveGameSessionsCalled = true
	m.mu.Unlock()

	if m.getActiveGameSessionsFunc != nil {
		return m.getActiveGameSessionsFunc(ctx)
	}
	return nil, nil
}

func (m *repoMock) GetHostInActiveSession(ctx context.Context, hostID string) (*sessionrepo.Session, error) {
	m.mu.Lock()
	m.getHostInActiveSessionCalled = true
	m.mu.Unlock()

	if m.getHostInActiveSessionFunc != nil {
		return m.getHostInActiveSessionFunc(ctx, hostID)
	}
	return nil, nil
}

func (m *repoMock) GetPlayerInActiveSession(ctx context.Context, playerID string) (*sessionrepo.Session, error) {
	m.mu.Lock()
	m.getPlayerInActiveSessionCalled = true
	m.mu.Unlock()

	if m.getPlayerInActiveSessionFunc != nil {
		return m.getPlayerInActiveSessionFunc(ctx, playerID)
	}
	return nil, nil
}

func (m *repoMock) wasGetHostsCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.getHostsCalled
}

func (m *repoMock) wasGetPlayersCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.getPlayersCalled
}

func (m *repoMock) wasStorePlayerCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.storePlayerCalled
}

func (m *repoMock) wasStoreGameSessionCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.storeGameSessionCalled
}

func (m *repoMock) wasUpdateHostAvailableSlotsCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.updateHostAvailableSlotsCalled
}

func (m *repoMock) wasRemovePlayerCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.removePlayerCalled
}

func (m *repoMock) wasRemoveHostCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.removeHostCalled
}

func (m *repoMock) wasGetGameSessionCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.getGameSessionCalled
}

func (m *repoMock) wasGetActiveGameSessionsCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.getActiveGameSessionsCalled
}

func (m *repoMock) wasGetHostInActiveSessionCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.getHostInActiveSessionCalled
}

func (m *repoMock) wasGetPlayerInActiveSessionCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.getPlayerInActiveSessionCalled
}

func (m *repoMock) resetCalls() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getHostsCalled = false
	m.getPlayersCalled = false
	m.storePlayerCalled = false
	m.storeGameSessionCalled = false
	m.updateHostAvailableSlotsCalled = false
	m.removePlayerCalled = false
	m.removeHostCalled = false
	m.getGameSessionCalled = false
	m.getActiveGameSessionsCalled = false
	m.getHostInActiveSessionCalled = false
	m.getPlayerInActiveSessionCalled = false
}

var _ publisher = &publisherMock{}

type publisherMock struct {
	mu                               sync.Mutex
	publishPlayerJoinRequestedCalled bool
	publishPlayerJoinRequestedFunc   func(ctx context.Context, event pubevts.PlayerJoinRequestedEvent) error
}

func (m *publisherMock) PublishPlayerJoinRequested(ctx context.Context, event pubevts.PlayerJoinRequestedEvent) error {
	m.mu.Lock()
	m.publishPlayerJoinRequestedCalled = true
	m.mu.Unlock()

	if m.publishPlayerJoinRequestedFunc != nil {
		return m.publishPlayerJoinRequestedFunc(ctx, event)
	}
	return nil
}

func (m *publisherMock) wasPublishPlayerJoinRequestedCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.publishPlayerJoinRequestedCalled
}

func (m *publisherMock) reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.publishPlayerJoinRequestedCalled = false
}
