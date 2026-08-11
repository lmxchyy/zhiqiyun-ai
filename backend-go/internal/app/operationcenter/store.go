package operationcenter

import (
	"context"
	"database/sql"
	"time"
)

type Store interface {
	CreateServiceOrder(context.Context, *OperationCenterServiceOrder) error
	GetServiceOrderForUpdate(context.Context, string) (*OperationCenterServiceOrder, error)
	GetServiceOrder(context.Context, string) (*OperationCenterServiceOrder, error)
	FindServiceOrderByCommercialOrderID(context.Context, string) (*OperationCenterServiceOrder, error)
	FindServiceOrderByPaymentRecordID(context.Context, string) (*OperationCenterServiceOrder, error)
	UpdateServiceOrder(context.Context, *OperationCenterServiceOrder, *OperationCenterStateTransition) error
	UpdateServiceOrderRefundProjection(context.Context, *OperationCenterServiceOrder) error

	CreateReviewEvent(context.Context, *OperationCenterReviewEvent) error
	GetReviewEventByIdempotencyKey(context.Context, string, string) (*OperationCenterReviewEvent, error)
	UpdateReviewEvent(context.Context, *OperationCenterReviewEvent) error

	CreateRefundTask(context.Context, *OperationCenterRefundTask) error
	GetRefundTaskByIdempotencyKey(context.Context, string) (*OperationCenterRefundTask, error)
	GetRefundTaskForUpdate(context.Context, string) (*OperationCenterRefundTask, error)
	GetRefundTask(context.Context, string) (*OperationCenterRefundTask, error)
	UpdateRefundTask(context.Context, *OperationCenterRefundTask, *OperationCenterStateTransition) error
	UpdateRefundTaskLease(context.Context, *OperationCenterRefundTask) error
	UpdateRefundTaskVerification(context.Context, *OperationCenterRefundTask) error
	ClaimRefundTasks(context.Context, time.Time, string, time.Time, int) ([]OperationCenterRefundTask, error)
	ClaimRetryableRefundTasks(context.Context, time.Time, string, time.Time, int) ([]OperationCenterRefundTask, error)
	ClaimVerificationRefundTasks(context.Context, time.Time, string, time.Time, int) ([]OperationCenterRefundTask, error)
	CreateRefundRequestEvent(context.Context, *RefundRequestEvent) error
	GetRefundRequestEventByIdempotencyKey(context.Context, string, string) (*RefundRequestEvent, error)

	SubmitManualRefund(context.Context, *OperationCenterManualRefund) error
	ApproveManualRefund(context.Context, string, string, time.Time, OperationCenterStateTransition) (*OperationCenterManualRefund, error)
	GetManualRefundByID(context.Context, string) (*OperationCenterManualRefund, error)
	GetManualRefundByProviderTransactionForUpdate(context.Context, string, string) (*OperationCenterManualRefund, error)
	GetLatestManualRefundForUpdate(context.Context, string) (*OperationCenterManualRefund, error)
	UpdateManualRefundRecord(context.Context, *OperationCenterManualRefund) error
	CreateManualRefundEvent(context.Context, *ManualRefundEvent) error
	GetManualRefundEventByIdempotencyKey(context.Context, string, string) (*ManualRefundEvent, error)

	CreateRewardReleaseTask(context.Context, *ReferralRewardReleaseTask) error
	GetRewardReleaseTaskByIdempotencyKey(context.Context, string, string) (*ReferralRewardReleaseTask, error)
	UpdateRewardReleaseTask(context.Context, *ReferralRewardReleaseTask, *OperationCenterStateTransition) error
	ClaimDueRewardReleaseTasks(context.Context, time.Time, string, time.Time, int) ([]ReferralRewardReleaseTask, error)

	AppendStateTransition(context.Context, *OperationCenterStateTransition) error
}

type transactionBinder interface {
	BindTx(*sql.Tx) (Store, error)
}
