package quest

import (
	"errors"
	"testing"
)

func goodCatalog() *Catalog {
	return &Catalog{
		ByID: map[string]*Quest{
			"lost_lamb": {
				ID:       "lost_lamb",
				Name:     "The Lost Lamb",
				Summary:  "A village elder's lamb wandered off.",
				GiverMob: "tr.elder",
				Steps: []Step{
					{Kind: StepReachRoom, Prompt: "Search the path.", Room: "tr.westwood.path_2"},
					{Kind: StepKillN, Prompt: "Drive off the wolves.", Mob: "tr.wolf", Count: 3},
					{Kind: StepTalkTo, Prompt: "Return to the elder.", Mob: "tr.elder"},
				},
				Rewards: Reward{XP: 200, Copper: 5000},
			},
		},
	}
}

func goodRefs() *RefSets {
	return &RefSets{
		Mobs:  map[string]bool{"tr.elder": true, "tr.wolf": true},
		Rooms: map[string]bool{"tr.westwood.path_2": true},
	}
}

func TestValidate_OK(t *testing.T) {
	if err := Validate(goodCatalog(), goodRefs()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_OK_WithoutRefs(t *testing.T) {
	if err := Validate(goodCatalog(), nil); err != nil {
		t.Fatalf("unexpected error without refs: %v", err)
	}
}

func TestValidate_AllowsEmptyCatalog(t *testing.T) {
	// Fresh deploys ship with zero quests until builders author one.
	if err := Validate(&Catalog{ByID: map[string]*Quest{}}, nil); err != nil {
		t.Fatalf("empty catalog should validate: %v", err)
	}
}

func TestValidate_RejectsNilCatalog(t *testing.T) {
	if err := Validate(nil, nil); err == nil {
		t.Fatal("expected error on nil catalog")
	}
}

func TestValidate_RejectCases(t *testing.T) {
	cases := []struct {
		name string
		mut  func(c *Catalog, refs *RefSets) // mutate one field on a fresh copy
	}{
		{"empty ID", func(c *Catalog, _ *RefSets) {
			c.ByID["lost_lamb"].ID = ""
		}},
		{"map key mismatch", func(c *Catalog, _ *RefSets) {
			c.ByID["lost_lamb"].ID = "different"
		}},
		{"empty name", func(c *Catalog, _ *RefSets) {
			c.ByID["lost_lamb"].Name = ""
		}},
		{"no steps", func(c *Catalog, _ *RefSets) {
			c.ByID["lost_lamb"].Steps = nil
		}},
		{"empty step prompt", func(c *Catalog, _ *RefSets) {
			c.ByID["lost_lamb"].Steps[0].Prompt = ""
		}},
		{"unknown step kind", func(c *Catalog, _ *RefSets) {
			c.ByID["lost_lamb"].Steps[0].Kind = "warp_reality"
		}},
		{"talk_to missing mob", func(c *Catalog, _ *RefSets) {
			c.ByID["lost_lamb"].Steps[2].Mob = ""
		}},
		{"kill_n missing mob", func(c *Catalog, _ *RefSets) {
			c.ByID["lost_lamb"].Steps[1].Mob = ""
		}},
		{"kill_n zero count", func(c *Catalog, _ *RefSets) {
			c.ByID["lost_lamb"].Steps[1].Count = 0
		}},
		{"kill_n negative count", func(c *Catalog, _ *RefSets) {
			c.ByID["lost_lamb"].Steps[1].Count = -3
		}},
		{"reach_room missing room", func(c *Catalog, _ *RefSets) {
			c.ByID["lost_lamb"].Steps[0].Room = ""
		}},
		{"talk_to unknown mob ref", func(c *Catalog, refs *RefSets) {
			delete(refs.Mobs, "tr.elder")
		}},
		{"kill_n unknown mob ref", func(c *Catalog, refs *RefSets) {
			delete(refs.Mobs, "tr.wolf")
		}},
		{"reach_room unknown room ref", func(c *Catalog, refs *RefSets) {
			delete(refs.Rooms, "tr.westwood.path_2")
		}},
		{"giver_mob unknown ref", func(c *Catalog, refs *RefSets) {
			c.ByID["lost_lamb"].GiverMob = "tr.ghost"
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := goodCatalog()
			refs := goodRefs()
			tc.mut(c, refs)
			err := Validate(c, refs)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, ErrInvalidCatalog) {
				t.Fatalf("expected ErrInvalidCatalog, got %v", err)
			}
		})
	}
}

func TestValidate_ScriptStep_OK(t *testing.T) {
	c := &Catalog{ByID: map[string]*Quest{
		"q": {
			ID:    "q",
			Name:  "Q",
			Steps: []Step{{Kind: StepScript, Prompt: "wait", Script: "advance_q"}},
		},
	}}
	refs := &RefSets{Scripts: map[string]bool{"advance_q": true}}
	if err := Validate(c, refs); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidate_ScriptStep_RejectsMissingScriptField(t *testing.T) {
	c := &Catalog{ByID: map[string]*Quest{
		"q": {
			ID:    "q",
			Name:  "Q",
			Steps: []Step{{Kind: StepScript, Prompt: "wait"}},
		},
	}}
	if err := Validate(c, nil); err == nil {
		t.Fatal("expected error for missing script field")
	}
}

func TestValidate_ScriptStep_RejectsUnknownScriptName(t *testing.T) {
	c := &Catalog{ByID: map[string]*Quest{
		"q": {
			ID:    "q",
			Name:  "Q",
			Steps: []Step{{Kind: StepScript, Prompt: "wait", Script: "ghost"}},
		},
	}}
	refs := &RefSets{Scripts: map[string]bool{"other": true}}
	if err := Validate(c, refs); err == nil {
		t.Fatal("expected error for unknown script name")
	}
}

func TestCatalog_Get(t *testing.T) {
	c := goodCatalog()
	if q, ok := c.Get("lost_lamb"); !ok || q == nil {
		t.Fatal("expected lost_lamb hit")
	}
	if _, ok := c.Get("ghost"); ok {
		t.Fatal("expected ghost miss")
	}
	var nilC *Catalog
	if _, ok := nilC.Get("lost_lamb"); ok {
		t.Fatal("nil catalog should miss")
	}
}
