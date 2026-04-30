package cmd

import (
	"bytes"
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// bufConn satisfies net.Conn against an in-memory buffer. Tests use it
// to inspect everything a Session writes without dealing with the
// net.Pipe's synchronous-read scheduling. Read is unused by these
// tests; it just blocks until Close.
type bufConn struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	closed chan struct{}
	once   sync.Once
}

func newBufConn() *bufConn { return &bufConn{closed: make(chan struct{})} }

func (b *bufConn) Read(_ []byte) (int, error) {
	<-b.closed
	return 0, errClosed
}

func (b *bufConn) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *bufConn) Close() error {
	b.once.Do(func() { close(b.closed) })
	return nil
}

func (b *bufConn) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *bufConn) LocalAddr() net.Addr                { return fakeAddr{} }
func (b *bufConn) RemoteAddr() net.Addr               { return fakeAddr{} }
func (b *bufConn) SetDeadline(_ time.Time) error      { return nil }
func (b *bufConn) SetReadDeadline(_ time.Time) error  { return nil }
func (b *bufConn) SetWriteDeadline(_ time.Time) error { return nil }

type fakeAddr struct{}

func (fakeAddr) Network() string { return "fake" }
func (fakeAddr) String() string  { return "fake:0" }

var errClosed = errors.New("buf conn closed")

func bufSession(t *testing.T) (*telnet.Session, *bufConn) {
	t.Helper()
	c := newBufConn()
	s := telnet.NewSession(c)
	if s == nil {
		t.Fatal("NewSession returned nil")
	}
	t.Cleanup(func() { c.Close() })
	return s, c
}

func seedWorld(t *testing.T) (*repo.MemoryRoomRepo, *repo.MemoryExitRepo, *repo.MemoryItemRepo, *repo.MemoryMobRepo) {
	t.Helper()
	rooms := repo.NewMemoryRoomRepo()
	exits := repo.NewMemoryExitRepo()
	items := repo.NewMemoryItemRepo()
	mobs := repo.NewMemoryMobRepo()
	rooms.Insert(repo.Room{ID: 1, Name: "Town Plaza", LongDesc: "Cobblestones radiate outward."})
	rooms.Insert(repo.Room{ID: 2, Name: "North Road", LongDesc: "A quieter road."})
	exits.Insert(repo.Exit{FromRoomID: 1, ToRoomID: 2, Direction: repo.DirNorth})
	exits.Insert(repo.Exit{FromRoomID: 2, ToRoomID: 1, Direction: repo.DirSouth})
	items.Insert(repo.Item{Name: "a small pebble", RoomID: 1})
	mobs.Insert(repo.Mob{Name: "a town crier", RoomID: 1})
	return rooms, exits, items, mobs
}

func TestLook_RendersRoomWithExitsItemsAndMobs(t *testing.T) {
	rooms, exits, items, mobs := seedWorld(t)
	s, conn := bufSession(t)
	s.CurrentRoomID = 1

	if err := RenderRoom(context.Background(), s, rooms, exits, items, mobs); err != nil {
		t.Fatalf("RenderRoom: %v", err)
	}

	got := conn.String()
	for _, want := range []string{
		"Town Plaza",
		"Cobblestones radiate outward.",
		"Exits: north",
		"You see: a small pebble",
		"Also here: a town crier",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q.\nGot:\n%s", want, got)
		}
	}
}

func TestLook_OmitsEmptySubsections(t *testing.T) {
	rooms := repo.NewMemoryRoomRepo()
	rooms.Insert(repo.Room{ID: 7, Name: "Empty Hall", LongDesc: "A bare stone hall."})
	exits := repo.NewMemoryExitRepo()
	items := repo.NewMemoryItemRepo()
	mobs := repo.NewMemoryMobRepo()

	s, conn := bufSession(t)
	s.CurrentRoomID = 7

	if err := RenderRoom(context.Background(), s, rooms, exits, items, mobs); err != nil {
		t.Fatalf("RenderRoom: %v", err)
	}
	got := conn.String()
	if !strings.Contains(got, "Empty Hall") {
		t.Fatalf("missing room name; got %q", got)
	}
	if !strings.Contains(got, "Exits: none") {
		t.Fatalf("expected 'Exits: none' in empty hall; got %q", got)
	}
	if strings.Contains(got, "You see:") {
		t.Fatalf("expected no 'You see:' line; got %q", got)
	}
	if strings.Contains(got, "Also here:") {
		t.Fatalf("expected no 'Also here:' line; got %q", got)
	}
}

func TestLook_MissingRoomMessage(t *testing.T) {
	rooms := repo.NewMemoryRoomRepo()
	exits := repo.NewMemoryExitRepo()
	items := repo.NewMemoryItemRepo()
	mobs := repo.NewMemoryMobRepo()

	s, conn := bufSession(t)
	s.CurrentRoomID = 999

	if err := RenderRoom(context.Background(), s, rooms, exits, items, mobs); err != nil {
		t.Fatalf("RenderRoom: %v", err)
	}
	got := conn.String()
	if !strings.Contains(got, "gone missing") {
		t.Fatalf("expected missing-room message; got %q", got)
	}
}
