package repo

import (
	"context"
	"sort"
	"sync"
)

// MemoryShopRepo is a map-backed ShopRepo for tests and non-persistent
// runs. Concurrent-safe.
type MemoryShopRepo struct {
	mu        sync.RWMutex
	nextID    int64
	shops     map[int64]Shop // by id
	byMobTpl  map[int64]int64 // mob_template_id -> shop_id
	stockNext int64
	stock     map[int64][]ShopStockRow // shop_id -> rows (unsorted)
}

func NewMemoryShopRepo() *MemoryShopRepo {
	return &MemoryShopRepo{
		shops:    make(map[int64]Shop),
		byMobTpl: make(map[int64]int64),
		stock:    make(map[int64][]ShopStockRow),
	}
}

func (r *MemoryShopRepo) Create(_ context.Context, s Shop) (Shop, error) {
	if s.MobTemplateID == 0 {
		return Shop{}, ErrInvalidExternalID
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.byMobTpl[s.MobTemplateID]; dup {
		return Shop{}, ErrDuplicateExternalID
	}
	r.nextID++
	s.ID = r.nextID
	if s.BuyTypes == nil {
		s.BuyTypes = []ItemType{}
	}
	r.shops[s.ID] = s
	r.byMobTpl[s.MobTemplateID] = s.ID
	return s, nil
}

func (r *MemoryShopRepo) GetByMobTemplateID(_ context.Context, mobTemplateID int64) (Shop, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.byMobTpl[mobTemplateID]
	if !ok {
		return Shop{}, ErrShopNotFound
	}
	return r.shops[id], nil
}

func (r *MemoryShopRepo) ListShops(_ context.Context) ([]Shop, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Shop, 0, len(r.shops))
	for _, s := range r.shops {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *MemoryShopRepo) ListStock(_ context.Context, shopID int64) ([]ShopStockRow, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rows := r.stock[shopID]
	out := make([]ShopStockRow, len(rows))
	copy(out, rows)
	sort.Slice(out, func(i, j int) bool { return out[i].ItemExternalID < out[j].ItemExternalID })
	return out, nil
}

func (r *MemoryShopRepo) UpsertStock(_ context.Context, row ShopStockRow) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	rows := r.stock[row.ShopID]
	for i, existing := range rows {
		if existing.ItemExternalID == row.ItemExternalID {
			row.ID = existing.ID
			rows[i] = row
			r.stock[row.ShopID] = rows
			return nil
		}
	}
	r.stockNext++
	row.ID = r.stockNext
	r.stock[row.ShopID] = append(rows, row)
	return nil
}

func (r *MemoryShopRepo) AdjustStock(_ context.Context, shopID int64, itemExternalID string, delta int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	rows, ok := r.stock[shopID]
	if !ok {
		return ErrShopNotFound
	}
	for i, row := range rows {
		if row.ItemExternalID != itemExternalID {
			continue
		}
		// Infinite stock: ignore decrements; clamp increments to qty_max.
		if row.QtyMax < 0 {
			return nil
		}
		next := row.Qty + delta
		if next < 0 {
			return ErrOutOfStock
		}
		if row.QtyMax > 0 && next > row.QtyMax {
			next = row.QtyMax
		}
		rows[i].Qty = next
		r.stock[shopID] = rows
		return nil
	}
	return ErrShopNotFound
}

func (r *MemoryShopRepo) StampRestock(_ context.Context, shopID int64, itemExternalID string, ts int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	rows := r.stock[shopID]
	for i, row := range rows {
		if row.ItemExternalID == itemExternalID {
			rows[i].LastRestockTs = ts
			r.stock[shopID] = rows
			return nil
		}
	}
	return ErrShopNotFound
}
