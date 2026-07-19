package payment

import (
	"context"
	"database/sql"
)

// FulfillmentHandler keeps payment providers isolated from entitlement logic.
type FulfillmentHandler interface {
	GetFulfillmentType() string
	Fulfill(context.Context, *sql.Tx, Order) error
}

type GrantTokenHandler struct{}

func (GrantTokenHandler) GetFulfillmentType() string { return "grant_token" }

func (GrantTokenHandler) Fulfill(ctx context.Context, tx *sql.Tx, order Order) error {
	return grantTokenTx(ctx, tx, order)
}

// Reserved fulfillment contracts intentionally have no phase-1 handler.
const (
	FulfillmentGrantMembership  = "grant_membership"
	FulfillmentGrantImageQuota  = "grant_image_quota"
	FulfillmentGrantVideoCredit = "grant_video_credit"
	FulfillmentUpgradePartner   = "upgrade_partner"
	FulfillmentEnterprisePlan   = "enterprise_plan"
)

// PaymentSuccessHandler is the single entry for verified provider success.
type PaymentSuccessHandler struct {
	service *Service
}

func NewPaymentSuccessHandler(service *Service) PaymentSuccessHandler {
	return PaymentSuccessHandler{service: service}
}

func (h PaymentSuccessHandler) Handle(ctx context.Context, notification PaymentNotification) error {
	if h.service == nil {
		return E(CodeInvalidRequest, "payment success handler is unavailable")
	}
	notification.Status = PaymentSuccess
	return h.service.HandlePaymentNotification(ctx, notification)
}
