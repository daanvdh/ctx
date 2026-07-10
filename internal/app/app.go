package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ctx/internal/config"
	"ctx/internal/model"
	"ctx/internal/render"
	"ctx/internal/session"
	"ctx/internal/store"
	"ctx/internal/textutil"
)

const (
	TreeFormatText = "text"
	TreeFormatJSON = "json"
	MaxDocBytes    = 500 * 1024
)

type Store interface {
	CreateSession(ctx context.Context, id string, parentID *string) error
	SetValue(ctx context.Context, sessionID, key, value string) error
	SetEntry(ctx context.Context, sessionID, key string, entry model.Entry) error
	RemoveEntry(ctx context.Context, sessionID, key string) error
	GetValue(ctx context.Context, sessionID, key string) (string, error)
	GetEntry(ctx context.Context, sessionID, key string) (model.Entry, error)
	Resolve(ctx context.Context, sessionID string) (map[string]string, error)
	ResolveEntries(ctx context.Context, sessionID string) (map[string]model.Entry, error)
	ShareContext(ctx context.Context, fromSessionID, toSessionID string) error
	SessionNodes(ctx context.Context) ([]model.SessionNode, error)
	DeleteSession(ctx context.Context, sessionID string, recursive, noVar, noChild bool) error
}

type App struct {
	store  Store
	stdout io.Writer
	stderr io.Writer
}

func New() (*App, error) {
	settings, err := config.LoadSettings()
	if err != nil {
		return nil, err
	}
	if settings.RemoteMCPURL != "" {
		return NewWithStore(store.NewRemote(settings.RemoteMCPURL, settings.RemoteMCPToken)), nil
	}

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

func (a *App) CreateSession(ctx context.Context, customID string, explicitParent *string, root bool) (string, error) {
	parentID := explicitParent
	if parentID == nil && !root {
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
	return a.SetEntry(ctx, sessionID, key, model.NewEntry(value, model.ValueTypeString))
}

func (a *App) SetEntry(ctx context.Context, sessionID, key string, entry model.Entry) error {
	entry = model.NewEntry(entry.Value, entry.ValueType)
	entry, err := prepareEntry(entry)
	if err != nil {
		return err
	}
	if err := validateEntry(entry); err != nil {
		return err
	}
	oldValue, oldErr := a.store.GetValue(ctx, sessionID, key)
	if err := a.store.SetEntry(ctx, sessionID, key, entry); err != nil {
		return err
	}
	if os.Getenv("CTX_SUPPRESS_TRIGGERS") == "1" {
		return nil
	}
	if oldErr != nil {
		oldValue = ""
	}
	return a.ExecuteMatchingTriggers(ctx, TriggerChange{
		SessionID: sessionID,
		Key:       key,
		OldValue:  oldValue,
		NewValue:  entry.Value,
	})
}

func (a *App) RemoveEntry(ctx context.Context, sessionID, key string) error {
	return a.store.RemoveEntry(ctx, sessionID, key)
}

func (a *App) GetValue(ctx context.Context, sessionID, key string) (string, error) {
	entry, err := a.store.GetEntry(ctx, sessionID, key)
	if err != nil {
		return "", err
	}
	return resolveEntryContent(key, entry, "get")
}

func (a *App) GetPath(ctx context.Context, sessionID, key string) (string, error) {
	entry, err := a.store.GetEntry(ctx, sessionID, key)
	if err != nil {
		return "", err
	}
	switch entry.ValueType {
	case model.ValueTypeString:
		return entry.Value, nil
	case model.ValueTypeDoc:
		file, err := os.CreateTemp("", "ctx-*")
		if err != nil {
			return "", fmt.Errorf("write temp doc: %w", err)
		}
		if _, err := file.WriteString(entry.Value); err != nil {
			_ = file.Close()
			return "", fmt.Errorf("write temp doc: %w", err)
		}
		if err := file.Close(); err != nil {
			return "", fmt.Errorf("write temp doc: %w", err)
		}
		return file.Name(), nil
	case model.ValueTypeFileRef:
		if _, err := os.Stat(entry.Value); err != nil {
			if os.IsNotExist(err) {
				return "", fmt.Errorf("file_ref path no longer exists: %s. Update with: ctx set %s --path <new-path>", entry.Value, key)
			}
			return "", fmt.Errorf("stat file_ref path %s: %w", entry.Value, err)
		}
		return entry.Value, nil
	default:
		return "", fmt.Errorf("%s value type is not implemented", entry.ValueType)
	}
}

func (a *App) GetPreview(ctx context.Context, sessionID, key string) (string, error) {
	value, err := a.GetValue(ctx, sessionID, key)
	if err != nil {
		return "", err
	}
	return firstLines(value, 10), nil
}

func (a *App) ShareContext(ctx context.Context, fromSessionID, toSessionID string) error {
	return a.store.ShareContext(ctx, fromSessionID, toSessionID)
}

func (a *App) Resolve(ctx context.Context, sessionID string) (map[string]string, error) {
	entries, err := a.store.ResolveEntries(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return resolveEntries(entries, "resolve")
}

// ResolveEntries returns all visible entries for a session as stored
// (doc values in full, file_ref values as their path, unlike Resolve which
// reads file_ref content). Used by the ctx_resolve_entries MCP tool so a
// remote-backed client can reconstruct typed entries.
func (a *App) ResolveEntries(ctx context.Context, sessionID string) (map[string]model.Entry, error) {
	return a.store.ResolveEntries(ctx, sessionID)
}

func (a *App) Export(ctx context.Context, sessionID string, includeDocs, filesAsPaths, quiet bool) ([]string, error) {
	entries, err := a.store.ResolveEntries(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	lines := make([]string, 0, len(entries)+1)
	lines = append(lines, fmt.Sprintf("export CTX_ID=%s", shellSingleQuote(sessionID)))
	for _, key := range sortedEntryKeys(entries) {
		if !session.ValidShellKey(key) {
			if !quiet {
				lines = append(lines, fmt.Sprintf("echo %s", shellSingleQuote(fmt.Sprintf("warning: ctx export: key %s is not a valid shell variable name and is ignored.", key))))
			}
			continue
		}
		entry := entries[key]
		switch entry.ValueType {
		case model.ValueTypeString:
			lines = append(lines, fmt.Sprintf("export %s=%s", key, shellSingleQuote(entry.Value)))
		case model.ValueTypeDoc:
			if includeDocs {
				lines = append(lines, fmt.Sprintf("export %s=%s", key, shellSingleQuote(entry.Value)))
			}
		case model.ValueTypeFileRef:
			if filesAsPaths {
				lines = append(lines, fmt.Sprintf("export %s=%s", key, shellSingleQuote(entry.Value)))
			}
		case model.ValueTypeFileBin:
		default:
			return nil, fmt.Errorf("unsupported value type %q for key %s", entry.ValueType, key)
		}
	}
	return lines, nil
}

type ShowOptions struct {
	Full   bool
	Render bool
}

func (a *App) Show(ctx context.Context, sessionID string, opts ShowOptions) ([]string, error) {
	entries, err := a.store.ResolveEntries(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	var resolvedForRender map[string]string
	if opts.Render {
		resolvedForRender, err = resolveEntries(entries, "render")
		if err != nil {
			return nil, err
		}
	}

	lines := make([]string, 0, len(entries))
	for _, key := range sortedEntryKeys(entries) {
		entry := entries[key]
		if !opts.Full && !opts.Render {
			lines = append(lines, textutil.Line(key, entry))
			continue
		}

		content, err := resolveEntryContent(key, entry, "show")
		if err != nil {
			return nil, err
		}
		if opts.Render {
			content, err = render.TemplateStringRecursive(content, resolvedForRender, render.TemplateOptions{}, render.MaxRenderDepth)
			if err != nil {
				return nil, err
			}
		}
		if !opts.Full {
			content = textutil.Preview(content, textutil.PreviewChars)
		}
		lines = append(lines, textutil.FullLine(key, entry, content))
	}
	return lines, nil
}

func (a *App) ShowEntries(ctx context.Context, sessionID string) ([]map[string]any, error) {
	entries, err := a.store.ResolveEntries(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(entries))
	for _, key := range sortedEntryKeys(entries) {
		entry := entries[key]
		item := map[string]any{
			"key":        key,
			"value_type": string(entry.ValueType),
		}
		switch entry.ValueType {
		case model.ValueTypeString:
			item["value"] = entry.Value
		case model.ValueTypeDoc:
			item["size_bytes"] = len([]byte(entry.Value))
			item["preview"] = textutil.Preview(entry.Value, textutil.PreviewChars)
		case model.ValueTypeFileRef:
			item["path"] = entry.Value
			_, err := os.Stat(entry.Value)
			item["path_exists"] = err == nil
		}
		out = append(out, item)
	}
	return out, nil
}

func (a *App) Tree(ctx context.Context, format, sessionID string) (string, error) {
	nodes, err := a.store.SessionNodes(ctx)
	if err != nil {
		return "", err
	}
	if sessionID != "" {
		nodes, err = filterTreeNodes(nodes, sessionID)
		if err != nil {
			return "", err
		}
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

// filterTreeNodes reduces nodes to the ancestor chain of sessionID (down to
// the root) plus the full descendant subtree of sessionID, so the resulting
// forest still renders correctly with render.TreeNodes.
func filterTreeNodes(nodes []model.SessionNode, sessionID string) ([]model.SessionNode, error) {
	byID := make(map[string]model.SessionNode, len(nodes))
	children := make(map[string][]string, len(nodes))
	for _, node := range nodes {
		byID[node.ID] = node
		if node.Parent != nil {
			children[*node.Parent] = append(children[*node.Parent], node.ID)
		}
	}

	if _, ok := byID[sessionID]; !ok {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	keep := make(map[string]bool)
	for id := sessionID; id != "" && !keep[id]; {
		keep[id] = true
		node := byID[id]
		if node.Parent == nil {
			break
		}
		id = *node.Parent
	}

	queue := []string{sessionID}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		for _, childID := range children[id] {
			if !keep[childID] {
				keep[childID] = true
				queue = append(queue, childID)
			}
		}
	}

	out := make([]model.SessionNode, 0, len(keep))
	for _, node := range nodes {
		if keep[node.ID] {
			out = append(out, node)
		}
	}
	return out, nil
}

func (a *App) Render(ctx context.Context, sessionID, key string, ignoreMissing bool) (string, error) {
	templateEntry, err := a.store.GetEntry(ctx, sessionID, key)
	if err != nil {
		return "", err
	}
	template, err := resolveEntryContent(key, templateEntry, "render")
	if err != nil {
		return "", err
	}
	entries, err := a.store.ResolveEntries(ctx, sessionID)
	if err != nil {
		return "", err
	}
	resolved, err := resolveEntries(entries, "render")
	if err != nil {
		return "", err
	}
	return render.TemplateStringWithOptions(template, resolved, render.TemplateOptions{IgnoreMissing: ignoreMissing})
}

func (a *App) DeleteSession(ctx context.Context, sessionID string, recursive, noVar, noChild bool) error {
	return a.store.DeleteSession(ctx, sessionID, recursive, noVar, noChild)
}

func (a *App) Execute(ctx context.Context, sessionID, templateName string) error {
	templatePath, err := triggerTemplatePath(templateName)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("failed to read template %s: %w", templatePath, err)
	}

	def, err := parseTriggerDefinition(templatePath, string(data))
	if err != nil {
		return err
	}

	vars, err := a.store.Resolve(ctx, sessionID)
	if err != nil {
		return err
	}

	triggerVars, err := renderTriggerVars(def.PromptTemplate, vars)
	if err != nil {
		return err
	}

	env := append(os.Environ(), "CTX_SUPPRESS_TRIGGERS=1", "CTX_ID="+sessionID)
	for name, value := range triggerVars {
		env = append(env, name+"="+value)
	}
	if _, err := runScript(ctx, def.Script, triggerVars["CTX_TRIGGER_PROMPT"], vars, triggerVars, env, a.stdout, a.stderr); err != nil {
		return fmt.Errorf("script execution failed: %w", err)
	}
	return nil
}

func triggerTemplatePath(templateName string) (string, error) {
	templatePath, err := config.TriggerPath(templateName)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(templatePath); err == nil {
		return templatePath, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}

	if ext := filepath.Ext(templateName); ext != "" {
		return templatePath, nil
	}

	triggerDir, err := config.TriggerDir()
	if err != nil {
		return "", err
	}
	matches, err := filepath.Glob(filepath.Join(triggerDir, templateName+".*"))
	if err != nil {
		return "", err
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("multiple trigger templates match %q", templateName)
	}
	return templatePath, nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedEntryKeys(m map[string]model.Entry) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func prepareEntry(entry model.Entry) (model.Entry, error) {
	switch entry.ValueType {
	case model.ValueTypeFileRef:
		path, err := absoluteExistingPath(entry.Value)
		if err != nil {
			return model.Entry{}, err
		}
		entry.Value = path
	}
	return entry, nil
}

func absoluteExistingPath(path string) (string, error) {
	if _, err := os.Stat(path); err == nil {
		abs, err := filepath.Abs(path)
		if err != nil {
			return "", fmt.Errorf("resolve absolute path %s: %w", path, err)
		}
		return abs, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat file_ref path %s: %w", path, err)
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path %s: %w", path, err)
	}
	if _, err := os.Stat(abs); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("file not found at path: %s", path)
		}
		return "", fmt.Errorf("stat file_ref path %s: %w", abs, err)
	}
	return abs, nil
}

func validateEntry(entry model.Entry) error {
	switch entry.ValueType {
	case model.ValueTypeString:
		return nil
	case model.ValueTypeDoc:
		if len([]byte(entry.Value)) > MaxDocBytes {
			return fmt.Errorf("doc content exceeds 500KB. Consider splitting into multiple keys or referencing a file with --path")
		}
		return nil
	case model.ValueTypeFileRef:
		if _, err := os.Stat(entry.Value); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("file not found at path: %s", entry.Value)
			}
			return fmt.Errorf("stat file_ref path %s: %w", entry.Value, err)
		}
		return nil
	case model.ValueTypeFileBin:
		return fmt.Errorf("file_bin value type is not implemented")
	default:
		return fmt.Errorf("unsupported value type %q", entry.ValueType)
	}
}

func resolveEntries(entries map[string]model.Entry, op string) (map[string]string, error) {
	resolved := make(map[string]string, len(entries))
	for key, entry := range entries {
		value, err := resolveEntryContent(key, entry, op)
		if err != nil {
			return nil, err
		}
		resolved[key] = value
	}
	return resolved, nil
}

func resolveEntryContent(key string, entry model.Entry, op string) (string, error) {
	switch entry.ValueType {
	case model.ValueTypeString, model.ValueTypeDoc:
		return entry.Value, nil
	case model.ValueTypeFileRef:
		data, err := os.ReadFile(entry.Value)
		if err != nil {
			if os.IsNotExist(err) {
				if op == "render" {
					return "", fmt.Errorf("cannot render template - file_ref key '%s' path not found: %s", key, entry.Value)
				}
				return "", fmt.Errorf("file_ref path no longer exists: %s. Update with: ctx set %s --path <new-path>", entry.Value, key)
			}
			return "", fmt.Errorf("read file_ref path %s: %w", entry.Value, err)
		}
		return string(data), nil
	case model.ValueTypeFileBin:
		return "", fmt.Errorf("file_bin value type is not implemented")
	default:
		return "", fmt.Errorf("unsupported value type %q for key %s", entry.ValueType, key)
	}
}

func firstLines(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	lines := strings.SplitAfter(value, "\n")
	if len(lines) <= limit {
		return value
	}
	return strings.Join(lines[:limit], "")
}

func shellSingleQuote(v string) string {
	return "'" + strings.ReplaceAll(v, "'", "'\\''") + "'"
}
