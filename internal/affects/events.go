package affects

// Expired fires when one or more timed buffs/debuffs on a character
// counted down to zero. Source is either combat's end-of-round tick
// (in-fight participants) or the out-of-combat session ticker.
//
// CharacterID is the affected character; RoomID is their location at
// expiry time so a cmd-layer subscriber broadcasting via WriteAsync
// has the room context without re-reading session state. Names is a
// snapshot of the expired Affect.Name values in the order they were
// dropped.
//
// The event lives in this package (not internal/combat) so the
// out-of-combat session ticker can publish without importing combat
// (combat already imports affects, which would be circular).
//
// Phase E #26.
// ExpiredEntry is one expired affect surfaced to the cmd-layer
// subscriber. Message is empty for admin-applied affects (slice 1) —
// the subscriber falls back to the generic "Your <name> fades." line
// in that case. Catalog-driven producers (slice 2 onward) carry the
// authored MessageOnExpire string here.
//
// Phase E #25 slice 3.
type ExpiredEntry struct {
	Name    string
	Message string
}

type Expired struct {
	CharacterID int64
	RoomID      int64
	Entries     []ExpiredEntry
}

// TickDamaged fires when one or more TickEffect-bearing affects on a
// character delivered a non-zero HP delta on the current pulse.
// Subscribers in cmd-layer render per-event lines via WriteAsync
// (cross-session output rule applies — the affects ticker runs on
// the eventbus goroutine).
//
// Phase E #25 slice 2.
type TickDamaged struct {
	CharacterID int64
	RoomID      int64
	Events      []TickEvent
	NewHP       int32
	HPMax       int32
}
