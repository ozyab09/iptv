# AGENTS.md

## Project

IPTV M3U playlist filter: downloads M3U from HTTP URLs, filters by category (deny-list), normalizes names (remove `orig`, exclude regional `+N` variants), assigns unique emoji pairs to channels deterministically derived from stream URL (first emoji from hostname, second from path) via FNV-1a hash, optionally processes EPG, uploads to S3-compatible storage (Yandex Cloud).

## Language & runtime

Rewritten from Python to Go. Go 1.25+.

## Entry points

- `cmd/iptv-filter/main.go` — script entry point
- `go run ./cmd/iptv-filter/` — run directly (compiles to temp, executes)
- `make build` → `./bin/iptv-filter` (binary also available)

## Commands

```bash
go test ./... -v -count=1           # run all tests
go vet ./...                        # run vet
make build                          # build binary (bin/iptv-filter)
make run                            # go run (requires .env or export vars)
DRY_RUN=true make run               # dry-run (no S3 upload)
make clean                          # rm -rf output/ bin/
```

## Development conventions

- Go standard project layout (`cmd/`, `internal/`)
- AWS SDK v2 for S3 operations (client-based, reusable `*s3.Client`)
- Standard library `net/http` for HTTP (configurable SSL via `SKIP_SSL_VERIFY` env var)
- `encoding/xml` for EPG XML parsing (streaming `xml.Decoder` for 463MB+ XML)
- `compress/gzip` and `archive/zip` for compression
- Environment-based config via `internal/config/` (cached in struct on `New()`)
- Context propagation throughout: graceful shutdown on SIGINT/SIGTERM
- Sanitized logging via `internal/utils/logger.go` (masks URLs + AWS/Yandex keys)

## Architecture

```
iptv/
├── cmd/iptv-filter/main.go        # Entry point (pipeline: download→filter→EPG→upload)
├── internal/
│   ├── config/config.go           # Cached config from env vars with validation
│   ├── config/config_test.go      # Tests + filter list assertions
│   ├── m3u/processor.go           # M3U download, filtering, normalization pipeline
│   ├── m3u/m3u_test.go            # Unit tests for all M3U functions
│   ├── epg/processor.go           # EPG download, streaming XML filtering
│   ├── epg/epg_test.go            # EPG filter tests
│   ├── s3/upload.go               # S3 upload via AWS SDK v2 (reusable client)
│   ├── s3/s3_test.go              # S3 endpoint/credential tests
│   └── utils/
│       ├── http.go                # HTTP client (context-aware, configurable SSL)
│       ├── logger.go              # Sanitized logger (masks credentials)
│       ├── helper.go              # ToLowerSlice, NormalizeLineEndings
│       ├── retry.go               # Retry with exponential backoff
│       ├── compress.go            # GZip/ZIP decompression utilities
│       └── utils_test.go          # Retry + sanitization tests
├── .github/workflows/filter-m3u.yml  # CI: test+build → filter → artifacts
├── go.mod / go.sum
├── categories.txt                 # Channel metadata (group-title/tvg-id/tvg-rec overrides)
├── AGENTS.md                      # This file
└── README.md
```

## Module descriptions

### internal/config/config.go

**Config struct** — all env vars read ONCE in `New()` and cached for fast access:

| Method | Env var | Default |
|--------|---------|---------|
| `M3USourceURL()` | `M3U_SOURCE_URL` | — |
| `S3DefaultBucketName()` | `S3_BUCKET_NAME` | — |
| `S3FilteredPlaylistKey()` | `S3_OBJECT_KEY` | `playlist.m3u` |
| `S3AllCategoriesPlaylistKey()` | — | `playlist-all.m3u` |
| `S3EndpointURL()` | `S3_ENDPOINT_URL` | — |
| `S3Region()` | `S3_REGION` | `us-east-1` |
| `EPGSourceURL()` | `EPG_SOURCE_URL` | — |
| `S3EPGKey()` | `S3_EPG_KEY` | — |
| `EPGRetentionDays()` | `EPG_RETENTION_DAYS` | `3` |
| `OutputDir()` | `OUTPUT_DIR` | `output` |
| `CategoriesFilePath()` | `CATEGORIES_FILE_PATH` | — |
| `DryRun()` | `DRY_RUN` | `false` |
| `SkipSSLVerify()` | `SKIP_SSL_VERIFY` | `false` |
| `EnsureOutputDir()` | — | creates output dir via `os.MkdirAll` |

**Filter lists** (package-level vars):

- `CategoriesToRemove` — deny-list for exact category match (26 entries, case-insensitive). After dedup, only **normalized** variants remain — the normalization step strips leading numbers and trailing emojis before matching.
  - Adult/18+, Religion, Support/INFO
  - Anti-Russia/Ukraine (АнтиРОССИЙСКИЕ, 𝕐𝕜𝕡𝕒Їℍ𝕒, Українські, Украина, Наш Нет)
  - Sport (bold unicode: ℂп𝕠𝕡т, 186)
  - Music (bold unicode: РАДИО ТВ, 𝕄𝕦𝕤𝕚𝕔)
  - Service/Test (bold unicode: 𝕋𝕧ℤ𝕒𝕋𝕒𝕜, 32, TVS, TvZaTak, MavTV, aleks-u-romki*)
  - Cinema (bold unicode: 𝐊𝐢𝐧𝐨, 𝕂иℍ𝕠)
  - Regional: РОССИЯ+
- `CategoriesToRemoveSubstring` — ~100 substrings matched against group-title. Covers: sport, kids, music, religion, relax, fashion, anti-Russia/Ukraine, service/test, countries/regions (~60), cinema/series, specific TV shows
- `ChannelNamesToExclude` — channel names excluded by substring (Fashion, СПАС, Три ангела, ЛДПР, UA, Sports)
- `EPGExcludedCategories` — EPG categories excluded (default: `Кино`)
- `EPGExcludedChannelIDs` — 31 specific EPG channel IDs excluded
- `Validate()` — validates all required env vars, URL format, bucket/key length

### internal/m3u/processor.go

**M3U pipeline** (extracted into small functions for testability):

```
DownloadM3U → FilterContent (NormalizeLineEndings → filterEntry → dedup → sort → emoji)
```

Key exported functions:
- `DownloadM3U(url)` / `DownloadM3UWithContext(ctx, url, skipSSL)` — HTTP download with 100MB size limit
- `FilterContent()` — main pipeline: normalization, category filtering (exact + substring), name exclusion, regional suffix removal, numeric suffix removal, `orig` removal, dedup, sort, emoji
- `RemoveDuplicateURLs()` — deduplicates by URL, merges attributes (tvg-id, group-title, tvg-logo, tvg-rec), keeps longest name
- `SortPlaylistAlphabetically()` — A-Z by channel name (case-insensitive, stable sort)
- `AddEmojiByURL()` — appends FNV-1a based emoji pair (🔴🐱) to each channel name; first emoji derived from URL hostname, second from URL path; 100+×100+ pools = ~10,000+ combinations
- `AddTvgIDsToPlaylist()` — adds `tvg-id` from EPG name-to-id map
- `RemoveOrigSuffix()` — strips trailing " orig"
- `ParseCategoriesFile()` / `ApplyChannelMetadata()` — categories.txt override
- `CountChannels()` — counts #EXTINF entries

Filtering steps per entry:
1. Extract `group-title`, normalize it (strip leading numbers + trailing emojis)
2. Check exact match against `CategoriesToRemove` (case-insensitive)
3. Check substring match against `CategoriesToRemoveSubstring` (case-insensitive)
4. Check channel name against `ChannelNamesToExclude` (substring, case-insensitive)
5. Check for regional suffix (`+N`, `+N HD`, `+N (region)`)
6. Check for numeric suffix (`HD 50`, `Channel 25`)
7. Strip `orig` suffix

### internal/epg/processor.go

**EPG processing** (streaming XML parser, single-pass):

- `DownloadEPG(ctx, url, cfg)` — downloads with gzip/zip decompression, 500MB limit, context-aware
- `ExtractChannelInfoFromPlaylist()` — extracts `tvg-id` → category and channel name → category from M3U
- `BuildEPGNameToIDMap()` — builds lowercase display-name → channel-id map via `xml.Unmarshal` (small struct)
- `FilterEPGContent()` — **single-pass streaming** `xml.Decoder` parsing:
  - Pre-computes retention window (`time.Now()` once, not per programme)
  - First pass: builds EPG channel display-name map
  - Second pass eliminated (merged into one pass — channels come before programmes in XMLTV)
  - Filters channels by ID/name matching
  - Applies category exclusions and channel ID exclusions
  - Applies time-based retention (default 3 days)
  - Returns filtered XML with `xml.MarshalIndent`
- `SaveFilteredEPGLocally()` — saves with gzip compression if filename ends with `.gz`

### internal/s3/upload.go

**S3 upload** (client-based API, reusable `*s3.Client`):

- `NewClient(ctx, endpoint, region)` — creates reusable S3 client (validates endpoint)
- `UploadToS3(ctx, client, content, bucket, key, contentType)` — string → S3
- `UploadFileToS3(ctx, client, filePath, bucket, key, outputDir, contentType)` — local file → S3 (with fallback to output dir)
- `UploadArchiveToS3(ctx, client, content, bucket, baseKey)` — gzip-compressed → `archive/YYYY-MM-DD/HH-MM-SS-UUID_key.gz`
- `UploadBoth(ctx, client, content, bucket, key, contentType)` — archive + direct upload
- Default `Content-Type`: `application/x-mpegurl` for M3U, `application/gzip` for EPG
- Metadata: `uploaded-by: iptv-m3u-filter`, `upload-timestamp`, `source-file`, size info
- Nil-client guard: all functions return error if client is nil

### internal/utils/

- `DownloadFile(url, maxSize)` / `DownloadFileWithContext(ctx, url, maxSize, skipSSL)` — HTTP download with size limit
- `NewHTTPClient(skipSSL)` — configurable SSL verification
- `Retry(maxAttempts, delay, backoff, fn)` — exponential backoff (3 attempts, 2s, 2x)
- `ToLowerSlice(slice)` — lowercase all strings in slice
- `NormalizeLineEndings(content)` — `\r\n` → `\n`
- `IsGzipped()` / `DecompressGZip()` / `DecompressZip()` — compression detection/helpers
- `SanitizedWriter` — log wrapper masking: URLs → `https://****/****`, AWS/Yandex keys → `YCAJ****abcd` / `AKIA****xxxx`

## Filtering logic (internal/m3u/)

- **Category filter**: deny-list approach — `CategoriesToRemove` (26 exact matches) + `CategoriesToRemoveSubstring` (~100 substrings)
- **Group-title normalization**: leading numbers (`^\\d+.\\s*`) stripped, trailing emojis stripped
- **Channel exclusions**: by name substring (6 patterns), regional suffix (`+N`), numeric suffix (`2+` digits at end)
- **Name processing**: `orig` suffix removed
- **Emoji identifiers**: FNV-1a 64-bit hash → first emoji from URL hostname (DNS name, port ignored), second from URL path (query ignored); pools of 100+ emojis each (~10,000+ combinations), appended to channel name
- **Dedup by URL**: first non-empty attributes merged, longest name wins
- **Sort**: A-Z case-insensitive stable sort after dedup
- **Metadata overrides**: `categories.txt` can supply `group-title`/`tvg-id` via `CATEGORIES_FILE_PATH` env var

## EPG processing (internal/epg/)

- Downloads from `EPG_SOURCE_URL` (supports `.gz` and `.zip`)
- `EPG_RETENTION_DAYS` (default: 3) — discards programmes outside this window
- `EPGExcludedCategories` — categories excluded from EPG (default: `Кино`)
- `EPGExcludedChannelIDs` — 31 specific channel IDs
- Filtering uses **single-pass streaming** `xml.Decoder` (no full 463MB `xml.Unmarshal`)
- Output saved as `.gz` compressed

## S3 upload (internal/s3/)

Uploads: filtered playlist, all-categories playlist, EPG file, plus `.gz` archives.
- Client created once via `NewClient()`; reused for all uploads
- `UploadBoth` combines archive + direct upload in one call
- Retry wrapper: 3 attempts, 2s delay, 2x backoff

## Security features

- **Input Validation**: Validates URLs, bucket names, keys before processing
- **Size Limiting**: 100MB for M3U, 500MB for EPG
- **Log Sanitization**: Masks URLs (`https://****/****`) and credentials:
  - Yandex Cloud: `YCAJEu...` (key), `YCON...` (secret)
  - AWS: `AKIA...`, `ASIA...` (access keys)
- **Credential Handling**: Uses env vars for sensitive data, never logged
- **SSL**: Configurable via `SKIP_SSL_VERIFY` (default: `false` — secure)

## CI (GitHub Actions)

Two jobs in `.github/workflows/filter-m3u.yml`:

**test** (timeout 10 min):
- `go build`, `go vet`, `go test -v -count=1`

**filter-m3u** (timeout 30 min, depends on test):
- Builds binary: `go build -o iptv-filter`
- Runs `./iptv-filter`
- PRs get `DRY_RUN=true` set automatically
- Uploads `output/` artifacts (retention: 7 days)

Secrets: `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `M3U_SOURCE_URL`, `S3_BUCKET_NAME`, `S3_OBJECT_KEY`, `S3_ENDPOINT_URL`, `S3_REGION`, `S3_EPG_KEY`, `EPG_SOURCE_URL`, `LOCAL_EPG_PATH`

## Environment quirks

- `.env` file exists locally — `export $(grep -v '^#' .env | xargs)` to load
- `SKIP_SSL_VERIFY=true` for local dev (not set in CI)
- `SanitizedWriter` wraps `log.Logger` and masks credentials in log output
- `Retry` helper on all download/upload functions (3 retries, 2s delay, 2x backoff)
- File size limits: 100 MB for M3U, 500 MB for EPG
- Graceful shutdown on SIGINT/SIGTERM via `context.Context` cancellation
- EPG streaming parser: single-pass `xml.Decoder`, pre-computed retention window, no dead code
