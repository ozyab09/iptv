package s3

import (
	"os"
	"strings"
	"testing"
)

func TestUploadToS3InvalidEndpoint(t *testing.T) {
	err := UploadToS3("content", "bucket", "key", "", "us-east-1", "")
	if err == nil {
		t.Error("expected error for empty endpoint")
	}
	if !strings.Contains(err.Error(), "invalid S3 endpoint URL") {
		t.Errorf("expected 'invalid S3 endpoint URL' error, got: %v", err)
	}
}

func TestUploadFileToS3InvalidEndpoint(t *testing.T) {
	err := UploadFileToS3("file.txt", "bucket", "key", "", "bad-url", "us-east-1", "")
	if err == nil {
		t.Error("expected error for invalid endpoint")
	}
}

func TestUploadArchiveToS3InvalidEndpoint(t *testing.T) {
	_, err := UploadArchiveToS3("content", "bucket", "key", "", "us-east-1")
	if err == nil {
		t.Error("expected error for empty endpoint")
	}
}

func TestNewS3ClientNoCreds(t *testing.T) {
	// Save original env vars
	origKey := os.Getenv("AWS_ACCESS_KEY_ID")
	origSecret := os.Getenv("AWS_SECRET_ACCESS_KEY")
	defer func() {
		os.Setenv("AWS_ACCESS_KEY_ID", origKey)
		os.Setenv("AWS_SECRET_ACCESS_KEY", origSecret)
	}()

	os.Setenv("AWS_ACCESS_KEY_ID", "test-key")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret")

	client, err := newS3Client("https://storage.example.com", "ru-central1")
	if err != nil {
		t.Fatalf("newS3Client failed: %v", err)
	}
	if client == nil {
		t.Error("expected non-nil client")
	}
}
