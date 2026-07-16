package httpserver

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
)

const (
	attributionHealthComplete   = "COMPLETE"
	attributionHealthPartial    = "PARTIAL"
	attributionHealthUnassigned = "UNASSIGNED"
	attributionHealthAnomaly    = "ANOMALY"
)

type adminCustomerAttributionParty struct {
	ID     string `json:"id,omitempty"`
	UserID string `json:"userId,omitempty"`
	Name   string `json:"name,omitempty"`
	Level  int    `json:"level,omitempty"`
}

type adminCustomerAttributionItem struct {
	ID              string                        `json:"id"`
	CustomerType    string                        `json:"customerType"`
	CustomerID      string                        `json:"customerId"`
	CustomerName    string                        `json:"customerName"`
	Email           string                        `json:"email,omitempty"`
	DirectAgent     adminCustomerAttributionParty `json:"directAgent"`
	ParentAgent     adminCustomerAttributionParty `json:"parentAgent"`
	OperationCenter adminCustomerAttributionParty `json:"operationCenter"`
	BindType        string                        `json:"bindType"`
	BindAt          string                        `json:"bindAt,omitempty"`
	RelationStatus  string                        `json:"relationStatus"`
	HealthStatus    string                        `json:"healthStatus"`
	Issues          []string                      `json:"issues"`
	Source          string                        `json:"source"`
	CreatedAt       string                        `json:"createdAt,omitempty"`
}

type adminCustomerAttributionStats struct {
	Total      int `json:"total"`
	Complete   int `json:"complete"`
	Partial    int `json:"partial"`
	Unassigned int `json:"unassigned"`
	Anomaly    int `json:"anomaly"`
}

type adminCustomerAttributionOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type adminCustomerAttributionFilters struct {
	Agents           []adminCustomerAttributionOption `json:"agents"`
	OperationCenters []adminCustomerAttributionOption `json:"operationCenters"`
}

type adminCustomerAttributionResult struct {
	Items    []adminCustomerAttributionItem  `json:"items"`
	Total    int                             `json:"total"`
	Page     int                             `json:"page"`
	PageSize int                             `json:"pageSize"`
	Stats    adminCustomerAttributionStats   `json:"stats"`
	Filters  adminCustomerAttributionFilters `json:"filters"`
}

type adminCustomerAttributionQuery struct {
	Page              int
	PageSize          int
	Keyword           string
	CustomerType      string
	HealthStatus      string
	AgentID           string
	OperationCenterID string
}

func (a adminAPI) customerAttributions(w http.ResponseWriter, r *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	enterprises := []adminEnterpriseListItem{}
	if enterpriseStore, ok := a.store.(adminEnterpriseStore); ok {
		result, listErr := enterpriseStore.ListAdminEnterprises(adminEnterpriseListQuery{Page: 1, PageSize: 5000})
		if listErr != nil {
			writeAdminEnterpriseError(w, listErr)
			return
		}
		enterprises = result.Items
	}
	writeJSON(w, buildAdminCustomerAttributionResult(data, enterprises, parseAdminCustomerAttributionQuery(r)))
}

func parseAdminCustomerAttributionQuery(r *http.Request) adminCustomerAttributionQuery {
	values := r.URL.Query()
	page, _ := strconv.Atoi(values.Get("page"))
	pageSize, _ := strconv.Atoi(values.Get("pageSize"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	return adminCustomerAttributionQuery{
		Page:              page,
		PageSize:          pageSize,
		Keyword:           strings.ToLower(strings.TrimSpace(values.Get("keyword"))),
		CustomerType:      strings.ToUpper(strings.TrimSpace(values.Get("customerType"))),
		HealthStatus:      strings.ToUpper(strings.TrimSpace(values.Get("healthStatus"))),
		AgentID:           strings.TrimSpace(values.Get("agentId")),
		OperationCenterID: strings.TrimSpace(values.Get("operationCenterId")),
	}
}

func buildAdminCustomerAttributionResult(data adminPlatformData, enterprises []adminEnterpriseListItem, query adminCustomerAttributionQuery) adminCustomerAttributionResult {
	rows := buildAdminCustomerAttributionRows(data, enterprises)
	stats := summarizeAdminCustomerAttributions(rows)
	filtered := make([]adminCustomerAttributionItem, 0, len(rows))
	for _, row := range rows {
		if matchesAdminCustomerAttribution(row, query) {
			filtered = append(filtered, row)
		}
	}
	total := len(filtered)
	start := (query.Page - 1) * query.PageSize
	if start > total {
		start = total
	}
	end := start + query.PageSize
	if end > total {
		end = total
	}
	return adminCustomerAttributionResult{
		Items:    append([]adminCustomerAttributionItem(nil), filtered[start:end]...),
		Total:    total,
		Page:     query.Page,
		PageSize: query.PageSize,
		Stats:    stats,
		Filters:  buildAdminCustomerAttributionFilters(data),
	}
}

func buildAdminCustomerAttributionRows(data adminPlatformData, enterprises []adminEnterpriseListItem) []adminCustomerAttributionItem {
	users := userMap(data.Users)
	agentsByID := agentByIDMap(data.ChannelAgents)
	agentsByUser := agentByUserMap(data.ChannelAgents)
	centersByID := map[string]adminOperationCenter{}
	for _, center := range data.OperationCenters {
		centersByID[center.ID] = center
	}
	relations := activeCustomerRelationMap(data.CustomerRelations)
	rows := make([]adminCustomerAttributionItem, 0, len(data.Users)+len(enterprises))
	for _, user := range data.Users {
		if !isAttributionCustomerRole(user.Role) {
			continue
		}
		relation, hasRelation := relations[user.ID]
		directAgentID := strings.TrimSpace(relation.DirectAgentID)
		parentAgentID := strings.TrimSpace(relation.ParentAgentID)
		operationCenterID := strings.TrimSpace(relation.OperationCenterID)
		bindType := strings.TrimSpace(relation.BindType)
		bindAt := firstNonEmptyAttribution(relation.BindStartAt, relation.CreatedAt)
		source := "CUSTOMER_RELATION"
		if !hasRelation {
			referredBy := strings.TrimSpace(user.ReferredBy)
			if sourceAgent, ok := agentsByUser[referredBy]; ok {
				directAgentID = sourceAgent.ID
			} else if _, ok := agentsByID[referredBy]; ok {
				directAgentID = referredBy
			}
			bindType = "REFERRAL"
			bindAt = user.CreatedAt
			source = "USER_REFERRAL"
		}
		if directAgent, ok := agentsByID[directAgentID]; ok {
			if parentAgentID == "" {
				parentAgentID = directAgent.ParentID
			}
			if operationCenterID == "" {
				operationCenterID = directAgent.OperationCenterID
			}
		}
		row := adminCustomerAttributionItem{
			ID:             "PERSONAL:" + user.ID,
			CustomerType:   "PERSONAL",
			CustomerID:     user.ID,
			CustomerName:   firstNonEmptyAttribution(user.Name, user.Email, user.ID),
			Email:          user.Email,
			BindType:       firstNonEmptyAttribution(bindType, "PLATFORM_DIRECT"),
			BindAt:         bindAt,
			RelationStatus: firstNonEmptyAttribution(relation.Status, user.Status),
			Source:         source,
			CreatedAt:      user.CreatedAt,
		}
		resolveAdminCustomerAttribution(&row, directAgentID, parentAgentID, operationCenterID, users, agentsByID, centersByID)
		rows = append(rows, row)
	}
	for _, enterprise := range enterprises {
		row := adminCustomerAttributionItem{
			ID:             "ENTERPRISE:" + enterprise.ID,
			CustomerType:   "ENTERPRISE",
			CustomerID:     enterprise.ID,
			CustomerName:   firstNonEmptyAttribution(enterprise.Name, enterprise.EnterpriseCode, enterprise.ID),
			BindType:       "ENTERPRISE_ATTRIBUTION",
			BindAt:         enterprise.CreatedAt,
			RelationStatus: enterprise.Status,
			Source:         "ENTERPRISE_TENANT",
			CreatedAt:      enterprise.CreatedAt,
		}
		resolveAdminCustomerAttribution(&row, enterprise.SourceAgent.ID, "", enterprise.OperationCenter.ID, users, agentsByID, centersByID)
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].CreatedAt != rows[j].CreatedAt {
			return rows[i].CreatedAt > rows[j].CreatedAt
		}
		return rows[i].ID > rows[j].ID
	})
	return rows
}

func activeCustomerRelationMap(relations []adminCustomerRelation) map[string]adminCustomerRelation {
	result := map[string]adminCustomerRelation{}
	for _, relation := range relations {
		if relation.CustomerUserID == "" || !strings.EqualFold(firstNonEmptyAttribution(relation.Status, "ACTIVE"), "ACTIVE") {
			continue
		}
		current, found := result[relation.CustomerUserID]
		if !found || firstNonEmptyAttribution(relation.UpdatedAt, relation.CreatedAt) > firstNonEmptyAttribution(current.UpdatedAt, current.CreatedAt) {
			result[relation.CustomerUserID] = relation
		}
	}
	return result
}

func isAttributionCustomerRole(role string) bool {
	switch strings.ToUpper(strings.TrimSpace(role)) {
	case "", "USER", "MEMBER", "CUSTOMER":
		return true
	default:
		return false
	}
}

func resolveAdminCustomerAttribution(row *adminCustomerAttributionItem, directAgentID string, parentAgentID string, operationCenterID string, users map[string]adminUser, agents map[string]adminChannelAgent, centers map[string]adminOperationCenter) {
	issues := []string{}
	if directAgentID != "" {
		if agent, ok := agents[directAgentID]; ok {
			row.DirectAgent = attributionAgentParty(agent, users)
			if parentAgentID == "" {
				parentAgentID = agent.ParentID
			}
			if operationCenterID != "" && agent.OperationCenterID != "" && operationCenterID != agent.OperationCenterID {
				issues = append(issues, "OPERATION_CENTER_MISMATCH")
			}
		} else {
			row.DirectAgent.ID = directAgentID
			issues = append(issues, "DIRECT_AGENT_NOT_FOUND")
		}
	}
	if parentAgentID != "" {
		if agent, ok := agents[parentAgentID]; ok {
			row.ParentAgent = attributionAgentParty(agent, users)
		} else {
			row.ParentAgent.ID = parentAgentID
			issues = append(issues, "PARENT_AGENT_NOT_FOUND")
		}
	}
	if operationCenterID != "" {
		if center, ok := centers[operationCenterID]; ok {
			row.OperationCenter = adminCustomerAttributionParty{ID: center.ID, UserID: center.UserID, Name: firstNonEmptyAttribution(center.Name, users[center.UserID].Name, center.ID)}
		} else {
			row.OperationCenter.ID = operationCenterID
			issues = append(issues, "OPERATION_CENTER_NOT_FOUND")
		}
	}
	row.Issues = issues
	switch {
	case len(issues) > 0:
		row.HealthStatus = attributionHealthAnomaly
	case directAgentID == "" && operationCenterID == "":
		row.HealthStatus = attributionHealthUnassigned
		row.Issues = []string{"ATTRIBUTION_UNASSIGNED"}
	case directAgentID == "" || operationCenterID == "":
		row.HealthStatus = attributionHealthPartial
		if directAgentID == "" {
			row.Issues = []string{"DIRECT_AGENT_UNASSIGNED"}
		} else {
			row.Issues = []string{"OPERATION_CENTER_UNASSIGNED"}
		}
	default:
		row.HealthStatus = attributionHealthComplete
		row.Issues = []string{}
	}
}

func attributionAgentParty(agent adminChannelAgent, users map[string]adminUser) adminCustomerAttributionParty {
	return adminCustomerAttributionParty{
		ID: agent.ID, UserID: agent.UserID, Name: firstNonEmptyAttribution(users[agent.UserID].Name, users[agent.UserID].Email, agent.ID), Level: agent.Level,
	}
}

func summarizeAdminCustomerAttributions(rows []adminCustomerAttributionItem) adminCustomerAttributionStats {
	stats := adminCustomerAttributionStats{Total: len(rows)}
	for _, row := range rows {
		switch row.HealthStatus {
		case attributionHealthComplete:
			stats.Complete++
		case attributionHealthPartial:
			stats.Partial++
		case attributionHealthUnassigned:
			stats.Unassigned++
		case attributionHealthAnomaly:
			stats.Anomaly++
		}
	}
	return stats
}

func matchesAdminCustomerAttribution(row adminCustomerAttributionItem, query adminCustomerAttributionQuery) bool {
	if query.CustomerType != "" && row.CustomerType != query.CustomerType {
		return false
	}
	if query.HealthStatus != "" && row.HealthStatus != query.HealthStatus {
		return false
	}
	if query.AgentID != "" && row.DirectAgent.ID != query.AgentID && row.ParentAgent.ID != query.AgentID {
		return false
	}
	if query.OperationCenterID != "" && row.OperationCenter.ID != query.OperationCenterID {
		return false
	}
	if query.Keyword == "" {
		return true
	}
	haystack := strings.ToLower(strings.Join([]string{
		row.CustomerID, row.CustomerName, row.Email, row.DirectAgent.ID, row.DirectAgent.Name,
		row.ParentAgent.ID, row.ParentAgent.Name, row.OperationCenter.ID, row.OperationCenter.Name,
	}, " "))
	return strings.Contains(haystack, query.Keyword)
}

func buildAdminCustomerAttributionFilters(data adminPlatformData) adminCustomerAttributionFilters {
	users := userMap(data.Users)
	filters := adminCustomerAttributionFilters{
		Agents:           make([]adminCustomerAttributionOption, 0, len(data.ChannelAgents)),
		OperationCenters: make([]adminCustomerAttributionOption, 0, len(data.OperationCenters)),
	}
	for _, agent := range data.ChannelAgents {
		label := firstNonEmptyAttribution(users[agent.UserID].Name, users[agent.UserID].Email, agent.ID)
		if agent.Level > 0 {
			label += " · L" + strconv.Itoa(agent.Level)
		}
		filters.Agents = append(filters.Agents, adminCustomerAttributionOption{Value: agent.ID, Label: label})
	}
	for _, center := range data.OperationCenters {
		filters.OperationCenters = append(filters.OperationCenters, adminCustomerAttributionOption{Value: center.ID, Label: firstNonEmptyAttribution(center.Name, users[center.UserID].Name, center.ID)})
	}
	sort.SliceStable(filters.Agents, func(i, j int) bool { return filters.Agents[i].Label < filters.Agents[j].Label })
	sort.SliceStable(filters.OperationCenters, func(i, j int) bool { return filters.OperationCenters[i].Label < filters.OperationCenters[j].Label })
	return filters
}

func firstNonEmptyAttribution(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
