# 代理商邀请注册与安卓 APK 分发（第一阶段）

## 上线前提

1. 应用数据库迁移 `077-agent-invite-apk-distribution.sql`。
   迁移会为缺失、重复、短码或格式不安全的历史代理邀请码生成 12 位安全随机码；
   旧码与新码映射保存在 `xz_agent_invite_code_migrations`，上线前应导出并通知受影响代理商。
2. 配置 `AGENT_INVITE_LANDING_BASE_URL=https://ai.zs-kjhn.cn`。
3. H5 构建需将 `VITE_API_BASE_URL` 指向公开 API 域名；同源部署可留空。
4. `ai.zs-kjhn.cn/d/{inviteCode}` 由 Go 服务跳转到同容器 `/h5/` 下的 uni-app H5 邀请页。
5. `ai.zs-kjhn.cn/android/latest` 由 Go 服务查询正式版本并返回 302。
6. 版本化 APK 由对象存储或 CDN 直接提供，Go 服务只返回 HTTP 302。
7. 生产环境必须配置真实短信服务与 Redis。不得启用开发验证码或本地会话回退。

执行迁移时必须让任意 SQL 错误立即终止，不能只依据 `psql` 进程退出表象判断成功：

```bash
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 \
  -f database/migrations/077-agent-invite-apk-distribution.sql
```

迁移后检查所有邀请码、迁移映射和唯一性：

```sql
SELECT count(*) AS invalid_invite_codes
FROM xz_channel_agents
WHERE invite_code !~ '^[A-Z0-9]{8,12}$';

SELECT agent_id, old_code, new_code, reason, migrated_at
FROM xz_agent_invite_code_migrations
ORDER BY migrated_at, agent_id;
```

`invalid_invite_codes` 必须为 `0`。迁移脚本允许重复执行；已生成的新邀请码由迁移映射表固定，不会在重跑时再次变化。

公开邀请页不会读取代理商账号姓名、手机号或邮箱。需要展示品牌别名时，由平台审核后写入代理商档案的公开字段；未配置时统一显示“知启云AI合作代理商”：

```sql
UPDATE xz_channel_agents
SET raw = jsonb_set(
      coalesce(raw, '{}'::jsonb),
      '{inviteDisplayName}',
      to_jsonb('已审核的公开品牌名'::text),
      true
    ),
    updated_at = now()::text
WHERE id = '<代理商ID>';
```

`inviteDisplayName` 只允许填写确认可公开的品牌名，不得填写手机号、邮箱、身份证件信息或其他个人敏感信息。

## 发布新 APK

先在对象存储中上传不可变版本化文件，例如：

`android/releases/0.2.4/zhiqiyun-ai-0.2.4.apk`

上传后独立核对文件大小和 SHA-256。确认 CDN URL 能直接下载，再写入草稿：

```sql
INSERT INTO xz_app_releases (
  id, platform, channel, version_name, version_code, apk_url,
  file_size, sha256, release_notes, min_supported_version_code,
  force_update, status
) VALUES (
  'android_official_0_2_4', 'android', 'official', '0.2.4', 24,
  'https://cdn.example.com/android/releases/0.2.4/zhiqiyun-ai-0.2.4.apk',
  <APK_FILE_SIZE>, '<APK_SHA256>',
  '首个代理邀请灰度版本', 23, false, 'draft'
);
```

当前工作区已核对的正式构建候选为
`0.2.4` 当前只生成 App-Plus release resources，不得把历史 `0.2.3`
APK 的大小、哈希或签名信息复用到 `0.2.4` 版本记录。正式签名 APK 生成后，
必须独立记录文件大小、SHA-256 和签名证书指纹。

完成测试后，在一个事务里切换正式版本：

```sql
BEGIN;

UPDATE xz_app_releases
SET status = 'testing', updated_at = now()
WHERE platform = 'android' AND channel = 'official' AND status = 'published';

UPDATE xz_app_releases
SET status = 'published', published_at = now(), updated_at = now()
WHERE id = 'android_official_0_2_4' AND status IN ('draft', 'testing');

COMMIT;
```

发布后验证：

```text
GET /api/v1/public/app/releases/latest?platform=android&channel=official
GET /api/v1/public/app/releases/android/latest/download
GET /android/latest
```

两个下载入口都应返回 `302`、`Cache-Control: no-store`，且 `Location` 为版本化 CDN URL。

## 版本回滚

不要删除或覆盖 APK。将当前版本下架并重新发布旧版本：

```sql
BEGIN;

UPDATE xz_app_releases
SET status = 'disabled', updated_at = now()
WHERE platform = 'android' AND channel = 'official' AND status = 'published';

UPDATE xz_app_releases
SET status = 'published', published_at = now(), updated_at = now()
WHERE id = '<旧版本ID>' AND status IN ('testing', 'disabled');

COMMIT;
```

随后再次验证最新版查询和两个固定下载入口。

## 代理归属规则

- 只有有效且状态为 `ACTIVE` 的代理商邀请码可注册。
- 手机验证码验证成功后，用户创建和代理归属在同一数据库事务内完成。
- 正式归属写入 `xz_user_relationships`；`xz_users.referred_by` 同步写入代理商用户 ID，以兼容现有充值和分佣链路。
- 新用户首次绑定后锁定。唯一当前关系索引和历史保护触发器共同阻止覆盖。
- 已注册但没有正式关系的用户不会被扫码自动抢绑，接口返回人工处理提示。
- 同一代理关系的重复幂等请求返回原注册结果，不重复创建用户、关系或奖励。
- 邀请漏斗事件只用于统计，不作为佣金结算依据。

## 灰度检查

- 选取 1–3 个 ACTIVE 代理商，确认邀请码为 8–12 位随机值且互不重复。
- 分别用微信、安卓浏览器、iPhone 打开邀请链接。
- 使用未注册手机号完成一次真实短信注册，再用同一手机号登录 APP。
- 检查 `xz_user_relationships`、`xz_marketing_invite_records` 和 `xz_agent_invite_events`。
- 完成一次测试充值时，只检查原有分佣链路是否读取既有归属；本功能不会在注册时创建佣金。
- 关闭代理商后重新访问邀请链接，确认无法继续注册。

隔离测试库可通过以下环境变量启用真实 PostgreSQL 并发归属测试：

```bash
XIANZHI_AGENT_INVITE_TEST_DATABASE_URL='postgresql://用户:密码@127.0.0.1:端口/隔离测试库?sslmode=disable' \
go test ./internal/httpserver \
  -run TestAgentInvitePostgresConcurrentAttributionAndIdempotency \
  -count=1 -v
```

该测试会写入测试数据，只能指向一次性或专用测试数据库，禁止指向生产库。

## Phase 1.5 紧急关闭与启用顺序

生产首次部署必须保持以下配置：

```dotenv
AGENT_INVITE_REGISTRATION_ENABLED=false
APK_DOWNLOAD_ENABLED=false
APP_ACTIVATION_REPORT_ENABLED=false
```

- 关闭邀请注册时，邀请码查询仍可返回有效状态，但
  `registrationAllowed=false`，注册接口返回 HTTP 503；普通登录不受影响。
- 关闭 APK 下载时，固定下载入口返回 HTTP 503、
  `APK_DOWNLOAD_DISABLED` 和明确维护提示，不写下载事件。
- 关闭激活上报时，接口返回 HTTP 200、`reportingEnabled=false`，
  不写激活事件，也不阻断 App 使用。
- 完成 077、真实短信、正式签名 APK、版本化存储 URL 和回归验证后，
  按“激活上报 → APK 下载 → 邀请注册”的顺序逐项启用并逐项验证。
- 紧急停止灰度时优先关闭邀请注册和 APK 下载；已有用户登录、充值与
  现有分佣链路不依赖这些开关。

## 短信限频配置

```dotenv
SMS_REDIS_NAMESPACE=zhiqiyun:production:sms
SMS_MOBILE_DAILY_LIMIT=10
SMS_DEVICE_DAILY_LIMIT=20
SMS_IP_DAILY_LIMIT=50
```

每日窗口按 Asia/Shanghai 自然日计算。现有手机号 60 秒发送间隔、
验证码 5 分钟有效期和最多 5 次错误保持不变。Redis 读写异常时短信接口
失败关闭，不得绕过限频继续调用供应商。

启用邀请注册前还必须提供并通过启动配置校验：
`SMS_PROVIDER_URL`、`SMS_PROVIDER_API_KEY`、`SMS_TEMPLATE_ID` 和
`SMS_SIGNATURE`。当前适配器是通用 HTTP 边界，接入供应商前必须按真实
请求与响应协议完成适配，不能用占位响应冒充发送成功。

## 生产迁移的显式选择

`compose.prod.yml` 的 migrate 服务默认不执行任何迁移。只允许在完成备份
恢复演练并复核文件清单后显式选择：

```bash
MIGRATION_FILES=077-agent-invite-apk-distribution.sql \
docker compose -f compose.prod.yml run --rm migrate
```

不要使用通配符，也不要把 074–078 一并交给部署流程自动执行。迁移完成后，
先核对邀请码、历史关系和新表，再部署应用；本阶段不得在生产执行此命令。

## 0.2.4 正式打包要求

- `versionName=0.2.4`、`versionCode=24`、`VITE_APP_VERSION=0.2.4`。
- 包名保持 `com.zhiqiyun.xianzhiai`，DCloud AppID 保持
  `__UNI__F1BEA6A`。
- `build:app-plus:release` 只生成原生资源，不生成可发布的正式 APK。
- 必须使用与 0.2.3 完全相同的正式签名证书，并用 Android 官方
  `apksigner verify --verbose --print-certs` 验证 V1/V2/V3 及 SHA-256
  指纹。无法确认签名连续性时不得发布 0.2.4。

## 尚需人工完成

- APK 上传、CDN/对象存储公开 URL、文件大小和 SHA-256 核验。
- 正式短信供应商、Redis、H5/API/下载域名证书与反向代理配置。
- 执行数据库迁移并插入首条正式 APK 版本记录。
- 安卓真机安装、系统浏览器下载以及 APP 首次激活埋点联调。
