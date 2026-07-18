package httpserver

import (
	"net/http"
	"os"
	"strconv"
	"strings"
)

type reviewModeConfig struct {
	Enabled                bool `json:"enabled"`
	HideRecharge           bool `json:"hideRecharge"`
	HideWallet             bool `json:"hideWallet"`
	HideInvite             bool `json:"hideInvite"`
	HideCommission         bool `json:"hideCommission"`
	HideAgentCenter        bool `json:"hideAgentCenter"`
	HideOperatorCenter     bool `json:"hideOperatorCenter"`
	HideSensitiveMarketing bool `json:"hideSensitiveMarketing"`
}

func reviewModeFlag(name string) bool {
	value := strings.TrimSpace(os.Getenv(name))
	enabled, err := strconv.ParseBool(value)
	return err == nil && enabled
}

func configuredReviewMode() reviewModeConfig {
	return reviewModeConfig{
		Enabled:                reviewModeFlag("REVIEW_MODE_ENABLED"),
		HideRecharge:           reviewModeFlag("REVIEW_MODE_HIDE_RECHARGE"),
		HideWallet:             reviewModeFlag("REVIEW_MODE_HIDE_WALLET"),
		HideInvite:             reviewModeFlag("REVIEW_MODE_HIDE_INVITE"),
		HideCommission:         reviewModeFlag("REVIEW_MODE_HIDE_COMMISSION"),
		HideAgentCenter:        reviewModeFlag("REVIEW_MODE_HIDE_AGENT_CENTER"),
		HideOperatorCenter:     reviewModeFlag("REVIEW_MODE_HIDE_OPERATOR_CENTER"),
		HideSensitiveMarketing: reviewModeFlag("REVIEW_MODE_HIDE_SENSITIVE_MARKETING"),
	}
}

func (a api) reviewMode(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "public, max-age=60, stale-while-revalidate=300")
	writeJSON(w, configuredReviewMode())
}
