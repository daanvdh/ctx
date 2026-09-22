# ctx — Deterministic AI Workflows

[![CI](https://github.com/daanvdh/ctx/actions/workflows/ci.yml/badge.svg)](https://github.com/daanvdh/ctx/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/daanvdh/ctx)](https://github.com/daanvdh/ctx/releases)

**Do more with simpler models.**

Deterministic workflows should run deterministically. 
Today we let agents do everything, which inflates prompt and orchestration complexity, burns tokens, and sometimes 
forces a stronger model than the task actually needs.

***ctx is a key-value store with built-in shell scripting that triggers on context changes.***

ctx brings back control over the workflow while leaving the reasoning to your favorite harness. 
You select a task for execution; ctx picks up the changed state, creates the branch, pulls the task and project data, 
and constructs the prompt — all deterministically, no LLM needed. The harness receives a prompt it can reason from 
immediately, without a single tool call first.

Anything you can run from a shell, ctx can run deterministically and add to the context.

## Features

- Manage context by session.
- Reference any value inside any prompt, e.g. `$TASK_ID`.
- Run any script from ctx.
- Trigger any script on any state change.
- Write to ctx from external tools via its CLI or MCP.
- Call any harness with precise control over its main prompt.
- Collect harness output to chain the next trigger.

## Use Cases

- **Build-and-report loop** — A trigger runs your build on any code change, writes the full log to ctx as `BUILD_LOG`, and sets `STATUS=FAILED` when it breaks. All deterministic; nothing is asked of a model.
- **Auto-fix on failure** — A second trigger fires on `STATUS=FAILED`, greps the failing test out of `BUILD_LOG`, adds it to the prompt, and calls your harness to fix only that test. The harness starts already knowing what broke.
- **Multi-harness handoff** — Route each phase to the tool that's best at it: a reasoning-strong model plans and writes the steps to ctx, a tool-strong harness picks up that context and implements. ctx carries the state between them, so neither redoes the other's work.
- **Delegate tool-calling from a small model** — A local model that reasons well but handles tools poorly designs the run up front; ctx executes every tool call deterministically and leaves the model only the thinking.
- **Scheduled watch** — A cron-style trigger (`schedule`, fired by `ctx serve`'s built-in scheduler) polls a project on an interval and starts a run when something changes, instead of an agent sitting idle in a loop.
- **Write from anywhere** — CI, a git hook, or another tool writes to ctx via the CLI or MCP, and that state change fires the right downstream trigger. ctx becomes the shared state your tools coordinate through.

## Example: a configuration-only agent harness

[`examples/agent-harness`](examples/agent-harness) is a complete agent loop — plan → implement → verdict → report, with failure recovery — built from nothing but six trigger files and ctx state. It runs end-to-end with stub commands (no AI required) and swaps onto a real harness with one `ctx set`.

## Quick Install

```bash
# Prerequisite: Go 1.21+
git clone https://github.com/daanvdh/ctx.git
cd ctx
make build               # compiles the binary to ./bin/ctx
sudo mv ./bin/ctx /usr/local/bin/
```

Or, with a single command using `go install`:

```bash
go install github.com/daanvdh/ctx@latest
```

## Configuration

`ctx` is configured through a YAML settings file at `$HOME/.config/ctx/settings.yml` (created on first write; every setting has a working default, so only add the keys you need to change). It covers things like the database and trigger locations, default sessions, the HTTP MCP server's address and authentication, and pointing the CLI at a remote ctx backend. Keep the file private when it contains secrets — new settings files created by `ctx` use owner-only permissions.

For the full, up-to-date list of settings with defaults and examples, see [`skills/ctx-settings/SKILL.md`](skills/ctx-settings/SKILL.md).

## Core Commands

| Command | Synopsis | Description |
|---|---|---|
| `ctx session [parent] [name] [--parent <p>\|--root]` | `ctx session`<br>`ctx session my-name`<br>`ctx session parent-id my-name`<br>`ctx session --parent <parent-id>`<br>`ctx session --root`<br>`ctx session --help` | Create a new session. If a name is supplied, it is used (must consist of letters, digits, hyphens or underscores). Otherwise an 8‑character hexadecimal ID is generated. Parent defaults to the `CTX_ID` environment variable, or the tree root if unset; it can be overridden with a leading positional argument or `--parent`, or forced to the root with `--root`. `--parent`/`--root` and a positional parent are mutually exclusive. Use `--help` for usage information. |
| `ctx session rm <session> [--recursive]` | `ctx session rm $SID`<br>`ctx session rm $SID --recursive` | Delete a session and its variables. Fails if the session has child sessions unless `--recursive` is given, in which case the session and all descendants (and their variables) are deleted. |
| `ctx set [session] <key> <value>` | `ctx set $SID PROJECT_ID "myproj"`<br>`CTX_ID=$SID ctx set PROJECT_ID "myproj"` | Store a scalar string under *key* in the specified session (overwrites existing key). |
| `ctx set [session] <key> --path <path>` | `ctx set $SID API_SPEC --path ./openapi.yaml` | Store a reference to an existing local file. The path must exist when it is set. Reads resolve file content at use time. |
| `ctx rm [session] <entry>` | `ctx rm $SID PROJECT_ID`<br>`CTX_ID=$SID ctx rm PROJECT_ID` | Remove an entry from the specified session. |
| `ctx get [session] <key> [--raw\|--allow-missing]` | `ctx get $SID PROJECT_ID`<br>`ctx get $SID STORY --raw`<br>`ctx get $SID STORY --allow-missing` | Retrieve a visible value, searching the session, shared contexts, and then ancestors, substituting `$VAR` placeholders by default. `file_ref` values are never rendered, since they reference files living outside ctx that may contain `$`-syntax unintentionally. `--raw` returns the value unprocessed instead: the stored path for `file_ref`, or the unrendered value for `string`/`doc` — since it never renders, missing placeholders can't make it fail. `--allow-missing` renders but leaves unresolved `$VAR` placeholders unchanged instead of failing; mutually exclusive with `--raw`. |
| `ctx list [session] [--full] [--raw]` (alias: `ctx ls`) | `ctx list $SID`<br>`ctx list $SID --full`<br>`ctx list $SID --raw` | Print all visible keys with type tags, rendering `$VAR` placeholders by default. Strings and documents are shown as first-line previews unless `--full` shows the complete value; `--raw` skips rendering. Unlike `ctx get`, a listing never fails over one entry's unresolved placeholder — it's left unchanged instead. |
| `ctx export [session] [--include-docs] [--files-as-paths]` | `ctx export $SID`<br>`ctx export $SID --include-docs --files-as-paths` | Emit shell-compatible assignments, including `CTX_ID`. Plain export includes only strings; opt into documents and file-reference paths with flags. Use with `eval "$(ctx export …)"` or `env $(ctx export …) command`. |
| `ctx share <from> <to>` | `ctx share root worker` | Make keys from one session visible to another session before ancestor lookup. |
| `ctx trigger [session] <template>` (alias: `ctx execute`) | `ctx trigger $SID review` | Fire a trigger template from the trigger directory. The filename extension is optional. |
| `ctx tree [session_id] [-a] [--format text\|json]` | `ctx tree`<br>`ctx tree $SID`<br>`ctx tree -a` | Render the session hierarchy as an ASCII tree, showing ids and key/value pairs. Scoped to `session_id` (or `CTX_ID` if set) — its ancestors and descendants only. Use `-a`/`--all`, or omit both `session_id` and `CTX_ID`, to show the full tree of all sessions. |
| `ctx --version` | `ctx --version` | Print the build version. |
| `ctx help` | `ctx help` | Show a short usage summary (also shown when calling `ctx` without arguments). |

**Note:** If `CTX_ID` is set in the environment, commands that take a session can omit that argument. `ctx session` uses `CTX_ID` as the implicit parent unless a parent is given explicitly or `--root` is passed.

All commands exit with status 0 on success; error details are written to **stderr**.

## Value Types

Every ctx entry has a value type:

| Type | Set with | `ctx get` | Export behavior |
|---|---|---|---|
| `string` | `ctx set KEY value` | Stored value | Included by default |
| `file_ref` | `ctx set KEY --path ./path` | Current file content, unrendered | Omitted by default; include path with `--files-as-paths` |
| `file_bin` | Reserved | Not implemented | Not exported |

`ctx get` and `ctx list` resolve `string` values to content and substitute
`$VAR` placeholders before returning it. `file_ref` content is returned
as-is instead: it references a file living outside ctx that wasn't
authored with ctx's `$VAR` syntax in mind. If a referenced file no longer
exists, both fail with a clear error.

`string` values are limited to 500KB. Use `--path` for larger local files
or for content that should be read fresh each time.

## MCP Server

`ctx` includes an MCP server (proof of concept) that exposes the ctx API as MCP tools for clients such as Claude Desktop or OpenAI-compatible MCP hosts, over stdio or Streamable HTTP.

See [`MCP_SERVER.md`](MCP_SERVER.md) for setup, authentication, and the full tool list.

## Using `CTX_ID`

The environment variable `CTX_ID` can be used as the default session for commands and as the parent of a new session when you do **not** specify `--parent`. This is handy in scripts:

```bash
# Create a root session and export its ID.
ROOT=$(ctx session --root)    # prints e.g. "a1b2c3d4"
export CTX_ID=$ROOT           # make it available to subsequent commands

# Create a child session with a custom name; parent defaults to $CTX_ID.
CHILD=$(ctx session review-agent)

# Commands can now omit the session argument.
ctx set PROJECT_ID "gitlab-org/myproject"
ctx get PROJECT_ID
ctx list
```

If you need to override the implicit parent, use `--parent`:

```bash
export CTX_ID=$ROOT
CHILD=$(ctx session lint-agent --parent $ROOT)   # explicit parent flag takes precedence
```

Remember to **export** the variable; otherwise it is only set for a single command and will not be visible to subsequent `ctx session` invocations.

```bash
# Orchestrator creates a root session and a child.
ROOT=$(ctx session --root)    # => e.g. "5f2a1c9b"
CHILD=$(ctx session --parent $ROOT)

# Store data in the hierarchy.
ctx set $ROOT PROJECT_ID "gitlab-org/myproject"
ctx set $ROOT MR_IID "412"
ctx set $CHILD DISCUSSION_ID "abc123def456"

# Sub‑agent can import the whole context with a single command:
eval "$(ctx export $CHILD)"

# The variables are now available in the shell.
echo "$PROJECT_ID"   # gitlab-org/myproject
echo "$MR_IID"       # 412
echo "$DISCUSSION_ID" # abc123def456

# Bonus: reference files through context.
ctx set $ROOT REPORT --path /tmp/report.txt
ctx get $CHILD REPORT                  # prints the file content from the child's view, unrendered
ctx get $CHILD REPORT --raw            # prints /tmp/report.txt
```

## Trigger Templates & Writing Workflows

Manual templates and automatic triggers are YAML files in the trigger
directory: frontmatter (matchers, script, timeouts, chaining) plus a prompt
body. Full syntax, every field: [`skills/create-workflow/trigger-documentation.md`](skills/create-workflow/trigger-documentation.md).
For the design judgement behind a workflow — what one unit of work is,
whether it deserves its own trigger chain, how to name the entries it writes
— see the [`create-workflow`](skills/create-workflow/SKILL.md) skill, and
`examples/` for complete, working ones.

## Webhooks

`ctx serve --http` also serves `POST /webhooks/{source}`: a config-driven ingestion endpoint for GitHub, GitLab, Jira, or any other service that can send an HTTP webhook. Adding a source needs no code — drop an adapter file at `~/.config/ctx/webhooks/{source}.yaml`:

```yaml
# ~/.config/ctx/webhooks/github.yaml
verify: { type: hmac_sha256, header: X-Hub-Signature-256, secret_env: GITHUB_WEBHOOK_SECRET }
event_type: { header: X-GitHub-Event }
delivery_id: { header: X-GitHub-Delivery }
fields: { repo: repository.full_name, ref_id: issue.number }
session: github        # optional; defaults to the source name
```

- `verify` — how deliveries are authenticated: `hmac_sha256` (HMAC of the raw body, GitHub-style `sha256=<hex>` header), `shared_token` (header equals the secret), `bearer` (`Authorization: Bearer <secret>`), or `none`. The secret is read from the environment variable named by `secret_env`; invalid or unverifiable requests get a 401 and are never persisted.
- `event_type` / `delivery_id` — where to find the event name and the delivery's unique id, either `{ header: X }` or `{ field: dotted.json.path }`. Duplicate deliveries (same source + delivery id) are ignored.
- `fields` — extra values to pull out of the JSON payload, each stored as `WEBHOOK_<NAME>`.

A verified delivery writes `WEBHOOK_SOURCE`, `WEBHOOK_DELIVERY`, `WEBHOOK_PAYLOAD` (the raw body), one `WEBHOOK_<NAME>` per configured field, and finally `WEBHOOK_EVENT` into the session — and that last write fires triggers as usual. So routing an event to a script is just an ordinary trigger with `entries` filters (include `WEBHOOK_EVENT`, the key written last, so the trigger sees the complete event):

```yaml
trigger-session: github
entries:
  WEBHOOK_EVENT:
    - value: issues
  WEBHOOK_REPO:
    - value: daanvdh/ctx
script: handle-issue "$WEBHOOK_REF_ID"
```

Working adapter examples for GitHub, GitLab, and Jira — including where to register the webhook on each platform — are in [`examples/webhooks/`](examples/webhooks). As a fallback for missed deliveries (the dedupe cache is in-memory and cleared on restart), pair the webhook with a coarse `schedule:` trigger that reconciles state on an interval.

## `ctx tree` output example

```text
5f2a1c9b
 PROJECT_ID: gitlab-org/myproject
 MR_IID: 412
├── 8e7d3a4f
│     DISCUSSION_ID: abc123def456
└── a1b2c3d4
      REPORT: [path] /tmp/report.txt
      STORY: Fix issue 45 ...
```

The tree displays sessions sorted alphabetically, with child nodes indented.
Entries belonging to a session are listed directly beneath its ID. Only
`file_ref` entries are tagged (`[path]`); other types show no tag. Strings
are shown as first-line previews, and file references are shown as paths.

## Design Highlights

- **SQLite backend** – guarantees atomic writes and handles concurrent reads/writes without external locking.
- **Hierarchical lookup** – `ctx get` walks up the parent chain (max 50 hops) so children automatically inherit keys from ancestors. Later entries shadow earlier ones.
- **Typed values** – scalar strings and local file references resolve consistently through `ctx get` and `ctx list`.
- **Session IDs** – generated with `crypto/rand`, yielding an eight‑character lowercase hex string (`xxxxxxxx`). Collisions are extremely unlikely.
- **Depth limit** – prevents infinite loops in corrupted data (e.g., circular parent references).
- **Portable** – No external dependencies; the only required runtime is the SQLite driver bundled via Go modules.

## Development

```bash
make build   # build ./bin/ctx, stamped with the version from VERSION
make test    # run unit tests
make lint    # static analysis with golangci‑lint
make clean   # remove ./bin
```

To cut a release, bump [`VERSION`](VERSION) and add an entry to
[`CHANGELOG.md`](CHANGELOG.md).

Contributions are welcome. Please open an issue or a pull request for bugs,
features, or documentation improvements.

---

`ctx` – Simple, deterministic context handling for multi‑agent AI workflows.
