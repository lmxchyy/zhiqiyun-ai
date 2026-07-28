package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var (
	pricePlanCodeFormat              = regexp.MustCompile(`^[a-z][a-z0-9_]{1,62}[a-z0-9]$`)
	pricePlanAdjacentPriceAmountCode = regexp.MustCompile(`(^|_)((price|rmb|amount|yuan)[0-9]+|[0-9]+(price|rmb|amount|yuan))(_|$)`)
)

type pricePlanAdminView struct {
	ID                 string         `json:"pricePlanId"`
	PlanID             string         `json:"planId"`
	PlanVersionID      string         `json:"planVersionId"`
	Code               string         `json:"code"`
	Name               string         `json:"name"`
	Kind               string         `json:"kind"`
	Channel            string         `json:"channel"`
	Environment        string         `json:"environment"`
	Currency           string         `json:"currency"`
	SalePriceCents     int64          `json:"salePriceCents"`
	ListPriceCents     int64          `json:"listPriceCents"`
	GiftPoints         int64          `json:"giftPoints"`
	GiftTokens         int64          `json:"giftTokens"`
	ValidFrom          *time.Time     `json:"validFrom,omitempty"`
	ValidUntil         *time.Time     `json:"validUntil,omitempty"`
	AudienceType       string         `json:"audienceType"`
	AudienceRule       map[string]any `json:"audienceRule"`
	IsVisible          bool           `json:"isVisible"`
	IsDefault          bool           `json:"isDefault"`
	IsEnabled          bool           `json:"isEnabled"`
	Status             string         `json:"status"`
	Revision           int64          `json:"revision"`
	ChangeReason       string         `json:"changeReason,omitempty"`
	CreatedBy          string         `json:"createdBy,omitempty"`
	UpdatedBy          string         `json:"updatedBy,omitempty"`
	EnabledBy          string         `json:"enabledBy,omitempty"`
	EnabledAt          *time.Time     `json:"enabledAt,omitempty"`
	DisabledBy         string         `json:"disabledBy,omitempty"`
	DisabledAt         *time.Time     `json:"disabledAt,omitempty"`
	CreatedAt          time.Time      `json:"createdAt"`
	UpdatedAt          time.Time      `json:"updatedAt"`
	HasQuote           bool           `json:"hasQuote"`
	HasOrder           bool           `json:"hasOrder"`
	EconomicFieldsLock bool           `json:"economicFieldsLocked"`

	storedKind string
}

type pricePlanCreateMutation struct {
	Revision       *int64         `json:"revision"`
	PlanVersionID  string         `json:"planVersionId"`
	Code           string         `json:"code"`
	Name           string         `json:"name"`
	Kind           string         `json:"kind"`
	Channel        string         `json:"channel"`
	Environment    string         `json:"environment"`
	Currency       string         `json:"currency"`
	SalePriceCents int64          `json:"salePriceCents"`
	ListPriceCents int64          `json:"listPriceCents"`
	GiftPoints     int64          `json:"giftPoints"`
	GiftTokens     int64          `json:"giftTokens"`
	ValidFrom      *time.Time     `json:"validFrom"`
	ValidUntil     *time.Time     `json:"validUntil"`
	AudienceType   string         `json:"audienceType"`
	AudienceRule   map[string]any `json:"audienceRule"`
	IsVisible      *bool          `json:"isVisible"`
	ChangeReason   string         `json:"changeReason"`
}

type pricePlanUpdateMutation struct {
	Revision        *int64         `json:"revision"`
	Name            *string        `json:"name"`
	PlanVersionID   *string        `json:"planVersionId"`
	Kind            *string        `json:"kind"`
	Channel         *string        `json:"channel"`
	Environment     *string        `json:"environment"`
	Currency        *string        `json:"currency"`
	SalePriceCents  *int64         `json:"salePriceCents"`
	ListPriceCents  *int64         `json:"listPriceCents"`
	GiftPoints      *int64         `json:"giftPoints"`
	GiftTokens      *int64         `json:"giftTokens"`
	ValidFrom       *time.Time     `json:"validFrom"`
	ValidUntil      *time.Time     `json:"validUntil"`
	ClearValidFrom  bool           `json:"clearValidFrom"`
	ClearValidUntil bool           `json:"clearValidUntil"`
	AudienceType    *string        `json:"audienceType"`
	AudienceRule    map[string]any `json:"audienceRule"`
	IsVisible       *bool          `json:"isVisible"`
	ChangeReason    string         `json:"changeReason"`
}

type pricePlanCloneMutation struct {
	Revision     *int64 `json:"revision"`
	Code         string `json:"code"`
	Name         string `json:"name"`
	ChangeReason string `json:"changeReason"`
}

type pricePlanTransitionMutation struct {
	Revision     *int64 `json:"revision"`
	ChangeReason string `json:"changeReason"`
}

type pricePlanValidationCheck struct {
	Code    string `json:"code"`
	Passed  bool   `json:"passed"`
	Message string `json:"message,omitempty"`
}

type pricePlanValidationResult struct {
	PricePlanID          string                     `json:"pricePlanId"`
	Valid                bool                       `json:"valid"`
	CheckedAt            time.Time                  `json:"checkedAt"`
	PaymentBindingID     string                     `json:"paymentBindingId,omitempty"`
	WeChatGoodID         string                     `json:"wechatGoodId,omitempty"`
	WeChatProductID      string                     `json:"wechatProductId,omitempty"`
	PricePlanPriceCents  int64                      `json:"pricePlanPriceCents"`
	BindingPriceCents    int64                      `json:"bindingPriceCents,omitempty"`
	WeChatGoodPriceCents int64                      `json:"wechatGoodPriceCents,omitempty"`
	Checks               []pricePlanValidationCheck `json:"checks"`
}

type pricePlanAdminAPI struct {
	store *postgresStore
}

func newPricePlanAdminAPI(store platformStore) pricePlanAdminAPI {
	postgres, _ := store.(*postgresStore)
	return pricePlanAdminAPI{store: postgres}
}

func (a pricePlanAdminAPI) requireStore(w http.ResponseWriter) bool {
	if a.store != nil && a.store.db != nil {
		return true
	}
	writeError(w, http.StatusServiceUnavailable, errors.New("price plan administration requires PostgreSQL"))
	return false
}

func (a pricePlanAdminAPI) plans(w http.ResponseWriter, r *http.Request) {
	if !a.requireStore(w) {
		return
	}
	items, err := a.store.listPricePlans(r.Context(), r.PathValue("planId"))
	if err != nil {
		writeBusinessPlanAdminError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items, "total": len(items)})
}

func (a pricePlanAdminAPI) plan(w http.ResponseWriter, r *http.Request) {
	if !a.requireStore(w) {
		return
	}
	item, err := a.store.pricePlan(r.Context(), r.PathValue("pricePlanId"))
	if err != nil {
		writeBusinessPlanAdminError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (a pricePlanAdminAPI) createPlan(w http.ResponseWriter, r *http.Request) {
	if !a.requireStore(w) {
		return
	}
	var mutation pricePlanCreateMutation
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&mutation); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	actorID, actorRole := actorFromRequest(r)
	item, err := a.store.createPricePlan(r.Context(), r.PathValue("planId"), mutation, actorID, actorRole)
	if err != nil {
		writeBusinessPlanAdminError(w, err)
		return
	}
	writeJSONWithStatus(w, http.StatusCreated, map[string]any{"item": item})
}

func (a pricePlanAdminAPI) updatePlan(w http.ResponseWriter, r *http.Request) {
	if !a.requireStore(w) {
		return
	}
	var mutation pricePlanUpdateMutation
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&mutation); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	actorID, actorRole := actorFromRequest(r)
	item, err := a.store.updatePricePlan(r.Context(), r.PathValue("pricePlanId"), mutation, actorID, actorRole)
	if err != nil {
		writeBusinessPlanAdminError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (a pricePlanAdminAPI) clonePlan(w http.ResponseWriter, r *http.Request) {
	if !a.requireStore(w) {
		return
	}
	var mutation pricePlanCloneMutation
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&mutation); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	actorID, actorRole := actorFromRequest(r)
	item, err := a.store.clonePricePlan(r.Context(), r.PathValue("pricePlanId"), mutation, actorID, actorRole)
	if err != nil {
		writeBusinessPlanAdminError(w, err)
		return
	}
	writeJSONWithStatus(w, http.StatusCreated, map[string]any{"item": item})
}

func (a pricePlanAdminAPI) validation(w http.ResponseWriter, r *http.Request) {
	if !a.requireStore(w) {
		return
	}
	result, err := a.store.validatePricePlan(r.Context(), r.PathValue("pricePlanId"))
	if err != nil {
		writeBusinessPlanAdminError(w, err)
		return
	}
	writeJSON(w, result)
}

func (a pricePlanAdminAPI) enablePlan(w http.ResponseWriter, r *http.Request) {
	a.transitionPlan(w, r, true)
}

func (a pricePlanAdminAPI) disablePlan(w http.ResponseWriter, r *http.Request) {
	a.transitionPlan(w, r, false)
}

func (a pricePlanAdminAPI) makeDefault(w http.ResponseWriter, r *http.Request) {
	if !a.requireStore(w) {
		return
	}
	var mutation pricePlanTransitionMutation
	if err := decodeStrictJSON(r, &mutation); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	actorID, actorRole := actorFromRequest(r)
	item, alreadyDefault, err := a.store.makeDefaultPricePlan(r.Context(), r.PathValue("pricePlanId"), mutation, actorID, actorRole)
	if err != nil {
		writeBusinessPlanAdminError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": item, "alreadyDefault": alreadyDefault})
}

func (a pricePlanAdminAPI) transitionPlan(w http.ResponseWriter, r *http.Request, enable bool) {
	if !a.requireStore(w) {
		return
	}
	var mutation pricePlanTransitionMutation
	if err := decodeStrictJSON(r, &mutation); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	actorID, actorRole := actorFromRequest(r)
	item, err := a.store.transitionPricePlan(r.Context(), r.PathValue("pricePlanId"), mutation, enable, actorID, actorRole)
	if err != nil {
		writeBusinessPlanAdminError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func normalizePricePlanKind(value string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "NORMAL":
		return "NORMAL", nil
	case "PROMOTION":
		return "ACTIVITY", nil
	case "TEST":
		return "TEST", nil
	default:
		return "", newBusinessPlanAdminError(http.StatusBadRequest, "PRICE_PLAN_KIND_INVALID", "kind must be NORMAL, PROMOTION or TEST")
	}
}

func publicPricePlanKind(stored string) string {
	switch strings.ToUpper(strings.TrimSpace(stored)) {
	case "ACTIVITY", "GRAY":
		return "PROMOTION"
	default:
		return strings.ToUpper(strings.TrimSpace(stored))
	}
}

func validatePricePlanCode(code string) error {
	code = strings.TrimSpace(code)
	if !pricePlanCodeFormat.MatchString(code) {
		return newBusinessPlanAdminError(http.StatusBadRequest, "PRICE_PLAN_CODE_FORMAT_INVALID", "code must use 3-64 lowercase letters, digits or underscores")
	}
	if businessPlanPriceWord.MatchString(code) || businessPlanPricedIdentity.MatchString(code) || pricePlanAdjacentPriceAmountCode.MatchString(code) {
		return newBusinessPlanAdminError(http.StatusBadRequest, "PRICE_PLAN_CODE_HAS_PRICE", "code cannot contain explicit price semantics")
	}
	return nil
}

func validatePricePlanCreateMutation(mutation *pricePlanCreateMutation) error {
	if mutation.Revision == nil {
		return newBusinessPlanAdminError(http.StatusBadRequest, "REVISION_REQUIRED", "revision is required")
	}
	if err := validateVersionMutationReason(mutation.ChangeReason); err != nil {
		return err
	}
	if err := validatePricePlanCode(mutation.Code); err != nil {
		return err
	}
	storedKind, err := normalizePricePlanKind(mutation.Kind)
	if err != nil {
		return err
	}
	mutation.Kind = storedKind
	mutation.PlanVersionID = strings.TrimSpace(mutation.PlanVersionID)
	mutation.Name = strings.TrimSpace(mutation.Name)
	if mutation.PlanVersionID == "" || mutation.Name == "" {
		return newBusinessPlanAdminError(http.StatusBadRequest, "PRICE_PLAN_INVALID", "planVersionId and name are required")
	}
	mutation.Channel, err = normalizeWechatGoodChannel(mutation.Channel)
	if err != nil {
		return err
	}
	mutation.Environment, err = normalizePaymentEnvironment(mutation.Environment)
	if err != nil {
		return err
	}
	mutation.Currency = strings.ToUpper(strings.TrimSpace(mutation.Currency))
	if mutation.Currency != "CNY" {
		return newBusinessPlanAdminError(http.StatusBadRequest, "PRICE_PLAN_CURRENCY_INVALID", "WeChat virtual price plans currently require CNY")
	}
	if mutation.SalePriceCents <= 0 || mutation.ListPriceCents < mutation.SalePriceCents || mutation.GiftPoints < 0 || mutation.GiftTokens < 0 {
		return newBusinessPlanAdminError(http.StatusBadRequest, "PRICE_PLAN_AMOUNT_INVALID", "sale price must be positive, list price cannot be lower, and gifts cannot be negative")
	}
	if mutation.ValidFrom != nil && mutation.ValidUntil != nil && !mutation.ValidUntil.After(*mutation.ValidFrom) {
		return newBusinessPlanAdminError(http.StatusBadRequest, "PRICE_PLAN_VALIDITY_INVALID", "validUntil must be after validFrom")
	}
	mutation.AudienceType = strings.ToUpper(strings.TrimSpace(mutation.AudienceType))
	switch mutation.AudienceType {
	case "PUBLIC", "RULE", "WHITELIST", "INVITE", "TEST":
	default:
		return newBusinessPlanAdminError(http.StatusBadRequest, "PRICE_PLAN_AUDIENCE_INVALID", "unsupported audienceType")
	}
	if mutation.AudienceRule == nil {
		mutation.AudienceRule = map[string]any{}
	}
	if mutation.IsVisible == nil {
		return newBusinessPlanAdminError(http.StatusBadRequest, "PRICE_PLAN_VISIBILITY_REQUIRED", "isVisible is required")
	}
	if mutation.Kind == "TEST" && (*mutation.IsVisible || mutation.AudienceType == "PUBLIC") {
		return newBusinessPlanAdminError(http.StatusBadRequest, "PRICE_PLAN_TEST_SCOPE_INVALID", "TEST price plans must be hidden and non-public")
	}
	return nil
}

func requirePricePlanWrite(revision *int64, changeReason string) error {
	if revision == nil {
		return newBusinessPlanAdminError(http.StatusBadRequest, "REVISION_REQUIRED", "revision is required")
	}
	if err := validateVersionMutationReason(changeReason); err != nil {
		return err
	}
	return nil
}

func applyPricePlanUpdate(current pricePlanAdminView, mutation pricePlanUpdateMutation) (pricePlanAdminView, bool, error) {
	if (mutation.ClearValidFrom && mutation.ValidFrom != nil) || (mutation.ClearValidUntil && mutation.ValidUntil != nil) {
		return current, false, newBusinessPlanAdminError(http.StatusBadRequest, "PRICE_PLAN_VALIDITY_MUTATION_CONFLICT", "validity value and matching clear flag cannot be provided together")
	}
	updated := current
	if mutation.Name != nil {
		updated.Name = strings.TrimSpace(*mutation.Name)
	}
	if mutation.PlanVersionID != nil {
		updated.PlanVersionID = strings.TrimSpace(*mutation.PlanVersionID)
	}
	if mutation.Kind != nil {
		kind, err := normalizePricePlanKind(*mutation.Kind)
		if err != nil {
			return current, false, err
		}
		updated.storedKind = kind
		updated.Kind = publicPricePlanKind(kind)
	}
	if mutation.Channel != nil {
		updated.Channel = *mutation.Channel
	}
	if mutation.Environment != nil {
		updated.Environment = *mutation.Environment
	}
	if mutation.Currency != nil {
		updated.Currency = *mutation.Currency
	}
	if mutation.SalePriceCents != nil {
		updated.SalePriceCents = *mutation.SalePriceCents
	}
	if mutation.ListPriceCents != nil {
		updated.ListPriceCents = *mutation.ListPriceCents
	}
	if mutation.GiftPoints != nil {
		updated.GiftPoints = *mutation.GiftPoints
	}
	if mutation.GiftTokens != nil {
		updated.GiftTokens = *mutation.GiftTokens
	}
	if mutation.ClearValidFrom {
		updated.ValidFrom = nil
	} else if mutation.ValidFrom != nil {
		updated.ValidFrom = mutation.ValidFrom
	}
	if mutation.ClearValidUntil {
		updated.ValidUntil = nil
	} else if mutation.ValidUntil != nil {
		updated.ValidUntil = mutation.ValidUntil
	}
	if mutation.AudienceType != nil {
		updated.AudienceType = strings.ToUpper(strings.TrimSpace(*mutation.AudienceType))
	}
	if mutation.AudienceRule != nil {
		updated.AudienceRule = mutation.AudienceRule
	}
	if mutation.IsVisible != nil {
		updated.IsVisible = *mutation.IsVisible
	}
	if updated.Name == "" || updated.PlanVersionID == "" {
		return current, false, newBusinessPlanAdminError(http.StatusBadRequest, "PRICE_PLAN_INVALID", "planVersionId and name are required")
	}
	var err error
	updated.Channel, err = normalizeWechatGoodChannel(updated.Channel)
	if err != nil {
		return current, false, err
	}
	updated.Environment, err = normalizePaymentEnvironment(updated.Environment)
	if err != nil {
		return current, false, err
	}
	updated.Currency = strings.ToUpper(strings.TrimSpace(updated.Currency))
	if updated.Currency != "CNY" {
		return current, false, newBusinessPlanAdminError(http.StatusBadRequest, "PRICE_PLAN_CURRENCY_INVALID", "WeChat virtual price plans currently require CNY")
	}
	if updated.SalePriceCents <= 0 || updated.ListPriceCents < updated.SalePriceCents || updated.GiftPoints < 0 || updated.GiftTokens < 0 {
		return current, false, newBusinessPlanAdminError(http.StatusBadRequest, "PRICE_PLAN_AMOUNT_INVALID", "sale price must be positive, list price cannot be lower, and gifts cannot be negative")
	}
	if updated.ValidFrom != nil && updated.ValidUntil != nil && !updated.ValidUntil.After(*updated.ValidFrom) {
		return current, false, newBusinessPlanAdminError(http.StatusBadRequest, "PRICE_PLAN_VALIDITY_INVALID", "validUntil must be after validFrom")
	}
	switch updated.AudienceType {
	case "PUBLIC", "RULE", "WHITELIST", "INVITE", "TEST":
	default:
		return current, false, newBusinessPlanAdminError(http.StatusBadRequest, "PRICE_PLAN_AUDIENCE_INVALID", "unsupported audienceType")
	}
	if updated.storedKind == "TEST" && (updated.IsVisible || updated.AudienceType == "PUBLIC") {
		return current, false, newBusinessPlanAdminError(http.StatusBadRequest, "PRICE_PLAN_TEST_SCOPE_INVALID", "TEST price plans must be hidden and non-public")
	}
	economicChanged := current.PlanVersionID != updated.PlanVersionID || current.storedKind != updated.storedKind ||
		current.Channel != updated.Channel || current.Environment != updated.Environment || current.Currency != updated.Currency ||
		current.SalePriceCents != updated.SalePriceCents || current.ListPriceCents != updated.ListPriceCents ||
		current.GiftPoints != updated.GiftPoints || current.GiftTokens != updated.GiftTokens ||
		current.AudienceType != updated.AudienceType || !jsonMapsEqual(current.AudienceRule, updated.AudienceRule) ||
		current.IsVisible != updated.IsVisible || !timesEqual(current.ValidFrom, updated.ValidFrom) || !timesEqual(current.ValidUntil, updated.ValidUntil)
	return updated, economicChanged, nil
}

func jsonMapsEqual(left, right map[string]any) bool {
	leftRaw, _ := json.Marshal(left)
	rightRaw, _ := json.Marshal(right)
	return string(leftRaw) == string(rightRaw)
}

func timesEqual(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}
