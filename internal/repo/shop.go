package repo

import (
	"context"
	"errors"
)

// Shop is one shopkeeper-NPC's pricing + hours config, keyed 1:1 to a
// MobTemplate. Stock is held separately in ShopStockRow rows so the
// per-line restock timer doesn't have to rewrite the parent config.
//
// Pricing model:
//   buy price  = round(item.value * SellMarkup)
//   sell price = item.value          when FlagTradeGood is set (full)
//              = round(item.value * BuyMarkdown)  otherwise (½ default)
//
// Hours: OpenHour == CloseHour means always open (24h). Otherwise the
// shop is open when wall-hour ∈ [OpenHour, CloseHour) — wraps across
// midnight when CloseHour < OpenHour (e.g. 22→4 covers a tavern's
// late-night hours).
type Shop struct {
	ID               int64
	MobTemplateID    int64
	BuyTypes         []ItemType
	SellMarkup       float64
	BuyMarkdown      float64
	OpenHour         int
	CloseHour        int
	RestockIntervalS int
}

// AcceptsType reports whether the shop's BuyTypes whitelist includes t.
// An empty whitelist refuses every sell.
func (s Shop) AcceptsType(t ItemType) bool {
	for _, allowed := range s.BuyTypes {
		if allowed == t {
			return true
		}
	}
	return false
}

// IsOpenAt reports whether the shop is open at the given wall-hour
// (0..23). OpenHour == CloseHour is the always-open sentinel.
func (s Shop) IsOpenAt(hour int) bool {
	if s.OpenHour == s.CloseHour {
		return true
	}
	if s.OpenHour < s.CloseHour {
		return hour >= s.OpenHour && hour < s.CloseHour
	}
	// Wraps across midnight, e.g. open=22, close=4.
	return hour >= s.OpenHour || hour < s.CloseHour
}

// ShopStockRow is one inventory line for a shop. ItemExternalID points
// at an item template (items.external_id with no owner/room/parent —
// the spawn path materializes a fresh row). Qty == -1 is the
// "infinite stock" sentinel; QtyMax == -1 disables restock.
type ShopStockRow struct {
	ID             int64
	ShopID         int64
	ItemExternalID string
	Qty            int
	QtyMax         int
	LastRestockTs  int64 // unix seconds; 0 = never restocked
}

// IsInfinite reports whether the stock row is configured for infinite
// supply (qty/qty_max both -1).
func (r ShopStockRow) IsInfinite() bool { return r.QtyMax < 0 }

// ShopRepo persists shopkeeper config + per-shop stock. Shops are
// re-created from YAML by the world loader on startup; mid-runtime
// edits go through the verbs (AdjustStock for buy/sell) and the
// restocker (UpsertStock + StampRestock).
type ShopRepo interface {
	// Create inserts a new shop config. MobTemplateID must be non-zero
	// and unique. Returns the row with its assigned ID populated.
	Create(ctx context.Context, s Shop) (Shop, error)
	// GetByMobTemplateID returns the shop attached to the given
	// template id, or ErrShopNotFound.
	GetByMobTemplateID(ctx context.Context, mobTemplateID int64) (Shop, error)
	// ListShops returns every shop, sorted by ID. Used by the
	// restocker tick.
	ListShops(ctx context.Context) ([]Shop, error)
	// ListStock returns every stock row for the shop, sorted by
	// item_external_id.
	ListStock(ctx context.Context, shopID int64) ([]ShopStockRow, error)
	// UpsertStock creates or replaces a stock row keyed by
	// (shop_id, item_external_id). Used by the world loader and the
	// restocker. Sell paths intentionally do NOT call this — V1 keeps
	// player-sold goods out of shop inventory.
	UpsertStock(ctx context.Context, row ShopStockRow) error
	// AdjustStock atomically applies delta to qty for the given
	// (shop, item_external_id) pair. Refuses to take qty below zero
	// (returns ErrOutOfStock). The qty == -1 sentinel for infinite
	// stock is a no-op on negative deltas. Returns ErrShopNotFound if
	// the row doesn't exist.
	AdjustStock(ctx context.Context, shopID int64, itemExternalID string, delta int) error
	// StampRestock writes lastRestockTs onto the (shop, item) row.
	// Called by the restocker after a refill.
	StampRestock(ctx context.Context, shopID int64, itemExternalID string, ts int64) error
}

// ErrShopNotFound is returned when a Shop or ShopStockRow lookup
// misses. Callers translate this into "this isn't a shop" or "this
// shopkeeper doesn't carry that".
var ErrShopNotFound = errors.New("repo: shop not found")

// ErrOutOfStock is returned by AdjustStock when applying delta would
// take qty negative. The buy verb surfaces this as "Bran is out of
// that — try later."
var ErrOutOfStock = errors.New("repo: shop out of stock")
