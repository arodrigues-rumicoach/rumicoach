package chat

import (
	"os"
	"testing"

	"github.com/rumi/rumi-be/config"
	"github.com/rumi/rumi-be/internal/services/aiusage"
)

// The same guard as the companion package's, for the models this one calls. It lives
// here because the defaults do: a copy over in aiusage would have to hard-code them
// and would keep passing after someone changed one.
//
// The defaults are read the way the call sites read them (os.Getenv with an inline
// fallback), so this checks the value that would actually be sent to the provider.
func TestChatModelsArePriced(t *testing.T) {
	reviewModel := os.Getenv("GEMINI_REVIEW_MODEL")
	if reviewModel == "" {
		reviewModel = "gemini-3.5-flash"
	}
	recommendationModel := os.Getenv("GEMINI_RECOMMENDATION_MODEL")
	if recommendationModel == "" {
		recommendationModel = "gemini-3.1-pro-preview"
	}

	cases := []struct {
		what  string
		model string
	}{
		{"session recap and QA review", reviewModel},
		{"recommendation agent", recommendationModel},
	}

	// The live model comes from config rather than a package constant, so it can drift
	// on its own. Skipped when config was never loaded (most unit tests).
	if config.AppConfig != nil && config.AppConfig.GeminiLiveModel != "" {
		cases = append(cases, struct {
			what  string
			model string
		}{"live voice session", config.AppConfig.GeminiLiveModel})
	}

	for _, c := range cases {
		if _, ok := aiusage.PriceFor(c.model); !ok {
			t.Errorf("%s uses %q, which has no entry in aiusage/prices.go — its cost would record as NULL",
				c.what, c.model)
		}
	}
}
