"""
Simple M3U Playlist Filter Package

This package provides functionality to filter M3U playlists based on categories
and channel names, and upload the filtered playlists to cloud storage.
"""

from .m3u_processor import download_m3u, filter_m3u_content
from .epg_processor import download_epg, filter_epg_content
from .config import Config
from .s3_operations import upload_to_s3

__version__ = "1.0.0"
__author__ = "Vyacheslav Egorov"