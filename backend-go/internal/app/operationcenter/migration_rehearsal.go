package operationcenter

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const MigrationRehearsalAcknowledgement = "NON_PRODUCTION_COPY"

type MigrationRehearsalOptions struct {
	Environment        string
	Acknowledgement    string
	BackupReference    string
	MigrationDirectory string
	AvailableDiskBytes int64
	Apply              bool
	LockWarningAfter   time.Duration
}

type MigrationRehearsalStep struct {
	Migration          string `json:"migration"`
	DurationMillis     int64  `json:"durationMillis"`
	WaitingLocksBefore int64  `json:"waitingLocksBefore"`
	WaitingLocksAfter  int64  `json:"waitingLocksAfter"`
	LockRisk           string `json:"lockRisk"`
}

type MigrationRelationSize struct {
	Relation string `json:"relation"`
	Bytes    int64  `json:"bytes"`
}

type OperationCenterMigrationRehearsalReport struct {
	StartedAt                time.Time                    `json:"startedAt"`
	FinishedAt               time.Time                    `json:"finishedAt"`
	DatabaseName             string                       `json:"databaseName"`
	InferredSchemaVersion    string                       `json:"inferredSchemaVersion"`
	BackupReference          string                       `json:"backupReference"`
	Passed                   bool                         `json:"passed"`
	Checks                   []ProductionReleaseGateCheck `json:"checks"`
	Migrations               []MigrationRehearsalStep     `json:"migrations"`
	RelationSizesBefore      []MigrationRelationSize      `json:"relationSizesBefore"`
	RelationSizesAfter       []MigrationRelationSize      `json:"relationSizesAfter"`
	RolloutFingerprintBefore string                       `json:"rolloutFingerprintBefore"`
	RolloutFingerprintAfter  string                       `json:"rolloutFingerprintAfter"`
	RuleFingerprintBefore    string                       `json:"ruleFingerprintBefore"`
	RuleFingerprintAfter     string                       `json:"ruleFingerprintAfter"`
	HistoricalOrdersBefore   int64                        `json:"historicalOrdersBefore"`
	HistoricalOrdersAfter    int64                        `json:"historicalOrdersAfter"`
}

func ValidateMigrationRehearsalOptions(options MigrationRehearsalOptions) error {
	if strings.EqualFold(strings.TrimSpace(options.Environment), "production") || strings.EqualFold(strings.TrimSpace(options.Environment), "prod") {
		return fmt.Errorf("migration rehearsal refuses production environment")
	}
	if strings.TrimSpace(options.Acknowledgement) != MigrationRehearsalAcknowledgement {
		return fmt.Errorf("migration rehearsal acknowledgement must be %s", MigrationRehearsalAcknowledgement)
	}
	if strings.TrimSpace(options.BackupReference) == "" {
		return fmt.Errorf("verified backup reference is required")
	}
	if strings.TrimSpace(options.MigrationDirectory) == "" {
		return fmt.Errorf("migration directory is required")
	}
	if options.AvailableDiskBytes <= 0 {
		return fmt.Errorf("available disk bytes must be supplied")
	}
	if !options.Apply {
		return fmt.Errorf("apply flag is required for the isolated rehearsal")
	}
	return nil
}

func RunOperationCenterMigrationRehearsal(ctx context.Context, db *sql.DB, options MigrationRehearsalOptions) (OperationCenterMigrationRehearsalReport, error) {
	report := OperationCenterMigrationRehearsalReport{
		StartedAt: time.Now().UTC(), BackupReference: strings.TrimSpace(options.BackupReference), Passed: true,
	}
	addCheck := func(name string, passed bool, detail string) {
		report.Checks = append(report.Checks, ProductionReleaseGateCheck{Name: name, Passed: passed, Detail: detail})
		if !passed {
			report.Passed = false
		}
	}
	if err := ValidateMigrationRehearsalOptions(options); err != nil {
		addCheck("rehearsal_options", false, err.Error())
		return report, err
	}
	if db == nil {
		err := fmt.Errorf("database is required")
		addCheck("database_available", false, err.Error())
		return report, err
	}
	if options.LockWarningAfter <= 0 {
		options.LockWarningAfter = 5 * time.Second
	}
	if err := db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&report.DatabaseName); err != nil {
		addCheck("database_identity", false, err.Error())
		return report, err
	}
	signatures, err := readOperationCenterMigrationSignatures(ctx, db)
	if err != nil {
		addCheck("pre_migration_signatures", false, err.Error())
		return report, err
	}
	report.InferredSchemaVersion = "088"
	addCheck("pre_migration_signatures", signatures.NoneApplied(), signatures.String())

	var databaseBytes int64
	if err := db.QueryRowContext(ctx, `SELECT pg_database_size(current_database())`).Scan(&databaseBytes); err != nil {
		addCheck("database_size", false, err.Error())
	} else {
		required := databaseBytes * 2
		addCheck("disk_space", options.AvailableDiskBytes >= required,
			fmt.Sprintf("database_bytes=%d available_bytes=%d required_bytes=%d", databaseBytes, options.AvailableDiskBytes, required))
	}
	var vectorVersion, pgcryptoVersion string
	err = db.QueryRowContext(ctx, `
		SELECT coalesce(max(extversion) FILTER (WHERE extname='vector'),''),
		       coalesce(max(extversion) FILTER (WHERE extname='pgcrypto'),'')
		FROM pg_extension WHERE extname IN ('vector','pgcrypto')
	`).Scan(&vectorVersion, &pgcryptoVersion)
	if err != nil {
		addCheck("required_extensions", false, err.Error())
	} else {
		addCheck("required_extensions", vectorVersion != "" && pgcryptoVersion != "",
			fmt.Sprintf("vector=%s pgcrypto=%s", vectorVersion, pgcryptoVersion))
	}
	var longTransactions, waitingLocks int64
	if err := db.QueryRowContext(ctx, `
		SELECT
		  count(*) FILTER (WHERE state<>'idle' AND xact_start<clock_timestamp()-interval '5 minutes'),
		  (SELECT count(*) FROM pg_locks WHERE NOT granted)
		FROM pg_stat_activity WHERE datname=current_database() AND pid<>pg_backend_pid()
	`).Scan(&longTransactions, &waitingLocks); err != nil {
		addCheck("transaction_and_lock_preflight", false, err.Error())
	} else {
		addCheck("transaction_and_lock_preflight", longTransactions == 0 && waitingLocks == 0,
			fmt.Sprintf("long_transactions=%d waiting_locks=%d", longTransactions, waitingLocks))
	}
	addCheck("backup_reference", report.BackupReference != "", "backup reference recorded")

	report.RolloutFingerprintBefore, err = migrationRolloutFingerprint(ctx, db)
	if err != nil {
		addCheck("rollout_fingerprint_before", false, err.Error())
	}
	report.RuleFingerprintBefore, err = migrationRuleFingerprint(ctx, db)
	if err != nil {
		addCheck("rule_fingerprint_before", false, err.Error())
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_orders`).Scan(&report.HistoricalOrdersBefore); err != nil {
		addCheck("historical_orders_before", false, err.Error())
	}
	report.RelationSizesBefore, _ = migrationRelationSizes(ctx, db)
	if !report.Passed {
		return report, fmt.Errorf("%w: pre-migration checks failed", ErrProductionReleaseGateFailed)
	}

	files, err := operationCenterMigrationFiles(options.MigrationDirectory)
	if err != nil {
		addCheck("migration_files", false, err.Error())
		return report, err
	}
	for _, file := range files {
		beforeLocks, lockErr := migrationWaitingLocks(ctx, db)
		if lockErr != nil {
			addCheck("lock_sample_"+filepath.Base(file), false, lockErr.Error())
			return report, lockErr
		}
		sqlBytes, readErr := os.ReadFile(file)
		if readErr != nil {
			addCheck("read_"+filepath.Base(file), false, readErr.Error())
			return report, readErr
		}
		started := time.Now()
		if _, execErr := db.ExecContext(ctx, string(sqlBytes)); execErr != nil {
			addCheck("apply_"+filepath.Base(file), false, execErr.Error())
			return report, execErr
		}
		duration := time.Since(started)
		afterLocks, lockErr := migrationWaitingLocks(ctx, db)
		if lockErr != nil {
			addCheck("lock_sample_"+filepath.Base(file), false, lockErr.Error())
			return report, lockErr
		}
		risk := "LOW"
		if duration >= options.LockWarningAfter || afterLocks > beforeLocks {
			risk = "REVIEW"
		}
		report.Migrations = append(report.Migrations, MigrationRehearsalStep{
			Migration: filepath.Base(file), DurationMillis: duration.Milliseconds(),
			WaitingLocksBefore: beforeLocks, WaitingLocksAfter: afterLocks, LockRisk: risk,
		})
	}

	signatures, err = readOperationCenterMigrationSignatures(ctx, db)
	if err != nil {
		addCheck("post_migration_signatures", false, err.Error())
	} else {
		addCheck("post_migration_signatures", signatures.AllApplied(), signatures.String())
	}
	report.RolloutFingerprintAfter, err = migrationRolloutFingerprint(ctx, db)
	if err != nil {
		addCheck("rollout_fingerprint_after", false, err.Error())
	}
	report.RuleFingerprintAfter, err = migrationRuleFingerprint(ctx, db)
	if err != nil {
		addCheck("rule_fingerprint_after", false, err.Error())
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_orders`).Scan(&report.HistoricalOrdersAfter); err != nil {
		addCheck("historical_orders_after", false, err.Error())
	}
	addCheck("rollout_configuration_unchanged", report.RolloutFingerprintBefore == report.RolloutFingerprintAfter,
		fmt.Sprintf("before=%s after=%s", report.RolloutFingerprintBefore, report.RolloutFingerprintAfter))
	addCheck("commercial_rules_unchanged", report.RuleFingerprintBefore == report.RuleFingerprintAfter,
		fmt.Sprintf("before=%s after=%s", report.RuleFingerprintBefore, report.RuleFingerprintAfter))
	addCheck("historical_orders_compatible", report.HistoricalOrdersBefore == report.HistoricalOrdersAfter,
		fmt.Sprintf("before=%d after=%d", report.HistoricalOrdersBefore, report.HistoricalOrdersAfter))

	var checks, foreignKeys, triggers, projectionBaseline int64
	err = db.QueryRowContext(ctx, `
		SELECT
		  count(*) FILTER (WHERE constraint_type='CHECK'),
		  count(*) FILTER (WHERE constraint_type='FOREIGN KEY'),
		  (SELECT count(*) FROM information_schema.triggers WHERE event_object_schema='public' AND
		    (event_object_table LIKE 'xz_operation_center_%' OR event_object_table LIKE 'xz_referral_%')),
		  CASE WHEN to_regclass('public.xz_billing_events') IS NOT NULL THEN 1 ELSE 0 END
		FROM information_schema.table_constraints
		WHERE table_schema='public' AND
		  (table_name LIKE 'xz_operation_center_%' OR table_name LIKE 'xz_referral_%' OR table_name LIKE 'xz_commission_wallet_%')
	`).Scan(&checks, &foreignKeys, &triggers, &projectionBaseline)
	if err != nil {
		addCheck("database_contracts", false, err.Error())
	} else {
		addCheck("database_contracts", checks > 0 && foreignKeys > 0 && triggers > 0 && projectionBaseline == 1,
			fmt.Sprintf("checks=%d foreign_keys=%d triggers=%d runtime_projection=%d", checks, foreignKeys, triggers, projectionBaseline))
	}
	report.RelationSizesAfter, _ = migrationRelationSizes(ctx, db)
	report.FinishedAt = time.Now().UTC()
	if !report.Passed {
		return report, ErrProductionReleaseGateFailed
	}
	return report, nil
}

func operationCenterMigrationFiles(directory string) ([]string, error) {
	files := make([]string, 0, 8)
	for number := 89; number <= 96; number++ {
		pattern := filepath.Join(directory, fmt.Sprintf("%03d-*.sql", number))
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, err
		}
		if len(matches) != 1 {
			return nil, fmt.Errorf("expected exactly one migration for %03d, found %d", number, len(matches))
		}
		files = append(files, matches[0])
	}
	sort.Strings(files)
	return files, nil
}

func migrationRolloutFingerprint(ctx context.Context, db *sql.DB) (string, error) {
	return migrationFingerprint(ctx, db, `
		SELECT md5(coalesce(string_agg(
		  id||'|'||tenant_id||'|'||mode||'|'||enabled::text||'|'||real_switch_enabled::text||'|'||
		  percentage_rollout_enabled::text||'|'||canary_basis_points::text||'|'||
		  allow_tenant_ids::text||'|'||allow_user_ids::text||'|'||allow_order_ids::text||'|'||allow_plan_ids::text,
		  E'\n' ORDER BY id),''))
		FROM xz_channel_rollout_configs
	`)
}

func migrationRuleFingerprint(ctx context.Context, db *sql.DB) (string, error) {
	return migrationFingerprint(ctx, db, `
		SELECT md5(coalesce(string_agg(id||'|'||status||'|'||version::text||'|'||config::text,E'\n' ORDER BY id),''))
		FROM xz_commercial_rule_sets
	`)
}

func migrationFingerprint(ctx context.Context, db *sql.DB, query string) (string, error) {
	var result string
	err := db.QueryRowContext(ctx, query).Scan(&result)
	return result, err
}

func migrationWaitingLocks(ctx context.Context, db *sql.DB) (int64, error) {
	var result int64
	err := db.QueryRowContext(ctx, `SELECT count(*) FROM pg_locks WHERE NOT granted`).Scan(&result)
	return result, err
}

func migrationRelationSizes(ctx context.Context, db *sql.DB) ([]MigrationRelationSize, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT c.relname,pg_total_relation_size(c.oid)
		FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
		WHERE n.nspname='public' AND c.relkind IN ('r','i') AND
		  (c.relname LIKE 'xz_operation_center_%' OR c.relname LIKE 'xz_referral_%' OR c.relname LIKE 'xz_commission_wallet_%')
		ORDER BY c.relname
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]MigrationRelationSize, 0)
	for rows.Next() {
		var item MigrationRelationSize
		if err := rows.Scan(&item.Relation, &item.Bytes); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func ParseAvailableDiskBytes(value string) (int64, error) {
	result, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || result <= 0 {
		return 0, fmt.Errorf("XIANZHI_REHEARSAL_AVAILABLE_DISK_BYTES must be a positive integer")
	}
	return result, nil
}
