package server

import (
	"embed"
	"io/fs"
)

//go:embed static
var staticEmbed embed.FS

var staticFS fs.FS

var (
	dashboardHTML []byte
	logsHTML      []byte
)

func init() {
	var err error
	staticFS, err = fs.Sub(staticEmbed, "static")
	if err != nil {
		panic("server: failed to create static sub-filesystem: " + err.Error())
	}

	dashboardHTML, err = staticEmbed.ReadFile("static/index.html")
	if err != nil {
		panic("server: failed to read embedded dashboard HTML: " + err.Error())
	}

	logsHTML, err = staticEmbed.ReadFile("static/logs.html")
	if err != nil {
		panic("server: failed to read embedded logs HTML: " + err.Error())
	}
}
