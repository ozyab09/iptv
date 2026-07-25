package main

import (
	"context"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/ozyab/iptv/internal/config"
	"github.com/ozyab/iptv/internal/epg"
	"github.com/ozyab/iptv/internal/m3u"
	"github.com/ozyab/iptv/internal/s3"
	"github.com/ozyab/iptv/internal/utils"
)

var log = utils.NewSanitizedLoggerWithPrefix("[main]")

// gracefulCtx returns a context that is cancelled on SIGINT/SIGTERM.
func gracefulCtx() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Info("Received shutdown signal, cancelling operations...")
		cancel()
	}()
	return ctx, cancel
}

// saveFile writes M3U content to a file in the output directory.
func saveFile(content, filename string, cfg *config.Config) {
	outputDir := cfg.OutputDir()
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Error("Failed to create output directory: %v", err)
		return
	}
	filepath := path.Join(outputDir, filename)
	if err := os.WriteFile(filepath, []byte(content), 0644); err != nil {
		log.Error("Failed to save file: %v", err)
		return
	}
	if fi, err := os.Stat(filepath); err == nil {
		log.Info("Saved locally as %s (size: %.2f KB)", filepath, float64(fi.Size())/1024)
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

// parseM3USources splits comma-separated M3U URLs.
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

// ─── Pipeline step: download M3U ────────────────────────────────────────────────

func downloadM3U(ctx context.Context, urlStr string) (string, error) {
	log.Info("Downloading M3U source: %s", urlStr)
	var original string
	err := utils.Retry(3, 2*time.Second, 2.0, func() error {
		var e error
		original, e = m3u.DownloadM3U(urlStr)
		return e
	})
	return original, err
}

// ─── Pipeline step: filter M3U ──────────────────────────────────────────────────

func filterM3U(content string, cfg *config.Config, customEPGURL string) string {
	return m3u.FilterContent(
		content,
		config.CategoriesToRemove,
		config.CategoriesToRemoveSubstring,
		config.ChannelNamesToExclude,
		customEPGURL,
	)
}

// ─── Pipeline step: apply metadata ───────────────────────────────────────────────

func applyMetadata(content string, cfg *config.Config) string {
	categoriesFilePath := cfg.CategoriesFilePath()
	if categoriesFilePath == "" {
		return content
	}
	if categoriesMapping := m3u.ParseCategoriesFile(categoriesFilePath); len(categoriesMapping) > 0 {
		return m3u.ApplyChannelMetadata(content, categoriesMapping)
	}
	return content
}

// ─── Pipeline step: process EPG ──────────────────────────────────────────────────

func processEPG(ctx context.Context, cfg *config.Config, epgURL string, filteredContent string, dryRun bool) (string, error) {
	log.Info("Starting EPG filtering process")

	var epgContent string
	if err := utils.Retry(3, 2*time.Second, 2.0, func() error {
		var e error
		epgContent, e = epg.DownloadEPG(ctx, epgURL, cfg)
		return e
	}); err != nil {
		return filteredContent, err
	}

	// Build tvg-id map from EPG and add missing tvg-ids to filtered playlist.
	epgNameToIDMap := epg.BuildEPGNameToIDMap(epgContent)
	filteredContent = m3u.AddTvgIDsToPlaylist(filteredContent, epgNameToIDMap)
	saveFile(filteredContent, cfg.LocalFilteredPlaylistPath(), cfg)

	// Extract channel info and filter EPG programmes.
	chIDs, chNames := epg.ExtractChannelInfoFromPlaylist(filteredContent)
	filteredEPG, err := epg.FilterEPGContent(epgContent, chIDs, config.EPGExcludedCategories, config.EPGExcludedChannelIDs, chNames, cfg.EPGRetentionDays())
	if err != nil {
		return filteredContent, err
	}

	epg.SaveFilteredEPGLocally(filteredEPG, cfg.LocalFilteredEPGPath(), cfg)

	// Upload filtered EPG to S3 (non-dry-run only).
	if !dryRun {
		uploadWithRetry(ctx, func() error {
			return s3.UploadFileToS3(ctx, cfg.LocalFilteredEPGPath(), cfg.S3DefaultBucketName(), cfg.S3EPGKey(), cfg.OutputDir(), cfg.S3EndpointURL(), cfg.S3Region(), "application/gzip")
		})
	}

	return filteredContent, nil
}

// ─── Pipeline step: upload ──────────────────────────────────────────────────────

func uploadWithRetry(ctx context.Context, fn func() error) {
	if err := utils.Retry(3, 2*time.Second, 2.0, fn); err != nil {
		log.Error("Upload failed after retries: %v", err)
	}
}

func uploadBoth(ctx context.Context, content, bucket, key, s3Endpoint, region string) {
	uploadWithRetry(ctx, func() error {
		return s3.UploadBoth(ctx, content, bucket, key, s3Endpoint, region)
	})
}

// ─── Pipeline ────────────────────────────────────────────────────────────────────

func run() int {
	cfg := config.New()

	if errs := cfg.Validate(); len(errs) > 0 {
		for _, e := range errs {
			log.Error("Configuration error: %s", e)
		}
		return 1
	}

	ctx, cancel := gracefulCtx()
	defer cancel()

	m3uURL := cfg.M3USourceURL()
	epgURL := cfg.EPGSourceURL()
	s3Bucket := cfg.S3DefaultBucketName()
	s3FilteredKey := cfg.S3FilteredPlaylistKey()
	s3AllKey := cfg.S3AllCategoriesPlaylistKey()
	dryRun := cfg.DryRun()
	s3Endpoint := cfg.S3EndpointURL()
	customEPGURL := cfg.BuildCustomEPGURL()

	m3uURLs := parseM3USources(m3uURL)
	log.Info("Processing %d M3U source(s)", len(m3uURLs))

	// Step 1: Download and filter M3U sources.
	var allFiltered []string
	var allOriginal []string
	for _, urlStr := range m3uURLs {
		original, err := downloadM3U(ctx, urlStr)
		if err != nil {
			log.Error("Failed to download M3U from %s: %v", urlStr, err)
			return 1
		}
		filtered := filterM3U(original, cfg, customEPGURL)
		allOriginal = append(allOriginal, original)
		allFiltered = append(allFiltered, filtered)
	}

	filteredContent := mergeParts(allFiltered)
	originalContent := mergeParts(allOriginal)

	// Step 2: Apply channel metadata from categories.txt.
	filteredContent = applyMetadata(filteredContent, cfg)

	// Step 3: Save files locally.
	saveFile(filteredContent, cfg.LocalFilteredPlaylistPath(), cfg)
	saveFile(originalContent, cfg.LocalAllCategoriesPlaylistPath(), cfg)

	// Step 4: Process EPG (adds tvg-ids and filters EPG).
	if epgURL != "" {
		var err error
		filteredContent, err = processEPG(ctx, cfg, epgURL, filteredContent, dryRun)
		if err != nil {
			log.Error("EPG processing failed: %v", err)
			return 1
		}
	}

	if dryRun {
		log.Info("Dry-run mode: Files saved locally, skipping S3 upload")
		return 0
	}

	// Step 5: Upload everything to S3.
	uploadBoth(ctx, filteredContent, s3Bucket, s3FilteredKey, s3Endpoint, cfg.S3Region())
	uploadBoth(ctx, originalContent, s3Bucket, s3AllKey, s3Endpoint, cfg.S3Region())

	log.Info("Process completed successfully")
	return 0
}

func main() {
	os.Exit(run())
}
