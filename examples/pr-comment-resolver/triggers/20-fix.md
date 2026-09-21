ancestor: pr-comments
entries:
  TASK:
execution-session: $CTX_TRIGGER_SESSION
output-entry: OPENCODE_LOG
failure-entry: AGENT_ERROR
timeout: 20m
order: 0
script: |
  set -e
  cd "$REPO_DIR"

  # Only one fix runs against the checkout at a time. mkdir is atomic, so
  # this is a safe lock even though the shell here has no flock built in.
  LOCKDIR="/tmp/ctx-pr-comments-$(basename "$REPO_DIR").lock"
  while ! mkdir "$LOCKDIR" 2>/dev/null; do sleep 2; done
  trap 'rmdir "$LOCKDIR"' EXIT

  git fetch origin
  gh pr checkout "$PR_NUMBER"
  git pull --ff-only

  opencode run --auto "$CTX_TRIGGER_PROMPT"

  if [ -n "$(git status --porcelain)" ]; then
    echo "opencode left uncommitted changes for PR #$PR_NUMBER" >&2
    exit 1
  fi

  git push
  ctx set COMMIT_HASH "$(git rev-parse HEAD)"
---
Address this GitHub PR review comment on PR #$PR_NUMBER:

$COMMENT_BODY

(comment thread: $COMMENT_URL)

Make the requested change in this repository. When you are done, commit
the fix with a clear, descriptive commit message that you write yourself,
and push it to the current branch. Do not leave the working tree dirty.
