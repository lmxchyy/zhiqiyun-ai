<template>
  <view :class="['ppt-page', { 'is-editor': currentMode === 'editor' }]" :style="miniProgramNavigationStyle">
    <AiGeneratedContentNotice v-if="currentMode !== 'editor'" />
    <template v-if="currentMode === 'editor'">
      <view class="ppt-editor-shell" :class="{ presenting: presentationMode === 'present' }">
        <view v-if="presentationMode === 'present'" class="ppt-present-stage">
          <view class="ppt-present-header">
            <text>{{ editorTitle }}</text>
            <button type="button" title="退出演示" aria-label="退出演示" @click="exitPresentationMode">退出演示</button>
          </view>
          <view class="ppt-present-canvas" :style="editorSlideStyle(activeEditorSlide)">
            <text class="ppt-present-count">{{ activeSlideIndex + 1 }} / {{ editorSlides.length }}</text>
            <text class="ppt-present-title">{{ activeEditorSlide.title }}</text>
            <text class="ppt-present-copy">{{ activeEditorSlide.content }}</text>
            <view class="ppt-present-points">
              <text v-for="point in activeEditorSlide.points" :key="point">• {{ point }}</text>
            </view>
            <view v-if="recordingMode" class="ppt-recording-bar">
              <text>录制准备中</text>
              <text>屏幕 · 麦克风 · 无摄像头 · 1080p</text>
            </view>
          </view>
          <button type="button" class="ppt-present-nav prev" :disabled="activeSlideIndex <= 0" title="上一页" aria-label="上一页" @click="selectEditorSlide(activeSlideIndex - 1)">‹</button>
          <button type="button" class="ppt-present-nav next" :disabled="activeSlideIndex >= editorSlides.length - 1" title="下一页" aria-label="下一页" @click="selectEditorSlide(activeSlideIndex + 1)">›</button>
        </view>

        <template v-else>
          <view class="ppt-editor-header">
            <view class="ppt-editor-titlebar">
              <button type="button" class="ppt-editor-icon is-brain" title="回到 PPT 工作台" aria-label="回到 PPT 工作台" @click="backToPptHome">
                <text>☁</text>
              </button>
              <view class="ppt-editor-menu-wrap">
                <button
                  type="button"
                  class="ppt-editor-icon"
                  title="打开演示文稿菜单"
                  aria-label="打开演示文稿菜单"
                  :aria-expanded="showEditorMenu"
                  aria-haspopup="menu"
                  @click="toggleEditorMenu"
                >
                  <text>☰</text>
                </button>
                <view v-if="showEditorMenu" class="ppt-editor-menu" role="menu" aria-label="演示文稿菜单">
                  <text>文件</text>
                  <button type="button" role="menuitem" title="新建演示文稿" aria-label="新建演示文稿" @click="createBlankEditorDeck">＋ 新建演示文稿</button>
                  <button type="button" role="menuitem" title="重命名演示文稿" aria-label="重命名演示文稿" @click="focusEditorTitle">✎ 重命名</button>
                  <button type="button" role="menuitem" title="复制当前演示文稿" aria-label="复制当前演示文稿" @click="duplicateEditorDeck">▣ 复制演示文稿</button>
                  <text>编辑</text>
                  <button type="button" role="menuitem" title="撤销上一步编辑" aria-label="撤销上一步编辑" aria-keyshortcuts="Control+Z" :disabled="!canUndoEditor" @click="undoEditor">↶ 撤销 <text>Ctrl+Z</text></button>
                  <button type="button" role="menuitem" title="重做下一步编辑" aria-label="重做下一步编辑" aria-keyshortcuts="Control+Y" :disabled="!canRedoEditor" @click="redoEditor">↷ 重做 <text>Ctrl+Y</text></button>
                  <text>工作区</text>
                  <button type="button" role="menuitem" title="打开页面设置" aria-label="打开页面设置" @click="openEditorPanel('settings')">⚙ 页面设置</button>
                  <button type="button" role="menuitem" title="打开主题面板" aria-label="打开主题面板" @click="openEditorPanel('theme')">◉ 主题面板</button>
                  <button type="button" role="menuitem" title="打开分享设置" aria-label="打开分享设置" @click="openShareSettings">⌯ 分享设置</button>
                  <text>视图</text>
                  <button type="button" role="menuitem" title="返回提示词与历史页" aria-label="返回提示词与历史页" @click="backToPptHome">▤ 返回提示词</button>
                  <button type="button" role="menuitem" title="查看全部演示文稿" aria-label="查看全部演示文稿" @click="backToPptHome">▭ 全部演示文稿</button>
                </view>
              </view>
              <input
                ref="editorTitleInputRef"
                v-model="editorTitle"
                class="ppt-editor-title-input"
                title="演示文稿标题"
                aria-label="演示文稿标题"
                @input="markEditorSaved"
              />
              <text class="ppt-save-state">✓ 已保存</text>
              <button type="button" class="ppt-editor-history" title="撤销 (Ctrl+Z)" aria-label="撤销" aria-keyshortcuts="Control+Z" :disabled="!canUndoEditor" @click="undoEditor">↶</button>
              <button type="button" class="ppt-editor-history" title="重做 (Ctrl+Y)" aria-label="重做" aria-keyshortcuts="Control+Y" :disabled="!canRedoEditor" @click="redoEditor">↷</button>
            </view>
            <view class="ppt-editor-actions">
              <button type="button" :class="{ active: editorPanel === 'theme' }" title="打开主题面板" aria-label="打开主题面板" @click="openEditorPanel('theme')">◉ 主题</button>
              <button type="button" title="导出演示文稿" aria-label="导出演示文稿" @click="exportEditorDeck">⇩ 导出</button>
              <button type="button" title="复制或打开分享设置" aria-label="复制或打开分享设置" @click="openShareSettings">⌯ 分享</button>
              <button type="button" class="primary" title="演示" aria-label="演示" @click="enterPresentationMode(false)">▷ 演示</button>
            </view>
          </view>

          <view class="ppt-editor-body">
            <aside class="ppt-slide-sidebar">
              <view class="ppt-slide-sidebar-head">
                <view>
                  <text>幻灯片</text>
                  <text>{{ editorSlides.length }} 张</text>
                </view>
              </view>
              <scroll-view scroll-y class="ppt-slide-thumbs">
                <button
                  v-for="(slide, index) in editorSlides"
                  :key="slide.id"
                  type="button"
                  :class="['ppt-slide-thumb', { active: activeSlideIndex === index }]"
                  :title="slide.title"
                  :aria-label="`第 ${index + 1} 页：${slide.title}`"
                  :aria-selected="activeSlideIndex === index"
                  @click="selectEditorSlide(index)"
                >
                  <text>{{ index + 1 }}</text>
                  <view>
                    <text>{{ slide.title }}</text>
                    <text>{{ slide.content }}</text>
                  </view>
                </button>
              </scroll-view>
            </aside>

            <scroll-view scroll-y class="ppt-slide-canvas">
              <view
                v-for="(slide, index) in editorSlides"
                :key="slide.id"
                :class="['ppt-edit-slide', `layout-${slide.layout}`, { active: activeSlideIndex === index }]"
                :style="editorSlideStyle(slide)"
                :data-ppt-slide-index="index"
                @click.self="selectEditorSlide(index)"
              >
                <button v-if="index === 0" type="button" class="ppt-slide-insert before" title="在前方插入幻灯片" aria-label="在前方插入幻灯片" @click="insertEditorSlide(index, 'before')">＋</button>
                <text class="ppt-slide-count">{{ index + 1 }} / {{ editorSlides.length }}</text>
                <text class="ppt-slide-title">{{ slide.title }}</text>
                <text class="ppt-slide-copy">{{ slide.content }}</text>
                <view class="ppt-slide-points">
                  <text v-for="point in slide.points" :key="point">• {{ point }}</text>
                </view>
                <image v-if="slide.imageUrl" class="ppt-slide-visual" :src="slide.imageUrl" mode="aspectFill" />

                <view class="ppt-slide-floating-tools">
                  <view class="ppt-floating-menu-wrap">
                    <button type="button" title="更多操作" aria-label="更多操作" :aria-expanded="slideMoreMenuIndex === index" aria-haspopup="menu" @click="toggleSlideMoreMenu(index)">⋮</button>
                    <view v-if="slideMoreMenuIndex === index" class="ppt-floating-menu" role="menu" aria-label="更多操作">
                      <button type="button" role="menuitem" title="复制幻灯片" aria-label="复制幻灯片" @click="duplicateEditorSlide(index)">▣ 复制幻灯片</button>
                      <button type="button" role="menuitem" class="danger" :disabled="editorSlides.length <= 1" title="删除幻灯片" aria-label="删除幻灯片" @click="deleteEditorSlide(index)">⌫ 删除</button>
                    </view>
                  </view>
                  <view class="ppt-floating-menu-wrap">
                    <button type="button" title="主题与布局" aria-label="主题与布局" :aria-expanded="slidePaletteMenuIndex === index" aria-haspopup="menu" @click="toggleSlidePaletteMenu(index)">◉</button>
                    <view v-if="slidePaletteMenuIndex === index" class="ppt-palette-menu" role="menu" aria-label="主题与布局">
                      <view>
                        <text>布局</text>
                        <view class="ppt-palette-layouts">
                          <button v-for="item in editorLayoutOptions" :key="item.value" type="button" role="menuitemradio" :aria-checked="slide.layout === item.value" :title="`切换为${item.label}布局`" :aria-label="`切换为${item.label}布局`" :class="{ active: slide.layout === item.value }" @click="applyEditorLayout(index, item.value)">{{ item.short }}</button>
                        </view>
                      </view>
                      <view>
                        <text>卡片颜色</text>
                        <view class="ppt-palette-colors">
                          <button v-for="item in editorColorOptions" :key="item.value" type="button" role="menuitemradio" :style="{ background: item.value }" :title="item.label" :aria-label="`应用${item.label}背景`" :aria-checked="slide.background === item.value" :class="{ active: slide.background === item.value }" @click="applyEditorBackground(index, item.value)"></button>
                        </view>
                      </view>
                      <view>
                        <text>内容对齐</text>
                        <view class="ppt-palette-segment">
                          <button v-for="item in editorAlignOptions" :key="item.value" type="button" role="menuitemradio" :title="`内容${item.label}`" :aria-label="`内容${item.label}`" :aria-checked="slide.align === item.value" :class="{ active: slide.align === item.value }" @click="applyEditorAlign(index, item.value)">{{ item.short }}</button>
                        </view>
                      </view>
                      <view>
                        <text>卡片宽度</text>
                        <view class="ppt-palette-segment">
                          <button v-for="item in editorWidthOptions" :key="item.value" type="button" role="menuitemradio" :title="`卡片宽度：${item.label}`" :aria-label="`切换卡片宽度为${item.label}`" :aria-checked="slide.width === item.value" :class="{ active: slide.width === item.value }" @click="applyEditorWidth(index, item.value)">{{ item.label }}</button>
                        </view>
                      </view>
                      <view class="ppt-palette-image-row">
                        <text>图片</text>
                        <button type="button" title="为当前页添加图片" aria-label="为当前页添加图片" @click="openEditorPanel('images')">+ 添加</button>
                      </view>
                    </view>
                  </view>
                  <view class="ppt-floating-menu-wrap">
                    <button type="button" title="AI 编辑这页幻灯片" aria-label="AI 编辑这页幻灯片" :aria-expanded="slideMagicMenuIndex === index" aria-haspopup="menu" @click="toggleSlideMagicMenu(index)">✦</button>
                    <view v-if="slideMagicMenuIndex === index" class="ppt-magic-menu" role="menu" aria-label="AI 编辑这页幻灯片">
                      <view class="ppt-magic-head">
                        <text>编辑这页幻灯片</text>
                        <text>输入修改要求，或使用快捷操作。</text>
                      </view>
                      <label class="ppt-magic-input">
                        <input v-model="slideMagicPrompt" title="AI 编辑这页幻灯片要求" aria-label="AI 编辑这页幻灯片要求" placeholder="想如何编辑这页幻灯片？" @keyup.enter="submitSlideMagicPrompt(index)" />
                        <button type="button" :disabled="!slideMagicPrompt.trim()" title="发送编辑要求" aria-label="发送编辑要求" @click="submitSlideMagicPrompt(index)">→</button>
                      </label>
                      <button type="button" class="ppt-magic-primary" role="menuitem" title="尝试新布局" aria-label="尝试新布局" @click="runSlideMagicAction(index, 'layout')">✦ 尝试新布局 →</button>
                      <text>文案</text>
                      <view class="ppt-magic-grid">
                        <button type="button" role="menuitem" title="优化文案" aria-label="优化文案" @click="runSlideMagicAction(index, 'writing')">✎ 优化文案</button>
                        <button type="button" role="menuitem" title="修正错别字" aria-label="修正错别字" @click="runSlideMagicAction(index, 'spelling')">✓ 修正错别字</button>
                        <button type="button" role="menuitem" title="中英文转换" aria-label="中英文转换" @click="runSlideMagicAction(index, 'translate')">文 中英文转换</button>
                        <button type="button" role="menuitem" title="精简内容" aria-label="精简内容" @click="runSlideMagicAction(index, 'simplify')">↓ 精简内容</button>
                      </view>
                      <text>图片</text>
                      <view class="ppt-magic-grid">
                        <button type="button" role="menuitem" title="图文增强" aria-label="图文增强" @click="runSlideMagicAction(index, 'visual')">▧ 图文增强</button>
                        <button type="button" role="menuitem" title="添加图片" aria-label="添加图片" @click="runSlideMagicAction(index, 'image')">＋ 添加图片</button>
                      </view>
                    </view>
                  </view>
                </view>
                <button type="button" class="ppt-slide-insert after" title="在后方插入幻灯片" aria-label="在后方插入幻灯片" @click="insertEditorSlide(index, 'after')">＋</button>
              </view>
            </scroll-view>

            <aside class="ppt-editor-panel">
              <nav class="ppt-tool-rail" aria-label="演示编辑工具">
                <button v-for="tool in editorTools" :key="tool.value" type="button" :class="{ active: editorPanel === tool.value }" :title="tool.label" :aria-label="tool.label" :aria-pressed="editorPanel === tool.value" @click="handleEditorTool(tool.value)">{{ tool.icon }}<text>{{ tool.label }}</text></button>
              </nav>
              <view class="ppt-panel-content">
                <view class="ppt-panel-title">
                  <text>{{ activeEditorTool.label }}</text>
                  <button type="button" title="关闭面板" aria-label="关闭面板" @click="editorPanel = 'empty'">×</button>
                </view>
                <view v-if="editorPanel === 'text'" class="ppt-panel-cards">
                  <button type="button" title="插入大标题" aria-label="插入大标题" @click="appendPointToActiveSlide('新增大标题模块')">T 大标题</button>
                  <button type="button" title="插入一级标题" aria-label="插入一级标题" @click="appendPointToActiveSlide('新增一级标题模块')">H1 一级标题</button>
                  <button type="button" title="插入无序列表" aria-label="插入无序列表" @click="appendPointToActiveSlide('新增列表要点')">UL 无序列表</button>
                </view>
                <view v-else-if="editorPanel === 'elements'" class="ppt-panel-cards">
                  <button type="button" title="插入重点卡片" aria-label="插入重点卡片" @click="appendPointToActiveSlide('重点卡片')">▣ 重点卡片</button>
                  <button type="button" title="插入对比模块" aria-label="插入对比模块" @click="appendPointToActiveSlide('对比模块')">⇄ 对比模块</button>
                </view>
                <view v-else-if="editorPanel === 'charts'" class="ppt-panel-cards">
                  <button type="button" title="插入柱状图" aria-label="插入柱状图" @click="appendPointToActiveSlide('柱状图占位')">▥ 柱状图</button>
                  <button type="button" title="插入趋势图" aria-label="插入趋势图" @click="appendPointToActiveSlide('趋势图占位')">⌁ 趋势图</button>
                </view>
                <view v-else-if="editorPanel === 'diagrams'" class="ppt-panel-cards">
                  <button type="button" title="插入流程图" aria-label="插入流程图" @click="appendPointToActiveSlide('流程图占位')">◇ 流程图</button>
                  <button type="button" title="插入时间线" aria-label="插入时间线" @click="appendPointToActiveSlide('时间线占位')">↦ 时间线</button>
                </view>
                <view v-else-if="editorPanel === 'images'" class="ppt-visual-settings">
                  <text class="ppt-visual-label">视觉类型</text>
                  <view class="ppt-visual-options">
                    <button v-for="item in visualTypeOptions" :key="item.value" type="button" :class="{ active: visualType === item.value }" @click="visualType = item.value">{{ item.label }}</button>
                  </view>
                  <text class="ppt-visual-label">构图</text>
                  <view class="ppt-visual-options">
                    <button v-for="item in visualCompositionOptions" :key="item.value" type="button" :class="{ active: visualComposition === item.value }" @click="visualComposition = item.value">{{ item.label }}</button>
                  </view>
                  <label class="ppt-visual-instruction">
                    <text>补充要求（不会修改正文）</text>
                    <input v-model="visualInstruction" placeholder="例如：更简洁、更具科技感" />
                  </label>
                  <view class="ppt-visual-rules">
                    <text>✓ 图片内禁止文字</text>
                    <text>✓ 保留正文留白</text>
                    <text>✓ 继承整套 PPT 风格</text>
                  </view>
                  <button type="button" class="ppt-visual-primary" :disabled="visualBusy" @click="regenerateActiveSlideVisual">{{ visualBusy ? "正在生成…" : "重新生成本页配图" }}</button>
                  <button type="button" :disabled="visualBusy" @click="regenerateActiveSlideVisual">更换视觉类型</button>
                  <button type="button" :disabled="visualBusy || !activeEditorSlide.imageUrl" @click="deleteActiveSlideVisual">删除配图</button>
                  <view v-if="activeEditorSlide.visualHistory?.length" class="ppt-visual-history">
                    <text class="ppt-visual-label">历史配图</text>
                    <scroll-view scroll-x class="ppt-visual-history-list">
                      <button
                        v-for="asset in [...activeEditorSlide.visualHistory].reverse()"
                        :key="`${asset.createdAt}-${asset.url}`"
                        type="button"
                        :disabled="visualBusy"
                        @click="restoreActiveSlideVisual(asset.createdAt, asset.url, asset.storageRef)"
                      >
                        <image :src="asset.url" mode="aspectFill" />
                        <text>{{ formatVisualTime(asset.createdAt) }}</text>
                      </button>
                    </scroll-view>
                  </view>
                  <text v-if="visualMessage" class="ppt-visual-message">{{ visualMessage }}</text>
                </view>
                <view v-else-if="editorPanel === 'embed'" class="ppt-panel-cards">
                  <input v-model="embedUrl" title="嵌入链接" aria-label="嵌入链接" placeholder="粘贴网页、视频或看板链接" />
                  <button type="button" title="添加嵌入链接" aria-label="添加嵌入链接" @click="appendPointToActiveSlide(embedUrl || '嵌入链接占位')">添加嵌入</button>
                </view>
                <view v-else class="ppt-panel-empty">
                  <text>选择一个工具</text>
                  <text>点击左侧图标打开文本、元素、图表、图示或媒体嵌入面板。</text>
                </view>
                <view class="ppt-panel-bottom">
                  <button type="button" title="选择缩放比例" aria-label="选择缩放比例" @click="showZoomMenu = !showZoomMenu">{{ Math.round(editorZoom * 100) }}%</button>
                  <view v-if="showZoomMenu" class="ppt-zoom-menu" role="menu" aria-label="缩放菜单">
                    <button v-for="zoom in editorZoomLevels" :key="zoom" type="button" role="menuitemradio" :aria-checked="editorZoom === zoom" :title="`缩放到 ${Math.round(zoom * 100)}%`" :aria-label="`缩放到 ${Math.round(zoom * 100)}%`" @click="editorZoom = zoom">{{ Math.round(zoom * 100) }}%</button>
                  </view>
                  <button type="button" title="打开帮助菜单" aria-label="打开帮助菜单" @click="showHelpMenu = !showHelpMenu">?</button>
                  <view v-if="showHelpMenu" class="ppt-help-menu" role="menu" aria-label="帮助菜单">
                    <button type="button" role="menuitem" title="打开键盘快捷键" aria-label="打开键盘快捷键" @click="showKeyboardShortcuts">键盘快捷键</button>
                    <button type="button" role="menuitem" title="打开帮助中心" aria-label="打开帮助中心" @click="showPptHelpCenter">帮助中心</button>
                  </view>
                </view>
              </view>
            </aside>
          </view>
        </template>
      </view>
    </template>

    <template v-else>
    <view class="ppt-page-head">
      <view>
        <text class="ppt-kicker">AI PRESENTATION</text>
        <text class="ppt-page-title">PPT文档生成</text>
      </view>
      <view class="ppt-head-meta">
        <text>{{ history.length }} 条记录</text>
        <text>{{ themeLabel(theme) }}</text>
      </view>
    </view>

    <view class="ppt-hero">
      <view class="ppt-hero-copy">
        <text class="ppt-hero-title">把一个主题生成一份演示文稿</text>
        <text class="ppt-hero-subtitle">主题、页数、语言、联网搜索与风格独立配置。</text>
      </view>
      <view class="ppt-hero-preview">
        <view class="ppt-preview-slide main">
          <text></text>
          <text></text>
          <text></text>
        </view>
        <view class="ppt-preview-stack">
          <view></view>
          <view></view>
          <view></view>
        </view>
      </view>
    </view>

    <view class="ppt-workspace-grid">
      <view class="ppt-form-card">
        <label class="ppt-field">
          <view class="ppt-field-head">
            <text>提示词</text>
            <text>{{ promptLength }}/500</text>
          </view>
          <textarea
            :key="promptHydrationKey"
            v-model="prompt"
            class="ppt-prompt-textarea"
            maxlength="500"
            placeholder="请输入你想生成的PPT主题，例如：AI赋能企业营销增长方案"
          />
        </label>

        <view class="ppt-control-group">
          <view class="ppt-control-head">
            <text>PPT页数</text>
            <text>slideCount</text>
          </view>
          <view class="ppt-segment-row">
            <button
              v-for="count in slideCountOptions"
              :key="count"
              type="button"
              :class="['ppt-chip', { active: slideCount === count }]"
              @click="slideCount = count"
            >
              {{ count }}页
            </button>
          </view>
        </view>

        <view class="ppt-control-grid">
          <view class="ppt-control-group">
            <view class="ppt-control-head">
              <text>语言</text>
              <text>language</text>
            </view>
            <view class="ppt-segment-row">
              <button
                v-for="item in languageOptions"
                :key="item.value"
                type="button"
                :class="['ppt-chip', { active: language === item.value }]"
                @click="language = item.value"
              >
                {{ item.label }}
              </button>
            </view>
          </view>

          <view class="ppt-search-control">
            <view>
              <text>联网搜索</text>
              <text>enableWebSearch</text>
            </view>
            <button
              type="button"
              :class="['ppt-switch', { checked: enableWebSearch }]"
              @click="enableWebSearch = !enableWebSearch"
            >
              <text></text>
            </button>
          </view>
        </view>

        <view class="ppt-control-group">
          <view class="ppt-control-head">
            <text>风格/主题</text>
            <text>theme</text>
          </view>
          <view class="ppt-theme-grid">
            <button
              v-for="item in themeOptions"
              :key="item.value"
              type="button"
              :class="['ppt-theme-card', { active: theme === item.value }]"
              @click="theme = item.value"
            >
              <text>{{ item.label }}</text>
              <text>{{ item.note }}</text>
            </button>
          </view>
        </view>

        <button
          type="button"
          :disabled="isGenerateDisabled"
          :class="['ppt-generate-button', { loading: generating }]"
          @click="generatePpt"
        >
          {{ generating ? "正在生成PPT，请稍候..." : "生成PPT" }}
        </button>

        <view v-if="generationError" class="ppt-error-state">
          {{ generationError }}
        </view>
      </view>

      <view class="ppt-side-panel">
        <view class="ppt-panel-head">
          <text>示例主题推荐</text>
          <text>点击填充</text>
        </view>
        <view class="ppt-example-grid">
          <button
            v-for="example in examplePrompts"
            :key="example"
            type="button"
            class="ppt-example-card"
            @click="applyExample(example)"
          >
            {{ example }}
          </button>
        </view>
      </view>
    </view>

    <view v-if="activeTask && isTaskRunning(activeTask.status)" class="ppt-progress-panel">
      <view class="ppt-progress-head">
        <text>{{ activeTask.title }}</text>
        <text>{{ statusLabel(activeTask.status) }}</text>
      </view>
      <view class="ppt-progress-track">
        <view :style="{ width: progressPercent }"></view>
      </view>
      <text class="ppt-progress-copy">正在生成PPT，请稍候...</text>
    </view>

    <view class="ppt-result-layout">
      <view class="ppt-result-card">
        <view class="ppt-panel-head">
          <text>生成结果预览</text>
          <text v-if="currentResult">{{ formatDate(currentResult.createdAt) }}</text>
        </view>
        <view v-if="currentResult" class="ppt-result-body">
          <view class="ppt-result-preview">
            <view class="ppt-result-slide">
              <text>{{ currentResult.title }}</text>
              <text>{{ currentResult.slideCount || slideCount }} 页 · {{ languageLabel(currentResult.language || language) }}</text>
              <view>
                <text></text>
                <text></text>
                <text></text>
              </view>
            </view>
          </view>
          <view class="ppt-result-info">
            <text class="ppt-result-title">{{ currentResult.title }}</text>
            <view class="ppt-result-meta">
              <text>{{ currentResult.slideCount || slideCount }} 页</text>
              <text>{{ themeLabel(currentResult.theme || theme) }}</text>
              <text :class="['ppt-status', currentResult.status]">{{ statusLabel(currentResult.status) }}</text>
            </view>
            <text v-if="currentResult.errorMessage" class="ppt-result-error">{{ currentResult.errorMessage }}</text>
            <text v-else class="ppt-result-time">生成时间：{{ formatDate(currentResult.createdAt) }}</text>
            <view class="ppt-action-row">
              <button type="button" :disabled="currentResult.status !== 'success'" @click="previewTask(currentResult)">预览</button>
              <button type="button" :disabled="!currentResult.pptUrl" @click="downloadTask(currentResult)">下载PPT</button>
              <button type="button" :disabled="generating" @click="regenerateTask(currentResult)">重新生成</button>
              <button type="button" class="danger" @click="removeTask(currentResult)">删除</button>
            </view>
            <text v-if="operationMessage" class="ppt-operation-message">{{ operationMessage }}</text>
          </view>
        </view>
        <view v-else class="ppt-empty-state">
          暂无生成结果
        </view>
      </view>

      <view class="ppt-history-card">
        <view class="ppt-panel-head">
          <text>最近生成记录</text>
          <text>{{ historyLoading ? "加载中" : `${history.length} 条` }}</text>
        </view>
        <view v-if="history.length" class="ppt-history-list">
          <view v-for="item in history" :key="item.taskId" class="ppt-history-row">
            <view>
              <text class="ppt-history-title">{{ item.title }}</text>
              <text class="ppt-history-subtitle">
                {{ item.slideCount || 5 }} 页 · {{ languageLabel(item.language || "zh") }} · {{ themeLabel(item.theme || "business") }}
              </text>
            </view>
            <view class="ppt-history-meta">
              <text>{{ formatDate(item.createdAt) }}</text>
              <text :class="['ppt-status', item.status]">{{ statusLabel(item.status) }}</text>
            </view>
            <view class="ppt-history-actions">
              <button type="button" @click="previewTask(item)">预览</button>
              <button type="button" :disabled="!item.pptUrl" @click="downloadTask(item)">下载PPT</button>
              <button type="button" :disabled="generating" @click="regenerateTask(item)">重新生成</button>
              <button type="button" class="danger" @click="removeTask(item)">删除</button>
            </view>
          </view>
        </view>
        <view v-else class="ppt-empty-state">
          暂无生成记录
        </view>
      </view>
    </view>
    </template>
  </view>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from "vue";
import { downloadTemporaryFile } from "../api/files";
import { authStorage } from "../api/client";
import { requireAuth } from "../features/auth/gate";
import AiGeneratedContentNotice from "./compliance/AiGeneratedContentNotice.vue";
import {
  createPptGenerationTask,
  deletePptSlideVisual,
  deletePptTask,
  getPptGenerationTask,
  listPptHistory,
  regeneratePptSlideVisual,
  restorePptSlideVisual,
  requestPptDownload,
  type PptGenerateRequest,
  type PptHistoryItem,
  type PptLanguage,
  type PptSlide,
  type PptTaskStatus,
  type PptTheme
} from "../api/ppt";

const props = withDefaults(defineProps<{
  initialTaskId?: string;
}>(), {
  initialTaskId: ""
});

const slideCountOptions = [5, 8, 10, 15, 20];
const languageOptions: Array<{ label: string; value: PptLanguage }> = [
  { label: "中文", value: "zh" },
  { label: "英文", value: "en" }
];
const themeOptions: Array<{ label: string; value: PptTheme; note: string }> = [
  { label: "商务简约", value: "business", note: "清晰稳重" },
  { label: "科技蓝", value: "techBlue", note: "产品发布" },
  { label: "教育培训", value: "education", note: "课程讲义" },
  { label: "营销方案", value: "marketing", note: "增长提案" },
  { label: "项目路演", value: "pitch", note: "融资汇报" },
  { label: "医疗科普", value: "medical", note: "患教内容" }
];
const examplePrompts = [
  "AI赋能企业营销增长方案",
  "灵活用工平台介绍",
  "企业数字员工解决方案",
  "GEO品牌曝光方案",
  "短视频矩阵运营方案",
  "糖尿病饮食运动患教"
];
type PptViewMode = "home" | "editor";
type EditorPresentationMode = "edit" | "present";
type EditorLayout = "cover" | "section" | "content" | "imageText" | "summary";
type EditorAlign = "left" | "center" | "right";
type EditorWidth = "compact" | "standard" | "wide";
type EditorTool = "text" | "elements" | "charts" | "diagrams" | "embed" | "record" | "theme" | "settings" | "images" | "empty";
type SlideMagicAction = "layout" | "writing" | "spelling" | "translate" | "simplify" | "visual" | "image";
interface EditorSlide {
  id: string;
  title: string;
  content: string;
  points: string[];
  layout: EditorLayout;
  background: string;
  align: EditorAlign;
  width: EditorWidth;
  imageUrl: string;
  slideType: string;
  visualPlan?: PptSlide["visualPlan"];
  visualHistory?: PptSlide["visualHistory"];
  visualStatus?: string;
  visualError?: string;
}
interface EditorSnapshot {
  title: string;
  slides: EditorSlide[];
  activeSlideIndex: number;
}
const editorLayoutOptions: Array<{ label: string; short: string; value: EditorLayout }> = [
  { label: "封面", short: "封", value: "cover" },
  { label: "章节", short: "章", value: "section" },
  { label: "正文", short: "文", value: "content" },
  { label: "图文", short: "图", value: "imageText" },
  { label: "总结", short: "总", value: "summary" }
];
const editorColorOptions = [
  { label: "深黑", value: "#050505" },
  { label: "雾白", value: "#eaf3ff" },
  { label: "科技蓝", value: "linear-gradient(135deg, #07111f, #2563eb)" },
  { label: "增长绿", value: "linear-gradient(135deg, #052e16, #16a34a)" },
  { label: "路演紫", value: "linear-gradient(135deg, #111827, #7c3aed)" },
  { label: "暖橙", value: "linear-gradient(135deg, #fff7ed, #fb923c)" }
];
const editorAlignOptions: Array<{ label: string; short: string; value: EditorAlign }> = [
  { label: "左对齐", short: "左", value: "left" },
  { label: "居中", short: "中", value: "center" },
  { label: "右对齐", short: "右", value: "right" }
];
const editorWidthOptions: Array<{ label: string; value: EditorWidth; width: number }> = [
  { label: "M", value: "compact", width: 760 },
  { label: "L", value: "wide", width: 1020 }
];
const editorTools: Array<{ label: string; value: EditorTool; icon: string }> = [
  { label: "文本", value: "text", icon: "T" },
  { label: "元素", value: "elements", icon: "▦" },
  { label: "图表", value: "charts", icon: "▥" },
  { label: "图示", value: "diagrams", icon: "◇" },
  { label: "视觉", value: "images", icon: "▧" },
  { label: "嵌入", value: "embed", icon: "⌁" },
  { label: "录制", value: "record", icon: "▣" }
];
const editorZoomLevels = [1.8, 1.5, 1.2, 1, 0.9, 0.8, 0.6];
const visualTypeOptions = [
  { label: "场景图", value: "scene" },
  { label: "概念插画", value: "illustration" },
  { label: "产品展示", value: "product" },
  { label: "企业办公", value: "office" },
  { label: "图标", value: "icon" },
  { label: "图表", value: "chart" },
  { label: "流程图", value: "diagram" },
  { label: "不需要视觉素材", value: "none" }
];
const visualCompositionOptions = [
  { label: "图片在左", value: "image_left" },
  { label: "图片在右", value: "image_right" },
  { label: "通栏图片", value: "full_width" },
  { label: "背景图片", value: "background" },
  { label: "卡片图片", value: "card" }
];

const pptGuestDraftKey = "zhiqiyun:web:ppt-guest-draft";
function readInitialPptDraft(): Partial<PptGenerateRequest> {
  try {
    const raw = typeof window !== "undefined" ? window.localStorage.getItem(pptGuestDraftKey) : "";
    const value = raw ? JSON.parse(raw) : uni.getStorageSync(pptGuestDraftKey);
    return value && typeof value === "object" ? value as Partial<PptGenerateRequest> : {};
  } catch {
    return {};
  }
}
const initialPptDraft = readInitialPptDraft();
const prompt = ref(String(initialPptDraft.prompt || ""));
const promptHydrationKey = ref(0);
const slideCount = ref(slideCountOptions.includes(Number(initialPptDraft.slideCount)) ? Number(initialPptDraft.slideCount) : 5);
const language = ref<PptLanguage>(initialPptDraft.language === "en" ? "en" : "zh");
const enableWebSearch = ref(Boolean(initialPptDraft.enableWebSearch));
const theme = ref<PptTheme>(themeOptions.some(item => item.value === initialPptDraft.theme) ? initialPptDraft.theme as PptTheme : "business");
const generating = ref(false);
const generationError = ref("");
const operationMessage = ref("");
const historyLoading = ref(false);
const history = ref<PptHistoryItem[]>([]);
const activeTask = ref<PptHistoryItem | null>(null);
const previewRecord = ref<PptHistoryItem | null>(null);
const currentMode = ref<PptViewMode>("home");
const presentationMode = ref<EditorPresentationMode>("edit");
const recordingMode = ref(false);
const editorTitle = ref("无标题演示文稿");
const editorSlides = ref<EditorSlide[]>([]);
const activeSlideIndex = ref(0);
const showEditorMenu = ref(false);
const showZoomMenu = ref(false);
const showHelpMenu = ref(false);
const slideMoreMenuIndex = ref<number | null>(null);
const slidePaletteMenuIndex = ref<number | null>(null);
const slideMagicMenuIndex = ref<number | null>(null);
const slideMagicPrompt = ref("");
const editorPanel = ref<EditorTool>("text");
const editorZoom = ref(0.92);
const embedUrl = ref("");
const editorUndoStack = ref<EditorSnapshot[]>([]);
const editorRedoStack = ref<EditorSnapshot[]>([]);
const editorTitleInputRef = ref<HTMLInputElement | null>(null);
const visualType = ref("illustration");
const visualComposition = ref("image_right");
const visualInstruction = ref("");
const visualBusy = ref(false);
const visualMessage = ref("");

const promptLength = computed(() => prompt.value.length);
const isGenerateDisabled = computed(() => generating.value || !prompt.value.trim());
const currentResult = computed(() => activeTask.value || previewRecord.value || history.value[0] || null);
const activeEditorSlide = computed(() => editorSlides.value[activeSlideIndex.value] || createEditorSlide("无标题演示文稿", 0, 1));
const activeEditorTool = computed(() => editorTools.find(tool => tool.value === editorPanel.value) || { label: "选择一个工具", value: "empty" as EditorTool, icon: "" });
const canUndoEditor = computed(() => editorUndoStack.value.length > 0);
const canRedoEditor = computed(() => editorRedoStack.value.length > 0);
const progressPercent = computed(() => {
  const status = activeTask.value?.status || "pending";
  if (status === "success") return "100%";
  if (status === "processing") return "68%";
  if (status === "failed") return "100%";
  return "34%";
});

onMounted(() => {
  restoreGuestDraft();
  setTimeout(restoreGuestDraft, 0);
  void initializePptWorkspace();
});

watch([prompt, slideCount, language, theme, enableWebSearch], persistGuestDraft);

function currentPptRoute() {
  return typeof window !== "undefined" ? `${window.location.pathname}${window.location.search}${window.location.hash}` : "/app/ppt";
}

function currentPptRequest(): PptGenerateRequest {
  return {
    prompt: prompt.value.trim(),
    slideCount: slideCount.value,
    language: language.value,
    theme: theme.value,
    enableWebSearch: enableWebSearch.value,
  };
}

function persistGuestDraft() {
  try {
    if (typeof window !== "undefined") {
      window.localStorage.setItem(pptGuestDraftKey, JSON.stringify(currentPptRequest()));
      return;
    }
    uni.setStorageSync(pptGuestDraftKey, currentPptRequest());
  } catch {
    /* local draft is best effort */
  }
}

function restoreGuestDraft() {
  try {
    const webDraft = typeof window !== "undefined" ? window.localStorage.getItem(pptGuestDraftKey) : "";
    const draft = (webDraft ? JSON.parse(webDraft) : uni.getStorageSync(pptGuestDraftKey)) as Partial<PptGenerateRequest> | "";
    if (!draft || typeof draft !== "object") return;
    prompt.value = String(draft.prompt || "");
    promptHydrationKey.value += 1;
    slideCount.value = slideCountOptions.includes(Number(draft.slideCount)) ? Number(draft.slideCount) : slideCount.value;
    if (draft.language === "zh" || draft.language === "en") language.value = draft.language;
    if (themeOptions.some(item => item.value === draft.theme)) theme.value = draft.theme as PptTheme;
    enableWebSearch.value = Boolean(draft.enableWebSearch);
  } catch {
    /* an invalid draft must not block the public editor */
  }
}

async function initializePptWorkspace() {
  if (!authStorage.getToken()) return;
  await loadHistory();
  const taskId = props.initialTaskId.trim();
  if (!taskId) return;
  try {
    const task = await getPptGenerationTask(taskId);
    previewRecord.value = task;
    activeTask.value = task;
    upsertHistory(task);
    if (task.status === "success") {
      openEditorFromTask(task);
      operationMessage.value = "已打开 PPT 移动编辑页";
    } else {
      operationMessage.value = task.status === "failed" ? task.errorMessage || "PPT 生成失败" : "PPT 仍在生成中";
    }
  } catch (error) {
    const message = errorMessage(error, "PPT 任务加载失败");
    operationMessage.value = message;
    uni.showToast({ title: message, icon: "none" });
  }
}

async function loadHistory() {
  historyLoading.value = true;
  try {
    history.value = await listPptHistory();
  } catch (error) {
    const message = errorMessage(error, "PPT历史记录加载失败");
    operationMessage.value = message;
    uni.showToast({ title: message, icon: "none" });
  } finally {
    historyLoading.value = false;
  }
}

function applyExample(example: string) {
  prompt.value = example;
  operationMessage.value = "";
}

async function generatePpt() {
  if (isGenerateDisabled.value) return;
  const request = currentPptRequest();
  persistGuestDraft();
  if (!authStorage.getToken()) {
    await requireAuth({
      action: "generate_ppt",
      route: currentPptRoute(),
      payload: request as unknown as Record<string, unknown>,
      resume: () => generatePpt(),
    });
    return;
  }
  generating.value = true;
  generationError.value = "";
  operationMessage.value = "";
  try {
    const created = await createPptGenerationTask(request);
    if (typeof window !== "undefined") window.localStorage.removeItem(pptGuestDraftKey);
    else uni.removeStorageSync(pptGuestDraftKey);
    const pending = taskFromRequest(created.taskId, created.status, request);
    activeTask.value = pending;
    previewRecord.value = pending;
    upsertHistory(pending);
    await pollTask(created.taskId, request);
    if (activeTask.value?.status === "success") {
      openEditorFromTask(activeTask.value);
    }
    await loadHistory();
  } catch (error) {
    const failed = taskFromRequest(`ppt_failed_${Date.now()}`, "failed", request, errorMessage(error));
    activeTask.value = failed;
    previewRecord.value = failed;
    upsertHistory(failed);
    generationError.value = failed.errorMessage;
  } finally {
    generating.value = false;
  }
}

async function pollTask(taskId: string, request: PptGenerateRequest) {
  for (let attempt = 0; attempt < 5; attempt += 1) {
    await wait(750);
    const task = normalizeTask(await getPptGenerationTask(taskId), request);
    activeTask.value = task;
    previewRecord.value = task;
    upsertHistory(task);
    if (task.status === "success" || task.status === "failed") return;
  }
}

function previewTask(item: PptHistoryItem) {
  previewRecord.value = item;
  activeTask.value = item.status === "processing" || item.status === "pending" ? item : activeTask.value?.taskId === item.taskId ? item : activeTask.value;
  if (item.status === "success") {
    openEditorFromTask(item);
    operationMessage.value = "已打开 PPT 编辑页";
    return;
  }
  operationMessage.value = "生成完成后可预览";
}

async function downloadTask(item: PptHistoryItem) {
  if (!authStorage.getToken()) {
    await requireAuth({
      action: "download_work",
      route: currentPptRoute(),
      payload: { taskId: item.taskId },
      resume: () => downloadTask(item),
    });
    return;
  }
  operationMessage.value = "";
  try {
    const result = await requestPptDownload(item.taskId);
    await openDownloadUrl(result.url);
  } catch (error) {
    const message = errorMessage(error);
    operationMessage.value = message;
    uni.showToast({ title: message, icon: "none" });
  }
}

function regenerateTask(item: PptHistoryItem) {
  prompt.value = item.prompt || item.title;
  slideCount.value = item.slideCount || 5;
  language.value = item.language || "zh";
  theme.value = item.theme || "business";
  enableWebSearch.value = Boolean(item.enableWebSearch);
  void generatePpt();
}

async function removeTask(item: PptHistoryItem) {
  const confirmed = await confirmAction("删除生成记录", "删除后该 PPT 任务记录将从后台移除，是否继续？");
  if (!confirmed) return;
  try {
    await deletePptTask(item.taskId);
  } catch (error) {
    const message = errorMessage(error, "删除失败，请稍后重试");
    operationMessage.value = message;
    uni.showToast({ title: message, icon: "none" });
    return;
  }
  history.value = history.value.filter(record => record.taskId !== item.taskId);
  if (activeTask.value?.taskId === item.taskId) activeTask.value = null;
  if (previewRecord.value?.taskId === item.taskId) previewRecord.value = null;
  operationMessage.value = "记录已删除";
}

function upsertHistory(item: PptHistoryItem) {
  history.value = [item, ...history.value.filter(record => record.taskId !== item.taskId)].slice(0, 12);
}

function taskFromRequest(taskId: string, status: PptTaskStatus, request: PptGenerateRequest, error = ""): PptHistoryItem {
  const now = new Date().toISOString();
  return {
    taskId,
    status,
    title: request.prompt.slice(0, 60),
    prompt: request.prompt,
    slideCount: request.slideCount,
    language: request.language,
    theme: request.theme,
    enableWebSearch: request.enableWebSearch,
    pptUrl: "",
    pdfUrl: "",
    errorMessage: error,
    createdAt: now,
    updatedAt: now
  };
}

function normalizeTask(task: PptHistoryItem, request: PptGenerateRequest): PptHistoryItem {
  return {
    ...task,
    title: task.title || request.prompt.slice(0, 60),
    prompt: task.prompt || request.prompt,
    slideCount: task.slideCount || request.slideCount,
    language: task.language || request.language,
    theme: task.theme || request.theme,
    enableWebSearch: task.enableWebSearch ?? request.enableWebSearch,
    pptUrl: task.pptUrl || "",
    pdfUrl: task.pdfUrl || "",
    errorMessage: task.errorMessage || ""
  };
}

function createEditorSlide(title: string, index: number, total: number): EditorSlide {
  const layout = editorLayoutOptions[index % editorLayoutOptions.length]?.value || "content";
  return {
    id: `slide_${Date.now()}_${index}_${Math.random().toString(36).slice(2, 6)}`,
    title: index === 0 ? title : `${title} · 第${index + 1}部分`,
    content: index === 0 ? "封面页，突出主题和演示定位。" : "围绕主题提炼核心观点，形成适合演示的页面内容。",
    points: index === total - 1 ? ["收束主要观点", "给出下一步行动建议"] : ["明确页面目标", "提炼关键论据", "形成适合演示的表达"],
    layout,
    background: "#eaf3ff",
    align: "left",
    width: "standard",
    imageUrl: "",
    slideType: index === 0 ? "cover" : "text_image"
  };
}

function openEditorFromTask(item: PptHistoryItem) {
  const total = Math.max(1, Math.min(item.slideCount || slideCount.value || 5, 20));
  editorTitle.value = item.title || item.prompt || "无标题演示文稿";
  editorSlides.value = item.slides?.length
    ? item.slides.map((slide, index) => editorSlideFromTask(slide, index))
    : Array.from({ length: total }, (_, index) => createEditorSlide(editorTitle.value, index, total));
  activeSlideIndex.value = 0;
  editorUndoStack.value = [];
  editorRedoStack.value = [];
  presentationMode.value = "edit";
  recordingMode.value = false;
  currentMode.value = "editor";
  closeEditorFloatingMenus();
  syncActiveVisualSettings();
}

function editorSlideFromTask(slide: PptSlide, index: number): EditorSlide {
  const fallback = createEditorSlide(slide.title || editorTitle.value, index, Math.max(1, activeTask.value?.slideCount || 1));
  const layout = editorLayoutOptions.some(item => item.value === slide.layout) ? slide.layout as EditorLayout : fallback.layout;
  return {
    ...fallback,
    id: slide.id,
    title: slide.title,
    content: slide.content,
    points: Array.isArray(slide.bulletPoints) ? [...slide.bulletPoints] : [],
    layout,
    imageUrl: slide.imageUrl || "",
    slideType: slide.slideType || "text_image",
    visualPlan: slide.visualPlan,
    visualHistory: Array.isArray(slide.visualHistory) ? [...slide.visualHistory] : [],
    visualStatus: slide.visualStatus,
    visualError: slide.visualError
  };
}

function cloneEditorSlides(slides: EditorSlide[]) {
  return slides.map(slide => ({ ...slide, points: [...slide.points], visualHistory: slide.visualHistory ? [...slide.visualHistory] : [] }));
}

function createEditorSnapshot(): EditorSnapshot {
  return {
    title: editorTitle.value,
    slides: cloneEditorSlides(editorSlides.value),
    activeSlideIndex: activeSlideIndex.value
  };
}

function restoreEditorSnapshot(snapshot: EditorSnapshot) {
  editorTitle.value = snapshot.title;
  editorSlides.value = cloneEditorSlides(snapshot.slides);
  activeSlideIndex.value = Math.min(Math.max(snapshot.activeSlideIndex, 0), Math.max(editorSlides.value.length - 1, 0));
  closeEditorFloatingMenus();
}

function rememberEditorSnapshot() {
  editorUndoStack.value = [...editorUndoStack.value.slice(-19), createEditorSnapshot()];
  editorRedoStack.value = [];
}

function undoEditor() {
  if (!canUndoEditor.value) return;
  const previous = editorUndoStack.value[editorUndoStack.value.length - 1];
  editorUndoStack.value = editorUndoStack.value.slice(0, -1);
  editorRedoStack.value = [...editorRedoStack.value, createEditorSnapshot()];
  restoreEditorSnapshot(previous);
}

function redoEditor() {
  if (!canRedoEditor.value) return;
  const next = editorRedoStack.value[editorRedoStack.value.length - 1];
  editorRedoStack.value = editorRedoStack.value.slice(0, -1);
  editorUndoStack.value = [...editorUndoStack.value, createEditorSnapshot()];
  restoreEditorSnapshot(next);
}

function markEditorSaved() {
  operationMessage.value = "编辑已保存到本地预览状态";
}

function backToPptHome() {
  currentMode.value = "home";
  presentationMode.value = "edit";
  recordingMode.value = false;
  showEditorMenu.value = false;
}

function toggleEditorMenu() {
  const next = !showEditorMenu.value;
  closeEditorFloatingMenus();
  showEditorMenu.value = next;
}

async function focusEditorTitle() {
  showEditorMenu.value = false;
  await nextTick();
  editorTitleInputRef.value?.focus();
}

function createBlankEditorDeck() {
  rememberEditorSnapshot();
  editorTitle.value = "无标题演示文稿";
  editorSlides.value = [createEditorSlide("无标题演示文稿", 0, 1)];
  activeSlideIndex.value = 0;
  showEditorMenu.value = false;
}

function duplicateEditorDeck() {
  rememberEditorSnapshot();
  editorTitle.value = `${editorTitle.value} 副本`;
  editorSlides.value = cloneEditorSlides(editorSlides.value).map((slide, index) => ({ ...slide, id: `${slide.id}_deck_copy_${Date.now()}_${index}` }));
  showEditorMenu.value = false;
}

function selectEditorSlide(index: number) {
  if (!editorSlides.value.length) return;
  activeSlideIndex.value = Math.min(Math.max(index, 0), editorSlides.value.length - 1);
  syncActiveVisualSettings();
  closeEditorFloatingMenus();
}

function closeEditorFloatingMenus() {
  slideMoreMenuIndex.value = null;
  slidePaletteMenuIndex.value = null;
  slideMagicMenuIndex.value = null;
  slideMagicPrompt.value = "";
  showZoomMenu.value = false;
  showHelpMenu.value = false;
}

function toggleSlideMoreMenu(index: number) {
  activeSlideIndex.value = index;
  slideMoreMenuIndex.value = slideMoreMenuIndex.value === index ? null : index;
  slidePaletteMenuIndex.value = null;
  slideMagicMenuIndex.value = null;
}

function toggleSlidePaletteMenu(index: number) {
  activeSlideIndex.value = index;
  slidePaletteMenuIndex.value = slidePaletteMenuIndex.value === index ? null : index;
  slideMoreMenuIndex.value = null;
  slideMagicMenuIndex.value = null;
}

function toggleSlideMagicMenu(index: number) {
  activeSlideIndex.value = index;
  slideMagicMenuIndex.value = slideMagicMenuIndex.value === index ? null : index;
  slideMoreMenuIndex.value = null;
  slidePaletteMenuIndex.value = null;
  slideMagicPrompt.value = "";
}

function insertEditorSlide(index: number, position: "before" | "after") {
  rememberEditorSnapshot();
  const insertIndex = position === "before" ? index : index + 1;
  editorSlides.value.splice(insertIndex, 0, createEditorSlide(editorTitle.value, insertIndex, editorSlides.value.length + 1));
  activeSlideIndex.value = insertIndex;
  closeEditorFloatingMenus();
}

function duplicateEditorSlide(index: number) {
  const slide = editorSlides.value[index];
  if (!slide) return;
  rememberEditorSnapshot();
  editorSlides.value.splice(index + 1, 0, { ...slide, id: `${slide.id}_copy_${Date.now()}`, title: `${slide.title} 副本`, points: [...slide.points] });
  activeSlideIndex.value = index + 1;
  closeEditorFloatingMenus();
}

function deleteEditorSlide(index: number) {
  if (editorSlides.value.length <= 1) return;
  rememberEditorSnapshot();
  editorSlides.value.splice(index, 1);
  activeSlideIndex.value = Math.min(index, editorSlides.value.length - 1);
  closeEditorFloatingMenus();
}

function applyEditorLayout(index: number, layoutValue: EditorLayout) {
  const slide = editorSlides.value[index];
  if (!slide || slide.layout === layoutValue) return;
  rememberEditorSnapshot();
  slide.layout = layoutValue;
}

function applyEditorBackground(index: number, background: string) {
  const slide = editorSlides.value[index];
  if (!slide || slide.background === background) return;
  rememberEditorSnapshot();
  slide.background = background;
}

function applyEditorAlign(index: number, align: EditorAlign) {
  const slide = editorSlides.value[index];
  if (!slide || slide.align === align) return;
  rememberEditorSnapshot();
  slide.align = align;
}

function applyEditorWidth(index: number, width: EditorWidth) {
  const slide = editorSlides.value[index];
  if (!slide || slide.width === width) return;
  rememberEditorSnapshot();
  slide.width = width;
}

function runSlideMagicAction(index: number, action: SlideMagicAction) {
  const slide = editorSlides.value[index];
  if (!slide) return;
  rememberEditorSnapshot();
  if (action === "layout") {
    const currentIndex = editorLayoutOptions.findIndex(item => item.value === slide.layout);
    slide.layout = editorLayoutOptions[(currentIndex + 1 + editorLayoutOptions.length) % editorLayoutOptions.length].value;
  } else if (action === "writing") {
    slide.content = `${slide.content.replace(/。?$/, "。")}表达已收敛为更适合演示呈现的版本。`;
  } else if (action === "spelling") {
    slide.title = slide.title.replace(/\s+/g, " ").trim();
    slide.content = slide.content.replace(/\s+/g, " ").trim();
    slide.points = slide.points.map(point => point.replace(/\s+/g, " ").trim()).filter(Boolean);
  } else if (action === "translate") {
    slide.title = language.value === "en" ? `English version: ${slide.title}` : `${slide.title}（中英文版）`;
  } else if (action === "simplify") {
    slide.content = slide.content.slice(0, 60);
    slide.points = slide.points.slice(0, 3);
  } else if (action === "visual") {
    slide.layout = "imageText";
    slide.points = slide.points.slice(0, 3);
  } else {
    openEditorPanel("images");
  }
  closeEditorFloatingMenus();
}

function submitSlideMagicPrompt(index: number) {
  const promptValue = slideMagicPrompt.value.trim();
  const slide = editorSlides.value[index];
  if (!promptValue || !slide) return;
  rememberEditorSnapshot();
  slide.content = `按「${promptValue}」优化：${slide.content}`;
  closeEditorFloatingMenus();
}

function appendPointToActiveSlide(point: string) {
  if (!point.trim()) return;
  rememberEditorSnapshot();
  activeEditorSlide.value.points = [...activeEditorSlide.value.points, point.trim()];
}

function openEditorPanel(panel: EditorTool) {
  editorPanel.value = panel;
  if (panel === "images") syncActiveVisualSettings();
  showEditorMenu.value = false;
  closeEditorFloatingMenus();
}

function syncActiveVisualSettings() {
  const plan = activeEditorSlide.value.visualPlan;
  visualType.value = plan?.visualType || (activeEditorSlide.value.imageUrl ? "illustration" : "none");
  const composition = plan?.composition || "";
  if (composition.includes("left") && composition.includes("right")) {
    visualComposition.value = composition.indexOf("subject on the left") >= 0 ? "image_left" : "image_right";
  }
  visualInstruction.value = "";
  visualMessage.value = activeEditorSlide.value.visualError || "";
}

async function regenerateActiveSlideVisual() {
  if (visualBusy.value) return;
  const taskId = currentResult.value?.taskId;
  const slide = activeEditorSlide.value;
  if (!taskId || !slide?.id) {
    visualMessage.value = "当前页面尚未关联后端 PPT 任务";
    return;
  }
  visualBusy.value = true;
  visualMessage.value = "正在规划并生成本页视觉素材…";
  const original = { title: slide.title, content: slide.content, points: [...slide.points] };
  try {
    const response = await regeneratePptSlideVisual(taskId, slide.id, {
      visualType: visualType.value,
      style: "corporate_3d",
      composition: visualComposition.value,
      customInstruction: visualInstruction.value.trim(),
      keepCurrentContent: true
    });
    slide.imageUrl = response.slide.imageUrl || "";
    slide.slideType = response.slide.slideType || slide.slideType;
    slide.visualPlan = response.slide.visualPlan;
    slide.visualHistory = Array.isArray(response.slide.visualHistory) ? [...response.slide.visualHistory] : [];
    slide.visualStatus = response.slide.visualStatus || response.status;
    slide.visualError = response.slide.visualError || "";
    slide.title = original.title;
    slide.content = original.content;
    slide.points = original.points;
    visualMessage.value = response.slide.visualPlan?.imageRequired === false ? "已切换为非图片视觉方式" : "本页配图已更新";
  } catch (error) {
    visualMessage.value = errorMessage(error, "本页配图生成失败，已保留原图片");
  } finally {
    visualBusy.value = false;
  }
}

async function deleteActiveSlideVisual() {
  if (visualBusy.value) return;
  const taskId = currentResult.value?.taskId;
  const slide = activeEditorSlide.value;
  if (!taskId || !slide?.id) return;
  visualBusy.value = true;
  visualMessage.value = "正在删除本页配图…";
  try {
    const response = await deletePptSlideVisual(taskId, slide.id);
    slide.imageUrl = "";
    slide.visualPlan = response.slide.visualPlan;
    slide.visualHistory = Array.isArray(response.slide.visualHistory) ? [...response.slide.visualHistory] : [];
    slide.visualStatus = response.slide.visualStatus || response.status;
    slide.visualError = "";
    visualType.value = "none";
    visualMessage.value = "本页配图已删除";
  } catch (error) {
    visualMessage.value = errorMessage(error, "删除配图失败");
  } finally {
    visualBusy.value = false;
  }
}

async function restoreActiveSlideVisual(createdAt: string, url: string, storageRef?: string) {
  if (visualBusy.value || !createdAt || !url) return;
  const taskId = currentResult.value?.taskId;
  const slide = activeEditorSlide.value;
  if (!taskId || !slide?.id) return;
  visualBusy.value = true;
  visualMessage.value = "正在恢复历史配图…";
  try {
    const response = await restorePptSlideVisual(taskId, slide.id, createdAt, url, storageRef);
    const restored = response.slide;
    slide.imageUrl = restored.imageUrl || "";
    slide.visualPlan = restored.visualPlan;
    slide.visualHistory = Array.isArray(restored.visualHistory) ? [...restored.visualHistory] : [];
    slide.visualStatus = restored.visualStatus || response.status;
    slide.visualError = "";
    visualType.value = restored.visualPlan?.visualType || "illustration";
    visualMessage.value = "历史配图已恢复，正文保持不变";
  } catch (error) {
    visualMessage.value = errorMessage(error, "恢复历史配图失败");
  } finally {
    visualBusy.value = false;
  }
}

function formatVisualTime(value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "历史版本" : `${date.getMonth() + 1}/${date.getDate()} ${String(date.getHours()).padStart(2, "0")}:${String(date.getMinutes()).padStart(2, "0")}`;
}

function handleEditorTool(panel: EditorTool) {
  if (panel === "record") {
    enterPresentationMode(true);
    return;
  }
  openEditorPanel(panel);
}

function openShareSettings() {
  showEditorMenu.value = false;
  const task = currentResult.value;
  const shareText = task
    ? `${task.title || "知启云 PPT"}\n任务：${task.taskId}\n状态：${statusLabel(task.status)}`
    : `${editorTitle.value}\n共 ${editorSlides.value.length} 张幻灯片`;
  uni.setClipboardData({
    data: shareText,
    success: () => {
      operationMessage.value = "分享信息已复制";
      uni.showToast({ title: "已复制分享信息", icon: "success" });
    }
  });
}

async function exportEditorDeck() {
  const task = currentResult.value;
  if (!task?.taskId) {
    operationMessage.value = "当前演示文稿还没有后端任务，无法导出";
    uni.showToast({ title: operationMessage.value, icon: "none" });
    return;
  }
  await downloadTask(task);
}

function enterPresentationMode(wantsRecord: boolean) {
  recordingMode.value = wantsRecord;
  presentationMode.value = "present";
  closeEditorFloatingMenus();
}

function exitPresentationMode() {
  recordingMode.value = false;
  presentationMode.value = "edit";
}

function editorSlideStyle(slide: EditorSlide) {
  const width = editorWidthOptions.find(item => item.value === slide.width)?.width || 880;
  return {
    background: slide.background,
    textAlign: slide.align,
    "--ppt-slide-width": `${width}px`,
    "--ppt-zoom": String(editorZoom.value)
  };
}

function isTaskRunning(status: PptTaskStatus) {
  return status === "pending" || status === "processing";
}

function statusLabel(status: PptTaskStatus) {
  const labels: Record<PptTaskStatus, string> = {
    pending: "生成中",
    processing: "生成中",
    success: "成功",
    failed: "失败"
  };
  return labels[status] || status;
}

function languageLabel(value: PptLanguage) {
  return value === "en" ? "英文" : "中文";
}

function themeLabel(value: PptTheme) {
  const item = themeOptions.find(option => option.value === value);
  return item?.label || "商务简约";
}

function formatDate(value?: string) {
  if (!value) return "--";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "--";
  return date.toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit"
  });
}

function wait(ms: number) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

function errorMessage(error: unknown, fallback = "PPT生成失败，请稍后重试") {
  return error instanceof Error ? error.message : fallback;
}

function confirmAction(title: string, content: string) {
  return new Promise<boolean>(resolve => {
    uni.showModal({
      title,
      content,
      confirmColor: "#ef4444",
      success: result => resolve(Boolean(result.confirm)),
      fail: () => resolve(false)
    });
  });
}

function showKeyboardShortcuts() {
  showHelpMenu.value = false;
  uni.showModal({
    title: "键盘快捷键",
    content: "撤销 Ctrl+Z · 重做 Ctrl+Shift+Z · 保存 Ctrl+S · 演示 Esc 退出",
    showCancel: false,
  });
}

function showPptHelpCenter() {
  showHelpMenu.value = false;
  uni.showModal({
    title: "PPT 帮助中心",
    content: "先生成大纲，再逐页编辑内容与配图，完成后使用导出按钮生成 PPTX 文件。",
    showCancel: false,
  });
}

async function openDownloadUrl(url: string) {
  if (typeof window !== "undefined" && typeof window.open === "function") {
    window.open(url, "_blank", "noopener");
    return;
  }
  const filePath = await downloadTemporaryFile(url);
  uni.openDocument({ filePath });
}
</script>

<style scoped>
.ppt-page {
  min-height: calc(100vh - 72px);
  padding: 26px;
  color: #0f172a;
  background: #f8fafc;
  font-family: -apple-system, BlinkMacSystemFont, "PingFang SC", "Microsoft YaHei", "Noto Sans SC", sans-serif;
  letter-spacing: 0;
}

.ppt-page,
.ppt-page view,
.ppt-page text,
.ppt-page button,
.ppt-page input,
.ppt-page textarea,
.ppt-page image,
.ppt-page picker,
.ppt-page scroll-view {
  box-sizing: border-box;
  letter-spacing: 0;
}

.ppt-page-head,
.ppt-hero,
.ppt-workspace-grid,
.ppt-result-layout {
  width: min(1220px, 100%);
  margin: 0 auto;
}

.ppt-page-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 18px;
}

.ppt-kicker,
.ppt-control-head text:last-child,
.ppt-head-meta text,
.ppt-panel-head text:last-child {
  color: #64748b;
  font-size: 12px;
  font-weight: 600;
}

.ppt-page-title {
  display: block;
  margin-top: 4px;
  color: #0f172a;
  font-size: 28px;
  font-weight: 700;
}

.ppt-head-meta {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.ppt-head-meta text {
  padding: 7px 10px;
  border: 1px solid #dbe4f0;
  border-radius: 8px;
  background: #fff;
}

.ppt-hero {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 280px;
  gap: 22px;
  align-items: stretch;
  padding: 28px;
  border: 1px solid #dbe4f0;
  border-radius: 8px;
  background: #ffffff;
  box-shadow: 0 18px 42px rgba(15, 23, 42, .08);
}

.ppt-hero-copy {
  display: grid;
  align-content: center;
  gap: 12px;
}

.ppt-hero-title {
  max-width: 620px;
  color: #0f172a;
  font-size: 36px;
  line-height: 1.15;
  font-weight: 950;
}

.ppt-hero-subtitle {
  color: #475569;
  font-size: 15px;
  line-height: 1.7;
}

.ppt-hero-preview {
  position: relative;
  min-height: 162px;
  overflow: hidden;
  border: 1px solid #dbe4f0;
  border-radius: 8px;
  background: #f1f5f9;
}

.ppt-preview-slide {
  position: absolute;
  inset: 24px 58px 24px 22px;
  display: grid;
  gap: 10px;
  align-content: center;
  padding: 18px;
  border: 1px solid #bfdbfe;
  border-radius: 8px;
  background: #fff;
  box-shadow: 0 12px 24px rgba(37, 99, 235, .14);
}

.ppt-preview-slide text {
  display: block;
  height: 9px;
  border-radius: 999px;
  background: #2563eb;
}

.ppt-preview-slide text:nth-child(1) {
  width: 56%;
  height: 13px;
  background: #0f172a;
}

.ppt-preview-slide text:nth-child(2) {
  width: 82%;
  background: #60a5fa;
}

.ppt-preview-slide text:nth-child(3) {
  width: 66%;
  background: #14b8a6;
}

.ppt-preview-stack {
  position: absolute;
  top: 25px;
  right: 18px;
  display: grid;
  gap: 10px;
}

.ppt-preview-stack view {
  width: 34px;
  height: 22px;
  border: 1px solid #cbd5e1;
  border-radius: 6px;
  background: #fff;
}

.ppt-workspace-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.25fr) minmax(300px, .75fr);
  gap: 18px;
  margin-top: 18px;
}

.ppt-form-card,
.ppt-side-panel,
.ppt-progress-panel,
.ppt-result-card,
.ppt-history-card {
  border: 1px solid #dbe4f0;
  border-radius: 8px;
  background: #fff;
  box-shadow: 0 12px 30px rgba(15, 23, 42, .06);
}

.ppt-form-card {
  display: grid;
  gap: 18px;
  padding: 22px;
}

.ppt-field,
.ppt-control-group {
  display: grid;
  gap: 10px;
}

.ppt-field-head,
.ppt-control-head,
.ppt-panel-head,
.ppt-progress-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.ppt-field-head text:first-child,
.ppt-control-head text:first-child,
.ppt-panel-head text:first-child,
.ppt-progress-head text:first-child {
  color: #0f172a;
  font-size: 15px;
  font-weight: 700;
}

.ppt-prompt-textarea {
  width: 100%;
  min-height: 148px;
  padding: 14px;
  border: 1px solid #cbd5e1;
  border-radius: 8px;
  background: #ffffff;
  color: #0f172a;
  caret-color: #2563eb;
  font-size: 15px;
  line-height: 1.7;
  resize: vertical;
  outline: none;
}

.ppt-prompt-textarea:focus {
  border-color: #2563eb;
  box-shadow: 0 0 0 3px rgba(37, 99, 235, .14);
}

.ppt-prompt-textarea::placeholder {
  color: #94a3b8;
}

.ppt-segment-row,
.ppt-theme-grid,
.ppt-example-grid,
.ppt-action-row,
.ppt-history-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

.ppt-chip,
.ppt-theme-card,
.ppt-example-card,
.ppt-action-row button,
.ppt-history-actions button {
  min-height: 38px;
  border: 1px solid #dbe4f0;
  border-radius: 8px;
  background: #fff;
  color: #334155;
  font-size: 13px;
  font-weight: 600;
}

.ppt-chip {
  min-width: 76px;
  padding: 0 14px;
}

.ppt-chip:hover,
.ppt-theme-card:hover,
.ppt-example-card:hover,
.ppt-action-row button:hover,
.ppt-history-actions button:hover {
  border-color: #93c5fd;
  color: #1d4ed8;
}

.ppt-chip.active,
.ppt-theme-card.active {
  border-color: #2563eb;
  background: #eff6ff;
  color: #1d4ed8;
}

.ppt-control-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(220px, .6fr);
  gap: 14px;
  align-items: stretch;
}

.ppt-search-control {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  min-height: 76px;
  padding: 14px;
  border: 1px solid #dbe4f0;
  border-radius: 8px;
  background: #f8fafc;
}

.ppt-search-control view {
  display: grid;
  gap: 5px;
}

.ppt-search-control text:first-child {
  color: #0f172a;
  font-size: 14px;
  font-weight: 700;
}

.ppt-search-control text:last-child {
  color: #64748b;
  font-size: 12px;
  font-weight: 600;
}

.ppt-switch {
  position: relative;
  width: 48px;
  height: 28px;
  padding: 0;
  border: 0;
  border-radius: 999px;
  background: #cbd5e1;
}

.ppt-switch text {
  position: absolute;
  top: 4px;
  left: 4px;
  width: 20px;
  height: 20px;
  border-radius: 999px;
  background: #fff;
  box-shadow: 0 2px 8px rgba(15, 23, 42, .18);
  transition: transform .18s ease;
}

.ppt-switch.checked {
  background: #2563eb;
}

.ppt-switch.checked text {
  transform: translateX(20px);
}

.ppt-theme-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

.ppt-theme-card {
  display: grid;
  gap: 4px;
  min-height: 64px;
  padding: 10px 12px;
  text-align: left;
}

.ppt-theme-card text:first-child {
  color: inherit;
  font-size: 14px;
  font-weight: 700;
}

.ppt-theme-card text:last-child {
  color: #64748b;
  font-size: 12px;
}

.ppt-generate-button {
  width: 100%;
  height: 48px;
  border: 0;
  border-radius: 8px;
  background: #2563eb;
  color: #fff;
  font-size: 15px;
  font-weight: 950;
}

.ppt-generate-button:hover {
  background: #1d4ed8;
}

.ppt-generate-button[disabled] {
  background: #cbd5e1;
  color: #64748b;
}

.ppt-generate-button.loading {
  background: #0f766e;
}

.ppt-error-state,
.ppt-result-error {
  padding: 10px 12px;
  border: 1px solid #fecaca;
  border-radius: 8px;
  background: #fef2f2;
  color: #b91c1c;
  font-size: 13px;
  font-weight: 600;
}

.ppt-side-panel {
  display: grid;
  align-content: start;
  gap: 16px;
  padding: 18px;
}

.ppt-example-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.ppt-example-card {
  min-height: 76px;
  padding: 12px;
  text-align: left;
  line-height: 1.45;
}

.ppt-progress-panel {
  width: min(1220px, 100%);
  margin: 18px auto 0;
  padding: 18px;
}

.ppt-progress-head text:last-child {
  color: #2563eb;
  font-size: 13px;
  font-weight: 700;
}

.ppt-progress-track {
  height: 9px;
  margin-top: 14px;
  overflow: hidden;
  border-radius: 999px;
  background: #e2e8f0;
}

.ppt-progress-track view {
  height: 100%;
  border-radius: inherit;
  background: #2563eb;
  transition: width .24s ease;
}

.ppt-progress-copy {
  display: block;
  margin-top: 10px;
  color: #475569;
  font-size: 13px;
  font-weight: 600;
}

.ppt-result-layout {
  display: grid;
  grid-template-columns: minmax(0, 1.05fr) minmax(340px, .95fr);
  gap: 18px;
  margin-top: 18px;
}

.ppt-result-card,
.ppt-history-card {
  padding: 18px;
}

.ppt-result-body {
  display: grid;
  grid-template-columns: minmax(220px, .8fr) minmax(0, 1fr);
  gap: 16px;
  margin-top: 16px;
}

.ppt-result-preview {
  display: grid;
  place-items: center;
  min-height: 230px;
  border: 1px solid #dbe4f0;
  border-radius: 8px;
  background: #f8fafc;
}

.ppt-result-slide {
  width: min(260px, 90%);
  aspect-ratio: 16 / 10;
  display: grid;
  gap: 10px;
  align-content: center;
  padding: 18px;
  border: 1px solid #bfdbfe;
  border-radius: 8px;
  background: #fff;
  box-shadow: 0 12px 24px rgba(15, 23, 42, .08);
}

.ppt-result-slide > text:first-child {
  color: #0f172a;
  font-size: 16px;
  font-weight: 950;
  line-height: 1.35;
}

.ppt-result-slide > text:nth-child(2) {
  color: #2563eb;
  font-size: 12px;
  font-weight: 700;
}

.ppt-result-slide view {
  display: grid;
  gap: 6px;
  margin-top: 6px;
}

.ppt-result-slide view text {
  height: 7px;
  border-radius: 999px;
  background: #cbd5e1;
}

.ppt-result-slide view text:nth-child(2) {
  width: 78%;
  background: #93c5fd;
}

.ppt-result-slide view text:nth-child(3) {
  width: 62%;
  background: #5eead4;
}

.ppt-result-info {
  display: grid;
  align-content: start;
  gap: 12px;
}

.ppt-result-title {
  color: #0f172a;
  font-size: 20px;
  font-weight: 950;
  line-height: 1.35;
}

.ppt-result-meta {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.ppt-result-meta text,
.ppt-status {
  display: inline-flex;
  align-items: center;
  min-height: 28px;
  padding: 4px 9px;
  border-radius: 8px;
  background: #f1f5f9;
  color: #475569;
  font-size: 12px;
  font-weight: 700;
}

.ppt-status.pending,
.ppt-status.processing {
  background: #eff6ff;
  color: #1d4ed8;
}

.ppt-status.success {
  background: #ecfdf5;
  color: #047857;
}

.ppt-status.failed {
  background: #fef2f2;
  color: #b91c1c;
}

.ppt-result-time,
.ppt-operation-message {
  color: #64748b;
  font-size: 13px;
  line-height: 1.6;
}

.ppt-operation-message {
  color: #1d4ed8;
  font-weight: 600;
}

.ppt-action-row button,
.ppt-history-actions button {
  min-width: 78px;
  padding: 0 11px;
}

.ppt-action-row button[disabled],
.ppt-history-actions button[disabled] {
  border-color: #e2e8f0;
  background: #f1f5f9;
  color: #94a3b8;
}

.ppt-action-row button.danger,
.ppt-history-actions button.danger {
  color: #b91c1c;
}

.ppt-history-list {
  display: grid;
  gap: 12px;
  margin-top: 16px;
}

.ppt-history-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 10px 14px;
  padding: 13px;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  background: #fff;
}

.ppt-history-title {
  display: block;
  color: #0f172a;
  font-size: 14px;
  font-weight: 700;
  line-height: 1.4;
}

.ppt-history-subtitle,
.ppt-history-meta text:first-child {
  color: #64748b;
  font-size: 12px;
  line-height: 1.6;
}

.ppt-history-meta {
  display: grid;
  justify-items: end;
  align-content: start;
  gap: 6px;
}

.ppt-history-actions {
  grid-column: 1 / -1;
}

.ppt-empty-state {
  display: grid;
  place-items: center;
  min-height: 160px;
  margin-top: 16px;
  border: 1px dashed #cbd5e1;
  border-radius: 8px;
  background: #f8fafc;
  color: #64748b;
  font-size: 14px;
  font-weight: 700;
}

@media (max-width: 1080px) {
  .ppt-hero,
  .ppt-workspace-grid,
  .ppt-result-layout,
  .ppt-result-body {
    grid-template-columns: 1fr;
  }

  .ppt-hero-preview {
    min-height: 150px;
  }
}

@media (max-width: 760px) {
  .ppt-page {
    padding: 16px;
  }

  .ppt-page-head,
  .ppt-control-grid,
  .ppt-history-row {
    grid-template-columns: 1fr;
  }

  .ppt-page-head {
    display: grid;
  }

  .ppt-page-title {
    font-size: 24px;
  }

  .ppt-hero {
    padding: 20px;
  }

  .ppt-hero-title {
    font-size: 28px;
  }

  .ppt-theme-grid,
  .ppt-example-grid {
    grid-template-columns: 1fr;
  }

  .ppt-history-meta {
    justify-items: start;
  }
}
.ppt-page.is-editor {
  min-height: calc(100vh - 72px);
  padding: 0;
  color: #f8fafc;
  background: #050505;
}

.ppt-editor-shell {
  min-height: calc(100vh - 72px);
  background: #050505;
  border: 1px solid #1f1f1f;
}

.ppt-editor-header {
  position: sticky;
  top: 0;
  z-index: 20;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 16px 22px;
  border-bottom: 1px solid #1f1f1f;
  background: rgba(5, 5, 5, .94);
  backdrop-filter: blur(16px);
}

.ppt-editor-titlebar,
.ppt-editor-actions,
.ppt-editor-body,
.ppt-tool-rail,
.ppt-panel-bottom {
  display: flex;
  align-items: center;
}

.ppt-editor-titlebar {
  min-width: 0;
  flex: 1;
  gap: 10px;
}

.ppt-editor-icon,
.ppt-editor-history,
.ppt-editor-actions button,
.ppt-tool-rail button,
.ppt-panel-title button,
.ppt-panel-bottom > button,
.ppt-slide-floating-tools button,
.ppt-slide-insert,
.ppt-present-header button,
.ppt-present-nav {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 36px;
  border: 1px solid #27272a;
  border-radius: 8px;
  color: #f8fafc;
  background: #101010;
  font-weight: 600;
  cursor: pointer;
}

.ppt-editor-icon {
  width: 38px;
  padding: 0;
}

.ppt-editor-icon.is-brain {
  color: #67e8f9;
  border-color: rgba(34, 211, 238, .42);
  background: linear-gradient(135deg, rgba(8, 47, 73, .8), rgba(8, 51, 68, .42));
}

.ppt-editor-actions {
  gap: 10px;
  flex-wrap: wrap;
}

.ppt-editor-actions button {
  gap: 8px;
  padding: 0 13px;
}

.ppt-editor-actions button.active,
.ppt-editor-actions button:hover,
.ppt-editor-icon:hover,
.ppt-editor-history:hover:not(:disabled),
.ppt-tool-rail button:hover,
.ppt-tool-rail button.active,
.ppt-slide-floating-tools button:hover {
  border-color: #3f3f46;
  background: #18181b;
}

.ppt-editor-actions .primary {
  color: #050505;
  border-color: #f8fafc;
  background: #f8fafc;
}

.ppt-editor-title-input {
  min-width: 120px;
  max-width: 420px;
  flex: 1;
  border: 0;
  color: #f8fafc;
  background: transparent;
  font-size: 18px;
  font-weight: 950;
  outline: none;
}

.ppt-save-state {
  color: #86efac;
  font-size: 13px;
  font-weight: 700;
  white-space: nowrap;
}

.ppt-editor-history {
  width: 34px;
  min-height: 34px;
}

.ppt-editor-history:disabled,
.ppt-slide-floating-tools button:disabled,
.ppt-panel-bottom button:disabled,
.ppt-present-nav:disabled {
  color: #52525b;
  cursor: not-allowed;
  opacity: .55;
}

.ppt-editor-menu-wrap,
.ppt-floating-menu-wrap,
.ppt-panel-bottom {
  position: relative;
}

.ppt-editor-menu,
.ppt-floating-menu,
.ppt-palette-menu,
.ppt-magic-menu,
.ppt-zoom-menu,
.ppt-help-menu {
  position: absolute;
  z-index: 40;
  display: grid;
  gap: 6px;
  padding: 12px;
  border: 1px solid #27272a;
  border-radius: 10px;
  color: #f8fafc;
  background: #0f0f10;
  box-shadow: 0 24px 80px rgba(0, 0, 0, .45);
}

.ppt-editor-menu {
  top: 44px;
  left: 0;
  width: 280px;
}

.ppt-editor-menu > text,
.ppt-magic-menu > text {
  margin-top: 8px;
  color: #a1a1aa;
  font-size: 12px;
  font-weight: 700;
}

.ppt-editor-menu button,
.ppt-floating-menu button,
.ppt-magic-grid button,
.ppt-magic-primary,
.ppt-panel-cards button,
.ppt-palette-image-row button,
.ppt-help-menu button,
.ppt-zoom-menu button {
  justify-content: flex-start;
  gap: 10px;
  min-height: 36px;
  border: 0;
  border-radius: 8px;
  color: #f8fafc;
  background: transparent;
  text-align: left;
}

.ppt-editor-menu button:hover:not(:disabled),
.ppt-floating-menu button:hover:not(:disabled),
.ppt-magic-grid button:hover,
.ppt-panel-cards button:hover,
.ppt-help-menu button:hover,
.ppt-zoom-menu button:hover {
  background: #1f1f22;
}

.ppt-editor-body {
  align-items: stretch;
  height: calc(100vh - 144px);
  min-height: 720px;
}

.ppt-slide-sidebar {
  width: 182px;
  flex: 0 0 182px;
  border-right: 1px solid #1f1f1f;
  background: #050505;
}

.ppt-slide-sidebar-head {
  padding: 16px 14px 8px;
}

.ppt-slide-sidebar-head view,
.ppt-slide-thumb,
.ppt-panel-title,
.ppt-palette-image-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.ppt-slide-sidebar-head text:first-child {
  display: block;
  color: #f8fafc;
  font-size: 15px;
  font-weight: 950;
}

.ppt-slide-sidebar-head text:last-child {
  color: #a1a1aa;
  font-size: 12px;
}

.ppt-slide-thumbs {
  height: calc(100% - 62px);
  padding: 8px 10px 18px;
}

.ppt-slide-thumb {
  width: 100%;
  min-height: 92px;
  margin-bottom: 10px;
  padding: 8px;
  border: 1px solid #262626;
  border-radius: 8px;
  color: #e5e7eb;
  background: #111;
  text-align: left;
}

.ppt-slide-thumb.active {
  border-color: #22d3ee;
  box-shadow: 0 0 0 2px rgba(34, 211, 238, .18);
}

.ppt-slide-thumb > text {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border-radius: 6px;
  color: #050505;
  background: #67e8f9;
  font-weight: 950;
}

.ppt-slide-thumb view {
  display: grid;
  min-width: 0;
  gap: 4px;
}

.ppt-slide-thumb view text:first-child {
  color: #f8fafc;
  font-size: 12px;
  font-weight: 700;
}

.ppt-slide-thumb view text:last-child {
  overflow: hidden;
  color: #a1a1aa;
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ppt-slide-canvas {
  flex: 1;
  min-width: 0;
  height: 100%;
  padding: 32px 32px 90px;
  background: #030303;
}

.ppt-edit-slide {
  position: relative;
  width: min(var(--ppt-slide-width, 880px), 100%);
  min-height: 486px;
  margin: 0 auto 28px;
  padding: 58px 64px;
  border: 1px solid rgba(148, 163, 184, .25);
  border-radius: 16px;
  color: #0f172a;
  box-shadow: 0 24px 80px rgba(0, 0, 0, .35);
  transform: scale(var(--ppt-zoom, .92));
  transform-origin: top center;
}

.ppt-edit-slide.active {
  border-color: #22d3ee;
  box-shadow: 0 0 0 3px rgba(34, 211, 238, .32), 0 24px 80px rgba(0, 0, 0, .42);
}

.ppt-slide-count {
  display: block;
  color: #2563eb;
  font-size: 18px;
  font-weight: 950;
}

.ppt-slide-title {
  display: block;
  margin-top: 22px;
  color: #0f172a;
  font-size: 42px;
  line-height: 1.12;
  font-weight: 950;
}

.ppt-slide-copy {
  display: block;
  margin-top: 20px;
  color: #334155;
  font-size: 17px;
  line-height: 1.7;
}

.ppt-slide-points {
  display: grid;
  gap: 10px;
  margin-top: 26px;
  color: #0f172a;
  font-size: 18px;
}

.ppt-slide-floating-tools {
  position: absolute;
  top: 18px;
  right: 20px;
  display: flex;
  gap: 8px;
}

.ppt-slide-floating-tools > .ppt-floating-menu-wrap > button {
  width: 38px;
  min-height: 38px;
  border-radius: 999px;
  color: #fff;
  background: rgba(15, 23, 42, .62);
  backdrop-filter: blur(12px);
}

.ppt-floating-menu {
  top: 44px;
  right: 0;
  width: 184px;
}

.ppt-palette-menu,
.ppt-magic-menu {
  top: 44px;
  right: 0;
  width: 360px;
}

.ppt-palette-menu {
  gap: 16px;
}

.ppt-palette-menu > view,
.ppt-magic-head {
  display: grid;
  gap: 8px;
}

.ppt-palette-menu text,
.ppt-magic-head text:first-child {
  color: #f8fafc;
  font-weight: 700;
}

.ppt-palette-layouts,
.ppt-palette-colors,
.ppt-palette-segment,
.ppt-magic-grid,
.ppt-panel-cards {
  display: grid;
  gap: 8px;
}

.ppt-palette-layouts {
  grid-template-columns: repeat(5, 1fr);
}

.ppt-palette-colors {
  grid-template-columns: repeat(6, 1fr);
}

.ppt-palette-segment,
.ppt-magic-grid {
  grid-template-columns: repeat(2, 1fr);
}

.ppt-palette-layouts button,
.ppt-palette-segment button {
  min-height: 38px;
  border: 1px solid #27272a;
  border-radius: 8px;
  color: #f8fafc;
  background: #18181b;
}

.ppt-palette-colors button {
  min-height: 34px;
  border: 2px solid #27272a;
  border-radius: 999px;
}

.ppt-palette-layouts button.active,
.ppt-palette-segment button.active,
.ppt-palette-colors button.active {
  border-color: #22d3ee;
  box-shadow: 0 0 0 2px rgba(34, 211, 238, .22);
}

.ppt-magic-menu {
  overflow: hidden;
  padding: 0;
}

.ppt-magic-head {
  padding: 16px;
  border-bottom: 1px solid #27272a;
  background: linear-gradient(180deg, rgba(255, 255, 255, .08), transparent);
}

.ppt-magic-head text:last-child {
  color: #a1a1aa;
  font-size: 12px;
}

.ppt-magic-input {
  display: flex;
  gap: 8px;
  margin: 12px;
  padding: 8px;
  border: 1px solid #2f2f34;
  border-radius: 10px;
  background: rgba(255, 255, 255, .06);
}

.ppt-magic-input input,
.ppt-panel-cards input {
  min-width: 0;
  flex: 1;
  border: 0;
  color: #f8fafc;
  background: transparent;
  outline: none;
}

.ppt-magic-input button {
  width: 34px;
  min-height: 34px;
  border: 0;
  border-radius: 999px;
  color: #050505;
  background: #f8fafc;
}

.ppt-magic-primary,
.ppt-magic-grid,
.ppt-magic-menu > text {
  margin: 0 12px 12px;
}

.ppt-slide-insert {
  position: absolute;
  left: 50%;
  z-index: 4;
  width: 36px;
  min-height: 36px;
  border-radius: 999px;
  color: #0f172a;
  background: #fff;
  transform: translateX(-50%);
}

.ppt-slide-insert.before {
  top: -18px;
}

.ppt-slide-insert.after {
  bottom: -18px;
}

.ppt-editor-panel {
  display: flex;
  width: 320px;
  flex: 0 0 320px;
  border-left: 1px solid #1f1f1f;
  background: #050505;
}

.ppt-tool-rail {
  width: 62px;
  flex-direction: column;
  justify-content: center;
  gap: 14px;
  border-right: 1px solid #1f1f1f;
}

.ppt-tool-rail button {
  width: 42px;
  min-height: 42px;
  flex-direction: column;
  gap: 2px;
  font-size: 12px;
}

.ppt-tool-rail button text {
  font-size: 10px;
}

.ppt-panel-content {
  position: relative;
  flex: 1;
  min-width: 0;
  padding: 16px;
}

.ppt-panel-title {
  margin-bottom: 18px;
}

.ppt-panel-title > text {
  color: #f8fafc;
  font-size: 16px;
  font-weight: 950;
}

.ppt-panel-title button {
  width: 32px;
  min-height: 32px;
}

.ppt-panel-cards {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.ppt-panel-cards button,
.ppt-panel-cards input {
  min-height: 68px;
  padding: 12px;
  border: 1px solid #27272a;
  border-radius: 10px;
  color: #f8fafc;
  background: #111;
}

.ppt-panel-empty {
  display: grid;
  place-items: center;
  min-height: 300px;
  gap: 10px;
  color: #a1a1aa;
  text-align: center;
}

.ppt-panel-empty text:first-child {
  color: #f8fafc;
  font-size: 18px;
  font-weight: 950;
}

.ppt-panel-bottom {
  position: absolute;
  right: 16px;
  bottom: 16px;
  left: 16px;
  justify-content: space-between;
  gap: 8px;
}

.ppt-zoom-menu,
.ppt-help-menu {
  right: 0;
  bottom: 44px;
  width: 180px;
}

.ppt-present-stage {
  position: relative;
  min-height: calc(100vh - 72px);
  padding: 80px 80px 64px;
  background: #050505;
}

.ppt-present-header {
  position: absolute;
  top: max(18px, var(--header-padding-top, 0px));
  right: max(24px, var(--capsule-right-space, 0px));
  left: 24px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  color: #f8fafc;
}

.ppt-present-canvas {
  position: relative;
  display: grid;
  align-content: center;
  width: min(1060px, 100%);
  min-height: 620px;
  margin: 0 auto;
  padding: 72px;
  border-radius: 18px;
  color: #0f172a;
}

.ppt-present-count {
  color: #2563eb;
  font-size: 20px;
  font-weight: 950;
}

.ppt-present-title {
  margin-top: 22px;
  font-size: 56px;
  font-weight: 950;
  line-height: 1.08;
}

.ppt-present-copy,
.ppt-present-points {
  margin-top: 24px;
  font-size: 20px;
  line-height: 1.7;
}

.ppt-recording-bar {
  position: absolute;
  right: 24px;
  bottom: 24px;
  display: flex;
  gap: 12px;
  padding: 12px 14px;
  border-radius: 999px;
  color: #ecfeff;
  background: rgba(15, 23, 42, .78);
}

.ppt-present-nav {
  position: absolute;
  top: 50%;
  width: 48px;
  min-height: 48px;
  border-radius: 999px;
  transform: translateY(-50%);
}

.ppt-present-nav.prev {
  left: 24px;
}

.ppt-present-nav.next {
  right: 24px;
}

@media (max-width: 1100px) {
  .ppt-editor-body {
    height: auto;
    min-height: 0;
    flex-direction: column;
  }

  .ppt-slide-sidebar,
  .ppt-editor-panel {
    width: 100%;
    flex-basis: auto;
  }

  .ppt-slide-thumbs {
    height: auto;
    max-height: 240px;
  }

  .ppt-editor-panel {
    min-height: 420px;
  }
}

@media (max-width: 760px) {
  .ppt-page.is-editor {
    min-height: 100vh;
    padding: 0;
    overflow-x: hidden;
  }

  .ppt-editor-shell {
    min-height: 100vh;
    border-radius: 0;
  }

  .ppt-editor-header {
    align-items: stretch;
    flex-direction: column;
    position: sticky;
    top: 0;
    z-index: 40;
    min-height: var(--header-height, 64px);
    padding: calc(var(--header-padding-top, 20px) + 10px) max(10px, var(--capsule-right-space, 0px)) 10px 10px;
    box-sizing: border-box;
  }

  .ppt-editor-titlebar {
    min-width: 0;
  }

  .ppt-editor-title-input {
    min-width: 0;
    flex: 1;
  }

  .ppt-save-state,
  .ppt-editor-history {
    display: none;
  }

  .ppt-editor-actions {
    justify-content: stretch;
  }

  .ppt-editor-actions button {
    flex: 1;
    min-width: 0;
    padding: 0 6px;
    font-size: 11px;
  }

  .ppt-editor-body {
    display: flex;
    flex-direction: column;
  }

  .ppt-slide-sidebar {
    order: 2;
    padding: 10px;
  }

  .ppt-slide-thumbs {
    width: 100%;
    height: auto;
    max-height: 150px;
    white-space: nowrap;
  }

  .ppt-slide-thumb {
    display: inline-flex;
    width: 156px;
    min-height: 74px;
    margin-right: 8px;
    vertical-align: top;
  }

  .ppt-slide-canvas {
    order: 1;
    width: 100%;
    padding: 24px 14px 80px;
  }

  .ppt-edit-slide {
    min-height: 420px;
    padding: 48px 28px;
  }

  .ppt-edit-slide:not(.active) {
    display: none;
  }

  .ppt-slide-title {
    font-size: 32px;
  }

  .ppt-editor-panel {
    order: 3;
    min-height: 0;
  }

  .ppt-tool-rail {
    width: 64px;
    flex: 0 0 64px;
  }

  .ppt-panel-content {
    min-width: 0;
    padding: 14px;
  }

  .ppt-visual-options {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .ppt-palette-menu,
  .ppt-magic-menu {
    right: -72px;
    width: min(340px, calc(100vw - 32px));
  }
}

.ppt-editor-actions uni-button,
.ppt-tool-rail uni-button,
.ppt-panel-title uni-button,
.ppt-panel-bottom > uni-button,
.ppt-slide-floating-tools uni-button,
.ppt-slide-insert,
.ppt-present-header uni-button,
.ppt-present-nav {
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.ppt-editor-actions uni-button {
  gap: 8px;
  min-height: 36px;
  padding: 0 13px;
  border: 1px solid #27272a;
  border-radius: 8px;
  color: #f8fafc;
  background: #101010;
  font-weight: 600;
}

.ppt-editor-actions uni-button.primary {
  color: #050505;
  border-color: #f8fafc;
  background: #f8fafc;
}

.ppt-editor-menu uni-button,
.ppt-floating-menu uni-button,
.ppt-magic-grid uni-button,
.ppt-magic-primary,
.ppt-panel-cards uni-button,
.ppt-palette-image-row uni-button,
.ppt-help-menu uni-button,
.ppt-zoom-menu uni-button {
  justify-content: flex-start;
  gap: 10px;
  min-height: 36px;
  border: 0;
  border-radius: 8px;
  color: #f8fafc;
  background: transparent;
  text-align: left;
}

.ppt-editor-menu uni-button:hover:not([disabled]),
.ppt-floating-menu uni-button:hover:not([disabled]),
.ppt-magic-grid uni-button:hover,
.ppt-panel-cards uni-button:hover,
.ppt-help-menu uni-button:hover,
.ppt-zoom-menu uni-button:hover {
  background: #1f1f22;
}

.ppt-palette-layouts uni-button,
.ppt-palette-segment uni-button {
  min-height: 38px;
  border: 1px solid #27272a;
  border-radius: 8px;
  color: #f8fafc;
  background: #18181b;
}

.ppt-palette-colors uni-button {
  min-height: 34px;
  border: 2px solid #27272a;
  border-radius: 999px;
}

.ppt-palette-layouts uni-button.active,
.ppt-palette-segment uni-button.active,
.ppt-palette-colors uni-button.active {
  border-color: #22d3ee;
  box-shadow: 0 0 0 2px rgba(34, 211, 238, .22);
}

.ppt-tool-rail uni-button {
  width: 42px;
  min-height: 42px;
  flex-direction: column;
  gap: 2px;
  border: 1px solid #27272a;
  border-radius: 8px;
  color: #f8fafc;
  background: #101010;
  font-size: 12px;
}

.ppt-tool-rail uni-button.active,
.ppt-tool-rail uni-button:hover {
  border-color: #3f3f46;
  background: #18181b;
}

.ppt-slide-floating-tools > .ppt-floating-menu-wrap > uni-button {
  width: 38px;
  min-height: 38px;
  border: 1px solid #27272a;
  border-radius: 999px;
  color: #fff;
  background: rgba(15, 23, 42, .62);
  backdrop-filter: blur(12px);
}

.ppt-magic-input uni-button {
  width: 34px;
  min-height: 34px;
  border: 0;
  border-radius: 999px;
  color: #050505;
  background: #f8fafc;
}

.ppt-slide-visual {
  position: absolute;
  z-index: 0;
  right: 5%;
  bottom: 9%;
  width: 38%;
  height: 42%;
  border-radius: 14px;
  background: #e2e8f0;
}

.ppt-slide-title,
.ppt-slide-copy,
.ppt-slide-points,
.ppt-slide-count {
  position: relative;
  z-index: 1;
}

.ppt-visual-settings {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.ppt-visual-label,
.ppt-visual-instruction > uni-text {
  color: #cbd5e1;
  font-size: 12px;
  font-weight: 800;
}

.ppt-visual-options {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
}

.ppt-visual-options uni-button,
.ppt-visual-settings > uni-button {
  min-height: 38px;
  border: 1px solid #334155;
  border-radius: 8px;
  background: #111827;
  color: #e2e8f0;
  font-size: 12px;
}

.ppt-visual-options uni-button.active,
.ppt-visual-primary {
  border-color: #38bdf8 !important;
  background: #075985 !important;
  color: #fff !important;
}

.ppt-visual-instruction {
  display: flex;
  flex-direction: column;
  gap: 7px;
}

.ppt-visual-instruction uni-input {
  min-height: 40px;
  border: 1px solid #334155;
  border-radius: 8px;
  background: #0f172a;
  color: #fff;
  padding: 0 10px;
}

.ppt-visual-rules {
  display: flex;
  flex-direction: column;
  gap: 5px;
  color: #94a3b8;
  font-size: 12px;
}

.ppt-visual-message {
  color: #7dd3fc;
  font-size: 12px;
  line-height: 1.6;
}

.ppt-visual-history {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.ppt-visual-history-list {
  width: 100%;
  white-space: nowrap;
}

.ppt-visual-history-list uni-button {
  display: inline-flex;
  width: 112px;
  min-height: 92px;
  margin: 0 8px 0 0;
  padding: 6px;
  flex-direction: column;
  gap: 5px;
  vertical-align: top;
}

.ppt-visual-history-list uni-image {
  width: 96px;
  height: 58px;
  border-radius: 6px;
}
</style>
