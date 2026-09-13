package storage

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"testing"
)

// fakeResult implements sql.Result
type fakeResult struct {
	rowsAffected int64
}

func (r fakeResult) LastInsertId() (int64, error) { return 0, nil }
func (r fakeResult) RowsAffected() (int64, error) { return r.rowsAffected, nil }

// fakeRowIterator implements RowsIterator for testing
type fakeRowIterator struct {
	rows [][]any
	idx  int
	err  error
}

func (it *fakeRowIterator) Next() bool {
	if it.idx < len(it.rows) {
		it.idx++
		return true
	}
	return false
}

func (it *fakeRowIterator) Scan(dest ...any) error {
	if it.idx == 0 || it.idx > len(it.rows) {
		return errors.New("scan called on invalid row index")
	}
	currentRow := it.rows[it.idx-1]
	if len(dest) != len(currentRow) {
		return fmt.Errorf("scan dest len %d != row len %d", len(dest), len(currentRow))
	}
	for i, d := range dest {
		switch target := d.(type) {
		case *string:
			*target = fmt.Sprint(currentRow[i])
		default:
			return fmt.Errorf("unsupported test scan dest type: %T", d)
		}
	}
	return nil
}

func (it *fakeRowIterator) Close() error { return nil }
func (it *fakeRowIterator) Err() error   { return it.err }

// inMemoryStorageConfigRow simulates a row in xz_storage_configs
type inMemoryStorageConfigRow struct {
	id           string
	accessKey    string
	secretKey    string
	sessionToken string
}

// inMemoryConnectorRow simulates a row in enterprise_connectors
type inMemoryConnectorRow struct {
	id                string
	appSecret         string
	verificationToken string
	encryptKey        string
}

// fakeDBExecutor simulates SQL queries and CAS updates for unit testing re-encryption.
type fakeDBExecutor struct {
	mu             sync.Mutex
	storageConfigs map[string]inMemoryStorageConfigRow
	connectors     map[string]inMemoryConnectorRow
	conflictNext   bool // If true, next ExecContext simulates a CAS mismatch (rowsAffected=0)
}

func newFakeDBExecutor() *fakeDBExecutor {
	return &fakeDBExecutor{
		storageConfigs: make(map[string]inMemoryStorageConfigRow),
		connectors:     make(map[string]inMemoryConnectorRow),
	}
}

func (db *fakeDBExecutor) QueryContext(_ context.Context, query string, args ...any) (RowsIterator, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	lastID := fmt.Sprint(args[0])
	limit := args[1].(int)

	if strings.Contains(query, "xz_storage_configs") {
		// Collect matching rows
		var ids []string
		for id := range db.storageConfigs {
			if id > lastID {
				ids = append(ids, id)
			}
		}
		sort.Strings(ids)

		var resultRows [][]any
		count := 0
		for _, id := range ids {
			if count >= limit {
				break
			}
			r := db.storageConfigs[id]
			resultRows = append(resultRows, []any{r.id, r.accessKey, r.secretKey, r.sessionToken})
			count++
		}
		return &fakeRowIterator{rows: resultRows}, nil
	}

	if strings.Contains(query, "enterprise_connectors") {
		var ids []string
		for id := range db.connectors {
			if id > lastID {
				ids = append(ids, id)
			}
		}
		sort.Strings(ids)

		var resultRows [][]any
		count := 0
		for _, id := range ids {
			if count >= limit {
				break
			}
			r := db.connectors[id]
			resultRows = append(resultRows, []any{r.id, r.appSecret, r.verificationToken, r.encryptKey})
			count++
		}
		return &fakeRowIterator{rows: resultRows}, nil
	}

	return nil, fmt.Errorf("unknown query in fakeDB: %s", query)
}

func (db *fakeDBExecutor) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.conflictNext {
		db.conflictNext = false
		return fakeResult{rowsAffected: 0}, nil
	}

	if strings.Contains(query, "UPDATE xz_storage_configs") {
		// args: newAK($1), newSK($2), newST($3), id($4), oldAK($5), oldSK($6), oldST($7)
		newAK := fmt.Sprint(args[0])
		newSK := fmt.Sprint(args[1])
		newST := fmt.Sprint(args[2])
		id := fmt.Sprint(args[3])
		oldAK := fmt.Sprint(args[4])
		oldSK := fmt.Sprint(args[5])
		oldST := fmt.Sprint(args[6])

		current, ok := db.storageConfigs[id]
		if !ok {
			return fakeResult{rowsAffected: 0}, nil
		}
		// CAS match
		if current.accessKey != oldAK || current.secretKey != oldSK || current.sessionToken != oldST {
			return fakeResult{rowsAffected: 0}, nil
		}

		db.storageConfigs[id] = inMemoryStorageConfigRow{
			id:           id,
			accessKey:    newAK,
			secretKey:    newSK,
			sessionToken: newST,
		}
		return fakeResult{rowsAffected: 1}, nil
	}

	if strings.Contains(query, "UPDATE enterprise_connectors") {
		// args: newSecret($1), newVT($2), newEK($3), id($4), oldSecret($5), oldVT($6), oldEK($7)
		newSecret := fmt.Sprint(args[0])
		newVT := fmt.Sprint(args[1])
		newEK := fmt.Sprint(args[2])
		id := fmt.Sprint(args[3])
		oldSecret := fmt.Sprint(args[4])
		oldVT := fmt.Sprint(args[5])
		oldEK := fmt.Sprint(args[6])

		current, ok := db.connectors[id]
		if !ok {
			return fakeResult{rowsAffected: 0}, nil
		}
		// CAS match
		if current.appSecret != oldSecret || current.verificationToken != oldVT || current.encryptKey != oldEK {
			return fakeResult{rowsAffected: 0}, nil
		}

		db.connectors[id] = inMemoryConnectorRow{
			id:                id,
			appSecret:         newSecret,
			verificationToken: newVT,
			encryptKey:        newEK,
		}
		return fakeResult{rowsAffected: 1}, nil
	}

	return nil, fmt.Errorf("unknown exec in fakeDB: %s", query)
}

// 1. ReencryptSecretValue 单元测试
func TestReencryptSecretValue(t *testing.T) {
	cipher, err := NewKeyringCipher(KeyringConfig{
		ActiveKeyID: "k2",
		Keys: map[string]string{
			"k1": testLegacyKey,
			"k2": testKey2,
		},
		LegacyV1Key: testLegacyKey,
	})
	if err != nil {
		t.Fatal(err)
	}

	aad := "cfg_001"
	plainValue := "my-secret-token-12345"

	// Case A: v1 ciphertext -> re-encrypted to active key k2
	v1Ciphertext := generateV1Fixture(testLegacyKey, plainValue, aad, []byte("112233445566"))
	newVal, changed, err := ReencryptSecretValue(cipher, v1Ciphertext, aad)
	if err != nil {
		t.Fatalf("ReencryptSecretValue on v1 failed: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true for v1 ciphertext")
	}
	if !strings.HasPrefix(newVal, "enc:v2:k2:") {
		t.Fatalf("expected new ciphertext with active key k2 prefix, got %s", newVal)
	}
	// Verify it decrypts back to original plainValue
	decrypted, err := cipher.Decrypt(newVal, aad)
	if err != nil || decrypted != plainValue {
		t.Fatalf("decrypted re-encrypted value mismatch: got %q, want %q, err=%v", decrypted, plainValue, err)
	}

	// Case B: v2 non-active key (k1) -> re-encrypted to active key k2
	cipherK1Active, err := NewKeyringCipher(KeyringConfig{
		ActiveKeyID: "k1",
		Keys:        map[string]string{"k1": testLegacyKey, "k2": testKey2},
	})
	if err != nil {
		t.Fatal(err)
	}
	v2K1Ciphertext, err := cipherK1Active.Encrypt(plainValue, aad)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(v2K1Ciphertext, "enc:v2:k1:") {
		t.Fatalf("expected enc:v2:k1: prefix, got %s", v2K1Ciphertext)
	}

	newValK2, changed, err := ReencryptSecretValue(cipher, v2K1Ciphertext, aad)
	if err != nil || !changed {
		t.Fatalf("expected re-encryption from k1 to k2, got err=%v, changed=%t", err, changed)
	}
	if !strings.HasPrefix(newValK2, "enc:v2:k2:") {
		t.Fatalf("expected enc:v2:k2: prefix, got %s", newValK2)
	}

	// Case C: already active key (k2) -> idempotent skip
	sameVal, changed, err := ReencryptSecretValue(cipher, newValK2, aad)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("expected changed=false when already active key")
	}
	if sameVal != newValK2 {
		t.Fatalf("expected untouched value, got %s", sameVal)
	}

	// Case D: empty string -> skipped
	emptyVal, changed, err := ReencryptSecretValue(cipher, "", aad)
	if err != nil || changed || emptyVal != "" {
		t.Fatalf("expected empty skip, got val=%q changed=%t err=%v", emptyVal, changed, err)
	}

	// Case E: corrupted ciphertext -> fail-closed, does not overwrite
	corrupted := v1Ciphertext[:len(v1Ciphertext)-5] + "XXXXX"
	_, _, err = ReencryptSecretValue(cipher, corrupted, aad)
	if err == nil {
		t.Fatal("expected error on corrupted ciphertext, got nil")
	}

	// Case F: AAD mismatch -> fail-closed error
	_, _, err = ReencryptSecretValue(cipher, v1Ciphertext, "wrong_aad")
	if err == nil {
		t.Fatal("expected error on AAD mismatch, got nil")
	}
}

// 2. ReencryptStorageConfigs 端到端与 CAS 测试
func TestReencryptStorageConfigs_Lifecycle(t *testing.T) {
	cipher, err := NewKeyringCipher(KeyringConfig{
		ActiveKeyID: "k2",
		Keys: map[string]string{
			"k1": testLegacyKey,
			"k2": testKey2,
		},
		LegacyV1Key: testLegacyKey,
	})
	if err != nil {
		t.Fatal(err)
	}

	db := newFakeDBExecutor()

	// Seed 4 records:
	// cfg_001: v1 credentials (needs migration)
	// cfg_002: v2 k1 credentials (needs migration to k2)
	// cfg_003: already v2 k2 credentials (should skip)
	// cfg_004: empty credentials (should skip)
	v1AK := generateV1Fixture(testLegacyKey, "ak-001", "cfg_001", []byte("111111111111"))
	v1SK := generateV1Fixture(testLegacyKey, "sk-001", "cfg_001", []byte("222222222222"))
	db.storageConfigs["cfg_001"] = inMemoryStorageConfigRow{
		id: "cfg_001", accessKey: v1AK, secretKey: v1SK, sessionToken: "",
	}

	c1, _ := NewKeyringCipher(KeyringConfig{ActiveKeyID: "k1", Keys: map[string]string{"k1": testLegacyKey}})
	v2K1AK, _ := c1.Encrypt("ak-002", "cfg_002")
	v2K1SK, _ := c1.Encrypt("sk-002", "cfg_002")
	db.storageConfigs["cfg_002"] = inMemoryStorageConfigRow{
		id: "cfg_002", accessKey: v2K1AK, secretKey: v2K1SK, sessionToken: "",
	}

	v2K2AK, _ := cipher.Encrypt("ak-003", "cfg_003")
	v2K2SK, _ := cipher.Encrypt("sk-003", "cfg_003")
	db.storageConfigs["cfg_003"] = inMemoryStorageConfigRow{
		id: "cfg_003", accessKey: v2K2AK, secretKey: v2K2SK, sessionToken: "",
	}

	db.storageConfigs["cfg_004"] = inMemoryStorageConfigRow{
		id: "cfg_004", accessKey: "", secretKey: "", sessionToken: "",
	}

	logBuf := &bytes.Buffer{}

	// --- Phase 1: Dry-Run ---
	dryOpts := ReencryptOptions{DryRun: true, BatchSize: 2, Logger: logBuf}
	dryStats, err := ReencryptStorageConfigs(context.Background(), db, cipher, dryOpts)
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}
	if dryStats.Scanned != 4 || dryStats.Migrated != 2 || dryStats.Skipped != 2 || dryStats.Failed != 0 || dryStats.Conflicted != 0 {
		t.Fatalf("unexpected dry-run stats: %#v", dryStats)
	}

	// Verify dry-run did NOT modify db records
	if db.storageConfigs["cfg_001"].accessKey != v1AK {
		t.Fatal("dry-run must not modify database records")
	}

	// --- Phase 2: Live Migration ---
	logBuf.Reset()
	liveOpts := ReencryptOptions{DryRun: false, BatchSize: 2, Logger: logBuf}
	liveStats, err := ReencryptStorageConfigs(context.Background(), db, cipher, liveOpts)
	if err != nil {
		t.Fatalf("live run failed: %v", err)
	}
	if liveStats.Scanned != 4 || liveStats.Migrated != 2 || liveStats.Skipped != 2 || liveStats.Failed != 0 || liveStats.Conflicted != 0 {
		t.Fatalf("unexpected live-run stats: %#v", liveStats)
	}

	// Verify cfg_001 is now v2 k2 and decrypts properly
	migratedAK := db.storageConfigs["cfg_001"].accessKey
	if !strings.HasPrefix(migratedAK, "enc:v2:k2:") {
		t.Fatalf("expected enc:v2:k2: prefix after migration, got: %s", migratedAK)
	}
	decAK, err := cipher.Decrypt(migratedAK, "cfg_001")
	if err != nil || decAK != "ak-001" {
		t.Fatalf("decrypted value mismatch: got %q, want 'ak-001', err=%v", decAK, err)
	}

	// --- Phase 3: Idempotency Second Run ---
	logBuf.Reset()
	secondStats, err := ReencryptStorageConfigs(context.Background(), db, cipher, liveOpts)
	if err != nil {
		t.Fatalf("second run failed: %v", err)
	}
	if secondStats.Scanned != 4 || secondStats.Migrated != 0 || secondStats.Skipped != 4 || secondStats.Failed != 0 {
		t.Fatalf("expected all skipped on second run, got: %#v", secondStats)
	}
}

// 3. CAS 并发冲突与损坏密文保护测试
func TestReencryptStorageConfigs_ConflictAndCorruptProtection(t *testing.T) {
	cipher, err := NewKeyringCipher(KeyringConfig{
		ActiveKeyID: "k2",
		Keys: map[string]string{
			"k1": testLegacyKey,
			"k2": testKey2,
		},
		LegacyV1Key: testLegacyKey,
	})
	if err != nil {
		t.Fatal(err)
	}

	db := newFakeDBExecutor()

	// cfg_conflict: needs migration, but db.conflictNext will simulate concurrent update
	v1AK := generateV1Fixture(testLegacyKey, "ak-conflict", "cfg_conflict", []byte("111111111111"))
	db.storageConfigs["cfg_conflict"] = inMemoryStorageConfigRow{
		id: "cfg_conflict", accessKey: v1AK, secretKey: "", sessionToken: "",
	}

	db.conflictNext = true // Trigger CAS conflict
	opts := ReencryptOptions{DryRun: false, BatchSize: 10, Logger: io.Discard}
	stats, err := ReencryptStorageConfigs(context.Background(), db, cipher, opts)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Conflicted != 1 || stats.Migrated != 0 {
		t.Fatalf("expected 1 conflict, got stats: %#v", stats)
	}

	// Verify old ciphertext remained intact after conflict
	if db.storageConfigs["cfg_conflict"].accessKey != v1AK {
		t.Fatal("ciphertext must remain untouched after CAS conflict")
	}

	// cfg_corrupt: decryption failure must be marked Failed and NOT overwritten
	db.storageConfigs["cfg_corrupt"] = inMemoryStorageConfigRow{
		id: "cfg_corrupt", accessKey: "enc:v1:CORRUPTED_BASE64_PAYLOAD", secretKey: "", sessionToken: "",
	}
	statsCorrupt, err := ReencryptStorageConfigs(context.Background(), db, cipher, opts)
	if err != nil {
		t.Fatal(err)
	}
	if statsCorrupt.Failed != 1 {
		t.Fatalf("expected 1 failure on corrupt ciphertext, got: %#v", statsCorrupt)
	}
	if db.storageConfigs["cfg_corrupt"].accessKey != "enc:v1:CORRUPTED_BASE64_PAYLOAD" {
		t.Fatal("corrupt ciphertext must NOT be overwritten")
	}
}

// 4. ReencryptEnterpriseConnectors 测试
func TestReencryptEnterpriseConnectors_Lifecycle(t *testing.T) {
	cipher, err := NewKeyringCipher(KeyringConfig{
		ActiveKeyID: "k2",
		Keys: map[string]string{
			"k1": testLegacyKey,
			"k2": testKey2,
		},
		LegacyV1Key: testLegacyKey,
	})
	if err != nil {
		t.Fatal(err)
	}

	db := newFakeDBExecutor()

	// conn_001: v1 credentials
	// Note AAD format: id + ":app_secret", id + ":verification_token", id + ":encrypt_key"
	v1Secret := generateV1Fixture(testLegacyKey, "feishu-app-secret", "conn_001:app_secret", []byte("111111111111"))
	v1VT := generateV1Fixture(testLegacyKey, "feishu-vt", "conn_001:verification_token", []byte("222222222222"))
	v1EK := generateV1Fixture(testLegacyKey, "feishu-ek", "conn_001:encrypt_key", []byte("333333333333"))

	db.connectors["conn_001"] = inMemoryConnectorRow{
		id:                "conn_001",
		appSecret:         v1Secret,
		verificationToken: v1VT,
		encryptKey:        v1EK,
	}

	opts := ReencryptOptions{DryRun: false, BatchSize: 10, Logger: io.Discard}
	stats, err := ReencryptEnterpriseConnectors(context.Background(), db, cipher, opts)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Scanned != 1 || stats.Migrated != 1 || stats.Failed != 0 {
		t.Fatalf("unexpected connector migration stats: %#v", stats)
	}

	// Verify all 3 fields migrated to k2 and decrypt with respective AAD
	migrated := db.connectors["conn_001"]
	if !strings.HasPrefix(migrated.appSecret, "enc:v2:k2:") {
		t.Fatalf("appSecret missing v2:k2 prefix: %s", migrated.appSecret)
	}
	decSecret, err := cipher.Decrypt(migrated.appSecret, "conn_001:app_secret")
	if err != nil || decSecret != "feishu-app-secret" {
		t.Fatalf("decrypted secret mismatch: got %q, want 'feishu-app-secret', err=%v", decSecret, err)
	}

	decVT, err := cipher.Decrypt(migrated.verificationToken, "conn_001:verification_token")
	if err != nil || decVT != "feishu-vt" {
		t.Fatalf("decrypted VT mismatch: got %q, want 'feishu-vt', err=%v", decVT, err)
	}

	decEK, err := cipher.Decrypt(migrated.encryptKey, "conn_001:encrypt_key")
	if err != nil || decEK != "feishu-ek" {
		t.Fatalf("decrypted EK mismatch: got %q, want 'feishu-ek', err=%v", decEK, err)
	}
}

// 5. 日志与输出绝不泄漏密钥与明文
func TestReencrypt_NoSecretLeakInLogs(t *testing.T) {
	secretPlaintext := "highly-confidential-secret-plaintext-12345"
	cipher, err := NewKeyringCipher(KeyringConfig{
		ActiveKeyID: "k2",
		Keys: map[string]string{
			"k1": testLegacyKey,
			"k2": testKey2,
		},
		LegacyV1Key: testLegacyKey,
	})
	if err != nil {
		t.Fatal(err)
	}

	db := newFakeDBExecutor()
	v1AK := generateV1Fixture(testLegacyKey, secretPlaintext, "cfg_leak_test", []byte("111111111111"))
	db.storageConfigs["cfg_leak_test"] = inMemoryStorageConfigRow{
		id: "cfg_leak_test", accessKey: v1AK,
	}

	logBuf := &bytes.Buffer{}
	opts := ReencryptOptions{DryRun: false, BatchSize: 10, Logger: logBuf}
	_, err = ReencryptStorageConfigs(context.Background(), db, cipher, opts)
	if err != nil {
		t.Fatal(err)
	}

	output := logBuf.String()
	if strings.Contains(output, secretPlaintext) {
		t.Fatalf("log leaked secret plaintext: %s", output)
	}
	if strings.Contains(output, testLegacyKey) || strings.Contains(output, testKey2) {
		t.Fatalf("log leaked key material: %s", output)
	}
}
