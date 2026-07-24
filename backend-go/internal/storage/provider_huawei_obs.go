package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	huaweiobs "github.com/huaweicloud/huaweicloud-sdk-go-obs/obs"
)

type huaweiOBSProvider struct {
	client           *huaweiobs.ObsClient
	signer           *huaweiobs.ObsClient
	bucket           string
	region           string
	autoCreateBucket bool
}

func buildHuaweiOBSProvider(cfg Config, autoCreateBucket bool) (Provider, error) {
	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" || strings.TrimSpace(cfg.Bucket) == "" {
		return nil, fmt.Errorf("storage endpoint and bucket are required")
	}
	region := strings.TrimSpace(cfg.Region)
	if region == "" {
		return nil, fmt.Errorf("huawei obs region is required")
	}
	client, err := newHuaweiOBSClient(endpoint, cfg, false)
	if err != nil {
		return nil, err
	}
	signer := client
	signingEndpoint := strings.TrimSpace(cfg.SigningEndpoint)
	if signingEndpoint != "" && signingEndpoint != endpoint {
		signer, err = newHuaweiOBSClient(signingEndpoint, cfg, true)
		if err != nil {
			client.Close()
			return nil, fmt.Errorf("invalid storage signing endpoint: %w", err)
		}
	}
	return &huaweiOBSProvider{
		client: client, signer: signer, bucket: strings.TrimSpace(cfg.Bucket),
		region: region, autoCreateBucket: autoCreateBucket,
	}, nil
}

func newHuaweiOBSClient(endpoint string, cfg Config, customDomain bool) (*huaweiobs.ObsClient, error) {
	region := strings.TrimSpace(cfg.Region)
	token := strings.TrimSpace(cfg.SessionToken)
	if customDomain && token != "" {
		return huaweiobs.New(cfg.AccessKey, cfg.SecretKey, endpoint,
			huaweiobs.WithSignature(huaweiobs.SignatureObs), huaweiobs.WithRegion(region), huaweiobs.WithPathStyle(false),
			huaweiobs.WithSecurityToken(token), huaweiobs.WithCustomDomainName(true))
	}
	if customDomain {
		return huaweiobs.New(cfg.AccessKey, cfg.SecretKey, endpoint,
			huaweiobs.WithSignature(huaweiobs.SignatureObs), huaweiobs.WithRegion(region), huaweiobs.WithPathStyle(false),
			huaweiobs.WithCustomDomainName(true))
	}
	if token != "" {
		return huaweiobs.New(cfg.AccessKey, cfg.SecretKey, endpoint,
			huaweiobs.WithSignature(huaweiobs.SignatureObs), huaweiobs.WithRegion(region), huaweiobs.WithPathStyle(false),
			huaweiobs.WithSecurityToken(token))
	}
	return huaweiobs.New(cfg.AccessKey, cfg.SecretKey, endpoint,
		huaweiobs.WithSignature(huaweiobs.SignatureObs), huaweiobs.WithRegion(region), huaweiobs.WithPathStyle(false))
}

func (p *huaweiOBSProvider) ensureBucket(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := p.client.HeadBucket(p.bucket)
	if err == nil {
		return nil
	}
	if !isHuaweiOBSNotFound(err) || !p.autoCreateBucket {
		return err
	}
	_, err = p.client.CreateBucket(&huaweiobs.CreateBucketInput{
		Bucket:         p.bucket,
		BucketLocation: huaweiobs.BucketLocation{Location: p.region},
	})
	return err
}

func (p *huaweiOBSProvider) PutObject(ctx context.Context, objectKey string, source io.Reader, size int64, contentType string) (ObjectMetadata, error) {
	if err := p.ensureBucket(ctx); err != nil {
		return ObjectMetadata{}, err
	}
	output, err := p.client.PutObject(&huaweiobs.PutObjectInput{
		PutObjectBasicInput: huaweiobs.PutObjectBasicInput{
			ObjectOperationInput: huaweiobs.ObjectOperationInput{Bucket: p.bucket, Key: objectKey},
			HttpHeader:           huaweiobs.HttpHeader{ContentType: contentType}, ContentLength: size,
		},
		Body: source,
	})
	if err != nil {
		return ObjectMetadata{}, err
	}
	return ObjectMetadata{Size: size, ETag: strings.Trim(output.ETag, "\""), ContentType: contentType}, nil
}

func (p *huaweiOBSProvider) HeadObject(ctx context.Context, objectKey string) (ObjectMetadata, error) {
	if err := ctx.Err(); err != nil {
		return ObjectMetadata{}, err
	}
	output, err := p.client.GetObjectMetadata(&huaweiobs.GetObjectMetadataInput{Bucket: p.bucket, Key: objectKey})
	if err != nil {
		if isHuaweiOBSNotFound(err) {
			return ObjectMetadata{}, ErrFileNotFound
		}
		return ObjectMetadata{}, err
	}
	return ObjectMetadata{
		Size: output.ContentLength, ETag: strings.Trim(output.ETag, "\""), ContentType: output.ContentType,
		LastModified: output.LastModified, Metadata: output.Metadata,
	}, nil
}

func (p *huaweiOBSProvider) DeleteObject(ctx context.Context, objectKey string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := p.client.DeleteObject(&huaweiobs.DeleteObjectInput{Bucket: p.bucket, Key: objectKey})
	return err
}

func (p *huaweiOBSProvider) CopyObject(ctx context.Context, sourceKey string, targetKey string) error {
	if err := p.ensureBucket(ctx); err != nil {
		return err
	}
	_, err := p.client.CopyObject(&huaweiobs.CopyObjectInput{
		ObjectOperationInput: huaweiobs.ObjectOperationInput{Bucket: p.bucket, Key: targetKey},
		CopySourceBucket:     p.bucket, CopySourceKey: sourceKey,
	})
	return err
}

func (p *huaweiOBSProvider) CreatePresignedUploadURL(ctx context.Context, objectKey, contentType string, ttl time.Duration) (string, error) {
	if err := p.ensureBucket(ctx); err != nil {
		return "", err
	}
	headers := map[string]string{}
	if value := strings.TrimSpace(contentType); value != "" {
		headers["Content-Type"] = value
	}
	return p.createSignedURL(ctx, huaweiobs.HttpMethodPut, objectKey, ttl, headers)
}

func (p *huaweiOBSProvider) CreatePresignedDownloadURL(ctx context.Context, objectKey string, ttl time.Duration) (string, error) {
	return p.createSignedURL(ctx, huaweiobs.HttpMethodGet, objectKey, ttl, nil)
}

func (p *huaweiOBSProvider) createSignedURL(ctx context.Context, method huaweiobs.HttpMethodType, objectKey string, ttl time.Duration, headers map[string]string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	expires := int(ttl / time.Second)
	if expires < 1 {
		expires = 1
	}
	output, err := p.signer.CreateSignedUrl(&huaweiobs.CreateSignedUrlInput{
		Method: method, Bucket: p.bucket, Key: objectKey, Expires: expires, Headers: headers,
	})
	if err != nil {
		return "", err
	}
	return output.SignedUrl, nil
}

func (p *huaweiOBSProvider) TestConnection(ctx context.Context) error {
	return p.ensureBucket(ctx)
}

func isHuaweiOBSNotFound(err error) bool {
	var obsErr huaweiobs.ObsError
	if !errors.As(err, &obsErr) {
		return false
	}
	return obsErr.StatusCode == 404 || obsErr.Code == "NoSuchBucket" || obsErr.Code == "NoSuchKey" || obsErr.Code == "NoSuchObject"
}
