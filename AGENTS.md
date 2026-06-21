# AGENTS.md

## Project

IPTV M3U playlist filter: downloads M3U from HTTP URLs, filters by category (deny-list), normalizes names (remove `orig`, exclude regional `+1`/`+4` variants), keeps all channel variants (adds `#1`/`#2` suffixes for duplicates), optionally processes EPG, uploads to S3-compatible storage (Yandex Cloud).

## Entry points

- `src/run_filter.py` — script entry point
- `src/m3u_simple_filter/main.py` — orchestration
- `pip install -e . && iptv-filter` — console_scripts entry

## Commands

```bash
pip install -e ".[dev]"          # install with test deps
python -m pytest tests/ -v       # run tests
python -m coverage run -m pytest tests/ && python -m coverage report
cd src && python run_filter.py   # run (requires .env or export vars)
DRY_RUN=true python src/run_filter.py  # dry-run (no S3 upload)
```

## Key architecture

- `src/m3u_simple_filter/` — package with `config.py`, `m3u_processor.py`, `epg_processor.py`, `s3_operations.py`, `utils.py`, `main.py`
- Config via `Config` class in `config.py` — reads env vars, validates before processing
- `DRY_RUN=true` env var skips S3 upload (files still saved locally in `output/`)
- Multiple M3U URLs can be comma-separated in `M3U_SOURCE_URL`

## Filtering logic (m3u_processor.py)

- **Category filter**: deny-list approach — `CATEGORIES_TO_REMOVE` (default: just `["Взрослые"]`). Everything else is kept.
- **Channel exclusions**: `CHANNEL_NAMES_TO_EXCLUDE` — matched case-insensitively as substring
- **Regional exclusion**: channels ending with `+N` (e.g. `+1`, `+4 HD`, `+2 (Приволжье)`) are removed
- **Number suffix exclusion**: channels ending with `2+` digits (e.g. `HD 50`, `50`) removed, unless in `CHANNELS_KEEP_ALL_VARIANTS`
- **Name processing**: `orig` suffix removed from channel names
- **No deduplication**: all channel variants kept. When multiple channels share the same normalized name, suffixes `#1`, `#2` etc. are appended
- `CHANNELS_KEEP_ALL_VARIANTS` config still exempts channels from number-suffix exclusion
- **Optional metadata**: `categories.txt` can supply `group-title`/`tvg-id` overrides via `CATEGORIES_FILE_PATH` env var (matched by lowercase name)
- **EPG-based tvg-id**: channels without `tvg-id` get one matched by name against EPG display-names

## EPG processing (epg_processor.py)

- Downloads from `EPG_SOURCE_URL` (supports `.gz` and `.zip`)
- `EPG_RETENTION_DAYS` (default: 10) — discards programmes outside this window
- `EPG_EXCLUDED_CATEGORIES` / `EPG_EXCLUDED_CHANNEL_IDS` — categories and specific channel IDs excluded from EPG
- Output saved as `.gz` compressed

## S3 upload (s3_operations.py)

Uploads: filtered playlist, all-categories playlist, EPG file, plus `.gz` archives of the playlists.
- `upload_to_s3` — string content → S3
- `upload_file_to_s3` — local file → S3
- `upload_archive_to_s3` — gzip-compressed content → S3
- Uses `retry` decorator (3 attempts, exponential backoff)

## CI (GitHub Actions)

- **test job**: `pytest` + `coverage --fail-under=70`
- **filter-m3u job**: depends on test; runs `cd src && python run_filter.py`
- PRs get `DRY_RUN=true` set automatically
- Secrets: `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `M3U_SOURCE_URL`, `S3_BUCKET_NAME`, `S3_OBJECT_KEY`, `S3_ENDPOINT_URL`, `S3_REGION`, `S3_EPG_KEY`, `EPG_SOURCE_URL`, `LOCAL_EPG_PATH`

## Environment quirks

- `.env` file exists locally — `export $(grep -v '^#' .env | xargs)` to load
- SSL cert verification disabled (`CERT_NONE`) for URL downloads — intentional for local dev
- `SanitizedLogger` wraps `logging.Logger` and masks credentials in log output
- `retry` decorator on `download_m3u`, `upload_to_s3`, `upload_file_to_s3` (3 retries, 2s delay, 2x backoff)
- File size limits: 100 MB for M3U, 500 MB for EPG

## Test quirks

- `conftest.py` adds both project root and `src/` to `sys.path`
- `pyproject.toml` sets `pythonpath = ["src"]` for pytest
- Tests use `unittest.TestCase` style with `unittest.mock`
- No formatter/linter config in the repo — code follows PEP 8 informally

## Other instruction files

This repo also has `CLAUDE.md` (for Claude Code) and `QWEN.md` — they share similar content but have stale claims (e.g. describe `CATEGORIES_TO_KEEP` with 70 categories, but actual code uses `CATEGORIES_TO_REMOVE` deny-list). Treat `AGENTS.md` and source code as canonical.
