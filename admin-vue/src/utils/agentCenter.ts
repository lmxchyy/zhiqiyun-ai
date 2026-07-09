export type AgentWorkspaceMessage = { role: "assistant" | "user"; text: string };

export type AgentCenterOpenable = {
  name: string;
  desc: string;
  type?: string;
  status?: string;
  model?: string;
  knowledge?: string;
  calls?: string;
  updated?: string;
  avatar?: string;
  icon?: string;
  tone?: string;
  agentKey?: string;
  officecli?: boolean;
  featured?: boolean;
  action?: string;
};

export type AgentCenterWorkspace = Required<Pick<AgentCenterOpenable, "name" | "desc" | "type" | "status" | "model" | "knowledge" | "calls" | "updated" | "avatar" | "tone">> & {
  agentKey: string;
  modeLabel: string;
  headline: string;
  prompt: string;
  toolTags: string[];
  quickActions: string[];
  sampleMessages: AgentWorkspaceMessage[];
};

type AgentCenterWorkspaceProfile = Pick<AgentCenterWorkspace, "modeLabel" | "headline" | "prompt" | "toolTags" | "quickActions" | "sampleMessages">;

export const agentCenterTemplates: AgentCenterOpenable[] = [
  { name: "OfficeCLI 文档智能体", desc: "Word / Excel / PPT 创建、读取、渲染与导出", icon: "DOC", tone: "orange", featured: true, action: "进入", agentKey: "officecli", officecli: true },
  { name: "基础对话智能体", desc: "通用问答，适合各类场景", icon: "Q", tone: "purple", action: "进入", agentKey: "chat" },
  { name: "企业知识库智能体", desc: "基于知识库，精准问答", icon: "K", tone: "green", action: "进入", agentKey: "knowledge" },
  { name: "销售助手", desc: "销售话术、客户跟进助力", icon: "S", tone: "orange", action: "进入", agentKey: "sales" },
  { name: "客服助手", desc: "7x24 智能客服，解答问题", icon: "C", tone: "purple", action: "进入", agentKey: "service" },
  { name: "招商助手", desc: "招商政策解答与线索收集", icon: "B", tone: "blue", action: "进入", agentKey: "investment" },
  { name: "内部 SOP 助手", desc: "制度、流程、审批查询", icon: "P", tone: "purple", action: "进入", agentKey: "sop" },
  { name: "表单收集助手", desc: "表单收集、结果采集", icon: "F", tone: "green", action: "进入", agentKey: "form" }
];

export const agentCenterRows: AgentCenterOpenable[] = [
  { name: "OfficeCLI 文档智能体", desc: "管理 Word/Excel/PPT 任务", type: "文档工具", status: "已接入", model: "gpt-4o-mini", knowledge: "Office 模板库", calls: "326", updated: "07-09 10:20", avatar: "DOC", tone: "orange", agentKey: "officecli", officecli: true },
  { name: "产品知识助手", desc: "解答业务问题，提升协作效率", type: "知识库", status: "已发布", model: "gpt-4o-mini", knowledge: "产品知识库", calls: "1,298", updated: "2024-06-12 14:30", avatar: "P", tone: "purple", agentKey: "knowledge" },
  { name: "销售跟进助手", desc: "解答业务问题，提升协作效率", type: "销售助手", status: "已发布", model: "gpt-4o", knowledge: "销售资料库", calls: "856", updated: "2024-06-12 10:15", avatar: "S", tone: "orange", agentKey: "sales" },
  { name: "智能客服助手", desc: "解答业务问题，提升协作效率", type: "客服助手", status: "已发布", model: "moonshot-v1", knowledge: "客服知识库", calls: "2,350", updated: "2024-06-11 16:45", avatar: "C", tone: "blue", agentKey: "service" },
  { name: "招商政策助手", desc: "解答业务问题，提升协作效率", type: "招商助手", status: "草稿", model: "gpt-4o-mini", knowledge: "招商资料库", calls: "128", updated: "2024-06-11 09:20", avatar: "B", tone: "orange", agentKey: "investment" },
  { name: "内部制度助手", desc: "解答业务问题，提升协作效率", type: "SOP 助手", status: "已停用", model: "qwen-max", knowledge: "内部制度库", calls: "312", updated: "2024-06-10 18:30", avatar: "P", tone: "purple", agentKey: "sop" },
  { name: "活动报名助手", desc: "解答业务问题，提升协作效率", type: "表单助手", status: "已发布", model: "gpt-4o-mini", knowledge: "-", calls: "689", updated: "2024-06-10 14:12", avatar: "F", tone: "green", agentKey: "form" },
  { name: "订单查询助手", desc: "解答业务问题，提升协作效率", type: "API 助手", status: "草稿", model: "gpt-4o-mini", knowledge: "-", calls: "56", updated: "2024-06-09 11:05", avatar: "API", tone: "blue", agentKey: "api" }
];

export const agentCenterMobileRows = [agentCenterRows[0], agentCenterRows[3]];

const agentCenterWorkspaceProfiles: Record<string, AgentCenterWorkspaceProfile> = {
  chat: {
    modeLabel: "通用对话智能体",
    headline: "多轮问答、任务拆解与内容生成",
    prompt: "帮我把今天的产品需求整理成三条优先级，并给出下一步行动。",
    toolTags: ["多轮对话", "内容生成", "任务拆解", "上下文记忆"],
    quickActions: ["整理会议纪要", "生成运营文案", "拆解待办事项"],
    sampleMessages: [
      { role: "assistant", text: "你好，我可以帮你做通用问答、文案生成和任务拆解。把目标发给我，我会先梳理结构再给出结果。" }
    ]
  },
  knowledge: {
    modeLabel: "知识库智能体",
    headline: "基于企业资料进行可追溯问答",
    prompt: "查询产品知识库，说明当前会员套餐的权益差异和适用客户。",
    toolTags: ["知识库检索", "引用来源", "权限过滤", "答案复核"],
    quickActions: ["查询产品资料", "生成 FAQ", "比对政策差异"],
    sampleMessages: [
      { role: "assistant", text: "我会优先检索已绑定知识库，并在回答里标注来源，适合产品资料、制度文档和客户问题。" }
    ]
  },
  sales: {
    modeLabel: "销售助手",
    headline: "线索跟进、异议处理与话术生成",
    prompt: "客户觉得价格偏高，请生成一段顾问式跟进话术，语气专业但不强推。",
    toolTags: ["销售话术", "客户画像", "异议处理", "跟进计划"],
    quickActions: ["生成跟进话术", "分析客户意向", "制定成交路径"],
    sampleMessages: [
      { role: "assistant", text: "我可以根据客户阶段生成跟进话术，并把下一次触达目标、风险点和成交信号拆开。" }
    ]
  },
  service: {
    modeLabel: "客服助手",
    headline: "7x24 问题解答与工单分流",
    prompt: "用户反馈生成失败但点数已扣，请给出客服回复和后续处理步骤。",
    toolTags: ["客服问答", "工单分流", "情绪安抚", "售后流程"],
    quickActions: ["回复售后问题", "创建工单摘要", "识别高优先级问题"],
    sampleMessages: [
      { role: "assistant", text: "我会先安抚用户，再收集订单号、任务 ID 和错误截图，必要时转人工或补偿点数。" }
    ]
  },
  investment: {
    modeLabel: "招商助手",
    headline: "招商政策解读与线索收集",
    prompt: "为一个咨询加盟政策的客户生成首轮回复，并收集预算、区域和资源情况。",
    toolTags: ["招商政策", "线索收集", "资格判断", "合作方案"],
    quickActions: ["生成招商回复", "整理客户画像", "输出合作建议"],
    sampleMessages: [
      { role: "assistant", text: "我可以解释招商政策，并引导客户补充区域、预算、资源和预期合作模式。" }
    ]
  },
  sop: {
    modeLabel: "SOP 助手",
    headline: "制度流程查询与执行检查",
    prompt: "查询客户退款流程，并列出需要运营、财务分别确认的事项。",
    toolTags: ["流程查询", "制度问答", "审批检查", "操作清单"],
    quickActions: ["查询制度流程", "生成执行清单", "检查审批材料"],
    sampleMessages: [
      { role: "assistant", text: "我会按制度步骤拆成操作清单，并提醒你哪些节点需要审批或留痕。" }
    ]
  },
  form: {
    modeLabel: "表单助手",
    headline: "表单收集、字段校验与结果归档",
    prompt: "设计一个活动报名表，包含姓名、手机号、公司、意向服务和备注字段。",
    toolTags: ["表单生成", "字段校验", "结果归档", "数据汇总"],
    quickActions: ["创建报名表", "汇总表单结果", "检查缺失字段"],
    sampleMessages: [
      { role: "assistant", text: "我可以根据业务目标生成表单字段，并帮助你检查必填项、选项和结果归档规则。" }
    ]
  },
  api: {
    modeLabel: "API 助手",
    headline: "外部接口查询、字段映射与调用排障",
    prompt: "帮我检查订单查询接口需要哪些入参，并输出一份前端调用说明。",
    toolTags: ["接口查询", "字段映射", "调用日志", "错误排查"],
    quickActions: ["生成接口说明", "排查调用失败", "整理字段映射"],
    sampleMessages: [
      { role: "assistant", text: "我会围绕接口入参、鉴权、返回字段和错误码生成说明，便于前端或运营排查。" }
    ]
  }
};

export const agentCenterMetrics = [
  { label: "已发布智能体", value: "7", trend: "+18.5%" },
  { label: "本周对话", value: "12.5K", trend: "+22.1%" },
  { label: "知识库命中", value: "894", trend: "+9.8%" },
  { label: "工具调用", value: "1.8K", trend: "+31.4%" }
];

export const agentCenterTrend = [
  { label: "06-06", height: "58%" },
  { label: "06-07", height: "38%" },
  { label: "06-08", height: "50%" },
  { label: "06-09", height: "30%" },
  { label: "06-10", height: "20%" },
  { label: "06-11", height: "34%" },
  { label: "06-12", height: "48%" },
  { label: "", height: "78%" }
];

export const agentCenterRanking = [
  { name: "智能客服助手", calls: "2,350" },
  { name: "产品知识助手", calls: "1,298" },
  { name: "销售跟进助手", calls: "856" },
  { name: "活动报名助手", calls: "689" },
  { name: "OfficeCLI 文档智能体", calls: "326" }
];

export const agentCenterShortcuts = [
  { label: "模板库", icon: "TPL" },
  { label: "工具配置", icon: "API" },
  { label: "知识库", icon: "KB" },
  { label: "调用日志", icon: "LOG" }
];

export function agentKeyForItem(item: AgentCenterOpenable) {
  if (item.officecli || item.name === "OfficeCLI 文档智能体") return "officecli";
  return item.agentKey || item.name;
}

export function isOfficeCLIItem(item: AgentCenterOpenable) {
  return agentKeyForItem(item) === "officecli";
}

export function findAgentCenterOpenable(agentKey: string) {
  return agentCenterRows.find((agent) => agent.agentKey === agentKey)
    || agentCenterTemplates.find((template) => template.agentKey === agentKey);
}

export function buildAgentCenterWorkspace(item: AgentCenterOpenable): AgentCenterWorkspace {
  const agentKey = agentKeyForItem(item);
  const row = agentCenterRows.find((candidate) => candidate.agentKey === agentKey || candidate.name === item.name);
  const profile = agentCenterWorkspaceProfiles[agentKey] || agentCenterWorkspaceProfiles.chat;
  const name = item.name || row?.name || profile.modeLabel;
  const desc = item.desc || row?.desc || "进入智能体工作台，进行测试、配置和运行状态查看。";
  return {
    agentKey,
    name,
    desc,
    type: item.type || row?.type || profile.modeLabel,
    status: item.status || row?.status || "待配置",
    model: item.model || row?.model || "gpt-4o-mini",
    knowledge: item.knowledge || row?.knowledge || "未绑定",
    calls: item.calls || row?.calls || "0",
    updated: item.updated || row?.updated || "刚刚",
    avatar: item.avatar || item.icon || row?.avatar || name.slice(0, 1),
    tone: item.tone || row?.tone || "purple",
    modeLabel: profile.modeLabel,
    headline: profile.headline,
    prompt: profile.prompt,
    toolTags: profile.toolTags,
    quickActions: profile.quickActions,
    sampleMessages: profile.sampleMessages
  };
}
