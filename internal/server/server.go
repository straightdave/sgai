package server

import (
	"context"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type sessionIDContextKey struct{}

// SessionIDFromContext returns the playback session ID extracted from the request, if any.
func SessionIDFromContext(ctx context.Context) string {
	sid, _ := ctx.Value(sessionIDContextKey{}).(string)
	return sid
}

func sessionIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sid := strings.TrimSpace(r.URL.Query().Get("sid"))
		if sid == "" {
			sid = strings.TrimSpace(r.Header.Get("X-Session-ID"))
		}
		if sid != "" {
			log.Printf("session=%s %s %s", sid, r.Method, r.URL.Path)
			r = r.WithContext(context.WithValue(r.Context(), sessionIDContextKey{}, sid))
		}
		next.ServeHTTP(w, r)
	})
}

// New constructs the Chi router with API routes, optional /priv static handler, and the embedded SPA handler.
func New(frontendFS, privFS fs.FS) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(sessionIDMiddleware)

	live := NewLiveStream(privFS)

	r.Route("/api", func(r chi.Router) {
		r.Get("/health", handleHealth)
		r.Get("/ads", handleAds)
		r.Post("/ads/click", handleAdClick)
		r.Route("/live", func(r chi.Router) {
			r.Post("/start", live.handleStart)
			r.Post("/stop", live.handleStop)
			r.Get("/status", live.handleStatus)
			r.Get("/master.m3u8", live.handleMasterPlaylist)
			r.Get("/{variant}/playlist.m3u8", live.handleVariantPlaylist)
		})
		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		})
	})

	if privFS != nil {
		r.Handle("/priv/*", privHandler(privFS))
	}

	if frontendFS != nil {
		r.NotFound(spaHandler(frontendFS))
	}

	return r
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type interstitialButton struct {
	Label  string `json:"LABEL"`
	Action string `json:"ACTION"`
	URL    string `json:"URL"`
}

type interstitialAsset struct {
	URI      string              `json:"URI"`
	Duration float64             `json:"DURATION"`
	Layout   string              `json:"LAYOUT,omitempty"`
	Button   *interstitialButton `json:"BUTTON,omitempty"`
}

func handleAds(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	sid := SessionIDFromContext(r.Context())

	// Get Player capability
	clickable := q.Get("clickable") == "1" || q.Get("clickable") == "true"
	supportsSqueezeback := q.Get("squeezeback") == "1" || q.Get("squeezeback") == "true"

	// Get Slot traits
	playback := q.Get("playback")

	// == Mimic remote ads bidding and decisioning ==
	if playback == "continue" && supportsSqueezeback {
		asset := interstitialAsset{
			URI:      "/priv/ads/squeezeback_l_10s.mp4",
			Duration: 10.0,
			Layout:   "l-squeezeback",
		}
		if clickable {
			clickURL := "/api/ads/click"
			if sid != "" {
				clickURL += "?sid=" + url.QueryEscape(sid)
			}
			asset.Button = &interstitialButton{
				Label:  "Claim Offer",
				Action: "request",
				URL:    clickURL,
			}
		}
		writeJSON(w, http.StatusOK, map[string][]interstitialAsset{
			"ASSETS": {asset},
		})
		return
	}

	// == prepare ASSET-LIST response ==
	asset := interstitialAsset{
		URI:      "/priv/ads/countdown_5s.mp4",
		Duration: 5.0,
	}
	if clickable {
		asset.Button = &interstitialButton{
			Label:  "Learn More",
			Action: "link",
			URL:    "https://example.com",
		}
	}
	writeJSON(w, http.StatusOK, map[string][]interstitialAsset{
		"ASSETS": {asset},
	})
}

func handleAdClick(w http.ResponseWriter, r *http.Request) {
	resp := map[string]string{"status": "ok"}
	if sid := SessionIDFromContext(r.Context()); sid != "" {
		resp["sid"] = sid
	}
	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func privHandler(privFS fs.FS) http.Handler {
	fileServer := http.StripPrefix("/priv/", http.FileServer(http.FS(privFS)))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}

		relPath := strings.TrimPrefix(path.Clean(r.URL.Path), "/priv/")
		if relPath == "" || relPath == "." || strings.HasPrefix(relPath, "..") {
			http.NotFound(w, r)
			return
		}

		if info, err := fs.Stat(privFS, relPath); err == nil && !info.IsDir() {
			if strings.HasSuffix(relPath, ".m3u8") {
				w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			}
			fileServer.ServeHTTP(w, r)
			return
		}

		http.NotFound(w, r)
	})
}

func spaHandler(frontendFS fs.FS) http.HandlerFunc {
	fileServer := http.FileServer(http.FS(frontendFS))

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}

		cleanPath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if cleanPath == "" || cleanPath == "." {
			serveIndex(w, frontendFS)
			return
		}

		if info, err := fs.Stat(frontendFS, cleanPath); err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}

		serveIndex(w, frontendFS)
	}
}

func serveIndex(w http.ResponseWriter, frontendFS fs.FS) {
	data, err := fs.ReadFile(frontendFS, "index.html")
	if err != nil {
		http.Error(w, "frontend index not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
