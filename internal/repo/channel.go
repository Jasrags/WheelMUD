package repo

import (
	"context"
	"errors"
)

// Channel is one row from the channels catalog seeded by migration
// 0011. The server registers a telnet.Command per channel at
// startup; toggle/broadcast handlers consult MinLevel and Color.
type Channel struct {
	ID       int64
	Name     string
	MinLevel int16
	Color    string
}

// ChannelRepo is the persistence boundary for the channel catalog.
// Read-only at runtime; admins curate it via migrations.
type ChannelRepo interface {
	// List returns every channel ordered by name. Errors propagate
	// from the underlying store.
	List(ctx context.Context) ([]Channel, error)
}

// ErrChannelNotFound is returned when a lookup misses; the catalog
// is small enough that List + filter is the common path.
var ErrChannelNotFound = errors.New("repo: channel not found")
