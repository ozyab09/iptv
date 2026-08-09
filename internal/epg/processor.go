package epg

import (
	"archive/zip"
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
	XMLName     xml.Name      `xml:"channel"`
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
	XMLName  xml.Name   `xml:"programme"`
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

// DownloadEPG downloads and decompresses (gz/zip) EPG content. New callers
// should prefer DownloadEPGToFile to avoid holding a large EPG in memory.
func DownloadEPG(ctx context.Context, urlStr string, cfg *config.Config) (string, error) {
	filePath, err := DownloadEPGToFile(ctx, urlStr, cfg)
	if err != nil {
		return "", err
	}
	defer os.Remove(filePath)
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read downloaded EPG: %w", err)
	}
	return string(content), nil
}

// DownloadEPGToFile downloads an EPG and expands it into a bounded temporary
// XML file. Both downloading and decompression are streamed to avoid retaining
// the compressed and expanded EPG in process memory.
func DownloadEPGToFile(ctx context.Context, urlStr string, cfg *config.Config) (string, error) {
	logger.Info("Downloading EPG file from: %s", urlStr)

	outputDir := cfg.OutputDir()
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("create EPG output directory: %w", err)
	}
	parsedURL, _ := url.Parse(urlStr)
	fname := path.Base(parsedURL.Path)
	if fname == "" || fname == "." || fname == "/" {
		fname = "downloaded_epg.xml"
	}
	originalFilePath := path.Join(outputDir, "original_"+fname)

	if err := utils.DownloadFileToPathWithContext(ctx, urlStr, originalFilePath, int64(config.MaxEPGFileSize), cfg.SkipSSLVerify()); err != nil {
		logger.Error("Error downloading EPG file: %v", err)
		return "", err
	}
	if fi, err := os.Stat(originalFilePath); err == nil {
		logger.Info("Original EPG file saved as: %s (size: %.2f KB)", originalFilePath, float64(fi.Size())/1024)
	}

	decompressed, err := os.CreateTemp(outputDir, "epg-source-*.xml")
	if err != nil {
		return "", fmt.Errorf("create EPG XML file: %w", err)
	}
	decompressedPath := decompressed.Name()
	success := false
	defer func() {
		_ = decompressed.Close()
		if !success {
			_ = os.Remove(decompressedPath)
		}
	}()

	if err := expandEPGFile(originalFilePath, decompressed, int64(config.MaxEPGFileSize)); err != nil {
		return "", err
	}
	if err := decompressed.Close(); err != nil {
		return "", fmt.Errorf("close expanded EPG: %w", err)
	}
	if fi, err := os.Stat(decompressedPath); err == nil {
		logger.Info("EPG file downloaded successfully, expanded size: %.2f KB", float64(fi.Size())/1024)
	}
	success = true
	return decompressedPath, nil
}

func expandEPGFile(sourcePath string, destination io.Writer, maxSize int64) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open downloaded EPG: %w", err)
	}
	defer source.Close()

	var magic [4]byte
	n, err := io.ReadFull(source, magic[:])
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return fmt.Errorf("read EPG header: %w", err)
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind downloaded EPG: %w", err)
	}

	switch {
	case utils.IsGzipped(magic[:n]):
		logger.Info("Detected gzipped EPG file, decompressing...")
		reader, err := gzip.NewReader(source)
		if err != nil {
			return fmt.Errorf("open gzip EPG: %w", err)
		}
		defer reader.Close()
		if _, err := utils.CopyLimited(destination, reader, maxSize); err != nil {
			return fmt.Errorf("decompress gzip EPG: %w", err)
		}
	case utils.IsZipped(magic[:n]):
		logger.Info("Detected zipped EPG file, extracting...")
		info, err := source.Stat()
		if err != nil {
			return fmt.Errorf("stat downloaded ZIP EPG: %w", err)
		}
		archive, err := zip.NewReader(source, info.Size())
		if err != nil {
			return fmt.Errorf("open ZIP EPG: %w", err)
		}
		var entry *zip.File
		for _, f := range archive.File {
			if !f.FileInfo().IsDir() {
				entry = f
				break
			}
		}
		if entry == nil {
			return fmt.Errorf("ZIP EPG has no files")
		}
		if entry.UncompressedSize64 > uint64(maxSize) {
			return fmt.Errorf("ZIP EPG exceeds maximum allowed size of %d bytes", maxSize)
		}
		reader, err := entry.Open()
		if err != nil {
			return fmt.Errorf("open ZIP EPG entry: %w", err)
		}
		defer reader.Close()
		if _, err := utils.CopyLimited(destination, reader, maxSize); err != nil {
			return fmt.Errorf("extract ZIP EPG: %w", err)
		}
	default:
		if _, err := utils.CopyLimited(destination, source, maxSize); err != nil {
			return fmt.Errorf("copy XML EPG: %w", err)
		}
	}
	return nil
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
// It is kept for callers that already hold a small EPG in memory.
func BuildEPGNameToIDMap(epgContent string) map[string]string {
	nameToID, err := BuildEPGNameToIDMapFromReader(strings.NewReader(epgContent))
	if err != nil {
		logger.Error("Error parsing EPG XML for name-to-id map: %v", err)
		return map[string]string{}
	}
	return nameToID
}

// BuildEPGNameToIDMapFromFile builds the EPG lookup map without loading the
// whole XML document into memory.
func BuildEPGNameToIDMapFromFile(filePath string) (map[string]string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open EPG XML: %w", err)
	}
	defer f.Close()
	return BuildEPGNameToIDMapFromReader(f)
}

// BuildEPGNameToIDMapFromReader parses only <channel> elements as a stream.
func BuildEPGNameToIDMapFromReader(r io.Reader) (map[string]string, error) {
	nameToID := make(map[string]string)
	decoder := xml.NewDecoder(r)
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse EPG XML: %w", err)
		}
		se, ok := token.(xml.StartElement)
		if !ok || se.Name.Local != "channel" {
			continue
		}
		var ch Channel
		if err := decoder.DecodeElement(&ch, &se); err != nil {
			return nil, fmt.Errorf("decode EPG channel: %w", err)
		}
		for _, dn := range ch.DisplayName {
			if dn.Value != "" {
				nameToID[strings.ToLower(strings.TrimSpace(dn.Value))] = ch.ID
			}
		}
	}

	logger.Info("Built EPG name-to-id map with %d entries", len(nameToID))
	return nameToID, nil
}

// FilterEPGContent filters an in-memory EPG. The executable uses
// FilterEPGFile so a full EPG is never retained in memory.
func FilterEPGContent(epgContent string, channelIDs map[string]string, excludedCategories, excludedChannelIDs []string, channelNames map[string]string, retentionDays int) (string, error) {
	var output bytes.Buffer
	if _, err := filterEPG(strings.NewReader(epgContent), &output, channelIDs, excludedCategories, excludedChannelIDs, channelNames, retentionDays); err != nil {
		return "", err
	}
	return output.String(), nil
}

// FilterEPGFile filters sourcePath into outputPath. A .gz output is compressed
// on the fly and atomically moved into place after a successful parse.
func FilterEPGFile(sourcePath, outputPath string, channelIDs map[string]string, excludedCategories, excludedChannelIDs []string, channelNames map[string]string, retentionDays int) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open source EPG: %w", err)
	}
	defer source.Close()

	outputDir := path.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("create filtered EPG directory: %w", err)
	}
	temporary, err := os.CreateTemp(outputDir, ".epg-filter-*.tmp")
	if err != nil {
		return fmt.Errorf("create filtered EPG temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	success := false
	defer func() {
		_ = temporary.Close()
		if !success {
			_ = os.Remove(temporaryPath)
		}
	}()

	var writer io.Writer = temporary
	var gzipWriter *gzip.Writer
	if strings.HasSuffix(strings.ToLower(outputPath), ".gz") {
		gzipWriter = gzip.NewWriter(temporary)
		writer = gzipWriter
	}
	stats, err := filterEPG(source, writer, channelIDs, excludedCategories, excludedChannelIDs, channelNames, retentionDays)
	if err != nil {
		return err
	}
	if gzipWriter != nil {
		if err := gzipWriter.Close(); err != nil {
			return fmt.Errorf("finalize compressed filtered EPG: %w", err)
		}
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close filtered EPG: %w", err)
	}
	if err := os.Rename(temporaryPath, outputPath); err != nil {
		return fmt.Errorf("publish filtered EPG: %w", err)
	}
	success = true
	if info, err := os.Stat(outputPath); err == nil {
		logger.Info("EPG filtering: %d channels, %d programmes; saved %s (%.2f KB)", stats.channels, stats.programmes, outputPath, float64(info.Size())/1024)
	}
	return nil
}

type epgFilterStats struct {
	channels   int
	programmes int
}

func filterEPG(r io.Reader, w io.Writer, channelIDs map[string]string, excludedCategories, excludedChannelIDs []string, channelNames map[string]string, retentionDays int) (epgFilterStats, error) {
	logger.Info("Filtering EPG content for %d channel IDs and %d channel names", len(channelIDs), len(channelNames))
	if len(channelIDs) == 0 && len(channelNames) == 0 {
		if _, err := io.WriteString(w, xml.Header+"<tv></tv>"); err != nil {
			return epgFilterStats{}, fmt.Errorf("write empty EPG: %w", err)
		}
		return epgFilterStats{}, nil
	}

	excludedIDSet := make(map[string]bool, len(excludedChannelIDs))
	for _, id := range excludedChannelIDs {
		excludedIDSet[id] = true
	}
	nameCategories := make(map[string]string, len(channelNames))
	for name, category := range channelNames {
		normalized := strings.ToLower(utils.StripTrailingEmoji(strings.TrimSpace(name)))
		if normalized != "" {
			nameCategories[normalized] = category
		}
	}

	now := time.Now()
	from := now.Add(-time.Hour)
	until := now.AddDate(0, 0, retentionDays)
	decoder := xml.NewDecoder(r)
	encoder := xml.NewEncoder(w)
	encoder.Indent("", "  ")
	if _, err := io.WriteString(w, xml.Header); err != nil {
		return epgFilterStats{}, fmt.Errorf("write EPG header: %w", err)
	}

	channelsToKeep := make(map[string]bool)
	stats := epgFilterStats{}
	rootWritten := false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return stats, fmt.Errorf("parse EPG XML: %w", err)
		}

		switch element := token.(type) {
		case xml.StartElement:
			switch element.Name.Local {
			case "tv":
				if !rootWritten {
					if err := encoder.EncodeToken(element); err != nil {
						return stats, fmt.Errorf("write EPG root: %w", err)
					}
					rootWritten = true
				}
			case "channel":
				var channel Channel
				if err := decoder.DecodeElement(&channel, &element); err != nil {
					return stats, fmt.Errorf("decode EPG channel: %w", err)
				}
				if keepEPGChannel(channel, channelIDs, nameCategories, excludedCategories, excludedIDSet) {
					channelsToKeep[channel.ID] = true
					if err := encoder.Encode(channel); err != nil {
						return stats, fmt.Errorf("write EPG channel: %w", err)
					}
					stats.channels++
				}
			case "programme":
				channelID := attribute(element, "channel")
				if !channelsToKeep[channelID] {
					if err := decoder.Skip(); err != nil {
						return stats, fmt.Errorf("skip EPG programme: %w", err)
					}
					continue
				}
				var programme Programme
				if err := decoder.DecodeElement(&programme, &element); err != nil {
					return stats, fmt.Errorf("decode EPG programme: %w", err)
				}
				if programmeWithinRetention(programme, from, until) {
					if err := encoder.Encode(programme); err != nil {
						return stats, fmt.Errorf("write EPG programme: %w", err)
					}
					stats.programmes++
				}
			}
		case xml.EndElement:
			if rootWritten && element.Name.Local == "tv" {
				if err := encoder.EncodeToken(element); err != nil {
					return stats, fmt.Errorf("write EPG end: %w", err)
				}
			}
		}
	}
	if !rootWritten {
		return stats, fmt.Errorf("EPG XML has no <tv> root element")
	}
	if err := encoder.Flush(); err != nil {
		return stats, fmt.Errorf("flush EPG output: %w", err)
	}
	logger.Info("EPG content filtering: %d channels after exclusions, %d programmes retained", stats.channels, stats.programmes)
	return stats, nil
}

func keepEPGChannel(channel Channel, channelIDs, nameCategories map[string]string, excludedCategories []string, excludedIDs map[string]bool) bool {
	if excludedIDs[channel.ID] {
		return false
	}
	matched := false
	if category, ok := channelIDs[channel.ID]; ok {
		matched = true
		if categoryExcluded(category, excludedCategories) {
			return false
		}
	}
	for _, displayName := range channel.DisplayName {
		category, ok := nameCategories[strings.ToLower(strings.TrimSpace(displayName.Value))]
		if !ok {
			continue
		}
		matched = true
		if categoryExcluded(category, excludedCategories) {
			return false
		}
	}
	return matched
}

func categoryExcluded(category string, excludedCategories []string) bool {
	for _, excluded := range excludedCategories {
		if strings.EqualFold(strings.TrimSpace(category), strings.TrimSpace(excluded)) {
			return true
		}
	}
	return false
}

func attribute(element xml.StartElement, name string) string {
	for _, attr := range element.Attr {
		if attr.Name.Local == name {
			return attr.Value
		}
	}
	return ""
}

func programmeWithinRetention(programme Programme, from, until time.Time) bool {
	startMatch := epgTimeRegex.FindStringSubmatch(programme.Start)
	stopMatch := epgTimeRegex.FindStringSubmatch(programme.Stop)
	if startMatch == nil || stopMatch == nil {
		return true
	}
	startTime, startErr := parseEPGTime(startMatch)
	stopTime, stopErr := parseEPGTime(stopMatch)
	if startErr != nil || stopErr != nil {
		return true
	}
	return !stopTime.Before(from) && !startTime.After(until)
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
