package epg

import (
	"encoding/xml"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/ozyab/iptv/internal/config"
)

func getRelativeTimeStr(hoursFromNow int) string {
	targetTime := time.Now().Add(time.Duration(hoursFromNow) * time.Hour)
	return targetTime.Format("20060102150405") + " +0000"
}

func TestExtractChannelInfoFromPlaylist(t *testing.T) {
	playlistContent := `#EXTM3U
#EXTINF:-1 tvg-id="channel1" group-title="Россия | Russia",Channel 1
http://example.com/1
#EXTINF:-1 tvg-id="channel2" group-title="News",Channel 2
http://example.com/2
#EXTINF:-1 tvg-id="channel3" group-title="Развлекательные",Channel 3
http://example.com/3
#EXTINF:-1 tvg-id="" group-title="Empty",Channel 4
http://example.com/4
#EXTINF:-1 group-title="No ID",Channel 5
http://example.com/5`

	channelIDs, channelNames := ExtractChannelInfoFromPlaylist(playlistContent)

	if _, ok := channelIDs["channel1"]; !ok {
		t.Error("expected channel1 in channelIDs")
	}
	if _, ok := channelIDs["channel2"]; !ok {
		t.Error("expected channel2 in channelIDs")
	}
	if _, ok := channelIDs["channel3"]; !ok {
		t.Error("expected channel3 in channelIDs")
	}
	if _, ok := channelIDs[""]; ok {
		t.Error("expected empty ID to not be included")
	}
	if len(channelIDs) != 3 {
		t.Errorf("expected 3 channel IDs, got %d", len(channelIDs))
	}

	if channelIDs["channel1"] != "Россия | Russia" {
		t.Errorf("expected channel1 category 'Россия | Russia', got '%s'", channelIDs["channel1"])
	}
	if channelIDs["channel2"] != "News" {
		t.Errorf("expected channel2 category 'News', got '%s'", channelIDs["channel2"])
	}
	if channelIDs["channel3"] != "Развлекательные" {
		t.Errorf("expected channel3 category 'Развлекательные', got '%s'", channelIDs["channel3"])
	}

	if _, ok := channelNames["Channel 1"]; !ok {
		t.Error("expected 'Channel 1' in channelNames")
	}
	if _, ok := channelNames["Channel 5"]; !ok {
		t.Error("expected 'Channel 5' in channelNames")
	}
	if len(channelNames) != 5 {
		t.Errorf("expected 5 channel names, got %d", len(channelNames))
	}

	if channelNames["Channel 1"] != "Россия | Russia" {
		t.Errorf("expected Channel 1 category 'Россия | Russia', got '%s'", channelNames["Channel 1"])
	}
	if channelNames["Channel 5"] != "No ID" {
		t.Errorf("expected Channel 5 category 'No ID', got '%s'", channelNames["Channel 5"])
	}
}

func TestFilterEPGContentBasic(t *testing.T) {
	tStart := getRelativeTimeStr(1)
	tStop := getRelativeTimeStr(2)

	epgContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="channel1">
    <display-name lang="en">Channel 1</display-name>
  </channel>
  <channel id="channel2">
    <display-name lang="en">Channel 2</display-name>
  </channel>
  <channel id="channel3">
    <display-name lang="en">Channel 3</display-name>
  </channel>
  <programme start="%s" stop="%s" channel="channel1">
    <title lang="en">Show 1</title>
  </programme>
  <programme start="%s" stop="%s" channel="channel2">
    <title lang="en">Show 2</title>
  </programme>
  <programme start="%s" stop="%s" channel="channel3">
    <title lang="en">Show 3</title>
  </programme>
</tv>`, tStart, tStop, tStart, tStop, tStart, tStop)

	channelIDs := map[string]string{"channel1": "", "channel3": ""}
	channelNames := map[string]string{}

	filtered, err := FilterEPGContent(epgContent, channelIDs, nil, nil, channelNames, 10)
	if err != nil {
		t.Fatalf("FilterEPGContent failed: %v", err)
	}

	var tv TV
	if err := xml.Unmarshal([]byte(filtered), &tv); err != nil {
		t.Fatalf("failed to parse filtered EPG: %v", err)
	}

	if len(tv.Channels) != 2 {
		t.Errorf("expected 2 channels, got %d", len(tv.Channels))
	}
	chSet := make(map[string]bool)
	for _, ch := range tv.Channels {
		chSet[ch.ID] = true
	}
	if !chSet["channel1"] || !chSet["channel3"] {
		t.Error("expected channel1 and channel3 in result")
	}

	if len(tv.Programmes) != 2 {
		t.Errorf("expected 2 programmes, got %d", len(tv.Programmes))
	}
	progSet := make(map[string]bool)
	for _, p := range tv.Programmes {
		progSet[p.Channel] = true
	}
	if !progSet["channel1"] || !progSet["channel3"] {
		t.Error("expected programmes for channel1 and channel3")
	}
}

func TestFilterEPGContentEmptyChannelIDs(t *testing.T) {
	channelIDs := map[string]string{}
	channelNames := map[string]string{}
	filtered, err := FilterEPGContent("<tv></tv>", channelIDs, nil, nil, channelNames, 10)
	if err != nil {
		t.Fatalf("FilterEPGContent failed: %v", err)
	}

	var tv TV
	if err := xml.Unmarshal([]byte(filtered), &tv); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if len(tv.Channels) != 0 {
		t.Errorf("expected 0 channels, got %d", len(tv.Channels))
	}
	if len(tv.Programmes) != 0 {
		t.Errorf("expected 0 programmes, got %d", len(tv.Programmes))
	}
}

func TestIsGzipped(t *testing.T) {
	if !isGzipped([]byte{0x1f, 0x8b, 0x08, 0x00}) {
		t.Error("expected gzip detection to be true")
	}
	if isGzipped([]byte("hello world")) {
		t.Error("expected gzip detection to be false")
	}
	if isGzipped([]byte{0x1f}) {
		t.Error("expected short data to not be gzipped")
	}
}

func TestFilterEPGContentExcludesSpecificChannelIDs(t *testing.T) {
	tStart := getRelativeTimeStr(1)
	tStop := getRelativeTimeStr(2)

	epgContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="channel1">
    <display-name lang="en">Channel 1</display-name>
  </channel>
  <channel id="channel2">
    <display-name lang="en">Channel 2</display-name>
  </channel>
  <channel id="channel3">
    <display-name lang="en">Channel 3</display-name>
  </channel>
  <programme start="%s" stop="%s" channel="channel1">
    <title lang="en">Show 1</title>
  </programme>
  <programme start="%s" stop="%s" channel="channel2">
    <title lang="en">Show 2</title>
  </programme>
  <programme start="%s" stop="%s" channel="channel3">
    <title lang="en">Show 3</title>
  </programme>
</tv>`, tStart, tStop, tStart, tStop, tStart, tStop)

	channelIDs := map[string]string{"channel1": "", "channel2": "", "channel3": ""}
	excludedIDs := []string{"channel2"}

	filtered, err := FilterEPGContent(epgContent, channelIDs, nil, excludedIDs, nil, 10)
	if err != nil {
		t.Fatalf("FilterEPGContent failed: %v", err)
	}

	var tv TV
	if err := xml.Unmarshal([]byte(filtered), &tv); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if len(tv.Channels) != 2 {
		t.Errorf("expected 2 channels, got %d", len(tv.Channels))
	}
	chSet := make(map[string]bool)
	for _, ch := range tv.Channels {
		chSet[ch.ID] = true
	}
	if !chSet["channel1"] || !chSet["channel3"] {
		t.Error("expected channel1 and channel3 in result")
	}
}

func TestFilterEPGContentMatchesByChannelName(t *testing.T) {
	tStart := getRelativeTimeStr(1)
	tStop := getRelativeTimeStr(2)

	epgContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="epg100">
    <display-name lang="ru">Первый канал</display-name>
  </channel>
  <channel id="epg200">
    <display-name lang="ru">Россия 1</display-name>
  </channel>
  <channel id="epg300">
    <display-name lang="ru">НТВ</display-name>
  </channel>
  <programme start="%s" stop="%s" channel="epg100">
    <title lang="ru">Show 1</title>
  </programme>
  <programme start="%s" stop="%s" channel="epg200">
    <title lang="ru">Show 2</title>
  </programme>
  <programme start="%s" stop="%s" channel="epg300">
    <title lang="ru">Show 3</title>
  </programme>
</tv>`, tStart, tStop, tStart, tStop, tStart, tStop)

	channelIDs := map[string]string{}
	channelNames := map[string]string{
		"Первый канал": "Общие",
		"НТВ":          "Общие",
	}

	filtered, err := FilterEPGContent(epgContent, channelIDs, nil, nil, channelNames, 10)
	if err != nil {
		t.Fatalf("FilterEPGContent failed: %v", err)
	}

	var tv TV
	if err := xml.Unmarshal([]byte(filtered), &tv); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if len(tv.Channels) != 2 {
		t.Errorf("expected 2 channels, got %d", len(tv.Channels))
	}
	chSet := make(map[string]bool)
	for _, ch := range tv.Channels {
		chSet[ch.ID] = true
	}
	if !chSet["epg100"] || !chSet["epg300"] {
		t.Error("expected epg100 and epg300 in result")
	}
}

func TestFilterEPGContentMatchesByNameAndIDCombined(t *testing.T) {
	tStart := getRelativeTimeStr(1)
	tStop := getRelativeTimeStr(2)

	epgContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="tvg-id-1">
    <display-name lang="ru">Channel By ID</display-name>
  </channel>
  <channel id="epg-no-tvg">
    <display-name lang="ru">Channel By Name</display-name>
  </channel>
  <channel id="epg-excluded">
    <display-name lang="ru">Excluded Channel</display-name>
  </channel>
  <programme start="%s" stop="%s" channel="tvg-id-1">
    <title lang="ru">Show 1</title>
  </programme>
  <programme start="%s" stop="%s" channel="epg-no-tvg">
    <title lang="ru">Show 2</title>
  </programme>
  <programme start="%s" stop="%s" channel="epg-excluded">
    <title lang="ru">Show 3</title>
  </programme>
</tv>`, tStart, tStop, tStart, tStop, tStart, tStop)

	channelIDs := map[string]string{"tvg-id-1": ""}
	channelNames := map[string]string{"Channel By Name": "Общие"}

	filtered, err := FilterEPGContent(epgContent, channelIDs, nil, nil, channelNames, 10)
	if err != nil {
		t.Fatalf("FilterEPGContent failed: %v", err)
	}

	var tv TV
	if err := xml.Unmarshal([]byte(filtered), &tv); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if len(tv.Channels) != 2 {
		t.Errorf("expected 2 channels, got %d", len(tv.Channels))
	}
	chSet := make(map[string]bool)
	for _, ch := range tv.Channels {
		chSet[ch.ID] = true
	}
	if !chSet["tvg-id-1"] || !chSet["epg-no-tvg"] {
		t.Error("expected tvg-id-1 and epg-no-tvg in result")
	}
}

func TestBuildEPGNameToIDMap(t *testing.T) {
	epgContent := `<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="ch1">
    <display-name lang="ru">Первый канал</display-name>
  </channel>
  <channel id="ch2">
    <display-name lang="en">Russia 1</display-name>
  </channel>
</tv>`

	nameMap := BuildEPGNameToIDMap(epgContent)
	if nameMap["первый канал"] != "ch1" {
		t.Errorf("expected 'первый канал' -> 'ch1', got '%s'", nameMap["первый канал"])
	}
	if nameMap["russia 1"] != "ch2" {
		t.Errorf("expected 'russia 1' -> 'ch2', got '%s'", nameMap["russia 1"])
	}
}

func TestSaveFilteredEPGLocally(t *testing.T) {
	cfg := config.New()
	tmpDir := t.TempDir()
	os.Setenv("OUTPUT_DIR", tmpDir)
	defer os.Unsetenv("OUTPUT_DIR")

	content := `<?xml version="1.0" encoding="UTF-8"?><tv></tv>`
	err := SaveFilteredEPGLocally(content, "test-epg.xml", cfg)
	if err != nil {
		t.Fatalf("SaveFilteredEPGLocally failed: %v", err)
	}

	os.Setenv("OUTPUT_DIR", tmpDir)
	err = SaveFilteredEPGLocally(content, "test-epg.xml.gz", cfg)
	if err != nil {
		t.Fatalf("SaveFilteredEPGLocally gz failed: %v", err)
	}
}
