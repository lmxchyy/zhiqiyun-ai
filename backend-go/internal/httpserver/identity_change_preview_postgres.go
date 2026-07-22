package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	commissionapp "xianzhi-ai/backend-go/internal/app/commission"
)

type identityChangeComputed struct {
	request  identityChangePreviewRequest
	result   identityChangePreviewResult
	plan     adminPlan
	relation adminUserRelationship
	rules    []commissionapp.CommissionRule
}

func (s *postgresStore) PreviewAdminIdentityChange(actorID, actorRole, userID string, request identityChangePreviewRequest) (identityChangePreviewResult, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return identityChangePreviewResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: false})
	if err != nil {
		return identityChangePreviewResult{}, err
	}
	defer tx.Rollback()
	computed, err := s.computeIdentityChangePreviewTx(ctx, tx, actorID, actorRole, userID, request)
	if err != nil {
		return identityChangePreviewResult{}, err
	}
	token, tokenHash, err := newIdentityPreviewToken()
	if err != nil {
		return identityChangePreviewResult{}, err
	}
	computed.result.PreviewToken = token
	requestPayload := jsonProjection(computed.request)
	storedResult := computed.result
	storedResult.PreviewToken = ""
	resultPayload := jsonProjection(storedResult)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO xz_identity_change_previews(
		  id,token_hash,user_id,actor_id,actor_role,change_action,change_method,target_identity,
		  request_snapshot,result_snapshot,status,high_risk,source_membership_order_id,expires_at
		) VALUES($1,$2,$3,$4,$5,$6,$7,nullif($8,''),$9::jsonb,$10::jsonb,$11,$12,nullif($13,''),$14)
	`, computed.result.PreviewID, tokenHash, userID, actorID, actorRole, computed.request.Action,
		computed.request.Method, computed.request.TargetIdentity, requestPayload, resultPayload,
		computed.result.Status, computed.result.HighRisk, computed.result.SourceMembershipOrderID,
		computed.result.ExpiresAt)
	if err != nil {
		return identityChangePreviewResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return identityChangePreviewResult{}, err
	}
	return computed.result, nil
}

func (s *postgresStore) computeIdentityChangePreviewTx(ctx context.Context, tx *sql.Tx, actorID, actorRole, userID string, request identityChangePreviewRequest) (identityChangeComputed, error) {
	request.Action = strings.ToUpper(strings.TrimSpace(request.Action))
	request.Method = strings.ToUpper(strings.TrimSpace(request.Method))
	request.TargetIdentity = strings.ToUpper(strings.TrimSpace(request.TargetIdentity))
	request.PlanID = strings.TrimSpace(request.PlanID)
	request.ParentAgentID = strings.TrimSpace(request.ParentAgentID)
	request.OperationCenterID = strings.TrimSpace(request.OperationCenterID)
	request.ConversionTokenPolicy = strings.ToUpper(strings.TrimSpace(request.ConversionTokenPolicy))
	request.Reason = strings.TrimSpace(request.Reason)
	request.Remark = strings.TrimSpace(request.Remark)
	request.PaymentProof.Reference = strings.TrimSpace(request.PaymentProof.Reference)
	request.PaymentProof.StorageFileID = strings.TrimSpace(request.PaymentProof.StorageFileID)
	request.PaymentProof.PayerName = strings.TrimSpace(request.PaymentProof.PayerName)
	request.PaymentProof.PaidAt = strings.TrimSpace(request.PaymentProof.PaidAt)
	request.PaymentProof.PaymentChannel = strings.ToUpper(strings.TrimSpace(request.PaymentProof.PaymentChannel))
	request.PaymentProof.Remark = strings.TrimSpace(request.PaymentProof.Remark)
	request.PaymentProof.URL = strings.TrimSpace(request.PaymentProof.URL)
	request.DiscountReason = strings.TrimSpace(request.DiscountReason)
	if actorID == "" || userID == "" || request.Reason == "" {
		return identityChangeComputed{}, fmt.Errorf("%w: actor, user and reason are required", errIdentityChangeInvalid)
	}
	if !identityChangeAdminRoleAllowed(actorRole) {
		return identityChangeComputed{}, errIdentityPermission
	}
	if request.Method != identityMethodOnlyIdentity && request.Method != identityMethodOfflineOrder && request.Method != identityMethodSpecialGrant && request.Method != identityMethodPackageConversion {
		return identityChangeComputed{}, fmt.Errorf("%w: unsupported change method", errIdentityChangeInvalid)
	}
	var userRole string
	if err := tx.QueryRowContext(ctx, `SELECT coalesce(role,'') FROM xz_users WHERE id=$1 FOR SHARE`, userID).Scan(&userRole); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return identityChangeComputed{}, errIdentityUserNotFound
		}
		return identityChangeComputed{}, err
	}
	oldIdentity, oldStatus, err := currentChannelIdentityTx(ctx, tx, userID)
	if err != nil {
		return identityChangeComputed{}, err
	}
	relation, err := currentIdentityRelationshipTx(ctx, tx, userID)
	if err != nil {
		return identityChangeComputed{}, err
	}
	previewID := "identity_preview_" + shortID(userID+actorID+time.Now().UTC().Format(time.RFC3339Nano))
	expiresAt := time.Now().UTC().Add(15 * time.Minute)
	result := identityChangePreviewResult{
		PreviewID: previewID, UserID: userID, OldIdentity: oldIdentity, TargetIdentity: request.TargetIdentity,
		Method: request.Method, Action: request.Action, EstimatedCommissions: []identityCommissionPreview{},
		RiskWarnings: []string{}, Blockers: []string{}, ExpiresAt: expiresAt.Format(time.RFC3339Nano),
		EffectiveAt:        "ON_CONFIRMATION",
		RelationshipBefore: identityRelationshipSnapshot(relation),
	}
	afterRelation := relation
	if request.ParentAgentID != "" || request.Action == "ADJUST_PARENT_AGENT" {
		afterRelation.ParentAgentID = request.ParentAgentID
	}
	if request.OperationCenterID != "" || request.Action == "ADJUST_OPERATION_CENTER" {
		afterRelation.OperationCenterID = request.OperationCenterID
	}
	result.RelationshipAfter = identityRelationshipSnapshot(afterRelation)

	upgrade := request.Action == "UPGRADE"
	switch request.Action {
	case "UPGRADE":
		if request.TargetIdentity != "AGENT" && request.TargetIdentity != "OPERATION_CENTER" {
			result.Blockers = append(result.Blockers, "升级目标必须是 AGENT 或 OPERATION_CENTER")
		}
		if request.TargetIdentity == oldIdentity && oldStatus != "TERMINATED" {
			result.Blockers = append(result.Blockers, "user already has the target business identity")
		}
		if oldIdentity == "OPERATION_CENTER" && request.TargetIdentity == "AGENT" {
			result.Blockers = append(result.Blockers, "operation center cannot be downgraded to agent")
		}
	case "FREEZE":
		request.TargetIdentity = oldIdentity
		result.TargetIdentity = oldIdentity
		if oldIdentity == "USER" || oldStatus != "ACTIVE" {
			result.Blockers = append(result.Blockers, "only an active agent or operation center can be frozen")
		}
	case "RESTORE":
		request.TargetIdentity = oldIdentity
		result.TargetIdentity = oldIdentity
		if oldIdentity == "USER" || oldStatus != "FROZEN" {
			result.Blockers = append(result.Blockers, "only a frozen business identity can be restored")
		}
	case "TERMINATE":
		request.TargetIdentity = oldIdentity
		result.TargetIdentity = oldIdentity
		result.Blockers = append(result.Blockers, "代理商或运营中心必须通过受控降级流程终止，不能直接改回普通用户")
	case "ADJUST_PARENT_AGENT", "ADJUST_OPERATION_CENTER":
		request.TargetIdentity = oldIdentity
		result.TargetIdentity = oldIdentity
	default:
		return identityChangeComputed{}, fmt.Errorf("%w: unsupported action", errIdentityChangeInvalid)
	}
	if !upgrade && request.Method != identityMethodOnlyIdentity {
		result.Blockers = append(result.Blockers, "lifecycle and relationship changes require ONLY_IDENTITY")
	}
	if request.TargetIdentity == "OPERATION_CENTER" && upgrade && !strings.EqualFold(actorRole, "SUPER_ADMIN") {
		return identityChangeComputed{}, errIdentityPermission
	}
	if request.Action == "ADJUST_PARENT_AGENT" && request.ParentAgentID == "" {
		result.RiskWarnings = append(result.RiskWarnings, "confirmation will remove the current parent agent")
	}
	resolvedCenterID, relationshipErr := resolveIdentityRelationshipTargetsTx(ctx, tx, userID, request.ParentAgentID, request.OperationCenterID)
	if relationshipErr != nil {
		result.Blockers = append(result.Blockers, relationshipErr.Error())
	} else {
		request.OperationCenterID = resolvedCenterID
		afterRelation.OperationCenterID = resolvedCenterID
		result.RelationshipAfter = identityRelationshipSnapshot(afterRelation)
	}

	var plan adminPlan
	var rules []commissionapp.CommissionRule
	if upgrade && request.Method != identityMethodOnlyIdentity && request.Method != identityMethodSpecialGrant || request.PlanID != "" {
		if request.PlanID == "" {
			if request.TargetIdentity == "OPERATION_CENTER" {
				request.PlanID = "plan_operation_center_5000"
			} else {
				request.PlanID = "plan_agent_join_996"
			}
		}
		plan, err = loadIdentityChangePlanTx(ctx, tx, request.PlanID)
		if err != nil {
			result.Blockers = append(result.Blockers, "upgrade plan does not exist or is inactive")
		} else if (request.TargetIdentity == "AGENT" && planBusinessType(plan) != planTypeAgentJoinPackage) || (request.TargetIdentity == "OPERATION_CENTER" && planBusinessType(plan) != planTypeOperationCenterPackage) {
			result.Blockers = append(result.Blockers, "plan does not match target identity")
		}
	}

	switch request.Method {
	case identityMethodOnlyIdentity:
		if request.PaidAmountCents != 0 || request.GiftTokenAmount != 0 || request.GrantPackageToken {
			result.Blockers = append(result.Blockers, "ONLY_IDENTITY does not allow money or token changes")
		}
	case identityMethodOfflineOrder:
		result.PaymentRequired = true
		result.PaidAmountCents = int64(request.PaidAmountCents)
		result.OriginalAmountCents = int64(plan.PriceCents)
		result.PayableAmountCents = int64(request.PaidAmountCents)
		if request.PaidAmountCents <= 0 || plan.PriceCents <= 0 || request.PaidAmountCents > plan.PriceCents {
			result.Blockers = append(result.Blockers, "offline paid amount must be positive and cannot exceed the package price")
		}
		if plan.PriceCents > 0 && request.PaidAmountCents != plan.PriceCents {
			result.SpecialPrice = true
			result.DiscountAmountCents = int64(plan.PriceCents - request.PaidAmountCents)
			if request.DiscountReason == "" {
				result.Blockers = append(result.Blockers, "special price requires discountReason")
			}
			allowed, permissionErr := identityActorHasPermissionTx(ctx, tx, actorID, actorRole, "identity:change:special-price")
			if permissionErr != nil {
				return identityChangeComputed{}, permissionErr
			}
			if !allowed {
				result.Blockers = append(result.Blockers, "special price permission is required")
			}
			result.ReviewRequired = true
			result.RiskWarnings = append(result.RiskWarnings, "special price requires approval by another administrator")
		}
		if proofErr := validateIdentityPaymentProofTx(ctx, tx, request.PaymentProof); proofErr != nil {
			result.Blockers = append(result.Blockers, proofErr.Error())
		}
		if request.GrantPackageToken {
			result.TokenDelta = int64(planTokenGrantAmount(plan))
			result.TokenChangeType = "OFFLINE_IDENTITY_PACKAGE_GRANT"
		}
	case identityMethodSpecialGrant:
		if request.PaidAmountCents != 0 || request.GrantPackageToken {
			result.Blockers = append(result.Blockers, "SPECIAL_GRANT does not allow revenue or package token grant")
		}
		if request.GiftTokenAmount < 0 {
			result.Blockers = append(result.Blockers, "gift token amount cannot be negative")
		}
		result.TokenDelta = int64(request.GiftTokenAmount)
		if request.GiftTokenAmount > 0 {
			result.TokenChangeType = "SPECIAL_IDENTITY_GIFT"
			result.RiskWarnings = append(result.RiskWarnings, "an independent administrator token gift ledger will be created")
		}
	case identityMethodPackageConversion:
		if request.TargetIdentity != "AGENT" {
			result.Blockers = append(result.Blockers, "package conversion only supports membership to agent plan")
		}
		sourceOrderID, sourcePaid, sourceToken, sourceErr := membershipConversionSourceTx(ctx, tx, userID, request.PlanID)
		if sourceErr != nil {
			result.Blockers = append(result.Blockers, sourceErr.Error())
		} else {
			result.SourceMembershipOrderID = sourceOrderID
			difference := int64(plan.PriceCents) - sourcePaid
			if difference < 0 {
				difference = 0
			}
			result.PaidAmountCents = difference
			result.OriginalAmountCents = int64(plan.PriceCents)
			result.PayableAmountCents = difference
			result.DiscountAmountCents = sourcePaid
			result.PaymentRequired = difference > 0
			if difference > 0 && request.PaidAmountCents != int(difference) {
				result.Blockers = append(result.Blockers, "conversion paid amount must equal the server-calculated difference")
			}
			if difference == 0 && request.PaidAmountCents != 0 {
				result.Blockers = append(result.Blockers, "conversion has no price difference; duplicate payment is forbidden")
			}
			if difference > 0 {
				if proofErr := validateIdentityPaymentProofTx(ctx, tx, request.PaymentProof); proofErr != nil {
					result.Blockers = append(result.Blockers, proofErr.Error())
				}
			}
			switch request.ConversionTokenPolicy {
			case "KEEP_EXISTING":
				result.TokenDelta = 0
				result.TokenChangeType = "PACKAGE_CONVERSION_KEEP_EXISTING"
			case "ADJUST_DIFFERENCE":
				result.TokenDelta = int64(planTokenGrantAmount(plan)) - sourceToken
				result.TokenChangeType = "PACKAGE_CONVERSION_DIFFERENCE"
			default:
				result.Blockers = append(result.Blockers, "conversionTokenPolicy must be KEEP_EXISTING or ADJUST_DIFFERENCE")
			}
			var exists bool
			if queryErr := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM xz_identity_change_executions WHERE source_membership_order_id=$1 AND change_method='PACKAGE_CONVERSION' AND status IN ('PROCESSING','SUCCEEDED'))`, sourceOrderID).Scan(&exists); queryErr != nil {
				return identityChangeComputed{}, queryErr
			} else if exists {
				result.Blockers = append(result.Blockers, "membership order is already converted or being converted")
			}
		}
		result.ReviewRequired = true
		result.RiskWarnings = append(result.RiskWarnings, "package conversion requires review by another administrator")
	}

	if result.TokenDelta < 0 {
		var available int64
		if err := tx.QueryRowContext(ctx, `SELECT coalesce(available,0) FROM xz_point_accounts WHERE user_id=$1`, userID).Scan(&available); errors.Is(err, sql.ErrNoRows) {
			available = 0
		} else if err != nil {
			return identityChangeComputed{}, err
		}
		if available+result.TokenDelta < 0 {
			result.Blockers = append(result.Blockers, "token balance is insufficient for the conversion deduction")
		}
		result.RiskWarnings = append(result.RiskWarnings, "confirmation will deduct token and create an immutable ledger record")
	}
	if result.PaidAmountCents > 0 && (request.Method == identityMethodOfflineOrder || request.Method == identityMethodPackageConversion) && len(result.Blockers) == 0 {
		rules, result.EstimatedCommissions, err = previewIdentityCommissionsTx(ctx, tx, previewID, userID, plan, result.PaidAmountCents, afterRelation)
		if err != nil {
			result.Blockers = append(result.Blockers, "commission preview failed: "+err.Error())
		} else {
			result.CommissionGenerated = len(result.EstimatedCommissions) > 0
			result.CommissionRuleSnapshot = snapshotCommissionRules(rules)
		}
	}
	result.HighRisk = request.TargetIdentity == "OPERATION_CENTER" || request.Method == identityMethodPackageConversion || request.Action == "FREEZE" || request.Action == "TERMINATE" || strings.HasPrefix(request.Action, "ADJUST_") || result.TokenDelta != 0 || result.PaidAmountCents > 0
	if result.HighRisk {
		result.RiskWarnings = append(result.RiskWarnings, "high risk operation requires explicit second confirmation")
	}
	if len(result.Blockers) > 0 {
		result.Status = "BLOCKED"
	} else if result.ReviewRequired {
		result.Status = "REVIEW_REQUIRED"
	} else {
		result.Status = "READY"
	}
	return identityChangeComputed{request: request, result: result, plan: plan, relation: afterRelation, rules: rules}, nil
}

func currentChannelIdentityTx(ctx context.Context, tx *sql.Tx, userID string) (string, string, error) {
	var identityType, status string
	err := tx.QueryRowContext(ctx, `
		SELECT identity_type,identity_status FROM xz_user_business_identities
		WHERE user_id=$1 AND identity_type IN ('AGENT','OPERATION_CENTER')
		  AND identity_status IN ('ACTIVE','FROZEN') AND ended_at IS NULL
		ORDER BY CASE identity_type WHEN 'OPERATION_CENTER' THEN 0 ELSE 1 END LIMIT 1
	`, userID).Scan(&identityType, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return "USER", "ACTIVE", nil
	}
	return identityType, status, err
}

func currentIdentityRelationshipTx(ctx context.Context, tx *sql.Tx, userID string) (adminUserRelationship, error) {
	var item adminUserRelationship
	var effectiveAt, createdAt, updatedAt time.Time
	err := tx.QueryRowContext(ctx, `
		SELECT id,tenant_id,user_id,coalesce(parent_agent_id,''),coalesce(operation_center_id,''),effective_at,status,source_type,created_by,created_at,updated_at
		FROM xz_user_relationships WHERE user_id=$1 AND status='ACTIVE' AND ended_at IS NULL
	`, userID).Scan(&item.ID, &item.TenantID, &item.UserID, &item.ParentAgentID, &item.OperationCenterID, &effectiveAt, &item.Status, &item.SourceType, &item.CreatedBy, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return adminUserRelationship{UserID: userID, TenantID: "tenant_default", Status: "ACTIVE", SourceType: "ADMIN_IDENTITY_CHANGE"}, nil
	}
	if err != nil {
		return adminUserRelationship{}, err
	}
	item.EffectiveAt = effectiveAt.UTC().Format(time.RFC3339Nano)
	item.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
	item.UpdatedAt = updatedAt.UTC().Format(time.RFC3339Nano)
	return item, nil
}

func identityRelationshipSnapshot(item adminUserRelationship) map[string]any {
	return map[string]any{"parentAgentId": item.ParentAgentID, "operationCenterId": item.OperationCenterID}
}

func validateIdentityRelationshipTargetsTx(ctx context.Context, tx *sql.Tx, userID, parentAgentID, operationCenterID string) error {
	_, err := resolveIdentityRelationshipTargetsTx(ctx, tx, userID, parentAgentID, operationCenterID)
	return err
}

func loadIdentityChangePlanTx(ctx context.Context, tx *sql.Tx, planID string) (adminPlan, error) {
	var item adminPlan
	var entitlements, raw []byte
	err := tx.QueryRowContext(ctx, `SELECT id,coalesce(code,''),coalesce(name,''),coalesce(plan_type,''),price_cents,grant_points,coalesce(token_amount,0),coalesce(member_level,''),coalesce(agent_level,''),duration_days,concurrency,active,entitlements,raw FROM xz_plans WHERE id=$1 AND active=true`, planID).Scan(
		&item.ID, &item.Code, &item.Name, &item.PlanType, &item.PriceCents, &item.GrantPoints, &item.TokenAmount, &item.MemberLevel, &item.AgentLevel, &item.DurationDays, &item.Concurrency, &item.Active, &entitlements, &raw)
	if err != nil {
		return adminPlan{}, err
	}
	_ = json.Unmarshal(entitlements, &item.Entitlements)
	var rawPlan adminPlan
	if json.Unmarshal(raw, &rawPlan) == nil {
		item.CommissionTemplateCode = firstNonEmptyString(rawPlan.CommissionTemplateCode, stringValue(item.Entitlements["commissionTemplateCode"]))
	}
	item.Points, item.Price = item.GrantPoints, item.PriceCents
	return item, nil
}

func membershipConversionSourceTx(ctx context.Context, tx *sql.Tx, userID, targetPlanID string) (string, int64, int64, error) {
	var orderID string
	var paid, tokens int64
	err := tx.QueryRowContext(ctx, `
		SELECT o.id,o.amount_cents,coalesce(o.token_grant_amount,o.token_amount,0)
		FROM xz_orders o JOIN xz_plans p ON p.id=o.plan_id
		WHERE o.user_id=$1 AND upper(coalesce(o.status,'')) IN ('PAID','SUCCEEDED')
		  AND upper(coalesce(p.plan_type,''))='MEMBER_PACKAGE'
		  AND coalesce((p.entitlements->>'convertibleToAgent')::boolean,false)=true
		  AND coalesce(p.entitlements->'conversionTargetPlanIds','[]'::jsonb) ? $2
		  AND upper(coalesce(o.fulfillment_status,o.price_snapshot->>'fulfillmentStatus',''))='FULFILLED'
		  AND upper(coalesce(o.order_status,'')) NOT IN ('CANCELLED','REVOKED','REFUNDED')
		  AND coalesce(nullif(o.paid_at,'')::timestamptz,nullif(o.created_at,'')::timestamptz,now())
		      + make_interval(days=>coalesce(nullif(p.entitlements->>'conversionValidityDays','')::int,0)) >= clock_timestamp()
		  AND NOT EXISTS (
		    SELECT 1 FROM xz_refund_records refund
		    WHERE refund.order_id=o.id AND upper(coalesce(refund.status,'')) NOT IN ('FAILED','REJECTED','CANCELLED')
		  )
		  AND NOT EXISTS (
		    SELECT 1 FROM xz_identity_change_executions execution
		    WHERE execution.source_membership_order_id=o.id
		      AND execution.change_method='PACKAGE_CONVERSION'
		      AND execution.status IN ('PROCESSING','SUCCEEDED')
		  )
		ORDER BY coalesce(nullif(o.paid_at,''),o.created_at) DESC,o.id DESC LIMIT 1
	`, userID, targetPlanID).Scan(&orderID, &paid, &tokens)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, 0, errors.New("user has no paid membership order eligible for conversion")
	}
	return orderID, paid, tokens, err
}

func previewIdentityCommissionsTx(ctx context.Context, tx *sql.Tx, previewID, userID string, plan adminPlan, paid int64, relation adminUserRelationship) ([]commissionapp.CommissionRule, []identityCommissionPreview, error) {
	now := time.Now().UTC()
	rules, err := loadEffectiveCommissionRulesTx(ctx, tx, "tenant_default", planBusinessType(plan), plan.ID, commissionTemplateCode(plan), now)
	if err != nil {
		return nil, nil, err
	}
	agents := map[int]string{}
	if relation.ParentAgentID != "" {
		eligible, eligibilityErr := commissionBeneficiaryEligibleTx(ctx, tx, "AGENT", relation.ParentAgentID)
		if eligibilityErr != nil {
			return nil, nil, eligibilityErr
		}
		if eligible {
			agents[1] = relation.ParentAgentID
		}
	}
	operationCenterID := relation.OperationCenterID
	if operationCenterID != "" {
		eligible, eligibilityErr := commissionBeneficiaryEligibleTx(ctx, tx, "OPERATION_CENTER", operationCenterID)
		if eligibilityErr != nil {
			return nil, nil, eligibilityErr
		}
		if !eligible {
			operationCenterID = ""
		}
	}
	result, err := commissionapp.NewEngine().Calculate(commissionapp.CalculationInput{
		TenantID: "tenant_default", OrderID: previewID, OrderNo: previewID, ProductType: planBusinessType(plan), ProductID: plan.ID,
		SourceUserID: userID, OrderAmountCents: commissionapp.AmountCents(paid), PaidAmountCents: commissionapp.AmountCents(paid), Quantity: 1, PaidAt: now,
		Rules: rules, Relationships: commissionapp.RelationshipSnapshot{AgentIDsByLevel: agents, OperationCenterID: operationCenterID, PlatformID: "platform:tenant_default"},
	})
	if err != nil {
		return nil, nil, err
	}
	rulesByID := map[string]commissionapp.CommissionRule{}
	for _, rule := range rules {
		rulesByID[rule.ID] = rule
	}
	items := make([]identityCommissionPreview, 0, len(result.Records))
	for _, record := range result.Records {
		items = append(items, identityCommissionPreview{BeneficiaryType: string(record.BeneficiaryType), BeneficiaryID: record.BeneficiaryID, RuleID: record.RuleID, RuleCode: rulesByID[record.RuleID].Code, AmountCents: int64(record.AmountCents)})
	}
	return rules, items, nil
}
