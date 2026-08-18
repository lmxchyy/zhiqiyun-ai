#!/bin/sh
set -eu

target="${1:-/host-volume}"
interval="${DISK_MONITOR_INTERVAL_SECONDS:-60}"
previous_state=""

while true; do
  set +e
  result="$(sh /usr/local/bin/disk-guard.sh "$target" 2>&1)"
  set -e
  state="$(printf '%s\n' "$result" | sed -n 's/.*disk_state=\([^ ]*\).*/\1/p')"
  state="${state:-UNKNOWN}"
  printf '%s\n' "$state" > /tmp/disk-state
  if [ "$state" != "$previous_state" ]; then
    printf '%s\n' "$result"
    previous_state="$state"
  fi
  sleep "$interval"
done
