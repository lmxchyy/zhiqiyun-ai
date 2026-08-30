package main

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

type backupArtifact struct {
	path      string
	metaPath  string
	metaBytes []byte
	bytes     int64
	sha256    string
	key       string
	category  string
}

type result struct {
	Status       string `json:"status"`
	Message      string `json:"message,omitempty"`
	ObjectKey    string `json:"object_key,omitempty"`
	OffsitePath  string `json:"offsite_path,omitempty"`
	LocalBytes   int64  `json:"local_bytes,omitempty"`
	RemoteBytes  int64  `json:"remote_bytes,omitempty"`
	LocalSHA256  string `json:"local_sha256,omitempty"`
	RemoteSHA256 string `json:"remote_sha256,omitempty"`
	RemoteETag   string `json:"remote_etag,omitempty"`
	Verification string `json:"verification,omitempty"`
	Exists       bool   `json:"remote_exists,omitempty"`
	SizeMatch    bool   `json:"remote_size_match,omitempty"`
	SHA256Match  bool   `json:"remote_sha256_match,omitempty"`
}

type remoteVerification struct {
	Status      string
	Message     string
	Exists      bool
	SizeMatch   bool
	SHA256Match bool
}

type metadataProvider interface {
	storagecenter.Provider
	PutObjectWithMetadata(context.Context, string, io.Reader, int64, string, map[string]string) (storagecenter.ObjectMetadata, error)
}

var errRemoteConflict = errors.New("remote object conflict")

func main() {
	resultValue, code := run()
	if os.Getenv("BACKUP_UPLOADER_JSON") == "1" {
		_ = json.NewEncoder(os.Stdout).Encode(resultValue)
	} else {
		fmt.Printf("%s: %s\n", resultValue.Status, resultValue.Message)
		if resultValue.ObjectKey != "" {
			fmt.Printf("object_key: %s\n", resultValue.ObjectKey)
		}
	}
	os.Exit(code)
}

func run() (result, int) {
	flags := flag.NewFlagSet("backup-uploader", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	root := flags.String("root", "/var/lib/zhiqiyun/backups/postgres", "backup root")
	file := flags.String("file", "", "backup file")
	configID := flags.String("storage-config-id", "", "dedicated backup storage config id")
	downloadTo := flags.String("download-to", "", "optional isolated temporary download path")
	upload := flags.Bool("upload", false, "upload exactly one backup")
	jsonOutput := flags.Bool("json", false, "emit JSON")
	verifyOnly := flags.Bool("verify-only", false, "perform read-only remote HEAD verification")
	remoteKey := flags.String("remote-key", "", "remote object key for --verify-only")
	expectedSize := flags.Int64("expected-size", -1, "expected remote object size for --verify-only")
	expectedSHA256 := flags.String("expected-sha256", "", "expected remote SHA256 metadata for --verify-only")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return result{Status: "INVALID_ARGUMENT", Message: err.Error()}, 2
	}
	if *jsonOutput {
		_ = os.Setenv("BACKUP_UPLOADER_JSON", "1")
	}
	if strings.TrimSpace(*configID) == "" || (!*verifyOnly && strings.TrimSpace(*file) == "") || (*verifyOnly && (strings.TrimSpace(*remoteKey) == "" || *expectedSize < 0 || strings.TrimSpace(*expectedSHA256) == "")) {
		return result{Status: "BACKUP_STORAGE_CONFIG_NOT_FOUND", Message: "--file and --storage-config-id are required"}, 1
	}

	ctx := context.Background()
	db, cfg, provider, err := openBackupProvider(ctx, *configID)
	if err != nil {
		return result{Status: statusForConfigError(err), Message: err.Error()}, 1
	}
	defer db.Close()
	if *verifyOnly {
		return verifyRemote(ctx, provider, *remoteKey, *expectedSize, *expectedSHA256)
	}

	artifact, err := loadArtifact(*root, *file, cfg)
	if err != nil {
		return result{Status: "LOCAL_BACKUP_INVALID", Message: err.Error()}, 1
	}
	if !*upload {
		return result{Status: "DRY_RUN", ObjectKey: artifact.key, LocalBytes: artifact.bytes, LocalSHA256: artifact.sha256}, 0
	}

	metadata, ok := provider.(metadataProvider)
	if !ok {
		return result{Status: "CONFIG_REQUIRED", Message: "backup provider does not support checksum metadata"}, 1
	}
	return uploadArtifact(ctx, metadata, artifact, cfg, *downloadTo)
}

func verifyRemoteMetadata(remote storagecenter.ObjectMetadata, expectedSize int64, expectedSHA256 string) remoteVerification {
	sha := metadataSHA(remote.Metadata)
	check := remoteVerification{
		Status:      "REMOTE_VERIFY_FAILED",
		Exists:      true,
		SizeMatch:   remote.Size == expectedSize,
		SHA256Match: strings.EqualFold(strings.TrimSpace(sha), strings.TrimSpace(expectedSHA256)),
	}
	if check.SizeMatch && check.SHA256Match {
		check.Status = "REMOTE_VERIFIED"
	}
	return check
}

func verifyRemote(ctx context.Context, provider storagecenter.Provider, key string, expectedSize int64, expectedSHA256 string) (result, int) {
	remote, err := provider.HeadObject(ctx, key)
	if err != nil {
		if errors.Is(err, storagecenter.ErrFileNotFound) {
			return result{Status: "REMOTE_VERIFY_FAILED", Message: "remote object is missing", ObjectKey: key}, 1
		}
		return result{Status: "REMOTE_VERIFY_FAILED", Message: "remote HEAD failed", ObjectKey: key}, 1
	}
	check := verifyRemoteMetadata(remote, expectedSize, expectedSHA256)
	if !check.SizeMatch || !check.SHA256Match {
		return result{Status: "REMOTE_VERIFY_FAILED", Message: "remote size or checksum metadata does not match", ObjectKey: key, RemoteBytes: remote.Size, RemoteSHA256: metadataSHA(remote.Metadata), Exists: true, SizeMatch: check.SizeMatch, SHA256Match: check.SHA256Match}, 1
	}
	return result{Status: check.Status, ObjectKey: key, RemoteBytes: remote.Size, RemoteSHA256: metadataSHA(remote.Metadata), Verification: "READ_ONLY_HEAD", Exists: true, SizeMatch: true, SHA256Match: true}, 0
}

func statusForConfigError(err error) string {
	if errors.Is(err, storagecenter.ErrBackupConfigNotFound) || errors.Is(err, storagecenter.ErrConfigNotFound) {
		return "BACKUP_STORAGE_CONFIG_NOT_FOUND"
	}
	return "CONFIG_REQUIRED"
}

func openBackupProvider(ctx context.Context, id string) (*sql.DB, storagecenter.Config, storagecenter.Provider, error) {
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	masterKey := strings.TrimSpace(os.Getenv("STORAGE_MASTER_KEY"))
	if databaseURL == "" || masterKey == "" {
		return nil, storagecenter.Config{}, nil, errors.New("DATABASE_URL and STORAGE_MASTER_KEY are required")
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, storagecenter.Config{}, nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, storagecenter.Config{}, nil, err
	}
	factory := storagecenter.S3ProviderFactory{AutoCreateBucket: false}
	service := storagecenter.NewService(storagecenter.NewPostgresRepository(db), factory, storagecenter.Options{MasterKey: masterKey})
	cfg, err := service.BackupConfigByID(ctx, id)
	if err != nil {
		db.Close()
		return nil, storagecenter.Config{}, nil, err
	}
	provider, err := factory.Build(cfg)
	if err != nil {
		db.Close()
		return nil, storagecenter.Config{}, nil, err
	}
	return db, cfg, provider, nil
}

func loadArtifact(root, file string, cfg storagecenter.Config) (backupArtifact, error) {
	rootPath, err := filepath.EvalSymlinks(root)
	if err != nil {
		return backupArtifact{}, errors.New("backup root is unavailable")
	}
	rootPath, err = filepath.Abs(rootPath)
	if err != nil || rootPath == string(filepath.Separator) || filepath.Clean(rootPath) == "/opt" || filepath.Clean(rootPath) == "/opt/zhiqiyun-ai" {
		return backupArtifact{}, errors.New("backup root is too broad")
	}
	info, err := os.Lstat(file)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return backupArtifact{}, errors.New("backup is missing, not regular, or is a symlink")
	}
	resolved, err := filepath.EvalSymlinks(file)
	if err != nil {
		return backupArtifact{}, errors.New("backup path cannot be resolved")
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil || !inside(resolved, rootPath) {
		return backupArtifact{}, errors.New("backup is outside backup root")
	}
	if err := storagecenter.ValidateBackupConfig(cfg); err != nil {
		return backupArtifact{}, storagecenter.ErrBackupConfigNotFound
	}
	name := filepath.Base(resolved)
	if strings.ContainsAny(name, `/\\`) || strings.Contains(name, "..") || name == "." || name == "" {
		return backupArtifact{}, errors.New("backup filename is unsafe")
	}
	category := "deploy"
	if strings.HasPrefix(name, "xianzhi-") {
		category = "daily"
	}
	if !strings.HasPrefix(name, "db_") && category != "daily" {
		return backupArtifact{}, errors.New("unsupported backup category")
	}
	if !strings.HasSuffix(name, ".sql.gz") && !strings.HasSuffix(name, ".sql") {
		return backupArtifact{}, errors.New("unsupported backup extension")
	}
	if info.Size() <= 0 {
		return backupArtifact{}, errors.New("backup is empty")
	}
	if strings.HasSuffix(name, ".gz") {
		if err := checkGzip(resolved); err != nil {
			return backupArtifact{}, err
		}
	}
	metaPath := resolved + ".meta.json"
	metaInfo, err := os.Lstat(metaPath)
	if err != nil || !metaInfo.Mode().IsRegular() || metaInfo.Mode()&os.ModeSymlink != 0 {
		return backupArtifact{}, errors.New("backup metadata is missing or unsafe")
	}
	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		return backupArtifact{}, errors.New("backup metadata cannot be read")
	}
	var meta struct {
		BackupFile string `json:"backup_file"`
		Bytes      int64  `json:"bytes"`
		SHA256     string `json:"sha256"`
	}
	if err := json.Unmarshal(metaBytes, &meta); err != nil || meta.Bytes <= 0 || meta.SHA256 == "" {
		return backupArtifact{}, errors.New("backup metadata is invalid")
	}
	actualHash, err := hashFile(resolved)
	if err != nil || actualHash.bytes != info.Size() || meta.Bytes != info.Size() || !strings.EqualFold(meta.SHA256, actualHash.sha256) {
		return backupArtifact{}, errors.New("backup bytes or sha256 does not match metadata")
	}
	if meta.BackupFile != "" && filepath.Base(meta.BackupFile) != name {
		return backupArtifact{}, errors.New("backup metadata filename does not match")
	}
	stamp := info.ModTime().UTC()
	key, err := backupObjectKey(cfg, category, fmt.Sprintf("%04d", stamp.Year()), fmt.Sprintf("%02d", stamp.Month()), name)
	if err != nil {
		return backupArtifact{}, err
	}
	return backupArtifact{path: resolved, metaPath: metaPath, metaBytes: metaBytes, bytes: info.Size(), sha256: actualHash.sha256, key: key, category: category}, nil
}

func backupObjectKey(cfg storagecenter.Config, category, year, month, name string) (string, error) {
	if cfg.ObjectPrefix != storagecenter.BackupObjectPrefix {
		return "", storagecenter.ErrBackupConfigNotFound
	}
	if category != "deploy" && category != "daily" && category != "event" {
		return "", errors.New("unsupported backup category")
	}
	if name == "" || strings.ContainsAny(name, `/\\`) || strings.Contains(name, "..") {
		return "", errors.New("unsafe backup filename")
	}
	return cfg.ObjectPrefix + category + "/" + year + "/" + month + "/" + name, nil
}

func uploadArtifact(ctx context.Context, provider metadataProvider, artifact backupArtifact, cfg storagecenter.Config, downloadTo string) (result, int) {
	remote, err := provider.HeadObject(ctx, artifact.key)
	if err != nil && !errors.Is(err, storagecenter.ErrFileNotFound) {
		return result{Status: "OFFSITE_UPLOAD_FAILED", Message: "remote HEAD failed"}, 1
	}
	if remote.Size > 0 || err == nil {
		if remote.Size != artifact.bytes || metadataSHA(remote.Metadata) != artifact.sha256 {
			return result{Status: "REMOTE_CONFLICT", Message: "remote object has different size or checksum", ObjectKey: artifact.key}, 1
		}
	} else {
		file, openErr := os.Open(artifact.path)
		if openErr != nil {
			return result{Status: "LOCAL_BACKUP_INVALID", Message: "backup cannot be opened"}, 1
		}
		_, putErr := provider.PutObjectWithMetadata(ctx, artifact.key, file, artifact.bytes, "application/gzip", map[string]string{"x-obs-meta-sha256": artifact.sha256})
		file.Close()
		if putErr != nil {
			return result{Status: "OFFSITE_UPLOAD_FAILED", Message: "backup upload failed"}, 1
		}
		remote, err = provider.HeadObject(ctx, artifact.key)
		if err != nil || remote.Size != artifact.bytes || metadataSHA(remote.Metadata) != artifact.sha256 {
			return result{Status: "REMOTE_CHECKSUM_MISMATCH", Message: "remote backup verification failed"}, 1
		}
	}
	if err := putVerifiedText(ctx, provider, artifact.key+".meta.json", artifact.metaBytes); err != nil {
		if errors.Is(err, errRemoteConflict) {
			return result{Status: "REMOTE_CONFLICT", Message: "remote metadata sidecar conflicts", ObjectKey: artifact.key}, 1
		}
		return result{Status: "OFFSITE_UPLOAD_FAILED", Message: "metadata sidecar upload failed"}, 1
	}
	checksum := []byte(artifact.sha256 + "  " + filepath.Base(artifact.path) + "\n")
	if err := putVerifiedText(ctx, provider, artifact.key+".sha256", checksum); err != nil {
		if errors.Is(err, errRemoteConflict) {
			return result{Status: "REMOTE_CONFLICT", Message: "remote checksum sidecar conflicts", ObjectKey: artifact.key}, 1
		}
		return result{Status: "OFFSITE_UPLOAD_FAILED", Message: "sha256 sidecar upload failed"}, 1
	}
	offsite := artifact.path + ".offsite.json"
	alreadyMarked := false
	if _, statErr := os.Stat(offsite); statErr == nil {
		alreadyMarked = true
	}
	payload := map[string]interface{}{"version": 1, "provider": "obs", "bucket": cfg.Bucket, "object_key": artifact.key, "uploaded_at": time.Now().UTC().Format(time.RFC3339), "local_bytes": artifact.bytes, "local_sha256": artifact.sha256, "remote_bytes": remote.Size, "remote_etag": remote.ETag, "remote_sha256": metadataSHA(remote.Metadata), "verification": "OFFSITE_VERIFIED"}
	if err := atomicJSON(offsite, payload); err != nil {
		return result{Status: "OFFSITE_UPLOAD_FAILED", Message: "offsite marker write failed"}, 1
	}
	if strings.TrimSpace(downloadTo) != "" {
		if err := downloadAndVerify(ctx, provider, artifact.key, downloadTo, artifact); err != nil {
			return result{Status: "DOWNLOAD_VERIFY_FAILED", Message: err.Error(), ObjectKey: artifact.key}, 1
		}
	}
	status := "OFFSITE_VERIFIED"
	if alreadyMarked && remote.Size == artifact.bytes && metadataSHA(remote.Metadata) == artifact.sha256 {
		status = "ALREADY_OFFSITE_VERIFIED"
	}
	return result{Status: status, ObjectKey: artifact.key, OffsitePath: offsite, LocalBytes: artifact.bytes, RemoteBytes: remote.Size, LocalSHA256: artifact.sha256, RemoteSHA256: metadataSHA(remote.Metadata), RemoteETag: remote.ETag, Verification: "OFFSITE_VERIFIED"}, 0
}

func downloadAndVerify(ctx context.Context, provider storagecenter.Provider, key, destination string, artifact backupArtifact) error {
	tempRoot, err := filepath.Abs(os.TempDir())
	if err != nil {
		return errors.New("temporary directory is unavailable")
	}
	destination, err = filepath.Abs(destination)
	if err != nil || !inside(destination, tempRoot) || destination == tempRoot {
		return errors.New("download destination must be inside the system temporary directory")
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return err
	}
	object, err := provider.OpenObject(ctx, key)
	if err != nil {
		return err
	}
	defer object.Close()
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, object)
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	info, err := os.Stat(destination)
	if err != nil || info.Size() != artifact.bytes {
		return errors.New("downloaded bytes do not match")
	}
	hash, err := hashFile(destination)
	if err != nil || hash.sha256 != artifact.sha256 {
		return errors.New("downloaded sha256 does not match")
	}
	if strings.HasSuffix(destination, ".gz") || strings.HasSuffix(artifact.path, ".gz") {
		if err := checkGzip(destination); err != nil {
			return err
		}
	}
	return nil
}

func putVerifiedText(ctx context.Context, provider metadataProvider, key string, content []byte) error {
	hash := sha256.Sum256(content)
	remote, err := provider.HeadObject(ctx, key)
	if err == nil && remote.Size == int64(len(content)) && metadataSHA(remote.Metadata) == hex.EncodeToString(hash[:]) {
		return nil
	}
	if err == nil {
		return errRemoteConflict
	}
	if err != nil && !errors.Is(err, storagecenter.ErrFileNotFound) {
		return err
	}
	_, err = provider.PutObjectWithMetadata(ctx, key, strings.NewReader(string(content)), int64(len(content)), "application/json", map[string]string{"x-obs-meta-sha256": hex.EncodeToString(hash[:])})
	if err != nil {
		return err
	}
	remote, err = provider.HeadObject(ctx, key)
	if err != nil || remote.Size != int64(len(content)) || metadataSHA(remote.Metadata) != hex.EncodeToString(hash[:]) {
		return errors.New("sidecar verification failed")
	}
	return nil
}

func metadataSHA(metadata map[string]string) string {
	for key, value := range metadata {
		if strings.EqualFold(key, "x-obs-meta-sha256") || strings.EqualFold(key, "sha256") {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func atomicJSON(path string, payload interface{}) error {
	temporary := path + ".part"
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(temporary, data, 0600); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func checkGzip(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	reader, err := gzip.NewReader(file)
	if err != nil {
		return errors.New("gzip validation failed")
	}
	_, err = io.Copy(io.Discard, reader)
	reader.Close()
	if err != nil {
		return errors.New("gzip validation failed")
	}
	return nil
}

type hashResult struct {
	bytes  int64
	sha256 string
}

func hashFile(path string) (hashResult, error) {
	file, err := os.Open(path)
	if err != nil {
		return hashResult{}, err
	}
	defer file.Close()
	hash := sha256.New()
	bytes, err := io.Copy(hash, file)
	if err != nil {
		return hashResult{}, err
	}
	return hashResult{bytes: bytes, sha256: hex.EncodeToString(hash.Sum(nil))}, nil
}

func inside(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
