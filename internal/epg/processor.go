package epg

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/ozyab/iptv/internal/config"
	"github.com/ozyab/iptv/internal/utils"
)

var logger = utils.NewSanitizedLoggerWithPrefix("[epg]")

var httpClient = &http.Client{
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	},
	Timeout: 30 * time.Minute,
}

type TV struct {
	XMLName    xml.Name  `xml:"tv"`
	Channels   []Channel `xml:"channel"`
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
	Channel string      `xml:"channel,attr"`
	Start   string      `xml:"start,attr"`
	Stop    string      `xml:"stop,attr"`
	Title   []Title     `xml:"title"`
	Desc    []Desc      `xml:"desc"`
	Category []Category `xml:"category"`
	Icon    []Icon      `xml:"icon"`
	Rating  []Rating    `xml:"rating"`
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

func DownloadEPG(urlStr string, cfg *config.Config) (string, error) {
	logger.Info("Downloading EPG file from: %s", urlStr)

	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return "", err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		logger.Error("Error downloading EPG file: %v", err)
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
			if totalSize > config.MaxEPGFileSize {
				return "", fmt.Errorf("EPG file exceeds maximum allowed size of %d bytes", config.MaxEPGFileSize)
			}
			chunks = append(chunks, buf[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			logger.Error("Error downloading EPG file: %v", err)
			return "", err
		}
	}

	rawContent := chunks

	outputDir := cfg.OUTPUT_DIR()
	os.MkdirAll(outputDir, 0755)

	parsedURL, _ := url.Parse(urlStr)
	fname := path.Base(parsedURL.Path)
	if fname == "" {
		fname = "downloaded_epg.xml"
	}
	originalFilePath := path.Join(outputDir, "original_"+fname)

	var data []byte
	if strings.HasSuffix(urlStr, ".gz") || isGzipped(rawContent) {
		logger.Info("Detected gzipped EPG file, decompressing...")
		os.WriteFile(originalFilePath, rawContent, 0644)
		fi, _ := os.Stat(originalFilePath)
		logger.Info("Original compressed EPG file saved as: %s (size: %.2f KB)", originalFilePath, float64(fi.Size())/1024)

		gr, err := gzip.NewReader(bytes.NewReader(rawContent))
		if err != nil {
			return "", fmt.Errorf("failed to decompress gzip: %w", err)
		}
		data, err = io.ReadAll(gr)
		gr.Close()
		if err != nil {
			return "", fmt.Errorf("failed to read decompressed data: %w", err)
		}
	} else if strings.HasSuffix(urlStr, ".zip") {
		logger.Info("Detected zipped EPG file, extracting...")
		os.WriteFile(originalFilePath, rawContent, 0644)
		fi, _ := os.Stat(originalFilePath)
		logger.Info("Original zipped EPG file saved as: %s (size: %.2f KB)", originalFilePath, float64(fi.Size())/1024)

		zr, err := zip.NewReader(bytes.NewReader(rawContent), int64(len(rawContent)))
		if err != nil {
			return "", fmt.Errorf("failed to read zip: %w", err)
		}
		if len(zr.File) == 0 {
			return "", fmt.Errorf("ZIP archive is empty")
		}
		f, err := zr.File[0].Open()
		if err != nil {
			return "", fmt.Errorf("failed to open first file in zip: %w", err)
		}
		data, err = io.ReadAll(f)
		f.Close()
		if err != nil {
			return "", fmt.Errorf("failed to read zip entry: %w", err)
		}
	} else {
		os.WriteFile(originalFilePath, rawContent, 0644)
		fi, _ := os.Stat(originalFilePath)
		logger.Info("Original EPG file saved as: %s (size: %.2f KB)", originalFilePath, float64(fi.Size())/1024)
		data = rawContent
	}

	content := string(data)
	logger.Info("EPG file downloaded successfully, size: %.2f KB", float64(len(data))/1024)
	return content, nil
}

func isGzipped(data []byte) bool {
	return len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b
}

func ExtractChannelInfoFromPlaylist(playlistContent string) (map[string]string, map[string]string) {
	logger.Info("Extracting channel IDs and categories from playlist")

	channelIDs := make(map[string]string)
	channelNames := make(map[string]string)

	gtRegex := regexp.MustCompile(`group-title="([^"]*)"`)
	tvgRegex := regexp.MustCompile(`tvg-id="([^"]*)"`)

	for _, line := range strings.Split(playlistContent, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#EXTINF:") {
			continue
		}

		var category string
		if m := gtRegex.FindStringSubmatch(line); m != nil {
			category = strings.TrimSpace(m[1])
		}

		if m := tvgRegex.FindStringSubmatch(line); m != nil {
			tvgID := strings.TrimSpace(m[1])
			if tvgID != "" {
				channelIDs[tvgID] = category
			}
		}

		parts := strings.SplitN(line, ",", 2)
		if len(parts) > 1 {
			chName := strings.TrimSpace(parts[1])
			if chName != "" {
				channelNames[chName] = category
			}
		}
	}

	logger.Info("Found %d unique channel IDs and %d channel names in playlist", len(channelIDs), len(channelNames))
	return channelIDs, channelNames
}

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

var epgTimeRegex = regexp.MustCompile(`(\d{4})(\d{2})(\d{2})(\d{2})(\d{2})(\d{2})\s+(\S+)`)

func FilterEPGContent(epgContent string, channelIDs map[string]string, excludedCategories, excludedChannelIDs []string, channelNames map[string]string) (string, error) {
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
		normalized := strings.ToLower(strings.TrimSpace(name))
		chNamesNormalized[normalized] = true
		if cat != "" {
			chNameCatLower[normalized] = cat
		}
	}

	var tv TV
	if err := xml.Unmarshal([]byte(epgContent), &tv); err != nil {
		logger.Error("Error parsing EPG XML: %v", err)
		return "", err
	}

	epgChDisplayNames := make(map[string][]string)
	for _, ch := range tv.Channels {
		for _, dn := range ch.DisplayName {
			if dn.Value != "" {
				epgChDisplayNames[ch.ID] = append(epgChDisplayNames[ch.ID], strings.ToLower(strings.TrimSpace(dn.Value)))
			}
		}
	}

	channelsToKeep := make(map[string]bool)
	channelsToKeepByName := make(map[string]string)

	for _, prog := range tv.Programmes {
		channelRef := prog.Channel
		_, matchedByID := channelIDs[channelRef]

		matchedByName := false
		var matchedCategory string

		if !matchedByID && len(chNamesNormalized) > 0 {
			if dns, ok := epgChDisplayNames[channelRef]; ok {
				for _, dn := range dns {
					if chNamesNormalized[dn] {
						matchedByName = true
						matchedCategory = chNameCatLower[dn]
						break
					}
				}
			}
		}

		if matchedByID || matchedByName {
			var categoryToCheck string
			if matchedByID {
				categoryToCheck = channelIDs[channelRef]
			} else if matchedByName {
				categoryToCheck = matchedCategory
			}

			excludeByCat := false
			if categoryToCheck != "" {
				catLower := strings.ToLower(categoryToCheck)
				for _, ec := range excludedCatLower {
					if catLower == ec {
						excludeByCat = true
						break
					}
				}
			}

			excludeByID := excludedIDSet[channelRef]

			if !excludeByCat && !excludeByID {
				channelsToKeep[channelRef] = true
				if matchedByName {
					channelsToKeepByName[channelRef] = matchedCategory
				}
			}
		}
	}

	logger.Info("EPG content filtering: %d channels after category and ID exclusions (from %d tvg-id + %d names)",
		len(channelsToKeep), len(channelIDs), len(channelNames))

	var result TV

	for _, ch := range tv.Channels {
		if !channelsToKeep[ch.ID] {
			continue
		}
		newCh := Channel{ID: ch.ID}
		if len(ch.DisplayName) > 0 {
			newCh.DisplayName = []DisplayName{{
				Lang:  ch.DisplayName[0].Lang,
				Value: ch.DisplayName[0].Value,
			}}
		}
		for _, child := range ch.DisplayName[1:] {
			_ = child
		}
		for _, icon := range ch.Icon {
			_ = icon
		}
		result.Channels = append(result.Channels, newCh)
	}

	now := time.Now()
	oneHourAgo := now.Add(-1 * time.Hour)
	retentionDays := 10
	cfg := config.New()
	if rd := cfg.EPGRetentionDays(); rd > 0 {
		retentionDays = rd
	}
	retentionLater := now.AddDate(0, 0, retentionDays)

	for _, prog := range tv.Programmes {
		if !channelsToKeep[prog.Channel] {
			continue
		}

		startMatch := epgTimeRegex.FindStringSubmatch(prog.Start)
		stopMatch := epgTimeRegex.FindStringSubmatch(prog.Stop)

		include := false
		if startMatch != nil && stopMatch != nil {
			startTime, err1 := parseEPGTime(startMatch)
			stopTime, err2 := parseEPGTime(stopMatch)
			if err1 == nil && err2 == nil {
				if !stopTime.Before(oneHourAgo) && !startTime.After(retentionLater) {
					include = true
				}
			} else {
				include = true
			}
		} else {
			include = true
		}

		if !include {
			continue
		}

		result.Programmes = append(result.Programmes, prog)
	}

	out, err := xml.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal XML: %w", err)
	}

	xmlStr := xml.Header + string(out)
	logger.Info("EPG filtering completed successfully")
	return xmlStr, nil
}

func parseEPGTime(match []string) (time.Time, error) {
	year := match[1]
	month := match[2]
	day := match[3]
	hour := match[4]
	min := match[5]
	sec := match[6]

	return time.Parse("2006-01-02 15:04:05",
		fmt.Sprintf("%s-%s-%s %s:%s:%s", year, month, day, hour, min, sec))
}

func SaveFilteredEPGLocally(content, filename string, cfg *config.Config) error {
	outputDir := cfg.OUTPUT_DIR()
	os.MkdirAll(outputDir, 0755)

	filepath := path.Join(outputDir, filename)

	if strings.HasSuffix(filename, ".gz") {
		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		if _, err := gw.Write([]byte(content)); err != nil {
			return err
		}
		gw.Close()

		if err := os.WriteFile(filepath, buf.Bytes(), 0644); err != nil {
			return err
		}

		fi, _ := os.Stat(filepath)
		logger.Info("EPG saved locally as compressed file: %s (compressed: %.2f KB, original: %.2f KB)",
			filepath, float64(fi.Size())/1024, float64(len(content))/1024)
	} else {
		if err := os.WriteFile(filepath, []byte(content), 0644); err != nil {
			return err
		}
		fi, _ := os.Stat(filepath)
		logger.Info("EPG saved locally as %s (size: %.2f KB)", filepath, float64(fi.Size())/1024)
	}

	return nil
}
