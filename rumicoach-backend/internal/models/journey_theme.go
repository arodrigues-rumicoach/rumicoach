package models

import "strings"

// JourneyThemes are the visual themes the frontend can render on the growth
// screen. The slugs must match what the app stores in users.theme ("waterfall"
// is the column default); the display names are Lavender, Fireplace, Mountain
// Lake, Rain, Sunset Beach and The Waterfall.
var JourneyThemes = []string{
	"lavender",
	"fireplace",
	"mountain_lake",
	"rain",
	"sunset_beach",
	"waterfall",
}

// NormalizeJourneyTheme maps a model-provided theme value onto a canonical slug
// ("The Waterfall" → "waterfall", "Mountain Lake" → "mountain_lake"). Returns
// "" when the value matches no known theme.
func NormalizeJourneyTheme(raw string) string {
	slug := strings.ToLower(strings.TrimSpace(raw))
	slug = strings.TrimPrefix(slug, "the ")
	slug = strings.ReplaceAll(slug, " ", "_")
	slug = strings.ReplaceAll(slug, "-", "_")
	for _, t := range JourneyThemes {
		if slug == t {
			return t
		}
	}
	return ""
}
