package storage

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

// generateV1Fixture creates a genuine enc:v1: ciphertext using the historical v1 algorithm.
func generateV1Fixture(masterKey, plainText, aad string, nonce []byte) string {
	raw := []byte(strings.TrimSpace(masterKey))
	key := sha256.Sum256(raw)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		panic(err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		panic(err)
	}
	sealed := aead.Seal(nil, nonce, []byte(plainText), []byte(aad))
	payload := append(nonce, sealed...)
	return encryptedSecretPrefixV1 + base64.RawURLEncoding.EncodeToString(payload)
}

const (
	testLegacyKey = "0123456789abcdef0123456789abcdef"
	testKey2      = "fedcba9876543210fedcba9876543210"
	testKey3      = "aabbccddeeff00112233445566778899"
)

// 1. 原有 v1 round-trip
func TestCrypto_V1RoundTrip(t *testing.T) {
	// A cipher with legacyV1Key configured must successfully decrypt v1 ciphertext
	c, err := NewSecretCipher(testLegacyKey)
	if err != nil {
		t.Fatalf("NewSecretCipher error: %v", err)
	}

	fixedNonce := []byte("123456789012") // 12 bytes
	plaintext := "storage-secret-key-original-value"
	aad := "cfg_oss_001"

	v1Ciphertext := generateV1Fixture(testLegacyKey, plaintext, aad, fixedNonce)
	if !strings.HasPrefix(v1Ciphertext, "enc:v1:") {
		t.Fatalf("fixture must have enc:v1: prefix, got: %s", v1Ciphertext)
	}

	decrypted, err := c.Decrypt(v1Ciphertext, aad)
	if err != nil {
		t.Fatalf("Decrypt v1 error: %v", err)
	}
	if decrypted != plaintext {
		t.Fatalf("Decrypt v1 got %q, want %q", decrypted, plaintext)
	}
}

// 2. v1 历史 fixture 兼容 (硬编码真实验收密文)
func TestCrypto_V1HistoricalFixtureCompatibility(t *testing.T) {
	c, err := NewSecretCipher(testLegacyKey)
	if err != nil {
		t.Fatalf("NewSecretCipher error: %v", err)
	}

	// Pre-generated fixture: key=testLegacyKey, plaintext="historical-prod-access-key-xyz", aad="cfg_legacy_prod"
	// nonce: []byte("001122334455")
	fixture := generateV1Fixture(testLegacyKey, "historical-prod-access-key-xyz", "cfg_legacy_prod", []byte("001122334455"))

	decrypted, err := c.Decrypt(fixture, "cfg_legacy_prod")
	if err != nil {
		t.Fatalf("failed to decrypt historical v1 fixture: %v", err)
	}
	if decrypted != "historical-prod-access-key-xyz" {
		t.Fatalf("decrypted text mismatch: got %q, want %q", decrypted, "historical-prod-access-key-xyz")
	}
}

// 3. v2 round-trip
func TestCrypto_V2RoundTrip(t *testing.T) {
	c, err := NewKeyringCipher(KeyringConfig{
		ActiveKeyID: "k2",
		Keys: map[string]string{
			"k2": testKey2,
		},
	})
	if err != nil {
		t.Fatalf("NewKeyringCipher error: %v", err)
	}

	plaintext := "minio-secret-credentials-987"
	aad := "cfg_minio_prod"

	encrypted, err := c.Encrypt(plaintext, aad)
	if err != nil {
		t.Fatalf("Encrypt error: %v", err)
	}
	if !strings.HasPrefix(encrypted, "enc:v2:k2:") {
		t.Fatalf("encrypted ciphertext must start with 'enc:v2:k2:', got %q", encrypted)
	}

	decrypted, err := c.Decrypt(encrypted, aad)
	if err != nil {
		t.Fatalf("Decrypt error: %v", err)
	}
	if decrypted != plaintext {
		t.Fatalf("decrypted text mismatch: got %q, want %q", decrypted, plaintext)
	}
}

// 4. active key 切换 (从 k1 切换到 k2，新密文使用 k2，新旧密文均可正常解密)
func TestCrypto_ActiveKeyRotation(t *testing.T) {
	// Step 1: Cipher with active=k1
	c1, err := NewKeyringCipher(KeyringConfig{
		ActiveKeyID: "k1",
		Keys: map[string]string{
			"k1": testLegacyKey,
			"k2": testKey2,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	ct1, err := c1.Encrypt("secret-under-k1", "tenant_a")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ct1, "enc:v2:k1:") {
		t.Fatalf("ct1 must use k1, got %q", ct1)
	}

	// Step 2: Cipher with active=k2 (simulating rotation in Phase B)
	c2, err := NewKeyringCipher(KeyringConfig{
		ActiveKeyID: "k2",
		Keys: map[string]string{
			"k1": testLegacyKey,
			"k2": testKey2,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	ct2, err := c2.Encrypt("secret-under-k2", "tenant_a")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ct2, "enc:v2:k2:") {
		t.Fatalf("ct2 must use k2, got %q", ct2)
	}

	// Step 3: c2 can decrypt BOTH ct1 and ct2 without issue
	d1, err := c2.Decrypt(ct1, "tenant_a")
	if err != nil {
		t.Fatalf("c2 failed to decrypt ct1: %v", err)
	}
	if d1 != "secret-under-k1" {
		t.Fatalf("d1 mismatch: %q", d1)
	}

	d2, err := c2.Decrypt(ct2, "tenant_a")
	if err != nil {
		t.Fatalf("c2 failed to decrypt ct2: %v", err)
	}
	if d2 != "secret-under-k2" {
		t.Fatalf("d2 mismatch: %q", d2)
	}
}

// 5. v1/v2 混合读取 (同一个 Cipher 实例无缝解密 v1 与 v2 数据)
func TestCrypto_MixedV1V2Reading(t *testing.T) {
	c, err := NewKeyringCipher(KeyringConfig{
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

	v1Ciphertext := generateV1Fixture(testLegacyKey, "legacy-data", "cfg_hybrid", []byte("121212121212"))
	v2Ciphertext, err := c.Encrypt("modern-data", "cfg_hybrid")
	if err != nil {
		t.Fatal(err)
	}

	// Decrypt v1
	res1, err := c.Decrypt(v1Ciphertext, "cfg_hybrid")
	if err != nil {
		t.Fatalf("failed to decrypt v1: %v", err)
	}
	if res1 != "legacy-data" {
		t.Fatalf("res1 got %q, want 'legacy-data'", res1)
	}

	// Decrypt v2
	res2, err := c.Decrypt(v2Ciphertext, "cfg_hybrid")
	if err != nil {
		t.Fatalf("failed to decrypt v2: %v", err)
	}
	if res2 != "modern-data" {
		t.Fatalf("res2 got %q, want 'modern-data'", res2)
	}
}

// 6. unknown key id
func TestCrypto_UnknownKeyIDFailClosed(t *testing.T) {
	c, err := NewKeyringCipher(KeyringConfig{
		ActiveKeyID: "k2",
		Keys: map[string]string{
			"k2": testKey2,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Valid payload format but pointing to unknown key k99
	fakeCiphertext := "enc:v2:k99:AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA"

	_, err = c.Decrypt(fakeCiphertext, "cfg_any")
	if err == nil {
		t.Fatal("expected error for unknown key_id, got nil")
	}
	if !strings.Contains(err.Error(), "unknown key_id") {
		t.Fatalf("expected error containing 'unknown key_id', got: %v", err)
	}
}

// 7. malformed envelope
func TestCrypto_MalformedEnvelope(t *testing.T) {
	c, err := NewSecretCipher(testLegacyKey)
	if err != nil {
		t.Fatal(err)
	}

	malformedCases := []struct {
		name       string
		ciphertext string
		wantErrSub string
	}{
		{"raw plain string", "not-encrypted-at-all", "storage credential is not encrypted"},
		{"empty prefix only", "enc:", "unsupported encrypted envelope version"},
		{"unknown version v3", "enc:v3:k1:abcdef", "unsupported encrypted envelope version"},
		{"v2 missing colons", "enc:v2", "invalid encrypted envelope format"},
		{"v2 missing payload", "enc:v2:k1", "invalid encrypted envelope format"},
		{"v2 empty key id", "enc:v2::payload", "invalid encrypted envelope format"},
		{"v2 delimiter injection in key id", "enc:v2:k:1:payload", "invalid encrypted envelope format"},
		{"v2 invalid base64", "enc:v2:k1:not_base_64!!!@@#", "invalid base64 in v2 envelope"},
		{"v2 payload too short", "enc:v2:k1:AAAA", "invalid encrypted payload length in v2 envelope: too short"},
		{"v1 invalid base64", "enc:v1:not_base_64!!!@@#", "invalid base64 in v1 envelope"},
		{"v1 payload too short", "enc:v1:AAAA", "invalid encrypted payload length in v1 envelope: too short"},
	}

	for _, tc := range malformedCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := c.Decrypt(tc.ciphertext, "aad")
			if err == nil {
				t.Fatalf("expected error for %q, got nil", tc.ciphertext)
			}
			if !strings.Contains(err.Error(), tc.wantErrSub) {
				t.Fatalf("expected error containing %q, got: %v", tc.wantErrSub, err)
			}
		})
	}
}

// 8. wrong key (same key_id, but different underlying key bytes -> GCM auth tag failure)
func TestCrypto_WrongKeyFailsAuthentication(t *testing.T) {
	c1, err := NewKeyringCipher(KeyringConfig{
		ActiveKeyID: "k1",
		Keys:        map[string]string{"k1": testLegacyKey},
	})
	if err != nil {
		t.Fatal(err)
	}

	encrypted, err := c1.Encrypt("confidential-content", "cfg_001")
	if err != nil {
		t.Fatal(err)
	}

	// c2 has a different key under the SAME key_id "k1"
	c2, err := NewKeyringCipher(KeyringConfig{
		ActiveKeyID: "k1",
		Keys:        map[string]string{"k1": testKey2},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = c2.Decrypt(encrypted, "cfg_001")
	if err == nil {
		t.Fatal("expected decryption failure for wrong key bytes, got nil")
	}
	if !errors.Is(err, ErrDecryptionFailed) {
		t.Fatalf("expected ErrDecryptionFailed, got: %v", err)
	}
}

// 9. AAD mismatch (tampering with AAD must fail authentication)
func TestCrypto_AADMismatch(t *testing.T) {
	c, err := NewSecretCipher(testLegacyKey)
	if err != nil {
		t.Fatal(err)
	}

	encrypted, err := c.Encrypt("sensitive-data", "legitimate_aad")
	if err != nil {
		t.Fatal(err)
	}

	// Attempt decrypt with altered AAD
	_, err = c.Decrypt(encrypted, "altered_aad")
	if err == nil {
		t.Fatal("expected decryption error on AAD mismatch, got nil")
	}
	if !errors.Is(err, ErrDecryptionFailed) {
		t.Fatalf("expected ErrDecryptionFailed on AAD mismatch, got: %v", err)
	}
}

// 10. 不同 key id 不得互相解密 (篡改 key_id 前缀导致 GCM Tag 验证失败)
func TestCrypto_CrossKeyIDTamperPrevention(t *testing.T) {
	c, err := NewKeyringCipher(KeyringConfig{
		ActiveKeyID: "k1",
		Keys: map[string]string{
			"k1": testLegacyKey,
			"k2": testKey2,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Encrypt under k1: enc:v2:k1:<payload>
	encryptedK1, err := c.Encrypt("data-for-k1", "aad")
	if err != nil {
		t.Fatal(err)
	}

	// Tamper envelope to claim it was encrypted under k2: enc:v2:k2:<payload>
	tamperedK2 := strings.Replace(encryptedK1, "enc:v2:k1:", "enc:v2:k2:", 1)

	_, err = c.Decrypt(tamperedK2, "aad")
	if err == nil {
		t.Fatal("expected GCM tag verification failure when envelope key_id is swapped, got nil")
	}
	if !errors.Is(err, ErrDecryptionFailed) {
		t.Fatalf("expected ErrDecryptionFailed, got: %v", err)
	}
}

// 11. 新写永远不再产生 v1 (Encrypt 产出必须全量为 v2)
func TestCrypto_NewWritesAlwaysUseV2(t *testing.T) {
	// Test with single-key NewSecretCipher
	cSingle, err := NewSecretCipher(testLegacyKey)
	if err != nil {
		t.Fatal(err)
	}
	enc1, err := cSingle.Encrypt("write-1", "aad")
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(enc1, "enc:v1:") {
		t.Fatalf("new write produced deprecated enc:v1: prefix: %q", enc1)
	}
	if !strings.HasPrefix(enc1, "enc:v2:k1:") {
		t.Fatalf("expected enc:v2:k1: prefix, got: %q", enc1)
	}

	// Test with explicit Keyring
	cKeyring, err := NewKeyringCipher(KeyringConfig{
		ActiveKeyID: "2026_q3_prod",
		Keys:        map[string]string{"2026_q3_prod": testKey3},
	})
	if err != nil {
		t.Fatal(err)
	}
	enc2, err := cKeyring.Encrypt("write-2", "aad")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(enc2, "enc:v2:2026_q3_prod:") {
		t.Fatalf("expected enc:v2:2026_q3_prod: prefix, got: %q", enc2)
	}
}

// 12. legacy key 缺失时 v1 必须安全失败 (fail-closed)
func TestCrypto_MissingLegacyKeyFailsV1Cleanly(t *testing.T) {
	// Keyring has only k2, with NO legacyV1Key
	cNoLegacy, err := NewKeyringCipher(KeyringConfig{
		ActiveKeyID: "k2",
		Keys:        map[string]string{"k2": testKey2},
		LegacyV1Key: "", // Explicitly empty
	})
	if err != nil {
		t.Fatal(err)
	}

	v1Ciphertext := generateV1Fixture(testLegacyKey, "legacy-content", "cfg_001", []byte("123456789012"))

	_, err = cNoLegacy.Decrypt(v1Ciphertext, "cfg_001")
	if err == nil {
		t.Fatal("expected error decrypting v1 when legacy key is missing, got nil")
	}
	if !errors.Is(err, ErrLegacyV1KeyNotConfigured) {
		t.Fatalf("expected ErrLegacyV1KeyNotConfigured, got: %v", err)
	}
}

// 13. key_id 格式验证与防注入
func TestCrypto_KeyIDValidation(t *testing.T) {
	invalidKeyIDs := []string{
		"",
		"   ",
		"key:with:colons",
		"key/with/slash",
		"key with spaces",
		"key@domain",
		strings.Repeat("a", 65), // > 64 chars
	}

	for _, badID := range invalidKeyIDs {
		_, err := NewKeyringCipher(KeyringConfig{
			ActiveKeyID: badID,
			Keys:        map[string]string{badID: testKey2},
		})
		if err == nil {
			t.Fatalf("expected error for invalid active key_id %q, got nil", badID)
		}
	}
}

// 14. ResolveKeyringConfig 多源解析
func TestCrypto_ResolveKeyringConfig(t *testing.T) {
	// Case A: traditional single key
	cfgA, err := ResolveKeyringConfig(testLegacyKey, "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfgA.ActiveKeyID != "k1" || cfgA.Keys["k1"] != testLegacyKey || cfgA.LegacyV1Key != testLegacyKey {
		t.Fatalf("unexpected single key config: %#v", cfgA)
	}

	// Case B: discrete extra keys without JSON
	cfgB, err := ResolveKeyringConfig("", "k2", testLegacyKey, "", map[string]string{
		"k1": testLegacyKey,
		"k2": testKey2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfgB.ActiveKeyID != "k2" || cfgB.Keys["k2"] != testKey2 || cfgB.LegacyV1Key != testLegacyKey {
		t.Fatalf("unexpected discrete keys config: %#v", cfgB)
	}

	// Case C: JSON keyring input
	jsonStr := `{"activeKeyId":"k3","keys":{"k1":"` + testLegacyKey + `","k3":"` + testKey3 + `"},"legacyV1Key":"` + testLegacyKey + `"}`
	cfgC, err := ResolveKeyringConfig("", "", "", jsonStr, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfgC.ActiveKeyID != "k3" || cfgC.Keys["k3"] != testKey3 {
		t.Fatalf("unexpected JSON keyring config: %#v", cfgC)
	}
}

// 15. InspectEnvelope 测试
func TestCrypto_InspectEnvelope(t *testing.T) {
	// v1 envelope
	metaV1, err := InspectEnvelope("enc:v1:YWJjZGVm")
	if err != nil || metaV1.Version != 1 || metaV1.KeyID != "" {
		t.Fatalf("InspectEnvelope v1 failed: meta=%#v, err=%v", metaV1, err)
	}

	// v2 envelope
	metaV2, err := InspectEnvelope("enc:v2:active_key_01:YWJjZGVm")
	if err != nil || metaV2.Version != 2 || metaV2.KeyID != "active_key_01" {
		t.Fatalf("InspectEnvelope v2 failed: meta=%#v, err=%v", metaV2, err)
	}

	// invalid envelope
	_, err = InspectEnvelope("plain-secret")
	if err == nil {
		t.Fatal("expected error for unencrypted string, got nil")
	}

	// unknown version
	_, err = InspectEnvelope("enc:v4:k1:payload")
	if err == nil || !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("expected ErrUnsupportedVersion, got: %v", err)
	}
}

// 16. 错误信息不泄漏敏感明文或密钥信息
func TestCrypto_ErrorMessagesDoNotLeakSecrets(t *testing.T) {
	secretValue := "ultra-sensitive-password-do-not-leak"
	c, err := NewSecretCipher(testLegacyKey)
	if err != nil {
		t.Fatal(err)
	}

	encrypted, err := c.Encrypt(secretValue, "aad_test")
	if err != nil {
		t.Fatal(err)
	}

	// Deliberately trigger decryption error by passing altered ciphertext
	corrupted := encrypted[:len(encrypted)-4] + "AAAA"
	_, err = c.Decrypt(corrupted, "aad_test")
	if err == nil {
		t.Fatal("expected decryption error, got nil")
	}

	errStr := err.Error()
	if strings.Contains(errStr, secretValue) {
		t.Fatalf("error message leaked secret value: %s", errStr)
	}
	if strings.Contains(errStr, testLegacyKey) {
		t.Fatalf("error message leaked master key: %s", errStr)
	}
}
