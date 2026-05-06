package mode

import (
	"context"
	"log/slog"
	"time"

	"github.com/Jasrags/WheelMUD/internal/chargen"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// postAuthDeps bundles the optional dependencies the AccountMenu
// needs for slice 1b+ (item cascade, audit row, live-session check,
// account username for audit attribution). Login + Create build it
// once and forward by value. Zero-value fields disable the
// corresponding feature gracefully — items=nil skips the cascade
// loop, audits=nil skips the audit row, sessions=nil skips the
// live-session check, accountUsername="" leaves the audit row's
// actor name empty.
type postAuthDeps struct {
	items           repo.ItemRepo
	audits          repo.AdminAuditRepo
	sessions        *session.Registry
	accountUsername string
}

// MOTDFunc is the hook fired once per successful login (immediately
// after Login.handlePassword / Create.handleConfirm succeed) by
// postAuth. It receives the watermark used to compute the unread
// count — the most-recently-played character's last_news_seen, or the
// zero time for fresh accounts. nil = skip.
//
// Lives on this package as a function type so internal/news (which
// imports telnet) can satisfy it without internal/mode taking a
// dependency on internal/news.
//
// Per the §6 ordering note in ROADMAP.md, the news block now fires
// before the AccountMenu is pushed (replacing the old in-game-promote
// firing) so players see news once per login regardless of which
// character they pick. The AccountMenu's `news` verb re-fires this
// hook on demand without advancing the watermark.
type MOTDFunc func(s *telnet.Session, lastSeen time.Time) error

// postAuth replaces the current mode with the appropriate
// post-authentication mode for the given account:
//
//	0 characters     → CharacterCreate (forced; menu has nothing to manage)
//	1+ characters    → AccountMenu (single hub, no auto-promote)
//
// Called only after Login or Create has set s.AccountID. The character
// list is fetched from the repo so a DB failure surfaces a generic
// error and leaves the session in its current mode for the user to
// retry. catalog is forwarded so the multi-step chargen flow is offered
// when content is available; nil preserves the legacy single-name flow.
//
// MOTD/news fires here once per login — before the menu lands, before
// any character is selected. The watermark used is chars[0].LastNewsSeen
// (most recently played, since ListByAccount orders by last_played_at
// desc) so a returning player sees only what's truly new since their
// last session on any character. Zero-character accounts pass the zero
// time which renders the full unread block.
func postAuth(ctx context.Context, s *telnet.Session, characters repo.CharacterRepo, motd MOTDFunc, catalog *chargen.Catalog, game telnet.Mode, deps postAuthDeps) error {
	chars, err := characters.ListByAccount(ctx, s.AccountID)
	if err != nil {
		slog.Warn("postAuth: list characters failed", "remote", s.RemoteAddress, "account", s.AccountID, "error", err)
		return s.WriteRaw([]byte("Could not load characters. Try again later.\r\n"))
	}
	// Fire MOTD/news once per successful login, before routing to a
	// post-auth mode. Errors here are surfaced — a write failure
	// almost certainly means the connection is dead and pushing a
	// mode after would do nothing useful.
	if motd != nil {
		var lastSeen time.Time
		if len(chars) > 0 {
			lastSeen = chars[0].LastNewsSeen
		}
		if err := motd(s, lastSeen); err != nil {
			return err
		}
	}
	if len(chars) == 0 {
		create := NewCharacterCreate(characters, game)
		create.SetCatalog(catalog)
		return s.ReplaceMode(create)
	}
	menu := NewAccountMenu(chars, characters, game)
	menu.SetMOTD(motd)
	menu.SetCatalog(catalog)
	menu.SetItems(deps.items)
	menu.SetAudits(deps.audits)
	menu.SetSessions(deps.sessions)
	menu.SetAccountUsername(deps.accountUsername)
	return s.ReplaceMode(menu)
}

// promoteToGame stamps the character onto the session, records play
// time (best-effort), and replaces the mode with game. The MOTD hook
// no longer fires here — postAuth runs it once per login before the
// AccountMenu lands, so re-selecting a character via `play` inside the
// menu does not re-render news.
func promoteToGame(ctx context.Context, s *telnet.Session, c repo.Character, characters repo.CharacterRepo, game telnet.Mode) error {
	roomID := c.CurrentRoomID
	if roomID == 0 {
		// Defensive: a character row missing a room id (e.g. created
		// before the column existed) gets dropped at the starter so
		// look / move have somewhere to anchor. Resolve before the
		// SetInWorld write so the snapshot any foreign reader sees
		// is internally consistent.
		roomID = repo.StarterRoomID
	}
	s.SetInWorld(c.ID, c.Name, roomID)
	s.Speed = c.Core.Speed
	s.SetChannelMuted(c.ChannelSettings)
	s.SetLastNewsSeen(c.LastNewsSeen)
	// AuthLevel lives on the character. CharacterRepo.Create promotes
	// the very first character on a fresh deploy to admin atomically,
	// so this restore picks up that promotion as well as any later
	// admin-grant tooling.
	//
	// Floor policy (deliberate): if a character somehow ends up
	// stored at AuthGuest (0), we log it as an error and clamp the
	// session to AuthPlayer rather than refusing to promote. The
	// alternative — fail-closed and lock the player out — was
	// considered and rejected because the only way a row can land
	// at guest is a backfill miss in migration 0019 or a buggy
	// future code path; in both cases the operator-visible
	// slog.Error gives us a paper trail without taking the player
	// offline. If this tradeoff ever changes, return an error from
	// promoteToGame instead of clamping.
	s.AuthLevel = telnet.AuthLevel(c.AuthLevel)
	if s.AuthLevel < telnet.AuthPlayer {
		slog.Error("postauth: character stored at sub-player auth level",
			"character", c.ID, "level", c.AuthLevel)
		s.AuthLevel = telnet.AuthPlayer
	}
	if err := characters.RecordPlay(ctx, c.ID, time.Now()); err != nil {
		slog.Warn("RecordPlay failed", "char", c.ID, "error", err)
	}
	if err := s.WriteRaw([]byte("Playing as " + c.Name + ".\r\n")); err != nil {
		return err
	}
	return s.ReplaceMode(game)
}
