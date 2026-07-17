package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"ctx/internal/config"
	"ctx/internal/model"
	"ctx/internal/render"
	"gopkg.in/yaml.v3"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// maxTriggerDepth bounds trigger chaining: a ctx write from inside a trigger
// script fires downstream triggers as usual, each nesting level incrementing
// CTX_TRIGGER_DEPTH, until this limit stops runaway loops.
const maxTriggerDepth = 5

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

type TriggerChange struct {
	SessionID string
	Key       string
	OldValue  string
	NewValue  string
	// Depth is the trigger chain nesting level of the write that caused
	// this change; writes made by a fired trigger carry Depth+1.
	Depth int
}

// TriggerDefinition holds a parsed trigger file.
type TriggerDefinition struct {
	Name             string
	Path             string
	TriggerSession   string
	Ancestor         string
	Entries          map[string][]string // key -> accepted values; nil/empty slice = wildcard
	AnyChange        bool
	Order            int
	ExecutionSession string
	Script           string
	PromptTemplate   string
	Schedule         string
	Logging          bool
	OutputEntry      string
	Timeout          time.Duration // zero = unbounded
}

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

// triggerEntryValue is a single accepted value within an entries list.
type triggerEntryValue struct {
	Value string `yaml:"value"`
}

// triggerFileData is the YAML structure for the trigger frontmatter.
type triggerFileData struct {
	Script           string                         `yaml:"script"`
	TriggerSession   string                         `yaml:"trigger-session"`
	Ancestor         string                         `yaml:"ancestor"`
	AnyChange        bool                           `yaml:"any-change"`
	Order            int                            `yaml:"order"`
	ExecutionSession string                         `yaml:"execution-session"`
	Entries          map[string][]triggerEntryValue `yaml:"entries"`
	Schedule         string                         `yaml:"schedule"`
	Logging          bool                           `yaml:"logging"`
	OutputEntry      string                         `yaml:"output-entry"`
	Timeout          string                         `yaml:"timeout"`
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

func (a *App) ExecuteMatchingTriggers(ctx context.Context, change TriggerChange) error {
	defs, err := loadTriggerDefinitions()
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

func loadTriggerDefinitions() ([]TriggerDefinition, error) {
	triggerDir, err := config.TriggerDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(triggerDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read trigger directory %s: %w", triggerDir, err)
	}

	defs := make([]TriggerDefinition, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(triggerDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read trigger %s: %w", path, err)
		}
		def, err := parseTriggerDefinition(path, string(data))
		if err != nil {
			return nil, err
		}
		defs = append(defs, def)
	}
	sort.Slice(defs, func(i, j int) bool {
		return defs[i].Path < defs[j].Path
	})
	return defs, nil
}

func parseTriggerDefinition(path, content string) (TriggerDefinition, error) {
	var frontmatter, promptTemplate string
	if i := strings.Index(content, "\n---\n"); i >= 0 {
		frontmatter = content[:i]
		promptTemplate = content[i+5:]
	} else {
		frontmatter = content
	}

	var data triggerFileData
	dec := yaml.NewDecoder(strings.NewReader(frontmatter))
	dec.KnownFields(true)
	if err := dec.Decode(&data); err != nil && err != io.EOF {
		return TriggerDefinition{}, fmt.Errorf("malformed trigger %s: %w", path, err)
	}

	if data.Script == "" {
		return TriggerDefinition{}, fmt.Errorf("malformed trigger %s: missing script", path)
	}
	if data.AnyChange && (data.TriggerSession != "" || data.Ancestor != "" || len(data.Entries) > 0) {
		return TriggerDefinition{}, fmt.Errorf("malformed trigger %s: any-change cannot be combined with trigger-session, ancestor, or entries", path)
	}
	hasMatchers := data.TriggerSession != "" || data.Ancestor != "" || len(data.Entries) > 0
	if data.Schedule != "" && data.AnyChange {
		return TriggerDefinition{}, fmt.Errorf("malformed trigger %s: schedule cannot be combined with any-change", path)
	}
	if data.Schedule != "" && !hasMatchers && data.ExecutionSession == "" {
		return TriggerDefinition{}, fmt.Errorf("malformed trigger %s: schedule without filters requires execution-session to be set", path)
	}
	if data.Schedule != "" && !hasMatchers && strings.Contains(data.ExecutionSession, "$") {
		return TriggerDefinition{}, fmt.Errorf("malformed trigger %s: a schedule-driven trigger without filters has no triggering session to resolve $VAR from; execution-session must be literal", path)
	}

	var timeout time.Duration
	if data.Timeout != "" {
		d, err := time.ParseDuration(data.Timeout)
		if err != nil || d <= 0 {
			return TriggerDefinition{}, fmt.Errorf("malformed trigger %s: invalid timeout %q (want a positive Go duration like 10m)", path, data.Timeout)
		}
		timeout = d
	}

	entries := make(map[string][]string, len(data.Entries))
	for key, vals := range data.Entries {
		values := make([]string, 0, len(vals))
		for _, v := range vals {
			values = append(values, v.Value)
		}
		entries[key] = values
	}

	return TriggerDefinition{
		Name:             strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
		Path:             path,
		TriggerSession:   data.TriggerSession,
		Ancestor:         data.Ancestor,
		Entries:          entries,
		AnyChange:        data.AnyChange,
		Order:            data.Order,
		ExecutionSession: data.ExecutionSession,
		Script:           data.Script,
		PromptTemplate:   promptTemplate,
		Schedule:         data.Schedule,
		Logging:          data.Logging,
		OutputEntry:      data.OutputEntry,
		Timeout:          timeout,
	}, nil
}

// triggerVarMarkerRe matches a "<!-- ctx:var NAME -->" block marker.
var triggerVarMarkerRe = regexp.MustCompile(`<!--\s*ctx:var\s+([A-Za-z_][A-Za-z0-9_]*)\s*-->`)

// parseTriggerVars splits a trigger body into named variable blocks,
// delimited by "<!-- ctx:var NAME -->" markers: each marker starts a new
// block running until the next marker (or EOF), with surrounding blank
// lines trimmed. Content before the first marker (or the whole body, if
// there are no markers) is assigned to CTX_TRIGGER_PROMPT.
func parseTriggerVars(body string) map[string]string {
	matches := triggerVarMarkerRe.FindAllStringSubmatchIndex(body, -1)
	if len(matches) == 0 {
		return map[string]string{"CTX_TRIGGER_PROMPT": strings.TrimSpace(body)}
	}

	vars := map[string]string{"CTX_TRIGGER_PROMPT": strings.TrimSpace(body[:matches[0][0]])}
	for i, m := range matches {
		name := body[m[2]:m[3]]
		end := len(body)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		vars[name] = strings.TrimSpace(body[m[1]:end])
	}
	return vars
}

// renderTriggerVars parses body into named variable blocks (see
// parseTriggerVars) and renders each block's $VAR placeholders against vars.
func renderTriggerVars(body string, vars map[string]string) (map[string]string, error) {
	blocks := parseTriggerVars(body)
	rendered := make(map[string]string, len(blocks))
	for name, raw := range blocks {
		value, err := render.TemplateString(raw, vars)
		if err != nil {
			return nil, err
		}
		rendered[name] = value
	}
	return rendered, nil
}

// Matches reports whether this trigger should fire for the given change.
// vars contains the fully resolved current values for the triggering session.
// ancestors contains the IDs of the triggering session's ancestors (not including itself).
func (d TriggerDefinition) Matches(change TriggerChange, vars map[string]string, ancestors map[string]bool) (bool, error) {
	if d.Schedule != "" {
		return false, nil // schedule-driven: fires on ticks, never on writes
	}
	if d.AnyChange {
		return true, nil
	}
	if d.TriggerSession == "" && d.Ancestor == "" && len(d.Entries) == 0 {
		return false, nil // manual only
	}
	if d.TriggerSession != "" && d.TriggerSession != change.SessionID {
		return false, nil
	}
	if d.Ancestor != "" && !ancestors[d.Ancestor] {
		return false, nil
	}
	if len(d.Entries) == 0 {
		return true, nil // session matched, no entry filter
	}

	// The changed key must be one of our entry keys.
	if _, ok := d.Entries[change.Key]; !ok {
		return false, nil
	}

	// All entries must have a matching current value (wildcard if no values specified).
	for key, values := range d.Entries {
		if len(values) == 0 {
			continue // wildcard: any value matches
		}
		currentValue := vars[key]
		if key == change.Key {
			currentValue = change.NewValue
		}
		matched := false
		for _, v := range values {
			if currentValue == v {
				matched = true
				break
			}
		}
		if !matched {
			return false, nil
		}
	}

	return true, nil
}

// hasMatchers reports whether any session/entry filter is set.
func (d TriggerDefinition) hasMatchers() bool {
	return d.TriggerSession != "" || d.Ancestor != "" || len(d.Entries) > 0
}

// MatchesState reports whether a session's current state satisfies this
// trigger's filters, with no write involved — the schedule-tick counterpart
// of Matches. An entries key with no values requires the key to be visible;
// with values, the current value must equal one of them.
func (d TriggerDefinition) MatchesState(sessionID string, vars map[string]string, ancestors map[string]bool) bool {
	if d.TriggerSession != "" && d.TriggerSession != sessionID {
		return false
	}
	if d.Ancestor != "" && !ancestors[d.Ancestor] {
		return false
	}
	for key, values := range d.Entries {
		current, ok := vars[key]
		if !ok {
			return false
		}
		if len(values) == 0 {
			continue
		}
		matched := false
		for _, v := range values {
			if current == v {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

// matchesSchedule reports whether t falls within expr, a standard 5-field
// cron expression (minute hour day-of-month month day-of-week), the same
// format used by crontab(5), Kubernetes CronJob and GitHub Actions.
//
// ponytail: supports "*", exact values, comma-lists and "*/step"; no
// hyphen ranges or named months/weekdays. Add if a trigger needs them.
func matchesSchedule(expr string, t time.Time) (bool, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return false, fmt.Errorf("schedule %q: want 5 fields (minute hour dom month dow), got %d", expr, len(fields))
	}
	values := [5]int{t.Minute(), t.Hour(), t.Day(), int(t.Month()), int(t.Weekday())}
	maxes := [5]int{59, 23, 31, 12, 6}
	for i, field := range fields {
		ok, err := matchesCronField(field, values[i], maxes[i])
		if err != nil {
			return false, fmt.Errorf("schedule %q: %w", expr, err)
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

func matchesCronField(field string, value, max int) (bool, error) {
	for _, part := range strings.Split(field, ",") {
		if part == "*" {
			return true, nil
		}
		if step, ok := strings.CutPrefix(part, "*/"); ok {
			n, err := strconv.Atoi(step)
			if err != nil || n <= 0 {
				return false, fmt.Errorf("invalid step %q", part)
			}
			if value%n == 0 {
				return true, nil
			}
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 || n > max {
			return false, fmt.Errorf("invalid value %q (want 0-%d)", part, max)
		}
		if n == value {
			return true, nil
		}
	}
	return false, nil
}

// renderDefinitionVars renders $VAR placeholders in the frontmatter fields
// that name runtime targets (execution-session, output-entry) against the
// triggering session's resolved values. Matcher fields stay literal.
func renderDefinitionVars(def TriggerDefinition, vars map[string]string) (TriggerDefinition, error) {
	for _, field := range []*string{&def.ExecutionSession, &def.OutputEntry} {
		if !strings.Contains(*field, "$") {
			continue
		}
		rendered, err := render.TemplateString(*field, vars)
		if err != nil {
			return def, fmt.Errorf("trigger %s frontmatter: %w", def.Name, err)
		}
		*field = rendered
	}
	return def, nil
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
	def, err = renderDefinitionVars(def, vars)
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
	triggerVars, err := renderTriggerVars(def.PromptTemplate, vars)
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
		if code, ok := interp.IsExitStatus(runErr); ok {
			return int(code), runErr
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
			fmt.Fprintf(a.stderr, "ctx: trigger %s: %s\n", log.Trigger, log.Error)
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
