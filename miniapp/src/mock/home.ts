import type { AgentEntry, FeatureEntry, MembershipPlan, UserProfile, WorkItem } from '@/types/domain'

export const mockUser: UserProfile = {
  id: 'u_10001',
  name: '知启云体验用户',
  avatarText: '知',
  memberLevel: 'Pro 会员',
  points: 3280,
  agentEnabled: true,
}

export const mockFeatures: FeatureEntry[] = [
  { id: 'image', title: 'AI 生图', subtitle: '商品图、海报、营销素材', icon: '图', tone: 'primary', path: '/pages/create/index' },
  { id: 'video', title: 'AI 视频', subtitle: '短视频脚本到成片', icon: '视', tone: 'green', path: '/pages/create/index' },
  { id: 'ppt', title: 'PPT 文档', subtitle: '主题生成演示文稿', icon: 'P', tone: 'accent', path: '/pages/create/index' },
  { id: 'agent', title: 'Agent 对话', subtitle: '业务智能体协作', icon: 'AI', tone: 'dark', path: '/pages/agents/index' },
]

export const mockWorks: WorkItem[] = [
  {
    id: 'work_1001',
    title: '618 商品主图',
    type: 'image',
    status: 'succeeded',
    model: 'gpt-image-2',
    prompt: '高质感手机电商主图，白底，突出新品卖点',
    createdAt: '今天 09:42',
  },
  {
    id: 'work_1002',
    title: '品牌宣传短片',
    type: 'video',
    status: 'processing',
    model: 'doubao-seedance',
    prompt: '15 秒企业品牌宣传短片，科技感，蓝紫色调',
    createdAt: '今天 09:20',
  },
  {
    id: 'work_1003',
    title: '企业营销增长方案',
    type: 'ppt',
    status: 'succeeded',
    model: 'ppt-agent',
    prompt: 'AI 赋能企业营销增长方案，10 页，商务简约',
    createdAt: '昨天 18:06',
  },
]

export const mockAgents: AgentEntry[] = [
  { id: 'brand', title: '品牌 Agent', description: '沉淀品牌话术、视觉规范和内容策略。', tags: ['品牌定位', '内容策略'], tone: '#7D8DF6' },
  { id: 'ecommerce', title: '电商 Agent', description: '生成商品卖点、详情页结构和投放素材。', tags: ['商品卖点', '转化'], tone: '#FF771B' },
  { id: 'poster', title: '海报 Agent', description: '活动海报、朋友圈海报、节日营销图。', tags: ['海报', '营销'], tone: '#5A4DB2' },
  { id: 'product', title: '商品图 Agent', description: '主图、场景图和多角度商品展示。', tags: ['主图', '场景图'], tone: '#18A058' },
  { id: 'moments', title: '朋友圈海报 Agent', description: '适合私域转发的轻量海报文案。', tags: ['私域', '裂变'], tone: '#FF9F45' },
  { id: 'ppt', title: 'PPT Agent', description: '大纲、页面文案、图表和演示结构。', tags: ['大纲', '演示'], tone: '#2563EB' },
  { id: 'sales', title: '销售 Agent', description: '销售话术、客户跟进和异议处理。', tags: ['话术', '跟进'], tone: '#0F766E' },
  { id: 'knowledge', title: '企业知识库 Agent', description: '连接企业知识库，回答内部业务问题。', tags: ['知识库', '问答'], tone: '#334155' },
]

export const mockPlans: MembershipPlan[] = [
  {
    id: 'starter',
    name: '体验版',
    price: '¥19.9',
    points: 300,
    benefits: ['适合轻量体验', 'AI 生图 30 次', '作品云端保存'],
  },
  {
    id: 'pro',
    name: 'Pro 创作会员',
    price: '¥99',
    points: 1800,
    benefits: ['高频创作者首选', '优先生成队列', '模板广场权益'],
    recommended: true,
  },
]
