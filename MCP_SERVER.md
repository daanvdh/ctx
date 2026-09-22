# MCP Server

`ctx` includes an MCP server proof of concept that exposes the ctx API as MCP
tools for clients such as Claude Desktop or OpenAI-compatible MCP hosts.

`ctx serve` has three modes: `--stdio` (MCP over stdio, no scheduler —
ephemeral, spawned per session by IDEs), `--http` (MCP over Streamable HTTP,
scheduler always on), and no flag at all (scheduler only, no MCP surface —
for running `ctx` purely to fire `schedule`-bearing triggers). `--http` and
`--stdio` are mutually exclusive. See the Trigger Templates section in the
[README](README.md#trigger-templates--writing-workflows) for
`schedule`-bearing triggers.

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
| `ctx_trigger` | Fire a trigger template from the ctx trigger directory. (`ctx_execute` still accepted as an alias.) |

The server uses the same settings and SQLite database as the CLI, so `db_path`
and `trigger_location` in `$HOME/.config/ctx/settings.yml` apply to both.
