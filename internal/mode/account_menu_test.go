package mode

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/auth"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// menuFixture wires every dep the AccountMenu needs for a numbered-
// picker-driven test. The feed() helper drives Handle one line at a
// time — tests express UX flows as a sequence of digit selections
// (e.g. "5" → "1" → "6" picks Settings → Color → Truecolor).
type menuFixture struct {
	t        *testing.T
	session  *telnet.Session
	peer     net.Conn
	chars    *repo.MemoryCharacterRepo
	items    *repo.MemoryItemRepo
	audits   *repo.MemoryAdminAuditRepo
	accounts *repo.MemoryAccountRepo
	account  repo.Account
	registry *session.Registry
	game     *stubMode
	captured *safeBuf
	menu     *AccountMenu
	motdHits *int32
}

func pushAccountMenu(t *testing.T, names []string) *menuFixture {
	t.Helper()
	server, client := net.Pipe()
	t.Cleanup(func() { server.Close(); client.Close() })

	cr := repo.NewMemoryCharacterRepo()
	// Burn the first-character bootstrap so seeded characters land at
	// the player tier instead of being elevated to admin.
	if _, err := cr.Create(context.Background(), repo.Character{AccountID: 999, Name: "Bootstrap"}); err != nil {
		t.Fatalf("seed bootstrap: %v", err)
	}
	var seeded []repo.Character
	for _, n := range names {
		c, err := cr.Create(context.Background(), repo.Character{
			AccountID: 1, Name: n, AuthLevel: repo.AuthLevelPlayer,
		})
		if err != nil {
			t.Fatalf("seed %q: %v", n, err)
		}
		seeded = append(seeded, c)
	}

	game := &stubMode{name: "game"}
	items := repo.NewMemoryItemRepo()
	audits := repo.NewMemoryAdminAuditRepo()
	accounts := repo.NewMemoryAccountRepo()
	hash, err := auth.Hash("oldpassword")
	if err != nil {
		t.Fatalf("hash old: %v", err)
	}
	acc, err := accounts.Create(context.Background(), repo.Account{Username: "rangerbob", PasswordHash: hash})
	if err != nil {
		t.Fatalf("seed account: %v", err)
	}
	registry := session.NewRegistry()
	menu := NewAccountMenu(seeded, cr, game)
	menu.SetItems(items)
	menu.SetAudits(audits)
	menu.SetAccounts(accounts)
	menu.SetSessions(registry)
	menu.SetAccountUsername("rangerbob")

	var motdHits int32
	menu.SetMOTD(func(_ *telnet.Session, _ time.Time) error {
		atomic.AddInt32(&motdHits, 1)
		return nil
	})

	s := telnet.NewSession(server)
	s.AccountID = acc.ID

	captured := &safeBuf{}
	drainPeer(t, client, captured)

	if err := s.PushMode(menu); err != nil {
		t.Fatalf("push: %v", err)
	}
	return &menuFixture{
		t: t, session: s, peer: client, chars: cr, items: items,
		audits: audits, accounts: accounts, account: acc,
		registry: registry, game: game,
		captured: captured, menu: menu, motdHits: &motdHits,
	}
}

func (f *menuFixture) feed(line string) {
	f.t.Helper()
	if err := f.session.CurrentMode().Handle(context.Background(), f.session, line); err != nil && err != telnet.ErrSessionEnded {
		f.t.Fatalf("Handle %q: %v", line, err)
	}
	time.Sleep(20 * time.Millisecond)
}

// rootChoice maps a label to the displayed number for the populated-
// roster root menu. Tests hard-code numbers (1=Play, 2=New, 3=Delete,
// 4=Password, 5=Settings, 6=Security, 7=News) — re-numbering would
// require updating every test, so the mapping is treated as part of
// the contract.
const (
	rootPlay     = "1"
	rootNew      = "2"
	rootDelete   = "3"
	rootPassword = "4"
	rootSettings = "5"
	rootSecurity = "6"
	rootNews     = "7"
)

// settings drilldown labels.
const (
	settingsColor  = "1"
	settingsPrompt = "2"
	settingsWidth  = "3"
	settingsLocale = "4"
	settingsMOTD   = "5"

	// settingsColorClear is the "Auto / clear override" pick inside the
	// color drilldown. Numbered 1 by colorOptions in account_menu_settings.go.
	settingsColorClear     = "1"
	settingsColorTruecolor = "6"
)

// ─── Root menu ───────────────────────────────────────────────────────

func TestAccountMenu_ListsOnEntry(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha", "Beta"})
	time.Sleep(20 * time.Millisecond)
	out := f.captured.String()
	if !strings.Contains(out, "Alpha") || !strings.Contains(out, "Beta") {
		t.Fatalf("expected both characters in listing: %q", out)
	}
	if !strings.Contains(out, "Account: rangerbob") {
		t.Fatalf("expected account header: %q", out)
	}
	if !strings.Contains(out, "1) Play") {
		t.Fatalf("expected numbered Play option: %q", out)
	}
}

func TestAccountMenu_RootHidesPlayDeleteWhenEmpty(t *testing.T) {
	f := pushAccountMenu(t, nil)
	time.Sleep(20 * time.Millisecond)
	out := f.captured.String()
	if strings.Contains(out, "1) Play") {
		t.Fatalf("Play must not appear in empty-roster root: %q", out)
	}
	if !strings.Contains(out, "1) Create a new character") {
		t.Fatalf("Create should be option 1 in empty-roster root: %q", out)
	}
}

func TestAccountMenu_InvalidChoiceRejects(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha"})
	f.captured.Reset()
	f.feed("frobnicate")
	if !strings.Contains(f.captured.String(), "Invalid choice") {
		t.Fatalf("expected invalid-choice notice: %q", f.captured.String())
	}
}

// ─── Play ────────────────────────────────────────────────────────────

func TestAccountMenu_PlaySingleAutoPicks(t *testing.T) {
	f := pushAccountMenu(t, []string{"Solo"})
	f.feed(rootPlay)
	if f.session.CurrentMode() != f.game {
		t.Fatalf("CurrentMode = %T, want game", f.session.CurrentMode())
	}
	if f.session.CharacterName != "Solo" {
		t.Fatalf("CharacterName = %q, want Solo", f.session.CharacterName)
	}
}

func TestAccountMenu_PlayMultiPicker(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha", "Beta"})
	f.feed(rootPlay)
	// Picker is now active; pick option 1.
	if f.menu.step != accountStepPlayPicker {
		t.Fatalf("step = %d, want PlayPicker", f.menu.step)
	}
	f.feed("1")
	if f.session.CurrentMode() != f.game {
		t.Fatalf("CurrentMode = %T, want game", f.session.CurrentMode())
	}
}

func TestAccountMenu_PlayPickerBackReturnsToRoot(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha", "Beta"})
	f.feed(rootPlay)
	f.feed("b")
	if f.menu.step != accountStepRoot {
		t.Fatalf("step = %d, want root after [B]ack", f.menu.step)
	}
}

// ─── New character ───────────────────────────────────────────────────

func TestAccountMenu_NewRoutesToCharacterCreate(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha"})
	f.feed(rootNew)
	if _, ok := f.session.CurrentMode().(*CharacterCreate); !ok {
		t.Fatalf("CurrentMode = %T, want *CharacterCreate", f.session.CurrentMode())
	}
}

// ─── News ────────────────────────────────────────────────────────────

func TestAccountMenu_NewsReplaysHook(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha"})
	if got := atomic.LoadInt32(f.motdHits); got != 0 {
		t.Fatalf("motd hits before news = %d, want 0", got)
	}
	f.feed(rootNews)
	if got := atomic.LoadInt32(f.motdHits); got != 1 {
		t.Fatalf("motd hits after news = %d, want 1", got)
	}
	// Stay in menu; news substep awaits Enter to return to root.
	if _, ok := f.session.CurrentMode().(*AccountMenu); !ok {
		t.Fatalf("news should stay in menu, got %T", f.session.CurrentMode())
	}
	if f.menu.step != accountStepNews {
		t.Fatalf("step = %d, want news", f.menu.step)
	}
	// Pressing any line returns to root.
	f.feed("")
	if f.menu.step != accountStepRoot {
		t.Fatalf("step = %d, want root after Enter", f.menu.step)
	}
}

// ─── Quit ────────────────────────────────────────────────────────────

func TestAccountMenu_QuitConfirmYes(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha"})
	f.feed("Q")
	if f.menu.step != accountStepQuitConfirm {
		t.Fatalf("step = %d, want quit confirm", f.menu.step)
	}
	f.captured.Reset()
	f.feed("y")
	if !strings.Contains(f.captured.String(), "Goodbye") {
		t.Fatalf("expected goodbye line: %q", f.captured.String())
	}
}

func TestAccountMenu_QuitConfirmNo(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha"})
	f.feed("Q")
	f.feed("n")
	if f.menu.step != accountStepRoot {
		t.Fatalf("step = %d, want root after declining quit", f.menu.step)
	}
}

// ─── Delete ──────────────────────────────────────────────────────────

func TestAccountMenu_DeletePickerBackReturnsToRoot(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha"})
	f.feed(rootDelete)
	if f.menu.step != accountStepDeletePicker {
		t.Fatalf("step = %d, want delete picker", f.menu.step)
	}
	f.feed("b")
	if f.menu.step != accountStepRoot {
		t.Fatalf("step = %d, want root", f.menu.step)
	}
}

func TestAccountMenu_DeleteConfirmCancel(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha", "Beta"})
	f.feed(rootDelete)
	f.feed("1") // pick Alpha
	if f.menu.step != accountStepConfirmDelete {
		t.Fatalf("step = %d, want confirm delete", f.menu.step)
	}
	f.captured.Reset()
	f.feed("cancel")
	if f.menu.step != accountStepRoot {
		t.Fatalf("step after cancel = %d, want root", f.menu.step)
	}
	// Character must still exist.
	if _, err := f.chars.FindByName(context.Background(), "Alpha"); err != nil {
		t.Fatalf("Alpha removed by cancel: %v", err)
	}
}

func TestAccountMenu_DeleteConfirmMismatchRepeats(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha", "Beta"})
	f.feed(rootDelete)
	f.feed("1")
	f.captured.Reset()
	f.feed("alpha") // case-sensitive — must not match
	if f.menu.step != accountStepConfirmDelete {
		t.Fatalf("step = %d, want still confirm delete", f.menu.step)
	}
	if !strings.Contains(f.captured.String(), "Names did not match") {
		t.Fatalf("expected mismatch warning: %q", f.captured.String())
	}
}

func TestAccountMenu_DeleteCascadesItemsAndAudits(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha", "Beta"})
	ctx := context.Background()

	target, err := f.chars.FindByName(ctx, "Alpha")
	if err != nil {
		t.Fatalf("find Alpha: %v", err)
	}
	outfit, err := f.items.Create(ctx, repo.Item{
		ExternalID: "outfit-1", Name: "leather jerkin", Type: repo.ItemTypeArmor,
		Stats: &repo.ArmorStats{Bonus: 1}, OwnerCharacterID: target.ID,
	})
	if err != nil {
		t.Fatalf("seed outfit: %v", err)
	}
	bag, err := f.items.Create(ctx, repo.Item{
		ExternalID: "bag-1", Name: "leather sack", Type: repo.ItemTypeContainer,
		Stats: &repo.ContainerStats{CapacityLbs: 50}, OwnerCharacterID: target.ID,
	})
	if err != nil {
		t.Fatalf("seed bag: %v", err)
	}
	pouch, err := f.items.Create(ctx, repo.Item{
		ExternalID: "pouch-1", Name: "coin pouch", Type: repo.ItemTypeTrash,
		ParentItemID: bag.ID,
	})
	if err != nil {
		t.Fatalf("seed pouch: %v", err)
	}
	foreignChar, err := f.chars.Create(ctx, repo.Character{AccountID: 1, Name: "Foreign"})
	if err != nil {
		t.Fatalf("seed foreign char: %v", err)
	}
	foreignItem, err := f.items.Create(ctx, repo.Item{
		ExternalID: "foreign-1", Name: "ring", Type: repo.ItemTypeTrash,
		OwnerCharacterID: foreignChar.ID,
	})
	if err != nil {
		t.Fatalf("seed foreign item: %v", err)
	}

	// Resolve Alpha's index in the menu's cached roster.
	var alphaIdx int
	for i, c := range f.menu.chars {
		if c.Name == "Alpha" {
			alphaIdx = i + 1
			break
		}
	}
	if alphaIdx == 0 {
		t.Fatalf("Alpha not in cached roster: %+v", f.menu.chars)
	}

	f.feed(rootDelete)
	f.feed(itoa(alphaIdx))
	f.feed("Alpha")

	if f.menu.step != accountStepRoot {
		t.Fatalf("step after delete = %d, want root", f.menu.step)
	}
	if _, err := f.chars.FindByName(ctx, "Alpha"); !errors.Is(err, repo.ErrCharacterNotFound) {
		t.Fatalf("Alpha not removed: err=%v", err)
	}
	for _, id := range []int64{outfit.ID, bag.ID, pouch.ID} {
		if _, err := f.items.GetByID(ctx, id); !errors.Is(err, repo.ErrItemNotFound) {
			t.Fatalf("item %d not deleted: err=%v", id, err)
		}
	}
	if _, err := f.items.GetByID(ctx, foreignItem.ID); err != nil {
		t.Fatalf("foreign item destroyed: %v", err)
	}

	rows, err := f.audits.List(ctx, repo.AdminAuditFilter{})
	if err != nil {
		t.Fatalf("list audits: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("audit rows = %d, want 1: %+v", len(rows), rows)
	}
	a := rows[0]
	if a.Verb != "delete-character" || a.Target != "Alpha" {
		t.Fatalf("audit row mismatch: %+v", a)
	}
	if a.ActorType != repo.ActorTypeAccount || a.ActorAccountID != 1 || a.ActorName != "rangerbob" {
		t.Fatalf("audit actor mismatch: %+v", a)
	}
}

func TestAccountMenu_DeleteWithoutItemsRepoRefuses(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha"})
	f.menu.SetItems(nil)
	f.feed(rootDelete)
	f.feed("1")
	f.captured.Reset()
	f.feed("Alpha")
	if !strings.Contains(f.captured.String(), "Could not delete") {
		t.Fatalf("expected refusal: %q", f.captured.String())
	}
	if _, err := f.chars.FindByName(context.Background(), "Alpha"); err != nil {
		t.Fatalf("Alpha removed despite refusal: %v", err)
	}
}

func TestAccountMenu_DeleteRefusesLoggedInTarget(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha", "Beta"})
	other, _ := net.Pipe()
	t.Cleanup(func() { _ = other.Close() })
	otherSess := telnet.NewSession(other)
	otherSess.SetInWorld(99, "Alpha", 1)
	f.registry.Bind(2, otherSess)

	f.feed(rootDelete)
	f.captured.Reset()
	f.feed("1") // Alpha is index 1 in the cached roster
	if !strings.Contains(f.captured.String(), "currently logged in") {
		t.Fatalf("expected live-session refusal: %q", f.captured.String())
	}
	if _, err := f.chars.FindByName(context.Background(), "Alpha"); err != nil {
		t.Fatalf("Alpha removed despite live-session block: %v", err)
	}
}

// ─── Password ────────────────────────────────────────────────────────

func TestAccountMenu_PasswordWithoutAccountsRepoRefuses(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha"})
	f.menu.SetAccounts(nil)
	f.captured.Reset()
	f.feed(rootPassword)
	if !strings.Contains(f.captured.String(), "not configured") {
		t.Fatalf("expected refusal: %q", f.captured.String())
	}
	if f.menu.step != accountStepRoot {
		t.Fatalf("step = %d, want root", f.menu.step)
	}
}

func TestAccountMenu_PasswordHappyPath(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha"})
	f.feed(rootPassword)
	if f.menu.step != accountStepCurrentPassword {
		t.Fatalf("step = %d, want CurrentPassword", f.menu.step)
	}
	f.feed("oldpassword")
	if f.menu.step != accountStepNewPassword {
		t.Fatalf("step = %d, want NewPassword", f.menu.step)
	}
	f.feed("brandnewpw")
	if f.menu.step != accountStepConfirmNewPassword {
		t.Fatalf("step = %d, want ConfirmNewPassword", f.menu.step)
	}
	f.captured.Reset()
	f.feed("brandnewpw")
	if !strings.Contains(f.captured.String(), "Password changed") {
		t.Fatalf("expected success line: %q", f.captured.String())
	}
	if f.menu.step != accountStepRoot {
		t.Fatalf("step = %d, want root", f.menu.step)
	}
	got, _ := f.accounts.FindByUsername(context.Background(), "rangerbob")
	if !auth.Verify(got.PasswordHash, "brandnewpw") {
		t.Fatal("new password did not verify after change")
	}
	rows, _ := f.audits.List(context.Background(), repo.AdminAuditFilter{})
	var found bool
	for _, r := range rows {
		if r.Verb == "change-password" {
			found = true
			if r.ActorType != repo.ActorTypeAccount || r.ActorAccountID != f.account.ID {
				t.Fatalf("audit actor mismatch: %+v", r)
			}
		}
	}
	if !found {
		t.Fatalf("change-password audit row missing: %+v", rows)
	}
}

func TestAccountMenu_PasswordWrongCurrentResetsToRoot(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha"})
	f.feed(rootPassword)
	f.captured.Reset()
	f.feed("notmypw")
	if !strings.Contains(f.captured.String(), "did not match") {
		t.Fatalf("expected mismatch notice: %q", f.captured.String())
	}
	if f.menu.step != accountStepRoot {
		t.Fatalf("step = %d, want root", f.menu.step)
	}
	got, _ := f.accounts.FindByUsername(context.Background(), "rangerbob")
	if !auth.Verify(got.PasswordHash, "oldpassword") {
		t.Fatal("old password should still verify")
	}
}

func TestAccountMenu_PasswordConfirmMismatchResets(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha"})
	f.feed(rootPassword)
	f.feed("oldpassword")
	f.feed("brandnewpw")
	f.captured.Reset()
	f.feed("differentpw")
	if !strings.Contains(f.captured.String(), "did not match") {
		t.Fatalf("expected confirm-mismatch notice: %q", f.captured.String())
	}
	if f.menu.step != accountStepRoot {
		t.Fatalf("step = %d, want root", f.menu.step)
	}
}

func TestAccountMenu_PasswordTooShortResets(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha"})
	f.feed(rootPassword)
	f.feed("oldpassword")
	f.captured.Reset()
	f.feed("short")
	if !strings.Contains(f.captured.String(), "too short") {
		t.Fatalf("expected too-short notice: %q", f.captured.String())
	}
}

func TestAccountMenu_PasswordCancelAtCurrent(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha"})
	f.feed(rootPassword)
	f.feed("cancel")
	if f.menu.step != accountStepRoot {
		t.Fatalf("step = %d, want root", f.menu.step)
	}
	if f.session.InPasswordMode {
		t.Fatal("session still in password mode after cancel")
	}
}

func TestAccountMenu_PasswordRepoFailureSurfacesGenericError(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha"})
	f.menu.SetAccounts(&failingUpdateAccounts{
		MemoryAccountRepo: f.accounts,
		err:               errors.New("synthetic repo failure"),
	})
	f.feed(rootPassword)
	f.feed("oldpassword")
	f.feed("brandnewpw")
	f.captured.Reset()
	f.feed("brandnewpw")
	if !strings.Contains(f.captured.String(), "Could not change password") {
		t.Fatalf("expected generic failure notice: %q", f.captured.String())
	}
	if f.menu.step != accountStepRoot {
		t.Fatalf("step = %d, want root", f.menu.step)
	}
	if f.session.InPasswordMode {
		t.Fatal("session still in password mode after failed update")
	}
	rows, _ := f.audits.List(context.Background(), repo.AdminAuditFilter{})
	for _, r := range rows {
		if r.Verb == "change-password" {
			t.Fatalf("audit row written despite repo error: %+v", r)
		}
	}
}

func TestAccountMenu_OnExitClearsPasswordFlow(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha"})
	f.feed(rootPassword)
	f.feed("oldpassword")
	if err := f.menu.OnExit(f.session); err != nil {
		t.Fatalf("OnExit: %v", err)
	}
	if f.session.InPasswordMode {
		t.Fatal("password mode persisted past OnExit")
	}
}

// failingUpdateAccounts wraps a real MemoryAccountRepo but forces
// UpdatePasswordHash to return a synthetic error so we can exercise
// the mode-layer "could not change password" branch.
type failingUpdateAccounts struct {
	*repo.MemoryAccountRepo
	err error
}

func (f *failingUpdateAccounts) UpdatePasswordHash(ctx context.Context, id int64, newHash string) error {
	return f.err
}

// ─── Settings ────────────────────────────────────────────────────────

func TestAccountMenu_SettingsRootShowsKeys(t *testing.T) {
	f := pushAccountMenu(t, []string{"Solo"})
	f.captured.Reset()
	f.feed(rootSettings)
	out := f.captured.String()
	for _, want := range []string{"1) Color", "2) Prompt", "3) Width", "4) Locale", "5) MOTD replay"} {
		if !strings.Contains(out, want) {
			t.Fatalf("settings root missing %q in:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "(auto)") {
		t.Fatalf("expected (auto) defaults: %q", out)
	}
}

func TestAccountMenu_SettingsColorSetsTruecolor(t *testing.T) {
	f := pushAccountMenu(t, []string{"Solo"})
	f.feed(rootSettings)
	f.feed(settingsColor)
	f.feed(settingsColorTruecolor)
	got, _ := f.accounts.FindByID(context.Background(), f.account.ID)
	if got.Settings.ColorOverride != "truecolor" {
		t.Fatalf("color = %q, want truecolor", got.Settings.ColorOverride)
	}
}

func TestAccountMenu_SettingsColorClear(t *testing.T) {
	f := pushAccountMenu(t, []string{"Solo"})
	// Set then clear.
	f.feed(rootSettings)
	f.feed(settingsColor)
	f.feed(settingsColorTruecolor)
	f.feed(settingsColor)
	f.feed(settingsColorClear)
	got, _ := f.accounts.FindByID(context.Background(), f.account.ID)
	if got.Settings.ColorOverride != "" {
		t.Fatalf("color = %q, want empty", got.Settings.ColorOverride)
	}
}

func TestAccountMenu_SettingsPromptFreeText(t *testing.T) {
	f := pushAccountMenu(t, []string{"Solo"})
	f.feed(rootSettings)
	f.feed(settingsPrompt)
	f.feed(`"<%h/%H hp>"`)
	got, _ := f.accounts.FindByID(context.Background(), f.account.ID)
	if got.Settings.PromptDefault != "<%h/%H hp>" {
		t.Fatalf("prompt = %q", got.Settings.PromptDefault)
	}
}

func TestAccountMenu_SettingsPromptStripsControlBytes(t *testing.T) {
	f := pushAccountMenu(t, []string{"Solo"})
	f.feed(rootSettings)
	f.feed(settingsPrompt)
	f.feed("hi\x1bX\x7f>")
	got, _ := f.accounts.FindByID(context.Background(), f.account.ID)
	stored := got.Settings.PromptDefault
	if stored == "" {
		t.Fatalf("expected non-empty stored prompt after stripping")
	}
	for _, r := range stored {
		if r < 0x20 || r == 0x7f {
			t.Fatalf("control byte %#x leaked into stored prompt %q", r, stored)
		}
	}
}

func TestAccountMenu_SettingsPromptClear(t *testing.T) {
	f := pushAccountMenu(t, []string{"Solo"})
	f.feed(rootSettings)
	f.feed(settingsPrompt)
	f.feed("hello")
	// After save we're back at settingsRoot; re-enter Prompt.
	f.feed(settingsPrompt)
	f.feed("clear")
	got, _ := f.accounts.FindByID(context.Background(), f.account.ID)
	if got.Settings.PromptDefault != "" {
		t.Fatalf("prompt not cleared: %q", got.Settings.PromptDefault)
	}
}

func TestAccountMenu_SettingsWidthSetsAndRejects(t *testing.T) {
	f := pushAccountMenu(t, []string{"Solo"})
	f.feed(rootSettings)
	f.feed(settingsWidth)
	f.captured.Reset()
	f.feed("xyz")
	if !strings.Contains(f.captured.String(), "Bad width") {
		t.Fatalf("expected bad-width notice: %q", f.captured.String())
	}
	// Out of range.
	f.captured.Reset()
	f.feed("10")
	if !strings.Contains(f.captured.String(), "Bad width") {
		t.Fatalf("expected bad-width notice (out of range): %q", f.captured.String())
	}
	// Valid.
	f.feed("100")
	got, _ := f.accounts.FindByID(context.Background(), f.account.ID)
	if got.Settings.WidthOverride != 100 {
		t.Fatalf("width = %d", got.Settings.WidthOverride)
	}
}

func TestAccountMenu_SettingsLocaleSetsAndRejects(t *testing.T) {
	f := pushAccountMenu(t, []string{"Solo"})
	f.feed(rootSettings)
	f.feed(settingsLocale)
	f.captured.Reset()
	f.feed("Nowhere/Made_Up")
	if !strings.Contains(f.captured.String(), "Bad locale") {
		t.Fatalf("expected bad-locale notice: %q", f.captured.String())
	}
	f.captured.Reset()
	f.feed("Local")
	if !strings.Contains(f.captured.String(), "Bad locale") {
		t.Fatalf("'Local' magic token must be rejected: %q", f.captured.String())
	}
	f.captured.Reset()
	f.feed("../etc/passwd")
	if !strings.Contains(f.captured.String(), "Bad locale") {
		t.Fatalf("path-bearing locale must be rejected: %q", f.captured.String())
	}
	f.feed("America/New_York")
	got, _ := f.accounts.FindByID(context.Background(), f.account.ID)
	if got.Settings.Locale != "America/New_York" {
		t.Fatalf("locale = %q", got.Settings.Locale)
	}
}

func TestAccountMenu_SettingsMOTDToggle(t *testing.T) {
	f := pushAccountMenu(t, []string{"Solo"})
	f.feed(rootSettings)
	f.feed(settingsMOTD)
	f.feed("1") // On
	got, _ := f.accounts.FindByID(context.Background(), f.account.ID)
	if !got.Settings.MOTDAlways {
		t.Fatalf("MOTDAlways not set")
	}
	f.feed(settingsMOTD)
	f.feed("2") // Off
	got, _ = f.accounts.FindByID(context.Background(), f.account.ID)
	if got.Settings.MOTDAlways {
		t.Fatalf("MOTDAlways not cleared")
	}
}

func TestAccountMenu_SettingsAuditsEachChange(t *testing.T) {
	f := pushAccountMenu(t, []string{"Solo"})
	f.feed(rootSettings)
	f.feed(settingsColor)
	f.feed(settingsColorTruecolor)
	f.feed(settingsWidth)
	f.feed("100")
	rows, _ := f.audits.List(context.Background(),
		repo.AdminAuditFilter{ActorAccount: f.account.ID, Limit: 50})
	if len(rows) != 2 {
		t.Fatalf("audit rows = %d, want 2", len(rows))
	}
	keys := map[string]bool{}
	for _, r := range rows {
		if r.Verb != "settings-update" {
			t.Fatalf("verb = %q", r.Verb)
		}
		keys[r.Target] = true
	}
	for _, want := range []string{"color", "width"} {
		if !keys[want] {
			t.Fatalf("missing audit row for %q (got %v)", want, keys)
		}
	}
}

func TestAccountMenu_SettingsAppliedOnPlay(t *testing.T) {
	f := pushAccountMenu(t, []string{"Solo"})
	f.feed(rootSettings)
	f.feed(settingsColor)
	f.feed("2") // None
	f.feed(settingsWidth)
	f.feed("80")
	f.feed("b") // back to root
	f.feed(rootPlay)
	if f.session.CurrentMode() != f.game {
		t.Fatalf("not promoted: %T", f.session.CurrentMode())
	}
	if f.session.ColorLevel != telnet.ColorLevelNone {
		t.Fatalf("ColorLevel = %d, want None", f.session.ColorLevel)
	}
	if f.session.Width != 80 {
		t.Fatalf("Width = %d, want 80", f.session.Width)
	}
}

func TestAccountMenu_SettingsForwardsPromptDefaultToCharacterCreate(t *testing.T) {
	f := pushAccountMenu(t, []string{"Solo"})
	f.feed(rootSettings)
	f.feed(settingsPrompt)
	f.feed("%h>")
	f.feed("b") // back to root
	f.feed(rootNew)
	create, ok := f.session.CurrentMode().(*CharacterCreate)
	if !ok {
		t.Fatalf("CurrentMode = %T, want *CharacterCreate", f.session.CurrentMode())
	}
	if create.settings.PromptDefault != "%h>" {
		t.Fatalf("forwarded prompt = %q, want %q", create.settings.PromptDefault, "%h>")
	}
}

func TestAccountMenu_SettingsMOTDAlwaysReplaysFromZero(t *testing.T) {
	f := pushAccountMenu(t, []string{"Solo"})
	f.menu.SetSettings(repo.AccountSettings{MOTDAlways: true})
	var got time.Time
	f.menu.SetMOTD(func(_ *telnet.Session, lastSeen time.Time) error {
		got = lastSeen
		return nil
	})
	f.feed(rootNews)
	if !got.IsZero() {
		t.Fatalf("MOTDAlways should pass zero time, got %v", got)
	}
}

// ─── postAuth ordering ───────────────────────────────────────────────

// TestPostAuth_FiresMOTDOncePerLogin locks in the §6 ordering note:
// MOTD/news fires before the AccountMenu, and is not re-fired by
// subsequent promote-to-game.
func TestPostAuth_FiresMOTDOncePerLogin(t *testing.T) {
	server, client := net.Pipe()
	t.Cleanup(func() { server.Close(); client.Close() })

	cr := repo.NewMemoryCharacterRepo()
	if _, err := cr.Create(context.Background(), repo.Character{AccountID: 999, Name: "Bootstrap"}); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if _, err := cr.Create(context.Background(), repo.Character{
		AccountID: 1, Name: "Hero", AuthLevel: repo.AuthLevelPlayer,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	game := &stubMode{name: "game"}
	s := telnet.NewSession(server)
	s.AccountID = 1
	captured := &safeBuf{}
	drainPeer(t, client, captured)

	var hits int32
	motd := func(_ *telnet.Session, _ time.Time) error {
		atomic.AddInt32(&hits, 1)
		return nil
	}

	if err := postAuth(context.Background(), s, cr, motd, nil, game, postAuthDeps{}); err != nil {
		t.Fatalf("postAuth: %v", err)
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("motd hits after postAuth = %d, want 1", got)
	}

	menu, ok := s.CurrentMode().(*AccountMenu)
	if !ok {
		t.Fatalf("CurrentMode = %T, want *AccountMenu", s.CurrentMode())
	}
	// Single character → "1) Play" auto-promotes.
	if err := menu.Handle(context.Background(), s, "1"); err != nil {
		t.Fatalf("play: %v", err)
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("motd hits after play = %d, want 1 (must not re-fire)", got)
	}
}
