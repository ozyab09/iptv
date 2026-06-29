# AGENTS.md

## Project

IPTV M3U playlist filter: downloads M3U from HTTP URLs, filters by category (deny-list), normalizes names (remove `orig`, exclude regional `+1`/`+4` variants), keeps all channel variants (adds `#1`/`#2` suffixes for duplicates), optionally processes EPG, uploads to S3-compatible storage (Yandex Cloud).

## Language & runtime

Rewritten from Python to Go. Runs via `go run ./cmd/iptv-filter/` — no binary build needed.

## Entry points

- `cmd/iptv-filter/main.go` — script entry point
- `go run ./cmd/iptv-filter/` — run directly (compiles to temp, executes)

## Commands

```bash
go test ./... -v -count=1           # run all tests
go run ./cmd/iptv-filter/           # run (requires .env or export vars)
DRY_RUN=true go run ./cmd/iptv-filter/  # dry-run (no S3 upload)
```

## Development conventions

- Go standard project layout (`cmd/`, `internal/`)
- AWS SDK v2 for S3 operations
- Standard library `net/http` for HTTP (with `InsecureSkipVerify: true` for local dev)
- `encoding/xml` for EPG XML parsing
- `compress/gzip` and `archive/zip` for compression
- Environment-based config via `internal/config/` (stateless, reads env vars)
- Sanitized logging via `internal/utils/logger.go` (masks URLs and credentials)

## Architecture

```
iptv/
├── cmd/iptv-filter/main.go        # Entry point (go run)
├── internal/
│   ├── config/config.go           # Configuration from env vars with validation
│   ├── m3u/processor.go           # M3U download, filtering, normalization
│   ├── epg/processor.go           # EPG download (gzip/zip), XML filtering
│   ├── s3/upload.go               # S3 upload via AWS SDK v2
│   └── utils/
│       ├── http.go                # Shared HTTP client and DownloadFile utility
│       ├── logger.go              # Sanitized logger
│       └── retry.go               # Retry with exponential backoff
├── .github/workflows/filter-m3u.yml
├── go.mod / go.sum
├── categories.txt                 # Channel metadata (group-title/tvg-id overrides)
└── README.md
```

## Module descriptions

### internal/config/config.go

Config struct reading env vars with validation. Includes:
- `CategoriesToRemove` — deny-list of categories (default: `["Взрослые"]`)
- `ChannelNamesToExclude` — channels to exclude by name substring
- `EPGExcludedCategories` / `EPGExcludedChannelIDs`
- `BuildCustomEPGURL()` — constructs public URL for EPG file in S3
- `Validate()` — validates all required env vars before processing

### internal/m3u/processor.go

M3U download, filtering, parsing, normalization. Key functions:
- `DownloadM3U()` — HTTP download with size check (100MB)
- `FilterContent()` — line-by-line filter: category deny-list, channel name exclusion, regional `+N` exclusion, number suffix exclusion, `orig` removal
- `RemoveOrigSuffix()` — strips trailing " orig"
- `NormalizeNameForComparison()` — strips HD/orig/SD/4K/UHD/FHD suffixes
- `ParseCategoriesFile()` — parses categories.txt for metadata overrides
- `ApplyChannelMetadata()` — applies group-title/tvg-id from categories file
- `AddTvgIDsToPlaylist()` — adds tvg-id from EPG name-to-id map
- `RemoveDuplicatesAndApplyHDPref()` — groups by normalized name, adds `#1`/`#2` suffixes

### internal/epg/processor.go

EPG download and XML filtering. Key functions:
- `DownloadEPG()` — downloads with gzip/zip decompression, 500MB limit
- `ExtractChannelInfoFromPlaylist()` — extracts tvg-ids and channel names from M3U
- `BuildEPGNameToIDMap()` — builds lowercase display-name → channel-id map from EPG XML
- `FilterEPGContent()` — filters EPG by channel IDs/names, excludes categories/IDs, time-based retention
- `SaveFilteredEPGLocally()` — saves with gzip compression

### internal/s3/upload.go

S3 upload via AWS SDK v2. Functions:
- `UploadToS3()` — string content → S3
- `UploadFileToS3()` — local file → S3 (with output dir fallback)
- `UploadArchiveToS3()` — gzip-compressed → archive/YYYY-MM-DD/HH-MM-SS-UUID_key.gz

### internal/utils/

- `DownloadFile()` — shared HTTP download with size limit enforcement
- `Retry()` — utility function with exponential backoff (3 attempts, 2s delay, 2x backoff)
- `SanitizedWriter` — log wrapper that masks URLs and AWS keys in output

## Filtering logic (internal/m3u/)

- **Category filter**: deny-list approach — `CategoriesToRemove` (default: just `["Взрослые"]`). Everything else is kept.
- **Channel exclusions**: `ChannelNamesToExclude` — matched case-insensitively as substring (default: `Fashion`, `СПАС`, `Три ангела`, `ЛДПР`, `UA`, `Sports`)
- **Regional exclusion**: channels ending with `+N` (e.g. `+1`, `+4 HD`, `+2 (Приволжье)`) are removed
- **Number suffix exclusion**: channels ending with `2+` digits (e.g. `HD 50`, `50`) are removed — all channels excluded uniformly, no exemptions
- **Name processing**: `orig` suffix removed from channel names
- **No deduplication**: all channel variants kept. When multiple channels share the same normalized name, suffixes `#1`, `#2` etc. are appended
- **Optional metadata**: `categories.txt` can supply `group-title`/`tvg-id` overrides via `CATEGORIES_FILE_PATH` env var (matched by lowercase name)
- **EPG-based tvg-id**: channels without `tvg-id` get one matched by name against EPG display-names

## EPG processing (internal/epg/)

- Downloads from `EPG_SOURCE_URL` (supports `.gz` and `.zip`)
- `EPG_RETENTION_DAYS` (default: 10) — discards programmes outside this window
- `EPGExcludedCategories` — categories excluded from EPG (default: `Кино`)
- `EPGExcludedChannelIDs` — specific channel IDs excluded from EPG (30+ IDs)
- Output saved as `.gz` compressed

## S3 upload (internal/s3/)

Uploads: filtered playlist, all-categories playlist, EPG file, plus `.gz` archives of the playlists.
- `UploadToS3` — string content → S3
- `UploadFileToS3` — local file → S3
- `UploadArchiveToS3` — gzip-compressed content → S3
- Uses `Retry` helper (3 attempts, exponential backoff)
- Default content type: `application/x-mpegurl`

## Security features

- **Input Validation**: Validates URLs and config before processing
- **Size Limiting**: 100MB for M3U, 500MB for EPG
- **Log Sanitization**: Masks URLs (`https://****/****`) and AWS keys (`YCAJ****abcd`)
- **Credential Handling**: Uses env vars for sensitive data
- **SSL**: InsecureSkipVerify enabled for local dev

## CI (GitHub Actions)

- **test job**: `go test ./... -v -count=1`
- **filter-m3u job**: depends on test; runs `go run ./cmd/iptv-filter/`
- PRs get `DRY_RUN=true` set automatically
- Secrets: `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `M3U_SOURCE_URL`, `S3_BUCKET_NAME`, `S3_OBJECT_KEY`, `S3_ENDPOINT_URL`, `S3_REGION`, `S3_EPG_KEY`, `EPG_SOURCE_URL`, `LOCAL_EPG_PATH`

## Environment quirks

- `.env` file exists locally — `export $(grep -v '^#' .env | xargs)` to load
- SSL cert verification disabled (`InsecureSkipVerify: true`) for URL downloads — intentional for local dev
- `SanitizedWriter` wraps `log.Logger` and masks credentials in log output
- `Retry` helper on download/upload functions (3 retries, 2s delay, 2x backoff)
- File size limits: 100 MB for M3U, 500 MB for EPG
