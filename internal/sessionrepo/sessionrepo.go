package sessionrepo

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	pubevts "github.com/alesr/chachacha/pkg/events"
	"github.com/go-redis/redis/v8"
)

const (
	hostKeyPrefix     = "host:"
	playerKeyPrefix   = "player:"
	sessionKeyPrefix  = "session:"
	hostsSetKey       = "hosts"
	playersSetKey     = "players"
	activeSessionsKey = "active_sessions"
	hostTTL           = 5 * time.Minute
	playerTTL         = 2 * time.Minute
	sessionTTL        = 2 * time.Hour

	// Session states

	SessionStateAwaiting  = "awaiting"
	SessionStateComplete  = "complete"
	SessionStateFinished  = "finished"
	SessionStateCancelled = "cancelled"
)

type redisClient interface {
	Set(ctx context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd
	SAdd(ctx context.Context, key string, members ...any) *redis.IntCmd
	Get(ctx context.Context, key string) *redis.StringCmd
	SRem(ctx context.Context, key string, members ...any) *redis.IntCmd
	SMembers(ctx context.Context, key string) *redis.StringSliceCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
}

type Session struct {
	ID             string           `json:"session_id"`
	HostID         string           `json:"host_id"`
	Mode           pubevts.GameMode `json:"mode"`
	CreatedAt      time.Time        `json:"created_at"`
	Players        []string         `json:"players"`
	AvailableSlots uint16           `json:"available_slots"`
	State          string           `json:"state"`
}

type Redis struct {
	client redisClient
}

func NewRedisRepo(redisCli redisClient) (*Redis, error) {
	return &Redis{client: redisCli}, nil
}

// StoreHosts adds a new host now waiting for players to join.
func (r *Redis) StoreHost(ctx context.Context, host pubevts.HostRegistrationEvent) error {
	key := hostKeyPrefix + host.HostID

	data, err := json.Marshal(host)
	if err != nil {
		return fmt.Errorf("could not marshal host data: %w", err)
	}

	if err := r.client.Set(ctx, key, data, hostTTL).Err(); err != nil {
		return fmt.Errorf("could not store host in Redis: %w", err)
	}

	if err := r.client.SAdd(ctx, hostsSetKey, key).Err(); err != nil {
		return fmt.Errorf("could not add host to set: %w", err)
	}
	return nil
}

// RemoveHost removes a host.
// During the process of removing a host we should check if it has active matches,
// and handle the players accordingly.
func (s *Redis) RemoveHost(ctx context.Context, hostID string) error {
	key := hostKeyPrefix + hostID

	if err := s.client.SRem(ctx, hostsSetKey, key).Err(); err != nil {
		return fmt.Errorf("could not remove host from set: %w", err)
	}

	if err := s.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("could not delete host data: %w", err)
	}
	return nil
}

// UpdateHostAvailableSlots updates the slots of a match when a new player joins or leave a session.
func (s *Redis) UpdateHostAvailableSlots(ctx context.Context, hostID string, slots uint16) error {
	key := hostKeyPrefix + hostID

	data, err := s.client.Get(ctx, key).Bytes()
	if err != nil {
		return fmt.Errorf("could not get host data for updating: %w", err)
	}

	var host pubevts.HostRegistrationEvent
	if err := json.Unmarshal(data, &host); err != nil {
		return fmt.Errorf("could not unmarshal host data: %w", err)
	}

	host.AvailableSlots = slots

	updatedData, err := json.Marshal(host)
	if err != nil {
		return fmt.Errorf("could not marshal updated host data: %w", err)
	}

	if err := s.client.Set(ctx, key, updatedData, hostTTL).Err(); err != nil {
		return fmt.Errorf("could not update host in Redis: %w", err)
	}
	return nil
}

// StorePlayer adds a new player looking for a match.
func (r *Redis) StorePlayer(ctx context.Context, player pubevts.PlayerMatchRequestEvent) error {
	key := playerKeyPrefix + player.PlayerID

	data, err := json.Marshal(player)
	if err != nil {
		return fmt.Errorf("could not marshal player data: %w", err)
	}

	if err := r.client.Set(ctx, key, data, playerTTL).Err(); err != nil {
		return fmt.Errorf("could not store player in Redis: %w", err)
	}

	if err := r.client.SAdd(ctx, playersSetKey, key).Err(); err != nil {
		return fmt.Errorf("could not add player to set: %w", err)
	}
	return nil
}

// RemovePlayer removes a player.
// If the player is in an existing session, we should be removed from it
// and the avaiable session slots be updated.
func (s *Redis) RemovePlayer(ctx context.Context, playerID string) error {
	key := playerKeyPrefix + playerID

	if err := s.client.SRem(ctx, playersSetKey, key).Err(); err != nil {
		return fmt.Errorf("could not remove player from set: %w", err)
	}

	if err := s.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("could not delete player data: %w", err)
	}
	return nil
}

func (r *Redis) StoreGameSession(ctx context.Context, session *Session) error {
	key := sessionKeyPrefix + session.ID

	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("could not marshal game session data: %w", err)
	}

	if err := r.client.Set(ctx, key, data, sessionTTL).Err(); err != nil {
		return fmt.Errorf("could not store game session in Redis: %w", err)
	}

	if err := r.client.SAdd(ctx, activeSessionsKey, key).Err(); err != nil {
		return fmt.Errorf("could not add session to active sessions set: %w", err)
	}
	return nil
}

func (r *Redis) GetGameSession(ctx context.Context, sessionID string) (*Session, error) {
	key := sessionKeyPrefix + sessionID

	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("game session not found: %s", sessionID)
		}
		return nil, fmt.Errorf("could not get game session from Redis: %w", err)
	}

	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("could not unmarshal game session data: %w", err)
	}
	return &session, nil
}

func (r *Redis) GetActiveGameSessions(ctx context.Context) ([]*Session, error) {
	sessionKeys, err := r.client.SMembers(ctx, activeSessionsKey).Result()
	if err != nil {
		return nil, fmt.Errorf("could not get active session keys from set: %w", err)
	}

	sessions := make([]*Session, 0, len(sessionKeys))
	for _, key := range sessionKeys {
		data, err := r.client.Get(ctx, key).Bytes()
		if err != nil {
			if err == redis.Nil {
				r.client.SRem(ctx, activeSessionsKey, key)
				continue
			}
			return nil, fmt.Errorf("could not get session data from Redis: %w", err)
		}

		var session Session
		if err := json.Unmarshal(data, &session); err != nil {
			return nil, fmt.Errorf("could not unmarshal session data: %w", err)
		}
		sessions = append(sessions, &session)
	}
	return sessions, nil
}

func (r *Redis) GetHosts(ctx context.Context) ([]pubevts.HostRegistrationEvent, error) {
	hostKeys, err := r.client.SMembers(ctx, hostsSetKey).Result()
	if err != nil {
		return nil, fmt.Errorf("could not get host keys from set: %w", err)
	}

	hosts := make([]pubevts.HostRegistrationEvent, 0, len(hostKeys))
	for _, key := range hostKeys {
		data, err := r.client.Get(ctx, key).Bytes()
		if err != nil {
			if err == redis.Nil {
				r.client.SRem(ctx, hostsSetKey, key)
				continue
			}
			return nil, fmt.Errorf("could not get host data from Redis: %w", err)
		}

		var host pubevts.HostRegistrationEvent
		if err := json.Unmarshal(data, &host); err != nil {
			return nil, fmt.Errorf("could not unmarshal host data: %w", err)
		}
		hosts = append(hosts, host)
	}
	return hosts, nil
}

func (r *Redis) GetPlayers(ctx context.Context) ([]pubevts.PlayerMatchRequestEvent, error) {
	playerKeys, err := r.client.SMembers(ctx, playersSetKey).Result()
	if err != nil {
		return nil, fmt.Errorf("could not get player keys from set: %w", err)
	}

	players := make([]pubevts.PlayerMatchRequestEvent, 0, len(playerKeys))
	for _, key := range playerKeys {
		data, err := r.client.Get(ctx, key).Bytes()
		if err != nil {
			if err == redis.Nil {
				r.client.SRem(ctx, playersSetKey, key)
				continue
			}
			return nil, fmt.Errorf("could not get player data from Redis: %w", err)
		}

		var player pubevts.PlayerMatchRequestEvent
		if err := json.Unmarshal(data, &player); err != nil {
			return nil, fmt.Errorf("could not unmarshal player data: %w", err)
		}
		players = append(players, player)
	}
	return players, nil
}

func (r *Redis) GetHostInActiveSession(ctx context.Context, hostID string) (*Session, error) {
	sessions, err := r.GetActiveGameSessions(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not get active sessions: %w", err)
	}

	for _, session := range sessions {
		if session.HostID == hostID {
			return session, nil
		}
	}
	return nil, nil
}

func (r *Redis) GetPlayerInActiveSession(ctx context.Context, playerID string) (*Session, error) {
	sessions, err := r.GetActiveGameSessions(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not get active sessions: %w", err)
	}

	for _, session := range sessions {
		for _, player := range session.Players {
			if player == playerID {
				return session, nil
			}
		}
	}
	return nil, nil
}
