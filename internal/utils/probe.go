package utils

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// knownHTMLSignatures are byte sequences that indicate an HTML response
// (captcha, auth page, error page) rather than a real stream or manifest.
var knownHTMLSignatures = [][]byte{
	[]byte("<!DOCTYPE html"),
	[]byte("<!doctype html"),
	[]byte("<html"),
	[]byte("<body"),
	[]byte("captcha"),
	[]byte("CAPTCHA"),
	[]byte("access denied"),
	[]byte("Access Denied"),
	[]byte("403 Forbidden"),
	[]byte("authentication required"),
	[]byte("Authorization Required"),
	[]byte("unauthorized"),
	[]byte("Unauthorised"),
	[]byte("need cookie"),
	[]byte("COOKIE"),
}

// contentTypeIsStream returns true if the Content-Type suggests video/audio/stream content,
// an HLS manifest, or at least something plausibly media-related.
func contentTypeIsStream(ct string) bool {
	ct = strings.ToLower(ct)
	// Direct video codecs
	if strings.Contains(ct, "video/") || strings.Contains(ct, "audio/") {
		return true
	}
	// HLS / MPEG-TS transport streams
	if ct == "application/x-mpegurl" || ct == "application/vnd.apple.mpegurl" ||
		ct == "application/mpegurl" || ct == "audio/mpegurl" {
		return true
	}
	// WebM / other web media
	if ct == "video/webm" || ct == "audio/webm" ||
		strings.Contains(ct, "video/mp4") || strings.Contains(ct, "video/ogg") {
		return true
	}
	return false
}

// contentTypeIsForbidden returns true if the Content-Type indicates static/HTML content
// that is never a valid IPTV stream.
func contentTypeIsForbidden(ct string) bool {
	ct = strings.ToLower(ct)
	// HTML is always suspicious for a stream probe
	if strings.Contains(ct, "text/html") || strings.Contains(ct, "application/xhtml") {
		return true
	}
	// JSON/XML API responses / plain text / CSS / JS — never video
	for _, prefix := range []string{"application/json", "application/xml", "text/plain", "text/css", "text/javascript", "text/calendar", "image/", "application/javascript"} {
		if strings.HasPrefix(ct, prefix) {
			return true
		}
	}
	return false
}

// hasHTMLBody inspects the first ~512 bytes of data and returns true
// when they look like HTML / captcha / auth / error pages.
func hasHTMLBody(data []byte) bool {
	for _, sig := range knownHTMLSignatures {
		if bytesContains(data, sig) {
			return true
		}
	}
	return false
}

// bytesContains reports whether data contains sub as a substring; handles
// the case where the io.LimitedReader may have consumed only part of data.
func bytesContains(data, sub []byte) bool {
	return len(data) >= len(sub) && findSubsequence(data, sub)
}

func findSubsequence(data, sub []byte) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i <= len(data)-len(sub); i++ {
		match := true
		for j := 0; j < len(sub); j++ {
			if data[i+j] != sub[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// probeUserAgent is sent when a candidate carries no explicit user-agent.
// Many IPTV servers reject the default Go user-agent, so a generic browser UA
// is used for best reachability.
const probeUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

// probeRetryPasses is the number of additional passes over dead candidates
// after the initial probe. Each pass retries only URLs that were unreachable
// in the previous pass (network hiccups, short-lived server restarts, etc.).
const probeRetryPasses = 2

var probeLogger = NewSanitizedLoggerWithPrefix("[probe]")

// ProbeCandidate is a URL to probe together with optional headers extracted
// from the playlist entry (e.g. #EXTVLCOPT:http-user-agent / http-referrer).
type ProbeCandidate struct {
	URL       string
	UserAgent string
	Referer   string
}

// hlsManifestExtension matches .m3u8 files — HLS manifest playlists.
const hlsManifestExtension = ".m3u8"

// urlHasM3u8Path checks whether a URL path segment looks like an HLS manifest
// (.m3u8 in the path or as a suffix). Unlike simple HasSuffix, this catches
// URLs such as /m3u-stream/xxx/index.m3u8?
func urlHasM3u8Path(u string) bool {
	lower := strings.ToLower(u)
	if strings.HasSuffix(lower, hlsManifestExtension) {
		return true
	}
	// Also catch .../something.m3u8?... (query-param-ed m3u8 URLs)
	if idx := strings.Index(lower, ".m3u8"); idx > 0 {
		// Verify it's a path component, not inside query tokens
		// A safe heuristic: make sure there's no 'http' before the match
		remaining := lower[:idx]
		if !strings.Contains(remaining, "http") || strings.LastIndex(remaining, ".m3u8") == idx {
			return true
		}
	}
	return false
}

// URLIsAlive reports whether url responds to an availability check:
// a HEAD request is tried first; servers that reject HEAD are retried with a
// GET + "Range: bytes=0-0". Any final status in [200, 400) counts as alive.
// For HLS manifests (.m3u8), a second step probes the first video segment
// (first *.ts URL found in the manifest); the channel is alive only if both
// manifest and segment respond 2xx. This prevents false-positives where the
// manifest is reachable but actual segments require auth or are broken.
// Non-HTTP(S) schemes (e.g. rtmp://) cannot be probed and are treated as alive.
func URLIsAlive(ctx context.Context, client *http.Client, url string, timeout time.Duration) bool {
	return URLIsAliveCandidate(ctx, client, ProbeCandidate{URL: url}, timeout)
}

// URLIsAliveCandidate is URLIsAlive with per-candidate request headers.
func URLIsAliveCandidate(ctx context.Context, client *http.Client, cand ProbeCandidate, timeout time.Duration) bool {
	if !strings.HasPrefix(cand.URL, "http://") && !strings.HasPrefix(cand.URL, "https://") {
		return true
	}

	// HLS manifests: two-step probe — manifest + first segment.
	if strings.HasSuffix(strings.ToLower(cand.URL), hlsManifestExtension) {
		return hlsProbe(ctx, client, cand, timeout)
	}

	// If URL path looks like an HLS manifest even with query params, try HLS probe first.
	if urlHasM3u8Path(cand.URL) {
		if hlsProbe(ctx, client, cand, timeout) {
			return true
		}
		// Fall through to regular probe — some servers serve m3u8 via a different
		// path without .m3u8 suffix when probed differently.
	}

	// Standard probe for plain URLs: HEAD → GET+Range fallback.
	if probeWithMethod(ctx, client, cand, http.MethodHead, timeout, nil) {
		return true
	}
	return probeWithMethod(ctx, client, cand, http.MethodGet, timeout, func(req *http.Request) {
		req.Header.Set("Range", "bytes=0-0")
	})
}

// probeWithMethod performs a single request and reports whether the final
// response looks like a real stream resource (status 200 + plausible
// Content-Type + body is not short HTML/captcha/error page).
func probeWithMethod(ctx context.Context, client *http.Client, cand ProbeCandidate, method string, timeout time.Duration, headerFn func(*http.Request)) bool {
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, method, cand.URL, nil)
	if err != nil {
		return false
	}
	ua := cand.UserAgent
	if ua == "" {
		ua = probeUserAgent
	}
	req.Header.Set("User-Agent", ua)
	if cand.Referer != "" {
		req.Header.Set("Referer", cand.Referer)
	}
	if headerFn != nil {
		headerFn(req)
	}

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	// --- Strict status gate ---
	// Only 2xx is acceptable; 3xx means we followed redirects and should
	// evaluate the final destination on its own merits below.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false
	}

	// Read up to 1 KiB of body to inspect it (enough for HTML detection
	// but does not hang on large video streams).
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if err != nil {
		// Could not read body — treat as dead.
		return false
	}

	// Gate 1: forbidden content types (HTML, JSON, plain text…)
	if contentTypeIsForbidden(resp.Header.Get("Content-Type")) {
		return false
	}

	// Gate 2: HTML/captcha/auth signatures in body
	if hasHTMLBody(data) {
		return false
	}

	// Gate 3: empty body — no stream would send zero bytes in response
	if len(data) == 0 {
		return false
	}

	// Gate 4: body too small to be anything useful (< 4 bytes is noise)
	if len(data) < 4 {
		return false
	}

	return true
}

// hlsProbe performs a two-step liveness check for HLS channels:
//  1. Download the .m3u8 manifest.
//  2. Extract the first *.ts segment URL and probe it with HEAD/GET fallback.
//     Both steps must succeed for the channel to be considered alive.
func hlsProbe(ctx context.Context, client *http.Client, cand ProbeCandidate, timeout time.Duration) bool {
	manifest, err := downloadManifest(ctx, client, cand, timeout)
	if err != nil || manifest == "" {
		probeLogger.Debug("HLS probe [%s]: manifest failed (%v) or empty", cand.URL, err)
		return false
	}

	segURL := findFirstSegment(manifest)
	if segURL == "" {
		probeLogger.Debug("HLS probe [%s]: no .ts segments in manifest", cand.URL)
		return false
	}

	ok := probeURL(ctx, client, segURL, cand.UserAgent, cand.Referer, timeout)
	if ok {
		probeLogger.Debug("HLS probe [%s]: manifest OK, segment %s alive", cand.URL, segURL)
	} else {
		probeLogger.Debug("HLS probe [%s]: manifest OK, segment %s dead", cand.URL, segURL)
	}
	return ok
}

// downloadManifest fetches an HLS .m3u8 manifest within the given timeout.
func downloadManifest(ctx context.Context, client *http.Client, cand ProbeCandidate, timeout time.Duration) (string, error) {
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, cand.URL, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	ua := cand.UserAgent
	if ua == "" {
		ua = probeUserAgent
	}
	req.Header.Set("User-Agent", ua)
	if cand.Referer != "" {
		req.Header.Set("Referer", cand.Referer)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("manifest GET failed: %w", err)
	}
	defer resp.Body.Close()

	// Strict 2xx gate — match probeWithMethod behavior.
	// 3xx means an intermediate redirect; accept only final 200-OK responses.
	// Body/content inspection below catches anything non-stream even at 200.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("manifest returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024)) // max 2 MiB
	if err != nil {
		return "", fmt.Errorf("read manifest body: %w", err)
	}

	// Gate: reject forbidden content types (HTML error pages served at 200)
	if contentTypeIsForbidden(resp.Header.Get("Content-Type")) {
		return "", fmt.Errorf("manifest returned forbidden content-type %q", resp.Header.Get("Content-Type"))
	}

	// Gate: reject HTML/captcha/auth signatures in body
	if hasHTMLBody(body) {
		return "", fmt.Errorf("manifest body contains HTML/error signature")
	}

	// Gate: empty manifest is useless
	if len(body) == 0 {
		return "", fmt.Errorf("manifest is empty")
	}

	return string(body), nil
}

// findFirstSegment extracts the first media segment URL (*.ts) from an m3u8
// manifest body. Skips blank lines, comments (#), EXT* directives, and local
// paths. Returns empty string if none found.
func findFirstSegment(manifest string) string {
	for _, line := range strings.Split(manifest, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Must start with http(s) and end with .ts (with optional query params).
		if len(line) < 4 {
			continue
		}
		hasHTTP := strings.HasPrefix(line, "http://") || strings.HasPrefix(line, "https://")
		if !hasHTTP {
			continue
		}
		if strings.HasSuffix(line, ".ts") {
			return line
		}
		// Check .ts with query string: cut after ?.ts prefix.
		qi := strings.Index(line, "?")
		if qi > 0 && strings.HasSuffix(line[:qi], ".ts") {
			return line
		}
	}
	return ""
}

// probeURL performs a HEAD→GET fallback probe on a single URL using the
// provided user-agent and referer.
func probeURL(ctx context.Context, client *http.Client, url, ua, referer string, timeout time.Duration) bool {
	cand := ProbeCandidate{URL: url, UserAgent: ua, Referer: referer}
	if probeWithMethod(ctx, client, cand, http.MethodHead, timeout, nil) {
		return true
	}
	return probeWithMethod(ctx, client, cand, http.MethodGet, timeout, func(req *http.Request) {
		req.Header.Set("Range", "bytes=0-0")
	})
}

// ProbeURLs checks the availability of a batch of URLs concurrently and returns
// a map url → alive. Duplicate URLs are probed once (with the default browser
// user-agent). concurrency bounds the number of in-flight requests; timeout is
// applied per request attempt.
func ProbeURLs(ctx context.Context, urls []string, concurrency int, timeout time.Duration, skipSSLVerify bool) map[string]bool {
	cands := make([]ProbeCandidate, len(urls))
	for i, u := range urls {
		cands[i] = ProbeCandidate{URL: u}
	}
	return ProbeCandidates(ctx, cands, concurrency, timeout, skipSSLVerify)
}

// ProbeCandidates checks a batch of candidates concurrently and returns a map
// url → alive. Duplicate URLs are probed once (first candidate's headers win).
func ProbeCandidates(ctx context.Context, candidates []ProbeCandidate, concurrency int, timeout time.Duration, skipSSLVerify bool) map[string]bool {
	result := make(map[string]bool)
	if len(candidates) == 0 {
		return result
	}
	if concurrency < 1 {
		concurrency = 1
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	// De-duplicate by URL.
	unique := make([]ProbeCandidate, 0, len(candidates))
	seen := make(map[string]bool, len(candidates))
	for _, c := range candidates {
		c.URL = strings.TrimSpace(c.URL)
		if c.URL == "" || seen[c.URL] {
			continue
		}
		seen[c.URL] = true
		unique = append(unique, c)
	}

	client := NewHTTPClient(skipSSLVerify)

	workers := concurrency
	if workers > len(unique) {
		workers = len(unique)
	}

	probeLogger.Info("Probe: checking availability of %d candidate URLs (concurrency %d)", len(unique), workers)

	start := time.Now()
	var done, aliveCount atomic.Int64
	var progressMu sync.Mutex
	var lastDecile int64

	queue := make(chan ProbeCandidate)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for cand := range queue {
				ok := URLIsAliveCandidate(ctx, client, cand, timeout)
				mu.Lock()
				result[cand.URL] = ok
				mu.Unlock()
				done.Add(1)
				if ok {
					aliveCount.Add(1)
				}

				var line string
				progressMu.Lock()
				if decile := done.Load() * 10 / int64(len(unique)); decile > lastDecile {
					lastDecile = decile
					line = probeProgressLine(int(decile*10), int(done.Load()), len(unique), int(aliveCount.Load()), time.Since(start))
				}
				progressMu.Unlock()
				if line != "" {
					probeLogger.Info("%s", line)
				}
			}
		}()
	}

	sendLoop:
	for _, cand := range unique {
		select {
		case queue <- cand:
		case <-ctx.Done():
			break sendLoop
		}
	}
	close(queue)
	wg.Wait()

	// Retry passes: re-probe dead candidates up to probeRetryPasses times.
	for pass := 1; pass <= probeRetryPasses; pass++ {
		if ctx.Err() != nil {
			break
		}
		var dead []ProbeCandidate
		for _, c := range unique {
			if ok, exists := result[c.URL]; exists && !ok {
				dead = append(dead, c)
			}
		}
		if len(dead) == 0 {
			break
		}

		probeLogger.Info("Probe retry pass %d/%d: re-checking %d previously dead URLs", pass, probeRetryPasses, len(dead))

		retryStart := time.Now()
		retryDone := atomic.Int64{}
		retryAlive := atomic.Int64{}
		retryProgressMu := sync.Mutex{}
		retryLastDecile := int64(0)

		retryQueue := make(chan ProbeCandidate)
		var retryWg sync.WaitGroup

		retryWorkers := concurrency
		if retryWorkers > len(dead) {
			retryWorkers = len(dead)
		}

		for i := 0; i < retryWorkers; i++ {
			retryWg.Add(1)
			go func() {
				defer retryWg.Done()
				for cand := range retryQueue {
					ok := URLIsAliveCandidate(ctx, client, cand, timeout)
					if ok {
						mu.Lock()
						result[cand.URL] = true
						mu.Unlock()
					}
					retryDone.Add(1)
					if ok {
						retryAlive.Add(1)
					}

					var line string
					retryProgressMu.Lock()
					if decile := retryDone.Load() * 10 / int64(len(dead)); decile > retryLastDecile {
						retryLastDecile = decile
						line = probeProgressLine(int(decile*10), int(retryDone.Load()), len(dead), int(retryAlive.Load()), time.Since(retryStart))
					}
					retryProgressMu.Unlock()
					if line != "" {
						probeLogger.Info("%s", line)
					}
				}
			}()
		}

		retrySendLoop:
		for _, c := range dead {
			select {
			case retryQueue <- c:
			case <-ctx.Done():
				break retrySendLoop
			}
		}
		close(retryQueue)
		retryWg.Wait()

		if n := retryAlive.Load(); n > 0 {
			probeLogger.Info("Probe retry pass %d/%d: recovered %d of %d dead URLs", pass, probeRetryPasses, n, len(dead))
		}
	}

	return result
}

// probeProgressLine formats a milestone availability-probe status line.
func probeProgressLine(percent, done, total, alive int, elapsed time.Duration) string {
	return fmt.Sprintf("Probe progress: %d%% (%d/%d URLs, alive %d of %d), elapsed %s",
		percent, done, total, alive, done, elapsed.Round(time.Second))
}
