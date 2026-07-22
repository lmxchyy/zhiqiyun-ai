package httpserver

import (
	"database/sql"
	"os"
	"strconv"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestAdminIdentityPostgresQueries(t *testing.T) {
	databaseURL := os.Getenv("XIANZHI_IDENTITY_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("XIANZHI_IDENTITY_TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	store := &postgresStore{db: db, ready: true}
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	adminID, userID := "identity_query_admin_"+suffix, "identity_query_user_"+suffix
	seedIdentityChangeUser(t, db, adminID, "SUPER_ADMIN")
	seedIdentityChangeUser(t, db, userID, "MEMBER")
	preview, err := store.PreviewAdminIdentityChange(adminID, "SUPER_ADMIN", userID, identityChangePreviewRequest{Action: "UPGRADE", Method: identityMethodOnlyIdentity, TargetIdentity: "AGENT", Reason: "seed identity query fixtures"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ConfirmAdminIdentityChange(adminID, "SUPER_ADMIN", userID, identityChangeConfirmRequest{PreviewToken: preview.PreviewToken}); err != nil {
		t.Fatal(err)
	}
	profile, err := store.GetAdminIdentityProfile(userID)
	if err != nil {
		t.Fatal(err)
	}
	if profile.UserID != userID || len(profile.Identities) == 0 || profile.PrimaryIdentity == "" {
		t.Fatalf("unexpected identity profile: %+v", profile)
	}
	history, err := store.GetAdminIdentityHistory(userID)
	if err != nil {
		t.Fatal(err)
	}
	if history.UserID != userID || len(history.Identities) == 0 || len(history.ChangeRecords) == 0 {
		t.Fatalf("unexpected identity history: %+v", history)
	}
	if _, err := store.GetAdminCurrentRelationship(userID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetAdminRelationshipHistory(userID); err != nil {
		t.Fatal(err)
	}
	overview, err := store.GetAdminIdentityFinancialOverview(userID)
	if err != nil {
		t.Fatal(err)
	}
	if overview.UserID != userID || overview.Membership == nil || overview.Wallet == nil || overview.Token == nil || overview.Commission == nil {
		t.Fatalf("unexpected identity financial overview: %+v", overview)
	}
}
