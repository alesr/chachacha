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

// Declare the exchanges we'll use.
var exchanges = []string{
	pubevts.ExchangeMatchRequest,
	pubevts.ExchangeGameCreated,
	pubevts.ExchangePlayerJoinRequested,
	pubevts.ExchangePlayerJoined,
}

// Publisher handles publishing events to RabbitMQ.
type Publisher struct {
	ch *amqp091.Channel
}

// NewPublisher creates a new event publisher.
func NewPublisher(ch *amqp091.Channel) (*Publisher, error) {
	for _, exchange := range exchanges {
		if err := ch.ExchangeDeclare(
			exchange, // name
			"direct", // type
			false,    // durable
			false,    // auto-deleted
			false,    // internal
			false,    // no-wait
			nil,      // arguments
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

// publishEvent publishes an event to a specific exchange.
func (p *Publisher) publishEvent(ctx context.Context, exchange string, event interface{}) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("could not marshal event: %w", err)
	}

	// Use context with timeout if one isn't provided
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultTimeout)
		defer cancel()
	}

	if err := p.ch.PublishWithContext(
		ctx,
		exchange,
		"",    // routing key
		false, // mandatory
		false, // immediate
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
		pubevts.ExchangeMatchRequest, // name
		"direct",                     // type
		false,                        // durable
		false,                        // auto-deleted
		false,                        // internal
		false,                        // no-wait
		nil,                          // arguments
	); err != nil {
		return fmt.Errorf("failed to declare input exchange: %w", err)
	}

	// Bind the host registration routing key to the matchmaking queue
	if err := ch.QueueBind(
		queueName,                          // queue name
		pubevts.RoutingKeyHostRegistration, // routing key
		pubevts.ExchangeMatchRequest,       // exchange
		false,                              // no-wait
		nil,                                // args
	); err != nil {
		return fmt.Errorf("could not bind queue to host registration routing key: %w", err)
	}

	// Bind the match request routing key to the matchmaking queue
	if err := ch.QueueBind(
		queueName,                      // queue name
		pubevts.RoutingKeyMatchRequest, // routing key
		pubevts.ExchangeMatchRequest,   // exchange (reusing the same direct exchange)
		false,                          // no-wait
		nil,                            // args
	); err != nil {
		return fmt.Errorf("could not bind queue to match request routing key: %w", err)
	}
	return nil
}

// SetupMonitoringQueues binds the exchanges for the monitoring queues.
func SetupMonitoringQueues(ch *amqp091.Channel) error {
	// Create a monitoring queue for each exchange

	for _, exchange := range exchanges {

		queueName := "monitor." + exchange

		if _, err := ch.QueueDeclare(
			queueName, // queue name
			false,     // not durable
			false,     // don't delete when unused
			false,     // not exclusive
			false,     // don't wait
			nil,       // no args
		); err != nil {
			return fmt.Errorf("failed to declare monitor queue for %s: %w", exchange, err)
		}

		// Bind queue to exchange
		if err := ch.QueueBind(
			queueName, // queue name
			"",        // routing key (empty for fanout exchanges)
			exchange,  // exchange name
			false,     // no-wait
			nil,       // no args
		); err != nil {
			return fmt.Errorf("failed to bind monitor queue to %s: %w", exchange, err)
		}
		fmt.Printf("Created monitoring queue %s for exchange %s\n", queueName, exchange)
	}
	return nil
}
