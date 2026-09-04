package httpserver

import "time"

// AnalyticsOverviewResponse 聚合概览：给首页 5-6 张核心卡片用
type AnalyticsOverviewResponse struct {
	NewUsersToday     int     `json:"newUsersToday"`     // 日新增用户
	DAU               int     `json:"dau"`               // 日活
	WAU               int     `json:"wau"`               // 周活
	MAU               int     `json:"mau"`               // 月活
	AIUsersToday      int     `json:"aiUsersToday"`      // 今日 AI 用户数
	ImagesGenerated   int     `json:"imagesGenerated"`   // 今日图片生成量
	VideosGenerated   int     `json:"videosGenerated"`   // 今日视频生成量
	PointsConsumed    int64   `json:"pointsConsumed"`    // 今日积分消耗（分）
	TokensUsed        int64   `json:"tokensUsed"`        // 今日 Token 使用量
	RevenueTodayCents int64   `json:"revenueTodayCents"` // 今日收入（分）
	CostTodayCents    int64   `json:"costTodayCents"`    // 今日上游成本（分）
	FailedTasksToday  int     `json:"failedTasksToday"`  // 今日失败任务数
	ProcessingTasks   int     `json:"processingTasks"`   // 当前处理中任务数
	ExceptionCount    int     `json:"exceptionCount"`    // 当前待处理/处理中异常任务数
	SuccessRate       float64 `json:"successRate"`       // 今日整体成功率
	AvgLatencyMs      int     `json:"avgLatencyMs"`      // 今日平均延迟(ms)
}

// AnalyticsUsersResponse 用户维度指标
type AnalyticsUsersResponse struct {
	NewUsersToday  int           `json:"newUsersToday"`
	DAU            int           `json:"dau"`
	WAU            int           `json:"wau"`
	MAU            int           `json:"mau"`
	AIUsersToday   int           `json:"aiUsersToday"`
	TotalUsers     int           `json:"totalUsers"`
	ActiveUsers7d  int           `json:"activeUsers7d"`
	ChurnedUsers7d int           `json:"churnedUsers7d"`
	NewUsersTrend  []DailyMetric `json:"newUsersTrend"` // 7 日新增趋势
	DAUTrend       []DailyMetric `json:"dauTrend"`      // 7 日日活趋势
	WAUTrend       []DailyMetric `json:"wauTrend"`      // 7 日周活趋势
	MAUTrend       []DailyMetric `json:"mauTrend"`      // 7 日月活趋势
	AIUsersTrend   []DailyMetric `json:"aiUsersTrend"`  // 7 日 AI 用户趋势
}

// AnalyticsGenerationResponse 生成任务维度指标
type AnalyticsGenerationResponse struct {
	ImagesToday     int              `json:"imagesToday"`
	VideosToday     int              `json:"videosToday"`
	TotalTasksToday int              `json:"totalTasksToday"`
	SuccessRate     float64          `json:"successRate"`
	AvgLatencyMs    int              `json:"avgLatencyMs"`
	FailedTasks     int              `json:"failedTasks"`
	ByType          []TypeMetric     `json:"byType"`       // 按类型分布
	ByModel         []ModelMetric    `json:"byModel"`      // 按模型分布
	ByProvider      []ProviderMetric `json:"byProvider"`   // 按供应商分布
	TasksTrend      []DailyMetric    `json:"tasksTrend"`   // 7 日任务量趋势
	SuccessTrend    []DailyMetric    `json:"successTrend"` // 7 日成功率趋势
	LatencyTrend    []DailyMetric    `json:"latencyTrend"` // 7 日延迟趋势
}

// AnalyticsTokensResponse Token 使用维度
type AnalyticsTokensResponse struct {
	TokensToday int64         `json:"tokensToday"`
	Tokens7d    int64         `json:"tokens7d"`
	Tokens30d   int64         `json:"tokens30d"`
	ByModel     []ModelMetric `json:"byModel"`     // 按模型 Token 用量
	ByUser      []UserMetric  `json:"byUser"`      // Top 用户 Token 用量
	TokensTrend []DailyMetric `json:"tokensTrend"` // 7 日 Token 趋势
}

// AnalyticsPointsResponse 积分维度
type AnalyticsPointsResponse struct {
	ConsumedToday  int64         `json:"consumedToday"`  // 今日消耗
	RechargedToday int64         `json:"rechargedToday"` // 今日充值
	GrantedToday   int64         `json:"grantedToday"`   // 今日赠送
	FrozenToday    int64         `json:"frozenToday"`    // 今日冻结
	ReleasedToday  int64         `json:"releasedToday"`  // 今日释放
	NetChangeToday int64         `json:"netChangeToday"` // 今日净变化
	TotalAvailable int64         `json:"totalAvailable"` // 全平台可用余额
	TotalFrozen    int64         `json:"totalFrozen"`    // 全平台冻结余额
	ConsumedTrend  []DailyMetric `json:"consumedTrend"`  // 7 日消耗趋势
	RechargedTrend []DailyMetric `json:"rechargedTrend"` // 7 日充值趋势
	ByType         []TypeMetric  `json:"byType"`         // 按交易类型分布
}

// AnalyticsModelsResponse 模型榜单
type AnalyticsModelsResponse struct {
	Models []ModelMetric `json:"models"` // 按调用量排序的模型榜单
}

// AnalyticsProvidersResponse 供应商状态
type AnalyticsProvidersResponse struct {
	Providers []ProviderMetric `json:"providers"` // 供应商成功率/延迟/成本
}

// AnalyticsTrendsResponse 7 日趋势（汇总）
type AnalyticsTrendsResponse struct {
	NewUsers []DailyMetric `json:"newUsers"`
	DAU      []DailyMetric `json:"dau"`
	WAU      []DailyMetric `json:"wau"`
	MAU      []DailyMetric `json:"mau"`
	AIUsers  []DailyMetric `json:"aiUsers"`
	Images   []DailyMetric `json:"images"`
	Videos   []DailyMetric `json:"videos"`
	Points   []DailyMetric `json:"points"`
	Tokens   []DailyMetric `json:"tokens"`
	Revenue  []DailyMetric `json:"revenue"`
	Cost     []DailyMetric `json:"cost"`
	Tasks    []DailyMetric `json:"tasks"`
	Success  []DailyMetric `json:"success"`
	Latency  []DailyMetric `json:"latency"`
	Failed   []DailyMetric `json:"failed"`
}

// 通用子结构
type DailyMetric struct {
	Date  string  `json:"date"`  // YYYY-MM-DD (Asia/Shanghai)
	Value float64 `json:"value"` // 根据指标类型可能是 int 或 float
}

type TypeMetric struct {
	Type  string  `json:"type"` // TEXT_TO_IMAGE, IMAGE_TO_IMAGE, TEXT_TO_VIDEO, IMAGE_TO_VIDEO, CHAT
	Count int     `json:"count"`
	Rate  float64 `json:"rate"` // 占比
}

type ModelMetric struct {
	ModelCode      string  `json:"modelCode"`
	CallCount      int     `json:"callCount"`
	SuccessCount   int     `json:"successCount"`
	SuccessRate    float64 `json:"successRate"`
	AvgLatencyMs   int     `json:"avgLatencyMs"`
	TotalCostCents int64   `json:"totalCostCents"`
	TotalTokens    int64   `json:"totalTokens,omitempty"`
}

type ProviderMetric struct {
	ProviderCode   string  `json:"providerCode"`
	CallCount      int     `json:"callCount"`
	SuccessCount   int     `json:"successCount"`
	SuccessRate    float64 `json:"successRate"`
	AvgLatencyMs   int     `json:"avgLatencyMs"`
	TotalCostCents int64   `json:"totalCostCents"`
}

type UserMetric struct {
	UserID   string `json:"userId"`
	UserName string `json:"userName"`
	Value    int64  `json:"value"`
}

// AnalyticsQueryParams 统一查询参数
type AnalyticsQueryParams struct {
	Days      int            `form:"days"`      // 默认 7，最大 90
	Timezone  string         `form:"timezone"`  // 默认 Asia/Shanghai
	StartDate string         `form:"startDate"` // 可选：显式起始日期 YYYY-MM-DD
	EndDate   string         `form:"endDate"`   // 可选：显式结束日期 YYYY-MM-DD
	Scope     AnalyticsScope `json:"-"`         // 服务端注入的数据范围，绝对禁止客户端直接覆盖
}

// 内部使用：解析后的时间范围
type analyticsTimeRange struct {
	start time.Time
	end   time.Time
	loc   *time.Location
}
