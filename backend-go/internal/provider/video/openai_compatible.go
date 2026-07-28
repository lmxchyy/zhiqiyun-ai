package video

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"
)

type OpenAICompatible struct {
	code          string
	baseURL       string
	apiKey        string
	model         string
	models        []string
	endpoint      string
	client        *http.Client
	timeoutMS     int
	outputDir     string
	publicURLBase string
	bridgeScript  string
	bridgePython  string
}

type OpenAICompatibleOptions struct {
	Code          string
	BaseURL       string
	APIKey        string
	Model         string
	Models        []string
	Endpoint      string
	TimeoutMS     int
	OutputDir     string
	PublicURLBase string
	BridgeScript  string
	BridgePython  string
}

func NewOpenAICompatibleWithOptions(opts OpenAICompatibleOptions) OpenAICompatible {
	timeoutMS := opts.TimeoutMS
	if timeoutMS <= 0 {
		timeoutMS = 180000
	}
	model := strings.TrimSpace(opts.Model)
	models := uniqueStrings(opts.Models...)
	if model != "" {
		models = appendIfMissing(models, model)
	}
	return OpenAICompatible{
		code:          strings.TrimSpace(opts.Code),
		baseURL:       strings.TrimSpace(opts.BaseURL),
		apiKey:        strings.TrimSpace(opts.APIKey),
		model:         model,
		models:        models,
		endpoint:      strings.TrimSpace(opts.Endpoint),
		client:        &http.Client{Timeout: time.Duration(timeoutMS) * time.Millisecond},
		timeoutMS:     timeoutMS,
		outputDir:     strings.TrimSpace(opts.OutputDir),
		publicURLBase: strings.TrimSpace(opts.PublicURLBase),
		bridgeScript:  strings.TrimSpace(opts.BridgeScript),
		bridgePython:  strings.TrimSpace(opts.BridgePython),
	}
}

func (p OpenAICompatible) DefaultModel() string {
	return p.model
}

func (p OpenAICompatible) Create(ctx context.Context, req generation.CreateRequest) (any, error) {
	if strings.TrimSpace(p.baseURL) == "" || strings.TrimSpace(p.apiKey) == "" {
		return nil, errors.New("video provider requires base url and api key")
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = p.model
	}
	if model == "" {
		return nil, errors.New("video model is required")
	}
	if len(p.models) > 0 && !containsString(p.models, model) {
		return nil, fmt.Errorf("video provider %s does not support model %s", p.providerCode(), model)
	}
	imageURLs := referenceImageURLs(req.Params)
	if isGrokVideo15Model(model) {
		if len(imageURLs) == 0 {
			return nil, errors.New("Grok Video 1.5 requires exactly one reference image")
		}
		if len(imageURLs) > 1 {
			return nil, errors.New("Grok Video 1.5 supports exactly one reference image")
		}
	}
	if p.shouldUseSeedanceBridge(model) {
		return p.createWithSeedanceBridge(ctx, model, req)
	}
	body := videoRequestBodyForEndpoint(model, req, p.endpoint, imageURLs)
	if len(imageURLs) > 0 && !useSeedanceContentTaskProtocol(p.endpoint) {
		body["image_urls"] = imageURLs
		if len(imageURLs) == 1 {
			body["input_reference"] = map[string]any{"image_url": imageURLs[0]}
		}
	}
	if ratio := strings.TrimSpace(fmt.Sprint(req.Params["ratio"])); ratio != "" && ratio != "<nil>" {
		body["ratio"] = ratio
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, videoProviderEndpointForModel(p.baseURL, p.endpoint, model), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	res, err := p.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 12<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("video provider %s returned HTTP %d: %s", p.providerCode(), res.StatusCode, strings.TrimSpace(string(raw)))
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("decode video provider response: %w", err)
	}
	decoded = p.pollVideoResult(ctx, decoded, model)
	videoURL := firstStringByKeys(decoded, "videoUrl", "video_url", "url", "output_url")
	if videoURL == "" {
		videoURL = firstStringByKeys(decoded, "result_url")
	}
	thumbnailURL := firstStringByKeys(decoded, "thumbnailUrl", "thumbnail_url", "coverUrl", "cover_url", "poster", "image_url")
	status := normalizeVideoStatus(firstNonEmptyString(firstStringByKeys(decoded, "status", "state"), "SUCCEEDED"))
	if status == "FAILED" {
		reason := firstStringByKeys(decoded, "fail_reason", "error", "message")
		if reason == "" {
			reason = "video generation failed"
		}
		return nil, errors.New(reason)
	}
	if videoURL == "" && strings.Contains(status, "PROCESS") {
		status = "PROCESSING"
	}
	taskID := firstNonEmptyString(firstStringByKeys(decoded, "id", "taskId", "task_id", "providerTaskId"), "video-"+strconv.FormatInt(time.Now().UnixNano(), 10))
	if videoURL == "" && strings.EqualFold(status, "SUCCEEDED") && taskID != "" {
		videoURL = videoContentEndpointForModel(p.baseURL, p.endpoint, taskID, model)
	}
	if videoURL == "" {
		return nil, fmt.Errorf("video task %s is still processing; no result_url returned yet", taskID)
	}
	return map[string]any{
		"provider":       p.providerCode(),
		"providerTaskId": taskID,
		"status":         status,
		"videoUrl":       videoURL,
		"thumbnailUrl":   thumbnailURL,
		"raw":            decoded,
		"metadata": map[string]any{
			"duration":   req.Params["duration"],
			"ratio":      req.Params["ratio"],
			"resolution": req.Params["resolution"],
		},
	}, nil
}

func videoRequestBody(model string, req generation.CreateRequest) map[string]any {
	if isDoubaoSeedance2Model(model) {
		return map[string]any{
			"model":      model,
			"prompt":     req.Prompt,
			"duration":   videoSeconds(req.Params),
			"size":       videoAspectRatio(req.Params),
			"resolution": videoResolution(req.Params),
		}
	}
	return map[string]any{
		"model":        model,
		"prompt":       req.Prompt,
		"seconds":      videoSeconds(req.Params),
		"aspect_ratio": videoAspectRatio(req.Params),
		"resolution":   videoResolution(req.Params),
	}
}

func videoRequestBodyForEndpoint(model string, req generation.CreateRequest, endpoint string, imageURLs []string) map[string]any {
	if isDoubaoSeedance2Model(model) && useSeedanceContentTaskProtocol(endpoint) {
		firstFrame, lastFrame := videoFrameURLs(req.Params, imageURLs)
		return map[string]any{
			"model":             model,
			"content":           seedanceContentItems(req.Prompt, firstFrame, lastFrame),
			"ratio":             videoAspectRatio(req.Params),
			"duration":          videoSeconds(req.Params),
			"resolution":        videoResolution(req.Params),
			"watermark":         false,
			"generate_audio":    videoGenerateAudio(req.Params),
			"return_last_frame": false,
		}
	}
	return videoRequestBody(model, req)
}

func seedanceContentItems(prompt string, firstFrame string, lastFrame string) []map[string]any {
	items := []map[string]any{{"type": "text", "text": prompt}}
	for _, frame := range []struct {
		url  string
		role string
	}{{firstFrame, "first_frame"}, {lastFrame, "last_frame"}} {
		if strings.TrimSpace(frame.url) == "" {
			continue
		}
		items = append(items, map[string]any{
			"type":      "image_url",
			"image_url": map[string]any{"url": frame.url},
			"role":      frame.role,
		})
	}
	return items
}

func normalizeVideoStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "SUCCESS", "SUCCEEDED", "COMPLETED":
		return "SUCCEEDED"
	case "FAILURE", "FAILED", "ERROR":
		return "FAILED"
	case "SUBMITTED", "QUEUED", "IN_PROGRESS", "NOT_START", "PENDING", "PROCESSING", "RUNNING":
		return "PROCESSING"
	default:
		if strings.TrimSpace(status) == "" {
			return "PROCESSING"
		}
		return strings.ToUpper(strings.TrimSpace(status))
	}
}

func (p OpenAICompatible) providerCode() string {
	if strings.TrimSpace(p.code) != "" {
		return p.code
	}
	return "openai-compatible-video"
}

func isGrokVideo15Model(model string) bool {
	normalized := strings.ToLower(strings.TrimSpace(model))
	normalized = strings.ReplaceAll(normalized, "_", "-")
	return normalized == "grok-video-1.5"
}

func isDoubaoSeedance2Model(model string) bool {
	normalized := strings.ToLower(strings.TrimSpace(model))
	normalized = strings.ReplaceAll(normalized, "_", "-")
	return normalized == "doubao-seedance-2.0" || strings.HasPrefix(normalized, "doubao-seedance-2.0-")
}

func useSeedanceContentTaskProtocol(endpoint string) bool {
	normalized := strings.ToLower(strings.Trim(strings.TrimSpace(endpoint), "/"))
	return strings.Contains(normalized, "contents/generations/tasks")
}

func (p OpenAICompatible) shouldUseSeedanceBridge(model string) bool {
	if !isDoubaoSeedance2Model(model) || !useSeedanceContentTaskProtocol(p.endpoint) {
		return false
	}
	code := strings.ToLower(strings.TrimSpace(p.code))
	base := strings.ToLower(strings.TrimSpace(p.baseURL))
	return strings.Contains(code, "cmecloud") || strings.Contains(code, "cme_cloud") || strings.Contains(base, "cmecloud.cn")
}

func (p OpenAICompatible) createWithSeedanceBridge(ctx context.Context, model string, req generation.CreateRequest) (any, error) {
	outputDir := strings.TrimSpace(p.outputDir)
	if outputDir == "" {
		outputDir = filepath.Join("data", "generated-media")
	}
	publicURLBase := strings.TrimSpace(p.publicURLBase)
	if publicURLBase == "" {
		publicURLBase = "/api/v1/generated-media/"
	}
	bridgeReq := map[string]any{
		"baseUrl":        p.baseURL,
		"apiKey":         seedanceBridgeAPIKey(p.apiKey),
		"model":          model,
		"prompt":         req.Prompt,
		"params":         req.Params,
		"imageUrls":      referenceImageURLs(req.Params),
		"firstFrame":     imageURLFromValue(req.Params["first_frame"]),
		"lastFrame":      imageURLFromValue(req.Params["last_frame"]),
		"outputDir":      outputDir,
		"publicURLBase":  publicURLBase,
		"timeoutSeconds": seedanceBridgeTimeoutSeconds(p.timeoutMS),
	}
	payload, err := json.Marshal(bridgeReq)
	if err != nil {
		return nil, err
	}
	python := firstNonEmptyString(p.bridgePython, os.Getenv("CME_SEEDANCE_PYTHON"), os.Getenv("PYTHON"), "python3", "python")
	script := firstNonEmptyString(p.bridgeScript, os.Getenv("CME_SEEDANCE_BRIDGE"), defaultSeedanceBridgeScript())
	if strings.TrimSpace(script) == "" {
		return nil, errors.New("CMECloud Seedance requires the official MaaS SDK bridge, but no bridge script was configured")
	}
	timeout := time.Duration(seedanceBridgeTimeoutSeconds(p.timeoutMS)+30) * time.Second
	bridgeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(bridgeCtx, python, script)
	cmd.Stdin = bytes.NewReader(payload)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if bridgeCtx.Err() != nil {
			return nil, fmt.Errorf("CMECloud Seedance bridge timed out after %s", timeout)
		}
		return nil, fmt.Errorf("CMECloud Seedance bridge failed: %w: %s", err, redactBridgeSecret(stderr.String(), p.apiKey))
	}
	result, err := decodeSeedanceBridgeResponse(stdout.Bytes())
	if err != nil {
		return nil, fmt.Errorf("decode CMECloud Seedance bridge response: %w: %s", err, redactBridgeSecret(stderr.String(), p.apiKey))
	}
	status := normalizeVideoStatus(firstNonEmptyString(firstStringByKeys(result, "status", "state"), "SUCCEEDED"))
	if status == "FAILED" {
		reason := firstStringByKeys(result, "error", "message", "errorMessage")
		if reason == "" {
			reason = "CMECloud Seedance video generation failed"
		}
		return nil, errors.New(reason)
	}
	videoURL := firstStringByKeys(result, "videoUrl", "video_url", "url", "output_url")
	if videoURL == "" {
		return nil, errors.New("CMECloud Seedance bridge did not return a playable video URL")
	}
	taskID := firstNonEmptyString(firstStringByKeys(result, "providerTaskId", "taskId", "task_id", "id"), "video-"+strconv.FormatInt(time.Now().UnixNano(), 10))
	actualModel := firstStringByKeys(result, "actualModel", "providerModel")
	return map[string]any{
		"provider":       p.providerCode(),
		"providerTaskId": taskID,
		"status":         status,
		"videoUrl":       videoURL,
		"thumbnailUrl":   firstStringByKeys(result, "thumbnailUrl", "thumbnail_url"),
		"raw":            result,
		"metadata": map[string]any{
			"duration":    req.Params["duration"],
			"ratio":       req.Params["ratio"],
			"resolution":  req.Params["resolution"],
			"actualModel": actualModel,
		},
	}, nil
}

const seedanceBridgeResultPrefix = "__XIANZHI_SEEDANCE_RESULT__"

func decodeSeedanceBridgeResponse(stdout []byte) (map[string]any, error) {
	text := strings.TrimSpace(string(stdout))
	if text == "" {
		return nil, errors.New("empty bridge stdout")
	}
	if idx := strings.LastIndex(text, seedanceBridgeResultPrefix); idx >= 0 {
		text = strings.TrimSpace(text[idx+len(seedanceBridgeResultPrefix):])
		if newline := strings.IndexAny(text, "\r\n"); newline >= 0 {
			text = strings.TrimSpace(text[:newline])
		}
		var result map[string]any
		if err := json.Unmarshal([]byte(text), &result); err != nil {
			return nil, err
		}
		return result, nil
	}
	start := strings.Index(text, "{")
	if start < 0 {
		return nil, errors.New("bridge stdout did not contain JSON")
	}
	decoder := json.NewDecoder(strings.NewReader(text[start:]))
	var result map[string]any
	if err := decoder.Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

func seedanceBridgeTimeoutSeconds(timeoutMS int) int {
	if timeoutMS < 600000 {
		return 900
	}
	return timeoutMS / 1000
}

func seedanceBridgeAPIKey(apiKey string) string {
	fields := strings.Fields(strings.TrimSpace(apiKey))
	value := strings.TrimSpace(apiKey)
	if len(fields) > 0 {
		value = strings.TrimSpace(fields[0])
	}
	if strings.HasPrefix(value, "vs-") && len(value) >= 43 {
		return value[:43]
	}
	return value
}

func defaultSeedanceBridgeScript() string {
	candidates := []string{
		"/app/seedance_bridge.py",
		filepath.Join("internal", "provider", "video", "seedance_bridge.py"),
		filepath.Join("backend-go", "internal", "provider", "video", "seedance_bridge.py"),
	}
	for _, item := range candidates {
		if _, err := os.Stat(item); err == nil {
			return item
		}
	}
	return candidates[0]
}

func redactBridgeSecret(value string, apiKey string) string {
	result := value
	for _, secret := range []string{strings.TrimSpace(apiKey), seedanceBridgeAPIKey(apiKey)} {
		if secret == "" {
			continue
		}
		result = strings.ReplaceAll(result, secret, "[REDACTED]")
	}
	return strings.TrimSpace(result)
}

func videoProviderEndpoint(baseURL string, configuredPath string) string {
	return videoProviderEndpointForModel(baseURL, configuredPath, "")
}

func videoProviderEndpointForModel(baseURL string, configuredPath string, model string) string {
	base := strings.TrimRight(baseURL, "/")
	path := strings.TrimSpace(configuredPath)
	defaultRoute := "video/generations"
	if isDoubaoSeedance2Model(model) {
		defaultRoute = "videos/generations"
	}
	if path == "" {
		if baseURLHasAPIVersion(base) {
			return base + "/" + defaultRoute
		}
		return base + "/v1/" + defaultRoute
	}
	if parsed, err := url.Parse(path); err == nil && parsed.IsAbs() {
		return path
	}
	if strings.HasPrefix(path, "/") {
		parsed, err := url.Parse(base)
		if err == nil && parsed.Scheme != "" && parsed.Host != "" {
			return parsed.Scheme + "://" + parsed.Host + path
		}
		return base + path
	}
	return base + "/" + strings.TrimLeft(path, "/")
}

func baseURLHasAPIVersion(baseURL string) bool {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	path := strings.Trim(strings.TrimSpace(parsed.Path), "/")
	if path == "" {
		return false
	}
	parts := strings.Split(path, "/")
	last := strings.ToLower(parts[len(parts)-1])
	if len(last) < 2 || last[0] != 'v' {
		return false
	}
	_, err = strconv.Atoi(last[1:])
	return err == nil
}

func (p OpenAICompatible) pollVideoResult(ctx context.Context, initial map[string]any, model string) map[string]any {
	taskID := firstStringByKeys(initial, "id", "taskId", "task_id", "providerTaskId")
	if taskID == "" {
		return initial
	}
	status := normalizeVideoStatus(firstStringByKeys(initial, "status", "state"))
	if firstStringByKeys(initial, "result_url", "videoUrl", "video_url", "url", "output_url") != "" || status == "FAILED" {
		return initial
	}
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return initial
		case <-time.After(5 * time.Second):
		}
		polled, err := p.getVideoTask(ctx, taskID, model)
		if err != nil {
			continue
		}
		initial = mergeMaps(initial, polled)
		status = normalizeVideoStatus(firstStringByKeys(initial, "status", "state"))
		if firstStringByKeys(initial, "result_url", "videoUrl", "video_url", "url", "output_url") != "" || status == "FAILED" {
			return initial
		}
	}
	return initial
}

func (p OpenAICompatible) getVideoTask(ctx context.Context, taskID string, model string) (map[string]any, error) {
	endpoints := []string{videoTaskEndpointForModel(p.baseURL, p.endpoint, taskID, model)}
	if isDoubaoSeedance2Model(model) && !useSeedanceContentTaskProtocol(p.endpoint) {
		endpoints = append([]string{videoUnifiedTaskEndpoint(p.baseURL, taskID)}, endpoints...)
	}
	var lastErr error
	for _, endpoint := range uniqueStrings(endpoints...) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
		res, err := p.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		raw, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
		_ = res.Body.Close()
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			lastErr = fmt.Errorf("poll video task returned HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(raw)))
			continue
		}
		var decoded map[string]any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			lastErr = err
			continue
		}
		return decoded, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("poll video task failed")
}

func videoTaskEndpoint(baseURL string, taskID string) string {
	return videoTaskEndpointForModel(baseURL, "", taskID, "")
}

func videoTaskEndpointForModel(baseURL string, configuredPath string, taskID string, model string) string {
	return strings.TrimRight(videoProviderEndpointForModel(baseURL, configuredPath, model), "/") + "/" + url.PathEscape(taskID)
}

func videoUnifiedTaskEndpoint(baseURL string, taskID string) string {
	base := strings.TrimRight(baseURL, "/")
	if baseURLHasAPIVersion(base) {
		return base + "/tasks/" + url.PathEscape(taskID) + "?language=zh"
	}
	return base + "/v1/tasks/" + url.PathEscape(taskID) + "?language=zh"
}

func videoContentEndpoint(baseURL string, taskID string) string {
	return videoContentEndpointForModel(baseURL, "", taskID, "")
}

func videoContentEndpointForModel(baseURL string, configuredPath string, taskID string, model string) string {
	if useSeedanceContentTaskProtocol(configuredPath) {
		if parsed, err := url.Parse(strings.TrimRight(baseURL, "/")); err == nil && parsed.Scheme != "" && parsed.Host != "" {
			return parsed.Scheme + "://" + parsed.Host + "/v1/videos/" + url.PathEscape(taskID) + "/content"
		}
	}
	return strings.TrimRight(videoProviderEndpointForModel(baseURL, configuredPath, model), "/") + "/" + url.PathEscape(taskID) + "/content"
}

func mergeMaps(base map[string]any, next map[string]any) map[string]any {
	merged := map[string]any{}
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range next {
		merged[key] = value
	}
	return merged
}

func videoSeconds(params map[string]any) int {
	text := strings.TrimSpace(fmt.Sprint(params["duration"]))
	if text == "" || text == "<nil>" {
		return 4
	}
	value, err := strconv.Atoi(text)
	if err != nil || value <= 0 {
		return 4
	}
	return value
}

func videoAspectRatio(params map[string]any) string {
	ratio := strings.TrimSpace(fmt.Sprint(params["ratio"]))
	if ratio == "" || ratio == "<nil>" {
		return "16:9"
	}
	return ratio
}

func videoResolution(params map[string]any) string {
	resolution := strings.ToLower(strings.TrimSpace(fmt.Sprint(params["resolution"])))
	switch resolution {
	case "4k", "2160p", "2160":
		return "4k"
	case "1080p", "1080":
		return "1080p"
	case "720p", "720":
		return "720p"
	case "480p", "480":
		return "480p"
	default:
		return "480p"
	}
}

func videoGenerateAudio(params map[string]any) bool {
	for _, key := range []string{"generate_audio", "generateAudio"} {
		value, ok := params[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case bool:
			return typed
		case string:
			normalized := strings.ToLower(strings.TrimSpace(typed))
			return normalized == "true" || normalized == "1" || normalized == "yes" || normalized == "on"
		case float64:
			return typed != 0
		case int:
			return typed != 0
		}
	}
	return false
}

func referenceImageURLs(params map[string]any) []string {
	result := []string{}
	for _, key := range []string{"first_frame", "last_frame", "imageUrl", "image_url", "inputImageUrl", "input_image_url"} {
		if value := strings.TrimSpace(fmt.Sprint(params[key])); value != "" && value != "<nil>" {
			result = appendIfMissing(result, value)
		}
	}
	for _, key := range []string{"image_urls", "referenceImages", "reference_images", "inputImages", "inputImagesSnapshot"} {
		for _, item := range imageListValues(params[key]) {
			if value := imageURLFromValue(item); value != "" {
				result = appendIfMissing(result, value)
			}
		}
	}
	if inputReference, ok := params["input_reference"].(map[string]any); ok {
		if value := imageURLFromValue(inputReference); value != "" {
			result = appendIfMissing(result, value)
		}
	}
	return result
}

func videoFrameURLs(params map[string]any, imageURLs []string) (string, string) {
	firstFrame := imageURLFromValue(params["first_frame"])
	lastFrame := imageURLFromValue(params["last_frame"])
	if firstFrame == "" && len(imageURLs) > 0 {
		firstFrame = imageURLs[0]
	}
	if lastFrame == "" && len(imageURLs) > 1 {
		lastFrame = imageURLs[1]
	}
	return firstFrame, lastFrame
}

func imageListValues(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	case []string:
		result := make([]any, 0, len(typed))
		for _, item := range typed {
			result = append(result, item)
		}
		return result
	default:
		return nil
	}
}

func imageURLFromValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		for _, key := range []string{"url", "imageUrl", "image_url", "src"} {
			if result := strings.TrimSpace(fmt.Sprint(typed[key])); result != "" && result != "<nil>" {
				return result
			}
		}
	}
	return ""
}

func firstStringByKeys(value any, keys ...string) string {
	keySet := map[string]bool{}
	for _, key := range keys {
		keySet[strings.ToLower(key)] = true
	}
	var visit func(any) string
	visit = func(current any) string {
		switch typed := current.(type) {
		case map[string]any:
			for key, item := range typed {
				if keySet[strings.ToLower(key)] {
					if text := firstTextValue(item); text != "" {
						return text
					}
				}
			}
			for _, item := range typed {
				if text := visit(item); text != "" {
					return text
				}
			}
		case []any:
			for _, item := range typed {
				if text := visit(item); text != "" {
					return text
				}
			}
		}
		return ""
	}
	return visit(value)
}

func firstTextValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []string:
		for _, item := range typed {
			if text := strings.TrimSpace(item); text != "" {
				return text
			}
		}
	case []any:
		for _, item := range typed {
			if text := firstTextValue(item); text != "" {
				return text
			}
		}
	case map[string]any:
		for _, key := range []string{"url", "videoUrl", "video_url", "output_url", "result_url"} {
			if text := firstTextValue(typed[key]); text != "" {
				return text
			}
		}
	case nil:
		return ""
	default:
		text := strings.TrimSpace(fmt.Sprint(value))
		if text != "" && text != "<nil>" {
			return text
		}
	}
	return ""
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func containsString(items []string, value string) bool {
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), value) {
			return true
		}
	}
	return false
}

func uniqueStrings(items ...string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		result = append(result, item)
	}
	return result
}

func appendIfMissing(items []string, item string) []string {
	if containsString(items, item) {
		return items
	}
	return append(items, item)
}
