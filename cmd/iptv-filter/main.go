package main

import (
	"context"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"

	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"

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

// saveFile writes content to a file in the output directory.
func saveFile(content, filename string, cfg *config.Config) {
	filepath := path.Join(cfg.OutputDir(), filename)
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

func downloadM3U(ctx context.Context, urlStr string, skipSSLVerify bool) (string, error) {
	log.Info("Downloading M3U source: %s", urlStr)
	var original string
	err := utils.Retry(3, 2*time.Second, 2.0, func() error {
		var e error
		original, e = m3u.DownloadM3UWithContext(ctx, urlStr, skipSSLVerify)
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

// processEPG filters the already-downloaded EPG content. epgContent and
// epgNameToIDMap are downloaded/built once earlier in the pipeline (step 2b) so
// the same data can validate tvg-ids inherited during dedup.
func processEPG(ctx context.Context, cfg *config.Config, epgContent string, epgNameToIDMap map[string]string, filteredContent string, s3Client *awss3.Client, dryRun bool) (string, error) {
	log.Info("Starting EPG filtering process")

	filteredContent = m3u.AddTvgIDsToPlaylist(filteredContent, epgNameToIDMap)
	saveFile(filteredContent, cfg.LocalFilteredPlaylistPath(), cfg)

	chIDs, chNames := epg.ExtractChannelInfoFromPlaylist(filteredContent)
	filteredEPG, err := epg.FilterEPGContent(epgContent, chIDs, config.EPGExcludedCategories, config.EPGExcludedChannelIDs, chNames, cfg.EPGRetentionDays())
	if err != nil {
		return filteredContent, err
	}

	epg.SaveFilteredEPGLocally(filteredEPG, cfg.LocalFilteredEPGPath(), cfg)

	if !dryRun && s3Client != nil {
		uploadWithRetry(ctx, func() error {
			return s3.UploadFileToS3(ctx, s3Client, cfg.LocalFilteredEPGPath(), cfg.S3DefaultBucketName(), cfg.S3EPGKey(), cfg.OutputDir(), "application/gzip")
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

func uploadBoth(ctx context.Context, client *awss3.Client, content, bucket, key string) {
	uploadWithRetry(ctx, func() error {
		return s3.UploadBoth(ctx, client, content, bucket, key, "")
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

	// Ensure output directory exists once.
	if err := cfg.EnsureOutputDir(); err != nil {
		log.Error("Failed to create output directory: %v", err)
		return 1
	}

	m3uURL := cfg.M3USourceURL()
	epgURL := cfg.EPGSourceURL()
	s3Bucket := cfg.S3DefaultBucketName()
	s3FilteredKey := cfg.S3FilteredPlaylistKey()
	s3AllKey := cfg.S3AllCategoriesPlaylistKey()
	dryRun := cfg.DryRun()
	s3Endpoint := cfg.S3EndpointURL()
	customEPGURL := cfg.BuildCustomEPGURL()
	skipSSL := cfg.SkipSSLVerify()

	m3uURLs := parseM3USources(m3uURL)
	log.Info("Processing %d M3U source(s)", len(m3uURLs))

	// Step 1: Download and filter M3U sources.
	var allFiltered []string
	var allOriginal []string
	for _, urlStr := range m3uURLs {
		original, err := downloadM3U(ctx, urlStr, skipSSL)
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

	// Step 2: Apply channel metadata.
	filteredContent = applyMetadata(filteredContent, cfg)

	// Step 2b: Download EPG once, early. Its channel-id set is used to validate
	// tvg-ids inherited from dropped variants during dedup (option C merge); the
	// same content feeds the EPG filtering step later.
	var epgContent string
	var epgNameToIDMap map[string]string
	if epgURL != "" {
		if err := utils.Retry(3, 2*time.Second, 2.0, func() error {
			var e error
			epgContent, e = epg.DownloadEPG(ctx, epgURL, cfg)
			return e
		}); err != nil {
			log.Error("Failed to download EPG: %v", err)
			return 1
		}
		epgNameToIDMap = epg.BuildEPGNameToIDMap(epgContent)
	}

	// Step 2c: Optionally deduplicate by channel name, keeping only working
	// sources (option C: HEAD/GET availability probing, quality-first). Kept
	// entries lacking a tvg-id inherit one from sibling variants when the id
	// exists in the EPG (stale ids are never inherited).
	if cfg.ProbeSources() {
		var epgIDSet map[string]bool
		if epgNameToIDMap != nil {
			epgIDSet = make(map[string]bool, len(epgNameToIDMap))
			for _, id := range epgNameToIDMap {
				epgIDSet[id] = true
			}
		}
		filteredContent = m3u.DeduplicateByName(filteredContent, cfg.MaxChannelVariants(), func(candidates []utils.ProbeCandidate) map[string]bool {
			return utils.ProbeCandidates(ctx, candidates, cfg.ProbeConcurrency(), cfg.ProbeTimeout(), skipSSL)
		}, epgIDSet)
	}

	// Step 3: Save files locally.
	saveFile(filteredContent, cfg.LocalFilteredPlaylistPath(), cfg)
	saveFile(originalContent, cfg.LocalAllCategoriesPlaylistPath(), cfg)

	// Step 4: Create reusable S3 client (if not dry-run).
	var s3Client *awss3.Client
	if !dryRun {
		var err error
		s3Client, err = s3.NewClient(ctx, s3Endpoint, cfg.S3Region())
		if err != nil {
			log.Error("Failed to create S3 client: %v", err)
			return 1
		}
	}

	// Step 5: Process EPG (content downloaded once in step 2b).
	if epgURL != "" {
		var err error
		filteredContent, err = processEPG(ctx, cfg, epgContent, epgNameToIDMap, filteredContent, s3Client, dryRun)
		if err != nil {
			log.Error("EPG processing failed: %v", err)
			return 1
		}
	}

	if dryRun {
		log.Info("Dry-run mode: Files saved locally, skipping S3 upload")
		return 0
	}

	// Step 6: Upload everything to S3 (reusing client).
	uploadBoth(ctx, s3Client, filteredContent, s3Bucket, s3FilteredKey)
	uploadBoth(ctx, s3Client, originalContent, s3Bucket, s3AllKey)

	log.Info("Process completed successfully")
	return 0
}

func main() {
	os.Exit(run())
}
