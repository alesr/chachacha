package matchdirector

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/alesr/chachacha/internal/sessionrepo"
	pubevts "github.com/alesr/chachacha/pkg/events"
	"github.com/oklog/ulid/v2"
)

type repository interface {
	GetHosts(ctx context.Context) ([]pubevts.HostRegistrationEvent, error)
	GetPlayers(ctx context.Context) ([]pubevts.PlayerMatchRequestEvent, error)
	StorePlayer(ctx context.Context, player pubevts.PlayerMatchRequestEvent) error
	StoreGameSession(ctx context.Context, session *sessionrepo.Session) error
	UpdateHostAvailableSlots(ctx context.Context, hostID string, slots uint16) error
	RemovePlayer(ctx context.Context, playerID string) error
	RemoveHost(ctx context.Context, hostID string) error
	GetGameSession(ctx context.Context, sessionID string) (*sessionrepo.Session, error)
	GetActiveGameSessions(ctx context.Context) ([]*sessionrepo.Session, error)
	GetHostInActiveSession(ctx context.Context, hostID string) (*sessionrepo.Session, error)
	GetPlayerInActiveSession(ctx context.Context, playerID string) (*sessionrepo.Session, error)
}

type publisher interface {
	PublishPlayerJoinRequested(ctx context.Context, event pubevts.PlayerJoinRequestedEvent) error
}

type MatchDirector struct {
	logger      *slog.Logger
	repo        repository
	publisher   publisher
	matchTicker *time.Ticker
	stopChan    chan struct{}
	wg          sync.WaitGroup
}

func New(logger *slog.Logger, repo repository, publisher publisher, matchInterval time.Duration) (*MatchDirector, error) {
	return &MatchDirector{
		logger:      logger.WithGroup("match_director"),
		repo:        repo,
		publisher:   publisher,
		matchTicker: time.NewTicker(matchInterval),
		stopChan:    make(chan struct{}),
	}, nil
}

func (md *MatchDirector) Start() {
	md.logger.Info("Starting match director")
	md.wg.Add(1)
	go func() {
		defer md.wg.Done()
		for {
			select {
			case <-md.matchTicker.C:
				if err := md.matchPlayers(); err != nil {
					md.logger.Error("Error during player matching", slog.String("error", err.Error()))
				}
			case <-md.stopChan:
				md.logger.Info("Stopping match director")
				md.matchTicker.Stop()
				return
			}
		}
	}()
}

func (md *MatchDirector) Stop() {
	if md.stopChan != nil {
		close(md.stopChan)
		md.wg.Wait()
	}
}

func (md *MatchDirector) HandleHostDisconnection(ctx context.Context, hostID string) error {
	session, err := md.repo.GetHostInActiveSession(ctx, hostID)
	if err != nil {
		return fmt.Errorf("failed to get session for host: %w", err)
	}

	if session == nil {
		return md.repo.RemoveHost(ctx, hostID)
	}

	for _, playerID := range session.Players {
		playerEvent := pubevts.PlayerMatchRequestEvent{
			PlayerID: playerID,
			Mode:     &session.Mode,
		}

		if err := md.repo.StorePlayer(ctx, playerEvent); err != nil {
			md.logger.Error("Failed to requeue player",
				slog.String("player_id", playerID),
				slog.String("error", err.Error()),
			)
			continue
		}

		joinRequestEvent := pubevts.PlayerJoinRequestedEvent{
			PlayerID:  playerID,
			GameMode:  &session.Mode,
			CreatedAt: time.Now(),
		}

		if err := md.publisher.PublishPlayerJoinRequested(ctx, joinRequestEvent); err != nil {
			md.logger.Error("Failed to publish player requeue event",
				slog.String("player_id", playerID),
				slog.String("error", err.Error()),
			)
		}
	}

	session.State = sessionrepo.SessionStateCancelled
	if err := md.repo.StoreGameSession(ctx, session); err != nil {
		md.logger.Error("Failed to update session state",
			slog.String("session_id", session.ID),
			slog.String("error", err.Error()),
		)
	}
	return md.repo.RemoveHost(ctx, hostID)
}

func (md *MatchDirector) HandlePlayerDisconnection(ctx context.Context, playerID string) error {
	session, err := md.repo.GetPlayerInActiveSession(ctx, playerID)
	if err != nil {
		return fmt.Errorf("failed to get session for player: %w", err)
	}

	if session == nil {
		return md.repo.RemovePlayer(ctx, playerID)
	}

	newPlayers := make([]string, 0, len(session.Players))
	for _, p := range session.Players {
		if p != playerID {
			newPlayers = append(newPlayers, p)
		}
	}
	session.Players = newPlayers
	session.AvailableSlots++

	if session.State == sessionrepo.SessionStateComplete {
		session.State = sessionrepo.SessionStateAwaiting
	}

	if err := md.repo.StoreGameSession(ctx, session); err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	if err := md.repo.UpdateHostAvailableSlots(ctx, session.HostID, session.AvailableSlots); err != nil {
		md.logger.Error("Failed to update host slots",
			slog.String("host_id", session.HostID),
			slog.String("error", err.Error()),
		)
	}
	return md.repo.RemovePlayer(ctx, playerID)
}

func (md *MatchDirector) matchPlayers() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	hosts, err := md.repo.GetHosts(ctx)
	if err != nil {
		return fmt.Errorf("could not get hosts: %w", err)
	}

	players, err := md.repo.GetPlayers(ctx)
	if err != nil {
		return fmt.Errorf("could not get players: %w", err)
	}

	if len(hosts) == 0 {
		md.logger.Debug("No hosts available for match")
		return nil
	}

	if len(players) == 0 {
		md.logger.Debug("No players available for matching")
		return nil
	}

	activeSessions, err := md.repo.GetActiveGameSessions(ctx)
	if err != nil {
		return fmt.Errorf("could not get active sessions: %w", err)
	}

	availableSessions := make(map[string]*sessionrepo.Session)
	for _, session := range activeSessions {
		if session.State == sessionrepo.SessionStateAwaiting {
			availableSessions[session.HostID] = session
		}
	}

	md.logger.Debug("Starting matching process",
		slog.Int("hosts", len(hosts)),
		slog.Int("players", len(players)),
		slog.Int("available_sessions", len(availableSessions)),
	)

	matchResults := md.processMatches(hosts, players, availableSessions)

	if err := md.storeMatchResults(ctx, matchResults); err != nil {
		return fmt.Errorf("failed to store match results: %w", err)
	}

	return nil
}

type matchResult struct {
	session        *sessionrepo.Session
	matchedPlayers []string
}

func (md *MatchDirector) processMatches(
	hosts []pubevts.HostRegistrationEvent,
	players []pubevts.PlayerMatchRequestEvent,
	availableSessions map[string]*sessionrepo.Session,
) map[string]matchResult {
	results := make(map[string]matchResult)
	matchedPlayers := make(map[string]bool)

	// First, try to fill existing sessions
	for _, player := range players {
		if matchedPlayers[player.PlayerID] {
			continue
		}

		for hostID, session := range availableSessions {
			if session.AvailableSlots == 0 {
				continue
			}

			if player.HostID != nil && *player.HostID != hostID {
				continue
			}

			if player.Mode != nil && *player.Mode != session.Mode {
				continue
			}

			// Match found!
			result := results[hostID]
			if result.session == nil {
				result.session = session
				result.matchedPlayers = make([]string, 0)
			}

			result.matchedPlayers = append(result.matchedPlayers, player.PlayerID)
			result.session.Players = append(result.session.Players, player.PlayerID)
			result.session.AvailableSlots--
			matchedPlayers[player.PlayerID] = true
			results[hostID] = result

			if result.session.AvailableSlots == 0 {
				result.session.State = sessionrepo.SessionStateComplete
				break
			}
		}
	}

	// Then create new sessions for remaining players
	for _, host := range hosts {
		if _, exists := results[host.HostID]; exists {
			continue
		}

		session := &sessionrepo.Session{
			ID:             ulid.Make().String(),
			HostID:         host.HostID,
			Mode:           host.Mode,
			CreatedAt:      time.Now(),
			AvailableSlots: host.AvailableSlots,
			State:          sessionrepo.SessionStateAwaiting,
			Players:        make([]string, 0),
		}

		matchedForHost := make([]string, 0)

		for _, player := range players {
			if matchedPlayers[player.PlayerID] {
				continue
			}

			if player.HostID != nil && *player.HostID != host.HostID {
				continue
			}

			if player.Mode != nil && *player.Mode != host.Mode {
				continue
			}

			matchedForHost = append(matchedForHost, player.PlayerID)
			session.Players = append(session.Players, player.PlayerID)
			session.AvailableSlots--
			matchedPlayers[player.PlayerID] = true

			if session.AvailableSlots == 0 {
				session.State = sessionrepo.SessionStateComplete
				break
			}
		}

		if len(matchedForHost) > 0 {
			results[host.HostID] = matchResult{
				session:        session,
				matchedPlayers: matchedForHost,
			}
		}
	}

	return results
}

func (md *MatchDirector) storeMatchResults(ctx context.Context, results map[string]matchResult) error {
	for hostID, result := range results {
		if err := md.repo.StoreGameSession(ctx, result.session); err != nil {
			md.logger.Error("Failed to store game session",
				slog.String("host_id", hostID),
				slog.String("error", err.Error()),
			)
			continue
		}

		if err := md.repo.UpdateHostAvailableSlots(ctx, hostID, result.session.AvailableSlots); err != nil {
			md.logger.Error("Failed to update host slots",
				slog.String("host_id", hostID),
				slog.String("error", err.Error()),
			)
		}

		// Remove matched players and send notifications
		for _, playerID := range result.matchedPlayers {
			if err := md.repo.RemovePlayer(ctx, playerID); err != nil {
				md.logger.Error("Failed to remove matched player",
					slog.String("player_id", playerID),
					slog.String("error", err.Error()),
				)
				continue
			}

			event := pubevts.PlayerJoinRequestedEvent{
				PlayerID:  playerID,
				HostID:    &hostID,
				GameMode:  &result.session.Mode,
				CreatedAt: time.Now(),
			}

			if err := md.publisher.PublishPlayerJoinRequested(ctx, event); err != nil {
				md.logger.Error("Failed to publish player join event",
					slog.String("player_id", playerID),
					slog.String("error", err.Error()),
				)
			}
		}
	}

	return nil
}
