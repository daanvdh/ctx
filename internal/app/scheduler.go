package app

import (
	"context"
	"fmt"
	"time"
)

const schedulerInterval = 30 * time.Second

// scheduleClaimer atomically claims a due schedule instant for a trigger, so
// two ctx serve processes sharing one database can't both fire the same
// trigger for the same due minute. Only *store.SQLite implements it today;
// a.store not implementing it (e.g. a remote MCP-backed store) means
// scheduling is unavailable.
type scheduleClaimer interface {
	ClaimTriggerSchedule(ctx context.Context, triggerPath string, dueAt time.Time) (bool, error)
}

// RunScheduler polls schedule-bearing triggers every schedulerInterval and
// fires each due one at most once per matching cron minute. It blocks until
// ctx is done. If the app's store doesn't support atomic claims, scheduling
// is unavailable; RunScheduler logs that and blocks without polling.
func (a *App) RunScheduler(ctx context.Context) error {
	claimer, ok := a.store.(scheduleClaimer)
	if !ok {
		fmt.Fprintln(a.stderr, "ctx: serve: scheduler disabled (requires local sqlite store)")
		<-ctx.Done()
		return ctx.Err()
	}

	ticker := time.NewTicker(schedulerInterval)
	defer ticker.Stop()
	for {
		if err := a.runDueSchedules(ctx, claimer, time.Now()); err != nil {
			fmt.Fprintf(a.stderr, "ctx: serve: scheduler tick: %v\n", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// runDueSchedules checks every schedule-bearing trigger against now and
// fires each one whose cron expression matches and hasn't already been
// claimed for now's minute (via claimer). Firing is sequential: schedules
// are minute-resolution and ticks run every 30s, so one trigger's script
// briefly delaying the next due trigger in the same tick is an acceptable
// trade for keeping this straightforward to test and reason about.
func (a *App) runDueSchedules(ctx context.Context, claimer scheduleClaimer, now time.Time) error {
	defs, err := loadTriggerDefinitions()
	if err != nil {
		return err
	}

	dueAt := now.Truncate(time.Minute)
	for _, def := range defs {
		if def.Schedule == "" {
			continue
		}
		due, err := matchesSchedule(def.Schedule, now)
		if err != nil {
			fmt.Fprintf(a.stderr, "ctx: serve: trigger %s: %v\n", def.Name, err)
			continue
		}
		if !due {
			continue
		}

		claimed, err := claimer.ClaimTriggerSchedule(ctx, def.Path, dueAt)
		if err != nil {
			fmt.Fprintf(a.stderr, "ctx: serve: trigger %s: claim schedule: %v\n", def.Name, err)
			continue
		}
		if !claimed {
			continue
		}

		change := TriggerChange{SessionID: def.ExecutionSession}
		if err := a.runTriggers(ctx, []TriggerDefinition{def}, change); err != nil {
			fmt.Fprintf(a.stderr, "ctx: serve: trigger %s: %v\n", def.Name, err)
		}
	}
	return nil
}
