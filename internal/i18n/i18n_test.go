package i18n

import (
	"testing"
)

func TestNewBundle_DefaultEN(t *testing.T) {
	b, err := NewBundle("en")
	if err != nil {
		t.Fatalf("NewBundle: %v", err)
	}
	langs := b.Languages()
	if len(langs) < 2 {
		t.Errorf("expected at least 2 languages, got %d", len(langs))
	}
}

func TestNewBundle_DefaultKO(t *testing.T) {
	b, err := NewBundle("ko")
	if err != nil {
		t.Fatalf("NewBundle: %v", err)
	}
	if b.defaultLang != "ko" {
		t.Errorf("expected default ko, got %q", b.defaultLang)
	}
}

func TestNewBundle_InvalidDefault(t *testing.T) {
	_, err := NewBundle("zz")
	if err == nil {
		t.Fatal("expected error for unknown default language")
	}
}

func TestLocalizer_T_English(t *testing.T) {
	b, _ := NewBundle("en")
	l := b.NewLocalizer("en")
	got := l.T(ToolListAgentsDesc)
	if got != "List all configured backend A2A agents and their current health status." {
		t.Errorf("unexpected translation: %q", got)
	}
}

func TestLocalizer_T_Korean(t *testing.T) {
	b, _ := NewBundle("en")
	l := b.NewLocalizer("ko")
	got := l.T(ToolListAgentsDesc)
	if got == "List all configured backend A2A agents and their current health status." {
		t.Error("expected Korean translation, got English")
	}
	if got == ToolListAgentsDesc {
		t.Error("got key itself, translation missing")
	}
}

func TestLocalizer_T_FallbackToDefault(t *testing.T) {
	b, _ := NewBundle("en")
	l := b.NewLocalizer("ko")
	// If a key exists only in en, should fallback
	got := l.T("nonexistent.key.that.does.not.exist")
	if got != "nonexistent.key.that.does.not.exist" {
		t.Errorf("expected key itself for missing translation, got %q", got)
	}
}

func TestLocalizer_Tf(t *testing.T) {
	b, _ := NewBundle("en")
	l := b.NewLocalizer("en")
	got := l.Tf(ErrUnknownTool, "my_tool")
	if got != "unknown tool: my_tool" {
		t.Errorf("unexpected formatted translation: %q", got)
	}
}

func TestLocalizer_Tf_Korean(t *testing.T) {
	b, _ := NewBundle("en")
	l := b.NewLocalizer("ko")
	got := l.Tf(ErrUnknownTool, "my_tool")
	if got == "unknown tool: my_tool" {
		t.Error("expected Korean formatted translation")
	}
}

func TestMatchLanguage_ExactMatch(t *testing.T) {
	b, _ := NewBundle("en")
	l := b.NewLocalizer("ko")
	if l.Lang() != "ko" {
		t.Errorf("expected ko, got %q", l.Lang())
	}
}

func TestMatchLanguage_WithRegion(t *testing.T) {
	b, _ := NewBundle("en")
	l := b.NewLocalizer("ko-KR")
	if l.Lang() != "ko" {
		t.Errorf("expected ko from ko-KR, got %q", l.Lang())
	}
}

func TestMatchLanguage_QualityWeights(t *testing.T) {
	b, _ := NewBundle("en")
	l := b.NewLocalizer("ko-KR,ko;q=0.9,en;q=0.8")
	if l.Lang() != "ko" {
		t.Errorf("expected ko (highest q), got %q", l.Lang())
	}
}

func TestMatchLanguage_PreferFirst(t *testing.T) {
	b, _ := NewBundle("en")
	l := b.NewLocalizer("en,ko;q=0.5")
	if l.Lang() != "en" {
		t.Errorf("expected en (q=1.0), got %q", l.Lang())
	}
}

func TestMatchLanguage_Empty(t *testing.T) {
	b, _ := NewBundle("en")
	l := b.NewLocalizer("")
	if l.Lang() != "en" {
		t.Errorf("expected default en for empty, got %q", l.Lang())
	}
}

func TestMatchLanguage_Unsupported(t *testing.T) {
	b, _ := NewBundle("en")
	l := b.NewLocalizer("fr-FR,de;q=0.9")
	if l.Lang() != "en" {
		t.Errorf("expected default en for unsupported, got %q", l.Lang())
	}
}

func TestAllKeysPresent(t *testing.T) {
	b, err := NewBundle("en")
	if err != nil {
		t.Fatalf("NewBundle: %v", err)
	}

	en := b.translations["en"]
	ko := b.translations["ko"]

	// Every key in en should exist in ko
	for key := range en {
		if _, ok := ko[key]; !ok {
			t.Errorf("key %q present in en.json but missing in ko.json", key)
		}
	}

	// Every key in ko should exist in en
	for key := range ko {
		if _, ok := en[key]; !ok {
			t.Errorf("key %q present in ko.json but missing in en.json", key)
		}
	}
}

func TestLocalizer_Lang(t *testing.T) {
	b, _ := NewBundle("en")

	tests := []struct {
		accept string
		want   string
	}{
		{"en", "en"},
		{"ko", "ko"},
		{"ko-KR", "ko"},
		{"en-US", "en"},
		{"", "en"},
		{"fr", "en"},
	}

	for _, tt := range tests {
		l := b.NewLocalizer(tt.accept)
		if l.Lang() != tt.want {
			t.Errorf("NewLocalizer(%q).Lang() = %q, want %q", tt.accept, l.Lang(), tt.want)
		}
	}
}
