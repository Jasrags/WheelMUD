package scripts

import (
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// defaultFS holds the canonical script catalog bundled into the
// binary. Mirrors internal/quest/embed.go and internal/chargen/embed.go
// — same env-override pattern.
//
//go:embed all:default
var defaultFS embed.FS

// ScriptDirEnv names the environment variable that overrides the
// embedded catalog. When set, Load reads `*.lua` from that directory
// directly via os.DirFS so builders can iterate without rebuilding.
const ScriptDirEnv = "SCRIPT_DIR"

// SourceFS returns the filesystem Load should read from. If
// SCRIPT_DIR is set, it points there; otherwise the embedded default
// rooted at `default/` is used so the walk visits
// `warden_alert.lua` rather than `default/warden_alert.lua`.
//
// On a malformed SCRIPT_DIR (missing, not a directory) SourceFS
// returns an error so the caller (cmd/server/main.go) can log it
// and exit cleanly. The embedded fallback panics because a missing
// embed is a build defect.
func SourceFS() (fs.FS, error) {
	dir := os.Getenv(ScriptDirEnv)
	if dir == "" {
		sub, err := fs.Sub(defaultFS, "default")
		if err != nil {
			panic("scripts: embedded default fs missing 'default' root: " + err.Error())
		}
		return sub, nil
	}
	resolved, err := resolveScriptDir(dir)
	if err != nil {
		return nil, fmt.Errorf("scripts: %s=%q is not usable: %w", ScriptDirEnv, dir, err)
	}
	slog.Info("scripts: SCRIPT_DIR override active", "raw", dir, "resolved", resolved)
	return os.DirFS(resolved), nil
}

func resolveScriptDir(dir string) (string, error) {
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

// Load walks fsys for every `*.lua` file at the root, compiles each
// via gopher-lua, and assembles a Catalog keyed by base filename
// (without the `.lua` suffix). Returns an error on:
//
//   - Read / IO failure (file path included)
//   - Parse failure (gopher-lua error verbatim with file prefix)
//   - Duplicate base name across files
//
// Empty catalog is valid: the runtime is no-op until builders
// author scripts. The boot log surfaces the count so an
// unintended-empty deploy is visible.
//
// l is a transient *lua.LState used only for parsing; the caller
// owns it and may close it after Load returns. The Runner owns its
// own pool of LStates for execution.
func Load(fsys fs.FS, l *lua.LState) (*Catalog, error) {
	cat := &Catalog{ByName: make(map[string]*Script)}

	walkErr := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(path), ".lua") {
			return nil
		}
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		if name == "" {
			return fmt.Errorf("%s: empty script name", path)
		}
		body, err := fs.ReadFile(fsys, path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		fn, err := compile(l, name, string(body))
		if err != nil {
			return fmt.Errorf("compile %s: %w", path, err)
		}
		if _, dup := cat.ByName[name]; dup {
			return fmt.Errorf("%s: duplicate script name %q", path, name)
		}
		cat.ByName[name] = &Script{
			Name:     name,
			Source:   string(body),
			Compiled: fn,
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return cat, nil
}
