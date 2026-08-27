package httpserver

import (
	"net/http"
	"net/url"
	"strconv"
)

// analyticsAPI handles analytics-related HTTP endpoints
type analyticsAPI struct {
	store platformStore
}

func parseAnalyticsQuery(values url.Values) AnalyticsQueryParams {
	params := AnalyticsQueryParams{Days: 7, Timezone: "Asia/Shanghai"}
	if raw := values.Get("days"); raw != "" {
		if _, err := strconv.Atoi(raw); err == nil {
			params.Days = parseAnalyticsDays(raw)
		}
	}
	if tz := values.Get("timezone"); tz != "" {
		params.Timezone = tz
	}
	return params
}

func parseAnalyticsDays(raw string) int {
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed < 1 {
		return 1
	}
	if parsed > 90 {
		return 90
	}
	return parsed
}

// newAnalyticsAPI creates a new analyticsAPI instance
func newAnalyticsAPI(store platformStore) analyticsAPI {
	return analyticsAPI{store: store}
}

// AnalyticsOverview handles GET /api/admin/analytics/overview
func (a analyticsAPI) AnalyticsOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse query parameters
	params := parseAnalyticsQuery(r.URL.Query())

	data, err := a.store.AnalyticsOverview(ctx, params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, data)
}

// AnalyticsUsers handles GET /api/admin/analytics/users
func (a analyticsAPI) AnalyticsUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse query parameters
	params := AnalyticsQueryParams{
		Days:     7,
		Timezone: "Asia/Shanghai",
	}

	if daysStr := r.URL.Query().Get("days"); daysStr != "" {
		if _, err := strconv.Atoi(daysStr); err == nil {
			params.Days = parseAnalyticsDays(daysStr)
			if params.Days <= 0 {
				params.Days = 1
			}
			if params.Days > 90 {
				params.Days = 90
			}
		}
	}

	if tz := r.URL.Query().Get("timezone"); tz != "" {
		params.Timezone = tz
	}

	data, err := a.store.AnalyticsUsers(ctx, params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, data)
}

// AnalyticsGeneration handles GET /api/admin/analytics/generation
func (a analyticsAPI) AnalyticsGeneration(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse query parameters
	params := AnalyticsQueryParams{
		Days:     7,
		Timezone: "Asia/Shanghai",
	}

	if daysStr := r.URL.Query().Get("days"); daysStr != "" {
		if _, err := strconv.Atoi(daysStr); err == nil {
			params.Days = parseAnalyticsDays(daysStr)
			if params.Days <= 0 {
				params.Days = 1
			}
			if params.Days > 90 {
				params.Days = 90
			}
		}
	}

	if tz := r.URL.Query().Get("timezone"); tz != "" {
		params.Timezone = tz
	}

	data, err := a.store.AnalyticsGeneration(ctx, params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, data)
}

// AnalyticsTokens handles GET /api/admin/analytics/tokens
func (a analyticsAPI) AnalyticsTokens(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse query parameters
	params := AnalyticsQueryParams{
		Days:     7,
		Timezone: "Asia/Shanghai",
	}

	if daysStr := r.URL.Query().Get("days"); daysStr != "" {
		if _, err := strconv.Atoi(daysStr); err == nil {
			params.Days = parseAnalyticsDays(daysStr)
			if params.Days <= 0 {
				params.Days = 1
			}
			if params.Days > 90 {
				params.Days = 90
			}
		}
	}

	if tz := r.URL.Query().Get("timezone"); tz != "" {
		params.Timezone = tz
	}

	data, err := a.store.AnalyticsTokens(ctx, params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, data)
}

// AnalyticsPoints handles GET /api/admin/analytics/points
func (a analyticsAPI) AnalyticsPoints(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse query parameters
	params := AnalyticsQueryParams{
		Days:     7,
		Timezone: "Asia/Shanghai",
	}

	if daysStr := r.URL.Query().Get("days"); daysStr != "" {
		if _, err := strconv.Atoi(daysStr); err == nil {
			params.Days = parseAnalyticsDays(daysStr)
			if params.Days <= 0 {
				params.Days = 1
			}
			if params.Days > 90 {
				params.Days = 90
			}
		}
	}

	if tz := r.URL.Query().Get("timezone"); tz != "" {
		params.Timezone = tz
	}

	data, err := a.store.AnalyticsPoints(ctx, params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, data)
}

// AnalyticsModels handles GET /api/admin/analytics/models
func (a analyticsAPI) AnalyticsModels(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse query parameters
	params := AnalyticsQueryParams{
		Days:     7,
		Timezone: "Asia/Shanghai",
	}

	if daysStr := r.URL.Query().Get("days"); daysStr != "" {
		if _, err := strconv.Atoi(daysStr); err == nil {
			params.Days = parseAnalyticsDays(daysStr)
			if params.Days <= 0 {
				params.Days = 1
			}
			if params.Days > 90 {
				params.Days = 90
			}
		}
	}

	if tz := r.URL.Query().Get("timezone"); tz != "" {
		params.Timezone = tz
	}

	data, err := a.store.AnalyticsModels(ctx, params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, data)
}

// AnalyticsProviders handles GET /api/admin/analytics/providers
func (a analyticsAPI) AnalyticsProviders(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse query parameters
	params := AnalyticsQueryParams{
		Days:     7,
		Timezone: "Asia/Shanghai",
	}

	if daysStr := r.URL.Query().Get("days"); daysStr != "" {
		if _, err := strconv.Atoi(daysStr); err == nil {
			params.Days = parseAnalyticsDays(daysStr)
			if params.Days <= 0 {
				params.Days = 1
			}
			if params.Days > 90 {
				params.Days = 90
			}
		}
	}

	if tz := r.URL.Query().Get("timezone"); tz != "" {
		params.Timezone = tz
	}

	data, err := a.store.AnalyticsProviders(ctx, params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, data)
}

// AnalyticsTrends handles GET /api/admin/analytics/trends
func (a analyticsAPI) AnalyticsTrends(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse query parameters
	params := AnalyticsQueryParams{
		Days:     7,
		Timezone: "Asia/Shanghai",
	}

	if daysStr := r.URL.Query().Get("days"); daysStr != "" {
		if _, err := strconv.Atoi(daysStr); err == nil {
			params.Days = parseAnalyticsDays(daysStr)
			if params.Days <= 0 {
				params.Days = 1
			}
			if params.Days > 90 {
				params.Days = 90
			}
		}
	}

	if tz := r.URL.Query().Get("timezone"); tz != "" {
		params.Timezone = tz
	}

	data, err := a.store.AnalyticsTrends(ctx, params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, data)
}
