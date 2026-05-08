package repo

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/db"
)

type mobInstanceFixture struct {
	templates MobTemplateRepo
	instances MobInstanceRepo
	rooms     RoomRepo
}

func runMobInstanceRepoTests(t *testing.T, name string, newFix func(t *testing.T) mobInstanceFixture) {
	t.Helper()

	makeFixtures := func(t *testing.T) (mobInstanceFixture, int64, int64) {
		t.Helper()
		fix := newFix(t)
		ctx := context.Background()
		room, err := fix.rooms.Create(ctx, Room{ExternalID: "plaza", Name: "Plaza"})
		if err != nil {
			t.Fatalf("create room: %v", err)
		}
		tpl, err := fix.templates.Create(ctx, creature.MobTemplate{
			ExternalID:    "rat.basic",
			ChallengeCode: 'A',
			Core: creature.Core{
				Name: "a brown rat", Size: creature.SizeTiny, Type: creature.TypeAnimal,
				HPMax: 4, Defense: 12,
			},
		})
		if err != nil {
			t.Fatalf("create template: %v", err)
		}
		return fix, room.ID, tpl.ID
	}

	t.Run(name+"/spawn_and_list", func(t *testing.T) {
		fix, roomID, tplID := makeFixtures(t)
		ctx := context.Background()
		spawn, err := fix.instances.Create(ctx, creature.MobInstance{
			TemplateID: tplID,
			Core:       creature.Core{HPCurrent: 4, CurrentRoomID: roomID},
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if spawn.ID == 0 {
			t.Fatal("zero ID")
		}
		list, err := fix.instances.ListInRoom(ctx, roomID)
		if err != nil {
			t.Fatalf("ListInRoom: %v", err)
		}
		if len(list) != 1 || list[0].ID != spawn.ID {
			t.Fatalf("got %+v", list)
		}
		if list[0].Core.HPCurrent != 4 || list[0].Core.CurrentRoomID != roomID {
			t.Fatalf("scan dropped fields: %+v", list[0].Core)
		}
	})

	t.Run(name+"/update_live_and_room", func(t *testing.T) {
		fix, roomID, tplID := makeFixtures(t)
		ctx := context.Background()
		room2, err := fix.rooms.Create(ctx, Room{ExternalID: "alley", Name: "Alley"})
		if err != nil {
			t.Fatalf("second room: %v", err)
		}
		spawn, err := fix.instances.Create(ctx, creature.MobInstance{
			TemplateID: tplID,
			Core:       creature.Core{HPCurrent: 4, CurrentRoomID: roomID},
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		if err := fix.instances.UpdateLive(ctx, spawn.ID, 1, 2, creature.CondStunned, creature.PosFlanked); err != nil {
			t.Fatalf("UpdateLive: %v", err)
		}
		got, err := fix.instances.GetByID(ctx, spawn.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.Core.HPCurrent != 1 || got.Core.Subdual != 2 {
			t.Fatalf("HP/subdual not persisted: %+v", got.Core)
		}
		if got.Core.Conditions != creature.CondStunned || got.Core.Position != creature.PosFlanked {
			t.Fatalf("conditions/position not persisted: %+v", got.Core)
		}

		if err := fix.instances.UpdateRoom(ctx, spawn.ID, room2.ID); err != nil {
			t.Fatalf("UpdateRoom: %v", err)
		}
		got, _ = fix.instances.GetByID(ctx, spawn.ID)
		if got.Core.CurrentRoomID != room2.ID {
			t.Fatalf("CurrentRoomID = %d, want %d", got.Core.CurrentRoomID, room2.ID)
		}
	})

	t.Run(name+"/delete", func(t *testing.T) {
		fix, roomID, tplID := makeFixtures(t)
		ctx := context.Background()
		spawn, _ := fix.instances.Create(ctx, creature.MobInstance{
			TemplateID: tplID,
			Core:       creature.Core{HPCurrent: 4, CurrentRoomID: roomID},
		})
		if err := fix.instances.Delete(ctx, spawn.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		_, err := fix.instances.GetByID(ctx, spawn.ID)
		if !errors.Is(err, ErrInstanceNotFound) {
			t.Fatalf("err = %v, want ErrInstanceNotFound", err)
		}
		if err := fix.instances.Delete(ctx, spawn.ID); !errors.Is(err, ErrInstanceNotFound) {
			t.Fatalf("Delete twice err = %v, want ErrInstanceNotFound", err)
		}
	})

	t.Run(name+"/missing_template_id_rejected", func(t *testing.T) {
		fix := newFix(t)
		_, err := fix.instances.Create(context.Background(), creature.MobInstance{})
		if err == nil {
			t.Fatal("expected error for missing TemplateID")
		}
	})

	t.Run(name+"/update_unknown_returns_not_found", func(t *testing.T) {
		fix := newFix(t)
		err := fix.instances.UpdateLive(context.Background(), 999, 0, 0, 0, 0)
		if !errors.Is(err, ErrInstanceNotFound) {
			t.Fatalf("err = %v, want ErrInstanceNotFound", err)
		}
	})

	t.Run(name+"/update_room_records_trail", func(t *testing.T) {
		fix, roomID, tplID := makeFixtures(t)
		ctx := context.Background()
		room2, err := fix.rooms.Create(ctx, Room{ExternalID: "alley", Name: "Alley"})
		if err != nil {
			t.Fatalf("second room: %v", err)
		}
		room3, err := fix.rooms.Create(ctx, Room{ExternalID: "square", Name: "Square"})
		if err != nil {
			t.Fatalf("third room: %v", err)
		}
		spawn, err := fix.instances.Create(ctx, creature.MobInstance{
			TemplateID: tplID,
			Core:       creature.Core{HPCurrent: 4, CurrentRoomID: roomID},
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := fix.instances.UpdateRoom(ctx, spawn.ID, room2.ID); err != nil {
			t.Fatalf("UpdateRoom 2: %v", err)
		}
		if err := fix.instances.UpdateRoom(ctx, spawn.ID, room3.ID); err != nil {
			t.Fatalf("UpdateRoom 3: %v", err)
		}
		got, err := fix.instances.RecentTrails(ctx, spawn.ID, 8)
		if err != nil {
			t.Fatalf("RecentTrails: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("len(trails) = %d, want 2: %+v", len(got), got)
		}
		// Newest first.
		if got[0].RoomID != room3.ID || got[1].RoomID != room2.ID {
			t.Fatalf("trail order = [%d, %d], want [%d, %d]",
				got[0].RoomID, got[1].RoomID, room3.ID, room2.ID)
		}
		if got[0].MobID != spawn.ID {
			t.Fatalf("MobID = %d, want %d", got[0].MobID, spawn.ID)
		}
		if got[0].At.IsZero() {
			t.Fatal("At not populated")
		}
	})

	t.Run(name+"/update_room_zero_skips_trail", func(t *testing.T) {
		fix, roomID, tplID := makeFixtures(t)
		ctx := context.Background()
		spawn, err := fix.instances.Create(ctx, creature.MobInstance{
			TemplateID: tplID,
			Core:       creature.Core{HPCurrent: 4, CurrentRoomID: roomID},
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := fix.instances.UpdateRoom(ctx, spawn.ID, 0); err != nil {
			t.Fatalf("UpdateRoom(0): %v", err)
		}
		got, err := fix.instances.RecentTrails(ctx, spawn.ID, 8)
		if err != nil {
			t.Fatalf("RecentTrails: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("trail recorded for roomID=0: %+v", got)
		}
		live, err := fix.instances.GetByID(ctx, spawn.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if live.Core.CurrentRoomID != 0 {
			t.Fatalf("CurrentRoomID = %d, want 0", live.Core.CurrentRoomID)
		}
	})

	t.Run(name+"/update_room_unknown_id_no_trail", func(t *testing.T) {
		fix, roomID, _ := makeFixtures(t)
		ctx := context.Background()
		err := fix.instances.UpdateRoom(ctx, 9999, roomID)
		if !errors.Is(err, ErrInstanceNotFound) {
			t.Fatalf("err = %v, want ErrInstanceNotFound", err)
		}
		got, err := fix.instances.RecentTrails(ctx, 9999, 8)
		if err != nil {
			t.Fatalf("RecentTrails: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("trail recorded for unknown mob: %+v", got)
		}
	})

	t.Run(name+"/recent_trails_caps_at_16", func(t *testing.T) {
		fix, roomID, tplID := makeFixtures(t)
		ctx := context.Background()
		// Build 20 distinct rooms so the trail rows reference real ids.
		rooms := make([]int64, 20)
		for i := range rooms {
			rm, err := fix.rooms.Create(ctx, Room{
				ExternalID: "trail." + strconv.Itoa(i),
				Name:       "Trail Room",
			})
			if err != nil {
				t.Fatalf("room %d: %v", i, err)
			}
			rooms[i] = rm.ID
		}
		spawn, err := fix.instances.Create(ctx, creature.MobInstance{
			TemplateID: tplID,
			Core:       creature.Core{HPCurrent: 4, CurrentRoomID: roomID},
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		for _, rid := range rooms {
			if err := fix.instances.UpdateRoom(ctx, spawn.ID, rid); err != nil {
				t.Fatalf("UpdateRoom %d: %v", rid, err)
			}
		}
		got, err := fix.instances.RecentTrails(ctx, spawn.ID, 100)
		if err != nil {
			t.Fatalf("RecentTrails: %v", err)
		}
		if len(got) != MobTrailCap {
			t.Fatalf("len(trails) = %d, want %d", len(got), MobTrailCap)
		}
		// Newest first; the last room visited should be first.
		if got[0].RoomID != rooms[len(rooms)-1] {
			t.Fatalf("newest = %d, want %d", got[0].RoomID, rooms[len(rooms)-1])
		}
		// Oldest retained should be the (20-16)=4th room (index 4).
		oldestKept := rooms[len(rooms)-MobTrailCap]
		if got[len(got)-1].RoomID != oldestKept {
			t.Fatalf("oldest kept = %d, want %d", got[len(got)-1].RoomID, oldestKept)
		}
	})

	t.Run(name+"/recent_trails_isolates_per_mob", func(t *testing.T) {
		fix, roomID, tplID := makeFixtures(t)
		ctx := context.Background()
		room2, err := fix.rooms.Create(ctx, Room{ExternalID: "alley", Name: "Alley"})
		if err != nil {
			t.Fatalf("second room: %v", err)
		}
		mobA, err := fix.instances.Create(ctx, creature.MobInstance{
			TemplateID: tplID, Core: creature.Core{HPCurrent: 4, CurrentRoomID: roomID},
		})
		if err != nil {
			t.Fatalf("Create A: %v", err)
		}
		mobB, err := fix.instances.Create(ctx, creature.MobInstance{
			TemplateID: tplID, Core: creature.Core{HPCurrent: 4, CurrentRoomID: roomID},
		})
		if err != nil {
			t.Fatalf("Create B: %v", err)
		}
		if err := fix.instances.UpdateRoom(ctx, mobA.ID, room2.ID); err != nil {
			t.Fatalf("UpdateRoom A: %v", err)
		}
		gotB, err := fix.instances.RecentTrails(ctx, mobB.ID, 8)
		if err != nil {
			t.Fatalf("RecentTrails B: %v", err)
		}
		if len(gotB) != 0 {
			t.Fatalf("mob B trails leaked from mob A: %+v", gotB)
		}
		gotA, err := fix.instances.RecentTrails(ctx, mobA.ID, 8)
		if err != nil {
			t.Fatalf("RecentTrails A: %v", err)
		}
		if len(gotA) != 1 || gotA[0].MobID != mobA.ID {
			t.Fatalf("mob A trails = %+v", gotA)
		}
	})

	t.Run(name+"/delete_clears_trails", func(t *testing.T) {
		fix, roomID, tplID := makeFixtures(t)
		ctx := context.Background()
		room2, err := fix.rooms.Create(ctx, Room{ExternalID: "alley", Name: "Alley"})
		if err != nil {
			t.Fatalf("second room: %v", err)
		}
		spawn, err := fix.instances.Create(ctx, creature.MobInstance{
			TemplateID: tplID,
			Core:       creature.Core{HPCurrent: 4, CurrentRoomID: roomID},
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := fix.instances.UpdateRoom(ctx, spawn.ID, room2.ID); err != nil {
			t.Fatalf("UpdateRoom: %v", err)
		}
		if err := fix.instances.Delete(ctx, spawn.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		got, err := fix.instances.RecentTrails(ctx, spawn.ID, 8)
		if err != nil {
			t.Fatalf("RecentTrails: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("trails survived Delete: %+v", got)
		}
	})

	t.Run(name+"/list_spawned_filters_zero_room_and_sorts", func(t *testing.T) {
		fix, roomID, tplID := makeFixtures(t)
		ctx := context.Background()
		room2, err := fix.rooms.Create(ctx, Room{ExternalID: "alley", Name: "Alley"})
		if err != nil {
			t.Fatalf("second room: %v", err)
		}
		mobA, err := fix.instances.Create(ctx, creature.MobInstance{
			TemplateID: tplID, Core: creature.Core{HPCurrent: 4, CurrentRoomID: roomID},
		})
		if err != nil {
			t.Fatalf("Create A: %v", err)
		}
		mobB, err := fix.instances.Create(ctx, creature.MobInstance{
			TemplateID: tplID, Core: creature.Core{HPCurrent: 4, CurrentRoomID: room2.ID},
		})
		if err != nil {
			t.Fatalf("Create B: %v", err)
		}
		mobC, err := fix.instances.Create(ctx, creature.MobInstance{
			TemplateID: tplID, Core: creature.Core{HPCurrent: 4, CurrentRoomID: roomID},
		})
		if err != nil {
			t.Fatalf("Create C: %v", err)
		}
		// Despawn C — should drop out of the spawned listing.
		if err := fix.instances.UpdateRoom(ctx, mobC.ID, 0); err != nil {
			t.Fatalf("UpdateRoom 0: %v", err)
		}

		got, err := fix.instances.ListSpawned(ctx, 10)
		if err != nil {
			t.Fatalf("ListSpawned: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2: %+v", len(got), got)
		}
		if got[0].ID != mobA.ID || got[1].ID != mobB.ID {
			t.Fatalf("order = [%d, %d], want [%d, %d]",
				got[0].ID, got[1].ID, mobA.ID, mobB.ID)
		}

		// limit honored.
		one, err := fix.instances.ListSpawned(ctx, 1)
		if err != nil {
			t.Fatalf("ListSpawned(1): %v", err)
		}
		if len(one) != 1 || one[0].ID != mobA.ID {
			t.Fatalf("limit 1 = %+v, want [%d]", one, mobA.ID)
		}

		// limit <= 0 is empty.
		zero, err := fix.instances.ListSpawned(ctx, 0)
		if err != nil {
			t.Fatalf("ListSpawned(0): %v", err)
		}
		if len(zero) != 0 {
			t.Fatalf("limit 0 returned %d", len(zero))
		}
	})

	t.Run(name+"/count_by_template", func(t *testing.T) {
		fix, roomID, tplID := makeFixtures(t)
		ctx := context.Background()
		got, err := fix.instances.CountByTemplate(ctx, tplID)
		if err != nil {
			t.Fatalf("CountByTemplate empty: %v", err)
		}
		if got != 0 {
			t.Fatalf("empty count = %d, want 0", got)
		}
		for i := 0; i < 3; i++ {
			if _, err := fix.instances.Create(ctx, creature.MobInstance{
				TemplateID: tplID,
				Core:       creature.Core{HPCurrent: 4, CurrentRoomID: roomID},
			}); err != nil {
				t.Fatalf("Create: %v", err)
			}
		}
		got, err = fix.instances.CountByTemplate(ctx, tplID)
		if err != nil {
			t.Fatalf("CountByTemplate: %v", err)
		}
		if got != 3 {
			t.Fatalf("count = %d, want 3", got)
		}
		other, err := fix.instances.CountByTemplate(ctx, tplID+999)
		if err != nil {
			t.Fatalf("CountByTemplate unknown: %v", err)
		}
		if other != 0 {
			t.Fatalf("unknown template count = %d, want 0", other)
		}
	})

	t.Run(name+"/recent_trails_limit_zero", func(t *testing.T) {
		fix, roomID, tplID := makeFixtures(t)
		ctx := context.Background()
		room2, err := fix.rooms.Create(ctx, Room{ExternalID: "alley", Name: "Alley"})
		if err != nil {
			t.Fatalf("second room: %v", err)
		}
		spawn, err := fix.instances.Create(ctx, creature.MobInstance{
			TemplateID: tplID, Core: creature.Core{HPCurrent: 4, CurrentRoomID: roomID},
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := fix.instances.UpdateRoom(ctx, spawn.ID, room2.ID); err != nil {
			t.Fatalf("UpdateRoom: %v", err)
		}
		got, err := fix.instances.RecentTrails(ctx, spawn.ID, 0)
		if err != nil {
			t.Fatalf("RecentTrails: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("limit=0 returned %d rows", len(got))
		}
	})
}

func TestMemoryMobInstanceRepo(t *testing.T) {
	runMobInstanceRepoTests(t, "memory", func(t *testing.T) mobInstanceFixture {
		return mobInstanceFixture{
			templates: NewMemoryMobTemplateRepo(),
			instances: NewMemoryMobInstanceRepo(),
			rooms:     NewMemoryRoomRepo(),
		}
	})
}

func TestSQLiteMobInstanceRepo(t *testing.T) {
	runMobInstanceRepoTests(t, "sqlite", func(t *testing.T) mobInstanceFixture {
		conn, err := db.Open(context.Background(), ":memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		t.Cleanup(func() { conn.Close() })
		return mobInstanceFixture{
			templates: NewSQLiteMobTemplateRepo(conn),
			instances: NewSQLiteMobInstanceRepo(conn),
			rooms:     NewSQLiteRoomRepo(conn),
		}
	})
}
