package httpserver

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/skip2/go-qrcode"

	"xianzhi-ai/backend-go/internal/config"
)

const (
	defaultPromotionTemplateID = "poster.brand.simple"
	defaultPromotionPage       = "pages/promotion/PromotionLandingPage"
	maxPromotionPageSize       = 100
)

type promotionAPI struct {
	store     platformStore
	sessions  authSessionStore
	rbac      *userRBACAPI
	miniCode  *wechatMiniProgramCodeService
	codeMu    sync.Mutex
	codeCache map[string]promotionCodeCacheEntry
}

type promotionCodeCacheEntry struct {
	Response  promotionCodeResponse
	ExpiresAt time.Time
}

type promotionCodeResponse struct {
	ImageDataURL  string `json:"imageDataUrl"`
	Scene         string `json:"scene"`
	Page          string `json:"page"`
	IsPlaceholder bool   `json:"isPlaceholder"`
	CacheKey      string `json:"cacheKey"`
	ExpiresAt     string `json:"expiresAt"`
}

type promotionContext struct {
	Access     userRoleAccess
	Data       adminPlatformData
	User       adminUser
	InviteCode string
	RoleLabel  string
}

func newPromotionAPI(store platformStore, sessions authSessionStore, rbac *userRBACAPI, cfg config.Config) *promotionAPI {
	return &promotionAPI{
		store: store, sessions: sessions, rbac: rbac,
		miniCode: newWechatMiniProgramCodeService(cfg), codeCache: map[string]promotionCodeCacheEntry{},
	}
}

func (a *promotionAPI) profile(w http.ResponseWriter, r *http.Request) {
	ctx, err := a.currentContext(r)
	if err != nil {
		writePromotionError(w, err)
		return
	}
	records, err := a.recordsForContext(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, a.profilePayload(ctx, records))
}

func (a *promotionAPI) overview(w http.ResponseWriter, r *http.Request) {
	ctx, err := a.currentContext(r)
	if err != nil {
		writePromotionError(w, err)
		return
	}
	records, err := a.recordsForContext(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	items := promotionTemplatesForRole(ctx.Access.CurrentRole)
	featured := items
	if len(featured) > 4 {
		featured = featured[:4]
	}
	writeJSON(w, map[string]any{
		"profile": a.profilePayload(ctx, records), "summary": promotionSummary(records),
		"featuredTemplates": featured, "defaultTemplateId": defaultPromotionTemplateID,
		"activity": activePromotionActivity(),
	})
}

func (a *promotionAPI) templates(w http.ResponseWriter, r *http.Request) {
	ctx, err := a.currentContext(r)
	if err != nil {
		writePromotionError(w, err)
		return
	}
	items := promotionTemplatesForRole(ctx.Access.CurrentRole)
	writeJSON(w, map[string]any{
		"items": items, "total": len(items), "defaultTemplateId": defaultPromotionTemplateID,
		"categories": []map[string]string{
			{"id": "all", "name": "全部"}, {"id": "brand", "name": "品牌"}, {"id": "product", "name": "产品"},
			{"id": "invite", "name": "邀新"}, {"id": "industry", "name": "行业"}, {"id": "campaign", "name": "活动"},
		},
	})
}

func (a *promotionAPI) activities(w http.ResponseWriter, r *http.Request) {
	if _, err := a.currentContext(r); err != nil {
		writePromotionError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": []promotionActivity{activePromotionActivity()}})
}

func (a *promotionAPI) miniProgramCode(w http.ResponseWriter, r *http.Request) {
	ctx, err := a.currentContext(r)
	if err != nil {
		writePromotionError(w, err)
		return
	}
	var req struct {
		TemplateID string `json:"templateId"`
		ActivityID string `json:"activityId"`
		Page       string `json:"page"`
		Invalidate bool   `json:"invalidate"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.TemplateID = firstNonEmptyString(strings.TrimSpace(req.TemplateID), defaultPromotionTemplateID)
	if !promotionTemplateAllowed(req.TemplateID, ctx.Access.CurrentRole) {
		writeError(w, http.StatusForbidden, errors.New("template is unavailable for current role"))
		return
	}
	req.Page = firstNonEmptyString(strings.TrimSpace(req.Page), defaultPromotionPage)
	if !validPromotionPage(req.Page) {
		writeError(w, http.StatusBadRequest, errors.New("invalid mini program page"))
		return
	}
	cacheKey := strings.Join([]string{ctx.Access.UserID, ctx.Access.TenantID, ctx.Access.CurrentRole, ctx.InviteCode, req.TemplateID, req.ActivityID}, "|")
	if !req.Invalidate {
		if cached, ok := a.cachedCode(cacheKey); ok {
			writeJSON(w, cached)
			return
		}
	}
	scene := promotionScene(ctx.InviteCode, req.TemplateID, req.ActivityID)
	png, placeholder, err := a.miniCode.Generate(scene, req.Page)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	expiresAt := time.Now().UTC().Add(6 * time.Hour)
	response := promotionCodeResponse{
		ImageDataURL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), Scene: scene, Page: req.Page,
		IsPlaceholder: placeholder, CacheKey: shortStableHash(cacheKey, 20), ExpiresAt: expiresAt.Format(time.RFC3339),
	}
	a.storeCode(cacheKey, response, expiresAt)
	writeJSON(w, response)
}

func (a *promotionAPI) renderConfig(w http.ResponseWriter, r *http.Request) {
	ctx, err := a.currentContext(r)
	if err != nil {
		writePromotionError(w, err)
		return
	}
	var req struct {
		TemplateID string `json:"templateId"`
		ActivityID string `json:"activityId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.TemplateID = firstNonEmptyString(strings.TrimSpace(req.TemplateID), defaultPromotionTemplateID)
	if !promotionTemplateAllowed(req.TemplateID, ctx.Access.CurrentRole) {
		writeError(w, http.StatusForbidden, errors.New("template is unavailable for current role"))
		return
	}
	template, _ := promotionTemplateByID(req.TemplateID)
	writeJSON(w, map[string]any{
		"width": 1080, "height": 1440, "format": "png", "template": template,
		"profile": a.profilePayload(ctx, nil), "activity": promotionActivityByID(req.ActivityID),
	})
}

func (a *promotionAPI) records(w http.ResponseWriter, r *http.Request) {
	ctx, err := a.currentContext(r)
	if err != nil {
		writePromotionError(w, err)
		return
	}
	items, err := a.recordsForContext(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	status := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status")))
	if status != "" && status != "all" {
		filtered := make([]promotionRecord, 0, len(items))
		for _, item := range items {
			if strings.EqualFold(item.Status, status) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	page := positiveQueryInt(r, "page", 1)
	pageSize := positiveQueryInt(r, "pageSize", 20)
	if pageSize > maxPromotionPageSize {
		pageSize = maxPromotionPageSize
	}
	total := len(items)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	writeJSON(w, map[string]any{"items": items[start:end], "total": total, "page": page, "pageSize": pageSize, "hasMore": end < total})
}

func (a *promotionAPI) analytics(w http.ResponseWriter, r *http.Request) {
	ctx, err := a.currentContext(r)
	if err != nil {
		writePromotionError(w, err)
		return
	}
	items, err := a.recordsForContext(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	days := positiveQueryInt(r, "days", 7)
	if days > 90 {
		days = 90
	}
	writeJSON(w, promotionAnalytics(items, days))
}

func (a *promotionAPI) shareCopy(w http.ResponseWriter, r *http.Request) {
	ctx, err := a.currentContext(r)
	if err != nil {
		writePromotionError(w, err)
		return
	}
	templateID := firstNonEmptyString(strings.TrimSpace(r.URL.Query().Get("templateId")), defaultPromotionTemplateID)
	if !promotionTemplateAllowed(templateID, ctx.Access.CurrentRole) {
		writeError(w, http.StatusForbidden, errors.New("template is unavailable for current role"))
		return
	}
	template, _ := promotionTemplateByID(templateID)
	writeJSON(w, map[string]any{
		"title": template.Title, "description": template.Subtitle,
		"text": fmt.Sprintf("%s 邀请你体验知启云AI，微信扫码即可开始。邀请码：%s", ctx.User.Name, ctx.InviteCode),
		"path": "/" + defaultPromotionPage + "?invite=" + url.QueryEscape(ctx.InviteCode) + "&templateId=" + url.QueryEscape(templateID),
	})
}

func (a *promotionAPI) visit(w http.ResponseWriter, r *http.Request) {
	ctx, err := a.currentContext(r)
	if err != nil {
		writePromotionError(w, err)
		return
	}
	var req struct {
		InviteCode string `json:"inviteCode"`
		Source     string `json:"source"`
		TemplateID string `json:"templateId"`
		ActivityID string `json:"activityId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	inviter, tenantID, _, err := promotionInviterByCode(ctx.Data, req.InviteCode)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if inviter.ID == ctx.User.ID {
		writeError(w, http.StatusConflict, errPromotionSelfInvite)
		return
	}
	store, ok := a.store.(promotionDataStore)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, errors.New("promotion persistence is unavailable"))
		return
	}
	now := time.Now().UTC()
	dayKey := now.Format("2006-01-02")
	item, err := store.RecordPromotionVisit(promotionVisitInput{
		ID:       "promotion_visit_" + shortStableHash(inviter.ID+"|"+ctx.User.ID+"|"+dayKey+"|"+req.TemplateID+"|"+req.ActivityID, 24),
		TenantID: tenantID, InviterUserID: inviter.ID, VisitorID: ctx.User.ID, VisitorName: ctx.User.Name,
		MaskedMobile: maskPromotionAccount(ctx.User.Email), InviteCode: strings.ToUpper(strings.TrimSpace(req.InviteCode)),
		Source: normalizePromotionSource(req.Source), TemplateID: firstNonEmptyString(req.TemplateID, defaultPromotionTemplateID),
		ActivityID: strings.TrimSpace(req.ActivityID), VisitedAt: now,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{"item": item, "bound": false})
}

func (a *promotionAPI) bind(w http.ResponseWriter, r *http.Request) {
	ctx, err := a.currentContext(r)
	if err != nil {
		writePromotionError(w, err)
		return
	}
	var req struct {
		InviteCode string `json:"inviteCode"`
		Source     string `json:"source"`
		TemplateID string `json:"templateId"`
		ActivityID string `json:"activityId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	inviter, tenantID, _, err := promotionInviterByCode(ctx.Data, req.InviteCode)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	store, ok := a.store.(promotionDataStore)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, errors.New("promotion persistence is unavailable"))
		return
	}
	item, err := store.BindPromotionInvite(promotionBindInput{
		TenantID: tenantID, InviterUserID: inviter.ID, InviteeUserID: ctx.User.ID,
		InviteCode: strings.ToUpper(strings.TrimSpace(req.InviteCode)), Source: normalizePromotionSource(req.Source),
		TemplateID: firstNonEmptyString(req.TemplateID, defaultPromotionTemplateID), ActivityID: strings.TrimSpace(req.ActivityID), BoundAt: time.Now().UTC(),
	})
	if err != nil {
		switch {
		case errors.Is(err, errPromotionSelfInvite), errors.Is(err, errPromotionInviteCycle), errors.Is(err, errPromotionInviteAlreadyBound):
			writeError(w, http.StatusConflict, err)
		default:
			writeError(w, http.StatusBadRequest, err)
		}
		return
	}
	writeJSON(w, map[string]any{"item": item, "bound": true})
}

func (a *promotionAPI) currentContext(r *http.Request) (promotionContext, error) {
	access, err := a.rbac.accessForRequest(r)
	if err != nil {
		return promotionContext{}, err
	}
	data, err := a.store.AdminData()
	if err != nil {
		return promotionContext{}, err
	}
	user, ok := userMap(data.Users)[access.UserID]
	if !ok {
		_, user, err = a.rbac.authenticatedUser(r)
		if err != nil {
			return promotionContext{}, err
		}
	}
	access.TenantID = firstNonEmptyString(user.TenantID, access.TenantID, "tenant_default")
	access.OrganizationID = firstNonEmptyString(user.OrganizationID, access.OrganizationID, "organization_default")
	code := promotionInviteCode(data, user, access.CurrentRole)
	return promotionContext{Access: access, Data: data, User: user, InviteCode: code, RoleLabel: promotionRoleLabel(access.CurrentRole)}, nil
}

func (a *promotionAPI) profilePayload(ctx promotionContext, records []promotionRecord) map[string]any {
	name := firstNonEmptyString(strings.TrimSpace(ctx.User.Name), "知启云用户")
	brandName := firstNonEmptyString(strings.TrimSpace(ctx.Data.SystemSettings.Brand.Name), "知启云AI")
	return map[string]any{
		"userId": ctx.Access.UserID, "tenantId": ctx.Access.TenantID, "organizationId": ctx.Access.OrganizationID,
		"name": name, "avatarUrl": "", "companyName": brandName, "currentRole": ctx.Access.CurrentRole,
		"roleLabel": ctx.RoleLabel, "roles": ctx.Access.Roles, "inviteCode": ctx.InviteCode,
		"summary": promotionSummary(records),
	}
}

func (a *promotionAPI) recordsForContext(ctx promotionContext) ([]promotionRecord, error) {
	items := []promotionRecord{}
	if store, ok := a.store.(promotionDataStore); ok {
		stored, err := store.ListPromotionRecords(ctx.Access.UserID, ctx.Access.TenantID)
		if err != nil {
			return nil, err
		}
		items = append(items, stored...)
	}
	seenInvitees := map[string]bool{}
	for _, item := range items {
		if item.InviteeUserID != "" {
			seenInvitees[item.InviteeUserID] = true
		}
	}
	for _, user := range ctx.Data.Users {
		if user.ReferredBy != ctx.Access.UserID || seenInvitees[user.ID] {
			continue
		}
		status, paidAt := promotionUserStatus(ctx.Data.Orders, user)
		createdAt := firstNonEmptyString(user.CreatedAt, time.Now().UTC().Format(time.RFC3339))
		items = append(items, promotionRecord{
			ID: "promotion_legacy_" + shortStableHash(user.ID, 20), TenantID: ctx.Access.TenantID,
			InviterUserID: ctx.Access.UserID, InviteeUserID: user.ID, VisitorID: user.ID, VisitorName: user.Name,
			MaskedMobile: maskPromotionAccount(user.Email), InviteCode: ctx.InviteCode, Status: status, Source: "invite_code",
			TemplateID: defaultPromotionTemplateID, VisitTime: createdAt, RegisterTime: createdAt, PaidTime: paidAt,
			RewardStatus: promotionRewardStatus(status), CreatedAt: createdAt, UpdatedAt: firstNonEmptyString(user.UpdatedAt, createdAt),
		})
	}
	enrichPromotionRecords(items, ctx.Data, ctx.User)
	sortPromotionRecords(items)
	return items, nil
}

func promotionInviteCode(data adminPlatformData, user adminUser, currentRole string) string {
	switch currentRole {
	case roleAgent:
		if agent, ok := channelAgentForUser(data.ChannelAgents, user.ID); ok && strings.TrimSpace(agent.InviteCode) != "" {
			return strings.ToUpper(strings.TrimSpace(agent.InviteCode))
		}
	case roleOperation:
		if center, ok := activeOperationCenterForUser(data.OperationCenters, user.ID); ok && strings.TrimSpace(center.InviteCode) != "" {
			return strings.ToUpper(strings.TrimSpace(center.InviteCode))
		}
	}
	return "ZQ" + strings.ToUpper(shortStableHash(firstNonEmptyString(user.TenantID, "tenant_default")+"|"+user.ID, 8))
}

func promotionInviterByCode(data adminPlatformData, rawCode string) (adminUser, string, string, error) {
	code := strings.ToUpper(strings.TrimSpace(rawCode))
	codeKey := promotionCodeKey(code)
	if code == "" {
		return adminUser{}, "", "", errors.New("inviteCode is required")
	}
	users := userMap(data.Users)
	for _, agent := range data.ChannelAgents {
		if promotionCodeKey(agent.InviteCode) == codeKey && strings.EqualFold(agent.Status, "ACTIVE") {
			if user, ok := users[agent.UserID]; ok {
				return user, firstNonEmptyString(user.TenantID, "tenant_default"), roleAgent, nil
			}
		}
	}
	for _, center := range data.OperationCenters {
		if promotionCodeKey(center.InviteCode) == codeKey && strings.EqualFold(center.Status, "ACTIVE") {
			if user, ok := users[center.UserID]; ok {
				return user, firstNonEmptyString(user.TenantID, "tenant_default"), roleOperation, nil
			}
		}
	}
	for _, user := range data.Users {
		if promotionCodeKey(promotionInviteCode(data, user, roleUser)) == codeKey {
			return user, firstNonEmptyString(user.TenantID, "tenant_default"), roleUser, nil
		}
	}
	return adminUser{}, "", "", errors.New("invite code is invalid or expired")
}

func promotionSummary(items []promotionRecord) map[string]any {
	var visits, registered, paid int
	var reward int64
	for _, item := range items {
		visits++
		switch strings.ToLower(item.Status) {
		case promotionStatusPaid:
			paid++
			registered++
		case promotionStatusRegistered:
			registered++
		}
		reward += item.RewardAmountCents
	}
	registerRate := 0.0
	paidRate := 0.0
	if visits > 0 {
		registerRate = float64(registered) * 100 / float64(visits)
	}
	if registered > 0 {
		paidRate = float64(paid) * 100 / float64(registered)
	}
	return map[string]any{"visitCount": visits, "registerCount": registered, "paidCount": paid, "rewardAmountCents": reward, "registerRate": registerRate, "paidRate": paidRate}
}

func promotionAnalytics(items []promotionRecord, days int) map[string]any {
	now := time.Now().UTC()
	trend := make([]map[string]any, 0, days)
	channels := map[string]int{}
	for offset := days - 1; offset >= 0; offset-- {
		day := now.AddDate(0, 0, -offset).Format("2006-01-02")
		visits, registered, paid := 0, 0, 0
		for _, item := range items {
			if !strings.HasPrefix(firstNonEmptyString(item.VisitTime, item.CreatedAt), day) {
				continue
			}
			visits++
			if item.Status == promotionStatusRegistered || item.Status == promotionStatusPaid {
				registered++
			}
			if item.Status == promotionStatusPaid {
				paid++
			}
		}
		trend = append(trend, map[string]any{"date": day, "visitCount": visits, "registerCount": registered, "paidCount": paid})
	}
	for _, item := range items {
		channels[firstNonEmptyString(item.Source, "wechat_friend")]++
	}
	channelItems := make([]map[string]any, 0, len(channels))
	for source, count := range channels {
		channelItems = append(channelItems, map[string]any{"source": source, "label": promotionSourceLabel(source), "count": count})
	}
	sort.Slice(channelItems, func(i, j int) bool { return intValue(channelItems[i]["count"]) > intValue(channelItems[j]["count"]) })
	return map[string]any{"summary": promotionSummary(items), "trend": trend, "channels": channelItems, "days": days}
}

func enrichPromotionRecords(items []promotionRecord, data adminPlatformData, inviter adminUser) {
	agent, _ := channelAgentForUser(data.ChannelAgents, inviter.ID)
	operation, _ := activeOperationCenterForUser(data.OperationCenters, inviter.ID)
	orders := map[string]adminOrder{}
	for _, order := range data.Orders {
		orders[order.ID] = order
	}
	for i := range items {
		if items[i].InviteeUserID != "" {
			if user, ok := userMap(data.Users)[items[i].InviteeUserID]; ok {
				status, paidAt := promotionUserStatus(data.Orders, user)
				if status == promotionStatusPaid {
					items[i].Status = status
					items[i].PaidTime = paidAt
				}
			}
		}
		for _, commission := range data.Commissions {
			belongsToInviter := (agent.ID != "" && commission.AgentID == agent.ID) ||
				(operation.ID != "" && strings.EqualFold(commission.ReceiverType, "OPERATION_CENTER") && commission.ReceiverID == operation.ID)
			order := orders[commission.OrderID]
			belongsToInvitee := items[i].InviteeUserID != "" && (order.UserID == items[i].InviteeUserID || order.BuyerUserID == items[i].InviteeUserID)
			if belongsToInviter && belongsToInvitee {
				items[i].RewardAmountCents += int64(commission.AmountCents)
				items[i].RewardStatus = firstNonEmptyString(commission.SettleStatus, commission.Status)
			}
		}
	}
}

func promotionUserStatus(orders []adminOrder, user adminUser) (string, string) {
	for _, order := range orders {
		if (order.UserID == user.ID || order.BuyerUserID == user.ID) && isPaidStatus(order.Status) {
			return promotionStatusPaid, firstNonEmptyString(order.PaidAt, order.CreatedAt)
		}
	}
	return promotionStatusRegistered, ""
}

func promotionRewardStatus(status string) string {
	if status == promotionStatusPaid {
		return "PENDING"
	}
	return "LOCKED"
}

func promotionScene(inviteCode, templateID, activityID string) string {
	templateIndex := 1
	for i, item := range promotionTemplates() {
		if item.ID == templateID {
			templateIndex = i + 1
			break
		}
	}
	value := fmt.Sprintf("C%sT%02d", promotionCodeKey(inviteCode), templateIndex)
	if strings.TrimSpace(activityID) != "" {
		value += "A" + strings.ToUpper(shortStableHash(activityID, 6))
	}
	if len(value) > 32 {
		value = fmt.Sprintf("C%sT%02d", promotionCodeKey(inviteCode)[:20], templateIndex)
	}
	return value
}

func promotionCodeKey(value string) string {
	var builder strings.Builder
	for _, char := range strings.ToUpper(strings.TrimSpace(value)) {
		if (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') {
			builder.WriteRune(char)
		}
	}
	return builder.String()
}

func validPromotionPage(page string) bool {
	return page == defaultPromotionPage || page == "pages/promotion/PromotionCenterPage"
}

func promotionTemplateAllowed(id, currentRole string) bool {
	item, ok := promotionTemplateByID(id)
	if !ok {
		return false
	}
	return containsString(item.AllowedRoles, currentRole)
}

func promotionTemplateByID(id string) (promotionTemplate, bool) {
	for _, item := range promotionTemplates() {
		if item.ID == id {
			return item, true
		}
	}
	return promotionTemplate{}, false
}

func promotionTemplatesForRole(role string) []promotionTemplate {
	items := []promotionTemplate{}
	for _, item := range promotionTemplates() {
		if containsString(item.AllowedRoles, role) {
			items = append(items, item)
		}
	}
	return items
}

func promotionTemplates() []promotionTemplate {
	allRoles := []string{roleUser, roleAgent, roleOperation, roleEnterpriseAdmin, roleAIAdmin, roleFinance, roleCustomerService, roleEnterpriseMember}
	return []promotionTemplate{
		promotionTemplateItem("poster.brand.simple", "品牌极简", "brand", "品牌推广", "#F3F6FF", "#7D8DF6", "#5A4DB2", "知启云AI，让创意更高效", "AI创作、知识与团队协作一站完成", "品牌推荐", "brand-focus", []string{"AI智能创作", "企业级安全", "多端协同"}, allRoles),
		promotionTemplateItem("poster.product.features", "产品能力", "product", "产品推广", "#EEF2FF", "#5D5FEF", "#132A77", "一套平台，释放团队AI生产力", "从灵感到作品，流程更清晰", "核心能力", "feature-grid", []string{"图片与视频", "PPT与信息图", "AI员工"}, allRoles),
		promotionTemplateItem("poster.invite.reward", "邀请有礼", "invite", "邀新活动", "#FFF5EC", "#FF771B", "#B84A08", "邀请好友，一起开启AI创作", "完成注册即可体验丰富AI能力", "邀请有礼", "reward-card", []string{"扫码即达", "专属邀请码", "活动奖励"}, allRoles),
		promotionTemplateItem("poster.enterprise.brand", "企业品牌", "brand", "品牌推广", "#F0F3FA", "#5A4DB2", "#132A77", "企业级AI内容生产平台", "统一品牌资产，沉淀组织知识", "企业方案", "enterprise-split", []string{"组织权限", "品牌一致", "数据治理"}, allRoles),
		promotionTemplateItem("poster.scene.marketing", "营销场景", "product", "产品推广", "#F2F7FF", "#7D8DF6", "#325AA8", "让每一次营销都有AI助力", "海报、视频、朋友圈与PPT快速交付", "营销提效", "scene-stack", []string{"新品发布", "社媒传播", "销售物料"}, allRoles),
		promotionTemplateItem("poster.industry.solution", "行业方案", "industry", "行业方案", "#ECF7F6", "#3F9E91", "#185E55", "面向行业的AI解决方案", "连接知识、内容与业务场景", "行业精选", "industry-panel", []string{"场景适配", "知识增强", "安全可控"}, allRoles),
		promotionTemplateItem("poster.case.study", "客户案例", "industry", "行业方案", "#F8F4EE", "#B88A58", "#694523", "真实案例，见证AI业务价值", "用可复用的方法加速团队落地", "客户案例", "case-quote", []string{"效率提升", "成本优化", "持续增长"}, allRoles),
		promotionTemplateItem("poster.trial.limited", "限时体验", "campaign", "活动推广", "#FFF0E8", "#FF771B", "#9A3500", "限时开放，立即体验知启云AI", "把握体验机会，快速完成第一份作品", "限时体验", "campaign-countdown", []string{"快速上手", "多种模板", "随时创作"}, allRoles),
		promotionTemplateItem("poster.partner.recruit", "伙伴招募", "invite", "伙伴招募", "#F3F0FF", "#7D8DF6", "#5A4DB2", "携手知启云AI，共创增长", "加入推广伙伴，拓展企业AI市场", "伙伴招募", "partner-steps", []string{"推广支持", "客户管理", "分润透明"}, []string{roleAgent, roleOperation, roleEnterpriseAdmin}),
		promotionTemplateItem("poster.festival.campaign", "节日活动", "campaign", "活动推广", "#FFF3F4", "#E75D70", "#8F2436", "节日灵感，由AI即刻点亮", "定制节日内容，传递品牌心意", "节日限定", "festival-frame", []string{"节日海报", "品牌祝福", "社媒传播"}, allRoles),
	}
}

func promotionTemplateItem(id, name, category, categoryLabel, background, primary, secondary, title, subtitle, badge, layout string, features, roles []string) promotionTemplate {
	return promotionTemplate{
		ID: id, Name: name, Category: category, CategoryLabel: categoryLabel, AllowedRoles: append([]string{}, roles...),
		Background: background, PrimaryColor: primary, SecondaryColor: secondary, Title: title, Subtitle: subtitle,
		Badge: badge, Description: subtitle, FeatureItems: append([]string{}, features...), Layout: layout,
		QRPosition: map[string]int{"x": 708, "y": 1112, "size": 244}, InviterPosition: map[string]int{"x": 120, "y": 1160},
	}
}

func promotionRoleLabel(role string) string {
	switch role {
	case roleAgent:
		return "推广伙伴"
	case roleOperation:
		return "运营中心"
	case roleEnterpriseAdmin:
		return "企业管理员"
	default:
		return "普通用户"
	}
}

func activePromotionActivity() promotionActivity {
	return promotionActivity{ID: "activity.always-on", Name: "知启云AI体验季", Badge: "推荐", Description: "邀请好友体验企业级AI创作平台", Status: "ACTIVE"}
}

func promotionActivityByID(id string) promotionActivity {
	activity := activePromotionActivity()
	if strings.TrimSpace(id) == "" || id == activity.ID {
		return activity
	}
	return promotionActivity{ID: strings.TrimSpace(id), Name: "专属推广活动", Badge: "活动", Description: "通过专属推广码访问", Status: "ACTIVE"}
}

func normalizePromotionSource(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "wechat_friend", "wechat_group", "moments", "poster", "copy_link", "invite_code":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "wechat_friend"
	}
}

func promotionSourceLabel(source string) string {
	switch source {
	case "wechat_group":
		return "微信群"
	case "moments":
		return "朋友圈"
	case "poster":
		return "推广海报"
	case "copy_link":
		return "复制链接"
	case "invite_code":
		return "邀请码"
	default:
		return "微信好友"
	}
}

func maskPromotionAccount(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "微信用户"
	}
	if at := strings.Index(value, "@"); at > 2 {
		return value[:2] + "***" + value[at:]
	}
	if len(value) > 7 {
		return value[:3] + "****" + value[len(value)-4:]
	}
	return "***"
}

func positiveQueryInt(r *http.Request, key string, fallbackValue int) int {
	value, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get(key)))
	if err != nil || value <= 0 {
		return fallbackValue
	}
	return value
}

func writePromotionError(w http.ResponseWriter, err error) {
	if errors.Is(err, errUnauthorized) {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	if errors.Is(err, errForbidden) {
		writeError(w, http.StatusForbidden, err)
		return
	}
	writeError(w, http.StatusInternalServerError, err)
}

func (a *promotionAPI) cachedCode(key string) (promotionCodeResponse, bool) {
	a.codeMu.Lock()
	defer a.codeMu.Unlock()
	item, ok := a.codeCache[key]
	if !ok || time.Now().After(item.ExpiresAt) {
		delete(a.codeCache, key)
		return promotionCodeResponse{}, false
	}
	return item.Response, true
}

func (a *promotionAPI) storeCode(key string, response promotionCodeResponse, expiresAt time.Time) {
	a.codeMu.Lock()
	defer a.codeMu.Unlock()
	a.codeCache[key] = promotionCodeCacheEntry{Response: response, ExpiresAt: expiresAt}
}

type wechatMiniProgramCodeService struct {
	cfg       config.Config
	client    *http.Client
	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

func newWechatMiniProgramCodeService(cfg config.Config) *wechatMiniProgramCodeService {
	return &wechatMiniProgramCodeService{cfg: cfg, client: &http.Client{Timeout: 12 * time.Second}}
}

func (s *wechatMiniProgramCodeService) Generate(scene, page string) ([]byte, bool, error) {
	appID := strings.TrimSpace(os.Getenv("WECHAT_MINI_PROGRAM_APPID"))
	secret := strings.TrimSpace(os.Getenv("WECHAT_MINI_PROGRAM_SECRET"))
	if appID == "" || secret == "" {
		if s.cfg.IsProduction() {
			return nil, false, errors.New("official mini program code is unavailable: server WeChat credentials are missing")
		}
		png, err := qrcode.Encode("zhiqiyun-dev://promotion?"+scene, qrcode.Medium, 512)
		return png, true, err
	}
	token, err := s.accessToken(appID, secret)
	if err != nil {
		return nil, false, err
	}
	payload, _ := json.Marshal(map[string]any{"scene": scene, "page": page, "check_path": false, "env_version": promotionEnvVersion(), "width": 430})
	endpoint := "https://api.weixin.qq.com/wxa/getwxacodeunlimit?access_token=" + url.QueryEscape(token)
	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, false, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, false, fmt.Errorf("request official mini program code: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return nil, false, err
	}
	if response.StatusCode != http.StatusOK || strings.Contains(response.Header.Get("Content-Type"), "application/json") || (len(body) > 0 && body[0] == '{') {
		var apiError struct {
			ErrCode int    `json:"errcode"`
			ErrMsg  string `json:"errmsg"`
		}
		_ = json.Unmarshal(body, &apiError)
		return nil, false, fmt.Errorf("official mini program code failed (%d): %s", apiError.ErrCode, firstNonEmptyString(apiError.ErrMsg, response.Status))
	}
	return body, false, nil
}

func (s *wechatMiniProgramCodeService) accessToken(appID, secret string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token != "" && time.Now().Before(s.expiresAt) {
		return s.token, nil
	}
	endpoint := "https://api.weixin.qq.com/cgi-bin/token?grant_type=client_credential&appid=" + url.QueryEscape(appID) + "&secret=" + url.QueryEscape(secret)
	response, err := s.client.Get(endpoint)
	if err != nil {
		return "", fmt.Errorf("request WeChat access token: %w", err)
	}
	defer response.Body.Close()
	var payload struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload); err != nil {
		return "", err
	}
	if payload.AccessToken == "" {
		return "", fmt.Errorf("WeChat access token failed (%d): %s", payload.ErrCode, payload.ErrMsg)
	}
	expiresIn := payload.ExpiresIn
	if expiresIn <= 300 {
		expiresIn = 7200
	}
	s.token = payload.AccessToken
	s.expiresAt = time.Now().Add(time.Duration(expiresIn-300) * time.Second)
	return s.token, nil
}

func promotionEnvVersion() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("WECHAT_MINI_PROGRAM_ENV_VERSION"))) {
	case "trial":
		return "trial"
	case "release":
		return "release"
	default:
		return "develop"
	}
}
