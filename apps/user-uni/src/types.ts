export type {
  Asset,
  AuthResponse,
  AuthUser,
  ChannelAgent,
  ChannelCenterResponse,
  ChannelCenterSummary,
  ChannelCommission,
  ChannelOrder,
  ChannelUsageEvent,
  ChannelWithdrawal,
  GenerationTask,
  ModelInfo,
  PointAccount,
  PointAccountResponse,
  ReferenceImage,
  TaskStatus,
  WorkspaceRole,
} from '@xianzhi/shared-types'

export type MineView = 'overview' | 'agent-upgrade' | 'recharge-history' | 'usage-details' | 'identity-permissions' | 'invite-promotion'

export interface MinePurchaseOption {
  kind: 'recharge' | 'agent'
  id: string
  amountCents: number
  points: number
  recommended?: boolean
}
