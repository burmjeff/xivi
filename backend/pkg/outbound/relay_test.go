package outbound

import (
	"bufio"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func testRelay(t *testing.T) *Relay {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	origin, _ := url.Parse("https://provider.example/live/master.m3u8?token=secret")
	return &Relay{
		origin: origin, listener: listener,
		resources: make(map[string]string), reverse: make(map[string]string),
	}
}

func TestRelayRewritesEveryHLSResourceToLoopback(t *testing.T) {
	relay := testRelay(t)
	manifest := `#EXTM3U
#EXT-X-KEY:METHOD=AES-128,URI="https://keys.example/key.bin?token=secret"
#EXT-X-MAP:URI="../init.mp4"
#EXTINF:1,
segment-1.ts?credential=secret
#EXT-X-I-FRAME-STREAM-INF:BANDWIDTH=1000,URI="file:///etc/passwd"
`
	rewritten, err := relay.rewriteManifest(relay.origin, []byte(manifest))
	if err != nil {
		t.Fatal(err)
	}
	value := string(rewritten)
	for _, forbidden := range []string{"provider.example", "keys.example", "token=secret", "credential=secret", "file:///"} {
		if strings.Contains(value, forbidden) {
			t.Fatalf("rewritten manifest retained %q:\n%s", forbidden, value)
		}
	}
	wantBase := "http://" + relay.listener.Addr().String()
	if strings.Count(value, wantBase) != 4 || !strings.Contains(value, wantBase+"/blocked") {
		t.Fatalf("manifest resources were not fully relayed:\n%s", value)
	}
}

func TestRelaySniffsExtensionlessHLSManifest(t *testing.T) {
	requestURL, _ := url.Parse("https://provider.example/live.php")
	response := &http.Response{Header: make(http.Header), Request: &http.Request{URL: requestURL}}
	body := bufio.NewReader(strings.NewReader("\ufeff  #EXTM3U\n#EXT-X-VERSION:3\n"))
	if !isHLSManifest(response, body) {
		t.Fatal("extensionless HLS manifest was not detected")
	}
}

func TestRelayRejectsLoopbackOrigin(t *testing.T) {
	if relay, err := StartRelay("http://127.0.0.1/private", Policy{AllowPrivate: true}); err == nil {
		_ = relay.Close()
		t.Fatal("loopback origin was accepted")
	}
}
