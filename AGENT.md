# Agent Configuration for M3U Playlist Filter

## Project Overview

This agent filters M3U playlists by keeping only specified categories, processes channel names, and uploads the filtered playlist to S3-compatible storage. It runs on a schedule in GitHub Actions and performs dry-run tests on pull requests.

## Capabilities

- Download M3U playlists from HTTP URLs
- Parse and extract categories from M3U files
- Keep only specified categories based on configuration (inverse filtering)
- Process channel names by removing 'orig' suffix and keeping only HD versions when both HD and non-HD versions exist
- Upload filtered playlists to S3-compatible storage
- Support dry-run mode for local testing
- Integration with GitHub Actions for scheduled execution

## Configuration

### Core Configuration (config.py)

The main configuration file `src/m3u_simple_filter/config.py` contains:

- `M3U_SOURCE_URL`: Source URL for the M3U playlist
- `S3_DEFAULT_BUCKET_NAME`: Default S3 bucket name
- `S3_FILTERED_PLAYLIST_KEY`: Default S3 object key for filtered playlist
- `S3_ALL_CATEGORIES_PLAYLIST_KEY`: Default S3 object key for all categories playlist
- `S3_COMPATIBLE_CONFIG`: S3-compatible storage endpoint and region
- `LOCAL_FILTERED_PLAYLIST_PATH`: Local output filename for filtered playlist (derived from S3_OBJECT_KEY)
- `LOCAL_ALL_CATEGORIES_PLAYLIST_PATH`: Local output filename for all categories playlist (derived from S3_OBJECT_KEY)
- `CATEGORIES_TO_KEEP`: Categories to keep (inverse filtering - only these categories are kept)

## Execution Modes

### Normal Mode
- Downloads M3U file
- Applies inverse filtering based on configuration (keeps only specified categories)
- Uploads filtered playlist to S3

### Dry-run Mode
- Set `DRY_RUN=true` environment variable
- Downloads and filters M3U file
- Saves filtered playlist locally with filenames derived from S3_OBJECT_KEY
- Skips S3 upload

## GitHub Actions Integration

The `.github/workflows/filter-m3u.yml` file configures:

- Scheduled execution for regular filtering and upload
- Pull request testing with dry-run mode
- Automatic dependency installation
- Error handling and retries
- Artifact upload for both filtered and all-categories playlists

## Environment Variables

Required for GitHub Actions:
- `AWS_ACCESS_KEY_ID`: S3-compatible storage access key
- `AWS_SECRET_ACCESS_KEY`: S3-compatible storage secret key
- `S3_BUCKET_NAME`: S3 bucket name
- `S3_OBJECT_KEY`: S3 object key

## File Structure

```
iptv/
├── src/
│   ├── run_filter.py              # Entry point script
│   └── m3u_simple_filter/         # Organized modules
│       ├── __init__.py
│       ├── config.py              # Configuration module with type hints
│       ├── m3u_processor.py       # M3U processing module with type hints
│       ├── s3_operations.py       # S3 operations module with type hints
│       └── main.py                # Main application module with type hints
├── .github/
│   └── workflows/
│       └── filter-m3u.yml         # GitHub Actions workflow
├── pyproject.toml                 # Project configuration and dependencies
├── .gitignore                     # Git ignore rules
├── test.sh                        # Local testing script
└── README.md                      # Documentation
```

## Inverse Filter Categories

The following categories are kept by default (all others are removed):
- Россия | Russia
- Общие
- Развлекательные
- Новостные
- Познавательные
- Детские
- Музыка
- Региональные
- Европа | Europe
- Австралия | Australia
- Беларусь | Беларускія
- Великобритания | United Kingdom
- Канада | Canada
- США | USA
- Германия | Germany
- Индия | India
- Казахстан | Қазақстан
- Кино
- Спорт

These can be modified in the `CATEGORIES_TO_KEEP` list in `m3u_simple_filter/config.py`.

## Modern Python Features

The codebase uses modern Python practices:
- Type hints for improved code clarity and maintainability
- Proper module organization with clear separation of concerns
- Dynamic configuration using environment variables
- Comprehensive logging with line and channel counts

## Logging Information

The script outputs detailed logging information including:
- Filtering results: `Filtering complete: XXXX lines -> YYYY lines (AAAA channels -> BBBB channels)`
- This shows both the number of lines and the actual number of channels before and after filtering

## Local Testing

The project supports local testing:

### Usage
```bash
# Test with dry-run mode (saves locally, no S3 upload)
DRY_RUN=true python src/run_filter.py

# Test with normal mode (requires valid S3 credentials)
python src/run_filter.py
```

## Troubleshooting

- In dry-run mode, the script will save the filtered playlist locally without uploading to S3
- Check logs for detailed information about the filtering process
- The local filenames are derived from the S3_OBJECT_KEY environment variable
- Only one "Filtering complete" message is shown with both line and channel counts