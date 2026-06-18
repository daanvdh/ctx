package cmd

import (
	"fmt"
	"os"

	"ctx/internal/app"
)

func newApp(command string) (*app.App, int) {
	a, err := app.New()
	if err != nil {
		printErr(command, err)
		return nil, 1
	}
	return a, 0
}

func printErr(command string, err error) {
	fmt.Fprintf(os.Stderr, "ctx: %s: %v\n", command, err)
}
