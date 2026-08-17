package models

import "testing"

func TestNormalizeJourneyTheme(t *testing.T) {
	cases := map[string]string{
		"waterfall":     "waterfall",
		"The Waterfall": "waterfall",
		"Mountain Lake": "mountain_lake",
		"mountain-lake": "mountain_lake",
		"SUNSET_BEACH":  "sunset_beach",
		" rain ":        "rain",
		"Lavender":      "lavender",
		"fireplace":     "fireplace",
		"space":         "",
		"":              "",
	}
	for input, want := range cases {
		if got := NormalizeJourneyTheme(input); got != want {
			t.Errorf("NormalizeJourneyTheme(%q) = %q, want %q", input, got, want)
		}
	}
}
