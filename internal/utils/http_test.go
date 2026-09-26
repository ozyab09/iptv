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

// TestNewHTTPClientDisablesKeepAlives verifies that the shared HTTP client
// disables keep-alive reuse. Probe responses are only status-checked, never
// fully drained, leaving idle channels that net/http later reaps with a noisy
// "Unsolicited response received on idle HTTP channel" log line (carrying the
// leftover playlist body). With DisableKeepAlives every connection is closed
// right after use, so the message can no longer appear.
func TestNewHTTPClientDisablesKeepAlives(t *testing.T) {
	client := NewHTTPClient(false)
	tr, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", client.Transport)
	}
	if !tr.DisableKeepAlives {
		t.Error("expected DisableKeepAlives=true to suppress idle-channel transport noise")
	}
}

// TestUnsolicitedResponseNoiseGone is an end-to-end reproduction of the CI
// noise: probe HLS-style responses without fully draining them, then force
// idle-channel cleanup. With keep-alives disabled, closing connections cannot
// produce the "Unsolicited response received on idle HTTP channel" log line.
func TestUnsolicitedResponseNoiseGone(t *testing.T) {
	// Large playlist-like body so a partially drained response leaves real
	// leftovers on a keep-alive connection.
	body := strings.Repeat("#EXT-X-STREAM-INF:PROGRAM-ID=1\nhttp://cdn.example/live/1\n", 500)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	client := NewHTTPClient(false)
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		URLIsAlive(ctx, client, server.URL, 2*time.Second)
	}
	server.CloseClientConnections() // force idle-channel reaping
	client.CloseIdleConnections()
}
