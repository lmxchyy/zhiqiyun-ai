package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ReencryptStats tracks counts for scanned, migrated, skipped, conflicted, and failed records.
type ReencryptStats struct {
	Scanned    int `json:"scanned"`
	Migrated   int `json:"migrated"`
	Skipped    int `json:"skipped"`
	Conflicted int `json:"conflicted"`
	Failed     int `json:"failed"`
}

// Add aggregates stats from another ReencryptStats.
func (s *ReencryptStats) Add(other ReencryptStats) {
	s.Scanned += other.Scanned
	s.Migrated += other.Migrated
	s.Skipped += other.Skipped
	s.Conflicted += other.Conflicted
	s.Failed += other.Failed
}

// String returns a human-readable summary.
func (s ReencryptStats) String() string {
	return fmt.Sprintf("scanned=%d migrated=%d skipped=%d conflicted=%d failed=%d",
		s.Scanned, s.Migrated, s.Skipped, s.Conflicted, s.Failed)
}

// ReencryptOptions specifies execution parameters for re-encryption.
type ReencryptOptions struct {
	DryRun    bool
	BatchSize int
	Logger    io.Writer
}

// RowsIterator abstracts row scanning to allow unit-testable database mocking.
type RowsIterator interface {
	Next() bool
	Scan(dest ...any) error
	Close() error
	Err() error
}

// QueryExecutor defines the SQL execution interface needed for re-encryption.
type QueryExecutor interface {
	QueryContext(ctx context.Context, query string, args ...any) (RowsIterator, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type sqlDBExecutor struct {
	db *sql.DB
}

func (e *sqlDBExecutor) QueryContext(ctx context.Context, query string, args ...any) (RowsIterator, error) {
	rows, err := e.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (e *sqlDBExecutor) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return e.db.ExecContext(ctx, query, args...)
}

// WrapDB wraps a *sql.DB into a QueryExecutor.
func WrapDB(db *sql.DB) QueryExecutor {
	return &sqlDBExecutor{db: db}
}

func logMessage(w io.Writer, format string, args ...any) {
	if w != nil {
		_, _ = fmt.Fprintf(w, format+"\n", args...)
	}
}

// ReencryptSecretValue attempts to re-encrypt a single ciphertext value under cipher's active key.
// Rules:
// - Empty/whitespace ciphertext -> returns "", false, nil.
// - Already v2 under activeKeyID -> returns original, false, nil (skipped).
// - v1 or non-active v2 -> decrypts with dual-read, re-encrypts with active key -> returns new, true, nil.
// - Decryption failure -> returns error; NEVER overwrites or falls back.
func ReencryptSecretValue(cipher *SecretCipher, cipherText, aad string) (string, bool, error) {
	trimmed := strings.TrimSpace(cipherText)
	if trimmed == "" {
		return "", false, nil
	}
	if cipher == nil {
		return "", false, ErrSecretCipherRequired
	}

	meta, err := InspectEnvelope(trimmed)
	if err != nil {
		return "", false, fmt.Errorf("envelope inspection failed: %w", err)
	}

	// Idempotent check: if already encrypted under active key ID, skip
	if meta.Version == 2 && meta.KeyID == cipher.ActiveKeyID() {
		return trimmed, false, nil
	}

	// Decrypt using dual-read (supports v1 legacy key and v2 keyring)
	plaintext, err := cipher.Decrypt(trimmed, aad)
	if err != nil {
		return "", false, fmt.Errorf("decryption failed: %w", err)
	}

	// Re-encrypt under active key
	newCiphertext, err := cipher.Encrypt(plaintext, aad)
	if err != nil {
		return "", false, fmt.Errorf("re-encryption failed: %w", err)
	}

	return newCiphertext, true, nil
}

type storageConfigRow struct {
	id           string
	accessKey    string
	secretKey    string
	sessionToken string
}

// ReencryptStorageConfigs scans xz_storage_configs and re-encrypts credentials to the active key.
func ReencryptStorageConfigs(ctx context.Context, db QueryExecutor, cipher *SecretCipher, opts ReencryptOptions) (ReencryptStats, error) {
	stats := ReencryptStats{}
	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = 50
	}

	lastID := ""
	for {
		query := `SELECT id, coalesce(access_key_encrypted, ''), coalesce(secret_key_encrypted, ''), coalesce(session_token_encrypted, '')
			FROM xz_storage_configs
			WHERE id > $1
			ORDER BY id
			LIMIT $2`
		rows, err := db.QueryContext(ctx, query, lastID, batchSize)
		if err != nil {
			return stats, fmt.Errorf("query storage configs batch failed: %w", err)
		}

		batch := make([]storageConfigRow, 0, batchSize)
		for rows.Next() {
			var r storageConfigRow
			if err := rows.Scan(&r.id, &r.accessKey, &r.secretKey, &r.sessionToken); err != nil {
				_ = rows.Close()
				return stats, fmt.Errorf("scan storage config row failed: %w", err)
			}
			batch = append(batch, r)
		}
		_ = rows.Close()
		if err := rows.Err(); err != nil {
			return stats, fmt.Errorf("rows iteration failed: %w", err)
		}

		if len(batch) == 0 {
			break
		}

		for _, row := range batch {
			lastID = row.id
			stats.Scanned++

			newAK, akChanged, akErr := ReencryptSecretValue(cipher, row.accessKey, row.id)
			newSK, skChanged, skErr := ReencryptSecretValue(cipher, row.secretKey, row.id)
			newST, stChanged, stErr := ReencryptSecretValue(cipher, row.sessionToken, row.id)

			// If ANY field decryption fails, fail-closed: do NOT update the row
			if akErr != nil || skErr != nil || stErr != nil {
				stats.Failed++
				logMessage(opts.Logger, "[reencrypt] table=xz_storage_configs id=%s status=FAILED action=RETAIN_OLD error=\"decryption failed on one or more credentials\"", row.id)
				continue
			}

			// If none changed, all fields were already active v2 (or empty)
			if !akChanged && !skChanged && !stChanged {
				stats.Skipped++
				logMessage(opts.Logger, "[reencrypt] table=xz_storage_configs id=%s status=SKIPPED reason=ALREADY_ACTIVE", row.id)
				continue
			}

			// Simulated update in dry-run mode
			if opts.DryRun {
				stats.Migrated++
				logMessage(opts.Logger, "[reencrypt] table=xz_storage_configs id=%s status=MIGRATED mode=DRY_RUN", row.id)
				continue
			}

			// CAS Optimistic Concurrency update
			updateQuery := `UPDATE xz_storage_configs
				SET access_key_encrypted = $1,
				    secret_key_encrypted = $2,
				    session_token_encrypted = $3,
				    updated_at = now()
				WHERE id = $4
				  AND coalesce(access_key_encrypted, '') = $5
				  AND coalesce(secret_key_encrypted, '') = $6
				  AND coalesce(session_token_encrypted, '') = $7`

			res, err := db.ExecContext(ctx, updateQuery, newAK, newSK, newST, row.id, row.accessKey, row.secretKey, row.sessionToken)
			if err != nil {
				stats.Failed++
				logMessage(opts.Logger, "[reencrypt] table=xz_storage_configs id=%s status=FAILED action=DB_ERROR error=%q", row.id, err.Error())
				continue
			}

			rowsAffected, err := res.RowsAffected()
			if err != nil {
				stats.Failed++
				logMessage(opts.Logger, "[reencrypt] table=xz_storage_configs id=%s status=FAILED action=ROWS_AFFECTED_ERROR", row.id)
				continue
			}

			if rowsAffected == 0 {
				stats.Conflicted++
				logMessage(opts.Logger, "[reencrypt] table=xz_storage_configs id=%s status=CONFLICTED action=CAS_MISMATCH_SKIPPED", row.id)
			} else {
				stats.Migrated++
				logMessage(opts.Logger, "[reencrypt] table=xz_storage_configs id=%s status=MIGRATED mode=LIVE", row.id)
			}
		}

		if len(batch) < batchSize {
			break
		}
	}

	return stats, nil
}

type connectorRow struct {
	id                string
	appSecret         string
	verificationToken string
	encryptKey        string
}

// ReencryptEnterpriseConnectors scans enterprise_connectors and re-encrypts credentials to the active key.
func ReencryptEnterpriseConnectors(ctx context.Context, db QueryExecutor, cipher *SecretCipher, opts ReencryptOptions) (ReencryptStats, error) {
	stats := ReencryptStats{}
	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = 50
	}

	lastID := ""
	for {
		query := `SELECT id, coalesce(app_secret_encrypted, ''), coalesce(verification_token_encrypted, ''), coalesce(encrypt_key_encrypted, '')
			FROM enterprise_connectors
			WHERE id > $1
			ORDER BY id
			LIMIT $2`
		rows, err := db.QueryContext(ctx, query, lastID, batchSize)
		if err != nil {
			return stats, fmt.Errorf("query enterprise connectors batch failed: %w", err)
		}

		batch := make([]connectorRow, 0, batchSize)
		for rows.Next() {
			var r connectorRow
			if err := rows.Scan(&r.id, &r.appSecret, &r.verificationToken, &r.encryptKey); err != nil {
				_ = rows.Close()
				return stats, fmt.Errorf("scan enterprise connector row failed: %w", err)
			}
			batch = append(batch, r)
		}
		_ = rows.Close()
		if err := rows.Err(); err != nil {
			return stats, fmt.Errorf("rows iteration failed: %w", err)
		}

		if len(batch) == 0 {
			break
		}

		for _, row := range batch {
			lastID = row.id
			stats.Scanned++

			newSecret, secretChanged, secretErr := ReencryptSecretValue(cipher, row.appSecret, row.id+":app_secret")
			newVT, vtChanged, vtErr := ReencryptSecretValue(cipher, row.verificationToken, row.id+":verification_token")
			newEK, ekChanged, ekErr := ReencryptSecretValue(cipher, row.encryptKey, row.id+":encrypt_key")

			// If ANY field decryption fails, fail-closed: do NOT update the row
			if secretErr != nil || vtErr != nil || ekErr != nil {
				stats.Failed++
				logMessage(opts.Logger, "[reencrypt] table=enterprise_connectors id=%s status=FAILED action=RETAIN_OLD error=\"decryption failed on one or more credentials\"", row.id)
				continue
			}

			// If none changed, all fields were already active v2 (or empty)
			if !secretChanged && !vtChanged && !ekChanged {
				stats.Skipped++
				logMessage(opts.Logger, "[reencrypt] table=enterprise_connectors id=%s status=SKIPPED reason=ALREADY_ACTIVE", row.id)
				continue
			}

			// Simulated update in dry-run mode
			if opts.DryRun {
				stats.Migrated++
				logMessage(opts.Logger, "[reencrypt] table=enterprise_connectors id=%s status=MIGRATED mode=DRY_RUN", row.id)
				continue
			}

			// CAS Optimistic Concurrency update
			updateQuery := `UPDATE enterprise_connectors
				SET app_secret_encrypted = $1,
				    verification_token_encrypted = $2,
				    encrypt_key_encrypted = $3,
				    updated_at = now()
				WHERE id = $4
				  AND coalesce(app_secret_encrypted, '') = $5
				  AND coalesce(verification_token_encrypted, '') = $6
				  AND coalesce(encrypt_key_encrypted, '') = $7`

			res, err := db.ExecContext(ctx, updateQuery, newSecret, newVT, newEK, row.id, row.appSecret, row.verificationToken, row.encryptKey)
			if err != nil {
				stats.Failed++
				logMessage(opts.Logger, "[reencrypt] table=enterprise_connectors id=%s status=FAILED action=DB_ERROR error=%q", row.id, err.Error())
				continue
			}

			rowsAffected, err := res.RowsAffected()
			if err != nil {
				stats.Failed++
				logMessage(opts.Logger, "[reencrypt] table=enterprise_connectors id=%s status=FAILED action=ROWS_AFFECTED_ERROR", row.id)
				continue
			}

			if rowsAffected == 0 {
				stats.Conflicted++
				logMessage(opts.Logger, "[reencrypt] table=enterprise_connectors id=%s status=CONFLICTED action=CAS_MISMATCH_SKIPPED", row.id)
			} else {
				stats.Migrated++
				logMessage(opts.Logger, "[reencrypt] table=enterprise_connectors id=%s status=MIGRATED mode=LIVE", row.id)
			}
		}

		if len(batch) < batchSize {
			break
		}
	}

	return stats, nil
}

// ReencryptRunner manages executing re-encryption across storage configs and enterprise connectors.
type ReencryptRunner struct {
	DB              QueryExecutor
	StorageCipher   *SecretCipher
	ConnectorCipher *SecretCipher
	Options         ReencryptOptions
}

// Run executes re-encryption for the specified table target ("all", "storage", "connectors").
func (r *ReencryptRunner) Run(ctx context.Context, target string) (map[string]ReencryptStats, error) {
	results := make(map[string]ReencryptStats)
	target = strings.ToLower(strings.TrimSpace(target))
	if target == "" {
		target = "all"
	}

	if target == "all" || target == "storage" || target == "xz_storage_configs" {
		if r.StorageCipher == nil {
			return results, errors.New("storage cipher is not configured")
		}
		logMessage(r.Options.Logger, "[reencrypt-start] table=xz_storage_configs active_key_id=%s dry_run=%t batch_size=%d",
			r.StorageCipher.ActiveKeyID(), r.Options.DryRun, r.Options.BatchSize)
		st, err := ReencryptStorageConfigs(ctx, r.DB, r.StorageCipher, r.Options)
		results["xz_storage_configs"] = st
		logMessage(r.Options.Logger, "[reencrypt-finish] table=xz_storage_configs %s", st)
		if err != nil {
			return results, err
		}
	}

	if target == "all" || target == "connectors" || target == "enterprise_connectors" {
		if r.ConnectorCipher == nil {
			return results, errors.New("connector cipher is not configured")
		}
		logMessage(r.Options.Logger, "[reencrypt-start] table=enterprise_connectors active_key_id=%s dry_run=%t batch_size=%d",
			r.ConnectorCipher.ActiveKeyID(), r.Options.DryRun, r.Options.BatchSize)
		st, err := ReencryptEnterpriseConnectors(ctx, r.DB, r.ConnectorCipher, r.Options)
		results["enterprise_connectors"] = st
		logMessage(r.Options.Logger, "[reencrypt-finish] table=enterprise_connectors %s", st)
		if err != nil {
			return results, err
		}
	}

	return results, nil
}
