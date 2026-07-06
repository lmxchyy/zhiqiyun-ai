# 用户与代理双身份架构说明

## 设计原则

系统只把 `users` 作为唯一登录主体。所有账号默认都是用户身份，负责 AI 功能使用、作品、充值、订阅和 Token 消费。代理不是第二套账号，而是同一个 `user_id` 在代理资料表中的一条身份记录。

用户购买代理加盟包后，系统创建代理身份；账号登录后仍默认进入用户端，同时额外获得代理中心入口和代理权限。

## 数据边界

- `users`：唯一登录主体。
- `agent_profiles` / `xz_agent_profiles`：代理身份资料，按 `user_id` 唯一绑定。
- `user_wallets` / `xz_user_wallets`：用户钱包，只管理 AI Token、现金余额、冻结 Token、Token 发放与消耗。
- `agent_wallets` / `xz_agent_wallets`：代理佣金钱包，只管理佣金余额、可提现余额、冻结佣金、累计佣金和累计提现。
- `orders` / `xz_orders`：订单必须记录购买人和当时分润上下文。

订单关键字段：

- `buyer_user_id`：实际购买人。
- `direct_agent_id`：直接推荐代理。
- `parent_agent_id`：上级代理。
- `operation_center_id`：所属运营中心。
- `business_order_type`：业务订单类型，如 `USER_PACKAGE`、`AGENT_JOIN`、`TOKEN_RECHARGE`。
- `token_amount`：发放给用户钱包的 Token 权益。
- `reward_snapshot`：本次结算时的分润快照。

## 后端接口

- `POST /api/v1/orders/create`：用户套餐或 Token 充值订单。
- `POST /api/v1/agent/join-order`：代理身份开通订单。
- `POST /api/v1/operation-center/join-order`：运营中心开通订单。
- `POST /api/v1/pay/callback`：支付成功后执行订单履约、Token 发放、代理身份创建和佣金入账。
- `GET /api/v1/member/wallet`：用户身份钱包。
- `GET /api/v1/channel/me`：代理身份、团队、佣金、提现视图。

## 前端权限

- 所有人都展示用户端功能。
- 认证响应包含 `agent` 时，额外展示代理中心菜单。
- 代理用户默认 `workspace=user`、`defaultModule=dashboard`，避免登录后直接跳到代理后台。
- 代理中心权限使用 `channel.dashboard`、`channel.customers.read`、`channel.commissions.read`、`channel.withdrawals.create`。

## 结算规则

996 用户套餐：

- 用户获得 400 Token。
- 直属代理获得 300 元。
- 运营中心获得 200 元。
- 平台获得 96 元。

996 代理加盟：

- 给购买用户创建代理身份。
- 用户获得 200 Token。
- 直属代理获得 300 元。
- 运营中心获得 200 元。
- 平台获得 296 元。

二级代理规则：

- 代理2推广普通用户：代理2 获得 300 元，代理1 获得 50 元，运营中心获得 200 元。
- 代理2发展代理3：代理2 获得 300 元，代理1 不获得收益，运营中心获得 200 元。

代理商自己使用 AI 功能时走用户身份和 `user_wallets`，不消耗代理佣金钱包。代理商查看团队、收益、提现时走代理身份和 `agent_wallets`。

## 履约流程

1. 创建订单时写入 `buyer_user_id`、`business_order_type`、`token_amount` 和初始价格快照。
2. 支付回调将订单置为 `PAID`。
3. 按订单类型计算直属代理、上级代理、运营中心和平台收入。
4. 写入 `reward_snapshot`。
5. 给用户钱包发放 Token。
6. 如为代理加盟订单，创建或激活代理身份。
7. 写入佣金记录并刷新代理佣金钱包。

