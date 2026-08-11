import { downloadApiFile, getApiBaseURL, uploadApiFile } from './client'
import { referenceAssetFromPayload, type ReferenceAssetUploadResult } from '../features/inspiration/referenceAsset'

type ReferenceImagePayload = {
  item?: { assetId?: string; id?: string; name?: string; contentType?: string; url?: string; path?: string }
  assetId?: string
  id?: string
  name?: string
  contentType?: string
  url?: string
  path?: string
}

export function absoluteApiURL(value: string) {
  if (/^https?:\/\//i.test(value)) return value
  const browserOrigin = typeof window !== 'undefined' ? window.location.origin : ''
  const base = (getApiBaseURL() || browserOrigin).replace(/\/+$/, '')
  if (!base) return value
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

export async function uploadReferenceAsset(filePath: string): Promise<ReferenceAssetUploadResult> {
  const payload = await uploadApiFile<ReferenceImagePayload>('/api/v1/reference-images', {
    filePath,
    name: 'file',
  })
  const asset = referenceAssetFromPayload(payload)
  return { ...asset, previewUrl: absoluteApiURL(asset.previewUrl) }
}

export async function downloadTemporaryFile(url: string) {
  const response = await downloadApiFile(url)
  if (!response.tempFilePath) throw new Error('下载完成但未返回临时文件')
  return response.tempFilePath
}
