package events

import "time"

const (
	// ExchangeMatchRequest is intend to be used by clients
	// to register new hosts (new matches),
	// and players wanting to join matches.
	ExchangeMatchRequest = "input.direct"

	// Routing keys for direct exchange (matchmaking_queue)
	RoutingKeyHostRegistration = "host.registration"
	RoutingKeyMatchRequest     = "player.match.request"

	// Message types for the matchmaking queue.
	MsgTypeHostRegistration   = "host_registration"
	MsgTypePlayerMatchRequest = "player_match_request"

	// Exchanges for monitoring queues.

	// ExchangeGameCreated reports a new game for users to join.
	ExchangeGameCreated = "game.created"

	// ExchangePlayerJoinRequested reports a new player that wants to join a hosted match.
	ExchangePlayerJoinRequested = "game.player.join.requested"

	// ExchangePlayerJoined reports a player joining a hosted match.
	ExchangePlayerJoined = "game.player.joined"

	ContentType = "application/json"
)

// GameMode represents the mode of the game.
// Eg.: 1v1, 2v2, classic, FreeForAll ...
// It's up to the user of the engine to specify the game mode identifier.
type GameMode string

// HostRegistratioEvent represents the event for hosting a new game.
type HostRegistratioEvent struct {
	HostID         string   `json:"host_id"`
	Mode           GameMode `json:"mode"`
	AvailableSlots uint16   `json:"available_slots"`
}

// MatchRequestEvent represents the message payload for joining an existing game.
type MatchRequestEvent struct {
	PlayerID string    `json:"player_id"`
	HostID   *string   `json:"host_id,omitempty"`
	Mode     *GameMode `json:"mode,omitempty"`
}

// GameCreatedEvent represents a new game creation event.
type GameCreatedEvent struct {
	GameID     string    `json:"game_id"`
	HostID     string    `json:"host_id"`
	GameMode   string    `json:"game_mode"`
	MaxPlayers uint16    `json:"max_players"`
	CreatedAt  time.Time `json:"createdAt"`
}

// PlayerJoinRequestedEvent represents a player wanting to join a match.
type PlayerJoinRequestedEvent struct {
	PlayerID  string    `json:"player_id"`
	HostID    *string   `json:"host_id,omitempty"`
	GameMode  *string   `json:"game_mode,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// PlayerJoinedEvent represents a player joining a game event.
type PlayerJoinedEvent struct {
	GameID             string    `json:"game_id"`
	HostID             string    `json:"host_id"`
	PlayerID           string    `json:"player_id"`
	CurrentPlayerCount int       `json:"current_player_count"`
	MaxPlayers         uint16    `json:"max_players"`
	AvailableSlots     uint16    `json:"available_slots"`
	Players            []string  `json:"players"`
	JoinedAt           time.Time `json:"joined_at"`
}
