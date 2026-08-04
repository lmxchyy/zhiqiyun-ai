package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

type JSONPersonalPointStore struct {
	path    string
	mu      sync.Mutex
	memory  *personalPointState
	initErr error
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

func appendPersonalWalletLedger(state *personalPointState, account PersonalPointAccount, entryType string, points, beforeAvailable, beforeFrozen int64, key, referenceType, referenceID string, metadata map[string]any, occurredAt time.Time) error {
	if state == nil || account.ID == "" || account.UserID == "" || strings.TrimSpace(key) == "" || points < 0 || beforeAvailable < 0 || beforeFrozen < 0 || account.AvailablePoints < 0 || account.FrozenPoints < 0 {
		return ErrInvalidPointCommand
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
	validTransition := false
	switch entryType {
	case "RECHARGE", "GRANT":
		validTransition = account.AvailablePoints == beforeAvailable+points && account.FrozenPoints == beforeFrozen
	case "RESERVE":
		validTransition = beforeAvailable >= points && account.AvailablePoints == beforeAvailable-points && account.FrozenPoints == beforeFrozen+points
	case "CAPTURE":
		validTransition = account.AvailablePoints == beforeAvailable && beforeFrozen >= points && account.FrozenPoints == beforeFrozen-points
	case "RELEASE":
		validTransition = account.AvailablePoints == beforeAvailable+points && beforeFrozen >= points && account.FrozenPoints == beforeFrozen-points
	case "EXPIRE":
		validTransition = beforeAvailable >= points && account.AvailablePoints == beforeAvailable-points && account.FrozenPoints == beforeFrozen
	}
	if !validTransition {
		return ErrInvalidPointCommand
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

func (s *JSONPersonalPointStore) withState(ctx context.Context, fn func(*personalPointState) error) error {
	if err := s.operationalError(); err != nil {
		return err
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
	if err := ctx.Err(); err != nil {
		return personalPointState{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
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
		ReferenceType: lot.ReferenceType, ReferenceID: lot.ReferenceID, ReservationID: reservationID, IdempotencyKey: key, CreatedAt: now,
	})
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

func (s *JSONPersonalPointStore) importLegacyAccounts(accounts []adminPointAccount) error {
	if s == nil {
		return ErrInvalidPointCommand
	}
	return s.withState(context.Background(), func(state *personalPointState) error {
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
			if err := appendPersonalWalletLedger(state, *account, "GRANT", int64(source.Available), beforeAvailable, beforeFrozen, personalWalletKey(accountID, "grant", legacyKey), lot.ReferenceType, lot.ReferenceID, map[string]any{"source_type": string(PointSourceLegacy), "legacy_import": true}, grantedAt); err != nil {
				return err
			}
		}
		return nil
	})
}
