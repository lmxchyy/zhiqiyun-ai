package storage

import (
	"context"
	"net/url"
	"testing"
	"time"

	huaweiobs "github.com/huaweicloud/huaweicloud-sdk-go-obs/obs"
)

func TestS3ProviderUsesPublicSigningEndpoint(t *testing.T) {
	provider, err := (S3ProviderFactory{}).Build(Config{
		Provider:        "minio",
		Endpoint:        "http://minio:9000",
		SigningEndpoint: "http://localhost:9000",
		Region:          "us-east-1",
		Bucket:          "xianzhi-assets",
		AccessKey:       "access",
		SecretKey:       "secret",
		ForcePathStyle:  true,
	})
	if err != nil {
		t.Fatalf("build provider: %v", err)
	}
	s3 := provider.(*s3Provider)
	signed, err := s3.signer.PresignedPutObject(context.Background(), s3.bucket, "tenants/t1/uploads/test.txt", time.Minute)
	if err != nil {
		t.Fatalf("presign upload: %v", err)
	}
	parsed, err := url.Parse(signed.String())
	if err != nil {
		t.Fatalf("parse presigned URL: %v", err)
	}
	if parsed.Host != "localhost:9000" {
		t.Fatalf("presigned URL host = %q, want localhost:9000", parsed.Host)
	}
	if parsed.Query().Get("X-Amz-Signature") == "" {
		t.Fatalf("presigned URL has no signature: %s", signed.String())
	}
}

func TestHuaweiOBSProviderUsesNativeClientAndSignature(t *testing.T) {
	provider, err := (S3ProviderFactory{}).Build(Config{
		Provider:  "huawei_obs",
		Endpoint:  "https://obs.cn-north-9.myhuaweicloud.com",
		Region:    "cn-north-9",
		Bucket:    "zhiqiyun-private",
		AccessKey: "access",
		SecretKey: "secret",
		UseSSL:    true,
	})
	if err != nil {
		t.Fatalf("build Huawei OBS provider: %v", err)
	}
	if _, ok := provider.(*huaweiOBSProvider); !ok {
		t.Fatalf("provider type = %T, want *huaweiOBSProvider", provider)
	}

	signed, err := provider.CreatePresignedDownloadURL(context.Background(), "tenants/t1/test.txt", time.Minute)
	if err != nil {
		t.Fatalf("presign download: %v", err)
	}
	parsed, err := url.Parse(signed)
	if err != nil {
		t.Fatalf("parse presigned URL: %v", err)
	}
	if parsed.Hostname() != "zhiqiyun-private.obs.cn-north-9.myhuaweicloud.com" {
		t.Fatalf("presigned URL host = %q", parsed.Host)
	}
	if parsed.Query().Get("AccessKeyId") == "" || parsed.Query().Get("Signature") == "" {
		t.Fatalf("presigned URL does not use native OBS signature: %s", signed)
	}
	if parsed.Query().Get("X-Amz-Signature") != "" {
		t.Fatalf("presigned URL unexpectedly uses AWS signature: %s", signed)
	}

	uploadURL, err := provider.(*huaweiOBSProvider).createSignedURL(
		context.Background(),
		huaweiobs.HttpMethodPut,
		"tenants/t1/test.jpg",
		time.Minute,
		map[string]string{"Content-Type": "image/jpeg"},
	)
	if err != nil {
		t.Fatalf("presign upload: %v", err)
	}
	uploadParsed, err := url.Parse(uploadURL)
	if err != nil {
		t.Fatalf("parse presigned upload URL: %v", err)
	}
	if uploadParsed.Query().Get("Signature") == "" {
		t.Fatalf("presigned upload URL has no OBS signature: %s", uploadURL)
	}
}

func TestHuaweiOBSProviderRequiresRegion(t *testing.T) {
	_, err := (S3ProviderFactory{}).Build(Config{
		Provider: "huawei_obs", Endpoint: "https://obs.cn-north-9.myhuaweicloud.com",
		Bucket: "zhiqiyun-private", AccessKey: "access", SecretKey: "secret",
	})
	if err == nil {
		t.Fatal("build Huawei OBS provider without region succeeded")
	}
}
