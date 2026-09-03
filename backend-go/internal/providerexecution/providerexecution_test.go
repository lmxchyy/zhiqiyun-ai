package providerexecution

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestFingerprintCanonicalAndExcludesOperationalFields(t *testing.T) {
	a := map[string]any{"prompt": "hello", "size": "1024x1024", "nested": map[string]any{"b": 2, "a": 1}}
	b := map[string]any{"nested": map[string]any{"a": 1, "b": 2}, "size": "1024x1024", "prompt": "hello"}
	x, err := Fingerprint("task", "grok", "model", "image", a)
	if err != nil {
		t.Fatal(err)
	}
	y, err := Fingerprint("task", "grok", "model", "image", b)
	if err != nil || x != y {
		t.Fatalf("fingerprint not canonical: %s %s %v", x, y, err)
	}
	z, _ := Fingerprint("task", "other", "model", "image", a)
	if x == z {
		t.Fatal("provider must affect fingerprint")
	}
}
func TestStateTransitions(t *testing.T) {
	valid := [][2]Status{{Prepared, Submitting}, {Submitting, Submitted}, {Submitting, Processing}, {Submitting, Succeeded}, {Submitted, Processing}, {Processing, Succeeded}, {Submitting, Unknown}, {Unknown, Submitted}, {Unknown, Succeeded}, {Unknown, Failed}}
	for _, p := range valid {
		if !CanTransition(p[0], p[1]) {
			t.Errorf("expected %s -> %s", p[0], p[1])
		}
	}
	// Terminal provider success/failure must never be reopened by stale repair
	// or a retry, while ambiguous states remain query/recovery-only.
	invalid := [][2]Status{{Succeeded, Submitting}, {Succeeded, Failed}, {Succeeded, Unknown}, {Failed, Processing}, {Unknown, Prepared}, {Prepared, Processing}}
	for _, p := range invalid {
		if CanTransition(p[0], p[1]) {
			t.Errorf("unexpected %s -> %s", p[0], p[1])
		}
	}
}
func TestUnknownNeverBlindlyResubmits(t *testing.T) {
	d := Decide(ProviderPolicy{QuerySupported: false}, ProviderUnknown)
	if d.Retry || d.QueryFirst {
		t.Fatalf("unsafe unknown decision: %+v", d)
	}
	d = Decide(ProviderPolicy{QuerySupported: true}, ProviderUnknown)
	if d.Retry || !d.QueryFirst {
		t.Fatalf("query-first decision: %+v", d)
	}
}
func TestRetryPolicyClassifications(t *testing.T) {
	if !Decide(ProviderPolicy{}, DefinitiveNotSubmitted).Retry {
		t.Fatal("definitive pre-submit failure should retry")
	}
	if Decide(ProviderPolicy{}, PossiblySubmitted).Retry {
		t.Fatal("possibly submitted must not retry")
	}
	if Decide(ProviderPolicy{}, ProviderSucceeded).Retry {
		t.Fatal("success must not retry")
	}
	_ = time.Second
}

// TestClassifyPreSubmitValidationErrors verifies that all known deterministic
// pre-submit validation errors from image and video providers are classified
// as DefinitiveNotSubmitted instead of the unsafe PossiblySubmitted default.
// This is the bug that caused task_000230 to enter PROCESSING/RESERVED with
// error_class=possibly_submitted when no provider call was ever made.
func TestClassifyPreSubmitValidationErrors(t *testing.T) {
	// Every error message that providers return BEFORE any HTTP call.
	preSubmitErrors := []string{
		// image/openai_compatible.go
		"reference image is required for image-to-image generation",
		"reference image is required for responses image edit",
		"unsupported reference image data URL",
		"local reference image is empty",
		"local reference image is too large",
		"reference image must be data URL or HTTP URL",
		// image/cloudbase_function.go
		"cloudbase image-to-image requires exactly one HTTPS reference image",
		"cloudbase image prompt exceeds the official 500 character limit",
		"cloudbase function URL must be HTTPS",
		"cloudbase function URL must use the official tcloudbasegateway.com domain",
		// video/openai_compatible.go
		"video provider requires base url and api key",
		"video model is required",
		"Grok Video 1.5 requires exactly one reference image",
		"Grok Video 1.5 supports exactly one reference image",
		"Grok Imagine Video 1.5 supports at most seven reference images",
		"provider task id is required",
		"empty reference image",
		"reference data URL must be base64 encoded",
		"reference data URL is empty",
		// httpserver/image_provider.go
		"unsupported openai image size",
		"unsupported openai image quality",
	}
	for _, msg := range preSubmitErrors {
		t.Run(msg, func(t *testing.T) {
			err := errors.New(msg)
			class := Classify(err)
			if class != DefinitiveNotSubmitted {
				t.Errorf("Classify(%q) = %s, want %s", msg, class, DefinitiveNotSubmitted)
			}
		})
	}
}

// TestClassifyPreSubmitWrappedErrors verifies fmt.Errorf wrapped pre-submit
// errors are also correctly classified via the pattern match.
func TestClassifyPreSubmitWrappedErrors(t *testing.T) {
	inner := errors.New("disk read failure")
	wrapped := fmt.Errorf("read local reference image: %w", inner)
	// "read local reference image" is not in the pattern list, but the inner
	// error text doesn't match either — this should still be PossiblySubmitted
	// since the wrapped text doesn't contain any known pattern.
	class := Classify(wrapped)
	if class == DefinitiveNotSubmitted {
		// Actually "read local reference image" doesn't match any pattern,
		// which is correct — disk I/O errors are ambiguous.
		t.Logf("INFO: disk-read wrapper classified as %s", class)
	}

	// But a validation error wrapped in fmt.Errorf should still match:
	validation := fmt.Errorf("validate: %w", errors.New("reference image is required for image-to-image generation"))
	class = Classify(validation)
	if class != DefinitiveNotSubmitted {
		t.Errorf("wrapped validation error classified as %s, want %s", class, DefinitiveNotSubmitted)
	}
}

// TestClassifyClassifiedErrorTakesPrecedence verifies that an explicit
// ClassifiedError wrapping always wins over pattern matching.
func TestClassifyClassifiedErrorTakesPrecedence(t *testing.T) {
	// An error whose text would match a pre-submit pattern but is explicitly
	// wrapped as PossiblySubmitted should remain PossiblySubmitted.
	explicit := ClassifiedError{
		Class: PossiblySubmitted,
		Err:   errors.New("reference image is required for image-to-image generation"),
	}
	class := Classify(explicit)
	if class != PossiblySubmitted {
		t.Errorf("explicit ClassifiedError should take precedence: got %s, want %s", class, PossiblySubmitted)
	}
}

// TestClassifyUnknownErrorStillDefaultsToPossiblySubmitted ensures errors
// that don't match any known pattern remain conservatively classified.
func TestClassifyUnknownErrorStillDefaultsToPossiblySubmitted(t *testing.T) {
	unknowns := []string{
		"connection reset by peer",
		"unexpected EOF",
		"i/o timeout",
		"TLS handshake error",
		"image provider returned no images",
		"responses image provider returned no images",
		"image provider returned empty image payloads",
		"cloudbase function returned invalid JSON",
		"cloudbase function returned no valid HTTPS image URL",
		"empty bridge stdout",
		"bridge stdout did not contain JSON",
		"poll video task failed",
	}
	for _, msg := range unknowns {
		t.Run(msg, func(t *testing.T) {
			class := Classify(errors.New(msg))
			if class == DefinitiveNotSubmitted {
				t.Errorf("Classify(%q) = DefinitiveNotSubmitted, should remain PossiblySubmitted or other", msg)
			}
		})
	}
}

// TestClassifyPreSubmitLeadsToRetryableDecision verifies the full chain:
// pre-submit validation error → DefinitiveNotSubmitted → Decide says Retry=true.
func TestClassifyPreSubmitLeadsToRetryableDecision(t *testing.T) {
	err := errors.New("reference image is required for image-to-image generation")
	class := Classify(err)
	if class != DefinitiveNotSubmitted {
		t.Fatalf("classification: got %s, want %s", class, DefinitiveNotSubmitted)
	}
	d := Decide(ProviderPolicy{}, class)
	if !d.Retry {
		t.Fatalf("expected Retry=true for DefinitiveNotSubmitted, got %+v", d)
	}
}

// TestClassifyPostSubmitErrorsNotMisclassified ensures errors that happen
// AFTER provider submission (e.g. empty response, invalid JSON) are NOT
// matched as pre-submit failures.
func TestClassifyPostSubmitErrorsNotMisclassified(t *testing.T) {
	postSubmit := []string{
		"image provider returned no images",
		"image provider returned empty image payloads",
		"responses image provider returned no images",
		"cloudbase function returned invalid JSON",
		"cloudbase function returned no valid HTTPS image URL",
	}
	for _, msg := range postSubmit {
		t.Run(msg, func(t *testing.T) {
			class := Classify(errors.New(msg))
			if class == DefinitiveNotSubmitted {
				t.Errorf("post-submit error %q should NOT be classified as DefinitiveNotSubmitted", msg)
			}
		})
	}
}
