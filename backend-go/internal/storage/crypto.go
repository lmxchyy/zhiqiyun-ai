package storage

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

const encryptedSecretPrefix = "enc:v1:"

type SecretCipher struct {
	aead cipher.AEAD
}

func NewSecretCipher(masterKey string) (*SecretCipher, error) {
	raw := []byte(strings.TrimSpace(masterKey))
	if decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(masterKey)); err == nil && len(decoded) >= 32 {
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
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &SecretCipher{aead: aead}, nil
}

func (c *SecretCipher) Encrypt(plainText string, configID string) (string, error) {
	if strings.TrimSpace(plainText) == "" {
		return "", nil
	}
	if c == nil || c.aead == nil {
		return "", ErrSecretCipherRequired
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := c.aead.Seal(nil, nonce, []byte(plainText), []byte(configID))
	payload := append(nonce, sealed...)
	return encryptedSecretPrefix + base64.RawURLEncoding.EncodeToString(payload), nil
}

func (c *SecretCipher) Decrypt(cipherText string, configID string) (string, error) {
	if strings.TrimSpace(cipherText) == "" {
		return "", nil
	}
	if !strings.HasPrefix(cipherText, encryptedSecretPrefix) {
		return "", fmt.Errorf("storage credential is not encrypted")
	}
	if c == nil || c.aead == nil {
		return "", ErrSecretCipherRequired
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(cipherText, encryptedSecretPrefix))
	if err != nil {
		return "", err
	}
	if len(payload) <= c.aead.NonceSize() {
		return "", fmt.Errorf("invalid encrypted storage credential")
	}
	nonce, sealed := payload[:c.aead.NonceSize()], payload[c.aead.NonceSize():]
	plainText, err := c.aead.Open(nil, nonce, sealed, []byte(configID))
	if err != nil {
		return "", err
	}
	return string(plainText), nil
}
