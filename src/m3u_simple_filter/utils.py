"""
Utility module for common helper functions.

This module contains utility functions for sanitizing sensitive data from logs
and other common operations.
"""

import os
import re
import time
import logging
from typing import Dict, List, Callable, Any
from functools import wraps


logger = logging.getLogger(__name__)


def retry(max_attempts: int = 3, delay: float = 1.0, backoff: float = 2.0, exceptions: tuple = (Exception,)):
    """
    Retry decorator with exponential backoff.

    Args:
        max_attempts: Maximum number of retry attempts (default: 3)
        delay: Initial delay between retries in seconds (default: 1.0)
        backoff: Multiplier for delay after each retry (default: 2.0)
        exceptions: Tuple of exception types to catch (default: all Exceptions)

    Returns:
        Decorated function with retry logic

    Example:
        @retry(max_attempts=3, delay=1.0, backoff=2.0)
        def unreliable_function():
            # This function will be retried up to 3 times on failure
            pass
    """
    def decorator(func: Callable) -> Callable:
        @wraps(func)
        def wrapper(*args: Any, **kwargs: Any) -> Any:
            current_delay = delay
            last_exception = None

            for attempt in range(1, max_attempts + 1):
                try:
                    return func(*args, **kwargs)
                except exceptions as e:
                    last_exception = e
                    if attempt < max_attempts:
                        logger.warning(
                            f"Attempt {attempt}/{max_attempts} failed for {func.__name__}: {e}. "
                            f"Retrying in {current_delay:.1f}s..."
                        )
                        time.sleep(current_delay)
                        current_delay *= backoff
                    else:
                        logger.error(
                            f"All {max_attempts} attempts failed for {func.__name__}: {e}"
                        )

            raise last_exception  # type: ignore[misc]
        return wrapper
    return decorator


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
    Sanitizes both the message template and any arguments to prevent
    sensitive data leakage through log messages.
    """

    def __init__(self, logger):
        """
        Initialize the sanitized logger wrapper.

        Args:
            logger: Python logger instance to wrap
        """
        self.logger = logger

    def _sanitize_message(self, msg, *args, **kwargs):
        """
        Sanitize message and arguments.

        First format the message with args, then sanitize the complete string.
        This ensures that sensitive data in args is also sanitized.

        Args:
            msg: Message template string
            *args: Message arguments
            **kwargs: Additional keyword arguments

        Returns:
            str: Sanitized message
        """
        # Format the message with args first
        try:
            if args:
                formatted_msg = str(msg) % tuple(str(arg) for arg in args)
            elif kwargs:
                formatted_msg = str(msg) % kwargs
            else:
                formatted_msg = str(msg)
        except (TypeError, ValueError):
            # If formatting fails, just use the original message
            formatted_msg = str(msg)

        # Sanitize the complete formatted message
        return sanitize_log_message(formatted_msg)

    def debug(self, msg, *args, **kwargs):
        sanitized_msg = self._sanitize_message(msg, *args, **kwargs)
        self.logger.debug(sanitized_msg, **kwargs)

    def info(self, msg, *args, **kwargs):
        sanitized_msg = self._sanitize_message(msg, *args, **kwargs)
        self.logger.info(sanitized_msg, **kwargs)

    def warning(self, msg, *args, **kwargs):
        sanitized_msg = self._sanitize_message(msg, *args, **kwargs)
        self.logger.warning(sanitized_msg, **kwargs)

    def error(self, msg, *args, **kwargs):
        sanitized_msg = self._sanitize_message(msg, *args, **kwargs)
        self.logger.error(sanitized_msg, **kwargs)

    def critical(self, msg, *args, **kwargs):
        sanitized_msg = self._sanitize_message(msg, *args, **kwargs)
        self.logger.critical(sanitized_msg, **kwargs)

    def exception(self, msg, *args, **kwargs):
        sanitized_msg = self._sanitize_message(msg, *args, **kwargs)
        self.logger.exception(sanitized_msg, **kwargs)

    def log(self, level, msg, *args, **kwargs):
        sanitized_msg = self._sanitize_message(msg, *args, **kwargs)
        self.logger.log(level, sanitized_msg, **kwargs)