# AGENTS.md

## Project

IPTV M3U playlist filter: downloads M3U from HTTP URLs, filters by category (deny-list), normalizes names (remove `orig`, exclude regional `+1`/`+4` variants), keeps all channel variants (adds `#1`/`#2` suffixes for duplicates), optionally processes EPG, uploads to S3-compatible storage (Yandex Cloud).

## Language & runtime

Rewritten from Python to Go. Runs via `go run ./cmd/iptv-filter/` — no binary build needed.

## Entry points

- `cmd/iptv-filter/main.go` — script entry point
- `go run ./cmd/iptv-filter/` — run directly (compiles to temp, executes)
- `go run ./cmd/iptv-filter/` — console equivalent of `iptv-filter`

## Commands

```bash
go test ./... -v -count=1           # run all tests
go run ./cmd/iptv-filter/           # run (requires .env or export vars)
DRY_RUN=true go run ./cmd/iptv-filter/  # dry-run (no S3 upload)
```

## Key architecture

- `internal/config/` — `Config` struct reading env vars, validation
- `internal/m3u/` — M3U download, filtering, normalization, deduplication
- `internal/epg/` — EPG download (gzip/zip), XML filtering, time-based retention
- `internal/s3/` — S3 upload (content, file, archive) via AWS SDK v2
- `internal/utils/` — Retry helper, sanitized logger
- `cmd/iptv-filter/main.go` — orchestration

Config via `Config` struct in `internal/config/` — reads env vars, validates before processing.

`DRY_RUN=true` env var skips S3 upload (files still saved locally in `output/`).

Multiple M3U URLs can be comma-separated in `M3U_SOURCE_URL`.

## Filtering logic (internal/m3u/)

- **Category filter**: deny-list approach — `CategoriesToRemove` (default: just `["Взрослые"]`). Everything else is kept.
- **Channel exclusions**: `ChannelNamesToExclude` — matched case-insensitively as substring
- **Regional exclusion**: channels ending with `+N` (e.g. `+1`, `+4 HD`, `+2 (Приволжье)`) are removed
- **Number suffix exclusion**: channels ending with `2+` digits (e.g. `HD 50`, `50`) are removed
- **Name processing**: `orig` suffix removed from channel names
- **No deduplication**: all channel variants kept. When multiple channels share the same normalized name, suffixes `#1`, `#2` etc. are appended
- All channels with numeric suffixes are excluded uniformly — no exemptions
- **Optional metadata**: `categories.txt` can supply `group-title`/`tvg-id` overrides via `CATEGORIES_FILE_PATH` env var (matched by lowercase name)
- **EPG-based tvg-id**: channels without `tvg-id` get one matched by name against EPG display-names

## EPG processing (internal/epg/)

- Downloads from `EPG_SOURCE_URL` (supports `.gz` and `.zip`)
- `EPG_RETENTION_DAYS` (default: 10) — discards programmes outside this window
- `EPGExcludedCategories` / `EPGExcludedChannelIDs` — categories and specific channel IDs excluded from EPG
- Output saved as `.gz` compressed

## S3 upload (internal/s3/)

Uploads: filtered playlist, all-categories playlist, EPG file, plus `.gz` archives of the playlists.
- `UploadToS3` — string content → S3
- `UploadFileToS3` — local file → S3
- `UploadArchiveToS3` — gzip-compressed content → S3
- Uses `Retry` helper (3 attempts, exponential backoff)

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
