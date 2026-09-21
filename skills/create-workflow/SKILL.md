---
name: create-workflow
description: Design a ctx workflow (trigger files chaining a deterministic script with the prompt(s) it hands an agent) before writing one. Use when asked to automate a task with ctx, write or edit a ctx trigger, turn a manual process into a trigger, fan out over a set of items with ctx, or when opening an existing trigger file to change it.
---

# Writing a ctx workflow

A workflow is one or more ctx trigger files: a script (deterministic) plus the
prompt(s) it hands an agent, chained by writes. Trigger syntax lives in
[trigger-documentation.md](trigger-documentation.md), read both before writing 
anything.

Agent work should be organized in sessions, possibly containing child sessions 
if it's a multistep process. Any data that should be passed to the agent should 
be put in the session context using `ctx set [session] <key> <value>`. These values 
should be inserted in the prompt using `My value is $<key>`. 

If it's unclear what one unit of work is, where the list of units comes from,
or what happens when one fails, ask before proposing a design; otherwise go
straight to one.

| Command                                                   | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
|-----------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `ctx session [parent] [name] [--parent <p>\|--root]`      | Create a new session. If a name is supplied, it is used (must consist of letters, digits, hyphens or underscores). Otherwise an 8‑character hexadecimal ID is generated. Parent defaults to the `CTX_ID` environment variable, or the tree root if unset; it can be overridden with a leading positional argument or `--parent`, or forced to the root with `--root`. `--parent`/`--root` and a positional parent are mutually exclusive. Use `--help` for usage information.                                                                                                                       |
| `ctx set [session] <key> <value>`                         | Store a scalar string under *key* in the specified session (overwrites existing key).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| `ctx set [session] <key> --path <path>`                   | Store a reference to an existing local file. The path must exist when it is set. Reads resolve file content at use time.                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| `ctx get [session] <key> [--raw\|--allow-missing]`        | Retrieve a visible value, searching the session, shared contexts, and then ancestors, substituting `$VAR` placeholders by default. `file_ref` values are never rendered, since they reference files living outside ctx that may contain `$`-syntax unintentionally. `--raw` returns the value unprocessed instead: the stored path for `file_ref`, or the unrendered value for `string`/`doc` — since it never renders, missing placeholders can't make it fail. `--allow-missing` renders but leaves unresolved `$VAR` placeholders unchanged instead of failing; mutually exclusive with `--raw`. |
| `ctx trigger [session] <template>` (alias: `ctx execute`) | Fire a trigger template from the trigger directory. The filename extension is optional.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `CTX_ID=<session>`                                        | Set the session ID for the following commands. Set it to prevent having to provide the session each command.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |

## Splitting workflows

Default to one workflow, however many trigger files it takes to chain its
steps. Split into separate workflows, one triggering the next, only when a
piece would make sense run standalone outside this task — "run the tests" is
useful without "write the code"; step 2 of a 5-step pipeline usually isn't.

Name sessions and entries functionally, in ctx's own casing convention
(`TASK`, `REVIEW`), not by position, type, or general code style — so
`ctx list`/`ctx tree` reads like the task when someone is debugging it.
