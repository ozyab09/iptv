"""
Pytest configuration file for M3U filter tests.
"""

import sys
import os

# Add the project root and src directory to the Python path to enable imports
project_root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, project_root)
sys.path.insert(0, os.path.join(project_root, 'src'))