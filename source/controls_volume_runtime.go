package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func validVolumeDependency(alias string) bool {
	if alias == "" || len(alias) > 64 {
		return false
	}
	for _, b := range []byte(alias) {
		if !(b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '-' || b == '_') {
			return false
		}
	}
	return true
}

// This mode is explicit and never falls back to sibling directories on error.
// Luna authorizes the declared alias and pins YAM payloads for this invocation.
// The existing guarded shell launcher remains the sole native-volume writer.
func findRuntimePackageVolume(ctx context.Context, api *yamAPI, alias string, device yamDevice) (*packageVolume, error) {
	if api == nil || !validVolumeDependency(alias) {
		return nil, errors.New("runtime station-volume requires a valid API and dependency alias")
	}
	var dependency struct {
		PluginID    string   `json:"plugin_id"`
		Directory   string   `json:"directory"`
		Program     string   `json:"program"`
		Interpreter string   `json:"interpreter"`
		Args        []string `json:"args"`
		LibraryPath string   `json:"library_path"`
	}
	if err := api.requestContext(ctx, "GET", "/dependencies/"+alias, nil, &dependency); err != nil {
		return nil, err
	}
	// station-volume's protocol is a plain shell launcher with no fixed flags,
	// especially no --force or alternate interpreter/library override.
	if dependency.PluginID == "" || dependency.Interpreter != "sh" || len(dependency.Args) != 0 || dependency.LibraryPath != "" {
		return nil, errors.New("runtime station-volume must be an unmodified plain shell launcher")
	}
	if !filepath.IsAbs(dependency.Directory) || !filepath.IsAbs(dependency.Program) {
		return nil, errors.New("runtime station-volume paths must be absolute")
	}
	relative, err := filepath.Rel(dependency.Directory, dependency.Program)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, errors.New("runtime station-volume launcher escapes its directory")
	}
	st, err := os.Stat(dependency.Program)
	if err != nil || !st.Mode().IsRegular() {
		return nil, errors.New("runtime station-volume launcher is unavailable")
	}
	return &packageVolume{script: dependency.Program, root: dependency.Directory, device: device, api: api, alias: alias}, nil
}
