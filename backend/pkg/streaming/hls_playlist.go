package streaming

import (
	"bufio"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var ErrHLSPlaylistNotReady = errors.New("HLS playlist is not ready")

// HLSPlaylistSnapshot is one complete, validated read of a live media
// playlist. Keeping the bytes and parsed segment list together prevents a
// request from observing two different revisions while GStreamer rewrites it.
type HLSPlaylistSnapshot struct {
	Content  []byte
	Segments []string
	Media    []HLSSegment
	Duration time.Duration
}

type HLSSegment struct {
	Name     string
	Duration time.Duration
}

// hlsMediaExpectation is the stable track layout discovered by the canonical
// producer before HLS is exposed to clients. A live playlist must not begin
// with a temporary one-track fragment when the canonical stream contains both
// audio and video: HLS.js creates SourceBuffers from that first fragment and
// cannot append a track that appears later.
type hlsMediaExpectation struct {
	Video bool
	Audio bool
}

type hlsSegmentInspector func(string) (BootstrapStatus, error)

// ReadHLSPlaylist rejects partial rewrites and unsafe segment references. HLS
// playlists are tiny, so serving a validated in-memory snapshot is cheaper and
// safer than streaming a file that is being replaced continuously.
func ReadHLSPlaylist(path string) (*HLSPlaylistSnapshot, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrHLSPlaylistNotReady, err)
	}
	if len(content) == 0 || content[len(content)-1] != '\n' {
		return nil, fmt.Errorf("%w: manifest rewrite is incomplete", ErrHLSPlaylistNotReady)
	}

	snapshot := &HLSPlaylistSnapshot{Content: content}
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	lineNumber := 0
	headerFound := false
	pendingDuration := -1.0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if lineNumber == 1 {
			if line != "#EXTM3U" {
				return nil, fmt.Errorf("%w: missing EXTM3U header", ErrHLSPlaylistNotReady)
			}
			headerFound = true
			continue
		}
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#EXTINF:") {
			value := strings.TrimSuffix(strings.TrimPrefix(line, "#EXTINF:"), ",")
			if comma := strings.IndexByte(value, ','); comma >= 0 {
				value = value[:comma]
			}
			duration, parseErr := strconv.ParseFloat(value, 64)
			if parseErr != nil || duration <= 0 {
				return nil, fmt.Errorf("%w: invalid segment duration", ErrHLSPlaylistNotReady)
			}
			pendingDuration = duration
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		if pendingDuration < 0 {
			return nil, fmt.Errorf("%w: segment has no duration", ErrHLSPlaylistNotReady)
		}
		parsed, parseErr := url.Parse(line)
		if parseErr != nil {
			return nil, fmt.Errorf("%w: invalid segment URI", ErrHLSPlaylistNotReady)
		}
		segment := filepath.Base(parsed.Path)
		if segment == "." || segment == "" || !strings.HasSuffix(strings.ToLower(segment), ".ts") {
			return nil, fmt.Errorf("%w: unsafe segment URI", ErrHLSPlaylistNotReady)
		}
		snapshot.Segments = append(snapshot.Segments, segment)
		segmentDuration := time.Duration(pendingDuration * float64(time.Second))
		snapshot.Media = append(snapshot.Media, HLSSegment{Name: segment, Duration: segmentDuration})
		snapshot.Duration += segmentDuration
		pendingDuration = -1
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrHLSPlaylistNotReady, err)
	}
	if !headerFound || pendingDuration >= 0 || len(snapshot.Segments) == 0 {
		return nil, fmt.Errorf("%w: manifest has no complete segments", ErrHLSPlaylistNotReady)
	}
	return snapshot, nil
}

// ValidateSegments confirms that every advertised fragment is complete before
// the first player is allowed to use the playlist.
func (s *HLSPlaylistSnapshot) ValidateSegments(directory string) error {
	for _, segment := range s.Segments {
		info, err := os.Stat(filepath.Join(directory, segment))
		if err != nil || info.IsDir() || info.Size() == 0 {
			return fmt.Errorf("%w: segment %q is unavailable", ErrHLSPlaylistNotReady, segment)
		}
	}
	return nil
}

// decoderReadyStartupWindow returns a live window whose first fragment can
// initialize the producer's stable track layout. Leading fragments emitted
// while GStreamer's dynamic pads are still settling are deliberately omitted.
// Once that independently playable fragment has been found, later completed
// fragments are retained without requiring each one to repeat every codec
// bootstrap payload. Requiring that in steady state can freeze a valid live
// playlist whenever a short fragment contains no keyframe or audio payload.
//
// Content is intentionally not copied: the session timeline rebuilds a
// monotonic client-facing manifest from Media after this validation.
func (s *HLSPlaylistSnapshot) decoderReadyStartupWindow(
	directory string,
	expected hlsMediaExpectation,
	inspect hlsSegmentInspector,
) (*HLSPlaylistSnapshot, error) {
	if !expected.Video && !expected.Audio {
		return nil, fmt.Errorf("%w: expected media tracks are unknown", ErrHLSPlaylistNotReady)
	}
	if inspect == nil {
		inspect = inspectHLSSegment
	}
	start := -1
	latestName := ""
	latestMissing := ""
	for index, media := range s.Media {
		status, err := inspect(filepath.Join(directory, media.Name))
		if err != nil {
			latestName = media.Name
			latestMissing = "inspection failed: " + err.Error()
			continue
		}
		if !hlsSegmentMatches(status, expected) {
			latestName = media.Name
			latestMissing = strings.Join(hlsSegmentMissing(status, expected), ", ")
			continue
		}
		// Prefer the newest independently playable fragment. This excludes any
		// unstable startup layout that appeared after an earlier valid fragment,
		// while still retaining completed trailing media after the chosen start.
		start = index
	}
	if start < 0 {
		if latestName != "" && latestMissing != "" {
			return nil, fmt.Errorf("%w: no decoder-ready startup segment; latest %q is missing %s",
				ErrHLSPlaylistNotReady, latestName, latestMissing)
		}
		return nil, fmt.Errorf("%w: no decoder-ready startup segment", ErrHLSPlaylistNotReady)
	}
	result := &HLSPlaylistSnapshot{}
	for _, media := range s.Media[start:] {
		result.Media = append(result.Media, media)
		result.Segments = append(result.Segments, media.Name)
		result.Duration += media.Duration
	}
	return result, nil
}

func hlsSegmentMissing(status BootstrapStatus, expected hlsMediaExpectation) []string {
	missing := make([]string, 0, 8)
	if !status.Transport {
		missing = append(missing, "transport sync")
	}
	if !status.PAT {
		missing = append(missing, "PAT")
	}
	if !status.PMT {
		missing = append(missing, "PMT")
	}
	if expected.Video {
		if !status.HasVideo {
			missing = append(missing, "video track")
		}
		if status.FirstVideoPTS90K == nil {
			missing = append(missing, "video timestamp")
		}
		if !status.Complete {
			missing = append(missing, "decoder bootstrap")
		}
	}
	if expected.Audio {
		if !status.HasAudio {
			missing = append(missing, "audio track")
		}
		if status.FirstAudioPTS90K == nil {
			missing = append(missing, "audio timestamp")
		}
	}
	if len(missing) == 0 {
		missing = append(missing, "expected media layout")
	}
	return missing
}

func inspectHLSSegment(path string) (BootstrapStatus, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return BootstrapStatus{}, err
	}
	if len(content) == 0 {
		return BootstrapStatus{}, fmt.Errorf("segment is empty")
	}
	// A temporary Hub reuses the canonical MPEG-TS inspection contract: PAT,
	// PMT, PCR, codec initialization, random access, and first audio/video PTS.
	// Keep the complete fragment in its ring even when a segment is unusually
	// large so decoder-bootstrap validation can inspect it as one unit.
	hub := NewHub(max(2*len(content), 1024*1024))
	hub.PublishOwned(content)
	return hub.BootstrapStatus(), nil
}

func hlsSegmentMatches(status BootstrapStatus, expected hlsMediaExpectation) bool {
	if !status.Transport || !status.PAT || !status.PMT {
		return false
	}
	if expected.Video && (!status.HasVideo || status.FirstVideoPTS90K == nil || !status.Complete) {
		return false
	}
	if expected.Audio && (!status.HasAudio || status.FirstAudioPTS90K == nil) {
		return false
	}
	return true
}
