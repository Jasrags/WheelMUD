package mode

import (
	"context"
	"log/slog"
	"time"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// MOTDFunc is the hook promoteToGame calls right before replacing
// into game mode. It receives the character's last_news_seen
// watermark so it can render an unread-count line. nil = skip.
//
// Lives on this package as a function type so internal/news (which
// imports telnet) can satisfy it without internal/mode taking a
// dependency on internal/news.
type MOTDFunc func(s *telnet.Session, lastSeen time.Time) error

// postAuth replaces the current mode with the appropriate
// post-authentication mode for the given account:
//
//	0 characters     → CharacterCreate (forced)
//	1 character      → game mode directly (auto-promote)
//	2+ characters    → CharacterSelect (lets the user pick)
//
// Called only after Login or Create has set s.AccountID and bumped
// AuthLevel. The character list is fetched from the repo, so a DB
// failure surfaces a generic error and leaves the session in its
// current mode for the user to retry.
func postAuth(ctx context.Context, s *telnet.Session, characters repo.CharacterRepo, motd MOTDFunc, game telnet.Mode) error {
	chars, err := characters.ListByAccount(ctx, s.AccountID)
	if err != nil {
		slog.Warn("postAuth: list characters failed", "remote", s.RemoteAddress, "account", s.AccountID, "error", err)
		return s.WriteRaw([]byte("Could not load characters. Try again later.\r\n"))
	}
	switch len(chars) {
	case 0:
		create := NewCharacterCreate(characters, game)
		create.SetMOTD(motd)
		return s.ReplaceMode(create)
	case 1:
		return promoteToGame(ctx, s, chars[0], characters, motd, game)
	default:
		sel := NewCharacterSelect(chars, characters, game)
		sel.SetMOTD(motd)
		return s.ReplaceMode(sel)
	}
}

// promoteToGame stamps the character onto the session, records play
// time (best-effort), runs the optional MOTD hook, and replaces the
// mode with game.
func promoteToGame(ctx context.Context, s *telnet.Session, c repo.Character, characters repo.CharacterRepo, motd MOTDFunc, game telnet.Mode) error {
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
	if motd != nil {
		if err := motd(s, c.LastNewsSeen); err != nil {
			return err
		}
	}
	return s.ReplaceMode(game)
}
