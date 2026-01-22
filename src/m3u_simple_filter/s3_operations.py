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

    # Initialize S3 client with endpoint from config
    # Use environment variables for credentials
    s3_client = boto3.client(
        's3',
        endpoint_url=config.S3_COMPATIBLE_CONFIG['endpoint_url'],  # S3-compatible storage endpoint from config
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
    logger.info(f"Uploading file to S3-compatible storage: s3://{bucket_name}/{object_key} from {file_path}")

    # Initialize S3 client with endpoint from config
    # Use environment variables for credentials
    s3_client = boto3.client(
        's3',
        endpoint_url=config.S3_COMPATIBLE_CONFIG['endpoint_url'],  # S3-compatible storage endpoint from config
        aws_access_key_id=os.environ.get('AWS_ACCESS_KEY_ID'),
        aws_secret_access_key=os.environ.get('AWS_SECRET_ACCESS_KEY'),
        region_name=config.S3_COMPATIBLE_CONFIG['region']  # S3-compatible storage region from config
    )

    try:
        # Read the file content
        with open(file_path, 'rb') as f:
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
                'source-file': file_path
            }
        )
        logger.info("File upload to S3-compatible storage completed successfully")
    except ClientError as e:
        logger.error(f"Error uploading file to S3-compatible storage: {e}")
        raise
    except Exception as e:
        logger.error(f"Unexpected error uploading file to S3-compatible storage: {e}")
        raise