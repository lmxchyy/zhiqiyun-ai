# 知启云 AI V6.0 用户角色与权限体系

## 1. 角色模型

授权链路为 `User -> Tenant -> Organization -> Roles -> Permissions`。用户始终拥有 `USER`，业务角色采用叠加授权：代理商为 `USER + AGENT`，运营中心为 `USER + OPERATION`，同时经营两类业务时为 `USER + AGENT + OPERATION`。

`currentRole` 仅表示当前工作上下文。切换角色不会重新登录、不会刷新或替换 Token。

## 2. Permission Matrix

| Role | Permissions |
| --- | --- |
| USER | `ai:use`, `assets:view`, `project:view`, `wallet:view`, `settings:view` |
| AGENT | `agent:promotion`, `agent:promotion:create`, `agent:qrcode:view`, `agent:customer:view`, `agent:commission:view`, `agent:withdraw`, `agent:material:view` |
| OPERATION | `operation:dashboard`, `operation:agent:list`, `operation:agent:approve`, `operation:order:view`, `operation:customer:view`, `operation:report:view`, `operation:announcement:manage`, `operation:renew` |
| ENTERPRISE_ADMIN | `enterprise:member:manage`, `enterprise:organization:manage` |
| AI_ADMIN | `ai:admin` |
| FINANCE | `finance:view`, `finance:approve` |
| CUSTOMER_SERVICE | `customer-service:manage` |

当前角色为业务角色时，接口返回该角色权限与 USER 基础权限的并集。

## 3. 数据库变更

正式管理模型使用 `tenants`、`organizations`、`roles`、`permissions`、`user_roles`、`role_permissions`、`user_role_context`。Go 运行时使用兼容现有文本 ID 的 `xz_tenants`、`xz_organizations`、`xz_user_roles`、`xz_role_permissions`、`xz_user_role_context` 投影。

迁移会为所有用户回填 USER，并从有效的 `xz_channel_agents`、`xz_operation_centers` 回填 AGENT、OPERATION。租户优先使用用户已有的有效 tenant membership，否则落入默认租户。

## 4. Pinia Store

`useUserStore` 维护 `userId`、`tenantId`、`organizationId`、`roles`、`currentRole`、`permissions`，对外提供 `hasRole()`、`hasPermission()`、`loadProfile()`、`switchRole()`。切换成功后只刷新当前角色与权限，并同步本地授权缓存。

## 5. Router Guard

uni-app 使用全局导航拦截器覆盖 `navigateTo`、`redirectTo`、`reLaunch`、`switchTab`。代理商路由要求 AGENT 与对应 `agent:*` 权限，运营中心路由要求 OPERATION 与对应 `operation:*` 权限；拒绝访问时统一进入 `/pages/ForbiddenPage`。

## 6. API

- `GET /api/v1/user/profile`：返回 `userId`、`tenantId`、`organizationId`、`roles`、`currentRole`、`permissions`。
- `POST /api/v1/user/current-role`：Body 为 `{ "role": "AGENT" }`。仅允许切换到当前用户已拥有的角色，未授权返回 403，不签发新 Token。

## 7. 页面修改点

- “我的”只在可切换角色数量大于 1 时显示 Role Switcher。
- 角色选项严格来自 `roles`；没有 AGENT 时不显示代理商，没有 OPERATION 时不显示运营中心。
- 普通用户顶部保留“升级代理商”；已拥有 AGENT 时显示“代理商工作台”。
- 菜单统一由 `RoleMenuConfig[currentRole]` 生成，并通过 `hasPermission()` 过滤。
- USER、AGENT、OPERATION 共用一个工作台组件和全局 Store，不维护三套登录状态。
- 原“身份与权限”入口升级为“角色与权限”，展示当前角色、已授权角色与当前权限列表。

## 8. 后续可扩展角色

已预留 `ENTERPRISE_ADMIN`、`AI_ADMIN`、`FINANCE`、`CUSTOMER_SERVICE`。扩展新角色时只需补充角色种子、Permission Matrix、`AppRole` 类型、角色菜单和路由权限映射，不需要修改 Token 模型。
