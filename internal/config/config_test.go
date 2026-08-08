package config

import (
	"os"
	"strings"
	"testing"
	"time"
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
	expected := []string{
		"Взрослые",
		"Adult (18+)",
		"XXX (18+)",
		"XXX",
		"Религия",
		"Религиозные",
		"💲💲💲Поддержи Проект💲💲💲",
		"🔺 INFO",
		"АнтиРОССИЙСКИЕ",
		"𝕐𝕜𝕡𝕒Їℍ𝕒",
		"Українські",
		"Украина",
		"Наш Нет",
		"ℂп𝕠𝕡т",
		"186",
		"РАДИО ТВ",
		"𝕄𝕦𝕤𝕚𝕔",
		"𝕋𝕧ℤ𝕒𝕋𝕒𝕜",
		"32",
		"TVS",
		"TvZaTak",
		"MavTV ⭐️",
		"aleks-u-romki* 😊",
		"Play-x",
		"𝐊𝐢𝐧𝐨",
		"𝕂иℍ𝕠",
		"РОССИЯ+",
		"Гранд",
	}
	if len(CategoriesToRemove) != len(expected) {
		t.Errorf("expected %d categories, got %d", len(expected), len(CategoriesToRemove))
	}
	for i, v := range expected {
		if CategoriesToRemove[i] != v {
			t.Errorf("expected category %q, got %q", v, CategoriesToRemove[i])
		}
	}
}

func TestCategoriesToRemoveSubstring(t *testing.T) {
	// Verify all substrings are non-empty and in lowercase.
	for i, s := range CategoriesToRemoveSubstring {
		if s == "" {
			t.Errorf("CategoriesToRemoveSubstring[%d] is empty", i)
		}
		if s != strings.ToLower(s) {
			t.Errorf("CategoriesToRemoveSubstring[%d] is not lowercase: %q", i, s)
		}
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

func TestEPGExcludedCategories(t *testing.T) {
	if len(EPGExcludedCategories) == 0 {
		t.Error("expected at least one EPG excluded category")
	}
}

func TestEPGExcludedChannelIDs(t *testing.T) {
	if len(EPGExcludedChannelIDs) == 0 {
		t.Error("expected at least one EPG excluded channel ID")
	}
}

func TestEPGRetentionDaysDefault(t *testing.T) {
	cfg := New()
	if cfg.EPGRetentionDays() != 3 {
		t.Errorf("expected EPGRetentionDays to be 3, got %d", cfg.EPGRetentionDays())
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

func TestSkipSSLVerifyDefault(t *testing.T) {
	cfg := New()
	if cfg.SkipSSLVerify() {
		t.Error("expected SkipSSLVerify to be false by default")
	}
}

func TestProbeConfigDefaults(t *testing.T) {
	cfg := New()
	if cfg.ProbeSources() {
		t.Error("expected ProbeSources to be false by default")
	}
	if cfg.ProbeTimeout() != 5*time.Second {
		t.Errorf("expected ProbeTimeout to be 5s, got %v", cfg.ProbeTimeout())
	}
	if cfg.ProbeConcurrency() != 20 {
		t.Errorf("expected ProbeConcurrency to be 20, got %d", cfg.ProbeConcurrency())
	}
	if cfg.MaxChannelVariants() != 1 {
		t.Errorf("expected MaxChannelVariants to be 1, got %d", cfg.MaxChannelVariants())
	}
}

func TestProbeConfigFromEnv(t *testing.T) {
	os.Setenv("PROBE_SOURCES", "true")
	os.Setenv("PROBE_TIMEOUT_SECONDS", "3")
	os.Setenv("PROBE_CONCURRENCY", "1")
	os.Setenv("MAX_CHANNEL_VARIANTS", "9")
	defer func() {
		os.Unsetenv("PROBE_SOURCES")
		os.Unsetenv("PROBE_TIMEOUT_SECONDS")
		os.Unsetenv("PROBE_CONCURRENCY")
		os.Unsetenv("MAX_CHANNEL_VARIANTS")
	}()

	cfg := New()
	if !cfg.ProbeSources() {
		t.Error("expected ProbeSources to be true")
	}
	if cfg.ProbeTimeout() != 3*time.Second {
		t.Errorf("expected ProbeTimeout to be 3s, got %v", cfg.ProbeTimeout())
	}
	if cfg.ProbeConcurrency() != 1 {
		t.Errorf("expected ProbeConcurrency to be 1, got %d", cfg.ProbeConcurrency())
	}
	// MAX_CHANNEL_VARIANTS=9 must be clamped to the max of 5.
	if cfg.MaxChannelVariants() != 5 {
		t.Errorf("expected MaxChannelVariants clamped to 5, got %d", cfg.MaxChannelVariants())
	}
}

func TestSkipSSLVerifyFromEnv(t *testing.T) {
	os.Setenv("SKIP_SSL_VERIFY", "true")
	defer os.Unsetenv("SKIP_SSL_VERIFY")
	cfg := New()
	if !cfg.SkipSSLVerify() {
		t.Error("expected SkipSSLVerify to be true when SKIP_SSL_VERIFY=true")
	}
}
