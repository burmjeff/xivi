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

	mu          sync.Mutex
	lifecycle   sync.Mutex
	graphMu     sync.Mutex
	pipeline    *gst.Pipeline
	ready       chan struct{}
	hlsReady    chan struct{}
	errors      chan error
	readyOnce   sync.Once
	hlsOnce     sync.Once
	errorOnce   sync.Once
	stopOnce    sync.Once
	stopping    atomic.Bool
	lastDataNS  atomic.Int64
	probe       tsProbe
	busDone     chan struct{}
	playlist    string
	inputTSOnce sync.Once
	routeMu     sync.Mutex
	routed      map[string]bool
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
	setOptional(source, "ring-buffer-max-size", uint64(8*1024*1024))
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
	if err := p.addHLSOutput(tee); err != nil {
		p.disposePipeline()
		return err
	}

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
	if err := p.routePadToMux(pad, parserName, mux); err != nil {
		p.routeMu.Lock()
		delete(p.routed, media)
		p.routeMu.Unlock()
		return err
	}
	return nil
}

func (p *gstProducer) routePadToMux(pad *gst.Pad, parserName string, mux *gst.Element) error {
	parser, err := gst.NewElement(parserName)
	if err != nil {
		return fmt.Errorf("create %s: %w", parserName, err)
	}
	queue, err := gst.NewElement("queue")
	if err != nil {
		return fmt.Errorf("create adaptive track queue: %w", err)
	}
	_ = queue.Set("max-size-time", uint64(3*time.Second))
	_ = queue.Set("max-size-bytes", uint(0))
	_ = queue.Set("max-size-buffers", uint(0))
	p.pipeline.Add(parser)
	p.pipeline.Add(queue)
	if result := pad.Link(parser.GetStaticPad("sink")); result != gst.PadLinkOK {
		return fmt.Errorf("link adaptive track to %s: %s", parserName, result.String())
	}
	if err := parser.Link(queue); err != nil {
		return fmt.Errorf("link %s to adaptive queue: %w", parserName, err)
	}
	muxPad := mux.GetRequestPad("sink_%d")
	if muxPad == nil {
		return errors.New("request adaptive muxer input pad")
	}
	if result := queue.GetStaticPad("src").Link(muxPad); result != gst.PadLinkOK {
		return fmt.Errorf("link adaptive queue to MPEG-TS muxer: %s", result.String())
	}
	parser.SyncStateWithParent()
	queue.SyncStateWithParent()
	return nil
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
				p.readyOnce.Do(func() { close(p.ready) })
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

func (p *gstProducer) addHLSOutput(tee *gst.Element) error {
	queue, err := gst.NewElement("queue")
	if err != nil {
		return fmt.Errorf("create HLS transport queue: %w", err)
	}
	_ = queue.Set("max-size-time", uint64(5*time.Second))
	_ = queue.Set("max-size-bytes", uint(0))
	_ = queue.Set("max-size-buffers", uint(0))
	demux, err := gst.NewElement("tsdemux")
	if err != nil {
		return fmt.Errorf("create HLS transport demuxer: %w", err)
	}
	sink, err := gst.NewElement("hlssink2")
	if err != nil {
		return fmt.Errorf("create HLS segmenter: %w", err)
	}
	hlsDirectory := filepath.Join(p.config.StreamRoot, p.id)
	p.playlist = filepath.Join(hlsDirectory, "playlist.m3u8")
	if err := setRequired(sink, "playlist-location", p.playlist); err != nil {
		return err
	}
	if err := setRequired(sink, "location", filepath.Join(hlsDirectory, fmt.Sprintf("segment.%d.%%05d.ts", p.generation))); err != nil {
		return err
	}
	if err := setRequired(sink, "playlist-root", fmt.Sprintf("/stream/hls/%s", p.id)); err != nil {
		return err
	}
	if err := setRequired(sink, "target-duration", uint(p.config.HLSSegmentSeconds)); err != nil {
		return err
	}
	if err := setRequired(sink, "playlist-length", uint(p.config.HLSPlaylistLength)); err != nil {
		return err
	}
	if err := setRequired(sink, "max-files", uint(p.config.HLSPlaylistLength+3)); err != nil {
		return err
	}
	setOptional(sink, "send-keyframe-requests", true)

	p.pipeline.Add(queue)
	p.pipeline.Add(demux)
	p.pipeline.Add(sink)
	if err := queue.Link(demux); err != nil {
		return fmt.Errorf("link HLS transport demuxer: %w", err)
	}
	if err := tee.Link(queue); err != nil {
		return fmt.Errorf("link HLS shared output: %w", err)
	}

	var routeMu sync.Mutex
	routed := map[string]bool{}
	demux.Connect("pad-added", func(self *gst.Element, pad *gst.Pad) {
		if p.stopping.Load() {
			return
		}
		caps := currentOrQueriedCaps(pad)
		if caps == nil {
			return
		}
		parserName, media, ok := parserForCaps(caps.String())
		if !ok {
			log.Warn().Str("stream_id", p.id).Str("caps", caps.String()).Msg("HLS cannot use this media track without transcoding")
			return
		}
		routeMu.Lock()
		if routed[media] {
			routeMu.Unlock()
			return
		}
		routed[media] = true
		routeMu.Unlock()
		p.graphMu.Lock()
		defer p.graphMu.Unlock()
		if p.stopping.Load() {
			return
		}
		if err := p.routePadToHLSSink(pad, parserName, media, sink); err != nil {
			p.reportError(err)
		}
	})
	return nil
}

func (p *gstProducer) routePadToHLSSink(pad *gst.Pad, parserName, media string, sink *gst.Element) error {
	parser, err := gst.NewElement(parserName)
	if err != nil {
		return fmt.Errorf("create HLS %s: %w", parserName, err)
	}
	queue, err := gst.NewElement("queue")
	if err != nil {
		return fmt.Errorf("create HLS %s queue: %w", media, err)
	}
	_ = queue.Set("max-size-time", uint64(3*time.Second))
	_ = queue.Set("max-size-bytes", uint(0))
	_ = queue.Set("max-size-buffers", uint(0))
	p.pipeline.Add(parser)
	p.pipeline.Add(queue)
	if result := pad.Link(parser.GetStaticPad("sink")); result != gst.PadLinkOK {
		return fmt.Errorf("link HLS track to %s: %s", parserName, result.String())
	}
	if err := parser.Link(queue); err != nil {
		return fmt.Errorf("link HLS %s parser: %w", media, err)
	}
	sinkPad := sink.GetRequestPad(media)
	if sinkPad == nil {
		return fmt.Errorf("request HLS %s pad", media)
	}
	if result := queue.GetStaticPad("src").Link(sinkPad); result != gst.PadLinkOK {
		return fmt.Errorf("link HLS %s queue: %s", media, result.String())
	}
	parser.SyncStateWithParent()
	queue.SyncStateWithParent()
	return nil
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
		content, err := os.ReadFile(p.playlist)
		if err != nil {
			continue
		}
		text := string(content)
		if !strings.Contains(text, "#EXTM3U") || !strings.Contains(text, "#EXTINF") {
			continue
		}
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			segment := filepath.Join(filepath.Dir(p.playlist), filepath.Base(line))
			if info, err := os.Stat(segment); err == nil && info.Size() > 0 {
				p.hlsOnce.Do(func() {
					p.removeOldSegments()
					close(p.hlsReady)
				})
				return
			}
		}
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
	return nil
}

func (p *gstProducer) removeOldSegments() {
	directory := filepath.Join(p.config.StreamRoot, p.id)
	currentPrefix := fmt.Sprintf("segment.%d.", p.generation)
	segments, err := filepath.Glob(filepath.Join(directory, "segment.*.ts"))
	if err != nil {
		return
	}
	for _, segment := range segments {
		if !strings.HasPrefix(filepath.Base(segment), currentPrefix) {
			if err := os.Remove(segment); err != nil && !os.IsNotExist(err) {
				log.Debug().Err(err).Str("path", segment).Msg("Could not remove old HLS segment")
			}
		}
	}
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
