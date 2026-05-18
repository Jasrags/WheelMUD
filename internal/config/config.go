// Package config loads server runtime configuration from an optional
// YAML file plus environment-variable overrides.
//
// Precedence (lowest -> highest): struct defaults -> YAML file -> env.
// A missing or empty path skips the file step. Env vars only override a
// field when their value is non-empty.
//
// Per-catalog disk overrides (WORLD_DIR / CHARGEN_DIR / QUEST_DIR /
// SCRIPT_DIR / EFFECTS_DIR) are NOT routed through this struct — they
// are read directly by each catalog's embed.go and remain env-only so
// the embedded-FS fallback path stays simple.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the server's top-level runtime configuration.
type Config struct {
	Server ServerConfig `yaml:"server"`
	DB     DBConfig     `yaml:"db"`
	World  WorldConfig  `yaml:"world"`
	Log    LogConfig    `yaml:"log"`
	Audit  AuditConfig  `yaml:"audit"`
	MSSP   MSSPConfig   `yaml:"mssp"`
	Combat CombatConfig `yaml:"combat"`
}

type ServerConfig struct {
	// ListenAddr is the telnet bind address. Default ":2323".
	ListenAddr string `yaml:"listen_addr"`

	// MetricsAddr is the Prometheus + pprof + healthz HTTP bind address
	// (Phase J slice J5). Empty disables the metrics server. Default
	// "127.0.0.1:9090" — loopback only.
	MetricsAddr string `yaml:"metrics_addr"`

	// FloodBytesPerSec caps outbound bytes per session via a token
	// bucket on Session.WriteRaw (§M.2). Zero or negative disables.
	// Default 65536 (64 KiB/s) — enough headroom for normal play and
	// boot-time room renders, tight enough to suffocate a runaway
	// script. FloodBurstBytes is the burst capacity; default 131072
	// (128 KiB).
	FloodBytesPerSec int `yaml:"flood_bytes_per_sec"`
	FloodBurstBytes  int `yaml:"flood_burst_bytes"`
}

type DBConfig struct {
	// DSN is the modernc/sqlite connection string. Default "wheelmud.db".
	DSN string `yaml:"dsn"`

	// BackupDir is the directory the backup manager (Phase J slice J4)
	// writes VACUUM INTO snapshots to. Empty disables backups.
	BackupDir string `yaml:"backup_dir"`

	// BackupIntervalHours controls cadence of the backup manager. 0
	// disables backups. Default 6 once enabled.
	BackupIntervalHours float64 `yaml:"backup_interval_hours"`

	// BackupRetention is the maximum number of snapshot files retained
	// in BackupDir; older ones are pruned. Default 14.
	BackupRetention int `yaml:"backup_retention"`
}

type WorldConfig struct {
	// Dir is the world content tree the YAML loader reads at boot. When
	// empty (the default) the loader uses its embedded FS. Equivalent
	// to setting WORLD_DIR.
	Dir string `yaml:"dir"`
}

type LogConfig struct {
	// Level is the slog level name (debug/info/warn/error). Default
	// "debug".
	Level string `yaml:"level"`
}

// MSSPConfig holds the public-facing strings published in MSSP
// responses to MUD crawlers. Zero-value strings are emitted as empty
// values (most crawlers tolerate empty fields). Status defaults to
// "Alpha" via Defaults(); the others have no default because they are
// deployment-specific.
type MSSPConfig struct {
	Contact  string `yaml:"contact"`  // e.g. "admin@example.com"
	Hostname string `yaml:"hostname"` // public DNS name
	Location string `yaml:"location"` // e.g. "USA"
	Website  string `yaml:"website"`  // e.g. "https://example.com"
	Status   string `yaml:"status"`   // "Alpha" / "Beta" / "Live"
}

// CombatConfig holds combat-side toggles. Phase D §19 closer.
type CombatConfig struct {
	// DropOnDeath dumps a dying player's top-level inventory, equipped
	// items, and carried coin (bank coin preserved) into a durable
	// player-corpse in the death room before the respawn move. When
	// true, the 10% XP-debt delta is waived — gear/coin loss replaces
	// XP debt as the death cost. Default false (V1 keeps everything,
	// applies the XP debt).
	DropOnDeath bool `yaml:"drop_on_death"`
}

type AuditConfig struct {
	// CommandsEnabled toggles per-character command audit logging
	// (Phase J slice J3). Default false.
	CommandsEnabled bool `yaml:"commands_enabled"`

	// CommandsExclude is a verb allow-list of high-frequency commands
	// to skip when CommandsEnabled is true (e.g. "look", "prompt").
	CommandsExclude []string `yaml:"commands_exclude"`
}

// Defaults returns a Config populated with the same fallback values the
// pre-config codebase used. Callers that don't need file overrides can
// use this directly and just apply env.
func Defaults() Config {
	return Config{
		Server: ServerConfig{
			ListenAddr:       ":2323",
			MetricsAddr:      "127.0.0.1:9090",
			FloodBytesPerSec: 64 << 10,  // 64 KiB/s sustained
			FloodBurstBytes:  128 << 10, // 128 KiB burst
		},
		DB: DBConfig{
			DSN:                 "wheelmud.db",
			BackupDir:           "",
			BackupIntervalHours: 6,
			BackupRetention:     14,
		},
		Log:  LogConfig{Level: "debug"},
		MSSP: MSSPConfig{Status: "Alpha"},
	}
}

// Load reads YAML from path (if non-empty) over Defaults, then applies
// environment overrides. A non-existent path returns an error; an empty
// path is "use defaults + env only".
func Load(path string) (Config, error) {
	cfg := Defaults()
	if path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return cfg, fmt.Errorf("read config %q: %w", path, err)
		}
		if err := yaml.Unmarshal(raw, &cfg); err != nil {
			return cfg, fmt.Errorf("parse config %q: %w", path, err)
		}
	}
	applyEnv(&cfg)
	return cfg, nil
}

// applyEnv overrides fields from non-empty environment variables.
// Mirrors the env-var surface the pre-config codebase already used.
func applyEnv(cfg *Config) {
	if v := os.Getenv("LISTEN_ADDR"); v != "" {
		cfg.Server.ListenAddr = v
	}
	if v := os.Getenv("METRICS_ADDR"); v != "" {
		cfg.Server.MetricsAddr = v
	}
	if v := os.Getenv("DB_DSN"); v != "" {
		cfg.DB.DSN = v
	}
	if v := os.Getenv("BACKUP_DIR"); v != "" {
		cfg.DB.BackupDir = v
	}
	if v := os.Getenv("WORLD_DIR"); v != "" {
		cfg.World.Dir = v
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.Log.Level = v
	}
	if v := os.Getenv("DROP_ON_DEATH"); v != "" {
		// strconv.ParseBool accepts 1/0/true/false/etc; malformed
		// values are ignored so a typo can't crash startup.
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.Combat.DropOnDeath = b
		}
	}
	if v := os.Getenv("AUDIT_COMMANDS_ENABLED"); v != "" {
		// Accept the same forms strconv.ParseBool does
		// ("1"/"0"/"true"/"false"/etc). Malformed values are ignored
		// so a typo in a side-channel env var can't crash startup.
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.Audit.CommandsEnabled = b
		}
	}
	if v := os.Getenv("MSSP_CONTACT"); v != "" {
		cfg.MSSP.Contact = v
	}
	if v := os.Getenv("MSSP_HOSTNAME"); v != "" {
		cfg.MSSP.Hostname = v
	}
	if v := os.Getenv("MSSP_LOCATION"); v != "" {
		cfg.MSSP.Location = v
	}
	if v := os.Getenv("MSSP_WEBSITE"); v != "" {
		cfg.MSSP.Website = v
	}
	if v := os.Getenv("MSSP_STATUS"); v != "" {
		cfg.MSSP.Status = v
	}
	if v := os.Getenv("AUDIT_COMMANDS_EXCLUDE"); v != "" {
		// Comma-separated list, trimmed; empty entries dropped so
		// trailing commas don't accidentally exclude "".
		parts := strings.Split(v, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if t := strings.TrimSpace(p); t != "" {
				out = append(out, t)
			}
		}
		cfg.Audit.CommandsExclude = out
	}
}
