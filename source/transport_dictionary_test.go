package main

import (
	"os"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

// readPackageManifest finds plugin.toml both from source/ (where go test runs
// this package) and from the repository root.
func readPackageManifest(t *testing.T) []byte {
	t.Helper()
	for _, path := range []string{"plugin.toml", "../plugin.toml"} {
		text, err := os.ReadFile(path)
		if err == nil {
			return text
		}
	}
	t.Fatal("plugin.toml not found in source/ or the repository root")
	return nil
}

func TestEveryTransportPhraseIsDeclared(t *testing.T) {
	var manifest struct {
		Commands struct{ Phrases []string }
	}
	text := readPackageManifest(t)
	if _, err := toml.Decode(string(text), &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Commands.Phrases) != len(controlPhrases) || len(controlPhrases) > 8192 {
		t.Fatalf("manifest=%d dictionary=%d", len(manifest.Commands.Phrases), len(controlPhrases))
	}
	declared := map[string]bool{}
	for _, phrase := range manifest.Commands.Phrases {
		declared[phrase] = true
		if controlPhrase(phrase) == "" {
			t.Fatalf("declared phrase lacks handler: %q", phrase)
		}
	}
	for phrase, command := range controlPhrases {
		if !declared[phrase] || len(phrase) > 128 || strings.Join(normalizePhrase(phrase), " ") != phrase {
			t.Fatalf("invalid or undeclared phrase: %q", phrase)
		}
		if got := controlPhrase("Алиса, " + phrase + "!"); got != command {
			t.Fatalf("%q = %s, want %s", phrase, got, command)
		}
		for _, negative := range []string{"не " + phrase, "расскажи про " + phrase, phrase + " и включи свет"} {
			if controlPhrase(negative) != "" {
				t.Fatalf("unsafe substring match: %q", negative)
			}
		}
	}
}

func TestReportedSkipAndCommonVariants(t *testing.T) {
	for phrase, want := range map[string]string{
		"Алиса, скип!": "Next", "скипни": "Next", "пропустить": "Next",
		"переключи на следующую песню": "Next", "поставь мне следующий трек пожалуйста": "Next",
		"пожалуйста верни предыдущую песню": "Previous", "Алиса, вперёд": "Next",
		"возобновить музыку": "Play", "сними с паузы": "Play", "остановить": "Pause",
		"начни с самого начала": "Restart", "повтори эту песню": "Restart",
	} {
		if got := controlPhrase(phrase); got != want {
			t.Fatalf("%q = %s, want %s", phrase, got, want)
		}
	}
	for _, phrase := range []string{"повтори ответ", "повтори сказанное", "включи свет", "не переключай", "что дальше", "поставь таймер", "включи другую музыку в спальне"} {
		if got := controlPhrase(phrase); got != "" {
			t.Fatalf("unrelated command %q = %s", phrase, got)
		}
	}
}
