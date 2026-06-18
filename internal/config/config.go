package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Settings struct {
	DBPath          string `json:"db_path"`
	TriggerLocation string `json:"trigger_location"`
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

func loadSettings() (Settings, string, error) {
	cfgDir, err := Dir()
	if err != nil {
		return Settings{}, "", err
	}
	settingsPath := filepath.Join(cfgDir, "settings.json")
	var settings Settings

	if data, err := os.ReadFile(settingsPath); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return Settings{}, "", fmt.Errorf("failed to parse settings file %s: %w", settingsPath, err)
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
	if err := os.WriteFile(settingsPath, data, 0o644); err != nil {
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
