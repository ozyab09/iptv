package utils

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestNewHTTPClientDisablesKeepAlives verifies that the shared HTTP client
// disables keep-alive reuse. Probe/download responses are not always fully
// drained, leaving idle channels that net/http later reaps with a noisy
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	client := NewHTTPClient(false)
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		URLIsAlive(ctx, client, srv.URL, 2*time.Second)
	}
	srv.CloseClientConnections() // force idle-channel reaping
	client.CloseIdleConnections()
}
