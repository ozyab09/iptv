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

func TestGetBaseChannelName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Channel Name orig", "Channel Name"},
		{"Channel Name hd", "Channel Name"},
		{"Channel Name orig hd", "Channel Name"},
		{"Channel Name HD", "Channel Name"},
		{"Channel Name", "Channel Name"},
	}
	for _, tc := range tests {
		result := GetBaseChannelName(tc.input)
		if result != tc.expected {
			t.Errorf("GetBaseChannelName(%q) = %q, want %q", tc.input, result, tc.expected)
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

func TestRemoveDuplicatesAndApplyHDPref(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 tvg-id="711",Channel 1
http://example.com/1
#EXTINF:-1 tvg-id="711",Channel 1 HD
http://example.com/1hd
#EXTINF:-1 tvg-id="162",Channel 2
http://example.com/2
#EXTINF:-1 tvg-id="162",Channel 2
http://example.com/2duplicate`

	result := RemoveDuplicatesAndApplyHDPref(content)
	if !strings.Contains(result, "Channel 1") {
		t.Error("expected 'Channel 1' in result")
	}
	if !strings.Contains(result, "Channel 1 HD") {
		t.Error("expected 'Channel 1 HD' in result")
	}
	if !strings.Contains(result, "Channel 2 #1") {
		t.Error("expected 'Channel 2 #1' in result")
	}
	if !strings.Contains(result, "Channel 2 #2") {
		t.Error("expected 'Channel 2 #2' in result")
	}
}

func TestRemoveDuplicatesWithTVGRec(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 tvg-id="711" tvg-rec="3",Channel 1
http://example.com/1low
#EXTINF:-1 tvg-id="711" tvg-rec="7",Channel 1
http://example.com/1high
#EXTINF:-1 tvg-id="162" tvg-rec="0",Channel 2
http://example.com/2low
#EXTINF:-1 tvg-id="162" tvg-rec="5",Channel 2
http://example.com/2high`

	result := RemoveDuplicatesAndApplyHDPref(content)
	if !strings.Contains(result, "Channel 1 #1") {
		t.Error("expected 'Channel 1 #1' in result")
	}
	if !strings.Contains(result, "Channel 1 #2") {
		t.Error("expected 'Channel 1 #2' in result")
	}
	if !strings.Contains(result, `tvg-rec="3"`) {
		t.Error("expected tvg-rec=3 in result")
	}
	if !strings.Contains(result, `tvg-rec="7"`) {
		t.Error("expected tvg-rec=7 in result")
	}
	if !strings.Contains(result, "Channel 2 #1") {
		t.Error("expected 'Channel 2 #1' in result")
	}
	if !strings.Contains(result, "Channel 2 #2") {
		t.Error("expected 'Channel 2 #2' in result")
	}
	if !strings.Contains(result, `tvg-rec="0"`) {
		t.Error("expected tvg-rec=0 in result")
	}
	if !strings.Contains(result, `tvg-rec="5"`) {
		t.Error("expected tvg-rec=5 in result")
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

	result := FilterContent(content, []string{"Взрослые"}, nil, "")

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

	result := FilterContent(content, nil, nil, "")

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

	result := FilterContent(content, nil, nil, "")

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

	result := FilterContent(content, nil, []string{"Fashion"}, "")

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

	result := FilterContent(content, nil, []string{"Fashion"}, "")

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

	result := FilterContent(content, nil, []string{"Fashion", "Adult", "Gambling"}, "")

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
