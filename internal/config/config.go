package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct{}

func New() *Config {
	return &Config{}
}

func (c *Config) M3USourceURL() string {
	return os.Getenv("M3U_SOURCE_URL")
}

func (c *Config) S3DefaultBucketName() string {
	return os.Getenv("S3_BUCKET_NAME")
}

func (c *Config) S3FilteredPlaylistKey() string {
	if v := os.Getenv("S3_OBJECT_KEY"); v != "" {
		return v
	}
	return "playlist.m3u"
}

func (c *Config) S3AllCategoriesPlaylistKey() string {
	return "playlist-all.m3u"
}

func (c *Config) S3EndpointURL() string {
	return os.Getenv("S3_ENDPOINT_URL")
}

func (c *Config) S3Region() string {
	if v := os.Getenv("S3_REGION"); v != "" {
		return v
	}
	return "us-east-1"
}

const MaxM3UFileSize = 100 * 1024 * 1024
const MaxEPGFileSize = 500 * 1024 * 1024

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

func (c *Config) LocalFilteredPlaylistPath() string {
	if v := os.Getenv("S3_OBJECT_KEY"); v != "" {
		return v
	}
	return "playlist.m3u"
}

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

func (c *Config) EPGSourceURL() string {
	return os.Getenv("EPG_SOURCE_URL")
}

func (c *Config) S3EPGKey() string {
	return os.Getenv("S3_EPG_KEY")
}

func (c *Config) LocalEPGPath() string {
	if v := os.Getenv("LOCAL_EPG_PATH"); v != "" {
		return v
	}
	return "epg.xml.gz"
}

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

var CategoriesToRemove = []string{
	"Взрослые",
}

var ChannelNamesToExclude = []string{
	"Fashion",
	"СПАС",
	"Три ангела",
	"ЛДПР",
	"UA",
	"Sports",
}

var ChannelsKeepAllVariants = []string{
	"tlc",
	"москва 24",
	"москва-24",
}

var EPGExcludedCategories = []string{
	"Кино",
}

var EPGExcludedChannelIDs = []string{
	"2745", "6170", "6168", "7553", "6171", "9228", "7552",
	"4729", "7594", "7595", "9233", "8822", "8817", "2438",
	"8811", "6848", "9025", "153", "66", "2760", "494",
	"6135", "9303", "5387", "2420", "2239", "9183", "774",
	"810", "6419",
}

func (c *Config) OUTPUT_DIR() string {
	if v := os.Getenv("OUTPUT_DIR"); v != "" {
		return v
	}
	return "output"
}

func (c *Config) CategoriesFilePath() string {
	return os.Getenv("CATEGORIES_FILE_PATH")
}

func (c *Config) DryRun() bool {
	return strings.EqualFold(os.Getenv("DRY_RUN"), "true") ||
		os.Getenv("DRY_RUN") == "1" ||
		strings.EqualFold(os.Getenv("DRY_RUN"), "yes") ||
		strings.EqualFold(os.Getenv("DRY_RUN"), "on")
}

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
