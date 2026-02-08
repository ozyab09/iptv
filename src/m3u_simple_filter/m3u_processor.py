"""
M3U processing module.

This module handles downloading, parsing, and filtering M3U playlist files.
"""

import os
import re
import logging
from typing import List, Tuple
from urllib.request import urlopen
from urllib.error import URLError

from .config import Config
from .utils import SanitizedLogger


# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(levelname)s - %(message)s'
)
logger = SanitizedLogger(logging.getLogger(__name__))


def download_m3u(url: str) -> str:
    """
    Download M3U file from the provided URL with security checks

    Args:
        url (str): URL to download the M3U file from

    Returns:
        str: Content of the M3U file as a string

    Raises:
        URLError: If there's an error downloading the file
        UnicodeDecodeError: If there's an error decoding the file
        ValueError: If the file exceeds the maximum allowed size
    """
    logger.info(f"Downloading M3U file from: {url}")

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
            if total_size > Config.MAX_M3U_FILE_SIZE:
                raise ValueError(f"M3U file exceeds maximum allowed size of {Config.MAX_M3U_FILE_SIZE} bytes")

            content_chunks.append(chunk)

        content = b''.join(content_chunks).decode('utf-8')
        logger.info(f"M3U file downloaded successfully, size: {len(content)} characters")
        return content
    except URLError as e:
        logger.error(f"Error downloading M3U file: {e}")
        raise
    except UnicodeDecodeError as e:
        logger.error(f"Error decoding M3U file: {e}")
        raise
    except Exception as e:
        logger.error(f"Unexpected error downloading M3U file: {e}")
        raise


def remove_orig_suffix(channel_name: str) -> str:
    """
    Remove 'orig' suffix from channel names if it exists at the end.

    Args:
        channel_name (str): Original channel name

    Returns:
        str: Channel name with 'orig' suffix removed if it was at the end
    """
    if channel_name.lower().endswith(' orig'):
        return channel_name[:-5]  # Remove ' orig' (5 characters)
    return channel_name


def get_base_channel_name(channel_name: str) -> str:
    """
    Get the base channel name by removing 'orig' and 'hd' suffixes and normalizing.

    Args:
        channel_name (str): Channel name

    Returns:
        str: Base channel name without 'orig' and 'hd' suffixes
    """
    # Remove 'orig' and 'hd' suffixes in a loop until no more can be removed
    temp_name = channel_name
    changed = True

    while changed:
        changed = False
        if temp_name.lower().endswith(' orig'):
            temp_name = temp_name[:-5].strip()
            changed = True
        elif temp_name.lower().endswith(' hd'):
            temp_name = temp_name[:-3].strip()
            changed = True

    return temp_name


def filter_m3u_content(content: str, categories_to_keep: List[str], channel_names_to_exclude: List[str] = None, custom_epg_url: str = None) -> str:
    """
    Filter M3U content to keep only specified categories and apply channel name rules

    Args:
        content (str): Original M3U content
        categories_to_keep (list): List of categories to keep
        channel_names_to_exclude (list): List of channel name patterns to exclude
        custom_epg_url (str): Custom EPG URL to replace in the header (optional)

    Returns:
        str: Filtered M3U content
    """
    logger.info("Starting filtering process")

    # Convert categories to lowercase for comparison
    categories_lower = [cat.lower() for cat in categories_to_keep] if categories_to_keep else []

    # Convert channel names to exclude to lowercase for comparison
    channel_names_to_exclude_lower = [name.lower() for name in channel_names_to_exclude] if channel_names_to_exclude else []

    lines = content.split('\n')
    filtered_lines = []

    # Track if we're in a section that should be included
    include_entry = False

    # Check if the content contains EXTINF lines (extended M3U format)
    has_extinf_lines = any(line.strip().startswith('#EXTINF:') for line in lines)

    for i, line in enumerate(lines):
        line_lower = line.lower()

        # Security: Basic validation to prevent injection attacks
        if len(line) > 10000:  # Extremely long lines might be malicious
            logger.warning(f"Skipping extremely long line {i} (potential security issue)")
            continue

        # Check if this is the header line and update url-tvg if custom EPG URL is provided
        if line.strip().startswith('#EXTM3U'):
            if custom_epg_url:
                # Replace or add url-tvg attribute with custom EPG URL
                if 'url-tvg=' in line_lower:
                    # Update existing url-tvg attribute
                    line = re.sub(r'url-tvg="[^"]*"', f'url-tvg="{custom_epg_url}"', line, flags=re.IGNORECASE)
                else:
                    # Add url-tvg attribute if it doesn't exist
                    if line.endswith('>'):
                        line = line[:-1] + f' url-tvg="{custom_epg_url}">'
                    else:
                        line += f' url-tvg="{custom_epg_url}"'
            filtered_lines.append(line)
        # Check if this is an info line with category (tvg-name or group-title)
        elif line.strip().startswith('#EXTINF:'):
            include_entry = False  # Reset for each new entry

            # Check for group-title attribute in the EXTINF line
            # Extract the group-title value specifically to avoid matching in channel names
            group_title_match = re.search(r'group-title="([^"]*)"', line, re.IGNORECASE)
            if group_title_match:
                group_title = group_title_match.group(1).lower()
                for cat in categories_lower:
                    if cat == group_title:  # Exact match for group-title only
                        include_entry = True
                        logger.debug(f"Including entry with category '{cat}': {line[:100]}...")
                        break

            # If no categories to keep are specified, include all entries
            if not categories_lower:
                include_entry = True

            # Additional check for regional channels matching the pattern +x (регион) or +x HD
            if include_entry:
                # Extract the channel name from the EXTINF line
                # The channel name is typically after the last comma in the EXTINF line
                parts = line.rsplit(',', 1)
                if len(parts) > 1:
                    channel_name = parts[1].strip()

                    # Check if the channel name contains any of the excluded patterns
                    if channel_names_to_exclude_lower:
                        for excluded_pattern in channel_names_to_exclude_lower:
                            if excluded_pattern in channel_name.lower():
                                include_entry = False
                                logger.debug(f"Excluding channel by name pattern '{excluded_pattern}': {channel_name}")
                                break

                    # Check for regional pattern: +digit followed by optional HD and/or region in parentheses - like "+1 (Приволжье)", "+4 HD", etc.
                    # The pattern should match +X at the end of the channel name (possibly followed by HD or region)
                    if re.search(r'\s\+\d+(?:\s+HD)?(?:\s*\([^)]+\))?\s*$', channel_name, re.IGNORECASE):
                        include_entry = False
                        logger.debug(f"Excluding regional entry: {channel_name} in line: {line[:100]}...")

                    # Check for channels ending with numbers like "HD 50", "50", etc.
                    # Only match when there's a space before 2+ digit number (not single digits that might be part of channel names)
                    if re.search(r'\s\d{2,}$', channel_name):
                        include_entry = False
                        logger.debug(f"Excluding channel ending with numbers: {channel_name} in line: {line[:100]}...")

            if include_entry:
                # Remove 'orig' suffix from the channel name
                parts = line.rsplit(',', 1)
                if len(parts) > 1:
                    channel_name = parts[1].strip()
                    new_channel_name = remove_orig_suffix(channel_name)
                    line = f"{parts[0]},{new_channel_name}"

                filtered_lines.append(line)
        elif line.strip().startswith('http'):  # Simple URL line
            # If the content doesn't have EXTINF lines, treat all HTTP lines as valid entries
            if not has_extinf_lines:
                # If no categories are specified to filter, include all URLs
                if not categories_lower:
                    filtered_lines.append(line)
            # If we're in an included entry (after an EXTINF line), include the URL line
            elif include_entry:
                filtered_lines.append(line)
        elif include_entry:
            # Include the URL line that corresponds to the entry we're including
            filtered_lines.append(line)
        else:
            # Keep other lines (like #EXTM3U header, empty lines, etc.)
            # Only add non-category lines if we're including the previous entry
            if not line.strip().startswith('#EXTINF:') and not include_entry:
                # Check if it's a header line or other important line
                if line.strip().startswith('#EXTM3U') or not line.strip():
                    filtered_lines.append(line)
                else:
                    # For non-EXTINF content, if we don't have EXTINF lines, include other lines too
                    if not has_extinf_lines and line.strip():
                        filtered_lines.append(line)

    # Apply duplicate removal and HD preference rules
    processed_content = remove_duplicates_and_apply_hd_preference('\n'.join(filtered_lines))

    # Count channels before and after processing
    original_channels = count_channels(content)
    processed_channels = count_channels(processed_content)

    # Log filtering results with both line and channel counts
    original_lines = len(content.split('\n'))
    processed_lines = len(processed_content.split('\n'))
    logger.info(f"Filtering complete: {original_lines} lines -> {processed_lines} lines ({original_channels} channels -> {processed_channels} channels)")

    logger.info("Filtering process completed")
    return processed_content


def count_channels(content: str) -> int:
    """
    Count the number of channels in M3U content.

    Args:
        content (str): M3U content

    Returns:
        int: Number of channels (EXTINF entries)
    """
    lines = content.split('\n')
    channel_count = 0

    for line in lines:
        if line.strip().startswith('#EXTINF:'):
            channel_count += 1

    return channel_count


def apply_hd_preference(content: str) -> str:
    """
    Apply HD preference rule: if both HD and non-HD versions exist, keep only HD

    Args:
        content (str): M3U content after initial filtering

    Returns:
        str: M3U content with HD preference applied
    """
    lines = content.split('\n')

    # Separate header lines from channel entries
    header_lines: List[str] = []
    channel_entries: List[Tuple[str, str]] = []

    i = 0
    while i < len(lines):
        line = lines[i]

        if line.strip().startswith('#EXTINF:'):
            # This is an EXTINF line, get the corresponding URL
            if i + 1 < len(lines):
                url_line = lines[i + 1]
                channel_entries.append((line, url_line))
                i += 2  # Skip the next line since we've already processed it
            else:
                header_lines.append(line)
                i += 1
        else:
            # This is not an EXTINF line, add it to headers
            header_lines.append(line)
            i += 1

    # Group channels by base name (without 'hd' suffix)
    channel_groups: dict = {}
    for extinf_line, url_line in channel_entries:
        parts = extinf_line.rsplit(',', 1)
        if len(parts) > 1:
            channel_name = parts[1].strip()
            base_name = get_base_channel_name(channel_name)

            if base_name not in channel_groups:
                channel_groups[base_name] = []
            channel_groups[base_name].append((extinf_line, url_line))

    # For each group, decide which version(s) to keep
    final_channel_entries: List[Tuple[str, str]] = []
    for base_name, variants in channel_groups.items():
        # Check if any variant is an HD version
        has_hd_version = any(extinf.rsplit(',', 1)[1].strip().lower().endswith(' hd')
                             for extinf, _ in variants)

        if has_hd_version:
            # Get only HD versions
            hd_variants = [(extinf, url) for extinf, url in variants
                           if extinf.rsplit(',', 1)[1].strip().lower().endswith(' hd')]
            final_channel_entries.extend(hd_variants)

            # Log which non-HD versions were removed
            non_hd_variants = [(extinf, url) for extinf, url in variants
                               if not extinf.rsplit(',', 1)[1].strip().lower().endswith(' hd')]
            if non_hd_variants:
                logger.debug(f"Removed non-HD versions for '{base_name}': {[ext.rsplit(',', 1)[1].strip() for ext, _ in non_hd_variants]}")
        else:
            # No HD version exists, keep all non-HD versions
            final_channel_entries.extend(variants)

    # Reconstruct the final content
    final_lines = header_lines
    for extinf_line, url_line in final_channel_entries:
        final_lines.append(extinf_line)
        final_lines.append(url_line)

    return '\n'.join(final_lines)


def normalize_channel_name_for_comparison(channel_name: str) -> str:
    """
    Normalize channel name for comparison purposes, removing HD, orig and other suffixes
    regardless of their position in the name.

    Args:
        channel_name (str): Original channel name

    Returns:
        str: Normalized channel name for comparison
    """
    import re

    # Convert to lowercase for case-insensitive comparison
    normalized = channel_name.lower()

    # Remove common suffixes that indicate quality or version, regardless of position
    # Using regex to remove these terms as whole words
    suffixes_to_remove = [
        r'\bhd\b',      # HD as a whole word
        r'\borig\b',    # orig as a whole word
        r'\bsd\b',      # SD as a whole word
        r'\bfull hd\b', # Full HD
        r'\b4k\b',      # 4K
        r'\buhd\b',     # UHD
        r'\buhd tv\b',  # UHD TV
    ]

    for suffix in suffixes_to_remove:
        # Remove the suffix and any leading/trailing spaces
        normalized = re.sub(r'\s*' + suffix + r'\s*', ' ', normalized)

    # Clean up extra spaces
    normalized = ' '.join(normalized.split())

    return normalized


def remove_duplicates_and_apply_hd_preference(content: str) -> str:
    """
    Remove duplicate channels based on tvg-id and channel name, keeping only the best version.
    Also applies HD preference rule: if both HD and non-HD versions exist, keep only HD.

    Args:
        content (str): M3U content after initial filtering

    Returns:
        str: M3U content with duplicates removed and HD preference applied
    """
    lines = content.split('\n')

    # Separate header lines from channel entries
    header_lines: List[str] = []
    channel_entries: List[Tuple[str, str]] = []

    i = 0
    while i < len(lines):
        line = lines[i]

        if line.strip().startswith('#EXTINF:'):
            # This is an EXTINF line, get the corresponding URL
            if i + 1 < len(lines):
                url_line = lines[i + 1]
                channel_entries.append((line, url_line))
                i += 2  # Skip the next line since we've already processed it
            else:
                header_lines.append(line)
                i += 1
        else:
            # This is not an EXTINF line, add it to headers
            header_lines.append(line)
            i += 1

    # Create a dictionary to track unique channels based on normalized name
    unique_channels: dict = {}

    for extinf_line, url_line in channel_entries:
        # Extract tvg-id from the EXTINF line
        tvg_id_match = re.search(r'tvg-id="([^"]*)"', extinf_line)
        tvg_id = tvg_id_match.group(1) if tvg_id_match else ""

        # Extract channel name from the EXTINF line
        parts = extinf_line.rsplit(',', 1)
        if len(parts) > 1:
            channel_name = parts[1].strip()
            # Use the normalized name for grouping
            normalized_name = normalize_channel_name_for_comparison(channel_name)
        else:
            channel_name = ""
            normalized_name = ""

        # Create a key based on normalized name for grouping similar channels
        # This allows us to handle cases where tvg-id differs but channel names suggest same channel
        key = normalized_name

        # If this key doesn't exist, add it
        if key not in unique_channels:
            unique_channels[key] = []

        # Add this variant to the list
        unique_channels[key].append((extinf_line, url_line))

    # For each group of channels, decide which version(s) to keep
    final_channel_entries: List[Tuple[str, str]] = []
    for key, variants in unique_channels.items():
        # Separate HD and non-HD versions
        hd_variants = []
        non_hd_variants = []

        for extinf, url in variants:
            channel_name = extinf.rsplit(',', 1)[1].strip()
            # Check if the channel name contains ' hd' anywhere (case insensitive)
            if ' hd' in channel_name.lower():
                hd_variants.append((extinf, url))
            else:
                non_hd_variants.append((extinf, url))

        # If both HD and non-HD versions exist, only consider HD versions
        if hd_variants and non_hd_variants:
            variants_to_process = hd_variants
            # Log which non-HD versions were removed
            removed_channels = [ext.rsplit(',', 1)[1].strip() for ext, _ in non_hd_variants]
            logger.debug(f"Removed non-HD versions for '{key}': {removed_channels}")
        else:
            # Otherwise, consider all variants for duplicate removal
            variants_to_process = variants

        # If there are multiple variants to process, apply tvg-rec preference
        if len(variants_to_process) > 1:
            # Sort by tvg-rec value (if present) in descending order
            sorted_variants = sorted(variants_to_process,
                                    key=lambda x: int(re.search(r'tvg-rec="(\d+)"', x[0]).group(1)) if re.search(r'tvg-rec="(\d+)"', x[0]) else 0,
                                    reverse=True)
            # Keep only the first one (highest tvg-rec)
            final_channel_entries.append(sorted_variants[0])

            # Log which duplicates were removed
            if len(sorted_variants) > 1:
                removed_channels = [ext.rsplit(',', 1)[1].strip() for ext, _ in sorted_variants[1:]]
                logger.debug(f"Removed duplicate versions for '{key}': {removed_channels}")
        else:
            # Only one variant, add it directly
            final_channel_entries.extend(variants_to_process)

    # Reconstruct the final content
    final_lines = header_lines
    for extinf_line, url_line in final_channel_entries:
        final_lines.append(extinf_line)
        final_lines.append(url_line)

    return '\n'.join(final_lines)