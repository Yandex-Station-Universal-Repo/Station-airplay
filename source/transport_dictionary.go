package main

import (
	_ "embed"
	"encoding/json"
	"strings"
	"unicode"
)

// The same JSON is consumed by scripts/sync_transport_phrases.py to produce
// YAM's exact permission allowlist. No independently maintained list of 32.
//
//go:embed transport_dictionary.json
var transportDictionaryJSON []byte

var controlPhrases = loadTransportDictionary()

func loadTransportDictionary() map[string]string {
	var phrases map[string]string
	if err := json.Unmarshal(transportDictionaryJSON, &phrases); err != nil {
		panic("invalid embedded transport dictionary")
	}
	return phrases
}

func normalizePhrase(phrase string) []string {
	phrase = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return unicode.ToLower(r)
		}
		return ' '
	}, phrase)
	return strings.Fields(phrase)
}

func controlPhrase(phrase string) string {
	phrase = strings.Join(normalizePhrase(phrase), " ")
	phrase = strings.TrimPrefix(phrase, "алиса ")
	return controlPhrases[phrase]
}
