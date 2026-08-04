const fs = require("node:fs");
const path = require("node:path");

const root = path.resolve(__dirname, "..");
const repoRoot = path.resolve(root, "../..");
const outputRoot = path.resolve(root, "dist", "build", "mp-weixin");
const pageRoot = path.resolve(outputRoot, "pages");
const assetRoot = path.resolve(outputRoot, "assets");

if (!fs.existsSync(pageRoot)) {
  console.error("mp-weixin pages directory not found. Run uni build first.");
  process.exit(1);
}

const logoFile = fs.existsSync(assetRoot)
  ? fs.readdirSync(assetRoot).find((name) => /^zhiqiyun-logo-transparent\..+\.png$/.test(name))
  : "";
const logoPath = "/static/brand/zhiqiyun-ai-logo.jpg";

function readEnvValue(filePath, key) {
  if (!fs.existsSync(filePath)) return "";
  const lines = fs.readFileSync(filePath, "utf8").split(/\r?\n/);
  for (const rawLine of lines) {
    const line = rawLine.trim();
    if (!line || line.startsWith("#")) continue;
    const index = line.indexOf("=");
    if (index <= 0) continue;
    if (line.slice(0, index).trim() !== key) continue;
    return line.slice(index + 1).trim().replace(/^["']|["']$/g, "");
  }
  return "";
}

const apiBaseFromFile =
  readEnvValue(path.resolve(root, ".env.local"), "VITE_API_BASE_URL") ||
  readEnvValue(path.resolve(root, ".env"), "VITE_API_BASE_URL") ||
  readEnvValue(path.resolve(repoRoot, ".env.local"), "VITE_API_BASE_URL") ||
  readEnvValue(path.resolve(repoRoot, ".env"), "VITE_API_BASE_URL");
const apiBase = String(process.env.VITE_API_BASE_URL || apiBaseFromFile || "http://127.0.0.1:3100").replace(/\/+$/, "");
const enableMockLogin = String(
  process.env.VITE_ENABLE_MOCK_LOGIN ||
    readEnvValue(path.resolve(root, ".env.local"), "VITE_ENABLE_MOCK_LOGIN") ||
    readEnvValue(path.resolve(root, ".env"), "VITE_ENABLE_MOCK_LOGIN") ||
    ""
).toLowerCase() === "true";

const wxml = `<view class="page">
  <view class="brand">
    <image wx:if="{{logo}}" class="logo" src="{{logo}}" mode="aspectFit" />
    <view>
      <text class="eyebrow">微信小程序</text>
      <text class="title">知启云 AI</text>
    </view>
  </view>

  <view class="heading">
    <text class="headline">登录知启云 AI</text>
    <text class="copy">当前页面使用小程序原生事件，支持账号密码登录和微信登录联调。</text>
  </view>

  <view class="card">
    <view class="field">
      <text>邮箱</text>
      <input value="{{email}}" bindinput="onEmailInput" placeholder="demo@xianzhi.ai" />
    </view>
    <view class="field">
      <text>密码</text>
      <input value="{{password}}" bindinput="onPasswordInput" password placeholder="请输入密码" />
    </view>

    <button class="button test" bindtap="testTap">点击测试 {{tapCount}}</button>
    <button class="button primary" loading="{{passwordLoading}}" disabled="{{busy}}" bindtap="passwordLogin">
      {{passwordText}}
    </button>
    <button class="button wechat" loading="{{wechatLoading}}" disabled="{{busy}}" open-type="getPhoneNumber" bindgetphonenumber="realWxPhoneLogin">
      {{wechatText}}
    </button>
${enableMockLogin ? `    <button class="button mock" loading="{{mockLoading}}" disabled="{{busy}}" bindtap="mockLogin">
      {{mockText}}
    </button>` : ""}

    <view class="status {{tone}}">
      <text>{{status}}</text>
    </view>

    <view class="debug">
      <text>API: {{apiBase}}</text>
      <text>如果点击测试有效但登录失败，状态区会显示接口返回信息。</text>
    </view>
  </view>
</view>
`;

const js = `"use strict";
const API_BASE = ${JSON.stringify(apiBase)};
const MOCK_LOGIN_ENABLED = ${JSON.stringify(enableMockLogin)};

function messageOf(error, fallback) {
  if (error instanceof Error && error.message) return error.message;
  if (error && typeof error === "object" && typeof error.errMsg === "string") return error.errMsg;
  return fallback;
}

function buildUrl(path) {
  if (/^https?:\\/\\//i.test(path)) return path;
  return API_BASE + (path.startsWith("/") ? path : "/" + path);
}

function api(path, options) {
  const token = wx.getStorageSync("token") || "";
  const headers = Object.assign({ "Content-Type": "application/json" }, options && options.headers ? options.headers : {});
  if (token) headers.Authorization = "Bearer " + token;
  return new Promise((resolve, reject) => {
    wx.request({
      url: buildUrl(path),
      method: options && options.method ? options.method : "GET",
      header: headers,
      data: options && Object.prototype.hasOwnProperty.call(options, "body") ? JSON.parse(String(options.body)) : undefined,
      timeout: 15000,
      success: (response) => {
        const body = response && response.data && typeof response.data === "object" ? response.data : {};
        if (response.statusCode < 200 || response.statusCode >= 300 || (body.code && body.code !== "0")) {
          reject(new Error(String(body.message || body.error || response.data || "HTTP " + response.statusCode)));
          return;
        }
        resolve(Object.prototype.hasOwnProperty.call(body, "data") ? body.data : body);
      },
      fail: (error) => reject(new Error(messageOf(error, "request failed")))
    });
  });
}

Page({
  data: {
    logo: ${JSON.stringify(logoPath)},
    apiBase: API_BASE,
    email: "demo@xianzhi.ai",
    password: "Demo123!",
    tapCount: 0,
    passwordLoading: false,
    wechatLoading: false,
    mockLoading: false,
    passwordText: "账号密码登录",
    wechatText: "微信登录联调",
    mockText: "模拟登录测试",
    busy: false,
    status: "准备就绪，可以先点击测试，也可以直接登录。",
    tone: "idle"
  },

  setStatus(status, tone) {
    this.setData({ status, tone: tone || "idle" });
  },

  setBusy(key, value) {
    const patch = {};
    patch[key] = value;
    patch.busy = value;
    if (key === "passwordLoading") patch.passwordText = value ? "账号密码登录中..." : "账号密码登录";
    if (key === "wechatLoading") patch.wechatText = value ? "微信登录中..." : "微信登录联调";
    if (key === "mockLoading") patch.mockText = value ? "模拟登录中..." : "模拟登录测试";
    this.setData(patch);
  },

  onEmailInput(event) {
    this.setData({ email: event.detail.value });
  },

  onPasswordInput(event) {
    this.setData({ password: event.detail.value });
  },

  testTap() {
    const next = this.data.tapCount + 1;
    this.setData({
      tapCount: next,
      status: "点击事件正常，次数=" + next,
      tone: "success"
    });
  },

  async completeLogin(auth, source) {
    const token = auth && auth.accessToken ? auth.accessToken : "";
    if (!token) throw new Error("missing accessToken");
    wx.setStorageSync("token", token);
    wx.setStorageSync("auth", auth || {});
    wx.setStorageSync("xianzhiMiniProgramAuth", auth || {});
    if (auth && auth.refreshToken) wx.setStorageSync("refreshToken", auth.refreshToken);
    const roles = Array.isArray(auth && auth.roles) ? auth.roles : ["USER"];
    const targetRole = roles.includes("OPERATION") ? "OPERATION" : roles.includes("AGENT") ? "AGENT" : "USER";
    let roleAccess;
    try {
      roleAccess = await api("/api/v1/user/current-role", {
        method: "POST",
        body: JSON.stringify({ role: targetRole })
      });
    } catch (error) {
      wx.removeStorageSync("token");
      wx.removeStorageSync("refreshToken");
      wx.removeStorageSync("auth");
      wx.removeStorageSync("xianzhiMiniProgramAuth");
      throw error;
    }
    const storedAuth = Object.assign({}, auth || {}, roleAccess || {}, { currentRole: targetRole });
    wx.setStorageSync("auth", storedAuth);
    wx.setStorageSync("xianzhiMiniProgramAuth", storedAuth);
    const landingPages = {
      USER: "/pages/user/UserHomePage",
      AGENT: "/pages/agent/AgentOverviewPage",
      OPERATION: "/pages/operation/OperationOverviewPage"
    };
    const landingPage = landingPages[targetRole] || landingPages.USER;
    setTimeout(() => {
      if (targetRole === "USER") {
        wx.switchTab({
          url: landingPage,
          fail: (error) => {
            wx.reLaunch({
              url: landingPage,
              fail: (relaunchError) => {
                this.setStatus(
                  "登录成功，但跳转工作台失败：" + messageOf(relaunchError || error, "跳转失败"),
                  "error"
                );
              }
            });
          }
        });
        return;
      }
      wx.reLaunch({
        url: landingPage,
        fail: (error) => {
          this.setStatus("登录成功，但跳转工作台失败：" + messageOf(error, "跳转失败"), "error");
        }
      });
    }, 300);
    this.setStatus(source + "成功，当前角色=" + targetRole, "success");
    wx.showToast({ title: "登录成功", icon: "success" });
  },

  async passwordLogin() {
    if (this.data.busy) return;
    if (!this.data.email || !this.data.password) {
      this.setStatus("请输入邮箱和密码。", "error");
      return;
    }
    this.setBusy("passwordLoading", true);
    this.setStatus("正在提交账号密码登录...", "loading");
    try {
      const auth = await api("/api/v1/auth/login", {
        method: "POST",
        body: JSON.stringify({
          email: this.data.email.trim(),
          password: this.data.password.trim()
        })
      });
      await this.completeLogin(auth, "账号密码登录");
    } catch (error) {
      this.setStatus("账号密码登录失败：" + messageOf(error, "登录失败"), "error");
    } finally {
      this.setBusy("passwordLoading", false);
    }
  },

${enableMockLogin ? `  async mockLogin() {
    if (this.data.busy) return;
    this.setBusy("mockLoading", true);
    this.setStatus("正在使用 mock-devtools-code 调用后端...", "loading");
    try {
      const auth = await api("/api/v1/auth/wechat/phone-login", {
        method: "POST",
        body: JSON.stringify({ wxLoginCode: "mock-devtools-code", phoneCode: "mock-phone-code" })
      });
      await this.completeLogin(auth, "模拟登录");
    } catch (error) {
      this.setStatus("模拟登录失败：" + messageOf(error, "模拟登录失败"), "error");
    } finally {
      this.setBusy("mockLoading", false);
    }
  },` : `  mockLogin() {
    this.setStatus("模拟登录未启用", "error");
  },`}

  realWxPhoneLogin(event) {
    if (this.data.busy) return;
    const detail = event && event.detail ? event.detail : {};
    const phoneCode = String(detail.code || "").trim();
    const errMsg = String(detail.errMsg || "");
    if (!phoneCode || !errMsg.toLowerCase().includes("ok")) {
      this.setStatus("需要授权手机号后才能微信登录", "error");
      return;
    }
    this.setBusy("wechatLoading", true);
    this.setStatus("正在调用 wx.login...", "loading");
    wx.login({
      success: async (result) => {
        if (!result.code) {
          this.setStatus("wx.login 未返回 code", "error");
          this.setBusy("wechatLoading", false);
          return;
        }
        this.setStatus("已获取微信 code，正在调用后端...", "loading");
        try {
          const auth = await api("/api/v1/auth/wechat/phone-login", {
            method: "POST",
            body: JSON.stringify({ wxLoginCode: result.code, phoneCode })
          });
          await this.completeLogin(auth, "微信登录");
        } catch (error) {
          this.setStatus("微信登录失败：" + messageOf(error, "微信登录失败"), "error");
        } finally {
          this.setBusy("wechatLoading", false);
        }
      },
      fail: (error) => {
        this.setStatus("wx.login 失败：" + messageOf(error, "wx.login 失败"), "error");
        this.setBusy("wechatLoading", false);
      }
    });
  }
});
`;

const wxss = `page {
  min-height: 100vh;
  background: #f6f9fe;
}

.page {
  min-height: 100vh;
  padding: 52px 24px 28px;
  box-sizing: border-box;
  color: #10233f;
  background:
    linear-gradient(150deg, rgba(37, 99, 235, 0.12), transparent 34%),
    linear-gradient(28deg, rgba(245, 111, 45, 0.1), transparent 30%),
    #f6f9fe;
}

.brand {
  display: flex;
  align-items: center;
  gap: 12px;
}

.logo {
  width: 74px;
  height: 74px;
  flex: 0 0 74px;
}

.brand view,
.heading,
.field,
.debug {
  display: flex;
  flex-direction: column;
}

.eyebrow {
  color: #2563eb;
  font-size: 13px;
  font-weight: 600;
}

.title {
  margin-top: 5px;
  color: #111827;
  font-size: 22px;
  font-weight: 700;
}

.heading {
  margin-top: 18px;
  gap: 10px;
}

.headline {
  color: #101c35;
  font-size: 30px;
  line-height: 1.15;
  font-weight: 700;
}

.copy {
  color: #65738c;
  font-size: 14px;
  line-height: 1.7;
}

.card {
  margin-top: 26px;
  padding: 18px;
  border: 1px solid rgba(24, 50, 88, 0.1);
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.92);
  box-shadow: 0 18px 42px rgba(22, 44, 78, 0.12);
}

.field {
  gap: 8px;
  margin-bottom: 14px;
  color: #4a5a70;
  font-size: 13px;
  font-weight: 700;
}

.field input {
  height: 46px;
  padding: 0 14px;
  box-sizing: border-box;
  border: 1px solid #d7dfec;
  border-radius: 8px;
  background: #f9fbff;
  color: #10233f;
  font-size: 15px;
}

.button {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 48px;
  margin: 0 0 14px;
  padding: 0 14px;
  border-radius: 8px;
  font-size: 15px;
  font-weight: 600;
  line-height: 1;
}

.button::after {
  display: none;
}

.button.test {
  color: #1d4ed8;
  background: #eff6ff;
  border: 1px solid #bfdbfe;
}

.button.primary {
  color: #fff;
  background: linear-gradient(135deg, #2563eb, #1ea7a1);
}

.button.wechat {
  color: #10233f;
  background: #fff;
  border: 1px solid rgba(31, 178, 116, 0.34);
}

.button.mock {
  color: #475467;
  background: #f8fafc;
  border: 1px dashed #cbd5e1;
}

.status {
  margin-top: 4px;
  padding: 12px;
  border-radius: 8px;
  color: #475467;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  font-size: 12px;
  line-height: 1.6;
}

.status.loading {
  color: #1d4ed8;
  background: #eff6ff;
  border-color: #bfdbfe;
}

.status.success {
  color: #047857;
  background: #ecfdf3;
  border-color: #bbf7d0;
}

.status.error {
  color: #b42318;
  background: #fef3f2;
  border-color: #fecaca;
}

.debug {
  gap: 6px;
  margin-top: 12px;
  color: #8b98aa;
  font-size: 11px;
  line-height: 1.5;
}
`;

const homeWxml = `<view class="home-page">
  <view class="home-top">
    <image wx:if="{{logo}}" class="home-logo" src="{{logo}}" mode="aspectFit" />
    <view class="home-title-block">
      <text class="home-eyebrow">{{texts.brandEyebrow}}</text>
      <text class="home-title">{{texts.title}}</text>
    </view>
  </view>

  <view class="home-card">
    <text class="home-status">{{texts.loggedIn}}</text>
    <text class="home-name">{{displayName}}</text>
    <text class="home-meta">{{texts.workspace}}: {{workspace}}</text>
    <text class="home-meta">{{texts.defaultModule}}: {{defaultModule}}</text>
  </view>

  <view class="module-grid">
    <view wx:for="{{modules}}" wx:key="id" data-id="{{item.id}}" class="module-card {{item.active ? 'active' : ''}}" bindtap="selectModule">
      <text class="module-icon">{{item.icon}}</text>
      <text class="module-title">{{item.title}}</text>
      <text class="module-copy">{{item.copy}}</text>
    </view>
  </view>

  <view class="module-panel">
    <view wx:if="{{activeModule === 'dashboard'}}" class="module-content">
      <text class="section-kicker">{{texts.dashboardKicker}}</text>
      <text class="section-title">{{texts.dashboardTitle}}</text>
      <text class="section-copy">{{texts.dashboardCopy}}</text>
      <view class="stat-row">
        <view class="stat-card">
          <text>{{assetCount}}</text>
          <text>{{texts.recentWorks}}</text>
        </view>
        <view class="stat-card">
          <text>{{workspace}}</text>
          <text>{{texts.currentIdentity}}</text>
        </view>
      </view>
    </view>

    <view wx:elif="{{activeModule === 'image'}}" class="module-content">
      <text class="section-kicker">{{texts.imageTitle}}</text>
      <text class="section-title">{{texts.imageSubtitle}}</text>
      <input class="module-input" value="{{imagePrompt}}" placeholder="{{texts.imagePlaceholder}}" bindinput="onImagePromptInput" />
      <view class="chip-row">
        <text class="chip active">1:1</text>
        <text class="chip">1024x1024</text>
        <text class="chip">gpt-image-2</text>
      </view>
      <button class="action-button" data-name="{{texts.imageTitle}}" bindtap="showModuleReady">{{texts.generateImage}}</button>
    </view>

    <view wx:elif="{{activeModule === 'video'}}" class="module-content">
      <text class="section-kicker">{{texts.videoTitle}}</text>
      <text class="section-title">{{texts.videoSubtitle}}</text>
      <input class="module-input" value="{{videoPrompt}}" placeholder="{{texts.videoPlaceholder}}" bindinput="onVideoPromptInput" />
      <view class="chip-row">
        <text class="chip active">doubao-seedance-2.0</text>
        <text class="chip">15s</text>
        <text class="chip">720p</text>
        <text class="chip">4:3</text>
      </view>
      <button class="action-button" data-name="{{texts.videoTitle}}" bindtap="showModuleReady">{{texts.generateVideo}}</button>
    </view>

    <view wx:elif="{{activeModule === 'ppt'}}" class="module-content">
      <text class="section-kicker">{{texts.pptTitle}}</text>
      <text class="section-title">{{texts.pptSubtitle}}</text>
      <input class="module-input" value="{{pptPrompt}}" placeholder="{{texts.pptPlaceholder}}" bindinput="onPptPromptInput" />
      <view class="chip-row">
        <text class="chip active">{{texts.fiveSlides}}</text>
        <text class="chip">{{texts.chinese}}</text>
        <text class="chip">{{texts.businessSimple}}</text>
      </view>
      <button class="action-button" data-name="{{texts.pptTitle}}" bindtap="showModuleReady">{{texts.generatePpt}}</button>
    </view>

    <view wx:else class="module-content">
      <text class="section-kicker">{{texts.assetsTitle}}</text>
      <text class="section-title">{{texts.assetsSubtitle}}</text>
      <text wx:if="{{assetsLoading}}" class="empty-text">{{texts.loadingAssets}}</text>
      <text wx:elif="{{assetsError}}" class="empty-text">{{assetsError}}</text>
      <view wx:elif="{{assets.length}}" class="work-list">
        <view wx:for="{{assets}}" wx:key="id" class="work-item">
          <image wx:if="{{item.isImage && item.thumbnailUrl}}" class="work-thumb" src="{{item.thumbnailUrl}}" mode="aspectFill" />
          <view wx:else class="work-file">
            <text>{{item.badge}}</text>
          </view>
          <view class="work-main">
            <text class="work-title">{{item.name}}</text>
            <text class="work-meta">{{item.mediaType}} · {{item.createdText}}</text>
          </view>
        </view>
      </view>
      <text wx:else class="empty-text">{{texts.noAssets}}</text>
      <button class="action-button secondary" bindtap="loadAssets">{{texts.refreshAssets}}</button>
    </view>
  </view>

  <button class="home-button" bindtap="refreshAuth">{{texts.refreshLogin}}</button>
  <button class="home-button ghost" bindtap="logout">{{texts.logout}}</button>
</view>
`;

const homeJs = `"use strict";
const API_BASE = ${JSON.stringify(apiBase)};
const zh = {
  brandEyebrow: "\\u5fae\\u4fe1\\u5c0f\\u7a0b\\u5e8f",
  title: "\\u77e5\\u542f\\u4e91 AI \\u5de5\\u4f5c\\u53f0",
  loggedIn: "\\u5df2\\u767b\\u5f55",
  workspace: "\\u5de5\\u4f5c\\u533a",
  defaultModule: "\\u9ed8\\u8ba4\\u6a21\\u5757",
  currentUser: "\\u5f53\\u524d\\u7528\\u6237",
  userHome: "\\u7528\\u6237\\u9996\\u9875",
  dashboardKicker: "\\u7528\\u6237\\u5de5\\u4f5c\\u53f0",
  dashboardTitle: "\\u9009\\u62e9\\u4e00\\u4e2a\\u80fd\\u529b\\u5f00\\u59cb\\u521b\\u4f5c",
  dashboardCopy: "\\u5c0f\\u7a0b\\u5e8f\\u7aef\\u5df2\\u63a5\\u5165\\u767b\\u5f55\\u6001\\uff0c\\u4e0b\\u9762\\u6309\\u7528\\u6237\\u7aef\\u6838\\u5fc3\\u6a21\\u5757\\u62c6\\u6210\\u8f7b\\u91cf\\u5de5\\u4f5c\\u533a\\u3002",
  recentWorks: "\\u8fd1\\u671f\\u4f5c\\u54c1",
  currentIdentity: "\\u5f53\\u524d\\u8eab\\u4efd",
  imageTitle: "AI \\u751f\\u56fe",
  imageSubtitle: "\\u751f\\u6210\\u7535\\u5546\\u4e3b\\u56fe\\u3001\\u6d77\\u62a5\\u548c\\u521b\\u610f\\u7d20\\u6750",
  imagePlaceholder: "\\u8bf7\\u8f93\\u5165\\u56fe\\u7247\\u63d0\\u793a\\u8bcd\\uff0c\\u4f8b\\u5982\\uff1a\\u751f\\u6210iphone17\\u7684\\u7535\\u5546\\u56fe",
  videoTitle: "\\u89c6\\u9891\\u751f\\u6210",
  videoSubtitle: "\\u751f\\u6210\\u77ed\\u89c6\\u9891\\u548c\\u8425\\u9500\\u89c6\\u9891\\u7d20\\u6750",
  videoPlaceholder: "\\u8bf7\\u8f93\\u5165\\u89c6\\u9891\\u811a\\u672c\\uff0c\\u4f8b\\u5982\\uff1a\\u5c0f\\u732b\\u5403\\u9c7c\\uff0c15\\u79d2\\uff0c720p\\uff0c4:3",
  pptTitle: "PPT \\u6587\\u6863\\u751f\\u6210",
  pptSubtitle: "\\u8f93\\u5165\\u4e3b\\u9898\\u751f\\u6210\\u6f14\\u793a\\u6587\\u7a3f",
  pptPlaceholder: "\\u8bf7\\u8f93\\u5165PPT\\u4e3b\\u9898\\uff0c\\u4f8b\\u5982\\uff1aAI\\u8d4b\\u80fd\\u4f01\\u4e1a\\u8425\\u9500\\u589e\\u957f\\u65b9\\u6848",
  assetsTitle: "\\u4f5c\\u54c1\\u4e2d\\u5fc3",
  assetsSubtitle: "\\u6700\\u8fd1\\u751f\\u6210\\u8bb0\\u5f55",
  loadingAssets: "\\u6b63\\u5728\\u52a0\\u8f7d\\u4f5c\\u54c1...",
  noAssets: "\\u6682\\u65e0\\u4f5c\\u54c1\\uff0c\\u5148\\u4ece AI \\u751f\\u56fe\\u3001\\u89c6\\u9891\\u751f\\u6210\\u6216 PPT \\u6587\\u6863\\u751f\\u6210\\u5f00\\u59cb\\u3002",
  refreshAssets: "\\u5237\\u65b0\\u4f5c\\u54c1",
  refreshLogin: "\\u5237\\u65b0\\u767b\\u5f55\\u72b6\\u6001",
  logout: "\\u9000\\u51fa\\u767b\\u5f55",
  generateImage: "\\u751f\\u6210\\u56fe\\u7247",
  generateVideo: "\\u751f\\u6210\\u89c6\\u9891",
  generatePpt: "\\u751f\\u6210 PPT",
  fiveSlides: "5\\u9875\\u5e7b\\u706f\\u7247",
  chinese: "\\u4e2d\\u6587",
  businessSimple: "\\u5546\\u52a1\\u7b80\\u7ea6",
  readySuffix: "\\u5c0f\\u7a0b\\u5e8f\\u7aef\\u5165\\u53e3\\u5df2\\u5c31\\u7eea\\uff0c\\u771f\\u5b9e\\u751f\\u6210\\u63a5\\u53e3\\u4e0b\\u4e00\\u6b65\\u63a5\\u5165",
  loginValid: "\\u767b\\u5f55\\u6709\\u6548",
  relogin: "\\u8bf7\\u91cd\\u65b0\\u767b\\u5f55",
  assetLoadFailed: "\\u4f5c\\u54c1\\u52a0\\u8f7d\\u5931\\u8d25"
};

function messageOf(error, fallback) {
  if (error instanceof Error && error.message) return error.message;
  if (error && typeof error === "object" && typeof error.errMsg === "string") return error.errMsg;
  return fallback;
}

function buildUrl(path) {
  if (/^https?:\\/\\//i.test(path)) return path;
  return API_BASE + (path.startsWith("/") ? path : "/" + path);
}

function api(path, options) {
  const token = wx.getStorageSync("token") || "";
  const headers = Object.assign({ "Content-Type": "application/json" }, options && options.headers ? options.headers : {});
  if (token) headers.Authorization = "Bearer " + token;
  return new Promise((resolve, reject) => {
    wx.request({
      url: buildUrl(path),
      method: options && options.method ? options.method : "GET",
      header: headers,
      data: options && Object.prototype.hasOwnProperty.call(options, "body") ? JSON.parse(String(options.body)) : undefined,
      timeout: 15000,
      success: (response) => {
        const body = response && response.data && typeof response.data === "object" ? response.data : {};
        if (response.statusCode < 200 || response.statusCode >= 300 || (body.code && body.code !== "0")) {
          reject(new Error(String(body.message || body.error || response.data || "HTTP " + response.statusCode)));
          return;
        }
        resolve(Object.prototype.hasOwnProperty.call(body, "data") ? body.data : body);
      },
      fail: (error) => reject(new Error(messageOf(error, "request failed")))
    });
  });
}

function formatDate(value) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return String(value);
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  const hour = String(date.getHours()).padStart(2, "0");
  const minute = String(date.getMinutes()).padStart(2, "0");
  return month + "/" + day + " " + hour + ":" + minute;
}

function normalizeAssets(items) {
  return (Array.isArray(items) ? items : []).slice(0, 5).map((asset) => {
    const mediaType = asset && asset.mediaType ? String(asset.mediaType) : "image";
    return {
      id: String(asset && asset.id ? asset.id : Math.random()),
      name: String(asset && asset.name ? asset.name : asset && asset.id ? asset.id : "-"),
      mediaType,
      thumbnailUrl: String(asset && (asset.thumbnailUrl || asset.url) ? asset.thumbnailUrl || asset.url : ""),
      isImage: mediaType === "image",
      badge: mediaType === "video" ? "\\u89c6" : mediaType === "document" ? "\\u6587" : "\\u56fe",
      createdText: formatDate(asset && asset.createdAt)
    };
  });
}

Page({
  data: {
    logo: ${JSON.stringify(logoPath)},
    texts: zh,
    displayName: zh.currentUser,
    workspace: "-",
    defaultModule: zh.userHome,
    activeModule: "dashboard",
    imagePrompt: "",
    videoPrompt: "",
    pptPrompt: "",
    assetCount: 0,
    assets: [],
    assetsLoading: false,
    assetsError: "",
    modules: []
  },

  onLoad() {
    this.resetModules("dashboard");
    this.readAuth();
    this.loadAssets();
  },

  onShow() {
    this.readAuth();
  },

  readAuth() {
    const token = wx.getStorageSync("token") || "";
    const auth = wx.getStorageSync("xianzhiMiniProgramAuth") || {};
    if (!token) {
      wx.reLaunch({ url: "/pages/WechatLoginPage" });
      return;
    }
    const user = auth.user || {};
    this.setData({
      displayName: user.name || user.email || zh.currentUser,
      workspace: auth.workspace || "-",
      defaultModule: auth.defaultModule || auth.defaultRoute || zh.userHome
    });
  },

  resetModules(activeId) {
    const modules = [
      { id: "image", icon: "\\u56fe", title: zh.imageTitle, copy: zh.imageSubtitle },
      { id: "video", icon: "\\u89c6", title: zh.videoTitle, copy: zh.videoSubtitle },
      { id: "ppt", icon: "P", title: zh.pptTitle, copy: zh.pptSubtitle },
      { id: "assets", icon: "\\u4f5c", title: zh.assetsTitle, copy: "\\u67e5\\u770b\\u5df2\\u751f\\u6210\\u5185\\u5bb9\\u548c\\u5386\\u53f2\\u8bb0\\u5f55" }
    ].map((item) => Object.assign({}, item, { active: item.id === activeId }));
    this.setData({ modules, activeModule: activeId });
  },

  selectModule(event) {
    const id = event.currentTarget.dataset.id || "dashboard";
    this.resetModules(id);
    if (id === "assets" && !this.data.assets.length) this.loadAssets();
  },

  onImagePromptInput(event) {
    this.setData({ imagePrompt: event.detail.value });
  },

  onVideoPromptInput(event) {
    this.setData({ videoPrompt: event.detail.value });
  },

  onPptPromptInput(event) {
    this.setData({ pptPrompt: event.detail.value });
  },

  showModuleReady(event) {
    const name = event.currentTarget.dataset.name || "";
    wx.showToast({ title: name + zh.readySuffix, icon: "none" });
  },

  refreshAuth() {
    this.readAuth();
    wx.showToast({ title: wx.getStorageSync("token") ? zh.loginValid : zh.relogin, icon: "none" });
  },

  async loadAssets() {
    if (!wx.getStorageSync("token")) return;
    this.setData({ assetsLoading: true, assetsError: "" });
    try {
      const items = await api("/api/v1/assets");
      const assets = normalizeAssets(items);
      this.setData({ assets, assetCount: assets.length });
    } catch (error) {
      this.setData({ assetsError: zh.assetLoadFailed + ": " + messageOf(error, zh.assetLoadFailed) });
    } finally {
      this.setData({ assetsLoading: false });
    }
  },

  logout() {
    wx.removeStorageSync("token");
    wx.removeStorageSync("refreshToken");
    wx.removeStorageSync("auth");
    wx.removeStorageSync("xianzhiMiniProgramAuth");
    wx.reLaunch({ url: "/pages/WechatLoginPage" });
  }
});
`;

const homeWxss = `page {
  min-height: 100vh;
  background: #f6f9fe;
}

.home-page {
  min-height: 100vh;
  padding: 52px 24px 28px;
  box-sizing: border-box;
  color: #10233f;
  background:
    linear-gradient(150deg, rgba(37, 99, 235, 0.12), transparent 34%),
    linear-gradient(28deg, rgba(245, 111, 45, 0.1), transparent 30%),
    #f6f9fe;
}

.home-top,
.module-card,
.work-item {
  display: flex;
  align-items: center;
}

.home-top {
  gap: 12px;
}

.home-logo {
  width: 72px;
  height: 72px;
  flex: 0 0 72px;
}

.home-title-block,
.home-card,
.module-content,
.work-main,
.stat-card {
  display: flex;
  flex-direction: column;
}

.home-title-block {
  gap: 5px;
}

.home-eyebrow {
  color: #2563eb;
  font-size: 13px;
  font-weight: 600;
}

.home-title {
  color: #111827;
  font-size: 22px;
  font-weight: 700;
}

.home-card,
.module-panel {
  margin-top: 18px;
  padding: 18px;
  border: 1px solid rgba(24, 50, 88, 0.1);
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.96);
  box-shadow: 0 14px 34px rgba(22, 44, 78, 0.1);
}

.home-card {
  gap: 8px;
  margin-top: 28px;
}

.home-status {
  align-self: flex-start;
  padding: 4px 10px;
  border-radius: 999px;
  color: #047857;
  background: #ecfdf3;
  border: 1px solid #bbf7d0;
  font-size: 12px;
  font-weight: 600;
}

.home-name {
  color: #101c35;
  font-size: 24px;
  font-weight: 700;
}

.home-meta,
.module-copy,
.section-copy,
.work-meta,
.empty-text {
  color: #64748b;
  font-size: 12px;
  line-height: 1.5;
}

.module-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  margin-top: 18px;
}

.module-card {
  min-height: 120px;
  padding: 14px;
  box-sizing: border-box;
  align-items: flex-start;
  flex-direction: column;
  border: 1px solid #dbe4f0;
  border-radius: 8px;
  background: #fff;
  box-shadow: 0 10px 24px rgba(22, 44, 78, 0.08);
}

.module-card.active {
  border-color: #2563eb;
  background: #eff6ff;
}

.module-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  border-radius: 8px;
  color: #fff;
  background: linear-gradient(135deg, #2563eb, #ff7a1a);
  font-size: 14px;
  font-weight: 700;
}

.module-title {
  margin-top: 12px;
  color: #111827;
  font-size: 15px;
  font-weight: 700;
}

.module-copy {
  margin-top: 6px;
}

.module-content {
  gap: 10px;
}

.section-kicker {
  color: #2563eb;
  font-size: 12px;
  font-weight: 700;
}

.section-title {
  color: #101c35;
  font-size: 20px;
  font-weight: 700;
  line-height: 1.25;
}

.stat-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
}

.stat-card {
  gap: 5px;
  padding: 12px;
  border-radius: 8px;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
}

.stat-card text:first-child {
  color: #111827;
  font-size: 18px;
  font-weight: 700;
}

.module-input {
  height: 48px;
  padding: 0 12px;
  box-sizing: border-box;
  border: 1px solid #d7dfec;
  border-radius: 8px;
  background: #f9fbff;
  color: #10233f;
  font-size: 14px;
}

.chip-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.chip {
  padding: 6px 10px;
  border-radius: 999px;
  color: #475467;
  background: #f1f5f9;
  border: 1px solid #e2e8f0;
  font-size: 12px;
  font-weight: 600;
}

.chip.active {
  color: #1d4ed8;
  background: #eff6ff;
  border-color: #bfdbfe;
}

.action-button,
.home-button {
  height: 46px;
  margin: 4px 0 0;
  border-radius: 8px;
  color: #fff;
  background: linear-gradient(135deg, #2563eb, #ff7a1a);
  font-size: 15px;
  font-weight: 700;
  line-height: 46px;
}

.home-button {
  margin-top: 18px;
  background: linear-gradient(135deg, #2563eb, #1ea7a1);
}

.action-button::after,
.home-button::after {
  display: none;
}

.action-button.secondary,
.home-button.ghost {
  color: #1d4ed8;
  background: #eff6ff;
  border: 1px solid #bfdbfe;
}

.home-button.ghost {
  color: #475467;
  background: #fff;
  border-color: #d7dfec;
}

.work-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.work-item {
  gap: 10px;
  padding: 10px;
  border-radius: 8px;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
}

.work-thumb,
.work-file {
  width: 58px;
  height: 58px;
  flex: 0 0 58px;
  border-radius: 8px;
  overflow: hidden;
  background: #e0f2fe;
}

.work-file {
  display: flex;
  align-items: center;
  justify-content: center;
  color: #2563eb;
  font-size: 18px;
  font-weight: 700;
}

.work-main {
  min-width: 0;
  flex: 1;
  gap: 5px;
}

.work-title {
  color: #111827;
  font-size: 14px;
  font-weight: 700;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
`;

const forceLegacyNativeLogin = String(process.env.XIANZHI_FORCE_LEGACY_NATIVE_LOGIN || "").toLowerCase() === "true";
if (forceLegacyNativeLogin) {
  fs.writeFileSync(path.resolve(pageRoot, "WechatLoginPage.wxml"), wxml);
  fs.writeFileSync(path.resolve(pageRoot, "WechatLoginPage.js"), js);
  fs.writeFileSync(path.resolve(pageRoot, "WechatLoginPage.wxss"), wxss);
} else {
  console.log("Preserved uni-app generated WechatLoginPage (set XIANZHI_FORCE_LEGACY_NATIVE_LOGIN=true only for legacy diagnostics).");
}

const loginEntryPageName = "WechatLoginPage";
const loginFormPageName = "WechatLoginFormPage";
if (!forceLegacyNativeLogin) {
  for (const extension of [".js", ".json", ".wxml", ".wxss"]) {
    const sourcePath = path.resolve(pageRoot, `${loginEntryPageName}${extension}`);
    const targetPath = path.resolve(pageRoot, `${loginFormPageName}${extension}`);
    if (!fs.existsSync(sourcePath)) {
      throw new Error(`Generated login page file was not found: ${sourcePath}`);
    }
    if (fs.existsSync(targetPath)) {
      throw new Error(`Generated login form target already exists: ${targetPath}`);
    }
    fs.renameSync(sourcePath, targetPath);
  }
  fs.writeFileSync(
    path.resolve(pageRoot, `${loginEntryPageName}.json`),
    JSON.stringify({ navigationBarTitleText: "知启云 AI", navigationStyle: "custom", usingComponents: {} }, null, 2)
  );
  fs.writeFileSync(
    path.resolve(pageRoot, `${loginEntryPageName}.wxml`),
    `<view class="login-gate">
  <image class="login-gate-logo" src="${logoPath}" mode="aspectFit" />
  <text class="login-gate-title">知启云 AI</text>
  <view wx:if="{{!failed}}" class="login-gate-status"><view class="login-gate-spinner" /><text>正在打开登录页面</text></view>
  <button wx:else class="login-gate-retry" bindtap="enterLogin">重新进入</button>
</view>
`
  );
  fs.writeFileSync(
    path.resolve(pageRoot, `${loginEntryPageName}.js`),
    `"use strict";
Page({
  data: { failed: false },
  onLoad(options) {
    this.loginOptions = options || {};
    this.enterTimer = setTimeout(() => this.enterLogin(), 80);
  },
  onUnload() {
    if (this.enterTimer) clearTimeout(this.enterTimer);
  },
  enterLogin() {
    if (this.entering) return;
    this.entering = true;
    this.setData({ failed: false });
    const query = Object.entries(this.loginOptions || {})
      .filter(([, value]) => value !== undefined && value !== null && String(value) !== "")
      .map(([key, value]) => encodeURIComponent(key) + "=" + encodeURIComponent(String(value)))
      .join("&");
    wx.redirectTo({
      url: "/pages/${loginFormPageName}" + (query ? "?" + query : ""),
      fail: () => {
        this.entering = false;
        this.setData({ failed: true });
      }
    });
  }
});
`
  );
  fs.writeFileSync(
    path.resolve(pageRoot, `${loginEntryPageName}.wxss`),
    `page { min-height: 100%; background: #f5f7fb; }
.login-gate { min-height: 100vh; box-sizing: border-box; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 24rpx; padding: 64rpx; color: #101828; background: linear-gradient(155deg, #eef3ff 0%, #f8f9fd 54%, #f2edff 100%); }
.login-gate-logo { width: 300rpx; height: 108rpx; border-radius: 24rpx; background: #fff; }
.login-gate-title { font-size: 38rpx; line-height: 1.3; font-weight: 800; }
.login-gate-status { display: flex; align-items: center; gap: 16rpx; color: #667085; font-size: 26rpx; }
.login-gate-spinner { width: 28rpx; height: 28rpx; border: 4rpx solid #d0d5dd; border-top-color: #4f46e5; border-radius: 50%; animation: login-gate-spin .8s linear infinite; }
.login-gate-retry { min-width: 240rpx; border: 0; border-radius: 999rpx; background: #4f46e5; color: #fff; font-size: 28rpx; font-weight: 700; }
.login-gate-retry::after { display: none; }
@keyframes login-gate-spin { to { transform: rotate(360deg); } }
`
  );
}

const legacyExperienceEntry = "pages/index/index";
const legacyExperienceRoot = path.resolve(outputRoot, "pages", "index");
fs.mkdirSync(legacyExperienceRoot, { recursive: true });
fs.writeFileSync(
  path.resolve(legacyExperienceRoot, "index.json"),
  JSON.stringify({ navigationBarTitleText: "知启云 AI", navigationStyle: "custom", usingComponents: {} }, null, 2)
);
fs.writeFileSync(
  path.resolve(legacyExperienceRoot, "index.wxml"),
  `<view class="legacy-entry"><image class="legacy-entry-logo" src="${logoPath}" mode="aspectFit" /><text>正在打开知启云 AI</text></view>\n`
);
fs.writeFileSync(
  path.resolve(legacyExperienceRoot, "index.js"),
  `"use strict";
Page({
  onLoad(options) {
    const query = Object.entries(options || {})
      .filter(([, value]) => value !== undefined && value !== null && String(value) !== "")
      .map(([key, value]) => encodeURIComponent(key) + "=" + encodeURIComponent(String(value)))
      .join("&");
    setTimeout(() => wx.reLaunch({ url: "/pages/user/UserHomePage" }), 50);
  }
});
`
);
fs.writeFileSync(
  path.resolve(legacyExperienceRoot, "index.wxss"),
  `page { min-height: 100%; background: #f5f7fb; }
.legacy-entry { min-height: 100vh; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 24rpx; color: #667085; font-size: 26rpx; background: linear-gradient(155deg, #eef3ff 0%, #f8f9fd 54%, #f2edff 100%); }
.legacy-entry-logo { width: 300rpx; height: 108rpx; border-radius: 24rpx; background: #fff; }
`
);
const homeWxssPath = path.resolve(pageRoot, "MiniProgramHomePage.wxss");
if (!fs.existsSync(homeWxssPath)) {
  fs.writeFileSync(homeWxssPath, "");
}

const startupPageName = "StartupPage";
const startupPagePath = path.resolve(pageRoot, startupPageName);
fs.writeFileSync(
  `${startupPagePath}.json`,
  JSON.stringify(
    {
      navigationBarTitleText: "知启云 AI",
      navigationStyle: "custom",
      usingComponents: {}
    },
    null,
    2
  )
);
fs.writeFileSync(
  `${startupPagePath}.wxml`,
  `<view class="startup-page">
  <view class="startup-brand">
    <image wx:if="{{logo}}" class="startup-logo" src="{{logo}}" mode="aspectFit" />
    <text class="startup-title">知启云 AI</text>
  </view>
  <view wx:if="{{!failed}}" class="startup-loading">
    <view class="startup-spinner" />
    <text>正在进入</text>
  </view>
  <button wx:else class="startup-retry" bindtap="enterLogin">重新进入</button>
</view>
`
);
fs.writeFileSync(
  `${startupPagePath}.js`,
  `"use strict";
Page({
  data: {
    logo: ${JSON.stringify(logoPath)},
    failed: false
  },
  onLoad() {
    this.entering = false;
    this.enterTimer = setTimeout(() => this.enterLogin(), 120);
  },
  onUnload() {
    if (this.enterTimer) clearTimeout(this.enterTimer);
  },
  enterLogin() {
    if (this.entering) return;
    this.entering = true;
    this.setData({ failed: false });
    wx.reLaunch({
      url: "/pages/user/UserHomePage",
      fail: () => {
        this.entering = false;
        this.setData({ failed: true });
      }
    });
  }
});
`
);
fs.writeFileSync(
  `${startupPagePath}.wxss`,
  `page { min-height: 100%; background: #f5f7fb; }
.startup-page { min-height: 100vh; box-sizing: border-box; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 28rpx; padding: 64rpx; color: #101828; }
.startup-brand { display: flex; flex-direction: column; align-items: center; gap: 20rpx; }
.startup-logo { width: 152rpx; height: 152rpx; border-radius: 28rpx; background: #fff; }
.startup-title { font-size: 40rpx; line-height: 1.3; font-weight: 800; }
.startup-loading { display: flex; align-items: center; gap: 16rpx; color: #667085; font-size: 26rpx; }
.startup-spinner { width: 28rpx; height: 28rpx; border: 4rpx solid #d0d5dd; border-top-color: #4f46e5; border-radius: 50%; animation: startup-spin .8s linear infinite; }
.startup-retry { min-width: 240rpx; border: 0; border-radius: 999rpx; background: #4f46e5; color: #fff; font-size: 28rpx; font-weight: 700; }
.startup-retry::after { display: none; }
@keyframes startup-spin { to { transform: rotate(360deg); } }
`
);
/* fs.writeFileSync(
  path.resolve(pageRoot, "NativeDebugLogin.json"),
  JSON.stringify(
    {
      navigationBarTitleText: "知启云 AI",
      navigationStyle: "custom",
      usingComponents: {}
    },
    null,
    2
  )
);
fs.writeFileSync(path.resolve(pageRoot, "NativeDebugLogin.wxss"), wxss); */

const appJsonPath = path.resolve(outputRoot, "app.json");
const appJson = JSON.parse(fs.readFileSync(appJsonPath, "utf8"));
const startupPage = `pages/${startupPageName}`;
const defaultHomePage = "pages/user/UserHomePage";
const loginEntryPage = `pages/${loginEntryPageName}`;
const loginFormPage = `pages/${loginFormPageName}`;
appJson.pages = appJson.pages.filter(
  (page) => page !== "pages/NativeDebugLogin" && page !== startupPage && page !== defaultHomePage && page !== loginEntryPage && page !== loginFormPage && page !== legacyExperienceEntry
);
const orderedPages = [
  defaultHomePage,
  loginEntryPage,
  loginFormPage,
  legacyExperienceEntry,
  startupPage,
  ...appJson.pages
];
const mainUserPages = new Set([
  "pages/user/UserHomePage",
  "pages/user/UserCreationPage",
  "pages/user/UserAssetsPage",
  "pages/user/UserMinePage"
]);
const userCreationPageNames = new Set([
  "UserImageCreationPage",
  "UserVideoCreationPage",
  "UserPptCreationPage",
  "UserPptEditorPage",
  "UserInfographicCreationPage",
  "UserReviewCreationPage",
  "UserReviewConversationPage",
  "UserAgentCreationPage",
  "UserKnowledgeAgentDetailPage"
]);
const userSecondaryPageNames = orderedPages
  .filter((page) => page.startsWith("pages/user/") && !mainUserPages.has(page))
  .map((page) => page.slice("pages/user/".length));
const relocatedUserSubPackages = [
  {
    root: "pages/user-creation",
    pages: userSecondaryPageNames.filter((page) => userCreationPageNames.has(page))
  },
  {
    root: "pages/user-account",
    pages: userSecondaryPageNames.filter((page) => !userCreationPageNames.has(page))
  }
].filter((item) => item.pages.length > 0);

function assertGeneratedPath(filePath) {
  const resolved = path.resolve(filePath);
  const relative = path.relative(outputRoot, resolved);
  if (relative.startsWith("..") || path.isAbsolute(relative)) {
    throw new Error(`Refusing to move generated page outside mp-weixin output: ${resolved}`);
  }
  return resolved;
}

function relocateGeneratedPage(sourceRoot, targetRoot, pageName) {
  const sourceDir = assertGeneratedPath(path.resolve(outputRoot, sourceRoot));
  const targetDir = assertGeneratedPath(path.resolve(outputRoot, targetRoot));
  fs.mkdirSync(targetDir, { recursive: true });
  for (const extension of [".js", ".json", ".wxml", ".wxss"]) {
    const sourcePath = assertGeneratedPath(path.resolve(sourceDir, `${pageName}${extension}`));
    const targetPath = assertGeneratedPath(path.resolve(targetDir, `${pageName}${extension}`));
    if (!fs.existsSync(sourcePath)) continue;
    if (fs.existsSync(targetPath)) {
      throw new Error(`Generated subpackage target already exists: ${targetPath}`);
    }
    fs.renameSync(sourcePath, targetPath);
  }
}

function relocateGeneratedModule(sourceRelativePath, targetRelativePath, transform) {
  const sourcePath = assertGeneratedPath(path.resolve(outputRoot, sourceRelativePath));
  const targetPath = assertGeneratedPath(path.resolve(outputRoot, targetRelativePath));
  if (!fs.existsSync(sourcePath)) {
    throw new Error(`Generated module to relocate was not found: ${sourceRelativePath}`);
  }
  if (fs.existsSync(targetPath)) {
    throw new Error(`Generated module target already exists: ${targetRelativePath}`);
  }
  fs.mkdirSync(path.dirname(targetPath), { recursive: true });
  const original = fs.readFileSync(sourcePath, "utf8");
  const updated = typeof transform === "function" ? transform(original) : original;
  fs.writeFileSync(targetPath, updated);
  fs.rmSync(sourcePath);
}

const relocatedUserRoutes = new Map();
for (const subPackage of relocatedUserSubPackages) {
  for (const pageName of subPackage.pages) {
    const oldRoute = `/pages/user/${pageName}`;
    const newRoute = `/${subPackage.root}/${pageName}`;
    relocateGeneratedPage("pages/user", subPackage.root, pageName);
    relocatedUserRoutes.set(oldRoute, newRoute);
  }
}

if (relocatedUserRoutes.has("/pages/user/UserWalletPage")) {
  relocateGeneratedModule(
    "features/wallet/personalPointsWallet.js",
    "pages/user-account/features/wallet/personalPointsWallet.js"
  );
  relocateGeneratedModule(
    "composables/usePersonalPointsWallet.js",
    "pages/user-account/composables/usePersonalPointsWallet.js",
    (source) => source
      .split('require("../common/vendor.js")').join('require("../../../common/vendor.js")')
      .split('require("../api/client.js")').join('require("../../../api/client.js")')
  );
  const walletPagePath = assertGeneratedPath(path.resolve(outputRoot, "pages/user-account/UserWalletPage.js"));
  const walletPageSource = fs.readFileSync(walletPagePath, "utf8")
    .split('require("../../composables/usePersonalPointsWallet.js")').join('require("./composables/usePersonalPointsWallet.js")')
    .split('require("../../features/wallet/personalPointsWallet.js")').join('require("./features/wallet/personalPointsWallet.js")');
  fs.writeFileSync(walletPagePath, walletPageSource);
}

function rewriteGeneratedUserRoutes(directory) {
  for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
    const filePath = assertGeneratedPath(path.resolve(directory, entry.name));
    if (entry.isDirectory()) {
      rewriteGeneratedUserRoutes(filePath);
      continue;
    }
    if (!entry.isFile() || path.extname(entry.name) !== ".js") continue;
    const original = fs.readFileSync(filePath, "utf8");
    let updated = original;
    for (const [oldRoute, newRoute] of relocatedUserRoutes) {
      updated = updated.split(oldRoute).join(newRoute);
    }
    if (updated !== original) fs.writeFileSync(filePath, updated);
  }
}

const splitPackageRoots = [
  "pages/enterprise",
  "pages/promotion",
  "pages/agent",
  "pages/operation"
];
const generatedSubPackages = splitPackageRoots
  .map((root) => ({
    root,
    pages: orderedPages
      .filter((page) => page.startsWith(`${root}/`))
      .map((page) => page.slice(root.length + 1))
  }))
  .filter((item) => item.pages.length > 0);
generatedSubPackages.push(...relocatedUserSubPackages);
const generatedSubPackageRoots = new Set(generatedSubPackages.map((item) => item.root));
appJson.pages = orderedPages.filter(
  (page) =>
    !splitPackageRoots.some((root) => page.startsWith(`${root}/`)) &&
    !relocatedUserRoutes.has(`/${page}`)
);
appJson.subPackages = [
  ...(Array.isArray(appJson.subPackages)
    ? appJson.subPackages.filter((item) => !generatedSubPackageRoots.has(item?.root))
    : []),
  ...generatedSubPackages
];
fs.writeFileSync(appJsonPath, JSON.stringify(appJson, null, 2));

function preserveGeneratedComponents(configPath) {
  if (!fs.existsSync(configPath)) return;
  const config = JSON.parse(fs.readFileSync(configPath, "utf8"));
  const existingIgnore = Array.isArray(config.packOptions?.ignore) ? config.packOptions.ignore : [];
  const retainedFallbackJpegs = new Set([
    "brand-gradient.jpg",
    "hero-orb.jpg",
    "capability-ai-design.jpg",
    "capability-ai-video.jpg",
    "capability-ppt.jpg",
    "default-capability.jpg",
    "default-project.jpg",
    "default-ai-avatar.jpg",
    "default-inspiration.jpg",
    "inspiration-video.jpg",
    "inspiration-ppt.jpg",
    "default-studio-hero.jpg",
    "default-template.jpg",
    "default-cover.jpg",
    "default-video-cover.jpg",
    "default-ppt-cover.jpg",
    "default-long-image.jpg",
    "default-avatar.jpg"
  ]);
  const generatedFallbackDirectory = path.resolve(outputRoot, "static", "fallbacks");
  const unusedFallbackIgnores = fs.existsSync(generatedFallbackDirectory)
    ? fs.readdirSync(generatedFallbackDirectory)
      .filter((name) => name.endsWith(".jpg") && !retainedFallbackJpegs.has(name))
      .map((name) => ({ type: "file", value: `static/fallbacks/${name}` }))
    : [];
  const optimizedPackageIgnores = [
    { type: "suffix", value: ".webp" },
    { type: "folder", value: "static/app-icons" },
    { type: "file", value: "static/fallbacks/profile-member-background.png" },
    { type: "file", value: "static/fallbacks/profile-header-background.png" },
    ...(logoFile ? [{ type: "file", value: `assets/${logoFile}` }] : []),
    ...unusedFallbackIgnores
  ];
  const optimizedIgnoreKeys = new Set(optimizedPackageIgnores.map((item) => `${item.type}:${item.value}`));
  config.packOptions = Object.assign({}, config.packOptions, {
    ignore: [
      ...existingIgnore.filter((item) =>
        !optimizedIgnoreKeys.has(`${item?.type}:${item?.value}`) &&
        !(item?.type === "file" && /^static\/fallbacks\/.+\.jpg$/.test(String(item?.value || "")))
      ),
      ...optimizedPackageIgnores
    ]
  });
  config.setting = Object.assign({}, config.setting, {
    minified: true,
    minifyWXML: true,
    minifyWXSS: true,
    ignoreDevUnusedFiles: true,
    ignoreUploadUnusedFiles: true,
    useLanDebug: true
  });
  if (config.condition && config.condition.miniprogram) {
    const miniprogram = config.condition.miniprogram;
    miniprogram.current = -1;
    miniprogram.list = Array.isArray(miniprogram.list)
      ? miniprogram.list.filter((item) => item && item.pathName !== "pages/user/UserMinePage")
      : [];
  }
  fs.writeFileSync(configPath, JSON.stringify(config, null, 2));
}

preserveGeneratedComponents(path.resolve(outputRoot, "project.config.json"));
preserveGeneratedComponents(path.resolve(outputRoot, "project.private.config.json"));

const homeComponentWxmlPath = path.resolve(outputRoot, "components", "v531", "V531HomePage.wxml");
const homeComponentJsPath = path.resolve(outputRoot, "components", "v531", "V531HomePage.js");
if (!fs.existsSync(homeComponentWxmlPath) || !fs.existsSync(homeComponentJsPath)) {
  throw new Error("V531HomePage component output not found after mp-weixin build");
}
let homeComponentWxml = fs.readFileSync(homeComponentWxmlPath, "utf8");
homeComponentWxml = homeComponentWxml.replace(
  /(<input class="hero-text-input[^"]*"[^>]*?)bindconfirm="\{\{[^}]+\}\}"/,
  `$1bindconfirm="nativeHomePromptSubmit"`
);
homeComponentWxml = homeComponentWxml.replace(
  /(<input class="hero-text-input[^"]*"[^>]*?)bindinput="\{\{[^}]+\}\}"/,
  `$1bindinput="nativeHomePromptInput"`
);
homeComponentWxml = homeComponentWxml.replace(
  /<button class="hero-input-action submit[^"]*"[^>]*>/,
  (tag) => tag
    .replace(/\s+(?:bind|catch)touchend="\{\{[^}]+\}\}"/g, "")
    .replace(/(?:bind|catch)tap="\{\{[^}]+\}\}"/, 'bindtap="nativeHomePromptSubmit"')
);
if (!homeComponentWxml.includes('bindinput="nativeHomePromptInput"') || !homeComponentWxml.includes('bindtap="nativeHomePromptSubmit"')) {
  throw new Error("V531HomePage native prompt bindings were not patched");
}
fs.writeFileSync(homeComponentWxmlPath, homeComponentWxml);

let homeComponentJs = fs.readFileSync(homeComponentJsPath, "utf8");
const homeComponentPattern = /wx\.createComponent\((\w+)\);\s*$/;
if (!homeComponentPattern.test(homeComponentJs)) {
  throw new Error("V531HomePage component registration not found");
}
const homePromptNativeBridge = String.raw`$1.methods=Object.assign({},$1.methods,{nativeHomePromptInput(event){this.__xianzhiHomePrompt=event&&event.detail?String(event.detail.value||""):""},nativeHomePromptSubmit(){const value=String(this.__xianzhiHomePrompt||"").trim();if(!value){wx.showToast({title:"\u8bf7\u8f93\u5165\u4f60\u7684\u9700\u6c42",icon:"none"});return}wx.setStorageSync("v531-creation-prompt",value);const mode=/\u89c6\u9891|\u77ed\u7247|\u53e3\u64ad|\u5206\u955c/.test(value)?"video":/ppt|\u6f14\u793a|\u6c47\u62a5|\u8def\u6f14|\u65b9\u6848/i.test(value)?"ppt":/\u667a\u80fd\u4f53|agent|\u5ba2\u670d|\u9500\u552e\u52a9\u624b|\u77e5\u8bc6\u5e93/i.test(value)?"agent":/\u4fe1\u606f\u56fe|\u6d41\u7a0b\u56fe|\u6570\u636e\u56fe|\u53ef\u89c6\u5316/.test(value)?"infographic":"image";const routes={image:"/pages/user/UserImageCreationPage",video:"/pages/user/UserVideoCreationPage",ppt:"/pages/user/UserPptCreationPage",infographic:"/pages/user/UserInfographicCreationPage",review:"/pages/user/UserReviewCreationPage",agent:"/pages/user/UserAgentCreationPage"};const url=routes[mode]||routes.image;wx.showToast({title:"\u6b63\u5728\u8fdb\u5165\u521b\u4f5c",icon:"none",duration:700});wx.navigateTo({url,fail(){wx.redirectTo({url,fail(){wx.reLaunch({url,fail(){wx.showToast({title:"\u9875\u9762\u6253\u5f00\u5931\u8d25\uff0c\u8bf7\u91cd\u8bd5",icon:"none"})}})}})}})}});$1.wxsCallMethods=Array.from(new Set([...(Array.isArray($1.wxsCallMethods)?$1.wxsCallMethods:[]),"nativeHomePromptInput","nativeHomePromptSubmit"]));wx.createComponent($1);`;
if (!homeComponentJs.includes("nativeHomePromptSubmit(){")) {
  homeComponentJs = homeComponentJs.replace(homeComponentPattern, homePromptNativeBridge);
}
fs.writeFileSync(homeComponentJsPath, homeComponentJs);

const workbenchWxmlPath = path.resolve(outputRoot, "components", "MiniProgramRoleWorkbench.wxml");
if (!fs.existsSync(workbenchWxmlPath)) {
  throw new Error("MiniProgramRoleWorkbench.wxml not found after mp-weixin build");
}
let workbenchWxml = fs.readFileSync(workbenchWxmlPath, "utf8");
let nativeGenerateBindingCount = (workbenchWxml.match(/bindtap="nativeGenerate"/g) || []).length;
workbenchWxml = workbenchWxml.replace(
  /(<view class="\{\{\[[^\]]*'v31-(?:ppt-submit|generate-button)'[^\]]*\]\}\}"[^>]*?)(?:bind|catch)(?:tap|touchend)="\{\{[^}]+\}\}"/g,
  (match, prefix) => {
    nativeGenerateBindingCount += 1;
    return `${prefix}bindtap="nativeGenerate"`;
  }
);
if (nativeGenerateBindingCount !== 2) {
  throw new Error(`Expected 2 native generate bindings, patched ${nativeGenerateBindingCount}`);
}

let nativeReferenceBindingCount = (workbenchWxml.match(/bindtap="nativeChooseReferenceImages"/g) || []).length;
workbenchWxml = workbenchWxml.replace(
  /(<button[^>]*class="v31-reference-(?:add|empty)[^"]*"[^>]*?)bindtap="\{\{[^}]+\}\}"/g,
  (match, prefix) => {
    nativeReferenceBindingCount += 1;
    return `${prefix}bindtap="nativeChooseReferenceImages"`;
  }
);
if (nativeReferenceBindingCount !== 2) {
  throw new Error(`Expected 2 native reference image bindings, patched ${nativeReferenceBindingCount}`);
}

let nativeBackBindingCount = (workbenchWxml.match(/bindtap="nativeBackToCreation"/g) || []).length;
workbenchWxml = workbenchWxml.replace(
  /(<button class="v31-back-button[^>]*?)bindtap="\{\{[^}]+\}\}"/g,
  (match, prefix) => {
    nativeBackBindingCount += 1;
    return `${prefix}bindtap="nativeBackToCreation"`;
  }
);
if (nativeBackBindingCount !== 1) {
  throw new Error(`Expected 1 workbench creation back binding, patched ${nativeBackBindingCount}`);
}
fs.writeFileSync(workbenchWxmlPath, workbenchWxml);

const workbenchJsPath = path.resolve(outputRoot, "components", "MiniProgramRoleWorkbench.js");
let workbenchJs = fs.readFileSync(workbenchJsPath, "utf8");
const createComponentPattern = /wx\.createComponent\((\w+)\);\s*$/;
if (!createComponentPattern.test(workbenchJs)) {
  throw new Error("MiniProgramRoleWorkbench component registration not found");
}
if (!workbenchJs.includes("nativeGenerate(){const invoke=")) {
  workbenchJs = workbenchJs.replace(
    createComponentPattern,
    `$1.methods=Object.assign({},$1.methods,{nativeGenerate(){const invoke=attempt=>{const handler=globalThis.__xianzhiMiniProgramGenerate;if(typeof handler==="function"){handler();return}if(attempt<8){setTimeout(()=>invoke(attempt+1),50);return}wx.showToast({title:"页面尚未准备好，请返回后重试",icon:"none"})};invoke(0)},nativeBackToCreation(event){const dataset=event&&event.currentTarget&&event.currentTarget.dataset?event.currentTarget.dataset:{};const fallback=String(dataset.returnFallback||"/pages/user/UserCreationPage");const returnToFallback=()=>{const tabPages=new Set(["/pages/user/UserHomePage","/pages/user/UserCreationPage","/pages/user/UserAssetsPage","/pages/user/UserMinePage"]);if(tabPages.has(fallback)){wx.switchTab({url:fallback,fail(){wx.reLaunch({url:fallback})}});return}wx.redirectTo({url:fallback,fail(){wx.reLaunch({url:fallback})}})};const pages=getCurrentPages();if(pages.length>1){wx.navigateBack({delta:1,fail:returnToFallback});return}returnToFallback()}});$1.wxsCallMethods=Array.from(new Set([...(Array.isArray($1.wxsCallMethods)?$1.wxsCallMethods:[]),"nativeGenerate","nativeBackToCreation"]));wx.createComponent($1);`
  );
}
if (!workbenchJs.includes("nativeChooseReferenceImages(event){const append=")) {
  workbenchJs = workbenchJs.replace(
    createComponentPattern,
    `$1.methods=Object.assign({},$1.methods,{nativeChooseReferenceImages(event){const append=globalThis.__xianzhiMiniProgramAppendReferences;const setSelecting=globalThis.__xianzhiMiniProgramSetReferenceSelecting;if(typeof append!=="function"){wx.showToast({title:"\u56fe\u7247\u9009\u62e9\u5165\u53e3\u521d\u59cb\u5316\u4e2d\uff0c\u8bf7\u7a0d\u540e\u91cd\u8bd5",icon:"none"});return}const dataset=event&&event.currentTarget&&event.currentTarget.dataset?event.currentTarget.dataset:{};const count=Math.max(1,Math.min(3,Number(dataset.referenceRemaining)||1));const success=result=>{const files=Array.isArray(result&&result.tempFiles)?result.tempFiles:[];const paths=files.map(file=>typeof file==="string"?file:String(file&&file.tempFilePath||file&&file.path||"")).filter(Boolean);if(paths.length){append(paths);wx.showToast({title:"\u5df2\u6dfb\u52a0 "+paths.length+" \u5f20\u53c2\u8003\u56fe",icon:"success"})}};const fail=error=>{const message=String(error&&error.errMsg||"");if(!/cancel/i.test(message))wx.showToast({title:"\u53c2\u8003\u56fe\u9009\u62e9\u5931\u8d25",icon:"none"})};const complete=()=>{if(typeof setSelecting==="function")setSelecting(false)};if(typeof setSelecting==="function")setSelecting(true);if(typeof wx.chooseMedia==="function"){wx.chooseMedia({count,mediaType:["image"],sourceType:["album","camera"],sizeType:["compressed"],success,fail,complete});return}wx.chooseImage({count,sourceType:["album","camera"],sizeType:["compressed"],success:result=>success({tempFiles:(result.tempFilePaths||[]).map(path=>({tempFilePath:path}))}),fail,complete})}});$1.wxsCallMethods=Array.from(new Set([...(Array.isArray($1.wxsCallMethods)?$1.wxsCallMethods:[]),"nativeChooseReferenceImages"]));wx.createComponent($1);`
  );
}
fs.writeFileSync(workbenchJsPath, workbenchJs);

const studioWxmlPath = path.resolve(outputRoot, "components", "v531", "V531StudioPage.wxml");
const studioJsPath = path.resolve(outputRoot, "components", "v531", "V531StudioPage.js");
if (!fs.existsSync(studioWxmlPath) || !fs.existsSync(studioJsPath)) {
  throw new Error("V531StudioPage build output not found");
}

let studioWxml = fs.readFileSync(studioWxmlPath, "utf8");
const studioBindings = [
  {
    label: "reference image",
    pattern: /(<button(?=[^>]*class="tool-button)(?=[^>]*data-studio-action="reference")[^>]*?)bindtap="\{\{[^}]+\}\}"/,
    replacement: '$1bindtap="nativeStudioChooseReference"'
  },
  {
    label: "file upload",
    pattern: /(<button(?=[^>]*class="tool-button)(?=[^>]*data-studio-action="file")[^>]*?)bindtap="\{\{[^}]+\}\}"/,
    replacement: '$1bindtap="nativeStudioChooseFile"'
  },
  {
    label: "generate",
    pattern: /(<button class="generate-button[^>]*?)bindtap="\{\{[^}]+\}\}"/,
    replacement: '$1bindtap="nativeStudioGenerate"'
  },
  {
    label: "capability",
    pattern: /(<button wx:for="\{\{[^}]+\}\}"[^>]*class="capability-button[^>]*?)bindtap="\{\{item\.[^}]+\}\}"/,
    replacement: '$1bindtap="nativeStudioOpenMode"'
  },
  {
    label: "scene center",
    pattern: /(<button(?=[^>]*class="section-more section-more-button)[^>]*?)bindtap="\{\{[^}]+\}\}"/,
    replacement: '$1bindtap="nativeStudioOpenSceneCenter"'
  },
  {
    label: "scene",
    pattern: /(<button wx:for="\{\{[^}]+\}\}"[^>]*class="\{\{\[[^>]*'scene-button'[^>]*?)bindtap="\{\{item\.[^}]+\}\}"/,
    replacement: '$1bindtap="nativeStudioOpenScene"'
  }
];
for (const binding of studioBindings) {
  const nativeHandler = binding.replacement.match(/bindtap="([^"]+)"/)?.[1];
  if (nativeHandler && studioWxml.includes(`bindtap="${nativeHandler}"`)) continue;
  if (!binding.pattern.test(studioWxml)) {
    throw new Error(`V531StudioPage ${binding.label} binding not found after mp-weixin build`);
  }
  studioWxml = studioWxml.replace(binding.pattern, binding.replacement);
}
fs.writeFileSync(studioWxmlPath, studioWxml);

let studioJs = fs.readFileSync(studioJsPath, "utf8");
if (!createComponentPattern.test(studioJs)) {
  throw new Error("V531StudioPage component registration not found");
}
if (!studioJs.includes("nativeStudioGenerate(event){")) {
  studioJs = studioJs.replace(
    createComponentPattern,
    `$1.methods=Object.assign({},$1.methods,{nativeStudioGenerate(event){const dataset=event&&event.currentTarget&&event.currentTarget.dataset?event.currentTarget.dataset:{};const mode=String(dataset.mode||"image");const prompt=String(dataset.prompt||"").trim();if(!prompt){wx.showToast({title:"请先输入你的创作需求",icon:"none"});return}const previous=wx.getStorageSync("v532-studio-draft")||{};wx.setStorageSync("v531-creation-prompt",prompt);wx.setStorageSync("v532-studio-draft",Object.assign({},previous,{mode,prompt}));const pages={image:"/pages/user/UserImageCreationPage",video:"/pages/user/UserVideoCreationPage",ppt:"/pages/user/UserPptCreationPage",infographic:"/pages/user/UserInfographicCreationPage",review:"/pages/user/UserReviewCreationPage",agent:"/pages/user/UserAgentCreationPage"};wx.navigateTo({url:pages[mode]||pages.image})},nativeStudioOpenMode(event){const dataset=event&&event.currentTarget&&event.currentTarget.dataset?event.currentTarget.dataset:{};const mode=String(dataset.mode||"image");const prompt=String(dataset.prompt||"").trim();if(prompt){const previous=wx.getStorageSync("v532-studio-draft")||{};wx.setStorageSync("v531-creation-prompt",prompt);wx.setStorageSync("v532-studio-draft",Object.assign({},previous,{mode,prompt}))}const pages={image:"/pages/user/UserImageCreationPage",video:"/pages/user/UserVideoCreationPage",ppt:"/pages/user/UserPptCreationPage",infographic:"/pages/user/UserInfographicCreationPage",review:"/pages/user/UserReviewCreationPage",agent:"/pages/user/UserAgentCreationPage"};wx.navigateTo({url:pages[mode]||pages.image})},nativeStudioOpenScene(event){const dataset=event&&event.currentTarget&&event.currentTarget.dataset?event.currentTarget.dataset:{};const mode=String(dataset.mode||"image");const prompt=String(dataset.prompt||"").trim();if(prompt){const previous=wx.getStorageSync("v532-studio-draft")||{};wx.setStorageSync("v531-creation-prompt",prompt);wx.setStorageSync("v532-studio-draft",Object.assign({},previous,{mode,prompt}))}const pages={image:"/pages/user/UserImageCreationPage",video:"/pages/user/UserVideoCreationPage",ppt:"/pages/user/UserPptCreationPage",infographic:"/pages/user/UserInfographicCreationPage",review:"/pages/user/UserReviewCreationPage",agent:"/pages/user/UserAgentCreationPage"};wx.navigateTo({url:pages[mode]||pages.image})},nativeStudioOpenSceneCenter(){const handler=globalThis.__xianzhiV531StudioOpenSceneCenter;if(typeof handler==="function"){handler();return}wx.showToast({title:"场景中心初始化中，请稍后重试",icon:"none"})}});$1.wxsCallMethods=Array.from(new Set([...(Array.isArray($1.wxsCallMethods)?$1.wxsCallMethods:[]),"nativeStudioGenerate","nativeStudioOpenMode","nativeStudioOpenScene","nativeStudioOpenSceneCenter"]));wx.createComponent($1);`
  );
}
if (!studioJs.includes("nativeStudioChooseReference(){")) {
  studioJs = studioJs.replace(
    createComponentPattern,
    `$1.methods=Object.assign({},$1.methods,{nativeStudioChooseReference(){const handler=globalThis.__xianzhiV531StudioChooseReference;if(typeof handler==="function"){handler();return}wx.showToast({title:"图片选择入口初始化中，请稍后重试",icon:"none"})},nativeStudioChooseFile(){const handler=globalThis.__xianzhiV531StudioChooseFile;if(typeof handler==="function"){handler();return}wx.showToast({title:"文件选择入口初始化中，请稍后重试",icon:"none"})}});$1.wxsCallMethods=Array.from(new Set([...(Array.isArray($1.wxsCallMethods)?$1.wxsCallMethods:[]),"nativeStudioChooseReference","nativeStudioChooseFile"]));wx.createComponent($1);`
  );
}
fs.writeFileSync(studioJsPath, studioJs);

function replaceNativeAssetBindings(content, pattern, replacement, expectedCount, label) {
  const nativeHandler = typeof replacement === "string" ? replacement.match(/(?:bind|catch)tap="([^"]+)"/)?.[1] : "";
  let count = nativeHandler ? (content.match(new RegExp(`(?:bind|catch)tap="${nativeHandler}"`, "g")) || []).length : 0;
  const output = content.replace(pattern, (...args) => {
    count += 1;
    if (typeof replacement === "function") return replacement(...args);
    return replacement.replace(/\$(\d+)/g, (token, index) => String(args[Number(index)] ?? token));
  });
  if (count !== expectedCount) {
    throw new Error(`${label}: expected ${expectedCount} binding(s), patched ${count}`);
  }
  return output;
}

function injectNativeAssetMethods(jsPath, label, methodsSource, methodNames) {
  let content = fs.readFileSync(jsPath, "utf8");
  if (methodNames.every((name) => content.includes(`${name}(`) && content.includes(JSON.stringify(name)))) return;
  const match = content.match(createComponentPattern);
  if (!match) throw new Error(`${label} component registration not found`);
  const componentName = match[1];
  const registration = `${componentName}.methods=Object.assign({},${componentName}.methods,{${methodsSource}});${componentName}.wxsCallMethods=Array.from(new Set([...(Array.isArray(${componentName}.wxsCallMethods)?${componentName}.wxsCallMethods:[]),${methodNames.map((name) => JSON.stringify(name)).join(",")} ]));wx.createComponent(${componentName});`;
  content = content.replace(createComponentPattern, registration);
  try {
    new Function(content);
  } catch (error) {
    throw new Error(`${label} native method injection produced invalid JavaScript: ${error.message}`);
  }
  fs.writeFileSync(jsPath, content);
}

const assetCenterWxmlPath = path.resolve(outputRoot, "components", "assets", "AssetCenterPage.wxml");
const assetCenterJsPath = path.resolve(outputRoot, "components", "assets", "AssetCenterPage.js");
if (!fs.existsSync(assetCenterWxmlPath) || !fs.existsSync(assetCenterJsPath)) {
  throw new Error("AssetCenterPage build output not found");
}
let assetCenterWxml = fs.readFileSync(assetCenterWxmlPath, "utf8");
let assetHeaderIconIndex = 0;
assetCenterWxml = replaceNativeAssetBindings(
  assetCenterWxml,
  /(<button class="icon-action[^"]*"[^>]*?)bindtap="\{\{[^}]+\}\}"/g,
  (match, prefix) => `${prefix}bindtap="${["nativeAssetOpenManage", "nativeAssetOpenMore"][assetHeaderIconIndex++]}"`,
  2,
  "AssetCenterPage header actions"
);
let assetHeaderButtonIndex = 0;
assetCenterWxml = replaceNativeAssetBindings(
  assetCenterWxml,
  /(<button class="toolbar-action data-v-[^"]+"[^>]*?)bindtap="\{\{[^}]+\}\}"/g,
  (match, prefix) => `${prefix}bindtap="${["nativeAssetOpenFilter", "nativeAssetOpenSort"][assetHeaderButtonIndex++]}"`,
  2,
  "AssetCenterPage filter and sort"
);
assetCenterWxml = replaceNativeAssetBindings(
  assetCenterWxml,
  /(<button[^>]*class="search-submit[^"]*"[^>]*?)bindtap="\{\{[^}]+\}\}"/g,
  "$1bindtap=\"nativeAssetSubmitSearch\"",
  1,
  "AssetCenterPage search submit"
);
assetCenterWxml = replaceNativeAssetBindings(
  assetCenterWxml,
  /(<button[^>]*class="search-clear[^"]*"[^>]*?)bindtap="\{\{[^}]+\}\}"/g,
  "$1bindtap=\"nativeAssetClearSearch\"",
  1,
  "AssetCenterPage search clear"
);
assetCenterWxml = replaceNativeAssetBindings(
  assetCenterWxml,
  /(<input[^>]*class="asset-search-input[^"]*"[^>]*?)bindinput="\{\{[^}]+\}\}"/g,
  "$1bindinput=\"nativeAssetSearchInput\"",
  1,
  "AssetCenterPage search input"
);
assetCenterWxml = replaceNativeAssetBindings(
  assetCenterWxml,
  /(<view class="section-head asset-section[^"]*"><text[^>]*>[^<]*<\/text><button[^>]*?)bindtap="\{\{[^}]+\}\}"/g,
  "$1bindtap=\"nativeAssetOpenAll\"",
  1,
  "AssetCenterPage asset view all"
);
assetCenterWxml = replaceNativeAssetBindings(
  assetCenterWxml,
  /(<view class="section-head task-section[^"]*"><text[^>]*>[^<]*<\/text><button[^>]*?)bindtap="\{\{[^}]+\}\}"/g,
  "$1bindtap=\"nativeAssetOpenTasks\"",
  1,
  "AssetCenterPage task view all"
);
fs.writeFileSync(assetCenterWxmlPath, assetCenterWxml);

const assetCenterNativeMethods = String.raw`nativeAssetToggleSearch(){const bridge=globalThis.__xianzhiAssetNativeBridge;if(bridge&&typeof bridge.toggleSearch==="function"){bridge.toggleSearch();return}wx.showToast({title:"搜索入口初始化中，请稍后重试",icon:"none"})},nativeAssetSearchInput(event){const bridge=globalThis.__xianzhiAssetNativeBridge;const value=event&&event.detail?String(event.detail.value||""):"";if(bridge&&typeof bridge.updateSearch==="function"){bridge.updateSearch(value);return}wx.showToast({title:"搜索入口初始化中，请稍后重试",icon:"none"})},nativeAssetSubmitSearch(){const bridge=globalThis.__xianzhiAssetNativeBridge;if(bridge&&typeof bridge.submitSearch==="function"){bridge.submitSearch();return}wx.showToast({title:"搜索入口初始化中，请稍后重试",icon:"none"})},nativeAssetClearSearch(){const bridge=globalThis.__xianzhiAssetNativeBridge;if(bridge&&typeof bridge.clearSearch==="function"){bridge.clearSearch();return}wx.showToast({title:"搜索入口初始化中，请稍后重试",icon:"none"})},nativeAssetOpenFilter(){const bridge=globalThis.__xianzhiAssetNativeBridge;if(bridge&&typeof bridge.openFilter==="function"){bridge.openFilter();return}wx.showToast({title:"筛选入口初始化中，请稍后重试",icon:"none"})},nativeAssetOpenSort(){const bridge=globalThis.__xianzhiAssetNativeBridge;if(bridge&&typeof bridge.openSort==="function"){bridge.openSort();return}wx.showToast({title:"排序入口初始化中，请稍后重试",icon:"none"})},nativeAssetOpenManage(){const bridge=globalThis.__xianzhiAssetNativeBridge;if(bridge&&typeof bridge.openManage==="function"){bridge.openManage();return}wx.navigateTo({url:"/pages/user/UserAssetsListPage?manage=1"})},nativeAssetOpenMore(){const bridge=globalThis.__xianzhiAssetNativeBridge;wx.showActionSheet({itemList:["批量管理","查看全部作品","查看全部任务"],success(result){if(result.tapIndex===0){if(bridge&&typeof bridge.openManage==="function")bridge.openManage();else wx.navigateTo({url:"/pages/user/UserAssetsListPage?manage=1"});return}if(result.tapIndex===1){if(bridge&&typeof bridge.openAllAssets==="function")bridge.openAllAssets();else wx.navigateTo({url:"/pages/user/UserAssetsListPage"});return}if(bridge&&typeof bridge.openAllTasks==="function")bridge.openAllTasks();else wx.navigateTo({url:"/pages/user/UserTasksPage"})}})},nativeAssetOpenAll(){const bridge=globalThis.__xianzhiAssetNativeBridge;if(bridge&&typeof bridge.openAllAssets==="function"){bridge.openAllAssets();return}wx.navigateTo({url:"/pages/user/UserAssetsListPage"})},nativeAssetOpenTasks(){const bridge=globalThis.__xianzhiAssetNativeBridge;if(bridge&&typeof bridge.openAllTasks==="function"){bridge.openAllTasks();return}wx.navigateTo({url:"/pages/user/UserTasksPage"})}`;
injectNativeAssetMethods(assetCenterJsPath, "AssetCenterPage", assetCenterNativeMethods, ["nativeAssetToggleSearch", "nativeAssetSearchInput", "nativeAssetSubmitSearch", "nativeAssetClearSearch", "nativeAssetOpenFilter", "nativeAssetOpenSort", "nativeAssetOpenManage", "nativeAssetOpenMore", "nativeAssetOpenAll", "nativeAssetOpenTasks"]);

const assetLibraryWxmlPath = path.resolve(outputRoot, "components", "assets", "AssetLibraryPage.wxml");
const assetLibraryJsPath = path.resolve(outputRoot, "components", "assets", "AssetLibraryPage.js");
if (!fs.existsSync(assetLibraryWxmlPath) || !fs.existsSync(assetLibraryJsPath)) {
  throw new Error("AssetLibraryPage build output not found");
}
let assetLibraryWxml = fs.readFileSync(assetLibraryWxmlPath, "utf8");
assetLibraryWxml = replaceNativeAssetBindings(
  assetLibraryWxml,
  /(<button[^>]*class="search-button[^"]*"[^>]*?)bindtap="\{\{[^}]+\}\}"/g,
  "$1bindtap=\"nativeAssetSubmitSearch\"",
  1,
  "AssetLibraryPage search submit"
);
assetLibraryWxml = replaceNativeAssetBindings(
  assetLibraryWxml,
  /(<button[^>]*class="search-clear[^"]*"[^>]*?)bindtap="\{\{[^}]+\}\}"/g,
  "$1bindtap=\"nativeAssetClearSearch\"",
  1,
  "AssetLibraryPage search clear"
);
assetLibraryWxml = replaceNativeAssetBindings(
  assetLibraryWxml,
  /(<input[^>]*class="asset-search-input[^"]*"[^>]*?)bindinput="\{\{[^}]+\}\}"/g,
  "$1bindinput=\"nativeAssetSearchInput\"",
  1,
  "AssetLibraryPage search input"
);
fs.writeFileSync(assetLibraryWxmlPath, assetLibraryWxml);
const assetLibraryNativeMethods = String.raw`nativeAssetSearchInput(event){const bridge=globalThis.__xianzhiAssetNativeBridge;const value=event&&event.detail?String(event.detail.value||""):"";if(bridge&&typeof bridge.updateSearch==="function"){bridge.updateSearch(value);return}wx.showToast({title:"搜索入口初始化中，请稍后重试",icon:"none"})},nativeAssetSubmitSearch(){const bridge=globalThis.__xianzhiAssetNativeBridge;if(bridge&&typeof bridge.submitSearch==="function"){bridge.submitSearch();return}wx.showToast({title:"搜索入口初始化中，请稍后重试",icon:"none"})},nativeAssetClearSearch(){const bridge=globalThis.__xianzhiAssetNativeBridge;if(bridge&&typeof bridge.clearSearch==="function"){bridge.clearSearch();return}wx.showToast({title:"搜索入口初始化中，请稍后重试",icon:"none"})}`;
injectNativeAssetMethods(assetLibraryJsPath, "AssetLibraryPage", assetLibraryNativeMethods, ["nativeAssetSearchInput", "nativeAssetSubmitSearch", "nativeAssetClearSearch"]);

const assetTypeTabsWxmlPath = path.resolve(outputRoot, "components", "assets", "AssetTypeTabs.wxml");
const assetTypeTabsJsPath = path.resolve(outputRoot, "components", "assets", "AssetTypeTabs.js");
let assetTypeTabsWxml = fs.readFileSync(assetTypeTabsWxmlPath, "utf8");
if (!assetTypeTabsWxml.includes("data-asset-value=")) throw new Error("AssetTypeTabs data-asset-value not found");
assetTypeTabsWxml = replaceNativeAssetBindings(assetTypeTabsWxml, /bindtap="\{\{item\.[^}]+\}\}"/g, 'bindtap="nativeAssetTypeSelect"', 1, "AssetTypeTabs");
fs.writeFileSync(assetTypeTabsWxmlPath, assetTypeTabsWxml);
const assetTypeNativeMethods = String.raw`nativeAssetTypeSelect(event){const dataset=event&&event.currentTarget&&event.currentTarget.dataset?event.currentTarget.dataset:{};const value=String(dataset.assetValue||"all");const bridge=globalThis.__xianzhiAssetNativeBridge;if(bridge&&typeof bridge.setType==="function"){bridge.setType(value);return}wx.navigateTo({url:"/pages/user/UserAssetsListPage?type="+encodeURIComponent(value)})}`;
injectNativeAssetMethods(assetTypeTabsJsPath, "AssetTypeTabs", assetTypeNativeMethods, ["nativeAssetTypeSelect"]);

const assetStatusTabsWxmlPath = path.resolve(outputRoot, "components", "assets", "AssetStatusTabs.wxml");
const assetStatusTabsJsPath = path.resolve(outputRoot, "components", "assets", "AssetStatusTabs.js");
let assetStatusTabsWxml = fs.readFileSync(assetStatusTabsWxmlPath, "utf8");
if (!assetStatusTabsWxml.includes("data-asset-value=")) throw new Error("AssetStatusTabs data-asset-value not found");
assetStatusTabsWxml = replaceNativeAssetBindings(assetStatusTabsWxml, /bindtap="\{\{item\.[^}]+\}\}"/g, 'bindtap="nativeAssetStatusSelect"', 1, "AssetStatusTabs");
fs.writeFileSync(assetStatusTabsWxmlPath, assetStatusTabsWxml);
const assetStatusNativeMethods = String.raw`nativeAssetStatusSelect(event){const dataset=event&&event.currentTarget&&event.currentTarget.dataset?event.currentTarget.dataset:{};const value=String(dataset.assetValue||"recent");const bridge=globalThis.__xianzhiAssetNativeBridge;if(bridge&&typeof bridge.setStatus==="function"){bridge.setStatus(value);return}wx.navigateTo({url:"/pages/user/UserAssetsListPage?status="+encodeURIComponent(value)})}`;
injectNativeAssetMethods(assetStatusTabsJsPath, "AssetStatusTabs", assetStatusNativeMethods, ["nativeAssetStatusSelect"]);

const generationTaskItemWxmlPath = path.resolve(outputRoot, "components", "assets", "GenerationTaskItem.wxml");
const generationTaskItemJsPath = path.resolve(outputRoot, "components", "assets", "GenerationTaskItem.js");
if (!fs.existsSync(generationTaskItemWxmlPath) || !fs.existsSync(generationTaskItemJsPath)) {
  throw new Error("GenerationTaskItem build output not found");
}
let generationTaskItemWxml = fs.readFileSync(generationTaskItemWxmlPath, "utf8");
if (!generationTaskItemWxml.includes("data-task-id=")) throw new Error("GenerationTaskItem data-task-id not found");
generationTaskItemWxml = replaceNativeAssetBindings(
  generationTaskItemWxml,
  /(<view class="task-item[^"]*"[^>]*?)bindtap="\{\{[^}]+\}\}"/g,
  '$1bindtap="nativeGenerationTaskOpen"',
  1,
  "GenerationTaskItem open"
);
for (const binding of [
  ["取消", "nativeGenerationTaskCancel"],
  ["重试", "nativeGenerationTaskRetry"],
  ["结果", "nativeGenerationTaskResult"],
]) {
  generationTaskItemWxml = replaceNativeAssetBindings(
    generationTaskItemWxml,
    new RegExp(`(<button[^>]*data-task-id="\\{\\{[^}]+\\}\\}"[^>]*?)(?:bind|catch)tap="\\{\\{[^}]+\\}\\}"([^>]*>${binding[0]}<\\/button>)`, "g"),
    `$1catchtap="${binding[1]}"$2`,
    1,
    `GenerationTaskItem ${binding[0]}`
  );
}
fs.writeFileSync(generationTaskItemWxmlPath, generationTaskItemWxml);
const generationTaskNativeMethods = String.raw`nativeGenerationTaskOpen(event){const dataset=event&&event.currentTarget&&event.currentTarget.dataset?event.currentTarget.dataset:{};const id=String(dataset.taskId||"");const bridge=globalThis.__xianzhiAssetNativeBridge;if(id&&bridge&&typeof bridge.openTask==="function"){bridge.openTask(id);return}wx.showToast({title:"任务入口初始化中，请稍后重试",icon:"none"})},nativeGenerationTaskCancel(event){const dataset=event&&event.currentTarget&&event.currentTarget.dataset?event.currentTarget.dataset:{};const id=String(dataset.taskId||"");const bridge=globalThis.__xianzhiAssetNativeBridge;if(id&&bridge&&typeof bridge.cancelTask==="function"){bridge.cancelTask(id);return}wx.showToast({title:"取消入口初始化中，请稍后重试",icon:"none"})},nativeGenerationTaskRetry(event){const dataset=event&&event.currentTarget&&event.currentTarget.dataset?event.currentTarget.dataset:{};const id=String(dataset.taskId||"");const bridge=globalThis.__xianzhiAssetNativeBridge;if(id&&bridge&&typeof bridge.retryTask==="function"){bridge.retryTask(id);return}wx.showToast({title:"重试入口初始化中，请稍后重试",icon:"none"})},nativeGenerationTaskResult(event){const dataset=event&&event.currentTarget&&event.currentTarget.dataset?event.currentTarget.dataset:{};const id=String(dataset.taskId||"");const bridge=globalThis.__xianzhiAssetNativeBridge;if(id&&bridge&&typeof bridge.openTaskResult==="function"){bridge.openTaskResult(id);return}wx.showToast({title:"结果入口初始化中，请稍后重试",icon:"none"})}`;
injectNativeAssetMethods(generationTaskItemJsPath, "GenerationTaskItem", generationTaskNativeMethods, ["nativeGenerationTaskOpen", "nativeGenerationTaskCancel", "nativeGenerationTaskRetry", "nativeGenerationTaskResult"]);

const assetCardWxmlPath = path.resolve(outputRoot, "components", "assets", "AssetCard.wxml");
const assetCardJsPath = path.resolve(outputRoot, "components", "assets", "AssetCard.js");
if (!fs.existsSync(assetCardWxmlPath) || !fs.existsSync(assetCardJsPath)) throw new Error("AssetCard build output not found");
let assetCardWxml = fs.readFileSync(assetCardWxmlPath, "utf8");
if (!assetCardWxml.includes("data-asset-id=")) throw new Error("AssetCard data-asset-id not found");
assetCardWxml = replaceNativeAssetBindings(
  assetCardWxml,
  /(<view class="\{\{\[[^\]]*'asset-card'[^\]]*\]\}\}"[^>]*data-asset-id="\{\{[^}]+\}\}"[^>]*?)bindtap="\{\{[^}]+\}\}"/g,
  '$1bindtap="nativeAssetCardOpen"',
  1,
  "AssetCard open"
);
assetCardWxml = replaceNativeAssetBindings(
  assetCardWxml,
  /(<button[^>]*class="favorite-button[^"]*"[^>]*data-asset-id="\{\{[^}]+\}\}"[^>]*?)(?:bind|catch)tap="\{\{[^}]+\}\}"/g,
  '$1catchtap="nativeAssetCardFavorite"',
  1,
  "AssetCard favorite"
);
assetCardWxml = replaceNativeAssetBindings(
  assetCardWxml,
  /(<button[^>]*class="more-button[^"]*"[^>]*data-asset-id="\{\{[^}]+\}\}"[^>]*?)(?:bind|catch)tap="\{\{[^}]+\}\}"/g,
  '$1catchtap="nativeAssetCardActions"',
  1,
  "AssetCard actions"
);
fs.writeFileSync(assetCardWxmlPath, assetCardWxml);
const assetCardNativeMethods = String.raw`nativeAssetCardOpen(event){const dataset=event&&event.currentTarget&&event.currentTarget.dataset?event.currentTarget.dataset:{};const id=String(dataset.assetId||"");const bridge=globalThis.__xianzhiAssetNativeBridge;if(id&&bridge&&typeof bridge.openAsset==="function"){bridge.openAsset(id);return}wx.showToast({title:"作品入口初始化中，请稍后重试",icon:"none"})},nativeAssetCardFavorite(event){const dataset=event&&event.currentTarget&&event.currentTarget.dataset?event.currentTarget.dataset:{};const id=String(dataset.assetId||"");const bridge=globalThis.__xianzhiAssetNativeBridge;if(id&&bridge&&typeof bridge.favoriteAsset==="function"){bridge.favoriteAsset(id);return}wx.showToast({title:"收藏入口初始化中，请稍后重试",icon:"none"})},nativeAssetCardActions(event){const dataset=event&&event.currentTarget&&event.currentTarget.dataset?event.currentTarget.dataset:{};const id=String(dataset.assetId||"");const bridge=globalThis.__xianzhiAssetNativeBridge;if(id&&bridge&&typeof bridge.openAssetActions==="function"){bridge.openAssetActions(id);return}wx.showToast({title:"操作入口初始化中，请稍后重试",icon:"none"})}`;
injectNativeAssetMethods(assetCardJsPath, "AssetCard", assetCardNativeMethods, ["nativeAssetCardOpen", "nativeAssetCardFavorite", "nativeAssetCardActions"]);

const assetDetailWxmlPath = path.resolve(outputRoot, "components", "assets", "AssetDetailCenterPage.wxml");
const assetDetailJsPath = path.resolve(outputRoot, "components", "assets", "AssetDetailCenterPage.js");
if (!fs.existsSync(assetDetailWxmlPath) || !fs.existsSync(assetDetailJsPath)) throw new Error("AssetDetailCenterPage build output not found");
let assetDetailWxml = fs.readFileSync(assetDetailWxmlPath, "utf8");
assetDetailWxml = replaceNativeAssetBindings(
  assetDetailWxml,
  /(<view[^>]*class="(?:cover-preview[^"]*|\{\{\[[^"]*'cover-preview'[^"]*\]\}\})"[^>]*data-asset-id="\{\{[^}]+\}\}"[^>]*?)bindtap="\{\{[^}]+\}\}"/g,
  '$1bindtap="nativeAssetDetailPreview"',
  1,
  "AssetDetail preview"
);
assetDetailWxml = replaceNativeAssetBindings(
  assetDetailWxml,
  /(<button[^>]*class="download-button[^"]*"[^>]*?)bindtap="\{\{[^}]+\}\}"/g,
  '$1bindtap="nativeAssetDetailDownload"',
  1,
  "AssetDetail download"
);
assetDetailWxml = replaceNativeAssetBindings(
  assetDetailWxml,
  /(<button[^>]*class="primary-button[^"]*"[^>]*?)bindtap="\{\{[^}]+\}\}"/g,
  '$1bindtap="nativeAssetDetailEdit"',
  1,
  "AssetDetail edit"
);
assetDetailWxml = replaceNativeAssetBindings(
  assetDetailWxml,
  /(<button[^>]*class="regenerate-button[^"]*"[^>]*?)bindtap="\{\{[^}]+\}\}"/g,
  '$1bindtap="nativeAssetDetailRegenerate"',
  1,
  "AssetDetail regenerate"
);
assetDetailWxml = replaceNativeAssetBindings(
  assetDetailWxml,
  /(<button[^>]*class="copy-button[^"]*"[^>]*?)bindtap="\{\{[^}]+\}\}"/g,
  '$1bindtap="nativeAssetDetailCopyPrompt"',
  1,
  "AssetDetail copy prompt"
);
assetDetailWxml = replaceNativeAssetBindings(
  assetDetailWxml,
  /(<button[^>]*class="back-button[^"]*"[^>]*?)bindtap="\{\{[^}]+\}\}"/g,
  '$1bindtap="nativeAssetDetailBack"',
  1,
  "AssetDetail back"
);
fs.writeFileSync(assetDetailWxmlPath, assetDetailWxml);
const assetDetailNativeMethods = String.raw`nativeAssetDetailOpenCreation(intent){const base=wx.getStorageSync("zhiqiyun:asset-detail:creation-draft")||{};const draft=Object.assign({},base,{intent});if(!draft.prompt){wx.showToast({title:"作品缺少原提示词",icon:"none"});return}wx.setStorageSync("v531-creation-prompt",String(draft.prompt||""));wx.setStorageSync("v532-studio-draft",draft);const routes={image:"/pages/user/UserImageCreationPage",video:"/pages/user/UserVideoCreationPage",ppt:"/pages/user/UserPptCreationPage",agent:"/pages/user/UserAgentCreationPage",infographic:"/pages/user/UserInfographicCreationPage"};wx.navigateTo({url:routes[String(draft.mode||"image")]||routes.image,fail(){wx.showToast({title:"创作页面打开失败",icon:"none"})}})},nativeAssetDetailPreview(){const bridge=globalThis.__xianzhiAssetNativeBridge;if(bridge&&typeof bridge.previewCurrentAsset==="function"){bridge.previewCurrentAsset();return}wx.showToast({title:"预览入口初始化中，请稍后重试",icon:"none"})},nativeAssetDetailDownload(){const bridge=globalThis.__xianzhiAssetNativeBridge;if(bridge&&typeof bridge.downloadCurrentAsset==="function"){bridge.downloadCurrentAsset();return}wx.showToast({title:"下载入口初始化中，请稍后重试",icon:"none"})},nativeAssetDetailEdit(){const bridge=globalThis.__xianzhiAssetNativeBridge;if(bridge&&typeof bridge.editCurrentAsset==="function"){bridge.editCurrentAsset();return}this.nativeAssetDetailOpenCreation("edit")},nativeAssetDetailRegenerate(){const bridge=globalThis.__xianzhiAssetNativeBridge;if(bridge&&typeof bridge.regenerateCurrentAsset==="function"){bridge.regenerateCurrentAsset();return}this.nativeAssetDetailOpenCreation("regenerate")},nativeAssetDetailCopyPrompt(){const bridge=globalThis.__xianzhiAssetNativeBridge;if(bridge&&typeof bridge.copyCurrentPrompt==="function"){bridge.copyCurrentPrompt();return}const draft=wx.getStorageSync("zhiqiyun:asset-detail:creation-draft")||{};if(draft.prompt)wx.setClipboardData({data:String(draft.prompt),success(){wx.showToast({title:"提示词已复制",icon:"success"})}})},nativeAssetDetailBack(){const pages=getCurrentPages();if(pages.length>1){wx.navigateBack();return}wx.switchTab({url:"/pages/user/UserAssetsPage",fail(){wx.reLaunch({url:"/pages/user/UserAssetsPage"})}})}`;
injectNativeAssetMethods(assetDetailJsPath, "AssetDetailCenterPage", assetDetailNativeMethods, ["nativeAssetDetailPreview", "nativeAssetDetailDownload", "nativeAssetDetailEdit", "nativeAssetDetailRegenerate", "nativeAssetDetailCopyPrompt", "nativeAssetDetailBack"]);

const assetEmptyStateWxmlPath = path.resolve(outputRoot, "components", "assets", "AssetEmptyState.wxml");
const assetEmptyStateJsPath = path.resolve(outputRoot, "components", "assets", "AssetEmptyState.js");
let assetEmptyStateWxml = fs.readFileSync(assetEmptyStateWxmlPath, "utf8");
assetEmptyStateWxml = replaceNativeAssetBindings(assetEmptyStateWxml, /(<button[^>]*class="empty-action[^"]*"[^>]*?)bindtap="\{\{[^}]+\}\}"/g, "$1bindtap=\"nativeAssetEmptyAction\"", 1, "AssetEmptyState");
fs.writeFileSync(assetEmptyStateWxmlPath, assetEmptyStateWxml);
const assetEmptyNativeMethods = String.raw`nativeAssetEmptyAction(){const bridge=globalThis.__xianzhiAssetNativeBridge;if(bridge&&typeof bridge.emptyAction==="function"){bridge.emptyAction();return}wx.switchTab({url:"/pages/user/UserCreationPage",fail(){wx.reLaunch({url:"/pages/user/UserCreationPage"})}})}`;
injectNativeAssetMethods(assetEmptyStateJsPath, "AssetEmptyState", assetEmptyNativeMethods, ["nativeAssetEmptyAction"]);

const commonAssetsPath = assertGeneratedPath(path.resolve(outputRoot, "common", "assets.js"));
if (!fs.existsSync(commonAssetsPath)) {
  throw new Error("Generated common/assets.js was not found");
}
const commonAssetsOriginal = fs.readFileSync(commonAssetsPath, "utf8");
const commonAssetsUpdated = logoFile
  ? commonAssetsOriginal.split(`/assets/${logoFile}`).join(logoPath)
  : commonAssetsOriginal;
if (logoFile && commonAssetsUpdated === commonAssetsOriginal) {
  throw new Error("Generated transparent logo reference was not found in common/assets.js");
}
fs.writeFileSync(commonAssetsPath, commonAssetsUpdated);

// PromotionCenterPage is now a thin wrapper around components/promotion/PromotionCenterScreen.
// No cross-package style module relocation is required.

for (const moduleName of ["api.js", "availability.js", "platform.js"]) {
  relocateGeneratedModule(
    `features/payment/${moduleName}`,
    `pages/user-account/features/payment/${moduleName}`,
    (source) => source.replace(/require\("\.\.\/\.\.\//g, 'require("../../../../')
  );
}
for (const pageName of ["UserOrderConfirmPage.js", "UserVirtualPaymentPage.js", "UserVirtualPaymentTestPage.js", "UserCommerceOrderConfirmPage.js"]) {
  const pagePath = assertGeneratedPath(path.resolve(outputRoot, "pages", "user-account", pageName));
  const original = fs.readFileSync(pagePath, "utf8");
  const updated = original
    .split('require("../../features/payment/')
    .join('require("./features/payment/');
  if (updated === original) {
    throw new Error(`Payment subpackage module reference was not rewritten: ${pageName}`);
  }
  fs.writeFileSync(pagePath, updated);
}

for (const extension of ["js", "json", "wxml", "wxss"]) {
  relocateGeneratedModule(
    `components/commerce/UserCommerceProductDetail.${extension}`,
    `pages/user-account/components/commerce/UserCommerceProductDetail.${extension}`,
    extension === "js"
      ? (source) => source
          .replace(/require\("\.\.\/\.\.\//g, 'require("../../../../')
          .replace('require("../../../../features/payment/', 'require("../../features/payment/')
      : undefined
  );
}
for (const pageName of ["UserMembershipDetailPage", "UserAgentDetailPage"]) {
  const pageJsPath = assertGeneratedPath(path.resolve(outputRoot, "pages", "user-account", `${pageName}.js`));
  const pageJsonPath = assertGeneratedPath(path.resolve(outputRoot, "pages", "user-account", `${pageName}.json`));
  const originalJs = fs.readFileSync(pageJsPath, "utf8");
  const updatedJs = originalJs
    .split('"../../components/commerce/UserCommerceProductDetail.js"')
    .join('"./components/commerce/UserCommerceProductDetail.js"');
  if (updatedJs === originalJs) throw new Error(`Commerce detail component JS reference was not rewritten: ${pageName}`);
  fs.writeFileSync(pageJsPath, updatedJs);
  const originalJson = fs.readFileSync(pageJsonPath, "utf8");
  const updatedJson = originalJson
    .split('"../../components/commerce/UserCommerceProductDetail"')
    .join('"./components/commerce/UserCommerceProductDetail"');
  if (updatedJson === originalJson) throw new Error(`Commerce detail component JSON reference was not rewritten: ${pageName}`);
  fs.writeFileSync(pageJsonPath, updatedJson);
}

rewriteGeneratedUserRoutes(outputRoot);
console.log("Preserved the generated login page and patched mp-weixin generation controls.");
