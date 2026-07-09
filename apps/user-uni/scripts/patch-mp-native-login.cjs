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
const logoPath = logoFile ? `/assets/${logoFile}` : "";

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
    <button class="button wechat" loading="{{wechatLoading}}" disabled="{{busy}}" bindtap="realWxLogin">
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
      timeout: 600000,
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

  completeLogin(auth, source) {
    const token = auth && auth.accessToken ? auth.accessToken : "";
    if (!token) throw new Error("missing accessToken");
    wx.setStorageSync("token", token);
    wx.setStorageSync("xianzhiMiniProgramAuth", auth || {});
    setTimeout(() => {
      wx.reLaunch({
        url: "/pages/MiniProgramHomePage",
        fail: (error) => {
          this.setStatus("登录成功，但跳转工作台失败：" + messageOf(error, "跳转失败"), "error");
        }
      });
    }, 300);
    this.setStatus(source + "成功，工作区=" + (auth.workspace || "-"), "success");
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
      this.completeLogin(auth, "账号密码登录");
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
      const auth = await api("/api/v1/auth/wechat-mini-program/login", {
        method: "POST",
        body: JSON.stringify({ code: "mock-devtools-code" })
      });
      this.completeLogin(auth, "模拟登录");
    } catch (error) {
      this.setStatus("模拟登录失败：" + messageOf(error, "模拟登录失败"), "error");
    } finally {
      this.setBusy("mockLoading", false);
    }
  },` : `  mockLogin() {
    this.setStatus("模拟登录未启用", "error");
  },`}

  realWxLogin() {
    if (this.data.busy) return;
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
          const auth = await api("/api/v1/auth/wechat-mini-program/login", {
            method: "POST",
            body: JSON.stringify({ code: result.code })
          });
          this.completeLogin(auth, "微信登录");
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
  font-weight: 800;
}

.title {
  margin-top: 5px;
  color: #111827;
  font-size: 22px;
  font-weight: 900;
}

.heading {
  margin-top: 18px;
  gap: 10px;
}

.headline {
  color: #101c35;
  font-size: 30px;
  line-height: 1.15;
  font-weight: 900;
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
  font-weight: 800;
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
      timeout: 600000,
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
  font-weight: 800;
}

.home-title {
  color: #111827;
  font-size: 22px;
  font-weight: 900;
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
  font-weight: 800;
}

.home-name {
  color: #101c35;
  font-size: 24px;
  font-weight: 900;
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
  font-weight: 900;
}

.module-title {
  margin-top: 12px;
  color: #111827;
  font-size: 15px;
  font-weight: 900;
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
  font-weight: 900;
}

.section-title {
  color: #101c35;
  font-size: 20px;
  font-weight: 900;
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
  font-weight: 900;
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
  font-weight: 800;
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
  font-weight: 900;
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
  font-weight: 900;
}

.work-main {
  min-width: 0;
  flex: 1;
  gap: 5px;
}

.work-title {
  color: #111827;
  font-size: 14px;
  font-weight: 900;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
`;

fs.writeFileSync(path.resolve(pageRoot, "WechatLoginPage.wxml"), wxml);
fs.writeFileSync(path.resolve(pageRoot, "WechatLoginPage.js"), js);
fs.writeFileSync(path.resolve(pageRoot, "WechatLoginPage.wxss"), wxss);
const homeWxssPath = path.resolve(pageRoot, "MiniProgramHomePage.wxss");
if (!fs.existsSync(homeWxssPath)) {
  fs.writeFileSync(homeWxssPath, "");
}
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
appJson.pages = appJson.pages.filter((page) => page !== "pages/NativeDebugLogin");
fs.writeFileSync(appJsonPath, JSON.stringify(appJson, null, 2));

console.log("Patched mp-weixin login page with native bindtap handlers.");
