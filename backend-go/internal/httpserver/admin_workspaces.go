package httpserver

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type adminWorkspaceItem struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Count       int      `json:"count"`
	Severity    string   `json:"severity"`
	Module      string   `json:"module"`
	Roles       []string `json:"roles"`
}

type adminGlobalSearchItem struct {
	Type        string `json:"type"`
	RecordID    string `json:"recordId"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Module      string `json:"module"`
}

func (a adminAPI) globalSearch(w http.ResponseWriter, r *http.Request) {
	keyword := strings.TrimSpace(r.URL.Query().Get("q"))
	if keyword == "" {
		writeJSON(w, map[string]any{"items": []adminGlobalSearchItem{}})
		return
	}
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	plans := planMap(data.Plans)
	users := userMap(data.Users)
	items := make([]adminGlobalSearchItem, 0, 30)
	for _, user := range data.Users {
		if !containsSearchKeyword(keyword, user.ID, user.Name, user.Email, user.Mobile) {
			continue
		}
		items = append(items, adminGlobalSearchItem{
			Type: "customer", RecordID: user.ID, Title: firstNonEmptyString(user.Name, user.Email, user.ID),
			Description: strings.TrimSpace(strings.Join([]string{user.Email, user.Mobile, planName(plans[user.PlanID])}, " · ")),
			Module:      "customers",
		})
		if countSearchItems(items, "customer") >= 6 {
			break
		}
	}
	for _, order := range data.Orders {
		user := users[firstNonEmptyString(order.BuyerUserID, order.UserID)]
		plan := plans[order.PlanID]
		if !containsSearchKeyword(keyword, order.ID, order.OrderNo, order.Status, user.ID, user.Name, user.Email, planName(plan)) {
			continue
		}
		items = append(items, adminGlobalSearchItem{
			Type: "order", RecordID: order.ID, Title: firstNonEmptyString(order.OrderNo, order.ID),
			Description: strings.TrimSpace(strings.Join([]string{firstNonEmptyString(user.Name, user.Email, user.ID), planName(plan), order.Status}, " · ")),
			Module:      "orders",
		})
		if countSearchItems(items, "order") >= 6 {
			break
		}
	}
	if enterpriseStore, ok := a.store.(adminEnterpriseStore); ok {
		if result, listErr := enterpriseStore.ListAdminEnterprises(adminEnterpriseListQuery{Page: 1, PageSize: 6, Keyword: keyword}); listErr == nil {
			for _, enterprise := range result.Items {
				items = append(items, adminGlobalSearchItem{Type: "enterprise", RecordID: enterprise.ID, Title: firstNonEmptyString(enterprise.Name, enterprise.EnterpriseCode, enterprise.ID), Description: joinSearchDescription(enterprise.EnterpriseCode, enterprise.CertificationStatus, enterprise.Status), Module: "enterpriseList"})
			}
		}
	}
	for _, task := range data.GenerationTasks {
		user := users[task.UserID]
		if !containsSearchKeyword(keyword, task.ID, task.ClientRequestID, task.UserID, user.Name, user.Email, task.Type, task.Model, task.Status, task.TaskStatus, task.BillingStatus, task.UpstreamRequestID) {
			continue
		}
		items = append(items, adminGlobalSearchItem{Type: "generation_task", RecordID: task.ID, Title: firstNonEmptyString(task.ID, task.ClientRequestID), Description: joinSearchDescription(firstNonEmptyString(user.Name, user.Email, task.UserID), task.Type, task.Model, firstNonEmptyString(task.TaskStatus, task.Status)), Module: "aiCapabilityLogs"})
		if countSearchItems(items, "generation_task") >= 6 {
			break
		}
	}
	paymentRows, invoiceRows := searchBillingRows(r, a, data, keyword)
	for _, payment := range paymentRows {
		id := stringValue(payment["id"])
		if !containsSearchKeyword(keyword, id, stringValue(payment["paymentNo"]), stringValue(payment["orderId"]), stringValue(payment["orderNo"]), stringValue(payment["paymentRequestId"]), stringValue(payment["channel"]), stringValue(payment["paymentChannel"]), stringValue(payment["provider"]), stringValue(payment["wechatTransactionId"]), stringValue(payment["customerName"]), stringValue(payment["status"])) {
			continue
		}
		items = append(items, adminGlobalSearchItem{Type: "payment", RecordID: id, Title: firstNonEmptyString(stringValue(payment["paymentNo"]), id), Description: joinSearchDescription(firstNonEmptyString(stringValue(payment["orderNo"]), stringValue(payment["orderId"])), firstNonEmptyString(stringValue(payment["paymentChannel"]), stringValue(payment["channel"])), stringValue(payment["status"])), Module: "billingPayments"})
		if countSearchItems(items, "payment") >= 6 {
			break
		}
	}
	for _, invoice := range invoiceRows {
		id := stringValue(invoice["id"])
		invoiceNo := stringValue(invoice["invoiceNo"])
		if !containsSearchKeyword(keyword, id, invoiceNo, stringValue(invoice["customer"]), stringValue(invoice["customerName"]), stringValue(invoice["customerId"]), stringValue(invoice["userId"]), stringValue(invoice["orderNo"]), stringValue(invoice["status"]), stringValue(invoice["paymentStatus"])) {
			continue
		}
		items = append(items, adminGlobalSearchItem{Type: "invoice", RecordID: id, Title: firstNonEmptyString(invoiceNo, id), Description: joinSearchDescription(firstNonEmptyString(stringValue(invoice["customerName"]), stringValue(invoice["customer"])), stringValue(invoice["orderNo"]), stringValue(invoice["status"])), Module: "billingInvoices"})
		if countSearchItems(items, "invoice") >= 6 {
			break
		}
	}
	writeJSON(w, map[string]any{"items": items})
}

func searchBillingRows(r *http.Request, a adminAPI, data adminPlatformData, keyword string) ([]map[string]any, []map[string]any) {
	db, err := a.commercialBillingDB()
	if err != nil {
		return billingPaymentRows(data), billingInvoiceRows(data)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	pattern := "%" + strings.TrimSpace(keyword) + "%"
	payments, paymentErr := queryCommercialBillingRows(ctx, db, `
		select p.id,p.payment_no as "paymentNo",p.order_no as "orderNo",p.user_id as "userId",coalesce(u.name,'') as "customerName",
		       p.payment_channel as "paymentChannel",p.amount_cents as "amountCents",p.prepay_status as status,
		       coalesce(p.wechat_transaction_id,'') as "wechatTransactionId",p.created_at as "createdAt"
		from xz_payment_records p left join xz_users u on u.id=p.user_id
		where p.id ilike $1 or p.payment_no ilike $1 or p.order_no ilike $1 or coalesce(p.wechat_transaction_id,'') ilike $1 or coalesce(u.name,'') ilike $1
		order by p.created_at desc limit 6`, pattern)
	invoices, invoiceErr := queryCommercialBillingRows(ctx, db, `
		select i.id,i.invoice_no as "invoiceNo",i.order_no as "orderNo",i.user_id as "userId",coalesce(u.name,'') as "customerName",
		       i.status,i.payment_status as "paymentStatus",i.total_cents as "totalCents",i.created_at as "createdAt"
		from xz_billing_invoices i left join xz_users u on u.id=i.user_id
		where i.id ilike $1 or i.invoice_no ilike $1 or i.order_no ilike $1 or i.user_id ilike $1 or coalesce(u.name,'') ilike $1
		order by i.created_at desc limit 6`, pattern)
	if paymentErr != nil {
		payments = billingPaymentRows(data)
	}
	if invoiceErr != nil {
		invoices = billingInvoiceRows(data)
	}
	return payments, invoices
}

func countSearchItems(items []adminGlobalSearchItem, itemType string) int {
	count := 0
	for _, item := range items {
		if item.Type == itemType {
			count++
		}
	}
	return count
}

func joinSearchDescription(values ...string) string {
	items := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			items = append(items, value)
		}
	}
	return strings.Join(items, " · ")
}

func containsSearchKeyword(keyword string, values ...string) bool {
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	if keyword == "" {
		return false
	}
	for _, value := range values {
		if strings.Contains(strings.ToLower(strings.TrimSpace(value)), keyword) {
			return true
		}
	}
	return false
}

func buildAdminOverviewWorkItems(data adminPlatformData) ([]adminWorkspaceItem, []adminWorkspaceItem) {
	pendingOrders := 0
	failedFulfillment := 0
	for _, order := range data.Orders {
		status := strings.ToUpper(strings.TrimSpace(order.Status))
		fulfillment := strings.ToUpper(strings.TrimSpace(order.FulfillmentStatus))
		if status == "PENDING" || status == "CREATED" {
			pendingOrders++
		}
		if fulfillment == "FAILED" || fulfillment == "ERROR" {
			failedFulfillment++
		}
	}
	pendingMerges := 0
	for _, item := range data.AuthMergeRequests {
		if statusIn(item.Status, "PENDING", "IN_REVIEW") {
			pendingMerges++
		}
	}
	pendingCertifications := 0
	for _, item := range data.Enterprise.Certifications {
		if statusIn(item.Status, "PENDING", "IN_REVIEW") {
			pendingCertifications++
		}
	}
	pendingWithdrawals := 0
	for _, item := range data.Withdrawals {
		if statusIn(item.Status, "PENDING", "IN_REVIEW") {
			pendingWithdrawals++
		}
	}
	failedTasks := 0
	billingExceptions := 0
	for _, task := range data.GenerationTasks {
		if statusIn(task.Status, "FAILED", "ERROR") || statusIn(task.TaskStatus, "FAILED", "ERROR") {
			failedTasks++
		}
		if statusIn(task.BillingStatus, "FAILED", "ERROR", "RECONCILIATION_REQUIRED") {
			billingExceptions++
		}
	}

	tasks := []adminWorkspaceItem{
		{ID: "pending-orders", Title: "待处理订单", Description: "检查待支付或待确认订单", Count: pendingOrders, Severity: severityForCount(pendingOrders, "warning"), Module: "orders", Roles: []string{"SUPER_ADMIN", "FINANCE", "OPERATOR"}},
		{ID: "enterprise-certifications", Title: "企业认证审核", Description: "处理企业认证资料与审核结论", Count: pendingCertifications, Severity: severityForCount(pendingCertifications, "warning"), Module: "enterpriseCertifications", Roles: []string{"SUPER_ADMIN", "ENTERPRISE_ADMIN", "OPERATOR"}},
		{ID: "account-merges", Title: "账号合并工单", Description: "解决手机号或微信身份冲突", Count: pendingMerges, Severity: severityForCount(pendingMerges, "warning"), Module: "customers", Roles: []string{"SUPER_ADMIN", "CUSTOMER_SERVICE", "OPERATOR"}},
		{ID: "settlement-reviews", Title: "结算审核", Description: "复核渠道提现与待结算记录", Count: pendingWithdrawals, Severity: severityForCount(pendingWithdrawals, "warning"), Module: "marketingSettlementStatements", Roles: []string{"SUPER_ADMIN", "FINANCE"}},
	}
	exceptions := []adminWorkspaceItem{
		{ID: "fulfillment-failures", Title: "权益发放异常", Description: "订单已进入履约但权益未成功到账", Count: failedFulfillment, Severity: severityForCount(failedFulfillment, "danger"), Module: "orders", Roles: []string{"SUPER_ADMIN", "FINANCE", "OPERATOR"}},
		{ID: "billing-exceptions", Title: "计费对账异常", Description: "计费状态需要人工核对", Count: billingExceptions, Severity: severityForCount(billingExceptions, "danger"), Module: "billingReconciliation", Roles: []string{"SUPER_ADMIN", "FINANCE", "AI_OPERATOR"}},
		{ID: "generation-failures", Title: "生成任务失败", Description: "模型调用或内容生成任务失败", Count: failedTasks, Severity: severityForCount(failedTasks, "danger"), Module: "aiCapabilityLogs", Roles: []string{"SUPER_ADMIN", "AI_OPERATOR"}},
	}
	return tasks, exceptions
}

func statusIn(value string, candidates ...string) bool {
	value = strings.ToUpper(strings.TrimSpace(value))
	for _, candidate := range candidates {
		if value == candidate {
			return true
		}
	}
	return false
}

func severityForCount(count int, active string) string {
	if count > 0 {
		return active
	}
	return "success"
}

func (a adminAPI) customer360(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.PathValue("id"))
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	users := userMap(data.Users)
	user, ok := users[userID]
	if !ok {
		writeError(w, http.StatusNotFound, errors.New("user not found"))
		return
	}
	plans := planMap(data.Plans)
	points := pointMap(data.PointAccounts)
	plan := plans[user.PlanID]
	account := points[userID]
	orders := make([]map[string]any, 0)
	orderIDs := map[string]bool{}
	paidOrders := 0
	paidAmount := 0
	for _, order := range data.Orders {
		if order.UserID != userID && order.BuyerUserID != userID {
			continue
		}
		orders = append(orders, adminOrderView(order, users, plans))
		orderIDs[order.ID] = true
		if isPaidStatus(order.Status) {
			paidOrders++
			paidAmount += orderAmount(order)
		}
	}
	tokenRecords := make([]adminTokenRecord, 0)
	for _, item := range data.TokenRecords {
		if item.UserID == userID {
			tokenRecords = append(tokenRecords, item)
		}
	}
	payments := make([]adminPayment, 0)
	for _, item := range data.Payments {
		if orderIDs[item.OrderID] {
			payments = append(payments, item)
		}
	}
	commissions := make([]adminCommission, 0)
	for _, item := range data.Commissions {
		if orderIDs[item.OrderID] {
			item.RuleSnapshot = nil
			commissions = append(commissions, item)
		}
	}
	billingEvents := make([]adminBillingEvent, 0)
	for _, item := range data.BillingEvents {
		if item.UserID == userID {
			item.Metadata = nil
			billingEvents = append(billingEvents, item)
		}
	}
	generationTasks := make([]map[string]any, 0)
	for _, item := range data.GenerationTasks {
		if item.UserID != userID {
			continue
		}
		generationTasks = append(generationTasks, map[string]any{
			"id": item.ID, "type": item.Type, "model": item.Model, "status": item.Status,
			"billingStatus": item.BillingStatus, "pointCost": item.PointCost, "createdAt": item.CreatedAt,
		})
	}
	mergeRequests := make([]map[string]any, 0)
	for _, item := range data.AuthMergeRequests {
		if item.PrimaryUserID == userID || item.SecondaryUserID == userID {
			mergeRequests = append(mergeRequests, adminAuthMergeRequestPayload(item))
		}
	}
	attribution := map[string]any{}
	for _, item := range buildAdminCustomerAttributionRows(data, nil) {
		if item.CustomerType == "PERSONAL" && item.CustomerID == userID {
			attribution = map[string]any{"item": item}
			break
		}
	}
	writeJSON(w, map[string]any{
		"profile": map[string]any{
			"id": user.ID, "name": user.Name, "email": user.Email, "role": user.Role, "status": user.Status,
			"planId": user.PlanID, "plan": planName(plan), "subscriptionExpiresAt": user.SubscriptionExpiresAt,
			"memberLevel": user.MemberLevel, "createdAt": user.CreatedAt, "updatedAt": user.UpdatedAt,
			"modelRoute": modelRouteSummary(primaryUserModelRoute(user)),
		},
		"identity": adminCustomerIdentityPayload(user),
		"wallet": map[string]any{
			"available": account.Available, "frozen": account.Frozen, "totalGranted": account.TotalGranted, "totalUsed": account.TotalUsed,
		},
		"summary": map[string]any{
			"orders": len(orders), "paidOrders": paidOrders, "paidAmountCents": paidAmount,
			"generationTasks": len(generationTasks), "tokenRecords": len(tokenRecords),
		},
		"attribution": attribution,
		"orders":      orders, "payments": payments, "tokenRecords": tokenRecords,
		"commissions": commissions, "billingEvents": billingEvents, "generationTasks": generationTasks,
		"mergeRequests": mergeRequests,
	})
}

func (a adminAPI) orderTimeline(w http.ResponseWriter, r *http.Request) {
	orderID := strings.TrimSpace(r.PathValue("id"))
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	var order adminOrder
	found := false
	for _, item := range data.Orders {
		if item.ID == orderID || item.OrderNo == orderID {
			order = item
			found = true
			break
		}
	}
	if !found {
		writeError(w, http.StatusNotFound, errors.New("order not found"))
		return
	}
	payments := make([]adminPayment, 0)
	for _, item := range data.Payments {
		if item.OrderID == order.ID {
			payments = append(payments, item)
		}
	}
	events := make([]map[string]any, 0)
	for _, item := range data.PaymentEvents {
		if item.OrderID == order.ID {
			events = append(events, map[string]any{
				"id": item.ID, "provider": item.Provider, "eventId": item.EventID, "transactionId": item.TransactionID,
				"amountCents": item.AmountCents, "verified": item.Verified, "status": item.Status,
				"processedAt": item.ProcessedAt, "createdAt": item.CreatedAt,
			})
		}
	}
	tokenRecords := make([]adminTokenRecord, 0)
	for _, item := range data.TokenRecords {
		if item.OrderID == order.ID {
			tokenRecords = append(tokenRecords, item)
		}
	}
	commissions := make([]adminCommission, 0)
	for _, item := range data.Commissions {
		if item.OrderID == order.ID {
			item.RuleSnapshot = nil
			commissions = append(commissions, item)
		}
	}
	timeline := buildOrderFulfillmentTimeline(order, payments, tokenRecords, commissions)
	writeJSON(w, map[string]any{
		"item":     adminOrderView(order, userMap(data.Users), planMap(data.Plans)),
		"timeline": timeline, "payments": payments, "paymentEvents": events,
		"tokenRecords": tokenRecords, "commissions": commissions,
	})
}

func buildOrderFulfillmentTimeline(order adminOrder, payments []adminPayment, tokenRecords []adminTokenRecord, commissions []adminCommission) []map[string]any {
	paid := isPaidStatus(order.Status) || strings.TrimSpace(order.PaidAt) != ""
	fulfillment := strings.ToUpper(strings.TrimSpace(order.FulfillmentStatus))
	fulfilled := fulfillment == "FULFILLED" || fulfillment == "COMPLETED" || strings.TrimSpace(order.FulfilledAt) != ""
	fulfillmentFailed := fulfillment == "FAILED" || fulfillment == "ERROR"
	paymentState := "pending"
	if paid {
		paymentState = "complete"
	} else if statusIn(order.Status, "FAILED", "CANCELLED", "CLOSED") {
		paymentState = "error"
	}
	fulfillmentState := "pending"
	if fulfilled {
		fulfillmentState = "complete"
	} else if fulfillmentFailed {
		fulfillmentState = "error"
	} else if paid {
		fulfillmentState = "current"
	}
	commissionState := "pending"
	if len(commissions) > 0 {
		commissionState = "complete"
		for _, item := range commissions {
			if statusIn(item.Status, "FAILED", "REJECTED") || statusIn(item.SettleStatus, "FAILED", "REJECTED") {
				commissionState = "error"
				break
			}
			if statusIn(item.Status, "PENDING", "IN_REVIEW") || statusIn(item.SettleStatus, "PENDING", "IN_REVIEW") {
				commissionState = "current"
			}
		}
	} else if fulfilled {
		commissionState = "complete"
	}
	return []map[string]any{
		{"id": "created", "title": "订单创建", "description": "订单与价格快照已生成", "state": "complete", "occurredAt": order.CreatedAt},
		{"id": "payment", "title": "支付确认", "description": fulfillmentCountText(len(payments), "支付记录"), "state": paymentState, "occurredAt": order.PaidAt},
		{"id": "entitlement", "title": "权益发放", "description": fulfillmentCountText(len(tokenRecords), "权益流水"), "state": fulfillmentState, "occurredAt": order.FulfilledAt},
		{"id": "commission", "title": "分佣结算", "description": fulfillmentCountText(len(commissions), "分佣记录"), "state": commissionState},
		{"id": "closed", "title": "履约完成", "description": "支付、权益和分佣链路完成后归档", "state": map[bool]string{true: "complete", false: "pending"}[fulfilled], "occurredAt": order.FulfilledAt},
	}
}

func fulfillmentCountText(count int, label string) string {
	if count == 0 {
		return "暂无" + label
	}
	return strconv.Itoa(count) + " 笔" + label
}
