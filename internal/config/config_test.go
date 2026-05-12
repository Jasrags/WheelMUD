package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// clearKnownEnv unsets every env var Load consults, so tests run hermetic.
// Callers Setenv specific vars after to test the env-override path.
func clearKnownEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"LISTEN_ADDR", "METRICS_ADDR", "DB_DSN", "BACKUP_DIR",
		"WORLD_DIR", "LOG_LEVEL",
		"AUDIT_COMMANDS_ENABLED", "AUDIT_COMMANDS_EXCLUDE",
	} {
		t.Setenv(k, "")
	}
}

func TestDefaults(t *testing.T) {
	d := Defaults()
	if d.Server.ListenAddr != ":2323" {
		t.Errorf("ListenAddr default = %q, want :2323", d.Server.ListenAddr)
	}
	if d.DB.DSN != "wheelmud.db" {
		t.Errorf("DSN default = %q, want wheelmud.db", d.DB.DSN)
	}
	if d.Log.Level != "debug" {
		t.Errorf("Log level default = %q, want debug", d.Log.Level)
	}
}

func TestLoad_NoPath_EnvOnly(t *testing.T) {
	clearKnownEnv(t)
	t.Setenv("LISTEN_ADDR", ":3000")
	t.Setenv("LOG_LEVEL", "warn")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load(\"\"): %v", err)
	}
	if cfg.Server.ListenAddr != ":3000" {
		t.Errorf("ListenAddr = %q, want :3000 (env override)", cfg.Server.ListenAddr)
	}
	if cfg.Log.Level != "warn" {
		t.Errorf("Log.Level = %q, want warn (env override)", cfg.Log.Level)
	}
	if cfg.DB.DSN != "wheelmud.db" {
		t.Errorf("DSN = %q, want wheelmud.db (default)", cfg.DB.DSN)
	}
}

func TestLoad_YAMLFile(t *testing.T) {
	clearKnownEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(`
server:
  listen_addr: ":4000"
  metrics_addr: "127.0.0.1:9999"
db:
  dsn: "/var/lib/wheelmud/wm.db"
  backup_dir: "/var/backups"
  backup_retention: 30
log:
  level: "info"
audit:
  commands_enabled: true
  commands_exclude: [look, prompt]
`), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load(%q): %v", path, err)
	}
	if cfg.Server.ListenAddr != ":4000" {
		t.Errorf("ListenAddr = %q, want :4000", cfg.Server.ListenAddr)
	}
	if cfg.Server.MetricsAddr != "127.0.0.1:9999" {
		t.Errorf("MetricsAddr = %q", cfg.Server.MetricsAddr)
	}
	if cfg.DB.DSN != "/var/lib/wheelmud/wm.db" {
		t.Errorf("DSN = %q", cfg.DB.DSN)
	}
	if cfg.DB.BackupRetention != 30 {
		t.Errorf("BackupRetention = %d, want 30", cfg.DB.BackupRetention)
	}
	if !cfg.Audit.CommandsEnabled {
		t.Errorf("Audit.CommandsEnabled = false, want true")
	}
	if len(cfg.Audit.CommandsExclude) != 2 || cfg.Audit.CommandsExclude[0] != "look" {
		t.Errorf("Audit.CommandsExclude = %v", cfg.Audit.CommandsExclude)
	}
}

func TestLoad_EnvOverridesFile(t *testing.T) {
	clearKnownEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(`
server:
  listen_addr: ":4000"
db:
  dsn: "/file/path.db"
`), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	t.Setenv("LISTEN_ADDR", ":5000")
	t.Setenv("DB_DSN", "/env/path.db")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.ListenAddr != ":5000" {
		t.Errorf("ListenAddr = %q, want :5000 (env wins over file)", cfg.Server.ListenAddr)
	}
	if cfg.DB.DSN != "/env/path.db" {
		t.Errorf("DSN = %q, want /env/path.db (env wins)", cfg.DB.DSN)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	clearKnownEnv(t)
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("Load on missing path: want error, got nil")
	}
	if !strings.Contains(err.Error(), "read config") {
		t.Errorf("error wrap = %q, want it to include 'read config'", err.Error())
	}
}

func TestLoad_AuditEnvOverrides(t *testing.T) {
	clearKnownEnv(t)
	t.Setenv("AUDIT_COMMANDS_ENABLED", "true")
	t.Setenv("AUDIT_COMMANDS_EXCLUDE", "look, prompt , , map")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Audit.CommandsEnabled {
		t.Errorf("Audit.CommandsEnabled = false, want true from env")
	}
	got := cfg.Audit.CommandsExclude
	want := []string{"look", "prompt", "map"}
	if len(got) != len(want) {
		t.Fatalf("CommandsExclude = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("CommandsExclude[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestLoad_AuditEnabledMalformedEnvIgnored(t *testing.T) {
	clearKnownEnv(t)
	t.Setenv("AUDIT_COMMANDS_ENABLED", "yesplease")
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Audit.CommandsEnabled {
		t.Errorf("CommandsEnabled = true, want false (malformed bool ignored)")
	}
}

func TestLoad_MalformedYAML(t *testing.T) {
	clearKnownEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("server: [invalid"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("Load on malformed YAML: want error, got nil")
	}
	if !strings.Contains(err.Error(), "parse config") {
		t.Errorf("error wrap = %q, want 'parse config'", err.Error())
	}
}

