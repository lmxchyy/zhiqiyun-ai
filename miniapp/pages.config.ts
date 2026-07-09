import { defineUniPages } from '@uni-helper/vite-plugin-uni-pages'

const tabBarList = [
  {
    pagePath: 'pages/index/index',
    text: '首页',
  },
  {
    pagePath: 'pages/create/index',
    text: '创作',
  },
  {
    pagePath: 'pages/works/index',
    text: '作品',
  },
  {
    pagePath: 'pages/agents/index',
    text: 'Agent',
  },
  {
    pagePath: 'pages/profile/index',
    text: '我的',
  },
]

export default defineUniPages({
  globalStyle: {
    navigationStyle: 'custom',
    navigationBarTitleText: '知启云 AI',
    navigationBarTextStyle: 'black',
    navigationBarBackgroundColor: '#F7F8FC',
    backgroundColor: '#F7F8FC',
  },
  easycom: {
    autoscan: true,
    custom: {
      '^wd-(.*)': 'wot-design-uni/components/wd-$1/wd-$1.vue',
      '^zq-(.*)': '@/components/zq-$1.vue',
    },
  },
  tabBar: {
    color: '#9CA3AF',
    selectedColor: '#7D8DF6',
    backgroundColor: '#FFFFFF',
    borderStyle: 'black',
    list: tabBarList,
  },
})
