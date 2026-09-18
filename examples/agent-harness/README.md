# Configuration-only agent harness

A complete agent loop — plan → implement → verdict → report, with failure
recovery — defined purely by trigger YAML files and ctx state. No
orchestration code, no daemon (the scheduled watcher is the one optional
exception), no SDK.

## How it works

Every trigger scopes itself to descendants of one root session
(`ancestor: agent-tasks`) and runs in the session that fired it
(`execution-session: $CTX_TRIGGER_SESSION`), so each task lives in its own
child session and the whole loop is just entries appearing in it:

```
TASK  ──10-plan──▶  PLAN  ──20-implement──▶  RESULT  ──30-verdict──▶  STATUS ──40-report──▶ REPORT
                                 │
                                 └── failure-entry: AGENT_ERROR ──50-recover──▶ STATUS=FAILED
```

| File | Fires on | Does |
|---|---|---|
| `10-plan.md` | `TASK` written | Calls `$PLANNER` with a planning prompt; stdout → `PLAN` |
| `20-implement.md` | `PLAN` written | Calls `$WORKER` with plan + task; stdout → `RESULT` |
| `30-verdict.md` | `RESULT` written | Deterministic check: `DONE` in result → `STATUS=DONE`, else `NEEDS_REVIEW` |
| `40-report.md` | `STATUS` written | Writes a one-line `REPORT`; keeps an audit log (`logging: true`) |
| `50-recover.md` | `AGENT_ERROR` written | A harness step failed or timed out → `STATUS=FAILED` |
| `60-watch.md` | cron, every 5 min | Optional: nags about tasks stuck in `NEEDS_REVIEW` (needs `ctx serve`) |

The harness commands are ctx values (`PLANNER`, `WORKER`), inherited from the
root session by every task session — swapping the whole loop onto a different
model is one `ctx set`.

## Run it (no AI required)

The loop runs end-to-end with stub "models" so you can watch the machinery:

```bash
# 1. Point ctx at these triggers (or copy them into your trigger dir)
cat > ~/.config/ctx/settings.yml <<EOF
trigger_location: /absolute/path/to/ctx/examples/agent-harness/triggers
EOF

# 2. Root session with shared config
ctx session agent-tasks --root
ctx set agent-tasks PROJECT "the demo project"
ctx set agent-tasks PLANNER "echo [stub planner]"
ctx set agent-tasks WORKER  "echo [stub worker] DONE"

# 3. One session per task; writing TASK starts the loop
TASK_SID=$(CTX_ID=agent-tasks ctx session)
ctx set $TASK_SID TASK "Add a --json flag to the list command"

# 4. Watch the loop fill the session (triggers run in the background)
sleep 2 && ctx list $TASK_SID
```

Expected output:

```
PLAN: [stub planner] You are the planner of a coding agent ...
REPORT: task finished with status DONE -- Add a --json flag to the list command
RESULT: [stub worker] DONE You are the implementer of a coding agent ...
STATUS: DONE
TASK: Add a --json flag to the list command
```

## Use a real harness

Replace the stubs with any CLI that takes a prompt argument. The
`sh -c "$PLANNER \"$CTX_TRIGGER_PROMPT\""` pattern in the triggers supports
multi-word commands:

```bash
ctx set agent-tasks PLANNER "claude -p"
ctx set agent-tasks WORKER  "claude -p --permission-mode acceptEdits"
```

Multiple tasks run concurrently — each task session carries its own chain.
Kick off the optional watcher with a bare `ctx serve` if you want the cron
trigger.

## What's still missing for fully functional agents

Findings from building this example (tracked on the issue tracker):

- **Per-session serialization** (#144) — two writes racing into the same
  session can overlap trigger runs; agents need an opt-in queue per
  (trigger, session).
- **Retry budgets** — `max_trigger_depth` bounds a chain, but a
  verdict→replan loop wants "retry at most N times per task", which today
  needs a counter entry maintained by script.
- **Working-directory control** — scripts inherit the writer's cwd; a
  `workdir:` frontmatter field would let a task pin its repo checkout
  instead of the script cd-ing itself.
- **Trigger-level env/secrets** — harness API keys currently come from the
  ambient environment; a settings-level env map would make runs
  reproducible.
