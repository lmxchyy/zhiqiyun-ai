package httpserver

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

const (
	identityMethodOnlyIdentity      = "ONLY_IDENTITY"
	identityMethodOfflineOrder      = "OFFLINE_ORDER"
	identityMethodSpecialGrant      = "SPECIAL_GRANT"
	identityMethodPackageConversion = "PACKAGE_CONVERSION"
)

var (
	errIdentityChangeInvalid   = errors.New("invalid identity change request")
	errIdentityChangeBlocked   = errors.New("identity change is blocked")
	errIdentityPreviewNotFound = errors.New("identity change preview not found")
	errIdentityPreviewExpired  = errors.New("identity change preview expired")
	errIdentityReviewRequired  = errors.New("identity change review is required")
	errIdentityHighRiskConfirm = errors.New("high risk confirmation is required")
	errIdentityPermission      = errors.New("identity change permission denied")
)

type identityPaymentProof struct {
	Reference      string `json:"reference"`
	StorageFileID  string `json:"storageFileId,omitempty"`
	PayerName      string `json:"payerName"`
	PaidAt         string `json:"paidAt"`
	PaymentChannel string `json:"paymentChannel"`
	Remark         string `json:"remark,omitempty"`
	URL            string `json:"url,omitempty"` // legacy preview compatibility; never accepted as sole proof
}

type identityChangePreviewRequest struct {
	Action                string               `json:"action"`
	Method                string               `json:"method"`
	TargetIdentity        string               `json:"targetIdentity,omitempty"`
	PlanID                string               `json:"planId,omitempty"`
	ParentAgentID         string               `json:"parentAgentId,omitempty"`
	OperationCenterID     string               `json:"operationCenterId,omitempty"`
	PaidAmountCents       int                  `json:"paidAmountCents,omitempty"`
	GrantPackageToken     bool                 `json:"grantPackageToken,omitempty"`
	GiftTokenAmount       int                  `json:"giftTokenAmount,omitempty"`
	ConversionTokenPolicy string               `json:"conversionTokenPolicy,omitempty"`
	PaymentProof          identityPaymentProof `json:"paymentProof,omitempty"`
	DiscountReason        string               `json:"discountReason,omitempty"`
	Reason                string               `json:"reason"`
	Remark                string               `json:"remark,omitempty"`
}

type identityCommissionPreview struct {
	BeneficiaryType string `json:"beneficiaryType"`
	BeneficiaryID   string `json:"beneficiaryId"`
	RuleID          string `json:"ruleId"`
	RuleCode        string `json:"ruleCode"`
	AmountCents     int64  `json:"amountCents"`
}

type identityChangePreviewResult struct {
	PreviewToken            string                      `json:"previewToken,omitempty"`
	PreviewID               string                      `json:"previewId"`
	UserID                  string                      `json:"userId"`
	OldIdentity             string                      `json:"oldIdentity"`
	TargetIdentity          string                      `json:"targetIdentity"`
	Method                  string                      `json:"method"`
	Action                  string                      `json:"action"`
	RelationshipBefore      map[string]any              `json:"relationshipBefore"`
	RelationshipAfter       map[string]any              `json:"relationshipAfter"`
	PaymentRequired         bool                        `json:"paymentRequired"`
	PaidAmountCents         int64                       `json:"paidAmountCents"`
	OriginalAmountCents     int64                       `json:"originalAmountCents"`
	DiscountAmountCents     int64                       `json:"discountAmountCents"`
	PayableAmountCents      int64                       `json:"payableAmountCents"`
	SpecialPrice            bool                        `json:"specialPrice"`
	TokenDelta              int64                       `json:"tokenDelta"`
	TokenChangeType         string                      `json:"tokenChangeType,omitempty"`
	CommissionGenerated     bool                        `json:"commissionGenerated"`
	EstimatedCommissions    []identityCommissionPreview `json:"estimatedCommissions"`
	CommissionRuleSnapshot  []commissionRuleSnapshot    `json:"commissionRuleSnapshot,omitempty"`
	RiskWarnings            []string                    `json:"riskWarnings"`
	Blockers                []string                    `json:"blockers"`
	HighRisk                bool                        `json:"highRisk"`
	ReviewRequired          bool                        `json:"reviewRequired"`
	Status                  string                      `json:"status"`
	ExpiresAt               string                      `json:"expiresAt"`
	EffectiveAt             string                      `json:"effectiveAt"`
	SourceMembershipOrderID string                      `json:"sourceMembershipOrderId,omitempty"`
}

type identityChangeConfirmRequest struct {
	PreviewToken      string `json:"previewToken"`
	HighRiskConfirmed bool   `json:"highRiskConfirmed"`
}

type identityChangeConfirmResult struct {
	ExecutionID string         `json:"executionId"`
	PreviewID   string         `json:"previewId"`
	UserID      string         `json:"userId"`
	Status      string         `json:"status"`
	OrderID     string         `json:"orderId,omitempty"`
	Result      map[string]any `json:"result"`
	Idempotent  bool           `json:"idempotent"`
}

type identityChangeReviewRequest struct {
	PreviewToken string `json:"previewToken"`
	Decision     string `json:"decision"`
	Reason       string `json:"reason"`
}

type adminIdentityChangeStore interface {
	PreviewAdminIdentityChange(actorID, actorRole, userID string, request identityChangePreviewRequest) (identityChangePreviewResult, error)
	ReviewAdminIdentityChange(actorID, actorRole, userID string, request identityChangeReviewRequest) (identityChangePreviewResult, error)
	ConfirmAdminIdentityChange(actorID, actorRole, userID string, request identityChangeConfirmRequest) (identityChangeConfirmResult, error)
}

type identityChangeAPI struct{ commands *identityCommandService }

func newIdentityChangeAPI(store platformStore) identityChangeAPI {
	return identityChangeAPI{commands: newIdentityCommandService(store)}
}

func (a identityChangeAPI) preview(w http.ResponseWriter, r *http.Request) {
	var request identityChangePreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	actorID, actorRole := actorFromRequest(r)
	result, err := a.commands.Preview(actorID, actorRole, strings.TrimSpace(r.PathValue("id")), request)
	if err != nil {
		writeIdentityChangeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": result})
}

func (a identityChangeAPI) review(w http.ResponseWriter, r *http.Request) {
	var request identityChangeReviewRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	actorID, actorRole := actorFromRequest(r)
	result, err := a.commands.Review(actorID, actorRole, strings.TrimSpace(r.PathValue("id")), request)
	if err != nil {
		writeIdentityChangeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": result})
}

func (a identityChangeAPI) confirm(w http.ResponseWriter, r *http.Request) {
	var request identityChangeConfirmRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	actorID, actorRole := actorFromRequest(r)
	result, err := a.commands.Confirm(actorID, actorRole, strings.TrimSpace(r.PathValue("id")), request)
	if err != nil {
		writeIdentityChangeError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": result})
}

func writeIdentityChangeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errIdentityPreviewNotFound), errors.Is(err, errIdentityUserNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, errIdentityPermission):
		writeError(w, http.StatusForbidden, err)
	case errors.Is(err, errIdentityChangeBlocked), errors.Is(err, errIdentityReviewRequired), errors.Is(err, errIdentityHighRiskConfirm), errors.Is(err, errIdentityPreviewExpired):
		writeError(w, http.StatusConflict, err)
	case errors.Is(err, errIdentityChangeInvalid):
		writeError(w, http.StatusBadRequest, err)
	default:
		writeError(w, http.StatusInternalServerError, err)
	}
}

func newIdentityPreviewToken() (string, string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", "", err
	}
	token := "icp_" + hex.EncodeToString(value)
	return token, identityPreviewTokenHash(token), nil
}

func identityPreviewTokenHash(token string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(digest[:])
}
