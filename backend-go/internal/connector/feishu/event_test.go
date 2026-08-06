package feishu

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/connector"
)

type countedFileSource struct {
	reader *bytes.Reader
	read   atomic.Int64
}

func newCountedFileSource(raw []byte) *countedFileSource {
	return &countedFileSource{reader: bytes.NewReader(raw)}
}

func (r *countedFileSource) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.read.Add(int64(n))
	return n, err
}

func privateMultipartTempPresent(t *testing.T, dir string) bool {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Errorf("read multipart temp dir: %v", err)
		return false
	}
	if len(entries) != 1 {
		t.Errorf("multipart temp entries=%d, want 1", len(entries))
		return false
	}
	info, err := entries[0].Info()
	if err != nil {
		t.Errorf("stat multipart temp: %v", err)
		return false
	}
	if got := info.Mode().Perm(); runtime.GOOS != "windows" && got != 0o600 {
		t.Errorf("multipart temp permissions=%#o, want 0600", got)
		return false
	}
	return true
}

func requireMultipartTempRemoved(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("multipart temp files remain: %v", entries)
	}
}

func TestEncryptedEventVerification(t *testing.T) {
	encryptKey := "event-encrypt-key"
	plain := []byte(`{"token":"verify","type":"url_verification","challenge":"encrypted-ok"}`)
	encrypted := encryptEventForTest(t, plain, encryptKey)
	body, err := json.Marshal(map[string]string{"encrypt": encrypted})
	if err != nil {
		t.Fatal(err)
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := "nonce-1"
	digest := sha256.Sum256([]byte(timestamp + nonce + encryptKey + string(body)))
	headers := http.Header{}
	headers.Set("X-Lark-Request-Timestamp", timestamp)
	headers.Set("X-Lark-Request-Nonce", nonce)
	headers.Set("X-Lark-Signature", hex.EncodeToString(digest[:]))
	client := New(Config{VerificationToken: "verify", EncryptKey: encryptKey})
	verified, err := client.VerifyEvent(context.Background(), connector.EventRequest{Body: body, Headers: headers})
	if err != nil || !bytes.Equal(verified, plain) {
		t.Fatalf("verified=%s err=%v", verified, err)
	}
	parsed, err := client.ParseEvent(context.Background(), verified)
	if err != nil || parsed.Challenge != "encrypted-ok" {
		t.Fatalf("parsed=%+v err=%v", parsed, err)
	}
	headers.Set("X-Lark-Signature", strings.Repeat("0", 64))
	if _, err := client.VerifyEvent(context.Background(), connector.EventRequest{Body: body, Headers: headers}); !errors.Is(err, ErrInvalidEventSignature) {
		t.Fatalf("invalid signature error=%v", err)
	}
}

func encryptEventForTest(t *testing.T, plain []byte, encryptKey string) string {
	t.Helper()
	key := sha256.Sum256([]byte(encryptKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		t.Fatal(err)
	}
	padding := aes.BlockSize - len(plain)%aes.BlockSize
	padded := append(append([]byte{}, plain...), bytes.Repeat([]byte{byte(padding)}, padding)...)
	encrypted := make([]byte, len(padded))
	var mode cipher.BlockMode = cipher.NewCBCEncrypter(block, key[:aes.BlockSize])
	mode.CryptBlocks(encrypted, padded)
	return base64.StdEncoding.EncodeToString(encrypted)
}

func TestChallengeAndTextEvent(t *testing.T) {
	client := New(Config{VerificationToken: "verify"})
	challenge := []byte(`{"token":"verify","type":"url_verification","challenge":"ok"}`)
	verified, err := client.VerifyEvent(context.Background(), connector.EventRequest{Body: challenge, Headers: http.Header{}})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := client.ParseEvent(context.Background(), verified)
	if err != nil || parsed.Challenge != "ok" {
		t.Fatalf("challenge parsed=%#v err=%v", parsed, err)
	}

	body := map[string]any{
		"schema": "2.0",
		"header": map[string]any{"event_id": "evt-1", "event_type": "im.message.receive_v1", "token": "verify", "tenant_key": "tenant-key"},
		"event": map[string]any{
			"sender":  map[string]any{"sender_id": map[string]any{"open_id": "ou-1", "union_id": "on-1"}},
			"message": map[string]any{"message_id": "om-1", "chat_id": "oc-1", "chat_type": "p2p", "message_type": "text", "content": `{"text":"生成 iPhone 17 的电商图"}`},
		},
	}
	event := body["event"].(map[string]any)
	message := event["message"].(map[string]any)
	message["chat_type"] = "group"
	message["mentions"] = []any{map[string]any{"id": map[string]any{"open_id": "ou-bot"}, "name": "bot"}}
	raw, _ := json.Marshal(body)
	verified, err = client.VerifyEvent(context.Background(), connector.EventRequest{Body: raw, Headers: http.Header{}})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err = client.ParseEvent(context.Background(), verified)
	if err != nil || parsed.Message == nil || parsed.Message.ExternalMessageID != "om-1" || !strings.Contains(parsed.Message.Text, "iPhone 17") {
		t.Fatalf("message parsed=%#v err=%v", parsed, err)
	}
	if len(parsed.Message.MentionOpenIDs) != 1 || parsed.Message.MentionOpenIDs[0] != "ou-bot" {
		t.Fatalf("mention open ids=%v", parsed.Message.MentionOpenIDs)
	}
}

func TestBotInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/auth/v3/tenant_access_token/internal":
			_, _ = w.Write([]byte(`{"code":0,"tenant_access_token":"token","expire":3600}`))
		case "/bot/v3/info":
			if r.Header.Get("Authorization") != "Bearer token" {
				t.Errorf("authorization=%q", r.Header.Get("Authorization"))
			}
			_, _ = w.Write([]byte(`{"code":0,"msg":"success","bot":{"app_name":"Test Bot","open_id":"ou-bot"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := New(Config{AppID: "app", AppSecret: "secret", BaseURL: server.URL, HTTPClient: server.Client()})
	info, err := client.BotInfo(context.Background())
	if err != nil || info.OpenID != "ou-bot" || info.AppName != "Test Bot" {
		t.Fatalf("bot info=%+v err=%v", info, err)
	}
}

func TestTokenCacheAndRetry(t *testing.T) {
	var tokenCalls atomic.Int32
	var messageCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/auth/v3/tenant_access_token/internal":
			tokenCalls.Add(1)
			_, _ = w.Write([]byte(`{"code":0,"tenant_access_token":"token","expire":3600}`))
		case "/im/v1/messages":
			if messageCalls.Add(1) == 1 {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"code":500,"msg":"temporary"}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"message_id":"om-out"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := New(Config{AppID: "app", AppSecret: "secret", VerificationToken: "verify", BaseURL: server.URL, Retries: 1, HTTPClient: server.Client()})
	for range 2 {
		result, err := client.SendText(context.Background(), connector.MessageTarget{ChatID: "chat"}, connector.OutgoingMessage{Text: "ok"})
		if err != nil || result.ExternalMessageID != "om-out" {
			t.Fatalf("send result=%#v err=%v", result, err)
		}
	}
	if tokenCalls.Load() != 1 || messageCalls.Load() != 3 {
		t.Fatalf("token calls=%d message calls=%d", tokenCalls.Load(), messageCalls.Load())
	}
}

func TestSendFileUploadsPPTAndSendsMessage(t *testing.T) {
	var uploaded, sent bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/auth/v3/tenant_access_token/internal":
			_, _ = w.Write([]byte(`{"code":0,"tenant_access_token":"token","expire":3600}`))
		case "/im/v1/files":
			if err := r.ParseMultipartForm(2 << 20); err != nil {
				t.Fatal(err)
			}
			if r.FormValue("file_type") != "ppt" || r.FormValue("file_name") != "deck.pptx" {
				t.Fatalf("file form type=%q name=%q", r.FormValue("file_type"), r.FormValue("file_name"))
			}
			uploaded = true
			_, _ = w.Write([]byte(`{"code":0,"data":{"file_key":"file-key"}}`))
		case "/im/v1/messages":
			if r.URL.Query().Get("receive_id_type") != "chat_id" {
				t.Fatalf("receive id type=%q", r.URL.Query().Get("receive_id_type"))
			}
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["msg_type"] != "file" || !strings.Contains(payload["content"], "file-key") {
				t.Fatalf("message payload=%v", payload)
			}
			sent = true
			_, _ = w.Write([]byte(`{"code":0,"data":{"message_id":"om-file"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := New(Config{AppID: "app", AppSecret: "secret", BaseURL: server.URL, HTTPClient: server.Client()})
	result, err := client.SendFile(context.Background(), connector.MessageTarget{ChatID: "chat"}, connector.OutgoingMessage{
		File: bytes.NewReader([]byte("pptx")), FileName: "deck.pptx", MIMEType: "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	})
	if err != nil || result.ExternalMessageID != "om-file" || !uploaded || !sent {
		t.Fatalf("result=%#v uploaded=%v sent=%v err=%v", result, uploaded, sent, err)
	}
}

func TestSendFileSpoolsPrivateMultipartAndReplaysAfterTokenRefreshAndRetry(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("TMPDIR", tempDir)
	t.Setenv("TMP", tempDir)
	t.Setenv("TEMP", tempDir)
	payload := []byte("one-pass-private-video")
	source := newCountedFileSource(payload)
	var tokenCalls atomic.Int32
	var uploadCalls atomic.Int32
	var sawPrivateTemp atomic.Bool
	var firstBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/auth/v3/tenant_access_token/internal":
			call := tokenCalls.Add(1)
			_, _ = w.Write([]byte(`{"code":0,"tenant_access_token":"token-` + strconv.Itoa(int(call)) + `","expire":3600}`))
		case "/im/v1/files":
			if privateMultipartTempPresent(t, tempDir) {
				sawPrivateTemp.Store(true)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read upload body: %v", err)
				return
			}
			call := uploadCalls.Add(1)
			if call == 1 {
				firstBody = append([]byte(nil), body...)
			} else if !bytes.Equal(body, firstBody) {
				t.Errorf("upload body %d was not fully replayed", call)
			}
			if !bytes.Contains(body, payload) {
				t.Errorf("upload body %d missing source payload", call)
			}
			switch call {
			case 1:
				if r.Header.Get("Authorization") != "Bearer token-1" {
					t.Errorf("first authorization=%q", r.Header.Get("Authorization"))
				}
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"code":99991663,"msg":"token expired"}`))
			case 2:
				if r.Header.Get("Authorization") != "Bearer token-2" {
					t.Errorf("refreshed authorization=%q", r.Header.Get("Authorization"))
				}
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"code":500,"msg":"temporary"}`))
			default:
				_, _ = w.Write([]byte(`{"code":0,"data":{"file_key":"file-key"}}`))
			}
		case "/im/v1/messages":
			_, _ = w.Write([]byte(`{"code":0,"data":{"message_id":"om-file"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := New(Config{AppID: "app", AppSecret: "secret", BaseURL: server.URL, Retries: 1, HTTPClient: server.Client()})
	result, err := client.SendFile(context.Background(), connector.MessageTarget{ChatID: "chat"}, connector.OutgoingMessage{
		File: source, FileName: "video.mp4", MIMEType: "video/mp4",
	})
	if err != nil || result.ExternalMessageID != "om-file" {
		t.Fatalf("send result=%#v err=%v", result, err)
	}
	if tokenCalls.Load() != 2 || uploadCalls.Load() != 3 {
		t.Fatalf("token calls=%d upload calls=%d", tokenCalls.Load(), uploadCalls.Load())
	}
	if source.read.Load() != int64(len(payload)) {
		t.Fatalf("source bytes read=%d, want %d", source.read.Load(), len(payload))
	}
	if !sawPrivateTemp.Load() {
		t.Fatal("upload did not use a private file-backed multipart body")
	}
	requireMultipartTempRemoved(t, tempDir)
}

func TestSendFileRemovesMultipartTempAfterFailure(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("TMPDIR", tempDir)
	t.Setenv("TMP", tempDir)
	t.Setenv("TEMP", tempDir)
	var sawPrivateTemp atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/auth/v3/tenant_access_token/internal" {
			_, _ = w.Write([]byte(`{"code":0,"tenant_access_token":"token","expire":3600}`))
			return
		}
		if privateMultipartTempPresent(t, tempDir) {
			sawPrivateTemp.Store(true)
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"code":500,"msg":"upload failed"}`))
	}))
	defer server.Close()
	client := New(Config{AppID: "app", AppSecret: "secret", BaseURL: server.URL, HTTPClient: server.Client()})
	_, err := client.SendFile(context.Background(), connector.MessageTarget{ChatID: "chat"}, connector.OutgoingMessage{
		File: bytes.NewReader([]byte("video")), FileName: "video.mp4", MIMEType: "video/mp4",
	})
	if err == nil || !strings.Contains(err.Error(), "upload failed") {
		t.Fatalf("send error=%v, want upload failure", err)
	}
	if !sawPrivateTemp.Load() {
		t.Fatal("failed upload did not use a private file-backed multipart body")
	}
	requireMultipartTempRemoved(t, tempDir)
}

func TestSendFileRemovesMultipartTempAfterCancellation(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("TMPDIR", tempDir)
	t.Setenv("TMP", tempDir)
	t.Setenv("TEMP", tempDir)
	requestEntered := make(chan struct{})
	releaseRequest := make(chan struct{})
	var sawPrivateTemp atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/auth/v3/tenant_access_token/internal" {
			_, _ = w.Write([]byte(`{"code":0,"tenant_access_token":"token","expire":3600}`))
			return
		}
		if privateMultipartTempPresent(t, tempDir) {
			sawPrivateTemp.Store(true)
		}
		close(requestEntered)
		<-releaseRequest
	}))
	defer server.Close()
	client := New(Config{AppID: "app", AppSecret: "secret", BaseURL: server.URL, HTTPClient: server.Client()})
	ctx, cancel := context.WithCancel(context.Background())
	resultCh := make(chan error, 1)
	go func() {
		_, err := client.SendFile(ctx, connector.MessageTarget{ChatID: "chat"}, connector.OutgoingMessage{
			File: bytes.NewReader([]byte("video")), FileName: "video.mp4", MIMEType: "video/mp4",
		})
		resultCh <- err
	}()
	select {
	case <-requestEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("upload request did not start")
	}
	cancel()
	close(releaseRequest)
	select {
	case err := <-resultCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("send error=%v, want context canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled upload did not return")
	}
	if !sawPrivateTemp.Load() {
		t.Fatal("cancelled upload did not use a private file-backed multipart body")
	}
	requireMultipartTempRemoved(t, tempDir)
}
