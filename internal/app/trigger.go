package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"ctx/internal/config"
	"ctx/internal/render"
)

type TriggerChange struct {
	SessionID string
	Key       string
	OldValue  string
	NewValue  string
}

type TriggerDefinition struct {
	Name             string
	Path             string
	TriggerSession   string
	Key              string
	Match            string
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

func (a *App) ExecuteMatchingTriggers(ctx context.Context, change TriggerChange) error {
	defs, err := loadTriggerDefinitions()
	if err != nil {
		return err
	}
	matching := []TriggerDefinition{}
	for _, def := range defs {
		matches, err := def.Matches(change)
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
	parts := strings.SplitN(content, "\n---\n", 2)
	if len(parts) != 2 {
		parts = strings.SplitN(content, "---", 2)
	}

	def := TriggerDefinition{
		Name: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
		Path: path,
	}
	if len(parts) == 2 {
		def.PromptTemplate = parts[1]
	}
	for _, line := range strings.Split(parts[0], "\n") {
		line = strings.TrimSpace(stripComment(line))
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return TriggerDefinition{}, fmt.Errorf("malformed trigger %s: invalid frontmatter line %q", path, line)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "session", "trigger-session":
			def.TriggerSession = value
		case "key":
			def.Key = value
		case "match":
			def.Match = value
		case "any-change":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return TriggerDefinition{}, fmt.Errorf("malformed trigger %s: any-change must be true or false", path)
			}
			def.AnyChange = parsed
		case "order":
			parsed, err := strconv.Atoi(value)
			if err != nil {
				return TriggerDefinition{}, fmt.Errorf("malformed trigger %s: order must be an integer", path)
			}
			def.Order = parsed
		case "execution-session":
			def.ExecutionSession = value
		case "command":
			def.Command = value
		default:
			return TriggerDefinition{}, fmt.Errorf("malformed trigger %s: unknown field %q", path, key)
		}
	}
	if def.Command == "" {
		return TriggerDefinition{}, fmt.Errorf("malformed trigger %s: missing command", path)
	}
	if def.AnyChange && (def.TriggerSession != "" || def.Key != "" || def.Match != "") {
		return TriggerDefinition{}, fmt.Errorf("malformed trigger %s: any-change cannot be combined with session, key, or match", path)
	}
	return def, nil
}

func stripComment(line string) string {
	if i := strings.Index(line, "#"); i != -1 {
		return line[:i]
	}
	return line
}

func (d TriggerDefinition) Matches(change TriggerChange) (bool, error) {
	if d.AnyChange {
		return true, nil
	}
	if d.TriggerSession == "" && d.Key == "" && d.Match == "" {
		return false, nil
	}
	if d.TriggerSession != "" && d.TriggerSession != change.SessionID {
		return false, nil
	}
	if d.Key != "" && d.Key != change.Key {
		return false, nil
	}
	if d.Match == "" {
		return true, nil
	}
	re, err := regexp.Compile(d.Match)
	if err != nil {
		return false, fmt.Errorf("trigger %s has invalid match regex: %w", d.Path, err)
	}
	return re.MatchString(change.NewValue) && !re.MatchString(change.OldValue), nil
}

func (a *App) executeTrigger(ctx context.Context, def TriggerDefinition, change TriggerChange) error {
	triggerCtx := context.WithoutCancel(ctx)

	executionSession, err := a.executionSession(triggerCtx, def, change)
	if err != nil {
		return err
	}
	vars, err := a.store.Resolve(triggerCtx, change.SessionID)
	if err != nil {
		return err
	}
	renderedPrompt, err := render.TemplateString(def.PromptTemplate, vars)
	if err != nil {
		return a.writeTriggerLog(triggerCtx, change, triggerLog{
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

	renderedCommand, err := render.TemplateString(def.Command, vars)
	if err != nil {
		return a.writeTriggerLog(triggerCtx, change, triggerLog{
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

	commandParts, err := splitCommandLine(renderedCommand)
	if err != nil {
		return fmt.Errorf("trigger %s has invalid command: %w", def.Path, err)
	}
	if len(commandParts) == 0 {
		return fmt.Errorf("trigger %s has empty command", def.Path)
	}
	args := commandParts[1:]
	if renderedPrompt != "" {
		args = append(args, renderedPrompt)
	}
	cmd := exec.CommandContext(triggerCtx, commandParts[0], args...)
	cmd.Env = append(os.Environ(), "CTX_SUPPRESS_TRIGGERS=1", "CTX_ID="+executionSession)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	exitCode := 0
	var errText string
	if runErr != nil {
		errText = runErr.Error()
		exitCode = 1
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
	}

	return a.writeTriggerLog(triggerCtx, change, triggerLog{
		Trigger:          def.Name,
		SessionID:        change.SessionID,
		Key:              change.Key,
		OldValue:         change.OldValue,
		NewValue:         change.NewValue,
		ExecutionSession: executionSession,
		ExitCode:         exitCode,
		Stdout:           stdout.String(),
		Stderr:           stderr.String(),
		Error:            errText,
	})
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
