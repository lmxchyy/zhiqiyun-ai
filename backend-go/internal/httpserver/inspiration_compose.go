package httpserver

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const creationDraftContractVersion = 1

type CreationDraftTemplateRef struct {
	ID      string `json:"id"`
	Slug    string `json:"slug"`
	Version int    `json:"version"`
}

type CreationDraft struct {
	ContractVersion int                       `json:"contractVersion"`
	TemplateRef     CreationDraftTemplateRef  `json:"templateRef"`
	ContentType     string                    `json:"contentType"`
	Handoff         TemplateHandoffDefinition `json:"handoff"`
	Values          map[string]any            `json:"values"`
	Materials       []TemplateComposeMaterial `json:"materials"`
	BasePrompt      string                    `json:"basePrompt"`
	NegativePrompt  string                    `json:"negativePrompt,omitempty"`
	Parameters      map[string]any            `json:"parameters"`
	CapabilityKey   string                    `json:"capabilityKey"`
	ModelHint       string                    `json:"modelHint,omitempty"`
	IntegrityToken  string                    `json:"integrityToken"`
	CreatedAt       string                    `json:"createdAt"`
	ExpiresAt       string                    `json:"expiresAt"`
}

type inspirationDraftSigner struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

type inspirationDraftClaims struct {
	ContractVersion int    `json:"v"`
	TemplateID      string `json:"templateId"`
	TemplateSlug    string `json:"templateSlug"`
	TemplateVersion int    `json:"templateVersion"`
	ContentType     string `json:"contentType"`
	CapabilityKey   string `json:"capabilityKey"`
	InputDigest     string `json:"inputDigest"`
	IssuedUnix      int64  `json:"iat"`
	ExpiresUnix     int64  `json:"exp"`
}

func newInspirationDraftSigner(secret []byte, ttl time.Duration, now func() time.Time) inspirationDraftSigner {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	if now == nil {
		now = time.Now
	}
	key := append([]byte(nil), secret...)
	if len(key) == 0 {
		key = make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			key = nil
		}
	}
	return inspirationDraftSigner{secret: key, ttl: ttl, now: now}
}

func (s inspirationDraftSigner) issue(draft *CreationDraft) error {
	if draft == nil || len(s.secret) == 0 {
		return errors.New("creation draft signer is unavailable")
	}
	created := s.now().UTC()
	expires := created.Add(s.ttl)
	draft.CreatedAt = created.Format(time.RFC3339Nano)
	draft.ExpiresAt = expires.Format(time.RFC3339Nano)
	claims, err := creationDraftClaims(*draft, expires.Unix())
	if err != nil {
		return err
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return err
	}
	signature := hmac.New(sha256.New, s.secret)
	_, _ = signature.Write(payload)
	draft.IntegrityToken = base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(signature.Sum(nil))
	return nil
}

// trustedAttribution deliberately returns only a trust signal. An invalid or
// expired token removes trusted template attribution but is not a creation error.
func (s inspirationDraftSigner) trustedAttribution(draft CreationDraft) bool {
	parts := strings.Split(draft.IntegrityToken, ".")
	if len(parts) != 2 || len(s.secret) == 0 {
		return false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	providedSignature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	expectedSignature := hmac.New(sha256.New, s.secret)
	_, _ = expectedSignature.Write(payload)
	if !hmac.Equal(providedSignature, expectedSignature.Sum(nil)) {
		return false
	}
	var claims inspirationDraftClaims
	if err = json.Unmarshal(payload, &claims); err != nil || s.now().UTC().Unix() >= claims.ExpiresUnix {
		return false
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, draft.ExpiresAt)
	if err != nil || expiresAt.Unix() != claims.ExpiresUnix {
		return false
	}
	expected, err := creationDraftClaims(draft, claims.ExpiresUnix)
	return err == nil && claims == expected
}

func creationDraftClaims(draft CreationDraft, expiresUnix int64) (inspirationDraftClaims, error) {
	createdAt, err := time.Parse(time.RFC3339Nano, draft.CreatedAt)
	if err != nil {
		return inspirationDraftClaims{}, err
	}
	input, err := json.Marshal(struct {
		Values    map[string]any            `json:"values"`
		Materials []TemplateComposeMaterial `json:"materials"`
	}{Values: draft.Values, Materials: draft.Materials})
	if err != nil {
		return inspirationDraftClaims{}, err
	}
	digest := sha256.Sum256(input)
	return inspirationDraftClaims{
		ContractVersion: draft.ContractVersion,
		TemplateID:      draft.TemplateRef.ID, TemplateSlug: draft.TemplateRef.Slug, TemplateVersion: draft.TemplateRef.Version,
		ContentType: draft.ContentType, CapabilityKey: draft.CapabilityKey,
		InputDigest: hex.EncodeToString(digest[:]), IssuedUnix: createdAt.Unix(), ExpiresUnix: expiresUnix,
	}, nil
}

func (a inspirationAPI) compose(w http.ResponseWriter, r *http.Request) {
	var request struct {
		TemplateVersion int                       `json:"templateVersion"`
		Values          map[string]any            `json:"values"`
		Materials       []TemplateComposeMaterial `json:"materials"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeInspirationComposeError(w, http.StatusBadRequest, "INSPIRATION_COMPOSE_REQUEST_INVALID", errors.New("invalid compose request"))
		return
	}
	if request.TemplateVersion < 1 {
		writeInspirationComposeError(w, http.StatusBadRequest, "INSPIRATION_TEMPLATE_VERSION_REQUIRED", errors.New("templateVersion is required"))
		return
	}
	userID, tenantID := a.optionalIdentity(r)
	item, err := a.repo.GetTemplateVersionBySlug(r.Context(), tenantID, userID, r.PathValue("slug"), request.TemplateVersion)
	if err != nil {
		if errors.Is(err, errInspirationVersionConflict) {
			writeInspirationComposeError(w, http.StatusConflict, "INSPIRATION_TEMPLATE_VERSION_CONFLICT", err)
			return
		}
		writeInspirationError(w, err)
		return
	}
	platform := firstNonEmptyString(strings.TrimSpace(r.URL.Query().Get("platform")), requestTerminal(r), "miniprogram")
	if len(item.Platforms) > 0 && !stringListContains(item.Platforms, platform) {
		writeInspirationError(w, errInspirationNotFound)
		return
	}
	if issues := validateTemplateDefinition(item.ContentType, item.Definition); len(issues) > 0 {
		writeInspirationComposeError(w, http.StatusInternalServerError, "INSPIRATION_TEMPLATE_DEFINITION_INVALID", errors.New("stored template definition is invalid"))
		return
	}
	composition, err := composeTemplateDefinition(item.Definition, request.Values, request.Materials)
	if err != nil {
		writeInspirationComposeError(w, http.StatusBadRequest, "INSPIRATION_INPUT_INVALID", err)
		return
	}
	if err = a.validateComposeMaterials(userID, item.Definition, composition.Materials); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errUnauthorized) {
			status = http.StatusUnauthorized
		}
		writeInspirationComposeError(w, status, "INSPIRATION_MATERIAL_INVALID", err)
		return
	}
	draft := CreationDraft{
		ContractVersion: creationDraftContractVersion,
		TemplateRef:     CreationDraftTemplateRef{ID: item.ID, Slug: item.Slug, Version: item.Version},
		ContentType:     item.ContentType, Handoff: item.Definition.Handoff,
		Values: composition.Values, Materials: composition.Materials,
		BasePrompt: composition.BasePrompt, NegativePrompt: composition.NegativePrompt,
		Parameters: composition.Parameters, CapabilityKey: item.Definition.Capability.CapabilityKey,
		ModelHint: item.Definition.Capability.ModelHint,
	}
	if err = a.draftSigner.issue(&draft); err != nil {
		writeInspirationComposeError(w, http.StatusInternalServerError, "INSPIRATION_DRAFT_SIGNING_UNAVAILABLE", err)
		return
	}
	writeJSON(w, map[string]any{"draft": draft})
}

func writeInspirationComposeError(w http.ResponseWriter, status int, code string, err error) {
	writeJSONStatus(w, status, map[string]any{"code": code, "error": err.Error()})
}

func (a inspirationAPI) validateComposeMaterials(userID string, definition InternalTemplateDefinition, materials []TemplateComposeMaterial) error {
	if len(materials) == 0 {
		return nil
	}
	if userID == "" {
		return errUnauthorized
	}
	inputs := make(map[string]TemplateInputDefinition, len(definition.Inputs))
	for _, input := range definition.Inputs {
		inputs[input.Key] = input
	}
	for _, material := range materials {
		input, exists := inputs[material.InputKey]
		if !exists || !templateMaterialInput(input.Type) {
			return fmt.Errorf("unknown material input %s", material.InputKey)
		}
		item, found, err := a.composeAssetForUser(userID, material.AssetID)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("material asset %s is unavailable", material.AssetID)
		}
		if !materialMatchesInput(item, input) {
			return fmt.Errorf("material asset %s has an incompatible type", material.AssetID)
		}
	}
	return nil
}

func (a inspirationAPI) composeAssetForUser(userID, assetID string) (asset, bool, error) {
	if optimized, ok := a.store.(optimizedUserAssetDetailStore); ok {
		return optimized.GetAssetForUser(userID, assetID)
	}
	items, err := a.store.ListAssets()
	if err != nil {
		return asset{}, false, err
	}
	for _, item := range items {
		if item.ID == assetID && item.UserID == userID && item.DeletedAt == "" {
			return item, true, nil
		}
	}
	return asset{}, false, nil
}

func materialMatchesInput(item asset, input TemplateInputDefinition) bool {
	mediaType := strings.ToLower(strings.TrimSpace(item.MediaType))
	mimeType := strings.ToLower(strings.TrimSpace(stringValue(item.Metadata["contentType"])))
	switch input.Type {
	case TemplateInputImage:
		if mediaType != "image" && !strings.HasPrefix(mimeType, "image/") {
			return false
		}
	case TemplateInputVideo:
		if mediaType != "video" && !strings.HasPrefix(mimeType, "video/") {
			return false
		}
	}
	if len(input.Validation.Accept) == 0 || mimeType == "" {
		return true
	}
	for _, accepted := range input.Validation.Accept {
		accepted = strings.ToLower(strings.TrimSpace(accepted))
		if accepted == mimeType || (strings.HasSuffix(accepted, "/*") && strings.HasPrefix(mimeType, strings.TrimSuffix(accepted, "*"))) {
			return true
		}
	}
	return false
}
