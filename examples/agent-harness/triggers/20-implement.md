ancestor: agent-tasks
entries:
  PLAN:
execution-session: $CTX_TRIGGER_SESSION
output-entry: RESULT
failure-entry: AGENT_ERROR
timeout: 30m
order: 0
script: |
  sh -c "$WORKER \"$CTX_TRIGGER_PROMPT\""
---
You are the implementer of a coding agent working on: $PROJECT

Execute this plan for the task below. Run the tests when you are done.
End your reply with the single word DONE if everything passed, or
NEEDS_REVIEW if you are unsure.

Task: $TASK

Plan:
$PLAN
