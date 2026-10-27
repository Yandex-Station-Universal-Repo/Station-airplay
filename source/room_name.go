package main

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Room comes from the shared YAM device API, not per-plugin configuration.
// Other YAM versions omit it, preserving the original receiver name.
func profileWithRoom(p stationProfile, room string) (stationProfile, error) {
	if !utf8.ValidString(room) || strings.ContainsFunc(room, unicode.IsControl) {
		return stationProfile{}, errors.New("YAM room must be valid text without control characters")
	}
	room = strings.TrimSpace(room)
	if room == "" {
		return p, nil
	}
	prefix := p.name + " ("
	// RAOP prepends12MAC hex digits and '@': DNS label limit63 leaves50bytes.
	budget := 50 - len(prefix) - 1
	if budget < 3 {
		return stationProfile{}, errors.New("receiver base name too long for a room suffix")
	}
	if len(room) > budget {
		room = utf8Prefix(room, budget-3) + "…"
	}
	p.name = prefix + room + ")"
	return p, nil
}

func utf8Prefix(s string, budget int) string {
	end := 0
	for index, r := range s {
		next := index + utf8.RuneLen(r)
		if next > budget {
			break
		}
		end = next
	}
	return s[:end]
}
