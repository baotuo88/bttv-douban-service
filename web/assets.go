package webassets

import (
	"embed"
	"io/fs"
)

//go:embed static/*
var assets embed.FS

// ReadIndexHTML returns embedded admin dashboard HTML.
func ReadIndexHTML() ([]byte, error) {
	return assets.ReadFile("static/index.html")
}

// StaticFS returns embedded static assets under /static.
func StaticFS() (fs.FS, error) {
	return fs.Sub(assets, "static")
}
