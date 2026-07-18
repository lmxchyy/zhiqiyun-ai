package httpserver

import (
	"fmt"
	"strings"
	"time"
)

const (
	permissionEnterpriseList                = "enterprise:list"
	permissionEnterpriseDetail              = "enterprise:detail"
	permissionEnterpriseCreate              = "enterprise:create"
	permissionEnterpriseUpdate              = "enterprise:update"
	permissionEnterpriseCertificationReview = "enterprise:certification:review"
	permissionEnterpriseMemberView          = "enterprise:member:view"
	permissionEnterprisePackageView         = "enterprise:package:view"
	permissionEnterprisePackageAdjust       = "enterprise:package:adjust"
	permissionEnterpriseSeatAdjust          = "enterprise:seat:adjust"
	permissionEnterpriseComputeView         = "enterprise:compute:view"
	permissionEnterpriseComputeAdjust       = "enterprise:compute:adjust"
	permissionEnterpriseTransactionView     = "enterprise:transaction:view"
	permissionEnterpriseOrderView           = "enterprise:order:view"
	permissionEnterpriseAIView              = "enterprise:ai:view"
	permissionEnterpriseAIConfigure         = "enterprise:ai:configure"
	permissionEnterpriseEmployeeView        = "enterprise:employee:view"
	permissionEnterpriseKnowledgeView       = "enterprise:knowledge:view"
	permissionEnterpriseAttributionView     = "enterprise:attribution:view"
	permissionEnterpriseAttributionChange   = "enterprise:attribution:change"
	permissionEnterpriseRiskView            = "enterprise:risk:view"
	permissionEnterpriseRiskDisable         = "enterprise:risk:disable"
	permissionEnterpriseRiskRestore         = "enterprise:risk:restore"
	permissionEnterpriseServiceTransition   = "enterprise:service:transition"
	permissionEnterpriseAuditView           = "enterprise:audit:view"
	permissionEnterpriseConnectorView       = "enterprise:connector:view"
	permissionEnterpriseExport              = "enterprise:export"
)

var adminEnterprisePermissions = []string{
	permissionEnterpriseList,
	permissionEnterpriseDetail,
	permissionEnterpriseCreate,
	permissionEnterpriseUpdate,
	permissionEnterpriseCertificationReview,
	permissionEnterpriseMemberView,
	permissionEnterprisePackageView,
	permissionEnterprisePackageAdjust,
	permissionEnterpriseSeatAdjust,
	permissionEnterpriseComputeView,
	permissionEnterpriseComputeAdjust,
	permissionEnterpriseTransactionView,
	permissionEnterpriseOrderView,
	permissionEnterpriseAIView,
	permissionEnterpriseAIConfigure,
	permissionEnterpriseEmployeeView,
	permissionEnterpriseKnowledgeView,
	permissionEnterpriseAttributionView,
	permissionEnterpriseAttributionChange,
	permissionEnterpriseRiskView,
	permissionEnterpriseRiskDisable,
	permissionEnterpriseRiskRestore,
	permissionEnterpriseServiceTransition,
	permissionEnterpriseAuditView,
	permissionEnterpriseConnectorView,
	permissionEnterpriseExport,
}

type adminEnterpriseListQuery struct {
	Page              int
	PageSize          int
	Keyword           string
	Certification     string
	PlanCode          string
	Status            string
	SourceAgentID     string
	OperationCenterID string
	CreatedFrom       *time.Time
	CreatedTo         *time.Time
}

type adminEnterprisePlanSummary struct {
	ID        string `json:"id,omitempty"`
	Code      string `json:"code"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	ExpiresAt string `json:"expiresAt,omitempty"`
}

type adminEnterpriseComputeSummary struct {
	Balance     int64  `json:"balance"`
	Frozen      int64  `json:"frozen"`
	Unit        string `json:"unit"`
	BalanceText string `json:"balanceText,omitempty"`
}

type adminEnterpriseRelationSummary struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

type adminEnterpriseListItem struct {
	ID                  string                         `json:"id"`
	EnterpriseCode      string                         `json:"enterpriseCode"`
	Name                string                         `json:"name"`
	CertificationStatus string                         `json:"certificationStatus"`
	Plan                adminEnterprisePlanSummary     `json:"plan"`
	MemberCount         int                            `json:"memberCount"`
	ActiveMemberCount   int                            `json:"activeMemberCount"`
	SeatLimit           int                            `json:"seatLimit"`
	Compute             adminEnterpriseComputeSummary  `json:"compute"`
	SourceAgent         adminEnterpriseRelationSummary `json:"sourceAgent"`
	OperationCenter     adminEnterpriseRelationSummary `json:"operationCenter"`
	Status              string                         `json:"status"`
	CreatedAt           string                         `json:"createdAt"`
	UpdatedAt           string                         `json:"updatedAt"`
}

type adminEnterpriseStats struct {
	Total            int `json:"total"`
	Certified        int `json:"certified"`
	CreatedThisMonth int `json:"createdThisMonth"`
	Abnormal         int `json:"abnormal"`
}

type adminEnterpriseFilterOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type adminEnterpriseFilters struct {
	Plans            []adminEnterpriseFilterOption `json:"plans"`
	SourceAgents     []adminEnterpriseFilterOption `json:"sourceAgents"`
	OperationCenters []adminEnterpriseFilterOption `json:"operationCenters"`
}

type adminEnterpriseListResult struct {
	Items    []adminEnterpriseListItem `json:"items"`
	Total    int                       `json:"total"`
	Page     int                       `json:"page"`
	PageSize int                       `json:"pageSize"`
	Stats    adminEnterpriseStats      `json:"stats"`
	Filters  adminEnterpriseFilters    `json:"filters"`
}

type adminEnterpriseProfile struct {
	LegalName               string `json:"legalName,omitempty"`
	UnifiedSocialCreditCode string `json:"unifiedSocialCreditCode,omitempty"`
	LegalRepresentativeName string `json:"legalRepresentativeName,omitempty"`
	Industry                string `json:"industry,omitempty"`
	CompanySize             string `json:"companySize,omitempty"`
	OwnerUserID             string `json:"ownerUserId,omitempty"`
}

type adminEnterpriseRecentOperation struct {
	ID        string `json:"id"`
	Actor     string `json:"actor"`
	Action    string `json:"action"`
	Summary   string `json:"summary"`
	CreatedAt string `json:"createdAt"`
}

type adminEnterpriseDetail struct {
	Enterprise        adminEnterpriseListItem          `json:"enterprise"`
	Profile           adminEnterpriseProfile           `json:"profile"`
	OrganizationCount int                              `json:"organizationCount"`
	RecentOperations  []adminEnterpriseRecentOperation `json:"recentOperations"`
	Privacy           map[string]any                   `json:"privacy"`
}

type adminEnterpriseCreateRequest struct {
	Name              string `json:"name"`
	EnterpriseCode    string `json:"enterpriseCode"`
	OwnerUserID       string `json:"ownerUserId"`
	PlanID            string `json:"planId"`
	PlanCode          string `json:"planCode"`
	SeatLimit         int    `json:"seatLimit"`
	Industry          string `json:"industry"`
	CompanySize       string `json:"companySize"`
	SourceAgentID     string `json:"sourceAgentId"`
	OperationCenterID string `json:"operationCenterId"`
}

type adminEnterpriseSectionResult struct {
	Section    string                         `json:"section"`
	Enterprise adminEnterpriseRelationSummary `json:"enterprise"`
	Summary    map[string]any                 `json:"summary"`
	Items      []map[string]any               `json:"items"`
	Total      int                            `json:"total"`
	Unit       string                         `json:"unit,omitempty"`
	Privacy    map[string]any                 `json:"privacy"`
}

type adminEnterpriseMutationRequest struct {
	Action            string         `json:"action"`
	RequestID         string         `json:"requestId"`
	Reason            string         `json:"reason"`
	Status            string         `json:"status,omitempty"`
	ReviewComment     string         `json:"reviewComment,omitempty"`
	PlanID            string         `json:"planId,omitempty"`
	PlanCode          string         `json:"planCode,omitempty"`
	ExpiresAt         string         `json:"expiresAt,omitempty"`
	SeatLimit         int            `json:"seatLimit,omitempty"`
	PointDelta        int64          `json:"pointDelta,omitempty"`
	AmountCents       int64          `json:"amountCents,omitempty"`
	RechargeUnits     int64          `json:"rechargeUnits,omitempty"`
	BonusUnits        int64          `json:"bonusUnits,omitempty"`
	SourceAgentID     string         `json:"sourceAgentId,omitempty"`
	OperationCenterID string         `json:"operationCenterId,omitempty"`
	Name              string         `json:"name,omitempty"`
	Industry          string         `json:"industry,omitempty"`
	CompanySize       string         `json:"companySize,omitempty"`
	ModuleCode        string         `json:"moduleCode,omitempty"`
	ModelName         string         `json:"modelName,omitempty"`
	Limits            map[string]any `json:"limits,omitempty"`
}

func validateAdminEnterpriseMutation(request adminEnterpriseMutationRequest) error {
	switch request.Action {
	case "certification-review":
		status := strings.ToUpper(strings.TrimSpace(request.Status))
		if status != "APPROVED" && status != "REJECTED" {
			return fmt.Errorf("%w: certification status must be APPROVED or REJECTED", errEnterpriseInvalid)
		}
		if status == "REJECTED" && strings.TrimSpace(request.ReviewComment) == "" {
			return fmt.Errorf("%w: reviewComment is required when rejecting certification", errEnterpriseInvalid)
		}
	case "package-adjust":
		if strings.TrimSpace(request.PlanID) == "" && strings.TrimSpace(request.PlanCode) == "" {
			return fmt.Errorf("%w: planId or planCode is required", errEnterpriseInvalid)
		}
		if value := strings.TrimSpace(request.ExpiresAt); value != "" {
			if _, err := time.Parse(time.RFC3339, value); err != nil {
				return fmt.Errorf("%w: expiresAt must use RFC3339", errEnterpriseInvalid)
			}
		}
	case "seat-adjust":
		if request.SeatLimit < 1 || request.SeatLimit > 100000 {
			return fmt.Errorf("%w: seatLimit must be between 1 and 100000", errEnterpriseInvalid)
		}
	case "compute-adjust":
		if request.PointDelta == 0 {
			return fmt.Errorf("%w: pointDelta must not be zero", errEnterpriseInvalid)
		}
	case "recharge":
		if request.RechargeUnits <= 0 && request.PointDelta <= 0 {
			return fmt.Errorf("%w: rechargeUnits or pointDelta must be greater than zero", errEnterpriseInvalid)
		}
		if request.AmountCents < 0 || request.BonusUnits < 0 {
			return fmt.Errorf("%w: amountCents and bonusUnits must not be negative", errEnterpriseInvalid)
		}
	case "service-state":
		status := strings.ToUpper(strings.TrimSpace(request.Status))
		if status != "ACTIVE" && status != "PAUSED" && status != "DISABLED" && status != "TERMINATED" {
			return fmt.Errorf("%w: service state must be ACTIVE, PAUSED, DISABLED or TERMINATED", errEnterpriseInvalid)
		}
	}
	return nil
}

type adminEnterpriseMutationResult struct {
	RequestID  string         `json:"requestId"`
	Action     string         `json:"action"`
	Status     string         `json:"status"`
	Enterprise string         `json:"enterpriseId"`
	Before     map[string]any `json:"before"`
	After      map[string]any `json:"after"`
	AuditID    string         `json:"auditId,omitempty"`
	Message    string         `json:"message"`
}

type adminEnterpriseStore interface {
	ListAdminEnterprises(adminEnterpriseListQuery) (adminEnterpriseListResult, error)
	GetAdminEnterprise(string) (adminEnterpriseDetail, error)
	CreateAdminEnterprise(actorID string, actorRole string, request adminEnterpriseCreateRequest) (adminEnterpriseDetail, error)
	GetAdminEnterpriseSection(string, string) (adminEnterpriseSectionResult, error)
	ListAdminEnterpriseCertifications() (adminEnterpriseSectionResult, error)
	MutateAdminEnterprise(string, string, string, adminEnterpriseMutationRequest) (adminEnterpriseMutationResult, error)
}

func adminEnterprisePrivacyBoundary() map[string]any {
	return map[string]any{
		"summaryOnly": true,
		"message":     "主控仅返回企业治理、计费与使用统计，不返回知识库正文、原始文件、作品、对话、提示词或 AI 员工任务输入输出。",
		"restrictedFields": []string{
			"knowledgeBaseContent", "originalFileContent", "workContent", "conversationContent", "promptContent", "aiEmployeeTaskContent",
		},
	}
}
