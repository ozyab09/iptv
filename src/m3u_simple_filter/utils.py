"""
Utility module for common helper functions.

This module contains utility functions for sanitizing sensitive data from logs
and other common operations.
"""

import os
import re
from typing import Dict, List


def sanitize_log_message(message: str) -> str:
    """
    Sanitize log messages by replacing sensitive data with masked values.

    Args:
        message (str): Original log message

    Returns:
        str: Sanitized log message with sensitive data masked
    """
    # Define default values that should not be considered sensitive
    default_values = {
        'M3U_SOURCE_URL': 'https://your-provider.com/playlist.m3u',
        'EPG_SOURCE_URL': 'https://your-epg-provider.com/epg.xml.gz',
        'S3_ENDPOINT_URL': 'https://s3.amazonaws.com',
        'S3_REGION': 'us-east-1',
        'S3_OBJECT_KEY': 'playlist.m3u',
        'S3_EPG_KEY': 'epg.xml.gz',
        'S3_BUCKET_NAME': 'your-bucket-name'
    }

    # Get sensitive values from environment variables (only if they differ from defaults)
    sensitive_values = []

    for var_name, default_val in default_values.items():
        env_val = os.getenv(var_name)
        # Only add to sensitive values if the environment variable is set AND differs from default
        if env_val and env_val != default_val:
            sensitive_values.append(env_val)

    # Sort by length (descending) to replace longer strings first
    sensitive_values.sort(key=len, reverse=True)

    sanitized_message = message

    # Replace sensitive values with masked versions
    for value in sensitive_values:
        if value:  # Only replace non-empty values
            # Mask the value - show first and last few characters
            if len(value) <= 8:
                masked_value = '*' * len(value)
            else:
                visible_chars = max(3, len(value) // 4)  # Show at least 3 chars
                masked_value = f"{value[:visible_chars]}{'*' * (len(value) - 2 * visible_chars)}{value[-visible_chars:]}"

            sanitized_message = sanitized_message.replace(value, masked_value)

    # Also mask potential URLs that might contain sensitive information
    # But only if they are not default values
    default_urls = [
        'https://your-provider.com/playlist.m3u',
        'https://your-epg-provider.com/epg.xml.gz',
        'https://s3.amazonaws.com',
        'https://your-bucket-name.s3.amazonaws.com/playlist.m3u',
        'us-east-1',
        'playlist.m3u',
        'epg.xml.gz'
    ]

    url_pattern = r'https?://[^\s\'"<>]+'
    # Find all URLs in the message
    urls = re.findall(url_pattern, message)

    for url in urls:
        # Only mask URLs that are not default values
        if url not in default_urls:
            sanitized_message = sanitized_message.replace(url, mask_url(url))

    return sanitized_message


def mask_url(url: str) -> str:
    """
    Mask a URL by showing only the domain and masking the rest.
    
    Args:
        url (str): Original URL
        
    Returns:
        str: Masked URL
    """
    try:
        from urllib.parse import urlparse, urlunparse
        parsed = urlparse(url)
        
        # Mask the path, query, and fragment parts
        masked_path = mask_sensitive_path(parsed.path)
        masked_query = mask_sensitive_query(parsed.query)
        masked_fragment = '*' * min(len(parsed.fragment), 10) if parsed.fragment else ''
        
        # Reconstruct the URL with masked parts
        masked_url = urlunparse((
            parsed.scheme,
            parsed.netloc,
            masked_path,
            '',  # params
            masked_query,
            masked_fragment
        ))
        
        return masked_url
    except Exception:
        # If parsing fails, return a generic masked version
        return f"{url.split('://')[0]}://***.***"


def mask_sensitive_path(path: str) -> str:
    """
    Mask sensitive parts of a URL path.
    
    Args:
        path (str): URL path
        
    Returns:
        str: Masked path
    """
    if not path:
        return path
    
    parts = path.split('/')
    masked_parts = []
    
    for part in parts:
        if not part:
            masked_parts.append('')
        elif is_potentially_sensitive(part):
            # Mask the part - show first and last few characters
            if len(part) <= 8:
                masked_part = '*' * len(part)
            else:
                visible_chars = min(3, len(part) // 4) or 1
                masked_part = f"{part[:visible_chars]}{'*' * (len(part) - 2 * visible_chars)}{part[-visible_chars:]}"
            masked_parts.append(masked_part)
        else:
            masked_parts.append(part)
    
    return '/'.join(masked_parts)


def mask_sensitive_query(query: str) -> str:
    """
    Mask sensitive parts of a URL query string.
    
    Args:
        query (str): Query string
        
    Returns:
        str: Masked query string
    """
    if not query:
        return query
    
    pairs = query.split('&')
    masked_pairs = []
    
    for pair in pairs:
        if '=' in pair:
            key, value = pair.split('=', 1)
            if is_potentially_sensitive(key) or is_potentially_sensitive_param(key):
                masked_pairs.append(f"{key}={'*' * min(len(value), 20)}")
            else:
                masked_pairs.append(pair)
        else:
            masked_pairs.append(pair)
    
    return '&'.join(masked_pairs)


def is_potentially_sensitive(text: str) -> bool:
    """
    Check if a text string is potentially sensitive.
    
    Args:
        text (str): Text to check
        
    Returns:
        bool: True if text is potentially sensitive
    """
    # Common patterns that might contain sensitive information
    sensitive_patterns = [
        r'.*[Ss]ecret.*',
        r'.*[Tt]oken.*',
        r'.*[Kk]ey.*',
        r'.*[Cc]redential.*',
        r'.*[Pp]assword.*',
        r'.*[Cc]ode.*',
        r'.*[Aa]uth.*',
        r'.*[Ss]ession.*',
        r'^[A-Za-z0-9_-]{20,}$',  # Long alphanumeric strings (likely tokens/keys)
    ]
    
    for pattern in sensitive_patterns:
        if re.match(pattern, text):
            return True
    
    return False


def is_potentially_sensitive_param(param_name: str) -> bool:
    """
    Check if a query parameter name is potentially sensitive.
    
    Args:
        param_name (str): Parameter name to check
        
    Returns:
        bool: True if parameter is potentially sensitive
    """
    sensitive_params = [
        'token', 'key', 'secret', 'password', 'auth', 'session', 'code', 'access_token',
        'refresh_token', 'api_key', 'client_secret', 'credential', 'signature'
    ]
    
    return param_name.lower() in sensitive_params


class SanitizedLogger:
    """
    A wrapper around a logger that sanitizes messages before logging.
    """
    
    def __init__(self, logger):
        self.logger = logger
    
    def debug(self, msg, *args, **kwargs):
        sanitized_msg = sanitize_log_message(str(msg))
        self.logger.debug(sanitized_msg, *args, **kwargs)
    
    def info(self, msg, *args, **kwargs):
        sanitized_msg = sanitize_log_message(str(msg))
        self.logger.info(sanitized_msg, *args, **kwargs)
    
    def warning(self, msg, *args, **kwargs):
        sanitized_msg = sanitize_log_message(str(msg))
        self.logger.warning(sanitized_msg, *args, **kwargs)
    
    def error(self, msg, *args, **kwargs):
        sanitized_msg = sanitize_log_message(str(msg))
        self.logger.error(sanitized_msg, *args, **kwargs)
    
    def critical(self, msg, *args, **kwargs):
        sanitized_msg = sanitize_log_message(str(msg))
        self.logger.critical(sanitized_msg, *args, **kwargs)
    
    def exception(self, msg, *args, **kwargs):
        sanitized_msg = sanitize_log_message(str(msg))
        self.logger.exception(sanitized_msg, *args, **kwargs)
    
    def log(self, level, msg, *args, **kwargs):
        sanitized_msg = sanitize_log_message(str(msg))
        self.logger.log(level, sanitized_msg, *args, **kwargs)