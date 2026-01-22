"""
Unit tests for M3U processor module.
"""

import unittest
from unittest.mock import patch, mock_open
from src.m3u_simple_filter.m3u_processor import (
    download_m3u,
    remove_orig_suffix,
    get_base_channel_name,
    filter_m3u_content,
    count_channels,
    apply_hd_preference,
    remove_duplicates_and_apply_hd_preference
)


class TestM3UProcessor(unittest.TestCase):
    """Test cases for M3U processor functions."""

    def test_remove_orig_suffix(self):
        """Test removing 'orig' suffix from channel names."""
        self.assertEqual(remove_orig_suffix("Channel Name orig"), "Channel Name")
        self.assertEqual(remove_orig_suffix("Channel Name ORIG"), "Channel Name")
        self.assertEqual(remove_orig_suffix("Channel Name Orig"), "Channel Name")
        self.assertEqual(remove_orig_suffix("Channel Name"), "Channel Name")
        self.assertEqual(remove_orig_suffix("Orig Channel"), "Orig Channel")
        self.assertEqual(remove_orig_suffix("Channel orig extra"), "Channel orig extra")

    def test_get_base_channel_name(self):
        """Test getting base channel name by removing 'orig' and 'hd' suffixes."""
        self.assertEqual(get_base_channel_name("Channel Name orig"), "Channel Name")
        self.assertEqual(get_base_channel_name("Channel Name hd"), "Channel Name")
        self.assertEqual(get_base_channel_name("Channel Name orig hd"), "Channel Name")
        self.assertEqual(get_base_channel_name("Channel Name HD"), "Channel Name")
        self.assertEqual(get_base_channel_name("Channel Name"), "Channel Name")

    def test_count_channels(self):
        """Test counting channels in M3U content."""
        content = """#EXTM3U
#EXTINF:-1,Channel 1
http://example.com/1
#EXTINF:-1,Channel 2
http://example.com/2
#EXTINF:-1,Channel 3 orig
http://example.com/3"""
        self.assertEqual(count_channels(content), 3)

    def test_apply_hd_preference_with_hd_and_non_hd(self):
        """Test HD preference rule: keep only HD when both versions exist."""
        content = """#EXTM3U
#EXTINF:-1,Channel 1
http://example.com/1
#EXTINF:-1,Channel 2
http://example.com/2
#EXTINF:-1,Channel 2 HD
http://example.com/2hd"""
        
        result = apply_hd_preference(content)
        # Should only contain the HD version of Channel 2
        self.assertIn("#EXTINF:-1,Channel 2 HD", result)  # HD version should be kept
        self.assertNotIn("#EXTINF:-1,Channel 2\n", result)  # Non-HD version should be removed
        self.assertIn("Channel 1", result)  # Non-duplicate channel should remain

    def test_apply_hd_preference_without_hd(self):
        """Test HD preference rule: keep all when no HD versions exist."""
        content = """#EXTM3U
#EXTINF:-1,Channel 1
http://example.com/1
#EXTINF:-1,Channel 2
http://example.com/2"""

        result = apply_hd_preference(content)
        self.assertIn("Channel 1", result)
        self.assertIn("Channel 2", result)

    def test_remove_duplicates_and_apply_hd_preference_with_tvg_id(self):
        """Test duplicate removal based on tvg-id and HD preference."""
        content = """#EXTM3U
#EXTINF:-1 tvg-id="711",Channel 1
http://example.com/1
#EXTINF:-1 tvg-id="711",Channel 1 HD
http://example.com/1hd
#EXTINF:-1 tvg-id="162",Channel 2
http://example.com/2
#EXTINF:-1 tvg-id="162",Channel 2
http://example.com/2duplicate"""

        result = remove_duplicates_and_apply_hd_preference(content)
        # Should keep only the HD version of Channel 1 and one version of Channel 2
        self.assertIn("Channel 1 HD", result)  # HD version should be kept
        self.assertNotIn("Channel 1\n", result)  # Non-HD version should be removed
        self.assertIn("Channel 2", result)  # Should have one version of Channel 2
        # Count occurrences of Channel 2 entries
        channel_2_count = result.count("Channel 2")
        self.assertEqual(channel_2_count, 1)  # Only one version should remain

    def test_remove_duplicates_with_tvg_rec_preference(self):
        """Test duplicate removal keeping the version with higher tvg-rec."""
        content = """#EXTM3U
#EXTINF:-1 tvg-id="711" tvg-rec="3",Channel 1
http://example.com/1low
#EXTINF:-1 tvg-id="711" tvg-rec="7",Channel 1
http://example.com/1high
#EXTINF:-1 tvg-id="162" tvg-rec="0",Channel 2
http://example.com/2low
#EXTINF:-1 tvg-id="162" tvg-rec="5",Channel 2
http://example.com/2high"""

        result = remove_duplicates_and_apply_hd_preference(content)
        # Should keep only the version with higher tvg-rec
        self.assertIn("Channel 1", result)
        self.assertIn("tvg-rec=\"7\"", result)  # Higher tvg-rec version should be kept
        self.assertNotIn("tvg-rec=\"3\"", result)  # Lower tvg-rec version should be removed
        self.assertIn("Channel 2", result)
        self.assertIn("tvg-rec=\"5\"", result)  # Higher tvg-rec version should be kept
        self.assertNotIn("tvg-rec=\"0\"", result)  # Lower tvg-rec version should be removed

    def test_filter_m3u_content_with_categories(self):
        """Test filtering M3U content based on categories."""
        content = """#EXTM3U
#EXTINF:-1 group-title="Россия | Russia",Channel 1
http://example.com/1
#EXTINF:-1 group-title="News",Channel 2
http://example.com/2
#EXTINF:-1 group-title="Развлекательные",Channel 3
http://example.com/3"""
        
        categories_to_keep = ["Россия | Russia", "Развлекательные"]
        result = filter_m3u_content(content, categories_to_keep)
        
        self.assertIn("Channel 1", result)  # Russia category should be kept
        self.assertNotIn("Channel 2", result)  # News category should be filtered out
        self.assertIn("Channel 3", result)  # Развлекательные category should be kept

    def test_filter_m3u_content_removes_orig_suffix(self):
        """Test that filtering also removes 'orig' suffix from channel names."""
        content = """#EXTM3U
#EXTINF:-1 group-title="Россия | Russia",Channel 1 orig
http://example.com/1
#EXTINF:-1 group-title="Развлекательные",Channel 2 orig
http://example.com/2"""
        
        categories_to_keep = ["Россия | Russia", "Развлекательные"]
        result = filter_m3u_content(content, categories_to_keep)
        
        self.assertIn("Channel 1", result)  # 'orig' suffix should be removed
        self.assertIn("Channel 2", result)  # 'orig' suffix should be removed
        self.assertNotIn("orig", result)  # No 'orig' suffix should remain

    def test_filter_m3u_content_excludes_regional_channels(self):
        """Test that regional channels matching the +x pattern are excluded."""
        content = """#EXTM3U
#EXTINF:-1 group-title="Россия | Russia",Channel 1
http://example.com/1
#EXTINF:-1 group-title="Россия | Russia",Channel +1 (Приволжье)
http://example.com/plus1
#EXTINF:-1 group-title="Россия | Russia",Channel +4 (Алтай)
http://example.com/plus4
#EXTINF:-1 group-title="Россия | Russia",Channel +2 (Москва)
http://example.com/plus2
#EXTINF:-1 group-title="Россия | Russia",Channel +3 (Краснодар)
http://example.com/plus3
#EXTINF:-1 group-title="Россия | Russia",Channel +5 HD
http://example.com/plus5hd
#EXTINF:-1 group-title="Россия | Russia",Channel +6
http://example.com/plus6
#EXTINF:-1 group-title="Россия | Russia",Channel +7 not regional
http://example.com/plus7
#EXTINF:-1 group-title="Россия | Russia",Channel HD 50
http://example.com/50
#EXTINF:-1 group-title="Россия | Russia",Channel 25
http://example.com/25
#EXTINF:-1 group-title="Россия | Russia",Normal Channel
http://example.com/normal"""

        categories_to_keep = ["Россия | Russia"]
        result = filter_m3u_content(content, categories_to_keep)

        self.assertIn("Channel 1", result)  # Regular channel should be kept
        self.assertNotIn("Channel +1 (Приволжье)", result)  # Regional channel should be excluded
        self.assertNotIn("Channel +4 (Алтай)", result)  # Regional channel should be excluded
        self.assertNotIn("Channel +2 (Москва)", result)  # Regional channel should be excluded
        self.assertNotIn("Channel +3 (Краснодар)", result)  # Regional channel should be excluded
        self.assertNotIn("Channel +5 HD", result)  # Regional channel should be excluded
        self.assertNotIn("Channel +6", result)  # Regional channel should be excluded
        self.assertIn("Channel +7 not regional", result)  # Non-regional channel should be kept (has + but not in the expected pattern)
        self.assertNotIn("Channel HD 50", result)  # Channel ending with numbers should be excluded
        self.assertNotIn("Channel 25", result)  # Channel ending with numbers should be excluded
        self.assertIn("Normal Channel", result)  # Normal channel should be kept

    def test_filter_m3u_content_excludes_channels_by_name(self):
        """Test that channels with names containing excluded patterns are filtered out."""
        content = """#EXTM3U
#EXTINF:-1 group-title="Россия | Russia",Fashion TV
http://example.com/fashion1
#EXTINF:-1 group-title="Россия | Russia",Russian Fashion
http://example.com/fashion2
#EXTINF:-1 group-title="Россия | Russia",Kids Fashion Channel
http://example.com/fashion3
#EXTINF:-1 group-title="Россия | Russia",News Channel
http://example.com/news
#EXTINF:-1 group-title="Россия | Russia",Sports Channel
http://example.com/sports"""

        categories_to_keep = ["Россия | Russia"]
        channel_names_to_exclude = ["Fashion"]
        result = filter_m3u_content(content, categories_to_keep, channel_names_to_exclude)

        # Channels with "Fashion" in the name should be excluded
        self.assertNotIn("Fashion TV", result)
        self.assertNotIn("Russian Fashion", result)
        self.assertNotIn("Kids Fashion Channel", result)

        # Channels without "Fashion" in the name should be kept
        self.assertIn("News Channel", result)
        self.assertIn("Sports Channel", result)

    def test_filter_m3u_content_case_insensitive_exclusion(self):
        """Test that channel name exclusion is case insensitive."""
        content = """#EXTM3U
#EXTINF:-1 group-title="Россия | Russia",FASHION TV
http://example.com/fashion1
#EXTINF:-1 group-title="Россия | Russia",fashion news
http://example.com/fashion2
#EXTINF:-1 group-title="Россия | Russia",FaShIoN Channel
http://example.com/fashion3
#EXTINF:-1 group-title="Россия | Russia",Regular Channel
http://example.com/regular"""

        categories_to_keep = ["Россия | Russia"]
        channel_names_to_exclude = ["Fashion"]
        result = filter_m3u_content(content, categories_to_keep, channel_names_to_exclude)

        # All variations of "Fashion" should be excluded
        self.assertNotIn("FASHION TV", result)
        self.assertNotIn("fashion news", result)
        self.assertNotIn("FaShIoN Channel", result)

        # Regular channel should be kept
        self.assertIn("Regular Channel", result)

    def test_filter_m3u_content_multiple_exclusions(self):
        """Test that multiple channel name patterns can be excluded."""
        content = """#EXTM3U
#EXTINF:-1 group-title="Россия | Russia",Fashion TV
http://example.com/fashion
#EXTINF:-1 group-title="Россия | Russia",Adult Channel
http://example.com/adult
#EXTINF:-1 group-title="Россия | Russia",Gambling Network
http://example.com/gambling
#EXTINF:-1 group-title="Россия | Russia",Regular Channel
http://example.com/regular"""

        categories_to_keep = ["Россия | Russia"]
        channel_names_to_exclude = ["Fashion", "Adult", "Gambling"]
        result = filter_m3u_content(content, categories_to_keep, channel_names_to_exclude)

        # All channels with excluded patterns should be filtered out
        self.assertNotIn("Fashion TV", result)
        self.assertNotIn("Adult Channel", result)
        self.assertNotIn("Gambling Network", result)

        # Regular channel should be kept
        self.assertIn("Regular Channel", result)

    @patch('src.m3u_simple_filter.m3u_processor.urlopen')
    def test_download_m3u_success(self, mock_urlopen):
        """Test successful M3U download."""
        mock_response = mock_urlopen.return_value
        # Use side_effect to simulate a file-like object that returns content then empty bytes
        mock_response.read.side_effect = [
            b"#EXTM3U\n#EXTINF:-1,Test Channel\nhttp://example.com",  # First read
            b""  # Subsequent reads return empty to break the loop
        ]

        result = download_m3u("http://example.com/test.m3u")

        self.assertEqual(result, "#EXTM3U\n#EXTINF:-1,Test Channel\nhttp://example.com")

    @patch('src.m3u_simple_filter.m3u_processor.urlopen')
    def test_download_m3u_failure(self, mock_urlopen):
        """Test M3U download failure."""
        from urllib.error import URLError
        mock_urlopen.side_effect = URLError("Network error")

        with self.assertRaises(URLError):
            download_m3u("http://example.com/test.m3u")


if __name__ == '__main__':
    unittest.main()