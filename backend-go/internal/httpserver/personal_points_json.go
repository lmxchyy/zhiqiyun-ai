package httpserver

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type personalPointState struct {
	Accounts     []PersonalPointAccount           `json:"accounts"`
	Lots         []PersonalPointLot               `json:"lots"`
	Reservations []PersonalPointReservation       `json:"reservations"`
	Allocations  []PersonalPointAllocation        `json:"allocations"`
	Movements    []PersonalPointLotMovement       `json:"movements"`
	WalletLedger []PersonalPointWalletLedgerEntry `json:"wallet_ledger"`
	Operations   []personalPointOperation         `json:"operations"`
	Policies     []PointExpiryPolicy              `json:"policies"`
}

type personalPointImportState struct {
	Version         int       `json:"version,omitempty"`
	SidecarChecksum string    `json:"sidecarChecksum,omitempty"`
	ImportedAt      time.Time `json:"importedAt,omitempty"`
}

type JSONPersonalPointStore struct {
	path    string
	mu      sync.Mutex
	memory  *personalPointState
	initErr error
	owner   *jsonStore
}

func NewJSONPersonalPointStore(path string) *JSONPersonalPointStore {
	return &JSONPersonalPointStore{path: path}
}

func (s *JSONPersonalPointStore) operationalError() error {
	if s == nil {
		return ErrInvalidPointCommand
	}
	return s.initErr
}

func defaultPersonalPointPolicy() PointExpiryPolicy {
	return PointExpiryPolicy{
		ID: "point_expiry_policy_v1", Version: 1, Revision: 1, Enabled: true,
		DurationValue: 3, DurationUnit: "CALENDAR_MONTH", TimeZone: "Asia/Shanghai",
		SourceTypes: []string{string(PointSourceRegistrationGift), string(PointSourceActivityGift), string(PointSourceAdminGift)},
		Status:      "PUBLISHED",
	}
}

func (s *JSONPersonalPointStore) loadLocked() (personalPointState, error) {
	state := personalPointState{}
	if len(state.Policies) == 0 {
		state.Policies = []PointExpiryPolicy{defaultPersonalPointPolicy()}
	}
	state.WalletLedger = []PersonalPointWalletLedgerEntry{}
	if s == nil {
		return state, nil
	}
	if s.path == "" {
		if s.memory == nil {
			s.memory = &state
		} else {
			state = *s.memory
		}
		return state, nil
	}
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if len(raw) == 0 {
		return state, nil
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return state, fmt.Errorf("decode personal points: %w", err)
	}
	if state.Accounts == nil {
		state.Accounts = []PersonalPointAccount{}
	}
	if state.Lots == nil {
		state.Lots = []PersonalPointLot{}
	}
	if state.Reservations == nil {
		state.Reservations = []PersonalPointReservation{}
	}
	if state.Allocations == nil {
		state.Allocations = []PersonalPointAllocation{}
	}
	if state.Movements == nil {
		state.Movements = []PersonalPointLotMovement{}
	}
	if state.WalletLedger == nil {
		state.WalletLedger = []PersonalPointWalletLedgerEntry{}
	}
	if state.Operations == nil {
		state.Operations = []personalPointOperation{}
	}
	if state.Policies == nil || len(state.Policies) == 0 {
		state.Policies = []PointExpiryPolicy{defaultPersonalPointPolicy()}
	}
	return state, nil
}

func validatePersonalWalletTransition(entryType string, points, availableBefore, availableAfter, frozenBefore, frozenAfter int64) error {
	const maxInt64 = int64(1<<63 - 1)
	if points <= 0 || availableBefore < 0 || availableAfter < 0 || frozenBefore < 0 || frozenAfter < 0 {
		return ErrInvalidPointCommand
	}
	switch entryType {
	case "GRANT", "RECHARGE", "REFUND":
		if availableBefore > maxInt64-points || availableAfter != availableBefore+points || frozenAfter != frozenBefore {
			return ErrInvalidPointCommand
		}
	case "RESERVE":
		if availableBefore < points || frozenBefore > maxInt64-points || availableAfter != availableBefore-points || frozenAfter != frozenBefore+points {
			return ErrInvalidPointCommand
		}
	case "CAPTURE":
		if availableAfter != availableBefore || frozenBefore < points || frozenAfter != frozenBefore-points {
			return ErrInvalidPointCommand
		}
	case "RELEASE":
		if availableBefore > maxInt64-points || frozenBefore < points || availableAfter != availableBefore+points || frozenAfter != frozenBefore-points {
			return ErrInvalidPointCommand
		}
	case "EXPIRE":
		if availableBefore < points || availableAfter != availableBefore-points || frozenAfter != frozenBefore {
			return ErrInvalidPointCommand
		}
	case "ADJUSTMENT":
		if frozenAfter != frozenBefore {
			return ErrInvalidPointCommand
		}
		positive := availableBefore <= maxInt64-points && availableAfter == availableBefore+points
		negative := availableBefore >= points && availableAfter == availableBefore-points
		if !positive && !negative {
			return ErrInvalidPointCommand
		}
	default:
		return ErrInvalidPointCommand
	}
	return nil
}

func appendPersonalWalletLedger(state *personalPointState, account PersonalPointAccount, entryType string, points, beforeAvailable, beforeFrozen int64, key, referenceType, referenceID string, metadata map[string]any, occurredAt time.Time) error {
	if state == nil || account.ID == "" || account.UserID == "" || strings.TrimSpace(key) == "" || points < 0 || beforeAvailable < 0 || beforeFrozen < 0 || account.AvailablePoints < 0 || account.FrozenPoints < 0 {
		return ErrInvalidPointCommand
	}
	if err := validatePersonalWalletTransition(entryType, points, beforeAvailable, account.AvailablePoints, beforeFrozen, account.FrozenPoints); err != nil {
		return err
	}
	for _, existing := range state.WalletLedger {
		if existing.IdempotencyKey != key {
			continue
		}
		if existing.AccountID != account.ID || existing.UserID != account.UserID || existing.EntryType != entryType || existing.Points != points || existing.AvailableBefore != beforeAvailable || existing.AvailableAfter != account.AvailablePoints || existing.FrozenBefore != beforeFrozen || existing.FrozenAfter != account.FrozenPoints {
			return ErrIdempotencyConflict
		}
		return nil
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	occurredAt = pointNow(occurredAt)
	state.WalletLedger = append(state.WalletLedger, PersonalPointWalletLedgerEntry{
		ID: stablePointID("wallet", account.ID, key), AccountID: account.ID, UserID: account.UserID,
		EntryType: entryType, Points: points, AvailableBefore: beforeAvailable, AvailableAfter: account.AvailablePoints,
		FrozenBefore: beforeFrozen, FrozenAfter: account.FrozenPoints, IdempotencyKey: key,
		ReferenceType: referenceType, ReferenceID: referenceID, Metadata: metadata, OccurredAt: occurredAt, CreatedAt: occurredAt,
	})
	return nil
}

func legacyWalletFloatToInt64(value float64) (int64, error) {
	const maxInt64Exclusive = float64(uint64(1) << 63)
	if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value || value < -maxInt64Exclusive || value >= maxInt64Exclusive || value < 0 {
		return 0, ErrInvalidPointCommand
	}
	return int64(value), nil
}

func legacyWalletCreatedAt(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, ErrInvalidPointCommand
	}
	return parsed.UTC(), nil
}

func resolveLegacyWalletAccount(entry walletLedgerEntry, accounts []adminPointAccount) (adminPointAccount, error) {
	accountID := strings.TrimSpace(entry.AccountID)
	userID := strings.TrimSpace(entry.UserID)
	if accountID == "" && userID == "" {
		return adminPointAccount{}, ErrInvalidPointCommand
	}
	var resolved adminPointAccount
	resolvedSet := false
	if accountID != "" {
		matches := 0
		for i := range accounts {
			if strings.TrimSpace(accounts[i].ID) == accountID {
				resolved = accounts[i]
				matches++
			}
		}
		if matches != 1 {
			return adminPointAccount{}, ErrPointNotFound
		}
		resolvedSet = true
	}
	if userID != "" {
		matches := 0
		for i := range accounts {
			if strings.TrimSpace(accounts[i].UserID) != userID {
				continue
			}
			if resolvedSet {
				if strings.TrimSpace(accounts[i].ID) != accountID {
					return adminPointAccount{}, ErrPointOwnership
				}
				matches++
				continue
			}
			resolved = accounts[i]
			resolvedSet = true
			matches++
		}
		if matches == 0 {
			return adminPointAccount{}, ErrPointNotFound
		}
		if !resolvedSet || (accountID == "" && matches != 1) {
			return adminPointAccount{}, ErrPointOwnership
		}
	}
	if !resolvedSet || strings.TrimSpace(resolved.ID) == "" || strings.TrimSpace(resolved.UserID) == "" || resolved.Available < 0 || resolved.Frozen < 0 {
		return adminPointAccount{}, ErrInvalidPointCommand
	}
	if accountID != "" && strings.TrimSpace(resolved.ID) != accountID {
		return adminPointAccount{}, ErrPointOwnership
	}
	if userID != "" && strings.TrimSpace(resolved.UserID) != userID {
		return adminPointAccount{}, ErrPointOwnership
	}
	resolved.ID, resolved.UserID = strings.TrimSpace(resolved.ID), strings.TrimSpace(resolved.UserID)
	return resolved, nil
}

func legacyWalletEntryType(value string) (string, error) {
	entryType := upperTrim(value)
	switch entryType {
	case "RECHARGE", "GRANT", "RESERVE", "CAPTURE", "RELEASE", "REFUND", "ADJUSTMENT", "EXPIRE":
		return entryType, nil
	default:
		return "", ErrInvalidPointCommand
	}
}

func isLegacySyntheticWalletEntry(entry walletLedgerEntry) bool {
	if upperTrim(entry.EntryType) != "GRANT" {
		return false
	}
	if upperTrim(entry.ReferenceType) == "LEGACY_IMPORT" {
		return true
	}
	return entry.Metadata != nil && boolValue(entry.Metadata["legacy_import"])
}

func legacyWalletLedgerKey(accountID string, entry walletLedgerEntry, synthetic bool) (string, error) {
	if synthetic {
		return personalWalletKey(accountID, "grant", "legacy-import:"+accountID), nil
	}
	legacyKey := strings.TrimSpace(entry.IdempotencyKey)
	legacyID := strings.TrimSpace(entry.ID)
	if legacyKey == "" && legacyID == "" {
		return "", ErrInvalidPointCommand
	}
	if legacyKey == "" {
		legacyKey = legacyID
	}
	if legacyID != "" {
		legacyKey += ":" + legacyID
	}
	return personalWalletKey(accountID, "legacy-wallet", legacyKey), nil
}

func appendMigratedWalletLedger(state *personalPointState, candidate PersonalPointWalletLedgerEntry) error {
	if state == nil || candidate.AccountID == "" || candidate.UserID == "" || strings.TrimSpace(candidate.IdempotencyKey) == "" {
		return ErrInvalidPointCommand
	}
	if err := validatePersonalWalletTransition(candidate.EntryType, candidate.Points, candidate.AvailableBefore, candidate.AvailableAfter, candidate.FrozenBefore, candidate.FrozenAfter); err != nil {
		return err
	}
	for _, existing := range state.WalletLedger {
		if existing.IdempotencyKey != candidate.IdempotencyKey {
			continue
		}
		if existing.AccountID != candidate.AccountID || existing.UserID != candidate.UserID || existing.EntryType != candidate.EntryType || existing.Points != candidate.Points || existing.AvailableBefore != candidate.AvailableBefore || existing.AvailableAfter != candidate.AvailableAfter || existing.FrozenBefore != candidate.FrozenBefore || existing.FrozenAfter != candidate.FrozenAfter || existing.ReferenceType != candidate.ReferenceType || existing.ReferenceID != candidate.ReferenceID {
			return ErrIdempotencyConflict
		}
		return nil
	}
	state.WalletLedger = append(state.WalletLedger, candidate)
	return nil
}

func migrateLegacyWalletLedgerState(state *personalPointState, entries []walletLedgerEntry, accounts []adminPointAccount) error {
	for _, legacy := range entries {
		account, err := resolveLegacyWalletAccount(legacy, accounts)
		if err != nil {
			return err
		}
		entryType, err := legacyWalletEntryType(legacy.EntryType)
		if err != nil {
			return err
		}
		points, err := legacyWalletFloatToInt64(legacy.Points)
		if err != nil {
			return err
		}
		availableBefore, err := legacyWalletFloatToInt64(legacy.AvailableBefore)
		if err != nil {
			return err
		}
		availableAfter, err := legacyWalletFloatToInt64(legacy.AvailableAfter)
		if err != nil {
			return err
		}
		frozenBefore, err := legacyWalletFloatToInt64(legacy.FrozenBefore)
		if err != nil {
			return err
		}
		frozenAfter, err := legacyWalletFloatToInt64(legacy.FrozenAfter)
		if err != nil {
			return err
		}
		occurredAt, err := legacyWalletCreatedAt(legacy.CreatedAt)
		if err != nil {
			return err
		}
		synthetic := isLegacySyntheticWalletEntry(legacy)
		key, err := legacyWalletLedgerKey(account.ID, legacy, synthetic)
		if err != nil {
			return err
		}
		metadata := cloneAnyMap(legacy.Metadata)
		if metadata == nil {
			metadata = map[string]any{}
		}
		if strings.TrimSpace(legacy.ID) != "" {
			metadata["legacy_ledger_id"] = legacy.ID
		}
		candidate := PersonalPointWalletLedgerEntry{
			ID: stablePointID("wallet", account.ID, key), AccountID: account.ID, UserID: account.UserID,
			TenantID: legacy.TenantID, TaskID: legacy.TaskID, BillingEventID: legacy.BillingEventID,
			EntryType: entryType, Points: points, AvailableBefore: availableBefore, AvailableAfter: availableAfter,
			FrozenBefore: frozenBefore, FrozenAfter: frozenAfter, IdempotencyKey: key,
			ReferenceType: legacy.ReferenceType, ReferenceID: legacy.ReferenceID, Remark: legacy.Remark,
			Metadata: metadata, OccurredAt: occurredAt, CreatedAt: occurredAt,
		}
		if err := appendMigratedWalletLedger(state, candidate); err != nil {
			return err
		}
	}
	return nil
}

func (s *JSONPersonalPointStore) saveLocked(state personalPointState) error {
	if s == nil {
		return nil
	}
	if s.path == "" {
		s.memory = &state
		return nil
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".personal-points-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, s.path)
}

func (s *JSONPersonalPointStore) AccountCount() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.loadLocked()
	if err != nil {
		return 0
	}
	return len(state.Accounts)
}

func (s *JSONPersonalPointStore) SetPolicy(policy PointExpiryPolicy) error {
	if err := validatePointExpiryPolicy(policy); err != nil {
		return err
	}
	if policy.Status == "" {
		policy.Status = "PUBLISHED"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.loadLocked()
	if err != nil {
		return err
	}
	for i := range state.Policies {
		if state.Policies[i].ID == policy.ID || state.Policies[i].Version == policy.Version {
			state.Policies[i] = policy
			return s.saveLocked(state)
		}
	}
	state.Policies = append(state.Policies, policy)
	return s.saveLocked(state)
}

func currentPublishedPersonalPointPolicy(state *personalPointState, now time.Time) (PointExpiryPolicy, error) {
	if state == nil {
		return PointExpiryPolicy{}, ErrInvalidPointCommand
	}
	now = pointNow(now)
	policies := append([]PointExpiryPolicy(nil), state.Policies...)
	sort.SliceStable(policies, func(i, j int) bool { return policies[i].Version > policies[j].Version })
	for _, policy := range policies {
		if policy.Status != "PUBLISHED" || (!policy.EffectiveFrom.IsZero() && policy.EffectiveFrom.After(now)) || (!policy.EffectiveTo.IsZero() && !policy.EffectiveTo.After(now)) {
			continue
		}
		if err := validatePointExpiryPolicy(policy); err != nil {
			continue
		}
		return policy, nil
	}
	return PointExpiryPolicy{}, ErrPointNotFound
}

func (s *JSONPersonalPointStore) currentPolicy(ctx context.Context) (PointExpiryPolicy, error) {
	state, err := s.readState(ctx)
	if err != nil {
		return PointExpiryPolicy{}, err
	}
	return currentPublishedPersonalPointPolicy(&state, time.Now().UTC())
}

func (s *JSONPersonalPointStore) publishPolicy(ctx context.Context, cmd PersonalPointPolicyPublishCommand) (PointExpiryPolicy, error) {
	var published PointExpiryPolicy
	err := s.withState(ctx, func(state *personalPointState) error {
		now := pointNow(cmd.PublishedAt)
		current, err := currentPublishedPersonalPointPolicy(state, now)
		if err != nil {
			return err
		}
		if current.Revision != cmd.ExpectedRevision {
			return ErrPointPolicyRevisionConflict
		}
		for i := range state.Policies {
			if state.Policies[i].ID == current.ID {
				state.Policies[i].Status = "ARCHIVED"
				state.Policies[i].EffectiveTo = now
				break
			}
		}
		published = PointExpiryPolicy{
			ID: "point_expiry_policy_v" + fmt.Sprint(current.Version+1), Version: current.Version + 1, Revision: current.Revision + 1,
			Enabled: cmd.Enabled, DurationValue: cmd.DurationValue, DurationUnit: "CALENDAR_MONTH", TimeZone: "Asia/Shanghai",
			SourceTypes:   []string{string(PointSourceRegistrationGift), string(PointSourceActivityGift), string(PointSourceAdminGift)},
			EffectiveFrom: now, Status: "PUBLISHED", CreatedBy: strings.TrimSpace(cmd.ActorID), ChangeReason: strings.TrimSpace(cmd.ChangeReason),
		}
		if err := validatePointExpiryPolicy(published); err != nil {
			return err
		}
		state.Policies = append(state.Policies, published)
		return nil
	})
	return published, err
}

func (s *JSONPersonalPointStore) listLots(ctx context.Context, accountID, userID string, filter PersonalPointLotFilter) ([]PersonalPointLot, error) {
	state, err := s.readState(ctx)
	if err != nil {
		return nil, err
	}
	account, err := findPersonalAccount(&state, accountID, userID)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return []PersonalPointLot{}, nil
	}
	items := make([]PersonalPointLot, 0)
	for _, lot := range state.Lots {
		if lot.AccountID != accountID || lot.UserID != userID {
			continue
		}
		if filter.Source != "" && lot.SourceType != filter.Source {
			continue
		}
		if strings.TrimSpace(filter.Status) != "" && !strings.EqualFold(lot.Status, filter.Status) {
			continue
		}
		items = append(items, lot)
	}
	sortLotsFEFO(items)
	start := filter.Offset
	if start > len(items) {
		start = len(items)
	}
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}
	return append([]PersonalPointLot(nil), items[start:end]...), nil
}

func personalPointSummaryFromState(state *personalPointState, accountID, userID string) (PersonalPointBalanceSummary, error) {
	account, err := findPersonalAccount(state, accountID, userID)
	if err != nil {
		return PersonalPointBalanceSummary{}, err
	}
	summary := PersonalPointBalanceSummary{PersonalPointBalance: PersonalPointBalance{AccountID: accountID, UserID: userID}}
	if account == nil {
		return summary, nil
	}
	summary.Available, summary.Frozen = account.AvailablePoints, account.FrozenPoints
	summary.Total = summary.Available + summary.Frozen
	for _, lot := range state.Lots {
		if lot.AccountID != accountID || lot.UserID != userID || lot.AvailablePoints <= 0 {
			continue
		}
		if lot.ExpiresAt.IsZero() {
			summary.PermanentAvailable += lot.AvailablePoints
			continue
		}
		summary.ExpiringAvailable += lot.AvailablePoints
		if summary.NextExpiryAt.IsZero() || lot.ExpiresAt.Before(summary.NextExpiryAt) {
			summary.NextExpiryAt = lot.ExpiresAt
			summary.NextExpiryPoints = lot.AvailablePoints
		} else if lot.ExpiresAt.Equal(summary.NextExpiryAt) {
			summary.NextExpiryPoints += lot.AvailablePoints
		}
	}
	return summary, nil
}

func (s *JSONPersonalPointStore) summary(ctx context.Context, accountID, userID string, now time.Time) (PersonalPointBalanceSummary, error) {
	var summary PersonalPointBalanceSummary
	err := s.withState(ctx, func(state *personalPointState) error {
		if err := expirePersonalPointState(state, accountID, userID, now); err != nil {
			return err
		}
		var err error
		summary, err = personalPointSummaryFromState(state, accountID, userID)
		return err
	})
	return summary, err
}

func (s *JSONPersonalPointStore) expireDue(ctx context.Context, now time.Time, limit int) (PersonalPointExpiryBatchResult, error) {
	result := PersonalPointExpiryBatchResult{}
	err := s.withState(ctx, func(state *personalPointState) error {
		now = pointNow(now)
		owners := map[string]string{}
		for _, lot := range state.Lots {
			if lot.AvailablePoints > 0 && !lot.ExpiresAt.IsZero() && !lot.ExpiresAt.After(now) {
				owners[lot.AccountID] = lot.UserID
			}
		}
		accountIDs := make([]string, 0, len(owners))
		for accountID := range owners {
			accountIDs = append(accountIDs, accountID)
		}
		sort.Strings(accountIDs)
		if len(accountIDs) > limit {
			accountIDs = accountIDs[:limit]
		}
		for _, accountID := range accountIDs {
			account, err := findPersonalAccount(state, accountID, owners[accountID])
			if err != nil {
				return err
			}
			before := int64(0)
			if account != nil {
				before = account.AvailablePoints
			}
			if err := expirePersonalPointState(state, accountID, owners[accountID], now); err != nil {
				return err
			}
			if account != nil && account.AvailablePoints < before {
				result.AccountsProcessed++
				result.PointsExpired += before - account.AvailablePoints
			}
		}
		return nil
	})
	return result, err
}

func (s *JSONPersonalPointStore) withState(ctx context.Context, fn func(*personalPointState) error) error {
	if err := s.operationalError(); err != nil {
		return err
	}
	if s.owner != nil {
		return s.owner.updateWithPersonalPoints(ctx, func(_ *platformData, points *JSONPersonalPointStore) error {
			return fn(points.memory)
		})
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.loadLocked()
	if err != nil {
		return err
	}
	if err := fn(&state); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.saveLocked(state)
}

func (s *JSONPersonalPointStore) readState(ctx context.Context) (personalPointState, error) {
	if err := s.operationalError(); err != nil {
		return personalPointState{}, err
	}
	if s.owner != nil {
		var result personalPointState
		err := s.owner.updateWithPersonalPoints(ctx, func(_ *platformData, points *JSONPersonalPointStore) error {
			result = clonePersonalPointState(*points.memory)
			return nil
		})
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return personalPointState{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

func normalizePersonalPointState(state *personalPointState) {
	if state.Accounts == nil {
		state.Accounts = []PersonalPointAccount{}
	}
	if state.Lots == nil {
		state.Lots = []PersonalPointLot{}
	}
	if state.Reservations == nil {
		state.Reservations = []PersonalPointReservation{}
	}
	if state.Allocations == nil {
		state.Allocations = []PersonalPointAllocation{}
	}
	if state.Movements == nil {
		state.Movements = []PersonalPointLotMovement{}
	}
	if state.WalletLedger == nil {
		state.WalletLedger = []PersonalPointWalletLedgerEntry{}
	}
	if state.Operations == nil {
		state.Operations = []personalPointOperation{}
	}
	if len(state.Policies) == 0 {
		state.Policies = []PointExpiryPolicy{defaultPersonalPointPolicy()}
	}
}

func clonePersonalPointState(state personalPointState) personalPointState {
	raw, _ := json.Marshal(state)
	var cloned personalPointState
	_ = json.Unmarshal(raw, &cloned)
	normalizePersonalPointState(&cloned)
	return cloned
}

func personalPointStateEmpty(state personalPointState) bool {
	return len(state.Accounts) == 0 && len(state.Lots) == 0 && len(state.Reservations) == 0 && len(state.Allocations) == 0 && len(state.Movements) == 0 && len(state.WalletLedger) == 0 && len(state.Operations) == 0 && len(state.Policies) == 0
}

func validatePersonalPointState(state *personalPointState) error {
	if state == nil {
		return ErrInvalidPointCommand
	}
	normalizePersonalPointState(state)
	accounts := make(map[string]PersonalPointAccount, len(state.Accounts))
	for _, account := range state.Accounts {
		if account.ID == "" || account.UserID == "" || account.AvailablePoints < 0 || account.FrozenPoints < 0 || account.TotalGranted < 0 || account.TotalConsumed < 0 || account.TotalExpired < 0 {
			return ErrInvalidPointCommand
		}
		if _, exists := accounts[account.ID]; exists {
			return ErrPersonalPointImportConflict
		}
		accounts[account.ID] = account
	}
	availableByAccount := map[string]int64{}
	reservedByAccount := map[string]int64{}
	lots := make(map[string]PersonalPointLot, len(state.Lots))
	for _, lot := range state.Lots {
		account, ok := accounts[lot.AccountID]
		if !ok || account.UserID != lot.UserID || lot.ID == "" || lot.OriginalPoints < 0 || lot.AvailablePoints < 0 || lot.ReservedPoints < 0 || lot.ConsumedPoints < 0 || lot.ExpiredPoints < 0 || lot.ReversedPoints < 0 {
			return ErrPersonalPointImportConflict
		}
		if lot.OriginalPoints != lot.AvailablePoints+lot.ReservedPoints+lot.ConsumedPoints+lot.ExpiredPoints+lot.ReversedPoints {
			return ErrPersonalPointImportConflict
		}
		if lot.SourceType != PointSourceLegacy && !isKnownPointSource(lot.SourceType) {
			return ErrPersonalPointImportConflict
		}
		if _, exists := lots[lot.ID]; exists {
			return ErrPersonalPointImportConflict
		}
		lots[lot.ID] = lot
		availableByAccount[lot.AccountID] += lot.AvailablePoints
		reservedByAccount[lot.AccountID] += lot.ReservedPoints
	}
	for id, account := range accounts {
		if availableByAccount[id] != account.AvailablePoints || reservedByAccount[id] != account.FrozenPoints {
			return ErrPersonalPointImportConflict
		}
	}
	reservations := make(map[string]PersonalPointReservation, len(state.Reservations))
	for _, reservation := range state.Reservations {
		account, ok := accounts[reservation.AccountID]
		if !ok || account.UserID != reservation.UserID || reservation.ID == "" || reservation.RequestedPoints < 0 || reservation.ReservedPoints < 0 || reservation.CapturedPoints < 0 || reservation.ReleasedPoints < 0 || reservation.ExpiredPoints < 0 || reservation.RequestedPoints != reservation.ReservedPoints+reservation.CapturedPoints+reservation.ReleasedPoints+reservation.ExpiredPoints {
			return ErrPersonalPointImportConflict
		}
		if _, exists := reservations[reservation.ID]; exists {
			return ErrPersonalPointImportConflict
		}
		reservations[reservation.ID] = reservation
	}
	for _, allocation := range state.Allocations {
		reservation, ok := reservations[allocation.ReservationID]
		lot, lotOK := lots[allocation.LotID]
		if !ok || !lotOK || allocation.ID == "" || allocation.AccountID != reservation.AccountID || allocation.UserID != reservation.UserID || allocation.AccountID != lot.AccountID || allocation.UserID != lot.UserID || allocation.AllocatedPoints < 0 || allocation.ReservedPoints < 0 || allocation.CapturedPoints < 0 || allocation.ReleasedPoints < 0 || allocation.ExpiredPoints < 0 || allocation.AllocatedPoints != allocation.ReservedPoints+allocation.CapturedPoints+allocation.ReleasedPoints+allocation.ExpiredPoints {
			return ErrPersonalPointImportConflict
		}
	}
	for _, entry := range state.WalletLedger {
		account, ok := accounts[entry.AccountID]
		if !ok || account.UserID != entry.UserID || entry.ID == "" || entry.IdempotencyKey == "" {
			return ErrPersonalPointImportConflict
		}
		if err := validatePersonalWalletTransition(entry.EntryType, entry.Points, entry.AvailableBefore, entry.AvailableAfter, entry.FrozenBefore, entry.FrozenAfter); err != nil {
			return ErrPersonalPointImportConflict
		}
	}
	for _, policy := range state.Policies {
		if err := validatePointExpiryPolicy(policy); err != nil {
			return ErrPersonalPointImportConflict
		}
	}
	return nil
}

func sidecarChecksum(raw []byte) string {
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%x", sum[:])
}

func validatePersonalPointJSONProjection(data *platformData) error {
	if data == nil {
		return ErrInvalidPointCommand
	}
	embeddedAccounts := make(map[string]struct{}, len(data.PersonalPoints.Accounts))
	for _, account := range data.PersonalPoints.Accounts {
		embeddedAccounts[account.ID] = struct{}{}
		found := false
		for _, projected := range data.PointAccounts {
			if projected.ID != account.ID {
				continue
			}
			if projected.UserID != account.UserID || int64(projected.Available) != account.AvailablePoints || int64(projected.Frozen) != account.FrozenPoints {
				return ErrPersonalPointImportConflict
			}
			found = true
			break
		}
		if !found {
			return ErrPersonalPointImportConflict
		}
	}
	for _, projected := range data.PointAccounts {
		if _, ok := embeddedAccounts[projected.ID]; !ok && (projected.Available != 0 || projected.Frozen != 0) {
			return ErrPersonalPointImportConflict
		}
	}
	personalLedgerIDs := make(map[string]struct{}, len(data.PersonalPoints.WalletLedger))
	for _, entry := range data.PersonalPoints.WalletLedger {
		personalLedgerIDs[entry.ID] = struct{}{}
	}
	legacyEntries := make([]walletLedgerEntry, 0, len(data.WalletLedger))
	for _, entry := range data.WalletLedger {
		if _, projected := personalLedgerIDs[entry.ID]; !projected {
			legacyEntries = append(legacyEntries, entry)
		}
	}
	before, _ := json.Marshal(data.PersonalPoints)
	candidate := clonePersonalPointState(data.PersonalPoints)
	if err := migrateLegacyWalletLedgerState(&candidate, legacyEntries, data.PointAccounts); err != nil {
		return ErrPersonalPointImportConflict
	}
	after, _ := json.Marshal(candidate)
	if string(before) != string(after) {
		return ErrPersonalPointImportConflict
	}
	return nil
}

func (s *jsonStore) preparePersonalPoints(data *platformData) error {
	if s == nil || data == nil {
		return ErrInvalidPointCommand
	}
	sidecarPath := s.path + ".personal-points.json"
	if data.PersonalPointImport.Version != 0 {
		if data.PersonalPointImport.Version != 1 {
			return ErrPersonalPointImportConflict
		}
		if data.PersonalPointImport.SidecarChecksum != "" {
			raw, err := os.ReadFile(sidecarPath)
			if err == nil && sidecarChecksum(raw) != data.PersonalPointImport.SidecarChecksum {
				return ErrPersonalPointImportConflict
			}
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		normalizePersonalPointState(&data.PersonalPoints)
		if err := validatePersonalPointState(&data.PersonalPoints); err != nil {
			return err
		}
		return validatePersonalPointJSONProjection(data)
	}
	if !personalPointStateEmpty(data.PersonalPoints) {
		return ErrPersonalPointImportConflict
	}
	candidate := personalPointState{}
	sidecarRaw, err := os.ReadFile(sidecarPath)
	if err == nil {
		if len(sidecarRaw) == 0 || json.Unmarshal(sidecarRaw, &candidate) != nil {
			return ErrPersonalPointImportConflict
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	normalizePersonalPointState(&candidate)
	memoryStore := &JSONPersonalPointStore{memory: &candidate}
	if err := memoryStore.importLegacyProjection(data.PointAccounts, data.WalletLedger); err != nil {
		return err
	}
	candidate = clonePersonalPointState(*memoryStore.memory)
	if err := validatePersonalPointState(&candidate); err != nil {
		return err
	}
	data.PersonalPoints = candidate
	data.PersonalPointImport = personalPointImportState{Version: 1, ImportedAt: time.Now().UTC()}
	if err == nil && len(sidecarRaw) > 0 {
		data.PersonalPointImport.SidecarChecksum = sidecarChecksum(sidecarRaw)
	}
	return nil
}

func syncPersonalPointJSONProjections(data *platformData) error {
	if data == nil {
		return ErrInvalidPointCommand
	}
	for _, account := range data.PersonalPoints.Accounts {
		if account.AvailablePoints > int64(^uint(0)>>1) || account.FrozenPoints > int64(^uint(0)>>1) || account.TotalGranted > int64(^uint(0)>>1) || account.TotalConsumed > int64(^uint(0)>>1) {
			return ErrInvalidPointCommand
		}
		found := false
		for i := range data.PointAccounts {
			if data.PointAccounts[i].ID != account.ID {
				continue
			}
			if data.PointAccounts[i].UserID != account.UserID {
				return ErrPointOwnership
			}
			data.PointAccounts[i].Available = int(account.AvailablePoints)
			data.PointAccounts[i].Frozen = int(account.FrozenPoints)
			data.PointAccounts[i].TotalGranted = int(account.TotalGranted)
			data.PointAccounts[i].TotalUsed = int(account.TotalConsumed)
			found = true
			break
		}
		if !found {
			data.PointAccounts = append(data.PointAccounts, adminPointAccount{ID: account.ID, UserID: account.UserID, Available: int(account.AvailablePoints), Frozen: int(account.FrozenPoints), TotalGranted: int(account.TotalGranted), TotalUsed: int(account.TotalConsumed)})
		}
	}
	ledgerIndex := make(map[string]int, len(data.WalletLedger))
	for i := range data.WalletLedger {
		ledgerIndex[data.WalletLedger[i].ID] = i
	}
	for _, entry := range data.PersonalPoints.WalletLedger {
		projected := walletLedgerEntry{ID: entry.ID, AccountID: entry.AccountID, UserID: entry.UserID, TenantID: entry.TenantID, TaskID: entry.TaskID, BillingEventID: entry.BillingEventID, EntryType: entry.EntryType, Points: float64(entry.Points), AvailableBefore: float64(entry.AvailableBefore), AvailableAfter: float64(entry.AvailableAfter), FrozenBefore: float64(entry.FrozenBefore), FrozenAfter: float64(entry.FrozenAfter), IdempotencyKey: entry.IdempotencyKey, ReferenceType: entry.ReferenceType, ReferenceID: entry.ReferenceID, Remark: entry.Remark, Metadata: cloneAnyMap(entry.Metadata), CreatedAt: entry.CreatedAt.UTC().Format(time.RFC3339Nano)}
		if index, ok := ledgerIndex[entry.ID]; ok {
			data.WalletLedger[index] = projected
		} else {
			ledgerIndex[entry.ID] = len(data.WalletLedger)
			data.WalletLedger = append(data.WalletLedger, projected)
		}
	}
	return nil
}

func (s *jsonStore) updateWithPersonalPoints(ctx context.Context, mutator func(*platformData, *JSONPersonalPointStore) error) error {
	if s == nil || mutator == nil {
		return ErrInvalidPointCommand
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.update(func(data *platformData) error {
		if err := s.preparePersonalPoints(data); err != nil {
			return err
		}
		state := clonePersonalPointState(data.PersonalPoints)
		points := &JSONPersonalPointStore{memory: &state}
		if err := mutator(data, points); err != nil {
			return err
		}
		data.PersonalPoints = clonePersonalPointState(*points.memory)
		if err := validatePersonalPointState(&data.PersonalPoints); err != nil {
			return err
		}
		return syncPersonalPointJSONProjections(data)
	})
}

func findPersonalAccount(state *personalPointState, accountID, userID string) (*PersonalPointAccount, error) {
	for i := range state.Accounts {
		if state.Accounts[i].ID != accountID {
			continue
		}
		if state.Accounts[i].UserID != userID {
			return nil, ErrPointOwnership
		}
		return &state.Accounts[i], nil
	}
	return nil, nil
}

func personalPointAccountForUserState(state *personalPointState, userID string) (*PersonalPointAccount, error) {
	var found *PersonalPointAccount
	for i := range state.Accounts {
		if state.Accounts[i].UserID != userID {
			continue
		}
		if found != nil {
			return nil, ErrPointOwnership
		}
		found = &state.Accounts[i]
	}
	if found == nil {
		return nil, ErrInsufficientPoints
	}
	return found, nil
}

func ensurePersonalAccount(state *personalPointState, accountID, userID string) (*PersonalPointAccount, error) {
	account, err := findPersonalAccount(state, accountID, userID)
	if err != nil {
		return nil, err
	}
	if account != nil {
		return account, nil
	}
	state.Accounts = append(state.Accounts, PersonalPointAccount{ID: accountID, UserID: userID})
	return &state.Accounts[len(state.Accounts)-1], nil
}

func currentPersonalPointPolicy(state *personalPointState, source PointSource, now time.Time) (PointExpiryPolicy, error) {
	policies := append([]PointExpiryPolicy(nil), state.Policies...)
	sort.SliceStable(policies, func(i, j int) bool { return policies[i].Version > policies[j].Version })
	now = pointNow(now)
	for _, policy := range policies {
		if policy.Status != "PUBLISHED" {
			continue
		}
		if !policy.EffectiveFrom.IsZero() && policy.EffectiveFrom.After(now) {
			continue
		}
		if !policy.EffectiveTo.IsZero() && !policy.EffectiveTo.After(now) {
			continue
		}
		if err := validatePointExpiryPolicy(policy); err != nil {
			continue
		}
		for _, allowed := range policy.SourceTypes {
			if allowed == string(source) {
				return policy, nil
			}
		}
	}
	return PointExpiryPolicy{}, fmt.Errorf("point expiry policy unavailable for %s", source)
}

func appendPersonalMovement(state *personalPointState, lot PersonalPointLot, movementType string, points int64, before PersonalPointLot, reservationID, key string, now time.Time) {
	state.Movements = append(state.Movements, PersonalPointLotMovement{
		ID: stablePointID("movement", lot.AccountID, key), LotID: lot.ID, AccountID: lot.AccountID, UserID: lot.UserID,
		MovementType: movementType, Points: points, AvailableBefore: before.AvailablePoints, AvailableAfter: lot.AvailablePoints,
		ReservedBefore: before.ReservedPoints, ReservedAfter: lot.ReservedPoints, ConsumedBefore: before.ConsumedPoints,
		ConsumedAfter: lot.ConsumedPoints, ExpiredBefore: before.ExpiredPoints, ExpiredAfter: lot.ExpiredPoints,
		ReversedBefore: before.ReversedPoints, ReversedAfter: lot.ReversedPoints,
		ReferenceType: lot.ReferenceType, ReferenceID: lot.ReferenceID, ReservationID: reservationID, IdempotencyKey: key, CreatedAt: now,
	})
}

func personalPointAccountForMergeState(state *personalPointState, userID string) (*PersonalPointAccount, error) {
	var found *PersonalPointAccount
	for i := range state.Accounts {
		if state.Accounts[i].UserID != userID {
			continue
		}
		if found != nil {
			return nil, ErrPointOwnership
		}
		found = &state.Accounts[i]
	}
	return found, nil
}

func mergePersonalPointState(state *personalPointState, targetUserID, sourceUserID, mergeID string, now time.Time) (personalPointMergeResult, error) {
	result := personalPointMergeResult{}
	targetUserID, sourceUserID, mergeID = strings.TrimSpace(targetUserID), strings.TrimSpace(sourceUserID), strings.TrimSpace(mergeID)
	if state == nil || targetUserID == "" || sourceUserID == "" || targetUserID == sourceUserID || mergeID == "" {
		return result, ErrInvalidPointCommand
	}
	candidate := clonePersonalPointState(*state)
	if err := validatePersonalPointState(&candidate); err != nil {
		return result, err
	}
	target, err := personalPointAccountForMergeState(&candidate, targetUserID)
	if err != nil {
		return result, err
	}
	source, err := personalPointAccountForMergeState(&candidate, sourceUserID)
	if err != nil {
		return result, err
	}
	if (source != nil && source.FrozenPoints > 0) || (target != nil && target.FrozenPoints > 0) {
		return result, ErrPersonalPointMergeActiveReservation
	}
	for _, reservation := range candidate.Reservations {
		if (reservation.UserID == targetUserID || reservation.UserID == sourceUserID) && reservation.ReservedPoints > 0 {
			return result, ErrPersonalPointMergeActiveReservation
		}
	}
	if source == nil {
		return result, nil
	}
	now = pointNow(now)
	if target != nil {
		if err := expirePersonalPointState(&candidate, target.ID, target.UserID, now); err != nil {
			return result, err
		}
	}
	if err := expirePersonalPointState(&candidate, source.ID, source.UserID, now); err != nil {
		return result, err
	}
	// Expiry can reallocate the backing slices, so resolve both account pointers again.
	target, err = personalPointAccountForMergeState(&candidate, targetUserID)
	if err != nil {
		return result, err
	}
	source, err = personalPointAccountForMergeState(&candidate, sourceUserID)
	if err != nil || source == nil {
		return result, err
	}
	if target == nil {
		candidate.Accounts = append(candidate.Accounts, PersonalPointAccount{ID: stablePointID("account", targetUserID, "auth-merge:"+mergeID), UserID: targetUserID})
		target = &candidate.Accounts[len(candidate.Accounts)-1]
		// Appending can reallocate candidate.Accounts and invalidate source.
		source, err = personalPointAccountForMergeState(&candidate, sourceUserID)
		if err != nil || source == nil {
			return result, err
		}
	}
	targetBefore, sourceBefore := target.AvailablePoints, source.AvailablePoints
	for i := range candidate.Lots {
		lot := &candidate.Lots[i]
		if lot.AccountID != source.ID || lot.UserID != sourceUserID || lot.AvailablePoints <= 0 {
			continue
		}
		amount := lot.AvailablePoints
		sourceLotID := lot.ID
		before := *lot
		lot.AvailablePoints = 0
		lot.ReversedPoints += amount
		setPointLotStatus(lot)
		appendPersonalMovement(&candidate, *lot, "REVERSE", amount, before, "", "auth-merge:reverse:"+mergeID+":"+sourceLotID, now)

		transferKey := "auth-merge:" + mergeID + ":" + sourceLotID
		transferred := before
		transferred.ID = stablePointID("lot", target.ID, transferKey)
		transferred.AccountID = target.ID
		transferred.UserID = targetUserID
		transferred.OriginalPoints = amount
		transferred.AvailablePoints = amount
		transferred.ReservedPoints = 0
		transferred.ConsumedPoints = 0
		transferred.ExpiredPoints = 0
		transferred.ReversedPoints = 0
		transferred.IdempotencyKey = transferKey
		setPointLotStatus(&transferred)
		candidate.Lots = append(candidate.Lots, transferred)
		appendPersonalMovement(&candidate, transferred, "OPENING", amount, PersonalPointLot{}, "", "auth-merge:opening:"+mergeID+":"+sourceLotID, now)
		result.PointsMoved += amount
	}
	source.AvailablePoints -= result.PointsMoved
	source.TotalReversed += result.PointsMoved
	target.AvailablePoints += result.PointsMoved
	target.TotalGranted += result.PointsMoved
	if result.PointsMoved > 0 {
		if err := appendPersonalWalletLedger(&candidate, *source, "ADJUSTMENT", result.PointsMoved, sourceBefore, source.FrozenPoints, personalWalletKey(source.ID, "auth-merge-out", mergeID), "AUTH_MERGE", mergeID, map[string]any{"direction": "OUT", "target_user_id": targetUserID}, now); err != nil {
			return personalPointMergeResult{}, err
		}
		if err := appendPersonalWalletLedger(&candidate, *target, "ADJUSTMENT", result.PointsMoved, targetBefore, target.FrozenPoints, personalWalletKey(target.ID, "auth-merge-in", mergeID), "AUTH_MERGE", mergeID, map[string]any{"direction": "IN", "source_user_id": sourceUserID}, now); err != nil {
			return personalPointMergeResult{}, err
		}
	}
	result.AccountsMoved = 1
	if err := validatePersonalPointState(&candidate); err != nil {
		return personalPointMergeResult{}, err
	}
	*state = candidate
	return result, nil
}

func findPersonalLot(state *personalPointState, id, accountID, userID string) (*PersonalPointLot, error) {
	for i := range state.Lots {
		if state.Lots[i].ID != id {
			continue
		}
		if err := validateOwned(accountID, userID, state.Lots[i].AccountID, state.Lots[i].UserID); err != nil {
			return nil, err
		}
		return &state.Lots[i], nil
	}
	return nil, ErrPointNotFound
}

func findPersonalReservation(state *personalPointState, id, accountID, userID string) (*PersonalPointReservation, error) {
	for i := range state.Reservations {
		if state.Reservations[i].ID != id {
			continue
		}
		if err := validateOwned(accountID, userID, state.Reservations[i].AccountID, state.Reservations[i].UserID); err != nil {
			return nil, err
		}
		return &state.Reservations[i], nil
	}
	return nil, ErrPointNotFound
}

func operationFor(state *personalPointState, kind, accountID, userID, key, fingerprint string) (*personalPointOperation, error) {
	for i := range state.Operations {
		op := &state.Operations[i]
		if op.Kind != kind || op.AccountID != accountID || op.UserID != userID || op.IdempotencyKey != key {
			continue
		}
		if op.Fingerprint != fingerprint {
			return nil, ErrIdempotencyConflict
		}
		return op, nil
	}
	return nil, nil
}

func appendOperation(state *personalPointState, kind, accountID, userID, key, fingerprint, reservationID string, points int64, now time.Time) {
	state.Operations = append(state.Operations, personalPointOperation{Kind: kind, AccountID: accountID, UserID: userID, IdempotencyKey: key, Fingerprint: fingerprint, ReservationID: reservationID, Points: points, CreatedAt: now})
}

func (s *JSONPersonalPointStore) grant(ctx context.Context, cmd PersonalPointGrantCommand) (result PersonalPointGrantResult, err error) {
	if err = normalizePointCommand(cmd); err != nil {
		return result, err
	}
	fingerprint := pointCommandFingerprint(cmd)
	cmd.GrantedAt = pointNow(cmd.GrantedAt)
	err = s.withState(ctx, func(state *personalPointState) error {
		op, opErr := operationFor(state, "GRANT", cmd.AccountID, cmd.UserID, cmd.IdempotencyKey, fingerprint)
		if opErr != nil {
			return opErr
		}
		if op != nil {
			for _, lot := range state.Lots {
				if lot.ID == op.ReservationID {
					result = PersonalPointGrantResult{Lot: lot, Idempotent: true}
					return nil
				}
			}
			return ErrPointNotFound
		}
		for _, lot := range state.Lots {
			if lot.AccountID == cmd.AccountID && lot.IdempotencyKey == cmd.IdempotencyKey {
				if lot.UserID != cmd.UserID {
					return ErrPointOwnership
				}
				if lot.SourceType != cmd.Source || lot.OriginalPoints != cmd.Points || lot.ReferenceType != cmd.ReferenceType || lot.ReferenceID != cmd.ReferenceID || !lot.GrantedAt.Equal(cmd.GrantedAt) {
					return ErrIdempotencyConflict
				}
				result = PersonalPointGrantResult{Lot: lot, Idempotent: true}
				return nil
			}
		}
		account, accountErr := ensurePersonalAccount(state, cmd.AccountID, cmd.UserID)
		if accountErr != nil {
			return accountErr
		}
		lot := PersonalPointLot{ID: stablePointID("lot", cmd.AccountID, cmd.IdempotencyKey), AccountID: cmd.AccountID, UserID: cmd.UserID, SourceType: cmd.Source, ReferenceType: cmd.ReferenceType, ReferenceID: cmd.ReferenceID, OriginalPoints: cmd.Points, AvailablePoints: cmd.Points, GrantedAt: cmd.GrantedAt, IdempotencyKey: cmd.IdempotencyKey, Status: "ACTIVE"}
		if isGiftPointSource(cmd.Source) {
			policy, policyErr := currentPersonalPointPolicy(state, cmd.Source, time.Now().UTC())
			if policyErr != nil {
				return policyErr
			}
			lot.PolicyVersionID, lot.PolicySnapshot = policy.ID, PointPolicySnapshot{Version: policy.Version, Enabled: policy.Enabled, DurationValue: policy.DurationValue, DurationUnit: policy.DurationUnit, TimeZone: policy.TimeZone}
			if policy.Enabled {
				expiry, expiryErr := addCalendarMonthsClamp(cmd.GrantedAt, policy.DurationValue, policy.TimeZone)
				if expiryErr != nil {
					return expiryErr
				}
				lot.ExpiresAt = expiry
			}
		}
		state.Lots = append(state.Lots, lot)
		beforeAvailable, beforeFrozen := account.AvailablePoints, account.FrozenPoints
		account.AvailablePoints += cmd.Points
		account.TotalGranted += cmd.Points
		appendPersonalMovement(state, lot, "OPENING", cmd.Points, PersonalPointLot{AvailablePoints: 0}, "", "grant:"+cmd.IdempotencyKey, cmd.GrantedAt)
		entryType := "GRANT"
		if cmd.Source == PointSourceRecharge {
			entryType = "RECHARGE"
		}
		if err := appendPersonalWalletLedger(state, *account, entryType, cmd.Points, beforeAvailable, beforeFrozen, personalWalletKey(cmd.AccountID, "grant", cmd.IdempotencyKey), cmd.ReferenceType, cmd.ReferenceID, map[string]any{"fingerprint": fingerprint, "source_type": string(cmd.Source)}, cmd.GrantedAt); err != nil {
			return err
		}
		appendOperation(state, "GRANT", cmd.AccountID, cmd.UserID, cmd.IdempotencyKey, fingerprint, lot.ID, 0, cmd.GrantedAt)
		result = PersonalPointGrantResult{Lot: lot}
		return nil
	})
	return result, err
}

func (s *JSONPersonalPointStore) grantRegistration(ctx context.Context, cmd PersonalPointRegistrationGrantCommand) (PersonalPointGrantResult, error) {
	if cmd.PlanGrantPoints <= 0 {
		return PersonalPointGrantResult{}, ErrInvalidPointCommand
	}
	return s.grant(ctx, PersonalPointGrantCommand{AccountID: cmd.AccountID, UserID: cmd.UserID, Source: PointSourceRegistrationGift, Points: cmd.PlanGrantPoints, ReferenceType: "PLAN", ReferenceID: cmd.PlanID, IdempotencyKey: cmd.IdempotencyKey, GrantedAt: cmd.GrantedAt})
}

func (s *JSONPersonalPointStore) getBalance(ctx context.Context, accountID, userID string) (PersonalPointBalance, error) {
	if accountID == "" || userID == "" {
		return PersonalPointBalance{}, ErrInvalidPointCommand
	}
	var balance PersonalPointBalance
	err := s.withState(ctx, func(state *personalPointState) error {
		if err := expirePersonalPointState(state, accountID, userID, time.Now().UTC()); err != nil {
			return err
		}
		account, err := findPersonalAccount(state, accountID, userID)
		if err != nil {
			return err
		}
		balance = PersonalPointBalance{AccountID: accountID, UserID: userID}
		if account != nil {
			balance.Available, balance.Frozen = account.AvailablePoints, account.FrozenPoints
			balance.Total = balance.Available + balance.Frozen
		}
		return nil
	})
	return balance, err
}

func (s *JSONPersonalPointStore) reserve(ctx context.Context, cmd PersonalPointReserveCommand) (result PersonalPointReserveResult, err error) {
	if cmd.AccountID == "" || cmd.UserID == "" || cmd.BusinessType == "" || cmd.BusinessID == "" || cmd.IdempotencyKey == "" || cmd.RequestedPoints <= 0 {
		return result, ErrInvalidPointCommand
	}
	fingerprint := pointCommandFingerprint(cmd)
	cmd.ReservedAt = pointNow(cmd.ReservedAt)
	err = s.withState(ctx, func(state *personalPointState) error {
		op, opErr := operationFor(state, "RESERVE", cmd.AccountID, cmd.UserID, cmd.IdempotencyKey, fingerprint)
		if opErr != nil {
			return opErr
		}
		if op != nil {
			return populateReserveResult(state, op.ReservationID, &result, true)
		}
		for i := range state.Reservations {
			r := state.Reservations[i]
			if r.AccountID == cmd.AccountID && r.IdempotencyKey == cmd.IdempotencyKey {
				if r.UserID != cmd.UserID || r.BusinessType != cmd.BusinessType || r.BusinessID != cmd.BusinessID || r.RequestedPoints != cmd.RequestedPoints {
					return ErrIdempotencyConflict
				}
				return populateReserveResult(state, r.ID, &result, true)
			}
			if r.AccountID == cmd.AccountID && r.BusinessType == cmd.BusinessType && r.BusinessID == cmd.BusinessID && r.IdempotencyKey != cmd.IdempotencyKey {
				return ErrIdempotencyConflict
			}
		}
		account, accountErr := findPersonalAccount(state, cmd.AccountID, cmd.UserID)
		if accountErr != nil {
			return accountErr
		}
		if account == nil {
			return ErrInsufficientPoints
		}
		if err := expirePersonalPointState(state, cmd.AccountID, cmd.UserID, cmd.ReservedAt); err != nil {
			return err
		}
		if account.AvailablePoints < cmd.RequestedPoints {
			return ErrInsufficientPoints
		}
		reservation := PersonalPointReservation{ID: stablePointID("reservation", cmd.AccountID, cmd.IdempotencyKey), AccountID: cmd.AccountID, UserID: cmd.UserID, BusinessType: cmd.BusinessType, BusinessID: cmd.BusinessID, RequestedPoints: cmd.RequestedPoints, ReservedPoints: cmd.RequestedPoints, IdempotencyKey: cmd.IdempotencyKey, CreatedAt: cmd.ReservedAt, UpdatedAt: cmd.ReservedAt, Status: "RESERVED"}
		remaining := cmd.RequestedPoints
		lots := make([]PersonalPointLot, 0)
		for _, lot := range state.Lots {
			if lot.AccountID == cmd.AccountID && lot.UserID == cmd.UserID && lot.AvailablePoints > 0 && (lot.Status == "ACTIVE" || lot.Status == "LEGACY") {
				lots = append(lots, lot)
			}
		}
		sortLotsFEFO(lots)
		for _, selected := range lots {
			if remaining == 0 {
				break
			}
			amount := selected.AvailablePoints
			if amount > remaining {
				amount = remaining
			}
			lot, lotErr := findPersonalLot(state, selected.ID, cmd.AccountID, cmd.UserID)
			if lotErr != nil {
				return lotErr
			}
			before := *lot
			lot.AvailablePoints -= amount
			lot.ReservedPoints += amount
			setPointLotStatus(lot)
			allocation := PersonalPointAllocation{ID: stablePointID("allocation", reservation.ID, lot.ID), ReservationID: reservation.ID, LotID: lot.ID, AccountID: cmd.AccountID, UserID: cmd.UserID, SourceType: lot.SourceType, AllocatedPoints: amount, ReservedPoints: amount, Status: "RESERVED"}
			state.Allocations = append(state.Allocations, allocation)
			appendPersonalMovement(state, *lot, "RESERVE", amount, before, reservation.ID, "reserve:"+cmd.IdempotencyKey+":"+lot.ID, cmd.ReservedAt)
			result.Allocations = append(result.Allocations, allocation)
			remaining -= amount
		}
		if remaining != 0 {
			return ErrInsufficientPoints
		}
		beforeAvailable, beforeFrozen := account.AvailablePoints, account.FrozenPoints
		account.AvailablePoints -= cmd.RequestedPoints
		account.FrozenPoints += cmd.RequestedPoints
		state.Reservations = append(state.Reservations, reservation)
		if err := appendPersonalWalletLedger(state, *account, "RESERVE", cmd.RequestedPoints, beforeAvailable, beforeFrozen, personalWalletKey(cmd.AccountID, "reserve", cmd.IdempotencyKey), cmd.BusinessType, cmd.BusinessID, map[string]any{"fingerprint": fingerprint, "business_type": cmd.BusinessType, "business_id": cmd.BusinessID}, cmd.ReservedAt); err != nil {
			return err
		}
		appendOperation(state, "RESERVE", cmd.AccountID, cmd.UserID, cmd.IdempotencyKey, fingerprint, reservation.ID, 0, cmd.ReservedAt)
		result.Reservation, result.Idempotent = reservation, false
		return nil
	})
	return result, err
}

func populateReserveResult(state *personalPointState, reservationID string, result *PersonalPointReserveResult, idempotent bool) error {
	for _, reservation := range state.Reservations {
		if reservation.ID == reservationID {
			result.Reservation = reservation
			result.Idempotent = idempotent
			for _, allocation := range state.Allocations {
				if allocation.ReservationID == reservationID {
					result.Allocations = append(result.Allocations, allocation)
				}
			}
			return nil
		}
	}
	return ErrPointNotFound
}

func (s *JSONPersonalPointStore) capture(ctx context.Context, cmd PersonalPointCaptureCommand) (result PersonalPointMutationResult, err error) {
	if cmd.AccountID == "" || cmd.UserID == "" || cmd.ReservationID == "" || cmd.IdempotencyKey == "" || cmd.Points <= 0 {
		return result, ErrInvalidPointCommand
	}
	fingerprint := pointCommandFingerprint(cmd)
	cmd.CapturedAt = pointNow(cmd.CapturedAt)
	err = s.withState(ctx, func(state *personalPointState) error {
		op, opErr := operationFor(state, "CAPTURE", cmd.AccountID, cmd.UserID, cmd.IdempotencyKey, fingerprint)
		if opErr != nil {
			return opErr
		}
		if op != nil {
			return populateMutationResult(state, op.ReservationID, &result, true)
		}
		reservation, reservationErr := findPersonalReservation(state, cmd.ReservationID, cmd.AccountID, cmd.UserID)
		if reservationErr != nil {
			return reservationErr
		}
		if reservation.ReservedPoints < cmd.Points {
			return ErrInsufficientPoints
		}
		remaining := cmd.Points
		for i := range state.Allocations {
			allocation := &state.Allocations[i]
			if allocation.ReservationID != reservation.ID || allocation.ReservedPoints <= 0 || remaining == 0 {
				continue
			}
			amount := allocation.ReservedPoints
			if amount > remaining {
				amount = remaining
			}
			lot, lotErr := findPersonalLot(state, allocation.LotID, cmd.AccountID, cmd.UserID)
			if lotErr != nil {
				return lotErr
			}
			before := *lot
			lot.ReservedPoints -= amount
			lot.ConsumedPoints += amount
			setPointLotStatus(lot)
			allocation.ReservedPoints -= amount
			allocation.CapturedPoints += amount
			setAllocationStatus(allocation)
			appendPersonalMovement(state, *lot, "CAPTURE", amount, before, reservation.ID, "capture:"+cmd.IdempotencyKey+":"+lot.ID, cmd.CapturedAt)
			remaining -= amount
		}
		if remaining != 0 {
			return ErrInsufficientPoints
		}
		reservation.ReservedPoints -= cmd.Points
		reservation.CapturedPoints += cmd.Points
		reservation.UpdatedAt = cmd.CapturedAt
		setReservationStatus(reservation)
		account, accountErr := findPersonalAccount(state, cmd.AccountID, cmd.UserID)
		if accountErr != nil {
			return accountErr
		}
		if account == nil {
			return ErrPointNotFound
		}
		beforeAvailable, beforeFrozen := account.AvailablePoints, account.FrozenPoints
		account.FrozenPoints -= cmd.Points
		account.TotalConsumed += cmd.Points
		if err := appendPersonalWalletLedger(state, *account, "CAPTURE", cmd.Points, beforeAvailable, beforeFrozen, personalWalletKey(cmd.AccountID, "capture", cmd.IdempotencyKey), reservation.BusinessType, reservation.BusinessID, map[string]any{"fingerprint": fingerprint, "reservation_id": reservation.ID, "business_type": reservation.BusinessType, "business_id": reservation.BusinessID}, cmd.CapturedAt); err != nil {
			return err
		}
		appendOperation(state, "CAPTURE", cmd.AccountID, cmd.UserID, cmd.IdempotencyKey, fingerprint, reservation.ID, cmd.Points, cmd.CapturedAt)
		return populateMutationResult(state, reservation.ID, &result, false)
	})
	return result, err
}

func (s *JSONPersonalPointStore) release(ctx context.Context, cmd PersonalPointReleaseCommand) (result PersonalPointMutationResult, err error) {
	if cmd.AccountID == "" || cmd.UserID == "" || cmd.ReservationID == "" || cmd.IdempotencyKey == "" {
		return result, ErrInvalidPointCommand
	}
	fingerprint := pointCommandFingerprint(cmd)
	cmd.ReleasedAt = pointNow(cmd.ReleasedAt)
	err = s.withState(ctx, func(state *personalPointState) error {
		op, opErr := operationFor(state, "RELEASE", cmd.AccountID, cmd.UserID, cmd.IdempotencyKey, fingerprint)
		if opErr != nil {
			return opErr
		}
		if op != nil {
			return populateMutationResult(state, op.ReservationID, &result, true)
		}
		reservation, reservationErr := findPersonalReservation(state, cmd.ReservationID, cmd.AccountID, cmd.UserID)
		if reservationErr != nil {
			return reservationErr
		}
		amountTotal := cmd.Points
		if amountTotal == 0 {
			amountTotal = reservation.ReservedPoints
		}
		if amountTotal <= 0 || reservation.ReservedPoints < amountTotal {
			return ErrInvalidPointCommand
		}
		remaining := amountTotal
		var expiredLots []struct {
			lot    PersonalPointLot
			amount int64
			key    string
		}
		for i := range state.Allocations {
			allocation := &state.Allocations[i]
			if allocation.ReservationID != reservation.ID || allocation.ReservedPoints <= 0 || remaining == 0 {
				continue
			}
			amount := allocation.ReservedPoints
			if amount > remaining {
				amount = remaining
			}
			lot, lotErr := findPersonalLot(state, allocation.LotID, cmd.AccountID, cmd.UserID)
			if lotErr != nil {
				return lotErr
			}
			before := *lot
			lot.ReservedPoints -= amount
			lot.AvailablePoints += amount
			setPointLotStatus(lot)
			allocation.ReservedPoints -= amount
			allocation.ReleasedPoints += amount
			setAllocationStatus(allocation)
			appendPersonalMovement(state, *lot, "RELEASE", amount, before, reservation.ID, "release:"+cmd.IdempotencyKey+":"+lot.ID, cmd.ReleasedAt)
			if !lot.ExpiresAt.IsZero() && !lot.ExpiresAt.After(cmd.ReleasedAt) && lot.AvailablePoints > 0 {
				expireAmount := lot.AvailablePoints
				expireBefore := *lot
				lot.AvailablePoints = 0
				lot.ExpiredPoints += expireAmount
				setPointLotStatus(lot)
				expireKey := personalWalletKey(cmd.AccountID, "expire", lot.ID+":"+reservation.ID+":"+cmd.IdempotencyKey)
				appendPersonalMovement(state, *lot, "EXPIRE", expireAmount, expireBefore, reservation.ID, "expire:"+lot.ID+":"+reservation.ID+":"+cmd.IdempotencyKey, cmd.ReleasedAt)
				expiredLots = append(expiredLots, struct {
					lot    PersonalPointLot
					amount int64
					key    string
				}{lot: *lot, amount: expireAmount, key: expireKey})
			}
			remaining -= amount
		}
		if remaining != 0 {
			return ErrInvalidPointCommand
		}
		reservation.ReservedPoints -= amountTotal
		reservation.ReleasedPoints += amountTotal
		reservation.UpdatedAt = cmd.ReleasedAt
		setReservationStatus(reservation)
		account, accountErr := findPersonalAccount(state, cmd.AccountID, cmd.UserID)
		if accountErr != nil {
			return accountErr
		}
		if account == nil {
			return ErrPointNotFound
		}
		beforeAvailable, beforeFrozen := account.AvailablePoints, account.FrozenPoints
		account.FrozenPoints -= amountTotal
		account.AvailablePoints += amountTotal
		if err := appendPersonalWalletLedger(state, *account, "RELEASE", amountTotal, beforeAvailable, beforeFrozen, personalWalletKey(cmd.AccountID, "release", cmd.IdempotencyKey), reservation.BusinessType, reservation.BusinessID, map[string]any{"fingerprint": fingerprint, "reservation_id": reservation.ID, "business_type": reservation.BusinessType, "business_id": reservation.BusinessID}, cmd.ReleasedAt); err != nil {
			return err
		}
		for _, expired := range expiredLots {
			beforeExpireAvailable := account.AvailablePoints
			account.AvailablePoints -= expired.amount
			account.TotalExpired += expired.amount
			if err := appendPersonalWalletLedger(state, *account, "EXPIRE", expired.amount, beforeExpireAvailable, account.FrozenPoints, expired.key, expired.lot.ReferenceType, expired.lot.ReferenceID, map[string]any{"source": "release_after_expiry", "source_type": string(expired.lot.SourceType), "reservation_id": reservation.ID, "business_type": reservation.BusinessType, "business_id": reservation.BusinessID}, cmd.ReleasedAt); err != nil {
				return err
			}
		}
		appendOperation(state, "RELEASE", cmd.AccountID, cmd.UserID, cmd.IdempotencyKey, fingerprint, reservation.ID, amountTotal, cmd.ReleasedAt)
		return populateMutationResult(state, reservation.ID, &result, false)
	})
	return result, err
}

func populateMutationResult(state *personalPointState, reservationID string, result *PersonalPointMutationResult, idempotent bool) error {
	for _, reservation := range state.Reservations {
		if reservation.ID == reservationID {
			result.Reservation = reservation
			result.Idempotent = idempotent
			for _, allocation := range state.Allocations {
				if allocation.ReservationID == reservationID {
					result.Allocations = append(result.Allocations, allocation)
				}
			}
			return nil
		}
	}
	return ErrPointNotFound
}

func expirePersonalPointState(state *personalPointState, accountID, userID string, now time.Time) error {
	now = pointNow(now)
	account, err := findPersonalAccount(state, accountID, userID)
	if err != nil {
		return err
	}
	if account == nil {
		return nil
	}
	for i := range state.Lots {
		lot := &state.Lots[i]
		if lot.AccountID != accountID || lot.UserID != userID || lot.ExpiresAt.IsZero() || lot.ExpiresAt.After(now) {
			continue
		}
		if lot.AvailablePoints > 0 {
			amount := lot.AvailablePoints
			before := *lot
			lot.AvailablePoints = 0
			lot.ExpiredPoints += amount
			setPointLotStatus(lot)
			beforeAvailable, beforeFrozen := account.AvailablePoints, account.FrozenPoints
			account.AvailablePoints -= amount
			account.TotalExpired += amount
			appendPersonalMovement(state, *lot, "EXPIRE", amount, before, "", "expire:"+lot.ID, now)
			if err := appendPersonalWalletLedger(state, *account, "EXPIRE", amount, beforeAvailable, beforeFrozen, personalWalletKey(accountID, "expire", lot.ID), lot.ReferenceType, lot.ReferenceID, map[string]any{"source": "scheduled_expiry", "source_type": string(lot.SourceType)}, now); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *JSONPersonalPointStore) expire(ctx context.Context, cmd PersonalPointExpiryCommand) error {
	return s.withState(ctx, func(state *personalPointState) error {
		return expirePersonalPointState(state, cmd.AccountID, cmd.UserID, cmd.Now)
	})
}

func (s *JSONPersonalPointStore) movementCount(ctx context.Context, accountID, userID, movementType string) int {
	state, err := s.readState(ctx)
	if err != nil {
		return 0
	}
	count := 0
	for _, movement := range state.Movements {
		if movement.AccountID == accountID && (userID == "" || movement.UserID == userID) && movement.MovementType == movementType {
			count++
		}
	}
	return count
}

func hasLegacyImportWalletLedger(state *personalPointState, accountID string) bool {
	for _, entry := range state.WalletLedger {
		if entry.AccountID != accountID || entry.EntryType != "GRANT" {
			continue
		}
		if entry.ReferenceType == "LEGACY_IMPORT" || boolValue(entry.Metadata["legacy_import"]) {
			return true
		}
	}
	return false
}

func (s *JSONPersonalPointStore) importLegacyProjection(accounts []adminPointAccount, ledger []walletLedgerEntry) error {
	if s == nil {
		return ErrInvalidPointCommand
	}
	return s.withState(context.Background(), func(state *personalPointState) error {
		if err := migrateLegacyWalletLedgerState(state, ledger, accounts); err != nil {
			return err
		}
		return importLegacyAccountsState(state, accounts)
	})
}

func (s *JSONPersonalPointStore) importLegacyAccounts(accounts []adminPointAccount) error {
	if s == nil {
		return ErrInvalidPointCommand
	}
	return s.withState(context.Background(), func(state *personalPointState) error {
		return importLegacyAccountsState(state, accounts)
	})
}

func importLegacyAccountsState(state *personalPointState, accounts []adminPointAccount) error {
	for _, source := range accounts {
		accountID := strings.TrimSpace(source.ID)
		userID := strings.TrimSpace(source.UserID)
		if accountID == "" || userID == "" || source.Available < 0 || source.Frozen < 0 {
			return ErrInvalidPointCommand
		}
		if source.Frozen > 0 {
			return ErrInvalidPointCommand
		}
		if source.Available == 0 {
			continue
		}
		legacyKey := "legacy-import:" + accountID
		alreadyImported := false
		for _, lot := range state.Lots {
			if lot.AccountID == accountID && lot.UserID == userID && lot.IdempotencyKey == legacyKey {
				alreadyImported = true
				break
			}
		}
		if alreadyImported {
			if !hasLegacyImportWalletLedger(state, accountID) {
				account, accountErr := findPersonalAccount(state, accountID, userID)
				if accountErr != nil {
					return accountErr
				}
				if account == nil || account.AvailablePoints < int64(source.Available) {
					return ErrInvalidPointCommand
				}
				legacyBefore := account.AvailablePoints - int64(source.Available)
				if err := appendPersonalWalletLedger(state, *account, "GRANT", int64(source.Available), legacyBefore, account.FrozenPoints, personalWalletKey(accountID, "grant", legacyKey), "LEGACY_IMPORT", accountID, map[string]any{"source_type": string(PointSourceLegacy), "legacy_import": true}, time.Now().UTC()); err != nil {
					return err
				}
			}
			continue
		}
		account, err := ensurePersonalAccount(state, accountID, userID)
		if err != nil {
			return err
		}
		grantedAt := time.Now().UTC()
		lot := PersonalPointLot{ID: stablePointID("lot", accountID, legacyKey), AccountID: accountID, UserID: userID, SourceType: PointSourceLegacy, ReferenceType: "LEGACY_IMPORT", ReferenceID: accountID, OriginalPoints: int64(source.Available), AvailablePoints: int64(source.Available), GrantedAt: grantedAt, IdempotencyKey: legacyKey, Status: "LEGACY"}
		state.Lots = append(state.Lots, lot)
		beforeAvailable, beforeFrozen := account.AvailablePoints, account.FrozenPoints
		account.AvailablePoints += int64(source.Available)
		account.TotalGranted += int64(source.Available)
		appendPersonalMovement(state, lot, "OPENING", int64(source.Available), PersonalPointLot{}, "", "legacy-import:"+accountID, grantedAt)
		if !hasLegacyImportWalletLedger(state, accountID) {
			if err := appendPersonalWalletLedger(state, *account, "GRANT", int64(source.Available), beforeAvailable, beforeFrozen, personalWalletKey(accountID, "grant", legacyKey), lot.ReferenceType, lot.ReferenceID, map[string]any{"source_type": string(PointSourceLegacy), "legacy_import": true}, grantedAt); err != nil {
				return err
			}
		}
	}
	return nil
}
