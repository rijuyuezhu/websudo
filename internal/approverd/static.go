package approverd

import (
	"bytes"
	"embed"
	"io/fs"
	"mime"
	"net/http"
	pathpkg "path"
	"path/filepath"
	"strings"
	"time"
)

//go:embed static/app/*
var frontendAssets embed.FS

func embeddedFrontendFS() fs.FS {
	sub, err := fs.Sub(frontendAssets, "static/app")
	if err != nil {
		panic(err)
	}
	return sub
}

func (s *Server) handleFrontend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	filePath := strings.TrimPrefix(pathpkg.Clean(r.URL.Path), "/")
	if filePath == "." || filePath == "" {
		filePath = "index.html"
	}
	data, err := fs.ReadFile(s.staticFS, filePath)
	if err != nil {
		filePath = "index.html"
		data, err = fs.ReadFile(s.staticFS, filePath)
	}
	if err != nil {
		http.Error(w, "frontend assets not built", http.StatusInternalServerError)
		return
	}
	if contentType := mime.TypeByExtension(filepath.Ext(filePath)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	http.ServeContent(w, r, filePath, time.Time{}, bytes.NewReader(data))
}
