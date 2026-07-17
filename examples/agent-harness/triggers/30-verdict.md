ancestor: agent-tasks
entries:
  RESULT:
execution-session: $CTX_TRIGGER_SESSION
order: 0
script: |
  case "$RESULT" in
    *DONE*) ctx set STATUS DONE ;;
    *)      ctx set STATUS NEEDS_REVIEW ;;
  esac
---
