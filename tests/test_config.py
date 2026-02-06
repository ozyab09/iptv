"""
Unit tests for configuration module.
"""

import unittest
import os
from src.m3u_simple_filter.config import Config


class TestConfig(unittest.TestCase):
    """Test cases for Config class."""

    def setUp(self):
        """Set up test fixtures before each test method."""
        # Store original environment variables
        self.original_env = {
            'M3U_SOURCE_URL': os.environ.get('M3U_SOURCE_URL'),
            'S3_BUCKET_NAME': os.environ.get('S3_BUCKET_NAME'),
            'S3_OBJECT_KEY': os.environ.get('S3_OBJECT_KEY'),
            'S3_ENDPOINT_URL': os.environ.get('S3_ENDPOINT_URL'),
            'S3_REGION': os.environ.get('S3_REGION'),
        }

    def tearDown(self):
        """Clean up after each test method."""
        # Restore original environment variables
        for key, value in self.original_env.items():
            if value is not None:
                os.environ[key] = value
            elif key in os.environ:
                del os.environ[key]


    def test_environment_variable_override(self):
        """Test that environment variables override default values."""
        # Set environment variables
        os.environ['M3U_SOURCE_URL'] = 'https://test.com/playlist.m3u'
        os.environ['S3_BUCKET_NAME'] = 'test-bucket'
        os.environ['S3_OBJECT_KEY'] = 'test-playlist.m3u'
        os.environ['S3_ENDPOINT_URL'] = 'https://test-storage.com'
        os.environ['S3_REGION'] = 'test-region'

        config = Config()

        self.assertEqual(config.M3U_SOURCE_URL, 'https://test.com/playlist.m3u')
        self.assertEqual(config.S3_DEFAULT_BUCKET_NAME, 'test-bucket')
        self.assertEqual(config.S3_FILTERED_PLAYLIST_KEY, 'test-playlist.m3u')
        self.assertEqual(config.S3_ENDPOINT_URL, 'https://test-storage.com')
        self.assertEqual(config.S3_REGION, 'test-region')
        self.assertEqual(config.S3_COMPATIBLE_CONFIG["endpoint_url"], 'https://test-storage.com')
        self.assertEqual(config.S3_COMPATIBLE_CONFIG["region"], 'test-region')

    def test_local_playlist_paths(self):
        """Test local playlist path properties."""
        # Test default case
        config = Config()
        self.assertEqual(config.LOCAL_FILTERED_PLAYLIST_PATH, 'playlist.m3u')
        self.assertEqual(config.LOCAL_ALL_CATEGORIES_PLAYLIST_PATH, 'playlist-all.m3u')

        # Test with custom S3_OBJECT_KEY
        os.environ['S3_OBJECT_KEY'] = 'custom.m3u'
        config = Config()
        self.assertEqual(config.LOCAL_FILTERED_PLAYLIST_PATH, 'custom.m3u')
        self.assertEqual(config.LOCAL_ALL_CATEGORIES_PLAYLIST_PATH, 'custom-all.m3u')

        # Test with S3_OBJECT_KEY without extension
        os.environ['S3_OBJECT_KEY'] = 'custom'
        config = Config()
        self.assertEqual(config.LOCAL_FILTERED_PLAYLIST_PATH, 'custom')
        self.assertEqual(config.LOCAL_ALL_CATEGORIES_PLAYLIST_PATH, 'custom-all')

    def test_categories_to_keep(self):
        """Test categories to keep configuration."""
        config = Config()
        expected_categories = [
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

        self.assertEqual(config.get_categories_to_keep(), expected_categories)
        self.assertEqual(config.CATEGORIES_TO_KEEP, expected_categories)

    def test_channel_names_to_exclude(self):
        """Test channel names to exclude configuration."""
        config = Config()
        expected_exclusions = [
            "Fashion",
            "СПАС",
            "Три ангела",
            "ЛДПР",
            "UA",
            "Sports"
        ]

        self.assertEqual(config.get_channel_names_to_exclude(), expected_exclusions)
        self.assertEqual(config.CHANNEL_NAMES_TO_EXCLUDE, expected_exclusions)

    def test_epg_retention_days_default(self):
        """Test that EPG retention days defaults to 10."""
        config = Config()
        self.assertEqual(config.EPG_RETENTION_DAYS, 10)

    def test_epg_retention_days_from_env(self):
        """Test that EPG retention days can be set from environment variable."""
        # Save original value
        original_value = os.environ.get('EPG_RETENTION_DAYS')

        try:
            # Set environment variable
            os.environ['EPG_RETENTION_DAYS'] = '7'

            # Create new config instance to pick up env var
            config = Config()
            self.assertEqual(config.EPG_RETENTION_DAYS, 7)
        finally:
            # Restore original value
            if original_value is not None:
                os.environ['EPG_RETENTION_DAYS'] = original_value
            else:
                os.environ.pop('EPG_RETENTION_DAYS', None)

    def test_epg_past_retention_days_default(self):
        """Test that EPG past retention days defaults to 0."""
        config = Config()
        self.assertEqual(config.EPG_PAST_RETENTION_DAYS, 0)

    def test_epg_past_retention_days_from_env(self):
        """Test that EPG past retention days can be set from environment variable."""
        # Save original value
        original_value = os.environ.get('EPG_PAST_RETENTION_DAYS')

        try:
            # Set environment variable
            os.environ['EPG_PAST_RETENTION_DAYS'] = '4'

            # Create new config instance to pick up env var
            config = Config()
            self.assertEqual(config.EPG_PAST_RETENTION_DAYS, 4)
        finally:
            # Restore original value
            if original_value is not None:
                os.environ['EPG_PAST_RETENTION_DAYS'] = original_value
            else:
                os.environ.pop('EPG_PAST_RETENTION_DAYS', None)


if __name__ == '__main__':
    unittest.main()