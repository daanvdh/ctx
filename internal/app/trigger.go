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
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"ctx/internal/config"
	"ctx/internal/render"
	"ctx/internal/session"
	"gopkg.in/yaml.v3"
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
	Session          string
	Ancestor         string
	Entries          map[string][]string // key -> accepted values; nil/empty slice = wildcard
	AnyChange        bool
	Order            int
	ExecutionSession string
	Command          string
	PromptTemplate   string
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
	Command          string                         `yaml:"command"`
	Session          string                         `yaml:"session"`
	Ancestor         string                         `yaml:"ancestor"`
	AnyChange        bool                           `yaml:"any-change"`
	Order            int                            `yaml:"order"`
	ExecutionSession string                         `yaml:"execution-session"`
	Entries          map[string][]triggerEntryValue `yaml:"entries"`
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

	if data.Command == "" {
		return TriggerDefinition{}, fmt.Errorf("malformed trigger %s: missing command", path)
	}
	if data.AnyChange && (data.Session != "" || data.Ancestor != "" || len(data.Entries) > 0) {
		return TriggerDefinition{}, fmt.Errorf("malformed trigger %s: any-change cannot be combined with session, ancestor, or entries", path)
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
		Session:          data.Session,
		Ancestor:         data.Ancestor,
		Entries:          entries,
		AnyChange:        data.AnyChange,
		Order:            data.Order,
		ExecutionSession: data.ExecutionSession,
		Command:          data.Command,
		PromptTemplate:   promptTemplate,
	}, nil
}

// Matches reports whether this trigger should fire for the given change.
// vars contains the fully resolved current values for the triggering session.
// ancestors contains the IDs of the triggering session's ancestors (not including itself).
func (d TriggerDefinition) Matches(change TriggerChange, vars map[string]string, ancestors map[string]bool) (bool, error) {
	if d.AnyChange {
		return true, nil
	}
	if d.Session == "" && d.Ancestor == "" && len(d.Entries) == 0 {
		return false, nil // manual only
	}
	if d.Session != "" && d.Session != change.SessionID {
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

func (a *App) executeTrigger(ctx context.Context, def TriggerDefinition, change TriggerChange) error {
	executionSession, err := a.executionSession(ctx, def, change)
	if err != nil {
		return err
	}
	vars, err := a.store.Resolve(ctx, change.SessionID)
	if err != nil {
		return err
	}
	renderedPrompt, err := render.TemplateString(def.PromptTemplate, vars)
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
	var outBuf, errBuf bytes.Buffer
	exitCode, runErr := runCommandLines(ctx, a, def.Command, renderedPrompt, vars, executionSession, env, &outBuf, &errBuf)

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

// runCommandLines executes a command string that may contain multiple lines.
// Each non-empty line is executed in isolation as a separate subprocess.
// Lines matching KEY=value (uppercase-starting key, no spaces before =) are
// stored in the ctx session and made available to subsequent lines via vars.
// If value is wrapped as $(...), the enclosed command is executed first and
// its trimmed stdout becomes the stored value, mirroring shell command
// substitution.
// The rendered prompt (if non-empty) is appended as the last argument to the
// last non-assignment command line.
//
// Quoting and word-splitting are resolved on the raw, unrendered line before
// any $VAR placeholder is substituted, and each resulting word is rendered on
// its own. This mirrors how a real shell expands "$VAR": the placeholder's
// value is spliced in as an opaque, literal string and can never introduce
// its own quotes or spaces to change argument boundaries.
//
// Output is written to stdout and stderr writers.
func runCommandLines(ctx context.Context, a *App, command, prompt string, vars map[string]string, sessionID string, env []string, stdout, stderr io.Writer) (exitCode int, err error) {
	localVars := make(map[string]string, len(vars))
	for k, v := range vars {
		localVars[k] = v
	}

	lines := commandLines(command)
	for i, rawLine := range lines {
		if key, rawValue, ok := parseAssignment(rawLine); ok {
			value, code, err := renderAssignmentValue(ctx, rawValue, localVars, env, stderr)
			if err != nil {
				return code, err
			}
			if err := a.store.SetValue(ctx, sessionID, key, value); err != nil {
				return -1, err
			}
			localVars[key] = value
			continue
		}

		rawParts, err := splitCommandLine(rawLine)
		if err != nil {
			return -1, fmt.Errorf("invalid command: %w", err)
		}
		if len(rawParts) == 0 {
			continue
		}

		cmdParts, err := renderArgs(rawParts, localVars)
		if err != nil {
			return -1, err
		}
		if allBlank(cmdParts) {
			continue
		}

		args := cmdParts[1:]
		if prompt != "" && i == len(lines)-1 {
			args = append(args, prompt)
		}

		if code, runErr := runSubprocess(ctx, append([]string{cmdParts[0]}, args...), env, stdout, stderr); runErr != nil {
			return code, runErr
		}
	}

	return 0, nil
}

// renderArgs renders each already-tokenized argument on its own, so a
// placeholder's value is substituted as a single opaque argument regardless
// of any quote or whitespace characters it contains.
func renderArgs(rawParts []string, vars map[string]string) ([]string, error) {
	rendered := make([]string, len(rawParts))
	for i, part := range rawParts {
		value, err := render.TemplateString(part, vars)
		if err != nil {
			return nil, err
		}
		rendered[i] = value
	}
	return rendered, nil
}

// allBlank reports whether every part is empty once trimmed, meaning the
// line resolved to nothing (e.g. a line consisting solely of a placeholder
// that rendered empty) and should be treated as a no-op.
func allBlank(parts []string) bool {
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			return false
		}
	}
	return true
}

// renderAssignmentValue renders the raw right-hand side of a KEY=value
// assignment. If the raw (unrendered) value is wrapped as $(...), the
// enclosed command is tokenized and rendered argument-by-argument (the same
// quote-safe process used for regular command lines), then executed; its
// trimmed stdout becomes the value. Otherwise the raw value is rendered as a
// single opaque string, since it is stored as-is and never split into argv.
// The returned exit code is only meaningful when err is non-nil.
func renderAssignmentValue(ctx context.Context, rawValue string, vars map[string]string, env []string, stderr io.Writer) (value string, exitCode int, err error) {
	if rawInner, isCmd := commandSubstitution(rawValue); isCmd {
		rawParts, err := splitCommandLine(rawInner)
		if err != nil {
			return "", -1, fmt.Errorf("invalid command: %w", err)
		}
		if len(rawParts) == 0 {
			return "", -1, fmt.Errorf("invalid command: empty command substitution")
		}
		cmdParts, err := renderArgs(rawParts, vars)
		if err != nil {
			return "", -1, err
		}
		var outBuf bytes.Buffer
		if code, runErr := runSubprocess(ctx, cmdParts, env, &outBuf, stderr); runErr != nil {
			return "", code, runErr
		}
		return strings.TrimRight(outBuf.String(), "\n"), 0, nil
	}

	value, err = render.TemplateString(rawValue, vars)
	if err != nil {
		return "", -1, err
	}
	return value, 0, nil
}

// runSubprocess runs cmdParts as a subprocess, returning the exit code and
// error (if any) with the same exit-code extraction used across the command
// runner.
func runSubprocess(ctx context.Context, cmdParts []string, env []string, stdout, stderr io.Writer) (int, error) {
	cmd := exec.CommandContext(ctx, cmdParts[0], cmdParts[1:]...)
	cmd.Env = env
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if runErr := cmd.Run(); runErr != nil {
		code := 1
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			code = exitErr.ExitCode()
		}
		return code, runErr
	}
	return 0, nil
}

// commandSubstitution reports whether value is wrapped as $(...), which marks
// the enclosed command for execution; its trimmed stdout becomes the assigned
// value, mirroring shell command substitution.
func commandSubstitution(value string) (inner string, ok bool) {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, "$(") || !strings.HasSuffix(trimmed, ")") {
		return "", false
	}
	return strings.TrimSpace(trimmed[2 : len(trimmed)-1]), true
}

// commandLines splits a command string into individual non-empty lines,
// trimming whitespace and trailing semicolons from each line.
func commandLines(command string) []string {
	var lines []string
	for _, line := range strings.Split(command, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimSuffix(line, ";")
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// parseAssignment detects a KEY=value assignment where KEY is a valid ctx key
// that starts with a letter or underscore and contains no whitespace before =.
// This distinguishes assignments from commands that happen to contain =.
func parseAssignment(line string) (key, value string, ok bool) {
	idx := strings.IndexByte(line, '=')
	if idx <= 0 {
		return "", "", false
	}
	k := line[:idx]
	if strings.ContainsAny(k, " \t") {
		return "", "", false
	}
	if !session.ValidShellKey(k) {
		return "", "", false
	}
	return k, line[idx+1:], true
}

// splitCommandLine splits a command string into argv-style parts while preserving quoted arguments.
func splitCommandLine(command string) ([]string, error) {
	var parts []string
	var current strings.Builder
	inSingle := false
	inDouble := false
	escaping := false
	hasPart := false

	for _, r := range command {
		if escaping {
			current.WriteRune(r)
			escaping = false
			hasPart = true
			continue
		}

		switch {
		case r == '\\' && !inSingle:
			escaping = true
			hasPart = true
		case r == '\'' && !inDouble:
			inSingle = !inSingle
			hasPart = true
		case r == '"' && !inSingle:
			inDouble = !inDouble
			hasPart = true
		case (r == ' ' || r == '\t' || r == '\n' || r == '\r') && !inSingle && !inDouble:
			if hasPart {
				parts = append(parts, current.String())
				current.Reset()
				hasPart = false
			}
		default:
			current.WriteRune(r)
			hasPart = true
		}
	}

	if escaping {
		return nil, fmt.Errorf("unterminated escape")
	}
	if inSingle || inDouble {
		return nil, fmt.Errorf("unterminated quote")
	}
	if hasPart {
		parts = append(parts, current.String())
	}
	return parts, nil
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
	id, err := a.CreateSession(ctx, "", &parent)
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
	key := fmt.Sprintf("trigger_log.%d", time.Now().UnixNano())
	return a.store.SetValue(ctx, change.SessionID, key, string(data))
}
