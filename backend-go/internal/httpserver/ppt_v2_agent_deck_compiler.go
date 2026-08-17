package httpserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

type nodePPTV2AgentDeckCompiler struct {
	nodeExecutable string
	cliPath        string
	files          *storagecenter.Service
}

func (c nodePPTV2AgentDeckCompiler) Compile(ctx context.Context, input pptapp.DeckBuildInput) (pptapp.DeckCompilation, error) {
	request, err := json.Marshal(input)
	if err != nil {
		return pptapp.DeckCompilation{}, err
	}
	stdout, err := c.run(ctx, []string{c.cliPath, "compile"}, request)
	if err != nil {
		return pptapp.DeckCompilation{}, err
	}
	var compiled pptapp.DeckCompilation
	if err := json.Unmarshal(stdout, &compiled); err != nil {
		return pptapp.DeckCompilation{}, fmt.Errorf("decode PPT V2 deck compilation: %w", err)
	}
	if compiled.DeckID == "" || compiled.Revision <= 0 || compiled.SlideCount != len(input.ApprovedOutline.Slides) || len(compiled.Deck) == 0 || len(compiled.LayoutResult) == 0 || len(compiled.RenderInput) == 0 {
		return pptapp.DeckCompilation{}, errors.New("PPT V2 deck compiler returned an invalid result")
	}
	return compiled, nil
}

func (c nodePPTV2AgentDeckCompiler) Render(ctx context.Context, compilation pptapp.DeckCompilation, assets []pptapp.ResolvedDeckAsset) (pptapp.DeckRenderOutput, error) {
	if c.files == nil {
		return pptapp.DeckRenderOutput{}, errors.New("private asset storage is unavailable")
	}
	assetData := make(map[string]string, len(assets))
	for _, asset := range assets {
		_, stream, err := c.files.OpenObject(ctx, storagecenter.AccessContext{TenantID: asset.TenantID, UserID: asset.UserID}, asset.FileID)
		if err != nil {
			return pptapp.DeckRenderOutput{}, fmt.Errorf("open private image asset %s: %w", asset.ID, err)
		}
		data, readErr := io.ReadAll(io.LimitReader(stream, 25<<20))
		closeErr := stream.Close()
		if readErr != nil {
			return pptapp.DeckRenderOutput{}, readErr
		}
		if closeErr != nil {
			return pptapp.DeckRenderOutput{}, closeErr
		}
		digest := sha256.Sum256(data)
		if hex.EncodeToString(digest[:]) != asset.SHA256 {
			return pptapp.DeckRenderOutput{}, fmt.Errorf("private image asset %s checksum mismatch", asset.ID)
		}
		assetData[asset.ID] = "data:" + asset.MIMEType + ";base64," + base64.StdEncoding.EncodeToString(data)
	}
	request, err := json.Marshal(map[string]any{"compilation": compilation, "assetData": assetData})
	if err != nil {
		return pptapp.DeckRenderOutput{}, err
	}
	output, err := os.CreateTemp("", "ppt-v2-slice-b-*.pptx")
	if err != nil {
		return pptapp.DeckRenderOutput{}, err
	}
	outputPath := output.Name()
	if err := output.Close(); err != nil {
		_ = os.Remove(outputPath)
		return pptapp.DeckRenderOutput{}, err
	}
	defer func() { _ = os.Remove(outputPath) }()
	stdout, err := c.run(ctx, []string{c.cliPath, "render", outputPath}, request)
	if err != nil {
		return pptapp.DeckRenderOutput{}, err
	}
	var metadata struct {
		DeckID     string `json:"deckId"`
		Revision   int    `json:"revision"`
		SlideCount int    `json:"slideCount"`
		Bytes      int    `json:"bytes"`
	}
	if err := json.Unmarshal(stdout, &metadata); err != nil {
		return pptapp.DeckRenderOutput{}, err
	}
	pptx, err := os.ReadFile(outputPath)
	if err != nil {
		return pptapp.DeckRenderOutput{}, err
	}
	if metadata.Bytes != len(pptx) || len(pptx) < 2 || string(pptx[:2]) != "PK" {
		return pptapp.DeckRenderOutput{}, errors.New("PPT V2 renderer returned an invalid package")
	}
	return pptapp.DeckRenderOutput{DeckID: metadata.DeckID, Revision: metadata.Revision, SlideCount: metadata.SlideCount, PPTX: pptx}, nil
}

func (c nodePPTV2AgentDeckCompiler) run(ctx context.Context, args []string, request []byte) ([]byte, error) {
	if strings.TrimSpace(c.nodeExecutable) == "" || strings.TrimSpace(c.cliPath) == "" {
		return nil, errors.New("PPT V2 Node compiler is not configured")
	}
	command := exec.CommandContext(ctx, c.nodeExecutable, args...)
	command.Stdin = bytes.NewReader(request)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("PPT V2 Node compiler failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

var _ pptapp.AgentDeckCompilerPort = nodePPTV2AgentDeckCompiler{}

func newConfiguredPPTV2AgentDeckCompiler(files *storagecenter.Service) nodePPTV2AgentDeckCompiler {
	node, _ := exec.LookPath("node")
	cli := ""
	for _, candidate := range []string{
		filepath.Join("packages", "ppt-v2", "src", "professional-cli.mjs"),
		filepath.Join("..", "packages", "ppt-v2", "src", "professional-cli.mjs"),
		filepath.Join("..", "..", "..", "packages", "ppt-v2", "src", "professional-cli.mjs"),
	} {
		absolute, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if info, err := os.Stat(absolute); err == nil && !info.IsDir() {
			cli = absolute
			break
		}
	}
	return nodePPTV2AgentDeckCompiler{nodeExecutable: node, cliPath: cli, files: files}
}
