package httpserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"xianzhi-ai/backend-go/internal/config"
)

const defaultMediaMaxUploadBytes int64 = 12 << 20

type mediaStoredObject struct {
	Provider  string
	Bucket    string
	Key       string
	PublicURL string
}

type mediaStorageProvider interface {
	Upload(context.Context, string, io.Reader) (mediaStoredObject, error)
	Delete(context.Context, string) error
	GetPublicURL(string) string
	GetSignedURL(context.Context, string, time.Duration) (string, error)
	Copy(context.Context, string, string) (mediaStoredObject, error)
	Exists(context.Context, string) (bool, error)
	GenerateThumbnail(context.Context, string, string) (mediaStoredObject, error)
}

type localMediaStorage struct {
	root       string
	publicBase string
	cdnBase    string
}

func newMediaStorage(cfg config.Config) (mediaStorageProvider, error) {
	provider := strings.ToLower(strings.TrimSpace(cfg.MediaStorageProvider))
	if provider == "" {
		provider = "local"
	}
	if provider != "local" {
		switch provider {
		case "s3", "aliyun_oss", "tencent_cos", "qiniu":
			return newS3CompatibleMediaStorage(provider, cfg)
		default:
			return nil, fmt.Errorf("unsupported media storage provider %q", provider)
		}
	}
	root := strings.TrimSpace(cfg.MediaStorageRoot)
	if root == "" {
		root = filepath.Join(filepath.Dir(cfg.DataPath), "media-assets")
	}
	publicBase := strings.TrimRight(strings.TrimSpace(cfg.MediaPublicBaseURL), "/")
	if publicBase == "" {
		publicBase = "/api/v1/media/files"
	}
	return &localMediaStorage{root: root, publicBase: publicBase, cdnBase: strings.TrimRight(strings.TrimSpace(cfg.MediaCDNBaseURL), "/")}, nil
}

type s3CompatibleMediaStorage struct {
	provider   string
	client     *minio.Client
	bucket     string
	endpoint   string
	publicBase string
}

func newS3CompatibleMediaStorage(provider string, cfg config.Config) (mediaStorageProvider, error) {
	rawEndpoint := strings.TrimSpace(cfg.S3Endpoint)
	if rawEndpoint == "" || strings.TrimSpace(cfg.S3AccessKey) == "" || strings.TrimSpace(cfg.S3SecretKey) == "" || strings.TrimSpace(cfg.S3Bucket) == "" {
		return nil, errors.New("S3_ENDPOINT, S3_ACCESS_KEY, S3_SECRET_KEY and S3_BUCKET are required for object media storage")
	}
	parsed, err := url.Parse(rawEndpoint)
	if err != nil {
		return nil, err
	}
	host := parsed.Host
	secure := strings.EqualFold(parsed.Scheme, "https")
	if host == "" {
		host = strings.TrimRight(strings.TrimPrefix(strings.TrimPrefix(rawEndpoint, "https://"), "http://"), "/")
	}
	client, err := minio.New(host, &minio.Options{Creds: credentials.NewStaticV4(cfg.S3AccessKey, cfg.S3SecretKey, ""), Secure: secure})
	if err != nil {
		return nil, err
	}
	publicBase := strings.TrimRight(strings.TrimSpace(cfg.MediaCDNBaseURL), "/")
	if publicBase == "" {
		publicBase = strings.TrimRight(rawEndpoint, "/") + "/" + strings.TrimSpace(cfg.S3Bucket)
	}
	return &s3CompatibleMediaStorage{provider: provider, client: client, bucket: strings.TrimSpace(cfg.S3Bucket), endpoint: rawEndpoint, publicBase: publicBase}, nil
}

func (s *s3CompatibleMediaStorage) ensureBucket(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{})
}
func (s *s3CompatibleMediaStorage) Upload(ctx context.Context, key string, source io.Reader) (mediaStoredObject, error) {
	if err := s.ensureBucket(ctx); err != nil {
		return mediaStoredObject{}, err
	}
	_, err := s.client.PutObject(ctx, s.bucket, key, source, -1, minio.PutObjectOptions{ContentType: "application/octet-stream"})
	if err != nil {
		return mediaStoredObject{}, err
	}
	return mediaStoredObject{Provider: s.provider, Bucket: s.bucket, Key: key, PublicURL: s.GetPublicURL(key)}, nil
}
func (s *s3CompatibleMediaStorage) Delete(ctx context.Context, key string) error {
	return s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}
func (s *s3CompatibleMediaStorage) GetPublicURL(key string) string {
	segments := strings.Split(filepath.ToSlash(strings.TrimLeft(key, "/")), "/")
	for i := range segments {
		segments[i] = url.PathEscape(segments[i])
	}
	return s.publicBase + "/" + strings.Join(segments, "/")
}
func (s *s3CompatibleMediaStorage) GetSignedURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	signed, err := s.client.PresignedGetObject(ctx, s.bucket, key, ttl, nil)
	if err != nil {
		return "", err
	}
	return signed.String(), nil
}
func (s *s3CompatibleMediaStorage) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		response := minio.ToErrorResponse(err)
		if response.StatusCode == http.StatusNotFound || response.Code == "NoSuchKey" {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
func (s *s3CompatibleMediaStorage) Copy(ctx context.Context, sourceKey, targetKey string) (mediaStoredObject, error) {
	if err := s.ensureBucket(ctx); err != nil {
		return mediaStoredObject{}, err
	}
	_, err := s.client.CopyObject(ctx, minio.CopyDestOptions{Bucket: s.bucket, Object: targetKey}, minio.CopySrcOptions{Bucket: s.bucket, Object: sourceKey})
	if err != nil {
		return mediaStoredObject{}, err
	}
	return mediaStoredObject{Provider: s.provider, Bucket: s.bucket, Key: targetKey, PublicURL: s.GetPublicURL(targetKey)}, nil
}
func (s *s3CompatibleMediaStorage) GenerateThumbnail(ctx context.Context, sourceKey, targetKey string) (mediaStoredObject, error) {
	return s.Copy(ctx, sourceKey, targetKey)
}

type unavailableMediaStorage struct{ err error }

func (s unavailableMediaStorage) Upload(context.Context, string, io.Reader) (mediaStoredObject, error) {
	return mediaStoredObject{}, s.err
}
func (s unavailableMediaStorage) Delete(context.Context, string) error { return s.err }
func (s unavailableMediaStorage) GetPublicURL(string) string           { return "" }
func (s unavailableMediaStorage) GetSignedURL(context.Context, string, time.Duration) (string, error) {
	return "", s.err
}
func (s unavailableMediaStorage) Copy(context.Context, string, string) (mediaStoredObject, error) {
	return mediaStoredObject{}, s.err
}
func (s unavailableMediaStorage) Exists(context.Context, string) (bool, error) { return false, s.err }
func (s unavailableMediaStorage) GenerateThumbnail(context.Context, string, string) (mediaStoredObject, error) {
	return mediaStoredObject{}, s.err
}

func (s *localMediaStorage) absolutePath(key string) (string, error) {
	key = filepath.FromSlash(strings.TrimLeft(strings.TrimSpace(key), "/"))
	if key == "" {
		return "", errors.New("storage key is required")
	}
	root, err := filepath.Abs(s.root)
	if err != nil {
		return "", err
	}
	target, err := filepath.Abs(filepath.Join(root, key))
	if err != nil {
		return "", err
	}
	if target != root && !strings.HasPrefix(target, root+string(os.PathSeparator)) {
		return "", errors.New("invalid media storage key")
	}
	return target, nil
}

func (s *localMediaStorage) Upload(_ context.Context, key string, source io.Reader) (mediaStoredObject, error) {
	target, err := s.absolutePath(key)
	if err != nil {
		return mediaStoredObject{}, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return mediaStoredObject{}, err
	}
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if errors.Is(err, os.ErrExist) {
		return mediaStoredObject{Provider: "local", Key: key, PublicURL: s.GetPublicURL(key)}, nil
	}
	if err != nil {
		return mediaStoredObject{}, err
	}
	_, copyErr := io.Copy(file, source)
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(target)
		return mediaStoredObject{}, copyErr
	}
	if closeErr != nil {
		return mediaStoredObject{}, closeErr
	}
	return mediaStoredObject{Provider: "local", Key: key, PublicURL: s.GetPublicURL(key)}, nil
}

func (s *localMediaStorage) Delete(_ context.Context, key string) error {
	target, err := s.absolutePath(key)
	if err != nil {
		return err
	}
	err = os.Remove(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
func (s *localMediaStorage) GetPublicURL(key string) string {
	base := s.publicBase
	if s.cdnBase != "" {
		base = s.cdnBase
	}
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(filepath.ToSlash(key), "/")
}
func (s *localMediaStorage) GetSignedURL(_ context.Context, key string, _ time.Duration) (string, error) {
	return s.GetPublicURL(key), nil
}
func (s *localMediaStorage) Exists(_ context.Context, key string) (bool, error) {
	target, err := s.absolutePath(key)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(target)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}
func (s *localMediaStorage) Copy(ctx context.Context, sourceKey, targetKey string) (mediaStoredObject, error) {
	source, err := s.absolutePath(sourceKey)
	if err != nil {
		return mediaStoredObject{}, err
	}
	file, err := os.Open(source)
	if err != nil {
		return mediaStoredObject{}, err
	}
	defer file.Close()
	return s.Upload(ctx, targetKey, file)
}
func (s *localMediaStorage) GenerateThumbnail(ctx context.Context, sourceKey, targetKey string) (mediaStoredObject, error) {
	return s.Copy(ctx, sourceKey, targetKey)
}

type validatedMediaUpload struct {
	Bytes     []byte
	Hash      string
	MimeType  string
	Extension string
	Width     int
	Height    int
	Ratio     float64
	Original  string
}

func validateMediaUpload(header *multipart.FileHeader, maxBytes int64) (validatedMediaUpload, error) {
	if header == nil {
		return validatedMediaUpload{}, errors.New("file is required")
	}
	if maxBytes <= 0 {
		maxBytes = defaultMediaMaxUploadBytes
	}
	if header.Size <= 0 || header.Size > maxBytes {
		return validatedMediaUpload{}, fmt.Errorf("file size must be between 1 and %d bytes", maxBytes)
	}
	file, err := header.Open()
	if err != nil {
		return validatedMediaUpload{}, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return validatedMediaUpload{}, err
	}
	if int64(len(raw)) > maxBytes {
		return validatedMediaUpload{}, fmt.Errorf("file exceeds %d bytes", maxBytes)
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(header.Filename), "."))
	allowed := map[string]string{"jpg": "image/jpeg", "jpeg": "image/jpeg", "png": "image/png", "webp": "image/webp", "avif": "image/avif", "svg": "image/svg+xml"}
	expected, ok := allowed[ext]
	if !ok {
		return validatedMediaUpload{}, errors.New("only jpg, jpeg, png, webp, avif and svg files are supported")
	}
	mimeType := detectMediaMIME(raw)
	if mimeType != expected {
		return validatedMediaUpload{}, fmt.Errorf("file MIME %q does not match extension .%s", mimeType, ext)
	}
	if mimeType == "image/svg+xml" {
		if err := validateSafeSVG(raw); err != nil {
			return validatedMediaUpload{}, err
		}
	}
	width, height := mediaDimensions(raw, mimeType)
	if width > 16384 || height > 16384 {
		return validatedMediaUpload{}, errors.New("image dimensions exceed 16384px")
	}
	ratio := 0.0
	if height > 0 {
		ratio = float64(width) / float64(height)
	}
	sum := sha256.Sum256(raw)
	return validatedMediaUpload{Bytes: raw, Hash: hex.EncodeToString(sum[:]), MimeType: mimeType, Extension: ext, Width: width, Height: height, Ratio: ratio, Original: filepath.Base(header.Filename)}, nil
}

func detectMediaMIME(raw []byte) string {
	trimmed := bytes.TrimSpace(raw)
	lower := bytes.ToLower(trimmed)
	if bytes.HasPrefix(lower, []byte("<svg")) || (bytes.HasPrefix(lower, []byte("<?xml")) && bytes.Contains(lower, []byte("<svg"))) {
		return "image/svg+xml"
	}
	if len(raw) >= 12 && string(raw[4:8]) == "ftyp" && (string(raw[8:12]) == "avif" || string(raw[8:12]) == "avis") {
		return "image/avif"
	}
	return strings.Split(http.DetectContentType(raw), ";")[0]
}

var svgDimensionPattern = regexp.MustCompile(`(?i)\b(width|height)\s*=\s*["']([0-9]+(?:\.[0-9]+)?)(?:px)?["']`)

func mediaDimensions(raw []byte, mimeType string) (int, int) {
	if mimeType == "image/svg+xml" {
		width, height := 0, 0
		for _, match := range svgDimensionPattern.FindAllSubmatch(raw, -1) {
			value, _ := strconv.ParseFloat(string(match[2]), 64)
			if strings.EqualFold(string(match[1]), "width") {
				width = int(value)
			} else {
				height = int(value)
			}
		}
		return width, height
	}
	if mimeType == "image/webp" && len(raw) >= 30 && string(raw[12:16]) == "VP8X" {
		width := 1 + int(raw[24]) + (int(raw[25]) << 8) + (int(raw[26]) << 16)
		height := 1 + int(raw[27]) + (int(raw[28]) << 8) + (int(raw[29]) << 16)
		return width, height
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err == nil {
		return cfg.Width, cfg.Height
	}
	return 0, 0
}
func validateSafeSVG(raw []byte) error {
	lower := strings.ToLower(string(raw))
	for _, forbidden := range []string{"<script", "javascript:", "data:text/html", "<!entity", "<!doctype", "onload=", "onclick=", "onerror=", "foreignobject", "xlink:href=\"http", "href=\"http"} {
		if strings.Contains(lower, forbidden) {
			return errors.New("SVG contains unsafe active or external content")
		}
	}
	return nil
}

func mediaStorageKey(tenant, category, hash, ext string) string {
	safe := func(value string) string {
		value = strings.ToLower(strings.TrimSpace(value))
		value = regexp.MustCompile(`[^a-z0-9_-]+`).ReplaceAllString(value, "-")
		value = strings.Trim(value, "-")
		if value == "" {
			return "default"
		}
		return value
	}
	now := time.Now().UTC()
	name := hash
	if len(name) > 32 {
		name = name[:32]
	}
	return fmt.Sprintf("tenant/%s/%s/%04d/%02d/%s.%s", safe(tenant), safe(category), now.Year(), int(now.Month()), name, safe(ext))
}

func parseMediaMaxBytes(value string) int64 {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || parsed <= 0 {
		return defaultMediaMaxUploadBytes
	}
	return parsed
}

func serveLocalMediaFile(storage mediaStorageProvider) http.HandlerFunc {
	local, ok := storage.(*localMediaStorage)
	return func(w http.ResponseWriter, r *http.Request) {
		if !ok {
			writeError(w, http.StatusNotFound, errMediaNotFound)
			return
		}
		key := strings.TrimPrefix(r.PathValue("filepath"), "/")
		target, err := local.absolutePath(key)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		http.ServeFile(w, r, target)
	}
}

var _ = binary.LittleEndian
