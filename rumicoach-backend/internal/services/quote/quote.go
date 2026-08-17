package quote

import (
	"math/rand"
	"sort"
	"strings"

	"go.uber.org/zap"
)

func ptr(s string) *string {
	return &s
}

type QuoteData struct {
	ID           string
	Author       *string
	Category     string
	Translations map[string]string // lang -> translated text
}

type QuoteService struct {
	logger           *zap.Logger
	allQuotes        []QuoteData
	quotesByCategory map[string][]QuoteData
}

var GlobalQuoteService = &QuoteService{}

// categorySources is the single source of truth for category names and their
// quote slices. Package-level (not built in InitQuoteService) so CategoryNames
// and NormalizeCategory work at any time, e.g. when tool schemas are declared.
var categorySources = map[string][]QuoteData{
	"growth":     growthQuotes,
	"resilience": resilienceQuotes,
	"commitment": actionQuotes,
	"mindset":    mindsetQuotes,
	"wisdom":     wisdomQuotes,
	"purpose":    purposeQuotes,
}

func InitQuoteService(l *zap.Logger) {
	GlobalQuoteService.logger = l
	GlobalQuoteService.quotesByCategory = make(map[string][]QuoteData)

	allQuotes := []QuoteData{}
	for cat, qList := range categorySources {
		GlobalQuoteService.quotesByCategory[cat] = qList
		allQuotes = append(allQuotes, qList...)
	}

	GlobalQuoteService.allQuotes = allQuotes
}

// CategoryNames returns the valid quote category names (sorted), for tool
// enums and validation.
func CategoryNames() []string {
	cats := make([]string, 0, len(categorySources))
	for c := range categorySources {
		cats = append(cats, c)
	}
	sort.Strings(cats)
	return cats
}

// NormalizeCategory maps a model-provided category onto a canonical name,
// returning "" when it matches no known category.
func NormalizeCategory(raw string) string {
	cat := strings.ToLower(strings.TrimSpace(raw))
	if _, ok := categorySources[cat]; ok {
		return cat
	}
	return ""
}

func (s *QuoteService) GetRandomQuoteData(lang string, category *string) (string, *string, string, string) {
	var quotes []QuoteData
	if category != nil && *category != "" {
		quotes = s.quotesByCategory[*category]
	} else {
		quotes = s.allQuotes
	}

	if len(quotes) == 0 {
		return "", nil, "", ""
	}
	index := rand.Intn(len(quotes))
	quote := quotes[index]

	text := ""
	// 1. Try exact match (e.g. es-ES)
	if t, ok := quote.Translations[lang]; ok {
		text = t
	} else {
		// 2. Try base language match (e.g. es-ES -> es)
		baseLang := strings.Split(lang, "-")[0]
		if t, ok := quote.Translations[baseLang]; ok {
			text = t
		} else {
			// 3. Default to en-US
			if t, ok := quote.Translations["en-US"]; ok {
				text = t
			} else {
				// 4. Fallback to any translation if en-US is missing
				for _, t := range quote.Translations {
					text = t
					break
				}
			}
		}
	}

	return text, quote.Author, quote.Category, quote.ID
}

func (s *QuoteService) GetQuoteByID(id string, lang string) (string, *string, string, bool) {
	var target QuoteData
	found := false
	for _, q := range s.allQuotes {
		if q.ID == id {
			target = q
			found = true
			break
		}
	}
	if !found {
		return "", nil, "", false
	}

	text := ""
	if t, ok := target.Translations[lang]; ok {
		text = t
	} else {
		baseLang := strings.Split(lang, "-")[0]
		if t, ok := target.Translations[baseLang]; ok {
			text = t
		} else {
			if t, ok := target.Translations["en-US"]; ok {
				text = t
			} else {
				for _, t := range target.Translations {
					text = t
					break
				}
			}
		}
	}

	return text, target.Author, target.Category, true
}
