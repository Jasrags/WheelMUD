package mode

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/Jasrags/WheelMUD/internal/audit"
	"github.com/Jasrags/WheelMUD/internal/display"
	"github.com/Jasrags/WheelMUD/internal/prompt"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// settingsWidthMin / settingsWidthMax bound the WidthOverride menu
// validator. The floor mirrors telnet/wrap.go's wrap minimum; the
// ceiling is a soft cap.
const (
	settingsWidthMin = 40
	settingsWidthMax = 200
)

// handleSettingsEnter pivots to the settings root and prints the
// numbered key list. The settings copy on the AccountMenu is the
// source of truth for display; postAuth loaded it once and the menu
// mutates in place on each successful drilldown.
func (m *AccountMenu) handleSettingsEnter(s *telnet.Session) error {
	m.step = accountStepSettingsRoot
	return m.writeSettingsRoot(s)
}

func (m *AccountMenu) writeSettingsRoot(s *telnet.Session) error {
	if err := display.SectionHeader(s, "Settings"); err != nil {
		return err
	}
	rows := []struct {
		label string
		value string
	}{
		{"Color", m.colorDisplay()},
		{"Prompt", m.promptDisplay()},
		{"Width", m.widthDisplay()},
		{"Locale", m.localeDisplay()},
		{"MOTD replay", m.motdDisplay()},
	}
	for i, r := range rows {
		if err := s.WriteRaw([]byte(fmt.Sprintf(
			"  %d) %-12s %s\r\n", i+1, r.label, r.value))); err != nil {
			return err
		}
	}
	if err := s.WriteRaw([]byte("\r\n  [B]ack\r\n")); err != nil {
		return err
	}
	return s.WriteRaw([]byte(fmt.Sprintf("[1-%d] >\r\n", len(rows))))
}

func (m *AccountMenu) handleSettingsRootLine(s *telnet.Session, line string) error {
	if isBack(line) {
		return m.returnToRoot(s)
	}
	idx, err := parsePositiveIndex(strings.TrimSpace(line), 5)
	if err != nil {
		if werr := writeError(s, "Invalid choice. [1-5] or [B]ack."); werr != nil {
			return werr
		}
		return nil
	}
	switch idx {
	case 0:
		m.step = accountStepSettingsColor
		return m.writeSettingsColor(s)
	case 1:
		m.step = accountStepSettingsPrompt
		return m.writeSettingsPrompt(s)
	case 2:
		m.step = accountStepSettingsWidth
		return m.writeSettingsWidth(s)
	case 3:
		m.step = accountStepSettingsLocale
		return m.writeSettingsLocale(s)
	case 4:
		m.step = accountStepSettingsMOTD
		return m.writeSettingsMOTD(s)
	}
	return nil
}

// ─── Color ───────────────────────────────────────────────────────────

// colorOption maps a display number to the value persisted on
// AccountSettings.ColorOverride. Index 0 ("Auto / clear override")
// stores the empty string — apply paths read that as "use TERM-detected
// level".
var colorOptions = []struct {
	label string
	value string
}{
	{"Auto / clear override", ""},
	{"None  (no ANSI)", "none"},
	{"Basic (8 colors)", "basic"},
	{"16 colors", "16"},
	{"256 colors", "256"},
	{"Truecolor", "truecolor"},
}

func (m *AccountMenu) writeSettingsColor(s *telnet.Session) error {
	if err := display.SectionHeader(s, "Settings · Color"); err != nil {
		return err
	}
	if err := s.WriteRaw([]byte("  Override TERM-detected color level.\r\n\r\n")); err != nil {
		return err
	}
	for i, opt := range colorOptions {
		if err := s.WriteRaw([]byte(fmt.Sprintf(
			"    %d) %s\r\n", i+1, opt.label))); err != nil {
			return err
		}
	}
	if err := s.WriteRaw([]byte("\r\n  [B]ack\r\n")); err != nil {
		return err
	}
	return s.WriteRaw([]byte(fmt.Sprintf("[1-%d] >\r\n", len(colorOptions))))
}

func (m *AccountMenu) handleSettingsColorLine(ctx context.Context, s *telnet.Session, line string) error {
	if isBack(line) {
		m.step = accountStepSettingsRoot
		return m.writeSettingsRoot(s)
	}
	idx, err := parsePositiveIndex(strings.TrimSpace(line), len(colorOptions))
	if err != nil {
		if werr := writeError(s, fmt.Sprintf(
			"Invalid choice. [1-%d] or [B]ack.", len(colorOptions))); werr != nil {
			return werr
		}
		return nil
	}
	next := m.settings
	v := colorOptions[idx].value
	if v == "" {
		next.ColorOverride = ""
	} else {
		level, ok := telnet.ParseColorLevel(v)
		if !ok {
			// Defensive: colorOptions table drift. Reject.
			return writeError(s, "Bad color choice.")
		}
		next.ColorOverride = telnet.ColorLevelName(level)
	}
	if err := m.persistSettings(ctx, s, "color", next); err != nil {
		return err
	}
	m.step = accountStepSettingsRoot
	return m.writeSettingsRoot(s)
}

// ─── Prompt ──────────────────────────────────────────────────────────

func (m *AccountMenu) writeSettingsPrompt(s *telnet.Session) error {
	if err := display.SectionHeader(s, "Settings · Prompt"); err != nil {
		return err
	}
	body := "  Default prompt template stamped onto new characters at chargen.\r\n" +
		"  cfmt tokens accepted. Examples:\r\n" +
		"    \"<%h/%H hp> \"          health bar\r\n" +
		"    \"%n@%r> \"              name@room\r\n\r\n" +
		"  Current: " + m.promptDisplay() + "\r\n\r\n" +
		"  Enter new template, \"clear\" to reset, or [B]ack:\r\n"
	return s.WriteRaw([]byte(body))
}

func (m *AccountMenu) handleSettingsPromptLine(ctx context.Context, s *telnet.Session, line string) error {
	if isBack(line) {
		m.step = accountStepSettingsRoot
		return m.writeSettingsRoot(s)
	}
	in := line // do NOT TrimSpace — quoted templates carry trailing space intentionally
	if strings.EqualFold(strings.TrimSpace(in), "clear") {
		next := m.settings
		next.PromptDefault = ""
		if err := m.persistSettings(ctx, s, "prompt", next); err != nil {
			return err
		}
		m.step = accountStepSettingsRoot
		return m.writeSettingsRoot(s)
	}
	v := unquoteSetting(in)
	clean, ok := prompt.SanitizeTemplate(v)
	if !ok {
		if werr := writeError(s, "Invalid prompt template (empty, too long, or contains control characters)."); werr != nil {
			return werr
		}
		return nil
	}
	next := m.settings
	next.PromptDefault = clean
	if err := m.persistSettings(ctx, s, "prompt", next); err != nil {
		return err
	}
	m.step = accountStepSettingsRoot
	return m.writeSettingsRoot(s)
}

// ─── Width ───────────────────────────────────────────────────────────

func (m *AccountMenu) writeSettingsWidth(s *telnet.Session) error {
	if err := display.SectionHeader(s, "Settings · Width"); err != nil {
		return err
	}
	body := fmt.Sprintf(
		"  Override NAWS terminal width. Range %d–%d.\r\n"+
			"  Current: %s\r\n\r\n"+
			"  Enter width, \"clear\" to reset, or [B]ack:\r\n",
		settingsWidthMin, settingsWidthMax, m.widthDisplay())
	return s.WriteRaw([]byte(body))
}

func (m *AccountMenu) handleSettingsWidthLine(ctx context.Context, s *telnet.Session, line string) error {
	in := strings.TrimSpace(line)
	if isBack(in) {
		m.step = accountStepSettingsRoot
		return m.writeSettingsRoot(s)
	}
	if strings.EqualFold(in, "clear") {
		next := m.settings
		next.WidthOverride = 0
		if err := m.persistSettings(ctx, s, "width", next); err != nil {
			return err
		}
		m.step = accountStepSettingsRoot
		return m.writeSettingsRoot(s)
	}
	w, err := strconv.Atoi(in)
	if err != nil || w < settingsWidthMin || w > settingsWidthMax {
		if werr := writeError(s, fmt.Sprintf(
			"Bad width: integer in [%d, %d] (or 'clear' / [B]ack).",
			settingsWidthMin, settingsWidthMax)); werr != nil {
			return werr
		}
		return nil
	}
	next := m.settings
	next.WidthOverride = w
	if err := m.persistSettings(ctx, s, "width", next); err != nil {
		return err
	}
	m.step = accountStepSettingsRoot
	return m.writeSettingsRoot(s)
}

// ─── Locale ──────────────────────────────────────────────────────────

func (m *AccountMenu) writeSettingsLocale(s *telnet.Session) error {
	if err := display.SectionHeader(s, "Settings · Locale"); err != nil {
		return err
	}
	body := "  IANA timezone for date display (e.g. America/New_York, UTC).\r\n" +
		"  Current: " + m.localeDisplay() + "\r\n\r\n" +
		"  Enter zone, \"clear\" to reset, or [B]ack:\r\n"
	return s.WriteRaw([]byte(body))
}

func (m *AccountMenu) handleSettingsLocaleLine(ctx context.Context, s *telnet.Session, line string) error {
	in := strings.TrimSpace(line)
	if isBack(in) {
		m.step = accountStepSettingsRoot
		return m.writeSettingsRoot(s)
	}
	if strings.EqualFold(in, "clear") {
		next := m.settings
		next.Locale = ""
		if err := m.persistSettings(ctx, s, "locale", next); err != nil {
			return err
		}
		m.step = accountStepSettingsRoot
		return m.writeSettingsRoot(s)
	}
	// Reject the "Local" magic token (Go's time.LoadLocation resolves
	// it to the server's system TZ — not an IANA zone) and any
	// path-separator-bearing input.
	if strings.EqualFold(in, "Local") || strings.ContainsAny(in, ".\\") {
		if werr := writeError(s, "Bad locale: use an IANA tz string (e.g. America/New_York, UTC)."); werr != nil {
			return werr
		}
		return nil
	}
	if _, err := time.LoadLocation(in); err != nil {
		if werr := writeError(s, "Bad locale: use an IANA tz string (e.g. America/New_York, UTC)."); werr != nil {
			return werr
		}
		return nil
	}
	next := m.settings
	next.Locale = in
	if err := m.persistSettings(ctx, s, "locale", next); err != nil {
		return err
	}
	m.step = accountStepSettingsRoot
	return m.writeSettingsRoot(s)
}

// ─── MOTD toggle ─────────────────────────────────────────────────────

func (m *AccountMenu) writeSettingsMOTD(s *telnet.Session) error {
	if err := display.SectionHeader(s, "Settings · MOTD replay"); err != nil {
		return err
	}
	body := "  Replay MOTD on every login (regardless of last_news_seen).\r\n\r\n" +
		"    1) On\r\n" +
		"    2) Off (default)\r\n\r\n" +
		"  [B]ack\r\n[1-2] >\r\n"
	return s.WriteRaw([]byte(body))
}

func (m *AccountMenu) handleSettingsMOTDLine(ctx context.Context, s *telnet.Session, line string) error {
	if isBack(line) {
		m.step = accountStepSettingsRoot
		return m.writeSettingsRoot(s)
	}
	idx, err := parsePositiveIndex(strings.TrimSpace(line), 2)
	if err != nil {
		if werr := writeError(s, "Invalid choice. [1-2] or [B]ack."); werr != nil {
			return werr
		}
		return nil
	}
	next := m.settings
	next.MOTDAlways = idx == 0 // 1 → on, 2 → off
	if err := m.persistSettings(ctx, s, "motd", next); err != nil {
		return err
	}
	m.step = accountStepSettingsRoot
	return m.writeSettingsRoot(s)
}

// ─── Display helpers ─────────────────────────────────────────────────

func (m *AccountMenu) colorDisplay() string {
	if m.settings.ColorOverride == "" {
		return "(auto)"
	}
	return m.settings.ColorOverride
}

func (m *AccountMenu) promptDisplay() string {
	if m.settings.PromptDefault == "" {
		return "(server default)"
	}
	return m.settings.PromptDefault
}

func (m *AccountMenu) widthDisplay() string {
	if m.settings.WidthOverride > 0 {
		return strconv.Itoa(m.settings.WidthOverride)
	}
	return "(auto)"
}

func (m *AccountMenu) localeDisplay() string {
	if m.settings.Locale == "" {
		return defaultLocale + " (default)"
	}
	return m.settings.Locale
}

func (m *AccountMenu) motdDisplay() string {
	if m.settings.MOTDAlways {
		return "on"
	}
	return "off"
}

// ─── Persistence ─────────────────────────────────────────────────────

// persistSettings writes the candidate settings through the account
// repo, updates the menu's in-memory copy on success, and records
// one audit row per change. Repo failures leave the in-memory copy
// untouched so the player sees a consistent view on the next render.
func (m *AccountMenu) persistSettings(ctx context.Context, s *telnet.Session, key string, next repo.AccountSettings) error {
	if m.accounts == nil {
		return writeError(s, "Settings are not configured on this server.")
	}
	if err := m.accounts.UpdateSettings(ctx, s.AccountID, next); err != nil {
		slog.Warn("account_menu: update settings failed",
			"account", s.AccountID, "key", key, "err", err)
		return writeError(s, "Could not save setting. Try again later.")
	}
	m.settings = next
	audit.RecordAccount(ctx, m.audits, s.AccountID, m.accountUsername,
		"settings-update", key, "")
	return display.OK(s, "Saved.")
}

// unquoteSetting strips a single matched pair of surrounding double
// or single quotes. Used by the prompt key so trailing whitespace in
// quoted templates ("<%h> ") survives the strings.TrimSpace at the
// dispatcher boundary. Only one layer is stripped — nested quoting is
// not a use case worth supporting here.
func unquoteSetting(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 {
		if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
			return v[1 : len(v)-1]
		}
	}
	return v
}
