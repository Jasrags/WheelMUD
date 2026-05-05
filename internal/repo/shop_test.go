package repo

import (
	"context"
	"errors"
	"testing"

	"github.com/Jasrags/WheelMUD/internal/db"
)

func runShopRepoTests(t *testing.T, name string, newRepo func(t *testing.T) ShopRepo) {
	t.Helper()

	t.Run(name+"/create_and_get_round_trip", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		seed := Shop{
			MobTemplateID:    42,
			BuyTypes:         []ItemType{ItemTypeFood, ItemTypeTradeGood},
			SellMarkup:       1.2,
			BuyMarkdown:      0.5,
			OpenHour:         6,
			CloseHour:        22,
			RestockIntervalS: 3600,
		}
		got, err := r.Create(ctx, seed)
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if got.ID == 0 {
			t.Fatal("ID not assigned")
		}
		fetched, err := r.GetByMobTemplateID(ctx, 42)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if fetched.SellMarkup != 1.2 || fetched.BuyMarkdown != 0.5 ||
			fetched.OpenHour != 6 || fetched.CloseHour != 22 ||
			fetched.RestockIntervalS != 3600 {
			t.Fatalf("scalars round-tripped wrong: %+v", fetched)
		}
		if len(fetched.BuyTypes) != 2 ||
			fetched.BuyTypes[0] != ItemTypeFood || fetched.BuyTypes[1] != ItemTypeTradeGood {
			t.Fatalf("buy_types round-tripped wrong: %+v", fetched.BuyTypes)
		}
	})

	t.Run(name+"/create_rejects_zero_template", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		if _, err := r.Create(ctx, Shop{}); err == nil {
			t.Fatal("expected error on zero MobTemplateID")
		}
	})

	t.Run(name+"/create_rejects_duplicate", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		if _, err := r.Create(ctx, Shop{MobTemplateID: 5}); err != nil {
			t.Fatalf("first create: %v", err)
		}
		if _, err := r.Create(ctx, Shop{MobTemplateID: 5}); !errors.Is(err, ErrDuplicateExternalID) {
			t.Fatalf("dup err = %v, want ErrDuplicateExternalID", err)
		}
	})

	t.Run(name+"/get_missing_returns_not_found", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		if _, err := r.GetByMobTemplateID(ctx, 999); !errors.Is(err, ErrShopNotFound) {
			t.Fatalf("err = %v, want ErrShopNotFound", err)
		}
	})

	t.Run(name+"/list_shops_sorted_by_id", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		if _, err := r.Create(ctx, Shop{MobTemplateID: 10}); err != nil {
			t.Fatalf("create A: %v", err)
		}
		if _, err := r.Create(ctx, Shop{MobTemplateID: 11}); err != nil {
			t.Fatalf("create B: %v", err)
		}
		got, err := r.ListShops(ctx)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 2 || got[0].ID >= got[1].ID {
			t.Fatalf("list order wrong: %+v", got)
		}
	})

	t.Run(name+"/upsert_and_list_stock", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		shop, err := r.Create(ctx, Shop{MobTemplateID: 1})
		if err != nil {
			t.Fatalf("create shop: %v", err)
		}
		if err := r.UpsertStock(ctx, ShopStockRow{
			ShopID: shop.ID, ItemExternalID: "tr.bread", Qty: 5, QtyMax: 10,
		}); err != nil {
			t.Fatalf("upsert bread: %v", err)
		}
		if err := r.UpsertStock(ctx, ShopStockRow{
			ShopID: shop.ID, ItemExternalID: "tr.ale", Qty: -1, QtyMax: -1,
		}); err != nil {
			t.Fatalf("upsert ale: %v", err)
		}
		// Re-upsert bread to update qty (idempotent on conflict).
		if err := r.UpsertStock(ctx, ShopStockRow{
			ShopID: shop.ID, ItemExternalID: "tr.bread", Qty: 7, QtyMax: 10,
		}); err != nil {
			t.Fatalf("re-upsert bread: %v", err)
		}
		got, err := r.ListStock(ctx, shop.ID)
		if err != nil {
			t.Fatalf("list stock: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("got %d rows, want 2", len(got))
		}
		// Sorted by item_external_id: ale first.
		if got[0].ItemExternalID != "tr.ale" || got[1].ItemExternalID != "tr.bread" {
			t.Fatalf("sort wrong: %+v", got)
		}
		if got[1].Qty != 7 {
			t.Fatalf("re-upsert didn't update qty: %d", got[1].Qty)
		}
	})

	t.Run(name+"/adjust_stock_decrements_and_guards", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		shop, _ := r.Create(ctx, Shop{MobTemplateID: 1})
		_ = r.UpsertStock(ctx, ShopStockRow{
			ShopID: shop.ID, ItemExternalID: "axe", Qty: 2, QtyMax: 5,
		})
		if err := r.AdjustStock(ctx, shop.ID, "axe", -1); err != nil {
			t.Fatalf("decrement: %v", err)
		}
		if err := r.AdjustStock(ctx, shop.ID, "axe", -1); err != nil {
			t.Fatalf("decrement to zero: %v", err)
		}
		// Third decrement crosses zero → ErrOutOfStock.
		if err := r.AdjustStock(ctx, shop.ID, "axe", -1); !errors.Is(err, ErrOutOfStock) {
			t.Fatalf("third decrement err = %v, want ErrOutOfStock", err)
		}
		got, _ := r.ListStock(ctx, shop.ID)
		if got[0].Qty != 0 {
			t.Fatalf("qty = %d, want 0 after failed decrement", got[0].Qty)
		}
	})

	t.Run(name+"/adjust_stock_clamps_to_max", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		shop, _ := r.Create(ctx, Shop{MobTemplateID: 1})
		_ = r.UpsertStock(ctx, ShopStockRow{
			ShopID: shop.ID, ItemExternalID: "axe", Qty: 4, QtyMax: 5,
		})
		if err := r.AdjustStock(ctx, shop.ID, "axe", 10); err != nil {
			t.Fatalf("over-increment: %v", err)
		}
		got, _ := r.ListStock(ctx, shop.ID)
		if got[0].Qty != 5 {
			t.Fatalf("clamp failed: qty = %d, want 5", got[0].Qty)
		}
	})

	t.Run(name+"/adjust_stock_infinite_is_noop", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		shop, _ := r.Create(ctx, Shop{MobTemplateID: 1})
		_ = r.UpsertStock(ctx, ShopStockRow{
			ShopID: shop.ID, ItemExternalID: "torch", Qty: -1, QtyMax: -1,
		})
		if err := r.AdjustStock(ctx, shop.ID, "torch", -1); err != nil {
			t.Fatalf("decrement infinite: %v", err)
		}
		got, _ := r.ListStock(ctx, shop.ID)
		if got[0].Qty != -1 {
			t.Fatalf("infinite stock disturbed: qty = %d", got[0].Qty)
		}
	})

	t.Run(name+"/adjust_stock_missing_returns_not_found", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		shop, _ := r.Create(ctx, Shop{MobTemplateID: 1})
		if err := r.AdjustStock(ctx, shop.ID, "no-such-item", -1); !errors.Is(err, ErrShopNotFound) {
			t.Fatalf("err = %v, want ErrShopNotFound", err)
		}
	})

	t.Run(name+"/stamp_restock_writes_ts", func(t *testing.T) {
		ctx := context.Background()
		r := newRepo(t)
		shop, _ := r.Create(ctx, Shop{MobTemplateID: 1})
		_ = r.UpsertStock(ctx, ShopStockRow{
			ShopID: shop.ID, ItemExternalID: "axe", Qty: 1, QtyMax: 5,
		})
		if err := r.StampRestock(ctx, shop.ID, "axe", 1234567); err != nil {
			t.Fatalf("stamp: %v", err)
		}
		got, _ := r.ListStock(ctx, shop.ID)
		if got[0].LastRestockTs != 1234567 {
			t.Fatalf("ts not stamped: %d", got[0].LastRestockTs)
		}
	})
}

func TestMemoryShopRepo(t *testing.T) {
	runShopRepoTests(t, "memory", func(t *testing.T) ShopRepo {
		return NewMemoryShopRepo()
	})
}

func TestSQLiteShopRepo(t *testing.T) {
	runShopRepoTests(t, "sqlite", func(t *testing.T) ShopRepo {
		conn, err := db.Open(context.Background(), ":memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		t.Cleanup(func() { conn.Close() })
		return NewSQLiteShopRepo(conn)
	})
}

func TestShop_AcceptsType(t *testing.T) {
	s := Shop{BuyTypes: []ItemType{ItemTypeFood, ItemTypeTradeGood}}
	if !s.AcceptsType(ItemTypeFood) {
		t.Fatal("food rejected")
	}
	if s.AcceptsType(ItemTypeWeapon) {
		t.Fatal("weapon accepted")
	}
	if (Shop{}).AcceptsType(ItemTypeFood) {
		t.Fatal("empty whitelist accepted")
	}
}

func TestShop_IsOpenAt(t *testing.T) {
	always := Shop{} // 0 == 0
	for h := 0; h < 24; h++ {
		if !always.IsOpenAt(h) {
			t.Fatalf("always-open closed at %d", h)
		}
	}
	day := Shop{OpenHour: 6, CloseHour: 22}
	cases := map[int]bool{0: false, 5: false, 6: true, 21: true, 22: false, 23: false}
	for h, want := range cases {
		if got := day.IsOpenAt(h); got != want {
			t.Fatalf("day-shop hour=%d got %v, want %v", h, got, want)
		}
	}
	tavern := Shop{OpenHour: 22, CloseHour: 4}
	wrap := map[int]bool{21: false, 22: true, 23: true, 0: true, 3: true, 4: false, 12: false}
	for h, want := range wrap {
		if got := tavern.IsOpenAt(h); got != want {
			t.Fatalf("tavern hour=%d got %v, want %v", h, got, want)
		}
	}
}
