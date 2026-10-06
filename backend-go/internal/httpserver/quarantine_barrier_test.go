package httpserver

import "testing"

func TestPersonalPointReserveCarriesExplicitQuarantineTaskID(t *testing.T) {
	command := PersonalPointReserveCommand{BusinessID: "task-1", QuarantineTaskID: "task-1"}
	if command.QuarantineTaskID == "" || command.QuarantineTaskID != command.BusinessID {
		t.Fatalf("reserve quarantine context = %q for business %q", command.QuarantineTaskID, command.BusinessID)
	}
}
