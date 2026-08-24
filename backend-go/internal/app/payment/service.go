package payment

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	billingdomain "xianzhi-ai/backend-go/internal/billing"
)

type CommissionHook func(context.Context, *sql.Tx, Order) error

type PersonalPointGrantRequest struct {
	UserID, TenantID, Source, ReferenceType, ReferenceID, IdempotencyKey string
	Points                                                               int64
	GrantedAt                                                            time.Time
}

type PersonalPointGrantResult struct {
	AccountID, UserID string
	AvailableBefore   int64
	AvailableAfter    int64
	FrozenBefore      int64
	FrozenAfter       int64
}

type PersonalPointGrantHook func(context.Context, *sql.Tx, PersonalPointGrantRequest) (PersonalPointGrantResult, error)

var ErrPersonalPointGrantHookUnavailable = errors.New("personal point grant hook is unavailable")

type ServiceOption func(*Service)

func WithPersonalPointGrantHook(hook PersonalPointGrantHook) ServiceOption {
	return func(service *Service) { service.personalPointGrant = hook }
}

type Service struct {
	db                 *sql.DB
	providers          map[string]PaymentProvider
	commission         CommissionHook
	personalPointGrant PersonalPointGrantHook
	logger             func(string, ...any)
	now                func() time.Time
}

type CreateOrderResult struct {
	Order         Order          `json:"order"`
	PaymentNo     string         `json:"paymentNo"`
	PaymentParams map[string]any `json:"paymentParams"`
}

func NewService(db *sql.DB, providers []PaymentProvider, commission CommissionHook, options ...ServiceOption) *Service {
	registry := make(map[string]PaymentProvider, len(providers))
	for _, provider := range providers {
		if provider != nil {
			registry[strings.ToLower(provider.GetProviderName())] = provider
		}
	}
	service := &Service{
		db: db, providers: registry, commission: commission,
		logger: log.Printf, now: func() time.Time { return time.Now().UTC() },
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

func (s *Service) Ready() bool { return s != nil && s.db != nil }

func (s *Service) HasProvider(name string) bool {
	if !s.Ready() {
		return false
	}
	_, ok := s.providers[strings.ToLower(strings.TrimSpace(name))]
	return ok

}
func (s *Service) CreateOrder(ctx context.Context, input CreateOrderInput) (CreateOrderResult, error) {
	if !s.Ready() {
		return CreateOrderResult{}, errors.New("payment database is unavailable")
	}
	input.ProductCode = strings.ToUpper(strings.TrimSpace(input.ProductCode))
	input.Platform = strings.ToLower(strings.TrimSpace(input.Platform))
	input.PaymentChannel = strings.ToLower(strings.TrimSpace(input.PaymentChannel))
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.Quantity == 0 {
		input.Quantity = 1
	}
	if err := input.Validate(); err != nil {
		return CreateOrderResult{}, err
	}
	provider, err := ProviderByName(s.providers, input.PaymentChannel)
	if err != nil {
		return CreateOrderResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return CreateOrderResult{}, err
	}
	defer tx.Rollback()

	if existing, paymentNo, found, findErr := findOrderByIdempotencyTx(ctx, tx, input.UserID, input.IdempotencyKey); findErr != nil {
		return CreateOrderResult{}, findErr
	} else if found {
		if existing.ProductCode != input.ProductCode || existing.Quantity != input.Quantity ||
			existing.Platform != input.Platform || existing.Channel != input.PaymentChannel {
			return CreateOrderResult{}, E(CodeIdempotencyConflict, "Idempotency-Key was already used with a different request")
		}
		if err := tx.Commit(); err != nil {
			return CreateOrderResult{}, err
		}
		return CreateOrderResult{Order: existing, PaymentNo: paymentNo, PaymentParams: map[string]any{"idempotentReplay": true}}, nil
	}

	product, price, err := activeProductPriceTx(ctx, tx, input.ProductCode, input.PaymentChannel, input.Platform)
	if err != nil {
		return CreateOrderResult{}, err
	}
	amount, err := checkedMultiply(input.Quantity, price.Amount)
	if err != nil {
		return CreateOrderResult{}, err
	}
	now := s.now()
	orderNo := newBusinessNo("ZQP", now)
	paymentNo := newBusinessNo("PAY", now)
	orderID := "payment_order_" + randomHex(16)
	order := Order{
		ID: orderID, OrderNo: orderNo, UserID: input.UserID, TenantID: input.TenantID,
		ProductID: product.ID, SourcePlanID: product.SourcePlanID, ProductCode: product.Code,
		ProductName: product.Name, ProductType: product.ProductType, FulfillmentType: product.FulfillmentType,
		Quantity: input.Quantity, Currency: normalizeCurrency(price.Currency),
		OriginalAmount: amount, PayableAmount: amount, Platform: input.Platform, Channel: input.PaymentChannel,
		OrderStatus: OrderCreated, PaymentStatus: PaymentInit, FulfillmentStatus: FulfillmentPending,
		IdempotencyKey: input.IdempotencyKey, ClientIP: input.ClientIP,
		FulfillmentPayload: product.FulfillmentPayload, CreatedAt: now,
	}
	snapshot, _ := json.Marshal(map[string]any{
		"productId": product.ID, "sourcePlanId": product.SourcePlanID, "productCode": product.Code,
		"productName": product.Name, "productType": product.ProductType, "fulfillmentType": product.FulfillmentType,
		"fulfillmentPayload": json.RawMessage(safeJSON(product.FulfillmentPayload)), "quantity": input.Quantity,
		"unitAmount": price.Amount, "originalAmount": amount, "discountAmount": 0, "payableAmount": amount,
		"currency": order.Currency, "platform": input.Platform, "channel": input.PaymentChannel,
	})
	raw, _ := json.Marshal(order)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO xz_orders(
		  id,order_no,user_id,buyer_user_id,tenant_id,plan_id,product_id,product_code,product_name,product_type,
		  quantity,currency,amount_cents,original_amount_cents,discount_amount_cents,payable_amount_cents,
		  platform,channel,payment_channel,payment_scene,status,order_status,fulfillment_status,entitlement_status,
		  idempotency_key,client_ip,created_at,updated_at,price_snapshot,raw
		) VALUES (
		  $1,$2,$3,$3,$4,nullif($5,''),$6,$7,$8,$9,$10,$11,$12,$12,0,$12,
		  $13,$14,$14,'UNIFIED_PAYMENT',$15,$15,$16,'PENDING',$17,$18,$19,$20,$21::jsonb,$22::jsonb
		)
	`, order.ID, order.OrderNo, order.UserID, order.TenantID, order.SourcePlanID, order.ProductID,
		order.ProductCode, order.ProductName, order.ProductType, order.Quantity, order.Currency, order.PayableAmount,
		order.Platform, order.Channel, order.OrderStatus, order.FulfillmentStatus, order.IdempotencyKey,
		order.ClientIP, now.Format(time.RFC3339Nano), now, snapshot, raw)
	if err != nil {
		if isUniqueViolation(err) {
			return CreateOrderResult{}, E(CodeIdempotencyConflict, "duplicate order request", err)
		}
		return CreateOrderResult{}, err
	}
	requestPayload, _ := json.Marshal(map[string]any{
		"orderNo": orderNo, "paymentNo": paymentNo, "amount": amount, "currency": order.Currency,
		"subject": order.ProductName, "clientIpMasked": maskIP(input.ClientIP),
	})
	_, err = tx.ExecContext(ctx, `
		INSERT INTO xz_payment_records(
		  id,payment_no,order_id,order_no,tenant_id,user_id,payment_channel,payment_scene,
		  amount_cents,prepay_status,provider,currency,payment_status,request_payload,created_at,updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,'UNIFIED_PAYMENT',$8,'INIT',$7,$9,'INIT',$10::jsonb,$11,$11)
	`, "payment_tx_"+randomHex(16), paymentNo, order.ID, order.OrderNo, order.TenantID, order.UserID,
		order.Channel, order.PayableAmount, order.Currency, requestPayload, now)
	if err != nil {
		return CreateOrderResult{}, err
	}

	providerResult, err := provider.CreatePayment(ctx, CreatePaymentRequest{
		OrderNo: orderNo, PaymentNo: paymentNo, Amount: amount, Currency: order.Currency, Subject: order.ProductName,
	})
	if err != nil {
		return CreateOrderResult{}, err
	}
	if err := ValidateOrderTransition(OrderCreated, OrderPaying); err != nil {
		return CreateOrderResult{}, err
	}
	if err := ValidatePaymentTransition(PaymentInit, providerResult.PaymentStatus); err != nil {
		return CreateOrderResult{}, err
	}
	responsePayload, _ := json.Marshal(providerResult)
	_, err = tx.ExecContext(ctx, `
		UPDATE xz_payment_records SET prepay_status=$2,payment_status=$2,provider_trade_no=$3,
		  response_payload=$4::jsonb,updated_at=now() WHERE payment_no=$1
	`, paymentNo, providerResult.PaymentStatus, providerResult.ProviderTradeNo, responsePayload)
	if err != nil {
		return CreateOrderResult{}, err
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE xz_orders SET status=$2,order_status=$2,updated_at=now() WHERE id=$1
	`, order.ID, OrderPaying)
	if err != nil {
		return CreateOrderResult{}, err
	}
	if err := insertAuditTx(ctx, tx, input.UserID, "payment.order.created", "payment_order", orderNo, input.ClientIP,
		map[string]any{"productCode": order.ProductCode, "amount": order.PayableAmount, "currency": order.Currency, "channel": order.Channel}); err != nil {
		return CreateOrderResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return CreateOrderResult{}, err
	}
	order.OrderStatus = OrderPaying
	order.PaymentStatus = providerResult.PaymentStatus
	s.logger("payment order created order_no=%s user_id=%s tenant_id=%s amount=%d currency=%s channel=%s",
		order.OrderNo, order.UserID, order.TenantID, order.PayableAmount, order.Currency, order.Channel)
	return CreateOrderResult{Order: order, PaymentNo: paymentNo, PaymentParams: providerResult.PaymentParams}, nil
}

func (s *Service) GetOrder(ctx context.Context, orderNo string) (Order, error) {
	return getOrder(ctx, s.db, orderNo)
}

func (s *Service) HandlePaymentNotification(ctx context.Context, notification PaymentNotification) error {
	if err := notification.Validate(); err != nil {
		return err
	}
	if notification.Status == PaymentFailed {
		return s.handlePaymentFailure(ctx, notification)
	}
	if notification.Status != PaymentSuccess {
		return E(CodeInvalidRequest, "only SUCCESS or FAILED notifications are supported")
	}
	err := s.handlePaymentSuccessTx(ctx, notification)
	if err == nil {
		return nil
	}
	if ErrorCodeOf(err) == CodePaymentMismatch || ErrorCodeOf(err) == CodeDuplicateTransaction ||
		ErrorCodeOf(err) == CodeInvalidTransition {
		s.logger("payment notification rejected code=%s order_no=%s provider=%s transaction_id=%s err=%v",
			ErrorCodeOf(err), notification.OrderNo, notification.Provider, notification.ProviderTransactionID, err)
		return err
	}
	if persistErr := s.persistPaidFulfillmentFailure(ctx, notification, err); persistErr != nil {
		s.logger("payment fulfillment failure persistence failed order_no=%s cause=%v persist_err=%v", notification.OrderNo, err, persistErr)
	}
	return err
}

func (s *Service) handlePaymentSuccessTx(ctx context.Context, notification PaymentNotification) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	order, paymentNo, err := lockOrderAndPaymentTx(ctx, tx, notification.OrderNo)
	if err != nil {
		return err
	}
	if !strings.EqualFold(order.Channel, notification.Provider) || order.PayableAmount != notification.Amount ||
		!strings.EqualFold(order.Currency, notification.Currency) {
		return E(CodePaymentMismatch, "payment amount, currency, provider or order does not match")
	}
	var otherOrder string
	err = tx.QueryRowContext(ctx, `
		SELECT order_no FROM xz_payment_records
		WHERE provider=$1 AND provider_transaction_id=$2 AND order_no<>$3 LIMIT 1
	`, strings.ToLower(notification.Provider), notification.ProviderTransactionID, order.OrderNo).Scan(&otherOrder)
	if err == nil {
		return E(CodeDuplicateTransaction, "provider transaction is already bound to another order")
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if order.PaymentStatus != PaymentSuccess {
		if err := ValidatePaymentTransition(order.PaymentStatus, PaymentSuccess); err != nil {
			return err
		}
		if err := ValidateOrderTransition(order.OrderStatus, OrderPaid); err != nil {
			return err
		}
	}
	paidAt := notification.PaidAt.UTC()
	if paidAt.IsZero() {
		paidAt = s.now()
	}
	notifyPayload := safeJSON(notification.Payload)
	_, err = tx.ExecContext(ctx, `
		UPDATE xz_payment_records
		SET prepay_status='SUCCESS',payment_status='SUCCESS',provider_trade_no=nullif($2,''),
		    provider_transaction_id=$3,notify_payload=$4::jsonb,callback_payload=$4::jsonb,
		    paid_at=$5,failure_code='',failure_message='',failure_reason='',updated_at=now()
		WHERE payment_no=$1
	`, paymentNo, notification.ProviderTradeNo, notification.ProviderTransactionID, notifyPayload, paidAt)
	if err != nil {
		if isUniqueViolation(err) {
			return E(CodeDuplicateTransaction, "provider transaction is already used", err)
		}
		return err
	}
	if order.OrderStatus != OrderPaid && order.OrderStatus != OrderFulfilling {
		_, err = tx.ExecContext(ctx, `UPDATE xz_orders SET status='PAID',order_status='PAID',paid_at=$2,updated_at=now() WHERE id=$1`,
			order.ID, paidAt.Format(time.RFC3339Nano))
		if err != nil {
			return err
		}
		order.OrderStatus = OrderPaid
	}
	if order.FulfillmentStatus == FulfillmentSuccess && order.OrderStatus == OrderCompleted {
		return tx.Commit()
	}
	if order.OrderStatus == OrderPaid {
		if err := ValidateOrderTransition(OrderPaid, OrderFulfilling); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE xz_orders SET status='FULFILLING',order_status='FULFILLING',
		  fulfillment_status='PROCESSING',entitlement_status='PROCESSING',entitlement_error='',updated_at=now()
		WHERE id=$1
	`, order.ID)
	if err != nil {
		return err
	}
	order.OrderStatus = OrderFulfilling
	order.PaymentStatus = PaymentSuccess
	order.FulfillmentStatus = FulfillmentProcessing
	if err := upsertFulfillmentProcessingTx(ctx, tx, order); err != nil {
		return err
	}
	handler := fulfillmentHandler(order.FulfillmentType, s.personalPointGrant)
	if handler == nil {
		return E(CodeFulfillmentUnsupported, fmt.Sprintf("fulfillment type %s is not supported in phase 1", order.FulfillmentType))
	}
	if err := handler.Fulfill(ctx, tx, order); err != nil {
		return E(CodeFulfillmentFailed, "fulfillment failed", err)
	}
	if s.commission != nil {
		if err := s.commission(ctx, tx, order); err != nil {
			return E(CodeFulfillmentFailed, "commission snapshot failed", err)
		}
	}
	fulfilledAt := s.now()
	_, err = tx.ExecContext(ctx, `
		UPDATE xz_fulfillment_records SET fulfillment_status='SUCCESS',failure_message='',
		  fulfilled_at=$2,updated_at=now() WHERE order_no=$1 AND fulfillment_type=$3
	`, order.OrderNo, fulfilledAt, order.FulfillmentType)
	if err != nil {
		return err
	}
	if err := ValidateOrderTransition(OrderFulfilling, OrderCompleted); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE xz_orders SET status='COMPLETED',order_status='COMPLETED',fulfillment_status='SUCCESS',
		  entitlement_status='SUCCESS',entitlement_error='',fulfilled_at=$2,entitlement_granted_at=$3,updated_at=now()
		WHERE id=$1
	`, order.ID, fulfilledAt.Format(time.RFC3339Nano), fulfilledAt)
	if err != nil {
		return err
	}
	if err := insertAuditTx(ctx, tx, order.UserID, "payment.order.completed", "payment_order", order.OrderNo, "",
		map[string]any{"provider": notification.Provider, "providerTransactionId": notification.ProviderTransactionID, "amount": order.PayableAmount}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.logger("payment completed order_no=%s user_id=%s transaction_id=%s", order.OrderNo, order.UserID, notification.ProviderTransactionID)
	return nil
}

func (s *Service) persistPaidFulfillmentFailure(ctx context.Context, notification PaymentNotification, cause error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	order, paymentNo, err := lockOrderAndPaymentTx(ctx, tx, notification.OrderNo)
	if err != nil {
		return err
	}
	if order.PayableAmount != notification.Amount || !strings.EqualFold(order.Currency, notification.Currency) ||
		!strings.EqualFold(order.Channel, notification.Provider) {
		return E(CodePaymentMismatch, "cannot persist mismatched payment")
	}
	message := truncate(cause.Error(), 500)
	paidAt := notification.PaidAt.UTC()
	if paidAt.IsZero() {
		paidAt = s.now()
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE xz_payment_records SET prepay_status='SUCCESS',payment_status='SUCCESS',
		  provider_transaction_id=$2,notify_payload=$3::jsonb,callback_payload=$3::jsonb,paid_at=$4,
		  failure_code=$5,failure_message=$6,failure_reason=$6,updated_at=now() WHERE payment_no=$1
	`, paymentNo, notification.ProviderTransactionID, safeJSON(notification.Payload), paidAt, ErrorCodeOf(cause), message)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE xz_orders SET status='FULFILLING',order_status='FULFILLING',paid_at=$2,
		  fulfillment_status='FAILED',entitlement_status='FAILED',entitlement_error=$3,updated_at=now() WHERE id=$1
	`, order.ID, paidAt.Format(time.RFC3339Nano), message)
	if err != nil {
		return err
	}
	order.FulfillmentStatus = FulfillmentFailed
	if err := upsertFulfillmentFailedTx(ctx, tx, order, message); err != nil {
		return err
	}
	if err := insertAuditTx(ctx, tx, order.UserID, "payment.fulfillment.failed", "payment_order", order.OrderNo, "",
		map[string]any{"code": ErrorCodeOf(cause), "message": message}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) handlePaymentFailure(ctx context.Context, notification PaymentNotification) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	order, paymentNo, err := lockOrderAndPaymentTx(ctx, tx, notification.OrderNo)
	if err != nil {
		return err
	}
	if order.PaymentStatus == PaymentFailed {
		return tx.Commit()
	}
	if err := ValidatePaymentTransition(order.PaymentStatus, PaymentFailed); err != nil {
		return err
	}
	if err := ValidateOrderTransition(order.OrderStatus, OrderFailed); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE xz_payment_records SET prepay_status='FAILED',payment_status='FAILED',notify_payload=$2::jsonb,updated_at=now() WHERE payment_no=$1`,
		paymentNo, safeJSON(notification.Payload))
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE xz_orders SET status='FAILED',order_status='FAILED',updated_at=now() WHERE id=$1`, order.ID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) RetryFulfillment(ctx context.Context, fulfillmentID string) error {
	var notification PaymentNotification
	err := s.db.QueryRowContext(ctx, `
		SELECT p.order_no,coalesce(p.provider,p.payment_channel),coalesce(p.provider_trade_no,''),
		  coalesce(p.provider_transaction_id,''),p.amount_cents,p.currency,coalesce(p.notify_payload,'{}'::jsonb),coalesce(p.paid_at,now())
		FROM xz_fulfillment_records f JOIN xz_payment_records p ON p.order_no=f.order_no
		WHERE f.id=$1 AND f.fulfillment_status='FAILED'
	`, strings.TrimSpace(fulfillmentID)).Scan(&notification.OrderNo, &notification.Provider, &notification.ProviderTradeNo,
		&notification.ProviderTransactionID, &notification.Amount, &notification.Currency, &notification.Payload, &notification.PaidAt)
	if errors.Is(err, sql.ErrNoRows) {
		return E(CodeOrderNotFound, "failed fulfillment record not found")
	}
	if err != nil {
		return err
	}
	notification.Status = PaymentSuccess
	return s.HandlePaymentNotification(ctx, notification)
}

func activeProductPriceTx(ctx context.Context, tx *sql.Tx, code, channel, platform string) (Product, Price, error) {
	var product Product
	var price Price
	var payload []byte
	err := tx.QueryRowContext(ctx, `
		SELECT p.id,coalesce(p.source_plan_id,''),p.product_code,p.product_name,p.product_type,
		  p.fulfillment_type,p.description,p.status,p.fulfillment_payload,
		  coalesce(pr.id,''),coalesce(pr.product_id,''),coalesce(pr.channel,''),coalesce(pr.platform,''),
		  coalesce(pr.currency,'CNY'),coalesce(pr.amount,0),coalesce(pr.external_product_id,''),coalesce(pr.status,'')
		FROM xz_payment_products p
		LEFT JOIN xz_product_prices pr ON pr.product_id=p.id AND lower(pr.channel)=lower($2) AND lower(pr.platform)=lower($3)
		WHERE upper(p.product_code)=upper($1)
	`, code, channel, platform).Scan(
		&product.ID, &product.SourcePlanID, &product.Code, &product.Name, &product.ProductType,
		&product.FulfillmentType, &product.Description, &product.Status, &payload,
		&price.ID, &price.ProductID, &price.Channel, &price.Platform, &price.Currency, &price.Amount, &price.ExternalID, &price.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return Product{}, Price{}, E(CodeProductNotFound, "product does not exist")
	}
	if err != nil {
		return Product{}, Price{}, err
	}
	product.FulfillmentPayload = payload
	if !strings.EqualFold(product.Status, "ACTIVE") {
		return Product{}, Price{}, E(CodeProductInactive, "product is inactive")
	}
	if price.ID == "" || !strings.EqualFold(price.Status, "ACTIVE") {
		return Product{}, Price{}, E(CodePriceNotConfigured, "channel/platform price is not configured or inactive")
	}
	return product, price, nil
}

func findOrderByIdempotencyTx(ctx context.Context, tx *sql.Tx, userID, key string) (Order, string, bool, error) {
	var orderNo, paymentNo string
	err := tx.QueryRowContext(ctx, `
		SELECT o.order_no,p.payment_no FROM xz_orders o JOIN xz_payment_records p ON p.order_id=o.id
		WHERE o.user_id=$1 AND o.idempotency_key=$2
	`, userID, key).Scan(&orderNo, &paymentNo)
	if errors.Is(err, sql.ErrNoRows) {
		return Order{}, "", false, nil
	}
	if err != nil {
		return Order{}, "", false, err
	}
	order, err := getOrder(ctx, tx, orderNo)
	return order, paymentNo, err == nil, err
}

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func getOrder(ctx context.Context, q queryer, orderNo string) (Order, error) {
	var order Order
	var createdAt, paidAt, fulfilledAt string
	var payload []byte
	err := q.QueryRowContext(ctx, `
		SELECT o.id,o.order_no,o.user_id,coalesce(o.tenant_id,''),coalesce(o.product_id,''),
		  coalesce(o.plan_id,''),coalesce(o.product_code,''),coalesce(o.product_name,''),coalesce(o.product_type,''),
		  coalesce(pp.fulfillment_type,''),o.quantity,o.currency,o.original_amount_cents,o.discount_amount_cents,
		  o.payable_amount_cents,coalesce(o.platform,''),coalesce(o.channel,o.payment_channel,''),
		  coalesce(o.order_status,o.status,'CREATED'),coalesce(pr.payment_status,pr.prepay_status,'INIT'),
		  coalesce(o.fulfillment_status,'PENDING'),coalesce(o.idempotency_key,''),coalesce(o.client_ip,''),
		  coalesce(pp.fulfillment_payload,'{}'::jsonb),coalesce(o.created_at::text,''),
		  coalesce(o.paid_at::text,''),coalesce(o.fulfilled_at::text,'')
		FROM xz_orders o
		LEFT JOIN xz_payment_products pp ON pp.id=o.product_id
		LEFT JOIN LATERAL (SELECT * FROM xz_payment_records item WHERE item.order_id=o.id ORDER BY item.created_at DESC LIMIT 1) pr ON true
		WHERE o.order_no=$1
	`, strings.TrimSpace(orderNo)).Scan(
		&order.ID, &order.OrderNo, &order.UserID, &order.TenantID, &order.ProductID, &order.SourcePlanID,
		&order.ProductCode, &order.ProductName, &order.ProductType, &order.FulfillmentType, &order.Quantity,
		&order.Currency, &order.OriginalAmount, &order.DiscountAmount, &order.PayableAmount, &order.Platform,
		&order.Channel, &order.OrderStatus, &order.PaymentStatus, &order.FulfillmentStatus, &order.IdempotencyKey,
		&order.ClientIP, &payload, &createdAt, &paidAt, &fulfilledAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Order{}, E(CodeOrderNotFound, "order not found")
	}
	if err != nil {
		return Order{}, err
	}
	order.FulfillmentPayload = payload
	order.CreatedAt = parseDBTime(createdAt)
	order.PaidAt = parseOptionalDBTime(paidAt)
	order.FulfilledAt = parseOptionalDBTime(fulfilledAt)
	return order, nil
}

func lockOrderAndPaymentTx(ctx context.Context, tx *sql.Tx, orderNo string) (Order, string, error) {
	order, err := getOrder(ctx, tx, orderNo)
	if err != nil {
		return Order{}, "", err
	}
	var paymentNo string
	if err := tx.QueryRowContext(ctx, `SELECT payment_no FROM xz_payment_records WHERE order_id=$1 ORDER BY created_at DESC LIMIT 1 FOR UPDATE`, order.ID).Scan(&paymentNo); err != nil {
		return Order{}, "", err
	}
	if err := tx.QueryRowContext(ctx, `SELECT id FROM xz_orders WHERE id=$1 FOR UPDATE`, order.ID).Scan(&order.ID); err != nil {
		return Order{}, "", err
	}
	order, err = getOrder(ctx, tx, orderNo)
	return order, paymentNo, err
}

func fulfillmentHandler(kind string, personalPointGrant PersonalPointGrantHook) FulfillmentHandler {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "grant_token":
		return GrantTokenHandler{PersonalPointGrant: personalPointGrant}
	default:
		return nil
	}
}

func grantTokenTx(ctx context.Context, tx *sql.Tx, order Order, personalPointGrant PersonalPointGrantHook) error {
	entitlement, err := billingdomain.ParsePaidPointEntitlement(safeJSON(order.FulfillmentPayload))
	if err != nil {
		return err
	}
	if personalPointGrant == nil {
		return ErrPersonalPointGrantHookUnavailable
	}
	idempotencyKey := "unified-payment:" + order.OrderNo + ":grant_token"
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM xz_token_records WHERE idempotency_key=$1)`, idempotencyKey).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	grantedAt := time.Now().UTC()
	grant, err := personalPointGrant(ctx, tx, PersonalPointGrantRequest{
		UserID: order.UserID, TenantID: order.TenantID, Source: "UNIFIED_PAYMENT_GRANT", Points: entitlement.Points,
		ReferenceType: "UNIFIED_PAYMENT_ORDER", ReferenceID: order.OrderNo, IdempotencyKey: idempotencyKey, GrantedAt: grantedAt,
	})
	if err != nil {
		return err
	}
	if grant.UserID != order.UserID || grant.AccountID == "" || grant.AvailableAfter-grant.AvailableBefore != entitlement.Points {
		return errors.New("personal point grant hook returned an invalid result")
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO xz_token_records(
		  id,user_id,order_id,change_type,amount,balance_before,balance_after,remark,
		  created_at,tenant_id,idempotency_key,source_order_no,raw
		) VALUES ($1,$2,$3,'UNIFIED_PAYMENT_GRANT',$4,$5,$6,'unified_payment_grant_token',$7,$8,$9,$3,$10::jsonb)
	`, "token_"+randomHex(16), order.UserID, order.OrderNo, entitlement.Points, grant.AvailableBefore, grant.AvailableAfter,
		grantedAt.Format(time.RFC3339Nano), order.TenantID, idempotencyKey,
		mustJSON(map[string]any{"orderNo": order.OrderNo, "amount": entitlement.Points, "balanceBefore": grant.AvailableBefore, "balanceAfter": grant.AvailableAfter}))
	if err != nil {
		return err
	}
	return nil
}

func upsertFulfillmentProcessingTx(ctx context.Context, tx *sql.Tx, order Order) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO xz_fulfillment_records(id,order_no,user_id,fulfillment_type,fulfillment_status,fulfillment_payload)
		VALUES ($1,$2,$3,$4,'PROCESSING',$5::jsonb)
		ON CONFLICT (order_no,fulfillment_type) DO UPDATE SET
		  fulfillment_status='PROCESSING',failure_message='',retry_count=xz_fulfillment_records.retry_count+1,
		  fulfillment_payload=EXCLUDED.fulfillment_payload,updated_at=now()
	`, "fulfillment_"+randomHex(16), order.OrderNo, order.UserID, order.FulfillmentType, safeJSON(order.FulfillmentPayload))
	return err
}

func upsertFulfillmentFailedTx(ctx context.Context, tx *sql.Tx, order Order, message string) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO xz_fulfillment_records(id,order_no,user_id,fulfillment_type,fulfillment_status,fulfillment_payload,retry_count,failure_message)
		VALUES ($1,$2,$3,$4,'FAILED',$5::jsonb,1,$6)
		ON CONFLICT (order_no,fulfillment_type) DO UPDATE SET
		  fulfillment_status='FAILED',failure_message=EXCLUDED.failure_message,retry_count=xz_fulfillment_records.retry_count+1,
		  fulfillment_payload=EXCLUDED.fulfillment_payload,updated_at=now()
	`, "fulfillment_"+randomHex(16), order.OrderNo, order.UserID, order.FulfillmentType, safeJSON(order.FulfillmentPayload), message)
	return err
}

func insertAuditTx(ctx context.Context, tx *sql.Tx, actorID, action, resource, resourceID, clientIP string, metadata map[string]any) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO xz_audit_logs(id,actor_id,actor_role,action,resource,resource_id,method,path,status,metadata)
		VALUES ($1,nullif($2,''),'USER',$3,$4,$5,'SYSTEM','payment-center',200,$6::jsonb)
	`, "audit_"+randomHex(16), actorID, action, resource, resourceID, mustJSON(metadata))
	return err
}

func newBusinessNo(prefix string, now time.Time) string {
	return fmt.Sprintf("%s%s%s", prefix, now.UTC().Format("20060102150405"), strings.ToUpper(randomHex(5)))
}

func randomHex(bytesCount int) string {
	buffer := make([]byte, bytesCount)
	if _, err := rand.Read(buffer); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(buffer)
}

func checkedAdd(left, right int64) (int64, error) {
	if right <= 0 || left > (1<<63-1)-right {
		return 0, errors.New("token balance would overflow")
	}
	return left + right, nil
}

func parseDBTime(value string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05.999999-07"} {
		if parsed, err := time.Parse(layout, strings.TrimSpace(value)); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

func parseOptionalDBTime(value string) *time.Time {
	if parsed := parseDBTime(value); !parsed.IsZero() {
		return &parsed
	}
	return nil
}

func maskIP(value string) string {
	parts := strings.Split(strings.TrimSpace(value), ".")
	if len(parts) == 4 {
		return strings.Join([]string{parts[0], parts[1], "*", "*"}, ".")
	}
	if value != "" {
		return "***"
	}
	return ""
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}

func mustJSON(value any) []byte {
	result, _ := json.Marshal(value)
	return result
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "duplicate key") || strings.Contains(text, "unique constraint") || strings.Contains(text, "sqlstate 23505")
}
