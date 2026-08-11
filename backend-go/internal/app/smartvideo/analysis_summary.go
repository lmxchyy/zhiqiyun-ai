package smartvideo

import (
	"fmt"
	"strings"
)

const (
	MinProjectAssets     = 2
	MaxProjectAssets     = 20
	MaxVideoFileBytes    = 500 << 20 // 500 MiB
	MaxImageFileBytes    = 30 << 20  // 30 MiB
	MaxProjectTotalBytes = 2 << 30   // 2 GiB
	MaxVideoDurationMs   = 10 * 60 * 1000
)

func BuildAssetAnalysisSummary(metadata NormalizedMediaMetadata, thumbnailFileID string) AssetAnalysisSummary {
	summary := AssetAnalysisSummary{
		Kind: strings.ToLower(strings.TrimSpace(metadata.Kind)),
	}
	if thumbnailFileID != "" {
		summary.RepresentativeFrames = []string{thumbnailFileID}
	}
	switch {
	case metadata.Video != nil:
		v := metadata.Video
		summary.Kind = "video"
		summary.DurationMs = v.DurationMS
		summary.Width = v.Width
		summary.Height = v.Height
		summary.MIMEType = mimeFromFormat(v.Format, true)
		summary.HasAudio = strings.TrimSpace(v.AudioCodec) != ""
		summary.SceneHints = []string{"unclassified"}
		summary.ColorHints = []string{"neutral"}
		summary.CompositionHints = compositionFromSize(v.Width, v.Height)
		summary.CandidateClips = defaultCandidateClips(v.DurationMS)
	case metadata.Image != nil:
		img := metadata.Image
		summary.Kind = "image"
		summary.DurationMs = 0
		summary.Width = img.Width
		summary.Height = img.Height
		summary.MIMEType = mimeFromFormat(img.Format, false)
		summary.SceneHints = []string{"still"}
		summary.ColorHints = []string{"neutral"}
		summary.CompositionHints = compositionFromSize(img.Width, img.Height)
		summary.CandidateClips = nil
	}
	return summary
}

func ValidateAssetQuota(existing []ProjectAsset, incoming FileReference, assetType string) error {
	if len(existing) >= MaxProjectAssets {
		return fmt.Errorf("%w: asset count exceeds %d", ErrInvalidInput, MaxProjectAssets)
	}
	assetType = strings.ToUpper(strings.TrimSpace(assetType))
	size := incoming.Metadata.FileSize
	switch assetType {
	case AssetTypeVideo:
		if size > MaxVideoFileBytes {
			return fmt.Errorf("%w: video file exceeds %d bytes", ErrInvalidInput, MaxVideoFileBytes)
		}
		if incoming.Metadata.DurationMS > MaxVideoDurationMs {
			return fmt.Errorf("%w: video duration exceeds %d ms", ErrInvalidInput, MaxVideoDurationMs)
		}
	case AssetTypeImage:
		if size > MaxImageFileBytes {
			return fmt.Errorf("%w: image file exceeds %d bytes", ErrInvalidInput, MaxImageFileBytes)
		}
	default:
		return fmt.Errorf("%w: unsupported asset type", ErrInvalidInput)
	}
	var total int64
	for _, asset := range existing {
		total += asset.Metadata.FileSize
	}
	if total+size > MaxProjectTotalBytes {
		return fmt.Errorf("%w: project total size exceeds %d bytes", ErrInvalidInput, MaxProjectTotalBytes)
	}
	return nil
}

func CanEditAssets(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case ProjectStatusDraft, ProjectStatusMaterialReady, ProjectStatusFailed:
		return true
	default:
		return false
	}
}

func defaultCandidateClips(durationMs int64) []CandidateClip {
	if durationMs <= 0 {
		return nil
	}
	clips := []CandidateClip{{
		StartMs: 0, EndMs: minInt64(durationMs, 3000), Confidence: 0.5, Reason: "opening",
	}}
	if durationMs > 6000 {
		midStart := durationMs / 2
		clips = append(clips, CandidateClip{
			StartMs: midStart, EndMs: minInt64(durationMs, midStart+3000), Confidence: 0.4, Reason: "midpoint",
		})
	}
	if durationMs > 3000 {
		endStart := maxInt64(0, durationMs-3000)
		clips = append(clips, CandidateClip{
			StartMs: endStart, EndMs: durationMs, Confidence: 0.4, Reason: "closing",
		})
	}
	return clips
}

func compositionFromSize(width, height int) []string {
	if width <= 0 || height <= 0 {
		return []string{"unknown"}
	}
	if width > height {
		return []string{"landscape"}
	}
	if height > width {
		return []string{"portrait"}
	}
	return []string{"square"}
}

func mimeFromFormat(format string, video bool) string {
	format = strings.ToLower(strings.TrimSpace(format))
	switch format {
	case "mp4", "mov", "m4v":
		return "video/mp4"
	case "webm":
		return "video/webm"
	case "jpeg", "jpg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "webp":
		return "image/webp"
	}
	if video {
		return "video/*"
	}
	return "image/*"
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
