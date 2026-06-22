package m3u

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/ozyab/iptv/internal/config"
	"github.com/ozyab/iptv/internal/utils"
)

var logger = utils.NewSanitizedLoggerWithPrefix("[m3u]")

func DownloadM3U(url string) (string, error) {
	logger.Info("Downloading M3U file from: %s", url)

	resp, err := utils.HTTPClient.Get(url)
	if err != nil {
		logger.Error("Error downloading M3U file: %v", err)
		return "", err
	}
	defer resp.Body.Close()

	var chunks []byte
	totalSize := 0
	buf := make([]byte, 8192)

	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			totalSize += n
			if totalSize > config.MaxM3UFileSize {
				return "", fmt.Errorf("M3U file exceeds maximum allowed size of %d bytes", config.MaxM3UFileSize)
			}
			chunks = append(chunks, buf[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			logger.Error("Error downloading M3U file: %v", err)
			return "", err
		}
	}

	content := string(chunks)
	logger.Info("M3U file downloaded successfully, size: %d characters", len(content))
	return content, nil
}

func RemoveOrigSuffix(name string) string {
	if len(name) >= 5 && strings.HasSuffix(strings.ToLower(name), " orig") {
		return name[:len(name)-5]
	}
	return name
}

func GetBaseChannelName(name string) string {
	temp := name
	for {
		changed := false
		lower := strings.ToLower(temp)
		if strings.HasSuffix(lower, " orig") {
			temp = strings.TrimSpace(temp[:len(temp)-5])
			changed = true
		} else if strings.HasSuffix(lower, " hd") {
			temp = strings.TrimSpace(temp[:len(temp)-3])
			changed = true
		}
		if !changed {
			break
		}
	}
	return temp
}

var regRegional = regexp.MustCompile(`\s\+\d+(?:\s+HD)?(?:\s*\([^)]+\))?\s*$`)
var regNumberSuffix = regexp.MustCompile(`\s\d{2,}$`)
var regGroupTitle = regexp.MustCompile(`group-title="([^"]*)"`)
var regTvgID = regexp.MustCompile(`tvg-id="([^"]*)"`)
var regURLTVG = regexp.MustCompile(`url-tvg="[^"]*"`)

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
	hasEXTINF := false

	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "#EXTINF:") {
			hasEXTINF = true
			break
		}
	}

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

					if includeEntry && regNumberSuffix.MatchString(channelName) {
						keepAll := config.ChannelsKeepAllVariants
						normalized := NormalizeNameForComparison(channelName)
						shouldKeep := false
						for _, k := range keepAll {
							if normalized == k {
								shouldKeep = true
								break
							}
						}
						if !shouldKeep {
							includeEntry = false
						}
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

		if strings.HasPrefix(trimmed, "http") {
			if !hasEXTINF {
				if len(categoriesLower) == 0 {
					filteredLines = append(filteredLines, line)
				}
			} else if includeEntry {
				filteredLines = append(filteredLines, line)
			}
		} else if includeEntry {
			filteredLines = append(filteredLines, line)
		} else {
			if !strings.HasPrefix(trimmed, "#EXTINF:") && !includeEntry {
				if strings.HasPrefix(trimmed, "#EXTM3U") || trimmed == "" {
					filteredLines = append(filteredLines, line)
				} else if !hasEXTINF && trimmed != "" {
					filteredLines = append(filteredLines, line)
				}
			}
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

var suffixesToRemove = []string{
	`\bhd\b`,
	`\borig\b`,
	`\bsd\b`,
	`\bfull hd\b`,
	`\b4k\b`,
	`\buhd\b`,
	`\buhd tv\b`,
}

func NormalizeNameForComparison(name string) string {
	normalized := strings.ToLower(name)
	for _, suffix := range suffixesToRemove {
		re := regexp.MustCompile(`\s*` + suffix + `\s*`)
		normalized = re.ReplaceAllString(normalized, " ")
	}
	return strings.Join(strings.Fields(normalized), " ")
}

func ParseCategoriesFile(filePath string) map[string]map[string]string {
	mapping := make(map[string]map[string]string)

	data, err := os.ReadFile(filePath)
	if err != nil {
		logger.Warning("Categories file not found: %s", filePath)
		return mapping
	}

	content := string(data)
	pattern := regexp.MustCompile(`group-title="([^"]+)".*?tvg-id="([^"]+)".+?,(.+)`)
	matches := pattern.FindAllStringSubmatch(content, -1)

	for _, m := range matches {
		group := m[1]
		tvgID := m[2]
		name := strings.TrimSpace(m[3])
		nameLower := strings.ToLower(name)
		if _, ok := mapping[nameLower]; !ok {
			mapping[nameLower] = map[string]string{
				"group":  group,
				"tvg_id": tvgID,
			}
		}
	}

	logger.Info("Parsed categories file: %d unique channel mappings", len(mapping))
	return mapping
}

func ApplyChannelMetadata(content string, categoriesMapping map[string]map[string]string) string {
	lines := strings.Split(content, "\n")
	updatedGroup := 0
	updatedTvgID := 0

	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "#EXTINF:") {
			parts := strings.SplitN(line, ",", 2)
			if len(parts) < 2 {
				continue
			}

			channelName := strings.TrimSpace(parts[1])
			nameLower := strings.ToLower(channelName)

			meta, ok := categoriesMapping[nameLower]
			if !ok {
				continue
			}

			extinfPart := parts[0]

			gtPattern := regexp.MustCompile(`group-title="[^"]*"`)
			if gtPattern.MatchString(strings.ToLower(extinfPart)) {
				extinfPart = gtPattern.ReplaceAllString(extinfPart, fmt.Sprintf(`group-title="%s"`, meta["group"]))
				updatedGroup++
			} else {
				extinfPart += fmt.Sprintf(` group-title="%s"`, meta["group"])
				updatedGroup++
			}

			tvgPattern := regexp.MustCompile(`tvg-id="[^"]*"`)
			if tvgPattern.MatchString(strings.ToLower(extinfPart)) {
				extinfPart = tvgPattern.ReplaceAllString(extinfPart, fmt.Sprintf(`tvg-id="%s"`, meta["tvg_id"]))
			} else {
				extinfPart += fmt.Sprintf(` tvg-id="%s"`, meta["tvg_id"])
			}
			updatedTvgID++

			lines[i] = extinfPart + "," + parts[1]
		}
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
