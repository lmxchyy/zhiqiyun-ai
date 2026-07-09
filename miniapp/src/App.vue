<script setup lang="ts">
import { onLaunch, onShow } from '@dcloudio/uni-app'
import { useAppStore } from '@/stores/app'

const appStore = useAppStore()

onLaunch(() => {
  appStore.bootstrap()
  setTimeout(() => {
    appStore.ensureAuthenticated()
  }, 0)
})

onShow(() => {
  appStore.syncSession()
  setTimeout(() => {
    appStore.ensureAuthenticated()
  }, 0)
})
</script>

<style lang="scss">
page {
  min-height: 100%;
  background: var(--color-bg-page);
  color: var(--color-text-primary);
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif;
}
</style>
