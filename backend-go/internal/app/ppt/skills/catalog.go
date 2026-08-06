// Package skills contains the server-side catalog of built-in PPT skills.
package skills

// Skill is the complete server-side configuration for a PPT skill.
// SystemPrompt must only be used by server-side orchestration.
type Skill struct {
	Code             string
	Name             string
	Description      string
	SystemPrompt     string `json:"-"`
	OutlineSchema    string `json:"-"`
	PreferredLayouts []string
	MaxSlides        int
}

// Summary is the public representation of a Skill. It intentionally omits
// server-only configuration such as the system prompt and outline schema.
type Summary struct {
	Code             string   `json:"code"`
	Name             string   `json:"name"`
	Description      string   `json:"description"`
	PreferredLayouts []string `json:"preferredLayouts"`
	MaxSlides        int      `json:"maxSlides"`
}

const standardOutlineSchema = `{"type":"object","required":["title","pages"],"properties":{"title":{"type":"string"},"pages":{"type":"array","minItems":1,"items":{"type":"object","required":["title","summary","bullets"],"properties":{"title":{"type":"string"},"summary":{"type":"string"},"bullets":{"type":"array","items":{"type":"string"}}}}}}}`

var catalog = []Skill{
	{
		Code:             "general",
		Name:             "通用演示",
		Description:      "适用于日常汇报、说明与通用主题表达。",
		SystemPrompt:     "Create a clear, audience-appropriate presentation with a concise narrative and actionable takeaways.",
		OutlineSchema:    standardOutlineSchema,
		PreferredLayouts: []string{"title_and_bullets", "section_header"},
		MaxSlides:        30,
	},
	{
		Code:             "pitch_deck",
		Name:             "融资路演",
		Description:      "面向投资人的商业机会、市场与融资叙事。",
		SystemPrompt:     "Create an investor pitch deck that explains the problem, solution, market, traction, business model, team, and ask with evidence-based claims.",
		OutlineSchema:    standardOutlineSchema,
		PreferredLayouts: []string{"hero_statement", "metrics"},
		MaxSlides:        20,
	},
	{
		Code:             "weekly_report",
		Name:             "周报",
		Description:      "总结周期内进展、风险、数据与下周计划。",
		SystemPrompt:     "Create a management-ready weekly report that distinguishes completed work, measurable progress, blockers, risks, and next-week commitments.",
		OutlineSchema:    standardOutlineSchema,
		PreferredLayouts: []string{"status_dashboard", "title_and_bullets"},
		MaxSlides:        15,
	},
	{
		Code:             "sales_proposal",
		Name:             "销售方案",
		Description:      "围绕客户痛点、方案价值与落地路径组织提案。",
		SystemPrompt:     "Create a customer-focused sales proposal that connects stated needs to differentiated value, delivery scope, proof points, and a concrete next step.",
		OutlineSchema:    standardOutlineSchema,
		PreferredLayouts: []string{"problem_solution", "timeline"},
		MaxSlides:        25,
	},
	{
		Code:             "training",
		Name:             "培训课件",
		Description:      "用于结构化教学、知识讲解和练习引导。",
		SystemPrompt:     "Create a learning-oriented training deck with explicit objectives, progressive concepts, practical examples, and a recap or practice activity.",
		OutlineSchema:    standardOutlineSchema,
		PreferredLayouts: []string{"learning_objectives", "step_by_step"},
		MaxSlides:        40,
	},
	{
		Code:             "product_launch",
		Name:             "产品发布",
		Description:      "介绍产品定位、关键能力、价值与发布计划。",
		SystemPrompt:     "Create a product launch deck that starts with customer value, demonstrates the product story, highlights key capabilities, and closes with launch actions.",
		OutlineSchema:    standardOutlineSchema,
		PreferredLayouts: []string{"product_hero", "feature_grid"},
		MaxSlides:        20,
	},
	{
		Code:             "consulting",
		Name:             "咨询汇报",
		Description:      "用于问题诊断、分析洞察和建议落地。",
		SystemPrompt:     "Create a consulting-style deck that states the question, structures the analysis, separates evidence from inference, and ends with prioritized recommendations.",
		OutlineSchema:    standardOutlineSchema,
		PreferredLayouts: []string{"executive_summary", "insight_chart"},
		MaxSlides:        30,
	},
	{
		Code:             "meeting_summary",
		Name:             "会议纪要",
		Description:      "沉淀会议背景、讨论结论、待办与责任人。",
		SystemPrompt:     "Create a meeting summary that separates decisions, open questions, action items, owners, and deadlines without inventing missing commitments.",
		OutlineSchema:    standardOutlineSchema,
		PreferredLayouts: []string{"decision_log", "action_items"},
		MaxSlides:        12,
	},
}

// Resolve returns a copy of the skill configuration for code. Unknown codes do
// not fall back to another skill.
func Resolve(code string) (Skill, bool) {
	for _, skill := range catalog {
		if skill.Code == code {
			return cloneSkill(skill), true
		}
	}
	return Skill{}, false
}

// List returns public summaries for every built-in skill in catalog order.
func List() []Summary {
	summaries := make([]Summary, 0, len(catalog))
	for _, skill := range catalog {
		summaries = append(summaries, Summary{
			Code:             skill.Code,
			Name:             skill.Name,
			Description:      skill.Description,
			PreferredLayouts: cloneStrings(skill.PreferredLayouts),
			MaxSlides:        skill.MaxSlides,
		})
	}
	return summaries
}

func cloneSkill(skill Skill) Skill {
	skill.PreferredLayouts = cloneStrings(skill.PreferredLayouts)
	return skill
}

func cloneStrings(values []string) []string {
	return append([]string(nil), values...)
}
