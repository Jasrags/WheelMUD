package help

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
)

// HelpDirEnv names the environment variable that overrides the
// embedded catalog. When set, LoadFS reads `<dir>/topics/*.md`
// directly via os.DirFS so operators can iterate on help content
// without rebuilding the binary.
const HelpDirEnv = "HELP_DIR"

// SourceFS returns the filesystem LoadFS should read from. If
// HELP_DIR is set in the environment, it points at that directory;
// otherwise the embedded default is used, rooted at `assets/` so the
// caller sees `topics/<id>.md` at the fs root (matching the on-disk
// override layout).
//
// On a malformed HELP_DIR (missing, not a directory, IO error)
// SourceFS returns an error so the caller (cmd/server/main.go) can
// log it through the structured logger and exit cleanly. The
// embedded fallback panics because a missing embed is a build
// defect, not a deployment one. Mirrors chargen.SourceFS.
func SourceFS() (fs.FS, error) {
	dir := os.Getenv(HelpDirEnv)
	if dir == "" {
		sub, err := fs.Sub(assets, "assets")
		if err != nil {
			panic("help: embedded default fs missing 'assets' root: " + err.Error())
		}
		return sub, nil
	}
	resolved, err := resolveHelpDir(dir)
	if err != nil {
		return nil, fmt.Errorf("help: %s=%q is not usable: %w", HelpDirEnv, dir, err)
	}
	slog.Info("help: HELP_DIR override active", "raw", dir, "resolved", resolved)
	return os.DirFS(resolved), nil
}

func resolveHelpDir(dir string) (string, error) {
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
