package image

import (
	"fmt"
	"strings"
)

// Billing size tiers are a charging classification, not the provider size.
// The provider still sends the real official WxH (or auto).
const (
	ImageBillingSizeAuto    = "auto"
	ImageBillingSizeTier720 = "tier_720p"
	ImageBillingSizeTier1K  = "tier_1k"
	ImageBillingSizeTier2K  = "tier_2k"
	ImageBillingSizeTier4K  = "tier_4k"
)

// Conservative GPT Image billing ceilings, anchored on product presets:
//
//	720P  1280x720 / 720x1280     pixels <= 921600  and max edge <= 1280
//	1K    1024x1024 / 1536x1024   pixels <= 1572864 and max edge <= 1536
//	2K    2048x1152 / 2048x2048   pixels <= 4194304 and max edge <= 2048
//	4K    3840x2160 / 2160x3840   remaining official-legal WxH
//
// A size that exceeds either the pixel ceiling or the max-edge ceiling of a
// tier is promoted to the next higher tier. Between two tiers, never round down.
const (
	imageBilling720PMaxPixels = 1280 * 720
	imageBilling720PMaxEdge   = 1280
	imageBilling1KMaxPixels   = 1536 * 1024
	imageBilling1KMaxEdge     = 1536
	imageBilling2KMaxPixels   = 2048 * 2048
	imageBilling2KMaxEdge     = 2048
)

// NormalizeImageBillingSizeTier maps an official GPT Image size to a billing tier.
// Illegal sizes return an error and must not be billed as the cheapest tier.
func NormalizeImageBillingSizeTier(size any) (string, error) {
	if err := ValidateGPTImageSize(size); err != nil {
		return "", err
	}
	raw := strings.ToLower(strings.TrimSpace(fmt.Sprint(size)))
	if raw == "" || raw == "<nil>" || raw == ImageBillingSizeAuto {
		return ImageBillingSizeAuto, nil
	}
	width, height, ok := parseGPTImageSize(raw)
	if !ok {
		return "", fmt.Errorf("unsupported OpenAI image size %q", raw)
	}
	return imageBillingSizeTierForPixels(width, height), nil
}

func imageBillingSizeTierForPixels(width, height int) string {
	pixels := width * height
	maxEdge := width
	if height > maxEdge {
		maxEdge = height
	}
	if pixels <= imageBilling720PMaxPixels && maxEdge <= imageBilling720PMaxEdge {
		return ImageBillingSizeTier720
	}
	if pixels <= imageBilling1KMaxPixels && maxEdge <= imageBilling1KMaxEdge {
		return ImageBillingSizeTier1K
	}
	if pixels <= imageBilling2KMaxPixels && maxEdge <= imageBilling2KMaxEdge {
		return ImageBillingSizeTier2K
	}
	return ImageBillingSizeTier4K
}
