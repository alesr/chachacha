package matchregistry

import (
	"context"
	"sync"

	pubevents "github.com/alesr/chachacha/pkg/events"
	"github.com/alesr/chachacha/pkg/game"
	"github.com/rabbitmq/amqp091-go"
)

var _ repository = &repoMock{}

type repoMock struct {
	mu                sync.Mutex
	storeHostCalled   bool
	storePlayerCalled bool
	storeHostFunc     func(ctx context.Context, host game.HostRegistratioMessage) error
	storePlayerFunc   func(ctx context.Context, player game.MatchRequestMessage) error
}

func (m *repoMock) StoreHost(ctx context.Context, host game.HostRegistratioMessage) error {
	if m.storeHostFunc != nil {
		m.mu.Lock()
		m.storeHostCalled = true
		m.mu.Unlock()
		return m.storeHostFunc(ctx, host)
	}
	return nil
}

func (m *repoMock) StorePlayer(ctx context.Context, player game.MatchRequestMessage) error {
	if m.storePlayerFunc != nil {
		m.mu.Lock()
		m.storePlayerCalled = true
		m.mu.Unlock()
		return m.storePlayerFunc(ctx, player)
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

var _ consumer = &consumerMock{}

type consumerMock struct {
	consumeFunc func(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp091.Table) (<-chan amqp091.Delivery, error)
}

func (m *consumerMock) Consume(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp091.Table) (<-chan amqp091.Delivery, error) {
	return m.consumeFunc(queue, consumer, autoAck, exclusive, noLocal, noWait, args)
}

var _ publisher = &publisherMock{}

type publisherMock struct {
	mu                               sync.Mutex
	publishGameCreatedCalled         bool
	publishPlayerJoinRequestedCalled bool
	publishGameCreatedFunc           func(ctx context.Context, event pubevents.GameCreatedEvent) error
	publishPlayerJoinRequestedFunc   func(ctx context.Context, event pubevents.PlayerJoinRequestedEvent) error
}

func (m *publisherMock) PublishGameCreated(ctx context.Context, event pubevents.GameCreatedEvent) error {
	if m.publishGameCreatedFunc != nil {
		m.mu.Lock()
		m.publishGameCreatedCalled = true
		m.mu.Unlock()
		return m.publishGameCreatedFunc(ctx, event)
	}
	return nil
}

func (m *publisherMock) PublishPlayerJoinRequested(ctx context.Context, event pubevents.PlayerJoinRequestedEvent) error {
	if m.publishPlayerJoinRequestedFunc != nil {
		m.mu.Lock()
		m.publishPlayerJoinRequestedCalled = true
		m.mu.Unlock()
		return m.publishPlayerJoinRequestedFunc(ctx, event)
	}
	return nil
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
