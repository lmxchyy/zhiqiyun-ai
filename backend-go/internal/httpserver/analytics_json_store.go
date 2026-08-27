package httpserver

import "context"

// jsonStore 没有分析数仓支撑：analytics 接口在 JSON 存储模式下统一返回空聚合结果，
// 保证 platformStore 接口完整且 /api/admin/analytics/* 不会 500。
// V1 安全约束：只返回聚合数据，不暴露任何明细敏感信息。

func (s *jsonStore) AnalyticsOverview(ctx context.Context, params AnalyticsQueryParams) (AnalyticsOverviewResponse, error) {
	return AnalyticsOverviewResponse{}, nil
}

func (s *jsonStore) AnalyticsUsers(ctx context.Context, params AnalyticsQueryParams) (AnalyticsUsersResponse, error) {
	return AnalyticsUsersResponse{
		NewUsersTrend: []DailyMetric{},
		DAUTrend:      []DailyMetric{},
		WAUTrend:      []DailyMetric{},
		MAUTrend:      []DailyMetric{},
		AIUsersTrend:  []DailyMetric{},
	}, nil
}

func (s *jsonStore) AnalyticsGeneration(ctx context.Context, params AnalyticsQueryParams) (AnalyticsGenerationResponse, error) {
	return AnalyticsGenerationResponse{
		ByType:       []TypeMetric{},
		ByModel:      []ModelMetric{},
		ByProvider:   []ProviderMetric{},
		TasksTrend:   []DailyMetric{},
		SuccessTrend: []DailyMetric{},
		LatencyTrend: []DailyMetric{},
	}, nil
}

func (s *jsonStore) AnalyticsTokens(ctx context.Context, params AnalyticsQueryParams) (AnalyticsTokensResponse, error) {
	return AnalyticsTokensResponse{
		ByModel:     []ModelMetric{},
		ByUser:      []UserMetric{},
		TokensTrend: []DailyMetric{},
	}, nil
}

func (s *jsonStore) AnalyticsPoints(ctx context.Context, params AnalyticsQueryParams) (AnalyticsPointsResponse, error) {
	return AnalyticsPointsResponse{
		ConsumedTrend:  []DailyMetric{},
		RechargedTrend: []DailyMetric{},
		ByType:         []TypeMetric{},
	}, nil
}

func (s *jsonStore) AnalyticsModels(ctx context.Context, params AnalyticsQueryParams) (AnalyticsModelsResponse, error) {
	return AnalyticsModelsResponse{Models: []ModelMetric{}}, nil
}

func (s *jsonStore) AnalyticsProviders(ctx context.Context, params AnalyticsQueryParams) (AnalyticsProvidersResponse, error) {
	return AnalyticsProvidersResponse{Providers: []ProviderMetric{}}, nil
}

func (s *jsonStore) AnalyticsTrends(ctx context.Context, params AnalyticsQueryParams) (AnalyticsTrendsResponse, error) {
	return AnalyticsTrendsResponse{
		NewUsers: []DailyMetric{},
		DAU:      []DailyMetric{},
		WAU:      []DailyMetric{},
		MAU:      []DailyMetric{},
		AIUsers:  []DailyMetric{},
		Images:   []DailyMetric{},
		Videos:   []DailyMetric{},
		Points:   []DailyMetric{},
		Tokens:   []DailyMetric{},
		Revenue:  []DailyMetric{},
		Cost:     []DailyMetric{},
		Tasks:    []DailyMetric{},
		Success:  []DailyMetric{},
		Latency:  []DailyMetric{},
		Failed:   []DailyMetric{},
	}, nil
}
