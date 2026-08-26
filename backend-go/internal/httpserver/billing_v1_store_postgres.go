package httpserver

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

type billingRowScanner interface {
	Scan(...any) error
}

const billingRuleVersionSelect = `
	select id, rule_key, coalesce(legacy_rule_id,''), model_name, model_code, module_code,
		billing_unit, base_price_points, minimum_charge_points, parameter_rules, rule_source,
		coalesce(tenant_id,''), coalesce(plan_id,''), version, status, effective_from, effective_to,
		validation_result, coalesce(created_by,''), created_at, updated_at, published_at
	from xz_billing_rule_versions`

func scanBillingRuleVersion(scanner billingRowScanner) (billingRuleVersion, error) {
	var item billingRuleVersion
	var parameterRules, validationResult []byte
	var effectiveFrom, effectiveTo, publishedAt sql.NullTime
	var createdAt, updatedAt time.Time
	err := scanner.Scan(
		&item.ID, &item.RuleKey, &item.LegacyRuleID, &item.ModelName, &item.ModelCode, &item.ModuleCode,
		&item.BillingUnit, &item.BasePrice, &item.MinimumCharge, &parameterRules, &item.RuleSource,
		&item.TenantID, &item.PlanID, &item.Version, &item.Status, &effectiveFrom, &effectiveTo,
		&validationResult, &item.CreatedBy, &createdAt, &updatedAt, &publishedAt,
	)
	if err != nil {
		return item, err
	}
	item.ParameterRules = map[string]any{}
	_ = json.Unmarshal(parameterRules, &item.ParameterRules)
	item.ValidationResult = billingRuleValidationResult{Issues: []billingRuleValidationIssue{}}
	_ = json.Unmarshal(validationResult, &item.ValidationResult)
	item.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
	item.UpdatedAt = updatedAt.UTC().Format(time.RFC3339Nano)
	if effectiveFrom.Valid {
		item.EffectiveFrom = effectiveFrom.Time.UTC().Format(time.RFC3339Nano)
	}
	if effectiveTo.Valid {
		item.EffectiveTo = effectiveTo.Time.UTC().Format(time.RFC3339Nano)
	}
	if publishedAt.Valid {
		item.PublishedAt = publishedAt.Time.UTC().Format(time.RFC3339Nano)
	}
	return item, nil
}

func (s *postgresStore) listBillingRuleVersionsContext(ctx context.Context) ([]billingRuleVersion, error) {
	rows, err := s.db.QueryContext(ctx, billingRuleVersionSelect+` order by rule_key, version desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []billingRuleVersion{}
	for rows.Next() {
		item, err := scanBillingRuleVersion(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) ListBillingRuleVersions() ([]billingRuleVersion, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return nil, err
	}
	return s.listBillingRuleVersionsContext(ctx)
}

func (s *postgresStore) GetBillingRuleVersion(id string) (billingRuleVersion, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return billingRuleVersion{}, err
	}
	return scanBillingRuleVersion(s.db.QueryRowContext(ctx, billingRuleVersionSelect+` where id = $1`, id))
}

func (s *postgresStore) ValidateBillingRuleVersion(id string) (billingRuleValidationResult, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return billingRuleValidationResult{}, err
	}
	item, err := scanBillingRuleVersion(s.db.QueryRowContext(ctx, billingRuleVersionSelect+` where id = $1`, id))
	if err != nil {
		return billingRuleValidationResult{}, err
	}
	data, err := s.AdminData()
	if err != nil {
		return billingRuleValidationResult{}, err
	}
	data.BillingRuleVersions, err = s.listBillingRuleVersionsContext(ctx)
	if err != nil {
		return billingRuleValidationResult{}, err
	}
	data.ProviderCosts, err = s.listProviderCostsContext(ctx)
	if err != nil {
		return billingRuleValidationResult{}, err
	}
	result := validateBillingRuleVersionData(item, data)
	payload, _ := json.Marshal(result)
	_, err = s.db.ExecContext(ctx, `update xz_billing_rule_versions set validation_result=$2::jsonb, updated_at=now() where id=$1`, id, payload)
	return result, err
}

func (s *postgresStore) PublishBillingRuleVersion(id string) (billingRuleVersion, error) {
	result, err := s.ValidateBillingRuleVersion(id)
	if err != nil {
		return billingRuleVersion{}, err
	}
	if !result.Valid {
		return billingRuleVersion{}, errors.New("billing rule validation failed")
	}
	ctx, cancel := s.withTimeout()
	defer cancel()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return billingRuleVersion{}, err
	}
	defer func() { _ = tx.Rollback() }()
	item, err := scanBillingRuleVersion(tx.QueryRowContext(ctx, billingRuleVersionSelect+` where id=$1 for update`, id))
	if err != nil {
		return billingRuleVersion{}, err
	}
	if upperTrim(item.Status) != "DRAFT" {
		return billingRuleVersion{}, errors.New("only draft billing rules can be published")
	}
	if _, err := tx.ExecContext(ctx, `
		update xz_billing_rule_versions
		set status='ARCHIVED', effective_to=coalesce(effective_to, now()), updated_at=now()
		where rule_key=$1 and id<>$2 and status='PUBLISHED'
	`, item.RuleKey, item.ID); err != nil {
		return billingRuleVersion{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		update xz_billing_rule_versions
		set status='PUBLISHED', effective_from=coalesce(effective_from,now()), published_at=now(), updated_at=now()
		where id=$1
	`, item.ID); err != nil {
		return billingRuleVersion{}, err
	}
	if err := tx.Commit(); err != nil {
		return billingRuleVersion{}, err
	}
	return s.GetBillingRuleVersion(id)
}

func (s *postgresStore) createBillingRuleDraft(id string, req adminBillingRuleMutation) (billingRuleVersion, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return billingRuleVersion{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return billingRuleVersion{}, err
	}
	defer func() { _ = tx.Rollback() }()
	row := tx.QueryRowContext(ctx, billingRuleVersionSelect+`
		where id=$1 or rule_key=$1 or legacy_rule_id=$1
		order by version desc limit 1 for update`, id)
	source, err := scanBillingRuleVersion(row)
	if err != nil {
		return billingRuleVersion{}, err
	}
	var nextVersion int
	if err := tx.QueryRowContext(ctx, `select coalesce(max(version),0)+1 from xz_billing_rule_versions where rule_key=$1`, source.RuleKey).Scan(&nextVersion); err != nil {
		return billingRuleVersion{}, err
	}
	draft := source
	draft.ID = fmt.Sprintf("brv_%s_v%d", safeID(source.RuleKey), nextVersion)
	draft.Version = nextVersion
	draft.Status = "DRAFT"
	if upperTrim(draft.RuleSource) == "CODE_DEFAULT" {
		draft.RuleSource = "DATABASE"
	}
	draft.EffectiveFrom, draft.EffectiveTo, draft.PublishedAt = "", "", ""
	if req.BillingType != "" {
		draft.BillingUnit = billingUnitFromLegacy(req.BillingType)
	}
	if req.BasePrice > 0 {
		draft.BasePrice = req.BasePrice
	}
	if req.MinimumCharge >= 0 {
		draft.MinimumCharge = req.MinimumCharge
	}
	if req.ParameterMultiplier != nil {
		draft.ParameterRules = cloneAnyMap(req.ParameterMultiplier)
	}
	parameterRules, _ := json.Marshal(draft.ParameterRules)
	validation := []byte(`{"valid":false,"issues":[]}`)
	if _, err := tx.ExecContext(ctx, `
		insert into xz_billing_rule_versions(
			id,rule_key,legacy_rule_id,model_name,model_code,module_code,billing_unit,base_price_points,
			minimum_charge_points,parameter_rules,rule_source,tenant_id,plan_id,version,status,
			validation_result,created_by,created_at,updated_at
		) values($1,$2,nullif($3,''),$4,$5,$6,$7,$8,$9,$10::jsonb,$11,nullif($12,''),nullif($13,''),$14,'DRAFT',$15::jsonb,nullif($16,''),now(),now())
	`, draft.ID, draft.RuleKey, draft.LegacyRuleID, draft.ModelName, draft.ModelCode, draft.ModuleCode,
		draft.BillingUnit, draft.BasePrice, draft.MinimumCharge, parameterRules, draft.RuleSource,
		draft.TenantID, draft.PlanID, draft.Version, validation, draft.CreatedBy); err != nil {
		return billingRuleVersion{}, err
	}
	if err := tx.Commit(); err != nil {
		return billingRuleVersion{}, err
	}
	return s.GetBillingRuleVersion(draft.ID)
}

const providerCostSelect = `
	select id,provider,channel,platform_model_code,upstream_model_name,billing_unit,parameter_range,
		unit_cost,currency,effective_from,effective_to,status,created_at,updated_at
	from xz_provider_costs`

func scanProviderCost(scanner billingRowScanner) (providerCost, error) {
	var item providerCost
	var parameterRange []byte
	var effectiveFrom, createdAt, updatedAt time.Time
	var effectiveTo sql.NullTime
	err := scanner.Scan(&item.ID, &item.Provider, &item.Channel, &item.PlatformModelCode, &item.UpstreamModelName,
		&item.BillingUnit, &parameterRange, &item.UnitCost, &item.Currency, &effectiveFrom, &effectiveTo,
		&item.Status, &createdAt, &updatedAt)
	if err != nil {
		return item, err
	}
	item.ParameterRange = map[string]any{}
	_ = json.Unmarshal(parameterRange, &item.ParameterRange)
	item.EffectiveFrom = effectiveFrom.UTC().Format(time.RFC3339Nano)
	item.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
	item.UpdatedAt = updatedAt.UTC().Format(time.RFC3339Nano)
	if effectiveTo.Valid {
		item.EffectiveTo = effectiveTo.Time.UTC().Format(time.RFC3339Nano)
	}
	return item, nil
}

func (s *postgresStore) listProviderCostsContext(ctx context.Context) ([]providerCost, error) {
	rows, err := s.db.QueryContext(ctx, providerCostSelect+` order by updated_at desc,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []providerCost{}
	for rows.Next() {
		item, err := scanProviderCost(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) ListProviderCosts() ([]providerCost, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return nil, err
	}
	return s.listProviderCostsContext(ctx)
}

func (s *postgresStore) UpdateProviderCost(id string, req providerCostMutation) (providerCost, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return providerCost{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return providerCost{}, err
	}
	defer func() { _ = tx.Rollback() }()
	item, err := scanProviderCost(tx.QueryRowContext(ctx, providerCostSelect+` where id=$1 for update`, id))
	if err != nil {
		return providerCost{}, err
	}
	applyProviderCostMutation(&item, req)
	if err := validateProviderCost(item); err != nil {
		return providerCost{}, err
	}
	parameterRange, _ := json.Marshal(item.ParameterRange)
	var effectiveTo any
	if item.EffectiveTo != "" {
		effectiveTo = item.EffectiveTo
	}
	if _, err := tx.ExecContext(ctx, `
		update xz_provider_costs set provider=$2,channel=$3,platform_model_code=$4,upstream_model_name=$5,
			billing_unit=$6,parameter_range=$7::jsonb,unit_cost=$8,currency=$9,effective_from=$10,
			effective_to=$11,status=$12,updated_at=now() where id=$1
	`, id, item.Provider, item.Channel, item.PlatformModelCode, item.UpstreamModelName, item.BillingUnit,
		parameterRange, item.UnitCost, item.Currency, item.EffectiveFrom, effectiveTo, item.Status); err != nil {
		return providerCost{}, err
	}
	if err := tx.Commit(); err != nil {
		return providerCost{}, err
	}
	return scanProviderCost(s.db.QueryRowContext(ctx, providerCostSelect+` where id=$1`, id))
}

func (s *postgresStore) ListBillingLifecycleEvents() ([]billingLifecycleEvent, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	rows, err := s.db.QueryContext(ctx, `
		select id,task_id,coalesce(user_id,''),coalesce(tenant_id,''),coalesce(model_code,''),event_type,
			billing_status,points,coalesce(rule_version_id,''),coalesce(provider_channel,''),idempotency_key,metadata,created_at
		from xz_billing_lifecycle_events order by created_at desc,id desc limit 1000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []billingLifecycleEvent{}
	for rows.Next() {
		var item billingLifecycleEvent
		var metadata []byte
		var createdAt time.Time
		if err := rows.Scan(&item.ID, &item.TaskID, &item.UserID, &item.TenantID, &item.ModelCode, &item.EventType, &item.BillingStatus, &item.Points, &item.RuleVersionID, &item.ProviderChannel, &item.IdempotencyKey, &metadata, &createdAt); err != nil {
			return nil, err
		}
		item.Metadata = map[string]any{}
		_ = json.Unmarshal(metadata, &item.Metadata)
		item.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) ListWalletLedger() ([]walletLedgerEntry, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	rows, err := s.db.QueryContext(ctx, `
		select id,account_id,coalesce(user_id,''),coalesce(tenant_id,''),coalesce(task_id,''),coalesce(billing_event_id,''),
			entry_type,points,available_before,available_after,frozen_before,frozen_after,idempotency_key,
			reference_type,reference_id,remark,metadata,created_at
		from xz_wallet_ledger order by created_at desc,id desc limit 2000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []walletLedgerEntry{}
	for rows.Next() {
		var item walletLedgerEntry
		var metadata []byte
		var createdAt time.Time
		if err := rows.Scan(&item.ID, &item.AccountID, &item.UserID, &item.TenantID, &item.TaskID, &item.BillingEventID, &item.EntryType, &item.Points, &item.AvailableBefore, &item.AvailableAfter, &item.FrozenBefore, &item.FrozenAfter, &item.IdempotencyKey, &item.ReferenceType, &item.ReferenceID, &item.Remark, &metadata, &createdAt); err != nil {
			return nil, err
		}
		item.Metadata = map[string]any{}
		_ = json.Unmarshal(metadata, &item.Metadata)
		item.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) ListBillingReconciliation() ([]billingReconciliationItem, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	rows, err := s.db.QueryContext(ctx, `
		select raw,coalesce(client_request_id,''),task_status,billing_status,coalesce(billing_rule_version_id,''),
			quoted_points,reserved_points,captured_points,released_points,refunded_points,supplier_cost,estimated_margin,
			coalesce(provider_channel,'') from xz_generation_tasks order by created_at desc,id desc limit 2000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks := []generationTask{}
	for rows.Next() {
		var task generationTask
		if err := rows.Scan(rawScanner(&task), &task.ClientRequestID, &task.TaskStatus, &task.BillingStatus, &task.BillingRuleVersionID, &task.QuotedPoints, &task.ReservedPoints, &task.CapturedPoints, &task.ReleasedPoints, &task.RefundedPoints, &task.SupplierCost, &task.EstimatedMargin, &task.ProviderChannel); err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	events, err := s.ListBillingLifecycleEvents()
	if err != nil {
		return nil, err
	}
	ledger, err := s.ListWalletLedger()
	if err != nil {
		return nil, err
	}
	costs, err := s.listProviderCostsContext(ctx)
	if err != nil {
		return nil, err
	}
	return buildBillingReconciliation(tasks, events, ledger, costs), nil
}

func deterministicBillingID(prefix, key string) string {
	digest := sha256.Sum256([]byte(key))
	return prefix + "_" + hex.EncodeToString(digest[:12])
}

func billingStatusForEvent(eventType string) string {
	switch upperTrim(eventType) {
	case "QUOTE":
		return billingStatusQuoted
	case "RESERVE":
		return billingStatusReserved
	case "CAPTURE":
		return billingStatusCaptured
	case "RELEASE":
		return billingStatusReleased
	case "REFUND":
		return billingStatusRefunded
	default:
		return billingStatusBillingFailed
	}
}

func insertBillingLifecycleEventV1(ctx context.Context, tx *sql.Tx, task generationTask, eventType string, points float64, metadata map[string]any) (billingLifecycleEvent, error) {
	idempotencyKey := task.ID + ":" + upperTrim(eventType)
	item := billingLifecycleEvent{
		ID: deterministicBillingID("ble", idempotencyKey), TaskID: task.ID, UserID: task.UserID, TenantID: task.TenantID,
		ModelCode: task.Model, EventType: upperTrim(eventType), BillingStatus: billingStatusForEvent(eventType), Points: points,
		RuleVersionID: task.BillingRuleVersionID, ProviderChannel: task.ProviderChannel, IdempotencyKey: idempotencyKey,
		Metadata: cloneAnyMap(metadata), CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	payload, _ := json.Marshal(item.Metadata)
	_, err := tx.ExecContext(ctx, `
		insert into xz_billing_lifecycle_events(id,task_id,user_id,tenant_id,model_code,event_type,billing_status,points,rule_version_id,provider_channel,idempotency_key,metadata,created_at)
		values($1,$2,nullif($3,''),nullif($4,''),nullif($5,''),$6,$7,$8,nullif($9,''),nullif($10,''),$11,$12::jsonb,now())
		on conflict(idempotency_key) do nothing
	`, item.ID, item.TaskID, item.UserID, item.TenantID, item.ModelCode, item.EventType, item.BillingStatus, item.Points, item.RuleVersionID, item.ProviderChannel, item.IdempotencyKey, payload)
	return item, err
}

func insertWalletLedgerEntryV1(ctx context.Context, tx *sql.Tx, item walletLedgerEntry) (walletLedgerEntry, error) {
	item.EntryType = upperTrim(item.EntryType)
	item.IdempotencyKey = firstNonEmptyString(item.IdempotencyKey, item.TaskID+":"+item.EntryType)
	item.ID = firstNonEmptyString(item.ID, deterministicBillingID("wle", item.IdempotencyKey))
	item.Metadata = cloneAnyMap(item.Metadata)
	payload, _ := json.Marshal(item.Metadata)
	_, err := tx.ExecContext(ctx, `
		insert into xz_wallet_ledger(id,account_id,user_id,tenant_id,task_id,billing_event_id,entry_type,points,available_before,available_after,frozen_before,frozen_after,idempotency_key,reference_type,reference_id,remark,metadata,created_at)
		values($1,$2,nullif($3,''),nullif($4,''),nullif($5,''),nullif($6,''),$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17::jsonb,now())
		on conflict(idempotency_key) do nothing
	`, item.ID, item.AccountID, item.UserID, item.TenantID, item.TaskID, item.BillingEventID, item.EntryType, item.Points, item.AvailableBefore, item.AvailableAfter, item.FrozenBefore, item.FrozenAfter, item.IdempotencyKey, item.ReferenceType, item.ReferenceID, item.Remark, payload)
	return item, err
}

func insertAccountBalanceLedgerV1(ctx context.Context, tx *sql.Tx, account adminPointAccount, entryType string, points int, availableBefore int, availableAfter int, referenceType string, referenceID string, remark string) error {
	if points <= 0 || availableBefore == availableAfter {
		return nil
	}
	idempotencyKey := strings.Join([]string{upperTrim(referenceType), strings.TrimSpace(referenceID), upperTrim(entryType)}, ":")
	if referenceID == "" {
		idempotencyKey = strings.Join([]string{account.UserID, upperTrim(entryType), fmt.Sprint(availableBefore), fmt.Sprint(availableAfter), time.Now().UTC().Format(time.RFC3339Nano)}, ":")
	}
	_, err := insertWalletLedgerEntryV1(ctx, tx, walletLedgerEntry{
		AccountID:       account.ID,
		UserID:          account.UserID,
		EntryType:       entryType,
		Points:          float64(points),
		AvailableBefore: float64(availableBefore),
		AvailableAfter:  float64(availableAfter),
		FrozenBefore:    float64(account.Frozen),
		FrozenAfter:     float64(account.Frozen),
		IdempotencyKey:  idempotencyKey,
		ReferenceType:   referenceType,
		ReferenceID:     referenceID,
		Remark:          remark,
		Metadata:        map[string]any{"direction": map[bool]string{true: "INCREASE", false: "DECREASE"}[availableAfter > availableBefore]},
	})
	return err
}

func applyPersonalWalletEntryV1(ctx context.Context, tx *sql.Tx, task generationTask, account adminPointAccount, entryType string, points int, remark string) (adminPointAccount, walletLedgerEntry, error) {
	entryType = upperTrim(entryType)
	if points < 0 {
		return account, walletLedgerEntry{}, errors.New("wallet points cannot be negative")
	}
	next := account
	switch entryType {
	case "RESERVE":
		next.Available -= points
		next.Frozen += points
	case "CAPTURE":
		next.Frozen -= points
	case "RELEASE":
		next.Available += points
		next.Frozen -= points
	case "REFUND", "RECHARGE", "GRANT", "ADJUSTMENT":
		next.Available += points
	case "EXPIRE":
		next.Available -= points
	default:
		return account, walletLedgerEntry{}, errors.New("unsupported wallet entry type")
	}
	if next.Available < 0 || next.Frozen < 0 {
		return account, walletLedgerEntry{}, errors.New("wallet balance would become negative")
	}
	idempotencyKey := task.ID + ":" + entryType
	var exists bool
	if err := tx.QueryRowContext(ctx, `select exists(select 1 from xz_wallet_ledger where idempotency_key=$1)`, idempotencyKey).Scan(&exists); err != nil {
		return account, walletLedgerEntry{}, err
	}
	if exists {
		return account, walletLedgerEntry{}, nil
	}
	if _, err := tx.ExecContext(ctx, `update xz_point_accounts set available=$1::bigint,frozen=$2::bigint,raw=jsonb_set(jsonb_set(raw,'{available}',to_jsonb($1::bigint),true),'{frozen}',to_jsonb($2::bigint),true) where id=$3`, next.Available, next.Frozen, account.ID); err != nil {
		return account, walletLedgerEntry{}, err
	}
	if err := upsertUserWalletFromPointAccount(ctx, tx, next); err != nil {
		return account, walletLedgerEntry{}, err
	}
	entry := walletLedgerEntry{AccountID: account.ID, UserID: task.UserID, TenantID: task.TenantID, TaskID: task.ID, EntryType: entryType, Points: float64(points), AvailableBefore: float64(account.Available), AvailableAfter: float64(next.Available), FrozenBefore: float64(account.Frozen), FrozenAfter: float64(next.Frozen), IdempotencyKey: idempotencyKey, ReferenceType: "GENERATION_TASK", ReferenceID: task.ID, Remark: remark, Metadata: map[string]any{"modelCode": task.Model, "ruleVersionId": task.BillingRuleVersionID}}
	entry, err := insertWalletLedgerEntryV1(ctx, tx, entry)
	return next, entry, err
}

func supplierCostForTask(cost providerCost, task generationTask) float64 {
	req := createGenerationTaskRequest{Type: task.Type, Model: task.Model, Params: task.Params}
	quantity := billingQuantity(legacyBillingType(cost.BillingUnit), req)
	return math.Round(cost.UnitCost*quantity*1_000_000) / 1_000_000
}

func generationTaskByClientRequestTx(ctx context.Context, tx *sql.Tx, userID, clientRequestID string) (generationTask, bool, error) {
	clientRequestID = strings.TrimSpace(clientRequestID)
	if clientRequestID == "" {
		return generationTask{}, false, nil
	}
	if _, err := tx.ExecContext(ctx, `select pg_advisory_xact_lock(hashtext($1))`, userID+":"+clientRequestID); err != nil {
		return generationTask{}, false, err
	}
	var item generationTask
	err := tx.QueryRowContext(ctx, `select raw from xz_generation_tasks where user_id=$1 and client_request_id=$2 limit 1`, userID, clientRequestID).Scan(rawScanner(&item))
	if errors.Is(err, sql.ErrNoRows) {
		return generationTask{}, false, nil
	}
	return item, err == nil, err
}

func insertScopedWalletEntryV1(ctx context.Context, tx *sql.Tx, task generationTask, accountID, entryType string, points int, availableBefore, availableAfter, frozenBefore, frozenAfter int, remark string) (walletLedgerEntry, error) {
	return insertWalletLedgerEntryV1(ctx, tx, walletLedgerEntry{AccountID: firstNonEmptyString(accountID, task.BillingAccountID, task.UserID), UserID: task.UserID, TenantID: task.TenantID, TaskID: task.ID, EntryType: upperTrim(entryType), Points: float64(points), AvailableBefore: float64(availableBefore), AvailableAfter: float64(availableAfter), FrozenBefore: float64(frozenBefore), FrozenAfter: float64(frozenAfter), IdempotencyKey: task.ID + ":" + upperTrim(entryType), ReferenceType: "GENERATION_TASK", ReferenceID: task.ID, Remark: remark, Metadata: map[string]any{"modelCode": task.Model, "ruleVersionId": task.BillingRuleVersionID}})
}
