package events

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	pubevts "github.com/alesr/chachacha/pkg/events"
	"github.com/rabbitmq/amqp091-go"
)

const defaultTimeout = 5 * time.Second

var exchanges = []string{
	pubevts.ExchangeMatchRequest,
	pubevts.ExchangeGameCreated,
	pubevts.ExchangePlayerJoinRequested,
	pubevts.ExchangePlayerJoined,
	pubevts.ExchangePlayerMatchError,
}

// Publisher handles publishing events to RabbitMQ.
type Publisher struct {
	ch *amqp091.Channel
}

// NewPublisher creates a new event publisher.
func NewPublisher(ch *amqp091.Channel) (*Publisher, error) {
	for _, exchange := range exchanges {
		if err := ch.ExchangeDeclare(
			exchange,
			"direct",
			false,
			false,
			false,
			false,
			nil,
		); err != nil {
			return nil, fmt.Errorf("could not declare exchange %s: %w", exchange, err)
		}
	}
	return &Publisher{ch: ch}, nil
}

// PublishGameCreated publishes a game created event.
func (p *Publisher) PublishGameCreated(ctx context.Context, event pubevts.GameCreatedEvent) error {
	return p.publishEvent(ctx, pubevts.ExchangeGameCreated, event)
}

// PublishPlayerJoinRequested publishes a player wanting to join a match event.
func (p *Publisher) PublishPlayerJoinRequested(ctx context.Context, event pubevts.PlayerJoinRequestedEvent) error {
	return p.publishEvent(ctx, pubevts.ExchangePlayerJoinRequested, event)
}

// PublishPlayerJoined publishes a player joined event.
func (p *Publisher) PublishPlayerJoined(ctx context.Context, event pubevts.PlayerJoinedEvent) error {
	return p.publishEvent(ctx, pubevts.ExchangePlayerJoined, event)
}

// PublishPlayerMatchError publishes a player match error event.
func (p *Publisher) PublishPlayerMatchError(ctx context.Context, event pubevts.PlayerMatchErrorEvent) error {
	return p.publishEvent(ctx, pubevts.ExchangePlayerMatchError, event)
}

// publishEvent publishes an event to a specific exchange.
func (p *Publisher) publishEvent(ctx context.Context, exchange string, event interface{}) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("could not marshal event: %w", err)
	}

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultTimeout)
		defer cancel()
	}

	if err := p.ch.PublishWithContext(
		ctx,
		exchange,
		"",
		false,
		false,
		amqp091.Publishing{
			ContentType: pubevts.ContentType,
			Body:        data,
		},
	); err != nil {
		return fmt.Errorf("could not publish event to %s: %w", exchange, err)
	}
	return nil
}

// SetupInputExchangeBindings binds the input exchanges to the matchmaking queue.
func SetupInputExchangeBindings(ch *amqp091.Channel, queueName string) error {
	if err := ch.ExchangeDeclare(
		pubevts.ExchangeMatchRequest,
		"direct",
		false,
		false,
		false,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("failed to declare input exchange: %w", err)
	}

	if err := ch.QueueBind(
		queueName,
		pubevts.RoutingKeyHostRegistration,
		pubevts.ExchangeMatchRequest,
		false,
		nil, // args
	); err != nil {
		return fmt.Errorf("could not bind queue to host registration routing key: %w", err)
	}

	if err := ch.QueueBind(
		queueName,
		pubevts.RoutingKeyHostRegistrationRemoval,
		pubevts.ExchangeMatchRequest,
		false,
		nil, // args
	); err != nil {
		return fmt.Errorf("could not bind queue to host registration routing key: %w", err)
	}

	if err := ch.QueueBind(
		queueName,
		pubevts.RoutingKeyPlayerMatchRequest,
		pubevts.ExchangeMatchRequest,
		false,
		nil, // args
	); err != nil {
		return fmt.Errorf("could not bind queue to match request routing key: %w", err)
	}

	if err := ch.QueueBind(
		queueName,
		pubevts.RoutingKeyPlayerMatchRequestRemoval,
		pubevts.ExchangeMatchRequest,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("could not bind queue to match request routing key: %w", err)
	}
	return nil
}

// SetupOutputExchangeQueueBindings binds the exchanges for the monitoring queues.
func SetupOutputExchangeQueueBindings(ch *amqp091.Channel) error {
	for _, exchange := range exchanges {
		queueName := "monitor." + exchange
		if _, err := ch.QueueDeclare(
			queueName,
			false,
			false,
			false,
			false,
			nil,
		); err != nil {
			return fmt.Errorf("failed to declare monitor queue for %s: %w", exchange, err)
		}

		if err := ch.QueueBind(
			queueName,
			"",
			exchange,
			false,
			nil,
		); err != nil {
			return fmt.Errorf("failed to bind monitor queue to %s: %w", exchange, err)
		}
		fmt.Printf("Created monitoring queue %s for exchange %s\n", queueName, exchange)
	}
	return nil
}
