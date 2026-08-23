package utils

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"log"
	"strings"
	"testing"
	"time"
)

func TestRetrySuccess(t *testing.T) {
	attempts := 0
	err := Retry(3, 10*time.Millisecond, 1.0, func() error {
		attempts++
		return nil
	})
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", attempts)
	}
}

func TestRetryFailure(t *testing.T) {
	attempts := 0
	err := Retry(3, 10*time.Millisecond, 1.0, func() error {
		attempts++
		return errors.New("test error")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetryRecovery(t *testing.T) {
	attempts := 0
	err := Retry(3, 10*time.Millisecond, 1.0, func() error {
		attempts++
		if attempts < 3 {
			return errors.New("transient error")
		}
		return nil
	})
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetryWithContextStopsDuringBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	err := RetryWithContext(ctx, 3, time.Second, 1.0, func() error {
		return errors.New("temporary failure")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("retry ignored cancellation for %s", elapsed)
	}
}

func TestRetryDelayWithJitterBounds(t *testing.T) {
	base := 100 * time.Millisecond
	for i := 0; i < 200; i++ {
		d := retryDelayWithJitter(base)
		lo := base - base/10
		hi := base + base/10
		if d < lo || d > hi {
			t.Fatalf("jitter out of ±10%% bounds: got %v, want within [%v, %v]", d, lo, hi)
		}
	}
	if d := retryDelayWithJitter(0); d != 0 {
		t.Fatalf("expected zero delay for zero input, got %v", d)
	}
}

func TestRetryWithContextReturnsEarlyOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled before the first attempt

	err := RetryWithContext(ctx, 3, time.Second, 1.0, func() error {
		t.Error("function should not be called when context is already cancelled")
		return errors.New("unreachable")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestDecompressGZipLimited(t *testing.T) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write([]byte("more than ten bytes")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := DecompressGZipLimited(compressed.Bytes(), 10); err == nil {
		t.Fatal("expected decompressed size limit error")
	}
}

func TestSanitizeLogMessageURLs(t *testing.T) {
	msg := sanitizeLogMessage("Downloading from https://raw.githubusercontent.com/foo/bar")
	if strings.Contains(msg, "raw.githubusercontent.com") {
		t.Error("expected URL to be masked")
	}
	if !strings.Contains(msg, "https://****/****") {
		t.Error("expected masked URL pattern")
	}
}

func TestSanitizeLogMessageAWSCreds(t *testing.T) {
	msg := sanitizeLogMessage("Key: YCAJEu1234567890abcdef")
	if strings.Contains(msg, "YCAJEu1234567890abcdef") {
		t.Error("expected AWS key to be masked")
	}
}

func TestMaskURL(t *testing.T) {
	masked := maskURL("https://storage.yandexcloud.net/bucket/key")
	if masked != "https://****/****" {
		t.Errorf("expected 'https://****/****', got '%s'", masked)
	}
}

// TestSanitizingWriterMasksURLsAndCreds verifies the stdlib-logger sanitizer
// (used for http.Transport.ErrorLog) masks URLs and credentials, e.g. the
// "Unsolicited response received on idle HTTP channel" transport message that
// carries full HLS probe URLs.
func TestSanitizingWriterMasksURLsAndCreds(t *testing.T) {
	var buf bytes.Buffer
	w := sanitizingWriter{w: &buf}
	msg := `Unsolicited response received on idle HTTP channel starting with "#EXTM3U" ` +
		`http://a3569457538-zabava-htlive.cdn.ngenix.net/hls/CH_MUZTV/seg1?useseq=t ` +
		`key=YCAJEu1234567890abcdef`
	if _, err := w.Write([]byte(msg)); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "ngenix.net") || strings.Contains(out, "YCAJEu1234567890abcdef") {
		t.Errorf("expected URL and credentials to be masked, got: %s", out)
	}
	if !strings.Contains(out, "://****/****") {
		t.Errorf("expected masked URL pattern, got: %s", out)
	}
}

// TestLogLoggerWithSanitizingWriterMasks verifies the full *log.Logger path:
// Printf output written through sanitizingWriter is sanitized.
func TestLogLoggerWithSanitizingWriterMasks(t *testing.T) {
	var buf bytes.Buffer
	l := log.New(sanitizingWriter{w: &buf}, "", 0)
	l.Printf("probe failed for https://cdn.example.com/playlist.m3u8?token=secret123")
	out := buf.String()
	if strings.Contains(out, "cdn.example.com") || strings.Contains(out, "secret123") {
		t.Errorf("expected URL to be masked, got: %s", out)
	}
	if !strings.Contains(out, "https://****/****") {
		t.Errorf("expected masked URL pattern, got: %s", out)
	}
}

// TestGlobalStdLogSanitizerInstalled verifies init() redirected the global
// stdlib logger (the path net/http uses for transport messages) through the
// sanitizer.
func TestGlobalStdLogSanitizerInstalled(t *testing.T) {
	if _, ok := log.Default().Writer().(sanitizingWriter); !ok {
		t.Errorf("expected global stdlib logger to write through sanitizingWriter, got %T", log.Default().Writer())
	}
}

func TestStripTrailingEmoji(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Первый канал 🔴🐱", "Первый канал"},
		{"Канал ❄️🍁 ", "Канал"},
		{"Обычный канал", "Обычный канал"},
		{"", ""},
		{"Канал 🎯 HD", "Канал 🎯 HD"}, // эмодзи не в конце — не трогаем
	}
	for _, tc := range tests {
		if got := StripTrailingEmoji(tc.input); got != tc.expected {
			t.Errorf("StripTrailingEmoji(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestStripTrailingEmojiExtendedBlocks(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Cartoon Network 💢🪗", "Cartoon Network"},
		{"Карусель ⚪🪕", "Карусель"},
		{"Канал 🌟🐱", "Канал"},
		{"Канал ◻️⬛", "Канал"},
		{"Канал ◼️⬜", "Канал"},
		{"Просто имя", "Просто имя"},
		{"", ""},
	}
	for _, tc := range tests {
		if got := StripTrailingEmoji(tc.in); got != tc.want {
			t.Errorf("StripTrailingEmoji(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"abc", "abc", 0},
		{"kitten", "sitting", 3},
		{"flaw", "lawn", 2},
		{"eurospor", "eurosport", 1},
		{"discoery", "discovery", 1},
		{"футбол", "футболл", 1},
	}
	for _, tc := range tests {
		if got := LevenshteinDistance(tc.a, tc.b); got != tc.want {
			t.Errorf("LevenshteinDistance(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
