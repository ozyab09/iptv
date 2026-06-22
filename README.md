# IPTV M3U Playlist Filter

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go](https://img.shields.io/badge/Go-1.25-blue.svg)](https://go.dev/)

A robust and secure IPTV M3U playlist filtering application written in Go. Downloads M3U playlists from HTTP URLs, filters out channels based on predefined categories, processes channel names, optionally processes EPG, and uploads to S3-compatible storage (Yandex Cloud).

## Features

- **Secure Download**: Downloads M3U/EPG files with size validation (100MB / 500MB limits)
- **Category Filtering**: Deny-list approach — removes specified categories, keeps everything else
- **Channel Name Processing**: Removes `orig` suffix, excludes regional `+N` variants, excludes number suffixes
- **No Deduplication**: All channel variants kept with `#1`/`#2` suffixes for duplicates
- **EPG Processing**: Downloads (gzip/zip), filters by channel, time-based retention (configurable days)
- **S3 Upload**: Playlists, EPG, and gzip archives uploaded to S3-compatible storage
- **Dry-Run Mode**: Test without uploading (`DRY_RUN=true`)
- **Log Sanitization**: Sensitive data (URLs, AWS keys) masked in logs
- **CI/CD**: GitHub Actions with automated testing and deployment

## Architecture

```
iptv/
├── cmd/iptv-filter/main.go        # Entry point (go run ./cmd/iptv-filter/)
├── internal/
│   ├── config/config.go           # Configuration from env vars
│   ├── m3u/processor.go           # M3U download, filtering, normalization
│   ├── epg/processor.go           # EPG download (gzip/zip), XML filtering
│   ├── s3/upload.go               # S3 upload via AWS SDK v2
│   └── utils/
│       ├── logger.go              # Sanitized logger
│       └── retry.go               # Retry with exponential backoff
├── .github/workflows/filter-m3u.yml
├── go.mod / go.sum
└── README.md
```

## Prerequisites

- Go 1.25+
- S3-compatible storage account (Yandex Cloud, AWS, etc.)
- AWS credentials (for S3 access)

## Configuration

Set these environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `M3U_SOURCE_URL` | Source URL(s) for M3U playlist (comma-separated) | required |
| `S3_BUCKET_NAME` | S3 bucket name | required |
| `S3_OBJECT_KEY` | S3 object key for filtered playlist | `playlist.m3u` |
| `S3_ENDPOINT_URL` | S3 endpoint URL | required |
| `S3_REGION` | S3 region | `us-east-1` |
| `S3_EPG_KEY` | S3 object key for EPG file | required |
| `EPG_SOURCE_URL` | EPG XML source URL | required |
| `AWS_ACCESS_KEY_ID` | S3 access key | required |
| `AWS_SECRET_ACCESS_KEY` | S3 secret key | required |
| `DRY_RUN` | Skip S3 upload | (unset) |
| `OUTPUT_DIR` | Local output directory | `output` |
| `EPG_RETENTION_DAYS` | EPG retention window | `10` |
| `CATEGORIES_FILE_PATH` | Path to categories.txt | (optional) |

Load with:
```bash
export $(grep -v '^#' .env | xargs)
```

## Usage

```bash
# Run directly (no binary build needed)
go run ./cmd/iptv-filter/

# Dry-run (save locally, skip S3)
DRY_RUN=true go run ./cmd/iptv-filter/
```

## Testing

```bash
go test ./... -v -count=1
```

## CI/CD

GitHub Actions workflow in `.github/workflows/filter-m3u.yml`:
- **test**: Runs `go test ./... -v -count=1`
- **filter-m3u**: Runs `go run ./cmd/iptv-filter/` (dry-run for PRs)

Required secrets: `M3U_SOURCE_URL`, `S3_BUCKET_NAME`, `S3_OBJECT_KEY`, `S3_ENDPOINT_URL`, `S3_REGION`, `S3_EPG_KEY`, `EPG_SOURCE_URL`, `LOCAL_EPG_PATH`, `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`.

## License

MIT License — see [LICENSE](LICENSE).
