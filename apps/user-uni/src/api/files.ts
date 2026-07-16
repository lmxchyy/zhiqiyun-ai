import { downloadApiFile, getApiBaseURL, uploadApiFile } from './client'

type ReferenceImagePayload = {
  item?: { url?: string; path?: string }
  url?: string
  path?: string
}

function absoluteApiURL(value: string) {
  if (/^https?:\/\//i.test(value)) return value
  const base = getApiBaseURL().replace(/\/+$/, '')
  return `${base}${value.startsWith('/') ? value : `/${value}`}`
}

export async function uploadReferenceImage(filePath: string) {
  const payload = await uploadApiFile<ReferenceImagePayload>('/api/v1/reference-images', {
    filePath,
    name: 'file',
  })
  const item = payload.item || payload
  const url = String(item.url || item.path || '').trim()
  if (!url) throw new Error('上传接口未返回文件地址')
  return absoluteApiURL(url)
}

export async function downloadTemporaryFile(url: string) {
  const response = await downloadApiFile(url)
  if (!response.tempFilePath) throw new Error('下载完成但未返回临时文件')
  return response.tempFilePath
}
