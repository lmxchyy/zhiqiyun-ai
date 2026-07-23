#!/bin/sh
set -eu

base_url="${WORKS_PERF_BASE_URL:-http://127.0.0.1:3199}"
email="${LOGIN_EMAIL:-demo@xianzhi.ai}"
password="${LOGIN_PASSWORD:-Demo123!}"

auth="$(
  wget -q -O - \
    --header="Content-Type: application/json" \
    --post-data="{\"email\":\"${email}\",\"password\":\"${password}\"}" \
    "${base_url}/api/v1/auth/login"
)"
token="$(printf '%s' "${auth}" | sed -n 's/.*"accessToken":"\([^"]*\)".*/\1/p')"
test -n "${token}"

index=1
while [ "${index}" -le 10 ]; do
  output="/tmp/recent-works-${index}.json"
  wget -q -O "${output}" \
    --header="Authorization: Bearer ${token}" \
    "${base_url}/api/v1/works/recent?limit=20"
  printf 'request=%s bytes=%s\n' "${index}" "$(wc -c <"${output}")"
  index=$((index + 1))
done

grep -F '[works-perf]' /tmp/works-perf-final.log | tail -n 10
