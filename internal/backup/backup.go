// Package backup provides scheduled SQLite snapshots via VACUUM INTO
// plus mtime-based retention pruning. Wall-clock cadence (decoupled
// from tick.Buckets so a paused world doesn't pause backups).
//
// Phase J slice J4 (#56). Disabled when Dir == "" or IntervalHours <= 0;
// the wired-up server (cmd/server/main.go) skips constructing the
// Manager in that case.
package backup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// FilePrefix is the prefix used for snapshot filenames so retention
// pruning can identify backup files without grabbing unrelated files
// that happen to live in the same directory.
const FilePrefix = "wheelmud-"

// FileSuffix is the snapshot filename suffix. The full filename shape
// is FilePrefix + "YYYYMMDD-HHMMSS" + FileSuffix.
const FileSuffix = ".db"

// timeLayout encodes the snapshot timestamp into a filename-safe
// lexicographically-sortable form. Stable so retention pruning can
// rely on filename order matching mtime order in the common case.
const timeLayout = "20060102-150405"

// Config controls Manager behavior. Zero IntervalHours and empty Dir
// both mean "disabled" — the Manager should not be constructed.
type Config struct {
	// Dir is the directory snapshots are written to. Created with
	// 0o755 if missing. Required.
	Dir string

	// IntervalHours is the wall-clock cadence between snapshots.
	// Sub-hour values (fractional hours) are accepted so tests and
	// short-lived deploys can take more frequent snapshots.
	IntervalHours float64

	// Retention caps the number of snapshot files kept in Dir; older
	// files (sorted by filename, which encodes ts) are deleted after
	// each new snapshot. Zero or negative is treated as 1 — keep at
	// least the snapshot just taken.
	Retention int

	// Now is a clock injection point for tests. Defaults to
	// time.Now().UTC().
	Now func() time.Time
}

// Manager periodically writes VACUUM INTO snapshots and prunes old
// ones by retention count. Run blocks until the context is canceled;
// RunOnce performs a single snapshot+prune cycle and is the unit-test
// entry point.
type Manager struct {
	db  *sql.DB
	cfg Config
}

// New returns a Manager wired against db with cfg. Returns an error
// if cfg is malformed (empty Dir, zero/negative IntervalHours).
func New(db *sql.DB, cfg Config) (*Manager, error) {
	if db == nil {
		return nil, errors.New("backup: nil db")
	}
	if cfg.Dir == "" {
		return nil, errors.New("backup: empty Dir")
	}
	if cfg.IntervalHours <= 0 {
		return nil, errors.New("backup: IntervalHours must be > 0")
	}
	if cfg.Retention < 1 {
		cfg.Retention = 1
	}
	if cfg.Now == nil {
		cfg.Now = func() time.Time { return time.Now().UTC() }
	}
	return &Manager{db: db, cfg: cfg}, nil
}

// Run takes snapshots on the configured cadence until ctx is canceled.
// One snapshot fires immediately on entry so an operator restart
// always produces a fresh baseline. Snapshot errors log at warn and
// the loop continues — a transient disk-full or VACUUM lock should
// not tear down the server.
func (m *Manager) Run(ctx context.Context) {
	if err := m.RunOnce(ctx); err != nil {
		slog.Warn("backup: initial snapshot failed", "error", err)
	}
	interval := time.Duration(m.cfg.IntervalHours * float64(time.Hour))
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := m.RunOnce(ctx); err != nil {
				slog.Warn("backup: snapshot failed", "error", err)
			}
		}
	}
}

// RunOnce writes a single snapshot then prunes Dir to Retention.
// Returns the snapshot's path on success.
func (m *Manager) RunOnce(ctx context.Context) error {
	if err := os.MkdirAll(m.cfg.Dir, 0o755); err != nil {
		return fmt.Errorf("backup: mkdir %q: %w", m.cfg.Dir, err)
	}
	now := m.cfg.Now()
	path := filepath.Join(m.cfg.Dir, FilePrefix+now.Format(timeLayout)+FileSuffix)

	// modernc/sqlite supports VACUUM INTO directly; the literal path
	// must be a quoted SQL string. Path goes through strconv.Quote so
	// embedded single-quotes / newlines / control bytes don't break
	// out of the statement. Operators control Dir; this is defense
	// against pathological hostnames or future Dir templating.
	q := "VACUUM INTO " + sqlQuote(path)
	if _, err := m.db.ExecContext(ctx, q); err != nil {
		return fmt.Errorf("backup: VACUUM INTO %q: %w", path, err)
	}
	slog.Info("backup: snapshot written", "path", path)

	if err := m.prune(); err != nil {
		// Log but don't fail — the snapshot itself succeeded.
		slog.Warn("backup: prune failed", "error", err)
	}
	return nil
}

// prune deletes snapshot files in Dir beyond Retention, oldest-first.
// Identification by FilePrefix + FileSuffix; non-matching files in
// the directory are ignored.
func (m *Manager) prune() error {
	entries, err := os.ReadDir(m.cfg.Dir)
	if err != nil {
		return fmt.Errorf("read dir %q: %w", m.cfg.Dir, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if !strings.HasPrefix(n, FilePrefix) || !strings.HasSuffix(n, FileSuffix) {
			continue
		}
		names = append(names, n)
	}
	if len(names) <= m.cfg.Retention {
		return nil
	}
	// Filenames embed the timestamp in lexicographic order; sorting
	// ascending puts oldest first.
	sort.Strings(names)
	for _, n := range names[:len(names)-m.cfg.Retention] {
		p := filepath.Join(m.cfg.Dir, n)
		// Defense against a world-writable backup dir: refuse to
		// follow symlinks during prune so a planted symlink with a
		// matching name can't be used as a deletion primitive on
		// arbitrary files. Lstat returns metadata for the link
		// itself, not the target.
		info, err := os.Lstat(p)
		if err != nil {
			slog.Warn("backup: lstat before prune failed", "path", p, "error", err)
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			slog.Warn("backup: refusing to prune non-regular file", "path", p, "mode", info.Mode())
			continue
		}
		if err := os.Remove(p); err != nil {
			slog.Warn("backup: failed to prune", "path", p, "error", err)
			continue
		}
		slog.Debug("backup: pruned old snapshot", "path", p)
	}
	return nil
}

// sqlQuote returns s wrapped in single quotes, with any embedded
// single quotes doubled. Standard SQL string literal escaping.
func sqlQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
