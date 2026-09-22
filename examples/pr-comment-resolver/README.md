# PR comment resolver

Watches every open PR on a repo for unresolved review comments from one
GitHub user, fixes each one with [OpenCode](https://opencode.ai), pushes the
fix, replies with the commit hash, and resolves the thread — no
orchestration code, just trigger YAML and `ctx` state.

## How it works

A schedule-driven trigger polls GitHub once a minute. For every new
unresolved review thread it finds where `$AUTHOR` left a comment, it creates
a dedicated child session and starts a chain in it:

```
(scan every minute)  ──10-scan──▶  TASK  ──20-fix──▶  COMMIT_HASH  ──30-comment──▶  STATUS=DONE
                                      │
                                      └── failure-entry: AGENT_ERROR ──40-recover──▶ STATUS=FAILED
```

| File | Fires on | Does |
|---|---|---|
| `10-scan.md` | cron, every minute | Lists open PRs, finds unresolved review threads with a comment from `$AUTHOR`, creates one child session per new thread (`pr-comment-<hash>`) and writes `TASK` to start it |
| `20-fix.md` | `TASK` written | Checks out the PR branch, runs `opencode run --auto` with the comment as the prompt, pushes what OpenCode committed, writes `COMMIT_HASH` |
| `30-comment.md` | `COMMIT_HASH` written | Replies on the review thread with the commit hash and resolves it |
| `40-recover.md` | `AGENT_ERROR` written | A `20-fix` run failed or timed out — replies on the thread flagging it for manual attention, sets `STATUS=FAILED` |

Each thread gets a session name derived from a hash of its GraphQL thread
ID, so re-running the scan is idempotent: `ctx session` fails for a thread
that's already being worked (or already finished), and the scan just moves
on. `20-fix.md` takes an `mkdir`-based lock around the checkout so two
threads fixed at the same time never race on the same working copy.

## Setup

Prerequisites: `gh` authenticated against the repo (with permission to
comment and resolve review threads), `jq`, and `opencode` installed and
authenticated.

```bash
# 1. Point ctx at these triggers
cat > ~/.config/ctx/settings.yml <<EOF
trigger_location: /absolute/path/to/ctx/examples/pr-comment-resolver/triggers
EOF

# 2. Root session with shared config
ctx session pr-comments --root
ctx set pr-comments REPO_DIR /absolute/path/to/your/checkout
ctx set pr-comments AUTHOR your-github-login

# 3. Keep a scheduler running so the every-minute scan actually fires
ctx serve
```

`ctx serve` polls schedule-driven triggers roughly every 30 seconds and
must stay running for `10-scan.md` to fire; run it under whatever process
supervisor you'd normally use to keep a long-lived process alive (e.g. a
launchd/systemd unit or a detached background process).

## Known limitations

- **`--auto` is required and is dangerous** — it's what lets OpenCode edit
  files, commit, and push without a human approving each action, the same
  trade-off as `claude --permission-mode acceptEdits` in the
  [agent-harness example](../agent-harness). Only point this at
  repos/branches you're comfortable letting an agent push to unattended.
- **No automatic retry.** A thread whose fix failed lands in
  `STATUS=FAILED` permanently — the deterministic session name means the
  scan will never recreate it. To retry by hand:
  `ctx session rm pr-comment-<hash>` and wait for the next scan tick.
- **First comment in a thread wins the fix target.** If `$AUTHOR` posts more
  than one comment in the same thread, the *last* one is used as the
  prompt; earlier ones are ignored.
- **Single page of threads/comments.** `10-scan.md` fetches up to 100
  review threads and 50 comments per thread per PR without pagination —
  plenty for normal use, but a very large PR could exceed it.
