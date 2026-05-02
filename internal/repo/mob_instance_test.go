package repo

import (
	"context"
	"errors"
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
