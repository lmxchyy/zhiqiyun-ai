import { defineManifestConfig } from '@uni-helper/vite-plugin-uni-manifest'

export default defineManifestConfig({
  name: '\u77e5\u542f\u4e91 AI',
  appid: '__UNI__ZHIQIYUN_AI_MINIAPP',
  description: '\u77e5\u542f\u4e91 AI / \u5148\u77e5 AI SaaS \u5c0f\u7a0b\u5e8f\u7528\u6237\u7aef',
  versionName: '0.1.0',
  versionCode: '10',
  transformPx: false,
  vueVersion: '3',
  h5: {
    router: {
      mode: 'hash',
    },
    devServer: {
      port: 5175,
    },
  },
  'app-plus': {
    usingComponents: true,
    nvueStyleCompiler: 'uni-app',
    compilerVersion: 3,
    splashscreen: {
      alwaysShowBeforeRender: true,
      waiting: true,
      autoclose: true,
      delay: 0,
    },
    modules: {},
    distribute: {
      android: {
        permissions: [],
      },
      ios: {},
      sdkConfigs: {},
    },
  },
  'app-harmony': {
    distribute: {},
  },
  'mp-harmony': {
    distribute: {},
  },
  'mp-weixin': {
    appid: process.env.VITE_WX_APPID || 'wxf02d04d39afb7c6d',
    setting: {
      urlCheck: false,
      es6: true,
      postcss: true,
      minified: true,
    },
    usingComponents: true,
  },
})
