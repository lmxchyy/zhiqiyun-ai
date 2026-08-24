package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	pointsapp "xianzhi-ai/backend-go/internal/points/application"
)

type personalPointInflowResult struct {
	Account                         adminPointAccount
	AvailableBefore, AvailableAfter int
}

func grantPermanentPersonalPointsTx(ctx context.Context, tx *sql.Tx, userID string, source PointSource, points int, referenceType, referenceID, idempotencyKey string, grantedAt time.Time) (personalPointInflowResult, error) {
	if tx == nil || strings.TrimSpace(userID) == "" || points <= 0 || isExpiringPersonalPointSource(source) {
		return personalPointInflowResult{}, ErrInvalidPointCommand
	}
	after, err := pointsapp.GrantPermanentTx(ctx, tx, pointsapp.PermanentGrantRequest{
		UserID: userID, Source: string(source), Points: int64(points), ReferenceType: referenceType,
		ReferenceID: referenceID, IdempotencyKey: idempotencyKey, GrantedAt: grantedAt,
	}, func(ctx context.Context, tx *sql.Tx, userID string) (pointsapp.AccountSnapshot, error) {
		account, err := pointAccountForUpdate(ctx, tx, userID)
		if err != nil {
			return pointsapp.AccountSnapshot{}, err
		}
		return pointsapp.AccountSnapshot{ID: account.ID, UserID: account.UserID, Available: int64(account.Available), Frozen: int64(account.Frozen), TotalGranted: int64(account.TotalGranted), TotalUsed: int64(account.TotalUsed)}, nil
	}, func(ctx context.Context, tx *sql.Tx, request pointsapp.PermanentGrantRequest, accountID string) error {
		_, err := NewPostgresPersonalPointStore(nil).grantTx(ctx, tx, PersonalPointGrantCommand{
			AccountID: accountID, UserID: request.UserID, Source: PointSource(request.Source), Points: request.Points, ReferenceType: request.ReferenceType,
			ReferenceID: request.ReferenceID, IdempotencyKey: request.IdempotencyKey, GrantedAt: request.GrantedAt,
		})
		return err
	})
	if err != nil {
		return personalPointInflowResult{}, err
	}
	return personalPointInflowResult{
		Account:         adminPointAccount{ID: after.ID, UserID: after.UserID, Available: int(after.Available), Frozen: int(after.Frozen), TotalGranted: int(after.TotalGranted), TotalUsed: int(after.TotalUsed)},
		AvailableBefore: int(after.Available) - points, AvailableAfter: int(after.Available),
	}, nil
}

func grantPermanentAdminJSONPersonalPoints(data *adminPlatformData, userID string, source PointSource, points int, referenceType, referenceID, idempotencyKey string, grantedAt time.Time) (personalPointInflowResult, error) {
	if data == nil || strings.TrimSpace(userID) == "" || points <= 0 || isExpiringPersonalPointSource(source) {
		return personalPointInflowResult{}, ErrInvalidPointCommand
	}
	accountID := ""
	for _, item := range data.PersonalPoints.Accounts {
		if item.UserID == userID {
			if accountID != "" {
				return personalPointInflowResult{}, ErrPointOwnership
			}
			accountID = item.ID
		}
	}
	if accountID == "" {
		for _, item := range data.PointAccounts {
			if item.UserID == userID {
				accountID = item.ID
				store := &JSONPersonalPointStore{memory: &data.PersonalPoints}
				if err := store.importLegacyAccounts([]adminPointAccount{item}); err != nil {
					return personalPointInflowResult{}, err
				}
				account, err := findPersonalAccount(store.memory, accountID, userID)
				if err != nil {
					return personalPointInflowResult{}, err
				}
				if account != nil {
					// The legacy opening balance is carried by its LEGACY lot; it is not
					// a newly granted amount and must not inflate TotalGranted.
					account.TotalGranted = int64(item.TotalGranted)
				}
				data.PersonalPoints = clonePersonalPointState(*store.memory)
				break
			}
		}
	}
	if accountID == "" {
		accountID = "points:" + userID
	}
	store := &JSONPersonalPointStore{memory: &data.PersonalPoints}
	before := int64(0)
	if existing, err := findPersonalAccount(&data.PersonalPoints, accountID, userID); err != nil {
		return personalPointInflowResult{}, err
	} else if existing != nil {
		before = existing.AvailablePoints
	}
	if _, err := store.grant(context.Background(), PersonalPointGrantCommand{
		AccountID: accountID, UserID: userID, Source: source, Points: int64(points), ReferenceType: referenceType,
		ReferenceID: referenceID, IdempotencyKey: idempotencyKey, GrantedAt: grantedAt,
	}); err != nil {
		return personalPointInflowResult{}, err
	}
	data.PersonalPoints = clonePersonalPointState(*store.memory)
	account, err := findPersonalAccount(&data.PersonalPoints, accountID, userID)
	if err != nil {
		return personalPointInflowResult{}, err
	}
	if account == nil {
		return personalPointInflowResult{}, ErrPointNotFound
	}
	if account.AvailablePoints-before != int64(points) {
		return personalPointInflowResult{}, errors.New("personal point JSON inflow projection mismatch")
	}
	projected := adminPointAccount{ID: account.ID, UserID: account.UserID, Available: int(account.AvailablePoints), Frozen: int(account.FrozenPoints), TotalGranted: int(account.TotalGranted), TotalUsed: int(account.TotalConsumed)}
	found := false
	for index := range data.PointAccounts {
		if data.PointAccounts[index].UserID == userID {
			data.PointAccounts[index] = projected
			found = true
			break
		}
	}
	if !found {
		data.PointAccounts = append(data.PointAccounts, projected)
	}
	ledgerIndex := make(map[string]int, len(data.WalletLedger))
	for index := range data.WalletLedger {
		ledgerIndex[data.WalletLedger[index].ID] = index
	}
	for _, entry := range data.PersonalPoints.WalletLedger {
		item := walletLedgerEntry{ID: entry.ID, AccountID: entry.AccountID, UserID: entry.UserID, TenantID: entry.TenantID, TaskID: entry.TaskID, BillingEventID: entry.BillingEventID, EntryType: entry.EntryType, Points: float64(entry.Points), AvailableBefore: float64(entry.AvailableBefore), AvailableAfter: float64(entry.AvailableAfter), FrozenBefore: float64(entry.FrozenBefore), FrozenAfter: float64(entry.FrozenAfter), IdempotencyKey: entry.IdempotencyKey, ReferenceType: entry.ReferenceType, ReferenceID: entry.ReferenceID, Remark: entry.Remark, Metadata: cloneAnyMap(entry.Metadata), CreatedAt: entry.CreatedAt.UTC().Format(time.RFC3339Nano)}
		if index, ok := ledgerIndex[item.ID]; ok {
			data.WalletLedger[index] = item
		} else {
			ledgerIndex[item.ID] = len(data.WalletLedger)
			data.WalletLedger = append(data.WalletLedger, item)
		}
	}
	data.PointsAvailable = &projected.Available
	return personalPointInflowResult{Account: projected, AvailableBefore: int(before), AvailableAfter: projected.Available}, nil
}

func isExpiringPersonalPointSource(source PointSource) bool {
	return source == PointSourceRegistrationGift || source == PointSourceActivityGift || source == PointSourceAdminGift
}

func paidPointSourceForChangeType(changeType string) PointSource {
	switch strings.ToUpper(strings.TrimSpace(changeType)) {
	case "MEMBER_PACKAGE_GRANT":
		return PointSourceMemberPackageGrant
	case "AGENT_JOIN_GRANT":
		return PointSourceAgentJoinGrant
	case "OPERATION_CENTER_GRANT":
		return PointSourceOperationCenterGrant
	case "USER_RECHARGE_GRANT":
		return PointSourceRecharge
	default:
		return PointSourceCommerceOrder
	}
}

func pointGrantedAt(value string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, strings.TrimSpace(value)); err == nil {
			return parsed.UTC()
		}
	}
	return time.Now().UTC()
}
