package main

import (
	"fmt"
	"net/url"
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

func saveFilteredM3ULocally(content, filename string, cfg *config.Config) {
	outputDir := cfg.OUTPUT_DIR()
	os.MkdirAll(outputDir, 0755)
	filepath := path.Join(outputDir, filename)
	if err := os.WriteFile(filepath, []byte(content), 0644); err != nil {
		log.Error("Failed to save M3U file: %v", err)
		return
	}
	fi, _ := os.Stat(filepath)
	log.Info("M3U saved locally as %s (size: %.2f KB)", filepath, float64(fi.Size())/1024)
}

func buildCustomEPGURL(endpointURL, bucketName, epgKey string) string {
	parsed, err := url.Parse(endpointURL)
	if err != nil {
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
	s3EPGKey := cfg.S3EPGKey()
	categoriesToRemove := config.CategoriesToRemove
	dryRun := cfg.DryRun()

	m3uURLs := strings.Split(m3uURL, ",")
	var validURLs []string
	for _, u := range m3uURLs {
		u = strings.TrimSpace(u)
		if u != "" {
			validURLs = append(validURLs, u)
		}
	}
	m3uURLs = validURLs
	log.Info("Processing %d M3U source(s)", len(m3uURLs))

	endpointURL := cfg.S3EndpointURL()
	bucketName := cfg.S3DefaultBucketName()
	epgKey := cfg.S3EPGKey()
	region := cfg.S3Region()
	s3Endpoint := endpointURL
	customEPGURL := buildCustomEPGURL(endpointURL, bucketName, epgKey)

	var allFiltered []string
	var allOriginal []string

	for i, urlStr := range m3uURLs {
		if len(m3uURLs) > 1 {
			log.Info("Downloading M3U source %d/%d: %s", i+1, len(m3uURLs), urlStr)
		} else {
			log.Info("Downloading M3U source: %s", urlStr)
		}

		var m3uContent string
		err := utils.Retry(3, 2*time.Second, 2.0, func() error {
			var e error
			m3uContent, e = m3u.DownloadM3U(urlStr)
			return e
		})
		if err != nil {
			log.Error("Failed to download M3U from %s: %v", urlStr, err)
			return 1
		}

		allOriginal = append(allOriginal, m3uContent)
		chNamesToExclude := config.ChannelNamesToExclude
		filtered := m3u.FilterContent(m3uContent, categoriesToRemove, chNamesToExclude, customEPGURL)
		allFiltered = append(allFiltered, filtered)
	}

	var filteredContent string
	if len(allFiltered) > 1 {
		var mergedLines []string
		for idx, part := range allFiltered {
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
		filteredContent = strings.Join(mergedLines, "\n")
	} else if len(allFiltered) == 1 {
		filteredContent = allFiltered[0]
	}

	categoriesFilePath := cfg.CategoriesFilePath()
	if categoriesFilePath != "" {
		log.Info("Loading channel metadata from: %s", categoriesFilePath)
		categoriesMapping := m3u.ParseCategoriesFile(categoriesFilePath)
		if len(categoriesMapping) > 0 {
			filteredContent = m3u.ApplyChannelMetadata(filteredContent, categoriesMapping)
		}
	}

	var originalContent string
	if len(allOriginal) > 1 {
		var mergedLines []string
		for idx, part := range allOriginal {
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
		originalContent = strings.Join(mergedLines, "\n")
	} else if len(allOriginal) == 1 {
		originalContent = allOriginal[0]
	}

	saveFilteredM3ULocally(filteredContent, cfg.LocalFilteredPlaylistPath(), cfg)
	saveFilteredM3ULocally(originalContent, cfg.LocalAllCategoriesPlaylistPath(), cfg)

	if epgURL != "" {
		log.Info("Starting EPG filtering process")

		var epgContent string
		err := utils.Retry(3, 2*time.Second, 2.0, func() error {
			var e error
			epgContent, e = epg.DownloadEPG(epgURL, cfg)
			return e
		})
		if err != nil {
			log.Error("Failed to download EPG: %v", err)
		} else {
			epgNameToIDMap := epg.BuildEPGNameToIDMap(epgContent)
			filteredContent = m3u.AddTvgIDsToPlaylist(filteredContent, epgNameToIDMap)
			saveFilteredM3ULocally(filteredContent, cfg.LocalFilteredPlaylistPath(), cfg)

			chIDs, chNames := epg.ExtractChannelInfoFromPlaylist(filteredContent)
			excludedCategories := config.EPGExcludedCategories
			excludedChannelIDs := config.EPGExcludedChannelIDs

			filteredEPG, err := epg.FilterEPGContent(epgContent, chIDs, excludedCategories, excludedChannelIDs, chNames)
			if err != nil {
				log.Error("Failed to filter EPG: %v", err)
			} else {
				epg.SaveFilteredEPGLocally(filteredEPG, cfg.LocalFilteredEPGPath(), cfg)

				if !dryRun {
					outputDir := cfg.OUTPUT_DIR()
					utils.Retry(3, 2*time.Second, 2.0, func() error {
						return s3.UploadFileToS3(cfg.LocalFilteredEPGPath(), s3Bucket, s3EPGKey, outputDir, s3Endpoint, region, "application/gzip")
					})
				}
			}
		}
	}

	if dryRun {
		log.Info("Dry-run mode: Files saved locally, skipping S3 upload")
		return 0
	}

	accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	if accessKey == "" || secretKey == "" {
		log.Warning("AWS credentials not found in environment variables. Make sure they are set.")
	}
	if s3Bucket == "" || s3Bucket == "your-bucket-name" {
		log.Error("S3_BUCKET_NAME environment variable not set. Please configure it.")
		return 1
	}

	utils.Retry(3, 2*time.Second, 2.0, func() error {
		_, err := s3.UploadArchiveToS3(filteredContent, s3Bucket, s3FilteredKey, s3Endpoint, region)
		return err
	})
	utils.Retry(3, 2*time.Second, 2.0, func() error {
		_, err := s3.UploadArchiveToS3(originalContent, s3Bucket, s3AllKey, s3Endpoint, region)
		return err
	})
	utils.Retry(3, 2*time.Second, 2.0, func() error {
		return s3.UploadToS3(filteredContent, s3Bucket, s3FilteredKey, s3Endpoint, region, "")
	})
	utils.Retry(3, 2*time.Second, 2.0, func() error {
		return s3.UploadToS3(originalContent, s3Bucket, s3AllKey, s3Endpoint, region, "")
	})

	log.Info("Process completed successfully")
	return 0
}

func main() {
	os.Exit(run())
}
