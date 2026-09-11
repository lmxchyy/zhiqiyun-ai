package httpserver

import (
	"testing"
	"time"
)

func TestParseReaperDuration(t *testing.T) {
	if got := parseReaperDuration("250ms", time.Second); got != 250*time.Millisecond {
		t.Fatalf("duration=%s", got)
	}
	if got := parseReaperDuration("bad", time.Second); got != time.Second {
		t.Fatalf("invalid duration=%s", got)
	}
	if got := parseReaperDuration("0s", time.Second); got != time.Second {
		t.Fatalf("zero duration=%s", got)
	}
}
