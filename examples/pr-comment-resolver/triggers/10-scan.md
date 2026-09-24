schedule: "* * * * *"
execution-session: pr-comments
timeout: 5m
script: |
  cd "$REPO_DIR" || exit 1

  OWNER=$(gh repo view --json owner -q .owner.login)
  REPO=$(gh repo view --json name -q .name)

  for PR in $(gh pr list --state open --json number -q '.[].number'); do
    ROWS=$(gh api graphql -f query='
      query($owner: String!, $repo: String!, $pr: Int!) {
        repository(owner: $owner, name: $repo) {
          pullRequest(number: $pr) {
            reviewThreads(first: 100) {
              nodes {
                id
                isResolved
                path
                line
                comments(first: 50) {
                  nodes { url body author { login } }
                }
              }
            }
          }
        }
      }' -f owner="$OWNER" -f repo="$REPO" -F pr="$PR" \
      | jq -c --arg author "$AUTHOR" '
          .data.repository.pullRequest.reviewThreads.nodes[]
          | select(.isResolved == false)
          | select(any(.comments.nodes[]; .author.login == $author))
          | {
              thread_id: .id,
              path: .path,
              line: .line,
              body: ([.comments.nodes[] | select(.author.login == $author)] | last | .body),
              url: ([.comments.nodes[] | select(.author.login == $author)] | last | .url)
            }
        ')

    [ -z "$ROWS" ] && continue

    # One worktree per PR, checked out on a local branch of the same name tracking 
    # the PR's own origin branch. Every thread on this PR shares it: 10-scan.md 
    # reads files from it, 20-fix.md edits, commits, and pushes from it.
    BRANCH=$(gh pr view "$PR" --json headRefName -q .headRefName)
    FIX_WORKTREE="/tmp/ctx-pr-comments-$(basename "$REPO_DIR")-pr-$PR.worktree"

    git fetch origin "$BRANCH:refs/remotes/origin/$BRANCH" -q || { echo "fetch failed for PR #$PR ($BRANCH)" >&2; continue; }
    if git -C "$FIX_WORKTREE" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
      git -C "$FIX_WORKTREE" reset --hard "origin/$BRANCH" -q || { echo "reset failed for PR #$PR worktree" >&2; continue; }
    else
      # Not a valid worktree (never created, or left behind as a plain
      # directory by a prior failed attempt) — (re)create it from scratch.
      rm -rf "$FIX_WORKTREE"
      git worktree prune
      git worktree add -q -B "$BRANCH" "$FIX_WORKTREE" "origin/$BRANCH" || { echo "worktree add failed for PR #$PR" >&2; continue; }
    fi

    echo "$ROWS" | while IFS= read -r ROW; do
      [ -z "$ROW" ] && continue
      THREAD_ID=$(printf '%s' "$ROW" | jq -r '.thread_id')
      HASH=$(printf '%s' "$THREAD_ID" | shasum -a 256 | cut -c1-16)
      SESSION="pr-comment-$HASH"

      # ctx session fails if $SESSION already exists, which is what makes
      # this idempotent across scan ticks: a thread already being worked
      # (or already resolved-but-not-yet-cleaned-up) is simply skipped.
      if ctx session pr-comments "$SESSION" >/dev/null 2>&1; then
        FILE_NAME=$(printf '%s' "$ROW" | jq -r '.path')
        mkdir -p "$(dirname "$FIX_WORKTREE/$FILE_NAME")"
        [ -f "$FIX_WORKTREE/$FILE_NAME" ] || touch "$FIX_WORKTREE/$FILE_NAME"

        ctx set "$SESSION" PR_NUMBER "$PR"
        ctx set "$SESSION" THREAD_ID "$THREAD_ID"
        ctx set "$SESSION" COMMENT_URL "$(printf '%s' "$ROW" | jq -r '.url')"
        ctx set "$SESSION" COMMENT_BODY "$(printf '%s' "$ROW" | jq -r '.body')"
        ctx set "$SESSION" FILE_NAME "$FILE_NAME"
        ctx set "$SESSION" LINE "$(printf '%s' "$ROW" | jq -r '.line')"
        ctx set "$SESSION" FIX_WORKTREE "$FIX_WORKTREE"
        ctx set "$SESSION" FILE_CONTENT --path "$FIX_WORKTREE/$FILE_NAME"
        ctx set "$SESSION" TASK started
      fi
    done
  done
---
