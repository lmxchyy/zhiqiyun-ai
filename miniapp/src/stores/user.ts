import { defineStore } from 'pinia'
import { mockPlans, mockUser } from '@/mock/home'

export const useUserStore = defineStore('user', {
  state: () => ({
    profile: mockUser,
    plans: mockPlans,
  }),
  getters: {
    pointsText: state => state.profile.points.toLocaleString('zh-CN'),
  },
  actions: {
    addPoints(points: number) {
      this.profile.points += points
    },
  },
})
