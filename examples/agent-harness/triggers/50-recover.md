ancestor: agent-tasks
entries:
  AGENT_ERROR:
execution-session: $CTX_TRIGGER_SESSION
order: 0
script: |
  ctx set STATUS FAILED
---
