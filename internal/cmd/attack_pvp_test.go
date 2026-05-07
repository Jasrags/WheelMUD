package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/combat"
	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/eventbus"
	"github.com/Jasrags/WheelMUD/internal/group"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// attackPvPFixture stages a room (optionally nopvp), commPair-aligned
// Alice (ID=1) and Bob (ID=2) characters with the supplied opt-in
// state and level (single-class Armsman so characterLevel sums
// cleanly), and a Manager wired to the in-memory repos.
type attackPvPFixture struct {
	mgr        *combat.Manager
	rooms      *repo.MemoryRoomRepo
	mobs       *repo.MemoryMobInstanceRepo
	characters *repo.MemoryCharacterRepo
}

type pvpSide struct {
	name  string
	level int8
	pvpOn bool
}

func newAttackPvPFixture(t *testing.T, nopvp bool, alice, bob pvpSide) attackPvPFixture {
	t.Helper()
	rooms := repo.NewMemoryRoomRepo()
	rooms.Insert(repo.Room{
		ID:    1,
		Name:  "Test Room",
		Flags: repo.RoomFlags{NoPVP: nopvp},
	})
	mobs := repo.NewMemoryMobInstanceRepo()
	chars := repo.NewMemoryCharacterRepo()
	for _, side := range []pvpSide{alice, bob} {
		ch := repo.Character{
			Name:          side.name,
			CurrentRoomID: 1,
			ClassLevels:   map[creature.Class]int8{creature.ClassArmsman: side.level},
			PvP:           side.pvpOn,
			Core: creature.Core{
				HPCurrent: 30, HPMax: 30, Defense: 12, BAB: 1,
			},
		}
		if _, err := chars.Create(context.Background(), ch); err != nil {
			t.Fatalf("seed %s: %v", side.name, err)
		}
	}
	mgr := combat.New(eventbus.New(), chars, mobs, repo.NewMemoryMobTemplateRepo(), repo.NewMemoryItemRepo())
	return attackPvPFixture{mgr: mgr, rooms: rooms, mobs: mobs, characters: chars}
}

func runAttackOnPlayer(t *testing.T, fx attackPvPFixture, target string) (aOut, bOut string, fightStarted bool) {
	t.Helper()
	sessions, alice, _, aOutBuf, bOutBuf := commPair(t)
	c := NewAttack(fx.mgr, fx.rooms, fx.mobs, fx.characters, sessions, nil)
	runCmd(t, c, alice, target)
	return aOutBuf.String(), bOutBuf.String(), fx.mgr.Active(alice.CurrentRoomID)
}

func TestAttackPvP_BothOptedInLevelOK_StartsFight(t *testing.T) {
	fx := newAttackPvPFixture(t, false,
		pvpSide{"Alice", 10, true},
		pvpSide{"Bob", 10, true})
	a, b, fight := runAttackOnPlayer(t, fx, "bob")
	if !fight {
		t.Fatalf("expected fight to start; alice=%q", a)
	}
	if !strings.Contains(a, "ready an attack against Bob") {
		t.Fatalf("missing self echo: %q", a)
	}
	// Defender-side reverse broadcast: Bob receives a second-person red
	// line and must NOT see the third-person room narration (that's for
	// uninvolved bystanders only).
	if !strings.Contains(b, "Alice readies an attack against you!") {
		t.Fatalf("missing defender second-person line: %q", b)
	}
	if strings.Contains(b, "moves to attack") {
		t.Fatalf("defender should not see third-person bystander line: %q", b)
	}
	got, ok := fx.mgr.PendingAction(1, ActorRefForCharacter(1))
	if !ok {
		t.Fatal("queued action missing")
	}
	if got.Target.Kind != combat.ActorKindCharacter || got.Target.ID != 2 {
		t.Fatalf("target = %+v, want char ID=2", got.Target)
	}
}

// TestAttackPvP_BystanderSeesThirdPerson confirms the room narration
// still reaches uninvolved peers — only attacker + defender are
// excluded. Adds a third "Carol" session bound into the same room.
func TestAttackPvP_BystanderSeesThirdPerson(t *testing.T) {
	fx := newAttackPvPFixture(t, false,
		pvpSide{"Alice", 10, true},
		pvpSide{"Bob", 10, true})
	sessions, alice, _, _, bOutBuf := commPair(t)

	carolSess, carolBuf := bufSession(t)
	carolSess.AccountID = 300
	carolSess.AuthLevel = telnet.AuthPlayer
	carolSess.CharacterID = 99
	carolSess.CharacterName = "Carol"
	carolSess.CurrentRoomID = 1
	sessions.Bind(carolSess.AccountID, carolSess)

	c := NewAttack(fx.mgr, fx.rooms, fx.mobs, fx.characters, sessions, nil)
	runCmd(t, c, alice, "bob")

	got := carolBuf.String()
	if !strings.Contains(got, "Alice moves to attack Bob") {
		t.Fatalf("bystander missing third-person line: %q", got)
	}
	if strings.Contains(got, "readies an attack against you") {
		t.Fatalf("bystander should not see defender second-person line: %q", got)
	}
	// Sanity: defender path still works alongside.
	if !strings.Contains(bOutBuf.String(), "readies an attack against you") {
		t.Fatalf("defender second-person line missing: %q", bOutBuf.String())
	}
}

func TestAttackPvP_NoPVPRoom_Refuses(t *testing.T) {
	fx := newAttackPvPFixture(t, true,
		pvpSide{"Alice", 10, true},
		pvpSide{"Bob", 10, true})
	a, _, fight := runAttackOnPlayer(t, fx, "bob")
	if fight {
		t.Fatal("nopvp room must not start a fight")
	}
	if !strings.Contains(a, "sanctified") {
		t.Fatalf("missing nopvp refusal: %q", a)
	}
}

func TestAttackPvP_AttackerNewbie_Refuses(t *testing.T) {
	fx := newAttackPvPFixture(t, false,
		pvpSide{"Alice", 5, true},
		pvpSide{"Bob", 10, true})
	a, _, fight := runAttackOnPlayer(t, fx, "bob")
	if fight {
		t.Fatal("newbie attacker must not start a fight")
	}
	if !strings.Contains(a, "too green for the killing fields") {
		t.Fatalf("missing attacker-newbie refusal: %q", a)
	}
}

func TestAttackPvP_TargetNewbie_Refuses(t *testing.T) {
	fx := newAttackPvPFixture(t, false,
		pvpSide{"Alice", 10, true},
		pvpSide{"Bob", 5, true})
	a, _, fight := runAttackOnPlayer(t, fx, "bob")
	if fight {
		t.Fatal("newbie target must not start a fight")
	}
	if !strings.Contains(a, "Bob is too green") {
		t.Fatalf("missing target-newbie refusal: %q", a)
	}
}

func TestAttackPvP_AttackerOptedOut_Refuses(t *testing.T) {
	fx := newAttackPvPFixture(t, false,
		pvpSide{"Alice", 10, false},
		pvpSide{"Bob", 10, true})
	a, _, fight := runAttackOnPlayer(t, fx, "bob")
	if fight {
		t.Fatal("attacker not opted in must not start a fight")
	}
	if !strings.Contains(a, "haven't enabled PvP") {
		t.Fatalf("missing attacker-opt refusal: %q", a)
	}
}

func TestAttackPvP_TargetOptedOut_Refuses(t *testing.T) {
	fx := newAttackPvPFixture(t, false,
		pvpSide{"Alice", 10, true},
		pvpSide{"Bob", 10, false})
	a, _, fight := runAttackOnPlayer(t, fx, "bob")
	if fight {
		t.Fatal("target not opted in must not start a fight")
	}
	if !strings.Contains(a, "Bob hasn't enabled PvP") {
		t.Fatalf("missing target-opt refusal: %q", a)
	}
}

func TestAttackPvP_RoomLookupFailureFailsClosed(t *testing.T) {
	// Drop the room row entirely so rooms.FindByID errors on the
	// player-target branch. The PvP guard must refuse (fail closed)
	// rather than silently bypass NoPVP enforcement.
	fx := newAttackPvPFixture(t, false,
		pvpSide{"Alice", 10, true},
		pvpSide{"Bob", 10, true})
	fx.rooms = repo.NewMemoryRoomRepo() // empty
	a, _, fight := runAttackOnPlayer(t, fx, "bob")
	if fight {
		t.Fatal("missing room must not start a PvP fight")
	}
	if !strings.Contains(a, "sanctified") {
		t.Fatalf("expected fail-closed sanctified refusal: %q", a)
	}
}

// TestAttackPvP_OrdinalDisambiguates verifies a player can pick the
// second matching peer via `attack 2.<keyword>` when two peers in the
// same room share a name prefix. Slice-2 of #21.
func TestAttackPvP_OrdinalDisambiguates(t *testing.T) {
	rooms := repo.NewMemoryRoomRepo()
	rooms.Insert(repo.Room{ID: 1, Name: "Test Room"})
	mobs := repo.NewMemoryMobInstanceRepo()
	chars := repo.NewMemoryCharacterRepo()
	for _, name := range []string{"Alice", "Jason", "Jasmine"} {
		ch := repo.Character{
			Name:          name,
			CurrentRoomID: 1,
			ClassLevels:   map[creature.Class]int8{creature.ClassArmsman: 10},
			PvP:           true,
			Core:          creature.Core{HPCurrent: 30, HPMax: 30, Defense: 12, BAB: 1},
		}
		if _, err := chars.Create(context.Background(), ch); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	mgr := combat.New(eventbus.New(), chars, mobs, repo.NewMemoryMobTemplateRepo(), repo.NewMemoryItemRepo())

	sessions, alice, _, aOutBuf, _ := commPair(t)

	// Memory repo auto-increments IDs in seed order: Alice=1,
	// Jason=2, Jasmine=3. Sessions reuse those IDs so the
	// PvP-guard repo lookup resolves the correct row.
	jasonSess, _ := bufSession(t)
	jasonSess.AccountID = 200
	jasonSess.AuthLevel = telnet.AuthPlayer
	jasonSess.CharacterID = 2
	jasonSess.CharacterName = "Jason"
	jasonSess.CurrentRoomID = 1
	sessions.Bind(jasonSess.AccountID, jasonSess)

	jasmineSess, _ := bufSession(t)
	jasmineSess.AccountID = 201
	jasmineSess.AuthLevel = telnet.AuthPlayer
	jasmineSess.CharacterID = 3
	jasmineSess.CharacterName = "Jasmine"
	jasmineSess.CurrentRoomID = 1
	sessions.Bind(jasmineSess.AccountID, jasmineSess)

	c := NewAttack(mgr, rooms, mobs, chars, sessions, nil)

	// Bare "jas" — first hit by CharacterID ascending is Jason (id 2).
	runCmd(t, c, alice, "jas")
	if got := aOutBuf.String(); !strings.Contains(got, "ready an attack against Jason") {
		t.Fatalf("expected Jason as first hit; got %q", got)
	}

	// "2.jas" — second hit is Jasmine (id 3).
	aOutBuf.Reset()
	runCmd(t, c, alice, "2.jas")
	if got := aOutBuf.String(); !strings.Contains(got, "ready an attack against Jasmine") {
		t.Fatalf("expected Jasmine on 2.jas; got %q", got)
	}

	// "3.jas" — no third match.
	aOutBuf.Reset()
	runCmd(t, c, alice, "3.jas")
	if got := aOutBuf.String(); !strings.Contains(got, "don't see them here") {
		t.Fatalf("expected miss on 3.jas; got %q", got)
	}
}

// TestAttackPvP_OrdinalSkipsSelf — the actor's own session must be
// filtered out of the ordinal count, so when a character named Jason
// types "attack jason" they hit the *peer* Jason, not themselves.
func TestAttackPvP_OrdinalSkipsSelf(t *testing.T) {
	rooms := repo.NewMemoryRoomRepo()
	rooms.Insert(repo.Room{ID: 1, Name: "Test Room"})
	mobs := repo.NewMemoryMobInstanceRepo()
	chars := repo.NewMemoryCharacterRepo()
	// Actor is "Alice" (commPair default) but we want to verify
	// self-filter; rename via session name + seed peer "Alice" too.
	for _, name := range []string{"Alice", "Bob", "Jason"} {
		ch := repo.Character{
			Name:          name,
			CurrentRoomID: 1,
			ClassLevels:   map[creature.Class]int8{creature.ClassArmsman: 10},
			PvP:           true,
			Core:          creature.Core{HPCurrent: 30, HPMax: 30, Defense: 12, BAB: 1},
		}
		if _, err := chars.Create(context.Background(), ch); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	mgr := combat.New(eventbus.New(), chars, mobs, repo.NewMemoryMobTemplateRepo(), repo.NewMemoryItemRepo())

	// commPair gives us Alice (CharacterID=1) as the actor.
	sessions, alice, _, aOutBuf, _ := commPair(t)
	// Override alice's name to "Jason" so attack jason would match self
	// without the self-filter.
	alice.CharacterName = "Jason"

	// Seed order Alice=1, Bob=2, Jason=3 — peer session reuses id 3.
	peerSess, _ := bufSession(t)
	peerSess.AccountID = 300
	peerSess.AuthLevel = telnet.AuthPlayer
	peerSess.CharacterID = 3
	peerSess.CharacterName = "Jason"
	peerSess.CurrentRoomID = 1
	sessions.Bind(peerSess.AccountID, peerSess)

	c := NewAttack(mgr, rooms, mobs, chars, sessions, nil)

	// alice (the actor) is named Jason but should be skipped; the
	// peer Jason resolves instead. The seeded character used by the
	// PvP guard is looked up by the actor's CharacterName ("Jason"),
	// which is also valid since we seeded one.
	runCmd(t, c, alice, "jason")
	if got := aOutBuf.String(); !strings.Contains(got, "ready an attack against Jason") {
		t.Fatalf("expected peer Jason resolved; got %q", got)
	}

	// "2.jason" — only one non-self match exists, so this misses.
	aOutBuf.Reset()
	runCmd(t, c, alice, "2.jason")
	if got := aOutBuf.String(); !strings.Contains(got, "don't see them here") {
		t.Fatalf("expected miss on 2.jason; got %q", got)
	}
}

// TestAttackPvP_OrdinalRespectsRoomFilter — peers in another room must
// not occupy ordinal slots, otherwise `2.jas` would shift whenever a
// peer walks out.
func TestAttackPvP_OrdinalRespectsRoomFilter(t *testing.T) {
	rooms := repo.NewMemoryRoomRepo()
	rooms.Insert(repo.Room{ID: 1, Name: "Test Room"})
	rooms.Insert(repo.Room{ID: 2, Name: "Other Room"})
	mobs := repo.NewMemoryMobInstanceRepo()
	chars := repo.NewMemoryCharacterRepo()
	for _, name := range []string{"Alice", "Jason", "Jasmine"} {
		ch := repo.Character{
			Name:          name,
			CurrentRoomID: 1,
			ClassLevels:   map[creature.Class]int8{creature.ClassArmsman: 10},
			PvP:           true,
			Core:          creature.Core{HPCurrent: 30, HPMax: 30, Defense: 12, BAB: 1},
		}
		if _, err := chars.Create(context.Background(), ch); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	mgr := combat.New(eventbus.New(), chars, mobs, repo.NewMemoryMobTemplateRepo(), repo.NewMemoryItemRepo())

	sessions, alice, _, aOutBuf, _ := commPair(t)

	// Seed order Alice=1, Jason=2, Jasmine=3.
	jasonSess, _ := bufSession(t)
	jasonSess.AccountID = 200
	jasonSess.AuthLevel = telnet.AuthPlayer
	jasonSess.CharacterID = 2
	jasonSess.CharacterName = "Jason"
	jasonSess.CurrentRoomID = 1
	sessions.Bind(jasonSess.AccountID, jasonSess)

	// Jasmine in room 2 — out of scope.
	jasmineSess, _ := bufSession(t)
	jasmineSess.AccountID = 201
	jasmineSess.AuthLevel = telnet.AuthPlayer
	jasmineSess.CharacterID = 3
	jasmineSess.CharacterName = "Jasmine"
	jasmineSess.CurrentRoomID = 2
	sessions.Bind(jasmineSess.AccountID, jasmineSess)

	c := NewAttack(mgr, rooms, mobs, chars, sessions, nil)

	// Only Jason is in the room — "2.jas" must miss, not pick Jasmine.
	runCmd(t, c, alice, "2.jas")
	if got := aOutBuf.String(); !strings.Contains(got, "don't see them here") {
		t.Fatalf("expected miss on 2.jas (other peer in different room); got %q", got)
	}
}

// TestMatchPlayer_SkipsHiddenForNonAdmin verifies the wizinvis gate
// is preserved in MatchPlayer: a hidden peer is invisible to a
// non-admin actor's ordinal scan but visible to an AuthAdmin actor.
func TestMatchPlayer_SkipsHiddenForNonAdmin(t *testing.T) {
	sessions := session.NewRegistry()

	hidden, _ := bufSession(t)
	hidden.AccountID = 200
	hidden.AuthLevel = telnet.AuthAdmin
	hidden.CharacterID = 10
	hidden.CharacterName = "Jasper"
	hidden.CurrentRoomID = 1
	hidden.SetHidden(true)
	sessions.Bind(hidden.AccountID, hidden)

	// Non-admin observer — must miss.
	player, _ := bufSession(t)
	player.AccountID = 1
	player.AuthLevel = telnet.AuthPlayer
	player.CharacterID = 1
	player.CharacterName = "Alice"
	player.CurrentRoomID = 1
	sessions.Bind(player.AccountID, player)

	if got, ok := MatchPlayer("jas", sessions, player); ok || got != nil {
		t.Fatalf("non-admin must not see wizinvis peer; got=%v ok=%v", got, ok)
	}

	// Admin observer — must hit.
	admin, _ := bufSession(t)
	admin.AccountID = 2
	admin.AuthLevel = telnet.AuthAdmin
	admin.CharacterID = 2
	admin.CharacterName = "Imrahil"
	admin.CurrentRoomID = 1
	sessions.Bind(admin.AccountID, admin)

	got, ok := MatchPlayer("jas", sessions, admin)
	if !ok || got != hidden {
		t.Fatalf("admin must see wizinvis peer; got=%v ok=%v", got, ok)
	}
}

// TestMatchPlayer_DeterministicOrder pounds on the matcher with a
// registry whose IDs are bound out of insertion order, asserting that
// `2.jas` always returns the same peer despite map-iteration randomness.
func TestMatchPlayer_DeterministicOrder(t *testing.T) {
	sessions := session.NewRegistry()
	self, _ := bufSession(t)
	self.AccountID = 1
	self.CharacterID = 1
	self.CharacterName = "Alice"
	self.CurrentRoomID = 1
	sessions.Bind(self.AccountID, self)

	// Bind Jasmine first (id 20) and Jason second (id 10) — so
	// insertion order disagrees with id order.
	jasmine, _ := bufSession(t)
	jasmine.AccountID = 201
	jasmine.CharacterID = 20
	jasmine.CharacterName = "Jasmine"
	jasmine.CurrentRoomID = 1
	sessions.Bind(jasmine.AccountID, jasmine)

	jason, _ := bufSession(t)
	jason.AccountID = 200
	jason.CharacterID = 10
	jason.CharacterName = "Jason"
	jason.CurrentRoomID = 1
	sessions.Bind(jason.AccountID, jason)

	for i := 0; i < 1000; i++ {
		got, ok := MatchPlayer("2.jas", sessions, self)
		if !ok || got != jasmine {
			t.Fatalf("iter %d: MatchPlayer 2.jas = %v ok=%v, want Jasmine", i, got, ok)
		}
		got, ok = MatchPlayer("jas", sessions, self)
		if !ok || got != jason {
			t.Fatalf("iter %d: MatchPlayer jas = %v ok=%v, want Jason (lowest id)", i, got, ok)
		}
	}
}

// TestAttackPvP_SameGroupRefused — Phase D #22 slice 2. Two opted-
// in, level-OK characters who share a party must refuse with the
// comrade line.
func TestAttackPvP_SameGroupRefused(t *testing.T) {
	fx := newAttackPvPFixture(t, false,
		pvpSide{"Alice", 10, true},
		pvpSide{"Bob", 10, true})
	sessions, alice, bob, aOutBuf, _ := commPair(t)

	groups := group.New()
	if err := groups.Invite(alice.CharacterID, alice.CharacterName, bob.CharacterID, bob.CharacterName); err != nil {
		t.Fatalf("Invite: %v", err)
	}
	if _, err := groups.Accept(bob.CharacterID, bob.CharacterName); err != nil {
		t.Fatalf("Accept: %v", err)
	}

	c := NewAttack(fx.mgr, fx.rooms, fx.mobs, fx.characters, sessions, groups)
	runCmd(t, c, alice, "bob")

	if got := aOutBuf.String(); !strings.Contains(got, "Bob is a comrade") {
		t.Fatalf("missing comrade refusal: %q", got)
	}
	if fx.mgr.Active(alice.CurrentRoomID) {
		t.Fatal("same-group attack must not start a fight")
	}
}

// TestAttackPvP_DifferentGroupsAllowed — positive control.
func TestAttackPvP_DifferentGroupsAllowed(t *testing.T) {
	fx := newAttackPvPFixture(t, false,
		pvpSide{"Alice", 10, true},
		pvpSide{"Bob", 10, true})
	sessions, alice, _, _, _ := commPair(t)

	groups := group.New()
	// Alice in solo group; Bob ungrouped — they aren't co-grouped.
	if err := groups.Invite(alice.CharacterID, alice.CharacterName, 999, "Phantom"); err != nil {
		t.Fatalf("Invite phantom: %v", err)
	}

	c := NewAttack(fx.mgr, fx.rooms, fx.mobs, fx.characters, sessions, groups)
	runCmd(t, c, alice, "bob")

	if !fx.mgr.Active(alice.CurrentRoomID) {
		t.Fatal("ungrouped pair must be able to fight")
	}
}

// TestAttackPvP_NilGroupManagerNoOp — backwards compat: nil
// groups arg behaves identically to slice-1.
func TestAttackPvP_NilGroupManagerNoOp(t *testing.T) {
	fx := newAttackPvPFixture(t, false,
		pvpSide{"Alice", 10, true},
		pvpSide{"Bob", 10, true})
	sessions, alice, _, _, _ := commPair(t)

	c := NewAttack(fx.mgr, fx.rooms, fx.mobs, fx.characters, sessions, nil)
	runCmd(t, c, alice, "bob")
	if !fx.mgr.Active(alice.CurrentRoomID) {
		t.Fatal("nil groups must not block a valid PvP attack")
	}
}

func TestCharacterLevel_SumsClassLevels(t *testing.T) {
	ch := repo.Character{
		ClassLevels: map[creature.Class]int8{
			creature.ClassArmsman:  3,
			creature.ClassWoodsman: 2,
		},
	}
	if got := characterLevel(ch); got != 5 {
		t.Fatalf("characterLevel = %d, want 5", got)
	}
	if got := characterLevel(repo.Character{}); got != 0 {
		t.Fatalf("empty characterLevel = %d, want 0", got)
	}
}
