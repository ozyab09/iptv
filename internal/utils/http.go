package utils

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HTTPClient is a shared HTTP client with disabled SSL verification (needed for local dev).
var HTTPClient = &http.Client{
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	},
	Timeout: 30 * time.Minute,
}

// DownloadFile performs an HTTP GET and returns raw bytes, enforcing a maxSize limit.
// Shared by both M3U and EPG downloaders to avoid code duplication.
func DownloadFile(url string, maxSize int) ([]byte, error) {
	resp, err := HTTPClient.Get(url)
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
