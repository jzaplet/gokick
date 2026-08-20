package shared_test

import (
	"context"
	"testing"

	"gokick/app/domain/shared"
)

func TestParseLang(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		want   shared.Lang
		wantOK bool
	}{
		{name: "czech", raw: "cs", want: shared.LangCS, wantOK: true},
		{name: "english", raw: "en", want: shared.LangEN, wantOK: true},
		{name: "empty", raw: "", wantOK: false},
		{name: "unsupported", raw: "de", wantOK: false},
		{name: "case sensitive", raw: "CS", wantOK: false},
		{name: "region subtag not normalized", raw: "cs-CZ", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := shared.ParseLang(tt.raw)
			if ok != tt.wantOK {
				t.Fatalf("ParseLang(%q) ok = %v, want %v", tt.raw, ok, tt.wantOK)
			}
			if ok && got != tt.want {
				t.Fatalf("ParseLang(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestLangContext(t *testing.T) {
	ctx := context.Background()

	if got := shared.LangFromContext(ctx); got != shared.LangEN {
		t.Fatalf("LangFromContext(empty ctx) = %q, want the en default", got)
	}

	ctx = shared.WithLang(ctx, shared.LangCS)
	if got := shared.LangFromContext(ctx); got != shared.LangCS {
		t.Fatalf("LangFromContext = %q, want %q", got, shared.LangCS)
	}
}

func TestSupportedLangsParse(t *testing.T) {
	for _, lang := range shared.SupportedLangs() {
		got, ok := shared.ParseLang(string(lang))
		if !ok || got != lang {
			t.Fatalf("ParseLang(%q) = (%q, %v), want (%q, true)", lang, got, ok, lang)
		}
	}
}
