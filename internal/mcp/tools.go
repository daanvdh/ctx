package mcp

func tools() []map[string]any {
	return []map[string]any{
		tool("ctx_new", "Create a ctx session. If id is omitted, ctx generates one. If parent is omitted, CTX_ID may be used as the parent.", map[string]any{
			"id":     stringSchema("Optional custom session id."),
			"parent": stringSchema("Optional parent session id."),
		}, []string{}),
		tool("ctx_set", "Set a value in a ctx session. Use is_doc for long-form content; docs are excluded from ctx_export by default and shown as previews in ctx_show.", map[string]any{
			"session_id": stringSchema("Session id."),
			"key":        stringSchema("Context key."),
			"value":      stringSchema("Context value. For long-form text, set is_doc to true."),
			"is_doc":     boolSchema("Mark this value as a long-form document."),
		}, []string{"session_id", "key", "value"}),
		tool("ctx_get", "Get a visible value from a session. Doc values return full content; file_ref values return referenced file content.", map[string]any{
			"session_id": stringSchema("Session id."),
			"key":        stringSchema("Context key."),
			"preview":    boolSchema("Return only the first 10 lines."),
		}, []string{"session_id", "key"}),
		tool("ctx_resolve", "Return all visible key/value pairs for a session as structured data.", map[string]any{
			"session_id": stringSchema("Session id."),
		}, []string{"session_id"}),
		tool("ctx_show", "Return all visible key/value pairs as human-readable KEY = VALUE lines.", map[string]any{
			"session_id": stringSchema("Session id."),
		}, []string{"session_id"}),
		tool("ctx_resolve_entries", "Return all visible entries for a session with full values and types, unlike ctx_show which previews doc values. Used by clients that use a remote ctx MCP server as their backend.", map[string]any{
			"session_id": stringSchema("Session id."),
		}, []string{"session_id"}),
		tool("ctx_export", "Return all visible key/value pairs as shell export lines, including CTX_ID.", map[string]any{
			"session_id": stringSchema("Session id."),
		}, []string{"session_id"}),
		tool("ctx_share", "Make keys from one session visible to another session before ancestor lookup.", map[string]any{
			"from_session_id": stringSchema("Session whose context should be shared."),
			"to_session_id":   stringSchema("Session that should see the shared context."),
		}, []string{"from_session_id", "to_session_id"}),
		tool("ctx_tree", "Render the session hierarchy. If session_id is omitted, renders the complete tree.", map[string]any{
			"format":     enumSchema("Output format.", []string{"text", "json"}),
			"session_id": stringSchema("Optional session id; scopes the tree to this session's ancestors and descendants."),
		}, []string{}),
		tool("ctx_render", "Render a stored template key by substituting $VAR placeholders from visible context.", map[string]any{
			"session_id":     stringSchema("Session id."),
			"key":            stringSchema("Key containing the template."),
			"ignore_missing": boolSchema("Leave missing placeholders unchanged instead of failing."),
		}, []string{"session_id", "key"}),
		tool("ctx_delete", "Delete a session and its descendants.", map[string]any{
			"session_id": stringSchema("Session id."),
		}, []string{"session_id"}),
		tool("ctx_execute", "Execute a trigger template from the ctx trigger directory.", map[string]any{
			"session_id": stringSchema("Session id."),
			"template":   stringSchema("Trigger template filename or basename."),
		}, []string{"session_id", "template"}),
	}
}

func tool(name, description string, properties map[string]any, required []string) map[string]any {
	return map[string]any{
		"name":        name,
		"description": description,
		"inputSchema": map[string]any{
			"type":                 "object",
			"properties":           properties,
			"required":             required,
			"additionalProperties": false,
		},
	}
}

func stringSchema(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func boolSchema(description string) map[string]any {
	return map[string]any{"type": "boolean", "description": description}
}

func enumSchema(description string, values []string) map[string]any {
	return map[string]any{"type": "string", "description": description, "enum": values}
}
