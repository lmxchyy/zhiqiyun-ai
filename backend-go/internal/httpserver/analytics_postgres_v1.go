package httpserver

import (
	"context"
	"fmt"
	"strings"
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

// buildScopedUserFilter produces a WHERE clause matching users belonging to the scope.
// When scope is PLATFORM or uninitialized (legacy/store tests), matches all users except SUPER_ADMIN.
// When scope is restricted, matches users who belong to the scoped tenants/agents.
func buildScopedUserFilter(scope AnalyticsScope, currentArgIndex int) (string, []any, int) {
	if scope.IsPlatform || scope.Level == "" {
		return "role <> 'SUPER_ADMIN'", nil, currentArgIndex
	}
	if scope.IsFailClosed() {
		return "1=0", nil, currentArgIndex
	}
	next := currentArgIndex
	var clauses []string
	var args []any
	if len(scope.TenantIDs) > 0 {
		clauses = append(clauses, fmt.Sprintf("id IN (SELECT user_id FROM xz_tenant_members WHERE tenant_id = ANY($%d))", next))
		args = append(args, scope.TenantIDs)
		next++
	}
	if len(scope.AgentIDs) > 0 {
		clauses = append(clauses, fmt.Sprintf("id IN (SELECT user_id FROM xz_channel_agents WHERE id = ANY($%d) UNION SELECT user_id FROM xz_user_relationships WHERE parent_agent_id = ANY($%d) AND status='ACTIVE')", next, next))
		args = append(args, scope.AgentIDs)
		next++
	}
	if len(scope.OperationCenterIDs) > 0 {
		clauses = append(clauses, fmt.Sprintf("id IN (SELECT user_id FROM xz_operation_centers WHERE id = ANY($%d) UNION SELECT user_id FROM xz_user_relationships WHERE operation_center_id = ANY($%d) AND status='ACTIVE')", next, next))
		args = append(args, scope.OperationCenterIDs)
		next++
	}
	if len(clauses) == 0 {
		return "1=0", nil, next
	}
	return "(" + strings.Join(clauses, " OR ") + ")", args, next
}

func (s *postgresStore) AnalyticsOverview(ctx context.Context, params AnalyticsQueryParams) (AnalyticsOverviewResponse, error) {
	rangeInfo := s.analyticsPostgresRange(params)
	var out AnalyticsOverviewResponse
	dayStart, dayEnd := rangeInfo.end.AddDate(0, 0, -1), rangeInfo.end

	scope := params.Scope
	if scope.IsFailClosed() {
		return out, nil
	}

	// 1. Users count (scoped)
	userClause, userArgs, _ := buildScopedUserFilter(scope, 3)
	userQuery := fmt.Sprintf("SELECT count(*) FROM xz_users WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND %s", userClause)
	userQueryParams := append([]any{dayStart, dayEnd}, userArgs...)
	if err := s.db.QueryRowContext(ctx, userQuery, userQueryParams...).Scan(&out.NewUsersToday); err != nil {
		return out, err
	}

	// 2. Active users (DAU / WAU / MAU) using scoped unions
	buildActiveSubquery := func(start, end time.Time) (int, error) {
		taskClause, taskArgs, idx := scope.ScopeSQLFilter("xz_generation_tasks", 3)
		billingClause, billingArgs, _ := scope.ScopeSQLFilter("xz_billing_events", idx)

		allArgs := []any{start, end}
		allArgs = append(allArgs, taskArgs...)
		allArgs = append(allArgs, billingArgs...)

		activeQuery := fmt.Sprintf(`
			SELECT count(DISTINCT user_id) FROM (
				SELECT user_id FROM xz_generation_tasks 
				WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 
				  AND upper(status)='SUCCEEDED' AND %s
				UNION
				SELECT user_id FROM xz_billing_events 
				WHERE NULLIF(occurred_at,'')::timestamptz >= $1 AND NULLIF(occurred_at,'')::timestamptz < $2 AND %s
			) active
		`, taskClause, billingClause)

		var count int
		err := s.db.QueryRowContext(ctx, activeQuery, allArgs...).Scan(&count)
		return count, err
	}

	var err error
	if out.DAU, err = buildActiveSubquery(dayStart, dayEnd); err != nil {
		return out, err
	}
	if out.WAU, err = buildActiveSubquery(dayEnd.AddDate(0, 0, -7), dayEnd); err != nil {
		return out, err
	}
	if out.MAU, err = buildActiveSubquery(dayEnd.AddDate(0, 0, -30), dayEnd); err != nil {
		return out, err
	}

	// 3. Generation task counts (scoped)
	genClause, genArgs, _ := scope.ScopeSQLFilter("xz_generation_tasks", 3)
	imgQuery := fmt.Sprintf("SELECT count(*) FROM xz_generation_tasks WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND type='TEXT_TO_IMAGE' AND upper(status)='SUCCEEDED' AND %s", genClause)
	imgArgs := append([]any{dayStart, dayEnd}, genArgs...)
	if err := s.db.QueryRowContext(ctx, imgQuery, imgArgs...).Scan(&out.ImagesGenerated); err != nil {
		return out, err
	}

	vidQuery := fmt.Sprintf("SELECT count(*) FROM xz_generation_tasks WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND type IN ('TEXT_TO_VIDEO','IMAGE_TO_VIDEO') AND upper(status)='SUCCEEDED' AND %s", genClause)
	vidArgs := append([]any{dayStart, dayEnd}, genArgs...)
	if err := s.db.QueryRowContext(ctx, vidQuery, vidArgs...).Scan(&out.VideosGenerated); err != nil {
		return out, err
	}

	failedQuery := fmt.Sprintf("SELECT count(*) FROM xz_generation_tasks WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND upper(status)='FAILED' AND %s", genClause)
	failedArgs := append([]any{dayStart, dayEnd}, genArgs...)
	if err := s.db.QueryRowContext(ctx, failedQuery, failedArgs...).Scan(&out.FailedTasksToday); err != nil {
		return out, err
	}

	procClause, procArgs, _ := scope.ScopeSQLFilter("xz_generation_tasks", 1)
	procQuery := fmt.Sprintf("SELECT count(*) FROM xz_generation_tasks WHERE upper(status) IN ('PROCESSING','RUNNING','PENDING','QUEUED') AND %s", procClause)
	if err := s.db.QueryRowContext(ctx, procQuery, procArgs...).Scan(&out.ProcessingTasks); err != nil {
		// Non-fatal if query fails, default 0
		out.ProcessingTasks = 0
	}

	if scope.IsPlatform || scope.Level == "" {
		_ = s.db.QueryRowContext(ctx, "SELECT count(*) FROM xz_admin_exception_cases WHERE status IN ('OPEN','IN_PROGRESS')").Scan(&out.ExceptionCount)
	}

	var total, succeeded int
	rateQuery := fmt.Sprintf("SELECT count(*), count(*) FILTER (WHERE upper(status)='SUCCEEDED') FROM xz_generation_tasks WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND %s", genClause)
	rateArgs := append([]any{dayStart, dayEnd}, genArgs...)
	if err := s.db.QueryRowContext(ctx, rateQuery, rateArgs...).Scan(&total, &succeeded); err != nil {
		return out, err
	}
	if total > 0 {
		out.SuccessRate = float64(succeeded) / float64(total) * 100
	}

	// 4. Financial metrics: Points, Revenue, Cost (scoped)
	billClause, billArgs, _ := scope.ScopeSQLFilter("xz_billing_events", 3)
	ptsQuery := fmt.Sprintf("SELECT coalesce(sum(point_cost),0) FROM xz_billing_events WHERE NULLIF(occurred_at,'')::timestamptz >= $1 AND NULLIF(occurred_at,'')::timestamptz < $2 AND point_cost > 0 AND %s", billClause)
	ptsArgs := append([]any{dayStart, dayEnd}, billArgs...)
	if err := s.db.QueryRowContext(ctx, ptsQuery, ptsArgs...).Scan(&out.PointsConsumed); err != nil {
		return out, err
	}

	revQuery := fmt.Sprintf("SELECT coalesce(sum(amount_cents),0) FROM xz_billing_events WHERE NULLIF(occurred_at,'')::timestamptz >= $1 AND NULLIF(occurred_at,'')::timestamptz < $2 AND amount_cents > 0 AND upper(metric_code) IN ('RECHARGE','PURCHASE','RECHARGE.POINTS') AND %s", billClause)
	revArgs := append([]any{dayStart, dayEnd}, billArgs...)
	if err := s.db.QueryRowContext(ctx, revQuery, revArgs...).Scan(&out.RevenueTodayCents); err != nil {
		return out, err
	}

	// Token usage: only platform or uninitialized scopes see platform-wide token consumption
	if scope.IsPlatform || scope.Level == "" {
		if err := s.db.QueryRowContext(ctx, `SELECT coalesce(sum(abs(amount)),0) FROM xz_token_records WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND upper(change_type) IN ('USE','USAGE','CONSUME','CONSUMPTION')`, dayStart, dayEnd).Scan(&out.TokensUsed); err != nil {
			return out, err
		}
	} else {
		out.TokensUsed = 0
	}

	// Supplier cost is confidential: only PLATFORM or uninitialized scope sees cost
	if scope.IsPlatform || scope.Level == "" {
		costQuery := fmt.Sprintf("SELECT coalesce(sum(supplier_cost),0)::bigint FROM xz_generation_tasks WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND upper(status)='SUCCEEDED' AND %s", genClause)
		costArgs := append([]any{dayStart, dayEnd}, genArgs...)
		if err := s.db.QueryRowContext(ctx, costQuery, costArgs...).Scan(&out.CostTodayCents); err != nil {
			return out, err
		}
	} else {
		out.CostTodayCents = 0
	}

	// 5. Model call latency
	modelClause, modelArgs, _ := scope.ScopeSQLFilter("model_call_logs", 3)
	latQuery := fmt.Sprintf("SELECT coalesce(avg(latency_ms),0)::bigint FROM model_call_logs WHERE created_at >= $1 AND created_at < $2 AND %s", modelClause)
	latArgs := append([]any{dayStart, dayEnd}, modelArgs...)
	if err := s.db.QueryRowContext(ctx, latQuery, latArgs...).Scan(&out.AvgLatencyMs); err != nil {
		return out, err
	}

	return out, nil
}

func (s *postgresStore) AnalyticsModels(ctx context.Context, params AnalyticsQueryParams) (AnalyticsModelsResponse, error) {
	r := s.analyticsPostgresRange(params)
	scope := params.Scope
	if scope.IsFailClosed() {
		return AnalyticsModelsResponse{Models: []ModelMetric{}}, nil
	}

	modelClause, modelArgs, _ := scope.ScopeSQLFilter("model_call_logs", 3)
	query := fmt.Sprintf(`
		SELECT model_code, count(*), count(*) FILTER (WHERE %s), 
		       coalesce(avg(latency_ms),0)::bigint, coalesce(sum(cost_cents),0)::bigint 
		FROM model_call_logs 
		WHERE created_at >= $1 AND created_at < $2 AND %s
		GROUP BY model_code ORDER BY count(*) DESC, model_code
	`, analyticsStatusSQL(), modelClause)

	args := append([]any{r.start, r.end}, modelArgs...)
	rows, err := s.db.QueryContext(ctx, query, args...)
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
		// Redact raw upstream cost for non-platform scopes
		if !scope.IsPlatform && scope.Level != "" {
			item.TotalCostCents = 0
		}
		result.Models = append(result.Models, item)
	}
	return result, rows.Err()
}

func (s *postgresStore) AnalyticsProviders(ctx context.Context, params AnalyticsQueryParams) (AnalyticsProvidersResponse, error) {
	r := s.analyticsPostgresRange(params)
	scope := params.Scope
	// Providers list contains confidential supplier topology: only PLATFORM or uninitialized scope allowed
	if (!scope.IsPlatform && scope.Level != "") || scope.IsFailClosed() {
		return AnalyticsProvidersResponse{Providers: []ProviderMetric{}}, nil
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT provider_code, count(*), count(*) FILTER (WHERE `+analyticsStatusSQL()+`), 
		       coalesce(avg(latency_ms),0)::bigint, coalesce(sum(cost_cents),0)::bigint 
		FROM model_call_logs 
		WHERE created_at >= $1 AND created_at < $2 
		GROUP BY provider_code ORDER BY count(*) DESC, provider_code
	`, r.start, r.end)
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
	scope := params.Scope
	if scope.IsFailClosed() {
		return out, nil
	}

	var err error
	// 1. New users trend (scoped)
	userClause, userArgs, _ := buildScopedUserFilter(scope, 4)
	newUserQuery := fmt.Sprintf(`
		SELECT to_char(NULLIF(created_at,'')::timestamptz AT TIME ZONE $3,'YYYY-MM-DD'), count(*) 
		FROM xz_users 
		WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND %s 
		GROUP BY 1 ORDER BY 1
	`, userClause)
	newUserParams := append([]any{r.start, r.end, r.loc.String()}, userArgs...)
	if out.NewUsers, err = s.analyticsDaily(ctx, newUserQuery, newUserParams...); err != nil {
		return out, err
	}

	// 2. DAU trend (scoped)
	taskClause, taskArgs, idx := scope.ScopeSQLFilter("xz_generation_tasks", 4)
	billClause, billArgs, _ := scope.ScopeSQLFilter("xz_billing_events", idx)
	dauQuery := fmt.Sprintf(`
		SELECT to_char(ts AT TIME ZONE $3,'YYYY-MM-DD'), count(DISTINCT user_id) FROM (
			SELECT user_id, NULLIF(created_at,'')::timestamptz AS ts FROM xz_generation_tasks 
			WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND %s
			UNION ALL 
			SELECT user_id, NULLIF(occurred_at,'')::timestamptz AS ts FROM xz_billing_events 
			WHERE NULLIF(occurred_at,'')::timestamptz >= $1 AND NULLIF(occurred_at,'')::timestamptz < $2 AND %s
		) active GROUP BY 1 ORDER BY 1
	`, taskClause, billClause)
	dauParams := append([]any{r.start, r.end, r.loc.String()}, taskArgs...)
	dauParams = append(dauParams, billArgs...)
	if out.DAU, err = s.analyticsDaily(ctx, dauQuery, dauParams...); err != nil {
		return out, err
	}

	// 3. AI Users trend (scoped)
	aiQuery := fmt.Sprintf(`
		SELECT to_char(NULLIF(created_at,'')::timestamptz AT TIME ZONE $3,'YYYY-MM-DD'), count(DISTINCT user_id) 
		FROM xz_generation_tasks 
		WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND upper(status)='SUCCEEDED' AND %s 
		GROUP BY 1 ORDER BY 1
	`, taskClause)
	aiParams := append([]any{r.start, r.end, r.loc.String()}, taskArgs...)
	if out.AIUsers, err = s.analyticsDaily(ctx, aiQuery, aiParams...); err != nil {
		return out, err
	}

	// 4. Images trend (scoped)
	imgQuery := fmt.Sprintf(`
		SELECT to_char(NULLIF(created_at,'')::timestamptz AT TIME ZONE $3,'YYYY-MM-DD'), count(*) 
		FROM xz_generation_tasks 
		WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND type='TEXT_TO_IMAGE' AND upper(status)='SUCCEEDED' AND %s 
		GROUP BY 1 ORDER BY 1
	`, taskClause)
	imgParams := append([]any{r.start, r.end, r.loc.String()}, taskArgs...)
	if out.Images, err = s.analyticsDaily(ctx, imgQuery, imgParams...); err != nil {
		return out, err
	}

	// 5. Videos trend (scoped)
	vidQuery := fmt.Sprintf(`
		SELECT to_char(NULLIF(created_at,'')::timestamptz AT TIME ZONE $3,'YYYY-MM-DD'), count(*) 
		FROM xz_generation_tasks 
		WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND type IN ('TEXT_TO_VIDEO','IMAGE_TO_VIDEO') AND upper(status)='SUCCEEDED' AND %s 
		GROUP BY 1 ORDER BY 1
	`, taskClause)
	vidParams := append([]any{r.start, r.end, r.loc.String()}, taskArgs...)
	if out.Videos, err = s.analyticsDaily(ctx, vidQuery, vidParams...); err != nil {
		return out, err
	}

	// 6. Points trend (scoped)
	ptsQuery := fmt.Sprintf(`
		SELECT to_char(NULLIF(occurred_at,'')::timestamptz AT TIME ZONE $3,'YYYY-MM-DD'), coalesce(sum(point_cost),0) 
		FROM xz_billing_events 
		WHERE NULLIF(occurred_at,'')::timestamptz >= $1 AND NULLIF(occurred_at,'')::timestamptz < $2 AND point_cost > 0 AND %s 
		GROUP BY 1 ORDER BY 1
	`, billClause)
	ptsParams := append([]any{r.start, r.end, r.loc.String()}, billArgs...)
	if out.Points, err = s.analyticsDaily(ctx, ptsQuery, ptsParams...); err != nil {
		return out, err
	}

	return out, nil
}

func (s *postgresStore) analyticsDaily(ctx context.Context, query string, args ...any) ([]DailyMetric, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
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
	scope := params.Scope
	if scope.IsFailClosed() {
		return AnalyticsUsersResponse{}, nil
	}

	var err error
	var out AnalyticsUsersResponse

	userClause, userArgs, _ := buildScopedUserFilter(scope, 1)
	totQuery := fmt.Sprintf("SELECT count(*) FROM xz_users WHERE %s", userClause)
	if err := s.db.QueryRowContext(ctx, totQuery, userArgs...).Scan(&out.TotalUsers); err != nil {
		return out, err
	}

	userClauseDay, userArgsDay, _ := buildScopedUserFilter(scope, 3)
	newQuery := fmt.Sprintf("SELECT count(*) FROM xz_users WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND %s", userClauseDay)
	newArgs := append([]any{r.end.AddDate(0, 0, -1), r.end}, userArgsDay...)
	if err := s.db.QueryRowContext(ctx, newQuery, newArgs...).Scan(&out.NewUsersToday); err != nil {
		return out, err
	}

	taskClause, taskArgs, _ := scope.ScopeSQLFilter("xz_generation_tasks", 3)
	dauQuery := fmt.Sprintf("SELECT count(DISTINCT user_id) FROM xz_generation_tasks WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND upper(status)='SUCCEEDED' AND %s", taskClause)
	dauArgs := append([]any{r.end.AddDate(0, 0, -1), r.end}, taskArgs...)
	if err := s.db.QueryRowContext(ctx, dauQuery, dauArgs...).Scan(&out.DAU); err != nil {
		return out, err
	}

	trendQuery := fmt.Sprintf(`
		SELECT to_char(NULLIF(created_at,'')::timestamptz AT TIME ZONE $3,'YYYY-MM-DD'), count(*) 
		FROM xz_users 
		WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND %s 
		GROUP BY 1 ORDER BY 1
	`, userClauseDay)
	trendArgs := append([]any{r.start, r.end, r.loc.String()}, userArgsDay...)
	if out.NewUsersTrend, err = s.analyticsDaily(ctx, trendQuery, trendArgs...); err != nil {
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
	scope := params.Scope
	if scope.IsFailClosed() {
		return AnalyticsGenerationResponse{}, nil
	}

	var out AnalyticsGenerationResponse
	dayStart := r.end.AddDate(0, 0, -1)
	genClause, genArgs, _ := scope.ScopeSQLFilter("xz_generation_tasks", 3)
	baseArgs := append([]any{dayStart, r.end}, genArgs...)

	imgQuery := fmt.Sprintf("SELECT count(*) FROM xz_generation_tasks WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND type='TEXT_TO_IMAGE' AND upper(status)='SUCCEEDED' AND %s", genClause)
	if err := s.db.QueryRowContext(ctx, imgQuery, baseArgs...).Scan(&out.ImagesToday); err != nil {
		return out, err
	}

	vidQuery := fmt.Sprintf("SELECT count(*) FROM xz_generation_tasks WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND type IN ('TEXT_TO_VIDEO','IMAGE_TO_VIDEO') AND upper(status)='SUCCEEDED' AND %s", genClause)
	if err := s.db.QueryRowContext(ctx, vidQuery, baseArgs...).Scan(&out.VideosToday); err != nil {
		return out, err
	}

	totQuery := fmt.Sprintf("SELECT count(*) FROM xz_generation_tasks WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND %s", genClause)
	if err := s.db.QueryRowContext(ctx, totQuery, baseArgs...).Scan(&out.TotalTasksToday); err != nil {
		return out, err
	}

	failQuery := fmt.Sprintf("SELECT count(*) FROM xz_generation_tasks WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND upper(status)='FAILED' AND %s", genClause)
	if err := s.db.QueryRowContext(ctx, failQuery, baseArgs...).Scan(&out.FailedTasks); err != nil {
		return out, err
	}

	var succeeded int
	succQuery := fmt.Sprintf("SELECT count(*) FILTER (WHERE upper(status)='SUCCEEDED') FROM xz_generation_tasks WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND %s", genClause)
	if err := s.db.QueryRowContext(ctx, succQuery, baseArgs...).Scan(&succeeded); err != nil {
		return out, err
	}
	if out.TotalTasksToday > 0 {
		out.SuccessRate = float64(succeeded) / float64(out.TotalTasksToday) * 100
	}

	// Breakdown by application type
	typeQuery := fmt.Sprintf(`
		SELECT coalesce(nullif(type,''), 'OTHER'), count(*) 
		FROM xz_generation_tasks 
		WHERE NULLIF(created_at,'')::timestamptz >= $1 AND NULLIF(created_at,'')::timestamptz < $2 AND %s
		GROUP BY 1 ORDER BY 2 DESC
	`, genClause)
	if rows, err := s.db.QueryContext(ctx, typeQuery, baseArgs...); err == nil {
		defer rows.Close()
		for rows.Next() {
			var tm TypeMetric
			if err := rows.Scan(&tm.Type, &tm.Count); err == nil {
				if out.TotalTasksToday > 0 {
					tm.Rate = float64(tm.Count) / float64(out.TotalTasksToday) * 100
				}
				out.ByType = append(out.ByType, tm)
			}
		}
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
	scope := params.Scope
	if scope.IsFailClosed() {
		return AnalyticsTokensResponse{}, nil
	}

	var out AnalyticsTokensResponse
	// Token records are strictly tied to platform administration; non-platform scopes report 0
	if !scope.IsPlatform && scope.Level != "" {
		return out, nil
	}

	var err error
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
	scope := params.Scope
	if scope.IsFailClosed() {
		return AnalyticsPointsResponse{}, nil
	}

	var err error
	var out AnalyticsPointsResponse
	dayStart := r.end.AddDate(0, 0, -1)

	billClause, billArgs, _ := scope.ScopeSQLFilter("xz_billing_events", 3)
	billParams := append([]any{dayStart, r.end}, billArgs...)

	cQuery := fmt.Sprintf("SELECT coalesce(sum(point_cost),0) FROM xz_billing_events WHERE NULLIF(occurred_at,'')::timestamptz >= $1 AND NULLIF(occurred_at,'')::timestamptz < $2 AND point_cost > 0 AND %s", billClause)
	if err := s.db.QueryRowContext(ctx, cQuery, billParams...).Scan(&out.ConsumedToday); err != nil {
		return out, err
	}

	rQuery := fmt.Sprintf("SELECT coalesce(sum(amount_cents),0) FROM xz_billing_events WHERE NULLIF(occurred_at,'')::timestamptz >= $1 AND NULLIF(occurred_at,'')::timestamptz < $2 AND amount_cents > 0 AND %s", billClause)
	if err := s.db.QueryRowContext(ctx, rQuery, billParams...).Scan(&out.RechargedToday); err != nil {
		return out, err
	}

	out.NetChangeToday = out.RechargedToday - out.ConsumedToday

	// Only platform admin sees total balances across the entire platform
	if scope.IsPlatform || scope.Level == "" {
		if err := s.db.QueryRowContext(ctx, `SELECT coalesce(sum(token_balance),0), coalesce(sum(frozen_token),0) FROM xz_user_wallets`).Scan(&out.TotalAvailable, &out.TotalFrozen); err != nil {
			return out, err
		}
	} else {
		out.TotalAvailable = 0
		out.TotalFrozen = 0
	}

	trendQueryC := fmt.Sprintf(`
		SELECT to_char(NULLIF(occurred_at,'')::timestamptz AT TIME ZONE $3,'YYYY-MM-DD'), coalesce(sum(point_cost),0) 
		FROM xz_billing_events 
		WHERE NULLIF(occurred_at,'')::timestamptz >= $1 AND NULLIF(occurred_at,'')::timestamptz < $2 AND point_cost > 0 AND %s 
		GROUP BY 1 ORDER BY 1
	`, billClause)
	trendArgsC := append([]any{r.start, r.end, r.loc.String()}, billArgs...)
	if out.ConsumedTrend, err = s.analyticsDaily(ctx, trendQueryC, trendArgsC...); err != nil {
		return out, err
	}

	trendQueryR := fmt.Sprintf(`
		SELECT to_char(NULLIF(occurred_at,'')::timestamptz AT TIME ZONE $3,'YYYY-MM-DD'), coalesce(sum(amount_cents),0) 
		FROM xz_billing_events 
		WHERE NULLIF(occurred_at,'')::timestamptz >= $1 AND NULLIF(occurred_at,'')::timestamptz < $2 AND amount_cents > 0 AND %s 
		GROUP BY 1 ORDER BY 1
	`, billClause)
	trendArgsR := append([]any{r.start, r.end, r.loc.String()}, billArgs...)
	if out.RechargedTrend, err = s.analyticsDaily(ctx, trendQueryR, trendArgsR...); err != nil {
		return out, err
	}

	return out, nil
}
