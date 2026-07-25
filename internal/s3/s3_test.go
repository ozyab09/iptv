package s3

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestNewClientInvalidEndpoint(t *testing.T) {
	ctx := context.Background()
	_, err := NewClient(ctx, "", "us-east-1")
	if err == nil {
		t.Error("expected error for empty endpoint")
	}
	if !strings.Contains(err.Error(), "invalid S3 endpoint URL") {
		t.Errorf("expected 'invalid S3 endpoint URL' error, got: %v", err)
	}
}

func TestNewClient(t *testing.T) {
	origKey := os.Getenv("AWS_ACCESS_KEY_ID")
	origSecret := os.Getenv("AWS_SECRET_ACCESS_KEY")
	defer func() {
		os.Setenv("AWS_ACCESS_KEY_ID", origKey)
		os.Setenv("AWS_SECRET_ACCESS_KEY", origSecret)
	}()

	os.Setenv("AWS_ACCESS_KEY_ID", "test-key")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret")

	ctx := context.Background()
	client, err := NewClient(ctx, "https://storage.example.com", "ru-central1")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	if client == nil {
		t.Error("expected non-nil client")
	}
}

func TestUploadToS3InvalidParams(t *testing.T) {
	ctx := context.Background()
	// Passing nil client should handle gracefully.
	err := UploadToS3(ctx, nil, "content", "bucket", "key", "text/plain")
	if err == nil {
		t.Error("expected error for nil client")
	}
}

func TestUploadArchiveToS3InvalidParams(t *testing.T) {
	ctx := context.Background()
	_, err := UploadArchiveToS3(ctx, nil, "content", "bucket", "key")
	if err == nil {
		t.Error("expected error for nil client")
	}
}

func TestUploadBothInvalidParams(t *testing.T) {
	ctx := context.Background()
	err := UploadBoth(ctx, nil, "content", "bucket", "key", "")
	if err == nil {
		t.Error("expected error for nil client")
	}
}
