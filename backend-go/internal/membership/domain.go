// Package membership owns transport-independent manual membership rules.
package membership

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const MaxManualGrantDays = 3650

var ErrInvalidGrant = errors.New("invalid membership grant")

type ManualGrantRequest struct {
	PlanID         string
	DurationDays   int
	Reason         string
	IdempotencyKey string
}

func NormalizeManualGrant(request ManualGrantRequest) (ManualGrantRequest, error) {
	request.PlanID = strings.TrimSpace(request.PlanID)
	request.Reason = strings.TrimSpace(request.Reason)
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	if request.PlanID == "" || request.Reason == "" || request.IdempotencyKey == "" || request.DurationDays < 0 || request.DurationDays > MaxManualGrantDays {
		return request, ErrInvalidGrant
	}
	return request, nil
}

func ResolveExpiry(now, previous time.Time, days int) (time.Time, error) {
	if days <= 0 || days > MaxManualGrantDays {
		return time.Time{}, ErrInvalidGrant
	}
	candidate := now.AddDate(0, 0, days)
	if previous.After(candidate) {
		return previous.UTC(), nil
	}
	return candidate.UTC(), nil
}

func InvalidExpiryError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("invalid existing membership expiry: %w", err)
}
