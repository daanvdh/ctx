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

type TemplateOptions struct {
	IgnoreMissing bool
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
			if opts.IgnoreMissing {
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

func isVarStart(r rune) bool {
	return r == '_' || unicode.IsLetter(r)
}

func isVarPart(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}
