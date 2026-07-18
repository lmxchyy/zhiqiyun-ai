package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"
)

type adminExceptionHistory struct {
	Action    string `json:"action"`
	ActorID   string `json:"actorId,omitempty"`
	ActorName string `json:"actorName,omitempty"`
	From      string `json:"from,omitempty"`
	To        string `json:"to,omitempty"`
	Note      string `json:"note,omitempty"`
	At        string `json:"at"`
}

type adminExceptionCase struct {
	ID              string                  `json:"id"`
	ExceptionKey    string                  `json:"exceptionKey"`
	Title           string                  `json:"title"`
	Description     string                  `json:"description"`
	Module          string                  `json:"module"`
	Severity        string                  `json:"severity"`
	Count           int                     `json:"count"`
	Roles           []string                `json:"roles,omitempty"`
	AssigneeID      string                  `json:"assigneeId,omitempty"`
	AssigneeName    string                  `json:"assigneeName,omitempty"`
	Status          string                  `json:"status"`
	SLADueAt        string                  `json:"slaDueAt,omitempty"`
	FirstDetectedAt string                  `json:"firstDetectedAt"`
	UpdatedAt       string                  `json:"updatedAt"`
	ClosedAt        string                  `json:"closedAt,omitempty"`
	CloseReason     string                  `json:"closeReason,omitempty"`
	History         []adminExceptionHistory `json:"history,omitempty"`
}

type adminExceptionMutation struct {
	AssigneeID   string `json:"assigneeId"`
	AssigneeName string `json:"assigneeName"`
	Status       string `json:"status"`
	SLADueAt     string `json:"slaDueAt"`
	Note         string `json:"note"`
	CloseReason  string `json:"closeReason"`
}

type adminExperienceEvent struct {
	ID         string         `json:"id"`
	ActorID    string         `json:"actorId,omitempty"`
	ActorRole  string         `json:"actorRole,omitempty"`
	EventType  string         `json:"eventType"`
	ModuleID   string         `json:"moduleId,omitempty"`
	TargetID   string         `json:"targetId,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	OccurredAt string         `json:"occurredAt"`
}

type adminExperienceStore interface {
	SyncAdminExceptionCases([]adminWorkspaceItem) ([]adminExceptionCase, error)
	UpdateAdminExceptionCase(string, adminExceptionMutation, string) (adminExceptionCase, error)
	RecordAdminExperienceEvent(adminExperienceEvent) error
	ListAdminExperienceEvents(time.Time) ([]adminExperienceEvent, error)
}

func exceptionSLADue(now time.Time, severity string) string {
	duration := 24 * time.Hour
	if strings.EqualFold(severity, "danger") {
		duration = 4 * time.Hour
	}
	return now.Add(duration).UTC().Format(time.RFC3339)
}

func (s *jsonStore) SyncAdminExceptionCases(items []adminWorkspaceItem) ([]adminExceptionCase, error) {
	var result []adminExceptionCase
	err := s.updateAdmin(func(data *adminPlatformData) error {
		now := time.Now().UTC()
		byKey := map[string]*adminExceptionCase{}
		for index := range data.AdminExceptionCases {
			byKey[data.AdminExceptionCases[index].ExceptionKey] = &data.AdminExceptionCases[index]
		}
		for _, item := range items {
			item := item
			current := byKey[item.ID]
			if current == nil && item.Count > 0 {
				created := adminExceptionCase{ID: item.ID, ExceptionKey: item.ID, Title: item.Title, Description: item.Description, Module: item.Module, Severity: item.Severity, Count: item.Count, Roles: item.Roles, Status: "OPEN", SLADueAt: exceptionSLADue(now, item.Severity), FirstDetectedAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), History: []adminExceptionHistory{{Action: "DETECTED", ActorName: "system", To: "OPEN", At: now.Format(time.RFC3339)}}}
				data.AdminExceptionCases = append(data.AdminExceptionCases, created)
				byKey[item.ID] = &data.AdminExceptionCases[len(data.AdminExceptionCases)-1]
				continue
			}
			if current == nil {
				continue
			}
			previousCount := current.Count
			current.Title, current.Description, current.Module, current.Severity, current.Count, current.Roles = item.Title, item.Description, item.Module, item.Severity, item.Count, item.Roles
			if previousCount > 0 && item.Count == 0 && current.Status != "CLOSED" {
				from := current.Status
				current.Status = "RESOLVED"
				current.History = append(current.History, adminExceptionHistory{Action: "AUTO_RESOLVED", ActorName: "system", From: from, To: "RESOLVED", At: now.Format(time.RFC3339)})
			}
			if previousCount == 0 && item.Count > 0 && (current.Status == "CLOSED" || current.Status == "RESOLVED") {
				from := current.Status
				current.Status, current.ClosedAt, current.CloseReason, current.SLADueAt = "OPEN", "", "", exceptionSLADue(now, item.Severity)
				current.History = append(current.History, adminExceptionHistory{Action: "REOPENED", ActorName: "system", From: from, To: "OPEN", At: now.Format(time.RFC3339)})
			}
			current.UpdatedAt = now.Format(time.RFC3339)
		}
		result = append([]adminExceptionCase(nil), data.AdminExceptionCases...)
		return nil
	})
	return result, err
}

func validateExceptionMutation(current adminExceptionCase, mutation adminExceptionMutation) (adminExceptionMutation, error) {
	mutation.Status = strings.ToUpper(strings.TrimSpace(mutation.Status))
	if mutation.Status == "" {
		mutation.Status = current.Status
	}
	if !statusIn(mutation.Status, "OPEN", "IN_PROGRESS", "RESOLVED", "CLOSED") {
		return mutation, errors.New("invalid exception status")
	}
	mutation.AssigneeID, mutation.AssigneeName, mutation.Note, mutation.CloseReason = strings.TrimSpace(mutation.AssigneeID), strings.TrimSpace(mutation.AssigneeName), strings.TrimSpace(mutation.Note), strings.TrimSpace(mutation.CloseReason)
	if mutation.Status == "CLOSED" && mutation.CloseReason == "" {
		return mutation, errors.New("closeReason is required when closing an exception")
	}
	if mutation.SLADueAt != "" {
		if _, err := time.Parse(time.RFC3339, mutation.SLADueAt); err != nil {
			return mutation, errors.New("slaDueAt must be RFC3339")
		}
	}
	return mutation, nil
}

func applyExceptionMutation(current adminExceptionCase, mutation adminExceptionMutation, actorID string) (adminExceptionCase, error) {
	mutation, err := validateExceptionMutation(current, mutation)
	if err != nil {
		return current, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	from := current.Status
	if mutation.AssigneeID != "" || mutation.AssigneeName != "" {
		current.AssigneeID, current.AssigneeName = mutation.AssigneeID, mutation.AssigneeName
	}
	if mutation.SLADueAt != "" {
		current.SLADueAt = mutation.SLADueAt
	}
	current.Status = mutation.Status
	if current.Status == "CLOSED" {
		current.ClosedAt, current.CloseReason = now, mutation.CloseReason
	} else {
		current.ClosedAt, current.CloseReason = "", ""
	}
	current.UpdatedAt = now
	current.History = append(current.History, adminExceptionHistory{Action: "UPDATED", ActorID: actorID, ActorName: firstNonEmptyString(mutation.AssigneeName, actorID), From: from, To: current.Status, Note: firstNonEmptyString(mutation.Note, mutation.CloseReason), At: now})
	return current, nil
}

func (s *jsonStore) UpdateAdminExceptionCase(id string, mutation adminExceptionMutation, actorID string) (adminExceptionCase, error) {
	var result adminExceptionCase
	err := s.updateAdmin(func(data *adminPlatformData) error {
		for index, item := range data.AdminExceptionCases {
			if item.ID != id {
				continue
			}
			updated, err := applyExceptionMutation(item, mutation, actorID)
			if err != nil {
				return err
			}
			data.AdminExceptionCases[index], result = updated, updated
			return nil
		}
		return errors.New("exception case not found")
	})
	return result, err
}

func normalizeExperienceEvent(event adminExperienceEvent) (adminExperienceEvent, error) {
	event.EventType = strings.ToUpper(strings.TrimSpace(event.EventType))
	if !statusIn(event.EventType,
		"MODULE_VIEW", "SEARCH_RESULT_CLICK", "TASK_STARTED", "TASK_COMPLETED", "LIST_EXPORT", "BATCH_ACTION",
		"GUEST_OPEN_APP", "GUEST_VIEW_HOME", "GUEST_OPEN_CREATOR", "GUEST_INPUT_PROMPT", "GUEST_CLICK_GENERATE",
		"LOGIN_MODAL_SHOW", "LOGIN_START", "LOGIN_SUCCESS", "LOGIN_CANCEL",
		"PENDING_ACTION_RESUME_SUCCESS", "PENDING_ACTION_RESUME_FAILED", "GENERATION_SUCCESS_AFTER_LOGIN",
	) {
		return event, errors.New("invalid experience event type")
	}
	if event.ID == "" {
		event.ID = newEnterpriseResourceID("admin_event")
	}
	if event.OccurredAt == "" {
		event.OccurredAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if event.Metadata == nil {
		event.Metadata = map[string]any{}
	}
	return event, nil
}

func (s *jsonStore) RecordAdminExperienceEvent(event adminExperienceEvent) error {
	event, err := normalizeExperienceEvent(event)
	if err != nil {
		return err
	}
	return s.updateAdmin(func(data *adminPlatformData) error {
		data.AdminExperienceEvents = append(data.AdminExperienceEvents, event)
		if len(data.AdminExperienceEvents) > 5000 {
			data.AdminExperienceEvents = append([]adminExperienceEvent(nil), data.AdminExperienceEvents[len(data.AdminExperienceEvents)-5000:]...)
		}
		return nil
	})
}

func (s *jsonStore) ListAdminExperienceEvents(since time.Time) ([]adminExperienceEvent, error) {
	data, err := s.AdminData()
	if err != nil {
		return nil, err
	}
	items := make([]adminExperienceEvent, 0)
	for _, item := range data.AdminExperienceEvents {
		occurred, _ := time.Parse(time.RFC3339Nano, item.OccurredAt)
		if !occurred.Before(since) {
			items = append(items, item)
		}
	}
	return items, nil
}

func (s *postgresStore) SyncAdminExceptionCases(items []adminWorkspaceItem) ([]adminExceptionCase, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.Count <= 0 {
			_, _ = s.db.ExecContext(ctx, `UPDATE xz_admin_exception_cases SET current_count=0,status=CASE WHEN status IN ('OPEN','IN_PROGRESS') THEN 'RESOLVED' ELSE status END,updated_at=now() WHERE exception_key=$1`, item.ID)
			continue
		}
		roles, _ := json.Marshal(item.Roles)
		_, err := s.db.ExecContext(ctx, `INSERT INTO xz_admin_exception_cases(id,exception_key,title,description,module_id,severity,current_count,roles,status,sla_due_at,history) VALUES($1,$1,$2,$3,$4,$5,$6,$7,'OPEN',$8,$9) ON CONFLICT(exception_key) DO UPDATE SET title=excluded.title,description=excluded.description,module_id=excluded.module_id,severity=excluded.severity,current_count=excluded.current_count,roles=excluded.roles,status=CASE WHEN xz_admin_exception_cases.current_count=0 AND xz_admin_exception_cases.status IN ('CLOSED','RESOLVED') THEN 'OPEN' ELSE xz_admin_exception_cases.status END,sla_due_at=CASE WHEN xz_admin_exception_cases.current_count=0 AND xz_admin_exception_cases.status IN ('CLOSED','RESOLVED') THEN excluded.sla_due_at ELSE xz_admin_exception_cases.sla_due_at END,closed_at=CASE WHEN xz_admin_exception_cases.current_count=0 THEN NULL ELSE xz_admin_exception_cases.closed_at END,updated_at=now()`, item.ID, item.Title, item.Description, item.Module, item.Severity, item.Count, roles, exceptionSLADue(time.Now().UTC(), item.Severity), `[ {"action":"DETECTED","actorName":"system","to":"OPEN","at":"`+time.Now().UTC().Format(time.RFC3339)+`"} ]`)
		if err != nil {
			return nil, err
		}
	}
	return s.listPostgresExceptionCases(ctx)
}

func (s *postgresStore) listPostgresExceptionCases(ctx context.Context) ([]adminExceptionCase, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,exception_key,title,description,module_id,severity,current_count,roles,assignee_id,assignee_name,status,coalesce(to_char(sla_due_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),''),to_char(first_detected_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),to_char(updated_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),coalesce(to_char(closed_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),''),close_reason,history FROM xz_admin_exception_cases ORDER BY CASE status WHEN 'OPEN' THEN 0 WHEN 'IN_PROGRESS' THEN 1 WHEN 'RESOLVED' THEN 2 ELSE 3 END,sla_due_at NULLS LAST`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []adminExceptionCase{}
	for rows.Next() {
		var item adminExceptionCase
		var roles, history []byte
		if err := rows.Scan(&item.ID, &item.ExceptionKey, &item.Title, &item.Description, &item.Module, &item.Severity, &item.Count, &roles, &item.AssigneeID, &item.AssigneeName, &item.Status, &item.SLADueAt, &item.FirstDetectedAt, &item.UpdatedAt, &item.ClosedAt, &item.CloseReason, &history); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(roles, &item.Roles)
		_ = json.Unmarshal(history, &item.History)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) UpdateAdminExceptionCase(id string, mutation adminExceptionMutation, actorID string) (adminExceptionCase, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	items, err := s.listPostgresExceptionCases(ctx)
	if err != nil {
		return adminExceptionCase{}, err
	}
	var current adminExceptionCase
	found := false
	for _, item := range items {
		if item.ID == id {
			current, found = item, true
			break
		}
	}
	if !found {
		return current, sql.ErrNoRows
	}
	updated, err := applyExceptionMutation(current, mutation, actorID)
	if err != nil {
		return current, err
	}
	history, _ := json.Marshal(updated.History)
	_, err = s.db.ExecContext(ctx, `UPDATE xz_admin_exception_cases SET assignee_id=$2,assignee_name=$3,status=$4,sla_due_at=NULLIF($5,'')::timestamptz,closed_at=NULLIF($6,'')::timestamptz,close_reason=$7,history=$8,updated_at=now() WHERE id=$1`, id, updated.AssigneeID, updated.AssigneeName, updated.Status, updated.SLADueAt, updated.ClosedAt, updated.CloseReason, history)
	return updated, err
}

func (s *postgresStore) RecordAdminExperienceEvent(event adminExperienceEvent) error {
	event, err := normalizeExperienceEvent(event)
	if err != nil {
		return err
	}
	ctx, cancel := s.withTimeout()
	defer cancel()
	metadata, _ := json.Marshal(event.Metadata)
	_, err = s.db.ExecContext(ctx, `INSERT INTO xz_admin_experience_events(id,actor_id,actor_role,event_type,module_id,target_id,metadata,occurred_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, event.ID, event.ActorID, event.ActorRole, event.EventType, event.ModuleID, event.TargetID, metadata, event.OccurredAt)
	return err
}

func (s *postgresStore) ListAdminExperienceEvents(since time.Time) ([]adminExperienceEvent, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	rows, err := s.db.QueryContext(ctx, `SELECT id,actor_id,actor_role,event_type,module_id,target_id,metadata,occurred_at::text FROM xz_admin_experience_events WHERE occurred_at >= $1 ORDER BY occurred_at DESC LIMIT 20000`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []adminExperienceEvent{}
	for rows.Next() {
		var item adminExperienceEvent
		var metadata []byte
		if err := rows.Scan(&item.ID, &item.ActorID, &item.ActorRole, &item.EventType, &item.ModuleID, &item.TargetID, &metadata, &item.OccurredAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(metadata, &item.Metadata)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (a adminAPI) updateExceptionCase(w http.ResponseWriter, r *http.Request) {
	store, ok := a.store.(adminExperienceStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, errors.New("experience operations are unavailable"))
		return
	}
	var mutation adminExceptionMutation
	if err := json.NewDecoder(r.Body).Decode(&mutation); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	actorID, _ := actorFromRequest(r)
	item, err := store.UpdateAdminExceptionCase(strings.TrimSpace(r.PathValue("id")), mutation, actorID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (a adminAPI) recordExperienceEvent(w http.ResponseWriter, r *http.Request) {
	store, ok := a.store.(adminExperienceStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, errors.New("experience analytics are unavailable"))
		return
	}
	var event adminExperienceEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	event.ActorID, event.ActorRole = actorFromRequest(r)
	if err := store.RecordAdminExperienceEvent(event); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a adminAPI) experienceAnalytics(w http.ResponseWriter, r *http.Request) {
	store, ok := a.store.(adminExperienceStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, errors.New("experience analytics are unavailable"))
		return
	}
	days := intValue(r.URL.Query().Get("days"))
	if days <= 0 || days > 90 {
		days = 30
	}
	events, err := store.ListAdminExperienceEvents(time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	const minimumEvents = 100
	const minimumActiveDays = 7
	moduleViews, eventCounts := map[string]int{}, map[string]int{}
	sessions, actors, activeDaySet := map[string]bool{}, map[string]bool{}, map[string]bool{}
	syntheticEvents := 0
	humanEvents := make([]adminExperienceEvent, 0, len(events))
	for _, event := range events {
		if boolValue(event.Metadata["synthetic"]) {
			syntheticEvents++
			continue
		}
		humanEvents = append(humanEvents, event)
		eventCounts[event.EventType]++
		if event.EventType == "MODULE_VIEW" && event.ModuleID != "" {
			moduleViews[event.ModuleID]++
		}
		if sessionID := stringValue(event.Metadata["sessionId"]); sessionID != "" {
			sessions[sessionID] = true
		}
		if event.ActorID != "" {
			actors[event.ActorID] = true
		}
		if len(event.OccurredAt) >= len("2006-01-02") {
			activeDaySet[event.OccurredAt[:len("2006-01-02")]] = true
		}
	}
	type moduleCount struct {
		ModuleID string `json:"moduleId"`
		Count    int    `json:"count"`
	}
	modules := make([]moduleCount, 0, len(moduleViews))
	for id, count := range moduleViews {
		modules = append(modules, moduleCount{ModuleID: id, Count: count})
	}
	sort.Slice(modules, func(i, j int) bool {
		if modules[i].Count != modules[j].Count {
			return modules[i].Count < modules[j].Count
		}
		return modules[i].ModuleID < modules[j].ModuleID
	})
	low := modules
	if len(low) > 8 {
		low = low[:8]
	}
	started, completed := eventCounts["TASK_STARTED"], eventCounts["TASK_COMPLETED"]
	completionRate := 0.0
	if started > 0 {
		completionRate = float64(completed) / float64(started)
	}
	writeJSON(w, map[string]any{
		"days": days, "observedEvents": len(events), "totalEvents": len(humanEvents), "syntheticEvents": syntheticEvents,
		"eventCounts": eventCounts, "moduleViews": modules, "lowFrequencyModules": low, "taskCompletionRate": completionRate,
		"uniqueSessions": len(sessions), "uniqueActors": len(actors), "activeDays": len(activeDaySet),
		"minimumEvents": minimumEvents, "minimumActiveDays": minimumActiveDays,
		"sampleReady": len(humanEvents) >= minimumEvents && len(activeDaySet) >= minimumActiveDays,
	})
}
