export type {
  Asset,
  AppRole,
  AuthResponse,
  AuthUser,
  ChannelAgent,
  ChannelCenterResponse,
  ChannelCenterSummary,
  ChannelCommission,
  ChannelOrder,
  ChannelUsageEvent,
  ChannelWithdrawal,
  CurrentContextRequest,
  EnterpriseContext,
  EnterpriseContextsResponse,
  EnterpriseDataScope,
  EnterpriseWalletSummary,
  GenerationTask,
  ModelInfo,
  PointAccount,
  PointAccountResponse,
  ReferenceImage,
  TaskStatus,
  UserAccessProfile,
  WorkspaceRole,
  UserContextType,
} from '@xianzhi/shared-types'

export type MineView = 'overview' | 'agent-upgrade' | 'recharge-history' | 'usage-details' | 'role-permissions' | 'invite-promotion'

export interface MinePurchaseOption {
  kind: 'recharge' | 'agent'
  id: string
  amountCents: number
  points: number
  recommended?: boolean
}
