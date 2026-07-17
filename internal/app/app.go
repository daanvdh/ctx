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
	"sync"

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
	MaxStringBytes = 500 * 1024
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
	// backgroundTriggers makes writes fire matching triggers in a detached
	// ctx process instead of blocking the caller. Enabled by the CLI set
	// path; stays off for tests, the MCP server, and the scheduler.
	backgroundTriggers bool
}

// EnableBackgroundTriggers makes this App fire triggers in a detached
// background process so writes return immediately.
func (a *App) EnableBackgroundTriggers() { a.backgroundTriggers = true }

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
	return a.setEntryAtDepth(ctx, sessionID, key, entry, triggerDepth())
}

// setEntryAtDepth stores an entry and fires matching triggers at the given
// trigger chain depth. Depth is threaded explicitly so in-process chained
// writes (e.g. output-entry) count against maxTriggerDepth like writes made
// by spawned ctx processes do via CTX_TRIGGER_DEPTH.
func (a *App) setEntryAtDepth(ctx context.Context, sessionID, key string, entry model.Entry, depth int) error {
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
	if depth >= maxTriggerDepth {
		fmt.Fprintf(a.stderr, "ctx: trigger depth limit (%d) reached; not firing triggers for %s\n", maxTriggerDepth, key)
		return nil
	}
	if oldErr != nil {
		oldValue = ""
	}
	change := TriggerChange{
		SessionID: sessionID,
		Key:       key,
		OldValue:  oldValue,
		NewValue:  entry.Value,
		Depth:     depth,
	}
	if a.backgroundTriggers {
		return spawnTriggerRunner(change)
	}
	return a.ExecuteMatchingTriggers(ctx, change)
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

// GetRendered returns a key's value with $VAR placeholders substituted
// recursively from the session's visible context, matching the rendering
// ctx list --full performs. If allowMissing is false and a placeholder can't
// be resolved, the returned error suggests --allow-missing.
func (a *App) GetRendered(ctx context.Context, sessionID, key string, allowMissing bool) (string, error) {
	entry, err := a.store.GetEntry(ctx, sessionID, key)
	if err != nil {
		return "", err
	}
	return renderEntryValue(key, entry, false, allowMissing, func() (map[string]string, error) {
		entries, err := a.store.ResolveEntries(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		return resolveEntries(entries, "render")
	})
}

// GetRaw returns a key's stored value without rendering $VAR placeholders.
// For file_ref entries it returns the referenced path itself rather than the
// file's content, since --raw is meant to expose exactly what's stored for
// the key. --raw never renders, so a missing placeholder can never make it
// fail; there is deliberately no allowMissing parameter here.
func (a *App) GetRaw(ctx context.Context, sessionID, key string) (string, error) {
	entry, err := a.store.GetEntry(ctx, sessionID, key)
	if err != nil {
		return "", err
	}
	return renderEntryValue(key, entry, true, false, nil)
}

// renderEntryValue turns a stored entry into the string ctx get and ctx list
// expose for it, so both commands apply the same rules for a given entry
// instead of duplicating (and drifting on) this decision. raw returns the
// entry's stored content unprocessed — the referenced path itself for
// file_ref, since that's what's stored in the db, and it's what --raw is
// meant to expose. Otherwise content is resolved (file_ref reads its
// file) and, unless the entry is a file_ref, rendered against the context
// resolveContext returns: file_ref entries reference files living outside
// ctx that weren't authored with ctx's $VAR syntax in mind, so rendering
// them could break on unrelated "$" occurrences. resolveContext is called
// lazily, and only when rendering is actually about to happen, so a raw or
// file_ref lookup never pays for (or can be broken by) resolving the rest
// of the session.
func renderEntryValue(key string, entry model.Entry, raw bool, allowMissing bool, resolveContext func() (map[string]string, error)) (string, error) {
	if raw && entry.ValueType == model.ValueTypeFileRef {
		if _, err := os.Stat(entry.Value); err != nil {
			if os.IsNotExist(err) {
				return "", fmt.Errorf("file_ref path no longer exists: %s. Update with: ctx set %s --path <new-path>", entry.Value, key)
			}
			return "", fmt.Errorf("stat file_ref path %s: %w", entry.Value, err)
		}
		return entry.Value, nil
	}

	content, err := resolveEntryContent(key, entry, "get")
	if err != nil {
		return "", err
	}
	if raw || entry.ValueType == model.ValueTypeFileRef {
		return content, nil
	}

	resolved, err := resolveContext()
	if err != nil {
		return "", err
	}
	rendered, err := render.TemplateStringRecursive(content, resolved, render.TemplateOptions{AllowMissing: allowMissing}, render.MaxRenderDepth)
	if err != nil {
		return "", fmt.Errorf("%w (use --allow-missing to leave unresolved placeholders unchanged)", err)
	}
	return rendered, nil
}

func (a *App) GetPreview(ctx context.Context, sessionID, key string, raw bool) (string, error) {
	var value string
	var err error
	if raw {
		value, err = a.GetValue(ctx, sessionID, key)
	} else {
		value, err = a.GetRendered(ctx, sessionID, key, false)
	}
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
// (string values in full, file_ref values as their path, unlike Resolve which
// reads file_ref content). Used by the ctx_resolve_entries MCP tool so a
// remote-backed client can reconstruct typed entries.
func (a *App) ResolveEntries(ctx context.Context, sessionID string) (map[string]model.Entry, error) {
	return a.store.ResolveEntries(ctx, sessionID)
}

func (a *App) Export(ctx context.Context, sessionID string, filesAsPaths, quiet bool) ([]string, error) {
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

type ListOptions struct {
	Full   bool
	Render bool
}

func (a *App) List(ctx context.Context, sessionID string, opts ListOptions) ([]string, error) {
	entries, err := a.store.ResolveEntries(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	// Memoized so the whole session is resolved at most once for this call,
	// and not at all when every entry is shown raw or is a file_ref.
	resolveContext := sync.OnceValues(func() (map[string]string, error) {
		return resolveEntries(entries, "render")
	})

	lines := make([]string, 0, len(entries))
	for _, key := range sortedEntryKeys(entries) {
		entry := entries[key]
		if !opts.Full && !opts.Render {
			lines = append(lines, textutil.Line(key, entry))
			continue
		}

		// allowMissing is always true here: a listing shouldn't fail outright
		// over one entry's unresolved placeholder; leave it unchanged instead.
		content, err := renderEntryValue(key, entry, !opts.Render, true, resolveContext)
		if err != nil {
			return nil, err
		}
		if !opts.Full {
			content = textutil.Preview(content, textutil.PreviewChars)
		}
		lines = append(lines, textutil.FullLine(key, entry, content))
	}
	return lines, nil
}

func (a *App) ListEntries(ctx context.Context, sessionID string) ([]map[string]any, error) {
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

func (a *App) Render(ctx context.Context, sessionID, key string, allowMissing bool) (string, error) {
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
	return render.TemplateStringWithOptions(template, resolved, render.TemplateOptions{AllowMissing: allowMissing})
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

	env := triggerEnv(triggerDepth()+1, sessionID)
	for name, value := range triggerVars {
		env = append(env, name+"="+value)
	}
	if def.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, def.Timeout)
		defer cancel()
	}
	if _, err := runScript(ctx, def.Script, vars, triggerVars, env, a.stdout, a.stderr); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("script timed out after %s", def.Timeout)
		}
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
		if len([]byte(entry.Value)) > MaxStringBytes {
			return fmt.Errorf("value exceeds 500KB. Consider splitting into multiple keys or referencing a file with --path")
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
	case model.ValueTypeString:
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
