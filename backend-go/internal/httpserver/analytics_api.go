package httpserver

import (
	"net/http"
	"time"
)

// analyticsAPI handles analytics-related HTTP endpoints
type analyticsAPI struct {
	store platformStore
}

// newAnalyticsAPI creates a new analyticsAPI instance
func newAnalyticsAPI(store platformStore) analyticsAPI {
	return analyticsAPI{store: store}
}

// AnalyticsOverview handles GET /api/admin/analytics/overview
func (a analyticsAPI) AnalyticsOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse query parameters
	params := AnalyticsQueryParams{
		Days:     7, // 默认7天
		Timezone: "Asia/Shanghai",
	}

	// Parse Days parameter
	if daysStr := r.URL.Query().Get("days"); daysStr != "" {
		if days, err := time.ParseDuration(daysStr + "h"); err == nil {
			params.Days = int(days.Hours() / 24)
			if params.Days <= 0 {
				params.Days = 1
			}
			if params.Days > 90 {
				params.Days = 90
			}
		}
	}

	// Parse Timezone parameter
	if tz := r.URL.Query().Get("timezone"); tz != "" {
		params.Timezone = tz
	}

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
		if days, err := time.ParseDuration(daysStr + "h"); err == nil {
			params.Days = int(days.Hours() / 24)
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
		if days, err := time.ParseDuration(daysStr + "h"); err == nil {
			params.Days = int(days.Hours() / 24)
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
		if days, err := time.ParseDuration(daysStr + "h"); err == nil {
			params.Days = int(days.Hours() / 24)
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
		if days, err := time.ParseDuration(daysStr + "h"); err == nil {
			params.Days = int(days.Hours() / 24)
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
		if days, err := time.ParseDuration(daysStr + "h"); err == nil {
			params.Days = int(days.Hours() / 24)
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
		if days, err := time.ParseDuration(daysStr + "h"); err == nil {
			params.Days = int(days.Hours() / 24)
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
		if days, err := time.ParseDuration(daysStr + "h"); err == nil {
			params.Days = int(days.Hours() / 24)
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
