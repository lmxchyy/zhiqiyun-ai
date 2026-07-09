export function switchTab(url: string) {
  uni.switchTab({ url })
}

export function showMockToast(title = '当前为前端 MVP，接口已预留') {
  uni.showToast({
    title,
    icon: 'none',
  })
}
