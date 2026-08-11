package smartvideo

import (
	"math"
	"time"
)

const (
	QuoteTTL             = 15 * time.Minute
	quoteBasePointsPerSec = 2.0
)

// EstimateRenderQuote applies the V1 server-side pricing rule. Clients cannot
// supply points; only target resolution/duration/voice affect the quote.
func EstimateRenderQuote(input RenderQuoteInput, now time.Time) RenderQuote {
	seconds := math.Ceil(float64(input.DurationMs) / 1000.0)
	if seconds < 1 {
		seconds = 1
	}
	multiplier := 1.0
	switch input.Resolution {
	case TargetResolution1080p:
		multiplier *= 1.5
	default:
		multiplier *= 1.0
	}
	if input.Voice {
		multiplier *= 1.2
	}
	points := int64(math.Ceil(seconds * quoteBasePointsPerSec * multiplier))
	if points < 1 {
		points = 1
	}
	return RenderQuote{Points: points, ExpiresAt: now.UTC().Add(QuoteTTL)}
}
