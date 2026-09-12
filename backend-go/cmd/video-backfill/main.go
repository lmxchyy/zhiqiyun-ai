package main

import (
	"context"
	"database/sql"
	"encoding/json"
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

func main() {
	dryRun := flag.Bool("dry-run", true, "only probe URLs and do not write storage or database")
	limit := flag.Int("limit", 100, "maximum assets to process")
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
	files := storagecenter.NewService(storagecenter.NewPostgresRepository(db), storagecenter.S3ProviderFactory{AutoCreateBucket: cfg.StorageAutoCreateBucket}, storagecenter.OptionsFromConfig(cfg))
	rows, err := db.QueryContext(ctx, `select id,user_id,coalesce(tenant_id,'tenant_default'),coalesce(task_id,''),url,coalesce(metadata,'{}'::jsonb),raw from xz_assets where deleted_at is null and lower(media_type)='video' and coalesce(metadata->>'fileId','')='' and coalesce(url,'')<>'' order by created_at asc limit $1`, *limit)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	for rows.Next() {
		var item assetRow
		if err := rows.Scan(&item.ID, &item.UserID, &item.TenantID, &item.TaskID, &item.URL, &item.Metadata, &item.Raw); err != nil {
			panic(err)
		}
		status, err := probe(item.URL)
		if err != nil {
			fmt.Printf("%s|probe_error|%v\n", item.ID, err)
			continue
		}
		fmt.Printf("%s|upstream=%d\n", item.ID, status)
		if *dryRun || status < 200 || status >= 300 {
			continue
		}
		if err := persist(ctx, db, files, item); err != nil {
			fmt.Printf("%s|persist_error|%v\n", item.ID, err)
			continue
		}
		fmt.Printf("%s|persisted\n", item.ID)
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}
}

func probe(raw string) (int, error) {
	request, err := http.NewRequest(http.MethodGet, raw, nil)
	if err != nil {
		return 0, err
	}
	request = request.WithContext(context.Background())
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1))
	return response.StatusCode, nil
}

func persist(ctx context.Context, db *sql.DB, files *storagecenter.Service, item assetRow) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, item.URL, nil)
	if err != nil {
		return err
	}
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
	video, err := files.StoreObjectIdempotent(ctx, storagecenter.UploadInitInput{TenantID: item.TenantID, UserID: item.UserID, FileName: businessID + "-01.mp4", FileSize: info.Size(), MIMEType: "video/mp4", BusinessType: "generation_result", BusinessID: businessID, Visibility: "PRIVATE"}, mustOpen(videoPath))
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
	cover, err := files.StoreObjectIdempotent(ctx, storagecenter.UploadInitInput{TenantID: item.TenantID, UserID: item.UserID, FileName: businessID + "-01-cover.jpg", FileSize: int64(len(coverRaw)), MIMEType: "image/jpeg", BusinessType: "generation_result", BusinessID: businessID, Visibility: "PRIVATE"}, strings.NewReader(string(coverRaw)))
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
	_, err = db.ExecContext(ctx, `update xz_assets set url=$2,thumbnail_url=$3,metadata=$4::jsonb,raw=$5::jsonb,updated_at=now() where id=$1 and deleted_at is null and coalesce(metadata->>'fileId','')=''`, item.ID, "storage://"+video.FileID, "storage://"+cover.FileID, encodedMetadata, encodedRaw)
	return err
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
