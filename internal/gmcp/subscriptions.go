package gmcp

import (
	"context"
	"log/slog"

	"github.com/Jasrags/WheelMUD/internal/affects"
	"github.com/Jasrags/WheelMUD/internal/combat"
	"github.com/Jasrags/WheelMUD/internal/eventbus"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/world"
	"github.com/Jasrags/WheelMUD/telnet"
)

// wireForSupports installs eventbus subscriptions for every category
// the session has opted into. Handles are stashed on the session via
// AddGMCPSub so UnwireSession can cancel them later.
//
// Each handler filters by the session's current CharacterID (or
// CurrentRoomID, for room-broadcast packages) before emitting. This
// means N subscribers iterate independently for each event, but at
// MUD scale (hundreds of players) the bus's RWMutex serialisation is
// fine. If we ever scale to thousands, move to a single shared
// subscription with a per-room/character session index.
func (m *Manager) wireForSupports(s *telnet.Session, supports map[string]int) {
	if len(supports) == 0 {
		return
	}
	if wantsPkg(supports, CatChar, PkgCharVitals) {
		m.wireCharVitals(s)
	}
	if wantsPkg(supports, CatChar, PkgCharStatus) {
		m.wireCharStatus(s)
	}
	if wantsPkg(supports, CatRoom, PkgRoomInfo) {
		m.wireRoomInfo(s)
	}
	if wantsCategory(supports, CatComm) {
		m.wireComm(s)
	}
}

// wireCharVitals subscribes to every HP/SP-mutating event and emits
// Char.Vitals when the event targets *this* session's character. The
// session-local last-vitals cache dedups runs of identical values
// (the regen + DoT pulse paths both produce streams of equal frames).
func (m *Manager) wireCharVitals(s *telnet.Session) {
	emit := func(_ context.Context) {
		charID, _, _ := s.InWorld()
		if charID == 0 {
			return
		}
		ch, err := m.characters.GetByID(context.Background(), charID)
		if err != nil {
			return
		}
		v := buildCharVitals(&ch)
		if s.GMCPLastVitalsEquals(v.HP, v.MaxHP, v.SP, v.MaxSP) {
			return
		}
		s.SetGMCPLastVitals(v.HP, v.MaxHP, v.SP, v.MaxSP)
		if err := s.WriteGMCP(PkgCharVitals, v); err != nil {
			slog.Debug("GMCP Char.Vitals write failed", "remote", s.RemoteAddress, "error", err)
		}
	}
	s.AddGMCPSub(eventbus.SubscribeAsync(m.bus, func(ctx context.Context, ev combat.CombatHit) {
		if isPlayer(ev.Defender, s) {
			emit(ctx)
		}
	}))
	s.AddGMCPSub(eventbus.SubscribeAsync(m.bus, func(ctx context.Context, ev affects.TickDamaged) {
		charID, _, _ := s.InWorld()
		if ev.CharacterID == charID {
			emit(ctx)
		}
	}))
	s.AddGMCPSub(eventbus.SubscribeAsync(m.bus, func(ctx context.Context, ev combat.CharacterDied) {
		if isPlayer(ev.Victim, s) {
			emit(ctx)
		}
	}))
	s.AddGMCPSub(eventbus.SubscribeAsync(m.bus, func(ctx context.Context, ev combat.CharacterRespawned) {
		if isPlayer(ev.Character, s) {
			emit(ctx)
		}
	}))
	s.AddGMCPSub(eventbus.SubscribeAsync(m.bus, func(ctx context.Context, ev combat.ScriptDamageDealt) {
		if isPlayer(ev.Target, s) {
			emit(ctx)
		}
	}))
	s.AddGMCPSub(eventbus.SubscribeAsync(m.bus, func(ctx context.Context, ev combat.ScriptHealingApplied) {
		if isPlayer(ev.Target, s) {
			emit(ctx)
		}
	}))
}

// wireCharStatus subscribes to identity-changing events. None of the
// event payloads carries the full character snapshot, so we re-fetch
// on each emit. Status events are rare enough (level-up, death/
// respawn) that the extra repo round-trip is negligible.
func (m *Manager) wireCharStatus(s *telnet.Session) {
	emit := func() {
		charID, _, _ := s.InWorld()
		if charID == 0 {
			return
		}
		ch, err := m.characters.GetByID(context.Background(), charID)
		if err != nil {
			return
		}
		_ = s.WriteGMCP(PkgCharStatus, buildCharStatus(&ch))
	}
	s.AddGMCPSub(eventbus.SubscribeAsync(m.bus, func(_ context.Context, ev combat.CharacterDied) {
		if isPlayer(ev.Victim, s) {
			emit()
		}
	}))
	s.AddGMCPSub(eventbus.SubscribeAsync(m.bus, func(_ context.Context, ev combat.CharacterRespawned) {
		if isPlayer(ev.Character, s) {
			emit()
		}
	}))
	s.AddGMCPSub(eventbus.SubscribeAsync(m.bus, func(_ context.Context, ev combat.CombatXPAwarded) {
		// XP-awarded carries an Awardee struct rather than a bare ActorRef;
		// match by ID.
		charID, _, _ := s.InWorld()
		if ev.Awardee.ID == charID {
			emit()
		}
	}))
}

// wireRoomInfo emits a Room.Info frame every time the session enters
// a new room. PlayerEntered carries CharacterID + ToRoomID, both of
// which we already trust the publisher to set.
func (m *Manager) wireRoomInfo(s *telnet.Session) {
	s.AddGMCPSub(eventbus.SubscribeAsync(m.bus, func(ctx context.Context, ev world.PlayerEntered) {
		charID, _, _ := s.InWorld()
		if ev.CharacterID != charID {
			return
		}
		m.emitRoomInfo(ctx, s, ev.ToRoomID)
	}))
}

// wireComm subscribes to every chat surface that produces a
// world.* event. PlayerSaid (room-local), ChannelBroadcast (zone-
// wide / global chat), and PlayerTold (point-to-point). Filter rules:
//
//   - PlayerSaid: subscriber must be in the speaker's room.
//   - ChannelBroadcast: subscriber must have the channel un-muted.
//   - PlayerTold: subscriber is the recipient (channel=tell.in) or
//     the sender (channel=tell.out).
func (m *Manager) wireComm(s *telnet.Session) {
	s.AddGMCPSub(eventbus.SubscribeAsync(m.bus, func(_ context.Context, ev world.PlayerSaid) {
		charID, _, roomID := s.InWorld()
		if charID == 0 || roomID != ev.RoomID {
			return
		}
		// Exclude the speaker's own session: their dispatcher already
		// wrote a synchronous `You say, "..."` line via WriteString.
		// Emitting a GMCP frame here would surface as a duplicate
		// entry in Mudlet's chat-capture pane.
		if charID == ev.SpeakerCharacterID {
			return
		}
		speaker := lookupSpeakerName(m, ev.SpeakerCharacterID)
		_ = s.WriteGMCP(PkgCommChannelText, CommChannelText{
			Channel: "say", Talker: speaker, Text: ev.Text,
		})
	}))
	s.AddGMCPSub(eventbus.SubscribeAsync(m.bus, func(_ context.Context, ev world.ChannelBroadcast) {
		charID, _, _ := s.InWorld()
		if charID == 0 {
			return
		}
		// Suppress if the receiver has muted the channel. Note the
		// speaker's own client doesn't receive the broadcast event-
		// driven Comm.Channel.Text — the channel verb's `selfMsg`
		// path takes a separate route in V1 (the synchronous reply).
		if s.IsChannelMuted(ev.Channel) {
			return
		}
		_ = s.WriteGMCP(PkgCommChannelText, CommChannelText{
			Channel: ev.Channel, Talker: ev.SpeakerName, Text: ev.Text,
		})
	}))
	s.AddGMCPSub(eventbus.SubscribeAsync(m.bus, func(_ context.Context, ev world.PlayerTold) {
		charID, _, _ := s.InWorld()
		switch charID {
		case ev.ToCharacterID:
			_ = s.WriteGMCP(PkgCommChannelText, CommChannelText{
				Channel: "tell.in", Talker: ev.FromName, Text: ev.Text,
			})
		case ev.FromCharacterID:
			_ = s.WriteGMCP(PkgCommChannelText, CommChannelText{
				Channel: "tell.out", Talker: ev.ToName, Text: ev.Text,
			})
		}
	}))
}

// isPlayer reports whether the actor reference points at the session's
// own character. Mob refs always return false even when the ID
// numerically matches a character ID — the kind tag is load-bearing.
func isPlayer(ref combat.ActorRef, s *telnet.Session) bool {
	if ref.Kind != combat.ActorKindCharacter {
		return false
	}
	charID, _, _ := s.InWorld()
	return ref.ID == charID
}

// lookupSpeakerName resolves a speaker character id to a display
// name. Best-effort: on repo error or zero id, returns "Someone" so
// the GMCP frame still ships intact. Repo errors log at debug level
// so operators can diagnose a broken DB without players noticing.
func lookupSpeakerName(m *Manager, charID int64) string {
	if charID == 0 {
		return "Someone"
	}
	ch, err := m.characters.GetByID(context.Background(), charID)
	if err != nil {
		slog.Debug("gmcp: lookupSpeakerName failed", "char_id", charID, "error", err)
		return "Someone"
	}
	return ch.Name
}

// Silence "imported and not used" on packages we will reference once
// future event sources land. Keep the import line; goimports will
// trim if truly unused.
var _ = repo.ErrCharacterNotFound
