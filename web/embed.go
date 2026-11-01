package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static/*
var staticEmbed embed.FS

// StaticHandler returns an http.Handler serving the embedded mobile web UI assets
// from the static/ directory at the root path.
func StaticHandler() (http.Handler, error) {
	sub, err := fs.Sub(staticEmbed, "static")
	if err != nil {
		return nil, err
	}
	return http.FileServer(http.FS(sub)), nil
}
