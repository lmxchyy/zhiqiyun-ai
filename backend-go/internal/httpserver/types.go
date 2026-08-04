package httpserver

import "xianzhi-ai/backend-go/internal/app/generation"

type platformData struct {
	Users                  []adminUser              `json:"users,omitempty"`
	Plans                  []adminPlan              `json:"plans,omitempty"`
	PointAccounts          []adminPointAccount      `json:"pointAccounts,omitempty"`
	TokenRecords           []adminTokenRecord       `json:"tokenRecords,omitempty"`
	Orders                 []adminOrder             `json:"orders,omitempty"`
	Payments               []adminPayment           `json:"payments,omitempty"`
	PaymentEvents          []adminPaymentEvent      `json:"paymentEvents,omitempty"`
	ChannelAgents          []adminChannelAgent      `json:"channelAgents,omitempty"`
	OperationCenters       []adminOperationCenter   `json:"operationCenters,omitempty"`
	CustomerRelations      []adminCustomerRelation  `json:"customerRelations,omitempty"`
	Commissions            []adminCommission        `json:"commissions,omitempty"`
	CommissionRules        []adminCommissionRule    `json:"commissionRules,omitempty"`
	BillingRules           []adminBillingRule       `json:"billingRules,omitempty"`
	BillingEvents          []adminBillingEvent      `json:"billingEvents,omitempty"`
	BillingRuleVersions    []billingRuleVersion     `json:"billingRuleVersions,omitempty"`
	ProviderCosts          []providerCost           `json:"providerCosts,omitempty"`
	BillingLifecycleEvents []billingLifecycleEvent  `json:"billingLifecycleEvents,omitempty"`
	WalletLedger           []walletLedgerEntry      `json:"walletLedger,omitempty"`
	PersonalPoints         personalPointState       `json:"personalPoints,omitempty"`
	PersonalPointImport    personalPointImportState `json:"personalPointImport,omitempty"`
	Withdrawals            []adminWithdrawal        `json:"withdrawals,omitempty"`
	Presentations          []adminPresentation      `json:"presentations,omitempty"`
	Agents                 []adminAgent             `json:"agents,omitempty"`
	AgentCalls             []adminAgentCall         `json:"agentCalls,omitempty"`
	GeoBrands              []adminGeoBrand          `json:"geoBrands,omitempty"`
	GeoTasks               []adminGeoTask           `json:"geoTasks,omitempty"`
	AdminProducts          []adminProduct           `json:"adminProducts,omitempty"`
	AIModules              []adminAIModule          `json:"aiModules,omitempty"`
	AIModels               []adminAIModel           `json:"aiModels,omitempty"`
	AIParameterSchemas     []adminAIParameterSchema `json:"aiParameterSchemas,omitempty"`
	TenantModuleLimits     []adminTenantModuleLimit `json:"tenantModuleLimits,omitempty"`
	GenerationTasks        []generationTask         `json:"generationTasks"`
	Assets                 []asset                  `json:"assets"`
	AIState                userAIState              `json:"aiState,omitempty"`
	Counters               map[string]int           `json:"counters"`
	PointsAvailable        *int                     `json:"pointsAvailable,omitempty"`
}

type generationTask struct {
	ID                        string         `json:"id"`
	ClientRequestID           string         `json:"clientRequestId,omitempty"`
	UserID                    string         `json:"userId"`
	TenantID                  string         `json:"tenantId,omitempty"`
	OrganizationID            string         `json:"organizationId,omitempty"`
	BillingAccountType        string         `json:"billingAccountType,omitempty"`
	BillingAccountID          string         `json:"billingAccountId,omitempty"`
	AgentID                   string         `json:"agentId,omitempty"`
	OperationCenterID         string         `json:"operationCenterId,omitempty"`
	ModuleCode                string         `json:"moduleCode,omitempty"`
	Type                      string         `json:"type"`
	Prompt                    string         `json:"prompt"`
	Params                    map[string]any `json:"params"`
	Model                     string         `json:"model"`
	BillingType               string         `json:"billingType,omitempty"`
	Status                    string         `json:"status"`
	TaskStatus                string         `json:"taskStatus,omitempty"`
	BillingStatus             string         `json:"billingStatus,omitempty"`
	Progress                  int            `json:"progress"`
	PointCost                 int            `json:"pointCost"`
	BillingRuleVersionID      string         `json:"billingRuleVersionId,omitempty"`
	QuotedPoints              float64        `json:"quotedPoints,omitempty"`
	ReservedPoints            float64        `json:"reservedPoints,omitempty"`
	CapturedPoints            float64        `json:"capturedPoints,omitempty"`
	ReleasedPoints            float64        `json:"releasedPoints,omitempty"`
	RefundedPoints            float64        `json:"refundedPoints,omitempty"`
	SupplierCost              *float64       `json:"supplierCost,omitempty"`
	EstimatedMargin           *float64       `json:"estimatedMargin,omitempty"`
	ProviderChannel           string         `json:"providerChannel,omitempty"`
	ResultIDs                 []string       `json:"resultIds"`
	ImageURL                  string         `json:"imageUrl,omitempty"`
	OutputURL                 string         `json:"outputUrl,omitempty"`
	ResultURL                 string         `json:"resultUrl,omitempty"`
	ThumbnailURL              string         `json:"thumbnailUrl,omitempty"`
	FinalSchemaSnapshot       map[string]any `json:"finalSchemaSnapshot,omitempty"`
	LimitSnapshot             map[string]any `json:"limitSnapshot,omitempty"`
	UpstreamProvider          string         `json:"upstreamProvider,omitempty"`
	UpstreamRequestID         string         `json:"upstreamRequestId,omitempty"`
	UserChargeAmount          int            `json:"userChargeAmount,omitempty"`
	UpstreamCost              int            `json:"upstreamCost,omitempty"`
	PlatformProfit            int            `json:"platformProfit,omitempty"`
	AgentCommission           int            `json:"agentCommission,omitempty"`
	OperationCenterCommission int            `json:"operationCenterCommission,omitempty"`
	FailureReason             string         `json:"failureReason,omitempty"`
	Error                     any            `json:"error"`
	CreatedAt                 string         `json:"createdAt"`
	UpdatedAt                 string         `json:"updatedAt"`
	WorkerFinishedAt          string         `json:"workerFinishedAt,omitempty"`
}

type asset struct {
	ID             string         `json:"id"`
	UserID         string         `json:"userId"`
	TenantID       string         `json:"tenantId,omitempty"`
	OrganizationID string         `json:"organizationId,omitempty"`
	TaskID         string         `json:"taskId"`
	Name           string         `json:"name"`
	MediaType      string         `json:"mediaType"`
	URL            string         `json:"url"`
	ThumbnailURL   string         `json:"thumbnailUrl,omitempty"`
	Favorite       bool           `json:"favorite"`
	Metadata       map[string]any `json:"metadata"`
	DeletedAt      string         `json:"deletedAt,omitempty"`
	CreatedAt      string         `json:"createdAt"`
	UpdatedAt      string         `json:"updatedAt"`
}

type createGenerationTaskRequest = generation.CreateRequest

type pointAccount struct {
	ID        string `json:"id"`
	UserID    string `json:"userId"`
	Available int    `json:"available"`
	Frozen    int    `json:"frozen"`
	Total     int    `json:"total"`
}

type generatedImage = generation.GeneratedImage

type assetImageInfo struct {
	ThumbnailURL string
	Width        int
	Height       int
}

type aiFavoriteCollection struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	TaskIDs   []string `json:"taskIds"`
	CreatedAt string   `json:"createdAt,omitempty"`
	UpdatedAt string   `json:"updatedAt,omitempty"`
}

type aiAgentMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	CreatedAt string `json:"createdAt,omitempty"`
}

type aiAgentConversation struct {
	ID        string           `json:"id"`
	Title     string           `json:"title"`
	Messages  []aiAgentMessage `json:"messages"`
	CreatedAt string           `json:"createdAt"`
	UpdatedAt string           `json:"updatedAt,omitempty"`
}

type userAIState struct {
	UserID               string                 `json:"userId"`
	FavoriteTaskIDs      []string               `json:"favoriteTaskIds"`
	HiddenTaskIDs        []string               `json:"hiddenTaskIds"`
	FavoriteCollections  []aiFavoriteCollection `json:"favoriteCollections"`
	DefaultCollectionID  string                 `json:"defaultFavoriteCollectionId,omitempty"`
	AgentConversations   []aiAgentConversation  `json:"agentConversations"`
	ActiveConversationID string                 `json:"activeConversationId,omitempty"`
	ActiveCollectionID   string                 `json:"activeCollectionId,omitempty"`
	UpdatedAt            string                 `json:"updatedAt,omitempty"`
}
