"""
Simple M3U Playlist Filter Package

This package provides functionality to filter M3U playlists based on categories
and channel names, and upload the filtered playlists to cloud storage.
"""

from .m3u_processor import (
    download_m3u,
    filter_m3u_content,
    parse_categories_file,
    apply_channel_metadata,
    add_tvg_ids_to_playlist,
)
from .epg_processor import (
    download_epg,
    filter_epg_content,
    extract_channel_info_from_playlist,
    build_epg_name_to_id_map,
    save_filtered_epg_locally,
)
from .config import Config
from .s3_operations import (
    upload_to_s3,
    upload_file_to_s3,
    upload_archive_to_s3,
)

__version__ = "1.0.0"
__author__ = "Vyacheslav Egorov"