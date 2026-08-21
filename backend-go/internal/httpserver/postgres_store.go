package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	operationcenter "xianzhi-ai/backend-go/internal/app/operationcenter"

	commissionapp "xianzhi-ai/backend-go/internal/app/commission"
	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
)

type postgresStore struct {
	db           *sql.DB
	fallbackPath string
	readyMu      sync.Mutex
	ready        bool
}

const aiCapabilitySettingsID = "ai_capability_config"

func newPostgresPrimaryStore(db *sql.DB, fallbackPath string) *postgresStore {
	return &postgresStore{db: db, fallbackPath: fallbackPath}
}

func (s *postgresStore) withTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

func (s *postgresStore) ensureReady(ctx context.Context) error {
	s.readyMu.Lock()
	defer s.readyMu.Unlock()
	if s.ready {
		return nil
	}
	backend := postgresStateBackend{db: s.db, fallbackPath: s.fallbackPath}
	if err := backend.ensureSchema(ctx); err != nil {
		return err
	}
	if err := backend.ensureProjectionSchema(ctx); err != nil {
		return err
	}
	if err := ensureDualIdentityCommerceSchema(ctx, s.db); err != nil {
		return err
	}
	if err := ensureGovernanceSchema(ctx, s.db); err != nil {
		return err
	}
	if err := ensureUserRBACSchema(ctx, s.db); err != nil {
		return err
	}
	if err := ensureEnterpriseCenterSchema(ctx, s.db); err != nil {
		return err
	}
	if err := ensureAdminEnterpriseSchema(ctx, s.db); err != nil {
		return err
	}
	if err := ensureMarketingSchema(ctx, s.db); err != nil {
		return err
	}
	if err := s.seedPrimaryTables(ctx); err != nil {
		return err
	}
	if err := syncUserRBACProjection(ctx, s.db); err != nil {
		return err
	}
	if err := s.seedAPITables(ctx); err != nil {
		return err
	}
	if err := s.seedMarketingPolicyCatalog(ctx); err != nil {
		return err
	}
	if err := s.ensureCanonicalBillingPlans(ctx); err != nil {
		return err
	}
	if err := s.ensureUserModelRouteProjection(ctx); err != nil {
		return err
	}
	s.ready = true
	return nil
}

func (s *postgresStore) seedPrimaryTables(ctx context.Context) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `select count(*) from xz_users`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	data := seedAdminData()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := upsertUsers(ctx, tx, data.Users); err != nil {
		return err
	}
	if err := upsertPlans(ctx, tx, data.Plans); err != nil {
		return err
	}
	if err := upsertPointAccounts(ctx, tx, data.PointAccounts); err != nil {
		return err
	}
	if err := upsertTokenRecords(ctx, tx, data.TokenRecords); err != nil {
		return err
	}
	if err := upsertChannelAgents(ctx, tx, data.ChannelAgents); err != nil {
		return err
	}
	if err := upsertOperationCenters(ctx, tx, data.OperationCenters); err != nil {
		return err
	}
	if err := upsertUserModelRoutesFromUsers(ctx, tx, data.Users); err != nil {
		return err
	}
	if err := upsertCommissions(ctx, tx, data.Commissions); err != nil {
		return err
	}
	if err := upsertWithdrawals(ctx, tx, data.Withdrawals); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *postgresStore) seedAPITables(ctx context.Context) error {
	var channelCount int
	if err := s.db.QueryRowContext(ctx, `select count(*) from xz_api_channels`).Scan(&channelCount); err != nil {
		return err
	}
	var keyCount int
	if err := s.db.QueryRowContext(ctx, `select count(*) from xz_api_keys`).Scan(&keyCount); err != nil {
		return err
	}
	if channelCount > 0 && keyCount > 0 {
		return nil
	}
	data := seedAdminData()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if channelCount == 0 {
		if err := upsertAPIChannels(ctx, tx, data.APIChannels); err != nil {
			return err
		}
	}
	if keyCount == 0 {
		if err := upsertAPIKeys(ctx, tx, data.APIKeys); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *postgresStore) ensureCanonicalBillingPlans(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := insertMissingPlans(ctx, tx, canonicalBillingPlans()); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *postgresStore) seedMarketingCommissionRules(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, rule := range defaultCommissionRules() {
		if err := upsertMarketingCommissionRule(ctx, tx, rule); err != nil {
			return err
		}
	}
	if err := disableLegacyMarketingRules(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *postgresStore) seedMarketingPolicyCatalog(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, role := range defaultMarketingRoleRows() {
		if err := upsertMarketingRole(ctx, tx, role); err != nil {
			return err
		}
	}
	for _, plan := range defaultMarketingUpgradePlans() {
		if err := upsertMarketingUpgradePlan(ctx, tx, plan); err != nil {
			return err
		}
	}
	for _, rule := range defaultCommissionRules() {
		if err := upsertMarketingCommissionRule(ctx, tx, rule); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *postgresStore) ensureUserModelRouteProjection(ctx context.Context) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `select count(*) from xz_user_model_routes`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	rows, err := s.db.QueryContext(ctx, `select raw from xz_users order by created_at, id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	users := []adminUser{}
	for rows.Next() {
		var item adminUser
		if err := scanRawJSON(rows, &item); err != nil {
			return err
		}
		if len(item.ModelRoutes) > 0 {
			users = append(users, item)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(users) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := upsertUserModelRoutesFromUsers(ctx, tx, users); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *postgresStore) AdminData() (adminPlatformData, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminPlatformData{}, err
	}
	data := seedAdminData()
	var err error
	if data.Users, err = s.listUsers(ctx); err != nil {
		return data, err
	}
	if data.AuthMergeRequests, err = s.listAuthMergeRequests(ctx, ""); err != nil {
		return data, err
	}
	if data.Plans, err = s.listPlans(ctx); err != nil {
		return data, err
	}
	if data.PointAccounts, err = s.listPointAccounts(ctx); err != nil {
		return data, err
	}
	if data.TokenRecords, err = s.listTokenRecords(ctx); err != nil {
		return data, err
	}
	if data.SystemSettings, err = s.getSystemSettings(ctx); err != nil {
		return data, err
	}
	if data.Orders, err = s.listOrders(ctx); err != nil {
		return data, err
	}
	if data.ChannelAgents, err = s.listChannelAgents(ctx); err != nil {
		return data, err
	}
	if data.OperationCenters, err = s.listOperationCenters(ctx); err != nil {
		return data, err
	}
	if data.CustomerRelations, err = s.listCustomerRelations(ctx); err != nil {
		return data, err
	}
	if data.Commissions, err = s.listCommissions(ctx); err != nil {
		return data, err
	}
	if data.CommissionRules, err = s.listMarketingCommissionRules(ctx); err != nil {
		return data, err
	}
	if data.BillingEvents, err = s.listBillingEvents(ctx); err != nil {
		return data, err
	}
	if data.Withdrawals, err = s.listWithdrawals(ctx); err != nil {
		return data, err
	}
	if data.APIChannels, err = s.listAPIChannels(ctx); err != nil {
		return data, err
	}
	if data.APIKeys, err = s.listAPIKeys(ctx); err != nil {
		return data, err
	}
	if data.GenerationTasks, err = s.ListGenerationTasks(); err != nil {
		return data, err
	}
	if data.Assets, err = s.ListAssets(); err != nil {
		return data, err
	}
	if data, err = s.applyAICapabilityConfig(ctx, data); err != nil {
		return data, err
	}
	// Estimate and other AdminData callers must see the same published billing
	// rules as CreatePendingGenerationTask (aiCapabilityAdminData), otherwise
	// Seedance defaults to code BasePrice 12 (90 pts) instead of published 80 (600).
	if data.BillingRuleVersions, err = s.listBillingRuleVersionsContext(ctx); err != nil {
		return data, err
	}
	data = withAdminDefaults(data)
	return data, nil
}

func (s *postgresStore) GetActiveUser(userID string) (adminUser, bool, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminUser{}, false, err
	}
	var item adminUser
	err := s.db.QueryRowContext(ctx, `
		select raw
		from xz_users
		where id = $1 and upper(coalesce(status, '')) = 'ACTIVE'
		limit 1
	`, userID).Scan(rawScanner(&item))
	if errors.Is(err, sql.ErrNoRows) {
		return adminUser{}, false, nil
	}
	if err != nil {
		return adminUser{}, false, err
	}
	return item, true, nil
}

func (s *postgresStore) GetChannelAgentForUser(userID string) (adminChannelAgent, bool, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminChannelAgent{}, false, err
	}
	var item adminChannelAgent
	err := s.db.QueryRowContext(ctx, `
		select raw
		from xz_channel_agents
		where user_id = $1
		order by created_at desc, id desc
		limit 1
	`, userID).Scan(rawScanner(&item))
	if errors.Is(err, sql.ErrNoRows) {
		return adminChannelAgent{}, false, nil
	}
	if err != nil {
		return adminChannelAgent{}, false, err
	}
	return item, true, nil
}

// GetChannelWorkbenchAgentForUser resolves commercial access from the canonical
// business identity and profile projections. It intentionally does not inspect
// xz_users.role. Operation centers use their dedicated workspace and are not
// projected as ordinary agents.
func (s *postgresStore) GetChannelWorkbenchAgentForUser(userID string) (adminChannelAgent, bool, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminChannelAgent{}, false, err
	}
	var identityType string
	err := s.db.QueryRowContext(ctx, `
		SELECT identity_type
		FROM xz_user_business_identities
		WHERE user_id=$1 AND identity_type='AGENT'
		  AND identity_status='ACTIVE' AND ended_at IS NULL
		  AND effective_at<=clock_timestamp() AND (expires_at IS NULL OR expires_at>clock_timestamp())
		  AND commission_enabled=true
		ORDER BY identity_version DESC
		LIMIT 1
	`, strings.TrimSpace(userID)).Scan(&identityType)
	if errors.Is(err, sql.ErrNoRows) {
		return adminChannelAgent{}, false, nil
	}
	if err != nil {
		return adminChannelAgent{}, false, err
	}
	_ = identityType
	var item adminChannelAgent
	err = s.db.QueryRowContext(ctx, `SELECT raw FROM xz_channel_agents WHERE user_id=$1 AND upper(coalesce(status,''))='ACTIVE' ORDER BY created_at DESC,id DESC LIMIT 1`, userID).Scan(rawScanner(&item))
	if errors.Is(err, sql.ErrNoRows) {
		return adminChannelAgent{}, false, nil
	}
	return item, err == nil, err
}

func (s *postgresStore) GetOperationCenterForUser(userID string) (adminOperationCenter, bool, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminOperationCenter{}, false, err
	}
	var item adminOperationCenter
	err := s.db.QueryRowContext(ctx, `
		select raw
		from xz_operation_centers
		where user_id = $1 and upper(coalesce(status, '')) = 'ACTIVE'
		order by created_at desc, id desc
		limit 1
	`, userID).Scan(rawScanner(&item))
	if errors.Is(err, sql.ErrNoRows) {
		return adminOperationCenter{}, false, nil
	}
	if err != nil {
		return adminOperationCenter{}, false, err
	}
	return item, true, nil
}

func (s *postgresStore) OnlineImageSettings() (adminPlatformData, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminPlatformData{}, err
	}
	data := seedAdminData()
	var err error
	if data.APIChannels, err = s.listAPIChannels(ctx); err != nil {
		return data, err
	}
	if data.APIKeys, err = s.listAPIKeys(ctx); err != nil {
		return data, err
	}
	data = withAdminDefaults(data)
	return data, nil
}

func (s *postgresStore) UserAccountData(userID string) (adminPlatformData, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminPlatformData{}, err
	}
	data := adminPlatformData{}
	var err error
	if data.Plans, err = s.listPlans(ctx); err != nil {
		return data, err
	}
	data.Plans = withPlanDefaults(data.Plans)
	if data.TokenRecords, err = s.listTokenRecordsForUser(ctx, userID); err != nil {
		return data, err
	}
	if data.Orders, err = s.listOrdersForUser(ctx, userID); err != nil {
		return data, err
	}
	if data.BillingEvents, err = s.listBillingEventsForUser(ctx, userID); err != nil {
		return data, err
	}
	return data, nil
}

func (s *postgresStore) ChannelDataForAgent(agentUserID string, agentID string, includeContent bool, billingEventLimit int) (adminPlatformData, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminPlatformData{}, err
	}
	data := adminPlatformData{}
	var err error
	if data.Users, err = s.listUsersBasic(ctx); err != nil {
		return data, err
	}
	if data.Plans, err = s.listPlans(ctx); err != nil {
		return data, err
	}
	data.Plans = withPlanDefaults(data.Plans)
	if data.ChannelAgents, err = s.listChannelAgents(ctx); err != nil {
		return data, err
	}
	visibleCustomerIDs := channelVisibleCustomerIDs(data.Users, data.ChannelAgents, agentUserID, agentID)
	visibleUserIDs := stringBoolMapKeys(visibleCustomerIDs)
	if data.PointAccounts, err = s.listPointAccountsForUsers(ctx, visibleUserIDs); err != nil {
		return data, err
	}
	if data.Orders, err = s.listOrdersForUsers(ctx, visibleUserIDs); err != nil {
		return data, err
	}
	if data.Commissions, err = s.listCommissionsForAgent(ctx, agentID); err != nil {
		return data, err
	}
	if billingEventLimit != 0 {
		if data.BillingEvents, err = s.listBillingEventsForUsers(ctx, visibleUserIDs, billingEventLimit); err != nil {
			return data, err
		}
	}
	if data.Withdrawals, err = s.listWithdrawalsForAgent(ctx, agentID); err != nil {
		return data, err
	}
	if data.SystemSettings, err = s.getSystemSettings(ctx); err != nil {
		return data, err
	}
	if data.SystemSettings.Brand.Name == "" {
		data.SystemSettings = defaultSystemSettings()
	}
	if includeContent {
		if data.GenerationTasks, err = s.listGenerationTasksForUsers(ctx, visibleUserIDs, 0); err != nil {
			return data, err
		}
		if data.Assets, err = s.listAssetsForUsers(ctx, visibleUserIDs, 0); err != nil {
			return data, err
		}
	}
	return data, nil
}

func (s *postgresStore) ChannelContentCountsForUsers(userIDs []string) (int, int, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return 0, 0, err
	}
	taskCount, err := s.countRowsForUsers(ctx, "xz_generation_tasks", userIDs)
	if err != nil {
		return 0, 0, err
	}
	assetCount, err := s.countRowsForUsers(ctx, "xz_assets", userIDs)
	if err != nil {
		return 0, 0, err
	}
	return taskCount, assetCount, nil
}

func (s *postgresStore) ListGenerationTasks() ([]generationTask, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, generationTaskSummarySelect+`
		order by created_at desc nulls last, id desc
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanGenerationTaskSummaryRows(rows)
}

func (s *postgresStore) ListAssets() ([]asset, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, assetSummarySelect+`
		where deleted_at is null
		order by created_at desc nulls last, id desc
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAssetSummaryRows(rows)
}

func (s *postgresStore) SaveUploadedAsset(item asset) (asset, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return asset{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return asset{}, err
	}
	defer tx.Rollback()
	if err := insertAsset(ctx, tx, item); err != nil {
		return asset{}, err
	}
	if err := tx.Commit(); err != nil {
		return asset{}, err
	}
	return item, nil
}

const generationTaskSummarySelect = `
	select
		id,
		user_id,
		coalesce(tenant_id, ''),
		coalesce(organization_id, ''),
		coalesce(billing_account_type, 'PERSONAL'),
		coalesce(billing_account_id, ''),
		coalesce(module_code, ''),
		coalesce(type, ''),
		coalesce(model, ''),
		coalesce(billing_type, ''),
		coalesce(status, ''),
		coalesce(progress, 0),
		coalesce(point_cost, 0),
		coalesce(prompt, ''),
		coalesce((params - 'referenceImages' - 'reference_images' - 'image_urls' - 'imageUrls' - 'inputImageUrls' - 'inputImagesSnapshot' - 'maskDraft')::text, '{}'),
		coalesce(result_ids::text, '[]'),
		coalesce(error::text, 'null'),
		coalesce(created_at, ''),
		coalesce(updated_at, ''),
		coalesce(worker_finished_at, ''),
		coalesce(client_request_id, ''),
		coalesce(task_status, 'CREATED'),
		coalesce(billing_status, 'UNQUOTED'),
		coalesce(billing_rule_version_id, ''),
		coalesce(quoted_points, 0),
		coalesce(reserved_points, 0),
		coalesce(captured_points, 0),
		coalesce(released_points, 0),
		coalesce(refunded_points, 0),
		supplier_cost,
		estimated_margin,
		coalesce(provider_channel, '')
	from xz_generation_tasks
`

const assetSummarySelect = `
	select
		id,
		user_id,
		coalesce(tenant_id, ''),
		coalesce(organization_id, ''),
		coalesce(task_id, ''),
		coalesce(name, ''),
		coalesce(media_type, ''),
		coalesce(url, ''),
		coalesce(thumbnail_url, ''),
		coalesce(favorite, false),
		coalesce(metadata::text, '{}'),
		coalesce(deleted_at::text, ''),
		coalesce(created_at, ''),
		coalesce(updated_at, '')
	from xz_assets
`

// assetWorkspaceListSelect is the first-paint projection. It keeps the same
// scan shape as assetSummarySelect but must not detoast thumbnail_url TEXT.
const assetWorkspaceListSelect = `
	select
		id,
		user_id,
		coalesce(tenant_id, ''),
		coalesce(organization_id, ''),
		coalesce(task_id, ''),
		coalesce(name, ''),
		coalesce(media_type, ''),
		coalesce(url, ''),
		'' as thumbnail_url,
		coalesce(favorite, false),
		coalesce(metadata::text, '{}'),
		coalesce(deleted_at::text, ''),
		coalesce(created_at, ''),
		coalesce(updated_at, '')
	from xz_assets
`

func (s *postgresStore) ListGenerationTasksForUser(userID string, limit int) ([]generationTask, error) {
	items, _, err := s.ListGenerationTasksPageForUser(userID, limit, 0, false)
	return items, err
}

func (s *postgresStore) ListGenerationTasksPageForUser(userID string, limit int, offset int, prioritize bool) ([]generationTask, int, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = defaultUserContentListLimit
	}
	if offset < 0 {
		offset = 0
	}
	contextType, tenantID, _, err := s.currentTenantScopeForUser(ctx, userID)
	if err != nil {
		return nil, 0, err
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `
		select count(*) from xz_generation_tasks
		where user_id=$1 and (($2='ENTERPRISE' and tenant_id=$3) or ($2<>'ENTERPRISE' and (tenant_id is null or tenant_id='tenant_default')))
	`, userID, contextType, tenantID).Scan(&total); err != nil {
		return nil, 0, err
	}
	orderBy := ` order by created_at desc nulls last, id desc`
	if prioritize {
		orderBy = ` order by case when upper(coalesce(status, '')) in ('PENDING','QUEUED','RUNNING','PROCESSING','RETRYING','FAILED','ERROR') then 0 else 1 end, created_at desc nulls last, id desc`
	}
	rows, err := s.db.QueryContext(ctx, generationTaskSummarySelect+`
		where user_id=$1 and (($4='ENTERPRISE' and tenant_id=$5) or ($4<>'ENTERPRISE' and (tenant_id is null or tenant_id='tenant_default')))
	`+orderBy+`
		limit $2 offset $3
	`, userID, limit, offset, contextType, tenantID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items, err := scanGenerationTaskSummaryRows(rows)
	return items, total, err
}

func (s *postgresStore) listGenerationTasksForUsers(ctx context.Context, userIDs []string, limit int) ([]generationTask, error) {
	where, args := postgresTextInCondition("user_id", userIDs)
	query := generationTaskSummarySelect + `
		where ` + where + `
		order by created_at desc nulls last, id desc
	`
	if limit > 0 {
		args = append(args, limit)
		query += ` limit $` + strconv.Itoa(len(args))
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanGenerationTaskSummaryRows(rows)
}

func (s *postgresStore) GetGenerationTaskForUser(userID string, id string) (generationTask, bool, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return generationTask{}, false, err
	}
	contextType, tenantID, _, err := s.currentTenantScopeForUser(ctx, userID)
	if err != nil {
		return generationTask{}, false, err
	}
	rows, err := s.db.QueryContext(ctx, generationTaskSummarySelect+`
		where user_id=$1 and id=$2 and (($3='ENTERPRISE' and tenant_id=$4) or ($3<>'ENTERPRISE' and (tenant_id is null or tenant_id='tenant_default')))
		limit 1
	`, userID, id, contextType, tenantID)
	if err != nil {
		return generationTask{}, false, err
	}
	defer rows.Close()
	items, err := scanGenerationTaskSummaryRows(rows)
	if err != nil {
		return generationTask{}, false, err
	}
	if len(items) == 0 {
		return generationTask{}, false, nil
	}
	return items[0], true, nil
}

func (s *postgresStore) ListAssetsForUser(userID string, limit int) ([]asset, error) {
	items, _, err := s.ListAssetsPageForUser(userID, limit, 0)
	return items, err
}

func (s *postgresStore) GetAssetForUser(userID string, id string) (asset, bool, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return asset{}, false, err
	}
	contextType, tenantID, _, err := s.currentTenantScopeForUser(ctx, userID)
	if err != nil {
		return asset{}, false, err
	}
	rows, err := s.db.QueryContext(ctx, assetSummarySelect+`
		where user_id=$1 and id=$2 and deleted_at is null
		  and (($3='ENTERPRISE' and tenant_id=$4) or ($3<>'ENTERPRISE' and (tenant_id is null or tenant_id='tenant_default')))
		limit 1
	`, userID, id, contextType, tenantID)
	if err != nil {
		return asset{}, false, err
	}
	defer rows.Close()
	items, err := scanAssetSummaryRows(rows)
	if err != nil {
		return asset{}, false, err
	}
	if len(items) == 0 {
		return asset{}, false, nil
	}
	return items[0], true, nil
}

func (s *postgresStore) ListAssetsPageForUser(userID string, limit int, offset int) ([]asset, int, error) {
	return s.listAssetsPageForUserSelect(userID, limit, offset, assetSummarySelect)
}

func (s *postgresStore) ListAssetsForWorkspaceList(userID string, limit int) ([]asset, error) {
	items, _, err := s.listAssetsPageForUserSelect(userID, limit, 0, assetWorkspaceListSelect)
	return items, err
}

func (s *postgresStore) GetAssetByID(id string) (asset, bool, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return asset{}, false, err
	}
	rows, err := s.db.QueryContext(ctx, assetSummarySelect+`
		where id=$1 and deleted_at is null
		limit 1
	`, id)
	if err != nil {
		return asset{}, false, err
	}
	defer rows.Close()
	items, err := scanAssetSummaryRows(rows)
	if err != nil {
		return asset{}, false, err
	}
	if len(items) == 0 {
		return asset{}, false, nil
	}
	return items[0], true, nil
}

func (s *postgresStore) listAssetsPageForUserSelect(userID string, limit int, offset int, selectSQL string) ([]asset, int, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = defaultUserContentListLimit
	}
	if offset < 0 {
		offset = 0
	}
	contextType, tenantID, _, err := s.currentTenantScopeForUser(ctx, userID)
	if err != nil {
		return nil, 0, err
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `
		select count(*) from xz_assets
		where user_id=$1 and deleted_at is null and (($2='ENTERPRISE' and tenant_id=$3) or ($2<>'ENTERPRISE' and (tenant_id is null or tenant_id='tenant_default')))
	`, userID, contextType, tenantID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx, selectSQL+`
		where user_id=$1 and deleted_at is null and (($4='ENTERPRISE' and tenant_id=$5) or ($4<>'ENTERPRISE' and (tenant_id is null or tenant_id='tenant_default')))
		order by created_at desc nulls last, id desc
		limit $2 offset $3
	`, userID, limit, offset, contextType, tenantID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items, err := scanAssetSummaryRows(rows)
	return items, total, err
}

func (s *postgresStore) AssetListSummaryForUser(userID string, monthPrefix string) (assetListSummary, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return assetListSummary{}, err
	}
	contextType, tenantID, _, err := s.currentTenantScopeForUser(ctx, userID)
	if err != nil {
		return assetListSummary{}, err
	}
	var summary assetListSummary
	err = s.db.QueryRowContext(ctx, `
		select
			count(*),
			count(*) filter (where coalesce(favorite, false)),
			count(*) filter (where coalesce(created_at, '') like $2 || '%'),
			coalesce(sum(
				case
					when jsonb_typeof(metadata->'fileSize') = 'number' then (metadata->>'fileSize')::bigint
					when jsonb_typeof(metadata->'fileSizeBytes') = 'number' then (metadata->>'fileSizeBytes')::bigint
					when jsonb_typeof(metadata->'sizeBytes') = 'number' then (metadata->>'sizeBytes')::bigint
					else 0
				end
			), 0)
		from xz_assets
		where user_id = $1 and deleted_at is null
		  and (($3='ENTERPRISE' and tenant_id=$4) or ($3<>'ENTERPRISE' and (tenant_id is null or tenant_id='tenant_default')))
	`, userID, monthPrefix, contextType, tenantID).Scan(&summary.Total, &summary.FavoriteTotal, &summary.MonthTotal, &summary.StorageBytes)
	return summary, err
}

func (s *postgresStore) listAssetsForUsers(ctx context.Context, userIDs []string, limit int) ([]asset, error) {
	where, args := postgresTextInCondition("user_id", userIDs)
	query := assetSummarySelect + `
		where ` + where + ` and deleted_at is null
		order by created_at desc nulls last, id desc
	`
	if limit > 0 {
		args = append(args, limit)
		query += ` limit $` + strconv.Itoa(len(args))
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAssetSummaryRows(rows)
}

func scanGenerationTaskSummaryRows(rows *sql.Rows) ([]generationTask, error) {
	items := []generationTask{}
	for rows.Next() {
		var item generationTask
		var paramsRaw string
		var resultIDsRaw string
		var errorRaw string
		if err := rows.Scan(
			&item.ID,
			&item.UserID,
			&item.TenantID,
			&item.OrganizationID,
			&item.BillingAccountType,
			&item.BillingAccountID,
			&item.ModuleCode,
			&item.Type,
			&item.Model,
			&item.BillingType,
			&item.Status,
			&item.Progress,
			&item.PointCost,
			&item.Prompt,
			&paramsRaw,
			&resultIDsRaw,
			&errorRaw,
			&item.CreatedAt,
			&item.UpdatedAt,
			&item.WorkerFinishedAt,
			&item.ClientRequestID,
			&item.TaskStatus,
			&item.BillingStatus,
			&item.BillingRuleVersionID,
			&item.QuotedPoints,
			&item.ReservedPoints,
			&item.CapturedPoints,
			&item.ReleasedPoints,
			&item.RefundedPoints,
			&item.SupplierCost,
			&item.EstimatedMargin,
			&item.ProviderChannel,
		); err != nil {
			return nil, err
		}
		item.Params = decodeObjectJSON(paramsRaw)
		item.ResultIDs = decodeStringSliceJSON(resultIDsRaw)
		item.Error = decodeAnyJSON(errorRaw)
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanAssetSummaryRows(rows *sql.Rows) ([]asset, error) {
	items := []asset{}
	for rows.Next() {
		var item asset
		var metadataRaw string
		if err := rows.Scan(
			&item.ID,
			&item.UserID,
			&item.TenantID,
			&item.OrganizationID,
			&item.TaskID,
			&item.Name,
			&item.MediaType,
			&item.URL,
			&item.ThumbnailURL,
			&item.Favorite,
			&metadataRaw,
			&item.DeletedAt,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.Metadata = decodeObjectJSON(metadataRaw)
		item.ThumbnailURL = compactListInlineMediaURL(item.ThumbnailURL)
		items = append(items, item)
	}
	return items, rows.Err()
}

func compactListInlineMediaURL(value string) string {
	text := strings.TrimSpace(value)
	// Compact thumbnails are intentionally returned inline so the first asset
	// grid does not wait for several multi-megabyte object-storage downloads.
	// Only discard unexpectedly large inline media that is likely an original.
	if strings.HasPrefix(text, "data:") && len(text) > 128<<10 {
		return ""
	}
	return value
}

func decodeObjectJSON(raw string) map[string]any {
	var item map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &item); err != nil || item == nil {
		return map[string]any{}
	}
	return item
}

func decodeStringSliceJSON(raw string) []string {
	var items []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &items); err == nil {
		return items
	}
	var values []any
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &values); err != nil {
		return nil
	}
	items = make([]string, 0, len(values))
	for _, value := range values {
		text := strings.TrimSpace(fmt.Sprint(value))
		if text != "" {
			items = append(items, text)
		}
	}
	return items
}

func decodeAnyJSON(raw string) any {
	var value any
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &value); err != nil {
		return nil
	}
	return value
}

func (s *postgresStore) UserAIState(userID string) (userAIState, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return userAIState{}, err
	}
	var item userAIState
	err := s.db.QueryRowContext(ctx, `select raw from xz_ai_state where user_id = $1`, userID).Scan(rawScanner(&item))
	if errors.Is(err, sql.ErrNoRows) {
		return normalizeUserAIState(userAIState{}, userID), nil
	}
	if err != nil {
		return userAIState{}, err
	}
	return normalizeUserAIState(item, userID), nil
}

func (s *postgresStore) UpdateUserAIState(userID string, req userAIState) (userAIState, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return userAIState{}, err
	}
	item := normalizeUserAIState(req, userID)
	item.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
		insert into xz_ai_state (user_id, favorite_task_ids, hidden_task_ids, favorite_collections, agent_conversations, active_conversation_id, active_collection_id, updated_at, raw)
		values ($1,$2::jsonb,$3::jsonb,$4::jsonb,$5::jsonb,$6,$7,$8,$9::jsonb)
		on conflict (user_id) do update set
			favorite_task_ids = excluded.favorite_task_ids,
			hidden_task_ids = excluded.hidden_task_ids,
			favorite_collections = excluded.favorite_collections,
			agent_conversations = excluded.agent_conversations,
			active_conversation_id = excluded.active_conversation_id,
			active_collection_id = excluded.active_collection_id,
			updated_at = excluded.updated_at,
			raw = excluded.raw
	`, item.UserID, jsonProjection(item.FavoriteTaskIDs), jsonProjection(item.HiddenTaskIDs), jsonProjection(item.FavoriteCollections), jsonProjection(item.AgentConversations), item.ActiveConversationID, item.ActiveCollectionID, item.UpdatedAt, jsonProjection(item))
	return item, err
}

func (s *postgresStore) PointAccount(userID string) (pointAccount, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return pointAccount{}, err
	}
	var item pointAccount
	err := s.db.QueryRowContext(ctx, `
		select id, user_id, available, frozen
		from xz_point_accounts
		where user_id = $1
	`, userID).Scan(&item.ID, &item.UserID, &item.Available, &item.Frozen)
	if errors.Is(err, sql.ErrNoRows) {
		item = pointAccount{ID: "points_" + shortID(userID), UserID: userID, Available: 0}
	} else if err != nil {
		return item, err
	}
	total, err := totalPointsForUserTx(ctx, s.db, userID, item.Available, item.Frozen)
	if err != nil {
		return pointAccount{}, err
	}
	item.Total = total
	return item, nil
}

func (s *postgresStore) CreateGenerationTask(req createGenerationTaskRequest) (generationTask, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return generationTask{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return generationTask{}, err
	}
	defer func() { _ = tx.Rollback() }()

	userID := strings.TrimSpace(req.UserID)
	if userID == "" {
		userID = "user_000002"
	}
	if req.Params == nil {
		req.Params = map[string]any{}
	}
	if existing, ok, err := generationTaskByClientRequestTx(ctx, tx, userID, req.ClientRequestID); err != nil {
		return generationTask{}, err
	} else if ok {
		return existing, tx.Commit()
	}
	capabilityData, err := s.aiCapabilityAdminData(ctx)
	if err != nil {
		return generationTask{}, err
	}
	rule := billingRuleForRequest(req, capabilityData)
	pointCost := generationPointCostForRequest(req, capabilityData)
	authorization, err := s.authorizeModelCallContext(ctx, tx, userID, requestModuleCode(req))
	if err != nil {
		return generationTask{}, err
	}
	if err := enforcePostgresGenerationConcurrencyTx(ctx, tx, userID, authorization); err != nil {
		return generationTask{}, err
	}
	req.Params["tenant_id"] = authorization.TenantID
	req.Params["organization_id"] = authorization.OrganizationID
	req.Params["billing_scope"] = authorization.BillingScope
	req.Params["billing_account_id"] = authorization.BillingAccountID
	var account pgPointAccount
	if authorization.ContextType != contextEnterprise {
		account, err = pgLoadPersonalAccountForUserTx(ctx, tx, userID)
		if err != nil {
			return generationTask{}, err
		}
		if account.Available < int64(pointCost) {
			return generationTask{}, fmt.Errorf("insufficient remaining points: available %d, required %d", account.Available, pointCost)
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	taskID, err := nextTableID(ctx, tx, "xz_generation_tasks", "task")
	if err != nil {
		return generationTask{}, err
	}
	task := generationTask{
		ID:               taskID,
		ClientRequestID:  strings.TrimSpace(req.ClientRequestID),
		UserID:           userID,
		Type:             req.Type,
		Prompt:           req.Prompt,
		Params:           req.Params,
		Model:            req.Model,
		Status:           "SUCCEEDED",
		TaskStatus:       taskStatusSucceeded,
		BillingStatus:    billingStatusCaptured,
		Progress:         100,
		PointCost:        pointCost,
		QuotedPoints:     float64(pointCost),
		ReservedPoints:   float64(pointCost),
		CapturedPoints:   float64(pointCost),
		ResultIDs:        []string{},
		CreatedAt:        now,
		UpdatedAt:        now,
		WorkerFinishedAt: now,
	}
	if authorization.ContextType != contextEnterprise {
		task.BillingEngine = personalLotBillingEngine
		task.PersonalPointAccountID = account.ID
	}
	applyGenerationTaskCapabilitySnapshot(&task, req, rule)
	task.ProviderChannel = firstNonEmptyString(stringValue(req.Params["provider_channel"]), stringValue(req.Params["channel_id"]))
	applyTaskSupplierCost(&task, capabilityData.ProviderCosts)
	balanceBefore, balanceAfter := int(account.Available), int(account.Available)-pointCost
	if authorization.ContextType == contextEnterprise {
		reservation, err := s.reserveEnterpriseComputeTx(ctx, tx, authorization, int64(pointCost), "GENERATION_TASK", task.ID)
		if err != nil {
			return generationTask{}, err
		}
		balanceBefore, balanceAfter = int(reservation.BalanceBefore), int(reservation.BalanceAfter)
		task.Params["billing_ledger_id"] = reservation.LedgerID
		task.Params["billing_reserved"] = true
	}
	count := imageCount(req.Params)
	for i := 0; i < count; i++ {
		assetID, err := nextTableID(ctx, tx, "xz_assets", "asset")
		if err != nil {
			return generationTask{}, err
		}
		task.ResultIDs = append(task.ResultIDs, assetID)
		item := generatedAssetForRequest(req, userID, taskID, assetID, i, now)
		if err := insertAsset(ctx, tx, item); err != nil {
			return generationTask{}, err
		}
	}
	if _, err := insertBillingLifecycleEventV1(ctx, tx, task, "QUOTE", float64(pointCost), map[string]any{"modelCode": task.Model}); err != nil {
		return generationTask{}, err
	}
	if authorization.ContextType != contextEnterprise {
		pointStore := NewPostgresPersonalPointStore(s.db)
		reserved, err := pointStore.reserveTx(ctx, tx, PersonalPointReserveCommand{AccountID: account.ID, UserID: userID, BusinessType: "GENERATION_TASK", BusinessID: task.ID, RequestedPoints: int64(pointCost), IdempotencyKey: "generation:reserve:" + task.ID})
		if err != nil {
			return generationTask{}, err
		}
		task.PersonalPointReservationID = reserved.Reservation.ID
		if _, err := pointStore.captureTx(ctx, tx, PersonalPointCaptureCommand{AccountID: account.ID, UserID: userID, ReservationID: task.PersonalPointReservationID, Points: int64(pointCost), IdempotencyKey: "generation:capture:" + task.ID}); err != nil {
			return generationTask{}, err
		}
	} else {
		if _, err := insertScopedWalletEntryV1(ctx, tx, task, authorization.BillingAccountID, "RESERVE", pointCost, balanceBefore, balanceAfter, 0, pointCost, "生成任务冻结"); err != nil {
			return generationTask{}, err
		}
		if _, err := insertScopedWalletEntryV1(ctx, tx, task, authorization.BillingAccountID, "CAPTURE", pointCost, balanceAfter, balanceAfter, pointCost, 0, "生成任务确认扣费"); err != nil {
			return generationTask{}, err
		}
	}
	if _, err := insertBillingLifecycleEventV1(ctx, tx, task, "RESERVE", float64(pointCost), nil); err != nil {
		return generationTask{}, err
	}
	if _, err := insertBillingLifecycleEventV1(ctx, tx, task, "CAPTURE", float64(pointCost), nil); err != nil {
		return generationTask{}, err
	}
	if err := insertGenerationTask(ctx, tx, task); err != nil {
		return generationTask{}, err
	}
	event, commissions, err := generationBillingArtifactsForTx(ctx, tx, task, balanceBefore, balanceAfter, now)
	if err != nil {
		return generationTask{}, err
	}
	if err := insertBillingEvent(ctx, tx, event); err != nil {
		return generationTask{}, err
	}
	for _, commission := range commissions {
		if err := insertCommission(ctx, tx, commission); err != nil {
			return generationTask{}, err
		}
	}
	if err := s.recordModelUsageTx(ctx, tx, authorization, task, req.Params); err != nil {
		return generationTask{}, err
	}
	if err := insertAuditLog(ctx, tx, userID, "MEMBER", "generation.create", "generation_task", task.ID, "", "", 200, map[string]any{"pointCost": pointCost}); err != nil {
		return generationTask{}, err
	}
	if err := tx.Commit(); err != nil {
		return generationTask{}, err
	}
	return task, nil
}

func (s *postgresStore) CreatePendingGenerationTask(req createGenerationTaskRequest) (generationTask, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return generationTask{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return generationTask{}, err
	}
	defer func() { _ = tx.Rollback() }()
	userID := strings.TrimSpace(req.UserID)
	if userID == "" {
		userID = "user_000002"
	}
	if req.Params == nil {
		req.Params = map[string]any{}
	}
	if existing, ok, err := generationTaskByClientRequestTx(ctx, tx, userID, req.ClientRequestID); err != nil {
		return generationTask{}, err
	} else if ok {
		return existing, tx.Commit()
	}
	capabilityData, err := s.aiCapabilityAdminData(ctx)
	if err != nil {
		return generationTask{}, err
	}
	rule := billingRuleForRequest(req, capabilityData)
	pointCost := generationPointCostForRequest(req, capabilityData)
	authorization, err := s.authorizeModelCallContext(ctx, tx, userID, requestModuleCode(req))
	if err != nil {
		return generationTask{}, err
	}
	if err := enforcePostgresGenerationConcurrencyTx(ctx, tx, userID, authorization); err != nil {
		return generationTask{}, err
	}
	req.Params["tenant_id"] = authorization.TenantID
	req.Params["organization_id"] = authorization.OrganizationID
	req.Params["billing_scope"] = authorization.BillingScope
	req.Params["billing_account_id"] = authorization.BillingAccountID
	var account pgPointAccount
	if authorization.ContextType != contextEnterprise {
		account, err = pgLoadPersonalAccountForUserTx(ctx, tx, userID)
		if err != nil {
			return generationTask{}, err
		}
		if account.Available < int64(pointCost) {
			return generationTask{}, fmt.Errorf("insufficient remaining points: available %d, required %d", account.Available, pointCost)
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	taskID, err := nextTableID(ctx, tx, "xz_generation_tasks", "task")
	if err != nil {
		return generationTask{}, err
	}
	balanceBefore, balanceAfter := int(account.Available), int(account.Available)-pointCost
	if authorization.ContextType == contextEnterprise {
		reservation, err := s.reserveEnterpriseComputeTx(ctx, tx, authorization, int64(pointCost), "GENERATION_TASK", taskID)
		if err != nil {
			return generationTask{}, err
		}
		balanceBefore, balanceAfter = int(reservation.BalanceBefore), int(reservation.BalanceAfter)
		req.Params["billing_ledger_id"] = reservation.LedgerID
	}
	params := generationBillingReservationParams(req.Params, now, pointCost, balanceBefore, balanceAfter)
	req.Params = params
	task := generationTask{
		ID:              taskID,
		ClientRequestID: strings.TrimSpace(req.ClientRequestID),
		UserID:          userID,
		Type:            req.Type,
		Prompt:          req.Prompt,
		Params:          params,
		Model:           req.Model,
		Status:          "PROCESSING",
		TaskStatus:      taskStatusQueued,
		BillingStatus:   billingStatusReserved,
		Progress:        5,
		PointCost:       pointCost,
		QuotedPoints:    float64(pointCost),
		ReservedPoints:  float64(pointCost),
		ResultIDs:       []string{},
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if authorization.ContextType != contextEnterprise {
		task.BillingEngine = personalLotBillingEngine
		task.PersonalPointAccountID = account.ID
	}
	applyGenerationTaskCapabilitySnapshot(&task, req, rule)
	task.ProviderChannel = firstNonEmptyString(stringValue(req.Params["provider_channel"]), stringValue(req.Params["channel_id"]))
	applyTaskSupplierCost(&task, capabilityData.ProviderCosts)
	if _, err := insertBillingLifecycleEventV1(ctx, tx, task, "QUOTE", float64(pointCost), map[string]any{"modelCode": task.Model}); err != nil {
		return generationTask{}, err
	}
	if authorization.ContextType != contextEnterprise {
		reserved, err := NewPostgresPersonalPointStore(s.db).reserveTx(ctx, tx, PersonalPointReserveCommand{AccountID: account.ID, UserID: userID, BusinessType: "GENERATION_TASK", BusinessID: task.ID, RequestedPoints: int64(pointCost), IdempotencyKey: "generation:reserve:" + task.ID})
		if err != nil {
			return generationTask{}, err
		}
		task.PersonalPointReservationID = reserved.Reservation.ID
	} else {
		if _, err := insertScopedWalletEntryV1(ctx, tx, task, authorization.BillingAccountID, "RESERVE", pointCost, balanceBefore, balanceAfter, 0, pointCost, "生成任务冻结"); err != nil {
			return generationTask{}, err
		}
	}
	if _, err := insertBillingLifecycleEventV1(ctx, tx, task, "RESERVE", float64(pointCost), nil); err != nil {
		return generationTask{}, err
	}
	if err := insertGenerationTask(ctx, tx, task); err != nil {
		return generationTask{}, err
	}
	if err := insertAuditLog(ctx, tx, userID, "MEMBER", "generation.enqueue", "generation_task", task.ID, "", "", 202, map[string]any{"pointCost": pointCost, "billingReserved": true}); err != nil {
		return generationTask{}, err
	}
	return task, tx.Commit()
}

func (s *postgresStore) CompleteGenerationTask(id string, req createGenerationTaskRequest) (generationTask, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return generationTask{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return generationTask{}, err
	}
	defer func() { _ = tx.Rollback() }()
	task, err := generationTaskForUpdate(ctx, tx, id)
	if err != nil {
		return generationTask{}, err
	}
	if task.Status == "SUCCEEDED" || task.Status == "FAILED" || task.Status == "CANCELLED" {
		return task, tx.Commit()
	}
	userID := strings.TrimSpace(task.UserID)
	if userID == "" {
		userID = strings.TrimSpace(req.UserID)
	}
	usesPersonalPoints, err := generationTaskUsesPersonalPoints(task)
	if err != nil {
		return generationTask{}, err
	}
	authorization, err := s.authorizationForStoredTaskTx(ctx, tx, task, true)
	if err != nil {
		return generationTask{}, err
	}
	if usesPersonalPoints != (authorization.ContextType != contextEnterprise) {
		return generationTask{}, ErrPersonalPointContextMismatch
	}
	var account pgPointAccount
	personalPointCost := 0
	if usesPersonalPoints {
		account, personalPointCost, err = validatePostgresGenerationPersonalLotMarkerTx(ctx, tx, task)
		if err != nil {
			return generationTask{}, err
		}
	}
	pointCost := task.PointCost
	now := time.Now().UTC().Format(time.RFC3339Nano)
	req.UserID = userID
	req.Type = firstNonEmptyString(req.Type, task.Type)
	req.Prompt = firstNonEmptyString(req.Prompt, task.Prompt)
	req.Model = firstNonEmptyString(req.Model, task.Model)
	if req.Params == nil {
		req.Params = task.Params
	}
	req.Params["tenant_id"] = authorization.TenantID
	req.Params["organization_id"] = authorization.OrganizationID
	req.Params["billing_scope"] = authorization.BillingScope
	req.Params["billing_account_id"] = authorization.BillingAccountID
	capabilityData, err := s.aiCapabilityAdminData(ctx)
	if err != nil {
		return generationTask{}, err
	}
	rule := billingRuleForRequest(req, capabilityData)
	if pointCost <= 0 {
		pointCost = generationPointCostForRequest(req, capabilityData)
	}
	pointCost = generationTaskReservedPointCost(task, pointCost)
	if usesPersonalPoints {
		if pointCost != personalPointCost {
			return generationTask{}, ErrPersonalPointImportConflict
		}
		pointCost = personalPointCost
	}
	reserved := generationTaskReservedAndActive(task)
	if usesPersonalPoints && !reserved {
		return generationTask{}, ErrPersonalPointReservationMarkerMissing
	}
	task.Status = "SUCCEEDED"
	task.TaskStatus = taskStatusSucceeded
	task.BillingStatus = billingStatusCaptured
	task.Progress = 100
	task.PointCost = pointCost
	task.CapturedPoints = float64(pointCost)
	task.Error = nil
	task.UpdatedAt = now
	task.WorkerFinishedAt = now
	task.ResultIDs = []string{}
	applyGenerationTaskCapabilitySnapshot(&task, req, rule)
	task.ProviderChannel = firstNonEmptyString(stringValue(req.Params["provider_channel"]), stringValue(req.Params["channel_id"]), task.ProviderChannel)
	applyTaskSupplierCost(&task, capabilityData.ProviderCosts)
	count := imageCount(req.Params)
	for i := 0; i < count; i++ {
		assetID, err := nextTableID(ctx, tx, "xz_assets", "asset")
		if err != nil {
			return generationTask{}, err
		}
		task.ResultIDs = append(task.ResultIDs, assetID)
		item := generatedAssetForRequest(req, userID, task.ID, assetID, i, now)
		if err := insertAsset(ctx, tx, item); err != nil {
			return generationTask{}, err
		}
	}
	balanceBefore := int(account.Available)
	balanceAfter := int(account.Available) - pointCost
	if reserved {
		fallbackBalance := int(account.Available)
		if !usesPersonalPoints {
			fallbackBalance = intValue(task.Params[generationBillingReservationBalanceAfterKey])
		}
		balanceBefore, balanceAfter = generationTaskReservationBalances(task, fallbackBalance, pointCost)
		if !usesPersonalPoints {
			if _, err := insertScopedWalletEntryV1(ctx, tx, task, authorization.BillingAccountID, "CAPTURE", pointCost, balanceAfter, balanceAfter, pointCost, 0, "生成任务确认扣费"); err != nil {
				return generationTask{}, err
			}
		} else {
			if _, err := NewPostgresPersonalPointStore(s.db).captureTx(ctx, tx, PersonalPointCaptureCommand{AccountID: task.PersonalPointAccountID, UserID: userID, ReservationID: task.PersonalPointReservationID, Points: int64(pointCost), IdempotencyKey: "generation:capture:" + task.ID}); err != nil {
				return generationTask{}, err
			}
		}
	} else {
		if usesPersonalPoints {
			return generationTask{}, ErrPersonalPointReservationMarkerMissing
		}
		task.QuotedPoints = float64(pointCost)
		task.ReservedPoints = float64(pointCost)
		if _, err := insertBillingLifecycleEventV1(ctx, tx, task, "QUOTE", float64(pointCost), map[string]any{"modelCode": task.Model}); err != nil {
			return generationTask{}, err
		}
		if !usesPersonalPoints {
			reservation, err := s.reserveEnterpriseComputeTx(ctx, tx, authorization, int64(pointCost), "GENERATION_TASK", task.ID)
			if err != nil {
				return generationTask{}, err
			}
			balanceBefore, balanceAfter = int(reservation.BalanceBefore), int(reservation.BalanceAfter)
			if _, err := insertScopedWalletEntryV1(ctx, tx, task, authorization.BillingAccountID, "RESERVE", pointCost, balanceBefore, balanceAfter, 0, pointCost, "生成任务冻结"); err != nil {
				return generationTask{}, err
			}
			if _, err := insertScopedWalletEntryV1(ctx, tx, task, authorization.BillingAccountID, "CAPTURE", pointCost, balanceAfter, balanceAfter, pointCost, 0, "生成任务确认扣费"); err != nil {
				return generationTask{}, err
			}
		}
		if _, err := insertBillingLifecycleEventV1(ctx, tx, task, "RESERVE", float64(pointCost), nil); err != nil {
			return generationTask{}, err
		}
	}
	if _, err := insertBillingLifecycleEventV1(ctx, tx, task, "CAPTURE", float64(pointCost), nil); err != nil {
		return generationTask{}, err
	}
	if err := insertGenerationTask(ctx, tx, task); err != nil {
		return generationTask{}, err
	}
	event, commissions, err := generationBillingArtifactsForTx(ctx, tx, task, balanceBefore, balanceAfter, now)
	if err != nil {
		return generationTask{}, err
	}
	if err := insertBillingEvent(ctx, tx, event); err != nil {
		return generationTask{}, err
	}
	for _, commission := range commissions {
		if err := insertCommission(ctx, tx, commission); err != nil {
			return generationTask{}, err
		}
	}
	if err := s.recordModelUsageTx(ctx, tx, authorization, task, req.Params); err != nil {
		return generationTask{}, err
	}
	if err := insertAuditLog(ctx, tx, userID, "MEMBER", "generation.complete", "generation_task", task.ID, "", "", 200, map[string]any{"pointCost": pointCost, "billingReserved": reserved}); err != nil {
		return generationTask{}, err
	}
	return task, tx.Commit()
}

func (s *postgresStore) RecordPPTGenerationUsage(task pptapp.Task) (adminBillingEvent, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	userID := strings.TrimSpace(task.UserID)
	taskID := strings.TrimSpace(task.TaskID)
	if userID == "" || taskID == "" {
		return adminBillingEvent{}, ErrInvalidPointCommand
	}
	task.UserID = userID
	task.TaskID = taskID
	if err := s.ensureReady(ctx); err != nil {
		return adminBillingEvent{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return adminBillingEvent{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if event, ok, err := billingEventForTaskMetricTx(ctx, tx, task.TaskID, billingMetricPPTGenerate); err != nil || ok {
		if err != nil {
			return adminBillingEvent{}, err
		}
		return event, tx.Commit()
	}

	capabilityData, err := s.aiCapabilityAdminData(ctx)
	if err != nil {
		return adminBillingEvent{}, err
	}
	pointCost := pptPointCostWithRules(task, capabilityData)
	authorization, err := s.authorizeModelCallContext(ctx, tx, userID, modulePPTGeneration)
	if err != nil {
		return adminBillingEvent{}, err
	}
	balanceBefore, balanceAfter := 0, 0
	if authorization.ContextType == contextEnterprise {
		reservation, err := s.reserveEnterpriseComputeTx(ctx, tx, authorization, int64(pointCost), "PPT_TASK", task.TaskID)
		if err != nil {
			return adminBillingEvent{}, err
		}
		balanceBefore, balanceAfter = int(reservation.BalanceBefore), int(reservation.BalanceAfter)
	} else {
		balanceBefore, balanceAfter, err = chargePostgresPersonalPointUsage(ctx, s.db, tx, personalPointUsageChargeCommand{
			UserID: userID, BusinessType: "PPT_TASK", BusinessID: task.TaskID,
			Points: int64(pointCost), IdempotencyPrefix: "ppt:" + task.TaskID,
		})
		if err != nil {
			return adminBillingEvent{}, err
		}
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	user, err := userByIDForTx(ctx, tx, userID)
	if err != nil {
		return adminBillingEvent{}, err
	}
	agent, hasAgent, err := directActiveAgentForUserTx(ctx, tx, userID)
	if err != nil {
		return adminBillingEvent{}, err
	}
	event := pptBillingEvent(task, pointCost, balanceBefore, balanceAfter, now, user, agent, hasAgent)
	event.TenantID = authorization.TenantID
	if err := insertBillingEvent(ctx, tx, event); err != nil {
		return adminBillingEvent{}, err
	}
	commissions, err := commissionArtifactsForUserTx(ctx, tx, userID, task.TaskID, "PPT_GENERATION", "ppt_generation", event.AmountCents, now)
	if err != nil {
		return adminBillingEvent{}, err
	}
	for _, commission := range commissions {
		if err := insertCommission(ctx, tx, commission); err != nil {
			return adminBillingEvent{}, err
		}
	}
	usageTask := generationTask{ID: task.TaskID, UserID: userID, TenantID: authorization.TenantID, OrganizationID: authorization.OrganizationID, BillingAccountType: authorization.BillingScope, BillingAccountID: authorization.BillingAccountID, ModuleCode: modulePPTGeneration, Model: event.Model, PointCost: pointCost}
	if err := s.recordModelUsageTx(ctx, tx, authorization, usageTask, map[string]any{}); err != nil {
		return adminBillingEvent{}, err
	}
	if err := insertAuditLog(ctx, tx, userID, "MEMBER", "ppt.generate", "ppt_task", task.TaskID, "", "", 200, map[string]any{"pointCost": pointCost, "slideCount": pptSlideQuantity(task)}); err != nil {
		return adminBillingEvent{}, err
	}
	if err := tx.Commit(); err != nil {
		return adminBillingEvent{}, err
	}
	return event, nil
}

func (s *postgresStore) FailGenerationTask(id string, message string) (generationTask, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return generationTask{}, err
	}
	defer func() { _ = tx.Rollback() }()
	task, err := generationTaskForUpdate(ctx, tx, id)
	if err != nil {
		return generationTask{}, err
	}
	task, refunded, changed, err := s.mutatePostgresGenerationFailureTx(ctx, tx, task, message, "FAILED", taskStatusFailed)
	if err != nil {
		return generationTask{}, err
	}
	if changed {
		if err := insertAuditLog(ctx, tx, task.UserID, "MEMBER", "generation.fail", "generation_task", task.ID, "", "", 502, map[string]any{"error": message, "pointCost": task.PointCost, "billingRefunded": refunded}); err != nil {
			return generationTask{}, err
		}
	}
	return task, tx.Commit()
}

func validatePostgresGenerationPersonalLotMarkerTx(ctx context.Context, tx *sql.Tx, task generationTask) (pgPointAccount, int, error) {
	if task.BillingEngine != personalLotBillingEngine || strings.TrimSpace(task.PersonalPointAccountID) == "" || strings.TrimSpace(task.PersonalPointReservationID) == "" {
		return pgPointAccount{}, 0, ErrPersonalPointReservationMarkerMissing
	}
	pointCost, err := generationTaskExactReservationPointCost(task)
	if err != nil {
		return pgPointAccount{}, 0, err
	}
	account, ok, err := pgLoadAccount(ctx, tx, task.PersonalPointAccountID, task.UserID, true)
	if err != nil {
		return pgPointAccount{}, 0, err
	}
	if !ok {
		return pgPointAccount{}, 0, ErrPersonalPointReservationMarkerMissing
	}
	reservation, err := pgScanReservation(tx.QueryRowContext(ctx, `SELECT `+pgReservationColumns+` FROM xz_personal_point_reservations WHERE id=$1 FOR UPDATE`, task.PersonalPointReservationID))
	if errors.Is(err, sql.ErrNoRows) {
		return pgPointAccount{}, 0, ErrPersonalPointReservationMarkerMissing
	}
	if err != nil {
		return pgPointAccount{}, 0, err
	}
	if reservation.AccountID != account.ID || reservation.UserID != task.UserID {
		return pgPointAccount{}, 0, ErrPointOwnership
	}
	if reservation.BusinessType != "GENERATION_TASK" || reservation.BusinessID != task.ID {
		return pgPointAccount{}, 0, ErrPersonalPointImportConflict
	}
	allocations, err := pgLoadAllocations(ctx, tx, reservation.ID, account.ID, task.UserID, true)
	if err != nil {
		return pgPointAccount{}, 0, err
	}
	if err := validatePersonalGenerationReservationState(reservation, allocations, pointCost); err != nil {
		return pgPointAccount{}, 0, err
	}
	return account, int(pointCost), nil
}

func (s *postgresStore) mutatePostgresGenerationFailureTx(ctx context.Context, tx *sql.Tx, task generationTask, message, terminalStatus, terminalTaskStatus string) (generationTask, bool, bool, error) {
	if task.Status == "SUCCEEDED" || task.Status == "FAILED" || task.Status == "CANCELLED" {
		return task, false, false, nil
	}
	usesPersonalPoints, err := generationTaskUsesPersonalPoints(task)
	if err != nil {
		return generationTask{}, false, false, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	pointCost := generationTaskReservedPointCost(task, task.PointCost)
	refunded := false
	if pointCost > 0 {
		authorization, err := s.authorizationForStoredTaskTx(ctx, tx, task, false)
		if err != nil {
			return generationTask{}, false, false, err
		}
		if usesPersonalPoints != (authorization.ContextType != contextEnterprise) {
			return generationTask{}, false, false, ErrPersonalPointContextMismatch
		}
		if usesPersonalPoints {
			account, validatedPointCost, err := validatePostgresGenerationPersonalLotMarkerTx(ctx, tx, task)
			if err != nil {
				return generationTask{}, false, false, err
			}
			if validatedPointCost != pointCost {
				return generationTask{}, false, false, ErrPersonalPointImportConflict
			}
			nextAvailable := int(account.Available) + validatedPointCost
			if _, err := NewPostgresPersonalPointStore(s.db).releaseTx(ctx, tx, PersonalPointReleaseCommand{AccountID: task.PersonalPointAccountID, UserID: task.UserID, ReservationID: task.PersonalPointReservationID, Points: int64(validatedPointCost), IdempotencyKey: "generation:release:" + task.ID}); err != nil {
				return generationTask{}, false, false, err
			}
			task.Params = generationBillingRefundParams(task.Params, now, int(account.Available), nextAvailable)
			refunded = true
		} else if generationTaskReservedAndActive(task) {
			before := int64(intValue(task.Params[generationBillingReservationBalanceAfterKey]))
			if err := s.reverseEnterpriseComputeTx(ctx, tx, authorization, int64(pointCost), "GENERATION_TASK", task.ID); err != nil {
				return generationTask{}, false, false, err
			}
			task.Params = generationBillingRefundParams(task.Params, now, int(before), int(before)+pointCost)
			if _, err := insertScopedWalletEntryV1(ctx, tx, task, authorization.BillingAccountID, "RELEASE", pointCost, int(before), int(before)+pointCost, pointCost, 0, "生成失败解冻"); err != nil {
				return generationTask{}, false, false, err
			}
			refunded = true
		}
		if refunded {
			task.BillingStatus = billingStatusReleased
			task.ReleasedPoints = float64(pointCost)
			if _, err := insertBillingLifecycleEventV1(ctx, tx, task, "RELEASE", float64(pointCost), nil); err != nil {
				return generationTask{}, false, false, err
			}
		}
	}
	task.Status = terminalStatus
	task.TaskStatus = terminalTaskStatus
	if task.BillingStatus == "" {
		task.BillingStatus = billingStatusBillingFailed
	}
	task.Progress = 100
	task.Error = map[string]any{"message": message}
	task.FailureReason = message
	task.UpdatedAt = now
	task.WorkerFinishedAt = now
	if err := insertGenerationTask(ctx, tx, task); err != nil {
		return generationTask{}, false, false, err
	}
	return task, refunded, true, nil
}

func generatedAssetForRequest(req createGenerationTaskRequest, userID string, taskID string, assetID string, index int, now string) asset {
	referenceCount := 0
	referenceImages := req.Params["referenceImages"]
	inputImageIds := req.Params["inputImageIds"]
	inputImagesSnapshot := req.Params["inputImagesSnapshot"]
	maskDraft := req.Params["maskDraft"]
	maskTargetImageId := req.Params["maskTargetImageId"]
	maskImageId := req.Params["maskImageId"]
	if items, ok := referenceImages.([]any); ok {
		referenceCount = len(items)
	}
	imageURL := promptPreviewImage(req.Prompt)
	contentType := "image/svg+xml"
	source := "local-prompt-preview"
	mediaType := "image"
	width := previewImageWidth
	height := previewImageHeight
	thumbnailURL := imageURL
	if index < len(req.GeneratedImages) && req.GeneratedImages[index].URL != "" {
		imageURL = req.GeneratedImages[index].URL
		thumbnailURL = req.GeneratedImages[index].ThumbnailURL
		contentType = req.GeneratedImages[index].ContentType
		source = req.GeneratedImages[index].Source
		width = req.GeneratedImages[index].Width
		height = req.GeneratedImages[index].Height
		if contentType == "" {
			contentType = "image/png"
		}
		if source == "" {
			source = "model-provider"
		}
		if width <= 0 || height <= 0 {
			width = previewImageWidth
			height = previewImageHeight
		}
	}
	if thumbnailURL == "" {
		thumbnailURL = imageURL
	}
	if isVideoGenerationType(req.Type) {
		if videoURL := providerTaskString(req, "videoUrl"); videoURL != "" {
			imageURL = videoURL
			mediaType = "video"
			contentType = "video/mp4"
			source = firstNonEmptyString(providerTaskString(req, "provider"), "video-provider")
			thumbnailURL = firstNonEmptyString(providerTaskString(req, "thumbnailUrl"), thumbnailURL)
		}
	}
	item := asset{
		ID:             assetID,
		UserID:         userID,
		TenantID:       stringValue(req.Params["tenant_id"]),
		OrganizationID: stringValue(req.Params["organization_id"]),
		TaskID:         taskID,
		Name:           generationAssetName(req.Type, taskID, index),
		MediaType:      mediaType,
		URL:            imageURL,
		ThumbnailURL:   thumbnailURL,
		Favorite:       false,
		Metadata: map[string]any{
			"prompt":                req.Prompt,
			"model":                 req.Model,
			"type":                  req.Type,
			"module_code":           requestModuleCode(req),
			"billing_type":          stringValue(req.Params["billing_type"]),
			"sourceType":            req.Type,
			"contentType":           contentType,
			"source":                source,
			"providerTaskId":        stringValue(req.Params["provider_task_id"]),
			"providerRevisedPrompt": stringValue(req.Params["provider_revised_prompt"]),
			"thumbnailUrl":          thumbnailURL,
			"width":                 width,
			"height":                height,
			"resolution":            fmt.Sprintf("%dx%d", width, height),
			"index":                 index + 1,
			"referenceCount":        referenceCount,
			"referenceImages":       referenceImages,
			"inputImageIds":         inputImageIds,
			"inputImagesSnapshot":   inputImagesSnapshot,
			"maskDraft":             maskDraft,
			"maskTargetImageId":     maskTargetImageId,
			"maskImageId":           maskImageId,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if stored, ok := generatedStorageRecord(req.Params, index); ok {
		item.Metadata["fileId"] = stringValue(stored["fileId"])
		item.Metadata["storageFileId"] = stringValue(stored["fileId"])
		item.Metadata["storageTenantId"] = stringValue(stored["tenantId"])
		item.Metadata["storageProvider"] = stringValue(stored["provider"])
		item.Metadata["storageBucket"] = stringValue(stored["bucket"])
		item.Metadata["storageObjectKey"] = stringValue(stored["objectKey"])
		item.Metadata["fileSize"] = int64Value(stored["fileSize"])
		item.Metadata["fileSizeBytes"] = int64Value(stored["fileSize"])
		if sourceURL := compactPersistedSourceURL(stringValue(stored["sourceUrl"])); sourceURL != "" {
			item.Metadata["sourceUrl"] = sourceURL
		}
		item.Metadata["storageManaged"] = true
		if item.Metadata["fileId"] != "" && strings.HasPrefix(strings.ToLower(strings.TrimSpace(item.URL)), "data:") {
			// The managed file is the durable original. Do not duplicate the same
			// multi-megabyte Data URL in the asset row.
			originalURL := item.URL
			item.URL = ""
			if item.ThumbnailURL == originalURL {
				item.ThumbnailURL = ""
				delete(item.Metadata, "thumbnailUrl")
			}
		}
		if storedContentType := stringValue(stored["contentType"]); storedContentType != "" {
			item.Metadata["contentType"] = storedContentType
		}
	}
	copyGenerationComplianceMetadata(item.Metadata, req.Params, assetID, now)
	return item
}

func (s *postgresStore) CreateAdminOrder(req adminOrderMutation) (adminOrder, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminOrder{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return adminOrder{}, err
	}
	defer func() { _ = tx.Rollback() }()
	managedV2, err := legacyOrderManagedV2Tx(ctx, tx, req.PlanID, req.PaymentEnvironment)
	if err != nil {
		return adminOrder{}, err
	}
	if managedV2 {
		return adminOrder{}, newBusinessPlanAdminError(http.StatusConflict, "MANAGED_PLAN_REQUIRES_PRICE_QUOTE", "V2 managed member and agent plans must be ordered with a server-issued price quote")
	}
	id, err := nextTableID(ctx, tx, "xz_orders", "order")
	if err != nil {
		return adminOrder{}, err
	}
	tenantID, err := currentEnterpriseTenantForOrderTx(ctx, tx, req.UserID)
	if err != nil {
		return adminOrder{}, err
	}
	priceSnapshot := orderPriceSnapshot(req)
	if tenantID != "" {
		priceSnapshot["tenantId"] = tenantID
	}
	item := adminOrder{ID: id, TenantID: tenantID, UserID: req.UserID, BuyerUserID: req.UserID, PlanID: req.PlanID, BusinessOrderType: businessOrderTypeForPlanID(req.PlanID), Amount: req.AmountCents, AmountCents: req.AmountCents, Status: fallback(req.Status, "PENDING"), PriceSnapshot: priceSnapshot, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if err := insertOrder(ctx, tx, item); err != nil {
		return adminOrder{}, err
	}
	if err := insertAuditLog(ctx, tx, "", "", "orders.create", "order", item.ID, "", "", 200, map[string]any{"userId": item.UserID, "planId": item.PlanID}); err != nil {
		return adminOrder{}, err
	}
	return item, tx.Commit()
}

func (s *postgresStore) RegisterPaymentCallbackEvent(event adminPaymentEvent) (bool, error) {
	event = normalizePaymentCallbackEvent(event)
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	_ = tx.QueryRowContext(ctx, `SELECT coalesce(tenant_id,'') FROM xz_orders WHERE id=$1`, event.OrderID).Scan(&event.TenantID)
	existing, found, err := paymentCallbackEventByEventIDForTx(ctx, tx, event.Provider, event.EventID)
	if err != nil {
		return false, err
	}
	if found {
		if samePaymentCallbackEventTarget(existing, event) {
			return true, tx.Commit()
		}
		return false, fmt.Errorf("payment event already belongs to another order: %s", event.EventID)
	}
	if event.TransactionID != "" {
		existing, found, err = paymentCallbackEventByTransactionIDForTx(ctx, tx, event.Provider, event.TransactionID)
		if err != nil {
			return false, err
		}
		if found {
			if samePaymentCallbackEventTarget(existing, event) {
				return true, tx.Commit()
			}
			return false, fmt.Errorf("payment transaction already belongs to another order: %s", event.TransactionID)
		}
	}
	inserted, err := insertPaymentCallbackEvent(ctx, tx, event)
	if err != nil {
		return false, err
	}
	if !inserted {
		existing, found, err = paymentCallbackEventByEventIDForTx(ctx, tx, event.Provider, event.EventID)
		if err != nil {
			return false, err
		}
		if found && samePaymentCallbackEventTarget(existing, event) {
			return true, tx.Commit()
		}
		if event.TransactionID != "" {
			existing, found, err = paymentCallbackEventByTransactionIDForTx(ctx, tx, event.Provider, event.TransactionID)
			if err != nil {
				return false, err
			}
			if found && samePaymentCallbackEventTarget(existing, event) {
				return true, tx.Commit()
			}
		}
		return false, errors.New("payment callback idempotency key already belongs to another event")
	}
	return false, tx.Commit()
}

func (s *postgresStore) MarkAdminOrderPaid(id string, metadata ...map[string]any) (adminOrder, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return adminOrder{}, err
	}
	defer func() { _ = tx.Rollback() }()
	item, err := getOrderForUpdate(ctx, tx, id)
	if err != nil {
		return adminOrder{}, err
	}
	mergeOrderPaymentMetadata(&item, metadata...)
	if strings.EqualFold(item.Status, "PAID") {
		if err := applyCommerceOrderFulfillmentForTx(ctx, tx, s.db, &item); err != nil {
			return adminOrder{}, err
		}
		if err := insertOrder(ctx, tx, item); err != nil {
			return adminOrder{}, err
		}
		if err := markPaymentEventsProcessedTx(ctx, tx, item.ID, metadata...); err != nil {
			return adminOrder{}, err
		}
		return item, tx.Commit()
	}
	item.Status = "PAID"
	item.PaidAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := applyCommerceOrderFulfillmentForTx(ctx, tx, s.db, &item); err != nil {
		return adminOrder{}, err
	}
	if isRechargeOrder(item) {
		if err := s.syncRechargeNewAPIQuotaTx(ctx, tx, &item); err != nil {
			return adminOrder{}, err
		}
	}
	if err := insertOrder(ctx, tx, item); err != nil {
		return adminOrder{}, err
	}
	if err := markPaymentEventsProcessedTx(ctx, tx, item.ID, metadata...); err != nil {
		return adminOrder{}, err
	}
	if err := insertAuditLog(ctx, tx, "", "", "orders.mark_paid", "order", item.ID, "", "", 200, nil); err != nil {
		return adminOrder{}, err
	}
	return item, tx.Commit()
}

func markPaymentEventsProcessedTx(ctx context.Context, tx *sql.Tx, orderID string, metadata ...map[string]any) error {
	eventID := ""
	for _, item := range metadata {
		if value := strings.TrimSpace(stringValue(item["eventId"])); value != "" {
			eventID = value
			break
		}
	}
	if eventID == "" {
		return nil
	}
	_, err := tx.ExecContext(ctx, `
		UPDATE xz_payment_events
		SET status='PROCESSED',processed_at=coalesce(processed_at,now())
		WHERE order_id=$1 AND event_id=$2
	`, orderID, eventID)
	return err
}

func (s *postgresStore) RequestOrderRefund(userID string, orderID string, reason string, remark string) (adminOrder, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return adminOrder{}, err
	}
	defer func() { _ = tx.Rollback() }()
	item, err := getOrderForUpdate(ctx, tx, orderID)
	if err != nil {
		return adminOrder{}, err
	}
	if item.UserID != userID && item.BuyerUserID != userID {
		return adminOrder{}, errors.New("order does not belong to current user")
	}
	if strings.EqualFold(item.Status, "REFUND_REQUESTED") {
		return item, tx.Commit()
	}
	if !isPaidStatus(item.Status) && strings.TrimSpace(item.PaidAt) == "" {
		return adminOrder{}, errors.New("only paid orders can request a refund")
	}
	if item.PriceSnapshot == nil {
		item.PriceSnapshot = map[string]any{}
	}
	item.PriceSnapshot["refundReason"] = strings.TrimSpace(reason)
	item.PriceSnapshot["refundRemark"] = strings.TrimSpace(remark)
	item.PriceSnapshot["refundRequestedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
	item.Status = "REFUND_REQUESTED"
	if err := insertOrder(ctx, tx, item); err != nil {
		return adminOrder{}, err
	}
	if err := insertAuditLog(ctx, tx, userID, "MEMBER", "orders.refund_requested", "order", item.ID, "", "", 200, map[string]any{"reason": reason}); err != nil {
		return adminOrder{}, err
	}
	return item, tx.Commit()
}

func ensurePaidRechargeRouteTx(ctx context.Context, tx *sql.Tx, order *adminOrder) error {
	if order == nil || !isRechargeOrder(*order) {
		return nil
	}
	if order.PriceSnapshot != nil && strings.EqualFold(strings.TrimSpace(fmt.Sprint(order.PriceSnapshot["newapiSyncStatus"])), "READY") {
		return nil
	}
	now := order.PaidAt
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339Nano)
	}
	points := rechargePointsForOrder(*order)
	route, err := ensureRechargeImageBackupRouteTx(ctx, tx, order.UserID, points, now)
	if err != nil {
		return err
	}
	if route.ID == "" {
		return nil
	}
	if order.PriceSnapshot == nil {
		order.PriceSnapshot = map[string]any{}
	}
	order.PriceSnapshot["orderType"] = "COMPUTE_RECHARGE"
	order.PriceSnapshot["rechargePoints"] = points
	order.PriceSnapshot["newapiSyncStatus"] = "READY"
	order.PriceSnapshot["newapiSyncAmountCents"] = orderAmount(*order)
	order.PriceSnapshot["modelRouteId"] = route.ID
	order.PriceSnapshot["newapiGroup"] = route.GroupName
	order.PriceSnapshot["newapiKeyId"] = route.APIKeyID
	return nil
}

func applyRechargeSettlementForTx(ctx context.Context, tx *sql.Tx, order *adminOrder) error {
	if order == nil || !isRechargeOrder(*order) {
		return nil
	}
	points := rechargePointsForOrder(*order)
	if points <= 0 {
		return nil
	}
	now := order.PaidAt
	if now == "" {
		now = time.Now().UTC().Format(time.RFC3339Nano)
	}
	exists, err := billingEventExistsTx(ctx, tx, order.ID, "compute.recharge")
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	if order.PriceSnapshot == nil {
		order.PriceSnapshot = map[string]any{}
	}
	order.PriceSnapshot["orderType"] = "COMPUTE_RECHARGE"
	order.PriceSnapshot["rechargePoints"] = points
	order.PriceSnapshot["newapiSyncAmountCents"] = orderAmount(*order)
	route, err := ensureRechargeImageBackupRouteTx(ctx, tx, order.UserID, points, now)
	if err != nil {
		return err
	}
	if route.ID != "" {
		order.PriceSnapshot["newapiSyncStatus"] = "READY"
		order.PriceSnapshot["modelRouteId"] = route.ID
		order.PriceSnapshot["newapiGroup"] = route.GroupName
		order.PriceSnapshot["newapiKeyId"] = route.APIKeyID
	} else {
		order.PriceSnapshot["newapiSyncStatus"] = "PENDING"
	}
	grant, err := grantPermanentPersonalPointsTx(ctx, tx, order.UserID, PointSourceRecharge, points,
		"RECHARGE_ORDER", order.ID, "recharge:"+order.ID, pointGrantedAt(now))
	if err != nil {
		return err
	}
	account, before := grant.Account, grant.AvailableBefore
	if route.ID != "" && account.Available > route.QuotaLimit {
		route, err = ensureRechargeImageBackupRouteTx(ctx, tx, order.UserID, account.Available, now)
		if err != nil {
			return err
		}
		order.PriceSnapshot["modelRouteId"] = route.ID
		order.PriceSnapshot["newapiGroup"] = route.GroupName
		order.PriceSnapshot["newapiKeyId"] = route.APIKeyID
	}
	agent, hasAgent, err := directActiveAgentForUserTx(ctx, tx, order.UserID)
	if err != nil {
		return err
	}
	agentID := ""
	if hasAgent {
		agentID = agent.ID
	}
	eventID, err := nextTableID(ctx, tx, "xz_billing_events", "evt")
	if err != nil {
		return err
	}
	event := adminBillingEvent{
		ID:              eventID,
		TransactionID:   "txn_" + shortID(order.ID),
		UserID:          order.UserID,
		AgentID:         agentID,
		TaskID:          order.ID,
		MetricCode:      "compute.recharge",
		Quantity:        points,
		UnitAmountCents: 10,
		AmountCents:     orderAmount(*order),
		PointCost:       -points,
		BalanceBefore:   before,
		BalanceAfter:    account.Available,
		Model:           "recharge",
		Status:          "SUCCEEDED",
		OccurredAt:      now,
		Metadata: map[string]any{
			"source":        "order_recharge",
			"orderId":       order.ID,
			"newapiSync":    order.PriceSnapshot["newapiSyncStatus"],
			"newapiGroup":   order.PriceSnapshot["newapiGroup"],
			"modelRouteId":  order.PriceSnapshot["modelRouteId"],
			"rechargeCents": orderAmount(*order),
		},
	}
	if err := insertBillingEvent(ctx, tx, event); err != nil {
		return err
	}
	commissions, err := commissionArtifactsForUserTx(ctx, tx, order.UserID, order.ID, "COMPUTE_RECHARGE", "compute_recharge", orderAmount(*order), now)
	if err != nil {
		return err
	}
	for _, commission := range commissions {
		if err := insertCommission(ctx, tx, commission); err != nil {
			return err
		}
	}
	return nil
}

func applyCommerceOrderFulfillmentForTx(ctx context.Context, tx *sql.Tx, db *sql.DB, order *adminOrder) error {
	if order == nil {
		return nil
	}
	if order.FulfillmentStatus == "FULFILLED" || stringValue(order.PriceSnapshot["fulfillmentStatus"]) == "FULFILLED" {
		if isRechargeOrder(*order) {
			return ensurePaidRechargeRouteTx(ctx, tx, order)
		}
		return nil
	}
	if isRechargeOrder(*order) {
		if err := applyRechargeSettlementForTx(ctx, tx, order); err != nil {
			return err
		}
		markOrderFulfilled(order, "COMPUTE_RECHARGE", nowForOrder(*order))
		return nil
	}
	plan, ok := planCatalogByID(order.PlanID)
	if !ok {
		return nil
	}
	return applyCommerceOrderFulfillmentWithPlanForTx(ctx, tx, db, order, plan)
}

func applyCommerceOrderFulfillmentWithPlanForTx(ctx context.Context, tx *sql.Tx, db *sql.DB, order *adminOrder, plan adminPlan) error {
	planType := planBusinessType(plan)
	switch planType {
	case planTypeMemberPackage, planTypeAgentJoinPackage, planTypeOperationCenterPackage:
	default:
		return nil
	}
	if planType == planTypeOperationCenterPackage {
		return applyOperationCenterPaymentSucceededForTx(ctx, tx, db, order)
	}
	commerceCtx, err := commerceContextForOrderTx(ctx, tx, *order, plan)
	if err != nil {
		return err
	}
	settlementDecision, err := resolveOrderSettlementDecisionTx(ctx, tx, order, plan)
	if err != nil {
		return err
	}
	if err := claimSettlementWriteSourceTx(ctx, tx, settlementDecision); err != nil {
		return err
	}
	var engineResult commissionapp.CalculationResult
	if settlementDecision.SettlementEngine == settlementEngineV132 {
		engineResult, err = generateV132CommissionRecordsForCommerceOrderTx(ctx, tx, order, plan, commerceCtx, settlementDecision)
	} else {
		engineResult, err = generateCommissionRecordsForCommerceOrderTx(ctx, tx, *order, plan, commerceCtx)
	}
	if err != nil {
		return err
	}
	result, err := compatibilitySettlementResult(commerceCtx, engineResult)
	if err != nil {
		return err
	}
	shadowLegacyResult := result
	if settlementDecision.SettlementEngine == settlementEngineV132 {
		if preview, previewErr := calculateCommissionSettlement(commerceCtx); previewErr == nil {
			shadowLegacyResult = preview
		}
	}
	recordCommerceShadowDifferenceTx(ctx, tx, *order, plan, commerceCtx, shadowLegacyResult)
	applySettlementToOrder(order, commerceCtx, result, planType)
	if result.TokenGrantAmount > 0 {
		if err := grantTokensToUserTx(ctx, tx, order.UserID, order.ID, tokenChangeTypeForPlan(planType), result.TokenGrantAmount, result.TokenGrantValueCents, nowForOrder(*order)); err != nil {
			return err
		}
	}
	if shouldWriteLegacyCommissionProjection(*order) {
		for _, commission := range settlementCommissionRecords(commerceCtx, result, nowForOrder(*order)) {
			if err := insertCommission(ctx, tx, commission); err != nil {
				return err
			}
		}
	}
	if err := fulfillIdentityForOrderTx(ctx, tx, order, plan, result, nowForOrder(*order)); err != nil {
		return err
	}
	markOrderFulfilled(order, result.OrderType, nowForOrder(*order))
	return nil
}

func applyOperationCenterPaymentSucceededForTx(ctx context.Context, tx *sql.Tx, db *sql.DB, order *adminOrder) error {
	if tx == nil || db == nil || order == nil {
		return operationcenter.ErrTransactionRequired
	}
	if order.PriceSnapshot == nil {
		order.PriceSnapshot = map[string]any{}
	}
	paymentRecordID, err := operationCenterPaymentRecordIDForTx(ctx, tx, order)
	if err != nil {
		return err
	}
	paidAt := nowForOrder(*order)
	providerTransactionID := firstOperationCenterCallbackValue(order.PriceSnapshot, "providerTransactionId", "transactionId")
	if _, err := tx.ExecContext(ctx, `
		UPDATE xz_payment_records
		SET payment_status='SUCCESS',
		    paid_at=coalesce(paid_at,$3::timestamptz),
		    provider_transaction_id=coalesce(nullif(provider_transaction_id,''),nullif($4,''))
		WHERE id=$1 AND order_id=$2 AND amount_cents=$5
		  AND upper(coalesce(payment_status,prepay_status,'')) IN ('PENDING','SIGNED','SUCCESS','PAID')
	`, paymentRecordID, order.ID, paidAt, providerTransactionID, orderAmount(*order)); err != nil {
		return err
	}
	order.Status = "PAID"
	order.OrderType = "OPERATION_CENTER_SERVICE"
	order.BusinessOrderType = "OPERATION_CENTER_SERVICE"
	order.FulfillmentStatus = string(operationcenter.OperationCenterServiceReviewRequired)
	order.FulfilledAt = ""
	order.PriceSnapshot["orderType"] = order.OrderType
	order.PriceSnapshot["businessOrderType"] = order.BusinessOrderType
	order.PriceSnapshot["fulfillmentStatus"] = order.FulfillmentStatus
	delete(order.PriceSnapshot, "fulfilledAt")
	priceSnapshot, err := json.Marshal(order.PriceSnapshot)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE xz_orders
		SET status='PAID',order_status='PAID',paid_at=$2::timestamptz,
		    fulfillment_status=$3,fulfilled_at=NULL,order_type=$4,business_order_type=$5,
		    price_snapshot=$6::jsonb,updated_at=clock_timestamp()
		WHERE id=$1
	`, order.ID, paidAt, order.FulfillmentStatus, order.OrderType, order.BusinessOrderType, string(priceSnapshot)); err != nil {
		return err
	}
	workflow, err := operationcenter.NewWorkflowService(db, operationcenter.WorkflowOptions{})
	if err != nil {
		return err
	}
	result, err := workflow.RecordPaymentSucceededForTx(ctx, tx, operationcenter.PaymentSucceededCommand{
		OrderID: order.ID, PaymentRecordID: paymentRecordID,
	})
	if err != nil {
		return err
	}
	order.PriceSnapshot["operationCenterServiceOrderId"] = result.ServiceOrder.ID
	order.PriceSnapshot["operationCenterServiceStatus"] = string(result.ServiceOrder.Status)
	return nil
}

func operationCenterPaymentRecordIDForTx(ctx context.Context, tx *sql.Tx, order *adminOrder) (string, error) {
	var paymentRecordID string
	err := tx.QueryRowContext(ctx, `
		SELECT coalesce(metadata->>'paymentRecordId','')
		FROM xz_operation_center_service_orders
		WHERE order_id=$1
		FOR UPDATE
	`, order.ID).Scan(&paymentRecordID)
	if err == nil && strings.TrimSpace(paymentRecordID) != "" {
		return paymentRecordID, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	err = tx.QueryRowContext(ctx, `
		SELECT id
		FROM xz_payment_records
		WHERE order_id=$1 AND amount_cents=$2
		ORDER BY CASE WHEN upper(coalesce(payment_status,prepay_status,'')) IN ('SUCCESS','PAID') THEN 0 ELSE 1 END,
		         created_at,id
		LIMIT 1
		FOR UPDATE
	`, order.ID, orderAmount(*order)).Scan(&paymentRecordID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", operationcenter.ErrPaymentNotSuccessful
	}
	return paymentRecordID, err
}

func firstOperationCenterCallbackValue(snapshot map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(stringValue(snapshot[key])); value != "" {
			return value
		}
	}
	return ""
}

func commerceContextForOrderTx(ctx context.Context, tx *sql.Tx, order adminOrder, plan adminPlan) (commissionOrderContext, error) {
	if order.CommissionSnapshotCaptured {
		return commissionOrderContext{
			OrderID: order.ID, OrderType: orderTypeForCommerceOrder(planBusinessType(plan), order.DirectAgentID != "", order.ParentAgentID),
			PlanType: planBusinessType(plan), AmountCents: orderAmount(order), BuyerUserID: order.UserID,
			DirectAgentID: order.DirectAgentID, ParentAgentID: order.ParentAgentID, OperationCenterID: order.OperationCenterID,
			TokenGrantAmount: planTokenGrantAmount(plan), TokenGrantValueCents: planTokenRightsValueCents(plan),
		}, nil
	}
	direct, hasDirect, err := directActiveAgentForUserTx(ctx, tx, order.UserID)
	if err != nil {
		return commissionOrderContext{}, err
	}
	parentID := ""
	operationCenterID := ""
	if hasDirect {
		parentID = direct.ParentID
		operationCenterID = direct.OperationCenterID
		if operationCenterID == "" && parentID != "" {
			parent, ok, err := channelAgentByIDForTx(ctx, tx, parentID)
			if err != nil {
				return commissionOrderContext{}, err
			}
			if ok {
				operationCenterID = parent.OperationCenterID
			}
		}
	}
	directID := ""
	if hasDirect {
		directID = direct.ID
	}
	return commissionOrderContext{
		OrderID:              order.ID,
		OrderType:            orderTypeForCommerceOrder(planBusinessType(plan), hasDirect, parentID),
		PlanType:             planBusinessType(plan),
		AmountCents:          orderAmount(order),
		BuyerUserID:          order.UserID,
		DirectAgentID:        directID,
		ParentAgentID:        parentID,
		OperationCenterID:    operationCenterID,
		TokenGrantAmount:     planTokenGrantAmount(plan),
		TokenGrantValueCents: planTokenRightsValueCents(plan),
	}, nil
}

func applySettlementToOrder(order *adminOrder, ctx commissionOrderContext, result commissionSettlementResult, planType string) {
	if order.BuyerUserID == "" {
		order.BuyerUserID = firstNonEmptyString(ctx.BuyerUserID, order.UserID)
	}
	order.OrderType = result.OrderType
	order.BusinessOrderType = businessOrderTypeForPlanType(planType)
	order.DirectAgentID = ctx.DirectAgentID
	order.ParentAgentID = ctx.ParentAgentID
	order.OperationCenterID = ctx.OperationCenterID
	order.TokenGrantAmount = result.TokenGrantAmount
	order.TokenAmount = result.TokenGrantAmount
	order.PlatformIncomeCents = result.PlatformIncomeCents
	order.RewardSnapshot = rewardSnapshotForSettlement(ctx, result, planType)
	if order.PriceSnapshot == nil {
		order.PriceSnapshot = map[string]any{}
	}
	order.PriceSnapshot["buyerUserId"] = order.BuyerUserID
	order.PriceSnapshot["orderType"] = result.OrderType
	order.PriceSnapshot["businessOrderType"] = businessOrderTypeForPlanType(planType)
	order.PriceSnapshot["planType"] = planType
	order.PriceSnapshot["directAgentId"] = ctx.DirectAgentID
	order.PriceSnapshot["parentAgentId"] = ctx.ParentAgentID
	order.PriceSnapshot["operationCenterId"] = ctx.OperationCenterID
	order.PriceSnapshot["tokenGrantAmount"] = result.TokenGrantAmount
	order.PriceSnapshot["tokenAmount"] = result.TokenGrantAmount
	order.PriceSnapshot["tokenGrantValueCents"] = result.TokenGrantValueCents
	order.PriceSnapshot["platformIncomeCents"] = result.PlatformIncomeCents
	order.PriceSnapshot["rewardSnapshot"] = order.RewardSnapshot
}

func markOrderFulfilled(order *adminOrder, orderType string, now string) {
	if order.PriceSnapshot == nil {
		order.PriceSnapshot = map[string]any{}
	}
	order.FulfillmentStatus = "FULFILLED"
	order.FulfilledAt = now
	if order.OrderType == "" {
		order.OrderType = orderType
	}
	order.PriceSnapshot["orderType"] = order.OrderType
	order.PriceSnapshot["fulfillmentStatus"] = "FULFILLED"
	order.PriceSnapshot["fulfilledAt"] = now
}

func nowForOrder(order adminOrder) string {
	if order.PaidAt != "" {
		return order.PaidAt
	}
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func grantTokensToUserTx(ctx context.Context, tx *sql.Tx, userID string, orderID string, changeType string, amount int, valueCents int, now string) error {
	exists, err := tokenRecordExistsTx(ctx, tx, orderID, changeType)
	if err != nil || exists {
		return err
	}
	grant, err := grantPermanentPersonalPointsTx(ctx, tx, userID, paidPointSourceForChangeType(changeType), amount,
		"COMMERCE_ORDER", orderID, "commerce:"+orderID+":"+strings.ToUpper(changeType), pointGrantedAt(now))
	if err != nil {
		return err
	}
	account, before := grant.Account, grant.AvailableBefore
	record := adminTokenRecord{
		ID:           "token_" + shortID(orderID+"_"+changeType),
		UserID:       userID,
		OrderID:      orderID,
		ChangeType:   changeType,
		Amount:       amount,
		BalanceAfter: account.Available,
		Remark:       "commerce_order_grant",
		CreatedAt:    now,
	}
	if err := insertTokenRecord(ctx, tx, record); err != nil {
		return err
	}
	eventExists, err := billingEventExistsTx(ctx, tx, orderID, "commerce.token_grant")
	if err != nil || eventExists {
		return err
	}
	return insertBillingEvent(ctx, tx, adminBillingEvent{
		ID:              "evt_" + shortID(orderID+"_token_grant"),
		TransactionID:   "txn_" + shortID(orderID+"_token_grant"),
		UserID:          userID,
		TaskID:          orderID,
		MetricCode:      "commerce.token_grant",
		Quantity:        amount,
		UnitAmountCents: 0,
		AmountCents:     valueCents,
		PointCost:       -amount,
		BalanceBefore:   before,
		BalanceAfter:    account.Available,
		Model:           "commerce",
		Status:          "SUCCEEDED",
		OccurredAt:      now,
		Metadata: map[string]any{
			"source":     "commerce_order",
			"orderId":    orderID,
			"changeType": changeType,
			"valueCents": valueCents,
		},
	})
}

func fulfillIdentityForOrderTx(ctx context.Context, tx *sql.Tx, order *adminOrder, plan adminPlan, result commissionSettlementResult, now string) error {
	user, err := userByIDForUpdateTx(ctx, tx, order.UserID)
	if err != nil {
		return err
	}
	switch planBusinessType(plan) {
	case planTypeMemberPackage:
		user.PlanID = order.PlanID
		user.MemberLevel = planMemberLevel(plan)
		if user.AgentStatus == "" {
			user.AgentStatus = agentStatusNone
		}
		if user.OperationCenterStatus == "" {
			user.OperationCenterStatus = operationStatusNone
		}
		if plan.DurationDays > 0 {
			paidAt := time.Now().UTC()
			if parsed, parseErr := time.Parse(time.RFC3339Nano, now); parseErr == nil {
				paidAt = parsed.UTC()
			}
			_, expiresAt := membershipExtensionWindow(user.SubscriptionExpiresAt, paidAt, int64(plan.DurationDays))
			user.SubscriptionExpiresAt = expiresAt.Format(time.RFC3339Nano)
		}
	case planTypeAgentJoinPackage:
		return fulfillCommercialIdentityForOrderTx(ctx, tx, order, plan, result, now)
	case planTypeOperationCenterPackage:
		return fulfillCommercialIdentityForOrderTx(ctx, tx, order, plan, result, now)
	}
	user.UpdatedAt = now
	return insertUser(ctx, tx, user)
}

func (s *postgresStore) syncRechargeNewAPIQuotaTx(ctx context.Context, tx *sql.Tx, order *adminOrder) error {
	if order == nil || !isRechargeOrder(*order) {
		return nil
	}
	points := rechargePointsForOrder(*order)
	if points <= 0 {
		return nil
	}
	if order.PriceSnapshot == nil {
		order.PriceSnapshot = map[string]any{}
	}
	settings, err := s.getSystemSettings(ctx)
	if err != nil {
		order.PriceSnapshot["newapiSyncStatus"] = "FAILED"
		order.PriceSnapshot["newapiSyncError"] = err.Error()
		return nil
	}
	cfg := newAPISyncConfigFromSettings(settings)
	if !cfg.Enabled || strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.AdminCookie) == "" {
		if order.PriceSnapshot["newapiSyncStatus"] == nil {
			order.PriceSnapshot["newapiSyncStatus"] = "PENDING"
		}
		return nil
	}
	var user adminUser
	if err := tx.QueryRowContext(ctx, `select raw from xz_users where id = $1 for update`, order.UserID).Scan(rawScanner(&user)); err != nil {
		order.PriceSnapshot["newapiSyncStatus"] = "FAILED"
		order.PriceSnapshot["newapiSyncError"] = err.Error()
		return nil
	}
	route := primaryUserModelRoute(user)
	if route.ID == "" {
		order.PriceSnapshot["newapiSyncStatus"] = "PENDING"
		order.PriceSnapshot["newapiSyncError"] = "用户未绑定可用的模型路由"
		return nil
	}
	result, err := addNewAPIQuotaForRoute(ctx, cfg, user, route, points)
	if err != nil {
		order.PriceSnapshot["newapiSyncStatus"] = "FAILED"
		order.PriceSnapshot["newapiSyncError"] = err.Error()
		return nil
	}
	route.ExternalKey = firstNonEmptyString(result.ExternalKey, route.ExternalKey)
	route.ExternalUser = firstNonEmptyString(result.ExternalUser, route.ExternalUser)
	if newAPIUsableSecret(result.Secret) {
		var key adminAPIKey
		if err := tx.QueryRowContext(ctx, `select raw from xz_api_keys where id = $1 for update`, route.APIKeyID).Scan(rawScanner(&key)); err == nil {
			key.Secret = result.Secret
			key.Prefix = apiKeyPrefix(result.Secret, 1)
			if err := insertAPIKey(ctx, tx, key); err != nil {
				order.PriceSnapshot["newapiSyncStatus"] = "FAILED"
				order.PriceSnapshot["newapiSyncError"] = err.Error()
				return nil
			}
			route.KeyPrefix = key.Prefix
		}
	}
	replaced := false
	for i := range user.ModelRoutes {
		if user.ModelRoutes[i].ID == route.ID || strings.EqualFold(user.ModelRoutes[i].GroupName, route.GroupName) {
			user.ModelRoutes[i] = route
			replaced = true
			break
		}
	}
	if !replaced {
		user.ModelRoutes = append(user.ModelRoutes, route)
	}
	user.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := insertUser(ctx, tx, user); err != nil {
		order.PriceSnapshot["newapiSyncStatus"] = "FAILED"
		order.PriceSnapshot["newapiSyncError"] = err.Error()
		return nil
	}
	order.PriceSnapshot["newapiSyncStatus"] = "SYNCED"
	order.PriceSnapshot["newapiExternalKey"] = route.ExternalKey
	order.PriceSnapshot["newapiExternalUser"] = route.ExternalUser
	if result.Created {
		order.PriceSnapshot["newapiMode"] = "created"
	} else if result.Updated {
		order.PriceSnapshot["newapiMode"] = "updated"
	}
	return nil
}

func (s *postgresStore) RenewAdminOrder(id string) (adminOrder, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return adminOrder{}, err
	}
	defer func() { _ = tx.Rollback() }()
	source, err := getOrderForUpdate(ctx, tx, id)
	if err != nil {
		return adminOrder{}, err
	}
	managedV2, err := legacyOrderManagedV2Tx(ctx, tx, source.PlanID, "")
	if err != nil {
		return adminOrder{}, err
	}
	if managedV2 {
		return adminOrder{}, newBusinessPlanAdminError(http.StatusConflict, "MANAGED_PLAN_REQUIRES_PRICE_QUOTE", "V2 managed member and agent plans cannot renew a legacy order without a new price quote")
	}
	nextID, err := nextTableID(ctx, tx, "xz_orders", "order")
	if err != nil {
		return adminOrder{}, err
	}
	item := adminOrder{ID: nextID, TenantID: source.TenantID, UserID: source.UserID, PlanID: source.PlanID, Amount: orderAmount(source), AmountCents: orderAmount(source), Status: "PENDING", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), PriceSnapshot: map[string]any{"renewOf": source.ID, "tenantId": source.TenantID}}
	if err := insertOrder(ctx, tx, item); err != nil {
		return adminOrder{}, err
	}
	if err := insertAuditLog(ctx, tx, "", "", "orders.renew", "order", item.ID, "", "", 200, map[string]any{"renewOf": source.ID}); err != nil {
		return adminOrder{}, err
	}
	return item, tx.Commit()
}

func (s *postgresStore) CreateAdminCommission(req adminCommissionMutation) (adminCommission, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return adminCommission{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if req.OrderID == "" || req.AgentID == "" || req.AmountCents <= 0 {
		return adminCommission{}, errors.New("orderId, agentId and positive amountCents are required")
	}
	id, err := nextTableID(ctx, tx, "xz_commissions", "commission")
	if err != nil {
		return adminCommission{}, err
	}
	item := adminCommission{ID: id, OrderID: req.OrderID, AgentID: req.AgentID, AmountCents: req.AmountCents, Rate: req.Rate, Status: fallback(req.Status, "PENDING"), RuleSnapshot: map[string]any{"source": "manual", "rate": req.Rate}, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if err := insertCommission(ctx, tx, item); err != nil {
		return adminCommission{}, err
	}
	if err := insertAuditLog(ctx, tx, "", "", "commissions.create", "commission", item.ID, "", "", 200, map[string]any{"orderId": item.OrderID, "agentId": item.AgentID}); err != nil {
		return adminCommission{}, err
	}
	return item, tx.Commit()
}

func (s *postgresStore) ReviewAdminCommission(id string, status string) (adminCommission, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return adminCommission{}, err
	}
	defer func() { _ = tx.Rollback() }()
	status = strings.ToUpper(strings.TrimSpace(status))
	if status != "APPROVED" && status != "REJECTED" {
		return adminCommission{}, fmt.Errorf("invalid commission status: %s", status)
	}
	var item adminCommission
	err = tx.QueryRowContext(ctx, `select raw from xz_commissions where id = $1 for update`, id).Scan(rawScanner(&item))
	if err != nil {
		return adminCommission{}, err
	}
	item.Status = status
	if item.RuleSnapshot == nil {
		item.RuleSnapshot = map[string]any{}
	}
	item.RuleSnapshot["reviewedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
	if err := insertCommission(ctx, tx, item); err != nil {
		return adminCommission{}, err
	}
	if err := insertAuditLog(ctx, tx, "", "", "commissions.review", "commission", item.ID, "", "", 200, map[string]any{"status": status}); err != nil {
		return adminCommission{}, err
	}
	return item, tx.Commit()
}

func (s *postgresStore) CreateAdminWithdrawal(req adminWithdrawalMutation) (adminWithdrawal, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return adminWithdrawal{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if req.AgentID == "" || req.AmountCents <= 0 {
		return adminWithdrawal{}, errors.New("agentId and positive amountCents are required")
	}
	available, err := availableWithdrawalCentsForTx(ctx, tx, req.AgentID)
	if err != nil {
		return adminWithdrawal{}, err
	}
	if req.AmountCents > available {
		return adminWithdrawal{}, fmt.Errorf("可提现余额不足：当前可提现 %s，申请提现 %s", moneyText(available), moneyText(req.AmountCents))
	}
	id, err := nextTableID(ctx, tx, "xz_withdrawals", "withdrawal")
	if err != nil {
		return adminWithdrawal{}, err
	}
	item := adminWithdrawal{ID: id, AgentID: req.AgentID, AmountCents: req.AmountCents, Status: "PENDING", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if err := insertWithdrawal(ctx, tx, item); err != nil {
		return adminWithdrawal{}, err
	}
	if err := insertAuditLog(ctx, tx, "", "", "withdrawals.create", "withdrawal", item.ID, "", "", 200, map[string]any{"agentId": item.AgentID}); err != nil {
		return adminWithdrawal{}, err
	}
	return item, tx.Commit()
}

func availableWithdrawalCentsForTx(ctx context.Context, tx *sql.Tx, agentID string) (int, error) {
	commissionRows, err := tx.QueryContext(ctx, `select raw from xz_commissions where agent_id = $1`, agentID)
	if err != nil {
		return 0, err
	}
	commissions := []adminCommission{}
	for commissionRows.Next() {
		var item adminCommission
		if err := scanRawJSON(commissionRows, &item); err != nil {
			commissionRows.Close()
			return 0, err
		}
		commissions = append(commissions, item)
	}
	if err := commissionRows.Close(); err != nil {
		return 0, err
	}
	if err := commissionRows.Err(); err != nil {
		return 0, err
	}
	withdrawalRows, err := tx.QueryContext(ctx, `select raw from xz_withdrawals where agent_id = $1`, agentID)
	if err != nil {
		return 0, err
	}
	withdrawals := []adminWithdrawal{}
	for withdrawalRows.Next() {
		var item adminWithdrawal
		if err := scanRawJSON(withdrawalRows, &item); err != nil {
			withdrawalRows.Close()
			return 0, err
		}
		withdrawals = append(withdrawals, item)
	}
	if err := withdrawalRows.Close(); err != nil {
		return 0, err
	}
	if err := withdrawalRows.Err(); err != nil {
		return 0, err
	}
	return availableWithdrawalCents(commissions, withdrawals, agentID), nil
}

func (s *postgresStore) ReviewAdminWithdrawal(id string, status string) (adminWithdrawal, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return adminWithdrawal{}, err
	}
	defer func() { _ = tx.Rollback() }()
	status = strings.ToUpper(strings.TrimSpace(status))
	if status != "APPROVED" && status != "REJECTED" {
		return adminWithdrawal{}, fmt.Errorf("invalid withdrawal status: %s", status)
	}
	item, err := getWithdrawalForUpdate(ctx, tx, id)
	if err != nil {
		return adminWithdrawal{}, err
	}
	item.Status = status
	item.ReviewedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := insertWithdrawal(ctx, tx, item); err != nil {
		return adminWithdrawal{}, err
	}
	if err := insertAuditLog(ctx, tx, "", "", "withdrawals.review", "withdrawal", item.ID, "", "", 200, map[string]any{"status": status}); err != nil {
		return adminWithdrawal{}, err
	}
	return item, tx.Commit()
}

func (s *postgresStore) DeleteAssetForUser(userID string, id string) error {
	userID = strings.TrimSpace(userID)
	ctx, cancel := s.withTimeout()
	defer cancel()
	contextType, tenantID, _, err := s.currentTenantScopeForUser(ctx, userID)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var taskID string
	var resultIDsRaw string
	err = tx.QueryRowContext(ctx, `
		select coalesce(task_id, '') from xz_assets
		where id=$1 and user_id=$2 and deleted_at is null
		  and (($3='ENTERPRISE' and tenant_id=$4) or ($3<>'ENTERPRISE' and (tenant_id is null or tenant_id='tenant_default')))
		for update
	`, id, userID, contextType, tenantID).Scan(&taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: %s", errAssetNotFound, id)
	}
	if err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, `select result_ids from xz_generation_tasks where id = $1 for update`, taskID).Scan(&resultIDsRaw); err == nil {
		var resultIDs []string
		_ = json.Unmarshal([]byte(resultIDsRaw), &resultIDs)
		filtered := []string{}
		for _, resultID := range resultIDs {
			if resultID != id {
				filtered = append(filtered, resultID)
			}
		}
		if _, err := tx.ExecContext(ctx, `update xz_generation_tasks set result_ids = $1::jsonb, raw = jsonb_set(raw, '{resultIds}', $1::jsonb, true) where id = $2`, jsonProjection(filtered), taskID); err != nil {
			return err
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := tx.ExecContext(ctx, `
		update xz_assets
		set deleted_at = $3::timestamptz,
			updated_at = $3,
			metadata = jsonb_set(coalesce(metadata, '{}'::jsonb), '{deletedAt}', to_jsonb($3::text), true),
			raw = jsonb_set(
				jsonb_set(
					jsonb_set(coalesce(raw, '{}'::jsonb), '{deletedAt}', to_jsonb($3::text), true),
					'{updatedAt}', to_jsonb($3::text), true
				),
				'{metadata,deletedAt}', to_jsonb($3::text), true
			)
		where id = $1 and user_id = $2 and deleted_at is null
		  and (($4='ENTERPRISE' and tenant_id=$5) or ($4<>'ENTERPRISE' and (tenant_id is null or tenant_id='tenant_default')))
	`, id, userID, now, contextType, tenantID)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("%w: %s", errAssetNotFound, id)
	}
	if err := insertAuditLog(ctx, tx, userID, "MEMBER", "assets.delete", "asset", id, "", "", 200, map[string]any{"taskId": taskID}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *postgresStore) UpdateAssetThumbnails(updates map[string]string) (int, error) {
	if len(updates) == 0 {
		return 0, nil
	}
	ctx, cancel := s.withTimeout()
	defer cancel()
	updated := 0
	for id, thumbnailURL := range updates {
		res, err := s.db.ExecContext(ctx, `
			update xz_assets
			set thumbnail_url = $2,
				updated_at = $3,
				raw = jsonb_set(jsonb_set(raw, '{thumbnailUrl}', to_jsonb($2::text), true), '{metadata,thumbnailUrl}', to_jsonb($2::text), true)
			where id = $1 and coalesce(thumbnail_url, '') = ''
			  and deleted_at is null
		`, id, thumbnailURL, time.Now().UTC().Format(time.RFC3339Nano))
		if err != nil {
			return updated, err
		}
		rows, _ := res.RowsAffected()
		updated += int(rows)
	}
	return updated, nil
}

func (s *postgresStore) UpdateAssetImageInfo(updates map[string]assetImageInfo) (int, error) {
	if len(updates) == 0 {
		return 0, nil
	}
	ctx, cancel := s.withTimeout()
	defer cancel()
	updated := 0
	for id, info := range updates {
		var item asset
		err := s.db.QueryRowContext(ctx, `select raw from xz_assets where id = $1 and deleted_at is null`, id).Scan(rawScanner(&item))
		if err != nil {
			continue
		}
		changed := false
		if item.Metadata == nil {
			item.Metadata = map[string]any{}
		}
		if info.ThumbnailURL != "" && (item.ThumbnailURL == "" || item.ThumbnailURL == item.URL) {
			item.ThumbnailURL = info.ThumbnailURL
			item.Metadata["thumbnailUrl"] = info.ThumbnailURL
			changed = true
		}
		if info.Width > 0 && info.Height > 0 {
			item.Metadata["width"] = info.Width
			item.Metadata["height"] = info.Height
			item.Metadata["resolution"] = fmt.Sprintf("%dx%d", info.Width, info.Height)
			changed = true
		}
		if !changed {
			continue
		}
		item.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := s.db.ExecContext(ctx, `
			update xz_assets
			set thumbnail_url = $2,
				metadata = $3::jsonb,
				updated_at = $4,
				raw = $5::jsonb
			where id = $1
			  and deleted_at is null
		`, item.ID, item.ThumbnailURL, jsonProjection(item.Metadata), item.UpdatedAt, jsonProjection(item)); err != nil {
			return updated, err
		}
		updated++
	}
	return updated, nil
}

func (s *postgresStore) UpdateUserPassword(userID string, passwordHash string) (adminUser, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminUser{}, err
	}
	var item adminUser
	err := s.db.QueryRowContext(ctx, `select raw from xz_users where id = $1`, userID).Scan(rawScanner(&item))
	if err != nil {
		return adminUser{}, err
	}
	item.PasswordHash = passwordHash
	item.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.db.ExecContext(ctx, `update xz_users set password_hash = $2, updated_at = $3, raw = $4::jsonb where id = $1`, item.ID, item.PasswordHash, item.UpdatedAt, jsonProjection(item))
	return item, err
}

func (s *postgresStore) CreateAdminCustomer(req adminCustomerMutation) (adminUser, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminUser{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return adminUser{}, err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	userID, err := nextTableID(ctx, tx, "xz_users", "user")
	if err != nil {
		return adminUser{}, err
	}
	item := adminUser{
		ID:                    userID,
		Email:                 req.Email,
		Mobile:                strings.TrimSpace(req.Mobile),
		WeChatOpenIDs:         appendUniqueString(nil, req.WeChatOpenID),
		WeChatUnionID:         strings.TrimSpace(req.WeChatUnionID),
		RegistrationSource:    cloneStringMap(req.RegistrationSource),
		Name:                  req.Name,
		Role:                  fallback(req.Role, "MEMBER"),
		Status:                fallback(req.Status, "ACTIVE"),
		PlanID:                fallback(req.PlanID, "plan_free"),
		ReferredBy:            strings.TrimSpace(req.ReferredBy),
		SubscriptionExpiresAt: strings.TrimSpace(req.SubscriptionExpiresAt),
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	if err := insertUser(ctx, tx, item); err != nil {
		return adminUser{}, err
	}
	available := 0
	if req.Available != nil {
		available = *req.Available
	}
	if err := upsertPointAccountByUser(ctx, tx, item.ID, available); err != nil {
		return adminUser{}, err
	}
	if err := insertAuditLog(ctx, tx, "", "", "customers.create", "user", item.ID, "", "", 200, map[string]any{"email": item.Email}); err != nil {
		return adminUser{}, err
	}
	return item, tx.Commit()
}

func (s *postgresStore) CreateRegisteredCustomer(req adminCustomerMutation, grantPoints int) (adminUser, error) {
	if grantPoints <= 0 {
		return adminUser{}, ErrInvalidPointCommand
	}
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminUser{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return adminUser{}, err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC()
	userID, err := nextTableID(ctx, tx, "xz_users", "user")
	if err != nil {
		return adminUser{}, err
	}
	item := adminUser{
		ID: userID, Email: req.Email, Mobile: strings.TrimSpace(req.Mobile), WeChatOpenIDs: appendUniqueString(nil, req.WeChatOpenID),
		WeChatUnionID: strings.TrimSpace(req.WeChatUnionID), RegistrationSource: cloneStringMap(req.RegistrationSource), Name: req.Name,
		Role: fallback(req.Role, "MEMBER"), Status: fallback(req.Status, "ACTIVE"), PlanID: fallback(req.PlanID, "plan_free"),
		ReferredBy: strings.TrimSpace(req.ReferredBy), SubscriptionExpiresAt: strings.TrimSpace(req.SubscriptionExpiresAt),
		CreatedAt: now.Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano),
	}
	if err := insertUser(ctx, tx, item); err != nil {
		return adminUser{}, err
	}
	if err := upsertPointAccountByUser(ctx, tx, item.ID, 0); err != nil {
		return adminUser{}, err
	}
	account, err := pgLoadPersonalAccountForUserTx(ctx, tx, item.ID)
	if err != nil {
		return adminUser{}, err
	}
	if _, err := NewPostgresPersonalPointStore(s.db).grantTx(ctx, tx, PersonalPointGrantCommand{
		AccountID: account.ID, UserID: item.ID, Source: PointSourceRegistrationGift, Points: int64(grantPoints),
		ReferenceType: "PLAN", ReferenceID: item.PlanID, IdempotencyKey: "registration:" + item.ID, GrantedAt: now,
	}); err != nil {
		return adminUser{}, err
	}
	if err := insertAuditLog(ctx, tx, item.ID, item.Role, "auth.registration", "user", item.ID, "", "", 201, map[string]any{"planId": item.PlanID, "grantPoints": grantPoints, "source": PointSourceRegistrationGift}); err != nil {
		return adminUser{}, err
	}
	return item, tx.Commit()
}

func (s *postgresStore) UpdateAdminCustomer(id string, req adminCustomerMutation) (adminUser, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return adminUser{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var item adminUser
	if err := tx.QueryRowContext(ctx, `select raw from xz_users where id = $1 for update`, id).Scan(rawScanner(&item)); err != nil {
		return adminUser{}, err
	}
	if req.Name != "" {
		item.Name = req.Name
	}
	if req.Email != "" {
		item.Email = req.Email
	}
	if req.Mobile != "" {
		item.Mobile = strings.TrimSpace(req.Mobile)
	}
	if req.WeChatOpenID != "" {
		item.WeChatOpenIDs = appendUniqueString(item.WeChatOpenIDs, req.WeChatOpenID)
	}
	if req.WeChatUnionID != "" {
		item.WeChatUnionID = strings.TrimSpace(req.WeChatUnionID)
	}
	if req.Status != "" {
		item.Status = req.Status
	}
	item.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if customerModelRouteRequested(req) {
		route, err := applyCustomerModelRouteTx(ctx, tx, item, req, item.UpdatedAt)
		if err != nil {
			return adminUser{}, err
		}
		if route.ID != "" {
			route, err = s.syncExistingNewAPIRouteForCustomerUpdate(ctx, tx, item, route)
			if err != nil {
				return adminUser{}, err
			}
			item.ModelRoutes = upsertUserModelRoute(item.ModelRoutes, route)
		}
	}
	if err := insertUser(ctx, tx, item); err != nil {
		return adminUser{}, err
	}
	if req.Available != nil {
		if err := upsertPointAccountByUser(ctx, tx, item.ID, *req.Available); err != nil {
			return adminUser{}, err
		}
	}
	if err := insertAuditLog(ctx, tx, "", "", "customers.update", "user", item.ID, "", "", 200, nil); err != nil {
		return adminUser{}, err
	}
	return item, tx.Commit()
}

func (s *postgresStore) UpdateAdminCustomerIdentity(id string, req adminCustomerIdentityMutation) (adminUser, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return adminUser{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var item adminUser
	if err := tx.QueryRowContext(ctx, `select raw from xz_users where id = $1 for update`, id).Scan(rawScanner(&item)); err != nil {
		return adminUser{}, err
	}
	if req.ClearMobile {
		item.Mobile = ""
	}
	if req.ClearWeChat {
		item.WeChatOpenIDs = nil
		item.WeChatUnionID = ""
	}
	if req.Status != "" {
		item.Status = strings.ToUpper(strings.TrimSpace(req.Status))
	}
	item.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := insertUser(ctx, tx, item); err != nil {
		return adminUser{}, err
	}
	if err := insertAuditLog(ctx, tx, "", "", "customers.identity.update", "user", item.ID, "", "", 200, map[string]any{
		"clearMobile": req.ClearMobile,
		"clearWechat": req.ClearWeChat,
		"status":      req.Status,
		"reason":      req.Reason,
	}); err != nil {
		return adminUser{}, err
	}
	return item, tx.Commit()
}

func (s *postgresStore) CreateAdminAuthMergeRequest(req adminAuthMergeRequestMutation) (adminAuthMergeRequest, error) {
	req = normalizeAuthMergeRequestMutation(req)
	if req.PrimaryUserID == "" || req.SecondaryUserID == "" {
		return adminAuthMergeRequest{}, errors.New("primaryUserId and secondaryUserId are required")
	}
	if strings.EqualFold(req.PrimaryUserID, req.SecondaryUserID) {
		return adminAuthMergeRequest{}, errors.New("merge request requires two different users")
	}
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminAuthMergeRequest{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return adminAuthMergeRequest{}, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `
		select raw
		from xz_auth_account_merge_requests
		where status in ('PENDING','IN_REVIEW')
		  and ((primary_user_id=$1 and secondary_user_id=$2) or (primary_user_id=$2 and secondary_user_id=$1))
		order by created_at desc, id desc
	`, req.PrimaryUserID, req.SecondaryUserID)
	if err != nil {
		return adminAuthMergeRequest{}, err
	}
	for rows.Next() {
		var item adminAuthMergeRequest
		if err := rows.Scan(rawScanner(&item)); err != nil {
			_ = rows.Close()
			return adminAuthMergeRequest{}, err
		}
		if sameOpenAuthMergeRequest(item, req) {
			_ = rows.Close()
			return item, nil
		}
	}
	if err := rows.Close(); err != nil {
		return adminAuthMergeRequest{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	id, err := nextTableID(ctx, tx, "xz_auth_account_merge_requests", "auth_merge")
	if err != nil {
		return adminAuthMergeRequest{}, err
	}
	item := adminAuthMergeRequest{
		ID:              id,
		PrimaryUserID:   req.PrimaryUserID,
		SecondaryUserID: req.SecondaryUserID,
		Mobile:          req.Mobile,
		WeChatOpenID:    req.WeChatOpenID,
		WeChatUnionID:   req.WeChatUnionID,
		ConflictCode:    fallback(req.ConflictCode, "AUTH_ACCOUNT_MERGE_REQUIRED"),
		Source:          fallback(req.Source, "auth_conflict"),
		Reason:          req.Reason,
		Status:          fallback(req.Status, "PENDING"),
		CreatedAt:       now,
		UpdatedAt:       now,
		Raw:             cloneAnyMap(req.Raw),
	}
	if err := insertAuthMergeRequest(ctx, tx, item); err != nil {
		return adminAuthMergeRequest{}, err
	}
	if err := insertAuditLog(ctx, tx, "", "", "auth.merge_request.create", "auth_merge_request", item.ID, "", "", 200, map[string]any{
		"primaryUserId":   item.PrimaryUserID,
		"secondaryUserId": item.SecondaryUserID,
		"source":          item.Source,
		"conflictCode":    item.ConflictCode,
	}); err != nil {
		return adminAuthMergeRequest{}, err
	}
	return item, tx.Commit()
}

func (s *postgresStore) ListAdminAuthMergeRequests(userID string) ([]adminAuthMergeRequest, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return nil, err
	}
	return s.listAuthMergeRequests(ctx, userID)
}

func (s *postgresStore) listAuthMergeRequests(ctx context.Context, userID string) ([]adminAuthMergeRequest, error) {
	userID = strings.TrimSpace(userID)
	var rows *sql.Rows
	var err error
	if userID == "" {
		rows, err = s.db.QueryContext(ctx, `select raw from xz_auth_account_merge_requests order by created_at desc, id desc`)
	} else {
		rows, err = s.db.QueryContext(ctx, `select raw from xz_auth_account_merge_requests where primary_user_id=$1 or secondary_user_id=$1 order by created_at desc, id desc`, userID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []adminAuthMergeRequest{}
	for rows.Next() {
		var item adminAuthMergeRequest
		if err := rows.Scan(rawScanner(&item)); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) UpdateAdminAuthMergeRequest(id string, req adminAuthMergeRequestMutation) (adminAuthMergeRequest, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminAuthMergeRequest{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return adminAuthMergeRequest{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var item adminAuthMergeRequest
	if err := tx.QueryRowContext(ctx, `select raw from xz_auth_account_merge_requests where id=$1 for update`, strings.TrimSpace(id)).Scan(rawScanner(&item)); err != nil {
		return adminAuthMergeRequest{}, err
	}
	status := normalizeAuthMergeStatus(req.Status, item.Status)
	if status == "" {
		return adminAuthMergeRequest{}, errors.New("status is required")
	}
	if !validAuthMergeStatus(status) {
		return adminAuthMergeRequest{}, errors.New("invalid merge request status")
	}
	item.Status = status
	if strings.TrimSpace(req.ReviewComment) != "" {
		item.ReviewComment = strings.TrimSpace(req.ReviewComment)
	}
	if strings.TrimSpace(req.ResolvedBy) != "" {
		item.ResolvedBy = strings.TrimSpace(req.ResolvedBy)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if authMergeClosedStatus(status) && item.ResolvedAt == "" {
		item.ResolvedAt = now
	}
	item.UpdatedAt = now
	if err := insertAuthMergeRequest(ctx, tx, item); err != nil {
		return adminAuthMergeRequest{}, err
	}
	if err := insertAuditLog(ctx, tx, "", "", "auth.merge_request.update", "auth_merge_request", item.ID, "", "", 200, map[string]any{
		"status":        item.Status,
		"reviewComment": item.ReviewComment,
		"resolvedBy":    item.ResolvedBy,
	}); err != nil {
		return adminAuthMergeRequest{}, err
	}
	return item, tx.Commit()
}

func (s *postgresStore) PreviewAdminAuthMergeRequest(id string, targetUserID string) (adminAuthMergeRequest, adminAuthMergePreviewResult, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminAuthMergeRequest{}, adminAuthMergePreviewResult{}, err
	}
	data, err := s.AdminData()
	if err != nil {
		return adminAuthMergeRequest{}, adminAuthMergePreviewResult{}, err
	}
	item, result, err := previewAdminAuthMergeRequestOnData(&data, id, targetUserID)
	if err != nil {
		return adminAuthMergeRequest{}, adminAuthMergePreviewResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return adminAuthMergeRequest{}, adminAuthMergePreviewResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if moved, warnings, err := previewPostgresUserOwnershipForAdminAuthMerge(ctx, tx, result.TargetUserID, result.SourceUserID); err != nil {
		return adminAuthMergeRequest{}, adminAuthMergePreviewResult{}, err
	} else {
		for key, value := range moved {
			result.Moved[key] += value
		}
		result.Moved = compactPositiveCounts(result.Moved)
		result.Warnings = append(result.Warnings, warnings...)
	}
	return item, result, nil
}

func (s *postgresStore) ExecuteAdminAuthMergeRequest(id string, req adminAuthMergeExecuteRequest) (adminAuthMergeRequest, adminAuthMergeExecuteResult, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminAuthMergeRequest{}, adminAuthMergeExecuteResult{}, err
	}
	data, err := s.AdminData()
	if err != nil {
		return adminAuthMergeRequest{}, adminAuthMergeExecuteResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return adminAuthMergeRequest{}, adminAuthMergeExecuteResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	lockedRequest, err := lockAdminAuthMergeRequestTx(ctx, tx, id)
	if err != nil {
		return adminAuthMergeRequest{}, adminAuthMergeExecuteResult{}, err
	}
	targetUserID := strings.TrimSpace(req.TargetUserID)
	if targetUserID == "" {
		targetUserID = lockedRequest.PrimaryUserID
	}
	sourceUserID := lockedRequest.SecondaryUserID
	if targetUserID == lockedRequest.SecondaryUserID {
		sourceUserID = lockedRequest.PrimaryUserID
	}
	lockedUsers, err := lockAdminAuthMergeUsersTx(ctx, tx, targetUserID, sourceUserID)
	if err != nil {
		return adminAuthMergeRequest{}, adminAuthMergeExecuteResult{}, err
	}
	requestIndex := adminAuthMergeRequestIndex(data.AuthMergeRequests, id)
	if requestIndex < 0 {
		return adminAuthMergeRequest{}, adminAuthMergeExecuteResult{}, fmt.Errorf("auth merge request not found: %s", id)
	}
	data.AuthMergeRequests[requestIndex] = lockedRequest
	for userID, locked := range lockedUsers {
		found := false
		for i := range data.Users {
			if data.Users[i].ID == userID {
				data.Users[i] = locked
				found = true
				break
			}
		}
		if !found {
			return adminAuthMergeRequest{}, adminAuthMergeExecuteResult{}, fmt.Errorf("user not found: %s", userID)
		}
	}
	_, _, _, _, sourceUserID, err = resolveAdminAuthMergeUsers(&data, id, req.TargetUserID)
	if err != nil {
		return adminAuthMergeRequest{}, adminAuthMergeExecuteResult{}, err
	}
	sourceOrderIDs := adminAuthMergeOrderIDs(data.Orders, sourceUserID)
	pointStore := NewPostgresPersonalPointStore(s.db)
	updated, result, err := executeAdminAuthMergeRequestOnDataWithPointMerge(&data, id, req, func(targetID, sourceID string) (int, error) {
		merged, mergeErr := pointStore.mergeTx(ctx, tx, targetID, sourceID, id, time.Now().UTC())
		return merged.AccountsMoved, mergeErr
	})
	if err != nil {
		return adminAuthMergeRequest{}, adminAuthMergeExecuteResult{}, err
	}
	target, source, err := usersForMergeResult(data.Users, result)
	if err != nil {
		return adminAuthMergeRequest{}, adminAuthMergeExecuteResult{}, err
	}
	// Persist the source first so its unique login identities (mobile and
	// WeChat identifiers) are released before they are assigned to the target.
	// Updating the target first violates the partial unique indexes while the
	// source row still owns those values.
	if err := insertUser(ctx, tx, source); err != nil {
		return adminAuthMergeRequest{}, adminAuthMergeExecuteResult{}, err
	}
	if err := insertUser(ctx, tx, target); err != nil {
		return adminAuthMergeRequest{}, adminAuthMergeExecuteResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `delete from xz_user_model_routes where user_id = $1`, result.SourceUserID); err != nil {
		return adminAuthMergeRequest{}, adminAuthMergeExecuteResult{}, err
	}
	for _, item := range data.TokenRecords {
		if item.UserID == result.TargetUserID {
			if err := insertTokenRecord(ctx, tx, item); err != nil {
				return adminAuthMergeRequest{}, adminAuthMergeExecuteResult{}, err
			}
		}
	}
	for _, item := range data.Orders {
		if sourceOrderIDs[item.ID] {
			if err := insertOrder(ctx, tx, item); err != nil {
				return adminAuthMergeRequest{}, adminAuthMergeExecuteResult{}, err
			}
		}
	}
	for _, item := range data.ChannelAgents {
		if item.UserID == result.TargetUserID {
			if err := insertChannelAgent(ctx, tx, item); err != nil {
				return adminAuthMergeRequest{}, adminAuthMergeExecuteResult{}, err
			}
		}
	}
	for _, item := range data.OperationCenters {
		if item.UserID == result.TargetUserID {
			if err := insertOperationCenter(ctx, tx, item); err != nil {
				return adminAuthMergeRequest{}, adminAuthMergeExecuteResult{}, err
			}
		}
	}
	for _, item := range data.GenerationTasks {
		if item.UserID == result.TargetUserID {
			if err := insertGenerationTask(ctx, tx, item); err != nil {
				return adminAuthMergeRequest{}, adminAuthMergeExecuteResult{}, err
			}
		}
	}
	for _, item := range data.Assets {
		if item.UserID == result.TargetUserID {
			if err := insertAsset(ctx, tx, item); err != nil {
				return adminAuthMergeRequest{}, adminAuthMergeExecuteResult{}, err
			}
		}
	}
	for _, item := range data.BillingEvents {
		if item.UserID == result.TargetUserID {
			if err := insertBillingEvent(ctx, tx, item); err != nil {
				return adminAuthMergeRequest{}, adminAuthMergeExecuteResult{}, err
			}
		}
	}
	if moved, err := mergeUserAIStateForAdminAuthMerge(ctx, tx, result.TargetUserID, result.SourceUserID); err != nil {
		return adminAuthMergeRequest{}, adminAuthMergeExecuteResult{}, err
	} else if moved > 0 {
		result.Moved["aiState"] = moved
	}
	if moved, warnings, err := migratePostgresUserOwnershipForAdminAuthMerge(ctx, tx, result.TargetUserID, result.SourceUserID); err != nil {
		return adminAuthMergeRequest{}, adminAuthMergeExecuteResult{}, err
	} else {
		for key, value := range moved {
			result.Moved[key] += value
		}
		result.Warnings = append(result.Warnings, warnings...)
	}
	if updated.Raw == nil {
		updated.Raw = map[string]any{}
	}
	updated.Raw["executeResult"] = map[string]any{
		"targetUserId": result.TargetUserID,
		"sourceUserId": result.SourceUserID,
		"moved":        result.Moved,
		"warnings":     result.Warnings,
	}
	if err := insertAuthMergeRequest(ctx, tx, updated); err != nil {
		return adminAuthMergeRequest{}, adminAuthMergeExecuteResult{}, err
	}
	if err := insertAuditLog(ctx, tx, "", "", "auth.merge_request.execute", "auth_merge_request", updated.ID, "", "", 200, map[string]any{
		"targetUserId": result.TargetUserID,
		"sourceUserId": result.SourceUserID,
		"moved":        result.Moved,
	}); err != nil {
		return adminAuthMergeRequest{}, adminAuthMergeExecuteResult{}, err
	}
	return updated, result, tx.Commit()
}

func lockAdminAuthMergeRequestTx(ctx context.Context, tx *sql.Tx, id string) (adminAuthMergeRequest, error) {
	var item adminAuthMergeRequest
	err := tx.QueryRowContext(ctx, `SELECT raw FROM xz_auth_account_merge_requests WHERE id=$1 FOR UPDATE`, strings.TrimSpace(id)).Scan(rawScanner(&item))
	if errors.Is(err, sql.ErrNoRows) {
		return item, fmt.Errorf("auth merge request not found: %s", id)
	}
	return item, err
}

func lockAdminAuthMergeUsersTx(ctx context.Context, tx *sql.Tx, targetUserID, sourceUserID string) (map[string]adminUser, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id,raw FROM xz_users WHERE id IN($1,$2) ORDER BY id FOR UPDATE`, targetUserID, sourceUserID)
	if err != nil {
		return nil, err
	}
	users := map[string]adminUser{}
	for rows.Next() {
		var id string
		var user adminUser
		if err := rows.Scan(&id, rawScanner(&user)); err != nil {
			_ = rows.Close()
			return nil, err
		}
		user.ID = id
		users[id] = user
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(users) != 2 {
		return nil, errors.New("target or source user not found")
	}
	routeRows, err := tx.QueryContext(ctx, `SELECT user_id,raw FROM xz_user_model_routes WHERE user_id IN($1,$2) ORDER BY user_id,id FOR UPDATE`, targetUserID, sourceUserID)
	if err != nil {
		return nil, err
	}
	for routeRows.Next() {
		var userID string
		var route adminUserModelRoute
		if err := routeRows.Scan(&userID, rawScanner(&route)); err != nil {
			_ = routeRows.Close()
			return nil, err
		}
		user := users[userID]
		user.ModelRoutes = mergeUserModelRoutes(user.ModelRoutes, []adminUserModelRoute{route})
		users[userID] = user
	}
	if err := routeRows.Close(); err != nil {
		return nil, err
	}
	return users, nil
}

func adminAuthMergeOrderIDs(items []adminOrder, userID string) map[string]bool {
	result := map[string]bool{}
	for _, item := range items {
		if item.ID == "" {
			continue
		}
		if item.UserID == userID || item.BuyerUserID == userID || (item.PriceSnapshot != nil && stringValue(item.PriceSnapshot["buyerUserId"]) == userID) {
			result[item.ID] = true
		}
	}
	return result
}

func mergeUserAIStateForAdminAuthMerge(ctx context.Context, tx *sql.Tx, targetID string, sourceID string) (int, error) {
	exists, err := txTableExists(ctx, tx, "xz_ai_state")
	if err != nil || !exists {
		return 0, err
	}
	target, targetExists, err := selectUserAIStateForMerge(ctx, tx, targetID)
	if err != nil {
		return 0, err
	}
	source, sourceExists, err := selectUserAIStateForMerge(ctx, tx, sourceID)
	if err != nil || !sourceExists {
		return 0, err
	}
	if !targetExists {
		target = userAIState{UserID: targetID}
	}
	merged, moved := mergeUserAIStateValues(target, source, targetID)
	if moved == 0 {
		return 0, nil
	}
	if err := upsertUserAIStateTx(ctx, tx, merged); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `delete from xz_ai_state where user_id = $1`, sourceID); err != nil {
		return 0, err
	}
	return moved, nil
}

func selectUserAIStateForMerge(ctx context.Context, tx *sql.Tx, userID string) (userAIState, bool, error) {
	var item userAIState
	err := tx.QueryRowContext(ctx, `select raw from xz_ai_state where user_id = $1 for update`, userID).Scan(rawScanner(&item))
	if errors.Is(err, sql.ErrNoRows) {
		return userAIState{}, false, nil
	}
	if err != nil {
		return userAIState{}, false, err
	}
	item.UserID = userID
	return item, true, nil
}

func upsertUserAIStateTx(ctx context.Context, tx *sql.Tx, item userAIState) error {
	_, err := tx.ExecContext(ctx, `
		insert into xz_ai_state (user_id, favorite_task_ids, hidden_task_ids, favorite_collections, agent_conversations, active_conversation_id, active_collection_id, updated_at, raw)
		values ($1,$2::jsonb,$3::jsonb,$4::jsonb,$5::jsonb,$6,$7,$8,$9::jsonb)
		on conflict (user_id) do update set
			favorite_task_ids = excluded.favorite_task_ids,
			hidden_task_ids = excluded.hidden_task_ids,
			favorite_collections = excluded.favorite_collections,
			agent_conversations = excluded.agent_conversations,
			active_conversation_id = excluded.active_conversation_id,
			active_collection_id = excluded.active_collection_id,
			updated_at = excluded.updated_at,
			raw = excluded.raw
	`, item.UserID, jsonProjection(item.FavoriteTaskIDs), jsonProjection(item.HiddenTaskIDs), jsonProjection(item.FavoriteCollections), jsonProjection(item.AgentConversations), item.ActiveConversationID, item.ActiveCollectionID, item.UpdatedAt, jsonProjection(item))
	return err
}

func previewPostgresUserOwnershipForAdminAuthMerge(ctx context.Context, tx *sql.Tx, targetID string, sourceID string) (map[string]int, []string, error) {
	moved := map[string]int{}
	warnings := []string{}
	counts := []struct {
		key     string
		table   string
		columns []string
		sql     string
		args    []any
	}{
		{key: "aiState", table: "xz_ai_state", columns: []string{"user_id"}, sql: `select count(*) from xz_ai_state where user_id = $1`, args: []any{sourceID}},
		{key: "tenantOwners", table: "xz_tenants", columns: []string{"owner_user_id"}, sql: `select count(*) from xz_tenants where owner_user_id = $1`, args: []any{sourceID}},
		{key: "knowledgeBases", table: "xz_knowledge_bases", columns: []string{"owner_user_id", "deleted_at"}, sql: `select count(*) from xz_knowledge_bases where owner_user_id = $1 and deleted_at is null`, args: []any{sourceID}},
		{key: "knowledgeDocuments", table: "xz_knowledge_documents", columns: []string{"owner_user_id", "deleted_at"}, sql: `select count(*) from xz_knowledge_documents where owner_user_id = $1 and deleted_at is null`, args: []any{sourceID}},
		{key: "knowledgeDocumentVersions", table: "xz_knowledge_document_versions", columns: []string{"created_by"}, sql: `select count(*) from xz_knowledge_document_versions where created_by = $1`, args: []any{sourceID}},
		{key: "knowledgeAgents", table: "xz_ai_agents", columns: []string{"owner_user_id", "deleted_at"}, sql: `select count(*) from xz_ai_agents where owner_user_id = $1 and deleted_at is null`, args: []any{sourceID}},
		{key: "knowledgeAgentConversations", table: "xz_ai_agent_conversations", columns: []string{"user_id", "deleted_at"}, sql: `select count(*) from xz_ai_agent_conversations where user_id = $1 and deleted_at is null`, args: []any{sourceID}},
		{key: "tenantJoinRequests", table: "xz_tenant_join_requests", columns: []string{"applicant_user_id"}, sql: `select count(*) from xz_tenant_join_requests where applicant_user_id = $1`, args: []any{sourceID}},
		{key: "userRoles", table: "xz_user_roles", columns: []string{"user_id"}, sql: `select count(*) from xz_user_roles where user_id = $1`, args: []any{sourceID}},
		{key: "userRoleContext", table: "xz_user_role_context", columns: []string{"user_id"}, sql: `select count(*) from xz_user_role_context where user_id = $1`, args: []any{sourceID}},
	}
	for _, item := range counts {
		count, skipped, err := countIfTableColumnsExist(ctx, tx, item.table, item.columns, item.sql, item.args...)
		if err != nil {
			return nil, nil, err
		}
		if skipped {
			warnings = append(warnings, fmt.Sprintf("%s preview skipped because table or columns are not available", item.key))
			continue
		}
		if count > 0 {
			moved[item.key] = count
		}
	}
	if exists, err := txTableExists(ctx, tx, "xz_tenant_members"); err != nil {
		return nil, nil, err
	} else if exists {
		count, err := countRows(ctx, tx, `
			select count(*)
			from xz_tenant_members member
			where member.user_id = $2
			  and not exists (
			    select 1 from xz_tenant_members target_member
			    where target_member.tenant_id = member.tenant_id and target_member.user_id = $1
			  )
		`, targetID, sourceID)
		if err != nil {
			return nil, nil, err
		}
		if count > 0 {
			moved["tenantMembers"] = count
		}
		conflicts, err := countRows(ctx, tx, `
			select count(*)
			from xz_tenant_members member
			where member.user_id = $2
			  and exists (
			    select 1 from xz_tenant_members target_member
			    where target_member.tenant_id = member.tenant_id and target_member.user_id = $1
			  )
		`, targetID, sourceID)
		if err != nil {
			return nil, nil, err
		}
		if conflicts > 0 {
			moved["tenantMemberConflictsDisabled"] = conflicts
			warnings = append(warnings, "source tenant memberships with existing target memberships will be disabled instead of overwritten")
		}
	}
	return moved, warnings, nil
}

func migratePostgresUserOwnershipForAdminAuthMerge(ctx context.Context, tx *sql.Tx, targetID string, sourceID string) (map[string]int, []string, error) {
	moved := map[string]int{}
	warnings := []string{}
	updates := []struct {
		key     string
		table   string
		columns []string
		sql     string
		args    []any
	}{
		{key: "tenantOwners", table: "xz_tenants", columns: []string{"owner_user_id", "updated_at"}, sql: `update xz_tenants set owner_user_id = $1, updated_at = now() where owner_user_id = $2`, args: []any{targetID, sourceID}},
		{key: "knowledgeBases", table: "xz_knowledge_bases", columns: []string{"owner_user_id", "updated_at", "deleted_at"}, sql: `update xz_knowledge_bases set owner_user_id = $1, updated_at = now() where owner_user_id = $2 and deleted_at is null`, args: []any{targetID, sourceID}},
		{key: "knowledgeDocuments", table: "xz_knowledge_documents", columns: []string{"owner_user_id", "updated_at", "deleted_at"}, sql: `update xz_knowledge_documents set owner_user_id = $1, updated_at = now() where owner_user_id = $2 and deleted_at is null`, args: []any{targetID, sourceID}},
		{key: "knowledgeDocumentVersions", table: "xz_knowledge_document_versions", columns: []string{"created_by"}, sql: `update xz_knowledge_document_versions set created_by = $1 where created_by = $2`, args: []any{targetID, sourceID}},
		{key: "knowledgeAgents", table: "xz_ai_agents", columns: []string{"owner_user_id", "updated_at", "deleted_at"}, sql: `update xz_ai_agents set owner_user_id = $1, updated_at = now() where owner_user_id = $2 and deleted_at is null`, args: []any{targetID, sourceID}},
		{key: "knowledgeAgentConversations", table: "xz_ai_agent_conversations", columns: []string{"user_id", "updated_at", "deleted_at"}, sql: `update xz_ai_agent_conversations set user_id = $1, updated_at = now() where user_id = $2 and deleted_at is null`, args: []any{targetID, sourceID}},
		{key: "tenantJoinRequests", table: "xz_tenant_join_requests", columns: []string{"applicant_user_id", "updated_at"}, sql: `update xz_tenant_join_requests set applicant_user_id = $1, updated_at = now() where applicant_user_id = $2`, args: []any{targetID, sourceID}},
	}
	for _, update := range updates {
		count, skipped, err := execIfTableColumnsExist(ctx, tx, update.table, update.columns, update.sql, update.args...)
		if err != nil {
			return nil, nil, err
		}
		if skipped {
			warnings = append(warnings, fmt.Sprintf("%s migration skipped because table or columns are not available", update.key))
			continue
		}
		if count > 0 {
			moved[update.key] = count
		}
	}

	if exists, err := txTableExists(ctx, tx, "xz_tenant_members"); err != nil {
		return nil, nil, err
	} else if exists {
		hasMemberStatus, err := txColumnExists(ctx, tx, "xz_tenant_members", "member_status")
		if err != nil {
			return nil, nil, err
		}
		count, err := execRowsAffected(ctx, tx, `
			update xz_tenant_members member
			set user_id = $1, updated_at = now()
			where member.user_id = $2
			  and not exists (
			    select 1 from xz_tenant_members target_member
			    where target_member.tenant_id = member.tenant_id and target_member.user_id = $1
			  )
		`, targetID, sourceID)
		if err != nil {
			return nil, nil, err
		}
		if count > 0 {
			moved["tenantMembers"] = count
		}
		disableSQL := `
			update xz_tenant_members member
			set status = 'DISABLED', updated_at = now()
			where member.user_id = $2
			  and exists (
			    select 1 from xz_tenant_members target_member
			    where target_member.tenant_id = member.tenant_id and target_member.user_id = $1
			  )
		`
		if hasMemberStatus {
			disableSQL = `
				update xz_tenant_members member
				set status = 'DISABLED', member_status = 'MERGED', disabled_at = now(), disabled_by = $1, updated_at = now()
				where member.user_id = $2
				  and exists (
				    select 1 from xz_tenant_members target_member
				    where target_member.tenant_id = member.tenant_id and target_member.user_id = $1
				  )
			`
		}
		disabled, err := execRowsAffected(ctx, tx, disableSQL, targetID, sourceID)
		if err != nil {
			return nil, nil, err
		}
		if disabled > 0 {
			moved["tenantMemberConflictsDisabled"] = disabled
			warnings = append(warnings, "source tenant memberships with existing target memberships were disabled instead of overwritten")
		}
	}

	if exists, err := txTableExists(ctx, tx, "xz_user_roles"); err != nil {
		return nil, nil, err
	} else if exists {
		count, err := execRowsAffected(ctx, tx, `
			insert into xz_user_roles (user_id, tenant_id, organization_id, role, status, assigned_at, updated_at)
			select $1, tenant_id, organization_id, role, status, assigned_at, now()
			from xz_user_roles
			where user_id = $2
			on conflict (user_id, tenant_id, organization_id, role)
			do update set status = excluded.status, updated_at = excluded.updated_at
		`, targetID, sourceID)
		if err != nil {
			return nil, nil, err
		}
		if count > 0 {
			moved["userRoles"] = count
		}
		if _, err := tx.ExecContext(ctx, `update xz_user_roles set status = 'DISABLED', updated_at = now() where user_id = $1`, sourceID); err != nil {
			return nil, nil, err
		}
	}

	if exists, err := txTableExists(ctx, tx, "xz_user_role_context"); err != nil {
		return nil, nil, err
	} else if exists {
		hasContextType, err := txColumnExists(ctx, tx, "xz_user_role_context", "context_type")
		if err != nil {
			return nil, nil, err
		}
		contextSQL := `
			insert into xz_user_role_context (user_id, tenant_id, organization_id, current_role_code, updated_at)
			select $1, tenant_id, organization_id, current_role_code, now()
			from xz_user_role_context
			where user_id = $2
			on conflict (user_id) do nothing
		`
		if hasContextType {
			contextSQL = `
				insert into xz_user_role_context (user_id, tenant_id, organization_id, current_role_code, updated_at, context_type)
				select $1, tenant_id, organization_id, current_role_code, now(), context_type
				from xz_user_role_context
				where user_id = $2
				on conflict (user_id) do nothing
			`
		}
		count, err := execRowsAffected(ctx, tx, contextSQL, targetID, sourceID)
		if err != nil {
			return nil, nil, err
		}
		if count > 0 {
			moved["userRoleContext"] = count
		}
		if _, err := tx.ExecContext(ctx, `delete from xz_user_role_context where user_id = $1`, sourceID); err != nil {
			return nil, nil, err
		}
	}
	return moved, warnings, nil
}

func execIfTableExists(ctx context.Context, tx *sql.Tx, table string, query string, args ...any) (int, error) {
	exists, err := txTableExists(ctx, tx, table)
	if err != nil || !exists {
		return 0, err
	}
	return execRowsAffected(ctx, tx, query, args...)
}

func execIfTableColumnsExist(ctx context.Context, tx *sql.Tx, table string, columns []string, query string, args ...any) (int, bool, error) {
	ready, err := txTableColumnsExist(ctx, tx, table, columns)
	if err != nil || !ready {
		return 0, !ready, err
	}
	count, err := execRowsAffected(ctx, tx, query, args...)
	return count, false, err
}

func countIfTableExists(ctx context.Context, tx *sql.Tx, table string, query string, args ...any) (int, error) {
	exists, err := txTableExists(ctx, tx, table)
	if err != nil || !exists {
		return 0, err
	}
	return countRows(ctx, tx, query, args...)
}

func countIfTableColumnsExist(ctx context.Context, tx *sql.Tx, table string, columns []string, query string, args ...any) (int, bool, error) {
	ready, err := txTableColumnsExist(ctx, tx, table, columns)
	if err != nil || !ready {
		return 0, !ready, err
	}
	count, err := countRows(ctx, tx, query, args...)
	return count, false, err
}

func txTableExists(ctx context.Context, tx *sql.Tx, table string) (bool, error) {
	var exists bool
	if err := tx.QueryRowContext(ctx, `select to_regclass($1) is not null`, "public."+table).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func txTableColumnsExist(ctx context.Context, tx *sql.Tx, table string, columns []string) (bool, error) {
	exists, err := txTableExists(ctx, tx, table)
	if err != nil || !exists {
		return false, err
	}
	for _, column := range columns {
		column = strings.TrimSpace(column)
		if column == "" {
			continue
		}
		exists, err := txColumnExists(ctx, tx, table, column)
		if err != nil || !exists {
			return false, err
		}
	}
	return true, nil
}

func txColumnExists(ctx context.Context, tx *sql.Tx, table string, column string) (bool, error) {
	var exists bool
	err := tx.QueryRowContext(ctx, `
		select exists(
			select 1
			from information_schema.columns
			where table_schema = 'public' and table_name = $1 and column_name = $2
		)
	`, table, column).Scan(&exists)
	return exists, err
}

func countRows(ctx context.Context, tx *sql.Tx, query string, args ...any) (int, error) {
	var count int
	if err := tx.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func execRowsAffected(ctx context.Context, tx *sql.Tx, query string, args ...any) (int, error) {
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

func usersForMergeResult(users []adminUser, result adminAuthMergeExecuteResult) (adminUser, adminUser, error) {
	var target, source adminUser
	for _, user := range users {
		if user.ID == result.TargetUserID {
			target = user
		}
		if user.ID == result.SourceUserID {
			source = user
		}
	}
	if target.ID == "" || source.ID == "" {
		return adminUser{}, adminUser{}, errors.New("merged users not found")
	}
	return target, source, nil
}

func (s *postgresStore) CreateAdminChannelAgent(req adminChannelCreateMutation) (adminChannelAgent, adminUser, error) {
	_ = req
	return adminChannelAgent{}, adminUser{}, errors.New("legacy channel-agent writes are disabled; use customer 360 identity management")
}

func (s *postgresStore) UpdateAdminChannelAgent(id string, req adminChannelMutation) (adminChannelAgent, error) {
	_, _ = id, req
	return adminChannelAgent{}, errors.New("legacy channel-agent writes are disabled; use customer 360 identity management")
}

func (s *postgresStore) UpdateAdminPlan(id string, req adminPlanMutation) (adminPlan, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return adminPlan{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var item adminPlan
	if err := tx.QueryRowContext(ctx, `select raw from xz_plans where id = $1 for update`, id).Scan(rawScanner(&item)); err != nil {
		return adminPlan{}, err
	}
	var managed bool
	if err := tx.QueryRowContext(ctx, `
		select exists(
			select 1 from xz_plan_versions
			where plan_id=$1 and business_type in('MEMBER','AGENT')
		)
	`, id).Scan(&managed); err != nil {
		return adminPlan{}, err
	}
	if managed {
		return adminPlan{}, managedPlanRequiresVersionError()
	}
	if req.Name != "" {
		item.Name = req.Name
	}
	if req.PriceCents != nil {
		item.Price = *req.PriceCents
		item.PriceCents = *req.PriceCents
	}
	if req.GrantPoints != nil {
		item.Points = *req.GrantPoints
		item.GrantPoints = *req.GrantPoints
	}
	if req.DurationDays != nil {
		item.DurationDays = *req.DurationDays
	}
	if req.Concurrency != nil {
		item.Concurrency = *req.Concurrency
	}
	if req.Active != nil {
		item.Active = *req.Active
	}
	if req.Entitlements != nil {
		item.Entitlements = req.Entitlements
	}
	if err := insertPlan(ctx, tx, item); err != nil {
		return adminPlan{}, err
	}
	if err := insertAuditLog(ctx, tx, "", "", "plans.update", "plan", item.ID, "", "", 200, map[string]any{
		"priceCents": item.PriceCents, "grantPoints": item.GrantPoints, "durationDays": item.DurationDays,
		"concurrency": item.Concurrency, "active": item.Active,
	}); err != nil {
		return adminPlan{}, err
	}
	return item, tx.Commit()
}

func (s *postgresStore) UpdateAdminProduct(id string, req adminProductMutation) (adminProduct, error) {
	data, err := s.AdminData()
	if err != nil {
		return adminProduct{}, err
	}
	for _, item := range productsWithUsage(data) {
		if item.ID == id {
			if req.Name != "" {
				item.Name = req.Name
			}
			if req.Type != "" {
				item.Type = req.Type
			}
			if req.Status != "" {
				item.Status = req.Status
			}
			if len(req.Entitlements) > 0 {
				item.Entitlements = req.Entitlements
			}
			return item, nil
		}
	}
	return adminProduct{}, fmt.Errorf("product not found: %s", id)
}

func (s *postgresStore) UpdateAdminDeliveryProject(id string, req adminDeliveryMutation) (map[string]any, error) {
	return map[string]any{"id": id, "status": req.Status, "progress": req.Progress}, nil
}

func (s *postgresStore) getSystemSettings(ctx context.Context) (adminSystemSettings, error) {
	settings := defaultSystemSettings()
	var item adminSystemSettings
	err := s.db.QueryRowContext(ctx, `select raw from xz_system_settings where id = 'default'`).Scan(rawScanner(&item))
	if errors.Is(err, sql.ErrNoRows) {
		return settings, nil
	}
	if err != nil {
		return settings, err
	}
	return mergeSystemSettings(settings, item), nil
}

func (s *postgresStore) applyAICapabilityConfig(ctx context.Context, data adminPlatformData) (adminPlatformData, error) {
	var cfg adminAICapabilityConfig
	err := s.db.QueryRowContext(ctx, `select raw from xz_system_settings where id = $1`, aiCapabilitySettingsID).Scan(rawScanner(&cfg))
	if errors.Is(err, sql.ErrNoRows) {
		return normalizeAICapabilityDefaults(data), nil
	}
	if err != nil {
		return data, err
	}
	if len(cfg.AIModules) > 0 {
		data.AIModules = cfg.AIModules
	}
	if len(cfg.AIModels) > 0 {
		data.AIModels = cfg.AIModels
	}
	if len(cfg.AIParameterSchemas) > 0 {
		data.AIParameterSchemas = cfg.AIParameterSchemas
	}
	if len(cfg.TenantModuleLimits) > 0 {
		data.TenantModuleLimits = cfg.TenantModuleLimits
	}
	if len(cfg.BillingRules) > 0 {
		data.BillingRules = cfg.BillingRules
	}
	return normalizeAICapabilityDefaults(data), nil
}

func (s *postgresStore) aiCapabilityAdminData(ctx context.Context) (adminPlatformData, error) {
	data, err := s.applyAICapabilityConfig(ctx, seedAdminData())
	if err != nil {
		return data, err
	}
	data.BillingRuleVersions, err = s.listBillingRuleVersionsContext(ctx)
	if err != nil {
		return data, err
	}
	data.ProviderCosts, err = s.listProviderCostsContext(ctx)
	if err != nil {
		return data, err
	}
	return applyPublishedBillingRulesV1(data), nil
}

type contextSQLExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func saveAICapabilityAdminDataWithExecutor(ctx context.Context, executor contextSQLExecer, data adminPlatformData) error {
	data = normalizeAICapabilityDefaults(data)
	cfg := adminAICapabilityConfig{
		AIModules:          data.AIModules,
		AIModels:           data.AIModels,
		AIParameterSchemas: data.AIParameterSchemas,
		TenantModuleLimits: data.TenantModuleLimits,
		BillingRules:       data.BillingRules,
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := executor.ExecContext(ctx, `
		insert into xz_system_settings (id, raw, updated_at)
		values ($1, $2::jsonb, $3)
		on conflict (id) do update set raw=excluded.raw, updated_at=excluded.updated_at
	`, aiCapabilitySettingsID, jsonProjection(cfg), now)
	return err
}

func (s *postgresStore) saveAICapabilityAdminData(ctx context.Context, data adminPlatformData) error {
	return saveAICapabilityAdminDataWithExecutor(ctx, s.db, data)
}

func (s *postgresStore) updateAICapabilityAdminData(mutator func(*adminPlatformData) error) error {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return err
	}
	data, err := s.aiCapabilityAdminData(ctx)
	if err != nil {
		return err
	}
	data = normalizeAICapabilityDefaults(data)
	if err := mutator(&data); err != nil {
		return err
	}
	return s.saveAICapabilityAdminData(ctx, data)
}

func (s *postgresStore) UpdateAdminSystemSettings(req adminSystemMutation) (adminSystemSettings, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminSystemSettings{}, err
	}
	current, err := s.getSystemSettings(ctx)
	if err != nil {
		return adminSystemSettings{}, err
	}
	settings := current
	if req.Brand.Name != "" {
		settings.Brand = req.Brand
	}
	if len(req.Payments) > 0 {
		settings.Payments = req.Payments
	}
	if len(req.Permissions) > 0 {
		settings.Permissions = req.Permissions
	}
	if len(req.APIGateway) > 0 {
		settings.APIGateway = mergeMap(settings.APIGateway, req.APIGateway)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.db.ExecContext(ctx, `
		insert into xz_system_settings (id, raw, updated_at)
		values ('default', $1::jsonb, $2)
		on conflict (id) do update set raw=excluded.raw, updated_at=excluded.updated_at
	`, jsonProjection(settings), now)
	return settings, err
}

func (s *postgresStore) CreateAdminAPIChannel(req adminAPIChannelMutation) (adminAPIChannel, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminAPIChannel{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return adminAPIChannel{}, err
	}
	defer func() { _ = tx.Rollback() }()
	item := adminAPIChannel{
		ID:                      "channel_api_" + strconv.FormatInt(time.Now().UnixNano(), 10),
		Name:                    fallback(req.Name, "API"),
		BaseURL:                 fallback(req.BaseURL, "https://example.com/v1"),
		Protocol:                fallback(req.Protocol, "openai"),
		ImageRequestMode:        fallback(req.ImageRequestMode, "openai"),
		ImageGenerationEndpoint: fallback(req.ImageGenerationEndpoint, "/v1/images/generations"),
		ImageEditEndpoint:       fallback(req.ImageEditEndpoint, "/v1/images/edits"),
		VideoGenerationEndpoint: req.VideoGenerationEndpoint,
		FetchModelsPath:         fallback(req.FetchModelsPath, "/models"),
		APIKeyEnv:               req.APIKeyEnv,
		ComfyInstances:          req.ComfyInstances,
		Notes:                   req.Notes,
		Primary:                 req.Primary,
		Status:                  fallback(req.Status, "CONFIGURABLE"),
		Priority:                req.Priority,
		Models:                  req.Models,
	}
	if item.Priority == 0 {
		item.Priority = 100
	}
	if len(item.Models) == 0 {
		item.Models = []string{"gpt-image-2", "mock-standard"}
	}
	if err := insertAPIChannel(ctx, tx, item); err != nil {
		return adminAPIChannel{}, err
	}
	return item, tx.Commit()
}

func (s *postgresStore) UpdateAdminAPIChannel(id string, req adminAPIChannelMutation) (adminAPIChannel, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminAPIChannel{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return adminAPIChannel{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var item adminAPIChannel
	if err := tx.QueryRowContext(ctx, `select raw from xz_api_channels where id = $1 for update`, id).Scan(rawScanner(&item)); err != nil {
		return adminAPIChannel{}, err
	}
	if req.Name != "" {
		item.Name = req.Name
	}
	if req.BaseURL != "" {
		item.BaseURL = req.BaseURL
	}
	if req.Protocol != "" {
		item.Protocol = req.Protocol
	}
	if req.ImageRequestMode != "" {
		item.ImageRequestMode = req.ImageRequestMode
	}
	if req.ImageGenerationEndpoint != "" {
		item.ImageGenerationEndpoint = req.ImageGenerationEndpoint
	}
	if req.ImageEditEndpoint != "" {
		item.ImageEditEndpoint = req.ImageEditEndpoint
	}
	item.VideoGenerationEndpoint = req.VideoGenerationEndpoint
	if req.FetchModelsPath != "" {
		item.FetchModelsPath = req.FetchModelsPath
	}
	if req.APIKeyEnv != "" {
		item.APIKeyEnv = req.APIKeyEnv
	}
	if req.ComfyInstances != nil {
		item.ComfyInstances = req.ComfyInstances
	}
	if req.Notes != "" {
		item.Notes = req.Notes
	}
	item.Primary = req.Primary
	if req.Status != "" {
		item.Status = req.Status
	}
	if req.Priority > 0 {
		item.Priority = req.Priority
	}
	if req.Models != nil {
		item.Models = req.Models
	}
	if err := insertAPIChannel(ctx, tx, item); err != nil {
		return adminAPIChannel{}, err
	}
	return item, tx.Commit()
}

func (s *postgresStore) MergeAdminAPIChannelModels(id string, discovered []string) (adminAPIChannel, []string, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminAPIChannel{}, nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return adminAPIChannel{}, nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var item adminAPIChannel
	if err := tx.QueryRowContext(ctx, `select raw from xz_api_channels where id = $1 for update`, id).Scan(rawScanner(&item)); err != nil {
		return adminAPIChannel{}, nil, err
	}
	merged, added := mergeAPIChannelModels(item.Models, discovered)
	item.Models = merged
	if err := insertAPIChannel(ctx, tx, item); err != nil {
		return adminAPIChannel{}, nil, err
	}
	if err := tx.Commit(); err != nil {
		return adminAPIChannel{}, nil, err
	}
	return item, added, nil
}

func (s *postgresStore) TestAdminAPIChannel(id string, req adminAPIChannelTestRequest) (map[string]any, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return nil, err
	}
	channels, err := s.listAPIChannels(ctx)
	if err != nil {
		return nil, err
	}
	for _, item := range channels {
		if item.ID != id {
			continue
		}
		if strings.TrimSpace(req.APIKey) == "" {
			keys, _ := s.listAPIKeys(ctx)
			req.APIKey = savedAPIKeyForChannel(keys, item)
		}
		return testAPIChannelConnection(item, req), nil
	}
	return nil, fmt.Errorf("api channel not found: %s", id)
}

func (s *postgresStore) UpdateAdminAPIModel(id string, req adminAPIModelMutation) (adminAPIModel, error) {
	return adminAPIModel{ID: id, Name: req.Name, Capability: req.Capability, BillingMode: req.BillingMode, FixedQuota: req.FixedQuota, ModelRatio: req.ModelRatio, CompletionRatio: req.CompletionRatio, Status: req.Status}, nil
}

func (s *postgresStore) CreateAdminAPIKey(req adminAPIKeyMutation) (adminAPIKey, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminAPIKey{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return adminAPIKey{}, err
	}
	defer func() { _ = tx.Rollback() }()
	secret := strings.TrimSpace(firstNonEmptyString(req.Secret, req.APIKey))
	item := adminAPIKey{ID: "key_" + strconv.FormatInt(time.Now().UnixNano(), 10), Customer: fallback(req.Customer, "API"), Prefix: apiKeyPrefix(secret, time.Now().Nanosecond()), Secret: secret, Status: fallback(req.Status, "ACTIVE"), Models: req.Models, QuotaLimit: req.QuotaLimit}
	if err := insertAPIKey(ctx, tx, item); err != nil {
		return adminAPIKey{}, err
	}
	return item, tx.Commit()
}

func (s *postgresStore) UpdateAdminAPIKey(id string, req adminAPIKeyMutation) (adminAPIKey, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminAPIKey{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return adminAPIKey{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var item adminAPIKey
	if err := tx.QueryRowContext(ctx, `select raw from xz_api_keys where id = $1 for update`, id).Scan(rawScanner(&item)); err != nil {
		return adminAPIKey{}, err
	}
	if req.Customer != "" {
		item.Customer = req.Customer
	}
	if req.Status != "" {
		item.Status = req.Status
	}
	if secret := strings.TrimSpace(firstNonEmptyString(req.Secret, req.APIKey)); secret != "" {
		item.Secret = secret
		item.Prefix = apiKeyPrefix(secret, time.Now().Nanosecond())
	}
	if req.Models != nil {
		item.Models = req.Models
	}
	if req.QuotaLimit > 0 {
		item.QuotaLimit = req.QuotaLimit
	}
	if err := insertAPIKey(ctx, tx, item); err != nil {
		return adminAPIKey{}, err
	}
	return item, tx.Commit()
}

func (s *postgresStore) UpdateAdminCustomerGroup(id string, req adminCustomerGroupMutation) (adminCustomerGroup, error) {
	return adminCustomerGroup{ID: id, Name: req.Name, Ratio: req.Ratio, Models: req.Models, Description: req.Description}, nil
}

func (s *postgresStore) UpdateAdminAIModule(code string, req adminAIModuleMutation) (adminAIModule, error) {
	var updated adminAIModule
	err := s.updateAICapabilityAdminData(func(data *adminPlatformData) error {
		code = canonicalModuleCode(code)
		for i := range data.AIModules {
			if canonicalModuleCode(data.AIModules[i].ModuleCode) != code {
				continue
			}
			if req.Name != "" {
				data.AIModules[i].Name = req.Name
			}
			if req.Description != "" {
				data.AIModules[i].Description = req.Description
			}
			if req.Status != "" {
				data.AIModules[i].Status = strings.ToUpper(strings.TrimSpace(req.Status))
			}
			if req.OpenTenantIDs != nil {
				data.AIModules[i].OpenTenantIDs = req.OpenTenantIDs
			}
			if req.OpenPackageIDs != nil {
				data.AIModules[i].OpenPackageIDs = req.OpenPackageIDs
			}
			if req.BoundModels != nil {
				data.AIModules[i].BoundModels = req.BoundModels
			}
			if req.DefaultSchemaID != "" {
				data.AIModules[i].DefaultSchemaID = req.DefaultSchemaID
			}
			if req.AllowAgents != nil {
				data.AIModules[i].AllowAgents = *req.AllowAgents
			}
			if req.AllowEndUsers != nil {
				data.AIModules[i].AllowEndUsers = *req.AllowEndUsers
			}
			if req.Config != nil {
				data.AIModules[i].Config = req.Config
			}
			data.AIModules[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			updated = data.AIModules[i]
			return nil
		}
		return fmt.Errorf("ai module not found: %s", code)
	})
	return updated, err
}

func (s *postgresStore) CreateAdminAIModel(req adminAIModelMutation) (adminAIModel, error) {
	var created adminAIModel
	err := s.updateAICapabilityAdminData(func(data *adminPlatformData) error {
		modelName := strings.TrimSpace(req.ModelName)
		if modelName == "" {
			return errors.New("model_name is required")
		}
		moduleCode := canonicalModuleCode(req.ModuleCode)
		if moduleCode == "" {
			moduleCode = moduleImageGeneration
		}
		for _, item := range data.AIModels {
			if canonicalModuleCode(item.ModuleCode) == moduleCode && strings.EqualFold(strings.TrimSpace(item.ModelName), modelName) {
				return fmt.Errorf("ai model already exists: %s", modelName)
			}
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		fallbackModel := ""
		if req.FallbackModel != nil {
			fallbackModel = strings.TrimSpace(*req.FallbackModel)
		}
		sortWeight := req.SortWeight
		if sortWeight <= 0 {
			sortWeight = len(data.AIModels)*10 + 10
		}
		modelType := strings.TrimSpace(req.ModelType)
		if modelType == "" {
			modelType = defaultAIModelTypeForModule(moduleCode)
		}
		provider := strings.TrimSpace(req.Provider)
		if provider == "" {
			provider = "NewAPI"
		}
		status := strings.ToUpper(strings.TrimSpace(req.Status))
		if status == "" {
			status = "ACTIVE"
		}
		created = adminAIModel{
			ID:                  uniqueAdminID("ai_model", aiModelIDs(data.AIModels)),
			ModelName:           modelName,
			ModelType:           modelType,
			Provider:            provider,
			ChannelID:           strings.TrimSpace(stringPointerValue(req.ChannelID)),
			CapabilityCode:      uniqueNonEmptyStrings(req.CapabilityCode),
			ModuleCode:          moduleCode,
			Status:              status,
			FallbackModel:       fallbackModel,
			SortWeight:          sortWeight,
			AllowFallbackSwitch: req.AllowFallbackSwitch != nil && *req.AllowFallbackSwitch,
			CreatedAt:           now,
			UpdatedAt:           now,
		}
		applyAIModelComplianceMutation(&created, req)
		if err := validateAIModelMiniProgramEnable(created); err != nil {
			return err
		}
		if len(created.CapabilityCode) == 0 {
			created.CapabilityCode = defaultAICapabilitiesForModule(moduleCode)
		}
		if err := applyAIModelVideoCapabilitiesMutation(&created, req); err != nil {
			return err
		}
		data.AIModels = append(data.AIModels, created)
		bindAIModelToModule(data, moduleCode, modelName)
		return nil
	})
	return created, err
}

func (s *postgresStore) UpdateAdminAIModel(id string, req adminAIModelMutation) (adminAIModel, error) {
	var updated adminAIModel
	err := s.updateAICapabilityAdminData(func(data *adminPlatformData) error {
		for i := range data.AIModels {
			if data.AIModels[i].ID != id {
				continue
			}
			oldModelName := data.AIModels[i].ModelName
			if req.ModelName != "" {
				nextModelName := strings.TrimSpace(req.ModelName)
				data.AIModels[i].ModelName = nextModelName
				for moduleIndex := range data.AIModules {
					for modelIndex, modelName := range data.AIModules[moduleIndex].BoundModels {
						if strings.EqualFold(strings.TrimSpace(modelName), oldModelName) {
							data.AIModules[moduleIndex].BoundModels[modelIndex] = nextModelName
						}
					}
				}
			}
			if req.ModelType != "" {
				data.AIModels[i].ModelType = req.ModelType
			}
			if req.Provider != "" {
				data.AIModels[i].Provider = req.Provider
			}
			if req.ChannelID != nil {
				data.AIModels[i].ChannelID = strings.TrimSpace(*req.ChannelID)
			}
			if req.CapabilityCode != nil {
				data.AIModels[i].CapabilityCode = req.CapabilityCode
			}
			if req.ModuleCode != "" {
				data.AIModels[i].ModuleCode = canonicalModuleCode(req.ModuleCode)
			}
			if req.Status != "" {
				data.AIModels[i].Status = strings.ToUpper(strings.TrimSpace(req.Status))
			}
			if req.FallbackModel != nil {
				data.AIModels[i].FallbackModel = strings.TrimSpace(*req.FallbackModel)
			}
			if req.SortWeight > 0 {
				data.AIModels[i].SortWeight = req.SortWeight
			}
			if req.AllowFallbackSwitch != nil {
				data.AIModels[i].AllowFallbackSwitch = *req.AllowFallbackSwitch
			}
			if err := applyAIModelVideoCapabilitiesMutation(&data.AIModels[i], req); err != nil {
				return err
			}
			applyAIModelComplianceMutation(&data.AIModels[i], req)
			if err := validateAIModelMiniProgramEnable(data.AIModels[i]); err != nil {
				return err
			}
			data.AIModels[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			updated = data.AIModels[i]
			return nil
		}
		return fmt.Errorf("ai model not found: %s", id)
	})
	return updated, err
}

func (s *postgresStore) UpdateAdminAIParameterSchema(id string, req adminAIParameterSchemaMutation) (adminAIParameterSchema, error) {
	var updated adminAIParameterSchema
	err := s.updateAICapabilityAdminData(func(data *adminPlatformData) error {
		for i := range data.AIParameterSchemas {
			if data.AIParameterSchemas[i].ID != id {
				continue
			}
			if req.SchemaJSON.Fields != nil {
				data.AIParameterSchemas[i].SchemaJSON = req.SchemaJSON
			}
			if req.Status != "" {
				data.AIParameterSchemas[i].Status = strings.ToUpper(strings.TrimSpace(req.Status))
			}
			data.AIParameterSchemas[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			updated = data.AIParameterSchemas[i]
			return nil
		}
		return fmt.Errorf("ai parameter schema not found: %s", id)
	})
	return updated, err
}

func (s *postgresStore) UpdateAdminTenantModuleLimit(id string, req adminTenantModuleLimitMutation) (adminTenantModuleLimit, error) {
	var updated adminTenantModuleLimit
	err := s.updateAICapabilityAdminData(func(data *adminPlatformData) error {
		for i := range data.TenantModuleLimits {
			if data.TenantModuleLimits[i].ID != id {
				continue
			}
			if req.TenantID != "" {
				data.TenantModuleLimits[i].TenantID = req.TenantID
			}
			if req.AgentID != "" {
				data.TenantModuleLimits[i].AgentID = req.AgentID
			}
			if req.PackageID != "" {
				data.TenantModuleLimits[i].PackageID = req.PackageID
			}
			if req.ModelName != "" {
				data.TenantModuleLimits[i].ModelName = req.ModelName
			}
			if req.LimitJSON != nil {
				data.TenantModuleLimits[i].LimitJSON = req.LimitJSON
			}
			if req.Status != "" {
				data.TenantModuleLimits[i].Status = strings.ToUpper(strings.TrimSpace(req.Status))
			}
			data.TenantModuleLimits[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			updated = data.TenantModuleLimits[i]
			return nil
		}
		return fmt.Errorf("tenant module limit not found: %s", id)
	})
	return updated, err
}

func (s *postgresStore) UpdateAdminPlanCapabilities(planID string, req adminPlanCapabilitiesMutation) error {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var lockedPlanID string
	if err := tx.QueryRowContext(ctx, `select id from xz_plans where id=$1 for update`, strings.TrimSpace(planID)).Scan(&lockedPlanID); err != nil {
		return err
	}
	var managed bool
	if err := tx.QueryRowContext(ctx, `
		select exists(
			select 1 from xz_plan_versions
			where plan_id=$1 and business_type in('MEMBER','AGENT')
		)
	`, lockedPlanID).Scan(&managed); err != nil {
		return err
	}
	if managed {
		return managedPlanRequiresVersionError()
	}

	data, err := s.aiCapabilityAdminData(ctx)
	if err != nil {
		return err
	}
	data = normalizeAICapabilityDefaults(data)
	if err := applyAdminPlanCapabilities(&data, lockedPlanID, req); err != nil {
		return err
	}
	if err := saveAICapabilityAdminDataWithExecutor(ctx, tx, data); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *postgresStore) UpdateAdminBillingRule(id string, req adminBillingRuleMutation) (adminBillingRule, error) {
	draft, err := s.createBillingRuleDraft(id, req)
	if err != nil {
		return adminBillingRule{}, err
	}
	return billingRuleVersionProjection(draft), nil
}

func (s *postgresStore) UpdateMarketingCommissionRule(id string, req adminCommissionRuleMutation) (adminCommissionRule, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminCommissionRule{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return adminCommissionRule{}, err
	}
	defer func() { _ = tx.Rollback() }()
	rule, err := marketingCommissionRuleByIDForUpdate(ctx, tx, id)
	if err != nil {
		return adminCommissionRule{}, err
	}
	rule = applyCommissionRuleMutation(rule, req)
	if err := upsertMarketingCommissionRule(ctx, tx, rule); err != nil {
		return adminCommissionRule{}, err
	}
	if err := insertAuditLog(ctx, tx, "", "", "marketing.commission_rule.update", "commission_rule", rule.ID, "", "", 200, map[string]any{"rate": rule.Rate, "status": rule.Status}); err != nil {
		return adminCommissionRule{}, err
	}
	return rule, tx.Commit()
}

type jsonRawScanner struct {
	target any
}

func rawScanner(target any) sql.Scanner {
	return jsonRawScanner{target: target}
}

type sqlRowScanner interface {
	Scan(dest ...any) error
}

func scanMarketingCommissionRule(row sqlRowScanner) (adminCommissionRule, error) {
	var item adminCommissionRule
	var metadataRaw []byte
	var createdAt time.Time
	var updatedAt time.Time
	err := row.Scan(&item.ID, &item.Name, &item.OrderType, &item.EarnerRole, &item.RelationDepth, &item.FixedAmountCents, &item.Rate, &item.MaxTotalRate, &item.Status, &metadataRaw, &createdAt, &updatedAt)
	if err != nil {
		return adminCommissionRule{}, err
	}
	if len(metadataRaw) > 0 {
		_ = json.Unmarshal(metadataRaw, &item.Metadata)
	}
	if item.Metadata == nil {
		item.Metadata = map[string]any{}
	}
	item.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
	item.UpdatedAt = updatedAt.UTC().Format(time.RFC3339Nano)
	return item, nil
}

func (s jsonRawScanner) Scan(value any) error {
	var raw []byte
	switch typed := value.(type) {
	case []byte:
		raw = typed
	case string:
		raw = []byte(typed)
	default:
		return fmt.Errorf("unsupported json raw type %T", value)
	}
	return json.Unmarshal(raw, s.target)
}

func scanRawJSON(rows *sql.Rows, target any) error {
	return rows.Scan(rawScanner(target))
}

func withPlanDefaults(plans []adminPlan) []adminPlan {
	if len(plans) == 0 {
		return canonicalBillingPlans()
	}
	return mergeCanonicalPlans(plans)
}

func stringBoolMapKeys(items map[string]bool) []string {
	keys := make([]string, 0, len(items))
	for key, ok := range items {
		key = strings.TrimSpace(key)
		if ok && key != "" {
			keys = append(keys, key)
		}
	}
	return keys
}

func postgresTextInCondition(column string, values []string) (string, []any) {
	args := make([]any, 0, len(values))
	placeholders := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		args = append(args, value)
		placeholders = append(placeholders, "$"+strconv.Itoa(len(args)))
	}
	if len(args) == 0 {
		return "false", nil
	}
	return column + " in (" + strings.Join(placeholders, ",") + ")", args
}

func (s *postgresStore) countRowsForUsers(ctx context.Context, table string, userIDs []string) (int, error) {
	switch table {
	case "xz_generation_tasks", "xz_assets":
	default:
		return 0, fmt.Errorf("unsupported user count table: %s", table)
	}
	where, args := postgresTextInCondition("user_id", userIDs)
	query := `select count(*) from ` + table + ` where ` + where
	if table == "xz_assets" {
		query += ` and deleted_at is null`
	}
	var count int
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *postgresStore) listUsersBasic(ctx context.Context) ([]adminUser, error) {
	rows, err := s.db.QueryContext(ctx, `select raw from xz_users order by created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []adminUser{}
	for rows.Next() {
		var item adminUser
		if err := scanRawJSON(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) listUsers(ctx context.Context) ([]adminUser, error) {
	rows, err := s.db.QueryContext(ctx, `select raw from xz_users order by created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []adminUser{}
	for rows.Next() {
		var item adminUser
		if err := scanRawJSON(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	routesByUser, err := s.listUserModelRoutes(ctx)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if routes := routesByUser[items[i].ID]; len(routes) > 0 {
			items[i].ModelRoutes = mergeUserModelRoutes(items[i].ModelRoutes, routes)
		}
	}
	return items, nil
}

func (s *postgresStore) listUserModelRoutes(ctx context.Context) (map[string][]adminUserModelRoute, error) {
	rows, err := s.db.QueryContext(ctx, `select user_id, raw from xz_user_model_routes order by updated_at desc, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := map[string][]adminUserModelRoute{}
	for rows.Next() {
		var userID string
		var route adminUserModelRoute
		if err := rows.Scan(&userID, rawScanner(&route)); err != nil {
			return nil, err
		}
		items[userID] = append(items[userID], route)
	}
	return items, rows.Err()
}

func mergeUserModelRoutes(existing []adminUserModelRoute, projected []adminUserModelRoute) []adminUserModelRoute {
	if len(projected) == 0 {
		return existing
	}
	merged := make([]adminUserModelRoute, 0, len(projected)+len(existing))
	seen := map[string]bool{}
	for _, route := range projected {
		key := route.ID
		if key == "" {
			key = strings.ToLower(route.Provider + "|" + route.ChannelID + "|" + route.GroupName + "|" + strings.Join(route.Models, ","))
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		merged = append(merged, route)
	}
	for _, route := range existing {
		key := route.ID
		if key == "" {
			key = strings.ToLower(route.Provider + "|" + route.ChannelID + "|" + route.GroupName + "|" + strings.Join(route.Models, ","))
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		merged = append(merged, route)
	}
	return merged
}

func (s *postgresStore) listPlans(ctx context.Context) ([]adminPlan, error) {
	rows, err := s.db.QueryContext(ctx, `select raw from xz_plans order by price_cents, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []adminPlan{}
	for rows.Next() {
		var item adminPlan
		if err := scanRawJSON(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) listPointAccounts(ctx context.Context) ([]adminPointAccount, error) {
	rows, err := s.db.QueryContext(ctx, `select raw from xz_point_accounts order by id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []adminPointAccount{}
	for rows.Next() {
		var item adminPointAccount
		if err := scanRawJSON(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) listPointAccountsForUsers(ctx context.Context, userIDs []string) ([]adminPointAccount, error) {
	where, args := postgresTextInCondition("user_id", userIDs)
	rows, err := s.db.QueryContext(ctx, `select raw from xz_point_accounts where `+where+` order by id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []adminPointAccount{}
	for rows.Next() {
		var item adminPointAccount
		if err := scanRawJSON(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) listTokenRecords(ctx context.Context) ([]adminTokenRecord, error) {
	rows, err := s.db.QueryContext(ctx, `select raw from xz_token_records order by created_at desc, id desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []adminTokenRecord{}
	for rows.Next() {
		var item adminTokenRecord
		if err := scanRawJSON(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) listTokenRecordsForUser(ctx context.Context, userID string) ([]adminTokenRecord, error) {
	rows, err := s.db.QueryContext(ctx, `select raw from xz_token_records where user_id = $1 order by created_at desc, id desc`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []adminTokenRecord{}
	for rows.Next() {
		var item adminTokenRecord
		if err := scanRawJSON(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) listOrders(ctx context.Context) ([]adminOrder, error) {
	rows, err := s.db.QueryContext(ctx, `select raw from xz_orders order by created_at desc, id desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []adminOrder{}
	for rows.Next() {
		var item adminOrder
		if err := scanRawJSON(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) listOrdersForUser(ctx context.Context, userID string) ([]adminOrder, error) {
	rows, err := s.db.QueryContext(ctx, `select raw from xz_orders where user_id = $1 order by created_at desc, id desc`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []adminOrder{}
	for rows.Next() {
		var item adminOrder
		if err := scanRawJSON(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) listOrdersForUsers(ctx context.Context, userIDs []string) ([]adminOrder, error) {
	where, args := postgresTextInCondition("user_id", userIDs)
	rows, err := s.db.QueryContext(ctx, `select raw from xz_orders where `+where+` order by created_at desc, id desc`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []adminOrder{}
	for rows.Next() {
		var item adminOrder
		if err := scanRawJSON(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) listChannelAgents(ctx context.Context) ([]adminChannelAgent, error) {
	rows, err := s.db.QueryContext(ctx, `select raw from xz_channel_agents order by level, created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []adminChannelAgent{}
	for rows.Next() {
		var item adminChannelAgent
		if err := scanRawJSON(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) listOperationCenters(ctx context.Context) ([]adminOperationCenter, error) {
	rows, err := s.db.QueryContext(ctx, `select raw from xz_operation_centers order by created_at desc, id desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []adminOperationCenter{}
	for rows.Next() {
		var item adminOperationCenter
		if err := scanRawJSON(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) listCustomerRelations(ctx context.Context) ([]adminCustomerRelation, error) {
	rows, err := s.db.QueryContext(ctx, `
		select id, customer_user_id, coalesce(direct_agent_id, ''),
		       coalesce(parent_agent_id, ''), coalesce(operation_center_id, ''),
		       bind_type, bind_start_at, status, created_at, updated_at
		from xz_customer_relations
		where upper(coalesce(status, 'ACTIVE')) <> 'DELETED'
		order by bind_start_at desc, created_at desc, id desc
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []adminCustomerRelation{}
	for rows.Next() {
		var item adminCustomerRelation
		var bindStartAt, createdAt, updatedAt time.Time
		if err := rows.Scan(
			&item.ID, &item.CustomerUserID, &item.DirectAgentID, &item.ParentAgentID,
			&item.OperationCenterID, &item.BindType, &bindStartAt, &item.Status, &createdAt, &updatedAt,
		); err != nil {
			return nil, err
		}
		item.BindStartAt = bindStartAt.UTC().Format(time.RFC3339Nano)
		item.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
		item.UpdatedAt = updatedAt.UTC().Format(time.RFC3339Nano)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) listCommissions(ctx context.Context) ([]adminCommission, error) {
	rows, err := s.db.QueryContext(ctx, `select raw from xz_commissions order by created_at desc, id desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []adminCommission{}
	for rows.Next() {
		var item adminCommission
		if err := scanRawJSON(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) listCommissionsForAgent(ctx context.Context, agentID string) ([]adminCommission, error) {
	rows, err := s.db.QueryContext(ctx, `select raw from xz_commissions where agent_id = $1 order by created_at desc, id desc`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []adminCommission{}
	for rows.Next() {
		var item adminCommission
		if err := scanRawJSON(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) listMarketingCommissionRules(ctx context.Context) ([]adminCommissionRule, error) {
	rows, err := s.db.QueryContext(ctx, `
		select id, name, order_type, earner_role, relation_depth, fixed_amount_cents, rate, max_total_rate, status, metadata, created_at, updated_at
		from xz_marketing_commission_rules
		order by order_type, relation_depth, id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []adminCommissionRule{}
	for rows.Next() {
		item, err := scanMarketingCommissionRule(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) listBillingEvents(ctx context.Context) ([]adminBillingEvent, error) {
	rows, err := s.db.QueryContext(ctx, `select raw from xz_billing_events order by occurred_at desc, id desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []adminBillingEvent{}
	for rows.Next() {
		var item adminBillingEvent
		if err := scanRawJSON(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) listBillingEventsForUser(ctx context.Context, userID string) ([]adminBillingEvent, error) {
	rows, err := s.db.QueryContext(ctx, `select raw from xz_billing_events where user_id = $1 order by occurred_at desc, id desc`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []adminBillingEvent{}
	for rows.Next() {
		var item adminBillingEvent
		if err := scanRawJSON(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) listBillingEventsForUsers(ctx context.Context, userIDs []string, limit int) ([]adminBillingEvent, error) {
	where, args := postgresTextInCondition("user_id", userIDs)
	query := `select raw from xz_billing_events where ` + where + ` order by occurred_at desc, id desc`
	if limit > 0 {
		args = append(args, limit)
		query += ` limit $` + strconv.Itoa(len(args))
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []adminBillingEvent{}
	for rows.Next() {
		var item adminBillingEvent
		if err := scanRawJSON(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) listWithdrawals(ctx context.Context) ([]adminWithdrawal, error) {
	rows, err := s.db.QueryContext(ctx, `select raw from xz_withdrawals order by created_at desc, id desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []adminWithdrawal{}
	for rows.Next() {
		var item adminWithdrawal
		if err := scanRawJSON(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) listWithdrawalsForAgent(ctx context.Context, agentID string) ([]adminWithdrawal, error) {
	rows, err := s.db.QueryContext(ctx, `select raw from xz_withdrawals where agent_id = $1 order by created_at desc, id desc`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []adminWithdrawal{}
	for rows.Next() {
		var item adminWithdrawal
		if err := scanRawJSON(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) listAPIChannels(ctx context.Context) ([]adminAPIChannel, error) {
	rows, err := s.db.QueryContext(ctx, `select raw from xz_api_channels order by priority, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []adminAPIChannel{}
	for rows.Next() {
		var item adminAPIChannel
		if err := scanRawJSON(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) listAPIKeys(ctx context.Context) ([]adminAPIKey, error) {
	rows, err := s.db.QueryContext(ctx, `select raw from xz_api_keys order by id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []adminAPIKey{}
	for rows.Next() {
		var item adminAPIKey
		if err := scanRawJSON(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func insertUser(ctx context.Context, tx *sql.Tx, item adminUser) error {
	_, err := tx.ExecContext(ctx, `
		insert into xz_users (id, email, mobile, wechat_union_id, name, role, member_level, agent_status, operation_center_status, status, password_hash, plan_id, referred_by, subscription_expires_at, created_at, updated_at, raw)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17::jsonb)
		on conflict (id) do update set email=excluded.email, mobile=excluded.mobile, wechat_union_id=excluded.wechat_union_id, name=excluded.name, role=excluded.role, member_level=excluded.member_level, agent_status=excluded.agent_status, operation_center_status=excluded.operation_center_status, status=excluded.status, password_hash=excluded.password_hash, plan_id=excluded.plan_id, referred_by=excluded.referred_by, subscription_expires_at=excluded.subscription_expires_at, created_at=excluded.created_at, updated_at=excluded.updated_at, raw=excluded.raw
	`, item.ID, item.Email, strings.TrimSpace(item.Mobile), strings.TrimSpace(item.WeChatUnionID), item.Name, item.Role, item.MemberLevel, item.AgentStatus, item.OperationCenterStatus, item.Status, item.PasswordHash, item.PlanID, item.ReferredBy, item.SubscriptionExpiresAt, item.CreatedAt, item.UpdatedAt, jsonProjection(item))
	if err != nil {
		return err
	}
	for _, route := range item.ModelRoutes {
		if route.ID == "" {
			continue
		}
		if err := insertUserModelRoute(ctx, tx, item.ID, route); err != nil {
			return err
		}
	}
	return nil
}

func insertUserModelRoute(ctx context.Context, tx *sql.Tx, userID string, route adminUserModelRoute) error {
	_, err := tx.ExecContext(ctx, `
		insert into xz_user_model_routes (id, user_id, provider, channel_id, channel, api_key_id, key_prefix, group_name, models, quota_limit, quota_used, status, updated_at, raw)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11,$12,$13,$14::jsonb)
		on conflict (id) do update set user_id=excluded.user_id, provider=excluded.provider, channel_id=excluded.channel_id, channel=excluded.channel, api_key_id=excluded.api_key_id, key_prefix=excluded.key_prefix, group_name=excluded.group_name, models=excluded.models, quota_limit=excluded.quota_limit, quota_used=excluded.quota_used, status=excluded.status, updated_at=excluded.updated_at, raw=excluded.raw
	`, route.ID, userID, route.Provider, route.ChannelID, route.Channel, route.APIKeyID, route.KeyPrefix, route.GroupName, jsonProjection(route.Models), route.QuotaLimit, route.QuotaUsed, route.Status, route.UpdatedAt, jsonProjection(route))
	return err
}

func insertAuthMergeRequest(ctx context.Context, tx *sql.Tx, item adminAuthMergeRequest) error {
	_, err := tx.ExecContext(ctx, `
		insert into xz_auth_account_merge_requests (id, primary_user_id, secondary_user_id, mobile, wechat_open_id, wechat_union_id, conflict_code, source, status, reason, review_comment, resolved_by, resolved_at, created_at, updated_at, raw)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16::jsonb)
		on conflict (id) do update set primary_user_id=excluded.primary_user_id, secondary_user_id=excluded.secondary_user_id, mobile=excluded.mobile, wechat_open_id=excluded.wechat_open_id, wechat_union_id=excluded.wechat_union_id, conflict_code=excluded.conflict_code, source=excluded.source, status=excluded.status, reason=excluded.reason, review_comment=excluded.review_comment, resolved_by=excluded.resolved_by, resolved_at=excluded.resolved_at, created_at=excluded.created_at, updated_at=excluded.updated_at, raw=excluded.raw
	`, item.ID, item.PrimaryUserID, item.SecondaryUserID, strings.TrimSpace(item.Mobile), strings.TrimSpace(item.WeChatOpenID), strings.TrimSpace(item.WeChatUnionID), item.ConflictCode, item.Source, item.Status, item.Reason, item.ReviewComment, item.ResolvedBy, item.ResolvedAt, item.CreatedAt, item.UpdatedAt, jsonProjection(item))
	return err
}

func insertPlan(ctx context.Context, tx *sql.Tx, item adminPlan) error {
	_, err := tx.ExecContext(ctx, `
		insert into xz_plans (id, code, name, price_cents, grant_points, duration_days, concurrency, active, entitlements, raw)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10::jsonb)
		on conflict (id) do update set code=excluded.code, name=excluded.name, price_cents=excluded.price_cents, grant_points=excluded.grant_points, duration_days=excluded.duration_days, concurrency=excluded.concurrency, active=excluded.active, entitlements=excluded.entitlements, raw=excluded.raw
	`, item.ID, item.Code, item.Name, planPriceCents(item), planGrantPoints(item), item.DurationDays, item.Concurrency, item.Active, jsonProjection(item.Entitlements), jsonProjection(item))
	return err
}

func insertPointAccount(ctx context.Context, tx *sql.Tx, item adminPointAccount) error {
	_, err := tx.ExecContext(ctx, `
		insert into xz_point_accounts (id, user_id, available, frozen, raw)
		values ($1,$2,$3,$4,$5::jsonb)
		on conflict (id) do update set user_id=excluded.user_id, available=excluded.available, frozen=excluded.frozen, raw=excluded.raw
	`, item.ID, item.UserID, item.Available, item.Frozen, jsonProjection(item))
	if err != nil {
		return err
	}
	return upsertUserWalletFromPointAccount(ctx, tx, item)
}

func upsertPointAccountByUser(ctx context.Context, tx *sql.Tx, userID string, available int) error {
	var item adminPointAccount
	err := tx.QueryRowContext(ctx, `select raw from xz_point_accounts where user_id = $1 for update`, userID).Scan(rawScanner(&item))
	if errors.Is(err, sql.ErrNoRows) {
		id, idErr := nextTableID(ctx, tx, "xz_point_accounts", "points")
		if idErr != nil {
			return idErr
		}
		item = adminPointAccount{ID: id, UserID: userID, Available: available}
		if err := insertPointAccount(ctx, tx, item); err != nil {
			return err
		}
		return insertAccountBalanceLedgerV1(ctx, tx, item, "ADJUSTMENT", available, 0, available, "ADMIN_BALANCE", userID+":"+time.Now().UTC().Format(time.RFC3339Nano), "后台初始余额设置")
	}
	if err != nil {
		return err
	}
	before := item.Available
	delta := available - before
	if delta < 0 {
		delta = -delta
	}
	item.Available = available
	if err := insertPointAccount(ctx, tx, item); err != nil {
		return err
	}
	return insertAccountBalanceLedgerV1(ctx, tx, item, "ADJUSTMENT", delta, before, available, "ADMIN_BALANCE", userID+":"+time.Now().UTC().Format(time.RFC3339Nano), "后台余额调整")
}

func insertTokenRecord(ctx context.Context, tx *sql.Tx, item adminTokenRecord) error {
	_, err := tx.ExecContext(ctx, `
		insert into xz_token_records (id, user_id, order_id, change_type, amount, balance_after, remark, created_at, raw)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb)
		on conflict (id) do update set user_id=excluded.user_id, order_id=excluded.order_id, change_type=excluded.change_type, amount=excluded.amount, balance_after=excluded.balance_after, remark=excluded.remark, created_at=excluded.created_at, raw=excluded.raw
	`, item.ID, item.UserID, item.OrderID, item.ChangeType, item.Amount, item.BalanceAfter, item.Remark, item.CreatedAt, jsonProjection(item))
	return err
}

func insertChannelAgent(ctx context.Context, tx *sql.Tx, item adminChannelAgent) error {
	_, err := tx.ExecContext(ctx, `
		insert into xz_channel_agents (id, user_id, parent_id, operation_center_id, level, status, invite_code, join_order_id, join_fee_cents, token_rights_amount, created_at, updated_at, raw)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13::jsonb)
		on conflict (id) do update set user_id=excluded.user_id, parent_id=excluded.parent_id, operation_center_id=excluded.operation_center_id, level=excluded.level, status=excluded.status, invite_code=excluded.invite_code, join_order_id=excluded.join_order_id, join_fee_cents=excluded.join_fee_cents, token_rights_amount=excluded.token_rights_amount, created_at=excluded.created_at, updated_at=excluded.updated_at, raw=excluded.raw
	`, item.ID, item.UserID, item.ParentID, item.OperationCenterID, item.Level, item.Status, item.InviteCode, item.JoinOrderID, item.JoinFeeCents, item.TokenRightsAmount, item.CreatedAt, item.UpdatedAt, jsonProjection(item))
	if err != nil {
		return err
	}
	return upsertAgentProfileFromChannelAgent(ctx, tx, item)
}

func insertOperationCenter(ctx context.Context, tx *sql.Tx, item adminOperationCenter) error {
	_, err := tx.ExecContext(ctx, `
		insert into xz_operation_centers (id, user_id, name, region, invite_code, status, join_order_id, join_fee_cents, approved_at, created_at, updated_at, raw)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::jsonb)
		on conflict (id) do update set user_id=excluded.user_id, name=excluded.name, region=excluded.region, invite_code=excluded.invite_code, status=excluded.status, join_order_id=excluded.join_order_id, join_fee_cents=excluded.join_fee_cents, approved_at=excluded.approved_at, created_at=excluded.created_at, updated_at=excluded.updated_at, raw=excluded.raw
	`, item.ID, item.UserID, item.Name, item.Region, item.InviteCode, item.Status, item.JoinOrderID, item.JoinFeeCents, item.ApprovedAt, item.CreatedAt, item.UpdatedAt, jsonProjection(item))
	return err
}

func insertOrder(ctx context.Context, tx *sql.Tx, item adminOrder) error {
	item.TenantID = firstNonEmptyString(item.TenantID, stringValue(item.PriceSnapshot["tenantId"]))
	if item.PriceSnapshot == nil {
		item.PriceSnapshot = map[string]any{}
	}
	if item.TenantID != "" {
		item.PriceSnapshot["tenantId"] = item.TenantID
	}
	_, err := tx.ExecContext(ctx, `
		insert into xz_orders (id, tenant_id, user_id, buyer_user_id, plan_id, order_type, business_order_type, amount_cents, token_amount, token_grant_amount, token_grant_value_cents, platform_income_cents, direct_agent_id, parent_agent_id, operation_center_id, fulfillment_status, fulfilled_at, status, paid_at, created_at, reward_snapshot, price_snapshot, raw)
		values ($1,nullif($2,''),$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21::jsonb,$22::jsonb,$23::jsonb)
		on conflict (id) do update set tenant_id=coalesce(excluded.tenant_id,xz_orders.tenant_id), user_id=excluded.user_id, buyer_user_id=excluded.buyer_user_id, plan_id=excluded.plan_id, order_type=excluded.order_type, business_order_type=excluded.business_order_type, amount_cents=excluded.amount_cents, token_amount=excluded.token_amount, token_grant_amount=excluded.token_grant_amount, token_grant_value_cents=excluded.token_grant_value_cents, platform_income_cents=excluded.platform_income_cents, direct_agent_id=excluded.direct_agent_id, parent_agent_id=excluded.parent_agent_id, operation_center_id=excluded.operation_center_id, fulfillment_status=excluded.fulfillment_status, fulfilled_at=excluded.fulfilled_at, status=excluded.status, paid_at=excluded.paid_at, created_at=excluded.created_at, reward_snapshot=excluded.reward_snapshot, price_snapshot=excluded.price_snapshot, raw=excluded.raw
	`, item.ID, item.TenantID, item.UserID, firstNonEmptyString(item.BuyerUserID, item.UserID), item.PlanID, item.OrderType, businessOrderTypeFromOrder(item), orderAmount(item), firstNonEmptyInt(item.TokenAmount, item.TokenGrantAmount), item.TokenGrantAmount, intValue(item.PriceSnapshot["tokenGrantValueCents"]), item.PlatformIncomeCents, item.DirectAgentID, item.ParentAgentID, item.OperationCenterID, item.FulfillmentStatus, item.FulfilledAt, item.Status, item.PaidAt, item.CreatedAt, jsonProjection(item.RewardSnapshot), jsonProjection(item.PriceSnapshot), jsonProjection(item))
	return err
}

func insertCommission(ctx context.Context, tx *sql.Tx, item adminCommission) error {
	_, err := tx.ExecContext(ctx, `
		insert into xz_commissions (id, order_id, agent_id, amount_cents, rate, status, rule_snapshot, created_at, raw)
		values ($1,$2,$3,$4,$5,$6,$7::jsonb,$8,$9::jsonb)
		on conflict (id) do update set order_id=excluded.order_id, agent_id=excluded.agent_id, amount_cents=excluded.amount_cents, rate=excluded.rate, status=excluded.status, rule_snapshot=excluded.rule_snapshot, created_at=excluded.created_at, raw=excluded.raw
	`, item.ID, item.OrderID, item.AgentID, item.AmountCents, item.Rate, item.Status, jsonProjection(item.RuleSnapshot), item.CreatedAt, jsonProjection(item))
	if err != nil {
		return err
	}
	return refreshAgentWallet(ctx, tx, item.AgentID)
}

func upsertUserWalletFromPointAccount(ctx context.Context, tx *sql.Tx, item adminPointAccount) error {
	if strings.TrimSpace(item.UserID) == "" {
		return nil
	}
	raw := map[string]any{
		"userId":            item.UserID,
		"tokenBalance":      item.Available,
		"cashBalance":       0,
		"cashBalanceCents":  0,
		"frozenToken":       item.Frozen,
		"totalTokenGranted": item.TotalGranted,
		"totalTokenUsed":    item.TotalUsed,
	}
	_, err := tx.ExecContext(ctx, `
		insert into xz_user_wallets (user_id, token_balance, cash_balance_cents, frozen_token, total_token_granted, total_token_used, updated_at, raw)
		values ($1,$2,0,$3,$4,$5,now(),$6::jsonb)
		on conflict (user_id) do update set token_balance=excluded.token_balance, cash_balance_cents=excluded.cash_balance_cents, frozen_token=excluded.frozen_token, total_token_granted=excluded.total_token_granted, total_token_used=excluded.total_token_used, updated_at=now(), raw=excluded.raw
	`, item.UserID, item.Available, item.Frozen, item.TotalGranted, item.TotalUsed, jsonProjection(raw))
	return err
}

func upsertAgentProfileFromChannelAgent(ctx context.Context, tx *sql.Tx, item adminChannelAgent) error {
	if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.UserID) == "" {
		return nil
	}
	inviteCode := item.InviteCode
	if inviteCode == "" {
		inviteCode = secureAgentInviteCode()
	}
	_, err := tx.ExecContext(ctx, `
		insert into xz_agent_profiles (id, user_id, parent_agent_id, operation_center_id, level, status, invite_code, join_order_id, join_fee_cents, token_rights_amount, created_at, updated_at, raw)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13::jsonb)
		on conflict (id) do update set user_id=excluded.user_id, parent_agent_id=excluded.parent_agent_id, operation_center_id=excluded.operation_center_id, level=excluded.level, status=excluded.status, invite_code=excluded.invite_code, join_order_id=excluded.join_order_id, join_fee_cents=excluded.join_fee_cents, token_rights_amount=excluded.token_rights_amount, created_at=excluded.created_at, updated_at=excluded.updated_at, raw=excluded.raw
	`, item.ID, item.UserID, item.ParentID, item.OperationCenterID, item.Level, item.Status, inviteCode, item.JoinOrderID, item.JoinFeeCents, item.TokenRightsAmount, item.CreatedAt, item.UpdatedAt, jsonProjection(item))
	if err != nil {
		return err
	}
	return refreshAgentWallet(ctx, tx, item.ID)
}

func refreshAgentWallet(ctx context.Context, tx *sql.Tx, agentID string) error {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil
	}
	var agent adminChannelAgent
	if err := tx.QueryRowContext(ctx, `select raw from xz_channel_agents where id = $1`, agentID).Scan(rawScanner(&agent)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	var totalCommission int
	if err := tx.QueryRowContext(ctx, `
		select coalesce(sum(amount_cents), 0)
		from xz_commissions
		where agent_id = $1 and upper(coalesce(status, '')) not in ('CANCELED', 'CANCELLED', 'VOID')
	`, agentID).Scan(&totalCommission); err != nil {
		return err
	}
	var frozenCommission int
	if err := tx.QueryRowContext(ctx, `
		select coalesce(sum(amount_cents), 0)
		from xz_withdrawals
		where agent_id = $1 and upper(coalesce(status, '')) in ('PENDING', 'REVIEWING', 'FROZEN')
	`, agentID).Scan(&frozenCommission); err != nil {
		return err
	}
	var totalWithdrawn int
	if err := tx.QueryRowContext(ctx, `
		select coalesce(sum(amount_cents), 0)
		from xz_withdrawals
		where agent_id = $1 and upper(coalesce(status, '')) in ('APPROVED', 'PAID', 'SETTLED', 'SUCCEEDED')
	`, agentID).Scan(&totalWithdrawn); err != nil {
		return err
	}
	withdrawable := totalCommission - frozenCommission - totalWithdrawn
	if withdrawable < 0 {
		withdrawable = 0
	}
	raw := map[string]any{
		"agentId":                  agentID,
		"userId":                   agent.UserID,
		"commissionBalanceCents":   totalCommission,
		"withdrawableBalanceCents": withdrawable,
		"frozenCommissionCents":    frozenCommission,
		"totalCommissionCents":     totalCommission,
		"totalWithdrawnCents":      totalWithdrawn,
	}
	_, err := tx.ExecContext(ctx, `
		insert into xz_agent_wallets (agent_id, user_id, commission_balance_cents, withdrawable_balance_cents, frozen_commission_cents, total_commission_cents, total_withdrawn_cents, updated_at, raw)
		values ($1,$2,$3,$4,$5,$6,$7,now(),$8::jsonb)
		on conflict (agent_id) do update set user_id=excluded.user_id, commission_balance_cents=excluded.commission_balance_cents, withdrawable_balance_cents=excluded.withdrawable_balance_cents, frozen_commission_cents=excluded.frozen_commission_cents, total_commission_cents=excluded.total_commission_cents, total_withdrawn_cents=excluded.total_withdrawn_cents, updated_at=now(), raw=excluded.raw
	`, agentID, agent.UserID, totalCommission, withdrawable, frozenCommission, totalCommission, totalWithdrawn, jsonProjection(raw))
	return err
}

func upsertMarketingCommissionRule(ctx context.Context, tx *sql.Tx, item adminCommissionRule) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if item.CreatedAt == "" {
		item.CreatedAt = now
	}
	if item.UpdatedAt == "" {
		item.UpdatedAt = now
	}
	if item.Metadata == nil {
		item.Metadata = map[string]any{}
	}
	_, err := tx.ExecContext(ctx, `
		insert into xz_marketing_commission_rules (id, name, order_type, earner_role, relation_depth, fixed_amount_cents, rate, max_total_rate, status, metadata, created_at, updated_at)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11,$12)
		on conflict (id) do update set name=excluded.name, order_type=excluded.order_type, earner_role=excluded.earner_role, relation_depth=excluded.relation_depth, fixed_amount_cents=excluded.fixed_amount_cents, rate=excluded.rate, max_total_rate=excluded.max_total_rate, status=excluded.status, metadata=excluded.metadata, updated_at=excluded.updated_at
	`, item.ID, item.Name, item.OrderType, item.EarnerRole, item.RelationDepth, item.FixedAmountCents, item.Rate, item.MaxTotalRate, item.Status, jsonProjection(item.Metadata), item.CreatedAt, item.UpdatedAt)
	return err
}

func disableLegacyMarketingRules(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
		update xz_marketing_commission_rules
		set status = 'DISABLED', updated_at = now()
		where id in ('rule_center_referral', 'rule_sales_direct', 'rule_sales_indirect')
	`)
	return err
}

func upsertMarketingRole(ctx context.Context, tx *sql.Tx, item map[string]any) error {
	metadata, _ := item["metadata"].(map[string]any)
	_, err := tx.ExecContext(ctx, `
		insert into xz_marketing_roles (id, code, name, level, status, metadata, created_at, updated_at)
		values ($1,$2,$3,$4,$5,$6::jsonb,$7,$8)
		on conflict (id) do update set code=excluded.code, name=excluded.name, level=excluded.level, status=excluded.status, metadata=excluded.metadata, updated_at=excluded.updated_at
	`, stringValue(item["id"]), stringValue(item["code"]), stringValue(item["name"]), intValue(item["level"]), fallback(stringValue(item["status"]), "ACTIVE"), jsonProjection(metadata), stringValue(item["createdAt"]), stringValue(item["updatedAt"]))
	return err
}

func upsertMarketingUpgradePlan(ctx context.Context, tx *sql.Tx, item map[string]any) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	metadata, _ := item["metadata"].(map[string]any)
	_, err := tx.ExecContext(ctx, `
		insert into xz_marketing_upgrade_plans (id, from_role, to_role, price_cents, condition_type, status, metadata, created_at, updated_at)
		values ($1,$2,$3,$4,$5,$6,$7::jsonb,$8,$9)
		on conflict (id) do update set from_role=excluded.from_role, to_role=excluded.to_role, price_cents=excluded.price_cents, condition_type=excluded.condition_type, status=excluded.status, metadata=excluded.metadata, updated_at=excluded.updated_at
	`, stringValue(item["id"]), stringValue(item["fromRole"]), stringValue(item["toRole"]), intValue(item["priceCents"]), stringValue(item["conditionType"]), fallback(stringValue(item["status"]), "ACTIVE"), jsonProjection(metadata), now, now)
	return err
}

func paymentCallbackEventByEventIDForTx(ctx context.Context, tx *sql.Tx, provider string, eventID string) (adminPaymentEvent, bool, error) {
	var item adminPaymentEvent
	err := tx.QueryRowContext(ctx, `
		select raw
		from xz_payment_events
		where provider = $1 and event_id = $2
		for update
	`, normalizePaymentMethod(provider), strings.TrimSpace(eventID)).Scan(rawScanner(&item))
	if errors.Is(err, sql.ErrNoRows) {
		return adminPaymentEvent{}, false, nil
	}
	return item, err == nil, err
}

func paymentCallbackEventByTransactionIDForTx(ctx context.Context, tx *sql.Tx, provider string, transactionID string) (adminPaymentEvent, bool, error) {
	var item adminPaymentEvent
	err := tx.QueryRowContext(ctx, `
		select raw
		from xz_payment_events
		where provider = $1 and transaction_id = $2
		for update
	`, normalizePaymentMethod(provider), strings.TrimSpace(transactionID)).Scan(rawScanner(&item))
	if errors.Is(err, sql.ErrNoRows) {
		return adminPaymentEvent{}, false, nil
	}
	return item, err == nil, err
}

func insertPaymentCallbackEvent(ctx context.Context, tx *sql.Tx, item adminPaymentEvent) (bool, error) {
	item = normalizePaymentCallbackEvent(item)
	result, err := tx.ExecContext(ctx, `
		insert into xz_payment_events (id, tenant_id, provider, event_id, idempotency_key, order_id, transaction_id, amount_cents, raw, verified, status, processed_at, created_at)
		values ($1,nullif($2,''),$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11,nullif($12,'')::timestamptz,$13::timestamptz)
		on conflict do nothing
	`, item.ID, item.TenantID, item.Provider, item.EventID, item.IdempotencyKey, item.OrderID, nullableSQLString(item.TransactionID), item.AmountCents, jsonProjection(item), item.Verified, item.Status, item.ProcessedAt, item.CreatedAt)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

func insertBillingEvent(ctx context.Context, tx *sql.Tx, item adminBillingEvent) error {
	_, err := tx.ExecContext(ctx, `
		insert into xz_billing_events (id, transaction_id, user_id, agent_id, tenant_id, operation_center_id, module_code, task_id, metric_code, quantity, unit_amount_cents, amount_cents, point_cost, balance_before, balance_after, model, status, occurred_at, metadata, raw)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19::jsonb,$20::jsonb)
		on conflict (id) do update set transaction_id=excluded.transaction_id, user_id=excluded.user_id, agent_id=excluded.agent_id, tenant_id=excluded.tenant_id, operation_center_id=excluded.operation_center_id, module_code=excluded.module_code, task_id=excluded.task_id, metric_code=excluded.metric_code, quantity=excluded.quantity, unit_amount_cents=excluded.unit_amount_cents, amount_cents=excluded.amount_cents, point_cost=excluded.point_cost, balance_before=excluded.balance_before, balance_after=excluded.balance_after, model=excluded.model, status=excluded.status, occurred_at=excluded.occurred_at, metadata=excluded.metadata, raw=excluded.raw
	`, item.ID, item.TransactionID, item.UserID, item.AgentID, item.TenantID, item.OperationCenterID, item.ModuleCode, item.TaskID, item.MetricCode, item.Quantity, item.UnitAmountCents, item.AmountCents, item.PointCost, item.BalanceBefore, item.BalanceAfter, item.Model, item.Status, item.OccurredAt, jsonProjection(item.Metadata), jsonProjection(item))
	return err
}

func insertWithdrawal(ctx context.Context, tx *sql.Tx, item adminWithdrawal) error {
	_, err := tx.ExecContext(ctx, `
		insert into xz_withdrawals (id, agent_id, amount_cents, status, created_at, reviewed_at, raw)
		values ($1,$2,$3,$4,$5,$6,$7::jsonb)
		on conflict (id) do update set agent_id=excluded.agent_id, amount_cents=excluded.amount_cents, status=excluded.status, created_at=excluded.created_at, reviewed_at=excluded.reviewed_at, raw=excluded.raw
	`, item.ID, item.AgentID, item.AmountCents, item.Status, item.CreatedAt, item.ReviewedAt, jsonProjection(item))
	if err != nil {
		return err
	}
	return refreshAgentWallet(ctx, tx, item.AgentID)
}

func insertAPIChannel(ctx context.Context, tx *sql.Tx, item adminAPIChannel) error {
	_, err := tx.ExecContext(ctx, `
		insert into xz_api_channels (id, name, base_url, protocol, status, priority, raw)
		values ($1,$2,$3,$4,$5,$6,$7::jsonb)
		on conflict (id) do update set name=excluded.name, base_url=excluded.base_url, protocol=excluded.protocol, status=excluded.status, priority=excluded.priority, raw=excluded.raw
	`, item.ID, item.Name, item.BaseURL, item.Protocol, item.Status, item.Priority, jsonProjection(item))
	return err
}

func insertAPIKey(ctx context.Context, tx *sql.Tx, item adminAPIKey) error {
	_, err := tx.ExecContext(ctx, `
		insert into xz_api_keys (id, customer, prefix, status, quota_limit, raw)
		values ($1,$2,$3,$4,$5,$6::jsonb)
		on conflict (id) do update set customer=excluded.customer, prefix=excluded.prefix, status=excluded.status, quota_limit=excluded.quota_limit, raw=excluded.raw
	`, item.ID, item.Customer, item.Prefix, item.Status, item.QuotaLimit, jsonProjection(item))
	return err
}

func ensureRechargeImageBackupRouteTx(ctx context.Context, tx *sql.Tx, userID string, quotaLimit int, now string) (adminUserModelRoute, error) {
	if strings.TrimSpace(userID) == "" {
		return adminUserModelRoute{}, nil
	}
	var user adminUser
	if err := tx.QueryRowContext(ctx, `select raw from xz_users where id = $1 for update`, userID).Scan(rawScanner(&user)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return adminUserModelRoute{}, nil
		}
		return adminUserModelRoute{}, err
	}
	channel, err := preferredImageBackupChannelTx(ctx, tx)
	if err != nil {
		return adminUserModelRoute{}, err
	}
	if channel.ID == "" {
		return adminUserModelRoute{}, nil
	}
	key, err := upsertUserModelAPIKeyTx(ctx, tx, user, quotaLimit, channel)
	if err != nil {
		return adminUserModelRoute{}, err
	}
	route := buildUserImageBackupRoute(user, channel, key, quotaLimit, now)
	routes := user.ModelRoutes
	replaced := false
	for i := range routes {
		if routes[i].ID == route.ID || strings.EqualFold(routes[i].GroupName, route.GroupName) {
			route.QuotaUsed = routes[i].QuotaUsed
			route.ExternalKey = routes[i].ExternalKey
			route.ExternalUser = routes[i].ExternalUser
			if route.QuotaLimit < routes[i].QuotaLimit {
				route.QuotaLimit = routes[i].QuotaLimit
			}
			routes[i] = route
			replaced = true
			break
		}
	}
	if !replaced {
		routes = append(routes, route)
	}
	user.ModelRoutes = routes
	user.UpdatedAt = now
	if err := insertUser(ctx, tx, user); err != nil {
		return adminUserModelRoute{}, err
	}
	return route, nil
}

func preferredImageBackupChannelTx(ctx context.Context, tx *sql.Tx) (adminAPIChannel, error) {
	rows, err := tx.QueryContext(ctx, `select raw from xz_api_channels order by priority, id`)
	if err != nil {
		return adminAPIChannel{}, err
	}
	defer rows.Close()
	channels := []adminAPIChannel{}
	for rows.Next() {
		var item adminAPIChannel
		if err := rows.Scan(rawScanner(&item)); err != nil {
			return adminAPIChannel{}, err
		}
		channels = append(channels, item)
	}
	if err := rows.Err(); err != nil {
		return adminAPIChannel{}, err
	}
	return preferredImageBackupChannel(channels), nil
}

func upsertUserModelAPIKeyTx(ctx context.Context, tx *sql.Tx, user adminUser, quotaLimit int, channel adminAPIChannel) (adminAPIKey, error) {
	keyID := "key_" + user.ID
	var item adminAPIKey
	err := tx.QueryRowContext(ctx, `select raw from xz_api_keys where id = $1 for update`, keyID).Scan(rawScanner(&item))
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return adminAPIKey{}, err
	}
	keys := []adminAPIKey{}
	if !errors.Is(err, sql.ErrNoRows) {
		keys = append(keys, item)
	}
	key := upsertUserModelAPIKey(&keys, user, quotaLimit)
	if len(keys) > 0 {
		key = keys[0]
	}
	if channel.ID != "" && isPlaceholderAPIKeySecret(key.Secret) {
		channelKeys, listErr := listAPIKeysForTx(ctx, tx)
		if listErr != nil {
			return adminAPIKey{}, listErr
		}
		if secret := savedNonPlaceholderAPIKeyForChannel(channelKeys, channel); secret != "" {
			key.Secret = secret
			key.Prefix = apiKeyPrefix(secret, 1)
		}
	}
	if err := insertAPIKey(ctx, tx, key); err != nil {
		return adminAPIKey{}, err
	}
	return key, nil
}

func applyCustomerModelRouteTx(ctx context.Context, tx *sql.Tx, user adminUser, req adminCustomerMutation, now string) (adminUserModelRoute, error) {
	channels, err := listAPIChannelsForTx(ctx, tx)
	if err != nil {
		return adminUserModelRoute{}, err
	}
	channel := findAPIChannelForRoute(channels, req.ModelChannelID, req.ModelChannel)
	if channel.ID == "" {
		channel = preferredImageBackupChannel(channels)
	}
	if channel.ID == "" {
		return adminUserModelRoute{}, nil
	}
	quota := req.ModelQuotaLimit
	if quota <= 0 {
		quota = 100000
	}
	key, err := upsertUserModelAPIKeyTx(ctx, tx, user, quota, channel)
	if err != nil {
		return adminUserModelRoute{}, err
	}
	if secret := strings.TrimSpace(req.ModelAPIKey); secret != "" {
		key.Secret = secret
		key.Prefix = apiKeyPrefix(secret, 1)
	}
	models := parseRouteModels(req.ModelModels)
	if len(models) == 0 {
		models = []string{"gpt-image-2"}
	}
	status := strings.ToUpper(strings.TrimSpace(req.ModelKeyStatus))
	if status == "" {
		status = "ACTIVE"
	}
	if req.ModelRouteEnabled != nil && !*req.ModelRouteEnabled {
		status = "DISABLED"
	}
	group := strings.TrimSpace(req.ModelGroup)
	if group == "" {
		group = "生图备份"
	}
	key.Status = status
	key.Models = mergeStringSet(key.Models, models)
	if key.QuotaLimit < quota {
		key.QuotaLimit = quota
	}
	if err := insertAPIKey(ctx, tx, key); err != nil {
		return adminUserModelRoute{}, err
	}
	existingRoute := adminUserModelRoute{}
	for _, item := range user.ModelRoutes {
		if item.ID == "route_"+user.ID+"_image_backup" || strings.EqualFold(item.GroupName, group) {
			existingRoute = item
			break
		}
	}
	return adminUserModelRoute{
		ID:           "route_" + user.ID + "_image_backup",
		Provider:     "newapi",
		ChannelID:    channel.ID,
		Channel:      fallback(channel.Name, req.ModelChannel),
		APIKeyID:     key.ID,
		KeyPrefix:    key.Prefix,
		GroupName:    group,
		Models:       models,
		QuotaLimit:   quota,
		Status:       status,
		UpdatedAt:    now,
		ExternalKey:  existingRoute.ExternalKey,
		ExternalUser: existingRoute.ExternalUser,
	}, nil
}

func listAPIChannelsForTx(ctx context.Context, tx *sql.Tx) ([]adminAPIChannel, error) {
	rows, err := tx.QueryContext(ctx, `select raw from xz_api_channels order by priority, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	channels := []adminAPIChannel{}
	for rows.Next() {
		var item adminAPIChannel
		if err := rows.Scan(rawScanner(&item)); err != nil {
			return nil, err
		}
		channels = append(channels, item)
	}
	return channels, rows.Err()
}

func listAPIKeysForTx(ctx context.Context, tx *sql.Tx) ([]adminAPIKey, error) {
	rows, err := tx.QueryContext(ctx, `select raw from xz_api_keys order by id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := []adminAPIKey{}
	for rows.Next() {
		var item adminAPIKey
		if err := rows.Scan(rawScanner(&item)); err != nil {
			return nil, err
		}
		keys = append(keys, item)
	}
	return keys, rows.Err()
}

func insertGenerationTask(ctx context.Context, tx *sql.Tx, item generationTask) error {
	_, err := tx.ExecContext(ctx, `
		insert into xz_generation_tasks (
			id,user_id,tenant_id,organization_id,billing_account_type,billing_account_id,module_code,type,model,billing_type,
			status,progress,point_cost,prompt,params,result_ids,error,created_at,updated_at,worker_finished_at,
			client_request_id,task_status,billing_status,billing_rule_version_id,quoted_points,reserved_points,captured_points,
			released_points,refunded_points,supplier_cost,estimated_margin,provider_channel,raw
		) values (
			$1,$2,nullif($3,''),nullif($4,''),$5,nullif($6,''),$7,$8,$9,$10,$11,$12,$13,$14,$15::jsonb,$16::jsonb,$17::jsonb,$18,$19,$20,
			nullif($21,''),$22,$23,nullif($24,''),$25,$26,$27,$28,$29,$30,$31,nullif($32,''),$33::jsonb
		)
		on conflict (id) do update set
			user_id=excluded.user_id,tenant_id=excluded.tenant_id,organization_id=excluded.organization_id,billing_account_type=excluded.billing_account_type,
			billing_account_id=excluded.billing_account_id,module_code=excluded.module_code,type=excluded.type,model=excluded.model,billing_type=excluded.billing_type,
			status=excluded.status,progress=excluded.progress,point_cost=excluded.point_cost,prompt=excluded.prompt,params=excluded.params,result_ids=excluded.result_ids,
			error=excluded.error,created_at=excluded.created_at,updated_at=excluded.updated_at,worker_finished_at=excluded.worker_finished_at,
			client_request_id=coalesce(excluded.client_request_id,xz_generation_tasks.client_request_id),task_status=excluded.task_status,billing_status=excluded.billing_status,
			billing_rule_version_id=coalesce(excluded.billing_rule_version_id,xz_generation_tasks.billing_rule_version_id),quoted_points=excluded.quoted_points,
			reserved_points=excluded.reserved_points,captured_points=excluded.captured_points,released_points=excluded.released_points,refunded_points=excluded.refunded_points,
			supplier_cost=coalesce(excluded.supplier_cost,xz_generation_tasks.supplier_cost),estimated_margin=coalesce(excluded.estimated_margin,xz_generation_tasks.estimated_margin),
			provider_channel=excluded.provider_channel,raw=excluded.raw
	`, item.ID, item.UserID, item.TenantID, item.OrganizationID, firstNonEmptyString(item.BillingAccountType, contextPersonal), item.BillingAccountID, item.ModuleCode, item.Type, item.Model, item.BillingType,
		item.Status, item.Progress, item.PointCost, item.Prompt, jsonProjection(item.Params), jsonProjection(item.ResultIDs), jsonProjection(item.Error), item.CreatedAt, item.UpdatedAt, item.WorkerFinishedAt,
		item.ClientRequestID, firstNonEmptyString(item.TaskStatus, canonicalTaskStatus(item.Status)), firstNonEmptyString(item.BillingStatus, billingStatusUnquoted), item.BillingRuleVersionID,
		item.QuotedPoints, item.ReservedPoints, item.CapturedPoints, item.ReleasedPoints, item.RefundedPoints, item.SupplierCost, item.EstimatedMargin, item.ProviderChannel, jsonProjection(item))
	return err
}

func generationTaskForUpdate(ctx context.Context, tx *sql.Tx, id string) (generationTask, error) {
	var item generationTask
	err := tx.QueryRowContext(ctx, `select raw,coalesce(client_request_id,''),task_status,billing_status,coalesce(billing_rule_version_id,''),quoted_points,reserved_points,captured_points,released_points,refunded_points,supplier_cost,estimated_margin,coalesce(provider_channel,'') from xz_generation_tasks where id = $1 for update`, id).Scan(rawScanner(&item), &item.ClientRequestID, &item.TaskStatus, &item.BillingStatus, &item.BillingRuleVersionID, &item.QuotedPoints, &item.ReservedPoints, &item.CapturedPoints, &item.ReleasedPoints, &item.RefundedPoints, &item.SupplierCost, &item.EstimatedMargin, &item.ProviderChannel)
	return item, err
}

func insertAsset(ctx context.Context, tx *sql.Tx, item asset) error {
	_, err := tx.ExecContext(ctx, `
		insert into xz_assets (id, user_id, tenant_id, organization_id, task_id, name, media_type, url, thumbnail_url, favorite, metadata, deleted_at, created_at, updated_at, raw)
		values ($1,$2,nullif($3,''),nullif($4,''),$5,$6,$7,$8,$9,$10,$11::jsonb,$12,$13,$14,$15::jsonb)
		on conflict (id) do update set user_id=excluded.user_id, tenant_id=excluded.tenant_id, organization_id=excluded.organization_id, task_id=excluded.task_id, name=excluded.name, media_type=excluded.media_type, url=excluded.url, thumbnail_url=excluded.thumbnail_url, favorite=excluded.favorite, metadata=excluded.metadata, deleted_at=excluded.deleted_at, created_at=excluded.created_at, updated_at=excluded.updated_at, raw=excluded.raw
	`, item.ID, item.UserID, item.TenantID, item.OrganizationID, item.TaskID, item.Name, item.MediaType, item.URL, item.ThumbnailURL, item.Favorite, jsonProjection(item.Metadata), nullableSQLString(item.DeletedAt), item.CreatedAt, item.UpdatedAt, jsonProjection(item))
	return err
}

func pointAccountForUpdate(ctx context.Context, tx *sql.Tx, userID string) (adminPointAccount, error) {
	var item adminPointAccount
	err := tx.QueryRowContext(ctx, `select raw from xz_point_accounts where user_id = $1 for update`, userID).Scan(rawScanner(&item))
	if errors.Is(err, sql.ErrNoRows) {
		id, idErr := nextTableID(ctx, tx, "xz_point_accounts", "points")
		if idErr != nil {
			return adminPointAccount{}, idErr
		}
		item = adminPointAccount{ID: id, UserID: userID, Available: 0}
		if err := insertPointAccount(ctx, tx, item); err != nil {
			return adminPointAccount{}, err
		}
		return item, nil
	}
	return item, err
}

func totalPointsForUserTx(ctx context.Context, db *sql.DB, userID string, available int, frozen int) (int, error) {
	consumed := 0
	err := db.QueryRowContext(ctx, `
		select coalesce(sum(point_cost), 0)
		from xz_billing_events
		where user_id = $1 and point_cost > 0
	`, userID).Scan(&consumed)
	if err != nil {
		return 0, err
	}
	total := available + frozen + consumed
	if total < available+frozen {
		return available + frozen, nil
	}
	return total, nil
}

func generationBillingArtifactsForTx(ctx context.Context, tx *sql.Tx, task generationTask, before int, after int, now string) (adminBillingEvent, []adminCommission, error) {
	user, err := userByIDForTx(ctx, tx, task.UserID)
	if err != nil {
		return adminBillingEvent{}, nil, err
	}
	agent, hasAgent, err := directActiveAgentForUserTx(ctx, tx, task.UserID)
	if err != nil {
		return adminBillingEvent{}, nil, err
	}
	event := generationBillingEvent(task, before, after, now, user, agent, hasAgent)
	moduleCode := firstNonEmptyString(task.ModuleCode, stringValue(task.Params["module_code"]), moduleCodeForType(task.Type))
	commissions, err := commissionArtifactsForUserTx(ctx, tx, task.UserID, task.ID, commissionOrderTypeForModule(moduleCode), moduleCode, event.AmountCents, now)
	if err != nil {
		return adminBillingEvent{}, nil, err
	}
	return event, commissions, nil
}

func userByIDForTx(ctx context.Context, tx *sql.Tx, id string) (adminUser, error) {
	var item adminUser
	err := tx.QueryRowContext(ctx, `select raw from xz_users where id = $1`, id).Scan(rawScanner(&item))
	if errors.Is(err, sql.ErrNoRows) {
		return adminUser{ID: id}, nil
	}
	return item, err
}

func userByIDForUpdateTx(ctx context.Context, tx *sql.Tx, id string) (adminUser, error) {
	var item adminUser
	err := tx.QueryRowContext(ctx, `select raw from xz_users where id = $1 for update`, id).Scan(rawScanner(&item))
	if errors.Is(err, sql.ErrNoRows) {
		return adminUser{}, fmt.Errorf("user not found: %s", id)
	}
	return item, err
}

func channelAgentByUserIDForTx(ctx context.Context, tx *sql.Tx, userID string) (adminChannelAgent, bool, error) {
	var item adminChannelAgent
	err := tx.QueryRowContext(ctx, `select raw from xz_channel_agents where user_id = $1 and status = 'ACTIVE' order by level, id limit 1`, userID).Scan(rawScanner(&item))
	if errors.Is(err, sql.ErrNoRows) {
		return adminChannelAgent{}, false, nil
	}
	return item, err == nil, err
}

func channelAgentByUserIDForUpdateTx(ctx context.Context, tx *sql.Tx, userID string) (adminChannelAgent, bool, error) {
	var item adminChannelAgent
	err := tx.QueryRowContext(ctx, `select raw from xz_channel_agents where user_id = $1 for update`, userID).Scan(rawScanner(&item))
	if errors.Is(err, sql.ErrNoRows) {
		return adminChannelAgent{}, false, nil
	}
	return item, err == nil, err
}

func channelAgentByIDForTx(ctx context.Context, tx *sql.Tx, id string) (adminChannelAgent, bool, error) {
	var item adminChannelAgent
	err := tx.QueryRowContext(ctx, `select raw from xz_channel_agents where id = $1 and status = 'ACTIVE'`, id).Scan(rawScanner(&item))
	if errors.Is(err, sql.ErrNoRows) {
		return adminChannelAgent{}, false, nil
	}
	return item, err == nil, err
}

func directActiveAgentForUserTx(ctx context.Context, tx *sql.Tx, userID string) (adminChannelAgent, bool, error) {
	var item adminChannelAgent
	err := tx.QueryRowContext(ctx, `
		SELECT agent.raw
		FROM xz_user_relationships relation
		JOIN xz_channel_agents agent ON agent.id=relation.parent_agent_id AND upper(coalesce(agent.status,''))='ACTIVE'
		JOIN xz_user_business_identities identity ON identity.user_id=agent.user_id
		 AND identity.identity_type='AGENT' AND identity.identity_status='ACTIVE' AND identity.ended_at IS NULL
		 AND identity.effective_at<=clock_timestamp() AND (identity.expires_at IS NULL OR identity.expires_at>clock_timestamp())
		WHERE relation.user_id=$1 AND relation.status='ACTIVE' AND relation.ended_at IS NULL
	`, userID).Scan(rawScanner(&item))
	if err == nil {
		return item, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return adminChannelAgent{}, false, err
	}
	// Compatibility only: legacy records without a canonical relationship may
	// still read referred_by until they are migrated. New writes use relationships.
	user, err := userByIDForTx(ctx, tx, userID)
	if err != nil {
		return adminChannelAgent{}, false, err
	}
	if strings.TrimSpace(user.ReferredBy) == "" {
		return adminChannelAgent{}, false, nil
	}
	return channelAgentByUserIDForTx(ctx, tx, user.ReferredBy)
}

func firstActiveOperationCenterIDTx(ctx context.Context, tx *sql.Tx) (string, error) {
	var item adminOperationCenter
	err := tx.QueryRowContext(ctx, `select raw from xz_operation_centers where status = 'ACTIVE' order by created_at, id limit 1`).Scan(rawScanner(&item))
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return item.ID, nil
}

func ensureAgentForUserTx(ctx context.Context, tx *sql.Tx, user adminUser, order *adminOrder, result commissionSettlementResult, now string) error {
	item, exists, err := channelAgentByUserIDForUpdateTx(ctx, tx, user.ID)
	if err != nil {
		return err
	}
	if !exists {
		agentID, err := nextTableID(ctx, tx, "xz_channel_agents", "channel")
		if err != nil {
			return err
		}
		item = adminChannelAgent{
			ID:        agentID,
			UserID:    user.ID,
			Level:     2,
			CreatedAt: now,
		}
	}
	if !exists {
		item.ParentID = order.DirectAgentID
		item.OperationCenterID = order.OperationCenterID
	}
	item.Status = "ACTIVE"
	if item.Level <= 0 {
		item.Level = 2
	}
	if item.InviteCode == "" {
		item.InviteCode = secureAgentInviteCode()
	}
	item.JoinOrderID = order.ID
	item.JoinFeeCents = orderAmount(*order)
	item.TokenRightsAmount = result.TokenGrantAmount
	item.UpdatedAt = now
	return insertChannelAgent(ctx, tx, item)
}

func ensureOperationCenterForUserTx(ctx context.Context, tx *sql.Tx, user adminUser, order *adminOrder, now string) error {
	item, exists, err := operationCenterByUserIDForUpdateTx(ctx, tx, user.ID)
	if err != nil {
		return err
	}
	if !exists {
		centerID, err := nextTableID(ctx, tx, "xz_operation_centers", "operation_center")
		if err != nil {
			return err
		}
		item = adminOperationCenter{
			ID:         centerID,
			UserID:     user.ID,
			Name:       user.Name + "运营中心",
			InviteCode: strings.ToUpper("OC" + shortID(centerID)),
			CreatedAt:  now,
		}
	}
	item.Status = "ACTIVE"
	item.JoinOrderID = order.ID
	item.JoinFeeCents = orderAmount(*order)
	item.ApprovedAt = now
	item.UpdatedAt = now
	return insertOperationCenter(ctx, tx, item)
}

func operationCenterByUserIDForUpdateTx(ctx context.Context, tx *sql.Tx, userID string) (adminOperationCenter, bool, error) {
	var item adminOperationCenter
	err := tx.QueryRowContext(ctx, `select raw from xz_operation_centers where user_id = $1 for update`, userID).Scan(rawScanner(&item))
	if errors.Is(err, sql.ErrNoRows) {
		return adminOperationCenter{}, false, nil
	}
	return item, err == nil, err
}

func rechargeAgentForUserTx(ctx context.Context, tx *sql.Tx, userID string) (adminChannelAgent, bool, error) {
	return directActiveAgentForUserTx(ctx, tx, userID)
}

func activeAgentChainForUserTx(ctx context.Context, tx *sql.Tx, userID string) ([]adminChannelAgent, error) {
	direct, ok, err := directActiveAgentForUserTx(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	chain := []adminChannelAgent{direct}
	current := direct
	for len(chain) < 2 && strings.TrimSpace(current.ParentID) != "" {
		parent, ok, err := channelAgentByIDForTx(ctx, tx, current.ParentID)
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}
		chain = append(chain, parent)
		current = parent
	}
	return chain, nil
}

func marketingCommissionRuleByIDForUpdate(ctx context.Context, tx *sql.Tx, id string) (adminCommissionRule, error) {
	row := tx.QueryRowContext(ctx, `
		select id, name, order_type, earner_role, relation_depth, fixed_amount_cents, rate, max_total_rate, status, metadata, created_at, updated_at
		from xz_marketing_commission_rules
		where id = $1
		for update
	`, id)
	item, err := scanMarketingCommissionRule(row)
	if errors.Is(err, sql.ErrNoRows) {
		return adminCommissionRule{}, fmt.Errorf("commission rule not found: %s", id)
	}
	return item, err
}

func marketingCommissionRulesForTx(ctx context.Context, tx *sql.Tx, orderType string) ([]adminCommissionRule, error) {
	rows, err := tx.QueryContext(ctx, `
		select id, name, order_type, earner_role, relation_depth, fixed_amount_cents, rate, max_total_rate, status, metadata, created_at, updated_at
		from xz_marketing_commission_rules
		where upper(order_type) = upper($1)
		order by relation_depth, id
	`, strings.TrimSpace(orderType))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []adminCommissionRule{}
	for rows.Next() {
		item, err := scanMarketingCommissionRule(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return activeCommissionRules(defaultCommissionRules(), orderType), nil
	}
	return activeCommissionRules(items, orderType), nil
}

func commissionArtifactsForUserTx(ctx context.Context, tx *sql.Tx, userID string, orderID string, orderType string, source string, amountCents int, now string) ([]adminCommission, error) {
	if amountCents <= 0 {
		return nil, nil
	}
	rules, err := marketingCommissionRulesForTx(ctx, tx, orderType)
	if err != nil {
		return nil, err
	}
	if len(rules) == 0 {
		return nil, nil
	}
	chain, err := activeAgentChainForUserTx(ctx, tx, userID)
	if err != nil || len(chain) == 0 {
		return nil, err
	}
	type matchedCommissionRule struct {
		rule  adminCommissionRule
		agent adminChannelAgent
	}
	matchedRules := []matchedCommissionRule{}
	maxTotalRate := 0.0
	for _, rule := range rules {
		if rule.RelationDepth <= 0 || rule.RelationDepth > len(chain) {
			continue
		}
		agent := chain[rule.RelationDepth-1]
		if !commissionRuleMatchesAgent(rule, agent) {
			continue
		}
		matchedRules = append(matchedRules, matchedCommissionRule{rule: rule, agent: agent})
		if rule.MaxTotalRate > maxTotalRate {
			maxTotalRate = rule.MaxTotalRate
		}
	}
	maxTotalCents := 0
	if maxTotalRate > 0 {
		maxTotalCents = int(math.Round(float64(amountCents) * maxTotalRate))
	}
	items := []adminCommission{}
	totalCents := 0
	for _, match := range matchedRules {
		rule := match.rule
		agent := match.agent
		exists, err := commissionExistsForRuleTx(ctx, tx, orderID, agent.ID, rule.ID)
		if err != nil || exists {
			return items, err
		}
		commissionCents := rule.FixedAmountCents
		if commissionCents <= 0 && rule.Rate > 0 {
			commissionCents = int(math.Round(float64(amountCents) * rule.Rate))
		}
		if commissionCents <= 0 {
			continue
		}
		if maxTotalCents > 0 && totalCents+commissionCents > maxTotalCents {
			commissionCents = maxTotalCents - totalCents
		}
		if commissionCents <= 0 {
			continue
		}
		id := "commission_" + shortID(orderID+"_"+agent.ID+"_"+rule.ID)
		totalCents += commissionCents
		items = append(items, adminCommission{
			ID:          id,
			OrderID:     orderID,
			AgentID:     agent.ID,
			AmountCents: commissionCents,
			Rate:        rule.Rate,
			Status:      "PENDING",
			RuleSnapshot: map[string]any{
				"source":           source,
				"orderType":        strings.ToUpper(strings.TrimSpace(orderType)),
				"amountCents":      amountCents,
				"rate":             rule.Rate,
				"fixedAmountCents": rule.FixedAmountCents,
				"maxTotalRate":     rule.MaxTotalRate,
				"relationDepth":    rule.RelationDepth,
				"ruleId":           rule.ID,
				"ruleName":         rule.Name,
				"settlementMode":   "RULE_ENGINE",
			},
			CreatedAt: now,
		})
	}
	return items, nil
}

func commissionExistsForRuleTx(ctx context.Context, tx *sql.Tx, orderID string, agentID string, ruleID string) (bool, error) {
	rows, err := tx.QueryContext(ctx, `select raw from xz_commissions where order_id = $1 and agent_id = $2`, orderID, agentID)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var item adminCommission
		if err := scanRawJSON(rows, &item); err != nil {
			return false, err
		}
		if ruleID == "" || stringMetadataValueFromMap(item.RuleSnapshot, "ruleId") == ruleID {
			return true, nil
		}
	}
	return false, rows.Err()
}

func tokenRecordExistsTx(ctx context.Context, tx *sql.Tx, orderID string, changeType string) (bool, error) {
	var count int
	err := tx.QueryRowContext(ctx, `select count(*) from xz_token_records where order_id = $1 and upper(change_type) = upper($2)`, orderID, changeType).Scan(&count)
	return count > 0, err
}

func billingEventExistsTx(ctx context.Context, tx *sql.Tx, taskID string, metricCode string) (bool, error) {
	var count int
	err := tx.QueryRowContext(ctx, `select count(*) from xz_billing_events where task_id = $1 and metric_code = $2`, taskID, metricCode).Scan(&count)
	return count > 0, err
}

func billingEventForTaskMetricTx(ctx context.Context, tx *sql.Tx, taskID string, metricCode string) (adminBillingEvent, bool, error) {
	var item adminBillingEvent
	err := tx.QueryRowContext(ctx, `
		select raw
		from xz_billing_events
		where task_id = $1 and metric_code = $2
		order by occurred_at desc
		limit 1
	`, taskID, metricCode).Scan(rawScanner(&item))
	if errors.Is(err, sql.ErrNoRows) {
		return adminBillingEvent{}, false, nil
	}
	return item, err == nil, err
}

func getOrderForUpdate(ctx context.Context, tx *sql.Tx, id string) (adminOrder, error) {
	var item adminOrder
	err := tx.QueryRowContext(ctx, `select raw,coalesce(tenant_id,'') from xz_orders where id = $1 for update`, id).Scan(rawScanner(&item), &item.TenantID)
	return item, err
}

func currentEnterpriseTenantForOrderTx(ctx context.Context, tx *sql.Tx, userID string) (string, error) {
	var tenantID string
	err := tx.QueryRowContext(ctx, `
		SELECT role_context.tenant_id
		FROM xz_user_role_context role_context
		JOIN xz_tenants tenant ON tenant.id=role_context.tenant_id AND tenant.tenant_type='ENTERPRISE'
		JOIN xz_tenant_members member ON member.tenant_id=role_context.tenant_id AND member.user_id=role_context.user_id
		WHERE role_context.user_id=$1 AND upper(role_context.context_type)='ENTERPRISE'
		  AND upper(coalesce(nullif(member.member_status,''),member.status,'ACTIVE'))='ACTIVE'
		LIMIT 1
	`, userID).Scan(&tenantID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return tenantID, err
}

func getWithdrawalForUpdate(ctx context.Context, tx *sql.Tx, id string) (adminWithdrawal, error) {
	var item adminWithdrawal
	err := tx.QueryRowContext(ctx, `select raw from xz_withdrawals where id = $1 for update`, id).Scan(rawScanner(&item))
	return item, err
}

func nextTableID(ctx context.Context, tx *sql.Tx, table string, prefix string) (string, error) {
	allowed := map[string]bool{"xz_users": true, "xz_auth_account_merge_requests": true, "xz_point_accounts": true, "xz_orders": true, "xz_channel_agents": true, "xz_operation_centers": true, "xz_commissions": true, "xz_token_records": true, "xz_billing_events": true, "xz_withdrawals": true, "xz_generation_tasks": true, "xz_assets": true, "xz_audit_logs": true, "xz_operation_logs": true, "xz_backup_runs": true}
	if !allowed[table] {
		return "", fmt.Errorf("unsupported id table: %s", table)
	}
	query := fmt.Sprintf(`select id from %s where id like $1`, table)
	rows, err := tx.QueryContext(ctx, query, prefix+"_%")
	if err != nil {
		return "", err
	}
	defer rows.Close()
	maxValue := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return "", err
		}
		parts := strings.Split(id, "_")
		value, _ := strconv.Atoi(parts[len(parts)-1])
		if value > maxValue {
			maxValue = value
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s_%06d", prefix, maxValue+1), nil
}

func insertAuditLog(ctx context.Context, tx *sql.Tx, actorID string, actorRole string, action string, resource string, resourceID string, method string, path string, status int, metadata map[string]any) error {
	if isPricingAuditAction(action) {
		return insertPricingAuditLog(ctx, tx, pricingAuditMutationFromLegacy(
			actorID, actorRole, action, resource, resourceID, method, path, status, metadata,
		))
	}
	id := newAuditID()
	_, err := tx.ExecContext(ctx, `
		insert into xz_audit_logs (id, actor_id, actor_role, action, resource, resource_id, method, path, status, metadata)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb)
		on conflict (id) do nothing
	`, id, actorID, actorRole, action, resource, resourceID, method, path, status, jsonProjection(metadata))
	return err
}

func ensureGovernanceSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		create table if not exists xz_audit_logs (id text primary key, actor_id text, actor_role text, action text not null, resource text not null, resource_id text, method text, path text, status int, metadata jsonb not null default '{}'::jsonb, created_at timestamptz not null default now());
		create table if not exists xz_operation_logs (id text primary key, actor_id text, operation text not null, target text not null, target_id text, before_state jsonb, after_state jsonb, created_at timestamptz not null default now());
		create table if not exists xz_role_permissions (role text not null, permission text not null, created_at timestamptz not null default now(), primary key(role, permission));
		create table if not exists xz_backup_runs (id text primary key, status text not null, target text not null, metadata jsonb not null default '{}'::jsonb, started_at timestamptz not null default now(), finished_at timestamptz);
	`)
	return err
}

func ensureMarketingSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		create table if not exists xz_marketing_roles (id text primary key, code text not null unique, name text not null, level int not null default 0, status text not null default 'ACTIVE', metadata jsonb not null default '{}'::jsonb, created_at timestamptz not null default now(), updated_at timestamptz not null default now());
		create table if not exists xz_marketing_permissions (id text primary key, code text not null unique, name text not null, module text not null, action text not null, metadata jsonb not null default '{}'::jsonb, created_at timestamptz not null default now());
		create table if not exists xz_marketing_role_permissions (role_code text not null, permission_code text not null, created_at timestamptz not null default now(), primary key(role_code, permission_code));
		create table if not exists xz_marketing_org_relations (ancestor_user_id text not null, descendant_user_id text not null, depth int not null, bind_type text not null default 'INVITE', status text not null default 'ACTIVE', metadata jsonb not null default '{}'::jsonb, created_at timestamptz not null default now(), primary key(ancestor_user_id, descendant_user_id, depth));
		create table if not exists xz_marketing_invite_codes (id text primary key, owner_user_id text not null, code text not null unique, qrcode_url text not null default '', landing_url text not null default '', status text not null default 'ACTIVE', expire_at timestamptz, created_at timestamptz not null default now(), updated_at timestamptz not null default now());
		create table if not exists xz_marketing_invite_records (id text primary key, inviter_user_id text not null, invitee_user_id text, invite_code text not null, source text not null default 'QR', register_status text not null default 'PENDING', recharge_status text not null default 'PENDING', upgrade_status text not null default 'PENDING', metadata jsonb not null default '{}'::jsonb, created_at timestamptz not null default now(), updated_at timestamptz not null default now());
		create table if not exists xz_marketing_wallets (user_id text primary key, balance_cents bigint not null default 0, frozen_cents bigint not null default 0, total_income_cents bigint not null default 0, total_withdraw_cents bigint not null default 0, updated_at timestamptz not null default now());
		create table if not exists xz_marketing_wallet_records (id text primary key, user_id text not null, biz_type text not null, biz_id text not null, amount_cents bigint not null, before_balance_cents bigint not null default 0, after_balance_cents bigint not null default 0, status text not null default 'POSTED', metadata jsonb not null default '{}'::jsonb, created_at timestamptz not null default now());
		create table if not exists xz_marketing_upgrade_plans (id text primary key, from_role text not null, to_role text not null, price_cents bigint not null default 0, condition_type text not null default 'PAID', status text not null default 'ACTIVE', metadata jsonb not null default '{}'::jsonb, created_at timestamptz not null default now(), updated_at timestamptz not null default now());
		create table if not exists xz_marketing_upgrade_records (id text primary key, user_id text not null, from_role text not null, to_role text not null, order_id text, amount_cents bigint not null default 0, status text not null default 'PENDING', metadata jsonb not null default '{}'::jsonb, created_at timestamptz not null default now(), updated_at timestamptz not null default now());
		create table if not exists xz_marketing_commission_rules (id text primary key, name text not null, order_type text not null default 'UPGRADE', earner_role text not null, relation_depth int not null default 1, fixed_amount_cents bigint not null default 0, rate numeric not null default 0, max_total_rate numeric not null default 0, status text not null default 'ACTIVE', metadata jsonb not null default '{}'::jsonb, created_at timestamptz not null default now(), updated_at timestamptz not null default now());
		alter table if exists xz_marketing_invite_records
			add column if not exists tenant_id text not null default 'tenant_default',
			add column if not exists visitor_id text,
			add column if not exists visitor_name text,
			add column if not exists masked_mobile text,
			add column if not exists status text not null default 'visited',
			add column if not exists template_id text not null default 'poster.brand.simple',
			add column if not exists activity_id text,
			add column if not exists visit_time timestamptz,
			add column if not exists register_time timestamptz,
			add column if not exists paid_time timestamptz,
			add column if not exists reward_amount_cents bigint not null default 0,
			add column if not exists reward_status text not null default 'PENDING';
		create index if not exists idx_xz_marketing_org_descendant on xz_marketing_org_relations(descendant_user_id, depth);
		create index if not exists idx_xz_marketing_invite_records_inviter on xz_marketing_invite_records(inviter_user_id, created_at desc);
		create index if not exists idx_xz_marketing_invite_records_tenant_inviter on xz_marketing_invite_records(tenant_id, inviter_user_id, created_at desc);
		create unique index if not exists idx_xz_marketing_invite_records_visit_id on xz_marketing_invite_records(id);
		create index if not exists idx_xz_marketing_wallet_records_user on xz_marketing_wallet_records(user_id, created_at desc);
	`)
	return err
}

func ensureDualIdentityCommerceSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		alter table if exists xz_orders
			add column if not exists buyer_user_id text,
			add column if not exists business_order_type text,
			add column if not exists token_amount bigint not null default 0,
			add column if not exists token_grant_value_cents bigint not null default 0,
			add column if not exists reward_snapshot jsonb not null default '{}'::jsonb;

		create table if not exists xz_agent_profiles (
			id text primary key,
			user_id text not null unique,
			parent_agent_id text,
			operation_center_id text,
			level int not null default 2,
			status text not null default 'ACTIVE',
			invite_code text not null,
			join_order_id text,
			join_fee_cents bigint not null default 0,
			token_rights_amount bigint not null default 0,
			created_at text,
			updated_at text,
			raw jsonb not null default '{}'::jsonb
		);

		create table if not exists xz_user_wallets (
			user_id text primary key,
			token_balance bigint not null default 0,
			cash_balance_cents bigint not null default 0,
			frozen_token bigint not null default 0,
			total_token_granted bigint not null default 0,
			total_token_used bigint not null default 0,
			updated_at timestamptz not null default now(),
			raw jsonb not null default '{}'::jsonb
		);

		create table if not exists xz_agent_wallets (
			agent_id text primary key,
			user_id text not null,
			commission_balance_cents bigint not null default 0,
			withdrawable_balance_cents bigint not null default 0,
			frozen_commission_cents bigint not null default 0,
			total_commission_cents bigint not null default 0,
			total_withdrawn_cents bigint not null default 0,
			updated_at timestamptz not null default now(),
			raw jsonb not null default '{}'::jsonb
		);

		create index if not exists idx_xz_agent_profiles_user_id on xz_agent_profiles(user_id);
		create index if not exists idx_xz_orders_buyer_user_id on xz_orders(buyer_user_id);
	`)
	return err
}
