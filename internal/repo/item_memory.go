package repo

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// MemoryItemRepo is an in-memory ItemRepo for tests. Concurrent-safe.
type MemoryItemRepo struct {
	mu    sync.Mutex
	items []Item
	byExt map[string]struct{}
	maxID int64
}

func NewMemoryItemRepo() *MemoryItemRepo {
	return &MemoryItemRepo{byExt: make(map[string]struct{})}
}

// Insert adds an item directly without ExternalID validation. Test
// fixtures use this; production code (the YAML loader) goes through
// Create.
func (r *MemoryItemRepo) Insert(i Item) Item {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.insertLocked(i)
}

func (r *MemoryItemRepo) Create(_ context.Context, i Item) (Item, error) {
	if i.ExternalID == "" {
		return Item{}, ErrInvalidExternalID
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
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.byExt[i.ExternalID]; dup {
		return Item{}, ErrDuplicateExternalID
	}
	return r.insertLocked(i), nil
}

func (r *MemoryItemRepo) insertLocked(i Item) Item {
	if i.ID == 0 {
		r.maxID++
		i.ID = r.maxID
	} else if i.ID > r.maxID {
		r.maxID = i.ID
	}
	if i.NameLower == "" {
		i.NameLower = strings.ToLower(i.Name)
	}
	if i.Type == "" {
		i.Type = ItemTypeTrash
	}
	if i.Quality == "" {
		i.Quality = QualityNormal
	}
	if i.CreatedAt.IsZero() {
		i.CreatedAt = time.Now().UTC()
	}
	r.items = append(r.items, i)
	if i.ExternalID != "" {
		r.byExt[i.ExternalID] = struct{}{}
	}
	return i
}

func (r *MemoryItemRepo) ListInRoom(_ context.Context, roomID int64) ([]Item, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Item
	for _, i := range r.items {
		if i.RoomID == roomID && i.OwnerCharacterID == 0 && i.ParentItemID == 0 {
			out = append(out, i)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NameLower < out[j].NameLower })
	return out, nil
}

func (r *MemoryItemRepo) ListInInventory(_ context.Context, ownerCharID int64) ([]Item, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Item
	for _, i := range r.items {
		if i.OwnerCharacterID == ownerCharID && ownerCharID != 0 && i.ParentItemID == 0 {
			out = append(out, i)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NameLower < out[j].NameLower })
	return out, nil
}

func (r *MemoryItemRepo) ListInContainer(_ context.Context, parentID int64) ([]Item, error) {
	if parentID == 0 {
		return nil, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Item
	for _, i := range r.items {
		if i.ParentItemID == parentID {
			out = append(out, i)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NameLower < out[j].NameLower })
	return out, nil
}

// ListAllOwnedTransitive walks the parent chain in-memory: collect
// top-level owned items, then iteratively pull anything whose parent
// is already in the result set. Bounded by len(items) iterations.
func (r *MemoryItemRepo) ListAllOwnedTransitive(_ context.Context, ownerCharID int64) ([]Item, error) {
	if ownerCharID == 0 {
		return nil, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	included := make(map[int64]bool)
	var out []Item
	for _, i := range r.items {
		if i.OwnerCharacterID == ownerCharID && i.ParentItemID == 0 {
			out = append(out, i)
			included[i.ID] = true
		}
	}
	// Fixed-point: keep adding children of already-included items.
	for grew := true; grew; {
		grew = false
		for _, i := range r.items {
			if included[i.ID] || i.ParentItemID == 0 {
				continue
			}
			if included[i.ParentItemID] {
				out = append(out, i)
				included[i.ID] = true
				grew = true
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ParentItemID != out[j].ParentItemID {
			return out[i].ParentItemID < out[j].ParentItemID
		}
		return out[i].NameLower < out[j].NameLower
	})
	return out, nil
}

func (r *MemoryItemRepo) FindByExternalID(_ context.Context, externalID string) (Item, error) {
	if externalID == "" {
		return Item{}, ErrItemNotFound
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, i := range r.items {
		if i.ExternalID == externalID {
			return i, nil
		}
	}
	return Item{}, ErrItemNotFound
}

func (r *MemoryItemRepo) ListExternalIDs(_ context.Context) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.items))
	for _, i := range r.items {
		if i.ExternalID != "" {
			out = append(out, i.ExternalID)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (r *MemoryItemRepo) Delete(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, it := range r.items {
		if it.ID != id {
			continue
		}
		delete(r.byExt, it.ExternalID)
		r.items = append(r.items[:i], r.items[i+1:]...)
		return nil
	}
	return ErrItemNotFound
}

func (r *MemoryItemRepo) UpdateStats(_ context.Context, id int64, stats ItemStats) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, it := range r.items {
		if it.ID != id {
			continue
		}
		if !statsTypeMatches(it.Type, stats) {
			return ErrItemStatsTypeMismatch
		}
		r.items[i].Stats = stats
		return nil
	}
	return ErrItemNotFound
}

func (r *MemoryItemRepo) GetByID(_ context.Context, id int64) (Item, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, i := range r.items {
		if i.ID == id {
			return i, nil
		}
	}
	return Item{}, ErrItemNotFound
}

func (r *MemoryItemRepo) SetOwner(_ context.Context, itemID, ownerCharID int64) error {
	if ownerCharID == 0 {
		return fmt.Errorf("set owner: ownerCharID must be non-zero")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for idx := range r.items {
		if r.items[idx].ID == itemID {
			r.items[idx].OwnerCharacterID = ownerCharID
			r.items[idx].RoomID = 0
			r.items[idx].ParentItemID = 0
			return nil
		}
	}
	return ErrItemNotFound
}

func (r *MemoryItemRepo) SetRoom(_ context.Context, itemID, roomID int64) error {
	if roomID == 0 {
		return fmt.Errorf("set room: roomID must be non-zero")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for idx := range r.items {
		if r.items[idx].ID == itemID {
			r.items[idx].RoomID = roomID
			r.items[idx].OwnerCharacterID = 0
			r.items[idx].ParentItemID = 0
			return nil
		}
	}
	return ErrItemNotFound
}

func (r *MemoryItemRepo) SetParent(_ context.Context, itemID, parentID int64) error {
	if itemID == 0 {
		return fmt.Errorf("set parent: itemID must be non-zero")
	}
	if parentID == 0 {
		return fmt.Errorf("set parent: parentID must be non-zero")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for idx := range r.items {
		if r.items[idx].ID == itemID {
			r.items[idx].ParentItemID = parentID
			r.items[idx].RoomID = 0
			r.items[idx].OwnerCharacterID = 0
			return nil
		}
	}
	return ErrItemNotFound
}

func (r *MemoryItemRepo) TransferRoomToOwner(_ context.Context, itemID, fromRoomID, toOwnerID int64) error {
	if fromRoomID == 0 || toOwnerID == 0 {
		return fmt.Errorf("transfer room->owner: ids must be non-zero")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for idx := range r.items {
		if r.items[idx].ID != itemID {
			continue
		}
		if r.items[idx].RoomID != fromRoomID || r.items[idx].OwnerCharacterID != 0 || r.items[idx].ParentItemID != 0 {
			return ErrItemMoved
		}
		r.items[idx].OwnerCharacterID = toOwnerID
		r.items[idx].RoomID = 0
		r.items[idx].ParentItemID = 0
		return nil
	}
	return ErrItemNotFound
}

func (r *MemoryItemRepo) TransferOwnerToRoom(_ context.Context, itemID, fromOwnerID, toRoomID int64) error {
	if fromOwnerID == 0 || toRoomID == 0 {
		return fmt.Errorf("transfer owner->room: ids must be non-zero")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for idx := range r.items {
		if r.items[idx].ID != itemID {
			continue
		}
		if r.items[idx].OwnerCharacterID != fromOwnerID || r.items[idx].RoomID != 0 || r.items[idx].ParentItemID != 0 {
			return ErrItemMoved
		}
		r.items[idx].RoomID = toRoomID
		r.items[idx].OwnerCharacterID = 0
		r.items[idx].ParentItemID = 0
		return nil
	}
	return ErrItemNotFound
}

func (r *MemoryItemRepo) TransferOwnerToOwner(_ context.Context, itemID, fromOwnerID, toOwnerID int64) error {
	if fromOwnerID == 0 || toOwnerID == 0 {
		return fmt.Errorf("transfer owner->owner: ids must be non-zero")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for idx := range r.items {
		if r.items[idx].ID != itemID {
			continue
		}
		if r.items[idx].OwnerCharacterID != fromOwnerID || r.items[idx].RoomID != 0 || r.items[idx].ParentItemID != 0 {
			return ErrItemMoved
		}
		r.items[idx].OwnerCharacterID = toOwnerID
		r.items[idx].ParentItemID = 0
		return nil
	}
	return ErrItemNotFound
}

func (r *MemoryItemRepo) TransferOwnerToContainer(_ context.Context, itemID, fromOwnerID, parentID int64) error {
	if fromOwnerID == 0 || parentID == 0 {
		return fmt.Errorf("transfer owner->container: ids must be non-zero")
	}
	if itemID == parentID {
		return fmt.Errorf("transfer owner->container: item cannot be its own parent")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for idx := range r.items {
		if r.items[idx].ID != itemID {
			continue
		}
		if r.items[idx].OwnerCharacterID != fromOwnerID || r.items[idx].RoomID != 0 || r.items[idx].ParentItemID != 0 {
			return ErrItemMoved
		}
		r.items[idx].ParentItemID = parentID
		r.items[idx].OwnerCharacterID = 0
		r.items[idx].RoomID = 0
		return nil
	}
	return ErrItemNotFound
}

func (r *MemoryItemRepo) TransferContainerToOwner(_ context.Context, itemID, fromParentID, toOwnerID int64) error {
	if fromParentID == 0 || toOwnerID == 0 {
		return fmt.Errorf("transfer container->owner: ids must be non-zero")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for idx := range r.items {
		if r.items[idx].ID != itemID {
			continue
		}
		if r.items[idx].ParentItemID != fromParentID || r.items[idx].RoomID != 0 || r.items[idx].OwnerCharacterID != 0 {
			return ErrItemMoved
		}
		r.items[idx].OwnerCharacterID = toOwnerID
		r.items[idx].ParentItemID = 0
		r.items[idx].RoomID = 0
		return nil
	}
	return ErrItemNotFound
}
