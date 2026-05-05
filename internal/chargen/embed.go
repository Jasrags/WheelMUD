package chargen

import (
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
)

// defaultFS holds the canonical chargen catalogs bundled into the
// binary. Mirrors internal/world/embed.go so the env-var override
// pattern is consistent across content loaders.
//
//go:embed all:default
var defaultFS embed.FS

// ChargenDirEnv names the environment variable that overrides the
// embedded catalog. When set, Load reads YAML from that directory
// directly (via os.DirFS) so builders can iterate without rebuilding.
const ChargenDirEnv = "CHARGEN_DIR"

// SourceFS returns the filesystem Load should read from. If
// CHARGEN_DIR is set in the environment, it points at that
// directory; otherwise the embedded default rooted at `default/` is
// used so YAML walks `backgrounds.yaml` rather than
// `default/backgrounds.yaml`.
//
// On a malformed CHARGEN_DIR (missing, not a directory, IO error)
// SourceFS returns an error so the caller (cmd/server/main.go) can
// log it through the structured logger and exit cleanly — matching
// every other boot-critical loader. The embedded fallback panics
// because a missing embed is a build defect, not a deployment one.
func SourceFS() (fs.FS, error) {
	dir := os.Getenv(ChargenDirEnv)
	if dir == "" {
		sub, err := fs.Sub(defaultFS, "default")
		if err != nil {
			panic("chargen: embedded default fs missing 'default' root: " + err.Error())
		}
		return sub, nil
	}
	resolved, err := resolveChargenDir(dir)
	if err != nil {
		return nil, fmt.Errorf("chargen: %s=%q is not usable: %w", ChargenDirEnv, dir, err)
	}
	slog.Info("chargen: CHARGEN_DIR override active", "raw", dir, "resolved", resolved)
	return os.DirFS(resolved), nil
}

func resolveChargenDir(dir string) (string, error) {
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
