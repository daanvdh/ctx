package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"ctx/internal/session"
	"ctx/internal/store"
)

func Export(args []string) int {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "ctx: export: unexpected error: %v\n", r)
			os.Exit(1)
		}
	}()

	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "ctx: export: usage: ctx export <session_id>\n")
		return 1
	}

	sessionID := args[0]

	path, err := getCtxPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ctx: export: %v\n", err)
		return 1
	}

	cf, err := store.Load(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ctx: export: %v\n", err)
		return 1
	}

	resolved, err := session.Resolve(cf, sessionID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ctx: export: %v\n", err)
		return 1
	}

	for _, key := range keys(resolved) {
		value := singleQuote(resolved[key])
		fmt.Printf("export %s=%s\n", key, value)
	}

	return 0
}

func keys(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func singleQuote(v string) string {
	return "'" + strings.ReplaceAll(v, "'", "'\\''") + "'"
}
