package telegram

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestFormatReport(t *testing.T) {
	r := Report{
		PlaylistsDownloadedBytes: 12_900_000,
		PlaylistsFilteredBytes:   4_700_000,
		EPGDownloadedBytes:       188_000_000,
		EPGFilteredBytes:         41_000_000,
		FailedURLs:               []string{"https://example.com/epg-2.xml.gz"},
	}
	got := FormatReport(r)

	for _, want := range []string{
		"📊 iptv-filter: отчёт",
		"📥 Плейлисты:",
		"скачано: 12.3 МБ",
		"после фильтрации: 4.5 МБ (-7.8 МБ, -64%)",
		"📥 EPG:",
		"скачано: 179.3 МБ",
		"после фильтрации: 39.1 МБ (-140.2 МБ, -78%)",
		"⚠️ Не удалось загрузить:",
		"• https://example.com/epg-2.xml.gz",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("FormatReport missing %q in:\n%s", want, got)
		}
	}
}

func TestFormatReportWithoutEPG(t *testing.T) {
	r := Report{
		PlaylistsDownloadedBytes: 1000,
		PlaylistsFilteredBytes:   400,
	}
	got := FormatReport(r)
	if strings.Contains(got, "EPG") {
		t.Errorf("FormatReport should omit the EPG section when EPG is unused:\n%s", got)
	}
	if strings.Contains(got, "Не удалось") {
		t.Errorf("FormatReport should omit failed-URLs section when there are none:\n%s", got)
	}
}

func TestFormatReportZeroBytes(t *testing.T) {
	got := FormatReport(Report{})
	if !strings.Contains(got, "скачано: 0 Б") {
		t.Errorf("expected zero-byte playlist section, got:\n%s", got)
	}
}

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{0, "0 Б"},
		{512, "512 Б"},
		{1024, "1.0 КБ"},
		{12_900_000, "12.3 МБ"},
		{188_000_000, "179.3 МБ"},
		{2 * 1024 * 1024 * 1024, "2.0 ГБ"},
	}
	for _, c := range cases {
		if got := formatBytes(c.n); got != c.want {
			t.Errorf("formatBytes(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

// TestSendReport verifies the request shape against the Bot API: POST to
// /bot<token>/sendMessage with form-encoded chat_id + text.
func TestSendReport(t *testing.T) {
	var gotForm url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/bot123456:test-token/sendMessage" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/x-www-form-urlencoded") {
			t.Errorf("unexpected Content-Type: %s", ct)
		}
		body, _ := io.ReadAll(r.Body)
		gotForm, _ = url.ParseQuery(string(body))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	oldBase := apiBase
	apiBase = server.URL
	defer func() { apiBase = oldBase }()

	report := Report{PlaylistsDownloadedBytes: 2048, PlaylistsFilteredBytes: 1024}
	if err := SendReport(context.Background(), "123456:test-token", "987654321", report, false); err != nil {
		t.Fatalf("SendReport returned error: %v", err)
	}
	if got := gotForm.Get("chat_id"); got != "987654321" {
		t.Errorf("expected chat_id=987654321 in form, got %q", got)
	}
	if text := gotForm.Get("text"); !strings.Contains(text, "iptv-filter") {
		t.Errorf("expected message text in form, got %q", text)
	}
}

// TestSendReportError verifies non-200 responses surface as errors.
func TestSendReportError(t *testing.T) {
	// Single attempt to keep the test fast (no retry backoff).
	oldAttempts, oldDelay := retryAttempts, retryDelay
	retryAttempts, retryDelay = 1, 0
	defer func() { retryAttempts, retryDelay = oldAttempts, oldDelay }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"ok":false,"description":"chat not found"}`, http.StatusBadRequest)
	}))
	defer server.Close()

	oldBase := apiBase
	apiBase = server.URL
	defer func() { apiBase = oldBase }()

	err := SendReport(context.Background(), "123456:test-token", "987654321", Report{}, false)
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("expected status code in error, got: %v", err)
	}
}

func TestSendReportRequiresCredentials(t *testing.T) {
	if err := SendReport(context.Background(), "", "987654321", Report{}, false); err == nil {
		t.Error("expected error when bot token is empty")
	}
	if err := SendReport(context.Background(), "123456:test-token", "", Report{}, false); err == nil {
		t.Error("expected error when chat id is empty")
	}
}
