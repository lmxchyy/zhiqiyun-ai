package points

import "testing"

func TestValidateGrantCommandRequiresServerOwnedProvenance(t *testing.T) {
	tests := []struct {
		name string
		cmd  GrantCommand
		want error
	}{
		{name: "non positive amount", cmd: GrantCommand{Source: SourceAdminGift, Points: 0, IdempotencyKey: "gift-1"}, want: ErrInvalidGrant},
		{name: "missing idempotency", cmd: GrantCommand{Source: SourceAdminGift, Points: 1}, want: ErrInvalidGrant},
		{name: "unknown source", cmd: GrantCommand{AccountID: "account-1", UserID: "user-1", Source: Source("UNTRUSTED"), Points: 1, IdempotencyKey: "gift-1"}, want: ErrUnknownSource},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateGrantCommand(test.cmd); err != test.want {
				t.Fatalf("ValidateGrantCommand() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestValidateExpiryPolicyRequiresPublishedCalendarPolicyShape(t *testing.T) {
	valid := ExpiryPolicy{ID: "point_expiry_policy_v1", Version: 1, TimeZone: "Asia/Shanghai", DurationUnit: "CALENDAR_MONTH", DurationValue: 3}
	if err := ValidateExpiryPolicy(valid); err != nil {
		t.Fatalf("valid policy rejected: %v", err)
	}
	invalid := valid
	invalid.DurationUnit = "DAYS"
	if err := ValidateExpiryPolicy(invalid); err != ErrInvalidPolicy {
		t.Fatalf("invalid policy error = %v, want %v", err, ErrInvalidPolicy)
	}
}
