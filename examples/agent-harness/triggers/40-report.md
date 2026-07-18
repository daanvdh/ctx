ancestor: agent-tasks
entries:
  STATUS:
    - value: DONE
    - value: NEEDS_REVIEW
    - value: FAILED
execution-session: $CTX_TRIGGER_SESSION
output-entry: REPORT
logging: true
order: 0
script: |
  echo "task finished with status $STATUS -- $TASK"
---
