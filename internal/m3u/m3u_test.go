package m3u

import (
	"os"
	"strings"
	"testing"

	"github.com/ozyab/iptv/internal/config"
	"github.com/ozyab/iptv/internal/utils"
)

func TestRemoveOrigSuffix(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Channel Name orig", "Channel Name"},
		{"Channel Name ORIG", "Channel Name"},
		{"Channel Name Orig", "Channel Name"},
		{"Channel Name", "Channel Name"},
		{"Orig Channel", "Orig Channel"},
		{"Channel orig extra", "Channel orig extra"},
	}
	for _, tc := range tests {
		result := RemoveOrigSuffix(tc.input)
		if result != tc.expected {
			t.Errorf("RemoveOrigSuffix(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

func TestCountChannels(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1,Channel 1
http://example.com/1
#EXTINF:-1,Channel 2
http://example.com/2
#EXTINF:-1,Channel 3 orig
http://example.com/3`
	if c := CountChannels(content); c != 3 {
		t.Errorf("expected 3 channels, got %d", c)
	}
}

func TestSortPlaylistAlphabetically(t *testing.T) {
	tests := []struct {
		name  string
		input string
		order []string // expected channel name order
	}{
		{
			name: "sort A-Z",
			input: `#EXTM3U
#EXTINF:-1 group-title="Новости",Channel C
http://example.com/c.m3u8
#EXTINF:-1 group-title="Спорт",Channel A
http://example.com/a.m3u8
#EXTINF:-1 group-title="Кино",Channel B
http://example.com/b.m3u8`,
			order: []string{"Channel A", "Channel B", "Channel C"},
		},
		{
			name: "case-insensitive sort",
			input: `#EXTM3U
#EXTINF:-1,channel z
http://example.com/z.m3u8
#EXTINF:-1,Channel A
http://example.com/a.m3u8
#EXTINF:-1,CHANNEL B
http://example.com/b.m3u8`,
			order: []string{"Channel A", "CHANNEL B", "channel z"},
		},
		{
			name: "sorts correctly with emoji suffixes",
			input: `#EXTM3U
#EXTINF:-1,Channel B 🔴🐱
http://example.com/b2.m3u8
#EXTINF:-1,Channel A
http://example.com/a.m3u8
#EXTINF:-1,Channel B 🟠🐶
http://example.com/b1.m3u8`,
			order: []string{"Channel A", "Channel B 🔴🐱", "Channel B 🟠🐶"},
		},
		{
			name: "stable sort keeps original order for equal names",
			input: `#EXTM3U
#EXTINF:-1,Same Name
http://example.com/1.m3u8
#EXTINF:-1,Same Name
http://example.com/2.m3u8`,
			order: []string{"Same Name", "Same Name"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := SortPlaylistAlphabetically(tc.input)

			// Extract channel names in order.
			var names []string
			for _, line := range strings.Split(result, "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "#EXTINF:") {
					parts := strings.SplitN(line, ",", 2)
					if len(parts) > 1 {
						names = append(names, strings.TrimSpace(parts[1]))
					}
				}
			}

			if len(names) != len(tc.order) {
				t.Fatalf("expected %d channels, got %d: %v", len(tc.order), len(names), names)
			}

			for i, expected := range tc.order {
				if names[i] != expected {
					t.Errorf("position %d: expected %q, got %q", i, expected, names[i])
				}
			}
		})
	}
}

func TestRemoveDuplicateURLs(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantURLs  int      // expected number of unique URLs
		checks    []string // substrings that must be present
		notChecks []string // substrings that must NOT be present
	}{
		{
			name: "deduplicate by URL, merge attributes, keep longest name",
			input: `#EXTM3U
#EXTINF:-1 group-title="Общие" tvg-id="1000" tvg-logo="http://epg.one/img/1000.png" tvg-rec="0",ОТР HD
http://example.com/otr.m3u8
#EXTINF:-1 tvg-rec="7",ОТР
http://example.com/otr.m3u8`,
			wantURLs:  1,
			checks:    []string{`group-title="Общие"`, `tvg-id="1000"`, `tvg-logo="http://epg.one/img/1000.png"`, `tvg-rec="0"`, `ОТР HD`},
			notChecks: nil,
		},
		{
			name: "different URLs kept separate",
			input: `#EXTM3U
#EXTINF:-1 group-title="Общие" tvg-id="1000",Channel A
http://example.com/a.m3u8
#EXTINF:-1 group-title="Новости" tvg-id="2000",Channel B
http://example.com/b.m3u8`,
			wantURLs:  2,
			checks:    []string{`Channel A`, `Channel B`, `http://example.com/a.m3u8`, `http://example.com/b.m3u8`},
			notChecks: nil,
		},
		{
			name: "merge fills missing tvg-logo from other entry",
			input: `#EXTM3U
#EXTINF:-1 tvg-id="500" tvg-logo="http://epg.one/img/500.png",Channel X
http://example.com/x.m3u8
#EXTINF:-1 tvg-id="500",Channel X Long Name Version
http://example.com/x.m3u8`,
			wantURLs:  1,
			checks:    []string{`tvg-logo="http://epg.one/img/500.png"`, `Channel X Long Name Version`},
			notChecks: []string{`Channel X\nhttp`}, // only the longest name should appear
		},
		{
			name: "no duplicates passes through unchanged",
			input: `#EXTM3U
#EXTINF:-1 group-title="Общие" tvg-id="100",Channel 1
http://example.com/1.m3u8
#EXTINF:-1 group-title="Новости" tvg-id="200",Channel 2
http://example.com/2.m3u8
#EXTINF:-1 group-title="Спорт" tvg-id="300",Channel 3
http://example.com/3.m3u8`,
			wantURLs:  3,
			checks:    []string{`Channel 1`, `Channel 2`, `Channel 3`},
			notChecks: nil,
		},
		{
			name: "three duplicate URLs merged into one",
			input: `#EXTM3U
#EXTINF:-1 group-title="A" tvg-id="1" tvg-rec="0",Short
http://example.com/same.m3u8
#EXTINF:-1 group-title="B" tvg-rec="5",Medium Name
http://example.com/same.m3u8
#EXTINF:-1 tvg-id="3" tvg-logo="http://logo.png",The Longest Channel Name Here
http://example.com/same.m3u8`,
			wantURLs:  1,
			checks:    []string{`The Longest Channel Name Here`, `tvg-rec="0"`, `group-title="A"`, `tvg-logo="http://logo.png"`},
			notChecks: []string{`Short`, `Medium Name`},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := RemoveDuplicateURLs(tc.input)

			// Count unique URLs in result.
			_, entries := ParseChannelEntries(strings.Split(result, "\n"))
			urls := make(map[string]bool)
			for _, e := range entries {
				for _, extra := range e.ExtraLines {
					if strings.HasPrefix(strings.TrimSpace(extra), "http") {
						urls[strings.TrimSpace(extra)] = true
					}
				}
			}
			if len(urls) != tc.wantURLs {
				t.Errorf("expected %d unique URLs, got %d", tc.wantURLs, len(urls))
			}

			for _, check := range tc.checks {
				if !strings.Contains(result, check) {
					t.Errorf("expected result to contain %q", check)
				}
			}

			for _, notCheck := range tc.notChecks {
				if strings.Contains(result, notCheck) {
					t.Errorf("expected result NOT to contain %q", notCheck)
				}
			}
		})
	}
}

func TestAddEmojiByURL(t *testing.T) {
	tests := []struct {
		name    string
		content string
		checks  []string // channels that should be present
	}{
		{
			name: "deterministic: same URL → same emoji pair",
			content: `#EXTM3U
#EXTINF:-1 tvg-id="711",Channel A
http://example.com/a.m3u8
#EXTINF:-1 tvg-id="162",Channel B
http://example.com/b.m3u8
#EXTINF:-1 tvg-id="711",Channel A
http://example.com/a.m3u8`,
			checks: []string{"Channel A", "Channel B"},
		},
		{
			name: "emoji pair appended to channel name",
			content: `#EXTM3U
#EXTINF:-1 tvg-id="100",Channel X
http://example.com/x.m3u8
#EXTINF:-1 tvg-id="200",Channel Y
http://example.com/y.m3u8`,
			checks: []string{"Channel X", "Channel Y"},
		},
		{
			name: "handles different URLs producing different emojis",
			content: `#EXTM3U
#EXTINF:-1,Channel 1
http://example.com/1.m3u8
#EXTINF:-1,Channel 2
http://example.com/2.m3u8
#EXTINF:-1,Channel 3
http://example.com/3.m3u8`,
			checks: []string{"Channel 1", "Channel 2", "Channel 3"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := AddEmojiByURL(tc.content)

			for _, ch := range tc.checks {
				if !strings.Contains(result, ch) {
					t.Errorf("expected '%s' to be present in result", ch)
				}
			}

			// Verify that original channel names without emoji no longer appear at the end of EXTINF.
			for _, line := range strings.Split(result, "\n") {
				if !strings.HasPrefix(strings.TrimSpace(line), "#EXTINF:") {
					continue
				}
				parts := strings.SplitN(line, ",", 2)
				if len(parts) < 2 {
					continue
				}
				name := strings.TrimSpace(parts[1])
				// Name should have an emoji appended (separated by space).
				if !strings.Contains(name, " ") {
					t.Errorf("expected emoji pair appended to channel name, got: %q", name)
				}
			}
		})
	}
}

func TestUrlToEmojiPairDeterministic(t *testing.T) {
	url1a := "http://example.com/stream1.m3u8"
	url1b := "http://example.com/stream1.m3u8"
	url2 := "http://example.com/stream2.m3u8"

	emoji1a := urlToEmojiPair(url1a)
	emoji1b := urlToEmojiPair(url1b)
	emoji2 := urlToEmojiPair(url2)

	// Same URL → same emoji.
	if emoji1a != emoji1b {
		t.Errorf("same URL should produce same emoji pair: %q vs %q", emoji1a, emoji1b)
	}

	// Different URL → different emoji (highly likely).
	// Not guaranteed but extremely improbable with 1600+ combinations.
	if emoji1a == emoji2 {
		t.Logf("note: different URLs produced same emoji pair %q (possible but unlikely)", emoji1a)
	}

	// Emoji pair should contain at least 2 non-ASCII characters (visual emojis).
	// Note: some emojis like ☀️ use variation selectors (U+FE0F), so rune count may be >2.
	if len(emoji1a) < 4 {
		t.Errorf("expected emoji pair to have at least 4 bytes, got %d: %q", len(emoji1a), emoji1a)
	}
}

func TestUrlToEmojiPairUsesHostnameAndPath(t *testing.T) {
	// Первый эмодзи (по hostname) одинаков для одного DNS-имени, порт не влияет.
	h1 := emojiFromHostname("http://cdn.example.com:8080/live/ch1.m3u8?token=abc")
	h2 := emojiFromHostname("http://cdn.example.com/live/ch2.m3u8?token=def")
	if h1 != h2 {
		t.Errorf("same hostname should produce same first emoji: %q vs %q", h1, h2)
	}

	// Разные hostname — почти наверняка разные первые эмодзи.
	h3 := emojiFromHostname("http://cdn.other.net/live/ch1.m3u8")
	if h1 == h3 {
		t.Logf("note: different hostnames produced same emoji %q (possible but unlikely)", h1)
	}

	// Второй эмодзи (по path) различается для разных путей на одном хосте.
	p1 := emojiFromPath("http://cdn.example.com/live/ch1.m3u8")
	p2 := emojiFromPath("http://cdn.example.com/live/ch2.m3u8")
	if p1 == p2 {
		t.Logf("note: different paths produced same emoji %q (possible but unlikely)", p1)
	}

	// Одинаковый путь на разных хостах → одинаковый второй эмодзи.
	p3 := emojiFromPath("http://cdn.other.net/live/ch1.m3u8")
	if p1 != p3 {
		t.Errorf("same path should produce same second emoji: %q vs %q", p1, p3)
	}

	// Query-строка не должна влиять на второй эмодзи.
	p4 := emojiFromPath("http://cdn.example.com/live/ch1.m3u8?token=xyz")
	if p1 != p4 {
		t.Errorf("query string must not affect path emoji: %q vs %q", p1, p4)
	}
}

func TestUrlComponentFallback(t *testing.T) {
	// Некорректный URL не должен паниковать и должен давать fallback.
	if got := urlComponent("not a url ://", true); got == "" {
		t.Error("expected non-empty fallback for malformed URL")
	}
	// Относительный URL (без хоста) → fallback на всю строку.
	if got := urlComponent("relative/path.m3u8", false); got != "relative/path.m3u8" {
		t.Errorf("expected raw fallback for relative URL, got %q", got)
	}
	// Hostname: порт отбрасывается, DNS-имя приводится к нижнему регистру.
	if got := urlComponent("http://CDN.Example.COM:8080/x", true); got != "cdn.example.com" {
		t.Errorf("expected lowercased hostname without port, got %q", got)
	}
	// Пустой path → "/".
	if got := urlComponent("http://cdn.example.com", false); got != "/" {
		t.Errorf("expected \"/\" for empty path, got %q", got)
	}
}

func TestEmojiPoolsExpanded(t *testing.T) {
	if len(emojiPoolA) < 100 {
		t.Errorf("emojiPoolA must have at least 100 emojis, got %d", len(emojiPoolA))
	}
	if len(emojiPoolB) < 100 {
		t.Errorf("emojiPoolB must have at least 100 emojis, got %d", len(emojiPoolB))
	}
	// Внутри пула не должно быть дубликатов (они бы тратили комбинации).
	for name, pool := range map[string][]string{"A": emojiPoolA, "B": emojiPoolB} {
		seen := make(map[string]bool)
		for _, e := range pool {
			if seen[e] {
				t.Errorf("duplicate emoji in pool %s: %q", name, e)
			}
			seen[e] = true
		}
	}
}

func TestFilterContentWithCategories(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 group-title="Взрослые",Adult Channel
http://example.com/adult
#EXTINF:-1 group-title="Россия | Russia",Channel 1
http://example.com/1
#EXTINF:-1 group-title="Развлекательные",Channel 2
http://example.com/2`

	result := FilterContent(content, []string{"Взрослые"}, nil, nil, "")

	if strings.Contains(result, "Adult Channel") {
		t.Error("expected Adult Channel to be filtered out")
	}
	if !strings.Contains(result, "Channel 1") {
		t.Error("expected Channel 1 to be kept")
	}
	if !strings.Contains(result, "Channel 2") {
		t.Error("expected Channel 2 to be kept")
	}
}

func TestFilterContentRemovesOrigSuffix(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 group-title="Россия | Russia",Channel 1 orig
http://example.com/1
#EXTINF:-1 group-title="Развлекательные",Channel 2 orig
http://example.com/2`

	result := FilterContent(content, nil, nil, nil, "")

	if !strings.Contains(result, "Channel 1") {
		t.Error("expected 'Channel 1' in result")
	}
	if !strings.Contains(result, "Channel 2") {
		t.Error("expected 'Channel 2' in result")
	}
	if strings.Contains(result, "orig") {
		t.Error("expected no 'orig' suffix in result")
	}
}

func TestFilterContentExcludesRegionalChannels(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 group-title="Россия | Russia",Channel 1
http://example.com/1
#EXTINF:-1 group-title="Россия | Russia",Channel +1 (Приволжье)
http://example.com/plus1
#EXTINF:-1 group-title="Россия | Russia",Channel +4 (Алтай)
http://example.com/plus4
#EXTINF:-1 group-title="Россия | Russia",Channel +5 HD
http://example.com/plus5hd
#EXTINF:-1 group-title="Россия | Russia",Channel +7 not regional
http://example.com/plus7
#EXTINF:-1 group-title="Россия | Russia",Channel HD 50
http://example.com/50
#EXTINF:-1 group-title="Россия | Russia",Channel 25
http://example.com/25
#EXTINF:-1 group-title="Россия | Russia",Normal Channel
http://example.com/normal`

	result := FilterContent(content, nil, nil, nil, "")

	if !strings.Contains(result, "Channel 1") {
		t.Error("expected 'Channel 1' in result")
	}
	if strings.Contains(result, "+1 (Приволжье)") {
		t.Error("expected regional channel +1 to be excluded")
	}
	if strings.Contains(result, "+4 (Алтай)") {
		t.Error("expected regional channel +4 to be excluded")
	}
	if strings.Contains(result, "+5 HD") {
		t.Error("expected regional channel +5 HD to be excluded")
	}
	if !strings.Contains(result, "Channel +7 not regional") {
		t.Error("expected '+7 not regional' to be kept")
	}
	if strings.Contains(result, "HD 50") {
		t.Error("expected 'HD 50' to be excluded")
	}
	if strings.Contains(result, "Channel 25") {
		t.Error("expected 'Channel 25' to be excluded")
	}
	if !strings.Contains(result, "Normal Channel") {
		t.Error("expected 'Normal Channel' to be kept")
	}
}

func TestFilterContentExcludesChannelsByName(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 group-title="Россия | Russia",Fashion TV
http://example.com/fashion1
#EXTINF:-1 group-title="Россия | Russia",Russian Fashion
http://example.com/fashion2
#EXTINF:-1 group-title="Россия | Russia",News Channel
http://example.com/news
#EXTINF:-1 group-title="Россия | Russia",Sports Channel
http://example.com/sports`

	result := FilterContent(content, nil, nil, []string{"Fashion"}, "")

	if strings.Contains(result, "Fashion TV") {
		t.Error("expected 'Fashion TV' to be excluded")
	}
	if strings.Contains(result, "Russian Fashion") {
		t.Error("expected 'Russian Fashion' to be excluded")
	}
	if !strings.Contains(result, "News Channel") {
		t.Error("expected 'News Channel' to be kept")
	}
	if !strings.Contains(result, "Sports Channel") {
		t.Error("expected 'Sports Channel' to be kept")
	}
}

func TestFilterContentCaseInsensitiveExclusion(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 group-title="Россия | Russia",FASHION TV
http://example.com/fashion1
#EXTINF:-1 group-title="Россия | Russia",fashion news
http://example.com/fashion2
#EXTINF:-1 group-title="Россия | Russia",FaShIoN Channel
http://example.com/fashion3
#EXTINF:-1 group-title="Россия | Russia",Regular Channel
http://example.com/regular`

	result := FilterContent(content, nil, nil, []string{"Fashion"}, "")

	if strings.Contains(result, "FASHION TV") {
		t.Error("expected 'FASHION TV' to be excluded")
	}
	if strings.Contains(result, "fashion news") {
		t.Error("expected 'fashion news' to be excluded")
	}
	if strings.Contains(result, "FaShIoN Channel") {
		t.Error("expected 'FaShIoN Channel' to be excluded")
	}
	if !strings.Contains(result, "Regular Channel") {
		t.Error("expected 'Regular Channel' to be kept")
	}
}

func TestFilterContentMultipleExclusions(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 group-title="Россия | Russia",Fashion TV
http://example.com/fashion
#EXTINF:-1 group-title="Россия | Russia",Adult Channel
http://example.com/adult
#EXTINF:-1 group-title="Россия | Russia",Gambling Network
http://example.com/gambling
#EXTINF:-1 group-title="Россия | Russia",Regular Channel
http://example.com/regular`

	result := FilterContent(content, nil, nil, []string{"Fashion", "Adult", "Gambling"}, "")

	if strings.Contains(result, "Fashion TV") {
		t.Error("expected 'Fashion TV' to be excluded")
	}
	if strings.Contains(result, "Adult Channel") {
		t.Error("expected 'Adult Channel' to be excluded")
	}
	if strings.Contains(result, "Gambling Network") {
		t.Error("expected 'Gambling Network' to be excluded")
	}
	if !strings.Contains(result, "Regular Channel") {
		t.Error("expected 'Regular Channel' to be kept")
	}
}

func TestFilterContentNormalizesGroupTitle(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 group-title="15.𝐊𝐢𝐧𝐨 📽",Movie Channel
http://example.com/movie
#EXTINF:-1 group-title="20.𝕂иℍ𝕠 🇷🇺",Cinema Channel
http://example.com/cinema
#EXTINF:-1 group-title="1.✨ ЕКБ 🦎",Local Channel
http://example.com/local
#EXTINF:-1 group-title="60.Квант-Телеком 🎡",Tech Channel
http://example.com/tech
#EXTINF:-1 group-title="Россия | Россия",Normal
http://example.com/normal
#EXTINF:-1 group-title="Тест 🛒 🎧",Shop Music
http://example.com/shop`

	result := FilterContent(content, nil, nil, nil, "")

	// Все каналы должны остаться (фильтрация не настроена).
	// group-title должен быть очищен: цифры + эмодзи.
	tests := []struct {
		name     string
		channel  string
		oldGroup string // should NOT be present
		newGroup string // SHOULD be present (cleaned)
	}{
		{"кино", "Movie Channel", `group-title="15.𝐊𝐢𝐧𝐨 📽"`, `group-title="𝐊𝐢𝐧𝐨"`},
		{"кино 2", "Cinema Channel", `group-title="20.𝕂иℍ𝕠 🇷🇺"`, `group-title="𝕂иℍ𝕠"`},
		{"екб", "Local Channel", `group-title="1.✨ ЕКБ 🦎"`, `group-title="✨ ЕКБ"`},
		{"квант", "Tech Channel", `group-title="60.Квант-Телеком 🎡"`, `group-title="Квант-Телеком"`},
		{"обычная", "Normal", `group-title="Россия"`, `group-title="Россия | Россия"`},
		{"мульти-эмодзи", "Shop Music", `group-title="Тест 🛒 🎧"`, `group-title="Тест"`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(result, tc.channel) {
				t.Errorf("expected '%s' to be kept in result", tc.channel)
			}
			if strings.Contains(result, tc.oldGroup) {
				t.Errorf("expected old group-title %q to be stripped, but found in result", tc.oldGroup)
			}
			if !strings.Contains(result, tc.newGroup) {
				t.Errorf("expected cleaned group-title %q in result", tc.newGroup)
			}
		})
	}
}

func TestFilterContentKeepsURLAfterEXTVLCOPT(t *testing.T) {
	// URL после #EXTVLCOPT / #KODIPROP-строк должен привязываться к записи
	// и не теряться (фикс спаривания строк).
	content := `#EXTM3U
#EXTINF:-1 group-title="Wink (VPN 🇷🇺)",Первый канал
#EXTVLCOPT:http-user-agent=WINK/1.130.1 (AndroidTV/9) HlsWinkPlayer
https://zabava-htlive.cdn.ngenix.net/hls/CH_1TVSD/variant.m3u8
#EXTINF:-1 group-title="Россия | Russia",Обычный канал
#KODIPROP:inputstream.adaptive.license_type=com.widevine.alpha
http://example.com/normal.m3u8
#EXTINF:-1 group-title="Россия | Russia",Канал без доп. строк
http://example.com/plain.m3u8
#EXTINF:-1 group-title="Россия | Russia",Канал с пустой строкой

https://example.com/blankline.m3u8
#EXTINF:-1 group-title="Россия | Russia",Канал с двумя URL
http://example.com/first.m3u8
http://example.com/second.m3u8`

	result := FilterContent(content, nil, nil, nil, "")

	for _, want := range []string{
		"Первый канал",
		"https://zabava-htlive.cdn.ngenix.net/hls/CH_1TVSD/variant.m3u8",
		`#EXTVLCOPT:http-user-agent=WINK/1.130.1 (AndroidTV/9) HlsWinkPlayer`,
		"Обычный канал",
		"http://example.com/normal.m3u8",
		"Канал без доп. строк",
		"http://example.com/plain.m3u8",
		"Канал с пустой строкой",
		"https://example.com/blankline.m3u8",
		"Канал с двумя URL",
		"http://example.com/first.m3u8",
	} {
		if !strings.Contains(result, want) {
			t.Errorf("expected result to contain %q", want)
		}
	}

	// Второй URL записи должен быть отброшен (первый URL привязывается).
	if strings.Contains(result, "http://example.com/second.m3u8") {
		t.Error("expected second URL line to be dropped")
	}

	if c := CountChannels(result); c != 5 {
		t.Errorf("expected 5 channels, got %d", c)
	}
}

func TestFilterContentDropsEntryWithoutURL(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 group-title="Россия | Russia",Канал с URL
http://example.com/ok.m3u8
#EXTINF:-1 group-title="Wink (VPN 🇷🇺)",Мёртвая запись
#EXTINF:-1 group-title="Россия | Russia",Ещё канал с URL
https://example.com/ok2.m3u8
#EXTINF:-1 group-title="X",Хвостовая мёртвая запись`

	result := FilterContent(content, nil, nil, nil, "")

	if !strings.Contains(result, "Канал с URL") {
		t.Error("expected 'Канал с URL' to be kept")
	}
	if strings.Contains(result, "Мёртвая запись") {
		t.Error("expected URL-less entry 'Мёртвая запись' to be dropped")
	}
	if !strings.Contains(result, "Ещё канал с URL") {
		t.Error("expected 'Ещё канал с URL' to be kept")
	}
	if strings.Contains(result, "Хвостовая мёртвая запись") {
		t.Error("expected trailing URL-less entry to be dropped")
	}
	if c := CountChannels(result); c != 2 {
		t.Errorf("expected 2 channels, got %d", c)
	}
}

func TestFilterContentKeepsRtmpURL(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 group-title="Россия | Russia",Rtmp канал
rtmp://cdn.example.com/live/ch1
#EXTINF:-1 group-title="Россия | Russia",Http канал
http://example.com/ch2.m3u8`

	result := FilterContent(content, nil, nil, nil, "")

	if !strings.Contains(result, "Rtmp канал") {
		t.Error("expected rtmp channel to be kept")
	}
	if !strings.Contains(result, "rtmp://cdn.example.com/live/ch1") {
		t.Error("expected rtmp URL to be kept")
	}
	if !strings.Contains(result, "Http канал") {
		t.Error("expected http channel to be kept")
	}
	if c := CountChannels(result); c != 2 {
		t.Errorf("expected 2 channels, got %d", c)
	}
}

func TestAddTvgIDsToPlaylistWithEmojiNames(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 group-title="A",Первый канал 🔴🐱
http://example.com/1.m3u8
#EXTINF:-1 group-title="A",Канал Без Эмодзи
http://example.com/2.m3u8
#EXTINF:-1 group-title="A" tvg-id="999",Уже с ID
http://example.com/3.m3u8`

	epgMap := map[string]string{
		"первый канал":     "100",
		"канал без эмодзи": "200",
	}

	result := AddTvgIDsToPlaylist(content, epgMap)

	if !strings.Contains(result, `tvg-id="100"`) {
		t.Error("expected tvg-id to be added to emoji-suffixed name")
	}
	if !strings.Contains(result, `tvg-id="200"`) {
		t.Error("expected tvg-id to be added to plain name")
	}
	if !strings.Contains(result, `tvg-id="999"`) {
		t.Error("expected existing tvg-id to be preserved")
	}
	if !strings.Contains(result, "Первый канал 🔴🐱") {
		t.Error("expected original name with emoji to be preserved in output")
	}
}

func TestDeduplicateByNameQualityPriority(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 group-title="A",Канал HD
http://example.com/hd.m3u8
#EXTINF:-1 group-title="A",Канал SD
http://example.com/sd.m3u8
#EXTINF:-1 group-title="B",Одинокий канал
http://example.com/one.m3u8`

	result := DeduplicateByName(content, 1, nil, nil)

	if !strings.Contains(result, "Канал HD") {
		t.Error("expected HD variant to be kept")
	}
	if strings.Contains(result, "Канал SD") {
		t.Error("expected SD variant to be removed")
	}
	if !strings.Contains(result, "Одинокий канал") {
		t.Error("expected single-variant channel to stay untouched")
	}
	if c := CountChannels(result); c != 2 {
		t.Errorf("expected 2 channels, got %d", c)
	}
}

func TestDeduplicateByNameStripsEmojiAndUnicodeHD(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 group-title="A",Канал HD 🔴🐱
http://example.com/hd.m3u8
#EXTINF:-1 group-title="A",КАНАЛᴴᴰ
http://example.com/uhd.m3u8
#EXTINF:-1 group-title="A",Канал SD 🔵💧
http://example.com/sd.m3u8`

	result := DeduplicateByName(content, 1, nil, nil)

	// "Канал HD" и "КАНАЛᴴᴰ" оба ранг 2 (HD) — остаётся первый по порядку.
	if !strings.Contains(result, "Канал HD 🔴🐱") {
		t.Error("expected HD variant (with emoji) to be kept")
	}
	if strings.Contains(result, "КАНАЛᴴᴰ") {
		t.Error("expected unicode-HD variant to be deduplicated")
	}
	if strings.Contains(result, "Канал SD") {
		t.Error("expected SD variant to be removed")
	}
	if c := CountChannels(result); c != 1 {
		t.Errorf("expected 1 channel, got %d", c)
	}
}

func TestDeduplicateByNameProbeSelection(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 group-title="A",Канал 4K
http://example.com/4k.m3u8
#EXTINF:-1 group-title="A",Канал HD
http://example.com/hd.m3u8
#EXTINF:-1 group-title="A",Канал SD
http://example.com/sd.m3u8`

	probe := func(cands []utils.ProbeCandidate) map[string]bool {
		m := make(map[string]bool)
		for _, c := range cands {
			m[c.URL] = strings.Contains(c.URL, "hd")
		}
		return m
	}

	result := DeduplicateByName(content, 1, probe, nil)

	// Мёртвый 4K пропускается в пользу живого HD, несмотря на качество.
	if !strings.Contains(result, "Канал HD") {
		t.Error("expected alive HD variant to be kept")
	}
	if strings.Contains(result, "Канал 4K") {
		t.Error("expected dead 4K variant to be removed")
	}
	if strings.Contains(result, "Канал SD") {
		t.Error("expected dead SD variant to be removed")
	}
	if c := CountChannels(result); c != 1 {
		t.Errorf("expected 1 channel, got %d", c)
	}
}

func TestDeduplicateByNameDoesNotProbeSingleVariants(t *testing.T) {
	var probed []string
	content := `#EXTM3U
#EXTINF:-1 group-title="A",Дубль HD
http://example.com/d1.m3u8
#EXTINF:-1 group-title="A",Дубль SD
http://example.com/d2.m3u8
#EXTINF:-1 group-title="B",Одиночка
http://example.com/single.m3u8`

	probe := func(cands []utils.ProbeCandidate) map[string]bool {
		for _, c := range cands {
			probed = append(probed, c.URL)
		}
		m := make(map[string]bool)
		for _, c := range cands {
			m[c.URL] = true
		}
		return m
	}

	DeduplicateByName(content, 1, probe, nil)

	for _, u := range probed {
		if u == "http://example.com/single.m3u8" {
			t.Error("single-variant URL should not be probed")
		}
	}
}

func TestDeduplicateByNameMaxVariants(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 group-title="A",Канал FHD
http://example.com/fhd.m3u8
#EXTINF:-1 group-title="A",Канал HD
http://example.com/hd.m3u8
#EXTINF:-1 group-title="A",Канал SD
http://example.com/sd.m3u8`

	result := DeduplicateByName(content, 2, nil, nil)

	if c := CountChannels(result); c != 2 {
		t.Errorf("expected 2 channels, got %d", c)
	}
	if !strings.Contains(result, "Канал FHD") || !strings.Contains(result, "Канал HD") {
		t.Error("expected FHD and HD variants to be kept")
	}
	if strings.Contains(result, "Канал SD") {
		t.Error("expected SD variant to be removed")
	}
}

func TestDeduplicateByNameAllDeadFallback(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 group-title="A",Канал 4K
http://example.com/4k.m3u8
#EXTINF:-1 group-title="A",Канал SD
http://example.com/sd.m3u8`

	probe := func(cands []utils.ProbeCandidate) map[string]bool {
		m := make(map[string]bool)
		for _, c := range cands {
			m[c.URL] = false
		}
		return m
	}

	result := DeduplicateByName(content, 1, probe, nil)

	if !strings.Contains(result, "Канал 4K") {
		t.Error("expected best-quality fallback to be kept when all variants are dead")
	}
	if c := CountChannels(result); c != 1 {
		t.Errorf("expected 1 channel, got %d", c)
	}
}

func TestDeduplicateByNameSkipsNonHTTPProbe(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 group-title="A",Канал
rtmp://cdn.example.com/live/1
#EXTINF:-1 group-title="A",Канал HD
http://example.com/dead.m3u8`

	probe := func(cands []utils.ProbeCandidate) map[string]bool {
		if len(cands) != 1 || cands[0].URL != "http://example.com/dead.m3u8" {
			t.Errorf("unexpected probe URLs: %v", cands)
		}
		return map[string]bool{cands[0].URL: false}
	}

	result := DeduplicateByName(content, 1, probe, nil)

	// rtmp не пробируется (считается живым), мёртвый HD удаляется,
	// несмотря на то что HD по качеству выше.
	if !strings.Contains(result, "Канал") {
		t.Error("expected rtmp variant to be kept")
	}
	if strings.Contains(result, "Канал HD") {
		t.Error("expected dead HD variant to be removed")
	}
	if c := CountChannels(result); c != 1 {
		t.Errorf("expected 1 channel, got %d", c)
	}
}

func TestDeduplicateByNamePassesUserAgent(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 group-title="A",Канал HD
#EXTVLCOPT:http-user-agent=WINK/1.130.1 (AndroidTV/9) HlsWinkPlayer
#EXTVLCOPT:http-referrer=https://televizor24tochka.ru/
https://example.com/hd.m3u8
#EXTINF:-1 group-title="A",Канал SD
http://example.com/sd.m3u8`

	var gotUA, gotReferer string
	probe := func(cands []utils.ProbeCandidate) map[string]bool {
		m := make(map[string]bool)
		for _, c := range cands {
			if c.URL == "https://example.com/hd.m3u8" {
				gotUA = c.UserAgent
				gotReferer = c.Referer
			}
			m[c.URL] = true
		}
		return m
	}

	DeduplicateByName(content, 1, probe, nil)

	if gotUA != "WINK/1.130.1 (AndroidTV/9) HlsWinkPlayer" {
		t.Errorf("expected #EXTVLCOPT user-agent to be passed to probe, got %q", gotUA)
	}
	if gotReferer != "https://televizor24tochka.ru/" {
		t.Errorf("expected #EXTVLCOPT referrer to be passed to probe, got %q", gotReferer)
	}
}

func TestDeduplicateByNamePreservesExtraLines(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 group-title="A",Канал HD
#EXTVLCOPT:http-user-agent=WINK/1.0
https://example.com/hd.m3u8
#EXTINF:-1 group-title="A",Канал SD
http://example.com/sd.m3u8`

	result := DeduplicateByName(content, 1, nil, nil)

	if !strings.Contains(result, "#EXTVLCOPT:http-user-agent=WINK/1.0") {
		t.Error("expected EXTVLCOPT line preserved with the kept variant")
	}
	if !strings.Contains(result, "https://example.com/hd.m3u8") {
		t.Error("expected kept variant URL to be preserved")
	}
	if strings.Contains(result, "Канал SD") {
		t.Error("expected SD variant to be removed")
	}
}

func TestDeduplicateByNameMergesTvgID(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 group-title="A",Канал HD
http://example.com/hd.m3u8
#EXTINF:-1 group-title="A" tvg-id="123",Канал SD
http://example.com/sd.m3u8`

	valid := map[string]bool{"123": true}
	result := DeduplicateByName(content, 1, nil, valid)

	if !strings.Contains(result, `tvg-id="123"`) {
		t.Error("expected tvg-id to be inherited from sibling variant")
	}
	if !strings.Contains(result, "Канал HD") {
		t.Error("expected HD variant to be kept")
	}
	if strings.Contains(result, "Канал SD") {
		t.Error("expected SD variant to be removed")
	}
	if c := CountChannels(result); c != 1 {
		t.Errorf("expected 1 channel, got %d", c)
	}
}

func TestDeduplicateByNameIgnoresStaleSiblingID(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 group-title="A",Канал HD
http://example.com/hd.m3u8
#EXTINF:-1 group-title="A" tvg-id="stale",Канал SD
http://example.com/sd.m3u8`

	// validEPGIDs не содержит "stale" → наследовать нельзя.
	valid := map[string]bool{"456": true}
	result := DeduplicateByName(content, 1, nil, valid)

	if strings.Contains(result, `tvg-id="stale"`) {
		t.Error("expected stale sibling tvg-id NOT to be inherited")
	}
	if !strings.Contains(result, "Канал HD") {
		t.Error("expected HD variant to be kept")
	}
	for _, line := range strings.Split(result, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#EXTINF:") && strings.Contains(line, "Канал HD") {
			if strings.Contains(line, "tvg-id=") {
				t.Error("expected no tvg-id on winner when sibling id is stale")
			}
		}
	}
}

func TestDeduplicateByNameMergesAnyIDWithoutEPGSet(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 group-title="A",Канал HD
http://example.com/hd.m3u8
#EXTINF:-1 group-title="A" tvg-id="777",Канал SD
http://example.com/sd.m3u8`

	// validEPGIDs = nil → наследуется любой непустой id.
	result := DeduplicateByName(content, 1, nil, nil)

	if !strings.Contains(result, `tvg-id="777"`) {
		t.Error("expected any sibling tvg-id to be inherited when EPG set is nil")
	}
	if !strings.Contains(result, "Канал HD") {
		t.Error("expected HD variant to be kept")
	}
	if c := CountChannels(result); c != 1 {
		t.Errorf("expected 1 channel, got %d", c)
	}
}

func TestDeduplicateByNameMergePreservesNameAndEmoji(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 group-title="A",Канал HD 🔴🐱
http://example.com/hd.m3u8
#EXTINF:-1 group-title="A" tvg-id="123",Канал SD
http://example.com/sd.m3u8`

	valid := map[string]bool{"123": true}
	result := DeduplicateByName(content, 1, nil, valid)

	if !strings.Contains(result, "Канал HD 🔴🐱") {
		t.Error("expected kept variant name with emoji to be preserved")
	}
	if !strings.Contains(result, `tvg-id="123"`) {
		t.Error("expected tvg-id to be inherited")
	}
	if c := CountChannels(result); c != 1 {
		t.Errorf("expected 1 channel, got %d", c)
	}
}

func TestParseCategoriesFileStripsEmojiKeys(t *testing.T) {
	file := t.TempDir() + "/categories.txt"
	content := `group-title="Общие" tvg-id="146",Первый канал HD
` +
		`group-title="Общие" tvg-id="711",Россия 1 🔴🐱
` +
		`group-title="Общие" tvg-id="2034" tvg-rec="3",Первый канал +2
` +
		`group-title="Кино" tvg-id="999",Канал, с запятой в имени`
	if err := os.WriteFile(file, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	m := ParseCategoriesFile(file)
	if _, ok := m["первый канал hd"]; !ok {
		t.Error("expected plain name key to be present")
	}
	if _, ok := m["россия 1"]; !ok {
		t.Error("expected emoji-suffixed name to be stripped to plain key")
	}
	if _, ok := m["россия 1 🔴🐱"]; ok {
		t.Error("expected emoji NOT to be part of the key")
	}
	// Legacy format with extra attributes (tvg-rec) must still parse.
	if got := m["первый канал +2"]; got == nil || got["tvg_id"] != "2034" {
		t.Errorf("expected legacy tvg-rec format to parse, got %v", got)
	}
	// Names containing commas must be captured whole (first-comma split).
	if got := m["канал, с запятой в имени"]; got == nil || got["tvg_id"] != "999" {
		t.Errorf("expected comma-containing name to be captured whole, got %v", got)
	}
}

func TestApplyChannelMetadataStripsEmoji(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 group-title="Старая группа",Первый канал HD 🔴🐱
http://example.com/1.m3u8
#EXTINF:-1 group-title="A",Без эмодзи
http://example.com/2.m3u8`

	mapping := map[string]map[string]string{
		"первый канал hd": {"group": "Общие", "tvg_id": "146"},
		"без эмодзи":      {"group": "Общие", "tvg_id": "711"},
	}

	result := ApplyChannelMetadata(content, mapping)

	if !strings.Contains(result, `group-title="Общие"`) || !strings.Contains(result, `tvg-id="146"`) {
		t.Error("expected metadata to be applied to emoji-suffixed name")
	}
	if !strings.Contains(result, "Первый канал HD 🔴🐱") {
		t.Error("expected original name with emoji to be preserved in output")
	}
	if !strings.Contains(result, `tvg-id="711"`) {
		t.Error("expected metadata to be applied to plain name")
	}
	if strings.Contains(result, `group-title="Старая группа"`) {
		t.Error("expected old group-title to be overridden")
	}
}

func TestFilterContentWithCategorySubstring(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 group-title="Кино и Сериалы",Movie Channel
http://example.com/movie
#EXTINF:-1 group-title="ДЕТСКИЕ 👶",Kids Channel
http://example.com/kids
#EXTINF:-1 group-title="Россия | Russia",News Channel
http://example.com/news
#EXTINF:-1 group-title="Спорт TM",Sports Channel
http://example.com/sports
#EXTINF:-1 group-title="TEST* 👈",Test Channel
http://example.com/test`

	result := FilterContent(content, nil, []string{"кино", "детск", "спорт", "test"}, nil, "")

	tests := []struct {
		name    string
		channel string
		kept    bool // true = should remain, false = should be filtered out
	}{
		{"кино категория", "Movie Channel", false},
		{"детские категория", "Kids Channel", false},
		{"спорт категория", "Sports Channel", false},
		{"test категория", "Test Channel", false},
		{"обычный канал", "News Channel", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			present := strings.Contains(result, tc.channel)
			if tc.kept && !present {
				t.Errorf("expected '%s' to be kept, but it was filtered out", tc.channel)
			}
			if !tc.kept && present {
				t.Errorf("expected '%s' to be filtered out, but it was kept", tc.channel)
			}
		})
	}
}

func TestNormalizeCategories(t *testing.T) {
	aliases := map[string]string{
		"РЕГИОНАЛЬНЫЕ":       "Региональные",
		"ЭФИРНЫЕ ⓵":          "Эфирные",
		"Германия | Germany": "Германия",
		"NEWS 🆕":             "Новости",
		"Onair8k2*":          "Onair8k2",
		"*MUZICA":            "Музыка",
	}
	allowed := map[string]bool{
		"Региональные": true, "Эфирные": true, "Германия": true, "Новости": true,
		"Познавательные": true, "Музыка": true, "Основные": true,
	}
	const fallback = "Основные"

	input := `#EXTM3U
#EXTINF:-1 group-title="РЕГИОНАЛЬНЫЕ" tvg-id="100",Channel A
http://example.com/a.m3u8
#EXTINF:-1 group-title="ЭФИРНЫЕ ⓵",Channel B
http://example.com/b.m3u8
#EXTINF:-1 group-title="Познавательные" tvg-id="300" tvg-logo="http://example.com/logo.png",Channel C
http://example.com/c.m3u8
#EXTINF:-1 group-title="TEST*" tvg-id="400",Channel D
http://example.com/d.m3u8
#EXTINF:-1 group-title="Onair8k2*",Channel E
http://example.com/e.m3u8
#EXTINF:-1 group-title="*MUZICA",Channel F
http://example.com/f.m3u8
#EXTINF:-1 group-title="Германия | Germany",Channel G
http://example.com/g.m3u8`

	want := map[string]string{
		"Channel A": `group-title="Региональные"`,
		"Channel B": `group-title="Эфирные"`,
		"Channel C": `group-title="Познавательные"`,
		"Channel D": `group-title="Основные"`,
		"Channel E": `group-title="Основные"`, // alias Onair8k2* → Onair8k2 не в allow-list → fallback
		"Channel F": `group-title="Музыка"`,
		"Channel G": `group-title="Германия"`,
	}

	got := NormalizeCategories(input, aliases, allowed, fallback)

	for ch, wantAttr := range want {
		line := ""
		for _, l := range strings.Split(got, "\n") {
			if strings.Contains(l, ch) {
				line = l
				break
			}
		}
		if line == "" {
			t.Errorf("channel %q missing from output", ch)
			continue
		}
		if !strings.Contains(line, wantAttr) {
			t.Errorf("channel %q: expected %q, got line %q", ch, wantAttr, line)
		}
	}
	// Категории на allow-list сохраняют атрибуты (tvg-id, tvg-logo).
	if !strings.Contains(got, `tvg-id="300"`) || !strings.Contains(got, `tvg-logo="http://example.com/logo.png"`) {
		t.Errorf("attributes of allow-listed channel were not preserved:\n%s", got)
	}
	// Заголовок и URL-строки не меняются.
	if !strings.HasPrefix(got, "#EXTM3U\n") {
		t.Errorf("header was modified:\n%s", got)
	}
	if !strings.Contains(got, "http://example.com/d.m3u8") {
		t.Errorf("stream URL line was modified")
	}
}

func TestNormalizeCategoriesLeavesLinesWithoutGroupTitle(t *testing.T) {
	input := `#EXTM3U
#EXTINF:-1 tvg-id="100",Channel A
http://example.com/a.m3u8
#EXTVLCOPT:http-user-agent=foo`
	got := NormalizeCategories(input, map[string]string{}, map[string]bool{"Основные": true}, "Основные")
	if got != input {
		t.Errorf("expected unchanged output, got:\n%s", got)
	}
}

func TestNormalizeCategoriesEmptyAllowList(t *testing.T) {
	input := `#EXTM3U
#EXTINF:-1 group-title="TEST*",Channel A
http://example.com/a.m3u8`
	got := NormalizeCategories(input, nil, map[string]bool{}, "Основные")
	if got != input {
		t.Errorf("expected unchanged output when allow-list is empty, got:\n%s", got)
	}
}

func TestNormalizeCategoriesAliasedCategoryOnAllowListKept(t *testing.T) {
	// *MUZICA → Музыка (в allow-list) — канал сохраняет осмысленную категорию.
	input := `#EXTM3U
#EXTINF:-1 group-title="*MUZICA",Mezzo HD
http://example.com/mezzo.m3u8`
	got := NormalizeCategories(input, map[string]string{"*MUZICA": "Музыка"}, map[string]bool{"Музыка": true}, "Основные")
	if !strings.Contains(got, `group-title="Музыка"`) {
		t.Errorf("expected alias to map to allowed category, got:\n%s", got)
	}
}

func TestAddTvgIDsToPlaylistNormalizedMatch(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 group-title="A",BBC News HD 🔴🐱
http://example.com/bbc.m3u8
#EXTINF:-1 group-title="B" tvg-id="",Eurosport 4K
http://example.com/eu.m3u8
#EXTINF:-1 group-title="C",Channel Unknown
http://example.com/u.m3u8
#EXTINF:-1 group-title="D" tvg-id="keep",Already Has HD
http://example.com/k.m3u8`

	// Нормализованные ключи: "bbc news" и "eurosport" (HD/4K срезаны).
	epgMap := map[string]string{
		"bbc news":  "100",
		"eurosport": "200",
	}

	result := AddTvgIDsToPlaylist(content, epgMap)

	if !strings.Contains(result, `tvg-id="100",BBC News HD 🔴🐱`) {
		t.Errorf("expected normalized match BBC News HD → 100, got:\n%s", result)
	}
	if !strings.Contains(result, `tvg-id="200",Eurosport 4K`) {
		t.Errorf("expected empty tvg-id=\"\" to be filled via normalized match, got:\n%s", result)
	}
	if strings.Contains(result, "Channel Unknown") && strings.Contains(result, `tvg-id="",Channel Unknown`) == false {
		// Channel Unknown без id — строка не должна получить id.
		if strings.Contains(result, `tvg-id="`) && strings.Contains(strings.Split(result, "Channel Unknown")[0], `tvg-id="")`) {
			t.Errorf("unexpected id added to unmatched channel:\n%s", result)
		}
	}
	if !strings.Contains(result, `tvg-id="keep",Already Has HD`) {
		t.Errorf("expected existing non-empty tvg-id to be preserved, got:\n%s", result)
	}
}

func TestInheritTvgIDsFromSiblings(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 group-title="A",Channel SD
http://example.com/sd.m3u8
#EXTINF:-1 group-title="A" tvg-id="100",Channel HD
http://example.com/hd.m3u8
#EXTINF:-1 group-title="B",Single Channel
http://example.com/single.m3u8
#EXTINF:-1 group-title="C" tvg-id="200",Channel FHD
http://example.com/fhd.m3u8
#EXTINF:-1 group-title="C",Channel UHD
http://example.com/uhd.m3u8`

	// Все четыре варианта (SD/HD/FHD/UHD) нормализуются в одну группу «channel»;
	// первый подходящий id в порядке плейлиста — «100» (от HD). UHD тоже получает «100».
	result := InheritTvgIDsFromSiblings(content, nil)
	if !strings.Contains(result, `tvg-id="100",Channel SD`) {
		t.Errorf("expected SD to inherit id from sibling, got:\n%s", result)
	}
	if !strings.Contains(result, `tvg-id="100",Channel UHD`) {
		t.Errorf("expected UHD to inherit group id from sibling, got:\n%s", result)
	}
	if !strings.Contains(result, `tvg-id="100",Channel HD`) {
		t.Errorf("expected existing HD id to be preserved, got:\n%s", result)
	}
	if !strings.Contains(result, `tvg-id="200",Channel FHD`) {
		t.Errorf("expected existing FHD id to be preserved, got:\n%s", result)
	}
	if !strings.Contains(result, `#EXTINF:-1 group-title="B",Single Channel`) {
		t.Errorf("expected single-variant channel untouched, got:\n%s", result)
	}
}

func TestInheritTvgIDsFromSiblingsEPGValidation(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1,Channel X
http://example.com/x.m3u8
#EXTINF:-1 tvg-id="stale",Channel X HD
http://example.com/xhd.m3u8`

	// "stale" нет в EPG — наследование не происходит, строка Channel X без атрибутов.
	result := InheritTvgIDsFromSiblings(content, map[string]bool{"100": true})
	if !strings.Contains(result, "#EXTINF:-1,Channel X\n") {
		t.Errorf("expected Channel X line untouched (stale not inherited), got:\n%s", result)
	}

	// С валидным id в наборе — наследование работает.
	result2 := InheritTvgIDsFromSiblings(content, map[string]bool{"stale": true})
	if !strings.Contains(result2, "#EXTINF:-1 tvg-id=\"stale\",Channel X\n") {
		t.Errorf("expected Channel X to inherit valid sibling id, got:\n%s", result2)
	}
}

// TestDeduplicateByNameMergesNumericSuffixVariants verifies that playlist
// authors' duplicate variants like "360 (2)" / "360 (3)" group together with
// the plain "360" channel and are collapsed to one entry.
func TestDeduplicateByNameMergesNumericSuffixVariants(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 group-title="Новости",360 (2) 🌦️🥔
http://example.com/360-2.m3u8
#EXTINF:-1 group-title="Новости",360 (3) ⬛🧀
http://example.com/360-3.m3u8
#EXTINF:-1 group-title="Новости",360 HD ◻️🌷
http://example.com/360-hd.m3u8
#EXTINF:-1 group-title="Региональные",360 (Армавир) 💟🦒
http://example.com/360-armavir.m3u8`

	result := DeduplicateByName(content, 1, nil, nil)

	// (2), (3) и HD-вариант склеиваются в один канал "360" (остаётся лучший — HD).
	if c := CountChannels(result); c != 2 {
		t.Errorf("expected 2 channels (360 merged + 360 Армавир), got %d:\n%s", c, result)
	}
	for _, gone := range []string{"360 (2)", "360 (3)"} {
		if strings.Contains(result, gone) {
			t.Errorf("expected %q to be merged away:\n%s", gone, result)
		}
	}
	// Победитель — лучший по качеству вариант "360 HD".
	if !strings.Contains(result, "360 HD ◻️🌷") {
		t.Errorf("expected best-quality 360 HD variant to be kept:\n%s", result)
	}
	// Региональный вариант с текстовым суффиксом не трогается.
	if !strings.Contains(result, "360 (Армавир) 💟🦒") {
		t.Errorf("expected regional variant to stay:\n%s", result)
	}
}

// TestCleanChannelName verifies display-name tidying: record badge,
// [Geo-blocked], resolution parens, leading junk, double spaces and the
// "| Region" separator are normalized while HD/country tokens survive.
func TestCleanChannelName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"100% NL TV HD ⏺ʳᵉᶜ", "100% NL TV HD"},
		{"RTR Planeta Europe [Geo-blocked]", "RTR Planeta Europe"},
		{"Belarus-1 (1080p)", "Belarus-1"},
		{"#dabeiTV HD DE", "dabeiTV HD DE"},
		{"&Pictures HD IN", "Pictures HD IN"},
		{".sci-fi HD", "sci-fi HD"},
		{"10 канал  (Саранск) SD", "10 канал (Саранск) SD"},
		{"Liberty Занавес  HD", "Liberty Занавес HD"},
		{"10 канал | Новокузнецк", "10 канал (Новокузнецк)"},
		{"10 канал (2) | Новокузнецк", "10 канал (2) (Новокузнецк)"},
		{"Просто имя HD", "Просто имя HD"},
		{"", ""},
	}
	for _, tc := range tests {
		if got := CleanChannelName(tc.in); got != tc.want {
			t.Errorf("CleanChannelName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestNormalizeChannelNameVariants verifies that duplicate variants from
// different sources collapse to the same grouping key: pipe regions, paren
// numbers anywhere, domain suffixes and latin/ukrainian spellings.
func TestNormalizeChannelNameVariants(t *testing.T) {
	groups := map[string][]string{
		"360":                  {"360 (2) 🌦️🥔", "360 (3) ⬛🧀", "360 HD ◻️🌷", "360"},
		"10 канал новокузнецк": {"10 канал | Новокузнецк", "10 канал (Новокузнецк) HD", "10 канал (2) | Новокузнецк"},
		"1 2 3 tv":             {"1-2-3 TV", "1-2-3.tv HD TV"},
		"1+1 украина":          {"1+1 Украина", "1+1 Україна HD"},
		"россия 1":             {"Россия 1 FHD", "Rossiya 1 HD", "Россия 1ᴴᴰ"},
	}
	for key, names := range groups {
		first := normalizeChannelName(names[0])
		if first != key {
			t.Errorf("normalizeChannelName(%q) = %q, want %q", names[0], first, key)
		}
		for _, n := range names[1:] {
			if got := normalizeChannelName(n); got != first {
				t.Errorf("normalizeChannelName(%q) = %q, want same as %q (%q)", n, got, names[0], first)
			}
		}
	}
}

// TestNormalizeChannelNameKeepsTimeShifts verifies that time-shift markers
// (+N)/(-N) are NOT collapsed: "Россия 1 (+2)" and "Россия 1 (+3)" are
// distinct channels and must stay in separate groups.
func TestNormalizeChannelNameKeepsTimeShifts(t *testing.T) {
	a := normalizeChannelName("Россия 1ᴴᴰ (+2)")
	b := normalizeChannelName("Россия 1 (+3) FD")
	if a == b {
		t.Errorf("expected time-shift variants to differ, both normalized to %q", a)
	}
	// Формат унифицируется: "Россия 1ᴴᴰ (+2)" и "Россия 1 (+2) FD" — один канал.
	c := normalizeChannelName("Россия 1 (+2) FD")
	if a != c {
		t.Errorf("expected same time-shift channel, got %q vs %q", a, c)
	}
}

// TestRegistrableDomain verifies that subdomains of one resource collapse to
// the same registrable domain, and that common second-level domains under
// country-code TLDs keep three labels.
func TestRegistrableDomain(t *testing.T) {
	tests := []struct{ host, want string }{
		{"bsttv.bonus-tv.ru", "bonus-tv.ru"},
		{"cdn-01.bonus-tv.ru", "bonus-tv.ru"},
		{"bonus-tv.ru", "bonus-tv.ru"},
		{"cdn.example.com", "example.com"},
		{"video.rt.com", "rt.com"},
		{"www.bbc.co.uk", "bbc.co.uk"},
		{"mail.google.com.ru", "google.com.ru"},
		{"a.b.c.d.e.org", "e.org"},
		{"192.168.1.1", "192.168.1.1"},
		{"localhost", "localhost"},
	}
	for _, tc := range tests {
		if got := registrableDomain(tc.host); got != tc.want {
			t.Errorf("registrableDomain(%q) = %q, want %q", tc.host, got, tc.want)
		}
	}
}

// TestEmojiFromHostnameUsesRegistrableDomain verifies that different subdomains
// of the same resource produce the same first emoji (bonus-tv.ru case from the
// issue), while different registrable domains produce (with overwhelming
// probability) different emoji.
func TestEmojiFromHostnameUsesRegistrableDomain(t *testing.T) {
	a := emojiFromHostname("https://bsttv.bonus-tv.ru/cdn/kurai/playlist.m3u8")
	b := emojiFromHostname("http://cdn-01.bonus-tv.ru:80/prosveschenie_edge/index.m3u8")
	if a != b {
		t.Errorf("subdomains of bonus-tv.ru should share the first emoji: %q vs %q", a, b)
	}

	c := emojiFromHostname("http://cdn.example.com/live/ch1.m3u8")
	d := emojiFromHostname("http://cdn.example.org/live/ch1.m3u8")
	if c == d {
		t.Logf("note: different registrable domains produced same emoji %q (possible but unlikely)", c)
	}

	// Порт и путь не влияют на первый эмодзи — только регистрируемый домен.
	e := emojiFromHostname("http://bsttv.bonus-tv.ru:8080/cdn/other/playlist.m3u8")
	if a != e {
		t.Errorf("port/path must not affect hostname emoji: %q vs %q", a, e)
	}
}

// TestClassifyFallbackCategories verifies that channels in the fallback
// category are reclassified by unambiguous name keywords, longer keywords win,
// only allow-listed categories are used, and denied categories (e.g. «Кино»)
// are removed entirely.
func TestClassifyFallbackCategories(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 group-title="Основные",Матч! Футбол 1 HD
http://example.com/football.m3u8
#EXTINF:-1 group-title="Основные",Discovery Sport HD
http://example.com/disc-sport.m3u8
#EXTINF:-1 group-title="Основные",Карусель
http://example.com/karusel.m3u8
#EXTINF:-1 group-title="Основные",Непонятный канал
http://example.com/unknown.m3u8
#EXTINF:-1 group-title="Спорт",Уже спорт
http://example.com/already-sport.m3u8
#EXTINF:-1 group-title="Основные",Кинопоказ HD
http://example.com/kino.m3u8
`
	allowed := map[string]bool{"Спорт": true, "Детские": true, "Познавательные": true, "Основные": true}
	remove := config.CategoriesToRemoveByKeywordSet()
	result := ClassifyFallbackCategories(content, config.CategoryKeywords, allowed, "Основные", remove)

	// «Спорт» и «Кино» are in the deny set — matching channels must be REMOVED
	// (entry + URL gone), not reclassified.
	for _, gone := range []string{"Матч! Футбол 1 HD", "football.m3u8", "Discovery Sport HD", "disc-sport.m3u8", "Кинопоказ", "kino.m3u8"} {
		if strings.Contains(result, gone) {
			t.Errorf("expected %q to be removed entirely:\n%s", gone, result)
		}
	}
	if !strings.Contains(result, `group-title="Детские",Карусель`) {
		t.Errorf("expected Карусель → Детские:\n%s", result)
	}
	if !strings.Contains(result, `group-title="Основные",Непонятный канал`) {
		t.Errorf("expected unknown channel to stay in fallback:\n%s", result)
	}
	if !strings.Contains(result, `group-title="Спорт",Уже спорт`) {
		t.Errorf("expected already-categorized channel untouched:\n%s", result)
	}
}

// TestClassifyFallbackCategoriesKeepsDeniedWhenNotConfigured verifies that a
// category not in the remove set is reclassified as usual (backward compat).
func TestClassifyFallbackCategoriesKeepsDeniedWhenNotConfigured(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 group-title="Основные",Кинопоказ HD
http://example.com/kino.m3u8
`
	allowed := map[string]bool{"Кино": true, "Основные": true}
	result := ClassifyFallbackCategories(content, config.CategoryKeywords, allowed, "Основные", nil)
	if !strings.Contains(result, `group-title="Кино",Кинопоказ HD`) {
		t.Errorf("expected Кинопоказ → Кино when not denied:\n%s", result)
	}
}

// TestFuzzyMatchTvgID verifies the edit-distance fallback catches typos but
// does not match unrelated names.
func TestFuzzyMatchTvgID(t *testing.T) {
	byLength := map[int][]fuzzyCandidate{
		7:  {{norm: "discovery", id: "discovery"}},
		10: {{norm: "eurosport 1", id: "eurosport1"}, {norm: "discovery x", id: "discx"}},
	}
	if got := fuzzyMatchTvgID("Discoery Channel", byLength); got != "" {
		t.Errorf("Discoery Channel (len 16) should not match short candidates, got %q", got)
	}
	// Точный/нормализованный матч уже обработан до fuzzy — здесь просто проверяем
	// что кандидат на расстоянии 1 находится.
	byLen2 := map[int][]fuzzyCandidate{
		9: {{norm: "eurosport", id: "eurosport"}},
	}
	if got := fuzzyMatchTvgID("Eurosport", byLen2); got != "eurosport" {
		t.Errorf("exact fuzzy candidate should match, got %q", got)
	}
	if got := fuzzyMatchTvgID("Eurospor", byLen2); got != "eurosport" {
		t.Errorf("edit-distance 1 typo should match, got %q", got)
	}
	if got := fuzzyMatchTvgID("Football", byLen2); got != "" {
		t.Errorf("unrelated name should not match, got %q", got)
	}
}

// TestMatchCategoryKeywordWordBoundary verifies that short keywords (<= 3
// runes) match on word boundaries only, so "рок" does not fire inside
// "Прокопьевск" and "ю" inside "Южно-Сахалинск", while longer morphological
// stems keep substring matching.
func TestMatchCategoryKeywordWordBoundary(t *testing.T) {
	tests := []struct {
		name string
		kw   []string
		want string
	}{
		// «рок» (3 runes) — граница слова.
		{"рок тв", []string{"рок"}, "рок"},
		{"27 канал Прокопьевск (Кемеровская обл.)", []string{"рок"}, ""},
		{"Про жизнь (Прокопьевск) HD", []string{"рок"}, ""},
		// «ю» (1 rune) — граница слова.
		{"Ю", []string{"ю"}, "ю"},
		{"Ю HD", []string{"ю"}, "ю"},
		{"Россия 1 (Южно-Сахалинск)", []string{"ю"}, ""},
		{"АСТВ (Южно-Сахалинск) HD", []string{"ю"}, ""},
		// «че» (2 runes) — граница слова («ЧЕ!» ок, «Чебоксары» нет).
		{"ЧЕ!", []string{"че"}, "че"},
		{"Россия 1 (Чебоксары)", []string{"че"}, ""},
		{"Архыз 24 (Черкеск) HD", []string{"че"}, ""},
		{"12 канал Череповец", []string{"че"}, ""},
		// Длинные морфологические корни — подстрока как раньше.
		{"музыкальный канал", []string{"музык"}, "музык"},
		{"Детский мир", []string{"детск"}, "детск"},
		{"Канал Новости 24", []string{"новост"}, "новост"},
		// «mtv» (3 runes) — граница.
		{"MTV Hits", []string{"mtv"}, "mtv"},
		{"Karaoke World", []string{"mtv"}, ""},
	}
	for _, tt := range tests {
		got := matchCategoryKeyword(strings.ToLower(tt.name), tt.kw)
		if got != tt.want {
			t.Errorf("matchCategoryKeyword(%q, %v) = %q, want %q", tt.name, tt.kw, got, tt.want)
		}
	}
}

// TestMatchWordBoundary covers runewise boundary detection incl. Cyrillic.
func TestMatchWordBoundary(t *testing.T) {
	tests := []struct {
		name, kw string
		want     bool
	}{
		{"рок тв", "рок", true},
		{"роктв", "рок", false},
		{"прокопьевск", "рок", false},
		{"ю", "ю", true},
		{"южно", "ю", false},
		{"че!", "че", true},
		{"чебоксары", "че", false},
		{"mtv hits", "mtv", true},
		{"xxmtv", "mtv", false},
		{"abc рок def", "рок", true},
	}
	for _, tt := range tests {
		if got := matchWordBoundary(tt.name, tt.kw); got != tt.want {
			t.Errorf("matchWordBoundary(%q, %q) = %v, want %v", tt.name, tt.kw, got, tt.want)
		}
	}
}

// TestShouldFilterByNameDateSuffixes verifies that a date-bearing name like
// "ОБНОВЛЕНО 20.08" is matched by the "обновлено" substring regardless of the
// date value — future runs with "ОБНОВЛЕНО 21.08" or any other date are
// covered without config changes.
func TestShouldFilterByNameDateSuffixes(t *testing.T) {
	excludeLower := utils.ToLowerSlice(config.ChannelNamesToExclude)
	for _, name := range []string{
		"ОБНОВЛЕНО 20.08",
		"ОБНОВЛЕНО 21.08",
		"обновлено 15.09.2024",
		"Channel ОБНОВЛЕНО 01.01",
	} {
		if !shouldFilterByName(name, excludeLower) {
			t.Errorf("expected %q to be filtered by 'обновлено' pattern", name)
		}
	}
	if shouldFilterByName("Обычный канал", excludeLower) {
		t.Error("ordinary channel must not be filtered")
	}
}
