export type WorkType = 'image' | 'video' | 'ppt'
export type WorkStatus = 'queued' | 'processing' | 'succeeded' | 'failed'
export type CreateMode = 'image' | 'video' | 'ppt' | 'agent'

export interface UserProfile {
  id: string
  name: string
  avatarText: string
  memberLevel: string
  points: number
  agentEnabled: boolean
}

export interface FeatureEntry {
  id: string
  title: string
  subtitle: string
  icon: string
  tone: 'primary' | 'accent' | 'dark' | 'green'
  path?: string
}

export interface WorkItem {
  id: string
  title: string
  type: WorkType
  status: WorkStatus
  model: string
  prompt: string
  createdAt: string
}

export interface AgentEntry {
  id: string
  title: string
  description: string
  tags: string[]
  tone: string
}

export interface MembershipPlan {
  id: string
  name: string
  price: string
  points: number
  benefits: string[]
  recommended?: boolean
}

export interface CreateDraft {
  mode: CreateMode
  prompt: string
  model: string
  style: string
  size: string
  quality: string
  count: number
  referenceImages: string[]
}
