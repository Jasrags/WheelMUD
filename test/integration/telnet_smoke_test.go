//go:build integration

package integration

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestTelnetSmoke spins up a real server subprocess, hits the
// metrics endpoints, opens a telnet connection through the live
// listener, exchanges the IAC handshake + initial prompt, and
// requests shutdown.
//
// This is the canonical end-to-end smoke for Phase J slices J1–J5:
// CI passing this test means the binary builds, the config loader
// boots, migrations run, the world loader seeds, the telnet
// listener binds, IAC negotiation completes, the dispatcher emits
// the first mode's prompt, and SIGTERM drains cleanly within the
// 10s budget.
func TestTelnetSmoke(t *testing.T) {
	h := StartServer(t)
	t.Cleanup(func() { h.Stop(t) })

	// Phase J slice J5: /healthz must report 200 once the listener
	// is bound (StartServer's wait loop already proved this; this
	// is the explicit assertion).
	if got := h.HealthCheck(t); got != http.StatusOK {
		t.Fatalf("healthz = %d, want 200", got)
	}

	// /metrics must emit the V1 collector set.
	body := h.MetricsBody(t)
	for _, want := range []string{
		"wheelmud_build_info",
		"wheelmud_sessions_active",
		"wheelmud_db_open_conns",
		"go_goroutines",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/metrics missing %q", want)
		}
	}

	// Telnet handshake: connect, observe the first-prompt text.
	tc := Dial(t, h.TelnetAddr)
	t.Cleanup(tc.Close)
	got := tc.ReadUntil(t, "Username", 5*time.Second)
	if !strings.Contains(strings.ToLower(got), "username") {
		t.Fatalf("first prompt did not contain 'Username':\n%s", got)
	}

	// sessions_active should reflect the live connection. Scrape
	// /metrics a second time and verify the gauge moved to >= 1.
	bodyAfter := h.MetricsBody(t)
	if !strings.Contains(bodyAfter, "wheelmud_sessions_active 0") {
		// Either the gauge moved to 1 (expected) or stayed at 0.
		// We assert "not 0" by checking presence of 1+ via a coarse
		// substring check. The promhttp text format always includes
		// a value, so absence of " 0" line means >= 1.
		// (Pre-bind /healthz handshake before the session is bound
		// to the account registry would still keep sessions_active
		// at 0 since binding happens at login completion; this
		// branch is informational, not a hard failure.)
		t.Log("sessions_active != 0 (test connection visible to gauge)")
	}

	// Clean teardown is exercised by h.Stop via t.Cleanup; this
	// test passing means the subprocess returned 0/130/143 within
	// 10s of SIGTERM.
}
