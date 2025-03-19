package matchregistry

import (
	"context"
	"sync"

	"github.com/alesr/chachacha/internal/sessionrepo"
	pubevts "github.com/alesr/chachacha/pkg/events"
	"github.com/rabbitmq/amqp091-go"
)

var _ repository = &repoMock{}

type repoMock struct {
	mu                             sync.Mutex
	storeHostCalled                bool
	storePlayerCalled              bool
	storeGameSessionCalled         bool
	getHostInActiveSessionCalled   bool
	removeHostCalled               bool
	getHostsCalled                 bool
	getPlayerInActiveSessionCalled bool
	removePlayerCalled             bool
	updateHostAvailableSlotsCalled bool

	storeHostFunc                func(ctx context.Context, host pubevts.HostRegistrationEvent) error
	storePlayerFunc              func(ctx context.Context, player pubevts.PlayerMatchRequestEvent) error
	storeGameSessionFunc         func(ctx context.Context, session *sessionrepo.Session) error
	getHostInActiveSessionFunc   func(ctx context.Context, hostID string) (*sessionrepo.Session, error)
	removeHostFunc               func(ctx context.Context, hostID string) error
	getHostsFunc                 func(ctx context.Context) ([]pubevts.HostRegistrationEvent, error)
	getPlayerInActiveSessionFunc func(ctx context.Context, playerID string) (*sessionrepo.Session, error)
	removePlayerFunc             func(ctx context.Context, playerID string) error
	updateHostAvailableSlotsFunc func(ctx context.Context, hostID string, slots uint16) error
}

func (m *repoMock) StoreHost(ctx context.Context, host pubevts.HostRegistrationEvent) error {
	m.mu.Lock()
	m.storeHostCalled = true
	m.mu.Unlock()

	if m.storeHostFunc != nil {
		return m.storeHostFunc(ctx, host)
	}
	return nil
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

func (m *repoMock) GetHostInActiveSession(ctx context.Context, hostID string) (*sessionrepo.Session, error) {
	m.mu.Lock()
	m.getHostInActiveSessionCalled = true
	m.mu.Unlock()

	if m.getHostInActiveSessionFunc != nil {
		return m.getHostInActiveSessionFunc(ctx, hostID)
	}
	return nil, nil
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

func (m *repoMock) GetHosts(ctx context.Context) ([]pubevts.HostRegistrationEvent, error) {
	m.mu.Lock()
	m.getHostsCalled = true
	m.mu.Unlock()

	if m.getHostsFunc != nil {
		return m.getHostsFunc(ctx)
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

func (m *repoMock) RemovePlayer(ctx context.Context, playerID string) error {
	m.mu.Lock()
	m.removePlayerCalled = true
	m.mu.Unlock()

	if m.removePlayerFunc != nil {
		return m.removePlayerFunc(ctx, playerID)
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

func (m *repoMock) wasStoreHostCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.storeHostCalled
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

func (m *repoMock) wasGetHostInActiveSessionCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.getHostInActiveSessionCalled
}

func (m *repoMock) wasRemoveHostCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.removeHostCalled
}

func (m *repoMock) wasGetHostsCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.getHostsCalled
}

func (m *repoMock) reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.storeHostCalled = false
	m.storePlayerCalled = false
	m.storeGameSessionCalled = false
	m.getHostInActiveSessionCalled = false
	m.removeHostCalled = false
	m.getHostsCalled = false
}

var _ consumer = &consumerMock{}

type consumerMock struct {
	mu            sync.Mutex
	consumeCalled bool
	consumeFunc   func(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp091.Table) (<-chan amqp091.Delivery, error)
}

func (m *consumerMock) Consume(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp091.Table) (<-chan amqp091.Delivery, error) {
	m.mu.Lock()
	m.consumeCalled = true
	m.mu.Unlock()

	if m.consumeFunc != nil {
		return m.consumeFunc(queue, consumer, autoAck, exclusive, noLocal, noWait, args)
	}
	return make(chan amqp091.Delivery), nil
}

func (m *consumerMock) wasConsumeCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.consumeCalled
}

type publisherMock struct {
	mu                               sync.Mutex
	publishGameCreatedCalled         bool
	publishPlayerJoinRequestedCalled bool
	publishPlayerMatchErrorCalled    bool

	publishGameCreatedFunc         func(ctx context.Context, event pubevts.GameCreatedEvent) error
	publishPlayerJoinRequestedFunc func(ctx context.Context, event pubevts.PlayerJoinRequestedEvent) error
	publishPlayerMatchErrorFunc    func(ctx context.Context, event pubevts.PlayerMatchErrorEvent) error
}

func (m *publisherMock) PublishGameCreated(ctx context.Context, event pubevts.GameCreatedEvent) error {
	m.mu.Lock()
	m.publishGameCreatedCalled = true
	m.mu.Unlock()

	if m.publishGameCreatedFunc != nil {
		return m.publishGameCreatedFunc(ctx, event)
	}
	return nil
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

func (m *publisherMock) PublishPlayerMatchError(ctx context.Context, event pubevts.PlayerMatchErrorEvent) error {
	m.mu.Lock()
	m.publishPlayerMatchErrorCalled = true
	m.mu.Unlock()

	if m.publishPlayerMatchErrorFunc != nil {
		return m.publishPlayerMatchErrorFunc(ctx, event)
	}
	return nil
}

func (m *publisherMock) wasPublishPlayerMatchErrorCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.publishPlayerMatchErrorCalled
}

func (m *publisherMock) wasPublishGameCreatedCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.publishGameCreatedCalled
}

func (m *publisherMock) wasPublishPlayerJoinRequestedCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.publishPlayerJoinRequestedCalled
}

func (m *consumerMock) reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.consumeCalled = false
}

func (m *publisherMock) reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.publishGameCreatedCalled = false
	m.publishPlayerJoinRequestedCalled = false
	m.publishPlayerMatchErrorCalled = false
}
