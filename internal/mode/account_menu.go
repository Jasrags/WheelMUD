package mode

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Jasrags/WheelMUD/internal/chargen"
	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/display"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// AccountMenu is the post-authentication hub. Login / Create push it
// after firing the once-per-login MOTD/news block. The UI is a
// numbered-picker hierarchy modelled on classic MUD post-login menus:
// the root shows a numbered list of actions, each option drills into
// a focused sub-page (numbered picker or free-text prompt depending
// on the action), and [B] returns to the parent. There are no typed
// verbs — every input is either a numbered choice, a free-text value
// for screens that ask for one (settings prompt/width/locale, the
// delete typed-name confirm, password fields), or [B]/[Q].
//
// Substep handlers live in sibling files for cohesion:
//
//	account_menu_play.go      — single-char auto-pick + multi-char picker
//	account_menu_delete.go    — picker → typed-name confirm → cascade
//	account_menu_password.go  — current → new → confirm 3-step flow
//	account_menu_settings.go  — settings root + 5 drilldowns
//	account_menu_security.go  — recent logins + active sessions + kick
//	account_menu_news.go      — MOTD/news replay
//	account_menu_quit.go      — Y/N quit confirm
type AccountMenu struct {
	chars    []repo.Character
	repo     repo.CharacterRepo
	items    repo.ItemRepo
	audits   repo.AdminAuditRepo
	accounts repo.AccountRepo
	session  *session.Registry
	game     telnet.Mode
	motd     MOTDFunc
	catalog  *chargen.Catalog
	// accountUsername snapshots the authed username at menu construction
	// time so audit rows attribute to a name without a per-action repo
	// lookup. Mirrors how AdminAuditEntry.ActorName snapshots names.
	accountUsername string
	// lastSeen feeds the news-replay screen so re-rendering shows the
	// same unseen entries the post-login block did. Sourced from the
	// most-recently-played character (chars[0] under ListByAccount's
	// ordering), or the zero value when the account has no characters.
	lastSeen time.Time
	// rootShown latches OnEnter so PushMode → repaint sequences don't
	// double-render the root.
	rootShown bool

	// Substep state. Most steps own a focused render via their entry
	// handler; the handler line dispatcher drives validation and step
	// transitions. Numbered-picker steps redraw their parent on [B].
	step          accountStep
	pendingDelete *repo.Character
	// pendingNewHash carries the bcrypt hash produced from the
	// new-password prompt across to the confirm step. Cleared on
	// completion / cancel / OnExit so plaintext-derived state never
	// outlives the flow.
	pendingNewHash string

	// settings is the slice-3 AccountSettings snapshot loaded by
	// postAuth. The menu mutates it in place when the player edits a
	// key, persists via accounts.UpdateSettings, and forwards it to
	// CharacterCreate (PromptDefault) and promoteToGame
	// (ColorOverride / WidthOverride). Zero value = defaults.
	settings repo.AccountSettings

	// logins is the slice-4 per-account authentication-event log.
	// nil leaves the security view functional but reports an empty
	// history.
	logins repo.AccountLoginRepo
}

// accountStep is the post-auth menu's substep enum. accountStepRoot
// runs the numbered-picker dispatcher; child steps own a focused
// render and a tighter line dispatcher.
type accountStep int

const (
	accountStepRoot accountStep = iota
	accountStepPlayPicker
	accountStepDeletePicker
	accountStepConfirmDelete
	accountStepCurrentPassword
	accountStepNewPassword
	accountStepConfirmNewPassword
	accountStepSettingsRoot
	accountStepSettingsColor
	accountStepSettingsPrompt
	accountStepSettingsWidth
	accountStepSettingsLocale
	accountStepSettingsMOTD
	accountStepSecurity
	accountStepNews
	accountStepQuitConfirm
)

// NewAccountMenu returns an AccountMenu over the given character list.
// chars must be the account's roster as returned by
// CharacterRepo.ListByAccount (ordered by last_played_at desc); chars[0]
// is the auto-pick target and the news-replay watermark source.
func NewAccountMenu(chars []repo.Character, characters repo.CharacterRepo, game telnet.Mode) *AccountMenu {
	m := &AccountMenu{chars: chars, repo: characters, game: game}
	if len(chars) > 0 {
		m.lastSeen = chars[0].LastNewsSeen
	}
	return m
}

// SetMOTD wires the MOTD/news hook used by the news-replay screen.
func (m *AccountMenu) SetMOTD(f MOTDFunc) { m.motd = f }

// SetCatalog forwards the chargen content catalog to the
// CharacterCreate spawned by the "Create a new character" option.
func (m *AccountMenu) SetCatalog(c *chargen.Catalog) { m.catalog = c }

// SetItems wires the item repo used by the delete-character cascade.
// nil leaves the option available but the cascade refuses (fail-loud).
func (m *AccountMenu) SetItems(r repo.ItemRepo) { m.items = r }

// SetAudits wires the admin_audit repo used to record destructive
// account-menu actions. nil silently skips the audit row.
func (m *AccountMenu) SetAudits(r repo.AdminAuditRepo) { m.audits = r }

// SetSessions wires the session registry for the live-session check
// when deleting a character + the security view's active-sessions
// list. nil disables both.
func (m *AccountMenu) SetSessions(r *session.Registry) { m.session = r }

// SetAccounts wires the account repo used by the password and
// settings flows. nil keeps the surrounding UI but the affected
// drilldowns refuse with "not configured".
func (m *AccountMenu) SetAccounts(r repo.AccountRepo) { m.accounts = r }

// SetAccountUsername stamps the audit-row actor name. Empty falls back
// to a generic placeholder when an audit row is written.
func (m *AccountMenu) SetAccountUsername(name string) { m.accountUsername = name }

// SetSettings forwards the loaded AccountSettings (slice 3). postAuth
// loads them once per login from the account row and hands them off
// here; the settings drilldowns mutate them in place and persist via
// UpdateSettings.
func (m *AccountMenu) SetSettings(s repo.AccountSettings) { m.settings = s }

// SetLogins wires the slice-4 account_logins repo. The security
// substep reads from it via ListRecentByAccount; nil leaves the view
// available but reports an empty history.
func (m *AccountMenu) SetLogins(r repo.AccountLoginRepo) { m.logins = r }

func (m *AccountMenu) Prompt(_ context.Context, _ *telnet.Session) string {
	switch m.step {
	case accountStepConfirmDelete:
		return "[delete] "
	case accountStepCurrentPassword:
		return "Current password: "
	case accountStepNewPassword:
		return "New password: "
	case accountStepConfirmNewPassword:
		return "Confirm new password: "
	case accountStepSettingsPrompt, accountStepSettingsWidth, accountStepSettingsLocale:
		return "> "
	case accountStepQuitConfirm:
		return "[Y/N] "
	}
	// Numbered-picker pages share a compact prompt; the screen body
	// itself shows the valid range (e.g. "[1-7] >").
	return "> "
}

func (m *AccountMenu) OnEnter(s *telnet.Session) error {
	if m.rootShown {
		return nil
	}
	m.rootShown = true
	return m.writeRoot(s)
}

// OnExit clears any in-progress password-change state so a torn-down
// session never leaves password mode on (the next mode would inherit
// it) or retains a bcrypt hash in the menu struct. Idempotent;
// matches Create.OnExit.
func (m *AccountMenu) OnExit(s *telnet.Session) error {
	m.pendingNewHash = ""
	s.SetPasswordMode(false)
	return nil
}

func (m *AccountMenu) Handle(ctx context.Context, s *telnet.Session, line string) error {
	switch m.step {
	case accountStepRoot:
		return m.handleRootChoice(ctx, s, line)
	case accountStepPlayPicker:
		return m.handlePlayPickerLine(ctx, s, line)
	case accountStepDeletePicker:
		return m.handleDeletePickerLine(s, line)
	case accountStepConfirmDelete:
		return m.handleConfirmDelete(ctx, s, line)
	case accountStepCurrentPassword:
		return m.handleCurrentPassword(ctx, s, line)
	case accountStepNewPassword:
		return m.handleNewPassword(s, line)
	case accountStepConfirmNewPassword:
		return m.handleConfirmNewPassword(ctx, s, line)
	case accountStepSettingsRoot:
		return m.handleSettingsRootLine(s, line)
	case accountStepSettingsColor:
		return m.handleSettingsColorLine(ctx, s, line)
	case accountStepSettingsPrompt:
		return m.handleSettingsPromptLine(ctx, s, line)
	case accountStepSettingsWidth:
		return m.handleSettingsWidthLine(ctx, s, line)
	case accountStepSettingsLocale:
		return m.handleSettingsLocaleLine(ctx, s, line)
	case accountStepSettingsMOTD:
		return m.handleSettingsMOTDLine(ctx, s, line)
	case accountStepSecurity:
		return m.handleSecurityLine(ctx, s, line)
	case accountStepNews:
		return m.handleNewsLine(s, line)
	case accountStepQuitConfirm:
		return m.handleQuitConfirmLine(s, line)
	}
	// Defensive: unknown step. Reset to root.
	return m.returnToRoot(s)
}

// writeRoot renders the top-level menu. The displayed numbering is
// the source of truth for handleRootChoice's dispatch — see the
// rootMenu function below for the option table.
func (m *AccountMenu) writeRoot(s *telnet.Session) error {
	if err := display.SectionHeader(s, "Account: "+menuName(m.accountUsername)); err != nil {
		return err
	}
	if err := s.WriteRaw([]byte(m.lastLoginLine())); err != nil {
		return err
	}
	if err := s.WriteRaw([]byte("\r\n  Your characters:\r\n")); err != nil {
		return err
	}
	if err := s.WriteRaw([]byte(m.charListBlock())); err != nil {
		return err
	}
	opts := m.rootMenu()
	if err := display.Rule(s); err != nil {
		return err
	}
	for _, opt := range opts {
		if err := s.WriteRaw([]byte(fmt.Sprintf("  %d) %s\r\n", opt.number, opt.label))); err != nil {
			return err
		}
	}
	if err := display.Rule(s); err != nil {
		return err
	}
	if err := s.WriteRaw([]byte("  [Q]uit\r\n\r\n")); err != nil {
		return err
	}
	return s.WriteRaw([]byte(rangeHint(opts) + " >\r\n"))
}

// rootOption captures one row in the top-level numbered menu. action
// is dispatched by handleRootChoice when the user picks `number`.
type rootOption struct {
	number int
	label  string
	action rootAction
}

type rootAction int

const (
	rootActionPlay rootAction = iota
	rootActionNew
	rootActionDelete
	rootActionPassword
	rootActionSettings
	rootActionSecurity
	rootActionNews
)

// rootMenu returns the displayed option list, contiguously numbered.
// Empty rosters omit Play and Delete so the numbering is stable for
// the actions that *are* available — picking 1) Create works whether
// you have characters or not.
func (m *AccountMenu) rootMenu() []rootOption {
	all := []rootOption{
		{label: "Play", action: rootActionPlay},
		{label: "Create a new character", action: rootActionNew},
		{label: "Delete a character", action: rootActionDelete},
		{label: "Change password", action: rootActionPassword},
		{label: "Settings", action: rootActionSettings},
		{label: "Security", action: rootActionSecurity},
		{label: "Re-display news / MOTD", action: rootActionNews},
	}
	out := make([]rootOption, 0, len(all))
	for _, o := range all {
		if len(m.chars) == 0 && (o.action == rootActionPlay || o.action == rootActionDelete) {
			continue
		}
		o.number = len(out) + 1
		out = append(out, o)
	}
	return out
}

// handleRootChoice dispatches one numeric (or "q"/"Q") line at root.
// Anything else writes an inline error and re-renders the root.
func (m *AccountMenu) handleRootChoice(ctx context.Context, s *telnet.Session, line string) error {
	in := strings.TrimSpace(line)
	if strings.EqualFold(in, "q") || strings.EqualFold(in, "quit") || strings.EqualFold(in, "exit") {
		return m.handleQuitEnter(s)
	}
	if in == "" {
		return m.writeRoot(s)
	}
	opts := m.rootMenu()
	idx, err := parsePositiveIndex(in, len(opts))
	if err != nil {
		if werr := writeError(s, "Invalid choice. "+rangeHint(opts)+" or [Q]uit."); werr != nil {
			return werr
		}
		return nil
	}
	switch opts[idx].action {
	case rootActionPlay:
		return m.handlePlayEnter(ctx, s)
	case rootActionNew:
		return m.handleNew(s)
	case rootActionDelete:
		return m.handleDeleteEnter(s)
	case rootActionPassword:
		return m.handlePasswordEnter(s)
	case rootActionSettings:
		return m.handleSettingsEnter(s)
	case rootActionSecurity:
		return m.handleSecurityEnter(s)
	case rootActionNews:
		return m.handleNewsEnter(s)
	}
	return nil
}

// handleNew replaces the current mode with chargen. The slice-1b
// destructive deps are intentionally NOT forwarded — CharacterCreate
// ends in promoteToGame, never re-enters this menu. The hub's [Q]uit
// confirm is wired back through onCancel so an aborted chargen lands
// back on the account menu rather than disconnecting.
func (m *AccountMenu) handleNew(s *telnet.Session) error {
	create := NewCharacterCreate(m.repo, m.game)
	create.SetCatalog(m.catalog)
	create.SetSettings(m.settings)
	create.SetItems(m.items)
	create.SetOnCancel(func(s *telnet.Session) error {
		// Force a repaint when ReplaceMode runs OnEnter on the menu
		// instance the player came from.
		m.rootShown = false
		return s.ReplaceMode(m)
	})
	return s.ReplaceMode(create)
}

// returnToRoot resets m.step to root and re-renders. Used by every
// [B]ack path so a single function owns the transition (and the
// repaint stays uniform).
func (m *AccountMenu) returnToRoot(s *telnet.Session) error {
	m.step = accountStepRoot
	return m.writeRoot(s)
}

// charListBlock renders the "Your characters" body for the root
// screen. Returns an empty-roster placeholder when the account has
// no characters.
func (m *AccountMenu) charListBlock() string {
	if len(m.chars) == 0 {
		return "    (none — pick \"Create a new character\" below to make your first one)\r\n"
	}
	loc := m.displayLocation()
	var b strings.Builder
	for i, c := range m.chars {
		last := "never"
		if c.LastPlayedAt != nil && !c.LastPlayedAt.IsZero() {
			last = c.LastPlayedAt.In(loc).Format("2006-01-02")
		}
		fmt.Fprintf(&b, "    %d) %-20s  %-12s lvl %-2d  last %s\r\n",
			i+1, c.Name, className(c), totalClassLevels(c.ClassLevels), last)
	}
	return b.String()
}

// lastLoginLine renders the "Last login: …" subtitle on the root
// page. Sourced from the highest-AccountID-bound account row's
// LastLoginAt — but the menu doesn't carry that scalar today, so we
// fall back to the most-recently-played character's last_played_at
// as a proxy. Future work: pass repo.Account.LastLoginAt directly
// from postAuth into NewAccountMenu.
func (m *AccountMenu) lastLoginLine() string {
	loc := m.displayLocation()
	switch {
	case len(m.chars) == 0:
		return "  Last login: never\r\n"
	case m.chars[0].LastPlayedAt == nil || m.chars[0].LastPlayedAt.IsZero():
		return "  Last login: never\r\n"
	default:
		return "  Last login: " +
			m.chars[0].LastPlayedAt.In(loc).Format("2006-01-02 15:04") + "\r\n"
	}
}

// rangeHint produces the "[1-N]" footer hint for a numbered page.
// Empty option lists return "[]" — caller should never reach that.
func rangeHint(opts []rootOption) string {
	if len(opts) == 0 {
		return "[]"
	}
	if len(opts) == 1 {
		return fmt.Sprintf("[%d]", opts[0].number)
	}
	return fmt.Sprintf("[%d-%d]", opts[0].number, opts[len(opts)-1].number)
}

// menuName returns a non-empty display name for the account header.
// Empty username (test paths that didn't call SetAccountUsername)
// falls back to a placeholder so the header still renders.
func menuName(name string) string {
	if name == "" {
		return "(unknown)"
	}
	return name
}

// className returns a short label for the character's class
// progression. With no class levels it's "—" (totalClassLevels still
// floors at 1, so the player sees "—  lvl 1" — accurate for fresh
// chargen rows that haven't filled ClassLevels yet). Multi-class is
// rare in V1; we render the highest-level class name. classFallback
// in cmd/score.go has the canonical mapping; this helper duplicates
// it to avoid a cmd → mode dependency.
func className(c repo.Character) string {
	if len(c.ClassLevels) == 0 {
		return "—"
	}
	var top creature.Class
	var topLv int8
	for cl, lv := range c.ClassLevels {
		if lv > topLv {
			top = cl
			topLv = lv
		}
	}
	switch top {
	case creature.ClassAlgaiDSiswai:
		return "Algai'd'Siswai"
	case creature.ClassArmsman:
		return "Armsman"
	case creature.ClassInitiate:
		return "Initiate"
	case creature.ClassNoble:
		return "Noble"
	case creature.ClassWanderer:
		return "Wanderer"
	case creature.ClassWilder:
		return "Wilder"
	case creature.ClassWoodsman:
		return "Woodsman"
	}
	return "—"
}

// isBack returns true when the line is a blank, "b", or "back". The
// menu sub-pages all accept these as the canonical "return to parent"
// affordance. Cancel/abort still work as aliases for in-progress
// flows (delete confirm, password steps).
func isBack(line string) bool {
	in := strings.TrimSpace(line)
	if in == "" {
		return true
	}
	switch strings.ToLower(in) {
	case "b", "back":
		return true
	}
	return false
}

// isCancelOrBlank captures the shared "user wants out" checks used at
// every password-flow substep. Whitespace-only lines abort to match
// the cancel-on-blank behavior of the delete-confirm step.
func isCancelOrBlank(line string) bool {
	in := strings.TrimSpace(line)
	return in == "" || strings.EqualFold(in, "cancel") || strings.EqualFold(in, "abort")
}

// defaultLocale is the timezone used when AccountSettings.Locale is
// empty or fails to load. America/Chicago is the project default for
// V1 since the wider game has no real player-base; the setting itself
// stays free-form for users who want to override it.
const defaultLocale = "America/Chicago"

// displayLocation returns the *time.Location to format
// account-menu-rendered timestamps in. Settings.Locale is validated at
// edit time, but a stale row could still carry a now-unloadable zone;
// the fallback to defaultLocale keeps rendering deterministic in that
// edge. If even the default tzdata is missing (extremely minimal
// container build), we fall through to time.UTC.
func (m *AccountMenu) displayLocation() *time.Location {
	if m.settings.Locale != "" {
		if loc, err := time.LoadLocation(m.settings.Locale); err == nil {
			return loc
		}
	}
	if loc, err := time.LoadLocation(defaultLocale); err == nil {
		return loc
	}
	return time.UTC
}

// totalClassLevels sums the multi-class level map. Returns 1 when the
// map is empty so a freshly-created character without populated
// ClassLevels still shows lvl 1 in the listing.
func totalClassLevels(cl map[creature.Class]int8) int {
	if len(cl) == 0 {
		return 1
	}
	total := 0
	for _, lv := range cl {
		total += int(lv)
	}
	if total < 1 {
		return 1
	}
	return total
}
