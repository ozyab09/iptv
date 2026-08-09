package utils

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// NewHTTPClient creates an HTTP client with optional SSL verification bypass.
func NewHTTPClient(skipSSLVerify bool) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: skipSSLVerify}
	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Minute,
	}
}

// DownloadFile performs an HTTP GET and returns raw bytes, enforcing a maxSize limit.
// Uses the default HTTP client with SSL verification enabled.
func DownloadFile(url string, maxSize int) ([]byte, error) {
	return DownloadFileWithContext(context.Background(), url, maxSize, false)
}

// DownloadFileWithContext performs an HTTP GET with context support, enforcing a maxSize limit.
func DownloadFileWithContext(ctx context.Context, url string, maxSize int, skipSSLVerify bool) ([]byte, error) {
	var content []byte
	err := downloadWithContext(ctx, url, int64(maxSize), skipSSLVerify, func(r io.Reader) error {
		var err error
		content, err = readAllLimited(r, int64(maxSize))
		return err
	})
	return content, err
}

// DownloadFileToPathWithContext downloads a URL directly to destination while
// enforcing maxSize. On failure, a partial destination file is removed.
func DownloadFileToPathWithContext(ctx context.Context, url, destination string, maxSize int64, skipSSLVerify bool) error {
	f, err := os.Create(destination)
	if err != nil {
		return fmt.Errorf("create download destination: %w", err)
	}
	success := false
	defer func() {
		_ = f.Close()
		if !success {
			_ = os.Remove(destination)
		}
	}()

	if err := downloadWithContext(ctx, url, maxSize, skipSSLVerify, func(r io.Reader) error {
		_, err := copyLimited(f, r, maxSize)
		return err
	}); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close download destination: %w", err)
	}
	success = true
	return nil
}

func downloadWithContext(ctx context.Context, url string, maxSize int64, skipSSLVerify bool, consume func(io.Reader) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if maxSize < 1 {
		return fmt.Errorf("maximum download size must be positive")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	resp, err := NewHTTPClient(skipSSLVerify).Do(req)
	if err != nil {
		return fmt.Errorf("HTTP GET failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("HTTP GET returned unexpected status %s", resp.Status)
	}
	if resp.ContentLength > maxSize {
		return fmt.Errorf("file exceeds maximum allowed size of %d bytes", maxSize)
	}
	if err := consume(resp.Body); err != nil {
		return fmt.Errorf("download error: %w", err)
	}
	return nil
}

func readAllLimited(r io.Reader, maxSize int64) ([]byte, error) {
	var buf []byte
	_, err := copyLimited((*sliceWriter)(&buf), r, maxSize)
	return buf, err
}

// sliceWriter lets io.Copy append into a byte slice without exposing it.
type sliceWriter []byte

func (w *sliceWriter) Write(p []byte) (int, error) {
	*w = append(*w, p...)
	return len(p), nil
}

// CopyLimited copies at most maxSize bytes and returns an error if src
// contains additional data. It is shared by download and decompression code.
func CopyLimited(dst io.Writer, src io.Reader, maxSize int64) (int64, error) {
	return copyLimited(dst, src, maxSize)
}

func copyLimited(dst io.Writer, src io.Reader, maxSize int64) (int64, error) {
	if maxSize < 1 {
		return 0, fmt.Errorf("maximum size must be positive")
	}
	n, err := io.Copy(dst, io.LimitReader(src, maxSize+1))
	if err != nil {
		return n, err
	}
	if n > maxSize {
		return n, fmt.Errorf("file exceeds maximum allowed size of %d bytes", maxSize)
	}
	return n, nil
}
