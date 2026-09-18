schedule: "*/5 * * * *"
ancestor: agent-tasks
entries:
  STATUS:
    - value: NEEDS_REVIEW
execution-session: $CTX_TRIGGER_SESSION
timeout: 5m
script: |
  echo "task still waiting for review: $TASK" >&2
---
