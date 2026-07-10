package knowledge

import "context"

type ListOptions struct {
	Limit  int
	Cursor string
	Query  string
	Status string
}

type TenantRepository interface {
	ResolveAccessContext(ctx context.Context, userID string, requestedTenantID string, requestedOrganizationID string) (AccessContext, error)
}

type KnowledgeRepository interface {
	CreateKnowledgeBase(ctx context.Context, access AccessContext, item KnowledgeBase) (KnowledgeBase, error)
	ListKnowledgeBases(ctx context.Context, access AccessContext, options ListOptions) ([]KnowledgeBase, string, error)
	GetKnowledgeBase(ctx context.Context, access AccessContext, id string) (KnowledgeBase, error)
	UpdateKnowledgeBase(ctx context.Context, access AccessContext, item KnowledgeBase, expectedVersion int64) (KnowledgeBase, error)
	SoftDeleteKnowledgeBase(ctx context.Context, access AccessContext, id string) error
	ReplaceKnowledgeBaseACL(ctx context.Context, access AccessContext, knowledgeBaseID string, rules []ACLRule) error
	ListKnowledgeBaseACL(ctx context.Context, access AccessContext, knowledgeBaseID string) ([]ACLRule, error)
	ListKnowledgeTags(ctx context.Context, access AccessContext) ([]Tag, error)
	SaveKnowledgeTag(ctx context.Context, access AccessContext, item Tag) (Tag, error)
	ReplaceKnowledgeBaseTags(ctx context.Context, access AccessContext, knowledgeBaseID string, tagIDs []string) ([]Tag, error)
	ListKnowledgeCategories(ctx context.Context, access AccessContext) ([]Category, error)
	SaveKnowledgeCategory(ctx context.Context, access AccessContext, item Category) (Category, error)

	CreateDocumentBundle(ctx context.Context, access AccessContext, document Document, version DocumentVersion, units []DocumentUnit, chunks []Chunk, job IngestionJob) (Document, IngestionJob, error)
	ListDocuments(ctx context.Context, access AccessContext, knowledgeBaseID string, options ListOptions) ([]Document, string, error)
	GetDocument(ctx context.Context, access AccessContext, id string) (Document, error)
	SoftDeleteDocument(ctx context.Context, access AccessContext, id string) error
	ListChunks(ctx context.Context, access AccessContext, knowledgeBaseIDs []string, options ListOptions) ([]Chunk, error)
	ReplaceChunks(ctx context.Context, access AccessContext, documentVersionID string, chunks []Chunk) error
	UpdateDocumentStatus(ctx context.Context, access AccessContext, documentID string, status string) error
	UpdateIngestionJob(ctx context.Context, access AccessContext, job IngestionJob) error
}

type AgentRepository interface {
	CreateAgent(ctx context.Context, access AccessContext, agent Agent) (Agent, error)
	ListAgents(ctx context.Context, access AccessContext, options ListOptions) ([]Agent, string, error)
	GetAgent(ctx context.Context, access AccessContext, id string) (Agent, error)
	ReplaceAgentKnowledgeBindings(ctx context.Context, access AccessContext, agentID string, bindings []AgentKnowledgeBinding) error
	ListAgentKnowledgeBindings(ctx context.Context, access AccessContext, agentID string) ([]AgentKnowledgeBinding, error)

	CreateConversation(ctx context.Context, access AccessContext, conversation Conversation) (Conversation, error)
	ListConversations(ctx context.Context, access AccessContext, agentID string, options ListOptions) ([]Conversation, string, error)
	GetConversation(ctx context.Context, access AccessContext, id string) (Conversation, error)
	CreateMessage(ctx context.Context, access AccessContext, message Message) (Message, error)
	ListMessages(ctx context.Context, access AccessContext, conversationID string, options ListOptions) ([]Message, string, error)
}

type RAGRepository interface {
	CreateRun(ctx context.Context, access AccessContext, run RAGRun) (RAGRun, error)
	GetRun(ctx context.Context, access AccessContext, id string) (RAGRun, error)
	UpdateRun(ctx context.Context, access AccessContext, run RAGRun) error
	SaveRetrievalHits(ctx context.Context, access AccessContext, runID string, hits []RetrievalHit) error
	SaveCitations(ctx context.Context, access AccessContext, runID string, citations []Citation) error
	ListCitations(ctx context.Context, access AccessContext, runID string) ([]Citation, error)
	AppendRunEvent(ctx context.Context, access AccessContext, event RunEvent) error
	ListRunEvents(ctx context.Context, access AccessContext, runID string, afterSequence int64) ([]RunEvent, error)
}

type AdminOverview struct {
	TenantCount          int64 `json:"tenantCount"`
	KnowledgeBaseCount   int64 `json:"knowledgeBaseCount"`
	DocumentCount        int64 `json:"documentCount"`
	ChunkCount           int64 `json:"chunkCount"`
	ReadyDocumentCount   int64 `json:"readyDocumentCount"`
	FailedDocumentCount  int64 `json:"failedDocumentCount"`
	AgentCount           int64 `json:"agentCount"`
	RAGRunCount          int64 `json:"ragRunCount"`
	CompletedRAGRunCount int64 `json:"completedRagRunCount"`
	InputTokens          int64 `json:"inputTokens"`
	OutputTokens         int64 `json:"outputTokens"`
	PointCost            int64 `json:"pointCost"`
}

type AdminRepository interface {
	KnowledgeAdminOverview(context.Context, string) (AdminOverview, error)
	ListKnowledgeAdminRecords(context.Context, string, string, ListOptions) ([]map[string]any, error)
	SaveKnowledgeAdminProfile(context.Context, string, map[string]any) (map[string]any, error)
}

type RuntimeProfileRepository interface {
	ResolveIngestionRuntimeProfile(context.Context, AccessContext, string) (IngestionRuntimeProfile, error)
	ResolveRetrievalRuntimeProfile(context.Context, AccessContext, string) (RetrievalRuntimeProfile, error)
}

type SourceDocument struct {
	Name      string
	MIMEType  string
	ObjectKey string
	Content   []byte
	Metadata  map[string]any
}

type Parser interface {
	Code() string
	Supports(mimeType string, fileName string) bool
	Parse(context.Context, SourceDocument) ([]DocumentUnit, error)
}

type OCRProvider interface {
	Code() string
	Recognize(context.Context, SourceDocument) ([]DocumentUnit, error)
}

type DocumentNormalizer interface {
	Code() string
	Normalize(context.Context, []DocumentUnit) ([]DocumentUnit, map[string]any, error)
}

type ChunkOptions struct {
	ChunkSize int
	Overlap   int
	MinTokens int
	MaxTokens int
}

type Chunker interface {
	Code() string
	Chunk(context.Context, []DocumentUnit, ChunkOptions) ([]Chunk, error)
}

type Embedder interface {
	Code() string
	Dimension() int
	Embed(context.Context, []string) ([][]float32, error)
}

type IngestionRuntimeSelection struct {
	Profile     IngestionRuntimeProfile
	Retrieval   RetrievalRuntimeProfile
	Embedder    Embedder
	VectorStore VectorStore
	Reranker    Reranker
}

type IngestionRuntimeResolver interface {
	ResolveIngestionRuntime(context.Context, AccessContext, KnowledgeBase) (IngestionRuntimeSelection, error)
}

type VectorRecord struct {
	TenantID             string
	KnowledgeBaseID      string
	DocumentVersionID    string
	VectorIndexID        string
	EmbeddingProfileID   string
	VectorStoreProfileID string
	ChunkID              string
	Embedding            []float32
	SearchText           string
	EmbeddingHash        string
	FilterMetadata       map[string]any
}

type SearchRequest struct {
	Access              AccessContext
	KnowledgeBaseIDs    []string
	Query               string
	Mode                string
	TopK                int
	Threshold           float64
	VectorWeight        float64
	KeywordWeight       float64
	Filters             map[string]any
	RetrievalProfileIDs map[string]string
}

type VectorStore interface {
	Code() string
	Upsert(context.Context, string, []VectorRecord) error
	DeleteByDocumentVersion(context.Context, AccessContext, string, string) error
	Search(context.Context, SearchRequest, []float32) ([]RetrievalHit, error)
}

type Reranker interface {
	Code() string
	Rerank(context.Context, string, []RetrievalHit, int) ([]RetrievalHit, error)
}

type QueryRewriter interface {
	Rewrite(context.Context, []Message, string) (string, error)
}

type AnswerRequest struct {
	Agent    Agent
	Messages []Message
	Question string
	Hits     []RetrievalHit
}

type AnswerChunk struct {
	Delta    string
	Done     bool
	Usage    map[string]any
	Metadata map[string]any
	Err      error
}

type AnswerGenerator interface {
	Generate(context.Context, AnswerRequest) (<-chan AnswerChunk, error)
}

type ObjectStore interface {
	Put(context.Context, string, []byte, string) error
	Get(context.Context, string) ([]byte, error)
	SignedURL(context.Context, string) (string, error)
}

type JobPublisher interface {
	PublishIngestion(context.Context, IngestionJob) error
	PublishReindex(context.Context, string, string) error
}

type RAGBillingUsage struct {
	TenantID     string
	UserID       string
	AgentID      string
	RunID        string
	Model        string
	InputTokens  int
	OutputTokens int
	PointCost    int64
}

type RAGBillingRecorder interface {
	RecordRAGUsage(context.Context, RAGBillingUsage) error
}
