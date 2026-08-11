package httpserver

import (
	"database/sql"

	"xianzhi-ai/backend-go/internal/app/operationcenter"
)

func newOperationCenterRuntime(db *sql.DB, environment string, virtualPayment *virtualPaymentService) (*operationcenter.OperationCenterRuntime, error) {
	config, err := operationcenter.LoadOperationCenterRuntimeConfig(environment, nil)
	if err != nil {
		return nil, err
	}
	bindings := []operationcenter.RefundProviderBinding{}
	if virtualPayment != nil {
		bindings = append(bindings, operationcenter.RefundProviderBinding{PaymentChannel: "WECHAT_VIRTUAL", Provider: newWechatVirtualRefundProvider(virtualPayment)})
	}
	return operationcenter.NewOperationCenterRuntime(db, config, nil, bindings...)
}
