package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"ctx/internal/config"
	"ctx/internal/render"
	"gopkg.in/yaml.v3"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

type TriggerChange struct {
	SessionID string
	Key       string
	OldValue  string
	NewValue  string
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

// RunScheduledTriggers executes every trigger whose schedule matches now,
// provided its other filters (ancestor/entries) still hold against the
// trigger's own session's current values. Intended to be invoked
// periodically (e.g. from an OS crontab entry running `ctx tick`).
func (a *App) RunScheduledTriggers(ctx context.Context, now time.Time) error {
	defs, err := loadTriggerDefinitions()
	if err != nil {
		return err
	}

	bySession := map[string][]TriggerDefinition{}
	for _, def := range defs {
		if def.Schedule == "" {
			continue
		}
		due, err := matchesSchedule(def.Schedule, now)
		if err != nil {
			return fmt.Errorf("trigger %s: %w", def.Name, err)
		}
		if !due {
			continue
		}

		vars, err := a.store.Resolve(ctx, def.TriggerSession)
		if err != nil {
			return err
		}
		ancestors, err := a.ancestorSet(ctx, def.TriggerSession)
		if err != nil {
			return err
		}
		if !def.matchesScheduleFilters(vars, ancestors) {
			continue
		}
		bySession[def.TriggerSession] = append(bySession[def.TriggerSession], def)
	}

	for sessionID, matching := range bySession {
		if err := a.runTriggers(ctx, matching, TriggerChange{SessionID: sessionID}); err != nil {
			return err
		}
	}
	return nil
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
	if data.Schedule != "" && data.TriggerSession == "" {
		return TriggerDefinition{}, fmt.Errorf("malformed trigger %s: schedule requires trigger-session to be set", path)
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

// matchesScheduleFilters reports whether a schedule-driven fire should
// proceed, checking the trigger's ancestor/entries filters against the
// current resolved values of its own session (there is no "changed key"
// for a schedule tick, so entries are matched by current value only,
// unlike Matches).
func (d TriggerDefinition) matchesScheduleFilters(vars map[string]string, ancestors map[string]bool) bool {
	if d.Ancestor != "" && !ancestors[d.Ancestor] {
		return false
	}
	for key, values := range d.Entries {
		if len(values) == 0 {
			continue // wildcard
		}
		matched := false
		for _, v := range values {
			if vars[key] == v {
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

func (a *App) executeTrigger(ctx context.Context, def TriggerDefinition, change TriggerChange) error {
	executionSession, err := a.executionSession(ctx, def, change)
	if err != nil {
		return err
	}
	vars, err := a.store.Resolve(ctx, change.SessionID)
	if err != nil {
		return err
	}
	triggerVars, err := renderTriggerVars(def.PromptTemplate, vars)
	if err != nil {
		return a.writeTriggerLog(ctx, change, triggerLog{
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

	env := append(os.Environ(), "CTX_SUPPRESS_TRIGGERS=1", "CTX_ID="+executionSession)
	for name, value := range triggerVars {
		env = append(env, name+"="+value)
	}
	var outBuf, errBuf bytes.Buffer
	exitCode, runErr := runScript(ctx, def.Script, triggerVars["CTX_TRIGGER_PROMPT"], vars, triggerVars, env, &outBuf, &errBuf)

	errText := ""
	if runErr != nil {
		errText = runErr.Error()
	}

	return a.writeTriggerLog(ctx, change, triggerLog{
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
// from env instead, so a trigger-defined variable takes precedence over a
// ctx session value of the same name. If prompt is non-empty, it is bound
// as one more trailing positional parameter and referenced at the end of
// the (trimmed) script, mirroring the old behavior of appending it to the
// last command's arguments.
// Output is written to stdout and stderr writers.
func runScript(ctx context.Context, command, prompt string, vars map[string]string, triggerVars map[string]string, env []string, stdout, stderr io.Writer) (exitCode int, err error) {
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
	if prompt != "" {
		args = append(args, prompt)
		script = fmt.Sprintf("%s \"$%d\"", strings.TrimRight(script, "\n"), len(args))
	}

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

func (a *App) writeTriggerLog(ctx context.Context, change TriggerChange, log triggerLog) error {
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
