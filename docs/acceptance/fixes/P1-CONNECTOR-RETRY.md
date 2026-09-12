# P1 Connector Retry / DLQ

## 问题

Connector Redis worker 在 handler 返回错误后会直接从 working list 删除消息，失败消息没有 durable retry 或 DLQ；Redis 不可用时的内存队列也没有进程级恢复能力。

## 根因

`connectorJobQueue.Run` 原先只区分 dequeue 与 ack，没有失败计数和终态队列。

## 修改

- 增加 Redis `attempts` hash，按 `message_id` 记录失败次数。
- handler 失败时最多重试 3 次；未达到上限的消息从 working 重新放回 pending。
- 达到上限或 payload 非法时移入 durable Redis DLQ；working -> pending/DLQ 使用 Redis Lua 原子移动，避免 remove 成功而 push 失败造成丢失。
- 成功处理后清理 attempt 计数。
- Redis 不可用的内存降级路径执行有界 3 次重试，并明确记录本地模式无法跨进程恢复的事实。
- 扩展 Redis 集成测试清理范围，并增加本地模式重试/有界失败测试。

## 测试

```text
cd backend-go && go test -count=1 ./internal/httpserver -run 'TestConnector(LocalQueue|RedisQueue)'
PASS
```

Redis integration test 在未设置 `XIANZHI_CONNECTOR_TEST_REDIS_URL` 时按既有约定 skip；真实 Redis DLQ 验证仍需在带 Redis 的环境执行。

## 状态

`IMPLEMENTED_LOCALLY / REAL_REDIS_DLQ_NOT_VERIFIED`
