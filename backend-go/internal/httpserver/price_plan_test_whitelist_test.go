package httpserver

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPricePlanTestWhitelistComputedStatus(t *testing.T) {
	now := time.Date(2026, time.July, 28, 1, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)

	tests := []struct {
		name       string
		stored     string
		effective  *time.Time
		expires    *time.Time
		wantStatus string
	}{
		{name: "future active row is pending", stored: pricePlanWhitelistLifecycleActive, effective: &future, wantStatus: pricePlanWhitelistStatusPending},
		{name: "currently effective row is active", stored: pricePlanWhitelistLifecycleActive, effective: &past, expires: &future, wantStatus: pricePlanWhitelistStatusActive},
		{name: "temporally elapsed active row is expired", stored: pricePlanWhitelistLifecycleActive, effective: &past, expires: &past, wantStatus: pricePlanWhitelistStatusExpired},
		{name: "stored expired remains expired", stored: pricePlanWhitelistLifecycleExpired, wantStatus: pricePlanWhitelistStatusExpired},
		{name: "stored disabled remains disabled", stored: pricePlanWhitelistLifecycleDisabled, expires: &past, wantStatus: pricePlanWhitelistStatusDisabled},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			item := pricePlanTestWhitelistView{
				lifecycleStatus: test.stored,
				ValidFrom:       test.effective,
				ValidUntil:      test.expires,
			}
			item.deriveStatus(now)
			if item.Status != test.wantStatus {
				t.Fatalf("status=%q want=%q", item.Status, test.wantStatus)
			}
		})
	}
}

func TestPricePlanTestWhitelistComputedStatusUsesInstantAcrossTimezoneOffset(t *testing.T) {
	nowUTC := time.Date(2026, time.July, 28, 1, 0, 0, 0, time.UTC)
	china := time.FixedZone("UTC+8", 8*60*60)
	exactBoundaryInChina := time.Date(2026, time.July, 28, 9, 0, 0, 0, china)

	activeAtInclusiveStart := pricePlanTestWhitelistView{
		lifecycleStatus: pricePlanWhitelistLifecycleActive,
		ValidFrom:       &exactBoundaryInChina,
	}
	activeAtInclusiveStart.deriveStatus(nowUTC)
	if activeAtInclusiveStart.Status != pricePlanWhitelistStatusActive {
		t.Fatalf("start boundary status=%s want ACTIVE", activeAtInclusiveStart.Status)
	}

	expiredAtExclusiveEnd := pricePlanTestWhitelistView{
		lifecycleStatus: pricePlanWhitelistLifecycleActive,
		ValidUntil:      &exactBoundaryInChina,
	}
	expiredAtExclusiveEnd.deriveStatus(nowUTC)
	if expiredAtExclusiveEnd.Status != pricePlanWhitelistStatusExpired {
		t.Fatalf("end boundary status=%s want EXPIRED", expiredAtExclusiveEnd.Status)
	}
}

func TestPricePlanTestWhitelistViewUsesPublicValidityNames(t *testing.T) {
	now := time.Date(2026, time.July, 28, 1, 0, 0, 0, time.UTC)
	raw, err := json.Marshal(pricePlanTestWhitelistView{ValidFrom: &now, ValidUntil: &now})
	if err != nil {
		t.Fatal(err)
	}
	payload := string(raw)
	if !strings.Contains(payload, `"validFrom"`) || !strings.Contains(payload, `"validUntil"`) {
		t.Fatalf("public validity fields missing: %s", payload)
	}
	if strings.Contains(payload, `"effectiveAt"`) || strings.Contains(payload, `"expiresAt"`) {
		t.Fatalf("database validity names leaked into API: %s", payload)
	}
}

func TestValidatePricePlanTestWhitelistCreate(t *testing.T) {
	now := time.Date(2026, time.July, 28, 1, 0, 0, 0, time.UTC)
	later := now.Add(time.Hour)
	createRevision := int64(0)
	invalidRevision := int64(1)

	tests := []struct {
		name     string
		mutation pricePlanTestWhitelistCreateMutation
		actorID  string
		wantCode string
	}{
		{name: "actor is required", mutation: pricePlanTestWhitelistCreateMutation{Revision: &createRevision, UserID: "user_1", Reason: "pilot member", ChangeReason: "controlled test"}, wantCode: "FORBIDDEN"},
		{name: "revision is required", actorID: "admin_1", mutation: pricePlanTestWhitelistCreateMutation{UserID: "user_1", Reason: "pilot member", ChangeReason: "controlled test"}, wantCode: "REVISION_REQUIRED"},
		{name: "create revision must be zero", actorID: "admin_1", mutation: pricePlanTestWhitelistCreateMutation{Revision: &invalidRevision, UserID: "user_1", Reason: "pilot member", ChangeReason: "controlled test"}, wantCode: "WHITELIST_CREATE_REVISION_INVALID"},
		{name: "user is required", actorID: "admin_1", mutation: pricePlanTestWhitelistCreateMutation{Revision: &createRevision, Reason: "pilot member", ChangeReason: "controlled test"}, wantCode: "WHITELIST_USER_REQUIRED"},
		{name: "qualification reason is required", actorID: "admin_1", mutation: pricePlanTestWhitelistCreateMutation{Revision: &createRevision, UserID: "user_1", ChangeReason: "controlled test"}, wantCode: "WHITELIST_REASON_REQUIRED"},
		{name: "change reason is required", actorID: "admin_1", mutation: pricePlanTestWhitelistCreateMutation{Revision: &createRevision, UserID: "user_1", Reason: "pilot member"}, wantCode: "REASON_REQUIRED"},
		{name: "validity must be ordered", actorID: "admin_1", mutation: pricePlanTestWhitelistCreateMutation{Revision: &createRevision, UserID: "user_1", Reason: "pilot member", ValidFrom: &later, ValidUntil: &now, ChangeReason: "controlled test"}, wantCode: "WHITELIST_VALIDITY_INVALID"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validatePricePlanTestWhitelistCreate(&test.mutation, test.actorID)
			if code := businessCode(err); code != test.wantCode {
				t.Fatalf("code=%q err=%v want=%q", code, err, test.wantCode)
			}
		})
	}

	mutation := pricePlanTestWhitelistCreateMutation{Revision: &createRevision, UserID: "  user_1  ", Reason: "  pilot member  ", ValidFrom: &now, ValidUntil: &later, ChangeReason: "  controlled test  "}
	if err := validatePricePlanTestWhitelistCreate(&mutation, "admin_1"); err != nil {
		t.Fatal(err)
	}
	if mutation.UserID != "user_1" || mutation.Reason != "pilot member" || mutation.ChangeReason != "controlled test" {
		t.Fatalf("mutation was not normalized: %+v", mutation)
	}
}

func TestValidatePricePlanTestWhitelistUpdateAndDisable(t *testing.T) {
	now := time.Date(2026, time.July, 28, 1, 0, 0, 0, time.UTC)
	later := now.Add(time.Hour)
	revision := int64(3)

	updateTests := []struct {
		name     string
		mutation pricePlanTestWhitelistUpdateMutation
		wantCode string
	}{
		{name: "revision is required", mutation: pricePlanTestWhitelistUpdateMutation{ValidUntil: &later, ChangeReason: "extend test"}, wantCode: "REVISION_REQUIRED"},
		{name: "change reason is required", mutation: pricePlanTestWhitelistUpdateMutation{Revision: &revision, ValidUntil: &later}, wantCode: "REASON_REQUIRED"},
		{name: "a temporal field is required", mutation: pricePlanTestWhitelistUpdateMutation{Revision: &revision, ChangeReason: "empty update"}, wantCode: "WHITELIST_MUTATION_REQUIRED"},
		{name: "blank qualification reason is rejected", mutation: pricePlanTestWhitelistUpdateMutation{Revision: &revision, Reason: stringPointer("  "), ChangeReason: "bad reason"}, wantCode: "WHITELIST_REASON_REQUIRED"},
		{name: "set and clear conflict", mutation: pricePlanTestWhitelistUpdateMutation{Revision: &revision, ValidFrom: &now, ClearValidFrom: true, ChangeReason: "bad patch"}, wantCode: "WHITELIST_VALIDITY_MUTATION_CONFLICT"},
		{name: "validity must be ordered", mutation: pricePlanTestWhitelistUpdateMutation{Revision: &revision, ValidFrom: &later, ValidUntil: &now, ChangeReason: "bad window"}, wantCode: "WHITELIST_VALIDITY_INVALID"},
	}
	for _, test := range updateTests {
		t.Run(test.name, func(t *testing.T) {
			err := validatePricePlanTestWhitelistUpdate(test.mutation, "admin_1")
			if code := businessCode(err); code != test.wantCode {
				t.Fatalf("code=%q err=%v want=%q", code, err, test.wantCode)
			}
		})
	}

	if err := validatePricePlanTestWhitelistUpdate(pricePlanTestWhitelistUpdateMutation{
		Revision: &revision, ValidUntil: &later, ChangeReason: "extend controlled test",
	}, "admin_1"); err != nil {
		t.Fatal(err)
	}
	if err := validatePricePlanTestWhitelistUpdate(pricePlanTestWhitelistUpdateMutation{
		Revision: &revision, Reason: stringPointer("pilot cohort 2"), ChangeReason: "correct qualification",
	}, "admin_1"); err != nil {
		t.Fatal(err)
	}
	if err := validatePricePlanTestWhitelistDisable(pricePlanTestWhitelistDisableMutation{
		Revision: &revision, ChangeReason: "stop controlled test",
	}, "admin_1"); err != nil {
		t.Fatal(err)
	}
	if code := businessCode(validatePricePlanTestWhitelistDisable(pricePlanTestWhitelistDisableMutation{
		ChangeReason: "stop controlled test",
	}, "admin_1")); code != "REVISION_REQUIRED" {
		t.Fatalf("disable code=%q want REVISION_REQUIRED", code)
	}
}

func TestPricePlanTestWhitelistValidityErrorUsesPublicFieldNames(t *testing.T) {
	validFrom := time.Date(2026, time.July, 28, 2, 0, 0, 0, time.UTC)
	validUntil := validFrom.Add(-time.Minute)
	err := validatePricePlanTestWhitelistValidity(&validFrom, &validUntil)
	if err == nil || err.Error() != "validUntil must be after validFrom" {
		t.Fatalf("error=%v", err)
	}
}

func stringPointer(value string) *string { return &value }

func businessCode(err error) string {
	if err == nil {
		return ""
	}
	var coded interface{ BusinessCode() string }
	if errors.As(err, &coded) {
		return coded.BusinessCode()
	}
	return ""
}
