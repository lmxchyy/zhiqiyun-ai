package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"xianzhi-ai/backend-go/internal/config"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

type assetRow struct {
	ID, UserID, TenantID, TaskID, URL string
	Metadata                          []byte
	Raw                               []byte
}

type singleAssetRecord struct {
	ID        string
	UserID    string
	TenantID  string
	TaskID    string
	URL       string
	Metadata  []byte
	Raw       []byte
	DeletedAt *string
	MediaType string
}

type backfillStore interface {
	GetAsset(ctx context.Context, id string) (*singleAssetRecord, error)
	ListUnmanagedVideoAssets(ctx context.Context, limit int) ([]assetRow, error)
	UpdateAssetPersisted(ctx context.Context, id string, url string, thumbURL string, metadata []byte, raw []byte) (int64, error)
}

type sqlBackfillStore struct {
	db *sql.DB
}

func newSQLBackfillStore(db *sql.DB) *sqlBackfillStore {
	return &sqlBackfillStore{db: db}
}

func (s *sqlBackfillStore) GetAsset(ctx context.Context, id string) (*singleAssetRecord, error) {
	query := `SELECT id, user_id, coalesce(tenant_id, 'tenant_default'), coalesce(task_id, ''), coalesce(url, ''), coalesce(metadata, '{}'::jsonb), coalesce(raw, '{}'::jsonb), deleted_at::text, coalesce(media_type, '') FROM xz_assets WHERE id = $1`
	var (
		item      singleAssetRecord
		deletedAt sql.NullString
	)
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&item.ID, &item.UserID, &item.TenantID, &item.TaskID, &item.URL, &item.Metadata, &item.Raw, &deletedAt, &item.MediaType,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if deletedAt.Valid && strings.TrimSpace(deletedAt.String) != "" {
		val := deletedAt.String
		item.DeletedAt = &val
	}
	return &item, nil
}

func (s *sqlBackfillStore) ListUnmanagedVideoAssets(ctx context.Context, limit int) ([]assetRow, error) {
	if limit <= 0 {
		limit = 100
	}
	query := `SELECT id, user_id, coalesce(tenant_id, 'tenant_default'), coalesce(task_id, ''), url, coalesce(metadata, '{}'::jsonb), raw FROM xz_assets WHERE deleted_at IS NULL AND lower(media_type) = 'video' AND coalesce(metadata->>'fileId', '') = '' AND coalesce(url, '') <> '' ORDER BY created_at ASC LIMIT $1`
	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []assetRow
	for rows.Next() {
		var item assetRow
		if err := rows.Scan(&item.ID, &item.UserID, &item.TenantID, &item.TaskID, &item.URL, &item.Metadata, &item.Raw); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *sqlBackfillStore) UpdateAssetPersisted(ctx context.Context, id string, url string, thumbURL string, metadata []byte, raw []byte) (int64, error) {
	query := `UPDATE xz_assets SET url = $2, thumbnail_url = $3, metadata = $4::jsonb, raw = $5::jsonb, updated_at = now() WHERE id = $1 AND deleted_at IS NULL AND coalesce(metadata->>'fileId', '') = ''`
	res, err := s.db.ExecContext(ctx, query, id, url, thumbURL, metadata, raw)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

type Options struct {
	DryRun  bool
	Limit   int
	AssetID string
	Probe   func(string) (int, error)
	Persist func(context.Context, backfillStore, *storagecenter.Service, assetRow) error
	Out     io.Writer
}

func main() {
	dryRun := flag.Bool("dry-run", true, "only probe URLs and do not write storage or database")
	limit := flag.Int("limit", 100, "maximum assets to process in batch mode")
	assetID := flag.String("asset-id", "", "optional single asset ID to target")
	flag.Parse()

	cfg := config.Load()
	ctx := context.Background()
	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		panic(err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		panic(err)
	}
	files := storagecenter.NewService(
		storagecenter.NewPostgresRepository(db),
		storagecenter.S3ProviderFactory{AutoCreateBucket: cfg.StorageAutoCreateBucket},
		storagecenter.OptionsFromConfig(cfg),
	)

	opts := Options{
		DryRun:  *dryRun,
		Limit:   *limit,
		AssetID: *assetID,
		Out:     os.Stdout,
	}

	store := newSQLBackfillStore(db)
	if err := runBackfill(ctx, store, files, opts); err != nil {
		fmt.Fprintf(os.Stderr, "video-backfill failed: %v\n", err)
		os.Exit(1)
	}
}

func runBackfill(ctx context.Context, store backfillStore, files *storagecenter.Service, opts Options) error {
	if opts.Out == nil {
		opts.Out = os.Stdout
	}
	if opts.Probe == nil {
		opts.Probe = probe
	}
	if opts.Persist == nil {
		opts.Persist = persist
	}

	targetAssetID := strings.TrimSpace(opts.AssetID)
	if targetAssetID != "" {
		return runSingleAsset(ctx, store, files, targetAssetID, opts)
	}
	return runBatchAssets(ctx, store, files, opts)
}

func runSingleAsset(ctx context.Context, store backfillStore, files *storagecenter.Service, assetID string, opts Options) error {
	item, err := store.GetAsset(ctx, assetID)
	if err != nil {
		return fmt.Errorf("query asset %s: %w", assetID, err)
	}
	if item == nil {
		fmt.Fprintf(opts.Out, "%s|not_found\n", assetID)
		return nil
	}

	if item.DeletedAt != nil && strings.TrimSpace(*item.DeletedAt) != "" {
		fmt.Fprintf(opts.Out, "%s|skipped_deleted\n", assetID)
		return nil
	}

	if !strings.EqualFold(strings.TrimSpace(item.MediaType), "video") {
		fmt.Fprintf(opts.Out, "%s|skipped_not_video\n", assetID)
		return nil
	}

	var meta map[string]any
	if len(item.Metadata) > 0 {
		_ = json.Unmarshal(item.Metadata, &meta)
	}
	if meta == nil {
		meta = map[string]any{}
	}

	fileID := strings.TrimSpace(stringValue(meta["fileId"]))
	if fileID == "" {
		fileID = strings.TrimSpace(stringValue(meta["storageFileId"]))
	}
	storageManaged := boolValue(meta["storageManaged"])

	rawURL := strings.TrimSpace(item.URL)
	if fileID != "" || storageManaged || strings.HasPrefix(strings.ToLower(rawURL), "storage://") {
		fmt.Fprintf(opts.Out, "%s|already_persisted\n", assetID)
		return nil
	}

	if rawURL == "" || strings.HasPrefix(rawURL, "upstream returned") {
		fmt.Fprintf(opts.Out, "%s|skipped_invalid_url\n", assetID)
		return nil
	}

	status, err := opts.Probe(rawURL)
	if err != nil {
		fmt.Fprintf(opts.Out, "%s|probe_error|%v\n", assetID, err)
		return nil
	}
	fmt.Fprintf(opts.Out, "%s|upstream=%d\n", assetID, status)
	if status < 200 || status >= 300 {
		fmt.Fprintf(opts.Out, "%s|skipped_upstream_status=%d\n", assetID, status)
		return nil
	}

	if opts.DryRun {
		fmt.Fprintf(opts.Out, "%s|dry_run_recoverable\n", assetID)
		return nil
	}

	row := assetRow{
		ID:       item.ID,
		UserID:   item.UserID,
		TenantID: item.TenantID,
		TaskID:   item.TaskID,
		URL:      rawURL,
		Metadata: item.Metadata,
		Raw:      item.Raw,
	}

	if err := opts.Persist(ctx, store, files, row); err != nil {
		fmt.Fprintf(opts.Out, "%s|persist_error|%v\n", assetID, err)
		return err
	}
	fmt.Fprintf(opts.Out, "%s|persisted\n", assetID)
	return nil
}

func runBatchAssets(ctx context.Context, store backfillStore, files *storagecenter.Service, opts Options) error {
	limit := opts.Limit
	if limit <= 0 {
		limit = 100
	}

	items, err := store.ListUnmanagedVideoAssets(ctx, limit)
	if err != nil {
		return err
	}

	for _, item := range items {
		status, err := opts.Probe(item.URL)
		if err != nil {
			fmt.Fprintf(opts.Out, "%s|probe_error|%v\n", item.ID, err)
			continue
		}
		fmt.Fprintf(opts.Out, "%s|upstream=%d\n", item.ID, status)
		if status < 200 || status >= 300 {
			continue
		}
		if opts.DryRun {
			fmt.Fprintf(opts.Out, "%s|dry_run_recoverable\n", item.ID)
			continue
		}
		if err := opts.Persist(ctx, store, files, item); err != nil {
			fmt.Fprintf(opts.Out, "%s|persist_error|%v\n", item.ID, err)
			continue
		}
		fmt.Fprintf(opts.Out, "%s|persisted\n", item.ID)
	}
	return nil
}

const backfillUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

func probe(raw string) (int, error) {
	request, err := http.NewRequest(http.MethodGet, raw, nil)
	if err != nil {
		return 0, err
	}
	request.Header.Set("User-Agent", backfillUserAgent)
	request.Header.Set("Range", "bytes=0-0")
	request = request.WithContext(context.Background())
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1))
	return response.StatusCode, nil
}

func persist(ctx context.Context, store backfillStore, files *storagecenter.Service, item assetRow) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, item.URL, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", backfillUserAgent)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("upstream status %d", response.StatusCode)
	}
	tmp, err := os.CreateTemp("", "video-backfill-*.mp4")
	if err != nil {
		return err
	}
	videoPath := tmp.Name()
	defer os.Remove(videoPath)
	if _, err = io.Copy(tmp, io.LimitReader(response.Body, 100<<20)); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	info, err := os.Stat(videoPath)
	if err != nil {
		return err
	}
	businessID := item.TaskID
	if businessID == "" {
		businessID = item.ID
	}
	video, err := files.StoreObjectIdempotent(ctx, storagecenter.UploadInitInput{
		TenantID:     item.TenantID,
		UserID:       item.UserID,
		FileName:     businessID + "-01.mp4",
		FileSize:     info.Size(),
		MIMEType:     "video/mp4",
		BusinessType: "generation_result",
		BusinessID:   businessID,
		Visibility:   "PRIVATE",
	}, mustOpen(videoPath))
	if err != nil {
		return err
	}
	coverPath := videoPath + ".jpg"
	defer os.Remove(coverPath)
	ffmpeg := os.Getenv("SMARTVIDEO_FFMPEG_PATH")
	if ffmpeg == "" {
		ffmpeg = "ffmpeg"
	}
	if output, err := exec.CommandContext(ctx, ffmpeg, "-y", "-i", videoPath, "-frames:v", "1", "-vf", "scale='min(640,iw)':-2", "-q:v", "3", coverPath).CombinedOutput(); err != nil {
		return fmt.Errorf("extract cover: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	coverRaw, err := os.ReadFile(coverPath)
	if err != nil {
		return err
	}
	cover, err := files.StoreObjectIdempotent(ctx, storagecenter.UploadInitInput{
		TenantID:     item.TenantID,
		UserID:       item.UserID,
		FileName:     businessID + "-01-cover.jpg",
		FileSize:     int64(len(coverRaw)),
		MIMEType:     "image/jpeg",
		BusinessType: "generation_result",
		BusinessID:   businessID,
		Visibility:   "PRIVATE",
	}, strings.NewReader(string(coverRaw)))
	if err != nil {
		return err
	}
	metadata := map[string]any{}
	if err := json.Unmarshal(item.Metadata, &metadata); err != nil {
		return err
	}
	metadata["fileId"] = video.FileID
	metadata["storageFileId"] = video.FileID
	metadata["storageManaged"] = true
	metadata["storageTenantId"] = video.TenantID
	metadata["storageBucket"] = video.Bucket
	metadata["storageObjectKey"] = video.ObjectKey
	metadata["coverFileId"] = cover.FileID
	metadata["contentType"] = "video/mp4"
	raw := map[string]any{}
	if err := json.Unmarshal(item.Raw, &raw); err != nil {
		return err
	}
	raw["url"] = "storage://" + video.FileID
	raw["thumbnailUrl"] = "storage://" + cover.FileID
	raw["metadata"] = metadata
	encodedRaw, _ := json.Marshal(raw)
	encodedMetadata, _ := json.Marshal(metadata)
	_, err = store.UpdateAssetPersisted(ctx, item.ID, "storage://"+video.FileID, "storage://"+cover.FileID, encodedMetadata, encodedRaw)
	return err
}

func stringValue(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func boolValue(v any) bool {
	if v == nil {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	if s, ok := v.(string); ok {
		return strings.EqualFold(strings.TrimSpace(s), "true")
	}
	return false
}

func mustOpen(path string) io.Reader {
	file, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	return file
}

var _ = filepath.Base
var _ = time.Second
