package httpserver

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

func (a fileCenterAPI) initMultipartUpload(w http.ResponseWriter, r *http.Request) {
	access, _, err := a.identity(r, false)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	var request struct {
		fileUploadInitRequest
		PartSize           int64  `json:"partSize"`
		PartSizeSnake      int64  `json:"part_size"`
		IdempotencyKey     string `json:"idempotencyKey"`
		IdempotencyKeySnake string `json:"idempotency_key"`
	}
	if err = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&request); err != nil {
		writeStorageError(w, err)
		return
	}
	fileSize := request.FileSize
	if fileSize == 0 {
		fileSize = request.FileSizeCamel
	}
	expiresAt := request.ExpiresAt
	if expiresAt == nil {
		expiresAt = request.ExpiresAtCamel
	}
	partSize := request.PartSize
	if partSize == 0 {
		partSize = request.PartSizeSnake
	}
	idempotencyKey := firstNonEmptyString(request.IdempotencyKey, request.IdempotencyKeySnake, strings.TrimSpace(r.Header.Get("Idempotency-Key")))
	session, err := a.service.InitMultipartUpload(r.Context(), storagecenter.MultipartInitInput{
		UploadInitInput: storagecenter.UploadInitInput{
			TenantID: access.TenantID, UserID: access.UserID,
			FileName: firstNonEmptyString(request.FileName, request.FileNameCamel), FileSize: fileSize,
			MIMEType:     firstNonEmptyString(request.MIMEType, request.MIMETypeCamel),
			BusinessType: firstNonEmptyString(request.BusinessType, request.BusinessTypeCamel),
			BusinessID:   firstNonEmptyString(request.BusinessID, request.BusinessIDCamel), Visibility: request.Visibility,
			IsTemporary: request.IsTemporary || request.IsTemporaryCamel, ExpiresAt: expiresAt,
			StorageConfigID: firstNonEmptyString(request.StorageConfigID, request.StorageConfigIDCamel),
		},
		PartSize: partSize, IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		writeStorageError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]any{
		"uploadId": session.UploadID, "upload_id": session.UploadID,
		"fileId": session.FileID, "file_id": session.FileID,
		"objectKey": session.ObjectKey, "object_key": session.ObjectKey,
		"partSize": session.PartSize, "part_size": session.PartSize,
		"totalParts": session.TotalParts, "total_parts": session.TotalParts,
		"expiresIn": session.ExpiresIn, "expires_in": session.ExpiresIn,
		"expiresAt": session.ExpiresAt, "expires_at": session.ExpiresAt,
	})
}

func (a fileCenterAPI) presignMultipartPart(w http.ResponseWriter, r *http.Request) {
	access, _, err := a.identity(r, false)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	uploadID := strings.TrimSpace(r.PathValue("uploadId"))
	partNumber, err := strconv.Atoi(strings.TrimSpace(r.PathValue("partNumber")))
	if err != nil {
		writeStorageError(w, storagecenter.ErrInvalidMultipartPart)
		return
	}
	ticket, err := a.service.PresignMultipartPart(r.Context(), access, uploadID, partNumber)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	writeJSON(w, map[string]any{
		"partNumber": ticket.PartNumber, "part_number": ticket.PartNumber,
		"uploadUrl": ticket.UploadURL, "upload_url": ticket.UploadURL,
		"headers": ticket.Headers,
		"expiresIn": ticket.ExpiresIn, "expires_in": ticket.ExpiresIn,
	})
}

func (a fileCenterAPI) completeMultipartUpload(w http.ResponseWriter, r *http.Request) {
	access, _, err := a.identity(r, false)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	var request struct {
		Parts []struct {
			PartNumber      int    `json:"partNumber"`
			PartNumberSnake int    `json:"part_number"`
			ETag            string `json:"etag"`
			ETagCamel       string `json:"eTag"`
			SizeBytes       int64  `json:"sizeBytes"`
			SizeBytesSnake  int64  `json:"size_bytes"`
		} `json:"parts"`
	}
	if err = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&request); err != nil {
		writeStorageError(w, err)
		return
	}
	parts := make([]storagecenter.CompletedPart, 0, len(request.Parts))
	for _, part := range request.Parts {
		partNumber := part.PartNumber
		if partNumber == 0 {
			partNumber = part.PartNumberSnake
		}
		parts = append(parts, storagecenter.CompletedPart{
			PartNumber: partNumber,
			ETag:       firstNonEmptyString(part.ETag, part.ETagCamel),
			SizeBytes:  firstNonZeroInt64(part.SizeBytes, part.SizeBytesSnake),
		})
	}
	file, err := a.service.CompleteMultipartUpload(r.Context(), access, strings.TrimSpace(r.PathValue("uploadId")), parts)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	writeJSON(w, map[string]any{"file": file})
}

func (a fileCenterAPI) abortMultipartUpload(w http.ResponseWriter, r *http.Request) {
	access, _, err := a.identity(r, false)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	if err := a.service.AbortMultipartUpload(r.Context(), access, strings.TrimSpace(r.PathValue("uploadId"))); err != nil {
		writeStorageError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func firstNonZeroInt64(values ...int64) int64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}
