export const tabRoutes = {
  home: '/pages/index/index',
  create: '/pages/create/index',
  works: '/pages/works/index',
  agents: '/pages/agents/index',
  profile: '/pages/profile/index',
} as const

export type TabRouteKey = keyof typeof tabRoutes
