package httpserver

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"xianzhi-ai/backend-go/internal/config"
)

func TestAdminCustomerAttributionEndpoint(t *testing.T) {
	server := New(config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "store.json"), StaticDir: t.TempDir()})
	handler := server.Handler
	adminToken := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")
	memberToken := loginToken(t, handler, "demo@xianzhi.ai", "Demo123!")

	forbidden := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/customer-attributions", nil, memberToken)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("member attribution overview = %d %s", forbidden.Code, forbidden.Body.String())
	}

	response := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/customer-attributions?page=1&pageSize=20", nil, adminToken)
	if response.Code != http.StatusOK {
		t.Fatalf("admin attribution overview = %d %s", response.Code, response.Body.String())
	}
	var result adminCustomerAttributionResult
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Stats.Total == 0 || result.Total == 0 || len(result.Items) == 0 {
		t.Fatalf("expected attribution rows, got %+v", result)
	}
	if len(result.Filters.Agents) == 0 {
		t.Fatalf("expected agent filter options, got %+v", result.Filters)
	}
}

func TestBuildAdminCustomerAttributionRowsDetectsCompleteAndAnomaly(t *testing.T) {
	data := adminPlatformData{
		Users: []adminUser{
			{ID: "customer-a", Name: "Customer A", Role: "MEMBER", Status: "ACTIVE", CreatedAt: "2026-07-01T00:00:00Z"},
			{ID: "agent-user", Name: "Agent A", Role: "AGENT_L1", Status: "ACTIVE"},
			{ID: "center-user", Name: "Center A", Role: "OPERATION_CENTER", Status: "ACTIVE"},
		},
		ChannelAgents:    []adminChannelAgent{{ID: "agent-a", UserID: "agent-user", OperationCenterID: "center-a", Level: 1, Status: "ACTIVE"}},
		OperationCenters: []adminOperationCenter{{ID: "center-a", UserID: "center-user", Name: "Center A", Status: "ACTIVE"}},
		CustomerRelations: []adminCustomerRelation{{
			ID: "relation-a", CustomerUserID: "customer-a", DirectAgentID: "agent-a", OperationCenterID: "center-a",
			BindType: "INVITE", Status: "ACTIVE", CreatedAt: "2026-07-01T00:00:00Z",
		}},
	}
	enterprises := []adminEnterpriseListItem{{
		ID: "enterprise-a", Name: "Enterprise A", SourceAgent: adminEnterpriseRelationSummary{ID: "agent-a"},
		OperationCenter: adminEnterpriseRelationSummary{ID: "missing-center"}, Status: "ACTIVE", CreatedAt: "2026-07-02T00:00:00Z",
	}}

	rows := buildAdminCustomerAttributionRows(data, enterprises)
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	statuses := map[string]string{}
	for _, row := range rows {
		statuses[row.CustomerID] = row.HealthStatus
	}
	if statuses["customer-a"] != attributionHealthComplete {
		t.Fatalf("personal health = %s", statuses["customer-a"])
	}
	if statuses["enterprise-a"] != attributionHealthAnomaly {
		t.Fatalf("enterprise health = %s", statuses["enterprise-a"])
	}
}
