package app

import (
	"context"
	"time"

	"ctx/internal/trigger"
)

const schedulerInterval = 30 * time.Second

// maxScheduleRunAge bounds how long a trigger's "running" marker is honored
// before it's treated as stale and cleared for reclaiming. It guards against
// a crashed ctx serve process wedging a trigger's schedule forever; it's
// deliberately generous since it should only ever kick in after a crash.
const maxScheduleRunAge = 24 * time.Hour

// scheduleClaimer atomically claims a due schedule instant for a trigger, so
// two ctx serve processes sharing one database can't both fire the same
// trigger for the same due minute, and tracks whether a claimed trigger's
// run is still in progress, so a new due minute doesn't start a second,
// overlapping run while the previous one hasn't finished. Only *store.SQLite
// implements it today; a.store not implementing it (e.g. a remote
// MCP-backed store) means scheduling is unavailable.
type scheduleClaimer interface {
	ClaimTriggerSchedule(ctx context.Context, triggerPath string, dueAt time.Time) (bool, error)
	MarkTriggerRunning(ctx context.Context, triggerPath string, now time.Time, staleAfter time.Duration) (bool, error)
	MarkTriggerFinished(ctx context.Context, triggerPath string) error
}

// RunScheduler polls schedule-bearing triggers every schedulerInterval and
// fires each due one at most once per matching cron minute. It blocks until
// ctx is done. If the app's store doesn't support atomic claims, scheduling
// is unavailable; RunScheduler logs that and blocks without polling.
func (a *App) RunScheduler(ctx context.Context) error {
	claimer, ok := a.store.(scheduleClaimer)
	if !ok {
		a.logger.Warn("serve: scheduler disabled (requires local sqlite store)")
		<-ctx.Done()
		return ctx.Err()
	}

	ticker := time.NewTicker(schedulerInterval)
	defer ticker.Stop()
	for {
		if err := a.runDueSchedules(ctx, claimer, time.Now()); err != nil {
			a.logger.Error("serve: scheduler tick failed", "error", err)
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
// claimed for now's minute (via claimer), skipping it (with a warning) if
// its previous run hasn't finished yet. Firing is sequential: schedules are
// minute-resolution and ticks run every 30s, so one trigger's script briefly
// delaying the next due trigger in the same tick is an acceptable trade for
// keeping this straightforward to test and reason about.
func (a *App) runDueSchedules(ctx context.Context, claimer scheduleClaimer, now time.Time) error {
	defs, err := trigger.LoadAll()
	if err != nil {
		return err
	}

	dueAt := now.Truncate(time.Minute)
	for _, def := range defs {
		if def.Schedule == "" {
			continue
		}
		due, err := trigger.MatchesSchedule(def.Schedule, now)
		if err != nil {
			a.logger.Error("serve: trigger failed", "trigger", def.Name, "error", err)
			continue
		}
		if !due {
			continue
		}

		claimed, err := claimer.ClaimTriggerSchedule(ctx, def.Path, dueAt)
		if err != nil {
			a.logger.Error("serve: claim schedule failed", "trigger", def.Name, "error", err)
			continue
		}
		if !claimed {
			continue
		}

		started, err := claimer.MarkTriggerRunning(ctx, def.Path, now, maxScheduleRunAge)
		if err != nil {
			a.logger.Error("serve: mark trigger running failed", "trigger", def.Name, "error", err)
			continue
		}
		if !started {
			a.logger.Warn("serve: skipping trigger, previous run still in progress", "trigger", def.Name)
			continue
		}

		a.runDueSchedule(ctx, claimer, def)
	}
	return nil
}

// runDueSchedule fires def for each of its schedule targets and clears its
// running marker afterward, however it finishes, so its next due run isn't
// skipped as still-running.
func (a *App) runDueSchedule(ctx context.Context, claimer scheduleClaimer, def TriggerDefinition) {
	defer func() {
		if err := claimer.MarkTriggerFinished(ctx, def.Path); err != nil {
			a.logger.Error("serve: mark trigger finished failed", "trigger", def.Name, "error", err)
		}
	}()

	for _, sessionID := range a.scheduleTargets(ctx, def) {
		change := TriggerChange{SessionID: sessionID}
		if err := a.runTriggers(ctx, []TriggerDefinition{def}, change); err != nil {
			a.logger.Error("serve: trigger failed", "trigger", def.Name, "error", err)
		}
	}
}

// scheduleTargets returns the sessions a due schedule trigger fires for.
// Without filters it fires once, for its execution-session. With filters
// (trigger-session, ancestor, entries) it fires once per session whose
// current state satisfies them, so one cron trigger can serve every
// matching session.
func (a *App) scheduleTargets(ctx context.Context, def TriggerDefinition) []string {
	if !def.HasMatchers() {
		return []string{def.ExecutionSession}
	}
	nodes, err := a.store.SessionNodes(ctx)
	if err != nil {
		a.logger.Error("serve: list sessions failed", "trigger", def.Name, "error", err)
		return nil
	}
	var targets []string
	for _, node := range nodes {
		vars, err := a.store.Resolve(ctx, node.ID)
		if err != nil {
			a.logger.Error("serve: resolve session failed", "trigger", def.Name, "session", node.ID, "error", err)
			continue
		}
		ancestors, err := a.ancestorSet(ctx, node.ID)
		if err != nil {
			a.logger.Error("serve: resolve ancestors failed", "trigger", def.Name, "session", node.ID, "error", err)
			continue
		}
		if def.MatchesState(node.ID, vars, ancestors) {
			targets = append(targets, node.ID)
		}
	}
	return targets
}
