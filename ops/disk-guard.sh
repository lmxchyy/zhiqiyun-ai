#!/bin/sh
set -eu

target="${1:-.}"
warn_percent="${DISK_WARN_PERCENT:-70}"
critical_percent="${DISK_CRITICAL_PERCENT:-80}"
emergency_percent="${DISK_EMERGENCY_PERCENT:-90}"
min_free_bytes="${DISK_MIN_FREE_BYTES:-10737418240}"

for value in "$warn_percent" "$critical_percent" "$emergency_percent" "$min_free_bytes"; do
  case "$value" in
    ''|*[!0-9]*)
      echo "disk_guard_error=invalid_numeric_threshold value=$value" >&2
      exit 64
      ;;
  esac
done

if [ "$warn_percent" -ge "$critical_percent" ] || [ "$critical_percent" -ge "$emergency_percent" ]; then
  echo "disk_guard_error=invalid_threshold_order" >&2
  exit 64
fi

stats="$(df -Pk "$target" | awk 'NR == 2 {gsub("%", "", $5); print $4, $5}')"
set -- $stats
available_kb="${1:-0}"
used_percent="${2:-100}"
available_bytes="$((available_kb * 1024))"
state=OK

if [ "$used_percent" -ge "$emergency_percent" ] || [ "$available_bytes" -le "$min_free_bytes" ]; then
  state=EMERGENCY
elif [ "$used_percent" -ge "$critical_percent" ]; then
  state=CRITICAL
elif [ "$used_percent" -ge "$warn_percent" ]; then
  state=WARNING
fi

message="disk_state=$state used_percent=$used_percent available_bytes=$available_bytes target=$target"
if [ "$state" = EMERGENCY ]; then
  echo "$message" >&2
  exit 2
fi

echo "$message"
