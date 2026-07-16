package httpserver

import (
	"encoding/json"
	"net/http"
)

func (a adminAPI) billingRulesV1(w http.ResponseWriter, _ *http.Request) {
	items, err := a.store.ListBillingRuleVersions()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"items": items, "total": len(items)})
}

func (a adminAPI) billingRuleV1(w http.ResponseWriter, r *http.Request) {
	item, err := a.store.GetBillingRuleVersion(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (a adminAPI) validateBillingRuleV1(w http.ResponseWriter, r *http.Request) {
	result, err := a.store.ValidateBillingRuleVersion(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{"validation": result})
}

func (a adminAPI) publishBillingRuleV1(w http.ResponseWriter, r *http.Request) {
	item, err := a.store.PublishBillingRuleVersion(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (a adminAPI) providerCostsV1(w http.ResponseWriter, _ *http.Request) {
	items, err := a.store.ListProviderCosts()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"items": items, "total": len(items)})
}

func (a adminAPI) updateProviderCostV1(w http.ResponseWriter, r *http.Request) {
	var req providerCostMutation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	item, err := a.store.UpdateProviderCost(r.PathValue("id"), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (a adminAPI) reconciliationV1(w http.ResponseWriter, _ *http.Request) {
	items, err := a.store.ListBillingReconciliation()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	abnormal := 0
	anomalyCounts := map[string]int{}
	for _, item := range items {
		if len(item.Anomalies) > 0 {
			abnormal++
		}
		for _, anomaly := range item.Anomalies {
			anomalyCounts[anomaly]++
		}
	}
	writeJSON(w, map[string]any{
		"summary": map[string]any{"tasks": len(items), "normal": len(items) - abnormal, "abnormal": abnormal, "anomalyCounts": anomalyCounts},
		"items":   items,
	})
}

func (a adminAPI) walletLedgerV1(w http.ResponseWriter, _ *http.Request) {
	items, err := a.store.ListWalletLedger()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	totals := map[string]float64{}
	for _, item := range items {
		totals[item.EntryType] += item.Points
	}
	writeJSON(w, map[string]any{"summary": map[string]any{"entries": len(items), "totals": totals}, "items": items})
}

func (a adminAPI) billingEventsV1Payload() (map[string]any, error) {
	items, err := a.store.ListBillingLifecycleEvents()
	if err != nil {
		return nil, err
	}
	data, err := a.store.AdminData()
	if err != nil {
		return nil, err
	}
	totals := map[string]int{}
	for _, item := range items {
		totals[item.EventType]++
	}
	return map[string]any{
		"summary": map[string]any{"events": len(items), "types": totals},
		"items":   items, "events": items,
		"legacyEvents": data.BillingEvents,
	}, nil
}

func (a adminAPI) billingOverviewV1Payload() (map[string]any, error) {
	rules, err := a.store.ListBillingRuleVersions()
	if err != nil {
		return nil, err
	}
	costs, err := a.store.ListProviderCosts()
	if err != nil {
		return nil, err
	}
	reconciliation, err := a.store.ListBillingReconciliation()
	if err != nil {
		return nil, err
	}
	ledger, err := a.store.ListWalletLedger()
	if err != nil {
		return nil, err
	}
	published, drafts, abnormal := 0, 0, 0
	margin := 0.0
	marginTasks := 0
	for _, item := range rules {
		switch upperTrim(item.Status) {
		case "PUBLISHED":
			published++
		case "DRAFT":
			drafts++
		}
	}
	for _, item := range reconciliation {
		if len(item.Anomalies) > 0 {
			abnormal++
		}
		if item.EstimatedMargin != nil {
			margin += *item.EstimatedMargin
			marginTasks++
		}
	}
	return map[string]any{
		"summary": map[string]any{
			"publishedRules": published, "draftRules": drafts, "providerCosts": len(costs),
			"tasks": len(reconciliation), "abnormalTasks": abnormal, "walletEntries": len(ledger),
			"estimatedMargin": margin, "marginTasks": marginTasks,
		},
		"recentTasks":  firstReconciliationItems(reconciliation, 8),
		"recentLedger": firstWalletLedgerItems(ledger, 8),
	}, nil
}

func firstReconciliationItems(items []billingReconciliationItem, limit int) []billingReconciliationItem {
	if len(items) <= limit {
		return items
	}
	return items[:limit]
}

func firstWalletLedgerItems(items []walletLedgerEntry, limit int) []walletLedgerEntry {
	if len(items) <= limit {
		return items
	}
	return items[:limit]
}
