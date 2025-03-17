package game

// GameMode represents the mode of the game.
// Eg.: 1v1, 2v2, classic, FreeForAll ...
// It's up to the user of the engine to specify the game mode identifier.
type GameMode string

// HostRegistratioMessage represents the message payload for hosting a new game.
type HostRegistratioMessage struct {
	HostID         string   `json:"host_id"`
	Mode           GameMode `json:"mode"`
	AvailableSlots int8     `json:"available_slots"`
}

// MatchRequestMessage represents the message payload for joining an existing game.
type MatchRequestMessage struct {
	PlayerID string    `json:"player_id"`
	HostID   *string   `json:"host_id,omitempty"`
	Mode     *GameMode `json:"mode,omitempty"`
}
