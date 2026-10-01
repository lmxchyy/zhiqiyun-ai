#!/usr/bin/env python3
"""Fail closed: every real PostgreSQL A-E case must run and pass; SKIP is failure."""
import json
import sys

required = {f"TestIssue190Postgres{case}" for case in "ABCDE"} | {"TestIssue190PostgresConnector"}
passed = set()
for line in open(sys.argv[1], encoding="utf-8"):
    event = json.loads(line)
    name = event.get("Test", "")
    if not name.startswith("TestIssue190Postgres"):
        continue
    if event.get("Action") in {"skip", "fail"}:
        raise SystemExit(f"Issue190 integration BLOCKED: {name} {event['Action']}")
    if name in required and event.get("Action") == "pass":
        passed.add(name)
missing = required - passed
if missing:
    raise SystemExit(f"Issue190 integration BLOCKED: missing successful cases {sorted(missing)}")
print("Issue190 real PostgreSQL A-E + Connector: 6/6 executed and passed; no SKIP")
