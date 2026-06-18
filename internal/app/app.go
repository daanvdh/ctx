package app

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"ctx/internal/config"
	"ctx/internal/render"
	"ctx/internal/session"
	"ctx/internal/store"
)

type App struct {
	DBPath string
}

func New() (*App, error) {
	dbPath, err := config.DBPath()
	if err != nil {
		return nil, err
	}
	return &App{DBPath: dbPath}, nil
}

func (a *App) CreateSession(customID string, explicitParent *string) (string, error) {
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
		return customID, store.CreateSession(a.DBPath, customID, parentID)
	}

	for i := 0; i < 8; i++ {
		id, err := session.GenID()
		if err != nil {
			return "", err
		}
		err = store.CreateSession(a.DBPath, id, parentID)
		if err == nil {
			return id, nil
		}
		if !store.IsAlreadyExists(err) {
			return "", err
		}
	}

	return "", fmt.Errorf("failed to generate a unique session ID")
}

func (a *App) SetValue(sessionID, key, value string) error {
	return store.SetValue(a.DBPath, sessionID, key, value)
}

func (a *App) GetValue(sessionID, key string) (string, error) {
	cf, err := store.Load(a.DBPath)
	if err != nil {
		return "", err
	}
	return session.Get(cf, sessionID, key)
}

func (a *App) Export(sessionID string) ([]string, error) {
	cf, err := store.Load(a.DBPath)
	if err != nil {
		return nil, err
	}
	resolved, err := session.Resolve(cf, sessionID)
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

func (a *App) Tree() (string, error) {
	cf, err := store.Load(a.DBPath)
	if err != nil {
		return "", err
	}
	return render.Tree(cf)
}

func (a *App) Render(sessionID, key string) (string, error) {
	cf, err := store.Load(a.DBPath)
	if err != nil {
		return "", err
	}
	return render.Render(cf, sessionID, key)
}

func (a *App) DeleteSessionTree(sessionID string) error {
	return store.DeleteSessionTree(a.DBPath, sessionID)
}

func (a *App) Execute(sessionID, templateName string) error {
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

	cf, err := store.Load(a.DBPath)
	if err != nil {
		return fmt.Errorf("failed to load context file: %w", err)
	}
	vars, err := session.Resolve(cf, sessionID)
	if err != nil {
		return err
	}

	renderedPrompt, err := render.TemplateString(promptTemplate, vars)
	if err != nil {
		return err
	}

	fullCmd := fmt.Sprintf("%s %s", command, strconv.Quote(renderedPrompt))
	execCmd := exec.Command("sh", "-c", fullCmd)
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr
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
