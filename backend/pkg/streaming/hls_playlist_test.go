package streaming

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadHLSPlaylistReturnsStableSnapshot(t *testing.T) {
	directory := t.TempDir()
	for _, name := range []string{"segment.1.00001.ts", "segment.1.00002.ts", "segment.1.00003.ts"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte("media"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	playlist := "#EXTM3U\n#EXT-X-TARGETDURATION:2\n#EXTINF:1.5,\n/stream/hls/channel/segment.1.00001.ts\n#EXTINF:2.0,\n/stream/hls/channel/segment.1.00002.ts\n#EXTINF:1.75,\n/stream/hls/channel/segment.1.00003.ts\n"
	path := filepath.Join(directory, "playlist.m3u8")
	if err := os.WriteFile(path, []byte(playlist), 0o600); err != nil {
		t.Fatal(err)
	}

	snapshot, err := ReadHLSPlaylist(path)
	if err != nil {
		t.Fatalf("read playlist: %v", err)
	}
	if len(snapshot.Segments) != 3 || snapshot.Duration != 5250*time.Millisecond {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
	if err := snapshot.ValidateSegments(directory); err != nil {
		t.Fatalf("validate advertised segments: %v", err)
	}
}

func TestReadHLSPlaylistRejectsPartialRewrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "playlist.m3u8")
	if err := os.WriteFile(path, []byte("#EXTM3U\n#EXTINF:2.0,\n/stream/hls/channel/segment.1.00001.ts"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadHLSPlaylist(path); !errors.Is(err, ErrHLSPlaylistNotReady) {
		t.Fatalf("expected partial playlist to be rejected, got %v", err)
	}
}

func TestHLSPlaylistValidationRejectsMissingSegment(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "playlist.m3u8")
	playlist := "#EXTM3U\n#EXTINF:2.0,\n/stream/hls/channel/missing.ts\n"
	if err := os.WriteFile(path, []byte(playlist), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := ReadHLSPlaylist(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := snapshot.ValidateSegments(directory); !errors.Is(err, ErrHLSPlaylistNotReady) {
		t.Fatalf("expected missing segment to be rejected, got %v", err)
	}
}

func TestDecoderReadyStartupWindowDropsLeadingAudioOnlySegmentFromAVStream(t *testing.T) {
	directory := t.TempDir()
	audioOnly := append(tsPacket(0, patSection()), tsPacket(0x0100, audioPMTSection())...)
	audioOnly = append(audioOnly, audioPESPacket(0x0101, 90_000)...)
	completeAV := append(tsPacket(0, patSection()), tsPacket(0x0100, audioVideoPMTSection())...)
	completeAV = append(completeAV, pcrPacket(0x0101)...)
	completeAV = append(completeAV, randomAccessPacket(0x0101)...)
	completeAV = append(completeAV, audioPESPacket(0x0102, 90_000)...)
	writeHLSSegment(t, directory, "audio-only.ts", audioOnly)
	writeHLSSegment(t, directory, "complete-av.ts", completeAV)

	snapshot := &HLSPlaylistSnapshot{Media: []HLSSegment{
		{Name: "audio-only.ts", Duration: 2 * time.Second},
		{Name: "complete-av.ts", Duration: 2 * time.Second},
	}}
	ready, err := snapshot.decoderReadyStartupWindow(directory, hlsMediaExpectation{Video: true, Audio: true}, nil)
	if err != nil {
		t.Fatalf("find decoder-ready startup window: %v", err)
	}
	if len(ready.Media) != 1 || ready.Media[0].Name != "complete-av.ts" {
		t.Fatalf("unexpected decoder-ready startup window: %#v", ready.Media)
	}
}

func TestDecoderReadyStartupWindowRequiresExpectedAudioPayload(t *testing.T) {
	directory := t.TempDir()
	videoOnly := append(tsPacket(0, patSection()), tsPacket(0x0100, audioVideoPMTSection())...)
	videoOnly = append(videoOnly, pcrPacket(0x0101)...)
	videoOnly = append(videoOnly, randomAccessPacket(0x0101)...)
	writeHLSSegment(t, directory, "missing-audio.ts", videoOnly)

	snapshot := &HLSPlaylistSnapshot{Media: []HLSSegment{{Name: "missing-audio.ts", Duration: 2 * time.Second}}}
	if _, err := snapshot.decoderReadyStartupWindow(directory, hlsMediaExpectation{Video: true, Audio: true}, nil); !errors.Is(err, ErrHLSPlaylistNotReady) {
		t.Fatalf("expected missing audio payload to delay HLS readiness, got %v", err)
	}
}

func TestDecoderReadyStartupWindowAcceptsGenuineAudioOnlyStream(t *testing.T) {
	directory := t.TempDir()
	audio := append(tsPacket(0, patSection()), tsPacket(0x0100, audioPMTSection())...)
	audio = append(audio, audioPESPacket(0x0101, 90_000)...)
	writeHLSSegment(t, directory, "audio.ts", audio)

	snapshot := &HLSPlaylistSnapshot{Media: []HLSSegment{{Name: "audio.ts", Duration: 2 * time.Second}}}
	ready, err := snapshot.decoderReadyStartupWindow(directory, hlsMediaExpectation{Audio: true}, nil)
	if err != nil || len(ready.Media) != 1 {
		t.Fatalf("genuine audio-only segment was rejected: ready=%#v err=%v", ready, err)
	}
}

func TestDecoderReadyStartupWindowAcceptsGenuineVideoOnlyStream(t *testing.T) {
	directory := t.TempDir()
	video := append(tsPacket(0, patSection()), tsPacket(0x0100, pmtSection())...)
	video = append(video, pcrPacket(0x0101)...)
	video = append(video, randomAccessPacket(0x0101)...)
	writeHLSSegment(t, directory, "video.ts", video)

	snapshot := &HLSPlaylistSnapshot{Media: []HLSSegment{{Name: "video.ts", Duration: 2 * time.Second}}}
	ready, err := snapshot.decoderReadyStartupWindow(directory, hlsMediaExpectation{Video: true}, nil)
	if err != nil || len(ready.Media) != 1 {
		t.Fatalf("genuine video-only segment was rejected: ready=%#v err=%v", ready, err)
	}
}

func TestDecoderReadyStartupWindowRetainsCompletedTrailingSegments(t *testing.T) {
	directory := t.TempDir()
	audioOnly := append(tsPacket(0, patSection()), tsPacket(0x0100, audioPMTSection())...)
	audioOnly = append(audioOnly, audioPESPacket(0x0101, 90_000)...)
	completeAV := append(tsPacket(0, patSection()), tsPacket(0x0100, audioVideoPMTSection())...)
	completeAV = append(completeAV, pcrPacket(0x0101)...)
	completeAV = append(completeAV, randomAccessPacket(0x0101)...)
	completeAV = append(completeAV, audioPESPacket(0x0102, 90_000)...)
	// A normal short steady-state fragment may advertise both tracks without
	// independently repeating a video keyframe and codec parameter sets.
	steadyState := append(tsPacket(0, patSection()), tsPacket(0x0100, audioVideoPMTSection())...)
	steadyState = append(steadyState, audioPESPacket(0x0102, 180_000)...)
	writeHLSSegment(t, directory, "startup-audio.ts", audioOnly)
	writeHLSSegment(t, directory, "decoder-ready.ts", completeAV)
	writeHLSSegment(t, directory, "steady-state.ts", steadyState)

	snapshot := &HLSPlaylistSnapshot{Media: []HLSSegment{
		{Name: "startup-audio.ts", Duration: time.Second},
		{Name: "decoder-ready.ts", Duration: time.Second},
		{Name: "steady-state.ts", Duration: time.Second},
	}}
	ready, err := snapshot.decoderReadyStartupWindow(directory, hlsMediaExpectation{Video: true, Audio: true}, nil)
	if err != nil {
		t.Fatalf("find decoder-ready startup window: %v", err)
	}
	if len(ready.Media) != 2 || ready.Media[0].Name != "decoder-ready.ts" || ready.Media[1].Name != "steady-state.ts" {
		t.Fatalf("trailing completed segment was withheld: %#v", ready.Media)
	}
}

func TestSteadyHLSWindowPublishesSegmentsWithoutTrackReinspection(t *testing.T) {
	producer := &gstProducer{hlsBootstrapReady: true, hlsSuppressed: map[string]struct{}{
		"startup-audio.ts": {},
	}}
	snapshot := &HLSPlaylistSnapshot{Media: []HLSSegment{
		{Name: "startup-audio.ts", Duration: time.Second},
		{Name: "decoder-ready.ts", Duration: time.Second},
		{Name: "no-keyframe.ts", Duration: time.Second},
	}}
	ready := producer.steadyHLSWindow(snapshot)
	if len(ready.Media) != 2 || ready.Media[0].Name != "decoder-ready.ts" || ready.Media[1].Name != "no-keyframe.ts" {
		t.Fatalf("steady-state publication was not monotonic: %#v", ready.Media)
	}

	ready = producer.steadyHLSWindow(&HLSPlaylistSnapshot{Media: []HLSSegment{
		{Name: "no-keyframe.ts", Duration: time.Second},
		{Name: "next.ts", Duration: time.Second},
	}})
	if len(ready.Media) != 2 || len(producer.hlsSuppressed) != 0 {
		t.Fatalf("expired startup suppression was retained: media=%#v suppressed=%#v", ready.Media, producer.hlsSuppressed)
	}
}

func TestHLSSegmentInspectionCacheIsBoundedToAdvertisedWindow(t *testing.T) {
	directory := t.TempDir()
	current := filepath.Join(directory, "current.ts")
	producer := &gstProducer{hlsMediaInspections: map[string]cachedHLSSegmentInspection{
		filepath.Join(directory, "expired.ts"): {},
		current:                                {},
	}}
	producer.pruneHLSSegmentInspections(directory, []string{"current.ts"})
	if len(producer.hlsMediaInspections) != 1 {
		t.Fatalf("inspection cache size = %d, want 1", len(producer.hlsMediaInspections))
	}
	if _, exists := producer.hlsMediaInspections[current]; !exists {
		t.Fatal("current advertised segment was pruned")
	}
}

func writeHLSSegment(t *testing.T, directory, name string, content []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(directory, name), content, 0o600); err != nil {
		t.Fatal(err)
	}
}
