package httpserver

type adminPlatformData struct {
	Users           []adminUser          `json:"users"`
	Plans           []adminPlan          `json:"plans"`
	PointAccounts   []adminPointAccount  `json:"pointAccounts"`
	Orders          []adminOrder         `json:"orders"`
	Payments        []adminPayment       `json:"payments"`
	ChannelAgents   []adminChannelAgent  `json:"channelAgents"`
	Commissions     []adminCommission    `json:"commissions"`
	Withdrawals     []adminWithdrawal    `json:"withdrawals"`
	Presentations   []adminPresentation  `json:"presentations"`
	Agents          []adminAgent         `json:"agents"`
	AgentCalls      []adminAgentCall     `json:"agentCalls"`
	GeoBrands       []adminGeoBrand      `json:"geoBrands"`
	GeoTasks        []adminGeoTask       `json:"geoTasks"`
	AdminProducts   []adminProduct       `json:"adminProducts"`
	SystemSettings  adminSystemSettings  `json:"systemSettings"`
	APIChannels     []adminAPIChannel    `json:"apiChannels"`
	APIModels       []adminAPIModel      `json:"apiModels"`
	APIKeys         []adminAPIKey        `json:"apiKeys"`
	CustomerGroups  []adminCustomerGroup `json:"customerGroups"`
	GenerationTasks []generationTask     `json:"generationTasks"`
	Assets          []asset              `json:"assets"`
	Counters        map[string]int       `json:"counters"`
	PointsAvailable *int                 `json:"pointsAvailable,omitempty"`
}

type adminUser struct {
	ID                    string `json:"id"`
	Email                 string `json:"email"`
	Name                  string `json:"name"`
	Role                  string `json:"role"`
	Status                string `json:"status"`
	PlanID                string `json:"planId"`
	ReferredBy            string `json:"referredBy"`
	SubscriptionExpiresAt string `json:"subscriptionExpiresAt"`
	CreatedAt             string `json:"createdAt"`
	UpdatedAt             string `json:"updatedAt"`
}

type adminPlan struct {
	ID           string         `json:"id"`
	Code         string         `json:"code"`
	Name         string         `json:"name"`
	Price        int            `json:"price"`
	PriceCents   int            `json:"priceCents"`
	Points       int            `json:"points"`
	GrantPoints  int            `json:"grantPoints"`
	DurationDays int            `json:"durationDays"`
	Concurrency  int            `json:"concurrency"`
	Entitlements map[string]any `json:"entitlements"`
	Active       bool           `json:"active"`
}

type adminPointAccount struct {
	ID        string `json:"id"`
	UserID    string `json:"userId"`
	Available int    `json:"available"`
	Frozen    int    `json:"frozen"`
}

type adminOrder struct {
	ID            string         `json:"id"`
	UserID        string         `json:"userId"`
	PlanID        string         `json:"planId"`
	Amount        int            `json:"amount"`
	AmountCents   int            `json:"amountCents"`
	Status        string         `json:"status"`
	PriceSnapshot map[string]any `json:"priceSnapshot"`
	PaidAt        string         `json:"paidAt"`
	CreatedAt     string         `json:"createdAt"`
}

type adminPayment struct {
	ID        string `json:"id"`
	OrderID   string `json:"orderId"`
	Channel   string `json:"channel"`
	Amount    int    `json:"amount"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
}

type adminChannelAgent struct {
	ID         string `json:"id"`
	UserID     string `json:"userId"`
	ParentID   string `json:"parentId"`
	Level      int    `json:"level"`
	Status     string `json:"status"`
	InviteCode string `json:"inviteCode"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
}

type adminCommission struct {
	ID           string         `json:"id"`
	OrderID      string         `json:"orderId"`
	AgentID      string         `json:"agentId"`
	AmountCents  int            `json:"amountCents"`
	Rate         float64        `json:"rate"`
	Status       string         `json:"status"`
	RuleSnapshot map[string]any `json:"ruleSnapshot"`
	CreatedAt    string         `json:"createdAt"`
}

type adminWithdrawal struct {
	ID          string `json:"id"`
	AgentID     string `json:"agentId"`
	AmountCents int    `json:"amountCents"`
	Status      string `json:"status"`
	CreatedAt   string `json:"createdAt"`
	ReviewedAt  string `json:"reviewedAt"`
}

type adminPresentation struct {
	ID        string           `json:"id"`
	UserID    string           `json:"userId"`
	Topic     string           `json:"topic"`
	Theme     string           `json:"theme"`
	Status    string           `json:"status"`
	Slides    []map[string]any `json:"slides"`
	CreatedAt string           `json:"createdAt"`
	UpdatedAt string           `json:"updatedAt"`
}

type adminAgent struct {
	ID          string           `json:"id"`
	OwnerID     string           `json:"ownerId"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Status      string           `json:"status"`
	Version     int              `json:"version"`
	Workflow    []map[string]any `json:"workflow"`
	CallCount   int              `json:"callCount"`
	CreatedAt   string           `json:"createdAt"`
	UpdatedAt   string           `json:"updatedAt"`
}

type adminAgentCall struct {
	ID         string `json:"id"`
	AgentID    string `json:"agentId"`
	UserID     string `json:"userId"`
	TokenUsage int    `json:"tokenUsage"`
	Cost       int    `json:"cost"`
	CostCents  int    `json:"costCents"`
	LatencyMs  int    `json:"latencyMs"`
	CreatedAt  string `json:"createdAt"`
}

type adminGeoBrand struct {
	ID          string   `json:"id"`
	OwnerID     string   `json:"ownerId"`
	Name        string   `json:"name"`
	Competitors []string `json:"competitors"`
	Keywords    []string `json:"keywords"`
	CreatedAt   string   `json:"createdAt"`
}

type adminGeoTask struct {
	ID        string         `json:"id"`
	OwnerID   string         `json:"ownerId"`
	BrandID   string         `json:"brandId"`
	Question  string         `json:"question"`
	Platform  string         `json:"platform"`
	Status    string         `json:"status"`
	Result    map[string]any `json:"result"`
	CreatedAt string         `json:"createdAt"`
}

type adminProduct struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	Status       string   `json:"status"`
	Usage        int      `json:"usage"`
	Entitlements []string `json:"entitlements"`
}

type adminCustomerMutation struct {
	Name      string `json:"name"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	Status    string `json:"status"`
	PlanID    string `json:"planId"`
	Available int    `json:"available"`
}

type adminChannelMutation struct {
	Status string `json:"status"`
}

type adminProductMutation struct {
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	Status       string   `json:"status"`
	Entitlements []string `json:"entitlements"`
}

type adminPlanMutation struct {
	Name         string         `json:"name"`
	PriceCents   int            `json:"priceCents"`
	GrantPoints  int            `json:"grantPoints"`
	DurationDays int            `json:"durationDays"`
	Concurrency  int            `json:"concurrency"`
	Active       bool           `json:"active"`
	Entitlements map[string]any `json:"entitlements"`
}

type adminOrderMutation struct {
	UserID      string `json:"userId"`
	PlanID      string `json:"planId"`
	AmountCents int    `json:"amountCents"`
	Status      string `json:"status"`
}

type adminDeliveryMutation struct {
	Status   string `json:"status"`
	Progress int    `json:"progress"`
}

type adminSystemSettings struct {
	Brand       adminBrandSetting     `json:"brand"`
	Payments    []adminPaymentChannel `json:"payments"`
	Permissions []string              `json:"permissions"`
	APIGateway  map[string]any        `json:"apiGateway"`
}

type adminBrandSetting struct {
	Name   string `json:"name"`
	Domain string `json:"domain"`
	Logo   string `json:"logo"`
}

type adminPaymentChannel struct {
	Channel string `json:"channel"`
	Status  string `json:"status"`
}

type adminAPIChannel struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	BaseURL  string   `json:"baseUrl"`
	Status   string   `json:"status"`
	Priority int      `json:"priority"`
	Models   []string `json:"models"`
}

type adminAPIModel struct {
	ID              string  `json:"id"`
	Model           string  `json:"model"`
	Name            string  `json:"name"`
	Capability      string  `json:"capability"`
	BillingMode     string  `json:"billingMode"`
	FixedQuota      int     `json:"fixedQuota"`
	ModelRatio      float64 `json:"modelRatio"`
	CompletionRatio float64 `json:"completionRatio"`
	Status          string  `json:"status"`
}

type adminAPIKey struct {
	ID         string   `json:"id"`
	Customer   string   `json:"customer"`
	Prefix     string   `json:"prefix"`
	Status     string   `json:"status"`
	Models     []string `json:"models"`
	QuotaLimit int      `json:"quotaLimit"`
}

type adminCustomerGroup struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Ratio       float64  `json:"ratio"`
	Models      []string `json:"models"`
	Description string   `json:"description"`
}

type adminSystemMutation struct {
	Brand       adminBrandSetting     `json:"brand"`
	Payments    []adminPaymentChannel `json:"payments"`
	Permissions []string              `json:"permissions"`
}

type adminAPIChannelMutation struct {
	Name     string   `json:"name"`
	BaseURL  string   `json:"baseUrl"`
	Status   string   `json:"status"`
	Priority int      `json:"priority"`
	Models   []string `json:"models"`
}

type adminAPIModelMutation struct {
	Name            string  `json:"name"`
	Capability      string  `json:"capability"`
	BillingMode     string  `json:"billingMode"`
	FixedQuota      int     `json:"fixedQuota"`
	ModelRatio      float64 `json:"modelRatio"`
	CompletionRatio float64 `json:"completionRatio"`
	Status          string  `json:"status"`
}

type adminAPIKeyMutation struct {
	Customer   string   `json:"customer"`
	Status     string   `json:"status"`
	Models     []string `json:"models"`
	QuotaLimit int      `json:"quotaLimit"`
}

type adminCustomerGroupMutation struct {
	Name        string   `json:"name"`
	Ratio       float64  `json:"ratio"`
	Models      []string `json:"models"`
	Description string   `json:"description"`
}

type adminCommissionMutation struct {
	OrderID     string  `json:"orderId"`
	AgentID     string  `json:"agentId"`
	AmountCents int     `json:"amountCents"`
	Rate        float64 `json:"rate"`
	Status      string  `json:"status"`
}

type adminWithdrawalMutation struct {
	AgentID     string `json:"agentId"`
	AmountCents int    `json:"amountCents"`
}
