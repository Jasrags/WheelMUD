package mode

import (
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/telnet"
)

// stubMode is a no-op telnet.Mode used as the post-login replacement
// target so tests can detect that ReplaceMode happened.
type stubMode struct{ name string }

func (m *stubMode) Handle(_ context.Context, _ *telnet.Session, _ string) error { return nil }
func (m *stubMode) Prompt(_ context.Context, _ *telnet.Session) string                             { return m.name + "> " }
func (m *stubMode) OnEnter(_ *telnet.Session) error                             { return nil }
func (m *stubMode) OnExit(_ *telnet.Session) error                              { return nil }

// safeBuf is a strings.Builder guarded by a mutex so the drain
// goroutine and the test assertion can both touch it without racing.
type safeBuf struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *safeBuf) write(p []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Write(p)
}

func (b *safeBuf) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *safeBuf) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Reset()
}

// drainPeer continuously reads from peer and appends to dst. Returns
// when the peer is closed; cleanup is the caller's responsibility.
func drainPeer(t *testing.T, peer net.Conn, dst *safeBuf) {
	t.Helper()
	go func() {
		buf := make([]byte, 1024)
		for {
			_ = peer.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			n, err := peer.Read(buf)
			if n > 0 {
				dst.write(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()
}
