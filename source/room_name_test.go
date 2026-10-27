package main

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestRoomFromYAMDoesNotChangeHardware(t *testing.T) {
	for _, room := range []string{"Кухня", "Детская", "Спальня", "Гостиная"} {
		p, err := profileWithRoom(plumSharedProfile, room)
		if err != nil || p.name != plumSharedProfile.name+" ("+room+")" {
			t.Fatalf("room %q: %+v %v", room, p, err)
		}
		if p.pcm != plumSharedProfile.pcm || p.receiverABI != plumSharedProfile.receiverABI || p.models[0] != plumSharedProfile.models[0] || !p.quasarCoexist {
			t.Fatal("room changed a hardware contract")
		}
	}
}

func TestRoomOptionalAndUTF8Safe(t *testing.T) {
	p, err := profileWithRoom(mini2Profile, "")
	if err != nil || p.name != mini2Profile.name {
		t.Fatal("old YAM without room must keep the original name")
	}
	p, err = profileWithRoom(mini2Profile, strings.Repeat("Длинная комната", 8))
	if err != nil || !utf8.ValidString(p.name) || len(p.name) > 50 || !strings.HasSuffix(p.name, "…)") {
		t.Fatalf("bad UTF8 DNS truncation %q %v", p.name, err)
	}
	for _, room := range []string{"bad\nroom", "bad\x00room", string([]byte{255})} {
		if _, err := profileWithRoom(mini2Profile, room); err == nil {
			t.Fatal("unsafe room accepted")
		}
	}
}
