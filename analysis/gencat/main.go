package main

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/ozyab/iptv/internal/config"
	"github.com/ozyab/iptv/internal/m3u"
	"github.com/ozyab/iptv/internal/utils"
)

var (
	regGroupTitle = regexp.MustCompile(`group-title="([^"]*)"`)
	regTvgID      = regexp.MustCompile(`tvg-id="([^"]*)"`)
	regChannelID  = regexp.MustCompile(`<channel id="([^"]+)"`)
	regDispName   = regexp.MustCompile(`<display-name[^>]*>([^<]*)</display-name>`)
	regChannelTag = regexp.MustCompile(`(?s)<channel\b.*?</channel>`)

	regQuality  = regexp.MustCompile(`(?i)\b(4k|2160p|uhd|fhd|full\s*hd|fullhd|1080p|720p|576p|480p|hdtv|hd|sd|hq|lq)\b`)
	regRegional = regexp.MustCompile(`\s\+\d+(?:\s+HD)?(?:\s*\([^)]+\))?\s*$`)
	regSpaces   = regexp.MustCompile(`\s+`)
)

func normalize(name string) string {
	s := utils.StripTrailingEmoji(strings.TrimSpace(name))
	s = strings.ToLower(s)
	s = regQuality.ReplaceAllString(s, " ")
	s = strings.ReplaceAll(s, "ᴴ", "")
	s = strings.ReplaceAll(s, "ᴰ", "")
	s = strings.ReplaceAll(s, " orig", "")
	s = regRegional.ReplaceAllString(s, " ")
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.ReplaceAll(s, "-", " ")
	s = regSpaces.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func entryGroup(extinf string) string {
	if m := regGroupTitle.FindStringSubmatch(extinf); len(m) > 1 {
		return m[1]
	}
	return "Общие"
}

func entryID(extinf string) string {
	if m := regTvgID.FindStringSubmatch(extinf); len(m) > 1 {
		return m[1]
	}
	return ""
}

// loadEPGMaps builds plain (exact lowercase) and normalized name→id maps.
func loadEPGMaps(path string) (plain map[string]string, norm map[string]string) {
	f, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		panic(err)
	}
	defer gz.Close()

	var sb strings.Builder
	buf := make([]byte, 1<<20)
	for {
		n, err := gz.Read(buf)
		sb.Write(buf[:n])
		if err == io.EOF {
			break
		}
		if err != nil {
			panic(err)
		}
	}

	plain = make(map[string]string)
	norm = make(map[string]string)
	for _, block := range regChannelTag.FindAllString(sb.String(), -1) {
		im := regChannelID.FindStringSubmatch(block)
		if im == nil {
			continue
		}
		id := im[1]
		for _, dm := range regDispName.FindAllStringSubmatch(block, -1) {
			name := strings.TrimSpace(dm[1])
			if name == "" {
				continue
			}
			lk := strings.ToLower(name)
			if _, ok := plain[lk]; !ok {
				plain[lk] = id
			}
			nk := normalize(name)
			if nk == "" {
				continue
			}
			if _, ok := norm[nk]; !ok {
				norm[nk] = id
			}
		}
	}
	return plain, norm
}

func main() {
	epgPlain, epgNorm := loadEPGMaps("output/original_epg_noarch.xml.gz")
	fmt.Printf("EPG: %d plain names, %d normalized names\n", len(epgPlain), len(epgNorm))

	// Base pool: filtered content (all surviving quality variants) from the
	// merged source playlist. Reuses the exact pipeline deny-lists.
	allData, err := os.ReadFile("output/playlist-all.m3u")
	if err != nil {
		panic(err)
	}
	filtered := m3u.FilterContent(string(allData), config.CategoriesToRemove, config.CategoriesToRemoveSubstring, config.ChannelNamesToExclude, "")
	_, entries := m3u.ParseChannelEntries(strings.Split(filtered, "\n"))
	fmt.Printf("Pool after filter: %d entries\n", len(entries))

	// Pool keyed case-insensitively (ParseCategoriesFile lowercases keys too),
	// keeping the first-seen display name and preferring a non-empty source id.
	type rec struct {
		name    string
		group   string
		existID string
	}
	pool := make(map[string]rec) // key: lowercase emoji-stripped name
	for _, e := range entries {
		parts := strings.SplitN(e.EXTINFLine, ",", 2)
		if len(parts) < 2 {
			continue
		}
		name := utils.StripTrailingEmoji(strings.TrimSpace(parts[1]))
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		id := entryID(e.EXTINFLine)
		if prev, ok := pool[key]; ok {
			if prev.existID == "" && id != "" {
				prev.existID = id
				pool[key] = prev
			}
			continue
		}
		pool[key] = rec{name: name, group: entryGroup(e.EXTINFLine), existID: id}
	}

	// Resolve tvg-id per name: EPG exact → EPG normalized → existing source id.
	type out struct {
		name, group, id string
	}
	resolved := make(map[string]out) // key: lowercase emoji-stripped name
	for key, r := range pool {
		id := ""
		switch {
		case epgPlain[key] != "":
			id = epgPlain[key]
		case epgNorm[normalize(r.name)] != "":
			id = epgNorm[normalize(r.name)]
		case r.existID != "":
			id = r.existID
		}
		if id == "" {
			continue
		}
		resolved[key] = out{name: r.name, group: r.group, id: id}
	}

	// Second pass: cover final-playlist names directly (handles exact-name
	// mismatches between pool variants and the kept variants).
	var finalData []byte
	if d, err := os.ReadFile("output/playlist.m3u"); err == nil {
		finalData = d
		_, finalEntries := m3u.ParseChannelEntries(strings.Split(string(d), "\n"))
		for _, e := range finalEntries {
			parts := strings.SplitN(e.EXTINFLine, ",", 2)
			if len(parts) < 2 {
				continue
			}
			n := utils.StripTrailingEmoji(strings.TrimSpace(parts[1]))
			if n == "" {
				continue
			}
			key := strings.ToLower(n)
			if _, ok := resolved[key]; ok {
				continue
			}
			id := ""
			switch {
			case epgPlain[key] != "":
				id = epgPlain[key]
			case epgNorm[normalize(n)] != "":
				id = epgNorm[normalize(n)]
			case pool[key].existID != "":
				id = pool[key].existID
			}
			if id == "" {
				continue
			}
			g := entryGroup(e.EXTINFLine)
			if r, ok := pool[key]; ok && r.group != "" {
				g = r.group
			}
			resolved[key] = out{name: n, group: g, id: id}
		}
	}

	// Stats.
	var nPlain, nNorm, nSource int
	for _, o := range resolved {
		switch {
		case epgPlain[strings.ToLower(o.name)] != "":
			nPlain++
		case epgNorm[normalize(o.name)] != "":
			nNorm++
		default:
			nSource++
		}
	}
	fmt.Printf("Resolved: %d EPG-exact, %d EPG-normalized, %d source ids\n", nPlain, nNorm, nSource)

	// Coverage of the final playlist.
	if finalData != nil {
		_, finalEntries := m3u.ParseChannelEntries(strings.Split(string(finalData), "\n"))
		covered, total := 0, 0
		for _, e := range finalEntries {
			parts := strings.SplitN(e.EXTINFLine, ",", 2)
			if len(parts) < 2 {
				continue
			}
			total++
			key := strings.ToLower(utils.StripTrailingEmoji(strings.TrimSpace(parts[1])))
			if _, ok := resolved[key]; ok {
				covered++
			}
		}
		fmt.Printf("Final playlist coverage: %d/%d names have a categories.txt entry\n", covered, total)
	}

	// Write categories.txt (sorted by display name).
	names := make([]string, 0, len(resolved))
	for _, o := range resolved {
		names = append(names, o.name)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		o := resolved[strings.ToLower(n)]
		b.WriteString(fmt.Sprintf("group-title=\"%s\" tvg-id=\"%s\",%s\n", o.group, o.id, o.name))
	}
	if err := os.WriteFile("categories.txt", []byte(b.String()), 0644); err != nil {
		panic(err)
	}
	fmt.Printf("Wrote categories.txt: %d entries\n", len(resolved))
}
