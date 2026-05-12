package backup

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Jasrags/WheelMUD/internal/db"
)

func TestNew_RejectsBadConfig(t *testing.T) {
	conn, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	cases := []struct {
		name string
		cfg  Config
	}{
		{"empty dir", Config{IntervalHours: 1}},
		{"zero interval", Config{Dir: t.TempDir()}},
		{"negative interval", Config{Dir: t.TempDir(), IntervalHours: -1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(conn, tc.cfg); err == nil {
				t.Fatalf("New(%+v): want error, got nil", tc.cfg)
			}
		})
	}
}

func TestNew_NilDB(t *testing.T) {
	if _, err := New(nil, Config{Dir: t.TempDir(), IntervalHours: 1}); err == nil {
		t.Fatal("New(nil db): want error, got nil")
	}
}

func TestRunOnce_WritesValidSnapshot(t *testing.T) {
	// Use a tempfile DB so VACUUM INTO writes a file we can re-open;
	// :memory: would refuse the VACUUM INTO target on some builds.
	dir := t.TempDir()
	srcDSN := filepath.Join(dir, "source.db")
	conn, err := db.Open(context.Background(), srcDSN)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	// Seed a row so the snapshot has observable content.
	if _, err := conn.Exec(`CREATE TABLE probe(id INTEGER PRIMARY KEY, v TEXT NOT NULL)`); err != nil {
		t.Fatalf("create probe: %v", err)
	}
	if _, err := conn.Exec(`INSERT INTO probe(v) VALUES (?)`, "hello"); err != nil {
		t.Fatalf("insert probe: %v", err)
	}

	backupDir := filepath.Join(dir, "backups")
	m, err := New(conn, Config{
		Dir:           backupDir,
		IntervalHours: 1,
		Retention:     5,
		Now:           func() time.Time { return time.Date(2026, 5, 12, 10, 30, 45, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := m.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	wantPath := filepath.Join(backupDir, "wheelmud-20260512-103045.db")
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("snapshot not created at %q: %v", wantPath, err)
	}

	// Open the snapshot and verify the probe row round-trips.
	snap, err := sql.Open("sqlite", wantPath)
	if err != nil {
		t.Fatalf("open snapshot: %v", err)
	}
	defer snap.Close()
	var v string
	if err := snap.QueryRow(`SELECT v FROM probe LIMIT 1`).Scan(&v); err != nil {
		t.Fatalf("scan snapshot: %v", err)
	}
	if v != "hello" {
		t.Fatalf("snapshot row = %q, want hello", v)
	}
}

func TestRunOnce_PrunesByRetention(t *testing.T) {
	dir := t.TempDir()
	srcDSN := filepath.Join(dir, "src.db")
	conn, err := db.Open(context.Background(), srcDSN)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	backupDir := filepath.Join(dir, "backups")
	var clock time.Time = time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC)
	m, err := New(conn, Config{
		Dir:           backupDir,
		IntervalHours: 1,
		Retention:     3,
		Now:           func() time.Time { return clock },
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Take 5 snapshots, advancing the clock by 1m each time. Final
	// directory should hold exactly 3 (the newest).
	for i := 0; i < 5; i++ {
		clock = clock.Add(1 * time.Minute)
		if err := m.RunOnce(context.Background()); err != nil {
			t.Fatalf("RunOnce %d: %v", i, err)
		}
	}

	files := listBackups(t, backupDir)
	if len(files) != 3 {
		t.Fatalf("retention not enforced: %d files: %v", len(files), files)
	}
	// Surviving names should be the three newest (minutes 03/04/05).
	wantTails := []string{"-000300.db", "-000400.db", "-000500.db"}
	for i, name := range files {
		if !strings.HasSuffix(name, wantTails[i]) {
			t.Errorf("file[%d]=%q, want suffix %q", i, name, wantTails[i])
		}
	}
}

func TestRunOnce_RetentionFloorIsOne(t *testing.T) {
	// Retention < 1 should be clamped so the snapshot just written
	// isn't immediately deleted.
	dir := t.TempDir()
	srcDSN := filepath.Join(dir, "src.db")
	conn, err := db.Open(context.Background(), srcDSN)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()
	backupDir := filepath.Join(dir, "backups")
	m, err := New(conn, Config{
		Dir:           backupDir,
		IntervalHours: 1,
		Retention:     0,
		Now:           func() time.Time { return time.Date(2026, 5, 12, 1, 2, 3, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := m.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if files := listBackups(t, backupDir); len(files) != 1 {
		t.Fatalf("retention=0 should clamp to 1, got %d files: %v", len(files), files)
	}
}

func TestPrune_IgnoresUnrelatedFiles(t *testing.T) {
	dir := t.TempDir()
	srcDSN := filepath.Join(dir, "src.db")
	conn, err := db.Open(context.Background(), srcDSN)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	backupDir := filepath.Join(dir, "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Drop some unrelated files that must NOT be pruned.
	for _, n := range []string{"README.md", "other-backup.tar.gz", "wheelmud-bad.txt"} {
		if err := os.WriteFile(filepath.Join(backupDir, n), []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", n, err)
		}
	}

	var clock time.Time = time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC)
	m, err := New(conn, Config{
		Dir:           backupDir,
		IntervalHours: 1,
		Retention:     1,
		Now:           func() time.Time { return clock },
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for i := 0; i < 3; i++ {
		clock = clock.Add(1 * time.Minute)
		if err := m.RunOnce(context.Background()); err != nil {
			t.Fatalf("RunOnce %d: %v", i, err)
		}
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	hasReadme, hasTar, hasTxt := false, false, false
	for _, e := range entries {
		switch e.Name() {
		case "README.md":
			hasReadme = true
		case "other-backup.tar.gz":
			hasTar = true
		case "wheelmud-bad.txt":
			hasTxt = true
		}
	}
	if !hasReadme || !hasTar || !hasTxt {
		t.Fatalf("unrelated files were pruned: README=%v tar=%v txt=%v",
			hasReadme, hasTar, hasTxt)
	}
}

func TestPrune_RefusesToFollowSymlinks(t *testing.T) {
	// Defense: a planted symlink whose name matches FilePrefix +
	// FileSuffix must NOT be followed during prune. Otherwise a
	// world-writable backup dir lets an attacker turn pruning into
	// an arbitrary-file deletion primitive.
	dir := t.TempDir()
	srcDSN := filepath.Join(dir, "src.db")
	conn, err := db.Open(context.Background(), srcDSN)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	backupDir := filepath.Join(dir, "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Target the symlink could point at — file outside backupDir.
	victim := filepath.Join(dir, "victim.txt")
	if err := os.WriteFile(victim, []byte("must survive"), 0o644); err != nil {
		t.Fatalf("write victim: %v", err)
	}
	// Symlink with a matching name that would otherwise be a prune target.
	link := filepath.Join(backupDir, FilePrefix+"20000101-000000"+FileSuffix)
	if err := os.Symlink(victim, link); err != nil {
		t.Skipf("symlink unsupported on this platform: %v", err)
	}

	var clock = time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC)
	m, err := New(conn, Config{
		Dir:           backupDir,
		IntervalHours: 1,
		Retention:     1,
		Now:           func() time.Time { return clock },
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for i := 0; i < 3; i++ {
		clock = clock.Add(1 * time.Minute)
		if err := m.RunOnce(context.Background()); err != nil {
			t.Fatalf("RunOnce %d: %v", i, err)
		}
	}

	// Symlink target must be untouched.
	if _, err := os.Stat(victim); err != nil {
		t.Fatalf("symlink target was deleted: %v", err)
	}
	body, _ := os.ReadFile(victim)
	if string(body) != "must survive" {
		t.Fatalf("victim contents mutated: %q", body)
	}
}

func TestRun_RespectsContextCancel(t *testing.T) {
	dir := t.TempDir()
	srcDSN := filepath.Join(dir, "src.db")
	conn, err := db.Open(context.Background(), srcDSN)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()
	m, err := New(conn, Config{
		Dir:           filepath.Join(dir, "backups"),
		IntervalHours: 24, // long interval so the test exits on ctx cancel, not tick
		Retention:     1,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		m.Run(ctx)
		close(done)
	}()
	// Give the initial RunOnce a moment to fire, then cancel.
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after ctx cancel")
	}
}

// listBackups returns snapshot filenames (sorted ascending) for the
// retention assertions.
func listBackups(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasPrefix(e.Name(), FilePrefix) || !strings.HasSuffix(e.Name(), FileSuffix) {
			continue
		}
		out = append(out, e.Name())
	}
	return out
}
