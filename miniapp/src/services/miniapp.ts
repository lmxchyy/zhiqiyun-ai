import { mockAgents, mockFeatures, mockPlans, mockUser, mockWorks } from '@/mock/home'
import { businessSdk } from './core'
import type { AgentEntry, CreateDraft, FeatureEntry, MembershipPlan, UserProfile, WorkItem } from '@/types/domain'
import type { GenerationTask } from '@xianzhi/shared-types'

const wait = (ms = 120) => new Promise(resolve => setTimeout(resolve, ms))
const allowMockFallback = Boolean((import.meta as unknown as { env?: { DEV?: boolean } }).env?.DEV)

function assertMockFallback(error: unknown) {
  if (!allowMockFallback) throw error
}

function fallbackHome(): {
  user: UserProfile
  features: FeatureEntry[]
  works: WorkItem[]
  plans: MembershipPlan[]
} {
  return {
    user: mockUser,
    features: mockFeatures,
    works: mockWorks,
    plans: mockPlans,
  }
}

function workFromTask(task: GenerationTask, draft: CreateDraft): WorkItem {
  return {
    id: task.id || `task_${Date.now()}`,
    title: draft.mode === 'ppt' ? '新建 PPT 任务' : draft.mode === 'video' ? '新建视频任务' : '新建图片任务',
    type: draft.mode === 'video' ? 'video' : draft.mode === 'ppt' ? 'ppt' : 'image',
    status: String(task.status || '').toUpperCase() === 'FAILED' ? 'failed' : 'queued',
    model: task.model || draft.model,
    prompt: task.prompt || draft.prompt,
    createdAt: task.createdAt || 'just now',
  }
}

export async function getHomeOverview(): Promise<{
  user: UserProfile
  features: FeatureEntry[]
  works: WorkItem[]
  plans: MembershipPlan[]
}> {
  try {
    return await businessSdk.dashboard.getHomeOverview()
  }
  catch (error) {
    assertMockFallback(error)
    await wait()
    return fallbackHome()
  }
}

export async function getWorks(): Promise<WorkItem[]> {
  try {
    const works = await businessSdk.assets.getWorks()
    return works.length || !allowMockFallback ? works : mockWorks
  }
  catch (error) {
    assertMockFallback(error)
    await wait()
    return mockWorks
  }
}

export async function getAgents(): Promise<AgentEntry[]> {
  try {
    const agents = await businessSdk.agents.list()
    return agents.length || !allowMockFallback ? agents : mockAgents
  }
  catch (error) {
    assertMockFallback(error)
    await wait()
    return mockAgents
  }
}

export async function createGenerationTask(draft: CreateDraft): Promise<WorkItem> {
  try {
    const task = await businessSdk.generation.createTask(draft)
    return workFromTask(task, draft)
  }
  catch (error) {
    assertMockFallback(error)
    await wait(240)
    return {
      id: `mock_${Date.now()}`,
      title: draft.mode === 'ppt' ? '新建 PPT 任务' : draft.mode === 'video' ? '新建视频任务' : '新建图片任务',
      type: draft.mode === 'video' ? 'video' : draft.mode === 'ppt' ? 'ppt' : 'image',
      status: 'queued',
      model: draft.model,
      prompt: draft.prompt,
      createdAt: 'just now',
    }
  }
}
