package httpserver

import (
	"strings"

	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
)

func pptOwnerForUser(user adminUser) pptapp.OwnerScope {
	return pptapp.OwnerScope{
		TenantID: firstNonEmptyString(user.TenantID, "tenant_default"),
		UserID:   strings.TrimSpace(user.ID),
	}
}

func pptOwnerForTask(user adminUser, task pptapp.Task) pptapp.OwnerScope {
	return pptapp.OwnerScope{
		TenantID: firstNonEmptyString(task.TenantID, user.TenantID, "tenant_default"),
		UserID:   strings.TrimSpace(user.ID),
	}
}

func applyPPTCapabilityContext(req *pptapp.GenerateRequest, user adminUser, params map[string]any) {
	if req == nil {
		return
	}
	tenantID := firstNonEmptyString(stringValue(params["tenant_id"]), user.TenantID, "tenant_default")
	req.Owner = pptapp.OwnerScope{TenantID: tenantID, UserID: strings.TrimSpace(user.ID)}
	req.TenantID = tenantID
	req.UserID = strings.TrimSpace(user.ID)
	req.OrganizationID = firstNonEmptyString(stringValue(params["organization_id"]), user.OrganizationID, defaultOrganizationID(tenantID))
	req.ContextType = firstNonEmptyString(stringValue(params["context_type"]), stringValue(params["billing_scope"]), contextPersonal)
	req.BillingScope = firstNonEmptyString(stringValue(params["billing_scope"]), contextPersonal)
	req.BillingAccountID = firstNonEmptyString(stringValue(params["billing_account_id"]), req.UserID)
}

func canonicalPPTSlideUpdate(slide pptapp.Slide) pptapp.Slide {
	if len(slide.Blocks) == 0 {
		if value := strings.TrimSpace(slide.Title); value != "" {
			slide.Blocks = append(slide.Blocks, pptapp.SlideBlock{Type: "title", Text: value})
		}
		if value := strings.TrimSpace(slide.Content); value != "" {
			slide.Blocks = append(slide.Blocks, pptapp.SlideBlock{Type: "paragraph", Text: value})
		}
		if len(slide.BulletPoints) > 0 {
			slide.Blocks = append(slide.Blocks, pptapp.SlideBlock{Type: "bullets", Items: append([]string(nil), slide.BulletPoints...)})
		}
		if value := strings.TrimSpace(slide.ImageURL); value != "" {
			slide.Blocks = append(slide.Blocks, pptapp.SlideBlock{Type: "image", ImageRef: value})
		}
		if value := strings.TrimSpace(slide.SpeakerNotes); value != "" {
			slide.Blocks = append(slide.Blocks, pptapp.SlideBlock{Type: "note", Text: value})
		}
	}
	slide.Title = ""
	slide.Content = ""
	slide.BulletPoints = nil
	slide.ImageURL = ""
	slide.SpeakerNotes = ""
	return slide
}

func projectPPTTaskForHTTP(task pptapp.Task) pptapp.Task {
	task.Slides = append([]pptapp.Slide(nil), task.Slides...)
	for index := range task.Slides {
		task.Slides[index] = projectPPTSlideForHTTP(task.Slides[index])
	}
	return task
}

func projectPPTSlideForHTTP(slide pptapp.Slide) pptapp.Slide {
	slide.Blocks = clonePPTSlideBlocks(slide.Blocks)
	slide.BulletPoints = nil
	slide.Title = firstPPTBlockText(slide.Blocks, "title")
	slide.Content = firstPPTBlockText(slide.Blocks, "paragraph")
	slide.SpeakerNotes = firstPPTBlockText(slide.Blocks, "note")
	for _, block := range slide.Blocks {
		switch strings.ToLower(strings.TrimSpace(block.Type)) {
		case "bullets":
			if slide.BulletPoints == nil {
				slide.BulletPoints = append([]string(nil), block.Items...)
			}
		case "image":
			if slide.ImageURL == "" {
				slide.ImageURL = strings.TrimSpace(block.ImageRef)
			}
		}
	}
	return slide
}

func clonePPTSlideBlocks(blocks []pptapp.SlideBlock) []pptapp.SlideBlock {
	cloned := append([]pptapp.SlideBlock(nil), blocks...)
	for index := range cloned {
		cloned[index].Items = append([]string(nil), cloned[index].Items...)
	}
	return cloned
}

func firstPPTBlockText(blocks []pptapp.SlideBlock, blockType string) string {
	for _, block := range blocks {
		if strings.EqualFold(strings.TrimSpace(block.Type), blockType) {
			return strings.TrimSpace(block.Text)
		}
	}
	return ""
}
