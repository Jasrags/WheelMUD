package gmcp

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/Jasrags/WheelMUD/internal/eventbus"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// Manager owns the GMCP server side: it routes inbound Core.* frames
// from clients, installs eventbus subscriptions per session opt-in,
// and tears them down on disconnect. One Manager instance per server.
//
// Manager is safe for concurrent use across the read goroutine (which
// invokes Handle) and the eventbus dispatch goroutine (which invokes
// the per-subscription callbacks): the only shared mutable state is
// the per-Session subscription slice, which is guarded by crossMu via
// Session.AddGMCPSub / TakeGMCPSubs.
type Manager struct {
	bus        *eventbus.Bus
	sessions   *session.Registry
	characters repo.CharacterRepo
	rooms      repo.RoomRepo
	exits      repo.ExitRepo
	zones      repo.ZoneRepo
}

// New constructs a Manager. Every dep is required — Manager makes no
// nil-guard concessions because a misconfigured server is better
// caught at boot than at first DO GMCP.
func New(bus *eventbus.Bus, sessions *session.Registry, characters repo.CharacterRepo,
	rooms repo.RoomRepo, exits repo.ExitRepo, zones repo.ZoneRepo) *Manager {
	return &Manager{
		bus:        bus,
		sessions:   sessions,
		characters: characters,
		rooms:      rooms,
		exits:      exits,
		zones:      zones,
	}
}

// Handle is the closure assigned to Session.GMCPHandler. It runs on
// the read goroutine, so it must not block on slow I/O. Inbound
// Core.* frames are processed synchronously; everything else is
// logged at debug.
func (m *Manager) Handle(s *telnet.Session, pkg string, body []byte) {
	switch pkg {
	case "Core.Hello":
		// Body is `{"client":"Mudlet","version":"4.18.6"}`. Logged for
		// diagnostics; nothing to reply with.
		var hello struct {
			Client  string `json:"client"`
			Version string `json:"version"`
		}
		if err := json.Unmarshal(body, &hello); err == nil {
			slog.Info("GMCP Core.Hello",
				"remote", s.RemoteAddress, "client", hello.Client, "version", hello.Version)
		}
	case "Core.Ping":
		// Echo back so Mudlet can measure round-trip time. Body is
		// usually a number-of-milliseconds the client wants reflected.
		// Validate the body parses as JSON before echoing — re-emitting
		// arbitrary client bytes inside a GMCP frame risks shipping
		// malformed JSON that any third-party listener would choke on.
		// On parse failure, echo `null` so the client still sees a
		// round-trip event but no malformed payload.
		var validated json.RawMessage
		if err := json.Unmarshal(body, &validated); err != nil || len(validated) == 0 {
			validated = json.RawMessage("null")
		}
		if err := s.WriteGMCP("Core.Ping", validated); err != nil {
			slog.Debug("GMCP Core.Ping echo failed", "remote", s.RemoteAddress, "error", err)
		}
	case "Core.Supports.Set":
		m.handleSupports(s, body, supportsSet)
	case "Core.Supports.Add":
		m.handleSupports(s, body, supportsAdd)
	case "Core.Supports.Remove":
		m.handleSupports(s, body, supportsRemove)
	default:
		slog.Debug("GMCP unhandled package", "remote", s.RemoteAddress, "pkg", pkg)
	}
}

// supportsAction is the discriminator for Set / Add / Remove on the
// Core.Supports family. Keeping the parsing path shared avoids three
// near-identical handlers.
type supportsAction uint8

const (
	supportsSet supportsAction = iota
	supportsAdd
	supportsRemove
)

// handleSupports parses a Core.Supports.{Set,Add,Remove} body and
// reconciles the session's subscription state.
//
// Wire shape: a JSON array of strings, each `"<package> <version>"`
// (e.g. `"Char.Vitals 1"`). We strip the version (we accept all
// versions — V1 doesn't gate on it) and key by package name.
func (m *Manager) handleSupports(s *telnet.Session, body []byte, action supportsAction) {
	var raw []string
	if err := json.Unmarshal(body, &raw); err != nil {
		slog.Debug("GMCP Core.Supports parse failed", "remote", s.RemoteAddress, "error", err)
		return
	}

	// Project current map (or nil) for the merge.
	cur := s.GMCPSupports()
	if cur == nil {
		cur = make(map[string]int)
	}

	switch action {
	case supportsSet:
		// Wipe and replace.
		cur = make(map[string]int, len(raw))
		fallthrough
	case supportsAdd:
		for _, entry := range raw {
			name, ver := splitSupports(entry)
			if name == "" {
				continue
			}
			cur[name] = ver
		}
	case supportsRemove:
		for _, entry := range raw {
			name, _ := splitSupports(entry)
			delete(cur, name)
		}
	}

	s.SetGMCPSupports(cur)
	// Rewire from scratch on any Supports change. The subscription
	// surface is small; simpler than computing a delta and risking a
	// stale subscription.
	m.unwireLocked(s)
	m.wireForSupports(s, cur)
	if action != supportsRemove {
		m.emitInitialSnapshot(s)
	}
}

// splitSupports takes `"Char.Vitals 1"` and returns ("Char.Vitals", 1).
// A missing version defaults to 0; bad input returns ("", 0).
func splitSupports(s string) (string, int) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", 0
	}
	if i := strings.IndexByte(s, ' '); i > 0 {
		name := s[:i]
		ver := 0
		for j := i + 1; j < len(s); j++ {
			if s[j] < '0' || s[j] > '9' {
				break
			}
			ver = ver*10 + int(s[j]-'0')
		}
		return name, ver
	}
	return s, 0
}

// UnwireSession cancels every eventbus subscription owned by the
// session. Called from cmd/server/main.go's handleConnection defer
// before the registry is unbound, so the session's CharacterID is
// still valid for any final filter calls inside cancellation.
func (m *Manager) UnwireSession(s *telnet.Session) {
	m.unwireLocked(s)
}

// unwireLocked is the shared cancel path used by both UnwireSession
// (full teardown) and handleSupports (rewire on opt-in change). The
// "Locked" suffix is aspirational — Session.TakeGMCPSubs uses crossMu
// internally; no explicit lock needed here. *eventbus.Subscription
// satisfies the telnet.Canceler interface, so the cancel call is
// type-safe and idempotent.
func (m *Manager) unwireLocked(s *telnet.Session) {
	for _, sub := range s.TakeGMCPSubs() {
		sub.Cancel()
	}
}

// emitInitialSnapshot is the one-shot push of currently-true state to
// a session that has just opted into one or more packages. Called
// after every Supports.Set and Supports.Add so a Mudlet reconnect
// repaints its UI without waiting for the next event tick.
func (m *Manager) emitInitialSnapshot(s *telnet.Session) {
	supports := s.GMCPSupports()
	if len(supports) == 0 {
		return
	}
	charID, _, _ := s.InWorld()
	if charID == 0 {
		// Pre-character session (still at account menu / chargen).
		// Nothing to snapshot.
		return
	}
	ctx := context.Background()
	ch, err := m.characters.GetByID(ctx, charID)
	if err != nil {
		slog.Debug("GMCP snapshot: character load failed",
			"char", charID, "error", err)
		return
	}

	if wantsPkg(supports, CatChar, PkgCharName) {
		writeSnapshot(s, PkgCharName, buildCharName(&ch))
	}
	if wantsPkg(supports, CatChar, PkgCharVitals) {
		v := buildCharVitals(&ch)
		s.SetGMCPLastVitals(v.HP, v.MaxHP, v.SP, v.MaxSP)
		writeSnapshot(s, PkgCharVitals, v)
	}
	if wantsPkg(supports, CatChar, PkgCharStatus) {
		writeSnapshot(s, PkgCharStatus, buildCharStatus(&ch))
	}
	if wantsPkg(supports, CatRoom, PkgRoomInfo) {
		m.emitRoomInfo(ctx, s, ch.CurrentRoomID)
	}
}

// writeSnapshot wraps Session.WriteGMCP so initial-snapshot failures
// (typically a dead pipe or a marshal bug) log at debug instead of
// being thrown away under a `_ =`. The session is left intact — a
// follow-on event-driven emit may still land if the connection
// recovers.
func writeSnapshot(s *telnet.Session, pkg string, body any) {
	if err := s.WriteGMCP(pkg, body); err != nil {
		slog.Debug("gmcp: snapshot write failed",
			"remote", s.RemoteAddress, "pkg", pkg, "error", err)
	}
}

// emitRoomInfo loads the room + its exits + zone name and writes a
// Room.Info frame. Best-effort: any repo error degrades to a no-op
// with a debug log.
func (m *Manager) emitRoomInfo(ctx context.Context, s *telnet.Session, roomID int64) {
	if roomID == 0 {
		return
	}
	room, err := m.rooms.FindByID(ctx, roomID)
	if err != nil {
		slog.Debug("GMCP Room.Info: room load failed", "room", roomID, "error", err)
		return
	}
	exits, err := m.exits.ListFrom(ctx, roomID)
	if err != nil {
		slog.Debug("GMCP Room.Info: exit load failed", "room", roomID, "error", err)
		// Still emit Room.Info with empty exits map; better than nothing.
		exits = nil
	}
	zoneExt := ""
	if room.ZoneID != 0 && m.zones != nil {
		if z, zerr := m.zones.GetByID(ctx, room.ZoneID); zerr == nil {
			zoneExt = z.ExternalID
		}
	}
	writeSnapshot(s, PkgRoomInfo, buildRoomInfo(room, exits, zoneExt))
}

// wantsPkg reports whether a Supports map enables a specific outbound
// package. Mudlet uses either a category opt-in ("Char 1" enables
// every Char.* package) or a specific opt-in ("Char.Vitals 1" enables
// just that one); we accept both.
func wantsPkg(supports map[string]int, category, pkg string) bool {
	if _, ok := supports[category]; ok {
		return true
	}
	_, ok := supports[pkg]
	return ok
}

// wantsCategory reports whether any opt-in enables the named
// category. Used by the comm subscribers, where we don't have a
// specific package name per channel.
func wantsCategory(supports map[string]int, category string) bool {
	if _, ok := supports[category]; ok {
		return true
	}
	prefix := category + "."
	for k := range supports {
		if strings.HasPrefix(k, prefix) {
			return true
		}
	}
	return false
}
