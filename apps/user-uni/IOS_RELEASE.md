# iOS 构建与发布

当前 iOS 与 Android 共用 `src` 下的 uni-app 业务代码。iOS Bundle ID 为 `com.zhiqiyun.xianzhiai`，最低系统版本为 iOS 13，当前只发布 iPhone 版。

## 构建前准备

1. 确认 DCloud 应用 `__UNI__F1BEA6A` 仍绑定在当前开发者账号下。
2. 在 Apple Developer 后台为 `com.zhiqiyun.xianzhiai` 创建 App ID。
3. 准备与该 Bundle ID 匹配的证书私钥文件（`.p12`）、密码和描述文件（`.mobileprovision`）。测试安装使用 Development/Ad Hoc 描述文件，上架使用 Distribution/App Store 描述文件。
4. 复制 `.env.production.example` 为 `.env.production`，把 `VITE_API_BASE_URL` 改成手机可访问的正式 HTTPS API。

证书、描述文件和密码只在 HBuilderX 打包窗口中选择或输入，不提交到 Git，也不写入 `manifest.json`。

## 本地资源构建

```powershell
cd E:\code\work\先知AI\apps\user-uni
npm.cmd run typecheck
npm.cmd run build:app-ios
```

生成的 App 资源位于 `dist/build/app`。它是 iOS/Android 共用的前端资源，不是可以直接安装的 IPA。

## 正式版检查与云打包

```powershell
cd E:\code\work\先知AI\apps\user-uni
npm.cmd run check:app-ios-release
npm.cmd run build:app-ios:release
```

检查通过后，在 HBuilderX 中选择“发行 -> 原生 App-云打包”，勾选 iOS，填写 Bundle ID、`.p12` 密码并选择 `.p12` 与 `.mobileprovision`。打包完成后下载 IPA，再使用 TestFlight 或受信任的安装方式测试。

## 当前功能边界

- 手机号验证码和账号密码登录、AI 创作、作品、资产、企业中心等沿用 App 共用实现。
- App 端不会创建微信小程序虚拟支付订单。iOS 数字权益购买在 StoreKit/Apple IAP 服务端验签与权益发放完成前保持关闭，避免形成无效订单或违反上架规则。
