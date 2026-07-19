package payment

import "fmt"

var orderTransitions = map[OrderStatus]map[OrderStatus]struct{}{
	OrderCreated:    setOrders(OrderPaying, OrderClosed, OrderCancelled),
	OrderPaying:     setOrders(OrderPaid, OrderFailed, OrderClosed),
	OrderPaid:       setOrders(OrderFulfilling),
	OrderFulfilling: setOrders(OrderCompleted),
	OrderCompleted:  setOrders(OrderRefunding),
	OrderRefunding:  setOrders(OrderRefunded, OrderPartialRefunded),
}

var paymentTransitions = map[PaymentStatus]map[PaymentStatus]struct{}{
	PaymentInit:      setPayments(PaymentPending, PaymentFailed, PaymentClosed),
	PaymentPending:   setPayments(PaymentSuccess, PaymentFailed, PaymentClosed),
	PaymentSuccess:   setPayments(PaymentRefunding),
	PaymentRefunding: setPayments(PaymentRefunded, PaymentFailed),
}

func ValidateOrderTransition(from, to OrderStatus) error {
	if from == to {
		return nil
	}
	if _, ok := orderTransitions[from][to]; ok {
		return nil
	}
	return E(CodeInvalidTransition, fmt.Sprintf("order state cannot transition from %s to %s", from, to))
}

func ValidatePaymentTransition(from, to PaymentStatus) error {
	if from == to {
		return nil
	}
	if _, ok := paymentTransitions[from][to]; ok {
		return nil
	}
	return E(CodeInvalidTransition, fmt.Sprintf("payment state cannot transition from %s to %s", from, to))
}

func setOrders(values ...OrderStatus) map[OrderStatus]struct{} {
	result := make(map[OrderStatus]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func setPayments(values ...PaymentStatus) map[PaymentStatus]struct{} {
	result := make(map[PaymentStatus]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}
