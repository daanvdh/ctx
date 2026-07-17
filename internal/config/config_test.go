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

func TestDefaultSessionForPrefersMostSpecificPath(t *testing.T) {
	settings := Settings{
		DefaultSession: "global",
		DefaultSessions: map[string]string{
			"/home/u/git":     "git-wide",
			"/home/u/git/ctx": "ctx-dev",
		},
	}
	cases := map[string]string{
		"/home/u/git/ctx/internal": "ctx-dev",
		"/home/u/git/ctx":          "ctx-dev",
		"/home/u/git/other":        "git-wide",
		"/home/u/git/ctx-fork":     "git-wide", // not under /home/u/git/ctx: prefix must respect path boundaries
		"/somewhere/else":          "global",
	}
	for cwd, want := range cases {
		if got := DefaultSessionFor(settings, cwd); got != want {
			t.Fatalf("DefaultSessionFor(%q) = %q, want %q", cwd, got, want)
		}
	}
}

func TestDefaultSessionForEmptyWithoutConfig(t *testing.T) {
	if got := DefaultSessionFor(Settings{}, "/anywhere"); got != "" {
		t.Fatalf("DefaultSessionFor = %q, want empty", got)
	}
}
