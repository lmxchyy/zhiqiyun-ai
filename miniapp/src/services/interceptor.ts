import { useAppStore } from '@/stores/app'

function normalizeUrl(url: string) {
  if (url.startsWith('http')) return url
  const baseUrl = import.meta.env.VITE_SERVER_BASEURL || ''
  return `${baseUrl}${url}`
}

export function setupInterceptors() {
  uni.addInterceptor('request', {
    invoke(options) {
      const appStore = useAppStore()
      options.url = normalizeUrl(String(options.url))
      options.timeout = options.timeout || 30000
      options.header = {
        ...options.header,
        Authorization: appStore.token ? `Bearer ${appStore.token}` : '',
        'X-Client': 'zhiqiyun-miniapp',
      }
      return options
    },
  })
}
