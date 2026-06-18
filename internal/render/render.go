package render

import (
    "fmt"
    "regexp"

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

    // Use a regular expression to find $VAR patterns. Supports identifiers that start with a letter or underscore,
    // followed by letters, digits, or underscores.
    re := regexp.MustCompile(`\$(?P<var>[A-Za-z_][A-Za-z0-9_]*)`)

    var missing []string
    result := re.ReplaceAllStringFunc(tmpl, func(m string) string {
        // m is of form "$VAR"
        varName := m[1:] // strip the leading '$'
        if val, ok := resolved[varName]; ok {
            return val
        }
        missing = append(missing, varName)
        // Return empty string for unresolved placeholders.
        return ""
    })

    if len(missing) > 0 {
        // Report the first missing placeholder for brevity.
        return "", fmt.Errorf("missing values for placeholders: %v", missing)
    }

    return result, nil
}
