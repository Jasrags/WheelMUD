package repo

import (
	"context"
	"errors"
)

// Banker is one banker-NPC's hours config, keyed 1:1 to a MobTemplate.
// V1 carries operating hours only — no fees, no min-deposit, no
// per-tier slots. Coin moves through CharacterRepo.RecordCoin against
// the existing characters.coin / bank_balance columns.
//
// Hours: OpenHour == CloseHour means always open (24h). Otherwise the
// banker is open when wall-hour ∈ [OpenHour, CloseHour) — wraps across
// midnight when CloseHour < OpenHour. Same semantics as Shop.IsOpenAt
// so a shopkeeper-and-banker NPC pair behave identically.
type Banker struct {
	ID            int64
	MobTemplateID int64
	OpenHour      int
	CloseHour     int
}

// IsOpenAt reports whether the banker is open at the given wall-hour
// (0..23). OpenHour == CloseHour is the always-open sentinel.
func (b Banker) IsOpenAt(hour int) bool {
	if b.OpenHour == b.CloseHour {
		return true
	}
	if b.OpenHour < b.CloseHour {
		return hour >= b.OpenHour && hour < b.CloseHour
	}
	return hour >= b.OpenHour || hour < b.CloseHour
}

// BankerRepo persists banker config. Bankers are re-created from YAML
// by the world loader on startup; there is no runtime mutation path
// (unlike shops, which carry stock that moves with buy/sell).
type BankerRepo interface {
	// Create inserts a new banker config. MobTemplateID must be
	// non-zero and unique. Returns the row with its assigned ID
	// populated.
	Create(ctx context.Context, b Banker) (Banker, error)
	// GetByMobTemplateID returns the banker attached to the given
	// template id, or ErrBankerNotFound.
	GetByMobTemplateID(ctx context.Context, mobTemplateID int64) (Banker, error)
	// ListBankers returns every banker, sorted by ID.
	ListBankers(ctx context.Context) ([]Banker, error)
}

// ErrBankerNotFound is returned when a Banker lookup misses. Callers
// translate this into "this isn't a banker" at the verb layer.
var ErrBankerNotFound = errors.New("repo: banker not found")
