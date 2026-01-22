"""
Additional test for the url-tvg replacement functionality in M3U processing.
"""

import unittest
from src.m3u_simple_filter.m3u_processor import filter_m3u_content


class TestUrlTvgReplacement(unittest.TestCase):
    """Test cases for url-tvg attribute replacement functionality."""

    def test_replace_existing_url_tvg_attribute(self):
        """Test replacing an existing url-tvg attribute with a custom URL."""
        content = """#EXTM3U url-tvg="http://old-epg.com/epg.xml"
#EXTINF:-1 tvg-id="channel1" group-title="Россия | Russia",Channel 1
http://example.com/1
#EXTINF:-1 tvg-id="channel2" group-title="News",Channel 2
http://example.com/2"""

        custom_epg_url = "https://test-bucket.storage.test-cloud.com/epg.xml.gz"
        result = filter_m3u_content(content, [], custom_epg_url=custom_epg_url)

        # Check that the new EPG URL is in the result
        self.assertIn(custom_epg_url, result)
        # Check that the old EPG URL is not in the result
        self.assertNotIn("http://old-epg.com/epg.xml", result)
        # Check that the header is still present
        self.assertTrue(result.startswith("#EXTM3U"))
        # Check that the channels are preserved
        self.assertIn("Channel 1", result)
        self.assertIn("Channel 2", result)

    def test_add_url_tvg_attribute_when_missing(self):
        """Test adding a url-tvg attribute when it doesn't exist."""
        content = """#EXTM3U
#EXTINF:-1 tvg-id="channel1" group-title="Россия | Russia",Channel 1
http://example.com/1
#EXTINF:-1 tvg-id="channel2" group-title="News",Channel 2
http://example.com/2"""

        custom_epg_url = "https://test-bucket.storage.test-cloud.com/epg.xml.gz"
        result = filter_m3u_content(content, [], custom_epg_url=custom_epg_url)

        # Check that the new EPG URL is in the result
        self.assertIn(custom_epg_url, result)
        # Check that the header is still present
        self.assertTrue(result.startswith("#EXTM3U"))
        # Check that the channels are preserved
        self.assertIn("Channel 1", result)
        self.assertIn("Channel 2", result)

    def test_no_url_tvg_modification_when_not_provided(self):
        """Test that url-tvg is not modified when custom URL is not provided."""
        content = """#EXTM3U url-tvg="http://old-epg.com/epg.xml"
#EXTINF:-1 tvg-id="channel1" group-title="Россия | Russia",Channel 1
http://example.com/1
#EXTINF:-1 tvg-id="channel2" group-title="News",Channel 2
http://example.com/2"""

        result = filter_m3u_content(content, [])

        # Check that the old EPG URL is preserved
        self.assertIn("http://old-epg.com/epg.xml", result)
        # Check that the channels are preserved
        self.assertIn("Channel 1", result)
        self.assertIn("Channel 2", result)

    def test_mixed_case_url_tvg_attribute(self):
        """Test replacing url-tvg attribute regardless of case."""
        content = """#EXTM3U URL-TVG="http://old-epg.com/epg.xml"
#EXTINF:-1 tvg-id="channel1" group-title="Россия | Russia",Channel 1
http://example.com/1"""

        custom_epg_url = "https://test-bucket.storage.test-cloud.com/epg.xml.gz"
        result = filter_m3u_content(content, [], custom_epg_url=custom_epg_url)

        # Check that the new EPG URL is in the result
        self.assertIn(custom_epg_url, result)
        # Check that the old EPG URL is not in the result
        self.assertNotIn("http://old-epg.com/epg.xml", result)


if __name__ == '__main__':
    unittest.main()