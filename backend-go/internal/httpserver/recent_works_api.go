package httpserver

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	maxRecentWorksLimit         = 20
	recentWorksFirstPaintCovers = 4
)

type recentWork struct {
	ID           string `json:"id"`
	TaskID       string `json:"taskId,omitempty"`
	Name         string `json:"name"`
	MediaType    string `json:"mediaType"`
	Status       string `json:"status"`
	ThumbnailURL string `json:"thumbnailUrl,omitempty"`
	Favorite     bool   `json:"favorite"`
	Archived     bool   `json:"archived,omitempty"`
	ProjectID    string `json:"projectId,omitempty"`
	ProjectName  string `json:"projectName,omitempty"`
	FileSize     int64  `json:"fileSize,omitempty"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}

type recentWorksDataStore interface {
	ListRecentWorksForUser(userID string, limit int) ([]recentWork, error)
}

func recentWorksLimit(r *http.Request) int {
	limit, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
	if limit <= 0 || limit > maxRecentWorksLimit {
		return maxRecentWorksLimit
	}
	return limit
}

func recentWorkFromAsset(item asset) recentWork {
	metadata := item.Metadata
	status := firstNonEmptyString(
		stringValue(metadata["status"]),
		stringValue(metadata["taskStatus"]),
		"COMPLETED",
	)
	fileSize := int64Value(metadata["fileSize"])
	if fileSize <= 0 {
		fileSize = int64Value(metadata["fileSizeBytes"])
	}
	if fileSize <= 0 {
		fileSize = int64Value(metadata["sizeBytes"])
	}
	return recentWork{
		ID:           item.ID,
		TaskID:       item.TaskID,
		Name:         item.Name,
		MediaType:    item.MediaType,
		Status:       status,
		ThumbnailURL: compactListInlineMediaURL(item.ThumbnailURL),
		Favorite:     item.Favorite,
		Archived:     boolValue(metadata["archived"]),
		ProjectID:    stringValue(metadata["projectId"]),
		ProjectName:  stringValue(metadata["projectName"]),
		FileSize:     fileSize,
		CreatedAt:    item.CreatedAt,
		UpdatedAt:    item.UpdatedAt,
	}
}

func (a api) recentWorksForUser(userID string, limit int) ([]recentWork, error) {
	if store, ok := a.store.(recentWorksDataStore); ok {
		return store.ListRecentWorksForUser(userID, limit)
	}
	items, err := a.store.ListAssets()
	if err != nil {
		return nil, err
	}
	items = filterAssetsForUser(items, userID)
	sortAssetsForUserList(items)
	if len(items) > limit {
		items = items[:limit]
	}
	result := make([]recentWork, 0, len(items))
	for index, item := range items {
		work := recentWorkFromAsset(item)
		if index >= recentWorksFirstPaintCovers {
			work.ThumbnailURL = ""
		}
		result = append(result, work)
	}
	return result, nil
}

func (a api) listRecentWorks(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	user, err := a.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	limit := recentWorksLimit(r)
	queryStartedAt := time.Now()
	items, err := a.recentWorksForUser(user.ID, limit)
	queryEndedAt := time.Now()
	queryDuration := queryEndedAt.Sub(queryStartedAt)
	w.Header().Set("Server-Timing", "recent-works-db;dur="+strconv.FormatFloat(float64(queryDuration.Microseconds())/1000, 'f', 2, 64))
	w.Header().Set("X-Works-Query-Duration-Ms", strconv.FormatInt(queryDuration.Milliseconds(), 10))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		logRecentWorksPerf(startedAt, time.Now(), queryStartedAt, queryEndedAt, r.URL.RequestURI(), 0, err)
		return
	}
	logRecentWorksPerf(startedAt, time.Now(), queryStartedAt, queryEndedAt, r.URL.RequestURI(), len(items), nil)
	writeJSON(w, map[string]any{
		"items": items,
		"limit": limit,
	})
}

func logRecentWorksPerf(startedAt time.Time, endedAt time.Time, queryStartedAt time.Time, queryEndedAt time.Time, requestURL string, itemCount int, requestErr error) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("WORKS_PERF_LOG")), "true") && strings.TrimSpace(os.Getenv("WORKS_PERF_LOG")) != "1" {
		return
	}
	payload := map[string]any{
		"step":          "recent_works_request",
		"startTime":     startedAt.UTC().Format(time.RFC3339Nano),
		"endTime":       endedAt.UTC().Format(time.RFC3339Nano),
		"durationMs":    float64(endedAt.Sub(startedAt).Microseconds()) / 1000,
		"queryStart":    queryStartedAt.UTC().Format(time.RFC3339Nano),
		"queryEnd":      queryEndedAt.UTC().Format(time.RFC3339Nano),
		"queryMs":       float64(queryEndedAt.Sub(queryStartedAt).Microseconds()) / 1000,
		"serialWait":    true,
		"source":        "GET /api/v1/works/recent",
		"requestUrl":    requestURL,
		"duplicate":     false,
		"itemCount":     itemCount,
		"requestFailed": requestErr != nil,
		"assetRefresh":  false,
		"taskSync":      false,
		"storageScan":   false,
		"upstreamQuery": false,
	}
	if requestErr != nil {
		payload["error"] = requestErr.Error()
	}
	encoded, _ := json.Marshal(payload)
	log.Printf("[works-perf] %s", encoded)
}

func (s *jsonStore) ListRecentWorksForUser(userID string, limit int) ([]recentWork, error) {
	data, err := s.load()
	if err != nil {
		return nil, err
	}
	items := filterAssetsForUser(data.Assets, userID)
	sortAssetsForUserList(items)
	if limit <= 0 || limit > maxRecentWorksLimit {
		limit = maxRecentWorksLimit
	}
	if len(items) > limit {
		items = items[:limit]
	}
	result := make([]recentWork, 0, len(items))
	for index, item := range items {
		work := recentWorkFromAsset(item)
		if index >= recentWorksFirstPaintCovers {
			work.ThumbnailURL = ""
		}
		result = append(result, work)
	}
	return result, nil
}

func (s *postgresStore) ListRecentWorksForUser(userID string, limit int) ([]recentWork, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > maxRecentWorksLimit {
		limit = maxRecentWorksLimit
	}
	contextType, tenantID, _, err := s.currentTenantScopeForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		with recent_ids as materialized (
			select
				id,
				created_at
			from xz_assets
			where user_id = $1
				and (($2 = 'ENTERPRISE' and tenant_id = $3)
					or ($2 <> 'ENTERPRISE' and (tenant_id is null or tenant_id = 'tenant_default')))
				and deleted_at is null
				and created_at is not null
			order by created_at desc, id desc
			limit $4
		)
		select
			a.id,
			coalesce(a.task_id, ''),
			coalesce(a.name, ''),
			coalesce(a.media_type, ''),
			coalesce(a.metadata->>'status', a.metadata->>'taskStatus', 'COMPLETED') as status,
			case
				when row_number() over (order by recent.created_at desc, recent.id desc) <= 4
				then coalesce(a.thumbnail_url, '')
				else ''
			end as thumbnail_url,
			coalesce(a.favorite, false),
			lower(coalesce(a.metadata->>'archived', 'false')) = 'true' as archived,
			coalesce(a.metadata->>'projectId', '') as project_id,
			coalesce(a.metadata->>'projectName', '') as project_name,
			case
				when coalesce(a.metadata->>'fileSize', a.metadata->>'fileSizeBytes', a.metadata->>'sizeBytes', '') ~ '^[0-9]+$'
				then coalesce(a.metadata->>'fileSize', a.metadata->>'fileSizeBytes', a.metadata->>'sizeBytes')::bigint
				else 0
			end as file_size,
			coalesce(a.created_at, ''),
			coalesce(a.updated_at, '')
		from recent_ids recent
		join xz_assets a on a.id = recent.id
		order by recent.created_at desc, recent.id desc
	`, userID, contextType, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRecentWorkRows(rows)
}

func scanRecentWorkRows(rows *sql.Rows) ([]recentWork, error) {
	items := make([]recentWork, 0, maxRecentWorksLimit)
	for rows.Next() {
		var item recentWork
		if err := rows.Scan(
			&item.ID,
			&item.TaskID,
			&item.Name,
			&item.MediaType,
			&item.Status,
			&item.ThumbnailURL,
			&item.Favorite,
			&item.Archived,
			&item.ProjectID,
			&item.ProjectName,
			&item.FileSize,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.ThumbnailURL = compactListInlineMediaURL(item.ThumbnailURL)
		items = append(items, item)
	}
	return items, rows.Err()
}
