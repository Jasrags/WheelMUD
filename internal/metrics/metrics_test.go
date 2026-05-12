package metrics

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/db"
)

func TestHandler_HealthzNotReady(t *testing.T) {
	m := New(Config{})
	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	m.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 when not ready", w.Code)
	}
}

func TestHandler_HealthzReady(t *testing.T) {
	m := New(Config{})
	m.SetReady(true)
	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	m.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 when ready (no db)", w.Code)
	}
	if !strings.Contains(w.Body.String(), "ok") {
		t.Errorf("body = %q, want to contain 'ok'", w.Body.String())
	}
}

func TestHandler_HealthzDBPing(t *testing.T) {
	conn, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	m := New(Config{DB: conn})
	m.SetReady(true)
	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	m.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%q", w.Code, w.Body.String())
	}

	// Close the DB — ping should fail, healthz should flip to 503.
	conn.Close()
	w2 := httptest.NewRecorder()
	m.Handler().ServeHTTP(w2, httptest.NewRequest("GET", "/healthz", nil))
	if w2.Code != http.StatusServiceUnavailable {
		t.Fatalf("post-close status = %d, want 503", w2.Code)
	}
}

func TestHandler_MetricsEndpointEmitsRegisteredMetrics(t *testing.T) {
	m := New(Config{
		BuildInfo:    BuildInfo{Version: "v0.test", Commit: "abc123", Date: "2026-05-12"},
		SessionCount: func() int { return 3 },
	})
	m.ObserveCommand("look", true)
	m.ObserveCommand("look", true)
	m.ObserveCommand("blargle", false)

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	m.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("metrics status = %d", w.Code)
	}
	body := w.Body.String()

	wantSubs := []string{
		`wheelmud_build_info{`,
		`version="v0.test"`,
		`wheelmud_commands_total{result="ok",verb="look"} 2`,
		`wheelmud_commands_total{result="error",verb="blargle"} 1`,
		`wheelmud_sessions_active 3`,
		`go_goroutines`, // from the Go collector
	}
	for _, w := range wantSubs {
		if !strings.Contains(body, w) {
			t.Errorf("metrics body missing %q\n---\n%s", w, body)
		}
	}
}

func TestHandler_PprofIndexResponds(t *testing.T) {
	m := New(Config{})
	req := httptest.NewRequest("GET", "/debug/pprof/", nil)
	w := httptest.NewRecorder()
	m.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("pprof index status = %d", w.Code)
	}
}

func TestObserveCommand_EmptyVerbNoop(t *testing.T) {
	m := New(Config{})
	m.ObserveCommand("", true)
	// Confirm nothing was registered for the empty label.
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	m.Handler().ServeHTTP(w, req)
	body, _ := io.ReadAll(w.Body)
	if strings.Contains(string(body), `wheelmud_commands_total{result="ok",verb=""}`) {
		t.Errorf("empty verb wrote a label series:\n%s", body)
	}
}
