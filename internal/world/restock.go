package world

import (
	"context"
	"log/slog"
	"time"

	"github.com/Jasrags/WheelMUD/internal/repo"
)

// Restocker walks every shop on each tick and refills any
// sub-max stock line whose last_restock_ts is older than the
// shop's restock_interval_s. Wired into tick.Buckets.AreaReset
// (5-minute default cadence) so refills feel ambient rather than
// instantaneous.
//
// The infinite-stock sentinel (qty_max < 0) is left alone — those
// lines never deplete.
type Restocker struct {
	Shops repo.ShopRepo
	// Now overrides time.Now for deterministic tests. Production
	// leaves it nil and falls back to time.Now.
	Now func() time.Time
}

// NewRestocker is a small constructor matching the wiring style of
// the wanderer / phase-ambient watchers in cmd/server/main.go.
func NewRestocker(shops repo.ShopRepo) *Restocker {
	return &Restocker{Shops: shops}
}

// Tick is the bucket subscription. Errors are logged and swallowed —
// one shop's broken row must not stop the rest from refilling.
func (r *Restocker) Tick(ctx context.Context) {
	if r == nil || r.Shops == nil {
		return
	}
	now := time.Now
	if r.Now != nil {
		now = r.Now
	}
	nowSec := now().Unix()

	shops, err := r.Shops.ListShops(ctx)
	if err != nil {
		slog.Warn("restock: list shops failed", "error", err)
		return
	}
	for _, shop := range shops {
		stock, err := r.Shops.ListStock(ctx, shop.ID)
		if err != nil {
			slog.Warn("restock: list stock failed", "shop", shop.ID, "error", err)
			continue
		}
		interval := int64(shop.RestockIntervalS)
		if interval <= 0 {
			continue
		}
		for _, row := range stock {
			if row.QtyMax < 0 || row.Qty >= row.QtyMax {
				continue
			}
			if nowSec-row.LastRestockTs < interval {
				continue
			}
			row.Qty = row.QtyMax
			row.LastRestockTs = nowSec
			if err := r.Shops.UpsertStock(ctx, row); err != nil {
				slog.Warn("restock: upsert failed",
					"shop", shop.ID, "item", row.ItemExternalID, "error", err)
			}
		}
	}
}
