package streaming

import (
	"errors"
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

func TestPersistentHLSPlaylistDelayIsRecordedAndRecovers(t *testing.T) {
	var events []EventRecord
	session := &Session{
		incidentID:    "incident-1",
		id:            "channel-1",
		hlsIssueSince: time.Now().Add(-3 * time.Second),
		observer: func(observation Observation) {
			if observation.Event != nil {
				events = append(events, *observation.Event)
			}
		},
	}
	session.recordHLSPlaylistResult(errors.New("HLS segment no-keyframe.ts is missing decoder bootstrap"))
	if len(events) != 1 || events[0].Code != "hls_playlist_delayed" || !strings.Contains(events[0].Details, "decoder bootstrap") {
		t.Fatalf("persistent delay event was not recorded with its reason: %#v", events)
	}

	session.recordHLSPlaylistResult(nil)
	if len(events) != 2 || events[1].Code != "hls_playlist_recovered" {
		t.Fatalf("playlist recovery event was not recorded: %#v", events)
	}
}
