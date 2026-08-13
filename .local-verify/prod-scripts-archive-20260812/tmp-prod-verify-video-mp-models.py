#!/usr/bin/env python3
import json
import urllib.request

req = urllib.request.Request(
    "https://ai.zs-kjhn.cn/api/v1/models",
    headers={
        "X-Client-Platform": "mp-weixin",
        "X-Client-Name": "zhiqiyun-mini-program",
    },
)
with urllib.request.urlopen(req, timeout=30) as resp:
    items = json.load(resp)

videos = []
for item in items:
    caps = [str(c).upper() for c in (item.get("capabilities") or [])]
    if any(c in caps for c in ("TEXT_TO_VIDEO", "IMAGE_TO_VIDEO")) or item.get("videoCapabilities") or item.get("video_capabilities"):
        videos.append(item)

print("count=", len(videos))
for item in videos:
    print(item.get("code"), "|", item.get("name"), "|", item.get("priceHint"), "|", item.get("capabilityHint"))
