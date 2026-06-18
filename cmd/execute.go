// Package cmd implements the command line interface for ctx.
// This file adds support for executing a template stored in the triggers directory.

package cmd

import (
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "regexp"
    "strconv"
    "strings"

    "ctx/internal/store"
    "ctx/internal/session"
)

// Execute handles the `ctx execute <session> <template>` command.
// It loads a template file from $HOME/.config/ctx/triggers/<template>,
// substitutes placeholders ($VAR) using session variables (including ancestors),
// and runs the specified command with the rendered prompt as a quoted argument.
func Execute(args []string) int {
    // Show help if requested.
    for _, a := range args {
        if a == "--help" || a == "-h" {
            fmt.Println(`Usage: ctx execute <session> <template>

Execute a stored trigger template. The template must be placed in $HOME/.config/ctx/triggers/<template>.
The first line should define the command to run, e.g.:`)
            fmt.Println("    command=pi        # Program invoked with the rendered prompt.")
            fmt.Println(`---
<prompt text possibly containing placeholders like $VAR>`)
            return 0
        }
    }

    if len(args) != 2 {
        fmt.Fprintf(os.Stderr, "ctx: execute: usage: ctx execute <session> <template>\n")
        return 1
    }
    sessionID := args[0]
    tmplName := args[1]

    // Determine paths.
    home, err := os.UserHomeDir()
    if err != nil {
        fmt.Fprintf(os.Stderr, "ctx: execute: %v\n", err)
        return 1
    }
    triggersPath := filepath.Join(home, ".config", "ctx", "triggers", tmplName)
    data, err := os.ReadFile(triggersPath)
    if err != nil {
        fmt.Fprintf(os.Stderr, "ctx: execute: failed to read template %s: %v\n", triggersPath, err)
        return 1
    }
    content := string(data)

    // Split header and prompt using the '---' separator.
    parts := strings.SplitN(content, "\n---\n", 2)
    if len(parts) != 2 {
        // Try a less strict split (in case there is no surrounding newlines).
        parts = strings.SplitN(content, "---", 2)
    }
    if len(parts) != 2 {
        fmt.Fprintf(os.Stderr, "ctx: execute: malformed template – missing '---' separator\n")
        return 1
    }
    header := parts[0]
    promptTemplate := parts[1]

    // Extract command from the first non‑empty line of the header.
    var cmdLine string
    for _, line := range strings.Split(header, "\n") {
        trimmed := strings.TrimSpace(line)
        if trimmed == "" {
            continue
        }
        cmdLine = trimmed
        break
    }
    if !strings.HasPrefix(cmdLine, "command=") {
        fmt.Fprintf(os.Stderr, "ctx: execute: missing 'command=' definition in template\n")
        return 1
    }
    cmdStr := strings.TrimSpace(strings.TrimPrefix(cmdLine, "command="))
    // Strip any trailing comment.
    if i := strings.Index(cmdStr, "#"); i != -1 {
        cmdStr = strings.TrimSpace(cmdStr[:i])
    }
    if cmdStr == "" {
        fmt.Fprintf(os.Stderr, "ctx: execute: empty command in template\n")
        return 1
    }

    // Load the context database.
    dbPath, err := getCtxPath()
    if err != nil {
        fmt.Fprintf(os.Stderr, "ctx: execute: %v\n", err)
        return 1
    }
    cf, err := store.Load(dbPath)
    if err != nil {
        fmt.Fprintf(os.Stderr, "ctx: execute: failed to load context file: %v\n", err)
        return 1
    }
    // Ensure the session exists.
    if _, ok := cf.Sessions[sessionID]; !ok {
        fmt.Fprintf(os.Stderr, "ctx: execute: session %s not found\n", sessionID)
        return 1
    }

    // Resolve placeholders using existing logic (similar to render.Render).
    vars, err := session.Resolve(cf, sessionID)
    if err != nil {
        fmt.Fprintf(os.Stderr, "ctx: execute: %v\n", err)
        return 1
    }
    re := regexp.MustCompile(`\$(?P<var>[A-Za-z_][A-Za-z0-9_]*)`)
    var missing []string
    renderedPrompt := re.ReplaceAllStringFunc(promptTemplate, func(m string) string {
        name := m[1:] // strip leading $
        if v, ok := vars[name]; ok {
            return v
        }
        missing = append(missing, name)
        return ""
    })
    if len(missing) > 0 {
        fmt.Fprintf(os.Stderr, "ctx: execute: missing values for placeholders: %v\n", missing)
        return 1
    }

    // Build the final command line. The prompt must be quoted.
    // Use the system shell to honour quoting semantics.
    fullCmd := fmt.Sprintf("%s %s", cmdStr, strconv.Quote(renderedPrompt))
    execCmd := exec.Command("sh", "-c", fullCmd)
    execCmd.Stdout = os.Stdout
    execCmd.Stderr = os.Stderr
    if err := execCmd.Run(); err != nil {
        fmt.Fprintf(os.Stderr, "ctx: execute: command execution failed: %v\n", err)
        return 1
    }
    return 0
}
