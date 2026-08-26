package streaming

import "testing"

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
