// Package config provides configuration from environment variables with validation.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Config is a stateless configuration reader.
// All methods read directly from environment variables for simplicity.
type Config struct{}

func New() *Config {
	return &Config{}
}

// M3USourceURL returns the M3U source URL (comma-separated for multiple sources).
func (c *Config) M3USourceURL() string {
	return os.Getenv("M3U_SOURCE_URL")
}

// S3DefaultBucketName returns the S3 bucket name.
func (c *Config) S3DefaultBucketName() string {
	return os.Getenv("S3_BUCKET_NAME")
}

// S3FilteredPlaylistKey returns the S3 key for the filtered playlist (default: playlist.m3u).
func (c *Config) S3FilteredPlaylistKey() string {
	if v := os.Getenv("S3_OBJECT_KEY"); v != "" {
		return v
	}
	return "playlist.m3u"
}

// S3AllCategoriesPlaylistKey returns the S3 key for the unfiltered playlist.
func (c *Config) S3AllCategoriesPlaylistKey() string {
	return "playlist-all.m3u"
}

// S3EndpointURL returns the S3-compatible endpoint URL.
func (c *Config) S3EndpointURL() string {
	return os.Getenv("S3_ENDPOINT_URL")
}

// S3Region returns the S3 region (default: us-east-1).
func (c *Config) S3Region() string {
	if v := os.Getenv("S3_REGION"); v != "" {
		return v
	}
	return "us-east-1"
}

// MaxM3UFileSize is the maximum M3U download size (100 MB).
const MaxM3UFileSize = 100 * 1024 * 1024

// MaxEPGFileSize is the maximum EPG download size (500 MB).
const MaxEPGFileSize = 500 * 1024 * 1024

// EPGRetentionDays returns how many days of EPG data to keep (default: 10).
func (c *Config) EPGRetentionDays() int {
	val := os.Getenv("EPG_RETENTION_DAYS")
	if val == "" {
		return 10
	}
	n, err := strconv.Atoi(val)
	if err != nil || n < 1 {
		return 10
	}
	return n
}

// LocalFilteredPlaylistPath returns the local filename for the filtered playlist.
func (c *Config) LocalFilteredPlaylistPath() string {
	if v := os.Getenv("S3_OBJECT_KEY"); v != "" {
		return v
	}
	return "playlist.m3u"
}

// LocalAllCategoriesPlaylistPath returns the local filename for the unfiltered playlist.
func (c *Config) LocalAllCategoriesPlaylistPath() string {
	s3Key := os.Getenv("S3_OBJECT_KEY")
	if s3Key == "" {
		s3Key = "playlist.m3u"
	}
	if idx := strings.LastIndex(s3Key, "."); idx >= 0 {
		return s3Key[:idx] + "-all" + s3Key[idx:]
	}
	return s3Key + "-all"
}

// EPGSourceURL returns the EPG XML source URL.
func (c *Config) EPGSourceURL() string {
	return os.Getenv("EPG_SOURCE_URL")
}

// S3EPGKey returns the S3 key for the EPG file.
func (c *Config) S3EPGKey() string {
	return os.Getenv("S3_EPG_KEY")
}

// LocalEPGPath returns the local filename for the downloaded EPG.
func (c *Config) LocalEPGPath() string {
	if v := os.Getenv("LOCAL_EPG_PATH"); v != "" {
		return v
	}
	return "epg.xml.gz"
}

// LocalFilteredEPGPath returns the local filename for the filtered EPG.
func (c *Config) LocalFilteredEPGPath() string {
	s3Key := os.Getenv("S3_EPG_KEY")
	if s3Key == "" {
		s3Key = "epg.xml.gz"
	}
	if idx := strings.LastIndex(s3Key, "."); idx >= 0 {
		return s3Key[:idx] + "-filtered" + s3Key[idx:]
	}
	return s3Key + "-filtered"
}

// CategoriesToRemove is the deny-list of channel groups to filter out.
var CategoriesToRemove = []string{"Взрослые"}

// ChannelNamesToExclude lists channels removed by name substring match (case-insensitive).
var ChannelNamesToExclude = []string{
	"Fashion",
	"СПАС",
	"Три ангела",
	"ЛДПР",
	"UA",
	"Sports",
}

// EPGExcludedCategories lists EPG categories to exclude from the output.
var EPGExcludedCategories = []string{"Кино"}

// EPGExcludedChannelIDs lists specific EPG channel IDs to exclude.
var EPGExcludedChannelIDs = []string{
	"2745", "6170", "6168", "7553", "6171", "9228", "7552",
	"4729", "7594", "7595", "9233", "8822", "8817", "2438",
	"8811", "6848", "9025", "153", "66", "2760", "494",
	"6135", "9303", "5387", "2420", "2239", "9183", "774",
	"810", "6419",
}

// OutputDir returns the local output directory (default: output/).
func (c *Config) OutputDir() string {
	if v := os.Getenv("OUTPUT_DIR"); v != "" {
		return v
	}
	return "output"
}

// CategoriesFilePath returns the optional path to categories.txt for metadata overrides.
func (c *Config) CategoriesFilePath() string {
	return os.Getenv("CATEGORIES_FILE_PATH")
}

// DryRun returns true if DRY_RUN env var is set to a truthy value.
func (c *Config) DryRun() bool {
	return strings.EqualFold(os.Getenv("DRY_RUN"), "true") ||
		os.Getenv("DRY_RUN") == "1" ||
		strings.EqualFold(os.Getenv("DRY_RUN"), "yes") ||
		strings.EqualFold(os.Getenv("DRY_RUN"), "on")
}

// BuildCustomEPGURL constructs the public URL for the EPG file in S3.
// Used to inject url-tvg into the M3U header.
func (c *Config) BuildCustomEPGURL() string {
	endpointURL := c.S3EndpointURL()
	bucketName := c.S3DefaultBucketName()
	epgKey := c.S3EPGKey()

	parsed, err := url.Parse(endpointURL)
	if err != nil {
		// Try to extract host part manually
		hostPart := endpointURL
		if idx := strings.Index(endpointURL, "://"); idx >= 0 {
			hostPart = endpointURL[idx+3:]
		}
		return fmt.Sprintf("https://%s.%s/%s", bucketName, hostPart, epgKey)
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return fmt.Sprintf("%s://%s%s/%s/%s", parsed.Scheme, parsed.Host, strings.TrimRight(parsed.Path, "/"), bucketName, epgKey)
	}
	return fmt.Sprintf("%s://%s.%s/%s", parsed.Scheme, bucketName, parsed.Host, epgKey)
}

// Validate checks all required configuration and returns a list of errors.
func (c *Config) Validate() []string {
	var errors []string
	placeholderPatterns := []string{"your-", "your_provider", "your-epg-provider"}

	m3uURL := c.M3USourceURL()
	if m3uURL == "" {
		errors = append(errors, "M3U_SOURCE_URL must be specified")
	} else {
		lower := strings.ToLower(m3uURL)
		for _, p := range placeholderPatterns {
			if strings.Contains(lower, p) {
				errors = append(errors, "M3U_SOURCE_URL appears to be a placeholder. Please set a valid URL")
				break
			}
		}
		if len(errors) == 0 {
			urls := strings.Split(m3uURL, ",")
			hasValid := false
			for _, u := range urls {
				u = strings.TrimSpace(u)
				if u != "" {
					hasValid = true
					if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
						errors = append(errors, fmt.Sprintf("M3U_SOURCE_URL contains invalid URL: %s", u))
					}
				}
			}
			if !hasValid {
				errors = append(errors, "M3U_SOURCE_URL must contain at least one valid HTTP/HTTPS URL")
			}
		}
	}

	epgURL := c.EPGSourceURL()
	if epgURL == "" {
		errors = append(errors, "EPG_SOURCE_URL must be specified")
	} else {
		lower := strings.ToLower(epgURL)
		for _, p := range placeholderPatterns {
			if strings.Contains(lower, p) {
				errors = append(errors, "EPG_SOURCE_URL appears to be a placeholder. Please set a valid URL")
				break
			}
		}
		if len(errors) == 0 && !strings.HasPrefix(epgURL, "http://") && !strings.HasPrefix(epgURL, "https://") {
			errors = append(errors, "EPG_SOURCE_URL must be a valid HTTP/HTTPS URL")
		}
	}

	bucketName := c.S3DefaultBucketName()
	if bucketName == "" {
		errors = append(errors, "S3_BUCKET_NAME must be specified")
	} else if len(bucketName) < 3 || len(bucketName) > 63 {
		errors = append(errors, "S3_BUCKET_NAME must be between 3 and 63 characters")
	}

	playlistKey := c.S3FilteredPlaylistKey()
	if playlistKey == "" || strings.Contains(playlistKey, "..") || strings.HasPrefix(playlistKey, "/") {
		errors = append(errors, "S3_OBJECT_KEY must not contain '..' or start with '/'")
	}

	epgKey := c.S3EPGKey()
	if epgKey == "" || strings.Contains(epgKey, "..") || strings.HasPrefix(epgKey, "/") {
		errors = append(errors, "S3_EPG_KEY must not contain '..' or start with '/'")
	}

	endpointURL := c.S3EndpointURL()
	if endpointURL == "" {
		errors = append(errors, "S3_ENDPOINT_URL must be specified")
	} else if !strings.HasPrefix(endpointURL, "http://") && !strings.HasPrefix(endpointURL, "https://") {
		errors = append(errors, "S3_ENDPOINT_URL must be a valid HTTP/HTTPS URL")
	} else {
		parsed, err := url.Parse(endpointURL)
		if err == nil && strings.Contains(parsed.Host, "@") {
			errors = append(errors, "S3_ENDPOINT_URL should not contain credentials in the URL")
		}
	}

	if c.S3Region() == "" {
		errors = append(errors, "S3_REGION must be specified")
	}

	return errors
}
