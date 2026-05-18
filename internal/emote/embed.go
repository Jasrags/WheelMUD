package emote

import (
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
)

// DirEnv names the environment variable that overrides the embedded
// catalog. When set, SourceFS reads `<dir>/*.yaml` directly via
// os.DirFS so operators can iterate on socials without rebuilding the
// binary. Mirrors HELP_DIR / CHARGEN_DIR.
const DirEnv = "EMOTE_DIR"

//go:embed default/*.yaml
var assets embed.FS

// SourceFS returns the filesystem Load should read from. If EMOTE_DIR
// is set in the environment, it points at that directory; otherwise
// the embedded default is used, rooted at `default/` so the caller
// sees `<file>.yaml` at the fs root (matching the on-disk override
// layout).
//
// On a malformed EMOTE_DIR (missing, not a directory, IO error)
// SourceFS returns an error so the caller can log it through the
// structured logger and exit cleanly. The embedded fallback panics
// because a missing embed is a build defect, not a deployment one.
// Mirrors help.SourceFS.
func SourceFS() (fs.FS, error) {
	dir := os.Getenv(DirEnv)
	if dir == "" {
		sub, err := fs.Sub(assets, "default")
		if err != nil {
			panic("emote: embedded default fs missing 'default' root: " + err.Error())
		}
		return sub, nil
	}
	resolved, err := resolveDir(dir)
	if err != nil {
		return nil, fmt.Errorf("emote: %s=%q is not usable: %w", DirEnv, dir, err)
	}
	slog.Info("emote: EMOTE_DIR override active", "raw", dir, "resolved", resolved)
	return os.DirFS(resolved), nil
}

func resolveDir(dir string) (string, error) {
	clean := filepath.Clean(dir)
	abs, err := filepath.Abs(clean)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path: %w", err)
	}
	// Lstat (not Stat) so a symlink pointing outside the intended tree
	// fails fast — matches internal/backup.go's anti-symlink stance.
	// EMOTE_DIR is operator-controlled so the risk is low, but the
	// project convention is to refuse symlinked roots explicitly.
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
