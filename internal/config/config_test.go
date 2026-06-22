package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSettingsParseErrorReportsLocationAndNearbySetting(t *testing.T) {
	data := []byte("{\n  \"db_path\": \"/tmp/ctx.sqlite\"\n  \"mcp_http_addr\": \"127.0.0.1:7331\"\n}")
	err := settingsParseError(data, jsonSyntaxError(t, data))

	for _, want := range []string{
		"line 3, column 3",
		`near setting "mcp_http_addr"`,
		"missing comma",
	} {
		if !strings.Contains(err, want) {
			t.Fatalf("settingsParseError = %q, want substring %q", err, want)
		}
	}
}

func jsonSyntaxError(t *testing.T, data []byte) error {
	t.Helper()
	var settings Settings
	if err := json.Unmarshal(data, &settings); err != nil {
		return err
	}
	t.Fatal("expected JSON syntax error")
	return nil
}
