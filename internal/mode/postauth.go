package mode

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"sync/atomic"
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
	accounts        repo.AccountRepo
	sessions        *session.Registry
	logins          repo.AccountLoginRepo
	builders        repo.BuilderZoneRepo
	accountUsername string
}

// LoginEventFunc is the optional hook fired by promoteToGame
// immediately after the character finishes promotion into the world
// (post-SetInWorld). main.go wires a closure that publishes
// world.PlayerLoggedIn on the eventbus so the trigger dispatcher can
// fan out `on_login` to room-owned triggers. nil disables — tests
// leave it unset and the publish is skipped.
//
// Lives as a package-level var rather than a constructor parameter
// because promoteToGame has four call sites (character_select,
// character_create x2, account_menu_play) and threading a closure
// through every one would balloon the diff without adding clarity.
// Tests in this package never set the publisher; production sets it
// once at boot before accepting connections.
type LoginEventFunc func(characterID, roomID int64)

// loginPublisher is the global hook stored as an atomic pointer so
// the boot-time write in main() establishes a happens-before edge
// to every read in promoteToGame regardless of which goroutine is
// dispatching. SetLoginPublisher installs it; promoteToGame
// Load()s and nil-checks before calling.
var loginPublisher atomic.Pointer[LoginEventFunc]

// SetLoginPublisher installs the optional login publisher. Called
// once at boot by cmd/server/main.go. Safe to call with nil to
// clear (tests). Atomic Store provides the memory barrier the
// promoteToGame readers rely on; no need to set before the listener
// accepts its first connection.
func SetLoginPublisher(f LoginEventFunc) {
	if f == nil {
		loginPublisher.Store(nil)
		return
	}
	loginPublisher.Store(&f)
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
		// Surface a generic notice and return the underlying error so
		// the caller (Login / Create) can reset its state machine
		// instead of silently looping at stepPassword with the mask
		// off. Write failures are reported back as-is.
		if werr := s.WriteRaw([]byte("Could not load characters. Try again later.\r\n")); werr != nil {
			return werr
		}
		return err
	}
	// Resolve the account row's AccountSettings (slice 3). A repo
	// failure or a nil accounts wiring leaves settings at the zero
	// value, which round-trips as "use server defaults" — login
	// proceeds rather than failing on a settings load. Logged for
	// forensics when a real lookup fails so a corrupt or missing row
	// doesn't disappear silently.
	var settings repo.AccountSettings
	if deps.accounts != nil && s.AccountID > 0 {
		if a, lookupErr := deps.accounts.FindByID(ctx, s.AccountID); lookupErr == nil {
			settings = a.Settings
		} else if !errors.Is(lookupErr, repo.ErrAccountNotFound) {
			slog.Warn("postAuth: load account settings failed",
				"account", s.AccountID, "error", lookupErr)
		}
	}
	// Fire MOTD/news once per successful login, before routing to a
	// post-auth mode. MOTDAlways flattens the per-character
	// last_news_seen watermark to zero so every entry re-renders.
	if motd != nil {
		var lastSeen time.Time
		if len(chars) > 0 {
			lastSeen = chars[0].LastNewsSeen
		}
		if settings.MOTDAlways {
			lastSeen = time.Time{}
		}
		if err := motd(s, lastSeen); err != nil {
			return err
		}
	}
	if len(chars) == 0 {
		create := NewCharacterCreate(characters, game)
		create.SetCatalog(catalog)
		create.SetSettings(settings)
		create.SetItems(deps.items)
		create.SetBuilders(deps.builders)
		return s.ReplaceMode(create)
	}
	menu := NewAccountMenu(chars, characters, game)
	menu.SetMOTD(motd)
	menu.SetCatalog(catalog)
	menu.SetItems(deps.items)
	menu.SetAudits(deps.audits)
	menu.SetAccounts(deps.accounts)
	menu.SetSessions(deps.sessions)
	menu.SetAccountUsername(deps.accountUsername)
	menu.SetSettings(settings)
	menu.SetLogins(deps.logins)
	menu.SetBuilders(deps.builders)
	return s.ReplaceMode(menu)
}

// remoteHost extracts the host portion of a RemoteAddress string
// (typically "host:port" from net.Conn.RemoteAddr().String()). Falls
// back to the input verbatim when the value doesn't contain a port —
// IPv6 link-local without brackets, unix sockets, test fakes — so the
// audit log still records *something* useful in odd environments.
func remoteHost(addr string) string {
	if addr == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}

// applyAccountSettings stamps the slice-3 session-scoped settings
// (color level, terminal width) onto the session. Called from
// promoteToGame just before in-world fields land so the next render
// (and the cached prompt repaint) sees the new values. Lives next to
// promoteToGame so the apply order stays one-stop-shoppable.
//
// ColorOverride is parsed at the menu boundary already, but a stale or
// hand-edited row could carry an unrecognised token; ParseColorLevel
// silently rejects rather than crashing the promote, and the session
// keeps its TERM-detected level. Width 0/<40/>200 is similarly tolerated.
func applyAccountSettings(s *telnet.Session, settings repo.AccountSettings) {
	if level, ok := telnet.ParseColorLevel(settings.ColorOverride); ok && settings.ColorOverride != "" {
		s.ColorLevel = level
	}
	if w := settings.WidthOverride; w >= 40 && w <= 200 {
		s.Width = w
	}
}

// promoteToGame stamps the character onto the session, records play
// time (best-effort), and replaces the mode with game. The MOTD hook
// no longer fires here — postAuth runs it once per login before the
// AccountMenu lands, so re-selecting a character via `play` inside the
// menu does not re-render news.
func promoteToGame(ctx context.Context, s *telnet.Session, c repo.Character, characters repo.CharacterRepo, builders repo.BuilderZoneRepo, game telnet.Mode) error {
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
	// Phase F #32 slice 5b — publish world.PlayerLoggedIn so the
	// trigger dispatcher can fire room-owned on_login triggers. The
	// publisher is wired by main.go at boot; nil in tests. Atomic
	// load is the synchronization barrier paired with the Store in
	// SetLoginPublisher.
	if fn := loginPublisher.Load(); fn != nil {
		(*fn)(c.ID, roomID)
	}
	s.Speed = c.Core.Speed
	s.SetChannelMuted(c.ChannelSettings)
	s.SetLastNewsSeen(c.LastNewsSeen)
	// Phase G #33 — cache the character's per-zone builder grants on
	// the session so #34 OLC verbs (redit / oedit / medit / zedit) can
	// gate via cmd.CanEditZone without a repo hit per dispatch. Load
	// failure is non-fatal: the session lands with an empty grant set
	// (AuthAdmin still bypasses; a builder will see "no permission"
	// until next login). nil repo skips the load entirely — tests that
	// don't wire it and the pre-§G login path both proceed unchanged.
	if builders != nil {
		rows, err := builders.ListForCharacter(ctx, c.ID)
		if err != nil {
			slog.Warn("promote: builder grants load failed",
				"char", c.ID, "error", err)
		} else if len(rows) > 0 {
			grants := make(map[int64]struct{}, len(rows))
			for _, r := range rows {
				grants[r.ZoneID] = struct{}{}
			}
			s.SetBuilderZones(grants)
		}
	}
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
