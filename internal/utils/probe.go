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

// probeUserAgent is sent when a candidate carries no explicit user-agent.
// Many IPTV servers reject the default Go user-agent, so a generic browser UA
// is used for best reachability.
const probeUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

var probeLogger = NewSanitizedLoggerWithPrefix("[probe]")

// ProbeCandidate is a URL to probe together with optional headers extracted
// from the playlist entry (e.g. #EXTVLCOPT:http-user-agent / http-referrer).
type ProbeCandidate struct {
	URL       string
	UserAgent string
	Referer   string
}

// URLIsAlive reports whether url responds to an availability check:
// a HEAD request is tried first; servers that reject HEAD are retried with a
// GET + "Range: bytes=0-0". Any final status in [200, 400) counts as alive.
// Non-HTTP(S) schemes (e.g. rtmp://) cannot be probed and are treated as alive.
func URLIsAlive(ctx context.Context, client *http.Client, url string, timeout time.Duration) bool {
	return URLIsAliveCandidate(ctx, client, ProbeCandidate{URL: url}, timeout)
}

// URLIsAliveCandidate is URLIsAlive with per-candidate request headers.
func URLIsAliveCandidate(ctx context.Context, client *http.Client, cand ProbeCandidate, timeout time.Duration) bool {
	if !strings.HasPrefix(cand.URL, "http://") && !strings.HasPrefix(cand.URL, "https://") {
		return true
	}
	if probeWithMethod(ctx, client, cand, http.MethodHead, timeout, nil) {
		return true
	}
	// Fallback for servers that reject HEAD (405, connection reset, ...).
	return probeWithMethod(ctx, client, cand, http.MethodGet, timeout, func(req *http.Request) {
		req.Header.Set("Range", "bytes=0-0")
	})
}

// probeWithMethod performs a single request and reports whether the final
// status code indicates the resource is reachable.
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

	// Drain a small amount so the connection can be reused.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))

	return resp.StatusCode >= 200 && resp.StatusCode < 400
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
	// Прогресс логируется при пересечении каждого 10%-рубежа (10, 20, ... 100),
	// вместе с накопленной статистикой: сколько из скольких ресурсов живы.
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

	return result
}

// probeProgressLine formats a milestone availability-probe status line: percent
// (10% steps), probed/total, alive-of-probed and elapsed wall time.
func probeProgressLine(percent, done, total, alive int, elapsed time.Duration) string {
	return fmt.Sprintf("Probe progress: %d%% (%d/%d URLs, alive %d of %d), elapsed %s",
		percent, done, total, alive, done, elapsed.Round(time.Second))
}
