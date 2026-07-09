import { authService } from './core'
import type { AuthResponse } from '@xianzhi/shared-types'

export type MiniAuthUser = AuthResponse['user']
export type MiniAuthResponse = AuthResponse

export function loginByWechatMiniProgram() {
  return authService.loginByWechatMiniProgram()
}

export function loginByPassword(email: string, password: string) {
  return authService.loginByPassword(email, password)
}
