package quote

import (
	"reflect"
	"testing"
)

func TestCategoryNames(t *testing.T) {
	want := []string{"commitment", "growth", "mindset", "purpose", "resilience", "wisdom"}
	if got := CategoryNames(); !reflect.DeepEqual(got, want) {
		t.Errorf("CategoryNames() = %v, want %v", got, want)
	}
}

func TestNormalizeCategory(t *testing.T) {
	cases := map[string]string{
		"growth":     "growth",
		"Resilience": "resilience",
		" WISDOM ":   "wisdom",
		"unknown":    "",
		"":           "",
	}
	for input, want := range cases {
		if got := NormalizeCategory(input); got != want {
			t.Errorf("NormalizeCategory(%q) = %q, want %q", input, got, want)
		}
	}
}
