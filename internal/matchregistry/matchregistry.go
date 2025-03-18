package matchregistry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	pubevents "github.com/alesr/chachacha/pkg/events"
	"github.com/alesr/chachacha/pkg/game"
	"github.com/rabbitmq/amqp091-go"
)

const defaultCtxTimeout = time.Second * 5

type repository interface {
	StoreHost(ctx context.Context, host game.HostRegistratioMessage) error
	StorePlayer(ctx context.Context, player game.MatchRequestMessage) error
}

type consumer interface {
	Consume(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp091.Table) (<-chan amqp091.Delivery, error)
}

type publisher interface {
	PublishGameCreated(ctx context.Context, event pubevents.GameCreatedEvent) error
	PublishPlayerJoinRequested(ctx context.Context, event pubevents.PlayerJoinRequestedEvent) error
}

type MatchRegistry struct {
	logger            *slog.Logger
	repo              repository
	consumerQueueName string
	consumer          consumer
	publisher         publisher
	ctx               context.Context
	cancel            context.CancelFunc
}

func New(
	logger *slog.Logger,
	repo repository,
	consumerQueueName string,
	consumer consumer,
	publisher publisher,
) *MatchRegistry {
	ctx, cancel := context.WithCancel(context.Background())
	return &MatchRegistry{
		logger:            logger.WithGroup("match_registry"),
		repo:              repo,
		consumerQueueName: consumerQueueName,
		consumer:          consumer,
		publisher:         publisher,
		ctx:               ctx,
		cancel:            cancel,
	}
}

func (mr *MatchRegistry) Start() error {
	msgs, err := mr.consumer.Consume(
		mr.consumerQueueName, // queue from which messages are consumed
		"match_registry_consumer",
		true,  // auto-acknowledge: messages are automatically marked as delivered
		false, // non-exclusive: allows multiple consumers on the same queue
		false, // no-local: not used by RabbitMQ
		false, // no-wait: wait for the server's response
		nil,   // additional arguments
	)
	if err != nil {
		return fmt.Errorf("could not register consumer for queue '%s': %w", mr.consumerQueueName, err)
	}

	go func() {
		for {
			select {
			case d, ok := <-msgs:
				if !ok {
					// Channel was closed
					mr.logger.Info("Message channel was closed, shutting down")
					return
				}

				mr.logger.Debug("Received a message", slog.String("body", string(d.Body)))

				messageType := d.Type

				switch messageType {
				case pubevents.MsgTypeHostRegistration:
					var hostMsg game.HostRegistratioMessage
					if err := json.Unmarshal(d.Body, &hostMsg); err != nil {
						mr.logger.Error("Error unmarshaling host registration message", slog.String("error", err.Error()))
						continue
					}

					if err := mr.registerHost(hostMsg); err != nil {
						mr.logger.Error("Error processing host registration", slog.String("error", err.Error()))
					} else {
						mr.logger.Debug("Host registered successfully", slog.String("host_ip", hostMsg.HostID))
					}

				case pubevents.MsgTypeMatchRequest:
					var playerMsg game.MatchRequestMessage
					if err := json.Unmarshal(d.Body, &playerMsg); err != nil {
						mr.logger.Error("Error unmarshaling match request message", slog.String("error", err.Error()))
						continue
					}
					if err := mr.registerPlayer(playerMsg); err != nil {
						mr.logger.Error("Error processing match request", slog.String("error", err.Error()))
					} else {
						mr.logger.Debug("Player match request registered", slog.String("player_id", playerMsg.PlayerID))
					}

				default:
					mr.logger.Error("Unknown message type received", slog.String("message_type", messageType))

					// Try to detect message type from content
					if err := mr.tryDetectAndProcessMessage(d.Body); err != nil {
						mr.logger.Error("Failed to process message", slog.String("error", err.Error()))
					}
				}

			case <-mr.ctx.Done():
				// Context was cancelled, time to exit
				mr.logger.Info("Context cancelled, shutting down match registry")
				return
			}
		}
	}()

	mr.logger.Info("Match registry started, waiting for messages")

	// Wait until context is cancelled
	<-mr.ctx.Done()
	mr.logger.Info("Match registry shutdown complete")
	return nil
}

func (mr *MatchRegistry) Shutdown() {
	mr.cancel()
}

// tryDetectAndProcessMessage attempts to determine message type from its content
func (mr *MatchRegistry) tryDetectAndProcessMessage(msgBody []byte) error {
	// First, try to parse as a generic JSON object to check message fields
	var msg map[string]any
	if err := json.Unmarshal(msgBody, &msg); err != nil {
		return fmt.Errorf("could not parse message: %w", err)
	}

	// Check if it has the PlayerID field, which would indicate a match request
	if _, hasPlayerID := msg["player_id"]; hasPlayerID {
		var playerMsg game.MatchRequestMessage
		if err := json.Unmarshal(msgBody, &playerMsg); err != nil {
			return fmt.Errorf("could not parse as match request: %w", err)
		}
		return mr.registerPlayer(playerMsg)
	}

	// Otherwise, try as host registration
	if _, hasHostID := msg["host_id"]; hasHostID {
		var hostMsg game.HostRegistratioMessage
		if err := json.Unmarshal(msgBody, &hostMsg); err != nil {
			return fmt.Errorf("could not parse as host registration: %w", err)
		}
		return mr.registerHost(hostMsg)
	}
	return errors.New("could not determine message type")
}

func (mr *MatchRegistry) registerHost(msg game.HostRegistratioMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultCtxTimeout)
	defer cancel()

	if err := mr.repo.StoreHost(ctx, msg); err != nil {
		return fmt.Errorf("could not store host game message: %w", err)
	}

	event := pubevents.GameCreatedEvent{
		GameID:     msg.HostID, // Using host ID as game ID
		HostID:     msg.HostID,
		MaxPlayers: uint16(msg.AvailableSlots),
		GameMode:   string(msg.Mode),
		CreatedAt:  time.Now(),
	}

	if err := mr.publisher.PublishGameCreated(ctx, event); err != nil {
		mr.logger.Error(
			"Failed to publish game created event",
			slog.String("error", err.Error()),
			slog.String("host_id", msg.HostID),
		)
		// Continue despite publishing error
	} else {
		mr.logger.Debug(
			"Published game created event",
			slog.String("host_id", msg.HostID),
			slog.String("game_mode", string(msg.Mode)),
		)
	}
	return nil
}

func (mr *MatchRegistry) registerPlayer(msg game.MatchRequestMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultCtxTimeout)
	defer cancel()

	if err := mr.repo.StorePlayer(ctx, msg); err != nil {
		return fmt.Errorf("could not store join game message: %w", err)
	}

	var hostID string
	if msg.HostID != nil {
		hostID = *msg.HostID
	}

	var gameMode string
	if msg.Mode != nil {
		gameMode = string(*msg.Mode)
	}

	event := pubevents.PlayerJoinRequestedEvent{
		PlayerID:  msg.PlayerID,
		HostID:    &hostID,
		GameMode:  &gameMode,
		CreatedAt: time.Now(),
	}

	if err := mr.publisher.PublishPlayerJoinRequested(ctx, event); err != nil {
		mr.logger.Error(
			"Failed to player join requested event",
			slog.String("error", err.Error()),
			slog.String("host_id", hostID),
		)
		// Continue despite publishing error
	} else {
		mr.logger.Debug(
			"Published player join requested event",
			slog.String("host_id", hostID),
			slog.String("game_mode", gameMode),
		)
	}
	return nil
}
