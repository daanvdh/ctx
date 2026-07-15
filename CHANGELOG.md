# Changelog

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
