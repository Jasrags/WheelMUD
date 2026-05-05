package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Jasrags/WheelMUD/internal/currency"
)

type SQLiteItemRepo struct {
	db *sql.DB
}

func NewSQLiteItemRepo(db *sql.DB) *SQLiteItemRepo {
	return &SQLiteItemRepo{db: db}
}

const itemSelectCols = `id, external_id, name, name_lower, short_desc, room_id, owner_character_id, parent_item_id, ` +
	`type, weight_lbs, value_cp, quality, flags, stats_json, created_at`

func (r *SQLiteItemRepo) Create(ctx context.Context, i Item) (Item, error) {
	if i.ExternalID == "" {
		return Item{}, ErrInvalidExternalID
	}
	if i.NameLower == "" {
		i.NameLower = strings.ToLower(i.Name)
	}
	if i.Type == "" {
		i.Type = ItemTypeTrash
	}
	if !i.Type.IsValid() {
		return Item{}, fmt.Errorf("invalid item type %q", i.Type)
	}
	if i.Quality == "" {
		i.Quality = QualityNormal
	}
	if !i.Quality.IsValid() {
		return Item{}, fmt.Errorf("invalid item quality %q", i.Quality)
	}
	if !statsTypeMatches(i.Type, i.Stats) {
		return Item{}, ErrItemStatsTypeMismatch
	}
	statsJSON, err := encodeStats(i.Stats)
	if err != nil {
		return Item{}, err
	}
	if i.CreatedAt.IsZero() {
		i.CreatedAt = time.Now().UTC()
	}
	var roomID any
	if i.RoomID != 0 {
		roomID = i.RoomID
	}
	var ownerID any
	if i.OwnerCharacterID != 0 {
		ownerID = i.OwnerCharacterID
	}
	var parentID any
	if i.ParentItemID != 0 {
		parentID = i.ParentItemID
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO items(external_id, name, name_lower, short_desc, room_id, owner_character_id, parent_item_id,
			type, weight_lbs, value_cp, quality, flags, stats_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		i.ExternalID, i.Name, i.NameLower, i.ShortDesc, roomID, ownerID, parentID,
		string(i.Type), i.Weight, int64(i.Value), string(i.Quality),
		int64(i.Flags), statsJSON, i.CreatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return Item{}, ErrDuplicateExternalID
		}
		return Item{}, fmt.Errorf("insert item: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Item{}, fmt.Errorf("last insert id: %w", err)
	}
	i.ID = id
	return i, nil
}

func (r *SQLiteItemRepo) ListInRoom(ctx context.Context, roomID int64) ([]Item, error) {
	// `AND owner_character_id IS NULL` keeps the listing honest even
	// if the location invariant is ever violated by a buggy code path
	// (an item with both columns set should never appear in the room
	// view alongside its inventory view). Mirrors the memory repo.
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+itemSelectCols+` FROM items WHERE room_id = ? AND owner_character_id IS NULL AND parent_item_id IS NULL ORDER BY name_lower`,
		roomID,
	)
	if err != nil {
		return nil, fmt.Errorf("list items: %w", err)
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		i, err := scanItemRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (r *SQLiteItemRepo) ListInInventory(ctx context.Context, ownerCharID int64) ([]Item, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+itemSelectCols+` FROM items WHERE owner_character_id = ? AND parent_item_id IS NULL ORDER BY name_lower`,
		ownerCharID,
	)
	if err != nil {
		return nil, fmt.Errorf("list inventory: %w", err)
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		i, err := scanItemRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (r *SQLiteItemRepo) GetByID(ctx context.Context, id int64) (Item, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+itemSelectCols+` FROM items WHERE id = ?`, id,
	)
	i, err := scanItemRow(row)
	if err != nil {
		// scanItemRow wraps Scan errors with %w, so unwrap.
		if errors.Is(err, sql.ErrNoRows) {
			return Item{}, ErrItemNotFound
		}
		return Item{}, err
	}
	return i, nil
}

// SetOwner moves an item into a character's inventory. Both location
// columns are updated in one statement so the location invariant
// (exactly one non-NULL) holds across the change.
func (r *SQLiteItemRepo) SetOwner(ctx context.Context, itemID, ownerCharID int64) error {
	if ownerCharID == 0 {
		return fmt.Errorf("set owner: ownerCharID must be non-zero")
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE items SET owner_character_id = ?, room_id = NULL, parent_item_id = NULL WHERE id = ?`,
		ownerCharID, itemID,
	)
	if err != nil {
		return fmt.Errorf("set owner: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("set owner rows: %w", err)
	}
	if n == 0 {
		return ErrItemNotFound
	}
	return nil
}

// SetRoom places an item on a room's floor and clears any owner.
func (r *SQLiteItemRepo) SetRoom(ctx context.Context, itemID, roomID int64) error {
	if roomID == 0 {
		return fmt.Errorf("set room: roomID must be non-zero")
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE items SET room_id = ?, owner_character_id = NULL, parent_item_id = NULL WHERE id = ?`,
		roomID, itemID,
	)
	if err != nil {
		return fmt.Errorf("set room: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("set room rows: %w", err)
	}
	if n == 0 {
		return ErrItemNotFound
	}
	return nil
}

// transferRowsResult inspects an UPDATE's rows-affected count and a
// follow-up existence probe to disambiguate ErrItemMoved (item exists
// but at a different location) from ErrItemNotFound (no such item).
func (r *SQLiteItemRepo) transferRowsResult(ctx context.Context, res sqlResult, itemID int64, op string) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s rows: %w", op, err)
	}
	if n != 0 {
		return nil
	}
	// Probe existence to distinguish "moved" from "missing".
	var probe int64
	row := r.db.QueryRowContext(ctx, `SELECT id FROM items WHERE id = ?`, itemID)
	if err := row.Scan(&probe); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrItemNotFound
		}
		return fmt.Errorf("%s probe: %w", op, err)
	}
	return ErrItemMoved
}

// sqlResult is the subset of sql.Result we use; named so transferRowsResult
// is testable without leaking the database/sql concrete type into helpers.
type sqlResult interface {
	RowsAffected() (int64, error)
}

// TransferRoomToOwner is the conditional pickup path. The WHERE guard
// prevents two players from racing to grab the same floor item: only
// one UPDATE will affect a row.
func (r *SQLiteItemRepo) TransferRoomToOwner(ctx context.Context, itemID, fromRoomID, toOwnerID int64) error {
	if fromRoomID == 0 || toOwnerID == 0 {
		return fmt.Errorf("transfer room->owner: ids must be non-zero")
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE items SET owner_character_id = ?, room_id = NULL, parent_item_id = NULL
		 WHERE id = ? AND room_id = ? AND owner_character_id IS NULL AND parent_item_id IS NULL`,
		toOwnerID, itemID, fromRoomID,
	)
	if err != nil {
		return fmt.Errorf("transfer room->owner: %w", err)
	}
	return r.transferRowsResult(ctx, res, itemID, "transfer room->owner")
}

// TransferOwnerToRoom is the conditional drop path.
func (r *SQLiteItemRepo) TransferOwnerToRoom(ctx context.Context, itemID, fromOwnerID, toRoomID int64) error {
	if fromOwnerID == 0 || toRoomID == 0 {
		return fmt.Errorf("transfer owner->room: ids must be non-zero")
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE items SET room_id = ?, owner_character_id = NULL, parent_item_id = NULL
		 WHERE id = ? AND owner_character_id = ? AND room_id IS NULL AND parent_item_id IS NULL`,
		toRoomID, itemID, fromOwnerID,
	)
	if err != nil {
		return fmt.Errorf("transfer owner->room: %w", err)
	}
	return r.transferRowsResult(ctx, res, itemID, "transfer owner->room")
}

// TransferOwnerToOwner is the conditional give path.
func (r *SQLiteItemRepo) TransferOwnerToOwner(ctx context.Context, itemID, fromOwnerID, toOwnerID int64) error {
	if fromOwnerID == 0 || toOwnerID == 0 {
		return fmt.Errorf("transfer owner->owner: ids must be non-zero")
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE items SET owner_character_id = ?, parent_item_id = NULL
		 WHERE id = ? AND owner_character_id = ? AND room_id IS NULL AND parent_item_id IS NULL`,
		toOwnerID, itemID, fromOwnerID,
	)
	if err != nil {
		return fmt.Errorf("transfer owner->owner: %w", err)
	}
	return r.transferRowsResult(ctx, res, itemID, "transfer owner->owner")
}

// ListInContainer returns items nested directly inside the given
// container item. Sorted by name. The parent_item_id index keeps
// this O(log n) on lookup.
func (r *SQLiteItemRepo) ListInContainer(ctx context.Context, parentID int64) ([]Item, error) {
	if parentID == 0 {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+itemSelectCols+` FROM items WHERE parent_item_id = ? ORDER BY name_lower`,
		parentID,
	)
	if err != nil {
		return nil, fmt.Errorf("list container: %w", err)
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		i, err := scanItemRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// ListAllOwnedTransitive returns the carrier's top-level inventory
// plus everything nested inside any container they own (any depth).
// Implemented as a recursive CTE so a deep stack of bag-in-pack-in-
// chest is one query, not N. Sort is stable by parent_item_id then
// name so callers can render the tree by walking the slice.
func (r *SQLiteItemRepo) ListAllOwnedTransitive(ctx context.Context, ownerCharID int64) ([]Item, error) {
	if ownerCharID == 0 {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx,
		`WITH RECURSIVE owned(id) AS (
			SELECT id FROM items WHERE owner_character_id = ? AND parent_item_id IS NULL
			UNION ALL
			SELECT i.id FROM items i JOIN owned o ON i.parent_item_id = o.id
		)
		SELECT `+itemSelectCols+` FROM items WHERE id IN owned ORDER BY parent_item_id, name_lower`,
		ownerCharID,
	)
	if err != nil {
		return nil, fmt.Errorf("list owned transitive: %w", err)
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		i, err := scanItemRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// TransferOwnerToContainer is the conditional `put` path. Only an
// item currently held by fromOwnerID with no other location can move
// — concurrent gives, drops, and rival puts surface as ErrItemMoved.
func (r *SQLiteItemRepo) TransferOwnerToContainer(ctx context.Context, itemID, fromOwnerID, parentID int64) error {
	if fromOwnerID == 0 || parentID == 0 {
		return fmt.Errorf("transfer owner->container: ids must be non-zero")
	}
	if itemID == parentID {
		return fmt.Errorf("transfer owner->container: item cannot be its own parent")
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE items SET parent_item_id = ?, owner_character_id = NULL, room_id = NULL
		 WHERE id = ? AND owner_character_id = ? AND room_id IS NULL AND parent_item_id IS NULL`,
		parentID, itemID, fromOwnerID,
	)
	if err != nil {
		return fmt.Errorf("transfer owner->container: %w", err)
	}
	return r.transferRowsResult(ctx, res, itemID, "transfer owner->container")
}

// TransferContainerToOwner is the conditional `get from` path.
func (r *SQLiteItemRepo) TransferContainerToOwner(ctx context.Context, itemID, fromParentID, toOwnerID int64) error {
	if fromParentID == 0 || toOwnerID == 0 {
		return fmt.Errorf("transfer container->owner: ids must be non-zero")
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE items SET owner_character_id = ?, parent_item_id = NULL, room_id = NULL
		 WHERE id = ? AND parent_item_id = ? AND room_id IS NULL AND owner_character_id IS NULL`,
		toOwnerID, itemID, fromParentID,
	)
	if err != nil {
		return fmt.Errorf("transfer container->owner: %w", err)
	}
	return r.transferRowsResult(ctx, res, itemID, "transfer container->owner")
}

// FindByExternalID looks an item up by its external_id. The column
// has a UNIQUE index so at most one row matches; returns
// ErrItemNotFound when nothing is found.
func (r *SQLiteItemRepo) FindByExternalID(ctx context.Context, externalID string) (Item, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+itemSelectCols+` FROM items WHERE external_id = ?`, externalID,
	)
	i, err := scanItemRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Item{}, ErrItemNotFound
		}
		return Item{}, err
	}
	return i, nil
}

// ListExternalIDs returns every item's external_id, sorted.
func (r *SQLiteItemRepo) ListExternalIDs(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT external_id FROM items ORDER BY external_id`)
	if err != nil {
		return nil, fmt.Errorf("list item external ids: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, fmt.Errorf("scan item external id: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// scanItemRow reads one row from a SELECT itemSelectCols result and
// builds a fully decoded Item, including the polymorphic stats blob.
// Centralized so the column list and decode contract live in one
// place — callers just hand it a *sql.Rows / *sql.Row.
func scanItemRow(s rowScanner) (Item, error) {
	var (
		i         Item
		rid       sql.NullInt64
		oid       sql.NullInt64
		pid       sql.NullInt64
		typeStr   string
		quality   string
		valueCP   int64
		flags     int64
		statsJSON string
	)
	if err := s.Scan(
		&i.ID, &i.ExternalID, &i.Name, &i.NameLower, &i.ShortDesc, &rid, &oid, &pid,
		&typeStr, &i.Weight, &valueCP, &quality, &flags, &statsJSON, &i.CreatedAt,
	); err != nil {
		return Item{}, fmt.Errorf("scan item row: %w", err)
	}
	if rid.Valid {
		i.RoomID = rid.Int64
	}
	if oid.Valid {
		i.OwnerCharacterID = oid.Int64
	}
	if pid.Valid {
		i.ParentItemID = pid.Int64
	}
	i.Type = ItemType(typeStr)
	i.Quality = ItemQuality(quality)
	i.Value = currency.Amount(valueCP)
	i.Flags = ItemFlags(flags)
	stats, err := decodeStats(i.Type, statsJSON)
	if err != nil {
		return Item{}, err
	}
	i.Stats = stats
	return i, nil
}
