package httpserver

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	pricePlanWhitelistDefaultPageSize = 50
	pricePlanWhitelistMaxPageSize     = 200
	pricePlanWhitelistMaxPage         = 10000
)

type pricePlanTestWhitelistQuery struct {
	Status   string
	UserID   string
	Page     int
	PageSize int
}

func parsePricePlanTestWhitelistQuery(values url.Values) (pricePlanTestWhitelistQuery, bool, error) {
	if len(values) == 0 {
		return pricePlanTestWhitelistQuery{}, true, nil
	}
	allowed := map[string]bool{"status": true, "userId": true, "page": true, "pageSize": true}
	for key, entries := range values {
		if !allowed[key] || len(entries) != 1 {
			return pricePlanTestWhitelistQuery{}, false, invalidPricePlanWhitelistQuery()
		}
		if strings.TrimSpace(entries[0]) == "" {
			return pricePlanTestWhitelistQuery{}, false, invalidPricePlanWhitelistQuery()
		}
	}
	query := pricePlanTestWhitelistQuery{Status: strings.ToUpper(strings.TrimSpace(values.Get("status"))), UserID: strings.TrimSpace(values.Get("userId")), Page: 1, PageSize: pricePlanWhitelistDefaultPageSize}
	if query.Status != "" && query.Status != pricePlanWhitelistStatusPending && query.Status != pricePlanWhitelistStatusActive && query.Status != pricePlanWhitelistStatusExpired && query.Status != pricePlanWhitelistStatusDisabled {
		return pricePlanTestWhitelistQuery{}, false, invalidPricePlanWhitelistQuery()
	}
	if len(query.UserID) > 128 {
		return pricePlanTestWhitelistQuery{}, false, invalidPricePlanWhitelistQuery()
	}
	var err error
	if raw := strings.TrimSpace(values.Get("page")); raw != "" {
		query.Page, err = strconv.Atoi(raw)
		if err != nil {
			return pricePlanTestWhitelistQuery{}, false, invalidPricePlanWhitelistQuery()
		}
	}
	if raw := strings.TrimSpace(values.Get("pageSize")); raw != "" {
		query.PageSize, err = strconv.Atoi(raw)
		if err != nil {
			return pricePlanTestWhitelistQuery{}, false, invalidPricePlanWhitelistQuery()
		}
	}
	if query.Page < 1 || query.Page > pricePlanWhitelistMaxPage || query.PageSize < 1 || query.PageSize > pricePlanWhitelistMaxPageSize {
		return pricePlanTestWhitelistQuery{}, false, invalidPricePlanWhitelistQuery()
	}
	return query, false, nil
}

func invalidPricePlanWhitelistQuery() error {
	return newBusinessPlanAdminError(http.StatusBadRequest, "INVALID_WHITELIST_QUERY", "status, userId, page or pageSize query is invalid")
}

const (
	pricePlanWhitelistLifecycleActive   = "ACTIVE"
	pricePlanWhitelistLifecycleExpired  = "EXPIRED"
	pricePlanWhitelistLifecycleDisabled = "DISABLED"

	pricePlanWhitelistStatusPending  = "PENDING"
	pricePlanWhitelistStatusActive   = "ACTIVE"
	pricePlanWhitelistStatusExpired  = "EXPIRED"
	pricePlanWhitelistStatusDisabled = "DISABLED"
)

type pricePlanTestWhitelistView struct {
	ID          string     `json:"whitelistEntryId"`
	PlanID      string     `json:"planId"`
	PricePlanID string     `json:"pricePlanId"`
	UserID      string     `json:"userId"`
	Status      string     `json:"status"`
	ValidFrom   *time.Time `json:"validFrom,omitempty"`
	ValidUntil  *time.Time `json:"validUntil,omitempty"`
	Reason      string     `json:"reason"`
	Revision    int64      `json:"revision"`
	CreatedBy   string     `json:"createdBy"`
	UpdatedBy   string     `json:"updatedBy,omitempty"`
	DisabledBy  string     `json:"disabledBy,omitempty"`
	DisabledAt  *time.Time `json:"disabledAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`

	lifecycleStatus string
}

func (item *pricePlanTestWhitelistView) deriveStatus(now time.Time) {
	switch item.lifecycleStatus {
	case pricePlanWhitelistLifecycleDisabled:
		item.Status = pricePlanWhitelistStatusDisabled
	case pricePlanWhitelistLifecycleExpired:
		item.Status = pricePlanWhitelistStatusExpired
	default:
		if item.ValidUntil != nil && !item.ValidUntil.After(now) {
			item.Status = pricePlanWhitelistStatusExpired
		} else if item.ValidFrom != nil && item.ValidFrom.After(now) {
			item.Status = pricePlanWhitelistStatusPending
		} else {
			item.Status = pricePlanWhitelistStatusActive
		}
	}
}

type pricePlanTestWhitelistCreateMutation struct {
	Revision     *int64     `json:"revision"`
	UserID       string     `json:"userId"`
	Reason       string     `json:"reason"`
	ValidFrom    *time.Time `json:"validFrom"`
	ValidUntil   *time.Time `json:"validUntil"`
	ChangeReason string     `json:"changeReason"`
}

type pricePlanTestWhitelistUpdateMutation struct {
	Revision        *int64     `json:"revision"`
	Reason          *string    `json:"reason"`
	ValidFrom       *time.Time `json:"validFrom"`
	ValidUntil      *time.Time `json:"validUntil"`
	ClearValidFrom  bool       `json:"clearValidFrom"`
	ClearValidUntil bool       `json:"clearValidUntil"`
	ChangeReason    string     `json:"changeReason"`
}

type pricePlanTestWhitelistDisableMutation struct {
	Revision     *int64 `json:"revision"`
	ChangeReason string `json:"changeReason"`
}

func validatePricePlanTestWhitelistCreate(mutation *pricePlanTestWhitelistCreateMutation, actorID string) error {
	if err := pricingAdminActor(actorID); err != nil {
		return err
	}
	if mutation.Revision == nil {
		return newBusinessPlanAdminError(http.StatusBadRequest, "REVISION_REQUIRED", "revision is required")
	}
	if *mutation.Revision != 0 {
		return newBusinessPlanAdminError(http.StatusBadRequest, "WHITELIST_CREATE_REVISION_INVALID", "new whitelist revision must be zero")
	}
	mutation.UserID = strings.TrimSpace(mutation.UserID)
	mutation.Reason = strings.TrimSpace(mutation.Reason)
	mutation.ChangeReason = strings.TrimSpace(mutation.ChangeReason)
	if mutation.UserID == "" {
		return newBusinessPlanAdminError(http.StatusBadRequest, "WHITELIST_USER_REQUIRED", "userId is required")
	}
	if mutation.Reason == "" {
		return newBusinessPlanAdminError(http.StatusBadRequest, "WHITELIST_REASON_REQUIRED", "reason is required")
	}
	if err := pricingAdminReason(mutation.ChangeReason); err != nil {
		return err
	}
	return validatePricePlanTestWhitelistValidity(mutation.ValidFrom, mutation.ValidUntil)
}

func validatePricePlanTestWhitelistUpdate(mutation pricePlanTestWhitelistUpdateMutation, actorID string) error {
	if err := pricingAdminActor(actorID); err != nil {
		return err
	}
	if mutation.Revision == nil {
		return newBusinessPlanAdminError(http.StatusBadRequest, "REVISION_REQUIRED", "revision is required")
	}
	if err := pricingAdminReason(mutation.ChangeReason); err != nil {
		return err
	}
	if mutation.Reason != nil && strings.TrimSpace(*mutation.Reason) == "" {
		return newBusinessPlanAdminError(http.StatusBadRequest, "WHITELIST_REASON_REQUIRED", "reason is required")
	}
	if (mutation.ValidFrom != nil && mutation.ClearValidFrom) || (mutation.ValidUntil != nil && mutation.ClearValidUntil) {
		return newBusinessPlanAdminError(http.StatusBadRequest, "WHITELIST_VALIDITY_MUTATION_CONFLICT", "validity value and matching clear flag cannot be provided together")
	}
	if mutation.Reason == nil && mutation.ValidFrom == nil && mutation.ValidUntil == nil && !mutation.ClearValidFrom && !mutation.ClearValidUntil {
		return newBusinessPlanAdminError(http.StatusBadRequest, "WHITELIST_MUTATION_REQUIRED", "reason, validFrom or validUntil mutation is required")
	}
	return validatePricePlanTestWhitelistValidity(mutation.ValidFrom, mutation.ValidUntil)
}

func validatePricePlanTestWhitelistDisable(mutation pricePlanTestWhitelistDisableMutation, actorID string) error {
	if err := pricingAdminActor(actorID); err != nil {
		return err
	}
	if mutation.Revision == nil {
		return newBusinessPlanAdminError(http.StatusBadRequest, "REVISION_REQUIRED", "revision is required")
	}
	return pricingAdminReason(mutation.ChangeReason)
}

func validatePricePlanTestWhitelistValidity(effectiveAt, expiresAt *time.Time) error {
	if effectiveAt != nil && expiresAt != nil && !expiresAt.After(*effectiveAt) {
		return newBusinessPlanAdminError(http.StatusBadRequest, "WHITELIST_VALIDITY_INVALID", "validUntil must be after validFrom")
	}
	return nil
}
