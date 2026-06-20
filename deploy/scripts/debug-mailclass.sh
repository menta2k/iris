#!/usr/bin/env bash
# debug-mailclass.sh — collect data to diagnose an empty Logs "Class" column
# on a host-native iris + kumomta install. READ-ONLY (no restarts, no writes).
#
# Usage:
#   1) send one fresh test message through kumomta that carries the matching
#      header (e.g. X-GreenArrow-MailClass: homesbg_h)
#   2) sudo ./debug-mailclass.sh   2>&1 | tee /tmp/mailclass-debug.txt
#   3) send /tmp/mailclass-debug.txt back.
#
# It deliberately does NOT print secrets (redis/pg passwords are masked).

POL=${IRIS_KUMO_POLICY_DIR:-/opt/kumomta/etc/policy}/init.lua
ENVF=/etc/iris/iris.env
DATA=/etc/iris/configs/data.yaml

sec(){ printf '\n========== %s ==========\n' "$1"; }
mask(){ sed -E 's#(://[^:/@]+):[^@]*@#\1:***@#g'; }

echo "=== iris mail-class / Logs Class debug @ $(date -u +%FT%TZ) ==="
echo "host: $(hostname)"

# ---------------------------------------------------------------------------
sec "1. VERSIONS & SERVICE STATE"
( /usr/local/bin/iris --version 2>/dev/null || iris --version 2>/dev/null || echo "iris: not found" ) | head -1
( kumod --version 2>/dev/null || dpkg-query -W -f='kumomta ${Version}\n' kumomta 2>/dev/null || echo "kumod: unknown" ) | head -1
systemctl show kumomta -p ActiveEnterTimestamp -p SubState -p MainPID 2>/dev/null
systemctl show iris    -p ActiveEnterTimestamp -p SubState -p MainPID 2>/dev/null
echo "init.lua mtime: $(stat -c '%y' "$POL" 2>/dev/null)"
echo "(If kumomta ActiveEnterTimestamp is OLDER than init.lua mtime, kumod is"
echo " running a stale policy — init-level changes like the log hook need a restart.)"

# ---------------------------------------------------------------------------
sec "2. RUNNING POLICY (on disk = what an applied policy looks like)"
echo "--- configure_log_hook block ---"
grep -n -A10 'configure_log_hook' "$POL" 2>/dev/null
if grep -q "meta = { 'mailclass' }" "$POL" 2>/dev/null; then
  echo ">> meta allowlist: PRESENT"
else
  echo ">> meta allowlist: ABSENT (this build/render predates the fix, or hand-edited)"
fi
echo "--- mail-class match table ---"
grep -nE 'MAIL_CLASS_MATCH|MAIL_CLASS_HEADERS' "$POL" 2>/dev/null | head -20
echo "--- route sets mailclass meta? ---"
grep -n "set_meta('mailclass'" "$POL" 2>/dev/null || echo "(no set_meta('mailclass') found!)"

# ---------------------------------------------------------------------------
sec "3. IRIS LOGSTREAM CONFIG (consumer side)"
grep -E '^IRIS_(LOGSTREAM|KUMO|REDIS)' "$ENVF" 2>/dev/null | mask || echo "(no IRIS_LOGSTREAM_* in $ENVF)"

# ---------------------------------------------------------------------------
sec "4. REDIS STREAM kumo.events (what kumod actually emits)"
RURL=$(grep -E '^IRIS_LOGSTREAM_REDIS_URL=' "$ENVF" 2>/dev/null | cut -d= -f2-)
[ -z "$RURL" ] && RURL=$(grep -oE 'redis://[^"]+' "$POL" 2>/dev/null | head -1)
[ -z "$RURL" ] && RURL="redis://127.0.0.1:6379/0"
SNAME=$(grep -oE '"kumo\.events"' "$POL" 2>/dev/null | head -1 | tr -d '"'); [ -z "$SNAME" ] && SNAME="kumo.events"
echo "redis url : $(printf '%s' "$RURL" | mask)"
echo "stream    : $SNAME"
RC(){ redis-cli -u "$RURL" "$@" 2>/dev/null; }
echo "XLEN      : $(RC XLEN "$SNAME")"
echo "--- XINFO GROUPS (is the iris consumer attached / lagging?) ---"
RC XINFO GROUPS "$SNAME"
echo "--- latest 6 records: type | queue | meta | header keys ---"
RC XREVRANGE "$SNAME" + - COUNT 6 | python3 -c '
import sys, json, codecs
n=0
for line in sys.stdin:
    i,j=line.find("{"),line.rfind("}")
    if i<0 or j<=i: continue
    try: d=json.loads(codecs.decode(line[i:j+1],"unicode_escape"))
    except Exception: continue
    if isinstance(d,dict) and d.get("type"):
        n+=1
        print(d.get("type"),"| q:",d.get("queue"),"| meta:",d.get("meta"),
              "| hdr_keys:",list((d.get("headers") or {}).keys()))
print("parsed records:",n)
' 2>&1

# ---------------------------------------------------------------------------
sec "5. DB log_event — column vs raw meta (THE decisive comparison)"
PGURL=$(grep -oE 'postgres://[^"]+' "$DATA" 2>/dev/null | head -1)
if [ -z "$PGURL" ]; then
  echo "(could not find postgres:// url in $DATA)"
elif ! command -v psql >/dev/null; then
  echo "(psql not installed — skipping DB section)"
else
  echo "pg: $(printf '%s' "$PGURL" | mask | sed -E 's#(postgres://[^/]+/[^?]+).*#\1#')"
  echo "--- last 10 events: mail_class column vs extra_json meta/header ---"
  psql "$PGURL" -P pager=off -X -c "
    select
      to_char(at,'HH24:MI:SS') as t,
      event_type as type,
      coalesce(nullif(mail_class,''),'(empty)') as col_mail_class,
      coalesce(extra_json::jsonb #>> '{meta,mailclass}','-') as raw_meta_mailclass,
      coalesce(extra_json::jsonb #>> '{headers,X-GreenArrow-MailClass}','-') as raw_ga_header,
      queue
    from log_event
    order by at desc
    limit 10;" 2>&1 | head -30
  echo "--- does extra_json even contain a meta object on recent rows? ---"
  psql "$PGURL" -P pager=off -X -tAc "
    select event_type, (extra_json::jsonb ? 'meta') as has_meta_key,
           jsonb_typeof(extra_json::jsonb -> 'meta') as meta_type
    from log_event order by at desc limit 5;" 2>&1 | head -10
fi

# ---------------------------------------------------------------------------
sec "6. RECENT IRIS LOG LINES (consumer / persister errors)"
journalctl -u iris --since '20 min ago' --no-pager 2>/dev/null \
  | grep -iE 'logstream|persist|consum|mail.?class|redis|xread|xadd|error' | tail -25 \
  || echo "(no matching iris log lines / journalctl unavailable)"

echo
echo "=== END — please send this whole output back ==="
