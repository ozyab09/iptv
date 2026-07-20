package m3u

import (
	"strings"
	"testing"
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
		name     string
		input    string
		order    []string // expected channel name order
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
			name: "sorts correctly with superscript suffixes",
			input: `#EXTM3U
#EXTINF:-1,Channel B ²
http://example.com/b2.m3u8
#EXTINF:-1,Channel A
http://example.com/a.m3u8
#EXTINF:-1,Channel B ¹
http://example.com/b1.m3u8`,
			order: []string{"Channel A", "Channel B ²", "Channel B ¹"},
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
		name     string
		input    string
		wantURLs int // expected number of unique URLs
		checks   []string // substrings that must be present
		notChecks []string // substrings that must NOT be present
	}{
		{
			name: "deduplicate by URL, merge attributes, keep longest name",
			input: `#EXTM3U
#EXTINF:-1 group-title="Общие" tvg-id="1000" tvg-logo="http://epg.one/img/1000.png" tvg-rec="0",ОТР HD
http://example.com/otr.m3u8
#EXTINF:-1 tvg-rec="7",ОТР
http://example.com/otr.m3u8`,
			wantURLs: 1,
			checks:   []string{`group-title="Общие"`, `tvg-id="1000"`, `tvg-logo="http://epg.one/img/1000.png"`, `tvg-rec="0"`, `ОТР HD`},
			notChecks: nil,
		},
		{
			name: "different URLs kept separate",
			input: `#EXTM3U
#EXTINF:-1 group-title="Общие" tvg-id="1000",Channel A
http://example.com/a.m3u8
#EXTINF:-1 group-title="Новости" tvg-id="2000",Channel B
http://example.com/b.m3u8`,
			wantURLs: 2,
			checks:   []string{`Channel A`, `Channel B`, `http://example.com/a.m3u8`, `http://example.com/b.m3u8`},
			notChecks: nil,
		},
		{
			name: "merge fills missing tvg-logo from other entry",
			input: `#EXTM3U
#EXTINF:-1 tvg-id="500" tvg-logo="http://epg.one/img/500.png",Channel X
http://example.com/x.m3u8
#EXTINF:-1 tvg-id="500",Channel X Long Name Version
http://example.com/x.m3u8`,
			wantURLs: 1,
			checks:   []string{`tvg-logo="http://epg.one/img/500.png"`, `Channel X Long Name Version`},
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
			wantURLs: 3,
			checks:   []string{`Channel 1`, `Channel 2`, `Channel 3`},
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
			wantURLs: 1,
			checks:   []string{`The Longest Channel Name Here`, `tvg-rec="0"`, `group-title="A"`, `tvg-logo="http://logo.png"`},
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

func TestAddSequentialNumbers(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 tvg-id="711",Channel A
http://example.com/a
#EXTINF:-1 tvg-id="162",Channel B
http://example.com/b
#EXTINF:-1 tvg-id="711",Channel A
http://example.com/a2
#EXTINF:-1 tvg-id="999",Channel C
http://example.com/c`

	result := AddSequentialNumbers(content)

	tests := []struct {
		name     string
		original string
		renamed  string
	}{
		{"channel 1", "Channel A", "Channel A ¹"},
		{"channel 2", "Channel B", "Channel B ²"},
		{"channel 3", "Channel A", "Channel A ³"},
		{"channel 4", "Channel C", "Channel C ⁴"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if strings.Contains(result, tc.original+",") {
				t.Errorf("expected original name %q to be renamed to %q", tc.original, tc.renamed)
			}
			if !strings.Contains(result, tc.renamed) {
				t.Errorf("expected renamed channel %q in result", tc.renamed)
			}
		})
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
