#!/usr/bin/env bash
# End-to-end verification of the MuseForge personal build against a real
# platform started from the current checkout on a temporary SQLite file.
set -uo pipefail
cd "$(dirname "$0")"

BASE=http://127.0.0.1:18099
DB=/tmp/mf-e2e/verify.db
BIN=/tmp/mf-e2e/agentflow
pass=0; fail=0
ok()   { printf '  \033[32mPASS\033[0m %s\n' "$1"; pass=$((pass+1)); }
bad()  { printf '  \033[31mFAIL\033[0m %s\n' "$1"; fail=$((fail+1)); }
check(){ if [ "$2" = "$3" ]; then ok "$1 ($2)"; else bad "$1 (got $2, want $3)"; fi; }
code() { curl -s -o /tmp/mf-e2e/out.json -w '%{http_code}' -m 15 "$@"; }
json() { python3 -c "import json,sys;d=json.load(open('/tmp/mf-e2e/out.json'));print($1)" 2>/dev/null; }

# A run that was killed leaves its platform holding the port and its database
# deleted underneath it; clear both before starting.
lsof -ti :18099 2>/dev/null | xargs -r kill 2>/dev/null || true
sleep 1
rm -rf /tmp/mf-e2e; mkdir -p /tmp/mf-e2e
echo "== build =="
( cd .. && GOCACHE=/tmp/agentflow-gocache go build -o "$BIN" ./cmd/agentflow ) || { echo "build failed"; exit 1; }

echo "== the platform refuses a non-loopback bind =="
# The secret must be given here too: without it the binary exits on its own
# requirement before it ever looks at the address, and the check passes locally
# only because the developer's shell happens to export one.
if AGENTFLOW_SESSION_SECRET=verify AGENTFLOW_AUTH_REQUIRED=false "$BIN" -datasource "file:$DB.refused" -addr 0.0.0.0:18098 >/tmp/mf-e2e/refused.log 2>&1; then
  bad "a public bind must be refused"
else
  if grep -q "refusing to listen" /tmp/mf-e2e/refused.log; then ok "a public bind is refused with a reason"; else bad "refusal message missing"; fi
fi

echo "== start on loopback, one SQLite file, no sign-in =="
AGENTFLOW_SESSION_SECRET=verify AGENTFLOW_RUNNER_BOOTSTRAP_TOKEN=boot-verify nohup "$BIN" -datasource "file:$DB" -addr 127.0.0.1:18099 >/tmp/mf-e2e/platform.log 2>&1 &
PID=$!
trap 'kill $PID 2>/dev/null' EXIT
for _ in $(seq 60); do curl -sf "$BASE/api/health" >/dev/null && break; sleep 1; done
check "health without a token" "$(code $BASE/api/health)" 200
check "no login page" "$(code $BASE/login.html)" 404
check "no session route" "$(code $BASE/api/auth/current)" 404

echo "== the account =="
check "the console records the account" "$(code -X POST $BASE/api/account -H 'Content-Type: application/json' -d '{}')" 200
SECOND=$(code -X POST $BASE/api/account -H 'Content-Type: application/json' -d '{"loginName":"x"}')
check "asking again returns the same account" "$SECOND" 200
ID=$(json 'd["id"]')
[ "$ID" = "1" ] && ok "the platform has exactly one account (id 1)" || bad "account id = $ID"

echo "== projects and requirements =="
check "create a project" "$(code -X PUT $BASE/api/projects -H 'Content-Type: application/json' -d '{"id":"demo","key":"DM","name":"演示项目","color":"#536dfe","ownerId":1}')" 200
check "create a requirement with tags" "$(code -X POST $BASE/api/issues -H 'Content-Type: application/json' -d '{"title":"第一个需求","description":"端到端验证","projectId":"demo","priority":"high","status":"open","tags":["bug","ui"]}')" 200
ISS=$(json 'd["id"]'); TAGS=$(json '"".join(d.get("tags") or [])')
[ "$TAGS" = "bugui" ] && ok "the requirement keeps its tags" || bad "tags = $TAGS"
check "comment on it" "$(code -X POST $BASE/api/issues/$ISS/comments -H 'Content-Type: application/json' -d '{"body":"一条评论"}')" 200
check "close it" "$(code -X POST $BASE/api/issues/$ISS/close -H 'Content-Type: application/json' -d '{}')" 200
check "reopen it" "$(code -X POST $BASE/api/issues/$ISS/reopen -H 'Content-Type: application/json' -d '{}')" 200
check "filter by tag" "$(code "$BASE/api/issues?tag=ui")" 200
check "delete it" "$(code -X DELETE $BASE/api/issues/$ISS)" 204
code "$BASE/api/issues" >/dev/null; COUNT=$(json 'len(d)')
[ "$COUNT" = "0" ] && ok "the list is empty after the delete" || bad "list still holds $COUNT"

echo "== personal settings =="
check "save a model key" "$(code -X PUT $BASE/api/users/1/credentials -H 'Content-Type: application/json' -d '{"codeBuddyApiKey":"ck_verify"}')" 200
CONF=$(json 'd["codeBuddyApiKeyConfigured"]')
[ "$CONF" = "True" ] && ok "the key reads back configured" || bad "configured = $CONF"
git -C .. >/dev/null 2>&1 || true
if grep -q "ck_verify" /tmp/mf-e2e/out.json; then bad "the response leaks the key"; else ok "the response never returns the key"; fi
check "the model catalogue" "$(code $BASE/api/model-catalog)" 200
check "a project credential list" "$(code $BASE/api/projects/demo/credentials)" 200

echo "== the machine inventory =="
check "register a machine" "$(code -X POST $BASE/api/runners/register -H 'Content-Type: application/json' -H 'X-Bootstrap-Token: boot-verify' -d '{"name":"我的 MacBook","os":"darwin","arch":"arm64","runnerVersion":"0.13.2","runtimes":[{"provider":"claude","binaryPath":"/usr/local/bin/claude","version":"1.2.3"}]}')" 200
RID=$(json 'd["runnerId"]'); TOK=$(json 'd["sessionToken"]')
check "heartbeat" "$(code -X POST $BASE/api/runners/heartbeat -H 'Content-Type: application/json' -H "Authorization: Bearer $TOK" -d "{\"runnerId\":\"$RID\",\"runtimes\":[{\"provider\":\"claude\",\"binaryPath\":\"/usr/local/bin/claude\",\"version\":\"1.2.3\",\"status\":\"online\"}]}")" 200
check "the machine list" "$(code $BASE/api/runners)" 200
N=$(json 'len(d)'); [ "$N" = "1" ] && ok "the list holds the machine" || bad "machine count = $N"
ONLINE=$(json 'd[0]["status"]'); SEEN=$(json 'bool(d[0].get("lastSeenAt"))')
[ "$ONLINE" = "online" ] && ok "the machine reads as online" || bad "status = $ONLINE"
[ "$SEEN" = "True" ] && ok "the machine carries its last-seen time" || bad "no lastSeenAt"
check "the machine no longer shares" "$(code -X PUT $BASE/api/runners/$RID/shared-projects -H 'Content-Type: application/json' -d '{"projectIds":[]}')" 404
check "the machine no longer holds a quota" "$(code -X PUT $BASE/api/runners/$RID/run-limit -H 'Content-Type: application/json' -d '{"runLimit":5}')" 404

echo "== the usage console =="
check "create the reporting source" "$(code -X POST $BASE/api/insights/v1/sources -H 'Content-Type: application/json' -d '{"projectId":"demo","name":"omp","expiresInDays":30}')" 201
SRCTOK=$(json 'd["token"]')
check "report a finished session" "$(code -X POST $BASE/api/insights/v1/ingest -H 'Content-Type: application/json' -H "Authorization: Bearer $SRCTOK" -d '{"id":"exec-1","type":"workflow_execution","externalExecutionId":"exec-1","workflow":{"id":"omp","name":"omp"},"status":"succeeded","startedAt":"2026-11-28T05:00:00Z","finishedAt":"2026-11-28T05:10:00Z","usage":[{"provider":"anthropic","model":"claude-opus-4","inputTokens":1200,"outputTokens":500,"cacheReadTokens":200,"costUsd":0.25,"costStatus":"reported"}]}')" 201
check "read the console" "$(code $BASE/api/usage)" 200
EXECS=$(json 'd["summary"]["executions"]'); TOKENS=$(json 'd["summary"]["totalTokens"]'); COST=$(json 'd["summary"]["totalCostUsd"]')
[ "$EXECS" = "1" ] && ok "the console shows the execution" || bad "executions = $EXECS"
[ "$TOKENS" = "1700" ] && ok "the tokens are summed" || bad "tokens = $TOKENS"
[ "$COST" = "0.25" ] && ok "the reported cost is shown" || bad "cost = $COST"

echo "== the tracker import =="
check "GitLab with no credential fails visibly" "$(code -X POST $BASE/api/tracker/import -H 'Content-Type: application/json' -d '{"provider":"gitlab","projectId":"demo"}')" 200
REASON=$(json 'd["run"]["reason"]')
case "$REASON" in *GitLab*) ok "the failure names GitLab and the missing credential";; *) bad "reason = $REASON";; esac
check "bind a GitLab repository" "$(code -X POST $BASE/api/repositories -H 'Content-Type: application/json' -d '{"projectId":"demo","gongfengProjectId":"gitlab:gitlab.com/me/repo"}')" 200
PROV=$(json 'd["provider"]')
[ "$PROV" = "gitlab" ] && ok "the repository is a GitLab one" || bad "provider = $PROV"
check "the import now finds it" "$(code -X POST $BASE/api/tracker/import -H 'Content-Type: application/json' -d '{"provider":"gitlab","projectId":"demo"}')" 200
FOUND=$(json 'len(d["run"].get("repositories") or [])')
[ "$FOUND" = "1" ] && ok "the import matched the repository" || bad "matched = $FOUND"

echo "== the removed surfaces answer nothing =="
for p in /api/runs /api/agents /api/workflows /api/sandboxes /api/schedule-jobs /api/tasks /api/users /api/sync/status /api/auth/tokens /api/platform/admins; do
  check "GET $p is gone" "$(code $BASE$p)" 404
done
check "dispatch is gone" "$(code -X POST $BASE/api/issues/any/dispatch -H 'Content-Type: application/json' -d '{}')" 404

echo "== the console =="
for p in / /index.html /app/issues.js /app/runners.js /app/observability.js /app/workbench.js /i18n-en.js /product-v4.css; do
  check "GET $p" "$(code $BASE$p)" 200
done

kill $PID 2>/dev/null; wait $PID 2>/dev/null
echo
echo "== restart keeps the data =="
AGENTFLOW_SESSION_SECRET=verify nohup "$BIN" -datasource "file:$DB" -addr 127.0.0.1:18099 >/tmp/mf-e2e/platform2.log 2>&1 &
PID=$!
for _ in $(seq 60); do curl -sf "$BASE/api/health" >/dev/null && break; sleep 1; done
check "the project survives the restart" "$(code $BASE/api/projects)" 200
PNAME=$(json 'd[0]["name"]'); [ "$PNAME" = "演示项目" ] && ok "the project name survives" || bad "name = $PNAME"
check "the reported usage survives the restart" "$(code $BASE/api/usage)" 200
EXECS=$(json 'd["summary"]["executions"]'); [ "$EXECS" = "1" ] && ok "the usage survives" || bad "executions = $EXECS"
code $BASE/api/model-catalog >/dev/null; CONF=$(json 'next(e["configured"] for e in d["engines"] if e["id"]=="codebuddy")'); [ "$CONF" = "True" ] && ok "the credentials survive" || bad "configured = $CONF"

echo
printf 'RESULT: %d passed, %d failed\n' "$pass" "$fail"
[ "$fail" = 0 ]