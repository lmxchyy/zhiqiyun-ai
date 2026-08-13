#!/usr/bin/env bash
set -euo pipefail
for path in / /app /app/ /app/works /app/smart-video /admin/ /admin/assets/index-DFhJ-FhG.js /admin/index.html; do
  echo "=== $path ==="
  curl -sS -o /tmp/body -w 'http=%{http_code} redirect=%{redirect_url} type=%{content_type} size=%{size_download}\n' "https://ai.zs-kjhn.cn$path" || true
  if [ -f /tmp/body ]; then
    head -c 180 /tmp/body; echo
    grep -oE 'assets/index-[^" ]+\.js|montageWork|SMART_VIDEO_MONTAGE|userWorks' /tmp/body | head -10 || true
  fi
done

# Follow redirects for /app/works
echo "=== follow /app/works ==="
curl -sS -L -o /tmp/works2.html -w 'http=%{http_code} url_effective=%{url_effective}\n' "https://ai.zs-kjhn.cn/app/works"
grep -oE 'assets/index-[^" ]+\.js|/admin/assets/index-[^" ]+\.js' /tmp/works2.html | head -10 || true
JS=$(grep -oE '/admin/assets/index-[^" ]+\.js|/assets/index-[^" ]+\.js' /tmp/works2.html | head -1 || true)
echo "js=$JS"
if [ -n "$JS" ]; then
  case "$JS" in
    http*) URL="$JS" ;;
    /*) URL="https://ai.zs-kjhn.cn$JS" ;;
    *) URL="https://ai.zs-kjhn.cn/$JS" ;;
  esac
  curl -sS "$URL" -o /tmp/app.js
  grep -o 'montageWork\|SMART_VIDEO_MONTAGE\|AI自动混剪成片\|含生图与混剪' /tmp/app.js | sort | uniq -c || echo NO_MARKERS
fi
echo DONE
