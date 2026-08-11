package httpserver

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"
	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
	"xianzhi-ai/backend-go/internal/config"
)

func TestPPTVisualHTTPDisablePreservesContentAndEnforcesOwnership(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	store := newJSONStore(dataPath)
	grantPermanentTestPoints(t, store, "user_000002", 100)
	server := newWithStore(config.Config{Addr: ":0", DataPath: dataPath, StaticDir: t.TempDir(), PPTAutoImageMode: "disabled"}, store)
	handler := server.Handler
	ownerToken := loginToken(t, handler, "demo@xianzhi.ai", "Demo123!")
	otherToken := loginToken(t, handler, "agent1@xianzhi.ai", "Agent123!")

	createBody := bytes.NewBufferString(`{
		"prompt":"HTTP visual contract test",
		"slideCount":1,
		"imageSource":"none",
		"outline":{"title":"Contract deck","slides":[{"page":1,"title":"Keep this title","summary":"Keep this complete slide body","bulletPoints":["Keep this bullet"],"layout":"imageText","slideType":"text_image"}]}
	}`)
	createdResponse := authedRequest(t, handler, http.MethodPost, "/api/v1/ppt/generate", createBody, ownerToken)
	if createdResponse.Code != http.StatusOK {
		t.Fatalf("create ppt status = %d, body = %s", createdResponse.Code, createdResponse.Body.String())
	}
	var created pptapp.GenerateResponse
	if err := json.NewDecoder(createdResponse.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}

	taskResponse := authedRequest(t, handler, http.MethodGet, "/api/v1/ppt/tasks/"+created.TaskID, nil, ownerToken)
	if taskResponse.Code != http.StatusOK {
		t.Fatalf("get ppt status = %d, body = %s", taskResponse.Code, taskResponse.Body.String())
	}
	var task pptapp.Task
	if err := json.NewDecoder(taskResponse.Body).Decode(&task); err != nil {
		t.Fatal(err)
	}
	if len(task.Slides) != 1 {
		t.Fatalf("unexpected slides: %#v", task.Slides)
	}
	original := task.Slides[0]
	visualPath := "/api/v1/presentations/" + created.TaskID + "/slides/" + original.ID + "/regenerate-visual"
	payload := `{"visualType":"none","style":"corporate_3d","composition":"image_right","customInstruction":"use a calm enterprise scene","keepCurrentContent":true}`

	unauthenticated := request(t, handler, http.MethodPost, visualPath, bytes.NewBufferString(payload))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated visual update status = %d, body = %s", unauthenticated.Code, unauthenticated.Body.String())
	}
	otherUser := authedRequest(t, handler, http.MethodPost, visualPath, bytes.NewBufferString(payload), otherToken)
	if otherUser.Code != http.StatusNotFound {
		t.Fatalf("cross-user visual update status = %d, body = %s", otherUser.Code, otherUser.Body.String())
	}

	updatedResponse := authedRequest(t, handler, http.MethodPost, visualPath, bytes.NewBufferString(payload), ownerToken)
	if updatedResponse.Code != http.StatusOK {
		t.Fatalf("disable visual status = %d, body = %s", updatedResponse.Code, updatedResponse.Body.String())
	}
	var updated pptRegenerateVisualResponse
	if err := json.NewDecoder(updatedResponse.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	if updated.Status != "success" || updated.Slide.Title != original.Title || updated.Slide.Content != original.Content || !equalStringSlices(updated.Slide.BulletPoints, original.BulletPoints) {
		t.Fatalf("visual-only HTTP update changed slide content: before=%#v after=%#v", original, updated.Slide)
	}
	if updated.Slide.VisualPlan == nil || updated.Slide.VisualPlan.ImageRequired || updated.Slide.VisualPlan.TextInImage || updated.Slide.VisualPlan.VisualType != "none" {
		t.Fatalf("visual disable response is invalid: %#v", updated.Slide.VisualPlan)
	}
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func TestPPTImagePromptUsesVisualPlanInsteadOfSlideBody(t *testing.T) {
	body := "Complete slide prose that must not be copied to an image prompt."
	plan := pptapp.NormalizeVisualPlan(pptapp.VisualPlan{}, pptapp.VisualPlannerInput{SlideType: "text_image", SlideTitle: "AI service", CoreIdea: "collaborative AI assistant"})
	prompt := pptImagePrompt(pptImageGenerateRequest{Slide: pptapp.Slide{Title: "AI service", Content: body, SlideType: "text_image"}, VisualPlan: &plan})
	if strings.Contains(prompt, body) {
		t.Fatalf("prompt contains full body: %s", prompt)
	}
	if !strings.Contains(strings.ToLower(prompt), "presentation visual without text") {
		t.Fatalf("prompt missing positive no-text instruction: %s", prompt)
	}
}

func TestPPTImageTaskAlwaysUsesNormalizedNoTextPlan(t *testing.T) {
	req := pptImageGenerateRequest{
		Slide:      pptapp.Slide{ID: "slide_1", Title: "AI service", Content: "Full slide copy that belongs in the layout only.", SlideType: "text_image"},
		VisualPlan: &pptapp.VisualPlan{VisualType: "illustration", ImageRequired: true, TextInImage: true, NegativePrompt: "low quality"},
	}
	created := pptImageGenerationCreateRequest(adminUser{ID: "user_1"}, req, "model_1", "ppt_1")
	negative := strings.ToLower(stringValue(created.Params["negativePrompt"]))
	for _, required := range []string{"low quality", "text", "letters", "words", "typography", "numbers", "logo", "watermark", "captions", "subtitles", "garbled text"} {
		if !strings.Contains(negative, required) {
			t.Fatalf("normalized negative prompt missing %q: %s", required, negative)
		}
	}
	plan, ok := created.Params["visualPlan"].(*pptapp.VisualPlan)
	if !ok || plan == nil || plan.TextInImage {
		t.Fatalf("generation task did not store normalized no-text visual plan: %#v", created.Params["visualPlan"])
	}
}

func TestPPTVisualRolloutModesDefaultSafeAndAllowExplicitDegradation(t *testing.T) {
	if !pptVisualPlannerModelEnabled(config.Config{}) || !pptAutoImageEnabled(config.Config{}) || !pptVisualOCRStrict(config.Config{}) {
		t.Fatal("zero-value rollout configuration must preserve enabled and strict defaults")
	}
	degraded := config.Config{
		PPTVisualPlannerMode: "local", PPTAutoImageMode: "disabled", PPTVisualOCRFailureMode: "fail_open",
	}
	if pptVisualPlannerModelEnabled(degraded) || pptAutoImageEnabled(degraded) || pptVisualOCRStrict(degraded) {
		t.Fatalf("explicit degradation modes were ignored: %#v", degraded)
	}
}

func TestDuplicateVisualRegenerationIsRejected(t *testing.T) {
	var locks sync.Map
	if !acquirePPTVisualTask(&locks, "presentation:slide") {
		t.Fatal("first visual task should acquire the slide lock")
	}
	if acquirePPTVisualTask(&locks, "presentation:slide") {
		t.Fatal("duplicate visual task should not acquire the slide lock")
	}
}

type sharedVisualTestLocker struct {
	mu   sync.Mutex
	held map[string]bool
}

func (l *sharedVisualTestLocker) TryAcquire(_ context.Context, key string, _ time.Duration) (func(), bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.held == nil {
		l.held = map[string]bool{}
	}
	if l.held[key] {
		return func() {}, false, nil
	}
	l.held[key] = true
	return func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		delete(l.held, key)
	}, true, nil
}

func TestSharedVisualLockerRejectsCrossInstanceDuplicateAndAllowsRetryAfterRelease(t *testing.T) {
	locker := &sharedVisualTestLocker{}
	release, acquired, err := locker.TryAcquire(context.Background(), "user:presentation:slide", pptVisualLockTTL)
	if err != nil || !acquired {
		t.Fatalf("first instance should acquire distributed lock: acquired=%v err=%v", acquired, err)
	}
	if _, acquired, err = locker.TryAcquire(context.Background(), "user:presentation:slide", pptVisualLockTTL); err != nil || acquired {
		t.Fatalf("second instance should be rejected while lock is held: acquired=%v err=%v", acquired, err)
	}
	release()
	if _, acquired, err = locker.TryAcquire(context.Background(), "user:presentation:slide", pptVisualLockTTL); err != nil || !acquired {
		t.Fatalf("retry should acquire after release: acquired=%v err=%v", acquired, err)
	}
}

func TestVisualOperationHelperCoordinatesLocalAndDistributedLocks(t *testing.T) {
	locker := &sharedVisualTestLocker{}
	var firstLocal, secondLocal sync.Map
	firstAPI := api{pptVisualTasks: &firstLocal, pptVisualLocker: locker}
	secondAPI := api{pptVisualTasks: &secondLocal, pptVisualLocker: locker}
	key := "user:presentation:slide"

	releaseFirst, acquired, err := firstAPI.tryAcquirePPTVisualOperation(context.Background(), key)
	if err != nil || !acquired {
		t.Fatalf("first operation should acquire both locks: acquired=%v err=%v", acquired, err)
	}
	if _, acquired, err = secondAPI.tryAcquirePPTVisualOperation(context.Background(), key); err != nil || acquired {
		t.Fatalf("second instance should be rejected: acquired=%v err=%v", acquired, err)
	}
	releaseFirst()
	releaseSecond, acquired, err := secondAPI.tryAcquirePPTVisualOperation(context.Background(), key)
	if err != nil || !acquired {
		t.Fatalf("operation should acquire after release: acquired=%v err=%v", acquired, err)
	}
	releaseSecond()
}

func TestPPTVisualOCRRejectsReadableTextBeforeStorage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"pages":[{"page":1,"text":"AI"}]}`))
	}))
	defer server.Close()
	a := api{cfg: config.Config{KnowledgeOCREndpoint: server.URL, KnowledgeOCRProvider: "test_ocr"}}
	raw := base64.StdEncoding.EncodeToString([]byte("test image bytes"))
	err := a.validatePPTImageHasNoText(t.Context(), "task_1", generation.CreateRequest{GeneratedImages: []generation.GeneratedImage{{URL: "data:image/png;base64," + raw, ContentType: "image/png"}}})
	if !errors.Is(err, errPPTImageContainsText) {
		t.Fatalf("expected readable text rejection, got %v", err)
	}
}

func TestPPTVisualOCRAllowsNoTextAndFailsClosedWhenConfiguredProviderIsUnavailable(t *testing.T) {
	responses := []struct {
		status int
		body   string
	}{{http.StatusOK, `{"pages":[{"page":1,"text":""}]}`}, {http.StatusBadGateway, `{"error":"offline"}`}}
	for _, response := range responses {
		response := response
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(response.status)
			_, _ = w.Write([]byte(response.body))
		}))
		a := api{cfg: config.Config{KnowledgeOCREndpoint: server.URL}}
		raw := base64.StdEncoding.EncodeToString([]byte("test image bytes"))
		err := a.validatePPTImageHasNoText(t.Context(), "task_1", generation.CreateRequest{GeneratedImages: []generation.GeneratedImage{{URL: "data:image/png;base64," + raw}}})
		if response.status == http.StatusOK && err != nil {
			server.Close()
			t.Fatalf("unexpected no-text validation error: %v", err)
		}
		if response.status != http.StatusOK && !errors.Is(err, errPPTVisualTextValidationUnavailable) {
			server.Close()
			t.Fatalf("configured OCR outage must fail closed, got %v", err)
		}
		server.Close()
	}
}

func TestPPTVisualOCRCanFailOpenOnlyWhenExplicitlyConfigured(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"offline"}`))
	}))
	defer server.Close()
	a := api{cfg: config.Config{KnowledgeOCREndpoint: server.URL, PPTVisualOCRFailureMode: "fail_open"}}
	raw := base64.StdEncoding.EncodeToString([]byte("test image bytes"))
	if err := a.validatePPTImageHasNoText(t.Context(), "task_1", generation.CreateRequest{GeneratedImages: []generation.GeneratedImage{{URL: "data:image/png;base64," + raw}}}); err != nil {
		t.Fatalf("explicit fail-open mode should allow emergency degradation: %v", err)
	}
}

func TestPPTOCRReadableTextThreshold(t *testing.T) {
	if pptOCRContainsReadableText([]knowledgeapp.DocumentUnit{{Content: "A"}}) {
		t.Fatal("one OCR glyph should be treated as noise")
	}
	if !pptOCRContainsReadableText([]knowledgeapp.DocumentUnit{{Content: "乱码"}}) {
		t.Fatal("two readable glyphs should be rejected")
	}
}

func TestPPTImageRetryAddsFreshSeedMetadata(t *testing.T) {
	req := pptImageGenerateRequest{Slide: pptapp.Slide{ID: "slide_1", SlideType: "cover"}, RetryAttempt: 2}
	created := pptImageGenerationCreateRequest(adminUser{ID: "user_1"}, req, "image-model", "ppt_1")
	if created.Params["retryAttempt"] != 2 {
		t.Fatalf("retry attempt missing from image request: %#v", created.Params)
	}
	if seed, ok := created.Params["seed"].(int64); !ok || seed == 0 {
		t.Fatalf("fresh seed missing from image request: %#v", created.Params["seed"])
	}
	prompt := pptImagePrompt(req)
	if !strings.Contains(prompt, "different camera angle") || !strings.Contains(strings.ToLower(prompt), "no text") {
		t.Fatalf("retry prompt should vary composition and retain no-text rules: %s", prompt)
	}
}
