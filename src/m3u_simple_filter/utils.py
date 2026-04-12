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
    sanitized_message = message

    # Mask all HTTP/HTTPS URLs — domain and path are completely hidden
    url_pattern = r'https?://[^\s\'"<>]+'
    urls = re.findall(url_pattern, sanitized_message)
    for url in urls:
        sanitized_message = sanitized_message.replace(url, mask_url(url))

    # Mask AWS credentials (partial mask — show first/last chars)
    aws_key_pattern = r'(YCAJEu[A-Za-z0-9_\-]+)'
    for match in re.findall(aws_key_pattern, sanitized_message):
        masked = match[:4] + '****' + match[-4:] if len(match) > 8 else '****'
        sanitized_message = sanitized_message.replace(match, masked)

    aws_secret_pattern = r'(YCON[A-Za-z0-9_\-]+)'
    for match in re.findall(aws_secret_pattern, sanitized_message):
        masked = match[:4] + '****' + match[-4:] if len(match) > 8 else '****'
        sanitized_message = sanitized_message.replace(match, masked)

    return sanitized_message


def mask_url(url: str) -> str:
    """
    Completely mask a URL — domain and path are hidden.

    Examples:
        https://raw.githubusercontent.com/foo/bar -> https://****/****
        https://storage.yandexcloud.net/bucket    -> https://****/****
        http://ru.epg.one/epg2.xml.gz            -> http://****/****

    Args:
        url (str): Original URL

    Returns:
        str: Fully masked URL preserving only the scheme
    """
    try:
        from urllib.parse import urlparse
        parsed = urlparse(url)
        return f"{parsed.scheme}://****/****"
    except Exception:
        return "https://****/****"


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