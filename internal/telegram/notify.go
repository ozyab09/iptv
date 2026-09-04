// Package telegram sends run statistics to a Telegram chat via the Bot API.
// It uses only the standard library net/http — no third-party dependencies.
package telegram

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ozyab/iptv/internal/utils"
)

// maskURL extracts the host (domain) from a URL. If parsing fails, the
// original string is returned unchanged.
func maskURL(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return u.Host
	}
	return raw
}

// apiBase is the Telegram Bot API base URL. Overridden in tests to point at an
// httptest server.
var apiBase = "https://api.telegram.org"

// Retry parameters, overridable in tests to avoid backoff delays.
var (
	retryAttempts = 3
	retryDelay    = 2 * time.Second
)

// Report is the run statistics payload sent to Telegram after a successful
// (non-dry-run) pipeline run.
type Report struct {
	// PlaylistsDownloadedBytes is the total raw bytes downloaded from all M3U
	// sources (before filtering).
	PlaylistsDownloadedBytes int64
	// PlaylistsFilteredBytes is the final filtered playlist size in bytes.
	PlaylistsFilteredBytes int64
	// TotalChannelsBefore is the total number of channels across all M3U
	// sources before any filtering.
	TotalChannelsBefore int
	// RemainingChannels is the number of channels after filtering.
	RemainingChannels int
	// EPGDownloadedBytes is the total raw bytes downloaded from all EPG
	// sources (before decompression/filtering). Zero when EPG is not configured.
	EPGDownloadedBytes int64
	// EPGFilteredBytes is the filtered EPG size in bytes (gzip-compressed).
	// Zero when EPG is not configured.
	EPGFilteredBytes int64
	// FailedURLs lists source URLs (playlist and/or EPG sources) that failed to
	// download but were skipped — the run continued with the remaining sources
	// instead of aborting.
	FailedURLs []string
	// UnavailableBySource maps M3U source index → number of unavailable channels
	// discovered during availability probing. Empty when probing is disabled.
	UnavailableBySource map[int]int
	// M3USourceURLs is the list of M3U source URLs, indexed to match
	// UnavailableBySource keys.
	M3USourceURLs []string
}

// FormatReport renders the report as the Telegram message text.
func FormatReport(r Report) string {
	var b strings.Builder
	b.WriteString("📊 iptv-filter: отчёт\n\n")

	b.WriteString("📥 Плейлисты:\n")
	b.WriteString(fmt.Sprintf("   скачано: %s\n", formatBytes(r.PlaylistsDownloadedBytes)))
	b.WriteString(fmt.Sprintf("   после фильтрации: %s (-%s, -%.0f%%)\n",
		formatBytes(r.PlaylistsFilteredBytes),
		formatBytes(r.PlaylistsDownloadedBytes-r.PlaylistsFilteredBytes),
		reductionPercent(r.PlaylistsDownloadedBytes, r.PlaylistsFilteredBytes)))
	if r.TotalChannelsBefore > 0 || r.RemainingChannels > 0 {
		filtered := r.TotalChannelsBefore - r.RemainingChannels
		if r.TotalChannelsBefore > 0 {
			b.WriteString(fmt.Sprintf("   каналов было: %d\n", r.TotalChannelsBefore))
		}
		if filtered > 0 {
			b.WriteString(fmt.Sprintf("   отфильтровано: %d\n", filtered))
		}
		if r.RemainingChannels > 0 {
			b.WriteString(fmt.Sprintf("   осталось: %d\n", r.RemainingChannels))
		}
	}

	if r.EPGDownloadedBytes > 0 || r.EPGFilteredBytes > 0 {
		b.WriteString("\n📥 EPG:\n")
		b.WriteString(fmt.Sprintf("   скачано: %s\n", formatBytes(r.EPGDownloadedBytes)))
		b.WriteString(fmt.Sprintf("   после фильтрации: %s (-%s, -%.0f%%)\n",
			formatBytes(r.EPGFilteredBytes),
			formatBytes(r.EPGDownloadedBytes-r.EPGFilteredBytes),
			reductionPercent(r.EPGDownloadedBytes, r.EPGFilteredBytes)))
	}

	if len(r.UnavailableBySource) > 0 {
		b.WriteString("\n📡 Недоступные каналы по источникам:\n")
		for idx, count := range r.UnavailableBySource {
			source := "?"
			if idx >= 0 && idx < len(r.M3USourceURLs) {
				source = maskURL(r.M3USourceURLs[idx])
			}
			b.WriteString(fmt.Sprintf("• %s: %d канал(ов) недоступно\n", source, count))
		}
	}

	if len(r.FailedURLs) > 0 {
		b.WriteString("\n⚠️ Не удалось загрузить:\n")
		for _, u := range r.FailedURLs {
			b.WriteString("• " + maskURL(u) + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// reductionPercent returns how many percent of the downloaded data was
// removed by filtering (0 when nothing was downloaded).
func reductionPercent(downloaded, filtered int64) float64 {
	if downloaded <= 0 {
		return 0
	}
	return (1 - float64(filtered)/float64(downloaded)) * 100
}

// formatBytes renders a byte count in a human-readable form (Б, КБ, МБ, ГБ).
func formatBytes(n int64) string {
	if n < 0 {
		n = 0
	}
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d Б", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %s", float64(n)/float64(div), []string{"КБ", "МБ", "ГБ", "ТБ"}[exp])
}

// SendReport posts the formatted report to the Telegram chat via the Bot API
// (POST /bot<token>/sendMessage). Retries with exponential backoff like the
// other network operations in the project. The bot token lives in the request
// path; it is never logged (the sanitizer masks URLs), and the message body
// carries chat_id + text.
func SendReport(ctx context.Context, botToken, chatID string, r Report, skipSSL bool) error {
	if botToken == "" || chatID == "" {
		return fmt.Errorf("telegram: bot token and chat id are required")
	}
	text := FormatReport(r)
	endpoint := fmt.Sprintf("%s/bot%s/sendMessage", apiBase, botToken)

	form := url.Values{}
	form.Set("chat_id", chatID)
	form.Set("text", text)
	form.Set("disable_web_page_preview", "true")

	return utils.RetryWithContext(ctx, retryAttempts, retryDelay, 2.0, func() error {
		attemptCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
		if err != nil {
			return fmt.Errorf("telegram: create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := utils.NewHTTPClient(skipSSL).Do(req)
		if err != nil {
			return fmt.Errorf("telegram: send request: %w", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("telegram: unexpected status %s: %s", resp.Status, strings.TrimSpace(string(body)))
		}
		return nil
	})
}
