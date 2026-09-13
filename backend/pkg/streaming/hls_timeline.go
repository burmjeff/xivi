package streaming

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"time"
)

type timelineSegment struct {
	HLSSegment
	generation    uint64
	discontinuity bool
}

// hlsTimeline converts independently generated GStreamer playlists into one
// monotonic client-facing timeline. Upstream replacement can therefore reset
// timestamps and local sequence numbers without making HLS.js reuse media
// sequence numbers or append incompatible bytes to the previous generation.
type hlsTimeline struct {
	mu                    sync.Mutex
	segments              []timelineSegment
	seen                  map[string]bool
	mediaSequence         uint64
	discontinuitySequence uint64
	lastGeneration        uint64
}

func (timeline *hlsTimeline) update(generation uint64, snapshot *HLSPlaylistSnapshot, length int) *HLSPlaylistSnapshot {
	timeline.mu.Lock()
	defer timeline.mu.Unlock()
	if timeline.seen == nil {
		timeline.seen = make(map[string]bool)
	}
	for _, media := range snapshot.Media {
		if timeline.seen[media.Name] {
			continue
		}
		discontinuity := len(timeline.segments) > 0 && generation != timeline.lastGeneration
		timeline.segments = append(timeline.segments, timelineSegment{HLSSegment: media,
			generation: generation, discontinuity: discontinuity})
		timeline.seen[media.Name] = true
		timeline.lastGeneration = generation
	}
	if length < 3 {
		length = 3
	}
	for len(timeline.segments) > length {
		removed := timeline.segments[0]
		delete(timeline.seen, removed.Name)
		if removed.discontinuity {
			timeline.discontinuitySequence++
		}
		timeline.segments = timeline.segments[1:]
		timeline.mediaSequence++
	}
	return timeline.snapshotLocked()
}

func (timeline *hlsTimeline) current() (*HLSPlaylistSnapshot, error) {
	timeline.mu.Lock()
	defer timeline.mu.Unlock()
	if len(timeline.segments) == 0 {
		return nil, ErrHLSPlaylistNotReady
	}
	return timeline.snapshotLocked(), nil
}

func (timeline *hlsTimeline) snapshotLocked() *HLSPlaylistSnapshot {
	maximum := time.Second
	result := &HLSPlaylistSnapshot{}
	var content bytes.Buffer
	content.WriteString("#EXTM3U\n#EXT-X-VERSION:3\n")
	for _, segment := range timeline.segments {
		if segment.Duration > maximum {
			maximum = segment.Duration
		}
	}
	fmt.Fprintf(&content, "#EXT-X-TARGETDURATION:%d\n", int(math.Ceil(maximum.Seconds())))
	fmt.Fprintf(&content, "#EXT-X-MEDIA-SEQUENCE:%d\n", timeline.mediaSequence)
	fmt.Fprintf(&content, "#EXT-X-DISCONTINUITY-SEQUENCE:%d\n", timeline.discontinuitySequence)
	for _, segment := range timeline.segments {
		if segment.discontinuity {
			content.WriteString("#EXT-X-DISCONTINUITY\n")
		}
		fmt.Fprintf(&content, "#EXTINF:%.6f,\n%s\n", segment.Duration.Seconds(), segment.Name)
		result.Segments = append(result.Segments, segment.Name)
		result.Media = append(result.Media, segment.HLSSegment)
		result.Duration += segment.Duration
	}
	result.Content = content.Bytes()
	return result
}

func (s *Session) HLSPlaylistSnapshot() (*HLSPlaylistSnapshot, error) {
	s.mu.RLock()
	producer := s.current
	length := s.config.HLSPlaylistLength
	s.mu.RUnlock()
	if provider, ok := producer.(HLSProducer); ok {
		if snapshot, err := provider.HLSPlaylistSnapshot(); err == nil {
			s.recordHLSPlaylistResult(nil)
			return s.hls.update(provider.HLSGeneration(), snapshot, length), nil
		} else {
			s.recordHLSPlaylistResult(err)
			if current, currentErr := s.hls.current(); currentErr == nil {
				return current, nil
			}
			return nil, err
		}
	}
	return s.hls.current()
}

// recordHLSPlaylistResult reports only persistent manifest delays. Transient
// partial rewrites are expected while the sink atomically advances a live
// playlist and should not flood diagnostics.
func (s *Session) recordHLSPlaylistResult(result error) {
	now := time.Now()
	var severity, code, message string
	var details []byte
	s.mu.Lock()
	if result == nil {
		if !s.hlsIssueReportedAt.IsZero() {
			severity, code, message = "info", "hls_playlist_recovered", "HLS playlist publication recovered."
			details, _ = json.Marshal(struct {
				DelayedMS int64 `json:"delayed_ms"`
			}{DelayedMS: now.Sub(s.hlsIssueSince).Milliseconds()})
		}
		s.hlsIssueSince = time.Time{}
		s.hlsIssueReportedAt = time.Time{}
		s.hlsIssueLast = ""
	} else {
		if s.hlsIssueSince.IsZero() {
			s.hlsIssueSince = now
		}
		s.hlsIssueLast = SanitizeDiagnostic(result.Error())
		if now.Sub(s.hlsIssueSince) >= 2*time.Second &&
			(s.hlsIssueReportedAt.IsZero() || now.Sub(s.hlsIssueReportedAt) >= 30*time.Second) {
			severity, code = "warning", "hls_playlist_delayed"
			message = "HLS playlist publication is delayed; the last playable window is being retained."
			details, _ = json.Marshal(struct {
				DelayedMS int64  `json:"delayed_ms"`
				Reason    string `json:"reason"`
			}{DelayedMS: now.Sub(s.hlsIssueSince).Milliseconds(), Reason: s.hlsIssueLast})
			s.hlsIssueReportedAt = now
		}
	}
	s.mu.Unlock()
	if code != "" {
		s.emitEvent(severity, code, message, string(details))
	}
}
