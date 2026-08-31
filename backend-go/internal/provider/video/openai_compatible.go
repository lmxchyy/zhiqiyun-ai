package video

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
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

// Get queries an existing provider task without creating a new task. It is
// intentionally separate from Create so recovery callers can enforce Get-only
// semantics after a process crash.
func (p OpenAICompatible) Get(ctx context.Context, providerTaskID string) (any, error) {
	if strings.TrimSpace(providerTaskID) == "" {
		return nil, errors.New("provider task id is required")
	}
	model := p.model
	endpoint := videoProviderEndpointForModel(p.baseURL, p.endpoint, model)
	u := strings.TrimRight(endpoint, "/") + "/" + url.PathEscape(providerTaskID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	res, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 12<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("video provider %s query HTTP %d", p.providerCode(), res.StatusCode)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, err
	}
	status := normalizeVideoStatus(firstNonEmptyString(firstStringByKeys(decoded, "status", "state"), "UNKNOWN"))
	return map[string]any{"provider": p.providerCode(), "providerTaskId": providerTaskID, "status": status, "videoUrl": extractPlayableVideoURL(decoded), "thumbnailUrl": firstStringByKeys(decoded, "thumbnailUrl", "thumbnail_url", "coverUrl", "cover_url", "poster")}, nil
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
	if err := validateVideoProviderParameters(req.Params, OpenAICompatibleSupportedParameters(OpenAICompatibleOptions{
		Code: p.code, BaseURL: p.baseURL, Model: p.model, Models: p.models, Endpoint: p.endpoint,
	}, model)); err != nil {
		return nil, err
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
	if isGrokImagine15VideoModel(model) && len(imageURLs) > 7 {
		return nil, errors.New("Grok Imagine Video 1.5 supports at most seven reference images")
	}
	if p.shouldUseSeedanceBridge(model) {
		return p.createWithSeedanceBridge(ctx, model, req)
	}
	if isGrokImagine15VideoModel(model) && len(imageURLs) > 0 && hasDataImageURL(imageURLs) {
		return p.createGrokImagineMultipart(ctx, model, req, imageURLs)
	}
	body := videoRequestBodyForEndpoint(model, req, p.endpoint, imageURLs)
	if len(imageURLs) > 0 && !useSeedanceContentTaskProtocol(p.endpoint) {
		if isGrokImagine15VideoModel(model) {
			// Grok Imagine / NewAPI reject data: URLs and treat `images` like Sora file ids.
			// Keep only public http(s) image_urls for the JSON contract.
			body["image_urls"] = imageURLs
		} else {
			body["images"] = imageURLs
			body["image_urls"] = imageURLs
			if len(imageURLs) == 1 {
				// NewAPI / OpenAI-compatible video gateways decode input_reference as a
				// plain URL/file-id string. Sending {"image_url":"..."} triggers:
				// cannot unmarshal object into Go struct field .Alias.input_reference of type string
				body["input_reference"] = imageURLs[0]
				body["image"] = imageURLs[0]
			}
		}
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
	return p.finishVideoCreate(ctx, httpReq, model, req)
}

func (p OpenAICompatible) createGrokImagineMultipart(ctx context.Context, model string, req generation.CreateRequest, imageURLs []string) (any, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fields := map[string]string{
		"model":        model,
		"prompt":       req.Prompt,
		"duration":     strconv.Itoa(videoSeconds(req.Params)),
		"size":         grokImagineVideoSize(req.Params),
		"quality":      videoResolution(req.Params),
		"aspect_ratio": videoAspectRatio(req.Params),
		"resolution":   videoResolution(req.Params),
	}
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return nil, err
		}
	}
	for index, imageURL := range imageURLs {
		raw, contentType, extension, err := decodeVideoReferenceImage(imageURL)
		if err != nil {
			return nil, fmt.Errorf("decode grok reference image %d: %w", index+1, err)
		}
		fieldName := "images"
		if len(imageURLs) == 1 {
			fieldName = "input_reference"
		}
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldName, fmt.Sprintf("reference-%d.%s", index+1, extension)))
		header.Set("Content-Type", contentType)
		part, err := writer.CreatePart(header)
		if err != nil {
			return nil, err
		}
		if _, err := part.Write(raw); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, videoProviderEndpointForModel(p.baseURL, p.endpoint, model), &body)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	return p.finishVideoCreate(ctx, httpReq, model, req)
}

func (p OpenAICompatible) finishVideoCreate(ctx context.Context, httpReq *http.Request, model string, req generation.CreateRequest) (any, error) {
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
	videoURL := extractPlayableVideoURL(decoded)
	thumbnailURL := firstStringByKeys(decoded, "thumbnailUrl", "thumbnail_url", "coverUrl", "cover_url", "poster", "image_url")
	status := normalizeVideoStatus(firstNonEmptyString(firstStringByKeys(decoded, "status", "state"), "SUCCEEDED"))
	if status == "FAILED" || videoURL == "" {
		reason := firstNonEmptyString(
			firstStringByKeys(decoded, "fail_reason", "error", "message"),
			firstStringByKeys(decoded, "url"),
		)
		if status == "FAILED" || looksLikeVideoProviderErrorText(reason) {
			if reason == "" {
				reason = "video generation failed"
			}
			return nil, errors.New(reason)
		}
	}
	if videoURL == "" && strings.Contains(status, "PROCESS") {
		status = "PROCESSING"
	}
	taskID := firstNonEmptyString(firstStringByKeys(decoded, "id", "taskId", "task_id", "providerTaskId"), "video-"+strconv.FormatInt(time.Now().UnixNano(), 10))
	if videoURL == "" && strings.EqualFold(status, "SUCCEEDED") && taskID != "" {
		candidate := videoContentEndpointForModel(p.baseURL, p.endpoint, taskID, model)
		if isPlayableVideoURL(candidate) {
			videoURL = candidate
		}
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
			"duration":     req.Params["duration"],
			"aspect_ratio": videoAspectRatio(req.Params),
			"resolution":   req.Params["resolution"],
		},
	}, nil
}

func videoRequestBody(model string, req generation.CreateRequest) map[string]any {
	if isGrokImagine15VideoModel(model) {
		ratio := videoAspectRatio(req.Params)
		resolution := videoResolution(req.Params)
		return map[string]any{
			"model":        model,
			"prompt":       req.Prompt,
			"duration":     videoSeconds(req.Params),
			"size":         grokImagineVideoSize(req.Params),
			"quality":      resolution,
			"aspect_ratio": ratio,
			"resolution":   resolution,
		}
	}
	if isDoubaoSeedance2Model(model) {
		seconds := videoSeconds(req.Params)
		ratio := videoAspectRatio(req.Params)
		resolution := videoResolution(req.Params)
		return map[string]any{
			"model":      model,
			"prompt":     req.Prompt,
			"duration":   seconds,
			"seconds":    strconv.Itoa(seconds),
			"size":       ratio,
			"resolution": resolution,
			// NewAPI Doubao adaptor reads ratio/resolution from metadata and duration from seconds.
			"metadata": map[string]any{
				"ratio":      ratio,
				"resolution": resolution,
				"watermark":  false,
				"duration":   seconds,
			},
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

func isGrokImagine15VideoModel(model string) bool {
	normalized := strings.ToLower(strings.TrimSpace(model))
	normalized = strings.ReplaceAll(normalized, "_", "-")
	switch normalized {
	case "grok-imagine-1.5-video", "grok-imagine-video-1.5-preview":
		return true
	default:
		return false
	}
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

var videoCoreParameterKeys = []string{"duration", "resolution", "aspect_ratio"}
var videoOptionalProviderParameterKeys = []string{"fps", "generate_audio", "motion_strength", "camera_movement"}

// OpenAICompatibleSupportedParameters returns only parameters that this adapter
// actually forwards for the selected upstream protocol.
func OpenAICompatibleSupportedParameters(opts OpenAICompatibleOptions, model string) []string {
	supported := append([]string(nil), videoCoreParameterKeys...)
	if isDoubaoSeedance2Model(model) && useSeedanceContentTaskProtocol(opts.Endpoint) {
		supported = append(supported, "generate_audio")
	}
	return supported
}

func containsVideoParameter(parameters []string, key string) bool {
	for _, parameter := range parameters {
		if strings.EqualFold(strings.TrimSpace(parameter), strings.TrimSpace(key)) {
			return true
		}
	}
	return false
}

func validateVideoProviderParameters(params map[string]any, supported []string) error {
	if params == nil {
		return nil
	}
	for _, key := range videoOptionalProviderParameterKeys {
		if _, exists := params[key]; exists && !containsVideoParameter(supported, key) {
			delete(params, key)
		}
	}
	if _, exists := params["generateAudio"]; exists && !containsVideoParameter(supported, "generate_audio") {
		delete(params, "generateAudio")
	}
	return nil
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
	if !isPlayableVideoURL(videoURL) {
		reason := firstNonEmptyString(firstStringByKeys(result, "error", "message", "errorMessage"), videoURL, "CMECloud Seedance bridge did not return a playable video URL")
		return nil, errors.New(reason)
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
			"duration":     req.Params["duration"],
			"aspect_ratio": videoAspectRatio(req.Params),
			"resolution":   req.Params["resolution"],
			"actualModel":  actualModel,
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
	// Grok Imagine uses OpenAI Videos API (/v1/videos). Ignore channel defaults that
	// point Seedance/Sora traffic at /v1/video/generations.
	if isGrokImagine15VideoModel(model) {
		path = ""
	}
	defaultRoute := "video/generations"
	if isGrokImagine15VideoModel(model) {
		defaultRoute = "videos"
	} else if isDoubaoSeedance2Model(model) {
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
	if extractPlayableVideoURL(initial) != "" || status == "FAILED" || looksLikeVideoProviderErrorText(firstStringByKeys(initial, "url", "fail_reason", "error", "message")) {
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
		if extractPlayableVideoURL(initial) != "" || status == "FAILED" || looksLikeVideoProviderErrorText(firstStringByKeys(initial, "url", "fail_reason", "error", "message")) {
			return initial
		}
	}
	return initial
}

func extractPlayableVideoURL(decoded map[string]any) string {
	if decoded == nil {
		return ""
	}
	if content, ok := decoded["content"].(map[string]any); ok {
		if url := firstTextValue(content["video_url"]); isPlayableVideoURL(url) {
			return url
		}
		if url := firstTextValue(content["url"]); isPlayableVideoURL(url) {
			return url
		}
	}
	if metadata, ok := decoded["metadata"].(map[string]any); ok {
		if url := firstTextValue(metadata["url"]); isPlayableVideoURL(url) {
			return url
		}
		if url := firstTextValue(metadata["video_url"]); isPlayableVideoURL(url) {
			return url
		}
	}
	if data, ok := decoded["data"].(map[string]any); ok {
		if url := extractPlayableVideoURL(data); url != "" {
			return url
		}
	}
	for _, key := range []string{"videoUrl", "video_url", "output_url", "result_url", "url"} {
		if url := firstStringByKeys(decoded, key); isPlayableVideoURL(url) {
			return url
		}
	}
	return ""
}

func isPlayableVideoURL(value string) bool {
	text := strings.TrimSpace(value)
	if text == "" {
		return false
	}
	lower := strings.ToLower(text)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return true
	}
	return strings.HasPrefix(lower, "/api/v1/generated-media/")
}

func hasDataImageURL(imageURLs []string) bool {
	for _, imageURL := range imageURLs {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(imageURL)), "data:image/") {
			return true
		}
	}
	return false
}

func decodeVideoReferenceImage(value string) ([]byte, string, string, error) {
	text := strings.TrimSpace(value)
	if text == "" {
		return nil, "", "", errors.New("empty reference image")
	}
	if strings.HasPrefix(strings.ToLower(text), "data:") {
		comma := strings.IndexByte(text, ',')
		if comma <= len("data:") || !strings.Contains(strings.ToLower(text[:comma]), ";base64") {
			return nil, "", "", errors.New("reference data URL must be base64 encoded")
		}
		raw, err := base64.StdEncoding.DecodeString(text[comma+1:])
		if err != nil {
			return nil, "", "", fmt.Errorf("decode reference data URL: %w", err)
		}
		if len(raw) == 0 {
			return nil, "", "", errors.New("reference data URL is empty")
		}
		declared := strings.TrimSpace(strings.Split(strings.TrimPrefix(text[:comma], "data:"), ";")[0])
		contentType := strings.ToLower(declared)
		if contentType == "" {
			contentType = http.DetectContentType(raw)
		}
		if parsed, _, err := mime.ParseMediaType(contentType); err == nil {
			contentType = parsed
		}
		extension := "jpg"
		switch contentType {
		case "image/png":
			extension = "png"
		case "image/webp":
			extension = "webp"
		case "image/gif":
			extension = "gif"
		case "image/jpeg", "image/jpg":
			extension = "jpg"
			contentType = "image/jpeg"
		default:
			if !strings.HasPrefix(contentType, "image/") {
				return nil, "", "", fmt.Errorf("unsupported reference image type %q", contentType)
			}
		}
		return raw, contentType, extension, nil
	}
	if !strings.HasPrefix(strings.ToLower(text), "http://") && !strings.HasPrefix(strings.ToLower(text), "https://") {
		return nil, "", "", fmt.Errorf("unsupported reference image URL %q", text)
	}
	res, err := http.Get(text)
	if err != nil {
		return nil, "", "", err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, "", "", fmt.Errorf("download reference image returned HTTP %d", res.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 12<<20))
	if err != nil {
		return nil, "", "", err
	}
	contentType := strings.ToLower(strings.TrimSpace(res.Header.Get("Content-Type")))
	if parsed, _, err := mime.ParseMediaType(contentType); err == nil {
		contentType = parsed
	}
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = http.DetectContentType(raw)
	}
	extension := "jpg"
	switch contentType {
	case "image/png":
		extension = "png"
	case "image/webp":
		extension = "webp"
	case "image/gif":
		extension = "gif"
	default:
		contentType = "image/jpeg"
		extension = "jpg"
	}
	return raw, contentType, extension, nil
}

func looksLikeVideoProviderErrorText(value string) bool {
	text := strings.TrimSpace(value)
	if text == "" || isPlayableVideoURL(text) {
		return false
	}
	lower := strings.ToLower(text)
	markers := []string{
		"unrecognized message",
		"upstream returned",
		"failed",
		"error",
		"invalid",
		"denied",
		"timeout",
		"not found",
		"无法",
		"失败",
		"错误",
	}
	for _, marker := range markers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
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
	for _, key := range []string{"aspect_ratio", "ratio"} {
		ratio := strings.TrimSpace(fmt.Sprint(params[key]))
		if ratio != "" && ratio != "<nil>" {
			return ratio
		}
	}
	return "16:9"
}

// grokImagineVideoSize maps ratio+resolution to OpenAI-compatible WxH size.
// NewAPI /v1/videos treats `size` as pixels (e.g. 1280x720). Sending "16:9"
// is ignored, so image-to-video falls back to the reference image orientation.
func grokImagineVideoSize(params map[string]any) string {
	return grokImagineVideoSizeFor(videoAspectRatio(params), videoResolution(params))
}

func grokImagineVideoSizeFor(aspectRatio, resolution string) string {
	ratio := strings.TrimSpace(aspectRatio)
	switch strings.ToLower(strings.TrimSpace(resolution)) {
	case "1080p", "1080", "4k", "2160p", "2160":
		switch ratio {
		case "9:16":
			return "1080x1920"
		case "1:1":
			return "1080x1080"
		case "3:2":
			return "1620x1080"
		case "2:3":
			return "1080x1620"
		default:
			return "1920x1080"
		}
	case "480p", "480":
		switch ratio {
		case "9:16":
			return "480x854"
		case "1:1":
			return "544x544"
		case "3:2":
			return "720x480"
		case "2:3":
			return "480x720"
		default:
			return "854x480"
		}
	default: // 720p
		switch ratio {
		case "9:16":
			return "720x1280"
		case "1:1":
			return "960x960"
		case "3:2":
			return "1080x720"
		case "2:3":
			return "720x1080"
		default:
			return "1280x720"
		}
	}
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
