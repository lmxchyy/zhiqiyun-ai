package httpserver

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	thumbnailMaxWidth    = 420
	thumbnailMaxBytes    = 12 << 20
	thumbnailReadTimeout = 60 * time.Second
)

func thumbnailForImage(ctx context.Context, imageURL string) string {
	thumbnailURL, _, _, ok := thumbnailAndDimensionsForImage(ctx, imageURL)
	if !ok || thumbnailURL == "" {
		return imageURL
	}
	return thumbnailURL
}

func thumbnailAndDimensionsForImage(ctx context.Context, imageURL string) (string, int, int, bool) {
	raw, ok := readImageBytes(ctx, imageURL)
	if !ok {
		return "", 0, 0, false
	}
	if strings.HasPrefix(imageURL, "data:image/svg+xml") {
		width, height, ok := svgDimensions(string(raw))
		return imageURL, width, height, ok
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		cfg, _, cfgErr := image.DecodeConfig(bytes.NewReader(raw))
		if cfgErr == nil && cfg.Width > 0 && cfg.Height > 0 {
			return imageURL, cfg.Width, cfg.Height, true
		}
		return "", 0, 0, false
	}
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return "", 0, 0, false
	}
	scale := math.Min(1, float64(thumbnailMaxWidth)/float64(width))
	targetWidth := int(math.Max(1, math.Round(float64(width)*scale)))
	targetHeight := int(math.Max(1, math.Round(float64(height)*scale)))
	dst := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	for y := 0; y < targetHeight; y++ {
		sourceY := bounds.Min.Y + y*height/targetHeight
		for x := 0; x < targetWidth; x++ {
			sourceX := bounds.Min.X + x*width/targetWidth
			dst.Set(x, y, img.At(sourceX, sourceY))
		}
	}

	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 78}); err != nil {
		return imageURL, width, height, true
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(out.Bytes()), width, height, true
}

func decodeImageForThumbnail(ctx context.Context, imageURL string) (image.Image, bool) {
	raw, ok := readImageBytes(ctx, imageURL)
	if !ok {
		return nil, false
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	return img, err == nil
}

func imageDimensionsForImage(ctx context.Context, imageURL string) (int, int, bool) {
	raw, ok := readImageBytes(ctx, imageURL)
	if !ok {
		return 0, 0, false
	}
	if strings.HasPrefix(imageURL, "data:image/svg+xml") {
		return svgDimensions(string(raw))
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return 0, 0, false
	}
	return cfg.Width, cfg.Height, true
}

func readImageBytes(ctx context.Context, imageURL string) ([]byte, bool) {
	if strings.HasPrefix(imageURL, "data:image/") {
		comma := strings.IndexByte(imageURL, ',')
		if comma < 0 || !strings.Contains(imageURL[:comma], ";base64") {
			return nil, false
		}
		raw, err := base64.StdEncoding.DecodeString(imageURL[comma+1:])
		if err != nil {
			return nil, false
		}
		return raw, true
	}
	if !strings.HasPrefix(imageURL, "http://") && !strings.HasPrefix(imageURL, "https://") {
		return nil, false
	}
	reqCtx, cancel := context.WithTimeout(ctx, thumbnailReadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, false
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, false
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, false
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, thumbnailMaxBytes))
	if err != nil {
		return nil, false
	}
	return raw, true
}

func svgDimensions(svg string) (int, int, bool) {
	width, okWidth := svgDimension(svg, "width")
	height, okHeight := svgDimension(svg, "height")
	return width, height, okWidth && okHeight && width > 0 && height > 0
}

func svgDimension(svg string, name string) (int, bool) {
	re := regexp.MustCompile(name + `="([0-9]+)`)
	matches := re.FindStringSubmatch(svg)
	if len(matches) < 2 {
		return 0, false
	}
	value, err := strconv.Atoi(matches[1])
	return value, err == nil
}
