// Package testhelper exposes shared building blocks for tests in
// other packages — most notably BufConn, an in-memory net.Conn
// implementation used to inspect everything a telnet.Session writes
// without dealing with net.Pipe's synchronous-read scheduling.
//
// Three packages used to carry near-identical local copies of this
// helper (internal/cmd, internal/mob, internal/world); diverging
// drift across them was tracked in world_aggregates_followups.md
// before being consolidated here.
package testhelper

import (
	"bytes"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/telnet"
)

// errBufClosed is the EOF-shaped sentinel returned by BufConn.Read
// after Close. Callers shouldn't depend on it; the value is here so
// the read path can give a stable error rather than block forever.
var errBufClosed = errors.New("testhelper: buf conn closed")

// BufConn satisfies net.Conn against an in-memory buffer. Read
// blocks until Close so a Session goroutine reading it can be
// shut down deterministically by the test cleanup.
type BufConn struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	closed chan struct{}
	once   sync.Once
}

// NewBufConn returns a BufConn ready to use. Callers that own the
// lifetime should arrange for Close so any Read goroutine unblocks;
// BufSession does this via t.Cleanup.
func NewBufConn() *BufConn {
	return &BufConn{closed: make(chan struct{})}
}

func (b *BufConn) Read(_ []byte) (int, error) {
	<-b.closed
	return 0, errBufClosed
}

func (b *BufConn) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *BufConn) Close() error {
	b.once.Do(func() { close(b.closed) })
	return nil
}

// String returns everything Session has written so far. Safe to
// call concurrently with Write.
func (b *BufConn) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// Reset clears the captured output. Used by tests that send
// multiple commands and want to assert only on the latest write.
func (b *BufConn) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Reset()
}

func (b *BufConn) LocalAddr() net.Addr                { return fakeAddr{} }
func (b *BufConn) RemoteAddr() net.Addr               { return fakeAddr{} }
func (b *BufConn) SetDeadline(_ time.Time) error      { return nil }
func (b *BufConn) SetReadDeadline(_ time.Time) error  { return nil }
func (b *BufConn) SetWriteDeadline(_ time.Time) error { return nil }

type fakeAddr struct{}

func (fakeAddr) Network() string { return "fake" }
func (fakeAddr) String() string  { return "fake:0" }

// BufSession constructs a telnet.Session backed by a fresh BufConn
// and registers a t.Cleanup hook that closes the conn — that
// unblocks any goroutine the session has parked on Read so the test
// process can exit cleanly. Returns the Session plus the BufConn
// so the test can both drive the session and read what it emitted.
func BufSession(t *testing.T) (*telnet.Session, *BufConn) {
	t.Helper()
	c := NewBufConn()
	s := telnet.NewSession(c)
	if s == nil {
		t.Fatal("NewSession returned nil")
	}
	t.Cleanup(func() { c.Close() })
	return s, c
}
