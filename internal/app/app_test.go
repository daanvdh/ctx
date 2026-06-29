package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ctx/internal/model"
)

type fakeStore struct {
	createdID       string
	parentID        *string
	values          map[string]string
	entries         map[string]model.Entry
	resolved        map[string]string
	resolvedEntries map[string]model.Entry
	nodes           []model.SessionNode
	err             error
}

func (f *fakeStore) CreateSession(_ context.Context, id string, parentID *string) error {
	f.createdID = id
	f.parentID = parentID
	return f.err
}

func (f *fakeStore) SetValue(_ context.Context, sessionID, key, value string) error {
	return f.SetEntry(context.Background(), sessionID, key, model.NewEntry(value, model.ValueTypeString))
}

func (f *fakeStore) SetEntry(_ context.Context, sessionID, key string, entry model.Entry) error {
	if f.values == nil {
		f.values = make(map[string]string)
	}
	if f.entries == nil {
		f.entries = make(map[string]model.Entry)
	}
	f.values[sessionID+"."+key] = entry.Value
	f.values[key] = entry.Value
	f.entries[sessionID+"."+key] = entry
	f.entries[key] = entry
	return f.err
}

func (f *fakeStore) GetValue(_ context.Context, _, key string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.values[key], nil
}

func (f *fakeStore) GetEntry(_ context.Context, _, key string) (model.Entry, error) {
	if f.err != nil {
		return model.Entry{}, f.err
	}
	if f.entries != nil {
		if entry, ok := f.entries[key]; ok {
			return entry, nil
		}
	}
	return model.NewEntry(f.values[key], model.ValueTypeString), nil
}

func (f *fakeStore) Resolve(context.Context, string) (map[string]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.resolved, nil
}

func (f *fakeStore) ResolveEntries(context.Context, string) (map[string]model.Entry, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.resolvedEntries != nil {
		return f.resolvedEntries, nil
	}
	entries := make(map[string]model.Entry, len(f.resolved))
	for key, value := range f.resolved {
		entries[key] = model.NewEntry(value, model.ValueTypeString)
	}
	return entries, nil
}

func (f *fakeStore) ShareContext(context.Context, string, string) error {
	return f.err
}

func (f *fakeStore) SessionNodes(context.Context) ([]model.SessionNode, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.nodes, nil
}

func (f *fakeStore) DeleteSessionTree(context.Context, string) error {
	return f.err
}

func TestCreateSessionUsesInjectedStore(t *testing.T) {
	parent := "root"
	fake := &fakeStore{}
	a := NewWithStore(fake)

	id, err := a.CreateSession(context.Background(), "child", &parent)
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}
	if id != "child" || fake.createdID != "child" {
		t.Fatalf("created id = %q/%q, want child", id, fake.createdID)
	}
	if fake.parentID == nil || *fake.parentID != "root" {
		t.Fatalf("parent = %v, want root", fake.parentID)
	}
}

func TestExportRejectsInvalidShellKey(t *testing.T) {
	a := NewWithStore(&fakeStore{resolved: map[string]string{"1BAD": "value"}})
	_, err := a.Export(context.Background(), "s1", false, false)
	if err == nil {
		t.Fatal("expected invalid shell key error")
	}
}

func TestExportIncludesCTXID(t *testing.T) {
	a := NewWithStore(&fakeStore{resolved: map[string]string{"KEY": "value"}})
	lines, err := a.Export(context.Background(), "s1", false, false)
	if err != nil {
		t.Fatalf("Export error: %v", err)
	}
	if lines[0] != "export CTX_ID='s1'" {
		t.Fatalf("first export line = %q, want CTX_ID", lines[0])
	}
}

func TestExportOmitsDocsAndFileRefsByDefault(t *testing.T) {
	a := NewWithStore(&fakeStore{resolvedEntries: map[string]model.Entry{
		"NAME": model.NewEntry("ctx", model.ValueTypeString),
		"DOC":  model.NewEntry("long text", model.ValueTypeDoc),
		"SPEC": model.NewEntry("/tmp/spec.yaml", model.ValueTypeFileRef),
	}})
	lines, err := a.Export(context.Background(), "s1", false, false)
	if err != nil {
		t.Fatalf("Export error: %v", err)
	}
	got := strings.Join(lines, "\n")
	if strings.Contains(got, "DOC=") || strings.Contains(got, "SPEC=") {
		t.Fatalf("default export leaked doc/file_ref: %s", got)
	}
	if !strings.Contains(got, "NAME='ctx'") {
		t.Fatalf("export = %s, want scalar", got)
	}
}

func TestExportIncludesDocsAndFilePathsWhenRequested(t *testing.T) {
	a := NewWithStore(&fakeStore{resolvedEntries: map[string]model.Entry{
		"DOC":  model.NewEntry("don't split", model.ValueTypeDoc),
		"SPEC": model.NewEntry("/tmp/spec.yaml", model.ValueTypeFileRef),
	}})
	lines, err := a.Export(context.Background(), "s1", true, true)
	if err != nil {
		t.Fatalf("Export error: %v", err)
	}
	got := strings.Join(lines, "\n")
	if !strings.Contains(got, "DOC='don'\\''t split'") || !strings.Contains(got, "SPEC='/tmp/spec.yaml'") {
		t.Fatalf("export = %s, want doc and file path", got)
	}
}

func TestGetFileRefReadsContentAndPath(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "doc.txt")
	if err := os.WriteFile(path, []byte("line1\nline2\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	a := NewWithStore(&fakeStore{entries: map[string]model.Entry{
		"SPEC": model.NewEntry(path, model.ValueTypeFileRef),
	}})

	value, err := a.GetValue(context.Background(), "s1", "SPEC")
	if err != nil {
		t.Fatalf("GetValue error: %v", err)
	}
	if value != "line1\nline2\n" {
		t.Fatalf("file_ref value = %q, want file content", value)
	}
	gotPath, err := a.GetPath(context.Background(), "s1", "SPEC")
	if err != nil {
		t.Fatalf("GetPath error: %v", err)
	}
	if gotPath != path {
		t.Fatalf("path = %q, want %q", gotPath, path)
	}
}

func TestSetFileRefStoresAbsolutePath(t *testing.T) {
	t.Setenv("CTX_SUPPRESS_TRIGGERS", "1")
	tmp := t.TempDir()
	t.Chdir(tmp)
	if err := os.WriteFile("spec.yaml", []byte("openapi: 3.0.0\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	want := filepath.Join(tmp, "spec.yaml")
	fake := &fakeStore{}
	a := NewWithStore(fake)

	if err := a.SetEntry(context.Background(), "s1", "SPEC", model.NewEntry("spec.yaml", model.ValueTypeFileRef)); err != nil {
		t.Fatalf("SetEntry error: %v", err)
	}
	got := fake.entries["SPEC"].Value
	if got != want {
		t.Fatalf("stored path = %q, want %q", got, want)
	}
}

func TestGetDocPathWritesTempFileAndPreview(t *testing.T) {
	content := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n"
	a := NewWithStore(&fakeStore{entries: map[string]model.Entry{
		"DOC": model.NewEntry(content, model.ValueTypeDoc),
	}})

	path, err := a.GetPath(context.Background(), "s1", "DOC")
	if err != nil {
		t.Fatalf("GetPath error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read temp doc: %v", err)
	}
	if string(data) != content {
		t.Fatalf("temp content = %q, want doc", string(data))
	}
	preview, err := a.GetPreview(context.Background(), "s1", "DOC")
	if err != nil {
		t.Fatalf("GetPreview error: %v", err)
	}
	if preview != "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n" {
		t.Fatalf("preview = %q, want first 10 lines", preview)
	}
}

func TestSetDocRejectsTooLargeContent(t *testing.T) {
	a := NewWithStore(&fakeStore{})
	err := a.SetEntry(context.Background(), "s1", "DOC", model.NewEntry(strings.Repeat("x", MaxDocBytes+1), model.ValueTypeDoc))
	if err == nil || !strings.Contains(err.Error(), "doc content exceeds 500KB") {
		t.Fatalf("SetEntry error = %v, want doc size error", err)
	}
}

func TestShow(t *testing.T) {
	a := NewWithStore(&fakeStore{resolved: map[string]string{"B": "2", "A": "1"}})
	lines, err := a.Show(context.Background(), "s1")
	if err != nil {
		t.Fatalf("Show error: %v", err)
	}
	want := []string{"A [string] 1", "B [string] 2"}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("Show = %v, want %v", lines, want)
	}
}

func TestShowFormatsFileRefAsPath(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "spec.yaml")
	if err := os.WriteFile(path, []byte("openapi: 3.0.0\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	a := NewWithStore(&fakeStore{resolvedEntries: map[string]model.Entry{
		"SPEC": model.NewEntry(path, model.ValueTypeFileRef),
	}})

	lines, err := a.Show(context.Background(), "s1")
	if err != nil {
		t.Fatalf("Show error: %v", err)
	}
	want := "SPEC [path] " + path
	if strings.Join(lines, "\n") != want {
		t.Fatalf("Show = %v, want %q", lines, want)
	}
}

func TestShowPreviewsFirstLine(t *testing.T) {
	a := NewWithStore(&fakeStore{resolvedEntries: map[string]model.Entry{
		"DOC":  model.NewEntry("doc line\nhidden line", model.ValueTypeDoc),
		"TEXT": model.NewEntry("text line\nhidden line", model.ValueTypeString),
	}})

	lines, err := a.Show(context.Background(), "s1")
	if err != nil {
		t.Fatalf("Show error: %v", err)
	}
	got := strings.Join(lines, "\n")
	if strings.Contains(got, `\n`) || strings.Contains(got, "hidden line") {
		t.Fatalf("Show = %q, want first-line previews only", got)
	}
	if !strings.Contains(got, "DOC [doc]") || !strings.Contains(got, "doc line") || !strings.Contains(got, "TEXT [string] text line") {
		t.Fatalf("Show = %q, want doc and string previews", got)
	}
}

func TestRenderUsesInjectedStore(t *testing.T) {
	a := NewWithStore(&fakeStore{
		values:   map[string]string{"PROMPT": "Fix $ISSUE"},
		resolved: map[string]string{"ISSUE": "22"},
	})

	got, err := a.Render(context.Background(), "s1", "PROMPT", false)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	if got != "Fix 22" {
		t.Fatalf("Render = %q, want Fix 22", got)
	}
}

func TestRenderResolvesDocAndFileRefContent(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "fragment.txt")
	if err := os.WriteFile(path, []byte("file body"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	a := NewWithStore(&fakeStore{
		entries: map[string]model.Entry{
			"PROMPT": model.NewEntry("$DOC / $FILE", model.ValueTypeString),
		},
		resolvedEntries: map[string]model.Entry{
			"PROMPT": model.NewEntry("$DOC / $FILE", model.ValueTypeString),
			"DOC":    model.NewEntry("doc body", model.ValueTypeDoc),
			"FILE":   model.NewEntry(path, model.ValueTypeFileRef),
		},
	})

	got, err := a.Render(context.Background(), "s1", "PROMPT", false)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	if got != "doc body / file body" {
		t.Fatalf("Render = %q, want resolved typed content", got)
	}
}

func TestRenderIgnoreMissing(t *testing.T) {
	a := NewWithStore(&fakeStore{
		values:   map[string]string{"PROMPT": "Fix $ISSUE for $OWNER"},
		resolved: map[string]string{"ISSUE": "22"},
	})

	got, err := a.Render(context.Background(), "s1", "PROMPT", true)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	if got != "Fix 22 for $OWNER" {
		t.Fatalf("Render = %q, want missing placeholder preserved", got)
	}
}

func TestTriggerTemplatePathFindsExtension(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	triggerDir := filepath.Join(home, ".config", "ctx", "triggers")
	if err := os.MkdirAll(triggerDir, 0o755); err != nil {
		t.Fatalf("mkdir triggers: %v", err)
	}
	want := filepath.Join(triggerDir, "test.md")
	if err := os.WriteFile(want, []byte("command=echo\n---\nhello"), 0o644); err != nil {
		t.Fatalf("write trigger: %v", err)
	}

	got, err := triggerTemplatePath("test")
	if err != nil {
		t.Fatalf("triggerTemplatePath error: %v", err)
	}
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

func TestTriggerDefinitionMatchesTransition(t *testing.T) {
	def := TriggerDefinition{Key: "STATUS", Match: "DONE"}
	matches, err := def.Matches(TriggerChange{Key: "STATUS", OldValue: "PENDING", NewValue: "DONE"})
	if err != nil {
		t.Fatalf("Matches error: %v", err)
	}
	if !matches {
		t.Fatal("expected transition into matching value to fire")
	}

	matches, err = def.Matches(TriggerChange{Key: "STATUS", OldValue: "DONE", NewValue: "DONE"})
	if err != nil {
		t.Fatalf("Matches error: %v", err)
	}
	if matches {
		t.Fatal("expected already-matching value not to fire")
	}
}

func TestParseTriggerRejectsAnyChangeWithMatcher(t *testing.T) {
	_, err := parseTriggerDefinition("test.md", "any-change=true\nkey=STATUS\ncommand=echo\n---\nhello")
	if err == nil {
		t.Fatal("expected any-change with matcher to fail")
	}
}

func TestParseTriggerRequiresIntegerOrder(t *testing.T) {
	_, err := parseTriggerDefinition("test.md", "order=first\ncommand=echo\n---\nhello")
	if err == nil {
		t.Fatal("expected non-integer order to fail")
	}
}

func TestTriggerOrderDefaultsToZero(t *testing.T) {
	def, err := parseTriggerDefinition("test.md", "command=echo\n---\nhello")
	if err != nil {
		t.Fatalf("parseTriggerDefinition error: %v", err)
	}
	if def.Order != 0 {
		t.Fatalf("Order = %d, want 0", def.Order)
	}
}

func TestParseTriggerExecutionSession(t *testing.T) {
	def, err := parseTriggerDefinition("test.md", "execution-session=worker\ncommand=echo\n---\nhello")
	if err != nil {
		t.Fatalf("parseTriggerDefinition error: %v", err)
	}
	if def.ExecutionSession != "worker" {
		t.Fatalf("ExecutionSession = %q, want worker", def.ExecutionSession)
	}
}

func TestParseTriggerWithoutPromptSeparator(t *testing.T) {
	def, err := parseTriggerDefinition("test.md", "command=echo\nkey=STATUS")
	if err != nil {
		t.Fatalf("parseTriggerDefinition error: %v", err)
	}
	if def.PromptTemplate != "" {
		t.Fatalf("PromptTemplate = %q, want empty", def.PromptTemplate)
	}
}

func TestSetValueExecutesMatchingTrigger(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	triggerDir := filepath.Join(home, ".config", "ctx", "triggers")
	if err := os.MkdirAll(triggerDir, 0o755); err != nil {
		t.Fatalf("mkdir triggers: %v", err)
	}
	trigger := "key=STATUS\nmatch=DONE\ncommand=/bin/echo\n---\nStory $STORY"
	if err := os.WriteFile(filepath.Join(triggerDir, "done.md"), []byte(trigger), 0o644); err != nil {
		t.Fatalf("write trigger: %v", err)
	}

	fake := &fakeStore{
		values:   map[string]string{"STATUS": "PENDING"},
		resolved: map[string]string{"STORY": "ship it"},
	}
	a := NewWithStore(fake)

	if err := a.SetValue(context.Background(), "s1", "STATUS", "DONE"); err != nil {
		t.Fatalf("SetValue error: %v", err)
	}
	if fake.parentID == nil || *fake.parentID != "s1" {
		t.Fatalf("trigger execution parent = %v, want s1", fake.parentID)
	}

	foundLog := false
	for key, value := range fake.values {
		if strings.HasPrefix(key, "s1.trigger_log.") {
			foundLog = strings.Contains(value, `"trigger":"done"`) &&
				strings.Contains(value, "Story ship it") &&
				strings.Contains(value, `"execution_session":"`+fake.createdID+`"`)
			break
		}
	}
	if !foundLog {
		t.Fatalf("expected trigger log in values, got %#v", fake.values)
	}
}

func TestSetValueExecutesTriggerWithoutPromptArg(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	triggerDir := filepath.Join(home, ".config", "ctx", "triggers")
	if err := os.MkdirAll(triggerDir, 0o755); err != nil {
		t.Fatalf("mkdir triggers: %v", err)
	}

	scriptPath := filepath.Join(t.TempDir(), "capture-args.sh")
	script := "#!/bin/sh\nprintf '%s\\n%s\\n' \"$#\" \"$2\" > \"$1\"\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	outPath := filepath.Join(t.TempDir(), "args.txt")
	trigger := "key=STATUS\nmatch=DONE\ncommand=/bin/sh " + scriptPath + " " + outPath
	if err := os.WriteFile(filepath.Join(triggerDir, "capture.md"), []byte(trigger), 0o644); err != nil {
		t.Fatalf("write trigger: %v", err)
	}

	fake := &fakeStore{
		values: map[string]string{"STATUS": "PENDING"},
	}
	a := NewWithStore(fake)

	if err := a.SetValue(context.Background(), "s1", "STATUS", "DONE"); err != nil {
		t.Fatalf("SetValue error: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read args output: %v", err)
	}
	if got := string(data); got != "1\n\n" {
		t.Fatalf("args output = %q, want prompt omitted", got)
	}
}

func TestExecuteRunsTriggerWithoutPromptArg(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	triggerDir := filepath.Join(home, ".config", "ctx", "triggers")
	if err := os.MkdirAll(triggerDir, 0o755); err != nil {
		t.Fatalf("mkdir triggers: %v", err)
	}

	scriptPath := filepath.Join(t.TempDir(), "capture-args.sh")
	script := "#!/bin/sh\nprintf '%s\\n%s\\n' \"$#\" \"$2\" > \"$1\"\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	outPath := filepath.Join(t.TempDir(), "args.txt")
	trigger := "command=/bin/sh " + scriptPath + " " + outPath
	if err := os.WriteFile(filepath.Join(triggerDir, "manual.md"), []byte(trigger), 0o644); err != nil {
		t.Fatalf("write trigger: %v", err)
	}

	fake := &fakeStore{resolved: map[string]string{}}
	a := NewWithStore(fake)

	if err := a.Execute(context.Background(), "s1", "manual"); err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read args output: %v", err)
	}
	if got := string(data); got != "1\n\n" {
		t.Fatalf("args output = %q, want prompt omitted", got)
	}
}

func TestExecutePreservesQuotedCommandArg(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	triggerDir := filepath.Join(home, ".config", "ctx", "triggers")
	if err := os.MkdirAll(triggerDir, 0o755); err != nil {
		t.Fatalf("mkdir triggers: %v", err)
	}

	scriptPath := filepath.Join(t.TempDir(), "capture-args.sh")
	script := "#!/bin/sh\nprintf '%s\\n%s\\n' \"$#\" \"$2\" > \"$1\"\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	outPath := filepath.Join(t.TempDir(), "args.txt")
	trigger := "command=/bin/sh " + scriptPath + " " + outPath + ` "success 1"`
	if err := os.WriteFile(filepath.Join(triggerDir, "manual.md"), []byte(trigger), 0o644); err != nil {
		t.Fatalf("write trigger: %v", err)
	}

	fake := &fakeStore{resolved: map[string]string{}}
	a := NewWithStore(fake)

	if err := a.Execute(context.Background(), "s1", "manual"); err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read args output: %v", err)
	}
	if got := string(data); got != "2\nsuccess 1\n" {
		t.Fatalf("args output = %q, want quoted arg preserved", got)
	}
}

func TestExecuteRendersCommandPlaceholders(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	triggerDir := filepath.Join(home, ".config", "ctx", "triggers")
	if err := os.MkdirAll(triggerDir, 0o755); err != nil {
		t.Fatalf("mkdir triggers: %v", err)
	}

	scriptPath := filepath.Join(t.TempDir(), "capture-args.sh")
	script := "#!/bin/sh\nprintf '%s\\n%s\\n' \"$#\" \"$2\" > \"$1\"\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	outPath := filepath.Join(t.TempDir(), "args.txt")
	trigger := "command=/bin/sh " + scriptPath + " " + outPath + ` "$exe"`
	if err := os.WriteFile(filepath.Join(triggerDir, "manual.md"), []byte(trigger), 0o644); err != nil {
		t.Fatalf("write trigger: %v", err)
	}

	fake := &fakeStore{resolved: map[string]string{"exe": "go test"}}
	a := NewWithStore(fake)

	if err := a.Execute(context.Background(), "s1", "manual"); err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read args output: %v", err)
	}
	if got := string(data); got != "2\ngo test\n" {
		t.Fatalf("args output = %q, want command placeholder rendered", got)
	}
}

func TestSetValueTriggerPreservesQuotedCommandArg(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	triggerDir := filepath.Join(home, ".config", "ctx", "triggers")
	if err := os.MkdirAll(triggerDir, 0o755); err != nil {
		t.Fatalf("mkdir triggers: %v", err)
	}

	scriptPath := filepath.Join(t.TempDir(), "capture-args.sh")
	script := "#!/bin/sh\nprintf '%s\\n%s\\n' \"$#\" \"$2\" > \"$1\"\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	outPath := filepath.Join(t.TempDir(), "args.txt")
	trigger := "key=STATUS\nmatch=DONE\ncommand=/bin/sh " + scriptPath + " " + outPath + ` "success 1"`
	if err := os.WriteFile(filepath.Join(triggerDir, "capture.md"), []byte(trigger), 0o644); err != nil {
		t.Fatalf("write trigger: %v", err)
	}

	fake := &fakeStore{values: map[string]string{"STATUS": "PENDING"}}
	a := NewWithStore(fake)

	if err := a.SetValue(context.Background(), "s1", "STATUS", "DONE"); err != nil {
		t.Fatalf("SetValue error: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read args output: %v", err)
	}
	if got := string(data); got != "2\nsuccess 1\n" {
		t.Fatalf("args output = %q, want quoted arg preserved", got)
	}
}

func TestSetValueTriggerRendersCommandPlaceholders(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	triggerDir := filepath.Join(home, ".config", "ctx", "triggers")
	if err := os.MkdirAll(triggerDir, 0o755); err != nil {
		t.Fatalf("mkdir triggers: %v", err)
	}

	scriptPath := filepath.Join(t.TempDir(), "capture-args.sh")
	script := "#!/bin/sh\nprintf '%s\\n%s\\n' \"$#\" \"$2\" > \"$1\"\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	outPath := filepath.Join(t.TempDir(), "args.txt")
	trigger := "key=STATUS\nmatch=DONE\ncommand=/bin/sh " + scriptPath + " " + outPath + ` "$exe"`
	if err := os.WriteFile(filepath.Join(triggerDir, "capture.md"), []byte(trigger), 0o644); err != nil {
		t.Fatalf("write trigger: %v", err)
	}

	fake := &fakeStore{
		values:   map[string]string{"STATUS": "PENDING"},
		resolved: map[string]string{"exe": "go test"},
	}
	a := NewWithStore(fake)

	if err := a.SetValue(context.Background(), "s1", "STATUS", "DONE"); err != nil {
		t.Fatalf("SetValue error: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read args output: %v", err)
	}
	if got := string(data); got != "2\ngo test\n" {
		t.Fatalf("args output = %q, want command placeholder rendered", got)
	}
}

func TestSplitCommandLineRejectsUnterminatedQuote(t *testing.T) {
	_, err := splitCommandLine(`ctx set ctx test2 "success 1`)
	if err == nil {
		t.Fatal("expected unterminated quote to fail")
	}
}

func TestTriggerUsesExplicitExecutionSession(t *testing.T) {
	fake := &fakeStore{}
	a := NewWithStore(fake)
	session, err := a.executionSession(context.Background(), TriggerDefinition{ExecutionSession: "worker"}, TriggerChange{SessionID: "s1"})
	if err != nil {
		t.Fatalf("executionSession error: %v", err)
	}
	if session != "worker" {
		t.Fatalf("session = %q, want worker", session)
	}
	if fake.createdID != "" {
		t.Fatalf("unexpected child session creation: %q", fake.createdID)
	}
}

func TestTreeJSONFormat(t *testing.T) {
	a := NewWithStore(&fakeStore{nodes: []model.SessionNode{{ID: "root", Data: map[string]string{"K": "V"}}}})
	got, err := a.Tree(context.Background(), TreeFormatJSON)
	if err != nil {
		t.Fatalf("Tree error: %v", err)
	}
	if !strings.Contains(got, `"id": "root"`) {
		t.Fatalf("json tree output = %s", got)
	}
}

func TestStoreErrorsPropagate(t *testing.T) {
	want := errors.New("boom")
	a := NewWithStore(&fakeStore{err: want})
	if err := a.SetValue(context.Background(), "s1", "K", "V"); !errors.Is(err, want) {
		t.Fatalf("SetValue error = %v, want %v", err, want)
	}
}
