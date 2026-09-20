---
name: create-workflow
description: Design a ctx workflow (trigger files chaining a deterministic script with the prompt(s) it hands an agent) before writing one. Use when asked to automate a task with ctx, write or edit a ctx trigger, turn a manual process into a trigger, fan out over a set of items with ctx, or when opening an existing trigger file to change it.
---

# Writing a ctx workflow

A workflow is one or more ctx trigger files: a script (deterministic) plus the
prompt(s) it hands an agent, chained by writes. Trigger syntax lives in
[trigger-documentation.md](trigger-documentation.md), next to this file, and
in `examples/` — read both before writing anything.

If it's unclear what one unit of work is, where the list of units comes from,
or what happens when one fails, ask before proposing a design; otherwise go
straight to one.

## Splitting workflows

Default to one workflow, however many trigger files it takes to chain its
steps. Split into separate workflows, one triggering the next, only when a
piece would make sense run standalone outside this task — "run the tests" is
useful without "write the code"; step 2 of a 5-step pipeline usually isn't.

Name sessions and entries functionally, in ctx's own casing convention
(`TASK`, `REVIEW`), not by position, type, or general code style — so
`ctx list`/`ctx tree` reads like the task when someone is debugging it.
