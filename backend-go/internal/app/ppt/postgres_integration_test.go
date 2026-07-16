package ppt

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPostgresPPTTaskPersistenceConcurrencyAndLegacyMirror(t *testing.T) {
	dsn := os.Getenv("PPT_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("XIANZHI_TEST_DATABASE_URL")
	}
	if dsn == "" {
		t.Skip("PPT_TEST_DATABASE_URL is not configured")
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

	userID := "ppt_postgres_test_" + time.Now().UTC().Format("20060102150405.000000000")
	defer func() {
		_, _ = db.ExecContext(context.Background(), `delete from xz_ppt_tasks where user_id=$1`, userID)
	}()
	legacyPath := filepath.Join(t.TempDir(), "ppt-tasks.json")
	services := []*Service{NewPostgresService(db, legacyPath), NewPostgresService(db, legacyPath)}
	req := GenerateRequest{
		UserID: userID, Prompt: "Postgres PPT integration", SlideCount: 1, Theme: "techBlue", ImageSource: "ai",
		Outline: &Outline{Title: "Integration", Slides: []OutlineSlide{{Title: "Cover", Summary: "AI assistant", Layout: "cover", SlideType: "cover"}}},
	}

	type result struct {
		response GenerateResponse
		err      error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for _, service := range services {
		wg.Add(1)
		go func(service *Service) {
			defer wg.Done()
			response, err := service.GenerateWithConcurrency(req, 0, 1)
			results <- result{response: response, err: err}
		}(service)
	}
	wg.Wait()
	close(results)
	var taskID string
	var successes, conflicts int
	for item := range results {
		switch {
		case item.err == nil:
			successes++
			taskID = item.response.TaskID
		case errors.Is(item.err, ErrConcurrency):
			conflicts++
		default:
			t.Fatalf("unexpected generation error: %v", item.err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}

	task, err := services[1].GetTask(userID, taskID)
	if err != nil || len(task.Slides) != 1 {
		t.Fatalf("cross-instance task read failed: task=%#v err=%v", task, err)
	}
	plan := NormalizeVisualPlan(VisualPlan{VisualType: "illustration"}, VisualPlannerInput{SlideType: "cover", SlideTitle: task.Slides[0].Title})
	updated, err := services[1].CompleteSlideVisual(userID, taskID, task.Slides[0].ID, plan, VisualAsset{URL: "https://example.test/new-image.png", TaskID: "image_task_test", ModelName: "image_model_test", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Slides[0].ImageURL != "https://example.test/new-image.png" || updated.Slides[0].VisualTaskID != "image_task_test" || updated.Slides[0].VisualModelName != "image_model_test" || len(updated.Slides[0].VisualHistory) != 1 {
		t.Fatalf("atomic visual update failed: %#v", updated.Slides[0])
	}

	mirrored, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	legacyTasks := decodePersistedTasks(mirrored)
	if len(legacyTasks) == 0 {
		t.Fatal("postgres task was not mirrored to the legacy rollback file")
	}
}
