# ADR-006：资产由 `asset://` 清单和 SHA-256 resolver 管理

- 状态：Accepted
- 日期：2026-08-15

## 背景

renderer 直接抓取任意 URL 会引入 SSRF、超时、鉴权泄漏和不可复现输出。

## 决策

Deck IR 只保存 `asset://` URI、MIME type 和 SHA-256。renderer 不执行 HTTP；调用方必须提供 `resolveAsset(asset) -> bytes`。

renderer 在生成任何 PPTX 前校验全部声明资产的 digest。目录型 Phase 0 resolver 拒绝绝对路径、空段、`.`、`..` 和越界解析。

## 后果

- 相同 digest 对应相同输入 bytes。
- 生产 resolver 可以接作品中心或对象存储，但其 SDK 类型不能进入 renderer。
- 任一资产缺失或 digest 不一致时整次渲染失败，不生成部分 artifact。
