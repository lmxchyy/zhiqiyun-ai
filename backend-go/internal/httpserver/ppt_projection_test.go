package httpserver

import (
	"encoding/json"
	"testing"

	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
)

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
	canonical := canonicalPPTSlideUpdate(pptapp.Slide{
		ID: "slide_1", Page: 1, Title: "Boundary title", Content: "Boundary body",
		BulletPoints: []string{"First"}, ImageURL: "https://example.invalid/image.png", SpeakerNotes: "Notes",
	})
	if canonical.Title != "" || canonical.Content != "" || canonical.BulletPoints != nil || canonical.ImageURL != "" || canonical.SpeakerNotes != "" {
		t.Fatalf("legacy boundary fields survived canonical conversion: %#v", canonical)
	}
	if len(canonical.Blocks) != 5 {
		t.Fatalf("canonical blocks = %#v", canonical.Blocks)
	}
	projected := projectPPTSlideForHTTP(canonical)
	if projected.Title != "Boundary title" || projected.Content != "Boundary body" || projected.ImageURL != "https://example.invalid/image.png" || projected.SpeakerNotes != "Notes" || len(projected.BulletPoints) != 1 {
		t.Fatalf("round-trip boundary projection = %#v", projected)
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
