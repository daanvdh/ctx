# ctx — Agent Context Manager

`ctx` is a tiny command‑line tool that provides a hierarchical, key–value store for
agent‑oriented workflows. It lets an orchestrator create *sessions* (a lightweight
context) and attach arbitrary string data to them. Sub‑agents can inherit the
data of their ancestors by simply receiving the session identifier.

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

- By default the database is stored at `$HOME/.config/ctx/ctx.sqlite`.
- Set the environment variable `CTX_DB_PATH` to point to a different file,
  e.g.:

```bash
export CTX_DB_PATH=/tmp/my‑ctx.db   # before running any ctx command
```

## Core Commands

| Command | Synopsis | Description |
|---|---|---|
| `ctx new [options] [custom_id]` | `ctx new`<br>`ctx new <parent-id>`<br>`ctx new my-custom-id`<br>`ctx new my-custom-id --parent <parent-id>`<br>`ctx new --help` | Create a new session. If a custom ID is supplied, it is used (must consist of letters, digits, hyphens or underscores). Otherwise an 8‑character hexadecimal ID is generated. Parent can be set explicitly via `--parent`, or implicitly from the `CTX_ID` environment variable if present. Use `--help` for usage information. |
| `ctx set <session> <key> <value>` | `ctx set $SID PROJECT_ID "myproj"` | Store *value* under *key* in the specified session (overwrites existing key). |
| `ctx get <session> <key>` | `ctx get $SID PROJECT_ID` | Retrieve a value, searching up the parent chain. Prints the value to stdout. Fails if the key cannot be found. |
| `ctx export <session>` | `ctx export $SID` | Emit all visible keys as shell‑compatible assignments (`export KEY='VALUE'`). Use with `eval "$(ctx export …)"` or `env $(ctx export …) command`. |
| `ctx tree` | `ctx tree` | Render the complete session hierarchy as an ASCII tree, showing ids and key/value pairs. |
| `ctx help` | `ctx help` | Show a short usage summary (also shown when calling `ctx` without arguments). |


**Note:** If `CTX_ID` is set in the environment, it will be used as the implicit parent for newly created sessions when no `--parent` flag is provided.



All commands exit with status 0 on success; error details are written to **stderr**.

## Using implicit parent via `CTX_ID`

The environment variable `CTX_ID` can be used to automatically set the parent of a new session when you do **not** specify `--parent`. This is handy in scripts:

```bash
# Create a root session and export its ID.
ROOT=$(ctx new)               # prints e.g. "a1b2c3d4"
export CTX_ID=$ROOT           # make it available to subsequent commands

# Create a child session with a custom name; parent defaults to $CTX_ID.
CHILD=$(ctx new review-agent)
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

# Bonus: share files via context.
ctx set $ROOT REPORT_PATH "/tmp/report.txt"
cat "$(ctx get $CHILD REPORT_PATH)"   # prints the file content from the child’s view
```

## `ctx tree` output example

```text
5f2a1c9b
 PROJECT_ID=gitlab-org/myproject
 MR_IID=412
├── 8e7d3a4f
│     DISCUSSION_ID=abc123def456
└── a1b2c3d4
      REPORT_PATH=/tmp/report.txt
```

The tree displays sessions sorted alphabetically, with child nodes indented.
Key/value pairs belonging to a session are listed directly beneath its ID.

## Design Highlights

- **SQLite backend** – guarantees atomic writes and handles concurrent reads/writes without external locking.
- **Hierarchical lookup** – `ctx get` walks up the parent chain (max 50 hops) so children automatically inherit keys from ancestors. Later entries shadow earlier ones.
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
