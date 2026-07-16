package httpserver

type adminPlatformData struct {
	Users                  []adminUser              `json:"users"`
	Plans                  []adminPlan              `json:"plans"`
	PointAccounts          []adminPointAccount      `json:"pointAccounts"`
	TokenRecords           []adminTokenRecord       `json:"tokenRecords,omitempty"`
	Orders                 []adminOrder             `json:"orders"`
	Payments               []adminPayment           `json:"payments"`
	PaymentEvents          []adminPaymentEvent      `json:"paymentEvents,omitempty"`
	ChannelAgents          []adminChannelAgent      `json:"channelAgents"`
	OperationCenters       []adminOperationCenter   `json:"operationCenters,omitempty"`
	CustomerRelations      []adminCustomerRelation  `json:"customerRelations,omitempty"`
	Commissions            []adminCommission        `json:"commissions"`
	CommissionRules        []adminCommissionRule    `json:"commissionRules"`
	BillingRules           []adminBillingRule       `json:"billingRules"`
	BillingEvents          []adminBillingEvent      `json:"billingEvents"`
	BillingRuleVersions    []billingRuleVersion     `json:"billingRuleVersions,omitempty"`
	ProviderCosts          []providerCost           `json:"providerCosts,omitempty"`
	BillingLifecycleEvents []billingLifecycleEvent  `json:"billingLifecycleEvents,omitempty"`
	WalletLedger           []walletLedgerEntry      `json:"walletLedger,omitempty"`
	Withdrawals            []adminWithdrawal        `json:"withdrawals"`
	Presentations          []adminPresentation      `json:"presentations"`
	Agents                 []adminAgent             `json:"agents"`
	AgentCalls             []adminAgentCall         `json:"agentCalls"`
	GeoBrands              []adminGeoBrand          `json:"geoBrands"`
	GeoTasks               []adminGeoTask           `json:"geoTasks"`
	AdminProducts          []adminProduct           `json:"adminProducts"`
	SystemSettings         adminSystemSettings      `json:"systemSettings"`
	APIChannels            []adminAPIChannel        `json:"apiChannels"`
	APIModels              []adminAPIModel          `json:"apiModels"`
	APIKeys                []adminAPIKey            `json:"apiKeys"`
	CustomerGroups         []adminCustomerGroup     `json:"customerGroups"`
	AIModules              []adminAIModule          `json:"aiModules"`
	AIModels               []adminAIModel           `json:"aiModels"`
	AIParameterSchemas     []adminAIParameterSchema `json:"aiParameterSchemas"`
	TenantModuleLimits     []adminTenantModuleLimit `json:"tenantModuleLimits"`
	Enterprise             enterpriseMemoryState    `json:"enterprise,omitempty"`
	PromotionRecords       []promotionRecord        `json:"promotionRecords,omitempty"`
	AuthMergeRequests      []adminAuthMergeRequest  `json:"authMergeRequests,omitempty"`
	GenerationTasks        []generationTask         `json:"generationTasks"`
	Assets                 []asset                  `json:"assets"`
	AIState                userAIState              `json:"aiState,omitempty"`
	Counters               map[string]int           `json:"counters"`
	PointsAvailable        *int                     `json:"pointsAvailable,omitempty"`
}

type adminUser struct {
	ID                    string                `json:"id"`
	TenantID              string                `json:"tenantId,omitempty"`
	OrganizationID        string                `json:"organizationId,omitempty"`
	Email                 string                `json:"email"`
	Mobile                string                `json:"mobile,omitempty"`
	WeChatOpenIDs         []string              `json:"wechatOpenIds,omitempty"`
	WeChatUnionID         string                `json:"wechatUnionId,omitempty"`
	RegistrationSource    map[string]string     `json:"registrationSource,omitempty"`
	Name                  string                `json:"name"`
	Role                  string                `json:"role"`
	MemberLevel           string                `json:"memberLevel,omitempty"`
	AgentStatus           string                `json:"agentStatus,omitempty"`
	OperationCenterStatus string                `json:"operationCenterStatus,omitempty"`
	Status                string                `json:"status"`
	PasswordHash          string                `json:"passwordHash,omitempty"`
	PlanID                string                `json:"planId"`
	ReferredBy            string                `json:"referredBy"`
	SubscriptionExpiresAt string                `json:"subscriptionExpiresAt"`
	CreatedAt             string                `json:"createdAt"`
	UpdatedAt             string                `json:"updatedAt"`
	ModelRoutes           []adminUserModelRoute `json:"modelRoutes,omitempty"`
}

type adminUserModelRoute struct {
	ID           string   `json:"id"`
	Provider     string   `json:"provider"`
	ChannelID    string   `json:"channelId"`
	Channel      string   `json:"channel"`
	APIKeyID     string   `json:"apiKeyId"`
	KeyPrefix    string   `json:"keyPrefix"`
	GroupName    string   `json:"groupName"`
	Models       []string `json:"models"`
	QuotaLimit   int      `json:"quotaLimit"`
	QuotaUsed    int      `json:"quotaUsed"`
	Status       string   `json:"status"`
	UpdatedAt    string   `json:"updatedAt"`
	ExternalKey  string   `json:"externalKey,omitempty"`
	ExternalUser string   `json:"externalUser,omitempty"`
}

type adminAuthMergeRequest struct {
	ID              string         `json:"id"`
	PrimaryUserID   string         `json:"primaryUserId"`
	SecondaryUserID string         `json:"secondaryUserId"`
	Mobile          string         `json:"mobile,omitempty"`
	WeChatOpenID    string         `json:"wechatOpenId,omitempty"`
	WeChatUnionID   string         `json:"wechatUnionId,omitempty"`
	ConflictCode    string         `json:"conflictCode"`
	Source          string         `json:"source"`
	Reason          string         `json:"reason,omitempty"`
	Status          string         `json:"status"`
	ReviewComment   string         `json:"reviewComment,omitempty"`
	ResolvedBy      string         `json:"resolvedBy,omitempty"`
	ResolvedAt      string         `json:"resolvedAt,omitempty"`
	CreatedAt       string         `json:"createdAt"`
	UpdatedAt       string         `json:"updatedAt"`
	Raw             map[string]any `json:"raw,omitempty"`
}

type adminAuthMergeRequestMutation struct {
	PrimaryUserID   string         `json:"primaryUserId"`
	SecondaryUserID string         `json:"secondaryUserId"`
	Mobile          string         `json:"mobile"`
	WeChatOpenID    string         `json:"wechatOpenId"`
	WeChatUnionID   string         `json:"wechatUnionId"`
	ConflictCode    string         `json:"conflictCode"`
	Source          string         `json:"source"`
	Reason          string         `json:"reason"`
	Status          string         `json:"status"`
	ReviewComment   string         `json:"reviewComment"`
	ResolvedBy      string         `json:"resolvedBy"`
	Raw             map[string]any `json:"raw"`
}

type adminAuthMergeExecuteRequest struct {
	TargetUserID  string `json:"targetUserId"`
	Confirm       bool   `json:"confirm"`
	ReviewComment string `json:"reviewComment"`
	ResolvedBy    string `json:"resolvedBy"`
}

type adminAuthMergeExecuteResult struct {
	RequestID    string         `json:"requestId"`
	TargetUserID string         `json:"targetUserId"`
	SourceUserID string         `json:"sourceUserId"`
	Moved        map[string]int `json:"moved"`
	Warnings     []string       `json:"warnings,omitempty"`
}

type adminAuthMergePreviewResult struct {
	RequestID    string         `json:"requestId"`
	TargetUserID string         `json:"targetUserId"`
	SourceUserID string         `json:"sourceUserId"`
	Executable   bool           `json:"executable"`
	Moved        map[string]int `json:"moved"`
	Warnings     []string       `json:"warnings,omitempty"`
	Blockers     []string       `json:"blockers,omitempty"`
}

type adminPlan struct {
	ID           string         `json:"id"`
	Code         string         `json:"code"`
	Name         string         `json:"name"`
	PlanType     string         `json:"planType,omitempty"`
	Price        int            `json:"price"`
	PriceCents   int            `json:"priceCents"`
	Points       int            `json:"points"`
	GrantPoints  int            `json:"grantPoints"`
	TokenAmount  int            `json:"tokenAmount,omitempty"`
	MemberLevel  string         `json:"memberLevel,omitempty"`
	AgentLevel   string         `json:"agentLevel,omitempty"`
	DurationDays int            `json:"durationDays"`
	Concurrency  int            `json:"concurrency"`
	Entitlements map[string]any `json:"entitlements"`
	Active       bool           `json:"active"`
}

type adminPointAccount struct {
	ID           string `json:"id"`
	UserID       string `json:"userId"`
	Available    int    `json:"available"`
	Frozen       int    `json:"frozen"`
	TotalGranted int    `json:"totalGranted,omitempty"`
	TotalUsed    int    `json:"totalUsed,omitempty"`
}

type adminTokenRecord struct {
	ID           string `json:"id"`
	UserID       string `json:"userId"`
	OrderID      string `json:"orderId,omitempty"`
	ChangeType   string `json:"changeType"`
	Amount       int    `json:"amount"`
	BalanceAfter int    `json:"balanceAfter"`
	Remark       string `json:"remark,omitempty"`
	CreatedAt    string `json:"createdAt"`
}

type adminOrder struct {
	ID                  string         `json:"id"`
	OrderNo             string         `json:"orderNo,omitempty"`
	TenantID            string         `json:"tenantId,omitempty"`
	UserID              string         `json:"userId"`
	BuyerUserID         string         `json:"buyerUserId,omitempty"`
	PlanID              string         `json:"planId"`
	OrderType           string         `json:"orderType,omitempty"`
	BusinessOrderType   string         `json:"businessOrderType,omitempty"`
	Amount              int            `json:"amount"`
	AmountCents         int            `json:"amountCents"`
	Status              string         `json:"status"`
	DirectAgentID       string         `json:"directAgentId,omitempty"`
	ParentAgentID       string         `json:"parentAgentId,omitempty"`
	OperationCenterID   string         `json:"operationCenterId,omitempty"`
	TokenGrantAmount    int            `json:"tokenGrantAmount,omitempty"`
	TokenAmount         int            `json:"tokenAmount,omitempty"`
	PlatformIncomeCents int            `json:"platformIncomeCents,omitempty"`
	FulfillmentStatus   string         `json:"fulfillmentStatus,omitempty"`
	FulfilledAt         string         `json:"fulfilledAt,omitempty"`
	RewardSnapshot      map[string]any `json:"rewardSnapshot,omitempty"`
	PriceSnapshot       map[string]any `json:"priceSnapshot"`
	PaidAt              string         `json:"paidAt"`
	CreatedAt           string         `json:"createdAt"`
}

type adminPayment struct {
	ID        string `json:"id"`
	OrderID   string `json:"orderId"`
	Channel   string `json:"channel"`
	Amount    int    `json:"amount"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
}

type adminPaymentEvent struct {
	ID             string         `json:"id"`
	TenantID       string         `json:"tenantId,omitempty"`
	Provider       string         `json:"provider"`
	EventID        string         `json:"eventId"`
	IdempotencyKey string         `json:"idempotencyKey"`
	OrderID        string         `json:"orderId"`
	TransactionID  string         `json:"transactionId,omitempty"`
	AmountCents    int            `json:"amountCents"`
	Verified       bool           `json:"verified"`
	Status         string         `json:"status"`
	ProcessedAt    string         `json:"processedAt,omitempty"`
	Raw            map[string]any `json:"raw,omitempty"`
	CreatedAt      string         `json:"createdAt"`
}

type adminChannelAgent struct {
	ID                string `json:"id"`
	UserID            string `json:"userId"`
	ParentID          string `json:"parentId"`
	OperationCenterID string `json:"operationCenterId,omitempty"`
	Level             int    `json:"level"`
	Status            string `json:"status"`
	InviteCode        string `json:"inviteCode"`
	JoinOrderID       string `json:"joinOrderId,omitempty"`
	JoinFeeCents      int    `json:"joinFeeCents,omitempty"`
	TokenRightsAmount int    `json:"tokenRightsAmount,omitempty"`
	CreatedAt         string `json:"createdAt"`
	UpdatedAt         string `json:"updatedAt"`
}

type adminOperationCenter struct {
	ID           string `json:"id"`
	UserID       string `json:"userId"`
	Name         string `json:"name"`
	Region       string `json:"region,omitempty"`
	InviteCode   string `json:"inviteCode"`
	Status       string `json:"status"`
	JoinOrderID  string `json:"joinOrderId,omitempty"`
	JoinFeeCents int    `json:"joinFeeCents,omitempty"`
	ApprovedAt   string `json:"approvedAt,omitempty"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt,omitempty"`
}

type adminCustomerRelation struct {
	ID                string `json:"id"`
	CustomerUserID    string `json:"customerUserId"`
	DirectAgentID     string `json:"directAgentId,omitempty"`
	ParentAgentID     string `json:"parentAgentId,omitempty"`
	OperationCenterID string `json:"operationCenterId,omitempty"`
	BindType          string `json:"bindType"`
	BindStartAt       string `json:"bindStartAt,omitempty"`
	Status            string `json:"status"`
	CreatedAt         string `json:"createdAt"`
	UpdatedAt         string `json:"updatedAt,omitempty"`
}

type adminCommission struct {
	ID             string         `json:"id"`
	OrderID        string         `json:"orderId"`
	AgentID        string         `json:"agentId"`
	ReceiverType   string         `json:"receiverType,omitempty"`
	ReceiverID     string         `json:"receiverId,omitempty"`
	AmountCents    int            `json:"amountCents"`
	CommissionType string         `json:"commissionType,omitempty"`
	Rate           float64        `json:"rate"`
	Status         string         `json:"status"`
	SettleStatus   string         `json:"settleStatus,omitempty"`
	RuleSnapshot   map[string]any `json:"ruleSnapshot"`
	CreatedAt      string         `json:"createdAt"`
	UpdatedAt      string         `json:"updatedAt,omitempty"`
}

type adminCommissionRule struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	OrderType        string         `json:"orderType"`
	EarnerRole       string         `json:"earnerRole"`
	RelationDepth    int            `json:"relationDepth"`
	FixedAmountCents int            `json:"fixedAmountCents"`
	Rate             float64        `json:"rate"`
	MaxTotalRate     float64        `json:"maxTotalRate"`
	Status           string         `json:"status"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	CreatedAt        string         `json:"createdAt,omitempty"`
	UpdatedAt        string         `json:"updatedAt,omitempty"`
}

type adminBillingEvent struct {
	ID                string         `json:"id"`
	TransactionID     string         `json:"transactionId"`
	UserID            string         `json:"userId"`
	AgentID           string         `json:"agentId,omitempty"`
	TenantID          string         `json:"tenantId,omitempty"`
	OperationCenterID string         `json:"operationCenterId,omitempty"`
	ModuleCode        string         `json:"moduleCode,omitempty"`
	TaskID            string         `json:"taskId"`
	AssetIDs          []string       `json:"assetIds"`
	MetricCode        string         `json:"metricCode"`
	Quantity          int            `json:"quantity"`
	UnitAmountCents   int            `json:"unitAmountCents"`
	AmountCents       int            `json:"amountCents"`
	PointCost         int            `json:"pointCost"`
	BalanceBefore     int            `json:"balanceBefore"`
	BalanceAfter      int            `json:"balanceAfter"`
	Model             string         `json:"model"`
	Status            string         `json:"status"`
	OccurredAt        string         `json:"occurredAt"`
	Metadata          map[string]any `json:"metadata"`
}

type adminBillingRule struct {
	ID                       string         `json:"id"`
	ModuleCode               string         `json:"module_code"`
	ModuleCodeCamel          string         `json:"moduleCode,omitempty"`
	ModelName                string         `json:"model_name"`
	ModelNameCamel           string         `json:"modelName,omitempty"`
	ModelCode                string         `json:"model_code,omitempty"`
	BillingUnit              string         `json:"billing_unit,omitempty"`
	BillingType              string         `json:"billing_type"`
	BillingTypeCamel         string         `json:"billingType,omitempty"`
	BasePrice                float64        `json:"base_price"`
	BasePriceCamel           float64        `json:"basePrice,omitempty"`
	MinimumCharge            float64        `json:"minimum_charge,omitempty"`
	CostPrice                float64        `json:"cost_price"`
	CostPriceCamel           float64        `json:"costPrice,omitempty"`
	CurrencyType             string         `json:"currency_type"`
	CurrencyTypeCamel        string         `json:"currencyType,omitempty"`
	ParameterMultiplier      map[string]any `json:"parameter_multiplier"`
	ParameterMultiplierCamel map[string]any `json:"parameterMultiplier,omitempty"`
	Status                   string         `json:"status"`
	RuleSource               string         `json:"rule_source,omitempty"`
	Version                  int            `json:"version,omitempty"`
	CreatedAt                string         `json:"created_at,omitempty"`
	UpdatedAt                string         `json:"updated_at,omitempty"`
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
	Name                  string            `json:"name"`
	Email                 string            `json:"email"`
	Mobile                string            `json:"mobile"`
	WeChatOpenID          string            `json:"wechatOpenId"`
	WeChatUnionID         string            `json:"wechatUnionId"`
	RegistrationSource    map[string]string `json:"registrationSource"`
	Role                  string            `json:"role"`
	Status                string            `json:"status"`
	PlanID                string            `json:"planId"`
	ReferredBy            string            `json:"referredBy"`
	SubscriptionExpiresAt string            `json:"subscriptionExpiresAt"`
	Available             *int              `json:"available"`
	ModelChannelID        string            `json:"modelChannelId"`
	ModelChannel          string            `json:"modelChannel"`
	ModelGroup            string            `json:"modelGroup"`
	ModelModels           string            `json:"modelModels"`
	ModelAPIKey           string            `json:"modelApiKey"`
	ModelKeyStatus        string            `json:"modelKeyStatus"`
	ModelQuotaLimit       int               `json:"modelQuotaLimit"`
	ModelRouteEnabled     *bool             `json:"modelRouteEnabled"`
}

type adminCustomerIdentityMutation struct {
	ClearMobile bool   `json:"clearMobile"`
	ClearWeChat bool   `json:"clearWechat"`
	Status      string `json:"status"`
	Reason      string `json:"reason"`
}

func pointBalancePointer(value int) *int {
	return &value
}

type adminChannelMutation struct {
	Name       string `json:"name"`
	Email      string `json:"email"`
	Level      int    `json:"level"`
	ParentID   string `json:"parentId"`
	Status     string `json:"status"`
	InviteCode string `json:"inviteCode"`
	Available  *int   `json:"available"`
}

type adminChannelCreateMutation struct {
	Name       string `json:"name"`
	Email      string `json:"email"`
	Level      int    `json:"level"`
	ParentID   string `json:"parentId"`
	Status     string `json:"status"`
	InviteCode string `json:"inviteCode"`
	Available  int    `json:"available"`
}

type adminProductMutation struct {
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	Status       string   `json:"status"`
	Entitlements []string `json:"entitlements"`
}

type adminPlanMutation struct {
	Name         string         `json:"name"`
	PriceCents   *int           `json:"priceCents"`
	GrantPoints  *int           `json:"grantPoints"`
	DurationDays *int           `json:"durationDays"`
	Concurrency  *int           `json:"concurrency"`
	Active       *bool          `json:"active"`
	Entitlements map[string]any `json:"entitlements"`
}

type adminOrderMutation struct {
	UserID         string `json:"userId"`
	PlanID         string `json:"planId"`
	AmountCents    int    `json:"amountCents"`
	Status         string `json:"status"`
	PaymentMethod  string `json:"paymentMethod"`
	IdempotencyKey string `json:"idempotencyKey"`
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
	ID                      string   `json:"id"`
	Name                    string   `json:"name"`
	BaseURL                 string   `json:"baseUrl"`
	Protocol                string   `json:"protocol"`
	ImageRequestMode        string   `json:"imageRequestMode"`
	ImageGenerationEndpoint string   `json:"imageGenerationEndpoint"`
	ImageEditEndpoint       string   `json:"imageEditEndpoint"`
	VideoGenerationEndpoint string   `json:"videoGenerationEndpoint"`
	FetchModelsPath         string   `json:"fetchModelsPath"`
	APIKeyEnv               string   `json:"apiKeyEnv"`
	ComfyInstances          []string `json:"comfyInstances"`
	Notes                   string   `json:"notes"`
	Primary                 bool     `json:"primary"`
	Status                  string   `json:"status"`
	Priority                int      `json:"priority"`
	Models                  []string `json:"models"`
	APIKeyConfigured        bool     `json:"apiKeyConfigured"`
	KeyPreview              string   `json:"keyPreview"`
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
	Secret     string   `json:"secret,omitempty"`
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

type adminAIModule struct {
	ID              string         `json:"id"`
	ModuleCode      string         `json:"module_code"`
	ModuleCodeCamel string         `json:"moduleCode,omitempty"`
	Name            string         `json:"name"`
	Description     string         `json:"description"`
	Status          string         `json:"status"`
	OpenTenantIDs   []string       `json:"open_tenant_ids"`
	OpenPackageIDs  []string       `json:"open_package_ids"`
	BoundModels     []string       `json:"bound_models"`
	DefaultSchemaID string         `json:"default_schema_id"`
	AllowAgents     bool           `json:"allow_agents"`
	AllowEndUsers   bool           `json:"allow_end_users"`
	Config          map[string]any `json:"config,omitempty"`
	CreatedAt       string         `json:"created_at,omitempty"`
	UpdatedAt       string         `json:"updated_at,omitempty"`
}

type adminAIModel struct {
	ID                       string   `json:"id"`
	ModelName                string   `json:"model_name"`
	ModelNameCamel           string   `json:"modelName,omitempty"`
	ModelType                string   `json:"model_type"`
	ModelTypeCamel           string   `json:"modelType,omitempty"`
	Provider                 string   `json:"provider"`
	CapabilityCode           []string `json:"capability_code"`
	CapabilityCodeCamel      []string `json:"capabilityCode,omitempty"`
	ModuleCode               string   `json:"module_code"`
	ModuleCodeCamel          string   `json:"moduleCode,omitempty"`
	Status                   string   `json:"status"`
	FallbackModel            string   `json:"fallback_model"`
	FallbackModelCamel       string   `json:"fallbackModel,omitempty"`
	SortWeight               int      `json:"sort_weight"`
	SortWeightCamel          int      `json:"sortWeight,omitempty"`
	AllowFallbackSwitch      bool     `json:"allow_fallback_switch"`
	AllowFallbackSwitchCamel bool     `json:"allowFallbackSwitch,omitempty"`
	CreatedAt                string   `json:"created_at,omitempty"`
	UpdatedAt                string   `json:"updated_at,omitempty"`
}

type adminAIParameterField struct {
	Key          string   `json:"key"`
	Label        string   `json:"label"`
	Type         string   `json:"type"`
	Required     bool     `json:"required"`
	Default      any      `json:"default,omitempty"`
	Options      []any    `json:"options,omitempty"`
	Min          *float64 `json:"min,omitempty"`
	Max          *float64 `json:"max,omitempty"`
	Unit         string   `json:"unit,omitempty"`
	Placeholder  string   `json:"placeholder,omitempty"`
	UserEditable bool     `json:"user_editable"`
	Visible      bool     `json:"visible"`
	HelpText     string   `json:"help_text,omitempty"`
}

type adminAIParameterSchemaJSON struct {
	Fields []adminAIParameterField `json:"fields"`
}

type adminAIParameterSchema struct {
	ID              string                     `json:"id"`
	ModuleCode      string                     `json:"module_code"`
	ModuleCodeCamel string                     `json:"moduleCode,omitempty"`
	ModelName       string                     `json:"model_name"`
	ModelNameCamel  string                     `json:"modelName,omitempty"`
	SchemaJSON      adminAIParameterSchemaJSON `json:"schema_json"`
	Status          string                     `json:"status"`
	CreatedAt       string                     `json:"created_at,omitempty"`
	UpdatedAt       string                     `json:"updated_at,omitempty"`
}

type adminTenantModuleLimit struct {
	ID              string         `json:"id"`
	TenantID        string         `json:"tenant_id"`
	TenantIDCamel   string         `json:"tenantId,omitempty"`
	AgentID         string         `json:"agent_id,omitempty"`
	AgentIDCamel    string         `json:"agentId,omitempty"`
	PackageID       string         `json:"package_id,omitempty"`
	PackageIDCamel  string         `json:"packageId,omitempty"`
	ModuleCode      string         `json:"module_code"`
	ModuleCodeCamel string         `json:"moduleCode,omitempty"`
	ModelName       string         `json:"model_name"`
	ModelNameCamel  string         `json:"modelName,omitempty"`
	LimitJSON       map[string]any `json:"limit_json"`
	LimitJSONCamel  map[string]any `json:"limitJson,omitempty"`
	Status          string         `json:"status"`
	CreatedAt       string         `json:"created_at,omitempty"`
	UpdatedAt       string         `json:"updated_at,omitempty"`
}

type adminAIModuleMutation struct {
	Name            string         `json:"name"`
	Description     string         `json:"description"`
	Status          string         `json:"status"`
	OpenTenantIDs   []string       `json:"open_tenant_ids"`
	OpenPackageIDs  []string       `json:"open_package_ids"`
	BoundModels     []string       `json:"bound_models"`
	DefaultSchemaID string         `json:"default_schema_id"`
	AllowAgents     *bool          `json:"allow_agents"`
	AllowEndUsers   *bool          `json:"allow_end_users"`
	Config          map[string]any `json:"config"`
}

type adminAIModelMutation struct {
	ModelName           string   `json:"model_name"`
	ModelType           string   `json:"model_type"`
	Provider            string   `json:"provider"`
	CapabilityCode      []string `json:"capability_code"`
	ModuleCode          string   `json:"module_code"`
	Status              string   `json:"status"`
	FallbackModel       *string  `json:"fallback_model"`
	SortWeight          int      `json:"sort_weight"`
	AllowFallbackSwitch *bool    `json:"allow_fallback_switch"`
}

type adminAIParameterSchemaMutation struct {
	SchemaJSON adminAIParameterSchemaJSON `json:"schema_json"`
	Status     string                     `json:"status"`
}

type adminTenantModuleLimitMutation struct {
	TenantID  string         `json:"tenant_id"`
	AgentID   string         `json:"agent_id"`
	PackageID string         `json:"package_id"`
	ModelName string         `json:"model_name"`
	LimitJSON map[string]any `json:"limit_json"`
	Status    string         `json:"status"`
}

type adminPlanCapabilityModule struct {
	ModuleCode      string         `json:"moduleCode"`
	Name            string         `json:"name"`
	Description     string         `json:"description"`
	Enabled         bool           `json:"enabled"`
	AllowedModels   []string       `json:"allowedModels"`
	AvailableModels []string       `json:"availableModels"`
	Limits          map[string]any `json:"limits"`
}

type adminPlanCapabilitiesMutation struct {
	Modules []adminPlanCapabilityModule `json:"modules"`
}

type adminBillingRuleMutation struct {
	BillingType         string         `json:"billing_type"`
	BasePrice           float64        `json:"base_price"`
	MinimumCharge       float64        `json:"minimum_charge"`
	CostPrice           float64        `json:"cost_price"`
	CurrencyType        string         `json:"currency_type"`
	ParameterMultiplier map[string]any `json:"parameter_multiplier"`
	Status              string         `json:"status"`
}

type adminSystemMutation struct {
	Brand       adminBrandSetting     `json:"brand"`
	Payments    []adminPaymentChannel `json:"payments"`
	Permissions []string              `json:"permissions"`
	APIGateway  map[string]any        `json:"apiGateway"`
}

type adminNewAPISyncRequest struct {
	ChannelID  string `json:"channelId"`
	GroupName  string `json:"groupName"`
	Models     string `json:"models"`
	QuotaLimit int    `json:"quotaLimit"`
}

type adminAPIChannelMutation struct {
	Name                    string   `json:"name"`
	BaseURL                 string   `json:"baseUrl"`
	Protocol                string   `json:"protocol"`
	ImageRequestMode        string   `json:"imageRequestMode"`
	ImageGenerationEndpoint string   `json:"imageGenerationEndpoint"`
	ImageEditEndpoint       string   `json:"imageEditEndpoint"`
	VideoGenerationEndpoint string   `json:"videoGenerationEndpoint"`
	FetchModelsPath         string   `json:"fetchModelsPath"`
	APIKeyEnv               string   `json:"apiKeyEnv"`
	ComfyInstances          []string `json:"comfyInstances"`
	Notes                   string   `json:"notes"`
	Primary                 bool     `json:"primary"`
	Status                  string   `json:"status"`
	Priority                int      `json:"priority"`
	Models                  []string `json:"models"`
}

type adminAPIChannelTestRequest struct {
	BaseURL          string `json:"baseUrl"`
	APIKey           string `json:"apiKey"`
	Protocol         string `json:"protocol"`
	ImageRequestMode string `json:"imageRequestMode"`
	FetchModelsPath  string `json:"fetchModelsPath"`
	ProbeProtocol    bool   `json:"probeProtocol"`
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
	Secret     string   `json:"secret"`
	APIKey     string   `json:"apiKey"`
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

type adminCommissionRuleMutation struct {
	Name             string  `json:"name"`
	OrderType        string  `json:"orderType"`
	EarnerRole       string  `json:"earnerRole"`
	RelationDepth    int     `json:"relationDepth"`
	FixedAmountCents int     `json:"fixedAmountCents"`
	Rate             float64 `json:"rate"`
	MaxTotalRate     float64 `json:"maxTotalRate"`
	Status           string  `json:"status"`
}

type adminWithdrawalMutation struct {
	AgentID     string `json:"agentId"`
	AmountCents int    `json:"amountCents"`
}
