package repo

import (
	"context"
	"errors"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/db"
)

type itemRepoFixture struct {
	items ItemRepo
	rooms RoomRepo
}

func runItemRepoTests(t *testing.T, name string, newFix func(t *testing.T) itemRepoFixture) {
	t.Helper()

	makeRoom := func(t *testing.T, fix itemRepoFixture) int64 {
		t.Helper()
		r, err := fix.rooms.Create(context.Background(), Room{ExternalID: "a", Name: "A"})
		if err != nil {
			t.Fatalf("create room: %v", err)
		}
		return r.ID
	}

	t.Run(name+"/create_and_list", func(t *testing.T) {
		fix := newFix(t)
		roomID := makeRoom(t, fix)
		ctx := context.Background()
		if _, err := fix.items.Create(ctx, Item{ExternalID: "pebble", Name: "a small pebble", RoomID: roomID}); err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, err := fix.items.ListInRoom(ctx, roomID)
		if err != nil {
			t.Fatalf("ListInRoom: %v", err)
		}
		if len(got) != 1 || got[0].Name != "a small pebble" {
			t.Fatalf("got %+v", got)
		}
		if got[0].NameLower != "a small pebble" {
			t.Fatalf("NameLower = %q", got[0].NameLower)
		}
	})

	t.Run(name+"/create_rejects_empty_external_id", func(t *testing.T) {
		fix := newFix(t)
		_, err := fix.items.Create(context.Background(), Item{Name: "ghost"})
		if !errors.Is(err, ErrInvalidExternalID) {
			t.Fatalf("err = %v, want ErrInvalidExternalID", err)
		}
	})

	t.Run(name+"/create_duplicate_external_id", func(t *testing.T) {
		fix := newFix(t)
		ctx := context.Background()
		if _, err := fix.items.Create(ctx, Item{ExternalID: "dup", Name: "a"}); err != nil {
			t.Fatalf("first Create: %v", err)
		}
		_, err := fix.items.Create(ctx, Item{ExternalID: "dup", Name: "b"})
		if !errors.Is(err, ErrDuplicateExternalID) {
			t.Fatalf("err = %v, want ErrDuplicateExternalID", err)
		}
	})

	t.Run(name+"/taxonomy_round_trip_weapon", func(t *testing.T) {
		fix := newFix(t)
		roomID := makeRoom(t, fix)
		ctx := context.Background()
		w := &WeaponStats{
			Proficiency: "martial", Size: "medium", Range: "melee",
			Damage: "1d8", ThreatLow: 19, CritMult: 2,
			DamageType: []string{"S"}, Special: []string{"finesse"},
		}
		in := Item{
			ExternalID: "longsword", Name: "a longsword", RoomID: roomID,
			Type: ItemTypeWeapon, Weight: 4, Value: 1500, // 15 mk = 1500 cp
			Quality: QualityMasterwork, Flags: FlagMagic | FlagGlow,
			Stats: w,
		}
		if _, err := fix.items.Create(ctx, in); err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, err := fix.items.ListInRoom(ctx, roomID)
		if err != nil || len(got) != 1 {
			t.Fatalf("ListInRoom: %v len=%d", err, len(got))
		}
		g := got[0]
		if g.Type != ItemTypeWeapon || g.Quality != QualityMasterwork {
			t.Errorf("type/quality lost: %+v", g)
		}
		if g.Weight != 4 || g.Value != 1500 {
			t.Errorf("weight/value lost: %+v", g)
		}
		if !g.HasFlag(FlagMagic) || !g.HasFlag(FlagGlow) || g.HasFlag(FlagNoTake) {
			t.Errorf("flags wrong: %b", g.Flags)
		}
		gw, ok := g.Stats.(*WeaponStats)
		if !ok {
			t.Fatalf("stats not WeaponStats: %T", g.Stats)
		}
		if gw.Damage != "1d8" || gw.ThreatLow != 19 || gw.Proficiency != "martial" {
			t.Errorf("weapon stats lost: %+v", gw)
		}
		if len(gw.Special) != 1 || gw.Special[0] != "finesse" {
			t.Errorf("special lost: %+v", gw.Special)
		}
	})

	t.Run(name+"/taxonomy_key_roundtrip", func(t *testing.T) {
		fix := newFix(t)
		roomID := makeRoom(t, fix)
		ctx := context.Background()
		in := Item{
			ExternalID: "iron.key", Name: "an iron key", RoomID: roomID,
			Type: ItemTypeKey, Stats: &KeyStats{KeyID: "iron.key"},
		}
		if _, err := fix.items.Create(ctx, in); err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, _ := fix.items.ListInRoom(ctx, roomID)
		ks, ok := got[0].Stats.(*KeyStats)
		if !ok || ks.KeyID != "iron.key" {
			t.Fatalf("KeyStats lost: %+v", got[0].Stats)
		}
	})

	t.Run(name+"/taxonomy_default_trash", func(t *testing.T) {
		fix := newFix(t)
		roomID := makeRoom(t, fix)
		ctx := context.Background()
		if _, err := fix.items.Create(ctx, Item{ExternalID: "pebble", Name: "pebble", RoomID: roomID}); err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, _ := fix.items.ListInRoom(ctx, roomID)
		if got[0].Type != ItemTypeTrash || got[0].Quality != QualityNormal {
			t.Errorf("defaults wrong: %+v", got[0])
		}
		if got[0].Stats != nil {
			t.Errorf("trash should have nil stats, got %T", got[0].Stats)
		}
	})

	t.Run(name+"/taxonomy_rejects_stats_mismatch", func(t *testing.T) {
		fix := newFix(t)
		_, err := fix.items.Create(context.Background(), Item{
			ExternalID: "bad", Name: "bad",
			Type: ItemTypeWeapon, Stats: &ArmorStats{},
		})
		if !errors.Is(err, ErrItemStatsTypeMismatch) {
			t.Fatalf("err = %v, want ErrItemStatsTypeMismatch", err)
		}
	})

	t.Run(name+"/taxonomy_rejects_typed_item_with_nil_stats", func(t *testing.T) {
		fix := newFix(t)
		_, err := fix.items.Create(context.Background(), Item{
			ExternalID: "naked", Name: "naked",
			Type: ItemTypeWeapon, // no Stats — must fail
		})
		if !errors.Is(err, ErrItemStatsTypeMismatch) {
			t.Fatalf("err = %v, want ErrItemStatsTypeMismatch", err)
		}
	})

	t.Run(name+"/taxonomy_rejects_untyped_with_stats", func(t *testing.T) {
		fix := newFix(t)
		_, err := fix.items.Create(context.Background(), Item{
			ExternalID: "weird", Name: "weird",
			Type: ItemTypeTrash, Stats: &WeaponStats{},
		})
		if !errors.Is(err, ErrItemStatsTypeMismatch) {
			t.Fatalf("err = %v, want ErrItemStatsTypeMismatch for trash+stats", err)
		}
	})

	t.Run(name+"/empty_room", func(t *testing.T) {
		fix := newFix(t)
		got, err := fix.items.ListInRoom(context.Background(), 99999)
		if err != nil {
			t.Fatalf("ListInRoom: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("got %d items, want 0", len(got))
		}
	})
}

func TestMemoryItemRepo(t *testing.T) {
	runItemRepoTests(t, "memory", func(t *testing.T) itemRepoFixture {
		return itemRepoFixture{items: NewMemoryItemRepo(), rooms: NewMemoryRoomRepo()}
	})
}

func TestSQLiteItemRepo(t *testing.T) {
	runItemRepoTests(t, "sqlite", func(t *testing.T) itemRepoFixture {
		conn, err := db.Open(context.Background(), ":memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		t.Cleanup(func() { conn.Close() })
		return itemRepoFixture{
			items: NewSQLiteItemRepo(conn),
			rooms: NewSQLiteRoomRepo(conn),
		}
	})
}

// TestSQLiteItemRepo_CorruptStatsJSON simulates an out-of-band DB
// edit (admin tool, partial restore) that leaves stats_json as
// non-JSON. ListInRoom must surface the decode error rather than
// pretend the row is fine — the read path is the last line of
// defense after the CHECK + Create-time validation.
func TestSQLiteItemRepo_CorruptStatsJSON(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	rooms := NewSQLiteRoomRepo(conn)
	r, err := rooms.Create(ctx, Room{ExternalID: "r", Name: "R"})
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	// Bypass the repo Create validation and write a typed row with
	// garbage stats_json directly. Mirrors what an external tool
	// could leave behind.
	if _, err := conn.ExecContext(ctx,
		`INSERT INTO items(external_id, name, name_lower, short_desc, room_id,
			type, weight_lbs, value_cp, quality, flags, stats_json)
		 VALUES (?, ?, ?, '', ?, 'weapon', 0, 0, 'normal', 0, ?)`,
		"corrupt", "broken", "broken", r.ID, "not-json",
	); err != nil {
		t.Fatalf("seed corrupt row: %v", err)
	}
	items := NewSQLiteItemRepo(conn)
	if _, err := items.ListInRoom(ctx, r.ID); err == nil {
		t.Fatal("ListInRoom: want decode error, got nil")
	}
}
