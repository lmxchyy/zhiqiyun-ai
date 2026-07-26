package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	errPromotionInviteTokenInvalid  = errors.New("邀请链接无效")
	errPromotionInviteTokenExpired  = errors.New("邀请链接已过期")
	errPromotionInviteTokenInactive = errors.New("邀请人已停用")
)

const promotionIdentityOperationCenter = "OPERATION_CENTER"

type promotionInviteTokenRecord struct {
	Token        string
	OwnerUserID  string
	InviteCode   string
	IdentityType string
	Status       string
	ExpiresAt    *time.Time
}

type promotionInvitation struct {
	InviteToken         string
	InviteCode          string
	InviterUserID       string
	InviterName         string
	IdentityType        string
	TenantID            string
	OperationCenterID   string
	OperationCenterName string
}

type promotionInviteTokenStore interface {
	EnsurePromotionInviteToken(context.Context, string, string, string) (string, error)
	ResolvePromotionInviteToken(context.Context, string) (promotionInviteTokenRecord, error)
}

func promotionInviteIdentityType(role string) string {
	switch strings.ToUpper(strings.TrimSpace(role)) {
	case roleAgent:
		return roleAgent
	case roleOperation, promotionIdentityOperationCenter:
		return promotionIdentityOperationCenter
	default:
		return roleUser
	}
}

func promotionInviteTokenMarker(identityType string) string {
	switch promotionInviteIdentityType(identityType) {
	case roleAgent:
		return "a"
	case promotionIdentityOperationCenter:
		return "o"
	default:
		return "u"
	}
}

func fallbackPromotionInviteToken(ownerUserID, identityType, inviteCode string) string {
	seed := strings.Join([]string{strings.TrimSpace(ownerUserID), promotionInviteIdentityType(identityType), promotionCodeKey(inviteCode)}, "|")
	return "inv_" + promotionInviteTokenMarker(identityType) + strings.ToLower(shortStableHash(seed, 16))
}

func validPromotionInviteToken(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) != 21 || !strings.HasPrefix(value, "inv_") || !strings.Contains("aou", value[4:5]) {
		return false
	}
	for _, char := range value[5:] {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func ensurePromotionInviteToken(ctx context.Context, store platformStore, ownerUserID, identityType, inviteCode string) (string, error) {
	identityType = promotionInviteIdentityType(identityType)
	if tokenStore, ok := store.(promotionInviteTokenStore); ok {
		return tokenStore.EnsurePromotionInviteToken(ctx, ownerUserID, identityType, inviteCode)
	}
	return fallbackPromotionInviteToken(ownerUserID, identityType, inviteCode), nil
}

func resolvePromotionInvitation(ctx context.Context, store platformStore, data adminPlatformData, rawToken, rawCode string) (promotionInvitation, error) {
	token := strings.ToLower(strings.TrimSpace(rawToken))
	if token == "" {
		inviter, tenantID, identityType, err := promotionInviterByCode(data, rawCode)
		if err != nil {
			return promotionInvitation{}, err
		}
		return promotionInvitationFromUser(data, promotionInviteTokenRecord{
			OwnerUserID: inviter.ID, InviteCode: strings.ToUpper(strings.TrimSpace(rawCode)), IdentityType: identityType, Status: "ACTIVE",
		}, tenantID)
	}
	if !validPromotionInviteToken(token) {
		return promotionInvitation{}, errPromotionInviteTokenInvalid
	}
	var record promotionInviteTokenRecord
	var err error
	if tokenStore, ok := store.(promotionInviteTokenStore); ok {
		record, err = tokenStore.ResolvePromotionInviteToken(ctx, token)
	} else {
		record, err = fallbackPromotionInviteTokenRecord(data, token)
	}
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return promotionInvitation{}, errPromotionInviteTokenInvalid
		}
		return promotionInvitation{}, err
	}
	if err := promotionInviteTokenRecordError(record, time.Now().UTC()); err != nil {
		return promotionInvitation{}, err
	}
	return promotionInvitationFromUser(data, record, "")
}

func promotionInviteTokenRecordError(record promotionInviteTokenRecord, now time.Time) error {
	if !strings.EqualFold(record.Status, "ACTIVE") {
		return errPromotionInviteTokenInactive
	}
	if record.ExpiresAt != nil && !record.ExpiresAt.After(now) {
		return errPromotionInviteTokenExpired
	}
	return nil
}

func fallbackPromotionInviteTokenRecord(data adminPlatformData, token string) (promotionInviteTokenRecord, error) {
	for _, user := range data.Users {
		if agent, ok := channelAgentForUser(data.ChannelAgents, user.ID); ok {
			candidate := fallbackPromotionInviteToken(user.ID, roleAgent, agent.InviteCode)
			if strings.EqualFold(candidate, token) {
				return promotionInviteTokenRecord{Token: candidate, OwnerUserID: user.ID, InviteCode: agent.InviteCode, IdentityType: roleAgent, Status: agent.Status}, nil
			}
		}
		if center, ok := activeOperationCenterForUser(data.OperationCenters, user.ID); ok {
			candidate := fallbackPromotionInviteToken(user.ID, promotionIdentityOperationCenter, center.InviteCode)
			if strings.EqualFold(candidate, token) {
				return promotionInviteTokenRecord{Token: candidate, OwnerUserID: user.ID, InviteCode: center.InviteCode, IdentityType: promotionIdentityOperationCenter, Status: center.Status}, nil
			}
		}
		inviteCode := promotionInviteCode(data, user, roleUser)
		candidate := fallbackPromotionInviteToken(user.ID, roleUser, inviteCode)
		if strings.EqualFold(candidate, token) {
			return promotionInviteTokenRecord{Token: candidate, OwnerUserID: user.ID, InviteCode: inviteCode, IdentityType: roleUser, Status: user.Status}, nil
		}
	}
	return promotionInviteTokenRecord{}, sql.ErrNoRows
}

func promotionInvitationFromUser(data adminPlatformData, record promotionInviteTokenRecord, tenantFallback string) (promotionInvitation, error) {
	user, ok := userMap(data.Users)[record.OwnerUserID]
	if !ok {
		return promotionInvitation{}, errPromotionInviteTokenInvalid
	}
	if !strings.EqualFold(strings.TrimSpace(user.Status), "ACTIVE") {
		return promotionInvitation{}, errPromotionInviteTokenInactive
	}
	identityType := promotionInviteIdentityType(record.IdentityType)
	invitation := promotionInvitation{
		InviteToken: strings.ToLower(strings.TrimSpace(record.Token)), InviteCode: strings.ToUpper(strings.TrimSpace(record.InviteCode)),
		InviterUserID: user.ID, InviterName: firstNonEmptyString(strings.TrimSpace(user.Name), "知启云AI用户"), IdentityType: identityType,
		TenantID: firstNonEmptyString(user.TenantID, tenantFallback, "tenant_default"),
	}
	switch identityType {
	case roleAgent:
		agent, found := channelAgentForUser(data.ChannelAgents, user.ID)
		if !found || !strings.EqualFold(agent.Status, "ACTIVE") || !strings.EqualFold(strings.TrimSpace(agent.InviteCode), invitation.InviteCode) {
			return promotionInvitation{}, errPromotionInviteTokenInactive
		}
		invitation.OperationCenterID = strings.TrimSpace(agent.OperationCenterID)
	case promotionIdentityOperationCenter:
		center, found := activeOperationCenterForUser(data.OperationCenters, user.ID)
		if !found || !strings.EqualFold(strings.TrimSpace(center.InviteCode), invitation.InviteCode) {
			return promotionInvitation{}, errPromotionInviteTokenInactive
		}
		invitation.OperationCenterID = center.ID
		invitation.OperationCenterName = center.Name
	}
	return invitation, nil
}

func (s *postgresStore) EnsurePromotionInviteToken(ctx context.Context, ownerUserID, identityType, inviteCode string) (string, error) {
	identityType = promotionInviteIdentityType(identityType)
	inviteCode = strings.ToUpper(strings.TrimSpace(inviteCode))
	var existing string
	err := s.db.QueryRowContext(ctx, `
		SELECT coalesce(invite_token, '') FROM xz_marketing_invite_codes
		WHERE owner_user_id=$1 AND upper(btrim(code))=$2 AND identity_type=$3
		  AND upper(coalesce(status, ''))='ACTIVE' AND (expire_at IS NULL OR expire_at>now())
		LIMIT 1
	`, ownerUserID, inviteCode, identityType).Scan(&existing)
	if err == nil && validPromotionInviteToken(existing) {
		return strings.ToLower(existing), nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	for attempt := 0; attempt < 3; attempt++ {
		token := "inv_" + promotionInviteTokenMarker(identityType) + strings.ToLower(randomOpaqueID()[:16])
		err = s.db.QueryRowContext(ctx, `
			INSERT INTO xz_marketing_invite_codes(
			  id, owner_user_id, code, invite_token, identity_type, qrcode_url, landing_url, status, created_at, updated_at
			) VALUES($1,$2,$3,$4,$5,'','','ACTIVE',now(),now())
			ON CONFLICT (code) DO UPDATE SET
			  invite_token=CASE WHEN coalesce(xz_marketing_invite_codes.invite_token, '')='' THEN excluded.invite_token ELSE xz_marketing_invite_codes.invite_token END,
			  identity_type=excluded.identity_type, status='ACTIVE', updated_at=now()
			WHERE xz_marketing_invite_codes.owner_user_id=excluded.owner_user_id
			RETURNING invite_token
		`, "marketing_invite_"+shortStableHash(ownerUserID+"|"+identityType+"|"+inviteCode, 20), ownerUserID, inviteCode, token, identityType).Scan(&existing)
		if err == nil && validPromotionInviteToken(existing) {
			return strings.ToLower(existing), nil
		}
	}
	return "", fmt.Errorf("create promotion invite token: %w", err)
}

func (s *postgresStore) ResolvePromotionInviteToken(ctx context.Context, rawToken string) (promotionInviteTokenRecord, error) {
	var record promotionInviteTokenRecord
	var expiresAt sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT lower(invite_token), owner_user_id, upper(btrim(code)), identity_type, status, expire_at
		FROM xz_marketing_invite_codes WHERE lower(invite_token)=$1 LIMIT 1
	`, strings.ToLower(strings.TrimSpace(rawToken))).Scan(
		&record.Token, &record.OwnerUserID, &record.InviteCode, &record.IdentityType, &record.Status, &expiresAt,
	)
	if expiresAt.Valid {
		record.ExpiresAt = &expiresAt.Time
	}
	return record, err
}

func (a authAPI) resolvePromotionInvite(w http.ResponseWriter, r *http.Request) {
	token := firstNonEmptyString(r.URL.Query().Get("inviteToken"), r.URL.Query().Get("token"))
	code := firstNonEmptyString(r.URL.Query().Get("inviteCode"), r.URL.Query().Get("invite_code"))
	data, err := a.store.AdminData()
	if err != nil {
		writeMappedAuthFlowError(w, err)
		return
	}
	invitation, err := resolvePromotionInvitation(r.Context(), a.store, data, token, code)
	if err != nil {
		status := "invalid"
		if errors.Is(err, errPromotionInviteTokenExpired) {
			status = "expired"
		} else if errors.Is(err, errPromotionInviteTokenInactive) {
			status = "inviter_inactive"
		}
		writeJSON(w, map[string]any{"valid": false, "status": status, "message": err.Error()})
		return
	}
	writeJSON(w, map[string]any{
		"valid": true, "status": "valid", "inviteToken": invitation.InviteToken, "inviteCode": invitation.InviteCode,
		"inviterId": invitation.InviterUserID, "inviterName": invitation.InviterName, "identityType": invitation.IdentityType,
		"operationCenterId": invitation.OperationCenterID, "operationCenterName": invitation.OperationCenterName,
	})
}

func promotionInviteTokenH5Redirect(w http.ResponseWriter, r *http.Request) {
	token := strings.ToLower(strings.TrimSpace(r.PathValue("inviteToken")))
	if !validPromotionInviteToken(token) {
		writeError(w, http.StatusNotFound, errPromotionInviteTokenInvalid)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, "/h5/#/pages/WechatLoginPage?inviteToken="+url.QueryEscape(token), http.StatusFound)
}
