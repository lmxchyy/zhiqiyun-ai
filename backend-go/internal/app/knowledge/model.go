package knowledge

import "time"

type TenantType string

const (
	TenantPlatform   TenantType = "PLATFORM"
	TenantEnterprise TenantType = "ENTERPRISE"
	TenantPersonal   TenantType = "PERSONAL"
)

type KnowledgeType string

const (
	KnowledgeEnterprise KnowledgeType = "ENTERPRISE"
	KnowledgeDepartment KnowledgeType = "DEPARTMENT"
	KnowledgePersonal   KnowledgeType = "PERSONAL"
	KnowledgeAgent      KnowledgeType = "AGENT"
)

type AccessContext struct {
	TenantID       string   `json:"tenantId"`
	OrganizationID string   `json:"organizationId,omitempty"`
	UserID         string   `json:"userId"`
	Roles          []string `json:"roles"`
	Permissions    []string `json:"permissions"`
}

func (c AccessContext) HasRole(roles ...string) bool {
	for _, current := range c.Roles {
		for _, expected := range roles {
			if current == expected {
				return true
			}
		}
	}
	return false
}

func (c AccessContext) HasPermission(permission string) bool {
	if c.HasRole("PLATFORM_ADMIN", "ENTERPRISE_ADMIN") {
		return true
	}
	for _, current := range c.Permissions {
		if current == permission || current == "knowledge.manage" {
			return true
		}
	}
	return false
}

type KnowledgeBase struct {
	ID                 string         `json:"id"`
	TenantID           string         `json:"tenantId"`
	OrganizationID     string         `json:"organizationId,omitempty"`
	OwnerUserID        string         `json:"ownerUserId"`
	CategoryID         string         `json:"categoryId,omitempty"`
	KnowledgeType      KnowledgeType  `json:"knowledgeType"`
	Name               string         `json:"name"`
	Description        string         `json:"description"`
	LogoObjectKey      string         `json:"logoObjectKey,omitempty"`
	Visibility         string         `json:"visibility"`
	Status             string         `json:"status"`
	DocumentCount      int64          `json:"documentCount"`
	ChunkCount         int64          `json:"chunkCount"`
	IngestionProfileID string         `json:"ingestionProfileId,omitempty"`
	RetrievalProfileID string         `json:"retrievalProfileId,omitempty"`
	Tags               []Tag          `json:"tags,omitempty"`
	Metadata           map[string]any `json:"metadata,omitempty"`
	Version            int64          `json:"version"`
	CreatedAt          time.Time      `json:"createdAt"`
	UpdatedAt          time.Time      `json:"updatedAt"`
	DeletedAt          *time.Time     `json:"deletedAt,omitempty"`
}

type Tag struct {
	ID       string `json:"id"`
	TenantID string `json:"tenantId"`
	Name     string `json:"name"`
	Color    string `json:"color,omitempty"`
}

type Category struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenantId"`
	ParentID  string    `json:"parentId,omitempty"`
	Name      string    `json:"name"`
	SortOrder int       `json:"sortOrder"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type ACLRule struct {
	ID              string     `json:"id"`
	TenantID        string     `json:"tenantId"`
	KnowledgeBaseID string     `json:"knowledgeBaseId"`
	SubjectType     string     `json:"subjectType"`
	SubjectID       string     `json:"subjectId,omitempty"`
	Permission      string     `json:"permission"`
	Effect          string     `json:"effect"`
	ExpiresAt       *time.Time `json:"expiresAt,omitempty"`
}

type Document struct {
	ID              string         `json:"id"`
	TenantID        string         `json:"tenantId"`
	KnowledgeBaseID string         `json:"knowledgeBaseId"`
	SourceID        string         `json:"sourceId,omitempty"`
	OwnerUserID     string         `json:"ownerUserId"`
	LatestVersionID string         `json:"latestVersionId,omitempty"`
	Name            string         `json:"name"`
	DocumentType    string         `json:"documentType"`
	MIMEType        string         `json:"mimeType"`
	Status          string         `json:"status"`
	Metadata        map[string]any `json:"metadata,omitempty"`
	Version         int64          `json:"version"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
	DeletedAt       *time.Time     `json:"deletedAt,omitempty"`
}

type DocumentVersion struct {
	ID                string         `json:"id"`
	TenantID          string         `json:"tenantId"`
	DocumentID        string         `json:"documentId"`
	VersionNo         int            `json:"versionNo"`
	OriginalObjectKey string         `json:"originalObjectKey,omitempty"`
	PreviewObjectKey  string         `json:"previewObjectKey,omitempty"`
	MIMEType          string         `json:"mimeType"`
	FileSize          int64          `json:"fileSize"`
	ContentHash       string         `json:"contentHash"`
	ParseStatus       string         `json:"parseStatus"`
	ParserMetadata    map[string]any `json:"parserMetadata,omitempty"`
	CreatedBy         string         `json:"createdBy"`
	CreatedAt         time.Time      `json:"createdAt"`
}

type DocumentUnit struct {
	ID                string         `json:"id"`
	TenantID          string         `json:"tenantId"`
	DocumentVersionID string         `json:"documentVersionId"`
	UnitType          string         `json:"unitType"`
	UnitNo            int            `json:"unitNo"`
	Title             string         `json:"title,omitempty"`
	Content           string         `json:"content"`
	Locator           map[string]any `json:"locator,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
}

type Chunk struct {
	ID                string         `json:"id"`
	TenantID          string         `json:"tenantId"`
	KnowledgeBaseID   string         `json:"knowledgeBaseId"`
	DocumentID        string         `json:"documentId"`
	DocumentVersionID string         `json:"documentVersionId"`
	SequenceNo        int            `json:"sequenceNo"`
	ChunkKey          string         `json:"chunkKey"`
	Content           string         `json:"content"`
	TokenCount        int            `json:"tokenCount"`
	PageStart         *int           `json:"pageStart,omitempty"`
	PageEnd           *int           `json:"pageEnd,omitempty"`
	Title             string         `json:"title,omitempty"`
	TitlePath         []string       `json:"titlePath,omitempty"`
	SourceLocator     map[string]any `json:"sourceLocator,omitempty"`
	ContentHash       string         `json:"contentHash"`
	Metadata          map[string]any `json:"metadata,omitempty"`
	Status            string         `json:"status"`
	CreatedAt         time.Time      `json:"createdAt"`
	UpdatedAt         time.Time      `json:"updatedAt"`
	DeletedAt         *time.Time     `json:"deletedAt,omitempty"`
}

type IngestionJob struct {
	ID                 string         `json:"id"`
	TenantID           string         `json:"tenantId"`
	DocumentVersionID  string         `json:"documentVersionId"`
	IngestionProfileID string         `json:"ingestionProfileId,omitempty"`
	IdempotencyKey     string         `json:"idempotencyKey"`
	Stage              string         `json:"stage"`
	Status             string         `json:"status"`
	Attempt            int            `json:"attempt"`
	MaxAttempts        int            `json:"maxAttempts"`
	Progress           int            `json:"progress"`
	ConfigSnapshot     map[string]any `json:"configSnapshot,omitempty"`
	ErrorCode          string         `json:"errorCode,omitempty"`
	ErrorMessage       string         `json:"errorMessage,omitempty"`
	CreatedAt          time.Time      `json:"createdAt"`
	UpdatedAt          time.Time      `json:"updatedAt"`
}

type EmbeddingRuntimeProfile struct {
	ID          string         `json:"id"`
	ProviderKey string         `json:"providerKey"`
	ModelName   string         `json:"modelName"`
	Dimension   int            `json:"dimension"`
	Config      map[string]any `json:"config,omitempty"`
}

type VectorStoreRuntimeProfile struct {
	ID               string         `json:"id"`
	ProviderKey      string         `json:"providerKey"`
	Endpoint         string         `json:"endpoint,omitempty"`
	CredentialRef    string         `json:"credentialRef,omitempty"`
	CollectionPrefix string         `json:"collectionPrefix,omitempty"`
	Config           map[string]any `json:"config,omitempty"`
}

type IngestionRuntimeProfile struct {
	ID             string                    `json:"id"`
	ParserKey      string                    `json:"parserKey"`
	OCRProviderKey string                    `json:"ocrProviderKey,omitempty"`
	ChunkerKey     string                    `json:"chunkerKey"`
	ChunkOptions   ChunkOptions              `json:"chunkOptions"`
	Embedding      EmbeddingRuntimeProfile   `json:"embedding"`
	VectorStore    VectorStoreRuntimeProfile `json:"vectorStore"`
}

type RetrievalRuntimeProfile struct {
	ID                    string                `json:"id"`
	SearchMode            string                `json:"searchMode"`
	TopK                  int                   `json:"topK"`
	Threshold             float64               `json:"threshold"`
	VectorWeight          float64               `json:"vectorWeight"`
	KeywordWeight         float64               `json:"keywordWeight"`
	RerankProfileID       string                `json:"rerankProfileId,omitempty"`
	ContextTokenLimit     int                   `json:"contextTokenLimit"`
	QueryRewriteEnabled   bool                  `json:"queryRewriteEnabled"`
	MetadataFilterEnabled bool                  `json:"metadataFilterEnabled"`
	Config                map[string]any        `json:"config,omitempty"`
	Rerank                *RerankRuntimeProfile `json:"rerank,omitempty"`
}

type RerankRuntimeProfile struct {
	ID             string         `json:"id"`
	ProviderKey    string         `json:"providerKey"`
	ModelName      string         `json:"modelName"`
	CandidateLimit int            `json:"candidateLimit"`
	Config         map[string]any `json:"config,omitempty"`
}

type Agent struct {
	ID           string         `json:"id"`
	TenantID     string         `json:"tenantId"`
	OwnerUserID  string         `json:"ownerUserId"`
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	ModelName    string         `json:"modelName"`
	SystemPrompt string         `json:"systemPrompt,omitempty"`
	Status       string         `json:"status"`
	Version      int64          `json:"version"`
	Config       map[string]any `json:"config,omitempty"`
	CreatedAt    time.Time      `json:"createdAt"`
	UpdatedAt    time.Time      `json:"updatedAt"`
}

type AgentKnowledgeBinding struct {
	ID                 string         `json:"id"`
	TenantID           string         `json:"tenantId"`
	AgentID            string         `json:"agentId"`
	KnowledgeBaseID    string         `json:"knowledgeBaseId"`
	RetrievalProfileID string         `json:"retrievalProfileId,omitempty"`
	Priority           int            `json:"priority"`
	Weight             float64        `json:"weight"`
	Enabled            bool           `json:"enabled"`
	RetrievalOverrides map[string]any `json:"retrievalOverrides,omitempty"`
}

type Conversation struct {
	ID             string     `json:"id"`
	TenantID       string     `json:"tenantId"`
	OrganizationID string     `json:"organizationId,omitempty"`
	AgentID        string     `json:"agentId"`
	UserID         string     `json:"userId"`
	Title          string     `json:"title"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
	DeletedAt      *time.Time `json:"deletedAt,omitempty"`
}

type Message struct {
	ID              string         `json:"id"`
	TenantID        string         `json:"tenantId"`
	ConversationID  string         `json:"conversationId"`
	ParentMessageID string         `json:"parentMessageId,omitempty"`
	Role            string         `json:"role"`
	Content         string         `json:"content"`
	Status          string         `json:"status"`
	InputTokens     int            `json:"inputTokens"`
	OutputTokens    int            `json:"outputTokens"`
	Metadata        map[string]any `json:"metadata,omitempty"`
	CreatedAt       time.Time      `json:"createdAt"`
}

type RAGRun struct {
	ID                  string                  `json:"id"`
	TenantID            string                  `json:"tenantId"`
	ConversationID      string                  `json:"conversationId"`
	UserMessageID       string                  `json:"userMessageId"`
	AssistantMessageID  string                  `json:"assistantMessageId,omitempty"`
	AgentID             string                  `json:"agentId"`
	RetryOfRunID        string                  `json:"retryOfRunId,omitempty"`
	OriginalQuery       string                  `json:"originalQuery"`
	RewrittenQuery      string                  `json:"rewrittenQuery,omitempty"`
	Status              string                  `json:"status"`
	RetrievalLatencyMS  int64                   `json:"retrievalLatencyMs"`
	GenerationLatencyMS int64                   `json:"generationLatencyMs"`
	InputTokens         int                     `json:"inputTokens"`
	OutputTokens        int                     `json:"outputTokens"`
	PointCost           int64                   `json:"pointCost"`
	BindingSnapshot     []AgentKnowledgeBinding `json:"bindingSnapshot,omitempty"`
	RetrievalSnapshot   map[string]any          `json:"retrievalSnapshot,omitempty"`
	ErrorCode           string                  `json:"errorCode,omitempty"`
	ErrorMessage        string                  `json:"errorMessage,omitempty"`
	CreatedAt           time.Time               `json:"createdAt"`
	UpdatedAt           time.Time               `json:"updatedAt"`
}

type RetrievalHit struct {
	ID                string         `json:"id"`
	TenantID          string         `json:"tenantId"`
	RAGRunID          string         `json:"ragRunId,omitempty"`
	KnowledgeBaseID   string         `json:"knowledgeBaseId"`
	ChunkID           string         `json:"chunkId"`
	DocumentID        string         `json:"documentId"`
	DocumentVersionID string         `json:"documentVersionId"`
	DocumentName      string         `json:"documentName"`
	Title             string         `json:"title,omitempty"`
	Content           string         `json:"content"`
	InitialRank       int            `json:"initialRank"`
	FinalRank         int            `json:"finalRank"`
	VectorScore       float64        `json:"vectorScore,omitempty"`
	KeywordScore      float64        `json:"keywordScore,omitempty"`
	RerankScore       float64        `json:"rerankScore,omitempty"`
	FinalScore        float64        `json:"finalScore"`
	SourceLocator     map[string]any `json:"sourceLocator,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
}

type Citation struct {
	ID                 string         `json:"id"`
	TenantID           string         `json:"tenantId"`
	RAGRunID           string         `json:"ragRunId"`
	AssistantMessageID string         `json:"assistantMessageId"`
	DocumentID         string         `json:"documentId"`
	DocumentVersionID  string         `json:"documentVersionId"`
	ChunkID            string         `json:"chunkId"`
	Order              int            `json:"order"`
	DocumentName       string         `json:"documentName"`
	Quote              string         `json:"quote"`
	Locator            map[string]any `json:"locator,omitempty"`
	SimilarityScore    float64        `json:"similarityScore,omitempty"`
}

type RunEvent struct {
	ID         string         `json:"id"`
	TenantID   string         `json:"tenantId"`
	RAGRunID   string         `json:"ragRunId"`
	SequenceNo int64          `json:"sequence"`
	EventType  string         `json:"event"`
	Payload    map[string]any `json:"data"`
	CreatedAt  time.Time      `json:"createdAt"`
}
