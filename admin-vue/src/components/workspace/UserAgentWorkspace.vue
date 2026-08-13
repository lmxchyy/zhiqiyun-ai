<template>
  <section :class="['user-agent-center-page', { 'has-officecli-workspace': officeCliWorkspaceOpen || agentCenterWorkspace, 'has-agent-workspace': officeCliWorkspaceOpen || agentCenterWorkspace }]">
    <div class="user-agent-desktop-view">
      <section v-if="officeCliWorkspaceOpen" class="user-agent-officecli-workspace">
        <header class="officecli-workspace-head">
          <button type="button" @click="closeOfficeCliWorkspace">返回智能体中心</button>
          <div>
            <span>OfficeCLI 文档智能体</span>
            <h2>文档生成工作台</h2>
            <p>选择 Word、Excel 或 PPT，输入需求后由后端 OfficeCLI 运行层生成可下载文件。</p>
          </div>
          <em :class="['officecli-status-badge', officeCliStatusTone]">{{ officeCliStatusLabel }}</em>
        </header>

        <section class="user-agent-officecli-workbench is-workspace">
          <header>
            <div>
              <span>文档生成控制台</span>
              <strong>输入需求，一键生成 Office 文件</strong>
            </div>
            <button type="button" :disabled="officeCliDocumentGenerating" @click="submitOfficeCliDocument">
              {{ officeCliDocumentGenerating ? '生成中...' : '生成文档' }}
            </button>
          </header>
          <div class="officecli-workbench-body">
            <div class="officecli-form-column">
              <div class="officecli-format-switch" role="radiogroup" aria-label="选择文档格式">
                <button v-for="format in officeCliFormatOptions" :key="format.value" type="button" :class="{ active: officeCliForm.format === format.value }" @click="officeCliForm.format = format.value">
                  <b>{{ format.label }}</b>
                  <span>{{ format.desc }}</span>
                </button>
              </div>
              <label class="officecli-field">
                <span>文档标题</span>
                <input v-model.trim="officeCliForm.title" placeholder="例如：AI 产品周报" />
              </label>
              <label class="officecli-field">
                <span>生成需求</span>
                <textarea v-model.trim="officeCliForm.prompt" rows="5" placeholder="描述你要生成的内容，例如：生成一份面向客户的 OfficeCLI 能力介绍，包含产品价值、适用场景和下一步计划。" />
              </label>
            </div>
            <aside class="officecli-result-card">
              <span>生成结果</span>
              <template v-if="officeCliDocumentResult">
                <strong>{{ officeCliDocumentResult.fileName }}</strong>
                <small>{{ officeCliDocumentResult.format.toUpperCase() }} · {{ officeCliDocumentSizeText }}</small>
                <button type="button" @click="downloadOfficeCliDocument()">下载文件</button>
              </template>
              <template v-else>
                <strong>等待生成</strong>
                <small>生成后会在这里出现下载入口，并保存在后端容器数据目录。</small>
              </template>
            </aside>
          </div>
        </section>
      </section>

      <KnowledgeAgentCenter v-else-if="agentCenterWorkspace?.agentKey === 'knowledge'" @close="closeAgentWorkspace" />
      <section v-else-if="agentCenterWorkspace" class="user-agent-workspace">
        <header class="agent-workspace-head">
          <button type="button" @click="closeAgentWorkspace">返回智能体中心</button>
          <span :class="['agent-workspace-avatar', agentCenterWorkspace.tone]">{{ agentCenterWorkspace.avatar }}</span>
          <div>
            <span>{{ agentCenterWorkspace.modeLabel }}</span>
            <h2>{{ agentCenterWorkspace.name }}</h2>
            <p>{{ agentCenterWorkspace.desc }}</p>
          </div>
          <em :class="['agent-workspace-status', { draft: agentCenterWorkspace.status === '草稿', disabled: agentCenterWorkspace.status === '已停用' }]">{{ agentCenterWorkspace.status }}</em>
        </header>

        <section class="agent-workspace-grid">
          <main class="agent-workspace-chat">
            <header>
              <div>
                <span>交互控制台</span>
                <strong>{{ agentCenterWorkspace.headline }}</strong>
              </div>
              <button type="button" @click="sendAgentWorkspaceMessage">发送测试</button>
            </header>
            <div class="agent-workspace-dialog">
              <article v-for="(message, index) in agentWorkspaceMessages" :key="`${message.role}-${index}`" :class="['agent-message', message.role]">
                <span>{{ message.role === 'user' ? '我' : agentCenterWorkspace.avatar }}</span>
                <p>{{ message.text }}</p>
              </article>
            </div>
            <label class="agent-workspace-input">
              <span>测试指令</span>
              <textarea :value="agentWorkspaceDraft" rows="5" placeholder="输入一条测试指令，检查这个智能体的回复风格与业务边界。" @input="setAgentWorkspaceDraft(($event.target as HTMLTextAreaElement).value)" />
            </label>
          </main>

          <aside class="agent-workspace-config">
            <section class="agent-workspace-card">
              <strong>运行信息</strong>
              <div class="agent-workspace-meta-grid">
                <div><span>类型</span><b>{{ agentCenterWorkspace.type }}</b></div>
                <div><span>模型</span><b>{{ agentCenterWorkspace.model }}</b></div>
                <div><span>知识库</span><b>{{ agentCenterWorkspace.knowledge }}</b></div>
                <div><span>调用次数</span><b>{{ agentCenterWorkspace.calls }}</b></div>
              </div>
            </section>
            <section class="agent-workspace-card">
              <strong>能力配置</strong>
              <div class="agent-tool-tags">
                <span v-for="tag in agentCenterWorkspace.toolTags" :key="tag">{{ tag }}</span>
              </div>
            </section>
            <section class="agent-workspace-card">
              <strong>快捷动作</strong>
              <div class="agent-quick-actions">
                <button v-for="action in agentCenterWorkspace.quickActions" :key="action" type="button" @click="setAgentWorkspaceDraft(action)">{{ action }}</button>
              </div>
            </section>
          </aside>
        </section>
      </section>

      <section v-else class="user-agent-center-layout">
        <main class="user-agent-main-column">
          <section class="user-agent-center-hero">
            <div>
              <span>AGENT CENTER</span>
              <h2>智能体中心</h2>
              <p>创建、调试与运行你的 AI 智能体，连接知识库、工具与业务流程。</p>
            </div>
            <div class="user-agent-hero-robot" aria-hidden="true">
              <i></i>
              <b></b>
              <em>AI</em>
              <strong>BOT</strong>
            </div>
          </section>

          <section class="user-agent-template-panel">
            <header>
              <strong>智能体模板</strong>
              <button type="button">查看更多</button>
            </header>
            <div class="user-agent-template-grid">
              <article
                v-for="template in agentCenterTemplates"
                :key="template.name"
                :class="['user-agent-template-card', { 'is-featured': template.featured, 'is-clickable': true }]"
                tabindex="0"
                @click="handleAgentTemplateCardClick(template)"
                @keydown.enter.prevent="handleAgentTemplateCardClick(template)"
              >
                <span class="user-agent-template-icon" :class="template.tone">{{ template.avatar || 'AI' }}</span>
                <strong>{{ template.name || '未命名模板' }}</strong>
                <p>{{ template.desc || '暂无描述' }}</p>
                <button type="button">{{ template.action || '打开' }}</button>
              </article>
            </div>
          </section>
        </main>
      </section>
    </div>
  </section>
</template>

<script setup lang="ts">
import KnowledgeAgentCenter from "../knowledge/KnowledgeAgentCenter.vue";

interface OfficeCliFormatOption {
  label: string;
  value: string;
  desc: string;
}

interface AgentCenterWorkspace {
  agentKey?: string;
  tone: string;
  avatar: string;
  modeLabel: string;
  name: string;
  desc: string;
  status: string;
  headline: string;
  type: string;
  model: string;
  knowledge: string;
  calls: number | string;
  toolTags: string[];
  quickActions: string[];
}

interface AgentCenterTemplate {
  name?: string;
  desc?: string;
  action?: string;
  avatar?: string;
  tone?: string;
  featured?: boolean;
}

defineProps<{
  officeCliWorkspaceOpen: boolean;
  officeCliStatusTone: string;
  officeCliStatusLabel: string;
  officeCliFormatOptions: OfficeCliFormatOption[];
  officeCliForm: { format: string; title: string; prompt: string };
  officeCliDocumentGenerating: boolean;
  officeCliDocumentResult: { fileName: string; format: string } | null;
  officeCliDocumentSizeText: string;
  agentCenterWorkspace: AgentCenterWorkspace | null;
  agentWorkspaceMessages: Array<{ role: string; text: string }>;
  agentWorkspaceDraft: string;
  agentCenterTemplates: AgentCenterTemplate[];
  closeOfficeCliWorkspace: () => void;
  submitOfficeCliDocument: () => void;
  downloadOfficeCliDocument: () => void;
  closeAgentWorkspace: () => void;
  sendAgentWorkspaceMessage: () => void;
  handleAgentTemplateCardClick: (template: any) => void;
}>();

const emit = defineEmits<{
  (event: 'update:agentWorkspaceDraft', value: string): void;
}>();

function setAgentWorkspaceDraft(value: string) {
  emit('update:agentWorkspaceDraft', value);
}
</script>
