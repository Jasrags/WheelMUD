//go:build integration

// Package integration spins up a real wheelmud-server subprocess
// against a temporary DB + on-disk config and exposes helpers for
// driving the telnet protocol from tests.
//
// Build-tag-gated so `go test ./...` stays fast; CI runs
// `go test -tags=integration ./test/integration/...`.
package integration

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// boundaryPort picks an unused TCP port the OS allocates for us via a
// bind-and-close. Used so concurrent test runs don't collide on a
// hardcoded port.
func boundaryPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for ephemeral port: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// ServerHandle wraps a running subprocess and the addresses it bound.
type ServerHandle struct {
	TelnetAddr  string
	MetricsAddr string
	cmd         *exec.Cmd
	logPath     string
	logFile     *os.File
}

// Stop sends SIGTERM, waits for the subprocess to exit, then closes
// the captured log file. Closing here (vs. via t.Cleanup) keeps the
// fd alive until cmd.Wait returns so the subprocess can't get
// EPIPE/EBADF mid-drain. Test failures dump the captured stdout/
// stderr from the server log so failures surface the server-side
// reason.
func (h *ServerHandle) Stop(t *testing.T) {
	t.Helper()
	defer func() {
		if h.logFile != nil {
			_ = h.logFile.Close()
		}
	}()
	if h.cmd == nil || h.cmd.Process == nil {
		return
	}
	_ = h.cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan error, 1)
	go func() { done <- h.cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil && !isExpectedExit(err) {
			h.dumpLog(t)
			t.Errorf("server subprocess exited with error: %v", err)
		}
	case <-time.After(10 * time.Second):
		_ = h.cmd.Process.Kill()
		h.dumpLog(t)
		t.Error("server subprocess did not exit within 10s of SIGTERM")
	}
}

// LogPath returns the captured stdout/stderr log file path; tests
// failing mid-flight call dumpLog to print the tail.
func (h *ServerHandle) LogPath() string { return h.logPath }

func (h *ServerHandle) dumpLog(t *testing.T) {
	t.Helper()
	if h.logPath == "" {
		return
	}
	data, err := os.ReadFile(h.logPath)
	if err != nil {
		t.Logf("could not read server log %q: %v", h.logPath, err)
		return
	}
	t.Logf("server log (%s):\n%s", h.logPath, string(data))
}

// isExpectedExit reports whether err looks like a clean SIGTERM
// teardown vs. a real crash.
func isExpectedExit(err error) bool {
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		return false
	}
	// signal.Notify catches SIGTERM and runs an ordered drain that
	// exits 0. But on some platforms a kill -TERM during early init
	// before the handler installs surfaces a non-zero exit; treat
	// both as expected here.
	if exit.ExitCode() == 0 {
		return true
	}
	// 130 = SIGINT, 143 = SIGTERM in shell convention.
	switch exit.ExitCode() {
	case 130, 143:
		return true
	}
	return false
}

// StartServer compiles cmd/server (if needed) and spawns it against a
// temporary DB pointing at the real ./data/world tree. Returns once
// /healthz responds 200 (signaling the listener is bound). Tests
// must call h.Stop in t.Cleanup.
func StartServer(t *testing.T) *ServerHandle {
	t.Helper()

	binPath := buildServerBinary(t)

	tmpDir := t.TempDir()
	dsn := filepath.Join(tmpDir, "wheelmud.db")
	configPath := filepath.Join(tmpDir, "config.yaml")
	logPath := filepath.Join(tmpDir, "server.log")

	telnetPort := boundaryPort(t)
	metricsPort := boundaryPort(t)

	cfg := fmt.Sprintf(`
server:
  listen_addr: "127.0.0.1:%d"
  metrics_addr: "127.0.0.1:%d"
db:
  dsn: %q
log:
  level: "info"
`, telnetPort, metricsPort, dsn)
	if err := os.WriteFile(configPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// WORLD_DIR points at the real tree so the embedded chargen +
	// quest catalogs find their referenced mob templates. Using
	// ./data/world from the repo root means tests must be invoked
	// from the module root (which `go test ./test/integration/...`
	// does automatically).
	worldDir, err := repoPath("data/world")
	if err != nil {
		t.Fatalf("resolve world dir: %v", err)
	}

	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("create server log: %v", err)
	}

	cmd := exec.Command(binPath, "-config", configPath)
	cmd.Env = append(os.Environ(), "WORLD_DIR="+worldDir)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		t.Fatalf("start server: %v", err)
	}

	h := &ServerHandle{
		TelnetAddr:  fmt.Sprintf("127.0.0.1:%d", telnetPort),
		MetricsAddr: fmt.Sprintf("127.0.0.1:%d", metricsPort),
		cmd:         cmd,
		logPath:     logPath,
		logFile:     logFile,
	}

	// Wait for healthz to flip 200, signaling both the metrics
	// listener AND the telnet listener are bound (SetReady(true)
	// runs immediately after the telnet bind).
	if err := waitForHealthz(h.MetricsAddr, 15*time.Second); err != nil {
		h.dumpLog(t)
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("server never reported healthy: %v", err)
	}
	return h
}

func waitForHealthz(addr string, timeout time.Duration) error {
	url := "http://" + addr + "/healthz"
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 1 * time.Second}
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("healthz did not return 200 within %v", timeout)
}

// cachedBinary holds the path to the pre-built server binary so
// subsequent tests in the same run reuse it. cachedBinaryMu guards
// the build to defeat the race where parallel tests both observe
// the empty string and both spawn `go build`. cachedBinaryDir is the
// containing temp directory; the first builder registers a cleanup
// to remove it at the end of its t (which still outlives every
// subsequent test that reuses the cached binary, since the cleanup
// only fires once the test that registered it has fully torn down).
var (
	cachedBinaryMu  sync.Mutex
	cachedBinary    string
	cachedBinaryDir string
)

func buildServerBinary(t *testing.T) string {
	t.Helper()
	cachedBinaryMu.Lock()
	defer cachedBinaryMu.Unlock()
	if cachedBinary != "" {
		return cachedBinary
	}
	root, err := repoPath("")
	if err != nil {
		t.Fatalf("locate repo root: %v", err)
	}
	binDir, err := os.MkdirTemp("", "wheelmud-integ-bin-")
	if err != nil {
		t.Fatalf("temp bin dir: %v", err)
	}
	binPath := filepath.Join(binDir, "server")
	if runtime.GOOS == "windows" {
		binPath += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/server")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.RemoveAll(binDir)
		t.Fatalf("go build cmd/server: %v\n%s", err, out)
	}
	cachedBinary = binPath
	cachedBinaryDir = binDir
	// Cleanup runs after the test that triggered the build finishes.
	// Subsequent tests reusing the cache hold their own references to
	// the binary path string; the binary file stays on disk until this
	// cleanup fires at parent-test teardown. For `go test ./...` the
	// outer go-test process cleans up its own temp tree on exit too,
	// so a missed Cleanup leaks only one directory per `go test`
	// invocation rather than per integration test.
	t.Cleanup(func() {
		cachedBinaryMu.Lock()
		defer cachedBinaryMu.Unlock()
		if cachedBinaryDir != "" {
			_ = os.RemoveAll(cachedBinaryDir)
			cachedBinary = ""
			cachedBinaryDir = ""
		}
	})
	return binPath
}

// repoPath returns the absolute path of rel relative to the module
// root. Walks up from this file's location until it finds go.mod.
func repoPath(rel string) (string, error) {
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("runtime.Caller failed")
	}
	dir := filepath.Dir(here)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, rel), nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("go.mod not found walking up from " + here)
		}
		dir = parent
	}
}

// TelnetClient wraps a connection with read helpers tuned for the
// integration smoke. ReadUntil reads bytes until the substring `want`
// appears (case-insensitive, post-IAC-strip) or `timeout` elapses.
type TelnetClient struct {
	conn net.Conn
	rd   *bufio.Reader
}

// Dial opens a telnet connection to addr with a 5s timeout.
func Dial(t *testing.T, addr string) *TelnetClient {
	t.Helper()
	c, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}
	return &TelnetClient{conn: c, rd: bufio.NewReader(c)}
}

// Close terminates the connection.
func (c *TelnetClient) Close() { _ = c.conn.Close() }

// ReadUntil reads bytes, stripping IAC sequences inline, until `want`
// appears in the accumulated text (case-insensitive) or timeout
// elapses. Returns the full captured text for assertion diagnostics.
func (c *TelnetClient) ReadUntil(t *testing.T, want string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var buf bytes.Buffer
	wantLower := strings.ToLower(want)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatalf("ReadUntil(%q) timeout; got:\n%s", want, buf.String())
		}
		_ = c.conn.SetReadDeadline(time.Now().Add(remaining))
		b, err := c.rd.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) || strings.Contains(err.Error(), "i/o timeout") {
				t.Fatalf("ReadUntil(%q): %v; got:\n%s", want, err, buf.String())
			}
			t.Fatalf("ReadUntil read: %v; got:\n%s", err, buf.String())
		}
		// IAC = 0xFF: strip the IAC sequence so it doesn't contaminate
		// the text buffer. Strip 2 trailing bytes for WILL/WONT/DO/DONT
		// (option byte follows). SB sequences end with IAC SE — strip
		// everything up to and including SE.
		if b == 0xFF {
			c.skipIAC(t)
			continue
		}
		buf.WriteByte(b)
		if strings.Contains(strings.ToLower(buf.String()), wantLower) {
			return buf.String()
		}
	}
}

func (c *TelnetClient) skipIAC(t *testing.T) {
	t.Helper()
	cmd, err := c.rd.ReadByte()
	if err != nil {
		return
	}
	switch cmd {
	case 0xFB, 0xFC, 0xFD, 0xFE: // WILL / WONT / DO / DONT
		_, _ = c.rd.ReadByte()
	case 0xFA: // SB — read until IAC SE
		for {
			b, err := c.rd.ReadByte()
			if err != nil {
				return
			}
			if b != 0xFF {
				continue
			}
			next, err := c.rd.ReadByte()
			if err != nil {
				return
			}
			if next == 0xF0 { // SE
				return
			}
		}
	}
}

// Write sends s as raw bytes to the server.
func (c *TelnetClient) Write(t *testing.T, s string) {
	t.Helper()
	_ = c.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	if _, err := c.conn.Write([]byte(s)); err != nil {
		t.Fatalf("write %q: %v", s, err)
	}
}

// HealthCheck issues a GET /healthz against the metrics addr and
// returns the response status code.
func (h *ServerHandle) HealthCheck(t *testing.T) int {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://" + h.MetricsAddr + "/healthz")
	if err != nil {
		t.Fatalf("healthz GET: %v", err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

// MetricsBody returns the /metrics endpoint body for assertions on
// series names. Errors fatal the test.
func (h *ServerHandle) MetricsBody(t *testing.T) string {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://" + h.MetricsAddr + "/metrics")
	if err != nil {
		t.Fatalf("metrics GET: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read metrics body: %v", err)
	}
	return string(body)
}

// CtxWithBudget returns a context that auto-cancels after the
// per-test integration budget (15s) so a wedged subprocess can't
// hang the whole suite.
func CtxWithBudget(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	return ctx
}

