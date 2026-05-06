package mode

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/Jasrags/WheelMUD/internal/audit"
	"github.com/Jasrags/WheelMUD/internal/auth"
	"github.com/Jasrags/WheelMUD/internal/chargen"
	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// AccountMenu is the post-authentication hub. Login / Create push it
// after firing the once-per-login MOTD/news block, so character
// management lives behind a stable set of verbs rather than the legacy
// auto-skip-to-select shortcut.
//
// Slice 1 verbs:
//
//	list           — re-display the character roster
//	play [name]    — promote into game; no-arg auto-picks when there's
//	                 exactly one character (preserves the "one keystroke
//	                 into the world" feel for single-character accounts)
//	new            — ReplaceMode into CharacterCreate
//	news           — replay the unseen-news block on demand without
//	                 advancing last_news_seen
//	help           — list verbs
//	quit           — close the connection
//
// Slice 1b adds:
//
//	delete <n|#>   — destroy a character on this account. Requires
//	                 typed-name confirmation; cascades owned items
//	                 (recursively, including container contents) and
//	                 records an account-mode admin_audit row. Wiring
//	                 the new soft-FK cleanup is application-level —
//	                 items.owner_character_id is a soft FK by design.
//
// Subsequent slices add password change (2), account settings (3),
// security/login audit (4), and email/recovery (5).
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
	// lastSeen feeds the `news` replay verb so re-rendering shows the
	// same unseen entries the post-login block did. Sourced from the
	// most-recently-played character (chars[0] under ListByAccount's
	// ordering), or the zero value when the account has no characters.
	lastSeen  time.Time
	listShown bool

	// Substep state. accountStepRoot dispatches verbs; child steps
	// own a focused prompt (typed-name delete confirm; the 3-step
	// password-change flow current → new → confirm).
	step          accountStep
	pendingDelete *repo.Character
	// pendingNewHash carries the bcrypt hash produced from the
	// new-password prompt across to the confirm step. Cleared on
	// completion / cancel / OnExit so plaintext-derived state never
	// outlives the flow.
	pendingNewHash string
}

// accountStep is the post-auth menu's substep enum. Mirrors the
// chargenStep pattern in character_create.go: a single Handle entry
// point branches on step, child steps capture in-progress state on
// the AccountMenu struct.
type accountStep int

const (
	accountStepRoot accountStep = iota
	accountStepConfirmDelete
	accountStepCurrentPassword
	accountStepNewPassword
	accountStepConfirmNewPassword
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

// SetMOTD wires the MOTD/news hook used by the `news` replay verb.
func (m *AccountMenu) SetMOTD(f MOTDFunc) { m.motd = f }

// SetCatalog forwards the chargen content catalog to a CharacterCreate
// spawned by the `new` verb.
func (m *AccountMenu) SetCatalog(c *chargen.Catalog) { m.catalog = c }

// SetItems wires the item repo used by the delete-character cascade.
// nil keeps the verb available but reports a generic failure if the
// owner has any items at delete time.
func (m *AccountMenu) SetItems(r repo.ItemRepo) { m.items = r }

// SetAudits wires the admin_audit repo used to record destructive
// account-menu actions. nil silently skips the audit row.
func (m *AccountMenu) SetAudits(r repo.AdminAuditRepo) { m.audits = r }

// SetSessions wires the session registry for the live-session check
// when deleting a character. nil disables the check (acceptable for
// tests; production wiring always supplies it).
func (m *AccountMenu) SetSessions(r *session.Registry) { m.session = r }

// SetAccounts wires the account repo used by the change-password
// flow. nil keeps the verb available but reports a generic
// "not configured" message — matches the SetMOTD/SetItems pattern
// of fail-soft when an optional dep is missing.
func (m *AccountMenu) SetAccounts(r repo.AccountRepo) { m.accounts = r }

// SetAccountUsername stamps the audit-row actor name. Empty falls back
// to a generic placeholder when an audit row is written.
func (m *AccountMenu) SetAccountUsername(name string) { m.accountUsername = name }

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
	}
	return "[account] "
}

func (m *AccountMenu) OnEnter(s *telnet.Session) error {
	if m.listShown {
		return nil
	}
	m.listShown = true
	return m.writeList(s)
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
	case accountStepConfirmDelete:
		return m.handleConfirmDelete(ctx, s, line)
	case accountStepCurrentPassword:
		return m.handleCurrentPassword(ctx, s, line)
	case accountStepNewPassword:
		return m.handleNewPassword(s, line)
	case accountStepConfirmNewPassword:
		return m.handleConfirmNewPassword(ctx, s, line)
	}
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) == 0 {
		return nil
	}
	verb := strings.ToLower(fields[0])
	args := fields[1:]
	switch verb {
	case "list", "ls", "characters":
		return m.writeList(s)
	case "play", "select":
		return m.handlePlay(ctx, s, args)
	case "new", "create":
		return m.handleNew(s)
	case "delete", "del", "remove":
		return m.handleDelete(s, args)
	case "password", "passwd":
		return m.handlePassword(s)
	case "news", "motd":
		return m.handleNews(s)
	case "help", "?":
		return m.writeHelp(s)
	case "quit", "exit":
		_ = s.WriteRaw([]byte("Goodbye.\r\n"))
		_ = s.Conn.Close()
		return telnet.ErrSessionEnded
	}
	return writeError(s, "Unknown command. Type 'help' for the menu.")
}

func (m *AccountMenu) writeList(s *telnet.Session) error {
	var b strings.Builder
	b.WriteString("\r\nYour characters:\r\n")
	if len(m.chars) == 0 {
		b.WriteString("  (none — type 'new' to create one)\r\n")
		return s.WriteRaw([]byte(b.String()))
	}
	for i, c := range m.chars {
		last := "never"
		if c.LastPlayedAt != nil && !c.LastPlayedAt.IsZero() {
			last = c.LastPlayedAt.Format("2006-01-02")
		}
		fmt.Fprintf(&b, "  %d) %-20s  lvl %-2d  last %s\r\n",
			i+1, c.Name, totalClassLevels(c.ClassLevels), last)
	}
	return s.WriteRaw([]byte(b.String()))
}

// handlePlay resolves the target character and promotes the session
// into game mode. With no arg, single-character accounts auto-pick;
// multi-character accounts are required to name the character. Lookup
// is constrained to the menu's pre-loaded roster so a player can't
// promote into a character they don't own — the same ownership gate
// CharacterSelect uses.
func (m *AccountMenu) handlePlay(ctx context.Context, s *telnet.Session, args []string) error {
	if len(m.chars) == 0 {
		return writeError(s, "No characters on this account. Type 'new' to create one.")
	}
	var target *repo.Character
	switch len(args) {
	case 0:
		if len(m.chars) > 1 {
			return writeError(s, "Specify a character: play <name>.")
		}
		target = &m.chars[0]
	default:
		name := args[0]
		// Numeric pick (matches the list ordering shown by `list`).
		if i, err := parsePositiveIndex(name, len(m.chars)); err == nil {
			target = &m.chars[i]
			break
		}
		for i := range m.chars {
			if strings.EqualFold(m.chars[i].Name, name) {
				target = &m.chars[i]
				break
			}
		}
		if target == nil {
			// Re-resolve via repo to distinguish "no such character
			// anywhere" from "not yours" without leaking either bit.
			if _, err := m.repo.FindByName(ctx, name); err != nil && !errors.Is(err, repo.ErrCharacterNotFound) {
				return writeError(s, "Could not load that character. Try again.")
			}
			return writeError(s, "No such character on this account.")
		}
	}
	return promoteToGame(ctx, s, *target, m.repo, m.game)
}

func (m *AccountMenu) handleNew(s *telnet.Session) error {
	create := NewCharacterCreate(m.repo, m.game)
	create.SetCatalog(m.catalog)
	// MOTD intentionally not threaded into CharacterCreate: the news
	// block fires once per login in postAuth, before the menu lands.
	// promoteToGame downstream is now MOTD-free for the same reason.
	//
	// Slice 1b deps (items / audits / sessions / accountUsername) are
	// intentionally NOT forwarded here: CharacterCreate ends with
	// promoteToGame, not a postAuth round-trip, so it never re-enters
	// this menu and never needs the destructive-action plumbing. If a
	// future refactor adds a "back to account menu" path off
	// CharacterCreate, the deps would need to be threaded through —
	// at which point this should be hoisted onto a shared deps
	// builder rather than copied.
	return s.ReplaceMode(create)
}

// handleDelete resolves the target character (by 1-based index or
// case-insensitive name) and pushes the typed-name confirm substep.
// Resolution is constrained to m.chars so a player can't see whether a
// foreign name exists. A defensive live-session check refuses deletion
// of a currently-online character even though the session registry's
// one-session-per-account policy means that character isn't this
// session's deleter; future multi-session work would lean on this
// check.
func (m *AccountMenu) handleDelete(s *telnet.Session, args []string) error {
	if len(m.chars) == 0 {
		return writeError(s, "No characters on this account.")
	}
	if len(args) == 0 {
		return writeError(s, "Usage: delete <name|#>")
	}
	target := m.resolveOwned(args[0])
	if target == nil {
		return writeError(s, "No such character on this account.")
	}
	if m.session != nil {
		if active := m.session.FindByCharacterName(target.Name); active != nil {
			return writeError(s, "That character is currently logged in. Try again later.")
		}
	}
	m.pendingDelete = target
	m.step = accountStepConfirmDelete
	last := "never"
	if target.LastPlayedAt != nil && !target.LastPlayedAt.IsZero() {
		last = target.LastPlayedAt.Format("2006-01-02")
	}
	var b strings.Builder
	b.WriteString("\r\nDelete this character?\r\n")
	fmt.Fprintf(&b, "  %s  lvl %d  last %s\r\n",
		target.Name, totalClassLevels(target.ClassLevels), last)
	b.WriteString("\r\nThis cannot be undone. All carried items, equipment,\r\n")
	b.WriteString("and bank balance will be destroyed.\r\n")
	b.WriteString("\r\nType the character's name exactly (case-sensitive) to confirm, or 'cancel' to abort.\r\n")
	return s.WriteRaw([]byte(b.String()))
}

// handleConfirmDelete runs at accountStepConfirmDelete. cancel/blank
// returns to root; an exact (case-sensitive) name match executes the
// cascade. Mismatches repeat the prompt rather than auto-aborting so
// a typo doesn't lose progress, and so the deletion stays explicit.
func (m *AccountMenu) handleConfirmDelete(ctx context.Context, s *telnet.Session, line string) error {
	in := strings.TrimSpace(line)
	if in == "" || strings.EqualFold(in, "cancel") || strings.EqualFold(in, "abort") {
		m.pendingDelete = nil
		m.step = accountStepRoot
		if err := s.WriteRaw([]byte("Cancelled.\r\n")); err != nil {
			return err
		}
		return m.writeList(s)
	}
	target := m.pendingDelete
	if target == nil {
		// Defensive: confirm step entered without a pending target
		// (shouldn't happen, but keep it deterministic).
		m.step = accountStepRoot
		return writeError(s, "Nothing to confirm. Type 'help' for the menu.")
	}
	if in != target.Name {
		return writeError(s, "Names did not match. Type the character's name exactly, or 'cancel'.")
	}
	if err := m.executeDelete(ctx, s, *target); err != nil {
		// Per-step state is reset so the user can retry without being
		// stuck in the confirm prompt with a stale target.
		m.pendingDelete = nil
		m.step = accountStepRoot
		slog.Warn("account_menu: delete cascade failed",
			"account", s.AccountID, "character", target.ID, "err", err)
		return writeError(s, "Could not delete that character. Try again later.")
	}
	m.pendingDelete = nil
	m.step = accountStepRoot
	if err := s.WriteRaw([]byte("Character deleted.\r\n")); err != nil {
		return err
	}
	return m.refreshAndList(ctx, s, target.ID)
}

// executeDelete performs the application-level cascade for a character
// row: every owned item (top-level inventory plus everything nested
// inside containers they own) is deleted, then the character row
// itself, then an account-mode audit row is written. Order matters —
// ListAllOwnedTransitive before character.Delete keeps the BFS through
// items.parent_item_id valid; character.Delete is the last destructive
// step so a partial item failure leaves a deletable character.
//
// Refuses if m.items is nil (the items repo wasn't wired). The
// alternative — silently skipping the cascade and deleting the row —
// would orphan items.owner_character_id references, which is the
// invariant slice 1b exists to enforce. Production wiring always
// supplies items; this guard catches misconfiguration loudly.
//
// The live-session check is re-run here (in addition to handleDelete)
// to close the TOCTOU window between confirm prompt and execute. Single-
// session-per-account makes this almost impossible today, but cheap.
func (m *AccountMenu) executeDelete(ctx context.Context, s *telnet.Session, target repo.Character) error {
	if m.items == nil {
		return errors.New("item repo not wired; refusing to delete character without item cascade")
	}
	if m.session != nil {
		if active := m.session.FindByCharacterName(target.Name); active != nil {
			return fmt.Errorf("character %q is logged in", target.Name)
		}
	}
	owned, err := m.items.ListAllOwnedTransitive(ctx, target.ID)
	if err != nil {
		return fmt.Errorf("list owned items: %w", err)
	}
	for _, it := range owned {
		if err := m.items.Delete(ctx, it.ID); err != nil {
			return fmt.Errorf("delete item %d: %w", it.ID, err)
		}
	}
	if err := m.repo.Delete(ctx, target.ID); err != nil {
		return fmt.Errorf("delete character row: %w", err)
	}
	audit.RecordAccount(ctx, m.audits, s.AccountID, m.accountUsername,
		"delete-character", target.Name,
		fmt.Sprintf("id=%d level=%d", target.ID, totalClassLevels(target.ClassLevels)))
	return nil
}

// refreshAndList reloads the character roster from the repo so the
// post-delete list reflects state-of-the-world, then renders it.
// deletedID is dropped from the cached roster on repo failure so the
// fallback render doesn't show the just-deleted character (which would
// be very confusing right after a "Character deleted." line).
func (m *AccountMenu) refreshAndList(ctx context.Context, s *telnet.Session, deletedID int64) error {
	chars, err := m.repo.ListByAccount(ctx, s.AccountID)
	if err != nil {
		slog.Warn("account_menu: refresh ListByAccount failed",
			"account", s.AccountID, "err", err)
		// Drop the deleted entry from the cached roster so the user
		// doesn't see it in the fallback list.
		filtered := m.chars[:0]
		for _, c := range m.chars {
			if c.ID != deletedID {
				filtered = append(filtered, c)
			}
		}
		m.chars = filtered
	} else {
		m.chars = chars
	}
	return m.writeList(s)
}

// resolveOwned looks up a character in the cached roster by 1-based
// list index or case-insensitive name. Returns nil for misses; never
// hits the repo so a foreign name leaks no information.
func (m *AccountMenu) resolveOwned(arg string) *repo.Character {
	if i, err := parsePositiveIndex(arg, len(m.chars)); err == nil {
		c := m.chars[i]
		return &c
	}
	for i := range m.chars {
		if strings.EqualFold(m.chars[i].Name, arg) {
			c := m.chars[i]
			return &c
		}
	}
	return nil
}

// handlePassword opens the change-password substep flow. It enters
// password-mode immediately so the very next keystroke (which becomes
// the current-password line) is masked. The dispatcher repaints the
// step-aware Prompt() automatically; no extra write here.
//
// Refuses with a "not configured" notice when the account repo isn't
// wired (memory-only test paths). Production wiring always supplies it.
func (m *AccountMenu) handlePassword(s *telnet.Session) error {
	if m.accounts == nil {
		return writeError(s, "Password change is not configured on this server.")
	}
	m.step = accountStepCurrentPassword
	s.SetPasswordMode(true)
	return nil
}

// handleCurrentPassword runs at accountStepCurrentPassword. cancel /
// blank → reset to root. Otherwise re-fetch the account by the
// snapshotted username (cheaper and safer than caching the hash on
// the menu) and bcrypt-verify. Mismatch resets to root with a generic
// notice; the user retypes `password` to retry. Match advances to
// accountStepNewPassword keeping password mode on.
func (m *AccountMenu) handleCurrentPassword(ctx context.Context, s *telnet.Session, line string) error {
	if isCancelOrBlank(line) {
		m.resetPasswordFlow(s)
		return s.WriteRaw([]byte("Cancelled.\r\n"))
	}
	if m.accounts == nil {
		// Defensive: handlePassword guards on this, but if accounts is
		// cleared mid-flow, fail closed.
		m.resetPasswordFlow(s)
		return writeError(s, "Password change is not configured on this server.")
	}
	a, err := m.accounts.FindByUsername(ctx, m.accountUsername)
	if err != nil {
		slog.Warn("account_menu: find account for password change failed",
			"account", s.AccountID, "err", err)
		m.resetPasswordFlow(s)
		return writeError(s, "Could not change password. Try again later.")
	}
	if !auth.Verify(a.PasswordHash, line) {
		m.resetPasswordFlow(s)
		return writeError(s, "Current password did not match.")
	}
	m.step = accountStepNewPassword
	// Password mode stays on for the next prompt.
	return nil
}

// handleNewPassword runs at accountStepNewPassword. cancel / blank →
// reset to root. Otherwise hash via auth.Hash and stash on the menu.
// Length-policy error wording matches Create.handlePassword so a user
// who hits a too-short / too-long during chargen sees the same notice
// during rotation.
func (m *AccountMenu) handleNewPassword(s *telnet.Session, line string) error {
	if isCancelOrBlank(line) {
		m.resetPasswordFlow(s)
		return s.WriteRaw([]byte("Cancelled.\r\n"))
	}
	hash, err := auth.Hash(line)
	switch {
	case errors.Is(err, auth.ErrPasswordTooShort):
		m.resetPasswordFlow(s)
		return s.WriteRaw([]byte("Password too short (minimum 8 characters).\r\n"))
	case errors.Is(err, auth.ErrPasswordTooLong):
		m.resetPasswordFlow(s)
		return s.WriteRaw([]byte("Password too long (maximum 72 bytes).\r\n"))
	case err != nil:
		m.resetPasswordFlow(s)
		return s.WriteRaw([]byte("Could not process password. Try again.\r\n"))
	}
	m.pendingNewHash = hash
	m.step = accountStepConfirmNewPassword
	// Password mode stays on for the confirm prompt.
	return nil
}

// handleConfirmNewPassword runs at accountStepConfirmNewPassword.
// Mirrors Create.handleConfirm: clears password mode first thing, then
// verifies the typed line against the stashed hash. Match → persist
// + audit + "Password changed.". Any other outcome (cancel, mismatch,
// repo failure) resets to root with the appropriate notice.
func (m *AccountMenu) handleConfirmNewPassword(ctx context.Context, s *telnet.Session, line string) error {
	s.SetPasswordMode(false)
	if isCancelOrBlank(line) {
		m.resetPasswordFlow(s)
		return s.WriteRaw([]byte("Cancelled.\r\n"))
	}
	if !auth.Verify(m.pendingNewHash, line) {
		m.resetPasswordFlow(s)
		return writeError(s, "Passwords did not match. Run 'password' to try again.")
	}
	if err := m.accounts.UpdatePasswordHash(ctx, s.AccountID, m.pendingNewHash); err != nil {
		slog.Warn("account_menu: password update failed",
			"account", s.AccountID, "err", err)
		m.resetPasswordFlow(s)
		return writeError(s, "Could not change password. Try again later.")
	}
	audit.RecordAccount(ctx, m.audits, s.AccountID, m.accountUsername,
		"change-password", "", "")
	m.resetPasswordFlow(s)
	return s.WriteRaw([]byte("Password changed.\r\n"))
}

// resetPasswordFlow clears in-progress state and exits password mode.
// Idempotent — safe to call from any step (including from within
// handleConfirmNewPassword which already turned masking off).
func (m *AccountMenu) resetPasswordFlow(s *telnet.Session) {
	m.step = accountStepRoot
	m.pendingNewHash = ""
	s.SetPasswordMode(false)
}

// isCancelOrBlank captures the shared "user wants out" checks used at
// every password-flow substep. Whitespace-only lines abort to match
// the cancel-on-blank behavior of the delete-confirm step.
func isCancelOrBlank(line string) bool {
	in := strings.TrimSpace(line)
	return in == "" || strings.EqualFold(in, "cancel") || strings.EqualFold(in, "abort")
}

func (m *AccountMenu) handleNews(s *telnet.Session) error {
	if m.motd == nil {
		return writeError(s, "News is not configured on this server.")
	}
	return m.motd(s, m.lastSeen)
}

func (m *AccountMenu) writeHelp(s *telnet.Session) error {
	help := "" +
		"\r\nAccount menu:\r\n" +
		"  list              show your characters\r\n" +
		"  play [name|#]     enter the world (no-arg picks the only character)\r\n" +
		"  new               create a new character\r\n" +
		"  delete <name|#>   permanently destroy a character (typed-name confirm)\r\n" +
		"  password          change your account password\r\n" +
		"  news              re-display the unread-news block\r\n" +
		"  help              this list\r\n" +
		"  quit              disconnect\r\n"
	return s.WriteRaw([]byte(help))
}

// parsePositiveIndex interprets `s` as a 1-based list index in
// [1, n] and returns the 0-based slot. Anything non-numeric or out of
// range returns an error so the caller can fall back to a name lookup.
func parsePositiveIndex(s string, n int) (int, error) {
	v, err := strconv.Atoi(s)
	if err != nil || v < 1 || v > n {
		return 0, errors.New("out of range")
	}
	return v - 1, nil
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
