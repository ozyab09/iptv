# IPTV M3U Filter — Code Audit

Date: 2026-09-05
Author: Holla (AI audit)

## Executive Summary

Project is solid — well-documented, testable, idiomatic Go with smart streaming
design. Five issues need attention before the next production run.

### Top 5 Issues by Severity

| # | Severity | Issue |
|---|----------|-------|
| 1 | 🔴 CRITICAL | CI: `always()` on `filter-m3u` job runs S3 upload even when tests fail |
| 2 | 🔴 CRITICAL | HTTP client timeout is 30 min — slow server → 2+ hour run, no abort |
| 3 | 🟡 WARNING | OOM risk: 100 MB M3U held entirely in memory (strings.Split + copies) |
| 4 | 🟡 WARNING | No `-race` in CI — concurrent probe + multi-source download untested |
| 5 | 🟡 WARNING | `LocalFilteredPlaylistPath()` returns S3 key, not a safe local path |

---

## Detailed Findings

### 🔴 CI/CD

**F1: `always()` runs S3 upload on failed tests**
File: `.github/workflows/filter-m3u.yml`, line ~58
```yaml
if: github.event_name != 'pull_request' || always()
```
`always()` causes `filter-m3u` to run even when the `test` job fails.
Fix: `if: github.event_name != 'pull_request'` (remove `|| always()`).

**F2: Secrets exposed to test job**
File: `.github/workflows/filter-m3u.yml`, lines 11-22
AWS/M3U/S3 secrets are in top-level `env:` — available to the `test` job which
doesn't need them. Move all secret-based env vars into the `filter-m3u` job only.
The comment in the file acknowledges this partially, but credentials remain global.

**F3: No `-race` in CI**
The `test` job runs `go test ./... -v -count=1` but never `go test -race`.
The project uses goroutines heavily (concurrent downloads, probe workers).
Add `go test -race ./... -v -count=1` as a separate step or separate job.

---

### 🔴 Network / Timeouts

**F4: 30-minute HTTP timeout**
File: `internal/utils/http.go`, line `Timeout: 30 * time.Minute`
The default HTTP client uses a 30-minute timeout. Combined with 4 retries in
`downloadM3U` (main.go), a single slow/stuck M3U source can stall the run
for up to 2 hours. EPG sources are similar.

Fix: Set a per-request timeout of 2-5 minutes via context timeout, not a
client-level 30-minute blanket timeout. Or set `client.Timeout = 5 * time.Minute`.

---

### 🟡 Memory

**F5: M3U content held in memory with multiple copies**
File: `internal/m3u/processor.go` (`FilterContent`)
The 100 MB M3U is loaded into a single string, then:
1. `strings.Split(content, "\n")` — ~100 MB slice
2. `RemoveDuplicateURLs` — another split + map of all entries
3. `SortPlaylistAlphabetically` — stable sort
4. `AddEmojiByURL` — iteration + string formatting

Peak memory can exceed 1 GB. EPG already streams to disk (good), but M3U
does not. On this 1.9 GB box (gateway consuming ~466 MB), OOM is a real risk
with large playlists.

Fix: Consider streaming M3U processing or at least gc-friendly chunked approach.

---

### 🟡 Path Handling

**F6: `LocalFilteredPlaylistPath()` returns S3 key as local filename**
File: `internal/config/config.go`
```go
func (c *Config) LocalFilteredPlaylistPath() string { return c.s3FilteredKey }
```
If `S3_OBJECT_KEY` contains subdirectories (e.g. `playlists/playlist.m3u`),
the `saveFile` function in main.go will fail because `os.MkdirAll` is not called
for the parent directory of the S3 key.

---

### 🟡 Code Quality

**F7: `replaceGroupTitle` compiles regex on every call**
File: `internal/m3u/processor.go`, line 1332
```go
func replaceGroupTitle(extinfLine, newGroup string) string {
    re := regexp.MustCompile(`group-title="[^"]*"`)
    ...
}
```
This compiles a new `regexp.Regexp` on every invocation. Move to package level.

**F8: Dead code**
These functions exist but are only used by tests (never in production):
- `SaveFilteredEPGLocally` (epg/processor.go)
- `DownloadEPG` (epg/processor.go)
- `BuildEPGNameToIDMap` (epg/processor.go, in-memory version)
- `FilterEPGContent` (epg/processor.go, in-memory version)
- `DownloadM3U` (m3u/processor.go, without context)

Recommendation: Run `staticcheck` or mark with `// Deprecated` for clarity.

**F9: Silent error swallowing**
- `UploadArchiveToS3`: `gw.Close()` at line 155 — gzip writer error discarded
- `BuildEPGNameToIDMap`: on parse error, logs and returns empty map (no error)
- `fmt.Sscanf` in `extractSourceIdx` — return value ignored

**F10: Magic number without logging**
File: `internal/m3u/processor.go`, line 1194
```go
if len(line) > 10000 {
    continue
}
```
Lines >10,000 characters are silently dropped with no log message. This should
log a warning at least once.

**F11: Global test mutation risk**
File: `internal/telegram/notify.go`
```go
var retryAttempts = 3
var retryDelay    = 2 * time.Second
```
Tests mutate these globals. If tests run in parallel (they don't currently),
this would cause flaky failures. Acceptable for single-goroutine tests, but
fragile.

**F12: `analysis/gencat` pollutes test runs**
`analysis/gencat/main.go` has 0% test coverage and is a dev tool. It runs
during `go test ./...` and `go build ./...`. Move to `cmd/tools/` or a
separate module to keep the main build clean.

---

### 🟡 Tests

**F13: `utils` package has lowest coverage (74.1%)**
This package contains the most dangerous code: HTTP client, retry logic,
and URL probing. Priority areas to add tests:
- `downloadWithContext` error paths
- `probeWithMethod` with various HTTP status codes
- `CopyLimited` edge cases (exactly at limit, one byte over)

**F14: No fuzz tests for parsers**
`ParseChannelEntries` and `filterEPG` process untrusted external data.
Fuzz testing (`go test -fuzz`) would catch panics from malformed input.

**F15: cmd tests are slow (74s)**
`cmd/iptv-filter` tests take 74 seconds — mostly from HTTP mock servers
and retry delays. Add `-short` flag support for faster CI:
```go
if testing.Short() { t.Skip("skipping slow tests") }
```

---

## Test Coverage Summary

| Package | Coverage | Notes |
|---------|----------|-------|
| m3u | 92.4% | Excellent — core parser well-tested |
| telegram | 95.6% | Excellent — stdlib-only, clean |
| s3 | 90.4% | Good — covers upload paths |
| config | 83.6% | Good — validation thorough |
| cmd/iptv-filter | 82.6% | Good — integration tests |
| epg | 77.4% | Acceptable — streaming parser |
| **utils** | **74.1%** | **Needs improvement** — HTTP/retry/probe |
| analysis/gencat | 0.0% | Dev tool, not production code |

**Total test count: 109 tests across all packages.**

---

## What's Good

- **Deterministic output**: sorted keys, stable sorts, deterministic emoji
- **Streaming EPG**: never loads full EPG into memory (smart xml.Decoder)
- **Log sanitization**: URLs and AWS keys masked in all output
- **Size limits**: enforced at download time (100 MB M3U, 500 MB EPG)
- **Retry with jitter**: exponential backoff with bounded jitter on all network ops
- **Context cancellation**: SIGINT/SIGTERM-aware pipeline, interruptible retries
- **Telegram in stdlib**: no extra dependency for notifications
- **Thoughtful regex normalization**: Russian/English/emoji-aware, well-commented
- **Robust category pipeline**: deny-list → allow-list → keyword reclassify → fallback
