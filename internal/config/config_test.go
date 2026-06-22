package config

import (
	"os"
	"testing"
)

func TestEnvironmentVariableOverride(t *testing.T) {
	os.Setenv("M3U_SOURCE_URL", "https://test.com/playlist.m3u")
	os.Setenv("S3_BUCKET_NAME", "test-bucket")
	os.Setenv("S3_OBJECT_KEY", "test-playlist.m3u")
	os.Setenv("S3_ENDPOINT_URL", "https://test-storage.com")
	os.Setenv("S3_REGION", "test-region")
	defer func() {
		os.Unsetenv("M3U_SOURCE_URL")
		os.Unsetenv("S3_BUCKET_NAME")
		os.Unsetenv("S3_OBJECT_KEY")
		os.Unsetenv("S3_ENDPOINT_URL")
		os.Unsetenv("S3_REGION")
	}()

	cfg := New()
	if cfg.M3USourceURL() != "https://test.com/playlist.m3u" {
		t.Errorf("expected M3U_SOURCE_URL to be 'https://test.com/playlist.m3u', got '%s'", cfg.M3USourceURL())
	}
	if cfg.S3DefaultBucketName() != "test-bucket" {
		t.Errorf("expected S3DefaultBucketName to be 'test-bucket', got '%s'", cfg.S3DefaultBucketName())
	}
	if cfg.S3FilteredPlaylistKey() != "test-playlist.m3u" {
		t.Errorf("expected S3FilteredPlaylistKey to be 'test-playlist.m3u', got '%s'", cfg.S3FilteredPlaylistKey())
	}
	if cfg.S3EndpointURL() != "https://test-storage.com" {
		t.Errorf("expected S3EndpointURL to be 'https://test-storage.com', got '%s'", cfg.S3EndpointURL())
	}
	if cfg.S3Region() != "test-region" {
		t.Errorf("expected S3Region to be 'test-region', got '%s'", cfg.S3Region())
	}
}

func TestLocalPlaylistPaths(t *testing.T) {
	cfg := New()
	if cfg.LocalFilteredPlaylistPath() != "playlist.m3u" {
		t.Errorf("expected LocalFilteredPlaylistPath to be 'playlist.m3u', got '%s'", cfg.LocalFilteredPlaylistPath())
	}
	if cfg.LocalAllCategoriesPlaylistPath() != "playlist-all.m3u" {
		t.Errorf("expected LocalAllCategoriesPlaylistPath to be 'playlist-all.m3u', got '%s'", cfg.LocalAllCategoriesPlaylistPath())
	}

	os.Setenv("S3_OBJECT_KEY", "custom.m3u")
	defer os.Unsetenv("S3_OBJECT_KEY")

	cfg2 := New()
	if cfg2.LocalFilteredPlaylistPath() != "custom.m3u" {
		t.Errorf("expected LocalFilteredPlaylistPath to be 'custom.m3u', got '%s'", cfg2.LocalFilteredPlaylistPath())
	}
	if cfg2.LocalAllCategoriesPlaylistPath() != "custom-all.m3u" {
		t.Errorf("expected LocalAllCategoriesPlaylistPath to be 'custom-all.m3u', got '%s'", cfg2.LocalAllCategoriesPlaylistPath())
	}

	os.Setenv("S3_OBJECT_KEY", "custom")
	cfg3 := New()
	if cfg3.LocalFilteredPlaylistPath() != "custom" {
		t.Errorf("expected LocalFilteredPlaylistPath to be 'custom', got '%s'", cfg3.LocalFilteredPlaylistPath())
	}
	if cfg3.LocalAllCategoriesPlaylistPath() != "custom-all" {
		t.Errorf("expected LocalAllCategoriesPlaylistPath to be 'custom-all', got '%s'", cfg3.LocalAllCategoriesPlaylistPath())
	}
}

func TestCategoriesToRemove(t *testing.T) {
	if len(CategoriesToRemove) != 1 {
		t.Errorf("expected 1 category, got %d", len(CategoriesToRemove))
	}
	if CategoriesToRemove[0] != "Взрослые" {
		t.Errorf("expected 'Взрослые', got '%s'", CategoriesToRemove[0])
	}
}

func TestChannelNamesToExclude(t *testing.T) {
	expected := []string{"Fashion", "СПАС", "Три ангела", "ЛДПР", "UA", "Sports"}
	if len(ChannelNamesToExclude) != len(expected) {
		t.Errorf("expected %d items, got %d", len(expected), len(ChannelNamesToExclude))
	}
	for i, v := range expected {
		if ChannelNamesToExclude[i] != v {
			t.Errorf("expected channel name %q, got %q", v, ChannelNamesToExclude[i])
		}
	}
}

func TestChannelsKeepAllVariants(t *testing.T) {
	expected := []string{"tlc", "москва 24", "москва-24"}
	if len(ChannelsKeepAllVariants) != len(expected) {
		t.Errorf("expected %d items, got %d", len(expected), len(ChannelsKeepAllVariants))
	}
	for i, v := range expected {
		if ChannelsKeepAllVariants[i] != v {
			t.Errorf("expected %q, got %q", v, ChannelsKeepAllVariants[i])
		}
	}
}

func TestEPGRetentionDaysDefault(t *testing.T) {
	cfg := New()
	if cfg.EPGRetentionDays() != 10 {
		t.Errorf("expected EPGRetentionDays to be 10, got %d", cfg.EPGRetentionDays())
	}
}

func TestEPGRetentionDaysFromEnv(t *testing.T) {
	os.Setenv("EPG_RETENTION_DAYS", "7")
	defer os.Unsetenv("EPG_RETENTION_DAYS")
	cfg := New()
	if cfg.EPGRetentionDays() != 7 {
		t.Errorf("expected EPGRetentionDays to be 7, got %d", cfg.EPGRetentionDays())
	}
}
