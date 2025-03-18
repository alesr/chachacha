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
)

type redisClient interface {
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
	SAdd(ctx context.Context, key string, members ...interface{}) *redis.IntCmd
	Get(ctx context.Context, key string) *redis.StringCmd
	SRem(ctx context.Context, key string, members ...interface{}) *redis.IntCmd
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
}

type Redis struct {
	client redisClient
}

func NewRedisRepo(redisCli redisClient) (*Redis, error) {
	return &Redis{client: redisCli}, nil
}

func (s *Redis) StoreHost(ctx context.Context, host pubevts.HostRegistratioEvent) error {
	key := hostKeyPrefix + host.HostID

	data, err := json.Marshal(host)
	if err != nil {
		return fmt.Errorf("could not marshal host data: %w", err)
	}

	if err := s.client.Set(ctx, key, data, hostTTL).Err(); err != nil {
		return fmt.Errorf("could not store host in Redis: %w", err)
	}

	if err := s.client.SAdd(ctx, hostsSetKey, key).Err(); err != nil {
		return fmt.Errorf("could not add host to set: %w", err)
	}
	return nil
}

func (s *Redis) UpdateHostAvailableSlots(ctx context.Context, hostIP string, slots uint16) error {
	key := hostKeyPrefix + hostIP

	// Get the current host data
	data, err := s.client.Get(ctx, key).Bytes()
	if err != nil {
		return fmt.Errorf("could not get host data for updating: %w", err)
	}

	var host pubevts.HostRegistratioEvent
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

func (s *Redis) StorePlayer(ctx context.Context, player pubevts.MatchRequestEvent) error {
	key := playerKeyPrefix + player.PlayerID

	data, err := json.Marshal(player)
	if err != nil {
		return fmt.Errorf("could not marshal player data: %w", err)
	}

	if err := s.client.Set(ctx, key, data, playerTTL).Err(); err != nil {
		return fmt.Errorf("could not store player in Redis: %w", err)
	}

	if err := s.client.SAdd(ctx, playersSetKey, key).Err(); err != nil {
		return fmt.Errorf("could not add player to set: %w", err)
	}
	return nil
}

func (s *Redis) RemovePlayer(ctx context.Context, playerID string) error {
	key := playerKeyPrefix + playerID

	// Remove from the players set
	if err := s.client.SRem(ctx, playersSetKey, key).Err(); err != nil {
		return fmt.Errorf("could not remove player from set: %w", err)
	}

	// Delete the player data
	if err := s.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("could not delete player data: %w", err)
	}
	return nil
}

func (s *Redis) StoreGameSession(ctx context.Context, session *Session) error {
	key := sessionKeyPrefix + session.ID

	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("could not marshal game session data: %w", err)
	}

	// Store the game session with TTL
	if err := s.client.Set(ctx, key, data, sessionTTL).Err(); err != nil {
		return fmt.Errorf("could not store game session in Redis: %w", err)
	}

	// Add to the active sessions set
	if err := s.client.SAdd(ctx, activeSessionsKey, key).Err(); err != nil {
		return fmt.Errorf("could not add session to active sessions set: %w", err)
	}
	return nil
}

func (s *Redis) GetGameSession(ctx context.Context, sessionID string) (*Session, error) {
	key := sessionKeyPrefix + sessionID

	data, err := s.client.Get(ctx, key).Bytes()
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

func (s *Redis) GetActiveGameSessions(ctx context.Context) ([]*Session, error) {
	sessionKeys, err := s.client.SMembers(ctx, activeSessionsKey).Result()
	if err != nil {
		return nil, fmt.Errorf("could not get active session keys from set: %w", err)
	}

	sessions := make([]*Session, 0, len(sessionKeys))
	for _, key := range sessionKeys {
		data, err := s.client.Get(ctx, key).Bytes()
		if err != nil {
			if err == redis.Nil {
				// Session expired but still in the set, clean it up
				s.client.SRem(ctx, activeSessionsKey, key)
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

func (s *Redis) GetHosts(ctx context.Context) ([]pubevts.HostRegistratioEvent, error) {
	hostKeys, err := s.client.SMembers(ctx, hostsSetKey).Result()
	if err != nil {
		return nil, fmt.Errorf("could not get host keys from set: %w", err)
	}

	hosts := make([]pubevts.HostRegistratioEvent, 0, len(hostKeys))
	for _, key := range hostKeys {
		data, err := s.client.Get(ctx, key).Bytes()
		if err != nil {
			if err == redis.Nil {
				// Host expired but still in the set, clean it up
				s.client.SRem(ctx, hostsSetKey, key)
				continue
			}
			return nil, fmt.Errorf("could not get host data from Redis: %w", err)
		}

		var host pubevts.HostRegistratioEvent
		if err := json.Unmarshal(data, &host); err != nil {
			return nil, fmt.Errorf("could not unmarshal host data: %w", err)
		}
		hosts = append(hosts, host)
	}
	return hosts, nil
}

func (s *Redis) GetPlayers(ctx context.Context) ([]pubevts.MatchRequestEvent, error) {
	playerKeys, err := s.client.SMembers(ctx, playersSetKey).Result()
	if err != nil {
		return nil, fmt.Errorf("could not get player keys from set: %w", err)
	}

	players := make([]pubevts.MatchRequestEvent, 0, len(playerKeys))
	for _, key := range playerKeys {
		data, err := s.client.Get(ctx, key).Bytes()
		if err != nil {
			if err == redis.Nil {
				// Player expired but still in the set, clean it up
				s.client.SRem(ctx, playersSetKey, key)
				continue
			}
			return nil, fmt.Errorf("could not get player data from Redis: %w", err)
		}

		var player pubevts.MatchRequestEvent
		if err := json.Unmarshal(data, &player); err != nil {
			return nil, fmt.Errorf("could not unmarshal player data: %w", err)
		}
		players = append(players, player)
	}
	return players, nil
}
