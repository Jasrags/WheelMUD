package quest

import (
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// defaultFS holds the canonical quest catalog bundled into the
// binary. Mirrors internal/chargen/embed.go and internal/world/
// embed.go so the env-var override pattern is consistent across
// content loaders. Each quest is one YAML file under `default/`;
// the loader walks the tree and reads every `*.yaml`.
//
//go:embed all:default
var defaultFS embed.FS

// QuestDirEnv names the environment variable that overrides the
// embedded catalog. When set, Load reads YAML from that directory
// directly (via os.DirFS) so builders can iterate without
// rebuilding.
const QuestDirEnv = "QUEST_DIR"

// SourceFS returns the filesystem Load should read from. If
// QUEST_DIR is set in the environment, it points at that directory;
// otherwise the embedded default rooted at `default/` is used so
// the YAML walk visits `lost_lamb.yaml` rather than
// `default/lost_lamb.yaml`.
//
// On a malformed QUEST_DIR (missing, not a directory, IO error)
// SourceFS returns an error so the caller (cmd/server/main.go) can
// log it and exit cleanly. The embedded fallback panics because a
// missing embed is a build defect, not a deployment one.
func SourceFS() (fs.FS, error) {
	dir := os.Getenv(QuestDirEnv)
	if dir == "" {
		sub, err := fs.Sub(defaultFS, "default")
		if err != nil {
			panic("quest: embedded default fs missing 'default' root: " + err.Error())
		}
		return sub, nil
	}
	resolved, err := resolveQuestDir(dir)
	if err != nil {
		return nil, fmt.Errorf("quest: %s=%q is not usable: %w", QuestDirEnv, dir, err)
	}
	slog.Info("quest: QUEST_DIR override active", "raw", dir, "resolved", resolved)
	return os.DirFS(resolved), nil
}

func resolveQuestDir(dir string) (string, error) {
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

// Load walks fsys for every `*.yaml` file at the root, parses each
// as one Quest, and assembles a Catalog keyed by Quest.ID. Returns
// an error on:
//
//   - YAML parse failure (file path included)
//   - duplicate Quest.ID across files
//   - empty catalog (zero quests authored)
//
// Cross-reference validation against world content is the caller's
// responsibility — pass the loaded Catalog to Validate with non-nil
// RefSets after the world loader has populated them.
func Load(fsys fs.FS) (*Catalog, error) {
	cat := &Catalog{ByID: make(map[string]*Quest)}

	walkErr := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(path), ".yaml") {
			return nil
		}
		body, err := fs.ReadFile(fsys, path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		var q Quest
		if err := yaml.Unmarshal(body, &q); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		if q.ID == "" {
			return fmt.Errorf("%s: quest has no id", path)
		}
		if _, dup := cat.ByID[q.ID]; dup {
			return fmt.Errorf("%s: duplicate quest id %q", path, q.ID)
		}
		// Take a copy so the slice header points to a stable backing
		// array even if the loader reuses buffers across iterations.
		qq := q
		cat.ByID[qq.ID] = &qq
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	// An empty catalog is valid: the engine + verb both handle it
	// without surfacing — players see "no active quests", and the
	// engine is a no-op until builders author content. The boot log
	// still surfaces the count so an unintended-empty deploy is
	// visible.
	return cat, nil
}
