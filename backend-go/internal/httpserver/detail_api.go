package httpserver

import (
	"errors"
	"net/http"
	"strings"
)

var errDetailNotFound = errors.New("detail not found")

func (a api) memberOrders(w http.ResponseWriter, r *http.Request) {
	user, err := a.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	data, err := a.userAccountData(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"items": userMembershipOrders(data, user.ID)})
}

func (a api) memberOrderDetail(w http.ResponseWriter, r *http.Request) {
	user, err := a.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	data, err := a.userAccountData(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	for _, item := range userMembershipOrders(data, user.ID) {
		if stringValue(item["id"]) == id || stringValue(item["orderNo"]) == id {
			writeJSON(w, map[string]any{"item": item})
			return
		}
	}
	writeError(w, http.StatusNotFound, errDetailNotFound)
}

func (a api) assetDetail(w http.ResponseWriter, r *http.Request) {
	user, err := a.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	item, found, err := a.assetForUser(r, user.ID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if found {
		writeJSON(w, map[string]any{"item": secureAssetForClient(item)})
		return
	}
	writeError(w, http.StatusNotFound, errAssetNotFound)
}

func (a api) userUsageDetail(w http.ResponseWriter, r *http.Request) {
	user, err := a.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	data, err := a.userAccountData(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	for _, event := range data.BillingEvents {
		if event.UserID != user.ID || event.ID != id || event.PointCost <= 0 || !isUsageBillingMetric(event.MetricCode) {
			continue
		}
		items := userPointTransactions([]adminBillingEvent{event}, user.ID)
		if len(items) == 1 {
			writeJSON(w, map[string]any{"item": items[0]})
			return
		}
	}
	writeError(w, http.StatusNotFound, errDetailNotFound)
}

func (a channelAPI) orderDetail(w http.ResponseWriter, r *http.Request) {
	data, user, agent, err := a.authenticatedAgentData(r, false, 0)
	if err != nil {
		writeChannelAuthError(w, err)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	visible := channelVisibleCustomerIDs(data.Users, data.ChannelAgents, user.ID, agent.ID)
	for _, item := range channelOrdersForUsers(data.Orders, data.Users, data.Plans, visible) {
		if stringValue(item["id"]) == id {
			writeJSON(w, map[string]any{"item": item})
			return
		}
	}
	writeError(w, http.StatusNotFound, errDetailNotFound)
}

func (a channelAPI) commissionDetail(w http.ResponseWriter, r *http.Request) {
	data, _, agent, err := a.authenticatedAgentData(r, false, 0)
	if err != nil {
		writeChannelAuthError(w, err)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	for _, item := range channelCommissions(data.Commissions, agent.ID) {
		if item.ID == id {
			writeJSON(w, map[string]any{"item": item})
			return
		}
	}
	writeError(w, http.StatusNotFound, errDetailNotFound)
}

func (a channelAPI) withdrawalDetail(w http.ResponseWriter, r *http.Request) {
	data, _, agent, err := a.authenticatedAgentData(r, false, 0)
	if err != nil {
		writeChannelAuthError(w, err)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	for _, item := range channelWithdrawals(data.Withdrawals, agent.ID) {
		if item.ID == id {
			writeJSON(w, map[string]any{"item": item})
			return
		}
	}
	writeError(w, http.StatusNotFound, errDetailNotFound)
}

func (a channelAPI) childDetail(w http.ResponseWriter, r *http.Request) {
	data, _, agent, err := a.authenticatedAgentData(r, false, 0)
	if err != nil {
		writeChannelAuthError(w, err)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	for _, item := range channelChildren(data.ChannelAgents, data.Users, agent.ID) {
		if stringValue(item["id"]) == id {
			writeJSON(w, map[string]any{"item": item})
			return
		}
	}
	writeError(w, http.StatusNotFound, errDetailNotFound)
}

func (a channelAPI) inviteRecords(w http.ResponseWriter, r *http.Request) {
	data, user, _, err := a.authenticatedAgentData(r, false, 0)
	if err != nil {
		writeChannelAuthError(w, err)
		return
	}
	items := []map[string]any{}
	registered := 0
	paid := 0
	upgraded := 0
	for _, item := range marketingInviteRecordRows(data) {
		if stringValue(item["inviterUserId"]) != user.ID {
			continue
		}
		items = append(items, item)
		if strings.EqualFold(stringValue(item["registerStatus"]), "REGISTERED") {
			registered++
		}
		if strings.EqualFold(stringValue(item["rechargeStatus"]), "PAID") {
			paid++
		}
		if strings.EqualFold(stringValue(item["upgradeStatus"]), "UPGRADED") {
			upgraded++
		}
	}
	writeJSON(w, map[string]any{
		"summary": map[string]any{"records": len(items), "registered": registered, "paid": paid, "upgraded": upgraded},
		"items":   items,
	})
}

func (a api) operationCenterAgentDetail(w http.ResponseWriter, r *http.Request) {
	data, _, center, ok := a.authenticatedOperationCenter(r)
	if !ok {
		writeError(w, http.StatusForbidden, errForbidden)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	users := userMap(data.Users)
	for _, agent := range data.ChannelAgents {
		if agent.OperationCenterID == center.ID && agent.ID == id {
			writeJSON(w, map[string]any{"item": channelAgentView(agent, users[agent.UserID])})
			return
		}
	}
	writeError(w, http.StatusNotFound, errDetailNotFound)
}

func (a api) operationCenterOrderDetail(w http.ResponseWriter, r *http.Request) {
	data, _, center, ok := a.authenticatedOperationCenter(r)
	if !ok {
		writeError(w, http.StatusForbidden, errForbidden)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	users := userMap(data.Users)
	plans := planMap(data.Plans)
	for _, order := range data.Orders {
		if orderOperationCenterID(order) == center.ID && order.ID == id {
			writeJSON(w, map[string]any{"item": adminOrderView(order, users, plans)})
			return
		}
	}
	writeError(w, http.StatusNotFound, errDetailNotFound)
}

func (a api) operationCenterCommissionDetail(w http.ResponseWriter, r *http.Request) {
	data, _, center, ok := a.authenticatedOperationCenter(r)
	if !ok {
		writeError(w, http.StatusForbidden, errForbidden)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	for _, item := range data.Commissions {
		if item.ReceiverType == receiverTypeOperationCenter && item.ReceiverID == center.ID && item.ID == id {
			writeJSON(w, map[string]any{"item": item})
			return
		}
	}
	writeError(w, http.StatusNotFound, errDetailNotFound)
}
