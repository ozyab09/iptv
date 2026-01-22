#!/usr/bin/env python3
"""
Entry point for the Simple M3U Playlist Filter.

This script provides an entry point to run the M3U filtering application.
"""

import sys
import os

# Add the src directory to the Python path to enable relative imports
current_dir = os.path.dirname(__file__)
sys.path.insert(0, current_dir)

from m3u_simple_filter.main import main


if __name__ == "__main__":
    sys.exit(main())