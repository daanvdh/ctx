package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"ctx/internal/app"
)

// FireTriggers is the internal entry point for background trigger execution:
// `ctx fire-triggers <json-change>`. A writing ctx process spawns it detached
// so the write returns immediately while triggers match and run here.
func FireTriggers(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return usage("fire-triggers", "ctx fire-triggers <json-change> (internal)")
	}
	var change app.TriggerChange
	if err := json.Unmarshal([]byte(args[0]), &change); err != nil {
		return fmt.Errorf("invalid trigger change: %w", err)
	}
	a, err := newApp()
	if err != nil {
		return err
	}
	return a.ExecuteMatchingTriggers(ctx, change)
}
