package httpserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

type pricePlanTestWhitelistAdminAPI struct {
	store *postgresStore
}

func newPricePlanTestWhitelistAdminAPI(store platformStore) pricePlanTestWhitelistAdminAPI {
	postgres, _ := store.(*postgresStore)
	return pricePlanTestWhitelistAdminAPI{store: postgres}
}

func (a pricePlanTestWhitelistAdminAPI) requireStore(w http.ResponseWriter) bool {
	if a.store != nil && a.store.db != nil {
		return true
	}
	writeBusinessPlanAdminError(w, newBusinessPlanAdminError(http.StatusServiceUnavailable, "PRICE_PLAN_WHITELIST_STORE_UNAVAILABLE", "price plan TEST whitelist administration requires PostgreSQL"))
	return false
}

func (a pricePlanTestWhitelistAdminAPI) list(w http.ResponseWriter, r *http.Request) {
	if !a.requireStore(w) {
		return
	}
	query, legacy, err := parsePricePlanTestWhitelistQuery(r.URL.Query())
	if err != nil {
		writeBusinessPlanAdminError(w, err)
		return
	}
	if legacy {
		items, err := a.store.listPricePlanTestWhitelist(r.Context(), r.PathValue("pricePlanId"))
		if err != nil {
			writeBusinessPlanAdminError(w, err)
			return
		}
		writeJSON(w, map[string]any{"items": items, "total": len(items)})
		return
	}
	items, total, err := a.store.listPricePlanTestWhitelistPage(r.Context(), r.PathValue("pricePlanId"), query)
	if err != nil {
		writeBusinessPlanAdminError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items, "total": total, "page": query.Page, "pageSize": query.PageSize})
}

func (a pricePlanTestWhitelistAdminAPI) create(w http.ResponseWriter, r *http.Request) {
	if !a.requireStore(w) {
		return
	}
	var mutation pricePlanTestWhitelistCreateMutation
	if err := decodePricePlanTestWhitelistJSON(r, &mutation); err != nil {
		writeBusinessPlanAdminError(w, err)
		return
	}
	actorID, actorRole := actorFromRequest(r)
	item, err := a.store.createPricePlanTestWhitelist(r.Context(), r.PathValue("pricePlanId"), mutation, actorID, actorRole)
	if err != nil {
		writeBusinessPlanAdminError(w, err)
		return
	}
	writeJSONWithStatus(w, http.StatusCreated, map[string]any{"item": item})
}

func (a pricePlanTestWhitelistAdminAPI) update(w http.ResponseWriter, r *http.Request) {
	if !a.requireStore(w) {
		return
	}
	var mutation pricePlanTestWhitelistUpdateMutation
	if err := decodePricePlanTestWhitelistJSON(r, &mutation); err != nil {
		writeBusinessPlanAdminError(w, err)
		return
	}
	actorID, actorRole := actorFromRequest(r)
	item, err := a.store.updatePricePlanTestWhitelist(
		r.Context(), r.PathValue("pricePlanId"), r.PathValue("entryId"), mutation, actorID, actorRole,
	)
	if err != nil {
		writeBusinessPlanAdminError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (a pricePlanTestWhitelistAdminAPI) disable(w http.ResponseWriter, r *http.Request) {
	if !a.requireStore(w) {
		return
	}
	var mutation pricePlanTestWhitelistDisableMutation
	if err := decodePricePlanTestWhitelistJSON(r, &mutation); err != nil {
		writeBusinessPlanAdminError(w, err)
		return
	}
	actorID, actorRole := actorFromRequest(r)
	item, alreadyDisabled, err := a.store.disablePricePlanTestWhitelist(
		r.Context(), r.PathValue("pricePlanId"), r.PathValue("entryId"), mutation, actorID, actorRole,
	)
	if err != nil {
		writeBusinessPlanAdminError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": item, "alreadyDisabled": alreadyDisabled})
}

func decodePricePlanTestWhitelistJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(r.Body)
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return newBusinessPlanAdminError(http.StatusBadRequest, "INVALID_REQUEST", "request body must be one valid JSON object with known fields")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return newBusinessPlanAdminError(http.StatusBadRequest, "INVALID_REQUEST", "request body must contain exactly one JSON object")
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return newBusinessPlanAdminError(http.StatusBadRequest, "INVALID_REQUEST", "request body must be a JSON object")
	}
	strict := json.NewDecoder(bytes.NewReader(raw))
	strict.DisallowUnknownFields()
	if err := strict.Decode(target); err != nil {
		return newBusinessPlanAdminError(http.StatusBadRequest, "INVALID_REQUEST", "request body must be one valid JSON object with known fields")
	}
	return nil
}
