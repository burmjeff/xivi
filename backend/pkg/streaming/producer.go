package streaming

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-gst/go-gst/gst"
	gstapp "github.com/go-gst/go-gst/gst/app"
	"github.com/rs/zerolog/log"
)

var gstInit sync.Once

type gstProducer struct {
	id         string
	source     string
	generation uint64
	config     Config
	hub        *Hub

	mu                   sync.Mutex
	lifecycle            sync.Mutex
	graphMu              sync.Mutex
	pipeline             *gst.Pipeline
	ready                chan struct{}
	hlsReady             chan struct{}
	errors               chan error
	readyOnce            sync.Once
	hlsOnce              sync.Once
	errorOnce            sync.Once
	stopOnce             sync.Once
	stopping             atomic.Bool
	lastDataNS           atomic.Int64
	probe                tsProbe
	busDone              chan struct{}
	playlist             string
	hlsSink              *gst.Element
	sourceBin            *gst.Element
	inputTSOnce          sync.Once
	routeMu              sync.Mutex
	routed               map[string]bool
	mediaMu              sync.RWMutex
	mediaTracks          []string
	compatibilityActions []string
}

func newGSTProducer(id, source string, generation uint64, config Config, hub *Hub) Producer {
	return &gstProducer{
		id:         id,
		source:     source,
		generation: generation,
		config:     config,
		hub:        hub,
		ready:      make(chan struct{}),
		hlsReady:   make(chan struct{}),
		errors:     make(chan error, 1),
		busDone:    make(chan struct{}),
		routed:     make(map[string]bool),
	}
}

func (p *gstProducer) Ready() <-chan struct{}    { return p.ready }
func (p *gstProducer) HLSReady() <-chan struct{} { return p.hlsReady }
func (p *gstProducer) Errors() <-chan error      { return p.errors }

func (p *gstProducer) LastDataAt() time.Time {
	nanoseconds := p.lastDataNS.Load()
	if nanoseconds == 0 {
		return time.Time{}
	}
	return time.Unix(0, nanoseconds)
}

func (p *gstProducer) MediaTracks() []string {
	p.mediaMu.RLock()
	defer p.mediaMu.RUnlock()
	return append([]string(nil), p.mediaTracks...)
}

func (p *gstProducer) HLSCompatibilityActions() []string {
	p.mediaMu.RLock()
	defer p.mediaMu.RUnlock()
	return append([]string(nil), p.compatibilityActions...)
}

func (p *gstProducer) HLSGeneration() uint64 { return p.generation }

func (p *gstProducer) HLSPlaylistSnapshot() (*HLSPlaylistSnapshot, error) {
	return ReadHLSPlaylist(p.playlist)
}

func (p *gstProducer) rememberMediaTrack(caps string) {
	p.mediaMu.Lock()
	defer p.mediaMu.Unlock()
	for _, current := range p.mediaTracks {
		if current == caps {
			return
		}
	}
	p.mediaTracks = append(p.mediaTracks, caps)
}

func (p *gstProducer) rememberCompatibilityAction(action string) {
	p.mediaMu.Lock()
	defer p.mediaMu.Unlock()
	for _, current := range p.compatibilityActions {
		if current == action {
			return
		}
	}
	p.compatibilityActions = append(p.compatibilityActions, action)
}

func (p *gstProducer) Start() error {
	p.lifecycle.Lock()
	defer p.lifecycle.Unlock()
	if p.source == "" {
		return errors.New("stream source is empty")
	}
	gstInit.Do(func() { gst.Init(nil) })
	if err := p.prepareHLSDirectory(); err != nil {
		return err
	}

	pipeline, err := gst.NewPipeline(fmt.Sprintf("stream-%s-%d", p.id, p.generation))
	if err != nil {
		return fmt.Errorf("create GStreamer pipeline: %w", err)
	}
	p.mu.Lock()
	p.pipeline = pipeline
	p.mu.Unlock()

	source, err := gst.NewElement("urisourcebin")
	if err != nil {
		p.disposePipeline()
		return fmt.Errorf("create URI source: %w", err)
	}
	mux, err := gst.NewElement("mpegtsmux")
	if err != nil {
		p.disposePipeline()
		return fmt.Errorf("create canonical MPEG-TS muxer: %w", err)
	}
	tee, err := gst.NewElement("tee")
	if err != nil {
		p.disposePipeline()
		return fmt.Errorf("create shared output tee: %w", err)
	}
	_ = tee.Set("name", "shared-transport")
	_ = tee.Set("allow-not-linked", false)

	if err := setRequired(source, "uri", p.source); err != nil {
		p.disposePipeline()
		return err
	}
	if err := setRequired(source, "parse-streams", true); err != nil {
		p.disposePipeline()
		return err
	}
	setOptional(source, "use-buffering", true)
	setOptional(source, "download", false)
	setOptional(source, "buffer-duration", int64(p.config.IngestBuffer))
	setOptional(source, "low-watermark", 0.02)
	setOptional(source, "high-watermark", 0.20)
	setOptional(source, "ring-buffer-max-size", uint64(8*1024*1024))
	p.sourceBin = source
	setOptional(mux, "alignment", 7)
	if _, err := source.Connect("source-setup", func(self *gst.Element, child *gst.Element) {
		setOptional(child, "user-agent", p.config.UserAgent)
		setOptional(child, "is-live", true)
		setOptional(child, "timeout", uint(max(int(p.config.StallTimeout/time.Second), 3)))
		setOptional(child, "retries", 1)
		setOptional(child, "ssl-strict", p.config.TLSVerify)
		setOptional(child, "keep-alive", true)
	}); err != nil {
		p.disposePipeline()
		return fmt.Errorf("watch URI source setup: %w", err)
	}

	pipeline.Add(source)
	pipeline.Add(mux)
	pipeline.Add(tee)
	if err := mux.Link(tee); err != nil {
		p.disposePipeline()
		return fmt.Errorf("link canonical muxer to shared outputs: %w", err)
	}
	if err := p.addTransportOutput(tee); err != nil {
		p.disposePipeline()
		return err
	}
	hlsSink, err := p.addHLSOutput()
	if err != nil {
		p.disposePipeline()
		return err
	}
	p.hlsSink = hlsSink

	if _, err := source.Connect("pad-added", func(self *gst.Element, pad *gst.Pad) {
		if p.stopping.Load() {
			return
		}
		if err := p.attachSourcePad(pad, mux); err != nil {
			p.reportError(err)
		}
	}); err != nil {
		p.disposePipeline()
		return fmt.Errorf("watch URI media tracks: %w", err)
	}

	if err := pipeline.SetState(gst.StatePlaying); err != nil {
		p.disposePipeline()
		return fmt.Errorf("start GStreamer pipeline: %w", err)
	}
	go p.busLoop(pipeline.GetPipelineBus())
	go p.stallLoop()
	go p.playlistLoop()
	return nil
}

func (p *gstProducer) attachSourcePad(pad *gst.Pad, mux *gst.Element) error {
	p.graphMu.Lock()
	locked := true
	defer func() {
		if locked {
			p.graphMu.Unlock()
		}
	}()
	if p.stopping.Load() {
		return nil
	}
	queue, err := gst.NewElement("queue")
	if err != nil {
		return fmt.Errorf("create source track queue: %w", err)
	}
	detector, err := gst.NewElement("typefind")
	if err != nil {
		return fmt.Errorf("create source track detector: %w", err)
	}
	_ = queue.Set("max-size-time", uint64(max(p.config.IngestBuffer, time.Second)))
	_ = queue.Set("max-size-bytes", uint(8*1024*1024))
	_ = queue.Set("max-size-buffers", uint(0))
	p.pipeline.Add(queue)
	p.pipeline.Add(detector)
	if result := pad.Link(queue.GetStaticPad("sink")); result != gst.PadLinkOK {
		return fmt.Errorf("link URI source track: %s", result.String())
	}
	if err := queue.Link(detector); err != nil {
		return fmt.Errorf("link source track detector: %w", err)
	}
	if _, err := detector.Connect("have-type", func(self *gst.Element, probability gst.TypeFindProbability, caps *gst.Caps) {
		if p.stopping.Load() || caps == nil {
			return
		}
		capsString := caps.String()
		log.Info().Str("stream_id", p.id).Str("caps", capsString).Msg("Discovered source media track")
		output := self.GetStaticPad("src")
		if strings.Contains(strings.ToLower(capsString), "video/mpegts") {
			p.inputTSOnce.Do(func() {
				if err := p.routeTransportPad(output, mux); err != nil {
					p.reportError(err)
				}
			})
			return
		}
		if err := p.routeElementaryPad(output, capsString, mux); err != nil {
			p.reportError(err)
		}
	}); err != nil {
		return fmt.Errorf("watch source track type: %w", err)
	}
	p.graphMu.Unlock()
	locked = false
	queue.SyncStateWithParent()
	detector.SyncStateWithParent()
	return nil
}

func (p *gstProducer) routeTransportPad(pad *gst.Pad, mux *gst.Element) error {
	p.graphMu.Lock()
	locked := true
	defer func() {
		if locked {
			p.graphMu.Unlock()
		}
	}()
	if p.stopping.Load() {
		return nil
	}
	queue, err := gst.NewElement("queue")
	if err != nil {
		return fmt.Errorf("create transport input queue: %w", err)
	}
	demux, err := gst.NewElement("tsdemux")
	if err != nil {
		return fmt.Errorf("create transport input demuxer: %w", err)
	}
	p.pipeline.Add(queue)
	p.pipeline.Add(demux)
	if result := pad.Link(queue.GetStaticPad("sink")); result != gst.PadLinkOK {
		return fmt.Errorf("link transport source queue: %s", result.String())
	}
	if err := queue.Link(demux); err != nil {
		return fmt.Errorf("link transport input demuxer: %w", err)
	}
	demux.Connect("pad-added", func(self *gst.Element, elementary *gst.Pad) {
		caps := currentOrQueriedCaps(elementary)
		if caps == nil || p.stopping.Load() {
			return
		}
		if err := p.routeElementaryPad(elementary, caps.String(), mux); err != nil {
			p.reportError(err)
		}
	})
	p.graphMu.Unlock()
	locked = false
	queue.SyncStateWithParent()
	demux.SyncStateWithParent()
	return nil
}

func (p *gstProducer) routeElementaryPad(pad *gst.Pad, capsString string, mux *gst.Element) error {
	p.rememberMediaTrack(capsString)
	p.graphMu.Lock()
	defer p.graphMu.Unlock()
	if p.stopping.Load() {
		return nil
	}
	parserName, media, ok := parserForCaps(capsString)
	if !ok {
		log.Warn().Str("stream_id", p.id).Str("caps", capsString).Msg("Ignoring unsupported source media track")
		return nil
	}
	p.routeMu.Lock()
	if p.routed[media] {
		p.routeMu.Unlock()
		return nil
	}
	p.routed[media] = true
	p.routeMu.Unlock()
	if err := p.routePadToMux(pad, parserName, capsString, media, mux); err != nil {
		p.routeMu.Lock()
		delete(p.routed, media)
		p.routeMu.Unlock()
		return err
	}
	return nil
}

func (p *gstProducer) routePadToMux(pad *gst.Pad, parserName, capsString, media string, mux *gst.Element) error {
	parser, err := gst.NewElement(parserName)
	if err != nil {
		return fmt.Errorf("create %s: %w", parserName, err)
	}
	configureStreamParser(parser, parserName)
	queue, err := gst.NewElement("queue")
	if err != nil {
		return fmt.Errorf("create adaptive track queue: %w", err)
	}
	_ = queue.Set("max-size-time", uint64(3*time.Second))
	_ = queue.Set("max-size-bytes", uint(0))
	_ = queue.Set("max-size-buffers", uint(0))
	tee, err := gst.NewElement("tee")
	if err != nil {
		return fmt.Errorf("create %s output tee: %w", media, err)
	}
	p.pipeline.Add(parser)
	p.pipeline.Add(tee)
	p.pipeline.Add(queue)
	if result := pad.Link(parser.GetStaticPad("sink")); result != gst.PadLinkOK {
		return fmt.Errorf("link adaptive track to %s: %s", parserName, result.String())
	}
	if err := parser.Link(tee); err != nil {
		return fmt.Errorf("link %s to shared track outputs: %w", parserName, err)
	}
	if err := tee.Link(queue); err != nil {
		return fmt.Errorf("link %s to transport queue: %w", parserName, err)
	}
	muxPad := mux.GetRequestPad("sink_%d")
	if muxPad == nil {
		return errors.New("request adaptive muxer input pad")
	}
	if result := queue.GetStaticPad("src").Link(muxPad); result != gst.PadLinkOK {
		return fmt.Errorf("link adaptive queue to MPEG-TS muxer: %s", result.String())
	}
	if p.hlsSink != nil {
		hlsQueue, queueErr := gst.NewElement("queue")
		if queueErr != nil {
			return fmt.Errorf("create %s HLS queue: %w", media, queueErr)
		}
		_ = hlsQueue.Set("max-size-time", uint64(10*time.Second))
		_ = hlsQueue.Set("max-size-bytes", uint(0))
		_ = hlsQueue.Set("max-size-buffers", uint(0))
		p.pipeline.Add(hlsQueue)
		if err := tee.Link(hlsQueue); err != nil {
			return fmt.Errorf("link %s to HLS queue: %w", media, err)
		}
		hlsTail := hlsQueue
		compatibility, action, compatibilityErr := p.hlsCompatibilityChain(parserName, capsString)
		if compatibilityErr != nil {
			return compatibilityErr
		}
		for _, element := range compatibility {
			p.pipeline.Add(element)
			if err := hlsTail.Link(element); err != nil {
				return fmt.Errorf("link %s HLS compatibility pipeline: %w", media, err)
			}
			hlsTail = element
		}
		hlsPad := p.hlsSink.GetRequestPad(media)
		if hlsPad == nil {
			return fmt.Errorf("request HLS %s input pad", media)
		}
		if result := hlsTail.GetStaticPad("src").Link(hlsPad); result != gst.PadLinkOK {
			return fmt.Errorf("link %s to HLS segmenter: %s", media, result.String())
		}
		hlsQueue.SyncStateWithParent()
		for _, element := range compatibility {
			element.SyncStateWithParent()
		}
		if action != "" {
			p.rememberCompatibilityAction(action)
		}
	}
	parser.SyncStateWithParent()
	tee.SyncStateWithParent()
	queue.SyncStateWithParent()
	return nil
}

// hlsCompatibilityChain preserves the original elementary stream for the
// canonical MPEG-TS branch, but normalizes codecs that browsers commonly
// reject in HLS. The resulting encoder is shared by every viewer of a channel.
func (p *gstProducer) hlsCompatibilityChain(parserName, capsString string) ([]*gst.Element, string, error) {
	if !p.config.HLSCompatibility {
		return nil, "", nil
	}
	names, action := hlsCompatibilityElements(parserName, capsString)
	if len(names) == 0 {
		return nil, "", nil
	}
	elements := make([]*gst.Element, 0, len(names))
	for _, name := range names {
		element, err := gst.NewElement(name)
		if err != nil {
			return nil, "", fmt.Errorf("create HLS compatibility element %s: %w", name, err)
		}
		switch name {
		case "x264enc":
			element.SetArg("tune", "zerolatency")
			element.SetArg("speed-preset", "veryfast")
			setOptional(element, "byte-stream", true)
			setOptional(element, "key-int-max", uint(60))
			setOptional(element, "bframes", uint(0))
		case "h264parse":
			configureStreamParser(element, name)
		case "voaacenc":
			setOptional(element, "bitrate", 128000)
		}
		elements = append(elements, element)
	}
	return elements, action, nil
}

func hlsCompatibilityElements(parserName, capsString string) ([]string, string) {
	lowerCaps := strings.ToLower(capsString)
	var names []string
	var action string
	switch {
	case parserName == "h265parse":
		names = []string{"libde265dec", "videoconvert", "x264enc", "h264parse"}
		action = "H.265 video is being normalized to low-latency H.264 for browser playback."
	case parserName == "mpegvideoparse":
		names = []string{"mpeg2dec", "videoconvert", "x264enc", "h264parse"}
		action = "MPEG video is being normalized to low-latency H.264 for browser playback."
	case parserName == "ac3parse" && !strings.Contains(lowerCaps, "eac3"):
		names = []string{"a52dec", "audioconvert", "audioresample", "voaacenc", "aacparse"}
		action = "AC-3 audio is being normalized to AAC for browser playback."
	case parserName == "mpegaudioparse":
		names = []string{"mpg123audiodec", "audioconvert", "audioresample", "voaacenc", "aacparse"}
		action = "MPEG audio is being normalized to AAC for browser playback."
	default:
		return nil, ""
	}
	return names, action
}

func (p *gstProducer) addTransportOutput(tee *gst.Element) error {
	queue, err := gst.NewElement("queue")
	if err != nil {
		return fmt.Errorf("create shared transport queue: %w", err)
	}
	_ = queue.Set("max-size-time", uint64(2*time.Second))
	_ = queue.Set("max-size-bytes", uint(0))
	_ = queue.Set("max-size-buffers", uint(0))
	_ = queue.Set("leaky", 2)

	sink, err := gstapp.NewAppSink()
	if err != nil {
		return fmt.Errorf("create shared transport sink: %w", err)
	}
	sink.SetMaxBuffers(8)
	sink.SetDrop(true)
	sink.SetWaitOnEOS(false)
	sink.SetCallbacks(&gstapp.SinkCallbacks{
		NewSampleFunc: func(appSink *gstapp.Sink) gst.FlowReturn {
			if p.stopping.Load() {
				return gst.FlowFlushing
			}
			sample := appSink.PullSample()
			if sample == nil {
				if appSink.IsEOS() {
					p.reportError(errors.New("source reached end of stream"))
					return gst.FlowEOS
				}
				return gst.FlowOK
			}
			buffer := sample.GetBuffer()
			if buffer == nil || buffer.GetSize() == 0 {
				return gst.FlowOK
			}
			data := buffer.Extract(0, buffer.GetSize())
			p.lastDataNS.Store(time.Now().UnixNano())
			if p.probe.Push(data) {
				p.readyOnce.Do(func() {
					close(p.ready)
					go p.enableSteadyBuffering()
				})
			}
			p.hub.Publish(data)
			return gst.FlowOK
		},
		EOSFunc: func(appSink *gstapp.Sink) {
			if !p.stopping.Load() {
				p.reportError(errors.New("source reached end of stream"))
			}
		},
	})

	p.pipeline.Add(queue)
	p.pipeline.Add(sink.Element)
	if err := queue.Link(sink.Element); err != nil {
		return fmt.Errorf("link shared transport sink: %w", err)
	}
	if err := tee.Link(queue); err != nil {
		return fmt.Errorf("link shared transport output: %w", err)
	}
	return nil
}

func (p *gstProducer) enableSteadyBuffering() {
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	select {
	case <-timer.C:
		if !p.stopping.Load() && p.sourceBin != nil {
			setOptional(p.sourceBin, "high-watermark", 0.60)
		}
	case <-p.busDone:
	}
}

func (p *gstProducer) addHLSOutput() (*gst.Element, error) {
	sink, err := gst.NewElement("hlssink2")
	if err != nil {
		return nil, fmt.Errorf("create HLS segmenter: %w", err)
	}
	hlsDirectory := filepath.Join(p.config.StreamRoot, p.id)
	p.playlist = filepath.Join(hlsDirectory, fmt.Sprintf("playlist.%d.m3u8", p.generation))
	if err := setRequired(sink, "playlist-location", p.playlist); err != nil {
		return nil, err
	}
	if err := setRequired(sink, "location", filepath.Join(hlsDirectory, fmt.Sprintf("segment.%d.%%05d.ts", p.generation))); err != nil {
		return nil, err
	}
	if err := setRequired(sink, "playlist-root", fmt.Sprintf("/stream/hls/%s", p.id)); err != nil {
		return nil, err
	}
	if err := setRequired(sink, "target-duration", uint(p.config.HLSSegmentSeconds)); err != nil {
		return nil, err
	}
	if err := setRequired(sink, "playlist-length", uint(p.config.HLSPlaylistLength)); err != nil {
		return nil, err
	}
	if err := setRequired(sink, "max-files", uint(p.config.HLSPlaylistLength+6)); err != nil {
		return nil, err
	}
	setOptional(sink, "send-keyframe-requests", true)
	p.pipeline.Add(sink)
	return sink, nil
}

// Repeating video parameter sets at each keyframe makes every HLS fragment
// independently decodable, including when a viewer joins after the producer
// has been running for a while.
func configureStreamParser(parser *gst.Element, parserName string) {
	if parserName != "h264parse" && parserName != "h265parse" {
		return
	}
	setOptional(parser, "config-interval", -1)
	setOptional(parser, "disable-passthrough", true)
}

func parserForCaps(caps string) (parser, media string, ok bool) {
	lower := strings.ToLower(caps)
	switch {
	case strings.Contains(lower, "video/x-h264"):
		return "h264parse", "video", true
	case strings.Contains(lower, "video/x-h265") || strings.Contains(lower, "video/x-hevc"):
		return "h265parse", "video", true
	case strings.Contains(lower, "video/mpeg"):
		return "mpegvideoparse", "video", true
	case strings.Contains(lower, "audio/x-eac3"):
		return "ac3parse", "audio", true
	case strings.Contains(lower, "audio/x-ac3"):
		return "ac3parse", "audio", true
	case strings.Contains(lower, "audio/mpeg") && (strings.Contains(lower, "mpegversion=(int)4") || strings.Contains(lower, "mpegversion=4")):
		return "aacparse", "audio", true
	case strings.Contains(lower, "audio/mpeg"):
		return "mpegaudioparse", "audio", true
	default:
		return "", "", false
	}
}

func currentOrQueriedCaps(pad *gst.Pad) *gst.Caps {
	if caps := pad.GetCurrentCaps(); caps != nil {
		return caps
	}
	return pad.QueryCaps(nil)
}

func (p *gstProducer) busLoop(bus *gst.Bus) {
	defer close(p.busDone)
	for !p.stopping.Load() {
		message := bus.TimedPop(gst.ClockTime(250 * time.Millisecond))
		if message == nil {
			continue
		}
		switch message.Type() {
		case gst.MessageError:
			parsed := message.ParseError()
			p.reportError(fmt.Errorf("GStreamer pipeline error: %w (%s)", parsed, parsed.DebugString()))
			return
		case gst.MessageEOS:
			p.reportError(errors.New("source reached end of stream"))
			return
		case gst.MessageClockLost:
			p.reportError(errors.New("pipeline clock was lost"))
			return
		}
	}
}

func (p *gstProducer) stallLoop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	ready := false
	for !p.stopping.Load() {
		select {
		case <-p.ready:
			ready = true
		case <-ticker.C:
			if ready {
				last := p.LastDataAt()
				if !last.IsZero() && time.Since(last) > p.config.StallTimeout {
					p.reportError(fmt.Errorf("source stalled for %s", time.Since(last).Round(time.Second)))
					return
				}
			}
		}
	}
}

func (p *gstProducer) playlistLoop() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for !p.stopping.Load() {
		<-ticker.C
		snapshot, err := ReadHLSPlaylist(p.playlist)
		// One validated, non-empty segment is enough to start. Waiting for three
		// makes startup depend on the upstream keyframe cadence and can turn a
		// healthy transport stream into a 30-second HLS timeout.
		if err != nil || len(snapshot.Segments) < 1 || snapshot.Duration <= 0 {
			continue
		}
		if err := snapshot.ValidateSegments(filepath.Dir(p.playlist)); err != nil {
			continue
		}
		p.hlsOnce.Do(func() { close(p.hlsReady) })
		return
	}
}

func (p *gstProducer) reportError(err error) {
	if err == nil || p.stopping.Load() {
		return
	}
	p.errorOnce.Do(func() {
		p.errors <- err
	})
}

func (p *gstProducer) prepareHLSDirectory() error {
	directory := filepath.Join(p.config.StreamRoot, p.id)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create HLS directory: %w", err)
	}
	if p.generation == 1 {
		entries, err := os.ReadDir(directory)
		if err != nil {
			return fmt.Errorf("inspect HLS directory: %w", err)
		}
		for _, entry := range entries {
			name := entry.Name()
			isPlaylist := strings.HasPrefix(name, "playlist.") && strings.HasSuffix(name, ".m3u8")
			if entry.IsDir() || (!isPlaylist && name != "playlist.m3u8" && !(strings.HasPrefix(name, "segment.") && strings.HasSuffix(name, ".ts"))) {
				continue
			}
			if err := os.Remove(filepath.Join(directory, name)); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove stale HLS asset %s: %w", name, err)
			}
		}
	}
	return nil
}

func (p *gstProducer) Stop() {
	p.stopOnce.Do(func() {
		p.stopping.Store(true)
		p.lifecycle.Lock()
		p.lifecycle.Unlock()
		p.graphMu.Lock()
		defer p.graphMu.Unlock()
		p.mu.Lock()
		pipeline := p.pipeline
		p.mu.Unlock()
		if pipeline == nil {
			return
		}
		if err := pipeline.SetState(gst.StateNull); err != nil {
			log.Warn().Err(err).Str("stream_id", p.id).Msg("Could not set pipeline to NULL during cleanup")
		}
		pipeline.GetState(gst.StateNull, gst.ClockTime(2*time.Second))
		select {
		case <-p.busDone:
		case <-time.After(time.Second):
		}
		p.disposePipeline()
	})
}

func (p *gstProducer) disposePipeline() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pipeline == nil {
		return
	}
	_ = p.pipeline.SetState(gst.StateNull)
	p.pipeline = nil
}

func setRequired(element *gst.Element, property string, value any) error {
	if err := element.Set(property, value); err != nil {
		return fmt.Errorf("set GStreamer property %s: %w", property, err)
	}
	return nil
}

func setOptional(element *gst.Element, property string, value any) {
	if err := element.Set(property, value); err != nil {
		log.Debug().Err(err).Str("property", property).Msg("Optional GStreamer property is unavailable")
	}
}
