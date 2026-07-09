package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type officeCLIStatusResponse struct {
	Available       bool                      `json:"available"`
	BinaryPath      string                    `json:"binaryPath,omitempty"`
	Version         string                    `json:"version,omitempty"`
	Error           string                    `json:"error,omitempty"`
	RunnerMode      string                    `json:"runnerMode"`
	InstallCommands []officeCLIInstallCommand `json:"installCommands"`
	MCPCommands     []officeCLIInstallCommand `json:"mcpCommands"`
	Capabilities    []officeCLICapability     `json:"capabilities"`
	Formats         []string                  `json:"formats"`
}

type officeCLIInstallCommand struct {
	Label   string `json:"label"`
	Command string `json:"command"`
}

type officeCLICapability struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}

type officeCLIDocumentRequest struct {
	Format string `json:"format"`
	Title  string `json:"title"`
	Prompt string `json:"prompt"`
}

type officeCLIDocumentResponse struct {
	ID          string                    `json:"id"`
	FileName    string                    `json:"fileName"`
	Format      string                    `json:"format"`
	Title       string                    `json:"title"`
	DownloadURL string                    `json:"downloadUrl"`
	Size        int64                     `json:"size"`
	Commands    []officeCLIInstallCommand `json:"commands"`
}

func (a api) officeCLIStatus(w http.ResponseWriter, r *http.Request) {
	if _, err := a.currentUser(r); err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	writeJSON(w, detectOfficeCLIStatus(r.Context()))
}

func (a api) createOfficeCLIDocument(w http.ResponseWriter, r *http.Request) {
	user, err := a.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var req officeCLIDocumentRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.Format = strings.ToLower(strings.TrimSpace(req.Format))
	req.Title = strings.TrimSpace(req.Title)
	req.Prompt = strings.TrimSpace(req.Prompt)
	if !officeCLIAllowedFormat(req.Format) {
		writeError(w, http.StatusBadRequest, errors.New("officecli format must be docx, xlsx, or pptx"))
		return
	}
	if req.Title == "" {
		req.Title = "OfficeCLI 文档"
	}
	if req.Prompt == "" {
		writeError(w, http.StatusBadRequest, errors.New("officecli prompt is required"))
		return
	}

	status := detectOfficeCLIStatus(r.Context())
	if !status.Available || status.BinaryPath == "" {
		writeError(w, http.StatusServiceUnavailable, errors.New("officecli is not available on server"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	response, err := a.generateOfficeCLIDocument(ctx, user, status.BinaryPath, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, response)
}

func (a api) downloadOfficeCLIDocument(w http.ResponseWriter, r *http.Request) {
	user, err := a.currentUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	fileName := filepath.Base(strings.TrimSpace(r.PathValue("fileName")))
	if fileName == "." || fileName == "" || fileName != strings.TrimSpace(r.PathValue("fileName")) || !strings.HasPrefix(fileName, "officecli_") || !officeCLIAllowedFormat(strings.TrimPrefix(strings.ToLower(filepath.Ext(fileName)), ".")) {
		writeError(w, http.StatusBadRequest, errors.New("invalid officecli file name"))
		return
	}
	filePath := filepath.Join(a.officeCLIUserDir(user), fileName)
	if _, err := os.Stat(filePath); err != nil {
		writeError(w, http.StatusNotFound, errors.New("officecli document not found"))
		return
	}
	writeAttachmentHeaders(w, officeCLIContentType(fileName), fileName)
	http.ServeFile(w, r, filePath)
}

func (a api) generateOfficeCLIDocument(ctx context.Context, user adminUser, binaryPath string, req officeCLIDocumentRequest) (officeCLIDocumentResponse, error) {
	outputDir := a.officeCLIUserDir(user)
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return officeCLIDocumentResponse{}, err
	}
	id := fmt.Sprintf("officecli_%d_%s", time.Now().UnixNano(), officeCLISlug(req.Title))
	fileName := id + "." + req.Format
	outputPath := filepath.Join(outputDir, fileName)
	commands := []officeCLIInstallCommand{}
	run := func(label string, args ...string) error {
		commands = append(commands, officeCLIInstallCommand{Label: label, Command: "officecli " + strings.Join(args, " ")})
		return runOfficeCLI(ctx, binaryPath, args...)
	}

	if err := run("Create", "create", outputPath); err != nil {
		return officeCLIDocumentResponse{}, err
	}
	switch req.Format {
	case "docx":
		if err := buildOfficeCLIDocx(run, outputPath, req); err != nil {
			return officeCLIDocumentResponse{}, err
		}
	case "xlsx":
		if err := buildOfficeCLIXlsx(run, outputPath, req); err != nil {
			return officeCLIDocumentResponse{}, err
		}
	case "pptx":
		if err := buildOfficeCLIPptx(run, outputPath, req); err != nil {
			return officeCLIDocumentResponse{}, err
		}
	}
	if err := run("Validate", "validate", outputPath); err != nil {
		return officeCLIDocumentResponse{}, err
	}
	if err := run("Close", "close", outputPath); err != nil {
		return officeCLIDocumentResponse{}, err
	}
	info, err := os.Stat(outputPath)
	if err != nil {
		return officeCLIDocumentResponse{}, err
	}
	return officeCLIDocumentResponse{
		ID:          id,
		FileName:    fileName,
		Format:      req.Format,
		Title:       req.Title,
		DownloadURL: "/api/v1/officecli/documents/" + fileName + "/download",
		Size:        info.Size(),
		Commands:    commands,
	}, nil
}

func detectOfficeCLIStatus(ctx context.Context) officeCLIStatusResponse {
	response := officeCLIBaseStatus()
	binaryPath, err := exec.LookPath("officecli")
	if err != nil {
		response.Error = "officecli binary was not found in PATH"
		return response
	}
	response.BinaryPath = binaryPath

	version, err := officeCLIVersion(ctx, binaryPath)
	if err != nil {
		response.Error = err.Error()
		return response
	}
	response.Available = true
	response.Version = version
	return response
}

func officeCLIVersion(ctx context.Context, binaryPath string) (string, error) {
	commandCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	output, err := exec.CommandContext(commandCtx, binaryPath, "--version").CombinedOutput()
	version := strings.TrimSpace(string(output))
	if commandCtx.Err() != nil {
		return "", errors.New("officecli --version timed out")
	}
	if err != nil {
		if version != "" {
			return "", errors.New(version)
		}
		return "", err
	}
	if version == "" {
		return "", errors.New("officecli --version returned empty output")
	}
	return version, nil
}

func buildOfficeCLIDocx(run func(string, ...string) error, outputPath string, req officeCLIDocumentRequest) error {
	if err := run("Add title", "add", outputPath, "/body", "--type", "paragraph", "--prop", "text="+req.Title, "--prop", "style=Heading1"); err != nil {
		return err
	}
	if err := run("Add summary", "add", outputPath, "/body", "--type", "paragraph", "--prop", "text=需求说明", "--prop", "bold=true"); err != nil {
		return err
	}
	for _, line := range officeCLIPromptLines(req.Prompt, 8) {
		if err := run("Add paragraph", "add", outputPath, "/body", "--type", "paragraph", "--prop", "text="+line); err != nil {
			return err
		}
	}
	return run("Add footer note", "add", outputPath, "/body", "--type", "paragraph", "--prop", "text=由知启云 AI 智能体中心调用 OfficeCLI 生成。")
}

func buildOfficeCLIXlsx(run func(string, ...string) error, outputPath string, req officeCLIDocumentRequest) error {
	rows := [][2]string{
		{"字段", "内容"},
		{"标题", req.Title},
		{"需求", req.Prompt},
		{"生成方式", "知启云 AI 智能体中心 / OfficeCLI"},
		{"生成时间", time.Now().Format("2006-01-02 15:04:05")},
	}
	for index, row := range rows {
		rowNumber := index + 1
		if err := run("Set cell", "set", outputPath, fmt.Sprintf("/Sheet1/A%d", rowNumber), "--prop", "value="+row[0]); err != nil {
			return err
		}
		if err := run("Set cell", "set", outputPath, fmt.Sprintf("/Sheet1/B%d", rowNumber), "--prop", "value="+row[1]); err != nil {
			return err
		}
	}
	return run("Format header", "set", outputPath, "/Sheet1/A1", "--prop", "bold=true")
}

func buildOfficeCLIPptx(run func(string, ...string) error, outputPath string, req officeCLIDocumentRequest) error {
	if err := run("Add cover slide", "add", outputPath, "/", "--type", "slide", "--prop", "title="+req.Title, "--prop", "background=F6F7FF"); err != nil {
		return err
	}
	if err := run("Add cover copy", "add", outputPath, "/slide[1]", "--type", "shape", "--prop", "text="+officeCLIShortText(req.Prompt, 160), "--prop", "x=1.5cm", "--prop", "y=4.4cm", "--prop", "w=22cm", "--prop", "h=3.2cm", "--prop", "size=22", "--prop", "color=1F2937"); err != nil {
		return err
	}
	if err := run("Add details slide", "add", outputPath, "/", "--type", "slide", "--prop", "title=执行要点"); err != nil {
		return err
	}
	return run("Add details", "add", outputPath, "/slide[2]", "--type", "shape", "--prop", "text="+strings.Join(officeCLIPromptLines(req.Prompt, 5), "\\n"), "--prop", "x=1.5cm", "--prop", "y=3cm", "--prop", "w=22cm", "--prop", "h=8cm", "--prop", "size=20")
}

func runOfficeCLI(ctx context.Context, binaryPath string, args ...string) error {
	output, err := exec.CommandContext(ctx, binaryPath, args...).CombinedOutput()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return errors.New(message)
	}
	return nil
}

func (a api) officeCLIUserDir(user adminUser) string {
	userID := officeCLISlug(firstNonEmptyString(user.ID, user.Email, "user"))
	return filepath.Join(filepath.Dir(a.cfg.DataPath), "officecli-documents", userID)
}

func officeCLIAllowedFormat(format string) bool {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "docx", "xlsx", "pptx":
		return true
	default:
		return false
	}
}

func officeCLIContentType(fileName string) string {
	switch strings.ToLower(filepath.Ext(fileName)) {
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".pptx":
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	default:
		return "application/octet-stream"
	}
}

var officeCLISlugPattern = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

func officeCLISlug(value string) string {
	slug := strings.Trim(officeCLISlugPattern.ReplaceAllString(value, "-"), "-_")
	if slug == "" {
		return "document"
	}
	if len(slug) > 48 {
		return slug[:48]
	}
	return slug
}

func officeCLIPromptLines(prompt string, limit int) []string {
	rawLines := strings.FieldsFunc(prompt, func(r rune) bool {
		return r == '\n' || r == '\r' || r == '；' || r == ';'
	})
	lines := make([]string, 0, limit)
	for _, line := range rawLines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lines = append(lines, officeCLIShortText(line, 220))
		if len(lines) >= limit {
			break
		}
	}
	if len(lines) == 0 {
		lines = append(lines, officeCLIShortText(prompt, 220))
	}
	return lines
}

func officeCLIShortText(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "..."
}

func officeCLIBaseStatus() officeCLIStatusResponse {
	return officeCLIStatusResponse{
		RunnerMode: "server-side-binary",
		InstallCommands: []officeCLIInstallCommand{
			{Label: "Windows PowerShell", Command: "irm https://raw.githubusercontent.com/iOfficeAI/OfficeCLI/main/install.ps1 | iex"},
			{Label: "macOS / Linux", Command: "curl -fsSL https://raw.githubusercontent.com/iOfficeAI/OfficeCLI/main/install.sh | bash"},
			{Label: "npm", Command: "npm install -g @officecli/officecli"},
		},
		MCPCommands: []officeCLIInstallCommand{
			{Label: "Claude Code", Command: "officecli mcp claude"},
			{Label: "Cursor", Command: "officecli mcp cursor"},
			{Label: "VS Code / Copilot", Command: "officecli mcp vscode"},
			{Label: "List status", Command: "officecli mcp list"},
		},
		Capabilities: []officeCLICapability{
			{Code: "create", Label: "Create Office files"},
			{Code: "read", Label: "Read structure and text"},
			{Code: "modify", Label: "Modify styles, formulas, charts"},
			{Code: "render", Label: "Render HTML and PNG previews"},
			{Code: "batch", Label: "Batch and resident operations"},
		},
		Formats: []string{"docx", "xlsx", "pptx"},
	}
}
