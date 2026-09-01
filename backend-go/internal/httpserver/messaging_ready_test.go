package httpserver

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestReadyReportsAsyncStatusWithoutFailingLiveness(t *testing.T) {
	for _, status := range []string{"DISABLED", "CONNECTING", "READY", "DEGRADED", "STOPPED"} {
		recorder := httptest.NewRecorder()
		readyWithStatus(func() string { return status })(recorder, httptest.NewRequest("GET", "/api/v1/ready", nil))
		if recorder.Code != 200 {
			t.Fatalf("status %s HTTP=%d", status, recorder.Code)
		}
		var body map[string]string
		if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["ready"] != "true" || body["asyncMessaging"] != status {
			t.Fatalf("status %s body=%v", status, body)
		}
	}
}
