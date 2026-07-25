package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestAgentInvitePostgresConcurrentAttributionAndIdempotency(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("XIANZHI_AGENT_INVITE_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("XIANZHI_AGENT_INVITE_TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}

	prefix := "invite_it_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	mobile := fmt.Sprintf("139%08d", time.Now().UnixNano()%100000000)
	if err := seedAgentInviteIntegrationAgents(ctx, db, prefix); err != nil {
		t.Fatal(err)
	}
	store := newPostgresPrimaryStore(db, "")
	publicInvite, err := store.ResolveAgentInvite(ctx, integrationInviteCode(prefix+"_a"))
	if err != nil {
		t.Fatal(err)
	}
	if publicInvite.DisplayName != "测试合作伙伴A" {
		t.Fatalf("public display name=%q want=%q", publicInvite.DisplayName, "测试合作伙伴A")
	}
	if strings.Contains(publicInvite.DisplayName, "并发代理商") {
		t.Fatalf("public invite leaked account name: %q", publicInvite.DisplayName)
	}
	invites := []agentInviteInfo{
		{
			AgentID: prefix + "_agent_a", InviterUserID: prefix + "_agent_user_a",
			TenantID: "tenant_default", InviteCode: integrationInviteCode(prefix + "_a"),
			DisplayName: "并发代理商A", AgentStatus: "ACTIVE", RegistrationOK: true,
		},
		{
			AgentID: prefix + "_agent_b", InviterUserID: prefix + "_agent_user_b",
			TenantID: "tenant_default", InviteCode: integrationInviteCode(prefix + "_b"),
			DisplayName: "并发代理商B", AgentStatus: "ACTIVE", RegistrationOK: true,
		},
	}
	inputs := []agentInviteRegistrationInput{
		{
			Mobile: mobile, IdempotencyKeyHash: stableSecretHash(prefix + "_request_a"),
			RegistrationEvent: prefix + "_event_a", Source: "integration_test", ClientFamily: "android",
			PlanID: "plan_free",
		},
		{
			Mobile: mobile, IdempotencyKeyHash: stableSecretHash(prefix + "_request_b"),
			RegistrationEvent: prefix + "_event_b", Source: "integration_test", ClientFamily: "android",
			PlanID: "plan_free",
		},
	}

	type outcome struct {
		index  int
		result agentInviteRegistrationResult
		err    error
	}
	start := make(chan struct{})
	outcomes := make(chan outcome, len(invites))
	var wg sync.WaitGroup
	for index := range invites {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			result, registerErr := store.RegisterAgentInvite(ctx, invites[index], inputs[index])
			outcomes <- outcome{index: index, result: result, err: registerErr}
		}(index)
	}
	close(start)
	wg.Wait()
	close(outcomes)

	successes := make([]outcome, 0, 1)
	locked := 0
	for item := range outcomes {
		switch {
		case item.err == nil:
			successes = append(successes, item)
		case errors.Is(item.err, errInviteAlreadyBoundOther):
			locked++
		default:
			t.Fatalf("unexpected concurrent registration error: %v", item.err)
		}
	}
	if len(successes) != 1 || locked != 1 {
		t.Fatalf("concurrent outcomes: successes=%d locked=%d", len(successes), locked)
	}
	winner := successes[0]
	if !winner.result.Created || winner.result.RelationshipStatus != "locked" {
		t.Fatalf("unexpected winning result: %+v", winner.result)
	}

	replay, err := store.RegisterAgentInvite(ctx, invites[winner.index], inputs[winner.index])
	if err != nil {
		t.Fatal(err)
	}
	if replay.Created || replay.UserID != winner.result.UserID || replay.RegistrationEventID != winner.result.RegistrationEventID {
		t.Fatalf("unexpected idempotent replay: %+v", replay)
	}

	var users, relationships, registeredEvents int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_users WHERE mobile=$1`, mobile).Scan(&users); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM xz_user_relationships relation
		JOIN xz_users users ON users.id=relation.user_id
		WHERE users.mobile=$1 AND relation.status='ACTIVE' AND relation.ended_at IS NULL
	`, mobile).Scan(&relationships); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM xz_agent_invite_events event
		JOIN xz_users users ON users.id=event.user_id
		WHERE users.mobile=$1 AND event.event_type='registered'
	`, mobile).Scan(&registeredEvents); err != nil {
		t.Fatal(err)
	}
	if users != 1 || relationships != 1 || registeredEvents != 1 {
		t.Fatalf("persisted counts: users=%d relationships=%d registered_events=%d", users, relationships, registeredEvents)
	}

	release := appRelease{
		ID: prefix + "_release", Platform: "android", Channel: prefix,
		VersionName: "9.9.9", VersionCode: 999999,
		APKURL: "https://cdn.example.test/android/releases/9.9.9/zhiqiyun-ai-9.9.9.apk",
		SHA256: strings.Repeat("a", 64), Status: "published",
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO xz_app_releases(
		  id,platform,channel,version_name,version_code,apk_url,sha256,status,published_at
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8,now())
	`, release.ID, release.Platform, release.Channel, release.VersionName, release.VersionCode,
		release.APKURL, release.SHA256, release.Status); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordAPKDownload(ctx, release, prefix+"_forged_registration", "android"); err != nil {
		t.Fatalf("forged registration attribution should degrade to anonymous: %v", err)
	}
	var unattributedDownloads int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM xz_apk_download_events
		WHERE release_id=$1 AND agent_id IS NULL AND user_id IS NULL AND invite_event_id IS NULL
	`, release.ID).Scan(&unattributedDownloads); err != nil {
		t.Fatal(err)
	}
	if unattributedDownloads != 1 {
		t.Fatalf("anonymous download events=%d want=1", unattributedDownloads)
	}
}

func seedAgentInviteIntegrationAgents(ctx context.Context, db *sql.DB, prefix string) error {
	for _, suffix := range []string{"a", "b"} {
		userID := prefix + "_agent_user_" + suffix
		agentID := prefix + "_agent_" + suffix
		inviteCode := integrationInviteCode(prefix + "_" + suffix)
		raw := fmt.Sprintf(`{"id":%q,"name":%q,"status":"ACTIVE","tenantId":"tenant_default"}`, userID, "并发代理商"+strings.ToUpper(suffix))
		if _, err := db.ExecContext(ctx, `
			INSERT INTO xz_users(id,email,name,role,status,created_at,updated_at,raw)
			VALUES($1,$2,$3,'AGENT','ACTIVE',now()::text,now()::text,$4::jsonb)
		`, userID, userID+"@example.test", "并发代理商"+strings.ToUpper(suffix), raw); err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO xz_channel_agents(id,user_id,level,status,invite_code,created_at,updated_at,raw)
			VALUES($1,$2,2,'ACTIVE',$3,now()::text,now()::text,
			       jsonb_build_object('inviteCode',$3::text,'inviteDisplayName',$4::text))
		`, agentID, userID, inviteCode, "测试合作伙伴"+strings.ToUpper(suffix)); err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO xz_agent_profiles(id,user_id,level,status,invite_code,created_at,updated_at,raw)
			VALUES($1,$2,2,'ACTIVE',$3,now()::text,now()::text,jsonb_build_object('inviteCode',$3::text))
		`, agentID, userID, inviteCode); err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO xz_user_business_identities(
			  id,tenant_id,user_id,identity_type,identity_status,commission_enabled,source_type,created_by
			) VALUES($1,'tenant_default',$2,'AGENT','ACTIVE',TRUE,'INTEGRATION_TEST','test')
		`, prefix+"_identity_"+suffix, userID); err != nil {
			return err
		}
	}
	return nil
}

func integrationInviteCode(value string) string {
	return strings.ToUpper(shortStableHash(value, 10))
}
