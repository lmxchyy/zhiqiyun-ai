export function formatNumber(value: number) {
  return value.toLocaleString('zh-CN')
}

export function workTypeLabel(type: string) {
  const labels: Record<string, string> = {
    image: '图片',
    video: '视频',
    ppt: 'PPT',
  }
  return labels[type] || type
}

export function statusLabel(status: string) {
  const labels: Record<string, string> = {
    queued: '排队中',
    processing: '生成中',
    succeeded: '已完成',
    failed: '失败',
  }
  return labels[status] || status
}
