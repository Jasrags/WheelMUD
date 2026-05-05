package cmd

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/telnet"
)

type fakeShutdownCtl struct {
	mu       sync.Mutex
	shutdown []shutdownCall
	reboot   []shutdownCall
	aborts   int
	err      error
}

type shutdownCall struct {
	reason string
	delay  time.Duration
}

func (f *fakeShutdownCtl) RequestShutdown(reason string, delay time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.shutdown = append(f.shutdown, shutdownCall{reason, delay})
	return f.err
}

func (f *fakeShutdownCtl) RequestReboot(reason string, delay time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reboot = append(f.reboot, shutdownCall{reason, delay})
	return f.err
}

func (f *fakeShutdownCtl) RequestAbort() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.aborts++
	return f.err
}

func TestParseDelayAndReason(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		wantDelay time.Duration
		wantMsg   string
	}{
		{"empty", nil, defaultShutdownDelay, ""},
		{"bare seconds", []string{"60"}, 60 * time.Second, ""},
		{"duration", []string{"2m"}, 2 * time.Minute, ""},
		{"seconds + reason", []string{"45", "going", "down"}, 45 * time.Second, "going down"},
		{"duration + reason", []string{"5m", "upgrades"}, 5 * time.Minute, "upgrades"},
		{"reason only", []string{"maintenance", "window"}, defaultShutdownDelay, "maintenance window"},
		{"clamp upper", []string{"99999"}, maxShutdownDelay, ""},
		{"clamp negative", []string{"-30"}, 0, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, m := parseDelayAndReason(tc.args)
			if d != tc.wantDelay {
				t.Errorf("delay: got %v, want %v", d, tc.wantDelay)
			}
			if m != tc.wantMsg {
				t.Errorf("reason: got %q, want %q", m, tc.wantMsg)
			}
		})
	}
}

func TestShutdown_NoArgsUsesDefault(t *testing.T) {
	s, _ := bufSession(t)
	s.AuthLevel = telnet.AuthAdmin
	ctl := &fakeShutdownCtl{}
	cmd := NewShutdown(ctl)

	runCmd(t, cmd, s, "")

	if len(ctl.shutdown) != 1 {
		t.Fatalf("expected 1 shutdown call, got %d", len(ctl.shutdown))
	}
	if got := ctl.shutdown[0]; got.delay != defaultShutdownDelay || got.reason != "" {
		t.Errorf("unexpected call: %+v", got)
	}
}

func TestShutdown_DelayAndReason(t *testing.T) {
	s, out := bufSession(t)
	s.AuthLevel = telnet.AuthAdmin
	ctl := &fakeShutdownCtl{}
	cmd := NewShutdown(ctl)

	runCmd(t, cmd, s, "2m maintenance window")

	if len(ctl.shutdown) != 1 {
		t.Fatalf("expected 1 shutdown call, got %d", len(ctl.shutdown))
	}
	got := ctl.shutdown[0]
	if got.delay != 2*time.Minute {
		t.Errorf("delay: got %v, want 2m", got.delay)
	}
	if got.reason != "maintenance window" {
		t.Errorf("reason: got %q", got.reason)
	}
	if !strings.Contains(out.String(), "maintenance window") {
		t.Errorf("ack missing reason: %q", out.String())
	}
	if !strings.Contains(out.String(), "2m") {
		t.Errorf("ack missing delay: %q", out.String())
	}
}

func TestShutdown_Cancel(t *testing.T) {
	s, out := bufSession(t)
	s.AuthLevel = telnet.AuthAdmin
	ctl := &fakeShutdownCtl{}
	cmd := NewShutdown(ctl)

	runCmd(t, cmd, s, "cancel")
	runCmd(t, cmd, s, "abort")

	if ctl.aborts != 2 {
		t.Errorf("expected 2 aborts, got %d", ctl.aborts)
	}
	if len(ctl.shutdown) != 0 {
		t.Errorf("cancel must not call RequestShutdown: %+v", ctl.shutdown)
	}
	if !strings.Contains(out.String(), "cancelled") {
		t.Errorf("missing cancel ack: %q", out.String())
	}
}

func TestReboot_DelayAndReason(t *testing.T) {
	s, out := bufSession(t)
	s.AuthLevel = telnet.AuthAdmin
	ctl := &fakeShutdownCtl{}
	cmd := NewReboot(ctl)

	runCmd(t, cmd, s, "5m upgrades")

	if len(ctl.reboot) != 1 {
		t.Fatalf("expected 1 reboot call, got %d", len(ctl.reboot))
	}
	if got := ctl.reboot[0]; got.delay != 5*time.Minute || got.reason != "upgrades" {
		t.Errorf("unexpected call: %+v", got)
	}
	if !strings.Contains(out.String(), "reboot scheduled") {
		t.Errorf("ack wrong verb: %q", out.String())
	}
}

func TestShutdown_PendingErrorSurfaces(t *testing.T) {
	s, out := bufSession(t)
	s.AuthLevel = telnet.AuthAdmin
	ctl := &fakeShutdownCtl{err: ErrShutdownPending}
	cmd := NewShutdown(ctl)

	runCmd(t, cmd, s, "60")

	if !strings.Contains(out.String(), "shutdown already pending") {
		t.Errorf("expected pending error in ack: %q", out.String())
	}
}

func TestShutdown_AbortErrorSurfaces(t *testing.T) {
	s, out := bufSession(t)
	s.AuthLevel = telnet.AuthAdmin
	ctl := &fakeShutdownCtl{err: errors.New("nothing pending")}
	cmd := NewShutdown(ctl)

	runCmd(t, cmd, s, "cancel")

	if !strings.Contains(out.String(), "nothing pending") {
		t.Errorf("expected error surfaced: %q", out.String())
	}
}
