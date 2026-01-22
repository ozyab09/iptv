"""
Unit tests for main application module.
"""

import unittest
import os
import tempfile
from unittest.mock import patch, MagicMock
from src.m3u_simple_filter.main import main, save_filtered_m3u_locally


class TestMain(unittest.TestCase):
    """Test cases for main application functions."""

    def setUp(self):
        """Set up test fixtures before each test method."""
        # Store original environment variables
        self.original_env = {
            'DRY_RUN': os.environ.get('DRY_RUN'),
            'AWS_ACCESS_KEY_ID': os.environ.get('AWS_ACCESS_KEY_ID'),
            'AWS_SECRET_ACCESS_KEY': os.environ.get('AWS_SECRET_ACCESS_KEY'),
            'S3_BUCKET_NAME': os.environ.get('S3_BUCKET_NAME'),
        }

    def tearDown(self):
        """Clean up after each test method."""
        # Restore original environment variables
        for key, value in self.original_env.items():
            if value is not None:
                os.environ[key] = value
            elif key in os.environ:
                del os.environ[key]

    def test_save_filtered_m3u_locally(self):
        """Test saving M3U content to local file."""
        content = "#EXTM3U\n#EXTINF:-1,Test Channel\nhttp://example.com"
        
        with tempfile.NamedTemporaryFile(mode='w', delete=False, suffix='.m3u') as temp_file:
            temp_filename = temp_file.name
        
        try:
            save_filtered_m3u_locally(content, temp_filename)
            
            with open(temp_filename, 'r', encoding='utf-8') as f:
                saved_content = f.read()
            
            self.assertEqual(saved_content, content)
        finally:
            os.unlink(temp_filename)

    @patch('src.m3u_simple_filter.main.download_m3u')
    @patch('src.m3u_simple_filter.main.download_epg')
    @patch('src.m3u_simple_filter.main.filter_m3u_content')
    @patch('src.m3u_simple_filter.main.upload_to_s3')
    def test_main_dry_run_mode(self, mock_upload, mock_filter, mock_download_epg, mock_download):
        """Test main function in dry-run mode."""
        # Set dry-run mode
        os.environ['DRY_RUN'] = 'true'

        # Mock return values
        mock_download.return_value = "#EXTM3U\n#EXTINF:-1,Test Channel\nhttp://example.com"
        mock_download_epg.return_value = "<?xml version='1.0'?><tv></tv>"
        mock_filter.return_value = "#EXTM3U\n#EXTINF:-1,Filtered Channel\nhttp://example.com"

        result = main()

        # Verify the function executed successfully
        self.assertEqual(result, 0)

        # Verify download and filter were called
        mock_download.assert_called_once()
        mock_download_epg.assert_called_once()
        mock_filter.assert_called_once()

        # Verify upload was NOT called in dry-run mode
        mock_upload.assert_not_called()

    @patch('src.m3u_simple_filter.main.download_m3u')
    @patch('src.m3u_simple_filter.main.download_epg')
    @patch('src.m3u_simple_filter.main.filter_m3u_content')
    @patch('src.m3u_simple_filter.main.upload_to_s3')
    @patch('src.m3u_simple_filter.main.upload_file_to_s3')
    def test_main_normal_mode(self, mock_upload_file, mock_upload, mock_filter, mock_download_epg, mock_download):
        """Test main function in normal mode."""
        # Ensure dry-run mode is disabled
        if 'DRY_RUN' in os.environ:
            del os.environ['DRY_RUN']

        # Set required environment variables
        os.environ['AWS_ACCESS_KEY_ID'] = 'test_access_key'
        os.environ['AWS_SECRET_ACCESS_KEY'] = 'test_secret_key'
        os.environ['S3_BUCKET_NAME'] = 'test-bucket'

        # Mock return values
        mock_download.return_value = "#EXTM3U\n#EXTINF:-1,Test Channel\nhttp://example.com"
        mock_download_epg.return_value = "<?xml version='1.0'?><tv><channel id='test'><display-name>Test</display-name></channel></tv>"
        mock_filter.return_value = "#EXTM3U\n#EXTINF:-1,Filtered Channel\nhttp://example.com"

        result = main()

        # Verify the function executed successfully
        self.assertEqual(result, 0)

        # Verify download, filter, and upload were called
        mock_download.assert_called_once()
        mock_download_epg.assert_called_once()
        mock_filter.assert_called_once()
        # upload_to_s3 should be called twice (for filtered playlist and all categories playlist)
        self.assertEqual(mock_upload.call_count, 2)
        # upload_file_to_s3 should be called once (for filtered EPG)
        self.assertEqual(mock_upload_file.call_count, 1)

    @patch('src.m3u_simple_filter.main.download_m3u')
    def test_main_download_failure(self, mock_download):
        """Test main function when download fails."""
        # Ensure dry-run mode is disabled
        if 'DRY_RUN' in os.environ:
            del os.environ['DRY_RUN']
        
        # Mock download to raise an exception
        mock_download.side_effect = Exception("Download failed")
        
        result = main()
        
        # Verify the function returns error code
        self.assertEqual(result, 1)


if __name__ == '__main__':
    unittest.main()