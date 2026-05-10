package effects

import (
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
)

// defaultFS holds the canonical effect catalog bundled into the
// binary. Mirrors internal/chargen/embed.go so the env-var override
// pattern is consistent across content loaders.
//
//go:embed all:default
var defaultFS embed.FS

// EffectsDirEnv names the environment variable that overrides the
// embedded catalog. When set, Load reads YAML from that directory
// directly so builders can iterate without rebuilding.
const EffectsDirEnv = "EFFECTS_DIR"

// SourceFS returns the filesystem Load should read from.
func SourceFS() (fs.FS, error) {
	dir := os.Getenv(EffectsDirEnv)
	if dir == "" {
		sub, err := fs.Sub(defaultFS, "default")
		if err != nil {
			panic("effects: embedded default fs missing 'default' root: " + err.Error())
		}
		return sub, nil
	}
	resolved, err := resolveDir(dir)
	if err != nil {
		return nil, fmt.Errorf("effects: %s=%q is not usable: %w", EffectsDirEnv, dir, err)
	}
	slog.Info("effects: EFFECTS_DIR override active", "raw", dir, "resolved", resolved)
	return os.DirFS(resolved), nil
}

func resolveDir(dir string) (string, error) {
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
