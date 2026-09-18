// Package render turns stored context into output: recursive $VAR template
// substitution and session tree rendering.
package render

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"ctx/internal/model"
	"ctx/internal/session"
)

// Render resolves placeholders in the value of a given key within a session.
// It fetches the raw template string stored under `key` (searching up the parent chain),
// then replaces occurrences of `$VAR_NAME` with the values resolved for the session (including ancestors).
// If any placeholder cannot be resolved, an error is returned.
func Render(cf *model.ContextFile, sessionID string, key string) (string, error) {
	// Retrieve the raw template value using session.Get, which searches up the hierarchy.
	tmpl, err := session.Get(cf, sessionID, key)
	if err != nil {
		return "", err
	}

	// Resolve all visible variables for this session (including ancestors).
	resolved, err := session.Resolve(cf, sessionID)
	if err != nil {
		return "", err
	}

	return TemplateString(tmpl, resolved)
}

func TemplateString(tmpl string, resolved map[string]string) (string, error) {
	return TemplateStringWithOptions(tmpl, resolved, TemplateOptions{})
}

// MaxRenderDepth caps recursive placeholder expansion so a cyclical or
// self-referential value can't loop forever.
const MaxRenderDepth = 10

// TemplateStringRecursive repeatedly substitutes $VAR placeholders, so a
// resolved value that itself contains placeholders gets expanded too, up to
// maxDepth passes or until a pass produces no further change.
func TemplateStringRecursive(tmpl string, resolved map[string]string, opts TemplateOptions, maxDepth int) (string, error) {
	out := tmpl
	for i := 0; i < maxDepth; i++ {
		next, err := TemplateStringWithOptions(out, resolved, opts)
		if err != nil {
			return "", err
		}
		if next == out {
			return next, nil
		}
		out = next
	}
	return out, nil
}

type TemplateOptions struct {
	AllowMissing bool
}

func TemplateStringWithOptions(tmpl string, resolved map[string]string, opts TemplateOptions) (string, error) {
	var out strings.Builder
	missing := map[string]struct{}{}

	for i := 0; i < len(tmpl); {
		if tmpl[i] != '$' {
			out.WriteByte(tmpl[i])
			i++
			continue
		}

		if i+1 < len(tmpl) && tmpl[i+1] == '$' {
			if name, end, ok := readVarName(tmpl, i+2); ok {
				out.WriteByte('$')
				out.WriteString(name)
				i = end
				continue
			}
			out.WriteByte('$')
			i += 2
			continue
		}

		name, end, ok := readVarName(tmpl, i+1)
		if !ok {
			out.WriteByte('$')
			i++
			continue
		}

		value, ok := resolved[name]
		if !ok {
			if opts.AllowMissing {
				out.WriteByte('$')
				out.WriteString(name)
			} else {
				missing[name] = struct{}{}
			}
			i = end
			continue
		}
		out.WriteString(value)
		i = end
	}

	if len(missing) > 0 {
		names := make([]string, 0, len(missing))
		for name := range missing {
			names = append(names, name)
		}
		sort.Strings(names)
		return "", fmt.Errorf("missing values for placeholders: %v", names)
	}

	return out.String(), nil
}

func readVarName(s string, start int) (string, int, bool) {
	if start >= len(s) {
		return "", start, false
	}

	r, size := rune(s[start]), 1
	if r >= utf8.RuneSelf {
		r, size = utf8.DecodeRuneInString(s[start:])
	}
	if !isVarStart(r) {
		return "", start, false
	}

	end := start + size
	for end < len(s) {
		r, size = rune(s[end]), 1
		if r >= utf8.RuneSelf {
			r, size = utf8.DecodeRuneInString(s[end:])
		}
		if !isVarPart(r) {
			break
		}
		end += size
	}

	return s[start:end], end, true
}

// RewriteVars replaces each $VAR_NAME placeholder in tmpl with $N, where N is
// looked up in indices (1-based positional parameter index). Names absent
// from indices are left untouched. "$$" escaping rules match TemplateString.
func RewriteVars(tmpl string, indices map[string]int) string {
	var out strings.Builder

	for i := 0; i < len(tmpl); {
		if tmpl[i] != '$' {
			out.WriteByte(tmpl[i])
			i++
			continue
		}

		if i+1 < len(tmpl) && tmpl[i+1] == '$' {
			if name, end, ok := readVarName(tmpl, i+2); ok {
				out.WriteByte('$')
				out.WriteString(name)
				i = end
				continue
			}
			out.WriteByte('$')
			i += 2
			continue
		}

		name, end, ok := readVarName(tmpl, i+1)
		if !ok {
			out.WriteByte('$')
			i++
			continue
		}

		idx, ok := indices[name]
		if !ok {
			out.WriteByte('$')
			out.WriteString(name)
			i = end
			continue
		}
		fmt.Fprintf(&out, "$%d", idx)
		i = end
	}

	return out.String()
}

// ExtractVarNames scans tmpl for $VAR_NAME placeholders and returns the
// unique names in order of first appearance. "$$" escapes are skipped, matching
// TemplateString's escaping rules.
func ExtractVarNames(tmpl string) []string {
	var names []string
	seen := map[string]struct{}{}

	for i := 0; i < len(tmpl); {
		if tmpl[i] != '$' {
			i++
			continue
		}
		if i+1 < len(tmpl) && tmpl[i+1] == '$' {
			i += 2
			continue
		}
		name, end, ok := readVarName(tmpl, i+1)
		if !ok {
			i++
			continue
		}
		if _, ok := seen[name]; !ok {
			seen[name] = struct{}{}
			names = append(names, name)
		}
		i = end
	}

	return names
}

func isVarStart(r rune) bool {
	return r == '_' || unicode.IsLetter(r)
}

func isVarPart(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}
