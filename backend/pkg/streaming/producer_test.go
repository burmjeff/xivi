package streaming

import "testing"

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
		{name: "mpeg audio", parser: "mpegaudioparse", caps: "audio/mpeg, mpegversion=1", wantFirst: "mpg123audiodec", wantLength: 5},
		{name: "h264 passthrough", parser: "h264parse", caps: "video/x-h264"},
		{name: "aac passthrough", parser: "aacparse", caps: "audio/mpeg, mpegversion=4"},
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
