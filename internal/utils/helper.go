package utils

import (
	"strings"
)

// ToLowerSlice converts a slice of strings to lowercase, returning a new slice.
func ToLowerSlice(src []string) []string {
	if src == nil {
		return nil
	}
	dst := make([]string, len(src))
	for i, s := range src {
		dst[i] = strings.ToLower(s)
	}
	return dst
}

// NormalizeLineEndings converts \r\n to \n for consistent handling.
func NormalizeLineEndings(content string) string {
	return strings.ReplaceAll(content, "\r\n", "\n")
}
