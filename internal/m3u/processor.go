package m3u

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/ozyab/iptv/internal/config"
	"github.com/ozyab/iptv/internal/utils"
)

// Package-level logger with sanitized output (masks URLs/credentials).
var logger = utils.NewSanitizedLoggerWithPrefix("[m3u]")

// Pre-compiled regexps for efficient filtering.
var (
	regRegional      = regexp.MustCompile(`\s\+\d+(?:\s+HD)?(?:\s*\([^)]+\))?\s*$`) // e.g. " +1", " +4 HD", " +2 (Приволжье)"
	regNumberSuffix  = regexp.MustCompile(`\s\d{2,}$`)                                     // e.g. " HD 50", " 25"
	regGroupTitle    = regexp.MustCompile(`group-title="([^"]*)"`)                        // group-title attribute
	regTvgID         = regexp.MustCompile(`tvg-id="([^"]*)"`)                             // tvg-id attribute
	regURLTVG        = regexp.MustCompile(`url-tvg="[^"]*"`)                              // url-tvg attribute
	regCategoriesFile = regexp.MustCompile(`group-title="([^"]+)".*?tvg-id="([^"]+)".+?,(.+)`) // categories.txt parser
	regGroupTitleAttr = regexp.MustCompile(`group-title="[^"]*"`)                          // for replacement
	regTvgIDAttr     = regexp.MustCompile(`tvg-id="[^"]*"`)                              // for replacement
)

// suffixesToRemove lists patterns stripped during name normalization.
var suffixesToRemove = []*regexp.Regexp{
	regexp.MustCompile(`\s*\bhd\b\s*`),
	regexp.MustCompile(`\s*\borig\b\s*`),
	regexp.MustCompile(`\s*\bsd\b\s*`),
	regexp.MustCompile(`\s*\bfull hd\b\s*`),
	regexp.MustCompile(`\s*\b4k\b\s*`),
	regexp.MustCompile(`\s*\buhd\b\s*`),
	regexp.MustCompile(`\s*\buhd tv\b\s*`),
}

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

// RemoveOrigSuffix strips trailing " orig" (case-insensitive) from channel name.
func RemoveOrigSuffix(name string) string {
	if len(name) >= 5 && strings.HasSuffix(strings.ToLower(name), " orig") {
		return name[:len(name)-5]
	}
	return name
}

func FilterContent(content string, categoriesToRemove, channelNamesToExclude []string, customEPGURL string) string {
	logger.Info("Starting filtering process")

	categoriesLower := make([]string, len(categoriesToRemove))
	for i, c := range categoriesToRemove {
		categoriesLower[i] = strings.ToLower(c)
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

			if len(categoriesLower) > 0 {
				if m := regGroupTitle.FindStringSubmatch(line); m != nil {
					groupTitle := strings.ToLower(m[1])
					keep := true
					for _, cat := range categoriesLower {
						if cat == groupTitle {
							keep = false
							break
						}
					}
					includeEntry = keep
				}
			} else {
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

	processed := RemoveDuplicatesAndApplyHDPref(strings.Join(filteredLines, "\n"))
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

// NormalizeNameForComparison strips HD/orig/SD/4K/UHD/FHD suffixes for deduplication matching.
func NormalizeNameForComparison(name string) string {
	normalized := strings.ToLower(name)
	for _, re := range suffixesToRemove {
		normalized = re.ReplaceAllString(normalized, " ")
	}
	return strings.Join(strings.Fields(normalized), " ")
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

func RemoveDuplicatesAndApplyHDPref(content string) string {
	lines := strings.Split(content, "\n")
	headers, entries := ParseChannelEntries(lines)

	grouped := make(map[string][]ChannelEntry)

	for _, entry := range entries {
		parts := strings.SplitN(entry.EXTINFLine, ",", 2)
		var channelName string
		if len(parts) > 1 {
			channelName = strings.TrimSpace(parts[1])
		}
		normalizedName := NormalizeNameForComparison(channelName)
		grouped[normalizedName] = append(grouped[normalizedName], entry)
	}

	var result []ChannelEntry
	for key, variants := range grouped {
		if len(variants) > 1 {
			for idx, v := range variants {
				parts := strings.SplitN(v.EXTINFLine, ",", 2)
				if len(parts) > 1 {
					chName := strings.TrimSpace(parts[1])
					newName := fmt.Sprintf("%s #%d", chName, idx+1)
					v.EXTINFLine = parts[0] + "," + newName
				}
				result = append(result, v)
			}
			logger.Info("Kept all %d variants for '%s' with numeric suffixes", len(variants), key)
		} else {
			result = append(result, variants...)
		}
	}

	var finalLines []string
	finalLines = append(finalLines, headers...)
	for _, entry := range result {
		finalLines = append(finalLines, entry.EXTINFLine)
		finalLines = append(finalLines, entry.ExtraLines...)
	}
	return strings.Join(finalLines, "\n")
}
