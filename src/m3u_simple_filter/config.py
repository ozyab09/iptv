"""
Configuration module for M3U filter script.

This module contains all the configuration settings for the M3U filtering script.
"""

import os
from typing import List, Optional


class Config:
    """
    Configuration class for M3U filter
    """
    
    # Security: Maximum allowed file sizes (100MB for M3U, 500MB for EPG)
    MAX_M3U_FILE_SIZE: int = 100 * 1024 * 1024
    MAX_EPG_FILE_SIZE: int = 500 * 1024 * 1024

    # Categories to keep (for initial config file creation)
    DEFAULT_CATEGORIES_TO_KEEP: List[str] = [
        "Россия | Russia",
        "Общие",
        "Развлекательные",
        "Новостные",
        "Познавательные",
        "Детские",
        "Музыка",
        "Региональные",
        "Европа | Europe",
        "Австралия | Australia",
        "Беларусь | Беларускія",
        "Великобритания | United Kingdom",
        "Канада | Canada",
        "США | USA",
        "Кино"
    ]

    # Channel names to exclude (for initial config file creation)
    DEFAULT_CHANNEL_NAMES_TO_EXCLUDE: List[str] = [
        "Fashion",
        "СПАС",
        "Три ангела",
        "ЛДПР",
        "UA",
        "Sports"
    ]

    # Categories for which EPG should NOT be saved (for initial config file creation)
    DEFAULT_EPG_EXCLUDED_CATEGORIES: List[str] = [
        "Кино"  # Exclude EPG for movie channels
    ]

    # Specific channel IDs for which EPG should NOT be saved (for initial config file creation)
    DEFAULT_EPG_EXCLUDED_CHANNEL_IDS: List[str] = [
        "2745",  # Home 4K
        "6170",  # VF Хит-парад
        "6168",  # VF Metal
        "7553",  # VF Britney Spears
        "6171",  # VF Русский рок
        "9228",  # VF Король и Шут
        "7552",  # VF Modern Talking
        "4729",  # Cartoons Short
        "7594",  # VF Союзмультфильм
        "7595",  # VF Ералаш
        "9233",  # VF Каламбур
        "8822",  # Z! Science HD
        "8817",  # BOX Gurman HD
        "2438",  # Капитан Фантастика
        "8811",  # Yosso TV Food
        "6848",  # BCU Little HD
        "9025",  # Kids TV HD
        "153",   # Авто Плюс
        "66",    # Уникум
        "2760",  # Анекдот ТВ
        "494",   # Jim Jam
        "6135",  # VF Музыкальный Новый год!
        "9303",  # BOX Kosmo 4K
        "5387",  # YOSSO TV Союзмульт
        "2420",  # ЕГЭ ТВ
        "2239",  # Малыш
        "9183",  # Cartoon Classics
        "774",   # Flix Snip
        "810",   # Gulli
        "6419"   # VF Баня
    ]

    def __init__(self):
        """Initialize configuration from environment variables"""
        # Initialize all configuration values
        self._m3u_source_url = os.getenv('M3U_SOURCE_URL', 'https://your-provider.com/playlist.m3u')
        self._s3_default_bucket_name = os.getenv('S3_BUCKET_NAME', 'your-bucket-name')
        self._s3_filtered_playlist_key = os.getenv('S3_OBJECT_KEY', 'playlist.m3u')
        self._s3_endpoint_url = os.getenv('S3_ENDPOINT_URL', 'https://s3.amazonaws.com')
        self._s3_region = os.getenv('S3_REGION', 'us-east-1')
        self._epg_source_url = os.getenv('EPG_SOURCE_URL', 'https://your-epg-provider.com/epg.xml.gz')
        self._s3_epg_key = os.getenv('S3_EPG_KEY', 'epg.xml.gz')
        self._local_epg_path = os.getenv('LOCAL_EPG_PATH', 'epg.xml.gz')
        self._output_dir = os.getenv('OUTPUT_DIR', 'output')
        
        # Parse numeric values
        self._epg_retention_days = int(os.getenv('EPG_RETENTION_DAYS', '10'))
        self._epg_past_retention_days = int(os.getenv('EPG_PAST_RETENTION_DAYS', '0'))
        self._excluded_channels_future_limit_days = int(os.getenv('EXCLUDED_CHANNELS_FUTURE_LIMIT_DAYS', '1'))
        self._excluded_channels_past_limit_hours = int(os.getenv('EXCLUDED_CHANNELS_PAST_LIMIT_HOURS', '1'))
        
        # Initialize lists
        self._categories_to_keep = self.DEFAULT_CATEGORIES_TO_KEEP.copy()
        self._channel_names_to_exclude = self.DEFAULT_CHANNEL_NAMES_TO_EXCLUDE.copy()
        self._epg_excluded_categories = self.DEFAULT_EPG_EXCLUDED_CATEGORIES.copy()
        self._epg_excluded_channel_ids = self.DEFAULT_EPG_EXCLUDED_CHANNEL_IDS.copy()

    @property
    def M3U_SOURCE_URL(self) -> str:
        """M3U source URL from environment variable or default"""
        return self._m3u_source_url

    @property
    def S3_DEFAULT_BUCKET_NAME(self) -> str:
        """S3 default bucket name from environment variable or default"""
        return self._s3_default_bucket_name

    @property
    def S3_FILTERED_PLAYLIST_KEY(self) -> str:
        """S3 filtered playlist key from environment variable or default"""
        return self._s3_filtered_playlist_key

    @property
    def S3_ALL_CATEGORIES_PLAYLIST_KEY(self) -> str:
        """S3 all categories playlist key"""
        return 'playlist-all.m3u'

    @property
    def S3_ENDPOINT_URL(self) -> str:
        """S3 endpoint URL from environment variable or default"""
        return self._s3_endpoint_url

    @property
    def S3_REGION(self) -> str:
        """S3 region from environment variable or default"""
        return self._s3_region

    @property
    def S3_COMPATIBLE_CONFIG(self) -> dict:
        """S3-compatible storage configuration from properties"""
        return {
            "endpoint_url": self.S3_ENDPOINT_URL,
            "region": self.S3_REGION
        }

    @property
    def EPG_RETENTION_DAYS(self) -> int:
        """Number of days to retain EPG data from current date"""
        return self._epg_retention_days

    @property
    def EPG_PAST_RETENTION_DAYS(self) -> int:
        """Number of days in the past to retain EPG data (programs that ended recently)"""
        return self._epg_past_retention_days

    @property
    def EXCLUDED_CHANNELS_FUTURE_LIMIT_DAYS(self) -> int:
        """Number of days in the future to retain programs for excluded channels"""
        return self._excluded_channels_future_limit_days

    @property
    def EXCLUDED_CHANNELS_PAST_LIMIT_HOURS(self) -> int:
        """Number of hours in the past (before now) to retain programs for excluded channels that have already ended"""
        return self._excluded_channels_past_limit_hours

    @property
    def LOCAL_FILTERED_PLAYLIST_PATH(self) -> str:
        """Local path for filtered playlist, using S3_OBJECT_KEY environment variable or default"""
        return os.getenv('S3_OBJECT_KEY', 'playlist.m3u')

    @property
    def LOCAL_ALL_CATEGORIES_PLAYLIST_PATH(self) -> str:
        """Local path for all categories playlist, derived from S3_OBJECT_KEY"""
        s3_object_key: str = os.getenv('S3_OBJECT_KEY', 'playlist.m3u')
        if '.' in s3_object_key:
            name, ext = s3_object_key.rsplit('.', 1)
            return f"{name}-all.{ext}"
        else:
            return f"{s3_object_key}-all"

    @property
    def EPG_SOURCE_URL(self) -> str:
        """EPG source URL from environment variable or default"""
        return self._epg_source_url

    @property
    def S3_EPG_KEY(self) -> str:
        """S3 EPG key from environment variable or default"""
        return self._s3_epg_key

    @property
    def LOCAL_EPG_PATH(self) -> str:
        """Local path for downloaded EPG file"""
        return self._local_epg_path

    @property
    def LOCAL_FILTERED_EPG_PATH(self) -> str:
        """Local path for filtered EPG file"""
        s3_epg_key: str = os.getenv('S3_EPG_KEY', 'epg.xml.gz')
        if '.' in s3_epg_key:
            name, ext = s3_epg_key.rsplit('.', 1)
            return f"{name}-filtered.{ext}"
        else:
            return f"{s3_epg_key}-filtered"

    @property
    def OUTPUT_DIR(self) -> str:
        """Output directory for saving processed files"""
        return self._output_dir

    @property
    def CATEGORIES_TO_KEEP(self) -> List[str]:
        """Return the list of categories to keep"""
        return self._categories_to_keep

    @property
    def CHANNEL_NAMES_TO_EXCLUDE(self) -> List[str]:
        """Return the list of channel names to exclude"""
        return self._channel_names_to_exclude

    def get_categories_to_keep(self) -> List[str]:
        """Return the list of categories to keep"""
        return self._categories_to_keep

    def get_channel_names_to_exclude(self) -> List[str]:
        """Return the list of channel names to exclude"""
        return self._channel_names_to_exclude

    def get_epg_excluded_categories(self) -> List[str]:
        """Return the list of categories for which EPG should not be saved"""
        return self._epg_excluded_categories

    def get_epg_excluded_channel_ids(self) -> List[str]:
        """Return the list of channel IDs for which EPG should not be saved"""
        return self._epg_excluded_channel_ids

    def validate_config(self) -> List[str]:
        """
        Validate configuration settings and return list of validation errors.

        Returns:
            List[str]: List of validation errors, empty if all validations pass
        """
        errors = []

        # Validate M3U source URL
        if not self.M3U_SOURCE_URL or not self.M3U_SOURCE_URL.startswith(('http://', 'https://')):
            errors.append("M3U_SOURCE_URL must be a valid HTTP/HTTPS URL")

        # Validate EPG source URL
        if not self.EPG_SOURCE_URL or not self.EPG_SOURCE_URL.startswith(('http://', 'https://')):
            errors.append("EPG_SOURCE_URL must be a valid HTTP/HTTPS URL")

        # Validate S3 bucket name
        if not self.S3_DEFAULT_BUCKET_NAME or len(self.S3_DEFAULT_BUCKET_NAME) < 3 or len(self.S3_DEFAULT_BUCKET_NAME) > 63:
            errors.append("S3_DEFAULT_BUCKET_NAME must be between 3 and 63 characters")

        # Validate S3 object key
        if not self.S3_FILTERED_PLAYLIST_KEY or '..' in self.S3_FILTERED_PLAYLIST_KEY or self.S3_FILTERED_PLAYLIST_KEY.startswith('/'):
            errors.append("S3_OBJECT_KEY must not contain '..' or start with '/'")

        # Validate S3 EPG key
        if not self.S3_EPG_KEY or '..' in self.S3_EPG_KEY or self.S3_EPG_KEY.startswith('/'):
            errors.append("S3_EPG_KEY must not contain '..' or start with '/'")

        # Validate S3 endpoint URL
        if not self.S3_ENDPOINT_URL or not self.S3_ENDPOINT_URL.startswith(('http://', 'https://')):
            errors.append("S3_ENDPOINT_URL must be a valid HTTP/HTTPS URL")

        # Additional validation to check for common malformed endpoint patterns
        endpoint_url = self.S3_ENDPOINT_URL
        if endpoint_url and len(endpoint_url) > 10:  # Basic length check
            # Check if the URL looks like it has credentials embedded or is malformed
            if '@' in endpoint_url.split('/')[2] if len(endpoint_url.split('/')) > 2 else False:
                errors.append("S3_ENDPOINT_URL should not contain credentials in the URL")

        # Validate S3 region
        if not self.S3_REGION:
            errors.append("S3_REGION must be specified")

        return errors