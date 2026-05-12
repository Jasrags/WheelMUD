package repo

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/db"
)

func runCharacterAuditRepoTests(t *testing.T, name string, newRepo func(t *testing.T) CharacterAuditRepo) {
	t.Helper()

	t.Run(name+"/record_assigns_id_and_ts", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		if err := r.Record(ctx, CharacterAuditEntry{
			CharacterID:   7,
			CharacterName: "Moiraine",
			RoomID:        42,
			Verb:          "look",
			Raw:           "look",
		}); err != nil {
			t.Fatalf("record: %v", err)
		}
		got, err := r.List(ctx, CharacterAuditFilter{})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d entries, want 1", len(got))
		}
		e := got[0]
		if e.ID == 0 {
			t.Fatal("ID not assigned")
		}
		if e.TS.IsZero() {
			t.Fatal("TS not stamped")
		}
		if e.Verb != "look" || e.CharacterName != "Moiraine" || e.RoomID != 42 {
			t.Fatalf("unexpected entry: %+v", e)
		}
	})

	t.Run(name+"/list_orders_newest_first", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		base := time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC)
		for i, v := range []string{"look", "north", "say"} {
			if err := r.Record(ctx, CharacterAuditEntry{
				TS:          base.Add(time.Duration(i) * time.Second),
				CharacterID: 1,
				Verb:        v,
			}); err != nil {
				t.Fatalf("record %s: %v", v, err)
			}
		}
		got, err := r.List(ctx, CharacterAuditFilter{})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		want := []string{"say", "north", "look"}
		for i, e := range got {
			if e.Verb != want[i] {
				t.Fatalf("position %d verb = %q, want %q", i, e.Verb, want[i])
			}
		}
	})

	t.Run(name+"/list_filters", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		base := time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC)
		seed := []CharacterAuditEntry{
			{TS: base, CharacterID: 1, Verb: "look"},
			{TS: base.Add(1 * time.Second), CharacterID: 2, Verb: "north"},
			{TS: base.Add(2 * time.Second), CharacterID: 1, Verb: "say"},
			{TS: base.Add(3 * time.Second), CharacterID: 2, Verb: "look"},
		}
		for _, e := range seed {
			if err := r.Record(ctx, e); err != nil {
				t.Fatalf("record: %v", err)
			}
		}

		got, err := r.List(ctx, CharacterAuditFilter{Verbs: []string{"look"}})
		if err != nil || len(got) != 2 {
			t.Fatalf("verbs filter: got %d (err=%v), want 2", len(got), err)
		}
		got, err = r.List(ctx, CharacterAuditFilter{Character: 1})
		if err != nil || len(got) != 2 {
			t.Fatalf("character filter: got %d (err=%v), want 2", len(got), err)
		}
		got, err = r.List(ctx, CharacterAuditFilter{Since: base.Add(2 * time.Second)})
		if err != nil || len(got) != 2 {
			t.Fatalf("since filter: got %d (err=%v), want 2", len(got), err)
		}
		got, err = r.List(ctx, CharacterAuditFilter{Character: 2, Verbs: []string{"look"}})
		if err != nil || len(got) != 1 || got[0].CharacterID != 2 {
			t.Fatalf("combined filter: %+v (err=%v)", got, err)
		}
	})

	t.Run(name+"/raw_truncated_to_cap", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		oversized := strings.Repeat("x", CharacterAuditRawCap+1024)
		if err := r.Record(ctx, CharacterAuditEntry{
			CharacterID: 1,
			Verb:        "say",
			Raw:         oversized,
		}); err != nil {
			t.Fatalf("record: %v", err)
		}
		got, err := r.List(ctx, CharacterAuditFilter{})
		if err != nil || len(got) != 1 {
			t.Fatalf("list: got %d (err=%v)", len(got), err)
		}
		if len(got[0].Raw) != CharacterAuditRawCap {
			t.Fatalf("raw length = %d, want %d", len(got[0].Raw), CharacterAuditRawCap)
		}
	})

	t.Run(name+"/list_limit", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		for i := 0; i < 5; i++ {
			if err := r.Record(ctx, CharacterAuditEntry{
				CharacterID: 1,
				Verb:        "look",
				TS:          time.Date(2026, 5, 12, 0, 0, i, 0, time.UTC),
			}); err != nil {
				t.Fatalf("record %d: %v", i, err)
			}
		}
		got, err := r.List(ctx, CharacterAuditFilter{Limit: 3})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 3 {
			t.Fatalf("limit not honored: got %d", len(got))
		}
	})
}

func TestMemoryCharacterAuditRepo(t *testing.T) {
	runCharacterAuditRepoTests(t, "memory", func(t *testing.T) CharacterAuditRepo {
		return NewMemoryCharacterAuditRepo()
	})
}

func TestSQLiteCharacterAuditRepo(t *testing.T) {
	runCharacterAuditRepoTests(t, "sqlite", func(t *testing.T) CharacterAuditRepo {
		conn, err := db.Open(context.Background(), ":memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		t.Cleanup(func() { conn.Close() })
		return NewSQLiteCharacterAuditRepo(conn)
	})
}
