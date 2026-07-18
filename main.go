package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"ctx/cmd"
)

var version = "dev"

// subcommand wraps an existing cmd.Command function as a cobra command.
// Flag parsing stays disabled: every subcommand parses its own args exactly
// as before, so behavior, flags and help output are unchanged; cobra only
// provides dispatch, aliases and shell completion.
func subcommand(name string, aliases []string, hidden bool, run func(context.Context, []string) error) *cobra.Command {
	return &cobra.Command{
		Use:                name,
		Aliases:            aliases,
		Hidden:             hidden,
		DisableFlagParsing: true,
		RunE: func(c *cobra.Command, args []string) error {
			if err := run(c.Context(), args); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			return nil
		},
	}
}

func main() {
	root := &cobra.Command{
		Use:           "ctx",
		Version:       version,
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help(c.Context(), nil)
			}
			return fmt.Errorf("unknown command: %s", args[0])
		},
	}
	root.SetVersionTemplate("{{.Version}}\n")
	// ctx's help is the hand-written overview in cmd.Help; per-command
	// detail lives behind each subcommand's own --help flag.
	root.SetHelpFunc(func(c *cobra.Command, _ []string) {
		if err := cmd.Help(c.Context(), nil); err != nil {
			fmt.Fprintf(os.Stderr, "ctx: help: %v\n", err)
		}
	})

	root.AddCommand(
		subcommand("session", nil, false, cmd.Session),
		subcommand("set", nil, false, cmd.Set),
		subcommand("rm", nil, false, cmd.Rm),
		subcommand("get", nil, false, cmd.Get),
		subcommand("export", nil, false, cmd.Export),
		subcommand("list", []string{"ls"}, false, cmd.List),
		subcommand("share", nil, false, cmd.Share),
		subcommand("tree", nil, false, cmd.Tree),
		subcommand("trigger", []string{"execute"}, false, cmd.Trigger),
		subcommand("serve", nil, false, cmd.Serve),
		subcommand("fire-triggers", nil, true, cmd.FireTriggers),
		&cobra.Command{
			Use:  "version",
			Args: cobra.NoArgs,
			Run:  func(*cobra.Command, []string) { fmt.Println(version) },
		},
		&cobra.Command{
			Use:  "help",
			RunE: func(c *cobra.Command, _ []string) error { return cmd.Help(c.Context(), nil) },
		},
	)

	if err := root.ExecuteContext(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "ctx: %v\n", err)
		os.Exit(1)
	}
}
