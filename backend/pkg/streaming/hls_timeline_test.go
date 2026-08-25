package streaming

import (
	"strings"
	"testing"
	"time"
)

func TestHLSTimelineKeepsSequenceMonotonicAcrossGenerations(t *testing.T) {
	timeline := hlsTimeline{}
	first := timeline.update(1, &HLSPlaylistSnapshot{Media: []HLSSegment{
		{Name: "segment.1.00001.ts", Duration: time.Second},
		{Name: "segment.1.00002.ts", Duration: time.Second},
	}}, 3)
	if strings.Contains(string(first.Content), "#EXT-X-DISCONTINUITY\n") {
		t.Fatal("first generation unexpectedly began with a discontinuity")
	}
	second := timeline.update(2, &HLSPlaylistSnapshot{Media: []HLSSegment{
		{Name: "segment.2.00000.ts", Duration: 2 * time.Second},
		{Name: "segment.2.00001.ts", Duration: time.Second},
	}}, 3)
	content := string(second.Content)
	if !strings.Contains(content, "#EXT-X-MEDIA-SEQUENCE:1\n") || !strings.Contains(content, "#EXT-X-DISCONTINUITY\n") {
		t.Fatalf("replacement playlist did not preserve continuity metadata:\n%s", content)
	}
	if len(second.Segments) != 3 || second.Segments[2] != "segment.2.00001.ts" {
		t.Fatalf("unexpected live window: %#v", second.Segments)
	}
}
