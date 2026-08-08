package epg

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ozyab/iptv/internal/config"
	"github.com/ozyab/iptv/internal/utils"
)

var logger = utils.NewSanitizedLoggerWithPrefix("[epg]")

// Pre-compiled regexps for EPG processing.
var (
	epgGTRegex   = regexp.MustCompile(`group-title="([^"]*)"`)
	epgTvgRegex  = regexp.MustCompile(`tvg-id="([^"]*)"`)
	epgTimeRegex = regexp.MustCompile(`(\d{4})(\d{2})(\d{2})(\d{2})(\d{2})(\d{2})\s+(\S+)`)
)

// XML structs for EPG (XMLTV format).
type TV struct {
	XMLName    xml.Name    `xml:"tv"`
	Channels   []Channel   `xml:"channel"`
	Programmes []Programme `xml:"programme"`
}

type Channel struct {
	ID          string        `xml:"id,attr"`
	DisplayName []DisplayName `xml:"display-name"`
	Icon        []Icon        `xml:"icon"`
	URL         string        `xml:"url,omitempty"`
}

type DisplayName struct {
	Lang  string `xml:"lang,attr"`
	Value string `xml:",chardata"`
}

type Icon struct {
	Src    string `xml:"src,attr"`
	Width  string `xml:"width,attr,omitempty"`
	Height string `xml:"height,attr,omitempty"`
}

type Programme struct {
	Channel  string     `xml:"channel,attr"`
	Start    string     `xml:"start,attr"`
	Stop     string     `xml:"stop,attr"`
	Title    []Title    `xml:"title"`
	Desc     []Desc     `xml:"desc"`
	Category []Category `xml:"category"`
	Icon     []Icon     `xml:"icon"`
	Rating   []Rating   `xml:"rating"`
}

type Title struct {
	Lang  string `xml:"lang,attr"`
	Value string `xml:",chardata"`
}

type Desc struct {
	Lang  string `xml:"lang,attr"`
	Value string `xml:",chardata"`
}

type Category struct {
	Lang  string `xml:"lang,attr"`
	Value string `xml:",chardata"`
}

type Rating struct {
	System string `xml:"system,attr"`
	Value  string `xml:"value"`
}

// DownloadEPG downloads and decompresses (gz/zip) EPG content.
func DownloadEPG(ctx context.Context, urlStr string, cfg *config.Config) (string, error) {
	logger.Info("Downloading EPG file from: %s", urlStr)

	rawContent, err := utils.DownloadFileWithContext(ctx, urlStr, config.MaxEPGFileSize, cfg.SkipSSLVerify())
	if err != nil {
		logger.Error("Error downloading EPG file: %v", err)
		return "", err
	}

	outputDir := cfg.OutputDir()
	os.MkdirAll(outputDir, 0755)
	parsedURL, _ := url.Parse(urlStr)
	fname := path.Base(parsedURL.Path)
	if fname == "" {
		fname = "downloaded_epg.xml"
	}
	originalFilePath := path.Join(outputDir, "original_"+fname)

	if err := os.WriteFile(originalFilePath, rawContent, 0644); err != nil {
		return "", fmt.Errorf("failed to save original EPG: %w", err)
	}
	if fi, err := os.Stat(originalFilePath); err == nil {
		logger.Info("Original EPG file saved as: %s (size: %.2f KB)", originalFilePath, float64(fi.Size())/1024)
	}

	var decompressed []byte
	var errDecompress error
	switch {
	case strings.HasSuffix(urlStr, ".gz") || utils.IsGzipped(rawContent):
		logger.Info("Detected gzipped EPG file, decompressing...")
		decompressed, errDecompress = utils.DecompressGZip(rawContent)
		if errDecompress != nil {
			return "", fmt.Errorf("failed to decompress gzip: %w", errDecompress)
		}
	case strings.HasSuffix(urlStr, ".zip"):
		logger.Info("Detected zipped EPG file, extracting...")
		decompressed, errDecompress = utils.DecompressZip(rawContent)
		if errDecompress != nil {
			return "", fmt.Errorf("failed to decompress zip: %w", errDecompress)
		}
	default:
		decompressed = rawContent
	}

	content := string(decompressed)
	logger.Info("EPG file downloaded successfully, size: %.2f KB", float64(len(decompressed))/1024)
	return content, nil
}

// ExtractChannelInfoFromPlaylist parses M3U EXTINF lines for channel IDs and names.
func ExtractChannelInfoFromPlaylist(playlistContent string) (map[string]string, map[string]string) {
	logger.Info("Extracting channel IDs and categories from playlist")

	channelIDs := make(map[string]string)
	channelNames := make(map[string]string)

	for _, line := range strings.Split(playlistContent, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#EXTINF:") {
			continue
		}

		var category string
		if m := epgGTRegex.FindStringSubmatch(line); m != nil {
			category = strings.TrimSpace(m[1])
		}

		if m := epgTvgRegex.FindStringSubmatch(line); m != nil {
			tvgID := strings.TrimSpace(m[1])
			if tvgID != "" {
				channelIDs[tvgID] = category
			}
		}

		parts := strings.SplitN(line, ",", 2)
		if len(parts) > 1 {
			// Срезаем эмодзи-пару, добавленную к имени при фильтрации,
			// чтобы имена матчились с EPG display-name (без эмодзи).
			chName := utils.StripTrailingEmoji(strings.TrimSpace(parts[1]))
			if chName != "" {
				channelNames[chName] = category
			}
		}
	}

	logger.Info("Found %d unique channel IDs and %d channel names in playlist", len(channelIDs), len(channelNames))
	return channelIDs, channelNames
}

// BuildEPGNameToIDMap creates a lowercase display-name → channel-id map from EPG XML.
func BuildEPGNameToIDMap(epgContent string) map[string]string {
	nameToID := make(map[string]string)

	var tv TV
	if err := xml.Unmarshal([]byte(epgContent), &tv); err != nil {
		logger.Error("Error parsing EPG XML for name-to-id map: %v", err)
		return nameToID
	}

	for _, ch := range tv.Channels {
		for _, dn := range ch.DisplayName {
			if dn.Value != "" {
				nameToID[strings.ToLower(strings.TrimSpace(dn.Value))] = ch.ID
			}
		}
	}

	logger.Info("Built EPG name-to-id map with %d entries", len(nameToID))
	return nameToID
}

// FilterEPGContent filters EPG by channel IDs/names, excludes categories/IDs, applies retention.
func FilterEPGContent(epgContent string, channelIDs map[string]string, excludedCategories, excludedChannelIDs []string, channelNames map[string]string, retentionDays int) (string, error) {
	logger.Info("Filtering EPG content for %d channel IDs and %d channel names", len(channelIDs), len(channelNames))

	if len(channelIDs) == 0 && len(channelNames) == 0 {
		logger.Warning("No channel IDs or names provided, returning empty EPG")
		return `<?xml version="1.0" encoding="UTF-8"?><tv></tv>`, nil
	}

	excludedCatLower := make([]string, len(excludedCategories))
	for i, c := range excludedCategories {
		excludedCatLower[i] = strings.ToLower(c)
	}
	excludedIDSet := make(map[string]bool)
	for _, id := range excludedChannelIDs {
		excludedIDSet[id] = true
	}

	chNamesNormalized := make(map[string]bool)
	chNameCatLower := make(map[string]string)
	for name, cat := range channelNames {
		// Защитно срезаем эмодзи-пары, если они попали в имена.
		normalized := strings.ToLower(utils.StripTrailingEmoji(strings.TrimSpace(name)))
		chNamesNormalized[normalized] = true
		if cat != "" {
			chNameCatLower[normalized] = cat
		}
	}

	// Pre-compute retention window to avoid time.Now() calls in the loop.
	now := time.Now()
	oneHourAgo := now.Add(-1 * time.Hour)
	retentionLater := now.AddDate(0, 0, retentionDays)

	// Single-pass streaming parse: channels come before programmes in XMLTV.
	var result TV
	result.XMLName = xml.Name{Local: "tv"}

	channelsToKeep := make(map[string]bool)
	checkedChannels := make(map[string]bool)
	epgChDisplayNames := make(map[string][]string)

	decoder := xml.NewDecoder(bytes.NewReader([]byte(epgContent)))
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("error parsing EPG XML token: %w", err)
		}

		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}

		switch se.Name.Local {
		case "channel":
			var ch Channel
			if err := decoder.DecodeElement(&ch, &se); err != nil {
				logger.Warning("Error decoding channel element: %v", err)
				continue
			}

			// Collect display names for name-based EPG matching.
			for _, dn := range ch.DisplayName {
				if dn.Value != "" {
					epgChDisplayNames[ch.ID] = append(epgChDisplayNames[ch.ID], strings.ToLower(strings.TrimSpace(dn.Value)))
				}
			}

			// Check if this channel should be kept by ID or name.
			_, matchedByID := channelIDs[ch.ID]
			if !matchedByID && len(chNamesNormalized) > 0 {
				for _, dn := range epgChDisplayNames[ch.ID] {
					if chNamesNormalized[dn] {
						matchedByID = true
						break
					}
				}
			}

			shouldKeep := matchedByID && !excludedIDSet[ch.ID]
			if shouldKeep {
				channelsToKeep[ch.ID] = true
				result.Channels = append(result.Channels, Channel{
					ID: ch.ID,
					DisplayName: func() []DisplayName {
						if len(ch.DisplayName) > 0 {
							return []DisplayName{{Lang: ch.DisplayName[0].Lang, Value: ch.DisplayName[0].Value}}
						}
						return nil
					}(),
				})
			}

		case "programme":
			chRef := ""
			for _, attr := range se.Attr {
				if attr.Name.Local == "channel" {
					chRef = attr.Value
					break
				}
			}

			if !channelsToKeep[chRef] {
				if err := decoder.Skip(); err != nil {
					logger.Warning("Error skipping programme element: %v", err)
				}
				continue
			}

			var prog Programme
			if err := decoder.DecodeElement(&prog, &se); err != nil {
				logger.Warning("Error decoding programme element: %v", err)
				continue
			}

			// Check exclusion by category or ID (once per channel).
			if !checkedChannels[prog.Channel] {
				checkedChannels[prog.Channel] = true
				catToCheck := channelIDs[prog.Channel]

				exclude := false
				if catToCheck != "" {
					catLower := strings.ToLower(catToCheck)
					for _, ec := range excludedCatLower {
						if catLower == ec {
							exclude = true
							break
						}
					}
				}
				if excludedIDSet[prog.Channel] {
					exclude = true
				}

				if exclude {
					delete(channelsToKeep, prog.Channel)
					for i, ch := range result.Channels {
						if ch.ID == prog.Channel {
							result.Channels = append(result.Channels[:i], result.Channels[i+1:]...)
							break
						}
					}
					checkedChannels[prog.Channel] = false
					continue
				}
			}

			if !channelsToKeep[prog.Channel] {
				continue
			}

			// Apply time retention filter.
			startMatch := epgTimeRegex.FindStringSubmatch(prog.Start)
			stopMatch := epgTimeRegex.FindStringSubmatch(prog.Stop)

			include := true
			if startMatch != nil && stopMatch != nil {
				startTime, err1 := parseEPGTime(startMatch)
				stopTime, err2 := parseEPGTime(stopMatch)
				if err1 == nil && err2 == nil {
					if stopTime.Before(oneHourAgo) || startTime.After(retentionLater) {
						include = false
					}
				}
			}

			if include {
				result.Programmes = append(result.Programmes, prog)
			}
		}
	}

	logger.Info("EPG content filtering: %d channels after exclusions, %d programmes retained",
		len(result.Channels), len(result.Programmes))

	out, err := xml.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal XML: %w", err)
	}

	xmlStr := xml.Header + string(out)
	logger.Info("EPG filtering completed successfully")
	return xmlStr, nil
}

// parseEPGTime converts EPG timestamp (e.g. "20250101000000 +0300") to UTC time.Time.
func parseEPGTime(match []string) (time.Time, error) {
	loc := time.UTC
	tz := match[7]
	if len(tz) >= 5 && (tz[0] == '+' || tz[0] == '-') {
		hours, _ := strconv.Atoi(tz[1:3])
		mins, _ := strconv.Atoi(tz[3:5])
		offset := hours*3600 + mins*60
		if tz[0] == '-' {
			offset = -offset
		}
		loc = time.FixedZone(tz, offset)
	}

	t, err := time.ParseInLocation("2006-01-02 15:04:05",
		fmt.Sprintf("%s-%s-%s %s:%s:%s", match[1], match[2], match[3], match[4], match[5], match[6]), loc)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

// SaveFilteredEPGLocally writes EPG content to disk, gzip-compressed if filename ends with .gz.
func SaveFilteredEPGLocally(content, filename string, cfg *config.Config) error {
	outputDir := cfg.OutputDir()
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	filepath := path.Join(outputDir, filename)

	if strings.HasSuffix(filename, ".gz") {
		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		if _, err := gw.Write([]byte(content)); err != nil {
			return fmt.Errorf("failed to compress EPG: %w", err)
		}
		if err := gw.Close(); err != nil {
			return fmt.Errorf("failed to finalize gzip: %w", err)
		}

		if err := os.WriteFile(filepath, buf.Bytes(), 0644); err != nil {
			return fmt.Errorf("failed to write EPG file: %w", err)
		}

		if fi, err := os.Stat(filepath); err == nil {
			logger.Info("EPG saved locally as compressed file: %s (compressed: %.2f KB, original: %.2f KB)",
				filepath, float64(fi.Size())/1024, float64(len(content))/1024)
		}
	} else {
		if err := os.WriteFile(filepath, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write EPG file: %w", err)
		}
		if fi, err := os.Stat(filepath); err == nil {
			logger.Info("EPG saved locally as %s (size: %.2f KB)", filepath, float64(fi.Size())/1024)
		}
	}

	return nil
}
