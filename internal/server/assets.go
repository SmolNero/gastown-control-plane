package server

import (
	"embed"
	"io/fs"
)

//go:embed web/*
var webFS embed.FS

func WebFS() fs.FS {
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		return webFS
	}
	return sub
}
