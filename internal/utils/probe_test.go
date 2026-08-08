package utils

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestURLIsAlive(t *testing.T) {
	var fallbackCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			w.WriteHeader(http.StatusOK)
		case "/moved":
			http.Redirect(w, r, "/ok", http.StatusFound)
		case "/notfound":
			w.WriteHeader(http.StatusNotFound)
		case "/nohead":
			if r.Method == http.MethodHead {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			fallbackCalls.Add(1)
			w.WriteHeader(http.StatusPartialContent)
		case "/error":
			w.WriteHeader(http.StatusInternalServerError)
		case "/slow":
			time.Sleep(200 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	client := NewHTTPClient(false)
	ctx := context.Background()

	tests := []struct {
		url     string
		timeout time.Duration
		want    bool
	}{
		{srv.URL + "/ok", time.Second, true},
		{srv.URL + "/moved", time.Second, true},
		{srv.URL + "/notfound", time.Second, false},
		{srv.URL + "/nohead", time.Second, true},  // HEAD rejected → GET fallback
		{srv.URL + "/error", time.Second, false},
		{srv.URL + "/slow", 50 * time.Millisecond, false}, // timeout
		{"rtmp://cdn.example.com/live/1", time.Second, true}, // non-HTTP → treated as alive
	}
	for _, tc := range tests {
		got := URLIsAlive(ctx, client, tc.url, tc.timeout)
		if got != tc.want {
			t.Errorf("URLIsAlive(%s, timeout=%v) = %v, want %v", tc.url, tc.timeout, got, tc.want)
		}
	}

	if fallbackCalls.Load() == 0 {
		t.Error("expected GET fallback to be used for /nohead")
	}
}

func TestProbeURLsConcurrentAndDedup(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path == "/dead" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	urls := []string{
		srv.URL + "/a",
		srv.URL + "/b",
		srv.URL + "/dead",
		srv.URL + "/a", // duplicate — must be probed only once
		"rtmp://x/y",   // non-HTTP: not probed, treated as alive
	}
	got := ProbeURLs(context.Background(), urls, 3, time.Second, false)

	if len(got) != 4 {
		t.Errorf("expected 4 results, got %d: %v", len(got), got)
	}
	if !got[srv.URL+"/a"] || !got[srv.URL+"/b"] {
		t.Errorf("expected /a and /b alive, got %v", got)
	}
	if got[srv.URL+"/dead"] {
		t.Error("expected /dead to be dead")
	}
	if !got["rtmp://x/y"] {
		t.Error("expected non-HTTP URL treated as alive")
	}
	// 3 unique HTTP URLs (duplicate /a not re-probed, rtmp not probed):
	// HEAD for /a, /b, /dead + GET fallback for /dead (404) = 4 requests.
	if calls.Load() != 4 {
		t.Errorf("expected 4 probe requests (3 HEAD + 1 GET fallback), got %d", calls.Load())
	}
}

func TestProbeCandidatesPassHeaders(t *testing.T) {
	var mu sync.Mutex
	head := make(map[string]struct{ ua, ref string })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		head[r.URL.Path] = struct{ ua, ref string }{r.Header.Get("User-Agent"), r.Header.Get("Referer")}
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cands := []ProbeCandidate{
		{URL: srv.URL + "/a", UserAgent: "WINK/1.130.1", Referer: "https://example.com/"},
		{URL: srv.URL + "/b"}, // no headers → default browser UA
	}
	got := ProbeCandidates(context.Background(), cands, 2, time.Second, false)

	if !got[srv.URL+"/a"] || !got[srv.URL+"/b"] {
		t.Errorf("unexpected results: %v", got)
	}

	mu.Lock()
	a, okA := head["/a"]
	b, okB := head["/b"]
	mu.Unlock()
	if !okA || !okB {
		t.Fatalf("expected both paths to be probed, got %v", head)
	}
	if a.ua != "WINK/1.130.1" {
		t.Errorf("expected per-candidate user-agent on /a, got %q", a.ua)
	}
	if a.ref != "https://example.com/" {
		t.Errorf("expected per-candidate referer on /a, got %q", a.ref)
	}
	if b.ua != probeUserAgent {
		t.Errorf("expected default browser UA on /b, got %q", b.ua)
	}
}

func TestProbeURLsEmpty(t *testing.T) {
	if got := ProbeURLs(context.Background(), nil, 5, time.Second, false); len(got) != 0 {
		t.Errorf("expected empty result, got %v", got)
	}
	if got := ProbeURLs(context.Background(), []string{"", " "}, 5, time.Second, false); len(got) != 0 {
		t.Errorf("expected empty result for blank URLs, got %v", got)
	}
}

func TestProbeURLsCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	urls := []string{srv.URL + "/a", srv.URL + "/b", srv.URL + "/c"}
	got := ProbeURLs(ctx, urls, 2, time.Second, false)
	// Cancellation may leave some URLs unprobed; must not panic and must not report dead for missing ones.
	if len(got) > len(urls) {
		t.Errorf("too many results: %v", got)
	}
	for _, ok := range got {
		if ok {
			t.Error("expected all probed results to be dead under cancelled context")
		}
	}
}
