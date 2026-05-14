package cmd

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// candidateTexts pulls Text out of a Candidate slice, sorted, so
// table-driven assertions don't depend on intra-source ordering. An
// empty input returns nil so `equalSlices(got, nil)` works.
func candidateTexts(cands []telnet.Candidate) []string {
	if len(cands) == 0 {
		return nil
	}
	out := make([]string, len(cands))
	for i, c := range cands {
		out[i] = c.Text
	}
	sort.Strings(out)
	return out
}

func TestCompleterSlot(t *testing.T) {
	cases := []struct {
		name        string
		rest        string
		wantSlot    int
		wantPartial string
	}{
		{"empty", "", 0, ""},
		{"first token typing", "swo", 0, "swo"},
		{"first token done", "sword ", 1, ""},
		{"second token typing", "sword ali", 1, "ali"},
		{"quoted blob counts as one", `"rusty sword" ali`, 1, "ali"},
		{"trailing whitespace empty partial", "sword   ", 1, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			slot, partial := completerSlot(tc.rest)
			if slot != tc.wantSlot || partial != tc.wantPartial {
				t.Fatalf("completerSlot(%q) = (%d,%q), want (%d,%q)",
					tc.rest, slot, partial, tc.wantSlot, tc.wantPartial)
			}
		})
	}
}

func TestSplitOrdinalPartial(t *testing.T) {
	cases := []struct {
		in          string
		wantPrefix  string
		wantKeyword string
	}{
		{"swo", "", "swo"},
		{"2.swo", "2.", "swo"},
		{"12.swo", "12.", "swo"},
		{"2.", "2.", ""},
		{".swo", "", ".swo"},     // leading dot, not an ordinal
		{"2a.swo", "", "2a.swo"}, // non-numeric prefix → plain keyword
		{"", "", ""},
	}
	for _, tc := range cases {
		p, k := splitOrdinalPartial(tc.in)
		if p != tc.wantPrefix || k != tc.wantKeyword {
			t.Errorf("splitOrdinalPartial(%q) = (%q,%q), want (%q,%q)",
				tc.in, p, k, tc.wantPrefix, tc.wantKeyword)
		}
	}
}

func TestItemKeywordCandidates(t *testing.T) {
	items := []repo.Item{
		{ID: 1, Name: "a rusty iron sword"},
		{ID: 2, Name: "an iron shield"},
		{ID: 3, Name: "a small pebble"},
	}

	t.Run("empty partial lists each item once", func(t *testing.T) {
		got := candidateTexts(itemKeywordCandidates(items, ""))
		want := []string{"a", "an"}
		// Each item's first token after lowercasing: "a", "an", "a" —
		// dedup leaves {"a", "an"}.
		if !equalSlices(got, want) {
			t.Fatalf("got %v want %v", got, want)
		}
	})

	t.Run("token prefix matches across items, deduped", func(t *testing.T) {
		got := candidateTexts(itemKeywordCandidates(items, "iro"))
		want := []string{"iron"}
		if !equalSlices(got, want) {
			t.Fatalf("got %v want %v", got, want)
		}
	})

	t.Run("ordinal preserved on candidate text", func(t *testing.T) {
		cands := itemKeywordCandidates(items, "2.iro")
		if len(cands) != 1 || cands[0].Text != "2.iron" {
			t.Fatalf("got %+v, want one candidate Text=%q", cands, "2.iron")
		}
	})

	t.Run("no match returns empty", func(t *testing.T) {
		if got := itemKeywordCandidates(items, "xyz"); len(got) != 0 {
			t.Fatalf("got %+v, want empty", got)
		}
	})
}

func TestMobKeywordCandidates(t *testing.T) {
	mobs := []creature.MobInstance{
		{ID: 1, Core: creature.Core{Name: "a town crier"}},
		{ID: 2, Core: creature.Core{Name: "a town guard"}},
	}
	got := candidateTexts(mobKeywordCandidates(mobs, "tow"))
	if !equalSlices(got, []string{"town"}) {
		t.Fatalf("got %v want [town]", got)
	}
}

func TestCompleteExits(t *testing.T) {
	exits := repo.NewMemoryExitRepo()
	exits.Insert(repo.Exit{FromRoomID: 1, Direction: repo.DirNorth})
	exits.Insert(repo.Exit{FromRoomID: 1, Direction: repo.DirEast})
	exits.Insert(repo.Exit{FromRoomID: 1, Direction: repo.DirNortheast})
	exits.Insert(repo.Exit{FromRoomID: 1, Direction: repo.DirUp, Flags: repo.ExitFlags{Hidden: true}})

	s, _ := bufSession(t)
	s.CurrentRoomID = 1

	completer := completeExits(exits)

	t.Run("empty partial lists every visible exit", func(t *testing.T) {
		got := candidateTexts(completer(s, ""))
		want := []string{"e", "n", "ne"}
		if !equalSlices(got, want) {
			t.Fatalf("got %v want %v", got, want)
		}
	})

	t.Run("partial filters by short-code prefix", func(t *testing.T) {
		got := candidateTexts(completer(s, "n"))
		want := []string{"n", "ne"}
		if !equalSlices(got, want) {
			t.Fatalf("got %v want %v", got, want)
		}
	})

	t.Run("hidden exits filtered out", func(t *testing.T) {
		got := candidateTexts(completer(s, "u"))
		if got != nil {
			t.Fatalf("hidden exit leaked: %v", got)
		}
	})

	t.Run("slot past zero returns nil", func(t *testing.T) {
		if got := completer(s, "n "); got != nil {
			t.Fatalf("got %v on slot 1, want nil", got)
		}
	})

	t.Run("no current room returns nil", func(t *testing.T) {
		s2, _ := bufSession(t)
		s2.CurrentRoomID = 0
		if got := completer(s2, ""); got != nil {
			t.Fatalf("got %v with no room, want nil", got)
		}
	})
}

func TestCompleteRoomItems(t *testing.T) {
	items := repo.NewMemoryItemRepo()
	items.Insert(repo.Item{ExternalID: "i1", Name: "an iron sword", RoomID: 1})
	items.Insert(repo.Item{ExternalID: "i2", Name: "an iron shield", RoomID: 1})
	items.Insert(repo.Item{ExternalID: "i3", Name: "a brass key", RoomID: 2}) // wrong room

	s, _ := bufSession(t)
	s.CurrentRoomID = 1
	s.CharacterID = 99

	got := candidateTexts(completeRoomItems(items)(s, "iro"))
	if !equalSlices(got, []string{"iron"}) {
		t.Fatalf("got %v want [iron]", got)
	}
}

func TestCompleteInventoryItems(t *testing.T) {
	items := repo.NewMemoryItemRepo()
	items.Insert(repo.Item{ExternalID: "i1", Name: "a healing potion", OwnerCharacterID: 7})
	items.Insert(repo.Item{ExternalID: "i2", Name: "a mana potion", OwnerCharacterID: 7})
	items.Insert(repo.Item{ExternalID: "i3", Name: "a shield", OwnerCharacterID: 8}) // other char

	s, _ := bufSession(t)
	s.CurrentRoomID = 1
	s.CharacterID = 7

	got := candidateTexts(completeInventoryItems(items)(s, "pot"))
	if !equalSlices(got, []string{"potion"}) {
		t.Fatalf("got %v want [potion]", got)
	}
}

func TestOnlineNameCandidates_AuthFiltersAdminPeers(t *testing.T) {
	sessions, alice, bob, _, _ := commPair(t)

	// Add a third session at AuthAdmin.
	admin, _ := bufSession(t)
	admin.AccountID = 300
	admin.AuthLevel = telnet.AuthAdmin
	admin.CharacterID = 3
	admin.CharacterName = "Adminus"
	admin.CurrentRoomID = 1
	sessions.Bind(admin.AccountID, admin)

	t.Run("player sees player peers only", func(t *testing.T) {
		got := candidateTexts(onlineNameCandidates(alice, sessions, ""))
		// Alice (self) filtered, Adminus filtered (higher auth).
		if !equalSlices(got, []string{"Bob"}) {
			t.Fatalf("got %v want [Bob]", got)
		}
	})

	t.Run("admin sees player and player peers see each other", func(t *testing.T) {
		got := candidateTexts(onlineNameCandidates(admin, sessions, ""))
		want := []string{"Alice", "Bob"}
		if !equalSlices(got, want) {
			t.Fatalf("got %v want %v", got, want)
		}
	})

	t.Run("self always filtered", func(t *testing.T) {
		got := candidateTexts(onlineNameCandidates(bob, sessions, "Bo"))
		if got != nil {
			t.Fatalf("got %v, want nil (self filtered)", got)
		}
	})

	t.Run("partial prefix is case-insensitive", func(t *testing.T) {
		got := candidateTexts(onlineNameCandidates(alice, sessions, "bo"))
		if !equalSlices(got, []string{"Bob"}) {
			t.Fatalf("got %v want [Bob]", got)
		}
	})
}

func TestRoomIDCandidates(t *testing.T) {
	rooms := repo.NewMemoryRoomRepo()
	rooms.Insert(repo.Room{ID: 1, ExternalID: "tr.emonds_field", Name: "Emond's Field"})
	rooms.Insert(repo.Room{ID: 2, ExternalID: "tr.emonds_inn", Name: "Winespring Inn"})
	rooms.Insert(repo.Room{ID: 3, ExternalID: "plaza.fountain", Name: "Plaza Fountain"})

	got := candidateTexts(roomIDCandidates(context.Background(), rooms, "tr."))
	want := []string{"tr.emonds_field", "tr.emonds_inn"}
	if !equalSlices(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}

	// Help text comes through on the un-sorted slice — re-call to inspect.
	cands := roomIDCandidates(context.Background(), rooms, "tr.emonds_f")
	if len(cands) != 1 || cands[0].Help != "Emond's Field" {
		t.Fatalf("got %+v, want one candidate with Help=%q", cands, "Emond's Field")
	}
}

func TestCompleteExamineTargets_UnionsSources(t *testing.T) {
	items := repo.NewMemoryItemRepo()
	items.Insert(repo.Item{ExternalID: "i1", Name: "an iron sword", RoomID: 1})
	items.Insert(repo.Item{ExternalID: "i2", Name: "a small key", OwnerCharacterID: 7})

	mobs := repo.NewMemoryMobInstanceRepo()
	if _, err := mobs.Create(context.Background(), creature.MobInstance{
		TemplateID: 1,
		Core:       creature.Core{Name: "a town crier", CurrentRoomID: 1},
	}); err != nil {
		t.Fatalf("seed mob: %v", err)
	}

	s, _ := bufSession(t)
	s.CurrentRoomID = 1
	s.CharacterID = 7

	completer := completeExamineTargets(items, mobs)

	t.Run("partial 'i' hits room item", func(t *testing.T) {
		got := candidateTexts(completer(s, "i"))
		// "i" matches "iron" (room item). Room mob "town crier" and
		// inventory "small key" don't share that token-prefix.
		if !equalSlices(got, []string{"iron"}) {
			t.Fatalf("got %v want [iron]", got)
		}
	})

	t.Run("partial 'k' hits inventory key", func(t *testing.T) {
		got := candidateTexts(completer(s, "k"))
		if !equalSlices(got, []string{"key"}) {
			t.Fatalf("got %v want [key]", got)
		}
	})

	t.Run("partial 'tow' hits room mob", func(t *testing.T) {
		got := candidateTexts(completer(s, "tow"))
		if !equalSlices(got, []string{"town"}) {
			t.Fatalf("got %v want [town]", got)
		}
	})

	t.Run("slot past 0 returns nil", func(t *testing.T) {
		if got := completer(s, "sword "); got != nil {
			t.Fatalf("got %v on slot 1, want nil", got)
		}
	})
}

func TestCompleteGive_SlotRouting(t *testing.T) {
	sessions, alice, _, _, _ := commPair(t)
	items := repo.NewMemoryItemRepo()
	items.Insert(repo.Item{ExternalID: "i1", Name: "an iron sword", OwnerCharacterID: alice.CharacterID})

	completer := completeGive(items, sessions)

	t.Run("slot 0 completes inventory keyword", func(t *testing.T) {
		got := candidateTexts(completer(alice, "iro"))
		if !equalSlices(got, []string{"iron"}) {
			t.Fatalf("got %v want [iron]", got)
		}
	})

	t.Run("slot 1 completes online name", func(t *testing.T) {
		got := candidateTexts(completer(alice, "iron "))
		if !equalSlices(got, []string{"Bob"}) {
			t.Fatalf("got %v want [Bob]", got)
		}
	})
}

func TestNewTell_CompleterEnumeratesPeers(t *testing.T) {
	sessions, alice, _, _, _ := commPair(t)
	tell := NewTell(sessions, nil)
	if tell.Completer == nil {
		t.Fatal("tell missing completer")
	}
	got := candidateTexts(tell.Completer(alice, "Bo"))
	if !equalSlices(got, []string{"Bob"}) {
		t.Fatalf("got %v want [Bob]", got)
	}

	// Slot 1 is message body — bell.
	if got := tell.Completer(alice, "Bob hello "); got != nil {
		t.Fatalf("got %v on slot 2, want nil", got)
	}
}

func TestCompleteTeleport_UnionsRoomsAndPeers(t *testing.T) {
	sessions, alice, _, _, _ := commPair(t)
	rooms := repo.NewMemoryRoomRepo()
	rooms.Insert(repo.Room{ID: 1, ExternalID: "tr.field", Name: "Field"})
	rooms.Insert(repo.Room{ID: 2, ExternalID: "tr.inn", Name: "Inn"})

	completer := completeTeleport(rooms, sessions)

	t.Run("slot 0 unions room ids and peer names", func(t *testing.T) {
		got := candidateTexts(completer(alice, ""))
		want := []string{"Bob", "tr.field", "tr.inn"}
		if !equalSlices(got, want) {
			t.Fatalf("got %v want %v", got, want)
		}
	})

	t.Run("slot 1 is room ids only", func(t *testing.T) {
		got := candidateTexts(completer(alice, "Bob "))
		want := []string{"tr.field", "tr.inn"}
		if !equalSlices(got, want) {
			t.Fatalf("got %v want %v", got, want)
		}
	})

	t.Run("slot 2 returns nil", func(t *testing.T) {
		if got := completer(alice, "Bob tr.inn "); got != nil {
			t.Fatalf("got %v, want nil", got)
		}
	})
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Sanity guard so future drift in the test helper is loud.
func TestCandidateTextsSorted(t *testing.T) {
	got := candidateTexts([]telnet.Candidate{{Text: "z"}, {Text: "a"}, {Text: "m"}})
	if !strings.EqualFold(got[0], "a") || !strings.EqualFold(got[2], "z") {
		t.Fatalf("not sorted: %v", got)
	}
}
