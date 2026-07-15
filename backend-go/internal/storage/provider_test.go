package storage

import (
	"context"
	"net/url"
	"testing"
	"time"
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
