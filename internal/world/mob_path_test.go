package world

import (
	"strings"
	"testing"
)

// Phase F #32a slice 1 — validateMobPath enforces:
//   * length >= 2
//   * no duplicate entries (so currentRoomID → pathIndex is unambiguous)
//   * every entry resolves to a known room external_id
//   * each consecutive pair (incl. closed-loop wraparound) is connected
//     by a walkable exit at boot
//
// The adjacency input is the buildRoomAdjacency output — walkable-only,
// mirroring the wander tick's exitWalkable gate.

func TestValidateMobPath(t *testing.T) {
	roomIDs := map[string]int64{
		"qa.hub":  1,
		"qa.shop": 2,
		"qa.bank": 3,
		"qa.lua":  4,
	}
	// Adjacency forms a 3-room loop hub → shop → bank → hub. qa.lua
	// is intentionally orphaned so we can test the "no walkable exit"
	// case.
	adj := map[string]map[string]bool{
		"qa.hub":  {"qa.shop": true},
		"qa.shop": {"qa.bank": true},
		"qa.bank": {"qa.hub": true},
		"qa.lua":  {},
	}

	cases := []struct {
		name    string
		path    []string
		wantErr string
	}{
		{
			name:    "happy_three_room_loop",
			path:    []string{"qa.hub", "qa.shop", "qa.bank"},
			wantErr: "",
		},
		{
			name:    "length_one_rejected",
			path:    []string{"qa.hub"},
			wantErr: "at least 2 entries",
		},
		{
			name:    "empty_rejected",
			path:    nil,
			wantErr: "at least 2 entries",
		},
		{
			name:    "unknown_room_rejected",
			path:    []string{"qa.hub", "qa.does_not_exist"},
			wantErr: "is not a known room external_id",
		},
		{
			name:    "duplicate_rejected",
			path:    []string{"qa.hub", "qa.shop", "qa.hub"},
			wantErr: "duplicate room",
		},
		{
			name:    "broken_adjacency_step_rejected",
			path:    []string{"qa.hub", "qa.lua"},
			wantErr: "no walkable exit",
		},
		{
			name: "broken_loop_wraparound_rejected",
			// 2-cycle that doesn't close back: bank → hub exists but
			// hub → bank doesn't, so the closing edge fails.
			path:    []string{"qa.bank", "qa.hub"},
			wantErr: "no walkable exit",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateMobPath("test.mob", tc.path, roomIDs, adj)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("want error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %q, want it to contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestBuildRoomAdjacency_FiltersUnwalkable(t *testing.T) {
	rooms := []Room{
		{
			ID: "a",
			Exits: map[string]Exit{
				"n": {To: "b"},               // walkable
				"s": {To: "c", Closed: true}, // closed → excluded
				"e": {To: "d", Locked: true}, // locked → excluded
				"w": {To: "e", Hidden: true}, // hidden → excluded
				"u": {To: "f", NoPass: true}, // nopass → excluded
				"d": {To: ""},                // empty target → excluded
			},
		},
		{ID: "b", Exits: map[string]Exit{"s": {To: "a"}}},
	}
	adj := buildRoomAdjacency(rooms)
	if len(adj["a"]) != 1 || !adj["a"]["b"] {
		t.Fatalf("room a adjacency = %v, want {b: true}", adj["a"])
	}
	if !adj["b"]["a"] {
		t.Fatalf("room b adjacency missing reverse to a: %v", adj["b"])
	}
}
