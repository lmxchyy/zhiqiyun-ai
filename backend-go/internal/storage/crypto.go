package storage

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
)

const (
	encryptedSecretPrefixV1 = "enc:v1:"
	encryptedSecretPrefixV2 = "enc:v2:"

	// encryptedSecretPrefix is retained as backward-compatible alias for v1 prefix.
	encryptedSecretPrefix = encryptedSecretPrefixV1
)

var (
	ErrLegacyV1KeyNotConfigured = errors.New("legacy v1 decryption key is not configured")
	ErrActiveKeyNotConfigured   = errors.New("active encryption key is not configured")
	ErrDecryptionFailed         = errors.New("decryption authentication failed")
	ErrInvalidEnvelope          = errors.New("invalid encrypted envelope format")
	ErrUnsupportedVersion       = errors.New("unsupported encrypted envelope version")
	ErrInvalidKeyID             = errors.New("invalid key_id format")
)

var keyIDRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

func isValidKeyID(id string) bool {
	return keyIDRegex.MatchString(id)
}

func deriveAEAD(secret string) (cipher.AEAD, error) {
	raw := []byte(strings.TrimSpace(secret))
	if decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(secret)); err == nil && len(decoded) >= 32 {
		raw = decoded
	}
	if len(raw) < 32 {
		return nil, ErrSecretCipherRequired
	}
	key := sha256.Sum256(raw)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// KeyringConfig defines multi-key keyring parameters.
type KeyringConfig struct {
	ActiveKeyID string            `json:"activeKeyId"`
	Keys        map[string]string `json:"keys"`
	LegacyV1Key string            `json:"legacyV1Key,omitempty"`
}

// SecretCipher manages AES-256-GCM encryption and decryption with key versioning,
// keyrings, and dual-read support for legacy enc:v1: and modern enc:v2:<key_id>: envelopes.
type SecretCipher struct {
	activeKeyID string
	keys        map[string]cipher.AEAD
	legacyV1Key cipher.AEAD
}

// NewKeyringCipher creates a SecretCipher from a structured KeyringConfig.
func NewKeyringCipher(cfg KeyringConfig) (*SecretCipher, error) {
	activeKeyID := strings.TrimSpace(cfg.ActiveKeyID)
	if activeKeyID == "" {
		return nil, errors.New("active key_id is required")
	}
	if !isValidKeyID(activeKeyID) {
		return nil, fmt.Errorf("%w: active key_id %q must match ^[a-zA-Z0-9_-]{1,64}$", ErrInvalidKeyID, activeKeyID)
	}
	if len(cfg.Keys) == 0 {
		return nil, errors.New("keyring must contain at least one key")
	}

	keys := make(map[string]cipher.AEAD, len(cfg.Keys))
	for kid, secret := range cfg.Keys {
		trimmedID := strings.TrimSpace(kid)
		if !isValidKeyID(trimmedID) {
			return nil, fmt.Errorf("%w: key_id %q must match ^[a-zA-Z0-9_-]{1,64}$", ErrInvalidKeyID, kid)
		}
		aead, err := deriveAEAD(secret)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize key %q: %w", kid, err)
		}
		keys[trimmedID] = aead
	}

	if _, ok := keys[activeKeyID]; !ok {
		return nil, fmt.Errorf("active key_id %q not found in keyring keys", activeKeyID)
	}

	var legacyAEAD cipher.AEAD
	if strings.TrimSpace(cfg.LegacyV1Key) != "" {
		var err error
		legacyAEAD, err = deriveAEAD(cfg.LegacyV1Key)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize legacy v1 key: %w", err)
		}
	}

	return &SecretCipher{
		activeKeyID: activeKeyID,
		keys:        keys,
		legacyV1Key: legacyAEAD,
	}, nil
}

// NewSecretCipher initializes a SecretCipher.
// If masterKey starts with '{' and parses as valid KeyringConfig JSON, it initializes as a Keyring.
// Otherwise, it initializes in single-key backward-compatible mode:
// masterKey is used as the legacyV1Key (for decrypting enc:v1:) and as active key "k1" (for enc:v2:k1:).
func NewSecretCipher(masterKey string) (*SecretCipher, error) {
	trimmed := strings.TrimSpace(masterKey)
	if trimmed == "" {
		return nil, ErrSecretCipherRequired
	}

	// 1. Check if masterKey is a JSON KeyringConfig
	if strings.HasPrefix(trimmed, "{") {
		var cfg KeyringConfig
		if err := json.Unmarshal([]byte(trimmed), &cfg); err == nil && len(cfg.Keys) > 0 {
			return NewKeyringCipher(cfg)
		}
	}

	// 2. Single-key mode: derive AEAD from masterKey
	aead, err := deriveAEAD(trimmed)
	if err != nil {
		return nil, err
	}

	return &SecretCipher{
		activeKeyID: "k1",
		keys: map[string]cipher.AEAD{
			"k1": aead,
		},
		legacyV1Key: aead,
	}, nil
}

// Encrypt encrypts plainText using the active key in the keyring.
// It always outputs modern v2 envelope format: enc:v2:<active_key_id>:<payload>.
// It never outputs v1 format.
func (c *SecretCipher) Encrypt(plainText string, configID string) (string, error) {
	if strings.TrimSpace(plainText) == "" {
		return "", nil
	}
	if c == nil || len(c.keys) == 0 {
		return "", ErrSecretCipherRequired
	}
	aead, ok := c.keys[c.activeKeyID]
	if !ok || aead == nil {
		return "", ErrActiveKeyNotConfigured
	}

	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := aead.Seal(nil, nonce, []byte(plainText), []byte(configID))
	payload := append(nonce, sealed...)
	return fmt.Sprintf("%s%s:%s", encryptedSecretPrefixV2, c.activeKeyID, base64.RawURLEncoding.EncodeToString(payload)), nil
}

// Decrypt decrypts cipherText with dual-read support:
// - enc:v1:<payload> decrypts using legacyV1Key (if configured);
// - enc:v2:<key_id>:<payload> decrypts using the specified key_id from the keyring.
// Decryption fail-closed without leaking key material or plaintext in errors.
func (c *SecretCipher) Decrypt(cipherText string, configID string) (string, error) {
	if strings.TrimSpace(cipherText) == "" {
		return "", nil
	}
	if c == nil {
		return "", ErrSecretCipherRequired
	}

	// 1. Handle legacy v1 envelope: enc:v1:<payload>
	if strings.HasPrefix(cipherText, encryptedSecretPrefixV1) {
		if c.legacyV1Key == nil {
			return "", ErrLegacyV1KeyNotConfigured
		}
		rawPayload := strings.TrimPrefix(cipherText, encryptedSecretPrefixV1)
		payload, err := base64.RawURLEncoding.DecodeString(rawPayload)
		if err != nil {
			return "", fmt.Errorf("invalid base64 in v1 envelope: %w", err)
		}
		nonceSize := c.legacyV1Key.NonceSize()
		if len(payload) <= nonceSize {
			return "", fmt.Errorf("invalid encrypted payload length in v1 envelope: too short")
		}
		nonce, sealed := payload[:nonceSize], payload[nonceSize:]
		plainText, err := c.legacyV1Key.Open(nil, nonce, sealed, []byte(configID))
		if err != nil {
			return "", ErrDecryptionFailed
		}
		return string(plainText), nil
	}

	// 2. Handle modern v2 envelope: enc:v2:<key_id>:<payload>
	if strings.HasPrefix(cipherText, "enc:v2") && !strings.HasPrefix(cipherText, encryptedSecretPrefixV2) {
		return "", fmt.Errorf("%w: incomplete or malformed v2 envelope prefix", ErrInvalidEnvelope)
	}
	if strings.HasPrefix(cipherText, encryptedSecretPrefixV2) {
		trimmed := strings.TrimPrefix(cipherText, encryptedSecretPrefixV2)
		parts := strings.Split(trimmed, ":")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return "", fmt.Errorf("%w: expected enc:v2:<key_id>:<payload>", ErrInvalidEnvelope)
		}
		keyID := parts[0]
		rawPayload := parts[1]

		if !isValidKeyID(keyID) {
			return "", fmt.Errorf("%w: invalid key_id format %q", ErrInvalidKeyID, keyID)
		}

		aead, ok := c.keys[keyID]
		if !ok || aead == nil {
			return "", fmt.Errorf("unknown key_id %q in keyring", keyID)
		}

		payload, err := base64.RawURLEncoding.DecodeString(rawPayload)
		if err != nil {
			return "", fmt.Errorf("invalid base64 in v2 envelope: %w", err)
		}
		nonceSize := aead.NonceSize()
		if len(payload) <= nonceSize {
			return "", fmt.Errorf("invalid encrypted payload length in v2 envelope: too short")
		}
		nonce, sealed := payload[:nonceSize], payload[nonceSize:]
		plainText, err := aead.Open(nil, nonce, sealed, []byte(configID))
		if err != nil {
			return "", ErrDecryptionFailed
		}
		return string(plainText), nil
	}

	// 3. Unknown envelope version with enc: prefix
	if strings.HasPrefix(cipherText, "enc:") {
		return "", fmt.Errorf("%w: unrecognized envelope prefix", ErrUnsupportedVersion)
	}

	return "", errors.New("storage credential is not encrypted")
}

// ActiveKeyID returns the currently configured active write key ID.
func (c *SecretCipher) ActiveKeyID() string {
	if c == nil {
		return ""
	}
	return c.activeKeyID
}

// HasLegacyV1Key returns whether a legacy v1 decryption key is configured.
func (c *SecretCipher) HasLegacyV1Key() bool {
	return c != nil && c.legacyV1Key != nil
}

// KeyIDs returns a sorted list of all configured key IDs in the keyring.
func (c *SecretCipher) KeyIDs() []string {
	if c == nil {
		return nil
	}
	ids := make([]string, 0, len(c.keys))
	for id := range c.keys {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// HasKey checks if a specific key_id is present in the keyring.
func (c *SecretCipher) HasKey(keyID string) bool {
	if c == nil {
		return false
	}
	_, ok := c.keys[keyID]
	return ok
}

// EnvelopeMetadata holds parsed metadata from an encrypted secret envelope.
type EnvelopeMetadata struct {
	Version int
	KeyID   string
}

// InspectEnvelope parses envelope version and key ID without decrypting the payload.
func InspectEnvelope(cipherText string) (EnvelopeMetadata, error) {
	trimmed := strings.TrimSpace(cipherText)
	if trimmed == "" {
		return EnvelopeMetadata{}, errors.New("empty ciphertext")
	}
	if strings.HasPrefix(trimmed, encryptedSecretPrefixV1) {
		return EnvelopeMetadata{Version: 1, KeyID: ""}, nil
	}
	if strings.HasPrefix(trimmed, "enc:v2") && !strings.HasPrefix(trimmed, encryptedSecretPrefixV2) {
		return EnvelopeMetadata{}, fmt.Errorf("%w: incomplete or malformed v2 envelope prefix", ErrInvalidEnvelope)
	}
	if strings.HasPrefix(trimmed, encryptedSecretPrefixV2) {
		rest := strings.TrimPrefix(trimmed, encryptedSecretPrefixV2)
		parts := strings.Split(rest, ":")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return EnvelopeMetadata{}, fmt.Errorf("%w: expected enc:v2:<key_id>:<payload>", ErrInvalidEnvelope)
		}
		if !isValidKeyID(parts[0]) {
			return EnvelopeMetadata{}, fmt.Errorf("%w: invalid key_id format %q", ErrInvalidKeyID, parts[0])
		}
		return EnvelopeMetadata{Version: 2, KeyID: parts[0]}, nil
	}
	if strings.HasPrefix(trimmed, "enc:") {
		return EnvelopeMetadata{}, ErrUnsupportedVersion
	}
	return EnvelopeMetadata{}, errors.New("not an encrypted envelope")
}

// ResolveKeyringConfig resolves a KeyringConfig from multiple configuration sources.
// - primaryKey: traditional single-value key (e.g. STORAGE_MASTER_KEY)
// - activeKeyID: explicit active key ID (e.g. STORAGE_MASTER_KEY_ACTIVE)
// - legacyV1Key: explicit legacy v1 decryption key (e.g. STORAGE_MASTER_KEY_LEGACY)
// - keyringJSON: JSON string containing KeyringConfig (e.g. STORAGE_MASTER_KEY_RING)
// - extraKeys: discrete key definitions (e.g. from individual secret mounts)
func ResolveKeyringConfig(primaryKey, activeKeyID, legacyV1Key, keyringJSON string, extraKeys map[string]string) (KeyringConfig, error) {
	cfg := KeyringConfig{
		Keys: make(map[string]string),
	}

	// 1. Parse base from JSON if provided
	if strings.TrimSpace(keyringJSON) != "" {
		if err := json.Unmarshal([]byte(keyringJSON), &cfg); err != nil {
			return KeyringConfig{}, fmt.Errorf("invalid keyring JSON: %w", err)
		}
		if cfg.Keys == nil {
			cfg.Keys = make(map[string]string)
		}
	}

	// 2. Merge discrete extraKeys
	for kid, ksecret := range extraKeys {
		if strings.TrimSpace(kid) != "" && strings.TrimSpace(ksecret) != "" {
			cfg.Keys[strings.TrimSpace(kid)] = strings.TrimSpace(ksecret)
		}
	}

	// 3. Apply primaryKey fallback if keys is still empty
	primaryTrimmed := strings.TrimSpace(primaryKey)
	if primaryTrimmed != "" {
		if len(cfg.Keys) == 0 {
			cfg.Keys["k1"] = primaryTrimmed
			if cfg.ActiveKeyID == "" {
				cfg.ActiveKeyID = "k1"
			}
			if cfg.LegacyV1Key == "" {
				cfg.LegacyV1Key = primaryTrimmed
			}
		} else if _, exists := cfg.Keys["k1"]; !exists {
			cfg.Keys["k1"] = primaryTrimmed
		}
	}

	// 4. Explicit activeKeyID override
	if strings.TrimSpace(activeKeyID) != "" {
		cfg.ActiveKeyID = strings.TrimSpace(activeKeyID)
	}

	// 5. Explicit legacyV1Key override
	if strings.TrimSpace(legacyV1Key) != "" {
		cfg.LegacyV1Key = strings.TrimSpace(legacyV1Key)
	}

	if cfg.ActiveKeyID == "" && len(cfg.Keys) > 0 {
		// If activeKeyID not set, pick "k1" if present, otherwise first sorted key
		if _, ok := cfg.Keys["k1"]; ok {
			cfg.ActiveKeyID = "k1"
		} else {
			keys := make([]string, 0, len(cfg.Keys))
			for k := range cfg.Keys {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			cfg.ActiveKeyID = keys[0]
		}
	}

	return cfg, nil
}
