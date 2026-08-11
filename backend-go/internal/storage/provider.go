package storage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Provider interface {
	PutObject(context.Context, string, io.Reader, int64, string) (ObjectMetadata, error)
	OpenObject(context.Context, string) (io.ReadCloser, error)
	HeadObject(context.Context, string) (ObjectMetadata, error)
	DeleteObject(context.Context, string) error
	CopyObject(context.Context, string, string) error
	CreatePresignedUploadURL(context.Context, string, string, time.Duration) (string, error)
	CreatePresignedDownloadURL(context.Context, string, time.Duration) (string, error)
	CreateMultipartUpload(context.Context, string, string) (string, error)
	PresignUploadPart(context.Context, string, string, int, time.Duration) (string, error)
	CompleteMultipartUpload(context.Context, string, string, []CompletedPart) (ObjectMetadata, error)
	AbortMultipartUpload(context.Context, string, string) error
	TestConnection(context.Context) error
}

type ProviderFactory interface {
	Build(Config) (Provider, error)
}

type S3ProviderFactory struct {
	AutoCreateBucket bool
}

func (f S3ProviderFactory) Build(cfg Config) (Provider, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "minio", "s3", "aliyun_oss", "tencent_cos", "huawei_obs", "cloudflare_r2":
	default:
		return nil, fmt.Errorf("%w: %s", ErrProviderUnsupported, cfg.Provider)
	}
	if strings.EqualFold(strings.TrimSpace(cfg.Provider), "huawei_obs") {
		return buildHuaweiOBSProvider(cfg, f.AutoCreateBucket)
	}
	region := strings.TrimSpace(cfg.Region)
	if region == "" {
		region = "us-east-1"
	}
	lookup := minio.BucketLookupAuto
	if cfg.ForcePathStyle || strings.EqualFold(cfg.Provider, "minio") || strings.EqualFold(cfg.Provider, "cloudflare_r2") {
		lookup = minio.BucketLookupPath
	}
	client, err := buildS3Client(cfg.Endpoint, cfg, region, lookup)
	if err != nil {
		return nil, err
	}
	signer := client
	if strings.TrimSpace(cfg.SigningEndpoint) != "" && strings.TrimSpace(cfg.SigningEndpoint) != strings.TrimSpace(cfg.Endpoint) {
		signer, err = buildS3Client(cfg.SigningEndpoint, cfg, region, lookup)
		if err != nil {
			return nil, fmt.Errorf("invalid storage signing endpoint: %w", err)
		}
	}
	return &s3Provider{client: client, signer: signer, bucket: cfg.Bucket, region: region, autoCreateBucket: f.AutoCreateBucket}, nil
}

func buildS3Client(endpoint string, cfg Config, region string, lookup minio.BucketLookupType) (*minio.Client, error) {
	endpoint = strings.TrimSpace(endpoint)
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid storage endpoint: %w", err)
	}
	host := parsed.Host
	secure := cfg.UseSSL
	if parsed.Scheme != "" {
		secure = strings.EqualFold(parsed.Scheme, "https")
	}
	if host == "" {
		host = strings.TrimRight(strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://"), "/")
	}
	if host == "" || strings.TrimSpace(cfg.Bucket) == "" {
		return nil, fmt.Errorf("storage endpoint and bucket are required")
	}
	client, err := minio.New(host, &minio.Options{
		Creds:        credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, cfg.SessionToken),
		Secure:       secure,
		Region:       region,
		BucketLookup: lookup,
	})
	if err != nil {
		return nil, err
	}
	return client, nil
}

type s3Provider struct {
	client           *minio.Client
	signer           *minio.Client
	bucket           string
	region           string
	autoCreateBucket bool
}

func (p *s3Provider) ensureBucket(ctx context.Context) error {
	exists, err := p.client.BucketExists(ctx, p.bucket)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	if !p.autoCreateBucket {
		return fmt.Errorf("bucket %s does not exist", p.bucket)
	}
	return p.client.MakeBucket(ctx, p.bucket, minio.MakeBucketOptions{Region: p.region})
}

func (p *s3Provider) PutObject(ctx context.Context, objectKey string, source io.Reader, size int64, contentType string) (ObjectMetadata, error) {
	if err := p.ensureBucket(ctx); err != nil {
		return ObjectMetadata{}, err
	}
	info, err := p.client.PutObject(ctx, p.bucket, objectKey, source, size, minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return ObjectMetadata{}, err
	}
	return ObjectMetadata{Size: info.Size, ETag: info.ETag, ContentType: contentType}, nil
}

func (p *s3Provider) OpenObject(ctx context.Context, objectKey string) (io.ReadCloser, error) {
	object, err := p.client.GetObject(ctx, p.bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	if _, err := object.Stat(); err != nil {
		_ = object.Close()
		response := minio.ToErrorResponse(err)
		if response.StatusCode == http.StatusNotFound || response.Code == "NoSuchKey" || response.Code == "NoSuchObject" {
			return nil, ErrFileNotFound
		}
		return nil, err
	}
	return object, nil
}

func (p *s3Provider) HeadObject(ctx context.Context, objectKey string) (ObjectMetadata, error) {
	info, err := p.client.StatObject(ctx, p.bucket, objectKey, minio.StatObjectOptions{})
	if err != nil {
		response := minio.ToErrorResponse(err)
		if response.StatusCode == http.StatusNotFound || response.Code == "NoSuchKey" || response.Code == "NoSuchObject" {
			return ObjectMetadata{}, ErrFileNotFound
		}
		return ObjectMetadata{}, err
	}
	metadata := make(map[string]string, len(info.Metadata))
	for key, values := range info.Metadata {
		metadata[key] = strings.Join(values, ",")
	}
	return ObjectMetadata{Size: info.Size, ETag: info.ETag, ContentType: info.ContentType, LastModified: info.LastModified, Metadata: metadata}, nil
}

func (p *s3Provider) DeleteObject(ctx context.Context, objectKey string) error {
	return p.client.RemoveObject(ctx, p.bucket, objectKey, minio.RemoveObjectOptions{})
}

func (p *s3Provider) CopyObject(ctx context.Context, sourceKey string, targetKey string) error {
	if err := p.ensureBucket(ctx); err != nil {
		return err
	}
	_, err := p.client.CopyObject(ctx,
		minio.CopyDestOptions{Bucket: p.bucket, Object: targetKey},
		minio.CopySrcOptions{Bucket: p.bucket, Object: sourceKey},
	)
	return err
}

func (p *s3Provider) CreatePresignedUploadURL(ctx context.Context, objectKey, _ string, ttl time.Duration) (string, error) {
	if err := p.ensureBucket(ctx); err != nil {
		return "", err
	}
	signed, err := p.signer.PresignedPutObject(ctx, p.bucket, objectKey, ttl)
	if err != nil {
		return "", err
	}
	return signed.String(), nil
}

func (p *s3Provider) CreatePresignedDownloadURL(ctx context.Context, objectKey string, ttl time.Duration) (string, error) {
	signed, err := p.signer.PresignedGetObject(ctx, p.bucket, objectKey, ttl, nil)
	if err != nil {
		return "", err
	}
	return signed.String(), nil
}

func (p *s3Provider) CreateMultipartUpload(ctx context.Context, objectKey, contentType string) (string, error) {
	if err := p.ensureBucket(ctx); err != nil {
		return "", err
	}
	opts := minio.PutObjectOptions{}
	if strings.TrimSpace(contentType) != "" {
		opts.ContentType = contentType
	}
	uploadID, err := minio.Core{Client: p.client}.NewMultipartUpload(ctx, p.bucket, objectKey, opts)
	if err != nil {
		return "", err
	}
	return uploadID, nil
}

func (p *s3Provider) PresignUploadPart(ctx context.Context, objectKey, uploadID string, partNumber int, ttl time.Duration) (string, error) {
	if err := p.ensureBucket(ctx); err != nil {
		return "", err
	}
	if partNumber < 1 {
		return "", fmt.Errorf("%w: invalid part number", ErrInvalidMultipartPart)
	}
	params := url.Values{}
	params.Set("uploadId", uploadID)
	params.Set("partNumber", fmt.Sprintf("%d", partNumber))
	signed, err := p.signer.Presign(ctx, http.MethodPut, p.bucket, objectKey, ttl, params)
	if err != nil {
		return "", err
	}
	return signed.String(), nil
}

func (p *s3Provider) CompleteMultipartUpload(ctx context.Context, objectKey, uploadID string, parts []CompletedPart) (ObjectMetadata, error) {
	if err := p.ensureBucket(ctx); err != nil {
		return ObjectMetadata{}, err
	}
	completeParts := make([]minio.CompletePart, 0, len(parts))
	for _, part := range parts {
		completeParts = append(completeParts, minio.CompletePart{
			PartNumber: part.PartNumber,
			ETag:       strings.Trim(part.ETag, `"`),
		})
	}
	info, err := minio.Core{Client: p.client}.CompleteMultipartUpload(ctx, p.bucket, objectKey, uploadID, completeParts, minio.PutObjectOptions{})
	if err != nil {
		return ObjectMetadata{}, err
	}
	meta, headErr := p.HeadObject(ctx, objectKey)
	if headErr == nil {
		return meta, nil
	}
	return ObjectMetadata{Size: info.Size, ETag: info.ETag}, nil
}

func (p *s3Provider) AbortMultipartUpload(ctx context.Context, objectKey, uploadID string) error {
	return minio.Core{Client: p.client}.AbortMultipartUpload(ctx, p.bucket, objectKey, uploadID)
}

func (p *s3Provider) TestConnection(ctx context.Context) error {
	return p.ensureBucket(ctx)
}
