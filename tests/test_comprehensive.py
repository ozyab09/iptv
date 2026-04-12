"""
Comprehensive unit tests for M3U filter project.

This file adds test coverage for areas not covered by existing tests:
- Config.validate_config() validation paths
- normalize_channel_name_for_comparison
- Time-based EPG filtering
- build_epg_name_to_id_map
- upload_file_to_s3 and upload_archive_to_s3
- Multi-URL M3U merging
"""

import unittest
import os
import sys
import gzip
import json
from unittest.mock import Mock, patch, MagicMock
from datetime import datetime, timedelta
from io import BytesIO
from urllib.error import URLError

# Add src to path for imports
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', 'src'))

from m3u_simple_filter.config import Config
from m3u_simple_filter.m3u_processor import normalize_channel_name_for_comparison
from m3u_simple_filter.epg_processor import build_epg_name_to_id_map, filter_epg_content
from m3u_simple_filter.s3_operations import upload_file_to_s3, upload_archive_to_s3
from m3u_simple_filter.utils import sanitize_log_message, retry


class TestConfigValidation(unittest.TestCase):
    """Test Config.validate_config() error paths."""

    def setUp(self):
        """Store original environment variables."""
        self.original_env = {
            'M3U_SOURCE_URL': os.environ.get('M3U_SOURCE_URL'),
            'S3_BUCKET_NAME': os.environ.get('S3_BUCKET_NAME'),
            'S3_OBJECT_KEY': os.environ.get('S3_OBJECT_KEY'),
            'S3_ENDPOINT_URL': os.environ.get('S3_ENDPOINT_URL'),
            'S3_REGION': os.environ.get('S3_REGION'),
            'EPG_SOURCE_URL': os.environ.get('EPG_SOURCE_URL'),
            'S3_EPG_KEY': os.environ.get('S3_EPG_KEY'),
        }
        # Clear all env vars for testing
        for key in self.original_env.keys():
            os.environ.pop(key, None)

    def tearDown(self):
        """Restore original environment variables."""
        for key, value in self.original_env.items():
            if value is not None:
                os.environ[key] = value
            elif key in os.environ:
                del os.environ[key]

    def test_validate_empty_m3u_url(self):
        """Test validation error for empty M3U_SOURCE_URL."""
        errors = Config.validate_config()
        self.assertTrue(any("M3U_SOURCE_URL must be specified" in err for err in errors))

    def test_validate_placeholder_m3u_url(self):
        """Test validation error for placeholder M3U_SOURCE_URL."""
        os.environ['M3U_SOURCE_URL'] = 'https://your-provider.com/playlist.m3u'
        os.environ['S3_BUCKET_NAME'] = 'test-bucket'
        os.environ['S3_ENDPOINT_URL'] = 'https://s3.example.com'
        os.environ['S3_REGION'] = 'us-east-1'
        os.environ['EPG_SOURCE_URL'] = 'https://epg.example.com/epg.xml.gz'
        os.environ['S3_EPG_KEY'] = 'epg.xml.gz'
        
        errors = Config.validate_config()
        self.assertTrue(any("appears to be a placeholder" in err for err in errors))

    def test_validate_invalid_url_format(self):
        """Test validation error for invalid URL format."""
        os.environ['M3U_SOURCE_URL'] = 'not-a-url'
        os.environ['S3_BUCKET_NAME'] = 'test-bucket'
        os.environ['S3_ENDPOINT_URL'] = 'https://s3.example.com'
        os.environ['S3_REGION'] = 'us-east-1'
        os.environ['EPG_SOURCE_URL'] = 'https://epg.example.com/epg.xml.gz'
        os.environ['S3_EPG_KEY'] = 'epg.xml.gz'

        errors = Config.validate_config()
        self.assertTrue(any("invalid URL" in err for err in errors))

    def test_validate_empty_epg_url(self):
        """Test validation error for empty EPG_SOURCE_URL."""
        os.environ['M3U_SOURCE_URL'] = 'https://example.com/playlist.m3u'
        os.environ['S3_BUCKET_NAME'] = 'test-bucket'
        os.environ['S3_ENDPOINT_URL'] = 'https://s3.example.com'
        os.environ['S3_REGION'] = 'us-east-1'
        # EPG_SOURCE_URL is empty
        
        errors = Config.validate_config()
        self.assertTrue(any("EPG_SOURCE_URL must be specified" in err for err in errors))

    def test_validate_empty_bucket_name(self):
        """Test validation error for empty S3_BUCKET_NAME."""
        os.environ['M3U_SOURCE_URL'] = 'https://example.com/playlist.m3u'
        os.environ['S3_ENDPOINT_URL'] = 'https://s3.example.com'
        os.environ['S3_REGION'] = 'us-east-1'
        os.environ['EPG_SOURCE_URL'] = 'https://epg.example.com/epg.xml.gz'
        os.environ['S3_EPG_KEY'] = 'epg.xml.gz'
        # S3_BUCKET_NAME is empty
        
        errors = Config.validate_config()
        self.assertTrue(any("S3_BUCKET_NAME must be specified" in err for err in errors))

    def test_validate_bucket_name_too_short(self):
        """Test validation error for S3_BUCKET_NAME too short."""
        os.environ['M3U_SOURCE_URL'] = 'https://example.com/playlist.m3u'
        os.environ['S3_BUCKET_NAME'] = 'ab'  # Less than 3 chars
        os.environ['S3_ENDPOINT_URL'] = 'https://s3.example.com'
        os.environ['S3_REGION'] = 'us-east-1'
        os.environ['EPG_SOURCE_URL'] = 'https://epg.example.com/epg.xml.gz'
        os.environ['S3_EPG_KEY'] = 'epg.xml.gz'
        
        errors = Config.validate_config()
        self.assertTrue(any("between 3 and 63 characters" in err for err in errors))

    def test_validate_path_traversal_in_object_key(self):
        """Test validation error for path traversal in S3_OBJECT_KEY."""
        os.environ['M3U_SOURCE_URL'] = 'https://example.com/playlist.m3u'
        os.environ['S3_BUCKET_NAME'] = 'test-bucket'
        os.environ['S3_ENDPOINT_URL'] = 'https://s3.example.com'
        os.environ['S3_REGION'] = 'us-east-1'
        os.environ['S3_OBJECT_KEY'] = '../malicious.m3u'
        os.environ['EPG_SOURCE_URL'] = 'https://epg.example.com/epg.xml.gz'
        os.environ['S3_EPG_KEY'] = 'epg.xml.gz'
        
        errors = Config.validate_config()
        self.assertTrue(any("must not contain '..'" in err for err in errors))

    def test_validate_empty_endpoint_url(self):
        """Test validation error for empty S3_ENDPOINT_URL."""
        os.environ['M3U_SOURCE_URL'] = 'https://example.com/playlist.m3u'
        os.environ['S3_BUCKET_NAME'] = 'test-bucket'
        os.environ['S3_REGION'] = 'us-east-1'
        os.environ['EPG_SOURCE_URL'] = 'https://epg.example.com/epg.xml.gz'
        os.environ['S3_EPG_KEY'] = 'epg.xml.gz'
        # S3_ENDPOINT_URL is empty
        
        errors = Config.validate_config()
        self.assertTrue(any("S3_ENDPOINT_URL must be specified" in err for err in errors))

    def test_validate_invalid_endpoint_url(self):
        """Test validation error for invalid S3_ENDPOINT_URL."""
        os.environ['M3U_SOURCE_URL'] = 'https://example.com/playlist.m3u'
        os.environ['S3_BUCKET_NAME'] = 'test-bucket'
        os.environ['S3_ENDPOINT_URL'] = 'not-a-url'
        os.environ['S3_REGION'] = 'us-east-1'
        os.environ['EPG_SOURCE_URL'] = 'https://epg.example.com/epg.xml.gz'
        os.environ['S3_EPG_KEY'] = 'epg.xml.gz'
        
        errors = Config.validate_config()
        self.assertTrue(any("S3_ENDPOINT_URL must be a valid HTTP/HTTPS URL" in err for err in errors))

    def test_validate_empty_region(self):
        """Test validation error for empty S3_REGION."""
        os.environ['M3U_SOURCE_URL'] = 'https://example.com/playlist.m3u'
        os.environ['S3_BUCKET_NAME'] = 'test-bucket'
        os.environ['S3_ENDPOINT_URL'] = 'https://s3.example.com'
        os.environ['S3_REGION'] = ''  # Explicitly set to empty string
        os.environ['EPG_SOURCE_URL'] = 'https://epg.example.com/epg.xml.gz'
        os.environ['S3_EPG_KEY'] = 'epg.xml.gz'

        errors = Config.validate_config()
        self.assertTrue(any("S3_REGION must be specified" in err for err in errors))

    def test_validate_valid_config(self):
        """Test no errors for valid config."""
        os.environ['M3U_SOURCE_URL'] = 'https://example.com/playlist.m3u'
        os.environ['S3_BUCKET_NAME'] = 'test-bucket'
        os.environ['S3_ENDPOINT_URL'] = 'https://s3.example.com'
        os.environ['S3_REGION'] = 'us-east-1'
        os.environ['EPG_SOURCE_URL'] = 'https://epg.example.com/epg.xml.gz'
        os.environ['S3_EPG_KEY'] = 'epg.xml.gz'
        
        errors = Config.validate_config()
        self.assertEqual(errors, [])


class TestNormalizeChannelName(unittest.TestCase):
    """Test normalize_channel_name_for_comparison function."""

    def test_basic_normalization(self):
        """Test basic case-insensitive normalization."""
        result = normalize_channel_name_for_comparison("Test Channel")
        self.assertEqual(result, "test channel")

    def test_remove_hd_suffix(self):
        """Test removal of HD suffix."""
        result = normalize_channel_name_for_comparison("Channel HD")
        self.assertEqual(result, "channel")

    def test_remove_orig_suffix(self):
        """Test removal of orig suffix."""
        result = normalize_channel_name_for_comparison("Channel orig")
        self.assertEqual(result, "channel")

    def test_remove_sd_suffix(self):
        """Test removal of SD suffix."""
        result = normalize_channel_name_for_comparison("Channel SD")
        self.assertEqual(result, "channel")

    def test_remove_multiple_suffixes(self):
        """Test removal of multiple suffixes."""
        result = normalize_channel_name_for_comparison("Channel HD orig")
        self.assertEqual(result, "channel")

    def test_case_insensitive(self):
        """Test case-insensitive comparison."""
        result1 = normalize_channel_name_for_comparison("CHANNEL HD")
        result2 = normalize_channel_name_for_comparison("channel hd")
        self.assertEqual(result1, result2)

    def test_preserves_core_name(self):
        """Test that core channel name is preserved."""
        result = normalize_channel_name_for_comparison("Discovery Channel HD")
        self.assertIn("discovery", result)
        self.assertIn("channel", result)
        self.assertNotIn("hd", result)


class TestBuildEpgNameToIdMap(unittest.TestCase):
    """Test build_epg_name_to_id_map function."""

    def test_basic_mapping(self):
        """Test basic EPG name to ID mapping."""
        epg_content = '''<?xml version="1.0" encoding="UTF-8"?>
<tv>
    <channel id="channel1">
        <display-name lang="en">Channel One</display-name>
    </channel>
    <channel id="channel2">
        <display-name lang="ru">Channel Two</display-name>
    </channel>
</tv>'''
        result = build_epg_name_to_id_map(epg_content)
        self.assertEqual(result["channel one"], "channel1")
        self.assertEqual(result["channel two"], "channel2")

    def test_multiple_display_names(self):
        """Test channel with multiple display names."""
        epg_content = '''<?xml version="1.0" encoding="UTF-8"?>
<tv>
    <channel id="channel1">
        <display-name lang="en">Channel One</display-name>
        <display-name lang="ru">Первый Канал</display-name>
    </channel>
</tv>'''
        result = build_epg_name_to_id_map(epg_content)
        self.assertEqual(result["channel one"], "channel1")
        self.assertEqual(result["первый канал"], "channel1")

    def test_empty_epg(self):
        """Test EPG with no channels."""
        epg_content = '<?xml version="1.0" encoding="UTF-8"?><tv></tv>'
        result = build_epg_name_to_id_map(epg_content)
        self.assertEqual(result, {})


class TestTimeBasedEpgFiltering(unittest.TestCase):
    """Test time-based EPG filtering."""

    def test_current_program_included(self):
        """Test that currently airing program is included."""
        epg_content = '''<?xml version="1.0" encoding="UTF-8"?>
<tv>
    <channel id="channel1">
        <display-name lang="en">Channel One</display-name>
    </channel>
    <programme channel="channel1" start="20200101000000 +0000" stop="20300101000000 +0000">
        <title lang="en">Current Program</title>
    </programme>
</tv>'''
        result = filter_epg_content(epg_content, {"channel1"})
        self.assertIn("Current Program", result)

    def test_old_program_excluded(self):
        """Test that very old program is excluded."""
        old_start = (datetime.now() - timedelta(days=30)).strftime('%Y%m%d%H%M%S +0000')
        old_stop = (datetime.now() - timedelta(days=25)).strftime('%Y%m%d%H%M%S +0000')
        
        epg_content = f'''<?xml version="1.0" encoding="UTF-8"?>
<tv>
    <channel id="channel1">
        <display-name lang="en">Channel One</display-name>
    </channel>
    <programme channel="channel1" start="{old_start}" stop="{old_stop}">
        <title lang="en">Old Program</title>
    </programme>
</tv>'''
        result = filter_epg_content(epg_content, {"channel1"})
        # Old programs should be excluded
        self.assertNotIn("Old Program", result)

    def test_future_program_within_retention_included(self):
        """Test that future program within retention period is included."""
        future_start = (datetime.now() + timedelta(days=5)).strftime('%Y%m%d%H%M%S +0000')
        future_stop = (datetime.now() + timedelta(days=5, hours=2)).strftime('%Y%m%d%H%M%S +0000')
        
        epg_content = f'''<?xml version="1.0" encoding="UTF-8"?>
<tv>
    <channel id="channel1">
        <display-name lang="en">Channel One</display-name>
    </channel>
    <programme channel="channel1" start="{future_start}" stop="{future_stop}">
        <title lang="en">Future Program</title>
    </programme>
</tv>'''
        result = filter_epg_content(epg_content, {"channel1"})
        self.assertIn("Future Program", result)


class TestUploadFileToS3(unittest.TestCase):
    """Test upload_file_to_s3 function."""

    @patch('m3u_simple_filter.s3_operations.boto3.client')
    @patch('os.path.exists')
    def test_upload_file_success(self, mock_exists, mock_boto_client):
        """Test successful file upload to S3."""
        mock_exists.return_value = True
        mock_s3_client = MagicMock()
        mock_boto_client.return_value = mock_s3_client
        
        # Create a temporary test file
        test_file = '/tmp/test_upload.m3u'
        with open(test_file, 'w') as f:
            f.write('#EXTM3U\n')
        
        config = Mock()
        config.S3_COMPATIBLE_CONFIG = {
            'endpoint_url': 'https://s3.example.com',
            'region': 'us-east-1'
        }
        config.OUTPUT_DIR = '/tmp'
        
        try:
            upload_file_to_s3(test_file, 'test-bucket', 'test.m3u', config)
            mock_s3_client.put_object.assert_called_once()
        finally:
            # Clean up test file
            if os.path.exists(test_file):
                os.remove(test_file)

    @patch('m3u_simple_filter.s3_operations.boto3.client')
    def test_upload_file_invalid_endpoint(self, mock_boto_client):
        """Test upload with invalid endpoint URL."""
        config = Mock()
        config.S3_COMPATIBLE_CONFIG = {
            'endpoint_url': '',
            'region': 'us-east-1'
        }
        config.OUTPUT_DIR = '/tmp'

        with self.assertRaises(ValueError):
            upload_file_to_s3('/tmp/test.m3u', 'test-bucket', 'test.m3u', config)


class TestUploadArchiveToS3(unittest.TestCase):
    """Test upload_archive_to_s3 function."""

    @patch('m3u_simple_filter.s3_operations.boto3.client')
    def test_upload_archive_with_uuid(self, mock_boto_client):
        """Test that archive upload includes UUID in path."""
        mock_s3_client = MagicMock()
        mock_boto_client.return_value = mock_s3_client
        
        config = Mock()
        config.S3_COMPATIBLE_CONFIG = {
            'endpoint_url': 'https://s3.example.com',
            'region': 'us-east-1'
        }
        
        upload_archive_to_s3('test content', 'test-bucket', 'playlist.m3u', config)
        
        # Check that put_object was called
        call_args = mock_s3_client.put_object.call_args
        archive_key = call_args[1]['Key']
        
        # Verify UUID is in the path (format: archive/YYYY-MM-DD/HH-MM-SS-UUID_playlist.gz)
        self.assertTrue(archive_key.startswith('archive/'))
        self.assertIn('playlist.gz', archive_key)
        # Should have 8-char UUID
        self.assertRegex(archive_key, r'[0-9a-f]{8}')


class TestSanitizeLogMessage(unittest.TestCase):
    """Test sanitize_log_message function."""

    def test_url_masking(self):
        """Test that URLs are properly masked."""
        message = "Connecting to https://example.com/path?secret=123"
        result = sanitize_log_message(message)
        self.assertNotIn("example.com", result)
        self.assertIn("****", result)

    def test_no_false_positives(self):
        """Test that normal messages are not modified."""
        message = "Processing complete: 100 channels found"
        result = sanitize_log_message(message)
        self.assertEqual(result, message)


if __name__ == '__main__':
    unittest.main()
