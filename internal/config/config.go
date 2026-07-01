package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Settings struct {
	DBPath               string   `yaml:"db_path" json:"db_path"`
	TriggerLocation      string   `yaml:"trigger_location" json:"trigger_location"`
	MCPHTTPAddr          string   `yaml:"mcp_http_addr" json:"mcp_http_addr"`
	MCPHTTPPath          string   `yaml:"mcp_http_path" json:"mcp_http_path"`
	MCPServerName        string   `yaml:"mcp_server_name" json:"mcp_server_name"`
	MCPAllowedOrigins    []string `yaml:"mcp_allowed_origins" json:"mcp_allowed_origins"`
	MCPToken             string   `yaml:"mcp_token" json:"mcp_token"`
	MCPOAuthClientID     string   `yaml:"mcp_oauth_client_id" json:"mcp_oauth_client_id"`
	MCPOAuthClientSecret string   `yaml:"mcp_oauth_client_secret" json:"mcp_oauth_client_secret"`
	MCPPublicURL         string   `yaml:"mcp_public_url" json:"mcp_public_url"`
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
	settingsPath := filepath.Join(cfgDir, "settings.yml")
	var settings Settings

	if data, err := os.ReadFile(settingsPath); err == nil {
		if err := yaml.Unmarshal(data, &settings); err != nil {
			return Settings{}, "", fmt.Errorf("failed to parse settings file %s: %w", settingsPath, err)
		}
		return settings, cfgDir, nil
	} else if !os.IsNotExist(err) {
		return Settings{}, "", fmt.Errorf("failed to read settings file %s: %w", settingsPath, err)
	}

	settings, migrated, err := migrateLegacySettings(cfgDir)
	if err != nil {
		return Settings{}, "", err
	}
	if migrated {
		if err := writeSettings(cfgDir, settings); err != nil {
			return Settings{}, "", err
		}
	}

	return settings, cfgDir, nil
}

// migrateLegacySettings reads a pre-existing settings.json file (from before ctx switched to
// YAML) so upgrading users keep their configuration instead of silently losing it.
func migrateLegacySettings(cfgDir string) (Settings, bool, error) {
	legacyPath := filepath.Join(cfgDir, "settings.json")
	data, err := os.ReadFile(legacyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return Settings{}, false, nil
		}
		return Settings{}, false, fmt.Errorf("failed to read legacy settings file %s: %w", legacyPath, err)
	}

	var settings Settings
	if err := json.Unmarshal(data, &settings); err != nil {
		return Settings{}, false, fmt.Errorf("failed to parse legacy settings file %s: %w", legacyPath, err)
	}
	return settings, true, nil
}

func writeSettings(cfgDir string, settings Settings) error {
	settingsPath := filepath.Join(cfgDir, "settings.yml")
	data, err := yaml.Marshal(settings)
	if err != nil {
		return fmt.Errorf("failed to encode settings: %w", err)
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
