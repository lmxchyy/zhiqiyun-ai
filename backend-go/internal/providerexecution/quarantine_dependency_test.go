package providerexecution

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func TestRejectQuarantineTypedNilDependencies(t *testing.T) {
	var db *sql.DB
	var tx *sql.Tx
	for name, q := range map[string]quarantineQuerier{"nil": nil, "typed DB": db, "typed tx": tx} {
		if err := RejectTask(context.Background(), q, "fixture", "nil"); !errors.Is(err, ErrQuarantineBarrierUnavailable) {
			t.Fatalf("%s failed open: %v", name, err)
		}
	}
}
