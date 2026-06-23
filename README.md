# ctx — Agent Context Manager

`ctx` is a tiny command-line tool that provides a hierarchical, typed key-value
store for agent‑oriented workflows. It lets an orchestrator create *sessions* (a
lightweight context) and attach scalar strings, long-form documents, or file
references to them. Sub‑agents can inherit the data of their ancestors by simply
receiving the session identifier.

The entire state lives in an SQLite database (`ctx.sqlite`) which is safe for
concurrent access, has atomic writes and does not require any external service.

## Quick Install

```bash
# Prerequisite: Go 1.21+ (the build requires Go 1.25)
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

- By default the database is stored at `$HOME/.config/ctx/ctx.sqlite`. This location can be changed by creating a JSON settings file (`$HOME/.config/ctx/settings.json`) with a `db_path` field, for example:
  ```json
  {
    "db_path": "/tmp/my‑ctx.db"
  }
  ```
- Trigger templates live in `$HOME/.config/ctx/triggers` by default. Set `trigger_location` in `settings.json` to use a different directory.
- HTTP MCP defaults and authentication can also be configured in `settings.json`:
  ```json
  {
    "mcp_http_addr": "127.0.0.1:7331",
    "mcp_http_path": "/ctx-mcp",
    "mcp_server_name": "ctx",
    "mcp_oauth_client_id": "claude",
    "mcp_oauth_client_secret": "long-random-secret"
  }
  ```
  Keep this file private when it contains secrets. New settings files created by `ctx` use owner-only permissions.

## Core Commands

| Command | Synopsis | Description |
|---|---|---|
| `ctx new [options] [custom_id]` | `ctx new`<br>`ctx new <parent-id>`<br>`ctx new my-custom-id`<br>`ctx new my-custom-id --parent <parent-id>`<br>`ctx new --help` | Create a new session. If a custom ID is supplied, it is used (must consist of letters, digits, hyphens or underscores). Otherwise an 8‑character hexadecimal ID is generated. Parent can be set explicitly via `--parent`, or implicitly from the `CTX_ID` environment variable if present. Use `--help` for usage information. |
| `ctx set [session] <key> <value>` | `ctx set $SID PROJECT_ID "myproj"`<br>`CTX_ID=$SID ctx set PROJECT_ID "myproj"` | Store a scalar string under *key* in the specified session (overwrites existing key). |
| `ctx set [session] <key> --doc [text]` | `ctx set $SID STORY --doc "Fix issue 45"`<br>`ctx set $SID SPEC --doc < openapi.md` | Store long-form text as a document. If no text argument is provided, content is read from stdin. Documents are excluded from plain `ctx export` and shown as previews in `ctx show` and `ctx tree`. |
| `ctx set [session] <key> --file-ref <path>` | `ctx set $SID API_SPEC --file-ref ./openapi.yaml` | Store a reference to an existing local file. The path must exist when it is set. Reads resolve file content at use time. |
| `ctx get [session] <key> [--path\|--preview]` | `ctx get $SID PROJECT_ID`<br>`ctx get $SID STORY --preview`<br>`ctx get $SID STORY --path` | Retrieve a visible value, searching the session, shared contexts, and then ancestors. For `file_ref`, default output is the referenced file content. `--preview` prints the first 10 lines. `--path` returns the stored path for `file_ref`, writes a `doc` to a temp file and returns that path, and returns the value for strings. |
| `ctx show [session]` | `ctx show $SID` | Print all visible keys with type tags. Documents and file references are shown as previews or paths, not full content. |
| `ctx export [session] [--include-docs] [--files-as-paths]` | `ctx export $SID`<br>`ctx export $SID --include-docs --files-as-paths` | Emit shell-compatible assignments, including `CTX_ID`. Plain export includes only strings; opt into documents and file-reference paths with flags. Use with `eval "$(ctx export …)"` or `env $(ctx export …) command`. |
| `ctx share <from> <to>` | `ctx share root worker` | Make keys from one session visible to another session before ancestor lookup. |
| `ctx render [--ignore-missing] [session] <key>` | `ctx render $SID PROMPT` | Render a stored template by substituting `$VAR` placeholders from visible context. |
| `ctx execute [session] <template>` | `ctx execute $SID review` | Execute a trigger template from the trigger directory. The filename extension is optional. |
| `ctx tree` | `ctx tree` | Render the complete session hierarchy as an ASCII tree, showing ids and key/value pairs. |
| `ctx --version` | `ctx --version` | Print the build version. |
| `ctx help` | `ctx help` | Show a short usage summary (also shown when calling `ctx` without arguments). |

**Note:** If `CTX_ID` is set in the environment, commands that take a session can omit that argument. `ctx new` uses `CTX_ID` as the implicit parent when no `--parent` flag is provided.

All commands exit with status 0 on success; error details are written to **stderr**.

## Value Types

Every ctx entry has a value type:

| Type | Set with | `ctx get` | Export behavior |
|---|---|---|---|
| `string` | `ctx set KEY value` | Stored value | Included by default |
| `doc` | `ctx set KEY --doc "..."` or `ctx set KEY --doc < file.md` | Full document content | Omitted by default; include with `--include-docs` |
| `file_ref` | `ctx set KEY --file-ref ./path` | Current file content | Omitted by default; include path with `--files-as-paths` |
| `file_bin` | Reserved | Not implemented | Not exported |

`ctx render` resolves all implemented types to content before template
substitution, so consumers can use `$KEY` without knowing whether it came from a
string, document, or file reference. If a referenced file no longer exists,
`ctx get` and `ctx render` fail with a clear error.

Documents are limited to 500KB. Use `--file-ref` for larger local files or for
content that should be read fresh each time.

## MCP Server POC

`ctx` includes an MCP server proof of concept that exposes the ctx API as MCP
tools for clients such as Claude Desktop or OpenAI-compatible MCP hosts.

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
      "args": ["serve"]
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
`settings.json` or `CTX_MCP_TOKEN` in the environment. OAuth credentials can also
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
| `ctx_set` | Store a value in a session. Pass `is_doc: true` for long-form documents. File references are CLI-only. |
| `ctx_get` | Get a visible value from a session, shared context, or ancestor. Pass `preview: true` to return the first 10 lines. |
| `ctx_resolve` | Return all visible key/value pairs as structured data. |
| `ctx_show` | Return human-readable lines and structured entries including `value_type`, previews, sizes, and file path status. |
| `ctx_export` | Return default shell `export` lines, including `CTX_ID`. Documents and file references are omitted. |
| `ctx_share` | Share one session's context into another session. |
| `ctx_tree` | Render the complete session tree as text or JSON. |
| `ctx_render` | Render a stored template key with visible context variables. |
| `ctx_delete` | Delete a session and all descendants. |
| `ctx_execute` | Execute a trigger template from the ctx trigger directory. |

The server uses the same settings and SQLite database as the CLI, so `db_path`
and `trigger_location` in `$HOME/.config/ctx/settings.json` apply to both.

## Using `CTX_ID`

The environment variable `CTX_ID` can be used as the default session for commands and as the parent of a new session when you do **not** specify `--parent`. This is handy in scripts:

```bash
# Create a root session and export its ID.
ROOT=$(ctx new)               # prints e.g. "a1b2c3d4"
export CTX_ID=$ROOT           # make it available to subsequent commands

# Create a child session with a custom name; parent defaults to $CTX_ID.
CHILD=$(ctx new review-agent)

# Commands can now omit the session argument.
ctx set PROJECT_ID "gitlab-org/myproject"
ctx get PROJECT_ID
ctx show
```

If you need to override the implicit parent, use `--parent`:

```bash
export CTX_ID=$ROOT
CHILD=$(ctx new lint-agent --parent $ROOT)   # explicit parent flag takes precedence
```

Remember to **export** the variable; otherwise it is only set for a single command and will not be visible to subsequent `ctx new` invocations.

```bash
# Orchestrator creates a root session and a child.
ROOT=$(ctx new)               # => e.g. "5f2a1c9b"
CHILD=$(ctx new $ROOT)

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
ctx set $ROOT REPORT --file-ref /tmp/report.txt
ctx get $CHILD REPORT                  # prints the file content from the child's view
ctx get $CHILD REPORT --path           # prints /tmp/report.txt
```

## Trigger Templates

Manual templates and automatic triggers use files in the trigger directory:

```text
trigger-session=Issue-1
key=status
match=PR_CREATED
order=0
command=pi
---
Analyse and comment on PR $PR_NUMBER:

$STORY
```

`command` is required. `trigger-session`, `key`, and `match` are optional matchers and are combined with logical AND. If no matcher is set, the template is only executed manually with `ctx execute`. Set `any-change=true` to fire on every `ctx` write; it cannot be combined with other matchers.

When a trigger fires, ctx renders the prompt from the triggering session, creates a child execution session by default, sets `CTX_ID` for the invoked command, and writes a JSON audit record under `trigger_log.<timestamp>` in the triggering session. Use `execution-session=<session>` to run the command with a specific existing session instead.

## `ctx tree` output example

```text
5f2a1c9b
 PROJECT_ID [string] gitlab-org/myproject
 MR_IID [string] 412
├── 8e7d3a4f
│     DISCUSSION_ID [string] abc123def456
└── a1b2c3d4
      REPORT [file_ref] /tmp/report.txt
      STORY [doc] 1.4 KB "# Story\nAs a user..."
```

The tree displays sessions sorted alphabetically, with child nodes indented.
Entries belonging to a session are listed directly beneath its ID with type
tags. Documents are shown as previews, and file references are shown as paths.

## Design Highlights

- **SQLite backend** – guarantees atomic writes and handles concurrent reads/writes without external locking.
- **Hierarchical lookup** – `ctx get` walks up the parent chain (max 50 hops) so children automatically inherit keys from ancestors. Later entries shadow earlier ones.
- **Typed values** – scalar strings, long-form documents, and local file references resolve consistently through `ctx get` and `ctx render`.
- **Session IDs** – generated with `crypto/rand`, yielding an eight‑character lowercase hex string (`xxxxxxxx`). Collisions are extremely unlikely.
- **Depth limit** – prevents infinite loops in corrupted data (e.g., circular parent references).
- **Portable** – No external dependencies; the only required runtime is the SQLite driver bundled via Go modules.

## Development

```bash
make test    # run unit tests
make lint    # static analysis with golangci‑lint
make clean   # remove ./bin
```

Contributions are welcome. Please open an issue or a pull request for bugs,
features, or documentation improvements.

---

`ctx` – Simple, deterministic context handling for multi‑agent AI workflows.
