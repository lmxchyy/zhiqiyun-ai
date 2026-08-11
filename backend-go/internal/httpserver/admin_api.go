package httpserver

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"

	"time"
	paymentapp "xianzhi-ai/backend-go/internal/app/payment"
)

type adminAPI struct {
	store    platformStore
	sessions authSessionStore
}

func newAdminAPI(store platformStore, sessions authSessionStore) adminAPI {
	return adminAPI{store: store, sessions: sessions}
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
	tasks, exceptions := buildAdminOverviewWorkItems(data)
	exceptionPayload := any(exceptions)
	if experienceStore, ok := a.store.(adminExperienceStore); ok {
		if cases, syncErr := experienceStore.SyncAdminExceptionCases(exceptions); syncErr == nil {
			exceptionPayload = cases
		}
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
		"tasks":      tasks,
		"exceptions": exceptionPayload,
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
		modelRoute := primaryUserModelRoute(user)
		items = append(items, map[string]any{
			"id": user.ID, "name": user.Name, "email": user.Email, "mobile": user.Mobile, "role": user.Role,
			"wechatOpenIds": user.WeChatOpenIDs, "wechatUnionId": user.WeChatUnionID,
			"registrationSource": user.RegistrationSource,
			"status":             user.Status, "plan": planName(plan), "planId": user.PlanID,
			"pointsAvailable": points[user.ID].Available, "subscriptionExpiresAt": user.SubscriptionExpiresAt,
			"referredBy": user.ReferredBy, "ownChannelAgentId": ownChannel.ID,
			"sourceAgentId": sourceAgent.ID, "sourceAgentName": sourceUser.Name,
			"sourceInviteCode": sourceAgent.InviteCode, "sourceChannelLevel": sourceAgent.Level,
			"sourceParentAgentId": parentAgent.ID, "sourceParentAgentName": parentUser.Name,
			"modelRoute": modelRouteSummary(modelRoute), "modelGroup": modelRoute.GroupName,
			"modelChannel": modelRoute.Channel, "modelChannelId": modelRoute.ChannelID,
			"modelKeyStatus": modelRoute.Status, "modelQuotaLimit": modelRoute.QuotaLimit,
			"modelModels":  strings.Join(modelRoute.Models, ","),
			"modelRouteId": modelRoute.ID, "modelApiKeyId": modelRoute.APIKeyID,
			"createdAt": user.CreatedAt,
		})
	}
	writeJSON(w, map[string]any{"items": items})
}

func primaryUserModelRoute(user adminUser) adminUserModelRoute {
	for _, route := range user.ModelRoutes {
		if strings.EqualFold(route.Status, "ACTIVE") && strings.Contains(strings.Join(route.Models, ","), "gpt-image-2") {
			return route
		}
	}
	if len(user.ModelRoutes) > 0 {
		return user.ModelRoutes[0]
	}
	return adminUserModelRoute{}
}

func modelRouteSummary(route adminUserModelRoute) string {
	if route.ID == "" {
		return "未绑定"
	}
	parts := []string{}
	if route.Channel != "" {
		parts = append(parts, route.Channel)
	} else if route.Provider != "" {
		parts = append(parts, route.Provider)
	}
	if route.GroupName != "" {
		parts = append(parts, route.GroupName)
	}
	if len(route.Models) > 0 {
		parts = append(parts, strings.Join(route.Models, "、"))
	}
	if len(parts) == 0 {
		return route.ID
	}
	return strings.Join(parts, " / ")
}
func (a adminAPI) createCustomer(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var submitted map[string]json.RawMessage
	if err := json.Unmarshal(body, &submitted); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	protected := make([]string, 0, 2)
	for _, field := range []string{"role", "referredBy"} {
		if _, exists := submitted[field]; exists {
			protected = append(protected, field)
		}
	}
	if len(protected) > 0 {
		a.auditRejectedCustomerFields(r, "", protected)
		writeError(w, http.StatusBadRequest, fmt.Errorf("customer creation cannot set protected fields: %s; use account-role or relationship management", strings.Join(protected, ", ")))
		return
	}
	var req adminCustomerMutation
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.TrimSpace(req.Email)
	if req.Name == "" || req.Email == "" {
		writeError(w, http.StatusBadRequest, errors.New("name and email are required"))
		return
	}
	if req.Available != nil && *req.Available < 0 {
		writeError(w, http.StatusBadRequest, errors.New("available points cannot be negative"))
		return
	}
	requestedAvailable := req.Available
	if requestedAvailable != nil {
		req.Available = nil
	}
	user, err := a.store.CreateAdminCustomer(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if requestedAvailable != nil && *requestedAvailable > 0 {
		account, accountErr := a.store.PointAccount(user.ID)
		if accountErr != nil {
			writeError(w, http.StatusInternalServerError, accountErr)
			return
		}
		service, serviceErr := personalPointServiceForStore(a.store)
		if serviceErr != nil {
			writeError(w, http.StatusServiceUnavailable, serviceErr)
			return
		}
		actorID, actorRole := actorFromRequest(r)
		_, correctionErr := service.Correct(r.Context(), PersonalPointCorrectionCommand{
			AccountID: account.ID, UserID: user.ID, Points: int64(*requestedAvailable), Reason: "legacy customer create available compatibility", IdempotencyKey: "legacy-create:" + user.ID + ":" + strconv.Itoa(*requestedAvailable),
			Audit: PersonalPointAudit{ActorID: actorID, ActorRole: actorRole, Action: "personal_points.legacy_absolute_correction", Method: r.Method, Path: r.URL.Path, RequestID: requestIDFromPointMutation(r, "legacy-create:"+user.ID)},
		})
		if correctionErr != nil {
			writeError(w, http.StatusInternalServerError, correctionErr)
			return
		}
		user, err = a.adminCustomerByIDValue(user.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		setLegacyPointMutationHeaders(w, user.ID)
	}
	writeJSON(w, map[string]any{"item": user})
}

func (a adminAPI) updateCustomer(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var submitted map[string]json.RawMessage
	if err := json.Unmarshal(body, &submitted); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	protected := make([]string, 0, 3)
	for _, field := range []string{"role", "planId", "referredBy"} {
		if _, exists := submitted[field]; exists {
			protected = append(protected, field)
		}
	}
	if len(protected) > 0 {
		a.auditRejectedCustomerFields(r, r.PathValue("id"), protected)
		writeError(w, http.StatusBadRequest, fmt.Errorf("customer profile cannot update protected fields: %s; use identity, membership, or relationship management", strings.Join(protected, ", ")))
		return
	}
	var req adminCustomerMutation
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Available != nil && *req.Available < 0 {
		writeError(w, http.StatusBadRequest, errors.New("available points cannot be negative"))
		return
	}
	if req.Available != nil {
		current, err := a.store.PointAccount(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		delta := int64(*req.Available) - int64(current.Available)
		if delta != 0 {
			service, serviceErr := personalPointServiceForStore(a.store)
			if serviceErr != nil {
				writeError(w, http.StatusServiceUnavailable, serviceErr)
				return
			}
			actorID, actorRole := actorFromRequest(r)
			_, grantErr := service.Correct(r.Context(), PersonalPointCorrectionCommand{
				AccountID: current.ID, UserID: r.PathValue("id"), Points: delta, Reason: "legacy customer available compatibility", IdempotencyKey: "legacy-absolute:" + r.PathValue("id") + ":" + strconv.Itoa(*req.Available),
				Audit: PersonalPointAudit{ActorID: actorID, ActorRole: actorRole, Action: "personal_points.legacy_absolute_correction", Method: r.Method, Path: r.URL.Path, RequestID: requestIDFromPointMutation(r, "legacy-absolute:"+r.PathValue("id")+":"+strconv.Itoa(*req.Available))},
			})
			if grantErr != nil {
				writeError(w, http.StatusInternalServerError, grantErr)
				return
			}
		}
		req.Available = nil
		setLegacyPointMutationHeaders(w, r.PathValue("id"))
	}
	user, err := a.store.UpdateAdminCustomer(r.PathValue("id"), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"item": user})
}

func setLegacyPointMutationHeaders(w http.ResponseWriter, userID string) {
	w.Header().Set("Deprecation", "true")
	w.Header().Set("Sunset", "Wed, 31 Dec 2026 23:59:59 GMT")
	w.Header().Set("Link", `</api/v1/admin/customers/`+userID+`/point-corrections>; rel="successor-version"`)
}

func (a adminAPI) adminCustomerByIDValue(userID string) (adminUser, error) {
	data, err := a.store.AdminData()
	if err != nil {
		return adminUser{}, err
	}
	for _, item := range data.Users {
		if item.ID == userID {
			return item, nil
		}
	}
	return adminUser{}, ErrPointNotFound
}

func (a adminAPI) auditRejectedCustomerFields(r *http.Request, userID string, fields []string) {
	store, ok := a.store.(*postgresStore)
	if !ok || store.db == nil {
		return
	}
	actorID, actorRole := actorFromRequest(r)
	requestID := firstNonEmptyString(strings.TrimSpace(r.Header.Get("X-Request-ID")), strings.TrimSpace(r.Header.Get("Idempotency-Key")))
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	_ = insertAuditDirect(ctx, store.db, actorID, actorRole, "customer.profile.protected_fields.rejected", "user", userID, r.Method, r.URL.Path, http.StatusBadRequest, map[string]any{
		"fields": fields, "requestId": requestID,
	})
}

func (a adminAPI) forceLogoutCustomer(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.PathValue("id"))
	if userID == "" {
		writeError(w, http.StatusBadRequest, errors.New("user id is required"))
		return
	}
	if a.sessions == nil {
		writeError(w, http.StatusServiceUnavailable, errAuthSessionUnavailable)
		return
	}
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	found := false
	for _, user := range data.Users {
		if user.ID == userID {
			found = true
			break
		}
	}
	if !found {
		writeError(w, http.StatusNotFound, errors.New("user not found"))
		return
	}
	revoker, ok := a.sessions.(authUserSessionStore)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, errAuthSessionUnavailable)
		return
	}
	revoked, err := revoker.DeleteUserSessions(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "userId": userID, "revokedSessions": revoked})
}

func (a adminAPI) customerIdentities(w http.ResponseWriter, r *http.Request) {
	user, ok, err := a.adminCustomerByID(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, errors.New("user not found"))
		return
	}
	writeJSON(w, map[string]any{"item": adminCustomerIdentityPayload(user)})
}

func (a adminAPI) unlinkCustomerMobile(w http.ResponseWriter, r *http.Request) {
	a.updateCustomerIdentity(w, r, adminCustomerIdentityMutation{ClearMobile: true}, "mobile")
}

func (a adminAPI) unlinkCustomerWeChat(w http.ResponseWriter, r *http.Request) {
	a.updateCustomerIdentity(w, r, adminCustomerIdentityMutation{ClearWeChat: true}, "wechat")
}

func (a adminAPI) freezeCustomerLogin(w http.ResponseWriter, r *http.Request) {
	a.updateCustomerIdentity(w, r, adminCustomerIdentityMutation{Status: "DISABLED"}, "freeze")
}

func (a adminAPI) unfreezeCustomerLogin(w http.ResponseWriter, r *http.Request) {
	a.updateCustomerIdentity(w, r, adminCustomerIdentityMutation{Status: "ACTIVE"}, "unfreeze")
}

func (a adminAPI) customerAuthMergeRequests(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.PathValue("id"))
	if userID == "" {
		writeError(w, http.StatusBadRequest, errors.New("user id is required"))
		return
	}
	items, err := a.store.ListAdminAuthMergeRequests(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"items": adminAuthMergeRequestPayloads(items), "total": len(items)})
}

func (a adminAPI) authMergeRequests(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.URL.Query().Get("userId"))
	items, err := a.store.ListAdminAuthMergeRequests(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"items": adminAuthMergeRequestPayloads(items), "total": len(items)})
}

func (a adminAPI) updateAuthMergeRequest(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("merge request id is required"))
		return
	}
	var req adminAuthMergeRequestMutation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.ResolvedBy) == "" {
		req.ResolvedBy = "admin"
	}
	item, err := a.store.UpdateAdminAuthMergeRequest(id, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "item": adminAuthMergeRequestPayload(item)})
}

func (a adminAPI) previewAuthMergeRequest(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("merge request id is required"))
		return
	}
	item, result, err := a.store.PreviewAdminAuthMergeRequest(id, strings.TrimSpace(r.URL.Query().Get("targetUserId")))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "item": adminAuthMergeRequestPayload(item), "result": result})
}

func (a adminAPI) executeAuthMergeRequest(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("merge request id is required"))
		return
	}
	var req adminAuthMergeExecuteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.ResolvedBy) == "" {
		req.ResolvedBy = "admin"
	}
	item, result, err := a.store.ExecuteAdminAuthMergeRequest(id, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, err := a.revokeCustomerSessions(r.Context(), result.SourceUserID); err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	if _, err := a.revokeCustomerSessions(r.Context(), result.TargetUserID); err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "item": adminAuthMergeRequestPayload(item), "result": result})
}

func (a adminAPI) updateCustomerIdentity(w http.ResponseWriter, r *http.Request, mutation adminCustomerIdentityMutation, action string) {
	userID := strings.TrimSpace(r.PathValue("id"))
	if userID == "" {
		writeError(w, http.StatusBadRequest, errors.New("user id is required"))
		return
	}
	var req adminCustomerIdentityMutation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	mutation.Reason = strings.TrimSpace(req.Reason)
	current, ok, err := a.adminCustomerByID(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, errors.New("user not found"))
		return
	}
	if err := validateAdminIdentityMutation(current, mutation); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	updated, err := a.store.UpdateAdminCustomerIdentity(userID, mutation)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	revoked := 0
	if action == "mobile" || action == "wechat" || action == "freeze" {
		revoked, err = a.revokeCustomerSessions(r.Context(), userID)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, err)
			return
		}
	}
	writeJSON(w, map[string]any{"ok": true, "action": action, "revokedSessions": revoked, "item": adminCustomerIdentityPayload(updated)})
}

func (a adminAPI) adminCustomerByID(userID string) (adminUser, bool, error) {
	data, err := a.store.AdminData()
	if err != nil {
		return adminUser{}, false, err
	}
	for _, user := range data.Users {
		if user.ID == userID {
			return user, true, nil
		}
	}
	return adminUser{}, false, nil
}

func (a adminAPI) revokeCustomerSessions(ctx context.Context, userID string) (int, error) {
	if a.sessions == nil {
		return 0, errAuthSessionUnavailable
	}
	revoker, ok := a.sessions.(authUserSessionStore)
	if !ok {
		return 0, errAuthSessionUnavailable
	}
	return revoker.DeleteUserSessions(ctx, userID)
}

func validateAdminIdentityMutation(user adminUser, mutation adminCustomerIdentityMutation) error {
	if mutation.Status != "" {
		status := strings.ToUpper(strings.TrimSpace(mutation.Status))
		if status != "ACTIVE" && status != "DISABLED" {
			return errors.New("status must be ACTIVE or DISABLED")
		}
	}
	passwordLogin := adminUserPasswordLoginAvailable(user)
	wechatLinked := adminUserWeChatLinked(user)
	mobileBound := strings.TrimSpace(user.Mobile) != ""
	if mutation.ClearMobile && mobileBound && !wechatLinked && !passwordLogin {
		return errors.New("cannot unlink the last usable login identity")
	}
	if mutation.ClearWeChat && wechatLinked && !mobileBound && !passwordLogin {
		return errors.New("cannot unlink the last usable login identity")
	}
	return nil
}

func adminCustomerIdentityPayload(user adminUser) map[string]any {
	wechatIDs := append([]string{}, user.WeChatOpenIDs...)
	loginMethods := []string{}
	if strings.TrimSpace(user.Mobile) != "" {
		loginMethods = append(loginMethods, "mobile_sms")
	}
	if adminUserWeChatLinked(user) {
		loginMethods = append(loginMethods, "wechat_mini_program")
	}
	if adminUserPasswordLoginAvailable(user) {
		loginMethods = append(loginMethods, "password")
	}
	return map[string]any{
		"userId":               user.ID,
		"status":               user.Status,
		"mobileMasked":         maskedMobile(user.Mobile),
		"mobileBound":          strings.TrimSpace(user.Mobile) != "",
		"wechatLinked":         adminUserWeChatLinked(user),
		"wechatOpenIdMasked":   maskAdminAuthIdentifier(firstNonEmptyString(firstString(wechatIDs), user.WeChatUnionID)),
		"wechatOpenIdsMasked":  maskAdminAuthIdentifiers(wechatIDs),
		"wechatUnionIdMasked":  maskAdminAuthIdentifier(user.WeChatUnionID),
		"passwordLoginEnabled": adminUserPasswordLoginAvailable(user),
		"loginMethods":         loginMethods,
		"canUnlinkMobile":      strings.TrimSpace(user.Mobile) == "" || adminUserWeChatLinked(user) || adminUserPasswordLoginAvailable(user),
		"canUnlinkWechat":      !adminUserWeChatLinked(user) || strings.TrimSpace(user.Mobile) != "" || adminUserPasswordLoginAvailable(user),
		"registrationSource":   user.RegistrationSource,
		"updatedAt":            user.UpdatedAt,
	}
}

func adminAuthMergeRequestPayloads(items []adminAuthMergeRequest) []map[string]any {
	payloads := make([]map[string]any, 0, len(items))
	for _, item := range items {
		payloads = append(payloads, adminAuthMergeRequestPayload(item))
	}
	return payloads
}

func adminAuthMergeRequestPayload(item adminAuthMergeRequest) map[string]any {
	return map[string]any{
		"id":                  item.ID,
		"primaryUserId":       item.PrimaryUserID,
		"secondaryUserId":     item.SecondaryUserID,
		"mobileMasked":        maskedMobile(item.Mobile),
		"wechatOpenIdMasked":  maskAdminAuthIdentifier(item.WeChatOpenID),
		"wechatUnionIdMasked": maskAdminAuthIdentifier(item.WeChatUnionID),
		"conflictCode":        item.ConflictCode,
		"source":              item.Source,
		"reason":              item.Reason,
		"status":              item.Status,
		"reviewComment":       item.ReviewComment,
		"resolvedBy":          item.ResolvedBy,
		"resolvedAt":          item.ResolvedAt,
		"createdAt":           item.CreatedAt,
		"updatedAt":           item.UpdatedAt,
	}
}

func adminUserWeChatLinked(user adminUser) bool {
	return len(user.WeChatOpenIDs) > 0 || strings.TrimSpace(user.WeChatUnionID) != ""
}

func adminUserPasswordLoginAvailable(user adminUser) bool {
	email := strings.ToLower(strings.TrimSpace(user.Email))
	if email == "" || strings.HasSuffix(email, "@wechat.local") || strings.HasSuffix(email, "@mobile.local") {
		return false
	}
	return user.PasswordHash != "" || strings.Contains(email, "@")
}

func firstString(items []string) string {
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			return strings.TrimSpace(item)
		}
	}
	return ""
}

func maskAdminAuthIdentifiers(items []string) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		if masked := maskAdminAuthIdentifier(item); masked != "" {
			result = append(result, masked)
		}
	}
	return result
}

func maskAdminAuthIdentifier(value string) string {
	text := strings.TrimSpace(value)
	if text == "" {
		return ""
	}
	if len(text) <= 10 {
		prefixLen := 2
		if len(text) < prefixLen {
			prefixLen = len(text)
		}
		return text[:prefixLen] + "***"
	}
	return text[:6] + "..." + text[len(text)-4:]
}

func (a adminAPI) syncCustomerNewAPI(w http.ResponseWriter, r *http.Request) {
	var req adminNewAPISyncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	route, err := a.store.SyncAdminCustomerNewAPI(r.PathValue("id"), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{"item": route})
}

func (a adminAPI) newAPIGroups(w http.ResponseWriter, r *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	cfg := newAPISyncConfigFromSettings(data.SystemSettings)
	if !cfg.Enabled || strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.AdminCookie) == "" {
		writeJSON(w, map[string]any{"items": []string{}, "configured": false, "available": false})
		return
	}
	groups, err := fetchNewAPIGroups(r.Context(), cfg)
	if err != nil {
		writeJSON(w, map[string]any{
			"items":      []string{},
			"configured": true,
			"available":  false,
			"warning":    err.Error(),
		})
		return
	}
	writeJSON(w, map[string]any{"items": groups, "configured": true, "available": true})
}

func (a adminAPI) channelAgents(w http.ResponseWriter, _ *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	users := userMap(data.Users)
	points := pointMap(data.PointAccounts)
	items := make([]map[string]any, 0, len(data.ChannelAgents))
	for _, agent := range data.ChannelAgents {
		user := users[agent.UserID]
		view := channelAgentView(agent, user)
		view["available"] = points[agent.UserID].Available
		items = append(items, view)
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
	points := pointMap(data.PointAccounts)
	children := map[string][]map[string]any{}
	roots := []map[string]any{}
	for _, agent := range data.ChannelAgents {
		view := channelAgentView(agent, users[agent.UserID])
		view["available"] = points[agent.UserID].Available
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
	writeError(w, http.StatusConflict, errors.New("legacy channel-agent writes are disabled; use customer 360 identity management"))
}

func (a adminAPI) updateChannelAgent(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusConflict, errors.New("legacy channel-agent writes are disabled; use customer 360 identity management"))
}

func (a adminAPI) operationCenters(w http.ResponseWriter, _ *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	users := userMap(data.Users)
	items := make([]map[string]any, 0, len(data.OperationCenters))
	for _, center := range data.OperationCenters {
		view := map[string]any{
			"id":                center.ID,
			"userId":            center.UserID,
			"name":              center.Name,
			"owner":             users[center.UserID].Name,
			"region":            center.Region,
			"inviteCode":        center.InviteCode,
			"responsiblePerson": center.ResponsiblePerson,
			"contactInfo":       center.ContactInfo,
			"settlementProfile": center.SettlementProfile,
			"agreementStatus":   center.AgreementStatus,
			"status":            center.Status,
			"joinOrderId":       center.JoinOrderID,
			"joinFeeCents":      center.JoinFeeCents,
			"approvedAt":        center.ApprovedAt,
			"createdAt":         center.CreatedAt,
			"updatedAt":         center.UpdatedAt,
			"summary":           operationCenterSummary(data, center.ID),
		}
		items = append(items, view)
	}
	writeJSON(w, map[string]any{"items": items})
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
			"active":       plan.Active,
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
	if (req.PriceCents != nil && *req.PriceCents < 0) ||
		(req.GrantPoints != nil && *req.GrantPoints < 0) ||
		(req.DurationDays != nil && *req.DurationDays < 0) ||
		(req.Concurrency != nil && *req.Concurrency < 0) {
		writeError(w, http.StatusBadRequest, errors.New("plan values cannot be negative"))
		return
	}
	plan, err := a.store.UpdateAdminPlan(r.PathValue("id"), req)
	if err != nil {
		writeBusinessPlanAdminError(w, err)
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
		items = append(items, adminOrderView(order, users, plans))
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
		writeAdminOrderMutationError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": order})
}

func (a adminAPI) markOrderPaid(w http.ResponseWriter, r *http.Request) {
	if store, ok := a.store.(*postgresStore); ok {
		var unified bool
		err := store.db.QueryRowContext(r.Context(), `
			SELECT EXISTS(
			  SELECT 1 FROM xz_orders
			  WHERE (id=$1 OR order_no=$1) AND product_id IS NOT NULL
			)
		`, r.PathValue("id")).Scan(&unified)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if unified {
			writePaymentErrorWithCode(w, http.StatusConflict, paymentapp.ErrorCode("PAYMENT_ADMIN_MARK_PAID_FORBIDDEN"), "unified payment orders cannot be marked paid by administrators")
			return
		}
	}
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
		writeAdminOrderMutationError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": order})
}

func writeAdminOrderMutationError(w http.ResponseWriter, err error) {
	var businessErr *businessPlanAdminError
	if errors.As(err, &businessErr) {
		writeBusinessPlanAdminError(w, businessErr)
		return
	}
	writeError(w, http.StatusInternalServerError, err)
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
		moduleCode := firstNonEmptyString(task.ModuleCode, stringValue(task.Params["module_code"]), moduleCodeForType(task.Type))
		items = append(items, map[string]any{
			"id": task.ID, "userId": task.UserID, "user": user.Name,
			"tenantId":          firstNonEmptyString(task.TenantID, stringValue(task.Params["tenant_id"])),
			"agentId":           firstNonEmptyString(task.AgentID, stringValue(task.Params["agent_id"])),
			"operationCenterId": task.OperationCenterID,
			"moduleCode":        moduleCode, "module_code": moduleCode,
			"type": task.Type, "model": task.Model, "status": task.Status,
			"billingType": firstNonEmptyString(task.BillingType, stringValue(task.Params["billing_type"])),
			"progress":    task.Progress, "pointCost": task.PointCost,
			"finalSchemaSnapshot": task.FinalSchemaSnapshot, "limitSnapshot": task.LimitSnapshot,
			"upstreamProvider": task.UpstreamProvider, "upstreamRequestId": task.UpstreamRequestID,
			"userChargeAmount": task.UserChargeAmount, "upstreamCost": task.UpstreamCost, "platformProfit": task.PlatformProfit,
			"failureReason": firstNonEmptyString(task.FailureReason, stringValue(task.Error)),
			"resultIds":     task.ResultIDs, "assets": assetsByTask[task.ID],
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

func (a adminAPI) tokenRecords(w http.ResponseWriter, _ *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	users := userMap(data.Users)
	items := make([]map[string]any, 0, len(data.TokenRecords))
	for _, record := range data.TokenRecords {
		items = append(items, map[string]any{
			"id":           record.ID,
			"userId":       record.UserID,
			"user":         users[record.UserID].Name,
			"orderId":      record.OrderID,
			"changeType":   record.ChangeType,
			"amount":       record.Amount,
			"balanceAfter": record.BalanceAfter,
			"remark":       record.Remark,
			"createdAt":    record.CreatedAt,
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		return stringValue(items[i]["createdAt"]) > stringValue(items[j]["createdAt"])
	})
	writeJSON(w, map[string]any{"items": items})
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

func (a adminAPI) commissionRecords(w http.ResponseWriter, _ *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{
		"summary": commissionRecordSummary(data.Commissions),
		"items":   data.Commissions,
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

func (a adminAPI) approveCommission(w http.ResponseWriter, r *http.Request) {
	item, err := a.store.ReviewAdminCommission(r.PathValue("id"), "APPROVED")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (a adminAPI) rejectCommission(w http.ResponseWriter, r *http.Request) {
	item, err := a.store.ReviewAdminCommission(r.PathValue("id"), "REJECTED")
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
	models := stringSliceFromAny(item["all"])
	var syncedChannel adminAPIChannel
	addedModels := []string{}
	if req.SyncModels {
		syncedChannel, addedModels, err = a.store.MergeAdminAPIChannelModels(r.PathValue("id"), models)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	writeJSON(w, map[string]any{
		"total":            intFromAny(item["modelCount"]),
		"all":              models,
		"imageModels":      stringSliceFromAny(item["imageModels"]),
		"chatModels":       stringSliceFromAny(item["chatModels"]),
		"videoModels":      stringSliceFromAny(item["videoModels"]),
		"protocol":         item["protocol"],
		"imageRequestMode": item["imageRequestMode"],
		"raw":              item["raw"],
		"item":             item,
		"synced":           req.SyncModels,
		"addedModels":      addedModels,
		"candidateModels":  syncedChannel.Models,
		"channel":          syncedChannel,
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

func (a adminAPI) marketingOverview(w http.ResponseWriter, _ *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, marketingPayload(data, "overview"))
}

func (a adminAPI) marketingAgentLevels(w http.ResponseWriter, _ *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, marketingPayload(data, "agentLevels"))
}

func (a adminAPI) marketingInviteRecords(w http.ResponseWriter, _ *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, marketingPayload(data, "inviteRecords"))
}

func (a adminAPI) marketingCommissionRules(w http.ResponseWriter, _ *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, marketingPayload(data, "commissionRules"))
}

func (a adminAPI) updateMarketingCommissionRule(w http.ResponseWriter, r *http.Request) {
	var req adminCommissionRuleMutation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	item, err := a.store.UpdateMarketingCommissionRule(r.PathValue("id"), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (a adminAPI) marketingUpgradePlans(w http.ResponseWriter, _ *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, marketingPayload(data, "upgradePlans"))
}

func (a adminAPI) marketingWallets(w http.ResponseWriter, _ *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, marketingPayload(data, "wallets"))
}

func (a adminAPI) marketingWalletRecords(w http.ResponseWriter, _ *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, marketingPayload(data, "walletRecords"))
}

func (a adminAPI) marketingSettlementStatements(w http.ResponseWriter, _ *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, marketingPayload(data, "settlementStatements"))
}

func (a adminAPI) billingOverview(w http.ResponseWriter, _ *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	payload := billingPayload(data, "overview")
	v1, err := a.billingOverviewV1Payload()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	for key, value := range v1 {
		payload[key] = value
	}
	writeJSON(w, payload)
}

func (a adminAPI) billingCustomers(w http.ResponseWriter, r *http.Request) {
	a.commercialBillingList(w, r, "customers")
}

func (a adminAPI) billingProducts(w http.ResponseWriter, r *http.Request) {
	a.commercialBillingList(w, r, "products")
}

func (a adminAPI) billingPlans(w http.ResponseWriter, r *http.Request) {
	a.commercialBillingList(w, r, "plans")
}

func (a adminAPI) billingSubscriptions(w http.ResponseWriter, r *http.Request) {
	a.commercialBillingList(w, r, "subscriptions")
}

func (a adminAPI) billingEvents(w http.ResponseWriter, _ *http.Request) {
	payload, err := a.billingEventsV1Payload()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, payload)
}

func (a adminAPI) billingUsageSummaries(w http.ResponseWriter, _ *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, billingPayload(data, "usage"))
}

func (a adminAPI) billingBillableMetrics(w http.ResponseWriter, _ *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, billingPayload(data, "billableMetrics"))
}

func (a adminAPI) billingCharges(w http.ResponseWriter, _ *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, billingPayload(data, "charges"))
}

func (a adminAPI) billingFees(w http.ResponseWriter, _ *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, billingPayload(data, "fees"))
}

func (a adminAPI) billingWallets(w http.ResponseWriter, _ *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, billingPayload(data, "wallets"))
}

func (a adminAPI) billingCoupons(w http.ResponseWriter, r *http.Request) {
	a.commercialBillingList(w, r, "coupons")
}

func (a adminAPI) billingInvoices(w http.ResponseWriter, r *http.Request) {
	a.commercialBillingList(w, r, "invoices")
}

func (a adminAPI) billingCreditNotes(w http.ResponseWriter, r *http.Request) {
	a.commercialBillingList(w, r, "creditNotes")
}

func (a adminAPI) billingPaymentRequests(w http.ResponseWriter, r *http.Request) {
	a.commercialBillingList(w, r, "paymentRequests")
}

func (a adminAPI) billingPayments(w http.ResponseWriter, r *http.Request) {
	a.commercialBillingList(w, r, "payments")
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

func marketingPayload(data adminPlatformData, view string) map[string]any {
	inviteRecords := marketingInviteRecordRows(data)
	rules := marketingCommissionRuleRows(data)
	upgradePlans := marketingUpgradePlanRows()
	agentLevels := agentLevelPolicyRows()
	wallets := marketingWalletRows(data)
	walletRecords := marketingWalletRecordRows(data)
	settlementStatements := marketingSettlementStatementRows(data)
	modules := []map[string]any{
		{"module": "角色权限", "p0": 3, "ready": len(agentLevels) == 6, "status": "L0_L5_POLICY_READY", "risk": "高等级代理仍需人工审核和合同流转"},
		{"module": "组织关系", "p0": 2, "ready": len(data.ChannelAgents) > 0, "status": "NEEDS_CLOSURE_TABLE", "risk": "当前仍以 parentId/referredBy 为主"},
		{"module": "邀请推广", "p0": 4, "ready": len(inviteRecords) > 0, "status": "API_READY", "risk": "扫码注册页和二维码生成需要接入真实入口"},
		{"module": "分佣体系", "p0": 5, "ready": len(data.Commissions) > 0 && len(rules) > 0, "status": "RULE_ENGINE_READY", "risk": "支付网关回调仍需接入生产签名验签"},
		{"module": "升级体系", "p0": 4, "ready": len(upgradePlans) == 5, "status": "L0_L5_RULE_READY", "risk": "升级订单和保级自动降级还需要支付回调与月度任务驱动"},
		{"module": "订单支付", "p0": 3, "ready": len(data.Orders) > 0, "status": "IDEMPOTENT_SETTLEMENT_READY", "risk": "营销端微信支付回调需要接入生产验签入口"},
		{"module": "钱包流水", "p0": 1, "ready": len(walletRecords) > 0, "status": "DERIVED_LEDGER_READY", "risk": "独立钱包流水表已预留，当前由佣金和提现实时派生"},
		{"module": "月度结算单", "p0": 1, "ready": len(settlementStatements) > 0, "status": "STATEMENT_VIEW_READY", "risk": "打款凭证和银行回单待接入附件"},
	}
	metrics := []map[string]any{
		{"key": "agents", "label": "代理账号", "value": len(data.ChannelAgents)},
		{"key": "agentLevels", "label": "代理等级", "value": len(agentLevels)},
		{"key": "invites", "label": "邀请记录", "value": len(inviteRecords)},
		{"key": "upgradePlans", "label": "升级方案", "value": len(upgradePlans)},
		{"key": "commissionRules", "label": "分佣规则", "value": len(rules)},
		{"key": "wallets", "label": "钱包账户", "value": len(wallets)},
	}
	payload := map[string]any{
		"view":                 view,
		"metrics":              metrics,
		"modules":              modules,
		"items":                modules,
		"agentLevels":          agentLevels,
		"inviteRecords":        inviteRecords,
		"commissionRules":      rules,
		"upgradePlans":         upgradePlans,
		"wallets":              wallets,
		"walletRecords":        walletRecords,
		"settlementStatements": settlementStatements,
		"qualityGates": []map[string]any{
			{"gate": "权限隔离", "status": "P0", "check": "后台接口按角色和团队范围二次校验"},
			{"gate": "支付幂等", "status": "P0", "check": "同一订单回调只能变更一次角色并生成一组佣金"},
			{"gate": "分佣上限", "status": "P0", "check": "按规则快照校验总分佣不超过平台配置上限"},
			{"gate": "审计留痕", "status": "P0", "check": "改绑、升级、结算、禁用等敏感操作写审计日志"},
		},
	}
	switch view {
	case "inviteRecords":
		payload["items"] = inviteRecords
	case "agentLevels":
		payload["items"] = agentLevels
	case "commissionRules":
		payload["items"] = rules
	case "upgradePlans":
		payload["items"] = upgradePlans
	case "wallets":
		payload["items"] = wallets
	case "walletRecords":
		payload["items"] = walletRecords
	case "settlementStatements":
		payload["items"] = settlementStatements
	}
	return payload
}

func marketingInviteRecordRows(data adminPlatformData) []map[string]any {
	users := userMap(data.Users)
	agentsByUser := agentByUserMap(data.ChannelAgents)
	items := []map[string]any{}
	for _, user := range data.Users {
		if strings.TrimSpace(user.ReferredBy) == "" {
			continue
		}
		agent := agentsByUser[user.ReferredBy]
		inviter := users[user.ReferredBy]
		rechargeStatus := "PENDING"
		upgradeStatus := "PENDING"
		for _, order := range data.Orders {
			if order.UserID != user.ID || !isPaidStatus(order.Status) {
				continue
			}
			rechargeStatus = "PAID"
			if userHasActiveChannelProfile(data, user.ID) {
				upgradeStatus = "UPGRADED"
			}
			break
		}
		items = append(items, map[string]any{
			"id":             "invite_" + shortID(user.ID),
			"inviterUserId":  user.ReferredBy,
			"inviter":        fallback(inviter.Name, user.ReferredBy),
			"inviteeUserId":  user.ID,
			"invitee":        user.Name,
			"inviteCode":     agent.InviteCode,
			"source":         "invite_code",
			"registerStatus": "REGISTERED",
			"rechargeStatus": rechargeStatus,
			"upgradeStatus":  upgradeStatus,
			"createdAt":      user.CreatedAt,
			"status":         "ACTIVE",
		})
	}
	return items
}

func marketingCommissionRuleRows(data adminPlatformData) []map[string]any {
	rules := data.CommissionRules
	if len(rules) == 0 {
		rules = defaultCommissionRules()
	}
	items := make([]map[string]any, 0, len(rules))
	for _, rule := range rules {
		items = append(items, map[string]any{
			"id": rule.ID, "name": rule.Name, "orderType": rule.OrderType, "earnerRole": rule.EarnerRole,
			"relationDepth": rule.RelationDepth, "fixedAmountCents": rule.FixedAmountCents,
			"rate": rule.Rate, "maxTotalRate": rule.MaxTotalRate, "status": rule.Status, "metadata": rule.Metadata,
		})
	}
	return items
}

func marketingUpgradePlanRows() []map[string]any {
	return defaultMarketingUpgradePlans()
}

func marketingWalletRows(data adminPlatformData) []map[string]any {
	users := userMap(data.Users)
	agents := agentByIDMap(data.ChannelAgents)
	withdrawn := map[string]int{}
	pendingWithdrawal := map[string]int{}
	for _, withdrawal := range data.Withdrawals {
		switch strings.ToUpper(withdrawal.Status) {
		case "APPROVED", "PAID", "SETTLED":
			withdrawn[withdrawal.AgentID] += withdrawal.AmountCents
		case "PENDING":
			pendingWithdrawal[withdrawal.AgentID] += withdrawal.AmountCents
		}
	}
	income := map[string]int{}
	pendingCommission := map[string]int{}
	for _, commission := range data.Commissions {
		switch strings.ToUpper(commission.Status) {
		case "SETTLED", "PAID", "APPROVED":
			income[commission.AgentID] += commission.AmountCents
		default:
			pendingCommission[commission.AgentID] += commission.AmountCents
		}
	}
	items := []map[string]any{}
	for _, agent := range data.ChannelAgents {
		user := users[agent.UserID]
		balance := income[agent.ID] - withdrawn[agent.ID] - pendingWithdrawal[agent.ID]
		if balance < 0 {
			balance = 0
		}
		items = append(items, map[string]any{
			"id":                  "wallet_" + shortID(agent.ID),
			"userId":              agent.UserID,
			"agentId":             agent.ID,
			"name":                fallback(user.Name, agent.ID),
			"role":                user.Role,
			"level":               agent.Level,
			"parentAgentId":       agent.ParentID,
			"parentAgentName":     fallback(users[agents[agent.ParentID].UserID].Name, "-"),
			"balanceCents":        balance,
			"frozenCents":         pendingWithdrawal[agent.ID],
			"totalIncomeCents":    income[agent.ID],
			"pendingCommission":   pendingCommission[agent.ID],
			"totalWithdrawCents":  withdrawn[agent.ID],
			"availableToWithdraw": balance,
			"inviteCode":          agent.InviteCode,
			"status":              agent.Status,
		})
	}
	return items
}

func marketingWalletRecordRows(data adminPlatformData) []map[string]any {
	agents := agentByIDMap(data.ChannelAgents)
	users := userMap(data.Users)
	items := []map[string]any{}
	for _, commission := range data.Commissions {
		agent := agents[commission.AgentID]
		user := users[agent.UserID]
		items = append(items, map[string]any{
			"id":          "wallet_record_" + shortID(commission.ID),
			"userId":      agent.UserID,
			"agentId":     commission.AgentID,
			"agentName":   fallback(user.Name, commission.AgentID),
			"bizType":     "COMMISSION_INCOME",
			"bizId":       commission.ID,
			"orderId":     commission.OrderID,
			"amountCents": commission.AmountCents,
			"status":      commission.Status,
			"source":      stringMetadataValueFromMap(commission.RuleSnapshot, "source"),
			"ruleId":      stringMetadataValueFromMap(commission.RuleSnapshot, "ruleId"),
			"createdAt":   commission.CreatedAt,
		})
	}
	for _, withdrawal := range data.Withdrawals {
		agent := agents[withdrawal.AgentID]
		user := users[agent.UserID]
		items = append(items, map[string]any{
			"id":          "wallet_record_" + shortID(withdrawal.ID),
			"userId":      agent.UserID,
			"agentId":     withdrawal.AgentID,
			"agentName":   fallback(user.Name, withdrawal.AgentID),
			"bizType":     "WITHDRAWAL_FREEZE",
			"bizId":       withdrawal.ID,
			"amountCents": -withdrawal.AmountCents,
			"status":      withdrawal.Status,
			"source":      "withdrawal",
			"createdAt":   withdrawal.CreatedAt,
			"reviewedAt":  withdrawal.ReviewedAt,
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		return fmt.Sprint(items[i]["createdAt"]) > fmt.Sprint(items[j]["createdAt"])
	})
	return items
}

func marketingSettlementStatementRows(data adminPlatformData) []map[string]any {
	agents := agentByIDMap(data.ChannelAgents)
	users := userMap(data.Users)
	type statement struct {
		agentID         string
		period          string
		commissionCents int
		withdrawalCents int
		commissionCount int
		withdrawalCount int
		pendingCents    int
	}
	groups := map[string]*statement{}
	for _, commission := range data.Commissions {
		period := settlementPeriod(commission.CreatedAt)
		key := commission.AgentID + ":" + period
		item := groups[key]
		if item == nil {
			item = &statement{agentID: commission.AgentID, period: period}
			groups[key] = item
		}
		item.commissionCount++
		switch strings.ToUpper(commission.Status) {
		case "SETTLED", "PAID", "APPROVED":
			item.commissionCents += commission.AmountCents
		default:
			item.pendingCents += commission.AmountCents
		}
	}
	for _, withdrawal := range data.Withdrawals {
		period := settlementPeriod(firstNonEmpty([]string{withdrawal.ReviewedAt, withdrawal.CreatedAt}))
		key := withdrawal.AgentID + ":" + period
		item := groups[key]
		if item == nil {
			item = &statement{agentID: withdrawal.AgentID, period: period}
			groups[key] = item
		}
		if isSettledStatus(withdrawal.Status) {
			item.withdrawalCents += withdrawal.AmountCents
			item.withdrawalCount++
		}
	}
	items := []map[string]any{}
	for _, group := range groups {
		agent := agents[group.agentID]
		user := users[agent.UserID]
		status := "READY"
		if group.pendingCents > 0 {
			status = "PENDING_COMMISSION"
		}
		if group.commissionCents == 0 && group.withdrawalCents == 0 && group.pendingCents == 0 {
			status = "EMPTY"
		}
		items = append(items, map[string]any{
			"id":              "statement_" + shortID(group.agentID+"_"+group.period),
			"agentId":         group.agentID,
			"agentName":       fallback(user.Name, group.agentID),
			"period":          group.period,
			"commissionCents": group.commissionCents,
			"withdrawalCents": group.withdrawalCents,
			"netPayableCents": group.commissionCents - group.withdrawalCents,
			"pendingCents":    group.pendingCents,
			"commissionCount": group.commissionCount,
			"withdrawalCount": group.withdrawalCount,
			"status":          status,
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if fmt.Sprint(items[i]["period"]) == fmt.Sprint(items[j]["period"]) {
			return fmt.Sprint(items[i]["agentName"]) < fmt.Sprint(items[j]["agentName"])
		}
		return fmt.Sprint(items[i]["period"]) > fmt.Sprint(items[j]["period"])
	})
	return items
}

func settlementPeriod(value string) string {
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed.Format("2006-01")
	}
	return time.Now().UTC().Format("2006-01")
}

func isSettledStatus(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "SETTLED", "PAID", "APPROVED":
		return true
	default:
		return false
	}
}

func stringMetadataValueFromMap(item map[string]any, key string) string {
	if item == nil {
		return ""
	}
	value := strings.TrimSpace(fmt.Sprint(item[key]))
	if value == "<nil>" {
		return ""
	}
	return value
}

func billingPayload(data adminPlatformData, view string) map[string]any {
	customers := billingCustomerRows(data)
	products := billingProductRows(data)
	plans := billingPlanRows(data)
	subscriptions := billingSubscriptionRows(data)
	usage := billingUsageSummaryRows(data)
	events := billingEventRows(data)
	billableMetrics := billingBillableMetricRows(data)
	charges := billingChargeRows(data)
	fees := billingFeeRows(data)
	wallets := billingWalletRows(data)
	coupons := billingCouponRows(data)
	invoices := billingInvoiceRows(data)
	creditNotes := billingCreditNoteRows(data)
	paymentRequests := billingPaymentRequestRows(data)
	payments := billingPaymentRows(data)
	summary := billingSummary(subscriptions, usage, invoices, wallets, creditNotes, paymentRequests)
	items := customers
	switch view {
	case "products":
		items = products
	case "plans":
		items = plans
	case "subscriptions":
		items = subscriptions
	case "events":
		items = events
	case "usage":
		items = usage
	case "billableMetrics":
		items = billableMetrics
	case "charges":
		items = charges
	case "fees":
		items = fees
	case "wallets":
		items = wallets
	case "coupons":
		items = coupons
	case "invoices":
		items = invoices
	case "creditNotes":
		items = creditNotes
	case "paymentRequests":
		items = paymentRequests
	case "payments":
		items = payments
	}
	return map[string]any{
		"metrics": []map[string]any{
			{"key": "mrr", "label": "MRR", "value": summary["mrrCents"], "unit": "cents"},
			{"key": "usageRevenue", "label": "本月用量收入", "value": summary["usageRevenueCents"], "unit": "cents"},
			{"key": "pendingInvoice", "label": "待开票", "value": summary["pendingInvoiceCents"], "unit": "cents"},
			{"key": "overdueInvoices", "label": "逾期账单", "value": summary["overdueInvoices"]},
			{"key": "walletBalance", "label": "钱包余额", "value": summary["walletBalanceCents"], "unit": "cents"},
		},
		"summary":         summary,
		"items":           items,
		"customers":       customers,
		"products":        products,
		"plans":           plans,
		"subscriptions":   subscriptions,
		"events":          events,
		"usage":           usage,
		"billableMetrics": billableMetrics,
		"charges":         charges,
		"fees":            fees,
		"wallets":         wallets,
		"coupons":         coupons,
		"invoices":        invoices,
		"creditNotes":     creditNotes,
		"paymentRequests": paymentRequests,
		"payments":        payments,
		"workflow": []map[string]any{
			{"stage": "客户档案", "count": len(customers), "status": "READY"},
			{"stage": "订阅生效", "count": len(subscriptions), "status": "ACTIVE"},
			{"stage": "指标计量", "count": len(billableMetrics), "status": "METERING"},
			{"stage": "费用生成", "count": len(fees), "status": "RATING"},
			{"stage": "账单生成", "count": len(invoices), "status": "FINALIZING"},
			{"stage": "支付请求", "count": len(paymentRequests), "status": "COLLECTING"},
			{"stage": "贷项调整", "count": len(creditNotes), "status": "ADJUSTING"},
		},
	}
}

func billingSummary(subscriptions []map[string]any, usage []map[string]any, invoices []map[string]any, wallets []map[string]any, creditNotes []map[string]any, paymentRequests []map[string]any) map[string]any {
	mrr := 0
	for _, item := range subscriptions {
		mrr += intValue(item["monthlyAmountCents"])
	}
	usageRevenue := 0
	for _, item := range usage {
		usageRevenue += intValue(item["amountCents"])
	}
	pendingInvoice := 0
	overdueInvoices := 0
	for _, item := range invoices {
		status := strings.ToUpper(stringValue(item["status"]))
		if status == "DRAFT" || status == "FINALIZED" || status == "PAYMENT_PENDING" {
			pendingInvoice += intValue(item["amountCents"])
		}
		if status == "OVERDUE" {
			overdueInvoices++
		}
	}
	walletBalance := 0
	for _, item := range wallets {
		walletBalance += intValue(item["balanceCents"])
	}
	return map[string]any{
		"mrrCents":             mrr,
		"usageRevenueCents":    usageRevenue,
		"pendingInvoiceCents":  pendingInvoice,
		"overdueInvoices":      overdueInvoices,
		"activeSubscriptions":  countRowsByStatus(subscriptions, "ACTIVE"),
		"meteredEventCount":    len(usage),
		"invoiceCount":         len(invoices),
		"walletBalanceCents":   walletBalance,
		"creditNoteCount":      len(creditNotes),
		"paymentRequestCount":  len(paymentRequests),
		"estimatedGrossMargin": 68,
	}
}

func billingCustomerRows(data adminPlatformData) []map[string]any {
	plans := planMap(data.Plans)
	points := pointMap(data.PointAccounts)
	users := userMap(data.Users)
	agentsByUser := agentByUserMap(data.ChannelAgents)
	items := []map[string]any{}
	for _, user := range data.Users {
		if user.Role == "SUPER_ADMIN" {
			continue
		}
		plan := plans[user.PlanID]
		sourceAgent := agentsByUser[user.ReferredBy]
		sourceUser := users[sourceAgent.UserID]
		billingStatus := "ACTIVE"
		if strings.ToUpper(user.Status) != "ACTIVE" {
			billingStatus = "PAUSED"
		}
		items = append(items, map[string]any{
			"id": user.ID, "customer": user.Name, "email": user.Email,
			"billingStatus": billingStatus, "status": billingStatus,
			"plan": planName(plan), "planId": user.PlanID,
			"subscription": "sub_" + shortID(user.ID), "subscriptionStatus": billingStatus,
			"prepaidBalanceCents": points[user.ID].Available * 10,
			"walletCode":          "wallet_" + shortID(user.ID),
			"netPaymentTerm":      7,
			"invoiceGracePeriod":  3,
			"taxRate":             "6%",
			"taxStatus":           "SUCCEEDED",
			"coupon":              billingCouponForPlan(user.PlanID),
			"paymentMethod":       "线下转账/人工确认",
			"invoiceTitle":        fallback(user.Name, "未填写") + " 发票抬头",
			"taxNumber":           "待补全",
			"sourceAgentName":     sourceUser.Name,
			"customerGroup":       billingCustomerGroupName(data, user),
			"createdAt":           user.CreatedAt,
		})
	}
	return items
}

type billingPlanOffer struct {
	ID                    string
	Code                  string
	Name                  string
	PlanType              string
	BillingMode           string
	ChargeModel           string
	PriceCents            int
	Points                int
	DurationDays          int
	Concurrency           int
	OverageUnitPriceCents int
	TrialDays             int
	ValidityPolicy        string
	PayInAdvance          bool
	Active                bool
}

func billingPlanOffers(data adminPlatformData) []billingPlanOffer {
	items := []billingPlanOffer{}
	for _, plan := range data.Plans {
		price := planPrice(plan)
		planType := "SUBSCRIPTION"
		billingMode := "订阅 + 超额按量"
		chargeModel := "standard subscription + metered charges"
		if price == 0 {
			planType = "FREE_TRIAL"
			billingMode = "免费额度 + 超额按量"
		} else if plan.DurationDays >= 360 {
			billingMode = "年付订阅 + 超额按量"
		} else {
			billingMode = "月付订阅 + 超额按量"
		}
		items = append(items, billingPlanOffer{
			ID: fallback(plan.ID, "plan_"+plan.Code), Code: fallback(plan.Code, plan.ID), Name: plan.Name,
			PlanType: planType, BillingMode: billingMode, ChargeModel: chargeModel,
			PriceCents: price, Points: planPoints(plan), DurationDays: plan.DurationDays, Concurrency: plan.Concurrency,
			OverageUnitPriceCents: 12, TrialDays: map[bool]int{true: 0, false: 7}[price > 0],
			ValidityPolicy: "",
			PayInAdvance:   price > 0, Active: plan.Active || plan.ID != "",
		})
	}
	items = append(items,
		billingPlanOffer{
			ID: "plan_credit_pack", Code: "credit_pack", Name: "计次包",
			PlanType: "PREPAID_PACKAGE", BillingMode: "计次预付，按次扣减", ChargeModel: "package",
			PriceCents: 19900, Points: 10000, DurationDays: 365, Concurrency: 2,
			OverageUnitPriceCents: 0, TrialDays: 0, ValidityPolicy: "永久有效", PayInAdvance: true, Active: true,
		},
		billingPlanOffer{
			ID: "plan_pay_as_you_go", Code: "pay_as_you_go", Name: "按量计费",
			PlanType: "USAGE_BASED", BillingMode: "后付费，月底按量出账", ChargeModel: "standard",
			PriceCents: 0, Points: 0, DurationDays: 30, Concurrency: 1,
			OverageUnitPriceCents: 0, TrialDays: 0, ValidityPolicy: "", PayInAdvance: false, Active: true,
		},
	)
	return items
}

func billingProductRows(data adminPlatformData) []map[string]any {
	items := []map[string]any{}
	products := productsWithUsage(data)
	for _, plan := range billingPlanOffers(data) {
		for index, product := range products {
			if index > 2 && plan.ID == "plan_free" {
				continue
			}
			includedQuota := billingIncludedQuota(plan, product.Type)
			unitPrice := billingOfferUnitPrice(plan, product.Type)
			items = append(items, map[string]any{
				"id":      "sku_" + safeID(plan.ID+"_"+product.ID),
				"name":    plan.Name + " - " + product.Name,
				"skuCode": strings.ToUpper(fallback(plan.Code, plan.ID)) + "_" + strings.ToUpper(safeID(product.Type)),
				"plan":    plan.Name, "planCode": fallback(plan.Code, plan.ID),
				"planType": plan.PlanType, "billingMode": plan.BillingMode,
				"product": product.Name, "type": product.Type,
				"cyclePolicy":     billingOfferCyclePolicy(plan),
				"baseAmountCents": plan.PriceCents, "monthlyAmountCents": monthlyAmount(plan.PriceCents, plan.DurationDays),
				"metricCode": billingMetricCode(product.Type), "aggregationType": billingAggregationType(product.Type),
				"chargeModel": billingChargeModel(product.Type), "includedQuota": includedQuota,
				"freeQuota": includedQuota, "overageUnitPriceCents": unitPrice,
				"unitPriceCents": unitPrice, "payInAdvance": plan.PayInAdvance || product.Type == "OPS_LOGIN",
				"invoiceable": product.Type != "OPS_LOGIN", "minAmountCents": billingMinimumAmount(product.Type),
				"pricingGroupKeys": billingPricingGroupKeys(product.Type), "taxes": "CN-VAT-6",
				"couponTargets": billingCouponForPlan(plan.ID), "entitlements": billingPlanProductEntitlements(plan, product),
				"status": map[bool]string{true: "ACTIVE", false: "DISABLED"}[plan.Active && product.Status != "DISABLED"],
			})
		}
	}
	return items
}

func billingPlanRows(data adminPlatformData) []map[string]any {
	items := []map[string]any{}
	for _, plan := range billingPlanOffers(data) {
		items = append(items, map[string]any{
			"id": plan.ID, "code": fallback(plan.Code, plan.ID), "name": plan.Name,
			"planType": plan.PlanType, "billingMode": plan.BillingMode,
			"version": "v1", "billingCycle": billingCycle(plan.DurationDays),
			"interval":           strings.ToLower(billingCycle(plan.DurationDays)),
			"payInAdvance":       plan.PayInAdvance,
			"trialDays":          plan.TrialDays,
			"billChargesMonthly": plan.DurationDays >= 360,
			"baseAmountCents":    plan.PriceCents, "monthlyAmountCents": monthlyAmount(plan.PriceCents, plan.DurationDays),
			"freeQuota": plan.Points, "overageUnitPriceCents": plan.OverageUnitPriceCents,
			"chargeModel":   plan.ChargeModel,
			"couponTargets": billingCouponForPlan(plan.ID),
			"taxes":         "CN-VAT-6",
			"entitlements": strings.Join([]string{
				"点数 " + strconv.Itoa(plan.Points),
				"并发 " + strconv.Itoa(plan.Concurrency),
				"周期 " + strconv.Itoa(plan.DurationDays) + " 天",
			}, " / "),
			"status": map[bool]string{true: "ACTIVE", false: "DISABLED"}[plan.Active],
		})
	}
	return items
}

func billingSubscriptionRows(data adminPlatformData) []map[string]any {
	plans := planMap(data.Plans)
	points := pointMap(data.PointAccounts)
	items := []map[string]any{}
	for _, user := range data.Users {
		if user.Role == "SUPER_ADMIN" {
			continue
		}
		plan := plans[user.PlanID]
		status := "ACTIVE"
		if strings.ToUpper(user.Status) != "ACTIVE" {
			status = "PAUSED"
		}
		items = append(items, map[string]any{
			"id": "sub_" + shortID(user.ID), "customerId": user.ID, "customer": user.Name,
			"externalId": "ext_" + shortID(user.ID),
			"plan":       planName(plan), "planId": user.PlanID, "planVersion": "v1",
			"status": status, "billingCycle": billingCycle(plan.DurationDays),
			"billingTime":          "calendar",
			"onTerminationInvoice": "generate",
			"onTerminationCredit":  "credit",
			"startedAt":            fallback(user.CreatedAt, "2026-06-01"),
			"currentPeriodStart":   fallback(user.CreatedAt, "2026-06-01"),
			"currentPeriodEnd":     fallback(user.SubscriptionExpiresAt, "2026-07-01"),
			"monthlyAmountCents":   monthlyAmount(planPrice(plan), plan.DurationDays),
			"prepaidBalanceCents":  points[user.ID].Available * 10,
			"lifetimeUsageCents":   points[user.ID].Frozen * 10,
			"entitlementSnapshot":  "鐐规暟 " + strconv.Itoa(planPoints(plan)) + " / 骞跺彂 " + strconv.Itoa(plan.Concurrency),
			"cancelAtPeriodEnd":    false,
		})
	}
	return items
}

func billingUsageSummaryRows(data adminPlatformData) []map[string]any {
	rows := usageRows(data, "")
	items := []map[string]any{}
	for index, row := range rows {
		metric := stringValue(row["metric"])
		metricCode := "image.generations"
		if strings.Contains(metric, "瀵硅瘽") {
			metricCode = "agent.messages"
		} else if strings.Contains(metric, "GEO") {
			metricCode = "geo.monitor_tasks"
		}
		usage := intValue(row["usage"])
		cost := intValue(row["costCents"])
		items = append(items, map[string]any{
			"id":     "usage_202606_" + strconv.Itoa(index+1),
			"period": "2026-06", "product": row["product"], "metric": metric,
			"metricCode": metricCode, "usage": usage, "quantity": usage,
			"costCents": cost, "amountCents": cost * 3,
			"aggregation": "sum_agg", "chargeModel": "standard",
			"feeType": "charge", "paymentStatus": "pending", "status": "METERED",
		})
	}
	return items
}

func billingEventRows(data adminPlatformData) []map[string]any {
	items := []map[string]any{}
	for index, event := range data.BillingEvents {
		items = append(items, map[string]any{
			"id": event.ID, "transactionId": event.TransactionID,
			"customerId": event.UserID, "subscriptionId": "sub_" + shortID(event.UserID),
			"agentId": event.AgentID, "tenantId": event.TenantID, "operationCenterId": event.OperationCenterID,
			"moduleCode": event.ModuleCode, "taskId": event.TaskID,
			"metricCode": event.MetricCode, "quantity": event.Quantity,
			"unitAmountCents": event.UnitAmountCents, "amountCents": event.AmountCents,
			"pointCost": event.PointCost, "balanceBefore": event.BalanceBefore,
			"balanceAfter": event.BalanceAfter, "model": event.Model,
			"status": strings.ToUpper(event.Status), "occurredAt": event.OccurredAt,
		})
		if index >= 19 {
			return items
		}
	}
	for index, task := range data.GenerationTasks {
		items = append(items, map[string]any{
			"id": "evt_img_" + shortID(task.ID), "transactionId": "txn_img_" + shortID(task.ID),
			"customerId": task.UserID, "subscriptionId": "sub_" + shortID(task.UserID),
			"metricCode": "image.generations", "quantity": 1,
			"unitAmountCents": 36, "status": strings.ToUpper(task.Status),
			"occurredAt": task.CreatedAt,
		})
		if index >= 19 {
			break
		}
	}
	for index, call := range data.AgentCalls {
		items = append(items, map[string]any{
			"id": "evt_agent_" + shortID(call.ID), "transactionId": "txn_agent_" + shortID(call.ID),
			"customerId": call.UserID, "subscriptionId": "sub_" + shortID(call.UserID),
			"metricCode": "agent.messages", "quantity": call.TokenUsage,
			"unitAmountCents": call.CostCents, "status": "SUCCEEDED",
			"occurredAt": call.CreatedAt,
		})
		if index >= 19 {
			break
		}
	}
	if len(items) == 0 {
		items = append(items, map[string]any{"id": "evt_demo_0001", "transactionId": "txn_demo_0001", "customerId": "user_000002", "subscriptionId": "sub_000002", "metricCode": "api.output_tokens", "quantity": 12800, "unitAmountCents": 128, "status": "SUCCEEDED", "occurredAt": "2026-06-24"})
	}
	return items
}

func billingBillableMetricRows(data adminPlatformData) []map[string]any {
	items := []map[string]any{}
	for _, product := range productsWithUsage(data) {
		code := billingMetricCode(product.Type)
		items = append(items, map[string]any{
			"id": "bm_" + shortID(product.ID), "code": code,
			"name": product.Name + "璁￠噺鎸囨爣", "product": product.Name,
			"aggregationType": billingAggregationType(product.Type),
			"fieldName":       billingFieldName(product.Type),
			"expression":      billingMetricExpression(product.Type),
			"recurring":       product.Type != "OPS_LOGIN",
			"rounding":        "ceil",
			"chargeModels":    billingChargeModel(product.Type),
			"status":          product.Status,
		})
	}
	for _, rule := range data.BillingRules {
		rule = normalizeBillingRuleAliases(rule)
		moduleCode := canonicalModuleCode(rule.ModuleCode)
		items = append(items, map[string]any{
			"id":              "bm_ai_" + safeID(moduleCode+"_"+rule.ModelName),
			"code":            billingMetricForModule(moduleCode),
			"name":            moduleCode + " " + rule.ModelName,
			"product":         "AI Capability",
			"moduleCode":      moduleCode,
			"modelName":       rule.ModelName,
			"aggregationType": "sum",
			"fieldName":       billingQuantityField(rule.BillingType),
			"expression":      "sum(" + billingQuantityField(rule.BillingType) + ")",
			"recurring":       true,
			"rounding":        "ceil",
			"chargeModels":    rule.BillingType,
			"status":          rule.Status,
		})
	}
	return items
}

func billingChargeRows(data adminPlatformData) []map[string]any {
	products := productsWithUsage(data)
	items := []map[string]any{}
	for _, plan := range data.Plans {
		if len(products) == 0 {
			break
		}
		for index, product := range products {
			if index > 2 && plan.ID == "plan_free" {
				continue
			}
			items = append(items, map[string]any{
				"id":   "charge_" + safeID(plan.ID+"_"+product.ID),
				"plan": plan.Name, "planCode": fallback(plan.Code, plan.ID),
				"product": product.Name, "billableMetricCode": billingMetricCode(product.Type),
				"chargeModel":      billingChargeModel(product.Type),
				"aggregationType":  billingAggregationType(product.Type),
				"amountCents":      billingUnitPrice(product.Type),
				"freeUnits":        billingFreeQuota(product.Type),
				"minAmountCents":   billingMinimumAmount(product.Type),
				"payInAdvance":     product.Type == "OPS_LOGIN",
				"invoiceable":      product.Type != "OPS_LOGIN",
				"prorated":         plan.DurationDays >= 30,
				"pricingGroupKeys": billingPricingGroupKeys(product.Type),
				"taxes":            "CN-VAT-6",
				"status":           map[bool]string{true: "ACTIVE", false: "DISABLED"}[plan.Active || plan.ID != ""],
			})
		}
	}
	for _, rule := range data.BillingRules {
		rule = normalizeBillingRuleAliases(rule)
		moduleCode := canonicalModuleCode(rule.ModuleCode)
		items = append(items, map[string]any{
			"id":                  "charge_ai_" + safeID(moduleCode+"_"+rule.ModelName),
			"plan":                "AI capability metered",
			"planCode":            "ai_capability",
			"product":             moduleCode,
			"moduleCode":          moduleCode,
			"modelName":           rule.ModelName,
			"billableMetricCode":  billingMetricForModule(moduleCode),
			"chargeModel":         rule.BillingType,
			"aggregationType":     "sum",
			"amountCents":         int(math.Ceil(rule.BasePrice)) * pointUnitAmountCents,
			"costPrice":           rule.CostPrice,
			"currencyType":        rule.CurrencyType,
			"parameterMultiplier": rule.ParameterMultiplier,
			"freeUnits":           0,
			"minAmountCents":      pointUnitAmountCents,
			"payInAdvance":        false,
			"invoiceable":         true,
			"prorated":            false,
			"pricingGroupKeys":    []string{"module_code", "model_name"},
			"taxes":               "CN-VAT-6",
			"status":              rule.Status,
		})
	}
	return items
}

func billingFeeRows(data adminPlatformData) []map[string]any {
	items := []map[string]any{}
	for index, sub := range billingSubscriptionRows(data) {
		items = append(items, map[string]any{
			"id":        "fee_sub_" + strconv.Itoa(index+1),
			"invoiceId": "bill_demo_" + strconv.Itoa(index+1),
			"customer":  sub["customer"], "subscriptionId": sub["id"],
			"feeType": "subscription", "invoiceableType": "Subscription",
			"amountCents": sub["monthlyAmountCents"], "taxesAmountCents": intValue(sub["monthlyAmountCents"]) * 6 / 100,
			"totalAmountCents": intValue(sub["monthlyAmountCents"]) * 106 / 100,
			"units":            1, "eventsCount": 0, "paymentStatus": "pending", "status": "active",
		})
	}
	for index, usage := range billingUsageSummaryRows(data) {
		items = append(items, map[string]any{
			"id":        "fee_usage_" + strconv.Itoa(index+1),
			"invoiceId": "bill_usage_202606",
			"customer":  usage["product"], "subscriptionId": "-",
			"feeType": "charge", "invoiceableType": "Charge",
			"amountCents": usage["amountCents"], "taxesAmountCents": intValue(usage["amountCents"]) * 6 / 100,
			"totalAmountCents": intValue(usage["amountCents"]) * 106 / 100,
			"units":            usage["quantity"], "eventsCount": usage["quantity"], "paymentStatus": "pending", "status": "active",
		})
	}
	return items
}

func billingWalletRows(data adminPlatformData) []map[string]any {
	users := userMap(data.Users)
	items := []map[string]any{}
	for _, account := range data.PointAccounts {
		user := users[account.UserID]
		if user.Role == "SUPER_ADMIN" {
			continue
		}
		items = append(items, map[string]any{
			"id": "wallet_" + shortID(account.UserID), "customerId": account.UserID, "customer": user.Name,
			"code":   "prepaid_points_" + shortID(account.UserID),
			"status": "active", "currency": "CNY",
			"rateAmount": 0.10, "priority": 50,
			"balanceCents": account.Available * 10, "consumedAmountCents": account.Frozen * 10,
			"ongoingUsageBalanceCents": 0, "invoiceRequiresSuccessfulPayment": true,
			"paidTopUpMinAmountCents": 10000, "paymentMethodType": "provider",
			"targetMetrics": "image.generations / api.tokens / agent.messages",
		})
	}
	return items
}

func billingCouponRows(data adminPlatformData) []map[string]any {
	items := []map[string]any{
		{"id": "coupon_launch_20", "code": "LAUNCH20", "name": "首年 8 折", "couponType": "percentage", "percentageRate": 20, "frequency": "recurring", "frequencyDuration": 12, "reusable": true, "status": "active", "targets": "plan_month / plan_year"},
		{"id": "coupon_agent_credit", "code": "AGENT_CREDIT", "name": "渠道客户抵扣", "couponType": "fixed_amount", "amountCents": 3000, "frequency": "once", "reusable": true, "status": "active", "targets": "channel_a"},
	}
	if len(data.Plans) > 2 {
		items = append(items, map[string]any{"id": "coupon_year_upgrade", "code": "YEAR_UPGRADE", "name": "年付升级券", "couponType": "fixed_amount", "amountCents": 10000, "frequency": "once", "reusable": false, "status": "active", "targets": "plan_year"})
	}
	return items
}

func billingInvoiceRows(data adminPlatformData) []map[string]any {
	users := userMap(data.Users)
	plans := planMap(data.Plans)
	items := []map[string]any{}
	for index, order := range data.Orders {
		user := users[order.UserID]
		status := "PAYMENT_PENDING"
		invoiceStatus := "REQUESTED"
		if isPaidStatus(order.Status) {
			status = "PAID"
			invoiceStatus = "ISSUED"
		} else if index%3 == 0 {
			status = "OVERDUE"
		}
		amount := orderAmount(order)
		couponAmount := amount / 20
		taxAmount := (amount - couponAmount) * 6 / 100
		items = append(items, map[string]any{
			"id": "bill_" + shortID(order.ID), "invoiceNo": "XZ-BILL-202606-" + strconv.Itoa(index+1),
			"customer": user.Name, "customerId": order.UserID,
			"subscriptionId": "sub_" + shortID(order.UserID), "plan": planName(plans[order.PlanID]),
			"invoiceType": "subscription", "amountCents": amount,
			"subtotalAmountCents": amount, "couponsAmountCents": couponAmount, "taxesAmountCents": taxAmount,
			"creditNotesAmountCents": 0, "prepaidCreditAmountCents": 0, "totalDueAmountCents": amount - couponAmount + taxAmount,
			"status": status, "paymentStatus": billingPaymentStatus(status), "taxStatus": "succeeded",
			"invoiceStatus": invoiceStatus, "readyToFinalize": status == "PAYMENT_PENDING", "billingPeriod": "2026-06",
			"dueAt": "2026-07-05", "createdAt": order.CreatedAt,
		})
	}
	if len(items) == 0 {
		for index, sub := range billingSubscriptionRows(data) {
			amount := intValue(sub["monthlyAmountCents"])
			items = append(items, map[string]any{
				"id": "bill_demo_" + strconv.Itoa(index+1), "invoiceNo": "XZ-BILL-202606-" + strconv.Itoa(index+1),
				"customer": sub["customer"], "customerId": sub["customerId"], "subscriptionId": sub["id"],
				"plan": sub["plan"], "invoiceType": "subscription", "amountCents": amount,
				"subtotalAmountCents": amount, "couponsAmountCents": 0, "taxesAmountCents": amount * 6 / 100,
				"creditNotesAmountCents": 0, "prepaidCreditAmountCents": 0, "totalDueAmountCents": amount * 106 / 100,
				"status": "DRAFT", "paymentStatus": "pending", "taxStatus": "pending",
				"invoiceStatus": "NOT_REQUESTED", "readyToFinalize": true, "billingPeriod": "2026-06",
				"dueAt": "2026-07-05", "createdAt": "2026-06-24",
			})
		}
	}
	return items
}

func billingCreditNoteRows(data adminPlatformData) []map[string]any {
	invoices := billingInvoiceRows(data)
	items := []map[string]any{}
	for index, invoice := range invoices {
		if index%3 != 0 && strings.ToUpper(stringValue(invoice["status"])) != "OVERDUE" {
			continue
		}
		total := intValue(invoice["amountCents"]) / 10
		items = append(items, map[string]any{
			"id": "cn_" + shortID(stringValue(invoice["id"])), "number": "XZ-CN-202606-" + strconv.Itoa(index+1),
			"invoiceNo": invoice["invoiceNo"], "customer": invoice["customer"],
			"reason": "duplicated_charge", "creditStatus": "available", "refundStatus": "pending", "status": "finalized",
			"creditAmountCents": total, "refundAmountCents": 0, "offsetAmountCents": total / 2,
			"balanceAmountCents": total / 2, "taxesAmountCents": total * 6 / 100,
			"createdAt": invoice["createdAt"],
		})
	}
	return items
}

func billingPaymentRequestRows(data adminPlatformData) []map[string]any {
	items := []map[string]any{}
	for index, invoice := range billingInvoiceRows(data) {
		if strings.ToUpper(stringValue(invoice["status"])) == "PAID" {
			continue
		}
		items = append(items, map[string]any{
			"id": "pr_" + shortID(stringValue(invoice["id"])), "customerId": invoice["customerId"], "customer": invoice["customer"],
			"email":    stringValue(invoice["customerId"]) + "@billing.local",
			"invoices": invoice["invoiceNo"], "amountCents": invoice["totalDueAmountCents"],
			"totalDueAmountCents": invoice["totalDueAmountCents"], "paymentStatus": billingPaymentStatus(stringValue(invoice["status"])),
			"readyForPaymentProcessing": true, "dunningCampaign": map[bool]string{true: "D+7 鑷姩鍌敹", false: "姝ｅ父鏀舵"}[strings.ToUpper(stringValue(invoice["status"])) == "OVERDUE"],
			"createdAt": invoice["createdAt"], "dueAt": invoice["dueAt"],
			"status": strings.ToUpper(billingPaymentStatus(stringValue(invoice["status"]))),
		})
		if index >= 30 {
			break
		}
	}
	return items
}

func billingPaymentRows(data adminPlatformData) []map[string]any {
	items := []map[string]any{}
	for index, payment := range data.Payments {
		items = append(items, map[string]any{
			"id": payment.ID, "orderId": payment.OrderID, "channel": payment.Channel,
			"paymentRequestId": "pr_" + shortID(payment.OrderID), "payableType": "PaymentRequest",
			"amountCents": payment.Amount, "status": payment.Status, "paymentStatus": billingPaymentStatus(payment.Status),
			"gateway": payment.Channel, "provider": payment.Channel, "paymentMethodType": "provider", "createdAt": payment.CreatedAt,
		})
		if index >= 30 {
			break
		}
	}
	if len(items) == 0 {
		for index, invoice := range billingInvoiceRows(data) {
			if strings.ToUpper(stringValue(invoice["status"])) != "PAID" {
				continue
			}
			items = append(items, map[string]any{
				"id": "pay_demo_" + strconv.Itoa(index+1), "orderId": invoice["id"],
				"paymentRequestId": "pr_" + shortID(stringValue(invoice["id"])), "payableType": "Invoice",
				"channel": "offline_transfer", "gateway": "绾夸笅杞处",
				"amountCents": invoice["amountCents"], "status": "PAID", "paymentStatus": "succeeded",
				"provider": "manual", "paymentMethodType": "manual", "createdAt": invoice["createdAt"],
			})
		}
	}
	return items
}

func billingCustomerGroupName(data adminPlatformData, user adminUser) string {
	if userHasActiveChannelProfile(data, user.ID) {
		return "channel_a"
	}
	if strings.Contains(strings.ToLower(user.Email), "vip") && len(data.CustomerGroups) > 1 {
		return data.CustomerGroups[1].Name
	}
	if len(data.CustomerGroups) > 0 {
		return data.CustomerGroups[0].Name
	}
	return "default"
}

func billingCouponForPlan(planID string) string {
	if strings.Contains(planID, "year") {
		return "YEAR_UPGRADE"
	}
	if strings.Contains(planID, "month") {
		return "LAUNCH20"
	}
	return "-"
}

func billingIncludedQuota(plan billingPlanOffer, productType string) int {
	if plan.PlanType == "USAGE_BASED" {
		return 0
	}
	points := plan.Points
	switch productType {
	case "API":
		return points * 100
	case "TEXT_TO_IMAGE":
		return points / 10
	case "AGENT":
		return points * 3
	case "GEO":
		return maxInt(1, plan.Concurrency*10)
	case "OPS_LOGIN":
		if plan.PriceCents > 0 {
			return 1
		}
		return 0
	default:
		return points
	}
}

func billingOfferUnitPrice(plan billingPlanOffer, productType string) int {
	if plan.PlanType == "PREPAID_PACKAGE" {
		switch productType {
		case "TEXT_TO_IMAGE":
			return 0
		case "API":
			return 1
		case "AGENT":
			return 2
		case "GEO":
			return 120
		default:
			return billingUnitPrice(productType)
		}
	}
	return billingUnitPrice(productType)
}

func billingOfferCyclePolicy(plan billingPlanOffer) string {
	switch plan.PlanType {
	case "PREPAID_PACKAGE":
		if plan.ValidityPolicy != "" {
			return plan.ValidityPolicy
		}
		return "有效期 " + strconv.Itoa(plan.DurationDays) + " 天"
	case "USAGE_BASED":
		return "月底按量出账"
	case "FREE_TRIAL":
		return "长期免费额度"
	default:
		if plan.DurationDays >= 360 {
			return "年度订阅"
		}
		if plan.DurationDays >= 28 {
			return "月度订阅"
		}
		return strconv.Itoa(plan.DurationDays) + " 天订阅"
	}
}

func billingPlanProductEntitlements(plan billingPlanOffer, product adminProduct) string {
	parts := []string{
		"套餐点数 " + strconv.Itoa(plan.Points),
		"并发 " + strconv.Itoa(plan.Concurrency),
	}
	if plan.PlanType == "PREPAID_PACKAGE" {
		parts = append(parts, "点数按次扣减", "余额不足需充值")
	}
	if plan.PlanType == "USAGE_BASED" {
		parts = append(parts, "无预置额度", "月底按量出账")
	}
	if len(product.Entitlements) > 0 {
		parts = append(parts, product.Entitlements...)
	}
	return strings.Join(parts, " / ")
}

func billingMetricCode(productType string) string {
	switch productType {
	case "API":
		return "api.tokens"
	case "TEXT_TO_IMAGE":
		return "image.generations"
	case "AGENT":
		return "agent.messages"
	case "GEO":
		return "geo.monitor_tasks"
	case "OPS_LOGIN":
		return "ops.service_items"
	default:
		return "usage.units"
	}
}

func billingAggregationType(productType string) string {
	switch productType {
	case "API", "AGENT":
		return "sum_agg"
	case "TEXT_TO_IMAGE", "GEO", "OPS_LOGIN":
		return "count_agg"
	default:
		return "sum_agg"
	}
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func billingFieldName(productType string) string {
	switch productType {
	case "API", "AGENT":
		return "tokens"
	case "GEO":
		return "tasks"
	case "OPS_LOGIN":
		return "deliverables"
	default:
		return "quantity"
	}
}

func billingMetricExpression(productType string) string {
	switch productType {
	case "API":
		return "input_tokens + output_tokens"
	case "AGENT":
		return "messages + tool_calls"
	case "TEXT_TO_IMAGE":
		return "successful_generations"
	default:
		return "-"
	}
}

func billingChargeModel(productType string) string {
	switch productType {
	case "API":
		return "standard"
	case "TEXT_TO_IMAGE":
		return "package"
	case "AGENT":
		return "graduated"
	case "GEO":
		return "volume"
	case "OPS_LOGIN":
		return "standard"
	default:
		return "standard"
	}
}

func billingFreeQuota(productType string) int {
	switch productType {
	case "API":
		return 100000
	case "TEXT_TO_IMAGE":
		return 300
	case "AGENT":
		return 10000
	case "GEO":
		return 20
	default:
		return 0
	}
}

func billingUnitPrice(productType string) int {
	switch productType {
	case "API":
		return 2
	case "TEXT_TO_IMAGE":
		return 36
	case "AGENT":
		return 4
	case "GEO":
		return 200
	case "OPS_LOGIN":
		return 30000
	default:
		return 1
	}
}

func billingMinimumAmount(productType string) int {
	switch productType {
	case "OPS_LOGIN":
		return 30000
	case "GEO":
		return 2000
	default:
		return 0
	}
}

func billingPricingGroupKeys(productType string) string {
	switch productType {
	case "API":
		return "model,provider"
	case "TEXT_TO_IMAGE":
		return "model,size,quality"
	case "AGENT":
		return "agent_id,tool"
	case "GEO":
		return "brand,region"
	default:
		return "-"
	}
}

func billingPaymentStatus(status string) string {
	switch strings.ToUpper(status) {
	case "PAID", "SUCCEEDED", "SUCCESS", "SETTLED":
		return "succeeded"
	case "FAILED", "REJECTED", "OVERDUE":
		return "failed"
	default:
		return "pending"
	}
}

func billingCycle(durationDays int) string {
	if durationDays >= 360 {
		return "YEARLY"
	}
	if durationDays >= 28 {
		return "MONTHLY"
	}
	if durationDays > 0 {
		return strconv.Itoa(durationDays) + "_DAYS"
	}
	return "MONTHLY"
}

func monthlyAmount(amountCents int, durationDays int) int {
	if durationDays >= 360 {
		return amountCents / 12
	}
	return amountCents
}

func countRowsByStatus(rows []map[string]any, status string) int {
	total := 0
	for _, row := range rows {
		if strings.ToUpper(stringValue(row["status"])) == status {
			total++
		}
	}
	return total
}

func seedAdminData() adminPlatformData {
	return withAdminDefaults(adminPlatformData{})
}

func withAdminDefaults(data adminPlatformData) adminPlatformData {
	if len(data.Users) == 0 {
		data.Users = []adminUser{
			{ID: "user_000001", Email: "admin@xianzhi.ai", Name: "平台管理员", Role: "SUPER_ADMIN", MemberLevel: memberLevelEnterprise, AgentStatus: agentStatusNone, OperationCenterStatus: operationStatusNone, Status: "ACTIVE", PlanID: "plan_free"},
			{ID: "user_000002", Email: "demo@xianzhi.ai", Name: "演示用户", Role: "MEMBER", MemberLevel: memberLevelBasic, AgentStatus: agentStatusNone, OperationCenterStatus: operationStatusNone, Status: "ACTIVE", PlanID: "plan_month"},
			{ID: "user_000003", Email: "agent1@xianzhi.ai", Name: "华东推广员", Role: "AGENT_L1", MemberLevel: memberLevelFree, AgentStatus: agentStatusActive, OperationCenterStatus: operationStatusNone, Status: "ACTIVE", PlanID: "plan_free"},
			{ID: "user_000004", Email: "operation@xianzhi.ai", Name: "华东运营中心", Role: "OPERATION_CENTER", MemberLevel: memberLevelFree, AgentStatus: agentStatusNone, OperationCenterStatus: operationStatusActive, Status: "ACTIVE", PlanID: "plan_free"},
			{ID: "user_000010", Email: "demo2@xianzhi.ai", Name: "demo2", Role: "MEMBER", MemberLevel: memberLevelFree, AgentStatus: agentStatusNone, OperationCenterStatus: operationStatusNone, Status: "ACTIVE", PlanID: "plan_free"},
		}
	}
	if len(data.Plans) == 0 {
		data.Plans = canonicalBillingPlans()
	} else {
		data.Plans = mergeCanonicalPlans(data.Plans)
	}
	if len(data.PointAccounts) == 0 {
		data.PointAccounts = []adminPointAccount{
			{ID: "points_000001", UserID: "user_000001", Available: 100000},
			{ID: "points_000002", UserID: "user_000002", Available: 0},
			{ID: "points_000003", UserID: "user_000003", Available: 5000},
			{ID: "points_000010", UserID: "user_000010", Available: 100},
		}
	}
	if len(data.ChannelAgents) == 0 {
		data.ChannelAgents = []adminChannelAgent{{ID: "channel_000001", UserID: "user_000003", OperationCenterID: "operation_center_000001", Level: 1, Status: "ACTIVE", InviteCode: "EAST001"}}
	}
	if len(data.OperationCenters) == 0 {
		data.OperationCenters = []adminOperationCenter{{ID: "operation_center_000001", UserID: "user_000004", Name: "华东运营中心", Region: "华东", InviteCode: "OC-EAST", Status: "ACTIVE", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}}
	}
	if len(data.Commissions) == 0 {
		data.Commissions = []adminCommission{{ID: "commission_000001", OrderID: "order_000001", AgentID: "channel_000001", AmountCents: 990, Rate: 0.1, Status: "SETTLED"}}
	}
	if len(data.CommissionRules) == 0 {
		data.CommissionRules = defaultCommissionRules()
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
	data = applyPublishedBillingRulesV1(normalizeAICapabilityDefaults(data))
	return data
}

func defaultCommissionRules() []adminCommissionRule {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return []adminCommissionRule{
		{ID: "rule_membership_l1_direct", Name: "L1 推广员会员套餐返佣", OrderType: "PLAN_ORDER", EarnerRole: "AGENT_L1", RelationDepth: 1, Rate: 0.15, MaxTotalRate: 0.2, Status: "ACTIVE", Metadata: map[string]any{"level": 1, "range": "10%-20%", "policy": "会员套餐返佣"}, CreatedAt: now, UpdatedAt: now},
		{ID: "rule_recharge_l1_direct", Name: "L1 推广员点数包返佣", OrderType: "COMPUTE_RECHARGE", EarnerRole: "AGENT_L1", RelationDepth: 1, Rate: 0.08, MaxTotalRate: 0.1, Status: "ACTIVE", Metadata: map[string]any{"level": 1, "range": "5%-10%", "policy": "点数包返佣"}, CreatedAt: now, UpdatedAt: now},
		{ID: "rule_membership_l2_direct", Name: "L2 初级代理会员套餐返佣", OrderType: "PLAN_ORDER", EarnerRole: "AGENT_L2", RelationDepth: 1, Rate: 0.25, MaxTotalRate: 0.3, Status: "ACTIVE", Metadata: map[string]any{"level": 2, "range": "20%-30%", "policy": "会员套餐返佣"}, CreatedAt: now, UpdatedAt: now},
		{ID: "rule_recharge_l2_direct", Name: "L2 初级代理点数充值返佣", OrderType: "COMPUTE_RECHARGE", EarnerRole: "AGENT_L2", RelationDepth: 1, Rate: 0.12, MaxTotalRate: 0.15, Status: "ACTIVE", Metadata: map[string]any{"level": 2, "range": "10%-15%", "policy": "点数充值返佣"}, CreatedAt: now, UpdatedAt: now},
		{ID: "rule_enterprise_l2_direct", Name: "L2 初级代理企业项目返佣", OrderType: "ENTERPRISE_PROJECT", EarnerRole: "AGENT_L2", RelationDepth: 1, Rate: 0.15, MaxTotalRate: 0.2, Status: "ACTIVE", Metadata: map[string]any{"level": 2, "range": "10%-20%", "policy": "企业项目返佣"}, CreatedAt: now, UpdatedAt: now},
		{ID: "rule_membership_l3_direct", Name: "L3 高级代理会员套餐返佣", OrderType: "PLAN_ORDER", EarnerRole: "AGENT_L3", RelationDepth: 1, Rate: 0.35, MaxTotalRate: 0.4, Status: "ACTIVE", Metadata: map[string]any{"level": 3, "range": "30%-40%", "policy": "会员套餐返佣"}, CreatedAt: now, UpdatedAt: now},
		{ID: "rule_recharge_l3_direct", Name: "L3 高级代理点数充值返佣", OrderType: "COMPUTE_RECHARGE", EarnerRole: "AGENT_L3", RelationDepth: 1, Rate: 0.2, MaxTotalRate: 0.25, Status: "ACTIVE", Metadata: map[string]any{"level": 3, "range": "15%-25%", "policy": "点数充值返佣"}, CreatedAt: now, UpdatedAt: now},
		{ID: "rule_enterprise_l3_direct", Name: "L3 高级代理企业版项目返佣", OrderType: "ENTERPRISE_PROJECT", EarnerRole: "AGENT_L3", RelationDepth: 1, Rate: 0.25, MaxTotalRate: 0.3, Status: "ACTIVE", Metadata: map[string]any{"level": 3, "range": "20%-30%", "policy": "企业版项目返佣"}, CreatedAt: now, UpdatedAt: now},
		{ID: "rule_membership_l3_diff_from_l2", Name: "L3 上级代理会员套餐差额分润", OrderType: "PLAN_ORDER", EarnerRole: "AGENT_L3", RelationDepth: 2, Rate: 0.10, MaxTotalRate: 0.35, Status: "ACTIVE", Metadata: map[string]any{"level": 3, "range": "L3-L2 差额 10%", "policy": "团队差额分润"}, CreatedAt: now, UpdatedAt: now},
		{ID: "rule_recharge_l3_diff_from_l2", Name: "L3 上级代理点数充值差额分润", OrderType: "COMPUTE_RECHARGE", EarnerRole: "AGENT_L3", RelationDepth: 2, Rate: 0.08, MaxTotalRate: 0.20, Status: "ACTIVE", Metadata: map[string]any{"level": 3, "range": "L3-L2 差额 8%", "policy": "团队差额分润"}, CreatedAt: now, UpdatedAt: now},
		{ID: "rule_enterprise_l3_diff_from_l2", Name: "L3 上级代理企业项目差额分润", OrderType: "ENTERPRISE_PROJECT", EarnerRole: "AGENT_L3", RelationDepth: 2, Rate: 0.10, MaxTotalRate: 0.25, Status: "ACTIVE", Metadata: map[string]any{"level": 3, "range": "L3-L2 差额 10%", "policy": "团队差额分润"}, CreatedAt: now, UpdatedAt: now},
		{ID: "rule_membership_l4_direct", Name: "L4 城市合伙人会员套餐返佣", OrderType: "PLAN_ORDER", EarnerRole: "AGENT_L4", RelationDepth: 1, Rate: 0.4, MaxTotalRate: 0.4, Status: "ACTIVE", Metadata: map[string]any{"level": 4, "range": "40%左右", "policy": "会员套餐返佣", "manualReview": true}, CreatedAt: now, UpdatedAt: now},
		{ID: "rule_recharge_l4_direct", Name: "L4 城市合伙人点数充值返佣", OrderType: "COMPUTE_RECHARGE", EarnerRole: "AGENT_L4", RelationDepth: 1, Rate: 0.25, MaxTotalRate: 0.3, Status: "ACTIVE", Metadata: map[string]any{"level": 4, "range": "20%-30%", "policy": "点数充值返佣", "manualReview": true}, CreatedAt: now, UpdatedAt: now},
		{ID: "rule_enterprise_l4_direct", Name: "L4 城市合伙人企业项目返佣", OrderType: "ENTERPRISE_PROJECT", EarnerRole: "AGENT_L4", RelationDepth: 1, Rate: 0.3, MaxTotalRate: 0.3, Status: "ACTIVE", Metadata: map[string]any{"level": 4, "range": "30%左右", "policy": "企业项目返佣", "manualReview": true}, CreatedAt: now, UpdatedAt: now},
		{ID: "rule_region_l4_team", Name: "L4 城市合伙人区域团队业绩返点", OrderType: "TEAM_PERFORMANCE", EarnerRole: "AGENT_L4", RelationDepth: 2, Rate: 0.08, MaxTotalRate: 0.1, Status: "ACTIVE", Metadata: map[string]any{"level": 4, "range": "5%-10%", "policy": "区域团队业绩返点", "manualReview": true}, CreatedAt: now, UpdatedAt: now},
		{ID: "rule_l5_strategic_contract", Name: "L5 联合运营商战略合作政策", OrderType: "STRATEGIC_CONTRACT", EarnerRole: "AGENT_L5", RelationDepth: 1, Rate: 0, MaxTotalRate: 0, Status: "MANUAL_REVIEW", Metadata: map[string]any{"level": 5, "policy": "独立 SaaS、独立价格和专属额度池，按单独合同执行", "manualReview": true}, CreatedAt: now, UpdatedAt: now},
	}
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
			ID: "channel_newapi_gateway", Name: "NewAPI Gateway", BaseURL: "https://newapi.zs-kjhn.cn", Protocol: "openai",
			ImageRequestMode: "openai", ImageGenerationEndpoint: "/v1/images/generations", ImageEditEndpoint: "/v1/images/edits",
			VideoGenerationEndpoint: "/v1/video/generations",
			FetchModelsPath:         "/models",
			Notes:                   "NewAPI unified gateway - all models routed through zhiqiyun-ai token (vip group)",
			Status:                  "CONFIGURABLE", Priority: 5, Models: []string{"doubao-seedance-2.0", "seedance-fast-2.0", "gpt-image-2", "grok-imagine-1.5-video", "grok-imagine-video-1.5-preview"},
		},
		{
			ID: "channel_apimart", Name: "APIMart 生图聚合", BaseURL: "https://api.apimart.ai", Protocol: "apimart",
			ImageRequestMode: "openai-json", ImageGenerationEndpoint: "/v1/images/generations", ImageEditEndpoint: "/v1/images/edits",
			FetchModelsPath: "/v1/models", APIKeyEnv: "APIMART_API_KEY", Notes: "参考 Infinite-Canvas 推荐平台，适合聚合图片、视频和 LLM 模型。",
			Status: "CONFIGURABLE", Priority: 10, Models: []string{"gpt-image-2", "nano-banana-edit", "veo3.1-fast"},
		},
		{
			ID: "channel_cmecloud_seedance", Name: "CMECloud Doubao Video", BaseURL: "https://zhenze-huhehaote.cmecloud.cn/api/v3", Protocol: "openai",
			ImageRequestMode: "openai", ImageGenerationEndpoint: "/v1/images/generations", ImageEditEndpoint: "/v1/images/edits",
			VideoGenerationEndpoint: "contents/generations/tasks",
			FetchModelsPath:         "/models", APIKeyEnv: "CME_CLOUD_API_KEY", Notes: "Doubao Seedance 2.0 video generation channel. Save the API Key in admin API keys or set CME_CLOUD_API_KEY.",
			Status: "CONFIGURABLE", Priority: 15, Models: []string{"doubao-seedance-2.0", "seedance-fast-2.0"},
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
			Name:                    "褰撳墠杩愯涓婃父",
			BaseURL:                 baseURL,
			Protocol:                "openai",
			ImageRequestMode:        "openai",
			ImageGenerationEndpoint: "/v1/images/generations",
			ImageEditEndpoint:       "/v1/images/edits",
			VideoGenerationEndpoint: "",
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
		{ID: "model_mock_standard", Model: "mock-standard", Name: "鏈湴婕旂ず妯″瀷", Capability: "TEXT_TO_IMAGE", BillingMode: "PER_REQUEST", FixedQuota: 1, ModelRatio: 1, CompletionRatio: 1, Status: "ACTIVE"},
		{ID: "model_gpt_image_2", Model: model, Name: "褰撳墠鍥惧儚妯″瀷", Capability: "IMAGE", BillingMode: "PER_REQUEST", FixedQuota: 10, ModelRatio: 1, CompletionRatio: 1, Status: "ACTIVE"},
		{ID: "model_agent_chat", Model: "agent-chat", Name: "Agent 瀵硅瘽", Capability: "CHAT", BillingMode: "TOKEN", ModelRatio: 1, CompletionRatio: 2, Status: "ACTIVE"},
	}
}

func defaultAPIKeys(data adminPlatformData) []adminAPIKey {
	items := []adminAPIKey{}
	for _, user := range data.Users {
		if user.Role == "MEMBER" || userHasActiveChannelProfile(data, user.ID) {
			items = append(items, adminAPIKey{ID: "key_" + user.ID, Customer: user.Name, Prefix: "sk-" + shortID(user.ID), Status: user.Status, Models: []string{"mock-standard", "gpt-image-2"}, QuotaLimit: 100000})
		}
	}
	return items
}

func defaultCustomerGroups() []adminCustomerGroup {
	return []adminCustomerGroup{
		{ID: "group_default", Name: "default", Ratio: 1, Models: []string{"mock-standard"}, Description: "榛樿瀹㈡埛鍒嗙粍"},
		{ID: "group_vip", Name: "vip", Ratio: 0.8, Models: []string{"mock-standard", "gpt-image-2", "agent-chat"}, Description: "浼佷笟瀹㈡埛浼樻儬鍊嶇巼"},
		{ID: "group_channel", Name: "channel_a", Ratio: 0.9, Models: []string{"mock-standard", "gpt-image-2"}, Description: "浠ｇ悊娓犻亾瀹㈡埛"},
	}
}

func usageRows(data adminPlatformData, productFilter string) []map[string]any {
	productFilter = strings.ToLower(strings.TrimSpace(productFilter))
	imageUsage := len(data.GenerationTasks)
	imageCostCents := 0
	if len(data.BillingEvents) > 0 {
		imageUsage = 0
		for _, event := range data.BillingEvents {
			if event.MetricCode == "image.generations" && strings.ToUpper(event.Status) == "SUCCEEDED" {
				imageUsage += event.Quantity
				imageCostCents += event.AmountCents
			}
		}
	} else {
		imageCostCents = len(data.GenerationTasks) * 12
	}
	rows := []map[string]any{
		{"product": "API/文生图", "metric": "生成任务", "usage": imageUsage, "costCents": imageCostCents},
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

func commissionRecordSummary(items []adminCommission) map[string]any {
	total := 0
	agentTotal := 0
	operationCenterTotal := 0
	platformTotal := 0
	pending := 0
	for _, item := range items {
		total += item.AmountCents
		switch item.ReceiverType {
		case receiverTypeOperationCenter:
			operationCenterTotal += item.AmountCents
		case receiverTypePlatform:
			platformTotal += item.AmountCents
		default:
			agentTotal += item.AmountCents
		}
		if !strings.EqualFold(item.SettleStatus, "SETTLED") && !strings.EqualFold(item.Status, "SETTLED") && !strings.EqualFold(item.Status, "APPROVED") {
			pending += item.AmountCents
		}
	}
	return map[string]any{
		"totalCents":           total,
		"agentCents":           agentTotal,
		"operationCenterCents": operationCenterTotal,
		"platformCents":        platformTotal,
		"pendingCents":         pending,
		"records":              len(items),
	}
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
	policy := agentLevelPolicyByLevel(agent.Level)
	return map[string]any{
		"id": agent.ID, "userId": agent.UserID, "name": user.Name, "email": user.Email, "level": agent.Level,
		"levelCode": policy.Code, "levelName": policy.Name, "levelLabel": agentLevelLabel(agent.Level), "identity": policy.Identity,
		"parentId": agent.ParentID, "operationCenterId": agent.OperationCenterID, "status": agent.Status, "inviteCode": agent.InviteCode,
		"joinOrderId": agent.JoinOrderID, "joinFeeCents": agent.JoinFeeCents, "tokenRightsAmount": agent.TokenRightsAmount, "createdAt": agent.CreatedAt,
		"openMethod": policy.OpenMethod, "openCondition": policy.OpenCondition, "keepCondition": policy.KeepCondition,
		"membershipCommission": policy.MembershipCommission, "rechargeCommission": policy.RechargeCommission, "enterpriseCommission": policy.EnterpriseCommission,
		"regionalRebate": policy.RegionalRebate, "permissions": policy.Permissions, "limitations": policy.Limitations, "manualReview": policy.ManualReview,
	}
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

func aiModelIDs(items []adminAIModel) map[string]bool {
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

func billingEventIDs(items []adminBillingEvent) map[string]bool {
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

func safeID(value string) string {
	builder := strings.Builder{}
	for _, ch := range strings.ToLower(value) {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
			builder.WriteRune(ch)
			continue
		}
		builder.WriteByte('_')
	}
	return strings.Trim(builder.String(), "_")
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
