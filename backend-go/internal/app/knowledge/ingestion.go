package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

type ParserSelector interface {
	Parse(context.Context, SourceDocument) ([]DocumentUnit, string, error)
}

type ChunkerSelector interface {
	Chunk(context.Context, string, []DocumentUnit, ChunkOptions) ([]Chunk, error)
}

type IngestionService struct {
	core       *Service
	repo       KnowledgeRepository
	parsers    ParserSelector
	chunkers   ChunkerSelector
	embedder   Embedder
	vectors    VectorStore
	normalizer DocumentNormalizer
	runtime    IngestionRuntimeResolver
	now        func() time.Time
	newID      func(string) string
}

func (s *IngestionService) SetRuntimeResolver(resolver IngestionRuntimeResolver) {
	if s != nil {
		s.runtime = resolver
	}
}

func NewIngestionService(core *Service, repo KnowledgeRepository, parsers ParserSelector, chunkers ChunkerSelector, embedder Embedder, vectors VectorStore) *IngestionService {
	return NewIngestionServiceWithNormalizer(core, repo, parsers, chunkers, embedder, vectors, nil)
}

func NewIngestionServiceWithNormalizer(core *Service, repo KnowledgeRepository, parsers ParserSelector, chunkers ChunkerSelector, embedder Embedder, vectors VectorStore, normalizer DocumentNormalizer) *IngestionService {
	return &IngestionService{
		core:       core,
		repo:       repo,
		parsers:    parsers,
		chunkers:   chunkers,
		embedder:   embedder,
		vectors:    vectors,
		normalizer: normalizer,
		now:        func() time.Time { return time.Now().UTC() },
		newID:      newID,
	}
}

type IngestDocumentInput struct {
	Name           string
	MIMEType       string
	Content        []byte
	ObjectKey      string
	DocumentType   string
	ChunkerKey     string
	ChunkOptions   ChunkOptions
	IdempotencyKey string
	Metadata       map[string]any
}

type IngestionResult struct {
	Document Document        `json:"document"`
	Version  DocumentVersion `json:"version"`
	Chunks   []Chunk         `json:"chunks"`
	Job      IngestionJob    `json:"job"`
}

func (s *IngestionService) Ingest(ctx context.Context, access AccessContext, knowledgeBaseID string, input IngestDocumentInput) (IngestionResult, error) {
	if s == nil || s.core == nil || s.repo == nil || s.parsers == nil || s.chunkers == nil || s.embedder == nil || s.vectors == nil {
		return IngestionResult{}, fmt.Errorf("knowledge ingestion service is not configured: %w", ErrValidation)
	}
	base, err := s.core.GetKnowledgeBase(ctx, access, knowledgeBaseID)
	if err != nil {
		return IngestionResult{}, err
	}
	allowed, authErr := s.core.AuthorizeKnowledgeBase(ctx, access, base, "UPLOAD")
	if authErr != nil {
		return IngestionResult{}, authErr
	}
	if !allowed {
		return IngestionResult{}, ErrForbidden
	}
	embedder, vectors := s.embedder, s.vectors
	runtimeProfile := IngestionRuntimeProfile{
		ID: base.IngestionProfileID, ChunkerKey: "fixed", ChunkOptions: ChunkOptions{ChunkSize: 800, Overlap: 120, MinTokens: 40, MaxTokens: 1200},
		Embedding: EmbeddingRuntimeProfile{ID: "embedding_deterministic_default"}, VectorStore: VectorStoreRuntimeProfile{ID: "vector_pgvector_default"},
	}
	if s.runtime != nil {
		selection, resolveErr := s.runtime.ResolveIngestionRuntime(ctx, access, base)
		if resolveErr != nil {
			return IngestionResult{}, fmt.Errorf("resolve ingestion runtime: %w", resolveErr)
		}
		if selection.Embedder == nil || selection.VectorStore == nil {
			return IngestionResult{}, fmt.Errorf("ingestion runtime returned incomplete providers: %w", ErrValidation)
		}
		runtimeProfile, embedder, vectors = selection.Profile, selection.Embedder, selection.VectorStore
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Content) == 0 {
		return IngestionResult{}, fmt.Errorf("document name and content are required: %w", ErrValidation)
	}
	if len(input.Content) > 20<<20 {
		return IngestionResult{}, fmt.Errorf("inline document exceeds 20 MiB: %w", ErrValidation)
	}
	if input.MIMEType == "" {
		input.MIMEType = "text/plain"
	}
	units, parserCode, err := s.parsers.Parse(ctx, SourceDocument{Name: input.Name, MIMEType: input.MIMEType, ObjectKey: input.ObjectKey, Content: input.Content, Metadata: input.Metadata})
	if err != nil {
		return IngestionResult{}, err
	}
	if len(units) == 0 {
		return IngestionResult{}, fmt.Errorf("parser returned no document units: %w", ErrValidation)
	}
	normalizerMetadata := map[string]any{}
	if s.normalizer != nil {
		units, normalizerMetadata, err = s.normalizer.Normalize(ctx, units)
		if err != nil {
			return IngestionResult{}, fmt.Errorf("normalize document: %w", err)
		}
		if len(units) == 0 {
			return IngestionResult{}, fmt.Errorf("normalizer removed all document units: %w", ErrValidation)
		}
	}
	if input.ChunkerKey == "" {
		input.ChunkerKey = runtimeProfile.ChunkerKey
		if input.ChunkerKey == "" {
			input.ChunkerKey = "fixed"
		}
	}
	if input.ChunkOptions.ChunkSize <= 0 {
		input.ChunkOptions = runtimeProfile.ChunkOptions
		if input.ChunkOptions.ChunkSize <= 0 {
			input.ChunkOptions = ChunkOptions{ChunkSize: 800, Overlap: 120, MinTokens: 40, MaxTokens: 1200}
		}
	}
	chunks, err := s.chunkers.Chunk(ctx, input.ChunkerKey, units, input.ChunkOptions)
	if err != nil {
		return IngestionResult{}, err
	}
	if len(chunks) == 0 {
		return IngestionResult{}, fmt.Errorf("document produced no chunks: %w", ErrValidation)
	}

	now := s.now()
	documentID, versionID := s.newID("doc"), s.newID("docver")
	contentHash := hashBytes(input.Content)
	documentType := strings.ToUpper(strings.TrimSpace(input.DocumentType))
	if documentType == "" {
		documentType = documentTypeFromName(input.Name)
	}
	document := Document{
		ID:              documentID,
		TenantID:        access.TenantID,
		KnowledgeBaseID: base.ID,
		OwnerUserID:     access.UserID,
		LatestVersionID: versionID,
		Name:            input.Name,
		DocumentType:    documentType,
		MIMEType:        input.MIMEType,
		Status:          "INDEXING",
		Metadata:        cloneMap(input.Metadata),
		Version:         1,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	version := DocumentVersion{
		ID:                versionID,
		TenantID:          access.TenantID,
		DocumentID:        documentID,
		VersionNo:         1,
		OriginalObjectKey: input.ObjectKey,
		MIMEType:          input.MIMEType,
		FileSize:          int64(len(input.Content)),
		ContentHash:       contentHash,
		ParseStatus:       "PARSED",
		ParserMetadata:    map[string]any{"parser": parserCode, "unitCount": len(units), "normalizer": normalizerMetadata},
		CreatedBy:         access.UserID,
		CreatedAt:         now,
	}
	for index := range units {
		units[index].ID = s.newID("unit")
		units[index].TenantID = access.TenantID
		units[index].DocumentVersionID = versionID
	}
	for index := range chunks {
		chunks[index].ID = s.newID("chunk")
		chunks[index].TenantID = access.TenantID
		chunks[index].KnowledgeBaseID = base.ID
		chunks[index].DocumentID = documentID
		chunks[index].DocumentVersionID = versionID
		chunks[index].SequenceNo = index + 1
		chunks[index].ContentHash = hashBytes([]byte(chunks[index].Content))
		chunks[index].ChunkKey = fmt.Sprintf("%04d_%s", index+1, chunks[index].ContentHash[:12])
		chunks[index].CreatedAt = now
		chunks[index].UpdatedAt = now
	}
	jobID := s.newID("kbjob")
	if strings.TrimSpace(input.IdempotencyKey) == "" {
		input.IdempotencyKey = "ingest:" + access.TenantID + ":" + base.ID + ":" + contentHash
	}
	job := IngestionJob{
		ID:                 jobID,
		TenantID:           access.TenantID,
		DocumentVersionID:  versionID,
		IngestionProfileID: base.IngestionProfileID,
		IdempotencyKey:     input.IdempotencyKey,
		Stage:              "INDEXING",
		Status:             "RUNNING",
		MaxAttempts:        3,
		Progress:           75,
		ConfigSnapshot: map[string]any{
			"parser": parserCode, "chunker": input.ChunkerKey, "embedding": embedder.Code(), "vectorStore": vectors.Code(),
			"embeddingProfileId": runtimeProfile.Embedding.ID, "vectorStoreProfileId": runtimeProfile.VectorStore.ID,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	createdDocument, createdJob, err := s.repo.CreateDocumentBundle(ctx, access, document, version, units, chunks, job)
	if err != nil {
		return IngestionResult{}, err
	}

	texts := make([]string, len(chunks))
	for index := range chunks {
		texts[index] = chunks[index].Content
	}
	embeddings, err := embedder.Embed(ctx, texts)
	if err != nil {
		s.fail(ctx, access, createdDocument.ID, &createdJob, "EMBEDDING_FAILED", err)
		return IngestionResult{}, err
	}
	if len(embeddings) != len(chunks) {
		err := fmt.Errorf("embedding result count mismatch")
		s.fail(ctx, access, createdDocument.ID, &createdJob, "EMBEDDING_MISMATCH", err)
		return IngestionResult{}, err
	}
	indexID := "kbindex_" + base.ID + "_1"
	records := make([]VectorRecord, len(chunks))
	for index := range chunks {
		records[index] = VectorRecord{
			TenantID:             access.TenantID,
			KnowledgeBaseID:      base.ID,
			DocumentVersionID:    versionID,
			VectorIndexID:        indexID,
			EmbeddingProfileID:   runtimeProfile.Embedding.ID,
			VectorStoreProfileID: runtimeProfile.VectorStore.ID,
			ChunkID:              chunks[index].ID,
			Embedding:            embeddings[index],
			SearchText:           chunks[index].Content,
			EmbeddingHash:        hashEmbedding(embeddings[index], embedder.Code()),
			FilterMetadata: map[string]any{
				"documentId": documentID, "documentVersionId": versionID, "documentName": input.Name,
				"title": chunks[index].Title, "sourceLocator": chunks[index].SourceLocator, "metadata": input.Metadata,
			},
		}
	}
	if err := vectors.Upsert(ctx, indexID, records); err != nil {
		s.fail(ctx, access, createdDocument.ID, &createdJob, "INDEXING_FAILED", err)
		return IngestionResult{}, err
	}
	createdDocument.Status = "READY"
	createdDocument.UpdatedAt = s.now()
	createdJob.Stage = "READY"
	createdJob.Status = "READY"
	createdJob.Progress = 100
	createdJob.UpdatedAt = s.now()
	_ = s.repo.UpdateDocumentStatus(ctx, access, createdDocument.ID, "READY")
	_ = s.repo.UpdateIngestionJob(ctx, access, createdJob)
	return IngestionResult{Document: createdDocument, Version: version, Chunks: chunks, Job: createdJob}, nil
}

func (s *IngestionService) fail(ctx context.Context, access AccessContext, documentID string, job *IngestionJob, code string, cause error) {
	job.Stage = "FAILED"
	job.Status = "FAILED"
	job.ErrorCode = code
	job.ErrorMessage = cause.Error()
	job.UpdatedAt = s.now()
	_ = s.repo.UpdateDocumentStatus(ctx, access, documentID, "FAILED")
	_ = s.repo.UpdateIngestionJob(ctx, access, *job)
}

func (s *IngestionService) DeleteDocument(ctx context.Context, access AccessContext, documentID string) error {
	if s == nil || s.core == nil || s.repo == nil || s.vectors == nil {
		return fmt.Errorf("knowledge ingestion service is not configured: %w", ErrValidation)
	}
	documentID = strings.TrimSpace(documentID)
	if documentID == "" {
		return fmt.Errorf("document id is required: %w", ErrValidation)
	}
	document, err := s.repo.GetDocument(ctx, access, documentID)
	if err != nil {
		return err
	}
	base, err := s.core.GetKnowledgeBase(ctx, access, document.KnowledgeBaseID)
	if err != nil {
		return err
	}
	allowed, err := s.core.AuthorizeKnowledgeBase(ctx, access, base, "DELETE")
	if err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}
	vectors := s.vectors
	if s.runtime != nil {
		selection, resolveErr := s.runtime.ResolveIngestionRuntime(ctx, access, base)
		if resolveErr != nil {
			return fmt.Errorf("resolve ingestion runtime: %w", resolveErr)
		}
		if selection.VectorStore == nil {
			return fmt.Errorf("ingestion runtime returned no vector store: %w", ErrValidation)
		}
		vectors = selection.VectorStore
	}
	indexID := "kbindex_" + base.ID + "_1"
	if err := vectors.DeleteByDocumentVersion(ctx, access, indexID, document.LatestVersionID); err != nil {
		return fmt.Errorf("delete document vectors: %w", err)
	}
	return s.repo.SoftDeleteDocument(ctx, access, document.ID)
}

func hashBytes(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}

func hashEmbedding(vector []float32, provider string) string {
	builder := strings.Builder{}
	builder.WriteString(provider)
	for _, value := range vector {
		builder.WriteString(fmt.Sprintf("|%.7g", value))
	}
	return hashBytes([]byte(builder.String()))
}

func documentTypeFromName(name string) string {
	name = strings.ToLower(name)
	if index := strings.LastIndex(name, "."); index >= 0 && index < len(name)-1 {
		return strings.ToUpper(name[index+1:])
	}
	return "TEXT"
}
