package httpserver

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"xianzhi-ai/backend-go/internal/config"
)

func beginPricingHealthTransaction(ctx context.Context, db *sql.DB) (*sql.Tx, error) {
	return db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
}

func (s *postgresStore) pricingHealth(ctx context.Context, cfg config.Config) (pricingHealthView, error) {
	checkedAt := time.Now().UTC()
	view := pricingHealthView{
		CheckedAt: checkedAt,
		Issues:    []pricingHealthIssue{}, BusinessPlans: []pricingHealthBusinessPlan{},
		PricePlans: []pricingHealthPricePlan{}, WeChatGoods: []pricingHealthWeChatGood{},
		Runtime: pricingHealthRuntime{
			PricePlanCreationEnabled:     cfg.PricePlanCreationEnabled,
			PricePlanTestEntryEnabled:    cfg.PricePlanTestEntryEnabled,
			SnapshotV2FulfillmentEnabled: cfg.SnapshotV2FulfillmentEnabled,
			V132Scope:                    "NONE", V132AffectedTenantIDs: []string{},
		},
	}
	tx, err := beginPricingHealthTransaction(ctx, s.db)
	if err != nil {
		return pricingHealthView{}, err
	}
	defer tx.Rollback()
	if err := assertPricingHealthTransaction(ctx, tx); err != nil {
		return pricingHealthView{}, err
	}
	if err := loadPricingHealthBusinessPlans(ctx, tx, &view); err != nil {
		return pricingHealthView{}, err
	}
	if err := loadPricingHealthPricePlans(ctx, tx, &view); err != nil {
		return pricingHealthView{}, err
	}
	if err := loadPricingHealthGoods(ctx, tx, &view); err != nil {
		return pricingHealthView{}, err
	}
	if err := loadPricingHealthRollout(ctx, tx, &view); err != nil {
		return pricingHealthView{}, err
	}
	evaluatePricingHealth(&view, checkedAt)
	finalizePricingHealth(&view)
	if err := tx.Commit(); err != nil {
		return pricingHealthView{}, err
	}
	return view, nil
}

func assertPricingHealthTransaction(ctx context.Context, tx *sql.Tx) error {
	var isolation, readOnly string
	if err := tx.QueryRowContext(ctx, `select current_setting('transaction_isolation'),current_setting('transaction_read_only')`).Scan(&isolation, &readOnly); err != nil {
		return err
	}
	if isolation != "repeatable read" || readOnly != "on" {
		return fmt.Errorf("pricing health transaction invariant failed")
	}
	return nil
}

func loadPricingHealthBusinessPlans(ctx context.Context, tx *sql.Tx, view *pricingHealthView) error {
	rows, err := tx.QueryContext(ctx, `
		select plans.id,plans.name,plans.active,coalesce(active_version.id,'')
		from xz_plans plans
		left join lateral (
			select versions.id from xz_plan_versions versions
			where versions.plan_id=plans.id and versions.status='ACTIVE'
			  and ((plans.plan_type='MEMBER_PACKAGE' and versions.business_type='MEMBER')
			    or (plans.plan_type='AGENT_JOIN_PACKAGE' and versions.business_type='AGENT'))
			  and (versions.effective_at is null or versions.effective_at <= $1)
			  and (versions.expires_at is null or versions.expires_at > $1)
			order by versions.version_no desc,versions.id limit 1
		) active_version on true
		where exists (
			select 1 from xz_plan_versions versions where versions.plan_id=plans.id
			  and ((plans.plan_type='MEMBER_PACKAGE' and versions.business_type='MEMBER')
			    or (plans.plan_type='AGENT_JOIN_PACKAGE' and versions.business_type='AGENT'))
		)
		order by plans.id
	`, view.CheckedAt)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		item := pricingHealthBusinessPlan{IssueCodes: []string{}, Defaults: pricingHealthBusinessPlanDefaults{}, channels: map[string]bool{}}
		if err := rows.Scan(&item.PlanID, &item.Name, &item.active, &item.ActiveVersionID); err != nil {
			return err
		}
		view.BusinessPlans = append(view.BusinessPlans, item)
	}
	return rows.Err()
}

func loadPricingHealthPricePlans(ctx context.Context, tx *sql.Tx, view *pricingHealthView) error {
	rows, err := tx.QueryContext(ctx, `
		select prices.id,prices.plan_id,prices.plan_version_id,prices.name,prices.price_type,prices.channel,prices.environment,
		       prices.status,prices.sale_price_cents,prices.currency,prices.is_default,prices.enabled,prices.bonus_points,
		       (plans.active and versions.status='ACTIVE'
		         and (versions.effective_at is null or versions.effective_at <= $1)
		         and (versions.expires_at is null or versions.expires_at > $1)
		         and prices.enabled and prices.status='ACTIVE' and prices.is_visible and prices.audience_type='PUBLIC'
		         and prices.price_type<>'TEST' and prices.currency='CNY' and prices.channel='WECHAT_VIRTUAL'
		         and prices.environment in ('PRODUCTION','SANDBOX')
		         and (prices.effective_at is null or prices.effective_at <= $1)
		         and (prices.expires_at is null or prices.expires_at > $1)),
		       (plans.active and versions.status='ACTIVE'
		         and (versions.effective_at is null or versions.effective_at <= $1)
		         and (versions.expires_at is null or versions.expires_at > $1)
		         and prices.enabled and prices.status='ACTIVE' and prices.price_type='TEST'
		         and prices.is_default=false and prices.is_visible=false
		         and (prices.audience_type<>'PUBLIC' or prices.created_by is null)
		         and prices.currency='CNY' and prices.channel='WECHAT_VIRTUAL'
		         and prices.environment in ('PRODUCTION','SANDBOX')
		         and (prices.effective_at is null or prices.effective_at <= $1)
		         and (prices.expires_at is null or prices.expires_at > $1)),
		       coalesce(bindings.id,''),coalesce(bindings.wechat_good_id,''),coalesce(bindings.enabled,false),
		       coalesce(bindings.status,''),coalesce(bindings.provider_price_snapshot_cents,0),
		       coalesce(goods.product_id,''),coalesce(goods.channel,''),coalesce(goods.environment,''),
		       coalesce(goods.platform_price_cents,0),coalesce(goods.verification_status,''),goods.verification_expires_at,
		       (select count(*) from xz_order_price_quotes quotes where quotes.price_plan_id=prices.id),
		       (select count(*) from xz_orders orders where orders.price_plan_id=prices.id),
		       (select count(*) from xz_price_plan_user_whitelist whitelist
		        where whitelist.price_plan_id=prices.id
		          and coalesce(whitelist.lifecycle_status,case when whitelist.enabled then 'ACTIVE' else 'DISABLED' end)='ACTIVE'
		          and (whitelist.effective_at is null or whitelist.effective_at <= $1)
		          and (whitelist.expires_at is null or whitelist.expires_at > $1))
		from xz_price_plans prices
		join xz_plans plans on plans.id=prices.plan_id
		join xz_plan_versions versions on versions.id=prices.plan_version_id and versions.plan_id=prices.plan_id
		left join lateral (
			select candidate.* from xz_price_plan_payment_bindings candidate
			where candidate.price_plan_id=prices.id
			order by (candidate.enabled and candidate.status='ACTIVE') desc,
			         (candidate.channel=prices.channel and candidate.environment=prices.environment) desc,
			         candidate.updated_at desc,candidate.id
			limit 1
		) bindings on true
		left join xz_wechat_virtual_goods goods on goods.id=bindings.wechat_good_id
		where (plans.plan_type='MEMBER_PACKAGE' and versions.business_type='MEMBER')
		   or (plans.plan_type='AGENT_JOIN_PACKAGE' and versions.business_type='AGENT')
		order by prices.plan_id,prices.environment,prices.channel,prices.id
	`, view.CheckedAt)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		item := pricingHealthPricePlan{IssueCodes: []string{}}
		var expiry sql.NullTime
		if err := rows.Scan(
			&item.PricePlanID, &item.PlanID, &item.PlanVersionID, &item.Name, &item.PriceType, &item.Channel, &item.Environment,
			&item.Status, &item.SalePriceCents, &item.Currency, &item.isDefault, &item.enabled, &item.giftPoints, &item.publicEligible, &item.testEligible,
			&item.PaymentBindingID, &item.WeChatGoodID, &item.bindingEnabled, &item.bindingStatus, &item.bindingPrice,
			&item.WeChatProductID, &item.goodChannel, &item.goodEnvironment, &item.goodPrice, &item.goodVerification, &expiry,
			&item.QuoteCount, &item.OrderCount, &item.whitelistCount,
		); err != nil {
			return err
		}
		if expiry.Valid {
			item.goodExpiry = &expiry.Time
		}
		view.PricePlans = append(view.PricePlans, item)
	}
	return rows.Err()
}

func loadPricingHealthGoods(ctx context.Context, tx *sql.Tx, view *pricingHealthView) error {
	rows, err := tx.QueryContext(ctx, `
		select goods.id,goods.product_id,goods.environment,count(bindings.id) filter (where
		  (plans.plan_type='MEMBER_PACKAGE' and versions.business_type='MEMBER')
		  or (plans.plan_type='AGENT_JOIN_PACKAGE' and versions.business_type='AGENT'))
		from xz_wechat_virtual_goods goods
		left join xz_price_plan_payment_bindings bindings on bindings.wechat_good_id=goods.id
		left join xz_price_plans prices on prices.id=bindings.price_plan_id
		left join xz_plans plans on plans.id=prices.plan_id
		left join xz_plan_versions versions on versions.id=prices.plan_version_id and versions.plan_id=prices.plan_id
		group by goods.id,goods.product_id,goods.environment
		order by goods.id
	`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var item pricingHealthWeChatGood
		if err := rows.Scan(&item.WeChatGoodID, &item.WeChatProductID, &item.Environment, &item.ReferenceCount); err != nil {
			return err
		}
		view.WeChatGoods = append(view.WeChatGoods, item)
	}
	return rows.Err()
}

func loadPricingHealthRollout(ctx context.Context, tx *sql.Tx, view *pricingHealthView) error {
	rows, err := tx.QueryContext(ctx, `
		select tenant_id,mode from xz_channel_rollout_configs
		where enabled=true and real_switch_enabled=true and mode in ('V132','CANARY','V132_CANARY','V132_FULL')
		order by tenant_id
	`)
	if err != nil {
		return err
	}
	defer rows.Close()
	full, canary := false, false
	for rows.Next() {
		var tenantID, mode string
		if err := rows.Scan(&tenantID, &mode); err != nil {
			return err
		}
		view.Runtime.V132AffectedTenantIDs = append(view.Runtime.V132AffectedTenantIDs, tenantID)
		if mode == "V132" || mode == "V132_FULL" {
			full = true
		} else {
			canary = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	view.Runtime.V132AffectedTenantCount = len(view.Runtime.V132AffectedTenantIDs)
	view.Runtime.V132Blocked = view.Runtime.V132AffectedTenantCount > 0
	switch {
	case full && canary:
		view.Runtime.V132Scope = "TENANT_MIXED"
	case full:
		view.Runtime.V132Scope = "TENANT_FULL"
	case canary:
		view.Runtime.V132Scope = "TENANT_CANARY"
	}
	return nil
}

func evaluatePricingHealth(view *pricingHealthView, now time.Time) {
	planIndexes := map[string]int{}
	for i := range view.BusinessPlans {
		planIndexes[view.BusinessPlans[i].PlanID] = i
	}
	defaultCombos := map[string]bool{}
	for i := range view.PricePlans {
		price := &view.PricePlans[i]
		planIndex, managed := planIndexes[price.PlanID]
		if !managed {
			continue
		}
		plan := &view.BusinessPlans[planIndex]
		plan.PricePlanCount++
		if price.publicEligible {
			combo := price.PlanID + "|" + price.Channel + "|" + price.Environment
			plan.channels[combo] = true
			if price.isDefault {
				defaultCombos[combo] = true
				summary := &pricingHealthDefaultSummary{PricePlanID: price.PricePlanID, SalePriceCents: price.SalePriceCents, Currency: price.Currency, WeChatGoodID: price.WeChatGoodID, WeChatProductID: price.WeChatProductID}
				if price.Environment == "PRODUCTION" && plan.Defaults.Production == nil {
					plan.Defaults.Production = summary
				}
				if price.Environment == "SANDBOX" && plan.Defaults.Sandbox == nil {
					plan.Defaults.Sandbox = summary
				}
			}
		}
		if !price.enabled || price.Status != "ACTIVE" {
			appendPricingHealthIssue(view, pricingHealthPriceIssue(*price, pricingHealthIssueDisabled, pricingHealthSeverityWarning, "PRICE_PLAN", "price plan is not locally active"))
		}
		chainEligible := price.publicEligible || price.testEligible
		if chainEligible && price.PaymentBindingID == "" {
			appendPricingHealthIssue(view, pricingHealthPriceIssue(*price, pricingHealthIssueBindingMissing, pricingHealthSeverityBlocking, "PRICE_PLAN", "price plan has no local payment binding"))
		} else if chainEligible {
			if !price.bindingEnabled || price.bindingStatus != "ACTIVE" {
				appendPricingHealthIssue(view, pricingHealthPriceIssue(*price, pricingHealthIssueDisabled, pricingHealthSeverityWarning, "PAYMENT_BINDING", "payment binding is not locally active"))
			}
			if price.goodVerification == "MANUALLY_CONFIRMED_PUBLISHED" && price.goodExpiry != nil && !now.Before(*price.goodExpiry) {
				appendPricingHealthIssue(view, pricingHealthPriceIssue(*price, pricingHealthIssueGoodVerificationExpired, pricingHealthSeverityBlocking, "WECHAT_GOOD", "local WeChat publication confirmation has expired"))
			} else if price.goodVerification != "MANUALLY_CONFIRMED_PUBLISHED" {
				appendPricingHealthIssue(view, pricingHealthPriceIssue(*price, pricingHealthIssueGoodNotConfirmed, pricingHealthSeverityBlocking, "WECHAT_GOOD", "WeChat good is not locally confirmed as published"))
			}
			if price.Channel != price.goodChannel || price.Environment != price.goodEnvironment {
				appendPricingHealthIssue(view, pricingHealthPriceIssue(*price, pricingHealthIssuePaymentEnvironmentMismatch, pricingHealthSeverityBlocking, "PRICE_PLAN", "price plan, binding and WeChat good channel or environment differ"))
			}
			if price.SalePriceCents != price.bindingPrice || price.SalePriceCents != price.goodPrice {
				appendPricingHealthIssue(view, pricingHealthPriceIssue(*price, pricingHealthIssuePriceMismatch, pricingHealthSeverityBlocking, "PRICE_PLAN", "price plan, binding and WeChat good prices differ"))
			}
		}
		if price.testEligible && price.whitelistCount == 0 {
			appendPricingHealthIssue(view, pricingHealthPriceIssue(*price, pricingHealthIssueWhitelistMissing, pricingHealthSeverityBlocking, "PRICE_PLAN", "TEST price plan has no currently active whitelist member"))
		}
		if price.giftPoints > 0 {
			appendPricingHealthIssue(view, pricingHealthPriceIssue(*price, pricingHealthIssueGiftPointsUnavailable, pricingHealthSeverityBlocking, "PRICE_PLAN", "giftPoints fulfillment remains unavailable and fail-closed"))
		}
	}
	for i := range view.BusinessPlans {
		plan := &view.BusinessPlans[i]
		if !plan.active {
			appendPricingHealthIssue(view, healthIssue(pricingHealthIssueDisabled, pricingHealthSeverityWarning, "BUSINESS_PLAN", plan.PlanID, "", "", "business plan is not active"))
		}
		if plan.ActiveVersionID == "" {
			appendPricingHealthIssue(view, healthIssue(pricingHealthIssueEntitlementVersionMissing, pricingHealthSeverityBlocking, "BUSINESS_PLAN", plan.PlanID, "", "", "business plan has no ACTIVE entitlement version"))
		}
		if plan.PricePlanCount == 0 {
			appendPricingHealthIssue(view, healthIssue(pricingHealthIssuePricePlanMissing, pricingHealthSeverityBlocking, "BUSINESS_PLAN", plan.PlanID, "", "", "business plan has no V2 price plan"))
		}
		keys := make([]string, 0, len(plan.channels))
		for key := range plan.channels {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, combo := range keys {
			if !defaultCombos[combo] {
				parts := strings.Split(combo, "|")
				appendPricingHealthIssue(view, healthIssue(pricingHealthIssueDefaultMissing, pricingHealthSeverityBlocking, "BUSINESS_PLAN", plan.PlanID, "", parts[2], "configured channel and environment has no default price plan"))
			}
		}
	}
	if view.Runtime.V132Blocked {
		appendPricingHealthIssue(view, pricingHealthIssue{Code: pricingHealthIssueV132Blocked, Severity: pricingHealthSeverityBlocking, Scope: "SYSTEM", Message: fmt.Sprintf("V132 runtime remains fail-closed for %d configured tenant scope(s)", view.Runtime.V132AffectedTenantCount)})
	}
	for name, enabled := range map[string]bool{"price plan creation": view.Runtime.PricePlanCreationEnabled, "TEST entry": view.Runtime.PricePlanTestEntryEnabled, "snapshot V2 fulfillment": view.Runtime.SnapshotV2FulfillmentEnabled} {
		if !enabled {
			appendPricingHealthIssue(view, pricingHealthIssue{Code: pricingHealthIssueDisabled, Severity: pricingHealthSeverityWarning, Scope: "RUNTIME", Message: name + " is disabled"})
		}
	}
}

func healthIssue(code, severity, scope, planID, pricePlanID, environment, message string) pricingHealthIssue {
	return pricingHealthIssue{Code: code, Severity: severity, Scope: scope, PlanID: planID, PricePlanID: pricePlanID, Environment: environment, Message: message}
}

func pricingHealthPriceIssue(price pricingHealthPricePlan, code, severity, scope, message string) pricingHealthIssue {
	issue := healthIssue(code, severity, scope, price.PlanID, price.PricePlanID, price.Environment, message)
	issue.PaymentBindingID = price.PaymentBindingID
	issue.WeChatGoodID = price.WeChatGoodID
	return issue
}
