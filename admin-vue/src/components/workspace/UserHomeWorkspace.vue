<template>
  <section class="user-home-page">
    <section class="user-home-layout">
      <main class="user-home-main">
        <section class="user-home-hero">
          <span class="user-home-shard user-home-shard-left"></span>
          <span class="user-home-shard user-home-shard-right"></span>
          <div class="user-home-title-block">
            <h2>专属 AI 视觉设计师已待命，<em>即刻开启创作</em></h2>
            <p>输入想法、上传参考图，或从下面的 Agent 入口开始一段创作。</p>
          </div>

          <div class="user-home-composer">
            <div class="user-home-mode-tabs">
              <button v-for="mode in userHomeCreationModes" :key="mode.id" type="button" :class="{ active: userHomeCreationMode === mode.id }" @click="selectUserHomeCreationMode(mode.id)">
                <el-icon><component :is="mode.icon" /></el-icon>
                <span>{{ mode.title }}</span>
              </button>
            </div>
            <el-input v-model="onlineImageForm.prompt" type="textarea" :rows="4" maxlength="2000" resize="none" placeholder="描述你想生成的画面、视频或文档..." />
            <div class="user-home-toolbar">
              <button type="button" @click="selectAdminModule('userAiImage')"><el-icon><Plus /></el-icon>上传参考图</button>
              <button type="button" @click="applyUserHomePrompt('润色')"><el-icon><EditPen /></el-icon>润色</button>
              <button type="button" @click="applyUserHomePrompt('爆款复刻')"><el-icon><Star /></el-icon>爆款复刻</button>
              <el-select v-model="onlineImageForm.model" class="user-home-select" size="default">
                <el-option v-for="model in onlineModelOptions" :key="model.value" :label="model.label" :value="model.value" />
              </el-select>
              <el-select v-model="onlineImageForm.ratio" class="user-home-select is-compact" size="default">
                <el-option v-for="ratio in userHomeRatioOptions" :key="ratio.value" :label="ratio.label" :value="ratio.value" />
              </el-select>
              <el-select v-model="onlineImageForm.quality" class="user-home-select is-compact" size="default">
                <el-option label="标准" value="standard" />
                <el-option label="高清" value="hd" />
                <el-option label="高质量" value="high" />
              </el-select>
              <button type="button" class="user-home-generate" @click="launchUserHomeCreation">生成 <el-icon><Star /></el-icon></button>
            </div>
          </div>
        </section>

        <section class="user-home-agent-row" aria-label="快捷 Agent">
          <button v-for="agent in userHomeAgentEntries" :key="agent.title" type="button" @click="openUserHomeEntry(agent)">
            <el-icon><component :is="agent.icon" /></el-icon>
            <span>{{ agent.title }}</span>
          </button>
        </section>

        <section class="user-home-template-section">
          <div class="user-home-section-head">
            <h3>模板广场</h3>
            <button type="button" @click="selectAdminModule('userAiImage')">查看全部</button>
          </div>
          <div class="user-home-template-grid">
            <button v-for="template in userHomeTemplates" :key="template.title" type="button" class="user-home-template-card" @click="openUserHomeEntry(template)">
              <span :class="['user-home-template-cover', template.coverClass]"><em>{{ template.coverText }}</em></span>
              <strong>{{ template.title }}</strong>
              <small>{{ template.desc }}</small>
            </button>
          </div>
        </section>
      </main>

      <aside class="user-home-inspiration">
        <div class="user-home-inspiration-head">
          <h3>灵感模板</h3>
        </div>
        <button v-for="item in userHomeInspirations" :key="item.title" type="button" class="user-home-inspiration-item" @click="openUserHomeEntry(item)">
          <span>
            <strong>{{ item.title }}</strong>
            <small>{{ item.desc }}</small>
          </span>
          <i :class="['user-home-inspiration-thumb', item.coverClass]"></i>
        </button>
      </aside>
    </section>
  </section>
</template>

<script setup lang="ts">
import { EditPen, Plus, Star } from "@element-plus/icons-vue";

defineProps<{
  userHomeCreationModes: any[];
  userHomeCreationMode: any;
  onlineImageForm: any;
  onlineModelOptions: any[];
  userHomeRatioOptions: any[];
  userHomeAgentEntries: any[];
  userHomeTemplates: any[];
  userHomeInspirations: any[];
  selectUserHomeCreationMode: any;
  selectAdminModule: any;
  applyUserHomePrompt: any;
  launchUserHomeCreation: any;
  openUserHomeEntry: any;
}>();
</script>
