package httpserver

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"xianzhi-ai/backend-go/internal/config"
)

var wirelessCanvasTasks sync.Map

type wirelessCanvasDocument struct {
	ID          string                 `json:"id"`
	Title       string                 `json:"title"`
	Icon        string                 `json:"icon"`
	Project     string                 `json:"project"`
	Kind        string                 `json:"kind"`
	BoardX      int                    `json:"board_x"`
	BoardY      int                    `json:"board_y"`
	Nodes       []map[string]any       `json:"nodes"`
	Connections []map[string]any       `json:"connections"`
	Viewport    map[string]any         `json:"viewport"`
	Logs        []map[string]any       `json:"logs"`
	Settings    map[string]any         `json:"settings"`
	UpdatedAt   int64                  `json:"updated_at"`
	DeletedAt   int64                  `json:"deleted_at,omitempty"`
	Extra       map[string]interface{} `json:"-"`
}

type wirelessCanvasProject struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Order       int    `json:"order"`
	CanvasCount int    `json:"canvas_count"`
}

func registerWirelessCanvasCompatibilityRoutes(router *gin.Engine, cfg config.Config) {
	api := wirelessCanvasCompatibilityAPI{dataDir: filepath.Join(wirelessCanvasDataRoot(cfg.DataPath), "wireless-canvas")}
	router.GET("/api/config", api.config)
	router.GET("/api/workflows", api.emptyWorkflows)
	router.GET("/api/prompt-libraries", api.promptLibraries)
	router.GET("/api/smart-canvas/prompt-templates", api.promptTemplates)
	router.GET("/api/asset-library", api.assetLibrary)
	router.GET("/api/local-assets", api.localAssets)
	router.GET("/api/projects", api.listProjects)
	router.POST("/api/projects", api.createProject)
	router.POST("/api/projects/:id", api.renameProject)
	router.DELETE("/api/projects/:id", api.deleteProject)
	router.GET("/api/canvases", api.listCanvases)
	router.POST("/api/canvases", api.createCanvas)
	router.GET("/api/canvases/:id", api.getCanvas)
	router.PUT("/api/canvases/:id", api.putCanvas)
	router.POST("/api/canvases/:id/meta", api.updateCanvasMeta)
	router.GET("/api/canvases/:id/meta", api.canvasMeta)
	router.DELETE("/api/canvases/:id", api.deleteCanvas)
	router.POST("/api/canvases/:id/restore", api.restoreCanvas)
	router.DELETE("/api/canvases/:id/purge", api.purgeCanvas)
	router.POST("/api/canvas-image-tasks", api.createImageTask)
	router.GET("/api/canvas-image-tasks/:id", api.getImageTask)
}

func wirelessCanvasDataRoot(dataPath string) string {
	if strings.TrimSpace(dataPath) == "" {
		return "data"
	}
	if filepath.Ext(dataPath) != "" {
		return filepath.Dir(dataPath)
	}
	return dataPath
}

type wirelessCanvasCompatibilityAPI struct {
	dataDir string
}

func (api wirelessCanvasCompatibilityAPI) config(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"api_providers": []gin.H{
			{
				"id":     "xianzhi-api",
				"name":   "先知 AI",
				"models": []string{"gpt-image-2", "flux-kontext-pro"},
			},
		},
		"providers": []gin.H{
			{
				"id":     "xianzhi-api",
				"name":   "先知 AI",
				"models": []string{"gpt-image-2", "flux-kontext-pro"},
			},
		},
		"runninghub": gin.H{"workflows": []any{}, "webapps": []any{}},
		"comfyui":    gin.H{"workflows": []any{}},
	})
}

func (api wirelessCanvasCompatibilityAPI) emptyWorkflows(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"workflows": []any{}})
}

func (api wirelessCanvasCompatibilityAPI) promptLibraries(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"library": gin.H{
			"libraries": []gin.H{
				{"id": "system", "name": "系统提示词库", "categories": []any{}, "items": []any{}},
			},
		},
	})
}

func (api wirelessCanvasCompatibilityAPI) promptTemplates(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"templates": []gin.H{
		{"id": "xianzhi-product", "title": "商业产品图", "category": "商业", "prompt": "商业产品摄影，主体清晰，高级光影，干净背景，细节完整"},
		{"id": "xianzhi-poster", "title": "海报视觉", "category": "设计", "prompt": "高端海报构图，强视觉中心，留白合理，精致排版"},
	}})
}

func (api wirelessCanvasCompatibilityAPI) assetLibrary(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"categories": []gin.H{
			{"id": "default", "name": "默认素材", "type": "image", "items": []any{}},
		},
		"items": []any{},
	})
}

func (api wirelessCanvasCompatibilityAPI) localAssets(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"items": []any{}, "tree": nil})
}

func (api wirelessCanvasCompatibilityAPI) listProjects(c *gin.Context) {
	projects, err := api.readProjects()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	counts, err := api.canvasCountsByProject()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for i := range projects {
		projects[i].CanvasCount = counts[projects[i].ID]
		delete(counts, projects[i].ID)
	}
	for projectID, count := range counts {
		if strings.TrimSpace(projectID) == "" {
			continue
		}
		projects = append(projects, wirelessCanvasProject{
			ID:          projectID,
			Name:        wirelessCanvasProjectName(projectID),
			Order:       len(projects),
			CanvasCount: count,
		})
	}
	c.JSON(http.StatusOK, gin.H{"projects": projects})
}

func (api wirelessCanvasCompatibilityAPI) createProject(c *gin.Context) {
	var payload struct {
		Name string `json:"name"`
	}
	_ = c.ShouldBindJSON(&payload)
	projects, err := api.readProjects()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	now := time.Now().UnixMilli()
	name := strings.TrimSpace(payload.Name)
	if name == "" {
		name = "新项目"
	}
	project := wirelessCanvasProject{
		ID:    "project_" + randomHash(fmt.Sprintf("%s-%d", name, now))[:12],
		Name:  name,
		Order: len(projects),
	}
	projects = append(projects, project)
	if err := api.writeProjects(projects); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"project": project})
}

func (api wirelessCanvasCompatibilityAPI) renameProject(c *gin.Context) {
	id := sanitizeWirelessCanvasID(c.Param("id"))
	var payload struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	projects, err := api.readProjects()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for i := range projects {
		if projects[i].ID == id {
			if name := strings.TrimSpace(payload.Name); name != "" {
				projects[i].Name = name
			}
			if err := api.writeProjects(projects); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"project": projects[i]})
			return
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
}

func (api wirelessCanvasCompatibilityAPI) deleteProject(c *gin.Context) {
	id := sanitizeWirelessCanvasID(c.Param("id"))
	if id == "default" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "default project cannot be deleted"})
		return
	}
	projects, err := api.readProjects()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	next := projects[:0]
	found := false
	for _, project := range projects {
		if project.ID == id {
			found = true
			continue
		}
		next = append(next, project)
	}
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}
	if err := api.writeProjects(next); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	canvases, err := api.readAllCanvases(true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for _, canvas := range canvases {
		if canvas.Project == id {
			canvas.Project = "default"
			canvas.UpdatedAt = time.Now().UnixMilli()
			_ = api.writeCanvas(canvas)
		}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (api wirelessCanvasCompatibilityAPI) listCanvases(c *gin.Context) {
	canvases, err := api.readAllCanvases(false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"canvases": canvases})
}

func (api wirelessCanvasCompatibilityAPI) createCanvas(c *gin.Context) {
	var payload struct {
		Title   string `json:"title"`
		Icon    string `json:"icon"`
		Kind    string `json:"kind"`
		Project string `json:"project"`
		BoardX  int    `json:"board_x"`
		BoardY  int    `json:"board_y"`
	}
	_ = c.ShouldBindJSON(&payload)
	title := strings.TrimSpace(payload.Title)
	if title == "" {
		title = "智能画布"
	}
	kind := strings.TrimSpace(payload.Kind)
	if kind == "" {
		kind = "smart"
	}
	id := "canvas_" + randomHash(fmt.Sprintf("%s-%s-%d", title, kind, time.Now().UnixNano()))[:14]
	canvas := defaultWirelessCanvas(id)
	canvas.Title = title
	canvas.Icon = strings.TrimSpace(payload.Icon)
	if canvas.Icon == "" {
		canvas.Icon = "sparkles"
	}
	canvas.Kind = kind
	canvas.Project = strings.TrimSpace(payload.Project)
	if canvas.Project == "" {
		canvas.Project = "default"
	}
	canvas.BoardX = payload.BoardX
	canvas.BoardY = payload.BoardY
	if kind != "smart" {
		canvas.Icon = "🧩"
		canvas.Nodes = []map[string]any{}
		canvas.Connections = []map[string]any{}
		canvas.Settings = map[string]any{}
	}
	if err := api.writeCanvas(canvas); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"canvas": canvas})
}

func (api wirelessCanvasCompatibilityAPI) getCanvas(c *gin.Context) {
	if c.Param("id") == "trash" {
		api.listTrash(c)
		return
	}
	canvas, err := api.readCanvas(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"canvas": canvas})
}

func (api wirelessCanvasCompatibilityAPI) putCanvas(c *gin.Context) {
	canvas, err := api.readCanvas(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var payload struct {
		Title       string           `json:"title"`
		Icon        string           `json:"icon"`
		Nodes       []map[string]any `json:"nodes"`
		Connections []map[string]any `json:"connections"`
		Viewport    map[string]any   `json:"viewport"`
		Logs        []map[string]any `json:"logs"`
		Settings    map[string]any   `json:"settings"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(payload.Title) != "" {
		canvas.Title = payload.Title
	}
	if strings.TrimSpace(payload.Icon) != "" {
		canvas.Icon = payload.Icon
	}
	canvas.Nodes = payload.Nodes
	canvas.Connections = payload.Connections
	canvas.Viewport = payload.Viewport
	canvas.Logs = payload.Logs
	canvas.Settings = payload.Settings
	canvas.UpdatedAt = time.Now().UnixMilli()
	if err := api.writeCanvas(canvas); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"canvas": canvas})
}

func (api wirelessCanvasCompatibilityAPI) canvasMeta(c *gin.Context) {
	canvas, err := api.readCanvas(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": canvas.ID, "updated_at": canvas.UpdatedAt, "title": canvas.Title})
}

func (api wirelessCanvasCompatibilityAPI) updateCanvasMeta(c *gin.Context) {
	canvas, err := api.readCanvas(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var patch struct {
		Title   *string `json:"title"`
		Icon    *string `json:"icon"`
		Project *string `json:"project"`
		BoardX  *int    `json:"board_x"`
		BoardY  *int    `json:"board_y"`
		Kind    *string `json:"kind"`
	}
	if err := c.ShouldBindJSON(&patch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if patch.Title != nil && strings.TrimSpace(*patch.Title) != "" {
		canvas.Title = strings.TrimSpace(*patch.Title)
	}
	if patch.Icon != nil {
		canvas.Icon = strings.TrimSpace(*patch.Icon)
	}
	if patch.Project != nil && strings.TrimSpace(*patch.Project) != "" {
		canvas.Project = strings.TrimSpace(*patch.Project)
	}
	if patch.BoardX != nil {
		canvas.BoardX = *patch.BoardX
	}
	if patch.BoardY != nil {
		canvas.BoardY = *patch.BoardY
	}
	if patch.Kind != nil && strings.TrimSpace(*patch.Kind) != "" {
		canvas.Kind = strings.TrimSpace(*patch.Kind)
	}
	canvas.UpdatedAt = time.Now().UnixMilli()
	if err := api.writeCanvas(canvas); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"canvas": canvas})
}

func (api wirelessCanvasCompatibilityAPI) deleteCanvas(c *gin.Context) {
	canvas, err := api.readCanvas(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	canvas.DeletedAt = time.Now().UnixMilli()
	canvas.UpdatedAt = canvas.DeletedAt
	if err := api.writeCanvas(canvas); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"canvas": canvas})
}

func (api wirelessCanvasCompatibilityAPI) restoreCanvas(c *gin.Context) {
	canvas, err := api.readCanvas(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	canvas.DeletedAt = 0
	canvas.UpdatedAt = time.Now().UnixMilli()
	if err := api.writeCanvas(canvas); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"canvas": canvas})
}

func (api wirelessCanvasCompatibilityAPI) purgeCanvas(c *gin.Context) {
	if err := os.Remove(api.canvasPath(c.Param("id"))); err != nil && !os.IsNotExist(err) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (api wirelessCanvasCompatibilityAPI) listTrash(c *gin.Context) {
	canvases, err := api.readAllCanvases(true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	deleted := make([]wirelessCanvasDocument, 0)
	for _, canvas := range canvases {
		if canvas.DeletedAt > 0 {
			deleted = append(deleted, canvas)
		}
	}
	sort.Slice(deleted, func(i, j int) bool { return deleted[i].DeletedAt > deleted[j].DeletedAt })
	c.JSON(http.StatusOK, gin.H{"canvases": deleted})
}

func (api wirelessCanvasCompatibilityAPI) createImageTask(c *gin.Context) {
	var payload struct {
		Prompt string `json:"prompt"`
		Model  string `json:"model"`
	}
	_ = c.ShouldBindJSON(&payload)
	taskID := "wireless_" + randomHash(payload.Prompt + payload.Model + time.Now().String())[:16]
	result := gin.H{
		"status": "succeeded",
		"result": gin.H{
			"images": []gin.H{
				{
					"url":  "https://picsum.photos/seed/" + taskID + "/1024/1024",
					"name": "无线画布生成结果",
				},
			},
		},
	}
	wirelessCanvasTasks.Store(taskID, result)
	c.JSON(http.StatusOK, gin.H{"task_id": taskID})
}

func (api wirelessCanvasCompatibilityAPI) getImageTask(c *gin.Context) {
	if value, ok := wirelessCanvasTasks.Load(c.Param("id")); ok {
		c.JSON(http.StatusOK, value)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status": "succeeded",
		"result": gin.H{
			"images": []gin.H{
				{"url": "https://picsum.photos/seed/" + c.Param("id") + "/1024/1024", "name": "无线画布生成结果"},
			},
		},
	})
}

func (api wirelessCanvasCompatibilityAPI) readCanvas(id string) (wirelessCanvasDocument, error) {
	id = sanitizeWirelessCanvasID(id)
	path := api.canvasPath(id)
	var canvas wirelessCanvasDocument
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &canvas); err == nil && canvas.ID != "" {
			normalizeWirelessCanvas(&canvas)
			return canvas, nil
		}
	}
	canvas = defaultWirelessCanvas(id)
	if err := api.writeCanvas(canvas); err != nil {
		return canvas, err
	}
	return canvas, nil
}

func (api wirelessCanvasCompatibilityAPI) readAllCanvases(includeDeleted bool) ([]wirelessCanvasDocument, error) {
	if err := os.MkdirAll(api.dataDir, 0o755); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(api.dataDir)
	if err != nil {
		return nil, err
	}
	canvases := make([]wirelessCanvasDocument, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" || entry.Name() == "projects.json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(api.dataDir, entry.Name()))
		if err != nil {
			return nil, err
		}
		var canvas wirelessCanvasDocument
		if err := json.Unmarshal(data, &canvas); err != nil || canvas.ID == "" {
			continue
		}
		normalizeWirelessCanvas(&canvas)
		if !includeDeleted && canvas.DeletedAt > 0 {
			continue
		}
		canvases = append(canvases, canvas)
	}
	sort.Slice(canvases, func(i, j int) bool {
		if canvases[i].UpdatedAt == canvases[j].UpdatedAt {
			return canvases[i].ID < canvases[j].ID
		}
		return canvases[i].UpdatedAt > canvases[j].UpdatedAt
	})
	return canvases, nil
}

func (api wirelessCanvasCompatibilityAPI) canvasCountsByProject() (map[string]int, error) {
	canvases, err := api.readAllCanvases(false)
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, canvas := range canvases {
		project := strings.TrimSpace(canvas.Project)
		if project == "" {
			project = "default"
		}
		counts[project]++
	}
	return counts, nil
}

func (api wirelessCanvasCompatibilityAPI) readProjects() ([]wirelessCanvasProject, error) {
	if err := os.MkdirAll(api.dataDir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(api.dataDir, "projects.json")
	var payload struct {
		Projects []wirelessCanvasProject `json:"projects"`
	}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &payload); err == nil && len(payload.Projects) > 0 {
			return ensureDefaultProject(payload.Projects), nil
		}
	}
	projects := []wirelessCanvasProject{{ID: "default", Name: "默认项目", Order: 0}}
	if err := api.writeProjects(projects); err != nil {
		return nil, err
	}
	return projects, nil
}

func (api wirelessCanvasCompatibilityAPI) writeProjects(projects []wirelessCanvasProject) error {
	if err := os.MkdirAll(api.dataDir, 0o755); err != nil {
		return err
	}
	projects = ensureDefaultProject(projects)
	sort.Slice(projects, func(i, j int) bool { return projects[i].Order < projects[j].Order })
	data, err := json.MarshalIndent(gin.H{"projects": projects}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(api.dataDir, "projects.json"), data, 0o644)
}

func (api wirelessCanvasCompatibilityAPI) writeCanvas(canvas wirelessCanvasDocument) error {
	if err := os.MkdirAll(api.dataDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(canvas, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(api.canvasPath(canvas.ID), data, 0o644)
}

func (api wirelessCanvasCompatibilityAPI) canvasPath(id string) string {
	return filepath.Join(api.dataDir, sanitizeWirelessCanvasID(id)+".json")
}

func defaultWirelessCanvas(id string) wirelessCanvasDocument {
	now := time.Now().UnixMilli()
	return wirelessCanvasDocument{
		ID:      id,
		Title:   "无线画布",
		Icon:    "sparkles",
		Project: "xianzhi",
		Kind:    "smart",
		BoardX:  0,
		BoardY:  0,
		Nodes: []map[string]any{
			{
				"id":     "seed_prompt",
				"type":   "smart-prompt",
				"x":      760,
				"y":      520,
				"width":  310,
				"height": 190,
				"text":   "未来科技城市，全景构图，干净的产品级视觉",
			},
			{
				"id":     "seed_generator",
				"type":   "smart-image",
				"x":      1160,
				"y":      520,
				"width":  260,
				"height": 178,
				"images": []any{},
			},
			{
				"id":     "seed_loop",
				"type":   "smart-loop",
				"x":      880,
				"y":      790,
				"width":  280,
				"height": 170,
				"rounds": 3,
				"mode":   "sequential",
			},
		},
		Connections: []map[string]any{
			{"id": "edge_seed_prompt_generator", "from": "seed_prompt", "to": "seed_generator"},
		},
		Viewport:  map[string]any{"x": -420, "y": -260, "scale": 1},
		Logs:      []map[string]any{},
		Settings:  map[string]any{"engine": "api", "provider_id": "xianzhi-api", "model": "gpt-image-2", "ratio": "wide", "quality": "auto", "count": 1},
		UpdatedAt: now,
	}
}

func sanitizeWirelessCanvasID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return "xianzhi-wireless-canvas"
	}
	replacer := strings.NewReplacer("/", "-", "\\", "-", "..", "-")
	return replacer.Replace(id)
}

func ensureDefaultProject(projects []wirelessCanvasProject) []wirelessCanvasProject {
	hasDefault := false
	for i := range projects {
		if projects[i].ID == "default" {
			hasDefault = true
			if strings.TrimSpace(projects[i].Name) == "" {
				projects[i].Name = "默认项目"
			}
			break
		}
	}
	if !hasDefault {
		projects = append([]wirelessCanvasProject{{ID: "default", Name: "默认项目", Order: 0}}, projects...)
	}
	for i := range projects {
		if projects[i].Order == 0 && projects[i].ID != "default" {
			projects[i].Order = i + 1
		}
	}
	return projects
}

func normalizeWirelessCanvas(canvas *wirelessCanvasDocument) {
	if strings.TrimSpace(canvas.Project) == "" {
		canvas.Project = "default"
	}
	if strings.TrimSpace(canvas.Kind) == "" {
		canvas.Kind = "classic"
		for _, node := range canvas.Nodes {
			if strings.HasPrefix(fmt.Sprint(node["type"]), "smart-") {
				canvas.Kind = "smart"
				break
			}
		}
	}
	if strings.TrimSpace(canvas.Icon) == "" {
		if canvas.Kind == "smart" {
			canvas.Icon = "sparkles"
		} else {
			canvas.Icon = "🧩"
		}
	}
}

func wirelessCanvasProjectName(id string) string {
	if id == "default" {
		return "默认项目"
	}
	if id == "xianzhi" {
		return "先知项目"
	}
	return id
}

func randomHash(value string) string {
	sum := sha1.Sum([]byte(value))
	return hex.EncodeToString(sum[:])
}
