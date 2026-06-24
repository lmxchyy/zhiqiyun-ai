package httpserver

import (
	"net/http"
	"strings"
)

type channelAPI struct {
	store    platformStore
	sessions authSessionStore
}

func newChannelAPI(store platformStore, sessions authSessionStore) channelAPI {
	return channelAPI{store: store, sessions: sessions}
}

func (a channelAPI) me(w http.ResponseWriter, r *http.Request) {
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	user, err := authAPI{store: a.store, sessions: a.sessions}.authenticatedUser(r, data)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	agent, ok := channelAgentForUser(data.ChannelAgents, user.ID)
	if !ok || !strings.HasPrefix(user.Role, "AGENT") {
		writeError(w, http.StatusForbidden, errForbidden)
		return
	}

	customers := channelCustomers(data.Users, user.ID)
	commissions := channelCommissions(data.Commissions, agent.ID)
	withdrawals := channelWithdrawals(data.Withdrawals, agent.ID)
	children := channelChildren(data.ChannelAgents, data.Users, agent.ID)

	writeJSON(w, map[string]any{
		"user":        userView(user),
		"agent":       channelAgentView(agent, user),
		"summary":     channelSummary(customers, commissions, withdrawals, children),
		"customers":   customers,
		"commissions": commissions,
		"withdrawals": withdrawals,
		"children":    children,
	})
}

func channelCustomers(users []adminUser, agentUserID string) []map[string]any {
	items := []map[string]any{}
	for _, user := range users {
		if user.ReferredBy != agentUserID {
			continue
		}
		items = append(items, userView(user))
	}
	return items
}

func channelCommissions(items []adminCommission, agentID string) []adminCommission {
	filtered := []adminCommission{}
	for _, item := range items {
		if item.AgentID == agentID {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func channelWithdrawals(items []adminWithdrawal, agentID string) []adminWithdrawal {
	filtered := []adminWithdrawal{}
	for _, item := range items {
		if item.AgentID == agentID {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func channelChildren(agents []adminChannelAgent, users []adminUser, parentID string) []map[string]any {
	usersByID := userMap(users)
	items := []map[string]any{}
	for _, agent := range agents {
		if agent.ParentID != parentID {
			continue
		}
		items = append(items, channelAgentView(agent, usersByID[agent.UserID]))
	}
	return items
}

func channelSummary(customers []map[string]any, commissions []adminCommission, withdrawals []adminWithdrawal, children []map[string]any) map[string]any {
	totalCommission := 0
	settledCommission := 0
	pendingCommission := 0
	for _, item := range commissions {
		totalCommission += item.AmountCents
		switch strings.ToUpper(item.Status) {
		case "SETTLED", "PAID", "APPROVED":
			settledCommission += item.AmountCents
		default:
			pendingCommission += item.AmountCents
		}
	}
	withdrawn := 0
	pendingWithdrawal := 0
	for _, item := range withdrawals {
		switch strings.ToUpper(item.Status) {
		case "APPROVED", "PAID", "SETTLED":
			withdrawn += item.AmountCents
		default:
			pendingWithdrawal += item.AmountCents
		}
	}
	return map[string]any{
		"directCustomers":     len(customers),
		"childAgents":         len(children),
		"totalCommission":     totalCommission,
		"settledCommission":   settledCommission,
		"pendingCommission":   pendingCommission,
		"withdrawn":           withdrawn,
		"pendingWithdrawal":   pendingWithdrawal,
		"availableToWithdraw": settledCommission - withdrawn,
	}
}
