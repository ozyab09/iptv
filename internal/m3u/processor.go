package m3u

import (
	"fmt"
	"hash/fnv"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/ozyab/iptv/internal/config"
	"github.com/ozyab/iptv/internal/utils"
)

// Package-level logger with sanitized output (masks URLs/credentials).
var logger = utils.NewSanitizedLoggerWithPrefix("[m3u]")

// Pre-compiled regexps for efficient filtering.
var (
	regRegional       = regexp.MustCompile(`\s\+\d+(?:\s+HD)?(?:\s*\([^)]+\))?\s*$`) // e.g. " +1", " +4 HD", " +2 (Приволжье)"
	regNumberSuffix   = regexp.MustCompile(`\s\d{2,}$`)                                     // e.g. " HD 50", " 25"
	regLeadingNumber  = regexp.MustCompile(`^\d+\.\s*`)                                     // e.g. "15.", "20. ", "60."
	regGroupTitle     = regexp.MustCompile(`group-title="([^"]*)"`)                        // group-title attribute
	regTvgID          = regexp.MustCompile(`tvg-id="([^"]*)"`)                             // tvg-id attribute
	regURLTVG         = regexp.MustCompile(`url-tvg="[^"]*"`)                              // url-tvg attribute
	regCategoriesFile = regexp.MustCompile(`group-title="([^"]+)".*?tvg-id="([^"]+)".+?,(.+)`) // categories.txt parser
	regGroupTitleAttr = regexp.MustCompile(`group-title="[^"]*"`)                          // for replacement
	regTvgIDAttr      = regexp.MustCompile(`tvg-id="[^"]*"`)                              // for replacement
	regTvgLogo        = regexp.MustCompile(`tvg-logo="([^"]*)"`)                          // tvg-logo attribute (capture)
	regTvgRec         = regexp.MustCompile(`tvg-rec="([^"]*)"`)                           // tvg-rec attribute (capture)
	regTvgLogoAttr    = regexp.MustCompile(`tvg-logo="[^"]*"`)                            // for replacement
	regTvgRecAttr     = regexp.MustCompile(`tvg-rec="[^"]*"`)                             // for replacement
)



// DownloadM3U downloads an M3U playlist from url with a 100 MB size limit.
func DownloadM3U(url string) (string, error) {
	logger.Info("Downloading M3U file from: %s", url)
	data, err := utils.DownloadFile(url, config.MaxM3UFileSize)
	if err != nil {
		logger.Error("Error downloading M3U file: %v", err)
		return "", err
	}
	content := string(data)
	logger.Info("M3U file downloaded successfully, size: %d characters", len(content))
	return content, nil
}

// removeTrailingEmojiAndSymbols strips trailing emoji and decorative characters from a string.
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

// isEmojiRune checks whether a rune belongs to common emoji/decorative Unicode ranges.
func isEmojiRune(r rune) bool {
	switch {
	case r >= 0x1F300 && r <= 0x1F9FF: // Misc symbols, pictographs, emoticons, etc.
		return true
	case r >= 0x1F1E0 && r <= 0x1F1FF: // Regional indicator symbols (flags)
		return true
	case r >= 0x1F600 && r <= 0x1F64F: // Emoticons
		return true
	case r >= 0x1F680 && r <= 0x1F6FF: // Transport and map symbols
		return true
	case r >= 0x2600 && r <= 0x27BF: // Misc symbols, dingbats
		return true
	case r >= 0xFE00 && r <= 0xFE0F: // Variation selectors
		return true
	case r == 0x200D: // Zero-width joiner
		return true
	case r == 0x2B50 || r == 0x2B55: // Star, circle
		return true
	case r >= 0x20E3 && r <= 0x20E3: // Combining enclosing keycap
		return true
	case r >= 0x231A && r <= 0x231B: // Watch, hourglass
		return true
	case r >= 0x23F0 && r <= 0x23F3: // Alarm clock, hourglass done
		return true
	case r == 0x00A9 || r == 0x00AE: // ©, ®
		return true
	case r == 0x2122: // ™
		return true
	case r == 0x3030 || r == 0x303D: // Wavy dash, part alternation
		return true
	case r == 0x3297 || r == 0x3299: // ㊗, ㊙
		return true
	default:
		return false
	}
}

// RemoveOrigSuffix strips trailing " orig" (case-insensitive) from channel name.
func RemoveOrigSuffix(name string) string {
	if len(name) >= 5 && strings.HasSuffix(strings.ToLower(name), " orig") {
		return name[:len(name)-5]
	}
	return name
}

func FilterContent(content string, categoriesToRemove, categoriesToRemoveSubstring, channelNamesToExclude []string, customEPGURL string) string {
	logger.Info("Starting filtering process")

	categoriesLower := make([]string, len(categoriesToRemove))
	for i, c := range categoriesToRemove {
		categoriesLower[i] = strings.ToLower(c)
	}

	substringLower := make([]string, len(categoriesToRemoveSubstring))
	for i, s := range categoriesToRemoveSubstring {
		substringLower[i] = strings.ToLower(s)
	}

	excludeLower := make([]string, len(channelNamesToExclude))
	for i, c := range channelNamesToExclude {
		excludeLower[i] = strings.ToLower(c)
	}

	lines := strings.Split(content, "\n")
	var filteredLines []string
	includeEntry := false
	for _, line := range lines {
		if len(line) > 10000 {
			continue
		}

		trimmed := strings.TrimSpace(line)
		lineLower := strings.ToLower(line)

		if strings.HasPrefix(trimmed, "#EXTM3U") {
			if customEPGURL != "" {
				if regURLTVG.MatchString(lineLower) {
					line = regURLTVG.ReplaceAllString(line, fmt.Sprintf(`url-tvg="%s"`, customEPGURL))
				} else {
					if strings.HasSuffix(line, ">") {
						line = line[:len(line)-1] + fmt.Sprintf(` url-tvg="%s">`, customEPGURL)
					} else {
						line += fmt.Sprintf(` url-tvg="%s"`, customEPGURL)
					}
				}
			}
			filteredLines = append(filteredLines, line)
			continue
		}

		if strings.HasPrefix(trimmed, "#EXTINF:") {
			includeEntry = false

			if m := regGroupTitle.FindStringSubmatch(line); m != nil {
				// Normalize group-title: strip leading numbers + trailing emojis.
				originalGroup := m[1]
				normalized := regLeadingNumber.ReplaceAllString(originalGroup, "")
				normalized = removeTrailingEmojiAndSymbols(normalized)
				groupTitle := strings.ToLower(normalized)

				// Update line with cleaned group-title if it changed.
				if normalized != originalGroup {
					newAttr := fmt.Sprintf(`group-title="%s"`, normalized)
					line = regGroupTitleAttr.ReplaceAllString(line, newAttr)
				}

				keep := true

				// Exact match against deny-list.
				if keep && len(categoriesLower) > 0 {
					for _, cat := range categoriesLower {
						if cat == groupTitle {
							keep = false
							break
						}
					}
				}

				// Substring match against deny-list.
				if keep && len(substringLower) > 0 {
					for _, substr := range substringLower {
						if strings.Contains(groupTitle, substr) {
							keep = false
							break
						}
					}
				}

				includeEntry = keep
			} else if len(categoriesLower) == 0 && len(substringLower) == 0 {
				// No category filtering configured — include all channels.
				includeEntry = true
			}

			if includeEntry {
				parts := strings.SplitN(line, ",", 2)
				if len(parts) > 1 {
					channelName := strings.TrimSpace(parts[1])

					if len(excludeLower) > 0 {
						cnLower := strings.ToLower(channelName)
						excluded := false
						for _, pat := range excludeLower {
							if strings.Contains(cnLower, pat) {
								excluded = true
								break
							}
						}
						if excluded {
							includeEntry = false
						}
					}

					if includeEntry && regRegional.MatchString(channelName) {
						includeEntry = false
					}

					// Remove channels with numeric suffixes (e.g. "HD 50", "Channel 25").
					// These are usually regional/time-shifted duplicates.
					if includeEntry && regNumberSuffix.MatchString(channelName) {
						includeEntry = false
					}
				}
			}

			if includeEntry {
				parts := strings.SplitN(line, ",", 2)
				if len(parts) > 1 {
					channelName := strings.TrimSpace(parts[1])
					newName := RemoveOrigSuffix(channelName)
					line = parts[0] + "," + newName
				}
				filteredLines = append(filteredLines, line)
			}
			continue
		}

			// Append URL or other non-EXTINF line only when the entry is included.
		// Empty lines and #EXTM3U headers are always kept.
		if strings.HasPrefix(trimmed, "http") {
			if includeEntry {
				filteredLines = append(filteredLines, line)
			}
		} else if includeEntry || trimmed == "" || strings.HasPrefix(trimmed, "#EXTM3U") {
			filteredLines = append(filteredLines, line)
		}
	}

	contentNoDups := RemoveDuplicateURLs(strings.Join(filteredLines, "\n"))
	processed := SortPlaylistAlphabetically(contentNoDups)
	processed = AddEmojiByURL(processed)
	origCh := CountChannels(content)
	procCh := CountChannels(processed)
	logger.Info("Filtering complete: %d channels -> %d channels", origCh, procCh)
	logger.Info("Filtering process completed")
	return processed
}

func CountChannels(content string) int {
	count := 0
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#EXTINF:") {
			count++
		}
	}
	return count
}

type ChannelEntry struct {
	EXTINFLine string
	ExtraLines []string
}

func ParseChannelEntries(lines []string) ([]string, []ChannelEntry) {
	var headers []string
	var entries []ChannelEntry

	i := 0
	for i < len(lines) {
		line := lines[i]
		if strings.HasPrefix(strings.TrimSpace(line), "#EXTINF:") {
			extinfLine := line
			i++
			var extraLines []string
			for i < len(lines) {
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

// ParseCategoriesFile reads categories.txt and returns a map of lowercase channel name → {group, tvg_id}.
func ParseCategoriesFile(filePath string) map[string]map[string]string {
	mapping := make(map[string]map[string]string)

	data, err := os.ReadFile(filePath)
	if err != nil {
		logger.Warning("Categories file not found: %s", filePath)
		return mapping
	}

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

		// Override or add group-title attribute.
		if regGroupTitleAttr.MatchString(extinfPart) {
			extinfPart = regGroupTitleAttr.ReplaceAllString(extinfPart, fmt.Sprintf(`group-title="%s"`, meta["group"]))
		} else {
			extinfPart += fmt.Sprintf(` group-title="%s"`, meta["group"])
		}
		updatedGroup++

		// Override or add tvg-id attribute.
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

// RemoveDuplicateURLs removes entries with duplicate URLs, keeping only one entry per unique URL.
// For duplicate entries, it merges attributes (tvg-id, group-title, tvg-logo, tvg-rec) by taking
// the first non-empty value and keeps the longest channel name.
func RemoveDuplicateURLs(content string) string {
	lines := strings.Split(content, "\n")
	headers, entries := ParseChannelEntries(lines)

	// Group entries by URL.
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

		// Merge attributes: take first non-empty value from any entry.
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

			// Longest channel name wins.
			parts := strings.SplitN(extinfLine, ",", 2)
			if len(parts) > 1 {
				name := strings.TrimSpace(parts[1])
				if len(name) > len(longestName) {
					longestName = name
				}
			}
		}

		// Build merged EXTINF line from the first entry's structure.
		firstEntry := group[0]
		extinfPart := strings.SplitN(firstEntry.EXTINFLine, ",", 2)[0]

		// Remove all existing attribute values.
		extinfPart = regGroupTitleAttr.ReplaceAllString(extinfPart, "")
		extinfPart = regTvgIDAttr.ReplaceAllString(extinfPart, "")
		extinfPart = regTvgLogoAttr.ReplaceAllString(extinfPart, "")
		extinfPart = regTvgRecAttr.ReplaceAllString(extinfPart, "")

		// Clean up extra whitespace.
		extinfPart = strings.TrimSpace(extinfPart)

		// Add merged attributes in consistent order.
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

		mergedLine := extinfPart + "," + longestName

		dedupedEntries = append(dedupedEntries, ChannelEntry{
			EXTINFLine: mergedLine,
			ExtraLines: firstEntry.ExtraLines,
		})
		totalRemoved += len(group) - 1
	}

	// Append entries without URLs (kept as-is).
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

// SortPlaylistAlphabetically sorts playlist entries alphabetically by channel name (case-insensitive).
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

// channelNameFromEntry extracts the channel name (part after comma) from an EXTINF line.
func channelNameFromEntry(entry ChannelEntry) string {
	parts := strings.SplitN(entry.EXTINFLine, ",", 2)
	if len(parts) > 1 {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

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

// emojiPoolA are visually distinct shape/color/symbol emojis used as the first component
// of the emoji pair identifier. The pair provides ~1500+ unique combinations.
var emojiPoolA = []string{
	"🔴", "🟠", "🟡", "🟢", "🔵", "🟣", "⚫", "⚪",
	"🔶", "🔷", "🔸", "🔹", "🔺", "🔻", "⬛", "⬜",
	"🔘", "⭕", "💠", "💮", "❓", "💢", "💥", "💫",
	"🌟", "⭐", "✨", "🔥", "💧", "🌊", "☀️", "🌙",
	"☁️", "⚡", "🌀", "🌈", "💨", "❄️", "🌪", "🌫",
}

// emojiPoolB are visually distinct object/animal/nature emojis used as the second component.
var emojiPoolB = []string{
	"🐶", "🐱", "🐼", "🐯", "🦁", "🦊", "🐻", "🐨",
	"🐰", "🦄", "🐸", "🐵", "🦋", "🐝", "🐞", "🐌",
	"🌻", "🌺", "🌹", "🌸", "🌴", "🍀", "🌿", "🍁",
	"🎯", "🏆", "🎮", "🎸", "🎺", "🎻", "🥁", "📺",
	"🎬", "🎵", "🎶", "🚀", "🛸", "🎪", "🎭", "🎨",
}

// urlToEmojiPair generates a deterministic emoji pair (from two pools) based on the URL hash.
// Same URL always produces the same emoji pair. Uses FNV-1a 64-bit hash for speed and distribution.
func urlToEmojiPair(url string) string {
	h := fnv.New64a()
	h.Write([]byte(url))
	sum := h.Sum64()
	a := emojiPoolA[sum%uint64(len(emojiPoolA))]
	b := emojiPoolB[(sum>>32)%uint64(len(emojiPoolB))]
	return a + b
}

// AddEmojiByURL adds a unique emoji pair (e.g. 🔴🐱) to every channel name,
// deterministically derived from the channel's stream URL via FNV-1a hash.
// This replaces the previous sequential superscript numbering.
func AddEmojiByURL(content string) string {
	lines := strings.Split(content, "\n")
	headers, entries := ParseChannelEntries(lines)

	for i, entry := range entries {
		// Extract URL from ExtraLines.
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

