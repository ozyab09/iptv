package utils

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
)

// DecompressGZip decompresses gzip-compressed data and returns the raw bytes.
func DecompressGZip(data []byte) ([]byte, error) {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gr.Close()

	out, err := io.ReadAll(gr)
	if err != nil {
		return nil, fmt.Errorf("failed to read gzip data: %w", err)
	}
	return out, nil
}

// DecompressZip extracts the first file from a zip archive and returns its contents.
func DecompressZip(data []byte) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("failed to create zip reader: %w", err)
	}
	if len(zr.File) == 0 {
		return nil, fmt.Errorf("zip archive is empty")
	}
	f, err := zr.File[0].Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open first file in zip: %w", err)
	}
	defer f.Close()

	out, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("failed to read zip entry: %w", err)
	}
	return out, nil
}

// IsGzipped detects gzip magic bytes (0x1f, 0x8b).
func IsGzipped(data []byte) bool {
	return len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b
}


