package cmd

import (
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
)

// getCtxPath returns the filesystem path to the SQLite database used by ctx.
//
// The location is configured via a JSON settings file stored in the user's
// configuration directory ("$HOME/.config/ctx/settings.json"). Currently the
// only supported setting is "db_path", which specifies where the SQLite file
// resides. If the settings file does not exist, or the field is empty, we fall back
// to the default location "$HOME/.config/ctx/ctx.sqlite" and create a new
// settings.json containing the default.
//
// The settings format was chosen as JSON because it has native support in the Go
// standard library and requires no additional dependencies.
func getCtxPath() (string, error) {
    // Determine config directory ($HOME/.config/ctx) and ensure it exists.
    home, err := os.UserHomeDir()
    if err != nil {
        return "", fmt.Errorf("failed to get user home directory: %w", err)
    }
    cfgDir := filepath.Join(home, ".config", "ctx")
    if mkErr := os.MkdirAll(cfgDir, 0o755); mkErr != nil {
        return "", fmt.Errorf("failed to create config directory %s: %w", cfgDir, mkErr)
    }

    // Load (or initialise) the settings file.
    settingsPath := filepath.Join(cfgDir, "settings.json")
    type Settings struct {
        DBPath string `json:"db_path"`
    }
    var s Settings

    if data, err := os.ReadFile(settingsPath); err == nil {
        // If we successfully read a file, try to unmarshal it.
        _ = json.Unmarshal(data, &s) // ignore errors – defaults will be used on failure
    } else if !os.IsNotExist(err) {
        return "", fmt.Errorf("failed to read settings file %s: %w", settingsPath, err)
    }

    // Determine the DB path.
    if s.DBPath == "" {
        s.DBPath = filepath.Join(cfgDir, "ctx.sqlite")
        // Persist default settings for future runs. Errors are ignored; they will be
        // surfaced later when trying to create the directory or file.
        if data, marshalErr := json.MarshalIndent(s, "", "  "); marshalErr == nil {
            _ = os.WriteFile(settingsPath, data, 0o644)
        }
    }

    // Resolve relative paths against the config directory.
    if !filepath.IsAbs(s.DBPath) {
        s.DBPath = filepath.Join(cfgDir, s.DBPath)
    }

    // Ensure that the parent directory for the DB file exists.
    if mkErr := os.MkdirAll(filepath.Dir(s.DBPath), 0o755); mkErr != nil {
        return "", fmt.Errorf("failed to create directory %s for db path: %w", filepath.Dir(s.DBPath), mkErr)
    }

    return s.DBPath, nil
}
