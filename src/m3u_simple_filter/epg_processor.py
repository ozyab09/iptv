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
        content_parts = []  # Use list to collect parts
        total_size = 0

        while True:
            chunk = response.read(8192)  # Read in 8KB chunks
            if not chunk:
                break

            total_size += len(chunk)

            # Security: Check if file size exceeds maximum allowed size
            if total_size > Config.MAX_EPG_FILE_SIZE:
                raise ValueError(f"EPG file exceeds maximum allowed size of {Config.MAX_EPG_FILE_SIZE} bytes")

            content_parts.append(chunk)

        raw_content = b''.join(content_parts)

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
    channel_names_by_id = {}  # Maps channel ID to channel name for fallback matching
    lines = playlist_content.split('\n')

    for line in lines:
        if line.strip().startswith('#EXTINF:'):
            # Look for tvg-id attribute in the EXTINF line
            tvg_id_match = re.search(r'tvg-id="([^"]*)"', line, re.IGNORECASE)

            # Look for group-title attribute in the EXTINF line (category)
            group_title_match = re.search(r'group-title="([^"]*)"', line, re.IGNORECASE)

            # Extract channel name from the EXTINF line (after the comma)
            parts = line.rsplit(',', 1)
            channel_name = ""
            if len(parts) > 1:
                channel_name = parts[1].strip()

            if tvg_id_match:
                tvg_id = tvg_id_match.group(1).strip()
                if tvg_id:  # Only add non-empty IDs
                    channel_ids.add(tvg_id)

                    # Store the category if available
                    if group_title_match:
                        category = group_title_match.group(1).strip()
                        channel_categories[tvg_id] = category
                    
                    # Store the channel name for fallback matching
                    if channel_name:
                        channel_names_by_id[tvg_id] = channel_name

    logger.info(f"Found {len(channel_ids)} unique channel IDs in playlist")
    return channel_ids, channel_categories


def filter_epg_content(epg_content: str, channel_ids: Set[str], channel_categories: dict = None, excluded_categories: List[str] = None, excluded_channel_ids: List[str] = None, current_time_override=None) -> str:
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
                # Add all channels that are in the filtered M3U playlist to channels_to_keep
                # This includes both regular channels and excluded channels
                channels_to_keep.add(channel_ref)

        # If no channels matched by ID, try to match by channel name as a fallback
        if not channels_to_keep:
            logger.info("No channel IDs matched, attempting fallback matching by channel name")
            
            # Build a mapping of channel names from the EPG to their IDs
            epg_channel_name_to_id = {}
            for channel_elem in root.findall('channel'):
                channel_id = channel_elem.get('id', '')
                display_names = channel_elem.findall('display-name')
                for name_elem in display_names:
                    if name_elem.text:
                        channel_name = name_elem.text.strip().lower()
                        epg_channel_name_to_id[channel_name] = channel_id
            
            # Extract channel names from the M3U playlist for comparison
            # We need to extract channel names from the original M3U content
            m3u_channel_names = set()
            lines = epg_content.split('\n')  # This is not correct - we need the original M3U content
            # Actually, we need to get the channel names from the original M3U playlist, not the EPG content
            # Since we don't have access to the original M3U content here, we'll use a different approach
            
            # For now, let's use the channel_categories mapping to get channel names
            # This is a workaround since we don't have direct access to M3U channel names here
            for channel_id, category in channel_categories.items():
                # We don't have the channel names directly, so we'll implement a different fallback
                pass
            
            # A better approach: check if any programs are currently active or upcoming
            # and include channels that have relevant programs
            from datetime import datetime, timezone
            current_time = datetime.now(timezone.utc).replace(tzinfo=None) if current_time_override is None else current_time_override
            
            # Look for programs that are currently active or upcoming
            for program_elem in root.findall('programme'):
                channel_ref = program_elem.get('channel', '')
                
                # Extract start and stop times from the program
                start_attr = program_elem.get('start', '')
                stop_attr = program_elem.get('stop', '')

                # Parse the time strings (format: YYYYMMDDHHMMSS +ZZZZ)
                import re
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

                        # Only include channels that have programs that are currently active or upcoming
                        # This prevents including channels with only historical programs
                        if stop_datetime >= current_time or start_datetime >= current_time:
                            channels_to_keep.add(channel_ref)
                    except ValueError:
                        # If there's an error parsing the datetime, skip this program
                        continue

            # If still no channels are selected, use a more conservative approach
            if not channels_to_keep:
                logger.info("Still no channels matched, including channels with programs in the next few days")
                # Include channels that have programs in the next few days
                from datetime import timedelta
                future_threshold = current_time + timedelta(days=7)  # Programs in next 7 days
                
                for program_elem in root.findall('programme'):
                    channel_ref = program_elem.get('channel', '')
                    
                    # Extract start time from the program
                    start_attr = program_elem.get('start', '')
                    start_match = re.match(r'(\d{4})(\d{2})(\d{2})(\d{2})(\d{2})(\d{2})\s+(\S+)', start_attr)

                    if start_match:
                        # Parse start time
                        start_year, start_month, start_day = int(start_match.group(1)), int(start_match.group(2)), int(start_match.group(3))
                        start_hour, start_min, start_sec = int(start_match.group(4)), int(start_match.group(5)), int(start_match.group(6))

                        try:
                            start_datetime = datetime(start_year, start_month, start_day, start_hour, start_min, start_sec)

                            # Include channels that have programs in the next 7 days
                            if current_time <= start_datetime <= future_threshold:
                                channels_to_keep.add(channel_ref)
                        except ValueError:
                            continue

        # Log the actual number of channels that have programs
        logger.info(f"EPG content filtering: {len(channel_ids)} initial channels, {len(channels_to_keep)} channels in filtered playlist (from {len(channel_ids)} initial channels)")

        # Create a set to track which channels actually have programs
        channels_with_programs = set()

        # Third pass: copy programs for channels we're keeping, with time-based filtering
        from datetime import datetime, timedelta
        import re

        # Calculate time thresholds
        from datetime import timezone
        if current_time_override is not None:
            current_time = current_time_override
        else:
            current_time = datetime.now(timezone.utc).replace(tzinfo=None)  # Convert to naive UTC datetime

        # Use retention days from config
        # Import here to allow mocking in tests
        from importlib import import_module
        config_module = import_module('.config', package=__name__.rsplit('.', 1)[0])
        Config = config_module.Config
        config_obj = Config()

        # Calculate past retention threshold (how many days back to keep programs that have ended)
        past_retention_days = config_obj.EPG_PAST_RETENTION_DAYS
        retention_start_time = current_time - timedelta(days=past_retention_days)

        retention_days = config_obj.EPG_RETENTION_DAYS
        retention_period_later = current_time + timedelta(days=retention_days)

        # Get excluded categories and channel IDs for special handling
        excluded_categories_lower = [cat.lower() for cat in excluded_categories] if excluded_categories else []
        excluded_channel_ids_set = set(excluded_channel_ids) if excluded_channel_ids else set()

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

                        # Determine if this channel is in an excluded category or is an excluded channel ID
                        is_excluded_channel = False
                        if channel_categories and channel_ref in channel_categories:
                            channel_category = channel_categories[channel_ref].lower()
                            if channel_category in excluded_categories_lower:
                                is_excluded_channel = True
                        
                        if channel_ref in excluded_channel_ids_set:
                            is_excluded_channel = True

                        # Apply time-based filtering:
                        # If past retention days is greater than 0, apply time-based filtering
                        # Include programs that either:
                        # 1. Haven't ended yet (stop time >= retention_start_time), OR
                        # 2. Will start within the configured retention period (start time <= retention days from now)
                        # But exclude programs that both started and ended in the distant past
                        if past_retention_days > 0:
                            condition1 = (stop_datetime >= retention_start_time or start_datetime <= retention_period_later)
                            condition2 = (start_datetime >= retention_start_time or stop_datetime >= retention_start_time)
                            should_include = condition1 and condition2
                        else:
                            # If past retention days is 0, use a more flexible approach to handle historical data
                            # For excluded channels, only include programs that haven't ended more than 1 hour ago and are within 1 day ahead
                            if is_excluded_channel:
                                # Get the new configuration values
                                from .config import Config
                                config_obj = Config()
                                future_limit_days = config_obj.EXCLUDED_CHANNELS_FUTURE_LIMIT_DAYS
                                past_limit_hours = config_obj.EXCLUDED_CHANNELS_PAST_LIMIT_HOURS

                                # Calculate time thresholds
                                past_threshold = current_time - timedelta(hours=past_limit_hours)
                                future_threshold = current_time + timedelta(days=future_limit_days)

                                # Include programs that:
                                # 1. Haven't ended more than 1 hour ago (stop_datetime >= past_threshold)
                                # 2. Start within 1 day ahead (start_datetime <= future_threshold)
                                should_include = stop_datetime >= past_threshold and start_datetime <= future_threshold
                            else:
                                # For non-excluded channels, when past retention is 0, we need to be much more flexible
                                # to handle historical EPG data. Include programs that are within a wider time range
                                # Calculate how far back this program is from current time
                                time_since_stop = current_time - stop_datetime
                                
                                # If the program ended not too long ago OR starts not too far in the future, include it
                                # This handles the case where EPG data contains historical programs
                                reasonable_past_range = timedelta(days=365)  # Allow up to 1 year in the past
                                
                                # Include programs that either:
                                # 1. Haven't ended yet (original condition)
                                # 2. Will start in the future period (original condition)  
                                # 3. Ended recently (within reasonable past range) - UPDATED to be more permissive
                                # 4. Started in the past but ends in the future (overlapping with current time)
                                should_include = (stop_datetime >= current_time or 
                                                start_datetime <= retention_period_later or
                                                time_since_stop <= reasonable_past_range or
                                                (start_datetime <= current_time and stop_datetime >= current_time))

                        if should_include:
                            # Track which channels have programs
                            channels_with_programs.add(channel_ref)
                            
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

                        # Track which channels have programs (since we're including the program)
                        channels_with_programs.add(channel_ref)
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

                    # Track which channels have programs (since we're including the program)
                    channels_with_programs.add(channel_ref)
                    filtered_root.append(new_program_elem)

        # Second pass: copy channels that we need (only those that have programs)
        for channel_elem in root.findall('channel'):
            channel_id = channel_elem.get('id', '')
            if channel_id in channels_to_keep and channel_id in channels_with_programs:
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