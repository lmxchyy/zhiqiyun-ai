package operationcenter

import (
	"database/sql"
	"errors"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestJSONSnapshotRoundTripAndNullScan(t *testing.T) {
	original := JSONSnapshot{
		"ruleSetId": "rule_v1",
		"amounts":   []any{float64(100), float64(200)},
		"nested":    map[string]any{"operationCenterId": "oc_1"},
	}
	value, err := original.Value()
	if err != nil {
		t.Fatal(err)
	}
	var decoded JSONSnapshot
	if err := decoded.Scan(value); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, original) {
		t.Fatalf("snapshot round trip mismatch: got %#v want %#v", decoded, original)
	}
	if err := decoded.Scan(nil); err != nil {
		t.Fatal(err)
	}
	if len(decoded) != 0 {
		t.Fatalf("NULL JSON snapshot must scan as empty object, got %#v", decoded)
	}
}

func TestJSONSnapshotRejectsNonObjectJSON(t *testing.T) {
	var snapshot JSONSnapshot
	if err := snapshot.Scan([]byte(`[1,2,3]`)); !errors.Is(err, ErrInvalidJSONSnapshot) {
		t.Fatalf("expected invalid JSON snapshot error, got %v", err)
	}
}

func TestNullableStringPointerPreservesUUIDAndNull(t *testing.T) {
	uuid := "123e4567-e89b-12d3-a456-426614174000"
	if got := nullableStringPointer(sql.NullString{String: uuid, Valid: true}); got == nil || *got != uuid {
		t.Fatalf("nullable UUID-shaped ID mismatch: %v", got)
	}
	if got := nullableStringPointer(sql.NullString{}); got != nil {
		t.Fatalf("NULL value must remain nil, got %q", *got)
	}
}

func TestPostgresErrorClassification(t *testing.T) {
	tests := []struct {
		name string
		pg   *pgconn.PgError
		want error
	}{
		{"idempotency", &pgconn.PgError{Code: "23505", ConstraintName: "ux_review_idempotency"}, ErrIdempotencyConflict},
		{"unique", &pgconn.PgError{Code: "23505", ConstraintName: "refund_service_scope_key"}, ErrUniqueConflict},
		{"foreign-key", &pgconn.PgError{Code: "23503", ConstraintName: "refund_service_fk"}, ErrForeignKeyConflict},
		{"check", &pgconn.PgError{Code: "23514", ConstraintName: "refund_status_check"}, ErrConstraintViolation},
	}
	for _, test := range tests {
		err := mapPostgresStoreError(test.name, test.pg)
		if !errors.Is(err, test.want) {
			t.Fatalf("%s error mapped to %v, want %v", test.name, err, test.want)
		}
	}
}
