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
| `10-scan.md` | cron, every minute | Lists open PRs, finds unresolved review threads with a comment from `$AUTHOR`, creates one child session per new thread (`pr-comment-<hash>`), maintains one `git worktree` per PR checked out on a local branch tracking the PR's own origin branch so the commented-on file can be referenced with `ctx set --path`, and writes `TASK` to start it |
| `20-fix.md` | `TASK` written | In that PR's worktree, runs `opencode run --auto` with the comment and the affected file as the prompt, then commits (with a message from a separate `opencode run` call), pushes, and writes `COMMIT_HASH` |
| `30-comment.md` | `COMMIT_HASH` written | Replies on the review thread with the commit hash and resolves it |
| `40-recover.md` | `AGENT_ERROR` written | A `20-fix` run failed or timed out — replies on the thread flagging it for manual attention, sets `STATUS=FAILED` |

Each thread gets a session name derived from a hash of its GraphQL thread
ID, so re-running the scan is idempotent: `ctx session` fails for a thread
that's already being worked (or already finished), and the scan just moves
on. Every thread on a given PR shares that PR's worktree, so `20-fix.md`
takes an `mkdir`-based lock scoped to it, keyed off the worktree path —
two threads on the same PR fixed at the same time never race on it, while
threads on different PRs run in parallel.

## Setup

Prerequisites: `gh` authenticated against the repo (with permission to
comment and resolve review threads), `jq`, and `opencode` installed and
authenticated (the default model is used).

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
- **Worktrees aren't cleaned up.** `10-scan.md` creates each PR's worktree
  under `/tmp` on first sight and keeps reusing it (`10-scan.md` only
  lists open PRs, so nothing ever removes it once one closes or merges).
  Clean up manually with `git worktree remove` (or `prune`) in
  `$REPO_DIR` for PRs you're done with.
