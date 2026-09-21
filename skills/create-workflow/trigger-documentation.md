# Trigger Templates

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

`script` is required. `trigger-session`, `ancestor`, and `entries` are optional matchers. If no matcher is set, the template is only fired manually with `ctx trigger`. Set `any-change: true` to fire on every `ctx` write; it cannot be combined with other matchers.

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

When a trigger fires, ctx renders the prompt from the triggering session, creates a child execution session by default, and sets `CTX_ID` for the invoked script. Use `execution-session: <session>` to run the script with a specific existing session instead.

**Variables in frontmatter** – `execution-session` and `output-entry` may reference ctx values of the triggering session, e.g. `execution-session: $STORY_ID`, rendered when the trigger fires. The built-in `$CTX_TRIGGER_SESSION` names the triggering session itself — `execution-session: $CTX_TRIGGER_SESSION` runs the script (and lands `output-entry`) in the session that fired the trigger; it is also exported to the script's environment. Matcher fields (`trigger-session`, `ancestor`, `entries`) stay literal, and a schedule-driven trigger without filters has no triggering session, so its `execution-session` must be literal.

**Background execution** – `ctx set` from the CLI returns immediately: matching and running triggers happens in a detached `ctx fire-triggers` process (an internal command), so a slow harness never blocks the writer. Within that runner, same-`order` triggers still run in parallel and order groups sequentially, exactly as before. Set `CTX_TRIGGERS_SYNC=1` to wait for triggers in the foreground instead (useful in scripts and CI that must observe the trigger's effect before continuing).

**Timeout** – set `timeout: <duration>` (Go syntax, e.g. `30s`, `10m`) to kill a script that runs too long; the run is recorded as `script timed out after <duration>`. Without it, scripts run unbounded.

**Output capture** – set `output-entry: <KEY>` in the frontmatter to store the script's trimmed stdout under that key in the execution session after a successful run (non-zero exit writes nothing). The write is an ordinary `ctx` write, so it fires downstream triggers — this is the standard way to collect harness output and chain the next step:

```yaml
entries:
  STATUS:
    - value: REVIEW
output-entry: REVIEW_RESULT
script: pi "$CTX_TRIGGER_PROMPT"
---
Review the change described in $STORY. End with APPROVED or CHANGES_REQUESTED.
```

**Failure capture** – set `failure-entry: <KEY>` to write a short failure summary (trigger name, exit code, stderr tail) to that key in the execution session when the script fails or times out. Like `output-entry`, it's an ordinary write, so a recovery trigger can fire on it instead of the chain stalling silently.

**Chaining** – a `ctx set` from inside a trigger script fires downstream triggers like any other write, so triggers can chain (build writes `STATUS=FAILED`, a second trigger fires on it). Each nesting level increments `CTX_TRIGGER_DEPTH`; past the depth limit (default 5, configurable with `max_trigger_depth` in `settings.yml`) writes stop firing triggers, so accidental loops terminate. Set `CTX_SUPPRESS_TRIGGERS=1` in a script for writes that should never fire anything.

**Logging** – set `logging: true` in the frontmatter to write a JSON audit record under `trigger_log_<trigger-name>_<timestamp>` (a valid shell variable name, so `ctx export` can expose it) in the triggering session for every run. Logging is off by default; script failures are still reported on stderr.

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

**`schedule` matching** – `schedule: "<cron expression>"` fires the trigger on a time schedule instead of a `ctx` write. It uses the standard 5-field cron format (`minute hour day-of-month month day-of-week`, e.g. `crontab(5)`, Kubernetes `CronJob`, GitHub Actions): `*` for any value, an exact number, a comma-separated list, or `*/N` for every Nth unit. A schedule-driven trigger never fires on writes (and `any-change` can't be combined with it).

A running `ctx serve --http` or bare `ctx serve` (see the main README's MCP Server POC section) polls every `schedule`-bearing trigger roughly every 30 seconds and fires each one at most once per matching cron minute:

```yaml
schedule: "*/15 * * * *"   # every 15 minutes
execution-session: watch
script: pi "$CTX_TRIGGER_PROMPT"
---
Check for updates on $PROJECT.
```

If a run is still in progress when the trigger comes due again, that due minute is skipped (with a warning logged) rather than starting a second, overlapping run — keep `script` well under the schedule interval so runs don't pile up.

`schedule` can be combined with the `trigger-session`, `ancestor`, and `entries` filters. On each matching tick, every session whose *current state* satisfies all filters becomes a triggering session, and the trigger fires once per matching session — so one cron trigger can poll every active task session. An `entries` key with no values means "the key must be visible in that session"; with values, the current value must equal one of them. Without filters, `execution-session` is required (there is no triggering session otherwise) and must be literal:

```yaml
schedule: "*/5 * * * *"
entries:
  STATUS:
    - value: WATCHING
script: check-upstream "$PROJECT" && ctx set STATUS CHANGED
---
```

Scheduled triggers are meant to stay narrowly scoped — poll something external and write the result into context — with a separate, ordinary transition-based trigger reacting to that write (e.g. "a PR was created, review it").

If no persistent `ctx serve` process is running, skip `schedule` entirely and point OS cron directly at a schedule-less trigger instead — no `ctx`-side due-checking needed:

```
* * * * * ctx trigger <session> <template>
```
