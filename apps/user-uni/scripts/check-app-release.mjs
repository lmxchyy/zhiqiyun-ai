import fs from 'node:fs'
import path from 'node:path'
import process from 'node:process'
import { fileURLToPath } from 'node:url'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const platformFlagIndex = process.argv.indexOf('--platform')
const platform = platformFlagIndex >= 0 ? String(process.argv[platformFlagIndex + 1] || '').toLowerCase() : 'android'

if (!['android', 'ios'].includes(platform)) {
  console.error(`不支持的发布平台：${platform || '(空)'}；可选值为 android 或 ios。`)
  process.exit(1)
}

function readEnvFile(file) {
  if (!fs.existsSync(file)) return {}
  const result = {}
  for (const rawLine of fs.readFileSync(file, 'utf8').split(/\r?\n/)) {
    const line = rawLine.trim()
    if (!line || line.startsWith('#')) continue
    const separator = line.indexOf('=')
    if (separator < 1) continue
    const key = line.slice(0, separator).trim()
    let value = line.slice(separator + 1).trim()
    if ((value.startsWith('"') && value.endsWith('"')) || (value.startsWith("'") && value.endsWith("'"))) {
      value = value.slice(1, -1)
    }
    result[key] = value
  }
  return result
}

function validatePng(relativePath, expectedWidth, expectedHeight, label, errors) {
  const file = path.resolve(root, 'src', relativePath)
  if (!fs.existsSync(file)) {
    errors.push(`${label}不存在：${relativePath}`)
    return
  }
  const header = fs.readFileSync(file).subarray(0, 24)
  const pngSignature = '89504e470d0a1a0a'
  if (header.length < 24 || header.subarray(0, 8).toString('hex') !== pngSignature) {
    errors.push(`${label}必须是 PNG 文件：${relativePath}`)
    return
  }
  const width = header.readUInt32BE(16)
  const height = header.readUInt32BE(20)
  if (width !== expectedWidth || height !== expectedHeight) {
    errors.push(`${label}尺寸必须为 ${expectedWidth}x${expectedHeight}，当前为 ${width}x${height}：${relativePath}`)
  }
}

const productionEnvPath = path.join(root, '.env.production')
const env = { ...readEnvFile(productionEnvPath), ...process.env }
const manifestPath = path.join(root, 'src', 'manifest.json')
const manifest = JSON.parse(fs.readFileSync(manifestPath, 'utf8').replace(/^\uFEFF/, ''))
const errors = []

const apiBaseURL = String(env.VITE_API_BASE_URL || '').trim()
if (!apiBaseURL) {
  errors.push('缺少 VITE_API_BASE_URL；请复制 .env.production.example 为 .env.production 并填写正式地址。')
} else {
  try {
    const url = new URL(apiBaseURL)
    if (url.protocol !== 'https:') errors.push('VITE_API_BASE_URL 必须使用 HTTPS。')
    if (url.hostname === 'example.com' || url.hostname.endsWith('.example.com')) {
      errors.push('VITE_API_BASE_URL 仍是示例域名，必须替换为真实生产 API。')
    }
  } catch {
    errors.push('VITE_API_BASE_URL 不是有效的绝对 URL。')
  }
}

const appId = String(manifest.appid || '').trim()
if (!/^__UNI__[0-9a-f]{7,8}$/i.test(appId)) {
  errors.push(`manifest.json 的 appid 不是有效 DCloud AppID：${appId || '(空)'}`)
}

if (platform === 'android') {
  const packageName = String(manifest['app-plus']?.distribute?.android?.packagename || '').trim()
  if (!/^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$/i.test(packageName)) {
    errors.push(`Android 包名不合法：${packageName || '(空)'}`)
  }
}

if (platform === 'ios') {
  const ios = manifest['app-plus']?.distribute?.ios || {}
  const bundleId = String(ios.appid || '').trim()
  if (!/^[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)+$/.test(bundleId)) {
    errors.push(`iOS Bundle ID 不合法：${bundleId || '(空)'}`)
  }
  if (!['iphone', 'ipad', 'universal'].includes(String(ios.devices || ''))) {
    errors.push('iOS devices 必须是 iphone、ipad 或 universal。')
  }
  const deploymentTarget = String(ios.deploymentTarget || '')
  if (!/^\d+\.\d+$/.test(deploymentTarget)) {
    errors.push(`iOS deploymentTarget 不合法：${deploymentTarget || '(空)'}`)
  }
  for (const [key, label] of [
    ['NSPhotoLibraryUsageDescription', '读取相册'],
    ['NSPhotoLibraryAddUsageDescription', '写入相册'],
    ['NSCameraUsageDescription', '使用相机'],
  ]) {
    if (!String(ios.privacyDescription?.[key] || '').trim()) errors.push(`iOS 缺少${label}隐私用途说明：${key}`)
  }

  const icons = manifest['app-plus']?.distribute?.icons?.ios || {}
  validatePng(String(icons.appstore || ''), 1024, 1024, 'iOS App Store 图标', errors)
  const iphoneIcons = icons.iphone || {}
  for (const [key, width, height] of [
    ['app@2x', 120, 120], ['app@3x', 180, 180],
    ['spotlight@2x', 80, 80], ['spotlight@3x', 120, 120],
    ['settings@2x', 58, 58], ['settings@3x', 87, 87],
    ['notification@2x', 40, 40], ['notification@3x', 60, 60],
  ]) validatePng(String(iphoneIcons[key] || ''), width, height, `iOS iPhone ${key} 图标`, errors)
}

if (String(env.VITE_ENABLE_MOCK_LOGIN || '').toLowerCase() === 'true') {
  errors.push('发布构建禁止启用 VITE_ENABLE_MOCK_LOGIN。')
}

if (!/^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$/.test(String(manifest.versionName || ''))) {
  errors.push('manifest.json 的 versionName 必须是有效版本号。')
}
if (!Number.isInteger(Number(manifest.versionCode)) || Number(manifest.versionCode) <= 0) {
  errors.push('manifest.json 的 versionCode 必须是正整数。')
}

const platformLabel = platform === 'ios' ? 'iOS' : 'Android'
if (errors.length) {
  console.error(`${platformLabel} App 发布前检查未通过：`)
  for (const error of errors) console.error(`- ${error}`)
  process.exit(1)
}

const nativeId = platform === 'ios'
  ? manifest['app-plus'].distribute.ios.appid
  : manifest['app-plus'].distribute.android.packagename
console.log(`${platformLabel} App 发布前检查通过：${appId} / ${nativeId} / ${apiBaseURL}`)
