---
name: ctx-settings
description: ctx is a commandline tool with global settings. Use when changing settings to ctx. 
---

Ctx is a commandline key value store with script automation. Its settings are stored in `~/.config/ctx/settings.yml`, a YAML file that does not exist until first written. Add only the keys you need to change — every setting has a working default.

## General settings

```yaml
# Path to the sqlite database file. Relative paths resolve against
# ~/.config/ctx. Default: ~/.config/ctx/ctx.sqlite
db_path: ~/.config/ctx/ctx.sqlite

# Directory containing trigger template files (see the create-workflow
# skill). Relative paths resolve against ~/.config/ctx.
# Default: ~/.config/ctx/triggers
trigger_location: ~/.config/ctx/triggers

# Session used when the CTX_ID environment variable is unset and no
# default_sessions entry matches the current directory. Default: none
# (commands that need a session fail if one isn't resolved some other way).
default_session: main

# Per-directory default sessions, keyed by absolute directory path. The
# entry whose path is the longest match for (i.e. equal to, or an ancestor
# of) the current working directory wins, beating default_session.
# CTX_ID, when set, always takes precedence over both. Default: none.
default_sessions:
  /Users/me/git/project-a: project-a
  /Users/me/git/project-b: project-b

# Maximum depth of chained triggers (a ctx write from inside a trigger's
# script firing another trigger, and so on) before ctx aborts the chain to
# prevent runaway recursion. Default: 5
max_trigger_depth: 5

# Maximum size, in bytes, of a single stored string value. Raise this if
# you need to store larger context blobs. Default: 512000 (500KB)
max_string_bytes: 512000
```

## Remote backend settings
Configure these to make the CLI operate against a remote ctx MCP server instead of the local sqlite database. When `remote_mcp_url` is set, `db_path` is ignored.

```yaml
# URL of a remote ctx MCP server to use as the backend instead of the local
# sqlite db. Default: "" (use the local sqlite db at db_path)
remote_mcp_url: http://ctx-host:7331/mcp

# Bearer token sent with requests to remote_mcp_url, if that server requires
# one. Default: ""
remote_mcp_token: long-random-secret
```

## MCP server settings
These configure `ctx serve --http`, which makes the command line tool available to external applications over an MCP (Model Context Protocol) server. Each has a corresponding `ctx serve` flag and, for the OAuth/token fields, an environment variable that takes precedence over both the flag and the setting (`CTX_MCP_CLIENT_ID`, `CTX_MCP_CLIENT_SECRET`, `CTX_MCP_TOKEN`, `CTX_MCP_PUBLIC_URL`).

```yaml
# Listen address for the HTTP MCP server. Default: 127.0.0.1:7331
mcp_http_addr: 127.0.0.1:7331

# URL path the MCP endpoint is served on. Default: /mcp
mcp_http_path: /ctx-mcp

# Server name reported to MCP clients. Default: ctx-mcp
mcp_server_name: ctx

# Origin header values allowed for browser-originated requests (CORS).
# Default: [] (no browser origins allowed)
mcp_allowed_origins: []

# Static bearer token accepted by the server for simple clients that can
# send it directly, instead of going through OAuth. Default: ""
mcp_token: ""

# OAuth client ID for clients that authenticate via OAuth (e.g. Claude).
# Required together with mcp_oauth_client_secret to enable OAuth.
# Default: ""
mcp_oauth_client_id: claude

# OAuth client secret paired with mcp_oauth_client_id. Default: ""
mcp_oauth_client_secret: long-random-secret

# Public URL of the MCP server, used to build correct OAuth redirect/issuer
# URLs when the server sits behind a reverse proxy that doesn't forward the
# original host. Default: "" (inferred from the incoming request)
mcp_public_url: ""
```
