# ctx — Agent Context Manager

A scoped, hierarchical key-value store for passing context between orchestrator agents and subagents in multi-agent LLM workflows. It enables models to never handle IDs or structured data directly — the orchestrator writes all context before spawning, and subagents receive one opaque session ID.

## Installation

```bash

# Install Go
brew install go

# Clone and build
git clone <repo>
cd ctx
make build

# Move binary to PATH
mv ./bin/ctx /usr/local/bin/ctx

# Verify
ctx --help
```

## Makefile Targets

| Target   | Description                              |
| -------- | ---------------------------------------- |
| `build`  | Build the binary to `./bin/ctx`          |
| `test`   | Run all tests (`go test ./...`)          |
| `lint`   | Run `golangci-lint run`                  |
| `clean`  | Remove the `./bin` directory              |

## Command Reference

### `ctx new [parent_session_id]`

Creates a new session. Prints the new session ID to stdout.

```bash
ROOT=$(ctx new)        # creates a root session
CHILD=$(ctx new $ROOT) # creates a child of ROOT
```

- No parent argument: creates a root session (`parent: null`)
- With parent: creates a child of the given session (exits 1 if parent not found)

### `ctx set <session_id> <key> <value>`

Stores `value` under `key` in the session's data.

```bash
ctx set $SESSION PROJECT_ID "gitlab-org/myproject"
ctx set $SESSION DISCUSSION_ID "abc123"
```

- Requires exactly 3 args: session_id, key, value
- Overwrites existing keys
- Exits 1 if session not found

### `ctx get <session_id> <key>`

Looks up a key walking up the parent chain (depth cap 50).

```bash
PROJECT=$(ctx get $SESSION PROJECT_ID)
```

- Prints the value to stdout (no newline decoration)
- Inherits from all ancestors (closer scope wins)
- Exits 1 if key not found anywhere in the chain

### `ctx export <session_id>`

Exports all visible keys as `KEY='VALUE'` lines (single-quoted, safe for eval).

```bash
eval "$(ctx export $SESSION)"
echo $PROJECT_ID
echo $DISCUSSION_ID
```

Or without modifying shell state:

```bash
env $(ctx export $SESSION) juni run prompt.md
```

Values are single-quoted. Internal single quotes are escaped as `'\''`.

### `ctx tree`

Prints an ASCII tree of all sessions in `ctx.json`.

```
abc12345
 PROJECT_ID=gitlab-org/myproject
 MR_IID=412
├── def67890
│     DISCUSSION_ID=abc123def456
└── ghi12345
     DISCUSSION_ID=xyz789abc012
```

## Usage Pattern for Agent Workflows

```bash
# 1. Orchestrator creates sessions and stores context
ROOT=$(ctx new)
CHILD=$(ctx new $ROOT)
ctx set $ROOT PROJECT_ID "gitlab-org/myproject"
ctx set $ROOT MR_IID "412"
ctx set $ROOT STORY_ID "PROJ-88"
ctx set $CHILD DISCUSSION_ID "abc123def456"

# 2. Export context for subagent
eval "$(ctx export $CHILD)"
# Now PROJECT_ID, MR_IID, STORY_ID, and DISCUSSION_ID are all available
# because they're visible through the parent chain

# 3. Spawn subagent
bash review.sh $CHILD

# Inside review.sh:
eval "$(ctx export $1)"
# All context is loaded — models never see raw IDs
```

## Key Design Notes

- **File is always `ctx.json`** in the current working directory
- **Auto-created** on first use — no `init` command required
- **Safe for concurrent agents** via file locking (`flock` on macOS)
- **Keys are case-sensitive** and never transformed — use the same casing in `ctx set` and `eval "$(ctx export $SESSION)"` calls
- **No `--file` flag** — one file per project directory means one workspace, one context
- **Scope chain**: key lookup walks up the parent chain (like lexical scoping in programming languages)
- **Session IDs**: 8-character lowercase hex strings from `crypto/rand`
- **Atomic writes**: temp file + rename on POSIX to prevent corruption
- **Depth cap of 50 hops** prevents infinite loops from corrupted parent references
