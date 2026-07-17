<template>
  <view class="ai-generation-notice" role="note" aria-label="人工智能生成内容提示">
    <view class="ai-generation-badge" aria-hidden="true">
      <text>AI生成</text>
    </view>
    <view class="ai-generation-copy">
      <text class="ai-generation-title">人工智能生成内容提示</text>
      <text class="ai-generation-description">
        本页面相关文本、图片、视频及演示文稿由人工智能生成，内容仅供参考，请核验后使用。
      </text>
    </view>
  </view>
</template>

<style scoped>
.ai-generation-notice {
  display: flex;
  align-items: center;
  gap: 12px;
  margin: 12px 16px 0;
  padding: 12px 14px;
  box-sizing: border-box;
  border: 1px solid #ffb66d;
  border-radius: 14px;
  color: #7c2d12;
  background: #fff7ed;
  box-shadow: 0 6px 18px rgba(194, 65, 12, 0.08);
}

.ai-generation-badge {
  display: flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 60px;
  min-height: 32px;
  padding: 0 8px;
  box-sizing: border-box;
  border-radius: 9px;
  color: #ffffff;
  background: #ea580c;
}

.ai-generation-badge text {
  font-size: 13px;
  line-height: 18px;
  font-weight: 700;
}

.ai-generation-copy {
  display: flex;
  flex: 1;
  min-width: 0;
  flex-direction: column;
}

.ai-generation-title {
  font-size: 13px;
  line-height: 19px;
  font-weight: 700;
}

.ai-generation-description {
  margin-top: 2px;
  color: #9a3412;
  font-size: 11px;
  line-height: 17px;
}

@media (max-width: 340px) {
  .ai-generation-notice {
    align-items: flex-start;
    gap: 9px;
    margin-right: 12px;
    margin-left: 12px;
    padding: 10px 11px;
  }

  .ai-generation-badge {
    flex-basis: 54px;
  }
}
</style>
