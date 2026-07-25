package s3

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestUploadToS3InvalidEndpoint(t *testing.T) {
	ctx := context.Background()
	err := UploadToS3(ctx, "content", "bucket", "key", "", "us-east-1", "")
	if err == nil {
		t.Error("expected error for empty endpoint")
	}
	if !strings.Contains(err.Error(), "invalid S3 endpoint URL") {
		t.Errorf("expected 'invalid S3 endpoint URL' error, got: %v", err)
	}
}

func TestUploadFileToS3InvalidEndpoint(t *testing.T) {
	ctx := context.Background()
	err := UploadFileToS3(ctx, "file.txt", "bucket", "key", "", "bad-url", "us-east-1", "")
	if err == nil {
		t.Error("expected error for invalid endpoint")
	}
}

func TestUploadArchiveToS3InvalidEndpoint(t *testing.T) {
	ctx := context.Background()
	_, err := UploadArchiveToS3(ctx, "content", "bucket", "key", "", "us-east-1")
	if err == nil {
		t.Error("expected error for empty endpoint")
	}
}

func TestNewS3ClientNoCreds(t *testing.T) {
	origKey := os.Getenv("AWS_ACCESS_KEY_ID")
	origSecret := os.Getenv("AWS_SECRET_ACCESS_KEY")
	defer func() {
		os.Setenv("AWS_ACCESS_KEY_ID", origKey)
		os.Setenv("AWS_SECRET_ACCESS_KEY", origSecret)
	}()

	os.Setenv("AWS_ACCESS_KEY_ID", "test-key")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret")

	client, err := newS3Client(context.Background(), "https://storage.example.com", "ru-central1")
	if err != nil {
		t.Fatalf("newS3Client failed: %v", err)
	}
	if client == nil {
		t.Error("expected non-nil client")
	}
}

func TestUploadBothInvalidEndpoint(t *testing.T) {
	ctx := context.Background()
	err := UploadBoth(ctx, "content", "bucket", "key", "", "us-east-1")
	if err == nil {
		t.Error("expected error for empty endpoint")
	}
}
