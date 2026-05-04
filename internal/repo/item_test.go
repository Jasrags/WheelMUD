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

	t.Run(name+"/set_owner_clears_room", func(t *testing.T) {
		fix := newFix(t)
		roomID := makeRoom(t, fix)
		ctx := context.Background()
		it, err := fix.items.Create(ctx, Item{ExternalID: "rock", Name: "a rock", RoomID: roomID})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := fix.items.SetOwner(ctx, it.ID, 42); err != nil {
			t.Fatalf("SetOwner: %v", err)
		}
		floor, _ := fix.items.ListInRoom(ctx, roomID)
		if len(floor) != 0 {
			t.Errorf("room still lists item after SetOwner: %+v", floor)
		}
		held, err := fix.items.ListInInventory(ctx, 42)
		if err != nil || len(held) != 1 || held[0].ID != it.ID {
			t.Fatalf("ListInInventory: err=%v got=%+v", err, held)
		}
		if held[0].RoomID != 0 || held[0].OwnerCharacterID != 42 {
			t.Errorf("location not flipped: %+v", held[0])
		}
	})

	t.Run(name+"/set_room_clears_owner", func(t *testing.T) {
		fix := newFix(t)
		roomID := makeRoom(t, fix)
		ctx := context.Background()
		it, err := fix.items.Create(ctx, Item{ExternalID: "ring", Name: "a ring", OwnerCharacterID: 7})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := fix.items.SetRoom(ctx, it.ID, roomID); err != nil {
			t.Fatalf("SetRoom: %v", err)
		}
		held, _ := fix.items.ListInInventory(ctx, 7)
		if len(held) != 0 {
			t.Errorf("inventory still lists item after SetRoom: %+v", held)
		}
		floor, _ := fix.items.ListInRoom(ctx, roomID)
		if len(floor) != 1 {
			t.Fatalf("ListInRoom: got %+v", floor)
		}
	})

	t.Run(name+"/get_by_id_round_trip", func(t *testing.T) {
		fix := newFix(t)
		roomID := makeRoom(t, fix)
		ctx := context.Background()
		in, err := fix.items.Create(ctx, Item{ExternalID: "torch", Name: "a torch", RoomID: roomID})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, err := fix.items.GetByID(ctx, in.ID)
		if err != nil || got.ExternalID != "torch" {
			t.Fatalf("GetByID: err=%v got=%+v", err, got)
		}
		_, err = fix.items.GetByID(ctx, 999999)
		if !errors.Is(err, ErrItemNotFound) {
			t.Fatalf("GetByID(missing) = %v, want ErrItemNotFound", err)
		}
	})

	t.Run(name+"/set_owner_missing_item", func(t *testing.T) {
		fix := newFix(t)
		err := fix.items.SetOwner(context.Background(), 999999, 1)
		if !errors.Is(err, ErrItemNotFound) {
			t.Fatalf("SetOwner(missing) = %v, want ErrItemNotFound", err)
		}
	})

	t.Run(name+"/list_in_inventory_zero_owner_is_empty", func(t *testing.T) {
		fix := newFix(t)
		roomID := makeRoom(t, fix)
		ctx := context.Background()
		if _, err := fix.items.Create(ctx, Item{ExternalID: "leaf", Name: "a leaf", RoomID: roomID}); err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, err := fix.items.ListInInventory(ctx, 0)
		if err != nil || len(got) != 0 {
			t.Fatalf("ListInInventory(0) should be empty: err=%v got=%+v", err, got)
		}
	})

	t.Run(name+"/transfer_room_to_owner_first_writer_wins", func(t *testing.T) {
		fix := newFix(t)
		roomID := makeRoom(t, fix)
		ctx := context.Background()
		it, err := fix.items.Create(ctx, Item{ExternalID: "rock", Name: "a rock", RoomID: roomID})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		// Alice (id 11) wins.
		if err := fix.items.TransferRoomToOwner(ctx, it.ID, roomID, 11); err != nil {
			t.Fatalf("first transfer: %v", err)
		}
		// Bob (id 22) racing for the same item — item is no longer on
		// the floor, so the second UPDATE must be a no-op surfacing
		// ErrItemMoved rather than silently overwriting.
		if err := fix.items.TransferRoomToOwner(ctx, it.ID, roomID, 22); !errors.Is(err, ErrItemMoved) {
			t.Fatalf("racing transfer: got %v, want ErrItemMoved", err)
		}
		got, _ := fix.items.GetByID(ctx, it.ID)
		if got.OwnerCharacterID != 11 {
			t.Fatalf("alice should own; got %+v", got)
		}
	})

	t.Run(name+"/transfer_owner_to_room_guards_owner", func(t *testing.T) {
		fix := newFix(t)
		roomID := makeRoom(t, fix)
		ctx := context.Background()
		it, err := fix.items.Create(ctx, Item{ExternalID: "ring", Name: "a ring", OwnerCharacterID: 7})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		// Wrong owner → ErrItemMoved.
		if err := fix.items.TransferOwnerToRoom(ctx, it.ID, 99, roomID); !errors.Is(err, ErrItemMoved) {
			t.Fatalf("wrong owner: got %v, want ErrItemMoved", err)
		}
		// Correct owner → success.
		if err := fix.items.TransferOwnerToRoom(ctx, it.ID, 7, roomID); err != nil {
			t.Fatalf("correct owner: %v", err)
		}
	})

	t.Run(name+"/transfer_owner_to_owner_guards_source", func(t *testing.T) {
		fix := newFix(t)
		ctx := context.Background()
		it, err := fix.items.Create(ctx, Item{ExternalID: "letter", Name: "a letter", OwnerCharacterID: 7})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := fix.items.TransferOwnerToOwner(ctx, it.ID, 99, 8); !errors.Is(err, ErrItemMoved) {
			t.Fatalf("wrong source: got %v, want ErrItemMoved", err)
		}
		if err := fix.items.TransferOwnerToOwner(ctx, it.ID, 7, 8); err != nil {
			t.Fatalf("correct source: %v", err)
		}
		got, _ := fix.items.GetByID(ctx, it.ID)
		if got.OwnerCharacterID != 8 || got.RoomID != 0 {
			t.Fatalf("transfer left bad state: %+v", got)
		}
	})

	t.Run(name+"/transfer_missing_item_returns_not_found", func(t *testing.T) {
		fix := newFix(t)
		err := fix.items.TransferRoomToOwner(context.Background(), 999999, 1, 2)
		if !errors.Is(err, ErrItemNotFound) {
			t.Fatalf("got %v, want ErrItemNotFound", err)
		}
	})

	t.Run(name+"/list_in_room_excludes_owned", func(t *testing.T) {
		// Defends against the location-invariant violation case: even
		// if a row ends up with both columns set (only achievable via
		// raw SQL today), ListInRoom must not surface it on the floor.
		fix := newFix(t)
		roomID := makeRoom(t, fix)
		ctx := context.Background()
		// Owned-only item — must not appear in room view.
		if _, err := fix.items.Create(ctx, Item{ExternalID: "owned", Name: "an owned cup", OwnerCharacterID: 5}); err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, _ := fix.items.ListInRoom(ctx, roomID)
		if len(got) != 0 {
			t.Fatalf("owned item leaked into ListInRoom: %+v", got)
		}
	})

	t.Run(name+"/transfer_owner_to_container_xor_invariant", func(t *testing.T) {
		fix := newFix(t)
		ctx := context.Background()
		bag, err := fix.items.Create(ctx, Item{
			ExternalID: "bag", Name: "a bag", OwnerCharacterID: 7,
			Type: ItemTypeContainer, Stats: &ContainerStats{CapacityLbs: 10},
		})
		if err != nil {
			t.Fatalf("Create bag: %v", err)
		}
		coin, err := fix.items.Create(ctx, Item{
			ExternalID: "coin", Name: "a coin", OwnerCharacterID: 7, Weight: 0.1,
		})
		if err != nil {
			t.Fatalf("Create coin: %v", err)
		}
		// Wrong source owner → ErrItemMoved.
		if err := fix.items.TransferOwnerToContainer(ctx, coin.ID, 99, bag.ID); !errors.Is(err, ErrItemMoved) {
			t.Fatalf("wrong source: got %v, want ErrItemMoved", err)
		}
		// Correct path → all three location columns reflect "in bag".
		if err := fix.items.TransferOwnerToContainer(ctx, coin.ID, 7, bag.ID); err != nil {
			t.Fatalf("transfer: %v", err)
		}
		got, err := fix.items.GetByID(ctx, coin.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.ParentItemID != bag.ID || got.OwnerCharacterID != 0 || got.RoomID != 0 {
			t.Fatalf("bad state after put: %+v", got)
		}
		// Top-level inventory excludes nested items.
		top, _ := fix.items.ListInInventory(ctx, 7)
		for _, x := range top {
			if x.ID == coin.ID {
				t.Fatalf("nested coin leaked into top-level inventory: %+v", x)
			}
		}
		// ListInContainer surfaces it.
		inside, _ := fix.items.ListInContainer(ctx, bag.ID)
		if len(inside) != 1 || inside[0].ID != coin.ID {
			t.Fatalf("ListInContainer = %+v", inside)
		}
	})

	t.Run(name+"/transfer_container_to_owner_xor_invariant", func(t *testing.T) {
		fix := newFix(t)
		ctx := context.Background()
		bag, err := fix.items.Create(ctx, Item{
			ExternalID: "bag2", Name: "a sack", OwnerCharacterID: 7,
			Type: ItemTypeContainer, Stats: &ContainerStats{CapacityLbs: 10},
		})
		if err != nil {
			t.Fatalf("Create bag: %v", err)
		}
		gem, err := fix.items.Create(ctx, Item{
			ExternalID: "gem", Name: "a gem", ParentItemID: bag.ID, Weight: 0.5,
		})
		if err != nil {
			t.Fatalf("Create gem: %v", err)
		}
		// Wrong source parent → ErrItemMoved.
		if err := fix.items.TransferContainerToOwner(ctx, gem.ID, 99999, 7); !errors.Is(err, ErrItemMoved) {
			t.Fatalf("wrong parent: got %v, want ErrItemMoved", err)
		}
		// Correct path.
		if err := fix.items.TransferContainerToOwner(ctx, gem.ID, bag.ID, 7); err != nil {
			t.Fatalf("transfer: %v", err)
		}
		got, _ := fix.items.GetByID(ctx, gem.ID)
		if got.OwnerCharacterID != 7 || got.ParentItemID != 0 || got.RoomID != 0 {
			t.Fatalf("bad state after get-from: %+v", got)
		}
	})

	t.Run(name+"/list_all_owned_transitive_walks_chain", func(t *testing.T) {
		fix := newFix(t)
		ctx := context.Background()
		// pack (top) → pouch (nested) → coin (deeply nested).
		pack, err := fix.items.Create(ctx, Item{
			ExternalID: "pack", Name: "a pack", OwnerCharacterID: 7,
			Type: ItemTypeContainer, Stats: &ContainerStats{CapacityLbs: 50},
		})
		if err != nil {
			t.Fatalf("Create pack: %v", err)
		}
		pouch, err := fix.items.Create(ctx, Item{
			ExternalID: "pouch", Name: "a pouch", ParentItemID: pack.ID,
			Type: ItemTypeContainer, Stats: &ContainerStats{CapacityLbs: 5},
		})
		if err != nil {
			t.Fatalf("Create pouch: %v", err)
		}
		_, err = fix.items.Create(ctx, Item{
			ExternalID: "coin2", Name: "a coin", ParentItemID: pouch.ID, Weight: 0.1,
		})
		if err != nil {
			t.Fatalf("Create coin: %v", err)
		}
		all, err := fix.items.ListAllOwnedTransitive(ctx, 7)
		if err != nil {
			t.Fatalf("ListAllOwnedTransitive: %v", err)
		}
		if len(all) != 3 {
			t.Fatalf("got %d items, want 3 (%+v)", len(all), all)
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
		`INSERT INTO items(external_id, name, name_lower, short_desc, room_id, owner_character_id,
			type, weight_lbs, value_cp, quality, flags, stats_json)
		 VALUES (?, ?, ?, '', ?, NULL, 'weapon', 0, 0, 'normal', 0, ?)`,
		"corrupt", "broken", "broken", r.ID, "not-json",
	); err != nil {
		t.Fatalf("seed corrupt row: %v", err)
	}
	items := NewSQLiteItemRepo(conn)
	if _, err := items.ListInRoom(ctx, r.ID); err == nil {
		t.Fatal("ListInRoom: want decode error, got nil")
	}
}
