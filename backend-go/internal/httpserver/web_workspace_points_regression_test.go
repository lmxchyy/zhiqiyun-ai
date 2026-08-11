package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestPointAccountTotalIncludesConsumedPoints(t *testing.T) {
	ctx := context.Background()
	store := newJSONStore(filepath.Join(t.TempDir(), "platform.json"))
	created, err := store.CreateAdminCustomer(adminCustomerMutation{Name: "Lifetime Points", Email: "lifetime-points@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	account, err := store.PointAccount(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	service := store.PersonalPointService()
	if _, err := service.Grant(ctx, PersonalPointGrantCommand{
		AccountID: account.ID, UserID: created.ID, Source: PointSourceAdminGift, Points: 100, IdempotencyKey: "lifetime-grant",
	}); err != nil {
		t.Fatal(err)
	}
	reserved, err := service.Reserve(ctx, PersonalPointReserveCommand{
		AccountID: account.ID, UserID: created.ID, BusinessType: "IMAGE_GENERATION", BusinessID: "task_lifetime_1",
		RequestedPoints: 30, IdempotencyKey: "lifetime-reserve",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Capture(ctx, PersonalPointCaptureCommand{
		AccountID: account.ID, UserID: created.ID, ReservationID: reserved.Reservation.ID, Points: 30, IdempotencyKey: "lifetime-capture",
	}); err != nil {
		t.Fatal(err)
	}
	// Lifetime total on /points/account comes from available + frozen + billing point_cost.
	if err := store.updateAdmin(func(data *adminPlatformData) error {
		data.BillingEvents = append(data.BillingEvents, adminBillingEvent{
			ID:         "bill_lifetime_1",
			UserID:     created.ID,
			TaskID:     "task_lifetime_1",
			PointCost:  30,
			Status:     "SUCCEEDED",
			OccurredAt: time.Now().UTC().Format(time.RFC3339Nano),
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	sessions := newLocalAuthSessions()
	if err := sessions.Put(ctx, "lifetime-token", created.ID, authSessionTTL); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/points/account", nil)
	req.Header.Set("Authorization", "Bearer lifetime-token")
	response := httptest.NewRecorder()
	api{store: store, sessions: sessions}.pointAccount(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}

	var payload struct {
		Account struct {
			Available    int64 `json:"available"`
			Frozen       int64 `json:"frozen"`
			Total        int64 `json:"total"`
			TotalUsed    int64 `json:"totalUsed"`
			TotalGranted int64 `json:"totalGranted"`
		} `json:"account"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Account.Available != 70 || payload.Account.Frozen != 0 {
		t.Fatalf("available/frozen=%d/%d want 70/0 body=%s", payload.Account.Available, payload.Account.Frozen, response.Body.String())
	}
	if payload.Account.Total != 100 {
		t.Fatalf("total=%d want lifetime 100 (available+consumed); body=%s", payload.Account.Total, response.Body.String())
	}
	if payload.Account.Total == payload.Account.Available {
		t.Fatalf("total collapsed to available; body=%s", response.Body.String())
	}
	if payload.Account.TotalUsed != 30 || payload.Account.TotalGranted != 100 {
		t.Fatalf("totalUsed/totalGranted=%d/%d want 30/100; body=%s", payload.Account.TotalUsed, payload.Account.TotalGranted, response.Body.String())
	}
}

func TestUserDashboardDefaultPayloadStaysSmallAndExposesTotalPoints(t *testing.T) {
	ctx := context.Background()
	store := newJSONStore(filepath.Join(t.TempDir(), "platform.json"))
	created, err := store.CreateAdminCustomer(adminCustomerMutation{Name: "Dashboard User", Email: "dashboard-user@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	account, err := store.PointAccount(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	service := store.PersonalPointService()
	if _, err := service.Grant(ctx, PersonalPointGrantCommand{
		AccountID: account.ID, UserID: created.ID, Source: PointSourceAdminGift, Points: 50, IdempotencyKey: "dashboard-grant",
	}); err != nil {
		t.Fatal(err)
	}
	reserved, err := service.Reserve(ctx, PersonalPointReserveCommand{
		AccountID: account.ID, UserID: created.ID, BusinessType: "IMAGE_GENERATION", BusinessID: "task_dashboard_cost",
		RequestedPoints: 30, IdempotencyKey: "dashboard-reserve",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Capture(ctx, PersonalPointCaptureCommand{
		AccountID: account.ID, UserID: created.ID, ReservationID: reserved.Reservation.ID, Points: 30, IdempotencyKey: "dashboard-capture",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.updateAdmin(func(data *adminPlatformData) error {
		data.BillingEvents = append(data.BillingEvents, adminBillingEvent{
			ID: "bill_dashboard_1", UserID: created.ID, TaskID: "task_dashboard_cost", PointCost: 30, Status: "SUCCEEDED",
			OccurredAt: time.Now().UTC().Format(time.RFC3339Nano),
		})
		tasks := make([]generationTask, 0, 45)
		assets := make([]asset, 0, 45)
		for i := 0; i < 45; i++ {
			id := "task_dash_" + strconv.Itoa(i)
			tasks = append(tasks, generationTask{ID: id, UserID: created.ID, Status: "SUCCEEDED", PointCost: 1, Type: "TEXT_TO_IMAGE"})
			assets = append(assets, asset{ID: "asset_dash_" + strconv.Itoa(i), UserID: created.ID, TaskID: id, MediaType: "IMAGE", URL: "https://example.test/" + strconv.Itoa(i) + ".png"})
		}
		data.GenerationTasks = append(data.GenerationTasks, tasks...)
		data.Assets = append(data.Assets, assets...)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	sessions := newLocalAuthSessions()
	if err := sessions.Put(ctx, "dashboard-token", created.ID, authSessionTTL); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/user/dashboard", nil)
	req.Header.Set("Authorization", "Bearer dashboard-token")
	response := httptest.NewRecorder()
	api{store: store, sessions: sessions}.userDashboard(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Summary struct {
			AvailablePoints int `json:"availablePoints"`
			TotalPoints     int `json:"totalPoints"`
		} `json:"summary"`
		RecentTasks  []generationTask `json:"recentTasks"`
		RecentAssets []asset          `json:"recentAssets"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Summary.AvailablePoints != 20 {
		t.Fatalf("availablePoints=%d want 20", payload.Summary.AvailablePoints)
	}
	if payload.Summary.TotalPoints != 50 {
		t.Fatalf("totalPoints=%d want 50", payload.Summary.TotalPoints)
	}
	if len(payload.RecentTasks) > 30 || len(payload.RecentAssets) > 30 {
		t.Fatalf("dashboard default payload too large: tasks=%d assets=%d", len(payload.RecentTasks), len(payload.RecentAssets))
	}
}

func TestUserOnlineImageDefaultListLimitsStayCapped(t *testing.T) {
	ctx := context.Background()
	store := newJSONStore(filepath.Join(t.TempDir(), "platform.json"))
	created, err := store.CreateAdminCustomer(adminCustomerMutation{Name: "Image User", Email: "image-user@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.updateAdmin(func(data *adminPlatformData) error {
		for i := 0; i < 55; i++ {
			id := "task_img_" + strconv.Itoa(i)
			data.GenerationTasks = append(data.GenerationTasks, generationTask{ID: id, UserID: created.ID, Status: "SUCCEEDED", Type: "TEXT_TO_IMAGE"})
			data.Assets = append(data.Assets, asset{ID: "asset_img_" + strconv.Itoa(i), UserID: created.ID, TaskID: id, MediaType: "IMAGE", URL: "https://example.test/img-" + strconv.Itoa(i) + ".png"})
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sessions := newLocalAuthSessions()
	if err := sessions.Put(ctx, "image-token", created.ID, authSessionTTL); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/user/online-image", nil)
	req.Header.Set("Authorization", "Bearer image-token")
	response := httptest.NewRecorder()
	api{store: store, sessions: sessions}.userOnlineImage(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Summary struct {
			TotalPoints *int `json:"totalPoints"`
		} `json:"summary"`
		RecentTasks []generationTask `json:"recentTasks"`
		Assets      []asset          `json:"assets"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.RecentTasks) > 40 || len(payload.Assets) > 40 {
		t.Fatalf("online-image default payload too large: tasks=%d assets=%d", len(payload.RecentTasks), len(payload.Assets))
	}
	if payload.Summary.TotalPoints == nil {
		t.Fatalf("summary.totalPoints missing; body=%s", response.Body.String())
	}
}
