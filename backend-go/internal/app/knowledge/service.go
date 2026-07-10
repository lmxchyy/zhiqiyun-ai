package knowledge

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

type Service struct {
	tenants TenantRepository
	repo    KnowledgeRepository
	now     func() time.Time
	newID   func(string) string
}

func NewService(tenants TenantRepository, repo KnowledgeRepository) *Service {
	return &Service{
		tenants: tenants,
		repo:    repo,
		now:     func() time.Time { return time.Now().UTC() },
		newID:   newID,
	}
}

func (s *Service) ResolveAccessContext(ctx context.Context, userID string, tenantID string, organizationID string) (AccessContext, error) {
	if s == nil || s.tenants == nil {
		return AccessContext{}, fmt.Errorf("resolve knowledge access context: %w", ErrValidation)
	}
	if strings.TrimSpace(userID) == "" {
		return AccessContext{}, fmt.Errorf("user id is required: %w", ErrValidation)
	}
	return s.tenants.ResolveAccessContext(ctx, strings.TrimSpace(userID), strings.TrimSpace(tenantID), strings.TrimSpace(organizationID))
}

type CreateKnowledgeBaseInput struct {
	Name               string
	Description        string
	OrganizationID     string
	CategoryID         string
	KnowledgeType      KnowledgeType
	Visibility         string
	LogoObjectKey      string
	IngestionProfileID string
	RetrievalProfileID string
	Metadata           map[string]any
	TagIDs             []string
}

func (s *Service) CreateKnowledgeBase(ctx context.Context, access AccessContext, input CreateKnowledgeBaseInput) (KnowledgeBase, error) {
	if s == nil || s.repo == nil {
		return KnowledgeBase{}, fmt.Errorf("create knowledge base: %w", ErrValidation)
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len([]rune(input.Name)) > 150 {
		return KnowledgeBase{}, fmt.Errorf("knowledge base name must be 1-150 characters: %w", ErrValidation)
	}
	if access.TenantID == "" || access.UserID == "" {
		return KnowledgeBase{}, fmt.Errorf("tenant and user context are required: %w", ErrValidation)
	}
	if access.HasRole("GUEST") {
		return KnowledgeBase{}, ErrForbidden
	}
	if input.KnowledgeType == "" {
		input.KnowledgeType = KnowledgePersonal
	}
	if input.IngestionProfileID == "" {
		input.IngestionProfileID = "ingestion_default"
	}
	if input.RetrievalProfileID == "" {
		input.RetrievalProfileID = "retrieval_default"
	}
	if !validKnowledgeType(input.KnowledgeType) {
		return KnowledgeBase{}, fmt.Errorf("unsupported knowledge type %q: %w", input.KnowledgeType, ErrValidation)
	}
	if input.KnowledgeType == KnowledgeDepartment && strings.TrimSpace(input.OrganizationID) == "" {
		return KnowledgeBase{}, fmt.Errorf("department knowledge base requires organization id: %w", ErrValidation)
	}
	if (input.KnowledgeType == KnowledgeEnterprise || input.KnowledgeType == KnowledgeDepartment) && !access.HasRole("PLATFORM_ADMIN", "ENTERPRISE_ADMIN") {
		return KnowledgeBase{}, ErrForbidden
	}
	if input.OrganizationID != "" && access.OrganizationID != "" && input.OrganizationID != access.OrganizationID && !access.HasRole("PLATFORM_ADMIN", "ENTERPRISE_ADMIN") {
		return KnowledgeBase{}, ErrForbidden
	}
	visibility := strings.ToUpper(strings.TrimSpace(input.Visibility))
	if visibility == "" {
		visibility = "PRIVATE"
	}
	if visibility != "PRIVATE" && visibility != "TENANT" && visibility != "ORGANIZATION" && visibility != "SHARED" {
		return KnowledgeBase{}, fmt.Errorf("unsupported visibility %q: %w", visibility, ErrValidation)
	}
	now := s.now()
	item := KnowledgeBase{
		ID:                 s.newID("kb"),
		TenantID:           access.TenantID,
		OrganizationID:     strings.TrimSpace(input.OrganizationID),
		OwnerUserID:        access.UserID,
		CategoryID:         strings.TrimSpace(input.CategoryID),
		KnowledgeType:      input.KnowledgeType,
		Name:               input.Name,
		Description:        strings.TrimSpace(input.Description),
		LogoObjectKey:      strings.TrimSpace(input.LogoObjectKey),
		Visibility:         visibility,
		Status:             "ACTIVE",
		IngestionProfileID: strings.TrimSpace(input.IngestionProfileID),
		RetrievalProfileID: strings.TrimSpace(input.RetrievalProfileID),
		Metadata:           cloneMap(input.Metadata),
		Version:            1,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	created, err := s.repo.CreateKnowledgeBase(ctx, access, item)
	if err != nil {
		return KnowledgeBase{}, err
	}
	if input.TagIDs != nil {
		created.Tags, err = s.repo.ReplaceKnowledgeBaseTags(ctx, access, created.ID, uniqueStrings(input.TagIDs))
	}
	return created, err
}

func (s *Service) ListKnowledgeBases(ctx context.Context, access AccessContext, options ListOptions) ([]KnowledgeBase, string, error) {
	options.Limit = normalizedLimit(options.Limit, 20, 100)
	return s.repo.ListKnowledgeBases(ctx, access, options)
}

func (s *Service) GetKnowledgeBase(ctx context.Context, access AccessContext, id string) (KnowledgeBase, error) {
	if strings.TrimSpace(id) == "" {
		return KnowledgeBase{}, fmt.Errorf("knowledge base id is required: %w", ErrValidation)
	}
	item, err := s.repo.GetKnowledgeBase(ctx, access, strings.TrimSpace(id))
	if err != nil {
		return KnowledgeBase{}, err
	}
	allowed, err := s.AuthorizeKnowledgeBase(ctx, access, item, "VIEW")
	if err != nil {
		return KnowledgeBase{}, err
	}
	if !allowed {
		return KnowledgeBase{}, ErrForbidden
	}
	return item, nil
}

type UpdateKnowledgeBaseInput struct {
	Name               *string
	Description        *string
	OrganizationID     *string
	CategoryID         *string
	Visibility         *string
	Status             *string
	LogoObjectKey      *string
	IngestionProfileID *string
	RetrievalProfileID *string
	Metadata           map[string]any
	ExpectedVersion    int64
	TagIDs             []string
}

func (s *Service) UpdateKnowledgeBase(ctx context.Context, access AccessContext, id string, input UpdateKnowledgeBaseInput) (KnowledgeBase, error) {
	item, err := s.GetKnowledgeBase(ctx, access, id)
	if err != nil {
		return KnowledgeBase{}, err
	}
	allowed, err := s.AuthorizeKnowledgeBase(ctx, access, item, "EDIT")
	if err != nil {
		return KnowledgeBase{}, err
	}
	if !allowed {
		return KnowledgeBase{}, ErrForbidden
	}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" || len([]rune(name)) > 150 {
			return KnowledgeBase{}, fmt.Errorf("knowledge base name must be 1-150 characters: %w", ErrValidation)
		}
		item.Name = name
	}
	if input.Description != nil {
		item.Description = strings.TrimSpace(*input.Description)
	}
	if input.OrganizationID != nil {
		nextOrganizationID := strings.TrimSpace(*input.OrganizationID)
		if nextOrganizationID != "" && access.OrganizationID != "" && nextOrganizationID != access.OrganizationID && !access.HasRole("PLATFORM_ADMIN", "ENTERPRISE_ADMIN") {
			return KnowledgeBase{}, ErrForbidden
		}
		item.OrganizationID = nextOrganizationID
	}
	if input.CategoryID != nil {
		item.CategoryID = strings.TrimSpace(*input.CategoryID)
	}
	if input.Visibility != nil {
		visibility := strings.ToUpper(strings.TrimSpace(*input.Visibility))
		if visibility != "PRIVATE" && visibility != "TENANT" && visibility != "ORGANIZATION" && visibility != "SHARED" {
			return KnowledgeBase{}, fmt.Errorf("unsupported visibility %q: %w", visibility, ErrValidation)
		}
		if visibility == "ORGANIZATION" && item.OrganizationID == "" {
			return KnowledgeBase{}, fmt.Errorf("organization visibility requires organization id: %w", ErrValidation)
		}
		item.Visibility = visibility
	}
	if input.Status != nil {
		status := strings.ToUpper(strings.TrimSpace(*input.Status))
		if status != "ACTIVE" && status != "DISABLED" && status != "ARCHIVED" {
			return KnowledgeBase{}, fmt.Errorf("unsupported knowledge base status %q: %w", status, ErrValidation)
		}
		item.Status = status
	}
	if input.LogoObjectKey != nil {
		item.LogoObjectKey = strings.TrimSpace(*input.LogoObjectKey)
	}
	if input.IngestionProfileID != nil {
		nextProfileID := strings.TrimSpace(*input.IngestionProfileID)
		if nextProfileID != "" && nextProfileID != item.IngestionProfileID && item.DocumentCount > 0 {
			return KnowledgeBase{}, fmt.Errorf("changing the ingestion profile requires an empty or reindexed knowledge base: %w", ErrConflict)
		}
		item.IngestionProfileID = nextProfileID
	}
	if input.RetrievalProfileID != nil {
		item.RetrievalProfileID = strings.TrimSpace(*input.RetrievalProfileID)
	}
	if input.Metadata != nil {
		item.Metadata = cloneMap(input.Metadata)
	}
	item.UpdatedAt = s.now()
	updated, err := s.repo.UpdateKnowledgeBase(ctx, access, item, input.ExpectedVersion)
	if err != nil {
		return KnowledgeBase{}, err
	}
	if input.TagIDs != nil {
		updated.Tags, err = s.repo.ReplaceKnowledgeBaseTags(ctx, access, updated.ID, uniqueStrings(input.TagIDs))
	}
	return updated, err
}

func (s *Service) ListTags(ctx context.Context, access AccessContext) ([]Tag, error) {
	return s.repo.ListKnowledgeTags(ctx, access)
}

func (s *Service) SaveTag(ctx context.Context, access AccessContext, name string, color string) (Tag, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 60 || access.HasRole("GUEST") {
		if access.HasRole("GUEST") {
			return Tag{}, ErrForbidden
		}
		return Tag{}, fmt.Errorf("tag name must be 1-60 characters: %w", ErrValidation)
	}
	return s.repo.SaveKnowledgeTag(ctx, access, Tag{ID: s.newID("tag"), TenantID: access.TenantID, Name: name, Color: strings.TrimSpace(color)})
}

func (s *Service) ListCategories(ctx context.Context, access AccessContext) ([]Category, error) {
	return s.repo.ListKnowledgeCategories(ctx, access)
}

func (s *Service) SaveCategory(ctx context.Context, access AccessContext, name string, parentID string, sortOrder int) (Category, error) {
	name = strings.TrimSpace(name)
	if access.HasRole("GUEST") {
		return Category{}, ErrForbidden
	}
	if name == "" || len([]rune(name)) > 100 {
		return Category{}, fmt.Errorf("category name must be 1-100 characters: %w", ErrValidation)
	}
	now := s.now()
	return s.repo.SaveKnowledgeCategory(ctx, access, Category{ID: s.newID("category"), TenantID: access.TenantID, ParentID: strings.TrimSpace(parentID), Name: name, SortOrder: sortOrder, CreatedAt: now, UpdatedAt: now})
}

func (s *Service) DeleteKnowledgeBase(ctx context.Context, access AccessContext, id string) error {
	item, err := s.GetKnowledgeBase(ctx, access, id)
	if err != nil {
		return err
	}
	allowed, err := s.AuthorizeKnowledgeBase(ctx, access, item, "DELETE")
	if err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}
	return s.repo.SoftDeleteKnowledgeBase(ctx, access, item.ID)
}

func (s *Service) AuthorizeKnowledgeBase(ctx context.Context, access AccessContext, item KnowledgeBase, permission string) (bool, error) {
	permission = strings.ToUpper(strings.TrimSpace(permission))
	if access.TenantID == "" || item.TenantID != access.TenantID {
		return false, nil
	}
	if canManageKnowledgeBase(access, item) {
		return true, nil
	}
	rules, err := s.repo.ListKnowledgeBaseACL(ctx, access, item.ID)
	if err != nil {
		return false, err
	}
	allowed := false
	for _, rule := range rules {
		if (rule.ExpiresAt != nil && rule.ExpiresAt.Before(s.now())) || !aclSubjectMatches(access, rule) {
			continue
		}
		if !aclPermissionMatches(permission, rule.Permission) {
			continue
		}
		if strings.EqualFold(rule.Effect, "DENY") {
			return false, nil
		}
		if strings.EqualFold(rule.Effect, "ALLOW") {
			allowed = true
		}
	}
	if permission == "VIEW" && canViewKnowledgeBase(access, item) {
		return true, nil
	}
	return allowed, nil
}

func aclPermissionMatches(requested string, granted string) bool {
	requested = strings.ToUpper(strings.TrimSpace(requested))
	granted = strings.ToUpper(strings.TrimSpace(granted))
	return granted == requested || granted == "MANAGE" || (requested == "VIEW" && granted == "READ")
}

func aclSubjectMatches(access AccessContext, rule ACLRule) bool {
	switch strings.ToUpper(strings.TrimSpace(rule.SubjectType)) {
	case "USER":
		return rule.SubjectID == access.UserID
	case "ORGANIZATION", "DEPARTMENT":
		return rule.SubjectID != "" && rule.SubjectID == access.OrganizationID
	case "TENANT":
		return rule.SubjectID == "" || rule.SubjectID == access.TenantID
	case "ROLE":
		return access.HasRole(rule.SubjectID)
	case "EVERYONE", "GUEST":
		return true
	default:
		return false
	}
}

func canViewKnowledgeBase(access AccessContext, item KnowledgeBase) bool {
	if access.TenantID != item.TenantID {
		return false
	}
	if canManageKnowledgeBase(access, item) {
		return true
	}
	switch item.Visibility {
	case "TENANT", "SHARED":
		return true
	case "ORGANIZATION":
		return access.OrganizationID != "" && access.OrganizationID == item.OrganizationID
	default:
		return false
	}
}

func canManageKnowledgeBase(access AccessContext, item KnowledgeBase) bool {
	return access.TenantID == item.TenantID && (access.UserID == item.OwnerUserID || access.HasRole("PLATFORM_ADMIN", "ENTERPRISE_ADMIN"))
}

func validKnowledgeType(value KnowledgeType) bool {
	switch value {
	case KnowledgeEnterprise, KnowledgeDepartment, KnowledgePersonal, KnowledgeAgent:
		return true
	default:
		return false
	}
}

func normalizedLimit(value int, fallback int, maximum int) int {
	if value <= 0 {
		return fallback
	}
	if value > maximum {
		return maximum
	}
	return value
}

func cloneMap(source map[string]any) map[string]any {
	if source == nil {
		return map[string]any{}
	}
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func newID(prefix string) string {
	var raw [10]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return prefix + "_" + hex.EncodeToString(raw[:])
	}
	return fmt.Sprintf("%s_%d", prefix, time.Now().UTC().UnixNano())
}
