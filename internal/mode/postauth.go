package mode

import (
	"context"
	"log/slog"
	"time"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

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
func postAuth(ctx context.Context, s *telnet.Session, characters repo.CharacterRepo, game telnet.Mode) error {
	chars, err := characters.ListByAccount(ctx, s.AccountID)
	if err != nil {
		slog.Warn("postAuth: list characters failed", "remote", s.RemoteAddress, "account", s.AccountID, "error", err)
		return s.WriteRaw([]byte("Could not load characters. Try again later.\r\n"))
	}
	switch len(chars) {
	case 0:
		return s.ReplaceMode(NewCharacterCreate(characters, game))
	case 1:
		return promoteToGame(ctx, s, chars[0], characters, game)
	default:
		return s.ReplaceMode(NewCharacterSelect(chars, characters, game))
	}
}

// promoteToGame stamps the character onto the session, records play
// time (best-effort), and replaces the mode with game.
func promoteToGame(ctx context.Context, s *telnet.Session, c repo.Character, characters repo.CharacterRepo, game telnet.Mode) error {
	s.CharacterID = c.ID
	s.CharacterName = c.Name
	s.CurrentRoomID = c.CurrentRoomID
	s.Speed = c.Core.Speed
	s.SetChannelMuted(c.ChannelSettings)
	if s.CurrentRoomID == 0 {
		// Defensive: a character row missing a room id (e.g. created
		// before the column existed) gets dropped at the starter so
		// look / move have somewhere to anchor.
		s.CurrentRoomID = repo.StarterRoomID
	}
	if err := characters.RecordPlay(ctx, c.ID, time.Now()); err != nil {
		slog.Warn("RecordPlay failed", "char", c.ID, "error", err)
	}
	if err := s.WriteRaw([]byte("Playing as " + c.Name + ".\r\n")); err != nil {
		return err
	}
	return s.ReplaceMode(game)
}
