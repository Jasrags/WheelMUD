package world

import (
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
)

// defaultFS holds the canonical world bundled into the binary, mirroring
// the migrations precedent at internal/db/db.go.
//
//go:embed all:default
var defaultFS embed.FS

// WorldDirEnv names the environment variable that overrides the
// embedded default. When set, the loader reads YAML from that directory
// (via os.DirFS) so builders can iterate without rebuilding.
const WorldDirEnv = "WORLD_DIR"

// SourceFS returns the filesystem the loader should read from. If
// WORLD_DIR is set in the environment, it points at that directory
// (rooted there — zone folders sit directly under it). Otherwise it
// returns the embedded default world rooted at the `default/`
// subfolder so the YAML walks "starter/zone.yaml" rather than
// "default/starter/zone.yaml".
//
// On a malformed WORLD_DIR (missing directory, not a directory, or
// IO error on stat) SourceFS panics — the loader runs at boot and a
// silent fallback to the embedded world would mask a misconfigured
// deployment.
func SourceFS() fs.FS {
	dir := os.Getenv(WorldDirEnv)
	if dir == "" {
		sub, err := fs.Sub(defaultFS, "default")
		if err != nil {
			// Programmer error — the embed directive guarantees the
			// `default` directory exists.
			panic("world: embedded default fs missing 'default' root: " + err.Error())
		}
		return sub
	}

	resolved, err := resolveWorldDir(dir)
	if err != nil {
		panic(fmt.Sprintf("world: %s=%q is not usable: %v", WorldDirEnv, dir, err))
	}
	slog.Info("world: WORLD_DIR override active", "raw", dir, "resolved", resolved)
	return os.DirFS(resolved)
}

// resolveWorldDir cleans the operator-supplied path, requires it to
// exist and be a directory, and returns the absolute form so the
// resulting log line is unambiguous about which tree was loaded.
// Relative paths are accepted (resolved against the process working
// directory); symlinks at the root are followed.
func resolveWorldDir(dir string) (string, error) {
	clean := filepath.Clean(dir)
	abs, err := filepath.Abs(clean)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", abs, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", abs)
	}
	return abs, nil
}
