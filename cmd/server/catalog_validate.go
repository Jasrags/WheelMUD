package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Jasrags/WheelMUD/internal/dialogue"
	"github.com/Jasrags/WheelMUD/internal/effects"
	"github.com/Jasrags/WheelMUD/internal/quest"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/scripts"
	"github.com/Jasrags/WheelMUD/internal/world"
)

// buildQuestRefSets assembles the (mobs, rooms) ExternalID sets that
// quest.Validate cross-references against. Phase F #31 — runs once
// at boot after the world loader has populated both repos. A typo'd
// reference in a quest YAML fails the boot loudly before the engine
// subscribes to any events.
func buildQuestRefSets(ctx context.Context, rooms repo.RoomRepo, templates repo.MobTemplateRepo, scriptCat *scripts.Catalog) (*quest.RefSets, error) {
	allRooms, err := rooms.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("list rooms: %w", err)
	}
	mobIDs, err := templates.ListExternalIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list mob templates: %w", err)
	}
	refs := &quest.RefSets{
		Mobs:  make(map[string]bool, len(mobIDs)),
		Rooms: make(map[string]bool, len(allRooms)),
	}
	for _, id := range mobIDs {
		refs.Mobs[id] = true
	}
	for _, r := range allRooms {
		if r.ExternalID != "" {
			refs.Rooms[r.ExternalID] = true
		}
	}
	// Phase F #32 slice 2: cross-ref StepScript against the loaded
	// Lua catalog. nil scriptCat (e.g. boots that disable scripting)
	// disables the check.
	if scriptCat != nil {
		refs.Scripts = make(map[string]bool, len(scriptCat.ByName))
		for name := range scriptCat.ByName {
			refs.Scripts[name] = true
		}
	}
	return refs, nil
}

// validateDialogueScriptRefs walks every mob_template's stored
// dialogue_json and asserts that every `effects: kind: script`
// references a script the catalog knows. A missing script at
// runtime degrades gracefully (the dialogue effect logs + no-ops),
// but boot-time fail-fast keeps authoring mistakes from sitting
// silently in the world. Phase F #32 slice 2.
//
// nil scriptCat disables the check (mirrors quest.RefSets.Scripts
// nil-disables): boots that ship without scripting authored skip
// the cross-ref entirely. An empty catalog is *not* the same as nil
// — we still validate and reject any script reference.
func validateDialogueScriptRefs(ctx context.Context, templates repo.MobTemplateRepo, scriptCat *scripts.Catalog) error {
	if scriptCat == nil {
		return nil
	}
	ids, err := templates.ListExternalIDs(ctx)
	if err != nil {
		return fmt.Errorf("list mob templates: %w", err)
	}
	for _, ext := range ids {
		t, err := templates.GetByExternalID(ctx, ext)
		if err != nil {
			return fmt.Errorf("get mob template %q: %w", ext, err)
		}
		if len(t.DialogueJSON) == 0 || string(t.DialogueJSON) == "null" {
			continue
		}
		var tree dialogue.Tree
		if err := json.Unmarshal(t.DialogueJSON, &tree); err != nil {
			return fmt.Errorf("decode dialogue for mob %q: %w", ext, err)
		}
		for nodeID, node := range tree.Nodes {
			for ri, resp := range node.Responses {
				for ei, eff := range resp.Effects {
					if eff.Kind != dialogue.EffectScript {
						continue
					}
					name := eff.Args["script"]
					if _, ok := scriptCat.Get(name); !ok {
						return fmt.Errorf("mob %q dialogue node %q response[%d] effect[%d]: unknown script %q",
							ext, nodeID, ri, ei, name)
					}
				}
			}
		}
	}
	return nil
}

// validateConsumableEffectRefs walks every consumable item parsed
// from the world YAML and confirms its EffectID resolves through the
// loaded effects catalog. Zero (no effect set) is treated as authored
// intent — the potion fizzles when quaffed. Phase E #25 slice 2.
func validateConsumableEffectRefs(loaded world.LoadedWorld, eff *effects.Catalog) error {
	for zone, specs := range loaded.ItemSpecsByZone {
		for _, spec := range specs {
			if spec.Item.Type != repo.ItemTypeConsumable {
				continue
			}
			stats, ok := spec.Item.Stats.(repo.ConsumableStats)
			if !ok {
				continue
			}
			if stats.EffectID == 0 {
				continue
			}
			if _, ok := eff.IDForHash(stats.EffectID); !ok {
				return fmt.Errorf("zone %s: consumable %q references unknown effect (hash=%d)",
					zone, spec.Item.ExternalID, stats.EffectID)
			}
		}
	}
	return nil
}
