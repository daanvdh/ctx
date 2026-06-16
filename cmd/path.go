package cmd

import (
	"fmt"
	"os"
	"path/filepath"
)

func getCtxPath() (string, error) {
    // First check if an environment variable overrides the location.
    // CTX_DB_PATH can be set to any filesystem path that the user wishes to store
    // the SQLite database in. If not set, we default to $HOME/.config/ctx/ctx.sqlite.
    if custom := os.Getenv("CTX_DB_PATH"); custom != "" {
        return custom, nil
    }

    home, err := os.UserHomeDir()
    if err != nil {
        return "", fmt.Errorf("failed to get user home directory: %w", err)
    }
    dir := filepath.Join(home, ".config", "ctx")
    // Ensure the config directory exists.
    if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
        return "", fmt.Errorf("failed to create config directory %s: %w", dir, mkErr)
    }
    return filepath.Join(dir, "ctx.sqlite"), nil
}
