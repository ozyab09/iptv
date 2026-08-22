package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path"
	"strings"
	"sync"
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
func saveFile(content, filename string, cfg *config.Config) error {
	filepath := path.Join(cfg.OutputDir(), filename)
	if err := os.WriteFile(filepath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write %s: %w", filepath, err)
	}
	if fi, err := os.Stat(filepath); err == nil {
		log.Info("Saved locally as %s (size: %.2f KB)", filepath, float64(fi.Size())/1024)
	}
	return nil
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
	err := utils.RetryWithContext(ctx, 3, 2*time.Second, 2.0, func() error {
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

// ─── Pipeline step: normalize categories (allow-list) ─────────────────────────

func normalizeCategories(content string) string {
	return m3u.NormalizeCategories(content, config.CategoryAliases, config.AllowedCategorySet(), config.FallbackCategory)
}

// ─── Pipeline step: process EPG ──────────────────────────────────────────────────

// processEPG filters the already-downloaded EPG content. epgContent and
// epgNameToIDMap are downloaded/built once earlier in the pipeline (step 2b) so
// the same data can validate tvg-ids inherited during dedup.
func processEPG(ctx context.Context, cfg *config.Config, epgPath string, epgNameToIDMap map[string]string, epgIDSet map[string]bool, filteredContent string, s3Client *awss3.Client, dryRun bool) (string, error) {
	log.Info("Starting EPG filtering process")

	filteredContent = m3u.AddTvgIDsToPlaylist(filteredContent, epgNameToIDMap)
	// Каналы без id наследуют его от соседних вариантов того же канала
	// (например "Channel SD" от "Channel HD"), но только если id есть в EPG.
	filteredContent = m3u.InheritTvgIDsFromSiblings(filteredContent, epgIDSet)
	if err := saveFile(filteredContent, cfg.LocalFilteredPlaylistPath(), cfg); err != nil {
		return filteredContent, err
	}

	chIDs, chNames := epg.ExtractChannelInfoFromPlaylist(filteredContent)
	if err := epg.FilterEPGFile(epgPath, path.Join(cfg.OutputDir(), cfg.LocalFilteredEPGPath()), chIDs, config.EPGExcludedCategories, config.EPGExcludedChannelIDs, chNames, cfg.EPGRetentionDays()); err != nil {
		return filteredContent, err
	}

	if !dryRun && s3Client != nil {
		if err := uploadWithRetry(ctx, func() error {
			return s3.UploadFileToS3(ctx, s3Client, cfg.LocalFilteredEPGPath(), cfg.S3DefaultBucketName(), cfg.S3EPGKey(), cfg.OutputDir(), "application/gzip")
		}); err != nil {
			return filteredContent, err
		}
	}

	return filteredContent, nil
}

// ─── Pipeline step: upload ──────────────────────────────────────────────────────

func uploadWithRetry(ctx context.Context, fn func() error) error {
	if err := utils.RetryWithContext(ctx, 3, 2*time.Second, 2.0, fn); err != nil {
		return fmt.Errorf("upload failed after retries: %w", err)
	}
	return nil
}

func uploadBoth(ctx context.Context, client *awss3.Client, content, bucket, key string) error {
	return uploadWithRetry(ctx, func() error {
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
	customEPGURL := ""
	if epgURL != "" && cfg.S3DefaultBucketName() != "" && cfg.S3EndpointURL() != "" {
		customEPGURL = cfg.BuildCustomEPGURL()
	}
	skipSSL := cfg.SkipSSLVerify()

	m3uURLs := parseM3USources(m3uURL)
	log.Info("Processing %d M3U source(s)", len(m3uURLs))

	// Step 1: Download and filter M3U sources in parallel (bounded concurrency,
	// source order preserved via index). Each source keeps its own retry, so one
	// slow/failing source does not block the others.
	type m3uResult struct {
		idx      int
		original string
		filtered string
		err      error
	}
	results := make(chan m3uResult, len(m3uURLs))
	var wg sync.WaitGroup
	for i, urlStr := range m3uURLs {
		wg.Add(1)
		go func(i int, urlStr string) {
			defer wg.Done()
			original, err := downloadM3U(ctx, urlStr, skipSSL)
			if err != nil {
				results <- m3uResult{idx: i, err: err}
				return
			}
			results <- m3uResult{idx: i, original: original, filtered: filterM3U(original, cfg, customEPGURL)}
		}(i, urlStr)
	}
	wg.Wait()
	close(results)

	allOriginal := make([]string, len(m3uURLs))
	allFiltered := make([]string, len(m3uURLs))
	for res := range results {
		if res.err != nil {
			log.Error("Failed to download M3U from %s: %v", m3uURLs[res.idx], res.err)
			return 1
		}
		allOriginal[res.idx] = res.original
		allFiltered[res.idx] = res.filtered
	}

	filteredContent := mergeParts(allFiltered)
	originalContent := mergeParts(allOriginal)

	// Step 2: Apply channel metadata, then normalize categories via allow-list
	// (duplicate/provider-specific categories collapse into canonical names;
	// anything not on the allow-list moves to the fallback category). Channels
	// are never removed — only their group-title changes.
	filteredContent = applyMetadata(filteredContent, cfg)
	filteredContent = normalizeCategories(filteredContent)
	// Reclassify channels that fell into the fallback category when their name
	// carries an unambiguous genre keyword (e.g. "...Футбол..." → Спорт).
	filteredContent = m3u.ClassifyFallbackCategories(
		filteredContent,
		config.CategoryKeywords,
		config.AllowedCategorySet(),
		config.FallbackCategory,
	)

	// Step 2b: Download EPG once, early. Its channel-id set is used to validate
	// tvg-ids inherited from dropped variants during dedup (option C merge); the
	// same content feeds the EPG filtering step later.
	var epgPath string
	var epgNameToIDMap map[string]string
	var epgIDSet map[string]bool
	if epgURL != "" {
		// Retry is handled per-source inside DownloadEPGToFile: a failing source
		// is retried and skipped (with a warning) so the remaining sources still
		// produce a merged EPG.
		var e error
		epgPath, e = epg.DownloadEPGToFile(ctx, epgURL, cfg)
		if e != nil {
			log.Error("Failed to download EPG: %v", e)
			return 1
		}
		var err error
		epgNameToIDMap, err = epg.BuildEPGNameToIDMapFromFile(epgPath)
		if err != nil {
			log.Error("Failed to build EPG name-to-id map: %v", err)
			return 1
		}
		epgIDSet = make(map[string]bool, len(epgNameToIDMap))
		for _, id := range epgNameToIDMap {
			epgIDSet[id] = true
		}
		defer func() {
			if err := os.Remove(epgPath); err != nil && !os.IsNotExist(err) {
				log.Warning("Failed to remove temporary EPG XML: %v", err)
			}
		}()
	}

	// Step 2c: Optionally deduplicate by channel name, keeping only working
	// sources (option C: HEAD/GET availability probing, quality-first). Kept
	// entries lacking a tvg-id inherit one from sibling variants when the id
	// exists in the EPG (stale ids are never inherited).
	// Availability probing is skipped in dry-run: the probe callback stays nil,
	// so dedup falls back to deterministic quality-first selection (best-quality
	// variant per channel) without checking source availability.
	if cfg.ProbeSources() {
		var probe func(candidates []utils.ProbeCandidate) map[string]bool
		if !dryRun {
			probe = func(candidates []utils.ProbeCandidate) map[string]bool {
				return utils.ProbeCandidates(ctx, candidates, cfg.ProbeConcurrency(), cfg.ProbeTimeout(), skipSSL)
			}
		}
		filteredContent = m3u.DeduplicateByName(filteredContent, cfg.MaxChannelVariants(), probe, epgIDSet)
	}

	// Step 3: Save files locally.
	if err := saveFile(filteredContent, cfg.LocalFilteredPlaylistPath(), cfg); err != nil {
		log.Error("Failed to save filtered playlist: %v", err)
		return 1
	}
	if err := saveFile(originalContent, cfg.LocalAllCategoriesPlaylistPath(), cfg); err != nil {
		log.Error("Failed to save unfiltered playlist: %v", err)
		return 1
	}

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
		filteredContent, err = processEPG(ctx, cfg, epgPath, epgNameToIDMap, epgIDSet, filteredContent, s3Client, dryRun)
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
	if err := uploadBoth(ctx, s3Client, filteredContent, s3Bucket, s3FilteredKey); err != nil {
		log.Error("Failed to upload filtered playlist: %v", err)
		return 1
	}
	if err := uploadBoth(ctx, s3Client, originalContent, s3Bucket, s3AllKey); err != nil {
		log.Error("Failed to upload unfiltered playlist: %v", err)
		return 1
	}

	log.Info("Process completed successfully")
	return 0
}

func main() {
	os.Exit(run())
}
