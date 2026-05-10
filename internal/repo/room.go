package repo

import (
	"context"
	"errors"
	"time"
)

// Sector classifies a room's terrain/medium for movement gating and
// rendering. The same enum lives in the rooms.sector CHECK constraint
// (originally migration 0012, widened by 0025); changing one without
// the other breaks inserts. The validate-loader keeps its own copy in
// internal/world/validate.go::validSectors and must move in lock-step.
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
	// Wheel-of-Time terrain extensions (migration 0025). See
	// docs/wot_geography_mud.md for builder guidance.
	SectorBlight   Sector = "blight"   // corrupted lands; ambient horror, future DoT hook
	SectorWaste    Sector = "waste"    // Aiel Waste; arid rocky steppe distinct from desert
	SectorStedding Sector = "stedding" // Ogier sanctuary; channeling suppressed (mechanic TBD)
	SectorSwamp    Sector = "swamp"    // Haddon Mirk, Drowned Lands, Paetrinh
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
	// NoMap hides the room from the §10 BFS minimap. Neighbors render
	// it as `[?]` and the BFS does not recurse through it, so secret
	// hideouts and admin zones stay topologically opaque.
	NoMap bool
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
	ZoneID    int64
	Name      string
	ShortDesc string
	LongDesc  string
	Flags     RoomFlags
	Sector    Sector
	// LightLevel: 0 = pitch black; positive = lit. Combined with
	// Flags.Dark in look to decide whether descriptions are visible.
	// Default is 100 ("brightly lit") so existing zones keep rendering.
	LightLevel int
	CoordX     int
	CoordY     int
	CoordZ     int
	// CoordsAnchor marks rooms whose coords were explicitly authored
	// in YAML. The auto-coord BFS runner (internal/world/coords_derive)
	// propagates *from* anchors but never overwrites them; non-anchor
	// rooms are derived. Zero value (false) means "auto-derive" so
	// repo-created rooms (test fixtures, OLC) inherit the SQL default
	// of coords_auto=1 without ceremony. The world loader sets this
	// true whenever a room's YAML carries a `coords:` block. The SQL
	// column on disk is `coords_auto` (1=derive, 0=anchor), inverted
	// from this field name; conversion happens at the repo boundary.
	// Migration 0026 added the column.
	CoordsAnchor bool
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
	// ListIDsByZone returns the int ids of every room whose
	// rooms.zone_id matches. Order is unspecified — callers that
	// need stability should sort. Used by the ZoneResetter to
	// scope door-state restoration to a single zone.
	ListIDsByZone(ctx context.Context, zoneID int64) ([]int64, error)
	// ListAll returns every room in the database in id order. Used by
	// the auto-coord BFS runner (internal/world/coords_derive) at boot
	// to enumerate the world graph. Order is stable so anchor
	// selection is deterministic across boots.
	ListAll(ctx context.Context) ([]Room, error)
	// UpdateCoords overwrites the (x,y,z) of an existing room without
	// touching CoordsAuto. The auto-coord runner uses it to persist
	// derived coords; an explicit anchor's CoordsAuto stays false
	// regardless of whether its coords change. Returns ErrRoomNotFound
	// when no row matches.
	UpdateCoords(ctx context.Context, id int64, x, y, z int) error
}

// CoordsAutoInt converts a CoordsAnchor bool to the int the SQL
// rooms.coords_auto column expects. The on-disk encoding is inverted
// from the Go field name on purpose: coords_auto=1 ("auto-derive me",
// the schema default from migration 0026) corresponds to
// CoordsAnchor=false ("not pinned"). Centralized here so every site
// that writes the column — repo.Create, the world loader's raw INSERT
// in roomInsertValues, and any future OLC code path — uses the same
// conversion. A divergence between sites would silently flip every
// affected room's anchor state.
func CoordsAutoInt(anchor bool) int {
	if anchor {
		return 0
	}
	return 1
}

// CoordsAnchorFromInt is the inverse of CoordsAutoInt: turns a
// scanned coords_auto int back into the CoordsAnchor bool. Used by
// every scan site so the encoding direction lives in exactly one
// place.
func CoordsAnchorFromInt(coordsAuto int) bool {
	return coordsAuto == 0
}

// StarterRoomID is where new characters spawn. The YAML loader pins the
// room flagged `starter: true` to this id so the constant stays valid
// across loads.
const StarterRoomID int64 = 1

// DefaultLightLevel is the value applied to rooms loaded without an
// explicit value. 100 is "fully lit"; 0 is pitch black.
const DefaultLightLevel = 100

var ErrRoomNotFound = errors.New("repo: room not found")
