package utils

// isEmojiRune reports whether r is an emoji or emoji-related code point.
func isEmojiRune(r rune) bool {
	// Regional indicators (flags) — NOT covered by 0x1F300+ range.
	if r >= 0x1F1E0 && r <= 0x1F1FF {
		return true
	}
	// Main emoji block: misc symbols, pictographs, emoticons, transport.
	if r >= 0x1F300 && r <= 0x1F9FF {
		return true
	}
	// Additional ranges outside the main emoji block.
	switch {
	case r >= 0x2600 && r <= 0x27BF:
		return true
	case r >= 0xFE00 && r <= 0xFE0F:
		return true
	case r == 0x200D:
		return true
	case r == 0x2B50 || r == 0x2B55:
		return true
	case r >= 0x20E3 && r <= 0x20E3:
		return true
	case r >= 0x231A && r <= 0x231B:
		return true
	case r >= 0x23F0 && r <= 0x23F3:
		return true
	case r == 0x00A9 || r == 0x00AE:
		return true
	case r == 0x2122:
		return true
	case r == 0x3030 || r == 0x303D:
		return true
	case r == 0x3297 || r == 0x3299:
		return true
	default:
		return false
	}
}

// StripTrailingEmoji removes trailing emoji symbols and surrounding whitespace
// from s. It is used to normalize channel names that carry emoji identifiers
// (e.g. "Первый канал 🔴🐱") before matching them against EPG display names.
func StripTrailingEmoji(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	end := len(runes)
	for end > 0 {
		r := runes[end-1]
		if isEmojiRune(r) || r == ' ' || r == '\t' || r == '\u00A0' {
			end--
		} else {
			break
		}
	}
	return string(runes[:end])
}
