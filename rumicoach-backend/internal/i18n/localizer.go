package i18n

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
)

type Localizer struct {
	mu           sync.RWMutex
	translations map[string]map[string]string
	defaultLoc   string
}

func NewLocalizer(fsys fs.FS, dirPath string, defaultLoc string) (*Localizer, error) {
	loc := &Localizer{
		translations: make(map[string]map[string]string),
		defaultLoc:   defaultLoc,
	}

	err := loc.loadFromFS(fsys, dirPath)
	if err != nil {
		return nil, err
	}

	return loc, nil
}

func (l *Localizer) loadFromFS(fsys fs.FS, dirPath string) error {
	entries, err := fs.ReadDir(fsys, dirPath)
	if err != nil {
		return fmt.Errorf("failed to read locales directory %s: %w", dirPath, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		filePath := filepath.Join(dirPath, entry.Name())
		data, err := fs.ReadFile(fsys, filePath)
		if err != nil {
			return fmt.Errorf("failed to read locale file %s: %w", filePath, err)
		}

		var translations map[string]string
		if err := json.Unmarshal(data, &translations); err != nil {
			return fmt.Errorf("failed to unmarshal locale file %s: %w", filePath, err)
		}

		localeName := strings.TrimSuffix(entry.Name(), ".json")
		l.translations[localeName] = translations
	}

	return nil
}

func (l *Localizer) Get(locale string, key string, vars map[string]interface{}) string {
	l.mu.RLock()
	defer l.mu.RUnlock()

	// Try the exact locale
	text, found := l.getTranslation(locale, key)
	if !found {
		// Try fallback to just language code (e.g. pt-BR -> pt)
		parts := strings.Split(locale, "-")
		if len(parts) > 1 {
			text, found = l.getTranslation(parts[0], key)
		}
	}

	if !found {
		// Try default locale
		text, found = l.getTranslation(l.defaultLoc, key)
	}

	if !found {
		return key // return key itself if no translation is found
	}

	// Simple variable interpolation {{.var}} -> value
	for k, v := range vars {
		placeholder := fmt.Sprintf("{{.%s}}", k)
		valStr := fmt.Sprintf("%v", v)
		text = strings.ReplaceAll(text, placeholder, valStr)
	}

	return text
}

func (l *Localizer) getTranslation(locale string, key string) (string, bool) {
	locDict, ok := l.translations[locale]
	if !ok {
		return "", false
	}
	text, ok := locDict[key]
	return text, ok
}
