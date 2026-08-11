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
)

const pricePlanTestWhitelistSelect = `
	select whitelist.id,prices.plan_id,whitelist.price_plan_id,whitelist.user_id,
	       coalesce(whitelist.lifecycle_status,case when whitelist.enabled then 'ACTIVE' else 'DISABLED' end),
	       whitelist.effective_at,whitelist.expires_at,whitelist.reason,whitelist.revision,
	       coalesce(whitelist.created_by,''),coalesce(whitelist.updated_by,''),
	       coalesce(whitelist.disabled_by,''),whitelist.disabled_at,
	       whitelist.created_at,whitelist.updated_at,prices.environment
	from xz_price_plan_user_whitelist whitelist
	join xz_price_plans prices on prices.id=whitelist.price_plan_id
`

type pricePlanTestWhitelistRecord struct {
	item          pricePlanTestWhitelistView
	environment   string
	planVersionID string
}

type pricePlanTestWhitelistScanner interface {
	Scan(...any) error
}

func scanPricePlanTestWhitelist(scanner pricePlanTestWhitelistScanner, now time.Time) (pricePlanTestWhitelistRecord, error) {
	var record pricePlanTestWhitelistRecord
	var validFrom, validUntil, disabledAt sql.NullTime
	err := scanner.Scan(
		&record.item.ID, &record.item.PlanID, &record.item.PricePlanID, &record.item.UserID,
		&record.item.lifecycleStatus, &validFrom, &validUntil, &record.item.Reason, &record.item.Revision,
		&record.item.CreatedBy, &record.item.UpdatedBy, &record.item.DisabledBy, &disabledAt,
		&record.item.CreatedAt, &record.item.UpdatedAt, &record.environment,
	)
	if err != nil {
		return pricePlanTestWhitelistRecord{}, err
	}
	if validFrom.Valid {
		record.item.ValidFrom = &validFrom.Time
	}
	if validUntil.Valid {
		record.item.ValidUntil = &validUntil.Time
	}
	if disabledAt.Valid {
		record.item.DisabledAt = &disabledAt.Time
	}
	record.item.deriveStatus(now)
	return record, nil
}

type pricePlanTestWhitelistPlanContext struct {
	planID, planVersionID, environment string
}

func loadPricePlanTestWhitelistPlan(ctx context.Context, db *sql.DB, pricePlanID string) (pricePlanTestWhitelistPlanContext, error) {
	var result pricePlanTestWhitelistPlanContext
	var priceType string
	err := db.QueryRowContext(ctx, `
		select prices.plan_id,prices.plan_version_id,prices.environment,prices.price_type
		from xz_price_plans prices
		join xz_plans plans on plans.id=prices.plan_id
		join xz_plan_versions versions
		  on versions.id=prices.plan_version_id and versions.plan_id=prices.plan_id
		where prices.id=$1
		  and ((versions.business_type='MEMBER' and plans.plan_type='MEMBER_PACKAGE')
		    or (versions.business_type='AGENT' and plans.plan_type='AGENT_JOIN_PACKAGE'))
	`, strings.TrimSpace(pricePlanID)).Scan(&result.planID, &result.planVersionID, &result.environment, &priceType)
	if errors.Is(err, sql.ErrNoRows) {
		return pricePlanTestWhitelistPlanContext{}, newBusinessPlanAdminError(http.StatusNotFound, "PRICE_PLAN_NOT_MANAGED", "price plan is not a V2 member or agent plan")
	}
	if err != nil {
		return pricePlanTestWhitelistPlanContext{}, err
	}
	if priceType != "TEST" {
		return pricePlanTestWhitelistPlanContext{}, newBusinessPlanAdminError(http.StatusUnprocessableEntity, "PRICE_PLAN_TEST_REQUIRED", "whitelist entries are limited to TEST price plans")
	}
	return result, nil
}

func lockPricePlanTestWhitelistPlan(ctx context.Context, tx *sql.Tx, pricePlanID string) (pricePlanTestWhitelistPlanContext, error) {
	var planID string
	if err := tx.QueryRowContext(ctx, `select plan_id from xz_price_plans where id=$1`, strings.TrimSpace(pricePlanID)).Scan(&planID); errors.Is(err, sql.ErrNoRows) {
		return pricePlanTestWhitelistPlanContext{}, newBusinessPlanAdminError(http.StatusNotFound, "PRICE_PLAN_NOT_MANAGED", "price plan is not a V2 member or agent plan")
	} else if err != nil {
		return pricePlanTestWhitelistPlanContext{}, err
	}
	if _, err := managedBusinessTypeForUpdate(ctx, tx, planID); err != nil {
		return pricePlanTestWhitelistPlanContext{}, newBusinessPlanAdminError(http.StatusNotFound, "PRICE_PLAN_NOT_MANAGED", "price plan is not a V2 member or agent plan")
	}
	var result pricePlanTestWhitelistPlanContext
	var priceType string
	if err := tx.QueryRowContext(ctx, `
		select prices.plan_id,prices.plan_version_id,prices.environment,prices.price_type
		from xz_price_plans prices
		join xz_plans plans on plans.id=prices.plan_id
		join xz_plan_versions versions
		  on versions.id=prices.plan_version_id and versions.plan_id=prices.plan_id
		where prices.id=$1
		  and ((versions.business_type='MEMBER' and plans.plan_type='MEMBER_PACKAGE')
		    or (versions.business_type='AGENT' and plans.plan_type='AGENT_JOIN_PACKAGE'))
		for update of prices
	`, strings.TrimSpace(pricePlanID)).Scan(&result.planID, &result.planVersionID, &result.environment, &priceType); errors.Is(err, sql.ErrNoRows) {
		return pricePlanTestWhitelistPlanContext{}, newBusinessPlanAdminError(http.StatusNotFound, "PRICE_PLAN_NOT_MANAGED", "price plan exact version is not managed by V2 member or agent pricing")
	} else if err != nil {
		return pricePlanTestWhitelistPlanContext{}, err
	}
	if result.planID != planID {
		return pricePlanTestWhitelistPlanContext{}, newBusinessPlanAdminError(http.StatusConflict, "PRICE_PLAN_CONFIGURATION_CHANGED", "price plan ownership changed while it was being locked")
	}
	if priceType != "TEST" {
		return pricePlanTestWhitelistPlanContext{}, newBusinessPlanAdminError(http.StatusUnprocessableEntity, "PRICE_PLAN_TEST_REQUIRED", "whitelist entries are limited to TEST price plans")
	}
	return result, nil
}

func (s *postgresStore) listPricePlanTestWhitelist(ctx context.Context, pricePlanID string) ([]pricePlanTestWhitelistView, error) {
	if _, err := loadPricePlanTestWhitelistPlan(ctx, s.db, pricePlanID); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	rows, err := s.db.QueryContext(ctx, pricePlanTestWhitelistSelect+`
		where whitelist.price_plan_id=$1
		order by whitelist.created_at desc,whitelist.id
	`, strings.TrimSpace(pricePlanID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []pricePlanTestWhitelistView{}
	for rows.Next() {
		record, err := scanPricePlanTestWhitelist(rows, now)
		if err != nil {
			return nil, err
		}
		items = append(items, record.item)
	}
	return items, rows.Err()
}

func (s *postgresStore) listPricePlanTestWhitelistPage(ctx context.Context, pricePlanID string, query pricePlanTestWhitelistQuery) ([]pricePlanTestWhitelistView, int, error) {
	if _, err := loadPricePlanTestWhitelistPlan(ctx, s.db, pricePlanID); err != nil {
		return nil, 0, err
	}
	pricePlanID = strings.TrimSpace(pricePlanID)
	now := time.Now().UTC()
	args := []any{pricePlanID}
	conditions := []string{"whitelist.price_plan_id=$1"}
	if query.Status != "" {
		args = append(args, now)
		nowPlaceholder := len(args)
		args = append(args, query.Status)
		statusPlaceholder := len(args)
		conditions = append(conditions, fmt.Sprintf(`case
			when coalesce(whitelist.lifecycle_status,case when whitelist.enabled then 'ACTIVE' else 'DISABLED' end)='DISABLED' then 'DISABLED'
			when coalesce(whitelist.lifecycle_status,case when whitelist.enabled then 'ACTIVE' else 'DISABLED' end)='EXPIRED' or (whitelist.expires_at is not null and whitelist.expires_at <= $%d) then 'EXPIRED'
			when whitelist.effective_at is not null and whitelist.effective_at > $%d then 'PENDING'
			else 'ACTIVE' end=$%d`, nowPlaceholder, nowPlaceholder, statusPlaceholder))
	}
	if query.UserID != "" {
		args = append(args, query.UserID)
		conditions = append(conditions, fmt.Sprintf("whitelist.user_id=$%d", len(args)))
	}
	where := " where " + strings.Join(conditions, " and ")
	var total int
	if err := s.db.QueryRowContext(ctx, `select count(*) from xz_price_plan_user_whitelist whitelist`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, query.PageSize, (query.Page-1)*query.PageSize)
	rows, err := s.db.QueryContext(ctx, pricePlanTestWhitelistSelect+where+fmt.Sprintf(" order by whitelist.created_at desc,whitelist.id limit $%d offset $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []pricePlanTestWhitelistView{}
	for rows.Next() {
		record, err := scanPricePlanTestWhitelist(rows, now)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, record.item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *postgresStore) createPricePlanTestWhitelist(ctx context.Context, pricePlanID string, mutation pricePlanTestWhitelistCreateMutation, actorID, actorRole string) (pricePlanTestWhitelistView, error) {
	if err := validatePricePlanTestWhitelistCreate(&mutation, actorID); err != nil {
		return pricePlanTestWhitelistView{}, err
	}
	pricePlanID = strings.TrimSpace(pricePlanID)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return pricePlanTestWhitelistView{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `select pg_advisory_xact_lock(hashtext($1),hashtext($2))`, pricePlanID, mutation.UserID); err != nil {
		return pricePlanTestWhitelistView{}, err
	}
	planContext, err := lockPricePlanTestWhitelistPlan(ctx, tx, pricePlanID)
	if err != nil {
		return pricePlanTestWhitelistView{}, err
	}
	var userExists bool
	if err := tx.QueryRowContext(ctx, `select exists(select 1 from xz_users where id=$1)`, mutation.UserID).Scan(&userExists); err != nil {
		return pricePlanTestWhitelistView{}, err
	}
	if !userExists {
		return pricePlanTestWhitelistView{}, newBusinessPlanAdminError(http.StatusNotFound, "WHITELIST_USER_NOT_FOUND", "whitelist user not found")
	}
	now := time.Now().UTC()
	expiredRows, err := tx.QueryContext(ctx, pricePlanTestWhitelistSelect+`
		where whitelist.price_plan_id=$1 and whitelist.user_id=$2
		  and coalesce(whitelist.lifecycle_status,case when whitelist.enabled then 'ACTIVE' else 'DISABLED' end)='ACTIVE'
		  and whitelist.expires_at is not null and whitelist.expires_at <= $3
		for update of whitelist
	`, pricePlanID, mutation.UserID, now)
	if err != nil {
		return pricePlanTestWhitelistView{}, err
	}
	var expired []pricePlanTestWhitelistRecord
	for expiredRows.Next() {
		record, scanErr := scanPricePlanTestWhitelist(expiredRows, now)
		if scanErr != nil {
			expiredRows.Close()
			return pricePlanTestWhitelistView{}, scanErr
		}
		expired = append(expired, record)
	}
	if err := expiredRows.Close(); err != nil {
		return pricePlanTestWhitelistView{}, err
	}
	for _, before := range expired {
		result, err := tx.ExecContext(ctx, `
			update xz_price_plan_user_whitelist
			set lifecycle_status='EXPIRED',enabled=false,updated_by=$2
			where id=$1 and revision=$3
		`, before.item.ID, actorID, before.item.Revision)
		if err != nil {
			return pricePlanTestWhitelistView{}, mapPricePlanTestWhitelistDatabaseError(err)
		}
		if affected, _ := result.RowsAffected(); affected != 1 {
			return pricePlanTestWhitelistView{}, pricingRevisionConflict("whitelist entry")
		}
		after, err := scanPricePlanTestWhitelist(tx.QueryRowContext(ctx, pricePlanTestWhitelistSelect+` where whitelist.id=$1`, before.item.ID), now)
		if err != nil {
			return pricePlanTestWhitelistView{}, err
		}
		if err := insertPricePlanTestWhitelistAudit(ctx, tx, "price_plan.test_whitelist.auto_expire", http.MethodPost, http.StatusOK, actorID, actorRole, mutation.ChangeReason, planContext, &before.item, &after.item); err != nil {
			return pricePlanTestWhitelistView{}, err
		}
	}
	var existingID string
	err = tx.QueryRowContext(ctx, `
		select id from xz_price_plan_user_whitelist
		where price_plan_id=$1 and user_id=$2
		  and coalesce(lifecycle_status,case when enabled then 'ACTIVE' else 'DISABLED' end)='ACTIVE'
		  and (expires_at is null or expires_at > $3)
		limit 1 for update
	`, pricePlanID, mutation.UserID, now).Scan(&existingID)
	if err == nil {
		return pricePlanTestWhitelistView{}, newBusinessPlanAdminError(http.StatusConflict, "WHITELIST_ACTIVE_EXISTS", "an active or pending whitelist entry already exists")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return pricePlanTestWhitelistView{}, err
	}
	entryID := strings.Replace(newAuditID(), "audit_", "price_plan_whitelist_", 1)
	_, err = tx.ExecContext(ctx, `
		insert into xz_price_plan_user_whitelist(
			id,price_plan_id,user_id,enabled,lifecycle_status,effective_at,expires_at,reason,
			created_by,updated_by
		) values($1,$2,$3,true,'ACTIVE',$4,$5,$6,$7,$7)
	`, entryID, pricePlanID, mutation.UserID, mutation.ValidFrom, mutation.ValidUntil, mutation.Reason, actorID)
	if err != nil {
		return pricePlanTestWhitelistView{}, mapPricePlanTestWhitelistDatabaseError(err)
	}
	created, err := scanPricePlanTestWhitelist(tx.QueryRowContext(ctx, pricePlanTestWhitelistSelect+` where whitelist.id=$1`, entryID), now)
	if err != nil {
		return pricePlanTestWhitelistView{}, err
	}
	if err := insertPricePlanTestWhitelistAudit(ctx, tx, "price_plan.test_whitelist.create", http.MethodPost, http.StatusCreated, actorID, actorRole, mutation.ChangeReason, planContext, nil, &created.item); err != nil {
		return pricePlanTestWhitelistView{}, err
	}
	if err := tx.Commit(); err != nil {
		return pricePlanTestWhitelistView{}, err
	}
	return created.item, nil
}

func (s *postgresStore) updatePricePlanTestWhitelist(ctx context.Context, pricePlanID, entryID string, mutation pricePlanTestWhitelistUpdateMutation, actorID, actorRole string) (pricePlanTestWhitelistView, error) {
	if err := validatePricePlanTestWhitelistUpdate(mutation, actorID); err != nil {
		return pricePlanTestWhitelistView{}, err
	}
	return s.mutatePricePlanTestWhitelist(ctx, pricePlanID, entryID, actorID, actorRole, mutation.ChangeReason, func(current pricePlanTestWhitelistView) (pricePlanTestWhitelistView, bool, error) {
		if current.lifecycleStatus != pricePlanWhitelistLifecycleActive || current.Status == pricePlanWhitelistStatusExpired {
			return current, false, newBusinessPlanAdminError(http.StatusConflict, "WHITELIST_ENTRY_TERMINAL", "terminal or elapsed whitelist entries cannot be edited")
		}
		if current.Revision != *mutation.Revision {
			return current, false, pricingRevisionConflict("whitelist entry")
		}
		updated := current
		if mutation.Reason != nil {
			updated.Reason = strings.TrimSpace(*mutation.Reason)
		}
		if mutation.ValidFrom != nil {
			updated.ValidFrom = mutation.ValidFrom
		}
		if mutation.ValidUntil != nil {
			updated.ValidUntil = mutation.ValidUntil
		}
		if mutation.ClearValidFrom {
			updated.ValidFrom = nil
		}
		if mutation.ClearValidUntil {
			updated.ValidUntil = nil
		}
		if err := validatePricePlanTestWhitelistValidity(updated.ValidFrom, updated.ValidUntil); err != nil {
			return current, false, err
		}
		changed := updated.Reason != current.Reason || !sameOptionalTime(updated.ValidFrom, current.ValidFrom) || !sameOptionalTime(updated.ValidUntil, current.ValidUntil)
		return updated, changed, nil
	}, "price_plan.test_whitelist.update")
}

func (s *postgresStore) mutatePricePlanTestWhitelist(
	ctx context.Context,
	pricePlanID, entryID, actorID, actorRole, changeReason string,
	apply func(pricePlanTestWhitelistView) (pricePlanTestWhitelistView, bool, error),
	action string,
) (pricePlanTestWhitelistView, error) {
	pricePlanID = strings.TrimSpace(pricePlanID)
	entryID = strings.TrimSpace(entryID)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return pricePlanTestWhitelistView{}, err
	}
	defer tx.Rollback()
	planContext, err := lockPricePlanTestWhitelistPlan(ctx, tx, pricePlanID)
	if err != nil {
		return pricePlanTestWhitelistView{}, err
	}
	now := time.Now().UTC()
	current, err := scanPricePlanTestWhitelist(tx.QueryRowContext(ctx, pricePlanTestWhitelistSelect+` where whitelist.id=$1 for update of whitelist`, entryID), now)
	if errors.Is(err, sql.ErrNoRows) {
		return pricePlanTestWhitelistView{}, newBusinessPlanAdminError(http.StatusNotFound, "WHITELIST_ENTRY_NOT_FOUND", "whitelist entry not found")
	}
	if err != nil {
		return pricePlanTestWhitelistView{}, err
	}
	if current.item.PricePlanID != pricePlanID {
		return pricePlanTestWhitelistView{}, newBusinessPlanAdminError(http.StatusUnprocessableEntity, "WHITELIST_ENTRY_PRICE_PLAN_MISMATCH", "whitelist entry does not belong to the requested price plan")
	}
	updated, changed, err := apply(current.item)
	if err != nil {
		return pricePlanTestWhitelistView{}, err
	}
	if !changed {
		return current.item, nil
	}
	result, err := tx.ExecContext(ctx, `
		update xz_price_plan_user_whitelist
		set reason=$2,effective_at=$3,expires_at=$4,updated_by=$5
		where id=$1 and revision=$6
	`, entryID, updated.Reason, updated.ValidFrom, updated.ValidUntil, actorID, current.item.Revision)
	if err != nil {
		return pricePlanTestWhitelistView{}, mapPricePlanTestWhitelistDatabaseError(err)
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return pricePlanTestWhitelistView{}, pricingRevisionConflict("whitelist entry")
	}
	after, err := scanPricePlanTestWhitelist(tx.QueryRowContext(ctx, pricePlanTestWhitelistSelect+` where whitelist.id=$1`, entryID), now)
	if err != nil {
		return pricePlanTestWhitelistView{}, err
	}
	if err := insertPricePlanTestWhitelistAudit(ctx, tx, action, http.MethodPatch, http.StatusOK, actorID, actorRole, changeReason, planContext, &current.item, &after.item); err != nil {
		return pricePlanTestWhitelistView{}, err
	}
	if err := tx.Commit(); err != nil {
		return pricePlanTestWhitelistView{}, err
	}
	return after.item, nil
}

func (s *postgresStore) disablePricePlanTestWhitelist(ctx context.Context, pricePlanID, entryID string, mutation pricePlanTestWhitelistDisableMutation, actorID, actorRole string) (pricePlanTestWhitelistView, bool, error) {
	if err := validatePricePlanTestWhitelistDisable(mutation, actorID); err != nil {
		return pricePlanTestWhitelistView{}, false, err
	}
	pricePlanID = strings.TrimSpace(pricePlanID)
	entryID = strings.TrimSpace(entryID)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return pricePlanTestWhitelistView{}, false, err
	}
	defer tx.Rollback()
	planContext, err := lockPricePlanTestWhitelistPlan(ctx, tx, pricePlanID)
	if err != nil {
		return pricePlanTestWhitelistView{}, false, err
	}
	now := time.Now().UTC()
	current, err := scanPricePlanTestWhitelist(tx.QueryRowContext(ctx, pricePlanTestWhitelistSelect+` where whitelist.id=$1 for update of whitelist`, entryID), now)
	if errors.Is(err, sql.ErrNoRows) {
		return pricePlanTestWhitelistView{}, false, newBusinessPlanAdminError(http.StatusNotFound, "WHITELIST_ENTRY_NOT_FOUND", "whitelist entry not found")
	}
	if err != nil {
		return pricePlanTestWhitelistView{}, false, err
	}
	if current.item.PricePlanID != pricePlanID {
		return pricePlanTestWhitelistView{}, false, newBusinessPlanAdminError(http.StatusUnprocessableEntity, "WHITELIST_ENTRY_PRICE_PLAN_MISMATCH", "whitelist entry does not belong to the requested price plan")
	}
	if current.item.lifecycleStatus == pricePlanWhitelistLifecycleDisabled {
		return current.item, true, nil
	}
	if current.item.lifecycleStatus != pricePlanWhitelistLifecycleActive || current.item.Status == pricePlanWhitelistStatusExpired {
		return pricePlanTestWhitelistView{}, false, newBusinessPlanAdminError(http.StatusConflict, "WHITELIST_ENTRY_TERMINAL", "terminal or elapsed whitelist entries cannot be disabled")
	}
	if current.item.Revision != *mutation.Revision {
		return pricePlanTestWhitelistView{}, false, pricingRevisionConflict("whitelist entry")
	}
	result, err := tx.ExecContext(ctx, `
		update xz_price_plan_user_whitelist
		set lifecycle_status='DISABLED',enabled=false,updated_by=$2,disabled_by=$2,disabled_at=$3
		where id=$1 and revision=$4
	`, entryID, actorID, now, current.item.Revision)
	if err != nil {
		return pricePlanTestWhitelistView{}, false, mapPricePlanTestWhitelistDatabaseError(err)
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return pricePlanTestWhitelistView{}, false, pricingRevisionConflict("whitelist entry")
	}
	after, err := scanPricePlanTestWhitelist(tx.QueryRowContext(ctx, pricePlanTestWhitelistSelect+` where whitelist.id=$1`, entryID), now)
	if err != nil {
		return pricePlanTestWhitelistView{}, false, err
	}
	if err := insertPricePlanTestWhitelistAudit(ctx, tx, "price_plan.test_whitelist.disable", http.MethodPost, http.StatusOK, actorID, actorRole, mutation.ChangeReason, planContext, &current.item, &after.item); err != nil {
		return pricePlanTestWhitelistView{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return pricePlanTestWhitelistView{}, false, err
	}
	return after.item, false, nil
}

func insertPricePlanTestWhitelistAudit(
	ctx context.Context,
	tx *sql.Tx,
	action string,
	method string,
	status int,
	actorID, actorRole, changeReason string,
	planContext pricePlanTestWhitelistPlanContext,
	before, after *pricePlanTestWhitelistView,
) error {
	var entryID, userID string
	var revisionBeforeValue, revisionAfterValue int64
	var revisionBefore, revisionAfter *int64
	var beforeSnapshot, afterSnapshot map[string]any
	revisionBeforeValue = 0
	revisionBefore = &revisionBeforeValue
	if before != nil {
		entryID, userID = before.ID, before.UserID
		revisionBeforeValue = before.Revision
		var err error
		beforeSnapshot, err = pricePlanTestWhitelistAuditSnapshot(before)
		if err != nil {
			return err
		}
	}
	if after != nil {
		entryID, userID = after.ID, after.UserID
		revisionAfterValue = after.Revision
		revisionAfter = &revisionAfterValue
		var err error
		afterSnapshot, err = pricePlanTestWhitelistAuditSnapshot(after)
		if err != nil {
			return err
		}
	}
	pricePlanID := ""
	if after != nil {
		pricePlanID = after.PricePlanID
	} else if before != nil {
		pricePlanID = before.PricePlanID
	}
	metadata := map[string]any{
		"planId": planContext.planID, "planVersionId": planContext.planVersionID,
		"pricePlanId":      pricePlanID,
		"whitelistEntryId": entryID, "userId": userID, "changeReason": changeReason,
		"beforeSnapshot": beforeSnapshot, "afterSnapshot": afterSnapshot,
		"revisionBefore": revisionBeforeValue, "revisionAfter": revisionAfterValue,
		"requestId": requestIDFromContext(ctx),
	}
	return insertPricingAuditLog(ctx, tx, pricingAuditMutation{
		ActorID: actorID, ActorRole: actorRole, Action: action, EntityType: "price_plan_test_whitelist",
		EntityID: entryID, Method: method, Status: status, Result: "SUCCEEDED", ChangeReason: changeReason,
		BeforeSnapshot: beforeSnapshot, AfterSnapshot: afterSnapshot,
		RevisionBefore: revisionBefore, RevisionAfter: revisionAfter,
		PlanID: planContext.planID, PlanVersionID: planContext.planVersionID, PricePlanID: pricePlanID,
		WhitelistEntryID: entryID, Environment: planContext.environment, Metadata: metadata,
	})
}

func pricePlanTestWhitelistAuditSnapshot(item *pricePlanTestWhitelistView) (map[string]any, error) {
	raw, err := json.Marshal(item)
	if err != nil {
		return nil, err
	}
	snapshot := map[string]any{}
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return nil, err
	}
	snapshot["lifecycleStatus"] = item.lifecycleStatus
	return snapshot, nil
}

func sameOptionalTime(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}

func mapPricePlanTestWhitelistDatabaseError(err error) error {
	if err == nil {
		return nil
	}
	if isPostgresUniqueViolation(err) {
		return newBusinessPlanAdminError(http.StatusConflict, "WHITELIST_ACTIVE_EXISTS", "an active or pending whitelist entry already exists")
	}
	message := err.Error()
	if strings.Contains(message, "WHITELIST_TEMPORALLY_EXPIRED_IMMUTABLE") || strings.Contains(message, "WHITELIST_TERMINAL_IMMUTABLE") {
		return newBusinessPlanAdminError(http.StatusConflict, "WHITELIST_ENTRY_TERMINAL", "terminal or elapsed whitelist entries cannot be edited")
	}
	return err
}
