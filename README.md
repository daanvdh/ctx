# ctx — Deterministic AI Workflows

[![CI](https://github.com/daanvdh/ctx/actions/workflows/ci.yml/badge.svg)](https://github.com/daanvdh/ctx/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/daanvdh/ctx)](https://github.com/daanvdh/ctx/releases)

**Do more with simpler models.**

Deterministic workflows should run deterministically. Today we let agents do everything, which inflates prompt and orchestration complexity, burns tokens, and sometimes forces a stronger model than the task actually needs.

***ctx is a key-value store with built-in shell scripting that triggers on context changes.***

ctx brings back control over the workflow while leaving the reasoning to your favorite harness. You select a task for execution; ctx picks up the changed state, creates the branch, pulls the task and project data, and constructs the prompt — all deterministically, no LLM needed. The harness receives a prompt it can reason from immediately, without a single tool call first.

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

- By default the database is stored at `$HOME/.config/ctx/ctx.sqlite`. This location can be changed by creating a YAML settings file (`$HOME/.config/ctx/settings.yml`) with a `db_path` field, for example:
  ```yaml
  db_path: /tmp/my‑ctx.db
  ```
- Trigger templates live in `$HOME/.config/ctx/triggers` by default. Set `trigger_location` in `settings.yml` to use a different directory.
- HTTP MCP defaults and authentication can also be configured in `settings.yml`:
  ```yaml
  mcp_http_addr: 127.0.0.1:7331
  mcp_http_path: /ctx-mcp
  mcp_server_name: ctx
  mcp_oauth_client_id: claude
  mcp_oauth_client_secret: long-random-secret
  ```
  Keep this file private when it contains secrets. New settings files created by `ctx` use owner-only permissions.
- To use a remote ctx MCP server as the backend instead of a local sqlite db, set `remote_mcp_url` (and `remote_mcp_token` if the server requires a bearer token) in `settings.yml`:
  ```yaml
  remote_mcp_url: http://ctx-host:7331/mcp
  remote_mcp_token: long-random-secret
  ```
  Every `ctx` command then talks to that server's `tools/call` endpoint instead of a local db. `ctx rm` and `ctx set --path` are not supported yet over a remote backend (the MCP protocol has no tool for them).

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
| `ctx execute [session] <template>` | `ctx execute $SID review` | Execute a trigger template from the trigger directory. The filename extension is optional. |
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

## MCP Server POC

`ctx` includes an MCP server proof of concept that exposes the ctx API as MCP
tools for clients such as Claude Desktop or OpenAI-compatible MCP hosts.

`ctx serve` has three modes: `--stdio` (MCP over stdio, no scheduler —
ephemeral, spawned per session by IDEs), `--http` (MCP over Streamable HTTP,
scheduler always on), and no flag at all (scheduler only, no MCP surface —
for running `ctx` purely to fire `schedule`-bearing triggers). `--http` and
`--stdio` are mutually exclusive. See [Trigger Templates](#trigger-templates)
below for `schedule`-bearing triggers.

Build it:

```bash
make build
```

Example client configuration:

```json
{
  "mcpServers": {
    "ctx": {
      "command": "/absolute/path/to/ctx/bin/ctx",
      "args": ["serve", "--stdio"]
    }
  }
}
```

For clients that require a remote MCP URL, run the same binary in Streamable
HTTP mode and expose it through HTTPS:

```bash
make build
./bin/ctx serve --http
```

In another terminal, publish the local server with a tunnel:

```bash
tailscale funnel --bg 7331
```

Use the HTTPS forwarding URL with `/ctx-mcp` appended as the remote MCP server URL,
for example:

```text
https://your-mac.your-tailnet.ts.net/ctx-mcp
```

If `mcp_oauth_client_id` and `mcp_oauth_client_secret` are configured, HTTP MCP
requests require authorization. `ctx serve --http` exposes the MCP endpoint and
the minimal OAuth authorization endpoints in the same HTTP process. Use the
configured client id and secret in clients such as Claude Desktop. The server
issues opaque bearer tokens after an authorization-code flow with S256 PKCE and
then requires `Authorization: Bearer <token>` on MCP requests.

For simple clients that can send bearer tokens directly, set `mcp_token` in
`settings.yml` or `CTX_MCP_TOKEN` in the environment. OAuth credentials can also
be provided with `CTX_MCP_CLIENT_ID` and `CTX_MCP_CLIENT_SECRET`; environment
values override settings.

When running behind a tunnel or reverse proxy, `ctx` infers its public URL from
forwarding headers. If the proxy does not provide them, set `mcp_public_url` to
the external origin, for example `https://your-mac.your-tailnet.ts.net`.

Publish the whole local HTTP server through the tunnel, not only the MCP path.
OAuth clients need both `/ctx-mcp` and `/.well-known/...` routes. For Tailscale,
use `tailscale funnel --bg 7331`, not
`tailscale funnel --bg http://127.0.0.1:7331/ctx-mcp`.

Available tools:

| Tool | Description |
|---|---|
| `ctx_new` | Create a session, optionally with a custom id and parent. |
| `ctx_set` | Store a value in a session. File references are CLI-only. |
| `ctx_get` | Get a visible value from a session, shared context, or ancestor. Pass `preview: true` to return the first 10 lines. |
| `ctx_resolve` | Return all visible key/value pairs as structured data. |
| `ctx_list` | Return human-readable lines and structured entries including `value_type` and file path status. |
| `ctx_export` | Return default shell `export` lines, including `CTX_ID`. File references are omitted. |
| `ctx_share` | Share one session's context into another session. |
| `ctx_tree` | Render the complete session tree as text or JSON. |
| `ctx_render` | Render a stored template key with visible context variables. |
| `ctx_delete` | Delete a session. Fails if it has child sessions unless `recursive` is set. |
| `ctx_execute` | Execute a trigger template from the ctx trigger directory. |

The server uses the same settings and SQLite database as the CLI, so `db_path`
and `trigger_location` in `$HOME/.config/ctx/settings.yml` apply to both.

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

## Trigger Templates

Manual templates and automatic triggers use YAML files in the trigger directory. The frontmatter (before the optional `---` separator) is YAML; everything after `---` is the prompt template, rendered and passed to the script as `$CTX_TRIGGER_PROMPT` (see **Multi-variable bodies** below) — reference it explicitly in `script`, nothing is auto-appended.

```yaml
trigger-session: Issue-1
order: 0
script: pi "$CTX_TRIGGER_PROMPT"
entries:
  status:
    - value: PR_CREATED
---
Analyse and comment on PR $PR_NUMBER:

$STORY
```

`script` is required. `trigger-session`, `ancestor`, and `entries` are optional matchers. If no matcher is set, the template is only executed manually with `ctx execute`. Set `any-change: true` to fire on every `ctx` write; it cannot be combined with other matchers.

**`ancestor` matching** – `ancestor: <session>` requires that `<session>` is an ancestor of the triggering session (found by walking its parent chain), based on an exact ID match. It's optional and ANDs with the other matchers; leave it unset to match any ancestor (or none).

```yaml
ancestor: root
entries:
  STATUS:
    - value: DONE
---
```

**`entries` matching** – each key in `entries` can have zero or more `value` items:
- Zero values: wildcard — any write to that key fires the trigger.
- One value: the key's current value must equal it.
- Multiple values: logical OR — the current value must equal at least one.

When multiple keys are listed, the trigger fires only when **all** entries satisfy their condition AND the key that was just written is one of the listed entry keys.

```yaml
entries:
  STATUS:
    - value: DONE
    - value: CANCELLED
  PRIORITY:             # wildcard — any priority is fine
---
```

**Multi-line scripts** – use a YAML block literal (`|`) to run the whole block as a single POSIX shell script (via [`mvdan/sh`](https://github.com/mvdan/sh)). `$VAR` placeholders naming a known ctx value are resolved once up front and bound as positional parameters; any other `$VAR` (e.g. one the script assigns itself, like `ID=$(...)`) is left for the shell to resolve natively, so ordinary shell variables persist across the whole script. Use `ctx set` from within the script to persist a value beyond its lifetime.

```yaml
script: |
  git checkout main
  git pull
  pi "$CTX_TRIGGER_PROMPT"
---
Summarise recent changes for $PROJECT.
```

When a trigger fires, ctx renders the prompt from the triggering session, creates a child execution session by default, sets `CTX_ID` for the invoked script, and writes a JSON audit record under `trigger_log_<trigger-name>_<timestamp>` (a valid shell variable name, so `ctx export` can expose it) in the triggering session. Use `execution-session: <session>` to run the script with a specific existing session instead.

**Multi-variable bodies** – a body can be split into named blocks with `<!-- ctx:var NAME -->` markers. Each marker starts a new block running until the next marker (or EOF); surrounding blank lines are trimmed. Content before the first marker (or the whole body, if there are no markers) becomes `CTX_TRIGGER_PROMPT`. Every block, `$VAR`-rendered against the triggering session's values, is exported as an environment variable of the same name to the script — the script decides how and where to use each one, nothing is auto-appended. A variable defined this way takes precedence over a ctx-session value of the same name.

```yaml
script: opencode run --agents "$CTX_AGENTS" --prompt "$CTX_PROMPT"
---
<!-- ctx:var CTX_AGENTS -->
## planner
tools: [read, grep]
Break the task into steps, hand off to coder.

## coder
tools: [edit, bash]
Implement each step. Run tests after each change.

<!-- ctx:var CTX_PROMPT -->
Coordinate planner → coder to fix the failing tests in this PR.
```

Trigger bodies with no markers still parse the same way: the whole body becomes `CTX_TRIGGER_PROMPT`. Existing trigger files that relied on the old implicit trailing-argument behavior now need `script` to reference `"$CTX_TRIGGER_PROMPT"` explicitly, as in the examples above.

**`schedule` matching** – `schedule: "<cron expression>"` fires the trigger on a time schedule instead of a `ctx` write. It uses the standard 5-field cron format (`minute hour day-of-month month day-of-week`, e.g. `crontab(5)`, Kubernetes `CronJob`, GitHub Actions): `*` for any value, an exact number, a comma-separated list, or `*/N` for every Nth unit. `schedule` is mutually exclusive with `any-change`, `trigger-session`, `ancestor`, and `entries` — a trigger is either schedule-driven or transition-driven, never both — and requires `execution-session` to be set, since a schedule-driven trigger has no triggering session to read/write vars from or log to.

A running `ctx serve --http` or bare `ctx serve` (see [MCP Server POC](#mcp-server-poc)) polls every `schedule`-bearing trigger roughly every 30 seconds and fires each one at most once per matching cron minute:

```yaml
schedule: "*/15 * * * *"   # every 15 minutes
execution-session: watch
script: pi "$CTX_TRIGGER_PROMPT"
---
Check for updates on $PROJECT.
```

Scheduled triggers are meant to stay narrowly scoped — poll something external and write the result into context — with a separate, ordinary transition-based trigger reacting to that write (e.g. "a PR was created, review it").

If no persistent `ctx serve` process is running, skip `schedule` entirely and point OS cron directly at a schedule-less trigger instead — no `ctx`-side due-checking needed:

```
* * * * * ctx execute <session> <template>
```

## `ctx tree` output example

```text
5f2a1c9b
 PROJECT_ID gitlab-org/myproject
 MR_IID 412
├── 8e7d3a4f
│     DISCUSSION_ID abc123def456
└── a1b2c3d4
      REPORT [path] /tmp/report.txt
      STORY Fix issue 45 ...
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
