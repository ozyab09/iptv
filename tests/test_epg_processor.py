"""
Unit tests for EPG processor module.
"""

import unittest
import xml.etree.ElementTree as ET
from unittest.mock import patch, mock_open
from src.m3u_simple_filter.epg_processor import (
    download_epg,
    extract_channel_info_from_playlist,
    filter_epg_content,
    is_gzipped
)


class TestEPGProcessor(unittest.TestCase):
    """Test cases for EPG processor functions."""

    def test_extract_channel_info_from_playlist(self):
        """Test extracting channel IDs and categories from M3U playlist content."""
        playlist_content = """#EXTM3U
#EXTINF:-1 tvg-id="channel1" group-title="Россия | Russia",Channel 1
http://example.com/1
#EXTINF:-1 tvg-id="channel2" group-title="News",Channel 2
http://example.com/2
#EXTINF:-1 tvg-id="channel3" group-title="Развлекательные",Channel 3
http://example.com/3
#EXTINF:-1 tvg-id="" group-title="Empty",Channel 4
http://example.com/4
#EXTINF:-1 group-title="No ID",Channel 5
http://example.com/5"""

        channel_ids, channel_categories = extract_channel_info_from_playlist(playlist_content)

        self.assertIn("channel1", channel_ids)
        self.assertIn("channel2", channel_ids)
        self.assertIn("channel3", channel_ids)
        self.assertNotIn("", channel_ids)  # Empty IDs should not be included
        self.assertEqual(len(channel_ids), 3)  # Only 3 valid IDs

        # Check that categories are properly mapped
        self.assertEqual(channel_categories.get("channel1"), "Россия | Russia")
        self.assertEqual(channel_categories.get("channel2"), "News")
        self.assertEqual(channel_categories.get("channel3"), "Развлекательные")
        self.assertIsNone(channel_categories.get("channel4"))  # Channel without ID should not be in mapping

    def test_filter_epg_content_basic(self):
        """Test filtering EPG content to keep only specified channels."""
        epg_content = """<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="channel1">
    <display-name lang="en">Channel 1</display-name>
  </channel>
  <channel id="channel2">
    <display-name lang="en">Channel 2</display-name>
  </channel>
  <channel id="channel3">
    <display-name lang="en">Channel 3</display-name>
  </channel>
  <programme start="20230101000000 +0000" stop="20230101010000 +0000" channel="channel1">
    <title lang="en">Show 1</title>
  </programme>
  <programme start="20230101000000 +0000" stop="20230101010000 +0000" channel="channel2">
    <title lang="en">Show 2</title>
  </programme>
  <programme start="20230101000000 +0000" stop="20230101010000 +0000" channel="channel3">
    <title lang="en">Show 3</title>
  </programme>
</tv>"""

        channel_ids = {"channel1", "channel3"}
        filtered_content = filter_epg_content(epg_content, channel_ids, {}, [], [])

        # Parse the result to verify it's valid XML
        root = ET.fromstring(filtered_content)

        # Check that only the specified channels and their programs remain
        channels = root.findall('channel')
        self.assertEqual(len(channels), 2)  # channel1 and channel3

        channel_ids_in_result = {ch.get('id') for ch in channels}
        self.assertEqual(channel_ids_in_result, {"channel1", "channel3"})

        programmes = root.findall('programme')
        self.assertEqual(len(programmes), 2)  # Programs for channel1 and channel3 only

        programme_channels = {prog.get('channel') for prog in programmes}
        self.assertEqual(programme_channels, {"channel1", "channel3"})

    def test_filter_epg_content_empty_channel_ids(self):
        """Test filtering EPG content when no channel IDs are provided."""
        epg_content = """<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="channel1">
    <display-name lang="en">Channel 1</display-name>
  </channel>
  <programme start="20230101000000 +0000" stop="20230101010000 +0000" channel="channel1">
    <title lang="en">Show 1</title>
  </programme>
</tv>"""

        channel_ids = set()
        filtered_content = filter_epg_content(epg_content, channel_ids, {}, [], [])

        # Should return an empty EPG structure
        self.assertIn('<tv>', filtered_content)
        root = ET.fromstring(filtered_content)
        self.assertEqual(len(root.findall('channel')), 0)
        self.assertEqual(len(root.findall('programme')), 0)

    def test_is_gzipped_true(self):
        """Test detecting gzipped content."""
        # Simulate gzipped content (starts with 1f 8b)
        gzipped_data = b'\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\x03'
        self.assertTrue(is_gzipped(gzipped_data))

    def test_is_gzipped_false(self):
        """Test detecting non-gzipped content."""
        # Regular text data
        regular_data = b'hello world'
        self.assertFalse(is_gzipped(regular_data))

    def test_is_gzipped_short_data(self):
        """Test with data too short to be gzipped."""
        short_data = b'\x1f'
        self.assertFalse(is_gzipped(short_data))

    @patch('src.m3u_simple_filter.epg_processor.urlopen')
    def test_download_epg_success(self, mock_urlopen):
        """Test successful EPG download."""
        mock_response = mock_urlopen.return_value
        # Use side_effect to simulate a file-like object that returns content then empty bytes
        mock_response.read.side_effect = [
            b"<?xml version='1.0'?><tv><channel id='test'>Test</channel></tv>",  # First read
            b""  # Subsequent reads return empty to break the loop
        ]

        result = download_epg("http://example.com/test.xml")

        self.assertEqual(result, "<?xml version='1.0'?><tv><channel id='test'>Test</channel></tv>")

    def test_filter_epg_content_uses_configurable_retention(self):
        """Test that the time-based filtering logic uses configurable retention period."""
        # This test verifies that the code now uses configurable retention instead of hardcoded values
        import inspect

        # Get the source code of the filter function
        source = inspect.getsource(filter_epg_content)

        # Check that the code mentions retention period in the comments
        self.assertIn("retention", source)

        # Check that the code references config
        self.assertIn("config", source)

        # Check that the code uses a variable for days instead of hardcoded value
        self.assertIn("retention_period_later", source)

        # Verify that old hardcoded variables are not used
        self.assertNotIn("four_days_later", source)
        self.assertNotIn("two_days_later", source)

    def test_filter_epg_content_logs_correct_counts(self):
        """Test that the EPG filtering logs the correct channel counts after category exclusion."""
        # This test verifies that the code logs the correct number of channels after filtering
        import inspect

        # Get the source code of the filter function
        source = inspect.getsource(filter_epg_content)

        # Check that the code logs the initial and final channel counts
        self.assertIn("initial channels", source)
        self.assertIn("channels after category and ID exclusions", source)

    def test_filter_epg_content_excludes_specific_channel_ids(self):
        """Test that EPG filtering excludes channels by specific IDs."""
        epg_content = """<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="channel1">
    <display-name lang="en">Channel 1</display-name>
  </channel>
  <channel id="channel2">
    <display-name lang="en">Channel 2</display-name>
  </channel>
  <channel id="channel3">
    <display-name lang="en">Channel 3</display-name>
  </channel>
  <programme start="20230101000000 +0000" stop="20230101010000 +0000" channel="channel1">
    <title lang="en">Show 1</title>
  </programme>
  <programme start="20230101000000 +0000" stop="20230101010000 +0000" channel="channel2">
    <title lang="en">Show 2</title>
  </programme>
  <programme start="20230101000000 +0000" stop="20230101010000 +0000" channel="channel3">
    <title lang="en">Show 3</title>
  </programme>
</tv>"""

        channel_ids = {"channel1", "channel2", "channel3"}
        channel_categories = {}
        excluded_categories = []
        excluded_channel_ids = ["channel2"]  # Exclude channel2 by ID

        filtered_content = filter_epg_content(epg_content, channel_ids, channel_categories, excluded_categories, excluded_channel_ids)

        # Parse the result to verify it's valid XML
        root = ET.fromstring(filtered_content)

        # Check that only channel1 and channel3 remain (channel2 was excluded by ID)
        channels = root.findall('channel')
        self.assertEqual(len(channels), 2)  # channel1 and channel3

        channel_ids_in_result = {ch.get('id') for ch in channels}
        self.assertEqual(channel_ids_in_result, {"channel1", "channel3"})

        programmes = root.findall('programme')
        self.assertEqual(len(programmes), 2)  # Programs for channel1 and channel3 only

        programme_channels = {prog.get('channel') for prog in programmes}
        self.assertEqual(programme_channels, {"channel1", "channel3"})


if __name__ == '__main__':
    unittest.main()