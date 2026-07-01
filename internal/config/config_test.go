package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

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
