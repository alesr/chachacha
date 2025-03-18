package matchdirector

import (
	"context"
	"sync"

	"github.com/alesr/chachacha/internal/sessionrepo"
	"github.com/alesr/chachacha/pkg/game"
)

var _ repository = &repoMock{}

type repoMock struct {
	mu                             sync.Mutex
	getHostsCalled                 bool
	getPlayersCalled               bool
	storeGameSessionCalled         bool
	updateHostAvailableSlotsCalled bool
	removePlayerCalled             bool

	getHostsFunc                 func(ctx context.Context) ([]game.HostRegistratioMessage, error)
	getPlayersFunc               func(ctx context.Context) ([]game.MatchRequestMessage, error)
	storeGameSessionFunc         func(ctx context.Context, session *sessionrepo.Session) error
	updateHostAvailableSlotsFunc func(ctx context.Context, hostIP string, slots int8) error
	removePlayerFunc             func(ctx context.Context, playerID string) error
}

func (m *repoMock) GetHosts(ctx context.Context) ([]game.HostRegistratioMessage, error) {
	m.mu.Lock()
	m.getHostsCalled = true
	m.mu.Unlock()

	if m.getHostsFunc != nil {
		return m.getHostsFunc(ctx)
	}
	return []game.HostRegistratioMessage{}, nil
}

func (m *repoMock) GetPlayers(ctx context.Context) ([]game.MatchRequestMessage, error) {
	m.mu.Lock()
	m.getPlayersCalled = true
	m.mu.Unlock()

	if m.getPlayersFunc != nil {
		return m.getPlayersFunc(ctx)
	}
	return []game.MatchRequestMessage{}, nil
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

func (m *repoMock) UpdateHostAvailableSlots(ctx context.Context, hostIP string, slots int8) error {
	m.mu.Lock()
	m.updateHostAvailableSlotsCalled = true
	m.mu.Unlock()

	if m.updateHostAvailableSlotsFunc != nil {
		return m.updateHostAvailableSlotsFunc(ctx, hostIP, slots)
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

func (m *repoMock) resetCalls() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getHostsCalled = false
	m.getPlayersCalled = false
	m.storeGameSessionCalled = false
	m.updateHostAvailableSlotsCalled = false
	m.removePlayerCalled = false
}
