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

The following 70 categories across 3 M3U sources are kept by default (all others are removed, excluding `🔺 INFO 🔺`):

**Source 1: Provider/ISP (45 categories):** `↕️ Торрент ТВ ↕️`, `Современные сетевые технологии (VPN)`, `С сайтов (VPN)`, `Большое ТВ (VPN)`, `С сайтов`, `Датагруп 🇺🇦`, `Скай Телеком`, `Музыкальные 🎶`, `Узбектелеком 🇺🇿`, `Сайт (VPN)`, `АСАРТА (VPN)`, `ТаймВэб (VPN)`, `Виктория (VPN)`, `Оргтехсервис (VPN)`, `Телевизор 24 (VPN)`, `📺 Usba TV`, `Квант-Телеком (VPN 🇷🇺)`, `Цитадель-Крым (VPN)`, `ОБИТ`, `Сириус`, `Казахтелеком`, `4K VIDEO (VPN)`, `AgroNet (VPN)`, `Catcast TV 🐈 Not 24/7`, `CloudFlare Inc (VPN 🇷🇺)`, `Cloudflare_Inc!`, `Cloudflare_Inc`, `Hetzner Online GmbH`, `Interhost`, `Internet42 LLC`, `Itv.uz (🇺🇿)`, `IZONE`, `KazTransCom`, `Lime (VPN 🇷🇺)`, `Peers (VPN)`, `RELAX`, `Rutube (VPN)`, `StarNet (VPN 🇳🇱)`, `TEST ⓵`, `TEST (VPN)`, `Tricolor (VPN 🇷🇺)`, `Turon Media`, `Voka`, `Webhost (VPN 🇷🇺)`, `Wink (VPN 🇷🇺)`

**Source 2: International (20 categories):** `Объединенные Арабские Эмираты 🇦🇪`, `Новая Зеландия 🇳🇿`, `Саудовская Аравия 🇸🇦`, `Северная Македония 🇲🇰`, `Доминиканская Республика 🇩🇴`, `Грузия 🇬🇪 (GEO)`, `Таджикистан 🇹🇯 (GEO)`, `США 🇺🇸`, `Ирак 🇮🇶`, `Алжир 🇩🇿`, `Греция 🇬🇷`, `Австрия 🇦🇹`, `Беларусь 🇧🇾`, `Австралия 🇦🇺`, `Афганистан 🇦🇫`, `Азербайджан 🇦🇿`, `Туркменистан 🇹🇲`, `Великобритания 🇬🇧`, `Шри-Ланка 🇱🇰`, `Коста-Рика 🇨🇷`

**Source 3: Content (5 categories):** `Кино`, `Общие`, `Знания`, `Новости`, `Развлечение`

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

## Workflow: Closing Issues with Code Fixes

When the user says **"закрывай проблему"** (close the issue), follow this workflow:

### 1. Create GitHub Issue
```bash
gh issue create --title "<descriptive title>" --body "<detailed description>"
```

The issue should include:
- **Problem**: Clear description of what went wrong
- **Root Cause**: Technical explanation of why it happened
- **Impact**: What was affected

### 2. Create Branch and Commit Changes
```bash
git checkout -b <type-feature>
git add <modified files>
git commit -m "<type>: <descriptive message>

<blank line>

<Detailed explanation of what was changed and why>
<Reference to the issue being fixed>"
```

Commit message guidelines:
- Use conventional commit prefixes: `fix:`, `feat:`, `docs:`, `refactor:`, `test:`, `chore:`
- First line: concise summary (50 chars max)
- Body: explain **why** not just **what**

### 3. Push Branch and Create MR
```bash
git push origin <branch-name>
gh pr create --title "<title>" --body "<description>" --base main
```

The PR body should include:
- **Changes**: Bullet list of what was modified
- **Problem Fixed**: Explanation of the bug
- **Result**: What the fix achieves
- Link to the issue with `Closes #<number>`

### Example
```bash
gh issue create --title "Channel X excluded by filter Y" --body "## Problem..."
git add src/module.py tests/test_module.py
git commit -m "fix: preserve Channel X variants from filter Y

- Exempt CHANNELS_KEEP_ALL_VARIANTS from number filter
- Add channel variant to config
- Update tests

Fixes issue where Channel X was excluded..."
git push origin fix-channel-x
gh pr create --title "fix: preserve Channel X variants" --body "## Changes...

Closes #49" --base main
```