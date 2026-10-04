package approverd

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestFrontendRoutesServeIndex(t *testing.T) {
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

func TestFrontendServesStaticAsset(t *testing.T) {
	srv := NewServer(Dependencies{StaticFS: fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(`<!doctype html><div id="app"></div>`)},
		"styles.css": &fstest.MapFile{Data: []byte(`body{color:#111}`)},
	}})

	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/styles.css", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if strings.TrimSpace(w.Body.String()) != "body{color:#111}" {
		t.Fatalf("asset body = %q", w.Body.String())
	}
}

func TestAPIMissDoesNotServeFrontendIndex(t *testing.T) {
	srv := NewServer(Dependencies{StaticFS: fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(`<!doctype html><div id="app"></div>`)},
	}})

	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/not-found", nil))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestFrontendAssetMissReturnsNotFound(t *testing.T) {
	srv := NewServer(Dependencies{StaticFS: fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(`<!doctype html><div id="app"></div>`)},
	}})

	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/missing.js", nil))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestFrontendReportsMissingAssets(t *testing.T) {
	srv := NewServer(Dependencies{StaticFS: fstest.MapFS{
		"other.txt": &fstest.MapFile{Data: []byte("placeholder")},
	}})

	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestBrowserSecurityHeaders(t *testing.T) {
	srv := NewServer(Dependencies{StaticFS: fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(`<!doctype html><div id="app"></div>`)},
	}})

	for _, path := range []string{"/", "/api/session"} {
		t.Run(path, func(t *testing.T) {
			w := httptest.NewRecorder()
			srv.Routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
			if got := w.Header().Get("Content-Security-Policy"); !strings.Contains(got, "frame-ancestors 'none'") {
				t.Fatalf("Content-Security-Policy = %q, want frame-ancestors 'none'", got)
			}
			if got := w.Header().Get("X-Frame-Options"); got != "DENY" {
				t.Fatalf("X-Frame-Options = %q, want DENY", got)
			}
			if strings.HasPrefix(path, "/api/") && w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("Cache-Control = %q, want no-store", w.Header().Get("Cache-Control"))
			}
		})
	}
}

func TestEmbeddedFrontendAssets(t *testing.T) {
	frontend := embeddedFrontendFS()
	for _, name := range []string{"index.html", "styles.css", "app.js"} {
		t.Run(name, func(t *testing.T) {
			data, err := fs.ReadFile(frontend, name)
			if err != nil {
				t.Fatalf("ReadFile(%q) error = %v", name, err)
			}
			if len(data) == 0 {
				t.Fatalf("ReadFile(%q) returned an empty asset", name)
			}
		})
	}

	index, err := fs.ReadFile(frontend, "index.html")
	if err != nil {
		t.Fatalf("ReadFile(index.html) error = %v", err)
	}
	for _, ref := range []string{`href="/styles.css"`, `src="/app.js"`} {
		if !strings.Contains(string(index), ref) {
			t.Fatalf("index.html missing asset reference %q", ref)
		}
	}
}

var _ fs.FS = fstest.MapFS{}
