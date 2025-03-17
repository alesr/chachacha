package matchdirector

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/alesr/chachacha/internal/sessionrepo"
	"github.com/alesr/chachacha/pkg/game"
	"github.com/oklog/ulid/v2"
)

type repository interface {
	GetHosts(ctx context.Context) ([]game.HostRegistratioMessage, error)
	GetPlayers(ctx context.Context) ([]game.MatchRequestMessage, error)
	StoreGameSession(ctx context.Context, session *sessionrepo.Session) error
	UpdateHostAvailableSlots(ctx context.Context, hostIP string, slots int8) error
	RemovePlayer(ctx context.Context, playerID string) error
}

type MatchDirector struct {
	logger      *slog.Logger
	repo        repository
	matchTicker *time.Ticker
	stopChan    chan struct{}
}

func New(logger *slog.Logger, repo repository, matchInterval time.Duration) (*MatchDirector, error) {
	return &MatchDirector{
		logger:      logger.WithGroup("match_director"),
		repo:        repo,
		matchTicker: time.NewTicker(matchInterval),
		stopChan:    make(chan struct{}),
	}, nil
}

func (md *MatchDirector) Start() {
	md.logger.Info("Starting match director")

	go func() {
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
	close(md.stopChan)
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

	md.logger.Debug("Found hosts and players for matching", slog.Int("len_hosts", len(hosts)), slog.Int("len_players", len(players)))

	var (
		matchesFound    int
		sessionsCreated int
	)

	// Maps to keep track of hosts and players that need updating
	playersToRemove := make(map[string]bool)
	sessionsByHost := make(map[string]*sessionrepo.Session)

	// First pass: Match players with specific host requests
	for i := range hosts {
		host := &hosts[i]

		// Get or create a session for this host
		session, exists := sessionsByHost[host.HostID]
		if !exists {
			session = &sessionrepo.Session{
				ID:             ulid.Make().String(),
				HostIP:         host.HostID,
				Mode:           host.Mode,
				CreatedAt:      time.Now(),
				Players:        []string{},
				AvailableSlots: host.AvailableSlots,
			}
			sessionsByHost[host.HostID] = session
		}

		// First, match players that specifically requested this host
		for _, player := range players {
			// Skip players that have already been matched
			if playersToRemove[player.PlayerID] {
				continue
			}

			// Check if player specified this host IP
			if player.HostID != nil && *player.HostID == host.HostID {
				// Check if host has available slots
				if session.AvailableSlots <= 0 {
					md.logger.Debug(
						"Host has no available slots for player",
						slog.String("host_ip", host.HostID),
						slog.String("player_id", player.PlayerID),
					)
					continue
				}

				// Check if player specified a game mode and it matches
				if player.Mode != nil && *player.Mode != host.Mode {
					md.logger.Info(
						"Game mode mismatch for player and host",
						slog.String("player_id", player.PlayerID), slog.String("host_ip", host.HostID),
						slog.String("player_mode", string(*player.Mode)),
						slog.String("host_mode", string(host.Mode)),
					)
					continue
				}

				// Match found!
				md.logger.Debug(
					"Matched player with specific host request for game mode",
					slog.String("player_id", player.PlayerID),
					slog.String("host_ip", host.HostID),
					slog.String("host_mode", string(host.Mode)),
				)

				session.Players = append(session.Players, player.PlayerID)
				session.AvailableSlots--
				playersToRemove[player.PlayerID] = true
				matchesFound++
			}
		}
	}

	// Second pass: Match remaining players with any compatible host
	for i := range hosts {
		host := &hosts[i]
		session := sessionsByHost[host.HostID]

		// Skip hosts with no available slots
		if session.AvailableSlots <= 0 {
			continue
		}

		for _, player := range players {
			// Skip players that have already been matched
			if playersToRemove[player.PlayerID] {
				continue
			}

			// Skip players who specified a different host
			if player.HostID != nil && *player.HostID != host.HostID {
				continue
			}

			// Check if player specified a game mode and it matches
			if player.Mode != nil && *player.Mode != host.Mode {
				continue
			}

			// Match found!
			md.logger.Debug(
				"Matched player with compatible host request for game mode",
				slog.String("player_id", player.PlayerID),
				slog.String("host_ip", host.HostID),
				slog.String("host_mode", string(host.Mode)),
			)

			session.Players = append(session.Players, player.PlayerID)
			session.AvailableSlots--
			playersToRemove[player.PlayerID] = true
			matchesFound++

			// If this host is full, move to the next host
			if session.AvailableSlots <= 0 {
				break
			}
		}
	}

	// Now, update all the hosts in Redis and create game sessions
	for hostIP, session := range sessionsByHost {
		if len(session.Players) > 0 {
			// Players were matched, create a game session
			if err := md.repo.StoreGameSession(ctx, session); err != nil {
				md.logger.Error(
					"Error storing game session for host",
					slog.String("host_ip", hostIP), slog.String("error", err.Error()),
				)
				continue
			}

			// Update the host's available slots
			err := md.repo.UpdateHostAvailableSlots(ctx, hostIP, session.AvailableSlots)
			if err != nil {
				md.logger.Error(
					"Error updating available slots for host",
					slog.String("host_ip", hostIP), slog.String("error", err.Error()),
				)
			}
			sessionsCreated++
		}
	}

	// Remove matched players from the queue
	for playerID := range playersToRemove {
		if err := md.repo.RemovePlayer(ctx, playerID); err != nil {
			md.logger.Error(
				"Error removing player from queue",
				slog.String("player_ip", playerID), slog.String("error", err.Error()),
			)
		}
	}

	md.logger.Debug(
		"Matching complete: found matches and created game sessions",
		slog.Int("matches_found", matchesFound), slog.Int("sessions_created", sessionsCreated),
	)
	return nil
}
