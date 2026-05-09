package trigger

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
)

// ActionDeps is the shared bag of dependencies handed to every action
// handler. Constructed once at boot in cmd/server/main.go; tests can
// build a partial bundle and leave optional fields nil.
//
// Every dependency is optional from the runner's perspective — a
// handler that needs an absent dep should log + return nil so the
// fan-out doesn't abort.
type ActionDeps struct {
	Rooms    repo.RoomRepo
	Mobs     repo.MobInstanceRepo
	Sessions *session.Registry
	Logger   *slog.Logger
	// Triggers is the repo handle the runner uses to persist
	// fault-budget mutations (Phase F #32 slice 1). nil disables
	// fault-budget persistence — tests that don't care about the
	// budget pass nil here and the runner falls back to in-memory
	// counter mutation only.
	Triggers repo.TriggerRepo
}

// ActionRegistry maps an ActionKind name (e.g. "say") to its handler.
// Concurrent-safe; new handlers can be registered at boot before the
// dispatcher starts.
type ActionRegistry struct {
	mu       sync.RWMutex
	handlers map[string]ActionHandler
}

// NewActionRegistry returns an empty registry. Use DefaultActions to
// get one pre-populated with the V1 builtins.
func NewActionRegistry() *ActionRegistry {
	return &ActionRegistry{handlers: make(map[string]ActionHandler)}
}

// DefaultActions returns a registry pre-populated with the V1 built-in
// action handlers (`noop`, `say`, `emote`).
func DefaultActions() *ActionRegistry {
	r := NewActionRegistry()
	r.Register("noop", NoopAction)
	r.Register("say", SayAction)
	r.Register("emote", EmoteAction)
	return r
}

// Register adds (or overwrites) the handler for the given kind name.
func (r *ActionRegistry) Register(kind string, h ActionHandler) {
	if r == nil || h == nil || strings.TrimSpace(kind) == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[kind] = h
}

// Lookup returns the handler for kind, or nil if none is registered.
func (r *ActionRegistry) Lookup(kind string) ActionHandler {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.handlers[kind]
}

// NoopAction is the default no-op handler. Logs an optional payload
// message at debug level. Useful for tests and as the V1 fallback when
// a custom action is mis-named in YAML — the dispatcher logs an
// "unknown action" warning at the runner instead.
func NoopAction(ctx context.Context, deps ActionDeps, owner OwnerRef, ev EventCtx, payload json.RawMessage) error {
	var p LogPayload
	_ = json.Unmarshal(payload, &p) // payload is optional
	loggerOr(deps).Debug("trigger.noop",
		"event", string(ev.Event),
		"owner_kind", string(owner.Kind),
		"owner_id", owner.ID,
		"room_id", owner.RoomID,
		"message", p.Message)
	return nil
}

// SayAction broadcasts third-person speech from the trigger's owner
// to every other session in the same room. Payload shape:
// `{"text": "..."}`. Empty text is a no-op.
//
// Mob owners speak as `<MobName> says, "<text>"` (resolved from the
// mob_instance row). Room owners speak as `A voice murmurs, "<text>"`.
//
// All peer writes go through Session.WriteAsync — the handler runs on
// the eventbus goroutine, NOT a dispatcher, so the cross-session
// output rule mandates async writes that re-paint the prompt cache.
func SayAction(ctx context.Context, deps ActionDeps, owner OwnerRef, ev EventCtx, payload json.RawMessage) error {
	text, ok := decodeText(payload)
	if !ok {
		return nil
	}
	speaker := resolveSpeaker(ctx, deps, owner)
	line := fmt.Sprintf("{{%s says,}}::cyan \"{{%s}}::white\"\r\n",
		defangCfmt(speaker), defangCfmt(text))
	broadcastToRoom(deps, owner.RoomID, line)
	return nil
}

// EmoteAction broadcasts a third-person emote — same plumbing as
// SayAction but formatted as `<name> <text>` with no quotes.
func EmoteAction(ctx context.Context, deps ActionDeps, owner OwnerRef, ev EventCtx, payload json.RawMessage) error {
	text, ok := decodeText(payload)
	if !ok {
		return nil
	}
	speaker := resolveSpeaker(ctx, deps, owner)
	line := fmt.Sprintf("{{%s %s}}::cyan\r\n", defangCfmt(speaker), defangCfmt(text))
	broadcastToRoom(deps, owner.RoomID, line)
	return nil
}

// decodeText parses a SayPayload. Returns ok=false on missing text so
// the action is silently skipped.
func decodeText(payload json.RawMessage) (string, bool) {
	if len(payload) == 0 {
		return "", false
	}
	var p SayPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return "", false
	}
	t := strings.TrimSpace(p.Text)
	if t == "" {
		return "", false
	}
	return t, true
}

// resolveSpeaker returns a display string for the trigger's owner.
// Mob owners look up the live MobInstance to grab Core.Name; room
// owners fall back to "A voice".
func resolveSpeaker(ctx context.Context, deps ActionDeps, owner OwnerRef) string {
	switch owner.Kind {
	case OwnerMobTemplate:
		if deps.Mobs != nil && owner.InstanceID != 0 {
			if inst, err := deps.Mobs.GetByID(ctx, owner.InstanceID); err == nil {
				name := strings.TrimSpace(inst.Core.Name)
				if name != "" {
					return name
				}
			}
		}
		return "Someone"
	case OwnerRoom:
		return "A voice"
	}
	return "Someone"
}

// broadcastToRoom writes line via WriteAsync to every session whose
// CurrentRoomID matches roomID. Caller is responsible for cfmt
// formatting; line should already end with CRLF.
func broadcastToRoom(deps ActionDeps, roomID int64, line string) {
	if deps.Sessions == nil || roomID == 0 {
		return
	}
	for _, peer := range deps.Sessions.Snapshot() {
		_, _, peerRoom := peer.InWorld()
		if peerRoom != roomID {
			continue
		}
		if err := peer.WriteAsync(line); err != nil {
			loggerOr(deps).Debug("trigger broadcast failed", "room", roomID, "error", err)
		}
	}
}

// defangCfmt neutralises cfmt template syntax inside a string spliced
// into a colored template, AND strips C0 control bytes (`< 0x20` plus
// DEL) so a payload can't smuggle a bare CR, ESC, or SGR sequence
// through to the terminal. Mirrors cmd/comm.go::sanitizeChat — when
// the §32 Lua surface lands and payloads carry runtime-computed
// strings, this is the only thing standing between an attacker-
// controlled byte and every session in the room.
func defangCfmt(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	return strings.NewReplacer("{{", "{ {", "}}", "} }", "::", ": :").Replace(b.String())
}
