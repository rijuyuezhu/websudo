package approverd

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestFrontendRoutesServeSPAIndex(t *testing.T) {
	srv := NewServer(Dependencies{StaticFS: fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(`<!doctype html><div id="app"></div>`)},
	}})

	for _, path := range []string{"/", "/login", "/askpass/abc", "/requests/abc"} {
		t.Run(path, func(t *testing.T) {
			w := httptest.NewRecorder()
			srv.Routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
			}
			if !strings.Contains(w.Body.String(), `id="app"`) {
				t.Fatalf("body did not contain app root: %s", w.Body.String())
			}
		})
	}
}

func TestFrontendServesBuiltAsset(t *testing.T) {
	srv := NewServer(Dependencies{StaticFS: fstest.MapFS{
		"index.html":     &fstest.MapFile{Data: []byte(`<!doctype html><div id="app"></div>`)},
		"assets/app.css": &fstest.MapFile{Data: []byte(`body{color:#111}`)},
	}})

	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/assets/app.css", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if strings.TrimSpace(w.Body.String()) != "body{color:#111}" {
		t.Fatalf("asset body = %q", w.Body.String())
	}
}

func TestAPIMissDoesNotServeSPAIndex(t *testing.T) {
	srv := NewServer(Dependencies{StaticFS: fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(`<!doctype html><div id="app"></div>`)},
	}})

	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/not-found", nil))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestFrontendReportsMissingBuild(t *testing.T) {
	srv := NewServer(Dependencies{StaticFS: fstest.MapFS{
		"keep.txt": &fstest.MapFile{Data: []byte("placeholder")},
	}})

	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

var _ fs.FS = fstest.MapFS{}
