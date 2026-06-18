package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"ctx/internal/config"
	"ctx/internal/model"
	"ctx/internal/render"
	"ctx/internal/session"
	"ctx/internal/store"
)

const (
	TreeFormatText = "text"
	TreeFormatJSON = "json"
)

type Store interface {
	CreateSession(ctx context.Context, id string, parentID *string) error
	SetValue(ctx context.Context, sessionID, key, value string) error
	GetValue(ctx context.Context, sessionID, key string) (string, error)
	Resolve(ctx context.Context, sessionID string) (map[string]string, error)
	SessionNodes(ctx context.Context) ([]model.SessionNode, error)
	DeleteSessionTree(ctx context.Context, sessionID string) error
}

type App struct {
	store  Store
	stdout io.Writer
	stderr io.Writer
}

func New() (*App, error) {
	dbPath, err := config.DBPath()
	if err != nil {
		return nil, err
	}
	return NewWithStore(store.NewSQLite(dbPath)), nil
}

func NewWithStore(s Store) *App {
	return &App{
		store:  s,
		stdout: os.Stdout,
		stderr: os.Stderr,
	}
}

func (a *App) CreateSession(ctx context.Context, customID string, explicitParent *string) (string, error) {
	parentID := explicitParent
	if parentID == nil {
		if env := os.Getenv("CTX_ID"); env != "" {
			parentID = &env
		}
	}

	if customID != "" {
		if !session.ValidID(customID) {
			return "", fmt.Errorf("invalid session ID: %s", customID)
		}
		return customID, a.store.CreateSession(ctx, customID, parentID)
	}

	for i := 0; i < 8; i++ {
		id, err := session.GenID()
		if err != nil {
			return "", err
		}
		err = a.store.CreateSession(ctx, id, parentID)
		if err == nil {
			return id, nil
		}
		if !store.IsAlreadyExists(err) {
			return "", err
		}
	}

	return "", fmt.Errorf("failed to generate a unique session ID")
}

func (a *App) SetValue(ctx context.Context, sessionID, key, value string) error {
	return a.store.SetValue(ctx, sessionID, key, value)
}

func (a *App) GetValue(ctx context.Context, sessionID, key string) (string, error) {
	return a.store.GetValue(ctx, sessionID, key)
}

func (a *App) Export(ctx context.Context, sessionID string) ([]string, error) {
	resolved, err := a.store.Resolve(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	lines := make([]string, 0, len(resolved))
	for _, key := range sortedKeys(resolved) {
		if !session.ValidShellKey(key) {
			return nil, fmt.Errorf("key %s is not a valid shell variable name", key)
		}
		lines = append(lines, fmt.Sprintf("export %s=%s", key, shellSingleQuote(resolved[key])))
	}
	return lines, nil
}

func (a *App) Tree(ctx context.Context, format string) (string, error) {
	nodes, err := a.store.SessionNodes(ctx)
	if err != nil {
		return "", err
	}
	switch format {
	case "", TreeFormatText:
		return render.TreeNodes(nodes)
	case TreeFormatJSON:
		data, err := json.MarshalIndent(nodes, "", "  ")
		if err != nil {
			return "", fmt.Errorf("render tree json: %w", err)
		}
		return string(data) + "\n", nil
	default:
		return "", fmt.Errorf("unsupported tree format %q", format)
	}
}

func (a *App) Render(ctx context.Context, sessionID, key string) (string, error) {
	template, err := a.store.GetValue(ctx, sessionID, key)
	if err != nil {
		return "", err
	}
	resolved, err := a.store.Resolve(ctx, sessionID)
	if err != nil {
		return "", err
	}
	return render.TemplateString(template, resolved)
}

func (a *App) DeleteSessionTree(ctx context.Context, sessionID string) error {
	return a.store.DeleteSessionTree(ctx, sessionID)
}

func (a *App) Execute(ctx context.Context, sessionID, templateName string) error {
	templatePath, err := config.TriggerPath(templateName)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("failed to read template %s: %w", templatePath, err)
	}

	command, promptTemplate, err := parseTriggerTemplate(string(data))
	if err != nil {
		return err
	}

	vars, err := a.store.Resolve(ctx, sessionID)
	if err != nil {
		return err
	}

	renderedPrompt, err := render.TemplateString(promptTemplate, vars)
	if err != nil {
		return err
	}

	fullCmd := fmt.Sprintf("%s %s", command, strconv.Quote(renderedPrompt))
	execCmd := exec.CommandContext(ctx, "sh", "-c", fullCmd)
	execCmd.Stdout = a.stdout
	execCmd.Stderr = a.stderr
	if err := execCmd.Run(); err != nil {
		return fmt.Errorf("command execution failed: %w", err)
	}
	return nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func shellSingleQuote(v string) string {
	return "'" + strings.ReplaceAll(v, "'", "'\\''") + "'"
}

func parseTriggerTemplate(content string) (string, string, error) {
	parts := strings.SplitN(content, "\n---\n", 2)
	if len(parts) != 2 {
		parts = strings.SplitN(content, "---", 2)
	}
	if len(parts) != 2 {
		return "", "", fmt.Errorf("malformed template: missing '---' separator")
	}

	var commandLine string
	for _, line := range strings.Split(parts[0], "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			commandLine = trimmed
			break
		}
	}

	if !strings.HasPrefix(commandLine, "command=") {
		return "", "", fmt.Errorf("missing 'command=' definition in template")
	}

	command := strings.TrimSpace(strings.TrimPrefix(commandLine, "command="))
	if i := strings.Index(command, "#"); i != -1 {
		command = strings.TrimSpace(command[:i])
	}
	if command == "" {
		return "", "", fmt.Errorf("empty command in template")
	}

	return command, parts[1], nil
}
