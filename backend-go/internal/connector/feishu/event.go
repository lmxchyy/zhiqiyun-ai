package feishu

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"xianzhi-ai/backend-go/internal/connector"
)

var (
	ErrInvalidEventSignature = errors.New("invalid Feishu event signature")
	ErrInvalidEventToken     = errors.New("invalid Feishu verification token")
	ErrUnsupportedMessage    = errors.New("unsupported Feishu message")
)

type eventEnvelope struct {
	Schema    string          `json:"schema"`
	Encrypt   string          `json:"encrypt"`
	Token     string          `json:"token"`
	Type      string          `json:"type"`
	Challenge string          `json:"challenge"`
	Header    eventHeader     `json:"header"`
	Event     json.RawMessage `json:"event"`
}

type eventHeader struct {
	EventID   string `json:"event_id"`
	EventType string `json:"event_type"`
	Token     string `json:"token"`
	TenantKey string `json:"tenant_key"`
}

type messageEvent struct {
	Sender struct {
		SenderID struct {
			OpenID  string `json:"open_id"`
			UnionID string `json:"union_id"`
			UserID  string `json:"user_id"`
		} `json:"sender_id"`
		TenantKey string `json:"tenant_key"`
	} `json:"sender"`
	Message struct {
		MessageID   string `json:"message_id"`
		ChatID      string `json:"chat_id"`
		ChatType    string `json:"chat_type"`
		MessageType string `json:"message_type"`
		Content     string `json:"content"`
		Mentions    []struct {
			ID struct {
				OpenID string `json:"open_id"`
			} `json:"id"`
			Name string `json:"name"`
		} `json:"mentions"`
	} `json:"message"`
}

func (c *Client) VerifyEvent(_ context.Context, request connector.EventRequest) ([]byte, error) {
	var envelope eventEnvelope
	if err := json.Unmarshal(request.Body, &envelope); err != nil {
		return nil, fmt.Errorf("decode Feishu event: %w", err)
	}
	body := request.Body
	if strings.TrimSpace(envelope.Encrypt) != "" {
		if strings.TrimSpace(c.encryptKey) == "" {
			return nil, errors.New("encrypted Feishu event received without Encrypt Key")
		}
		if err := verifySignature(request, c.encryptKey); err != nil {
			return nil, err
		}
		plain, err := decryptEvent(envelope.Encrypt, c.encryptKey)
		if err != nil {
			return nil, err
		}
		body = plain
		if err := json.Unmarshal(body, &envelope); err != nil {
			return nil, fmt.Errorf("decode decrypted Feishu event: %w", err)
		}
	}
	token := strings.TrimSpace(envelope.Header.Token)
	if token == "" {
		token = strings.TrimSpace(envelope.Token)
	}
	if !secureEqual(token, c.verificationToken) {
		return nil, ErrInvalidEventToken
	}
	return body, nil
}

func (c *Client) ParseEvent(_ context.Context, body []byte) (connector.ParsedEvent, error) {
	var envelope eventEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return connector.ParsedEvent{}, fmt.Errorf("decode Feishu event: %w", err)
	}
	if envelope.Challenge != "" || strings.EqualFold(envelope.Type, "url_verification") {
		return connector.ParsedEvent{Challenge: envelope.Challenge, EventID: envelope.Header.EventID}, nil
	}
	if envelope.Header.EventType != "im.message.receive_v1" {
		return connector.ParsedEvent{EventID: envelope.Header.EventID}, ErrUnsupportedMessage
	}
	var event messageEvent
	if err := json.Unmarshal(envelope.Event, &event); err != nil {
		return connector.ParsedEvent{}, fmt.Errorf("decode Feishu message event: %w", err)
	}
	if event.Message.MessageType != "text" {
		return connector.ParsedEvent{EventID: envelope.Header.EventID}, ErrUnsupportedMessage
	}
	var content struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(event.Message.Content), &content); err != nil {
		return connector.ParsedEvent{}, fmt.Errorf("decode Feishu text content: %w", err)
	}
	externalUserID := firstNonEmpty(event.Sender.SenderID.OpenID, event.Sender.SenderID.UserID)
	if event.Message.MessageID == "" || event.Message.ChatID == "" || externalUserID == "" {
		return connector.ParsedEvent{}, errors.New("Feishu event is missing message, chat, or sender id")
	}
	message := &connector.IncomingMessage{
		Platform: "feishu", ExternalMessageID: event.Message.MessageID,
		ExternalChatID: event.Message.ChatID, ExternalUserID: externalUserID,
		ExternalUnionID:   event.Sender.SenderID.UnionID,
		ExternalTenantKey: firstNonEmpty(event.Sender.TenantKey, envelope.Header.TenantKey),
		ChatType:          event.Message.ChatType, MessageType: event.Message.MessageType,
		Text: strings.TrimSpace(content.Text),
	}
	for _, mention := range event.Message.Mentions {
		if openID := strings.TrimSpace(mention.ID.OpenID); openID != "" {
			message.MentionOpenIDs = append(message.MentionOpenIDs, openID)
		}
	}
	return connector.ParsedEvent{Message: message, EventID: envelope.Header.EventID}, nil
}

func verifySignature(request connector.EventRequest, encryptKey string) error {
	timestamp := strings.TrimSpace(request.Headers.Get("X-Lark-Request-Timestamp"))
	nonce := strings.TrimSpace(request.Headers.Get("X-Lark-Request-Nonce"))
	signature := strings.TrimSpace(request.Headers.Get("X-Lark-Signature"))
	if timestamp == "" || nonce == "" || signature == "" {
		return ErrInvalidEventSignature
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || time.Since(time.Unix(seconds, 0)).Abs() > 10*time.Minute {
		return ErrInvalidEventSignature
	}
	digest := sha256.Sum256([]byte(timestamp + nonce + encryptKey + string(request.Body)))
	expected := hex.EncodeToString(digest[:])
	if !secureEqual(signature, expected) {
		return ErrInvalidEventSignature
	}
	return nil
}

func decryptEvent(value string, encryptKey string) ([]byte, error) {
	encrypted, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("decode encrypted Feishu event: %w", err)
	}
	key := sha256.Sum256([]byte(encryptKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	if len(encrypted) == 0 || len(encrypted)%aes.BlockSize != 0 {
		return nil, errors.New("invalid encrypted Feishu event length")
	}
	plain := make([]byte, len(encrypted))
	var mode cipher.BlockMode = cipher.NewCBCDecrypter(block, key[:aes.BlockSize])
	mode.CryptBlocks(plain, encrypted)
	return unpadPKCS7(plain, aes.BlockSize)
}

func unpadPKCS7(value []byte, blockSize int) ([]byte, error) {
	if len(value) == 0 {
		return nil, errors.New("empty encrypted Feishu event")
	}
	padding := int(value[len(value)-1])
	if padding <= 0 || padding > blockSize || padding > len(value) {
		return nil, errors.New("invalid encrypted Feishu event padding")
	}
	if !bytes.Equal(value[len(value)-padding:], bytes.Repeat([]byte{byte(padding)}, padding)) {
		return nil, errors.New("invalid encrypted Feishu event padding")
	}
	return value[:len(value)-padding], nil
}

func secureEqual(left string, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if len(left) != len(right) || len(left) == 0 {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
