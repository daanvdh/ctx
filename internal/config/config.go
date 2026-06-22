package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Settings struct {
	DBPath               string   `json:"db_path"`
	TriggerLocation      string   `json:"trigger_location"`
	MCPHTTPAddr          string   `json:"mcp_http_addr"`
	MCPHTTPPath          string   `json:"mcp_http_path"`
	MCPServerName        string   `json:"mcp_server_name"`
	MCPAllowedOrigins    []string `json:"mcp_allowed_origins"`
	MCPToken             string   `json:"mcp_token"`
	MCPOAuthClientID     string   `json:"mcp_oauth_client_id"`
	MCPOAuthClientSecret string   `json:"mcp_oauth_client_secret"`
	MCPPublicURL         string   `json:"mcp_public_url"`
}

func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	cfgDir := filepath.Join(home, ".config", "ctx")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		return "", fmt.Errorf("failed to create config directory %s: %w", cfgDir, err)
	}

	return cfgDir, nil
}

func DBPath() (string, error) {
	settings, cfgDir, err := loadSettings()
	if err != nil {
		return "", err
	}

	if settings.DBPath == "" {
		settings.DBPath = filepath.Join(cfgDir, "ctx.sqlite")
		if err := writeSettings(cfgDir, settings); err != nil {
			return "", err
		}
	}

	if !filepath.IsAbs(settings.DBPath) {
		settings.DBPath = filepath.Join(cfgDir, settings.DBPath)
	}

	if err := os.MkdirAll(filepath.Dir(settings.DBPath), 0o755); err != nil {
		return "", fmt.Errorf("failed to create directory %s for db path: %w", filepath.Dir(settings.DBPath), err)
	}

	return settings.DBPath, nil
}

func TriggerDir() (string, error) {
	settings, cfgDir, err := loadSettings()
	if err != nil {
		return "", err
	}

	dir := settings.TriggerLocation
	if dir == "" {
		dir = filepath.Join(cfgDir, "triggers")
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(cfgDir, dir)
	}
	return dir, nil
}

func LoadSettings() (Settings, error) {
	settings, _, err := loadSettings()
	return settings, err
}

func loadSettings() (Settings, string, error) {
	cfgDir, err := Dir()
	if err != nil {
		return Settings{}, "", err
	}
	settingsPath := filepath.Join(cfgDir, "settings.json")
	var settings Settings

	if data, err := os.ReadFile(settingsPath); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return Settings{}, "", fmt.Errorf("failed to parse settings file %s: %s", settingsPath, settingsParseError(data, err))
		}
	} else if !os.IsNotExist(err) {
		return Settings{}, "", fmt.Errorf("failed to read settings file %s: %w", settingsPath, err)
	}

	return settings, cfgDir, nil
}

func writeSettings(cfgDir string, settings Settings) error {
	settingsPath := filepath.Join(cfgDir, "settings.json")
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode default settings: %w", err)
	}
	if err := os.WriteFile(settingsPath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write settings file %s: %w", settingsPath, err)
	}
	return nil
}

func TriggerPath(name string) (string, error) {
	triggerDir, err := TriggerDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(triggerDir, name), nil
}

func settingsParseError(data []byte, err error) string {
	syntaxErr, ok := err.(*json.SyntaxError)
	if !ok {
		return err.Error()
	}

	line, col := lineColumn(data, syntaxErr.Offset)
	key := nearbyJSONKey(data, syntaxErr.Offset)
	msg := fmt.Sprintf("%s at line %d, column %d", err.Error(), line, col)
	if key != "" {
		msg += fmt.Sprintf(" near setting %q", key)
	}
	if strings.Contains(err.Error(), "after object key:value pair") {
		msg += "; check the previous setting for a missing comma"
	}
	return msg
}

func lineColumn(data []byte, offset int64) (int, int) {
	if offset < 1 {
		offset = 1
	}
	if offset > int64(len(data)) {
		offset = int64(len(data))
	}
	line, col := 1, 1
	for i := int64(0); i < offset-1; i++ {
		if data[i] == '\n' {
			line++
			col = 1
			continue
		}
		col++
	}
	return line, col
}

func nearbyJSONKey(data []byte, offset int64) string {
	if offset < 1 {
		offset = 1
	}
	if offset > int64(len(data)) {
		offset = int64(len(data))
	}
	start := int(offset) - 1
	for start > 0 && data[start] != '\n' && data[start] != '{' && data[start] != ',' {
		start--
	}
	fragment := string(data[start:int(offset)])
	if key := firstQuotedString(fragment); key != "" {
		return key
	}

	end := int(offset)
	for end < len(data) && data[end] != '\n' && data[end] != '}' && data[end] != ',' {
		end++
	}
	fragment = string(data[int(offset)-1 : end])
	return firstQuotedString(fragment)
}

func firstQuotedString(s string) string {
	start := strings.IndexByte(s, '"')
	if start == -1 {
		return ""
	}
	end := start + 1
	for end < len(s) {
		if s[end] == '"' && s[end-1] != '\\' {
			value, err := strconv.Unquote(s[start : end+1])
			if err == nil {
				return value
			}
			return ""
		}
		end++
	}
	return ""
}
