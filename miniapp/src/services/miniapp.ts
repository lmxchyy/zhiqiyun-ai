import { mockAgents, mockFeatures, mockPlans, mockUser, mockWorks } from '@/mock/home'
import type { AgentEntry, CreateDraft, FeatureEntry, MembershipPlan, UserProfile, WorkItem } from '@/types/domain'

const wait = (ms = 120) => new Promise(resolve => setTimeout(resolve, ms))

export async function getHomeOverview(): Promise<{
  user: UserProfile
  features: FeatureEntry[]
  works: WorkItem[]
  plans: MembershipPlan[]
}> {
  await wait()
  return {
    user: mockUser,
    features: mockFeatures,
    works: mockWorks,
    plans: mockPlans,
  }
}

export async function getWorks(): Promise<WorkItem[]> {
  await wait()
  return mockWorks
}

export async function getAgents(): Promise<AgentEntry[]> {
  await wait()
  return mockAgents
}

export async function createGenerationTask(draft: CreateDraft): Promise<WorkItem> {
  await wait(240)
  return {
    id: `mock_${Date.now()}`,
    title: draft.mode === 'ppt' ? '新建 PPT 任务' : draft.mode === 'video' ? '新建视频任务' : '新建图片任务',
    type: draft.mode === 'video' ? 'video' : draft.mode === 'ppt' ? 'ppt' : 'image',
    status: 'queued',
    model: draft.model,
    prompt: draft.prompt,
    createdAt: '刚刚',
  }
}
