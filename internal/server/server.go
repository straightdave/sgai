package server

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// New constructs the Chi router with API routes, optional /priv static handler, and the embedded SPA handler.
func New(frontendFS, privFS fs.FS) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Route("/api", func(r chi.Router) {
		r.Get("/health", handleHealth)
		r.Get("/ads", handleAds)
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

type interstitialAsset struct {
	URI      string  `json:"URI"`
	Duration float64 `json:"DURATION"`
}

func handleAds(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string][]interstitialAsset{
		"ASSETS": {
			{
				URI:      "/priv/ads/countdown_5s.m3u8",
				Duration: 5.0,
			},
		},
	})
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
