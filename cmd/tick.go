package cmd

import (
	"context"
	"fmt"
	"time"
)

// Tick handles the `ctx tick` command: it runs every trigger whose
// schedule matches the current time. It is meant to be invoked
// periodically by the OS scheduler (e.g. a crontab entry).
func Tick(ctx context.Context, args []string) error {
	if helpRequested(args) {
		fmt.Println(`Usage: ctx tick

Run every trigger whose schedule (a 5-field cron expression: minute
hour day-of-month month day-of-week) matches the current time,
provided its other filters (ancestor/entries) still match. Intended
to be invoked periodically by the OS scheduler, e.g. a crontab entry:
    * * * * * ctx tick`)
		return nil
	}
	if len(args) != 0 {
		return usage("tick", "ctx tick")
	}

	a, err := newApp()
	if err != nil {
		return err
	}
	return a.RunScheduledTriggers(ctx, time.Now())
}
