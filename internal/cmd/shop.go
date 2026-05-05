package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/currency"
	"github.com/Jasrags/WheelMUD/internal/repo"
	"github.com/Jasrags/WheelMUD/internal/session"
	"github.com/Jasrags/WheelMUD/telnet"
)

// HourSource is the narrow read of internal/world.Clock the shop verbs
// need. Defined here so the verb tests don't have to construct a real
// Clock — a func value satisfies it. The runtime wires *world.Clock.
type HourSource interface {
	HourOfDay() int
}

// ErrNoShopHere is the sentinel for "no shopkeeper in the current room".
// Callers translate it into a player-facing line.
var errNoShopHere = errors.New("cmd: no shop here")

// resolvedShop pairs a Shop config with the mob in the room that backs
// it. Returned by findShopkeeper; nil shop pointer means lookup failed.
type resolvedShop struct {
	shop      repo.Shop
	keeper    creature.MobInstance
	keeperTpl creature.MobTemplate
}

// keeperName returns a player-friendly name for the shopkeeper.
// Falls back to "the shopkeeper" if the template name is missing —
// the cfmt-safety policy elsewhere already strips brace/colon bytes
// at character_create, so the raw Core.Name is fine here.
func (r resolvedShop) keeperName() string {
	if r.keeperTpl.Core.Name != "" {
		return r.keeperTpl.Core.Name
	}
	return "the shopkeeper"
}

// findShopkeeper walks the mobs in roomID and returns the first one
// whose template has a matching shops row. Returns errNoShopHere if
// the room has no shop-capable mob.
func findShopkeeper(ctx context.Context, roomID int64,
	mobs repo.MobInstanceRepo, templates repo.MobTemplateRepo, shops repo.ShopRepo,
) (resolvedShop, error) {
	if roomID == 0 {
		return resolvedShop{}, errNoShopHere
	}
	occupants, err := mobs.ListInRoom(ctx, roomID)
	if err != nil {
		return resolvedShop{}, fmt.Errorf("list room mobs: %w", err)
	}
	for _, m := range occupants {
		shop, err := shops.GetByMobTemplateID(ctx, m.TemplateID)
		if errors.Is(err, repo.ErrShopNotFound) {
			continue
		}
		if err != nil {
			return resolvedShop{}, fmt.Errorf("get shop: %w", err)
		}
		tpl, err := templates.GetByID(ctx, m.TemplateID)
		if err != nil {
			return resolvedShop{}, fmt.Errorf("get template: %w", err)
		}
		return resolvedShop{shop: shop, keeper: m, keeperTpl: tpl}, nil
	}
	return resolvedShop{}, errNoShopHere
}

// priceToBuy is the player-pays-shop price for one unit of item.
// Shops never sell for less than 1cp — a free item exposes a coin-econ
// hole. Round-half-up for predictable pricing.
func priceToBuy(value currency.Amount, shop repo.Shop) currency.Amount {
	cents := math.Round(float64(int64(value)) * shop.SellMarkup)
	if cents < 1 {
		cents = 1
	}
	return currency.Amount(int64(cents))
}

// priceToSell is the shop-pays-player price for one unit of item.
// FlagTradeGood items pay the full Value (the WoT tradegood rule
// PLAN.md:88 calls out). Everything else pays Value × BuyMarkdown
// (default 0.5 → half price). Floors at 0 — nothing pays negative.
func priceToSell(it repo.Item, shop repo.Shop) currency.Amount {
	if it.HasFlag(repo.FlagTradeGood) {
		return it.Value
	}
	cents := math.Round(float64(int64(it.Value)) * shop.BuyMarkdown)
	if cents < 0 {
		cents = 0
	}
	return currency.Amount(int64(cents))
}

// shopOpen reports whether the shop is open right now per the clock.
// A nil HourSource is treated as always-open (test default).
func shopOpen(shop repo.Shop, clock HourSource) bool {
	if clock == nil {
		return true
	}
	return shop.IsOpenAt(clock.HourOfDay())
}

// matchStockKeyword finds the first stock row whose backing item's
// name token-prefix matches `keyword` (or whose external_id matches
// exactly). Returns the row + the resolved item template, or
// (zero, false) on miss.
func matchStockKeyword(ctx context.Context, keyword string,
	stock []repo.ShopStockRow, items repo.ItemRepo,
) (repo.ShopStockRow, repo.Item, bool) {
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	if keyword == "" {
		return repo.ShopStockRow{}, repo.Item{}, false
	}
	tplsByName := make([]repo.Item, 0, len(stock))
	rowsByItemID := make(map[int64]repo.ShopStockRow, len(stock))
	for _, row := range stock {
		tpl, err := items.FindByExternalID(ctx, row.ItemExternalID)
		if err != nil {
			continue
		}
		tplsByName = append(tplsByName, tpl)
		rowsByItemID[tpl.ID] = row
	}
	if it, ok := MatchItem(keyword, tplsByName); ok {
		return rowsByItemID[it.ID], it, true
	}
	return repo.ShopStockRow{}, repo.Item{}, false
}

// renderStockLine formats one row of the `list` table.
func renderStockLine(it repo.Item, row repo.ShopStockRow, price currency.Amount) string {
	stock := "infinite"
	if row.QtyMax >= 0 {
		stock = fmt.Sprintf("%d", row.Qty)
	}
	return fmt.Sprintf("  %-32s %12s   %s\r\n", it.Name, price.Format(), stock)
}

// NewList builds the `list` verb. Shows the in-room shopkeeper's
// wares with prices and stock counts.
func NewList(items repo.ItemRepo, mobs repo.MobInstanceRepo,
	templates repo.MobTemplateRepo, shops repo.ShopRepo, clock HourSource,
) *telnet.Command {
	return &telnet.Command{
		Name: "list",
		Help: "List — show what the shopkeeper here is selling",
		Auth: telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			s := c.Session
			res, err := findShopkeeper(c.Ctx, s.CurrentRoomID, mobs, templates, shops)
			if errors.Is(err, errNoShopHere) {
				return s.WriteString("{{There's no shopkeeper here.}}::yellow\r\n")
			}
			if err != nil {
				slog.Error("list: shopkeeper lookup", "error", err)
				return s.WriteString("{{The shop isn't open right now.}}::red\r\n")
			}
			if !shopOpen(res.shop, clock) {
				return s.WriteString(fmt.Sprintf("{{%s isn't trading right now. Come back later.}}::yellow\r\n", res.keeperName()))
			}
			rows, err := shops.ListStock(c.Ctx, res.shop.ID)
			if err != nil {
				slog.Error("list: liststock", "shop", res.shop.ID, "error", err)
				return s.WriteString("{{The shop's ledgers are a mess.}}::red\r\n")
			}
			var b strings.Builder
			b.WriteString(fmt.Sprintf("{{%s offers:}}::green|bold\r\n", res.keeperName()))
			if len(rows) == 0 {
				b.WriteString("  {{(nothing for sale)}}::gray\r\n")
				return s.WriteString(b.String())
			}
			b.WriteString(fmt.Sprintf("{{  %-32s %12s   %s}}::yellow|bold\r\n", "Item", "Price", "Stock"))
			for _, row := range rows {
				tpl, err := items.FindByExternalID(c.Ctx, row.ItemExternalID)
				if err != nil {
					continue
				}
				b.WriteString(renderStockLine(tpl, row, priceToBuy(tpl.Value, res.shop)))
			}
			return s.WriteString(b.String())
		},
	}
}

// NewBuy builds the `buy <keyword>` verb. Decrements stock, debits
// coin, materializes a fresh item in the buyer's inventory, broadcasts
// to the room. V1 buys one unit at a time.
func NewBuy(items repo.ItemRepo, characters repo.CharacterRepo,
	mobs repo.MobInstanceRepo, templates repo.MobTemplateRepo,
	shops repo.ShopRepo, clock HourSource, sessions *session.Registry,
) *telnet.Command {
	return &telnet.Command{
		Name:    "buy",
		Help:    "Buy <item> — purchase an item from the shopkeeper here",
		MinArgs: 1,
		Long:    "Usage: buy <item>\n\nMatches by name keyword (prefix or whole-word).",
		Auth:    telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			s := c.Session
			res, err := findShopkeeper(c.Ctx, s.CurrentRoomID, mobs, templates, shops)
			if errors.Is(err, errNoShopHere) {
				return s.WriteString("{{There's no shopkeeper here.}}::yellow\r\n")
			}
			if err != nil {
				slog.Error("buy: shopkeeper lookup", "error", err)
				return s.WriteString("{{The shop isn't open right now.}}::red\r\n")
			}
			if !shopOpen(res.shop, clock) {
				return s.WriteString(fmt.Sprintf("{{%s isn't trading right now.}}::yellow\r\n", res.keeperName()))
			}

			stock, err := shops.ListStock(c.Ctx, res.shop.ID)
			if err != nil {
				slog.Error("buy: liststock", "shop", res.shop.ID, "error", err)
				return s.WriteString("{{The shop's ledgers are a mess.}}::red\r\n")
			}
			row, tpl, ok := matchStockKeyword(c.Ctx, c.Args[0], stock, items)
			if !ok {
				return s.WriteString(fmt.Sprintf("{{%s doesn't carry that.}}::yellow\r\n", res.keeperName()))
			}
			price := priceToBuy(tpl.Value, res.shop)

			// Coin first — cheaper to fail before mutating stock.
			char, err := characters.FindByName(c.Ctx, s.CharacterName)
			if err != nil {
				slog.Error("buy: char lookup", "char", s.CharacterID, "error", err)
				return s.WriteString("{{You can't seem to find your purse.}}::red\r\n")
			}
			newCoin, err := char.Coin.Sub(price)
			if err != nil {
				if errors.Is(err, currency.ErrInsufficientFunds) {
					return s.WriteString(fmt.Sprintf("{{You don't have %s.}}::yellow\r\n", price.Format()))
				}
				return s.WriteString("{{Something went wrong with the coin.}}::red\r\n")
			}

			// Stock decrement (CAS). Race with another buyer or the
			// restocker surfaces as ErrOutOfStock / ErrShopNotFound.
			if err := shops.AdjustStock(c.Ctx, res.shop.ID, row.ItemExternalID, -1); err != nil {
				if errors.Is(err, repo.ErrOutOfStock) {
					return s.WriteString(fmt.Sprintf("{{%s is out of those.}}::yellow\r\n", res.keeperName()))
				}
				if errors.Is(err, repo.ErrShopNotFound) {
					return s.WriteString(fmt.Sprintf("{{%s no longer carries that.}}::yellow\r\n", res.keeperName()))
				}
				slog.Error("buy: adjust stock", "error", err)
				return s.WriteString("{{The deal falls through.}}::red\r\n")
			}

			// Materialize the item in the buyer's inventory.
			now := time.Now().UnixNano()
			stats := cloneItemStats(tpl.Stats)
			if stats == nil {
				// Some seeded templates (esp. test fixtures) skip the
				// typed stats sub-record. The repo's stats-type check
				// rejects Create on stats-required types with nil. Mint
				// a zero-valued stats struct so the buy doesn't trip
				// over fixture sloppiness.
				stats = repo.StatsForType(tpl.Type)
			}
			fresh := repo.Item{
				ExternalID:       fmt.Sprintf("%s#shop-%d", tpl.ExternalID, now),
				Name:             tpl.Name,
				NameLower:        tpl.NameLower,
				ShortDesc:        tpl.ShortDesc,
				OwnerCharacterID: s.CharacterID,
				Type:             tpl.Type,
				Weight:           tpl.Weight,
				Value:            tpl.Value,
				Quality:          tpl.Quality,
				Flags:            tpl.Flags,
				Stats:            stats,
			}
			if _, err := items.Create(c.Ctx, fresh); err != nil {
				slog.Error("buy: item create", "ext", tpl.ExternalID, "error", err)
				// Best-effort stock rollback. Use a fresh ctx with a
				// tight timeout — c.Ctx may already be canceled (the
				// disconnect that broke item Create), and we don't want
				// the rollback to hang indefinitely. If the rollback
				// itself fails, the row stays one short until the
				// restocker catches up.
				rbCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				if rbErr := shops.AdjustStock(rbCtx, res.shop.ID, row.ItemExternalID, +1); rbErr != nil {
					slog.Error("buy: stock rollback failed",
						"shop", res.shop.ID, "item", row.ItemExternalID, "error", rbErr)
				}
				cancel()
				return s.WriteString("{{The shopkeeper fumbles and the deal falls apart.}}::red\r\n")
			}

			// Debit coin. If this fails after the item exists, the
			// buyer keeps the item with a full purse — log and accept
			// the lost revenue rather than risk losing the item too.
			if err := characters.RecordCoin(c.Ctx, char.ID, newCoin, char.BankBalance); err != nil {
				slog.Error("buy: record coin", "char", char.ID, "error", err)
			}

			actor := safeActor(s)
			broadcastRoom(sessions, s.CurrentRoomID, s,
				fmt.Sprintf("{{%s buys %s from %s.}}::cyan\r\n", actor, tpl.Name, res.keeperName()))
			return s.WriteString(fmt.Sprintf("{{You buy %s for %s.}}::cyan\r\n", tpl.Name, price.Format()))
		},
	}
}

// NewSell builds the `sell <keyword>` verb. Looks the item up in the
// player's inventory, refuses NoSell or out-of-whitelist types,
// credits the player, deletes the item.
func NewSell(items repo.ItemRepo, characters repo.CharacterRepo,
	mobs repo.MobInstanceRepo, templates repo.MobTemplateRepo,
	shops repo.ShopRepo, clock HourSource, sessions *session.Registry,
) *telnet.Command {
	return &telnet.Command{
		Name:    "sell",
		Help:    "Sell <item> — sell something from your inventory",
		MinArgs: 1,
		Long:    "Usage: sell <item>\n\nThe shopkeeper only buys items they're set up to handle.",
		Auth:    telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			s := c.Session
			res, err := findShopkeeper(c.Ctx, s.CurrentRoomID, mobs, templates, shops)
			if errors.Is(err, errNoShopHere) {
				return s.WriteString("{{There's no shopkeeper here.}}::yellow\r\n")
			}
			if err != nil {
				slog.Error("sell: shopkeeper lookup", "error", err)
				return s.WriteString("{{The shop isn't open right now.}}::red\r\n")
			}
			if !shopOpen(res.shop, clock) {
				return s.WriteString(fmt.Sprintf("{{%s isn't trading right now.}}::yellow\r\n", res.keeperName()))
			}

			held, err := items.ListInInventory(c.Ctx, s.CharacterID)
			if err != nil {
				slog.Error("sell: list inv", "char", s.CharacterID, "error", err)
				return s.WriteString("{{You can't focus on what you're carrying.}}::red\r\n")
			}
			keyword := c.Args[0]
			it, ok := MatchItem(strings.ToLower(keyword), held)
			if !ok {
				return s.WriteString("{{You aren't carrying that.}}::yellow\r\n")
			}
			if it.HasFlag(repo.FlagNoSell) {
				return s.WriteString(fmt.Sprintf("{{%s won't take that.}}::yellow\r\n", res.keeperName()))
			}
			if !res.shop.AcceptsType(it.Type) {
				return s.WriteString(fmt.Sprintf("{{%s has no use for %s.}}::yellow\r\n", res.keeperName(), it.Name))
			}

			price := priceToSell(it, res.shop)
			char, err := characters.FindByName(c.Ctx, s.CharacterName)
			if err != nil {
				slog.Error("sell: char lookup", "char", s.CharacterID, "error", err)
				return s.WriteString("{{You can't find your purse.}}::red\r\n")
			}
			newCoin, err := char.Coin.Add(price)
			if err != nil {
				return s.WriteString("{{You can't carry any more coin.}}::yellow\r\n")
			}

			autoUnequipIfHeld(c, characters, it.ID)

			// Credit coin first. If this fails, the player keeps the
			// item — better UX than the inverse (deleting first would
			// lose the item if credit then fails, with no rollback path
			// since Delete is destructive).
			if err := characters.RecordCoin(c.Ctx, char.ID, newCoin, char.BankBalance); err != nil {
				slog.Error("sell: record coin", "char", char.ID, "error", err)
				return s.WriteString("{{The shopkeeper fumbles the change.}}::red\r\n")
			}
			if err := items.Delete(c.Ctx, it.ID); err != nil {
				// Coin already credited; player got paid and kept the
				// item. Loud log — this is real revenue created from
				// nothing — but accept rather than try to claw back
				// coin (RecordCoin failure is unlikely so close to
				// success, but the rollback can't be guaranteed).
				slog.Error("sell: delete item AFTER coin credited — duplicated value",
					"char", char.ID, "item", it.ID, "amount_cp", int64(price), "error", err)
				return s.WriteString("{{You sell " + it.Name + " for " + price.Format() + ".}}::cyan\r\n")
			}
			// Remove from inventory ordering JSON best-effort.
			_ = characters.RecordInventory(c.Ctx, char.ID, removeID(char.Inventory, it.ID))

			actor := safeActor(s)
			broadcastRoom(sessions, s.CurrentRoomID, s,
				fmt.Sprintf("{{%s sells %s to %s.}}::cyan\r\n", actor, it.Name, res.keeperName()))
			return s.WriteString(fmt.Sprintf("{{You sell %s for %s.}}::cyan\r\n", it.Name, price.Format()))
		},
	}
}

// NewValue builds the `value <keyword>` verb — a no-side-effect price
// preview for either an inventory item (sell preview) or a stocked
// item (buy preview). Inventory match wins if both apply.
func NewValue(items repo.ItemRepo,
	mobs repo.MobInstanceRepo, templates repo.MobTemplateRepo,
	shops repo.ShopRepo, clock HourSource,
) *telnet.Command {
	return &telnet.Command{
		Name:    "value",
		Help:    "Value <item> — preview the shopkeeper's price for an item",
		MinArgs: 1,
		Long:    "Usage: value <item>\n\nPreviews the sell price for an item you're carrying, or\nthe buy price for an item the shop is selling.",
		Auth:    telnet.AuthPlayer,
		Run: func(c *telnet.Context) error {
			s := c.Session
			res, err := findShopkeeper(c.Ctx, s.CurrentRoomID, mobs, templates, shops)
			if errors.Is(err, errNoShopHere) {
				return s.WriteString("{{There's no shopkeeper here.}}::yellow\r\n")
			}
			if err != nil {
				slog.Error("value: shopkeeper lookup", "error", err)
				return s.WriteString("{{The shop isn't open right now.}}::red\r\n")
			}
			keyword := c.Args[0]
			lower := strings.ToLower(keyword)

			// Inventory first.
			held, err := items.ListInInventory(c.Ctx, s.CharacterID)
			if err == nil {
				if it, ok := MatchItem(lower, held); ok {
					if it.HasFlag(repo.FlagNoSell) || !res.shop.AcceptsType(it.Type) {
						return s.WriteString(fmt.Sprintf("{{%s has no use for %s.}}::yellow\r\n", res.keeperName(), it.Name))
					}
					return s.WriteString(fmt.Sprintf("{{%s would pay you %s for %s.}}::cyan\r\n",
						res.keeperName(), priceToSell(it, res.shop).Format(), it.Name))
				}
			}

			// Fall through to shop stock.
			stock, err := shops.ListStock(c.Ctx, res.shop.ID)
			if err == nil {
				if _, tpl, ok := matchStockKeyword(c.Ctx, keyword, stock, items); ok {
					return s.WriteString(fmt.Sprintf("{{%s sells %s for %s.}}::cyan\r\n",
						res.keeperName(), tpl.Name, priceToBuy(tpl.Value, res.shop).Format()))
				}
			}
			return s.WriteString(fmt.Sprintf("{{Neither you nor %s has that.}}::yellow\r\n", res.keeperName()))
		},
	}
}
