package providerexecution

import (
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
