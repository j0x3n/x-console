#!/bin/sh
# Fake Claude Code for tests: prints stream-json lines like
# `claude -p --output-format stream-json --verbose` and edits a file.
#
# The prompt comes on stdin. Words in it change the behavior:
#   SLEEP  keep running for a long time (for cancel and timeout tests)
#   TRAP   ignore Ctrl+C, so only the kill after the grace period stops it
#   FAIL   report an error and exit with status 3
#   SLOW   pause between lines so output arrives in several batches
#   NOEDIT change nothing
if [ "$1" = "--version" ]; then
  echo "9.9.9 (Fake Claude)"
  exit 0
fi
prompt=$(cat)
case "$prompt" in
  *ASKONCE*) printf '%s\n' '{"title":"选择数据库","options":["SQLite","Postgres"]}' > .xc-question.md; exit 0 ;;
esac
pause() { case "$prompt" in *SLOW*) sleep 0.3 ;; esac; }
case "$prompt" in *TRAP*) trap '' INT ;; esac

echo '{"type":"system","subtype":"init","model":"fake-model","session_id":"s-1","tools":["Edit"]}'
pause
printf '{"type":"assistant","message":{"content":[{"type":"text","text":"Working on it."}]}}\n'
pause
echo "not json: plain progress line"
echo "warning: fake stderr line" >&2
case "$prompt" in
  *NOEDIT*) ;;
  *)
    printf '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Write","input":{"file_path":"hello.txt","content":"hi"}}]}}\n'
    printf '%s\n' "$prompt" > hello.txt
    echo "changed" >> README.md
    printf '{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"File written"}]}}\n'
    ;;
esac
pause
case "$prompt" in
  *SLEEP*)
    echo '{"type":"assistant","message":{"content":[{"type":"text","text":"Sleeping."}]}}'
    sleep 30 &
    wait $!
    sleep 30
    ;;
esac
case "$prompt" in
  *FAIL*)
    echo '{"type":"result","subtype":"error_during_execution","is_error":true,"result":"fake failure","num_turns":1}'
    exit 3
    ;;
esac
echo '{"type":"result","subtype":"success","is_error":false,"result":"Done.","num_turns":2,"total_cost_usd":0.01,"duration_ms":1200}'
