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

const pricePlanAdminSelect = `
	select pp.id,pp.plan_id,pp.plan_version_id,pp.code,pp.name,pp.price_type,
	       pp.channel,pp.environment,pp.currency,pp.sale_price_cents,pp.original_price_cents,
	       pp.bonus_points,pp.bonus_tokens,pp.effective_at,pp.expires_at,pp.audience_type,
	       pp.audience_rule,pp.is_visible,pp.is_default,pp.enabled,pp.status,pp.revision,
	       pp.change_reason,coalesce(pp.created_by,''),coalesce(pp.updated_by,''),
	       coalesce(pp.enabled_by,''),pp.enabled_at,coalesce(pp.disabled_by,''),pp.disabled_at,
	       pp.created_at,pp.updated_at,
	       exists(select 1 from xz_order_price_quotes q where q.price_plan_id=pp.id),
	       exists(select 1 from xz_orders o where o.price_plan_id=pp.id)
	from xz_price_plans pp
`

type pricePlanAdminScanner interface {
	Scan(...any) error
}

func scanPricePlanAdmin(scanner pricePlanAdminScanner) (pricePlanAdminView, error) {
	var item pricePlanAdminView
	var validFrom, validUntil, enabledAt, disabledAt sql.NullTime
	var audienceRaw []byte
	err := scanner.Scan(
		&item.ID, &item.PlanID, &item.PlanVersionID, &item.Code, &item.Name, &item.storedKind,
		&item.Channel, &item.Environment, &item.Currency, &item.SalePriceCents, &item.ListPriceCents,
		&item.GiftPoints, &item.GiftTokens, &validFrom, &validUntil, &item.AudienceType,
		&audienceRaw, &item.IsVisible, &item.IsDefault, &item.IsEnabled, &item.Status, &item.Revision,
		&item.ChangeReason, &item.CreatedBy, &item.UpdatedBy, &item.EnabledBy, &enabledAt,
		&item.DisabledBy, &disabledAt, &item.CreatedAt, &item.UpdatedAt, &item.HasQuote, &item.HasOrder,
	)
	if err != nil {
		return pricePlanAdminView{}, err
	}
	item.Kind = publicPricePlanKind(item.storedKind)
	item.AudienceRule = map[string]any{}
	if len(audienceRaw) > 0 {
		if err := json.Unmarshal(audienceRaw, &item.AudienceRule); err != nil {
			return pricePlanAdminView{}, err
		}
	}
	if validFrom.Valid {
		item.ValidFrom = &validFrom.Time
	}
	if validUntil.Valid {
		item.ValidUntil = &validUntil.Time
	}
	if enabledAt.Valid {
		item.EnabledAt = &enabledAt.Time
	}
	if disabledAt.Valid {
		item.DisabledAt = &disabledAt.Time
	}
	item.EconomicFieldsLock = item.Status != "DRAFT" || item.EnabledAt != nil || item.HasQuote || item.HasOrder
	return item, nil
}

func (s *postgresStore) listPricePlans(ctx context.Context, planID string) ([]pricePlanAdminView, error) {
	if _, err := s.businessPlan(ctx, planID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, pricePlanAdminSelect+` where pp.plan_id=$1 order by pp.created_at desc,pp.id`, strings.TrimSpace(planID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []pricePlanAdminView{}
	for rows.Next() {
		item, err := scanPricePlanAdmin(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) pricePlan(ctx context.Context, pricePlanID string) (pricePlanAdminView, error) {
	item, err := scanPricePlanAdmin(s.db.QueryRowContext(ctx, pricePlanAdminSelect+` where pp.id=$1`, strings.TrimSpace(pricePlanID)))
	if errors.Is(err, sql.ErrNoRows) {
		return pricePlanAdminView{}, newBusinessPlanAdminError(http.StatusNotFound, "PRICE_PLAN_NOT_FOUND", "price plan not found")
	}
	if err != nil {
		return pricePlanAdminView{}, err
	}
	if _, err := s.businessPlan(ctx, item.PlanID); err != nil {
		return pricePlanAdminView{}, newBusinessPlanAdminError(http.StatusNotFound, "PRICE_PLAN_NOT_MANAGED", "price plan is not a V2 member or agent plan")
	}
	return item, nil
}

func (s *postgresStore) createPricePlan(ctx context.Context, planID string, mutation pricePlanCreateMutation, actorID, actorRole string) (pricePlanAdminView, error) {
	if err := pricingAdminActor(actorID); err != nil {
		return pricePlanAdminView{}, err
	}
	if err := validatePricePlanCreateMutation(&mutation); err != nil {
		return pricePlanAdminView{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return pricePlanAdminView{}, err
	}
	defer tx.Rollback()
	planID = strings.TrimSpace(planID)
	if _, err := managedBusinessTypeForUpdate(ctx, tx, planID); err != nil {
		return pricePlanAdminView{}, err
	}
	version, err := loadBusinessPlanVersionForUpdate(ctx, tx, mutation.PlanVersionID)
	if err != nil {
		return pricePlanAdminView{}, err
	}
	if version.PlanID != planID {
		return pricePlanAdminView{}, newBusinessPlanAdminError(http.StatusBadRequest, "PRICE_PLAN_VERSION_MISMATCH", "plan version does not belong to the business plan")
	}
	if mutation.Revision == nil || version.Revision != *mutation.Revision {
		return pricePlanAdminView{}, newBusinessPlanAdminError(http.StatusConflict, "REVISION_CONFLICT", "plan version revision conflict")
	}
	if version.Status != "ACTIVE" {
		return pricePlanAdminView{}, newBusinessPlanAdminError(http.StatusUnprocessableEntity, "PRICE_PLAN_VERSION_NOT_ACTIVE", "price plan must bind an ACTIVE entitlement version")
	}
	audienceRaw, err := json.Marshal(mutation.AudienceRule)
	if err != nil {
		return pricePlanAdminView{}, err
	}
	pricePlanID := strings.Replace(newAuditID(), "audit_", "price_plan_", 1)
	_, err = tx.ExecContext(ctx, `
		insert into xz_price_plans(
			id,plan_id,plan_version_id,code,name,price_type,channel,environment,currency,
			sale_price_cents,original_price_cents,bonus_points,bonus_tokens,effective_at,expires_at,
			audience_type,audience_rule,is_visible,is_default,enabled,status,
			created_by,updated_by,change_reason
		) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17::jsonb,$18,false,false,'DRAFT',$19,$19,$20)
	`, pricePlanID, planID, version.ID, strings.TrimSpace(mutation.Code), mutation.Name, mutation.Kind,
		mutation.Channel, mutation.Environment, mutation.Currency, mutation.SalePriceCents, mutation.ListPriceCents,
		mutation.GiftPoints, mutation.GiftTokens, mutation.ValidFrom, mutation.ValidUntil, mutation.AudienceType,
		audienceRaw, *mutation.IsVisible, actorID, strings.TrimSpace(mutation.ChangeReason))
	if isPostgresUniqueViolation(err) {
		return pricePlanAdminView{}, newBusinessPlanAdminError(http.StatusConflict, "PRICE_PLAN_CODE_EXISTS", "price plan code already exists")
	}
	if err != nil {
		return pricePlanAdminView{}, err
	}
	created, err := scanPricePlanAdmin(tx.QueryRowContext(ctx, pricePlanAdminSelect+` where pp.id=$1`, pricePlanID))
	if err != nil {
		return pricePlanAdminView{}, err
	}
	if err := insertAuditLog(ctx, tx, actorID, actorRole, "price_plan.create", "price_plan", created.ID, "POST", "", http.StatusCreated, map[string]any{
		"planId": planID, "planVersionId": version.ID, "pricePlanId": created.ID, "code": created.Code, "kind": created.Kind,
		"channel": created.Channel, "environment": created.Environment, "currency": created.Currency,
		"salePriceCents": created.SalePriceCents, "changeReason": mutation.ChangeReason,
		"revisionBefore": int64(0), "revisionAfter": created.Revision, "afterSnapshot": created,
	}); err != nil {
		return pricePlanAdminView{}, err
	}
	if err := tx.Commit(); err != nil {
		return pricePlanAdminView{}, err
	}
	return created, nil
}

func loadPricePlanAdminForUpdate(ctx context.Context, tx *sql.Tx, pricePlanID string) (pricePlanAdminView, error) {
	item, err := scanPricePlanAdmin(tx.QueryRowContext(ctx, pricePlanAdminSelect+` where pp.id=$1 for update of pp`, strings.TrimSpace(pricePlanID)))
	if errors.Is(err, sql.ErrNoRows) {
		return pricePlanAdminView{}, newBusinessPlanAdminError(http.StatusNotFound, "PRICE_PLAN_NOT_FOUND", "price plan not found")
	}
	return item, err
}

func (s *postgresStore) updatePricePlan(ctx context.Context, pricePlanID string, mutation pricePlanUpdateMutation, actorID, actorRole string) (pricePlanAdminView, error) {
	if err := pricingAdminActor(actorID); err != nil {
		return pricePlanAdminView{}, err
	}
	if err := requirePricePlanWrite(mutation.Revision, mutation.ChangeReason); err != nil {
		return pricePlanAdminView{}, err
	}
	var planID, currentVersionID string
	if err := s.db.QueryRowContext(ctx, `select plan_id,plan_version_id from xz_price_plans where id=$1`, strings.TrimSpace(pricePlanID)).Scan(&planID, &currentVersionID); errors.Is(err, sql.ErrNoRows) {
		return pricePlanAdminView{}, newBusinessPlanAdminError(http.StatusNotFound, "PRICE_PLAN_NOT_FOUND", "price plan not found")
	} else if err != nil {
		return pricePlanAdminView{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return pricePlanAdminView{}, err
	}
	defer tx.Rollback()
	if _, err := managedBusinessTypeForUpdate(ctx, tx, planID); err != nil {
		return pricePlanAdminView{}, err
	}
	versionIDs := []string{currentVersionID}
	if mutation.PlanVersionID != nil && strings.TrimSpace(*mutation.PlanVersionID) != currentVersionID {
		versionIDs = append(versionIDs, strings.TrimSpace(*mutation.PlanVersionID))
	}
	for _, versionID := range sortedUniqueStrings(versionIDs) {
		if _, err := loadBusinessPlanVersionForUpdate(ctx, tx, versionID); err != nil {
			return pricePlanAdminView{}, err
		}
	}
	current, err := loadPricePlanAdminForUpdate(ctx, tx, pricePlanID)
	if err != nil {
		return pricePlanAdminView{}, err
	}
	if current.PlanID != planID || current.PlanVersionID != currentVersionID {
		return pricePlanAdminView{}, newBusinessPlanAdminError(http.StatusConflict, "PRICE_PLAN_CONFIGURATION_CHANGED", "price plan ownership changed while it was being locked")
	}
	if current.Revision != *mutation.Revision {
		return pricePlanAdminView{}, pricingRevisionConflict("price plan")
	}
	updated, economicChanged, err := applyPricePlanUpdate(current, mutation)
	if err != nil {
		return pricePlanAdminView{}, err
	}
	if !economicChanged && current.Name == updated.Name {
		return pricePlanAdminView{}, newBusinessPlanAdminError(http.StatusBadRequest, "PRICE_PLAN_MUTATION_REQUIRED", "at least one mutable field is required")
	}
	if economicChanged && current.EconomicFieldsLock {
		return pricePlanAdminView{}, newBusinessPlanAdminError(http.StatusConflict, "PRICE_PLAN_CLONE_REQUIRED", "economic fields are immutable after activation, quote or order history")
	}
	if economicChanged {
		var activeBinding bool
		if err := tx.QueryRowContext(ctx, `
			select exists(select 1 from xz_price_plan_payment_bindings where price_plan_id=$1 and enabled=true and status='ACTIVE')
		`, current.ID).Scan(&activeBinding); err != nil {
			return pricePlanAdminView{}, err
		}
		if activeBinding {
			return pricePlanAdminView{}, newBusinessPlanAdminError(http.StatusConflict, "PRICE_PLAN_ACTIVE_BINDING_REQUIRES_DISABLE", "disable the active payment binding before changing draft economics")
		}
	}
	version, err := loadBusinessPlanVersionForUpdate(ctx, tx, updated.PlanVersionID)
	if err != nil {
		return pricePlanAdminView{}, err
	}
	if version.PlanID != current.PlanID || version.Status != "ACTIVE" {
		return pricePlanAdminView{}, newBusinessPlanAdminError(http.StatusUnprocessableEntity, "PRICE_PLAN_VERSION_NOT_ACTIVE", "price plan must bind an ACTIVE entitlement version")
	}
	audienceRaw, err := json.Marshal(updated.AudienceRule)
	if err != nil {
		return pricePlanAdminView{}, err
	}
	result, err := tx.ExecContext(ctx, `
		update xz_price_plans set
			name=$2,plan_version_id=$3,price_type=$4,channel=$5,environment=$6,currency=$7,
			sale_price_cents=$8,original_price_cents=$9,bonus_points=$10,bonus_tokens=$11,
			effective_at=$12,expires_at=$13,audience_type=$14,audience_rule=$15::jsonb,is_visible=$16,
			updated_by=$17,change_reason=$18
		where id=$1 and revision=$19
	`, current.ID, updated.Name, updated.PlanVersionID, updated.storedKind, updated.Channel, updated.Environment,
		updated.Currency, updated.SalePriceCents, updated.ListPriceCents, updated.GiftPoints, updated.GiftTokens,
		updated.ValidFrom, updated.ValidUntil, updated.AudienceType, audienceRaw, updated.IsVisible, actorID,
		strings.TrimSpace(mutation.ChangeReason), *mutation.Revision)
	if err != nil {
		return pricePlanAdminView{}, mapPricePlanDatabaseError(err)
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return pricePlanAdminView{}, pricingRevisionConflict("price plan")
	}
	updated, err = scanPricePlanAdmin(tx.QueryRowContext(ctx, pricePlanAdminSelect+` where pp.id=$1`, current.ID))
	if err != nil {
		return pricePlanAdminView{}, err
	}
	if err := insertAuditLog(ctx, tx, actorID, actorRole, "price_plan.update", "price_plan", current.ID, "PATCH", "", http.StatusOK, map[string]any{
		"planId": current.PlanID, "planVersionId": updated.PlanVersionID, "pricePlanId": current.ID,
		"revisionBefore": current.Revision, "revisionAfter": updated.Revision,
		"economicFieldsChanged": economicChanged, "changeReason": mutation.ChangeReason,
		"environment": updated.Environment, "beforeSnapshot": current, "afterSnapshot": updated,
	}); err != nil {
		return pricePlanAdminView{}, err
	}
	if err := tx.Commit(); err != nil {
		return pricePlanAdminView{}, err
	}
	return updated, nil
}

func (s *postgresStore) clonePricePlan(ctx context.Context, pricePlanID string, mutation pricePlanCloneMutation, actorID, actorRole string) (pricePlanAdminView, error) {
	if err := pricingAdminActor(actorID); err != nil {
		return pricePlanAdminView{}, err
	}
	if err := requirePricePlanWrite(mutation.Revision, mutation.ChangeReason); err != nil {
		return pricePlanAdminView{}, err
	}
	if err := validatePricePlanCode(mutation.Code); err != nil {
		return pricePlanAdminView{}, err
	}
	var planID, versionID string
	if err := s.db.QueryRowContext(ctx, `select plan_id,plan_version_id from xz_price_plans where id=$1`, strings.TrimSpace(pricePlanID)).Scan(&planID, &versionID); errors.Is(err, sql.ErrNoRows) {
		return pricePlanAdminView{}, newBusinessPlanAdminError(http.StatusNotFound, "PRICE_PLAN_NOT_FOUND", "price plan not found")
	} else if err != nil {
		return pricePlanAdminView{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return pricePlanAdminView{}, err
	}
	defer tx.Rollback()
	if _, err := managedBusinessTypeForUpdate(ctx, tx, planID); err != nil {
		return pricePlanAdminView{}, err
	}
	version, err := loadBusinessPlanVersionForUpdate(ctx, tx, versionID)
	if err != nil {
		return pricePlanAdminView{}, err
	}
	current, err := loadPricePlanAdminForUpdate(ctx, tx, pricePlanID)
	if err != nil {
		return pricePlanAdminView{}, err
	}
	if current.Revision != *mutation.Revision {
		return pricePlanAdminView{}, pricingRevisionConflict("price plan")
	}
	if version.PlanID != current.PlanID || version.Status != "ACTIVE" {
		return pricePlanAdminView{}, newBusinessPlanAdminError(http.StatusUnprocessableEntity, "PRICE_PLAN_VERSION_NOT_ACTIVE", "cloned price plan must bind an ACTIVE entitlement version")
	}
	name := strings.TrimSpace(mutation.Name)
	if name == "" {
		name = current.Name + " copy"
	}
	cloneAudienceType := current.AudienceType
	cloneVisible := current.IsVisible
	normalizedLegacyTestScope := false
	if current.storedKind == "TEST" {
		cloneVisible = false
		if cloneAudienceType == "PUBLIC" {
			cloneAudienceType = "TEST"
			normalizedLegacyTestScope = true
		}
	}
	audienceRaw, err := json.Marshal(current.AudienceRule)
	if err != nil {
		return pricePlanAdminView{}, err
	}
	cloneID := strings.Replace(newAuditID(), "audit_", "price_plan_", 1)
	_, err = tx.ExecContext(ctx, `
		insert into xz_price_plans(
			id,plan_id,plan_version_id,code,name,price_type,channel,environment,currency,
			sale_price_cents,original_price_cents,bonus_points,bonus_tokens,effective_at,expires_at,
			audience_type,audience_rule,is_visible,is_default,enabled,status,created_by,updated_by,change_reason
		) values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17::jsonb,$18,false,false,'DRAFT',$19,$19,$20)
	`, cloneID, current.PlanID, current.PlanVersionID, strings.TrimSpace(mutation.Code), name, current.storedKind,
		current.Channel, current.Environment, current.Currency, current.SalePriceCents, current.ListPriceCents,
		current.GiftPoints, current.GiftTokens, current.ValidFrom, current.ValidUntil, cloneAudienceType,
		audienceRaw, cloneVisible, actorID, strings.TrimSpace(mutation.ChangeReason))
	if isPostgresUniqueViolation(err) {
		return pricePlanAdminView{}, newBusinessPlanAdminError(http.StatusConflict, "PRICE_PLAN_CODE_EXISTS", "price plan code already exists")
	}
	if err != nil {
		return pricePlanAdminView{}, err
	}
	cloned, err := scanPricePlanAdmin(tx.QueryRowContext(ctx, pricePlanAdminSelect+` where pp.id=$1`, cloneID))
	if err != nil {
		return pricePlanAdminView{}, err
	}
	if err := insertAuditLog(ctx, tx, actorID, actorRole, "price_plan.clone", "price_plan", cloned.ID, "POST", "", http.StatusCreated, map[string]any{
		"sourcePricePlanId": current.ID, "planId": current.PlanID, "planVersionId": cloned.PlanVersionID, "pricePlanId": cloned.ID,
		"sourceRevision": current.Revision, "revisionBefore": int64(0), "revisionAfter": cloned.Revision,
		"code": cloned.Code, "changeReason": mutation.ChangeReason,
		"sourceAudienceType": current.AudienceType, "clonedAudienceType": cloned.AudienceType,
		"normalizedLegacyTestScope": normalizedLegacyTestScope, "environment": cloned.Environment,
		"afterSnapshot": cloned, "sourceSnapshot": current,
	}); err != nil {
		return pricePlanAdminView{}, err
	}
	if err := tx.Commit(); err != nil {
		return pricePlanAdminView{}, err
	}
	return cloned, nil
}

func (s *postgresStore) validatePricePlan(ctx context.Context, pricePlanID string) (pricePlanValidationResult, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return pricePlanValidationResult{}, err
	}
	defer tx.Rollback()
	price, err := scanPricePlanAdmin(tx.QueryRowContext(ctx, pricePlanAdminSelect+` where pp.id=$1`, strings.TrimSpace(pricePlanID)))
	if errors.Is(err, sql.ErrNoRows) {
		return pricePlanValidationResult{}, newBusinessPlanAdminError(http.StatusNotFound, "PRICE_PLAN_NOT_FOUND", "price plan not found")
	}
	if err != nil {
		return pricePlanValidationResult{}, err
	}
	var planActive bool
	var planType, businessType string
	if err := tx.QueryRowContext(ctx, `
		select coalesce(p.plan_type,''),p.active,pv.business_type
		from xz_plans p join xz_plan_versions pv on pv.id=$2 and pv.plan_id=p.id
		where p.id=$1
	`, price.PlanID, price.PlanVersionID).Scan(&planType, &planActive, &businessType); errors.Is(err, sql.ErrNoRows) {
		return pricePlanValidationResult{}, newBusinessPlanAdminError(http.StatusNotFound, "PRICE_PLAN_NOT_MANAGED", "price plan is not a V2 member or agent plan")
	} else if err != nil {
		return pricePlanValidationResult{}, err
	}
	managed := (planType == planTypeMemberPackage && businessType == "MEMBER") ||
		(planType == planTypeAgentJoinPackage && businessType == "AGENT")
	if !managed {
		return pricePlanValidationResult{}, newBusinessPlanAdminError(http.StatusNotFound, "PRICE_PLAN_NOT_MANAGED", "price plan is not a V2 member or agent plan")
	}
	var version businessPlanVersionView
	version, err = scanBusinessPlanVersion(tx.QueryRowContext(ctx, businessPlanVersionSelect+` where id=$1`, price.PlanVersionID))
	if err != nil {
		return pricePlanValidationResult{}, err
	}
	var binding *paymentBindingRow
	var good *wechatVirtualGoodAdminView
	var bindingRow paymentBindingRow
	err = tx.QueryRowContext(ctx, `
		select id,price_plan_id,wechat_good_id,channel,environment,provider_price_snapshot_cents,
		       enabled,status,revision
		from xz_price_plan_payment_bindings
		where price_plan_id=$1 and enabled=true and status='ACTIVE'
	`, price.ID).Scan(&bindingRow.ID, &bindingRow.PricePlanID, &bindingRow.WeChatGoodID, &bindingRow.Channel,
		&bindingRow.Environment, &bindingRow.ProviderPriceSnapshotCents, &bindingRow.Enabled, &bindingRow.Status,
		&bindingRow.Revision)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return pricePlanValidationResult{}, err
	}
	if err == nil {
		binding = &bindingRow
		goodRow, goodErr := scanWechatVirtualGood(tx.QueryRowContext(ctx, wechatVirtualGoodAdminSelect+` where id=$1`, bindingRow.WeChatGoodID))
		if goodErr != nil && !errors.Is(goodErr, sql.ErrNoRows) {
			return pricePlanValidationResult{}, goodErr
		}
		if goodErr == nil {
			good = &goodRow
		}
	}
	result, _ := evaluatePricePlanActivation(price, planActive, version, binding, good, time.Now().UTC())
	if err := tx.Commit(); err != nil {
		return pricePlanValidationResult{}, err
	}
	return result, nil
}

func evaluatePricePlanActivation(price pricePlanAdminView, planActive bool, version businessPlanVersionView, binding *paymentBindingRow, good *wechatVirtualGoodAdminView, now time.Time) (pricePlanValidationResult, error) {
	result := pricePlanValidationResult{
		PricePlanID: price.ID, CheckedAt: now, PricePlanPriceCents: price.SalePriceCents,
		Checks: make([]pricePlanValidationCheck, 0, 10),
	}
	var firstErr error
	add := func(code string, passed bool, message string) {
		check := pricePlanValidationCheck{Code: code, Passed: passed}
		if !passed {
			check.Message = message
		}
		result.Checks = append(result.Checks, check)
		if !passed && firstErr == nil {
			firstErr = newBusinessPlanAdminError(http.StatusUnprocessableEntity, code, message)
		}
	}
	add("BUSINESS_PLAN_INACTIVE", planActive, "business plan must be active")
	add("PRICE_PLAN_VERSION_NOT_ACTIVE", version.ID == price.PlanVersionID && version.PlanID == price.PlanID && version.Status == "ACTIVE", "price plan must bind an ACTIVE entitlement version")
	versionWithinValidity := (version.EffectiveAt == nil || !now.Before(*version.EffectiveAt)) && (version.ExpiresAt == nil || now.Before(*version.ExpiresAt))
	add("PRICE_PLAN_VERSION_OUTSIDE_VALIDITY", versionWithinValidity, "entitlement version is outside its validity window")
	commissionRules, commissionErr := decodeCommissionRuleSnapshots(version.CommissionSnapshot)
	if commissionErr == nil && len(commissionRules) > 0 {
		_, commissionErr = rebuildCommissionRulesFromSnapshot(commissionRuleSnapshotContext{
			TenantID: "price-plan-activation", ProductType: version.BusinessType,
			ProductID: version.PlanID, PaidAt: now,
		}, commissionRules)
	}
	add("PRICE_PLAN_COMMISSION_SNAPSHOT_INVALID", commissionErr == nil, "entitlement version commission snapshot must be complete and self-contained")
	withinValidity := (price.ValidFrom == nil || !now.Before(*price.ValidFrom)) && (price.ValidUntil == nil || now.Before(*price.ValidUntil))
	add("PRICE_PLAN_OUTSIDE_VALIDITY", withinValidity, "current time is outside the price plan validity window")
	add("PRICE_PLAN_CURRENCY_INVALID", price.Currency == "CNY", "WeChat virtual price plans currently require CNY")
	add("PRICE_PLAN_GIFT_POINTS_FULFILLMENT_UNAVAILABLE", price.GiftPoints == 0, "giftPoints cannot be enabled until an independent idempotent points fulfillment path is available")
	rightsCopy := map[string]any{}
	for key, value := range version.RightsSnapshot {
		rightsCopy[key] = value
	}
	rightsErr := mergePricePlanBonusRights(rightsCopy, price.GiftPoints, price.GiftTokens)
	add("PRICE_PLAN_GIFT_RIGHTS_INVALID", rightsErr == nil, "price plan gifts cannot be merged safely into the entitlement snapshot")
	bindingActive := binding != nil && binding.PricePlanID == price.ID && binding.Enabled && binding.Status == "ACTIVE"
	add("PRICE_PLAN_BINDING_NOT_ACTIVE", bindingActive, "an ACTIVE payment binding is required")
	if binding != nil {
		result.PaymentBindingID = binding.ID
		result.WeChatGoodID = binding.WeChatGoodID
		result.BindingPriceCents = binding.ProviderPriceSnapshotCents
	}
	goodConfirmed := good != nil && good.manuallyConfirmedAt(now)
	confirmationCode := "WECHAT_GOOD_NOT_CONFIRMED"
	confirmationMessage := "WeChat good must be manually confirmed as published"
	if good != nil && good.recordedVerificationStatus == wechatGoodVerificationManual && good.VerificationExpiresAt != nil && !now.Before(*good.VerificationExpiresAt) {
		confirmationCode = "WECHAT_GOOD_VERIFICATION_EXPIRED"
		confirmationMessage = "manual WeChat publication confirmation has expired"
	}
	add(confirmationCode, goodConfirmed, confirmationMessage)
	goodAvailable := good != nil && good.Enabled && good.Published && good.Status == "PUBLISHED"
	add("WECHAT_GOOD_NOT_AVAILABLE", goodAvailable, "WeChat good is not locally enabled and published")
	if good != nil {
		result.WeChatGoodID = good.ID
		result.WeChatProductID = good.ProductID
		result.WeChatGoodPriceCents = good.PlatformPriceCents
	}
	identityConsistent := binding != nil && good != nil && binding.WeChatGoodID == good.ID &&
		price.Channel == binding.Channel && price.Channel == good.Channel &&
		price.Environment == binding.Environment && price.Environment == good.Environment
	add("PRICE_PLAN_PAYMENT_ENV_MISMATCH", identityConsistent, "price plan, binding and WeChat good channel/environment must match")
	priceConsistent := binding != nil && good != nil && price.SalePriceCents == binding.ProviderPriceSnapshotCents &&
		price.SalePriceCents == good.PlatformPriceCents
	priceMessage := fmt.Sprintf("pricePlan=%d binding=%d wechatGood=%d", price.SalePriceCents, result.BindingPriceCents, result.WeChatGoodPriceCents)
	add("PRICE_PLAN_WECHAT_PRICE_MISMATCH", priceConsistent, priceMessage)
	result.Valid = firstErr == nil
	return result, firstErr
}

func (s *postgresStore) transitionPricePlan(ctx context.Context, pricePlanID string, mutation pricePlanTransitionMutation, enable bool, actorID, actorRole string) (pricePlanAdminView, error) {
	if err := pricingAdminActor(actorID); err != nil {
		return pricePlanAdminView{}, err
	}
	if err := requirePricePlanWrite(mutation.Revision, mutation.ChangeReason); err != nil {
		return pricePlanAdminView{}, err
	}
	pricePlanID = strings.TrimSpace(pricePlanID)
	var planID, versionID string
	if err := s.db.QueryRowContext(ctx, `select plan_id,plan_version_id from xz_price_plans where id=$1`, pricePlanID).Scan(&planID, &versionID); errors.Is(err, sql.ErrNoRows) {
		return pricePlanAdminView{}, newBusinessPlanAdminError(http.StatusNotFound, "PRICE_PLAN_NOT_FOUND", "price plan not found")
	} else if err != nil {
		return pricePlanAdminView{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return pricePlanAdminView{}, err
	}
	defer tx.Rollback()
	if _, err := managedBusinessTypeForUpdate(ctx, tx, planID); err != nil {
		return pricePlanAdminView{}, err
	}
	var planActive bool
	if err := tx.QueryRowContext(ctx, `select active from xz_plans where id=$1`, planID).Scan(&planActive); err != nil {
		return pricePlanAdminView{}, err
	}
	version, err := loadBusinessPlanVersionForUpdate(ctx, tx, versionID)
	if err != nil {
		return pricePlanAdminView{}, err
	}
	current, err := loadPricePlanAdminForUpdate(ctx, tx, pricePlanID)
	if err != nil {
		return pricePlanAdminView{}, err
	}
	if current.PlanID != planID || current.PlanVersionID != versionID {
		return pricePlanAdminView{}, newBusinessPlanAdminError(http.StatusConflict, "PRICE_PLAN_CONFIGURATION_CHANGED", "price plan ownership changed while it was being locked")
	}
	if current.Revision != *mutation.Revision {
		return pricePlanAdminView{}, pricingRevisionConflict("price plan")
	}
	now := time.Now().UTC()
	action := "price_plan.disable"
	if enable {
		action = "price_plan.enable"
		stateAllowsEnable := (!current.IsEnabled && (current.Status == "DRAFT" || current.Status == "INACTIVE")) ||
			(current.IsEnabled && current.Status == "ACTIVE")
		if !stateAllowsEnable {
			return pricePlanAdminView{}, newBusinessPlanAdminError(http.StatusConflict, "PRICE_PLAN_STATE_INVALID", "price plan state cannot transition to ACTIVE; EXPIRED plans must be cloned as a new DRAFT")
		}
		var bindingID, goodID string
		err := tx.QueryRowContext(ctx, `
			select id,wechat_good_id from xz_price_plan_payment_bindings
			where price_plan_id=$1 and enabled=true and status='ACTIVE'
		`, current.ID).Scan(&bindingID, &goodID)
		var binding *paymentBindingRow
		var good *wechatVirtualGoodAdminView
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return pricePlanAdminView{}, err
		}
		if err == nil {
			goodRow, goodErr := loadWechatVirtualGoodForUpdate(ctx, tx, goodID)
			if goodErr != nil {
				return pricePlanAdminView{}, goodErr
			}
			bindingRow, bindingErr := loadPaymentBindingForUpdate(ctx, tx, bindingID)
			if bindingErr != nil {
				return pricePlanAdminView{}, bindingErr
			}
			if bindingRow.PricePlanID != current.ID || bindingRow.WeChatGoodID != goodRow.ID {
				return pricePlanAdminView{}, newBusinessPlanAdminError(http.StatusConflict, "PRICE_PLAN_CONFIGURATION_CHANGED", "payment binding changed while it was being locked")
			}
			binding = &bindingRow
			good = &goodRow
		}
		validation, validationErr := evaluatePricePlanActivation(current, planActive, version, binding, good, now)
		if validationErr != nil {
			return pricePlanAdminView{}, validationErr
		}
		if current.IsEnabled && current.Status == "ACTIVE" {
			return current, nil
		}
		result, err := tx.ExecContext(ctx, `
			update xz_price_plans set enabled=true,status='ACTIVE',updated_by=$2,change_reason=$3,
				enabled_by=coalesce(enabled_by,$2),enabled_at=coalesce(enabled_at,$4),
				disabled_by=null,disabled_at=null
			where id=$1 and revision=$5
		`, current.ID, actorID, strings.TrimSpace(mutation.ChangeReason), now, *mutation.Revision)
		if err != nil {
			return pricePlanAdminView{}, mapPricePlanDatabaseError(err)
		}
		if affected, _ := result.RowsAffected(); affected != 1 {
			return pricePlanAdminView{}, pricingRevisionConflict("price plan")
		}
		updated, err := scanPricePlanAdmin(tx.QueryRowContext(ctx, pricePlanAdminSelect+` where pp.id=$1`, current.ID))
		if err != nil {
			return pricePlanAdminView{}, err
		}
		if err := insertAuditLog(ctx, tx, actorID, actorRole, action, "price_plan", current.ID, "POST", "", http.StatusOK, map[string]any{
			"planId": current.PlanID, "planVersionId": current.PlanVersionID, "pricePlanId": current.ID,
			"paymentBindingId": binding.ID, "wechatGoodId": good.ID, "environment": current.Environment,
			"revisionBefore": current.Revision, "revisionAfter": updated.Revision,
			"changeReason": mutation.ChangeReason, "validation": validation,
			"beforeSnapshot": current, "afterSnapshot": updated,
		}); err != nil {
			return pricePlanAdminView{}, err
		}
		if err := tx.Commit(); err != nil {
			return pricePlanAdminView{}, err
		}
		return updated, nil
	}
	if current.IsDefault {
		return pricePlanAdminView{}, newBusinessPlanAdminError(http.StatusConflict, "PRICE_PLAN_DEFAULT_DISABLE_FORBIDDEN", "switch the default price plan before disabling it")
	}
	if !current.IsEnabled && current.Status == "INACTIVE" {
		return current, nil
	}
	if current.Status == "DRAFT" {
		return pricePlanAdminView{}, newBusinessPlanAdminError(http.StatusConflict, "PRICE_PLAN_NOT_ENABLED", "a DRAFT price plan cannot be disabled")
	}
	if current.Status != "ACTIVE" || !current.IsEnabled {
		return pricePlanAdminView{}, newBusinessPlanAdminError(http.StatusConflict, "PRICE_PLAN_STATE_INVALID", "only an enabled ACTIVE price plan can transition to INACTIVE")
	}
	result, err := tx.ExecContext(ctx, `
		update xz_price_plans set enabled=false,status='INACTIVE',updated_by=$2,change_reason=$3,
			disabled_by=$2,disabled_at=$4
		where id=$1 and revision=$5
	`, current.ID, actorID, strings.TrimSpace(mutation.ChangeReason), now, *mutation.Revision)
	if err != nil {
		return pricePlanAdminView{}, mapPricePlanDatabaseError(err)
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return pricePlanAdminView{}, pricingRevisionConflict("price plan")
	}
	updated, err := scanPricePlanAdmin(tx.QueryRowContext(ctx, pricePlanAdminSelect+` where pp.id=$1`, current.ID))
	if err != nil {
		return pricePlanAdminView{}, err
	}
	if err := insertAuditLog(ctx, tx, actorID, actorRole, action, "price_plan", current.ID, "POST", "", http.StatusOK, map[string]any{
		"planId": current.PlanID, "planVersionId": current.PlanVersionID, "pricePlanId": current.ID,
		"environment": current.Environment, "revisionBefore": current.Revision, "revisionAfter": updated.Revision,
		"changeReason": mutation.ChangeReason, "beforeSnapshot": current, "afterSnapshot": updated,
	}); err != nil {
		return pricePlanAdminView{}, err
	}
	if err := tx.Commit(); err != nil {
		return pricePlanAdminView{}, err
	}
	return updated, nil
}

type pricePlanDefaultGroup struct {
	PlanID      string
	VersionID   string
	Channel     string
	Environment string
	Currency    string
}

func (g pricePlanDefaultGroup) lockKey() string {
	return strings.Join([]string{
		"xz:price-plan-default:v1", strings.TrimSpace(g.PlanID), strings.ToUpper(strings.TrimSpace(g.Channel)),
		strings.ToUpper(strings.TrimSpace(g.Environment)), strings.ToUpper(strings.TrimSpace(g.Currency)),
	}, "|")
}

func (s *postgresStore) makeDefaultPricePlan(ctx context.Context, pricePlanID string, mutation pricePlanTransitionMutation, actorID, actorRole string) (pricePlanAdminView, bool, error) {
	if err := pricingAdminActor(actorID); err != nil {
		return pricePlanAdminView{}, false, err
	}
	if err := requirePricePlanWrite(mutation.Revision, mutation.ChangeReason); err != nil {
		return pricePlanAdminView{}, false, err
	}
	pricePlanID = strings.TrimSpace(pricePlanID)
	var initial pricePlanDefaultGroup
	if err := s.db.QueryRowContext(ctx, `
		select plan_id,plan_version_id,channel,environment,currency from xz_price_plans where id=$1
	`, pricePlanID).Scan(&initial.PlanID, &initial.VersionID, &initial.Channel, &initial.Environment, &initial.Currency); errors.Is(err, sql.ErrNoRows) {
		return pricePlanAdminView{}, false, newBusinessPlanAdminError(http.StatusNotFound, "PRICE_PLAN_NOT_FOUND", "price plan not found")
	} else if err != nil {
		return pricePlanAdminView{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return pricePlanAdminView{}, false, err
	}
	defer tx.Rollback()
	var advisoryResult string
	if err := tx.QueryRowContext(ctx, `select pg_advisory_xact_lock(hashtextextended($1,0))::text`, initial.lockKey()).Scan(&advisoryResult); err != nil {
		return pricePlanAdminView{}, false, err
	}
	if _, err := managedBusinessTypeForUpdate(ctx, tx, initial.PlanID); err != nil {
		return pricePlanAdminView{}, false, err
	}
	var planActive bool
	if err := tx.QueryRowContext(ctx, `select active from xz_plans where id=$1`, initial.PlanID).Scan(&planActive); err != nil {
		return pricePlanAdminView{}, false, err
	}
	version, err := loadBusinessPlanVersionForUpdate(ctx, tx, initial.VersionID)
	if err != nil {
		return pricePlanAdminView{}, false, err
	}
	var currentDefaultID string
	err = tx.QueryRowContext(ctx, `
		select id from xz_price_plans
		where plan_id=$1 and channel=$2 and environment=$3 and currency=$4 and is_default=true
	`, initial.PlanID, initial.Channel, initial.Environment, initial.Currency).Scan(&currentDefaultID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return pricePlanAdminView{}, false, err
	}
	if errors.Is(err, sql.ErrNoRows) {
		currentDefaultID = ""
	}
	lockedPrices := map[string]pricePlanAdminView{}
	for _, id := range sortedUniqueStrings([]string{pricePlanID, currentDefaultID}) {
		locked, lockErr := loadPricePlanAdminForUpdate(ctx, tx, id)
		if lockErr != nil {
			return pricePlanAdminView{}, false, lockErr
		}
		lockedPrices[id] = locked
	}
	target, ok := lockedPrices[pricePlanID]
	if !ok {
		return pricePlanAdminView{}, false, newBusinessPlanAdminError(http.StatusConflict, "PRICE_PLAN_CONFIGURATION_CHANGED", "target price plan could not be locked")
	}
	if target.PlanID != initial.PlanID || target.PlanVersionID != initial.VersionID || target.Channel != initial.Channel ||
		target.Environment != initial.Environment || target.Currency != initial.Currency {
		return pricePlanAdminView{}, false, newBusinessPlanAdminError(http.StatusConflict, "PRICE_PLAN_CONFIGURATION_CHANGED", "price plan default group changed while it was being locked")
	}
	if version.PlanID != target.PlanID {
		return pricePlanAdminView{}, false, newBusinessPlanAdminError(http.StatusConflict, "PRICE_PLAN_CONFIGURATION_CHANGED", "entitlement version ownership changed")
	}
	var bindingID, goodID string
	err = tx.QueryRowContext(ctx, `
		select id,wechat_good_id from xz_price_plan_payment_bindings
		where price_plan_id=$1 and enabled=true and status='ACTIVE'
		order by (channel=$2 and environment=$3) desc,id
		limit 1
	`, target.ID, target.Channel, target.Environment).Scan(&bindingID, &goodID)
	var binding *paymentBindingRow
	var good *wechatVirtualGoodAdminView
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return pricePlanAdminView{}, false, err
	}
	if err == nil {
		goodRow, goodErr := loadWechatVirtualGoodForUpdate(ctx, tx, goodID)
		if goodErr != nil {
			return pricePlanAdminView{}, false, goodErr
		}
		bindingRow, bindingErr := loadPaymentBindingForUpdate(ctx, tx, bindingID)
		if bindingErr != nil {
			return pricePlanAdminView{}, false, bindingErr
		}
		if bindingRow.PricePlanID != target.ID || bindingRow.WeChatGoodID != goodRow.ID {
			return pricePlanAdminView{}, false, newBusinessPlanAdminError(http.StatusConflict, "PRICE_PLAN_CONFIGURATION_CHANGED", "payment binding changed while it was being locked")
		}
		binding = &bindingRow
		good = &goodRow
	}
	now := time.Now().UTC()
	if target.storedKind == "TEST" {
		return pricePlanAdminView{}, false, newBusinessPlanAdminError(http.StatusUnprocessableEntity, "PRICE_PLAN_DEFAULT_TEST_FORBIDDEN", "TEST price plans cannot be default")
	}
	if target.Status != "ACTIVE" || !target.IsEnabled {
		return pricePlanAdminView{}, false, newBusinessPlanAdminError(http.StatusUnprocessableEntity, "PRICE_PLAN_DEFAULT_NOT_ACTIVE", "default price plan must be ACTIVE and enabled")
	}
	if !target.IsVisible {
		return pricePlanAdminView{}, false, newBusinessPlanAdminError(http.StatusUnprocessableEntity, "PRICE_PLAN_DEFAULT_HIDDEN", "hidden price plans cannot be default")
	}
	if target.AudienceType != "PUBLIC" {
		return pricePlanAdminView{}, false, newBusinessPlanAdminError(http.StatusUnprocessableEntity, "PRICE_PLAN_DEFAULT_AUDIENCE_INVALID", "default price plan must target PUBLIC audience")
	}
	validation, validationErr := evaluatePricePlanActivation(target, planActive, version, binding, good, now)
	if validationErr != nil {
		return pricePlanAdminView{}, false, validationErr
	}
	if target.IsDefault {
		return target, true, nil
	}
	if target.Revision != *mutation.Revision {
		return pricePlanAdminView{}, false, pricingRevisionConflict("price plan")
	}
	var oldDefault pricePlanAdminView
	if currentDefaultID != "" {
		oldDefault = lockedPrices[currentDefaultID]
		result, updateErr := tx.ExecContext(ctx, `
			update xz_price_plans set is_default=false,updated_by=$2,change_reason=$3
			where id=$1 and is_default=true
		`, oldDefault.ID, actorID, strings.TrimSpace(mutation.ChangeReason))
		if updateErr != nil {
			return pricePlanAdminView{}, false, mapPricePlanDatabaseError(updateErr)
		}
		if affected, _ := result.RowsAffected(); affected != 1 {
			return pricePlanAdminView{}, false, newBusinessPlanAdminError(http.StatusConflict, "PRICE_PLAN_DEFAULT_CONFLICT", "current default changed while it was being switched")
		}
	}
	result, err := tx.ExecContext(ctx, `
		update xz_price_plans set is_default=true,updated_by=$2,change_reason=$3
		where id=$1 and revision=$4
	`, target.ID, actorID, strings.TrimSpace(mutation.ChangeReason), *mutation.Revision)
	if isPostgresUniqueViolation(err) {
		return pricePlanAdminView{}, false, newBusinessPlanAdminError(http.StatusConflict, "PRICE_PLAN_DEFAULT_CONFLICT", "another default price plan already exists")
	}
	if err != nil {
		return pricePlanAdminView{}, false, mapPricePlanDatabaseError(err)
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return pricePlanAdminView{}, false, pricingRevisionConflict("price plan")
	}
	updated, err := scanPricePlanAdmin(tx.QueryRowContext(ctx, pricePlanAdminSelect+` where pp.id=$1`, target.ID))
	if err != nil {
		return pricePlanAdminView{}, false, err
	}
	var oldDefaultAfter any
	if oldDefault.ID != "" {
		refreshedOldDefault, refreshErr := scanPricePlanAdmin(tx.QueryRowContext(ctx, pricePlanAdminSelect+` where pp.id=$1`, oldDefault.ID))
		if refreshErr != nil {
			return pricePlanAdminView{}, false, refreshErr
		}
		oldDefaultAfter = refreshedOldDefault
	}
	metadata := map[string]any{
		"changeReason": mutation.ChangeReason, "planId": target.PlanID, "planVersionId": target.PlanVersionID,
		"pricePlanId": target.ID, "channel": target.Channel,
		"environment": target.Environment, "currency": target.Currency, "oldDefaultPricePlanId": oldDefault.ID,
		"newDefaultPricePlanId": target.ID, "oldPriceCents": oldDefault.SalePriceCents,
		"newPriceCents": target.SalePriceCents, "paymentBindingId": binding.ID, "wechatGoodId": good.ID,
		"wechatProductId": good.ProductID, "wechatOfferId": good.OfferID,
		"pricePlanPriceCents": target.SalePriceCents, "bindingPriceCents": binding.ProviderPriceSnapshotCents,
		"wechatGoodPriceCents": good.PlatformPriceCents, "validation": validation,
		"targetRevisionBefore": target.Revision, "targetRevisionAfter": updated.Revision,
		"oldDefaultRevisionBefore": oldDefault.Revision,
		"revisionBefore":           target.Revision, "revisionAfter": updated.Revision,
		"beforeSnapshot": map[string]any{"target": target, "oldDefault": func() any {
			if oldDefault.ID == "" {
				return nil
			}
			return oldDefault
		}()},
		"afterSnapshot": map[string]any{"target": updated, "oldDefault": oldDefaultAfter},
	}
	if err := insertRequiredPricePlanAudit(ctx, tx, actorID, actorRole, target.ID, metadata); err != nil {
		return pricePlanAdminView{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return pricePlanAdminView{}, false, err
	}
	return updated, false, nil
}

func insertRequiredPricePlanAudit(ctx context.Context, tx *sql.Tx, actorID, actorRole, resourceID string, metadata map[string]any) error {
	return insertPricingAuditLog(ctx, tx, pricingAuditMutationFromLegacy(
		actorID, actorRole, "price_plan.make_default", "price_plan", resourceID, http.MethodPost,
		"/api/v1/admin/price-plans/"+resourceID+"/make-default", http.StatusOK, metadata,
	))
}

func sortedUniqueStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	if len(result) == 2 && result[0] > result[1] {
		result[0], result[1] = result[1], result[0]
	}
	return result
}

func mapPricePlanDatabaseError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) && pgErr.SQLState() == "23514" {
		message := err.Error()
		switch {
		case strings.Contains(message, "PRICE_PLAN_CLONE_REQUIRED"):
			return newBusinessPlanAdminError(http.StatusConflict, "PRICE_PLAN_CLONE_REQUIRED", "economic fields require a cloned price plan")
		case strings.Contains(message, "PRICE_PLAN_ACTIVE_BINDING_REQUIRES_DISABLE"):
			return newBusinessPlanAdminError(http.StatusConflict, "PRICE_PLAN_ACTIVE_BINDING_REQUIRES_DISABLE", "disable the active payment binding first")
		}
	}
	return err
}
