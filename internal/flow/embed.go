package flow

import (
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
)

// DirEnv names the environment variable that overrides the embedded
// flow catalog. When set, SourceFS reads `<dir>/*.yaml` directly via
// os.DirFS so operators can iterate without rebuilding the binary.
// Mirrors HELP_DIR / EMOTE_DIR / CHARGEN_DIR.
const DirEnv = "FLOW_DIR"

//go:embed default/*.yaml
var assets embed.FS

// SourceFS returns the filesystem Load reads flow definitions from.
// FLOW_DIR override resolves to `os.DirFS(abs)` after an Lstat-based
// symlink rejection (matches §M.4's anti-symlink stance on
// EMOTE_DIR). Missing FLOW_DIR / not-a-directory / symlink → error.
// Missing embed default → panic (build defect).
func SourceFS() (fs.FS, error) {
	dir := os.Getenv(DirEnv)
	if dir == "" {
		sub, err := fs.Sub(assets, "default")
		if err != nil {
			panic("flow: embedded default fs missing 'default' root: " + err.Error())
		}
		return sub, nil
	}
	resolved, err := resolveDir(dir)
	if err != nil {
		return nil, fmt.Errorf("flow: %s=%q is not usable: %w", DirEnv, dir, err)
	}
	slog.Info("flow: FLOW_DIR override active", "raw", dir, "resolved", resolved)
	return os.DirFS(resolved), nil
}

func resolveDir(dir string) (string, error) {
	clean := filepath.Clean(dir)
	abs, err := filepath.Abs(clean)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path: %w", err)
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return "", fmt.Errorf("lstat %s: %w", abs, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("%s is a symlink", abs)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", abs)
	}
	return abs, nil
}
