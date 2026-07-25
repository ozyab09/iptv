package utils

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"time"
)

// NewHTTPClient creates an HTTP client with optional SSL verification bypass.
func NewHTTPClient(skipSSLVerify bool) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: skipSSLVerify},
		},
		Timeout: 30 * time.Minute,
	}
}

// DownloadFile performs an HTTP GET and returns raw bytes, enforcing a maxSize limit.
// Uses the default HTTP client with SSL verification enabled.
func DownloadFile(url string, maxSize int) ([]byte, error) {
	return DownloadFileWithContext(context.Background(), url, maxSize, false)
}

// DownloadFileWithContext performs an HTTP GET with context support, enforcing a maxSize limit.
func DownloadFileWithContext(ctx context.Context, url string, maxSize int, skipSSLVerify bool) ([]byte, error) {
	client := NewHTTPClient(skipSSLVerify)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP GET failed: %w", err)
	}
	defer resp.Body.Close()

	var chunks []byte
	totalSize := 0
	buf := make([]byte, 32768)

	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			totalSize += n
			if totalSize > maxSize {
				return nil, fmt.Errorf("file exceeds maximum allowed size of %d bytes", maxSize)
			}
			chunks = append(chunks, buf[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("download error: %w", err)
		}
	}

	return chunks, nil
}
