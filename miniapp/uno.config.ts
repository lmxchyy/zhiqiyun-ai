import { presetUni } from '@uni-helper/unocss-preset-uni'
import { presetLegacyCompat } from '@unocss/preset-legacy-compat'
import { defineConfig, transformerDirectives, transformerVariantGroup } from 'unocss'

export default defineConfig({
  presets: [
    presetUni({ attributify: false }),
    presetLegacyCompat({
      commaStyleColorFunction: true,
      legacyColorSpace: true,
    }),
  ],
  transformers: [transformerDirectives(), transformerVariantGroup()],
  shortcuts: {
    center: 'flex items-center justify-center',
    'zq-card': 'rounded-16rpx bg-white shadow-sm',
    'zq-page': 'min-h-screen bg-[#F7F8FC] text-[#111827]',
  },
  safelist: [
    'i-carbon-home',
    'i-carbon-add',
    'i-carbon-image',
    'i-carbon-user',
    'i-carbon-bot',
  ],
  rules: [
    ['pt-safe', { 'padding-top': 'env(safe-area-inset-top)' }],
    ['pb-safe', { 'padding-bottom': 'env(safe-area-inset-bottom)' }],
  ],
  theme: {
    colors: {
      primary: 'var(--color-primary)',
      accent: 'var(--color-accent)',
    },
  },
})
