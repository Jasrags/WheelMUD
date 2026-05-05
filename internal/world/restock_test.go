package world

import (
	"context"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/repo"
)

// fakeClock returns a fixed time, advanced via mu.set.
type fakeClock struct{ now time.Time }

func (f *fakeClock) Now() time.Time { return f.now }

func TestRestocker_RefillsAfterInterval(t *testing.T) {
	ctx := context.Background()
	shops := repo.NewMemoryShopRepo()
	shop, err := shops.Create(ctx, repo.Shop{
		MobTemplateID:    1,
		RestockIntervalS: 60,
	})
	if err != nil {
		t.Fatalf("create shop: %v", err)
	}
	if err := shops.UpsertStock(ctx, repo.ShopStockRow{
		ShopID: shop.ID, ItemExternalID: "ale", Qty: 2, QtyMax: 12,
		LastRestockTs: 1000,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	clock := &fakeClock{now: time.Unix(1059, 0)}
	r := &Restocker{Shops: shops, Now: clock.Now}

	// 59 seconds elapsed — short of the 60s interval, no refill.
	r.Tick(ctx)
	got, _ := shops.ListStock(ctx, shop.ID)
	if got[0].Qty != 2 {
		t.Fatalf("premature refill: qty = %d", got[0].Qty)
	}

	// Advance to 1061 — past the interval. Refill to qty_max.
	clock.now = time.Unix(1061, 0)
	r.Tick(ctx)
	got, _ = shops.ListStock(ctx, shop.ID)
	if got[0].Qty != 12 {
		t.Fatalf("did not refill: qty = %d, want 12", got[0].Qty)
	}
	if got[0].LastRestockTs != 1061 {
		t.Fatalf("ts not stamped: %d", got[0].LastRestockTs)
	}
}

func TestRestocker_LeavesInfiniteStockAlone(t *testing.T) {
	ctx := context.Background()
	shops := repo.NewMemoryShopRepo()
	shop, _ := shops.Create(ctx, repo.Shop{MobTemplateID: 1, RestockIntervalS: 60})
	_ = shops.UpsertStock(ctx, repo.ShopStockRow{
		ShopID: shop.ID, ItemExternalID: "torch", Qty: -1, QtyMax: -1,
	})

	r := &Restocker{Shops: shops, Now: func() time.Time { return time.Unix(1_000_000, 0) }}
	r.Tick(ctx)

	got, _ := shops.ListStock(ctx, shop.ID)
	if got[0].Qty != -1 {
		t.Fatalf("infinite-stock disturbed: qty = %d", got[0].Qty)
	}
}

func TestRestocker_LeavesAtMaxAlone(t *testing.T) {
	ctx := context.Background()
	shops := repo.NewMemoryShopRepo()
	shop, _ := shops.Create(ctx, repo.Shop{MobTemplateID: 1, RestockIntervalS: 60})
	_ = shops.UpsertStock(ctx, repo.ShopStockRow{
		ShopID: shop.ID, ItemExternalID: "ale", Qty: 12, QtyMax: 12,
		LastRestockTs: 0,
	})
	r := &Restocker{Shops: shops, Now: func() time.Time { return time.Unix(1_000_000, 0) }}
	r.Tick(ctx)

	got, _ := shops.ListStock(ctx, shop.ID)
	if got[0].LastRestockTs != 0 {
		t.Fatalf("at-max row's ts mutated: %d", got[0].LastRestockTs)
	}
}

func TestRestocker_NilShopsRepoIsNoop(t *testing.T) {
	(&Restocker{}).Tick(context.Background()) // must not panic
}
