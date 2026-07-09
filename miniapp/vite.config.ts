import path from 'node:path'
import process from 'node:process'
import Uni from '@uni-helper/plugin-uni'
import UniComponents from '@uni-helper/vite-plugin-uni-components'
import UniLayouts from '@uni-helper/vite-plugin-uni-layouts'
import UniManifest from '@uni-helper/vite-plugin-uni-manifest'
import UniPages from '@uni-helper/vite-plugin-uni-pages'
import UniPlatform from '@uni-helper/vite-plugin-uni-platform'
import UnoCSS from 'unocss/vite'
import AutoImport from 'unplugin-auto-import/vite'
import { defineConfig, loadEnv } from 'vite'
import ViteRestart from 'vite-plugin-restart'

export default defineConfig(({ mode }) => {
  const envDir = path.resolve(process.cwd(), 'env')
  const env = loadEnv(mode, envDir)

  return {
    envDir,
    base: env.VITE_APP_PUBLIC_BASE || '/',
    plugins: [
      UniLayouts(),
      UniPlatform(),
      UniManifest(),
      UniComponents({
        extensions: ['vue'],
        deep: true,
        directoryAsNamespace: false,
        dts: 'src/types/components.d.ts',
      }),
      UniPages({
        exclude: ['**/components/**/**.*'],
        dts: 'src/types/uni-pages.d.ts',
      }),
      Uni(),
      UnoCSS(),
      AutoImport({
        imports: ['vue', 'uni-app'],
        dts: 'src/types/auto-import.d.ts',
        dirs: ['src/hooks', 'src/stores'],
        vueTemplate: true,
      }),
      ViteRestart({
        restart: ['vite.config.ts', 'pages.config.ts', 'manifest.config.ts', 'uno.config.ts'],
      }),
    ],
    resolve: {
      alias: {
        '@': path.resolve(process.cwd(), 'src'),
      },
    },
    server: {
      host: '0.0.0.0',
      hmr: true,
      port: Number.parseInt(env.VITE_APP_PORT || '5175', 10),
      proxy: env.VITE_APP_PROXY_ENABLE === 'true'
        ? {
            [env.VITE_APP_PROXY_PREFIX || '/api']: {
              target: env.VITE_SERVER_BASEURL,
              changeOrigin: true,
            },
          }
        : undefined,
    },
    build: {
      target: 'es6',
      sourcemap: false,
    },
  }
})
