package httpserver

import "time"

const (
	promotionStatusVisited    = "visited"
	promotionStatusRegistered = "registered"
	promotionStatusPaid       = "paid"
	promotionStatusInvalid    = "invalid"
)

type promotionRecord struct {
	ID                string `json:"id"`
	TenantID          string `json:"tenantId"`
	InviterUserID     string `json:"inviterUserId"`
	InviteeUserID     string `json:"inviteeUserId,omitempty"`
	VisitorID         string `json:"visitorId,omitempty"`
	VisitorName       string `json:"visitorName,omitempty"`
	MaskedMobile      string `json:"maskedMobile,omitempty"`
	InviteCode        string `json:"inviteCode"`
	Status            string `json:"status"`
	Source            string `json:"source"`
	TemplateID        string `json:"templateId"`
	ActivityID        string `json:"activityId,omitempty"`
	VisitTime         string `json:"visitTime,omitempty"`
	RegisterTime      string `json:"registerTime,omitempty"`
	PaidTime          string `json:"paidTime,omitempty"`
	RewardAmountCents int64  `json:"rewardAmountCents"`
	RewardStatus      string `json:"rewardStatus,omitempty"`
	CreatedAt         string `json:"createdAt"`
	UpdatedAt         string `json:"updatedAt"`
}

type promotionVisitInput struct {
	ID            string
	TenantID      string
	InviterUserID string
	VisitorID     string
	VisitorName   string
	MaskedMobile  string
	InviteCode    string
	Source        string
	TemplateID    string
	ActivityID    string
	VisitedAt     time.Time
}

type promotionBindInput struct {
	TenantID      string
	InviterUserID string
	InviteeUserID string
	InviteCode    string
	Source        string
	TemplateID    string
	ActivityID    string
	BoundAt       time.Time
}

type promotionDataStore interface {
	ListPromotionRecords(inviterUserID string, tenantID string) ([]promotionRecord, error)
	RecordPromotionVisit(input promotionVisitInput) (promotionRecord, error)
	BindPromotionInvite(input promotionBindInput) (promotionRecord, error)
}

type promotionTemplate struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	Category        string         `json:"category"`
	CategoryLabel   string         `json:"categoryLabel"`
	AllowedRoles    []string       `json:"allowedRoles"`
	Background      string         `json:"background"`
	PrimaryColor    string         `json:"primaryColor"`
	SecondaryColor  string         `json:"secondaryColor"`
	Title           string         `json:"title"`
	Subtitle        string         `json:"subtitle"`
	Badge           string         `json:"badge"`
	Description     string         `json:"description"`
	FeatureItems    []string       `json:"featureItems"`
	Layout          string         `json:"layout"`
	QRPosition      map[string]int `json:"qrPosition"`
	InviterPosition map[string]int `json:"inviterPosition"`
	ActivityConfig  map[string]any `json:"activityConfig,omitempty"`
}

type promotionActivity struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Badge       string `json:"badge"`
	Description string `json:"description"`
	Status      string `json:"status"`
	StartsAt    string `json:"startsAt,omitempty"`
	EndsAt      string `json:"endsAt,omitempty"`
}
