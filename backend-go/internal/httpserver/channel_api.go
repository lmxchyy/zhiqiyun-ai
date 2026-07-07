package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

type channelAPI struct {
	store    platformStore
	sessions authSessionStore
}

type optimizedChannelDataStore interface {
	ChannelDataForAgent(agentUserID string, agentID string, includeContent bool, billingEventLimit int) (adminPlatformData, error)
}

type optimizedChannelStatsStore interface {
	ChannelContentCountsForUsers(userIDs []string) (int, int, error)
}

func newChannelAPI(store platformStore, sessions authSessionStore) channelAPI {
	return channelAPI{store: store, sessions: sessions}
}

func (a channelAPI) me(w http.ResponseWriter, r *http.Request) {
	data, user, agent, err := a.authenticatedAgentData(r, false, 50)
	if err != nil {
		writeChannelAuthError(w, err)
		return
	}

	visibleCustomerIDs := channelVisibleCustomerIDs(data.Users, data.ChannelAgents, user.ID, agent.ID)
	customers := channelCustomers(data.Users, data.Plans, data.PointAccounts, visibleCustomerIDs)
	orders := channelOrdersForUsers(data.Orders, data.Users, data.Plans, visibleCustomerIDs)
	commissions := channelCommissions(data.Commissions, agent.ID)
	withdrawals := channelWithdrawals(data.Withdrawals, agent.ID)
	children := channelChildren(data.ChannelAgents, data.Users, agent.ID)
	usageEvents := channelBillingEvents(data.BillingEvents, visibleCustomerIDs)
	promotion := channelPromotion(agent, r, data)
	agentView := channelAgentView(agent, user)
	agentView["inviteLink"] = promotion["inviteLink"]

	writeJSON(w, map[string]any{
		"user":        userView(user),
		"agent":       agentView,
		"promotion":   promotion,
		"summary":     channelSummary(customers, commissions, withdrawals, children),
		"customers":   customers,
		"orders":      orders,
		"commissions": commissions,
		"usageEvents": usageEvents,
		"withdrawals": withdrawals,
		"children":    children,
	})
}

func (a channelAPI) customers(w http.ResponseWriter, r *http.Request) {
	data, user, agent, err := a.authenticatedAgentData(r, false, 0)
	if err != nil {
		writeChannelAuthError(w, err)
		return
	}
	visibleCustomerIDs := channelVisibleCustomerIDs(data.Users, data.ChannelAgents, user.ID, agent.ID)
	writeJSON(w, map[string]any{
		"summary": a.channelCustomerSummary(data, visibleCustomerIDs),
		"items":   channelCustomers(data.Users, data.Plans, data.PointAccounts, visibleCustomerIDs),
	})
}

func (a channelAPI) customerDetail(w http.ResponseWriter, r *http.Request) {
	data, user, agent, err := a.authenticatedAgentData(r, false, 0)
	if err != nil {
		writeChannelAuthError(w, err)
		return
	}
	customerID := strings.TrimSpace(r.PathValue("id"))
	visibleCustomerIDs := channelVisibleCustomerIDs(data.Users, data.ChannelAgents, user.ID, agent.ID)
	if !visibleCustomerIDs[customerID] {
		writeError(w, http.StatusNotFound, errors.New("customer not found"))
		return
	}
	customers := userMap(data.Users)
	customer, ok := customers[customerID]
	if !ok {
		writeError(w, http.StatusNotFound, errors.New("customer not found"))
		return
	}
	tasks, assets, err := a.channelCustomerContent(customerID, data)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{
		"item":            channelCustomerView(customer, planMap(data.Plans), pointMap(data.PointAccounts)),
		"orders":          channelOrdersForUsers(data.Orders, data.Users, data.Plans, map[string]bool{customerID: true}),
		"generationTasks": tasks,
		"assets":          assets,
	})
}

func (a channelAPI) orders(w http.ResponseWriter, r *http.Request) {
	data, user, agent, err := a.authenticatedAgentData(r, false, 0)
	if err != nil {
		writeChannelAuthError(w, err)
		return
	}
	writeJSON(w, map[string]any{
		"items": channelOrdersForUsers(data.Orders, data.Users, data.Plans, channelVisibleCustomerIDs(data.Users, data.ChannelAgents, user.ID, agent.ID)),
	})
}

func (a channelAPI) usage(w http.ResponseWriter, r *http.Request) {
	data, user, agent, err := a.authenticatedAgentData(r, false, -1)
	if err != nil {
		writeChannelAuthError(w, err)
		return
	}
	visibleCustomerIDs := channelVisibleCustomerIDs(data.Users, data.ChannelAgents, user.ID, agent.ID)
	items := channelBillingEvents(data.BillingEvents, visibleCustomerIDs)
	writeJSON(w, map[string]any{
		"summary": map[string]any{
			"events":      len(items),
			"customers":   len(visibleCustomerIDs),
			"pointCost":   channelUsagePointCost(items),
			"amountCents": channelUsageAmountCents(items),
		},
		"items": items,
	})
}

func (a channelAPI) commissions(w http.ResponseWriter, r *http.Request) {
	data, _, agent, err := a.authenticatedAgentData(r, false, 0)
	if err != nil {
		writeChannelAuthError(w, err)
		return
	}
	items := channelCommissions(data.Commissions, agent.ID)
	writeJSON(w, map[string]any{"summary": channelCommissionSummary(items), "items": items})
}

func (a channelAPI) withdrawals(w http.ResponseWriter, r *http.Request) {
	data, _, agent, err := a.authenticatedAgentData(r, false, 0)
	if err != nil {
		writeChannelAuthError(w, err)
		return
	}
	items := channelWithdrawals(data.Withdrawals, agent.ID)
	writeJSON(w, map[string]any{"summary": channelWithdrawalSummary(items), "items": items})
}

func (a channelAPI) createWithdrawal(w http.ResponseWriter, r *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	_, agent, err := a.authenticatedAgent(r, data)
	if err != nil {
		writeChannelAuthError(w, err)
		return
	}
	var req adminWithdrawalMutation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.AgentID = agent.ID
	item, err := a.store.CreateAdminWithdrawal(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (a channelAPI) createChildAgent(w http.ResponseWriter, r *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	_, agent, err := a.authenticatedAgent(r, data)
	if err != nil {
		writeChannelAuthError(w, err)
		return
	}
	var req adminChannelCreateMutation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.TrimSpace(req.Email)
	req.Status = strings.ToUpper(strings.TrimSpace(req.Status))
	if req.Name == "" || req.Email == "" {
		writeError(w, http.StatusBadRequest, errors.New("name and email are required"))
		return
	}
	if req.Level == 0 {
		req.Level = 1
	}
	if !isAgentLevel(req.Level) {
		writeError(w, http.StatusBadRequest, errors.New("level must be between L1 and L5 for channel agents"))
		return
	}
	if req.Available < 0 {
		writeError(w, http.StatusBadRequest, errors.New("available must be greater than or equal to 0"))
		return
	}
	if !canAgentCreateChildLevel(agent.Level, req.Level) {
		writeError(w, http.StatusForbidden, errors.New("当前代理等级无权开通该下级等级"))
		return
	}
	req.ParentID = agent.ID
	req.Status = fallback(req.Status, "ACTIVE")
	req.InviteCode = strings.ToUpper(strings.TrimSpace(req.InviteCode))
	createdAgent, createdUser, err := a.store.CreateAdminChannelAgent(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	item := channelAgentView(createdAgent, createdUser)
	writeJSON(w, map[string]any{"item": item, "user": userView(createdUser)})
}

func canAgentCreateChildLevel(parentLevel int, childLevel int) bool {
	if parentLevel < 3 || childLevel < 1 {
		return false
	}
	switch parentLevel {
	case 3:
		return childLevel == 1 || childLevel == 2
	case 4:
		return childLevel >= 1 && childLevel <= 3
	default:
		return childLevel >= 1 && childLevel <= 4
	}
}

func (a channelAPI) authenticatedAgent(r *http.Request, data adminPlatformData) (adminUser, adminChannelAgent, error) {
	user, err := authAPI{store: a.store, sessions: a.sessions}.authenticatedUser(r, data)
	if err != nil {
		return adminUser{}, adminChannelAgent{}, err
	}
	agent, ok := channelAgentForUser(data.ChannelAgents, user.ID)
	if !ok || !strings.HasPrefix(user.Role, "AGENT") {
		return adminUser{}, adminChannelAgent{}, errForbidden
	}
	return user, agent, nil
}

func (a channelAPI) authenticatedAgentData(r *http.Request, includeContent bool, billingEventLimit int) (adminPlatformData, adminUser, adminChannelAgent, error) {
	if optimized, ok := a.store.(optimizedChannelDataStore); ok {
		user, agent, err := a.currentAgent(r)
		if err != nil {
			return adminPlatformData{}, adminUser{}, adminChannelAgent{}, err
		}
		data, err := optimized.ChannelDataForAgent(user.ID, agent.ID, includeContent, billingEventLimit)
		if err != nil {
			return adminPlatformData{}, adminUser{}, adminChannelAgent{}, err
		}
		return data, user, agent, nil
	}
	data, err := a.store.AdminData()
	if err != nil {
		return adminPlatformData{}, adminUser{}, adminChannelAgent{}, err
	}
	user, agent, err := a.authenticatedAgent(r, data)
	return data, user, agent, err
}

func (a channelAPI) currentAgent(r *http.Request) (adminUser, adminChannelAgent, error) {
	store, ok := a.store.(activeIdentityStore)
	if !ok {
		return adminUser{}, adminChannelAgent{}, errUnauthorized
	}
	userID, err := authenticatedUserID(r, a.sessions)
	if err != nil {
		return adminUser{}, adminChannelAgent{}, err
	}
	user, found, err := store.GetActiveUser(userID)
	if err != nil {
		return adminUser{}, adminChannelAgent{}, err
	}
	if !found {
		return adminUser{}, adminChannelAgent{}, errUnauthorized
	}
	agent, found, err := store.GetChannelAgentForUser(user.ID)
	if err != nil {
		return adminUser{}, adminChannelAgent{}, err
	}
	if !found || !strings.HasPrefix(user.Role, "AGENT") {
		return adminUser{}, adminChannelAgent{}, errForbidden
	}
	return user, agent, nil
}

func (a channelAPI) channelCustomerSummary(data adminPlatformData, visibleCustomerIDs map[string]bool) map[string]any {
	summary := channelCustomerSummary(data, visibleCustomerIDs)
	if optimized, ok := a.store.(optimizedChannelStatsStore); ok {
		taskCount, assetCount, err := optimized.ChannelContentCountsForUsers(stringBoolMapKeys(visibleCustomerIDs))
		if err == nil {
			summary["generationTasks"] = taskCount
			summary["assets"] = assetCount
		}
	}
	return summary
}

func (a channelAPI) channelCustomerContent(customerID string, data adminPlatformData) ([]generationTask, []asset, error) {
	visible := map[string]bool{customerID: true}
	if optimized, ok := a.store.(optimizedUserContentStore); ok {
		tasks, err := optimized.ListGenerationTasksForUser(customerID, maxUserContentListLimit)
		if err != nil {
			return nil, nil, err
		}
		assets, err := optimized.ListAssetsForUser(customerID, maxUserContentListLimit)
		if err != nil {
			return nil, nil, err
		}
		return tasks, assets, nil
	}
	return channelGenerationTasksForUsers(data.GenerationTasks, visible), channelAssetsForUsers(data.Assets, visible), nil
}

func channelPromotion(agent adminChannelAgent, r *http.Request, data adminPlatformData) map[string]any {
	baseURL := publicBaseURL(r, data)
	inviteCode := strings.TrimSpace(agent.InviteCode)
	inviteLink := ""
	if inviteCode != "" {
		inviteLink = strings.TrimRight(baseURL, "/") + "/register?invite=" + url.QueryEscape(inviteCode)
	}
	return map[string]any{
		"inviteCode": inviteCode,
		"inviteLink": inviteLink,
		"landingURL": inviteLink,
	}
}

func publicBaseURL(r *http.Request, data adminPlatformData) string {
	domain := strings.TrimSpace(data.SystemSettings.Brand.Domain)
	if domain != "" {
		if strings.HasPrefix(domain, "http://") || strings.HasPrefix(domain, "https://") {
			return strings.TrimRight(domain, "/")
		}
		return forwardedScheme(r) + "://" + strings.TrimRight(domain, "/")
	}
	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = strings.TrimSpace(r.Host)
	}
	if host == "" {
		host = "localhost:3100"
	}
	return forwardedScheme(r) + "://" + strings.TrimRight(host, "/")
}

func forwardedScheme(r *http.Request) string {
	scheme := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0])
	if scheme == "http" || scheme == "https" {
		return scheme
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func writeChannelAuthError(w http.ResponseWriter, err error) {
	if errors.Is(err, errForbidden) {
		writeError(w, http.StatusForbidden, err)
		return
	}
	writeError(w, http.StatusUnauthorized, err)
}

func channelCustomers(users []adminUser, plans []adminPlan, points []adminPointAccount, visibleCustomerIDs map[string]bool) []map[string]any {
	plansByID := planMap(plans)
	pointsByUserID := pointMap(points)
	items := []map[string]any{}
	for _, user := range users {
		if !visibleCustomerIDs[user.ID] {
			continue
		}
		items = append(items, channelCustomerView(user, plansByID, pointsByUserID))
	}
	return items
}

func channelCustomerView(user adminUser, plans map[string]adminPlan, points map[string]adminPointAccount) map[string]any {
	view := userView(user)
	if plan, ok := plans[user.PlanID]; ok {
		view["plan"] = planName(plan)
	}
	if account, ok := points[user.ID]; ok {
		view["pointsAvailable"] = account.Available
		view["pointsFrozen"] = account.Frozen
	}
	view["referredBy"] = user.ReferredBy
	view["subscriptionExpiresAt"] = user.SubscriptionExpiresAt
	return view
}

func channelVisibleCustomerIDs(users []adminUser, agents []adminChannelAgent, agentUserID string, agentID string) map[string]bool {
	agentUserIDs := map[string]bool{agentUserID: true}
	for _, item := range agents {
		if item.ParentID == agentID {
			agentUserIDs[item.UserID] = true
		}
	}
	visibleCustomerIDs := map[string]bool{}
	for _, user := range users {
		if agentUserIDs[user.ReferredBy] {
			visibleCustomerIDs[user.ID] = true
		}
	}
	return visibleCustomerIDs
}

func channelCommissions(items []adminCommission, agentID string) []adminCommission {
	filtered := []adminCommission{}
	for _, item := range items {
		if item.AgentID == agentID {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func channelBillingEvents(events []adminBillingEvent, visibleCustomerIDs map[string]bool) []adminBillingEvent {
	items := []adminBillingEvent{}
	for _, event := range events {
		if visibleCustomerIDs[event.UserID] && isUsageBillingMetric(event.MetricCode) && event.PointCost > 0 {
			items = append(items, event)
		}
	}
	return items
}

func channelUsagePointCost(items []adminBillingEvent) int {
	total := 0
	for _, item := range items {
		total += item.PointCost
	}
	return total
}

func channelUsageAmountCents(items []adminBillingEvent) int {
	total := 0
	for _, item := range items {
		total += item.AmountCents
	}
	return total
}

func channelWithdrawals(items []adminWithdrawal, agentID string) []adminWithdrawal {
	filtered := []adminWithdrawal{}
	for _, item := range items {
		if item.AgentID == agentID {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func channelChildren(agents []adminChannelAgent, users []adminUser, parentID string) []map[string]any {
	usersByID := userMap(users)
	items := []map[string]any{}
	for _, agent := range agents {
		if agent.ParentID != parentID {
			continue
		}
		items = append(items, channelAgentView(agent, usersByID[agent.UserID]))
	}
	return items
}

func channelCustomerSummary(data adminPlatformData, visibleCustomerIDs map[string]bool) map[string]any {
	activeCustomers := 0
	totalPoints := 0
	for _, user := range data.Users {
		if !visibleCustomerIDs[user.ID] {
			continue
		}
		if strings.EqualFold(user.Status, "ACTIVE") {
			activeCustomers++
		}
	}
	for _, account := range data.PointAccounts {
		if visibleCustomerIDs[account.UserID] {
			totalPoints += account.Available
		}
	}
	return map[string]any{
		"customers":       len(visibleCustomerIDs),
		"activeCustomers": activeCustomers,
		"orders":          len(channelOrdersForUsers(data.Orders, data.Users, data.Plans, visibleCustomerIDs)),
		"generationTasks": len(channelGenerationTasksForUsers(data.GenerationTasks, visibleCustomerIDs)),
		"assets":          len(channelAssetsForUsers(data.Assets, visibleCustomerIDs)),
		"totalPoints":     totalPoints,
	}
}

func channelOrdersForUsers(orders []adminOrder, users []adminUser, plans []adminPlan, visibleCustomerIDs map[string]bool) []map[string]any {
	usersByID := userMap(users)
	plansByID := planMap(plans)
	items := []map[string]any{}
	for _, order := range orders {
		if !visibleCustomerIDs[order.UserID] {
			continue
		}
		user := usersByID[order.UserID]
		plan := plansByID[order.PlanID]
		items = append(items, map[string]any{
			"id":          order.ID,
			"userId":      order.UserID,
			"customer":    user.Name,
			"planId":      order.PlanID,
			"plan":        planName(plan),
			"amountCents": orderAmount(order),
			"status":      order.Status,
			"paidAt":      order.PaidAt,
			"createdAt":   order.CreatedAt,
		})
	}
	return items
}

func channelGenerationTasksForUsers(tasks []generationTask, visibleCustomerIDs map[string]bool) []generationTask {
	items := []generationTask{}
	for _, task := range tasks {
		if visibleCustomerIDs[task.UserID] {
			items = append(items, task)
		}
	}
	return items
}

func channelAssetsForUsers(assets []asset, visibleCustomerIDs map[string]bool) []asset {
	items := []asset{}
	for _, item := range assets {
		if visibleCustomerIDs[item.UserID] {
			items = append(items, item)
		}
	}
	return items
}

func channelCommissionSummary(items []adminCommission) map[string]any {
	total := 0
	settled := 0
	pending := 0
	for _, item := range items {
		total += item.AmountCents
		switch strings.ToUpper(item.Status) {
		case "SETTLED", "PAID", "APPROVED":
			settled += item.AmountCents
		default:
			pending += item.AmountCents
		}
	}
	return map[string]any{"totalCents": total, "settledCents": settled, "pendingCents": pending, "records": len(items)}
}

func channelWithdrawalSummary(items []adminWithdrawal) map[string]any {
	total := 0
	approved := 0
	pending := 0
	for _, item := range items {
		total += item.AmountCents
		switch strings.ToUpper(item.Status) {
		case "APPROVED", "PAID", "SETTLED":
			approved += item.AmountCents
		default:
			pending += item.AmountCents
		}
	}
	return map[string]any{"totalCents": total, "approvedCents": approved, "pendingCents": pending, "records": len(items)}
}

func channelSummary(customers []map[string]any, commissions []adminCommission, withdrawals []adminWithdrawal, children []map[string]any) map[string]any {
	totalCommission := 0
	settledCommission := 0
	pendingCommission := 0
	for _, item := range commissions {
		totalCommission += item.AmountCents
		switch strings.ToUpper(item.Status) {
		case "SETTLED", "PAID", "APPROVED":
			settledCommission += item.AmountCents
		default:
			pendingCommission += item.AmountCents
		}
	}
	withdrawn := 0
	pendingWithdrawal := 0
	for _, item := range withdrawals {
		switch strings.ToUpper(item.Status) {
		case "APPROVED", "PAID", "SETTLED":
			withdrawn += item.AmountCents
		default:
			pendingWithdrawal += item.AmountCents
		}
	}
	rawAvailableToWithdraw := settledCommission - withdrawn
	availableToWithdraw := rawAvailableToWithdraw
	if availableToWithdraw < 0 {
		availableToWithdraw = 0
	}
	return map[string]any{
		"directCustomers":        len(customers),
		"childAgents":            len(children),
		"totalCommission":        totalCommission,
		"settledCommission":      settledCommission,
		"pendingCommission":      pendingCommission,
		"withdrawn":              withdrawn,
		"pendingWithdrawal":      pendingWithdrawal,
		"availableToWithdraw":    availableToWithdraw,
		"rawAvailableToWithdraw": rawAvailableToWithdraw,
	}
}
