package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
	"xianzhi-ai/backend-go/internal/app/generation"
	chatprovider "xianzhi-ai/backend-go/internal/provider/chat"
)

func (a api) runPPTTaskGeneration(user adminUser, task pptapp.Task) {
	if task.TaskID == "" || len(task.Slides) == 0 {
		log.Printf("ppt content generation skipped: task has no slides taskId=%s", task.TaskID)
		a.finalizePPTGeneration(user, task, pptapp.StatusFailed)
		return
	}

	model := firstNonEmptyString(task.TextModel, a.cfg.PPTTextModel)
	if model == "" || !a.pptProviderConfigured() {
		log.Printf("ppt content generation skipped: no text model configured taskId=%s", task.TaskID)
		a.finalizePPTGeneration(user, task, pptapp.StatusSuccess)
		return
	}

	log.Printf("ppt content generation started taskId=%s slides=%d model=%s", task.TaskID, len(task.Slides), model)

	if _, err := a.pptService.UpdateTaskStatus(user.ID, task.TaskID, pptapp.StatusGenerating, 5, 0); err != nil {
		log.Printf("ppt content generation failed to set status taskId=%s err=%v", task.TaskID, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	generatedSlides, err := a.generatePPTContentBatch(ctx, task, model)
	if err != nil {
		log.Printf("ppt content generation failed taskId=%s err=%v", task.TaskID, err)
		a.finalizePPTGeneration(user, task, pptapp.StatusFailed)
		_, _ = a.pptService.UpdateTaskStatus(user.ID, task.TaskID, pptapp.StatusFailed, 0, 0)
		_ = a.pptService.SetTaskError(user.ID, task.TaskID, generationErrorMessage(err))
		return
	}

	if _, err := a.pptService.UpdateTaskStatus(user.ID, task.TaskID, pptapp.StatusGenerating, 60, 0); err != nil {
		log.Printf("ppt content generation status update failed taskId=%s err=%v", task.TaskID, err)
	}

	for i, genSlide := range generatedSlides {
		if i >= len(task.Slides) {
			break
		}
		progress := 60 + int(float64(i+1)/float64(len(task.Slides))*30)
		update := pptapp.Slide{
			Title:        firstNonEmptyString(genSlide.Title, task.Slides[i].Title),
			Content:      firstNonEmptyString(genSlide.Content, task.Slides[i].Content),
			BulletPoints: selectNonEmptyStrings(genSlide.BulletPoints, task.Slides[i].BulletPoints),
			SpeakerNotes: firstNonEmptyString(genSlide.SpeakerNotes, task.Slides[i].SpeakerNotes),
			Layout:       task.Slides[i].Layout,
		}
		_, err := a.pptService.UpdateSlideContent(user.ID, task.TaskID, task.Slides[i].ID, update)
		if err != nil {
			log.Printf("ppt slide update failed taskId=%s slideId=%s err=%v", task.TaskID, task.Slides[i].ID, err)
		}
		if _, statusErr := a.pptService.UpdateTaskStatus(user.ID, task.TaskID, pptapp.StatusGenerating, progress, i+1); statusErr != nil {
			log.Printf("ppt content progress update failed taskId=%s err=%v", task.TaskID, statusErr)
		}
	}

	if a.shouldAutoGeneratePPTImagesFromTask(task) && pptAutoImageEnabled(a.cfg) {
		if _, err := a.pptService.UpdateTaskStatus(user.ID, task.TaskID, pptapp.StatusRendering, 90, len(task.Slides)); err != nil {
			log.Printf("ppt rendering status update failed taskId=%s err=%v", task.TaskID, err)
		}
		go a.runPPTTaskImageGeneration(user, task)
		return
	}

	a.finalizePPTGeneration(user, task, pptapp.StatusSuccess)
}

func (a api) finalizePPTGeneration(user adminUser, task pptapp.Task, status string) {
	if status == pptapp.StatusFailed {
		if _, err := a.pptService.UpdateTaskStatus(user.ID, task.TaskID, status, 0, task.SlideCount); err != nil {
			log.Printf("ppt task finalize failed taskId=%s err=%v", task.TaskID, err)
		}
		return
	}
	if !a.shouldAutoGeneratePPTImagesFromTask(task) {
		if _, err := a.pptService.UpdateTaskStatus(user.ID, task.TaskID, pptapp.StatusSuccess, 100, task.SlideCount); err != nil {
			log.Printf("ppt task success update failed taskId=%s err=%v", task.TaskID, err)
		}
	}
}

func (a api) generatePPTContentBatch(ctx context.Context, task pptapp.Task, model string) ([]generatedSlideContent, error) {
	provider := chatprovider.NewOpenAICompatibleForModel(a.cfg, model)
	response, err := provider.Chat(ctx, generation.CreateRequest{
		Type:  "CHAT_COMPLETION",
		Model: model,
		Params: map[string]any{
			"temperature": 0.3,
			"max_tokens":  maxPPTSlideContentTokens(len(task.Slides)),
			"messages":    pptContentMessages(task),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("ppt content generation model call failed: %w", err)
	}
	return parsePPTSlideContentOutput(response.Message.Content)
}

type generatedSlideContent struct {
	Page         int      `json:"page"`
	Title        string   `json:"title"`
	Content      string   `json:"content"`
	BulletPoints []string `json:"bulletPoints"`
	SpeakerNotes string   `json:"speakerNotes"`
}

func pptContentMessages(task pptapp.Task) []any {
	var outlineBuilder strings.Builder
	for i, slide := range task.Slides {
		outlineBuilder.WriteString(fmt.Sprintf(
			"Page %d | type: %s | layout: %s\nTitle: %s\nSummary: %s\nBullet points: %s\n\n",
			i+1, slide.SlideType, slide.Layout, slide.Title, slide.Content, strings.Join(slide.BulletPoints, " | "),
		))
	}
	return []any{
		map[string]any{
			"role": "system",
			"content": strings.Join([]string{
				"You generate rich presentation slide content from outlines.",
				"Return only a JSON array. Do not wrap in Markdown.",
				"Each item: {\"page\":1,\"title\":\"...\",\"content\":\"2-3 sentence paragraph\",\"bulletPoints\":[\"3-5 polished bullets\"],\"speakerNotes\":\"2-3 sentences for the presenter\"}",
				"Use Simplified Chinese when the outline is in Chinese. Use English when it's in English.",
				"Make the content professional, substantive, and presentation-ready.",
				"Do not copy the outline verbatim — expand and enrich each slide.",
			}, "\n"),
		},
		map[string]any{
			"role": "user",
			"content": fmt.Sprintf(
				"Deck title: %s\nTheme: %s\nLanguage: %s\nTone: %s\nAudience: %s\nScenario: %s\n\nSlide outline:\n%sGenerate rich, polished content for each slide. Return a JSON array with one object per page in order.",
				task.Title, task.Theme, task.Language, task.Tone, task.Audience, task.Scenario, outlineBuilder.String(),
			),
		},
	}
}

func maxPPTSlideContentTokens(slideCount int) int {
	if slideCount <= 0 {
		slideCount = 5
	}
	tokens := 2000 + slideCount*600
	if tokens < 3200 {
		return 3200
	}
	if tokens > 12000 {
		return 12000
	}
	return tokens
}

func parsePPTSlideContentOutput(content string) ([]generatedSlideContent, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, fmt.Errorf("ppt content model output is empty")
	}
	var slides []generatedSlideContent
	if err := json.Unmarshal([]byte(content), &slides); err != nil {
		extracted := extractJSONArray(content)
		if extracted == "" {
			return nil, fmt.Errorf("parse ppt content model output: %w", err)
		}
		if parseErr := json.Unmarshal([]byte(extracted), &slides); parseErr != nil {
			return nil, fmt.Errorf("parse ppt content model output: %w", parseErr)
		}
	}
	if len(slides) == 0 {
		return nil, fmt.Errorf("ppt content model output contains no slides")
	}
	return slides, nil
}

func extractJSONArray(content string) string {
	start := strings.Index(content, "[")
	end := strings.LastIndex(content, "]")
	if start < 0 || end <= start {
		return ""
	}
	return content[start : end+1]
}

func selectNonEmptyStrings(preferred []string, fallback []string) []string {
	if len(preferred) > 0 {
		result := make([]string, 0, len(preferred))
		for _, s := range preferred {
			if strings.TrimSpace(s) != "" {
				result = append(result, s)
			}
		}
		if len(result) > 0 {
			return result
		}
	}
	return fallback
}

func (a api) shouldAutoGeneratePPTImagesFromTask(task pptapp.Task) bool {
	if strings.TrimSpace(task.ImageSource) != "ai" {
		return false
	}
	model := pptImageProviderModel(task.ImageModel, a.cfg.ImageModel)
	if strings.TrimSpace(model) == "" {
		return false
	}
	return len(task.Slides) > 0
}
