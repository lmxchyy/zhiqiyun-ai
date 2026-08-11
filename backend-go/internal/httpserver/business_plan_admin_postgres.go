package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

const businessPlanVersionSelect = `
	select id,plan_id,version_no,business_type,rights_snapshot,
	       coalesce(member_level,''),coalesce(agent_level,''),token_amount,points_amount,duration_days,
	       commission_rule_version,commission_snapshot,status,revision,effective_at,expires_at,
	       coalesce(created_by,''),coalesce(updated_by,''),coalesce(activated_by,''),activated_at,
	       coalesce(retired_by,''),retired_at,change_reason,created_at,updated_at
	from xz_plan_versions
`

type businessPlanVersionScanner interface {
	Scan(...any) error
}

func scanBusinessPlanVersion(scanner businessPlanVersionScanner) (businessPlanVersionView, error) {
	var item businessPlanVersionView
	var rightsRaw, commissionRaw []byte
	var effectiveAt, expiresAt, activatedAt, retiredAt sql.NullTime
	err := scanner.Scan(
		&item.ID, &item.PlanID, &item.VersionNo, &item.BusinessType, &rightsRaw,
		&item.MemberLevel, &item.AgentLevel, &item.TokenAmount, &item.PointsAmount, &item.DurationDays,
		&item.CommissionRuleVersion, &commissionRaw, &item.Status, &item.Revision, &effectiveAt, &expiresAt,
		&item.CreatedBy, &item.UpdatedBy, &item.ActivatedBy, &activatedAt,
		&item.RetiredBy, &retiredAt, &item.ChangeReason, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return businessPlanVersionView{}, err
	}
	item.RightsSnapshot = map[string]any{}
	item.CommissionSnapshot = map[string]any{}
	if len(rightsRaw) > 0 {
		if err := json.Unmarshal(rightsRaw, &item.RightsSnapshot); err != nil {
			return businessPlanVersionView{}, err
		}
	}
	if len(commissionRaw) > 0 {
		if err := json.Unmarshal(commissionRaw, &item.CommissionSnapshot); err != nil {
			return businessPlanVersionView{}, err
		}
	}
	if effectiveAt.Valid {
		item.EffectiveAt = &effectiveAt.Time
	}
	if expiresAt.Valid {
		item.ExpiresAt = &expiresAt.Time
	}
	if activatedAt.Valid {
		item.ActivatedAt = &activatedAt.Time
	}
	if retiredAt.Valid {
		item.RetiredAt = &retiredAt.Time
	}
	return item, nil
}

func (s *postgresStore) listBusinessPlans(ctx context.Context) ([]businessPlanAdminView, error) {
	rows, err := s.db.QueryContext(ctx, `
		with managed as (
			select plan_id,min(business_type) business_type
			from xz_plan_versions
			where business_type in ('MEMBER','AGENT')
			group by plan_id
			having count(distinct business_type)=1
		)
		select p.id,coalesce(nullif(p.code,''),p.id),coalesce(p.name,''),managed.business_type,p.active,
		       coalesce(active_version.id,'')
		from xz_plans p
		join managed on managed.plan_id=p.id
		left join xz_plan_versions active_version on active_version.plan_id=p.id and active_version.status='ACTIVE'
		where (p.plan_type='MEMBER_PACKAGE' and managed.business_type='MEMBER')
		   or (p.plan_type='AGENT_JOIN_PACKAGE' and managed.business_type='AGENT')
		order by managed.business_type,p.id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []businessPlanAdminView{}
	for rows.Next() {
		var item businessPlanAdminView
		if err := rows.Scan(&item.ID, &item.Code, &item.Name, &item.BusinessType, &item.Active, &item.ActiveVersionID); err != nil {
			return nil, err
		}
		item.LegacyCode = isLegacyBusinessPlanCode(item.ID, item.Code)
		item.CodeReadOnly = true
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) businessPlan(ctx context.Context, planID string) (businessPlanAdminView, error) {
	var item businessPlanAdminView
	err := s.db.QueryRowContext(ctx, `
		with managed as (
			select plan_id,min(business_type) business_type
			from xz_plan_versions
			where business_type in ('MEMBER','AGENT')
			group by plan_id
			having count(distinct business_type)=1
		)
		select p.id,coalesce(nullif(p.code,''),p.id),coalesce(p.name,''),managed.business_type,p.active,
		       coalesce(active_version.id,'')
		from xz_plans p
		join managed on managed.plan_id=p.id
		left join xz_plan_versions active_version on active_version.plan_id=p.id and active_version.status='ACTIVE'
		where p.id=$1
		  and ((p.plan_type='MEMBER_PACKAGE' and managed.business_type='MEMBER')
		    or (p.plan_type='AGENT_JOIN_PACKAGE' and managed.business_type='AGENT'))
	`, strings.TrimSpace(planID)).Scan(&item.ID, &item.Code, &item.Name, &item.BusinessType, &item.Active, &item.ActiveVersionID)
	if errors.Is(err, sql.ErrNoRows) {
		return businessPlanAdminView{}, newBusinessPlanAdminError(http.StatusNotFound, "BUSINESS_PLAN_NOT_FOUND", "business plan not found")
	}
	if err != nil {
		return businessPlanAdminView{}, err
	}
	item.LegacyCode = isLegacyBusinessPlanCode(item.ID, item.Code)
	item.CodeReadOnly = true
	return item, nil
}

func (s *postgresStore) listBusinessPlanVersions(ctx context.Context, planID string) ([]businessPlanVersionView, error) {
	if _, err := s.businessPlan(ctx, planID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, businessPlanVersionSelect+` where plan_id=$1 order by version_no desc`, strings.TrimSpace(planID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []businessPlanVersionView{}
	for rows.Next() {
		item, err := scanBusinessPlanVersion(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func managedBusinessTypeForUpdate(ctx context.Context, tx *sql.Tx, planID string) (string, error) {
	var lockedID, planType string
	if err := tx.QueryRowContext(ctx, `select id,coalesce(plan_type,'') from xz_plans where id=$1 for update`, planID).Scan(&lockedID, &planType); errors.Is(err, sql.ErrNoRows) {
		return "", newBusinessPlanAdminError(http.StatusNotFound, "BUSINESS_PLAN_NOT_FOUND", "business plan not found")
	} else if err != nil {
		return "", err
	}
	rows, err := tx.QueryContext(ctx, `select distinct business_type from xz_plan_versions where plan_id=$1 and business_type in('MEMBER','AGENT')`, planID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var businessTypes []string
	for rows.Next() {
		var businessType string
		if err := rows.Scan(&businessType); err != nil {
			return "", err
		}
		businessTypes = append(businessTypes, businessType)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(businessTypes) != 1 {
		return "", newBusinessPlanAdminError(http.StatusNotFound, "BUSINESS_PLAN_NOT_FOUND", "business plan is not managed by V2 member/agent versions")
	}
	if (planType != "MEMBER_PACKAGE" || businessTypes[0] != "MEMBER") &&
		(planType != "AGENT_JOIN_PACKAGE" || businessTypes[0] != "AGENT") {
		return "", newBusinessPlanAdminError(http.StatusNotFound, "BUSINESS_PLAN_NOT_FOUND", "business plan type does not match its V2 entitlement versions")
	}
	return businessTypes[0], nil
}

func requireBusinessPlanActor(actorID string) error {
	if strings.TrimSpace(actorID) == "" {
		return newBusinessPlanAdminError(http.StatusForbidden, "FORBIDDEN", "operator identity is required")
	}
	return nil
}

func applyVersionMutation(item businessPlanVersionView, mutation businessPlanVersionMutation) businessPlanVersionView {
	if mutation.MemberLevel != nil {
		item.MemberLevel = strings.TrimSpace(*mutation.MemberLevel)
	}
	if mutation.AgentLevel != nil {
		item.AgentLevel = strings.TrimSpace(*mutation.AgentLevel)
	}
	if mutation.TokenAmount != nil {
		item.TokenAmount = *mutation.TokenAmount
	}
	if mutation.PointsAmount != nil {
		item.PointsAmount = *mutation.PointsAmount
	}
	if mutation.DurationDays != nil {
		item.DurationDays = *mutation.DurationDays
	}
	if mutation.RightsSnapshot != nil {
		item.RightsSnapshot = mutation.RightsSnapshot
	}
	if mutation.CommissionRuleVersion != nil {
		item.CommissionRuleVersion = strings.TrimSpace(*mutation.CommissionRuleVersion)
	}
	if mutation.CommissionSnapshot != nil {
		item.CommissionSnapshot = mutation.CommissionSnapshot
	}
	if mutation.EffectiveAt != nil {
		item.EffectiveAt = mutation.EffectiveAt
	}
	if mutation.ExpiresAt != nil {
		item.ExpiresAt = mutation.ExpiresAt
	}
	if item.BusinessType == "MEMBER" {
		item.AgentLevel = ""
	} else if item.BusinessType == "AGENT" {
		item.MemberLevel = ""
	}
	item.RightsSnapshot = canonicalVersionRights(item)
	return item
}

func marshalBusinessPlanJSON(value map[string]any) ([]byte, error) {
	if value == nil {
		value = map[string]any{}
	}
	return json.Marshal(value)
}

func (s *postgresStore) createBusinessPlanVersion(ctx context.Context, planID string, mutation businessPlanVersionMutation, actorID, actorRole string) (businessPlanVersionView, error) {
	if err := requireBusinessPlanActor(actorID); err != nil {
		return businessPlanVersionView{}, err
	}
	if err := validateVersionMutationReason(mutation.Reason); err != nil {
		return businessPlanVersionView{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return businessPlanVersionView{}, err
	}
	defer tx.Rollback()
	businessType, err := managedBusinessTypeForUpdate(ctx, tx, strings.TrimSpace(planID))
	if err != nil {
		return businessPlanVersionView{}, err
	}
	item := applyVersionMutation(businessPlanVersionView{
		ID:                 strings.Replace(newAuditID(), "audit_", "plan_version_", 1),
		PlanID:             strings.TrimSpace(planID),
		BusinessType:       businessType,
		RightsSnapshot:     map[string]any{},
		CommissionSnapshot: map[string]any{},
		Status:             "DRAFT",
		Revision:           1,
		CreatedBy:          actorID,
		UpdatedBy:          actorID,
		ChangeReason:       strings.TrimSpace(mutation.Reason),
	}, mutation)
	if err := validateVersionRights(item); err != nil {
		return businessPlanVersionView{}, err
	}
	if err := tx.QueryRowContext(ctx, `select coalesce(max(version_no),0)+1 from xz_plan_versions where plan_id=$1`, item.PlanID).Scan(&item.VersionNo); err != nil {
		return businessPlanVersionView{}, err
	}
	rightsRaw, err := marshalBusinessPlanJSON(item.RightsSnapshot)
	if err != nil {
		return businessPlanVersionView{}, err
	}
	commissionRaw, err := marshalBusinessPlanJSON(item.CommissionSnapshot)
	if err != nil {
		return businessPlanVersionView{}, err
	}
	_, err = tx.ExecContext(ctx, `
		insert into xz_plan_versions(
			id,plan_id,version_no,business_type,rights_snapshot,member_level,agent_level,
			token_amount,points_amount,duration_days,commission_rule_version,commission_snapshot,
			status,effective_at,expires_at,created_by,updated_by,change_reason
		) values($1,$2,$3,$4,$5::jsonb,nullif($6,''),nullif($7,''),$8,$9,$10,$11,$12::jsonb,
			'DRAFT',$13,$14,$15,$15,$16)
	`, item.ID, item.PlanID, item.VersionNo, item.BusinessType, rightsRaw, item.MemberLevel, item.AgentLevel,
		item.TokenAmount, item.PointsAmount, item.DurationDays, item.CommissionRuleVersion, commissionRaw,
		item.EffectiveAt, item.ExpiresAt, actorID, strings.TrimSpace(mutation.Reason))
	if err != nil {
		return businessPlanVersionView{}, err
	}
	created, err := scanBusinessPlanVersion(tx.QueryRowContext(ctx, businessPlanVersionSelect+` where id=$1`, item.ID))
	if err != nil {
		return businessPlanVersionView{}, err
	}
	if err := insertAuditLog(ctx, tx, actorID, actorRole, "business_plan.version.create", "plan_version", created.ID, "POST", "", http.StatusCreated, map[string]any{
		"planId": created.PlanID, "planVersionId": created.ID, "versionNo": created.VersionNo,
		"changeReason": mutation.Reason, "revisionBefore": int64(0), "revisionAfter": created.Revision,
		"afterSnapshot": created,
	}); err != nil {
		return businessPlanVersionView{}, err
	}
	if err := tx.Commit(); err != nil {
		return businessPlanVersionView{}, err
	}
	return created, nil
}

func loadBusinessPlanVersionForUpdate(ctx context.Context, tx *sql.Tx, versionID string) (businessPlanVersionView, error) {
	item, err := scanBusinessPlanVersion(tx.QueryRowContext(ctx, businessPlanVersionSelect+` where id=$1 for update`, strings.TrimSpace(versionID)))
	if errors.Is(err, sql.ErrNoRows) {
		return businessPlanVersionView{}, newBusinessPlanAdminError(http.StatusNotFound, "PLAN_VERSION_NOT_FOUND", "plan version not found")
	}
	return item, err
}

func checkVersionRevision(item businessPlanVersionView, revision int64) error {
	if revision <= 0 || item.Revision != revision {
		return newBusinessPlanAdminError(http.StatusConflict, "REVISION_CONFLICT", "plan version revision conflict")
	}
	return nil
}

func (s *postgresStore) updateBusinessPlanVersion(ctx context.Context, versionID string, mutation businessPlanVersionMutation, actorID, actorRole string) (businessPlanVersionView, error) {
	if err := requireBusinessPlanActor(actorID); err != nil {
		return businessPlanVersionView{}, err
	}
	if err := validateVersionMutationReason(mutation.Reason); err != nil {
		return businessPlanVersionView{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return businessPlanVersionView{}, err
	}
	defer tx.Rollback()
	var planID string
	if err := tx.QueryRowContext(ctx, `select plan_id from xz_plan_versions where id=$1`, strings.TrimSpace(versionID)).Scan(&planID); errors.Is(err, sql.ErrNoRows) {
		return businessPlanVersionView{}, newBusinessPlanAdminError(http.StatusNotFound, "PLAN_VERSION_NOT_FOUND", "plan version not found")
	} else if err != nil {
		return businessPlanVersionView{}, err
	}
	if _, err := managedBusinessTypeForUpdate(ctx, tx, planID); err != nil {
		return businessPlanVersionView{}, err
	}
	current, err := loadBusinessPlanVersionForUpdate(ctx, tx, versionID)
	if err != nil {
		return businessPlanVersionView{}, err
	}
	if err := checkVersionRevision(current, mutation.Revision); err != nil {
		return businessPlanVersionView{}, err
	}
	if current.Status != "DRAFT" {
		return businessPlanVersionView{}, newBusinessPlanAdminError(http.StatusConflict, "PLAN_VERSION_NOT_DRAFT", "only DRAFT plan versions can be edited")
	}
	updated := applyVersionMutation(current, mutation)
	if err := validateVersionRights(updated); err != nil {
		return businessPlanVersionView{}, err
	}
	rightsRaw, err := marshalBusinessPlanJSON(updated.RightsSnapshot)
	if err != nil {
		return businessPlanVersionView{}, err
	}
	commissionRaw, err := marshalBusinessPlanJSON(updated.CommissionSnapshot)
	if err != nil {
		return businessPlanVersionView{}, err
	}
	result, err := tx.ExecContext(ctx, `
		update xz_plan_versions set
			rights_snapshot=$2::jsonb,member_level=nullif($3,''),agent_level=nullif($4,''),
			token_amount=$5,points_amount=$6,duration_days=$7,
			commission_rule_version=$8,commission_snapshot=$9::jsonb,
			effective_at=$10,expires_at=$11,updated_by=$12,change_reason=$13
		where id=$1 and status='DRAFT' and revision=$14
	`, current.ID, rightsRaw, updated.MemberLevel, updated.AgentLevel, updated.TokenAmount, updated.PointsAmount,
		updated.DurationDays, updated.CommissionRuleVersion, commissionRaw, updated.EffectiveAt, updated.ExpiresAt,
		actorID, strings.TrimSpace(mutation.Reason), mutation.Revision)
	if err != nil {
		return businessPlanVersionView{}, err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return businessPlanVersionView{}, newBusinessPlanAdminError(http.StatusConflict, "REVISION_CONFLICT", "plan version revision conflict")
	}
	updated, err = scanBusinessPlanVersion(tx.QueryRowContext(ctx, businessPlanVersionSelect+` where id=$1`, current.ID))
	if err != nil {
		return businessPlanVersionView{}, err
	}
	if err := insertAuditLog(ctx, tx, actorID, actorRole, "business_plan.version.update", "plan_version", updated.ID, "PATCH", "", http.StatusOK, map[string]any{
		"planId": updated.PlanID, "planVersionId": updated.ID, "changeReason": mutation.Reason,
		"revisionBefore": current.Revision, "revisionAfter": updated.Revision,
		"beforeSnapshot": current, "afterSnapshot": updated,
	}); err != nil {
		return businessPlanVersionView{}, err
	}
	if err := tx.Commit(); err != nil {
		return businessPlanVersionView{}, err
	}
	return updated, nil
}

func (s *postgresStore) transitionBusinessPlanVersion(ctx context.Context, versionID, targetStatus string, transition businessPlanVersionTransition, actorID, actorRole string) (businessPlanVersionView, error) {
	if err := requireBusinessPlanActor(actorID); err != nil {
		return businessPlanVersionView{}, err
	}
	if err := validateVersionMutationReason(transition.Reason); err != nil {
		return businessPlanVersionView{}, err
	}
	targetStatus = strings.ToUpper(strings.TrimSpace(targetStatus))
	if targetStatus != "ACTIVE" && targetStatus != "RETIRED" {
		return businessPlanVersionView{}, newBusinessPlanAdminError(http.StatusBadRequest, "INVALID_PLAN_VERSION_TRANSITION", "invalid plan version transition")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return businessPlanVersionView{}, err
	}
	defer tx.Rollback()
	var planID string
	if err := tx.QueryRowContext(ctx, `select plan_id from xz_plan_versions where id=$1`, strings.TrimSpace(versionID)).Scan(&planID); errors.Is(err, sql.ErrNoRows) {
		return businessPlanVersionView{}, newBusinessPlanAdminError(http.StatusNotFound, "PLAN_VERSION_NOT_FOUND", "plan version not found")
	} else if err != nil {
		return businessPlanVersionView{}, err
	}
	if _, err := managedBusinessTypeForUpdate(ctx, tx, planID); err != nil {
		return businessPlanVersionView{}, err
	}
	current, err := loadBusinessPlanVersionForUpdate(ctx, tx, versionID)
	if err != nil {
		return businessPlanVersionView{}, err
	}
	if err := checkVersionRevision(current, transition.Revision); err != nil {
		return businessPlanVersionView{}, err
	}
	now := time.Now().UTC()
	if targetStatus == "ACTIVE" {
		if current.Status != "DRAFT" {
			return businessPlanVersionView{}, newBusinessPlanAdminError(http.StatusConflict, "PLAN_VERSION_NOT_DRAFT", "only DRAFT plan versions can be activated")
		}
		var oldActiveID string
		err := tx.QueryRowContext(ctx, `select id from xz_plan_versions where plan_id=$1 and status='ACTIVE' and id<>$2 for update`, current.PlanID, current.ID).Scan(&oldActiveID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return businessPlanVersionView{}, err
		}
		if oldActiveID != "" {
			oldActiveBefore, err := loadBusinessPlanVersionForUpdate(ctx, tx, oldActiveID)
			if err != nil {
				return businessPlanVersionView{}, err
			}
			if _, err := tx.ExecContext(ctx, `
				update xz_plan_versions set status='RETIRED',retired_by=$2,retired_at=$3,updated_by=$2,change_reason=$4
				where id=$1 and status='ACTIVE'
			`, oldActiveID, actorID, now, "automatically retired: "+strings.TrimSpace(transition.Reason)); err != nil {
				return businessPlanVersionView{}, err
			}
			oldActiveAfter, err := scanBusinessPlanVersion(tx.QueryRowContext(ctx, businessPlanVersionSelect+` where id=$1`, oldActiveID))
			if err != nil {
				return businessPlanVersionView{}, err
			}
			if err := insertAuditLog(ctx, tx, actorID, actorRole, "business_plan.version.auto_retire", "plan_version", oldActiveID, "POST", "", http.StatusOK, map[string]any{
				"planId": oldActiveAfter.PlanID, "planVersionId": oldActiveID, "replacementVersionId": current.ID,
				"changeReason": transition.Reason, "revisionBefore": oldActiveBefore.Revision, "revisionAfter": oldActiveAfter.Revision,
				"beforeSnapshot": oldActiveBefore, "afterSnapshot": oldActiveAfter,
			}); err != nil {
				return businessPlanVersionView{}, err
			}
		}
		_, err = tx.ExecContext(ctx, `
			update xz_plan_versions set status='ACTIVE',activated_by=$2,activated_at=$3,updated_by=$2,change_reason=$4
			where id=$1 and status='DRAFT' and revision=$5
		`, current.ID, actorID, now, strings.TrimSpace(transition.Reason), transition.Revision)
	} else {
		if current.Status != "ACTIVE" {
			return businessPlanVersionView{}, newBusinessPlanAdminError(http.StatusConflict, "PLAN_VERSION_NOT_ACTIVE", "only ACTIVE plan versions can be retired")
		}
		_, err = tx.ExecContext(ctx, `
			update xz_plan_versions set status='RETIRED',retired_by=$2,retired_at=$3,updated_by=$2,change_reason=$4
			where id=$1 and status='ACTIVE' and revision=$5
		`, current.ID, actorID, now, strings.TrimSpace(transition.Reason), transition.Revision)
	}
	if err != nil {
		return businessPlanVersionView{}, err
	}
	beforeTransition := current
	current, err = scanBusinessPlanVersion(tx.QueryRowContext(ctx, businessPlanVersionSelect+` where id=$1`, current.ID))
	if err != nil {
		return businessPlanVersionView{}, err
	}
	action := "business_plan.version.activate"
	if targetStatus == "RETIRED" {
		action = "business_plan.version.retire"
	}
	if err := insertAuditLog(ctx, tx, actorID, actorRole, action, "plan_version", current.ID, "POST", "", http.StatusOK, map[string]any{
		"planId": current.PlanID, "planVersionId": current.ID, "changeReason": transition.Reason,
		"revisionBefore": beforeTransition.Revision, "revisionAfter": current.Revision,
		"beforeSnapshot": beforeTransition, "afterSnapshot": current,
	}); err != nil {
		return businessPlanVersionView{}, err
	}
	if err := tx.Commit(); err != nil {
		return businessPlanVersionView{}, err
	}
	return current, nil
}
