"""
Test for M3U filtering with simple playlists (no EXTINF lines).
"""

import unittest
from src.m3u_simple_filter.m3u_processor import filter_m3u_content


class TestSimplePlaylistFiltering(unittest.TestCase):
    """Test cases for filtering simple M3U playlists without EXTINF lines."""

    def test_filter_simple_playlist_with_no_extinf_lines(self):
        """Test filtering a simple playlist that has only URLs without EXTINF lines."""
        content = """#EXTM3U url-tvg="http://example.com/epg.xml"
http://example.com/5.m3u8
http://example.com/6.m3u8
http://example.com/7.m3u8
http://example.com/8.m3u8"""

        # Filter with no categories specified (should keep all entries)
        result = filter_m3u_content(content, categories_to_keep=[], custom_epg_url="https://test-bucket.storage.test-cloud.com/epg.xml.gz")

        # Check that the header is updated with the custom EPG URL
        self.assertIn('https://test-bucket.storage.test-cloud.com/epg.xml.gz', result)

        # Check that all URLs are preserved
        self.assertIn('http://example.com/5.m3u8', result)
        self.assertIn('http://example.com/6.m3u8', result)
        self.assertIn('http://example.com/7.m3u8', result)
        self.assertIn('http://example.com/8.m3u8', result)

    def test_filter_simple_playlist_with_custom_epg_url(self):
        """Test that custom EPG URL is properly added to simple playlists."""
        content = """#EXTM3U
http://example.com/5.m3u8
http://example.com/6.m3u8"""

        custom_epg_url = "https://test-bucket.storage.test-cloud.com/epg.xml.gz"
        result = filter_m3u_content(content, categories_to_keep=[], custom_epg_url=custom_epg_url)

        # Check that the custom EPG URL is in the header
        self.assertIn(custom_epg_url, result)

        # Check that all URLs are preserved
        self.assertIn('http://example.com/5.m3u8', result)
        self.assertIn('http://example.com/6.m3u8', result)


if __name__ == '__main__':
    unittest.main()