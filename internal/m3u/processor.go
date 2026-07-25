package m3u

import (
	"context"
	"fmt"
	"hash/fnv"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/ozyab/iptv/internal/config"
	"github.com/ozyab/iptv/internal/utils"
)

var logger = utils.NewSanitizedLoggerWithPrefix("[m3u]")

// Pre-compiled regexps for efficient filtering.
var (
	regRegional       = regexp.MustCompile(`\s\+\d+(?:\s+HD)?(?:\s*\([^)]+\))?\s*$`)
	regNumberSuffix   = regexp.MustCompile(`\s\d{2,}$`)
	regLeadingNumber  = regexp.MustCompile(`^\d+\.\s*`)
	regGroupTitle     = regexp.MustCompile(`group-title="([^"]*)"`)
	regTvgID          = regexp.MustCompile(`tvg-id="([^"]*)"`)
	regURLTVG         = regexp.MustCompile(`url-tvg="[^"]*"`)
	regTVGURL         = regexp.MustCompile(`tvg-url="[^"]*"`)
	regTvgLogo        = regexp.MustCompile(`tvg-logo="([^"]*)"`)
	regTvgRec         = regexp.MustCompile(`tvg-rec="([^"]*)"`)

	// Replacement-only regexps (no capture groups).
	regGroupTitleAttr = regexp.MustCompile(`group-title="[^"]*"`)
	regTvgIDAttr      = regexp.MustCompile(`tvg-id="[^"]*"`)
	regTvgLogoAttr    = regexp.MustCompile(`tvg-logo="[^"]*"`)
	regTvgRecAttr     = regexp.MustCompile(`tvg-rec="[^"]*"`)
)

// ChannelEntry represents a parsed M3U channel entry.
type ChannelEntry struct {
	EXTINFLine string
	ExtraLines []string
}

// ─── Parsing ─────────────────────────────────────────────────────────────────────

// ParseLine splits M3U content into headers (EXTM3U, comments) and channel entries.
func ParseChannelEntries(lines []string) ([]string, []ChannelEntry) {
	var headers []string
	var entries []ChannelEntry

	i := 0
	linesLen := len(lines)
	for i < linesLen {
		line := lines[i]
		if strings.HasPrefix(strings.TrimSpace(line), "#EXTINF:") {
			extinfLine := line
			i++
			var extraLines []string
			for i < linesLen {
				nextLine := lines[i]
				extraLines = append(extraLines, nextLine)
				if strings.HasPrefix(strings.TrimSpace(nextLine), "http") {
					i++
					break
				}
				i++
			}
			entries = append(entries, ChannelEntry{EXTINFLine: extinfLine, ExtraLines: extraLines})
		} else {
			headers = append(headers, line)
			i++
		}
	}
	return headers, entries
}

// channelNameFromEntry extracts the channel name (part after comma) from an EXTINF line.
func channelNameFromEntry(entry ChannelEntry) string {
	parts := strings.SplitN(entry.EXTINFLine, ",", 2)
	if len(parts) > 1 {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

// ─── Emoji helpers ────────────────────────────────────────────────────────────────

var emojiPoolA = []string{
	"🔴", "🟠", "🟡", "🟢", "🔵", "🟣", "⚫", "⚪",
	"🔶", "🔷", "🔸", "🔹", "🔺", "🔻", "⬛", "⬜",
	"🔘", "⭕", "💠", "💮", "❓", "💢", "💥", "💫",
	"🌟", "⭐", "✨", "🔥", "💧", "🌊", "☀️", "🌙",
	"☁️", "⚡", "🌀", "🌈", "💨", "❄️", "🌪", "🌫",
}

var emojiPoolB = []string{
	"🐶", "🐱", "🐼", "🐯", "🦁", "🦊", "🐻", "🐨",
	"🐰", "🦄", "🐸", "🐵", "🦋", "🐝", "🐞", "🐌",
	"🌻", "🌺", "🌹", "🌸", "🌴", "🍀", "🌿", "🍁",
	"🎯", "🏆", "🎮", "🎸", "🎺", "🎻", "🥁", "📺",
	"🎬", "🎵", "🎶", "🚀", "🛸", "🎪", "🎭", "🎨",
}

func urlToEmojiPair(url string) string {
	h := fnv.New64a()
	h.Write([]byte(url))
	sum := h.Sum64()
	a := emojiPoolA[sum%uint64(len(emojiPoolA))]
	b := emojiPoolB[(sum>>32)%uint64(len(emojiPoolB))]
	return a + b
}

// ─── Downloads ────────────────────────────────────────────────────────────────────

func DownloadM3U(url string) (string, error) {
	return DownloadM3UWithContext(nil, url, false)
}

// DownloadM3UWithContext downloads M3U with context support for cancellation.
func DownloadM3UWithContext(ctx context.Context, url string, skipSSLVerify bool) (string, error) {
	logger.Info("Downloading M3U file from: %s", url)
	if ctx == nil {
		ctx = context.Background()
	}
	data, err := utils.DownloadFileWithContext(ctx, url, config.MaxM3UFileSize, skipSSLVerify)
	if err != nil {
		logger.Error("Error downloading M3U file: %v", err)
		return "", err
	}
	content := string(data)
	logger.Info("M3U file downloaded successfully, size: %d characters", len(content))
	return content, nil
}

// ─── Normalization ────────────────────────────────────────────────────────────────

func isEmojiRune(r rune) bool {
	// Regional indicators (flags) — NOT covered by 0x1F300+ range.
	if r >= 0x1F1E0 && r <= 0x1F1FF {
		return true
	}
	// Main emoji block: misc symbols, pictographs, emoticons, transport.
	if r >= 0x1F300 && r <= 0x1F9FF {
		return true
	}
	// Additional ranges outside the main emoji block.
	switch {
	case r >= 0x2600 && r <= 0x27BF:
		return true
	case r >= 0xFE00 && r <= 0xFE0F:
		return true
	case r == 0x200D:
		return true
	case r == 0x2B50 || r == 0x2B55:
		return true
	case r >= 0x20E3 && r <= 0x20E3:
		return true
	case r >= 0x231A && r <= 0x231B:
		return true
	case r >= 0x23F0 && r <= 0x23F3:
		return true
	case r == 0x00A9 || r == 0x00AE:
		return true
	case r == 0x2122:
		return true
	case r == 0x3030 || r == 0x303D:
		return true
	case r == 0x3297 || r == 0x3299:
		return true
	default:
		return false
	}
}

func removeTrailingEmojiAndSymbols(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	end := len(runes)
	for end > 0 {
		r := runes[end-1]
		if isEmojiRune(r) || r == ' ' || r == '\t' || r == '\u00A0' {
			end--
		} else {
			break
		}
	}
	return string(runes[:end])
}

// RemoveOrigSuffix strips trailing " orig" (case-insensitive) from channel name.
func RemoveOrigSuffix(name string) string {
	if len(name) >= 5 && strings.HasSuffix(strings.ToLower(name), " orig") {
		return name[:len(name)-5]
	}
	return name
}

// ─── Pipeline: header processing ─────────────────────────────────────────────────

func processHeader(line, customEPGURL string) string {
	if customEPGURL == "" {
		return line
	}
	lineLower := strings.ToLower(line)
	if regTVGURL.MatchString(lineLower) {
		line = regTVGURL.ReplaceAllString(line, fmt.Sprintf(`tvg-url="%s"`, customEPGURL))
	}
	if regURLTVG.MatchString(lineLower) {
		line = regURLTVG.ReplaceAllString(line, fmt.Sprintf(`url-tvg="%s"`, customEPGURL))
	} else {
		if strings.HasSuffix(line, ">") {
			line = line[:len(line)-1] + fmt.Sprintf(` url-tvg="%s">`, customEPGURL)
		} else {
			line += fmt.Sprintf(` url-tvg="%s"`, customEPGURL)
		}
	}
	return line
}

// ─── Pipeline: category filtering ────────────────────────────────────────────────

func normalizeGroupTitle(group string) string {
	normalized := regLeadingNumber.ReplaceAllString(group, "")
	normalized = removeTrailingEmojiAndSymbols(normalized)
	return normalized
}

func shouldFilterByCategory(groupTitle string, exactMatchLower, substringLower []string) bool {
	if len(exactMatchLower) == 0 && len(substringLower) == 0 {
		return false
	}
	groupLower := strings.ToLower(groupTitle)

	for _, cat := range exactMatchLower {
		if cat == groupLower {
			return true
		}
	}
	for _, substr := range substringLower {
		if strings.Contains(groupLower, substr) {
			return true
		}
	}
	return false
}

func shouldFilterByName(channelName string, excludeLower []string) bool {
	if len(excludeLower) == 0 {
		return false
	}
	cnLower := strings.ToLower(channelName)
	for _, pat := range excludeLower {
		if strings.Contains(cnLower, pat) {
			return true
		}
	}
	return false
}

func isRegionalChannel(name string) bool {
	return regRegional.MatchString(name)
}

func hasNumericSuffix(name string) bool {
	return regNumberSuffix.MatchString(name)
}

// ─── Pipeline: single entry filtering ───────────────────────────────────────────

type filterResult struct {
	filteredLine string
	keep         bool
}

func filterEntry(line string, exactMatchLower, substringLower, excludeLower []string) filterResult {
	// Default: keep if no filters configured for the category.
	keep := len(exactMatchLower) == 0 && len(substringLower) == 0

	if m := regGroupTitle.FindStringSubmatch(line); m != nil {
		originalGroup := m[1]
		normalized := normalizeGroupTitle(originalGroup)

		// Update line with cleaned group-title if it changed.
		if normalized != originalGroup {
			newAttr := fmt.Sprintf(`group-title="%s"`, normalized)
			line = regGroupTitleAttr.ReplaceAllString(line, newAttr)
		}

		keep = !shouldFilterByCategory(normalized, exactMatchLower, substringLower)
	}

	if !keep {
		return filterResult{keep: false}
	}

	// Channel name checks.
	parts := strings.SplitN(line, ",", 2)
	if len(parts) > 1 {
		channelName := strings.TrimSpace(parts[1])

		if shouldFilterByName(channelName, excludeLower) {
			return filterResult{keep: false}
		}
		if isRegionalChannel(channelName) {
			return filterResult{keep: false}
		}
		if hasNumericSuffix(channelName) {
			return filterResult{keep: false}
		}

		// Remove "orig" suffix.
		newName := RemoveOrigSuffix(channelName)
		if newName != channelName {
			line = parts[0] + "," + newName
		}
	}

	return filterResult{filteredLine: line, keep: true}
}

// ─── Pipeline: post-processing ───────────────────────────────────────────────────

// RemoveDuplicateURLs removes entries with duplicate URLs, merging attributes.
func RemoveDuplicateURLs(content string) string {
	lines := strings.Split(content, "\n")
	headers, entries := ParseChannelEntries(lines)

	urlGroups := make(map[string][]ChannelEntry)
	var noURLEntries []ChannelEntry

	for _, entry := range entries {
		url := ""
		for _, extraLine := range entry.ExtraLines {
			trimmed := strings.TrimSpace(extraLine)
			if strings.HasPrefix(trimmed, "http") {
				url = trimmed
				break
			}
		}
		if url != "" {
			urlGroups[url] = append(urlGroups[url], entry)
		} else {
			noURLEntries = append(noURLEntries, entry)
		}
	}

	var dedupedEntries []ChannelEntry
	totalRemoved := 0

	for _, group := range urlGroups {
		if len(group) <= 1 {
			dedupedEntries = append(dedupedEntries, group...)
			continue
		}

		mergedTvgID := ""
		mergedGroupTitle := ""
		mergedTvgLogo := ""
		mergedTvgRec := ""
		longestName := ""

		for _, entry := range group {
			extinfLine := entry.EXTINFLine

			if m := regTvgID.FindStringSubmatch(extinfLine); len(m) > 1 && m[1] != "" && mergedTvgID == "" {
				mergedTvgID = m[1]
			}
			if m := regGroupTitle.FindStringSubmatch(extinfLine); len(m) > 1 && m[1] != "" && mergedGroupTitle == "" {
				mergedGroupTitle = m[1]
			}
			if m := regTvgLogo.FindStringSubmatch(extinfLine); len(m) > 1 && m[1] != "" && mergedTvgLogo == "" {
				mergedTvgLogo = m[1]
			}
			if m := regTvgRec.FindStringSubmatch(extinfLine); len(m) > 1 && m[1] != "" && mergedTvgRec == "" {
				mergedTvgRec = m[1]
			}

			parts := strings.SplitN(extinfLine, ",", 2)
			if len(parts) > 1 {
				name := strings.TrimSpace(parts[1])
				if len(name) > len(longestName) {
					longestName = name
				}
			}
		}

		firstEntry := group[0]
		extinfPart := strings.SplitN(firstEntry.EXTINFLine, ",", 2)[0]

		extinfPart = regGroupTitleAttr.ReplaceAllString(extinfPart, "")
		extinfPart = regTvgIDAttr.ReplaceAllString(extinfPart, "")
		extinfPart = regTvgLogoAttr.ReplaceAllString(extinfPart, "")
		extinfPart = regTvgRecAttr.ReplaceAllString(extinfPart, "")
		extinfPart = strings.TrimSpace(extinfPart)

		if mergedGroupTitle != "" {
			extinfPart += fmt.Sprintf(` group-title="%s"`, mergedGroupTitle)
		}
		if mergedTvgID != "" {
			extinfPart += fmt.Sprintf(` tvg-id="%s"`, mergedTvgID)
		}
		if mergedTvgLogo != "" {
			extinfPart += fmt.Sprintf(` tvg-logo="%s"`, mergedTvgLogo)
		}
		if mergedTvgRec != "" {
			extinfPart += fmt.Sprintf(` tvg-rec="%s"`, mergedTvgRec)
		}

		dedupedEntries = append(dedupedEntries, ChannelEntry{
			EXTINFLine: extinfPart + "," + longestName,
			ExtraLines: firstEntry.ExtraLines,
		})
		totalRemoved += len(group) - 1
	}

	dedupedEntries = append(dedupedEntries, noURLEntries...)

	if totalRemoved > 0 {
		logger.Info("Removed %d duplicate URLs from playlist", totalRemoved)
	}

	var finalLines []string
	finalLines = append(finalLines, headers...)
	for _, entry := range dedupedEntries {
		finalLines = append(finalLines, entry.EXTINFLine)
		finalLines = append(finalLines, entry.ExtraLines...)
	}
	return strings.Join(finalLines, "\n")
}

// SortPlaylistAlphabetically sorts playlist entries alphabetically by channel name.
func SortPlaylistAlphabetically(content string) string {
	lines := strings.Split(content, "\n")
	headers, entries := ParseChannelEntries(lines)

	sort.SliceStable(entries, func(i, j int) bool {
		nameI := channelNameFromEntry(entries[i])
		nameJ := channelNameFromEntry(entries[j])
		return strings.ToLower(nameI) < strings.ToLower(nameJ)
	})

	var finalLines []string
	finalLines = append(finalLines, headers...)
	for _, entry := range entries {
		finalLines = append(finalLines, entry.EXTINFLine)
		finalLines = append(finalLines, entry.ExtraLines...)
	}
	return strings.Join(finalLines, "\n")
}

// AddEmojiByURL adds a unique emoji pair to every channel name based on its URL.
func AddEmojiByURL(content string) string {
	lines := strings.Split(content, "\n")
	headers, entries := ParseChannelEntries(lines)

	for i, entry := range entries {
		url := ""
		for _, extraLine := range entry.ExtraLines {
			trimmed := strings.TrimSpace(extraLine)
			if strings.HasPrefix(trimmed, "http") {
				url = trimmed
				break
			}
		}

		parts := strings.SplitN(entry.EXTINFLine, ",", 2)
		if len(parts) > 1 {
			chName := strings.TrimSpace(parts[1])
			emoji := urlToEmojiPair(url)
			newName := fmt.Sprintf("%s %s", chName, emoji)
			entries[i].EXTINFLine = parts[0] + "," + newName
		}
	}

	var finalLines []string
	finalLines = append(finalLines, headers...)
	for _, entry := range entries {
		finalLines = append(finalLines, entry.EXTINFLine)
		finalLines = append(finalLines, entry.ExtraLines...)
	}
	return strings.Join(finalLines, "\n")
}

// AddTvgIDsToPlaylist adds tvg-id from EPG name-to-id map to channels that lack it.
func AddTvgIDsToPlaylist(content string, epgNameToIDMap map[string]string) string {
	lines := strings.Split(content, "\n")
	addedCount := 0

	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "#EXTINF:") {
			if regTvgID.MatchString(line) {
				continue
			}

			parts := strings.SplitN(line, ",", 2)
			if len(parts) > 1 {
				channelName := strings.TrimSpace(parts[1])
				normalizedName := strings.ToLower(channelName)

				if tvgID, ok := epgNameToIDMap[normalizedName]; ok {
					extinfPart := parts[0]
					lines[i] = fmt.Sprintf(`%s tvg-id="%s",%s`, extinfPart, tvgID, parts[1])
					addedCount++
				}
			}
		}
	}

	if addedCount > 0 {
		logger.Info("Added tvg-id to %d channels", addedCount)
	}
	return strings.Join(lines, "\n")
}

// ─── Main pipeline ───────────────────────────────────────────────────────────────

// FilterContent is the main pipeline: normalize → filter → dedup → sort → add emoji.
func FilterContent(content string, categoriesToRemove, categoriesToRemoveSubstring, channelNamesToExclude []string, customEPGURL string) string {
	logger.Info("Starting filtering process")

	// Normalize line endings.
	content = utils.NormalizeLineEndings(content)

	// Pre-lowercase filter lists for faster matching.
	exactMatchLower := utils.ToLowerSlice(categoriesToRemove)
	substringLower := utils.ToLowerSlice(categoriesToRemoveSubstring)
	excludeLower := utils.ToLowerSlice(channelNamesToExclude)

	lines := strings.Split(content, "\n")
	var filteredLines []string

	for _, line := range lines {
		if len(line) > 10000 {
			continue
		}

		trimmed := strings.TrimSpace(line)

		// Header line: process EPG URL and always keep.
		if strings.HasPrefix(trimmed, "#EXTM3U") {
			filteredLines = append(filteredLines, processHeader(line, customEPGURL))
			continue
		}

		// Channel entry: filter and normalize.
		if strings.HasPrefix(trimmed, "#EXTINF:") {
			result := filterEntry(line, exactMatchLower, substringLower, excludeLower)
			if result.keep {
				filteredLines = append(filteredLines, result.filteredLine)
			}
			continue
		}

		// URL or other lines: keep if the previous entry was included.
		if strings.HasPrefix(trimmed, "http") {
			// We track inclusion via a different approach: we append URLs
			// only when they follow a kept EXTINF line. But since we're
			// iterating line-by-line, we need to check if the last kept
			// line was an EXTINF without a URL.
			// Simpler: just append all non-EXTINF lines; the subsequent
			// dedup step will handle orphans. But that's not ideal.
			//
			// Better approach: re-add line pairing. Since we removed
			// the includeEntry flag, let's use a simple heuristic:
			// a URL line after a kept EXTINF line should be kept.
			// We'll check if the last added line was an EXTINF (no URL follows it yet).
			if len(filteredLines) > 0 {
				lastLine := filteredLines[len(filteredLines)-1]
				if strings.HasPrefix(lastLine, "#EXTINF:") {
					filteredLines = append(filteredLines, line)
				}
			}
			continue
		}

		// Empty lines and other headers: always keep.
		if trimmed == "" || strings.HasPrefix(trimmed, "#EXTM3U") {
			filteredLines = append(filteredLines, line)
			continue
		}

		// Other non-extinf lines: keep as-is (they'll be associated with an entry later).
		// This catches things like #KODIPROP lines.
		// Only keep if they follow an EXTINF line.
		if len(filteredLines) > 0 {
			lastLine := filteredLines[len(filteredLines)-1]
			if strings.HasPrefix(lastLine, "#EXTINF:") {
				filteredLines = append(filteredLines, line)
			}
		}
	}

	contentNoDups := RemoveDuplicateURLs(strings.Join(filteredLines, "\n"))
	processed := SortPlaylistAlphabetically(contentNoDups)
	processed = AddEmojiByURL(processed)

	logger.Info("Filtering complete: %d channels -> %d channels", CountChannels(content), CountChannels(processed))
	logger.Info("Filtering process completed")
	return processed
}

// CountChannels counts #EXTINF entries in M3U content.
func CountChannels(content string) int {
	count := 0
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#EXTINF:") {
			count++
		}
	}
	return count
}

// ─── Categories file handling ─────────────────────────────────────────────────────

// ParseCategoriesFile reads categories.txt and returns a map of lowercase channel name → {group, tvg_id}.
func ParseCategoriesFile(filePath string) map[string]map[string]string {
	mapping := make(map[string]map[string]string)

	data, err := os.ReadFile(filePath)
	if err != nil {
		logger.Warning("Categories file not found: %s", filePath)
		return mapping
	}
	logger.Info("Categories file loaded: %s (%d bytes)", filePath, len(data))

	regCategoriesFile := regexp.MustCompile(`group-title="([^"]+)".*?tvg-id="([^"]+)".+?,(.+)`)
	matches := regCategoriesFile.FindAllStringSubmatch(string(data), -1)
	for _, m := range matches {
		nameLower := strings.ToLower(strings.TrimSpace(m[3]))
		if _, ok := mapping[nameLower]; !ok {
			mapping[nameLower] = map[string]string{
				"group":  m[1],
				"tvg_id": m[2],
			}
		}
	}

	logger.Info("Parsed categories file: %d unique channel mappings", len(mapping))
	return mapping
}

// ApplyChannelMetadata overrides group-title and tvg-id for channels listed in categories.txt.
func ApplyChannelMetadata(content string, categoriesMapping map[string]map[string]string) string {
	lines := strings.Split(content, "\n")
	updatedGroup := 0
	updatedTvgID := 0

	for i, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), "#EXTINF:") {
			continue
		}
		parts := strings.SplitN(line, ",", 2)
		if len(parts) < 2 {
			continue
		}

		channelName := strings.TrimSpace(parts[1])
		meta, ok := categoriesMapping[strings.ToLower(channelName)]
		if !ok {
			continue
		}

		extinfPart := parts[0]

		if regGroupTitleAttr.MatchString(extinfPart) {
			extinfPart = regGroupTitleAttr.ReplaceAllString(extinfPart, fmt.Sprintf(`group-title="%s"`, meta["group"]))
		} else {
			extinfPart += fmt.Sprintf(` group-title="%s"`, meta["group"])
		}
		updatedGroup++

		if regTvgIDAttr.MatchString(extinfPart) {
			extinfPart = regTvgIDAttr.ReplaceAllString(extinfPart, fmt.Sprintf(`tvg-id="%s"`, meta["tvg_id"]))
		} else {
			extinfPart += fmt.Sprintf(` tvg-id="%s"`, meta["tvg_id"])
		}
		updatedTvgID++

		lines[i] = extinfPart + "," + parts[1]
	}

	if updatedGroup > 0 || updatedTvgID > 0 {
		logger.Info("Updated metadata: %d group-title, %d tvg-id from categories file", updatedGroup, updatedTvgID)
	}
	return strings.Join(lines, "\n")
}
