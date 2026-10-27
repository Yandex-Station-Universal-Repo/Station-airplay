package main

import (
	"strings"
	"testing"
)

func TestControlMetadataIsNotDisabledWithDACP(t *testing.T) {
	plain := receiverControlConfig(mini2Profile, nil)
	if !strings.Contains(plain, `metadata = { enabled = "no"; };`) {
		t.Fatal("audio-only default changed")
	}
	for _, native := range []bool{false, true} {
		config := receiverControlConfig(mini2Profile, &controlsConfig{Voice: true, NativeVolume: native})
		if !strings.Contains(config, `metadata = { enabled = "yes";`) {
			t.Fatal("DACP metadata hub disabled")
		}
		if !strings.Contains(config, `include_cover_art = "no"`) || !strings.Contains(config, `pipe_name = "/run/station-airplay/control-metadata"`) {
			t.Fatal("metadata privacy scope missing")
		}
		if strings.Contains(config, `ignore_volume_control = "yes"`) != native {
			t.Fatal("source attenuation policy changed")
		}
	}
}
