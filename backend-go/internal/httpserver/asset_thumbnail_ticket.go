package httpserver

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"xianzhi-ai/backend-go/internal/config"
)

const (
	assetThumbnailTicketPurpose = "asset-thumbnail"
	assetThumbnailTicketTTL     = 15 * time.Minute
	assetThumbnailCacheControl  = "private, max-age=60"
)

type assetThumbnailSigner struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

func newAssetThumbnailSigner(cfg config.Config, now func() time.Time) assetThumbnailSigner {
	if now == nil {
		now = time.Now
	}
	ttl := assetThumbnailTicketTTL
	if seconds, err := strconv.Atoi(strings.TrimSpace(cfg.StorageAccessURLTTLSeconds)); err == nil && seconds > 0 {
		ttl = time.Duration(seconds) * time.Second
	}
	secret := strings.TrimSpace(cfg.InspirationDraftHMACSecret)
	if secret == "" {
		secret = strings.TrimSpace(cfg.PaymentCallbackSecret)
	}
	if secret == "" {
		secret = strings.TrimSpace(cfg.StorageMasterKey)
	}
	if secret == "" {
		// Keep sign/verify consistent in local tests without extra env.
		secret = "xianzhi-dev-asset-thumbnail"
	}
	return assetThumbnailSigner{secret: []byte(secret), ttl: ttl, now: now}
}

func (s assetThumbnailSigner) issue(assetID, userID string) (exp int64, sig string) {
	exp = s.now().UTC().Add(s.ttl).Unix()
	return exp, s.signature(assetID, userID, exp)
}

func (s assetThumbnailSigner) verify(assetID, userID string, exp int64, sig string) bool {
	if strings.TrimSpace(assetID) == "" || strings.TrimSpace(userID) == "" || exp <= 0 || strings.TrimSpace(sig) == "" {
		return false
	}
	if s.now().UTC().Unix() >= exp {
		return false
	}
	provided, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil {
		return false
	}
	expected, err := base64.RawURLEncoding.DecodeString(s.signature(assetID, userID, exp))
	if err != nil || len(expected) == 0 {
		return false
	}
	return hmac.Equal(provided, expected)
}

func (s assetThumbnailSigner) signature(assetID, userID string, exp int64) string {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(assetThumbnailTicketCanonical(assetID, userID, exp)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func assetThumbnailTicketCanonical(assetID, userID string, exp int64) string {
	return strings.Join([]string{
		assetThumbnailTicketPurpose,
		strings.TrimSpace(assetID),
		strings.TrimSpace(userID),
		strconv.FormatInt(exp, 10),
	}, "\n")
}

func publicAPIBaseURL(r *http.Request) string {
	if r == nil {
		return ""
	}
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0]), "https") {
		scheme = "https"
	}
	host := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Host"), ",")[0])
	if host == "" {
		host = strings.TrimSpace(r.Host)
	}
	if host == "" {
		return ""
	}
	return scheme + "://" + host
}

func assetThumbnailTicketURL(base string, assetID string, exp int64, sig string) string {
	path := "/api/v1/assets/" + url.PathEscape(strings.TrimSpace(assetID)) + "/thumbnail"
	query := url.Values{}
	query.Set("exp", strconv.FormatInt(exp, 10))
	query.Set("sig", sig)
	if strings.TrimRight(strings.TrimSpace(base), "/") == "" {
		return path + "?" + query.Encode()
	}
	return strings.TrimRight(strings.TrimSpace(base), "/") + path + "?" + query.Encode()
}
