package providerexecution

import (
	"testing"
	"time"
)

func quarantineRecord() QuarantineRecord {
	generation := int64(2)
	return QuarantineRecord{
		ExecutionID: 14, TaskID: "task_000234", Attempt: 1, Generation: &generation,
		SnapshotSHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		EvidenceSHA256: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
		ApprovalID:     "approval-test", ReleaseSHA: "0123456789abcdef0123456789abcdef01234567",
		NotBefore: time.Unix(10, 0).UTC(), ExpiresAt: time.Unix(100, 0).UTC(),
	}
}

func TestDecideQuarantineFailClosed(t *testing.T) {
	now := time.Unix(20, 0).UTC()
	record := quarantineRecord()
	query := QuarantineQuery{ExecutionID: record.ExecutionID, TaskID: record.TaskID, Attempt: record.Attempt, Generation: record.Generation, ReleaseSHA: record.ReleaseSHA, EvidenceSHA256: record.EvidenceSHA256, Now: now}
	if block, reason := DecideQuarantine(query, nil, false); block || reason != "none" {
		t.Fatalf("no row must allow normal path, block=%v reason=%s", block, reason)
	}
	cases := []struct {
		name    string
		mutate  func(*QuarantineQuery, *QuarantineRecord)
		records int
		failed  bool
		reason  string
	}{
		{name: "query failure", failed: true, reason: "query_failed"},
		{name: "valid row still blocks", reason: "quarantined"},
		{name: "expired", mutate: func(_ *QuarantineQuery, record *QuarantineRecord) { record.ExpiresAt = now }, reason: "expired"},
		{name: "generation", mutate: func(query *QuarantineQuery, _ *QuarantineRecord) {
			next := *query.Generation + 1
			query.Generation = &next
		}, reason: "generation_mismatch"},
		{name: "release", mutate: func(query *QuarantineQuery, _ *QuarantineRecord) {
			query.ReleaseSHA = "ffffffffffffffffffffffffffffffffffffffff"
		}, reason: "release_mismatch"},
		{name: "evidence", mutate: func(query *QuarantineQuery, _ *QuarantineRecord) {
			query.EvidenceSHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
		}, reason: "evidence_mismatch"},
		{name: "duplicate", records: 2, reason: "duplicate"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q, rec := query, record
			if tc.mutate != nil {
				tc.mutate(&q, &rec)
			}
			records := []QuarantineRecord{rec}
			if tc.records == 2 {
				records = append(records, rec)
			}
			if tc.failed {
				records = nil
			}
			block, reason := DecideQuarantine(q, records, tc.failed)
			if !block || reason != tc.reason {
				t.Fatalf("block=%v reason=%s, want block reason %s", block, reason, tc.reason)
			}
		})
	}
}
