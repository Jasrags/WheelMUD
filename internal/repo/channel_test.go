package repo

import (
	"context"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/db"
)

func runChannelRepoTests(t *testing.T, name string, newRepo func(t *testing.T) ChannelRepo) {
	t.Helper()
	t.Run(name+"/list_returns_seeds_alphabetical", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		got, err := r.List(ctx)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(got) != 3 {
			t.Fatalf("got %d channels, want 3 seeds", len(got))
		}
		want := []string{"gossip", "newbie", "ooc"}
		for i, ch := range got {
			if ch.Name != want[i] {
				t.Fatalf("position %d: got %q, want %q", i, ch.Name, want[i])
			}
			if ch.ID == 0 {
				t.Fatalf("channel %q has zero ID", ch.Name)
			}
			if ch.Color == "" {
				t.Fatalf("channel %q has empty color", ch.Name)
			}
		}
	})
}

func TestMemoryChannelRepo(t *testing.T) {
	runChannelRepoTests(t, "memory", func(t *testing.T) ChannelRepo {
		return NewMemoryChannelRepo(
			Channel{Name: "ooc", MinLevel: 0, Color: "cyan"},
			Channel{Name: "gossip", MinLevel: 0, Color: "magenta"},
			Channel{Name: "newbie", MinLevel: 0, Color: "green"},
		)
	})
}

func TestSQLiteChannelRepo(t *testing.T) {
	runChannelRepoTests(t, "sqlite", func(t *testing.T) ChannelRepo {
		conn, err := db.Open(context.Background(), ":memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		t.Cleanup(func() { conn.Close() })
		return NewSQLiteChannelRepo(conn)
	})
}
