#!/usr/bin/env python3
"""
Main application module for Simple M3U Playlist Filter.

This script downloads an M3U playlist, filters out specific categories,
removes 'orig' suffixes from channel names, keeps only HD versions when both
HD and non-HD versions exist, and uploads the filtered playlist to S3-compatible storage.
"""

import os
import sys
import logging
from typing import NoReturn

from .config import Config
from .m3u_processor import download_m3u, filter_m3u_content
from .epg_processor import download_epg, extract_channel_info_from_playlist, filter_epg_content, save_filtered_epg_locally
from .s3_operations import upload_to_s3, upload_file_to_s3
from .utils import SanitizedLogger


# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(levelname)s - %(message)s'
)
logger = SanitizedLogger(logging.getLogger(__name__))


def save_filtered_m3u_locally(content: str, filename: str, config=None) -> None:
    """
    Save the M3U content to a local file in the output directory

    Args:
        content (str): M3U content
        filename (str): Filename to save to
        config: Configuration object with output directory setting
    """
    import os
    from .config import Config

    # Use config if provided, otherwise create a new instance
    if config is None:
        config = Config()

    # Create output directory if it doesn't exist
    output_dir = config.OUTPUT_DIR
    os.makedirs(output_dir, exist_ok=True)

    # Create full file path
    filepath = os.path.join(output_dir, filename)

    with open(filepath, 'w', encoding='utf-8') as f:
        f.write(content)

    # Get file size
    file_size = os.path.getsize(filepath)
    file_size_kb = file_size / 1024

    logger.info(f"M3U saved locally as {filepath} (size: {file_size_kb:.2f} KB)")


def main() -> int:
    """
    Main function to orchestrate the M3U filtering and upload process.
    """
    # Use configuration from settings
    config = Config()

    # Validate configuration
    validation_errors = config.validate_config()
    if validation_errors:
        for error in validation_errors:
            logger.error(f"Configuration error: {error}")
        return 1

    m3u_url = config.M3U_SOURCE_URL
    epg_url = config.EPG_SOURCE_URL
    s3_bucket = config.S3_DEFAULT_BUCKET_NAME
    s3_filtered_key = config.S3_FILTERED_PLAYLIST_KEY
    s3_all_categories_key = config.S3_ALL_CATEGORIES_PLAYLIST_KEY
    s3_epg_key = config.S3_EPG_KEY
    categories_to_keep = config.get_categories_to_keep()

    # Check for dry-run mode
    dry_run = os.environ.get('DRY_RUN', '').lower() in ('true', '1', 'yes', 'on')

    try:
        # Step 1: Download M3U file
        logger.info("Starting M3U filtering process")
        m3u_content = download_m3u(m3u_url)

        # Step 2: Filter content to keep only selected categories
        channel_names_to_exclude = config.get_channel_names_to_exclude()

        # Construct the custom EPG URL based on S3 configuration
        # Extract the host part from the endpoint URL properly
        endpoint_url = config.S3_ENDPOINT_URL
        if '://' in endpoint_url:
            host_part = endpoint_url.split('://', 1)[1]  # Take everything after the protocol
        else:
            host_part = endpoint_url  # Assume it's just the host if no protocol

        custom_epg_url = f"https://{config.S3_DEFAULT_BUCKET_NAME}.{host_part}/{config.S3_EPG_KEY}"
        filtered_content = filter_m3u_content(m3u_content, categories_to_keep, channel_names_to_exclude, custom_epg_url)

        # Save both files locally in all modes for artifact availability
        save_filtered_m3u_locally(filtered_content, config.LOCAL_FILTERED_PLAYLIST_PATH, config)
        save_filtered_m3u_locally(m3u_content, config.LOCAL_ALL_CATEGORIES_PLAYLIST_PATH, config)

        # Step 3: Process EPG if EPG_URL is provided
        if epg_url:
            logger.info("Starting EPG filtering process")

            # Download original EPG
            epg_content = download_epg(epg_url, config)

            # Extract channel IDs and categories from the filtered M3U playlist
            channel_ids, channel_categories = extract_channel_info_from_playlist(filtered_content)

            # Get excluded categories from config
            excluded_categories = config.get_epg_excluded_categories()

            # Filter EPG content to only include programs for channels in the filtered playlist, excluding specified categories
            filtered_epg_content = filter_epg_content(epg_content, channel_ids, channel_categories, excluded_categories)

            # Save filtered EPG locally
            save_filtered_epg_locally(filtered_epg_content, config.LOCAL_FILTERED_EPG_PATH, config)

            if not dry_run:
                # Upload the compressed EPG file to S3
                upload_file_to_s3(config.LOCAL_FILTERED_EPG_PATH, s3_bucket, s3_epg_key, config, content_type='application/gzip')

        if dry_run:
            logger.info("Dry-run mode: Files saved locally, skipping S3 upload")
            return 0
        else:
            # Validate required environment variables for S3 upload
            if not os.environ.get('AWS_ACCESS_KEY_ID') or not os.environ.get('AWS_SECRET_ACCESS_KEY'):
                logger.warning("AWS credentials not found in environment variables. Make sure they are set.")

            if s3_bucket == 'your-bucket-name':
                logger.error("S3_BUCKET_NAME environment variable not set. Please configure it.")
                return 1

            # Upload both files to S3 in normal mode
            upload_to_s3(filtered_content, s3_bucket, s3_filtered_key, config)
            upload_to_s3(m3u_content, s3_bucket, s3_all_categories_key, config)

            logger.info("Process completed successfully")
            return 0

    except Exception as e:
        logger.error(f"Process failed: {e}")
        return 1


if __name__ == "__main__":
    sys.exit(main())