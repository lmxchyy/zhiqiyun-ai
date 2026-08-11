package httpserver

import (
	"net/http"

	"xianzhi-ai/backend-go/internal/config"
)

type pricingHealthAdminAPI struct {
	store *postgresStore
	cfg   config.Config
}

func newPricingHealthAdminAPI(store platformStore, cfg config.Config) pricingHealthAdminAPI {
	postgres, _ := store.(*postgresStore)
	return pricingHealthAdminAPI{store: postgres, cfg: cfg}
}

func (a pricingHealthAdminAPI) get(w http.ResponseWriter, r *http.Request) {
	if a.store == nil || a.store.db == nil {
		writeBusinessPlanAdminError(w, newBusinessPlanAdminError(http.StatusServiceUnavailable, "PRICING_HEALTH_STORE_UNAVAILABLE", "pricing health requires PostgreSQL"))
		return
	}
	view, err := a.store.pricingHealth(r.Context(), a.cfg)
	if err != nil {
		writeBusinessPlanAdminError(w, err)
		return
	}
	writeJSON(w, view)
}
