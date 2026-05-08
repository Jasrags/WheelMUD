package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

func newEmbraceFixture(t *testing.T, channeling *creature.Channeling) (*repo.MemoryCharacterRepo, *telnet.Session, *bufConn) {
	t.Helper()
	chars := repo.NewMemoryCharacterRepo()
	if _, err := chars.Create(context.Background(), repo.Character{
		AccountID:     1,
		Name:          "Rand",
		CurrentRoomID: 1,
		Channeling:    channeling,
		AuthLevel:     uint8(telnet.AuthPlayer),
	}); err != nil {
		t.Fatalf("seed rand: %v", err)
	}
	s, conn := bufSession(t)
	s.AccountID = 1
	s.AuthLevel = telnet.AuthPlayer
	s.CharacterID = 1
	s.CharacterName = "Rand"
	s.CurrentRoomID = 1
	return chars, s, conn
}

func TestEmbrace_HappyPath(t *testing.T) {
	chars, s, out := newEmbraceFixture(t, &creature.Channeling{
		GenderSource: creature.SourceSaidin,
	})
	runCmd(t, NewEmbrace(chars), s, "")

	got, _ := chars.FindByName(context.Background(), "Rand")
	if !got.Channeling.Embraced {
		t.Fatalf("not embraced")
	}
	if got.Channeling.EmbracedSince.IsZero() {
		t.Fatalf("EmbracedSince not stamped")
	}
	if !strings.Contains(out.String(), "open yourself") {
		t.Fatalf("ack missing: %s", out.String())
	}
}

func TestEmbrace_NonChannelerRefuses(t *testing.T) {
	chars, s, out := newEmbraceFixture(t, nil)
	runCmd(t, NewEmbrace(chars), s, "")

	got, _ := chars.FindByName(context.Background(), "Rand")
	if got.Channeling != nil {
		t.Fatalf("channeling created: %+v", got.Channeling)
	}
	if !strings.Contains(out.String(), "cannot channel") {
		t.Fatalf("refusal missing: %s", out.String())
	}
}

func TestEmbrace_StilledRefuses(t *testing.T) {
	chars, s, out := newEmbraceFixture(t, &creature.Channeling{
		GenderSource: creature.SourceSaidin,
		Stilled:      true,
	})
	runCmd(t, NewEmbrace(chars), s, "")

	got, _ := chars.FindByName(context.Background(), "Rand")
	if got.Channeling.Embraced {
		t.Fatalf("stilled char became embraced")
	}
	if !strings.Contains(out.String(), "stilled") {
		t.Fatalf("refusal missing: %s", out.String())
	}
}

func TestRelease_HappyPath(t *testing.T) {
	chars, s, out := newEmbraceFixture(t, &creature.Channeling{
		GenderSource: creature.SourceSaidin,
		Embraced:     true,
	})
	runCmd(t, NewRelease(chars), s, "")

	got, _ := chars.FindByName(context.Background(), "Rand")
	if got.Channeling.Embraced {
		t.Fatalf("still embraced")
	}
	if !got.Channeling.EmbracedSince.IsZero() {
		t.Fatalf("EmbracedSince not cleared")
	}
	if !strings.Contains(out.String(), "release") {
		t.Fatalf("ack missing: %s", out.String())
	}
}

func TestRelease_NotEmbracedRefuses(t *testing.T) {
	chars, s, out := newEmbraceFixture(t, &creature.Channeling{
		GenderSource: creature.SourceSaidin,
		Embraced:     false,
	})
	runCmd(t, NewRelease(chars), s, "")

	if !strings.Contains(out.String(), "not holding") {
		t.Fatalf("refusal missing: %s", out.String())
	}
}
