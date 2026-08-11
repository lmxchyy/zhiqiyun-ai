package operationcenter

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

var ErrProductionReleaseGateFailed = errors.New("operation center production release gate failed")

type ReleaseGateQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type ProductionReleaseGateConfig struct {
	Environment           string
	Runtime               OperationCenterRuntimeConfig
	ProviderMappings      map[string]string
	FinancialSubmitterID  string
	FinancialApproverID   string
	TestConfigurationKeys []string
}

type ProductionReleaseGateCheck struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

type ProductionReleaseGateMetric struct {
	Name   string            `json:"name"`
	Value  int64             `json:"value"`
	Unit   string            `json:"unit"`
	Labels map[string]string `json:"labels,omitempty"`
}

type ProductionReleaseGateReport struct {
	CheckedAt time.Time                     `json:"checkedAt"`
	Passed    bool                          `json:"passed"`
	Checks    []ProductionReleaseGateCheck  `json:"checks"`
	Metrics   []ProductionReleaseGateMetric `json:"metrics"`
}

func LoadProductionReleaseGateConfig(lookup RuntimeEnvironmentLookup, environment []string) (ProductionReleaseGateConfig, error) {
	if lookup == nil {
		lookup = os.LookupEnv
	}
	runtimeConfig, err := LoadOperationCenterRuntimeConfig("production", lookup)
	if err != nil {
		return ProductionReleaseGateConfig{}, err
	}
	mappingValue, _ := lookup("XIANZHI_PRODUCTION_OPERATION_CENTER_REFUND_PROVIDER_MAPPINGS")
	mappings, err := parseProductionProviderMappings(mappingValue)
	if err != nil {
		return ProductionReleaseGateConfig{}, err
	}
	submitter, _ := lookup("XIANZHI_PRODUCTION_OPERATION_CENTER_FINANCIAL_SUBMITTER_ID")
	approver, _ := lookup("XIANZHI_PRODUCTION_OPERATION_CENTER_FINANCIAL_APPROVER_ID")
	testKeys := make([]string, 0)
	for _, entry := range environment {
		key, value, found := strings.Cut(entry, "=")
		if found && strings.HasPrefix(strings.TrimSpace(key), "XIANZHI_TEST_OPERATION_CENTER_") && strings.TrimSpace(value) != "" {
			testKeys = append(testKeys, strings.TrimSpace(key))
		}
	}
	sort.Strings(testKeys)
	return ProductionReleaseGateConfig{
		Environment: "production", Runtime: runtimeConfig, ProviderMappings: mappings,
		FinancialSubmitterID: strings.TrimSpace(submitter), FinancialApproverID: strings.TrimSpace(approver),
		TestConfigurationKeys: testKeys,
	}, nil
}

func parseProductionProviderMappings(value string) (map[string]string, error) {
	result := map[string]string{}
	for _, entry := range strings.Split(value, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		channel, adapter, ok := strings.Cut(entry, "=")
		channel = strings.ToUpper(strings.TrimSpace(channel))
		adapter = strings.ToUpper(strings.TrimSpace(adapter))
		if !ok || channel == "" || adapter == "" {
			return nil, fmt.Errorf("invalid refund provider mapping %q", entry)
		}
		if _, exists := result[channel]; exists {
			return nil, fmt.Errorf("duplicate refund provider mapping for %s", channel)
		}
		result[channel] = adapter
	}
	return result, nil
}

func RunProductionReleaseGate(ctx context.Context, db ReleaseGateQueryer, config ProductionReleaseGateConfig) (ProductionReleaseGateReport, error) {
	report := ProductionReleaseGateReport{CheckedAt: time.Now().UTC(), Passed: true}
	addCheck := func(name string, passed bool, detail string) {
		report.Checks = append(report.Checks, ProductionReleaseGateCheck{Name: name, Passed: passed, Detail: detail})
		if !passed {
			report.Passed = false
		}
	}
	if db == nil {
		addCheck("database_available", false, "database query interface is nil")
		return report, ErrProductionReleaseGateFailed
	}
	addCheck("environment_is_production", strings.EqualFold(config.Environment, "production"), "release gate must use production configuration namespace")
	addCheck("test_configuration_isolated", len(config.TestConfigurationKeys) == 0, fmt.Sprintf("test configuration keys present=%d", len(config.TestConfigurationKeys)))
	schedulersClosed := !config.Runtime.RefundRetrySchedulerEnabled && !config.Runtime.RefundVerificationEnabled && !config.Runtime.RewardReleaseSchedulerEnabled
	addCheck("all_schedulers_disabled", schedulersClosed, "refund retry, refund verification and reward release must all be false")
	addCheck("manual_refund_auto_approval_disabled", !config.Runtime.ManualRefundAutoApproval, "manual refund auto approval must be false")
	addCheck("provider_mappings_present", len(config.ProviderMappings) > 0, fmt.Sprintf("configured channels=%d", len(config.ProviderMappings)))
	addCheck("wechat_virtual_manual_only", config.ProviderMappings["WECHAT_VIRTUAL"] == "MANUAL", "WECHAT_VIRTUAL must map to MANUAL")
	addCheck("financial_accounts_distinct",
		config.FinancialSubmitterID != "" && config.FinancialApproverID != "" && config.FinancialSubmitterID != config.FinancialApproverID,
		"financial submitter and approver must be non-empty distinct accounts")

	signatures, err := readOperationCenterMigrationSignatures(ctx, db)
	if err != nil {
		addCheck("migrations_089_096", false, err.Error())
	} else {
		addCheck("migrations_089_096", signatures.AllApplied(), signatures.String())
	}

	var rolloutTotal, shadowCount, unsafeSwitches, whitelistEntries int64
	err = db.QueryRowContext(ctx, `
		SELECT count(*),
		       count(*) FILTER (WHERE mode='SHADOW'),
		       count(*) FILTER (WHERE real_switch_enabled OR percentage_rollout_enabled OR canary_basis_points<>0),
		       coalesce(sum(jsonb_array_length(allow_tenant_ids)+jsonb_array_length(allow_user_ids)+
		                    jsonb_array_length(allow_order_ids)+jsonb_array_length(allow_plan_ids)),0)
		FROM xz_channel_rollout_configs
	`).Scan(&rolloutTotal, &shadowCount, &unsafeSwitches, &whitelistEntries)
	if err != nil {
		addCheck("rollout_safe_defaults", false, err.Error())
	} else {
		addCheck("rollout_safe_defaults", rolloutTotal > 0 && shadowCount == rolloutTotal && unsafeSwitches == 0 && whitelistEntries == 0,
			fmt.Sprintf("rows=%d shadow=%d unsafe=%d whitelist_entries=%d", rolloutTotal, shadowCount, unsafeSwitches, whitelistEntries))
	}

	var publishedRules, operationCenterPlans, fullOnlyRules int64
	err = db.QueryRowContext(ctx, `SELECT count(*) FROM xz_commercial_rule_sets WHERE status='PUBLISHED'`).Scan(&publishedRules)
	if err != nil {
		addCheck("published_rule_set_exists", false, err.Error())
	} else {
		addCheck("published_rule_set_exists", publishedRules > 0, fmt.Sprintf("published rule sets=%d", publishedRules))
	}
	err = db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM xz_commercial_plan_versions plan
		JOIN xz_commercial_rule_sets rules ON rules.id=plan.rule_set_id AND rules.status='PUBLISHED'
		WHERE plan.identity_type='OPERATION_CENTER' AND plan.price_cents>0
		  AND coalesce(plan.currency,'')<>'' AND coalesce(plan.config->>'rbacRole','')<>''
		  AND plan.config->>'scenarioCode'='OPERATION_CENTER_SERVICE'
	`).Scan(&operationCenterPlans)
	if err != nil {
		addCheck("operation_center_plan_complete", false, err.Error())
	} else {
		addCheck("operation_center_plan_complete", operationCenterPlans > 0, fmt.Sprintf("complete published plans=%d", operationCenterPlans))
	}
	err = db.QueryRowContext(ctx, `
		SELECT count(*) FROM xz_commercial_rule_sets
		WHERE status='PUBLISHED' AND config->>'operationCenterActiveRefundMode' LIKE 'FULL_ONLY%'
	`).Scan(&fullOnlyRules)
	if err != nil {
		addCheck("full_only_refund_policy", false, err.Error())
	} else {
		addCheck("full_only_refund_policy", fullOnlyRules > 0, fmt.Sprintf("published FULL_ONLY rule sets=%d", fullOnlyRules))
	}

	var submitterFinance, approverFinance, privilegedReviewers int64
	err = db.QueryRowContext(ctx, `
		SELECT
		  count(*) FILTER (WHERE role.user_id=$1 AND role.role='FINANCE' AND upper(role.status)='ACTIVE'),
		  count(*) FILTER (WHERE role.user_id=$2 AND role.role='FINANCE' AND upper(role.status)='ACTIVE'),
		  count(*) FILTER (
		    WHERE role.user_id IN ($1,$2) AND upper(role.status)='ACTIVE'
		      AND (role.role='SUPER_ADMIN' OR EXISTS (
		        SELECT 1 FROM xz_role_permissions permission
		        WHERE permission.role=role.role AND permission.permission='channel:operation-center:review'
		      ))
		  )
		FROM xz_user_roles role
		JOIN xz_users users ON users.id=role.user_id AND upper(users.status)='ACTIVE'
		WHERE role.user_id IN ($1,$2)
	`, config.FinancialSubmitterID, config.FinancialApproverID).Scan(&submitterFinance, &approverFinance, &privilegedReviewers)
	if err != nil {
		addCheck("financial_permission_separation", false, err.Error())
	} else {
		addCheck("financial_permission_separation", submitterFinance > 0 && approverFinance > 0 && privilegedReviewers == 0,
			fmt.Sprintf("submitter_finance=%d approver_finance=%d review_privilege_rows=%d", submitterFinance, approverFinance, privilegedReviewers))
	}

	collectProductionReleaseMetrics(ctx, db, &report, addCheck)
	if !report.Passed {
		return report, ErrProductionReleaseGateFailed
	}
	return report, nil
}

func collectProductionReleaseMetrics(ctx context.Context, db ReleaseGateQueryer, report *ProductionReleaseGateReport, addCheck func(string, bool, string)) {
	type scalarMetric struct {
		name, unit, query string
	}
	scalars := []scalarMetric{
		{"review_required_orders", "count", `SELECT count(*) FROM xz_operation_center_service_orders WHERE status='REVIEW_REQUIRED'`},
		{"referral_reward_frozen_cents", "cents", `SELECT coalesce(sum(amount_cents),0) FROM xz_referral_rewards WHERE record_type='REWARD' AND status='FROZEN'`},
		{"referral_reward_released_cents", "cents", `SELECT coalesce(sum(amount_cents),0) FROM xz_referral_rewards WHERE record_type='REWARD' AND status IN ('AVAILABLE','SETTLED')`},
		{"recoverable_cents", "cents", `SELECT coalesce(sum(recoverable_cents),0) FROM xz_commission_wallet_accounts`},
		{"expired_refund_leases", "count", `SELECT count(*) FROM xz_operation_center_refund_tasks WHERE lease_expires_at<clock_timestamp() AND refund_status IN ('PROVIDER_PENDING','REFUND_RETRYABLE','UNKNOWN_VERIFYING')`},
		{"expired_reward_release_leases", "count", `SELECT count(*) FROM xz_referral_reward_release_tasks WHERE lease_expires_at<clock_timestamp() AND release_status IN ('PENDING','PROCESSING','FAILED')`},
		{"state_invariant_failures", "count", `SELECT count(*) FROM xz_operation_center_refund_tasks WHERE failure_class IN ('INVARIANT','DATA_INVARIANT','CONSTRAINT')`},
	}
	for _, metric := range scalars {
		var value int64
		if err := db.QueryRowContext(ctx, metric.query).Scan(&value); err != nil {
			addCheck("metric_"+metric.name, false, err.Error())
			continue
		}
		report.Metrics = append(report.Metrics, ProductionReleaseGateMetric{Name: metric.name, Value: value, Unit: metric.unit})
	}
	grouped := []struct {
		name, query string
	}{
		{"review_decisions", `SELECT decision,count(*) FROM xz_operation_center_review_events GROUP BY decision ORDER BY decision`},
		{"refund_statuses", `SELECT refund_status,count(*) FROM xz_operation_center_refund_tasks GROUP BY refund_status ORDER BY refund_status`},
		{"provider_results", `SELECT coalesce(provider_outcome,'NONE'),count(*) FROM xz_operation_center_refund_tasks GROUP BY coalesce(provider_outcome,'NONE') ORDER BY 1`},
	}
	for _, metric := range grouped {
		rows, err := db.QueryContext(ctx, metric.query)
		if err != nil {
			addCheck("metric_"+metric.name, false, err.Error())
			continue
		}
		for rows.Next() {
			var label string
			var value int64
			if err := rows.Scan(&label, &value); err != nil {
				addCheck("metric_"+metric.name, false, err.Error())
				break
			}
			report.Metrics = append(report.Metrics, ProductionReleaseGateMetric{
				Name: metric.name, Value: value, Unit: "count", Labels: map[string]string{"status": label},
			})
		}
		if err := rows.Err(); err != nil {
			addCheck("metric_"+metric.name, false, err.Error())
		}
		_ = rows.Close()
	}
	sort.Slice(report.Metrics, func(i, j int) bool {
		left, _ := json.Marshal(report.Metrics[i].Labels)
		right, _ := json.Marshal(report.Metrics[j].Labels)
		return report.Metrics[i].Name+string(left) < report.Metrics[j].Name+string(right)
	})
}

type operationCenterMigrationSignatures struct {
	Migration089, Migration090, Migration091, Migration092 bool
	Migration093, Migration094, Migration095, Migration096 bool
}

func (signatures operationCenterMigrationSignatures) AllApplied() bool {
	return signatures.Migration089 && signatures.Migration090 && signatures.Migration091 && signatures.Migration092 &&
		signatures.Migration093 && signatures.Migration094 && signatures.Migration095 && signatures.Migration096
}

func (signatures operationCenterMigrationSignatures) NoneApplied() bool {
	return !signatures.Migration089 && !signatures.Migration090 && !signatures.Migration091 && !signatures.Migration092 &&
		!signatures.Migration093 && !signatures.Migration094 && !signatures.Migration095 && !signatures.Migration096
}

func (signatures operationCenterMigrationSignatures) String() string {
	return fmt.Sprintf("089=%t 090=%t 091=%t 092=%t 093=%t 094=%t 095=%t 096=%t",
		signatures.Migration089, signatures.Migration090, signatures.Migration091, signatures.Migration092,
		signatures.Migration093, signatures.Migration094, signatures.Migration095, signatures.Migration096)
}

func readOperationCenterMigrationSignatures(ctx context.Context, db ReleaseGateQueryer) (operationCenterMigrationSignatures, error) {
	var result operationCenterMigrationSignatures
	err := db.QueryRowContext(ctx, `
		SELECT
		  EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='xz_operation_center_service_orders' AND column_name='refund_status'),
		  to_regclass('public.xz_referral_eligibilities') IS NOT NULL,
		  EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='xz_referral_rewards' AND column_name='referral_eligibility_id'),
		  EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='xz_commission_wallet_ledger' AND column_name='referral_release_task_id'),
		  EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='xz_referral_rewards' AND column_name='reversal_amount_cents'),
		  EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='xz_operation_center_refund_tasks' AND column_name='provider_refunded_at'),
		  EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='xz_operation_center_refund_tasks' AND column_name='verification_attempt_count'),
		  to_regclass('public.xz_operation_center_refund_request_events') IS NOT NULL
	`).Scan(&result.Migration089, &result.Migration090, &result.Migration091, &result.Migration092,
		&result.Migration093, &result.Migration094, &result.Migration095, &result.Migration096)
	return result, err
}
