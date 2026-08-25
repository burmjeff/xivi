package streaming

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGSTProducerSharesMPEGTSAndCreatesHLS(t *testing.T) {
	if _, err := exec.LookPath("gst-launch-1.0"); err != nil {
		t.Skip("gst-launch-1.0 is not installed")
	}
	temporary := t.TempDir()
	fixture := filepath.Join(temporary, "live.ts")
	command := exec.Command(
		"gst-launch-1.0", "-q",
		"videotestsrc", "num-buffers=90", "pattern=smpte",
		"!", "video/x-raw,format=I420,framerate=30/1,width=320,height=180",
		"!", "x264enc", "tune=zerolatency", "speed-preset=ultrafast", "key-int-max=30",
		"!", "h264parse", "config-interval=-1",
		"!", "mpegtsmux",
		"!", "filesink", "location="+fixture,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("generate MPEG-TS fixture: %v\n%s", err, output)
	}
	transport, err := os.ReadFile(fixture)
	if err != nil || len(transport) == 0 {
		t.Fatalf("read MPEG-TS fixture: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "video/mp2t")
		flusher, _ := writer.(http.Flusher)
		for {
			for offset := 0; offset < len(transport); offset += 7 * 188 {
				end := min(offset+7*188, len(transport))
				select {
				case <-request.Context().Done():
					return
				default:
				}
				if _, err := writer.Write(transport[offset:end]); err != nil {
					return
				}
				if flusher != nil {
					flusher.Flush()
				}
				time.Sleep(2 * time.Millisecond)
			}
		}
	}))
	defer server.Close()

	config := testConfig(filepath.Join(temporary, "hls"))
	config.StartupTimeout = 8 * time.Second
	config.StallTimeout = 5 * time.Second
	staleDirectory := filepath.Join(config.StreamRoot, "integration-channel")
	if err := os.MkdirAll(staleDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	staleSegment := filepath.Join(staleDirectory, "segment.1.99999.ts")
	if err := os.WriteFile(staleSegment, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	hub := NewHub(config.ClientBufferBytes)
	producer := newGSTProducer("integration-channel", server.URL, 1, config, hub)
	if err := producer.Start(); err != nil {
		t.Fatalf("start GStreamer producer: %v", err)
	}
	defer producer.Stop()

	context, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	select {
	case <-producer.Ready():
	case err := <-producer.Errors():
		t.Fatalf("producer failed before MPEG-TS readiness: %v", err)
	case <-context.Done():
		t.Fatal("producer did not emit valid PAT/PMT media")
	}

	subscription := hub.Subscribe()
	defer subscription.Close()
	received := make(chan bool, 1)
	go func() {
		chunk, ok := subscription.Next()
		received <- ok && len(chunk) > 0
	}()
	select {
	case ok := <-received:
		if !ok {
			t.Fatal("shared MPEG-TS subscriber received no media")
		}
	case <-context.Done():
		t.Fatal("shared MPEG-TS subscriber timed out")
	}

	select {
	case <-producer.HLSReady():
	case err := <-producer.Errors():
		t.Fatalf("producer failed before HLS readiness: %v", err)
	case <-context.Done():
		t.Fatal("producer did not create a playable HLS playlist and segment")
	}
	playlistPath := producer.(*gstProducer).playlist
	playlist, err := os.ReadFile(playlistPath)
	if err != nil {
		t.Fatalf("read generated HLS playlist: %v", err)
	}
	if !bytes.Contains(playlist, []byte("/stream/hls/integration-channel/segment.")) {
		t.Fatalf("playlist did not contain route-safe segment URLs:\n%s", playlist)
	}
	snapshot, err := ReadHLSPlaylist(playlistPath)
	if err != nil {
		t.Fatalf("read stable HLS playlist: %v", err)
	}
	if len(snapshot.Segments) < 1 || snapshot.Duration <= 0 {
		t.Fatalf("HLS became ready without a complete playable segment: %#v", snapshot)
	}
	if _, err := os.Stat(staleSegment); !os.IsNotExist(err) {
		t.Fatalf("stale HLS generation was not cleaned after the replacement became ready: %v", err)
	}

	producer.Stop()
	concrete := producer.(*gstProducer)
	concrete.mu.Lock()
	pipeline := concrete.pipeline
	concrete.mu.Unlock()
	if pipeline != nil {
		t.Fatal("GStreamer pipeline was retained after Stop")
	}
}

func TestGSTProducerRemuxesHLSIntoSharedOutputs(t *testing.T) {
	if _, err := exec.LookPath("gst-launch-1.0"); err != nil {
		t.Skip("gst-launch-1.0 is not installed")
	}
	temporary := t.TempDir()
	sourceDirectory := filepath.Join(temporary, "source")
	if err := os.MkdirAll(sourceDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(
		"gst-launch-1.0", "-q",
		"videotestsrc", "num-buffers=300", "pattern=ball",
		"!", "video/x-raw,framerate=30/1,width=320,height=180",
		"!", "x264enc", "tune=zerolatency", "speed-preset=ultrafast", "key-int-max=30",
		"!", "h264parse", "config-interval=-1",
		"!", "hlssink2", "target-duration=1", "playlist-length=0", "max-files=0",
		"location="+filepath.Join(sourceDirectory, "source.%05d.ts"),
		"playlist-location="+filepath.Join(sourceDirectory, "source.m3u8"),
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("generate HLS fixture: %v\n%s", err, output)
	}

	server := httptest.NewServer(http.FileServer(http.Dir(sourceDirectory)))
	defer server.Close()
	config := testConfig(filepath.Join(temporary, "output"))
	config.StartupTimeout = 12 * time.Second
	config.StallTimeout = 5 * time.Second
	hub := NewHub(config.ClientBufferBytes)
	producer := newGSTProducer("hls-integration-channel", server.URL+"/source.m3u8", 1, config, hub)
	if err := producer.Start(); err != nil {
		t.Fatalf("start HLS GStreamer producer: %v", err)
	}
	defer producer.Stop()

	timeout := time.NewTimer(15 * time.Second)
	defer timeout.Stop()
	select {
	case <-producer.Ready():
	case err := <-producer.Errors():
		t.Fatalf("HLS ingest failed before shared MPEG-TS readiness: %v", err)
	case <-timeout.C:
		t.Fatal("HLS ingest did not remux into valid MPEG-TS")
	}
	select {
	case <-producer.HLSReady():
	case err := <-producer.Errors():
		t.Fatalf("HLS ingest failed before shared HLS readiness: %v", err)
	case <-time.After(8 * time.Second):
		t.Fatal("remuxed HLS output did not become ready")
	}
}

func TestGSTProducerNormalizesH265ForBrowserHLS(t *testing.T) {
	if _, err := exec.LookPath("gst-launch-1.0"); err != nil {
		t.Skip("gst-launch-1.0 is not installed")
	}
	temporary := t.TempDir()
	fixture := filepath.Join(temporary, "h265.ts")
	command := exec.Command(
		"gst-launch-1.0", "-q",
		"videotestsrc", "num-buffers=90", "pattern=ball",
		"!", "video/x-raw,format=I420,framerate=30/1,width=320,height=180",
		"!", "x265enc", "speed-preset=ultrafast", "key-int-max=30",
		"!", "h265parse", "config-interval=-1",
		"!", "mpegtsmux",
		"!", "filesink", "location="+fixture,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("generate H.265 fixture: %v\n%s", err, output)
	}
	transport, err := os.ReadFile(fixture)
	if err != nil || len(transport) == 0 {
		t.Fatalf("read H.265 fixture: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "video/mp2t")
		flusher, _ := writer.(http.Flusher)
		for {
			select {
			case <-request.Context().Done():
				return
			default:
			}
			if _, err := writer.Write(transport); err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(25 * time.Millisecond)
		}
	}))
	defer server.Close()

	config := testConfig(filepath.Join(temporary, "output"))
	config.HLSCompatibility = true
	config.StartupTimeout = 12 * time.Second
	config.StallTimeout = 5 * time.Second
	producer := newGSTProducer("h265-compatibility", server.URL, 1, config, NewHub(config.ClientBufferBytes))
	if err := producer.Start(); err != nil {
		t.Fatalf("start H.265 producer: %v", err)
	}
	defer producer.Stop()
	timeout := time.NewTimer(15 * time.Second)
	defer timeout.Stop()
	select {
	case <-producer.HLSReady():
	case err := <-producer.Errors():
		t.Fatalf("H.265 compatibility pipeline failed: %v", err)
	case <-timeout.C:
		t.Fatal("H.265 compatibility pipeline did not create browser HLS")
	}
	actions := producer.(*gstProducer).HLSCompatibilityActions()
	if len(actions) != 1 || !strings.Contains(actions[0], "H.265") {
		t.Fatalf("H.265 normalization was not reported: %#v", actions)
	}
}
