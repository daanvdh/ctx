package cmd

import (
	"fmt"
	"os"
	"path/filepath"
)

func getCtxPath() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}
	return filepath.Join(wd, "ctx.json"), nil
}
