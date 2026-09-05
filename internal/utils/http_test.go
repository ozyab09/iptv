package utils

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestDownloadFileWithContextRejectsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	_, err := DownloadFileWithContext(context.Background(), server.URL, 1024, false)
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected HTTP status error, got %v", err)
	}
}

func TestDownloadFileWithContextEnforcesBodyLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "11")
		_, _ = w.Write([]byte("hello world"))
	}))
	defer server.Close()

	_, err := DownloadFileWithContext(context.Background(), server.URL, 10, false)
	if err == nil || !strings.Contains(err.Error(), "maximum allowed size") {
		t.Fatalf("expected size limit error, got %v", err)
	}
}

func TestDownloadFileWithContextStopsOnCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Hold the response open until the client gives up.
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	start := time.Now()
	done := make(chan error, 1)
	go func() {
		_, err := DownloadFileWithContext(ctx, server.URL, 1024, false)
		done <- err
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error after cancellation")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("download did not stop after context cancellation")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("cancellation was slow: %s", elapsed)
	}
}

func TestDownloadFileToPathWithContextRejectsHTTPErrorAndCleansUp(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer server.Close()

	dest := t.TempDir() + "/partial.bin"
	err := DownloadFileToPathWithContext(context.Background(), server.URL, dest, 1024, false)
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("expected HTTP status error, got %v", err)
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Fatalf("partial destination should be removed, stat err: %v", statErr)
	}
}

// TestDownloadWithContextAppliesDefaultDeadline verifies that a download
// without a caller deadline is bounded by DefaultDownloadTimeout instead of
// hanging on the server forever (the client-level timeout alone could stall a
// run for 30 minutes per attempt).
func TestDownloadWithContextAppliesDefaultDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Never respond; hang until the client gives up.
		<-r.Context().Done()
	}))
	defer server.Close()

	// Shrink the default so the test finishes quickly; restored on exit.
	old := DefaultDownloadTimeout
	DefaultDownloadTimeout = 200 * time.Millisecond
	defer func() { DefaultDownloadTimeout = old }()

	start := time.Now()
	err := downloadWithContext(context.Background(), server.URL, 1024, false, func(r io.Reader) error {
		_, err := copyLimited(io.Discard, r, 1024)
		return err
	})
	if err == nil {
		t.Fatal("expected error from hanging server")
	}
	// The default deadline must fire well before the hard 10m client timeout.
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("download did not respect the default deadline, took %s", elapsed)
	}
}

// TestDownloadWithContextRespectsCallerDeadline verifies that a caller-supplied
// deadline wins over the default one.
func TestDownloadWithContextRespectsCallerDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := downloadWithContext(ctx, server.URL, 1024, false, func(r io.Reader) error {
		_, err := copyLimited(io.Discard, r, 1024)
		return err
	})
	if err == nil {
		t.Fatal("expected error from hanging server")
	}
	if duration := time.Since(start); duration > 3*time.Second {
		t.Fatalf("caller deadline was not respected: %s", duration)
	}
}
