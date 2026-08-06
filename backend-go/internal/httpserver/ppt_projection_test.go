package httpserver

import (
	"encoding/json"
	"path/filepath"
	"testing"

	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
)

func TestPPTOwnerScopeRequiresCapabilityAuthorizerAndUsesActiveTenant(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "store.json"))
	user := adminUser{ID: "user_owner_scope", TenantID: "tenant_user_record"}
	if owner, err := pptOwnerForCapability(store, user); err == nil {
		t.Fatalf("owner without capability authorizer = %#v, want fail closed", owner)
	}
	authorized := &pptAgentAuthorizationStore{
		jsonStore: store,
		authorization: modelCallAuthorization{
			TenantID: "tenant_active_capability", OrganizationID: "organization_active",
			ContextType: contextEnterprise, BillingScope: contextEnterprise,
			BillingAccountID: "tenant_active_capability", ServiceState: "ACTIVE",
		},
	}
	owner, err := pptOwnerForCapability(authorized, user)
	if err != nil {
		t.Fatalf("owner from capability: %v", err)
	}
	if owner != (pptapp.OwnerScope{TenantID: "tenant_active_capability", UserID: "user_owner_scope"}) {
		t.Fatalf("owner = %#v, want active capability tenant and authenticated user", owner)
	}
}

func TestProjectPPTTaskForHTTPDoesNotMutateCanonicalBlocks(t *testing.T) {
	canonical := pptapp.Task{
		TaskID: "ppt_projection", TenantID: "tenant_projection", UserID: "user_projection",
		Slides: []pptapp.Slide{{
			ID: "slide_1", Page: 1,
			Blocks: []pptapp.SlideBlock{
				{Type: "title", Text: "Canonical title"},
				{Type: "paragraph", Text: "Canonical body"},
				{Type: "bullets", Items: []string{"One", "Two"}},
				{Type: "image", ImageRef: "storage://tenant_projection/image_1"},
				{Type: "note", Text: "Canonical notes"},
			},
		}},
	}
	before, err := json.Marshal(canonical)
	if err != nil {
		t.Fatal(err)
	}

	projected := projectPPTTaskForHTTP(canonical)
	slide := projected.Slides[0]
	if slide.Title != "Canonical title" || slide.Content != "Canonical body" || slide.ImageURL != "storage://tenant_projection/image_1" || slide.SpeakerNotes != "Canonical notes" {
		t.Fatalf("HTTP projection = %#v", slide)
	}
	if len(slide.BulletPoints) != 2 || len(slide.Blocks) != 5 {
		t.Fatalf("HTTP projection blocks/bullets = %#v", slide)
	}
	// Mutating projected nested data must not mutate the canonical input.
	projected.Slides[0].Blocks[2].Items[0] = "changed"
	after, err := json.Marshal(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("projection mutated canonical task: before=%s after=%s", before, after)
	}
}

func TestCanonicalPPTSlideUpdateConvertsLegacyBoundaryFieldsToBlocks(t *testing.T) {
	canonical, err := canonicalPPTSlideUpdate("tenant_projection", pptapp.Slide{
		ID: "slide_1", Page: 1, Title: "Boundary title", Content: "Boundary body",
		BulletPoints: []string{"First"}, ImageURL: "storage://tenant_projection/image_1", SpeakerNotes: "Notes",
	})
	if err != nil {
		t.Fatalf("canonical same-tenant update: %v", err)
	}
	if canonical.Title != "" || canonical.Content != "" || canonical.BulletPoints != nil || canonical.ImageURL != "" || canonical.SpeakerNotes != "" {
		t.Fatalf("legacy boundary fields survived canonical conversion: %#v", canonical)
	}
	if len(canonical.Blocks) != 5 {
		t.Fatalf("canonical blocks = %#v", canonical.Blocks)
	}
	projected := projectPPTSlideForHTTP(canonical)
	if projected.Title != "Boundary title" || projected.Content != "Boundary body" || projected.ImageURL != "storage://tenant_projection/image_1" || projected.SpeakerNotes != "Notes" || len(projected.BulletPoints) != 1 {
		t.Fatalf("round-trip boundary projection = %#v", projected)
	}
}

func TestCanonicalPPTSlideUpdateRejectsNetworkAndCrossTenantImages(t *testing.T) {
	for _, reference := range []string{
		"https://example.invalid/image.png",
		"data:image/png;base64,AAAA",
		"file:///tmp/image.png",
		"storage://tenant_other/image_1",
	} {
		t.Run(reference, func(t *testing.T) {
			if _, err := canonicalPPTSlideUpdate("tenant_projection", pptapp.Slide{ImageURL: reference}); err == nil {
				t.Fatalf("legacy image reference %q was accepted", reference)
			}
			if _, err := canonicalPPTSlideUpdate("tenant_projection", pptapp.Slide{Blocks: []pptapp.SlideBlock{{Type: "image", ImageRef: reference}}}); err == nil {
				t.Fatalf("canonical image block reference %q was accepted", reference)
			}
		})
	}
}

func TestApplyPPTCapabilityContextOverwritesClientOwnershipAndBillingFields(t *testing.T) {
	req := pptapp.GenerateRequest{
		Owner:    pptapp.OwnerScope{TenantID: "attacker_tenant", UserID: "attacker_user"},
		TenantID: "attacker_tenant", UserID: "attacker_user", OrganizationID: "attacker_org",
		ContextType: "ATTACKER", BillingScope: "ATTACKER", BillingAccountID: "attacker_account",
	}
	user := adminUser{ID: "trusted_user", TenantID: "user_tenant", OrganizationID: "user_org"}
	applyPPTCapabilityContext(&req, user, map[string]any{
		"tenant_id": "trusted_tenant", "organization_id": "trusted_org", "context_type": contextEnterprise,
		"billing_scope": contextEnterprise, "billing_account_id": "trusted_tenant",
	})
	if req.Owner != (pptapp.OwnerScope{TenantID: "trusted_tenant", UserID: "trusted_user"}) || req.TenantID != "trusted_tenant" || req.UserID != "trusted_user" {
		t.Fatalf("trusted owner context = %#v request=%#v", req.Owner, req)
	}
	if req.OrganizationID != "trusted_org" || req.ContextType != contextEnterprise || req.BillingScope != contextEnterprise || req.BillingAccountID != "trusted_tenant" {
		t.Fatalf("trusted billing context = %#v", req)
	}
}
