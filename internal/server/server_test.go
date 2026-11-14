package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/straightdave/sgai/internal/server"
)

func testFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html": {
			Data: []byte("<!doctype html><html><body><div id=\"root\"></div></body></html>"),
		},
		"assets/app.js": {
			Data: []byte("console.log('ok');"),
		},
	}
}

func testPrivFS() fstest.MapFS {
	return fstest.MapFS{
		"x36xhzz.m3u8": {
			Data: []byte("#EXTM3U\n"),
		},
	}
}

func TestHealthEndpoint(t *testing.T) {
	h := server.New(testFS(), testPrivFS())
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("expected status 'ok', got %q", body["status"])
	}
}

func TestUnknownAPIRouteReturns404JSON(t *testing.T) {
	h := server.New(testFS(), testPrivFS())
	req := httptest.NewRequest(http.MethodGet, "/api/does-not-exist", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("expected JSON content-type, got %q", ct)
	}
}

func TestStaticAssetAndSPAFallback(t *testing.T) {
	h := server.New(testFS(), testPrivFS())

	t.Run("root serves index.html", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), `<div id="root">`) {
			t.Fatalf("expected index.html body, got %q", rec.Body.String())
		}
	})

	t.Run("static asset served directly", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "console.log('ok')") {
			t.Fatalf("unexpected asset body: %q", rec.Body.String())
		}
	})

	t.Run("client-side route falls back to index.html", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/dashboard/settings", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), `<div id="root">`) {
			t.Fatalf("expected index.html fallback, got %q", rec.Body.String())
		}
	})
}

func TestPrivHLSManifestServing(t *testing.T) {
	h := server.New(testFS(), testPrivFS())

	t.Run("serves local manifest", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/priv/x36xhzz.m3u8", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "mpegurl") {
			t.Fatalf("expected mpegurl content-type, got %q", ct)
		}
		if !strings.Contains(rec.Body.String(), "#EXTM3U") {
			t.Fatalf("expected playlist content, got %q", rec.Body.String())
		}
	})

	t.Run("missing file returns 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/priv/missing.m3u8", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected status 404, got %d", rec.Code)
		}
	})
}

func TestAdsEndpoint(t *testing.T) {
	h := server.New(testFS(), testPrivFS())
	req := httptest.NewRequest(http.MethodGet, "/api/ads?_HLS_start_offset=0", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var body struct {
		Assets []struct {
			URI      string  `json:"URI"`
			Duration float64 `json:"DURATION"`
		} `json:"ASSETS"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	if len(body.Assets) != 1 || body.Assets[0].URI != "/priv/ads/countdown_5s.m3u8" {
		t.Fatalf("unexpected assets payload: %+v", body.Assets)
	}
}
