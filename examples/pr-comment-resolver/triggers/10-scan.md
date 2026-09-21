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
              body: ([.comments.nodes[] | select(.author.login == $author)] | last | .body),
              url: ([.comments.nodes[] | select(.author.login == $author)] | last | .url)
            }
        ')

    [ -z "$ROWS" ] && continue

    echo "$ROWS" | while IFS= read -r ROW; do
      [ -z "$ROW" ] && continue
      THREAD_ID=$(printf '%s' "$ROW" | jq -r '.thread_id')
      HASH=$(printf '%s' "$THREAD_ID" | shasum -a 256 | cut -c1-16)
      SESSION="pr-comment-$HASH"

      # ctx session fails if $SESSION already exists, which is what makes
      # this idempotent across scan ticks: a thread already being worked
      # (or already resolved-but-not-yet-cleaned-up) is simply skipped.
      if ctx session pr-comments "$SESSION" >/dev/null 2>&1; then
        ctx set "$SESSION" PR_NUMBER "$PR"
        ctx set "$SESSION" THREAD_ID "$THREAD_ID"
        ctx set "$SESSION" COMMENT_URL "$(printf '%s' "$ROW" | jq -r '.url')"
        ctx set "$SESSION" COMMENT_BODY "$(printf '%s' "$ROW" | jq -r '.body')"
        ctx set "$SESSION" TASK started
      fi
    done
  done
---
