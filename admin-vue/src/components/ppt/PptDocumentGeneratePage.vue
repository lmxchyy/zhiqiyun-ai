<template>
  <section class="ppt-generate-page">
    <div class="ppt-reference-shell">
      <header class="ppt-reference-topbar" aria-label="PPT 文档生成">
        <button class="ppt-brand-mark" type="button" title="回到 PPT 工作台" aria-label="回到 PPT 工作台" @click="handlePptHomeClick">
          <img :src="xianzhiLogo" alt="知启云 AI" />
        </button>
        <span class="ppt-module-title">PPT文档生成</span>
      </header>

      <main class="ppt-reference-main" :class="{ 'is-home-layout': isHomeWorkspace }">
        <section v-if="isHomeWorkspace" class="ppt-hero-composer">
          <h1>您今天想制作什么样的演示文稿?</h1>

          <p class="ppt-hero-subtitle">输入主题或上传参考资料，AI 将帮您智能生成专业的演示文稿</p>

          <article class="ppt-composer-card" :class="{ 'is-loading': isBusy }">
            <PptPromptInput
              v-model="store.prompt"
              class="ppt-reference-prompt"
              @clear="handleClearPrompt"
              @submit="handlePrimaryGenerate"
            />

            <div class="ppt-composer-footer">
              <div class="ppt-pill-row">
                <div class="ppt-pill-dropdown">
                  <button
                    type="button"
                    class="ppt-pill"
                    :title="`选择幻灯片页数，当前 ${store.slideCount} 张`"
                    :aria-label="`选择幻灯片页数，当前 ${store.slideCount} 张`"
                    :aria-expanded="showSlideCountMenu"
                    aria-haspopup="menu"
                    @click="toggleSlideCountMenu"
                  >
                    <svg class="ppt-panels-icon" viewBox="0 0 24 24" aria-hidden="true">
                      <rect width="18" height="18" x="3" y="3" rx="2" />
                      <path d="M3 9h18" />
                      <path d="M9 21V9" />
                    </svg>
                    <span>{{ store.slideCount }}张幻灯片</span>
                  </button>
                  <div v-if="showSlideCountMenu" class="ppt-pill-menu" role="menu" @keydown.esc="showSlideCountMenu = false">
                    <strong>幻灯片页数</strong>
                    <button
                      v-for="option in slideCountOptions"
                      :key="option"
                      type="button"
                      role="menuitemradio"
                      :aria-checked="store.slideCount === option"
                      :title="`生成 ${option} 张幻灯片`"
                      :aria-label="`${option} 张幻灯片${store.slideCount === option ? '，已选' : ''}`"
                      :class="{ active: store.slideCount === option }"
                      @click="selectSlideCount(option)"
                    >
                      <span>{{ option }}张幻灯片</span>
                      <small v-if="store.slideCount === option">已选</small>
                    </button>
                  </div>
                </div>
                <div class="ppt-pill-dropdown">
                  <button
                    type="button"
                    class="ppt-pill"
                    :title="`选择演示格式，当前 ${generationAspectRatioLabel}`"
                    :aria-label="`选择演示格式，当前 ${generationAspectRatioLabel}`"
                    :aria-expanded="showFormatMenu"
                    aria-haspopup="menu"
                    @click="toggleFormatMenu"
                  >
                    <svg class="ppt-layout-icon" viewBox="0 0 24 24" aria-hidden="true">
                      <rect width="18" height="7" x="3" y="3" rx="1" />
                      <rect width="9" height="7" x="3" y="14" rx="1" />
                      <rect width="5" height="7" x="16" y="14" rx="1" />
                    </svg>
                    <span>{{ generationAspectRatioLabel }}</span>
                  </button>
                  <div v-if="showFormatMenu" class="ppt-pill-menu" role="menu" @keydown.esc="showFormatMenu = false">
                    <strong>演示格式</strong>
                    <button
                      v-for="option in generationAspectRatioOptions"
                      :key="option.value"
                      type="button"
                      role="menuitemradio"
                      :aria-checked="store.generationAspectRatio === option.value"
                      :title="`选择演示格式：${option.label}`"
                      :aria-label="`选择演示格式：${option.label}${store.generationAspectRatio === option.value ? '，已选' : ''}`"
                      :class="{ active: store.generationAspectRatio === option.value }"
                      @click="selectGenerationAspectRatio(option.value)"
                    >
                      <span>{{ option.label }}</span>
                      <small v-if="store.generationAspectRatio === option.value">已选</small>
                    </button>
                  </div>
                </div>
                <div class="ppt-pill-dropdown">
                  <button
                    type="button"
                    class="ppt-pill"
                    :title="`选择语言，当前 ${languageLabel}`"
                    :aria-label="`选择语言，当前 ${languageLabel}`"
                    :aria-expanded="showLanguageMenu"
                    aria-haspopup="menu"
                    @click="toggleLanguageMenu"
                  >
                    <svg class="ppt-language-icon" viewBox="0 0 24 24" aria-hidden="true">
                      <path d="m5 8 6 6" />
                      <path d="m4 14 6-6 2-3" />
                      <path d="M2 5h12" />
                      <path d="M7 2h1" />
                      <path d="m22 22-5-10-5 10" />
                      <path d="M14 18h6" />
                    </svg>
                    <span>{{ languageLabel }}</span>
                  </button>
                  <div v-if="showLanguageMenu" class="ppt-pill-menu" role="menu" @keydown.esc="showLanguageMenu = false">
                    <strong>语言</strong>
                    <button
                      v-for="option in languageOptions"
                      :key="option.value"
                      type="button"
                      role="menuitemradio"
                      :aria-checked="store.language === option.value"
                      :title="`选择语言：${option.label}`"
                      :aria-label="`选择语言：${option.label}${store.language === option.value ? '，已选' : ''}`"
                      :class="{ active: store.language === option.value }"
                      @click="selectLanguage(option.value)"
                    >
                      <span>{{ option.label }}</span>
                      <small v-if="store.language === option.value">已选</small>
                    </button>
                  </div>
                </div>
                <div class="ppt-pill-dropdown">
                  <button
                    type="button"
                    class="ppt-pill"
                    title="打开更多生成选项"
                    aria-label="打开更多生成选项"
                    :aria-expanded="showMoreMenu"
                    aria-haspopup="menu"
                    @click="toggleMoreMenu"
                  >
                    <svg class="ppt-wand-icon" viewBox="0 0 24 24" aria-hidden="true">
                      <path d="m21.64 3.64-1.28-1.28a1.21 1.21 0 0 0-1.72 0L2.36 18.64a1.21 1.21 0 0 0 0 1.72l1.28 1.28a1.2 1.2 0 0 0 1.72 0L21.64 5.36a1.2 1.2 0 0 0 0-1.72" />
                      <path d="m14 7 3 3" />
                      <path d="M5 6v4" />
                      <path d="M19 14v4" />
                      <path d="M10 2v2" />
                      <path d="M7 8H3" />
                      <path d="M21 16h-4" />
                      <path d="M11 3H9" />
                    </svg>
                    <span>更多的</span>
                  </button>
                  <div v-if="showMoreMenu" class="ppt-pill-menu ppt-more-menu" role="menu" @keydown.esc="showMoreMenu = false">
                    <button
                      type="button"
                      role="menuitemcheckbox"
                      :aria-checked="store.enableWebSearch"
                      :title="store.enableWebSearch ? '关闭联网搜索' : '开启联网搜索'"
                      :aria-label="store.enableWebSearch ? '关闭联网搜索' : '开启联网搜索'"
                      :class="{ active: store.enableWebSearch }"
                      @click="toggleWebSearch"
                    >
                      <svg class="ppt-menu-icon" viewBox="0 0 24 24" aria-hidden="true">
                        <circle cx="12" cy="12" r="10" />
                        <path d="M12 2a14.5 14.5 0 0 0 0 20 14.5 14.5 0 0 0 0-20" />
                        <path d="M2 12h20" />
                      </svg>
                      <span>联网搜索</span>
                      <small>{{ store.enableWebSearch ? "开启" : "关闭" }}</small>
                    </button>
                    <button
                      type="button"
                      role="menuitemcheckbox"
                      :aria-checked="store.autoThemeEnabled"
                      :title="store.autoThemeEnabled ? '关闭自动主题' : '开启自动主题'"
                      :aria-label="store.autoThemeEnabled ? '关闭自动主题' : '开启自动主题'"
                      :class="{ active: store.autoThemeEnabled }"
                      @click="toggleAutoTheme"
                    >
                      <svg class="ppt-menu-icon" viewBox="0 0 24 24" aria-hidden="true">
                        <path d="m21.64 3.64-1.28-1.28a1.21 1.21 0 0 0-1.72 0L2.36 18.64a1.21 1.21 0 0 0 0 1.72l1.28 1.28a1.2 1.2 0 0 0 1.72 0L21.64 5.36a1.2 1.2 0 0 0 0-1.72" />
                        <path d="m14 7 3 3" />
                        <path d="M5 6v4" />
                        <path d="M19 14v4" />
                        <path d="M10 2v2" />
                        <path d="M7 8H3" />
                        <path d="M21 16h-4" />
                        <path d="M11 3H9" />
                      </svg>
                      <span>自动主题</span>
                      <small>{{ store.autoThemeEnabled ? "开启" : "关闭" }}</small>
                    </button>
                  </div>
                </div>
                <div class="ppt-pill-dropdown">
                  <button
                    type="button"
                    class="ppt-pill ppt-model-pill"
                    :title="`选择模型，当前 ${currentTextModelLabel}`"
                    :aria-label="`选择模型，当前 ${currentTextModelLabel}`"
                    :aria-expanded="showModelMenu"
                    aria-haspopup="listbox"
                    @click="toggleModelMenu"
                  >
                    <svg class="ppt-bot-icon" viewBox="0 0 24 24" aria-hidden="true">
                      <path d="M12 8V4H8" />
                      <rect width="16" height="12" x="4" y="8" rx="2" />
                      <path d="M2 14h2" />
                      <path d="M20 14h2" />
                      <path d="M15 13v2" />
                      <path d="M9 13v2" />
                    </svg>
                    <span>{{ currentTextModelLabel }}</span>
                    <svg class="ppt-chevron-icon" viewBox="0 0 24 24" aria-hidden="true">
                      <path d="m6 9 6 6 6-6" />
                    </svg>
                  </button>
                  <div v-if="showModelMenu" class="ppt-pill-menu ppt-model-menu" role="listbox" @keydown.esc="showModelMenu = false">
                    <div v-if="textModelsLoading" class="ppt-model-menu-state" aria-live="polite">
                      <span class="ppt-spinner"></span>
                      <span>
                        <b>模型加载中</b>
                        <small>正在读取 PPT 可用模型</small>
                      </span>
                    </div>
                    <template v-else-if="textModelGroups.length">
                      <div
                        v-for="group in textModelGroups"
                        :key="group.label"
                        class="ppt-model-group"
                        role="group"
                        :aria-label="group.label"
                      >
                        <strong>{{ group.label }}</strong>
                        <button
                          v-for="model in group.models"
                          :key="model.value"
                          type="button"
                          role="option"
                          :disabled="model.disabled"
                          :aria-selected="store.textModel === model.value"
                          :title="model.disabled ? `${model.label} 暂不可用` : `选择模型：${model.label}`"
                          :aria-label="`${model.label}${store.textModel === model.value ? '，已选' : ''}${model.disabled ? '，暂不可用' : ''}`"
                          :class="{ active: store.textModel === model.value }"
                          @click="selectTextModel(model.value)"
                        >
                          <svg class="ppt-menu-icon" viewBox="0 0 24 24" aria-hidden="true">
                            <template v-if="model.providerType === 'ollama'">
                              <rect x="5" y="5" width="14" height="14" rx="3" />
                              <path d="M9 9h6v6H9z" />
                              <path d="M9 1v3M15 1v3M9 20v3M15 20v3M1 9h3M1 15h3M20 9h3M20 15h3" />
                            </template>
                            <template v-else-if="model.providerType === 'lmstudio'">
                              <rect x="3" y="4" width="18" height="13" rx="2" />
                              <path d="M8 21h8" />
                              <path d="M12 17v4" />
                            </template>
                            <template v-else>
                              <path d="M12 8V4H8" />
                              <rect width="16" height="12" x="4" y="8" rx="2" />
                              <path d="M2 14h2" />
                              <path d="M20 14h2" />
                              <path d="M15 13v2" />
                              <path d="M9 13v2" />
                            </template>
                          </svg>
                          <span>
                            <b>{{ model.label }}</b>
                            <small>{{ model.description || model.provider || "Text model" }}</small>
                          </span>
                          <small v-if="store.textModel === model.value">已选</small>
                        </button>
                      </div>
                    </template>
                    <div v-else class="ppt-model-menu-state" aria-live="polite">
                      <span>
                        <b>暂无可用模型</b>
                        <small>稍后可在模型接口接入后自动显示</small>
                      </span>
                    </div>
                  </div>
                </div>
              </div>

              <button
                type="button"
                class="ppt-submit-button"
                :disabled="!canSubmit"
                :title="isBusy ? '正在生成PPT，请稍候...' : '生成PPT'"
                :aria-busy="isBusy"
                :aria-label="isBusy ? '正在生成PPT' : '生成PPT'"
                @click="handlePrimaryGenerate"
              >
                <span v-if="isBusy" class="ppt-spinner"></span>
                <svg v-else class="ppt-submit-icon" viewBox="0 0 24 24" aria-hidden="true">
                  <path d="M5 12h14" />
                  <path d="m12 5 7 7-7 7" />
                </svg>
              </button>
            </div>

            <section v-if="showConfig" class="ppt-floating-config">
              <div class="ppt-config-block">
                <div class="ppt-mini-heading">
                  <strong>创建方式</strong>
                  <span>{{ createModeHint }}</span>
                </div>
                <PptCreateModeSelector
                  v-model="store.createMode"
                  :uploaded-document-name="store.uploadedDocumentName"
                  @upload-document="handleUploadDocument"
                />
              </div>

              <div class="ppt-config-block">
                <div class="ppt-mini-heading">
                  <strong>生成参数</strong>
                  <span>后续会作为接口参数提交</span>
                </div>
                <PptGenerationConfigPanel
                  v-model:slide-count="store.slideCount"
                  v-model:language="store.language"
                  v-model:tone="store.tone"
                  v-model:text-content="store.textContent"
                  v-model:audience="store.audience"
                  v-model:scenario="store.scenario"
                  v-model:generation-aspect-ratio="store.generationAspectRatio"
                  v-model:image-source="store.imageSource"
                  v-model:text-model="store.textModel"
                  v-model:image-model="store.imageModel"
                  v-model:enable-web-search="store.enableWebSearch"
                  :text-models="store.textModels"
                  :image-models="store.imageModels"
                />
              </div>

              <div class="ppt-config-block">
                <div class="ppt-mini-heading">
                  <strong>风格/主题</strong>
                  <span>默认商务简约</span>
                </div>
                <PptThemeSelector
                  v-model="store.theme"
                  @create-theme="openPresentationThemeCreator(false)"
                  @import-theme="handlePresentationThemeImport"
                />
              </div>

              <div class="ppt-config-block">
                <div class="ppt-mini-heading">
                  <strong>示例主题推荐</strong>
                  <span>点击后自动填入提示词</span>
                </div>
                <PptExamplePrompts :selected="store.selectedExample" @select="handleExampleSelect" />
              </div>
            </section>
          </article>

          <PptErrorState
            v-if="store.error"
            class="ppt-hero-error"
            :title="store.error.title"
            :message="store.error.message"
            @retry="store.retry"
          />

          <PptGenerationProgress
            v-if="showProgress"
            class="ppt-hero-progress"
            :status="store.status"
            :progress="store.progress"
            :current-page="store.currentPage"
            :slide-count="store.slideCount"
            :status-text="store.statusText"
            :error-message="store.error?.message"
            @retry="store.retry"
          />
        </section>

        <section
          v-else-if="isGenerationWorkspace"
          ref="generationWorkspaceRef"
          class="ppt-generation-workspace"
          :class="{ 'is-pre-outline': isPreOutlineSetup }"
        >
          <header class="ppt-generate-header" :class="{ 'is-pre-outline': isPreOutlineSetup }">
            <button type="button" class="ppt-back-button" title="返回创建页" aria-label="返回创建页" @click="handlePptHomeClick">
              <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                <path d="m15 18-6-6 6-6" />
              </svg>
              <span>返回创建</span>
            </button>
            <div>
              <span>生成工作台</span>
              <h1>{{ workspaceTitle }}</h1>
              <p>{{ workspacePrompt }}</p>
            </div>
            <div class="ppt-generate-header-actions">
              <div class="ppt-generate-header-chips" aria-label="当前生成参数">
                <span>{{ store.slideCount }}张幻灯片</span>
                <span>{{ generationAspectRatioLabel }}</span>
                <span>{{ languageLabel }}</span>
                <span>{{ store.enableWebSearch ? "搜索" : "未联网" }}</span>
                <span>{{ currentTextModelLabel }}</span>
              </div>
              <span v-if="!isPreOutlineSetup" class="ppt-workspace-status" :class="`status-${store.status}`">{{ workspaceStatusLabel }}</span>
              <button
                type="button"
                class="ppt-generation-settings-toggle ppt-generation-regenerate-inline"
                :disabled="workspacePrimaryDisabled || !store.prompt.trim()"
                :title="hasOutline ? '重新生成大纲' : '生成大纲'"
                :aria-label="hasOutline ? '重新生成大纲' : '生成大纲'"
                @click="store.regenerateAllOutline"
              >
                <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                  <path d="M21 12a9 9 0 0 1-15.5 6.2" />
                  <path d="M3 12a9 9 0 0 1 15.5-6.2" />
                  <path d="M18 3v4h-4" />
                  <path d="M6 21v-4h4" />
                </svg>
                <span>{{ isPreOutlineSetup ? "再生" : hasOutline ? "重新生成" : "生成大纲" }}</span>
              </button>
              <button
                v-if="!isPreOutlineSetup"
                type="button"
                class="ppt-generation-settings-toggle"
                :title="isGenerationSettingsExpanded ? '收起生成参数' : '编辑生成参数'"
                :aria-label="isGenerationSettingsExpanded ? '收起生成参数' : '编辑生成参数'"
                :aria-expanded="isGenerationSettingsExpanded"
                @click="isGenerationSettingsExpanded = !isGenerationSettingsExpanded"
              >
                <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                  <path d="M12 20h9" />
                  <path d="M12 4h9" />
                  <path d="M4 9h16" />
                  <path d="M4 15h16" />
                  <circle cx="7" cy="4" r="2" />
                  <circle cx="17" cy="15" r="2" />
                </svg>
                <span>{{ isGenerationSettingsExpanded ? "收起参数" : "编辑参数" }}</span>
              </button>
            </div>
          </header>

          <PptErrorState
            v-if="store.error"
            class="ppt-workspace-error"
            :title="store.error.title"
            :message="store.error.message"
            @retry="store.retry"
          />

          <PptGenerationProgress
            v-if="showProgress"
            class="ppt-workspace-progress"
            :status="store.status"
            :progress="store.progress"
            :current-page="store.currentPage"
            :slide-count="store.slideCount"
            :status-text="store.statusText"
            :error-message="store.error?.message"
            @retry="store.retry"
          />

          <section v-if="!isPreOutlineSetup" class="ppt-generation-settings" aria-label="生成参数">
            <span class="ppt-setting-chip">
              <b>{{ store.slideCount }}</b>
              <small>张幻灯片</small>
            </span>
            <span class="ppt-setting-chip">
              <b>{{ languageLabel }}</b>
              <small>语言</small>
            </span>
            <span class="ppt-setting-chip">
              <b>{{ generationAspectRatioLabel }}</b>
              <small>格式</small>
            </span>
            <span class="ppt-setting-chip">
              <b>{{ themeLabel }}</b>
              <small>主题</small>
            </span>
            <span class="ppt-setting-chip">
              <b>{{ store.enableWebSearch ? "开启" : "关闭" }}</b>
              <small>联网搜索</small>
            </span>
            <span class="ppt-setting-chip">
              <b>{{ currentTextModelLabel }}</b>
              <small>模型</small>
            </span>
          </section>

          <section v-if="!isPreOutlineSetup && isGenerationSettingsExpanded" class="ppt-generation-settings-editor" :class="{ 'is-disabled': workspacePrimaryDisabled }">
            <label class="ppt-outline-prompt-field">
              <span>Prompt</span>
              <textarea
                v-model="store.prompt"
                rows="3"
                :disabled="workspacePrimaryDisabled"
                title="编辑生成提示词"
                aria-label="编辑生成提示词"
                placeholder="描述你想生成的演示文稿主题..."
              ></textarea>
            </label>
            <PptGenerationConfigPanel
              v-model:slide-count="store.slideCount"
              v-model:language="store.language"
              v-model:tone="store.tone"
              v-model:text-content="store.textContent"
              v-model:audience="store.audience"
              v-model:scenario="store.scenario"
              v-model:generation-aspect-ratio="store.generationAspectRatio"
              v-model:image-source="store.imageSource"
              v-model:text-model="store.textModel"
              v-model:image-model="store.imageModel"
              v-model:enable-web-search="store.enableWebSearch"
              :text-models="store.textModels"
              :image-models="store.imageModels"
            />
            <div class="ppt-generation-extra-settings">
              <label>
                <span>主题风格</span>
                <select
                  v-model="store.theme"
                  :disabled="workspacePrimaryDisabled"
                  title="选择主题风格"
                  aria-label="选择主题风格"
                >
                  <option v-for="option in pptThemeOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
                </select>
              </label>
              <button
                type="button"
                class="ppt-generation-switch"
                :class="{ active: store.autoThemeEnabled }"
                :disabled="workspacePrimaryDisabled"
                :title="store.autoThemeEnabled ? '关闭自动生成主题' : '开启自动生成主题'"
                :aria-label="store.autoThemeEnabled ? '关闭自动生成主题' : '开启自动生成主题'"
                :aria-pressed="store.autoThemeEnabled"
                @click="store.autoThemeEnabled = !store.autoThemeEnabled"
              >
                <span>自动生成主题</span>
                <b>{{ store.autoThemeEnabled ? "开启" : "关闭" }}</b>
              </button>
              <button
                type="button"
                class="ppt-generation-regenerate"
                :disabled="workspacePrimaryDisabled || !store.prompt.trim()"
                :title="hasOutline ? '重新生成大纲' : '生成大纲'"
                :aria-label="hasOutline ? '重新生成大纲' : '生成大纲'"
                @click="store.regenerateAllOutline"
              >
                <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                  <path d="M21 12a9 9 0 0 1-15.5 6.2" />
                  <path d="M3 12a9 9 0 0 1 15.5-6.2" />
                  <path d="M18 3v4h-4" />
                  <path d="M6 21v-4h4" />
                </svg>
                <span>{{ hasOutline ? "重新生成大纲" : "生成大纲" }}</span>
              </button>
            </div>
          </section>

          <template v-if="isPreOutlineSetup">
            <section class="ppt-pre-outline-panel ppt-pre-text-panel" aria-label="文本内容">
              <div class="ppt-pre-section-heading">
                <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                  <path d="M8 6h13" />
                  <path d="M8 12h13" />
                  <path d="M8 18h13" />
                  <path d="M3 6h.01" />
                  <path d="M3 12h.01" />
                  <path d="M3 18h.01" />
                </svg>
                <div>
                  <h2>文本内容</h2>
                  <p>每张卡片上的文字量</p>
                </div>
              </div>

              <div class="ppt-pre-text-content-grid" role="group" aria-label="每张卡片文字量">
                <button
                  v-for="option in textContentOptions"
                  :key="option.value"
                  type="button"
                  :class="{ active: store.textContent === option.value }"
                  :aria-pressed="store.textContent === option.value"
                  :title="`文字量：${option.label}`"
                  :aria-label="`文字量：${option.label}，${option.description}`"
                  @click="store.textContent = option.value"
                >
                  <span class="ppt-pre-text-lines" aria-hidden="true">
                    <i v-for="line in option.lines" :key="line" :style="{ width: line === option.lines ? '58%' : '82%' }"></i>
                  </span>
                  <strong>{{ option.label }}</strong>
                </button>
              </div>

              <div class="ppt-pre-select-grid">
                <label>
                  <span>语气</span>
                  <select v-model="store.tone" title="选择语气" aria-label="选择语气">
                    <option v-for="option in toneOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
                  </select>
                </label>
                <label>
                  <span>观众</span>
                  <select v-model="store.audience" title="选择观众" aria-label="选择观众">
                    <option v-for="option in audienceOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
                  </select>
                </label>
                <label>
                  <span>设想</span>
                  <select v-model="store.scenario" title="选择设想" aria-label="选择设想">
                    <option v-for="option in scenarioOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
                  </select>
                </label>
              </div>
            </section>

            <PptThemeSettingsPanel
              v-model="store.theme"
              class="ppt-pre-theme-panel"
              v-model:image-source="store.imageSource"
              v-model:image-model="store.imageModel"
              :image-models="store.imageModels"
              :disabled="workspacePrimaryDisabled"
            />
          </template>
          <template v-else>
            <section class="ppt-generation-steps" aria-label="生成阶段">
              <article v-for="step in workspaceGenerationSteps" :key="step.key" :class="`is-${step.state}`">
                <span class="ppt-generation-step-index">{{ step.index }}</span>
                <div>
                  <strong>{{ step.label }}</strong>
                  <small>{{ step.description }}</small>
                </div>
              </article>
            </section>

            <PptAgentActivityInline
              :activity-items="workspaceActivityItems"
              :is-running="isGenerationActivityRunning"
              :enable-web-search="store.enableWebSearch"
              :prompt="store.prompt"
              :uploaded-document-name="store.uploadedDocumentName"
              :default-expanded="isGenerationActivityRunning"
            />

            <section class="ppt-generation-flow">
              <article class="ppt-board-panel ppt-outline-panel">
                <div class="ppt-panel-heading">
                  <strong>演示大纲</strong>
                  <span>{{ store.outline ? "可编辑、保存并确认生成完整 PPT" : "正在准备可编辑大纲" }}</span>
                </div>
                <PptOutlineGenerator
                  v-if="!store.outline"
                  :can-generate="store.canGenerateOutline"
                  :status="store.status"
                  @generate="handleWorkspacePrimaryAction"
                />
                <PptOutlineEditor
                  v-if="store.outline"
                  :outline="store.outline"
                  :status="store.status"
                  @update-title="updateOutlineTitle"
                  @update-slide="store.updateOutlineSlide"
                  @add-slide="store.addOutlineSlide"
                  @delete-slide="store.deleteOutlineSlide"
                  @move-slide="store.moveOutlineSlide"
                  @regenerate-slide="store.regenerateOutlineSlide"
                  @regenerate-all="store.regenerateAllOutline"
                  @save="store.saveOutline"
                  @confirm="handleWorkspacePrimaryAction"
                />
              </article>

              <article
                v-if="store.slides.length || store.status === 'generating' || store.status === 'rendering' || store.status === 'success'"
                class="ppt-board-panel"
              >
                <div class="ppt-panel-heading">
                  <strong>结果预览</strong>
                  <span>{{ store.slides.length ? "生成结果会先在这里预览" : "确认大纲后生成幻灯片预览" }}</span>
                </div>
                <PptSlidePreview
                  v-if="store.slides.length"
                  :slides="store.slides"
                  :current-index="store.currentSlideIndex"
                  :theme="store.theme"
                  @select="store.selectSlide"
                  @prev="store.selectSlide(Math.max(0, store.currentSlideIndex - 1))"
                  @next="store.selectSlide(Math.min(store.slides.length - 1, store.currentSlideIndex + 1))"
                  @fullscreen="ElMessage.info('全屏预览入口已预留')"
                  @regenerate="store.regenerateCurrentSlide"
                />
                <PptEmptyState
                  v-else
                  title="等待生成预览"
                  description="确认大纲后会在这里展示幻灯片预览。"
                />
              </article>
            </section>

            <PptThemeSettingsPanel
              v-model="store.theme"
              v-model:image-source="store.imageSource"
              v-model:image-model="store.imageModel"
              :image-models="store.imageModels"
              :disabled="workspacePrimaryDisabled"
            />

            <section class="ppt-editor-board" v-if="store.slides.length && store.status === 'success'">
              <article class="ppt-board-panel">
                <PptSlideEditor
                  :slide="store.currentSlide"
                  @save="handleSlideSave"
                  @cancel="ElMessage.info('已取消本次编辑')"
                  @regenerate="store.regenerateCurrentSlide"
                />
              </article>
              <article class="ppt-board-panel">
                <PptImageSourcePanel
                  :image-source="store.imageSource"
                  :image-model="store.imageModel"
                  :image-models="store.imageModels"
                  :current-slide="store.currentSlide"
                  :results="store.imageSearchResults"
                  :generating="store.imageGenerating"
                  :visual-operation-status="store.visualOperationStatus"
                  @update:image-source="store.imageSource = $event"
                  @update:image-model="store.imageModel = $event"
                  @generate-image="store.generateImageForCurrentSlide"
                  @update-visual-plan="store.updateCurrentSlideVisualPlan"
                  @delete-visual="store.deleteCurrentSlideVisual"
                  @restore-visual="store.restoreCurrentSlideVisual"
                  @search-images="store.searchImages"
                  @apply-image="store.applyImageToCurrentSlide"
                />
              </article>
            </section>
          </template>

          <footer class="ppt-generation-bottom-bar">
            <button
              type="button"
              class="ppt-generation-primary"
              :disabled="workspacePrimaryDisabled"
              :aria-busy="workspacePrimaryDisabled && isBusy"
              :aria-label="workspacePrimaryButtonTitle"
              :title="workspacePrimaryButtonTitle"
              @click="handleWorkspacePrimaryAction"
            >
              <span v-if="workspacePrimaryDisabled && isBusy" class="ppt-spinner"></span>
              <svg v-else class="ppt-wand-icon" viewBox="0 0 24 24" aria-hidden="true">
                <path d="m21.64 3.64-1.28-1.28a1.21 1.21 0 0 0-1.72 0L2.36 18.64a1.21 1.21 0 0 0 0 1.72l1.28 1.28a1.2 1.2 0 0 0 1.72 0L21.64 5.36a1.2 1.2 0 0 0 0-1.72" />
                <path d="m14 7 3 3" />
                <path d="M5 6v4" />
                <path d="M19 14v4" />
                <path d="M10 2v2" />
                <path d="M7 8H3" />
                <path d="M21 16h-4" />
                <path d="M11 3H9" />
              </svg>
              <span>{{ workspacePrimaryLabel }}</span>
            </button>
          </footer>

          <div class="ppt-generation-help-control">
            <button
              type="button"
              title="帮助"
              aria-label="打开帮助菜单"
              :aria-expanded="showGenerationHelpMenu"
              aria-haspopup="menu"
              @click="toggleGenerationHelpMenu"
            >
              ?
            </button>
            <div v-if="showGenerationHelpMenu" class="ppt-help-menu" role="menu" aria-label="帮助菜单">
              <button type="button" role="menuitem" title="查看键盘快捷键" aria-label="查看键盘快捷键" @click="openGenerationKeyboardShortcuts">
                <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                  <rect x="3" y="5" width="18" height="14" rx="2" />
                  <path d="M7 9h.01M11 9h.01M15 9h.01M19 9h.01M7 13h10" />
                </svg>
                <span>键盘快捷键</span>
              </button>
              <button type="button" role="menuitem" title="打开帮助中心" aria-label="打开帮助中心" @click="openGenerationHelpCenter">
                <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                  <circle cx="12" cy="12" r="10" />
                  <path d="M9.1 9a3 3 0 1 1 5.8 1c-.5 1-1.5 1.5-2.2 2.1-.5.4-.7.8-.7 1.4" />
                  <path d="M12 17h.01" />
                </svg>
                <span>帮助中心</span>
              </button>
              <div class="ppt-menu-separator" />
                <p>知启云 AI PPT 生成</p>
            </div>
          </div>

          <div v-if="showGenerationKeyboardShortcutsDialog" class="ppt-export-dialog-overlay" role="presentation" @click.self="closeGenerationKeyboardShortcuts">
            <section class="ppt-help-dialog" role="dialog" aria-modal="true" aria-labelledby="ppt-generation-shortcuts-title" @keydown.esc.stop.prevent="closeGenerationKeyboardShortcuts">
              <header>
                <h2 id="ppt-generation-shortcuts-title">键盘快捷键</h2>
                <p>生成工作台常用操作。输入框聚焦时不会触发全局快捷键。</p>
              </header>
              <div class="ppt-shortcut-list">
                <span>生成 / 下一步</span><kbd>Ctrl + Enter</kbd>
                <span>保存大纲</span><kbd>Ctrl + S</kbd>
                <span>打开帮助菜单</span><kbd>?</kbd>
                <span>返回创建页</span><kbd>Esc</kbd>
              </div>
              <footer>
                <button type="button" class="ppt-dialog-primary" title="关闭键盘快捷键" aria-label="关闭键盘快捷键" @click="closeGenerationKeyboardShortcuts">知道了</button>
              </footer>
            </section>
          </div>
        </section>

        <section v-else ref="presentationWorkspaceRef" class="ppt-presentation-workspace" :class="{ 'is-presenting': presentationViewMode === 'present' }">
          <div
            v-if="presentationViewMode === 'present'"
            class="ppt-present-header"
            :class="{ visible: presentModeHeaderVisible }"
            aria-label="演示模式顶部栏"
          >
            <strong>{{ workspaceTitle }}</strong>
            <button
              type="button"
              :disabled="presentModeBusy"
              :aria-busy="presentModeBusy"
              :aria-label="presentModeBusy ? '正在退出演示' : '退出演示'"
              :title="presentModeBusy ? '正在退出演示，请稍候' : '退出演示'"
              @click="exitPresentationMode"
            >
              <span v-if="presentModeBusy" class="ppt-spinner"></span>
              <span>{{ presentModeBusy ? "正在退出..." : "退出演示" }}</span>
            </button>
          </div>
          <button
            v-if="presentationViewMode === 'present'"
            type="button"
            class="ppt-present-phone-top-hitarea"
            aria-label="显示演示退出控制"
            title="显示演示退出控制"
            @click="showPresentModeHeaderFromTap"
          />
          <header class="ppt-presentation-header">
            <div class="ppt-presentation-titlebar">
              <button type="button" class="ppt-header-icon-button is-brain-entry" title="回到 PPT 工作台" aria-label="回到 PPT 工作台" @click="handlePptHomeClick">
                <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                  <path d="M11.2 3.1c-1.4 0-2.6.7-3.3 1.8-.3-.1-.6-.1-.9-.1-1.8 0-3.2 1.4-3.2 3.2 0 .4.1.8.2 1.1-1 .7-1.7 1.9-1.7 3.2 0 1.7 1.1 3.2 2.6 3.7.1 1.8 1.6 3.2 3.4 3.2 1 0 1.9-.4 2.5-1.1.4.7 1.2 1.1 2.1 1.1h1.1l2.5 2.7c.4.4 1.1.1 1-.4l-.4-2.2c2.1-.5 3.7-2.2 3.7-4.2 0-.8-.2-1.5-.6-2.1.7-.7 1.1-1.7 1.1-2.8 0-2.1-1.6-3.8-3.7-4-.8-1.5-2.4-2.5-4.2-2.5-.7 0-1.3.1-1.9.4-.7-.7-1.7-1.1-2.8-1.1Z" />
                  <path d="M8.6 8.2c.9.5 1.9.5 2.8 0m1.4 2.3c-.7.8-.9 1.7-.5 2.8m3.9-4.7c-.7.3-1.1.8-1.3 1.5m-7.8 4.3c.8-.2 1.4-.7 1.8-1.4" />
                </svg>
              </button>
              <div class="ppt-presentation-menu">
                <button
                  type="button"
                  class="ppt-header-icon-button"
                  title="打开演示文稿菜单"
                  aria-label="打开演示文稿菜单"
                  :aria-expanded="showPresentationMenu"
                  aria-haspopup="menu"
                  @click="togglePresentationMenu"
                >
                  <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                    <path d="M4 12h16" />
                    <path d="M4 6h16" />
                    <path d="M4 18h16" />
                  </svg>
                </button>
                <div v-if="showPresentationMenu" class="ppt-presentation-menu-popover" role="menu" aria-label="演示文稿菜单" @click.stop @keydown.esc.stop.prevent="showPresentationMenu = false">
                  <p>文件</p>
                  <button
                    type="button"
                    role="menuitem"
                    :disabled="isPresentationActionBusy || presentModeBusy"
                    :title="isPresentationActionBusy || presentModeBusy ? '当前任务处理中，暂不能新建演示文稿' : '新建演示文稿'"
                    :aria-label="isPresentationActionBusy || presentModeBusy ? '当前任务处理中，暂不能新建演示文稿' : '新建演示文稿'"
                    @click="handleMenuCreateBlank"
                  >
                    <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                      <path d="M5 12h14" />
                      <path d="M12 5v14" />
                    </svg>
                    <span>新建演示文稿</span>
                  </button>
                  <button type="button" role="menuitem" title="重命名演示文稿" aria-label="重命名演示文稿" @click="focusPresentationTitle">
                    <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                      <path d="M12 20h9" />
                      <path d="M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4Z" />
                    </svg>
                    <span>重命名</span>
                  </button>
                  <button
                    type="button"
                    role="menuitem"
                    :disabled="!store.slides.length || isPresentationActionBusy"
                    :title="!store.slides.length ? '暂无可复制的幻灯片' : '复制当前演示文稿'"
                    :aria-label="!store.slides.length ? '暂无可复制的幻灯片' : '复制当前演示文稿'"
                    @click="duplicateCurrentPresentation"
                  >
                    <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                      <rect x="8" y="8" width="12" height="12" rx="2" />
                      <path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2" />
                    </svg>
                    <span>复制演示文稿</span>
                  </button>
                  <div class="ppt-menu-separator" />
                  <p>编辑</p>
                  <button
                    type="button"
                    role="menuitem"
                    :disabled="!canUndoPresentation"
                    title="撤销上一步编辑"
                    aria-label="撤销上一步编辑"
                    aria-keyshortcuts="Control+Z"
                    @click="handlePresentationMenuUndo"
                  >
                    <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                      <path d="M9 14 4 9l5-5" />
                      <path d="M4 9h10a6 6 0 0 1 0 12h-1" />
                    </svg>
                    <span>撤销</span>
                    <small>Ctrl+Z</small>
                  </button>
                  <button
                    type="button"
                    role="menuitem"
                    :disabled="!canRedoPresentation"
                    title="重做下一步编辑"
                    aria-label="重做下一步编辑"
                    aria-keyshortcuts="Control+Y"
                    @click="handlePresentationMenuRedo"
                  >
                    <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                      <path d="m15 14 5-5-5-5" />
                      <path d="M20 9H10a6 6 0 0 0 0 12h1" />
                    </svg>
                    <span>重做</span>
                    <small>Ctrl+Y</small>
                  </button>
                  <div class="ppt-menu-separator" />
                  <p>工作区</p>
                  <button type="button" role="menuitem" title="打开页面设置" aria-label="打开页面设置" @click="openGlobalSettingsFromMenu">
                    <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                      <path d="M12 15.5A3.5 3.5 0 1 0 12 8a3.5 3.5 0 0 0 0 7.5Z" />
                      <path d="M19.4 15a1.7 1.7 0 0 0 .34 1.88l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.7 1.7 0 0 0-1.88-.34 1.7 1.7 0 0 0-1 1.55V21a2 2 0 1 1-4 0v-.08a1.7 1.7 0 0 0-1-1.55 1.7 1.7 0 0 0-1.88.34l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.7 1.7 0 0 0 4.6 15a1.7 1.7 0 0 0-1.55-1H3a2 2 0 1 1 0-4h.08a1.7 1.7 0 0 0 1.55-1 1.7 1.7 0 0 0-.34-1.88l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.7 1.7 0 0 0 9 4.6 1.7 1.7 0 0 0 10 3.05V3a2 2 0 1 1 4 0v.08a1.7 1.7 0 0 0 1 1.55 1.7 1.7 0 0 0 1.88-.34l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.7 1.7 0 0 0 19.4 9c.2.6.78 1 1.55 1H21a2 2 0 1 1 0 4h-.08a1.7 1.7 0 0 0-1.55 1Z" />
                    </svg>
                    <span>页面设置</span>
                  </button>
                  <button type="button" role="menuitem" title="打开主题面板" aria-label="打开主题面板" @click="openThemePanelFromMenu">
                    <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                      <circle cx="13.5" cy="6.5" r=".5" />
                      <circle cx="17.5" cy="10.5" r=".5" />
                      <path d="M12 2C6.5 2 2 6 2 11c0 5.5 4.5 10 10 10h1.5a2.5 2.5 0 0 0 0-5H12a2 2 0 0 1 0-4h2a8 8 0 0 0 8-8c0-1.1-.9-2-2-2z" />
                    </svg>
                    <span>主题面板</span>
                  </button>
                  <button type="button" role="menuitem" title="打开分享设置" aria-label="打开分享设置" @click="openShareDialogFromMenu">
                    <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                      <circle cx="18" cy="5" r="3" />
                      <circle cx="6" cy="12" r="3" />
                      <circle cx="18" cy="19" r="3" />
                      <path d="m8.6 13.5 6.8 4" />
                      <path d="m15.4 6.5-6.8 4" />
                    </svg>
                    <span>分享设置</span>
                  </button>
                  <div class="ppt-menu-separator" />
                  <p>视图</p>
                  <button type="button" role="menuitem" title="返回提示词与大纲页" aria-label="返回提示词与大纲页" @click="handlePresentationBackToOutline">
                    <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                      <path d="M4 4h16v16H4z" />
                      <path d="M8 8h8" />
                      <path d="M8 12h8" />
                      <path d="M8 16h5" />
                    </svg>
                    <span>返回提示词</span>
                  </button>
                  <button type="button" role="menuitem" title="查看全部演示文稿" aria-label="查看全部演示文稿" @click="handlePptHomeClick">
                    <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                      <path d="M3 9h18" />
                      <path d="M4 9v11h16V9" />
                      <path d="M10 13h4" />
                    </svg>
                    <span>全部演示文稿</span>
                  </button>
                </div>
              </div>
              <input
                id="presentation-title-input"
                ref="presentationTitleInputRef"
                :value="workspaceTitle"
                title="演示文稿标题"
                aria-label="演示文稿标题"
                @input="handlePresentationTitleInput"
              />
              <span
                v-if="presentationSaveStatus !== 'idle'"
                class="ppt-saving-indicator"
                :class="`is-${presentationSaveStatus}`"
                aria-live="polite"
                :title="presentationSaveStatusLabel"
              >
                <span v-if="presentationSaveStatus === 'saving'" class="ppt-spinner"></span>
                <svg v-else-if="presentationSaveStatus === 'saved'" class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                  <path d="M20 6 9 17l-5-5" />
                </svg>
                <svg v-else class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                  <circle cx="12" cy="12" r="10" />
                  <path d="M12 8v5" />
                  <path d="M12 17h.01" />
                </svg>
                {{ presentationSaveStatusLabel }}
              </span>
              <div class="ppt-history-buttons" aria-label="编辑历史">
                <button type="button" title="撤销 (Ctrl+Z)" aria-label="撤销" aria-keyshortcuts="Control+Z" :disabled="!canUndoPresentation" @click="undoPresentationEdit">
                  <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                    <path d="M9 14 4 9l5-5" />
                    <path d="M4 9h10a6 6 0 0 1 0 12h-1" />
                  </svg>
                </button>
                <button type="button" title="重做 (Ctrl+Y)" aria-label="重做" aria-keyshortcuts="Control+Y" :disabled="!canRedoPresentation" @click="redoPresentationEdit">
                  <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                    <path d="m15 14 5-5-5-5" />
                    <path d="M20 9H10a6 6 0 0 0 0 12h1" />
                  </svg>
                </button>
              </div>
            </div>

            <div class="ppt-presentation-actions">
              <button
                type="button"
                class="ppt-presentation-action"
                :class="{ active: presentationRightPanelOpen && presentationRightPanel === 'theme' }"
                :aria-pressed="presentationRightPanelOpen && presentationRightPanel === 'theme'"
                title="打开主题面板"
                aria-label="打开主题面板"
                @click="selectPresentationRightPanel('theme')"
              >
                <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                  <circle cx="13.5" cy="6.5" r=".5" />
                  <circle cx="17.5" cy="10.5" r=".5" />
                  <circle cx="8.5" cy="7.5" r=".5" />
                  <circle cx="6.5" cy="12.5" r=".5" />
                  <path d="M12 2C6.5 2 2 6 2 11c0 5.5 4.5 10 10 10h1.5a2.5 2.5 0 0 0 0-5H12a2 2 0 0 1 0-4h2a8 8 0 0 0 8-8c0-1.1-.9-2-2-2z" />
                </svg>
                <span>主题</span>
              </button>
              <div class="ppt-presentation-export">
                <button
                  type="button"
                  class="ppt-presentation-action"
                  :disabled="!store.slides.length || exportBusy || isPresentationActionBusy"
                  :aria-expanded="showExportDialog"
                  :aria-label="exportButtonLabel"
                  :title="exportButtonLabel"
                  aria-haspopup="dialog"
                  @click="openExportDialog"
                >
                  <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                    <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
                    <path d="M7 10l5 5 5-5" />
                    <path d="M12 15V3" />
                  </svg>
                  <span>导出</span>
                </button>
              </div>
              <button
                type="button"
                class="ppt-presentation-action"
                :disabled="!activePresentationId && !store.taskId"
                :aria-expanded="showShareDialog"
                :aria-label="shareButtonLabel"
                :title="shareButtonLabel"
                aria-haspopup="dialog"
                @click="openShareDialog"
              >
                <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                  <circle cx="18" cy="5" r="3" />
                  <circle cx="6" cy="12" r="3" />
                  <circle cx="18" cy="19" r="3" />
                  <path d="m8.6 13.5 6.8 4" />
                  <path d="m15.4 6.5-6.8 4" />
                </svg>
                <span>分享</span>
              </button>
              <button
                type="button"
                class="ppt-presentation-action is-primary"
                :disabled="presentModeBusy || isPresentationActionBusy"
                :aria-busy="presentModeBusy"
                :aria-label="presentButtonLabel"
                :title="presentButtonLabel"
                @click="togglePresentationMode"
              >
                <span v-if="presentModeBusy" class="ppt-spinner"></span>
                <svg v-else class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                  <template v-if="presentationViewMode === 'present'">
                    <path d="M18 6 6 18" />
                    <path d="m6 6 12 12" />
                  </template>
                  <template v-else>
                    <polygon points="6 4 20 12 6 20 6 4" />
                  </template>
                </svg>
                <span>{{ presentButtonLabel }}</span>
              </button>
            </div>
          </header>

          <PptGenerationProgress
            v-if="showProgress"
            class="ppt-presentation-progress"
            :status="store.status"
            :progress="store.progress"
            :current-page="store.currentPage"
            :slide-count="store.slideCount"
            :status-text="store.statusText"
            :error-message="store.error?.message"
            @retry="store.retry"
          />

          <section
            v-if="store.slides.length"
            class="ppt-presentation-editor"
            :class="{ 'is-sidebar-collapsed': presentationSidebarCollapsed, 'is-sidebar-resizing': presentationSidebarResizing }"
            :style="presentationEditorStyle"
          >
            <div class="ppt-slide-sidebar-shell" aria-label="幻灯片列表">
              <button
                v-if="presentationSidebarCollapsed"
                type="button"
                class="ppt-slide-sidebar-expand"
                title="展开幻灯片列表"
                aria-label="展开幻灯片列表"
                aria-controls="ppt-slide-sidebar-panel"
                aria-expanded="false"
                @click="setPresentationSidebarCollapsed(false)"
              >
                <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                  <path d="m9 18 6-6-6-6" />
                  <path d="M4 5h4" />
                  <path d="M4 12h4" />
                  <path d="M4 19h4" />
                </svg>
              </button>

              <aside v-else id="ppt-slide-sidebar-panel" class="ppt-slide-sidebar" aria-label="幻灯片列表">
                <header class="ppt-slide-sidebar-header">
                  <div>
                    <strong>幻灯片</strong>
                    <span>{{ store.slides.length }} 张</span>
                  </div>
                  <button
                    type="button"
                    class="ppt-slide-sidebar-collapse"
                    title="收起幻灯片列表"
                    aria-label="收起幻灯片列表"
                    aria-controls="ppt-slide-sidebar-panel"
                    aria-expanded="true"
                    @click="setPresentationSidebarCollapsed(true)"
                  >
                    <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                      <path d="m15 18-6-6 6-6" />
                      <path d="M20 5h-4" />
                      <path d="M20 12h-4" />
                      <path d="M20 19h-4" />
                    </svg>
                  </button>
                </header>

                <div class="ppt-slide-sidebar-list" role="listbox" aria-label="幻灯片缩略图">
                  <button
                    v-for="(slide, index) in store.slides"
                    :key="slide.id"
                    type="button"
                    role="option"
                    class="ppt-slide-thumbnail-card"
                    :class="{ active: store.currentSlideIndex === index }"
                    :aria-label="`第 ${index + 1} 页：${presentationSlideTitle(slide)}`"
                    :aria-current="store.currentSlideIndex === index ? 'page' : undefined"
                    :aria-selected="store.currentSlideIndex === index"
                    :aria-posinset="index + 1"
                    :aria-setsize="store.slides.length"
                    :title="presentationSlideTitle(slide)"
                    :data-ppt-sidebar-slide-index="index"
                    @click="selectPresentationSlide(index)"
                  >
                    <span class="ppt-slide-number">{{ index + 1 }}</span>
                    <div class="ppt-slide-thumb-stage" :class="[`layout-${slide.layout}`, `theme-${store.theme}`, { 'has-image': Boolean(slide.imageUrl) }]" :style="presentationSlideStyle(slide)">
                      <strong>{{ presentationSlideTitle(slide) }}</strong>
                      <p>{{ slide.content }}</p>
                      <img v-if="slide.imageUrl" :src="slide.imageUrl" alt="" loading="lazy" />
                    </div>
                  </button>
                </div>
              </aside>

              <div
                v-if="!presentationSidebarCollapsed"
                class="ppt-slide-sidebar-resize"
                role="separator"
                aria-orientation="vertical"
                tabindex="0"
                aria-label="调整幻灯片缩略栏宽度"
                aria-valuemin="100"
                aria-valuemax="300"
                :aria-valuenow="presentationSidebarWidth"
                :aria-valuetext="`当前宽度 ${presentationSidebarWidth} 像素`"
                :title="`拖拽或使用方向键调整缩略栏宽度，当前 ${presentationSidebarWidth} 像素`"
                @pointerdown="startSidebarResize"
                @keydown.left.prevent="setPresentationSidebarWidth(presentationSidebarWidth - 10)"
                @keydown.right.prevent="setPresentationSidebarWidth(presentationSidebarWidth + 10)"
                @keydown.home.prevent="setPresentationSidebarWidth(100)"
                @keydown.end.prevent="setPresentationSidebarWidth(300)"
              >
                <span></span>
              </div>
            </div>

            <main class="ppt-presentation-stage" :style="presentationStageStyle">
              <div v-if="presentationViewMode === 'present'" class="ppt-present-mode">
                <button
                  type="button"
                  class="ppt-present-nav is-prev"
                  :disabled="store.currentSlideIndex <= 0"
                  :title="store.currentSlideIndex <= 0 ? '已经是第一页' : '上一页'"
                  :aria-label="store.currentSlideIndex <= 0 ? '已经是第一页' : '上一页'"
                  @click="previousPresentationSlide"
                >
                  <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                    <path d="m15 18-6-6 6-6" />
                  </svg>
                </button>
                <article v-if="store.currentSlide" class="ppt-present-canvas" :class="[`theme-${store.theme}`, { 'has-image': Boolean(store.currentSlide.imageUrl) }]" :style="presentationSlideStyle(store.currentSlide)" :aria-label="`${activeSlideLabel}：${presentationSlideTitle(store.currentSlide)}`">
                  <span>{{ activeSlideLabel }}</span>
                  <h2>{{ presentationSlideTitle(store.currentSlide) }}</h2>
                  <p>{{ store.currentSlide.content }}</p>
                  <ul>
                    <li v-for="point in store.currentSlide.bulletPoints" :key="point">{{ point }}</li>
                  </ul>
                  <figure v-if="store.currentSlide.imageUrl" class="ppt-slide-image-frame">
                    <img :src="store.currentSlide.imageUrl" :alt="`${store.currentSlide.title} 配图`" />
                  </figure>
                </article>
                <button
                  type="button"
                  class="ppt-present-nav is-next"
                  :disabled="store.currentSlideIndex >= store.slides.length - 1"
                  :title="store.currentSlideIndex >= store.slides.length - 1 ? '已经是最后一页' : '下一页'"
                  :aria-label="store.currentSlideIndex >= store.slides.length - 1 ? '已经是最后一页' : '下一页'"
                  @click="nextPresentationSlide"
                >
                  <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                    <path d="m9 18 6-6-6-6" />
                  </svg>
                </button>
                <div v-if="recordingWantsToRecord" class="ppt-recording-status-bar">
                  <span>录制准备中</span>
                  <small>{{ recordingOptionSummary }}</small>
                </div>
                <div
                  v-if="store.slides.length > 1"
                  class="ppt-present-phone-overlay"
                  role="application"
                  aria-label="移动端演示导航覆盖层"
                  @pointerdown="handlePresentPhonePointerDown"
                  @pointerup="handlePresentPhonePointerUp"
                  @pointercancel="handlePresentPhonePointerCancel"
                  title="点击左半区上一页，右半区下一页；左右滑动也可翻页"
                >
                  <div class="is-left" aria-hidden="true"></div>
                  <div class="is-right" aria-hidden="true"></div>
                </div>
                <div class="ppt-present-progress-bar" role="navigation" aria-label="演示进度">
                  <button
                    v-for="(slide, index) in store.slides"
                    :key="slide.id"
                    type="button"
                    :class="{ active: store.currentSlideIndex === index }"
                    :aria-current="store.currentSlideIndex === index ? 'step' : undefined"
                    :aria-pressed="store.currentSlideIndex === index"
                    :aria-label="`跳转到第 ${index + 1} 页`"
                    :title="`第 ${index + 1} 页：${presentationSlideTitle(slide)}`"
                    :data-ppt-present-progress-index="index"
                    @click="selectPresentationProgressSlide(index)"
                  />
                </div>
              </div>

              <div v-else class="ppt-slide-stack">
                <article
                  v-for="(slide, index) in store.slides"
                  :key="slide.id"
                  class="ppt-edit-slide"
                  :class="[`layout-${slide.layout}`, `theme-${store.theme}`, { active: store.currentSlideIndex === index, 'has-image': Boolean(slide.imageUrl) }]"
                  :style="presentationSlideStyle(slide)"
                  :data-ppt-slide-index="index"
                  @click="selectPresentationSlideOnly(index)"
                >
                  <button
                    v-if="index === 0"
                    type="button"
                    class="ppt-slide-insert-button is-before"
                    :disabled="isPresentationActionBusy"
                    :title="isPresentationActionBusy ? '正在生成中，暂不可插入幻灯片' : '在前方插入幻灯片'"
                    :aria-label="isPresentationActionBusy ? '正在生成中，暂不可插入幻灯片' : '在前方插入幻灯片'"
                    :aria-busy="isPresentationActionBusy ? 'true' : undefined"
                    @click.stop="insertSlideAt(index, 'before')"
                  >
                    <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                      <path d="M5 12h14" />
                      <path d="M12 5v14" />
                    </svg>
                  </button>
                  <span>{{ index + 1 }} / {{ store.slides.length }}</span>
                  <h2
                    class="ppt-slide-element"
                    :class="{ selected: isPresentationElementSelected(index, 'title') }"
                    :style="presentationElementStyle(slide.id, 'title')"
                    tabindex="0"
                    role="button"
                    :aria-pressed="isPresentationElementSelected(index, 'title')"
                    :aria-label="`选择第 ${index + 1} 页标题元素`"
                    :title="`选择标题：${presentationSlideTitle(slide)}`"
                    @click.stop="selectPresentationElement(index, 'title')"
                    @keydown="handlePresentationElementKeydown($event, index, 'title')"
                  >
                    {{ presentationSlideTitle(slide) }}
                  </h2>
                  <p
                    class="ppt-slide-element"
                    :class="{ selected: isPresentationElementSelected(index, 'content') }"
                    :style="presentationElementStyle(slide.id, 'content')"
                    tabindex="0"
                    role="button"
                    :aria-pressed="isPresentationElementSelected(index, 'content')"
                    :aria-label="`选择第 ${index + 1} 页正文元素`"
                    title="选择正文元素"
                    @click.stop="selectPresentationElement(index, 'content')"
                    @keydown="handlePresentationElementKeydown($event, index, 'content')"
                  >
                    {{ slide.content }}
                  </p>
                  <ul
                    class="ppt-slide-element"
                    :class="{ selected: isPresentationElementSelected(index, 'bullets') }"
                    :style="presentationElementStyle(slide.id, 'bullets')"
                    tabindex="0"
                    role="button"
                    :aria-pressed="isPresentationElementSelected(index, 'bullets')"
                    :aria-label="`选择第 ${index + 1} 页要点元素`"
                    title="选择要点元素"
                    @click.stop="selectPresentationElement(index, 'bullets')"
                    @keydown="handlePresentationElementKeydown($event, index, 'bullets')"
                  >
                    <li v-for="point in slide.bulletPoints" :key="point">{{ point }}</li>
                  </ul>
                  <em
                    v-if="slide.speakerNotes"
                    class="ppt-slide-element ppt-slide-notes"
                    :class="{ selected: isPresentationElementSelected(index, 'notes') }"
                    :style="presentationElementStyle(slide.id, 'notes')"
                    tabindex="0"
                    role="button"
                    :aria-pressed="isPresentationElementSelected(index, 'notes')"
                    :aria-label="`选择第 ${index + 1} 页讲稿元素`"
                    title="选择讲稿元素"
                    @click.stop="selectPresentationElement(index, 'notes')"
                    @keydown="handlePresentationElementKeydown($event, index, 'notes')"
                  >
                    {{ slide.speakerNotes }}
                  </em>
                  <figure v-if="slide.imageUrl" class="ppt-slide-image-frame">
                    <img :src="slide.imageUrl" :alt="`${slide.title} 配图`" loading="lazy" />
                  </figure>
                  <div
                    v-if="isPresentationElementToolbarVisible(index)"
                    class="ppt-element-floating-toolbar"
                    role="toolbar"
                    :aria-label="`第 ${index + 1} 页 ${selectedPresentationElementLabel} 工具栏`"
                    @click.stop
                  >
                    <div class="ppt-element-type-menu">
                      <button
                        type="button"
                        class="ppt-element-toolbar-chip"
                        :title="`切换元素类型，当前为${selectedPresentationElementLabel}`"
                        :aria-label="`切换元素类型，当前为${selectedPresentationElementLabel}`"
                        :aria-expanded="showElementTypeMenu"
                        aria-haspopup="menu"
                        :aria-pressed="showElementTypeMenu"
                        @click="toggleElementTypeMenu"
                      >
                        <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                          <path d="M4 7h16" />
                          <path d="M7 12h10" />
                          <path d="M9 17h6" />
                        </svg>
                        <span>{{ selectedPresentationElementLabel }}</span>
                      </button>
                      <div v-if="showElementTypeMenu" class="ppt-element-type-popover" role="menu" aria-label="切换元素类型" @keydown.esc.stop.prevent="showElementTypeMenu = false">
                        <button
                          v-for="item in presentationElementKinds"
                          :key="item.value"
                          type="button"
                          role="menuitemradio"
                          :aria-checked="selectedPresentationElement?.kind === item.value"
                          :title="`切换为${item.label}`"
                          :aria-label="`切换为${item.label}：${item.hint}`"
                          :class="{ active: selectedPresentationElement?.kind === item.value }"
                          @click="selectPresentationElementKind(item.value)"
                        >
                          <span>{{ item.label }}</span>
                          <small>{{ item.hint }}</small>
                        </button>
                      </div>
                    </div>
                    <button type="button" title="切换当前页布局" aria-label="切换当前页布局" @click="cycleSelectedSlideLayout">
                      <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                        <rect x="3" y="3" width="18" height="7" rx="1" />
                        <rect x="3" y="14" width="8" height="7" rx="1" />
                        <rect x="15" y="14" width="6" height="7" rx="1" />
                      </svg>
                    </button>
                    <div class="ppt-element-toolbar-group" aria-label="对齐">
                      <button
                        v-for="align in presentationElementAlignments"
                        :key="align.value"
                        type="button"
                        :title="align.label"
                        :aria-label="align.label"
                        :aria-pressed="selectedPresentationElementStyle?.align === align.value"
                        :class="{ active: selectedPresentationElementStyle?.align === align.value }"
                        @click="setPresentationElementAlignment(align.value)"
                      >
                        <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                          <template v-if="align.value === 'left'">
                            <path d="M4 6h14" />
                            <path d="M4 12h10" />
                            <path d="M4 18h16" />
                          </template>
                          <template v-else-if="align.value === 'center'">
                            <path d="M5 6h14" />
                            <path d="M8 12h8" />
                            <path d="M4 18h16" />
                          </template>
                          <template v-else>
                            <path d="M6 6h14" />
                            <path d="M10 12h10" />
                            <path d="M4 18h16" />
                          </template>
                        </svg>
                      </button>
                    </div>
                    <div class="ppt-element-color-row" aria-label="颜色">
                      <button
                        v-for="color in presentationElementColors"
                        :key="color.value"
                        type="button"
                        :title="`文字颜色：${color.label}`"
                        :aria-label="`文字颜色：${color.label}`"
                        :aria-pressed="selectedPresentationElementStyle?.color === color.value"
                        :class="{ active: selectedPresentationElementStyle?.color === color.value }"
                        :style="{ '--element-color': color.value }"
                        @click="setPresentationElementColor(color.value)"
                      />
                    </div>
                    <button
                      type="button"
                      title="强调"
                      aria-label="强调"
                      :aria-pressed="Boolean(selectedPresentationElementStyle?.emphasis)"
                      :class="{ active: selectedPresentationElementStyle?.emphasis }"
                      @click="togglePresentationElementEmphasis"
                    >
                      <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                        <path d="M7 5h6a4 4 0 0 1 0 8H7z" />
                        <path d="M7 13h7a3 3 0 0 1 0 6H7z" />
                      </svg>
                    </button>
                    <button
                      type="button"
                      title="AI 编辑元素"
                      aria-label="AI 编辑元素"
                      aria-controls="ppt-element-ai-editor"
                      :aria-expanded="showElementAiEditor"
                      :aria-pressed="showElementAiEditor"
                      :class="{ active: showElementAiEditor }"
                      @click="openSelectedElementAiEditor"
                    >
                      <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                        <path d="M9.9 2.8 8.8 6.2 5.4 7.3l3.4 1.1 1.1 3.4 1.1-3.4 3.4-1.1-3.4-1.1z" />
                        <path d="m18 10 1 3 3 1-3 1-1 3-1-3-3-1 3-1z" />
                      </svg>
                    </button>
                    <div
                      v-if="showElementAiEditor"
                      id="ppt-element-ai-editor"
                      class="ppt-element-ai-editor"
                      role="dialog"
                      aria-modal="false"
                      aria-label="AI 编辑选中元素"
                      :aria-busy="elementAiBusy"
                      @keydown.esc.stop.prevent="closeSelectedElementAiEditor"
                    >
                      <div class="ppt-element-ai-head">
                        <div>
                          <strong>AI 编辑</strong>
                          <small>{{ selectedPresentationElementLabel }}</small>
                        </div>
                        <button
                          type="button"
                          title="关闭 AI 编辑"
                          aria-label="关闭 AI 编辑"
                          :disabled="elementAiBusy"
                          @click="closeSelectedElementAiEditor"
                        >
                          <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                            <path d="M6 6l12 12" />
                            <path d="M18 6 6 18" />
                          </svg>
                        </button>
                      </div>
                      <div class="ppt-element-ai-input-wrap">
                        <textarea
                          ref="elementAiPromptInputRef"
                          v-model="elementAiPrompt"
                          rows="3"
                          placeholder="想让 AI 怎么修改选中内容?"
                          title="AI 编辑要求"
                          aria-label="AI 编辑要求"
                          :disabled="elementAiBusy"
                          @keydown.enter.exact.prevent="submitSelectedElementAiEdit()"
                        ></textarea>
                        <div class="ppt-element-ai-footer">
                          <span>Enter 发送 · Esc 关闭</span>
                          <button
                            type="button"
                            class="ppt-element-ai-send"
                            :disabled="elementAiBusy || !elementAiPrompt.trim()"
                            :aria-busy="elementAiBusy"
                            :title="elementAiBusy ? '正在编辑选中元素' : '发送 AI 编辑要求'"
                            :aria-label="elementAiBusy ? '正在编辑选中元素' : '发送 AI 编辑要求'"
                            @click="submitSelectedElementAiEdit()"
                          >
                            <span v-if="elementAiBusy" class="ppt-spinner"></span>
                            <svg v-else class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                              <path d="M5 12h14" />
                              <path d="m13 6 6 6-6 6" />
                            </svg>
                          </button>
                        </div>
                      </div>
                      <div v-if="elementAiBusy" class="ppt-element-ai-state" role="status" aria-live="polite">
                        <span class="ppt-spinner"></span>
                        <span>正在编辑选中内容...</span>
                        <button type="button" title="取消 AI 编辑" aria-label="取消 AI 编辑" @click="cancelSelectedElementAiEdit">取消</button>
                      </div>
                      <p v-if="elementAiError" class="ppt-element-ai-error" role="alert">{{ elementAiError }}</p>
                      <div v-if="!elementAiBusy" class="ppt-element-ai-suggestions" aria-label="AI 编辑快捷建议">
                        <button
                          v-for="suggestion in elementAiQuickSuggestions"
                          :key="suggestion.label"
                          type="button"
                          :title="`使用建议：${suggestion.label}`"
                          :aria-label="`使用建议：${suggestion.label}`"
                          @click="useElementAiQuickSuggestion(suggestion.prompt)"
                        >
                          <span>{{ suggestion.icon }}</span>
                          <small>{{ suggestion.label }}</small>
                        </button>
                      </div>
                    </div>
                    <button type="button" :title="`复制${selectedPresentationElementLabel}`" :aria-label="`复制${selectedPresentationElementLabel}`" @click="duplicateSelectedPresentationElement">
                      <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                        <rect x="8" y="8" width="12" height="12" rx="2" />
                        <path d="M4 16V4h12" />
                      </svg>
                    </button>
                    <button type="button" :title="`删除${selectedPresentationElementLabel}`" :aria-label="`删除${selectedPresentationElementLabel}`" class="danger" @click="deleteSelectedPresentationElement">
                      <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                        <path d="M3 6h18" />
                        <path d="M8 6V4h8v2" />
                        <path d="M19 6l-1 14H6L5 6" />
                      </svg>
                    </button>
                  </div>
                <div v-if="!isPresentationElementToolbarVisible(index)" class="ppt-slide-floating-tools">
                  <div class="ppt-slide-more">
                    <button
                      type="button"
                      :title="`第 ${index + 1} 页更多操作`"
                      :aria-label="`第 ${index + 1} 页更多操作`"
                      :aria-expanded="slideMoreMenuIndex === index"
                      aria-haspopup="menu"
                      @click.stop="toggleSlideMoreMenu(index)"
                    >
                      <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                        <circle cx="12" cy="12" r="1" />
                        <circle cx="12" cy="5" r="1" />
                        <circle cx="12" cy="19" r="1" />
                      </svg>
                    </button>
                    <div
                      v-if="slideMoreMenuIndex === index"
                      class="ppt-slide-more-menu"
                      role="menu"
                      :aria-label="`第 ${index + 1} 页更多操作`"
                      @click.stop
                      @keydown.esc.stop.prevent="slideMoreMenuIndex = null"
                    >
                      <button
                        type="button"
                        role="menuitem"
                        :title="`复制第 ${index + 1} 页`"
                        :aria-label="`复制第 ${index + 1} 页幻灯片`"
                        @click="duplicateSlideAt(index)"
                      >
                        <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                          <rect x="8" y="8" width="12" height="12" rx="2" />
                          <path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2" />
                        </svg>
                        <span>复制幻灯片</span>
                      </button>
                      <div class="ppt-menu-separator" />
                      <button
                        type="button"
                        role="menuitem"
                        class="danger"
                        :disabled="store.slides.length <= 1"
                        :title="store.slides.length <= 1 ? '至少保留一页幻灯片' : `删除第 ${index + 1} 页`"
                        :aria-label="store.slides.length <= 1 ? '至少保留一页幻灯片' : `删除第 ${index + 1} 页幻灯片`"
                        @click="deleteSlideAt(index)"
                      >
                        <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                          <path d="M3 6h18" />
                          <path d="M8 6V4h8v2" />
                          <path d="M19 6l-1 14H6L5 6" />
                        </svg>
                        <span>删除</span>
                      </button>
                    </div>
                  </div>
                  <div class="ppt-slide-palette">
                    <button
                      type="button"
                      :title="`第 ${index + 1} 页主题与布局`"
                      :aria-label="`第 ${index + 1} 页主题与布局`"
                      :aria-expanded="slidePaletteMenuIndex === index"
                      aria-haspopup="menu"
                      @click.stop="toggleSlidePaletteMenu(index)"
                    >
                      <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                        <circle cx="13.5" cy="6.5" r=".5" />
                        <circle cx="17.5" cy="10.5" r=".5" />
                        <path d="M12 2C6.5 2 2 6 2 11c0 5.5 4.5 10 10 10h1.5a2.5 2.5 0 0 0 0-5H12a2 2 0 0 1 0-4h2a8 8 0 0 0 8-8c0-1.1-.9-2-2-2z" />
                      </svg>
                    </button>
                    <div
                      v-if="slidePaletteMenuIndex === index"
                      class="ppt-slide-palette-menu"
                      role="menu"
                      :aria-label="`第 ${index + 1} 页主题与布局`"
                      @click.stop
                      @keydown.esc.stop.prevent="slidePaletteMenuIndex = null"
                    >
                      <section>
                        <h4>布局</h4>
                        <div class="ppt-slide-palette-layouts">
                          <button
                            v-for="layout in presentationLayoutOptions"
                            :key="layout.value"
                            type="button"
                            role="menuitemradio"
                            :aria-checked="slide.layout === layout.value"
                            :title="`切换为${layout.label}布局`"
                            :aria-label="`切换为${layout.label}布局`"
                            :class="{ active: slide.layout === layout.value }"
                            @click="applySlidePaletteLayout(index, layout.value)"
                          >
                            <span :data-layout="layout.value"></span>
                          </button>
                        </div>
                      </section>
                      <section>
                        <h4>页面颜色</h4>
                        <div class="ppt-slide-palette-colors">
                          <button
                            v-for="item in slidePaletteBackgroundOptions"
                            :key="item.value"
                            type="button"
                            role="menuitemradio"
                            :title="item.label"
                            :aria-label="`应用${item.label}背景`"
                            :aria-checked="slideBackgroundFor(slide) === item.value"
                            :class="{ active: slideBackgroundFor(slide) === item.value }"
                            :style="{ '--palette-bg': item.value }"
                            @click="applySlidePaletteBackground(index, item.value)"
                          />
                          <button
                            type="button"
                            class="is-reset"
                            role="menuitemradio"
                            title="重置背景"
                            aria-label="重置背景"
                            :aria-checked="!slideBackgroundFor(slide)"
                            :class="{ active: !slideBackgroundFor(slide) }"
                            @click="resetSlidePaletteBackground(index)"
                          >
                            ×
                          </button>
                        </div>
                      </section>
                      <section>
                        <h4>内容对齐</h4>
                        <div class="ppt-slide-palette-segment">
                          <button
                            v-for="align in presentationElementAlignments"
                            :key="align.value"
                            type="button"
                            role="menuitemradio"
                            :title="`内容${align.label}`"
                            :aria-label="`内容${align.label}`"
                            :aria-checked="presentationGlobalAlignment === align.value"
                            :class="{ active: presentationGlobalAlignment === align.value }"
                            @click="applySlidePaletteAlignment(align.value)"
                          >
                            {{ align.label.replace("对齐", "") }}
                          </button>
                        </div>
                      </section>
                      <section>
                        <h4>页面宽度</h4>
                        <div class="ppt-slide-palette-segment">
                          <button
                            v-for="width in presentationDeckWidthOptions"
                            :key="width.value"
                            type="button"
                            role="menuitemradio"
                            :title="`页面宽度：${width.label}`"
                            :aria-label="`切换页面宽度为${width.label}`"
                            :aria-checked="presentationDeckWidth === width.value"
                            :class="{ active: presentationDeckWidth === width.value }"
                            @click="applySlidePaletteWidth(width.value)"
                          >
                            {{ width.label }}
                          </button>
                        </div>
                      </section>
                      <section class="ppt-slide-palette-image-row">
                        <div>
                          <strong>图片</strong>
                          <span>{{ slide.imageUrl ? "当前页已有图片" : "为当前页生成或搜索配图" }}</span>
                        </div>
                        <button
                          type="button"
                          :title="slide.imageUrl ? '编辑当前页图片' : '为当前页添加图片'"
                          :aria-label="slide.imageUrl ? '编辑当前页图片' : '为当前页添加图片'"
                          @click="openSlideImagePanelFromPalette(index)"
                        >
                          {{ slide.imageUrl ? "编辑" : "+ 添加" }}
                        </button>
                      </section>
                    </div>
                  </div>
                  <div class="ppt-slide-magic">
                    <button
                      type="button"
                      :title="`AI 编辑第 ${index + 1} 页幻灯片`"
                      :aria-label="`AI 编辑第 ${index + 1} 页幻灯片`"
                      :aria-expanded="slideMagicMenuIndex === index"
                      aria-haspopup="menu"
                      @click.stop="toggleSlideMagicMenu(index)"
                    >
                      <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                        <path d="M9.9 2.8 8.8 6.2 5.4 7.3l3.4 1.1 1.1 3.4 1.1-3.4 3.4-1.1-3.4-1.1z" />
                        <path d="m18 10 1 3 3 1-3 1-1 3-1-3-3-1 3-1z" />
                        <path d="M4 17.5 3.4 19 2 19.6l1.4.6L4 21.6l.6-1.4 1.4-.6-1.4-.6z" />
                      </svg>
                    </button>
                    <div
                      v-if="slideMagicMenuIndex === index"
                      class="ppt-slide-magic-menu"
                      role="menu"
                      :aria-label="`AI 编辑第 ${index + 1} 页幻灯片`"
                      @click.stop
                      @keydown.esc.stop.prevent="slideMagicMenuIndex = null"
                    >
                      <header>
                        <strong>编辑这页幻灯片</strong>
                        <span>输入修改要求，或使用快捷操作。</span>
                      </header>
                      <label>
                        <input
                          ref="slideMagicPromptInputRef"
                          v-model="slideMagicPrompt"
                          type="text"
                          placeholder="想如何编辑这页幻灯片？"
                          title="AI 编辑这页幻灯片要求"
                          aria-label="AI 编辑这页幻灯片要求"
                          :disabled="slideMagicBusy"
                          @keydown.enter.prevent="submitSlideMagicPrompt(index)"
                        />
                        <button
                          type="button"
                          :disabled="slideMagicBusy || !slideMagicPrompt.trim()"
                          :title="slideMagicBusy ? '正在编辑这页幻灯片' : '发送编辑要求'"
                          :aria-label="slideMagicBusy ? '正在编辑这页幻灯片' : '发送编辑要求'"
                          :aria-busy="slideMagicBusy ? 'true' : undefined"
                          @click="submitSlideMagicPrompt(index)"
                        >
                          <span v-if="slideMagicBusy" class="ppt-spinner"></span>
                          <svg v-else class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                            <path d="M5 12h14" />
                            <path d="m13 6 6 6-6 6" />
                          </svg>
                        </button>
                      </label>
                      <div v-if="slideMagicBusy" class="ppt-slide-magic-busy" role="status" aria-live="polite">
                        <span class="ppt-spinner"></span>
                        <span>正在编辑这页幻灯片...</span>
                      </div>
                      <p v-if="slideMagicError" class="ppt-slide-magic-error">{{ slideMagicError }}</p>
                      <button
                        type="button"
                        class="ppt-slide-magic-primary"
                        role="menuitem"
                        :disabled="slideMagicBusy"
                        title="尝试新布局"
                        aria-label="尝试新布局"
                        :aria-busy="slideMagicBusy ? 'true' : undefined"
                        @click="runSlideMagicAction(index, 'layout')"
                      >
                        <span>
                          <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                            <path d="M9.9 2.8 8.8 6.2 5.4 7.3l3.4 1.1 1.1 3.4 1.1-3.4 3.4-1.1-3.4-1.1z" />
                            <path d="m18 10 1 3 3 1-3 1-1 3-1-3-3-1 3-1z" />
                          </svg>
                          尝试新布局
                        </span>
                        <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                          <path d="M5 12h14" />
                          <path d="m13 6 6 6-6 6" />
                        </svg>
                      </button>
                      <section class="ppt-slide-magic-section" aria-label="文案编辑">
                        <h4>文案</h4>
                        <div class="ppt-slide-magic-actions">
                          <button
                            v-for="action in slideMagicWritingActions"
                            :key="action.value"
                            type="button"
                            role="menuitem"
                            :disabled="slideMagicBusy"
                            :title="action.label"
                            :aria-label="action.label"
                            :aria-busy="slideMagicBusy ? 'true' : undefined"
                            @click="runSlideMagicAction(index, action.value)"
                          >
                            <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                              <template v-if="action.icon === 'pen'">
                                <path d="M12 20h9" />
                                <path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4z" />
                              </template>
                              <template v-else-if="action.icon === 'check'">
                                <path d="M20 6 9 17l-5-5" />
                              </template>
                              <template v-else-if="action.icon === 'globe'">
                                <circle cx="12" cy="12" r="10" />
                                <path d="M2 12h20" />
                                <path d="M12 2a15.3 15.3 0 0 1 0 20" />
                                <path d="M12 2a15.3 15.3 0 0 0 0 20" />
                              </template>
                              <template v-else>
                                <path d="M12 5v14" />
                                <path d="m19 12-7 7-7-7" />
                              </template>
                            </svg>
                            <span>{{ action.label }}</span>
                          </button>
                        </div>
                      </section>
                      <section class="ppt-slide-magic-section" aria-label="图片编辑">
                        <h4>图片</h4>
                        <div class="ppt-slide-magic-actions">
                          <button
                            v-for="action in slideMagicImageActions"
                            :key="action.value"
                            type="button"
                            role="menuitem"
                            :disabled="slideMagicBusy"
                            :title="action.label"
                            :aria-label="action.label"
                            :aria-busy="slideMagicBusy ? 'true' : undefined"
                            @click="runSlideMagicAction(index, action.value)"
                          >
                            <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                              <template v-if="action.icon === 'image'">
                                <rect x="3" y="5" width="18" height="14" rx="2" />
                                <circle cx="8.5" cy="10.5" r="1.5" />
                                <path d="M21 15l-5-5L5 19" />
                              </template>
                              <template v-else>
                                <path d="M5 12h14" />
                                <path d="M12 5v14" />
                              </template>
                            </svg>
                            <span>{{ action.label }}</span>
                          </button>
                        </div>
                      </section>
                    </div>
                  </div>
                </div>
                <button
                  type="button"
                  class="ppt-slide-insert-button is-after"
                  :disabled="isPresentationActionBusy"
                  :title="isPresentationActionBusy ? '正在生成中，暂不可插入幻灯片' : '在后方插入幻灯片'"
                  :aria-label="isPresentationActionBusy ? '正在生成中，暂不可插入幻灯片' : '在后方插入幻灯片'"
                  :aria-busy="isPresentationActionBusy ? 'true' : undefined"
                  @click.stop="insertSlideAt(index, 'after')"
                >
                  <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                    <path d="M5 12h14" />
                    <path d="M12 5v14" />
                  </svg>
                </button>
              </article>
            </div>
            </main>

            <aside class="ppt-right-edit-panel" :class="{ 'is-panel-closed': !presentationRightPanelOpen }" aria-label="编辑面板">
              <div class="ppt-right-panel-shell">
                <nav v-if="!presentationRightPanelOpen" class="ppt-right-tool-rail" aria-label="演示编辑工具">
                  <button
                    v-for="item in presentationOrderedToolPanels"
                    :key="item.value"
                    type="button"
                    class="ppt-right-tool-button"
                    :class="{ active: isPresentationToolPanelActive(item.value), 'is-group-start': isPresentationToolGroupStart(item.value) }"
                    :title="presentationToolButtonTitle(item)"
                    :aria-label="presentationToolButtonTitle(item)"
                    :aria-pressed="isPresentationToolPanelActive(item.value)"
                    :aria-busy="item.value === 'record' && presentModeBusy ? 'true' : undefined"
                    :disabled="isPresentationToolPanelDisabled(item.value)"
                    :data-ppt-right-tool="item.value"
                    :data-ppt-right-tool-group="presentationToolPanelGroup(item.value)"
                    @click="handlePresentationToolPanelClick(item.value)"
                  >
                    <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                      <template v-if="item.icon === 'text'">
                        <path d="M4 6h16" />
                        <path d="M9 6v12" />
                        <path d="M15 6v12" />
                        <path d="M7 18h10" />
                      </template>
                      <template v-else-if="item.icon === 'agent'">
                        <path d="M12 8V4" />
                        <rect x="5" y="8" width="14" height="10" rx="3" />
                        <path d="M8 13h.01" />
                        <path d="M16 13h.01" />
                        <path d="M9 18v2" />
                        <path d="M15 18v2" />
                      </template>
                      <template v-else-if="item.icon === 'blocks'">
                        <rect x="3" y="4" width="7" height="7" rx="1.5" />
                        <rect x="14" y="4" width="7" height="7" rx="1.5" />
                        <rect x="3" y="15" width="18" height="5" rx="1.5" />
                      </template>
                      <template v-else-if="item.icon === 'chart'">
                        <path d="M4 19V5" />
                        <path d="M4 19h16" />
                        <rect x="7" y="11" width="3" height="5" rx="1" />
                        <rect x="12" y="7" width="3" height="9" rx="1" />
                        <rect x="17" y="9" width="3" height="7" rx="1" />
                      </template>
                      <template v-else-if="item.icon === 'diagram'">
                        <circle cx="6" cy="7" r="2" />
                        <circle cx="18" cy="7" r="2" />
                        <circle cx="12" cy="17" r="2" />
                        <path d="M8 8.5 11 15" />
                        <path d="M16 8.5 13 15" />
                        <path d="M8 7h8" />
                      </template>
                      <template v-else-if="item.icon === 'embed'">
                        <path d="M10 13a5 5 0 0 0 7.07 0l2.12-2.12a5 5 0 0 0-7.07-7.07L11 4.93" />
                        <path d="M14 11a5 5 0 0 0-7.07 0L4.81 13.12a5 5 0 0 0 7.07 7.07L13 19.07" />
                      </template>
                      <template v-else-if="item.icon === 'image'">
                        <rect x="3" y="5" width="18" height="14" rx="2" />
                        <circle cx="8" cy="10" r="1.5" />
                        <path d="m21 16-5-5L5 19" />
                      </template>
                      <template v-else-if="item.icon === 'theme'">
                        <path d="M12 3a9 9 0 1 0 0 18h1.5a2.5 2.5 0 0 0 0-5H12a2 2 0 0 1 0-4h2a7 7 0 0 0 7-7 2 2 0 0 0-2-2z" />
                        <circle cx="7.5" cy="10.5" r=".7" />
                        <circle cx="10.5" cy="7.5" r=".7" />
                      </template>
                      <template v-else-if="item.icon === 'layout'">
                        <rect x="3" y="4" width="18" height="6" rx="1" />
                        <rect x="3" y="14" width="8" height="6" rx="1" />
                        <rect x="15" y="14" width="6" height="6" rx="1" />
                      </template>
                      <template v-else-if="item.icon === 'background'">
                        <rect x="4" y="5" width="16" height="14" rx="2" />
                        <path d="M4 15c4-4 7-4 10 0 2-2 4-3 6-2" />
                      </template>
                      <template v-else-if="item.icon === 'settings'">
                        <circle cx="12" cy="12" r="3" />
                        <path d="M19.4 15a1.7 1.7 0 0 0 .34 1.88l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.7 1.7 0 0 0-1.88-.34 1.7 1.7 0 0 0-1 1.55V21a2 2 0 1 1-4 0v-.08a1.7 1.7 0 0 0-1-1.55 1.7 1.7 0 0 0-1.88.34l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.7 1.7 0 0 0 4.6 15a1.7 1.7 0 0 0-1.55-1H3a2 2 0 1 1 0-4h.08a1.7 1.7 0 0 0 1.55-1 1.7 1.7 0 0 0-.34-1.88l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.7 1.7 0 0 0 9 4.6 1.7 1.7 0 0 0 10 3.05V3a2 2 0 1 1 4 0v.08a1.7 1.7 0 0 0 1 1.55 1.7 1.7 0 0 0 1.88-.34l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.7 1.7 0 0 0 19.4 9c.2.6.78 1 1.55 1H21a2 2 0 1 1 0 4h-.08a1.7 1.7 0 0 0-1.55 1Z" />
                      </template>
                      <template v-else-if="item.icon === 'sparkles'">
                        <path d="M9.9 2.8 8.8 6.2 5.4 7.3l3.4 1.1 1.1 3.4 1.1-3.4 3.4-1.1-3.4-1.1z" />
                        <path d="m18 10 1 3 3 1-3 1-1 3-1-3-3-1 3-1z" />
                        <path d="M4 17.5 3.4 19 2 19.6l1.4.6L4 21.6l.6-1.4 1.4-.6-1.4-.6z" />
                      </template>
                      <template v-else-if="item.icon === 'notes'">
                        <path d="M5 4h11l3 3v13H5z" />
                        <path d="M16 4v4h4" />
                        <path d="M8 12h8" />
                        <path d="M8 16h5" />
                      </template>
                      <template v-else>
                        <path d="M15 10l4.55-2.27A1 1 0 0 1 21 8.62v6.76a1 1 0 0 1-1.45.89L15 14" />
                        <rect x="3" y="6" width="12" height="12" rx="2" />
                      </template>
                    </svg>
                    <span>{{ item.label }}</span>
                  </button>
                </nav>

                <section v-if="presentationRightPanelOpen" ref="presentationRightPanelContentRef" class="ppt-right-panel-content">
                    <header v-if="!isPresentationSelfContainedPanel" class="ppt-right-panel-header">
                      <div class="ppt-right-panel-title">
                        <span class="ppt-right-panel-icon" aria-hidden="true">
                          <svg class="ppt-toolbar-icon" viewBox="0 0 24 24">
                            <path v-if="activeRightPanelMeta.icon === 'text'" d="M4 6h16M9 6v12M15 6v12M7 18h10" />
                            <path v-else-if="activeRightPanelMeta.icon === 'agent'" d="M12 8V4M5 11a3 3 0 0 1 3-3h8a3 3 0 0 1 3 3v4a3 3 0 0 1-3 3H8a3 3 0 0 1-3-3zM8 13h.01M16 13h.01M9 18v2M15 18v2" />
                            <path v-else-if="activeRightPanelMeta.icon === 'blocks'" d="M4 6h6v6H4zM14 6h6v6h-6zM4 16h16v3H4z" />
                            <path v-else-if="activeRightPanelMeta.icon === 'chart'" d="M4 19V5M4 19h16M8 16v-5M13 16V7M18 16V9" />
                            <path v-else-if="activeRightPanelMeta.icon === 'diagram'" d="M6 7h12M8 9l4 8 4-8M12 17h.01" />
                            <path v-else-if="activeRightPanelMeta.icon === 'embed'" d="M10 13a5 5 0 0 0 7 0l2-2a5 5 0 0 0-7-7l-1 1M14 11a5 5 0 0 0-7 0l-2 2a5 5 0 0 0 7 7l1-1" />
                            <path v-else-if="activeRightPanelMeta.icon === 'image'" d="M3 5h18v14H3zM8 10h.01M21 16l-5-5L5 19" />
                            <path v-else-if="activeRightPanelMeta.icon === 'theme'" d="M12 3a9 9 0 1 0 0 18h2a2.5 2.5 0 0 0 0-5h-2a2 2 0 0 1 0-4h2a7 7 0 0 0 7-7 2 2 0 0 0-2-2z" />
                            <path v-else-if="activeRightPanelMeta.icon === 'layout'" d="M3 4h18v6H3zM3 14h8v6H3zM15 14h6v6h-6z" />
                            <path v-else-if="activeRightPanelMeta.icon === 'background'" d="M4 5h16v14H4zM4 15c4-4 7-4 10 0 2-2 4-3 6-2" />
                            <path v-else-if="activeRightPanelMeta.icon === 'settings'" d="M12 15.5A3.5 3.5 0 1 0 12 8a3.5 3.5 0 0 0 0 7.5ZM19.4 15a1.7 1.7 0 0 0 .34 1.88l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.7 1.7 0 0 0-1.88-.34 1.7 1.7 0 0 0-1 1.55V21a2 2 0 1 1-4 0v-.08a1.7 1.7 0 0 0-1-1.55 1.7 1.7 0 0 0-1.88.34l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.7 1.7 0 0 0 4.6 15a1.7 1.7 0 0 0-1.55-1H3a2 2 0 1 1 0-4h.08a1.7 1.7 0 0 0 1.55-1 1.7 1.7 0 0 0-.34-1.88l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.7 1.7 0 0 0 9 4.6 1.7 1.7 0 0 0 10 3.05V3a2 2 0 1 1 4 0v.08a1.7 1.7 0 0 0 1 1.55 1.7 1.7 0 0 0 1.88-.34l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.7 1.7 0 0 0 19.4 9c.2.6.78 1 1.55 1H21a2 2 0 1 1 0 4h-.08a1.7 1.7 0 0 0-1.55 1Z" />
                            <path v-else-if="activeRightPanelMeta.icon === 'sparkles'" d="M9.9 2.8 8.8 6.2 5.4 7.3l3.4 1.1 1.1 3.4 1.1-3.4 3.4-1.1-3.4-1.1zM18 10l1 3 3 1-3 1-1 3-1-3-3-1 3-1zM4 17.5 3.4 19 2 19.6l1.4.6L4 21.6l.6-1.4 1.4-.6-1.4-.6z" />
                            <path v-else-if="activeRightPanelMeta.icon === 'notes'" d="M5 4h11l3 3v13H5zM16 4v4h4M8 12h8M8 16h5" />
                            <path v-else d="M15 10l5-2.5v9L15 14M3 6h12v12H3z" />
                          </svg>
                        </span>
                        <div>
                          <strong>{{ activeRightPanelMeta.label }}</strong>
                          <span>{{ activeRightPanelMeta.hint }}</span>
                        </div>
                      </div>
                      <button type="button" title="关闭面板" aria-label="关闭面板" @click="closePresentationRightPanel">
                        <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                          <path d="M18 6 6 18" />
                          <path d="m6 6 12 12" />
                        </svg>
                      </button>
                    </header>

                    <div v-if="isPresentationPanelContentLoaded" class="ppt-right-panel-body" :class="{ 'is-self-contained': isPresentationSelfContainedPanel }">
                    <div v-if="isPresentationSearchablePanel" class="ppt-panel-search-stack">
                      <label class="ppt-panel-search">
                        <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                          <circle cx="11" cy="11" r="7" />
                          <path d="m20 20-3.5-3.5" />
                        </svg>
                        <input
                          v-model="presentationPanelSearchQuery"
                          :title="activeRightPanelMeta.searchPlaceholder || '搜索当前面板'"
                          :aria-label="activeRightPanelMeta.searchPlaceholder || '搜索当前面板'"
                          :placeholder="activeRightPanelMeta.searchPlaceholder"
                          @keydown.stop
                        />
                        <button
                          v-if="presentationPanelSearchQuery"
                          type="button"
                          title="清空搜索"
                          aria-label="清空搜索"
                          @click="clearPresentationPanelSearch"
                        >
                          <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                            <path d="M18 6 6 18" />
                            <path d="m6 6 12 12" />
                          </svg>
                        </button>
                      </label>
                      <div v-if="activePanelCategoryOptions.length > 1" class="ppt-panel-filter-row" role="tablist" aria-label="面板分类">
                        <button
                          v-for="option in activePanelCategoryOptions"
                          :key="option.value"
                          type="button"
                          :title="`筛选${activeRightPanelMeta.label}：${option.label}`"
                          :aria-label="`筛选${activeRightPanelMeta.label}：${option.label}${presentationPanelCategory === option.value ? '，当前分类' : ''}`"
                          :aria-pressed="presentationPanelCategory === option.value"
                          :class="{ active: presentationPanelCategory === option.value }"
                          @click="presentationPanelCategory = option.value"
                        >
                          {{ option.label }}
                        </button>
                      </div>
                    </div>

                    <section v-if="presentationRightPanel === 'content'" class="ppt-panel-section">
                      <div v-if="filteredPresentationTextBlocks.length" class="ppt-insert-grid is-basic-blocks">
                        <button
                          v-for="(item, index) in filteredPresentationTextBlocks"
                          :key="item.title"
                          type="button"
                          class="ppt-insert-card is-basic-block"
                          :class="{ active: isPresentationPaletteBlockSelected('content', item) }"
                          :aria-pressed="isPresentationPaletteBlockSelected('content', item)"
                          :title="`插入${item.title}`"
                          :aria-label="`插入${item.title}：${item.description}`"
                          :tabindex="presentationPaletteCardTabIndex('content', item, index)"
                          data-panel-arrow-target="true"
                          @click="insertTextBlock(item)"
                          @keydown="handlePresentationPaletteCardKeydown($event, index)"
                        >
                          <span class="ppt-card-grip" aria-hidden="true">
                            <svg class="ppt-toolbar-icon" viewBox="0 0 24 24">
                              <path d="M9 5h.01M15 5h.01M9 12h.01M15 12h.01M9 19h.01M15 19h.01" />
                            </svg>
                          </span>
                          <span class="ppt-card-preview" :data-kind="item.icon">
                            {{ item.iconLabel }}
                          </span>
                          <strong>{{ item.title }}</strong>
                          <small>{{ item.description }}</small>
                        </button>
                      </div>
                      <PptEmptyState
                        v-else
                        class="ppt-panel-empty-state"
                        title="没有匹配的文本块"
                        description="换个关键词或清空搜索后再试。"
                      />
                      <button
                        type="button"
                        class="ppt-panel-secondary-toggle"
                        :title="showCurrentSlideTextEditor ? '收起当前页文案' : '展开当前页文案'"
                        :aria-label="showCurrentSlideTextEditor ? '收起当前页文案' : '展开当前页文案'"
                        :aria-expanded="showCurrentSlideTextEditor"
                        @click="showCurrentSlideTextEditor = !showCurrentSlideTextEditor"
                      >
                        <span>当前页文案</span>
                        <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true" :class="{ 'is-expanded': showCurrentSlideTextEditor }">
                          <path d="m6 9 6 6 6-6" />
                        </svg>
                      </button>
                      <div v-if="showCurrentSlideTextEditor" class="ppt-current-slide-editor">
                        <PptSlideEditor
                          :slide="store.currentSlide"
                          @save="handleSlideSave"
                          @cancel="ElMessage.info('已取消本次编辑')"
                          @regenerate="store.regenerateCurrentSlide"
                        />
                      </div>
                    </section>
                  <section v-else-if="presentationRightPanel === 'agent'" class="ppt-agent-panel">
                    <div class="ppt-agent-current-slide">
                      <span>当前页</span>
                      <strong>{{ store.currentSlide ? presentationSlideTitle(store.currentSlide) : "未选择幻灯片" }}</strong>
                      <small>{{ activeSlideLabel }}</small>
                    </div>
                    <div class="ppt-agent-quick-prompts">
                      <button
                        v-for="prompt in presentationAgentQuickPrompts"
                        :key="prompt"
                        type="button"
                        :disabled="presentationAgentBusy || !store.currentSlide"
                        :title="`发送快捷要求：${prompt}`"
                        :aria-label="`发送快捷要求：${prompt}`"
                        @click="runPresentationAgentPrompt(prompt)"
                      >
                        {{ prompt }}
                      </button>
                    </div>
                    <div class="ppt-agent-messages" aria-live="polite">
                      <article
                        v-for="(message, index) in presentationAgentMessages"
                        :key="`${message.role}-${index}`"
                        :class="message.role"
                      >
                        <strong>{{ message.role === "assistant" ? "PPT 助手" : "你" }}</strong>
                        <p>{{ message.content }}</p>
                      </article>
                      <article v-if="presentationAgentBusy" class="assistant">
                        <strong>PPT 助手</strong>
                        <p>正在分析当前页...</p>
                      </article>
                    </div>
                    <div class="ppt-agent-input">
                      <textarea
                        v-model="presentationAgentPrompt"
                        placeholder="告诉 PPT 助手你想怎么改当前页..."
                        title="PPT 助手编辑要求"
                        aria-label="PPT 助手编辑要求"
                        :disabled="presentationAgentBusy"
                        @keydown.enter.exact.prevent="runPresentationAgentPrompt()"
                      ></textarea>
                      <button
                        type="button"
                        :disabled="presentationAgentBusy || !presentationAgentPrompt.trim() || !store.currentSlide"
                        :title="presentationAgentBusy ? 'PPT 助手处理中' : '发送给 PPT 助手'"
                        :aria-label="presentationAgentBusy ? 'PPT 助手处理中' : '发送给 PPT 助手'"
                        :aria-busy="presentationAgentBusy ? 'true' : undefined"
                        @click="runPresentationAgentPrompt()"
                      >
                        <span v-if="presentationAgentBusy" class="ppt-spinner"></span>
                        <span>{{ presentationAgentBusy ? "处理中" : "发送" }}</span>
                      </button>
                    </div>
                    <button
                      type="button"
                      class="ppt-agent-apply"
                      :disabled="!latestAgentSuggestion || !store.currentSlide"
                      title="应用上一条建议到当前页"
                      aria-label="应用上一条建议到当前页"
                      @click="applyLatestAgentSuggestion"
                    >
                      应用上一条建议到当前页
                    </button>
                  </section>
                  <section v-else-if="presentationRightPanel === 'elements'" class="ppt-panel-section">
                    <div v-if="filteredPresentationElementBlocks.length" class="ppt-insert-grid is-two-column">
                      <button
                        v-for="(item, index) in filteredPresentationElementBlocks"
                        :key="item.title"
                        type="button"
                        class="ppt-insert-card"
                        :class="{ active: isPresentationPaletteBlockSelected('elements', item) }"
                        :aria-pressed="isPresentationPaletteBlockSelected('elements', item)"
                        :title="`插入${item.title}`"
                        :aria-label="`插入${item.title}：${item.description}`"
                        :tabindex="presentationPaletteCardTabIndex('elements', item, index)"
                        data-panel-arrow-target="true"
                        @click="insertElementBlock(item)"
                        @keydown="handlePresentationPaletteCardKeydown($event, index)"
                      >
                        <span class="ppt-card-grip" aria-hidden="true">
                          <svg class="ppt-toolbar-icon" viewBox="0 0 24 24">
                            <path d="M9 5h.01M15 5h.01M9 12h.01M15 12h.01M9 19h.01M15 19h.01" />
                          </svg>
                        </span>
                        <span class="ppt-card-preview" :data-kind="item.icon">{{ item.iconLabel }}</span>
                        <strong>{{ item.title }}</strong>
                        <small>{{ item.description }}</small>
                      </button>
                    </div>
                    <PptEmptyState
                      v-else
                      class="ppt-panel-empty-state"
                      title="没有匹配的元素"
                      description="可切换分类或清空搜索。"
                    />
                  </section>
                  <section v-else-if="presentationRightPanel === 'charts'" class="ppt-panel-section">
                    <div v-if="filteredPresentationChartBlocks.length" class="ppt-insert-grid is-two-column">
                      <button
                        v-for="(item, index) in filteredPresentationChartBlocks"
                        :key="item.title"
                        type="button"
                        class="ppt-insert-card is-chart-card"
                        :class="{ active: isPresentationPaletteBlockSelected('charts', item) }"
                        :aria-pressed="isPresentationPaletteBlockSelected('charts', item)"
                        :title="`插入${item.title}图表`"
                        :aria-label="`插入${item.title}图表：${item.description}`"
                        :tabindex="presentationPaletteCardTabIndex('charts', item, index)"
                        data-panel-arrow-target="true"
                        @click="insertChartBlock(item)"
                        @keydown="handlePresentationPaletteCardKeydown($event, index)"
                      >
                        <span class="ppt-chart-card-header">
                          <span class="ppt-card-grip" aria-hidden="true">
                            <svg class="ppt-toolbar-icon" viewBox="0 0 24 24">
                              <path d="M9 5h.01M15 5h.01M9 12h.01M15 12h.01M9 19h.01M15 19h.01" />
                            </svg>
                          </span>
                          <strong>{{ item.title }}</strong>
                        </span>
                        <span class="ppt-card-preview is-chart" :data-kind="item.icon">{{ item.iconLabel }}</span>
                      </button>
                    </div>
                    <PptEmptyState
                      v-else
                      class="ppt-panel-empty-state"
                      title="没有匹配的图表"
                      description="参考项目支持多种图表模板，这里先保留可插入占位。"
                    />
                  </section>
                  <section v-else-if="presentationRightPanel === 'diagrams'" class="ppt-panel-section">
                    <div v-if="groupedFilteredPresentationDiagramBlocks.length" class="ppt-diagram-category-list">
                      <section
                        v-for="group in groupedFilteredPresentationDiagramBlocks"
                        :key="group.category"
                        class="ppt-diagram-category"
                      >
                        <button
                          type="button"
                          class="ppt-diagram-category-trigger"
                          :title="`${isPresentationDiagramCategoryCollapsed(group.category) ? '展开' : '收起'}${group.label}`"
                          :aria-label="`${isPresentationDiagramCategoryCollapsed(group.category) ? '展开' : '收起'}${group.label}图示分类，共 ${group.items.length} 个`"
                          :aria-expanded="!isPresentationDiagramCategoryCollapsed(group.category)"
                          @click="togglePresentationDiagramCategory(group.category)"
                        >
                          <span>{{ group.label }} <small>({{ group.items.length }})</small></span>
                          <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true" :class="{ 'is-collapsed': isPresentationDiagramCategoryCollapsed(group.category) }">
                            <path d="m6 9 6 6 6-6" />
                          </svg>
                        </button>
                        <div v-if="!isPresentationDiagramCategoryCollapsed(group.category)" class="ppt-insert-grid is-two-column">
                          <button
                            v-for="(item, index) in group.items"
                            :key="item.title"
                            type="button"
                            class="ppt-insert-card is-diagram-card"
                            :class="{ active: isPresentationPaletteBlockSelected('diagrams', item) }"
                            :aria-pressed="isPresentationPaletteBlockSelected('diagrams', item)"
                            :title="`插入${item.title}图示`"
                            :aria-label="`插入${item.title}图示：${item.description}`"
                            :tabindex="presentationDiagramCardTabIndex(item, index)"
                            data-panel-arrow-target="true"
                            @click="insertDiagramBlock(item)"
                            @keydown="handlePresentationPaletteCardKeydown($event, index)"
                          >
                            <span class="ppt-card-preview is-diagram" :data-kind="item.icon">{{ item.iconLabel }}</span>
                            <span class="ppt-diagram-card-label">
                              <span class="ppt-card-grip" aria-hidden="true">
                                <svg class="ppt-toolbar-icon" viewBox="0 0 24 24">
                                  <path d="M9 5h.01M15 5h.01M9 12h.01M15 12h.01M9 19h.01M15 19h.01" />
                                </svg>
                              </span>
                              <strong>{{ item.title }}</strong>
                            </span>
                          </button>
                        </div>
                      </section>
                    </div>
                    <PptEmptyState
                      v-else
                      class="ppt-panel-empty-state"
                      title="没有匹配的图示"
                      description="可搜索流程、漏斗、时间线等关键词。"
                    />
                  </section>
                  <section v-else-if="presentationRightPanel === 'embed'" class="ppt-panel-section ppt-embed-panel">
                    <div v-if="filteredPresentationEmbedBlocks.length" class="ppt-insert-grid is-two-column">
                      <button
                        v-for="(item, index) in filteredPresentationEmbedBlocks"
                        :key="item.title"
                        type="button"
                        class="ppt-insert-card is-embed-card"
                        :class="{ active: isPresentationPaletteBlockSelected('embed', item) }"
                        :aria-pressed="isPresentationPaletteBlockSelected('embed', item)"
                        :title="`选择${item.title}`"
                        :aria-label="`选择${item.title}：${item.description}`"
                        :tabindex="presentationPaletteCardTabIndex('embed', item, index)"
                        data-panel-arrow-target="true"
                        @click="insertEmbedTemplateBlock(item)"
                        @keydown="handlePresentationPaletteCardKeydown($event, index)"
                      >
                        <span class="ppt-card-grip" aria-hidden="true">
                          <svg class="ppt-toolbar-icon" viewBox="0 0 24 24">
                            <path d="M9 5h.01M15 5h.01M9 12h.01M15 12h.01M9 19h.01M15 19h.01" />
                          </svg>
                        </span>
                        <span class="ppt-embed-card-icon" :data-kind="item.icon">{{ item.iconLabel }}</span>
                        <span class="ppt-embed-card-copy">
                          <strong>{{ item.title }}</strong>
                          <small>{{ item.description }}</small>
                        </span>
                      </button>
                    </div>
                    <PptEmptyState
                      v-else
                      class="ppt-panel-empty-state"
                      title="没有匹配的嵌入类型"
                      description="可搜索视频、网页、设计稿或图片。"
                    />
                    <button
                      type="button"
                      class="ppt-panel-secondary-toggle"
                      :title="showEmbedUrlCard ? '收起自定义链接' : '展开自定义链接'"
                      :aria-label="showEmbedUrlCard ? '收起自定义链接' : '展开自定义链接'"
                      :aria-expanded="showEmbedUrlCard"
                      @click="showEmbedUrlCard = !showEmbedUrlCard"
                    >
                      <span>自定义链接</span>
                      <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true" :class="{ 'is-expanded': showEmbedUrlCard }">
                        <path d="m6 9 6 6 6-6" />
                      </svg>
                    </button>
                    <div v-if="showEmbedUrlCard" class="ppt-embed-url-card">
                      <label>
                        <span>媒体或网页链接</span>
                        <input
                          v-model="presentationEmbedUrl"
                          title="嵌入链接"
                          aria-label="嵌入链接"
                          placeholder="粘贴网页、视频或数据看板链接"
                          @keydown.enter.prevent="insertEmbedBlock"
                        />
                      </label>
                      <button
                        type="button"
                        :disabled="!presentationEmbedUrl.trim()"
                        title="插入链接占位"
                        aria-label="插入链接占位"
                        @click="insertEmbedBlock"
                      >插入链接占位</button>
                      <p>第一版先写入当前页占位内容，后续可接入网页卡片、视频 iframe 或数据看板。</p>
                    </div>
                  </section>
                  <div v-else-if="presentationRightPanel === 'theme'" class="ppt-right-theme-panel">
                    <PptThemeSelector
                      v-model="store.theme"
                      show-close
                      @close="closePresentationRightPanel"
                      @create-theme="openPresentationThemeCreator(false)"
                      @import-theme="handlePresentationThemeImport"
                    />
                  </div>
                  <section v-else-if="presentationRightPanel === 'layout'" class="ppt-layout-panel">
                    <div class="ppt-layout-panel-head">
                      <strong>当前页布局</strong>
                      <button
                        type="button"
                        :disabled="!store.currentSlide"
                        :title="store.currentSlide ? '尝试新布局' : '请先选择一页幻灯片'"
                        :aria-label="store.currentSlide ? '尝试新布局' : '请先选择一页幻灯片后再尝试新布局'"
                        @click="applyNextSlideLayout"
                      >尝试新布局</button>
                    </div>
                    <div class="ppt-layout-choice-grid">
                      <button
                        v-for="item in presentationLayoutOptions"
                        :key="item.value"
                        type="button"
                        class="ppt-layout-choice-card"
                        :class="{ active: store.currentSlide?.layout === item.value }"
                        :disabled="!store.currentSlide"
                        :title="store.currentSlide ? `切换为${item.label}布局` : '请先选择一页幻灯片'"
                        :aria-label="store.currentSlide ? `切换为${item.label}布局：${item.description}` : '请先选择一页幻灯片后再切换布局'"
                        :aria-pressed="store.currentSlide?.layout === item.value"
                        @click="applySlideLayout(item.value)"
                      >
                        <span class="ppt-layout-choice-preview" :data-layout="item.value">
                          <i></i><i></i><i></i>
                        </span>
                        <strong>{{ item.label }}</strong>
                        <small>{{ item.description }}</small>
                      </button>
                    </div>
                  </section>
                  <section v-else-if="presentationRightPanel === 'background'" class="ppt-background-panel">
                    <div class="ppt-background-actions">
                      <label class="ppt-background-custom-color" :class="{ disabled: !store.currentSlide }">
                        <input
                          type="color"
                          :disabled="!store.currentSlide"
                          :value="customBackgroundColorValue"
                          title="自定义背景颜色"
                          aria-label="自定义背景颜色"
                          @input="applyCustomBackgroundColor"
                        />
                        <span>+ 自定义</span>
                      </label>
                      <button type="button" :disabled="!store.currentSlide" title="重置当前页背景" aria-label="重置当前页背景" @click="resetCurrentSlideBackground">重置</button>
                      <button type="button" :disabled="!store.currentSlide" title="切换下一个背景预设" aria-label="切换下一个背景预设" @click="applyNextBackgroundPreset">换一个</button>
                    </div>
                    <div class="ppt-background-tabs" role="tablist" aria-label="背景类型">
                      <button type="button" role="tab" title="纯色背景" aria-label="纯色背景" :aria-selected="presentationBackgroundMode === 'solid'" :class="{ active: presentationBackgroundMode === 'solid' }" @click="presentationBackgroundMode = 'solid'">纯色</button>
                      <button type="button" role="tab" title="线性渐变背景" aria-label="线性渐变背景" :aria-selected="presentationBackgroundMode === 'linear'" :class="{ active: presentationBackgroundMode === 'linear' }" @click="presentationBackgroundMode = 'linear'">线性</button>
                      <button type="button" role="tab" title="径向渐变背景" aria-label="径向渐变背景" :aria-selected="presentationBackgroundMode === 'radial'" :class="{ active: presentationBackgroundMode === 'radial' }" @click="presentationBackgroundMode = 'radial'">径向</button>
                      <button type="button" role="tab" title="图片背景" aria-label="图片背景" :aria-selected="presentationBackgroundMode === 'image'" :class="{ active: presentationBackgroundMode === 'image' }" @click="presentationBackgroundMode = 'image'">图片</button>
                    </div>
                    <div v-if="presentationBackgroundMode === 'solid'" class="ppt-background-swatch-grid">
                      <button
                        v-for="item in solidBackgroundPresets"
                        :key="item.value"
                        type="button"
                        :class="{ active: currentSlideBackground === item.value }"
                        :style="{ background: item.value }"
                        :title="item.label"
                        :aria-label="`应用${item.label}背景`"
                        :aria-pressed="currentSlideBackground === item.value"
                        @click="applySlideBackground(item.value)"
                      ><span>{{ item.label }}</span></button>
                    </div>
                    <div v-else-if="presentationBackgroundMode === 'linear'" class="ppt-background-swatch-grid">
                      <button
                        v-for="item in linearBackgroundPresets"
                        :key="item.value"
                        type="button"
                        :class="{ active: currentSlideBackground === item.value }"
                        :style="{ background: item.value }"
                        :title="item.label"
                        :aria-label="`应用${item.label}背景`"
                        :aria-pressed="currentSlideBackground === item.value"
                        @click="applySlideBackground(item.value)"
                      ><span>{{ item.label }}</span></button>
                    </div>
                    <div v-else-if="presentationBackgroundMode === 'radial'" class="ppt-background-swatch-grid">
                      <button
                        v-for="item in radialBackgroundPresets"
                        :key="item.value"
                        type="button"
                        :class="{ active: currentSlideBackground === item.value }"
                        :style="{ background: item.value }"
                        :title="item.label"
                        :aria-label="`应用${item.label}背景`"
                        :aria-pressed="currentSlideBackground === item.value"
                        @click="applySlideBackground(item.value)"
                      ><span>{{ item.label }}</span></button>
                    </div>
                    <label v-else class="ppt-background-image-control">
                      <span>图片 URL</span>
                      <input v-model="presentationBackgroundImageUrl" title="图片背景 URL" aria-label="图片背景 URL" placeholder="粘贴图片地址，例如 https://..." @keydown.enter.prevent="applySlideBackgroundImage" />
                      <button type="button" :disabled="!presentationBackgroundImageUrl.trim()" title="应用图片背景" aria-label="应用图片背景" @click="applySlideBackgroundImage">应用图片背景</button>
                    </label>
                  </section>
                  <section v-else-if="presentationRightPanel === 'globalSettings'" class="ppt-global-settings-panel">
                    <header class="ppt-global-settings-header">
                      <div>
                        <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                          <circle cx="12" cy="12" r="3" />
                          <path d="M19.4 15a1.7 1.7 0 0 0 .34 1.88l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.7 1.7 0 0 0-1.88-.34 1.7 1.7 0 0 0-1 1.55V21a2 2 0 1 1-4 0v-.08a1.7 1.7 0 0 0-1-1.55 1.7 1.7 0 0 0-1.88.34l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.7 1.7 0 0 0 4.6 15a1.7 1.7 0 0 0-1.55-1H3a2 2 0 1 1 0-4h.08a1.7 1.7 0 0 0 1.55-1 1.7 1.7 0 0 0-.34-1.88l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.7 1.7 0 0 0 9 4.6 1.7 1.7 0 0 0 10 3.05V3a2 2 0 1 1 4 0v.08a1.7 1.7 0 0 0 1 1.55 1.7 1.7 0 0 0 1.88-.34l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.7 1.7 0 0 0 19.4 9c.2.6.78 1 1.55 1H21a2 2 0 1 1 0 4h-.08a1.7 1.7 0 0 0-1.55 1Z" />
                        </svg>
                        <strong>页面设置</strong>
                      </div>
                      <button type="button" title="关闭设置面板" aria-label="关闭设置面板" @click="closePresentationRightPanel">
                        <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                          <path d="M18 6 6 18" />
                          <path d="m6 6 12 12" />
                        </svg>
                      </button>
                    </header>
                    <div class="ppt-global-settings-tabs" role="tablist" aria-label="页面设置">
                      <button type="button" role="tab" title="页面设置" aria-label="页面设置" :aria-selected="presentationGlobalSettingsTab === 'cards'" :class="{ active: presentationGlobalSettingsTab === 'cards' }" @click="presentationGlobalSettingsTab = 'cards'">
                        <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                          <rect x="3" y="4" width="18" height="6" rx="1" />
                          <rect x="3" y="14" width="8" height="6" rx="1" />
                          <rect x="15" y="14" width="6" height="6" rx="1" />
                        </svg>
                        <span>页面</span>
                      </button>
                      <button type="button" role="tab" title="主题设置" aria-label="主题设置" :aria-selected="presentationGlobalSettingsTab === 'theme'" :class="{ active: presentationGlobalSettingsTab === 'theme' }" @click="presentationGlobalSettingsTab = 'theme'">
                        <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                          <path d="M4 7V4h16v3" />
                          <path d="M9 20h6" />
                          <path d="M12 4v16" />
                        </svg>
                        <span>主题</span>
                      </button>
                      <button type="button" role="tab" title="背景设置" aria-label="背景设置" :aria-selected="presentationGlobalSettingsTab === 'background'" :class="{ active: presentationGlobalSettingsTab === 'background' }" @click="presentationGlobalSettingsTab = 'background'">
                        <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                          <rect x="4" y="5" width="16" height="14" rx="2" />
                          <path d="M4 15c4-4 7-4 10 0 2-2 4-3 6-2" />
                        </svg>
                        <span>背景</span>
                      </button>
                    </div>
                    <div v-if="presentationGlobalSettingsTab === 'cards'" class="ppt-global-settings-section">
                      <strong>页面宽度</strong>
                      <div class="ppt-setting-choice-grid">
                        <button
                          v-for="item in presentationDeckWidthOptions"
                          :key="item.value"
                          type="button"
                          :class="{ active: presentationDeckWidth === item.value }"
                          :title="`切换页面宽度为${item.label}`"
                          :aria-label="`切换页面宽度为${item.label}：${item.description}`"
                          :aria-pressed="presentationDeckWidth === item.value"
                          @click="applyDeckWidth(item.value)"
                        >
                          <span>{{ item.label }}</span>
                          <small>{{ item.description }}</small>
                        </button>
                      </div>
                      <strong>内容对齐</strong>
                      <div class="ppt-setting-segmented" role="group" aria-label="内容对齐">
                        <button
                          v-for="item in presentationElementAlignments"
                          :key="item.value"
                          type="button"
                          :class="{ active: presentationGlobalAlignment === item.value }"
                          :title="`全部页面${item.label}`"
                          :aria-label="`全部页面${item.label}`"
                          :aria-pressed="presentationGlobalAlignment === item.value"
                          @click="applyGlobalAlignment(item.value)"
                        >
                          {{ item.label }}
                        </button>
                      </div>
                      <strong>字号比例</strong>
                      <div class="ppt-setting-segmented" role="group" aria-label="字号比例">
                        <button
                          v-for="item in presentationTypographyScaleOptions"
                          :key="item.value"
                          type="button"
                          :class="{ active: presentationTypographyScale === item.value }"
                          :title="`字号比例：${item.label}`"
                          :aria-label="`字号比例：${item.label}`"
                          :aria-pressed="presentationTypographyScale === item.value"
                          @click="applyTypographyScale(item.value)"
                        >
                          {{ item.label }}
                        </button>
                      </div>
                    </div>
                    <div v-else-if="presentationGlobalSettingsTab === 'theme'" class="ppt-global-settings-section">
                      <PptThemeSelector
                        v-model="store.theme"
                        @create-theme="openPresentationThemeCreator(false)"
                        @import-theme="handlePresentationThemeImport"
                      />
                    </div>
                    <div v-else class="ppt-global-settings-section">
                      <strong>背景应用范围</strong>
                      <p>当前“背景”面板默认只作用于所选页面。这里可把当前页背景同步到整套演示，或快速回到背景面板继续调整。</p>
                      <button type="button" class="ppt-setting-wide-button" :disabled="!currentSlideBackground" :title="currentSlideBackground ? '把当前页背景应用到全部页面' : '当前页还没有自定义背景'" :aria-label="currentSlideBackground ? '把当前页背景应用到全部页面' : '当前页还没有自定义背景'" @click="applyCurrentBackgroundToAllSlides">应用当前背景到全部页面</button>
                      <button type="button" class="ppt-setting-wide-button" title="打开当前页背景面板" aria-label="打开当前页背景面板" @click="selectPresentationRightPanel('background')">打开当前页背景</button>
                    </div>
                  </section>
                  <section v-else-if="presentationRightPanel === 'iconPicker'" class="ppt-icon-picker-panel">
                    <label class="ppt-panel-search">
                      <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                        <path d="m21 21-4.3-4.3" />
                        <circle cx="11" cy="11" r="7" />
                      </svg>
                      <input ref="presentationIconSearchInputRef" v-model="presentationIconSearchQuery" title="搜索图标" aria-label="搜索图标" placeholder="搜索图标，例如 AI、数据、重点..." />
                      <button v-if="presentationIconSearchQuery" type="button" title="清空搜索" aria-label="清空搜索" @click="presentationIconSearchQuery = ''">
                        <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                          <path d="M18 6 6 18" />
                          <path d="m6 6 12 12" />
                        </svg>
                      </button>
                    </label>
                    <div class="ppt-icon-current-card">
                      <span>{{ currentSlideIcon?.glyph || "Aa" }}</span>
                      <div>
                        <strong>{{ currentSlideIcon ? currentSlideIcon.label : "当前页未设置图标" }}</strong>
                        <small>{{ store.currentSlide ? presentationSlideTitle(store.currentSlide) : "请先选择一页幻灯片" }}</small>
                      </div>
                      <button type="button" :disabled="!currentSlideIcon" title="移除当前页图标" aria-label="移除当前页图标" @click="removeCurrentSlideIcon">移除</button>
                    </div>
                    <div v-if="filteredPresentationIconOptions.length" class="ppt-icon-picker-grid">
                      <button
                        v-for="item in filteredPresentationIconOptions"
                        :key="item.name"
                        type="button"
                        :class="{ active: currentSlideIcon?.name === item.name }"
                        :title="item.label"
                        :aria-label="`选择${item.label}图标`"
                        :aria-pressed="currentSlideIcon?.name === item.name"
                        @click="selectPresentationIcon(item.name)"
                      >
                        <span>{{ item.glyph }}</span>
                        <small>{{ item.label }}</small>
                      </button>
                    </div>
                    <div v-else class="ppt-panel-empty-state">
                      <strong>没有找到图标</strong>
                      <span>换个关键词试试，例如“AI”“数据”“目标”。</span>
                    </div>
                  </section>
                  <PptImageSourcePanel
                    v-else-if="presentationRightPanel === 'images'"
                    :image-source="store.imageSource"
                    :image-model="store.imageModel"
                    :image-models="store.imageModels"
                    :current-slide="store.currentSlide"
                    :results="store.imageSearchResults"
                    :generating="store.imageGenerating"
                    :visual-operation-status="store.visualOperationStatus"
                    @update:image-source="store.imageSource = $event"
                    @update:image-model="store.imageModel = $event"
                    @generate-image="store.generateImageForCurrentSlide"
                    @update-visual-plan="store.updateCurrentSlideVisualPlan"
                    @delete-visual="store.deleteCurrentSlideVisual"
                    @restore-visual="store.restoreCurrentSlideVisual"
                    @search-images="store.searchImages"
                    @apply-image="store.applyImageToCurrentSlide"
                    @close="closePresentationRightPanel"
                  />
                  <section v-else-if="presentationRightPanel === 'notes'" class="ppt-speaker-notes">
                    <header>
                      <div>
                        <strong>当前页讲稿</strong>
                        <span>{{ speakerNotesCharCount }} 字 · 约 {{ speakerNotesReadMinutes }} 分钟</span>
                      </div>
                      <button type="button" :disabled="!store.currentSlide" :title="store.currentSlide ? '复制当前页讲稿' : '请先选择一页幻灯片'" :aria-label="store.currentSlide ? '复制当前页讲稿' : '请先选择一页幻灯片后再复制讲稿'" @click="copyCurrentSlideNotes">复制</button>
                    </header>
                    <textarea
                      :value="store.currentSlide?.speakerNotes || ''"
                      placeholder="为当前页补充演讲备注、转场提示或口播重点"
                      title="当前页讲稿"
                      aria-label="当前页讲稿"
                      @input="updateCurrentSlideNotes"
                    ></textarea>
                    <div class="ppt-speaker-note-actions">
                      <button type="button" :disabled="!store.currentSlide" :title="store.currentSlide ? '为当前页生成讲稿' : '请先选择一页幻灯片'" :aria-label="store.currentSlide ? '为当前页生成讲稿' : '请先选择一页幻灯片后再生成讲稿'" @click="generateCurrentSlideNotes">生成讲稿</button>
                      <button type="button" :disabled="!store.currentSlide" :title="store.currentSlide ? '优化当前页讲稿表达' : '请先选择一页幻灯片'" :aria-label="store.currentSlide ? '优化当前页讲稿表达' : '请先选择一页幻灯片后再优化表达'" @click="polishCurrentSlideNotes">优化表达</button>
                      <button type="button" :disabled="!store.currentSlide || !currentSpeakerNotes" :title="!store.currentSlide ? '请先选择一页幻灯片' : currentSpeakerNotes ? '清空当前页讲稿' : '当前页暂无讲稿可清空'" :aria-label="!store.currentSlide ? '请先选择一页幻灯片后再清空讲稿' : currentSpeakerNotes ? '清空当前页讲稿' : '当前页暂无讲稿可清空'" @click="clearCurrentSlideNotes">清空</button>
                    </div>
                    <div class="ppt-notes-progress" role="status" aria-live="polite">
                      <span>{{ deckSpeakerNotesCount }} / {{ store.slides.length }} 页已有讲稿</span>
                      <div><i :style="{ width: `${speakerNotesCoverage}%` }"></i></div>
                    </div>
                  </section>
                  <section v-else class="ppt-panel-section ppt-record-panel">
                    <header>
                      <strong>演示录制</strong>
                      <span :class="{ active: recordingWantsToRecord }">{{ recordingWantsToRecord ? "录制准备中" : "未进入录制" }}</span>
                    </header>
                    <div class="ppt-record-preview">
                      <div>
                        <svg class="ppt-record-icon" viewBox="0 0 24 24" aria-hidden="true">
                          <path d="M23 7l-7 5 7 5V7Z" />
                          <rect x="2" y="5" width="14" height="14" rx="2" />
                        </svg>
                      </div>
                      <p>{{ recordingOptionSummary }}</p>
                    </div>
                    <div class="ppt-record-options">
                      <button
                        type="button"
                        :class="{ active: recordingScreenEnabled }"
                        :aria-pressed="recordingScreenEnabled"
                        :title="recordingScreenEnabled ? '关闭屏幕录制' : '开启屏幕录制'"
                        aria-label="切换屏幕录制"
                        @click="recordingScreenEnabled = !recordingScreenEnabled"
                      >
                        <span>屏幕</span>
                        <small>{{ recordingScreenEnabled ? "开启" : "关闭" }}</small>
                      </button>
                      <button
                        type="button"
                        :class="{ active: recordingMicrophoneEnabled }"
                        :aria-pressed="recordingMicrophoneEnabled"
                        :title="recordingMicrophoneEnabled ? '关闭麦克风录制' : '开启麦克风录制'"
                        aria-label="切换麦克风录制"
                        @click="recordingMicrophoneEnabled = !recordingMicrophoneEnabled"
                      >
                        <span>麦克风</span>
                        <small>{{ recordingMicrophoneEnabled ? "开启" : "关闭" }}</small>
                      </button>
                      <button
                        type="button"
                        :class="{ active: recordingCameraEnabled }"
                        :aria-pressed="recordingCameraEnabled"
                        :title="recordingCameraEnabled ? '关闭摄像头录制' : '开启摄像头录制'"
                        aria-label="切换摄像头录制"
                        @click="recordingCameraEnabled = !recordingCameraEnabled"
                      >
                        <span>摄像头</span>
                        <small>{{ recordingCameraEnabled ? "开启" : "关闭" }}</small>
                      </button>
                    </div>
                    <label class="ppt-record-quality">
                      <span>录制清晰度</span>
                      <select v-model="recordingQuality" title="录制清晰度" aria-label="录制清晰度">
                        <option v-for="item in recordingQualityOptions" :key="item.value" :value="item.value">{{ item.label }}</option>
                      </select>
                    </label>
                    <div class="ppt-record-actions">
                      <button
                        type="button"
                        class="is-primary"
                        :disabled="presentModeBusy || isPresentationActionBusy || !store.slides.length"
                        :title="presentationRecordingActionTitle"
                        :aria-label="presentationRecordingActionTitle"
                        :aria-busy="presentModeBusy ? 'true' : undefined"
                        @click="openRecordingPreview"
                      >
                        {{ presentModeBusy ? "正在进入..." : "进入录制预览" }}
                      </button>
                      <button
                        type="button"
                        :disabled="presentModeBusy || isPresentationActionBusy || !store.slides.length"
                        :title="presentationOnlyActionTitle"
                        :aria-label="presentationOnlyActionTitle"
                        :aria-busy="presentModeBusy ? 'true' : undefined"
                        @click="openPresentationOnly"
                      >仅演示</button>
                    </div>
                  </section>
                    </div>
                    <div v-else class="ppt-panel-loading-state" aria-live="polite">
                      <span class="ppt-spinner"></span>
                      <strong>正在打开{{ activeRightPanelMeta.label }}</strong>
                    </div>
                </section>
              </div>

              <div class="ppt-right-utility-rail" aria-label="视图工具">
                <div class="ppt-zoom-control">
                  <button
                    type="button"
                    class="ppt-zoom-value"
                    title="选择缩放比例"
                    aria-label="选择缩放比例"
                    :aria-expanded="showZoomMenu"
                    aria-haspopup="menu"
                    @click="toggleZoomMenu"
                  >
                    {{ Math.round(presentationZoom * 100) }}%
                  </button>
                  <div v-if="showZoomMenu" class="ppt-zoom-menu" role="menu" aria-label="缩放比例">
                    <button
                      v-for="level in presentationZoomLevels"
                      :key="level.value"
                      type="button"
                      role="menuitemradio"
                      :aria-checked="isPresentationZoomValueActive(level.value)"
                      :class="{ active: isPresentationZoomValueActive(level.value) }"
                      :title="`缩放到 ${level.label}`"
                      :aria-label="`缩放到 ${level.label}`"
                      @click="setPresentationZoom(level.value)"
                    >
                      <span>{{ level.label }}</span>
                      <svg v-if="isPresentationZoomValueActive(level.value)" class="ppt-check-icon" viewBox="0 0 24 24" aria-hidden="true">
                        <path d="M20 6 9 17l-5-5" />
                      </svg>
                    </button>
                    <div class="ppt-menu-separator" />
                    <button
                      type="button"
                      role="menuitemradio"
                      :aria-checked="isPresentationZoomValueActive(fitPresentationZoomValue)"
                      :class="{ active: isPresentationZoomValueActive(fitPresentationZoomValue) }"
                      title="适配画布"
                      aria-label="适配画布"
                      @click="fitPresentationZoom"
                    >
                      <span>适配画布</span>
                      <svg v-if="isPresentationZoomValueActive(fitPresentationZoomValue)" class="ppt-check-icon" viewBox="0 0 24 24" aria-hidden="true">
                        <path d="M20 6 9 17l-5-5" />
                      </svg>
                    </button>
                  </div>
                </div>
                <div class="ppt-help-control">
                  <button
                    type="button"
                    title="帮助"
                    aria-label="打开帮助菜单"
                    :aria-expanded="showPresentationHelpMenu"
                    aria-haspopup="menu"
                    @click="togglePresentationHelpMenu"
                  >
                    ?
                  </button>
                  <div v-if="showPresentationHelpMenu" class="ppt-help-menu" role="menu" aria-label="帮助菜单">
                    <button type="button" role="menuitem" title="打开键盘快捷键" aria-label="打开键盘快捷键" @click="openKeyboardShortcuts">
                      <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                        <rect x="3" y="5" width="18" height="14" rx="2" />
                        <path d="M7 9h.01M11 9h.01M15 9h.01M19 9h.01M7 13h10" />
                      </svg>
                      <span>键盘快捷键</span>
                    </button>
                    <button type="button" role="menuitem" title="打开帮助中心" aria-label="打开帮助中心" @click="openReservedHelpCenter">
                      <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                        <circle cx="12" cy="12" r="10" />
                        <path d="M9.1 9a3 3 0 1 1 5.8 1c-.5 1-1.5 1.5-2.2 2.1-.5.4-.7.8-.7 1.4" />
                        <path d="M12 17h.01" />
                      </svg>
                      <span>帮助中心</span>
                    </button>
                    <div class="ppt-menu-separator" />
                    <p>知启云 AI PPT 编辑器</p>
                  </div>
                </div>
              </div>
            </aside>
          </section>

          <section v-else class="ppt-presentation-empty-panel" aria-labelledby="ppt-empty-slide-title">
            <article class="ppt-presentation-empty-canvas">
              <div class="ppt-presentation-empty-frame" aria-hidden="true"></div>
              <div class="ppt-presentation-empty-content">
                <span class="ppt-presentation-empty-icon">
                  <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                    <rect x="3" y="4" width="18" height="14" rx="2" />
                    <path d="M8 21h8" />
                    <path d="M12 18v3" />
                  </svg>
                </span>
                <div>
                  <h2 id="ppt-empty-slide-title">暂无幻灯片</h2>
                  <p>从第一页开始创建这个演示文稿。</p>
                </div>
                <button
                  type="button"
                  class="ppt-presentation-empty-action"
                  :disabled="isPresentationActionBusy"
                  :title="isPresentationActionBusy ? '正在生成中，请稍候' : '添加第一页'"
                  :aria-label="isPresentationActionBusy ? '正在生成中，请稍候' : '添加第一页'"
                  :aria-busy="isPresentationActionBusy ? 'true' : undefined"
                  @click="addFirstPresentationSlide"
                >
                  <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                    <path d="M5 12h14" />
                    <path d="M12 5v14" />
                  </svg>
                  <span>添加第一页</span>
                </button>
              </div>
            </article>
          </section>

          <div v-if="showExportDialog" class="ppt-export-dialog-overlay" role="presentation" @click.self="closeExportDialog" @keydown.esc.stop.prevent="closeExportDialog">
            <section class="ppt-export-dialog" role="dialog" aria-modal="true" aria-labelledby="ppt-export-title" aria-describedby="ppt-export-description" :aria-busy="exportBusy">
              <header>
                <h2 id="ppt-export-title">导出演示文稿</h2>
                <p id="ppt-export-description">导出为标准 PowerPoint 文件。系统会根据当前页面内容生成 .pptx 文件并触发浏览器下载。</p>
              </header>
              <div class="ppt-export-options" role="radiogroup" aria-label="导出格式">
                <div
                  class="ppt-export-option active"
                  role="radio"
                  aria-checked="true"
                  aria-label="PowerPoint pptx"
                  title="PowerPoint (.pptx)"
                >
                  <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                    <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
                    <path d="M14 2v6h6" />
                    <path d="M8 13h8" />
                    <path d="M8 17h5" />
                  </svg>
                  <span>
                    <b>PowerPoint (.pptx)</b>
                    <small>标准 PowerPoint 文件</small>
                  </span>
                </div>
              </div>
              <div v-if="exportBusy || presentationExportPhase !== 'idle'" class="ppt-export-progress" role="status" aria-live="polite">
                <div
                  v-for="step in presentationExportSteps"
                  :key="step.value"
                  :class="exportStepState(step.value)"
                  :aria-label="`${step.label}：${step.description}`"
                >
                  <i>
                    <svg v-if="exportStepState(step.value) === 'done' || presentationExportPhase === 'complete'" class="ppt-check-icon" viewBox="0 0 24 24" aria-hidden="true">
                      <path d="M20 6 9 17l-5-5" />
                    </svg>
                    <span v-else-if="exportStepState(step.value) === 'active'" class="ppt-spinner"></span>
                    <b v-else></b>
                  </i>
                  <span>
                    <strong>{{ step.label }}</strong>
                    <small>{{ step.description }}</small>
                  </span>
                </div>
                <a
                  v-if="presentationExportDownloadUrl"
                  class="ppt-export-download-link"
                  :href="presentationExportDownloadUrl"
                  :download="presentationExportDownloadName"
                  :title="`重新下载 ${presentationExportDownloadName}`"
                  :aria-label="`重新下载 ${presentationExportDownloadName}`"
                >
                  如果下载没有自动开始，点击这里重新下载
                </a>
              </div>
              <footer>
                <button type="button" class="ppt-dialog-secondary" :disabled="exportBusy" title="取消导出" aria-label="取消导出" @click="closeExportDialog">取消</button>
                <button ref="exportPrimaryButtonRef" type="button" class="ppt-dialog-primary" :disabled="exportBusy" :aria-busy="exportBusy" :title="exportBusy ? '正在导出演示文稿' : '导出为 PowerPoint'" :aria-label="exportBusy ? '正在导出演示文稿' : '导出为 PowerPoint'" @click="handleExportCurrent('pptx')">
                  <span v-if="exportBusy" class="ppt-spinner"></span>
                  <span>{{ exportBusy ? "正在导出..." : "导出为 PowerPoint" }}</span>
                </button>
              </footer>
            </section>
          </div>

          <div v-if="showShareDialog" class="ppt-export-dialog-overlay" role="presentation" @click.self="closeShareDialog" @keydown.esc.stop.prevent="closeShareDialog">
            <section class="ppt-share-dialog" role="dialog" aria-modal="true" aria-labelledby="ppt-share-title" aria-describedby="ppt-share-description">
              <header>
                <h2 id="ppt-share-title">分享演示文稿</h2>
                <p id="ppt-share-description">复制当前演示文稿链接。权限接口后续接入前，先保留当前账号可编辑的本地链接。</p>
              </header>
              <div class="ppt-share-link-card">
                <span>演示文稿链接</span>
                <div>
                  <input :value="presentationShareUrl" readonly title="演示文稿分享链接" aria-label="演示文稿分享链接" />
                  <button ref="shareCopyButtonRef" type="button" title="复制演示文稿分享链接" aria-label="复制演示文稿分享链接" @click="copyPresentationShareUrl">复制链接</button>
                </div>
              </div>
              <div class="ppt-share-permission">
                <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                  <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10Z" />
                  <path d="m9 12 2 2 4-4" />
                </svg>
                <span>
                  <b>当前账号可编辑</b>
                  <small>只读分享、团队协作和公开访问后续接入权限服务。</small>
                </span>
              </div>
              <footer>
                <button type="button" class="ppt-dialog-secondary" title="关闭分享设置" aria-label="关闭分享设置" @click="closeShareDialog">关闭</button>
                <button type="button" class="ppt-dialog-primary" title="复制演示文稿分享链接" aria-label="复制演示文稿分享链接" @click="copyPresentationShareUrl">复制分享链接</button>
              </footer>
            </section>
          </div>

          <div v-if="showKeyboardShortcutsDialog" class="ppt-export-dialog-overlay" role="presentation" @click.self="closeKeyboardShortcuts" @keydown.esc.stop.prevent="closeKeyboardShortcuts">
            <section class="ppt-help-dialog" role="dialog" aria-modal="true" aria-labelledby="ppt-shortcuts-title" aria-describedby="ppt-shortcuts-description">
              <header>
                <h2 id="ppt-shortcuts-title">键盘快捷键</h2>
                <p id="ppt-shortcuts-description">演示和编辑常用操作。输入框聚焦时不会触发翻页快捷键。</p>
              </header>
              <div class="ppt-shortcut-list">
                <span>退出演示</span><kbd>Esc</kbd>
                <span>下一页</span><kbd>→ / ↓ / Space</kbd>
                <span>上一页</span><kbd>← / ↑ / Shift + Space</kbd>
                <span>第一页 / 最后一页</span><kbd>Home / End</kbd>
                <span>撤销 / 重做</span><kbd>Ctrl + Z / Ctrl + Y</kbd>
              </div>
              <footer>
                <button ref="shortcutsCloseButtonRef" type="button" class="ppt-dialog-primary" title="关闭键盘快捷键" aria-label="关闭键盘快捷键" @click="closeKeyboardShortcuts">知道了</button>
              </footer>
            </section>
          </div>
        </section>

        <template v-if="isHomeWorkspace">
        <section class="ppt-library-toolbar">
          <nav class="ppt-library-tabs" aria-label="PPT 记录筛选">
            <button
              v-for="item in libraryFilters"
              :key="item.value"
              type="button"
              :title="`查看${item.label}演示文稿`"
              :aria-label="`查看${item.label}演示文稿${libraryFilter === item.value ? '，当前筛选' : ''}`"
              :aria-pressed="libraryFilter === item.value"
              :class="{ active: libraryFilter === item.value }"
              @click="libraryFilter = item.value"
            >
              <svg class="ppt-tab-icon" viewBox="0 0 24 24" aria-hidden="true">
                <template v-if="item.value === 'all'">
                  <rect width="20" height="5" x="2" y="3" rx="1" />
                  <path d="M4 8v11a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8" />
                  <path d="M10 12h4" />
                </template>
                <template v-else-if="item.value === 'recent'">
                  <path d="M12 6v6h4" />
                  <circle cx="12" cy="12" r="10" />
                </template>
                <path
                  v-else
                  d="M11.525 2.295a.53.53 0 0 1 .95 0l2.31 4.679a2.123 2.123 0 0 0 1.595 1.16l5.166.756a.53.53 0 0 1 .294.904l-3.736 3.638a2.123 2.123 0 0 0-.611 1.878l.882 5.14a.53.53 0 0 1-.771.56l-4.618-2.428a2.122 2.122 0 0 0-1.973 0L6.396 21.01a.53.53 0 0 1-.77-.56l.881-5.139a2.122 2.122 0 0 0-.611-1.879L2.16 9.795a.53.53 0 0 1 .294-.906l5.165-.755a2.122 2.122 0 0 0 1.597-1.16z"
                />
              </svg>
              <span>{{ item.label }}</span>
            </button>
          </nav>

          <div class="ppt-library-actions">
            <div class="ppt-history-search" :class="{ open: shouldShowHistorySearchInput }">
              <button
                type="button"
                class="ppt-icon-button ppt-search-trigger"
                :class="{ hidden: shouldShowHistorySearchInput }"
                title="搜索"
                aria-label="搜索文件"
                @click="openHistorySearch"
              >
                <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                  <path d="m21 21-4.34-4.34" />
                  <circle cx="11" cy="11" r="8" />
                </svg>
                <span class="ppt-sr-only">搜索文件</span>
              </button>
              <div class="ppt-history-search-box" :class="{ open: shouldShowHistorySearchInput }">
                <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                  <path d="m21 21-4.34-4.34" />
                  <circle cx="11" cy="11" r="8" />
                </svg>
                <input
                  ref="historySearchInputRef"
                  v-model="historySearchQuery"
                  type="text"
                  title="搜索文件"
                  aria-label="搜索文件"
                  placeholder="搜索"
                />
                <button type="button" title="关闭搜索" aria-label="关闭搜索" @click="closeHistorySearch">
                  <svg class="ppt-toolbar-icon small" viewBox="0 0 24 24" aria-hidden="true">
                    <path d="M18 6 6 18" />
                    <path d="m6 6 12 12" />
                  </svg>
                </button>
              </div>
            </div>
            <button
              type="button"
              class="ppt-create-button"
              title="创建新的演示文稿"
              aria-label="创建新的演示文稿"
              @click="handleCreateNewFromLibrary"
            >
              <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                <path d="M5 12h14" />
                <path d="M12 5v14" />
              </svg>
              <span>创建新的</span>
            </button>
            <div class="ppt-filter-dropdown">
              <button
                type="button"
                class="ppt-icon-button ppt-filter-button"
                :class="{ active: showHistoryFilterMenu || activeHistoryFiltersCount > 0 }"
                title="排序和筛选"
                aria-label="排序和筛选文件"
                :aria-expanded="showHistoryFilterMenu"
                aria-haspopup="menu"
                @click="toggleHistoryFilterMenu"
              >
                <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                  <line x1="21" x2="14" y1="4" y2="4" />
                  <line x1="10" x2="3" y1="4" y2="4" />
                  <line x1="21" x2="12" y1="12" y2="12" />
                  <line x1="8" x2="3" y1="12" y2="12" />
                  <line x1="21" x2="16" y1="20" y2="20" />
                  <line x1="12" x2="3" y1="20" y2="20" />
                  <line x1="14" x2="14" y1="2" y2="6" />
                  <line x1="8" x2="8" y1="10" y2="14" />
                  <line x1="16" x2="16" y1="18" y2="22" />
                </svg>
                <span v-if="activeHistoryFiltersCount > 0" class="ppt-filter-count">{{ activeHistoryFiltersCount }}</span>
              </button>
              <div v-if="showHistoryFilterMenu" class="ppt-history-filter-menu" role="menu" aria-label="排序和筛选文件">
                <p>排序</p>
                <button
                  v-for="option in historySortOptions"
                  :key="option.value"
                  type="button"
                  role="menuitemradio"
                  :aria-checked="historySortBy === option.value"
                  :title="`按${option.label}排序`"
                  :aria-label="`按${option.label}排序${historySortBy === option.value ? '，已选' : ''}`"
                  @click="selectHistorySort(option.value)"
                >
                  <span>
                    <svg class="ppt-toolbar-icon" viewBox="0 0 24 24" aria-hidden="true">
                      <template v-if="option.icon === 'clock'">
                        <path d="M12 6v6h4" />
                        <circle cx="12" cy="12" r="10" />
                      </template>
                      <template v-else>
                        <path d="M12 3v18" />
                        <path d="M3 12h18" />
                        <rect x="3" y="3" width="18" height="18" rx="2" />
                      </template>
                    </svg>
                    {{ option.label }}
                  </span>
                  <svg v-if="historySortBy === option.value" class="ppt-check-icon" viewBox="0 0 24 24" aria-hidden="true">
                    <path d="M20 6 9 17l-5-5" />
                  </svg>
                </button>
                <div class="ppt-menu-separator" />
                <p>筛选</p>
                <button
                  type="button"
                  role="menuitemcheckbox"
                  :aria-checked="showFavoritesOnlyFilter"
                  :title="showFavoritesOnlyFilter ? '取消仅看收藏' : '仅看收藏'"
                  :aria-label="showFavoritesOnlyFilter ? '取消仅看收藏' : '仅看收藏'"
                  @click="toggleFavoritesOnlyFilter"
                >
                  <span>
                    <svg class="ppt-toolbar-icon ppt-star-filter-icon" :class="{ active: showFavoritesOnlyFilter }" viewBox="0 0 24 24" aria-hidden="true">
                      <path d="M11.525 2.295a.53.53 0 0 1 .95 0l2.31 4.679a2.123 2.123 0 0 0 1.595 1.16l5.166.756a.53.53 0 0 1 .294.904l-3.736 3.638a2.123 2.123 0 0 0-.611 1.878l.882 5.14a.53.53 0 0 1-.771.56l-4.618-2.428a2.122 2.122 0 0 0-1.973 0L6.396 21.01a.53.53 0 0 1-.77-.56l.881-5.139a2.122 2.122 0 0 0-.611-1.879L2.16 9.795a.53.53 0 0 1 .294-.906l5.165-.755a2.122 2.122 0 0 0 1.597-1.16z" />
                    </svg>
                    仅看收藏
                  </span>
                  <svg v-if="showFavoritesOnlyFilter" class="ppt-check-icon" viewBox="0 0 24 24" aria-hidden="true">
                    <path d="M20 6 9 17l-5-5" />
                  </svg>
                </button>
                <div class="ppt-menu-separator" />
                <p>类型</p>
                <button
                  v-for="option in historyTypeOptions"
                  :key="option.value"
                  type="button"
                  role="menuitemradio"
                  :aria-checked="historyTypeFilter === option.value"
                  :title="`筛选类型：${option.label}`"
                  :aria-label="`筛选类型：${option.label}${historyTypeFilter === option.value ? '，已选' : ''}`"
                  @click="selectHistoryTypeFilter(option.value)"
                >
                  <span>{{ option.label }}</span>
                  <svg v-if="historyTypeFilter === option.value" class="ppt-check-icon" viewBox="0 0 24 24" aria-hidden="true">
                    <path d="M20 6 9 17l-5-5" />
                  </svg>
                </button>
              </div>
            </div>
            <div class="ppt-view-toggle">
              <button
                type="button"
                :class="{ active: historyViewMode === 'grid' }"
                title="切换为网格视图"
                :aria-label="historyViewMode === 'grid' ? '网格视图，当前视图' : '切换为网格视图'"
                :aria-pressed="historyViewMode === 'grid'"
                @click="historyViewMode = 'grid'"
              >
                <svg class="ppt-view-icon" viewBox="0 0 24 24" aria-hidden="true">
                  <path d="M12 3v18" />
                  <path d="M3 12h18" />
                  <rect x="3" y="3" width="18" height="18" rx="2" />
                </svg>
                <span>网格</span>
              </button>
              <button
                type="button"
                :class="{ active: historyViewMode === 'list' }"
                title="切换为列表视图"
                :aria-label="historyViewMode === 'list' ? '列表视图，当前视图' : '切换为列表视图'"
                :aria-pressed="historyViewMode === 'list'"
                @click="historyViewMode = 'list'"
              >
                <svg class="ppt-view-icon" viewBox="0 0 24 24" aria-hidden="true">
                  <path d="M3 12h.01" />
                  <path d="M3 18h.01" />
                  <path d="M3 6h.01" />
                  <path d="M8 12h13" />
                  <path d="M8 18h13" />
                  <path d="M8 6h13" />
                </svg>
                <span>列表</span>
              </button>
            </div>
          </div>
        </section>

        <section class="ppt-workflow-board" v-if="store.outline || store.slides.length">
          <article class="ppt-board-panel ppt-outline-panel">
            <PptOutlineGenerator
              :can-generate="store.canGenerateOutline"
              :status="store.status"
              @generate="store.generateOutlineFlow"
            />
            <PptOutlineEditor
              v-if="store.outline"
              :outline="store.outline"
              :status="store.status"
              @update-title="updateOutlineTitle"
              @update-slide="store.updateOutlineSlide"
              @add-slide="store.addOutlineSlide"
              @delete-slide="store.deleteOutlineSlide"
              @move-slide="store.moveOutlineSlide"
              @regenerate-slide="store.regenerateOutlineSlide"
              @regenerate-all="store.regenerateAllOutline"
              @save="store.saveOutline"
              @confirm="store.confirmOutlineAndGeneratePpt"
            />
          </article>

          <article class="ppt-board-panel">
            <PptSlidePreview
              v-if="store.slides.length"
              :slides="store.slides"
              :current-index="store.currentSlideIndex"
              :theme="store.theme"
              @select="store.selectSlide"
              @prev="store.selectSlide(Math.max(0, store.currentSlideIndex - 1))"
              @next="store.selectSlide(Math.min(store.slides.length - 1, store.currentSlideIndex + 1))"
              @fullscreen="ElMessage.info('全屏预览入口已预留')"
              @regenerate="store.regenerateCurrentSlide"
            />
            <PptEmptyState
              v-else
              title="等待生成预览"
              description="确认大纲后会在这里展示幻灯片预览。"
            />
          </article>
        </section>

        <section class="ppt-editor-board" v-if="store.slides.length">
          <article class="ppt-board-panel">
            <PptSlideEditor
              :slide="store.currentSlide"
              @save="handleSlideSave"
              @cancel="ElMessage.info('已取消本次编辑')"
              @regenerate="store.regenerateCurrentSlide"
            />
          </article>
          <article class="ppt-board-panel">
            <PptImageSourcePanel
              :image-source="store.imageSource"
              :image-model="store.imageModel"
              :image-models="store.imageModels"
              :current-slide="store.currentSlide"
              :results="store.imageSearchResults"
              :generating="store.imageGenerating"
              :visual-operation-status="store.visualOperationStatus"
              @update:image-source="store.imageSource = $event"
              @update:image-model="store.imageModel = $event"
              @generate-image="store.generateImageForCurrentSlide"
              @update-visual-plan="store.updateCurrentSlideVisualPlan"
              @delete-visual="store.deleteCurrentSlideVisual"
              @restore-visual="store.restoreCurrentSlideVisual"
              @search-images="store.searchImages"
              @apply-image="store.applyImageToCurrentSlide"
            />
          </article>
        </section>

        <section class="ppt-library-panel">
          <PptHistoryList
            :history="filteredHistory"
            :view-mode="historyViewMode"
            :favorite-task-ids="favoriteTaskIds"
            :empty-title="historyEmptyTitle"
            :empty-description="historyEmptyDescription"
            @preview="handlePreviewHistory"
            @edit="handleEditHistory"
            @download-ppt="handleDownloadPpt"
            @download-pdf="handleDownloadPdf"
            @regenerate="handleRegenerateHistory"
            @toggle-favorite="toggleFavorite"
            @delete="handleDeleteHistory"
          />
        </section>
        </template>
        <aside v-if="isHomeWorkspace" class="ppt-home-side-panel" aria-label="PPT 热门模板与创作灵感">
          <section class="ppt-home-side-card">
            <header class="ppt-home-side-head">
              <h2>热门模板</h2>
              <button type="button" title="查看更多模板" aria-label="查看更多模板" @click="showConfig = true">
                <span>更多模板</span>
                <svg class="ppt-toolbar-icon small" viewBox="0 0 24 24" aria-hidden="true">
                  <path d="m9 18 6-6-6-6" />
                </svg>
              </button>
            </header>
            <div class="ppt-home-template-grid">
              <button
                v-for="template in pptHomeTemplates"
                :key="template.title"
                type="button"
                class="ppt-home-template-card"
                :title="`使用模板：${template.title}`"
                :aria-label="`使用模板：${template.title}`"
                @click="applyHomeTemplate(template)"
              >
                <span class="ppt-home-template-cover" :style="{ '--template-accent': template.accent }">
                  <b>{{ template.tag }}</b>
                  <em>{{ template.subtitle }}</em>
                </span>
                <strong>{{ template.title }}</strong>
              </button>
            </div>
          </section>

          <section class="ppt-home-side-card">
            <header class="ppt-home-side-head">
              <h2>创作灵感</h2>
              <button type="button" title="换一批创作灵感" aria-label="换一批创作灵感" @click="rotatePptInspirations">
                <svg class="ppt-toolbar-icon small" viewBox="0 0 24 24" aria-hidden="true">
                  <path d="M21 12a9 9 0 0 1-15.5 6.2" />
                  <path d="M3 12a9 9 0 0 1 15.5-6.2" />
                  <path d="M18 3v4h-4" />
                  <path d="M6 21v-4h4" />
                </svg>
                <span>换一批</span>
              </button>
            </header>
            <div class="ppt-home-inspiration-list">
              <button
                v-for="item in visiblePptHomeInspirations"
                :key="item.title"
                type="button"
                class="ppt-home-inspiration-item"
                :title="`使用灵感：${item.title}`"
                :aria-label="`使用灵感：${item.title}`"
                @click="applyHomeInspiration(item)"
              >
                <span class="ppt-home-inspiration-icon">{{ item.icon }}</span>
                <strong>{{ item.title }}</strong>
                <svg class="ppt-toolbar-icon small" viewBox="0 0 24 24" aria-hidden="true">
                  <path d="m9 18 6-6-6-6" />
                </svg>
              </button>
            </div>
          </section>
        </aside>
      </main>
    </div>
    <PptCreateThemeModal
      v-if="presentationThemeCreatorOpen"
      v-model:open="presentationThemeCreatorOpen"
      :base-theme="store.theme"
      :current-slides="store.slides"
      :is-customizing="presentationThemeCreatorIsCustomizing"
      @apply-theme="applyPresentationThemeFromCreator"
      @saved="handlePresentationThemeCreatorSaved"
    />
  </section>
</template>

<script setup lang="ts">
import { computed, defineAsyncComponent, nextTick, onBeforeUnmount, onMounted, ref, watch, type CSSProperties } from "vue";
import { ElMessage } from "element-plus/es/components/message/index";
import { ElMessageBox } from "element-plus/es/components/message-box/index";
import { pptThemes, pptThemeLabel } from "../../config/pptThemes";
import { usePptStore } from "../../stores/ppt";
import type {
  PptAudience,
  PptGenerationAspectRatio,
  PptHistoryItem,
  PptLanguage,
  PptModelOption,
  PptScenario,
  PptSlide,
  PptSlideLayout,
  PptTextContent,
  PptTheme,
  PptTone
} from "../../types/ppt";
import PptEmptyState from "./PptEmptyState.vue";
import PptErrorState from "./PptErrorState.vue";
import PptExamplePrompts from "./PptExamplePrompts.vue";
import PptGenerationProgress from "./PptGenerationProgress.vue";
import PptHistoryList from "./PptHistoryList.vue";
import PptPromptInput from "./PptPromptInput.vue";
import xianzhiLogo from "../../assets/xianzhi-ai-logo.webp";

const PptAgentActivityInline = defineAsyncComponent(() => import("./PptAgentActivityInline.vue"));
const PptCreateModeSelector = defineAsyncComponent(() => import("./PptCreateModeSelector.vue"));
const PptCreateThemeModal = defineAsyncComponent(() => import("./PptCreateThemeModal.vue"));
const PptGenerationConfigPanel = defineAsyncComponent(() => import("./PptGenerationConfigPanel.vue"));
const PptImageSourcePanel = defineAsyncComponent(() => import("./PptImageSourcePanel.vue"));
const PptOutlineEditor = defineAsyncComponent(() => import("./PptOutlineEditor.vue"));
const PptOutlineGenerator = defineAsyncComponent(() => import("./PptOutlineGenerator.vue"));
const PptSlideEditor = defineAsyncComponent(() => import("./PptSlideEditor.vue"));
const PptSlidePreview = defineAsyncComponent(() => import("./PptSlidePreview.vue"));
const PptThemeSelector = defineAsyncComponent(() => import("./PptThemeSelector.vue"));
const PptThemeSettingsPanel = defineAsyncComponent(() => import("./PptThemeSettingsPanel.vue"));

type LibraryFilter = "all" | "recent" | "favorites";
type HistoryViewMode = "grid" | "list";
type HistorySortBy = "date-desc" | "date-asc" | "name-asc" | "name-desc";
type HistoryTypeFilter = "all" | "presentation";
type PptHomeTemplate = {
  title: string;
  subtitle: string;
  tag: string;
  accent: string;
  prompt: string;
};
type PptHomeInspiration = {
  title: string;
  icon: string;
  prompt: string;
};
type PresentationRightPanel = "content" | "agent" | "elements" | "charts" | "diagrams" | "embed" | "theme" | "layout" | "background" | "globalSettings" | "iconPicker" | "images" | "notes" | "record";
type PresentationToolIcon = "text" | "agent" | "blocks" | "chart" | "diagram" | "embed" | "theme" | "layout" | "background" | "settings" | "sparkles" | "image" | "notes" | "record";
type SlideInsertPosition = "before" | "after";
type SlideMagicAction = "layout" | "writing" | "spelling" | "translate" | "simplify" | "visual" | "image";
type PresentationElementKind = "title" | "content" | "bullets" | "notes";
type PresentationElementAlign = "left" | "center" | "right";
type PresentationSaveStatus = "idle" | "saving" | "saved" | "error";
type PresentationElementStyle = {
  align?: PresentationElementAlign;
  color?: string;
  emphasis?: boolean;
};
type PresentationPanelCategoryOption = {
  label: string;
  value: string;
};
type TextModelGroup = {
  label: string;
  models: PptModelOption[];
};
type GenerationStepState = "pending" | "active" | "done" | "failed";
type InsertablePresentationBlock = {
  title: string;
  description: string;
  content: string;
  bulletPoints: string[];
  category: string;
  icon: string;
  iconLabel: string;
  keywords?: string[];
};
type PresentationPaletteSelection = {
  slideId: string;
  panel: PresentationRightPanel;
  title: string;
  content: string;
  bulletPoints: string[];
};
type PresentationAgentMessage = {
  role: "assistant" | "user";
  content: string;
};
type PresentationBackgroundMode = "solid" | "linear" | "radial" | "image";
type PresentationGlobalSettingsTab = "cards" | "theme" | "background";
type PresentationDeckWidth = "compact" | "standard" | "wide";
type PresentationTypographyScale = "small" | "medium" | "large";
type PresentationExportPhase = "idle" | "scanning" | "generating" | "downloading" | "complete";
type PresentPhoneGesture = {
  pointerId: number;
  startX: number;
  startY: number;
};
type PresentationEditorSnapshot = {
  slides: PptSlide[];
  slideBackgrounds: Record<string, string>;
  elementStyles: Record<string, Partial<Record<PresentationElementKind, PresentationElementStyle>>>;
  deckWidth: PresentationDeckWidth;
  globalAlignment: PresentationElementAlign;
  typographyScale: PresentationTypographyScale;
  theme: PptTheme;
  backgroundMode: PresentationBackgroundMode;
  backgroundImageUrl: string;
  currentSlideIndex: number;
};

const slideLayoutCycle: PptSlideLayout[] = ["cover", "section", "content", "imageText", "summary"];
const presentationPanelArrowKeys = new Set(["ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown", "Home", "End"]);

const store = usePptStore();
const favoriteHistoryStorageKey = "xianzhi_ppt_generation_favorites";
const showConfig = ref(false);
const showSlideCountMenu = ref(false);
const showFormatMenu = ref(false);
const showLanguageMenu = ref(false);
const showMoreMenu = ref(false);
const showModelMenu = ref(false);
const showHistoryFilterMenu = ref(false);
const libraryFilter = ref<LibraryFilter>("recent");
const historyViewMode = ref<HistoryViewMode>("grid");
const historySortBy = ref<HistorySortBy>("date-desc");
const historyTypeFilter = ref<HistoryTypeFilter>("all");
const historySearchQuery = ref("");
const isHistorySearchOpen = ref(false);
const showFavoritesOnlyFilter = ref(false);
const historySearchInputRef = ref<HTMLInputElement | null>(null);
const favoriteTaskIds = ref<string[]>([]);
const pptInspirationPage = ref(0);
const isGenerationWorkspace = ref(false);
const isGenerationSettingsExpanded = ref(false);
const isPresentationWorkspace = ref(false);
const activeGenerationId = ref("");
const activePresentationId = ref("");
const generationWorkspaceRef = ref<HTMLElement | null>(null);
const presentationWorkspaceRef = ref<HTMLElement | null>(null);
const presentationRightPanelContentRef = ref<HTMLElement | null>(null);
const presentationTitleInputRef = ref<HTMLInputElement | null>(null);
const exportPrimaryButtonRef = ref<HTMLButtonElement | null>(null);
const shareCopyButtonRef = ref<HTMLButtonElement | null>(null);
const shortcutsCloseButtonRef = ref<HTMLButtonElement | null>(null);
const presentationViewMode = ref<"edit" | "present">("edit");
const presentationRightPanel = ref<PresentationRightPanel>("content");
const presentationRightPanelOpen = ref(false);
const presentationLoadedRightPanel = ref<PresentationRightPanel | null>("content");
const presentationPanelSearchQuery = ref("");
const presentationPanelCategory = ref("all");
const showCurrentSlideTextEditor = ref(false);
const showEmbedUrlCard = ref(false);
const collapsedPresentationDiagramCategories = ref<string[]>([]);
const presentationBackgroundMode = ref<PresentationBackgroundMode>("solid");
const presentationBackgroundImageUrl = ref("");
const presentationSlideBackgrounds = ref<Record<string, string>>({});
const presentationGlobalSettingsTab = ref<PresentationGlobalSettingsTab>("cards");
const presentationDeckWidth = ref<PresentationDeckWidth>("standard");
const presentationGlobalAlignment = ref<PresentationElementAlign>("left");
const presentationTypographyScale = ref<PresentationTypographyScale>("medium");
const presentationIconSearchQuery = ref("");
const presentationIconSearchInputRef = ref<HTMLInputElement | null>(null);
const presentationSlideIcons = ref<Record<string, string>>({});
const presentationAgentPrompt = ref("");
const presentationAgentBusy = ref(false);
const presentationAgentMessages = ref<PresentationAgentMessage[]>([
  { role: "assistant", content: "我是 PPT 助手。你可以让我优化当前页标题、精简文案、补充要点或给出视觉建议。" }
]);
const presentationSidebarCollapsed = ref(false);
const presentationSidebarWidth = ref(150);
const presentationSidebarResizing = ref(false);
const showPresentationMenu = ref(false);
const showExportDialog = ref(false);
const showShareDialog = ref(false);
const showZoomMenu = ref(false);
const showPresentationHelpMenu = ref(false);
const showKeyboardShortcutsDialog = ref(false);
const showGenerationHelpMenu = ref(false);
const showGenerationKeyboardShortcutsDialog = ref(false);
const presentationThemeCreatorOpen = ref(false);
const presentationThemeCreatorIsCustomizing = ref(false);
const showElementTypeMenu = ref(false);
const showElementAiEditor = ref(false);
const elementAiPrompt = ref("");
const elementAiBusy = ref(false);
const elementAiError = ref("");
const elementAiPromptInputRef = ref<HTMLTextAreaElement | null>(null);
const recordingWantsToRecord = ref(false);
const recordingScreenEnabled = ref(true);
const recordingMicrophoneEnabled = ref(true);
const recordingCameraEnabled = ref(false);
const recordingQuality = ref("1080p");
const exportBusy = ref(false);
const presentationExportPhase = ref<PresentationExportPhase>("idle");
const presentationExportDownloadUrl = ref("");
const presentationExportDownloadName = ref("");
const presentModeBusy = ref(false);
const presentModeHeaderVisible = ref(true);
const slideMoreMenuIndex = ref<number | null>(null);
const slidePaletteMenuIndex = ref<number | null>(null);
const slideMagicMenuIndex = ref<number | null>(null);
const slideMagicPrompt = ref("");
const slideMagicBusy = ref(false);
const slideMagicError = ref("");
const slideMagicPromptInputRef = ref<HTMLInputElement | null>(null);
const presentationZoom = ref(0.92);
const presentationViewportSize = ref({ width: 0, height: 0 });
const presentationSaveStatus = ref<PresentationSaveStatus>("saved");
const presentationEmbedUrl = ref("");
const selectedPresentationElement = ref<{ slideIndex: number; kind: PresentationElementKind } | null>(null);
const selectedPresentationPaletteBlock = ref<PresentationPaletteSelection | null>(null);
const presentationElementStyles = ref<Record<string, Partial<Record<PresentationElementKind, PresentationElementStyle>>>>({});
const presentationUndoStack = ref<PresentationEditorSnapshot[]>([]);
const presentationRedoStack = ref<PresentationEditorSnapshot[]>([]);
const presentationSidebarWidthStorageKey = "xianzhi_ppt_sidebar_width";
const presentationSidebarCollapsedStorageKey = "xianzhi_ppt_sidebar_collapsed";
let presentationSidebarResizeStartX = 0;
let presentationSidebarResizeStartWidth = 150;
let presentModeWheelLastAt = 0;
let presentPhoneGesture: PresentPhoneGesture | null = null;
let elementAiRequestToken = 0;
let presentationSaveTimer: ReturnType<typeof window.setTimeout> | null = null;
let presentationPanelLoadTimer: ReturnType<typeof window.setTimeout> | null = null;
let generationDraftSaveTimer: ReturnType<typeof window.setTimeout> | null = null;
const presentTapDistanceThreshold = 12;
const presentSwipeDistanceThreshold = 56;
const slideCountOptions = Array.from({ length: 12 }, (_, index) => index + 1);
const fitPresentationZoomValue = 0.92;
const presentationZoomLevels = [
  { value: 1.8, label: "180%" },
  { value: 1.7, label: "170%" },
  { value: 1.6, label: "160%" },
  { value: 1.5, label: "150%" },
  { value: 1.4, label: "140%" },
  { value: 1.3, label: "130%" },
  { value: 1.2, label: "120%" },
  { value: 1.1, label: "110%" },
  { value: 1, label: "100%" },
  { value: 0.9, label: "90%" },
  { value: 0.8, label: "80%" },
  { value: 0.7, label: "70%" },
  { value: 0.6, label: "60%" },
  { value: 0.5, label: "50%" }
] satisfies Array<{ value: number; label: string }>;
const recordingQualityOptions = [
  { value: "720p", label: "720p" },
  { value: "1080p", label: "1080p" },
  { value: "source", label: "跟随画布" }
] satisfies Array<{ value: string; label: string }>;
const presentationExportSteps = [
  { value: "scanning", label: "扫描幻灯片", description: "读取当前画布里的页面内容和顺序。" },
  { value: "generating", label: "生成文件", description: "调用后端导出接口，打包为标准 PowerPoint 文件。" },
  { value: "downloading", label: "准备下载", description: "触发浏览器下载，并保留临时下载入口。" }
] satisfies Array<{ value: PresentationExportPhase; label: string; description: string }>;
const presentationLayoutOptions = [
  { value: "cover", label: "封面", description: "适合标题、开场页和章节主张。" },
  { value: "section", label: "章节", description: "用于分隔主题、阶段或结构转场。" },
  { value: "content", label: "正文", description: "承载正文段落和列表要点。" },
  { value: "imageText", label: "图文", description: "左文右图或视觉说明页。" },
  { value: "summary", label: "总结", description: "用于结论、行动计划和收尾。" }
] satisfies Array<{ value: PptSlideLayout; label: string; description: string }>;
const solidBackgroundPresets = [
  { label: "深黑", value: "#050505" },
  { label: "石墨", value: "#111827" },
  { label: "雾白", value: "#f8fafc" },
  { label: "海蓝", value: "#082f49" },
  { label: "绿松", value: "#064e3b" },
  { label: "暖橙", value: "#7c2d12" },
  { label: "酒红", value: "#7f1d1d" },
  { label: "紫灰", value: "#312e81" }
];
const linearBackgroundPresets = [
  { label: "科技蓝", value: "linear-gradient(135deg, #08111f 0%, #1d4ed8 58%, #38bdf8 100%)" },
  { label: "增长绿", value: "linear-gradient(135deg, #052e16 0%, #16a34a 62%, #86efac 100%)" },
  { label: "路演紫", value: "linear-gradient(135deg, #111827 0%, #7c3aed 56%, #22d3ee 100%)" },
  { label: "黑金", value: "linear-gradient(135deg, #050505 0%, #78350f 58%, #f59e0b 100%)" },
  { label: "暖日", value: "linear-gradient(135deg, #fff7ed 0%, #fb923c 50%, #be123c 100%)" },
  { label: "冷雾", value: "linear-gradient(135deg, #f8fafc 0%, #dbeafe 54%, #93c5fd 100%)" }
];
const radialBackgroundPresets = [
  { label: "中心光", value: "radial-gradient(circle at 50% 32%, #38bdf8 0%, #1e3a8a 44%, #020617 100%)" },
  { label: "左上光", value: "radial-gradient(circle at 18% 20%, #86efac 0%, #166534 42%, #020617 100%)" },
  { label: "右侧光", value: "radial-gradient(circle at 78% 28%, #f97316 0%, #7c2d12 38%, #09090b 100%)" },
  { label: "柔紫光", value: "radial-gradient(circle at 50% 50%, #a78bfa 0%, #3730a3 42%, #020617 100%)" },
  { label: "白场", value: "radial-gradient(circle at 50% 32%, #ffffff 0%, #e0f2fe 46%, #bfdbfe 100%)" },
  { label: "医疗绿", value: "radial-gradient(circle at 42% 30%, #ccfbf1 0%, #14b8a6 46%, #134e4a 100%)" }
];
const slidePaletteBackgroundOptions = [
  ...solidBackgroundPresets.slice(0, 4),
  ...linearBackgroundPresets.slice(0, 2)
] satisfies Array<{ label: string; value: string }>;
const presentationDeckWidthOptions = [
  { label: "紧凑", value: "compact", width: 760, description: "适合信息密度高的页。" },
  { label: "标准", value: "standard", width: 880, description: "默认 16:9 编辑宽度。" },
  { label: "宽屏", value: "wide", width: 1020, description: "适合图表和大视觉页。" }
] satisfies Array<{ label: string; value: PresentationDeckWidth; width: number; description: string }>;
const presentationTypographyScaleOptions = [
  { label: "小", value: "small", scale: 0.92 },
  { label: "中", value: "medium", scale: 1 },
  { label: "大", value: "large", scale: 1.12 }
] satisfies Array<{ label: string; value: PresentationTypographyScale; scale: number }>;
const presentationIconOptions = [
  { name: "sparkles", label: "灵感", glyph: "✦", keywords: ["spark", "idea", "灵感", "AI"] },
  { name: "star", label: "重点", glyph: "★", keywords: ["star", "重点", "收藏"] },
  { name: "check", label: "完成", glyph: "✓", keywords: ["check", "done", "完成"] },
  { name: "flag", label: "目标", glyph: "⚑", keywords: ["flag", "goal", "目标"] },
  { name: "arrow", label: "推进", glyph: "→", keywords: ["arrow", "next", "推进"] },
  { name: "number", label: "数据", glyph: "#", keywords: ["number", "data", "数据"] },
  { name: "percent", label: "增长", glyph: "%", keywords: ["percent", "growth", "增长"] },
  { name: "info", label: "说明", glyph: "i", keywords: ["info", "说明", "提示"] },
  { name: "plus", label: "新增", glyph: "+", keywords: ["plus", "add", "新增"] },
  { name: "warning", label: "提醒", glyph: "!", keywords: ["warning", "alert", "提醒"] },
  { name: "quote", label: "观点", glyph: "“”", keywords: ["quote", "观点", "引用"] },
  { name: "ai", label: "AI", glyph: "AI", keywords: ["ai", "model", "智能"] }
] satisfies Array<{ name: string; label: string; glyph: string; keywords: string[] }>;
const presentationAgentQuickPrompts = [
  "优化当前页标题和正文",
  "把当前页改得更适合项目路演",
  "为当前页补充三个更有说服力的要点",
  "给当前页一个更清晰的视觉布局建议"
];
const elementAiQuickSuggestions = [
  { label: "补充要点", prompt: "为选中内容补充更有说服力的要点", icon: "+" },
  { label: "改标题", prompt: "把选中内容改成更清晰的标题表达", icon: "T" },
  { label: "更简洁", prompt: "让选中内容更简洁，保留核心判断", icon: "✓" },
  { label: "更自然", prompt: "把选中内容改成更自然的演讲表达", icon: "AI" }
] satisfies Array<{ label: string; prompt: string; icon: string }>;
const slideMagicWritingActions = [
  { value: "writing", label: "优化文案", icon: "pen" },
  { value: "spelling", label: "修正错别字", icon: "check" },
  { value: "translate", label: "中英文转换", icon: "globe" },
  { value: "simplify", label: "精简内容", icon: "down" }
] satisfies Array<{ value: SlideMagicAction; label: string; icon: "pen" | "check" | "globe" | "down" }>;
const slideMagicImageActions = [
  { value: "visual", label: "图文增强", icon: "image" },
  { value: "image", label: "添加图片", icon: "plus" }
] satisfies Array<{ value: SlideMagicAction; label: string; icon: "image" | "plus" }>;
const presentationElementKinds = [
  { label: "标题", value: "title", hint: "主标题和章节标题" },
  { label: "正文", value: "content", hint: "说明段落和叙述文本" },
  { label: "要点", value: "bullets", hint: "列表、步骤和结论" },
  { label: "讲稿", value: "notes", hint: "演讲备注和提示词" }
] satisfies Array<{ label: string; value: PresentationElementKind; hint: string }>;
const presentationElementAlignments = [
  { label: "左对齐", value: "left" },
  { label: "居中", value: "center" },
  { label: "右对齐", value: "right" }
] satisfies Array<{ label: string; value: PresentationElementAlign }>;
const presentationElementColors = [
  { label: "默认深色", value: "#0f172a" },
  { label: "科技蓝", value: "#2563eb" },
  { label: "青色强调", value: "#0891b2" },
  { label: "增长绿", value: "#16a34a" },
  { label: "警示橙", value: "#ea580c" },
  { label: "浅色", value: "#f8fafc" }
] satisfies Array<{ label: string; value: string }>;
const generationAspectRatioOptions: Array<{ label: string; value: PptGenerationAspectRatio }> = [
  { label: "动态的", value: "dynamic" },
  { label: "16:9", value: "16:9" }
];
const languageOptions: Array<{ label: string; value: PptLanguage }> = [
  { label: "中文", value: "zh" },
  { label: "英文", value: "en" }
];
const textContentOptions: Array<{ value: PptTextContent; label: string; description: string; lines: number }> = [
  { value: "minimal", label: "极简主义", description: "只保留核心句", lines: 2 },
  { value: "concise", label: "简洁的", description: "适合路演汇报", lines: 3 },
  { value: "detailed", label: "详细的", description: "包含解释说明", lines: 3 },
  { value: "extensive", label: "广泛的", description: "内容更完整", lines: 4 }
];
const toneOptions: Array<{ value: PptTone; label: string }> = [
  { value: "professional", label: "Auto" },
  { value: "simple", label: "轻松易懂" },
  { value: "marketing", label: "营销转化" },
  { value: "education", label: "教育培训" },
  { value: "pitch", label: "汇报路演" }
];
const audienceOptions: Array<{ value: PptAudience; label: string }> = [
  { value: "auto", label: "Auto" },
  { value: "general", label: "大众用户" },
  { value: "business", label: "企业客户" },
  { value: "investor", label: "投资人" },
  { value: "teacher", label: "教师/讲师" },
  { value: "student", label: "学生/学员" }
];
const scenarioOptions: Array<{ value: PptScenario; label: string }> = [
  { value: "auto", label: "Auto" },
  { value: "general", label: "通用演示" },
  { value: "analysis-report", label: "分析报告" },
  { value: "teaching-training", label: "教学培训" },
  { value: "promotional-materials", label: "营销物料" },
  { value: "public-speeches", label: "公开演讲" }
];

const libraryFilters = [
  { label: "全部", value: "all" },
  { label: "最近浏览", value: "recent" },
  { label: "收藏夹", value: "favorites" }
] satisfies Array<{ label: string; value: LibraryFilter }>;
const pptHomeTemplates = [
  {
    title: "商务汇报",
    subtitle: "季度复盘",
    tag: "BUSINESS",
    accent: "#5A4DB2",
    prompt: "制作一份商务汇报 PPT，包含项目背景、核心进展、数据表现、问题复盘和下一步计划，整体风格专业、简洁、适合管理层汇报。"
  },
  {
    title: "商业计划书",
    subtitle: "融资路演",
    tag: "PLAN",
    accent: "#7D8DF6",
    prompt: "生成一份商业计划书 BP，包含市场机会、产品方案、商业模式、竞争优势、增长路径、财务预测和融资计划，适合投资人路演。"
  },
  {
    title: "产品发布会",
    subtitle: "新品亮相",
    tag: "LAUNCH",
    accent: "#FF771B",
    prompt: "制作一份产品发布会演示稿，突出新品亮点、核心卖点、目标用户、应用场景、价格策略和发布节奏，视觉风格科技感强。"
  },
  {
    title: "年终总结",
    subtitle: "年度复盘",
    tag: "20XX",
    accent: "#D2D4D6",
    prompt: "生成一份年终工作总结 PPT，包含年度目标、关键成果、数据亮点、经验沉淀、问题反思和来年规划，风格稳重清晰。"
  },
  {
    title: "培训课件",
    subtitle: "企业培训",
    tag: "TRAINING",
    accent: "#6EA8FF",
    prompt: "制作一份企业培训课件，主题结构包括学习目标、知识框架、案例讲解、互动练习、总结回顾和行动建议，适合内部培训。"
  },
  {
    title: "学术答辩",
    subtitle: "论文汇报",
    tag: "THESIS",
    accent: "#A78BFA",
    prompt: "生成一份学术答辩 PPT，包含研究背景、问题定义、方法设计、实验结果、创新点、结论与展望，风格严谨清爽。"
  }
] satisfies PptHomeTemplate[];
const pptHomeInspirations = [
  { title: "如何制作一份吸引投资的商业计划书", icon: "BP", prompt: "制作一份吸引投资人的商业计划书 PPT，重点突出市场规模、商业模式、增长数据、团队优势和融资用途。" },
  { title: "年终总结PPT的结构和要点", icon: "年", prompt: "生成一份年终总结 PPT，结构包含年度回顾、关键成绩、数据复盘、问题改进和下一年度计划。" },
  { title: "产品发布会PPT设计技巧", icon: "发", prompt: "制作一份产品发布会 PPT，要求开场有记忆点，突出产品卖点、使用场景、差异化优势和发布节奏。" },
  { title: "企业培训PPT内容规划指南", icon: "训", prompt: "生成一份企业培训 PPT，围绕课程目标、知识模块、案例演示、练习互动和课后行动清单展开。" },
  { title: "数据可视化在PPT中的应用", icon: "数", prompt: "制作一份数据可视化主题 PPT，说明如何用图表呈现趋势、对比、占比、漏斗和业务洞察。" },
  { title: "项目进度汇报怎么讲更清楚", icon: "项", prompt: "生成一份项目进度汇报 PPT，包含项目目标、阶段进展、风险问题、资源需求和后续里程碑。" },
  { title: "品牌推广方案的核心结构", icon: "品", prompt: "制作一份品牌推广方案 PPT，包含目标人群、品牌定位、传播主张、渠道策略、内容规划和效果指标。" },
  { title: "销售复盘汇报的关键指标", icon: "销", prompt: "生成一份销售复盘 PPT，围绕销售目标、成交数据、渠道表现、客户画像、问题分析和改进动作展开。" }
] satisfies PptHomeInspiration[];
const historySortOptions = [
  { label: "最新优先", value: "date-desc", icon: "clock" },
  { label: "最早优先", value: "date-asc", icon: "clock" },
  { label: "名称 A-Z", value: "name-asc", icon: "grid" },
  { label: "名称 Z-A", value: "name-desc", icon: "grid" }
] satisfies Array<{ label: string; value: HistorySortBy; icon: "clock" | "grid" }>;
const historyTypeOptions = [
  { label: "全部", value: "all" },
  { label: "演示文稿", value: "presentation" }
] satisfies Array<{ label: string; value: HistoryTypeFilter }>;
const presentationToolPanels = [
  { label: "文本", value: "content", icon: "text", hint: "插入基础文本块，也可编辑当前页文案。", searchPlaceholder: "搜索文本、表格、列表..." },
  { label: "AI", value: "agent", icon: "agent", hint: "让 PPT 助手针对当前页生成编辑建议。", searchPlaceholder: "" },
  { label: "元素", value: "elements", icon: "blocks", hint: "添加重点卡片、对比块和行动清单。", searchPlaceholder: "搜索元素、流程、对比..." },
  { label: "图表", value: "charts", icon: "chart", hint: "预留饼图、柱状图、折线图等图表结构。", searchPlaceholder: "搜索图表、趋势、占比..." },
  { label: "图示", value: "diagrams", icon: "diagram", hint: "插入流程、时间线、漏斗和层级结构。", searchPlaceholder: "搜索图示、流程、漏斗..." },
  { label: "嵌入", value: "embed", icon: "embed", hint: "预留网页、视频和数据看板嵌入。", searchPlaceholder: "搜索嵌入类型..." },
  { label: "主题", value: "theme", icon: "theme", hint: "切换整套演示主题风格。", searchPlaceholder: "" },
  { label: "布局", value: "layout", icon: "layout", hint: "选择当前页版式或快速尝试新布局。", searchPlaceholder: "" },
  { label: "背景", value: "background", icon: "background", hint: "设置当前页背景颜色、渐变或图片。", searchPlaceholder: "" },
  { label: "设置", value: "globalSettings", icon: "settings", hint: "调整页面宽度、对齐、字号、主题和背景。", searchPlaceholder: "" },
  { label: "图标", value: "iconPicker", icon: "sparkles", hint: "为当前页标题挑选一个强调图标。", searchPlaceholder: "" },
  { label: "图片", value: "images", icon: "image", hint: "生成或搜索当前页配图。", searchPlaceholder: "" },
  { label: "讲稿", value: "notes", icon: "notes", hint: "编辑当前页演讲备注。", searchPlaceholder: "" },
  { label: "录制", value: "record", icon: "record", hint: "进入演示录制预览。", searchPlaceholder: "" }
] satisfies Array<{ label: string; value: PresentationRightPanel; icon: PresentationToolIcon; hint: string; searchPlaceholder: string }>;
const referenceRightToolPanelOrder: PresentationRightPanel[] = ["content", "elements", "charts", "diagrams", "embed", "record"];
const referenceRightToolPanelSet = new Set<PresentationRightPanel>(referenceRightToolPanelOrder);
const presentationImmediateLoadPanels = new Set<PresentationRightPanel>(["content", "elements"]);
const presentationSelfContainedPanels = new Set<PresentationRightPanel>(["agent", "theme", "globalSettings", "images", "notes"]);
const presentationOrderedToolPanels: Array<(typeof presentationToolPanels)[number]> = [
  ...referenceRightToolPanelOrder
    .map(panel => presentationToolPanels.find(item => item.value === panel))
    .filter((item): item is (typeof presentationToolPanels)[number] => Boolean(item))
];
const presentationTextBlocks: InsertablePresentationBlock[] = [
  {
    title: "大标题",
    description: "适合章节标题或封面主张。",
    content: "新增大标题模块，用于强化本页的核心观点。",
    bulletPoints: ["主标题", "一句话价值", "展示重点"],
    category: "text",
    icon: "title",
    iconLabel: "T",
    keywords: ["title", "heading", "标题"]
  },
  {
    title: "一级标题",
    description: "用于页面中的重要分段。",
    content: "新增一级标题模块，帮助页面形成清晰层次。",
    bulletPoints: ["分段主题", "关键说明"],
    category: "text",
    icon: "heading",
    iconLabel: "H1",
    keywords: ["heading", "标题", "分段"]
  },
  {
    title: "正文段落",
    description: "补充一段完整解释文字。",
    content: "新增正文段落，用于承接当前页观点并补充业务背景。",
    bulletPoints: ["背景说明", "核心判断"],
    category: "text",
    icon: "paragraph",
    iconLabel: "Aa",
    keywords: ["paragraph", "正文"]
  },
  {
    title: "引用语",
    description: "突出一句结论、客户原话或洞察。",
    content: "新增引用语模块：用一句高可信度表达强化当前页结论。",
    bulletPoints: ["引用来源", "关键观点"],
    category: "text",
    icon: "quote",
    iconLabel: "\"",
    keywords: ["quote", "引用"]
  },
  {
    title: "标签",
    description: "为内容增加短标签或状态标记。",
    content: "新增标签模块，可用于标记重点、阶段或风险等级。",
    bulletPoints: ["标签名称", "适用范围"],
    category: "text",
    icon: "label",
    iconLabel: "TAG",
    keywords: ["label", "tag", "标签"]
  },
  {
    title: "2x2 表格",
    description: "适合四象限或能力对照。",
    content: "新增 2x2 表格占位，用于呈现维度化信息。",
    bulletPoints: ["维度一", "维度二", "结论"],
    category: "tables",
    icon: "table",
    iconLabel: "2x2",
    keywords: ["table", "表格", "矩阵"]
  },
  {
    title: "无序列表",
    description: "快速插入三条要点。",
    content: "新增无序列表，用于拆解本页核心信息。",
    bulletPoints: ["要点一", "要点二", "要点三"],
    category: "lists",
    icon: "list",
    iconLabel: "UL",
    keywords: ["list", "bullet", "列表"]
  },
  {
    title: "编号列表",
    description: "适合步骤、优先级或流程。",
    content: "新增编号列表，用于表达顺序化行动。",
    bulletPoints: ["第一步", "第二步", "第三步"],
    category: "lists",
    icon: "ordered",
    iconLabel: "01",
    keywords: ["numbered", "steps", "编号"]
  },
  {
    title: "提示框",
    description: "突出提醒、风险或结论。",
    content: "新增提示框模块，用于放置提醒、风险或关键判断。",
    bulletPoints: ["提醒内容", "处理建议"],
    category: "callouts",
    icon: "callout",
    iconLabel: "!",
    keywords: ["callout", "warning", "提示"]
  },
  {
    title: "行动按钮",
    description: "模拟 CTA 或下一步入口。",
    content: "新增行动按钮占位，可用于展示下一步动作。",
    bulletPoints: ["按钮文案", "触发动作"],
    category: "interactive",
    icon: "button",
    iconLabel: "CTA",
    keywords: ["button", "cta", "行动"]
  },
  {
    title: "目录",
    description: "生成页面结构索引。",
    content: "新增目录占位，用于承接演示文稿章节结构。",
    bulletPoints: ["章节一", "章节二", "章节三"],
    category: "other",
    icon: "toc",
    iconLabel: "TOC",
    keywords: ["toc", "目录", "结构"]
  }
];
const presentationElementBlocks: InsertablePresentationBlock[] = [
  {
    title: "重点数字",
    description: "突出一个核心指标和解释。",
    content: "新增重点数字模块，用于强调当前页最关键的业务指标。",
    bulletPoints: ["关键指标", "指标解释", "业务影响"],
    category: "data",
    icon: "stats",
    iconLabel: "#",
    keywords: ["stats", "data", "数字"]
  },
  {
    title: "对比模块",
    description: "适合方案前后或竞品对比。",
    content: "新增对比模块，可用于呈现现状与目标方案的差异。",
    bulletPoints: ["当前状态", "目标方案", "差异价值"],
    category: "compare",
    icon: "compare",
    iconLabel: "VS",
    keywords: ["compare", "before after", "对比"]
  },
  {
    title: "行动清单",
    description: "把结论转为下一步动作。",
    content: "新增行动清单模块，将页面观点沉淀为可执行步骤。",
    bulletPoints: ["负责人", "关键动作", "完成节点"],
    category: "utility",
    icon: "todo",
    iconLabel: "✓",
    keywords: ["todo", "action", "清单"]
  },
  {
    title: "步骤流程",
    description: "呈现 3-4 步执行路径。",
    content: "新增步骤流程模块，用于说明从输入到交付的执行链路。",
    bulletPoints: ["启动", "执行", "交付"],
    category: "process",
    icon: "steps",
    iconLabel: "1-3",
    keywords: ["steps", "process", "流程"]
  },
  {
    title: "前后对比",
    description: "适合方案升级、改版前后。",
    content: "新增前后对比模块，用于呈现优化前后的变化。",
    bulletPoints: ["Before", "After", "提升点"],
    category: "compare",
    icon: "before-after",
    iconLabel: "A/B",
    keywords: ["before", "after", "前后"]
  },
  {
    title: "多列版式",
    description: "拆成两到三列并列信息。",
    content: "新增多列版式模块，用于并列展示多个观点。",
    bulletPoints: ["左侧观点", "中间观点", "右侧观点"],
    category: "layout",
    icon: "columns",
    iconLabel: "COL",
    keywords: ["columns", "layout", "版式"]
  },
  {
    title: "媒体占位",
    description: "为图片、视频或截图预留空间。",
    content: "新增媒体占位模块，用于承接图片、视频或产品截图。",
    bulletPoints: ["媒体来源", "展示目的", "说明文字"],
    category: "media",
    icon: "media",
    iconLabel: "IMG",
    keywords: ["image", "media", "媒体"]
  },
  {
    title: "图标清单",
    description: "用图标强化多个功能点。",
    content: "新增图标清单模块，用于表达功能、能力或亮点。",
    bulletPoints: ["能力一", "能力二", "能力三"],
    category: "utility",
    icon: "icons",
    iconLabel: "ICO",
    keywords: ["icon", "list", "图标"]
  }
];
const presentationChartBlocks: InsertablePresentationBlock[] = [
  {
    title: "饼图",
    description: "展示类别占比和结构分布。",
    content: "新增饼图占位，用于呈现结构占比。",
    bulletPoints: ["主要类别", "占比结构", "关键结论"],
    category: "basic",
    icon: "pie",
    iconLabel: "Pie",
    keywords: ["pie", "占比"]
  },
  {
    title: "环形图",
    description: "突出完成度或份额构成。",
    content: "新增环形图占位，用于呈现完成度或份额。",
    bulletPoints: ["完成比例", "剩余空间", "业务解释"],
    category: "basic",
    icon: "donut",
    iconLabel: "Donut",
    keywords: ["donut", "环形"]
  },
  {
    title: "柱状图",
    description: "比较不同渠道、产品或阶段数据。",
    content: "新增柱状图占位，后续可接入真实数据渲染。",
    bulletPoints: ["数据维度", "对比指标", "趋势结论"],
    category: "basic",
    icon: "bar",
    iconLabel: "Bar",
    keywords: ["bar", "柱状"]
  },
  {
    title: "折线图",
    description: "展示增长、留存或转化趋势。",
    content: "新增折线图占位，用于呈现时间序列变化。",
    bulletPoints: ["时间周期", "趋势变化", "关键拐点"],
    category: "basic",
    icon: "line",
    iconLabel: "Line",
    keywords: ["line", "趋势"]
  },
  {
    title: "面积图",
    description: "展示趋势与规模叠加变化。",
    content: "新增面积图占位，用于呈现趋势和累积规模。",
    bulletPoints: ["趋势区间", "规模变化", "增长解释"],
    category: "basic",
    icon: "area",
    iconLabel: "Area",
    keywords: ["area", "面积"]
  },
  {
    title: "散点图",
    description: "分析变量关系和分布。",
    content: "新增散点图占位，用于表达变量间关系。",
    bulletPoints: ["横轴变量", "纵轴变量", "相关性"],
    category: "statistical",
    icon: "scatter",
    iconLabel: "Dot",
    keywords: ["scatter", "散点"]
  },
  {
    title: "热力图",
    description: "适合二维矩阵强弱比较。",
    content: "新增热力图占位，用于展示强弱分布。",
    bulletPoints: ["维度一", "维度二", "高低区域"],
    category: "statistical",
    icon: "heatmap",
    iconLabel: "Heat",
    keywords: ["heatmap", "热力"]
  },
  {
    title: "瀑布图",
    description: "展示增减项如何影响结果。",
    content: "新增瀑布图占位，用于呈现指标增减拆解。",
    bulletPoints: ["起始值", "增减项", "最终结果"],
    category: "range",
    icon: "waterfall",
    iconLabel: "WF",
    keywords: ["waterfall", "瀑布"]
  },
  {
    title: "漏斗图",
    description: "适合营销、销售和转化分析。",
    content: "新增漏斗图占位，用于呈现转化路径。",
    bulletPoints: ["曝光", "转化", "成交"],
    category: "funnel",
    icon: "funnel",
    iconLabel: "FUN",
    keywords: ["funnel", "漏斗"]
  },
  {
    title: "仪表盘",
    description: "展示目标达成或健康度。",
    content: "新增仪表盘占位，用于呈现目标达成情况。",
    bulletPoints: ["目标值", "当前值", "风险判断"],
    category: "gauge",
    icon: "gauge",
    iconLabel: "G",
    keywords: ["gauge", "仪表"]
  },
  {
    title: "桑基图",
    description: "展示流向、来源和去向。",
    content: "新增桑基图占位，用于表达资源或流量路径。",
    bulletPoints: ["来源", "流向", "流失点"],
    category: "flow",
    icon: "sankey",
    iconLabel: "SK",
    keywords: ["sankey", "flow", "流向"]
  }
];
const presentationDiagramBlocks: InsertablePresentationBlock[] = [
  {
    title: "流程图",
    description: "表达从输入到交付的步骤。",
    content: "新增流程图占位，用于表达方案推进链路。",
    bulletPoints: ["输入", "处理", "输出"],
    category: "process",
    icon: "flow",
    iconLabel: "Flow",
    keywords: ["flow", "process", "流程"]
  },
  {
    title: "时间线",
    description: "展示阶段计划和里程碑。",
    content: "新增时间线占位，用于展示项目推进节奏。",
    bulletPoints: ["启动阶段", "实施阶段", "复盘阶段"],
    category: "process",
    icon: "timeline",
    iconLabel: "Time",
    keywords: ["timeline", "时间线"]
  },
  {
    title: "步骤卡",
    description: "把复杂工作拆成步骤。",
    content: "新增步骤卡占位，用于表达清晰行动路径。",
    bulletPoints: ["第一步", "第二步", "第三步"],
    category: "process",
    icon: "steps",
    iconLabel: "Step",
    keywords: ["steps", "步骤"]
  },
  {
    title: "金字塔",
    description: "表达层级、优先级或能力模型。",
    content: "新增金字塔占位，用于表达层级结构。",
    bulletPoints: ["基础层", "能力层", "目标层"],
    category: "hierarchy",
    icon: "pyramid",
    iconLabel: "PYR",
    keywords: ["pyramid", "层级"]
  },
  {
    title: "循环图",
    description: "展示闭环增长或运营周期。",
    content: "新增循环图占位，用于表达闭环机制。",
    bulletPoints: ["触达", "转化", "复购"],
    category: "cycle",
    icon: "cycle",
    iconLabel: "Loop",
    keywords: ["cycle", "循环"]
  },
  {
    title: "连接圆",
    description: "表达模块之间的关联关系。",
    content: "新增连接圆占位，用于呈现多模块协同关系。",
    bulletPoints: ["模块一", "模块二", "模块三"],
    category: "hierarchy",
    icon: "circles",
    iconLabel: "Net",
    keywords: ["connected", "circles", "关联"]
  },
  {
    title: "优劣势",
    description: "展示 Pros & Cons 结构。",
    content: "新增优劣势图示，用于表达利弊判断。",
    bulletPoints: ["优势", "风险", "取舍建议"],
    category: "compare",
    icon: "pros-cons",
    iconLabel: "+/-",
    keywords: ["pros", "cons", "优劣"]
  },
  {
    title: "漏斗图",
    description: "适合营销、销售和转化分析。",
    content: "新增漏斗图占位，用于呈现转化路径。",
    bulletPoints: ["曝光", "转化", "成交"],
    category: "funnel",
    icon: "funnel",
    iconLabel: "FUN",
    keywords: ["funnel", "漏斗"]
  }
];
const presentationEmbedBlocks: InsertablePresentationBlock[] = [
  {
    title: "视频嵌入",
    description: "YouTube / Vimeo / Loom 链接占位。",
    content: "新增视频嵌入占位，后续可接入 iframe 或媒体卡片。",
    bulletPoints: ["视频来源", "核心片段", "观看目的"],
    category: "video",
    icon: "video",
    iconLabel: "VID",
    keywords: ["youtube", "vimeo", "loom", "视频"]
  },
  {
    title: "网页链接",
    description: "官网、报告、数据看板入口。",
    content: "新增网页嵌入占位，用于承接外部链接。",
    bulletPoints: ["网页标题", "链接价值", "打开方式"],
    category: "web",
    icon: "web",
    iconLabel: "URL",
    keywords: ["web", "url", "网页"]
  },
  {
    title: "Figma 设计稿",
    description: "设计稿或原型链接占位。",
    content: "新增 Figma 设计稿占位，用于展示界面、原型或视觉稿。",
    bulletPoints: ["设计链接", "关键画面", "说明重点"],
    category: "design",
    icon: "figma",
    iconLabel: "FIG",
    keywords: ["figma", "design", "设计"]
  },
  {
    title: "CodePen 示例",
    description: "交互 demo 或代码示例占位。",
    content: "新增 CodePen 示例占位，用于呈现交互或代码演示。",
    bulletPoints: ["示例链接", "交互说明", "技术重点"],
    category: "design",
    icon: "codepen",
    iconLabel: "CODE",
    keywords: ["codepen", "代码"]
  },
  {
    title: "社媒帖子",
    description: "社交媒体内容引用占位。",
    content: "新增社媒帖子占位，用于承接品牌声量或用户反馈。",
    bulletPoints: ["平台来源", "引用内容", "传播价值"],
    category: "social",
    icon: "social",
    iconLabel: "SOC",
    keywords: ["social", "twitter", "社媒"]
  },
  {
    title: "图片媒体",
    description: "外部图片或产品截图链接。",
    content: "新增图片媒体占位，用于展示产品、案例或场景图。",
    bulletPoints: ["图片来源", "展示内容", "配图说明"],
    category: "media",
    icon: "image",
    iconLabel: "IMG",
    keywords: ["image", "图片"]
  },
  {
    title: "AI 信息图",
    description: "预留信息图生成或嵌入入口。",
    content: "新增 AI 信息图占位，后续可接入信息图生成服务。",
    bulletPoints: ["信息图主题", "数据来源", "展示目标"],
    category: "media",
    icon: "infographic",
    iconLabel: "INFO",
    keywords: ["infographic", "信息图"]
  }
];
const allPanelCategoryOption: PresentationPanelCategoryOption = { label: "全部", value: "all" };
const presentationTextCategoryOptions: PresentationPanelCategoryOption[] = [
  allPanelCategoryOption,
  { label: "文本", value: "text" },
  { label: "表格", value: "tables" },
  { label: "列表", value: "lists" },
  { label: "提示框", value: "callouts" },
  { label: "交互", value: "interactive" },
  { label: "其他", value: "other" }
];
const presentationElementCategoryOptions: PresentationPanelCategoryOption[] = [
  allPanelCategoryOption,
  { label: "流程", value: "process" },
  { label: "对比", value: "compare" },
  { label: "版式", value: "layout" },
  { label: "数据", value: "data" },
  { label: "媒体", value: "media" },
  { label: "工具", value: "utility" }
];
const presentationChartCategoryOptions: PresentationPanelCategoryOption[] = [
  allPanelCategoryOption,
  { label: "基础", value: "basic" },
  { label: "统计", value: "statistical" },
  { label: "区间", value: "range" },
  { label: "流向", value: "flow" },
  { label: "漏斗", value: "funnel" },
  { label: "仪表", value: "gauge" }
];
const presentationDiagramCategoryOptions: PresentationPanelCategoryOption[] = [
  allPanelCategoryOption,
  { label: "流程", value: "process" },
  { label: "层级", value: "hierarchy" },
  { label: "循环", value: "cycle" },
  { label: "对比", value: "compare" },
  { label: "漏斗", value: "funnel" }
];
const presentationEmbedCategoryOptions: PresentationPanelCategoryOption[] = [
  allPanelCategoryOption,
  { label: "视频", value: "video" },
  { label: "设计", value: "design" },
  { label: "网页", value: "web" },
  { label: "媒体", value: "media" },
  { label: "社媒", value: "social" }
];

const isBusy = computed(() => ["outlining", "pending", "generating", "rendering"].includes(store.status));
const hasEditablePresentationSlides = computed(() => isPresentationWorkspace.value && store.slides.length > 0);
const isPresentationActionBusy = computed(() => isBusy.value && !hasEditablePresentationSlides.value);
const canSubmit = computed(() => {
  if (isBusy.value) return false;
  if (store.createMode === "blank") return true;
  return Boolean(store.prompt.trim());
});
const showProgress = computed(() => {
  if (!["outlining", "pending", "generating", "rendering", "failed"].includes(store.status)) return false;
  return !hasEditablePresentationSlides.value;
});
const visiblePptHomeInspirations = computed(() => {
  const pageSize = 5;
  const start = (pptInspirationPage.value * pageSize) % pptHomeInspirations.length;
  return Array.from(
    { length: pageSize },
    (_, index) => pptHomeInspirations[(start + index) % pptHomeInspirations.length]
  );
});
const generationAspectRatioLabel = computed(() => {
  return generationAspectRatioOptions.find((option) => option.value === store.generationAspectRatio)?.label || "动态的";
});
const languageLabel = computed(() => {
  return languageOptions.find((option) => option.value === store.language)?.label || "中文";
});
const themeLabel = computed(() => pptThemeLabel(store.theme));
const pptThemeOptions = computed(() => (
  pptThemes.map((item) => ({ value: item.value, label: item.label }))
));
const currentTextModelLabel = computed(() => {
  const model = store.textModels.find((item) => item.value === store.textModel);
  if (!model) return "GPT-4o-mini";
  return model.label;
});
const textModelsLoading = computed(() => !store.textModels.length && !store.initialized);
const textModelGroups = computed<TextModelGroup[]>(() => {
  const source = store.textModels;
  const groups: TextModelGroup[] = [];
  const groupMap = new Map<string, PptModelOption[]>();
  source.forEach((model) => {
    const label = model.group || model.provider || "其他模型";
    if (!groupMap.has(label)) {
      groupMap.set(label, []);
      groups.push({ label, models: groupMap.get(label) || [] });
    }
    groupMap.get(label)?.push(model);
  });
  return groups.filter((group) => group.models.length);
});
const createModeHint = computed(() => {
  if (store.createMode === "blank") return "空白演示文稿";
  if (store.createMode === "document") return store.uploadedDocumentName || "上传文档生成";
  return "AI 根据主题生成";
});
const shouldShowHistorySearchInput = computed(() => Boolean(historySearchQuery.value) || isHistorySearchOpen.value);
const activeHistoryFiltersCount = computed(() => {
  return (showFavoritesOnlyFilter.value ? 1 : 0) + (historyTypeFilter.value !== "all" ? 1 : 0);
});
const filteredHistory = computed(() => {
  const query = historySearchQuery.value.trim().toLowerCase();
  let source = [...store.history];
  if (libraryFilter.value === "favorites" || showFavoritesOnlyFilter.value) {
    source = source.filter((item) => isFavoriteTask(item.taskId));
  }
  if (query) {
    source = source.filter((item) => {
      const haystack = `${item.title} ${item.language || ""} ${item.theme || ""} ${item.status || ""}`.toLowerCase();
      return haystack.includes(query);
    });
  }
  source.sort((a, b) => {
    if (historySortBy.value === "date-asc") return historyTime(a.createdAt) - historyTime(b.createdAt);
    if (historySortBy.value === "name-asc") return a.title.localeCompare(b.title, "zh-CN");
    if (historySortBy.value === "name-desc") return b.title.localeCompare(a.title, "zh-CN");
    return historyTime(b.createdAt) - historyTime(a.createdAt);
  });
  return libraryFilter.value === "recent" ? source.slice(0, 6) : source;
});
const historyEmptyTitle = computed(() => {
  if (historySearchQuery.value.trim()) return "没有匹配的演示文稿";
  return libraryFilter.value === "favorites" ? "暂无收藏演示文稿" : "暂无演示文稿";
});
const historyEmptyDescription = computed(() => {
  if (historySearchQuery.value.trim()) return "换一个关键词，或清空搜索后查看全部记录。";
  if (showFavoritesOnlyFilter.value) return "当前开启了仅看收藏，取消筛选后可查看其它记录。";
  if (libraryFilter.value === "favorites") return "点击历史记录里的星标后，会在这里集中查看。";
  if (libraryFilter.value === "recent") return "最近浏览的 PPT 会优先展示在这里。";
  return "生成完成后会出现在全部记录中。";
});
const workspaceTitle = computed(() => store.outline?.title || store.prompt.trim() || "无标题演示文稿");
function activePptPath(path: string) {
  if (typeof window !== "undefined" && window.location.pathname.startsWith("/workspace") && (path === "/app" || path.startsWith("/app/"))) {
    return `/workspace${path.slice(4)}`;
  }
  return path;
}

function canonicalPptPath(path: string) {
  return path === "/workspace" || path.startsWith("/workspace/") ? `/app${path.slice("/workspace".length)}` : path;
}

const presentationShareUrl = computed(() => {
  const id = activePresentationId.value || store.taskId || activeGenerationId.value || "draft";
  const path = activePptPath(`/app/ppt-generation/presentation/${encodeURIComponent(id)}`);
  if (typeof window === "undefined") return path;
  return `${window.location.origin}${path}`;
});
const workspacePrompt = computed(() => store.prompt.trim() || store.outline?.title || "正在准备演示文稿主题");
const hasOutline = computed(() => Boolean(store.outline?.slides.length));
const isPreOutlineSetup = computed(() => {
  if (hasOutline.value) return false;
  return !["outlining", "pending", "generating", "rendering", "success"].includes(store.status);
});
const workspaceGenerationSteps = computed(() => {
  const deckBusy = ["pending", "generating", "rendering"].includes(store.status);
  const deckDone = store.status === "success";
  const outlineDone = hasOutline.value && store.status !== "outlining";
  const outlineState: GenerationStepState = store.status === "failed" && !hasOutline.value
    ? "failed"
    : store.status === "outlining"
      ? "active"
      : outlineDone
        ? "done"
        : "pending";
  const reviewState: GenerationStepState = store.status === "failed" && hasOutline.value
    ? "failed"
    : deckBusy || deckDone
      ? "done"
      : outlineDone
        ? "active"
        : "pending";
  const deckState: GenerationStepState = store.status === "failed" && hasOutline.value
    ? "failed"
    : deckDone
      ? "done"
      : deckBusy
        ? "active"
        : "pending";
  return [
    {
      key: "outline",
      index: "1",
      label: "生成大纲",
      description: store.enableWebSearch ? "结合联网搜索整理结构" : "根据主题整理演示结构",
      state: outlineState
    },
    {
      key: "review",
      index: "2",
      label: "确认大纲",
      description: hasOutline.value ? `${store.outline?.slides.length || 0} 页大纲可调整` : "等待大纲生成",
      state: reviewState
    },
    {
      key: "deck",
      index: "3",
      label: "生成演示",
      description: deckDone ? "已进入可编辑演示文稿" : "确认后生成完整 PPT 页面",
      state: deckState
    }
  ];
});
const workspaceActivityItems = computed(() => {
  const outlineReady = hasOutline.value;
  const deckBusy = ["pending", "generating", "rendering"].includes(store.status);
  const deckDone = store.status === "success";
  const outlineActive = store.status === "outlining";
  const items: Array<{ key: string; label: string; description: string; state: GenerationStepState }> = [
    {
      key: "intent",
      label: "理解主题与生成参数",
      description: `${store.slideCount}页 · ${languageLabel.value} · ${generationAspectRatioLabel.value} · ${themeLabel.value}`,
      state: outlineActive || outlineReady || deckBusy || deckDone ? "done" : "active"
    }
  ];
  if (store.enableWebSearch) {
    items.push({
      key: "search",
      label: "联网搜索上下文",
      description: outlineActive ? "正在预留搜索资料入口" : "搜索上下文将随大纲一起保存",
      state: outlineActive ? "active" : outlineReady || deckBusy || deckDone ? "done" : "pending"
    });
  }
  items.push(
    {
      key: "outline",
      label: "生成可编辑大纲",
      description: outlineReady ? `${store.outline?.slides.length || 0} 页大纲已就绪` : "等待模型返回页面结构",
      state: outlineActive ? "active" : outlineReady || deckBusy || deckDone ? "done" : "pending"
    },
    {
      key: "render",
      label: "生成完整 PPT",
      description: deckBusy ? `正在处理第 ${Math.max(1, store.currentPage)} / ${store.slideCount} 页` : deckDone ? "完整演示已生成" : "确认大纲后开始生成",
      state: deckDone ? "done" : deckBusy ? "active" : "pending"
    }
  );
  if (store.status === "failed") {
    return items.map((item, index) => index === items.length - 1 ? { ...item, state: "failed" } : item);
  }
  return items;
});
const isGenerationActivityRunning = computed(() => ["outlining", "pending", "generating", "rendering"].includes(store.status));
const isHomeWorkspace = computed(() => !isGenerationWorkspace.value && !isPresentationWorkspace.value);
const activeSlideLabel = computed(() => {
  if (!store.slides.length) return "0 / 0";
  return `${store.currentSlideIndex + 1} / ${store.slides.length}`;
});
const selectedPresentationElementLabel = computed(() => {
  const kind = selectedPresentationElement.value?.kind;
  return presentationElementKinds.find((item) => item.value === kind)?.label || "选择元素";
});
const selectedPresentationElementStyle = computed(() => {
  const selection = selectedPresentationElement.value;
  const slide = selection ? store.slides[selection.slideIndex] : null;
  if (!selection || !slide) return undefined;
  return presentationElementStyles.value[slide.id]?.[selection.kind];
});
const presentationEditorStyle = computed(() => ({
  "--ppt-sidebar-width": presentationSidebarCollapsed.value ? "52px" : `${presentationSidebarWidth.value}px`
}));
const currentPresentationDeckWidth = computed(() => (
  presentationDeckWidthOptions.find(item => item.value === presentationDeckWidth.value)?.width || 880
));
const currentPresentationPresentWidth = computed(() => {
  const { width, height } = presentationViewportSize.value;
  if (!width || !height) return 1280;
  const isPhone = width <= 640;
  const isShortLandscape = height <= 520;
  const horizontalPadding = isPhone ? 24 : 96;
  const verticalPadding = isShortLandscape ? 56 : isPhone ? 92 : 150;
  const availableWidth = Math.max(280, width - horizontalPadding);
  const availableHeight = Math.max(180, height - verticalPadding);
  const widthByHeight = Math.floor(availableHeight * (16 / 9));
  return Math.round(Math.min(1280, availableWidth, widthByHeight));
});
const currentPresentationTypographyScale = computed(() => (
  presentationTypographyScaleOptions.find(item => item.value === presentationTypographyScale.value)?.scale || 1
));
const presentationStageStyle = computed(() => ({
  "--ppt-zoom": String(presentationZoom.value),
  "--ppt-slide-width": `${Math.round(currentPresentationDeckWidth.value * presentationZoom.value)}px`,
  "--ppt-slide-base-width": `${currentPresentationDeckWidth.value}px`,
  "--ppt-present-width": `${currentPresentationPresentWidth.value}px`,
  "--ppt-global-align": presentationGlobalAlignment.value,
  "--ppt-title-size": `clamp(${Math.round(28 * currentPresentationTypographyScale.value)}px, ${4 * currentPresentationTypographyScale.value}vw, ${Math.round(48 * currentPresentationTypographyScale.value)}px)`,
  "--ppt-body-size": `${Math.round(15 * currentPresentationTypographyScale.value)}px`
}));
const activeRightPanelMeta = computed(() => (
  presentationToolPanels.find(item => item.value === presentationRightPanel.value) || presentationToolPanels[0]
));
const isPresentationSelfContainedPanel = computed(() => presentationSelfContainedPanels.has(presentationRightPanel.value));
const isPresentationPanelContentLoaded = computed(() => (
  presentationRightPanelOpen.value && presentationLoadedRightPanel.value === presentationRightPanel.value
));
const currentSlideBackground = computed(() => {
  const slideId = store.currentSlide?.id || "";
  return slideId ? presentationSlideBackgrounds.value[slideId] || "" : "";
});
const customBackgroundColorValue = computed(() => (
  /^#[0-9a-f]{6}$/i.test(currentSlideBackground.value) ? currentSlideBackground.value : "#111827"
));
const currentSlideIcon = computed(() => {
  const slideId = store.currentSlide?.id || "";
  const iconName = slideId ? presentationSlideIcons.value[slideId] : "";
  return presentationIconOptions.find(item => item.name === iconName);
});
const filteredPresentationIconOptions = computed(() => {
  const query = presentationIconSearchQuery.value.trim().toLowerCase();
  if (!query) return presentationIconOptions;
  return presentationIconOptions.filter(item => (
    item.name.toLowerCase().includes(query) ||
    item.label.toLowerCase().includes(query) ||
    item.glyph.toLowerCase().includes(query) ||
    item.keywords.some(keyword => keyword.toLowerCase().includes(query))
  ));
});
const latestAgentSuggestion = computed(() => {
  const latest = [...presentationAgentMessages.value].reverse().find(message => message.role === "assistant" && message.content.includes("建议："));
  return latest?.content || "";
});
const currentSpeakerNotes = computed(() => store.currentSlide?.speakerNotes || "");
const speakerNotesCharCount = computed(() => currentSpeakerNotes.value.trim().length);
const speakerNotesReadMinutes = computed(() => Math.max(1, Math.ceil(speakerNotesCharCount.value / 260)));
const deckSpeakerNotesCount = computed(() => store.slides.filter(slide => slide.speakerNotes?.trim()).length);
const speakerNotesCoverage = computed(() => {
  if (!store.slides.length) return 0;
  return Math.round((deckSpeakerNotesCount.value / store.slides.length) * 100);
});
const recordingOptionSummary = computed(() => {
  const items = [
    recordingScreenEnabled.value ? "屏幕" : "无屏幕",
    recordingMicrophoneEnabled.value ? "麦克风" : "静音",
    recordingCameraEnabled.value ? "摄像头" : "无摄像头",
    recordingQualityOptions.find(item => item.value === recordingQuality.value)?.label || "1080p"
  ];
  return items.join(" · ");
});
const presentationRecordingActionTitle = computed(() => {
  if (presentModeBusy.value) return "正在进入录制预览";
  if (!store.slides.length) return "暂无可录制的幻灯片";
  if (isPresentationActionBusy.value) return "生成完成后可录制";
  return "进入录制预览";
});
const presentationOnlyActionTitle = computed(() => {
  if (presentModeBusy.value) return "正在进入演示预览";
  if (!store.slides.length) return "暂无可演示的幻灯片";
  if (isPresentationActionBusy.value) return "生成完成后可演示";
  return "仅演示";
});
const presentationSaveStatusLabel = computed(() => {
  if (presentationSaveStatus.value === "saving") return "正在保存";
  if (presentationSaveStatus.value === "error") return "保存失败";
  if (presentationSaveStatus.value === "saved") return "已保存";
  return "";
});
function presentationSlideStyle(slide: PptSlide | null | undefined): CSSProperties {
  if (!slide) return {};
  const background = presentationSlideBackgrounds.value[slide.id];
  return background ? { background } : {};
}

function presentationSlideTitle(slide: PptSlide | null | undefined) {
  if (!slide) return "";
  const iconName = presentationSlideIcons.value[slide.id];
  const icon = presentationIconOptions.find(item => item.name === iconName);
  return icon ? `${icon.glyph} ${slide.title}` : slide.title;
}

function updatePresentationViewportSize() {
  if (typeof window === "undefined") return;
  const visualViewport = window.visualViewport;
  const width = Math.round(visualViewport?.width || window.innerWidth || document.documentElement.clientWidth || 0);
  const height = Math.round(visualViewport?.height || window.innerHeight || document.documentElement.clientHeight || 0);
  presentationViewportSize.value = { width, height };
}

const isPresentationSearchablePanel = computed(() => ["content", "elements", "charts", "diagrams", "embed"].includes(presentationRightPanel.value));
const activePanelCategoryOptions = computed<PresentationPanelCategoryOption[]>(() => {
  if (presentationRightPanel.value === "content") return presentationTextCategoryOptions;
  if (presentationRightPanel.value === "elements") return presentationElementCategoryOptions;
  if (presentationRightPanel.value === "charts") return presentationChartCategoryOptions;
  if (presentationRightPanel.value === "diagrams") return presentationDiagramCategoryOptions;
  if (presentationRightPanel.value === "embed") return presentationEmbedCategoryOptions;
  return [allPanelCategoryOption];
});
const filteredPresentationTextBlocks = computed(() => filterPresentationBlocks(presentationTextBlocks));
const filteredPresentationElementBlocks = computed(() => filterPresentationBlocks(presentationElementBlocks));
const filteredPresentationChartBlocks = computed(() => filterPresentationBlocks(presentationChartBlocks));
const filteredPresentationDiagramBlocks = computed(() => filterPresentationBlocks(presentationDiagramBlocks));
const groupedFilteredPresentationDiagramBlocks = computed(() => {
  const groups: Array<{ category: string; label: string; items: InsertablePresentationBlock[] }> = [];
  for (const block of filteredPresentationDiagramBlocks.value) {
    const previousGroup = groups.at(-1);
    const label = presentationDiagramCategoryOptions.find(item => item.value === block.category)?.label || block.category;
    if (previousGroup?.category === block.category) {
      previousGroup.items.push(block);
    } else {
      groups.push({ category: block.category, label, items: [block] });
    }
  }
  return groups;
});
const visiblePresentationDiagramBlocks = computed(() => (
  groupedFilteredPresentationDiagramBlocks.value
    .filter(group => !isPresentationDiagramCategoryCollapsed(group.category))
    .flatMap(group => group.items)
));
const filteredPresentationEmbedBlocks = computed(() => filterPresentationBlocks(presentationEmbedBlocks));
const canUndoPresentation = computed(() => presentationUndoStack.value.length > 0);
const canRedoPresentation = computed(() => presentationRedoStack.value.length > 0);
const presentButtonLabel = computed(() => {
  if (presentModeBusy.value) return presentationViewMode.value === "present" ? "正在退出" : "正在演示";
  return presentationViewMode.value === "present" ? "退出演示" : "演示";
});
const exportButtonLabel = computed(() => {
  if (exportBusy.value) return "正在导出演示文稿";
  if (!store.slides.length) return "暂无可导出的幻灯片";
  if (isPresentationActionBusy.value) return "生成完成后可导出演示文稿";
  return "导出演示文稿";
});
const shareButtonLabel = computed(() => {
  if (!activePresentationId.value && !store.taskId) return "请先生成或打开演示文稿";
  return "复制或打开分享设置";
});
const workspaceStatusLabel = computed(() => store.statusText);
const workspacePrimaryDisabled = computed(() => ["outlining", "pending", "generating", "rendering"].includes(store.status));
const workspacePrimaryLabel = computed(() => {
  if (store.status === "outlining") return "正在生成大纲...";
  if (["pending", "generating", "rendering"].includes(store.status)) return "正在生成PPT...";
  if (store.status === "success") return "重新生成PPT";
  if (hasOutline.value) return "生成完整PPT";
  return "生成大纲";
});
const workspacePrimaryButtonTitle = computed(() => {
  if (store.status === "outlining") return "正在生成大纲，请稍候";
  if (["pending", "generating", "rendering"].includes(store.status)) return "正在生成 PPT，请稍候";
  if (store.status === "success") return "重新生成 PPT";
  if (hasOutline.value) return "确认大纲并生成完整 PPT";
  return "生成演示大纲";
});
onMounted(() => {
  loadFavoriteTaskIds();
  loadPresentationSidebarState();
  updatePresentationViewportSize();
  syncGenerationWorkspaceFromPath();
  document.addEventListener("pointerdown", handlePptPillDropdownOutsidePointerDown, true);
  window.addEventListener("popstate", syncGenerationWorkspaceFromPath);
  window.addEventListener("keydown", handleGenerationWorkspaceKeydown);
  window.addEventListener("keydown", handlePresentationWorkspaceKeydown);
  window.addEventListener("keydown", handlePresentModeKeydown);
  window.addEventListener("keydown", handlePresentationPanelExternalArrowKeydown, true);
  window.addEventListener("wheel", handlePresentModeWheel, { passive: false });
  window.addEventListener("mousemove", handlePresentModeMouseMove);
  window.addEventListener("resize", updatePresentationViewportSize);
  window.addEventListener("orientationchange", updatePresentationViewportSize);
  window.visualViewport?.addEventListener("resize", updatePresentationViewportSize);
  void store.initialize().then(() => {
    hydrateWorkspaceFromHistory();
  });
});

onBeforeUnmount(() => {
  document.removeEventListener("pointerdown", handlePptPillDropdownOutsidePointerDown, true);
  window.removeEventListener("popstate", syncGenerationWorkspaceFromPath);
  window.removeEventListener("keydown", handleGenerationWorkspaceKeydown);
  window.removeEventListener("keydown", handlePresentationWorkspaceKeydown);
  window.removeEventListener("keydown", handlePresentModeKeydown);
  window.removeEventListener("keydown", handlePresentationPanelExternalArrowKeydown, true);
  window.removeEventListener("wheel", handlePresentModeWheel);
  window.removeEventListener("mousemove", handlePresentModeMouseMove);
  window.removeEventListener("resize", updatePresentationViewportSize);
  window.removeEventListener("orientationchange", updatePresentationViewportSize);
  window.visualViewport?.removeEventListener("resize", updatePresentationViewportSize);
  stopSidebarResize();
  clearGenerationDraftSaveTimer();
  clearPresentationPanelLoadTimer();
  clearPresentationSaveTimer();
  void cleanupPresentationModeSideEffects();
});

watch(
  () => store.taskId,
  (taskId) => {
    if (!taskId) return;
    if (isPresentationWorkspace.value && activePresentationId.value !== taskId) {
      void openPresentationWorkspace(taskId, { scroll: false, replace: true });
      return;
    }
    if (isGenerationWorkspace.value && activeGenerationId.value !== taskId) {
      void openGenerationWorkspace(taskId, { scroll: false, replace: true });
    }
  }
);

watch(
  () => store.currentSlideIndex,
  (index) => {
    if (!isPresentationWorkspace.value || presentationSidebarCollapsed.value) return;
    void scrollPresentationSidebarThumbIntoView(index);
  }
);

watch(
  [
    () => store.prompt,
    () => store.slideCount,
    () => store.language,
    () => store.tone,
    () => store.textContent,
    () => store.audience,
    () => store.scenario,
    () => store.generationAspectRatio,
    () => store.theme,
    () => store.autoThemeEnabled,
    () => store.enableWebSearch,
    () => store.imageSource,
    () => store.textModel,
    () => store.imageModel
  ],
  scheduleGenerationDraftSave
);

watch(
  [presentationRightPanel, presentationRightPanelOpen],
  ([panel, isOpen]) => {
    schedulePresentationPanelContentLoad(panel, isOpen);
  },
  { immediate: true }
);

function generationIdFromPath() {
  if (typeof window === "undefined") return "";
  const match = canonicalPptPath(window.location.pathname).replace(/\/$/, "").match(/\/app\/ppt-generation\/generate\/([^/]+)$/);
  return match?.[1] ? decodeURIComponent(match[1]) : "";
}

function presentationIdFromPath() {
  if (typeof window === "undefined") return "";
  const match = canonicalPptPath(window.location.pathname).replace(/\/$/, "").match(/\/app\/ppt-generation\/presentation\/([^/]+)$/);
  return match?.[1] ? decodeURIComponent(match[1]) : "";
}

function syncGenerationWorkspaceFromPath() {
  const presentationId = presentationIdFromPath();
  const generationId = generationIdFromPath();
  isPresentationWorkspace.value = Boolean(presentationId);
  isGenerationWorkspace.value = Boolean(generationId) && !presentationId;
  activePresentationId.value = presentationId;
  activeGenerationId.value = generationId;
  if (presentationId || generationId) hydrateWorkspaceFromHistory();
}

function hydrateWorkspaceFromHistory() {
  const activeTaskId = activePresentationId.value || activeGenerationId.value;
  if (!activeTaskId || store.taskId === activeTaskId) return;
  const historyItem = store.history.find((item) => item.taskId === activeTaskId);
  if (historyItem) {
    store.loadHistoryItem(historyItem);
  }
}

function closePresentationFloatingControls(options: { keepKeyboardDialog?: boolean } = {}) {
  showPresentationMenu.value = false;
  showExportDialog.value = false;
  showShareDialog.value = false;
  showZoomMenu.value = false;
  showPresentationHelpMenu.value = false;
  showGenerationHelpMenu.value = false;
  showElementTypeMenu.value = false;
  showElementAiEditor.value = false;
  elementAiError.value = "";
  slideMoreMenuIndex.value = null;
  slidePaletteMenuIndex.value = null;
  slideMagicMenuIndex.value = null;
  slideMagicError.value = "";
  if (!options.keepKeyboardDialog) {
    showKeyboardShortcutsDialog.value = false;
    showGenerationKeyboardShortcutsDialog.value = false;
  }
}

async function openGenerationWorkspace(
  generationId = "",
  options: { scroll?: boolean; replace?: boolean } = {}
) {
  const nextId = generationId || store.taskId || activeGenerationId.value || `draft_${Date.now()}`;
  isGenerationWorkspace.value = true;
  isPresentationWorkspace.value = false;
  activeGenerationId.value = nextId;
  activePresentationId.value = "";
  presentationViewMode.value = "edit";
  presentationRightPanel.value = "content";
  presentationRightPanelOpen.value = false;
  resetPresentationPanelFilters();
  void cleanupPresentationModeSideEffects();
  closePresentationFloatingControls();
  slideMoreMenuIndex.value = null;
  showConfig.value = false;
  showSlideCountMenu.value = false;
  showFormatMenu.value = false;
  showLanguageMenu.value = false;
  showMoreMenu.value = false;
  showModelMenu.value = false;
  showHistoryFilterMenu.value = false;
  if (typeof window !== "undefined") {
    const nextPath = activePptPath(`/app/ppt-generation/generate/${encodeURIComponent(nextId)}`);
    const currentPath = window.location.pathname.replace(/\/$/, "");
    if (currentPath !== nextPath) {
      if (options.replace) window.history.replaceState({}, "", nextPath);
      else window.history.pushState({}, "", nextPath);
    }
  }
  await nextTick();
  if (options.scroll !== false) {
    generationWorkspaceRef.value?.scrollIntoView({ block: "start", behavior: "smooth" });
  }
}

async function openPresentationWorkspace(
  presentationId = "",
  options: { scroll?: boolean; replace?: boolean } = {}
) {
  const nextId = presentationId || store.taskId || activePresentationId.value || activeGenerationId.value || `draft_${Date.now()}`;
  isGenerationWorkspace.value = false;
  isPresentationWorkspace.value = true;
  activePresentationId.value = nextId;
  activeGenerationId.value = "";
  presentationRightPanelOpen.value = false;
  presentationRightPanel.value = "content";
  resetPresentationPanelFilters();
  closePresentationFloatingControls();
  slideMoreMenuIndex.value = null;
  showConfig.value = false;
  showSlideCountMenu.value = false;
  showFormatMenu.value = false;
  showLanguageMenu.value = false;
  showMoreMenu.value = false;
  showModelMenu.value = false;
  showHistoryFilterMenu.value = false;
  if (typeof window !== "undefined") {
    const nextPath = activePptPath(`/app/ppt-generation/presentation/${encodeURIComponent(nextId)}`);
    const currentPath = window.location.pathname.replace(/\/$/, "");
    if (currentPath !== nextPath) {
      if (options.replace) window.history.replaceState({}, "", nextPath);
      else window.history.pushState({}, "", nextPath);
    }
  }
  await nextTick();
  if (options.scroll !== false) {
    presentationWorkspaceRef.value?.scrollIntoView({ block: "start", behavior: "smooth" });
  }
}

function pushPptHomePath() {
  if (typeof window === "undefined") return;
  const currentPath = window.location.pathname.replace(/\/$/, "");
  const homePath = activePptPath("/app/ppt-generation");
  if (currentPath !== homePath) {
    window.history.pushState({}, "", homePath);
  }
}

function historyTime(value?: string) {
  if (!value) return 0;
  const time = new Date(value).getTime();
  return Number.isNaN(time) ? 0 : time;
}

function cloneSlides(slides: PptSlide[]) {
  return slides.map((slide) => ({ ...slide, bulletPoints: [...slide.bulletPoints] }));
}

function clonePresentationElementStyles(
  styles: Record<string, Partial<Record<PresentationElementKind, PresentationElementStyle>>>
) {
  return Object.fromEntries(
    Object.entries(styles).map(([slideId, kinds]) => [
      slideId,
      Object.fromEntries(
        Object.entries(kinds).map(([kind, style]) => [kind, { ...style }])
      ) as Partial<Record<PresentationElementKind, PresentationElementStyle>>
    ])
  );
}

function createPresentationEditorSnapshot(): PresentationEditorSnapshot {
  return {
    slides: cloneSlides(store.slides),
    slideBackgrounds: { ...presentationSlideBackgrounds.value },
    elementStyles: clonePresentationElementStyles(presentationElementStyles.value),
    deckWidth: presentationDeckWidth.value,
    globalAlignment: presentationGlobalAlignment.value,
    typographyScale: presentationTypographyScale.value,
    theme: store.theme,
    backgroundMode: presentationBackgroundMode.value,
    backgroundImageUrl: presentationBackgroundImageUrl.value,
    currentSlideIndex: store.currentSlideIndex
  };
}

function restorePresentationEditorSnapshot(snapshot: PresentationEditorSnapshot) {
  store.slides = cloneSlides(snapshot.slides);
  normalizePresentationSlides(store.slides);
  store.theme = snapshot.theme;
  presentationSlideBackgrounds.value = { ...snapshot.slideBackgrounds };
  presentationElementStyles.value = clonePresentationElementStyles(snapshot.elementStyles);
  presentationDeckWidth.value = snapshot.deckWidth;
  presentationGlobalAlignment.value = snapshot.globalAlignment;
  presentationTypographyScale.value = snapshot.typographyScale;
  presentationBackgroundMode.value = snapshot.backgroundMode;
  presentationBackgroundImageUrl.value = snapshot.backgroundImageUrl;
  store.selectSlide(Math.min(Math.max(snapshot.currentSlideIndex, 0), Math.max(store.slides.length - 1, 0)));
  selectedPresentationElement.value = null;
  showElementTypeMenu.value = false;
  showElementAiEditor.value = false;
}

function normalizePresentationSlides(slides: PptSlide[]) {
  slides.forEach((slide, index) => {
    slide.page = index + 1;
  });
  store.slideCount = slides.length || store.slideCount;
  if (store.currentSlideIndex >= slides.length) {
    store.currentSlideIndex = Math.max(0, slides.length - 1);
  }
}

function loadPresentationSidebarState() {
  if (typeof window === "undefined") return;
  const storedWidth = Number(window.localStorage.getItem(presentationSidebarWidthStorageKey));
  if (Number.isFinite(storedWidth) && storedWidth >= 100 && storedWidth <= 300) {
    presentationSidebarWidth.value = storedWidth;
  }
  presentationSidebarCollapsed.value = window.localStorage.getItem(presentationSidebarCollapsedStorageKey) === "true";
}

function setPresentationSidebarCollapsed(value: boolean) {
  presentationSidebarCollapsed.value = value;
  if (typeof window !== "undefined") {
    window.localStorage.setItem(presentationSidebarCollapsedStorageKey, String(value));
  }
}

function setPresentationSidebarWidth(value: number) {
  const nextWidth = Math.min(300, Math.max(100, Math.round(value)));
  presentationSidebarWidth.value = nextWidth;
  if (typeof window !== "undefined") {
    window.localStorage.setItem(presentationSidebarWidthStorageKey, String(nextWidth));
  }
}

function startSidebarResize(event: PointerEvent) {
  if (presentationSidebarCollapsed.value) return;
  event.preventDefault();
  presentationSidebarResizing.value = true;
  presentationSidebarResizeStartX = event.clientX;
  presentationSidebarResizeStartWidth = presentationSidebarWidth.value;
  window.addEventListener("pointermove", handleSidebarResizeMove);
  window.addEventListener("pointerup", stopSidebarResize);
  window.addEventListener("pointercancel", stopSidebarResize);
}

function handleSidebarResizeMove(event: PointerEvent) {
  if (!presentationSidebarResizing.value) return;
  const nextWidth = presentationSidebarResizeStartWidth + event.clientX - presentationSidebarResizeStartX;
  presentationSidebarWidth.value = Math.min(300, Math.max(100, Math.round(nextWidth)));
}

function stopSidebarResize() {
  if (presentationSidebarResizing.value && typeof window !== "undefined") {
    window.localStorage.setItem(presentationSidebarWidthStorageKey, String(presentationSidebarWidth.value));
  }
  presentationSidebarResizing.value = false;
  if (typeof window === "undefined") return;
  window.removeEventListener("pointermove", handleSidebarResizeMove);
  window.removeEventListener("pointerup", stopSidebarResize);
  window.removeEventListener("pointercancel", stopSidebarResize);
}

async function selectPresentationSlide(index: number) {
  store.selectSlide(index);
  selectedPresentationElement.value = null;
  showElementTypeMenu.value = false;
  showElementAiEditor.value = false;
  elementAiError.value = "";
  await scrollPresentationSlideIntoView(index);
}

function selectPresentationSlideOnly(index: number) {
  store.selectSlide(index);
  selectedPresentationElement.value = null;
  showElementTypeMenu.value = false;
  showElementAiEditor.value = false;
  elementAiError.value = "";
  slideMoreMenuIndex.value = null;
  slidePaletteMenuIndex.value = null;
  slideMagicMenuIndex.value = null;
}

function selectPresentationElement(index: number, kind: PresentationElementKind) {
  store.selectSlide(index);
  selectedPresentationElement.value = { slideIndex: index, kind };
  showElementTypeMenu.value = false;
  showElementAiEditor.value = false;
  elementAiError.value = "";
  slideMoreMenuIndex.value = null;
  slidePaletteMenuIndex.value = null;
  slideMagicMenuIndex.value = null;
}

function handlePresentationElementKeydown(event: KeyboardEvent, index: number, kind: PresentationElementKind) {
  if (event.key !== "Enter" && event.key !== " ") return;
  event.preventDefault();
  event.stopPropagation();
  selectPresentationElement(index, kind);
}

function isPresentationElementSelected(index: number, kind: PresentationElementKind) {
  return selectedPresentationElement.value?.slideIndex === index && selectedPresentationElement.value?.kind === kind;
}

function isPresentationElementToolbarVisible(index: number) {
  return presentationViewMode.value === "edit" && selectedPresentationElement.value?.slideIndex === index;
}

function presentationElementStyle(slideId: string, kind: PresentationElementKind): CSSProperties {
  const style = presentationElementStyles.value[slideId]?.[kind];
  const nextStyle: CSSProperties = {};
  if (style?.align) nextStyle.textAlign = style.align;
  if (style?.color) nextStyle.color = style.color;
  if (style?.emphasis) {
    nextStyle.background = "rgba(255, 255, 255, 0.28)";
    nextStyle.borderRadius = "10px";
    nextStyle.padding = kind === "title" ? "6px 10px" : "8px 10px";
  }
  return nextStyle;
}

function updateSelectedPresentationElementStyle(patch: PresentationElementStyle) {
  const selection = selectedPresentationElement.value;
  const slide = selection ? store.slides[selection.slideIndex] : null;
  if (!selection || !slide) return;
  const currentSlideStyles = presentationElementStyles.value[slide.id] || {};
  const currentStyle = currentSlideStyles[selection.kind] || {};
  presentationElementStyles.value = {
    ...presentationElementStyles.value,
    [slide.id]: {
      ...currentSlideStyles,
      [selection.kind]: {
        ...currentStyle,
        ...patch
      }
    }
  };
}

function toggleElementTypeMenu() {
  showElementTypeMenu.value = !showElementTypeMenu.value;
  if (showElementTypeMenu.value) {
    showElementAiEditor.value = false;
    elementAiError.value = "";
  }
}

function selectPresentationElementKind(kind: PresentationElementKind) {
  if (!selectedPresentationElement.value) return;
  const slideIndex = selectedPresentationElement.value.slideIndex;
  selectedPresentationElement.value = { slideIndex, kind };
  showElementTypeMenu.value = false;
  showElementAiEditor.value = false;
  elementAiError.value = "";
}

function setPresentationElementAlignment(align: PresentationElementAlign) {
  updateSelectedPresentationElementStyle({ align });
}

function setPresentationElementColor(color: string) {
  updateSelectedPresentationElementStyle({ color });
}

function togglePresentationElementEmphasis() {
  updateSelectedPresentationElementStyle({ emphasis: !selectedPresentationElementStyle.value?.emphasis });
}

function cycleSelectedSlideLayout() {
  const selection = selectedPresentationElement.value;
  if (!selection) return;
  runSlideMagicAction(selection.slideIndex, "layout");
  selectedPresentationElement.value = { slideIndex: selection.slideIndex, kind: selection.kind };
}

function selectedSlideForElementAction() {
  const selection = selectedPresentationElement.value;
  if (!selection) return null;
  const slide = store.slides[selection.slideIndex];
  if (!slide) return null;
  return { selection, slide };
}

function getPresentationElementText(slide: PptSlide, kind: PresentationElementKind) {
  if (kind === "title") return slide.title;
  if (kind === "content") return slide.content;
  if (kind === "bullets") return slide.bulletPoints.join("；");
  return slide.speakerNotes || "";
}

function buildElementAiDefaultPrompt(slide: PptSlide, kind: PresentationElementKind) {
  const label = presentationElementKinds.find(item => item.value === kind)?.label || "内容";
  const text = getPresentationElementText(slide, kind).trim();
  return text ? `优化选中的${label}：${text}` : `为当前页补充一段${label}内容`;
}

function openSelectedElementAiEditor() {
  const result = selectedSlideForElementAction();
  if (!result) {
    ElMessage.warning("请先选择要编辑的内容");
    return;
  }
  showElementTypeMenu.value = false;
  showElementAiEditor.value = true;
  elementAiError.value = "";
  if (!elementAiPrompt.value.trim()) {
    elementAiPrompt.value = buildElementAiDefaultPrompt(result.slide, result.selection.kind);
  }
  nextTick(() => {
    window.requestAnimationFrame(() => {
      document.querySelector<HTMLTextAreaElement>("#ppt-element-ai-editor textarea")?.focus();
    });
  });
}

function closeSelectedElementAiEditor() {
  if (elementAiBusy.value) return;
  showElementAiEditor.value = false;
  elementAiError.value = "";
}

function cancelSelectedElementAiEdit() {
  elementAiRequestToken += 1;
  elementAiBusy.value = false;
  showElementAiEditor.value = false;
  elementAiError.value = "";
}

function useElementAiQuickSuggestion(prompt: string) {
  elementAiPrompt.value = prompt;
  elementAiError.value = "";
  nextTick(() => {
    window.requestAnimationFrame(() => {
      document.querySelector<HTMLTextAreaElement>("#ppt-element-ai-editor textarea")?.focus();
    });
  });
}

function buildSelectedElementAiReplacement(prompt: string, slide: PptSlide, kind: PresentationElementKind) {
  const cleanPrompt = prompt.replace(/\s+/g, " ").trim();
  const currentText = getPresentationElementText(slide, kind).trim();
  if (kind === "title") {
    const base = currentText.replace(/[。！？!?]$/, "");
    if (/简洁|简单|short|clear/i.test(cleanPrompt)) {
      return `${base || "核心结论"}：一句话说明价值`;
    }
    return `${base || "当前页"}：更清晰的演示表达`;
  }
  if (kind === "content") {
    const base = currentText || `围绕“${slide.title}”补充正文说明。`;
    if (/路演|融资|项目/.test(cleanPrompt)) {
      return `${base.replace(/[。！？!?]$/, "。")}本页重点突出痛点、方案和增长空间，方便观众快速判断项目价值。`;
    }
    return `${base.replace(/[。！？!?]$/, "。")}已压缩为更适合演示的表达，先给结论，再说明关键依据。`;
  }
  if (kind === "bullets") {
    if (/补充|更多|说服/.test(cleanPrompt)) {
      return ["核心问题：明确当前业务痛点", "关键方案：展示可执行路径", "预期结果：给出可衡量变化"];
    }
    return slide.bulletPoints.length ? slide.bulletPoints.map(point => `${point.replace(/：更清晰$/, "")}：更清晰`) : ["核心观点：更清晰", "关键证据：更聚焦", "行动建议：更明确"];
  }
  const baseNotes = currentText || "讲稿提示";
  return `${baseNotes.replace(/[。！？!?]$/, "")}。请用更自然的口语节奏讲述，先抛出结论，再补充一个具体例子。`;
}

function applySelectedElementAiReplacement(selection: { slideIndex: number; kind: PresentationElementKind }, slide: PptSlide, replacement: string | string[]) {
  if (selection.kind === "title") {
    store.updateSlide(selection.slideIndex, { title: String(replacement) });
  } else if (selection.kind === "content") {
    store.updateSlide(selection.slideIndex, { content: String(replacement) });
  } else if (selection.kind === "bullets") {
    store.updateSlide(selection.slideIndex, { bulletPoints: Array.isArray(replacement) ? replacement : [String(replacement)] });
  } else {
    store.updateSlide(selection.slideIndex, { speakerNotes: String(replacement) });
  }
  selectedPresentationElement.value = { slideIndex: selection.slideIndex, kind: selection.kind };
}

async function submitSelectedElementAiEdit(prompt?: string) {
  const result = selectedSlideForElementAction();
  const userPrompt = (prompt || elementAiPrompt.value).trim();
  if (!result) {
    elementAiError.value = "请先选择要编辑的内容";
    return;
  }
  if (!userPrompt) {
    elementAiError.value = "请输入修改要求";
    return;
  }
  if (elementAiBusy.value) return;
  const { selection, slide } = result;
  const requestToken = ++elementAiRequestToken;
  elementAiBusy.value = true;
  elementAiError.value = "";
  await new Promise(resolve => window.setTimeout(resolve, 520));
  if (requestToken !== elementAiRequestToken) return;
  try {
    rememberPresentationSnapshot();
    applySelectedElementAiReplacement(selection, slide, buildSelectedElementAiReplacement(userPrompt, slide, selection.kind));
    elementAiPrompt.value = "";
    showElementAiEditor.value = false;
    ElMessage.success("已按 AI 编辑结果更新选中元素");
  } catch {
    elementAiError.value = "AI 编辑失败，请重试";
  } finally {
    if (requestToken === elementAiRequestToken) {
      elementAiBusy.value = false;
    }
  }
}

function duplicateSelectedPresentationElement() {
  const result = selectedSlideForElementAction();
  if (!result) return;
  const { selection, slide } = result;
  rememberPresentationSnapshot();
  if (selection.kind === "title") {
    store.updateSlide(selection.slideIndex, { content: `${slide.content}\n\n${slide.title}`.trim() });
  } else if (selection.kind === "content") {
    store.updateSlide(selection.slideIndex, { content: `${slide.content}\n\n${slide.content}`.trim() });
  } else if (selection.kind === "bullets") {
    store.updateSlide(selection.slideIndex, { bulletPoints: [...slide.bulletPoints, ...slide.bulletPoints] });
  } else {
    store.updateSlide(selection.slideIndex, { speakerNotes: `${slide.speakerNotes || ""}\n${slide.speakerNotes || "讲稿副本"}`.trim() });
  }
  ElMessage.success("已复制选中元素内容");
}

function deleteSelectedPresentationElement() {
  const result = selectedSlideForElementAction();
  if (!result) return;
  const { selection } = result;
  rememberPresentationSnapshot();
  if (selection.kind === "title") {
    store.updateSlide(selection.slideIndex, { title: "未命名标题" });
  } else if (selection.kind === "content") {
    store.updateSlide(selection.slideIndex, { content: "" });
  } else if (selection.kind === "bullets") {
    store.updateSlide(selection.slideIndex, { bulletPoints: [] });
  } else {
    store.updateSlide(selection.slideIndex, { speakerNotes: "" });
  }
  showElementTypeMenu.value = false;
  showElementAiEditor.value = false;
  elementAiError.value = "";
  selectedPresentationElement.value = null;
  ElMessage.success("已删除选中元素内容");
}

async function scrollPresentationSlideIntoView(index: number) {
  await nextTick();
  document
    .querySelector<HTMLElement>(`[data-ppt-slide-index="${index}"]`)
    ?.scrollIntoView({ block: "center", behavior: "smooth" });
}

async function scrollPresentationSidebarThumbIntoView(index: number) {
  await nextTick();
  document
    .querySelector<HTMLElement>(`[data-ppt-sidebar-slide-index="${index}"]`)
    ?.scrollIntoView({ block: "nearest", behavior: "smooth" });
}

function createInsertedSlide(index: number): PptSlide {
  const baseSlide = store.slides[index];
  const page = Math.max(1, index + 1);
  return {
    id: `slide_insert_${Date.now()}_${Math.random().toString(36).slice(2, 6)}`,
    page,
    title: baseSlide ? `${baseSlide.title} · 新增页` : "新增幻灯片",
    content: baseSlide ? `围绕「${baseSlide.title}」补充一页新的结构化内容。` : "请输入本页正文说明。",
    bulletPoints: baseSlide ? ["核心观点", "关键证据", "下一步行动"] : ["主题名称", "主要内容", "结论建议"],
    imageUrl: "",
    layout: baseSlide?.layout === "cover" ? "content" : baseSlide?.layout || "content",
    speakerNotes: "新增页讲稿占位，后续可由 AI 自动扩写。"
  };
}

async function addFirstPresentationSlide() {
  if (isPresentationActionBusy.value || store.slides.length) return;
  rememberPresentationSnapshot();
  const title = workspaceTitle.value || "无标题演示文稿";
  store.prompt = store.prompt.trim() || title;
  store.status = "success";
  store.progress = 100;
  store.slides = [{
    id: `slide_first_${Date.now()}_${Math.random().toString(36).slice(2, 6)}`,
    page: 1,
    title,
    content: "请输入本页正文说明。",
    bulletPoints: ["主题名称", "主要内容", "结论建议"],
    imageUrl: "",
    layout: "cover",
    speakerNotes: "第一页讲稿占位，后续可由 AI 自动扩写。"
  }];
  normalizePresentationSlides(store.slides);
  store.selectSlide(0);
  await scrollPresentationSlideIntoView(0);
  ElMessage.success("已添加第一页幻灯片");
}

async function insertSlideAt(index: number, position: SlideInsertPosition) {
  if (isPresentationActionBusy.value) return;
  const safeIndex = Math.min(Math.max(index, 0), Math.max(store.slides.length - 1, 0));
  const insertIndex = position === "before" ? safeIndex : safeIndex + 1;
  rememberPresentationSnapshot();
  store.slides.splice(insertIndex, 0, createInsertedSlide(safeIndex));
  normalizePresentationSlides(store.slides);
  store.selectSlide(insertIndex);
  selectedPresentationElement.value = null;
  showElementTypeMenu.value = false;
  slideMoreMenuIndex.value = null;
  slidePaletteMenuIndex.value = null;
  slideMagicMenuIndex.value = null;
  await scrollPresentationSlideIntoView(insertIndex);
  ElMessage.success("已新增一页幻灯片");
}

function rememberPresentationSnapshot() {
  presentationUndoStack.value = [...presentationUndoStack.value.slice(-19), createPresentationEditorSnapshot()];
  presentationRedoStack.value = [];
  markPresentationSaving();
}

function clearPresentationSaveTimer() {
  if (presentationSaveTimer === null) return;
  window.clearTimeout(presentationSaveTimer);
  presentationSaveTimer = null;
}

function markPresentationSaving() {
  presentationSaveStatus.value = "saving";
  clearPresentationSaveTimer();
  presentationSaveTimer = window.setTimeout(() => {
    presentationSaveStatus.value = "saved";
    presentationSaveTimer = null;
  }, 700);
}

function clearGenerationDraftSaveTimer() {
  if (generationDraftSaveTimer === null) return;
  window.clearTimeout(generationDraftSaveTimer);
  generationDraftSaveTimer = null;
}

function scheduleGenerationDraftSave() {
  if (!isGenerationWorkspace.value) return;
  const draftId = activeGenerationId.value || store.taskId;
  if (!draftId.startsWith("draft_")) return;
  if (!store.prompt.trim() && !store.uploadedDocumentName) return;
  clearGenerationDraftSaveTimer();
  generationDraftSaveTimer = window.setTimeout(() => {
    generationDraftSaveTimer = null;
    void store.saveDraft(draftId);
  }, 180);
}

async function openHistorySearch() {
  isHistorySearchOpen.value = true;
  await nextTick();
  historySearchInputRef.value?.focus();
}

function closeHistorySearch() {
  historySearchQuery.value = "";
  isHistorySearchOpen.value = false;
}

function toggleHistoryFilterMenu() {
  showHistoryFilterMenu.value = !showHistoryFilterMenu.value;
  if (showHistoryFilterMenu.value) {
    showConfig.value = false;
    showSlideCountMenu.value = false;
    showFormatMenu.value = false;
    showLanguageMenu.value = false;
    showMoreMenu.value = false;
    showModelMenu.value = false;
  }
}

function selectHistorySort(value: HistorySortBy) {
  historySortBy.value = value;
  showHistoryFilterMenu.value = false;
}

function toggleFavoritesOnlyFilter() {
  showFavoritesOnlyFilter.value = !showFavoritesOnlyFilter.value;
  showHistoryFilterMenu.value = false;
}

function selectHistoryTypeFilter(value: HistoryTypeFilter) {
  historyTypeFilter.value = value;
  showHistoryFilterMenu.value = false;
}

function loadFavoriteTaskIds() {
  try {
    const rawValue = window.localStorage.getItem(favoriteHistoryStorageKey);
    if (!rawValue) return;
    const parsedValue = JSON.parse(rawValue);
    if (!Array.isArray(parsedValue)) return;
    favoriteTaskIds.value = parsedValue.filter((item): item is string => typeof item === "string");
  } catch {
    favoriteTaskIds.value = [];
  }
}

function saveFavoriteTaskIds(nextIds: string[]) {
  favoriteTaskIds.value = nextIds;
  window.localStorage.setItem(favoriteHistoryStorageKey, JSON.stringify(nextIds));
}

function isFavoriteTask(taskId: string) {
  return favoriteTaskIds.value.includes(taskId);
}

function toggleFavorite(item: PptHistoryItem) {
  if (isFavoriteTask(item.taskId)) {
    saveFavoriteTaskIds(favoriteTaskIds.value.filter((taskId) => taskId !== item.taskId));
    ElMessage.success("已取消收藏");
    return;
  }
  saveFavoriteTaskIds([item.taskId, ...favoriteTaskIds.value]);
  ElMessage.success("已加入收藏夹");
}

async function handlePrimaryGenerate() {
  if (!canSubmit.value) return;
  if (store.createMode === "blank") {
    await openGenerationWorkspace();
    await store.startBlankPpt();
    await openPresentationWorkspace(activeGenerationId.value || store.taskId);
    return;
  }
  prepareGenerationSetup();
  const draft = await store.saveDraft();
  await openGenerationWorkspace(draft.taskId);
}

async function handleWorkspacePrimaryAction() {
  if (workspacePrimaryDisabled.value) return;
  if (store.createMode === "blank" && !store.outline) {
    await store.startBlankPpt();
    return;
  }
  if (!hasOutline.value) {
    await store.generateOutlineFlow();
    return;
  }
  await store.confirmOutlineAndGeneratePpt();
  if (store.status === "success") {
    await openPresentationWorkspace(store.taskId || activeGenerationId.value);
  }
}

function handleClearPrompt() {
  store.prompt = "";
  store.selectedExample = "";
}

function handleExampleSelect(prompt: string) {
  store.selectExample(prompt);
  showConfig.value = false;
  showFormatMenu.value = false;
  showLanguageMenu.value = false;
  showMoreMenu.value = false;
  showModelMenu.value = false;
  showHistoryFilterMenu.value = false;
  isGenerationSettingsExpanded.value = false;
}

function applyHomeTemplate(template: PptHomeTemplate) {
  handleExampleSelect(template.prompt);
  ElMessage.success(`已套用「${template.title}」模板提示词`);
}

function applyHomeInspiration(item: PptHomeInspiration) {
  handleExampleSelect(item.prompt);
  ElMessage.success("已填入创作灵感");
}

function rotatePptInspirations() {
  const pageCount = Math.ceil(pptHomeInspirations.length / 5);
  pptInspirationPage.value = (pptInspirationPage.value + 1) % pageCount;
}

function handleUploadDocument(file?: File) {
  store.setUploadedDocument(file);
}

function closePptPillDropdownMenus() {
  showSlideCountMenu.value = false;
  showFormatMenu.value = false;
  showLanguageMenu.value = false;
  showMoreMenu.value = false;
  showModelMenu.value = false;
}

function handlePptPillDropdownOutsidePointerDown(event: PointerEvent) {
  const target = event.target;
  if (!(target instanceof Element)) {
    closePptPillDropdownMenus();
    return;
  }
  if (target.closest(".ppt-pill-dropdown")) return;
  closePptPillDropdownMenus();
}

function toggleSlideCountMenu() {
  showSlideCountMenu.value = !showSlideCountMenu.value;
  if (showSlideCountMenu.value) {
    showConfig.value = false;
    showFormatMenu.value = false;
    showLanguageMenu.value = false;
    showMoreMenu.value = false;
    showModelMenu.value = false;
    showHistoryFilterMenu.value = false;
  }
}

function selectSlideCount(count: number) {
  store.slideCount = count;
  showSlideCountMenu.value = false;
}

function toggleFormatMenu() {
  showFormatMenu.value = !showFormatMenu.value;
  if (showFormatMenu.value) {
    showConfig.value = false;
    showSlideCountMenu.value = false;
    showLanguageMenu.value = false;
    showMoreMenu.value = false;
    showModelMenu.value = false;
    showHistoryFilterMenu.value = false;
  }
}

function selectGenerationAspectRatio(value: PptGenerationAspectRatio) {
  store.generationAspectRatio = value;
  showFormatMenu.value = false;
}

function toggleLanguageMenu() {
  showLanguageMenu.value = !showLanguageMenu.value;
  if (showLanguageMenu.value) {
    showConfig.value = false;
    showSlideCountMenu.value = false;
    showFormatMenu.value = false;
    showMoreMenu.value = false;
    showModelMenu.value = false;
    showHistoryFilterMenu.value = false;
  }
}

function selectLanguage(value: PptLanguage) {
  store.language = value;
  showLanguageMenu.value = false;
}

function toggleMoreMenu() {
  showMoreMenu.value = !showMoreMenu.value;
  if (showMoreMenu.value) {
    showConfig.value = false;
    showSlideCountMenu.value = false;
    showFormatMenu.value = false;
    showLanguageMenu.value = false;
    showModelMenu.value = false;
    showHistoryFilterMenu.value = false;
  }
}

function toggleWebSearch() {
  store.enableWebSearch = !store.enableWebSearch;
}

function toggleAutoTheme() {
  store.autoThemeEnabled = !store.autoThemeEnabled;
}

function toggleModelMenu() {
  showModelMenu.value = !showModelMenu.value;
  if (showModelMenu.value) {
    showConfig.value = false;
    showSlideCountMenu.value = false;
    showFormatMenu.value = false;
    showLanguageMenu.value = false;
    showMoreMenu.value = false;
    showHistoryFilterMenu.value = false;
  }
}

function selectTextModel(value: string) {
  const model = store.textModels.find((item) => item.value === value);
  if (model?.disabled) return;
  store.textModel = value;
  showModelMenu.value = false;
}

function toggleConfigPanel() {
  showConfig.value = !showConfig.value;
  if (showConfig.value) {
    showSlideCountMenu.value = false;
    showFormatMenu.value = false;
    showLanguageMenu.value = false;
    showMoreMenu.value = false;
    showModelMenu.value = false;
    showHistoryFilterMenu.value = false;
  }
}

function resetComposer() {
  store.createMode = "ai";
  store.prompt = "";
  store.selectedExample = "";
  store.outline = null;
  store.slides = [];
  store.currentSlideIndex = 0;
  store.taskId = "";
  store.status = "idle";
  store.progress = 0;
  store.currentPage = 0;
  showConfig.value = false;
  showSlideCountMenu.value = false;
  showFormatMenu.value = false;
  showLanguageMenu.value = false;
  showMoreMenu.value = false;
  showModelMenu.value = false;
  showHistoryFilterMenu.value = false;
  isGenerationSettingsExpanded.value = false;
}

function prepareGenerationSetup() {
  store.error = null;
  store.outline = null;
  store.slides = [];
  store.currentSlideIndex = 0;
  store.taskId = "";
  store.status = "idle";
  store.progress = 0;
  store.currentPage = 0;
  store.imageSearchResults = [];
  store.imageSearchKeyword = "";
  store.imageGenerating = false;
  isGenerationSettingsExpanded.value = false;
}

async function handlePptHomeClick() {
  resetComposer();
  isGenerationWorkspace.value = false;
  isPresentationWorkspace.value = false;
  activeGenerationId.value = "";
  activePresentationId.value = "";
  presentationViewMode.value = "edit";
  void cleanupPresentationModeSideEffects();
  showPresentationMenu.value = false;
  showExportDialog.value = false;
  slideMoreMenuIndex.value = null;
  pushPptHomePath();
  await nextTick();
  const root = document.querySelector(".ppt-generate-page");
  root?.scrollIntoView({ block: "start", behavior: "smooth" });
  const prompt = document.querySelector<HTMLTextAreaElement>(".ppt-reference-prompt textarea");
  prompt?.focus();
}

function handleCreateNewFromLibrary() {
  showHistoryFilterMenu.value = false;
  void handlePptHomeClick();
}

function updateOutlineTitle(title: string) {
  if (!store.outline) return;
  store.outline.title = title;
}

function handlePresentationTitleInput(event: Event) {
  rememberPresentationSnapshot();
  const title = (event.target as HTMLInputElement).value;
  if (store.outline) store.outline.title = title;
  store.prompt = title;
}

function handleSlideSave(patch: Partial<PptSlide>) {
  rememberPresentationSnapshot();
  store.updateSlide(store.currentSlideIndex, patch);
  ElMessage.success("当前页已保存");
}

function normalizePresentationPanelSearchText(value: string): string {
  return value
    .toLowerCase()
    .replace(/[-_/&]+/g, " ")
    .replace(/\s+/g, " ")
    .trim();
}

function matchesPresentationPanelSearch(query: string, values: Array<string | null | undefined>): boolean {
  const normalizedQuery = normalizePresentationPanelSearchText(query);
  if (!normalizedQuery) return true;
  return values.some(value => normalizePresentationPanelSearchText(value || "").includes(normalizedQuery));
}

function filterPresentationBlocks(blocks: InsertablePresentationBlock[]) {
  const query = presentationPanelSearchQuery.value;
  const category = presentationPanelCategory.value;
  return blocks.filter(block => {
    const matchesCategory = category === "all" || block.category === category;
    const matchesSearch = matchesPresentationPanelSearch(query, [
      block.title,
      block.description,
      block.content,
      block.category,
      block.icon,
      block.iconLabel,
      ...(block.keywords || [])
    ]);
    return matchesCategory && matchesSearch;
  });
}

function resetPresentationPanelFilters() {
  presentationPanelSearchQuery.value = "";
  presentationPanelCategory.value = "all";
  presentationIconSearchQuery.value = "";
  showCurrentSlideTextEditor.value = false;
  showEmbedUrlCard.value = false;
}

function clearPresentationPanelSearch() {
  presentationPanelSearchQuery.value = "";
}

function selectPresentationRightPanel(panel: PresentationRightPanel) {
  presentationRightPanel.value = panel;
  presentationRightPanelOpen.value = true;
  resetPresentationPanelFilters();
  closePresentationFloatingControls();
  slideMoreMenuIndex.value = null;
}

function presentationToolPanelGroup(panel: PresentationRightPanel) {
  return referenceRightToolPanelSet.has(panel) ? "reference" : "extension";
}

function isPresentationToolGroupStart(panel: PresentationRightPanel) {
  return panel === presentationOrderedToolPanels.find(item => !referenceRightToolPanelSet.has(item.value))?.value;
}

function isPresentationToolPanelActive(panel: PresentationRightPanel) {
  if (panel === "record") return presentationViewMode.value === "present" && recordingWantsToRecord.value;
  return presentationRightPanelOpen.value && presentationRightPanel.value === panel;
}

function isPresentationToolPanelDisabled(panel: PresentationRightPanel) {
  return panel === "record" && (presentModeBusy.value || isPresentationActionBusy.value || !store.slides.length);
}

function presentationToolButtonTitle(item: (typeof presentationToolPanels)[number]) {
  if (item.value !== "record") return item.label;
  if (presentModeBusy.value) return "正在进入录制预览";
  if (!store.slides.length) return "暂无可录制的幻灯片";
  if (isPresentationActionBusy.value) return "生成完成后可录制";
  return item.label;
}

async function handlePresentationToolPanelClick(panel: PresentationRightPanel) {
  if (isPresentationToolPanelDisabled(panel)) return;
  if (panel === "record") {
    presentationRightPanelOpen.value = false;
    resetPresentationPanelFilters();
    await openRecordingPreview();
    return;
  }
  selectPresentationRightPanel(panel);
}

function closePresentationRightPanel() {
  presentationRightPanelOpen.value = false;
  resetPresentationPanelFilters();
}

function applySlideLayout(layout: PptSlideLayout) {
  if (!store.currentSlide) {
    ElMessage.warning("请先选择一页幻灯片");
    return;
  }
  rememberPresentationSnapshot();
  store.updateSlide(store.currentSlideIndex, { layout });
  selectedPresentationElement.value = null;
  showElementAiEditor.value = false;
  elementAiError.value = "";
  ElMessage.success(`已切换为${presentationLayoutOptions.find(item => item.value === layout)?.label || "当前"}布局`);
}

function applyNextSlideLayout() {
  const slide = store.currentSlide;
  if (!slide) return;
  const currentLayoutIndex = slideLayoutCycle.indexOf(slide.layout);
  const nextLayout = slideLayoutCycle[(currentLayoutIndex + 1 + slideLayoutCycle.length) % slideLayoutCycle.length];
  applySlideLayout(nextLayout);
}

function applySlideBackground(background: string) {
  const slide = store.currentSlide;
  if (!slide) {
    ElMessage.warning("请先选择一页幻灯片");
    return;
  }
  if (presentationSlideBackgrounds.value[slide.id] === background) return;
  rememberPresentationSnapshot();
  presentationSlideBackgrounds.value = {
    ...presentationSlideBackgrounds.value,
    [slide.id]: background
  };
  ElMessage.success("已应用当前页背景");
}

function applyCustomBackgroundColor(event: Event) {
  const value = (event.target as HTMLInputElement).value;
  if (!value) return;
  presentationBackgroundMode.value = "solid";
  applySlideBackground(value);
}

function resetCurrentSlideBackground() {
  const slide = store.currentSlide;
  if (!slide) return;
  if (!presentationSlideBackgrounds.value[slide.id] && !presentationBackgroundImageUrl.value) return;
  rememberPresentationSnapshot();
  const next = { ...presentationSlideBackgrounds.value };
  delete next[slide.id];
  presentationSlideBackgrounds.value = next;
  presentationBackgroundImageUrl.value = "";
  ElMessage.success("已重置当前页背景");
}

function applyNextBackgroundPreset() {
  const source =
    presentationBackgroundMode.value === "radial" ? radialBackgroundPresets :
    presentationBackgroundMode.value === "solid" ? solidBackgroundPresets :
    linearBackgroundPresets;
  if (!source.length) return;
  const current = currentSlideBackground.value;
  const index = source.findIndex(item => item.value === current);
  const next = source[(index + 1 + source.length) % source.length];
  applySlideBackground(next.value);
}

function applySlideBackgroundImage() {
  const rawUrl = presentationBackgroundImageUrl.value.trim();
  if (!rawUrl) return;
  try {
    new URL(rawUrl);
  } catch {
    ElMessage.warning("请输入完整图片链接，例如 https://example.com/image.png");
    return;
  }
  applySlideBackground(`linear-gradient(135deg, rgba(0,0,0,.18), rgba(0,0,0,.38)), url("${rawUrl}") center / cover no-repeat`);
}

function applyDeckWidth(width: PresentationDeckWidth) {
  if (presentationDeckWidth.value === width) return;
  rememberPresentationSnapshot();
  presentationDeckWidth.value = width;
  const label = presentationDeckWidthOptions.find(item => item.value === width)?.label || "当前";
  ElMessage.success(`已切换为${label}页面宽度`);
}

function applyGlobalAlignment(align: PresentationElementAlign) {
  if (presentationGlobalAlignment.value === align) return;
  rememberPresentationSnapshot();
  presentationGlobalAlignment.value = align;
  const label = presentationElementAlignments.find(item => item.value === align)?.label || "当前";
  ElMessage.success(`内容对齐已切换为${label}`);
}

function applyTypographyScale(scale: PresentationTypographyScale) {
  if (presentationTypographyScale.value === scale) return;
  rememberPresentationSnapshot();
  presentationTypographyScale.value = scale;
  const label = presentationTypographyScaleOptions.find(item => item.value === scale)?.label || "当前";
  ElMessage.success(`字号比例已切换为${label}`);
}

function openPresentationThemeCreator(isCustomizing: boolean) {
  presentationThemeCreatorIsCustomizing.value = isCustomizing;
  presentationThemeCreatorOpen.value = true;
}

function applyPresentationThemeFromCreator(theme: PptTheme) {
  if (store.theme === theme) return;
  rememberPresentationSnapshot();
  store.theme = theme;
}

function handlePresentationThemeCreatorSaved() {
  // 弹窗内部已展示保存结果，这里只保留后续接真实主题服务的挂点。
}

function handlePresentationThemeImport() {
  ElMessage.info("导入 PPTX 主题接口已预留，当前先使用主题创建流程");
  openPresentationThemeCreator(false);
}

function applyCurrentBackgroundToAllSlides() {
  if (!currentSlideBackground.value) {
    ElMessage.warning("请先为当前页设置背景");
    return;
  }
  const nextBackgrounds: Record<string, string> = {};
  store.slides.forEach((slide) => {
    nextBackgrounds[slide.id] = currentSlideBackground.value;
  });
  presentationSlideBackgrounds.value = nextBackgrounds;
  ElMessage.success("已应用当前背景到全部页面");
}

function selectPresentationIcon(iconName: string) {
  const slide = store.currentSlide;
  if (!slide) {
    ElMessage.warning("请先选择一页幻灯片");
    return;
  }
  if (!presentationIconOptions.some(item => item.name === iconName)) return;
  presentationSlideIcons.value = {
    ...presentationSlideIcons.value,
    [slide.id]: iconName
  };
  ElMessage.success("已更新当前页图标");
}

function removeCurrentSlideIcon() {
  const slide = store.currentSlide;
  if (!slide) return;
  const nextIcons = { ...presentationSlideIcons.value };
  delete nextIcons[slide.id];
  presentationSlideIcons.value = nextIcons;
  ElMessage.success("已移除当前页图标");
}

function buildPresentationAgentSuggestion(prompt: string, slide: PptSlide) {
  const cleanPrompt = prompt.replace(/\s+/g, " ").trim();
  const baseTitle = slide.title.replace(/\s+/g, " ").trim();
  if (/标题|文案|优化/.test(cleanPrompt)) {
    return `建议：把「${baseTitle}」改成更清晰的结论式表达，并在正文里先讲背景、再讲价值、最后讲下一步。`;
  }
  if (/路演|融资|项目/.test(cleanPrompt)) {
    return `建议：当前页可强化为路演页，标题突出商业结果，正文补充市场机会，三条要点分别对应痛点、方案和增长空间。`;
  }
  if (/要点|补充|说服/.test(cleanPrompt)) {
    return `建议：新增三条要点：核心问题是什么、为什么现在解决、采用该方案后能带来什么可量化变化。`;
  }
  if (/视觉|布局|版式/.test(cleanPrompt)) {
    return `建议：使用左文右图布局，左侧保留一句结论和三条要点，右侧放产品截图、流程图或关键数字卡片。`;
  }
  return `建议：围绕「${baseTitle}」压缩正文长度，保留一个核心判断、三个支撑要点和一个明确行动。`;
}

async function runPresentationAgentPrompt(prompt?: string) {
  const slide = store.currentSlide;
  const userPrompt = (prompt || presentationAgentPrompt.value).trim();
  if (!slide || !userPrompt || presentationAgentBusy.value) return;
  presentationAgentMessages.value = [
    ...presentationAgentMessages.value,
    { role: "user", content: userPrompt }
  ];
  presentationAgentPrompt.value = "";
  presentationAgentBusy.value = true;
  await new Promise(resolve => window.setTimeout(resolve, 520));
  presentationAgentMessages.value = [
    ...presentationAgentMessages.value,
    { role: "assistant", content: buildPresentationAgentSuggestion(userPrompt, slide) }
  ];
  presentationAgentBusy.value = false;
}

function applyLatestAgentSuggestion() {
  const slide = store.currentSlide;
  if (!slide || !latestAgentSuggestion.value) return;
  rememberPresentationSnapshot();
  const suggestion = latestAgentSuggestion.value.replace(/^建议：/, "");
  const nextBullets = [...slide.bulletPoints];
  const newPoint = suggestion.length > 34 ? `${suggestion.slice(0, 34)}...` : suggestion;
  if (!nextBullets.includes(newPoint)) {
    nextBullets.unshift(newPoint);
  }
  store.updateSlide(store.currentSlideIndex, {
    content: `${slide.content.replace(/\s+$/, "")}\n\nAI建议：${suggestion}`,
    bulletPoints: nextBullets.slice(0, 5)
  });
  ElMessage.success("已把 AI 建议应用到当前页");
}

function replaceLastOccurrence(value: string, previous: string, next: string) {
  const index = value.lastIndexOf(previous);
  if (index < 0) return "";
  return `${value.slice(0, index)}${next}${value.slice(index + previous.length)}`;
}

function endsWithBulletSequence(points: string[], sequence: string[]) {
  if (!sequence.length || points.length < sequence.length) return false;
  const offset = points.length - sequence.length;
  return sequence.every((point, index) => points[offset + index] === point);
}

function isPresentationPaletteBlockSelected(panel: PresentationRightPanel, block: InsertablePresentationBlock) {
  const selection = selectedPresentationPaletteBlock.value;
  const slide = store.currentSlide;
  return Boolean(selection && slide && selection.slideId === slide.id && selection.panel === panel && selection.title === block.title);
}

function isPresentationDiagramCategoryCollapsed(category: string) {
  return collapsedPresentationDiagramCategories.value.includes(category);
}

function togglePresentationDiagramCategory(category: string) {
  collapsedPresentationDiagramCategories.value = isPresentationDiagramCategoryCollapsed(category)
    ? collapsedPresentationDiagramCategories.value.filter(item => item !== category)
    : [...collapsedPresentationDiagramCategories.value, category];
}

function presentationPaletteCardTabIndex(panel: PresentationRightPanel, block: InsertablePresentationBlock, index: number) {
  if (isPresentationPaletteBlockSelected(panel, block)) return 0;
  const selection = selectedPresentationPaletteBlock.value;
  if (!selection || selection.panel !== panel) return index === 0 ? 0 : -1;
  return -1;
}

function presentationDiagramCardTabIndex(block: InsertablePresentationBlock, fallbackIndex: number) {
  const visibleIndex = visiblePresentationDiagramBlocks.value.findIndex(item => (
    item.title === block.title &&
    item.category === block.category
  ));
  return presentationPaletteCardTabIndex("diagrams", block, visibleIndex >= 0 ? visibleIndex : fallbackIndex);
}

function clearPresentationPanelLoadTimer() {
  if (!presentationPanelLoadTimer) return;
  window.clearTimeout(presentationPanelLoadTimer);
  presentationPanelLoadTimer = null;
}

async function focusPresentationPanelAfterLoad() {
  await nextTick();
  window.requestAnimationFrame(() => {
    if (presentationRightPanel.value === "iconPicker") {
      presentationIconSearchInputRef.value?.focus();
      return;
    }
    focusPresentationPanelArrowTarget();
  });
}

function schedulePresentationPanelContentLoad(panel: PresentationRightPanel, isOpen: boolean) {
  clearPresentationPanelLoadTimer();
  if (!isOpen) {
    presentationLoadedRightPanel.value = null;
    return;
  }

  if (presentationImmediateLoadPanels.has(panel)) {
    presentationLoadedRightPanel.value = panel;
    void focusPresentationPanelAfterLoad();
    return;
  }

  presentationLoadedRightPanel.value = null;
  presentationPanelLoadTimer = window.setTimeout(() => {
    presentationLoadedRightPanel.value = panel;
    presentationPanelLoadTimer = null;
    void focusPresentationPanelAfterLoad();
  }, 350);
}

function focusPresentationPanelArrowTarget() {
  const panel = presentationRightPanelContentRef.value;
  if (!panel || !presentationRightPanelOpen.value) return null;
  const activeTarget =
    panel.querySelector<HTMLElement>("[data-panel-arrow-target='true'][tabindex='0']") ||
    panel.querySelector<HTMLElement>("[data-panel-arrow-target='true']");
  activeTarget?.focus();
  return activeTarget || null;
}

function handlePresentationPanelExternalArrowKeydown(event: KeyboardEvent) {
  if (!presentationRightPanelOpen.value || presentationViewMode.value === "present") return;
  if (!presentationPanelArrowKeys.has(event.key)) return;
  const panel = presentationRightPanelContentRef.value;
  if (!panel) return;
  const target = event.target;
  if (target instanceof Node && panel.contains(target)) return;
  const targetElement = target instanceof HTMLElement ? target : null;
  const isTypingTarget = targetElement?.tagName === "INPUT" || targetElement?.tagName === "TEXTAREA" || targetElement?.isContentEditable;
  if (isTypingTarget) return;
  if (targetElement?.closest(".ppt-slide-sidebar-shell")) return;
  const panelTarget = focusPresentationPanelArrowTarget();
  if (!panelTarget) return;
  event.preventDefault();
  event.stopPropagation();
  event.stopImmediatePropagation();
  panelTarget.dispatchEvent(new KeyboardEvent("keydown", {
    key: event.key,
    code: event.code,
    bubbles: true,
    cancelable: true,
    altKey: event.altKey,
    ctrlKey: event.ctrlKey,
    metaKey: event.metaKey,
    shiftKey: event.shiftKey
  }));
}

function handlePresentationPaletteCardKeydown(event: KeyboardEvent, index: number) {
  if (event.key === "Enter" || event.key === " ") {
    event.preventDefault();
    event.stopPropagation();
    (event.currentTarget as HTMLButtonElement).click();
    return;
  }

  const columns = presentationRightPanel.value === "content" ? 3 : 2;
  const movement: Record<string, number> = {
    ArrowLeft: -1,
    ArrowRight: 1,
    ArrowUp: -columns,
    ArrowDown: columns
  };
  const section = (event.currentTarget as HTMLElement).closest(".ppt-panel-section");
  const cards = Array.from(section?.querySelectorAll<HTMLButtonElement>(".ppt-insert-card") || []);
  const currentIndex = cards.indexOf(event.currentTarget as HTMLButtonElement);
  if (!cards.length) return;
  if (currentIndex < 0) return;
  const nextIndex = event.key === "Home"
    ? 0
    : event.key === "End"
      ? cards.length - 1
      : Math.min(Math.max(currentIndex + (movement[event.key] || 0), 0), cards.length - 1);
  if (!presentationPanelArrowKeys.has(event.key)) return;
  if (nextIndex === currentIndex || !cards[nextIndex]) return;
  event.preventDefault();
  event.stopPropagation();
  cards[nextIndex].focus();
}

function appendCurrentSlideBlock(panel: PresentationRightPanel, block: InsertablePresentationBlock, successMessage: string) {
  const slide = store.currentSlide;
  if (!slide) {
    ElMessage.warning("请先选择一页幻灯片");
    return;
  }
  rememberPresentationSnapshot();
  const previousSelection = selectedPresentationPaletteBlock.value;
  const canReplace =
    previousSelection &&
    previousSelection.slideId === slide.id &&
    previousSelection.panel === panel &&
    replaceLastOccurrence(slide.content, previousSelection.content, block.content);
  const replacedContent = canReplace
    ? replaceLastOccurrence(slide.content, previousSelection.content, block.content)
    : "";
  const nextContent = replacedContent || (slide.content ? `${slide.content}\n\n${block.content}` : block.content);
  const nextBulletPoints = canReplace && endsWithBulletSequence(slide.bulletPoints, previousSelection.bulletPoints)
    ? [...slide.bulletPoints.slice(0, -previousSelection.bulletPoints.length), ...block.bulletPoints]
    : [...slide.bulletPoints, ...block.bulletPoints];
  store.updateSlide(store.currentSlideIndex, {
    content: nextContent,
    bulletPoints: nextBulletPoints
  });
  selectedPresentationPaletteBlock.value = {
    slideId: slide.id,
    panel,
    title: block.title,
    content: block.content,
    bulletPoints: [...block.bulletPoints]
  };
  selectedPresentationElement.value = { slideIndex: store.currentSlideIndex, kind: block.bulletPoints.length ? "bullets" : "content" };
  ElMessage.success(replacedContent ? `已替换为「${block.title}」` : successMessage);
}

function insertTextBlock(block: InsertablePresentationBlock) {
  appendCurrentSlideBlock("content", block, `已添加「${block.title}」文本块`);
}

function insertElementBlock(block: InsertablePresentationBlock) {
  appendCurrentSlideBlock("elements", block, `已添加「${block.title}」元素`);
}

function insertChartBlock(block: InsertablePresentationBlock) {
  appendCurrentSlideBlock("charts", block, `已添加「${block.title}」图表占位`);
}

function insertDiagramBlock(block: InsertablePresentationBlock) {
  appendCurrentSlideBlock("diagrams", block, `已添加「${block.title}」图示占位`);
}

function insertEmbedTemplateBlock(block: InsertablePresentationBlock) {
  appendCurrentSlideBlock("embed", block, `已添加「${block.title}」嵌入占位`);
}

function insertEmbedBlock() {
  const rawUrl = presentationEmbedUrl.value.trim();
  if (!rawUrl) {
    ElMessage.warning("请先粘贴需要嵌入的链接");
    return;
  }
  try {
    new URL(rawUrl);
  } catch {
    ElMessage.warning("请输入完整链接，例如 https://example.com");
    return;
  }
  appendCurrentSlideBlock(
    "embed",
    {
      title: "链接占位",
      description: rawUrl,
      content: `新增嵌入链接占位：${rawUrl}`,
      bulletPoints: ["嵌入来源", "内容摘要", "展示入口"],
      category: "web",
      icon: "web",
      iconLabel: "URL",
      keywords: ["embed", "url", "链接"]
    },
    "已添加链接占位"
  );
  presentationEmbedUrl.value = "";
  showEmbedUrlCard.value = false;
}

async function openRecordingPreview() {
  if (presentModeBusy.value || isPresentationActionBusy.value || !store.slides.length) return;
  recordingWantsToRecord.value = true;
  if (presentationViewMode.value !== "present") {
    await enterPresentationMode();
  }
  presentModeHeaderVisible.value = true;
  ElMessage.success("已进入演示录制准备");
}

async function openPresentationOnly() {
  if (presentModeBusy.value || isPresentationActionBusy.value || !store.slides.length) return;
  recordingWantsToRecord.value = false;
  if (presentationViewMode.value !== "present") {
    await enterPresentationMode();
  }
  presentModeHeaderVisible.value = true;
  ElMessage.info("已进入演示预览");
}

function handlePreviewHistory(item: PptHistoryItem) {
  store.loadHistoryItem(item);
  if (item.status === "success") {
    void openPresentationWorkspace(item.taskId);
  } else {
    void openGenerationWorkspace(item.taskId);
  }
  ElMessage.info(item.status === "success" ? "已加载预览" : item.status === "draft" ? "已打开未完成草稿" : "该任务仍在生成中");
}

function handleEditHistory(item: PptHistoryItem) {
  store.loadHistoryItem(item);
  showConfig.value = false;
  if (item.status === "success") {
    void openPresentationWorkspace(item.taskId);
  } else {
    void openGenerationWorkspace(item.taskId);
  }
}

async function handleDownloadPpt(item: PptHistoryItem) {
  if (!item.pptUrl) {
    ElMessage.warning("下载PPT接口已预留，当前 mock 记录暂无文件地址");
    return;
  }
  window.open(item.pptUrl, "_blank", "noopener");
}

async function handleDownloadPdf(item: PptHistoryItem) {
  if (!item.pdfUrl) {
    ElMessage.warning("下载PDF接口已预留，当前 mock 记录暂无文件地址");
    return;
  }
  window.open(item.pdfUrl, "_blank", "noopener");
}

function handleRegenerateHistory(item: PptHistoryItem) {
  store.loadHistoryItem(item);
  if (item.status === "draft") {
    void openGenerationWorkspace(item.taskId);
    return;
  }
  void openPresentationWorkspace(item.taskId);
  if (store.outline) {
    void handleWorkspacePrimaryAction();
    return;
  }
  void store.generateOutlineFlow();
}

function handlePresentationBackToOutline() {
  showPresentationMenu.value = false;
  void openGenerationWorkspace(store.taskId || activePresentationId.value || activeGenerationId.value);
}

function togglePresentationMenu() {
  const nextOpen = !showPresentationMenu.value;
  closePresentationFloatingControls();
  showPresentationMenu.value = nextOpen;
}

async function handleMenuCreateBlank() {
  showPresentationMenu.value = false;
  resetComposer();
  await store.startBlankPpt();
  await openPresentationWorkspace(`blank_${Date.now()}`);
}

async function focusPresentationTitle() {
  showPresentationMenu.value = false;
  await nextTick();
  presentationTitleInputRef.value?.focus();
  presentationTitleInputRef.value?.select();
}

function duplicateCurrentPresentation() {
  showPresentationMenu.value = false;
  if (!store.slides.length) {
    ElMessage.warning("当前还没有可复制的幻灯片");
    return;
  }
  const title = `${workspaceTitle.value} 副本`;
  store.prompt = title;
  if (store.outline) store.outline.title = title;
  store.taskId = `draft_copy_${Date.now()}`;
  store.slides = cloneSlides(store.slides).map((slide, index) => ({
    ...slide,
    id: `${slide.id}_copy_${Date.now()}_${index}`,
    page: index + 1
  }));
  activePresentationId.value = store.taskId;
  void openPresentationWorkspace(store.taskId, { replace: false });
  ElMessage.success("已复制演示文稿");
}

function undoPresentationEdit() {
  if (!presentationUndoStack.value.length) return;
  const previous = presentationUndoStack.value[presentationUndoStack.value.length - 1];
  presentationUndoStack.value = presentationUndoStack.value.slice(0, -1);
  presentationRedoStack.value = [...presentationRedoStack.value, createPresentationEditorSnapshot()];
  restorePresentationEditorSnapshot(previous);
  markPresentationSaving();
}

function redoPresentationEdit() {
  if (!presentationRedoStack.value.length) return;
  const next = presentationRedoStack.value[presentationRedoStack.value.length - 1];
  presentationRedoStack.value = presentationRedoStack.value.slice(0, -1);
  presentationUndoStack.value = [...presentationUndoStack.value, createPresentationEditorSnapshot()];
  restorePresentationEditorSnapshot(next);
  markPresentationSaving();
}

function handlePresentationMenuUndo() {
  showPresentationMenu.value = false;
  undoPresentationEdit();
}

function handlePresentationMenuRedo() {
  showPresentationMenu.value = false;
  redoPresentationEdit();
}

function openThemePanelFromMenu() {
  showPresentationMenu.value = false;
  selectPresentationRightPanel("theme");
}

function openGlobalSettingsFromMenu() {
  showPresentationMenu.value = false;
  presentationGlobalSettingsTab.value = "cards";
  selectPresentationRightPanel("globalSettings");
}

function openShareDialogFromMenu() {
  showPresentationMenu.value = false;
  openShareDialog();
}

function openExportDialog() {
  if (exportBusy.value) return;
  if (!store.slides.length) {
    ElMessage.warning("当前没有可导出的幻灯片");
    return;
  }
  closePresentationFloatingControls();
  presentationExportPhase.value = "idle";
  showExportDialog.value = true;
  void nextTick(() => {
    exportPrimaryButtonRef.value?.focus();
  });
}

function closeExportDialog() {
  if (exportBusy.value) return;
  showExportDialog.value = false;
}

function openShareDialog() {
  if (!activePresentationId.value && !store.taskId) {
    ElMessage.warning("请先生成或打开一个演示文稿");
    return;
  }
  closePresentationFloatingControls();
  showShareDialog.value = true;
  void nextTick(() => {
    shareCopyButtonRef.value?.focus();
  });
}

function closeShareDialog() {
  showShareDialog.value = false;
}

async function copyTextToClipboard(value: string) {
  if (!value) return;
  if (navigator?.clipboard?.writeText) {
    await navigator.clipboard.writeText(value);
    return;
  }
  const input = document.createElement("textarea");
  input.value = value;
  input.setAttribute("readonly", "true");
  input.style.position = "fixed";
  input.style.opacity = "0";
  document.body.appendChild(input);
  input.select();
  document.execCommand("copy");
  document.body.removeChild(input);
}

async function copyPresentationShareUrl() {
  try {
    await copyTextToClipboard(presentationShareUrl.value);
    ElMessage.success("演示文稿链接已复制");
  } catch {
    ElMessage.warning("复制失败，请手动复制链接");
  }
}

function toggleZoomMenu() {
  const nextOpen = !showZoomMenu.value;
  closePresentationFloatingControls();
  showZoomMenu.value = nextOpen;
}

function isPresentationZoomValueActive(value: number) {
  return Math.abs(presentationZoom.value - value) < 0.005;
}

function setPresentationZoom(value: number) {
  presentationZoom.value = Number(Math.min(1.8, Math.max(0.5, value)).toFixed(2));
  showZoomMenu.value = false;
}

function fitPresentationZoom() {
  setPresentationZoom(fitPresentationZoomValue);
}

function togglePresentationHelpMenu() {
  const nextOpen = !showPresentationHelpMenu.value;
  closePresentationFloatingControls();
  showPresentationHelpMenu.value = nextOpen;
}

function openKeyboardShortcuts() {
  closePresentationFloatingControls({ keepKeyboardDialog: true });
  showKeyboardShortcutsDialog.value = true;
  void nextTick(() => {
    shortcutsCloseButtonRef.value?.focus();
  });
}

function closeKeyboardShortcuts() {
  showKeyboardShortcutsDialog.value = false;
}

function openReservedHelpCenter() {
  showPresentationHelpMenu.value = false;
  ElMessage.info("帮助中心入口已预留");
}

function toggleGenerationHelpMenu() {
  showGenerationHelpMenu.value = !showGenerationHelpMenu.value;
}

function openGenerationKeyboardShortcuts() {
  showGenerationHelpMenu.value = false;
  showGenerationKeyboardShortcutsDialog.value = true;
}

function closeGenerationKeyboardShortcuts() {
  showGenerationKeyboardShortcutsDialog.value = false;
}

function openGenerationHelpCenter() {
  showGenerationHelpMenu.value = false;
  ElMessage.info("PPT 生成帮助中心入口已预留");
}

async function togglePresentationMode() {
  if (presentModeBusy.value || isPresentationActionBusy.value) return;
  if (presentationViewMode.value === "present") {
    await exitPresentationMode();
    return;
  }
  await enterPresentationMode();
}

async function enterPresentationMode() {
  if (presentModeBusy.value || isPresentationActionBusy.value || !store.slides.length) return;
  closePresentationFloatingControls();
  slideMoreMenuIndex.value = null;
  slideMagicMenuIndex.value = null;
  updatePresentationViewportSize();
  presentModeBusy.value = true;
  await new Promise(resolve => window.setTimeout(resolve, 90));
  presentationViewMode.value = "present";
  updatePresentationViewportSize();
  presentModeHeaderVisible.value = true;
  presentModeWheelLastAt = 0;
  if (typeof document !== "undefined") {
    document.body.classList.add("ppt-presenting-body");
  }
  store.selectSlide(Math.min(Math.max(store.currentSlideIndex, 0), store.slides.length - 1));
  presentModeBusy.value = false;
}

async function exitPresentationMode() {
  if (presentModeBusy.value) return;
  presentModeBusy.value = true;
  await new Promise(resolve => window.setTimeout(resolve, 70));
  presentationViewMode.value = "edit";
  presentModeHeaderVisible.value = true;
  await cleanupPresentationModeSideEffects();
  presentModeBusy.value = false;
  await scrollPresentationSlideIntoView(store.currentSlideIndex);
}

async function cleanupPresentationModeSideEffects() {
  recordingWantsToRecord.value = false;
  presentPhoneGesture = null;
  if (typeof document === "undefined") return;
  document.body.classList.remove("ppt-presenting-body");
  if (document.fullscreenElement) {
    await document.exitFullscreen().catch(() => undefined);
  }
  if (typeof screen !== "undefined" && "orientation" in screen) {
    const orientationController = screen.orientation as ScreenOrientation & { unlock?: () => void };
    if (typeof orientationController.unlock === "function") orientationController.unlock();
  }
}

function nextPresentationSlide() {
  if (!store.slides.length || store.currentSlideIndex >= store.slides.length - 1) return;
  store.selectSlide(store.currentSlideIndex + 1);
}

function previousPresentationSlide() {
  if (!store.slides.length || store.currentSlideIndex <= 0) return;
  store.selectSlide(store.currentSlideIndex - 1);
}

function selectPresentationProgressSlide(index: number) {
  store.selectSlide(index);
}

function handleGenerationWorkspaceKeydown(event: KeyboardEvent) {
  if (!isGenerationWorkspace.value || isPresentationWorkspace.value) return;
  const target = event.target as HTMLElement | null;
  const isTypingTarget = target?.tagName === "INPUT" || target?.tagName === "TEXTAREA" || target?.isContentEditable;
  if (isTypingTarget) return;
  if ((event.ctrlKey || event.metaKey) && event.key === "Enter") {
    event.preventDefault();
    handleWorkspacePrimaryAction();
    return;
  }
  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "s") {
    event.preventDefault();
    void store.saveOutline();
    return;
  }
  if (event.key === "?") {
    event.preventDefault();
    showGenerationHelpMenu.value = true;
    return;
  }
  if (event.key === "Escape") {
    if (showGenerationKeyboardShortcutsDialog.value) {
      closeGenerationKeyboardShortcuts();
      return;
    }
    if (showGenerationHelpMenu.value) {
      showGenerationHelpMenu.value = false;
      return;
    }
    event.preventDefault();
    void handlePptHomeClick();
  }
}

function handlePresentationWorkspaceKeydown(event: KeyboardEvent) {
  if (!isPresentationWorkspace.value || presentationViewMode.value === "present") return;
  if (event.key !== "Escape") return;
  const hasOpenFloatingControl = showPresentationMenu.value ||
    showExportDialog.value ||
    showShareDialog.value ||
    showZoomMenu.value ||
    showPresentationHelpMenu.value ||
    showKeyboardShortcutsDialog.value ||
    showElementTypeMenu.value ||
    showElementAiEditor.value ||
    slideMoreMenuIndex.value !== null ||
    slidePaletteMenuIndex.value !== null ||
    slideMagicMenuIndex.value !== null;
  if (!hasOpenFloatingControl) return;
  event.preventDefault();
  closePresentationFloatingControls();
}

function handlePresentModeKeydown(event: KeyboardEvent) {
  if (presentationViewMode.value !== "present") return;
  const target = event.target as HTMLElement | null;
  const isTypingTarget = target?.tagName === "INPUT" || target?.tagName === "TEXTAREA" || target?.isContentEditable;
  if (isTypingTarget) return;
  if (event.key === "Escape") {
    event.preventDefault();
    void exitPresentationMode();
    return;
  }
  if (event.key === "Home") {
    event.preventDefault();
    store.selectSlide(0);
    return;
  }
  if (event.key === "End") {
    event.preventDefault();
    store.selectSlide(Math.max(0, store.slides.length - 1));
    return;
  }
  if (["ArrowRight", "ArrowDown", "PageDown", " "].includes(event.key) || event.code === "Space") {
    if (event.shiftKey && (event.key === " " || event.code === "Space")) {
      event.preventDefault();
      previousPresentationSlide();
      return;
    }
    event.preventDefault();
    nextPresentationSlide();
    return;
  }
  if (["ArrowLeft", "ArrowUp", "PageUp"].includes(event.key)) {
    event.preventDefault();
    previousPresentationSlide();
  }
}

function handlePresentModeWheel(event: WheelEvent) {
  if (presentationViewMode.value !== "present") return;
  const now = typeof performance !== "undefined" ? performance.now() : Date.now();
  if (now - presentModeWheelLastAt < 420) {
    event.preventDefault();
    return;
  }
  if (Math.abs(event.deltaY) < 42) return;
  event.preventDefault();
  event.stopPropagation();
  if (event.deltaY > 0) nextPresentationSlide();
  if (event.deltaY < 0) previousPresentationSlide();
  presentModeWheelLastAt = now;
}

function handlePresentModeMouseMove(event: MouseEvent) {
  if (presentationViewMode.value !== "present") return;
  presentModeHeaderVisible.value = event.clientY < 96;
}

function showPresentModeHeaderFromTap() {
  if (presentationViewMode.value !== "present") return;
  presentModeHeaderVisible.value = true;
}

function handlePresentPhonePointerDown(event: PointerEvent) {
  if (presentationViewMode.value !== "present" || !event.isPrimary) return;
  presentModeHeaderVisible.value = false;
  const target = event.currentTarget as HTMLElement;
  target.setPointerCapture(event.pointerId);
  presentPhoneGesture = {
    pointerId: event.pointerId,
    startX: event.clientX,
    startY: event.clientY
  };
}

function handlePresentPhonePointerCancel(event: PointerEvent) {
  const target = event.currentTarget as HTMLElement;
  if (target.hasPointerCapture(event.pointerId)) {
    target.releasePointerCapture(event.pointerId);
  }
  presentPhoneGesture = null;
}

function handlePresentPhonePointerUp(event: PointerEvent) {
  const gesture = presentPhoneGesture;
  if (!gesture || gesture.pointerId !== event.pointerId) return;

  const target = event.currentTarget as HTMLElement;
  if (target.hasPointerCapture(event.pointerId)) {
    target.releasePointerCapture(event.pointerId);
  }

  const deltaX = event.clientX - gesture.startX;
  const deltaY = event.clientY - gesture.startY;
  presentPhoneGesture = null;

  if (Math.abs(deltaX) >= presentSwipeDistanceThreshold && Math.abs(deltaX) > Math.abs(deltaY)) {
    if (deltaX > 0) {
      nextPresentationSlide();
      return;
    }
    previousPresentationSlide();
    return;
  }

  if (Math.abs(deltaX) > presentTapDistanceThreshold || Math.abs(deltaY) > presentTapDistanceThreshold) {
    return;
  }

  const bounds = target.getBoundingClientRect();
  const tapX = event.clientX - bounds.left;
  if (tapX >= bounds.width / 2) {
    nextPresentationSlide();
    return;
  }
  previousPresentationSlide();
}

function updateCurrentSlideNotes(event: Event) {
  rememberPresentationSnapshot();
  store.updateSlide(store.currentSlideIndex, {
    speakerNotes: (event.target as HTMLTextAreaElement).value
  });
}

function generatedNotesForSlide(slide: PptSlide) {
  const bullets = slide.bulletPoints.length
    ? `\n\n可以重点展开：${slide.bulletPoints.join("、")}。`
    : "";
  return `开场先点出「${slide.title}」这一页的核心信息。\n\n${slide.content}${bullets}\n\n收尾时用一句话承接到下一页，提醒听众关注后续行动。`;
}

function generateCurrentSlideNotes() {
  const slide = store.currentSlide;
  if (!slide) return;
  rememberPresentationSnapshot();
  store.updateSlide(store.currentSlideIndex, { speakerNotes: generatedNotesForSlide(slide) });
  ElMessage.success("已生成当前页讲稿");
}

function polishCurrentSlideNotes() {
  const slide = store.currentSlide;
  if (!slide) return;
  const source = currentSpeakerNotes.value.trim() || generatedNotesForSlide(slide);
  rememberPresentationSnapshot();
  store.updateSlide(store.currentSlideIndex, {
    speakerNotes: `${source.replace(/。?$/, "。")}\n\n表达建议：语速放稳，先讲结论，再补充一个具体例子。`
  });
  ElMessage.success("已优化讲稿表达");
}

async function copyCurrentSlideNotes() {
  const notes = currentSpeakerNotes.value.trim();
  if (!notes) {
    ElMessage.warning("当前页暂无讲稿可复制");
    return;
  }
  try {
    await navigator.clipboard?.writeText(notes);
    ElMessage.success("讲稿已复制");
  } catch {
    ElMessage.warning("浏览器暂不允许复制，请手动选中文本");
  }
}

function clearCurrentSlideNotes() {
  if (!store.currentSlide) return;
  rememberPresentationSnapshot();
  store.updateSlide(store.currentSlideIndex, { speakerNotes: "" });
  ElMessage.success("已清空当前页讲稿");
}

function exportStepState(step: PresentationExportPhase) {
  if (presentationExportPhase.value === "complete") return "done";
  const order: PresentationExportPhase[] = ["scanning", "generating", "downloading"];
  const currentIndex = order.indexOf(presentationExportPhase.value);
  const stepIndex = order.indexOf(step);
  if (currentIndex < 0 || stepIndex < 0) return "pending";
  if (currentIndex === stepIndex) return "active";
  return currentIndex > stepIndex ? "done" : "pending";
}

function sanitizeExportFileName(value: string) {
  return (value || "presentation")
    .replace(/[\\/:*?"<>|]+/g, "-")
    .replace(/\s+/g, "-")
    .replace(/-+/g, "-")
    .replace(/^-|-$/g, "")
    .slice(0, 80) || "presentation";
}

function revokePresentationExportDownloadUrl() {
  if (!presentationExportDownloadUrl.value) return;
  URL.revokeObjectURL(presentationExportDownloadUrl.value);
  presentationExportDownloadUrl.value = "";
  presentationExportDownloadName.value = "";
}

async function handleExportCurrent(kind: "pptx" | "pdf") {
  exportBusy.value = true;
  presentationExportPhase.value = "scanning";
  if (!store.slides.length) {
    ElMessage.warning("当前没有可导出的幻灯片");
    exportBusy.value = false;
    presentationExportPhase.value = "idle";
    return;
  }
  try {
    await new Promise(resolve => window.setTimeout(resolve, 260));
    presentationExportPhase.value = "generating";
    const result = await store.exportCurrentPpt(kind);
    if (!result?.url) {
      throw new Error("PPTX 导出接口没有返回文件内容");
    }
    presentationExportPhase.value = "downloading";
    revokePresentationExportDownloadUrl();
    const downloadUrl = result.url;
    const exportResult = result as unknown as { filename?: unknown };
    const resultFileName = typeof exportResult.filename === "string" ? exportResult.filename : "";
    const fileName = resultFileName || `${sanitizeExportFileName(workspaceTitle.value)}.${kind === "pptx" ? "pptx" : "pdf"}`;
    presentationExportDownloadUrl.value = downloadUrl;
    presentationExportDownloadName.value = fileName;
    const link = document.createElement("a");
    link.href = downloadUrl;
    link.download = fileName;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    presentationExportPhase.value = "complete";
    ElMessage.success(`${kind === "pptx" ? "PPTX" : "PDF"} 已开始下载`);
    window.setTimeout(() => {
      if (presentationExportDownloadUrl.value === downloadUrl) {
        revokePresentationExportDownloadUrl();
      } else {
        URL.revokeObjectURL(downloadUrl);
      }
    }, 60_000);
    showExportDialog.value = false;
    return;
  } catch (error) {
    presentationExportPhase.value = "idle";
    ElMessage.warning(error instanceof Error ? error.message : "导出失败，请稍后重试");
  } finally {
    exportBusy.value = false;
  }
}

function toggleSlideMoreMenu(index: number) {
  slideMoreMenuIndex.value = slideMoreMenuIndex.value === index ? null : index;
  slidePaletteMenuIndex.value = null;
  slideMagicMenuIndex.value = null;
  selectedPresentationElement.value = null;
  showElementTypeMenu.value = false;
  showElementAiEditor.value = false;
  store.selectSlide(index);
}

function toggleSlidePaletteMenu(index: number) {
  slidePaletteMenuIndex.value = slidePaletteMenuIndex.value === index ? null : index;
  slideMoreMenuIndex.value = null;
  slideMagicMenuIndex.value = null;
  selectedPresentationElement.value = null;
  showElementTypeMenu.value = false;
  showElementAiEditor.value = false;
  store.selectSlide(index);
}

function toggleSlideMagicMenu(index: number) {
  if (slideMagicBusy.value) return;
  slideMagicMenuIndex.value = slideMagicMenuIndex.value === index ? null : index;
  slideMoreMenuIndex.value = null;
  slidePaletteMenuIndex.value = null;
  selectedPresentationElement.value = null;
  showElementTypeMenu.value = false;
  showElementAiEditor.value = false;
  slideMagicPrompt.value = "";
  slideMagicError.value = "";
  store.selectSlide(index);
  if (slideMagicMenuIndex.value === index) {
    nextTick(() => slideMagicPromptInputRef.value?.focus());
  }
}

function slideBackgroundFor(slide: PptSlide | null | undefined) {
  return slide ? presentationSlideBackgrounds.value[slide.id] || "" : "";
}

function applySlidePaletteLayout(index: number, layout: PptSlideLayout) {
  const slide = store.slides[index];
  if (!slide || slide.layout === layout) return;
  rememberPresentationSnapshot();
  store.selectSlide(index);
  store.updateSlide(index, { layout });
  ElMessage.success(`已切换为${presentationLayoutOptions.find(item => item.value === layout)?.label || "当前"}布局`);
}

function applySlidePaletteBackground(index: number, background: string) {
  const slide = store.slides[index];
  if (!slide) return;
  if (presentationSlideBackgrounds.value[slide.id] === background) return;
  rememberPresentationSnapshot();
  store.selectSlide(index);
  presentationSlideBackgrounds.value = {
    ...presentationSlideBackgrounds.value,
    [slide.id]: background
  };
  ElMessage.success("已应用当前页背景");
}

function resetSlidePaletteBackground(index: number) {
  const slide = store.slides[index];
  if (!slide) return;
  if (!presentationSlideBackgrounds.value[slide.id]) return;
  rememberPresentationSnapshot();
  const next = { ...presentationSlideBackgrounds.value };
  delete next[slide.id];
  presentationSlideBackgrounds.value = next;
  store.selectSlide(index);
  ElMessage.success("已重置当前页背景");
}

function applySlidePaletteAlignment(align: PresentationElementAlign) {
  applyGlobalAlignment(align);
}

function applySlidePaletteWidth(width: PresentationDeckWidth) {
  applyDeckWidth(width);
}

function openSlideImagePanelFromPalette(index: number) {
  store.selectSlide(index);
  slidePaletteMenuIndex.value = null;
  selectPresentationRightPanel("images");
}

function duplicateSlideAt(index: number) {
  const slide = store.slides[index];
  if (!slide) return;
  rememberPresentationSnapshot();
  const copy: PptSlide = {
    ...slide,
    id: `${slide.id}_copy_${Date.now()}`,
    title: `${slide.title} 副本`,
    bulletPoints: [...slide.bulletPoints]
  };
  store.slides.splice(index + 1, 0, copy);
  normalizePresentationSlides(store.slides);
  store.selectSlide(index + 1);
  slideMoreMenuIndex.value = null;
  slidePaletteMenuIndex.value = null;
  slideMagicMenuIndex.value = null;
  ElMessage.success("已复制当前页");
}

async function deleteSlideAt(index: number) {
  if (store.slides.length <= 1) {
    ElMessage.warning("至少保留一页幻灯片");
    return;
  }
  const slide = store.slides[index];
  slideMoreMenuIndex.value = null;
  try {
    await ElMessageBox.confirm(`确定删除「${slide?.title || `第 ${index + 1} 页`}」吗？删除后不可恢复。`, "删除幻灯片", {
      confirmButtonText: "删除",
      cancelButtonText: "取消",
      type: "warning"
    });
  } catch {
    return;
  }
  rememberPresentationSnapshot();
  store.slides.splice(index, 1);
  normalizePresentationSlides(store.slides);
  selectedPresentationElement.value = null;
  showElementTypeMenu.value = false;
  showElementAiEditor.value = false;
  slideMoreMenuIndex.value = null;
  slidePaletteMenuIndex.value = null;
  slideMagicMenuIndex.value = null;
  ElMessage.success("当前页已删除");
}

async function runSlideMagicAction(index: number, action: SlideMagicAction) {
  const slide = store.slides[index];
  if (!slide) return;
  if (slideMagicBusy.value) return;
  slideMagicBusy.value = true;
  slideMagicError.value = "";
  try {
    await new Promise(resolve => window.setTimeout(resolve, 420));
    rememberPresentationSnapshot();
    if (action === "layout") {
      const currentLayoutIndex = slideLayoutCycle.indexOf(slide.layout);
      const nextLayout = slideLayoutCycle[(currentLayoutIndex + 1 + slideLayoutCycle.length) % slideLayoutCycle.length];
      store.updateSlide(index, { layout: nextLayout });
      ElMessage.success("已切换本页布局");
    } else if (action === "writing") {
      store.updateSlide(index, {
        content: `${slide.content.replace(/。?$/, "。")}表达已收敛为更适合演示呈现的版本。`
      });
      ElMessage.success("已优化本页文案");
    } else if (action === "spelling") {
      store.updateSlide(index, {
        title: slide.title.replace(/\s+/g, " ").trim(),
        content: slide.content.replace(/\s+/g, " ").trim(),
        bulletPoints: slide.bulletPoints.map(point => point.replace(/\s+/g, " ").trim()).filter(Boolean)
      });
      ElMessage.success("已整理错别字与空格");
    } else if (action === "translate") {
      const isEnglish = store.language === "en";
      store.updateSlide(index, {
        title: isEnglish ? `English version: ${slide.title}` : `${slide.title}（中英文版）`,
        content: isEnglish ? `English summary: ${slide.content}` : `中文概述：${slide.content}`
      });
      ElMessage.success("已生成翻译占位");
    } else if (action === "simplify") {
      store.updateSlide(index, {
        content: `${slide.content.replace(/\s+/g, " ").slice(0, 72)}${slide.content.length > 72 ? "..." : ""}`,
        bulletPoints: slide.bulletPoints.slice(0, 3)
      });
      ElMessage.success("已精简本页内容");
    } else if (action === "visual") {
      store.updateSlide(index, {
        layout: "imageText",
        bulletPoints: slide.bulletPoints.length ? slide.bulletPoints.slice(0, 3) : ["视觉主张", "关键证据", "行动建议"]
      });
      selectPresentationRightPanel("images");
      ElMessage.success("已切换为图文增强结构");
    } else {
      selectPresentationRightPanel("images");
      ElMessage.info("已打开图片面板，可为本页生成或搜索配图");
    }
    slidePaletteMenuIndex.value = null;
    slideMagicMenuIndex.value = null;
  } catch {
    slideMagicError.value = "AI 编辑失败，请重试";
  } finally {
    slideMagicBusy.value = false;
  }
}

async function submitSlideMagicPrompt(index: number) {
  const prompt = slideMagicPrompt.value.trim();
  const slide = store.slides[index];
  if (!prompt || !slide) return;
  if (slideMagicBusy.value) return;
  slideMagicBusy.value = true;
  slideMagicError.value = "";
  try {
    await new Promise(resolve => window.setTimeout(resolve, 480));
    rememberPresentationSnapshot();
    store.updateSlide(index, {
      content: `按「${prompt}」优化：${slide.content}`
    });
    slideMagicPrompt.value = "";
    slideMagicMenuIndex.value = null;
    ElMessage.success("已按输入要求更新本页");
  } catch {
    slideMagicError.value = "AI 编辑失败，请重试";
  } finally {
    slideMagicBusy.value = false;
  }
}

async function handleDeleteHistory(item: PptHistoryItem) {
  try {
    await ElMessageBox.confirm(`确定删除「${item.title}」吗？删除后不可恢复。`, "删除演示文稿", {
      confirmButtonText: "删除",
      cancelButtonText: "取消",
      type: "warning"
    });
  } catch {
    return;
  }
  await store.removeHistoryItem(item.taskId);
  if (isFavoriteTask(item.taskId)) {
    saveFavoriteTaskIds(favoriteTaskIds.value.filter((taskId) => taskId !== item.taskId));
  }
  ElMessage.success("记录已删除");
}
</script>

<style scoped>
.ppt-generate-page {
  --ppt-bg-canvas: #070707;
  --ppt-bg-panel: #090909;
  --ppt-bg-card-dark: #0d0d0d;
  --ppt-bg-card-light: rgba(255, 255, 255, 0.92);
  --ppt-border-dark: #242424;
  --ppt-border-light: rgba(210, 212, 214, 0.7);
  --ppt-text-main-dark: #f4f4f5;
  --ppt-text-main-light: #111827;
  --ppt-text-muted: #8d8d93;
  --ppt-brand: #5a4db2;
  --ppt-brand-soft: #7d8df6;
  --ppt-accent-orange: #ff771b;
  --ppt-success: #20d4bf;
  --ppt-warning: #facc15;
  --ppt-danger: #fecaca;
  --ppt-radius-sm: 8px;
  --ppt-radius-md: 10px;
  --ppt-radius-lg: 14px;
  --ppt-radius-xl: 22px;
  --ppt-radius-pill: 999px;
  --ppt-focus-ring: 0 0 0 4px rgba(125, 141, 246, 0.16);
  --ppt-shadow-composer: 0 18px 48px rgba(90, 77, 178, 0.1);
  --ppt-shadow-menu: 0 18px 60px rgba(0, 0, 0, 0.5);
  --ppt-shadow-card-hover: 0 14px 30px rgba(90, 77, 178, 0.14);
  min-height: calc(100vh - 176px);
  margin: -16px;
  color: var(--ppt-text-main-dark);
  background: var(--ppt-bg-canvas);
}

.ppt-reference-shell {
  min-height: calc(100vh - 176px);
  border: 1px solid rgba(255, 255, 255, 0.07);
  background: var(--ppt-bg-canvas);
}

.ppt-reference-topbar {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 56px;
  padding: 0 18px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
  color: #8f8f95;
}

.ppt-brand-mark {
  display: grid;
  place-items: center;
  width: 32px;
  height: 32px;
  border: 0;
  border-radius: 999px;
  color: #9a9aa0;
  background: transparent;
  cursor: pointer;
  transition: color 0.16s ease, background-color 0.16s ease, transform 0.16s ease;
}

.ppt-brand-mark:hover,
.ppt-brand-mark:focus-visible {
  color: #f4f4f5;
  background: #171717;
  outline: 0;
  transform: translateY(-1px);
}

.ppt-brand-mark img {
  display: block;
  width: 26px;
  height: 26px;
  border-radius: 7px;
  object-fit: contain;
}

.ppt-module-title {
  color: #c7c7cc;
  font-size: 13px;
  font-weight: 760;
}

.ppt-reference-main {
  width: min(1280px, calc(100% - 64px));
  margin: 0 auto;
  padding: 26px 0 52px;
}

.ppt-reference-main.is-home-layout {
  width: min(1480px, calc(100% - 48px));
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(300px, 356px);
  align-items: start;
  column-gap: 24px;
  row-gap: 12px;
}

.ppt-reference-main.is-home-layout > .ppt-hero-composer,
.ppt-reference-main.is-home-layout > .ppt-library-toolbar,
.ppt-reference-main.is-home-layout > .ppt-workflow-board,
.ppt-reference-main.is-home-layout > .ppt-editor-board,
.ppt-reference-main.is-home-layout > .ppt-library-panel {
  grid-column: 1;
  min-width: 0;
}

.ppt-reference-main.is-home-layout > .ppt-home-side-panel {
  grid-column: 2;
  grid-row: 1 / span 5;
}

.ppt-hero-composer {
  position: relative;
  margin: 0 auto 18px;
}

.ppt-hero-composer h1 {
  margin: 0 0 26px;
  color: #f8fafc;
  text-align: center;
  font-size: 32px;
  font-weight: 860;
  line-height: 1.25;
  letter-spacing: 0;
}

.ppt-reference-main.is-home-layout .ppt-hero-composer {
  width: 100%;
  margin-bottom: 0;
}

.ppt-reference-main.is-home-layout .ppt-hero-composer h1 {
  margin-bottom: 12px;
  color: #111827;
  font-size: clamp(34px, 3.2vw, 44px);
  font-weight: 860;
}

.ppt-hero-subtitle {
  margin: 0 0 22px;
  color: #6b7280;
  text-align: center;
  font-size: 16px;
  line-height: 1.7;
}

.ppt-reference-main.is-home-layout .ppt-composer-card {
  z-index: 6;
  overflow: visible;
  min-height: 232px;
  border: 1px solid rgba(125, 141, 246, 0.36);
  border-radius: var(--ppt-radius-xl);
  background: var(--ppt-bg-card-light);
  box-shadow: var(--ppt-shadow-composer);
  backdrop-filter: blur(18px);
}

:global(.user-console-shell .ppt-doc-shell .ppt-reference-main.is-home-layout .ppt-composer-card) {
  min-height: 232px !important;
}

.ppt-reference-main.is-home-layout .ppt-composer-card::before {
  position: absolute;
  inset: 0;
  pointer-events: none;
  background:
    radial-gradient(circle at 16% 12%, rgba(125, 141, 246, 0.14), transparent 28%),
    radial-gradient(circle at 92% 90%, rgba(255, 119, 27, 0.12), transparent 26%);
  content: "";
}

.ppt-reference-main.is-home-layout .ppt-composer-card:focus-within {
  border-color: rgba(125, 141, 246, 0.62);
  box-shadow: var(--ppt-focus-ring), 0 20px 54px rgba(90, 77, 178, 0.13);
}

.ppt-reference-main.is-home-layout .ppt-reference-prompt :deep(textarea) {
  min-height: 152px;
  padding: 24px 26px 12px;
  color: var(--ppt-text-main-light);
  caret-color: var(--ppt-text-main-light);
}

.ppt-reference-main.is-home-layout .ppt-reference-prompt :deep(textarea::placeholder) {
  color: #9ca3af;
}

.ppt-reference-main.is-home-layout .ppt-reference-prompt :deep(footer) {
  color: #8a93a7;
}

.ppt-reference-main.is-home-layout .ppt-composer-footer {
  position: relative;
  z-index: 1;
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: end;
  gap: 14px;
  padding: 8px 18px 10px;
}

.ppt-reference-main.is-home-layout .ppt-pill-row {
  justify-content: flex-start;
}

.ppt-home-side-panel {
  position: sticky;
  top: 18px;
  display: grid;
  gap: 18px;
  min-width: 0;
}

.ppt-home-side-card {
  padding: 20px;
  border: 1px solid var(--ppt-border-light);
  border-radius: var(--ppt-radius-xl);
  background: rgba(255, 255, 255, 0.9);
  box-shadow: 0 16px 40px rgba(90, 77, 178, 0.08);
  backdrop-filter: blur(18px);
}

.ppt-home-side-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 16px;
}

.ppt-home-side-head h2 {
  margin: 0;
  color: #111827;
  font-size: 18px;
  font-weight: 800;
}

.ppt-home-side-head button {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  min-height: 32px;
  border: 0;
  color: #5a4db2;
  background: transparent;
  font-size: 13px;
  font-weight: 700;
}

.ppt-home-template-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}

.ppt-home-template-card {
  display: grid;
  gap: 9px;
  min-width: 0;
  padding: 0;
  border: 0;
  color: #111827;
  text-align: left;
  background: transparent;
}

.ppt-home-template-card strong {
  overflow: hidden;
  color: #111827;
  font-size: 14px;
  font-weight: 800;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ppt-home-template-card small {
  margin-top: -6px;
  color: #6b7280;
  font-size: 12px;
}

.ppt-home-template-cover {
  position: relative;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  min-height: 78px;
  padding: 12px;
  overflow: hidden;
  border: 1px solid rgba(210, 212, 214, 0.58);
  border-radius: 14px;
  background:
    linear-gradient(135deg, color-mix(in srgb, var(--template-accent) 84%, #ffffff 16%), rgba(255, 255, 255, 0.78)),
    radial-gradient(circle at 90% 18%, rgba(255, 255, 255, 0.58), transparent 34%);
  box-shadow: 0 10px 24px rgba(90, 77, 178, 0.08);
  transition: transform 0.18s ease, box-shadow 0.18s ease;
}

.ppt-home-template-cover::after {
  position: absolute;
  right: -20px;
  bottom: -28px;
  width: 82px;
  height: 82px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.34);
  content: "";
}

.ppt-home-template-cover b,
.ppt-home-template-cover em {
  position: relative;
  z-index: 1;
  color: #ffffff;
  font-style: normal;
}

.ppt-home-template-cover b {
  font-size: 12px;
  letter-spacing: 0.02em;
}

.ppt-home-template-cover em {
  font-size: 15px;
  font-weight: 800;
}

.ppt-home-template-card:hover .ppt-home-template-cover {
  box-shadow: 0 14px 30px rgba(90, 77, 178, 0.14);
  transform: translateY(-2px);
}

.ppt-home-inspiration-list {
  display: grid;
  gap: 10px;
}

.ppt-home-inspiration-item {
  display: grid;
  grid-template-columns: 34px minmax(0, 1fr) 16px;
  align-items: center;
  gap: 12px;
  min-height: 48px;
  padding: 10px 6px;
  border: 0;
  border-bottom: 1px solid rgba(210, 212, 214, 0.55);
  color: #1f2937;
  text-align: left;
  background: transparent;
}

.ppt-home-inspiration-item:last-child {
  border-bottom: 0;
}

.ppt-home-inspiration-item span:first-child {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  border-radius: 12px;
  color: #5a4db2;
  background: linear-gradient(135deg, rgba(125, 141, 246, 0.2), rgba(90, 77, 178, 0.08));
  font-size: 13px;
  font-weight: 800;
}

.ppt-home-inspiration-item strong {
  overflow: hidden;
  color: #374151;
  font-size: 14px;
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ppt-home-inspiration-item .ppt-toolbar-icon {
  width: 16px;
  height: 16px;
  color: #9ca3af;
}

.ppt-home-inspiration-item:hover strong,
.ppt-home-inspiration-item:hover .ppt-toolbar-icon {
  color: #5a4db2;
}

.ppt-composer-card {
  position: relative;
  min-height: 252px;
  border: 1px solid #232323;
  border-radius: var(--ppt-radius-lg);
  background: #0a0a0a;
  box-shadow: 0 18px 50px rgba(0, 0, 0, 0.28);
}

.ppt-composer-card:focus-within {
  border-color: #3d3d3d;
  box-shadow: 0 0 0 1px rgba(255, 255, 255, 0.05), 0 18px 50px rgba(0, 0, 0, 0.32);
}

.ppt-reference-prompt {
  display: block;
}

.ppt-reference-prompt :deep(textarea) {
  min-height: 178px;
  padding: 24px 24px 8px;
  border: 0;
  border-radius: 14px 14px 0 0;
  color: #f8fafc;
  caret-color: #ffffff;
  background: transparent;
  resize: none;
}

.ppt-reference-prompt :deep(textarea::placeholder) {
  color: #8d8d93;
}

.ppt-reference-prompt :deep(footer) {
  position: absolute;
  right: 74px;
  bottom: 30px;
  gap: 12px;
  color: #777;
}

.ppt-reference-prompt :deep(button) {
  color: #c9c9cf;
}

.ppt-composer-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18px;
  padding: 10px 18px 18px;
}

.ppt-pill-row {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  min-width: 0;
}

.ppt-pill-dropdown {
  position: relative;
}

.ppt-pill,
.ppt-library-tabs button,
.ppt-icon-button,
.ppt-library-actions > button,
.ppt-view-toggle button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  border: 1px solid #242424;
  color: #f2f2f2;
  background: #0c0c0c;
  cursor: pointer;
  transition: background-color 0.16s ease, border-color 0.16s ease, color 0.16s ease, transform 0.16s ease;
}

.ppt-pill:focus-visible,
.ppt-library-tabs button:focus-visible,
.ppt-icon-button:focus-visible,
.ppt-library-actions > button:focus-visible,
.ppt-view-toggle button:focus-visible,
.ppt-history-search-box input:focus-visible,
.ppt-history-search-box button:focus-visible {
  outline: 2px solid rgba(125, 141, 246, 0.72);
  outline-offset: 2px;
}

.ppt-pill:disabled,
.ppt-icon-button:disabled,
.ppt-library-actions > button:disabled,
.ppt-view-toggle button:disabled {
  opacity: 0.46;
  cursor: not-allowed;
}

.ppt-pill {
  min-height: 44px;
  padding: 0 16px;
  border-radius: 999px;
  font-size: 15px;
  font-weight: 780;
  white-space: nowrap;
}

.ppt-panels-icon,
.ppt-layout-icon,
.ppt-language-icon,
.ppt-wand-icon,
.ppt-menu-icon,
.ppt-bot-icon,
.ppt-tab-icon,
.ppt-toolbar-icon,
.ppt-view-icon,
.ppt-check-icon,
.ppt-chevron-icon,
.ppt-submit-icon {
  width: 17px;
  height: 17px;
  fill: none;
  stroke: currentColor;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.ppt-toolbar-icon.small {
  width: 14px;
  height: 14px;
}

.ppt-check-icon {
  width: 16px;
  height: 16px;
}

.ppt-pill:hover,
.ppt-pill.active,
.ppt-library-tabs button:hover,
.ppt-library-tabs button.active,
.ppt-icon-button:hover,
.ppt-icon-button.active,
.ppt-library-actions > button:hover,
.ppt-view-toggle button:hover,
.ppt-view-toggle button.active {
  border-color: #3a3a3a;
  background: #181818;
}

.ppt-pill[aria-expanded="true"] {
  border-color: #3a3a3a;
  background: #181818;
}

.ppt-pill-menu {
  position: absolute;
  left: 0;
  top: calc(100% + 10px);
  z-index: 24;
  display: grid;
  gap: 4px;
  width: min(210px, calc(100vw - 24px));
  max-height: min(420px, calc(100vh - 160px));
  overflow: auto;
  padding: 8px;
  border: 1px solid #2b2b2b;
  border-radius: 10px;
  background: #101010;
  box-shadow: var(--ppt-shadow-menu);
  overscroll-behavior: contain;
}

.ppt-more-menu {
  width: min(230px, calc(100vw - 24px));
}

.ppt-model-menu {
  width: 320px;
  max-width: calc(100vw - 24px);
  max-height: min(520px, calc(100vh - 180px));
  overflow: auto;
}

.ppt-model-menu-state {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 58px;
  padding: 10px;
  color: #f4f4f5;
}

.ppt-model-menu-state > span:last-child {
  display: grid;
  gap: 3px;
  min-width: 0;
}

.ppt-model-menu-state b {
  color: #f8fafc;
  font-size: 14px;
  font-weight: 780;
}

.ppt-model-menu-state small {
  color: #a1a1aa;
  font-size: 12px;
}

.ppt-pill-menu strong {
  padding: 6px 8px 4px;
  color: #8f8f95;
  font-size: 12px;
}

.ppt-model-group {
  display: grid;
  gap: 4px;
}

.ppt-model-group + .ppt-model-group {
  margin-top: 6px;
  padding-top: 6px;
  border-top: 1px solid #202020;
}

.ppt-pill-menu button {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-height: 36px;
  padding: 0 10px;
  border: 0;
  border-radius: 8px;
  color: #f4f4f5;
  background: transparent;
  text-align: left;
  cursor: pointer;
}

.ppt-pill-menu button:hover,
.ppt-pill-menu button:focus-visible,
.ppt-pill-menu button.active {
  background: #1f1f1f;
  outline: 0;
}

.ppt-pill-menu button:disabled {
  opacity: 0.48;
  cursor: not-allowed;
}

.ppt-pill-menu button:disabled:hover {
  background: transparent;
}

.ppt-pill-menu button small {
  color: #a3a3a3;
  font-size: 12px;
}

.ppt-more-menu button span {
  flex: 1;
}

.ppt-more-menu .ppt-menu-icon {
  flex: 0 0 auto;
}

.ppt-model-menu button {
  min-height: 56px;
  align-items: flex-start;
  padding: 9px 10px;
}

.ppt-model-menu button > span {
  flex: 1;
  display: grid;
  gap: 2px;
  min-width: 0;
}

.ppt-model-menu button b {
  overflow: hidden;
  font-size: 14px;
  font-weight: 760;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ppt-model-menu button small {
  line-height: 1.35;
}

.ppt-model-menu .ppt-menu-icon {
  flex: 0 0 auto;
  margin-top: 2px;
}

.ppt-chevron-icon {
  width: 14px;
  height: 14px;
  color: #a3a3a3;
}

.ppt-submit-icon {
  width: 20px;
  height: 20px;
  stroke-width: 2.25;
}

.ppt-model-pill small {
  color: #a3a3a3;
  font-size: 13px;
}

.ppt-submit-button {
  flex: 0 0 auto;
  display: grid;
  place-items: center;
  width: 44px;
  height: 44px;
  border: 0;
  border-radius: 999px;
  color: #111;
  background: #a8a8a8;
  cursor: pointer;
  transition: transform 0.16s ease, background-color 0.16s ease, opacity 0.16s ease;
}

.ppt-submit-button:focus-visible {
  outline: 2px solid rgba(125, 141, 246, 0.72);
  outline-offset: 3px;
}

.ppt-submit-button:hover:not(:disabled) {
  transform: translateY(-1px);
  background: #f4f4f4;
}

.ppt-submit-button:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

.ppt-spinner {
  width: 18px;
  height: 18px;
  border: 2px solid rgba(0, 0, 0, 0.2);
  border-top-color: #111;
  border-radius: 999px;
  animation: ppt-spin 0.8s linear infinite;
}

.ppt-floating-config {
  position: absolute;
  left: 18px;
  right: 18px;
  top: calc(100% + 10px);
  z-index: 10;
  display: grid;
  gap: 18px;
  max-height: min(70vh, 720px);
  overflow: auto;
  padding: 18px;
  border: 1px solid #2b2b2b;
  border-radius: 12px;
  background: #101010;
  box-shadow: 0 26px 80px rgba(0, 0, 0, 0.52);
  overscroll-behavior: contain;
}

.ppt-config-block {
  display: grid;
  gap: 12px;
}

.ppt-mini-heading {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 16px;
}

.ppt-mini-heading strong {
  color: #f4f4f5;
  font-size: 14px;
}

.ppt-mini-heading span {
  color: #8f8f95;
  font-size: 12px;
}

.ppt-hero-error,
.ppt-hero-progress {
  margin-top: 14px;
}

.ppt-library-toolbar {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  padding: 0 0 10px;
}

.ppt-library-tabs,
.ppt-library-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.ppt-library-tabs button {
  min-height: 44px;
  padding: 0 14px;
  border-color: transparent;
  border-radius: 9px;
  color: #a7a7a7;
  background: transparent;
  font-size: 15px;
  font-weight: 760;
}

.ppt-library-tabs button.active {
  color: #f5f5f5;
  border-color: #2b2b2b;
  background: #333;
}

.ppt-sr-only {
  position: absolute;
  overflow: hidden;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  border: 0;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
}

.ppt-history-search {
  position: relative;
  flex: 0 0 auto;
  width: 42px;
  height: 42px;
  overflow: hidden;
  transition: width 0.24s ease;
}

.ppt-history-search.open {
  width: clamp(180px, 22vw, 260px);
}

.ppt-icon-button,
.ppt-library-actions > button {
  height: 42px;
  min-width: 42px;
  padding: 0 12px;
  border-radius: 9px;
  font-size: 15px;
}

.ppt-search-trigger,
.ppt-history-search-box {
  position: absolute;
  inset: 0;
}

.ppt-search-trigger {
  padding: 0;
  transition: opacity 0.16s ease, transform 0.16s ease;
}

.ppt-search-trigger.hidden {
  pointer-events: none;
  opacity: 0;
  transform: scale(0.95);
}

.ppt-history-search-box {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 9px 0 12px;
  border: 1px solid #242424;
  border-radius: 9px;
  color: #a7a7a7;
  background: #0c0c0c;
  opacity: 0;
  pointer-events: none;
  transform: translateX(8px);
  transition: opacity 0.2s ease, transform 0.2s ease, border-color 0.16s ease;
}

.ppt-history-search-box.open {
  opacity: 1;
  pointer-events: auto;
  transform: translateX(0);
}

.ppt-history-search-box:focus-within {
  border-color: #3a3a3a;
  color: #f4f4f5;
  box-shadow: 0 0 0 3px rgba(125, 141, 246, 0.12);
}

.ppt-history-search-box input {
  min-width: 0;
  flex: 1;
  height: 100%;
  border: 0;
  outline: 0;
  color: #f4f4f5;
  caret-color: #f4f4f5;
  background: transparent;
  font-size: 14px;
}

.ppt-history-search-box input::placeholder {
  color: #777;
}

.ppt-history-search-box button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border: 0;
  border-radius: 6px;
  color: #9d9d9d;
  background: transparent;
  cursor: pointer;
}

.ppt-history-search-box button:hover {
  color: #f4f4f5;
  background: #191919;
}

.ppt-create-button {
  padding: 0 16px !important;
  font-weight: 780;
}

.ppt-filter-dropdown {
  position: relative;
  flex: 0 0 auto;
}

.ppt-filter-button {
  position: relative;
  padding: 0;
}

.ppt-filter-count {
  position: absolute;
  top: -6px;
  right: -6px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 18px;
  height: 18px;
  padding: 0 5px;
  border-radius: 999px;
  color: #06211f;
  background: #20d4bf;
  font-size: 11px;
  font-weight: 800;
}

.ppt-history-filter-menu {
  position: absolute;
  top: calc(100% + 10px);
  right: 0;
  z-index: 28;
  display: grid;
  gap: 4px;
  width: min(218px, calc(100vw - 24px));
  max-height: min(460px, calc(100vh - 170px));
  overflow: auto;
  padding: 8px;
  border: 1px solid #2b2b2b;
  border-radius: 10px;
  background: #101010;
  box-shadow: var(--ppt-shadow-menu);
  overscroll-behavior: contain;
}

.ppt-history-filter-menu p {
  margin: 4px 8px 6px;
  color: #8f8f95;
  font-size: 12px;
  font-weight: 760;
}

.ppt-history-filter-menu button {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  min-height: 36px;
  padding: 0 9px;
  border: 0;
  border-radius: 7px;
  color: #e5e5e5;
  background: transparent;
  cursor: pointer;
  font-size: 13px;
  text-align: left;
}

.ppt-history-filter-menu button:hover,
.ppt-history-filter-menu button:focus-visible {
  background: #1c1c1c;
  outline: 0;
}

.ppt-history-filter-menu button span {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.ppt-menu-separator {
  height: 1px;
  margin: 5px 2px;
  background: #262626;
}

.ppt-star-filter-icon.active {
  color: #facc15;
  fill: currentColor;
}

.ppt-view-toggle {
  display: inline-flex;
  padding: 3px;
  border: 1px solid #242424;
  border-radius: 10px;
  background: #0c0c0c;
}

.ppt-view-toggle button {
  min-height: 34px;
  padding: 0 12px;
  border: 0;
  border-radius: 8px;
  color: #9d9d9d;
}

.ppt-view-toggle button.active {
  color: #f4f4f4;
  background: #2c2c2c;
}

.ppt-library-panel,
.ppt-workflow-board,
.ppt-editor-board {
  border: 1px solid #202020;
  border-radius: 8px;
  background: #090909;
}

.ppt-library-panel {
  min-height: 292px;
  padding: 18px;
}

.ppt-workflow-board,
.ppt-editor-board {
  display: grid;
  grid-template-columns: minmax(320px, 0.92fr) minmax(420px, 1.08fr);
  gap: 16px;
  margin-bottom: 18px;
  padding: 16px;
}

.ppt-board-panel {
  min-width: 0;
  padding: 16px;
  border: 1px solid #242424;
  border-radius: 8px;
  background: #0d0d0d;
}

.ppt-outline-panel {
  display: grid;
  gap: 14px;
}

.ppt-library-panel :deep(.ppt-history-list.is-grid) {
  grid-template-columns: repeat(4, minmax(220px, 1fr));
}

.ppt-library-panel :deep(.ppt-history-list.is-grid .ppt-history-card) {
  min-height: 252px;
}

.ppt-library-panel :deep(.ppt-history-list.is-grid .ppt-history-preview) {
  height: 168px;
}

.ppt-library-panel :deep(.ppt-history-card-body) {
  background: #090909;
}

.ppt-generation-workspace {
  display: grid;
  gap: 18px;
  width: min(100%, 960px);
  margin: 0 auto;
  padding-bottom: 96px;
}

.ppt-generation-workspace.is-pre-outline {
  gap: 28px;
  width: min(100%, 980px);
}

.ppt-generate-header {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  align-items: start;
  gap: 16px;
  padding: 18px;
  border: 1px solid #202020;
  border-radius: 12px;
  background: #0a0a0a;
}

.ppt-generate-header.is-pre-outline {
  align-items: center;
  padding: 14px 16px;
}

.ppt-back-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  min-height: 38px;
  padding: 0 12px;
  border: 1px solid #242424;
  border-radius: 9px;
  color: #d4d4d8;
  background: #0d0d0d;
  cursor: pointer;
  font-weight: 760;
  transition: background-color 0.16s ease, border-color 0.16s ease, transform 0.16s ease;
}

.ppt-back-button:hover {
  border-color: #3a3a3a;
  background: #171717;
  transform: translateY(-1px);
}

.ppt-generate-header div {
  display: grid;
  gap: 8px;
  min-width: 0;
}

.ppt-generate-header div > span {
  color: #8f8f95;
  font-size: 13px;
  font-weight: 760;
}

.ppt-generate-header h1 {
  overflow: hidden;
  margin: 0;
  color: #f8fafc;
  font-size: 28px;
  line-height: 1.22;
  letter-spacing: 0;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ppt-generate-header p {
  display: -webkit-box;
  overflow: hidden;
  margin: 0;
  color: #a1a1aa;
  line-height: 1.65;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
  word-break: break-word;
}

.ppt-generate-header.is-pre-outline h1 {
  font-size: 17px;
}

.ppt-generate-header.is-pre-outline p,
.ppt-generate-header.is-pre-outline div > span {
  display: none;
}

.ppt-workspace-status {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 30px;
  padding: 0 10px;
  border: 1px solid #2b2b2b;
  border-radius: 999px;
  color: #d4d4d8;
  background: #111;
  font-size: 12px;
  font-weight: 820;
  white-space: nowrap;
}

.ppt-workspace-status.status-success,
.ppt-workspace-status.status-outline_ready {
  border-color: rgba(34, 197, 94, 0.42);
  color: #bbf7d0;
  background: rgba(22, 101, 52, 0.24);
}

.ppt-workspace-status.status-failed {
  border-color: rgba(248, 113, 113, 0.44);
  color: #fecaca;
  background: rgba(127, 29, 29, 0.25);
}

.ppt-workspace-status.status-outlining,
.ppt-workspace-status.status-pending,
.ppt-workspace-status.status-generating,
.ppt-workspace-status.status-rendering {
  border-color: rgba(96, 165, 250, 0.38);
  color: #bfdbfe;
  background: rgba(30, 64, 175, 0.24);
}

.ppt-generate-header .ppt-generate-header-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
}

.ppt-generate-header.is-pre-outline .ppt-generate-header-actions {
  flex-wrap: wrap;
}

.ppt-generate-header-chips {
  display: flex !important;
  align-items: center;
  justify-content: flex-end;
  gap: 8px !important;
  min-width: 0;
}

.ppt-generate-header-chips span {
  display: inline-flex !important;
  align-items: center;
  min-height: 28px;
  padding: 0 10px;
  border: 1px solid #202020;
  border-radius: 999px;
  color: #d4d4d8 !important;
  background: #111;
  font-size: 12px !important;
  font-weight: 760;
  line-height: 1;
  white-space: nowrap;
}

.ppt-generation-settings-toggle {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 7px;
  min-height: 32px;
  padding: 0 10px;
  border: 1px solid #2b2b2b;
  border-radius: 999px;
  color: #d4d4d8;
  background: #111;
  cursor: pointer;
  font-size: 12px;
  font-weight: 780;
  white-space: nowrap;
}

.ppt-generation-settings-toggle:hover {
  border-color: #3f3f46;
  color: #f4f4f5;
  background: #171717;
}

.ppt-generation-settings-toggle:disabled {
  opacity: 0.48;
  cursor: not-allowed;
}

.ppt-generation-settings-toggle:disabled:hover {
  border-color: #2b2b2b;
  color: #d4d4d8;
  background: #111;
}

.ppt-generation-settings-toggle:focus-visible,
.ppt-generation-primary:focus-visible,
.ppt-generation-help-control > button:focus-visible,
.ppt-back-button:focus-visible,
.ppt-generation-regenerate:focus-visible,
.ppt-generation-switch:focus-visible {
  outline: 2px solid rgba(34, 211, 238, 0.72);
  outline-offset: 2px;
}

.ppt-workspace-error,
.ppt-workspace-progress {
  margin: 0;
}

.ppt-generation-settings {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.ppt-setting-chip {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  flex: 0 1 auto;
  min-width: 0;
  max-width: 100%;
  min-height: 30px;
  padding: 0 10px;
  border: 1px solid #202020;
  border-radius: 999px;
  background: #111;
}

.ppt-setting-chip b {
  max-width: min(220px, 58vw);
  overflow: hidden;
  color: #f4f4f5;
  font-size: 13px;
  line-height: 1;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ppt-setting-chip small {
  color: #8f8f95;
  font-size: 11px;
  line-height: 1;
  white-space: nowrap;
}

.ppt-setting-chip small::before {
  content: "/";
  margin-right: 7px;
  color: #52525b;
}

.ppt-generation-settings-editor {
  display: grid;
  gap: 16px;
  padding: 16px;
  border: 1px solid #202020;
  border-radius: 10px;
  background: #090909;
}

.ppt-generation-settings-editor.is-disabled {
  opacity: 0.74;
}

.ppt-outline-prompt-field,
.ppt-generation-extra-settings label {
  display: grid;
  gap: 8px;
}

.ppt-outline-prompt-field span,
.ppt-generation-extra-settings label span {
  color: #d4d4d8;
  font-size: 13px;
  font-weight: 760;
}

.ppt-outline-prompt-field textarea,
.ppt-generation-extra-settings select {
  width: 100%;
  box-sizing: border-box;
  border: 1px solid #2b2b2b;
  border-radius: 8px;
  color: #f4f4f5;
  caret-color: #fff;
  background: #0d0d0d;
  outline: none;
}

.ppt-outline-prompt-field textarea {
  min-height: 92px;
  padding: 10px;
  resize: vertical;
}

.ppt-generation-extra-settings select {
  min-height: 38px;
  padding: 0 10px;
}

.ppt-outline-prompt-field textarea:focus,
.ppt-generation-extra-settings select:focus {
  border-color: #3f3f46;
  box-shadow: 0 0 0 3px rgba(34, 211, 238, 0.1);
}

.ppt-generation-extra-settings {
  display: grid;
  grid-template-columns: minmax(180px, 1fr) minmax(180px, 0.8fr) auto;
  gap: 12px;
  align-items: end;
}

.ppt-generation-switch,
.ppt-generation-regenerate {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 9px;
  min-height: 38px;
  padding: 0 12px;
  border: 1px solid #2b2b2b;
  border-radius: 8px;
  color: #f4f4f5;
  background: #111;
  cursor: pointer;
  font-weight: 780;
}

.ppt-generation-switch {
  justify-content: space-between;
}

.ppt-generation-switch b {
  color: #a1a1aa;
  font-size: 12px;
}

.ppt-generation-switch.active {
  border-color: rgba(34, 211, 238, 0.48);
  background: rgba(8, 145, 178, 0.16);
}

.ppt-generation-switch.active b {
  color: #67e8f9;
}

.ppt-generation-regenerate:hover:not(:disabled),
.ppt-generation-switch:hover:not(:disabled) {
  border-color: #3f3f46;
  background: #171717;
}

.ppt-generation-regenerate:disabled,
.ppt-generation-switch:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.ppt-generation-steps {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
}

.ppt-generation-steps article {
  display: flex;
  gap: 10px;
  min-width: 0;
  padding: 12px;
  border: 1px solid #202020;
  border-radius: 10px;
  background: #090909;
}

.ppt-generation-step-index {
  display: grid;
  place-items: center;
  flex: 0 0 30px;
  width: 30px;
  height: 30px;
  border: 1px solid #2b2b2b;
  border-radius: 50%;
  color: #a1a1aa;
  background: #111;
  font-size: 12px;
  font-weight: 860;
}

.ppt-generation-steps article div {
  display: grid;
  gap: 4px;
  min-width: 0;
}

.ppt-generation-steps strong {
  color: #f4f4f5;
  font-size: 13px;
}

.ppt-generation-steps small {
  color: #8f8f95;
  font-size: 12px;
  line-height: 1.45;
}

.ppt-generation-steps article.is-active {
  border-color: rgba(96, 165, 250, 0.38);
  background: rgba(30, 64, 175, 0.18);
}

.ppt-generation-steps article.is-active .ppt-generation-step-index {
  color: #bfdbfe;
  border-color: rgba(96, 165, 250, 0.45);
  background: rgba(37, 99, 235, 0.2);
}

.ppt-generation-steps article.is-done .ppt-generation-step-index {
  color: #bbf7d0;
  border-color: rgba(34, 197, 94, 0.44);
  background: rgba(22, 101, 52, 0.24);
}

.ppt-generation-steps article.is-failed .ppt-generation-step-index {
  color: #fecaca;
  border-color: rgba(248, 113, 113, 0.44);
  background: rgba(127, 29, 29, 0.26);
}

.ppt-pre-outline-panel {
  display: grid;
  gap: 22px;
  padding: 28px;
  border: 1px solid #202020;
  border-radius: 14px;
  background: #090909;
  box-shadow: 0 18px 48px rgba(0, 0, 0, 0.22);
}

.ppt-pre-section-heading {
  display: flex;
  align-items: flex-start;
  gap: 12px;
}

.ppt-pre-section-heading > svg {
  flex: 0 0 24px;
  margin-top: 4px;
  color: #f4f4f5;
}

.ppt-pre-section-heading div {
  display: grid;
  gap: 10px;
  min-width: 0;
}

.ppt-pre-section-heading h2 {
  margin: 0;
  color: #f8fafc;
  font-size: 24px;
  line-height: 1.2;
  letter-spacing: 0;
}

.ppt-pre-section-heading p {
  margin: 0;
  color: #a1a1aa;
  line-height: 1.5;
}

.ppt-pre-text-content-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 14px;
}

.ppt-pre-text-content-grid button {
  display: grid;
  justify-items: center;
  align-content: center;
  gap: 14px;
  min-height: 136px;
  padding: 18px 12px;
  border: 2px solid #242424;
  border-radius: 10px;
  color: #e4e4e7;
  background: #0d0d0d;
  cursor: pointer;
  transition: border-color 0.16s ease, background-color 0.16s ease, transform 0.16s ease, box-shadow 0.16s ease;
}

.ppt-pre-text-content-grid button:hover,
.ppt-pre-text-content-grid button.active {
  border-color: #52525b;
  background: #18181b;
  transform: translateY(-1px);
}

.ppt-pre-text-content-grid button.active {
  color: #f8fafc;
  box-shadow: inset 0 0 0 1px rgba(244, 244, 245, 0.2);
}

.ppt-pre-text-lines {
  display: grid;
  justify-items: center;
  align-content: center;
  gap: 6px;
  width: 100%;
  height: 44px;
}

.ppt-pre-text-lines i {
  display: block;
  height: 4px;
  border-radius: 999px;
  background: #71717a;
}

.ppt-pre-text-content-grid button.active .ppt-pre-text-lines i {
  background: #f4f4f5;
}

.ppt-pre-text-content-grid strong {
  color: currentColor;
  font-size: 14px;
}

.ppt-pre-select-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 20px;
}

.ppt-pre-select-grid label {
  display: grid;
  gap: 10px;
  min-width: 0;
}

.ppt-pre-select-grid span {
  color: #e4e4e7;
  font-size: 14px;
  font-weight: 780;
}

.ppt-pre-select-grid select {
  width: 100%;
  min-height: 46px;
  padding: 0 12px;
  border: 1px solid #2b2b2b;
  border-radius: 8px;
  color: #f4f4f5;
  background: #0d0d0d;
  outline: none;
}

.ppt-pre-select-grid select:focus {
  border-color: #3f3f46;
  box-shadow: 0 0 0 3px rgba(34, 211, 238, 0.1);
}

.ppt-pre-theme-panel {
  margin-bottom: 8px;
}

.ppt-generation-flow {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 16px;
  padding: 0;
  border: 0;
  border-radius: 0;
  background: transparent;
}

.ppt-panel-heading {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 14px;
}

.ppt-panel-heading strong {
  color: #f4f4f5;
  font-size: 15px;
}

.ppt-panel-heading span {
  color: #8f8f95;
  font-size: 12px;
}

.ppt-generation-bottom-bar {
  position: fixed;
  right: 0;
  bottom: 0;
  left: 200px;
  z-index: 26;
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 76px;
  margin: 0;
  padding: 14px 20px;
  border-top: 1px solid #222;
  background: rgba(7, 7, 7, 0.94);
  box-shadow: 0 -18px 48px rgba(0, 0, 0, 0.34);
  backdrop-filter: blur(14px);
}

:global(.desktop-sidebar-collapsed) .ppt-generation-bottom-bar {
  left: 0;
}

.ppt-generation-primary {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 9px;
  min-height: 44px;
  min-width: 196px;
  padding: 0 18px;
  border: 0;
  border-radius: 999px;
  color: #111;
  background: #f4f4f5;
  cursor: pointer;
  font-weight: 860;
  transition: transform 0.16s ease, background-color 0.16s ease, opacity 0.16s ease;
}

.ppt-generation-primary:hover:not(:disabled) {
  transform: translateY(-1px);
  background: #ffffff;
}

.ppt-generation-primary span {
  color: #111;
  font-size: 14px;
}

.ppt-generation-primary:disabled {
  opacity: 0.52;
  cursor: not-allowed;
}

.ppt-generation-help-control {
  position: fixed;
  right: 24px;
  bottom: 92px;
  z-index: 70;
}

.ppt-generation-help-control > button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  border: 1px solid #303030;
  border-radius: 999px;
  color: #f4f4f5;
  background: #151515;
  box-shadow: 0 14px 32px rgba(0, 0, 0, 0.42);
  cursor: pointer;
  font-size: 17px;
  font-weight: 850;
  transition: transform 0.16s ease, border-color 0.16s ease, background-color 0.16s ease;
}

.ppt-generation-help-control > button:hover {
  transform: translateY(-1px);
  border-color: #4a4a4a;
  background: #202020;
}

.ppt-presentation-workspace {
  display: grid;
  gap: 14px;
  min-height: calc(100vh - 230px);
}

:global(body.ppt-presenting-body) {
  overflow: hidden;
}

.ppt-presentation-workspace.is-presenting {
  position: fixed;
  inset: 0;
  z-index: 9999;
  display: block;
  width: 100vw;
  height: 100vh;
  min-height: 100vh;
  margin: 0;
  padding: 0;
  overflow: hidden;
  background: #050505;
}

@supports (height: 100dvh) {
  .ppt-presentation-workspace.is-presenting {
    width: 100dvw;
    height: 100dvh;
    min-height: 100dvh;
  }
}

.ppt-presentation-workspace.is-presenting .ppt-presentation-header,
.ppt-presentation-workspace.is-presenting .ppt-presentation-progress,
.ppt-presentation-workspace.is-presenting .ppt-slide-sidebar-shell,
.ppt-presentation-workspace.is-presenting .ppt-right-edit-panel {
  display: none;
}

.ppt-presentation-workspace.is-presenting .ppt-presentation-editor {
  display: block;
  width: 100vw;
  height: 100vh;
  min-height: 100vh;
  margin: 0;
  padding: 0;
  overflow: hidden;
}

.ppt-presentation-workspace.is-presenting .ppt-presentation-stage {
  display: grid;
  place-items: center;
  width: 100vw;
  height: 100vh;
  min-height: 100vh;
  margin: 0;
  padding: 0;
  border: 0;
  border-radius: 0;
  background: #050505;
  overflow: hidden;
}

@supports (height: 100dvh) {
  .ppt-presentation-workspace.is-presenting .ppt-presentation-editor,
  .ppt-presentation-workspace.is-presenting .ppt-presentation-stage {
    width: 100dvw;
    height: 100dvh;
    min-height: 100dvh;
  }
}

.ppt-present-header {
  position: fixed;
  top: 0;
  right: 0;
  left: 0;
  z-index: 10020;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18px;
  padding: 14px 28px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.12);
  color: #fff;
  background: rgba(0, 0, 0, 0.82);
  backdrop-filter: blur(10px);
  transform: translateY(-110%);
  transition: transform 0.24s ease;
}

.ppt-present-header.visible {
  transform: translateY(0);
}

.ppt-present-header strong {
  min-width: 0;
  overflow: hidden;
  color: #fff;
  font-size: 17px;
  font-weight: 840;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ppt-present-header button {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-height: 36px;
  padding: 0 14px;
  border: 1px solid rgba(255, 255, 255, 0.18);
  border-radius: 8px;
  color: #fff;
  background: rgba(255, 255, 255, 0.09);
  cursor: pointer;
}

.ppt-present-header button:hover:not(:disabled) {
  background: rgba(255, 255, 255, 0.16);
}

.ppt-present-header button:focus-visible,
.ppt-present-phone-top-hitarea:focus-visible,
.ppt-header-icon-button:focus-visible,
.ppt-presentation-action:focus-visible {
  outline: 2px solid rgba(34, 211, 238, 0.72);
  outline-offset: 2px;
}

.ppt-present-phone-top-hitarea,
.ppt-present-phone-overlay {
  display: none;
}

.ppt-presentation-header {
  position: sticky;
  top: 0;
  z-index: 30;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18px;
  padding: 10px 12px;
  border: 1px solid #202020;
  border-radius: 10px;
  background: rgba(9, 9, 9, 0.95);
  backdrop-filter: blur(14px);
}

.ppt-presentation-titlebar,
.ppt-presentation-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.ppt-presentation-titlebar {
  flex: 1;
}

.ppt-header-icon-button,
.ppt-presentation-action {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  min-height: 36px;
  border: 1px solid #262626;
  border-radius: 8px;
  color: #f4f4f5;
  background: #101010;
  cursor: pointer;
  transition: background-color 0.16s ease, border-color 0.16s ease, transform 0.16s ease;
}

.ppt-header-icon-button {
  width: 38px;
  padding: 0;
}

.ppt-header-icon-button.is-brain-entry {
  color: #67e8f9;
  border-color: rgba(34, 211, 238, 0.34);
  background: linear-gradient(135deg, rgba(8, 47, 73, 0.72), rgba(8, 51, 68, 0.42));
}

.ppt-header-icon-button.is-brain-entry:hover {
  color: #ecfeff;
  border-color: rgba(34, 211, 238, 0.58);
  background: linear-gradient(135deg, rgba(14, 116, 144, 0.42), rgba(8, 47, 73, 0.72));
}

.ppt-presentation-action {
  padding: 0 12px;
  font-weight: 760;
  white-space: nowrap;
}

.ppt-header-icon-button:hover,
.ppt-presentation-action:hover,
.ppt-presentation-action.active {
  border-color: #3a3a3a;
  background: #181818;
  transform: translateY(-1px);
}

.ppt-presentation-action.is-primary {
  color: #111;
  border-color: #f4f4f5;
  background: #f4f4f5;
}

.ppt-presentation-action:disabled {
  opacity: 0.6;
  cursor: not-allowed;
  transform: none;
}

.ppt-presentation-titlebar input {
  min-width: 0;
  flex: 1;
  height: 36px;
  border: 0;
  outline: 0;
  color: #f8fafc;
  background: transparent;
  font-size: 16px;
  font-weight: 820;
}

.ppt-saving-indicator {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  min-width: 64px;
  color: #86efac;
  font-size: 12px;
  font-weight: 760;
  white-space: nowrap;
}

.ppt-saving-indicator.is-saving {
  color: #a1a1aa;
}

.ppt-saving-indicator.is-error {
  color: #fca5a5;
}

.ppt-saving-indicator .ppt-toolbar-icon {
  width: 14px;
  height: 14px;
}

.ppt-saving-indicator .ppt-spinner {
  width: 13px;
  height: 13px;
  border-width: 2px;
}

.ppt-history-buttons {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding-left: 2px;
}

.ppt-history-buttons button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  min-height: 32px;
  border: 1px solid transparent;
  border-radius: 8px;
  color: #d4d4d8;
  background: transparent;
  cursor: pointer;
}

.ppt-history-buttons button:hover:not(:disabled) {
  border-color: #333;
  color: #f4f4f5;
  background: #181818;
}

.ppt-history-buttons button:disabled {
  color: #555;
  cursor: not-allowed;
}

.ppt-presentation-menu {
  position: relative;
}

.ppt-presentation-menu-popover {
  position: absolute;
  top: calc(100% + 8px);
  left: 0;
  z-index: 40;
  display: grid;
  gap: 4px;
  width: 256px;
  padding: 8px;
  border: 1px solid #2b2b2b;
  border-radius: 10px;
  background: #101010;
  box-shadow: 0 18px 60px rgba(0, 0, 0, 0.5);
}

.ppt-presentation-menu-popover p {
  margin: 5px 8px 4px;
  color: #8f8f95;
  font-size: 11px;
  font-weight: 860;
  letter-spacing: 0;
  text-transform: uppercase;
}

.ppt-presentation-menu-popover button {
  display: grid;
  grid-template-columns: 18px minmax(0, 1fr) auto;
  align-items: center;
  gap: 10px;
  min-height: 36px;
  padding: 0 9px;
  border: 0;
  border-radius: 8px;
  color: #f4f4f5;
  background: transparent;
  cursor: pointer;
  text-align: left;
}

.ppt-presentation-menu-popover button:hover:not(:disabled) {
  background: #1d1d1d;
}

.ppt-presentation-menu-popover button:focus-visible {
  outline: 2px solid #22d3ee;
  outline-offset: 2px;
  background: #1d1d1d;
}

.ppt-presentation-menu-popover button:disabled {
  color: #666;
  cursor: not-allowed;
}

.ppt-presentation-menu-popover small {
  color: #8f8f95;
  font-size: 11px;
}

.ppt-presentation-export {
  position: relative;
}

.ppt-presentation-progress {
  margin: 0;
}

.ppt-presentation-editor {
  display: grid;
  grid-template-columns: var(--ppt-sidebar-width, 150px) minmax(0, 1fr) 320px;
  gap: 14px;
  min-height: 680px;
}

.ppt-slide-sidebar-shell,
.ppt-right-edit-panel {
  min-width: 0;
  border: 1px solid #202020;
  border-radius: 10px;
  background: #090909;
}

.ppt-presentation-editor.is-sidebar-resizing,
.ppt-presentation-editor.is-sidebar-resizing * {
  cursor: col-resize;
  user-select: none;
}

.ppt-slide-sidebar-shell {
  position: relative;
  z-index: 4;
  min-height: 0;
}

.ppt-slide-sidebar {
  display: grid;
  grid-template-rows: auto minmax(0, 1fr);
  gap: 10px;
  height: 100%;
  max-height: calc(100vh - 250px);
  padding: 10px;
}

.ppt-slide-sidebar-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 2px 2px 8px;
  border-bottom: 1px solid #202020;
}

.ppt-slide-sidebar-header div {
  display: grid;
  gap: 2px;
}

.ppt-slide-sidebar-header strong {
  color: #f4f4f5;
  font-size: 13px;
  font-weight: 820;
}

.ppt-slide-sidebar-header span {
  color: #8f8f95;
  font-size: 11px;
  font-weight: 650;
}

.ppt-slide-sidebar-header button,
.ppt-slide-sidebar-expand {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  min-height: 32px;
  border: 1px solid #2b2b2b;
  border-radius: 8px;
  color: #d4d4d8;
  background: #101010;
  cursor: pointer;
}

.ppt-slide-sidebar-header button:hover,
.ppt-slide-sidebar-expand:hover {
  border-color: #3f3f46;
  background: #191919;
}

.ppt-slide-sidebar-header button:focus-visible,
.ppt-slide-sidebar-expand:focus-visible {
  outline: 2px solid #22d3ee;
  outline-offset: 2px;
}

.ppt-slide-sidebar-expand {
  margin: 10px auto;
}

.ppt-slide-sidebar-list {
  display: grid;
  gap: 10px;
  align-content: start;
  overflow: auto;
  padding-right: 2px;
}

.ppt-slide-thumbnail-card {
  position: relative;
  display: grid;
  min-height: 0;
  padding: 6px;
  border: 1px solid #262626;
  border-radius: 7px;
  color: #d4d4d8;
  background: #0d0d0d;
  cursor: pointer;
  text-align: left;
  transition: border-color 0.16s ease, background-color 0.16s ease, box-shadow 0.16s ease, transform 0.16s ease;
}

.ppt-slide-thumbnail-card.active,
.ppt-slide-thumbnail-card:hover {
  border-color: rgba(34, 211, 238, 0.5);
  background: #1b1b1b;
}

.ppt-slide-thumbnail-card.active {
  box-shadow: 0 0 0 1px rgba(34, 211, 238, 0.65) inset, 0 0 0 3px rgba(34, 211, 238, 0.12);
}

.ppt-slide-thumbnail-card.active .ppt-slide-number {
  color: #051014;
  background: #22d3ee;
}

.ppt-slide-thumbnail-card:focus-visible {
  outline: 2px solid #22d3ee;
  outline-offset: 2px;
}

.ppt-slide-thumbnail-card:hover {
  transform: translateY(-1px);
}

.ppt-slide-number {
  position: absolute;
  top: 11px;
  left: 11px;
  z-index: 2;
  min-width: 20px;
  padding: 2px 5px;
  border-radius: 5px;
  color: #d4d4d8;
  background: rgba(9, 9, 9, 0.74);
  font-size: 11px;
  font-weight: 780;
}

.ppt-slide-thumb-stage {
  display: grid;
  align-content: start;
  gap: 5px;
  aspect-ratio: 16 / 9;
  overflow: hidden;
  padding: 18px 10px 10px;
  border-radius: 7px;
  color: #0f172a;
  background: linear-gradient(135deg, #f8fafc, #dbeafe);
}

.ppt-slide-thumb-stage strong {
  display: -webkit-box;
  overflow: hidden;
  color: inherit;
  font-size: clamp(9px, calc(var(--ppt-sidebar-width, 150px) * 0.07), 15px);
  line-height: 1.08;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.ppt-slide-thumb-stage p {
  display: -webkit-box;
  overflow: hidden;
  margin: 0;
  color: rgba(15, 23, 42, 0.68);
  font-size: 9px;
  line-height: 1.35;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.ppt-slide-thumb-stage img {
  position: absolute;
  right: 8px;
  bottom: 8px;
  width: 42%;
  height: 34%;
  object-fit: cover;
  border-radius: 6px;
  box-shadow: 0 8px 18px rgba(15, 23, 42, 0.22);
}

.ppt-slide-thumb-stage.has-image p {
  max-width: 56%;
}

.ppt-slide-sidebar-resize {
  position: absolute;
  top: 0;
  right: -8px;
  bottom: 0;
  z-index: 8;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 14px;
  cursor: col-resize;
}

.ppt-slide-sidebar-resize span {
  position: relative;
  width: 4px;
  height: 46px;
  border-radius: 999px;
  background: #2b2b2b;
  opacity: 0.6;
}

.ppt-slide-sidebar-resize span::before {
  content: "";
  position: absolute;
  top: 50%;
  left: 50%;
  width: 10px;
  height: 18px;
  border-radius: 999px;
  background:
    radial-gradient(circle, #71717a 1px, transparent 1.5px) 0 0 / 5px 6px;
  opacity: 0;
  transform: translate(-50%, -50%);
  transition: opacity 0.16s ease;
}

.ppt-slide-sidebar-resize:hover span {
  background: #22d3ee;
  opacity: 1;
}

.ppt-slide-sidebar-resize:focus-visible {
  outline: 2px solid #22d3ee;
  outline-offset: 2px;
}

.ppt-slide-sidebar-resize:focus-visible span {
  background: #22d3ee;
  opacity: 1;
}

.ppt-slide-sidebar-resize:hover span::before {
  opacity: 1;
}

.ppt-slide-sidebar-resize:focus-visible span::before {
  opacity: 1;
}

.ppt-presentation-stage {
  position: relative;
  z-index: 9;
  min-width: 0;
  border: 1px solid #202020;
  border-radius: 10px;
  background: radial-gradient(circle at top, #121212 0, #080808 44%);
  overflow: hidden;
}

.ppt-slide-stack {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 26px;
  max-height: calc(100vh - 300px);
  overflow: auto;
  padding: 30px 18px 56px;
}

.ppt-edit-slide,
.ppt-present-canvas {
  position: relative;
  box-sizing: border-box;
  aspect-ratio: 16 / 9;
  padding: clamp(28px, 4vw, 56px);
  border: 1px solid transparent;
  border-radius: 12px;
  color: #0f172a;
  background: linear-gradient(135deg, #f8fafc, #dbeafe);
  box-shadow: 0 22px 70px rgba(0, 0, 0, 0.3);
  text-align: var(--ppt-global-align, left);
  overflow: hidden;
  transform-origin: top center;
}

.ppt-edit-slide {
  flex: 0 0 auto;
  width: min(var(--ppt-slide-width, 880px), 100%);
  cursor: pointer;
  overflow: visible;
}

.ppt-present-canvas {
  width: min(var(--ppt-slide-base-width, 880px), 100%);
  transform: scale(var(--ppt-zoom));
}

.ppt-edit-slide.active {
  border-color: #22d3ee;
  box-shadow: 0 0 0 2px rgba(34, 211, 238, 0.45), 0 0 0 7px rgba(34, 211, 238, 0.14), 0 22px 70px rgba(0, 0, 0, 0.34);
}

.ppt-edit-slide span,
.ppt-present-canvas span {
  color: #2563eb;
  font-weight: 820;
}

.ppt-edit-slide h2,
.ppt-present-canvas h2 {
  margin: 18px 0 12px;
  font-size: var(--ppt-title-size, clamp(28px, 4vw, 48px));
  line-height: 1.14;
  word-break: break-word;
}

.ppt-edit-slide p,
.ppt-present-canvas p {
  max-width: 760px;
  font-size: var(--ppt-body-size, 15px);
  line-height: 1.75;
  word-break: break-word;
}

.ppt-edit-slide li,
.ppt-present-canvas li {
  margin: 8px 0;
  font-size: var(--ppt-body-size, 15px);
}

.ppt-edit-slide em {
  display: block;
  margin-top: 18px;
  color: rgba(15, 23, 42, 0.58);
  font-size: 13px;
  font-style: normal;
}

.ppt-edit-slide.has-image p,
.ppt-edit-slide.has-image ul,
.ppt-edit-slide.has-image em,
.ppt-present-canvas.has-image p,
.ppt-present-canvas.has-image ul {
  max-width: 55%;
}

.ppt-slide-image-frame {
  position: absolute;
  right: clamp(26px, 4vw, 56px);
  bottom: clamp(26px, 4vw, 54px);
  width: min(36%, 320px);
  aspect-ratio: 16 / 10;
  margin: 0;
  overflow: hidden;
  border: 1px solid rgba(255, 255, 255, 0.48);
  border-radius: 18px;
  background: rgba(255, 255, 255, 0.18);
  box-shadow: 0 22px 56px rgba(15, 23, 42, 0.28);
}

.ppt-slide-image-frame img {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.ppt-slide-element {
  position: relative;
  border: 1px solid transparent;
  transition: border-color 0.14s ease, box-shadow 0.14s ease, background-color 0.14s ease;
}

.ppt-slide-element:hover,
.ppt-slide-element.selected {
  border-color: rgba(37, 99, 235, 0.42);
  box-shadow: 0 0 0 4px rgba(37, 99, 235, 0.1);
}

.ppt-slide-element:focus-visible {
  outline: 2px solid #22d3ee;
  outline-offset: 4px;
}

.ppt-slide-element.selected {
  background: rgba(255, 255, 255, 0.2);
}

.ppt-element-floating-toolbar {
  position: absolute;
  top: 12px;
  right: 14px;
  z-index: 11;
  display: flex;
  align-items: center;
  gap: 6px;
  max-width: min(720px, calc(100% - 28px));
  padding: 6px;
  border: 1px solid rgba(255, 255, 255, 0.2);
  border-radius: 10px;
  color: #f8fafc;
  background: rgba(10, 10, 10, 0.78);
  box-shadow: 0 18px 52px rgba(0, 0, 0, 0.32);
  backdrop-filter: blur(14px);
}

.ppt-element-floating-toolbar button,
.ppt-element-toolbar-chip {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  min-width: 32px;
  min-height: 32px;
  padding: 0 8px;
  border: 1px solid rgba(255, 255, 255, 0.14);
  border-radius: 8px;
  color: #f8fafc;
  background: rgba(255, 255, 255, 0.07);
  cursor: pointer;
  font-weight: 760;
  white-space: nowrap;
}

.ppt-element-floating-toolbar button:hover,
.ppt-element-floating-toolbar button.active,
.ppt-element-toolbar-chip:hover {
  border-color: rgba(34, 211, 238, 0.42);
  background: rgba(255, 255, 255, 0.13);
}

.ppt-element-floating-toolbar button:focus-visible,
.ppt-element-toolbar-chip:focus-visible,
.ppt-element-ai-input-wrap textarea:focus-visible,
.ppt-element-ai-suggestions button:focus-visible {
  outline: 2px solid #22d3ee;
  outline-offset: 2px;
}

.ppt-element-floating-toolbar button.danger {
  color: #fecaca;
}

.ppt-element-ai-editor {
  position: absolute;
  top: calc(100% + 10px);
  right: 0;
  z-index: 28;
  display: grid;
  gap: 10px;
  width: min(330px, calc(100vw - 44px));
  padding: 10px;
  border: 1px solid rgba(255, 255, 255, 0.16);
  border-radius: 14px;
  color: #f8fafc;
  background: rgba(13, 13, 13, 0.96);
  box-shadow: 0 22px 72px rgba(0, 0, 0, 0.5);
  backdrop-filter: blur(18px);
}

.ppt-element-ai-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.ppt-element-ai-head > div {
  display: grid;
  gap: 2px;
}

.ppt-element-ai-head strong {
  color: #ffffff;
  font-size: 13px;
  font-weight: 820;
}

.ppt-element-ai-head small {
  color: #a1a1aa;
  font-size: 11px;
}

.ppt-element-ai-head button {
  min-width: 28px;
  min-height: 28px;
  padding: 0;
}

.ppt-element-ai-input-wrap {
  overflow: hidden;
  border: 1px solid rgba(255, 255, 255, 0.13);
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.05);
}

.ppt-element-ai-input-wrap:focus-within {
  border-color: rgba(34, 211, 238, 0.56);
  box-shadow: 0 0 0 3px rgba(34, 211, 238, 0.12);
}

.ppt-element-ai-input-wrap textarea {
  width: 100%;
  min-height: 78px;
  max-height: 150px;
  padding: 10px 11px;
  resize: vertical;
  border: 0;
  outline: 0;
  color: #f8fafc;
  caret-color: #22d3ee;
  background: transparent;
  font-size: 13px;
  line-height: 1.55;
}

.ppt-element-ai-input-wrap textarea::placeholder {
  color: #71717a;
}

.ppt-element-ai-input-wrap textarea:disabled {
  cursor: not-allowed;
  opacity: 0.68;
}

.ppt-element-ai-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 7px 8px;
  border-top: 1px solid rgba(255, 255, 255, 0.1);
  background: rgba(255, 255, 255, 0.04);
}

.ppt-element-ai-footer > span {
  color: #a1a1aa;
  font-size: 10px;
}

.ppt-element-ai-send {
  width: 30px;
  min-width: 30px;
  height: 30px;
  min-height: 30px;
  padding: 0;
  border-color: rgba(34, 211, 238, 0.4);
  color: #020617;
  background: #22d3ee;
}

.ppt-element-ai-send:hover {
  background: #67e8f9;
}

.ppt-element-ai-send:disabled {
  cursor: not-allowed;
  color: #94a3b8;
  background: rgba(255, 255, 255, 0.08);
  border-color: rgba(255, 255, 255, 0.12);
}

.ppt-element-ai-state,
.ppt-element-ai-error {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 34px;
  padding: 8px 9px;
  border-radius: 10px;
  font-size: 12px;
  font-weight: 720;
}

.ppt-element-ai-state {
  justify-content: space-between;
  color: #e0f2fe;
  background: rgba(14, 165, 233, 0.14);
}

.ppt-element-ai-state button {
  min-height: 24px;
  padding: 0 8px;
  color: #bae6fd;
}

.ppt-element-ai-error {
  color: #fecaca;
  background: rgba(239, 68, 68, 0.14);
}

.ppt-element-ai-suggestions {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 6px;
}

.ppt-element-ai-suggestions button {
  justify-content: flex-start;
  min-height: 36px;
  padding: 0 9px;
  text-align: left;
}

.ppt-element-ai-suggestions span {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border-radius: 6px;
  color: #e0f2fe;
  background: rgba(34, 211, 238, 0.16);
  font-size: 10px;
  font-weight: 860;
}

.ppt-element-ai-suggestions small {
  overflow: hidden;
  color: #e5e7eb;
  font-size: 11px;
  font-weight: 760;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ppt-element-type-menu {
  position: relative;
}

.ppt-element-type-popover {
  position: absolute;
  top: calc(100% + 8px);
  left: 0;
  z-index: 26;
  display: grid;
  gap: 4px;
  width: 210px;
  padding: 8px;
  border: 1px solid #2b2b2b;
  border-radius: 10px;
  background: #101010;
  box-shadow: 0 18px 60px rgba(0, 0, 0, 0.5);
}

.ppt-element-type-popover button {
  display: grid;
  grid-template-columns: 1fr;
  justify-items: start;
  gap: 2px;
  width: 100%;
  min-height: 46px;
  padding: 7px 9px;
  text-align: left;
}

.ppt-element-type-popover small {
  color: #9ca3af;
  font-size: 11px;
  font-weight: 500;
}

.ppt-element-toolbar-group,
.ppt-element-color-row {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding-left: 4px;
  border-left: 1px solid rgba(255, 255, 255, 0.12);
}

.ppt-element-color-row button {
  width: 24px;
  min-width: 24px;
  height: 24px;
  min-height: 24px;
  padding: 0;
  border-radius: 999px;
  background: var(--element-color);
  box-shadow: inset 0 0 0 2px rgba(255, 255, 255, 0.3);
}

.ppt-element-color-row button.active {
  box-shadow: 0 0 0 2px #22d3ee, inset 0 0 0 2px rgba(255, 255, 255, 0.36);
}

.ppt-slide-floating-tools {
  position: absolute;
  top: 12px;
  right: 14px;
  z-index: 16;
  display: inline-flex;
  gap: 6px;
  opacity: 0;
  transform: translateY(-4px);
  transition: opacity 0.16s ease, transform 0.16s ease;
}

.ppt-edit-slide:hover .ppt-slide-floating-tools,
.ppt-edit-slide.active .ppt-slide-floating-tools {
  opacity: 1;
  transform: translateY(0);
}

.ppt-slide-floating-tools button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  border: 1px solid rgba(255, 255, 255, 0.26);
  border-radius: 999px;
  color: #f8fafc;
  background: rgba(15, 23, 42, 0.58);
  box-shadow: 0 10px 28px rgba(15, 23, 42, 0.22);
  cursor: pointer;
  backdrop-filter: blur(12px);
}

.ppt-slide-floating-tools button:hover {
  background: rgba(15, 23, 42, 0.78);
}

.ppt-slide-floating-tools button:focus-visible,
.ppt-slide-insert-button:focus-visible,
.ppt-slide-more-menu button:focus-visible,
.ppt-slide-palette-menu button:focus-visible,
.ppt-slide-magic-menu button:focus-visible {
  outline: 2px solid #22d3ee;
  outline-offset: 2px;
}

.ppt-slide-insert-button {
  position: absolute;
  left: 50%;
  z-index: 7;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  min-height: 32px;
  padding: 0;
  border: 1px solid rgba(228, 228, 231, 0.86);
  border-radius: 999px;
  color: #18181b;
  background: rgba(255, 255, 255, 0.94);
  box-shadow: 0 12px 34px rgba(0, 0, 0, 0.24);
  opacity: 0;
  transform: translateX(-50%) scale(0.92);
  transition: opacity 0.16s ease, transform 0.16s ease, border-color 0.16s ease;
}

.ppt-slide-insert-button.is-before {
  top: -16px;
}

.ppt-slide-insert-button.is-after {
  bottom: -16px;
}

.ppt-edit-slide:hover .ppt-slide-insert-button,
.ppt-edit-slide.active .ppt-slide-insert-button {
  opacity: 1;
  transform: translateX(-50%) scale(1);
}

.ppt-slide-insert-button:hover {
  border-color: #22d3ee;
  color: #0891b2;
}

.ppt-slide-insert-button:disabled {
  opacity: 0.46;
  cursor: not-allowed;
  transform: translateX(-50%) scale(0.92);
}

.ppt-edit-slide:hover .ppt-slide-insert-button:disabled,
.ppt-edit-slide.active .ppt-slide-insert-button:disabled {
  transform: translateX(-50%) scale(0.92);
}

.ppt-slide-more,
.ppt-slide-palette,
.ppt-slide-magic {
  position: relative;
}

.ppt-slide-more {
  order: 1;
}

.ppt-slide-magic {
  order: 2;
}

.ppt-slide-palette {
  order: 3;
}

.ppt-slide-more-menu {
  position: absolute;
  top: calc(100% + 8px);
  right: 0;
  z-index: 20;
  display: grid;
  gap: 4px;
  min-width: 158px;
  padding: 7px;
  border: 1px solid #2b2b2b;
  border-radius: 9px;
  background: #101010;
  box-shadow: 0 18px 60px rgba(0, 0, 0, 0.48);
}

.ppt-slide-more-menu button {
  display: grid;
  grid-template-columns: 16px minmax(0, 1fr);
  gap: 9px;
  width: 100%;
  height: 34px;
  padding: 0 8px;
  border: 0;
  border-radius: 7px;
  color: #f4f4f5;
  background: transparent;
  box-shadow: none;
  text-align: left;
}

.ppt-slide-more-menu button:hover:not(:disabled) {
  background: #1d1d1d;
}

.ppt-slide-more-menu button.danger {
  color: #fecaca;
}

.ppt-slide-more-menu button span {
  color: inherit;
}

.ppt-slide-palette-menu {
  position: absolute;
  top: calc(100% + 8px);
  right: 0;
  z-index: 22;
  display: grid;
  gap: 12px;
  width: min(360px, 84vw);
  padding: 14px;
  border: 1px solid rgba(255, 255, 255, 0.14);
  border-radius: 14px;
  color: #f8fafc;
  background: rgba(10, 10, 10, 0.96);
  box-shadow: 0 22px 70px rgba(0, 0, 0, 0.55);
  backdrop-filter: blur(18px);
}

.ppt-slide-palette-menu section {
  display: grid;
  gap: 8px;
}

.ppt-slide-palette-menu h4 {
  margin: 0;
  color: #a1a1aa;
  font-size: 12px;
  font-weight: 820;
  letter-spacing: 0;
}

.ppt-slide-palette-layouts {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 7px;
  padding-bottom: 4px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.1);
}

.ppt-slide-palette-layouts button {
  width: 100%;
  height: 42px;
  border-radius: 10px;
  background: rgba(255, 255, 255, 0.07);
  box-shadow: none;
}

.ppt-slide-palette-layouts button.active,
.ppt-slide-palette-layouts button:hover {
  border-color: rgba(34, 211, 238, 0.58);
  background: rgba(34, 211, 238, 0.13);
}

.ppt-slide-palette-layouts button > span {
  display: grid;
  width: 34px;
  height: 22px;
  border: 1px solid rgba(255, 255, 255, 0.42);
  border-radius: 5px;
  background: rgba(255, 255, 255, 0.1);
}

.ppt-slide-palette-layouts [data-layout="cover"] {
  background: linear-gradient(#f8fafc 0 42%, transparent 42%);
}

.ppt-slide-palette-layouts [data-layout="section"] {
  background: linear-gradient(90deg, #f8fafc 0 36%, transparent 36%);
}

.ppt-slide-palette-layouts [data-layout="content"] {
  background: repeating-linear-gradient(0deg, #f8fafc 0 2px, transparent 2px 6px);
}

.ppt-slide-palette-layouts [data-layout="imageText"] {
  background: linear-gradient(90deg, transparent 0 48%, #f8fafc 48% 100%);
}

.ppt-slide-palette-layouts [data-layout="summary"] {
  background: radial-gradient(circle at 50% 50%, #f8fafc 0 26%, transparent 28%);
}

.ppt-slide-palette-colors {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

.ppt-slide-palette-colors button {
  width: 28px;
  min-width: 28px;
  height: 28px;
  min-height: 28px;
  padding: 0;
  border-radius: 999px;
  background: var(--palette-bg);
  box-shadow: inset 0 0 0 2px rgba(255, 255, 255, 0.3);
}

.ppt-slide-palette-colors button.active {
  border-color: #22d3ee;
  box-shadow: 0 0 0 2px rgba(34, 211, 238, 0.48), inset 0 0 0 2px rgba(255, 255, 255, 0.32);
}

.ppt-slide-palette-colors .is-reset {
  color: #e5e7eb;
  background: rgba(255, 255, 255, 0.06);
  font-size: 16px;
  font-weight: 760;
}

.ppt-slide-palette-segment {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 6px;
}

.ppt-slide-palette-segment button {
  width: 100%;
  height: 34px;
  border-radius: 9px;
  color: #e5e7eb;
  background: rgba(255, 255, 255, 0.07);
  box-shadow: none;
  font-size: 12px;
  font-weight: 760;
}

.ppt-slide-palette-segment button.active,
.ppt-slide-palette-segment button:hover {
  border-color: rgba(34, 211, 238, 0.5);
  color: #ffffff;
  background: rgba(34, 211, 238, 0.14);
}

.ppt-slide-palette-image-row {
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: 12px;
  padding-top: 10px;
  border-top: 1px solid rgba(255, 255, 255, 0.1);
}

.ppt-slide-palette-image-row div {
  display: grid;
  gap: 2px;
}

.ppt-slide-palette-image-row strong {
  color: #f8fafc;
  font-size: 13px;
}

.ppt-slide-palette-image-row span {
  color: #a1a1aa;
  font-size: 11px;
}

.ppt-slide-palette-image-row button {
  width: auto;
  min-width: 68px;
  height: 32px;
  padding: 0 12px;
  border-radius: 999px;
  color: #020617;
  background: #f8fafc;
  box-shadow: none;
  font-weight: 820;
}

.ppt-slide-palette-image-row button:hover {
  color: #020617;
  background: #67e8f9;
}

.ppt-slide-magic-menu {
  position: absolute;
  top: calc(100% + 8px);
  right: 0;
  z-index: 22;
  display: grid;
  gap: 7px;
  width: min(340px, 82vw);
  padding: 12px;
  border: 1px solid rgba(255, 255, 255, 0.13);
  border-radius: 12px;
  color: #f8fafc;
  background: #0a0a0a;
  box-shadow: 0 22px 70px rgba(0, 0, 0, 0.55);
}

.ppt-slide-magic-menu header {
  display: grid;
  gap: 3px;
  padding-bottom: 7px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.1);
}

.ppt-slide-magic-menu header strong {
  color: #f8fafc;
  font-size: 14px;
}

.ppt-slide-magic-menu header span {
  color: #a1a1aa;
  font-size: 12px;
}

.ppt-slide-magic-menu label {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 34px;
  gap: 7px;
  align-items: center;
  margin: 2px 0 4px;
  padding: 7px;
  border: 1px solid rgba(255, 255, 255, 0.14);
  border-radius: 9px;
  background: rgba(255, 255, 255, 0.06);
}

.ppt-slide-magic-menu label:focus-within {
  border-color: rgba(34, 211, 238, 0.62);
  box-shadow: 0 0 0 2px rgba(34, 211, 238, 0.14);
}

.ppt-slide-magic-menu input {
  min-width: 0;
  border: 0;
  color: #ffffff;
  background: transparent;
  outline: 0;
}

.ppt-slide-magic-menu input::placeholder {
  color: #71717a;
}

.ppt-slide-magic-menu label button {
  width: 34px;
  height: 34px;
  min-height: 34px;
  border: 0;
  color: #09090b;
  background: #f8fafc;
  box-shadow: none;
}

.ppt-slide-magic-menu label button:disabled {
  color: #52525b;
  background: rgba(255, 255, 255, 0.1);
  cursor: not-allowed;
}

.ppt-slide-magic-busy,
.ppt-slide-magic-error {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 34px;
  padding: 8px 9px;
  border-radius: 10px;
  font-size: 12px;
  font-weight: 720;
}

.ppt-slide-magic-busy {
  color: #e0f2fe;
  background: rgba(14, 165, 233, 0.14);
}

.ppt-slide-magic-error {
  color: #fecaca;
  background: rgba(239, 68, 68, 0.14);
}

.ppt-slide-magic-primary {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  min-height: 42px;
  padding: 0 12px;
  border: 1px solid rgba(34, 211, 238, 0.34);
  border-radius: 10px;
  color: #f0f9ff;
  background: rgba(34, 211, 238, 0.12);
  box-shadow: none;
}

.ppt-slide-magic-primary > span {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-weight: 820;
}

.ppt-slide-magic-primary:hover:not(:disabled) {
  border-color: rgba(34, 211, 238, 0.58);
  color: #ffffff;
  background: rgba(34, 211, 238, 0.18);
}

.ppt-slide-magic-section {
  display: grid;
  gap: 8px;
}

.ppt-slide-magic-section h4 {
  margin: 0;
  padding: 0 2px;
  color: #71717a;
  font-size: 11px;
  font-weight: 820;
  letter-spacing: 0;
}

.ppt-slide-magic-actions {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 7px;
}

.ppt-slide-magic-actions button {
  display: flex;
  align-items: center;
  justify-content: flex-start;
  gap: 8px;
  width: 100%;
  min-width: 0;
  min-height: 42px;
  height: auto;
  padding: 8px 10px;
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 10px;
  color: #f4f4f5;
  background: rgba(255, 255, 255, 0.07);
  box-shadow: none;
}

.ppt-slide-magic-actions button:hover:not(:disabled) {
  border-color: rgba(34, 211, 238, 0.42);
  color: #ffffff;
  background: rgba(255, 255, 255, 0.12);
}

.ppt-slide-magic-actions button:disabled,
.ppt-slide-magic-primary:disabled {
  cursor: not-allowed;
  opacity: 0.56;
}

.ppt-slide-magic-actions button span {
  min-width: 0;
  overflow: hidden;
  font-size: 12px;
  font-weight: 760;
  line-height: 1.25;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ppt-slide-more-menu button:disabled {
  color: #666;
  cursor: not-allowed;
}

.ppt-export-dialog-overlay {
  position: fixed;
  inset: 0;
  z-index: 1200;
  display: grid;
  place-items: center;
  padding: 24px;
  background: rgba(0, 0, 0, 0.58);
}

.ppt-export-dialog {
  display: grid;
  gap: 18px;
  width: min(460px, 100%);
  padding: 20px;
  border: 1px solid #2b2b2b;
  border-radius: 12px;
  background: #101010;
  box-shadow: 0 26px 88px rgba(0, 0, 0, 0.56);
}

.ppt-export-dialog header {
  display: grid;
  gap: 7px;
}

.ppt-export-dialog h2 {
  margin: 0;
  color: #f8fafc;
  font-size: 18px;
}

.ppt-export-dialog p {
  margin: 0;
  color: #a1a1aa;
  line-height: 1.6;
  font-size: 13px;
}

.ppt-export-options {
  display: grid;
  gap: 10px;
}

.ppt-export-option {
  display: grid;
  grid-template-columns: 42px minmax(0, 1fr);
  align-items: center;
  gap: 12px;
  min-height: 78px;
  padding: 12px;
  border: 1px solid #2b2b2b;
  border-radius: 10px;
  color: #f4f4f5;
  background: #0b0b0b;
  cursor: default;
  text-align: left;
}

.ppt-export-option.active {
  border-color: #f4f4f5;
  background: #181818;
}

.ppt-dialog-secondary:focus-visible,
.ppt-dialog-primary:focus-visible,
.ppt-export-download-link:focus-visible {
  outline: 2px solid #22d3ee;
  outline-offset: 2px;
}

.ppt-export-option > .ppt-toolbar-icon {
  width: 28px;
  height: 28px;
}

.ppt-export-options span {
  display: grid;
  gap: 4px;
}

.ppt-export-options b {
  font-size: 14px;
}

.ppt-export-options small {
  color: #9ca3af;
  font-size: 12px;
}

.ppt-export-progress {
  display: grid;
  gap: 10px;
  padding: 12px;
  border: 1px solid #262626;
  border-radius: 10px;
  background: #0b0b0b;
}

.ppt-export-progress > div {
  display: grid;
  grid-template-columns: 28px minmax(0, 1fr);
  gap: 10px;
  align-items: start;
}

.ppt-export-progress i {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border: 1px solid #333;
  border-radius: 50%;
  color: #85858c;
  background: #151515;
  font-style: normal;
}

.ppt-export-progress i b {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
}

.ppt-export-progress > div.active i {
  border-color: rgba(56, 189, 248, 0.7);
  color: #38bdf8;
  background: rgba(14, 165, 233, 0.12);
}

.ppt-export-progress > div.done i {
  border-color: rgba(134, 239, 172, 0.6);
  color: #86efac;
  background: rgba(34, 197, 94, 0.1);
}

.ppt-export-progress span {
  display: grid;
  gap: 3px;
}

.ppt-export-progress strong {
  color: #f4f4f5;
  font-size: 13px;
}

.ppt-export-progress small {
  color: #9ca3af;
  font-size: 11px;
  line-height: 1.45;
}

.ppt-export-download-link {
  color: #bfdbfe;
  font-size: 12px;
  text-decoration: underline;
  text-underline-offset: 3px;
}

.ppt-export-dialog footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}

.ppt-dialog-secondary,
.ppt-dialog-primary {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  min-height: 38px;
  padding: 0 14px;
  border-radius: 8px;
  cursor: pointer;
  font-weight: 780;
}

.ppt-dialog-secondary {
  border: 1px solid #333;
  color: #f4f4f5;
  background: #151515;
}

.ppt-dialog-primary {
  border: 0;
  color: #111;
  background: #f4f4f5;
}

.ppt-dialog-secondary:disabled,
.ppt-dialog-primary:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}

.ppt-share-dialog,
.ppt-help-dialog {
  display: grid;
  gap: 18px;
  width: min(480px, 100%);
  padding: 20px;
  border: 1px solid #2b2b2b;
  border-radius: 12px;
  background: #101010;
  box-shadow: 0 26px 88px rgba(0, 0, 0, 0.56);
}

.ppt-share-dialog header,
.ppt-help-dialog header {
  display: grid;
  gap: 7px;
}

.ppt-share-dialog h2,
.ppt-help-dialog h2 {
  margin: 0;
  color: #f8fafc;
  font-size: 18px;
}

.ppt-share-dialog p,
.ppt-help-dialog p {
  margin: 0;
  color: #a1a1aa;
  line-height: 1.6;
  font-size: 13px;
}

.ppt-share-link-card {
  display: grid;
  gap: 8px;
}

.ppt-share-link-card > span {
  color: #d4d4d8;
  font-size: 12px;
  font-weight: 780;
}

.ppt-share-link-card > div {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 8px;
}

.ppt-share-link-card input {
  min-width: 0;
  height: 38px;
  padding: 0 10px;
  border: 1px solid #333;
  border-radius: 8px;
  color: #f8fafc;
  background: #0b0b0b;
  outline: 0;
}

.ppt-share-link-card button {
  min-height: 38px;
  padding: 0 12px;
  border: 1px solid #f4f4f5;
  border-radius: 8px;
  color: #111;
  background: #f4f4f5;
  cursor: pointer;
  font-weight: 780;
}

.ppt-share-permission {
  display: grid;
  grid-template-columns: 28px minmax(0, 1fr);
  gap: 10px;
  align-items: start;
  padding: 12px;
  border: 1px solid rgba(134, 239, 172, 0.24);
  border-radius: 10px;
  color: #86efac;
  background: rgba(34, 197, 94, 0.08);
}

.ppt-share-permission span {
  display: grid;
  gap: 4px;
  color: #f4f4f5;
}

.ppt-share-permission small {
  color: #a1a1aa;
  line-height: 1.5;
}

.ppt-share-dialog footer,
.ppt-help-dialog footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}

.ppt-shortcut-list {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 10px 16px;
  align-items: center;
  padding: 12px;
  border: 1px solid #262626;
  border-radius: 10px;
  background: #0b0b0b;
}

.ppt-shortcut-list span {
  color: #d4d4d8;
  font-size: 13px;
}

.ppt-shortcut-list kbd {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 26px;
  padding: 0 8px;
  border: 1px solid #3a3a3a;
  border-radius: 7px;
  color: #f8fafc;
  background: #171717;
  font-family: inherit;
  font-size: 12px;
  font-weight: 780;
  white-space: nowrap;
}

.theme-blackGold.ppt-edit-slide,
.theme-blackGold.ppt-present-canvas {
  color: #f8fafc;
  background: linear-gradient(135deg, #050505, #78350f);
}

.theme-techBlue.ppt-edit-slide,
.theme-techBlue.ppt-present-canvas {
  color: #eff6ff;
  background: linear-gradient(135deg, #08111f, #1d4ed8);
}

.ppt-right-edit-panel {
  position: relative;
  z-index: 10;
  display: flex;
  flex-direction: column;
  gap: 10px;
  max-height: calc(100vh - 250px);
  overflow: hidden;
  padding: 12px;
}

.ppt-right-panel-shell {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  min-height: 0;
  overflow: hidden;
  border: 1px solid #242424;
  border-radius: 12px;
  background: rgba(12, 12, 12, 0.94);
}

.ppt-right-edit-panel.is-panel-closed .ppt-right-panel-shell {
  width: 58px;
  grid-template-columns: 54px;
}

.ppt-right-tool-rail {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 8px 6px;
  border-right: 1px solid #242424;
  background: #090909;
}

.ppt-right-tool-button {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  min-height: 40px;
  padding: 0;
  border: 0;
  border-radius: 10px;
  color: #a1a1aa;
  background: transparent;
  cursor: pointer;
}

.ppt-right-tool-button.is-group-start {
  margin-top: 10px;
}

.ppt-right-tool-button.is-group-start::before {
  content: "";
  position: absolute;
  top: -6px;
  left: 8px;
  right: 8px;
  height: 1px;
  background: #2a2a2a;
  pointer-events: none;
}

.ppt-right-tool-button span {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
}

.ppt-right-tool-button:hover,
.ppt-right-tool-button.active {
  color: #f4f4f5;
  background: #2a2a2a;
}

.ppt-right-tool-button:disabled {
  color: #555;
  cursor: not-allowed;
  background: transparent;
}

.ppt-right-tool-button.active {
  box-shadow: inset 3px 0 0 #22d3ee;
}

.ppt-right-panel-content {
  display: grid;
  align-content: start;
  gap: 14px;
  min-width: 0;
  min-height: 0;
  overflow: auto;
  padding: 12px;
}

.ppt-right-panel-content.is-closed {
  align-content: center;
  place-items: center;
  min-height: 360px;
  overflow: hidden;
}

.ppt-right-panel-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 10px;
  padding-bottom: 10px;
  border-bottom: 1px solid #242424;
}

.ppt-right-panel-title {
  display: flex;
  min-width: 0;
  gap: 9px;
}

.ppt-right-panel-title > div {
  display: grid;
  gap: 3px;
}

.ppt-right-panel-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 28px;
  width: 28px;
  height: 28px;
  border: 1px solid #303030;
  border-radius: 9px;
  color: #22d3ee;
  background: #111;
}

.ppt-right-panel-header strong {
  color: #f4f4f5;
  font-size: 14px;
}

.ppt-right-panel-header span {
  color: #8b8b94;
  font-size: 12px;
  line-height: 1.45;
}

.ppt-right-panel-header button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  min-height: 28px;
  border: 1px solid #333;
  border-radius: 50%;
  color: #f4f4f5;
  background: #151515;
}

.ppt-right-panel-body {
  display: grid;
  gap: 14px;
  min-width: 0;
}

.ppt-right-panel-body.is-self-contained {
  gap: 0;
}

.ppt-panel-loading-state {
  display: grid;
  justify-items: center;
  align-content: center;
  gap: 10px;
  min-height: 220px;
  border: 1px solid #242424;
  border-radius: 12px;
  color: #a1a1aa;
  background: #0b0b0b;
}

.ppt-panel-loading-state strong {
  color: #e4e4e7;
  font-size: 13px;
}

.ppt-panel-section {
  display: grid;
  gap: 12px;
}

.ppt-panel-search-stack {
  display: grid;
  gap: 10px;
  padding-bottom: 2px;
}

.ppt-panel-search {
  position: relative;
  display: grid;
  align-items: center;
}

.ppt-panel-search > .ppt-toolbar-icon {
  position: absolute;
  left: 10px;
  z-index: 1;
  color: #8b8b94;
  pointer-events: none;
}

.ppt-panel-search input {
  width: 100%;
  min-height: 38px;
  padding: 0 38px 0 36px;
  border: 1px solid #2b2b2b;
  border-radius: 9px;
  color: #f4f4f5;
  caret-color: #ffffff;
  background: #0d0d0d;
  outline: none;
}

.ppt-panel-search input:focus {
  border-color: #3f3f46;
  box-shadow: 0 0 0 3px rgba(34, 211, 238, 0.12);
}

.ppt-panel-search button {
  position: absolute;
  right: 5px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  min-height: 28px;
  border: 0;
  border-radius: 50%;
  color: #a1a1aa;
  background: transparent;
  cursor: pointer;
}

.ppt-panel-search button:hover {
  color: #f4f4f5;
  background: #1f1f1f;
}

.ppt-panel-filter-row {
  display: flex;
  gap: 6px;
  overflow-x: auto;
  padding-bottom: 2px;
}

.ppt-panel-filter-row button {
  flex: 0 0 auto;
  min-height: 30px;
  padding: 0 10px;
  border: 1px solid #2b2b2b;
  border-radius: 999px;
  color: #a1a1aa;
  background: #101010;
  cursor: pointer;
  font-size: 12px;
  font-weight: 760;
}

.ppt-panel-filter-row button:hover,
.ppt-panel-filter-row button.active {
  color: #f4f4f5;
  border-color: #3f3f46;
  background: #262626;
}

.ppt-insert-grid {
  display: grid;
  gap: 10px;
}

.ppt-insert-grid.is-two-column {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.ppt-insert-grid.is-basic-blocks {
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 18px 10px;
  padding-top: 4px;
}

.ppt-insert-card {
  position: relative;
  display: grid;
  align-content: start;
  gap: 7px;
  min-height: 138px;
  padding: 10px;
  border: 1px solid #2b2b2b;
  border-radius: 12px;
  color: #f4f4f5;
  text-align: left;
  background: #111;
  cursor: pointer;
  transition: border-color 0.18s ease, background 0.18s ease, transform 0.18s ease;
}

.ppt-insert-card.is-basic-block {
  justify-items: center;
  gap: 8px;
  min-height: auto;
  padding: 0;
  border: 0;
  border-radius: 8px;
  text-align: center;
  background: transparent;
}

.ppt-insert-card.is-chart-card {
  gap: 9px;
  min-height: 132px;
  padding: 9px 12px 10px;
}

.ppt-insert-card.is-diagram-card {
  gap: 8px;
  min-height: 132px;
  padding: 8px;
}

.ppt-chart-card-header {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.ppt-chart-card-header .ppt-card-grip {
  position: static;
  flex: 0 0 auto;
}

.ppt-chart-card-header strong {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ppt-insert-card:hover {
  border-color: #3f3f46;
  background: #171717;
  transform: translateY(-1px);
}

.ppt-insert-card.is-basic-block:hover {
  background: transparent;
  transform: none;
}

.ppt-insert-card.active {
  border-color: rgba(34, 211, 238, 0.82);
  background: linear-gradient(135deg, rgba(14, 165, 233, 0.16), rgba(15, 23, 42, 0.52)), #111;
  box-shadow: 0 0 0 1px rgba(34, 211, 238, 0.32);
}

.ppt-insert-card.is-basic-block.active {
  background: transparent;
  box-shadow: none;
}

.ppt-insert-card.active::after {
  content: "";
  position: absolute;
  top: 9px;
  right: 9px;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #22d3ee;
  box-shadow: 0 0 0 4px rgba(34, 211, 238, 0.14);
}

.ppt-insert-card.is-basic-block.active::after {
  display: none;
}

.ppt-insert-card:focus-visible {
  outline: 2px solid #22d3ee;
  outline-offset: 2px;
}

.ppt-insert-card.is-basic-block:focus-visible {
  border-radius: 12px;
}

.ppt-card-grip {
  position: absolute;
  top: 7px;
  left: 7px;
  color: #52525b;
}

.ppt-insert-card.is-basic-block .ppt-card-grip {
  display: none;
}

.ppt-card-preview {
  display: grid;
  place-items: center;
  width: 100%;
  aspect-ratio: 16 / 9;
  margin-bottom: 2px;
  border: 1px solid #262626;
  border-radius: 8px;
  color: #d4d4d8;
  background:
    linear-gradient(90deg, rgba(255, 255, 255, 0.06) 1px, transparent 1px),
    linear-gradient(0deg, rgba(255, 255, 255, 0.05) 1px, transparent 1px),
    #151515;
  background-size: 18px 18px;
  font-size: 12px;
  font-weight: 840;
  letter-spacing: 0;
}

.ppt-insert-card.is-basic-block .ppt-card-preview {
  width: 100%;
  aspect-ratio: 1;
  margin-bottom: 0;
  border-radius: 12px;
  color: #67e8f9;
  background: #121212;
  font-size: 18px;
}

.ppt-insert-card.is-basic-block:hover .ppt-card-preview,
.ppt-insert-card.is-basic-block.active .ppt-card-preview {
  border-color: rgba(34, 211, 238, 0.55);
  background: #151515;
}

.ppt-card-preview.is-chart {
  position: relative;
  overflow: hidden;
  color: #bfdbfe;
  background:
    linear-gradient(180deg, transparent 35%, rgba(96, 165, 250, 0.22) 36%, transparent 38%),
    linear-gradient(90deg, rgba(96, 165, 250, 0.16), rgba(34, 211, 238, 0.08)),
    #111827;
  font-size: 0;
}

.ppt-card-preview.is-chart::before,
.ppt-card-preview.is-chart::after {
  content: "";
  position: absolute;
  inset: 14px;
}

.ppt-card-preview.is-chart[data-kind="pie"]::before {
  inset: 17px auto auto 50%;
  width: 46px;
  height: 46px;
  border-radius: 50%;
  background: conic-gradient(#38bdf8 0 42%, #6366f1 42% 72%, #22c55e 72% 100%);
  transform: translateX(-50%);
}

.ppt-card-preview.is-chart[data-kind="donut"]::before {
  inset: 17px auto auto 50%;
  width: 46px;
  height: 46px;
  border-radius: 50%;
  background: radial-gradient(circle, #111827 0 38%, transparent 39%), conic-gradient(#38bdf8 0 62%, #334155 62% 100%);
  transform: translateX(-50%);
}

.ppt-card-preview.is-chart[data-kind="bar"]::before,
.ppt-card-preview.is-chart[data-kind="waterfall"]::before {
  inset: auto 18px 16px;
  height: 50px;
  background: linear-gradient(to top, #38bdf8 0 42%, transparent 42%) 0 100% / 14% 100% no-repeat,
    linear-gradient(to top, #22c55e 0 68%, transparent 68%) 28% 100% / 14% 100% no-repeat,
    linear-gradient(to top, #818cf8 0 84%, transparent 84%) 56% 100% / 14% 100% no-repeat,
    linear-gradient(to top, #f59e0b 0 54%, transparent 54%) 84% 100% / 14% 100% no-repeat;
}

.ppt-card-preview.is-chart[data-kind="line"]::before,
.ppt-card-preview.is-chart[data-kind="area"]::before {
  inset: 20px 16px 16px;
  border-bottom: 2px solid rgba(148, 163, 184, 0.35);
  background: linear-gradient(135deg, transparent 0 23%, #38bdf8 24% 27%, transparent 28% 48%, #22c55e 49% 52%, transparent 53% 70%, #818cf8 71% 74%, transparent 75%);
}

.ppt-card-preview.is-chart[data-kind="area"]::after {
  inset: 40px 18px 18px;
  clip-path: polygon(0 76%, 24% 48%, 48% 58%, 72% 24%, 100% 36%, 100% 100%, 0 100%);
  background: linear-gradient(180deg, rgba(56, 189, 248, 0.48), rgba(56, 189, 248, 0.04));
}

.ppt-card-preview.is-chart[data-kind="scatter"]::before,
.ppt-card-preview.is-chart[data-kind="heatmap"]::before {
  background:
    radial-gradient(circle at 20% 68%, #38bdf8 0 3px, transparent 4px),
    radial-gradient(circle at 34% 38%, #22c55e 0 3px, transparent 4px),
    radial-gradient(circle at 56% 58%, #818cf8 0 3px, transparent 4px),
    radial-gradient(circle at 74% 30%, #f59e0b 0 3px, transparent 4px),
    radial-gradient(circle at 82% 72%, #38bdf8 0 3px, transparent 4px);
}

.ppt-card-preview.is-chart[data-kind="heatmap"]::before {
  inset: 18px 22px;
  background:
    linear-gradient(90deg, rgba(56, 189, 248, 0.24) 1px, transparent 1px),
    linear-gradient(0deg, rgba(56, 189, 248, 0.24) 1px, transparent 1px),
    linear-gradient(135deg, rgba(14, 165, 233, 0.7), rgba(34, 197, 94, 0.5), rgba(245, 158, 11, 0.68));
  background-size: 18px 18px, 18px 18px, 100% 100%;
}

.ppt-card-preview.is-chart[data-kind="funnel"]::before {
  inset: 16px 26px;
  clip-path: polygon(0 0, 100% 0, 80% 30%, 64% 30%, 54% 100%, 46% 100%, 36% 30%, 20% 30%);
  background: linear-gradient(180deg, #38bdf8, #6366f1);
}

.ppt-card-preview.is-chart[data-kind="gauge"]::before {
  inset: 20px auto auto 50%;
  width: 54px;
  height: 30px;
  border-radius: 54px 54px 0 0;
  background: conic-gradient(from 270deg, #22c55e 0 34%, #f59e0b 34% 66%, #ef4444 66% 100%);
  transform: translateX(-50%);
}

.ppt-card-preview.is-chart[data-kind="sankey"]::before {
  inset: 22px 14px;
  border-top: 10px solid rgba(56, 189, 248, 0.8);
  border-bottom: 10px solid rgba(34, 197, 94, 0.62);
  border-radius: 999px;
  transform: skewX(-18deg);
}

.ppt-card-preview.is-diagram {
  position: relative;
  overflow: hidden;
  font-size: 0;
  color: #c4b5fd;
  background:
    radial-gradient(circle at 25% 32%, rgba(196, 181, 253, 0.26) 0 14px, transparent 15px),
    radial-gradient(circle at 72% 68%, rgba(34, 211, 238, 0.2) 0 12px, transparent 13px),
    #141116;
}

.ppt-card-preview.is-diagram::before,
.ppt-card-preview.is-diagram::after {
  content: "";
  position: absolute;
  inset: 14px;
}

.ppt-card-preview.is-diagram[data-kind="flow"]::before,
.ppt-card-preview.is-diagram[data-kind="steps"]::before {
  background:
    linear-gradient(#38bdf8, #38bdf8) 8% 50% / 26px 3px no-repeat,
    linear-gradient(#38bdf8, #38bdf8) 50% 50% / 26px 3px no-repeat,
    radial-gradient(circle at 14% 50%, #38bdf8 0 10px, transparent 11px),
    radial-gradient(circle at 50% 50%, #818cf8 0 10px, transparent 11px),
    radial-gradient(circle at 86% 50%, #22c55e 0 10px, transparent 11px);
}

.ppt-card-preview.is-diagram[data-kind="timeline"]::before {
  inset: 36px 14px auto;
  height: 4px;
  border-radius: 999px;
  background: linear-gradient(90deg, #38bdf8, #818cf8, #22c55e);
}

.ppt-card-preview.is-diagram[data-kind="timeline"]::after {
  background:
    radial-gradient(circle at 18% 50%, #38bdf8 0 5px, transparent 6px),
    radial-gradient(circle at 50% 50%, #818cf8 0 5px, transparent 6px),
    radial-gradient(circle at 82% 50%, #22c55e 0 5px, transparent 6px);
}

.ppt-card-preview.is-diagram[data-kind="pyramid"]::before {
  inset: 16px 26px;
  clip-path: polygon(50% 0, 100% 100%, 0 100%);
  background: linear-gradient(180deg, #38bdf8 0 33%, #6366f1 33% 66%, #22c55e 66%);
}

.ppt-card-preview.is-diagram[data-kind="cycle"]::before {
  inset: 16px auto auto 50%;
  width: 52px;
  height: 52px;
  border: 7px solid rgba(56, 189, 248, 0.76);
  border-right-color: rgba(34, 197, 94, 0.78);
  border-bottom-color: rgba(129, 140, 248, 0.78);
  border-radius: 50%;
  transform: translateX(-50%) rotate(28deg);
}

.ppt-card-preview.is-diagram[data-kind="circles"]::before {
  background:
    radial-gradient(circle at 28% 38%, rgba(56, 189, 248, 0.8) 0 17px, transparent 18px),
    radial-gradient(circle at 60% 38%, rgba(129, 140, 248, 0.72) 0 17px, transparent 18px),
    radial-gradient(circle at 44% 68%, rgba(34, 197, 94, 0.7) 0 17px, transparent 18px);
}

.ppt-card-preview.is-diagram[data-kind="pros-cons"]::before {
  inset: 18px 18px;
  background:
    linear-gradient(#22c55e, #22c55e) 20% 50% / 28px 4px no-repeat,
    linear-gradient(90deg, #22c55e, #22c55e) 20% 50% / 4px 28px no-repeat,
    linear-gradient(#ef4444, #ef4444) 78% 50% / 30px 4px no-repeat;
}

.ppt-card-preview.is-diagram[data-kind="funnel"]::before {
  inset: 16px 28px;
  clip-path: polygon(0 0, 100% 0, 72% 42%, 58% 42%, 52% 100%, 48% 100%, 42% 42%, 28% 42%);
  background: linear-gradient(180deg, #a78bfa, #22d3ee);
}

.ppt-diagram-category-list {
  display: grid;
  gap: 10px;
}

.ppt-diagram-category {
  display: grid;
  gap: 8px;
}

.ppt-diagram-category-trigger {
  position: sticky;
  top: 0;
  z-index: 4;
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  min-height: 34px;
  padding: 0 2px 6px;
  border: 0;
  border-bottom: 1px solid #242424;
  color: #a1a1aa;
  text-align: left;
  background: rgba(13, 13, 13, 0.96);
  cursor: pointer;
  backdrop-filter: blur(10px);
}

.ppt-diagram-category-trigger:hover,
.ppt-diagram-category-trigger:focus-visible {
  color: #f4f4f5;
  outline: none;
}

.ppt-diagram-category-trigger span {
  font-size: 12px;
  font-weight: 760;
}

.ppt-diagram-category-trigger small {
  color: #71717a;
  font-weight: 560;
}

.ppt-diagram-category-trigger .ppt-toolbar-icon {
  transition: transform 0.18s ease;
}

.ppt-diagram-category-trigger .ppt-toolbar-icon.is-collapsed {
  transform: rotate(-90deg);
}

.ppt-diagram-card-label {
  display: flex;
  align-items: flex-start;
  gap: 5px;
  min-width: 0;
}

.ppt-diagram-card-label .ppt-card-grip {
  position: static;
  flex: 0 0 auto;
  margin-top: 2px;
}

.ppt-diagram-card-label strong {
  display: -webkit-box;
  overflow: hidden;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}

.ppt-card-preview.is-embed {
  color: #bbf7d0;
  background:
    linear-gradient(135deg, rgba(34, 197, 94, 0.16), rgba(20, 184, 166, 0.1)),
    #0f1613;
}

.ppt-insert-card.is-embed-card {
  place-items: center;
  min-height: 164px;
  padding: 20px 14px 16px;
  text-align: center;
}

.ppt-insert-card.is-embed-card:hover {
  transform: translateY(-2px) scale(1.01);
}

.ppt-embed-card-icon {
  display: grid;
  place-items: center;
  width: 56px;
  height: 56px;
  border: 1px solid #262626;
  border-radius: 14px;
  color: #bbf7d0;
  background: #151515;
  box-shadow: 0 14px 28px rgba(0, 0, 0, 0.22);
  font-size: 14px;
  font-weight: 840;
}

.ppt-embed-card-copy {
  display: grid;
  gap: 5px;
}

.ppt-embed-card-copy strong,
.ppt-embed-card-copy small {
  display: block;
}

.ppt-insert-card strong {
  color: #f4f4f5;
  font-size: 13px;
  line-height: 1.35;
}

.ppt-insert-card.is-basic-block strong {
  font-size: 13px;
  line-height: 1.25;
}

.ppt-insert-card small,
.ppt-embed-panel p,
.ppt-record-panel p {
  color: #a1a1aa;
  font-size: 12px;
  line-height: 1.55;
}

.ppt-insert-card.is-basic-block small {
  line-height: 1.35;
}

.ppt-panel-empty-state {
  min-height: 160px;
  border: 1px dashed #2b2b2b;
  border-radius: 12px;
  background: #0d0d0d;
}

.ppt-agent-panel {
  display: grid;
  grid-template-rows: auto auto minmax(180px, 1fr) auto auto;
  gap: 12px;
  min-height: 0;
}

.ppt-agent-current-slide {
  display: grid;
  gap: 4px;
  padding: 10px;
  border: 1px solid #2b2b2b;
  border-radius: 10px;
  background: #101010;
}

.ppt-agent-current-slide span,
.ppt-agent-current-slide small {
  color: #85858c;
  font-size: 11px;
}

.ppt-agent-current-slide strong {
  overflow: hidden;
  color: #f4f4f5;
  font-size: 13px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ppt-agent-quick-prompts {
  display: grid;
  gap: 8px;
}

.ppt-agent-quick-prompts button,
.ppt-agent-apply,
.ppt-agent-input button {
  min-height: 36px;
  padding: 0 10px;
  border: 1px solid #333;
  border-radius: 8px;
  color: #f4f4f5;
  background: #151515;
  cursor: pointer;
  font-weight: 780;
}

.ppt-agent-quick-prompts button {
  text-align: left;
}

.ppt-agent-quick-prompts button:hover:not(:disabled),
.ppt-agent-apply:hover:not(:disabled),
.ppt-agent-input button:hover:not(:disabled) {
  border-color: #525252;
  background: #202020;
}

.ppt-agent-quick-prompts button:disabled,
.ppt-agent-apply:disabled,
.ppt-agent-input button:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}

.ppt-agent-messages {
  display: grid;
  align-content: start;
  gap: 10px;
  min-height: 0;
  overflow: auto;
  padding-right: 2px;
}

.ppt-agent-messages article {
  display: grid;
  gap: 5px;
  padding: 10px;
  border: 1px solid #262626;
  border-radius: 10px;
  background: #101010;
}

.ppt-agent-messages article.user {
  border-color: rgba(34, 211, 238, 0.28);
  background: rgba(14, 165, 233, 0.1);
}

.ppt-agent-messages strong {
  color: #f4f4f5;
  font-size: 12px;
}

.ppt-agent-messages p {
  margin: 0;
  color: #cbd5e1;
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
}

.ppt-agent-input {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 62px;
  gap: 8px;
}

.ppt-agent-input textarea {
  min-height: 66px;
  padding: 9px;
  border: 1px solid #2b2b2b;
  border-radius: 8px;
  color: #f4f4f5;
  caret-color: #fff;
  background: #0d0d0d;
  resize: vertical;
  outline: 0;
}

.ppt-agent-input textarea:focus {
  border-color: #3f3f46;
  box-shadow: 0 0 0 3px rgba(34, 211, 238, 0.12);
}

.ppt-agent-apply {
  width: 100%;
}

.ppt-panel-secondary-toggle {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  width: 100%;
  min-height: 34px;
  margin-top: 2px;
  padding: 0 2px;
  border: 0;
  color: #a1a1aa;
  background: transparent;
  cursor: pointer;
  font-size: 12px;
  font-weight: 780;
  text-align: left;
}

.ppt-panel-secondary-toggle:hover {
  color: #f4f4f5;
}

.ppt-panel-secondary-toggle:focus-visible {
  border-radius: 8px;
  color: #f4f4f5;
  outline: 2px solid #22d3ee;
  outline-offset: 2px;
}

.ppt-panel-secondary-toggle .ppt-toolbar-icon {
  width: 16px;
  height: 16px;
  transition: transform 0.16s ease;
}

.ppt-panel-secondary-toggle .ppt-toolbar-icon.is-expanded {
  transform: rotate(180deg);
}

.ppt-current-slide-editor {
  display: grid;
  gap: 10px;
  margin-top: 4px;
  padding-top: 12px;
  border-top: 1px solid #242424;
}

.ppt-current-slide-editor > strong {
  color: #f4f4f5;
  font-size: 13px;
}

.ppt-panel-closed-state {
  display: grid;
  gap: 8px;
  max-width: 210px;
  color: #a1a1aa;
  text-align: center;
}

.ppt-panel-closed-state strong {
  color: #f4f4f5;
  font-size: 14px;
}

.ppt-panel-closed-state span {
  font-size: 12px;
  line-height: 1.55;
}

.ppt-embed-url-card {
  display: grid;
  gap: 10px;
  padding: 12px;
  border: 1px solid #242424;
  border-radius: 12px;
  background: #0d0d0d;
}

.ppt-embed-panel label {
  display: grid;
  gap: 6px;
}

.ppt-embed-panel label span {
  color: #f4f4f5;
  font-size: 13px;
  font-weight: 780;
}

.ppt-embed-panel input {
  min-height: 38px;
  padding: 0 10px;
  border: 1px solid #2b2b2b;
  border-radius: 8px;
  color: #f4f4f5;
  caret-color: #fff;
  background: #0d0d0d;
}

.ppt-embed-url-card > button,
.ppt-record-panel button {
  min-height: 36px;
  padding: 0 12px;
  border: 1px solid #333;
  border-radius: 8px;
  color: #111;
  background: #f4f4f5;
  cursor: pointer;
  font-weight: 780;
}

.ppt-embed-url-card > button:disabled {
  color: #71717a;
  background: #1f1f1f;
  cursor: not-allowed;
}

.ppt-record-panel strong {
  color: #f4f4f5;
}

.ppt-right-utility-rail {
  position: relative;
  display: grid;
  grid-template-columns: minmax(66px, 1fr) 34px;
  align-items: center;
  gap: 6px;
  padding: 6px;
  border: 1px solid #242424;
  border-radius: 10px;
  background: #0c0c0c;
}

.ppt-right-utility-rail button {
  min-height: 32px;
  border: 1px solid #333;
  border-radius: 8px;
  color: #f4f4f5;
  background: #151515;
  cursor: pointer;
}

.ppt-right-utility-rail button:hover {
  border-color: #4a4a4a;
  background: #202020;
}

.ppt-right-utility-rail > span {
  color: #d4d4d8;
  text-align: center;
  font-size: 12px;
  font-weight: 780;
}

.ppt-zoom-control,
.ppt-help-control {
  position: relative;
  min-width: 0;
}

.ppt-zoom-value {
  width: 100%;
  padding: 0 6px;
  color: #d4d4d8;
  font-size: 12px;
  font-weight: 780;
}

.ppt-zoom-menu,
.ppt-help-menu {
  position: absolute;
  right: 0;
  bottom: calc(100% + 8px);
  z-index: 50;
  display: grid;
  gap: 4px;
  padding: 8px;
  border: 1px solid #2b2b2b;
  border-radius: 10px;
  background: #101010;
  box-shadow: 0 18px 60px rgba(0, 0, 0, 0.5);
}

.ppt-zoom-menu {
  width: 128px;
}

.ppt-help-menu {
  width: 210px;
}

.ppt-zoom-menu button,
.ppt-help-menu button {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  width: 100%;
  min-height: 34px;
  padding: 0 9px;
  border: 0;
  color: #f4f4f5;
  background: transparent;
  text-align: left;
}

.ppt-help-menu button {
  justify-content: flex-start;
}

.ppt-zoom-menu button:hover,
.ppt-zoom-menu button.active,
.ppt-help-menu button:hover {
  background: #1d1d1d;
}

.ppt-zoom-value:focus-visible,
.ppt-help-control > button:focus-visible,
.ppt-zoom-menu button:focus-visible,
.ppt-help-menu button:focus-visible,
.ppt-share-link-card input:focus-visible,
.ppt-share-link-card button:focus-visible {
  outline: 2px solid #22d3ee;
  outline-offset: 2px;
}

.ppt-help-menu p {
  margin: 2px 8px 4px;
  color: #85858c;
  font-size: 11px;
  line-height: 1.4;
}

.ppt-utility-icon {
  width: 15px;
  height: 15px;
  fill: none;
  stroke: currentColor;
  stroke-width: 1.8;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.ppt-right-theme-panel,
.ppt-speaker-notes {
  display: grid;
  gap: 12px;
}

.ppt-right-theme-panel strong,
.ppt-speaker-notes span {
  color: #f4f4f5;
  font-weight: 780;
}

.ppt-layout-panel,
.ppt-background-panel {
  display: grid;
  gap: 12px;
}

.ppt-layout-panel-head,
.ppt-background-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.ppt-layout-panel-head strong {
  color: #f4f4f5;
  font-size: 13px;
}

.ppt-layout-panel-head button,
.ppt-background-custom-color,
.ppt-background-actions button,
.ppt-background-image-control button {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 34px;
  padding: 0 10px;
  border: 1px solid #333;
  border-radius: 8px;
  color: #f4f4f5;
  background: #151515;
  cursor: pointer;
  font-weight: 780;
}

.ppt-layout-panel-head button:hover:not(:disabled),
.ppt-background-custom-color:hover:not(.disabled),
.ppt-background-actions button:hover:not(:disabled),
.ppt-background-image-control button:hover:not(:disabled) {
  border-color: #525252;
  background: #202020;
}

.ppt-layout-panel-head button:focus-visible,
.ppt-background-actions button:focus-visible,
.ppt-background-image-control button:focus-visible {
  outline: 2px solid #22d3ee;
  outline-offset: 2px;
}

.ppt-layout-panel-head button:disabled,
.ppt-background-custom-color.disabled,
.ppt-background-actions button:disabled,
.ppt-background-image-control button:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}

.ppt-background-custom-color input {
  position: absolute;
  inset: 0;
  width: 100%;
  opacity: 0;
  cursor: pointer;
}

.ppt-background-custom-color.disabled input {
  cursor: not-allowed;
}

.ppt-layout-choice-grid {
  display: grid;
  gap: 10px;
}

.ppt-layout-choice-card {
  display: grid;
  grid-template-columns: 72px minmax(0, 1fr);
  gap: 8px 10px;
  min-height: 96px;
  padding: 10px;
  border: 1px solid #2b2b2b;
  border-radius: 12px;
  color: #f4f4f5;
  background: #111;
  cursor: pointer;
  text-align: left;
}

.ppt-layout-choice-card:hover:not(:disabled),
.ppt-layout-choice-card.active {
  border-color: rgba(34, 211, 238, 0.72);
  background: linear-gradient(135deg, rgba(14, 165, 233, 0.14), rgba(15, 23, 42, 0.5)), #111;
}

.ppt-layout-choice-card:focus-visible {
  border-color: #22d3ee;
  outline: 2px solid #22d3ee;
  outline-offset: 2px;
}

.ppt-layout-choice-card:disabled {
  cursor: not-allowed;
  opacity: 0.48;
}

.ppt-layout-choice-card strong {
  align-self: end;
  color: #f4f4f5;
  font-size: 13px;
}

.ppt-layout-choice-card small {
  grid-column: 2;
  color: #9ca3af;
  font-size: 11px;
  line-height: 1.5;
}

.ppt-layout-choice-preview {
  grid-row: span 2;
  display: grid;
  gap: 5px;
  align-content: center;
  width: 72px;
  aspect-ratio: 16 / 10;
  padding: 8px;
  border: 1px solid #2f2f35;
  border-radius: 8px;
  background: linear-gradient(135deg, #f8fafc, #dbeafe);
}

.ppt-layout-choice-preview i {
  display: block;
  min-height: 5px;
  border-radius: 999px;
  background: #2563eb;
}

.ppt-layout-choice-preview i:nth-child(2),
.ppt-layout-choice-preview i:nth-child(3) {
  background: rgba(15, 23, 42, 0.34);
}

.ppt-layout-choice-preview[data-layout="cover"] i:first-child {
  width: 76%;
  height: 10px;
}

.ppt-layout-choice-preview[data-layout="section"] {
  place-content: center;
}

.ppt-layout-choice-preview[data-layout="section"] i:first-child {
  width: 42px;
  height: 18px;
}

.ppt-layout-choice-preview[data-layout="section"] i:nth-child(n + 2) {
  display: none;
}

.ppt-layout-choice-preview[data-layout="content"] i {
  width: 88%;
}

.ppt-layout-choice-preview[data-layout="imageText"] {
  grid-template-columns: 1fr 26px;
}

.ppt-layout-choice-preview[data-layout="imageText"] i:first-child {
  grid-row: span 3;
  align-self: stretch;
  width: 100%;
  border-radius: 6px;
  background: rgba(37, 99, 235, 0.24);
}

.ppt-layout-choice-preview[data-layout="summary"] {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.ppt-layout-choice-preview[data-layout="summary"] i:first-child {
  grid-column: span 2;
}

.ppt-background-tabs {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 6px;
  padding: 4px;
  border: 1px solid #262626;
  border-radius: 10px;
  background: #0d0d0d;
}

.ppt-background-tabs button {
  min-height: 32px;
  border: 0;
  border-radius: 7px;
  color: #a1a1aa;
  background: transparent;
  cursor: pointer;
  font-size: 12px;
  font-weight: 780;
}

.ppt-background-tabs button:hover,
.ppt-background-tabs button.active {
  color: #f4f4f5;
  background: #262626;
}

.ppt-background-tabs button:focus-visible {
  color: #f4f4f5;
  background: #262626;
  outline: 2px solid #22d3ee;
  outline-offset: 2px;
}

.ppt-background-swatch-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 9px;
}

.ppt-background-swatch-grid button {
  position: relative;
  display: flex;
  align-items: flex-end;
  min-height: 76px;
  padding: 8px;
  border: 1px solid #2f2f35;
  border-radius: 10px;
  cursor: pointer;
  overflow: hidden;
}

.ppt-background-swatch-grid button:hover,
.ppt-background-swatch-grid button.active {
  border-color: #f4f4f5;
  box-shadow: 0 0 0 1px rgba(244, 244, 245, 0.32);
}

.ppt-background-swatch-grid button:focus-visible {
  border-color: #22d3ee;
  outline: 2px solid #22d3ee;
  outline-offset: 2px;
}

.ppt-background-swatch-grid button::before {
  content: "";
  position: absolute;
  inset: 0;
  background: linear-gradient(180deg, transparent 22%, rgba(0, 0, 0, 0.56));
}

.ppt-background-swatch-grid span {
  position: relative;
  color: #fff;
  font-size: 12px;
  font-weight: 820;
  text-shadow: 0 1px 8px rgba(0, 0, 0, 0.55);
}

.ppt-background-image-control {
  display: grid;
  gap: 8px;
}

.ppt-background-image-control span {
  color: #f4f4f5;
  font-size: 13px;
  font-weight: 780;
}

.ppt-background-image-control input {
  min-height: 38px;
  padding: 0 10px;
  border: 1px solid #2b2b2b;
  border-radius: 8px;
  color: #f4f4f5;
  caret-color: #fff;
  background: #0d0d0d;
  outline: 0;
}

.ppt-background-image-control input:focus {
  border-color: #3f3f46;
  box-shadow: 0 0 0 3px rgba(34, 211, 238, 0.12);
}

.ppt-global-settings-panel,
.ppt-global-settings-section,
.ppt-icon-picker-panel {
  display: grid;
  gap: 12px;
}

.ppt-global-settings-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding-bottom: 10px;
  border-bottom: 1px solid #242424;
}

.ppt-global-settings-header > div,
.ppt-global-settings-tabs button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
}

.ppt-global-settings-header strong {
  color: #f4f4f5;
  font-size: 14px;
}

.ppt-global-settings-header .ppt-toolbar-icon {
  color: #22d3ee;
}

.ppt-global-settings-header button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  min-height: 30px;
  border: 1px solid #333;
  border-radius: 50%;
  color: #f4f4f5;
  background: #151515;
  cursor: pointer;
}

.ppt-global-settings-header button:hover {
  border-color: #525252;
  background: #202020;
}

.ppt-global-settings-tabs {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 6px;
  padding: 4px;
  border: 1px solid #262626;
  border-radius: 10px;
  background: #0d0d0d;
}

.ppt-global-settings-tabs button,
.ppt-setting-segmented button {
  min-height: 32px;
  border: 0;
  border-radius: 7px;
  color: #a1a1aa;
  background: transparent;
  cursor: pointer;
  font-size: 12px;
  font-weight: 780;
}

.ppt-global-settings-tabs button:hover,
.ppt-global-settings-tabs button.active,
.ppt-setting-segmented button:hover,
.ppt-setting-segmented button.active {
  color: #f4f4f5;
  background: #262626;
}

.ppt-global-settings-section > strong {
  color: #f4f4f5;
  font-size: 13px;
}

.ppt-global-settings-section p {
  margin: 0;
  color: #a1a1aa;
  font-size: 12px;
  line-height: 1.6;
}

.ppt-setting-choice-grid {
  display: grid;
  gap: 8px;
}

.ppt-setting-choice-grid button {
  display: grid;
  gap: 4px;
  min-height: 62px;
  padding: 10px;
  border: 1px solid #2b2b2b;
  border-radius: 10px;
  color: #f4f4f5;
  background: #111;
  cursor: pointer;
  text-align: left;
}

.ppt-setting-choice-grid button:hover,
.ppt-setting-choice-grid button.active {
  border-color: rgba(34, 211, 238, 0.72);
  background: linear-gradient(135deg, rgba(14, 165, 233, 0.14), rgba(15, 23, 42, 0.48)), #111;
}

.ppt-setting-choice-grid span {
  font-size: 13px;
  font-weight: 820;
}

.ppt-setting-choice-grid small {
  color: #9ca3af;
  font-size: 11px;
  line-height: 1.45;
}

.ppt-setting-segmented {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 4px;
  padding: 4px;
  border: 1px solid #262626;
  border-radius: 10px;
  background: #0d0d0d;
}

.ppt-setting-wide-button {
  min-height: 38px;
  padding: 0 12px;
  border: 1px solid #333;
  border-radius: 8px;
  color: #f4f4f5;
  background: #151515;
  cursor: pointer;
  font-weight: 780;
}

.ppt-setting-wide-button:hover:not(:disabled) {
  border-color: #525252;
  background: #202020;
}

.ppt-setting-wide-button:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}

.ppt-icon-current-card {
  display: grid;
  grid-template-columns: 44px minmax(0, 1fr) auto;
  align-items: center;
  gap: 10px;
  padding: 10px;
  border: 1px solid #2b2b2b;
  border-radius: 10px;
  background: #101010;
}

.ppt-icon-current-card > span {
  display: grid;
  place-items: center;
  width: 44px;
  height: 44px;
  border-radius: 10px;
  color: #111;
  background: #f4f4f5;
  font-size: 16px;
  font-weight: 900;
}

.ppt-icon-current-card div {
  display: grid;
  gap: 3px;
  min-width: 0;
}

.ppt-icon-current-card strong {
  overflow: hidden;
  color: #f4f4f5;
  font-size: 13px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ppt-icon-current-card small {
  overflow: hidden;
  color: #9ca3af;
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ppt-icon-current-card button {
  min-height: 32px;
  padding: 0 10px;
  border: 1px solid #333;
  border-radius: 8px;
  color: #f4f4f5;
  background: #151515;
  cursor: pointer;
  font-weight: 780;
}

.ppt-icon-current-card button:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}

.ppt-icon-picker-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 8px;
}

.ppt-icon-picker-grid button {
  display: grid;
  place-items: center;
  gap: 5px;
  min-height: 68px;
  padding: 8px 4px;
  border: 1px solid #2b2b2b;
  border-radius: 10px;
  color: #f4f4f5;
  background: #111;
  cursor: pointer;
}

.ppt-icon-picker-grid button:hover,
.ppt-icon-picker-grid button.active {
  border-color: rgba(34, 211, 238, 0.72);
  background: rgba(14, 165, 233, 0.14);
}

.ppt-icon-current-card button:focus-visible,
.ppt-icon-picker-grid button:focus-visible {
  border-color: #22d3ee;
  outline: 2px solid #22d3ee;
  outline-offset: 2px;
}

.ppt-icon-picker-grid span {
  display: grid;
  place-items: center;
  min-width: 26px;
  height: 26px;
  color: #111;
  background: #f4f4f5;
  border-radius: 7px;
  font-size: 13px;
  font-weight: 900;
}

.ppt-icon-picker-grid small {
  max-width: 100%;
  overflow: hidden;
  color: #a1a1aa;
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ppt-speaker-notes textarea {
  min-height: 220px;
  padding: 10px;
  border: 1px solid #2b2b2b;
  border-radius: 8px;
  color: #f4f4f5;
  caret-color: #fff;
  background: #0d0d0d;
  resize: vertical;
}

.ppt-speaker-notes header,
.ppt-record-panel header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.ppt-speaker-notes header div {
  display: grid;
  gap: 3px;
  min-width: 0;
}

.ppt-speaker-notes header strong,
.ppt-record-panel header strong {
  color: #f4f4f5;
  font-size: 14px;
}

.ppt-speaker-notes header span,
.ppt-record-panel header span {
  color: #85858c;
  font-size: 12px;
}

.ppt-record-panel header span.active {
  color: #bbf7d0;
}

.ppt-speaker-notes header button,
.ppt-speaker-note-actions button,
.ppt-record-actions button,
.ppt-record-options button {
  min-height: 34px;
  padding: 0 10px;
  border: 1px solid #333;
  border-radius: 8px;
  color: #f4f4f5;
  background: #151515;
  cursor: pointer;
  font-weight: 780;
}

.ppt-speaker-notes header button:hover,
.ppt-speaker-note-actions button:hover,
.ppt-record-actions button:hover,
.ppt-record-options button:hover {
  border-color: #525252;
  background: #202020;
}

.ppt-speaker-notes button:focus-visible,
.ppt-speaker-note-actions button:focus-visible,
.ppt-record-actions button:focus-visible,
.ppt-record-options button:focus-visible,
.ppt-record-quality select:focus-visible {
  outline: 2px solid #22d3ee;
  outline-offset: 2px;
}

.ppt-speaker-notes button:disabled,
.ppt-record-actions button:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}

.ppt-speaker-note-actions,
.ppt-record-actions {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
}

.ppt-notes-progress {
  display: grid;
  gap: 8px;
  color: #a1a1aa;
  font-size: 12px;
}

.ppt-notes-progress div {
  overflow: hidden;
  height: 6px;
  border-radius: 999px;
  background: #242424;
}

.ppt-notes-progress i {
  display: block;
  height: 100%;
  border-radius: inherit;
  background: linear-gradient(90deg, #22c55e, #38bdf8);
}

.ppt-record-panel {
  gap: 12px;
}

.ppt-record-preview {
  display: grid;
  grid-template-columns: 54px minmax(0, 1fr);
  align-items: center;
  gap: 12px;
  padding: 12px;
  border: 1px solid #2b2b2b;
  border-radius: 10px;
  background: #0d0d0d;
}

.ppt-record-preview > div {
  display: grid;
  place-items: center;
  width: 54px;
  height: 54px;
  border-radius: 10px;
  color: #f4f4f5;
  background: linear-gradient(135deg, #27272a, #111827);
}

.ppt-record-preview p {
  margin: 0;
}

.ppt-record-icon {
  width: 24px;
  height: 24px;
  fill: none;
  stroke: currentColor;
  stroke-width: 1.8;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.ppt-record-options {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
}

.ppt-record-options button {
  display: grid;
  gap: 3px;
  justify-items: start;
  min-height: 54px;
  color: #a1a1aa;
  background: #101010;
}

.ppt-record-options button.active {
  border-color: rgba(56, 189, 248, 0.65);
  color: #e0f2fe;
  background: rgba(14, 165, 233, 0.12);
}

.ppt-record-options button span {
  color: inherit;
  font-size: 13px;
}

.ppt-record-options button small {
  color: #85858c;
  font-size: 11px;
}

.ppt-record-quality {
  display: grid;
  gap: 6px;
}

.ppt-record-quality span {
  color: #f4f4f5;
  font-size: 13px;
  font-weight: 780;
}

.ppt-record-quality select {
  min-height: 38px;
  padding: 0 10px;
  border: 1px solid #2b2b2b;
  border-radius: 8px;
  color: #f4f4f5;
  background: #0d0d0d;
}

.ppt-record-actions {
  grid-template-columns: 1fr 1fr;
}

.ppt-record-actions .is-primary {
  color: #111;
  background: #f4f4f5;
}

.ppt-recording-status-bar {
  position: fixed;
  left: 50%;
  bottom: 104px;
  z-index: 35;
  display: flex;
  align-items: center;
  gap: 10px;
  max-width: min(620px, calc(100vw - 32px));
  padding: 9px 12px;
  border: 1px solid rgba(56, 189, 248, 0.35);
  border-radius: 999px;
  color: #dbeafe;
  background: rgba(8, 17, 31, 0.86);
  box-shadow: 0 18px 50px rgba(0, 0, 0, 0.35);
  transform: translateX(-50%);
  backdrop-filter: blur(16px);
}

.ppt-recording-status-bar span {
  color: #f8fafc;
  font-size: 12px;
  font-weight: 820;
  white-space: nowrap;
}

.ppt-recording-status-bar small {
  overflow: hidden;
  color: #93c5fd;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12px;
}

.ppt-present-mode {
  position: relative;
  display: grid;
  gap: 16px;
  place-items: center;
  min-height: 100vh;
  padding: 76px 76px 52px;
}

.ppt-presentation-workspace.is-presenting .ppt-present-canvas {
  width: var(--ppt-present-width, min(92vw, 1280px));
  max-width: calc(100vw - 24px);
  max-height: calc(100vh - 150px);
  transform: none;
}

.ppt-present-nav {
  position: fixed;
  top: 50%;
  z-index: 10010;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 46px;
  height: 46px;
  min-height: 46px;
  padding: 0;
  border: 1px solid rgba(255, 255, 255, 0.13);
  border-radius: 999px;
  color: #f8fafc;
  background: rgba(0, 0, 0, 0.46);
  box-shadow: 0 18px 44px rgba(0, 0, 0, 0.32);
  cursor: pointer;
  backdrop-filter: blur(12px);
  transform: translateY(-50%);
}

.ppt-present-nav.is-prev {
  left: 22px;
}

.ppt-present-nav.is-next {
  right: 22px;
}

.ppt-present-nav:hover:not(:disabled) {
  background: rgba(255, 255, 255, 0.14);
}

.ppt-present-nav:disabled {
  opacity: 0.28;
  cursor: not-allowed;
}

.ppt-present-progress-bar {
  position: fixed;
  right: 10px;
  bottom: 5px;
  left: 10px;
  z-index: 10009;
  display: flex;
  gap: 6px;
  height: 6px;
}

.ppt-present-progress-bar button {
  flex: 1 1 0;
  min-height: 6px;
  padding: 0;
  border: 0;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.2);
  cursor: pointer;
  transition: background 0.16s ease, box-shadow 0.16s ease;
}

.ppt-present-progress-bar button:hover,
.ppt-present-progress-bar button.active {
  background: #22d3ee;
  box-shadow: 0 0 12px rgba(34, 211, 238, 0.55);
}

.ppt-present-progress-bar button:focus-visible {
  outline: 2px solid #f8fafc;
  outline-offset: 2px;
}

.ppt-present-phone-overlay div {
  position: absolute;
  inset-block: 0;
  width: 50%;
}

.ppt-present-phone-overlay .is-left {
  left: 0;
}

.ppt-present-phone-overlay .is-right {
  right: 0;
}

.ppt-presentation-empty-panel {
  display: grid;
  justify-items: center;
  align-items: center;
  min-height: 420px;
  padding: 40px 18px 64px;
}

.ppt-presentation-empty-canvas {
  position: relative;
  display: grid;
  place-items: center;
  width: min(920px, 100%);
  aspect-ratio: 16 / 9;
  min-height: 340px;
  overflow: hidden;
  border: 1px dashed rgba(113, 113, 122, 0.76);
  border-radius: 16px;
  color: #f4f4f5;
  background: rgba(11, 11, 11, 0.84);
  box-shadow: 0 24px 80px rgba(0, 0, 0, 0.32);
}

.ppt-presentation-empty-frame {
  position: absolute;
  inset: 22px;
  border: 1px dashed rgba(113, 113, 122, 0.42);
  border-radius: 12px;
  pointer-events: none;
}

.ppt-presentation-empty-content {
  position: relative;
  z-index: 1;
  display: grid;
  justify-items: center;
  gap: 16px;
  max-width: min(420px, calc(100% - 48px));
  text-align: center;
}

.ppt-presentation-empty-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 54px;
  height: 54px;
  border: 1px solid rgba(82, 82, 91, 0.72);
  border-radius: 999px;
  color: #a1a1aa;
  background: rgba(39, 39, 42, 0.62);
}

.ppt-presentation-empty-icon .ppt-toolbar-icon {
  width: 24px;
  height: 24px;
}

.ppt-presentation-empty-content h2 {
  margin: 0 0 6px;
  color: #f8fafc;
  font-size: 17px;
  font-weight: 860;
  letter-spacing: 0;
}

.ppt-presentation-empty-content p {
  margin: 0;
  color: #a1a1aa;
  font-size: 13px;
  line-height: 1.6;
}

.ppt-presentation-empty-action {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  min-height: 40px;
  padding: 0 16px;
  border: 0;
  border-radius: 999px;
  color: #111;
  background: #f4f4f5;
  cursor: pointer;
  font-weight: 840;
  transition: transform 0.16s ease, background-color 0.16s ease, opacity 0.16s ease;
}

.ppt-presentation-empty-action:hover:not(:disabled) {
  transform: translateY(-1px);
  background: #fff;
}

.ppt-presentation-empty-action:disabled {
  opacity: 0.52;
  cursor: not-allowed;
}

.ppt-presentation-empty-action:focus-visible {
  outline: 2px solid rgba(34, 211, 238, 0.72);
  outline-offset: 2px;
}

@keyframes ppt-spin {
  to {
    transform: rotate(360deg);
  }
}

@media (max-width: 1280px) {
  .ppt-reference-main.is-home-layout {
    grid-template-columns: minmax(0, 1fr) minmax(280px, 320px);
    column-gap: 18px;
    row-gap: 12px;
  }

  .ppt-library-panel :deep(.ppt-history-list.is-grid) {
    grid-template-columns: repeat(3, minmax(220px, 1fr));
  }
}

@media (max-width: 1120px) {
  .ppt-reference-main.is-home-layout {
    grid-template-columns: 1fr;
  }

  .ppt-reference-main.is-home-layout > .ppt-home-side-panel {
    grid-column: 1;
    grid-row: auto;
    position: static;
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 980px) {
  .ppt-reference-main {
    width: min(100% - 28px, 760px);
  }

  .ppt-reference-main.is-home-layout {
    width: min(100% - 28px, 760px);
  }

  .ppt-reference-main.is-home-layout > .ppt-home-side-panel {
    grid-template-columns: 1fr;
  }

  .ppt-reference-main.is-home-layout .ppt-composer-footer {
    grid-template-columns: 1fr;
  }

  .ppt-reference-main.is-home-layout .ppt-pill-row {
    justify-content: flex-start;
  }

  .ppt-hero-composer h1 {
    font-size: 26px;
  }

  .ppt-library-toolbar {
    align-items: stretch;
    flex-direction: column;
  }

  .ppt-library-tabs,
  .ppt-library-actions {
    flex-wrap: wrap;
  }

  .ppt-workflow-board,
  .ppt-editor-board,
  .ppt-generation-flow {
    grid-template-columns: 1fr;
  }

  .ppt-presentation-header {
    align-items: stretch;
    flex-direction: column;
  }

  .ppt-presentation-actions {
    flex-wrap: wrap;
  }

  .ppt-presentation-editor {
    grid-template-columns: 1fr;
  }

  .ppt-slide-sidebar-shell {
    max-height: none;
  }

  .ppt-slide-sidebar {
    display: flex;
    flex-direction: column;
    max-height: none;
  }

  .ppt-slide-sidebar-list {
    display: flex;
    overflow-x: auto;
    padding-bottom: 4px;
  }

  .ppt-slide-thumbnail-card {
    min-width: 180px;
  }

  .ppt-slide-sidebar-resize {
    display: none;
  }

  .ppt-right-edit-panel,
  .ppt-slide-stack {
    max-height: none;
  }

  .ppt-right-panel-shell {
    grid-template-columns: 1fr;
  }

  .ppt-right-tool-rail {
    flex-direction: row;
    justify-content: flex-start;
    overflow-x: auto;
    border-right: 0;
    border-bottom: 1px solid #242424;
  }

  .ppt-right-tool-button {
    flex: 0 0 40px;
  }

  .ppt-generation-extra-settings,
  .ppt-generation-steps,
  .ppt-pre-select-grid {
    grid-template-columns: 1fr;
  }

  .ppt-pre-text-content-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .ppt-generate-header-chips {
    flex-wrap: wrap;
    justify-content: flex-start;
  }

  .ppt-library-panel :deep(.ppt-history-list.is-grid) {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .ppt-right-utility-rail {
    grid-template-columns: 34px minmax(72px, 1fr) 34px 34px 38px 34px;
  }

  .ppt-element-floating-toolbar {
    align-items: flex-start;
    flex-wrap: wrap;
    left: 14px;
    right: 14px;
    max-width: calc(100% - 28px);
  }

  .ppt-element-ai-editor {
    right: auto;
    left: 0;
  }
}

@media (max-width: 640px) {
  .ppt-generate-page {
    margin: -10px;
  }

  .ppt-reference-main {
    padding-top: 20px;
  }

  .ppt-reference-main.is-home-layout .ppt-hero-composer h1 {
    font-size: 28px;
  }

  .ppt-reference-main.is-home-layout .ppt-pill-row {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    width: 100%;
  }

  .ppt-reference-main.is-home-layout .ppt-pill {
    width: 100%;
  }

  .ppt-reference-main.is-home-layout .ppt-pill-dropdown:nth-child(even) .ppt-pill-menu {
    right: 0;
    left: auto;
  }

  .ppt-pill-menu,
  .ppt-more-menu,
  .ppt-model-menu,
  .ppt-history-filter-menu {
    max-width: calc(100vw - 24px);
  }

  .ppt-home-template-grid {
    grid-template-columns: 1fr;
  }

  .ppt-composer-footer {
    align-items: flex-start;
  }

  .ppt-pill {
    min-height: 38px;
    padding: 0 12px;
    font-size: 13px;
  }

  .ppt-submit-button {
    width: 40px;
    height: 40px;
  }

  .ppt-generate-header {
    grid-template-columns: 1fr;
  }

  .ppt-generate-header h1 {
    font-size: 22px;
    white-space: normal;
  }

  .ppt-share-link-card > div,
  .ppt-shortcut-list {
    grid-template-columns: 1fr;
  }

  .ppt-share-link-card button {
    width: 100%;
  }

  .ppt-zoom-menu,
  .ppt-help-menu {
    right: auto;
    left: 0;
  }

  .ppt-generate-header-actions {
    align-items: flex-start;
    justify-content: flex-start;
    flex-wrap: wrap;
  }

  .ppt-generation-flow,
  .ppt-editor-board {
    padding: 12px;
  }

  .ppt-pre-outline-panel {
    padding: 18px;
  }

  .ppt-pre-text-content-grid {
    grid-template-columns: 1fr;
  }

  .ppt-generation-bottom-bar {
    right: 0;
    left: 0;
    min-height: 68px;
    margin: 0;
    padding: 12px;
  }

  .ppt-generation-primary {
    width: 100%;
  }

  .ppt-generation-help-control {
    right: 16px;
    bottom: 86px;
  }

  .ppt-presentation-titlebar {
    align-items: stretch;
    flex-wrap: wrap;
  }

  .ppt-presentation-titlebar input {
    flex-basis: calc(100% - 48px);
  }

  .ppt-saving-indicator {
    margin-left: 48px;
  }

  .ppt-presentation-action {
    flex: 1 1 42%;
  }

  .ppt-edit-slide {
    width: 100%;
  }

  .ppt-edit-slide.has-image p,
  .ppt-edit-slide.has-image ul,
  .ppt-edit-slide.has-image em,
  .ppt-present-canvas.has-image p,
  .ppt-present-canvas.has-image ul {
    max-width: 100%;
  }

  .ppt-slide-image-frame {
    position: relative;
    right: auto;
    bottom: auto;
    width: 100%;
    margin-top: 14px;
    border-radius: 12px;
  }

  .ppt-element-floating-toolbar {
    position: static;
    margin: 12px 0 0;
    color: #f8fafc;
  }

  .ppt-element-ai-editor {
    position: fixed;
    top: auto;
    right: 14px;
    bottom: 18px;
    left: 14px;
    width: auto;
    max-height: calc(100vh - 36px);
    overflow: auto;
  }

  .ppt-element-type-popover {
    width: min(210px, 74vw);
  }

  .ppt-present-canvas {
    transform: scale(1);
  }

  .ppt-presentation-workspace.is-presenting,
  .ppt-presentation-workspace.is-presenting .ppt-presentation-editor,
  .ppt-presentation-workspace.is-presenting .ppt-presentation-stage,
  .ppt-presentation-workspace.is-presenting .ppt-present-mode {
    width: 100vw;
    height: 100vh;
    min-height: 100vh;
    margin: 0;
    overflow: hidden;
  }

  @supports (height: 100dvh) {
    .ppt-presentation-workspace.is-presenting,
    .ppt-presentation-workspace.is-presenting .ppt-presentation-editor,
    .ppt-presentation-workspace.is-presenting .ppt-presentation-stage,
    .ppt-presentation-workspace.is-presenting .ppt-present-mode {
      width: 100dvw;
      height: 100dvh;
      min-height: 100dvh;
    }
  }

  .ppt-presentation-workspace.is-presenting .ppt-present-mode {
    box-sizing: border-box;
    padding: max(34px, env(safe-area-inset-top)) 0 max(34px, env(safe-area-inset-bottom));
    place-items: center;
  }

  .ppt-presentation-workspace.is-presenting .ppt-present-canvas {
    width: var(--ppt-present-width, calc(100vw - 24px));
    max-width: calc(100vw - 24px);
    max-height: calc(100vh - 92px);
    margin: auto;
    padding: clamp(18px, 5vw, 28px);
    border-radius: 10px;
  }

  @supports (width: 100dvw) {
    .ppt-presentation-workspace.is-presenting .ppt-present-canvas {
      max-width: calc(100dvw - 24px);
      max-height: calc(100dvh - 92px);
    }
  }

  .ppt-present-phone-top-hitarea {
    position: fixed;
    top: 0;
    right: 0;
    left: 0;
    z-index: 10019;
    display: block;
    height: 72px;
    padding: 0;
    border: 0;
    background: transparent;
    cursor: pointer;
  }

  .ppt-present-phone-overlay {
    position: fixed;
    top: 0;
    right: 0;
    bottom: 10px;
    left: 0;
    z-index: 10000;
    display: block;
    touch-action: pan-y;
    user-select: none;
  }

  .ppt-present-nav {
    display: none;
  }

  .ppt-present-progress-bar {
    right: 12px;
    bottom: max(8px, env(safe-area-inset-bottom));
    left: 12px;
  }

  .ppt-floating-config {
    left: 10px;
    right: 10px;
  }

  .ppt-library-panel {
    padding: 12px;
  }

  .ppt-library-panel :deep(.ppt-history-list.is-grid) {
    grid-template-columns: 1fr;
  }

}

@media (max-height: 520px) {
  .ppt-presentation-workspace.is-presenting .ppt-present-mode {
    padding: max(20px, env(safe-area-inset-top)) 72px max(20px, env(safe-area-inset-bottom));
  }

  .ppt-presentation-workspace.is-presenting .ppt-present-canvas {
    max-height: calc(100vh - 48px);
  }

  @supports (height: 100dvh) {
    .ppt-presentation-workspace.is-presenting .ppt-present-canvas {
      max-height: calc(100dvh - 48px);
    }
  }
}

@media (prefers-reduced-motion: reduce) {
  .ppt-generate-page *,
  .ppt-generate-page *::before,
  .ppt-generate-page *::after {
    scroll-behavior: auto !important;
    transition-duration: 0.01ms !important;
    animation-duration: 0.01ms !important;
    animation-iteration-count: 1 !important;
  }
}
</style>
