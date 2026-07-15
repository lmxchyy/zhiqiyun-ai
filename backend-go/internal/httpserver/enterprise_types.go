package httpserver

import "time"

const (
	contextPersonal   = "PERSONAL"
	contextEnterprise = "ENTERPRISE"
	contextAgent      = "AGENT"
	contextOperation  = "OPERATION"

	dataScopeTenantAll      = "TENANT_ALL"
	dataScopeOrgAndChildren = "ORG_AND_CHILDREN"
	dataScopeOrgSelf        = "ORG_SELF"
	dataScopeOwner          = "OWNER"
	dataScopeSelf           = "SELF"
)

type enterpriseWalletSummary struct {
	PointBalance     int64  `json:"pointBalance"`
	FrozenPoints     int64  `json:"frozenPoints"`
	CashBalanceCents int64  `json:"cashBalanceCents"`
	Status           string `json:"status"`
}

type enterpriseContext struct {
	Type                string                  `json:"type"`
	TenantID            string                  `json:"tenantId"`
	TenantName          string                  `json:"tenantName"`
	OrganizationID      string                  `json:"organizationId"`
	OrganizationName    string                  `json:"organizationName"`
	MemberStatus        string                  `json:"memberStatus"`
	CertificationStatus string                  `json:"certificationStatus"`
	Roles               []string                `json:"roles"`
	CurrentRole         string                  `json:"currentRole"`
	Permissions         []string                `json:"permissions"`
	DataScope           string                  `json:"dataScope"`
	Entitlements        map[string]any          `json:"entitlements"`
	Wallet              enterpriseWalletSummary `json:"wallet"`
	Current             bool                    `json:"current"`
}

type enterpriseContextSwitchRequest struct {
	Type           string `json:"type"`
	TenantID       string `json:"tenantId"`
	OrganizationID string `json:"organizationId"`
	Role           string `json:"role"`
}

type enterpriseAccess struct {
	UserID         string
	TenantID       string
	TenantName     string
	OrganizationID string
	MemberID       string
	Role           string
	Roles          []string
	Permissions    []string
	DataScope      string
}

func (a enterpriseAccess) hasPermission(permission string) bool {
	return containsString(a.Permissions, permission)
}

type enterpriseTenant struct {
	ID                  string         `json:"id"`
	Name                string         `json:"name"`
	OwnerUserID         string         `json:"ownerUserId"`
	Status              string         `json:"status"`
	CertificationStatus string         `json:"certificationStatus"`
	Config              map[string]any `json:"config,omitempty"`
	CreatedAt           string         `json:"createdAt"`
	UpdatedAt           string         `json:"updatedAt"`
}

type enterpriseMember struct {
	ID                    string   `json:"id"`
	TenantID              string   `json:"tenantId"`
	UserID                string   `json:"userId"`
	Name                  string   `json:"name"`
	Email                 string   `json:"email"`
	PrimaryOrganizationID string   `json:"primaryOrganizationId"`
	OrganizationName      string   `json:"organizationName"`
	MemberStatus          string   `json:"memberStatus"`
	CertificationStatus   string   `json:"certificationStatus"`
	DataScope             string   `json:"dataScope"`
	Roles                 []string `json:"roles"`
	JoinedAt              string   `json:"joinedAt"`
	InvitedBy             string   `json:"invitedBy,omitempty"`
	CreatedAt             string   `json:"createdAt"`
	UpdatedAt             string   `json:"updatedAt"`
}

type enterpriseInvitation struct {
	ID                    string `json:"id"`
	TenantID              string `json:"tenantId"`
	InvitationCode        string `json:"invitationCode"`
	InvitedUserID         string `json:"invitedUserId,omitempty"`
	InvitedEmail          string `json:"invitedEmail,omitempty"`
	DefaultOrganizationID string `json:"defaultOrganizationId"`
	DefaultRole           string `json:"defaultRole"`
	ExpiresAt             string `json:"expiresAt"`
	Status                string `json:"status"`
	CreatedBy             string `json:"createdBy"`
	AcceptedBy            string `json:"acceptedBy,omitempty"`
	AcceptedAt            string `json:"acceptedAt,omitempty"`
	CreatedAt             string `json:"createdAt"`
	UpdatedAt             string `json:"updatedAt"`
}

type enterpriseJoinRequest struct {
	ID                      string `json:"id"`
	TenantID                string `json:"tenantId"`
	ApplicantUserID         string `json:"applicantUserId"`
	ApplicantName           string `json:"applicantName,omitempty"`
	ApplicantEmail          string `json:"applicantEmail,omitempty"`
	RequestedOrganizationID string `json:"requestedOrganizationId"`
	RequestedRole           string `json:"requestedRole"`
	Reason                  string `json:"reason"`
	Status                  string `json:"status"`
	ReviewedBy              string `json:"reviewedBy,omitempty"`
	ReviewedAt              string `json:"reviewedAt,omitempty"`
	ReviewComment           string `json:"reviewComment,omitempty"`
	CreatedAt               string `json:"createdAt"`
	UpdatedAt               string `json:"updatedAt"`
}

type enterpriseOrganization struct {
	ID               string                   `json:"id"`
	TenantID         string                   `json:"tenantId"`
	ParentID         string                   `json:"parentId,omitempty"`
	OrganizationType string                   `json:"organizationType"`
	Name             string                   `json:"name"`
	Status           string                   `json:"status"`
	Metadata         map[string]any           `json:"metadata,omitempty"`
	MemberCount      int                      `json:"memberCount"`
	Children         []enterpriseOrganization `json:"children,omitempty"`
	CreatedAt        string                   `json:"createdAt"`
	UpdatedAt        string                   `json:"updatedAt"`
}

type enterpriseAuditLog struct {
	ID             string         `json:"id"`
	TenantID       string         `json:"tenantId"`
	ActorUserID    string         `json:"actorUserId"`
	ActorRole      string         `json:"actorRole"`
	OrganizationID string         `json:"organizationId,omitempty"`
	Action         string         `json:"action"`
	ResourceType   string         `json:"resourceType"`
	ResourceID     string         `json:"resourceId,omitempty"`
	TargetUserID   string         `json:"targetUserId,omitempty"`
	Status         string         `json:"status"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	BeforeValue    map[string]any `json:"beforeValue,omitempty"`
	AfterValue     map[string]any `json:"afterValue,omitempty"`
	RequestID      string         `json:"requestId,omitempty"`
	CreatedAt      string         `json:"createdAt"`
}

type enterprisePointTransaction struct {
	ID              string `json:"id"`
	TenantID        string `json:"tenantId"`
	TransactionType string `json:"transactionType"`
	PointDelta      int64  `json:"pointDelta"`
	BalanceAfter    int64  `json:"balanceAfter"`
	ReferenceType   string `json:"referenceType"`
	ReferenceID     string `json:"referenceId"`
	Reason          string `json:"reason"`
	ActorUserID     string `json:"actorUserId,omitempty"`
	RequestID       string `json:"requestId"`
	CreatedAt       string `json:"createdAt"`
}

type enterpriseCertification struct {
	ID                      string         `json:"id"`
	TenantID                string         `json:"tenantId"`
	LegalName               string         `json:"legalName"`
	UnifiedSocialCreditCode string         `json:"unifiedSocialCreditCode"`
	LegalRepresentativeName string         `json:"legalRepresentativeName"`
	DocumentURLs            []string       `json:"documentUrls"`
	Status                  string         `json:"status"`
	SubmittedBy             string         `json:"submittedBy"`
	ReviewedBy              string         `json:"reviewedBy,omitempty"`
	ReviewedAt              string         `json:"reviewedAt,omitempty"`
	ReviewComment           string         `json:"reviewComment,omitempty"`
	Metadata                map[string]any `json:"metadata,omitempty"`
	CreatedAt               string         `json:"createdAt"`
	UpdatedAt               string         `json:"updatedAt"`
}

type enterpriseCertificationSubmitRequest struct {
	LegalName               string         `json:"legalName"`
	UnifiedSocialCreditCode string         `json:"unifiedSocialCreditCode"`
	LegalRepresentativeName string         `json:"legalRepresentativeName"`
	DocumentURLs            []string       `json:"documentUrls"`
	Metadata                map[string]any `json:"metadata"`
}

type enterpriseSubscriptionSummary struct {
	ID             string         `json:"id"`
	PlanID         string         `json:"planId,omitempty"`
	PlanCode       string         `json:"planCode"`
	Status         string         `json:"status"`
	TrialExpiresAt string         `json:"trialExpiresAt,omitempty"`
	Entitlements   map[string]any `json:"entitlements"`
}

type enterpriseOverview struct {
	Tenant              enterpriseTenant              `json:"tenant"`
	MemberCount         int                           `json:"memberCount"`
	ActiveMembers       int                           `json:"activeMembers"`
	OrganizationCount   int                           `json:"organizationCount"`
	PendingJoinRequests int                           `json:"pendingJoinRequests"`
	Wallet              enterpriseWalletSummary       `json:"wallet"`
	Subscription        enterpriseSubscriptionSummary `json:"subscription"`
	Current             enterpriseContext             `json:"currentContext"`
}

type enterpriseCreateRequest struct {
	Name string `json:"name"`
}

type enterpriseCreateResult struct {
	Tenant       enterpriseTenant       `json:"tenant"`
	Context      enterpriseContext      `json:"context"`
	Invitation   enterpriseInvitation   `json:"invitation"`
	Organization enterpriseOrganization `json:"organization"`
}

type enterpriseInvitationCreateRequest struct {
	InvitedUserID         string `json:"invitedUserId"`
	InvitedEmail          string `json:"invitedEmail"`
	DefaultOrganizationID string `json:"defaultOrganizationId"`
	DefaultRole           string `json:"defaultRole"`
	ExpiresInHours        int    `json:"expiresInHours"`
}

type enterpriseInvitationAcceptRequest struct {
	InvitationCode string `json:"invitationCode"`
}

type enterpriseJoinRequestCreate struct {
	TenantID                string `json:"tenantId"`
	RequestedOrganizationID string `json:"requestedOrganizationId"`
	Reason                  string `json:"reason"`
}

type enterpriseMemberUpdateRequest struct {
	PrimaryOrganizationID string   `json:"primaryOrganizationId"`
	Roles                 []string `json:"roles"`
	DataScope             string   `json:"dataScope"`
}

type enterpriseOrganizationCreateRequest struct {
	ParentID         string         `json:"parentId"`
	OrganizationType string         `json:"organizationType"`
	Name             string         `json:"name"`
	Metadata         map[string]any `json:"metadata"`
}

type enterpriseOrganizationUpdateRequest struct {
	Name             string         `json:"name"`
	OrganizationType string         `json:"organizationType"`
	Status           string         `json:"status"`
	Metadata         map[string]any `json:"metadata"`
}

type enterpriseOrganizationMoveRequest struct {
	ParentID string `json:"parentId"`
}

type enterpriseCurrentState struct {
	Type           string `json:"type"`
	TenantID       string `json:"tenantId"`
	OrganizationID string `json:"organizationId"`
	CurrentRole    string `json:"currentRole"`
}

type enterpriseMemoryState struct {
	Tenants           []enterpriseTenant                       `json:"tenants,omitempty"`
	Members           []enterpriseMember                       `json:"members,omitempty"`
	Invitations       []enterpriseInvitation                   `json:"invitations,omitempty"`
	JoinRequests      []enterpriseJoinRequest                  `json:"joinRequests,omitempty"`
	Organizations     []enterpriseOrganization                 `json:"organizations,omitempty"`
	Wallets           map[string]enterpriseWalletSummary       `json:"wallets,omitempty"`
	Subscriptions     map[string]enterpriseSubscriptionSummary `json:"subscriptions,omitempty"`
	PointTransactions []enterprisePointTransaction             `json:"pointTransactions,omitempty"`
	AuditLogs         []enterpriseAuditLog                     `json:"auditLogs,omitempty"`
	Certifications    []enterpriseCertification                `json:"certifications,omitempty"`
	CurrentContexts   map[string]enterpriseCurrentState        `json:"currentContexts,omitempty"`
}

type enterpriseStore interface {
	EnterpriseContexts(userID string) ([]enterpriseContext, error)
	CurrentEnterpriseContext(userID string) (enterpriseContext, error)
	SetEnterpriseCurrentContext(userID string, request enterpriseContextSwitchRequest) (enterpriseContext, error)
	CreateEnterprise(userID string, request enterpriseCreateRequest) (enterpriseCreateResult, error)
	EnterpriseAccess(userID string, permission string) (enterpriseAccess, error)
	EnterpriseOverview(access enterpriseAccess) (enterpriseOverview, error)
	ListEnterpriseMembers(access enterpriseAccess) ([]enterpriseMember, error)
	GetEnterpriseMember(access enterpriseAccess, id string) (enterpriseMember, error)
	CreateEnterpriseInvitation(access enterpriseAccess, request enterpriseInvitationCreateRequest) (enterpriseInvitation, error)
	AcceptEnterpriseInvitation(userID string, request enterpriseInvitationAcceptRequest) (enterpriseContext, error)
	CreateEnterpriseJoinRequest(userID string, request enterpriseJoinRequestCreate) (enterpriseJoinRequest, error)
	ListEnterpriseJoinRequests(access enterpriseAccess) ([]enterpriseJoinRequest, error)
	ReviewEnterpriseJoinRequest(access enterpriseAccess, id string, approved bool, comment string) (enterpriseJoinRequest, error)
	UpdateEnterpriseMember(access enterpriseAccess, id string, request enterpriseMemberUpdateRequest) (enterpriseMember, error)
	DisableEnterpriseMember(access enterpriseAccess, id string) (enterpriseMember, error)
	RemoveEnterpriseMember(access enterpriseAccess, id string) error
	EnterpriseOrganizationTree(access enterpriseAccess) ([]enterpriseOrganization, error)
	CreateEnterpriseOrganization(access enterpriseAccess, request enterpriseOrganizationCreateRequest) (enterpriseOrganization, error)
	UpdateEnterpriseOrganization(access enterpriseAccess, id string, request enterpriseOrganizationUpdateRequest) (enterpriseOrganization, error)
	MoveEnterpriseOrganization(access enterpriseAccess, id string, request enterpriseOrganizationMoveRequest) (enterpriseOrganization, error)
	DeleteEnterpriseOrganization(access enterpriseAccess, id string) error
	SubmitEnterpriseCertification(access enterpriseAccess, request enterpriseCertificationSubmitRequest) (enterpriseCertification, error)
	EnterpriseAuditLogs(access enterpriseAccess, limit int) ([]enterpriseAuditLog, error)
}

func enterpriseNow() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
