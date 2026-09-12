# P1 Connector Evidence

- **采集时间**：2026-09-09
- **结论**：`PARTIAL / NOT VERIFIED`

## 已验证

- Redis health：`PONG`。
- 真实本地 Redis integration test：

```text
XIANZHI_CONNECTOR_TEST_REDIS_URL=redis://127.0.0.1:63791/0
TestConnectorRedisQueueRecoveryAndAck: PASS
```

- 该测试验证 durable pending/working recovery 和成功 ack。
- 本地单测验证失败任务有界重试。
- 代码使用 Redis Lua 原子移动，避免 working remove 后 requeue/DLQ push 失败导致丢失。

## 未验证

以下真实 Redis 行为尚无独立运行证据：

```text
handler temporary failure
  -> retry 1
  -> retry 2
  -> retry 3
  -> durable DLQ
  -> manual recovery
```

现有 integration test 未覆盖失败到 DLQ 的完整路径，也没有 DLQ 告警、消费和人工恢复记录。

## 状态

`PARTIAL / REAL_RETRY_EXHAUSTION_AND_DLQ_NOT_VERIFIED`
