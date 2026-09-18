// Package trigger loads, parses and matches trigger definitions: YAML
// frontmatter files in the trigger directory that bind a script to session
// changes, schedules or manual firing. Executing the matched triggers is
// internal/app's job; this package is pure definition handling so it can be
// tested (and evolved) in isolation.
package trigger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"ctx/internal/config"
	"ctx/internal/render"
	"gopkg.in/yaml.v3"
)

// Change describes a session write that may fire triggers.
type Change struct {
	SessionID string
	Key       string
	OldValue  string
	NewValue  string
	// Depth is the trigger chain nesting level of the write that caused
	// this change; writes made by a fired trigger carry Depth+1.
	Depth int
}

// Definition holds a parsed trigger file.
type Definition struct {
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
	FailureEntry     string
	Timeout          time.Duration // zero = unbounded
}

// entryValue is a single accepted value within an entries list.
type entryValue struct {
	Value string `yaml:"value"`
}

// fileData is the YAML structure for the trigger frontmatter.
type fileData struct {
	Script           string                  `yaml:"script"`
	TriggerSession   string                  `yaml:"trigger-session"`
	Ancestor         string                  `yaml:"ancestor"`
	AnyChange        bool                    `yaml:"any-change"`
	Order            int                     `yaml:"order"`
	ExecutionSession string                  `yaml:"execution-session"`
	Entries          map[string][]entryValue `yaml:"entries"`
	Schedule         string                  `yaml:"schedule"`
	Logging          bool                    `yaml:"logging"`
	OutputEntry      string                  `yaml:"output-entry"`
	FailureEntry     string                  `yaml:"failure-entry"`
	Timeout          string                  `yaml:"timeout"`
}

// LoadAll parses every file in the configured trigger directory, sorted by
// path. A missing directory yields no definitions and no error.
func LoadAll() ([]Definition, error) {
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

	defs := make([]Definition, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(triggerDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read trigger %s: %w", path, err)
		}
		def, err := Parse(path, string(data))
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

// Parse turns a trigger file's content into a Definition: YAML frontmatter,
// optionally followed by "---" and a prompt template body. path is used for
// the definition's name and error messages.
func Parse(path, content string) (Definition, error) {
	var frontmatter, promptTemplate string
	if i := strings.Index(content, "\n---\n"); i >= 0 {
		frontmatter = content[:i]
		promptTemplate = content[i+5:]
	} else {
		frontmatter = content
	}

	var data fileData
	dec := yaml.NewDecoder(strings.NewReader(frontmatter))
	dec.KnownFields(true)
	if err := dec.Decode(&data); err != nil && err != io.EOF {
		return Definition{}, fmt.Errorf("malformed trigger %s: %w", path, err)
	}

	if data.Script == "" {
		return Definition{}, fmt.Errorf("malformed trigger %s: missing script", path)
	}
	if data.AnyChange && (data.TriggerSession != "" || data.Ancestor != "" || len(data.Entries) > 0) {
		return Definition{}, fmt.Errorf("malformed trigger %s: any-change cannot be combined with trigger-session, ancestor, or entries", path)
	}
	hasMatchers := data.TriggerSession != "" || data.Ancestor != "" || len(data.Entries) > 0
	if data.Schedule != "" && data.AnyChange {
		return Definition{}, fmt.Errorf("malformed trigger %s: schedule cannot be combined with any-change", path)
	}
	if data.Schedule != "" && !hasMatchers && data.ExecutionSession == "" {
		return Definition{}, fmt.Errorf("malformed trigger %s: schedule without filters requires execution-session to be set", path)
	}
	if data.Schedule != "" && !hasMatchers && strings.Contains(data.ExecutionSession, "$") {
		return Definition{}, fmt.Errorf("malformed trigger %s: a schedule-driven trigger without filters has no triggering session to resolve $VAR from; execution-session must be literal", path)
	}

	var timeout time.Duration
	if data.Timeout != "" {
		d, err := time.ParseDuration(data.Timeout)
		if err != nil || d <= 0 {
			return Definition{}, fmt.Errorf("malformed trigger %s: invalid timeout %q (want a positive Go duration like 10m)", path, data.Timeout)
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

	return Definition{
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
		FailureEntry:     data.FailureEntry,
		Timeout:          timeout,
	}, nil
}

// varMarkerRe matches a "<!-- ctx:var NAME -->" block marker.
var varMarkerRe = regexp.MustCompile(`<!--\s*ctx:var\s+([A-Za-z_][A-Za-z0-9_]*)\s*-->`)

// ParseVars splits a trigger body into named variable blocks, delimited by
// "<!-- ctx:var NAME -->" markers: each marker starts a new block running
// until the next marker (or EOF), with surrounding blank lines trimmed.
// Content before the first marker (or the whole body, if there are no
// markers) is assigned to CTX_TRIGGER_PROMPT.
func ParseVars(body string) map[string]string {
	matches := varMarkerRe.FindAllStringSubmatchIndex(body, -1)
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

// RenderVars parses body into named variable blocks (see ParseVars) and
// renders each block's $VAR placeholders against vars.
func RenderVars(body string, vars map[string]string) (map[string]string, error) {
	blocks := ParseVars(body)
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
func (d Definition) Matches(change Change, vars map[string]string, ancestors map[string]bool) (bool, error) {
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

// HasMatchers reports whether any session/entry filter is set.
func (d Definition) HasMatchers() bool {
	return d.TriggerSession != "" || d.Ancestor != "" || len(d.Entries) > 0
}

// MatchesState reports whether a session's current state satisfies this
// trigger's filters, with no write involved — the schedule-tick counterpart
// of Matches. An entries key with no values requires the key to be visible;
// with values, the current value must equal one of them.
func (d Definition) MatchesState(sessionID string, vars map[string]string, ancestors map[string]bool) bool {
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

// MatchesSchedule reports whether t falls within expr, a standard 5-field
// cron expression (minute hour day-of-month month day-of-week), the same
// format used by crontab(5), Kubernetes CronJob and GitHub Actions.
//
// ponytail: supports "*", exact values, comma-lists and "*/step"; no
// hyphen ranges or named months/weekdays. Add if a trigger needs them.
func MatchesSchedule(expr string, t time.Time) (bool, error) {
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

// RenderDefinitionVars renders $VAR placeholders in the frontmatter fields
// that name runtime targets (execution-session, output-entry, failure-entry)
// against the triggering session's resolved values. Matcher fields stay
// literal.
func RenderDefinitionVars(def Definition, vars map[string]string) (Definition, error) {
	for _, field := range []*string{&def.ExecutionSession, &def.OutputEntry, &def.FailureEntry} {
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
