package events

import "time"

const (
	// Exchanges for different event types
	ExchangeGameCreated         = "game.created"
	ExchangePlayerJoinRequested = "game.player.join.requested"
	ExchangePlayerJoined        = "game.player.joined"

	MsgTypeHostRegistration = "host_registration"
	MsgTypeMatchRequest     = "match_request"

	ContentType = "application/json"
)

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
	MaxPlayers         int8      `json:"max_players"`
	AvailableSlots     int8      `json:"available_slots"`
	Players            []string  `json:"players"`
	JoinedAt           time.Time `json:"joined_at"`
}
