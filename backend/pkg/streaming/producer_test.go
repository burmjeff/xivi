package streaming

import (
	"strings"
	"testing"
	"time"
)

func TestProducerHLSInputsWaitForCompleteTrackDiscovery(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name     string
		routed   map[string]bool
		expected map[string]bool
		changed  time.Time
		want     bool
	}{
		{name: "no tracks"},
		{name: "discovery still active", routed: map[string]bool{"video": true}, changed: now},
		{name: "audio not routed", routed: map[string]bool{"video": true}, expected: map[string]bool{"video": true, "audio": true}, changed: now.Add(-time.Second)},
		{name: "video only", routed: map[string]bool{"video": true}, changed: now.Add(-time.Second), want: true},
		{name: "audio only", routed: map[string]bool{"audio": true}, changed: now.Add(-time.Second), want: true},
		{name: "audio and video", routed: map[string]bool{"video": true, "audio": true}, expected: map[string]bool{"video": true, "audio": true}, changed: now.Add(-time.Second), want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			producer := &gstProducer{routed: test.routed, expectedMedia: test.expected, lastRouteChange: test.changed}
			if got := producer.startHLSInputs(now); got != test.want || producer.hlsInputsStarted != test.want {
				t.Fatalf("startHLSInputs = %v, sealed = %v; want %v", got, producer.hlsInputsStarted, test.want)
			}
		})
	}
}

func TestProducerRejectsLateHLSMediaTrack(t *testing.T) {
	producer := &gstProducer{hlsInputsStarted: true, routed: map[string]bool{"video": true}}
	// Nil native objects are deliberate: neither an extra pad nor a duplicate
	// track may touch the running native muxer after its layout is sealed.
	if err := producer.routeElementaryPad(nil, "video/x-h264", nil); err != nil {
		t.Fatalf("duplicate video track should be ignored: %v", err)
	}
	err := producer.routeElementaryPad(nil, "audio/mpeg, mpegversion=(int)4", nil)
	if err == nil || !strings.Contains(err.Error(), "restart required") {
		t.Fatalf("late audio did not request a new producer: %v", err)
	}
	if producer.routed["audio"] {
		t.Fatal("late audio changed the running muxer layout")
	}
}

func TestProducerHLSInputsDoNotStartDuringStop(t *testing.T) {
	producer := &gstProducer{routed: map[string]bool{"video": true}, lastRouteChange: time.Now().Add(-time.Second)}
	producer.stopping.Store(true)
	if !producer.startHLSInputs(time.Now()) || producer.hlsInputsStarted {
		t.Fatal("stopping producer must exit discovery without unblocking HLS inputs")
	}
}

func TestProducerStallLoopExitsWhenBusStops(t *testing.T) {
	producer := &gstProducer{ready: make(chan struct{}), busDone: make(chan struct{})}
	close(producer.ready)
	done := make(chan struct{})
	go func() {
		producer.stallLoop()
		close(done)
	}()
	close(producer.busDone)
	select {
	case <-done:
	case <-time.After(time.Second):
		producer.stopping.Store(true)
		t.Fatal("stall monitor did not exit when the bus stopped")
	}
}

func TestProducerReadinessRequiresEveryRoutedTrack(t *testing.T) {
	hub := NewHub(1024 * 1024)
	producer := &gstProducer{
		hub:             hub,
		routed:          map[string]bool{"video": true, "audio": true},
		lastRouteChange: time.Now().Add(-trackDiscoverySettle),
	}
	hub.Publish(append(tsPacket(0, patSection()), tsPacket(0x0100, pmtSection())...))
	hub.Publish(pcrPacket(0x0101))
	hub.Publish(randomAccessPacket(0x0101))
	if producer.decoderReady(time.Now()) {
		t.Fatal("producer became ready while the canonical PMT omitted routed audio")
	}
	hub.Publish(tsPacket(0x0100, audioVideoPMTSection()))
	hub.Publish(audioPESPacket(0x0102, 90_000))
	if producer.decoderReady(time.Now()) {
		t.Fatal("producer reused a video bootstrap from before the complete PMT")
	}
	hub.Publish(pcrPacket(0x0101))
	hub.Publish(randomAccessPacket(0x0101))
	if !producer.decoderReady(time.Now()) {
		t.Fatal("producer did not become ready with synchronized audio/video bootstrap data")
	}
}

func TestParserForCapsRoutesOnlyNegotiatedMPEGAudioVersions(t *testing.T) {
	tests := []struct {
		name       string
		caps       string
		wantParser string
		wantOK     bool
	}{
		{
			name:       "MPEG-4 AAC",
			caps:       "audio/mpeg, mpegversion=(int)4, stream-format=(string)adts",
			wantParser: "aacparse",
			wantOK:     true,
		},
		{
			name:       "MPEG-2 AAC",
			caps:       "audio/mpeg, mpegversion=(int)2, stream-format=(string)adts",
			wantParser: "aacparse",
			wantOK:     true,
		},
		{
			name:       "MPEG audio layers",
			caps:       "audio/mpeg, mpegversion=(int)1",
			wantParser: "mpegaudioparse",
			wantOK:     true,
		},
		{
			name:   "unnegotiated AAC version list",
			caps:   "audio/mpeg, mpegversion=(int){ 2, 4 }, stream-format=(string)adts",
			wantOK: false,
		},
		{
			name:   "unnegotiated version range",
			caps:   "audio/mpeg, mpegversion=(int)[ 1, 4 ]",
			wantOK: false,
		},
		{
			name:   "missing MPEG version",
			caps:   "audio/mpeg",
			wantOK: false,
		},
		{
			name:   "unknown MPEG version",
			caps:   "audio/mpeg, mpegversion=(int)3",
			wantOK: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parser, media, ok := parserForCaps(test.caps)
			if ok != test.wantOK || parser != test.wantParser {
				t.Fatalf("parserForCaps(%q) = parser %q, media %q, ok %v; want parser %q, ok %v", test.caps, parser, media, ok, test.wantParser, test.wantOK)
			}
			if ok && media != "audio" {
				t.Fatalf("parserForCaps(%q) media = %q, want audio", test.caps, media)
			}
		})
	}
}

func TestCapsIntegerFieldRejectsNonScalarValues(t *testing.T) {
	tests := []struct {
		caps  string
		value int
		ok    bool
	}{
		{caps: "audio/mpeg, mpegversion=4", value: 4, ok: true},
		{caps: "audio/mpeg, mpegversion=(int) 2", value: 2, ok: true},
		{caps: "audio/mpeg, mpegversion=(int){ 2, 4 }"},
		{caps: "audio/mpeg, mpegversion=(int)[ 1, 4 ]"},
		{caps: "audio/mpeg"},
	}
	for _, test := range tests {
		value, ok := capsIntegerField(test.caps, "mpegversion")
		if value != test.value || ok != test.ok {
			t.Fatalf("capsIntegerField(%q) = %d, %v; want %d, %v", test.caps, value, ok, test.value, test.ok)
		}
	}
}

func TestHLSCompatibilityElementsNormalizeOnlyBrowserIncompatibleCodecs(t *testing.T) {
	tests := []struct {
		name       string
		parser     string
		caps       string
		wantFirst  string
		wantLength int
	}{
		{name: "h265", parser: "h265parse", caps: "video/x-h265", wantFirst: "libde265dec", wantLength: 4},
		{name: "mpeg2", parser: "mpegvideoparse", caps: "video/mpeg, mpegversion=2", wantFirst: "mpeg2dec", wantLength: 4},
		{name: "ac3", parser: "ac3parse", caps: "audio/x-ac3", wantFirst: "a52dec", wantLength: 5},
		{name: "aac main", parser: "aacparse", caps: "audio/mpeg, mpegversion=(int)4, profile=(string)main", wantFirst: "faad", wantLength: 5},
		{name: "aac main base profile", parser: "aacparse", caps: "audio/mpeg, mpegversion=4, base-profile=main", wantFirst: "faad", wantLength: 5},
		{name: "mpeg audio", parser: "mpegaudioparse", caps: "audio/mpeg, mpegversion=1", wantFirst: "mpg123audiodec", wantLength: 5},
		{name: "h264 passthrough", parser: "h264parse", caps: "video/x-h264"},
		{name: "aac lc passthrough", parser: "aacparse", caps: "audio/mpeg, mpegversion=4, profile=lc"},
		{name: "aac unknown passthrough", parser: "aacparse", caps: "audio/mpeg, mpegversion=4"},
		{name: "eac3 remains visible for diagnostics", parser: "ac3parse", caps: "audio/x-eac3"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			elements, action := hlsCompatibilityElements(test.parser, test.caps)
			if len(elements) != test.wantLength {
				t.Fatalf("got %d compatibility elements, want %d: %#v", len(elements), test.wantLength, elements)
			}
			if test.wantLength == 0 {
				if action != "" {
					t.Fatalf("passthrough codec reported an action: %q", action)
				}
				return
			}
			if elements[0] != test.wantFirst || action == "" {
				t.Fatalf("unexpected compatibility plan: elements=%#v action=%q", elements, action)
			}
		})
	}
}
