package repo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// SQLiteShopRepo persists shops + shop_stock rows. Created in
// migration 0030. BuyTypes is JSON-encoded into buy_types_json.
//
// The constructor takes DBTX so the world loader can pass an
// in-flight *sql.Tx and the runtime can pass a pooled *sql.DB.
type SQLiteShopRepo struct {
	db DBTX
}

func NewSQLiteShopRepo(db DBTX) *SQLiteShopRepo {
	return &SQLiteShopRepo{db: db}
}

const shopSelectCols = `id, mob_template_id, buy_types_json, sell_markup, buy_markdown, open_hour, close_hour, restock_interval_s`

func scanShop(scanner interface {
	Scan(dest ...interface{}) error
}) (Shop, error) {
	var (
		s            Shop
		buyTypesJSON string
	)
	if err := scanner.Scan(&s.ID, &s.MobTemplateID, &buyTypesJSON,
		&s.SellMarkup, &s.BuyMarkdown, &s.OpenHour, &s.CloseHour,
		&s.RestockIntervalS); err != nil {
		return Shop{}, err
	}
	if strings.TrimSpace(buyTypesJSON) == "" {
		s.BuyTypes = []ItemType{}
	} else {
		var raw []string
		if err := json.Unmarshal([]byte(buyTypesJSON), &raw); err != nil {
			return Shop{}, fmt.Errorf("decode buy_types_json: %w", err)
		}
		s.BuyTypes = make([]ItemType, 0, len(raw))
		for _, v := range raw {
			s.BuyTypes = append(s.BuyTypes, ItemType(v))
		}
	}
	return s, nil
}

func (r *SQLiteShopRepo) Create(ctx context.Context, s Shop) (Shop, error) {
	if s.MobTemplateID == 0 {
		return Shop{}, ErrInvalidExternalID
	}
	if s.BuyTypes == nil {
		s.BuyTypes = []ItemType{}
	}
	raw := make([]string, 0, len(s.BuyTypes))
	for _, t := range s.BuyTypes {
		raw = append(raw, string(t))
	}
	bt, err := json.Marshal(raw)
	if err != nil {
		return Shop{}, fmt.Errorf("encode buy_types: %w", err)
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO shops(mob_template_id, buy_types_json, sell_markup, buy_markdown,
		                    open_hour, close_hour, restock_interval_s)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		s.MobTemplateID, string(bt), s.SellMarkup, s.BuyMarkdown,
		s.OpenHour, s.CloseHour, s.RestockIntervalS)
	if err != nil {
		if isUniqueViolation(err) {
			return Shop{}, ErrDuplicateExternalID
		}
		return Shop{}, fmt.Errorf("insert shop: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Shop{}, fmt.Errorf("lastInsertId shop: %w", err)
	}
	s.ID = id
	return s, nil
}

func (r *SQLiteShopRepo) GetByMobTemplateID(ctx context.Context, mobTemplateID int64) (Shop, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+shopSelectCols+` FROM shops WHERE mob_template_id = ?`, mobTemplateID)
	s, err := scanShop(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Shop{}, ErrShopNotFound
	}
	if err != nil {
		return Shop{}, fmt.Errorf("get shop: %w", err)
	}
	return s, nil
}

func (r *SQLiteShopRepo) ListShops(ctx context.Context) ([]Shop, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+shopSelectCols+` FROM shops ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list shops: %w", err)
	}
	defer rows.Close()
	var out []Shop
	for rows.Next() {
		s, err := scanShop(rows)
		if err != nil {
			return nil, fmt.Errorf("scan shop: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate shops: %w", err)
	}
	return out, nil
}

func (r *SQLiteShopRepo) ListStock(ctx context.Context, shopID int64) ([]ShopStockRow, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, shop_id, item_external_id, qty, qty_max, last_restock_ts
		 FROM shop_stock WHERE shop_id = ? ORDER BY item_external_id`,
		shopID)
	if err != nil {
		return nil, fmt.Errorf("list shop_stock: %w", err)
	}
	defer rows.Close()
	var out []ShopStockRow
	for rows.Next() {
		var row ShopStockRow
		if err := rows.Scan(&row.ID, &row.ShopID, &row.ItemExternalID,
			&row.Qty, &row.QtyMax, &row.LastRestockTs); err != nil {
			return nil, fmt.Errorf("scan shop_stock: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate shop_stock: %w", err)
	}
	return out, nil
}

func (r *SQLiteShopRepo) UpsertStock(ctx context.Context, row ShopStockRow) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO shop_stock(shop_id, item_external_id, qty, qty_max, last_restock_ts)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(shop_id, item_external_id) DO UPDATE SET
		   qty             = excluded.qty,
		   qty_max         = excluded.qty_max,
		   last_restock_ts = excluded.last_restock_ts`,
		row.ShopID, row.ItemExternalID, row.Qty, row.QtyMax, row.LastRestockTs)
	if err != nil {
		return fmt.Errorf("upsert shop_stock: %w", err)
	}
	return nil
}

func (r *SQLiteShopRepo) AdjustStock(ctx context.Context, shopID int64, itemExternalID string, delta int) error {
	// SELECT-then-UPDATE without an explicit transaction. SQLite
	// serializes writes at the database level, and the verb path
	// for buy/sell is single-call-per-action, so the racy window is
	// no wider than the item Transfer* family already accepts (see
	// CLAUDE.md:204). If the row vanishes between the two
	// statements, the UPDATE simply affects zero rows and we surface
	// ErrShopNotFound.
	var qty, qtyMax int
	err := r.db.QueryRowContext(ctx,
		`SELECT qty, qty_max FROM shop_stock WHERE shop_id = ? AND item_external_id = ?`,
		shopID, itemExternalID).Scan(&qty, &qtyMax)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrShopNotFound
	}
	if err != nil {
		return fmt.Errorf("read shop_stock: %w", err)
	}
	// Infinite-stock sentinel: leave qty alone.
	if qtyMax < 0 {
		return nil
	}
	next := qty + delta
	if next < 0 {
		return ErrOutOfStock
	}
	if qtyMax > 0 && next > qtyMax {
		next = qtyMax
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE shop_stock SET qty = ? WHERE shop_id = ? AND item_external_id = ?`,
		next, shopID, itemExternalID)
	if err != nil {
		return fmt.Errorf("update shop_stock qty: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows_affected adjust_stock: %w", err)
	}
	if n == 0 {
		return ErrShopNotFound
	}
	return nil
}

func (r *SQLiteShopRepo) StampRestock(ctx context.Context, shopID int64, itemExternalID string, ts int64) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE shop_stock SET last_restock_ts = ? WHERE shop_id = ? AND item_external_id = ?`,
		ts, shopID, itemExternalID)
	if err != nil {
		return fmt.Errorf("stamp restock: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows_affected stamp restock: %w", err)
	}
	if n == 0 {
		return ErrShopNotFound
	}
	return nil
}
