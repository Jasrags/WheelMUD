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

// PlayerSaid fires after the `say` verb has finished its
// room-broadcast. Subscribers (Phase F #29 trigger dispatcher, future
// NPC dialogue #30) consume it to resolve `on_say` triggers attached
// to mobs/rooms in the speaker's room. Text is the post-sanitised
// utterance — control bytes already stripped, cfmt already defanged.
type PlayerSaid struct {
	SpeakerCharacterID int64
	RoomID             int64
	Text               string
}

// PlayerLoggedIn fires once when a character finishes promoteToGame —
// i.e., the session has stamped CharacterID + CurrentRoomID and the
// game mode is now driving input. Trigger dispatcher consumes this
// to fire room-owned `on_login` triggers for the room the character
// materialized in. Phase F #32 slice 5b. Published synchronously by
// internal/mode/postauth.go's loginPublisher hook so any ordering
// between this and PlayerEntered (which only fires on movement) is
// deterministic — login does NOT currently publish a PlayerEntered.
type PlayerLoggedIn struct {
	CharacterID int64
	RoomID      int64
}

// ChannelBroadcast fires after a successful chat-channel send (gossip,
// ooc, newbie, etc.) — every per-channel verb in
// internal/cmd/channel.go publishes one of these. Subscribers (Phase
// I #46 GMCP `Comm.Channel.Text` emitter) consume it to push
// out-of-band frames to opted-in clients. Text is post-sanitized
// (control bytes stripped, cfmt defanged) so subscribers can render
// it verbatim. Channel is the lowercase canonical name from the
// channel catalog.
type ChannelBroadcast struct {
	Channel            string
	SpeakerCharacterID int64
	SpeakerName        string
	Text               string
}

// PlayerTold fires after a successful `tell` delivery — one event per
// resolved recipient. Phase I #46 GMCP `Comm.Channel.Text` emits two
// frames per event: one to the recipient (channel=tell.in) and one
// to the sender (channel=tell.out) so Mudlet's chat-capture pane
// can route both halves to a tell-only window. Text is sanitized.
type PlayerTold struct {
	FromCharacterID int64
	ToCharacterID   int64
	FromName        string
	ToName          string
	Text            string
}

// PlayerLoggedOut fires once when a session with an active character
// disconnects. Published from handleConnection's defer block before
// session.Registry.Unbind so trigger action handlers can still find
// the session via the registry if they want a final write. Phase F
// #32 slice 5b. Account-menu-only sessions (no character ever
// selected) do NOT publish — the defer guards on s.CharacterID != 0.
type PlayerLoggedOut struct {
	CharacterID int64
	RoomID      int64
}
