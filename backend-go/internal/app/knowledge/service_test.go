package knowledge_test

import (
	"context"
	"errors"
	"testing"
	"time"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
	knowledgerepo "xianzhi-ai/backend-go/internal/repository/knowledge"
)

func TestKnowledgeBaseTenantIsolation(t *testing.T) {
	t.Parallel()
	repo := knowledgerepo.NewMemory()
	service := knowledgeapp.NewService(repo, repo)
	owner, err := service.ResolveAccessContext(context.Background(), "user_owner", "tenant_a", "")
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.CreateKnowledgeBase(context.Background(), owner, knowledgeapp.CreateKnowledgeBaseInput{Name: "Owner KB", KnowledgeType: knowledgeapp.KnowledgePersonal})
	if err != nil {
		t.Fatal(err)
	}
	other, err := service.ResolveAccessContext(context.Background(), "user_other", "tenant_b", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.GetKnowledgeBase(context.Background(), other, created.ID)
	if !errors.Is(err, knowledgeapp.ErrNotFound) {
		t.Fatalf("expected tenant-isolated not found, got %v", err)
	}
}

func TestKnowledgeBaseACLAllowsPrivateViewAndUpload(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := knowledgerepo.NewMemory()
	service := knowledgeapp.NewService(repo, repo)
	owner, _ := service.ResolveAccessContext(ctx, "user_owner", "tenant_a", "org_a")
	viewer, _ := service.ResolveAccessContext(ctx, "user_viewer", "tenant_a", "org_a")
	created, err := service.CreateKnowledgeBase(ctx, owner, knowledgeapp.CreateKnowledgeBaseInput{Name: "Private KB", Visibility: "PRIVATE"})
	if err != nil {
		t.Fatal(err)
	}
	rules := []knowledgeapp.ACLRule{
		{ID: "acl_view", TenantID: owner.TenantID, KnowledgeBaseID: created.ID, SubjectType: "USER", SubjectID: viewer.UserID, Permission: "VIEW", Effect: "ALLOW"},
		{ID: "acl_upload", TenantID: owner.TenantID, KnowledgeBaseID: created.ID, SubjectType: "USER", SubjectID: viewer.UserID, Permission: "UPLOAD", Effect: "ALLOW"},
	}
	if err := repo.ReplaceKnowledgeBaseACL(ctx, owner, created.ID, rules); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetKnowledgeBase(ctx, viewer, created.ID); err != nil {
		t.Fatalf("expected ACL view access, got %v", err)
	}
	allowed, err := service.AuthorizeKnowledgeBase(ctx, viewer, created, "UPLOAD")
	if err != nil || !allowed {
		t.Fatalf("expected ACL upload access, allowed=%v err=%v", allowed, err)
	}
	items, _, err := service.ListKnowledgeBases(ctx, viewer, knowledgeapp.ListOptions{})
	if err != nil || len(items) != 1 || items[0].ID != created.ID {
		t.Fatalf("expected private ACL knowledge base in list, items=%v err=%v", items, err)
	}
}

func TestKnowledgeBaseACLDenyOverridesSharedVisibility(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := knowledgerepo.NewMemory()
	service := knowledgeapp.NewService(repo, repo)
	owner, _ := service.ResolveAccessContext(ctx, "user_owner", "tenant_a", "")
	viewer, _ := service.ResolveAccessContext(ctx, "user_viewer", "tenant_a", "")
	created, err := service.CreateKnowledgeBase(ctx, owner, knowledgeapp.CreateKnowledgeBaseInput{Name: "Tenant KB", Visibility: "TENANT"})
	if err != nil {
		t.Fatal(err)
	}
	expires := time.Now().UTC().Add(time.Hour)
	if err := repo.ReplaceKnowledgeBaseACL(ctx, owner, created.ID, []knowledgeapp.ACLRule{{
		ID: "acl_deny", TenantID: owner.TenantID, KnowledgeBaseID: created.ID, SubjectType: "USER", SubjectID: viewer.UserID,
		Permission: "VIEW", Effect: "DENY", ExpiresAt: &expires,
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetKnowledgeBase(ctx, viewer, created.ID); !errors.Is(err, knowledgeapp.ErrForbidden) {
		t.Fatalf("expected explicit deny to override tenant visibility, got %v", err)
	}
	items, _, err := service.ListKnowledgeBases(ctx, viewer, knowledgeapp.ListOptions{})
	if err != nil || len(items) != 0 {
		t.Fatalf("expected denied knowledge base hidden from list, items=%v err=%v", items, err)
	}
}

func TestEnterpriseKnowledgeBaseRequiresAdmin(t *testing.T) {
	t.Parallel()
	repo := knowledgerepo.NewMemory()
	service := knowledgeapp.NewService(repo, repo)
	member, err := service.ResolveAccessContext(context.Background(), "user_member", "tenant_a", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.CreateKnowledgeBase(context.Background(), member, knowledgeapp.CreateKnowledgeBaseInput{Name: "Enterprise KB", KnowledgeType: knowledgeapp.KnowledgeEnterprise})
	if !errors.Is(err, knowledgeapp.ErrForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
}

func TestKnowledgeBaseOptimisticConcurrency(t *testing.T) {
	t.Parallel()
	repo := knowledgerepo.NewMemory()
	service := knowledgeapp.NewService(repo, repo)
	owner, err := service.ResolveAccessContext(context.Background(), "user_owner", "tenant_a", "")
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.CreateKnowledgeBase(context.Background(), owner, knowledgeapp.CreateKnowledgeBaseInput{Name: "Versioned KB"})
	if err != nil {
		t.Fatal(err)
	}
	name := "Updated KB"
	updated, err := service.UpdateKnowledgeBase(context.Background(), owner, created.ID, knowledgeapp.UpdateKnowledgeBaseInput{Name: &name, ExpectedVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != 2 {
		t.Fatalf("expected version 2, got %d", updated.Version)
	}
	_, err = service.UpdateKnowledgeBase(context.Background(), owner, created.ID, knowledgeapp.UpdateKnowledgeBaseInput{Name: &name, ExpectedVersion: 1})
	if !errors.Is(err, knowledgeapp.ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestKnowledgeBaseCategoryAndTags(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := knowledgerepo.NewMemory()
	service := knowledgeapp.NewService(repo, repo)
	access, _ := service.ResolveAccessContext(ctx, "user_owner", "tenant_a", "")
	category, err := service.SaveCategory(ctx, access, "产品文档", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	tag, err := service.SaveTag(ctx, access, "核心产品", "#655ce7")
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.CreateKnowledgeBase(ctx, access, knowledgeapp.CreateKnowledgeBaseInput{Name: "分类知识库", CategoryID: category.ID, TagIDs: []string{tag.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if created.CategoryID != category.ID || len(created.Tags) != 1 || created.Tags[0].ID != tag.ID {
		t.Fatalf("category or tags were not persisted: %#v", created)
	}
	loaded, err := service.GetKnowledgeBase(ctx, access, created.ID)
	if err != nil || len(loaded.Tags) != 1 {
		t.Fatalf("category tags not loaded: %#v err=%v", loaded, err)
	}
}
