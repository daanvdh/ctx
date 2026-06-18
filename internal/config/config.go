package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Settings struct {
	DBPath string `json:"db_path"`
}

func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	cfgDir := filepath.Join(home, ".config", "ctx")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create config directory %s: %w", cfgDir, err)
	}

	return cfgDir, nil
}

func DBPath() (string, error) {
	cfgDir, err := Dir()
	if err != nil {
		return "", err
	}

	settingsPath := filepath.Join(cfgDir, "settings.json")
	var settings Settings

	if data, err := os.ReadFile(settingsPath); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return "", fmt.Errorf("failed to parse settings file %s: %w", settingsPath, err)
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("failed to read settings file %s: %w", settingsPath, err)
	}

	if settings.DBPath == "" {
		settings.DBPath = filepath.Join(cfgDir, "ctx.sqlite")
		data, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			return "", fmt.Errorf("failed to encode default settings: %w", err)
		}
		if err := os.WriteFile(settingsPath, data, 0o644); err != nil {
			return "", fmt.Errorf("failed to write settings file %s: %w", settingsPath, err)
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

func TriggerPath(name string) (string, error) {
	cfgDir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfgDir, "triggers", name), nil
}
