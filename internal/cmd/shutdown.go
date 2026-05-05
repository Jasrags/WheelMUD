package cmd

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Jasrags/WheelMUD/telnet"
)

// ShutdownController is the small contract the shutdown / reboot
// admin commands need to drive the server's lifecycle. main.go's
// *server satisfies this; the cmd package stays free of the main-
// package import.
type ShutdownController interface {
	RequestShutdown(reason string, delay time.Duration) error
	RequestReboot(reason string, delay time.Duration) error
	RequestAbort() error
}

// ErrShutdownPending is returned when a second shutdown / reboot is
// requested before the first one has fired or been aborted.
var ErrShutdownPending = errors.New("shutdown already pending")

const (
	defaultShutdownDelay = 30 * time.Second
	maxShutdownDelay     = 1 * time.Hour
)

// NewShutdown builds the `shutdown` admin verb. Forms:
//
//	shutdown                       — 30s default, no reason
//	shutdown <delay>               — delay (bare seconds or 30s/2m/1h)
//	shutdown <delay> <reason...>   — delay + reason
//	shutdown <reason...>           — 30s, reason
//	shutdown cancel | abort        — cancel an in-flight countdown
func NewShutdown(ctrl ShutdownController) *telnet.Command {
	return &telnet.Command{
		Name: "shutdown",
		Help: "shutdown [<delay>] [<reason>] | shutdown cancel — bring the server down",
		Long: "Usage: shutdown                       - 30s countdown, no reason\n" +
			"       shutdown <delay>               - <delay> in seconds (60) or duration (2m, 1h)\n" +
			"       shutdown <delay> <reason...>   - announce <reason> during countdown\n" +
			"       shutdown <reason...>           - 30s default delay\n" +
			"       shutdown cancel|abort          - cancel an in-flight countdown\n\n" +
			"Drains active sessions, flushes the autosave manager, and exits.\n" +
			"Delay is clamped to [0, 1h].",
		Auth: telnet.AuthAdmin,
		Run: func(c *telnet.Context) error {
			return runShutdownLike(c, ctrl, false)
		},
	}
}

// NewReboot builds the `reboot` admin verb. Same arg shape as
// `shutdown`; on a Linux/macOS host the server re-execs itself after
// drain + flush.
func NewReboot(ctrl ShutdownController) *telnet.Command {
	return &telnet.Command{
		Name: "reboot",
		Help: "reboot [<delay>] [<reason>] | reboot cancel — bring the server down and re-launch",
		Long: "Usage: reboot                       - 30s countdown, no reason\n" +
			"       reboot <delay>               - <delay> in seconds (60) or duration (2m, 1h)\n" +
			"       reboot <delay> <reason...>   - announce <reason> during countdown\n" +
			"       reboot <reason...>           - 30s default delay\n" +
			"       reboot cancel|abort          - cancel an in-flight countdown\n\n" +
			"Same drain/flush path as shutdown, then re-execs the server binary.\n" +
			"Delay is clamped to [0, 1h]. Re-exec is POSIX-only.",
		Auth: telnet.AuthAdmin,
		Run: func(c *telnet.Context) error {
			return runShutdownLike(c, ctrl, true)
		},
	}
}

func runShutdownLike(c *telnet.Context, ctrl ShutdownController, reboot bool) error {
	verb := "shutdown"
	if reboot {
		verb = "reboot"
	}
	if len(c.Args) > 0 {
		first := strings.ToLower(c.Args[0])
		if first == "cancel" || first == "abort" {
			if err := ctrl.RequestAbort(); err != nil {
				return c.Session.WriteString("{{" + verb + ": " + sanitizeArg(err.Error()) + "}}::yellow\r\n")
			}
			return c.Session.WriteString("{{" + verb + " cancelled.}}::green\r\n")
		}
	}

	delay, reason := parseDelayAndReason(c.Args)
	// Defang at the boundary so any future caller of the
	// ShutdownController contract also gets a cfmt-safe reason
	// before it lands in broadcast templates.
	reason = defangWorldField(reason)

	var err error
	if reboot {
		err = ctrl.RequestReboot(reason, delay)
	} else {
		err = ctrl.RequestShutdown(reason, delay)
	}
	if err != nil {
		return c.Session.WriteString("{{" + verb + ": " + sanitizeArg(err.Error()) + "}}::red\r\n")
	}

	msg := fmt.Sprintf("{{%s scheduled in %s.}}::green\r\n", verb, formatDelay(delay))
	if reason != "" {
		msg = fmt.Sprintf("{{%s scheduled in %s: %s}}::green\r\n", verb, formatDelay(delay), reason)
	}
	return c.Session.WriteString(msg)
}

// parseDelayAndReason splits args into a clamped delay and a free-text
// reason. The first arg is treated as a delay if and only if it parses
// as an integer (seconds) or a Go duration; otherwise the entire arg
// list becomes the reason and the delay falls back to the default.
func parseDelayAndReason(args []string) (time.Duration, string) {
	if len(args) == 0 {
		return defaultShutdownDelay, ""
	}
	if d, ok := tryParseDelay(args[0]); ok {
		return clampDelay(d), strings.Join(args[1:], " ")
	}
	return defaultShutdownDelay, strings.Join(args, " ")
}

func tryParseDelay(s string) (time.Duration, bool) {
	if n, err := strconv.Atoi(s); err == nil {
		return time.Duration(n) * time.Second, true
	}
	if d, err := time.ParseDuration(s); err == nil {
		return d, true
	}
	return 0, false
}

func clampDelay(d time.Duration) time.Duration {
	if d < 0 {
		return 0
	}
	if d > maxShutdownDelay {
		return maxShutdownDelay
	}
	return d
}

func formatDelay(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	// Round to whole seconds; players don't care about ms.
	return d.Round(time.Second).String()
}
