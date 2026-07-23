<template>
  <view class="guest-site">
    <view class="guest-site-glow guest-site-glow-a"></view>
    <view class="guest-site-glow guest-site-glow-b"></view>

    <header class="guest-nav">
      <view class="guest-brand">
        <image :src="logo" class="guest-brand-logo" mode="aspectFit" />
        <view>
          <text class="guest-brand-name">知启云 AI</text>
          <text class="guest-brand-note">创意，从体验开始</text>
        </view>
      </view>
      <view class="guest-nav-links">
        <button @click="open('videoGeneration')">AI 视频</button>
        <button @click="open('wirelessCanvas')">无线画布</button>
        <button @click="open('ppt')">PPT 文档</button>
        <button @click="open('agents')">智能体</button>
      </view>
      <button class="guest-login-button" @click="$emit('login')">登录</button>
    </header>

    <main>
      <section class="guest-hero">
        <view class="guest-hero-copy">
          <view class="guest-kicker"><text></text>无需登录，先浏览、先构思、先填写</view>
          <text class="guest-hero-title">把一个想法，变成<br /><text>可以交付的作品</text></text>
          <text class="guest-hero-description">视频、画布、PPT、文档与智能体集中在一个创作空间。你可以先体验产品，真正生成时再登录，已经填写的内容会完整保留。</text>
          <view class="guest-prompt-card">
            <textarea v-model="prompt" maxlength="1000" placeholder="描述你的创意，例如：为一款东方茶饮制作 9:16 夏日产品短片……" />
            <view class="guest-prompt-foot">
              <view class="guest-prompt-hints">
                <button v-for="item in promptHints" :key="item" @click="prompt = item">{{ item }}</button>
              </view>
              <button class="guest-primary-action" @click="startWithPrompt">开始创作 <text>→</text></button>
            </view>
          </view>
          <view class="guest-safe-note"><text>✓</text>点击“开始创作”不会要求登录，提交生成时才会提示</view>
        </view>

        <view class="guest-hero-preview" aria-label="知启云 AI 创作能力预览">
          <view class="preview-window">
            <view class="preview-window-head">
              <view><text></text><text></text><text></text></view>
              <text>创作工作台</text>
              <text>游客</text>
            </view>
            <view class="preview-canvas">
              <view class="preview-sidebar">
                <text class="active">✦</text><text>▶</text><text>▤</text><text>◇</text>
              </view>
              <view class="preview-stage">
                <view class="preview-art">
                  <view class="preview-orbit"></view>
                  <view class="preview-product"><text>IDEA</text><text>TO LIFE</text></view>
                </view>
                <view class="preview-progress"><view></view><text>AI 正在构建你的创意</text></view>
              </view>
              <view class="preview-panel">
                <text>创作参数</text>
                <view><text>模型</text><text>Video Pro</text></view>
                <view><text>比例</text><text>9 : 16</text></view>
                <view><text>时长</text><text>10 秒</text></view>
                <button>生成作品</button>
              </view>
            </view>
          </view>
          <view class="preview-float preview-float-top"><text>12+</text><text>创作能力</text></view>
          <view class="preview-float preview-float-bottom"><text>参数已保存</text><text>登录后可继续生成</text></view>
        </view>
      </section>

      <section class="guest-proof">
        <text>一站式 AI 创作工作台</text>
        <view><text>先体验</text><text>参数自动保留</text><text>多端同步</text><text>按需登录</text></view>
      </section>

      <section class="guest-section guest-capabilities">
        <view class="guest-section-heading">
          <text class="guest-section-kicker">EXPLORE CAPABILITIES</text>
          <text class="guest-section-title">先找到适合你的创作方式</text>
          <text class="guest-section-copy">所有能力入口均可直接打开浏览，创作参数可以先填，确认需要生成时再登录。</text>
        </view>
        <view class="guest-capability-grid">
          <button v-for="item in capabilities" :key="item.module" class="guest-capability-card" @click="open(item.module)">
            <view :class="['guest-capability-icon', item.tone]">{{ item.icon }}</view>
            <text>{{ item.title }}</text>
            <text>{{ item.description }}</text>
            <view><text v-for="tag in item.tags" :key="tag">{{ tag }}</text></view>
            <text class="guest-card-link">打开体验 →</text>
          </button>
        </view>
      </section>

      <section class="guest-section guest-flow-section">
        <view class="guest-section-heading">
          <text class="guest-section-kicker">HOW IT WORKS</text>
          <text class="guest-section-title">登录不再是第一步</text>
        </view>
        <view class="guest-flow">
          <view v-for="(item, index) in flow" :key="item.title" class="guest-flow-item">
            <text class="guest-flow-index">0{{ index + 1 }}</text>
            <view><text>{{ item.title }}</text><text>{{ item.description }}</text></view>
          </view>
        </view>
      </section>

      <section class="guest-final-cta">
        <view>
          <text>你的下一个作品，可以从一句话开始</text>
          <text>不用先注册账号。进入工作台、完成构思，准备生成时再登录。</text>
        </view>
        <button @click="open('videoGeneration')">进入创作工作台 →</button>
      </section>
    </main>

    <footer class="guest-footer">
      <view class="guest-brand">
        <image :src="logo" class="guest-brand-logo" mode="aspectFit" />
        <text class="guest-brand-name">知启云 AI</text>
      </view>
      <text>AI 创作，从体验开始</text>
      <view class="guest-legal-links">
        <text @click="openLegalDocument('user-agreement')">用户协议</text>
        <text>·</text>
        <text @click="openLegalDocument('privacy-policy')">隐私政策</text>
        <text>· 帮助中心</text>
      </view>
    </footer>
  </view>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { openLegalDocument } from "../features/legal/navigation";

type GuestModule = "videoGeneration" | "wirelessCanvas" | "ppt" | "agents";

defineProps<{ logo: string }>();
const emit = defineEmits<{
  login: [];
  open: [payload: { module: GuestModule; prompt?: string }];
}>();

const prompt = ref("");
const promptHints = ["产品宣传短片", "品牌发布会 PPT", "社交媒体创意"];
const capabilities: Array<{ module: GuestModule; icon: string; title: string; description: string; tags: string[]; tone: string }> = [
  { module: "videoGeneration", icon: "▶", title: "AI 视频", description: "从文字或首尾帧出发，配置比例、清晰度和时长。", tags: ["文生视频", "图生视频"], tone: "violet" },
  { module: "wirelessCanvas", icon: "✦", title: "无线画布", description: "在无限画布中组合素材、提示词、节点与工作流。", tags: ["自由编排", "灵感探索"], tone: "cyan" },
  { module: "ppt", icon: "▤", title: "PPT 与文档", description: "输入主题、页数和风格，先完善大纲再生成内容。", tags: ["演示文稿", "智能文档"], tone: "amber" },
  { module: "agents", icon: "◇", title: "智能体", description: "浏览官方智能体，让专业角色协助完成具体任务。", tags: ["官方智能体", "专业助手"], tone: "blue" },
];
const flow = [
  { title: "浏览能力", description: "先看模板、案例、模型和产品能力，无需账号。" },
  { title: "填写创意", description: "输入 Prompt，选择模型、比例、时长与高级参数。" },
  { title: "确认生成", description: "只有真正执行生成、保存或下载时才提示登录。" },
  { title: "继续刚才操作", description: "登录后回到原页面，参数保留并继续执行一次。" },
];

function open(module: GuestModule) {
  emit("open", { module });
}

function startWithPrompt() {
  emit("open", { module: "videoGeneration", prompt: prompt.value.trim() });
}
</script>

<style scoped>
.guest-site {
  --guest-ink: #ecf4ff;
  --guest-muted: #93a4bc;
  --guest-line: rgba(148, 163, 184, .17);
  --guest-blue: #5b8cff;
  position: relative;
  min-height: 100vh;
  overflow: hidden;
  background: #070b14;
  color: var(--guest-ink);
  font-family: Inter, "PingFang SC", "Microsoft YaHei", sans-serif;
}

.guest-site-glow { position: absolute; border-radius: 999px; filter: blur(30px); pointer-events: none; }
.guest-site-glow-a { top: 40px; left: -180px; width: 520px; height: 520px; background: rgba(37, 99, 235, .16); }
.guest-site-glow-b { top: 420px; right: -240px; width: 640px; height: 640px; background: rgba(124, 58, 237, .12); }

.guest-nav {
  position: relative;
  z-index: 5;
  display: grid;
  grid-template-columns: 1fr auto 1fr;
  align-items: center;
  width: min(1180px, calc(100% - 48px));
  height: 76px;
  margin: 0 auto;
  border-bottom: 1px solid var(--guest-line);
}

.guest-brand { display: flex; align-items: center; gap: 10px; }
.guest-brand > view { display: grid; gap: 2px; }
.guest-brand-logo { width: 34px; height: 34px; }
.guest-brand-name { font-size: 16px; font-weight: 900; letter-spacing: .04em; }
.guest-brand-note { color: #718096; font-size: 10px; letter-spacing: .08em; }
.guest-nav-links { display: flex; gap: 6px; }
.guest-nav-links button, .guest-login-button { margin: 0; border: 0; background: transparent; color: #aebbd0; font-size: 13px; }
.guest-nav-links button::after, .guest-login-button::after, .guest-prompt-hints button::after, .guest-capability-card::after { border: 0; }
.guest-nav-links button:hover { color: #fff; }
.guest-login-button { justify-self: end; min-width: 72px; border: 1px solid rgba(147, 197, 253, .38); border-radius: 10px; color: #dbeafe; }

.guest-hero {
  position: relative;
  z-index: 1;
  display: grid;
  grid-template-columns: minmax(0, .92fr) minmax(500px, 1.08fr);
  align-items: center;
  gap: 64px;
  width: min(1180px, calc(100% - 48px));
  min-height: 690px;
  margin: 0 auto;
  padding: 54px 0 72px;
  box-sizing: border-box;
}

.guest-hero-copy { display: grid; justify-items: start; }
.guest-kicker { display: flex; align-items: center; gap: 8px; margin-bottom: 22px; color: #9db7e9; font-size: 12px; letter-spacing: .08em; }
.guest-kicker > text { width: 7px; height: 7px; border-radius: 50%; background: #67e8f9; box-shadow: 0 0 16px #22d3ee; }
.guest-hero-title { color: #f8fbff; font-size: clamp(42px, 5vw, 68px); font-weight: 900; line-height: 1.08; letter-spacing: -.055em; }
.guest-hero-title > text { background: linear-gradient(100deg, #75a7ff, #a78bfa 56%, #67e8f9); -webkit-background-clip: text; color: transparent; }
.guest-hero-description { max-width: 600px; margin-top: 22px; color: var(--guest-muted); font-size: 16px; line-height: 1.85; }

.guest-prompt-card { width: 100%; margin-top: 30px; padding: 8px; border: 1px solid rgba(96, 165, 250, .34); border-radius: 18px; background: rgba(12, 20, 35, .86); box-shadow: 0 24px 70px rgba(0, 0, 0, .32), inset 0 1px rgba(255,255,255,.04); box-sizing: border-box; }
.guest-prompt-card textarea { width: 100%; min-height: 92px; padding: 14px 15px; box-sizing: border-box; color: #eaf2ff; font-size: 14px; line-height: 1.7; }
.guest-prompt-foot { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.guest-prompt-hints { display: flex; flex-wrap: wrap; gap: 6px; }
.guest-prompt-hints button { margin: 0; padding: 5px 9px; border: 0; border-radius: 8px; background: rgba(148, 163, 184, .08); color: #8191a8; font-size: 10px; line-height: 1.5; }
.guest-primary-action { flex: 0 0 auto; margin: 0; padding: 11px 17px; border: 0; border-radius: 11px; background: linear-gradient(135deg, #3976f6, #6d5dfc); color: #fff; font-size: 13px; font-weight: 800; line-height: 1.5; box-shadow: 0 10px 28px rgba(76, 99, 255, .28); }
.guest-primary-action::after, .guest-final-cta button::after { border: 0; }
.guest-safe-note { display: flex; align-items: center; gap: 7px; margin-top: 12px; color: #708198; font-size: 11px; }
.guest-safe-note > text { color: #34d399; }

.guest-hero-preview { position: relative; perspective: 1200px; }
.preview-window { overflow: hidden; border: 1px solid rgba(148, 163, 184, .18); border-radius: 20px; background: #0c1320; box-shadow: 0 45px 100px rgba(0,0,0,.48), 0 0 80px rgba(59, 130, 246, .08); transform: rotateY(-3deg) rotateX(1deg); }
.preview-window-head { display: grid; grid-template-columns: 1fr auto 1fr; align-items: center; height: 42px; padding: 0 14px; border-bottom: 1px solid var(--guest-line); color: #78879c; font-size: 9px; }
.preview-window-head > view { display: flex; gap: 5px; }
.preview-window-head > view text { width: 7px; height: 7px; border-radius: 50%; background: #263245; }
.preview-window-head > text:last-child { justify-self: end; padding: 3px 7px; border-radius: 999px; background: rgba(52, 211, 153, .09); color: #6ee7b7; }
.preview-canvas { display: grid; grid-template-columns: 48px 1fr 130px; min-height: 390px; }
.preview-sidebar { display: flex; flex-direction: column; align-items: center; gap: 24px; padding-top: 24px; border-right: 1px solid var(--guest-line); color: #617086; }
.preview-sidebar .active { color: #79a5ff; }
.preview-stage { position: relative; display: grid; place-items: center; overflow: hidden; background-image: radial-gradient(rgba(148,163,184,.13) 1px, transparent 1px); background-size: 18px 18px; }
.preview-art { position: relative; display: grid; place-items: center; width: 72%; aspect-ratio: 4/5; overflow: hidden; border: 1px solid rgba(125, 211, 252, .2); border-radius: 8px; background: radial-gradient(circle at 50% 40%, rgba(59,130,246,.5), transparent 30%), linear-gradient(155deg, #08131f, #15284a 55%, #2e1b4d); box-shadow: 0 20px 60px rgba(0,0,0,.4); }
.preview-orbit { position: absolute; width: 180px; height: 180px; border: 1px solid rgba(103, 232, 249, .45); border-radius: 50%; box-shadow: 0 0 40px rgba(34,211,238,.16); transform: rotateX(65deg) rotateZ(-18deg); }
.preview-product { position: relative; z-index: 1; display: grid; justify-items: center; color: #fff; font-weight: 900; letter-spacing: .18em; text-shadow: 0 5px 24px rgba(0,0,0,.5); }
.preview-product text:first-child { font-size: 28px; }
.preview-product text:last-child { color: #8ee8ff; font-size: 10px; }
.preview-progress { position: absolute; right: 10%; bottom: 8%; left: 10%; display: grid; gap: 5px; padding: 8px 10px; border: 1px solid rgba(255,255,255,.08); border-radius: 8px; background: rgba(3,7,18,.76); color: #8da0b9; font-size: 8px; backdrop-filter: blur(8px); }
.preview-progress > view { width: 72%; height: 2px; background: linear-gradient(90deg, #60a5fa, #a78bfa); }
.preview-panel { display: grid; align-content: start; gap: 16px; padding: 20px 12px; border-left: 1px solid var(--guest-line); color: #8fa0b7; font-size: 9px; }
.preview-panel > text { color: #dbeafe; font-size: 11px; font-weight: 800; }
.preview-panel > view { display: grid; gap: 5px; padding-bottom: 10px; border-bottom: 1px solid var(--guest-line); }
.preview-panel > view text:last-child { color: #e5edfa; }
.preview-panel button { margin: 4px 0 0; border: 0; border-radius: 7px; background: #4f6fe9; color: white; font-size: 9px; }
.preview-float { position: absolute; display: grid; gap: 2px; padding: 10px 13px; border: 1px solid rgba(147,197,253,.24); border-radius: 12px; background: rgba(12,19,32,.9); box-shadow: 0 16px 40px rgba(0,0,0,.35); backdrop-filter: blur(12px); }
.preview-float text:first-child { color: #dbeafe; font-size: 12px; font-weight: 900; }
.preview-float text:last-child { color: #75879f; font-size: 8px; }
.preview-float-top { top: 54px; right: -28px; }
.preview-float-bottom { bottom: 40px; left: -34px; }

.guest-proof { position: relative; z-index: 1; display: flex; align-items: center; justify-content: space-between; gap: 30px; width: min(1180px, calc(100% - 48px)); margin: 0 auto; padding: 20px 0; border-top: 1px solid var(--guest-line); border-bottom: 1px solid var(--guest-line); color: #697a91; font-size: 11px; letter-spacing: .08em; }
.guest-proof > view { display: flex; flex-wrap: wrap; gap: 34px; color: #9aa9bd; }

.guest-section { position: relative; z-index: 1; width: min(1180px, calc(100% - 48px)); margin: 0 auto; padding: 112px 0; }
.guest-section-heading { display: grid; justify-items: center; max-width: 680px; margin: 0 auto 44px; text-align: center; }
.guest-section-kicker { color: #6f9cff; font-size: 10px; font-weight: 800; letter-spacing: .2em; }
.guest-section-title { margin-top: 12px; color: #f1f6ff; font-size: 34px; font-weight: 900; letter-spacing: -.035em; }
.guest-section-copy { margin-top: 13px; color: #788aa2; font-size: 14px; line-height: 1.8; }
.guest-capability-grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: 14px; }
.guest-capability-card { display: grid; justify-items: start; gap: 12px; min-height: 270px; margin: 0; padding: 24px; border: 1px solid var(--guest-line); border-radius: 16px; background: linear-gradient(145deg, rgba(19,29,47,.8), rgba(10,15,25,.76)); color: inherit; text-align: left; line-height: 1.5; transition: transform .2s ease, border-color .2s ease; }
.guest-capability-card:hover { transform: translateY(-5px); border-color: rgba(96,165,250,.38); }
.guest-capability-icon { display: grid; place-items: center; width: 42px; height: 42px; border-radius: 12px; font-size: 16px; }
.guest-capability-icon.violet { background: rgba(139,92,246,.15); color: #c4b5fd; }
.guest-capability-icon.cyan { background: rgba(34,211,238,.12); color: #67e8f9; }
.guest-capability-icon.amber { background: rgba(245,158,11,.12); color: #fcd34d; }
.guest-capability-icon.blue { background: rgba(59,130,246,.14); color: #93c5fd; }
.guest-capability-card > text:nth-child(2) { color: #edf5ff; font-size: 17px; font-weight: 900; }
.guest-capability-card > text:nth-child(3) { min-height: 62px; color: #8293aa; font-size: 12px; }
.guest-capability-card > view { display: flex; flex-wrap: wrap; gap: 6px; }
.guest-capability-card > view text { padding: 3px 7px; border: 1px solid rgba(148,163,184,.13); border-radius: 999px; color: #75869d; font-size: 9px; }
.guest-card-link { align-self: end; color: #7ba6ff !important; font-size: 11px !important; }

.guest-flow-section { padding-top: 30px; }
.guest-flow { display: grid; grid-template-columns: repeat(4, 1fr); border-top: 1px solid var(--guest-line); }
.guest-flow-item { position: relative; display: grid; gap: 24px; padding: 26px 22px; border-right: 1px solid var(--guest-line); }
.guest-flow-item:last-child { border-right: 0; }
.guest-flow-index { color: #45699f; font-size: 11px; font-family: monospace; }
.guest-flow-item > view { display: grid; gap: 8px; }
.guest-flow-item > view text:first-child { color: #e6effd; font-size: 15px; font-weight: 800; }
.guest-flow-item > view text:last-child { color: #74869e; font-size: 11px; line-height: 1.75; }

.guest-final-cta { position: relative; z-index: 1; display: flex; align-items: center; justify-content: space-between; gap: 30px; width: min(1120px, calc(100% - 48px)); margin: 10px auto 100px; padding: 42px 48px; border: 1px solid rgba(96,165,250,.2); border-radius: 22px; background: radial-gradient(circle at 12% 10%, rgba(59,130,246,.2), transparent 35%), linear-gradient(120deg, #101a2c, #111426); box-sizing: border-box; }
.guest-final-cta > view { display: grid; gap: 10px; }
.guest-final-cta > view text:first-child { color: #f0f6ff; font-size: 25px; font-weight: 900; }
.guest-final-cta > view text:last-child { color: #8495ac; font-size: 12px; }
.guest-final-cta button { flex: 0 0 auto; margin: 0; padding: 12px 20px; border: 0; border-radius: 11px; background: #f3f7ff; color: #14213a; font-size: 12px; font-weight: 900; }

.guest-footer { position: relative; z-index: 1; display: grid; grid-template-columns: 1fr auto 1fr; align-items: center; width: min(1180px, calc(100% - 48px)); margin: 0 auto; padding: 28px 0 40px; border-top: 1px solid var(--guest-line); color: #607086; font-size: 10px; }
.guest-legal-links { display: flex; gap: 4px; justify-self: end; }
.guest-legal-links text:nth-child(odd) { cursor: pointer; }

@media (max-width: 900px) {
  .guest-nav { grid-template-columns: 1fr auto; }
  .guest-nav-links { display: none; }
  .guest-hero { grid-template-columns: 1fr; padding-top: 62px; }
  .guest-hero-preview { width: min(620px, 100%); margin: 0 auto; }
  .guest-capability-grid, .guest-flow { grid-template-columns: repeat(2, 1fr); }
  .guest-flow-item:nth-child(2) { border-right: 0; }
  .guest-flow-item { border-bottom: 1px solid var(--guest-line); }
}

@media (max-width: 600px) {
  .guest-nav, .guest-hero, .guest-proof, .guest-section, .guest-final-cta, .guest-footer { width: calc(100% - 28px); }
  .guest-nav { height: 66px; }
  .guest-brand-note { display: none; }
  .guest-hero { min-height: auto; gap: 46px; padding: 48px 0 58px; }
  .guest-hero-title { font-size: 42px; }
  .guest-hero-description { font-size: 14px; }
  .guest-prompt-foot { align-items: stretch; flex-direction: column; }
  .guest-primary-action { width: 100%; }
  .preview-canvas { grid-template-columns: 38px 1fr; min-height: 320px; }
  .preview-panel { display: none; }
  .preview-float-top { right: -6px; }
  .preview-float-bottom { left: -6px; }
  .guest-proof { align-items: flex-start; flex-direction: column; }
  .guest-proof > view { gap: 14px 22px; }
  .guest-section { padding: 82px 0; }
  .guest-section-title { font-size: 28px; }
  .guest-capability-grid, .guest-flow { grid-template-columns: 1fr; }
  .guest-flow-item { border-right: 0; }
  .guest-final-cta { align-items: stretch; flex-direction: column; padding: 30px 24px; }
  .guest-final-cta button { width: 100%; }
  .guest-footer { grid-template-columns: 1fr; gap: 12px; }
  .guest-legal-links { justify-self: start; }
}
</style>
