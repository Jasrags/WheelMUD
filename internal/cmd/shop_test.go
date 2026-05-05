package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/currency"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/telnet"
)

// fixedHour is a tiny HourSource for deterministic shop-hour tests.
type fixedHour int

func (f fixedHour) HourOfDay() int { return int(f) }

// shopFixture extends invFixture with a shopkeeper mob, a shop config,
// and a couple of stock lines for a typical innkeeper. The shop sits
// in room 1 alongside Alice + Bob.
type shopFixture struct {
	*invFixture
	mobs      *repo.MemoryMobInstanceRepo
	templates *repo.MemoryMobTemplateRepo
	shops     *repo.MemoryShopRepo
	keeperTpl int64
	shopID    int64
	hour      *fixedHour
}

func newShopFixture(t *testing.T) *shopFixture {
	t.Helper()
	inv := newInvFixture(t)

	templates := repo.NewMemoryMobTemplateRepo()
	mobs := repo.NewMemoryMobInstanceRepo()
	shops := repo.NewMemoryShopRepo()

	tpl, err := templates.Create(context.Background(), creature.MobTemplate{
		ExternalID: "inn.bran",
		Core:       creature.Core{Name: "Bran al'Vere"},
	})
	if err != nil {
		t.Fatalf("create template: %v", err)
	}
	if _, err := mobs.Create(context.Background(), creature.MobInstance{
		TemplateID: tpl.ID,
		Core:       creature.Core{CurrentRoomID: 1},
	}); err != nil {
		t.Fatalf("create instance: %v", err)
	}
	shop, err := shops.Create(context.Background(), repo.Shop{
		MobTemplateID: tpl.ID,
		BuyTypes:      []repo.ItemType{repo.ItemTypeFood, repo.ItemTypeConsumable, repo.ItemTypeTradeGood},
		SellMarkup:    1.0,
		BuyMarkdown:   0.5,
		// OpenHour == CloseHour → always open
	})
	if err != nil {
		t.Fatalf("create shop: %v", err)
	}

	// Seed item templates the shop carries, on the shopkeeper's room
	// floor (the YAML loader does the same).
	inv.items.Insert(repo.Item{
		ExternalID: "inn.ale", Name: "a mug of ale", NameLower: "a mug of ale",
		RoomID: 1, Type: repo.ItemTypeConsumable, Value: 5, Weight: 1,
	})
	inv.items.Insert(repo.Item{
		ExternalID: "inn.bread", Name: "a loaf of bread", NameLower: "a loaf of bread",
		RoomID: 1, Type: repo.ItemTypeFood, Value: 2, Weight: 1,
	})
	if err := shops.UpsertStock(context.Background(), repo.ShopStockRow{
		ShopID: shop.ID, ItemExternalID: "inn.ale", Qty: 3, QtyMax: 12,
	}); err != nil {
		t.Fatalf("stock ale: %v", err)
	}
	if err := shops.UpsertStock(context.Background(), repo.ShopStockRow{
		ShopID: shop.ID, ItemExternalID: "inn.bread", Qty: -1, QtyMax: -1,
	}); err != nil {
		t.Fatalf("stock bread: %v", err)
	}

	hour := fixedHour(12)
	return &shopFixture{
		invFixture: inv,
		mobs:       mobs,
		templates:  templates,
		shops:      shops,
		keeperTpl:  tpl.ID,
		shopID:     shop.ID,
		hour:       &hour,
	}
}

// giveCoin tops up Alice's purse to amount (in cp).
func (f *shopFixture) giveAliceCoin(t *testing.T, cp int64) {
	t.Helper()
	c, err := f.characters.FindByName(context.Background(), "Alice")
	if err != nil {
		t.Fatalf("find alice: %v", err)
	}
	if err := f.characters.RecordCoin(context.Background(), c.ID,
		currency.Amount(cp), c.BankBalance); err != nil {
		t.Fatalf("record coin: %v", err)
	}
}

func (f *shopFixture) listCmd() *telnet.Command {
	return NewList(f.items, f.mobs, f.templates, f.shops, f.hour)
}
func (f *shopFixture) buyCmd() *telnet.Command {
	return NewBuy(f.items, f.characters, f.mobs, f.templates, f.shops, f.hour, f.sessions)
}
func (f *shopFixture) sellCmd() *telnet.Command {
	return NewSell(f.items, f.characters, f.mobs, f.templates, f.shops, f.hour, f.sessions)
}
func (f *shopFixture) valueCmd() *telnet.Command {
	return NewValue(f.items, f.mobs, f.templates, f.shops, f.hour)
}

func TestShop_ListRendersStockAndPrices(t *testing.T) {
	f := newShopFixture(t)
	runCmd(t, f.listCmd(), f.alice, "")
	out := f.aOut.String()
	if !strings.Contains(out, "Bran al'Vere offers") {
		t.Fatalf("missing header; got %q", out)
	}
	if !strings.Contains(out, "a mug of ale") || !strings.Contains(out, "a loaf of bread") {
		t.Fatalf("missing stock rows; got %q", out)
	}
	if !strings.Contains(out, "infinite") {
		t.Fatalf("missing infinite-stock marker; got %q", out)
	}
}

func TestShop_ListClosedShopMessage(t *testing.T) {
	f := newShopFixture(t)
	// Make Bran nocturnal (open 22-6). Hour 12 → closed.
	cur, _ := f.shops.GetByMobTemplateID(context.Background(), f.keeperTpl)
	cur.OpenHour, cur.CloseHour = 22, 6
	// Memory repo Create rejects duplicates; instead we mutate via the
	// internal map. The simpler approach: build a fresh repo with the
	// new config.
	shops := repo.NewMemoryShopRepo()
	newShop, _ := shops.Create(context.Background(), cur)
	stock, _ := f.shops.ListStock(context.Background(), f.shopID)
	for _, row := range stock {
		row.ShopID = newShop.ID
		_ = shops.UpsertStock(context.Background(), row)
	}
	f.shops = shops

	runCmd(t, f.listCmd(), f.alice, "")
	if !strings.Contains(f.aOut.String(), "isn't trading") {
		t.Fatalf("expected closed-shop message, got %q", f.aOut.String())
	}
}

func TestShop_ListNoShopHere(t *testing.T) {
	f := newShopFixture(t)
	f.alice.CurrentRoomID = 999 // empty room
	runCmd(t, f.listCmd(), f.alice, "")
	if !strings.Contains(f.aOut.String(), "no shopkeeper") {
		t.Fatalf("expected 'no shopkeeper' message, got %q", f.aOut.String())
	}
}

func TestShop_BuyTransfersCoinAndStock(t *testing.T) {
	f := newShopFixture(t)
	f.giveAliceCoin(t, 100)

	runCmd(t, f.buyCmd(), f.alice, "ale")

	if !strings.Contains(f.aOut.String(), "You buy a mug of ale") {
		t.Fatalf("missing self echo; got %q", f.aOut.String())
	}
	// Bob (in same room) sees the broadcast.
	if !strings.Contains(f.bOut.String(), "Alice buys a mug of ale from Bran") {
		t.Fatalf("missing broadcast; got %q", f.bOut.String())
	}
	// Stock decremented.
	stock, _ := f.shops.ListStock(context.Background(), f.shopID)
	for _, row := range stock {
		if row.ItemExternalID == "inn.ale" && row.Qty != 2 {
			t.Fatalf("ale qty = %d, want 2", row.Qty)
		}
	}
	// Coin debited.
	c, _ := f.characters.FindByName(context.Background(), "Alice")
	if int64(c.Coin) != 95 { // 100 - 5cp
		t.Fatalf("coin = %d, want 95", int64(c.Coin))
	}
	// Item materialized in inventory.
	held, _ := f.items.ListInInventory(context.Background(), f.alice.CharacterID)
	if len(held) != 1 || held[0].Name != "a mug of ale" {
		t.Fatalf("inventory: %+v", held)
	}
}

func TestShop_BuyInsufficientFunds(t *testing.T) {
	f := newShopFixture(t)
	// Alice has 0cp.
	runCmd(t, f.buyCmd(), f.alice, "ale")
	if !strings.Contains(f.aOut.String(), "don't have") {
		t.Fatalf("expected funds error, got %q", f.aOut.String())
	}
	// Stock unchanged.
	stock, _ := f.shops.ListStock(context.Background(), f.shopID)
	for _, row := range stock {
		if row.ItemExternalID == "inn.ale" && row.Qty != 3 {
			t.Fatalf("ale qty changed to %d on failed buy", row.Qty)
		}
	}
}

func TestShop_BuyOutOfStock(t *testing.T) {
	f := newShopFixture(t)
	f.giveAliceCoin(t, 100)
	// Drain ale to 0.
	_ = f.shops.AdjustStock(context.Background(), f.shopID, "inn.ale", -3)

	runCmd(t, f.buyCmd(), f.alice, "ale")
	if !strings.Contains(f.aOut.String(), "out of those") {
		t.Fatalf("expected out-of-stock, got %q", f.aOut.String())
	}
}

func TestShop_BuyInfiniteStockUnchanged(t *testing.T) {
	f := newShopFixture(t)
	f.giveAliceCoin(t, 100)

	runCmd(t, f.buyCmd(), f.alice, "bread")

	if !strings.Contains(f.aOut.String(), "You buy a loaf of bread") {
		t.Fatalf("buy bread failed; got %q", f.aOut.String())
	}
	stock, _ := f.shops.ListStock(context.Background(), f.shopID)
	for _, row := range stock {
		if row.ItemExternalID == "inn.bread" && row.Qty != -1 {
			t.Fatalf("infinite stock disturbed: qty = %d", row.Qty)
		}
	}
}

func TestShop_SellHalfPriceDefault(t *testing.T) {
	f := newShopFixture(t)
	// Alice carries a 10cp consumable (default markdown 0.5 → 5cp).
	it := f.items.Insert(repo.Item{
		ExternalID: "potion", Name: "a healing potion", NameLower: "a healing potion",
		OwnerCharacterID: f.alice.CharacterID, Type: repo.ItemTypeConsumable, Value: 10, Weight: 1,
	})
	_ = it

	runCmd(t, f.sellCmd(), f.alice, "potion")

	if !strings.Contains(f.aOut.String(), "You sell a healing potion for 5cp") {
		t.Fatalf("expected 5cp sale, got %q", f.aOut.String())
	}
	c, _ := f.characters.FindByName(context.Background(), "Alice")
	if int64(c.Coin) != 5 {
		t.Fatalf("coin = %d, want 5", int64(c.Coin))
	}
}

func TestShop_SellTradeGoodFullPrice(t *testing.T) {
	f := newShopFixture(t)
	f.items.Insert(repo.Item{
		ExternalID: "pelt", Name: "a rabbit pelt", NameLower: "a rabbit pelt",
		OwnerCharacterID: f.alice.CharacterID,
		Type:             repo.ItemTypeTradeGood, Value: 20, Flags: repo.FlagTradeGood,
	})

	runCmd(t, f.sellCmd(), f.alice, "pelt")

	// 20cp renders as "2sp" via the greedy roll-up format.
	if !strings.Contains(f.aOut.String(), "for 2sp") {
		t.Fatalf("expected 2sp (=20cp) full-price for trade good; got %q", f.aOut.String())
	}
	c, _ := f.characters.FindByName(context.Background(), "Alice")
	if int64(c.Coin) != 20 {
		t.Fatalf("coin = %d cp, want 20", int64(c.Coin))
	}
}

func TestShop_SellRefusesNoSellFlag(t *testing.T) {
	f := newShopFixture(t)
	f.items.Insert(repo.Item{
		ExternalID: "heirloom", Name: "an heirloom dagger", NameLower: "an heirloom dagger",
		OwnerCharacterID: f.alice.CharacterID,
		Type:             repo.ItemTypeConsumable, Value: 100, Flags: repo.FlagNoSell,
	})

	runCmd(t, f.sellCmd(), f.alice, "heirloom")

	if !strings.Contains(f.aOut.String(), "won't take that") {
		t.Fatalf("expected NoSell refusal; got %q", f.aOut.String())
	}
}

func TestShop_SellRefusesUnacceptedType(t *testing.T) {
	f := newShopFixture(t)
	// An axe (weapon) — not in BuyTypes ([food, consumable, trade_good]).
	f.items.Insert(repo.Item{
		ExternalID: "axe", Name: "a hand axe", NameLower: "a hand axe",
		OwnerCharacterID: f.alice.CharacterID,
		Type:             repo.ItemTypeWeapon, Value: 50,
	})

	runCmd(t, f.sellCmd(), f.alice, "axe")

	if !strings.Contains(f.aOut.String(), "no use for") {
		t.Fatalf("expected type-refusal; got %q", f.aOut.String())
	}
}

// Regression: a previous implementation derived the keyword from
// c.Raw[len(c.Name):] which silently produced an empty string in
// production (because the dispatcher hands Raw as the post-verb
// remainder, not the full line) and panicked on keywords shorter
// than the verb name. The fix is to read c.Args[0] directly.
func TestShop_BuyShortKeywordDoesNotPanic(t *testing.T) {
	f := newShopFixture(t)
	f.giveAliceCoin(t, 100)
	// "x" is shorter than len("buy")=3 — would have panicked with
	// the old c.Raw[len(c.Name):] slicing. Should miss cleanly.
	runCmd(t, f.buyCmd(), f.alice, "x")
	if !strings.Contains(f.aOut.String(), "doesn't carry") {
		t.Fatalf("expected miss message, got %q", f.aOut.String())
	}
}

func TestShop_ValueShortKeywordDoesNotPanic(t *testing.T) {
	f := newShopFixture(t)
	// "x" is shorter than len("value")=5 — same panic risk.
	runCmd(t, f.valueCmd(), f.alice, "x")
	// Either inventory or shop miss is acceptable; the goal is no panic.
	out := f.aOut.String()
	if !strings.Contains(out, "Neither you") && !strings.Contains(out, "no use") {
		t.Fatalf("expected miss-style message, got %q", out)
	}
}

func TestShop_ValueNoSideEffects(t *testing.T) {
	f := newShopFixture(t)
	f.giveAliceCoin(t, 100)
	f.items.Insert(repo.Item{
		ExternalID: "potion", Name: "a healing potion", NameLower: "a healing potion",
		OwnerCharacterID: f.alice.CharacterID, Type: repo.ItemTypeConsumable, Value: 10,
	})

	runCmd(t, f.valueCmd(), f.alice, "potion")
	if !strings.Contains(f.aOut.String(), "would pay you 5cp") {
		t.Fatalf("expected sell preview; got %q", f.aOut.String())
	}

	// Inventory + coin unchanged.
	held, _ := f.items.ListInInventory(context.Background(), f.alice.CharacterID)
	if len(held) != 1 {
		t.Fatalf("inventory disturbed: %d items", len(held))
	}
	c, _ := f.characters.FindByName(context.Background(), "Alice")
	if int64(c.Coin) != 100 {
		t.Fatalf("coin disturbed: %d", int64(c.Coin))
	}

	// Stock-side preview.
	f.aOut.Reset()
	runCmd(t, f.valueCmd(), f.alice, "bread")
	if !strings.Contains(f.aOut.String(), "sells a loaf of bread for") {
		t.Fatalf("expected stock preview; got %q", f.aOut.String())
	}
}
