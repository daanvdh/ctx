ancestor: pr-comments
entries:
  AGENT_ERROR:
execution-session: $CTX_TRIGGER_SESSION
logging: true
order: 0
script: |
  set -e
  cd "$REPO_DIR"

  BODY=$(printf 'This PR comment is being handled by an automated workflow that ran into an error and needs manual follow-up:\n\n```\n%s\n```' "$AGENT_ERROR")

  gh api graphql -f query='
    mutation($id: ID!, $body: String!) {
      addPullRequestReviewThreadReply(input: { pullRequestReviewThreadId: $id, body: $body }) {
        comment { id }
      }
    }' -f id="$THREAD_ID" -f body="$BODY" >/dev/null

  ctx set STATUS FAILED
---
