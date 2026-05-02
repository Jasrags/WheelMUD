package repo

import (
	"context"
	"errors"
	"time"
)

// Sector classifies a room's terrain/medium for movement gating and
// rendering. The same enum lives in the rooms.sector CHECK constraint
// (see migration 0012); changing one without the other breaks inserts.
type Sector string

const (
	SectorCity        Sector = "city"
	SectorForest      Sector = "forest"
	SectorField       Sector = "field"
	SectorHills       Sector = "hills"
	SectorMountain    Sector = "mountain"
	SectorDesert      Sector = "desert"
	SectorWater       Sector = "water"      // surface — needs swim or boat
	SectorUnderwater  Sector = "underwater" // submerged — needs swim
	SectorAir         Sector = "air"        // needs fly
	SectorUnderground Sector = "underground"
)

// RoomFlags groups the boolean tags that gate gameplay behavior in a
// room. Bool-per-flag instead of a bitmask keeps SQL CHECK constraints
// trivial and table introspection readable; performance is not a
// concern at this scale.
type RoomFlags struct {
	Indoors    bool
	NoPVP      bool
	NoTeleport bool
	Dark       bool
	Silent     bool
	Peaceful   bool
}

// Room is the canonical "place" in the world. Description text is rendered
// verbatim by the look command — callers are responsible for any cfmt
// styling baked into the strings. ExternalID is the stable identifier
// referenced from YAML; the int ID is the surrogate primary key.
type Room struct {
	ID         int64
	ExternalID string
	// ZoneID points at the owning zones row. Zero means "unscoped" —
	// used by test fixtures that bypass the world loader. The loader
	// stamps a real value on every room it inserts; §16 admin
	// room-create will require it once that command lands.
	ZoneID     int64
	Name       string
	ShortDesc  string
	LongDesc   string
	Flags      RoomFlags
	Sector     Sector
	// LightLevel: 0 = pitch black; positive = lit. Combined with
	// Flags.Dark in look to decide whether descriptions are visible.
	// Default is 100 ("brightly lit") so existing zones keep rendering.
	LightLevel int
	CoordX     int
	CoordY     int
	CoordZ     int
	// ExtraDescs maps lowercased keyword -> long-form description.
	// `look <noun>` resolves against this map. Keys are normalized to
	// lowercase on write so lookups are case-insensitive without
	// rewriting the rendered text.
	ExtraDescs map[string]string
	CreatedAt  time.Time
}

// RoomRepo is the persistence boundary the look / move commands and the
// YAML world loader talk to.
type RoomRepo interface {
	// FindByID returns the room with the given int id. Returns
	// ErrRoomNotFound when missing.
	FindByID(ctx context.Context, id int64) (Room, error)
	// FindByExternalID resolves a room by its stable string id (e.g.
	// "plaza.fountain"). Returns ErrRoomNotFound when missing.
	FindByExternalID(ctx context.Context, externalID string) (Room, error)
	// Create inserts a new room. ExternalID must be non-empty; an empty
	// value returns ErrInvalidExternalID. A duplicate external_id returns
	// ErrDuplicateExternalID. If r.ID is non-zero the row is inserted
	// with that exact id (the loader uses this to pin the starter room
	// to id=1); otherwise SQLite assigns one.
	Create(ctx context.Context, r Room) (Room, error)
	// CountByZone returns the number of rooms whose rooms.zone_id
	// matches. Used by `zones show <id>` and the §10 ambient ticker.
	// Rooms inserted via in-memory test fixtures default to zone_id=0
	// and are counted under that bucket like any other.
	CountByZone(ctx context.Context, zoneID int64) (int, error)
}

// StarterRoomID is where new characters spawn. The YAML loader pins the
// room flagged `starter: true` to this id so the constant stays valid
// across loads.
const StarterRoomID int64 = 1

// DefaultLightLevel is the value applied to rooms loaded without an
// explicit value. 100 is "fully lit"; 0 is pitch black.
const DefaultLightLevel = 100

var ErrRoomNotFound = errors.New("repo: room not found")
