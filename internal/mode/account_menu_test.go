package mode

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

type menuFixture struct {
	t        *testing.T
	session  *telnet.Session
	peer     net.Conn
	chars    *repo.MemoryCharacterRepo
	items    *repo.MemoryItemRepo
	audits   *repo.MemoryAdminAuditRepo
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
	registry := session.NewRegistry()
	menu := NewAccountMenu(seeded, cr, game)
	menu.SetItems(items)
	menu.SetAudits(audits)
	menu.SetSessions(registry)
	menu.SetAccountUsername("rangerbob")

	var motdHits int32
	menu.SetMOTD(func(_ *telnet.Session, _ time.Time) error {
		atomic.AddInt32(&motdHits, 1)
		return nil
	})

	s := telnet.NewSession(server)
	s.AccountID = 1

	captured := &safeBuf{}
	drainPeer(t, client, captured)

	if err := s.PushMode(menu); err != nil {
		t.Fatalf("push: %v", err)
	}
	return &menuFixture{
		t: t, session: s, peer: client, chars: cr, items: items,
		audits: audits, registry: registry, game: game,
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

func TestAccountMenu_ListsOnEntry(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha", "Beta"})
	time.Sleep(20 * time.Millisecond)
	out := f.captured.String()
	if !strings.Contains(out, "Alpha") || !strings.Contains(out, "Beta") {
		t.Fatalf("expected both characters in listing: %q", out)
	}
}

func TestAccountMenu_PlayNoArgAutoPicksSingle(t *testing.T) {
	f := pushAccountMenu(t, []string{"Solo"})
	f.feed("play")
	if f.session.CurrentMode() != f.game {
		t.Fatalf("CurrentMode = %T, want game", f.session.CurrentMode())
	}
	if f.session.CharacterName != "Solo" {
		t.Fatalf("CharacterName = %q, want Solo", f.session.CharacterName)
	}
}

func TestAccountMenu_PlayNoArgRefusesAmbiguous(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha", "Beta"})
	f.feed("play")
	if _, ok := f.session.CurrentMode().(*AccountMenu); !ok {
		t.Fatalf("ambiguous play should stay in menu, got %T", f.session.CurrentMode())
	}
	if !strings.Contains(f.captured.String(), "Specify") {
		t.Fatalf("expected disambiguation hint: %q", f.captured.String())
	}
}

func TestAccountMenu_PlayByNamePromotes(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha", "Beta"})
	f.feed("play beta")
	if f.session.CurrentMode() != f.game {
		t.Fatalf("CurrentMode = %T, want game", f.session.CurrentMode())
	}
	if f.session.CharacterName != "Beta" {
		t.Fatalf("CharacterName = %q, want Beta", f.session.CharacterName)
	}
}

func TestAccountMenu_PlayByIndexPromotes(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha", "Beta"})
	f.feed("play 2")
	if f.session.CurrentMode() != f.game {
		t.Fatalf("CurrentMode = %T, want game", f.session.CurrentMode())
	}
	// chars[0] is most-recently-played; "Beta" was created last so
	// it lands at index 0 under MemoryCharacterRepo's ordering. Index 2
	// addresses the second slot regardless — assert the session has
	// exactly one of the seeded names.
	if f.session.CharacterName != "Alpha" && f.session.CharacterName != "Beta" {
		t.Fatalf("CharacterName = %q, want Alpha or Beta", f.session.CharacterName)
	}
}

func TestAccountMenu_PlayForeignCharacterRejected(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha"})
	if _, err := f.chars.Create(context.Background(), repo.Character{AccountID: 999, Name: "Stranger"}); err != nil {
		t.Fatalf("seed stranger: %v", err)
	}
	f.feed("play Stranger")
	if _, ok := f.session.CurrentMode().(*AccountMenu); !ok {
		t.Fatal("foreign character must not promote — must stay in menu")
	}
	if !strings.Contains(f.captured.String(), "No such character on this account") {
		t.Fatalf("expected reject message: %q", f.captured.String())
	}
}

func TestAccountMenu_NewRoutesToCharacterCreate(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha"})
	f.feed("new")
	if _, ok := f.session.CurrentMode().(*CharacterCreate); !ok {
		t.Fatalf("CurrentMode = %T, want *CharacterCreate", f.session.CurrentMode())
	}
}

func TestAccountMenu_NewsReplaysHook(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha"})
	if got := atomic.LoadInt32(f.motdHits); got != 0 {
		t.Fatalf("motd hits before news = %d, want 0", got)
	}
	f.feed("news")
	if got := atomic.LoadInt32(f.motdHits); got != 1 {
		t.Fatalf("motd hits after news = %d, want 1", got)
	}
	// Replay must not advance the watermark or change mode.
	if _, ok := f.session.CurrentMode().(*AccountMenu); !ok {
		t.Fatalf("news should stay in menu, got %T", f.session.CurrentMode())
	}
}

func TestAccountMenu_HelpListsVerbs(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha"})
	f.captured.Reset()
	f.feed("help")
	out := f.captured.String()
	for _, want := range []string{"list", "play", "new", "news", "quit"} {
		if !strings.Contains(out, want) {
			t.Fatalf("help missing %q: %q", want, out)
		}
	}
}

func TestAccountMenu_QuitClosesConnection(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha"})
	f.feed("quit")
	if !strings.Contains(f.captured.String(), "Goodbye") {
		t.Fatalf("expected goodbye line: %q", f.captured.String())
	}
}

func TestAccountMenu_UnknownVerbStaysInMenu(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha"})
	f.feed("frobnicate")
	if _, ok := f.session.CurrentMode().(*AccountMenu); !ok {
		t.Fatal("unknown verb should stay in menu")
	}
	if !strings.Contains(f.captured.String(), "Unknown command") {
		t.Fatalf("expected error message: %q", f.captured.String())
	}
}

func TestAccountMenu_DeleteRequiresArg(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha"})
	f.captured.Reset()
	f.feed("delete")
	if !strings.Contains(f.captured.String(), "Usage: delete") {
		t.Fatalf("expected usage hint: %q", f.captured.String())
	}
}

func TestAccountMenu_DeleteUnknownNameStaysInRoot(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha"})
	f.captured.Reset()
	f.feed("delete Stranger")
	if _, ok := f.session.CurrentMode().(*AccountMenu); !ok {
		t.Fatalf("CurrentMode = %T, want *AccountMenu", f.session.CurrentMode())
	}
	if !strings.Contains(f.captured.String(), "No such character on this account") {
		t.Fatalf("expected reject message: %q", f.captured.String())
	}
	// Confirmation step must NOT have been pushed.
	if got := f.menu.Prompt(context.Background(), f.session); !strings.HasPrefix(got, "[account]") {
		t.Fatalf("prompt = %q, want [account]", got)
	}
}

func TestAccountMenu_DeleteForeignCharacterRejected(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha"})
	if _, err := f.chars.Create(context.Background(), repo.Character{
		AccountID: 999, Name: "Stranger",
	}); err != nil {
		t.Fatalf("seed stranger: %v", err)
	}
	f.captured.Reset()
	f.feed("delete Stranger")
	if !strings.Contains(f.captured.String(), "No such character on this account") {
		t.Fatalf("expected reject: %q", f.captured.String())
	}
	// Stranger must still exist.
	if _, err := f.chars.FindByName(context.Background(), "Stranger"); err != nil {
		t.Fatalf("stranger removed despite ownership gate: %v", err)
	}
}

func TestAccountMenu_DeleteCancelReturnsToRoot(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha", "Beta"})
	f.feed("delete Alpha")
	if got := f.menu.Prompt(context.Background(), f.session); !strings.HasPrefix(got, "[delete]") {
		t.Fatalf("prompt = %q, want [delete]", got)
	}
	f.captured.Reset()
	f.feed("cancel")
	if got := f.menu.Prompt(context.Background(), f.session); !strings.HasPrefix(got, "[account]") {
		t.Fatalf("prompt after cancel = %q, want [account]", got)
	}
	if !strings.Contains(f.captured.String(), "Cancelled") {
		t.Fatalf("expected cancel notice: %q", f.captured.String())
	}
	// Character must still exist.
	if _, err := f.chars.FindByName(context.Background(), "Alpha"); err != nil {
		t.Fatalf("Alpha removed by cancel: %v", err)
	}
}

func TestAccountMenu_DeleteMismatchedNameRepeatsConfirm(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha", "Beta"})
	f.feed("delete Alpha")
	f.captured.Reset()
	f.feed("alpha") // case-sensitive — must not match
	if got := f.menu.Prompt(context.Background(), f.session); !strings.HasPrefix(got, "[delete]") {
		t.Fatalf("prompt = %q, want still [delete]", got)
	}
	if !strings.Contains(f.captured.String(), "Names did not match") {
		t.Fatalf("expected mismatch warning: %q", f.captured.String())
	}
	// Character must still exist.
	if _, err := f.chars.FindByName(context.Background(), "Alpha"); err != nil {
		t.Fatalf("Alpha removed by mismatch: %v", err)
	}
}

func TestAccountMenu_DeleteCascadesItemsAndAudits(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha", "Beta"})
	ctx := context.Background()

	// Resolve the to-be-deleted character so we know its ID for the
	// item seed and the audit assertion.
	target, err := f.chars.FindByName(ctx, "Alpha")
	if err != nil {
		t.Fatalf("find Alpha: %v", err)
	}

	// Seed two items: a top-level worn outfit, and a bag containing
	// a coin pouch (verifies the BFS through parent_item_id is used,
	// not just ListInInventory).
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

	// Foreign item on a different character must NOT be touched.
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

	// Drive the delete.
	f.feed("delete Alpha")
	f.feed(target.Name) // exact case-sensitive match

	if got := f.menu.Prompt(ctx, f.session); !strings.HasPrefix(got, "[account]") {
		t.Fatalf("prompt after delete = %q, want [account]", got)
	}

	if _, err := f.chars.FindByName(ctx, "Alpha"); !errors.Is(err, repo.ErrCharacterNotFound) {
		t.Fatalf("Alpha not removed: err=%v", err)
	}
	for _, id := range []int64{outfit.ID, bag.ID, pouch.ID} {
		if _, err := f.items.GetByID(ctx, id); !errors.Is(err, repo.ErrItemNotFound) {
			t.Fatalf("item %d not deleted: err=%v", id, err)
		}
	}
	// Foreign item must survive.
	if _, err := f.items.GetByID(ctx, foreignItem.ID); err != nil {
		t.Fatalf("foreign item destroyed: %v", err)
	}

	// Audit row.
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

	// Roster reload — Alpha gone, Beta + Foreign still listed.
	if !strings.Contains(f.captured.String(), "Beta") {
		t.Fatalf("post-delete listing missing Beta: %q", f.captured.String())
	}
	if strings.Contains(f.captured.String()[strings.LastIndex(f.captured.String(), "Your characters:"):], "Alpha") {
		t.Fatalf("post-delete listing still contains Alpha: %q", f.captured.String())
	}
}

func TestAccountMenu_DeleteLastCharacterRendersEmptyRoster(t *testing.T) {
	f := pushAccountMenu(t, []string{"Solo"})
	f.feed("delete Solo")
	f.captured.Reset()
	f.feed("Solo")
	if got := f.menu.Prompt(context.Background(), f.session); !strings.HasPrefix(got, "[account]") {
		t.Fatalf("prompt = %q, want [account]", got)
	}
	out := f.captured.String()
	if !strings.Contains(out, "Character deleted") {
		t.Fatalf("expected delete confirmation: %q", out)
	}
	if !strings.Contains(out, "(none — type 'new' to create one)") {
		t.Fatalf("expected empty-roster line: %q", out)
	}
	// Solo gone from repo.
	if _, err := f.chars.FindByName(context.Background(), "Solo"); !errors.Is(err, repo.ErrCharacterNotFound) {
		t.Fatalf("Solo not removed: %v", err)
	}
}

func TestAccountMenu_DeleteWithoutItemsRepoRefuses(t *testing.T) {
	// Without SetItems, the cascade guard must refuse rather than
	// silently skip — orphaned items.owner_character_id rows are the
	// invariant slice 1b enforces.
	f := pushAccountMenu(t, []string{"Alpha"})
	f.menu.SetItems(nil)
	f.feed("delete Alpha")
	f.captured.Reset()
	f.feed("Alpha")
	if !strings.Contains(f.captured.String(), "Could not delete") {
		t.Fatalf("expected refusal: %q", f.captured.String())
	}
	// Character must still exist.
	if _, err := f.chars.FindByName(context.Background(), "Alpha"); err != nil {
		t.Fatalf("Alpha removed despite refusal: %v", err)
	}
}

func TestAccountMenu_DeleteRefusesLoggedInTarget(t *testing.T) {
	f := pushAccountMenu(t, []string{"Alpha", "Beta"})
	// Stand up a parallel session bound under a different account that
	// happens to be playing as "Alpha" (test contrives this; production
	// can't reach this state today, but the defensive check forward-
	// proofs the code).
	other, _ := net.Pipe()
	t.Cleanup(func() { _ = other.Close() })
	otherSess := telnet.NewSession(other)
	otherSess.SetInWorld(99, "Alpha", 1)
	f.registry.Bind(2, otherSess)

	f.captured.Reset()
	f.feed("delete Alpha")
	if !strings.Contains(f.captured.String(), "currently logged in") {
		t.Fatalf("expected live-session refusal: %q", f.captured.String())
	}
	if _, err := f.chars.FindByName(context.Background(), "Alpha"); err != nil {
		t.Fatalf("Alpha removed despite live-session block: %v", err)
	}
}

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
		t.Fatalf("motd hits after postAuth = %d, want 1 (fires before menu)", got)
	}

	menu, ok := s.CurrentMode().(*AccountMenu)
	if !ok {
		t.Fatalf("CurrentMode = %T, want *AccountMenu", s.CurrentMode())
	}
	if err := menu.Handle(context.Background(), s, "play"); err != nil {
		t.Fatalf("play: %v", err)
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("motd hits after play = %d, want 1 (must not re-fire)", got)
	}
}
