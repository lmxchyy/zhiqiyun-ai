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

	"github.com/jackc/pgx/v5/pgconn"
)

const wechatVirtualGoodAdminSelect = `
	select id,channel,environment,offer_id,product_id,goods_name,platform_price_cents,mode,
	       published,enabled,status,verification_status,coalesce(verified_by,''),verified_at,
	       verification_reason,verification_evidence,verification_snapshot,verification_expires_at,
	       revision,coalesce(created_by,''),coalesce(updated_by,''),created_at,updated_at
	from xz_wechat_virtual_goods
`

type wechatVirtualGoodScanner interface {
	Scan(...any) error
}

func scanWechatVirtualGood(scanner wechatVirtualGoodScanner) (wechatVirtualGoodAdminView, error) {
	var item wechatVirtualGoodAdminView
	var verifiedAt, expiresAt sql.NullTime
	var snapshotRaw []byte
	err := scanner.Scan(
		&item.ID, &item.Channel, &item.Environment, &item.OfferID, &item.ProductID, &item.GoodsName,
		&item.PlatformPriceCents, &item.Mode, &item.Published, &item.Enabled, &item.Status,
		&item.recordedVerificationStatus, &item.VerifiedBy, &verifiedAt, &item.VerificationReason,
		&item.VerificationEvidence, &snapshotRaw, &expiresAt, &item.Revision, &item.CreatedBy,
		&item.UpdatedBy, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return wechatVirtualGoodAdminView{}, err
	}
	if verifiedAt.Valid {
		item.VerifiedAt = &verifiedAt.Time
	}
	if expiresAt.Valid {
		item.VerificationExpiresAt = &expiresAt.Time
	}
	item.VerificationSnapshot = map[string]any{}
	if len(snapshotRaw) > 0 {
		if err := json.Unmarshal(snapshotRaw, &item.VerificationSnapshot); err != nil {
			return wechatVirtualGoodAdminView{}, err
		}
	}
	item.deriveVerification(time.Now().UTC())
	return item, nil
}

func pricingAdminReason(reason string) error {
	return validateVersionMutationReason(reason)
}

func pricingAdminActor(actorID string) error {
	return requireBusinessPlanActor(actorID)
}

func pricingRevisionConflict(resource string) error {
	return newBusinessPlanAdminError(http.StatusConflict, "REVISION_CONFLICT", resource+" revision conflict")
}

func defaultPricePlanDependencyDisableError() error {
	return newBusinessPlanAdminError(
		http.StatusConflict,
		"PRICE_PLAN_DEFAULT_DEPENDENCY_DISABLE_FORBIDDEN",
		"switch the default price plan before disabling or rebinding its payment dependency",
	)
}

func isPostgresUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func validateWechatGoodFields(item wechatVirtualGoodAdminView) (wechatVirtualGoodAdminView, error) {
	var err error
	item.Channel, err = normalizeWechatGoodChannel(item.Channel)
	if err != nil {
		return item, err
	}
	item.Environment, err = normalizePaymentEnvironment(item.Environment)
	if err != nil {
		return item, err
	}
	item.Mode, err = normalizeWechatGoodMode(item.Mode)
	if err != nil {
		return item, err
	}
	item.OfferID = strings.TrimSpace(item.OfferID)
	item.ProductID = strings.TrimSpace(item.ProductID)
	item.GoodsName = strings.TrimSpace(item.GoodsName)
	if item.OfferID == "" || item.ProductID == "" || item.GoodsName == "" || item.PlatformPriceCents <= 0 {
		return item, newBusinessPlanAdminError(http.StatusBadRequest, "WECHAT_GOOD_INVALID", "offerId, productId, goodsName and a positive platformPriceCents are required")
	}
	return item, nil
}

func (s *postgresStore) listWechatVirtualGoods(ctx context.Context) ([]wechatVirtualGoodAdminView, error) {
	rows, err := s.db.QueryContext(ctx, wechatVirtualGoodAdminSelect+` order by environment,updated_at desc,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []wechatVirtualGoodAdminView{}
	for rows.Next() {
		item, err := scanWechatVirtualGood(rows)
		if err != nil {
			return nil, err
		}
		if err := deriveWechatGoodPaymentStatus(ctx, s.db, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) wechatVirtualGood(ctx context.Context, goodID string) (wechatVirtualGoodAdminView, error) {
	item, err := scanWechatVirtualGood(s.db.QueryRowContext(ctx, wechatVirtualGoodAdminSelect+` where id=$1`, strings.TrimSpace(goodID)))
	if errors.Is(err, sql.ErrNoRows) {
		return wechatVirtualGoodAdminView{}, newBusinessPlanAdminError(http.StatusNotFound, "WECHAT_GOOD_NOT_FOUND", "WeChat virtual good not found")
	}
	if err != nil {
		return item, err
	}
	if err := deriveWechatGoodPaymentStatus(ctx, s.db, &item); err != nil {
		return item, err
	}
	return item, nil
}

func (s *postgresStore) listWechatVirtualGoodReferences(ctx context.Context, goodID string) ([]wechatVirtualGoodReferenceAdminView, error) {
	goodID = strings.TrimSpace(goodID)
	var exists bool
	if err := s.db.QueryRowContext(ctx, `select exists(select 1 from xz_wechat_virtual_goods where id=$1)`, goodID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, newBusinessPlanAdminError(http.StatusNotFound, "WECHAT_GOOD_NOT_FOUND", "WeChat virtual good not found")
	}
	rows, err := s.db.QueryContext(ctx, `
		select bindings.id,prices.id,prices.code,prices.name,plans.id,plans.name,prices.is_default,
		       bindings.status,bindings.enabled,prices.sale_price_cents,bindings.provider_price_snapshot_cents,
		       bindings.channel,bindings.environment,bindings.wechat_good_id,
		       (select count(*) from xz_order_price_quotes quotes where quotes.price_plan_id=prices.id),
		       (select count(*) from xz_orders orders where orders.price_plan_id=prices.id)
		from xz_price_plan_payment_bindings bindings
		join xz_price_plans prices on prices.id=bindings.price_plan_id
		join xz_plans plans on plans.id=prices.plan_id
		join xz_plan_versions versions on versions.id=prices.plan_version_id and versions.plan_id=prices.plan_id
		where bindings.wechat_good_id=$1
		  and ((plans.plan_type='MEMBER_PACKAGE' and versions.business_type='MEMBER')
		    or (plans.plan_type='AGENT_JOIN_PACKAGE' and versions.business_type='AGENT'))
		order by plans.id,prices.id,bindings.created_at,bindings.id
	`, goodID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []wechatVirtualGoodReferenceAdminView{}
	for rows.Next() {
		var item wechatVirtualGoodReferenceAdminView
		if err := rows.Scan(
			&item.BindingID, &item.PricePlanID, &item.PricePlanCode, &item.PricePlanName, &item.PlanID, &item.PlanName,
			&item.IsDefault, &item.BindingStatus, &item.BindingEnabled, &item.SalePriceCents,
			&item.ProviderPriceSnapshotCents, &item.Channel, &item.Environment, &item.WeChatGoodID,
			&item.QuoteCount, &item.OrderCount,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func deriveWechatGoodPaymentStatus(ctx context.Context, queryer pricingQueryRower, item *wechatVirtualGoodAdminView) error {
	var mismatch bool
	err := queryer.QueryRowContext(ctx, `
		select exists(
			select 1
			from xz_price_plan_payment_bindings b
			join xz_price_plans pp on pp.id=b.price_plan_id
			where b.wechat_good_id=$1 and b.enabled=true and b.status='ACTIVE'
			  and (
				pp.sale_price_cents<>b.provider_price_snapshot_cents
				or pp.sale_price_cents<>$2
				or pp.channel<>b.channel or pp.channel<>$3
				or pp.environment<>b.environment or pp.environment<>$4
			  )
		)
	`, item.ID, item.PlatformPriceCents, item.Channel, item.Environment).Scan(&mismatch)
	if err != nil {
		return err
	}
	if mismatch && item.recordedVerificationStatus != wechatGoodVerificationDisabled {
		item.VerificationStatus = wechatGoodVerificationMismatch
	}
	return nil
}

func loadWechatVirtualGoodForUpdate(ctx context.Context, tx *sql.Tx, goodID string) (wechatVirtualGoodAdminView, error) {
	item, err := scanWechatVirtualGood(tx.QueryRowContext(ctx, wechatVirtualGoodAdminSelect+` where id=$1 for update`, strings.TrimSpace(goodID)))
	if errors.Is(err, sql.ErrNoRows) {
		return wechatVirtualGoodAdminView{}, newBusinessPlanAdminError(http.StatusNotFound, "WECHAT_GOOD_NOT_FOUND", "WeChat virtual good not found")
	}
	return item, err
}

func (s *postgresStore) createWechatVirtualGood(ctx context.Context, mutation wechatVirtualGoodCreateMutation, actorID, actorRole string) (wechatVirtualGoodAdminView, error) {
	if err := pricingAdminActor(actorID); err != nil {
		return wechatVirtualGoodAdminView{}, err
	}
	if err := pricingAdminReason(mutation.Reason); err != nil {
		return wechatVirtualGoodAdminView{}, err
	}
	item, err := validateWechatGoodFields(wechatVirtualGoodAdminView{
		ID: strings.Replace(newAuditID(), "audit_", "wechat_good_", 1), Channel: mutation.Channel,
		Environment: mutation.Environment, OfferID: mutation.OfferID, ProductID: mutation.ProductID,
		GoodsName: mutation.GoodsName, PlatformPriceCents: mutation.PlatformPriceCents, Mode: mutation.Mode,
		Status: "DRAFT", VerificationStatus: wechatGoodVerificationUnconfirmed, Revision: 1,
	})
	if err != nil {
		return wechatVirtualGoodAdminView{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return wechatVirtualGoodAdminView{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `
		insert into xz_wechat_virtual_goods(
			id,channel,environment,offer_id,product_id,goods_name,platform_price_cents,mode,
			published,enabled,status,verification_status,created_by,updated_by
		) values($1,$2,$3,$4,$5,$6,$7,$8,false,false,'DRAFT','UNCONFIRMED',$9,$9)
	`, item.ID, item.Channel, item.Environment, item.OfferID, item.ProductID, item.GoodsName,
		item.PlatformPriceCents, item.Mode, actorID)
	if isPostgresUniqueViolation(err) {
		return wechatVirtualGoodAdminView{}, newBusinessPlanAdminError(http.StatusConflict, "WECHAT_GOOD_ALREADY_EXISTS", "the WeChat virtual good already exists in this channel and environment")
	}
	if err != nil {
		return wechatVirtualGoodAdminView{}, err
	}
	created, err := scanWechatVirtualGood(tx.QueryRowContext(ctx, wechatVirtualGoodAdminSelect+` where id=$1`, item.ID))
	if err != nil {
		return wechatVirtualGoodAdminView{}, err
	}
	if err := insertAuditLog(ctx, tx, actorID, actorRole, "wechat_good.create", "wechat_virtual_good", created.ID, "POST", "", http.StatusCreated, map[string]any{
		"changeReason": mutation.Reason, "wechatGoodId": created.ID, "channel": created.Channel, "environment": created.Environment,
		"productId": created.ProductID, "offerId": created.OfferID, "platformPriceCents": created.PlatformPriceCents,
		"verificationSource": wechatGoodVerificationSource, "revisionBefore": int64(0), "revisionAfter": created.Revision,
		"afterSnapshot": created,
	}); err != nil {
		return wechatVirtualGoodAdminView{}, err
	}
	if err := tx.Commit(); err != nil {
		return wechatVirtualGoodAdminView{}, err
	}
	return created, nil
}

func applyWechatVirtualGoodUpdate(current wechatVirtualGoodAdminView, mutation wechatVirtualGoodUpdateMutation) (wechatVirtualGoodAdminView, bool, error) {
	updated := current
	if mutation.Channel != nil {
		updated.Channel = *mutation.Channel
	}
	if mutation.Environment != nil {
		updated.Environment = *mutation.Environment
	}
	if mutation.OfferID != nil {
		updated.OfferID = *mutation.OfferID
	}
	if mutation.ProductID != nil {
		updated.ProductID = *mutation.ProductID
	}
	if mutation.GoodsName != nil {
		updated.GoodsName = *mutation.GoodsName
	}
	if mutation.PlatformPriceCents != nil {
		updated.PlatformPriceCents = *mutation.PlatformPriceCents
	}
	if mutation.Mode != nil {
		updated.Mode = *mutation.Mode
	}
	var err error
	updated, err = validateWechatGoodFields(updated)
	if err != nil {
		return current, false, err
	}
	criticalChanged := current.Channel != updated.Channel || current.Environment != updated.Environment ||
		current.OfferID != updated.OfferID || current.ProductID != updated.ProductID ||
		current.PlatformPriceCents != updated.PlatformPriceCents || current.Mode != updated.Mode
	return updated, criticalChanged, nil
}

func (s *postgresStore) updateWechatVirtualGood(ctx context.Context, goodID string, mutation wechatVirtualGoodUpdateMutation, actorID, actorRole string) (wechatVirtualGoodAdminView, error) {
	if err := pricingAdminActor(actorID); err != nil {
		return wechatVirtualGoodAdminView{}, err
	}
	if err := pricingAdminReason(mutation.Reason); err != nil {
		return wechatVirtualGoodAdminView{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return wechatVirtualGoodAdminView{}, err
	}
	defer tx.Rollback()
	current, err := loadWechatVirtualGoodForUpdate(ctx, tx, goodID)
	if err != nil {
		return wechatVirtualGoodAdminView{}, err
	}
	if mutation.Revision <= 0 || current.Revision != mutation.Revision {
		return wechatVirtualGoodAdminView{}, pricingRevisionConflict("WeChat virtual good")
	}
	updated, criticalChanged, err := applyWechatVirtualGoodUpdate(current, mutation)
	if err != nil {
		return wechatVirtualGoodAdminView{}, err
	}
	if criticalChanged {
		var paymentBinding, liveQuote bool
		if err := tx.QueryRowContext(ctx, `select exists(select 1 from xz_price_plan_payment_bindings where wechat_good_id=$1)`, current.ID).Scan(&paymentBinding); err != nil {
			return wechatVirtualGoodAdminView{}, err
		}
		if paymentBinding {
			return wechatVirtualGoodAdminView{}, newBusinessPlanAdminError(http.StatusConflict, "WECHAT_GOOD_HAS_PAYMENT_BINDING", "rebind every payment binding before changing payment identity or price")
		}
		if err := tx.QueryRowContext(ctx, `select exists(select 1 from xz_order_price_quotes where wechat_good_id=$1 and status='AVAILABLE' and expires_at>now())`, current.ID).Scan(&liveQuote); err != nil {
			return wechatVirtualGoodAdminView{}, err
		}
		if liveQuote {
			return wechatVirtualGoodAdminView{}, newBusinessPlanAdminError(http.StatusConflict, "WECHAT_GOOD_HAS_LIVE_QUOTE", "wait for outstanding quotes to expire before changing payment identity or price")
		}
		updated.Published = false
		updated.Enabled = false
		updated.Status = "DRAFT"
		updated.recordedVerificationStatus = wechatGoodVerificationUnconfirmed
		updated.VerificationStatus = wechatGoodVerificationUnconfirmed
		updated.VerifiedBy = ""
		updated.VerifiedAt = nil
		updated.VerificationReason = ""
		updated.VerificationEvidence = ""
		updated.VerificationSnapshot = map[string]any{}
		updated.VerificationExpiresAt = nil
	}
	snapshotRaw, _ := json.Marshal(updated.VerificationSnapshot)
	result, err := tx.ExecContext(ctx, `
		update xz_wechat_virtual_goods set
			channel=$2,environment=$3,offer_id=$4,product_id=$5,goods_name=$6,
			platform_price_cents=$7,mode=$8,published=$9,enabled=$10,status=$11,
			verification_status=$12,verified_by=nullif($13,''),verified_at=$14,
			verification_reason=$15,verification_evidence=$16,verification_snapshot=$17::jsonb,
			verification_expires_at=$18,updated_by=$19
		where id=$1 and revision=$20
	`, current.ID, updated.Channel, updated.Environment, updated.OfferID, updated.ProductID, updated.GoodsName,
		updated.PlatformPriceCents, updated.Mode, updated.Published, updated.Enabled, updated.Status,
		updated.recordedVerificationStatus, updated.VerifiedBy, updated.VerifiedAt, updated.VerificationReason,
		updated.VerificationEvidence, snapshotRaw, updated.VerificationExpiresAt, actorID, mutation.Revision)
	if err != nil {
		return wechatVirtualGoodAdminView{}, err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return wechatVirtualGoodAdminView{}, pricingRevisionConflict("WeChat virtual good")
	}
	updated, err = scanWechatVirtualGood(tx.QueryRowContext(ctx, wechatVirtualGoodAdminSelect+` where id=$1`, current.ID))
	if err != nil {
		return wechatVirtualGoodAdminView{}, err
	}
	if err := insertAuditLog(ctx, tx, actorID, actorRole, "wechat_good.update", "wechat_virtual_good", current.ID, "PATCH", "", http.StatusOK, map[string]any{
		"changeReason": mutation.Reason, "wechatGoodId": current.ID, "environment": updated.Environment,
		"revisionBefore": current.Revision, "revisionAfter": updated.Revision,
		"criticalFieldsChanged": criticalChanged, "beforeSnapshot": current, "afterSnapshot": updated,
	}); err != nil {
		return wechatVirtualGoodAdminView{}, err
	}
	if err := tx.Commit(); err != nil {
		return wechatVirtualGoodAdminView{}, err
	}
	return updated, nil
}

func validateExistingActiveBindingsForGood(ctx context.Context, tx *sql.Tx, good wechatVirtualGoodAdminView) error {
	var priceMismatch, environmentMismatch bool
	err := tx.QueryRowContext(ctx, `
		select
			coalesce(bool_or(pp.sale_price_cents<>b.provider_price_snapshot_cents or pp.sale_price_cents<>$2),false),
			coalesce(bool_or(pp.channel<>b.channel or pp.channel<>$3 or pp.environment<>b.environment or pp.environment<>$4),false)
		from xz_price_plan_payment_bindings b
		join xz_price_plans pp on pp.id=b.price_plan_id
		where b.wechat_good_id=$1 and b.enabled=true and b.status='ACTIVE'
	`, good.ID, good.PlatformPriceCents, good.Channel, good.Environment).Scan(&priceMismatch, &environmentMismatch)
	if err != nil {
		return err
	}
	if priceMismatch {
		return newBusinessPlanAdminError(http.StatusConflict, "PRICE_PLAN_WECHAT_PRICE_MISMATCH", "active binding price differs from the WeChat good")
	}
	if environmentMismatch {
		return newBusinessPlanAdminError(http.StatusConflict, "PRICE_PLAN_PAYMENT_ENV_MISMATCH", "active binding channel or environment differs from the WeChat good")
	}
	return nil
}

func (s *postgresStore) confirmWechatVirtualGood(ctx context.Context, goodID string, confirmation wechatVirtualGoodConfirmation, actorID, actorRole string) (wechatVirtualGoodAdminView, error) {
	if err := pricingAdminActor(actorID); err != nil {
		return wechatVirtualGoodAdminView{}, err
	}
	if err := pricingAdminReason(confirmation.Reason); err != nil {
		return wechatVirtualGoodAdminView{}, err
	}
	confirmation.VerificationReason = strings.TrimSpace(confirmation.VerificationReason)
	if confirmation.VerificationReason == "" {
		return wechatVirtualGoodAdminView{}, newBusinessPlanAdminError(
			http.StatusBadRequest,
			"WECHAT_GOOD_VERIFICATION_REASON_REQUIRED",
			"verificationReason is required for manual WeChat publication confirmation",
		)
	}
	now := time.Now().UTC()
	if confirmation.VerificationExpiresAt != nil && !confirmation.VerificationExpiresAt.After(now) {
		return wechatVirtualGoodAdminView{}, newBusinessPlanAdminError(http.StatusBadRequest, "WECHAT_GOOD_VERIFICATION_EXPIRY_INVALID", "verificationExpiresAt must be in the future")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return wechatVirtualGoodAdminView{}, err
	}
	defer tx.Rollback()
	current, err := loadWechatVirtualGoodForUpdate(ctx, tx, goodID)
	if err != nil {
		return wechatVirtualGoodAdminView{}, err
	}
	if confirmation.Revision <= 0 || current.Revision != confirmation.Revision {
		return wechatVirtualGoodAdminView{}, pricingRevisionConflict("WeChat virtual good")
	}
	if err := validateExistingActiveBindingsForGood(ctx, tx, current); err != nil {
		return wechatVirtualGoodAdminView{}, err
	}
	snapshot := map[string]any{
		"productId": current.ProductID, "offerId": current.OfferID, "environment": current.Environment,
		"platformPriceCents": current.PlatformPriceCents, "channel": current.Channel, "mode": current.Mode,
		"confirmationType": "MANUAL_OPERATOR",
	}
	snapshotRaw, _ := json.Marshal(snapshot)
	result, err := tx.ExecContext(ctx, `
		update xz_wechat_virtual_goods set
			published=true,enabled=true,status='PUBLISHED',verification_status='MANUALLY_CONFIRMED_PUBLISHED',
			verified_by=$2,verified_at=$3,verification_reason=$4,verification_evidence=$5,
			verification_snapshot=$6::jsonb,verification_expires_at=$7,updated_by=$2
		where id=$1 and revision=$8
	`, current.ID, actorID, now, confirmation.VerificationReason, strings.TrimSpace(confirmation.Evidence),
		snapshotRaw, confirmation.VerificationExpiresAt, confirmation.Revision)
	if err != nil {
		return wechatVirtualGoodAdminView{}, err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return wechatVirtualGoodAdminView{}, pricingRevisionConflict("WeChat virtual good")
	}
	confirmed, err := scanWechatVirtualGood(tx.QueryRowContext(ctx, wechatVirtualGoodAdminSelect+` where id=$1`, current.ID))
	if err != nil {
		return wechatVirtualGoodAdminView{}, err
	}
	if err := insertAuditLog(ctx, tx, actorID, actorRole, "wechat_good.confirm_published", "wechat_virtual_good", current.ID, "POST", "", http.StatusOK, map[string]any{
		"changeReason": strings.TrimSpace(confirmation.Reason), "verificationReason": confirmation.VerificationReason,
		"evidence": strings.TrimSpace(confirmation.Evidence), "verificationSource": wechatGoodVerificationSource,
		"confirmationSnapshot": snapshot, "wechatRealtimeVerified": false, "wechatGoodId": current.ID,
		"environment": confirmed.Environment, "revisionBefore": current.Revision, "revisionAfter": confirmed.Revision,
		"beforeSnapshot": current, "afterSnapshot": confirmed,
	}); err != nil {
		return wechatVirtualGoodAdminView{}, err
	}
	if err := tx.Commit(); err != nil {
		return wechatVirtualGoodAdminView{}, err
	}
	return confirmed, nil
}

func (s *postgresStore) disableWechatVirtualGood(ctx context.Context, goodID string, transition wechatVirtualGoodTransition, actorID, actorRole string) (wechatVirtualGoodAdminView, error) {
	if err := pricingAdminActor(actorID); err != nil {
		return wechatVirtualGoodAdminView{}, err
	}
	if err := pricingAdminReason(transition.Reason); err != nil {
		return wechatVirtualGoodAdminView{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return wechatVirtualGoodAdminView{}, err
	}
	defer tx.Rollback()
	goodID = strings.TrimSpace(goodID)
	rows, err := tx.QueryContext(ctx, `
		select distinct price_plan_id
		from xz_price_plan_payment_bindings
		where wechat_good_id=$1 and enabled=true and status='ACTIVE'
		order by price_plan_id
	`, goodID)
	if err != nil {
		return wechatVirtualGoodAdminView{}, err
	}
	var pricePlanIDs []string
	for rows.Next() {
		var pricePlanID string
		if err := rows.Scan(&pricePlanID); err != nil {
			rows.Close()
			return wechatVirtualGoodAdminView{}, err
		}
		pricePlanIDs = append(pricePlanIDs, pricePlanID)
	}
	if err := rows.Close(); err != nil {
		return wechatVirtualGoodAdminView{}, err
	}
	priceContextByPricePlan := make(map[string]managedPricePlanBindingContext, len(pricePlanIDs))
	for _, pricePlanID := range pricePlanIDs {
		var priceContext managedPricePlanBindingContext
		priceContext.ID = pricePlanID
		if err := tx.QueryRowContext(ctx, `
			select plan_id,plan_version_id,is_default,channel,environment
			from xz_price_plans where id=$1 for update
		`, pricePlanID).Scan(&priceContext.PlanID, &priceContext.PlanVersionID, &priceContext.IsDefault, &priceContext.Channel, &priceContext.Environment); err != nil {
			return wechatVirtualGoodAdminView{}, err
		}
		priceContextByPricePlan[pricePlanID] = priceContext
	}
	current, err := loadWechatVirtualGoodForUpdate(ctx, tx, goodID)
	if err != nil {
		return wechatVirtualGoodAdminView{}, err
	}
	if transition.Revision <= 0 || current.Revision != transition.Revision {
		return wechatVirtualGoodAdminView{}, pricingRevisionConflict("WeChat virtual good")
	}
	rows, err = tx.QueryContext(ctx, `
		select id,price_plan_id
		from xz_price_plan_payment_bindings
		where wechat_good_id=$1 and enabled=true and status='ACTIVE'
		order by price_plan_id,id
		for update
	`, current.ID)
	if err != nil {
		return wechatVirtualGoodAdminView{}, err
	}
	var disabledBindings []string
	for rows.Next() {
		var bindingID, pricePlanID string
		if err := rows.Scan(&bindingID, &pricePlanID); err != nil {
			rows.Close()
			return wechatVirtualGoodAdminView{}, err
		}
		priceContext, locked := priceContextByPricePlan[pricePlanID]
		if !locked {
			rows.Close()
			return wechatVirtualGoodAdminView{}, newBusinessPlanAdminError(http.StatusConflict, "PRICE_PLAN_CONFIGURATION_CHANGED", "payment binding changed while the WeChat good was being locked")
		}
		if priceContext.IsDefault {
			rows.Close()
			return wechatVirtualGoodAdminView{}, defaultPricePlanDependencyDisableError()
		}
		disabledBindings = append(disabledBindings, bindingID)
	}
	if err := rows.Close(); err != nil {
		return wechatVirtualGoodAdminView{}, err
	}
	bindingSnapshotsBefore := make(map[string]paymentBindingRow, len(disabledBindings))
	for _, bindingID := range disabledBindings {
		binding, loadErr := loadPaymentBindingForUpdate(ctx, tx, bindingID)
		if loadErr != nil {
			return wechatVirtualGoodAdminView{}, loadErr
		}
		bindingSnapshotsBefore[bindingID] = binding
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `
		update xz_price_plan_payment_bindings set enabled=false,status='DISABLED',disabled_by=$2,
			disabled_at=$3,updated_by=$2,
			enabled_by=coalesce(enabled_by,$2),enabled_at=coalesce(enabled_at,created_at,$3)
		where wechat_good_id=$1 and enabled=true and status='ACTIVE'
	`, current.ID, actorID, now); err != nil {
		return wechatVirtualGoodAdminView{}, err
	}
	result, err := tx.ExecContext(ctx, `
		update xz_wechat_virtual_goods set published=false,enabled=false,status='DISABLED',
			verification_status='DISABLED',updated_by=$2 where id=$1 and revision=$3
	`, current.ID, actorID, transition.Revision)
	if err != nil {
		return wechatVirtualGoodAdminView{}, err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return wechatVirtualGoodAdminView{}, pricingRevisionConflict("WeChat virtual good")
	}
	disabled, err := scanWechatVirtualGood(tx.QueryRowContext(ctx, wechatVirtualGoodAdminSelect+` where id=$1`, current.ID))
	if err != nil {
		return wechatVirtualGoodAdminView{}, err
	}
	for _, bindingID := range disabledBindings {
		beforeBinding := bindingSnapshotsBefore[bindingID]
		priceContext := priceContextByPricePlan[beforeBinding.PricePlanID]
		afterBinding, loadErr := loadPaymentBindingForUpdate(ctx, tx, bindingID)
		if loadErr != nil {
			return wechatVirtualGoodAdminView{}, loadErr
		}
		if err := insertAuditLog(ctx, tx, actorID, actorRole, "price_plan.payment_binding.disable", "price_plan_payment_binding", bindingID, "POST", "", http.StatusOK, map[string]any{
			"changeReason": "WeChat good disabled: " + transition.Reason, "planId": priceContext.PlanID,
			"planVersionId": priceContext.PlanVersionID, "pricePlanId": beforeBinding.PricePlanID,
			"paymentBindingId": bindingID, "wechatGoodId": current.ID, "environment": beforeBinding.Environment,
			"revisionBefore": beforeBinding.Revision, "revisionAfter": afterBinding.Revision,
			"beforeSnapshot": beforeBinding, "afterSnapshot": afterBinding,
		}); err != nil {
			return wechatVirtualGoodAdminView{}, err
		}
	}
	if err := insertAuditLog(ctx, tx, actorID, actorRole, "wechat_good.disable", "wechat_virtual_good", current.ID, "POST", "", http.StatusOK, map[string]any{
		"changeReason": transition.Reason, "wechatGoodId": current.ID, "environment": current.Environment,
		"disabledBindingIds": disabledBindings, "revisionBefore": current.Revision, "revisionAfter": disabled.Revision,
		"beforeSnapshot": current, "afterSnapshot": disabled,
	}); err != nil {
		return wechatVirtualGoodAdminView{}, err
	}
	if err := tx.Commit(); err != nil {
		return wechatVirtualGoodAdminView{}, err
	}
	return disabled, nil
}

type managedPricePlanBindingContext struct {
	ID             string
	PlanID         string
	PlanVersionID  string
	SalePriceCents int64
	Channel        string
	Environment    string
	Enabled        bool
	Status         string
	IsDefault      bool
	VersionStatus  string
	BusinessType   string
	PlanType       string
	PlanActive     bool
}

type pricingQueryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func validateManagedPricePlanBindingContext(item managedPricePlanBindingContext) (managedPricePlanBindingContext, error) {
	managed := (item.PlanType == planTypeMemberPackage && item.BusinessType == "MEMBER") ||
		(item.PlanType == planTypeAgentJoinPackage && item.BusinessType == "AGENT")
	if !managed {
		return item, newBusinessPlanAdminError(http.StatusNotFound, "PRICE_PLAN_NOT_MANAGED", "price plan is not a V2 member or agent plan")
	}
	return item, nil
}

func loadManagedPricePlanBindingContext(ctx context.Context, queryer pricingQueryRower, pricePlanID string) (managedPricePlanBindingContext, error) {
	query := `
		select pp.id,pp.plan_id,pp.plan_version_id,pp.sale_price_cents,pp.channel,pp.environment,
		       pp.enabled,pp.status,pp.is_default,pv.status,pv.business_type,coalesce(p.plan_type,''),p.active
		from xz_price_plans pp
		join xz_plan_versions pv on pv.id=pp.plan_version_id and pv.plan_id=pp.plan_id
		join xz_plans p on p.id=pp.plan_id
		where pp.id=$1
	`
	var item managedPricePlanBindingContext
	err := queryer.QueryRowContext(ctx, query, strings.TrimSpace(pricePlanID)).Scan(
		&item.ID, &item.PlanID, &item.PlanVersionID, &item.SalePriceCents, &item.Channel, &item.Environment,
		&item.Enabled, &item.Status, &item.IsDefault, &item.VersionStatus, &item.BusinessType, &item.PlanType, &item.PlanActive,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return item, newBusinessPlanAdminError(http.StatusNotFound, "PRICE_PLAN_NOT_FOUND", "price plan not found")
	}
	if err != nil {
		return item, err
	}
	return validateManagedPricePlanBindingContext(item)
}

func loadManagedPricePlanBindingContextForUpdate(ctx context.Context, tx *sql.Tx, pricePlanID string) (managedPricePlanBindingContext, error) {
	pricePlanID = strings.TrimSpace(pricePlanID)
	var initialPlanID, initialVersionID string
	if err := tx.QueryRowContext(ctx, `select plan_id,plan_version_id from xz_price_plans where id=$1`, pricePlanID).Scan(&initialPlanID, &initialVersionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return managedPricePlanBindingContext{}, newBusinessPlanAdminError(http.StatusNotFound, "PRICE_PLAN_NOT_FOUND", "price plan not found")
		}
		return managedPricePlanBindingContext{}, err
	}
	var item managedPricePlanBindingContext
	item.ID = pricePlanID
	if err := tx.QueryRowContext(ctx, `
		select coalesce(plan_type,''),active from xz_plans where id=$1 for update
	`, initialPlanID).Scan(&item.PlanType, &item.PlanActive); err != nil {
		return item, err
	}
	var versionPlanID string
	if err := tx.QueryRowContext(ctx, `
		select plan_id,status,business_type from xz_plan_versions where id=$1 for update
	`, initialVersionID).Scan(&versionPlanID, &item.VersionStatus, &item.BusinessType); err != nil {
		return item, err
	}
	if err := tx.QueryRowContext(ctx, `
		select plan_id,plan_version_id,sale_price_cents,channel,environment,enabled,status,is_default
		from xz_price_plans where id=$1 for update
	`, pricePlanID).Scan(&item.PlanID, &item.PlanVersionID, &item.SalePriceCents, &item.Channel,
		&item.Environment, &item.Enabled, &item.Status, &item.IsDefault); err != nil {
		return item, err
	}
	if item.PlanID != initialPlanID || item.PlanVersionID != initialVersionID || versionPlanID != item.PlanID {
		return item, newBusinessPlanAdminError(http.StatusConflict, "PRICE_PLAN_CONFIGURATION_CHANGED", "price plan ownership changed while it was being locked")
	}
	return validateManagedPricePlanBindingContext(item)
}

const paymentBindingAdminSelect = `
	select b.id,b.price_plan_id,b.wechat_good_id,b.channel,b.environment,b.provider_price_snapshot_cents,
	       b.enabled,b.status,b.revision,coalesce(b.created_by,''),coalesce(b.updated_by,''),
	       coalesce(b.enabled_by,''),b.enabled_at,coalesce(b.disabled_by,''),b.disabled_at,b.created_at,b.updated_at,
	       pp.sale_price_cents,g.platform_price_cents,g.product_id,
	       case when g.verification_status='MANUALLY_CONFIRMED_PUBLISHED'
	                  and g.verification_expires_at is not null and g.verification_expires_at<=now()
	            then 'VERIFICATION_EXPIRED' else g.verification_status end,
	       pp.sale_price_cents=b.provider_price_snapshot_cents and pp.sale_price_cents=g.platform_price_cents,
	       pp.channel=b.channel and pp.channel=g.channel and pp.environment=b.environment and pp.environment=g.environment
	from xz_price_plan_payment_bindings b
	join xz_price_plans pp on pp.id=b.price_plan_id
	join xz_wechat_virtual_goods g on g.id=b.wechat_good_id
`

type paymentBindingScanner interface {
	Scan(...any) error
}

func scanPricePlanPaymentBinding(scanner paymentBindingScanner) (pricePlanPaymentBindingAdminView, error) {
	var item pricePlanPaymentBindingAdminView
	var enabledAt, disabledAt sql.NullTime
	err := scanner.Scan(
		&item.ID, &item.PricePlanID, &item.WeChatGoodID, &item.Channel, &item.Environment,
		&item.ProviderPriceSnapshotCents, &item.Enabled, &item.Status, &item.Revision, &item.CreatedBy,
		&item.UpdatedBy, &item.EnabledBy, &enabledAt, &item.DisabledBy, &disabledAt, &item.CreatedAt,
		&item.UpdatedAt, &item.PricePlanSalePriceCents, &item.WeChatGoodPriceCents, &item.WeChatProductID,
		&item.VerificationStatus, &item.PriceConsistent, &item.EnvironmentConsistent,
	)
	if err != nil {
		return item, err
	}
	if enabledAt.Valid {
		item.EnabledAt = &enabledAt.Time
	}
	if disabledAt.Valid {
		item.DisabledAt = &disabledAt.Time
	}
	return item, nil
}

func validatePaymentBindingCompatibility(price managedPricePlanBindingContext, good wechatVirtualGoodAdminView, bindingPrice int64, activating bool, now time.Time) error {
	if !strings.EqualFold(price.Channel, good.Channel) || !strings.EqualFold(price.Environment, good.Environment) {
		return newBusinessPlanAdminError(http.StatusConflict, "PRICE_PLAN_PAYMENT_ENV_MISMATCH", fmt.Sprintf("price plan %s/%s does not match WeChat good %s/%s", price.Channel, price.Environment, good.Channel, good.Environment))
	}
	if price.SalePriceCents <= 0 || price.SalePriceCents != bindingPrice || price.SalePriceCents != good.PlatformPriceCents {
		return newBusinessPlanAdminError(http.StatusConflict, "PRICE_PLAN_WECHAT_PRICE_MISMATCH", fmt.Sprintf("pricePlan=%d binding=%d wechatGood=%d", price.SalePriceCents, bindingPrice, good.PlatformPriceCents))
	}
	if !activating {
		return nil
	}
	if !price.PlanActive || !strings.EqualFold(price.VersionStatus, "ACTIVE") {
		return newBusinessPlanAdminError(http.StatusConflict, "PRICE_PLAN_NOT_ACTIVE", "business plan and entitlement version must be active before enabling a binding")
	}
	if !strings.EqualFold(price.Status, "DRAFT") && !strings.EqualFold(price.Status, "INACTIVE") && !strings.EqualFold(price.Status, "ACTIVE") {
		return newBusinessPlanAdminError(http.StatusConflict, "PRICE_PLAN_STATE_INVALID", "payment binding cannot be enabled for the current price plan state")
	}
	if good.VerificationStatus == wechatGoodVerificationExpired {
		return newBusinessPlanAdminError(http.StatusConflict, "WECHAT_GOOD_VERIFICATION_EXPIRED", "manual WeChat publication confirmation has expired")
	}
	if !good.manuallyConfirmedAt(now) {
		return newBusinessPlanAdminError(http.StatusConflict, "WECHAT_GOOD_NOT_CONFIRMED", "WeChat good must be manually confirmed as published before enabling a binding")
	}
	if !good.Enabled || !good.Published || good.Status != "PUBLISHED" {
		return newBusinessPlanAdminError(http.StatusConflict, "WECHAT_GOOD_NOT_AVAILABLE", "WeChat good is not locally enabled and published")
	}
	return nil
}

func (s *postgresStore) listPricePlanPaymentBindings(ctx context.Context, pricePlanID string) ([]pricePlanPaymentBindingAdminView, error) {
	if _, err := loadManagedPricePlanBindingContext(ctx, s.db, pricePlanID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, paymentBindingAdminSelect+` where b.price_plan_id=$1 order by b.created_at desc`, strings.TrimSpace(pricePlanID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []pricePlanPaymentBindingAdminView{}
	for rows.Next() {
		item, err := scanPricePlanPaymentBinding(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) createPricePlanPaymentBinding(ctx context.Context, pricePlanID string, mutation pricePlanPaymentBindingCreateMutation, actorID, actorRole string) (pricePlanPaymentBindingAdminView, error) {
	if err := pricingAdminActor(actorID); err != nil {
		return pricePlanPaymentBindingAdminView{}, err
	}
	if err := pricingAdminReason(mutation.Reason); err != nil {
		return pricePlanPaymentBindingAdminView{}, err
	}
	if strings.TrimSpace(mutation.WeChatGoodID) == "" {
		return pricePlanPaymentBindingAdminView{}, newBusinessPlanAdminError(http.StatusBadRequest, "WECHAT_GOOD_REQUIRED", "wechatGoodId is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return pricePlanPaymentBindingAdminView{}, err
	}
	defer tx.Rollback()
	price, err := loadManagedPricePlanBindingContextForUpdate(ctx, tx, pricePlanID)
	if err != nil {
		return pricePlanPaymentBindingAdminView{}, err
	}
	good, err := loadWechatVirtualGoodForUpdate(ctx, tx, mutation.WeChatGoodID)
	if err != nil {
		return pricePlanPaymentBindingAdminView{}, err
	}
	if err := validatePaymentBindingCompatibility(price, good, good.PlatformPriceCents, false, time.Now().UTC()); err != nil {
		return pricePlanPaymentBindingAdminView{}, err
	}
	bindingID := strings.Replace(newAuditID(), "audit_", "payment_binding_", 1)
	_, err = tx.ExecContext(ctx, `
		insert into xz_price_plan_payment_bindings(
			id,price_plan_id,wechat_good_id,channel,environment,provider_price_snapshot_cents,
			enabled,status,created_by,updated_by
		) values($1,$2,$3,$4,$5,$6,false,'DRAFT',$7,$7)
	`, bindingID, price.ID, good.ID, price.Channel, price.Environment, good.PlatformPriceCents, actorID)
	if isPostgresUniqueViolation(err) {
		return pricePlanPaymentBindingAdminView{}, newBusinessPlanAdminError(http.StatusConflict, "PAYMENT_BINDING_ALREADY_EXISTS", "price plan already has a payment binding in this channel and environment")
	}
	if err != nil {
		return pricePlanPaymentBindingAdminView{}, err
	}
	created, err := scanPricePlanPaymentBinding(tx.QueryRowContext(ctx, paymentBindingAdminSelect+` where b.id=$1`, bindingID))
	if err != nil {
		return pricePlanPaymentBindingAdminView{}, err
	}
	if err := insertAuditLog(ctx, tx, actorID, actorRole, "price_plan.payment_binding.create", "price_plan_payment_binding", bindingID, "POST", "", http.StatusCreated, map[string]any{
		"changeReason": mutation.Reason, "planId": price.PlanID, "planVersionId": price.PlanVersionID,
		"pricePlanId": price.ID, "paymentBindingId": bindingID, "wechatGoodId": good.ID,
		"channel": price.Channel, "environment": price.Environment, "providerPriceSnapshotCents": good.PlatformPriceCents,
		"revisionBefore": int64(0), "revisionAfter": created.Revision, "afterSnapshot": created,
	}); err != nil {
		return pricePlanPaymentBindingAdminView{}, err
	}
	if err := tx.Commit(); err != nil {
		return pricePlanPaymentBindingAdminView{}, err
	}
	return created, nil
}

type paymentBindingRow struct {
	ID                         string
	PricePlanID                string
	WeChatGoodID               string
	Channel                    string
	Environment                string
	ProviderPriceSnapshotCents int64
	Enabled                    bool
	Status                     string
	Revision                   int64
}

func loadPaymentBindingForUpdate(ctx context.Context, tx *sql.Tx, bindingID string) (paymentBindingRow, error) {
	var item paymentBindingRow
	err := tx.QueryRowContext(ctx, `
		select id,price_plan_id,wechat_good_id,channel,environment,provider_price_snapshot_cents,
		       enabled,status,revision
		from xz_price_plan_payment_bindings where id=$1 for update
	`, strings.TrimSpace(bindingID)).Scan(&item.ID, &item.PricePlanID, &item.WeChatGoodID, &item.Channel,
		&item.Environment, &item.ProviderPriceSnapshotCents, &item.Enabled, &item.Status, &item.Revision)
	if errors.Is(err, sql.ErrNoRows) {
		return item, newBusinessPlanAdminError(http.StatusNotFound, "PAYMENT_BINDING_NOT_FOUND", "payment binding not found")
	}
	return item, err
}

func (s *postgresStore) updatePricePlanPaymentBinding(ctx context.Context, bindingID string, mutation pricePlanPaymentBindingMutation, actorID, actorRole string) (pricePlanPaymentBindingAdminView, error) {
	if err := pricingAdminActor(actorID); err != nil {
		return pricePlanPaymentBindingAdminView{}, err
	}
	if err := pricingAdminReason(mutation.Reason); err != nil {
		return pricePlanPaymentBindingAdminView{}, err
	}
	requestedGoodID := ""
	if mutation.WeChatGoodID != nil {
		requestedGoodID = strings.TrimSpace(*mutation.WeChatGoodID)
	}
	if mutation.Enabled == nil && requestedGoodID == "" {
		return pricePlanPaymentBindingAdminView{}, newBusinessPlanAdminError(http.StatusBadRequest, "PAYMENT_BINDING_MUTATION_REQUIRED", "enabled or wechatGoodId is required")
	}
	if mutation.Enabled != nil && requestedGoodID != "" {
		return pricePlanPaymentBindingAdminView{}, newBusinessPlanAdminError(http.StatusBadRequest, "PAYMENT_BINDING_MUTATION_CONFLICT", "rebind and state transition must be separate operations")
	}
	var pricePlanID, goodID string
	if err := s.db.QueryRowContext(ctx, `select price_plan_id,wechat_good_id from xz_price_plan_payment_bindings where id=$1`, strings.TrimSpace(bindingID)).Scan(&pricePlanID, &goodID); errors.Is(err, sql.ErrNoRows) {
		return pricePlanPaymentBindingAdminView{}, newBusinessPlanAdminError(http.StatusNotFound, "PAYMENT_BINDING_NOT_FOUND", "payment binding not found")
	} else if err != nil {
		return pricePlanPaymentBindingAdminView{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return pricePlanPaymentBindingAdminView{}, err
	}
	defer tx.Rollback()
	price, err := loadManagedPricePlanBindingContextForUpdate(ctx, tx, pricePlanID)
	if err != nil {
		return pricePlanPaymentBindingAdminView{}, err
	}
	targetGoodID := goodID
	if requestedGoodID != "" {
		targetGoodID = requestedGoodID
	}
	good, err := loadWechatVirtualGoodForUpdate(ctx, tx, targetGoodID)
	if err != nil {
		return pricePlanPaymentBindingAdminView{}, err
	}
	current, err := loadPaymentBindingForUpdate(ctx, tx, bindingID)
	if err != nil {
		return pricePlanPaymentBindingAdminView{}, err
	}
	if current.PricePlanID != pricePlanID || current.WeChatGoodID != goodID {
		return pricePlanPaymentBindingAdminView{}, newBusinessPlanAdminError(http.StatusConflict, "PAYMENT_BINDING_CONFIGURATION_CHANGED", "payment binding changed while it was being updated")
	}
	if mutation.Revision <= 0 || current.Revision != mutation.Revision {
		return pricePlanPaymentBindingAdminView{}, pricingRevisionConflict("payment binding")
	}
	beforeView, err := scanPricePlanPaymentBinding(tx.QueryRowContext(ctx, paymentBindingAdminSelect+` where b.id=$1`, current.ID))
	if err != nil {
		return pricePlanPaymentBindingAdminView{}, err
	}
	defaultDependencyActive := price.IsDefault && current.Enabled && strings.EqualFold(current.Status, "ACTIVE")
	if defaultDependencyActive && (requestedGoodID != "" || (mutation.Enabled != nil && !*mutation.Enabled)) {
		return pricePlanPaymentBindingAdminView{}, defaultPricePlanDependencyDisableError()
	}
	now := time.Now().UTC()
	action := "price_plan.payment_binding.rebind"
	status := "DRAFT"
	if requestedGoodID != "" {
		if current.Enabled || strings.EqualFold(current.Status, "ACTIVE") {
			return pricePlanPaymentBindingAdminView{}, newBusinessPlanAdminError(http.StatusConflict, "PAYMENT_BINDING_ACTIVE", "disable the payment binding before rebinding it")
		}
		var hasHistory bool
		if err := tx.QueryRowContext(ctx, `select exists(select 1 from xz_order_price_quotes where payment_binding_id=$1)`, current.ID).Scan(&hasHistory); err != nil {
			return pricePlanPaymentBindingAdminView{}, err
		}
		if hasHistory {
			return pricePlanPaymentBindingAdminView{}, newBusinessPlanAdminError(http.StatusConflict, "PAYMENT_BINDING_HAS_HISTORY", "a payment binding referenced by a quote cannot be rebound")
		}
		if err := validatePaymentBindingCompatibility(price, good, good.PlatformPriceCents, false, now); err != nil {
			return pricePlanPaymentBindingAdminView{}, err
		}
		result, err := tx.ExecContext(ctx, `
			update xz_price_plan_payment_bindings set
				wechat_good_id=$2,channel=$3,environment=$4,provider_price_snapshot_cents=$5,
				enabled=false,status='DRAFT',updated_by=$6
			where id=$1 and revision=$7
		`, current.ID, good.ID, price.Channel, price.Environment, good.PlatformPriceCents, actorID, mutation.Revision)
		if err != nil {
			return pricePlanPaymentBindingAdminView{}, err
		}
		if affected, _ := result.RowsAffected(); affected != 1 {
			return pricePlanPaymentBindingAdminView{}, pricingRevisionConflict("payment binding")
		}
	} else {
		action = "price_plan.payment_binding.disable"
		status = "DISABLED"
		if *mutation.Enabled {
			if err := validatePaymentBindingCompatibility(price, good, current.ProviderPriceSnapshotCents, true, now); err != nil {
				return pricePlanPaymentBindingAdminView{}, err
			}
			action = "price_plan.payment_binding.activate"
			status = "ACTIVE"
		}
		result, err := tx.ExecContext(ctx, `
			update xz_price_plan_payment_bindings set enabled=$2,status=$3,updated_by=$4,
				enabled_by=case when $2 then coalesce(enabled_by,$4)
				  when enabled or status='ACTIVE' then coalesce(enabled_by,$4) else enabled_by end,
				enabled_at=case when $2 then coalesce(enabled_at,$5)
				  when enabled or status='ACTIVE' then coalesce(enabled_at,created_at,$5) else enabled_at end,
				disabled_by=case when not $2 then $4 else null end,
				disabled_at=case when not $2 then $5 else null end
			where id=$1 and revision=$6
		`, current.ID, *mutation.Enabled, status, actorID, now, mutation.Revision)
		if err != nil {
			return pricePlanPaymentBindingAdminView{}, err
		}
		if affected, _ := result.RowsAffected(); affected != 1 {
			return pricePlanPaymentBindingAdminView{}, pricingRevisionConflict("payment binding")
		}
	}
	updated, err := scanPricePlanPaymentBinding(tx.QueryRowContext(ctx, paymentBindingAdminSelect+` where b.id=$1`, current.ID))
	if err != nil {
		return pricePlanPaymentBindingAdminView{}, err
	}
	if err := insertAuditLog(ctx, tx, actorID, actorRole, action, "price_plan_payment_binding", current.ID, "PATCH", "", http.StatusOK, map[string]any{
		"changeReason": mutation.Reason, "planId": price.PlanID, "planVersionId": price.PlanVersionID,
		"pricePlanId": price.ID, "paymentBindingId": current.ID, "wechatGoodId": good.ID,
		"previousWechatGoodId": current.WeChatGoodID, "environment": updated.Environment,
		"revisionBefore": current.Revision, "revisionAfter": updated.Revision,
		"providerPriceSnapshotCents": updated.ProviderPriceSnapshotCents,
		"beforeSnapshot":             beforeView, "afterSnapshot": updated,
	}); err != nil {
		return pricePlanPaymentBindingAdminView{}, err
	}
	if err := tx.Commit(); err != nil {
		return pricePlanPaymentBindingAdminView{}, err
	}
	return updated, nil
}
