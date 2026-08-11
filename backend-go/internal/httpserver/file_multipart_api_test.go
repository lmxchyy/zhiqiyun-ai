package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

func TestMultipartUploadHTTPLifecycle(t *testing.T) {
	const totalSize int64 = 12 << 20
	ctx := context.Background()
	store := newJSONStore(filepath.Join(t.TempDir(), "platform.json"))
	user, err := store.CreateAdminCustomer(adminCustomerMutation{Name: "MPU User", Email: "mpu@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	sessions := newLocalAuthSessions()
	if err := sessions.Put(ctx, "mpu-token", user.ID, authSessionTTL); err != nil {
		t.Fatal(err)
	}

	repo := storagecenter.NewMemoryRepository()
	provider := &generatedStorageTestProvider{objects: map[string]storagecenter.ObjectMetadata{}}
	service := storagecenter.NewService(repo, generatedStorageTestFactory{provider: provider}, storagecenter.Options{
		DefaultProvider: "s3", Endpoint: "https://storage.example", AccessKey: "access", SecretKey: "secret", Bucket: "files",
		DefaultQuotaBytes: totalSize * 2, MaxUploadBytes: totalSize * 2, UploadURLTTL: time.Hour,
		MasterKey: "0123456789abcdef0123456789abcdef",
	})
	api := newFileCenterAPI(service, store, sessions)

	initBody, _ := json.Marshal(map[string]any{
		"fileName": "clip.mp4", "fileSize": totalSize, "mimeType": "video/mp4", "businessType": "smart_video",
	})
	initReq := httptest.NewRequest(http.MethodPost, "/api/v1/files/upload/multipart/init", bytes.NewReader(initBody))
	initReq.Header.Set("Authorization", "Bearer mpu-token")
	initReq.Header.Set("Idempotency-Key", "http-mpu-1")
	initReq.Header.Set("Content-Type", "application/json")
	initRec := httptest.NewRecorder()
	api.initMultipartUpload(initRec, initReq)
	if initRec.Code != http.StatusCreated {
		t.Fatalf("init status=%d body=%s", initRec.Code, initRec.Body.String())
	}
	var session struct {
		UploadID   string `json:"uploadId"`
		FileID     string `json:"fileId"`
		TotalParts int    `json:"totalParts"`
		PartSize   int64  `json:"partSize"`
	}
	if err := json.Unmarshal(initRec.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if session.UploadID == "" || session.TotalParts != 2 {
		t.Fatalf("unexpected session: %+v", session)
	}

	partReq := httptest.NewRequest(http.MethodPost, "/api/v1/files/upload/multipart/"+session.UploadID+"/parts/1", nil)
	partReq.Header.Set("Authorization", "Bearer mpu-token")
	partReq.SetPathValue("uploadId", session.UploadID)
	partReq.SetPathValue("partNumber", "1")
	partRec := httptest.NewRecorder()
	api.presignMultipartPart(partRec, partReq)
	if partRec.Code != http.StatusOK {
		t.Fatalf("presign status=%d body=%s", partRec.Code, partRec.Body.String())
	}
	var ticket struct {
		UploadURL string `json:"uploadUrl"`
	}
	if err := json.Unmarshal(partRec.Body.Bytes(), &ticket); err != nil || ticket.UploadURL == "" {
		t.Fatalf("presign ticket=%+v err=%v body=%s", ticket, err, partRec.Body.String())
	}

	completeBody, _ := json.Marshal(map[string]any{
		"parts": []map[string]any{
			{"partNumber": 1, "etag": "etag-1"},
			{"partNumber": 2, "etag": "etag-2"},
		},
	})
	completeReq := httptest.NewRequest(http.MethodPost, "/api/v1/files/upload/multipart/"+session.UploadID+"/complete", bytes.NewReader(completeBody))
	completeReq.Header.Set("Authorization", "Bearer mpu-token")
	completeReq.Header.Set("Content-Type", "application/json")
	completeReq.SetPathValue("uploadId", session.UploadID)
	completeRec := httptest.NewRecorder()
	api.completeMultipartUpload(completeRec, completeReq)
	if completeRec.Code != http.StatusOK {
		t.Fatalf("complete status=%d body=%s", completeRec.Code, completeRec.Body.String())
	}
	var completed struct {
		File storagecenter.FileObject `json:"file"`
	}
	if err := json.Unmarshal(completeRec.Body.Bytes(), &completed); err != nil {
		t.Fatal(err)
	}
	if completed.File.Status != storagecenter.StatusActive || completed.File.FileSize != totalSize || completed.File.FileID != session.FileID {
		t.Fatalf("unexpected completed file: %+v", completed.File)
	}
}

func TestMultipartAbortHTTP(t *testing.T) {
	const totalSize int64 = storagecenter.MinMultipartPartSize
	ctx := context.Background()
	store := newJSONStore(filepath.Join(t.TempDir(), "platform.json"))
	user, err := store.CreateAdminCustomer(adminCustomerMutation{Name: "MPU Abort", Email: "mpu-abort@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	sessions := newLocalAuthSessions()
	if err := sessions.Put(ctx, "mpu-abort-token", user.ID, authSessionTTL); err != nil {
		t.Fatal(err)
	}
	repo := storagecenter.NewMemoryRepository()
	provider := &generatedStorageTestProvider{objects: map[string]storagecenter.ObjectMetadata{}}
	service := storagecenter.NewService(repo, generatedStorageTestFactory{provider: provider}, storagecenter.Options{
		DefaultProvider: "s3", Endpoint: "https://storage.example", AccessKey: "access", SecretKey: "secret", Bucket: "files",
		DefaultQuotaBytes: totalSize * 2, MaxUploadBytes: totalSize * 2, UploadURLTTL: time.Minute,
		MasterKey: "0123456789abcdef0123456789abcdef",
	})
	api := newFileCenterAPI(service, store, sessions)

	initBody, _ := json.Marshal(map[string]any{
		"fileName": "clip.mp4", "fileSize": totalSize, "mimeType": "video/mp4",
		"partSize": storagecenter.MinMultipartPartSize,
	})
	initReq := httptest.NewRequest(http.MethodPost, "/api/v1/files/upload/multipart/init", bytes.NewReader(initBody))
	initReq.Header.Set("Authorization", "Bearer mpu-abort-token")
	initReq.Header.Set("Content-Type", "application/json")
	initRec := httptest.NewRecorder()
	api.initMultipartUpload(initRec, initReq)
	if initRec.Code != http.StatusCreated {
		t.Fatalf("init status=%d body=%s", initRec.Code, initRec.Body.String())
	}
	var session struct {
		UploadID string `json:"uploadId"`
	}
	if err := json.Unmarshal(initRec.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}

	abortReq := httptest.NewRequest(http.MethodPost, "/api/v1/files/upload/multipart/"+session.UploadID+"/abort", nil)
	abortReq.Header.Set("Authorization", "Bearer mpu-abort-token")
	abortReq.SetPathValue("uploadId", session.UploadID)
	abortRec := httptest.NewRecorder()
	api.abortMultipartUpload(abortRec, abortReq)
	if abortRec.Code != http.StatusNoContent {
		t.Fatalf("abort status=%d body=%s", abortRec.Code, abortRec.Body.String())
	}
}
