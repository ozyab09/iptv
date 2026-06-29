package main

import (
	"os"
	"path"
	"strings"
	"time"

	"github.com/ozyab/iptv/internal/config"
	"github.com/ozyab/iptv/internal/epg"
	"github.com/ozyab/iptv/internal/m3u"
	"github.com/ozyab/iptv/internal/s3"
	"github.com/ozyab/iptv/internal/utils"
)

var log = utils.NewSanitizedLoggerWithPrefix("[main]")

// saveFilteredM3ULocally writes M3U content to a file in the output directory.
func saveFilteredM3ULocally(content, filename string, cfg *config.Config) {
	outputDir := cfg.OutputDir()
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Error("Failed to create output directory: %v", err)
		return
	}
	filepath := path.Join(outputDir, filename)
	if err := os.WriteFile(filepath, []byte(content), 0644); err != nil {
		log.Error("Failed to save M3U file: %v", err)
		return
	}
	if fi, err := os.Stat(filepath); err == nil {
		log.Info("M3U saved locally as %s (size: %.2f KB)", filepath, float64(fi.Size())/1024)
	}
}

// mergeParts combines multiple M3U playlists, keeping only the first #EXTM3U header.
func mergeParts(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		return parts[0]
	}
	var mergedLines []string
	for idx, part := range parts {
		lines := strings.Split(part, "\n")
		for _, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "#EXTM3U") {
				if idx == 0 && len(mergedLines) == 0 {
					mergedLines = append(mergedLines, line)
				}
				continue
			}
			if strings.TrimSpace(line) != "" {
				mergedLines = append(mergedLines, line)
			}
		}
	}
	return strings.Join(mergedLines, "\n")
}

// parseM3USources splits comma-separated M3U URLs and returns valid URLs.
func parseM3USources(m3uURL string) []string {
	parts := strings.Split(m3uURL, ",")
	var valid []string
	for _, u := range parts {
		u = strings.TrimSpace(u)
		if u != "" {
			valid = append(valid, u)
		}
	}
	return valid
}

// downloadAndFilterM3U downloads M3U from url, filters it, and returns filtered + original content.
func downloadAndFilterM3U(urlStr string, categoriesToRemove, chNamesToExclude []string, customEPGURL string) (filtered, original string, err error) {
	log.Info("Downloading M3U source: %s", urlStr)

	if err = utils.Retry(3, 2*time.Second, 2.0, func() error {
		original, err = m3u.DownloadM3U(urlStr)
		return err
	}); err != nil {
		return "", "", err
	}

	filtered = m3u.FilterContent(original, categoriesToRemove, chNamesToExclude, customEPGURL)
	return filtered, original, nil
}

// uploadWithRetry wraps an S3 upload function with retry logic.
func uploadWithRetry(fn func() error) {
	if err := utils.Retry(3, 2*time.Second, 2.0, fn); err != nil {
		log.Error("Upload failed after retries: %v", err)
	}
}

func run() int {
	cfg := config.New()

	if errs := cfg.Validate(); len(errs) > 0 {
		for _, e := range errs {
			log.Error("Configuration error: %s", e)
		}
		return 1
	}

	m3uURL := cfg.M3USourceURL()
	epgURL := cfg.EPGSourceURL()
	s3Bucket := cfg.S3DefaultBucketName()
	s3FilteredKey := cfg.S3FilteredPlaylistKey()
	s3AllKey := cfg.S3AllCategoriesPlaylistKey()
	categoriesToRemove := config.CategoriesToRemove
	dryRun := cfg.DryRun()
	s3Endpoint := cfg.S3EndpointURL()
	customEPGURL := cfg.BuildCustomEPGURL()

	m3uURLs := parseM3USources(m3uURL)
	log.Info("Processing %d M3U source(s)", len(m3uURLs))

	// Download and filter each M3U source.
	var allFiltered []string
	var allOriginal []string
	for _, urlStr := range m3uURLs {
		filtered, original, err := downloadAndFilterM3U(urlStr, categoriesToRemove, config.ChannelNamesToExclude, customEPGURL)
		if err != nil {
			log.Error("Failed to download M3U from %s: %v", urlStr, err)
			return 1
		}
		allOriginal = append(allOriginal, original)
		allFiltered = append(allFiltered, filtered)
	}

	filteredContent := mergeParts(allFiltered)
	originalContent := mergeParts(allOriginal)

	// Apply channel metadata from categories.txt if configured.
	categoriesFilePath := cfg.CategoriesFilePath()
	if categoriesFilePath != "" {
		if categoriesMapping := m3u.ParseCategoriesFile(categoriesFilePath); len(categoriesMapping) > 0 {
			filteredContent = m3u.ApplyChannelMetadata(filteredContent, categoriesMapping)
		}
	}

	saveFilteredM3ULocally(filteredContent, cfg.LocalFilteredPlaylistPath(), cfg)
	saveFilteredM3ULocally(originalContent, cfg.LocalAllCategoriesPlaylistPath(), cfg)

	// Process EPG: download, match tvg-ids, filter programmes.
	if epgURL != "" {
		processEPG(cfg, epgURL, &filteredContent, s3Bucket, s3Endpoint, dryRun)
	}

	if dryRun {
		log.Info("Dry-run mode: Files saved locally, skipping S3 upload")
		return 0
	}

	// Upload all content to S3.
	uploadWithRetry(func() error {
		_, err := s3.UploadArchiveToS3(filteredContent, s3Bucket, s3FilteredKey, s3Endpoint, cfg.S3Region())
		return err
	})
	uploadWithRetry(func() error {
		_, err := s3.UploadArchiveToS3(originalContent, s3Bucket, s3AllKey, s3Endpoint, cfg.S3Region())
		return err
	})
	uploadWithRetry(func() error {
		return s3.UploadToS3(filteredContent, s3Bucket, s3FilteredKey, s3Endpoint, cfg.S3Region(), "")
	})
	uploadWithRetry(func() error {
		return s3.UploadToS3(originalContent, s3Bucket, s3AllKey, s3Endpoint, cfg.S3Region(), "")
	})

	log.Info("Process completed successfully")
	return 0
}

// processEPG handles EPG download, tvg-id matching, filtering, and upload.
func processEPG(cfg *config.Config, epgURL string, filteredContent *string, s3Bucket, s3Endpoint string, dryRun bool) {
	log.Info("Starting EPG filtering process")

	var epgContent string
	if err := utils.Retry(3, 2*time.Second, 2.0, func() error {
		var e error
		epgContent, e = epg.DownloadEPG(epgURL, cfg)
		return e
	}); err != nil {
		log.Error("Failed to download EPG: %v", err)
		return
	}

	// Build tvg-id map from EPG and add missing tvg-ids to filtered playlist.
	epgNameToIDMap := epg.BuildEPGNameToIDMap(epgContent)
	*filteredContent = m3u.AddTvgIDsToPlaylist(*filteredContent, epgNameToIDMap)
	saveFilteredM3ULocally(*filteredContent, cfg.LocalFilteredPlaylistPath(), cfg)

	// Extract channel info from playlist and filter EPG programmes.
	chIDs, chNames := epg.ExtractChannelInfoFromPlaylist(*filteredContent)
	retentionDays := cfg.EPGRetentionDays()

	filteredEPG, err := epg.FilterEPGContent(epgContent, chIDs, config.EPGExcludedCategories, config.EPGExcludedChannelIDs, chNames, retentionDays)
	if err != nil {
		log.Error("Failed to filter EPG: %v", err)
		return
	}

	epg.SaveFilteredEPGLocally(filteredEPG, cfg.LocalFilteredEPGPath(), cfg)

	// Upload filtered EPG to S3 (non-dry-run only).
	if !dryRun {
		outputDir := cfg.OutputDir()
		uploadWithRetry(func() error {
			return s3.UploadFileToS3(cfg.LocalFilteredEPGPath(), s3Bucket, cfg.S3EPGKey(), outputDir, s3Endpoint, cfg.S3Region(), "application/gzip")
		})
	}
}

func main() {
	os.Exit(run())
}
