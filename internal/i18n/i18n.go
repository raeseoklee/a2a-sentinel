package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

//go:embed locales/*.json
var localeFS embed.FS

// Bundle holds all loaded translations keyed by language code.
type Bundle struct {
	translations map[string]map[string]string // lang -> key -> value
	defaultLang  string
}

// NewBundle creates a Bundle by loading all embedded JSON locale files.
// defaultLang is the fallback language (e.g., "en").
func NewBundle(defaultLang string) (*Bundle, error) {
	b := &Bundle{
		translations: make(map[string]map[string]string),
		defaultLang:  defaultLang,
	}

	entries, err := localeFS.ReadDir("locales")
	if err != nil {
		return nil, fmt.Errorf("reading locales dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		lang := strings.TrimSuffix(entry.Name(), ".json")
		data, err := localeFS.ReadFile("locales/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("reading locale %s: %w", lang, err)
		}

		var messages map[string]string
		if err := json.Unmarshal(data, &messages); err != nil {
			return nil, fmt.Errorf("parsing locale %s: %w", lang, err)
		}
		b.translations[lang] = messages
	}

	if _, ok := b.translations[defaultLang]; !ok {
		return nil, fmt.Errorf("default language %q not found in embedded locales", defaultLang)
	}

	return b, nil
}

// Languages returns the list of supported language codes.
func (b *Bundle) Languages() []string {
	langs := make([]string, 0, len(b.translations))
	for lang := range b.translations {
		langs = append(langs, lang)
	}
	sort.Strings(langs)
	return langs
}

// Localizer translates message keys for a specific language.
type Localizer struct {
	lang   string
	bundle *Bundle
}

// NewLocalizer creates a Localizer from an Accept-Language header value.
// It picks the best matching language from the bundle. Falls back to defaultLang.
func (b *Bundle) NewLocalizer(acceptLanguage string) *Localizer {
	lang := b.matchLanguage(acceptLanguage)
	return &Localizer{lang: lang, bundle: b}
}

// Lang returns the resolved language code for this localizer.
func (l *Localizer) Lang() string {
	return l.lang
}

// T returns the translated string for key in the localizer's language.
// Falls back to the default language, then returns the key itself if not found.
func (l *Localizer) T(key string) string {
	if msgs, ok := l.bundle.translations[l.lang]; ok {
		if val, ok := msgs[key]; ok {
			return val
		}
	}
	// Fallback to default language
	if l.lang != l.bundle.defaultLang {
		if msgs, ok := l.bundle.translations[l.bundle.defaultLang]; ok {
			if val, ok := msgs[key]; ok {
				return val
			}
		}
	}
	return key // Return key as last resort
}

// Tf returns the translated string with fmt.Sprintf formatting applied.
func (l *Localizer) Tf(key string, args ...interface{}) string {
	return fmt.Sprintf(l.T(key), args...)
}

// matchLanguage parses Accept-Language and returns the best match.
// Format: "ko-KR,ko;q=0.9,en;q=0.8"
func (b *Bundle) matchLanguage(acceptLanguage string) string {
	if acceptLanguage == "" {
		return b.defaultLang
	}

	type langQ struct {
		lang string
		q    float64
	}

	var candidates []langQ
	for _, part := range strings.Split(acceptLanguage, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		fields := strings.SplitN(part, ";", 2)
		tag := strings.TrimSpace(fields[0])
		q := 1.0

		if len(fields) == 2 {
			qPart := strings.TrimSpace(fields[1])
			if strings.HasPrefix(qPart, "q=") {
				if parsed, err := strconv.ParseFloat(qPart[2:], 64); err == nil {
					q = parsed
				}
			}
		}

		candidates = append(candidates, langQ{lang: tag, q: q})
	}

	// Sort by quality descending
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].q > candidates[j].q
	})

	// Try exact match first, then base language (e.g., "ko-KR" -> "ko")
	for _, c := range candidates {
		if _, ok := b.translations[c.lang]; ok {
			return c.lang
		}
		base := strings.SplitN(c.lang, "-", 2)[0]
		if _, ok := b.translations[base]; ok {
			return base
		}
	}

	return b.defaultLang
}
