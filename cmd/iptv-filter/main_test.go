package main

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ozyab/iptv/internal/m3u"
)

const happyPlaylist = `#EXTM3U
#EXTINF:-1 tvg-id="ch1" group-title="Новости",Channel One
http://example.com/1.m3u8
#EXTINF:-1 tvg-id="ch2" group-title="Общие",Channel Two
http://example.com/2.m3u8
`

// clearConfigEnv blanks every env var the pipeline reads so tests are hermetic
// and do not pick up the developer's .env / exported variables.
func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"M3U_SOURCE_URL", "S3_BUCKET_NAME", "S3_OBJECT_KEY", "S3_ENDPOINT_URL",
		"S3_REGION", "EPG_SOURCE_URL", "S3_EPG_KEY", "LOCAL_EPG_PATH",
		"EPG_RETENTION_DAYS", "OUTPUT_DIR", "CATEGORIES_FILE_PATH", "DRY_RUN",
		"SKIP_SSL_VERIFY", "PROBE_SOURCES", "PROBE_TIMEOUT_SECONDS",
		"PROBE_CONCURRENCY", "MAX_CHANNEL_VARIANTS",
		"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY",
	} {
		t.Setenv(key, "")
	}
}

func TestRunFailsForInvalidConfiguration(t *testing.T) {
	clearConfigEnv(t)
	if got := run(); got != 1 {
		t.Fatalf("run() = %d, want 1 for invalid configuration", got)
	}
}

func TestRunSucceedsInDryRun(t *testing.T) {
	clearConfigEnv(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(happyPlaylist))
	}))
	defer server.Close()

	outDir := t.TempDir()
	t.Setenv("M3U_SOURCE_URL", server.URL)
	t.Setenv("DRY_RUN", "true")
	t.Setenv("OUTPUT_DIR", outDir)

	if got := run(); got != 0 {
		t.Fatalf("run() = %d, want 0", got)
	}
	filtered, err := os.ReadFile(filepath.Join(outDir, "playlist.m3u"))
	if err != nil {
		t.Fatalf("read filtered playlist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "playlist-all.m3u")); err != nil {
		t.Errorf("expected playlist-all.m3u to be saved: %v", err)
	}
	// The filter must have kept both source channels (not filtered them out).
	if m3u.CountChannels(string(filtered)) != 2 {
		t.Errorf("expected 2 channels in filtered playlist, got:\n%s", filtered)
	}
	for _, name := range []string{"Channel One", "Channel Two"} {
		if !strings.Contains(string(filtered), name) {
			t.Errorf("expected %q in filtered playlist:\n%s", name, filtered)
		}
	}
}

func TestRunSucceedsInDryRunWithEPG(t *testing.T) {
	clearConfigEnv(t)
	start := time.Now().Add(1 * time.Hour).Format("20060102150405") + " +0000"
	stop := time.Now().Add(2 * time.Hour).Format("20060102150405") + " +0000"
	epgContent := fmt.Sprintf(`<?xml version="1.0"?><tv>
<channel id="ch1"><display-name>Channel One</display-name></channel>
<channel id="ch2"><display-name>Channel Two</display-name></channel>
<programme channel="ch1" start="%s" stop="%s"><title>News Hour</title></programme>
</tv>`, start, stop)

	mux := http.NewServeMux()
	mux.HandleFunc("/playlist.m3u", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(happyPlaylist))
	})
	mux.HandleFunc("/epg.xml.gz", func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		_, _ = gw.Write([]byte(epgContent))
		_ = gw.Close()
		_, _ = w.Write(buf.Bytes())
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	outDir := t.TempDir()
	t.Setenv("M3U_SOURCE_URL", server.URL+"/playlist.m3u")
	t.Setenv("EPG_SOURCE_URL", server.URL+"/epg.xml.gz")
	t.Setenv("S3_EPG_KEY", "epg.xml.gz")
	t.Setenv("DRY_RUN", "true")
	t.Setenv("OUTPUT_DIR", outDir)

	if got := run(); got != 0 {
		t.Fatalf("run() = %d, want 0", got)
	}
	epgOut, err := os.ReadFile(filepath.Join(outDir, "epg.xml-filtered.gz"))
	if err != nil {
		t.Fatalf("read filtered EPG: %v", err)
	}
	gz, err := gzip.NewReader(bytes.NewReader(epgOut))
	if err != nil {
		t.Fatalf("filtered EPG is not valid gzip: %v", err)
	}
	defer gz.Close()
	decoded, err := io.ReadAll(gz)
	if err != nil {
		t.Fatalf("read filtered EPG content: %v", err)
	}
	for _, want := range []string{`<channel id="ch1">`, `<channel id="ch2">`, "News Hour"} {
		if !strings.Contains(string(decoded), want) {
			t.Errorf("expected %q in filtered EPG:\n%s", want, decoded)
		}
	}
}

func TestRunFailsOnM3UHTTPError(t *testing.T) {
	clearConfigEnv(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	t.Setenv("M3U_SOURCE_URL", server.URL)
	t.Setenv("DRY_RUN", "true")
	t.Setenv("OUTPUT_DIR", t.TempDir())

	if got := run(); got != 1 {
		t.Fatalf("run() = %d, want 1 when M3U download returns HTTP error", got)
	}
}

// TestRunToleratesFailedM3USource verifies that a failing M3U source is
// skipped with a warning and the run continues with the remaining sources
// (the same tolerance EPG sources have). The merged playlist must still carry
// the #EXTM3U header even though the first source failed.
func TestRunToleratesFailedM3USource(t *testing.T) {
	clearConfigEnv(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/good.m3u", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(happyPlaylist))
	})
	mux.HandleFunc("/bad.m3u", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream unavailable", http.StatusServiceUnavailable)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	outDir := t.TempDir()
	// The failing source comes first so a naive "header from part 0 only"
	// merge would produce a header-less playlist.
	t.Setenv("M3U_SOURCE_URL", server.URL+"/bad.m3u,"+server.URL+"/good.m3u")
	t.Setenv("DRY_RUN", "true")
	t.Setenv("OUTPUT_DIR", outDir)

	if got := run(); got != 0 {
		t.Fatalf("run() = %d, want 0 when at least one M3U source succeeds", got)
	}
	filtered, err := os.ReadFile(filepath.Join(outDir, "playlist.m3u"))
	if err != nil {
		t.Fatalf("read filtered playlist: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(string(filtered)), "#EXTM3U") {
		t.Errorf("expected #EXTM3U header even when the first source fails:\n%s", filtered)
	}
	if m3u.CountChannels(string(filtered)) != 2 {
		t.Errorf("expected 2 channels from the good source, got:\n%s", filtered)
	}
}

func TestRunFailsWhenAllM3USourcesFail(t *testing.T) {
	clearConfigEnv(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	t.Setenv("M3U_SOURCE_URL", server.URL+"/a.m3u,"+server.URL+"/b.m3u")
	t.Setenv("DRY_RUN", "true")
	t.Setenv("OUTPUT_DIR", t.TempDir())

	if got := run(); got != 1 {
		t.Fatalf("run() = %d, want 1 when every M3U source fails", got)
	}
}

// TestBuildTelegramReportIncludesFailedPlaylistURL verifies that a failed
// playlist source URL reaches the Telegram report's failed-URLs list.
func TestBuildTelegramReportIncludesFailedPlaylistURL(t *testing.T) {
	r := buildTelegramReport(1000, "filtered", 0, 0, []string{"https://bad.example/pl.m3u", "https://bad.example/epg.xml.gz"})
	if len(r.FailedURLs) != 2 {
		t.Fatalf("expected 2 failed URLs, got: %v", r.FailedURLs)
	}
	if r.FailedURLs[0] != "https://bad.example/pl.m3u" {
		t.Errorf("expected the failed playlist URL first, got %q", r.FailedURLs[0])
	}
}

// TestMergePartsKeepsFirstHeaderFromAnyPart verifies mergeParts keeps the
// #EXTM3U header even when the first part is empty (a failed source).
func TestMergePartsKeepsFirstHeaderFromAnyPart(t *testing.T) {
	header := "#EXTM3U\n"
	part := header + "#EXTINF:-1 group-title=\"Общие\",Ch\nhttp://example.com/1.m3u8\n"
	merged := mergeParts([]string{"", part})
	if !strings.HasPrefix(merged, "#EXTM3U") {
		t.Errorf("expected merged output to start with #EXTM3U, got: %q", merged)
	}
	if got := strings.Count(merged, "#EXTM3U"); got != 1 {
		t.Errorf("expected exactly 1 #EXTM3U header, got %d", got)
	}
	if m3u.CountChannels(merged) != 1 {
		t.Errorf("expected 1 channel in merged output, got:\n%s", merged)
	}
}

func TestRunFailsOnEPGDownloadError(t *testing.T) {
	clearConfigEnv(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/playlist.m3u", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(happyPlaylist))
	})
	mux.HandleFunc("/epg.xml", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	t.Setenv("M3U_SOURCE_URL", server.URL+"/playlist.m3u")
	t.Setenv("EPG_SOURCE_URL", server.URL+"/epg.xml")
	t.Setenv("S3_EPG_KEY", "epg.xml.gz")
	t.Setenv("DRY_RUN", "true")
	t.Setenv("OUTPUT_DIR", t.TempDir())

	if got := run(); got != 1 {
		t.Fatalf("run() = %d, want 1 when EPG download fails", got)
	}
}

func TestRunFailsOnS3UploadError(t *testing.T) {
	clearConfigEnv(t)
	playlistServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(happyPlaylist))
	}))
	defer playlistServer.Close()

	s3Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer s3Server.Close()

	t.Setenv("M3U_SOURCE_URL", playlistServer.URL)
	// Empty string (not "true") keeps the pipeline in non-dry-run mode so it
	// actually attempts the S3 upload against the failing mock endpoint.
	t.Setenv("DRY_RUN", "")
	t.Setenv("OUTPUT_DIR", t.TempDir())
	t.Setenv("S3_BUCKET_NAME", "test-bucket")
	t.Setenv("S3_ENDPOINT_URL", s3Server.URL)
	t.Setenv("S3_REGION", "us-east-1")
	t.Setenv("AWS_ACCESS_KEY_ID", "test-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret")

	if got := run(); got != 1 {
		t.Fatalf("run() = %d, want 1 when S3 upload fails", got)
	}
}

// TestRunDryRunProbeSourcesSkipsProbing verifies that with PROBE_SOURCES=true in
// dry-run mode the pipeline still deduplicates duplicate channel variants by
// quality, but does NOT perform availability probing (the probe callback stays
// nil), so no network requests are made to the stream URLs.
func TestRunDryRunProbeSourcesSkipsProbing(t *testing.T) {
	clearConfigEnv(t)

	var mu sync.Mutex
	requests := 0

	// The playlist is built after the server exists so its stream URLs can
	// point back at the same server. If availability probing ran, the handler
	// would receive extra requests for /streams/*; the counter catches that.
	var playlist string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		if r.URL.Path == "/playlist.m3u" {
			_, _ = w.Write([]byte(playlist))
			return
		}
		// Stream URLs answer 200 so the test stays deterministic even if a
		// probe somehow ran — the request counter still detects it.
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// The two variants use different stream URLs on purpose: RemoveDuplicateURLs
	// (inside FilterContent) keeps entries with distinct URLs, so both reach the
	// quality-based DeduplicateByName step. "Channel X SD" sorts before
	// "Channel X UHD" alphabetically, but UHD has a higher quality rank.
	// Keeping UHD proves DeduplicateByName ranks by quality instead of just
	// keeping the first entry.
	playlist = `#EXTM3U
#EXTINF:-1 tvg-id="100" group-title="Новости",Channel X SD
` + server.URL + `/streams/sd.m3u8
#EXTINF:-1 tvg-id="100" group-title="Новости",Channel X UHD
` + server.URL + `/streams/uhd.m3u8
`

	outDir := t.TempDir()
	t.Setenv("M3U_SOURCE_URL", server.URL+"/playlist.m3u")
	t.Setenv("PROBE_SOURCES", "true")
	t.Setenv("DRY_RUN", "true")
	t.Setenv("OUTPUT_DIR", outDir)

	if got := run(); got != 0 {
		t.Fatalf("run() = %d, want 0", got)
	}

	mu.Lock()
	gotRequests := requests
	mu.Unlock()
	// Only the playlist download may hit the server — probing must be skipped.
	if gotRequests != 1 {
		t.Errorf("expected exactly 1 request (playlist download), got %d — availability probing ran in dry-run", gotRequests)
	}

	filtered, err := os.ReadFile(filepath.Join(outDir, "playlist.m3u"))
	if err != nil {
		t.Fatalf("read filtered playlist: %v", err)
	}
	if m3u.CountChannels(string(filtered)) != 1 {
		t.Errorf("expected 1 channel after quality dedup, got:\n%s", filtered)
	}
	if !strings.Contains(string(filtered), "uhd.m3u8") {
		t.Errorf("expected UHD variant to be kept by quality ranking, got:\n%s", filtered)
	}
	if strings.Contains(string(filtered), "sd.m3u8") {
		t.Errorf("expected SD variant to be removed by quality dedup, got:\n%s", filtered)
	}
}
