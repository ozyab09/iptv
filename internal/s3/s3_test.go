package s3

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
)

// newTestClient builds an S3 client pointed at a mock HTTP endpoint. Static
// credentials are injected via env vars so no real cloud access is needed.
func newTestClient(t *testing.T, endpoint string) *awss3.Client {
	t.Helper()
	t.Setenv("AWS_ACCESS_KEY_ID", "test-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret")

	ctx := context.Background()
	client, err := NewClient(ctx, endpoint, "us-east-1")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	return client
}

// requestLog records everything the mock S3 endpoint received.
type requestLog struct {
	mu       sync.Mutex
	method   string
	path     string
	body     []byte
	headers  http.Header
	requests int
}

// handle serves a successful (200) response and records the request.
func (l *requestLog) handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.method = r.Method
	l.path = r.URL.Path
	l.body = body
	l.headers = r.Header.Clone()
	l.requests++
	w.WriteHeader(http.StatusOK)
}

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

func TestUploadToS3Success(t *testing.T) {
	log := &requestLog{}
	server := httptest.NewServer(http.HandlerFunc(log.handle))
	defer server.Close()

	client := newTestClient(t, server.URL)
	content := "#EXTM3U\n#EXTINF:-1,Channel\nhttp://example.com/1"
	err := UploadToS3(context.Background(), client, content, "test-bucket", "playlist.m3u", "")
	if err != nil {
		t.Fatalf("UploadToS3 failed: %v", err)
	}

	log.mu.Lock()
	defer log.mu.Unlock()
	if log.method != http.MethodPut {
		t.Errorf("expected PUT, got %s", log.method)
	}
	// Path-style addressing: /bucket/key
	if log.path != "/test-bucket/playlist.m3u" {
		t.Errorf("expected path /test-bucket/playlist.m3u, got %s", log.path)
	}
	if string(log.body) != content {
		t.Errorf("body mismatch: got %q, want %q", log.body, content)
	}
	if ct := log.headers.Get("Content-Type"); ct != "application/x-mpegurl" {
		t.Errorf("expected default Content-Type application/x-mpegurl, got %q", ct)
	}
	if v := log.headers.Get("x-amz-meta-uploaded-by"); v != "iptv-m3u-filter" {
		t.Errorf("expected uploaded-by metadata, got %q", v)
	}
	if v := log.headers.Get("x-amz-meta-upload-timestamp"); v == "" {
		t.Error("expected upload-timestamp metadata")
	}
}

func TestUploadToS3CustomContentType(t *testing.T) {
	log := &requestLog{}
	server := httptest.NewServer(http.HandlerFunc(log.handle))
	defer server.Close()

	client := newTestClient(t, server.URL)
	if err := UploadToS3(context.Background(), client, "x", "test-bucket", "k", "text/plain"); err != nil {
		t.Fatalf("UploadToS3 failed: %v", err)
	}

	log.mu.Lock()
	defer log.mu.Unlock()
	if ct := log.headers.Get("Content-Type"); ct != "text/plain" {
		t.Errorf("expected Content-Type text/plain, got %q", ct)
	}
}

func TestUploadToS3ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := UploadToS3(context.Background(), client, "content", "test-bucket", "key", "")
	if err == nil {
		t.Fatal("expected error when server rejects upload")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("expected error to reference the 403 rejection, got: %v", err)
	}
}

func TestUploadFileToS3Success(t *testing.T) {
	log := &requestLog{}
	server := httptest.NewServer(http.HandlerFunc(log.handle))
	defer server.Close()

	dir := t.TempDir()
	fileContent := "#EXTM3U\n#EXTINF:-1,Channel\nhttp://example.com/1"
	if err := os.WriteFile(dir+"/playlist.m3u", []byte(fileContent), 0644); err != nil {
		t.Fatal(err)
	}

	client := newTestClient(t, server.URL)
	// Pass a bare filename; the function must resolve it inside outputDir.
	err := UploadFileToS3(context.Background(), client, "playlist.m3u", "test-bucket", "playlist.m3u", dir, "")
	if err != nil {
		t.Fatalf("UploadFileToS3 failed: %v", err)
	}

	log.mu.Lock()
	defer log.mu.Unlock()
	if log.path != "/test-bucket/playlist.m3u" {
		t.Errorf("expected path /test-bucket/playlist.m3u, got %s", log.path)
	}
	if string(log.body) != fileContent {
		t.Errorf("body mismatch: got %q, want %q", log.body, fileContent)
	}
	if v := log.headers.Get("x-amz-meta-source-file"); !strings.Contains(v, "playlist.m3u") {
		t.Errorf("expected source-file metadata to contain playlist.m3u, got %q", v)
	}
}

func TestUploadFileToS3MissingFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := UploadFileToS3(context.Background(), client, "missing.m3u", "test-bucket", "key", t.TempDir(), "")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestUploadArchiveToS3Success(t *testing.T) {
	log := &requestLog{}
	server := httptest.NewServer(http.HandlerFunc(log.handle))
	defer server.Close()

	client := newTestClient(t, server.URL)
	content := strings.Repeat("channel data ", 100)
	key, err := UploadArchiveToS3(context.Background(), client, content, "test-bucket", "playlist.m3u")
	if err != nil {
		t.Fatalf("UploadArchiveToS3 failed: %v", err)
	}

	log.mu.Lock()
	defer log.mu.Unlock()
	if !strings.HasPrefix(log.path, "/test-bucket/archive/") {
		t.Errorf("expected archive path prefix, got %s", log.path)
	}
	if !strings.HasSuffix(log.path, "_playlist.gz") {
		t.Errorf("expected archive key suffix _playlist.gz, got %s", log.path)
	}
	if ct := log.headers.Get("Content-Type"); ct != "application/gzip" {
		t.Errorf("expected Content-Type application/gzip, got %q", ct)
	}
	if v := log.headers.Get("x-amz-meta-original-size-kb"); v == "" {
		t.Error("expected original-size-kb metadata")
	}

	// The returned key must match the path the server actually received.
	if key != strings.TrimPrefix(log.path, "/test-bucket/") {
		t.Errorf("returned key %q does not match uploaded path %q", key, log.path)
	}

	// Body must be a valid gzip stream decompressing to the original content.
	gr, err := gzip.NewReader(bytes.NewReader(log.body))
	if err != nil {
		t.Fatalf("uploaded body is not valid gzip: %v", err)
	}
	defer gr.Close()
	decoded, err := io.ReadAll(gr)
	if err != nil {
		t.Fatalf("failed to decompress uploaded body: %v", err)
	}
	if string(decoded) != content {
		t.Errorf("decompressed content mismatch")
	}
}

func TestUploadBothSuccess(t *testing.T) {
	log := &requestLog{}
	server := httptest.NewServer(http.HandlerFunc(log.handle))
	defer server.Close()

	client := newTestClient(t, server.URL)
	if err := UploadBoth(context.Background(), client, "content", "test-bucket", "playlist.m3u", ""); err != nil {
		t.Fatalf("UploadBoth failed: %v", err)
	}

	log.mu.Lock()
	defer log.mu.Unlock()
	if log.requests != 2 {
		t.Errorf("expected 2 uploads (archive + direct), got %d", log.requests)
	}
}

func TestGetContentType(t *testing.T) {
	if got := getContentType(""); got == nil || *got != "application/x-mpegurl" {
		t.Errorf("default content type = %v, want application/x-mpegurl", got)
	}
	ct := "application/gzip"
	if got := getContentType(ct); got == nil || *got != ct {
		t.Errorf("explicit content type = %v, want %q", got, ct)
	}
}

func TestBuildS3Metadata(t *testing.T) {
	m := buildS3Metadata()
	if m["uploaded-by"] != "iptv-m3u-filter" {
		t.Errorf("uploaded-by = %q, want iptv-m3u-filter", m["uploaded-by"])
	}
	if m["upload-timestamp"] == "" {
		t.Error("expected upload-timestamp metadata")
	}

	withExtra := buildS3Metadata(map[string]string{"source-file": "x.m3u"})
	if withExtra["source-file"] != "x.m3u" {
		t.Errorf("extra metadata not merged: %v", withExtra)
	}
	if withExtra["uploaded-by"] != "iptv-m3u-filter" {
		t.Errorf("default metadata lost when merging extras: %v", withExtra)
	}
}
