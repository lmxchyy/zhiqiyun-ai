package httpserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"
)

const (
	terminalMiniProgram = "miniprogram"
	auditPending        = "pending"
	auditApproved       = "approved"
	auditRejected       = "rejected"
	auditManualReview   = "manual_review"
)

var (
	errMiniProgramModelNotCompliant = errors.New("该模型尚未完成小程序上线合规审核")
	errOutputAuditRejected          = errors.New("生成结果未通过内容安全审核，暂不可展示、下载或分享")
	errLegalAcceptanceRequired      = errors.New("请先确认最新版本的用户协议、隐私政策和AI生成内容使用规范")
)

var requiredGenerationLegalCodes = []string{"user-agreement", "privacy-policy", "ai-content-rules"}

func applyAIModelComplianceMutation(item *adminAIModel, req adminAIModelMutation) {
	if item == nil {
		return
	}
	if value := strings.TrimSpace(req.ProviderName); value != "" {
		item.ProviderName = value
	}
	if value := strings.TrimSpace(req.ProviderCompany); value != "" {
		item.ProviderCompany = value
	}
	if value := strings.TrimSpace(req.AlgorithmName); value != "" {
		item.AlgorithmName = value
	}
	if value := strings.TrimSpace(req.AlgorithmFilingNo); value != "" {
		item.AlgorithmFilingNo = value
	}
	if value := strings.TrimSpace(req.AlgorithmType); value != "" {
		item.AlgorithmType = value
	}
	if value := strings.ToLower(strings.TrimSpace(req.ContractStatus)); value != "" {
		item.ContractStatus = value
	}
	if value := strings.TrimSpace(req.ContractExpireAt); value != "" {
		item.ContractExpireAt = value
	}
	if value := strings.ToLower(strings.TrimSpace(req.ComplianceStatus)); value != "" {
		item.ComplianceStatus = value
	}
	if req.AllowedTerminals != nil {
		item.AllowedTerminals = uniqueLowerStrings(req.AllowedTerminals)
	}
	if req.AllowedCapabilities != nil {
		item.AllowedCapabilities = uniqueLowerStrings(req.AllowedCapabilities)
	}
	if req.MiniProgramEnabled != nil {
		item.MiniProgramEnabled = *req.MiniProgramEnabled
	}
	if value := strings.TrimSpace(req.ComplianceRemark); value != "" {
		item.ComplianceRemark = value
	}
	if value := strings.TrimSpace(req.ModelVersion); value != "" {
		item.ModelVersion = value
	}
}

func uniqueLowerStrings(items []string) []string {
	result := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, value := range items {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func modelAllowedForMiniProgram(item adminAIModel, now time.Time) (bool, string) {
	if !strings.EqualFold(strings.TrimSpace(item.ComplianceStatus), "approved") {
		return false, "compliance_status_not_approved"
	}
	if !item.MiniProgramEnabled {
		return false, "miniprogram_disabled"
	}
	if !stringListContainsFold(item.AllowedTerminals, terminalMiniProgram) {
		return false, "terminal_not_allowed"
	}
	if !strings.EqualFold(strings.TrimSpace(item.ContractStatus), "valid") &&
		!strings.EqualFold(strings.TrimSpace(item.ContractStatus), "approved") &&
		!strings.EqualFold(strings.TrimSpace(item.ContractStatus), "active") {
		return false, "contract_not_valid"
	}
	if strings.TrimSpace(item.ProviderCompany) == "" || strings.TrimSpace(item.AlgorithmName) == "" || strings.TrimSpace(item.AlgorithmFilingNo) == "" {
		return false, "algorithm_filing_incomplete"
	}
	if isModelGatewayName(item.ProviderCompany) || isModelGatewayName(item.ProviderName) {
		return false, "gateway_cannot_be_filing_subject"
	}
	if expiry := strings.TrimSpace(item.ContractExpireAt); expiry != "" {
		parsed, err := parseComplianceTime(expiry)
		if err != nil || !parsed.After(now) {
			return false, "contract_expired"
		}
	} else {
		return false, "contract_expiry_missing"
	}
	return true, ""
}

func validateAIModelMiniProgramEnable(item adminAIModel) error {
	if !item.MiniProgramEnabled {
		return nil
	}
	if allowed, reason := modelAllowedForMiniProgram(item, time.Now().UTC()); !allowed {
		return fmt.Errorf("cannot enable model for mini-program: %s", reason)
	}
	return nil
}

func parseComplianceTime(value string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid compliance time %q", value)
}

func stringListContainsFold(items []string, target string) bool {
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), target) {
			return true
		}
	}
	return false
}

func requestTerminal(r *http.Request) string {
	if isWeChatMiniProgramRequest(r) {
		return terminalMiniProgram
	}
	platform := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Client-Platform")))
	switch platform {
	case "h5", "web", "pc", "desktop":
		return platform
	default:
		return "web"
	}
}

func findConfiguredAIModel(data adminPlatformData, modelName string) (adminAIModel, bool) {
	for _, item := range data.AIModels {
		if strings.EqualFold(strings.TrimSpace(item.ModelName), strings.TrimSpace(modelName)) {
			return item, true
		}
	}
	return adminAIModel{}, false
}

func enforceMiniProgramModelCompliance(data adminPlatformData, req *generation.CreateRequest) error {
	if req == nil || !strings.EqualFold(stringValue(req.Params["terminal"]), terminalMiniProgram) {
		return nil
	}
	mode := miniprogramCreationModeForTaskType(req.Type)
	if !stringListContainsFold(configuredMiniProgramCreationModes(), mode) {
		return fmt.Errorf("%w: terminal_capability_not_enabled", errMiniProgramModelNotCompliant)
	}
	model, found := findConfiguredAIModel(data, req.Model)
	if !found {
		return errMiniProgramModelNotCompliant
	}
	allowed, reason := modelAllowedForMiniProgram(model, time.Now().UTC())
	if !allowed {
		return fmt.Errorf("%w: %s", errMiniProgramModelNotCompliant, reason)
	}
	capability := strings.ToLower(strings.TrimSpace(model.ModelType))
	if isImageGenerationRequest(req.Type) {
		capability = "image"
	}
	if capability == "" || !stringListContainsFold(model.AllowedCapabilities, capability) {
		return fmt.Errorf("%w: capability_not_allowed", errMiniProgramModelNotCompliant)
	}
	req.Params["provider_name"] = model.ProviderName
	req.Params["provider_company"] = model.ProviderCompany
	req.Params["algorithm_name"] = model.AlgorithmName
	req.Params["algorithm_filing_no"] = model.AlgorithmFilingNo
	req.Params["algorithm_type"] = model.AlgorithmType
	req.Params["model_version"] = model.ModelVersion
	req.Params["configured_channel_id"] = configuredModelChannelID(model)
	req.Params["compliance_snapshot_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	return nil
}

func resolveMiniProgramCompliantModuleSchema(data adminPlatformData, user adminUser, moduleCode string, resolved resolvedModuleSchema) (resolvedModuleSchema, error) {
	moduleCode = canonicalModuleCode(moduleCode)
	expectedCapability := defaultAIModelTypeForModule(moduleCode)
	allowed, reason := modelAllowedForMiniProgram(resolved.Model, time.Now().UTC())
	if allowed && stringListContainsFold(resolved.Model.AllowedCapabilities, expectedCapability) {
		return resolved, nil
	}
	if allowed {
		reason = "capability_not_allowed"
	}

	candidates := append([]adminAIModel(nil), data.AIModels...)
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].SortWeight != candidates[j].SortWeight {
			return candidates[i].SortWeight > candidates[j].SortWeight
		}
		return strings.ToLower(candidates[i].ModelName) < strings.ToLower(candidates[j].ModelName)
	})
	for _, candidate := range candidates {
		if canonicalModuleCode(firstNonEmptyString(candidate.ModuleCode, candidate.ModuleCodeCamel)) != moduleCode ||
			strings.EqualFold(strings.TrimSpace(candidate.ModelName), strings.TrimSpace(resolved.Model.ModelName)) ||
			!isActiveLike(candidate.Status) {
			continue
		}
		if ok, _ := modelAllowedForMiniProgram(candidate, time.Now().UTC()); !ok || !stringListContainsFold(candidate.AllowedCapabilities, expectedCapability) {
			continue
		}
		fallback, err := resolveModuleSchema(data, user, moduleCode, candidate.ModelName)
		if err == nil {
			return fallback, nil
		}
	}
	if reason == "" {
		reason = "no_compliant_model_available"
	}
	return resolvedModuleSchema{}, fmt.Errorf("%w: %s", errMiniProgramModelNotCompliant, reason)
}

func configuredMiniProgramCreationModes() []string {
	allowed := map[string]bool{"image": true, "infographic": true, "video": true, "ppt": true, "agent": true, "review": true}
	raw := strings.TrimSpace(os.Getenv("MINIPROGRAM_CREATION_MODES"))
	if raw == "" {
		return []string{"image", "infographic", "video"}
	}
	result := []string{}
	for _, item := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '，' || r == ';' }) {
		item = strings.ToLower(strings.TrimSpace(item))
		if allowed[item] && !stringListContainsFold(result, item) {
			result = append(result, item)
		}
	}
	return result
}

func miniprogramCreationModeForTaskType(taskType string) string {
	switch strings.ToUpper(strings.TrimSpace(taskType)) {
	case "TEXT_TO_VIDEO", "IMAGE_TO_VIDEO", "VIDEO_TO_VIDEO":
		return "video"
	case "PPT_GENERATION":
		return "ppt"
	default:
		return "image"
	}
}

func (a api) publicTerminalCapabilities(w http.ResponseWriter, r *http.Request) {
	terminal := requestTerminal(r)
	modes := []string{"image", "video", "ppt", "agent", "infographic", "review"}
	if terminal == terminalMiniProgram {
		modes = configuredMiniProgramCreationModes()
	}
	writeJSON(w, map[string]any{
		"terminal": terminal, "creationModes": modes,
		"features": []string{"guest-home", "product-image", "poster", "text-to-image", "image-to-image", "task-progress", "works", "download", "delete", "recreate", "token-balance", "legal-documents", "complaints"},
	})
}

func isModelGatewayName(value string) bool {
	value = strings.ToLower(strings.NewReplacer("-", "", "_", "", " ", "").Replace(strings.TrimSpace(value)))
	return value == "newapi"
}

func markInputAudit(req *generation.CreateRequest, formal bool) {
	if req == nil || req.Params == nil {
		return
	}
	req.Params["input_audit_status"] = auditApproved
	if formal {
		req.Params["input_audit_service"] = "wechat-content-security"
	} else {
		req.Params["input_audit_service"] = "mock"
	}
	req.Params["input_audit_request_id"] = ""
}

func auditGeneratedOutput(req *generation.CreateRequest) error {
	if req == nil || req.Params == nil {
		return nil
	}
	if !strings.EqualFold(stringValue(req.Params["terminal"]), terminalMiniProgram) {
		return nil
	}
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("CONTENT_AUDIT_OUTPUT_MODE")))
	if mode == "" {
		mode = "mock"
	}
	status := auditApproved
	if mode == "formal" {
		// No formal output-audit adapter is connected in P0. Merely changing an
		// environment value must never make unchecked output look production-ready.
		status = auditManualReview
		mode = "formal-unconfigured"
	} else if strings.Contains(strings.ToLower(req.Prompt), "[audit-reject]") {
		status = auditRejected
	} else if strings.Contains(strings.ToLower(req.Prompt), "[audit-review]") {
		status = auditManualReview
	}
	req.Params["output_audit_status"] = status
	req.Params["output_audit_service"] = mode
	req.Params["output_audit_request_id"] = ""
	if status != auditApproved {
		req.Params["output_audit_reason"] = "content_policy_not_approved"
		return errOutputAuditRejected
	}
	req.Params["ai_label_status"] = "applied"
	req.Params["ai_generated"] = true
	req.Params["ai_label_text"] = "本内容由人工智能生成"
	req.Params["generated_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	return nil
}

func (a api) auditPreparedGeneratedOutput(ctx context.Context, req *generation.CreateRequest) error {
	if req == nil || req.Params == nil || !strings.EqualFold(stringValue(req.Params["terminal"]), terminalMiniProgram) {
		return nil
	}
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("CONTENT_AUDIT_OUTPUT_MODE")))
	if mode != "formal" {
		return auditGeneratedOutput(req)
	}
	if a.contentSecurity == nil || len(req.GeneratedImages) == 0 {
		markGeneratedOutputAudit(req, auditManualReview, "wechat-content-security", "audit_service_unavailable")
		return errContentSecurityUnavailable
	}
	for index, image := range req.GeneratedImages {
		raw, contentType, _, err := readGeneratedArtifact(ctx, image.URL, image.ContentType)
		if err != nil {
			markGeneratedOutputAudit(req, auditManualReview, "wechat-content-security", "generated_image_unavailable")
			return errContentSecurityUnavailable
		}
		filename := fmt.Sprintf("generated-%02d", index+1)
		if thumbnailURL, _, _, ok := thumbnailAndDimensionsFromBytes(raw); ok && strings.HasPrefix(thumbnailURL, "data:image/jpeg;base64,") {
			if compact, compactType, _, compactErr := readGeneratedDataURL(thumbnailURL, "image/jpeg"); compactErr == nil {
				raw = compact
				contentType = compactType
				filename += ".jpg"
			}
		}
		if err := a.contentSecurity.CheckImage(ctx, raw, filename, contentType); err != nil {
			if errors.Is(err, errContentSecurityRejected) {
				markGeneratedOutputAudit(req, auditRejected, "wechat-content-security", "content_policy_not_approved")
				return errOutputAuditRejected
			}
			markGeneratedOutputAudit(req, auditManualReview, "wechat-content-security", "audit_service_unavailable")
			return errContentSecurityUnavailable
		}
	}
	markGeneratedOutputAudit(req, auditApproved, "wechat-content-security", "")
	return nil
}

func markGeneratedOutputAudit(req *generation.CreateRequest, status, service, reason string) {
	if req == nil || req.Params == nil {
		return
	}
	req.Params["output_audit_status"] = status
	req.Params["output_audit_service"] = service
	req.Params["output_audit_request_id"] = ""
	if reason != "" {
		req.Params["output_audit_reason"] = reason
	} else {
		delete(req.Params, "output_audit_reason")
	}
	if status != auditApproved {
		return
	}
	req.Params["ai_label_status"] = "applied"
	req.Params["ai_generated"] = true
	req.Params["ai_label_text"] = "本内容由人工智能生成"
	req.Params["generated_at"] = time.Now().UTC().Format(time.RFC3339Nano)
}

func copyGenerationComplianceMetadata(metadata map[string]any, params map[string]any, contentID string, generatedAt string) {
	if metadata == nil || params == nil {
		return
	}
	for _, key := range []string{"terminal", "provider_name", "provider_company", "algorithm_name", "algorithm_filing_no", "algorithm_type", "model_version", "input_audit_status", "input_audit_service", "input_audit_request_id", "output_audit_status", "output_audit_service", "output_audit_request_id", "output_audit_reason", "ai_label_status", "ai_label_text"} {
		if value, ok := params[key]; ok {
			metadata[key] = value
		}
	}
	if boolValue(params["ai_generated"]) {
		metadata["ai_generated"] = true
		metadata["provider"] = firstNonEmptyString(stringValue(params["provider_company"]), stringValue(params["provider_name"]))
		metadata["content_id"] = contentID
		metadata["generated_at"] = firstNonEmptyString(stringValue(params["generated_at"]), generatedAt)
		metadata["download_derivative_required"] = true
	}
}

type legalAcceptanceDocument struct {
	Code     string `json:"code"`
	Title    string `json:"title"`
	Version  string `json:"version"`
	Accepted bool   `json:"accepted"`
}

func (a api) currentLegalAcceptanceStatus(userID, terminal string) ([]legalAcceptanceDocument, bool, error) {
	store, ok := a.store.(*postgresStore)
	if !ok {
		return nil, false, errors.New("协议确认记录需要 PostgreSQL")
	}
	rows, err := store.db.Query(`
		WITH latest AS (
			SELECT DISTINCT ON (code) code,title,version
			FROM xz_legal_documents
			WHERE status='PUBLISHED' AND code IN ('user-agreement','privacy-policy','ai-content-rules')
			ORDER BY code,published_at DESC,updated_at DESC
		)
		SELECT l.code,l.title,l.version,(a.id IS NOT NULL)
		FROM latest l
		LEFT JOIN xz_user_agreement_acceptances a
		  ON a.user_id=$1 AND a.terminal=$2 AND a.document_code=l.code AND a.document_version=l.version
		ORDER BY l.code`, userID, terminal)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	documents := make([]legalAcceptanceDocument, 0, len(requiredGenerationLegalCodes))
	for rows.Next() {
		var item legalAcceptanceDocument
		if err := rows.Scan(&item.Code, &item.Title, &item.Version, &item.Accepted); err != nil {
			return nil, false, err
		}
		documents = append(documents, item)
	}
	ready := len(documents) == len(requiredGenerationLegalCodes)
	for _, item := range documents {
		ready = ready && item.Accepted
	}
	return documents, ready, rows.Err()
}

func (a api) enforceRequiredLegalAcceptances(userID, terminal string) error {
	if !strings.EqualFold(terminal, terminalMiniProgram) {
		return nil
	}
	_, ready, err := a.currentLegalAcceptanceStatus(userID, terminal)
	if err != nil || !ready {
		return errLegalAcceptanceRequired
	}
	return nil
}

func (a api) legalAcceptanceStatus(w http.ResponseWriter, r *http.Request) {
	_, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	documents, ready, err := a.currentLegalAcceptanceStatus(user.ID, requestTerminal(r))
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	writeJSON(w, map[string]any{"ready": ready, "items": documents})
}

func (a api) acceptCurrentLegalDocuments(w http.ResponseWriter, r *http.Request) {
	_, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	terminal := requestTerminal(r)
	documents, _, err := a.currentLegalAcceptanceStatus(user.ID, terminal)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	if len(documents) != len(requiredGenerationLegalCodes) {
		writeError(w, http.StatusPreconditionFailed, errors.New("必要协议尚未全部发布，暂不能确认"))
		return
	}
	store := a.store.(*postgresStore)
	tx, err := store.db.Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer tx.Rollback()
	requestID := firstNonEmptyString(strings.TrimSpace(r.Header.Get("X-Request-ID")), strings.TrimSpace(r.Header.Get("Idempotency-Key")))
	for _, item := range documents {
		id := legalAcceptanceID(user.ID, terminal, item.Code, item.Version)
		if _, err = tx.Exec(`INSERT INTO xz_user_agreement_acceptances(id,user_id,document_code,document_version,terminal,accepted_at,request_id)
			VALUES($1,$2,$3,$4,$5,now(),$6) ON CONFLICT(user_id,document_code,document_version,terminal) DO NOTHING`,
			id, user.ID, item.Code, item.Version, terminal, requestID); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	if err = tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"ready": true, "acceptedAt": time.Now().UTC().Format(time.RFC3339Nano), "items": documents})
}

func legalAcceptanceID(userID, terminal, documentCode, documentVersion string) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{userID, terminal, documentCode, documentVersion}, "|")))
	return "accept_" + hex.EncodeToString(digest[:12])
}

func (a api) recordContentAudit(taskID, stage, contentType, contentID string, req generation.CreateRequest) {
	store, ok := a.store.(*postgresStore)
	if !ok || !strings.EqualFold(stringValue(req.Params["terminal"]), terminalMiniProgram) {
		return
	}
	prefix := stage + "_audit_"
	status := firstNonEmptyString(stringValue(req.Params[prefix+"status"]), auditPending)
	service := firstNonEmptyString(stringValue(req.Params[prefix+"service"]), "mock")
	serviceRequestID := stringValue(req.Params[prefix+"request_id"])
	reason := stringValue(req.Params[prefix+"reason"])
	tenantID := stringValue(req.Params["tenant_id"])
	id := "audit_" + shortID(taskID+"_"+stage+"_"+contentType+"_"+contentID)
	_, _ = store.db.Exec(`INSERT INTO xz_content_audits(id,task_id,user_id,tenant_id,terminal,stage,content_type,content_id,status,service_kind,service_request_id,reason_code,created_at,updated_at)
		VALUES($1,$2,$3,NULLIF($4,''),$5,$6,$7,$8,$9,$10,NULLIF($11,''),NULLIF($12,''),now(),now())
		ON CONFLICT(task_id,stage,content_type,content_id) DO UPDATE SET status=excluded.status,service_kind=excluded.service_kind,
		service_request_id=excluded.service_request_id,reason_code=excluded.reason_code,updated_at=now()`,
		id, taskID, req.UserID, tenantID, terminalMiniProgram, stage, contentType, contentID, status, service, serviceRequestID, reason)
}

func (a adminAPI) contentAudits(w http.ResponseWriter, r *http.Request) {
	store, ok := a.store.(*postgresStore)
	if !ok {
		writeJSON(w, map[string]any{"items": []any{}})
		return
	}
	status := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status")))
	if status == "" {
		status = auditManualReview
	}
	if status != auditPending && status != auditApproved && status != auditRejected && status != auditManualReview {
		writeError(w, http.StatusBadRequest, errors.New("invalid audit status"))
		return
	}
	rows, err := store.db.Query(`SELECT id,task_id,user_id,coalesce(tenant_id,''),terminal,stage,content_type,coalesce(content_id,''),status,
		service_kind,coalesce(service_request_id,''),coalesce(reason_code,''),coalesce(reviewed_by,''),coalesce(reviewed_at::text,''),created_at::text,updated_at::text
		FROM xz_content_audits WHERE status=$1 ORDER BY created_at DESC LIMIT 200`, status)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, taskID, userID, tenantID, terminal, stage, contentType, contentID, auditStatus, service, serviceRequestID, reason, reviewedBy, reviewedAt, createdAt, updatedAt string
		if err := rows.Scan(&id, &taskID, &userID, &tenantID, &terminal, &stage, &contentType, &contentID, &auditStatus, &service, &serviceRequestID, &reason, &reviewedBy, &reviewedAt, &createdAt, &updatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		items = append(items, map[string]any{"id": id, "taskId": taskID, "userId": userID, "tenantId": tenantID, "terminal": terminal, "stage": stage, "contentType": contentType, "contentId": contentID, "status": auditStatus, "service": service, "serviceRequestId": serviceRequestID, "reason": reason, "reviewedBy": reviewedBy, "reviewedAt": reviewedAt, "createdAt": createdAt, "updatedAt": updatedAt})
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a adminAPI) reviewContentAudit(w http.ResponseWriter, r *http.Request) {
	store, ok := a.store.(*postgresStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, errors.New("content audit review requires PostgreSQL"))
		return
	}
	var req struct {
		Status string `json:"status"`
		Remark string `json:"remark"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.Status = strings.ToLower(strings.TrimSpace(req.Status))
	if req.Status != auditApproved && req.Status != auditRejected {
		writeError(w, http.StatusBadRequest, errors.New("manual review status must be approved or rejected"))
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	reviewerID, _ := actorFromRequest(r)
	result, err := store.db.Exec(`UPDATE xz_content_audits SET status=$2,reason_code=NULLIF($3,''),reviewed_by=NULLIF($4,''),reviewed_at=now(),updated_at=now()
		WHERE id=$1 AND status='manual_review'`, id, req.Status, strings.TrimSpace(req.Remark), reviewerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		writeError(w, http.StatusConflict, errors.New("audit record is not awaiting manual review"))
		return
	}
	writeJSON(w, map[string]any{"id": id, "status": req.Status})
}

func (a api) writeCompliantAssetDownload(w http.ResponseWriter, r *http.Request, item asset) bool {
	if !boolValue(item.Metadata["ai_generated"]) {
		return false
	}
	if !strings.EqualFold(stringMetadataValue(item, "output_audit_status"), auditApproved) {
		writeError(w, http.StatusUnprocessableEntity, errOutputAuditRejected)
		return true
	}
	if markedURL := strings.TrimSpace(stringMetadataValue(item, "download_marked_url")); markedURL != "" {
		clone := item
		clone.URL = markedURL
		clone.Metadata = cloneAnyMap(item.Metadata)
		clone.Metadata["ai_generated"] = false
		a.writeAssetDownload(w, r, clone)
		return true
	}
	if strings.HasPrefix(strings.ToLower(item.URL), "data:image/svg+xml") {
		comma := strings.IndexByte(item.URL, ',')
		if comma < 0 {
			writeError(w, http.StatusBadRequest, errors.New("invalid generated SVG"))
			return true
		}
		header := item.URL[:comma]
		payload := item.URL[comma+1:]
		var raw []byte
		var err error
		if strings.Contains(strings.ToLower(header), ";base64") {
			raw, err = base64.StdEncoding.DecodeString(payload)
		} else {
			var decoded string
			decoded, err = url.PathUnescape(payload)
			raw = []byte(decoded)
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid generated SVG"))
			return true
		}
		label := `<g id="ai-generated-label"><rect x="20" y="20" width="180" height="42" rx="10" fill="rgba(0,0,0,.62)"/><text x="36" y="49" fill="#fff" font-size="22" font-family="sans-serif">AI生成</text></g>`
		body := strings.Replace(string(raw), "</svg>", label+"</svg>", 1)
		writeAttachmentHeaders(w, "image/svg+xml", downloadAssetName(item, "image/svg+xml"))
		w.Header().Set("X-AI-Generated", "true")
		_, _ = w.Write([]byte(body))
		return true
	}
	raw, contentType, err := func() ([]byte, string, error) {
		if stored, storedType, ok := a.readStoredAssetBytes(r.Context(), item); ok {
			return stored, firstNonEmptyString(storedType, item.MediaType), nil
		}
		payload, mediaType, _, readErr := readGeneratedArtifact(r.Context(), item.URL, item.MediaType)
		return payload, mediaType, readErr
	}()
	if err == nil && len(raw) > 0 {
		if strings.EqualFold(contentType, "image/png") || strings.EqualFold(contentType, "image/jpeg") {
			if marked, markErr := renderRasterAILabel(raw, contentType, a.aiLabelSetting()); markErr == nil {
				a.recordInlineDownloadDerivative(item)
				writeAttachmentHeaders(w, contentType, downloadAssetName(item, contentType))
				w.Header().Set("X-AI-Generated", "true")
				w.Header().Set("X-AI-Derivative-Of", item.ID)
				_, _ = w.Write(marked)
				return true
			}
		}
		// Private object storage URLs are not browser-reachable; still serve the
		// original bytes so web detail/preview can load when watermarking fails.
		writeAttachmentHeaders(w, firstNonEmptyString(contentType, "application/octet-stream"), downloadAssetName(item, contentType))
		w.Header().Set("X-AI-Generated", "true")
		_, _ = w.Write(raw)
		return true
	}
	writeError(w, http.StatusServiceUnavailable, errors.New("带AI标识的下载文件尚未生成，请稍后重试"))
	return true
}

type aiLabelSetting struct {
	Position string
	Opacity  float64
	Size     float64
}

func (a api) aiLabelSetting() aiLabelSetting {
	setting := aiLabelSetting{Position: "bottom-right", Opacity: .65, Size: .035}
	if store, ok := a.store.(*postgresStore); ok {
		_ = store.db.QueryRow(`SELECT position,opacity,size_ratio FROM xz_ai_label_settings WHERE terminal='miniprogram' AND enabled=TRUE`).Scan(&setting.Position, &setting.Opacity, &setting.Size)
	}
	return setting
}

func (a api) recordInlineDownloadDerivative(item asset) {
	store, ok := a.store.(*postgresStore)
	if !ok {
		return
	}
	originalFileID := firstNonEmptyString(stringValue(item.Metadata["fileId"]), stringValue(item.Metadata["storageFileId"]))
	if originalFileID == "" {
		return
	}
	labelSnapshot, _ := json.Marshal(a.aiLabelSetting())
	metadataSnapshot, _ := json.Marshal(map[string]any{"ai_generated": true, "content_id": item.ID, "provider": item.Metadata["provider"], "algorithm_name": item.Metadata["algorithm_name"], "algorithm_filing_no": item.Metadata["algorithm_filing_no"], "generated_at": item.Metadata["generated_at"]})
	id := "derivative_" + shortID(item.ID+"_"+originalFileID)
	_, _ = store.db.Exec(`INSERT INTO xz_ai_download_derivatives(id,content_id,original_file_id,label_config_snapshot,metadata_snapshot,status,created_at,updated_at)
		VALUES($1,$2,$3,$4::jsonb,$5::jsonb,'GENERATED_INLINE',now(),now())
		ON CONFLICT(content_id,original_file_id) DO UPDATE SET label_config_snapshot=excluded.label_config_snapshot,
		metadata_snapshot=excluded.metadata_snapshot,status='GENERATED_INLINE',updated_at=now()`, id, item.ID, originalFileID, string(labelSnapshot), string(metadataSnapshot))
}

func renderRasterAILabel(raw []byte, contentType string, setting aiLabelSetting) ([]byte, error) {
	source, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	bounds := source.Bounds()
	canvas := image.NewRGBA(bounds)
	draw.Draw(canvas, bounds, source, bounds.Min, draw.Src)
	scale := bounds.Dx() / 260
	if scale < 2 {
		scale = 2
	}
	if setting.Size > 0 {
		candidate := int(float64(bounds.Dx()) * setting.Size / 7)
		if candidate > scale {
			scale = candidate
		}
	}
	patterns := [][]string{glyphA, glyphI, glyphSheng, glyphCheng}
	labelWidth := 0
	for _, pattern := range patterns {
		labelWidth += (len(pattern[0]) + 1) * scale
	}
	padding := 3 * scale
	boxWidth, boxHeight := labelWidth+padding*2-scale, 7*scale+padding*2
	x, y := bounds.Min.X+padding, bounds.Max.Y-boxHeight-padding
	if strings.Contains(strings.ToLower(setting.Position), "right") {
		x = bounds.Max.X - boxWidth - padding
	}
	if strings.Contains(strings.ToLower(setting.Position), "top") {
		y = bounds.Min.Y + padding
	}
	alpha := uint8(166)
	if setting.Opacity >= 0 && setting.Opacity <= 1 {
		alpha = uint8(setting.Opacity * 255)
	}
	draw.Draw(canvas, image.Rect(x, y, x+boxWidth, y+boxHeight), &image.Uniform{C: color.RGBA{0, 0, 0, alpha}}, image.Point{}, draw.Over)
	cursor := x + padding
	for _, pattern := range patterns {
		drawBitmapGlyph(canvas, cursor, y+padding, scale, pattern)
		cursor += (len(pattern[0]) + 1) * scale
	}
	var output bytes.Buffer
	if strings.EqualFold(contentType, "image/jpeg") {
		err = jpeg.Encode(&output, canvas, &jpeg.Options{Quality: 92})
	} else {
		err = png.Encode(&output, canvas)
	}
	return output.Bytes(), err
}

func drawBitmapGlyph(target *image.RGBA, x, y, scale int, pattern []string) {
	for row, line := range pattern {
		for column, value := range line {
			if value != '1' {
				continue
			}
			draw.Draw(target, image.Rect(x+column*scale, y+row*scale, x+(column+1)*scale, y+(row+1)*scale), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
		}
	}
}

var (
	glyphA     = []string{"01110", "10001", "10001", "11111", "10001", "10001", "10001"}
	glyphI     = []string{"111", "010", "010", "010", "010", "010", "111"}
	glyphSheng = []string{"00100", "11111", "00100", "11111", "10100", "10100", "11111"}
	glyphCheng = []string{"10001", "11111", "10101", "11101", "10110", "11010", "10101"}
)

func (a api) publicLegalDocuments(w http.ResponseWriter, _ *http.Request) {
	if store, ok := a.store.(*postgresStore); ok {
		rows, err := store.db.Query(`SELECT code,title,version,content FROM xz_legal_documents WHERE status='PUBLISHED' ORDER BY code,published_at DESC`)
		if err == nil {
			defer rows.Close()
			documents := []map[string]any{}
			seen := map[string]bool{}
			for rows.Next() {
				var code, title, version, content string
				if rows.Scan(&code, &title, &version, &content) == nil && !seen[code] {
					seen[code] = true
					documents = append(documents, map[string]any{"code": code, "title": title, "version": version, "content": content})
				}
			}
			if len(documents) > 0 {
				writeJSON(w, map[string]any{"items": documents, "complaintUrl": envOrPlaceholder("COMPLAINT_ENTRY_URL"), "infringementUrl": envOrPlaceholder("INFRINGEMENT_COMPLAINT_URL")})
				return
			}
		}
	}
	documents := []map[string]any{
		{"code": "user-agreement", "title": "用户服务协议", "version": envOrPlaceholder("LEGAL_USER_AGREEMENT_VERSION"), "content": envOrPlaceholder("LEGAL_USER_AGREEMENT_CONTENT")},
		{"code": "privacy-policy", "title": "隐私政策", "version": envOrPlaceholder("LEGAL_PRIVACY_POLICY_VERSION"), "content": envOrPlaceholder("LEGAL_PRIVACY_POLICY_CONTENT")},
		{"code": "ai-content-rules", "title": "AI生成内容使用规范", "version": envOrPlaceholder("LEGAL_AI_CONTENT_RULES_VERSION"), "content": envOrPlaceholder("LEGAL_AI_CONTENT_RULES_CONTENT")},
		{"code": "platform-convention", "title": "平台公约", "version": envOrPlaceholder("LEGAL_PLATFORM_CONVENTION_VERSION"), "content": envOrPlaceholder("LEGAL_PLATFORM_CONVENTION_CONTENT")},
		{"code": "minor-protection", "title": "未成年人保护说明", "version": envOrPlaceholder("LEGAL_MINOR_PROTECTION_VERSION"), "content": envOrPlaceholder("LEGAL_MINOR_PROTECTION_CONTENT")},
		{"code": "member-service-agreement", "title": "知启云AI会员服务协议", "version": envOrPlaceholder("LEGAL_MEMBER_SERVICE_AGREEMENT_VERSION"), "content": envOrPlaceholder("LEGAL_MEMBER_SERVICE_AGREEMENT_CONTENT")},
		{"code": "agent-service-agreement", "title": "知启云AI代理商服务协议", "version": envOrPlaceholder("LEGAL_AGENT_SERVICE_AGREEMENT_VERSION"), "content": envOrPlaceholder("LEGAL_AGENT_SERVICE_AGREEMENT_CONTENT")},
		{"code": "enterprise-space-service-agreement", "title": "企业空间服务协议", "version": envOrPlaceholder("LEGAL_ENTERPRISE_SPACE_SERVICE_AGREEMENT_VERSION"), "content": envOrPlaceholder("LEGAL_ENTERPRISE_SPACE_SERVICE_AGREEMENT_CONTENT")},
		{"code": "recharge-service-agreement", "title": "点数充值服务协议", "version": envOrPlaceholder("LEGAL_RECHARGE_SERVICE_AGREEMENT_VERSION"), "content": envOrPlaceholder("LEGAL_RECHARGE_SERVICE_AGREEMENT_CONTENT")},
	}
	writeJSON(w, map[string]any{"items": documents, "complaintUrl": envOrPlaceholder("COMPLAINT_ENTRY_URL"), "infringementUrl": envOrPlaceholder("INFRINGEMENT_COMPLAINT_URL")})
}

type legalDocumentMutation struct {
	Title   string `json:"title"`
	Version string `json:"version"`
	Content string `json:"content"`
	Status  string `json:"status"`
}

func (a adminAPI) legalDocuments(w http.ResponseWriter, _ *http.Request) {
	store, ok := a.store.(*postgresStore)
	if !ok {
		writeJSON(w, map[string]any{"items": []any{}})
		return
	}
	rows, err := store.db.Query(`SELECT id,code,title,version,content,status,coalesce(published_at::text,''),created_at::text,updated_at::text FROM xz_legal_documents ORDER BY code,created_at DESC`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, code, title, version, content, status, publishedAt, createdAt, updatedAt string
		if err := rows.Scan(&id, &code, &title, &version, &content, &status, &publishedAt, &createdAt, &updatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		items = append(items, map[string]any{"id": id, "code": code, "title": title, "version": version, "content": content, "status": status, "publishedAt": publishedAt, "createdAt": createdAt, "updatedAt": updatedAt})
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a adminAPI) saveLegalDocument(w http.ResponseWriter, r *http.Request) {
	store, ok := a.store.(*postgresStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, errors.New("legal document editing requires PostgreSQL"))
		return
	}
	code := strings.ToLower(strings.TrimSpace(r.PathValue("code")))
	var req legalDocumentMutation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	req.Version = strings.TrimSpace(req.Version)
	req.Content = strings.TrimSpace(req.Content)
	req.Status = strings.ToUpper(strings.TrimSpace(req.Status))
	if code == "" || req.Title == "" || req.Version == "" || req.Content == "" {
		writeError(w, http.StatusBadRequest, errors.New("code, title, version and content are required"))
		return
	}
	if req.Status == "" {
		req.Status = "DRAFT"
	}
	if req.Status != "DRAFT" && req.Status != "PUBLISHED" && req.Status != "ARCHIVED" {
		writeError(w, http.StatusBadRequest, errors.New("invalid legal document status"))
		return
	}
	id := "legal_" + shortID(code+"_"+req.Version)
	_, err := store.db.Exec(`INSERT INTO xz_legal_documents(id,code,title,version,content,status,published_at,created_at,updated_at)
		VALUES($1,$2,$3,$4,$5,$6,CASE WHEN $6='PUBLISHED' THEN now() ELSE NULL END,now(),now())
		ON CONFLICT(code,version) DO UPDATE SET title=excluded.title,content=excluded.content,status=excluded.status,
		published_at=CASE WHEN excluded.status='PUBLISHED' THEN coalesce(xz_legal_documents.published_at,now()) ELSE xz_legal_documents.published_at END,updated_at=now()`, id, code, req.Title, req.Version, req.Content, req.Status)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"id": id, "code": code, "version": req.Version, "status": req.Status})
}

func envOrPlaceholder(key string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return "待配置"
}

func (a adminAPI) miniProgramComplianceCheck(w http.ResponseWriter, _ *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	now := time.Now().UTC()
	available := 0
	routable := 0
	unsafeEnabled := []map[string]string{}
	unroutableEnabled := []map[string]string{}
	expiring := []map[string]string{}
	for _, model := range data.AIModels {
		allowed, reason := modelAllowedForMiniProgram(model, now)
		if allowed {
			available++
			if _, routed, routeErr := selectAPIChannelForConfiguredModel(data, model.ModelName); routed && routeErr == nil {
				routable++
			} else if model.MiniProgramEnabled {
				routeReason := "model_channel_not_configured"
				if routeErr != nil {
					routeReason = routeErr.Error()
				}
				unroutableEnabled = append(unroutableEnabled, map[string]string{"id": model.ID, "model": model.ModelName, "reason": routeReason})
			}
		}
		if model.MiniProgramEnabled && !allowed {
			unsafeEnabled = append(unsafeEnabled, map[string]string{"id": model.ID, "model": model.ModelName, "reason": reason})
		}
		if expiry := strings.TrimSpace(model.ContractExpireAt); expiry != "" {
			if parsed, parseErr := parseComplianceTime(expiry); parseErr == nil && parsed.After(now) && parsed.Before(now.Add(30*24*time.Hour)) {
				expiring = append(expiring, map[string]string{"id": model.ID, "model": model.ModelName, "expireAt": expiry})
			}
		}
	}
	sort.Slice(unsafeEnabled, func(i, j int) bool { return unsafeEnabled[i]["model"] < unsafeEnabled[j]["model"] })
	sort.Slice(unroutableEnabled, func(i, j int) bool { return unroutableEnabled[i]["model"] < unroutableEnabled[j]["model"] })
	formalOutputAudit := strings.EqualFold(strings.TrimSpace(os.Getenv("CONTENT_AUDIT_OUTPUT_MODE")), "formal")
	publishedLegal := map[string]bool{}
	visibleLabel, implicitLabel := false, false
	if store, ok := a.store.(*postgresStore); ok {
		rows, queryErr := store.db.Query(`SELECT DISTINCT code FROM xz_legal_documents WHERE status='PUBLISHED'`)
		if queryErr == nil {
			defer rows.Close()
			for rows.Next() {
				var code string
				if rows.Scan(&code) == nil {
					publishedLegal[code] = true
				}
			}
		}
		_ = store.db.QueryRow(`SELECT enabled,implicit_label_enabled FROM xz_ai_label_settings WHERE terminal='miniprogram'`).Scan(&visibleLabel, &implicitLabel)
	}
	checks := []map[string]any{
		{"code": "enterprise_profile", "label": "企业主体资料已配置", "passed": strings.TrimSpace(os.Getenv("ENTERPRISE_LEGAL_NAME")) != ""},
		{"code": "available_models", "label": "存在小程序合规模型", "passed": available > 0, "value": available},
		{"code": "routable_models", "label": "小程序合规模型存在可用技术通道", "passed": routable > 0, "value": routable},
		{"code": "unroutable_models", "label": "不存在已启用但无法路由的合规模型", "passed": len(unroutableEnabled) == 0, "items": unroutableEnabled},
		{"code": "content_audit", "label": "内容审核正式启用", "passed": strings.TrimSpace(os.Getenv("WECHAT_MINI_PROGRAM_APPID")) != "" && strings.TrimSpace(os.Getenv("WECHAT_MINI_PROGRAM_SECRET")) != "" && formalOutputAudit},
		{"code": "visible_label", "label": "AI显式标识已启用", "passed": visibleLabel},
		{"code": "implicit_label", "label": "AI隐式标识字段已启用", "passed": implicitLabel},
		{"code": "user_agreement", "label": "用户协议已发布", "passed": publishedLegal["user-agreement"]},
		{"code": "privacy_policy", "label": "隐私政策已发布", "passed": publishedLegal["privacy-policy"]},
		{"code": "ai_content_rules", "label": "AI生成内容使用规范已发布", "passed": publishedLegal["ai-content-rules"]},
		{"code": "complaint_entry", "label": "投诉举报入口可用", "passed": envOrPlaceholder("COMPLAINT_ENTRY_URL") != "待配置"},
		{"code": "unsafe_models", "label": "不存在违规开启模型", "passed": len(unsafeEnabled) == 0, "items": unsafeEnabled},
		{"code": "expiring_contracts", "label": "无30天内到期协议", "passed": len(expiring) == 0, "items": expiring},
	}
	blocked := false
	for _, check := range checks {
		if passed, _ := check["passed"].(bool); !passed {
			blocked = true
		}
	}
	writeJSON(w, map[string]any{"blocked": blocked, "availableModelCount": available, "checks": checks, "openCapabilities": configuredMiniProgramCreationModes()})
}
