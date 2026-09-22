---
name: ctx-settings
description: ctx is a commandline tool with global settings. Use when changing settings to ctx. 
---

Ctx is a commandline key value store with script automation. It's settings are stored in `~/.config/ctx/settings.yml`. 

## General settings

```yaml
db_path: ~/.config/ctx/ctx.sqlite
trigger_location: ~/git/ctx/examples/pr-comment-resolver/triggers
```

## MCP settings
This is for making the command line tool available for external applications that rely on an MCP (authenticated) server. 

```yaml
mcp_http_addr: 127.0.0.1:7331
mcp_http_path: /ctx-mcp
mcp_server_name: ctx
mcp_allowed_origins: []
mcp_token: ""
mcp_oauth_client_id: claude
mcp_oauth_client_secret: my-secret
mcp_public_url: ""
```
