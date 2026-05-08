package trigger

import (
	"testing"

	"github.com/Jasrags/WheelMUD/internal/repo"
)

func TestRegistry_ReplaceAndForOwnerEvent(t *testing.T) {
	reg := NewRegistry()
	reg.Replace([]repo.Trigger{
		{ID: 1, OwnerKind: OwnerMobTemplate, OwnerID: 7, Event: EventOnEnter, Action: "say", Priority: 1},
		{ID: 2, OwnerKind: OwnerMobTemplate, OwnerID: 7, Event: EventOnEnter, Action: "noop", Priority: 5},
		{ID: 3, OwnerKind: OwnerRoom, OwnerID: 100, Event: EventOnSay, Action: "emote"},
	})
	got := reg.ForOwnerEvent(OwnerMobTemplate, 7, EventOnEnter)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Priority != 5 || got[1].Priority != 1 {
		t.Fatalf("priority order: %+v", got)
	}
	if rs := reg.ForOwnerEvent(OwnerRoom, 100, EventOnSay); len(rs) != 1 {
		t.Fatalf("room owner missing: %+v", rs)
	}
	if rs := reg.ForOwnerEvent(OwnerRoom, 999, EventOnEnter); len(rs) != 0 {
		t.Fatalf("unknown owner returned %+v", rs)
	}
}

func TestRegistry_HasOwnerKindEvent(t *testing.T) {
	reg := NewRegistry()
	reg.Replace([]repo.Trigger{
		{ID: 1, OwnerKind: OwnerMobTemplate, OwnerID: 7, Event: EventOnEnter, Action: "noop"},
	})
	if !reg.HasOwnerKindEvent(OwnerMobTemplate, EventOnEnter) {
		t.Fatal("expected true for mob+on_enter")
	}
	if reg.HasOwnerKindEvent(OwnerMobTemplate, EventOnSay) {
		t.Fatal("on_say should be empty")
	}
	if reg.HasOwnerKindEvent(OwnerRoom, EventOnEnter) {
		t.Fatal("room kind should be empty")
	}
}

func TestRegistry_AllByEvent(t *testing.T) {
	reg := NewRegistry()
	reg.Replace([]repo.Trigger{
		{ID: 1, OwnerKind: OwnerRoom, OwnerID: 1, Event: EventOnTick, Action: "noop", Priority: 1},
		{ID: 2, OwnerKind: OwnerRoom, OwnerID: 2, Event: EventOnTick, Action: "noop", Priority: 5},
		{ID: 3, OwnerKind: OwnerRoom, OwnerID: 3, Event: EventOnEnter, Action: "noop"},
	})
	got := reg.AllByEvent(EventOnTick)
	if len(got) != 2 {
		t.Fatalf("AllByEvent on_tick len = %d", len(got))
	}
	if got[0].Priority != 5 {
		t.Fatalf("AllByEvent priority: %+v", got)
	}
}

func TestRegistry_ReplaceSkipsInvalid(t *testing.T) {
	reg := NewRegistry()
	reg.Replace([]repo.Trigger{
		{ID: 1, OwnerKind: "item", OwnerID: 1, Event: EventOnEnter, Action: "noop"},
		{ID: 2, OwnerKind: OwnerRoom, OwnerID: 1, Event: "on_lol", Action: "noop"},
		{ID: 3, OwnerKind: OwnerRoom, OwnerID: 1, Event: EventOnEnter, Action: "noop"},
	})
	if got := reg.ForOwnerEvent(OwnerRoom, 1, EventOnEnter); len(got) != 1 {
		t.Fatalf("invalid rows leaked: %+v", got)
	}
}

func TestMatchSay(t *testing.T) {
	cases := []struct {
		match string
		text  string
		want  bool
	}{
		{"", "anything", true},
		{"rumor", "I have a Rumor for you", true},
		{"news", "rumor of trolloks", false},
		{"WORD", "the word is given", true},
	}
	for _, c := range cases {
		got := MatchSay(repo.Trigger{Match: c.match}, c.text)
		if got != c.want {
			t.Errorf("MatchSay(%q,%q) = %v, want %v", c.match, c.text, got, c.want)
		}
	}
}
