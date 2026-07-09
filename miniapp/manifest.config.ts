import { defineManifestConfig } from '@uni-helper/vite-plugin-uni-manifest'

export default defineManifestConfig({
  name: '知启云 AI',
  appid: '__UNI__ZHIQIYUN_AI_MINIAPP',
  description: '知启云 AI / 先知 AI SaaS 小程序用户端',
  versionName: '0.1.0',
  versionCode: '10',
  transformPx: false,
  h5: {
    router: {
      mode: 'hash',
    },
    devServer: {
      port: 5175,
    },
  },
  'mp-weixin': {
    appid: process.env.VITE_WX_APPID || 'wxf02d04d39afb7c6d',
    setting: {
      urlCheck: false,
      es6: true,
      postcss: true,
      minified: true,
    },
  },
})
