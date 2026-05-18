package cmd

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/emote"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

func loadTestSocials(t *testing.T) *emote.Catalog {
	t.Helper()
	fsys := fstest.MapFS{
		"test.yaml": &fstest.MapFile{Data: []byte(`socials:
  - id: smile
    help: smile [target] — a small, warm smile
    self: You smile.
    other: "{actor} smiles."
    target_self: You smile at {target}.
    target_view: "{actor} smiles at you."
    target_other: "{actor} smiles at {target}."
  - id: sigh
    help: sigh — a weary breath
    self: You sigh.
    other: "{actor} sighs."
`)},
	}
	cat, err := emote.Load(fsys)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return cat
}

func findSocialCmd(t *testing.T, cmds []*telnet.Command, name string) *telnet.Command {
	t.Helper()
	for _, c := range cmds {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("missing command %q", name)
	return nil
}

func TestSocial_UntargetedBroadcast(t *testing.T) {
	sessions, alice, _, aOut, bOut := commPair(t)
	cat := loadTestSocials(t)
	cmds := NewSocials(cat, sessions, nil)
	smile := findSocialCmd(t, cmds, "smile")

	runCmd(t, smile, alice, "")

	if !strings.Contains(aOut.String(), "You smile.") {
		t.Fatalf("alice self: %q", aOut.String())
	}
	if !strings.Contains(bOut.String(), "Alice smiles.") {
		t.Fatalf("bob other: %q", bOut.String())
	}
	// Style tag present (purple).
	if !strings.Contains(bOut.String(), "\x1b[35m") {
		t.Fatalf("bob output missing purple style: %q", bOut.String())
	}
}

func TestSocial_TargetedThreeWay(t *testing.T) {
	sessions, alice, bob, aOut, bOut := commPair(t)
	// Add Carol in the same room so we can observe the bystander line.
	carol, cOut := bufSession(t)
	carol.AccountID = 300
	carol.AuthLevel = telnet.AuthPlayer
	carol.CharacterID = 3
	carol.CharacterName = "Carol"
	carol.CurrentRoomID = 1
	sessions.Bind(carol.AccountID, carol)

	cat := loadTestSocials(t)
	smile := findSocialCmd(t, NewSocials(cat, sessions, nil), "smile")

	runCmd(t, smile, alice, "Bob")

	if !strings.Contains(aOut.String(), "You smile at Bob.") {
		t.Fatalf("alice self: %q", aOut.String())
	}
	if !strings.Contains(bOut.String(), "Alice smiles at you.") {
		t.Fatalf("bob view: %q", bOut.String())
	}
	if !strings.Contains(cOut.String(), "Alice smiles at Bob.") {
		t.Fatalf("carol other: %q", cOut.String())
	}
	_ = bob
}

func TestSocial_TargetNotInRoom(t *testing.T) {
	sessions, alice, bob, aOut, bOut := commPair(t)
	bob.SetInWorld(bob.CharacterID, bob.CharacterName, 99) // move bob elsewhere
	cat := loadTestSocials(t)
	smile := findSocialCmd(t, NewSocials(cat, sessions, nil), "smile")

	runCmd(t, smile, alice, "Bob")

	// MatchPlayer room-scopes its candidate set, so an out-of-room
	// target is reported as "not here" using the same anti-enumeration
	// wording as a totally unknown name. This mirrors `attack`'s
	// resolver and prevents a player from probing whether someone is
	// online in a different room.
	if !strings.Contains(aOut.String(), "No one by that name is here.") {
		t.Fatalf("alice: %q", aOut.String())
	}
	if strings.Contains(bOut.String(), "Alice smiles") {
		t.Fatalf("bob should not have received: %q", bOut.String())
	}
}

func TestSocial_TargetUnknown(t *testing.T) {
	sessions, alice, _, aOut, _ := commPair(t)
	cat := loadTestSocials(t)
	smile := findSocialCmd(t, NewSocials(cat, sessions, nil), "smile")

	runCmd(t, smile, alice, "Ghost")

	if !strings.Contains(aOut.String(), "No one by that name is here.") {
		t.Fatalf("alice: %q", aOut.String())
	}
}

func TestSocial_UntargetedRejectsTarget(t *testing.T) {
	sessions, alice, _, aOut, bOut := commPair(t)
	cat := loadTestSocials(t)
	sigh := findSocialCmd(t, NewSocials(cat, sessions, nil), "sigh")

	runCmd(t, sigh, alice, "Bob")

	if !strings.Contains(aOut.String(), "You can't sigh at someone.") {
		t.Fatalf("alice: %q", aOut.String())
	}
	if strings.Contains(bOut.String(), "Alice sighs") {
		t.Fatalf("bob should not have heard: %q", bOut.String())
	}
}

func TestSocial_WizinvisHidesActor(t *testing.T) {
	sessions, alice, _, _, bOut := commPair(t)
	// alice is admin and wizinvis; bob is a normal player who should not see her.
	alice.AuthLevel = telnet.AuthAdmin
	alice.SetHidden(true)
	cat := loadTestSocials(t)
	smile := findSocialCmd(t, NewSocials(cat, sessions, nil), "smile")

	runCmd(t, smile, alice, "")

	if strings.Contains(bOut.String(), "Alice smiles") {
		t.Fatalf("non-admin bob saw wizinvis admin: %q", bOut.String())
	}
}

func TestSocial_TargetSelfFallsBackToUntargeted(t *testing.T) {
	sessions, alice, _, aOut, bOut := commPair(t)
	cat := loadTestSocials(t)
	smile := findSocialCmd(t, NewSocials(cat, sessions, nil), "smile")

	runCmd(t, smile, alice, "Alice")

	if !strings.Contains(aOut.String(), "You smile.") {
		t.Fatalf("alice self: %q", aOut.String())
	}
	if !strings.Contains(bOut.String(), "Alice smiles.") {
		t.Fatalf("bob other: %q", bOut.String())
	}
}

func TestSocial_SilentRoomStillBroadcasts(t *testing.T) {
	// Silent room flag is text-only; physical socials carry through.
	// We don't need a RoomRepo here because socials don't consult it —
	// pin the behavior so a future refactor that adds a Silent check
	// has to update the test deliberately.
	sessions, alice, _, _, bOut := commPair(t)
	cat := loadTestSocials(t)
	smile := findSocialCmd(t, NewSocials(cat, sessions, nil), "smile")

	runCmd(t, smile, alice, "")

	if !strings.Contains(bOut.String(), "Alice smiles.") {
		t.Fatalf("bob should have seen smile despite hypothetical silence: %q", bOut.String())
	}
}

func TestSocial_HelpFieldPropagates(t *testing.T) {
	cat := loadTestSocials(t)
	cmds := NewSocials(cat, nil, nil)
	smile := findSocialCmd(t, cmds, "smile")
	if !strings.Contains(smile.Help, "small, warm smile") {
		t.Fatalf("Help did not propagate: %q", smile.Help)
	}
	sigh := findSocialCmd(t, cmds, "sigh")
	// Untargeted-only social must not have a completer (would be misleading).
	if sigh.Completer != nil {
		t.Fatalf("sigh (untargeted-only) should have no completer")
	}
	// Targeted social must have a completer.
	if smile.Completer == nil {
		t.Fatalf("smile should have a completer")
	}
}

func TestSocial_NilCatalogReturnsNil(t *testing.T) {
	if got := NewSocials(nil, nil, nil); got != nil {
		t.Fatalf("nil catalog must yield nil slice, got %v", got)
	}
}

func TestSocial_TargetsMobInRoom(t *testing.T) {
	// Mob match has precedence over player match (mirrors `attack`),
	// and renders the actor self-line plus the target_other broadcast
	// to bystanders. The mob has no session, so there is no view line
	// to deliver.
	sessions, alice, _, aOut, bOut := commPair(t)
	mobs := repo.NewMemoryMobInstanceRepo()
	mob, err := mobs.Create(context.Background(), creature.MobInstance{
		TemplateID: 1,
		Core: creature.Core{
			Name:          "scout",
			HPCurrent:     10,
			HPMax:         10,
			CurrentRoomID: 1,
		},
	})
	if err != nil {
		t.Fatalf("seed mob: %v", err)
	}
	if err := mobs.UpdateRoom(context.Background(), mob.ID, 1); err != nil {
		t.Fatalf("set mob room: %v", err)
	}
	cat := loadTestSocials(t)
	smile := findSocialCmd(t, NewSocials(cat, sessions, mobs), "smile")

	runCmd(t, smile, alice, "scout")

	if !strings.Contains(aOut.String(), "You smile at scout.") {
		t.Fatalf("alice self: %q", aOut.String())
	}
	if !strings.Contains(bOut.String(), "Alice smiles at scout.") {
		t.Fatalf("bob bystander: %q", bOut.String())
	}
}

func TestSocial_MobTargetTakesPrecedenceOverPlayer(t *testing.T) {
	// A mob and a player share the name "Bob" in the same room. The
	// resolver must pick the mob (matches `attack`'s ordering); the
	// player Bob should see the third-party broadcast, not the
	// target_view line.
	sessions, alice, bob, _, bOut := commPair(t)
	mobs := repo.NewMemoryMobInstanceRepo()
	mob, err := mobs.Create(context.Background(), creature.MobInstance{
		TemplateID: 1,
		Core: creature.Core{
			Name:          "Bob",
			HPCurrent:     10,
			HPMax:         10,
			CurrentRoomID: 1,
		},
	})
	if err != nil {
		t.Fatalf("seed mob: %v", err)
	}
	if err := mobs.UpdateRoom(context.Background(), mob.ID, 1); err != nil {
		t.Fatalf("set mob room: %v", err)
	}
	cat := loadTestSocials(t)
	smile := findSocialCmd(t, NewSocials(cat, sessions, mobs), "smile")

	runCmd(t, smile, alice, "Bob")

	// Player Bob receives target_other (the mob is the target), NOT
	// target_view (which says "smiles at you").
	if strings.Contains(bOut.String(), "smiles at you.") {
		t.Fatalf("player bob mistakenly received view line: %q", bOut.String())
	}
	if !strings.Contains(bOut.String(), "Alice smiles at Bob.") {
		t.Fatalf("player bob missing bystander line: %q", bOut.String())
	}
	_ = bob
}

func TestSocial_DefangsCfmtActorName(t *testing.T) {
	// A name containing cfmt control tokens must not be able to close
	// the outer `{{...}}::magenta` wrapper or inject a competing
	// style. chargen constrains names tightly today; this test pins
	// the belt-and-suspenders guarantee so a future name producer
	// (imports, admin rename) can't quietly regress it.
	sessions, alice, _, _, bOut := commPair(t)
	alice.CharacterName = "Foo}}::red {{evil"
	cat := loadTestSocials(t)
	smile := findSocialCmd(t, NewSocials(cat, sessions, nil), "smile")

	runCmd(t, smile, alice, "")

	if strings.Contains(bOut.String(), "::red") {
		t.Fatalf("cfmt actor-name injection not defused: %q", bOut.String())
	}
	if !strings.Contains(bOut.String(), "\x1b[35m") {
		t.Fatalf("magenta style missing: %q", bOut.String())
	}
}
