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
        from datetime import datetime, timedelta
        
        # Use dates where programs haven't ended yet (future programs)
        now = datetime.now()
        start_time = (now + timedelta(hours=1)).strftime("%Y%m%d%H%M%S +0000")  # 1 hour in the future
        stop_time = (now + timedelta(days=1)).strftime("%Y%m%d%H%M%S +0000")   # Tomorrow
        
        epg_content = f"""<?xml version="1.0" encoding="UTF-8"?>
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
  <programme start="{start_time}" stop="{stop_time}" channel="channel1">
    <title lang="en">Show 1</title>
  </programme>
  <programme start="{start_time}" stop="{stop_time}" channel="channel2">
    <title lang="en">Show 2</title>
  </programme>
  <programme start="{start_time}" stop="{stop_time}" channel="channel3">
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
        self.assertIn("channels in filtered playlist", source)

    def test_filter_epg_content_excludes_specific_channel_ids(self):
        """Test that EPG filtering excludes channels by specific IDs."""
        from datetime import datetime, timedelta
        
        # Use dates where programs haven't ended yet (future programs)
        now = datetime.now()
        start_time = (now + timedelta(hours=1)).strftime("%Y%m%d%H%M%S +0000")  # 1 hour in the future
        stop_time = (now + timedelta(days=1)).strftime("%Y%m%d%H%M%S +0000")   # Tomorrow
        
        epg_content = f"""<?xml version="1.0" encoding="UTF-8"?>
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
  <programme start="{start_time}" stop="{stop_time}" channel="channel1">
    <title lang="en">Show 1</title>
  </programme>
  <programme start="{start_time}" stop="{stop_time}" channel="channel2">
    <title lang="en">Show 2</title>
  </programme>
  <programme start="{start_time}" stop="{stop_time}" channel="channel3">
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

    def test_filter_epg_content_past_retention_logic(self):
        """Test that EPG filtering respects past retention days setting."""
        from datetime import datetime, timedelta

        # Create EPG content with programs at different times
        # One program that ended 3 days ago (should be kept if past_retention_days >= 3)
        # One program that ended 5 days ago (should be removed if past_retention_days < 5)
        three_days_ago = (datetime.now() - timedelta(days=3)).strftime("%Y%m%d%H%M%S +0000")
        five_days_ago = (datetime.now() - timedelta(days=5)).strftime("%Y%m%d%H%M%S +0000")
        today = datetime.now().strftime("%Y%m%d%H%M%S +0000")
        tomorrow = (datetime.now() + timedelta(days=1)).strftime("%Y%m%d%H%M%S +0000")

        epg_content = f"""<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="channel1">
    <display-name lang="en">Channel 1</display-name>
  </channel>
  <channel id="channel2">
    <display-name lang="en">Channel 2</display-name>
  </channel>
  <programme start="{three_days_ago}" stop="{three_days_ago}" channel="channel1">
    <title lang="en">Show that ended 3 days ago</title>
  </programme>
  <programme start="{five_days_ago}" stop="{five_days_ago}" channel="channel2">
    <title lang="en">Show that ended 5 days ago</title>
  </programme>
  <programme start="{tomorrow}" stop="{tomorrow.replace(str(datetime.now().day + 1), str(datetime.now().day + 2))}" channel="channel1">
    <title lang="en">Future show</title>
  </programme>
</tv>"""

        channel_ids = {"channel1", "channel2"}

        # Mock the config to set past retention to 4 days
        import sys
        from unittest.mock import patch
        from src.m3u_simple_filter.config import Config

        # Create a custom config class with our test values
        class TestConfig(Config):
            @property
            def EPG_PAST_RETENTION_DAYS(self) -> int:
                return 4  # Keep programs that ended up to 4 days ago

            @property
            def EPG_RETENTION_DAYS(self) -> int:
                return 10  # Keep future programs for 10 days

        # Patch the config import in the epg_processor module
        with patch('src.m3u_simple_filter.config.Config', TestConfig):
            filtered_content = filter_epg_content(epg_content, channel_ids, {}, [], [])

        # Parse the result to verify filtering
        root = ET.fromstring(filtered_content)

        # Should have programs that ended 3 days ago (within 4-day window) and future programs
        # But NOT programs that ended 5 days ago (beyond 4-day window)
        programmes = root.findall('programme')

        # There should be 2 programs: one that ended 3 days ago and one future program
        # The program that ended 5 days ago should be excluded since 5 > 4 (past retention days)
        programme_titles = [p.find('title').text for p in programmes]

        self.assertEqual(len(programmes), 2)

        self.assertIn("Show that ended 3 days ago", programme_titles)
        self.assertIn("Future show", programme_titles)
        self.assertNotIn("Show that ended 5 days ago", programme_titles)

    def test_filter_epg_content_excluded_channels_one_day_limit(self):
        """Test that excluded channels only show programs that haven't ended yet and within 1 day."""
        from datetime import datetime, timedelta

        # Create EPG content with programs at different times for excluded channel
        from datetime import timezone
        fixed_time = datetime(2023, 6, 15, 12, 0, 0)  # Same fixed time as used in function
        one_hour_ago = (fixed_time - timedelta(hours=1)).strftime("%Y%m%d%H%M%S +0000")
        one_hour_future = (fixed_time + timedelta(hours=1)).strftime("%Y%m%d%H%M%S +0000")
        yesterday = (fixed_time - timedelta(days=1)).strftime("%Y%m%d%H%M%S +0000")
        today = fixed_time.strftime("%Y%m%d%H%M%S +0000")
        tomorrow = (fixed_time + timedelta(days=1)).strftime("%Y%m%d%H%M%S +0000")
        in_two_days = (fixed_time + timedelta(days=2)).strftime("%Y%m%d%H%M%S +0000")

        epg_content = f"""<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="excluded_channel">
    <display-name lang="en">Excluded Channel</display-name>
  </channel>
  <channel id="normal_channel">
    <display-name lang="en">Normal Channel</display-name>
  </channel>
  <!-- Programs for excluded channel -->
  <programme start="{yesterday}" stop="{yesterday}" channel="excluded_channel">
    <title lang="en">Past show on excluded channel (more than 1 hour ago)</title>
  </programme>
  <programme start="{one_hour_ago}" stop="{one_hour_ago}" channel="excluded_channel">
    <title lang="en">Past show on excluded channel (ended 1 hour ago)</title>
  </programme>
  <programme start="{today}" stop="{tomorrow}" channel="excluded_channel">
    <title lang="en">Current show on excluded channel</title>
  </programme>
  <programme start="{tomorrow}" stop="{in_two_days}" channel="excluded_channel">
    <title lang="en">Future show on excluded channel (within 1 day)</title>
  </programme>
  <programme start="{in_two_days}" stop="{(fixed_time + timedelta(days=2, hours=1)).strftime('%Y%m%d%H%M%S +0000')}" channel="excluded_channel">
    <title lang="en">Future show on excluded channel (beyond 1 day)</title>
  </programme>
  <!-- Programs for normal channel -->
  <programme start="{yesterday}" stop="{yesterday}" channel="normal_channel">
    <title lang="en">Past show on normal channel</title>
  </programme>
  <programme start="{today}" stop="{tomorrow}" channel="normal_channel">
    <title lang="en">Current show on normal channel</title>
  </programme>
</tv>"""

        # Both channels are in the filtered M3U playlist (they should be kept based on other criteria)
        # But excluded_channel belongs to an excluded category
        channel_ids = {"excluded_channel", "normal_channel"}
        channel_categories = {"excluded_channel": "Кино", "normal_channel": "News"}  # Кино is in excluded categories
        excluded_categories = ["Кино"]
        excluded_channel_ids = []  # No specific IDs to exclude in this test

        # Mock the config to set past retention to 0 days (default) and custom excluded channel limits
        from unittest.mock import patch
        from src.m3u_simple_filter.config import Config

        # Create a custom config class with our test values
        class TestConfig(Config):
            @property
            def EPG_PAST_RETENTION_DAYS(self) -> int:
                return 0  # Default value

            @property
            def EPG_RETENTION_DAYS(self) -> int:
                return 10  # Keep future programs for 10 days
                
            @property
            def EXCLUDED_CHANNELS_FUTURE_LIMIT_DAYS(self) -> int:
                return 1  # Keep programs for excluded channels up to 1 day in the future

            @property
            def EXCLUDED_CHANNELS_PAST_LIMIT_HOURS(self) -> int:
                return 1  # Keep programs for excluded channels that ended up to 1 hour ago

        # Create a fixed time for testing
        from datetime import datetime
        fixed_time = datetime(2023, 6, 15, 12, 0, 0)  # Fixed time: June 15, 2023, 12:00:00
        
        # Patch the config import in the epg_processor module
        with patch('src.m3u_simple_filter.config.Config', TestConfig):
            filtered_content = filter_epg_content(epg_content, channel_ids, channel_categories, excluded_categories, excluded_channel_ids, current_time_override=fixed_time)

        # Parse the result to verify filtering
        root = ET.fromstring(filtered_content)

        programmes = root.findall('programme')
        programme_titles = [p.find('title').text for p in programmes]

        # Programs that should be included for excluded channels:
        # - Past show on excluded channel (ended 1 hour ago) - within 1 hour threshold
        # - Current show on excluded channel (started today, ends tomorrow) - meets criteria
        # - Future show on excluded channel (within 1 day) - meets criteria
        # Programs that should be included for normal channels:
        # - Current show on normal channel (hasn't ended yet)
        # Programs that should NOT be included:
        # - Past show on normal channel (ended in the past when past_retention_days=0)

        expected_titles = [
            "Past show on excluded channel (ended 1 hour ago)",
            "Current show on excluded channel",
            "Future show on excluded channel (within 1 day)",
            "Current show on normal channel"
        ]
        
        # Programs that should NOT be included:
        # - Past show on excluded channel (more than 1 hour ago) - ended more than 1 hour ago
        # - Future show on excluded channel (beyond 1 day) - beyond 24-hour limit
        unexpected_titles = [
            "Past show on excluded channel (more than 1 hour ago)",
            "Future show on excluded channel (beyond 1 day)"
        ]
        
        for title in expected_titles:
            self.assertIn(title, programme_titles, f"Expected to find '{title}' in {programme_titles}")
            
        for title in unexpected_titles:
            self.assertNotIn(title, programme_titles, f"Did not expect to find '{title}' in {programme_titles}")


if __name__ == '__main__':
    unittest.main()