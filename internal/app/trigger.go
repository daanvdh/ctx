package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"ctx/internal/config"
	"ctx/internal/model"
	"ctx/internal/render"
	"ctx/internal/trigger"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// defaultMaxTriggerDepth bounds trigger chaining: a ctx write from inside a
// trigger script fires downstream triggers as usual, each nesting level
// incrementing CTX_TRIGGER_DEPTH, until the limit stops runaway loops.
const defaultMaxTriggerDepth = 5

// maxTriggerDepth returns the chain depth limit, overridable with
// max_trigger_depth in settings.yml for long agent loops.
func maxTriggerDepth() int {
	if s, err := config.LoadSettings(); err == nil && s.MaxTriggerDepth > 0 {
		return s.MaxTriggerDepth
	}
	return defaultMaxTriggerDepth
}

// triggerDepth returns the current trigger nesting depth from the environment.
func triggerDepth() int {
	n, _ := strconv.Atoi(os.Getenv("CTX_TRIGGER_DEPTH"))
	return n
}

// triggerEnv returns the base environment for a trigger script: the chain
// nesting depth (so ctx writes from the script keep firing triggers up to
// maxTriggerDepth), and CTX_ID pointing at the execution session.
func triggerEnv(depth int, executionSession string) []string {
	return append(os.Environ(),
		"CTX_TRIGGER_DEPTH="+strconv.Itoa(depth),
		"CTX_ID="+executionSession)
}

// TriggerChange and TriggerDefinition alias the trigger package's types so
// existing callers (cmd, webhooks) keep working under the app-level names.
type (
	TriggerChange     = trigger.Change
	TriggerDefinition = trigger.Definition
)

type triggerLog struct {
	Trigger          string `json:"trigger"`
	SessionID        string `json:"session_id"`
	Key              string `json:"key"`
	OldValue         string `json:"old_value"`
	NewValue         string `json:"new_value"`
	ExecutionSession string `json:"execution_session"`
	ExitCode         int    `json:"exit_code"`
	Stdout           string `json:"stdout"`
	Stderr           string `json:"stderr"`
	Error            string `json:"error,omitempty"`
}

// spawnTriggerRunner starts a detached ctx process that matches and runs
// triggers for change, so the writing command returns without waiting.
// The child is its own session leader and survives the parent exiting.
func spawnTriggerRunner(change TriggerChange) error {
	data, err := json.Marshal(change)
	if err != nil {
		return fmt.Errorf("encode trigger change: %w", err)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "fire-triggers", string(data))
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start background trigger runner: %w", err)
	}
	return cmd.Process.Release()
}

// ExecuteMatchingTriggers loads all trigger definitions and runs every one
// whose key pattern and conditions match the given change, honoring each
// definition's session scope, timeout and failure handling.
func (a *App) ExecuteMatchingTriggers(ctx context.Context, change TriggerChange) error {
	defs, err := trigger.LoadAll()
	if err != nil {
		return err
	}

	vars, err := a.store.Resolve(ctx, change.SessionID)
	if err != nil {
		return err
	}
	ancestors, err := a.ancestorSet(ctx, change.SessionID)
	if err != nil {
		return err
	}

	matching := []TriggerDefinition{}
	for _, def := range defs {
		matches, err := def.Matches(change, vars, ancestors)
		if err != nil {
			return err
		}
		if !matches {
			continue
		}
		matching = append(matching, def)
	}
	return a.runTriggers(ctx, matching, change)
}

func (a *App) runTriggers(ctx context.Context, matching []TriggerDefinition, change TriggerChange) error {
	sort.Slice(matching, func(i, j int) bool {
		if matching[i].Order != matching[j].Order {
			return matching[i].Order < matching[j].Order
		}
		return matching[i].Path < matching[j].Path
	})

	for i := 0; i < len(matching); {
		order := matching[i].Order
		j := i + 1
		for j < len(matching) && matching[j].Order == order {
			j++
		}
		if err := a.executeTriggerGroup(ctx, matching[i:j], change); err != nil {
			return err
		}
		i = j
	}
	return nil
}

func (a *App) executeTriggerGroup(ctx context.Context, defs []TriggerDefinition, change TriggerChange) error {
	if len(defs) == 1 {
		return a.executeTrigger(ctx, defs[0], change)
	}

	errCh := make(chan error, len(defs))
	var wg sync.WaitGroup
	for _, def := range defs {
		def := def
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- a.executeTrigger(ctx, def, change)
		}()
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			return err
		}
	}
	return nil
}

func (a *App) executeTrigger(ctx context.Context, def TriggerDefinition, change TriggerChange) error {
	vars, err := a.store.Resolve(ctx, change.SessionID)
	if err != nil {
		return err
	}
	if vars == nil {
		vars = map[string]string{}
	}
	// Built-in: the session that fired this trigger, so frontmatter like
	// "execution-session: $CTX_TRIGGER_SESSION" and prompts can name it
	// without a self-referencing ctx entry.
	vars["CTX_TRIGGER_SESSION"] = change.SessionID
	def, err = trigger.RenderDefinitionVars(def, vars)
	if err != nil {
		return a.writeTriggerLog(ctx, def, change, triggerLog{
			Trigger:   def.Name,
			SessionID: change.SessionID,
			Key:       change.Key,
			OldValue:  change.OldValue,
			NewValue:  change.NewValue,
			ExitCode:  -1,
			Error:     err.Error(),
		})
	}
	executionSession, err := a.executionSession(ctx, def, change)
	if err != nil {
		return err
	}
	triggerVars, err := trigger.RenderVars(def.PromptTemplate, vars)
	if err != nil {
		return a.writeTriggerLog(ctx, def, change, triggerLog{
			Trigger:          def.Name,
			SessionID:        change.SessionID,
			Key:              change.Key,
			OldValue:         change.OldValue,
			NewValue:         change.NewValue,
			ExecutionSession: executionSession,
			ExitCode:         -1,
			Error:            err.Error(),
		})
	}

	env := triggerEnv(change.Depth+1, executionSession)
	env = append(env, "CTX_TRIGGER_SESSION="+change.SessionID)
	for name, value := range triggerVars {
		env = append(env, name+"="+value)
	}
	runCtx := ctx
	if def.Timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, def.Timeout)
		defer cancel()
	}
	var outBuf, errBuf bytes.Buffer
	exitCode, runErr := runScript(runCtx, def.Script, vars, triggerVars, env, &outBuf, &errBuf)
	if runErr != nil && runCtx.Err() == context.DeadlineExceeded {
		runErr = fmt.Errorf("script timed out after %s", def.Timeout)
	}

	errText := ""
	if runErr != nil {
		errText = runErr.Error()
	}

	if def.OutputEntry != "" && runErr == nil {
		output := strings.TrimSpace(outBuf.String())
		if err := a.setEntryAtDepth(ctx, executionSession, def.OutputEntry,
			model.NewEntry(output, model.ValueTypeString), change.Depth+1); err != nil {
			errText = fmt.Sprintf("write output-entry %s: %v", def.OutputEntry, err)
		}
	}
	if def.FailureEntry != "" && runErr != nil {
		failure := fmt.Sprintf("trigger %s failed (exit %d): %s", def.Name, exitCode, runErr)
		if tail := tailString(strings.TrimSpace(errBuf.String()), 2000); tail != "" {
			failure += "\n" + tail
		}
		if err := a.setEntryAtDepth(ctx, executionSession, def.FailureEntry,
			model.NewEntry(failure, model.ValueTypeString), change.Depth+1); err != nil {
			errText = fmt.Sprintf("%s; write failure-entry %s: %v", errText, def.FailureEntry, err)
		}
	}

	return a.writeTriggerLog(ctx, def, change, triggerLog{
		Trigger:          def.Name,
		SessionID:        change.SessionID,
		Key:              change.Key,
		OldValue:         change.OldValue,
		NewValue:         change.NewValue,
		ExecutionSession: executionSession,
		ExitCode:         exitCode,
		Stdout:           outBuf.String(),
		Stderr:           errBuf.String(),
		Error:            errText,
	})
}

// runScript executes command as a single POSIX shell script via mvdan/sh.
// Every unique $VAR_NAME placeholder that names a known ctx value is bound
// once as an opaque positional parameter ($1..$N); the script text only ever
// sees $1, $2, etc. in their place. $NAME references that aren't known ctx
// values (e.g. a variable the script assigns itself, like STORY_ID=$(...))
// are left untouched for the shell to resolve natively. Names present in
// triggerVars are always skipped here and left for the shell to resolve
// from env instead, so a trigger-defined variable (including
// CTX_TRIGGER_PROMPT) takes precedence over a ctx session value of the same
// name, and reaches the script only if it's referenced explicitly -- nothing
// is auto-appended.
// Output is written to stdout and stderr writers.
func runScript(ctx context.Context, command string, vars map[string]string, triggerVars map[string]string, env []string, stdout, stderr io.Writer) (exitCode int, err error) {
	indices := make(map[string]int)
	args := make([]string, 0)
	for _, name := range render.ExtractVarNames(command) {
		if _, isTriggerVar := triggerVars[name]; isTriggerVar {
			continue
		}
		value, ok := vars[name]
		if !ok {
			continue
		}
		indices[name] = len(args) + 1
		args = append(args, value)
	}

	script := render.RewriteVars(command, indices)

	file, err := syntax.NewParser().Parse(strings.NewReader(script), "")
	if err != nil {
		return -1, fmt.Errorf("invalid command: %w", err)
	}

	runner, err := interp.New(
		interp.Params(args...),
		interp.Env(expand.ListEnviron(env...)),
		interp.StdIO(nil, stdout, stderr),
	)
	if err != nil {
		return -1, err
	}

	if runErr := runner.Run(ctx, file); runErr != nil {
		var status interp.ExitStatus
		if errors.As(runErr, &status) {
			return int(status), runErr
		}
		return 1, runErr
	}

	return 0, nil
}

// ancestorSet returns the IDs of sessionID's ancestors, not including sessionID itself.
func (a *App) ancestorSet(ctx context.Context, sessionID string) (map[string]bool, error) {
	nodes, err := a.store.SessionNodes(ctx)
	if err != nil {
		return nil, err
	}
	parents := make(map[string]string, len(nodes))
	for _, n := range nodes {
		if n.Parent != nil {
			parents[n.ID] = *n.Parent
		}
	}

	ancestors := make(map[string]bool)
	currentID := sessionID
	for hops := 0; hops < 50; hops++ {
		parent, ok := parents[currentID]
		if !ok || parent == "" || ancestors[parent] {
			break
		}
		ancestors[parent] = true
		currentID = parent
	}
	return ancestors, nil
}

func (a *App) executionSession(ctx context.Context, def TriggerDefinition, change TriggerChange) (string, error) {
	if def.ExecutionSession != "" {
		return def.ExecutionSession, nil
	}
	parent := change.SessionID
	id, err := a.CreateSession(ctx, "", &parent, false)
	if err != nil {
		return "", err
	}
	return id, nil
}

// writeTriggerLog records a trigger run as a trigger_log_* entry in the
// triggering session, but only when the trigger opts in with "logging: true".
// With logging off, failures are still reported on stderr so they don't
// vanish silently.
func (a *App) writeTriggerLog(ctx context.Context, def TriggerDefinition, change TriggerChange, log triggerLog) error {
	if !def.Logging {
		if log.Error != "" {
			a.logger.Error("trigger failed", "trigger", log.Trigger, "error", log.Error)
		}
		return nil
	}
	data, err := json.Marshal(log)
	if err != nil {
		return fmt.Errorf("encode trigger log: %w", err)
	}
	now := time.Now()
	timestamp := now.Format("060102150405") + fmt.Sprintf("%02d", now.Nanosecond()/1e7)
	key := fmt.Sprintf("trigger_log_%s_%s", shellSafeIdentifier(log.Trigger), timestamp)
	return a.store.SetValue(ctx, change.SessionID, key, string(data))
}

// tailString returns the last max bytes of s (from a rune-safe boundary is
// not needed for log tails; byte cut is fine for error context).
func tailString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[len(s)-max:]
}

// shellSafeIdentifier rewrites s so it only contains characters valid in a
// shell variable name, replacing anything else (including a leading digit)
// with an underscore.
func shellSafeIdentifier(s string) string {
	b := make([]rune, 0, len(s))
	for i, r := range s {
		switch {
		case r == '_' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z'):
			b = append(b, r)
		case r >= '0' && r <= '9' && i > 0:
			b = append(b, r)
		default:
			b = append(b, '_')
		}
	}
	return string(b)
}
