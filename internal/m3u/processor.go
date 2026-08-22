package m3u

import (
	"context"
	"fmt"
	"hash/fnv"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ozyab/iptv/internal/config"
	"github.com/ozyab/iptv/internal/utils"
)

var logger = utils.NewSanitizedLoggerWithPrefix("[m3u]")

// Pre-compiled regexps for efficient filtering.
var (
	regRegional     = regexp.MustCompile(`\s\+\d+(?:\s+HD)?(?:\s*\([^)]+\))?\s*$`)
	regNumberSuffix = regexp.MustCompile(`\s\d{2,}$`)
	// regNumericGroupSuffix matches a trailing parenthesized number like
	// "360 (2)" / "360 (3)" — duplicate variants of the same channel that
	// playlist authors number to disambiguate multiple URLs.
	regNumericGroupSuffix = regexp.MustCompile(`\s*\(\d+\)\s*$`)
	regLeadingNumber      = regexp.MustCompile(`^\d+\.\s*`)
	regGroupTitle         = regexp.MustCompile(`group-title="([^"]*)"`)
	regTvgID              = regexp.MustCompile(`tvg-id="([^"]*)"`)
	regURLTVG             = regexp.MustCompile(`url-tvg="[^"]*"`)
	regTVGURL             = regexp.MustCompile(`tvg-url="[^"]*"`)
	regTvgLogo            = regexp.MustCompile(`tvg-logo="([^"]*)"`)
	regTvgRec             = regexp.MustCompile(`tvg-rec="([^"]*)"`)

	// Replacement-only regexps (no capture groups).
	regGroupTitleAttr = regexp.MustCompile(`group-title="[^"]*"`)
	regTvgIDAttr      = regexp.MustCompile(`tvg-id="[^"]*"`)
	regTvgLogoAttr    = regexp.MustCompile(`tvg-logo="[^"]*"`)
	regTvgRecAttr     = regexp.MustCompile(`tvg-rec="[^"]*"`)

	// regStreamURL matches a line that starts with a URL scheme (http, https, rtmp, udp, ...).
	regStreamURL = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*://`)

	// Display-name cleanup regexps.
	regRecordBadge  = regexp.MustCompile(`⏺ʳᵉᶜ`)
	regGeoBlocked   = regexp.MustCompile(`(?i)\s*\[geo-blocked\]`)
	regResolution   = regexp.MustCompile(`(?i)\s*\((720p|1080p|576p|480p|2160p)\)`)
	regGarbageStart = regexp.MustCompile(`^[.&#]+\s*`)
	// regPipeRegion matches "Name | Region" (used both for display normalization
	// to "Name (Region)" and for grouping to "Name Region").
	regPipeRegion = regexp.MustCompile(`\s*\|\s+([^|]+)$`)
	// regTrailingParenNumber matches "Name (2)" style duplicate-variant numbers
	// anywhere in the name (used for grouping; display keeps regional text).
	regParenNumber = regexp.MustCompile(`\s*\(\d+\)`)
	// regParenText matches a parenthesized region/qualifier like "(Новокузнецк)";
	// kept for display but flattened for grouping so it matches the "| Region" form.
	regParenText = regexp.MustCompile(`\s*\(([^()]*)\)`)
	// regTrailingDomain matches a trailing domain-like suffix (.tv/.com/.ru/...)
	// used for grouping so "1-2-3.tv" and "1-2-3" group together.
	regTrailingDomain = regexp.MustCompile(`(?i)\.(tv|com|net|ru|ua|org|co|info|biz)\b`)

	// Transliteration for grouping: latin → cyrillic for well-known channel words,
	// and ukrainian → russian letters. Applied only in normalizeChannelName so
	// display names are untouched.
	translitWords = map[string]string{
		"rossiya":    "россия",
		"russia":     "россия",
		"ukraina":    "украина",
		"ukraine":    "украина",
		"belarus":    "беларусь",
		"kazakh":     "казах",
		"kazakhstan": "казахстан",
	}
	regTranslitWord = regexp.MustCompile(`[a-zа-я]+`)

	// Quality tokens used for ranking channel variants (order matters: full hd before hd).
	regQualityRank = regexp.MustCompile(`(?i)\b(4k|2160p|uhd|fhd|full\s*hd|fullhd|1080p|720p|576p|480p|hdtv|hd|sd|fd|hq|lq)\b`)

	regSpaces = regexp.MustCompile(`\s+`)
)

// isStreamURL reports whether the line looks like a stream URL (any scheme:// prefix).
func isStreamURL(line string) bool {
	return regStreamURL.MatchString(line)
}

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
				if isStreamURL(strings.TrimSpace(nextLine)) {
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

// emojiPoolA: 100+ символов (геометрия, погода, космос, сердца, знаки).
// Первый эмодзи пары канала формируется из hostname (DNS-имени) URL потока.
var emojiPoolA = []string{
	// Геометрия и цвета.
	"🔴", "🟠", "🟡", "🟢", "🔵", "🟣", "⚫", "⚪",
	"🔶", "🔷", "🔸", "🔹", "🔺", "🔻", "⬛", "⬜",
	"🔘", "⭕", "💠", "💮", "❓", "💢", "💥", "💫",
	"🟥", "🟧", "🟨", "🟩", "🟦", "🟪", "🟫",
	"◼️", "◻️", "◽", "◾", "🔲", "🔳",
	// Погода, небо и космос.
	"🌟", "⭐", "✨", "🔥", "💧", "🌊", "☀️", "🌙",
	"☁️", "⚡", "🌀", "🌈", "💨", "❄️", "🌪", "🌫",
	"🌤️", "🌥️", "🌦️", "🌧️", "🌨️", "🌩️", "⛈️", "🌬️",
	"🌍", "🌎", "🌏", "🌕", "🌖", "🌗", "🌘", "🌑",
	"🌒", "🌓", "🌔", "🌝", "🌚", "🌛", "🌜", "🌞",
	"🌠", "☄️",
	// Сердца.
	"❤️", "🧡", "💛", "💚", "💙", "💜", "🖤", "🤍",
	"🤎", "💖", "💗", "💓", "💞", "💕", "💘", "💝",
	"💟", "❣️", "💔",
	// Знаки и символы.
	"✅", "❌", "❎", "➕", "➖", "✖️", "💯", "🔟",
	"🏁", "🚩", "🎌",
}

// emojiPoolB: 100+ эмодзи (животные, растения, еда, объекты, транспорт).
// Второй эмодзи пары канала формируется из path-части URL потока.
var emojiPoolB = []string{
	// Животные.
	"🐶", "🐱", "🐼", "🐯", "🦁", "🦊", "🐻", "🐨",
	"🐰", "🦄", "🐸", "🐵", "🦋", "🐝", "🐞", "🐌",
	"🐭", "🐹", "🐺", "🐗", "🐴", "🐮", "🐷", "🐽",
	"🐍", "🦎", "🐢", "🐙", "🦑", "🦐", "🦞", "🦀",
	"🐡", "🐠", "🐟", "🐬", "🐳", "🐋", "🦈", "🐊",
	"🐅", "🐆", "🦓", "🦍", "🦧", "🐘", "🦛", "🦏",
	"🐪", "🐫", "🦒", "🦘", "🐃", "🐂", "🐄", "🐎",
	"🐖", "🐏", "🐑", "🦙", "🐐", "🦌", "🐕", "🐩",
	"🦮", "🦚", "🦜", "🦢", "🦩", "🦝", "🦨", "🦡",
	"🦫", "🦦", "🦥", "🐀", "🐿️", "🦔", "🐉", "🐲",
	// Растения.
	"🌻", "🌺", "🌹", "🌸", "🌴", "🍀", "🌿", "🍁",
	"🌵", "🌲", "🌳", "🌱", "🍃", "🍂", "🌾", "💐",
	"🌷", "🌼", "🌽", "🌶️", "🥀",
	// Еда.
	"🍎", "🍊", "🍋", "🍇", "🍓", "🍉", "🍒", "🍑",
	"🥭", "🍍", "🥥", "🍅", "🥑", "🥕", "🥔", "🍞",
	"🧀", "🥚", "🍳", "🥞", "🥓", "🍗", "🍖", "🌭",
	"🍔", "🍟", "🍕", "🌮", "🥗", "🍜", "🍣", "🍦",
	"🍰", "🎂", "🍭", "🍬", "🍫", "🍿", "🍩", "🍪",
	"🍺", "🍷", "🥛", "🍵", "☕",
	// Объекты и транспорт.
	"🏀", "⚽", "🎾", "🏐", "🎱", "🏓", "🎳", "⛳",
	"🏅", "🎟️", "🎫", "🎰", "🧩", "🎲", "♟️", "🎧",
	"🎤", "🎹", "🪗", "🪕", "💻", "📱", "⌚", "🗿",
	"🗼", "🏰", "🏯", "🗽", "🚗", "🚕", "🚙", "🚌",
	"🏎️", "🚓", "🚑", "🚒", "🚚", "🚜", "🛵", "🚲",
	"✈️", "🛫", "🛬", "🚁", "🛶", "⛵", "🚤", "🚢",
}

// urlToEmojiPair returns a two-emoji identifier for a stream URL:
// the first emoji is derived from the URL hostname, the second from the URL path.
func urlToEmojiPair(rawURL string) string {
	return emojiFromHostname(rawURL) + emojiFromPath(rawURL)
}

// emojiFromHostname derives an emoji from the URL hostname's registrable
// domain (port ignored). Only the second-level domain is hashed, so subdomains
// of the same resource — e.g. https://bsttv.bonus-tv.ru/... and
// http://cdn-01.bonus-tv.ru:80/... — produce the same first emoji.
func emojiFromHostname(rawURL string) string {
	host := urlComponent(rawURL, true)
	return emojiPoolA[fnv64(registrableDomain(host))%uint64(len(emojiPoolA))]
}

// registrableDomain reduces a hostname to the domain that its owner controls:
// the last two labels ("bsttv.bonus-tv.ru" → "bonus-tv.ru"), or three labels
// when the TLD is a two-letter country code with a common second-level domain
// like co.uk / com.ru ("www.bbc.co.uk" → "bbc.co.uk"). IP addresses and
// single-label hosts are returned unchanged.
func registrableDomain(host string) string {
	labels := strings.Split(host, ".")
	n := len(labels)
	if n <= 2 {
		return host
	}
	// IPv4 address — no registrable domain, hash the whole thing.
	if isIPv4Labels(labels) {
		return host
	}
	// "label.co.uk" / "label.com.ru" / "label.net.de" style: keep 3 labels.
	if len(labels[n-1]) == 2 && isCommonSecondLevelDomain(labels[n-2]) {
		return strings.Join(labels[n-3:], ".")
	}
	// Plain TLD (com/net/org/... or 2-letter ccTLD without common SLD): last 2.
	return strings.Join(labels[n-2:], ".")
}

// isIPv4Labels reports whether every label is numeric (a dotted IPv4 address).
func isIPv4Labels(labels []string) bool {
	for _, l := range labels {
		if l == "" {
			return false
		}
		for _, r := range l {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

// isCommonSecondLevelDomain reports whether a label is a common second-level
// domain under a country-code TLD (co.uk, com.ru, net.de, org.pl, ...).
func isCommonSecondLevelDomain(label string) bool {
	switch strings.ToLower(label) {
	case "co", "com", "net", "org", "ac", "edu", "gov", "mil", "gen", "name":
		return true
	}
	return false
}

// emojiFromPath derives an emoji from the URL path part.
func emojiFromPath(rawURL string) string {
	path := urlComponent(rawURL, false)
	return emojiPoolB[fnv64(path)%uint64(len(emojiPoolB))]
}

// fnv64 computes the FNV-1a 64-bit hash of a string.
func fnv64(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}

// urlComponent extracts the hostname (wantHost=true) or path (wantHost=false) part
// of a URL for emoji derivation. Falls back to the raw URL string when parsing
// fails or the URL has no hostname (relative/malformed URLs).
func urlComponent(rawURL string, wantHost bool) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		return rawURL
	}
	if wantHost {
		return strings.ToLower(u.Hostname())
	}
	path := u.Path
	if path == "" {
		path = "/"
	}
	return path
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

// RemoveOrigSuffix strips trailing " orig" (case-insensitive) from channel name.
func RemoveOrigSuffix(name string) string {
	if len(name) >= 5 && strings.HasSuffix(strings.ToLower(name), " orig") {
		return name[:len(name)-5]
	}
	return name
}

// CleanChannelName tidies the displayed channel name while keeping informative
// tokens (HD/SD, country codes). It removes technical clutter that playlist
// authors paste into names:
//   - record badge "⏺ʳᵉᶜ"
//   - "[Geo-blocked]" marker
//   - resolution in parens like "(720p)"
//   - leading junk characters (# . &)
//   - double spaces
//   - normalizes "Name | Region" to "Name (Region)" so regional variants read
//     consistently across sources.
func CleanChannelName(name string) string {
	s := strings.TrimSpace(name)
	s = regRecordBadge.ReplaceAllString(s, "")
	s = regGeoBlocked.ReplaceAllString(s, "")
	s = regResolution.ReplaceAllString(s, "")
	s = regGarbageStart.ReplaceAllString(s, "")
	// "10 канал | Новокузнецк" → "10 канал (Новокузнецк)".
	s = regPipeRegion.ReplaceAllStringFunc(s, func(m string) string {
		parts := regPipeRegion.FindStringSubmatch(m)
		if len(parts) > 1 {
			return " (" + strings.TrimSpace(parts[1]) + ")"
		}
		return m
	})
	s = regSpaces.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
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
	normalized = utils.StripTrailingEmoji(normalized)
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

		// Remove "orig" suffix, then tidy the displayed name (record badge,
		// [Geo-blocked], (720p), leading junk, double spaces, "| Region" → "(Region)").
		newName := RemoveOrigSuffix(channelName)
		newName = CleanChannelName(newName)
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
			if isStreamURL(trimmed) {
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

// ─── Pipeline: deduplicate by name (options B/C) ─────────────────────────────────

// qualityRank maps a channel name to a quality priority: 4K/UHD > FHD > HD > SD > none.
func qualityRank(name string) int {
	lower := strings.ToLower(name)
	if m := regQualityRank.FindStringSubmatch(lower); len(m) > 1 {
		switch m[1] {
		case "4k", "2160p", "uhd":
			return 4
		case "fhd", "full hd", "fullhd", "1080p":
			return 3
		case "hd", "hdtv", "720p", "hq":
			return 2
		case "sd", "lq":
			return 1
		}
	}
	// Unicode superscript HD (e.g. "РОССИЯ 24ᴴᴰ").
	if strings.Contains(name, "ᴴᴰ") {
		return 2
	}
	return 0
}

// normalizeChannelName reduces a channel name to its base form for grouping:
// emoji pairs, quality tokens, regional/numeric suffixes, separators and case
// are stripped so "Канал HD", "КАНАЛᴴᴰ" and "Канал" group together. Duplicate
// variants from different sources ("360 (2)", "1-2-3.tv HD DE", "10 канал |
// Новокузнецк") collapse to the same key.
func normalizeChannelName(name string) string {
	s := utils.StripTrailingEmoji(strings.TrimSpace(name))
	s = strings.ToLower(s)
	s = regQualityRank.ReplaceAllString(s, " ")
	s = strings.ReplaceAll(s, "ᴴ", "")
	s = strings.ReplaceAll(s, "ᴰ", "")
	s = strings.ReplaceAll(s, " orig", "")
	s = regRegional.ReplaceAllString(s, " ")
	// Duplicate-variant numbers anywhere: "360 (2)", "10 канал (2) | X".
	s = regParenNumber.ReplaceAllString(s, " ")
	// Regional text in parens: "(Новокузнецк)" → " новокузнецк" so it matches
	// the "| Новокузнецк" form. Time shifts "(+2)" survive (not digits-only).
	s = regParenText.ReplaceAllString(s, " $1")
	// "| Новокузнецк" → " новокузнецк".
	s = regPipeRegion.ReplaceAllString(s, " $1")
	// Domain suffix: "1-2-3.tv" → "1-2-3", "360.ru" → "360".
	s = regTrailingDomain.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.ReplaceAll(s, "-", " ")
	// Well-known latin/ukrainian channel words → russian.
	s = transliterate(s)
	s = regSpaces.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// transliterate maps well-known latin spellings and ukrainian letters to their
// russian grouping forms (e.g. "ukraine"→"украина", "Україна"→"украина").
func transliterate(s string) string {
	// Ukrainian → russian letters inside any word.
	s = strings.ReplaceAll(s, "ї", "и")
	s = strings.ReplaceAll(s, "і", "и")
	s = strings.ReplaceAll(s, "є", "е")
	s = strings.ReplaceAll(s, "ґ", "г")
	// Whole latin words → cyrillic (word-boundary safe via token walk).
	return regTranslitWord.ReplaceAllStringFunc(s, func(w string) string {
		if r, ok := translitWords[w]; ok {
			return r
		}
		return w
	})
}

// entryURL returns the first stream URL of an entry, or "".
func entryURL(entry ChannelEntry) string {
	for _, extra := range entry.ExtraLines {
		t := strings.TrimSpace(extra)
		if isStreamURL(t) {
			return t
		}
	}
	return ""
}

// canProbeURL reports whether a URL can be checked over HTTP(S).
func canProbeURL(u string) bool {
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")
}

// extractProbeInfo builds the probe candidate for an entry, carrying the URL
// plus any per-entry request headers from #EXTVLCOPT lines (http-user-agent,
// http-referrer) so UA-locked sources are probed with the right identity.
func extractProbeInfo(entry ChannelEntry) utils.ProbeCandidate {
	c := utils.ProbeCandidate{URL: entryURL(entry)}
	for _, extra := range entry.ExtraLines {
		t := strings.TrimSpace(extra)
		switch {
		case strings.HasPrefix(t, "#EXTVLCOPT:http-user-agent="):
			c.UserAgent = strings.TrimSpace(strings.TrimPrefix(t, "#EXTVLCOPT:http-user-agent="))
		case strings.HasPrefix(t, "#EXTVLCOPT:http-referrer="):
			c.Referer = strings.TrimSpace(strings.TrimPrefix(t, "#EXTVLCOPT:http-referrer="))
		}
	}
	return c
}

// entryTvgID returns the tvg-id attribute of an entry, or "" when absent/empty.
func entryTvgID(entry ChannelEntry) string {
	if m := regTvgID.FindStringSubmatch(entry.EXTINFLine); len(m) > 1 {
		return m[1]
	}
	return ""
}

// dedupCandidate is a parsed channel variant grouped by normalized name.
type dedupCandidate struct {
	entry ChannelEntry
	qual  int
	probe utils.ProbeCandidate
	tvgID string
}

// inheritTvgID returns the first non-empty tvg-id among the group's other
// candidates. When validEPGIDs is non-nil, only ids present in it are eligible
// (stale ids are never inherited); a nil set means any id is inherited.
func inheritTvgID(group []dedupCandidate, selfIdx int, validEPGIDs map[string]bool) string {
	for i, c := range group {
		if i == selfIdx || c.tvgID == "" {
			continue
		}
		if validEPGIDs == nil || validEPGIDs[c.tvgID] {
			return c.tvgID
		}
	}
	return ""
}

// DeduplicateByName keeps at most maxVariants entries per normalized channel
// name, preferring higher quality (4K/UHD > FHD > HD > SD > unlabeled). When
// probe is non-nil, candidates are ranked by quality and only entries whose URL
// is reported alive are kept. Entries in single-variant groups pass through
// untouched. If every candidate of a group is dead, the best-quality entry is
// kept as a fallback so the channel does not disappear. Probe results are keyed
// by URL; a missing result (e.g. cancelled probe) is treated as alive.
//
// Kept entries that lack a tvg-id inherit it from a sibling variant in the same
// group (option C merge): only ids present in validEPGIDs are inherited; a nil
// validEPGIDs disables validation and inherits any non-empty id.
func DeduplicateByName(content string, maxVariants int, probe func(candidates []utils.ProbeCandidate) map[string]bool, validEPGIDs map[string]bool) string {
	if maxVariants < 1 {
		maxVariants = 1
	}

	lines := strings.Split(content, "\n")
	headers, entries := ParseChannelEntries(lines)

	groups := make(map[string][]dedupCandidate)
	for _, e := range entries {
		name := channelNameFromEntry(e)
		key := normalizeChannelName(name)
		groups[key] = append(groups[key], dedupCandidate{
			entry: e,
			qual:  qualityRank(name),
			probe: extractProbeInfo(e),
			tvgID: entryTvgID(e),
		})
	}

	// Collect unique probe-able candidate URLs from duplicate groups only.
	var probeCands []utils.ProbeCandidate
	probeSet := make(map[string]bool)
	for _, group := range groups {
		if len(group) <= 1 {
			continue
		}
		for _, cand := range group {
			if cand.probe.URL != "" && canProbeURL(cand.probe.URL) && !probeSet[cand.probe.URL] {
				probeSet[cand.probe.URL] = true
				probeCands = append(probeCands, cand.probe)
			}
		}
	}

	var alive map[string]bool
	if probe != nil && len(probeCands) > 0 {
		alive = probe(probeCands)
	}

	// Deterministic output: iterate groups by normalized name.
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var kept []ChannelEntry
	removed := 0
	fallbackKept := 0
	mergedTvgID := 0
	groupsProbed := 0

	for _, key := range keys {
		group := groups[key]
		if len(group) == 1 {
			kept = append(kept, group[0].entry)
			continue
		}

		// Stable sort by quality (descending), preserving playlist order on ties.
		sort.SliceStable(group, func(i, j int) bool { return group[i].qual > group[j].qual })

		count := 0
		best := group[0] // highest-quality fallback
		for idx, cand := range group {
			isAlive := true
			if alive != nil && cand.probe.URL != "" && canProbeURL(cand.probe.URL) {
				if ok, exists := alive[cand.probe.URL]; exists {
					isAlive = ok
				}
				// Missing result (e.g. cancelled probe) → keep, treat as alive.
			}
			if count < maxVariants && isAlive {
				entry := cand.entry
				if id := inheritTvgID(group, idx, validEPGIDs); id != "" && entryTvgID(entry) == "" {
					if parts := strings.SplitN(entry.EXTINFLine, ",", 2); len(parts) == 2 {
						entry.EXTINFLine = parts[0] + ` tvg-id="` + id + `",` + parts[1]
						mergedTvgID++
					}
				}
				kept = append(kept, entry)
				count++
			} else {
				removed++
			}
		}
		if count == 0 {
			// All candidates dead — keep the best-quality one as a fallback.
			entry := best.entry
			if id := inheritTvgID(group, 0, validEPGIDs); id != "" && entryTvgID(entry) == "" {
				if parts := strings.SplitN(entry.EXTINFLine, ",", 2); len(parts) == 2 {
					entry.EXTINFLine = parts[0] + ` tvg-id="` + id + `",` + parts[1]
					mergedTvgID++
				}
			}
			kept = append(kept, entry)
			removed--
			fallbackKept++
		}
		groupsProbed++
	}

	if probe != nil && len(probeCands) > 0 && alive != nil {
		aliveCount := 0
		for _, ok := range alive {
			if ok {
				aliveCount++
			}
		}
		logger.Info("Availability probe: %d/%d candidate URLs alive", aliveCount, len(probeCands))
	}
	if removed > 0 {
		logger.Info("DeduplicateByName: processed %d groups, removed %d entries (%d best-quality fallbacks kept)", groupsProbed, removed, fallbackKept)
	}
	if mergedTvgID > 0 {
		logger.Info("DeduplicateByName: inherited tvg-id into %d entries from sibling variants", mergedTvgID)
	}

	var finalLines []string
	finalLines = append(finalLines, headers...)
	for _, e := range kept {
		finalLines = append(finalLines, e.EXTINFLine)
		finalLines = append(finalLines, e.ExtraLines...)
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
			if isStreamURL(trimmed) {
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
// Matching is done twice: exact lowercase name (emoji-stripped) first, then a
// normalized match (quality markers, regional suffixes, "orig" and separators
// stripped) so e.g. "BBC News HD" resolves via EPG display name "BBC News".
// Only lines with a non-empty tvg-id are skipped (tvg-id="" is filled).
// fuzzyCandidate is a normalized EPG display name with its channel id, kept in
// a length-bucketed index for O(length-window) fuzzy lookup.
type fuzzyCandidate struct {
	norm string
	id   string
}

// AddTvgIDsToPlaylist adds tvg-id from EPG name-to-id map to channels that lack it.
// Matching is done in three passes: exact lowercase name (emoji-stripped) first,
// then a normalized match (quality markers, regional suffixes, "orig" and
// separators stripped) so e.g. "BBC News HD" resolves via EPG display name
// "BBC News", and finally a fuzzy edit-distance match (Levenshtein ≤ 1, or ≤ 2
// for names of 10+ runes) to catch typos like "Discoery Channel". Only lines
// with a non-empty tvg-id are skipped (tvg-id="" is filled).
func AddTvgIDsToPlaylist(content string, epgNameToIDMap map[string]string) string {
	lines := strings.Split(content, "\n")
	addedCount := 0

	// Normalized display-name → id lookup, built once from the exact-name map.
	// Exact names take priority when both match; on normalized collisions the
	// first EPG channel wins.
	normalizedMap := make(map[string]string)
	// Length-bucketed index of normalized names for fuzzy lookup.
	byLength := make(map[int][]fuzzyCandidate)
	for name, id := range epgNameToIDMap {
		nk := normalizeChannelName(name)
		if nk == "" {
			continue
		}
		if _, ok := normalizedMap[nk]; !ok {
			normalizedMap[nk] = id
		}
		l := len([]rune(nk))
		byLength[l] = append(byLength[l], fuzzyCandidate{norm: nk, id: id})
	}
	// Deterministic candidate order for fuzzy ties.
	for l := range byLength {
		sort.Slice(byLength[l], func(i, j int) bool { return byLength[l][i].norm < byLength[l][j].norm })
	}

	for i, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), "#EXTINF:") {
			continue
		}
		if m := regTvgID.FindStringSubmatch(line); len(m) > 1 && m[1] != "" {
			continue // уже есть непустой tvg-id
		}

		parts := strings.SplitN(line, ",", 2)
		if len(parts) < 2 {
			continue
		}
		channelName := strings.TrimSpace(parts[1])
		exactName := strings.ToLower(utils.StripTrailingEmoji(channelName))

		tvgID, ok := epgNameToIDMap[exactName]
		if !ok {
			tvgID, ok = normalizedMap[normalizeChannelName(channelName)]
		}
		if !ok {
			tvgID = fuzzyMatchTvgID(channelName, byLength)
			ok = tvgID != ""
		}
		if !ok {
			continue
		}
		lines[i] = fmt.Sprintf(`%s tvg-id="%s",%s`, parts[0], tvgID, parts[1])
		addedCount++
	}

	if addedCount > 0 {
		logger.Info("Added tvg-id to %d channels", addedCount)
	}
	return strings.Join(lines, "\n")
}

// fuzzyMatchTvgID finds the closest EPG display name within edit distance 1
// (or 2 for long names) and returns its channel id. Only names differing in
// length by at most 2 runes are considered, keeping the lookup local to a few
// length buckets and avoiding spurious matches.
func fuzzyMatchTvgID(channelName string, byLength map[int][]fuzzyCandidate) string {
	norm := normalizeChannelName(channelName)
	n := len([]rune(norm))
	if n < 4 {
		return ""
	}
	bestID := ""
	bestDist := 3 // strictly worse than any accepted distance
	for l := n - 2; l <= n+2; l++ {
		for _, cand := range byLength[l] {
			maxDist := 1
			if n >= 10 || l >= 10 {
				maxDist = 2
			}
			d := utils.LevenshteinDistance(norm, cand.norm)
			if d <= maxDist && d < bestDist {
				bestDist = d
				bestID = cand.id
			}
		}
	}
	return bestID
}

// InheritTvgIDsFromSiblings assigns a tvg-id to channels that lack one by
// copying it from another variant of the same channel (same normalized name)
// already present in the playlist — e.g. "Channel SD" inherits the id of
// "Channel HD". Only ids present in validEPGIDs are inherited (nil disables
// validation), so stale/placeholder ids never leak. Entries that already carry
// a non-empty tvg-id are untouched.
func InheritTvgIDsFromSiblings(content string, validEPGIDs map[string]bool) string {
	lines := strings.Split(content, "\n")

	type groupMember struct {
		lineIdx int
		tvgID   string
	}
	groups := make(map[string][]groupMember)
	for i, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), "#EXTINF:") {
			continue
		}
		parts := strings.SplitN(line, ",", 2)
		if len(parts) < 2 {
			continue
		}
		name := normalizeChannelName(parts[1])
		if name == "" {
			continue
		}
		id := ""
		if m := regTvgID.FindStringSubmatch(line); len(m) > 1 {
			id = m[1]
		}
		groups[name] = append(groups[name], groupMember{lineIdx: i, tvgID: id})
	}

	inherited := 0
	for _, members := range groups {
		// First eligible id in playlist order (deterministic).
		inheritedID := ""
		for _, m := range members {
			if m.tvgID == "" {
				continue
			}
			if validEPGIDs == nil || validEPGIDs[m.tvgID] {
				inheritedID = m.tvgID
				break
			}
		}
		if inheritedID == "" {
			continue
		}
		for _, m := range members {
			if m.tvgID != "" {
				continue
			}
			parts := strings.SplitN(lines[m.lineIdx], ",", 2)
			if len(parts) < 2 {
				continue
			}
			lines[m.lineIdx] = fmt.Sprintf(`%s tvg-id="%s",%s`, parts[0], inheritedID, parts[1])
			inherited++
		}
	}

	if inherited > 0 {
		logger.Info("InheritTvgIDsFromSiblings: added tvg-id to %d channels from sibling variants", inherited)
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

	// Per-entry state: pendingEntry is the index of the current entry's EXTINF
	// line in filteredLines (-1 = no open entry), entryHasURL tracks whether the
	// entry has already received its stream URL.
	pendingEntry := -1
	entryHasURL := false
	droppedNoURL := 0

	// closeEntry closes the current entry, dropping it if it never got a stream URL.
	closeEntry := func() {
		if pendingEntry >= 0 && !entryHasURL {
			filteredLines = filteredLines[:pendingEntry]
			droppedNoURL++
		}
		pendingEntry = -1
		entryHasURL = false
	}

	for _, line := range lines {
		if len(line) > 10000 {
			continue
		}

		trimmed := strings.TrimSpace(line)

		// Header line: process EPG URL and always keep; closes any open entry.
		if strings.HasPrefix(trimmed, "#EXTM3U") {
			closeEntry()
			filteredLines = append(filteredLines, processHeader(line, customEPGURL))
			continue
		}

		// Channel entry: filter and normalize.
		if strings.HasPrefix(trimmed, "#EXTINF:") {
			closeEntry()
			result := filterEntry(line, exactMatchLower, substringLower, excludeLower)
			if result.keep {
				filteredLines = append(filteredLines, result.filteredLine)
				pendingEntry = len(filteredLines) - 1
			}
			continue
		}

		// Stream URL line (any scheme, e.g. http, https, rtmp).
		// Attach only the first URL of an entry — extra URL lines are dropped.
		if isStreamURL(trimmed) {
			if pendingEntry >= 0 && !entryHasURL {
				filteredLines = append(filteredLines, line)
				entryHasURL = true
			}
			continue
		}

		// Extra lines (EXTVLCOPT, KODIPROP, blank lines): belong to the current entry.
		if pendingEntry >= 0 {
			filteredLines = append(filteredLines, line)
		} else if trimmed == "" {
			filteredLines = append(filteredLines, line)
		}
	}

	// Close the trailing entry.
	closeEntry()

	if droppedNoURL > 0 {
		logger.Info("Removed %d channel entries without stream URL", droppedNoURL)
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

	// Ленивый `.*?,` останавливается на первой запятой после атрибутов — работает
	// и для `tvg-id="146",Name`, и для форматов с дополнительными атрибутами
	// (`tvg-rec="7",Name`); имя (.+) сохраняет внутренние запятые.
	regCategoriesFile := regexp.MustCompile(`group-title="([^"]+)".*?tvg-id="([^"]+)".*?,(.+)`)
	matches := regCategoriesFile.FindAllStringSubmatch(string(data), -1)
	for _, m := range matches {
		// Эмодзи-пары из имён плейлиста при сопоставлении срезаются (см. ApplyChannelMetadata).
		nameLower := strings.ToLower(utils.StripTrailingEmoji(strings.TrimSpace(m[3])))
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

// ApplyChannelMetadata overrides group-title and tvg-id for channels listed in
// categories.txt. Channel names in the playlist carry emoji pairs (added by
// FilterContent), so they are stripped before matching against the plain-name
// keys of the categories file.
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

		channelName := strings.ToLower(utils.StripTrailingEmoji(strings.TrimSpace(parts[1])))
		meta, ok := categoriesMapping[channelName]
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

// NormalizeCategories rewrites group-title attributes: first applies the alias
// map (provider-specific spellings → canonical names), then keeps only
// categories present in the allow-list; any other category is renamed to
// fallback. This reduces the number of distinct categories in the final
// playlist without removing channels. Non-#EXTINF lines and lines without a
// group-title attribute are left untouched.
func NormalizeCategories(content string, aliases map[string]string, allowed map[string]bool, fallback string) string {
	if len(allowed) == 0 {
		return content
	}

	lines := strings.Split(content, "\n")
	updated := 0
	movedToFallback := 0

	for i, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), "#EXTINF:") {
			continue
		}
		m := regGroupTitle.FindStringSubmatch(line)
		if m == nil || m[1] == "" {
			continue
		}

		original := m[1]
		category := original
		if canonical, ok := aliases[category]; ok {
			category = canonical
		}
		target := category
		if !allowed[category] {
			target = fallback
			movedToFallback++
		}
		if target != original {
			lines[i] = regGroupTitleAttr.ReplaceAllString(line, fmt.Sprintf(`group-title="%s"`, target))
			updated++
		}
	}

	if updated > 0 {
		logger.Info("NormalizeCategories: %d group-titles updated (%d moved to fallback %q)", updated, movedToFallback, fallback)
	}
	return strings.Join(lines, "\n")
}

// ClassifyFallbackCategories reclassifies channels sitting in the fallback
// category ("Основные") into a genre category when their channel name contains
// an unambiguous keyword (e.g. "Спорт": "футбол"). Runs after
// NormalizeCategories so only channels that failed the allow-list are
// considered; category order follows the map's value slice (specific genres
// first) and within a category longer keywords win. Only keywords mapping to
// categories on the allow-list are applied. Categories listed in
// removeCategories are filtered out ENTIRELY (entry + URL dropped), not
// reclassified — used to deny a genre category that the allow-list pass would
// otherwise re-create by name (e.g. «Кино»). Channels without a group-title or
// without a match stay untouched.
func ClassifyFallbackCategories(content string, keywords map[string][]string, allowed map[string]bool, fallback string, removeCategories map[string]bool) string {
	if len(keywords) == 0 {
		return content
	}

	lines := strings.Split(content, "\n")
	// Deterministic category iteration: specific genres (Спорт, Детские, Кино)
	// are checked before broad ones (Познавательные, Развлекательные) so e.g.
	// "Discovery Sport" lands in Спорт, not Познавательные.
	categoryPriority := map[string]int{
		"Спорт": 1, "Детские": 2, "Кино": 3, "Новости": 4, "Музыка": 5,
		"Путешествия": 6, "Природа": 7, "Хобби": 8,
		"Познавательные": 9, "Развлекательные": 10,
	}
	type catRule struct {
		cat      string
		keywords []string
	}
	var rules []catRule
	for cat, words := range keywords {
		if !allowed[cat] && !removeCategories[cat] {
			continue
		}
		rules = append(rules, catRule{cat: cat, keywords: words})
	}
	sort.Slice(rules, func(i, j int) bool {
		pi := categoryPriority[rules[i].cat]
		pj := categoryPriority[rules[j].cat]
		if pi != pj {
			return pi < pj
		}
		return rules[i].cat < rules[j].cat
	})

	classified := 0
	removed := 0
	var out []string
	skipEntry := false // строки удаляемой записи (URL, опции) пропускаются до следующего #EXTINF
	for _, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), "#EXTINF:") {
			if skipEntry {
				continue
			}
			out = append(out, line)
			continue
		}

		skipEntry = false
		gm := regGroupTitle.FindStringSubmatch(line)
		if gm == nil || gm[1] != fallback {
			out = append(out, line)
			continue // только каналы в fallback-категории
		}
		parts := strings.SplitN(line, ",", 2)
		if len(parts) < 2 {
			out = append(out, line)
			continue
		}
		channelName := strings.ToLower(utils.StripTrailingEmoji(parts[1]))
		if channelName == "" {
			out = append(out, line)
			continue
		}
		matchedCat := ""
		for _, rule := range rules {
			if matched := matchCategoryKeyword(channelName, rule.keywords); matched != "" {
				matchedCat = rule.cat
				break
			}
		}
		if matchedCat == "" {
			out = append(out, line)
			continue
		}
		if removeCategories[matchedCat] {
			// Категория запрещена — запись удаляется целиком (EXTINF + URL + опции).
			skipEntry = true
			removed++
			continue
		}
		out = append(out, regGroupTitleAttr.ReplaceAllString(line, fmt.Sprintf(`group-title="%s"`, matchedCat)))
		classified++
	}

	if classified > 0 {
		logger.Info("ClassifyFallbackCategories: reclassified %d channels out of %q", classified, fallback)
	}
	if removed > 0 {
		logger.Info("ClassifyFallbackCategories: removed %d channels in denied categories %v", removed, removeCategories)
	}
	return strings.Join(out, "\n")
}

// matchCategoryKeyword returns the longest keyword found in name, or "".
// Longer keywords are checked first so "viasat sport" wins over "sport" and
// "russia 24" over "24".
//
// Short keywords (<= 3 runes, e.g. "ю", "че", "рок", "mtv") are matched on
// word boundaries only — otherwise "рок" would match inside "Прокопьевск" and
// "ю" inside "Южно-Сахалинск". Longer keywords are morphological stems
// ("музык", "детск", "новост") and keep substring matching.
func matchCategoryKeyword(name string, keywords []string) string {
	var best string
	for _, kw := range keywords {
		kwLower := strings.ToLower(kw)
		matched := false
		if utf8.RuneCountInString(kwLower) <= 3 {
			matched = matchWordBoundary(name, kwLower)
		} else {
			matched = strings.Contains(name, kwLower)
		}
		if matched && len(kwLower) > len(best) {
			best = kwLower
		}
	}
	return best
}

// matchWordBoundary reports whether kw occurs in name as a standalone word
// (surrounded by non-letter runes or string edges). Runewise so Cyrillic
// letters are treated as letters too.
func matchWordBoundary(name, kw string) bool {
	nameRunes := []rune(name)
	kwRunes := []rune(kw)
	if len(kwRunes) == 0 || len(kwRunes) > len(nameRunes) {
		return false
	}
	for i := 0; i+len(kwRunes) <= len(nameRunes); i++ {
		if !equalRuneSlice(nameRunes[i:i+len(kwRunes)], kwRunes) {
			continue
		}
		beforeOK := i == 0 || !unicode.IsLetter(nameRunes[i-1])
		afterOK := i+len(kwRunes) == len(nameRunes) || !unicode.IsLetter(nameRunes[i+len(kwRunes)])
		if beforeOK && afterOK {
			return true
		}
	}
	return false
}

func equalRuneSlice(a, b []rune) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
