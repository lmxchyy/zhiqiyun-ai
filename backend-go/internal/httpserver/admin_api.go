package httpserver

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
)

type adminAPI struct {
	store platformStore
}

func newAdminAPI(store platformStore) adminAPI {
	return adminAPI{store: store}
}

func (a adminAPI) overview(w http.ResponseWriter, _ *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	revenue := 0
	paidOrders := 0
	for _, order := range data.Orders {
		if isPaidStatus(order.Status) {
			revenue += orderAmount(order)
			paidOrders++
		}
	}
	modelCost := len(data.GenerationTasks)*12 + sumAgentCost(data.AgentCalls)
	commissionCost := 0
	for _, item := range data.Commissions {
		commissionCost += item.AmountCents
	}
	writeJSON(w, map[string]any{
		"metrics": []map[string]any{
			{"key": "revenue", "label": "收入", "value": revenue, "unit": "cents"},
			{"key": "cost", "label": "成本", "value": modelCost + commissionCost, "unit": "cents"},
			{"key": "profit", "label": "利润", "value": revenue - modelCost - commissionCost, "unit": "cents"},
			{"key": "customers", "label": "客户", "value": len(data.Users)},
			{"key": "orders", "label": "订单", "value": len(data.Orders)},
			{"key": "paidOrders", "label": "已收款订单", "value": paidOrders},
			{"key": "agents", "label": "渠道", "value": len(data.ChannelAgents)},
			{"key": "usage", "label": "总用量", "value": len(data.GenerationTasks) + len(data.AgentCalls) + len(data.GeoTasks)},
		},
		"usage": map[string]any{
			"apiCalls":        len(data.GenerationTasks),
			"agentChats":      len(data.AgentCalls),
			"geoTasks":        len(data.GeoTasks),
			"generatedAssets": len(data.Assets),
		},
		"profit": map[string]any{
			"revenueCents":         revenue,
			"modelCostCents":       modelCost,
			"commissionCents":      commissionCost,
			"estimatedProfitCents": revenue - modelCost - commissionCost,
		},
	})
}

func (a adminAPI) customers(w http.ResponseWriter, _ *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	plans := planMap(data.Plans)
	points := pointMap(data.PointAccounts)
	users := userMap(data.Users)
	agentsByUser := agentByUserMap(data.ChannelAgents)
	agentsByID := agentByIDMap(data.ChannelAgents)
	items := make([]map[string]any, 0, len(data.Users))
	for _, user := range data.Users {
		plan := plans[user.PlanID]
		ownChannel := agentsByUser[user.ID]
		sourceAgent := agentsByUser[user.ReferredBy]
		sourceUser := users[sourceAgent.UserID]
		parentAgent := agentsByID[sourceAgent.ParentID]
		parentUser := users[parentAgent.UserID]
		items = append(items, map[string]any{
			"id": user.ID, "name": user.Name, "email": user.Email, "role": user.Role,
			"status": user.Status, "plan": planName(plan), "planId": user.PlanID,
			"pointsAvailable": points[user.ID].Available, "subscriptionExpiresAt": user.SubscriptionExpiresAt,
			"referredBy": user.ReferredBy, "ownChannelAgentId": ownChannel.ID,
			"sourceAgentId": sourceAgent.ID, "sourceAgentName": sourceUser.Name,
			"sourceInviteCode": sourceAgent.InviteCode, "sourceChannelLevel": sourceAgent.Level,
			"sourceParentAgentId": parentAgent.ID, "sourceParentAgentName": parentUser.Name,
			"createdAt": user.CreatedAt,
		})
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a adminAPI) createCustomer(w http.ResponseWriter, r *http.Request) {
	var req adminCustomerMutation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.TrimSpace(req.Email)
	if req.Name == "" || req.Email == "" {
		writeError(w, http.StatusBadRequest, errors.New("name and email are required"))
		return
	}
	user, err := a.store.CreateAdminCustomer(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"item": user})
}

func (a adminAPI) updateCustomer(w http.ResponseWriter, r *http.Request) {
	var req adminCustomerMutation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	user, err := a.store.UpdateAdminCustomer(r.PathValue("id"), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"item": user})
}

func (a adminAPI) channelAgents(w http.ResponseWriter, _ *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	users := userMap(data.Users)
	items := make([]map[string]any, 0, len(data.ChannelAgents))
	for _, agent := range data.ChannelAgents {
		user := users[agent.UserID]
		items = append(items, channelAgentView(agent, user))
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a adminAPI) channelAgentTree(w http.ResponseWriter, _ *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	users := userMap(data.Users)
	children := map[string][]map[string]any{}
	roots := []map[string]any{}
	for _, agent := range data.ChannelAgents {
		view := channelAgentView(agent, users[agent.UserID])
		if agent.ParentID == "" {
			roots = append(roots, view)
			continue
		}
		children[agent.ParentID] = append(children[agent.ParentID], view)
	}
	for _, root := range roots {
		if id, ok := root["id"].(string); ok {
			root["children"] = children[id]
		}
	}
	writeJSON(w, map[string]any{"items": roots})
}

func (a adminAPI) createChannelAgent(w http.ResponseWriter, r *http.Request) {
	var req adminChannelCreateMutation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.TrimSpace(req.Email)
	req.ParentID = strings.TrimSpace(req.ParentID)
	req.Status = strings.ToUpper(strings.TrimSpace(req.Status))
	req.InviteCode = strings.ToUpper(strings.TrimSpace(req.InviteCode))
	if req.Name == "" || req.Email == "" {
		writeError(w, http.StatusBadRequest, errors.New("name and email are required"))
		return
	}
	if req.Level == 0 {
		req.Level = 1
	}
	if req.Level < 1 || req.Level > 2 {
		writeError(w, http.StatusBadRequest, errors.New("level must be 1 or 2"))
		return
	}
	if req.Level == 2 && req.ParentID == "" {
		writeError(w, http.StatusBadRequest, errors.New("parentId is required for level 2 agents"))
		return
	}
	agent, user, err := a.store.CreateAdminChannelAgent(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{"item": channelAgentView(agent, user), "user": userView(user)})
}

func (a adminAPI) updateChannelAgent(w http.ResponseWriter, r *http.Request) {
	var req adminChannelMutation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	agent, err := a.store.UpdateAdminChannelAgent(r.PathValue("id"), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"item": agent})
}

func (a adminAPI) products(w http.ResponseWriter, _ *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"items": productsWithUsage(data)})
}

func (a adminAPI) updateProduct(w http.ResponseWriter, r *http.Request) {
	var req adminProductMutation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	product, err := a.store.UpdateAdminProduct(r.PathValue("id"), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"item": product})
}

func (a adminAPI) plans(w http.ResponseWriter, _ *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	items := make([]map[string]any, 0, len(data.Plans))
	for _, plan := range data.Plans {
		items = append(items, map[string]any{
			"id": plan.ID, "code": fallback(plan.Code, plan.ID), "name": plan.Name,
			"priceCents": planPrice(plan), "grantPoints": planPoints(plan),
			"durationDays": plan.DurationDays, "concurrency": plan.Concurrency,
			"active":       plan.Active || plan.ID != "",
			"entitlements": planEntitlements(plan),
		})
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a adminAPI) updatePlan(w http.ResponseWriter, r *http.Request) {
	var req adminPlanMutation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	plan, err := a.store.UpdateAdminPlan(r.PathValue("id"), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"item": plan})
}

func (a adminAPI) orders(w http.ResponseWriter, _ *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	users := userMap(data.Users)
	plans := planMap(data.Plans)
	items := make([]map[string]any, 0, len(data.Orders))
	for _, order := range data.Orders {
		items = append(items, map[string]any{
			"id": order.ID, "customer": users[order.UserID].Name, "userId": order.UserID,
			"plan": planName(plans[order.PlanID]), "planId": order.PlanID,
			"amountCents": orderAmount(order), "status": order.Status,
			"paidAt": order.PaidAt, "createdAt": order.CreatedAt,
		})
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a adminAPI) createOrder(w http.ResponseWriter, r *http.Request) {
	var req adminOrderMutation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.UserID == "" || req.PlanID == "" || req.AmountCents < 0 {
		writeError(w, http.StatusBadRequest, errors.New("userId, planId and amountCents are required"))
		return
	}
	order, err := a.store.CreateAdminOrder(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"item": order})
}

func (a adminAPI) markOrderPaid(w http.ResponseWriter, r *http.Request) {
	order, err := a.store.MarkAdminOrderPaid(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"item": order})
}

func (a adminAPI) renewOrder(w http.ResponseWriter, r *http.Request) {
	order, err := a.store.RenewAdminOrder(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"item": order})
}

func (a adminAPI) deliveryProjects(w http.ResponseWriter, _ *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	users := userMap(data.Users)
	items := []map[string]any{}
	for _, item := range data.Presentations {
		items = append(items, map[string]any{"id": item.ID, "name": item.Topic, "type": "AI_PPT", "customer": users[item.UserID].Name, "status": item.Status, "progress": 35, "owner": "交付团队", "updatedAt": item.UpdatedAt})
	}
	for _, item := range data.GeoBrands {
		items = append(items, map[string]any{"id": item.ID, "name": item.Name + " GEO 代运营", "type": "GEO", "customer": users[item.OwnerID].Name, "status": "IN_PROGRESS", "progress": 45, "owner": "GEO 运营", "updatedAt": item.CreatedAt})
	}
	for _, item := range data.Agents {
		items = append(items, map[string]any{"id": item.ID, "name": item.Name + " Agent 搭建", "type": "AGENT", "customer": users[item.OwnerID].Name, "status": item.Status, "progress": 70, "owner": "Agent 顾问", "updatedAt": item.UpdatedAt})
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a adminAPI) updateDeliveryProject(w http.ResponseWriter, r *http.Request) {
	var req adminDeliveryMutation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	item, err := a.store.UpdateAdminDeliveryProject(r.PathValue("id"), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (a adminAPI) generationTasks(w http.ResponseWriter, _ *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	users := userMap(data.Users)
	assetsByTask := map[string][]asset{}
	for _, item := range data.Assets {
		assetsByTask[item.TaskID] = append(assetsByTask[item.TaskID], item)
	}
	items := make([]map[string]any, 0, len(data.GenerationTasks))
	for _, task := range data.GenerationTasks {
		user := users[task.UserID]
		items = append(items, map[string]any{
			"id": task.ID, "userId": task.UserID, "user": user.Name,
			"type": task.Type, "model": task.Model, "status": task.Status,
			"progress": task.Progress, "pointCost": task.PointCost,
			"resultIds": task.ResultIDs, "assets": assetsByTask[task.ID],
			"createdAt": task.CreatedAt, "updatedAt": task.UpdatedAt,
		})
	}
	writeJSON(w, map[string]any{"items": items})
}
func (a adminAPI) usage(w http.ResponseWriter, r *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	items := usageRows(data, r.URL.Query().Get("product"))
	writeJSON(w, map[string]any{
		"summary": map[string]any{"apiCalls": len(data.GenerationTasks), "agentChats": len(data.AgentCalls), "geoTasks": len(data.GeoTasks), "assets": len(data.Assets)},
		"items":   items,
	})
}

func (a adminAPI) exportUsage(w http.ResponseWriter, r *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="xianzhi-usage.csv"`)
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	writer := csv.NewWriter(w)
	_ = writer.Write([]string{"product", "metric", "usage", "costCents"})
	for _, row := range usageRows(data, r.URL.Query().Get("product")) {
		_ = writer.Write([]string{
			stringValue(row["product"]),
			stringValue(row["metric"]),
			strconv.Itoa(intValue(row["usage"])),
			strconv.Itoa(intValue(row["costCents"])),
		})
	}
	writer.Flush()
}

func (a adminAPI) commissions(w http.ResponseWriter, _ *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	total := 0
	pendingWithdrawalCents := 0
	for _, item := range data.Commissions {
		total += item.AmountCents
	}
	for _, item := range data.Withdrawals {
		if strings.ToUpper(item.Status) == "PENDING" {
			pendingWithdrawalCents += item.AmountCents
		}
	}
	writeJSON(w, map[string]any{
		"summary": map[string]any{
			"totalCents":             total,
			"agents":                 len(data.ChannelAgents),
			"withdrawals":            len(data.Withdrawals),
			"pendingWithdrawalCents": pendingWithdrawalCents,
		},
		"items":       data.Commissions,
		"withdrawals": data.Withdrawals,
	})
}

func (a adminAPI) createCommission(w http.ResponseWriter, r *http.Request) {
	var req adminCommissionMutation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	item, err := a.store.CreateAdminCommission(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (a adminAPI) createWithdrawal(w http.ResponseWriter, r *http.Request) {
	var req adminWithdrawalMutation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	item, err := a.store.CreateAdminWithdrawal(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (a adminAPI) approveWithdrawal(w http.ResponseWriter, r *http.Request) {
	item, err := a.store.ReviewAdminWithdrawal(r.PathValue("id"), "APPROVED")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (a adminAPI) rejectWithdrawal(w http.ResponseWriter, r *http.Request) {
	item, err := a.store.ReviewAdminWithdrawal(r.PathValue("id"), "REJECTED")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (a adminAPI) systemSettings(w http.ResponseWriter, _ *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	settings := data.SystemSettings
	apiKeys := publicAPIKeys(data.APIKeys)
	apiChannels := annotateAPIChannelsWithKeys(data.APIChannels, data.APIKeys)
	writeJSON(w, map[string]any{
		"brand":          settings.Brand,
		"payments":       settings.Payments,
		"permissions":    settings.Permissions,
		"apiGateway":     settings.APIGateway,
		"apiChannels":    apiChannels,
		"apiModels":      data.APIModels,
		"apiKeys":        apiKeys,
		"customerGroups": data.CustomerGroups,
	})
}

func (a adminAPI) updateSystemSettings(w http.ResponseWriter, r *http.Request) {
	var req adminSystemMutation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	settings, err := a.store.UpdateAdminSystemSettings(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"item": settings})
}

func (a adminAPI) apiProviderChannels(w http.ResponseWriter, _ *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"items": data.APIChannels})
}

func (a adminAPI) createAPIProviderChannel(w http.ResponseWriter, r *http.Request) {
	var req adminAPIChannelMutation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	item, err := a.store.CreateAdminAPIChannel(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (a adminAPI) updateAPIProviderChannel(w http.ResponseWriter, r *http.Request) {
	var req adminAPIChannelMutation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	item, err := a.store.UpdateAdminAPIChannel(r.PathValue("id"), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (a adminAPI) testAPIProviderChannel(w http.ResponseWriter, r *http.Request) {
	var req adminAPIChannelTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	item, err := a.store.TestAdminAPIChannel(r.PathValue("id"), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (a adminAPI) fetchAPIProviderChannelModels(w http.ResponseWriter, r *http.Request) {
	var req adminAPIChannelTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.ProbeProtocol = false
	item, err := a.store.TestAdminAPIChannel(r.PathValue("id"), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if ok, _ := item["ok"].(bool); !ok {
		writeJSON(w, map[string]any{"item": item})
		return
	}
	writeJSON(w, map[string]any{
		"total":            intFromAny(item["modelCount"]),
		"all":              stringSliceFromAny(item["all"]),
		"imageModels":      stringSliceFromAny(item["imageModels"]),
		"chatModels":       stringSliceFromAny(item["chatModels"]),
		"videoModels":      stringSliceFromAny(item["videoModels"]),
		"protocol":         item["protocol"],
		"imageRequestMode": item["imageRequestMode"],
		"raw":              item["raw"],
		"item":             item,
	})
}

func (a adminAPI) apiModels(w http.ResponseWriter, _ *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"items": data.APIModels})
}

func (a adminAPI) updateAPIModel(w http.ResponseWriter, r *http.Request) {
	var req adminAPIModelMutation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	item, err := a.store.UpdateAdminAPIModel(r.PathValue("id"), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (a adminAPI) apiKeys(w http.ResponseWriter, _ *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"items": publicAPIKeys(data.APIKeys)})
}

func (a adminAPI) createAPIKey(w http.ResponseWriter, r *http.Request) {
	var req adminAPIKeyMutation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	item, err := a.store.CreateAdminAPIKey(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	item.Secret = ""
	writeJSON(w, map[string]any{"item": item})
}

func (a adminAPI) updateAPIKey(w http.ResponseWriter, r *http.Request) {
	var req adminAPIKeyMutation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	item, err := a.store.UpdateAdminAPIKey(r.PathValue("id"), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (a adminAPI) customerGroups(w http.ResponseWriter, _ *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"items": data.CustomerGroups})
}

func (a adminAPI) updateCustomerGroup(w http.ResponseWriter, r *http.Request) {
	var req adminCustomerGroupMutation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	item, err := a.store.UpdateAdminCustomerGroup(r.PathValue("id"), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (a adminAPI) billingSubscription(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"object": "billing_subscription", "has_payment_method": true, "soft_limit_usd": 100, "hard_limit_usd": 120, "system_hard_limit_usd": 120})
}

func (a adminAPI) billingUsage(w http.ResponseWriter, _ *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"object": "list", "total_usage": (len(data.GenerationTasks)*12 + sumAgentCost(data.AgentCalls)) * 100, "daily_costs": []any{}})
}

func seedAdminData() adminPlatformData {
	return withAdminDefaults(adminPlatformData{})
}

func withAdminDefaults(data adminPlatformData) adminPlatformData {
	if len(data.Users) == 0 {
		data.Users = []adminUser{
			{ID: "user_000001", Email: "admin@xianzhi.ai", Name: "平台管理员", Role: "SUPER_ADMIN", Status: "ACTIVE", PlanID: "plan_free"},
			{ID: "user_000002", Email: "demo@xianzhi.ai", Name: "演示用户", Role: "MEMBER", Status: "ACTIVE", PlanID: "plan_month", ReferredBy: "user_000003"},
			{ID: "user_000003", Email: "agent1@xianzhi.ai", Name: "华东一级代理", Role: "AGENT_L1", Status: "ACTIVE", PlanID: "plan_free"},
		}
	}
	if len(data.Plans) == 0 {
		data.Plans = []adminPlan{
			{ID: "plan_free", Code: "free", Name: "免费会员", Price: 0, Points: 100, DurationDays: 36500, Concurrency: 1, Active: true},
			{ID: "plan_month", Code: "month", Name: "月度会员", Price: 9900, Points: 3000, DurationDays: 30, Concurrency: 3, Active: true},
			{ID: "plan_year", Code: "year", Name: "年度会员", Price: 89900, Points: 50000, DurationDays: 365, Concurrency: 8, Active: true},
		}
	}
	if len(data.PointAccounts) == 0 {
		data.PointAccounts = []adminPointAccount{
			{ID: "points_000001", UserID: "user_000001", Available: 100000},
			{ID: "points_000002", UserID: "user_000002", Available: 959},
			{ID: "points_000003", UserID: "user_000003", Available: 5000},
		}
	}
	if len(data.ChannelAgents) == 0 {
		data.ChannelAgents = []adminChannelAgent{{ID: "channel_000001", UserID: "user_000003", Level: 1, Status: "ACTIVE", InviteCode: "EAST001"}}
	}
	if len(data.Commissions) == 0 {
		data.Commissions = []adminCommission{{ID: "commission_000001", OrderID: "order_000001", AgentID: "channel_000001", AmountCents: 990, Rate: 0.1, Status: "SETTLED"}}
	}
	if len(data.Withdrawals) == 0 {
		data.Withdrawals = []adminWithdrawal{{ID: "withdrawal_000001", AgentID: "channel_000001", AmountCents: 300, Status: "PENDING"}}
	}
	if len(data.AdminProducts) == 0 {
		data.AdminProducts = defaultAdminProducts(data)
	}
	if data.SystemSettings.Brand.Name == "" {
		data.SystemSettings = defaultSystemSettings()
	}
	if len(data.APIChannels) == 0 {
		data.APIChannels = defaultAPIChannels()
	}
	if len(data.APIModels) == 0 {
		data.APIModels = defaultAPIModels()
	}
	if len(data.APIKeys) == 0 {
		data.APIKeys = defaultAPIKeys(data)
	}
	if len(data.CustomerGroups) == 0 {
		data.CustomerGroups = defaultCustomerGroups()
	}
	return data
}

func defaultAdminProducts(data adminPlatformData) []adminProduct {
	return []adminProduct{
		{ID: "prod_text_to_image", Name: "文生图", Type: "TEXT_TO_IMAGE", Status: "ACTIVE", Usage: len(data.GenerationTasks), Entitlements: []string{"图片生成次数", "参考图生图", "作品资产"}},
		{ID: "prod_api", Name: "API 中转", Type: "API", Status: "ACTIVE", Usage: len(data.GenerationTasks), Entitlements: []string{"OpenAI 兼容接口", "API Key", "模型分组倍率", "用量查询"}},
		{ID: "prod_agent", Name: "Agent", Type: "AGENT", Status: "ACTIVE", Usage: len(data.AgentCalls), Entitlements: []string{"智能体数量", "对话量", "知识库"}},
		{ID: "prod_geo", Name: "GEO", Type: "GEO", Status: "ACTIVE", Usage: len(data.GeoTasks), Entitlements: []string{"品牌监控", "GEO 任务", "趋势报告"}},
		{ID: "prod_ops_login", Name: "代运营登录", Type: "OPS_LOGIN", Status: "PLANNED", Usage: len(data.Presentations), Entitlements: []string{"交付项目", "客户资料收集", "服务工单"}},
	}
}

func productsWithUsage(data adminPlatformData) []adminProduct {
	products := data.AdminProducts
	if len(products) == 0 {
		products = defaultAdminProducts(data)
	}
	for i := range products {
		switch products[i].Type {
		case "TEXT_TO_IMAGE", "API":
			products[i].Usage = len(data.GenerationTasks)
		case "AGENT":
			products[i].Usage = len(data.AgentCalls)
		case "GEO":
			products[i].Usage = len(data.GeoTasks)
		case "OPS_LOGIN":
			products[i].Usage = len(data.Presentations)
		}
	}
	return products
}

func defaultSystemSettings() adminSystemSettings {
	return adminSystemSettings{
		Brand:       adminBrandSetting{Name: "先知 AI", Domain: "localhost:3100", Logo: "先"},
		Payments:    []adminPaymentChannel{{Channel: "wechat", Status: "CONFIGURABLE"}, {Channel: "alipay", Status: "CONFIGURABLE"}, {Channel: "manual", Status: "ACTIVE"}},
		Permissions: []string{"SUPER_ADMIN", "ADMIN", "FINANCE", "CHANNEL_MANAGER", "DELIVERY_MANAGER"},
		APIGateway:  map[string]any{"openAICompatible": true, "billingMode": []string{"TOKEN", "PER_REQUEST"}, "quotaEnabled": true},
	}
}

func defaultAPIChannels() []adminAPIChannel {
	items := []adminAPIChannel{
		{
			ID: "channel_apimart", Name: "APIMart 生图聚合", BaseURL: "https://api.apimart.ai", Protocol: "apimart",
			ImageRequestMode: "openai-json", ImageGenerationEndpoint: "/v1/images/generations", ImageEditEndpoint: "/v1/images/edits",
			FetchModelsPath: "/v1/models", APIKeyEnv: "APIMART_API_KEY", Notes: "参考 Infinite-Canvas 推荐平台，适合聚合图片、视频和 LLM 模型。",
			Status: "CONFIGURABLE", Priority: 10, Models: []string{"gpt-image-2", "nano-banana-edit", "veo3.1-fast"},
		},
		{
			ID: "channel_openai", Name: "OpenAI 官方", BaseURL: "https://api.openai.com/v1", Protocol: "openai",
			ImageRequestMode: "openai", ImageGenerationEndpoint: "/v1/images/generations", ImageEditEndpoint: "/v1/images/edits",
			FetchModelsPath: "/models", APIKeyEnv: "OPENAI_API_KEY", Status: "CONFIGURABLE", Priority: 20, Models: []string{"gpt-image-2", "mock-standard"},
		},
		{
			ID: "channel_modelscope", Name: "ModelScope", BaseURL: "https://api-inference.modelscope.cn/v1", Protocol: "openai",
			ImageRequestMode: "openai", ImageGenerationEndpoint: "/v1/images/generations", ImageEditEndpoint: "/v1/images/edits",
			FetchModelsPath: "/models", APIKeyEnv: "MODELSCOPE_API_KEY", Notes: "可作为免费工作流、LoRA 和国产模型补充通道。",
			Status: "CONFIGURABLE", Priority: 30, Models: []string{"Tongyi-MAI/Z-Image-Turbo", "Qwen/Qwen-Image-2512"},
		},
		{
			ID: "channel_comfyui", Name: "本地 ComfyUI 集群", BaseURL: "http://127.0.0.1:8188", Protocol: "comfyui",
			ImageRequestMode: "workflow", FetchModelsPath: "/api/workflows", ComfyInstances: comfyInstancesFromEnv(),
			Notes:  "用于内网工作流和私有部署，主控后台只管理节点与工作流可见范围。",
			Status: "CONFIGURABLE", Priority: 40, Models: []string{"custom-workflow"},
		},
	}
	baseURL := strings.TrimSpace(os.Getenv("MODEL_PROVIDER_URL"))
	if baseURL == "" {
		baseURL = strings.TrimSpace(os.Getenv("OPENAI_BASE_URL"))
	}
	model := strings.TrimSpace(os.Getenv("MODEL_PROVIDER_IMAGE_MODEL"))
	if model == "" {
		model = "gpt-image-2"
	}
	if baseURL != "" {
		items = append([]adminAPIChannel{{
			ID:                      "channel_runtime_env",
			Name:                    "当前运行上游",
			BaseURL:                 baseURL,
			Protocol:                "openai",
			ImageRequestMode:        "openai",
			ImageGenerationEndpoint: "/v1/images/generations",
			ImageEditEndpoint:       "/v1/images/edits",
			FetchModelsPath:         "/models",
			APIKeyEnv:               "MODEL_PROVIDER_API_KEY",
			Primary:                 true,
			Status:                  "ACTIVE",
			Priority:                1,
			Models:                  []string{model},
			APIKeyConfigured:        os.Getenv("MODEL_PROVIDER_API_KEY") != "" || os.Getenv("OPENAI_API_KEY") != "",
		}}, items...)
	}
	return items
}

func comfyInstancesFromEnv() []string {
	raw := strings.TrimSpace(os.Getenv("COMFYUI_INSTANCES"))
	if raw == "" {
		return []string{"127.0.0.1:8188"}
	}
	parts := strings.Split(raw, ",")
	items := []string{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			items = append(items, part)
		}
	}
	if len(items) == 0 {
		return []string{"127.0.0.1:8188"}
	}
	return items
}

func defaultAPIModels() []adminAPIModel {
	model := strings.TrimSpace(os.Getenv("MODEL_PROVIDER_IMAGE_MODEL"))
	if model == "" {
		model = "gpt-image-2"
	}
	return []adminAPIModel{
		{ID: "model_mock_standard", Model: "mock-standard", Name: "本地演示模型", Capability: "TEXT_TO_IMAGE", BillingMode: "PER_REQUEST", FixedQuota: 1, ModelRatio: 1, CompletionRatio: 1, Status: "ACTIVE"},
		{ID: "model_gpt_image_2", Model: model, Name: "当前图像模型", Capability: "IMAGE", BillingMode: "PER_REQUEST", FixedQuota: 10, ModelRatio: 1, CompletionRatio: 1, Status: "ACTIVE"},
		{ID: "model_agent_chat", Model: "agent-chat", Name: "Agent 对话", Capability: "CHAT", BillingMode: "TOKEN", ModelRatio: 1, CompletionRatio: 2, Status: "ACTIVE"},
	}
}

func defaultAPIKeys(data adminPlatformData) []adminAPIKey {
	items := []adminAPIKey{}
	for _, user := range data.Users {
		if user.Role == "MEMBER" || strings.HasPrefix(user.Role, "AGENT") {
			items = append(items, adminAPIKey{ID: "key_" + user.ID, Customer: user.Name, Prefix: "sk-" + shortID(user.ID), Status: user.Status, Models: []string{"mock-standard", "gpt-image-2"}, QuotaLimit: 100000})
		}
	}
	return items
}

func defaultCustomerGroups() []adminCustomerGroup {
	return []adminCustomerGroup{
		{ID: "group_default", Name: "default", Ratio: 1, Models: []string{"mock-standard"}, Description: "默认客户分组"},
		{ID: "group_vip", Name: "vip", Ratio: 0.8, Models: []string{"mock-standard", "gpt-image-2", "agent-chat"}, Description: "企业客户优惠倍率"},
		{ID: "group_channel", Name: "channel_a", Ratio: 0.9, Models: []string{"mock-standard", "gpt-image-2"}, Description: "代理渠道客户"},
	}
}

func usageRows(data adminPlatformData, productFilter string) []map[string]any {
	productFilter = strings.ToLower(strings.TrimSpace(productFilter))
	rows := []map[string]any{
		{"product": "API/文生图", "metric": "生成任务", "usage": len(data.GenerationTasks), "costCents": len(data.GenerationTasks) * 12},
		{"product": "Agent", "metric": "对话量", "usage": len(data.AgentCalls), "costCents": sumAgentCost(data.AgentCalls)},
		{"product": "GEO", "metric": "GEO 任务", "usage": len(data.GeoTasks), "costCents": len(data.GeoTasks) * 20},
	}
	if productFilter == "" {
		return rows
	}
	filtered := []map[string]any{}
	for _, row := range rows {
		product := strings.ToLower(stringValue(row["product"]))
		metric := strings.ToLower(stringValue(row["metric"]))
		if strings.Contains(product, productFilter) || strings.Contains(metric, productFilter) {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func userMap(users []adminUser) map[string]adminUser {
	items := map[string]adminUser{}
	for _, item := range users {
		items[item.ID] = item
	}
	return items
}

func planMap(plans []adminPlan) map[string]adminPlan {
	items := map[string]adminPlan{}
	for _, item := range plans {
		items[item.ID] = item
	}
	return items
}

func pointMap(points []adminPointAccount) map[string]adminPointAccount {
	items := map[string]adminPointAccount{}
	for _, item := range points {
		items[item.UserID] = item
	}
	return items
}

func agentByUserMap(agents []adminChannelAgent) map[string]adminChannelAgent {
	items := map[string]adminChannelAgent{}
	for _, item := range agents {
		items[item.UserID] = item
	}
	return items
}

func agentByIDMap(agents []adminChannelAgent) map[string]adminChannelAgent {
	items := map[string]adminChannelAgent{}
	for _, item := range agents {
		items[item.ID] = item
	}
	return items
}

func channelAgentView(agent adminChannelAgent, user adminUser) map[string]any {
	return map[string]any{"id": agent.ID, "userId": agent.UserID, "name": user.Name, "email": user.Email, "level": agent.Level, "parentId": agent.ParentID, "status": agent.Status, "inviteCode": agent.InviteCode, "createdAt": agent.CreatedAt}
}

func isPaidStatus(status string) bool {
	status = strings.ToUpper(status)
	return status == "PAID" || status == "SUCCEEDED" || status == "SUCCESS"
}

func orderAmount(order adminOrder) int {
	if order.AmountCents != 0 {
		return order.AmountCents
	}
	return order.Amount
}

func planPrice(plan adminPlan) int {
	if plan.PriceCents != 0 {
		return plan.PriceCents
	}
	return plan.Price
}

func planPoints(plan adminPlan) int {
	if plan.GrantPoints != 0 {
		return plan.GrantPoints
	}
	return plan.Points
}

func planName(plan adminPlan) string {
	return fallback(plan.Name, "未绑定套餐")
}

func planEntitlements(plan adminPlan) map[string]any {
	if plan.Entitlements != nil {
		return plan.Entitlements
	}
	return map[string]any{"points": planPoints(plan), "concurrency": plan.Concurrency, "durationDays": plan.DurationDays}
}

func sumAgentCost(calls []adminAgentCall) int {
	total := 0
	for _, call := range calls {
		if call.CostCents != 0 {
			total += call.CostCents
		} else {
			total += call.Cost
		}
	}
	return total
}

func fallback(value string, fallbackValue string) string {
	if value == "" {
		return fallbackValue
	}
	return value
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[len(id)-8:]
}

func uniqueAdminID(prefix string, existing map[string]bool) string {
	for i := 1; ; i++ {
		id := prefix + "_" + fmtSix(i)
		if !existing[id] {
			return id
		}
	}
}

func fmtSix(value int) string {
	raw := "000000" + strconv.Itoa(value)
	return raw[len(raw)-6:]
}

func userIDs(items []adminUser) map[string]bool {
	ids := map[string]bool{}
	for _, item := range items {
		ids[item.ID] = true
	}
	return ids
}

func pointIDs(items []adminPointAccount) map[string]bool {
	ids := map[string]bool{}
	for _, item := range items {
		ids[item.ID] = true
	}
	return ids
}

func channelAgentIDs(items []adminChannelAgent) map[string]bool {
	ids := map[string]bool{}
	for _, item := range items {
		ids[item.ID] = true
	}
	return ids
}

func orderIDs(items []adminOrder) map[string]bool {
	ids := map[string]bool{}
	for _, item := range items {
		ids[item.ID] = true
	}
	return ids
}

func apiChannelIDs(items []adminAPIChannel) map[string]bool {
	ids := map[string]bool{}
	for _, item := range items {
		ids[item.ID] = true
	}
	return ids
}

func apiKeyIDs(items []adminAPIKey) map[string]bool {
	ids := map[string]bool{}
	for _, item := range items {
		ids[item.ID] = true
	}
	return ids
}

func commissionIDs(items []adminCommission) map[string]bool {
	ids := map[string]bool{}
	for _, item := range items {
		ids[item.ID] = true
	}
	return ids
}

func withdrawalIDs(items []adminWithdrawal) map[string]bool {
	ids := map[string]bool{}
	for _, item := range items {
		ids[item.ID] = true
	}
	return ids
}

func setPointAccount(data *adminPlatformData, userID string, available int) {
	for i := range data.PointAccounts {
		if data.PointAccounts[i].UserID == userID {
			data.PointAccounts[i].Available = available
			return
		}
	}
	data.PointAccounts = append(data.PointAccounts, adminPointAccount{
		ID:        uniqueAdminID("points", pointIDs(data.PointAccounts)),
		UserID:    userID,
		Available: available,
	})
}

func stringValue(value any) string {
	return strings.TrimSpace(toString(value))
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		num, _ := strconv.Atoi(typed)
		return num
	default:
		return 0
	}
}

func toString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		return ""
	}
}
