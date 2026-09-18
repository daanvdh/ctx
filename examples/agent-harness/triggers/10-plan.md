ancestor: agent-tasks
entries:
  TASK:
execution-session: $CTX_TRIGGER_SESSION
output-entry: PLAN
failure-entry: AGENT_ERROR
timeout: 15m
order: 0
script: |
  sh -c "$PLANNER \"$CTX_TRIGGER_PROMPT\""
---
You are the planner of a coding agent working on: $PROJECT

Break the following task into a short numbered list of concrete
implementation steps. Reply with the steps only.

Task: $TASK
