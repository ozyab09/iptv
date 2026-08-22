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

// LevenshteinDistance returns the edit distance between two strings (insert,
// delete, substitute = 1). Used for fuzzy name matching.
func LevenshteinDistance(a, b string) int {
	aRunes := []rune(a)
	bRunes := []rune(b)
	la, lb := len(aRunes), len(bRunes)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	// Two-row DP keeps memory O(min(la,lb)) and is fast enough for short names.
	if la > lb {
		aRunes, bRunes = bRunes, aRunes
		la, lb = lb, la
	}
	prev := make([]int, la+1)
	curr := make([]int, la+1)
	for i := 0; i <= la; i++ {
		prev[i] = i
	}
	for j := 1; j <= lb; j++ {
		curr[0] = j
		bj := bRunes[j-1]
		for i := 1; i <= la; i++ {
			cost := 1
			if aRunes[i-1] == bj {
				cost = 0
			}
			curr[i] = min3(curr[i-1]+1, prev[i]+1, prev[i-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[la]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
