package main

import (
	"errors"
	"os"
	"strings"
)

type runtimeCredentials struct {
	url, token, header string
}

// A token belongs to one invocation and endpoint. Never combine environment
// families or retry a rejected request with another token/header.
func runtimeAPIEnvironment() (runtimeCredentials, error) {
	return resolveRuntimeAPI(
		os.Getenv("LUNA_API_URL"), os.Getenv("LUNA_API_TOKEN"),
		os.Getenv("YAM_API_URL"), os.Getenv("YAM_API_TOKEN"),
	)
}

func resolveRuntimeAPI(lunaURL, lunaToken, yamURL, yamToken string) (runtimeCredentials, error) {
	luna := lunaURL != "" || lunaToken != ""
	yam := yamURL != "" || yamToken != ""
	if luna {
		if lunaURL == "" || lunaToken == "" {
			return runtimeCredentials{}, errors.New("incomplete LUNA_API_URL/LUNA_API_TOKEN pair")
		}
		if yam && (strings.TrimRight(lunaURL, "/") != strings.TrimRight(yamURL, "/") || lunaToken != yamToken) {
			return runtimeCredentials{}, errors.New("conflicting Luna and legacy YAM runtime credentials")
		}
		return runtimeCredentials{lunaURL, lunaToken, "X-Luna-Token"}, nil
	}
	if yamURL == "" || yamToken == "" {
		return runtimeCredentials{}, errors.New("missing runtime credentials: start through Luna or a compatible legacy YAM host, not directly")
	}
	return runtimeCredentials{yamURL, yamToken, "X-YAM-Token"}, nil
}
