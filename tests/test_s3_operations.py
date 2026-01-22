"""
Unit tests for S3 operations module.
"""

import unittest
import os
from unittest.mock import patch, MagicMock
from src.m3u_simple_filter.s3_operations import upload_to_s3


class TestS3Operations(unittest.TestCase):
    """Test cases for S3 operations functions."""

    def setUp(self):
        """Set up test fixtures before each test method."""
        # Store original environment variables
        self.original_env = {
            'AWS_ACCESS_KEY_ID': os.environ.get('AWS_ACCESS_KEY_ID'),
            'AWS_SECRET_ACCESS_KEY': os.environ.get('AWS_SECRET_ACCESS_KEY'),
        }

    def tearDown(self):
        """Clean up after each test method."""
        # Restore original environment variables
        for key, value in self.original_env.items():
            if value is not None:
                os.environ[key] = value
            elif key in os.environ:
                del os.environ[key]

    @patch('src.m3u_simple_filter.s3_operations.boto3.client')
    def test_upload_to_s3_success(self, mock_boto_client):

        """Test successful S3 upload."""
        # Set up environment variables
        os.environ['AWS_ACCESS_KEY_ID'] = 'test_access_key'
        os.environ['AWS_SECRET_ACCESS_KEY'] = 'test_secret_key'
        
        # Mock S3 client
        mock_s3_client = MagicMock()
        mock_boto_client.return_value = mock_s3_client

        # Create a mock config object
        mock_config = MagicMock()
        mock_config.S3_ENDPOINT_URL = 'https://s3.amazonaws.com'
        mock_config.S3_REGION = 'us-east-1'
        mock_config.S3_COMPATIBLE_CONFIG = {
            'endpoint_url': 'https://s3.amazonaws.com',
            'region': 'us-east-1'
        }

        content = "#EXTM3U\n#EXTINF:-1,Test Channel\nhttp://example.com"
        bucket_name = "test-bucket"
        object_key = "test-playlist.m3u"

        # Call the function
        upload_to_s3(content, bucket_name, object_key, mock_config)

        # Verify boto3.client was called with correct parameters
        mock_boto_client.assert_called_once_with(
            's3',
            endpoint_url='https://s3.amazonaws.com',
            aws_access_key_id='test_access_key',
            aws_secret_access_key='test_secret_key',
            region_name='us-east-1'
        )
        
        # Verify put_object was called with correct parameters
        mock_s3_client.put_object.assert_called_once()
        call_args = mock_s3_client.put_object.call_args
        self.assertEqual(call_args[1]['Bucket'], bucket_name)
        self.assertEqual(call_args[1]['Key'], object_key)
        self.assertEqual(call_args[1]['Body'], content.encode('utf-8'))
        self.assertEqual(call_args[1]['ContentType'], 'application/x-mpegurl')

    @patch('src.m3u_simple_filter.s3_operations.boto3.client')
    def test_upload_to_s3_client_error(self, mock_boto_client):
        """Test S3 upload with client error."""
        from botocore.exceptions import ClientError
        
        # Set up environment variables
        os.environ['AWS_ACCESS_KEY_ID'] = 'test_access_key'
        os.environ['AWS_SECRET_ACCESS_KEY'] = 'test_secret_key'
        
        # Mock S3 client to raise ClientError
        mock_s3_client = MagicMock()
        mock_s3_client.put_object.side_effect = ClientError(
            error_response={'Error': {'Code': 'TestError', 'Message': 'Test error message'}},
            operation_name='PutObject'
        )
        mock_boto_client.return_value = mock_s3_client

        # Create a mock config object
        mock_config = MagicMock()
        mock_config.S3_ENDPOINT_URL = 'https://s3.amazonaws.com'
        mock_config.S3_REGION = 'us-east-1'
        mock_config.S3_COMPATIBLE_CONFIG = {
            'endpoint_url': 'https://s3.amazonaws.com',
            'region': 'us-east-1'
        }

        content = "#EXTM3U\n#EXTINF:-1,Test Channel\nhttp://example.com"
        bucket_name = "test-bucket"
        object_key = "test-playlist.m3u"

        # Verify that the ClientError is raised
        with self.assertRaises(ClientError):
            upload_to_s3(content, bucket_name, object_key, mock_config)

    @patch('src.m3u_simple_filter.s3_operations.boto3.client')
    def test_upload_to_s3_general_error(self, mock_boto_client):
        """Test S3 upload with general error."""
        # Set up environment variables
        os.environ['AWS_ACCESS_KEY_ID'] = 'test_access_key'
        os.environ['AWS_SECRET_ACCESS_KEY'] = 'test_secret_key'
        
        # Mock S3 client to raise a general exception
        mock_s3_client = MagicMock()
        mock_s3_client.put_object.side_effect = Exception("General error")
        mock_boto_client.return_value = mock_s3_client
        
        # Create a mock config object
        mock_config = MagicMock()
        mock_config.S3_ENDPOINT_URL = 'https://s3.amazonaws.com'
        mock_config.S3_REGION = 'us-east-1'
        mock_config.S3_COMPATIBLE_CONFIG = {
            'endpoint_url': 'https://s3.amazonaws.com',
            'region': 'us-east-1'
        }

        content = "#EXTM3U\n#EXTINF:-1,Test Channel\nhttp://example.com"
        bucket_name = "test-bucket"
        object_key = "test-playlist.m3u"

        # Verify that the general exception is raised
        with self.assertRaises(Exception):
            upload_to_s3(content, bucket_name, object_key, mock_config)


if __name__ == '__main__':
    unittest.main()