ancestor: pr-comments
entries:
  COMMIT_HASH:
execution-session: $CTX_TRIGGER_SESSION
failure-entry: AGENT_ERROR
logging: true
order: 0
script: |
  set -e
  cd "$REPO_DIR"

  gh api graphql -f query='
    mutation($id: ID!, $body: String!) {
      addPullRequestReviewThreadReply(input: { pullRequestReviewThreadId: $id, body: $body }) {
        comment { id }
      }
    }' -f id="$THREAD_ID" -f body="Fixed in $COMMIT_HASH" >/dev/null

  gh api graphql -f query='
    mutation($id: ID!) {
      resolveReviewThread(input: { threadId: $id }) {
        thread { isResolved }
      }
    }' -f id="$THREAD_ID" >/dev/null

  ctx set STATUS DONE
---
