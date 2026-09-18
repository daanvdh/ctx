# Changelog

## 1.1.0

- Config-driven webhook ingestion: `ctx serve --http` now exposes
  `POST /webhooks/{source}`, driven by per-source adapter configs in
  `~/.config/ctx/webhooks/` (GitHub, GitLab, Jira). Deliveries land as
  `WEBHOOK_*` entries, and the final `WEBHOOK_EVENT` write fires the
  existing trigger machinery.
- `ctx set` writes now fire triggers asynchronously by default: a detached
  `ctx fire-triggers` process matches and runs them so `ctx set` returns
  immediately. Set `CTX_TRIGGERS_SYNC=1` to restore the previous blocking
  behavior.
- Trigger scripts can now chain: writes from inside a trigger script fire
  downstream triggers, bounded by `CTX_TRIGGER_DEPTH` (default max 5,
  configurable via `max_trigger_depth` in settings.yml). This replaces the
  previous blanket suppression of writes made from within trigger scripts.
  `CTX_SUPPRESS_TRIGGERS=1` remains as an explicit opt-out.
- New trigger frontmatter fields: `timeout` bounds script runtime,
  `output-entry` captures script stdout into a ctx entry, `failure-entry`
  routes script failures into context, and `logging: true` opts a trigger
  into run logging (off by default).
- Schedule triggers (`schedule: "<cron expr>"`) now support `session` and
  `entries` filters and fire once per matching session instead of only the
  configured execution session.
- The triggering session is now exposed to scripts as the built-in
  `CTX_TRIGGER_SESSION` variable, and `ctx` variables now render in
  `execution-session` and `output-entry` frontmatter.
- The default session is now configurable, both globally and per directory.
- `ctx execute` is renamed to `ctx trigger`; `execute` and `ctx_execute`
  remain as aliases.
- `ctx tree` and `ctx list` now format entries as `key: value`.
- CLI dispatch now goes through Cobra, adding shell completion
  (`ctx completion <shell>`) and a `version` subcommand alongside
  `--version`; existing flags, help text, and error/exit behavior are
  unchanged.
- Diagnostics now go through structured logging (`log/slog`), configurable
  via `CTX_LOG_LEVEL` (debug|info|warn|error); script stdout/stderr
  passthrough is unchanged.
- `max_string_bytes` in settings.yml replaces the previous hard-coded
  500KB string/webhook payload size limit (same 500KB default).
- Added an example configuration-only agent harness: six trigger files
  driving a full plan/implement/verdict/report/recovery/scheduled-watcher
  loop.
- Internal: schema migrations moved to embedded versioned `.sql` files,
  trigger loading/matching extracted into `internal/trigger`, the
  duplicated `Store` interface consolidated, and golangci-lint added to CI.

## 1.0.0

- `ctx serve` now has three modes: `--stdio` (MCP over stdio, no scheduler),
  `--http` (MCP over Streamable HTTP, scheduler always on), and no flag at
  all (trigger scheduler only, no MCP surface). `--http` and `--stdio` are
  mutually exclusive. **Breaking:** bare `ctx serve` no longer serves MCP
  over stdio — existing IDE integrations must switch to `ctx serve --stdio`.
- Triggers can now fire on a cron schedule via `schedule: "<cron expr>"`,
  polled internally by `ctx serve`'s scheduler (~30s resolution, at most one
  fire per matching cron minute, safe across multiple `ctx serve` processes
  sharing one database). `schedule` is mutually exclusive with `any-change`,
  `trigger-session`, `ancestor`, and `entries`, and requires
  `execution-session` to be set.
- Removed `ctx tick`, superseded by the internal scheduler. For cron-driven
  triggers with no persistent `ctx serve` process, invoke
  `ctx execute <session> <template>` directly from external cron instead.

## 0.0.1

Initial release
