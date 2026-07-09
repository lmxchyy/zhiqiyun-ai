import { request } from './request'

export interface MiniAuthUser {
  id?: string
  email?: string
  name?: string
  role?: string
}

export interface MiniAuthResponse {
  accessToken: string
  defaultModule?: string
  defaultRoute?: string
  workspace?: string
  user?: MiniAuthUser
}

function requestWeChatCode() {
  return new Promise<string>((resolve, reject) => {
    // #ifdef MP-WEIXIN
    uni.login({
      provider: 'weixin',
      success(result) {
        if (result.code) {
          resolve(result.code)
          return
        }
        reject(new Error('wx.login returned no code'))
      },
      fail(error) {
        reject(error)
      },
    })
    // #endif

    // #ifndef MP-WEIXIN
    resolve('mock-devtools-code')
    // #endif
  })
}

export async function loginByWechatMiniProgram() {
  const code = await requestWeChatCode()
  return request<MiniAuthResponse, { code: string }>({
    url: '/api/v1/auth/wechat-mini-program/login',
    method: 'POST',
    data: { code },
  })
}

export function loginByPassword(email: string, password: string) {
  return request<MiniAuthResponse, { email: string, password: string }>({
    url: '/api/v1/auth/login',
    method: 'POST',
    data: { email, password },
  })
}
