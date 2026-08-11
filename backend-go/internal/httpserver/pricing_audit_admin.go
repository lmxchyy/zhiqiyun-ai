package httpserver

import (
	"net/http"
)

type pricingAuditAdminAPI struct {
	store *postgresStore
}

func newPricingAuditAdminAPI(store platformStore) pricingAuditAdminAPI {
	postgres, _ := store.(*postgresStore)
	return pricingAuditAdminAPI{store: postgres}
}

func (a pricingAuditAdminAPI) list(w http.ResponseWriter, r *http.Request) {
	if a.store == nil || a.store.db == nil {
		writeBusinessPlanAdminError(w, newBusinessPlanAdminError(http.StatusServiceUnavailable, "PRICING_AUDIT_STORE_UNAVAILABLE", "pricing audit PostgreSQL store is unavailable"))
		return
	}
	query, err := parsePricingAuditQuery(r.URL.Query())
	if err != nil {
		writeBusinessPlanAdminError(w, err)
		return
	}
	page, err := a.store.listPricingAuditLogs(r.Context(), query)
	if err != nil {
		writeBusinessPlanAdminError(w, err)
		return
	}
	writeJSON(w, page)
}
