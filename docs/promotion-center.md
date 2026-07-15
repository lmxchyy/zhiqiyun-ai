# 推广中心 / 我的推广码

## 页面路由

- `/pages/promotion/PromotionCenterPage`：推广中心与专属小程序码
- `/pages/promotion/PromotionTemplateCenterPage`：10 套固定模板
- `/pages/promotion/PromotionPosterPreviewPage`：3:4 海报预览、保存与分享
- `/pages/promotion/PromotionRecordsPage`：访问、注册、成交记录
- `/pages/promotion/PromotionStatsPage`：趋势、转化率与来源渠道
- `/pages/promotion/PromotionLandingPage`：扫码与分享归因入口

原普通用户“邀请推广”和代理商“推广中心”入口均指向统一推广中心，根据 `currentRole` 动态展示可用模板。底部首页、创作、作品、我的四个 Tab 未改变。

## 服务端接口

所有业务接口使用现有 Bearer Token，并从会话读取 `userId`、`tenantId`、`organizationId`、`roles` 和 `currentRole`。

- `GET /api/v1/promotion/overview`
- `GET /api/v1/promotion/profile`
- `GET /api/v1/promotion/poster-templates`
- `POST /api/v1/promotion/miniprogram-code`
- `POST /api/v1/promotion/poster/render`
- `GET /api/v1/promotion/records`
- `GET /api/v1/promotion/analytics`
- `GET /api/v1/promotion/activities`
- `GET /api/v1/promotion/share-copy`
- `POST /api/v1/promotion/visit`
- `POST /api/v1/promotion/bind`

归因只允许首次有效绑定，禁止自邀、循环邀请和覆盖已有邀请关系。成交与奖励由服务端订单和分润数据计算，前端没有发奖接口。

## 微信小程序码配置

服务端环境变量：

```dotenv
WECHAT_MINI_PROGRAM_APPID=
WECHAT_MINI_PROGRAM_SECRET=
WECHAT_MINI_PROGRAM_ENV_VERSION=develop
```

`WECHAT_MINI_PROGRAM_SECRET` 只配置在后端运行环境，不写入 `VITE_*`、小程序包、页面参数或日志。

- `develop`：开发版路径；缺少微信配置时返回可扫描的本地联调码，并明确返回 `isPlaceholder: true`。
- `trial`：体验版路径。
- `release`：正式版路径；生产环境缺少微信配置会返回 503，不允许占位码伪装成正式小程序码。

正式发布前需确认 `pages/promotion/PromotionLandingPage` 已包含在微信发布版本中，并在微信后台配置合法业务域名。

## 海报与缓存

- 海报逻辑尺寸为 1080 × 1440，PNG，比例 3:4。
- 10 套模板由 `template_id` 和绘制配置驱动，不复制十份页面。
- 小程序码缓存键包含用户、租户、模板和活动；切换账号不会复用其他账号缓存。
- 访问记录按邀请人、访客、自然日、模板和活动幂等。
- 海报保存前检查相册权限；拒绝后引导用户打开设置。
