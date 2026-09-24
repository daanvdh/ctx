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
  cd "$FIX_WORKTREE"

  # Threads on the same PR share one worktree. mkdir is atomic, so this
  # is a safe lock even though the shell here has no flock built in.
  LOCKDIR="$FIX_WORKTREE.lock"
  while ! mkdir "$LOCKDIR" 2>/dev/null; do sleep 2; done
  trap 'rmdir "$LOCKDIR"' EXIT

  git fetch origin -q
  git reset --hard "@{upstream}" -q

  opencode run --auto "$CTX_TRIGGER_PROMPT"

  if [ -z "$(git status --porcelain)" ]; then
    echo "opencode made no changes for PR #$PR_NUMBER" >&2
    exit 1
  fi

  git add -A
  COMMIT_MSG=$(opencode run --auto "Write a single-line, descriptive git commit message for this staged diff. Output only the commit message, nothing else. Diff: $(git diff --cached)")
  git commit -m "$COMMIT_MSG"
  git push
  ctx set COMMIT_HASH "$(git rev-parse HEAD)"
---
$COMMENT_BODY

file name: $FILE_NAME
line: $LINE
file content: 
$FILE_CONTENT
