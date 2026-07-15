package httpserver

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"time"
)

var (
	errPromotionInviteAlreadyBound = errors.New("promotion invite relationship already exists")
	errPromotionSelfInvite         = errors.New("users cannot invite themselves")
	errPromotionInviteCycle        = errors.New("promotion invite relationship would create a cycle")
)

func (s *jsonStore) ListPromotionRecords(inviterUserID string, tenantID string) ([]promotionRecord, error) {
	data, err := s.AdminData()
	if err != nil {
		return nil, err
	}
	items := make([]promotionRecord, 0)
	for _, item := range data.PromotionRecords {
		if item.InviterUserID == inviterUserID && firstNonEmptyString(item.TenantID, "tenant_default") == firstNonEmptyString(tenantID, "tenant_default") {
			items = append(items, item)
		}
	}
	sortPromotionRecords(items)
	return items, nil
}

func (s *jsonStore) RecordPromotionVisit(input promotionVisitInput) (promotionRecord, error) {
	var result promotionRecord
	err := s.updateAdmin(func(data *adminPlatformData) error {
		for i := range data.PromotionRecords {
			if data.PromotionRecords[i].ID == input.ID {
				result = data.PromotionRecords[i]
				return nil
			}
		}
		now := input.VisitedAt.UTC()
		result = promotionRecord{
			ID: input.ID, TenantID: firstNonEmptyString(input.TenantID, "tenant_default"),
			InviterUserID: input.InviterUserID, VisitorID: input.VisitorID,
			VisitorName: input.VisitorName, MaskedMobile: input.MaskedMobile,
			InviteCode: strings.ToUpper(strings.TrimSpace(input.InviteCode)), Status: promotionStatusVisited,
			Source: firstNonEmptyString(input.Source, "wechat_friend"), TemplateID: firstNonEmptyString(input.TemplateID, "poster.brand.simple"),
			ActivityID: input.ActivityID, VisitTime: now.Format(time.RFC3339),
			RewardStatus: "PENDING", CreatedAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339),
		}
		data.PromotionRecords = append(data.PromotionRecords, result)
		return nil
	})
	return result, err
}

func (s *jsonStore) BindPromotionInvite(input promotionBindInput) (promotionRecord, error) {
	var result promotionRecord
	err := s.updateAdmin(func(data *adminPlatformData) error {
		if input.InviterUserID == input.InviteeUserID {
			return errPromotionSelfInvite
		}
		inviteeIndex := -1
		referrals := map[string]string{}
		for i := range data.Users {
			referrals[data.Users[i].ID] = data.Users[i].ReferredBy
			if data.Users[i].ID == input.InviteeUserID {
				inviteeIndex = i
			}
		}
		if inviteeIndex < 0 {
			return errUnauthorized
		}
		if existing := strings.TrimSpace(data.Users[inviteeIndex].ReferredBy); existing != "" {
			if existing != input.InviterUserID {
				return errPromotionInviteAlreadyBound
			}
			return bindPromotionRecordInMemory(data, input, &result)
		}
		if promotionReferralCycle(referrals, input.InviterUserID, input.InviteeUserID) {
			return errPromotionInviteCycle
		}
		data.Users[inviteeIndex].ReferredBy = input.InviterUserID
		data.Users[inviteeIndex].UpdatedAt = input.BoundAt.UTC().Format(time.RFC3339)
		return bindPromotionRecordInMemory(data, input, &result)
	})
	return result, err
}

func bindPromotionRecordInMemory(data *adminPlatformData, input promotionBindInput, result *promotionRecord) error {
	now := input.BoundAt.UTC().Format(time.RFC3339)
	for i := len(data.PromotionRecords) - 1; i >= 0; i-- {
		item := &data.PromotionRecords[i]
		if item.InviterUserID == input.InviterUserID && item.InviteCode == input.InviteCode && (item.VisitorID == input.InviteeUserID || item.InviteeUserID == input.InviteeUserID) {
			item.InviteeUserID = input.InviteeUserID
			item.Status = promotionStatusRegistered
			item.RegisterTime = now
			item.UpdatedAt = now
			*result = *item
			return nil
		}
	}
	*result = promotionRecord{
		ID: promotionBoundRecordID(input.InviterUserID, input.InviteeUserID), TenantID: firstNonEmptyString(input.TenantID, "tenant_default"),
		InviterUserID: input.InviterUserID, InviteeUserID: input.InviteeUserID, VisitorID: input.InviteeUserID,
		InviteCode: input.InviteCode, Status: promotionStatusRegistered, Source: firstNonEmptyString(input.Source, "wechat_friend"),
		TemplateID: firstNonEmptyString(input.TemplateID, "poster.brand.simple"), ActivityID: input.ActivityID,
		VisitTime: now, RegisterTime: now, RewardStatus: "PENDING", CreatedAt: now, UpdatedAt: now,
	}
	data.PromotionRecords = append(data.PromotionRecords, *result)
	return nil
}

func (s *postgresStore) ListPromotionRecords(inviterUserID string, tenantID string) ([]promotionRecord, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, promotionRecordSelect+`
		where inviter_user_id=$1 and tenant_id=$2 order by created_at desc
	`, inviterUserID, firstNonEmptyString(tenantID, "tenant_default"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []promotionRecord{}
	for rows.Next() {
		item, err := scanPromotionRecord(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) RecordPromotionVisit(input promotionVisitInput) (promotionRecord, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return promotionRecord{}, err
	}
	now := input.VisitedAt.UTC()
	_, err := s.db.ExecContext(ctx, `
		insert into xz_marketing_invite_records
		(id, inviter_user_id, invitee_user_id, invite_code, source, register_status, recharge_status, upgrade_status,
		 tenant_id, visitor_id, visitor_name, masked_mobile, status, template_id, activity_id, visit_time, reward_status, metadata, created_at, updated_at)
		values ($1,$2,null,$3,$4,'PENDING','PENDING','PENDING',$5,$6,$7,$8,$9,$10,$11,$12,'PENDING','{}'::jsonb,$12,$12)
		on conflict (id) do nothing
	`, input.ID, input.InviterUserID, strings.ToUpper(input.InviteCode), firstNonEmptyString(input.Source, "wechat_friend"),
		firstNonEmptyString(input.TenantID, "tenant_default"), input.VisitorID, input.VisitorName, input.MaskedMobile,
		promotionStatusVisited, firstNonEmptyString(input.TemplateID, "poster.brand.simple"), nullIfEmpty(input.ActivityID), now)
	if err != nil {
		return promotionRecord{}, err
	}
	return s.promotionRecordByID(ctx, input.ID)
}

func (s *postgresStore) BindPromotionInvite(input promotionBindInput) (promotionRecord, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return promotionRecord{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return promotionRecord{}, err
	}
	defer tx.Rollback()
	if input.InviterUserID == input.InviteeUserID {
		return promotionRecord{}, errPromotionSelfInvite
	}
	var invitee adminUser
	if err := tx.QueryRowContext(ctx, `select raw from xz_users where id=$1 for update`, input.InviteeUserID).Scan(rawScanner(&invitee)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return promotionRecord{}, errUnauthorized
		}
		return promotionRecord{}, err
	}
	if existing := strings.TrimSpace(invitee.ReferredBy); existing != "" && existing != input.InviterUserID {
		return promotionRecord{}, errPromotionInviteAlreadyBound
	}
	rows, err := tx.QueryContext(ctx, `select id, coalesce(referred_by,'') from xz_users`)
	if err != nil {
		return promotionRecord{}, err
	}
	referrals := map[string]string{}
	for rows.Next() {
		var id, parent string
		if err := rows.Scan(&id, &parent); err != nil {
			rows.Close()
			return promotionRecord{}, err
		}
		referrals[id] = parent
	}
	if err := rows.Close(); err != nil {
		return promotionRecord{}, err
	}
	if promotionReferralCycle(referrals, input.InviterUserID, input.InviteeUserID) {
		return promotionRecord{}, errPromotionInviteCycle
	}
	now := input.BoundAt.UTC()
	invitee.ReferredBy = input.InviterUserID
	invitee.UpdatedAt = now.Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `update xz_users set referred_by=$2, updated_at=$3, raw=$4::jsonb where id=$1`, invitee.ID, invitee.ReferredBy, invitee.UpdatedAt, jsonProjection(invitee)); err != nil {
		return promotionRecord{}, err
	}
	var recordID string
	err = tx.QueryRowContext(ctx, `
		select id from xz_marketing_invite_records
		where inviter_user_id=$1 and invite_code=$2 and (visitor_id=$3 or invitee_user_id=$3)
		order by created_at desc limit 1 for update
	`, input.InviterUserID, input.InviteCode, input.InviteeUserID).Scan(&recordID)
	if errors.Is(err, sql.ErrNoRows) {
		recordID = promotionBoundRecordID(input.InviterUserID, input.InviteeUserID)
		_, err = tx.ExecContext(ctx, `
			insert into xz_marketing_invite_records
			(id, inviter_user_id, invitee_user_id, invite_code, source, register_status, recharge_status, upgrade_status,
			 tenant_id, visitor_id, status, template_id, activity_id, visit_time, register_time, reward_status, metadata, created_at, updated_at)
			values ($1,$2,$3,$4,$5,'REGISTERED','PENDING','PENDING',$6,$3,$7,$8,$9,$10,$10,'PENDING','{}'::jsonb,$10,$10)
			on conflict (id) do update set invitee_user_id=excluded.invitee_user_id, register_status='REGISTERED', status=$7, register_time=$10, updated_at=$10
		`, recordID, input.InviterUserID, input.InviteeUserID, input.InviteCode, firstNonEmptyString(input.Source, "wechat_friend"),
			firstNonEmptyString(input.TenantID, "tenant_default"), promotionStatusRegistered,
			firstNonEmptyString(input.TemplateID, "poster.brand.simple"), nullIfEmpty(input.ActivityID), now)
	} else if err == nil {
		_, err = tx.ExecContext(ctx, `
			update xz_marketing_invite_records set invitee_user_id=$2, register_status='REGISTERED', status=$3, register_time=$4, updated_at=$4 where id=$1
		`, recordID, input.InviteeUserID, promotionStatusRegistered, now)
	}
	if err != nil {
		return promotionRecord{}, err
	}
	if err := insertAuditLog(ctx, tx, input.InviteeUserID, roleUser, "promotion.bind", "promotion_invite", recordID, "POST", "/api/v1/promotion/bind", 200, map[string]any{"inviterUserId": input.InviterUserID}); err != nil {
		return promotionRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return promotionRecord{}, err
	}
	return s.promotionRecordByID(ctx, recordID)
}

const promotionRecordSelect = `select id, tenant_id, inviter_user_id, coalesce(invitee_user_id,''), coalesce(visitor_id,''),
	coalesce(visitor_name,''), coalesce(masked_mobile,''), invite_code, status, source, template_id, coalesce(activity_id,''),
	visit_time, register_time, paid_time, reward_amount_cents, reward_status, created_at, updated_at
	from xz_marketing_invite_records`

type promotionRowScanner interface {
	Scan(dest ...any) error
}

func scanPromotionRecord(scanner promotionRowScanner) (promotionRecord, error) {
	var item promotionRecord
	var visitTime, registerTime, paidTime sql.NullTime
	var createdAt, updatedAt time.Time
	err := scanner.Scan(&item.ID, &item.TenantID, &item.InviterUserID, &item.InviteeUserID, &item.VisitorID,
		&item.VisitorName, &item.MaskedMobile, &item.InviteCode, &item.Status, &item.Source, &item.TemplateID, &item.ActivityID,
		&visitTime, &registerTime, &paidTime, &item.RewardAmountCents, &item.RewardStatus, &createdAt, &updatedAt)
	if err != nil {
		return promotionRecord{}, err
	}
	item.VisitTime = nullableTimeString(visitTime)
	item.RegisterTime = nullableTimeString(registerTime)
	item.PaidTime = nullableTimeString(paidTime)
	item.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	item.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	return item, nil
}

func (s *postgresStore) promotionRecordByID(ctx context.Context, id string) (promotionRecord, error) {
	return scanPromotionRecord(s.db.QueryRowContext(ctx, promotionRecordSelect+` where id=$1`, id))
}

func nullableTimeString(value sql.NullTime) string {
	if !value.Valid {
		return ""
	}
	return value.Time.UTC().Format(time.RFC3339)
}

func promotionReferralCycle(referrals map[string]string, inviterUserID string, inviteeUserID string) bool {
	seen := map[string]bool{}
	current := inviterUserID
	for current != "" && !seen[current] {
		if current == inviteeUserID {
			return true
		}
		seen[current] = true
		current = referrals[current]
	}
	return false
}

func promotionBoundRecordID(inviterUserID string, inviteeUserID string) string {
	return "promotion_bind_" + shortStableHash(inviterUserID+"|"+inviteeUserID, 20)
}

func shortStableHash(value string, length int) string {
	digest := sha256.Sum256([]byte(value))
	encoded := hex.EncodeToString(digest[:])
	if length <= 0 || length >= len(encoded) {
		return encoded
	}
	return encoded[:length]
}

func sortPromotionRecords(items []promotionRecord) {
	sort.SliceStable(items, func(i, j int) bool {
		return firstNonEmptyString(items[i].VisitTime, items[i].CreatedAt) > firstNonEmptyString(items[j].VisitTime, items[j].CreatedAt)
	})
}

func nullIfEmpty(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.TrimSpace(value)
}

var _ promotionDataStore = (*jsonStore)(nil)
var _ promotionDataStore = (*postgresStore)(nil)
