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
			Button   *struct {
				Label  string `json:"LABEL"`
				Action string `json:"ACTION"`
				URL    string `json:"URL"`
			} `json:"BUTTON"`
		} `json:"ASSETS"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	if len(body.Assets) != 1 || body.Assets[0].URI != "/priv/ads/countdown_5s.mp4" || body.Assets[0].Button != nil {
		t.Fatalf("unexpected assets payload: %+v", body.Assets)
	}

	t.Run("clickable param returns button metadata", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/ads?clickable=1", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
		var res struct {
			Assets []struct {
				URI    string `json:"URI"`
				Button *struct {
					Label  string `json:"LABEL"`
					Action string `json:"ACTION"`
					URL    string `json:"URL"`
				} `json:"BUTTON"`
			} `json:"ASSETS"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
			t.Fatalf("failed to decode JSON: %v", err)
		}
		if len(res.Assets) != 1 || res.Assets[0].Button == nil {
			t.Fatalf("expected button metadata, got %+v", res.Assets)
		}
		btn := res.Assets[0].Button
		if btn.Label == "" || btn.Action != "link" || btn.URL == "" {
			t.Fatalf("unexpected button metadata: %+v", btn)
		}
	})

	t.Run("continue playback without squeezeback capability falls back to standard ad", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/ads?slot=ad-break-40s&duration=10.0&playback=continue", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
		var res struct {
			Assets []struct {
				URI    string `json:"URI"`
				Layout string `json:"LAYOUT"`
			} `json:"ASSETS"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
			t.Fatalf("failed to decode JSON: %v", err)
		}
		if len(res.Assets) != 1 || res.Assets[0].URI != "/priv/ads/countdown_5s.mp4" || res.Assets[0].Layout != "" {
			t.Fatalf("expected fallback to standard ad, got %+v", res.Assets)
		}
	})

	t.Run("continue playback with squeezeback and clickable returns l-shape ad and request button", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/ads?slot=ad-break-40s&duration=10.0&playback=continue&squeezeback=1&clickable=1", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
		var sb struct {
			Assets []struct {
				URI      string  `json:"URI"`
				Duration float64 `json:"DURATION"`
				Layout   string  `json:"LAYOUT"`
				Button   *struct {
					Label  string `json:"LABEL"`
					Action string `json:"ACTION"`
					URL    string `json:"URL"`
				} `json:"BUTTON"`
			} `json:"ASSETS"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&sb); err != nil {
			t.Fatalf("failed to decode JSON: %v", err)
		}
		if len(sb.Assets) != 1 || sb.Assets[0].URI != "/priv/ads/squeezeback_l_10s.mp4" || sb.Assets[0].Layout != "l-squeezeback" {
			t.Fatalf("unexpected squeezeback payload: %+v", sb.Assets)
		}
		if sb.Assets[0].Button == nil || sb.Assets[0].Button.Action != "request" || sb.Assets[0].Button.URL != "/api/ads/click" {
			t.Fatalf("unexpected squeezeback button: %+v", sb.Assets[0].Button)
		}
	})

	t.Run("ads click endpoint returns ok", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/ads/click", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
	})

	t.Run("sid is propagated into request button URL and recognized on click", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/ads?slot=ad-break-40s&duration=10.0&playback=continue&squeezeback=1&clickable=1&sid=sess-abc-123", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
		var sb struct {
			Assets []struct {
				Button *struct {
					Action string `json:"ACTION"`
					URL    string `json:"URL"`
				} `json:"BUTTON"`
			} `json:"ASSETS"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&sb); err != nil {
			t.Fatalf("failed to decode JSON: %v", err)
		}
		if len(sb.Assets) != 1 || sb.Assets[0].Button == nil || sb.Assets[0].Button.URL != "/api/ads/click?sid=sess-abc-123" {
			t.Fatalf("expected click URL with sid, got %+v", sb.Assets)
		}

		clickReq := httptest.NewRequest(http.MethodPost, sb.Assets[0].Button.URL, nil)
		clickRec := httptest.NewRecorder()
		h.ServeHTTP(clickRec, clickReq)
		if clickRec.Code != http.StatusOK {
			t.Fatalf("expected status 200 on click, got %d", clickRec.Code)
		}
		var clickBody map[string]string
		if err := json.NewDecoder(clickRec.Body).Decode(&clickBody); err != nil {
			t.Fatalf("failed to decode click JSON: %v", err)
		}
		if clickBody["status"] != "ok" || clickBody["sid"] != "sess-abc-123" {
			t.Fatalf("unexpected click response: %+v", clickBody)
		}
	})
}
