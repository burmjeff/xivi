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
	Duration time.Duration
}

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
		snapshot.Duration += time.Duration(pendingDuration * float64(time.Second))
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
