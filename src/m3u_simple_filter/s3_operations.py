"""
S3 operations module.

This module handles uploading files to S3-compatible storage.
"""

import os
import logging
import time
import boto3
from typing import Any
from botocore.exceptions import ClientError

from .utils import SanitizedLogger


# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(levelname)s - %(message)s'
)
logger = SanitizedLogger(logging.getLogger(__name__))


def upload_to_s3(content: str, bucket_name: str, object_key: str, config: Any, content_type: str = 'application/x-mpegurl') -> None:
    """
    Upload content to S3-compatible storage

    Args:
        content (str): Content to upload
        bucket_name (str): S3 bucket name
        object_key (str): S3 object key
        config: Configuration object with S3 settings
        content_type (str): Content type for the uploaded object (default: 'application/x-mpegurl')
    """
    logger.info(f"Uploading to S3-compatible storage: s3://{bucket_name}/{object_key}")

    # Validate S3 endpoint URL before initializing client
    endpoint_url = config.S3_COMPATIBLE_CONFIG['endpoint_url']
    if not endpoint_url or not isinstance(endpoint_url, str) or not endpoint_url.startswith(('http://', 'https://')):
        raise ValueError(f"Invalid S3 endpoint URL: {endpoint_url}. Must be a valid HTTP/HTTPS URL.")

    # Initialize S3 client with endpoint from config
    # Use environment variables for credentials
    s3_client = boto3.client(
        's3',
        endpoint_url=endpoint_url,  # S3-compatible storage endpoint from config
        aws_access_key_id=os.environ.get('AWS_ACCESS_KEY_ID'),
        aws_secret_access_key=os.environ.get('AWS_SECRET_ACCESS_KEY'),
        region_name=config.S3_COMPATIBLE_CONFIG['region']  # S3-compatible storage region from config
    )

    try:
        # Upload the content
        s3_client.put_object(
            Bucket=bucket_name,
            Key=object_key,
            Body=content.encode('utf-8'),
            ContentType=content_type,
            # Add metadata for tracking
            Metadata={
                'uploaded-by': 'm3u-simple-filter-script',
                'upload-timestamp': str(int(time.time()))
            }
        )
        logger.info("Upload to S3-compatible storage completed successfully")
    except ClientError as e:
        logger.error(f"Error uploading to S3-compatible storage: {e}")
        raise
    except Exception as e:
        logger.error(f"Unexpected error uploading to S3-compatible storage: {e}")
        raise


def upload_file_to_s3(file_path: str, bucket_name: str, object_key: str, config: Any, content_type: str = 'application/x-mpegurl') -> None:
    """
    Upload a local file to S3-compatible storage

    Args:
        file_path (str): Path to the local file to upload
        bucket_name (str): S3 bucket name
        object_key (str): S3 object key
        config: Configuration object with S3 settings
        content_type (str): Content type for the uploaded object (default: 'application/x-mpegurl')
    """
    import os
    from .config import Config as ConfigClass

    # If config is available and file doesn't exist at the given path, try looking in output directory
    full_file_path = file_path
    if config and hasattr(config, 'OUTPUT_DIR'):
        output_file_path = os.path.join(config.OUTPUT_DIR, os.path.basename(file_path))
        if os.path.exists(output_file_path):
            full_file_path = output_file_path

    logger.info(f"Uploading file to S3-compatible storage: s3://{bucket_name}/{object_key} from {full_file_path}")

    # Validate S3 endpoint URL before initializing client
    endpoint_url = config.S3_COMPATIBLE_CONFIG['endpoint_url']
    if not endpoint_url or not isinstance(endpoint_url, str) or not endpoint_url.startswith(('http://', 'https://')):
        raise ValueError(f"Invalid S3 endpoint URL: {endpoint_url}. Must be a valid HTTP/HTTPS URL.")

    # Initialize S3 client with endpoint from config
    # Use environment variables for credentials
    s3_client = boto3.client(
        's3',
        endpoint_url=endpoint_url,  # S3-compatible storage endpoint from config
        aws_access_key_id=os.environ.get('AWS_ACCESS_KEY_ID'),
        aws_secret_access_key=os.environ.get('AWS_SECRET_ACCESS_KEY'),
        region_name=config.S3_COMPATIBLE_CONFIG['region']  # S3-compatible storage region from config
    )

    try:
        # Read the file content
        with open(full_file_path, 'rb') as f:
            file_content = f.read()

        # Upload the file content
        s3_client.put_object(
            Bucket=bucket_name,
            Key=object_key,
            Body=file_content,
            ContentType=content_type,
            # Add metadata for tracking
            Metadata={
                'uploaded-by': 'm3u-simple-filter-script',
                'upload-timestamp': str(int(time.time())),
                'source-file': full_file_path
            }
        )
        logger.info("File upload to S3-compatible storage completed successfully")
    except ClientError as e:
        logger.error(f"Error uploading file to S3-compatible storage: {e}")
        raise
    except Exception as e:
        logger.error(f"Unexpected error uploading file to S3-compatible storage: {e}")
        raise