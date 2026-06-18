package httpserver

type platformData struct {
	Users           []adminUser         `json:"users,omitempty"`
	Plans           []adminPlan         `json:"plans,omitempty"`
	PointAccounts   []adminPointAccount `json:"pointAccounts,omitempty"`
	Orders          []adminOrder        `json:"orders,omitempty"`
	Payments        []adminPayment      `json:"payments,omitempty"`
	ChannelAgents   []adminChannelAgent `json:"channelAgents,omitempty"`
	Commissions     []adminCommission   `json:"commissions,omitempty"`
	Withdrawals     []adminWithdrawal   `json:"withdrawals,omitempty"`
	Presentations   []adminPresentation `json:"presentations,omitempty"`
	Agents          []adminAgent        `json:"agents,omitempty"`
	AgentCalls      []adminAgentCall    `json:"agentCalls,omitempty"`
	GeoBrands       []adminGeoBrand     `json:"geoBrands,omitempty"`
	GeoTasks        []adminGeoTask      `json:"geoTasks,omitempty"`
	AdminProducts   []adminProduct      `json:"adminProducts,omitempty"`
	GenerationTasks []generationTask    `json:"generationTasks"`
	Assets          []asset             `json:"assets"`
	Counters        map[string]int      `json:"counters"`
	PointsAvailable *int                `json:"pointsAvailable,omitempty"`
}

type generationTask struct {
	ID               string         `json:"id"`
	UserID           string         `json:"userId"`
	Type             string         `json:"type"`
	Prompt           string         `json:"prompt"`
	Params           map[string]any `json:"params"`
	Model            string         `json:"model"`
	Status           string         `json:"status"`
	Progress         int            `json:"progress"`
	PointCost        int            `json:"pointCost"`
	ResultIDs        []string       `json:"resultIds"`
	Error            any            `json:"error"`
	CreatedAt        string         `json:"createdAt"`
	UpdatedAt        string         `json:"updatedAt"`
	WorkerFinishedAt string         `json:"workerFinishedAt,omitempty"`
}

type asset struct {
	ID        string         `json:"id"`
	UserID    string         `json:"userId"`
	TaskID    string         `json:"taskId"`
	Name      string         `json:"name"`
	MediaType string         `json:"mediaType"`
	URL       string         `json:"url"`
	Favorite  bool           `json:"favorite"`
	Metadata  map[string]any `json:"metadata"`
	CreatedAt string         `json:"createdAt"`
	UpdatedAt string         `json:"updatedAt"`
}

type createGenerationTaskRequest struct {
	Type            string           `json:"type"`
	Prompt          string           `json:"prompt"`
	Model           string           `json:"model"`
	Params          map[string]any   `json:"params"`
	GeneratedImages []generatedImage `json:"-"`
}

type pointAccount struct {
	ID        string `json:"id"`
	UserID    string `json:"userId"`
	Available int    `json:"available"`
	Frozen    int    `json:"frozen"`
}

type generatedImage struct {
	URL         string
	ContentType string
	Width       int
	Height      int
	Source      string
}
