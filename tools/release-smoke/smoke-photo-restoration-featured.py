#!/usr/bin/env python3
"""Release smoke gate: featured inspirations must expose photo restoration comparison sources."""

import argparse
import json
import sys
import urllib.error
import urllib.request


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--base-url", default="https://ai.zs-kjhn.cn")
    parser.add_argument("--title", default="老照片修复")
    args = parser.parse_args()
    url = f"{args.base_url.rstrip('/')}/api/v1/inspirations/featured?category=recommend&offset=0&limit=20"
    try:
        with urllib.request.urlopen(url, timeout=20) as resp:
            payload = json.loads(resp.read().decode("utf-8"))
    except urllib.error.URLError as exc:
        print(f"FAIL cannot fetch featured: {exc}")
        return 2

    items = payload.get("items") or []
    target = None
    for item in items:
        if item.get("title") == args.title or item.get("scenarioCode") == "photo_restoration":
            target = item
            break
    if target is None:
        print(f"FAIL missing template title={args.title!r} in featured list (count={len(items)})")
        return 1

    display = target.get("displayConfig") or {}
    before = str(display.get("beforeUrl") or "").strip()
    after = str(display.get("afterUrl") or "").strip()
    scenario = str(target.get("scenarioCode") or "").strip()
    print(f"id={target.get('id')}")
    print(f"scenarioCode={scenario}")
    print(f"beforeUrl={before}")
    print(f"afterUrl={after}")
    if scenario != "photo_restoration":
        print("FAIL scenarioCode != photo_restoration")
        return 1
    if not before or not after:
        print("FAIL displayConfig.beforeUrl/afterUrl missing")
        return 1
    print("PASS photo restoration comparison sources present")
    return 0


if __name__ == "__main__":
    sys.exit(main())
