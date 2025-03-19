package events

import "time"

const (
	// ExchangeMatchRequest is intend to be used by clients
	// to register new hosts (new matches),
	// and players wanting to join matches.
	ExchangeMatchRequest = "input.direct"

	// Routing keys for direct exchange (matchmaking_queue)
	RoutingKeyHostRegistration          = "host.registration"
	RoutingKeyHostRegistrationRemoval   = "host.registration.removal"
	RoutingKeyPlayerMatchRequest        = "player.match.request"
	RoutingKeyPlayerMatchRequestRemoval = "player.match.request.removal"

	// Message types for the matchmaking queue.
	MsgTypeHostRegistration          = "host_registration"
	MsgTypeHostRegistrationRemoval   = "host_registration_removal"
	MsgTypePlayerMatchRequest        = "player_match_request"
	MsgTypePlayerMatchRequestRemoval = "player_match_request_removal"

	// Exchanges for monitoring queues.

	// ExchangeGameCreated reports a new game for users to join.
	ExchangeGameCreated = "game.created"

	// ExchangePlayerJoinRequested reports a new player that wants to join a hosted match.
	ExchangePlayerJoinRequested = "game.player.join.requested"

	// ExchangePlayerJoined reports a player joining a hosted match.
	ExchangePlayerJoined = "game.player.joined"

	// ExchangePlayerMatchError reports errors in the player match process.
	ExchangePlayerMatchError = "game.player.match.error"

	ContentType = "application/json"

	// Error codes

	ErrorCodeHostNotFound = "HOST_NOT_FOUND"
)

// GameMode represents the mode of the game.
// Eg.: 1v1, 2v2, classic, FreeForAll ...
// It's up to the user of the engine to specify the game mode identifier.
type GameMode string

// HostRegistrationEvent represents the event for hosting a new match.
type HostRegistrationEvent struct {
	HostID         string   `json:"host_id"`
	Mode           GameMode `json:"mode"`
	AvailableSlots uint16   `json:"available_slots"`
}

// HostRegistrationRemovalEvent represents the event for removing a host.
type HostRegistrationRemovalEvent struct {
	HostID string `json:"host_id"`
}

// PlayerMatchRequestEvent represents the event for a player wanting to joining a match.
type PlayerMatchRequestEvent struct {
	PlayerID string    `json:"player_id"`
	HostID   *string   `json:"host_id,omitempty"`
	Mode     *GameMode `json:"mode,omitempty"`
}

// PlayerMatchRequestRemovalEvent represents the event for a player leaving the lobby.
type PlayerMatchRequestRemovalEvent struct {
	PlayerID string `json:"player_id"`
}

// GameCreatedEvent represents a new game creation event.
type GameCreatedEvent struct {
	GameID     string    `json:"game_id"`
	HostID     string    `json:"host_id"`
	GameMode   GameMode  `json:"game_mode"`
	MaxPlayers uint16    `json:"max_players"`
	CreatedAt  time.Time `json:"createdAt"`
}

// PlayerJoinRequestedEvent represents a player wanting to join a match.
type PlayerJoinRequestedEvent struct {
	PlayerID  string    `json:"player_id"`
	HostID    *string   `json:"host_id,omitempty"`
	GameMode  *GameMode `json:"game_mode,omitempty"`
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

// PlayerMatchErrorEvent represents an error in the player matching process.
type PlayerMatchErrorEvent struct {
	PlayerID        string    `json:"player_id"`
	ErrorCode       string    `json:"error_code"`
	ErrorMessage    string    `json:"error_message"`
	RequestedHostID *string   `json:"requested_host_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}
