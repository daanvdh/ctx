package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestMigrateLegacySettingsReadsJSON(t *testing.T) {
	cfgDir := t.TempDir()
	legacyPath := filepath.Join(cfgDir, "settings.json")
	if err := os.WriteFile(legacyPath, []byte(`{"db_path": "/tmp/ctx.sqlite", "mcp_allowed_origins": ["a", "b"]}`), 0o600); err != nil {
		t.Fatalf("write legacy settings: %v", err)
	}

	settings, migrated, err := migrateLegacySettings(cfgDir)
	if err != nil {
		t.Fatalf("migrateLegacySettings() error = %v", err)
	}
	if !migrated {
		t.Fatal("migrateLegacySettings() migrated = false, want true")
	}
	if settings.DBPath != "/tmp/ctx.sqlite" {
		t.Errorf("DBPath = %q, want /tmp/ctx.sqlite", settings.DBPath)
	}
	if len(settings.MCPAllowedOrigins) != 2 || settings.MCPAllowedOrigins[0] != "a" || settings.MCPAllowedOrigins[1] != "b" {
		t.Errorf("MCPAllowedOrigins = %#v, want [a b]", settings.MCPAllowedOrigins)
	}
}

func TestMigrateLegacySettingsNoFile(t *testing.T) {
	cfgDir := t.TempDir()

	settings, migrated, err := migrateLegacySettings(cfgDir)
	if err != nil {
		t.Fatalf("migrateLegacySettings() error = %v", err)
	}
	if migrated {
		t.Fatal("migrateLegacySettings() migrated = true, want false")
	}
	if !reflect.DeepEqual(settings, Settings{}) {
		t.Errorf("settings = %#v, want zero value", settings)
	}
}

func TestWriteSettingsThenLoadSettingsRoundTrips(t *testing.T) {
	cfgDir := t.TempDir()
	want := Settings{
		DBPath:            "/tmp/ctx.sqlite",
		TriggerLocation:   "triggers",
		MCPHTTPAddr:       "127.0.0.1:7331",
		MCPAllowedOrigins: []string{"https://example.com"},
	}
	if err := writeSettings(cfgDir, want); err != nil {
		t.Fatalf("writeSettings() error = %v", err)
	}

	settingsPath := filepath.Join(cfgDir, "settings.yml")
	if _, err := os.Stat(settingsPath); err != nil {
		t.Fatalf("settings.yml not written: %v", err)
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings.yml: %v", err)
	}
	var got Settings
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal settings.yml: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round-tripped settings = %#v, want %#v", got, want)
	}
}
