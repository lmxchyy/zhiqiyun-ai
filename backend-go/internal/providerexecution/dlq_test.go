package providerexecution

import "testing"

func TestDecideDLQActionNeverRetriesActiveOrUnknown(t *testing.T) {
	for _, status := range []Status{Submitting, Submitted, Processing, Unknown, Succeeded} {
		if got := DecideDLQAction(status, false); got != DLQManualReview {
			t.Fatalf("status %s action %s, want %s", status, got, DLQManualReview)
		}
	}
}

func TestDecideDLQActionTerminalFailures(t *testing.T) {
	if got := DecideDLQAction(Failed, false); got != DLQFail {
		t.Fatalf("failed action %s, want %s", got, DLQFail)
	}
	if got := DecideDLQAction(Failed, true); got != DLQExpire {
		t.Fatalf("expired action %s, want %s", got, DLQExpire)
	}
	if got := DecideDLQAction(Prepared, false); got != DLQManualReview {
		t.Fatalf("prepared action %s, want %s", got, DLQManualReview)
	}
}
