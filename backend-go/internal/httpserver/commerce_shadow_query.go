package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	channelrules "xianzhi-ai/backend-go/internal/app/channelrules"
)

var errCommerceShadowDifferenceNotFound = errors.New("commerce shadow difference not found")

type commerceShadowDifferenceQuery struct {
	TenantID     string
	Status       string
	PlanID       string
	RuleSetID    string
	OrderKeyword string
	Limit        int
	Offset       int
}

type commerceShadowDifferenceItem struct {
	ID                   string          `json:"id"`
	TenantID             string          `json:"tenantId"`
	OrderID              string          `json:"orderId"`
	OrderNo              string          `json:"orderNo"`
	PlanID               string          `json:"planId"`
	ScenarioCode         string          `json:"scenarioCode"`
	ShadowRuleSetID      string          `json:"shadowRuleSetId"`
	ShadowRuleSetVersion int             `json:"shadowRuleSetVersion"`
	ShadowVersion        string          `json:"shadowVersion"`
	ComparisonStatus     string          `json:"comparisonStatus"`
	LegacyResult         json.RawMessage `json:"legacyResult"`
	V132Result           json.RawMessage `json:"v132Result"`
	Difference           json.RawMessage `json:"difference"`
	ErrorCode            string          `json:"errorCode"`
	ErrorMessage         string          `json:"errorMessage"`
	RelationshipSnapshot json.RawMessage `json:"relationshipSnapshot"`
	CreatedAt            time.Time       `json:"createdAt"`
}

type channelRolloutConfigView struct {
	TenantID                 string          `json:"tenantId"`
	ConfigVersion            int             `json:"configVersion"`
	Mode                     string          `json:"mode"`
	Enabled                  bool            `json:"enabled"`
	PinnedRuleSetID          string          `json:"pinnedRuleSetId"`
	PinnedRuleSetVersion     int             `json:"pinnedRuleSetVersion"`
	CanaryBasisPoints        int             `json:"canaryBasisPoints"`
	AllowOrderIDs            json.RawMessage `json:"allowOrderIds"`
	AllowUserIDs             json.RawMessage `json:"allowUserIds"`
	AllowPlanIDs             json.RawMessage `json:"allowPlanIds"`
	AllowPackageIDs          json.RawMessage `json:"allowPackageIds"`
	AllowTenantIDs           json.RawMessage `json:"allowTenantIds"`
	DenyOrderIDs             json.RawMessage `json:"denyOrderIds"`
	DenyUserIDs              json.RawMessage `json:"denyUserIds"`
	PercentageRolloutEnabled bool            `json:"percentageRolloutEnabled"`
	RealSwitchEnabled        bool            `json:"realSwitchEnabled"`
	ChangeReason             string          `json:"changeReason"`
	UpdatedBy                string          `json:"updatedBy"`
	UpdatedAt                time.Time       `json:"updatedAt"`
}

type channelRolloutConfigMutation struct {
	ExpectedVersion          int      `json:"expectedVersion"`
	Mode                     string   `json:"mode"`
	Enabled                  bool     `json:"enabled"`
	PinnedRuleSetID          string   `json:"pinnedRuleSetId"`
	PinnedRuleSetVersion     int      `json:"pinnedRuleSetVersion"`
	CanaryBasisPoints        int      `json:"canaryBasisPoints"`
	AllowOrderIDs            []string `json:"allowOrderIds"`
	AllowUserIDs             []string `json:"allowUserIds"`
	AllowPlanIDs             []string `json:"allowPlanIds"`
	AllowPackageIDs          []string `json:"allowPackageIds"`
	AllowTenantIDs           []string `json:"allowTenantIds"`
	DenyOrderIDs             []string `json:"denyOrderIds"`
	DenyUserIDs              []string `json:"denyUserIds"`
	PercentageRolloutEnabled bool     `json:"percentageRolloutEnabled"`
	RealSwitchEnabled        bool     `json:"realSwitchEnabled"`
	ChangeReason             string   `json:"changeReason"`
}

type commerceShadowQueryRepository interface {
	ListCommerceShadowDifferences(context.Context, commerceShadowDifferenceQuery) ([]commerceShadowDifferenceItem, int, error)
	GetCommerceShadowDifference(context.Context, string, string) (commerceShadowDifferenceItem, error)
	GetChannelRolloutConfig(context.Context, string) (channelRolloutConfigView, error)
	UpdateChannelRolloutConfig(context.Context, string, channelRolloutConfigMutation, string) (channelRolloutConfigView, error)
}

func commerceShadowDifferenceQueryFromRequest(r *http.Request, tenantID string) commerceShadowDifferenceQuery {
	page := positiveQueryInt(r, "page", 1)
	pageSize := positiveQueryInt(r, "pageSize", 50)
	if pageSize > 200 {
		pageSize = 200
	}
	return commerceShadowDifferenceQuery{
		TenantID: tenantID, Status: strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("status"))),
		PlanID: strings.TrimSpace(r.URL.Query().Get("planId")), RuleSetID: strings.TrimSpace(r.URL.Query().Get("ruleSetId")),
		OrderKeyword: strings.TrimSpace(r.URL.Query().Get("orderKeyword")), Limit: pageSize, Offset: (page - 1) * pageSize,
	}
}

func (a adminAPI) commerceShadowDifferences(w http.ResponseWriter, r *http.Request) {
	repository, ok := a.store.(commerceShadowQueryRepository)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, errors.New("commerce shadow query requires PostgreSQL"))
		return
	}
	tenantID, err := a.commissionTenantForRequest(r)
	if err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	query := commerceShadowDifferenceQueryFromRequest(r, tenantID)
	items, total, err := repository.ListCommerceShadowDifferences(r.Context(), query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"items": items, "total": total, "page": query.Offset/query.Limit + 1, "pageSize": query.Limit})
}

func (a adminAPI) commerceShadowDifference(w http.ResponseWriter, r *http.Request) {
	repository, ok := a.store.(commerceShadowQueryRepository)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, errors.New("commerce shadow query requires PostgreSQL"))
		return
	}
	tenantID, err := a.commissionTenantForRequest(r)
	if err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	item, err := repository.GetCommerceShadowDifference(r.Context(), tenantID, r.PathValue("id"))
	if errors.Is(err, errCommerceShadowDifferenceNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, item)
}

func (a adminAPI) channelRolloutConfig(w http.ResponseWriter, r *http.Request) {
	repository, ok := a.store.(commerceShadowQueryRepository)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, errors.New("channel rollout query requires PostgreSQL"))
		return
	}
	tenantID, err := a.commissionTenantForRequest(r)
	if err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	item, err := repository.GetChannelRolloutConfig(r.Context(), tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, item)
}

func (a adminAPI) updateChannelRolloutConfig(w http.ResponseWriter, r *http.Request) {
	repository, ok := a.store.(commerceShadowQueryRepository)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, errors.New("channel rollout update requires PostgreSQL"))
		return
	}
	tenantID, err := a.commissionTenantForRequest(r)
	if err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	var mutation channelRolloutConfigMutation
	if err := json.NewDecoder(r.Body).Decode(&mutation); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	mutation.Mode = strings.ToUpper(strings.TrimSpace(mutation.Mode))
	mutation.ChangeReason = strings.TrimSpace(mutation.ChangeReason)
	if mutation.ExpectedVersion <= 0 || mutation.ChangeReason == "" {
		writeError(w, http.StatusBadRequest, errors.New("expectedVersion and changeReason are required"))
		return
	}
	if mutation.PercentageRolloutEnabled {
		writeError(w, http.StatusBadRequest, errors.New("global percentage rollout is not enabled in the whitelist phase"))
		return
	}
	mutation.AllowPlanIDs = append(append([]string{}, mutation.AllowPlanIDs...), mutation.AllowPackageIDs...)
	config := channelrules.RolloutConfig{
		TenantID: tenantID, ConfigVersion: mutation.ExpectedVersion,
		Mode: channelrules.RolloutMode(mutation.Mode), Enabled: mutation.Enabled,
		PinnedRuleSetID: mutation.PinnedRuleSetID, PinnedRuleSetVersion: mutation.PinnedRuleSetVersion,
		CanaryBasisPoints: mutation.CanaryBasisPoints, AllowOrderIDs: mutation.AllowOrderIDs,
		AllowUserIDs: mutation.AllowUserIDs, AllowPlanIDs: mutation.AllowPlanIDs, AllowTenantIDs: mutation.AllowTenantIDs,
		DenyOrderIDs: mutation.DenyOrderIDs, DenyUserIDs: mutation.DenyUserIDs,
		PercentageRolloutEnabled: mutation.PercentageRolloutEnabled, RealSwitchEnabled: mutation.RealSwitchEnabled,
	}
	if err := config.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	actorID, _ := actorFromRequest(r)
	item, err := repository.UpdateChannelRolloutConfig(r.Context(), tenantID, mutation, actorID)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, item)
}

func (s *postgresStore) ListCommerceShadowDifferences(ctx context.Context, query commerceShadowDifferenceQuery) ([]commerceShadowDifferenceItem, int, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return nil, 0, err
	}
	args := []any{query.TenantID}
	where := []string{"tenant_id = $1"}
	appendFilter := func(column, value string) {
		if value != "" {
			args = append(args, value)
			where = append(where, fmt.Sprintf("%s = $%d", column, len(args)))
		}
	}
	appendFilter("comparison_status", query.Status)
	appendFilter("plan_id", query.PlanID)
	appendFilter("shadow_rule_set_id", query.RuleSetID)
	if query.OrderKeyword != "" {
		args = append(args, "%"+query.OrderKeyword+"%")
		where = append(where, fmt.Sprintf("(order_id ILIKE $%d OR order_no ILIKE $%d)", len(args), len(args)))
	}
	whereSQL := strings.Join(where, " AND ")
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM xz_commercial_shadow_differences WHERE "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, query.Limit, query.Offset)
	rows, err := s.db.QueryContext(ctx, `
		SELECT id,tenant_id,order_id,order_no,plan_id,coalesce(scenario_code,''),
		       coalesce(shadow_rule_set_id,''),coalesce(shadow_rule_set_version,0),shadow_version,
		       comparison_status,legacy_result,coalesce(v132_result,'null'::jsonb),
		       coalesce(difference,'null'::jsonb),error_code,error_message,relationship_snapshot,created_at
		FROM xz_commercial_shadow_differences WHERE `+whereSQL+`
		ORDER BY created_at DESC, id DESC
		LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]commerceShadowDifferenceItem, 0)
	for rows.Next() {
		item, err := scanCommerceShadowDifference(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (s *postgresStore) GetCommerceShadowDifference(ctx context.Context, tenantID, id string) (commerceShadowDifferenceItem, error) {
	item, err := scanCommerceShadowDifference(s.db.QueryRowContext(ctx, `
		SELECT id,tenant_id,order_id,order_no,plan_id,coalesce(scenario_code,''),
		       coalesce(shadow_rule_set_id,''),coalesce(shadow_rule_set_version,0),shadow_version,
		       comparison_status,legacy_result,coalesce(v132_result,'null'::jsonb),
		       coalesce(difference,'null'::jsonb),error_code,error_message,relationship_snapshot,created_at
		FROM xz_commercial_shadow_differences WHERE tenant_id=$1 AND id=$2
	`, strings.TrimSpace(tenantID), strings.TrimSpace(id)))
	if errors.Is(err, sql.ErrNoRows) {
		return commerceShadowDifferenceItem{}, errCommerceShadowDifferenceNotFound
	}
	return item, err
}

func (s *postgresStore) GetChannelRolloutConfig(ctx context.Context, tenantID string) (channelRolloutConfigView, error) {
	var item channelRolloutConfigView
	err := s.db.QueryRowContext(ctx, `
		SELECT tenant_id,config_version,mode,enabled,pinned_rule_set_id,pinned_rule_set_version,
		       canary_basis_points,allow_order_ids,allow_user_ids,allow_plan_ids,allow_plan_ids,allow_tenant_ids,deny_order_ids,deny_user_ids,
		       percentage_rollout_enabled,real_switch_enabled,change_reason,updated_by,updated_at
		FROM xz_channel_rollout_configs WHERE tenant_id=$1
	`, strings.TrimSpace(tenantID)).Scan(
		&item.TenantID, &item.ConfigVersion, &item.Mode, &item.Enabled,
		&item.PinnedRuleSetID, &item.PinnedRuleSetVersion, &item.CanaryBasisPoints,
		&item.AllowOrderIDs, &item.AllowUserIDs, &item.AllowPlanIDs, &item.AllowPackageIDs, &item.AllowTenantIDs, &item.DenyOrderIDs, &item.DenyUserIDs,
		&item.PercentageRolloutEnabled, &item.RealSwitchEnabled, &item.ChangeReason, &item.UpdatedBy, &item.UpdatedAt,
	)
	return item, err
}

func (s *postgresStore) UpdateChannelRolloutConfig(ctx context.Context, tenantID string, mutation channelRolloutConfigMutation, actorID string) (channelRolloutConfigView, error) {
	allowOrders, _ := json.Marshal(mutation.AllowOrderIDs)
	allowUsers, _ := json.Marshal(mutation.AllowUserIDs)
	allowPlans, _ := json.Marshal(mutation.AllowPlanIDs)
	allowTenants, _ := json.Marshal(mutation.AllowTenantIDs)
	denyOrders, _ := json.Marshal(mutation.DenyOrderIDs)
	denyUsers, _ := json.Marshal(mutation.DenyUserIDs)
	result, err := s.db.ExecContext(ctx, `
		UPDATE xz_channel_rollout_configs
		SET mode=$3,enabled=$4,pinned_rule_set_id=$5,pinned_rule_set_version=$6,
		    canary_basis_points=$7,allow_order_ids=$8::jsonb,allow_user_ids=$9::jsonb,
		    allow_plan_ids=$10::jsonb,allow_tenant_ids=$11::jsonb,deny_order_ids=$12::jsonb,deny_user_ids=$13::jsonb,
		    percentage_rollout_enabled=$14,real_switch_enabled=$15,change_reason=$16,updated_by=$17
		WHERE tenant_id=$1 AND config_version=$2
	`, tenantID, mutation.ExpectedVersion, mutation.Mode, mutation.Enabled,
		mutation.PinnedRuleSetID, mutation.PinnedRuleSetVersion, mutation.CanaryBasisPoints,
		allowOrders, allowUsers, allowPlans, allowTenants, denyOrders, denyUsers,
		mutation.PercentageRolloutEnabled, mutation.RealSwitchEnabled, mutation.ChangeReason, actorID)
	if err != nil {
		return channelRolloutConfigView{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return channelRolloutConfigView{}, err
	}
	if affected != 1 {
		return channelRolloutConfigView{}, errors.New("rollout config version conflict")
	}
	return s.GetChannelRolloutConfig(ctx, tenantID)
}

type commerceShadowScanner interface{ Scan(...any) error }

func scanCommerceShadowDifference(scanner commerceShadowScanner) (commerceShadowDifferenceItem, error) {
	var item commerceShadowDifferenceItem
	err := scanner.Scan(
		&item.ID, &item.TenantID, &item.OrderID, &item.OrderNo, &item.PlanID,
		&item.ScenarioCode, &item.ShadowRuleSetID, &item.ShadowRuleSetVersion,
		&item.ShadowVersion, &item.ComparisonStatus, &item.LegacyResult, &item.V132Result,
		&item.Difference, &item.ErrorCode, &item.ErrorMessage, &item.RelationshipSnapshot, &item.CreatedAt,
	)
	return item, err
}
