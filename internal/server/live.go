package server

import (
	"bufio"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

const (
	liveMasterManifestPath = "big_buck_bunny/x36xhzz.m3u8"
	liveInitialLead        = 30 * time.Second
	liveWindowSize         = 6
	liveBreakIntervalSec   = 30.0
	liveTargetDuration     = 11
)

type liveSegment struct {
	Duration float64
	URI      string
}

type liveVariant struct {
	Key           string
	StreamInfLine string
	Segments      []liveSegment
	CumulOffsets  []float64
	TotalDuration float64
}

// LiveStream manages an on-the-fly looping HLS linear stream backed by Big Buck Bunny VOD segments.
type LiveStream struct {
	privFS fs.FS
	now    func() time.Time

	mu          sync.RWMutex
	running     bool
	startedAt   time.Time
	variants    map[string]*liveVariant
	variantKeys []string
}

// NewLiveStream creates a LiveStream controller using privFS for source playlists.
func NewLiveStream(privFS fs.FS) *LiveStream {
	return &LiveStream{
		privFS: privFS,
		now:    time.Now,
	}
}

// LiveStatus represents the current state of the linear stream generator.
type LiveStatus struct {
	Running        bool    `json:"running"`
	MasterURL      string  `json:"masterUrl,omitempty"`
	StartedAt      string  `json:"startedAt,omitempty"`
	ElapsedSeconds float64 `json:"elapsedSeconds,omitempty"`
}

func (ls *LiveStream) loadVariantsLocked() error {
	if len(ls.variants) > 0 {
		return nil
	}
	if ls.privFS == nil {
		return fmt.Errorf("media filesystem is not configured")
	}

	data, err := fs.ReadFile(ls.privFS, liveMasterManifestPath)
	if err != nil {
		return fmt.Errorf("read master manifest: %w", err)
	}

	baseDir := path.Dir(liveMasterManifestPath)
	variants := make(map[string]*liveVariant)
	var keys []string

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	var pendingStreamInf string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#EXT-X-STREAM-INF:") {
			pendingStreamInf = line
			continue
		}
		if strings.HasPrefix(line, "#") || pendingStreamInf == "" {
			continue
		}

		variantRel := path.Clean(path.Join(baseDir, line))
		key := path.Base(path.Dir(variantRel))
		if key == "" || key == "." {
			key = fmt.Sprintf("v%d", len(keys))
		}

		segments, cumul, totalDur, err := parseVariantSegments(ls.privFS, variantRel)
		if err != nil {
			return fmt.Errorf("parse variant %s: %w", key, err)
		}

		variants[key] = &liveVariant{
			Key:           key,
			StreamInfLine: pendingStreamInf,
			Segments:      segments,
			CumulOffsets:  cumul,
			TotalDuration: totalDur,
		}
		keys = append(keys, key)
		pendingStreamInf = ""
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan master manifest: %w", err)
	}
	if len(variants) == 0 {
		return fmt.Errorf("no variants found in %s", liveMasterManifestPath)
	}

	ls.variants = variants
	ls.variantKeys = keys
	return nil
}

func parseVariantSegments(fsys fs.FS, manifestPath string) ([]liveSegment, []float64, float64, error) {
	data, err := fs.ReadFile(fsys, manifestPath)
	if err != nil {
		return nil, nil, 0, err
	}

	var segments []liveSegment
	var cumul []float64
	total := 0.0
	pendingDur := -1.0

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#EXTINF:") {
			raw := strings.TrimPrefix(line, "#EXTINF:")
			if idx := strings.IndexByte(raw, ','); idx >= 0 {
				raw = raw[:idx]
			}
			dur, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
			if err == nil && dur > 0 {
				pendingDur = dur
			}
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		if pendingDur > 0 {
			cumul = append(cumul, total)
			segments = append(segments, liveSegment{
				Duration: pendingDur,
				URI:      line,
			})
			total += pendingDur
			pendingDur = -1.0
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, 0, err
	}
	if len(segments) == 0 {
		return nil, nil, 0, fmt.Errorf("no segments found in %s", manifestPath)
	}
	return segments, cumul, total, nil
}

// Start begins hosting the live linear stream. If already running, it returns the current status.
func (ls *LiveStream) Start() (LiveStatus, error) {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	if err := ls.loadVariantsLocked(); err != nil {
		return LiveStatus{}, err
	}
	if !ls.running {
		ls.running = true
		ls.startedAt = ls.now().UTC()
	}
	return ls.statusLocked(), nil
}

// Stop halts the live linear stream.
func (ls *LiveStream) Stop() LiveStatus {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	ls.running = false
	ls.startedAt = time.Time{}
	return ls.statusLocked()
}

// Status returns the current live stream status.
func (ls *LiveStream) Status() LiveStatus {
	ls.mu.RLock()
	defer ls.mu.RUnlock()

	return ls.statusLocked()
}

func (ls *LiveStream) statusLocked() LiveStatus {
	if !ls.running {
		return LiveStatus{Running: false}
	}
	elapsed := ls.now().UTC().Sub(ls.startedAt) + liveInitialLead
	return LiveStatus{
		Running:        true,
		MasterURL:      "/api/live/master.m3u8",
		StartedAt:      ls.startedAt.Format(time.RFC3339),
		ElapsedSeconds: elapsed.Seconds(),
	}
}

// MasterPlaylist renders the live master M3U8 playlist.
func (ls *LiveStream) MasterPlaylist() (string, bool) {
	ls.mu.RLock()
	defer ls.mu.RUnlock()

	if !ls.running || len(ls.variantKeys) == 0 {
		return "", false
	}

	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	for _, key := range ls.variantKeys {
		v := ls.variants[key]
		b.WriteString(v.StreamInfLine)
		b.WriteByte('\n')
		b.WriteString(key)
		b.WriteString("/playlist.m3u8\n")
	}
	return b.String(), true
}

// VariantPlaylist renders the sliding-window live media M3U8 playlist for the given variant key.
func (ls *LiveStream) VariantPlaylist(key string) (string, bool) {
	ls.mu.RLock()
	defer ls.mu.RUnlock()

	if !ls.running {
		return "", false
	}
	v, ok := ls.variants[key]
	if !ok {
		return "", false
	}

	n := len(v.Segments)
	elapsed := (ls.now().UTC().Sub(ls.startedAt) + liveInitialLead).Seconds()
	if elapsed < 0 {
		elapsed = 0
	}

	loopCount := int(elapsed / v.TotalDuration)
	rem := elapsed - float64(loopCount)*v.TotalDuration

	segIdx := n - 1
	for i := 0; i < n; i++ {
		nextOffset := v.TotalDuration
		if i+1 < n {
			nextOffset = v.CumulOffsets[i+1]
		}
		if rem < nextOffset {
			segIdx = i
			break
		}
	}

	headSeq := loopCount*n + segIdx
	firstSeq := headSeq - liveWindowSize + 1
	if firstSeq < 0 {
		firstSeq = 0
	}
	discSeq := firstSeq / n

	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	b.WriteString("#EXT-X-VERSION:3\n")
	fmt.Fprintf(&b, "#EXT-X-TARGETDURATION:%d\n", liveTargetDuration)
	fmt.Fprintf(&b, "#EXT-X-MEDIA-SEQUENCE:%d\n", firstSeq)
	fmt.Fprintf(&b, "#EXT-X-DISCONTINUITY-SEQUENCE:%d\n", discSeq)

	windowStart := segmentStartOffset(v, firstSeq)
	headLoop := headSeq / n
	headIdx := headSeq % n
	windowEnd := segmentStartOffset(v, headSeq) + v.Segments[headIdx].Duration
	_ = headLoop

	// Emit recurring interstitial DATERANGEs for breaks overlapping or just ahead of the live window.
	firstBreak := int(windowStart / liveBreakIntervalSec)
	if firstBreak < 1 {
		firstBreak = 1
	}
	lastBreak := int((windowEnd + 15.0) / liveBreakIntervalSec)
	for k := firstBreak; k <= lastBreak; k++ {
		breakOffset := float64(k) * liveBreakIntervalSec
		duration := 5.0
		playback := "block"
		if k%2 == 0 {
			duration = 10.0
			playback = "continue"
		}
		if breakOffset+duration < windowStart {
			continue
		}
		breakPDT := ls.startedAt.Add(time.Duration(breakOffset * float64(time.Second))).UTC()
		slotID := fmt.Sprintf("live-break-%d", k)
		assetList := fmt.Sprintf("/api/ads?slot=%s&duration=%.1f&playback=%s", slotID, duration, playback)
		fmt.Fprintf(
			&b,
			"#EXT-X-DATERANGE:ID=%q,CLASS=\"com.apple.hls.interstitial\",START-DATE=%q,DURATION=%.1f,X-ASSET-LIST=%q\n",
			slotID,
			breakPDT.Format("2006-01-02T15:04:05.000Z"),
			duration,
			assetList,
		)
	}

	for seq := firstSeq; seq <= headSeq; seq++ {
		idx := seq % n
		if seq > 0 && idx == 0 {
			b.WriteString("#EXT-X-DISCONTINUITY\n")
		}
		segOffset := segmentStartOffset(v, seq)
		segPDT := ls.startedAt.Add(time.Duration(segOffset * float64(time.Second))).UTC()
		fmt.Fprintf(&b, "#EXT-X-PROGRAM-DATE-TIME:%s\n", segPDT.Format("2006-01-02T15:04:05.000Z"))
		fmt.Fprintf(&b, "#EXTINF:%.3f,\n", v.Segments[idx].Duration)
		b.WriteString(v.Segments[idx].URI)
		b.WriteByte('\n')
	}

	return b.String(), true
}

func segmentStartOffset(v *liveVariant, seq int) float64 {
	n := len(v.Segments)
	loop := seq / n
	idx := seq % n
	return float64(loop)*v.TotalDuration + v.CumulOffsets[idx]
}

func (ls *LiveStream) handleStart(w http.ResponseWriter, r *http.Request) {
	status, err := ls.Start()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (ls *LiveStream) handleStop(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, ls.Stop())
}

func (ls *LiveStream) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, ls.Status())
}

func (ls *LiveStream) handleMasterPlaylist(w http.ResponseWriter, r *http.Request) {
	playlist, ok := ls.MasterPlaylist()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "live stream is not running"})
		return
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(playlist))
}

func (ls *LiveStream) handleVariantPlaylist(w http.ResponseWriter, r *http.Request) {
	variant := chi.URLParam(r, "variant")
	playlist, ok := ls.VariantPlaylist(variant)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "live variant not found or stream not running"})
		return
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(playlist))
}
