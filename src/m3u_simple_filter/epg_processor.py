"""
EPG processing module.

This module handles downloading, parsing, and filtering EPG (Electronic Program Guide) XML files.
"""

import os
import re
import logging
import xml.etree.ElementTree as ET
from xml.dom import minidom
from typing import Set, List
from urllib.request import urlopen
from urllib.error import URLError
from io import BytesIO
import gzip
import zipfile

from .config import Config
from .utils import SanitizedLogger


# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(levelname)s - %(message)s'
)
logger = SanitizedLogger(logging.getLogger(__name__))


def copy_element_with_children(element):
    """
    Рекурсивно копирует XML элемент со всеми атрибутами и дочерними элементами.
    Для элементов 'desc' очищает содержимое, оставляя атрибуты.
    """
    new_element = ET.Element(element.tag)

    # Копируем атрибуты
    for attr, value in element.attrib.items():
        new_element.set(attr, value)

    # Для тега 'desc' оставляем атрибуты, но очищаем содержимое
    if element.tag.lower() == 'desc':
        new_element.text = ""  # Очищаем текстовое содержимое
    else:
        new_element.text = element.text

    new_element.tail = element.tail  # Сохраняем хвостовой текст

    # Рекурсивно копируем дочерние элементы
    for child in element:
        new_child = copy_element_with_children(child)
        new_element.append(new_child)

    return new_element


def download_epg(url: str, config=None) -> str:
    """
    Download EPG file from the provided URL with security checks

    Args:
        url (str): URL to download the EPG file from
        config: Configuration object with output directory setting

    Returns:
        str: Content of the EPG file as a string

    Raises:
        URLError: If there's an error downloading the file
        UnicodeDecodeError: If there's an error decoding the file
        ValueError: If the file exceeds the maximum allowed size
    """
    logger.info(f"Downloading EPG file from: {url}")

    try:
        response = urlopen(url)

        # Read content in chunks to prevent memory issues with large files
        content_chunks = []
        total_size = 0

        while True:
            chunk = response.read(8192)  # Read in 8KB chunks
            if not chunk:
                break

            total_size += len(chunk)

            # Security: Check if file size exceeds maximum allowed size
            if total_size > Config.MAX_EPG_FILE_SIZE:
                raise ValueError(f"EPG file exceeds maximum allowed size of {Config.MAX_EPG_FILE_SIZE} bytes")

            content_chunks.append(chunk)

        raw_content = b''.join(content_chunks)

        # Use config if provided, otherwise create a new instance
        if config is None:
            config = Config()

        # Create output directory if it doesn't exist
        output_dir = config.OUTPUT_DIR
        import os
        os.makedirs(output_dir, exist_ok=True)

        # Save the original downloaded content to a file in the output directory
        import tempfile
        from urllib.parse import urlparse
        parsed_url = urlparse(url)
        filename = os.path.basename(parsed_url.path)
        if not filename:
            filename = "downloaded_epg.xml"

        original_file_path = os.path.join(output_dir, f"original_{filename}")

        # Check if content is compressed (gzip or zip)
        if url.endswith('.gz') or is_gzipped(raw_content):
            logger.info("Detected gzipped EPG file, decompressing...")
            # Save the compressed file first
            with open(original_file_path, 'wb') as f:
                f.write(raw_content)

            # Get file size
            original_file_size = os.path.getsize(original_file_path)
            original_file_size_kb = original_file_size / 1024

            logger.info(f"Original compressed EPG file saved as: {original_file_path} (size: {original_file_size_kb:.2f} KB)")

            raw_content = gzip.decompress(raw_content)
        elif url.endswith('.zip'):
            logger.info("Detected zipped EPG file, extracting...")
            # Save the compressed file first
            with open(original_file_path, 'wb') as f:
                f.write(raw_content)

            # Get file size
            original_file_size = os.path.getsize(original_file_path)
            original_file_size_kb = original_file_size / 1024

            logger.info(f"Original zipped EPG file saved as: {original_file_path} (size: {original_file_size_kb:.2f} KB)")

            with zipfile.ZipFile(BytesIO(raw_content), 'r') as zip_file:
                # Get the first file in the zip archive
                file_list = zip_file.namelist()
                if file_list:
                    first_file = file_list[0]
                    raw_content = zip_file.read(first_file)
                else:
                    raise ValueError("ZIP archive is empty")
        else:
            # Save the plain content
            with open(original_file_path, 'wb') as f:
                f.write(raw_content)

            # Get file size
            original_file_size = os.path.getsize(original_file_path)
            original_file_size_kb = original_file_size / 1024

            logger.info(f"Original EPG file saved as: {original_file_path} (size: {original_file_size_kb:.2f} KB)")

        content = raw_content.decode('utf-8')
        content_size_kb = len(content) / 1024
        logger.info(f"EPG file downloaded successfully, size: {content_size_kb:.2f} KB")
        return content
    except URLError as e:
        logger.error(f"Error downloading EPG file: {e}")
        raise
    except UnicodeDecodeError as e:
        logger.error(f"Error decoding EPG file: {e}")
        raise
    except Exception as e:
        logger.error(f"Unexpected error downloading EPG file: {e}")
        raise


def is_gzipped(data: bytes) -> bool:
    """
    Check if the given data is gzipped by looking at the magic bytes.

    Args:
        data (bytes): Raw data to check

    Returns:
        bool: True if data appears to be gzipped, False otherwise
    """
    return len(data) >= 2 and data[0] == 0x1f and data[1] == 0x8b


def extract_channel_info_from_playlist(playlist_content: str) -> tuple:
    """
    Extract channel IDs and their categories from M3U playlist content.

    Args:
        playlist_content (str): M3U playlist content

    Returns:
        tuple: (Set of unique channel IDs, Dict mapping channel IDs to categories)
    """
    logger.info("Extracting channel IDs and categories from playlist")

    channel_ids = set()
    channel_categories = {}  # Maps channel ID to its category
    lines = playlist_content.split('\n')

    for line in lines:
        if line.strip().startswith('#EXTINF:'):
            # Look for tvg-id attribute in the EXTINF line
            tvg_id_match = re.search(r'tvg-id="([^"]*)"', line, re.IGNORECASE)

            # Look for group-title attribute in the EXTINF line (category)
            group_title_match = re.search(r'group-title="([^"]*)"', line, re.IGNORECASE)

            if tvg_id_match:
                tvg_id = tvg_id_match.group(1).strip()
                if tvg_id:  # Only add non-empty IDs
                    channel_ids.add(tvg_id)

                    # Store the category if available
                    if group_title_match:
                        category = group_title_match.group(1).strip()
                        channel_categories[tvg_id] = category

    logger.info(f"Found {len(channel_ids)} unique channel IDs in playlist")
    return channel_ids, channel_categories


def filter_epg_content(epg_content: str, channel_ids: Set[str], channel_categories: dict = None, excluded_categories: List[str] = None, excluded_channel_ids: List[str] = None) -> str:
    """
    Filter EPG content to keep only programs for specified channel IDs, excluding channels from specified categories and specific channel IDs.

    Args:
        epg_content (str): Original EPG XML content
        channel_ids (Set[str]): Set of channel IDs to keep in the EPG
        channel_categories (dict): Dictionary mapping channel IDs to their categories
        excluded_categories (List[str]): List of categories to exclude from EPG
        excluded_channel_ids (List[str]): List of specific channel IDs to exclude from EPG

    Returns:
        str: Filtered EPG XML content
    """
    logger.info(f"Filtering EPG content for {len(channel_ids)} initial channels")

    if not channel_ids:
        logger.warning("No channel IDs provided, returning empty EPG")
        return '<?xml version="1.0" encoding="UTF-8"?><tv></tv>'

    # Initialize excluded categories if not provided
    if excluded_categories is None:
        from .config import Config
        config_obj = Config()
        excluded_categories = config_obj.get_epg_excluded_categories()

    # Initialize excluded channel IDs if not provided
    if excluded_channel_ids is None:
        from .config import Config
        config_obj = Config()
        excluded_channel_ids = config_obj.get_epg_excluded_channel_ids()

    # Convert excluded categories to lowercase for comparison
    excluded_categories_lower = [cat.lower() for cat in excluded_categories] if excluded_categories else []

    # Convert excluded channel IDs to a set for faster lookup
    excluded_channel_ids_set = set(excluded_channel_ids) if excluded_channel_ids else set()

    try:
        # Pre-build sets for faster lookup
        channel_ids_set = set(channel_ids)

        # Parse the XML content
        root = ET.fromstring(epg_content)

        # Create a new root element for the filtered content
        filtered_root = ET.Element("tv")

        # Create sets to track which channels we've seen and need to keep
        channels_to_keep = set()

        # First pass: identify which channels have programs we need to keep
        for program_elem in root.findall('programme'):
            channel_ref = program_elem.get('channel', '')
            if channel_ref in channel_ids_set:
                # Check if this channel belongs to an excluded category
                should_exclude_by_category = False
                if channel_categories and excluded_categories_lower:
                    if channel_ref in channel_categories:
                        channel_category = channel_categories[channel_ref].lower()
                        if channel_category in excluded_categories_lower:
                            should_exclude_by_category = True

                # Check if this channel ID is in the excluded list
                should_exclude_by_id = channel_ref in excluded_channel_ids_set

                # Only add to channels_to_keep if not in excluded category AND not in excluded channel IDs
                if not should_exclude_by_category and not should_exclude_by_id:
                    channels_to_keep.add(channel_ref)

        # Log the actual number of channels after filtering by categories and specific IDs
        logger.info(f"EPG content filtering: {len(channels_to_keep)} channels after category and ID exclusions (from {len(channel_ids)} initial channels)")

        # Second pass: copy channels that we need
        for channel_elem in root.findall('channel'):
            channel_id = channel_elem.get('id', '')
            if channel_id in channels_to_keep:
                # Create a new channel element with only the first display-name
                new_channel_elem = ET.Element("channel")
                new_channel_elem.set("id", channel_id)

                # Find and add only the first display-name, skip the rest
                display_names = channel_elem.findall('display-name')
                if display_names:
                    first_display_name = display_names[0]
                    new_display_name = ET.Element("display-name")
                    new_display_name.text = first_display_name.text
                    new_display_name.set("lang", first_display_name.get("lang", "ru"))
                    new_channel_elem.append(new_display_name)

                # Add all other child elements except display-names and icons
                for child in channel_elem:
                    if child.tag == 'display-name':
                        # Skip additional display-names (we already added the first one)
                        continue
                    elif child.tag == 'icon':
                        # Skip icon elements to reduce file size
                        continue
                    else:
                        # Copy other elements (like url, etc.)
                        new_child = ET.Element(child.tag)
                        new_child.text = child.text
                        # Copy attributes
                        for attr, value in child.attrib.items():
                            new_child.set(attr, value)
                        new_channel_elem.append(new_child)

                filtered_root.append(new_channel_elem)

        # Third pass: copy programs for channels we're keeping, with time-based filtering
        from datetime import datetime, timedelta
        import re

        # Calculate time thresholds
        current_time = datetime.now()

        # Use retention days from config
        # Import here to allow mocking in tests
        from importlib import import_module
        config_module = import_module('.config', package=__name__.rsplit('.', 1)[0])
        Config = config_module.Config
        config_obj = Config()

        # Check if strict old programs filtering is enabled
        strict_old_programs_filter = config_obj.STRICT_EPG_OLD_PROGRAMS_FILTER

        # Calculate past retention threshold (how many days back to keep programs that have ended)
        past_retention_days = config_obj.EPG_PAST_RETENTION_DAYS
        retention_start_time = current_time - timedelta(days=past_retention_days)

        retention_days = config_obj.EPG_RETENTION_DAYS
        retention_period_later = current_time + timedelta(days=retention_days)

        for program_elem in root.findall('programme'):
            channel_ref = program_elem.get('channel', '')
            if channel_ref in channels_to_keep:
                # Extract start and stop times from the program
                start_attr = program_elem.get('start', '')
                stop_attr = program_elem.get('stop', '')

                # Parse the time strings (format: YYYYMMDDHHMMSS +ZZZZ)
                start_match = re.match(r'(\d{4})(\d{2})(\d{2})(\d{2})(\d{2})(\d{2})\s+(\S+)', start_attr)
                stop_match = re.match(r'(\d{4})(\d{2})(\d{2})(\d{2})(\d{2})(\d{2})\s+(\S+)', stop_attr)

                if start_match and stop_match:
                    # Parse start time
                    start_year, start_month, start_day = int(start_match.group(1)), int(start_match.group(2)), int(start_match.group(3))
                    start_hour, start_min, start_sec = int(start_match.group(4)), int(start_match.group(5)), int(start_match.group(6))

                    # Parse stop time
                    stop_year, stop_month, stop_day = int(stop_match.group(1)), int(stop_match.group(2)), int(stop_match.group(3))
                    stop_hour, stop_min, stop_sec = int(stop_match.group(4)), int(stop_match.group(5)), int(stop_match.group(6))

                    try:
                        start_datetime = datetime(start_year, start_month, start_day, start_hour, start_min, start_sec)
                        stop_datetime = datetime(stop_year, stop_month, stop_day, stop_hour, stop_min, stop_sec)

                        # Check if strict old programs filtering is enabled
                        # If the program is from a year that's significantly different from current year,
                        # treat it as too old and exclude it (this solves the original issue with old programs)
                        if strict_old_programs_filter:
                            year_difference = abs(stop_datetime.year - current_time.year)
                            if year_difference > 1:
                                # Skip this program as it's too old
                                continue

                        # Apply time-based filtering:
                        # If past retention days is greater than 0, apply time-based filtering
                        # Include programs that either:
                        # 1. Ended recently (within past retention days) and started before retention start time, OR
                        # 2. Haven't ended yet and start within the configured retention period
                        if past_retention_days > 0:
                            condition1 = (stop_datetime >= retention_start_time or start_datetime <= retention_period_later)
                            condition2 = (start_datetime >= retention_start_time or stop_datetime >= retention_start_time)
                            should_include = condition1 and condition2
                        else:
                            # If past retention days is 0, use the original logic (no time-based filtering for past programs)
                            should_include = stop_datetime >= current_time or start_datetime <= retention_period_later

                        if should_include:
                            # Create a new program element with only essential elements
                            new_program_elem = ET.Element("programme")
                            # Copy attributes
                            for attr, value in program_elem.attrib.items():
                                new_program_elem.set(attr, value)

                            # Copy child elements (keeping all elements including icons, descriptions, ratings, and categories)
                            for child in program_elem:
                                # Recursively copy the entire element with all sub-elements and attributes
                                new_child = copy_element_with_children(child)
                                new_program_elem.append(new_child)

                            filtered_root.append(new_program_elem)
                    except ValueError:
                        # If there's an error parsing the datetime, include the program anyway to avoid losing data
                        logger.warning(f"Could not parse datetime for program on channel {channel_ref}, including it anyway")
                        # Create a new program element with only essential elements
                        new_program_elem = ET.Element("programme")
                        # Copy attributes
                        for attr, value in program_elem.attrib.items():
                            new_program_elem.set(attr, value)

                        # Copy child elements (keeping all elements including icons, descriptions, ratings, and categories)
                        for child in program_elem:
                            # Recursively copy the entire element with all sub-elements and attributes
                            new_child = copy_element_with_children(child)
                            new_program_elem.append(new_child)

                        filtered_root.append(new_program_elem)
                else:
                    # If we can't parse the time format, include the program anyway to avoid losing data
                    logger.warning(f"Could not parse time format for program on channel {channel_ref}, including it anyway")
                    # Create a new program element with only essential elements
                    new_program_elem = ET.Element("programme")
                    # Copy attributes
                    for attr, value in program_elem.attrib.items():
                        new_program_elem.set(attr, value)

                    # Copy child elements (keeping all elements including icons, descriptions, ratings, and categories)
                    for child in program_elem:
                            # Recursively copy the entire element with all sub-elements and attributes
                        new_child = copy_element_with_children(child)
                        new_program_elem.append(new_child)

                    filtered_root.append(new_program_elem)

        # Convert back to string with proper formatting
        # Create a string buffer to write the prettified XML
        rough_string = ET.tostring(filtered_root, encoding='unicode')

        # Parse and prettify the XML
        reparsed = minidom.parseString(rough_string)
        filtered_xml_str = reparsed.toprettyxml(indent="  ", encoding=None)

        # Remove extra blank lines introduced by minidom
        lines = [line for line in filtered_xml_str.split('\n') if line.strip()]
        filtered_xml_str = '\n'.join(lines)

        logger.info("EPG filtering completed successfully")
        return filtered_xml_str

    except ET.ParseError as e:
        logger.error(f"Error parsing EPG XML: {e}")
        raise
    except Exception as e:
        logger.error(f"Unexpected error filtering EPG content: {e}")
        raise


def save_filtered_epg_locally(content: str, filename: str, config=None) -> None:
    """
    Save the EPG content to a local file in the output directory, compressing to gz if filename ends with .gz

    Args:
        content (str): EPG content
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

    if filename.endswith('.gz'):
        import gzip
        # Compress content and save to .gz file
        # Encode the string content to bytes before compressing
        content_bytes = content.encode('utf-8')
        original_size_kb = len(content_bytes) / 1024

        with gzip.open(filepath, 'wb') as f:
            f.write(content_bytes)

        # Get compressed file size
        compressed_size = os.path.getsize(filepath)
        compressed_size_kb = compressed_size / 1024

        logger.info(f"EPG saved locally as compressed file: {filepath} (compressed: {compressed_size_kb:.2f} KB, original: {original_size_kb:.2f} KB)")
    else:
        # Save as plain text
        with open(filepath, 'w', encoding='utf-8') as f:
            f.write(content)

        # Get file size
        file_size = os.path.getsize(filepath)
        file_size_kb = file_size / 1024

        logger.info(f"EPG saved locally as {filepath} (size: {file_size_kb:.2f} KB)")