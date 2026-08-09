package utils

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
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
