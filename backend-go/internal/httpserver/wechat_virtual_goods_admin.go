package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

const (
	wechatGoodVerificationUnconfirmed = "UNCONFIRMED"
	wechatGoodVerificationManual      = "MANUALLY_CONFIRMED_PUBLISHED"
	wechatGoodVerificationMismatch    = "PRICE_MISMATCH"
	wechatGoodVerificationExpired     = "VERIFICATION_EXPIRED"
	wechatGoodVerificationDisabled    = "DISABLED"
	wechatGoodVerificationSource      = "LOCAL_MANUAL_OPERATOR"
)

type wechatVirtualGoodAdminView struct {
	ID                       string         `json:"id"`
	Channel                  string         `json:"channel"`
	Environment              string         `json:"environment"`
	OfferID                  string         `json:"offerId"`
	ProductID                string         `json:"productId"`
	GoodsName                string         `json:"goodsName"`
	PlatformPriceCents       int64          `json:"platformPriceCents"`
	Mode                     string         `json:"mode"`
	Published                bool           `json:"published"`
	Enabled                  bool           `json:"enabled"`
	Status                   string         `json:"status"`
	VerificationStatus       string         `json:"verificationStatus"`
	VerificationSource       string         `json:"verificationSource"`
	PlatformRealtimeVerified bool           `json:"platformRealtimeVerified"`
	VerifiedBy               string         `json:"verifiedBy,omitempty"`
	VerifiedAt               *time.Time     `json:"verifiedAt,omitempty"`
	VerificationReason       string         `json:"verificationReason,omitempty"`
	VerificationEvidence     string         `json:"verificationEvidence,omitempty"`
	VerificationSnapshot     map[string]any `json:"verificationSnapshot"`
	VerificationExpiresAt    *time.Time     `json:"verificationExpiresAt,omitempty"`
	Revision                 int64          `json:"revision"`
	CreatedBy                string         `json:"createdBy,omitempty"`
	UpdatedBy                string         `json:"updatedBy,omitempty"`
	CreatedAt                time.Time      `json:"createdAt"`
	UpdatedAt                time.Time      `json:"updatedAt"`

	recordedVerificationStatus string
}

func (item *wechatVirtualGoodAdminView) deriveVerification(now time.Time) {
	item.VerificationSource = wechatGoodVerificationSource
	item.PlatformRealtimeVerified = false
	if item.recordedVerificationStatus == "" {
		item.recordedVerificationStatus = item.VerificationStatus
	}
	item.VerificationStatus = item.recordedVerificationStatus
	if item.recordedVerificationStatus == wechatGoodVerificationManual && item.VerificationExpiresAt != nil && !now.Before(*item.VerificationExpiresAt) {
		item.VerificationStatus = wechatGoodVerificationExpired
	}
	if item.VerificationSnapshot == nil {
		item.VerificationSnapshot = map[string]any{}
	}
}

func (item wechatVirtualGoodAdminView) manuallyConfirmedAt(now time.Time) bool {
	return item.recordedVerificationStatus == wechatGoodVerificationManual &&
		(item.VerificationExpiresAt == nil || now.Before(*item.VerificationExpiresAt))
}

type wechatVirtualGoodCreateMutation struct {
	Channel            string `json:"channel"`
	Environment        string `json:"environment"`
	OfferID            string `json:"offerId"`
	ProductID          string `json:"productId"`
	GoodsName          string `json:"goodsName"`
	PlatformPriceCents int64  `json:"platformPriceCents"`
	Mode               string `json:"mode"`
	Reason             string `json:"reason"`
}

type wechatVirtualGoodUpdateMutation struct {
	Revision           int64   `json:"revision"`
	Channel            *string `json:"channel"`
	Environment        *string `json:"environment"`
	OfferID            *string `json:"offerId"`
	ProductID          *string `json:"productId"`
	GoodsName          *string `json:"goodsName"`
	PlatformPriceCents *int64  `json:"platformPriceCents"`
	Mode               *string `json:"mode"`
	Reason             string  `json:"reason"`
}

type wechatVirtualGoodConfirmation struct {
	Revision              int64      `json:"revision"`
	Reason                string     `json:"reason"`
	VerificationReason    string     `json:"verificationReason"`
	Evidence              string     `json:"evidence"`
	VerificationExpiresAt *time.Time `json:"verificationExpiresAt"`
}

type wechatVirtualGoodTransition struct {
	Revision int64  `json:"revision"`
	Reason   string `json:"reason"`
}

type pricePlanPaymentBindingAdminView struct {
	ID                         string     `json:"id"`
	PricePlanID                string     `json:"pricePlanId"`
	WeChatGoodID               string     `json:"wechatGoodId"`
	Channel                    string     `json:"channel"`
	Environment                string     `json:"environment"`
	ProviderPriceSnapshotCents int64      `json:"providerPriceSnapshotCents"`
	Enabled                    bool       `json:"enabled"`
	Status                     string     `json:"status"`
	Revision                   int64      `json:"revision"`
	CreatedBy                  string     `json:"createdBy,omitempty"`
	UpdatedBy                  string     `json:"updatedBy,omitempty"`
	EnabledBy                  string     `json:"enabledBy,omitempty"`
	EnabledAt                  *time.Time `json:"enabledAt,omitempty"`
	DisabledBy                 string     `json:"disabledBy,omitempty"`
	DisabledAt                 *time.Time `json:"disabledAt,omitempty"`
	CreatedAt                  time.Time  `json:"createdAt"`
	UpdatedAt                  time.Time  `json:"updatedAt"`
	PricePlanSalePriceCents    int64      `json:"pricePlanSalePriceCents"`
	WeChatGoodPriceCents       int64      `json:"wechatGoodPriceCents"`
	WeChatProductID            string     `json:"wechatProductId"`
	VerificationStatus         string     `json:"verificationStatus"`
	PriceConsistent            bool       `json:"priceConsistent"`
	EnvironmentConsistent      bool       `json:"environmentConsistent"`
}

type pricePlanPaymentBindingCreateMutation struct {
	WeChatGoodID string `json:"wechatGoodId"`
	Reason       string `json:"reason"`
}

type pricePlanPaymentBindingMutation struct {
	Revision     int64   `json:"revision"`
	Enabled      *bool   `json:"enabled"`
	WeChatGoodID *string `json:"wechatGoodId"`
	Reason       string  `json:"reason"`
}

type wechatVirtualGoodReferenceAdminView struct {
	BindingID                  string `json:"bindingId"`
	PricePlanID                string `json:"pricePlanId"`
	PricePlanCode              string `json:"pricePlanCode"`
	PricePlanName              string `json:"pricePlanName"`
	PlanID                     string `json:"planId"`
	PlanName                   string `json:"planName"`
	IsDefault                  bool   `json:"isDefault"`
	BindingStatus              string `json:"bindingStatus"`
	BindingEnabled             bool   `json:"bindingEnabled"`
	SalePriceCents             int64  `json:"salePriceCents"`
	ProviderPriceSnapshotCents int64  `json:"providerPriceSnapshotCents"`
	Channel                    string `json:"channel"`
	Environment                string `json:"environment"`
	WeChatGoodID               string `json:"wechatGoodId"`
	QuoteCount                 int64  `json:"quoteCount"`
	OrderCount                 int64  `json:"orderCount"`
}

type wechatVirtualGoodsAdminAPI struct {
	store *postgresStore
}

func newWechatVirtualGoodsAdminAPI(store platformStore) wechatVirtualGoodsAdminAPI {
	postgres, _ := store.(*postgresStore)
	return wechatVirtualGoodsAdminAPI{store: postgres}
}

func (a wechatVirtualGoodsAdminAPI) requireStore(w http.ResponseWriter) bool {
	if a.store != nil && a.store.db != nil {
		return true
	}
	writeError(w, http.StatusServiceUnavailable, errors.New("WeChat virtual goods administration requires PostgreSQL"))
	return false
}

func decodeStrictJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func (a wechatVirtualGoodsAdminAPI) goods(w http.ResponseWriter, r *http.Request) {
	if !a.requireStore(w) {
		return
	}
	items, err := a.store.listWechatVirtualGoods(r.Context())
	if err != nil {
		writeBusinessPlanAdminError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items, "total": len(items), "verificationSource": wechatGoodVerificationSource})
}

func (a wechatVirtualGoodsAdminAPI) good(w http.ResponseWriter, r *http.Request) {
	if !a.requireStore(w) {
		return
	}
	item, err := a.store.wechatVirtualGood(r.Context(), r.PathValue("goodId"))
	if err != nil {
		writeBusinessPlanAdminError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (a wechatVirtualGoodsAdminAPI) references(w http.ResponseWriter, r *http.Request) {
	if !a.requireStore(w) {
		return
	}
	items, err := a.store.listWechatVirtualGoodReferences(r.Context(), r.PathValue("goodId"))
	if err != nil {
		writeBusinessPlanAdminError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items, "total": len(items)})
}

func (a wechatVirtualGoodsAdminAPI) createGood(w http.ResponseWriter, r *http.Request) {
	if !a.requireStore(w) {
		return
	}
	var mutation wechatVirtualGoodCreateMutation
	if err := decodeStrictJSON(r, &mutation); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	actorID, actorRole := actorFromRequest(r)
	item, err := a.store.createWechatVirtualGood(r.Context(), mutation, actorID, actorRole)
	if err != nil {
		writeBusinessPlanAdminError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]any{"item": item})
}

func (a wechatVirtualGoodsAdminAPI) updateGood(w http.ResponseWriter, r *http.Request) {
	if !a.requireStore(w) {
		return
	}
	var mutation wechatVirtualGoodUpdateMutation
	if err := decodeStrictJSON(r, &mutation); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	actorID, actorRole := actorFromRequest(r)
	item, err := a.store.updateWechatVirtualGood(r.Context(), r.PathValue("goodId"), mutation, actorID, actorRole)
	if err != nil {
		writeBusinessPlanAdminError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (a wechatVirtualGoodsAdminAPI) confirmGood(w http.ResponseWriter, r *http.Request) {
	if !a.requireStore(w) {
		return
	}
	var confirmation wechatVirtualGoodConfirmation
	if err := decodeStrictJSON(r, &confirmation); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	actorID, actorRole := actorFromRequest(r)
	item, err := a.store.confirmWechatVirtualGood(r.Context(), r.PathValue("goodId"), confirmation, actorID, actorRole)
	if err != nil {
		writeBusinessPlanAdminError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": item, "confirmation": "LOCAL_MANUAL_ONLY", "wechatRealtimeVerified": false})
}

func (a wechatVirtualGoodsAdminAPI) disableGood(w http.ResponseWriter, r *http.Request) {
	if !a.requireStore(w) {
		return
	}
	var transition wechatVirtualGoodTransition
	if err := decodeStrictJSON(r, &transition); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	actorID, actorRole := actorFromRequest(r)
	item, err := a.store.disableWechatVirtualGood(r.Context(), r.PathValue("goodId"), transition, actorID, actorRole)
	if err != nil {
		writeBusinessPlanAdminError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (a wechatVirtualGoodsAdminAPI) bindings(w http.ResponseWriter, r *http.Request) {
	if !a.requireStore(w) {
		return
	}
	items, err := a.store.listPricePlanPaymentBindings(r.Context(), r.PathValue("pricePlanId"))
	if err != nil {
		writeBusinessPlanAdminError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items, "total": len(items)})
}

func (a wechatVirtualGoodsAdminAPI) createBinding(w http.ResponseWriter, r *http.Request) {
	if !a.requireStore(w) {
		return
	}
	var mutation pricePlanPaymentBindingCreateMutation
	if err := decodeStrictJSON(r, &mutation); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	actorID, actorRole := actorFromRequest(r)
	item, err := a.store.createPricePlanPaymentBinding(r.Context(), r.PathValue("pricePlanId"), mutation, actorID, actorRole)
	if err != nil {
		writeBusinessPlanAdminError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]any{"item": item})
}

func (a wechatVirtualGoodsAdminAPI) updateBinding(w http.ResponseWriter, r *http.Request) {
	if !a.requireStore(w) {
		return
	}
	var mutation pricePlanPaymentBindingMutation
	if err := decodeStrictJSON(r, &mutation); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	actorID, actorRole := actorFromRequest(r)
	item, err := a.store.updatePricePlanPaymentBinding(r.Context(), r.PathValue("bindingId"), mutation, actorID, actorRole)
	if err != nil {
		writeBusinessPlanAdminError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func normalizeWechatGoodChannel(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		value = virtualPaymentChannel
	}
	if value != virtualPaymentChannel {
		return "", newBusinessPlanAdminError(http.StatusBadRequest, "WECHAT_GOOD_CHANNEL_INVALID", "channel must be WECHAT_VIRTUAL")
	}
	return value, nil
}

func normalizePaymentEnvironment(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value != "PRODUCTION" && value != "SANDBOX" {
		return "", newBusinessPlanAdminError(http.StatusBadRequest, "PAYMENT_ENVIRONMENT_INVALID", "environment must be PRODUCTION or SANDBOX")
	}
	return value, nil
}

func normalizeWechatGoodMode(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "short_series_goods"
	}
	if value != "short_series_goods" && value != "short_series_coin" {
		return "", newBusinessPlanAdminError(http.StatusBadRequest, "WECHAT_GOOD_MODE_INVALID", "unsupported WeChat virtual payment mode")
	}
	return value, nil
}
