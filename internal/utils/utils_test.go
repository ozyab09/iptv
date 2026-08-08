package utils

import (
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
