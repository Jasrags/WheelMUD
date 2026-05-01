package world

// Events emitted by the world layer. Subscribers register against the
// concrete type via eventbus.Subscribe. New event types live here so
// consumers (scripting, NPC AI, logging) have a single import.

// PlayerEntered fires after a character has been moved into a room
// and the move has been persisted (best-effort) to the character row.
// The previous room is FromRoomID; 0 means "first placement" (login,
// teleport into the world from nowhere).
type PlayerEntered struct {
	CharacterID int64
	FromRoomID  int64
	ToRoomID    int64
}

// PlayerLeft fires before a character is moved out of a room. Pair
// with PlayerEntered for full transition semantics.
type PlayerLeft struct {
	CharacterID int64
	FromRoomID  int64
	ToRoomID    int64
}
