package matchregistry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/alesr/chachacha/internal/sessionrepo"
	pubevts "github.com/alesr/chachacha/pkg/events"
	"github.com/rabbitmq/amqp091-go"
)

const defaultCtxTimeout = time.Second * 15

type repository interface {
	StoreHost(ctx context.Context, host pubevts.HostRegistrationEvent) error
	StorePlayer(ctx context.Context, player pubevts.PlayerMatchRequestEvent) error
	StoreGameSession(ctx context.Context, session *sessionrepo.Session) error
	GetHostInActiveSession(ctx context.Context, hostID string) (*sessionrepo.Session, error)
	RemoveHost(ctx context.Context, hostID string) error
	GetHosts(ctx context.Context) ([]pubevts.HostRegistrationEvent, error)
	GetPlayerInActiveSession(ctx context.Context, playerID string) (*sessionrepo.Session, error)
	RemovePlayer(ctx context.Context, playerID string) error
	UpdateHostAvailableSlots(ctx context.Context, hostID string, slots uint16) error
}

type consumer interface {
	Consume(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp091.Table) (<-chan amqp091.Delivery, error)
}

type publisher interface {
	PublishGameCreated(ctx context.Context, event pubevts.GameCreatedEvent) error
	PublishPlayerJoinRequested(ctx context.Context, event pubevts.PlayerJoinRequestedEvent) error
	PublishPlayerMatchError(ctx context.Context, event pubevts.PlayerMatchErrorEvent) error
}

type MatchRegistry struct {
	logger               *slog.Logger
	repo                 repository
	matchmakingQueueName string
	consumer             consumer
	publisher            publisher
	ctx                  context.Context
	cancel               context.CancelFunc
}

func New(
	logger *slog.Logger,
	repo repository,
	matchmakingQueueName string,
	consumer consumer,
	publisher publisher,
) *MatchRegistry {
	ctx, cancel := context.WithCancel(context.Background())
	return &MatchRegistry{
		logger:               logger.WithGroup("match_registry"),
		repo:                 repo,
		matchmakingQueueName: matchmakingQueueName,
		consumer:             consumer,
		publisher:            publisher,
		ctx:                  ctx,
		cancel:               cancel,
	}
}

func (mr *MatchRegistry) Start() error {
	msgs, err := mr.consumer.Consume(
		mr.matchmakingQueueName,
		"match_registry_consumer",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("could not register consumer: %w", err)
	}

	go func() {
		for {
			select {
			case d, ok := <-msgs:
				if !ok {
					mr.logger.Info("Message channel was closed, shutting down")
					return
				}

				mr.logger.Debug("Received a message", slog.String("body", string(d.Body)))

				switch d.Type {
				case pubevts.MsgTypeHostRegistration:
					var hostMsg pubevts.HostRegistrationEvent
					if err := json.Unmarshal(d.Body, &hostMsg); err != nil {
						mr.logger.Error("Error unmarshaling host registration", slog.String("error", err.Error()))
						continue
					}
					if err := mr.registerHost(hostMsg); err != nil {
						mr.logger.Error("Error processing host registration", slog.String("error", err.Error()))
					}

				case pubevts.MsgTypeHostRegistrationRemoval:
					var hostRemovalMsg pubevts.HostRegistrationRemovalEvent
					if err := json.Unmarshal(d.Body, &hostRemovalMsg); err != nil {
						mr.logger.Error("Error unmarshaling host removal", slog.String("error", err.Error()))
						continue
					}
					if err := mr.handleHostRemoval(hostRemovalMsg); err != nil {
						mr.logger.Error("Error processing host removal", slog.String("error", err.Error()))
					}

				case pubevts.MsgTypePlayerMatchRequest:
					var playerMsg pubevts.PlayerMatchRequestEvent
					if err := json.Unmarshal(d.Body, &playerMsg); err != nil {
						mr.logger.Error("Error unmarshaling match request", slog.String("error", err.Error()))
						continue
					}
					if err := mr.registerPlayer(playerMsg); err != nil {
						mr.logger.Error("Error processing match request", slog.String("error", err.Error()))
					} else {
						mr.logger.Debug("Player match request registered", slog.String("player_id", playerMsg.PlayerID))
					}

				case pubevts.MsgTypePlayerMatchRequestRemoval:
					var playerRemovalMsg pubevts.PlayerMatchRequestRemovalEvent
					if err := json.Unmarshal(d.Body, &playerRemovalMsg); err != nil {
						mr.logger.Error("Error unmarshaling player removal", slog.String("error", err.Error()))
						continue
					}
					if err := mr.handlePlayerRemoval(playerRemovalMsg); err != nil {
						mr.logger.Error("Error processing player removal", slog.String("error", err.Error()))
					}
				default:
					mr.logger.Error("Unknown message type received", slog.String("message_type", d.Type))

					if err := mr.tryDetectAndProcessMessage(d.Body); err != nil {
						mr.logger.Error("Failed to process message", slog.String("error", err.Error()))
					}
				}

			case <-mr.ctx.Done():
				mr.logger.Info("Context cancelled, shutting down match registry")
				return
			}
		}
	}()

	mr.logger.Info("Match registry started, waiting for messages")

	<-mr.ctx.Done()
	mr.logger.Info("Match registry shutdown complete")
	return nil
}

func (mr *MatchRegistry) Shutdown() {
	mr.cancel()
}

// tryDetectAndProcessMessage attempts to determine message type from its content
func (mr *MatchRegistry) tryDetectAndProcessMessage(msgBody []byte) error {
	var msg map[string]any
	if err := json.Unmarshal(msgBody, &msg); err != nil {
		return fmt.Errorf("could not parse message: %w", err)
	}

	// check if it has the PlayerID field, which would indicate a match request
	if _, hasPlayerID := msg["player_id"]; hasPlayerID {
		var playerMsg pubevts.PlayerMatchRequestEvent
		if err := json.Unmarshal(msgBody, &playerMsg); err != nil {
			return fmt.Errorf("could not parse as match request: %w", err)
		}
		return mr.registerPlayer(playerMsg)
	}

	// otherwise, try as host registration
	if _, hasHostID := msg["host_id"]; hasHostID {
		var hostMsg pubevts.HostRegistrationEvent
		if err := json.Unmarshal(msgBody, &hostMsg); err != nil {
			return fmt.Errorf("could not parse as host registration: %w", err)
		}
		return mr.registerHost(hostMsg)
	}
	return errors.New("could not determine message type")
}

func (mr *MatchRegistry) registerHost(msg pubevts.HostRegistrationEvent) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultCtxTimeout)
	defer cancel()

	if err := mr.repo.StoreHost(ctx, msg); err != nil {
		return fmt.Errorf("could not store host game message: %w", err)
	}

	event := pubevts.GameCreatedEvent{
		GameID:     msg.HostID, // using host ID as game ID
		HostID:     msg.HostID,
		MaxPlayers: uint16(msg.AvailableSlots),
		GameMode:   msg.Mode,
		CreatedAt:  time.Now(),
	}

	if err := mr.publisher.PublishGameCreated(ctx, event); err != nil {
		mr.logger.Error(
			"Failed to publish game created event",
			slog.String("error", err.Error()),
			slog.String("host_id", msg.HostID),
		)
	} else {
		mr.logger.Debug(
			"Published game created event",
			slog.String("host_id", msg.HostID),
			slog.String("game_mode", string(msg.Mode)),
		)
	}
	return nil
}

func (mr *MatchRegistry) handleHostRemoval(msg pubevts.HostRegistrationRemovalEvent) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultCtxTimeout)
	defer cancel()

	session, err := mr.repo.GetHostInActiveSession(ctx, msg.HostID)
	if err != nil {
		return fmt.Errorf("could not get session for host: %w", err)
	}

	if session == nil {
		// no active session, just remove the host
		return mr.repo.RemoveHost(ctx, msg.HostID)
	}

	// for each player in the session, create a new match request
	for _, playerID := range session.Players {
		playerMatchRequest := pubevts.PlayerMatchRequestEvent{
			PlayerID: playerID,
			// if they specifically requested this host, remove that preference
			// TODO: maybe we want to just publish an error and remove this player?
			HostID: nil,
			Mode:   &session.Mode,
		}

		if err := mr.repo.StorePlayer(ctx, playerMatchRequest); err != nil {
			mr.logger.Error(
				"Failed to requeue player after host removal",
				slog.String("player_id", playerID),
				slog.String("error", err.Error()),
			)
			continue
		}

		// publish event that player is back in matchmaking
		event := pubevts.PlayerJoinRequestedEvent{
			PlayerID:  playerID,
			GameMode:  &session.Mode,
			CreatedAt: time.Now(),
		}

		if err := mr.publisher.PublishPlayerJoinRequested(ctx, event); err != nil {
			mr.logger.Error(
				"Failed to publish player requeue event",
				slog.String("player_id", playerID),
				slog.String("error", err.Error()),
			)
		}
	}

	session.State = sessionrepo.SessionStateCancelled
	if err := mr.repo.StoreGameSession(ctx, session); err != nil {
		mr.logger.Error(
			"Failed to mark session as cancelled",
			slog.String("session_id", session.ID),
			slog.String("error", err.Error()),
		)
	}
	return mr.repo.RemoveHost(ctx, msg.HostID)
}

func (mr *MatchRegistry) registerPlayer(msg pubevts.PlayerMatchRequestEvent) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultCtxTimeout)
	defer cancel()

	if msg.HostID != nil && *msg.HostID != "" {
		hosts, err := mr.repo.GetHosts(ctx)
		if err != nil {
			mr.logger.Error("Could not check for host existence",
				slog.String("error", err.Error()),
				slog.String("player_id", msg.PlayerID))
		} else {
			var hostExists bool
			for _, host := range hosts {
				if host.HostID == *msg.HostID {
					hostExists = true
					break
				}
			}

			if !hostExists {
				mr.logger.Debug("Requested host not found",
					slog.String("player_id", msg.PlayerID),
					slog.String("requested_host", *msg.HostID))

				errorEvent := pubevts.PlayerMatchErrorEvent{
					PlayerID:        msg.PlayerID,
					ErrorCode:       pubevts.ErrorCodeHostNotFound,
					ErrorMessage:    "The requested host does not exist",
					RequestedHostID: msg.HostID,
					CreatedAt:       time.Now(),
				}

				if err := mr.publisher.PublishPlayerMatchError(ctx, errorEvent); err != nil {
					mr.logger.Error(
						"Failed to publish player match error event",
						slog.String("error", err.Error()),
						slog.String("player_id", msg.PlayerID),
					)
				} else {
					mr.logger.Debug(
						"Published player match error event",
						slog.String("player_id", msg.PlayerID),
						slog.String("error_code", errorEvent.ErrorCode),
					)
				}
				return nil
			}
		}
	}

	if err := mr.repo.StorePlayer(ctx, msg); err != nil {
		return fmt.Errorf("could not store join game message: %w", err)
	}

	var hostID string
	if msg.HostID != nil {
		hostID = *msg.HostID
	}

	var gameMode pubevts.GameMode
	if msg.Mode != nil {
		gameMode = *msg.Mode
	}

	event := pubevts.PlayerJoinRequestedEvent{
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
	} else {
		mr.logger.Debug(
			"Published player join requested event",
			slog.String("host_id", hostID),
			slog.String("game_mode", string(gameMode)),
		)
	}
	return nil
}

func (mr *MatchRegistry) handlePlayerRemoval(msg pubevts.PlayerMatchRequestRemovalEvent) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultCtxTimeout)
	defer cancel()

	session, err := mr.repo.GetPlayerInActiveSession(ctx, msg.PlayerID)
	if err != nil {
		return fmt.Errorf("could not get session for player: %w", err)
	}

	if session == nil {
		return mr.repo.RemovePlayer(ctx, msg.PlayerID)
	}

	newPlayers := make([]string, 0, len(session.Players))
	for _, p := range session.Players {
		if p != msg.PlayerID {
			newPlayers = append(newPlayers, p)
		}
	}
	session.Players = newPlayers
	session.AvailableSlots++

	if session.State == sessionrepo.SessionStateComplete {
		session.State = sessionrepo.SessionStateAwaiting
	}

	if err := mr.repo.StoreGameSession(ctx, session); err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	if err := mr.repo.UpdateHostAvailableSlots(ctx, session.HostID, session.AvailableSlots); err != nil {
		mr.logger.Error("Failed to update host slots",
			slog.String("host_id", session.HostID),
			slog.String("error", err.Error()),
		)
	}
	return mr.repo.RemovePlayer(ctx, msg.PlayerID)
}
