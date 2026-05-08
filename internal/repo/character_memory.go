package repo

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Jasrags/WheelMUD/internal/creature"
	"github.com/Jasrags/WheelMUD/internal/currency"
)

// MemoryCharacterRepo is an in-memory CharacterRepo for tests.
// Concurrent-safe.
type MemoryCharacterRepo struct {
	mu      sync.Mutex
	nextID  int64
	byLower map[string]*Character // name_lower -> character (single source of truth)
}

func NewMemoryCharacterRepo() *MemoryCharacterRepo {
	return &MemoryCharacterRepo{byLower: make(map[string]*Character)}
}

func (r *MemoryCharacterRepo) Create(_ context.Context, c Character) (Character, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c.NameLower = strings.ToLower(c.Name)
	if _, exists := r.byLower[c.NameLower]; exists {
		return Character{}, ErrDuplicateCharacterName
	}
	// First-character bootstrap mirrors sqlite Create. Holding mu
	// makes the count + insert atomic for free.
	if len(r.byLower) == 0 {
		c.AuthLevel = AuthLevelAdmin
	}
	r.nextID++
	c.ID = r.nextID
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	if c.CurrentRoomID == 0 {
		c.CurrentRoomID = StarterRoomID
	}
	if c.BoundRoomID == 0 {
		c.BoundRoomID = StarterRoomID
	}
	stored := c
	r.byLower[c.NameLower] = &stored
	return stored, nil
}

func (r *MemoryCharacterRepo) FindByName(_ context.Context, name string) (Character, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.byLower[strings.ToLower(name)]
	if !ok {
		return Character{}, ErrCharacterNotFound
	}
	return *c, nil
}

func (r *MemoryCharacterRepo) GetByID(_ context.Context, id int64) (Character, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.byLower {
		if c.ID == id {
			return *c, nil
		}
	}
	return Character{}, ErrCharacterNotFound
}

func (r *MemoryCharacterRepo) ListByAccount(_ context.Context, accountID int64) ([]Character, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Character
	for _, c := range r.byLower {
		if c.AccountID == accountID {
			out = append(out, *c)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		// last_played_at DESC nulls last, then name_lower asc
		ai, aj := out[i].LastPlayedAt, out[j].LastPlayedAt
		switch {
		case ai != nil && aj == nil:
			return true
		case ai == nil && aj != nil:
			return false
		case ai != nil && aj != nil && !ai.Equal(*aj):
			return ai.After(*aj)
		}
		return out[i].NameLower < out[j].NameLower
	})
	return out, nil
}

func (r *MemoryCharacterRepo) RecordPlay(_ context.Context, id int64, when time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.byLower {
		if c.ID == id {
			t := when
			c.LastPlayedAt = &t
			return nil
		}
	}
	return ErrCharacterNotFound
}

func (r *MemoryCharacterRepo) RecordRoom(_ context.Context, id, roomID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.byLower {
		if c.ID == id {
			c.CurrentRoomID = roomID
			return nil
		}
	}
	return ErrCharacterNotFound
}

func (r *MemoryCharacterRepo) RecordChannelSettings(_ context.Context, id int64, settings map[string]bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.byLower {
		if c.ID == id {
			if len(settings) == 0 {
				c.ChannelSettings = nil
				return nil
			}
			cp := make(map[string]bool, len(settings))
			for k, v := range settings {
				cp[k] = v
			}
			c.ChannelSettings = cp
			return nil
		}
	}
	return ErrCharacterNotFound
}

func (r *MemoryCharacterRepo) RecordCore(_ context.Context, id int64, hp, subdual int32, cond creature.Condition, pos creature.PositionFlags) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.byLower {
		if c.ID == id {
			c.Core.HPCurrent = hp
			c.Core.Subdual = subdual
			c.Core.Conditions = cond
			c.Core.Position = pos
			return nil
		}
	}
	return ErrCharacterNotFound
}

func (r *MemoryCharacterRepo) RecordInventory(_ context.Context, id int64, ids []int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.byLower {
		if c.ID == id {
			if len(ids) == 0 {
				c.Inventory = nil
				return nil
			}
			cp := make([]int64, len(ids))
			copy(cp, ids)
			c.Inventory = cp
			return nil
		}
	}
	return ErrCharacterNotFound
}

func (r *MemoryCharacterRepo) RecordEquipment(_ context.Context, id int64, eq creature.Equipment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.byLower {
		if c.ID == id {
			// Deep-copy slice fields so the caller mutating eq after
			// the call doesn't bleed back into stored state.
			cp := eq
			if len(eq.BeltPouches) > 0 {
				cp.BeltPouches = append([]int64(nil), eq.BeltPouches...)
			} else {
				cp.BeltPouches = nil
			}
			if len(eq.WornMisc) > 0 {
				cp.WornMisc = append([]int64(nil), eq.WornMisc...)
			} else {
				cp.WornMisc = nil
			}
			c.Equipment = cp
			return nil
		}
	}
	return ErrCharacterNotFound
}

func (r *MemoryCharacterRepo) RecordCoin(_ context.Context, id int64, coin, bank currency.Amount, expectedVersion int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.byLower {
		if c.ID == id {
			if c.CoinVersion != expectedVersion {
				return ErrCoinConflict
			}
			c.Coin = coin
			c.BankBalance = bank
			c.CoinVersion++
			return nil
		}
	}
	return ErrCharacterNotFound
}

func (r *MemoryCharacterRepo) RecordXP(_ context.Context, id int64, xp int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.byLower {
		if c.ID == id {
			c.XP = xp
			return nil
		}
	}
	return ErrCharacterNotFound
}

func (r *MemoryCharacterRepo) RecordXPDebt(_ context.Context, id int64, debt int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.byLower {
		if c.ID == id {
			c.XPDebt = debt
			return nil
		}
	}
	return ErrCharacterNotFound
}

func (r *MemoryCharacterRepo) RecordPromptTemplate(_ context.Context, id int64, tmpl string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.byLower {
		if c.ID == id {
			c.PromptTemplate = tmpl
			return nil
		}
	}
	return ErrCharacterNotFound
}

func (r *MemoryCharacterRepo) RecordLevelUp(_ context.Context, id int64, f LevelUpFields) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.byLower {
		if c.ID == id {
			// Defensive copy of the ClassLevels map so the caller
			// mutating it after the call doesn't bleed back.
			cp := make(map[creature.Class]int8, len(f.ClassLevels))
			for k, v := range f.ClassLevels {
				cp[k] = v
			}
			c.ClassLevels = cp
			c.Core.HPCurrent = f.HPCurrent
			c.Core.HPMax = f.HPMax
			c.Core.BAB = f.BAB
			c.Core.Saves = f.Saves
			c.PendingFeats += f.PendingFeatsDelta
			c.PendingSkillPoints += f.PendingSkillPointsDelta
			c.PendingAbilityBumps += f.PendingAbilityBumpsDelta
			c.PendingWeaves += f.PendingWeavesDelta
			return nil
		}
	}
	return ErrCharacterNotFound
}

func (r *MemoryCharacterRepo) RecordSkillRank(_ context.Context, id int64,
	skillID int32, newRanks int8, isClassSkill bool, newPending int32) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.byLower {
		if c.ID == id {
			if c.Skills == nil {
				c.Skills = make(map[int32]creature.SkillRanks, 1)
			}
			c.Skills[skillID] = creature.SkillRanks{
				Ranks:        newRanks,
				IsClassSkill: isClassSkill,
			}
			c.PendingSkillPoints = newPending
			return nil
		}
	}
	return ErrCharacterNotFound
}

func (r *MemoryCharacterRepo) RecordFeatPick(_ context.Context, id int64,
	featID int32, newPending int32) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.byLower {
		if c.ID == id {
			c.Feats = append(c.Feats, featID)
			c.PendingFeats = newPending
			return nil
		}
	}
	return ErrCharacterNotFound
}

func (r *MemoryCharacterRepo) RecordAbilityBump(_ context.Context, id int64,
	ability AbilityKey, newScore int8, newPending int32) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.byLower {
		if c.ID == id {
			switch ability {
			case AbilityStr:
				c.Core.Abilities.Str.Current = newScore
			case AbilityDex:
				c.Core.Abilities.Dex.Current = newScore
			case AbilityCon:
				c.Core.Abilities.Con.Current = newScore
			case AbilityInt:
				c.Core.Abilities.Int.Current = newScore
			case AbilityWis:
				c.Core.Abilities.Wis.Current = newScore
			case AbilityCha:
				c.Core.Abilities.Cha.Current = newScore
			default:
				return ErrCharacterNotFound // shouldn't happen — verb refuses earlier
			}
			c.PendingAbilityBumps = newPending
			return nil
		}
	}
	return ErrCharacterNotFound
}

func (r *MemoryCharacterRepo) RecordWeavePick(_ context.Context, id int64,
	weaveID string, newPending int32) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.byLower {
		if c.ID == id {
			if c.Channeling == nil {
				return ErrNotChanneler
			}
			cp := *c.Channeling
			cp.WeavesKnownIDs = append([]string(nil), c.Channeling.WeavesKnownIDs...)
			cp.WeavesKnownIDs = append(cp.WeavesKnownIDs, weaveID)
			c.Channeling = &cp
			c.PendingWeaves = newPending
			return nil
		}
	}
	return ErrCharacterNotFound
}

func (r *MemoryCharacterRepo) RecordPvP(_ context.Context, id int64, on bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.byLower {
		if c.ID == id {
			c.PvP = on
			return nil
		}
	}
	return ErrCharacterNotFound
}

func (r *MemoryCharacterRepo) Delete(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, c := range r.byLower {
		if c.ID == id {
			delete(r.byLower, k)
			return nil
		}
	}
	return ErrCharacterNotFound
}

func (r *MemoryCharacterRepo) MarkNewsSeen(_ context.Context, id int64, when time.Time) error {
	if when.IsZero() {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.byLower {
		if c.ID == id {
			if when.After(c.LastNewsSeen) {
				c.LastNewsSeen = when
			}
			return nil
		}
	}
	return ErrCharacterNotFound
}
