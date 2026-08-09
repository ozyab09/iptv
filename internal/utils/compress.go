package utils

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
)

// DecompressGZip decompresses gzip-compressed data and returns the raw bytes.
func DecompressGZip(data []byte) ([]byte, error) {
	return DecompressGZipLimited(data, int64(^uint(0)>>1))
}

// DecompressGZipLimited decompresses data while refusing output larger than
// maxSize. The limit applies to decompressed bytes, not the archive itself.
func DecompressGZipLimited(data []byte, maxSize int64) ([]byte, error) {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gr.Close()

	var out bytes.Buffer
	_, err = CopyLimited(&out, gr, maxSize)
	if err != nil {
		return nil, fmt.Errorf("failed to read gzip data: %w", err)
	}
	return out.Bytes(), nil
}

// DecompressZip extracts the first file from a zip archive and returns its contents.
func DecompressZip(data []byte) ([]byte, error) {
	return DecompressZipLimited(data, int64(^uint(0)>>1))
}

// DecompressZipLimited extracts the first non-directory entry while refusing
// output larger than maxSize.
func DecompressZipLimited(data []byte, maxSize int64) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("failed to create zip reader: %w", err)
	}
	return ReadFirstZipEntryLimited(zr, maxSize)
}

// ReadFirstZipEntryLimited extracts the first non-directory entry from zr.
func ReadFirstZipEntryLimited(zr *zip.Reader, maxSize int64) ([]byte, error) {
	var selected *zip.File
	for _, f := range zr.File {
		if !f.FileInfo().IsDir() {
			selected = f
			break
		}
	}
	if selected == nil {
		return nil, fmt.Errorf("zip archive is empty")
	}
	if selected.UncompressedSize64 > uint64(maxSize) {
		return nil, fmt.Errorf("zip entry exceeds maximum allowed size of %d bytes", maxSize)
	}
	f, err := selected.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open zip entry: %w", err)
	}
	defer f.Close()

	var out bytes.Buffer
	_, err = CopyLimited(&out, f, maxSize)
	if err != nil {
		return nil, fmt.Errorf("failed to read zip entry: %w", err)
	}
	return out.Bytes(), nil
}

// IsGzipped detects gzip magic bytes (0x1f, 0x8b).
func IsGzipped(data []byte) bool {
	return len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b
}

// IsZipped detects ZIP magic bytes (PK\x03\x04, PK\x05\x06, or PK\x07\x08).
func IsZipped(data []byte) bool {
	return len(data) >= 4 && data[0] == 'P' && data[1] == 'K' &&
		(data[2] == 3 || data[2] == 5 || data[2] == 7) &&
		(data[3] == 4 || data[3] == 6 || data[3] == 8)
}
