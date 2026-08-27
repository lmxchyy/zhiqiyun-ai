package httpserver

import (
	"context"
	"time"
)

type analyticsPostgresRange struct {
	start time.Time
	end   time.Time
	loc   *time.Location
}

func (s *postgresStore) analyticsPostgresRange(params AnalyticsQueryParams) analyticsPostgresRange {
	loc, err := time.LoadLocation(params.Timezone)
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	days := params.Days
	if days < 1 {
		days = 1
	}
	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	return analyticsPostgresRange{start: today.AddDate(0, 0, -days+1), end: today.AddDate(0, 0, 1), loc: loc}
}

func analyticsStatusSQL() string { return "upper(status) IN ('SUCCESS','SUCCEEDED','COMPLETED')" }

func (s *postgresStore) AnalyticsOverview(ctx context.Context, params AnalyticsQueryParams) (AnalyticsOverviewResponse, error) {
	rangeInfo := s.analyticsPostgresRange(params)
	var out AnalyticsOverviewResponse
	dayStart, dayEnd := rangeInfo.end.AddDate(0, 0, -1), rangeInfo.end
	queries := []struct {
		query string
		dest  any
	}{
		{`SELECT count(*) FROM xz_users WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND role <> 'SUPER_ADMIN'`, &out.NewUsersToday},
		{`SELECT count(DISTINCT user_id) FROM (SELECT user_id FROM xz_generation_tasks WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND upper(status)='SUCCEEDED' UNION SELECT user_id::text FROM agent_call_logs WHERE created_at >= $1 AND created_at < $2 UNION SELECT user_id FROM xz_billing_events WHERE NULLIF(occurred_at,'')::timestamptz >= $1 AND NULLIF(occurred_at,'')::timestamptz < $2) active`, &out.DAU},
		{`SELECT count(DISTINCT user_id) FROM (SELECT user_id FROM xz_generation_tasks WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND upper(status)='SUCCEEDED' UNION SELECT user_id::text FROM agent_call_logs WHERE created_at >= $1 AND created_at < $2 UNION SELECT user_id FROM xz_billing_events WHERE NULLIF(occurred_at,'')::timestamptz >= $1 AND NULLIF(occurred_at,'')::timestamptz < $2) active`, &out.WAU},
		{`SELECT count(DISTINCT user_id) FROM (SELECT user_id FROM xz_generation_tasks WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND upper(status)='SUCCEEDED' UNION SELECT user_id::text FROM agent_call_logs WHERE created_at >= $1 AND created_at < $2 UNION SELECT user_id FROM xz_billing_events WHERE NULLIF(occurred_at,'')::timestamptz >= $1 AND NULLIF(occurred_at,'')::timestamptz < $2) active`, &out.MAU},
		{`SELECT count(*) FROM xz_generation_tasks WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND type='TEXT_TO_IMAGE' AND upper(status)='SUCCEEDED'`, &out.ImagesGenerated},
		{`SELECT count(*) FROM xz_generation_tasks WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND type IN ('TEXT_TO_VIDEO','IMAGE_TO_VIDEO') AND upper(status)='SUCCEEDED'`, &out.VideosGenerated},
		{`SELECT coalesce(sum(point_cost),0) FROM xz_billing_events WHERE NULLIF(occurred_at,'')::timestamptz >= $1 AND NULLIF(occurred_at,'')::timestamptz < $2 AND point_cost > 0`, &out.PointsConsumed},
		{`SELECT coalesce(sum(abs(amount)),0) FROM xz_token_records WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND upper(change_type) IN ('USE','USAGE','CONSUME','CONSUMPTION')`, &out.TokensUsed},
		{`SELECT coalesce(sum(amount_cents),0) FROM xz_billing_events WHERE NULLIF(occurred_at,'')::timestamptz >= $1 AND NULLIF(occurred_at,'')::timestamptz < $2 AND amount_cents > 0 AND upper(metric_code) IN ('RECHARGE','PURCHASE','RECHARGE.POINTS')`, &out.RevenueTodayCents},
		{`SELECT coalesce(sum(supplier_cost),0)::bigint FROM xz_generation_tasks WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND upper(status)='SUCCEEDED'`, &out.CostTodayCents},
		{`SELECT count(*) FROM xz_generation_tasks WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND upper(status)='FAILED'`, &out.FailedTasksToday},
	}
	for _, item := range queries {
		if err := s.db.QueryRowContext(ctx, item.query, dayStart, dayEnd).Scan(item.dest); err != nil {
			return AnalyticsOverviewResponse{}, err
		}
	}
	var total, succeeded int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*), count(*) FILTER (WHERE upper(status)='SUCCEEDED') FROM xz_generation_tasks WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2`, dayStart, dayEnd).Scan(&total, &succeeded); err != nil {
		return AnalyticsOverviewResponse{}, err
	}
	if total > 0 {
		out.SuccessRate = float64(succeeded) / float64(total) * 100
	}
	if err := s.db.QueryRowContext(ctx, `SELECT coalesce(avg(latency_ms),0)::bigint FROM model_call_logs WHERE created_at >= $1 AND created_at < $2`, dayStart, dayEnd).Scan(&out.AvgLatencyMs); err != nil {
		return AnalyticsOverviewResponse{}, err
	}
	activeUsersQuery := `SELECT count(DISTINCT user_id) FROM (SELECT user_id FROM xz_generation_tasks WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND upper(status)='SUCCEEDED' UNION SELECT user_id::text FROM agent_call_logs WHERE created_at >= $1 AND created_at < $2 UNION SELECT user_id FROM xz_billing_events WHERE NULLIF(occurred_at,'')::timestamptz >= $1 AND NULLIF(occurred_at,'')::timestamptz < $2) active`
	if err := s.db.QueryRowContext(ctx, activeUsersQuery, dayEnd.AddDate(0, 0, -7), dayEnd).Scan(&out.WAU); err != nil {
		return AnalyticsOverviewResponse{}, err
	}
	if err := s.db.QueryRowContext(ctx, activeUsersQuery, dayEnd.AddDate(0, 0, -30), dayEnd).Scan(&out.MAU); err != nil {
		return AnalyticsOverviewResponse{}, err
	}
	return out, nil
}

func (s *postgresStore) AnalyticsModels(ctx context.Context, params AnalyticsQueryParams) (AnalyticsModelsResponse, error) {
	r := s.analyticsPostgresRange(params)
	rows, err := s.db.QueryContext(ctx, `SELECT model_code, count(*), count(*) FILTER (WHERE `+analyticsStatusSQL()+`), coalesce(avg(latency_ms),0)::bigint, coalesce(sum(cost_cents),0)::bigint FROM model_call_logs WHERE created_at >= $1 AND created_at < $2 GROUP BY model_code ORDER BY count(*) DESC, model_code`, r.start, r.end)
	if err != nil {
		return AnalyticsModelsResponse{}, err
	}
	defer rows.Close()
	result := AnalyticsModelsResponse{Models: []ModelMetric{}}
	for rows.Next() {
		var item ModelMetric
		if err := rows.Scan(&item.ModelCode, &item.CallCount, &item.SuccessCount, &item.AvgLatencyMs, &item.TotalCostCents); err != nil {
			return AnalyticsModelsResponse{}, err
		}
		if item.CallCount > 0 {
			item.SuccessRate = float64(item.SuccessCount) / float64(item.CallCount) * 100
		}
		result.Models = append(result.Models, item)
	}
	return result, rows.Err()
}

func (s *postgresStore) AnalyticsProviders(ctx context.Context, params AnalyticsQueryParams) (AnalyticsProvidersResponse, error) {
	r := s.analyticsPostgresRange(params)
	rows, err := s.db.QueryContext(ctx, `SELECT provider_code, count(*), count(*) FILTER (WHERE `+analyticsStatusSQL()+`), coalesce(avg(latency_ms),0)::bigint, coalesce(sum(cost_cents),0)::bigint FROM model_call_logs WHERE created_at >= $1 AND created_at < $2 GROUP BY provider_code ORDER BY count(*) DESC, provider_code`, r.start, r.end)
	if err != nil {
		return AnalyticsProvidersResponse{}, err
	}
	defer rows.Close()
	result := AnalyticsProvidersResponse{Providers: []ProviderMetric{}}
	for rows.Next() {
		var item ProviderMetric
		if err := rows.Scan(&item.ProviderCode, &item.CallCount, &item.SuccessCount, &item.AvgLatencyMs, &item.TotalCostCents); err != nil {
			return AnalyticsProvidersResponse{}, err
		}
		if item.CallCount > 0 {
			item.SuccessRate = float64(item.SuccessCount) / float64(item.CallCount) * 100
		}
		result.Providers = append(result.Providers, item)
	}
	return result, rows.Err()
}

func (s *postgresStore) AnalyticsTrends(ctx context.Context, params AnalyticsQueryParams) (AnalyticsTrendsResponse, error) {
	r := s.analyticsPostgresRange(params)
	var out AnalyticsTrendsResponse
	var err error
	if out.NewUsers, err = s.analyticsDaily(ctx, `SELECT to_char(NULLIF(created_at,'')::timestamptz AT TIME ZONE $3,'YYYY-MM-DD'), count(*) FROM xz_users WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND role <> 'SUPER_ADMIN' GROUP BY 1 ORDER BY 1`, r.start, r.end, r.loc.String()); err != nil {
		return out, err
	}
	if out.DAU, err = s.analyticsDaily(ctx, `SELECT to_char(ts AT TIME ZONE $3,'YYYY-MM-DD'), count(DISTINCT user_id) FROM (SELECT user_id, NULLIF(created_at,'')::timestamptz AS ts FROM xz_generation_tasks WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 UNION ALL SELECT user_id::text, created_at AS ts FROM agent_call_logs WHERE created_at >= $1 AND created_at < $2 UNION ALL SELECT user_id, NULLIF(occurred_at,'')::timestamptz AS ts FROM xz_billing_events WHERE NULLIF(occurred_at,'')::timestamptz >= $1 AND NULLIF(occurred_at,'')::timestamptz < $2) active GROUP BY 1 ORDER BY 1`, r.start, r.end, r.loc.String()); err != nil {
		return out, err
	}
	if out.AIUsers, err = s.analyticsDaily(ctx, `SELECT to_char(NULLIF(created_at,'')::timestamptz AT TIME ZONE $3,'YYYY-MM-DD'), count(DISTINCT user_id) FROM xz_generation_tasks WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND upper(status)='SUCCEEDED' GROUP BY 1 ORDER BY 1`, r.start, r.end, r.loc.String()); err != nil {
		return out, err
	}
	if out.Images, err = s.analyticsDaily(ctx, `SELECT to_char(NULLIF(created_at,'')::timestamptz AT TIME ZONE $3,'YYYY-MM-DD'), count(*) FROM xz_generation_tasks WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND type='TEXT_TO_IMAGE' AND upper(status)='SUCCEEDED' GROUP BY 1 ORDER BY 1`, r.start, r.end, r.loc.String()); err != nil {
		return out, err
	}
	if out.Videos, err = s.analyticsDaily(ctx, `SELECT to_char(NULLIF(created_at,'')::timestamptz AT TIME ZONE $3,'YYYY-MM-DD'), count(*) FROM xz_generation_tasks WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND type IN ('TEXT_TO_VIDEO','IMAGE_TO_VIDEO') AND upper(status)='SUCCEEDED' GROUP BY 1 ORDER BY 1`, r.start, r.end, r.loc.String()); err != nil {
		return out, err
	}
	if out.Points, err = s.analyticsDaily(ctx, `SELECT to_char(NULLIF(occurred_at,'')::timestamptz AT TIME ZONE $3,'YYYY-MM-DD'), coalesce(sum(point_cost),0) FROM xz_billing_events WHERE NULLIF(occurred_at,'')::timestamptz >= $1 AND NULLIF(occurred_at,'')::timestamptz < $2 AND point_cost > 0 GROUP BY 1 ORDER BY 1`, r.start, r.end, r.loc.String()); err != nil {
		return out, err
	}
	if out.Tokens, err = s.analyticsDaily(ctx, `SELECT to_char(NULLIF(created_at,'')::timestamptz AT TIME ZONE $3,'YYYY-MM-DD'), coalesce(sum(abs(amount)),0) FROM xz_token_records WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND upper(change_type) IN ('USE','USAGE','CONSUME','CONSUMPTION') GROUP BY 1 ORDER BY 1`, r.start, r.end, r.loc.String()); err != nil {
		return out, err
	}
	return out, nil
}

func (s *postgresStore) analyticsDaily(ctx context.Context, query string, start, end time.Time, timezone string) ([]DailyMetric, error) {
	rows, err := s.db.QueryContext(ctx, query, start, end, timezone)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []DailyMetric{}
	for rows.Next() {
		var item DailyMetric
		if err := rows.Scan(&item.Date, &item.Value); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) AnalyticsUsers(ctx context.Context, params AnalyticsQueryParams) (AnalyticsUsersResponse, error) {
	r := s.analyticsPostgresRange(params)
	var err error
	var out AnalyticsUsersResponse
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM xz_users WHERE role <> 'SUPER_ADMIN'`).Scan(&out.TotalUsers); err != nil {
		return out, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM xz_users WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND role <> 'SUPER_ADMIN'`, r.end.AddDate(0, 0, -1), r.end).Scan(&out.NewUsersToday); err != nil {
		return out, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT count(DISTINCT user_id) FROM xz_generation_tasks WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND upper(status)='SUCCEEDED'`, r.end.AddDate(0, 0, -1), r.end).Scan(&out.DAU); err != nil {
		return out, err
	}
	if out.NewUsersTrend, err = s.analyticsDaily(ctx, `SELECT to_char(NULLIF(created_at,'')::timestamptz AT TIME ZONE $3,'YYYY-MM-DD'), count(*) FROM xz_users WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND role <> 'SUPER_ADMIN' GROUP BY 1 ORDER BY 1`, r.start, r.end, r.loc.String()); err != nil {
		return out, err
	}
	out.DAUTrend = out.NewUsersTrend
	out.WAUTrend = out.NewUsersTrend
	out.MAUTrend = out.NewUsersTrend
	out.AIUsersTrend = out.NewUsersTrend
	return out, nil
}
func (s *postgresStore) AnalyticsGeneration(ctx context.Context, params AnalyticsQueryParams) (AnalyticsGenerationResponse, error) {
	r := s.analyticsPostgresRange(params)
	var out AnalyticsGenerationResponse
	dayStart := r.end.AddDate(0, 0, -1)
	queries := []struct {
		query string
		dest  any
	}{
		{`SELECT count(*) FROM xz_generation_tasks WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND type='TEXT_TO_IMAGE' AND upper(status)='SUCCEEDED'`, &out.ImagesToday},
		{`SELECT count(*) FROM xz_generation_tasks WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND type IN ('TEXT_TO_VIDEO','IMAGE_TO_VIDEO') AND upper(status)='SUCCEEDED'`, &out.VideosToday},
		{`SELECT count(*) FROM xz_generation_tasks WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2`, &out.TotalTasksToday},
		{`SELECT count(*) FROM xz_generation_tasks WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND upper(status)='FAILED'`, &out.FailedTasks},
	}
	for _, item := range queries {
		if err := s.db.QueryRowContext(ctx, item.query, dayStart, r.end).Scan(item.dest); err != nil {
			return out, err
		}
	}
	var succeeded int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FILTER (WHERE upper(status)='SUCCEEDED') FROM xz_generation_tasks WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2`, dayStart, r.end).Scan(&succeeded); err != nil {
		return out, err
	}
	if out.TotalTasksToday > 0 {
		out.SuccessRate = float64(succeeded) / float64(out.TotalTasksToday) * 100
	}
	models, err := s.AnalyticsModels(ctx, params)
	if err != nil {
		return out, err
	}
	providers, err := s.AnalyticsProviders(ctx, params)
	if err != nil {
		return out, err
	}
	out.ByModel = models.Models
	out.ByProvider = providers.Providers
	return out, nil
}
func (s *postgresStore) AnalyticsTokens(ctx context.Context, params AnalyticsQueryParams) (AnalyticsTokensResponse, error) {
	r := s.analyticsPostgresRange(params)
	var err error
	var out AnalyticsTokensResponse
	if err := s.db.QueryRowContext(ctx, `SELECT coalesce(sum(abs(amount)),0) FROM xz_token_records WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND upper(change_type) IN ('USE','USAGE','CONSUME','CONSUMPTION')`, r.end.AddDate(0, 0, -1), r.end).Scan(&out.TokensToday); err != nil {
		return out, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT coalesce(sum(abs(amount)),0) FROM xz_token_records WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND upper(change_type) IN ('USE','USAGE','CONSUME','CONSUMPTION')`, r.end.AddDate(0, 0, -7), r.end).Scan(&out.Tokens7d); err != nil {
		return out, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT coalesce(sum(abs(amount)),0) FROM xz_token_records WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND upper(change_type) IN ('USE','USAGE','CONSUME','CONSUMPTION')`, r.end.AddDate(0, 0, -30), r.end).Scan(&out.Tokens30d); err != nil {
		return out, err
	}
	out.TokensTrend, err = s.analyticsDaily(ctx, `SELECT to_char(NULLIF(created_at,'')::timestamptz AT TIME ZONE $3,'YYYY-MM-DD'), coalesce(sum(abs(amount)),0) FROM xz_token_records WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND upper(change_type) IN ('USE','USAGE','CONSUME','CONSUMPTION') GROUP BY 1 ORDER BY 1`, r.start, r.end, r.loc.String())
	return out, err
}
func (s *postgresStore) AnalyticsPoints(ctx context.Context, params AnalyticsQueryParams) (AnalyticsPointsResponse, error) {
	r := s.analyticsPostgresRange(params)
	var err error
	var out AnalyticsPointsResponse
	dayStart := r.end.AddDate(0, 0, -1)
	queries := []struct {
		query string
		dest  any
	}{
		{`SELECT coalesce(sum(point_cost),0) FROM xz_billing_events WHERE NULLIF(occurred_at,'')::timestamptz >= $1 AND NULLIF(occurred_at,'')::timestamptz < $2 AND point_cost > 0`, &out.ConsumedToday},
		{`SELECT coalesce(sum(amount_cents),0) FROM xz_billing_events WHERE NULLIF(occurred_at,'')::timestamptz >= $1 AND NULLIF(occurred_at,'')::timestamptz < $2 AND amount_cents > 0`, &out.RechargedToday},
		{`SELECT coalesce(sum(point_cost),0) FROM xz_billing_events WHERE NULLIF(occurred_at,'')::timestamptz >= $1 AND NULLIF(occurred_at,'')::timestamptz < $2 AND point_cost > 0`, &out.NetChangeToday},
	}
	for _, item := range queries {
		if err := s.db.QueryRowContext(ctx, item.query, dayStart, r.end).Scan(item.dest); err != nil {
			return out, err
		}
	}
	if err := s.db.QueryRowContext(ctx, `SELECT coalesce(sum(token_balance),0), coalesce(sum(frozen_token),0) FROM xz_user_wallets`).Scan(&out.TotalAvailable, &out.TotalFrozen); err != nil {
		return out, err
	}
	out.ConsumedTrend, err = s.analyticsDaily(ctx, `SELECT to_char(NULLIF(occurred_at,'')::timestamptz AT TIME ZONE $3,'YYYY-MM-DD'), coalesce(sum(point_cost),0) FROM xz_billing_events WHERE NULLIF(occurred_at,'')::timestamptz >= $1 AND NULLIF(occurred_at,'')::timestamptz < $2 AND point_cost > 0 GROUP BY 1 ORDER BY 1`, r.start, r.end, r.loc.String())
	if err != nil {
		return out, err
	}
	out.RechargedTrend, err = s.analyticsDaily(ctx, `SELECT to_char(NULLIF(occurred_at,'')::timestamptz AT TIME ZONE $3,'YYYY-MM-DD'), coalesce(sum(amount_cents),0) FROM xz_billing_events WHERE NULLIF(occurred_at,'')::timestamptz >= $1 AND NULLIF(occurred_at,'')::timestamptz < $2 AND amount_cents > 0 GROUP BY 1 ORDER BY 1`, r.start, r.end, r.loc.String())
	return out, err
}
