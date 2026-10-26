package config

// adultSubstrings is the list of substrings used to match against category names
// for adult/18+ content filtering. These are matched case-insensitively against
// the normalized group-title. Matching is substring-based, so an entry like
// "взрослые" will catch categories like "Взрослые", "🔞 Взрослые", "Adult (18+)",
// "Adult (18+) ❤️" and similar variants that would otherwise escape the
// exact-match CategoriesToRemove list.
var adultSubstrings = []string{
	// Russian
	"взрослые",
	"для взрослых",
	"18+",

	// English variants (category names, not channel names)
	"adult",
	"18+",
	"xxx",
}

// AdultSubstrings returns the slice of adult category substrings. The slice is
// guaranteed non-nil so callers can range over it safely.
func AdultSubstrings() []string {
	return adultSubstrings
}
