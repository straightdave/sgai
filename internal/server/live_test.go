package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func testLivePrivFS() fstest.MapFS {
	return fstest.MapFS{
		"big_buck_bunny/x36xhzz.m3u8": {
			Data: []byte(`#EXTM3U
#EXT-X-STREAM-INF:PROGRAM-ID=1,BANDWIDTH=2149280,RESOLUTION=1280x720,NAME="720"
url_0/playlist.m3u8
`),
		},
		"big_buck_bunny/url_0/playlist.m3u8": {
			Data: []byte(`#EXTM3U
#EXT-X-VERSION:3
#EXT-X-PLAYLIST-TYPE:VOD
#EXT-X-TARGETDURATION:11
#EXTINF:10.000,
https://example.com/seg0.ts
#EXTINF:10.000,
https://example.com/seg1.ts
#EXTINF:10.000,
https://example.com/seg2.ts
#EXTINF:10.000,
https://example.com/seg3.ts
#EXT-X-ENDLIST
`),
		},
	}
}

func TestLiveStreamLifecycleAndLooping(t *testing.T) {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	ls := NewLiveStream(testLivePrivFS())
	ls.now = func() time.Time { return now }

	if ls.Status().Running {
		t.Fatal("expected live stream to be initially stopped")
	}
	if _, ok := ls.MasterPlaylist(); ok {
		t.Fatal("expected master playlist to be unavailable when stopped")
	}

	status, err := ls.Start()
	if err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}
	if !status.Running || status.MasterURL != "/api/live/master.m3u8" {
		t.Fatalf("unexpected start status: %+v", status)
	}

	master, ok := ls.MasterPlaylist()
	if !ok {
		t.Fatal("expected master playlist when running")
	}
	if !strings.Contains(master, "url_0/playlist.m3u8") {
		t.Fatalf("expected rewritten variant URI in master playlist, got:\n%s", master)
	}

	// At t=0 (+30s initial lead), headSeq should be 3 (seg3), window 0..3.
	pl, ok := ls.VariantPlaylist("url_0")
	if !ok {
		t.Fatal("expected variant playlist")
	}
	if strings.Contains(pl, "#EXT-X-ENDLIST") || strings.Contains(pl, "#EXT-X-PLAYLIST-TYPE:VOD") {
		t.Fatalf("live playlist must not contain VOD/ENDLIST tags:\n%s", pl)
	}
	if !strings.Contains(pl, "#EXT-X-MEDIA-SEQUENCE:0") {
		t.Fatalf("expected media sequence 0, got:\n%s", pl)
	}
	if !strings.Contains(pl, `CLASS="com.apple.hls.interstitial"`) {
		t.Fatalf("expected interstitial DATERANGE in live playlist, got:\n%s", pl)
	}
	if strings.Contains(pl, "X-RESUME-OFFSET") {
		t.Fatalf("live playlist should not contain X-RESUME-OFFSET:\n%s", pl)
	}

	// Advance clock by 25s -> elapsed = 55s -> loop 1, seg 1 (seq = 5).
	// Window should include seg 0 of loop 1 (seq 4) with #EXT-X-DISCONTINUITY.
	now = now.Add(25 * time.Second)
	pl2, ok := ls.VariantPlaylist("url_0")
	if !ok {
		t.Fatal("expected variant playlist after advance")
	}
	if !strings.Contains(pl2, "#EXT-X-DISCONTINUITY\n") {
		t.Fatalf("expected #EXT-X-DISCONTINUITY on loop boundary, got:\n%s", pl2)
	}

	// Advance clock further so loop boundary slides out of window (seq 4..9 -> firstSeq=4, discSeq=1).
	now = now.Add(40 * time.Second)
	pl3, ok := ls.VariantPlaylist("url_0")
	if !ok {
		t.Fatal("expected variant playlist on second loop")
	}
	if !strings.Contains(pl3, "#EXT-X-DISCONTINUITY-SEQUENCE:1") {
		t.Fatalf("expected discontinuity sequence 1, got:\n%s", pl3)
	}

	stopped := ls.Stop()
	if stopped.Running {
		t.Fatal("expected live stream to be stopped")
	}
	if _, ok := ls.VariantPlaylist("url_0"); ok {
		t.Fatal("expected variant playlist to be unavailable after stop")
	}
}

func TestLiveHTTPHandlers(t *testing.T) {
	h := New(nil, testLivePrivFS())

	// Initially stopped: master.m3u8 returns 404.
	req := httptest.NewRequest(http.MethodGet, "/api/live/master.m3u8", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 before start, got %d", rec.Code)
	}

	// Start live stream.
	req = httptest.NewRequest(http.MethodPost, "/api/live/start", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on start, got %d: %s", rec.Code, rec.Body.String())
	}
	var st LiveStatus
	if err := json.NewDecoder(rec.Body).Decode(&st); err != nil {
		t.Fatalf("decode start response: %v", err)
	}
	if !st.Running {
		t.Fatalf("expected running=true, got %+v", st)
	}

	// Master playlist should now be served with no-store cache header.
	req = httptest.NewRequest(http.MethodGet, "/api/live/master.m3u8", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on master.m3u8, got %d", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("expected Cache-Control: no-store, got %q", cc)
	}

	// Variant playlist should be served.
	req = httptest.NewRequest(http.MethodGet, "/api/live/url_0/playlist.m3u8", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on variant playlist, got %d", rec.Code)
	}

	// Stop live stream.
	req = httptest.NewRequest(http.MethodPost, "/api/live/stop", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on stop, got %d", rec.Code)
	}
}
