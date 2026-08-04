# Release smoke gates

Run before production API cutover:

`ash
python3 tools/release-smoke/smoke-photo-restoration-featured.py --base-url https://ai.zs-kjhn.cn
`

This guards against regressions where inspiration featured responses omit displayConfig / scenarioCode for photo restoration.
