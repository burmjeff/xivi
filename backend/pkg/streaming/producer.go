package streaming

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-gst/go-gst/gst"
	gstapp "github.com/go-gst/go-gst/gst/app"
	"github.com/rs/zerolog/log"
)

var gstInit sync.Once

// Dynamic demuxers may announce audio and video a few callbacks apart. Give
// track discovery a short quiet window before declaring the canonical mux
// ready so its initial decoder bootstrap cannot expose a one-track PMT.
const trackDiscoverySettle = 250 * time.Millisecond

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
	expectedMedia        map[string]bool
	lastRouteChange      time.Time
	mediaMu              sync.RWMutex
	mediaTracks          []string
	compatibilityActions []string
	hlsStartupMu         sync.Mutex
	hlsBootstrapReady    bool
	hlsSuppressed        map[string]struct{}
	hlsMediaMu           sync.Mutex
	hlsMediaInspections  map[string]cachedHLSSegmentInspection
}

type cachedHLSSegmentInspection struct {
	size       int64
	modifiedNS int64
	status     BootstrapStatus
}

func newGSTProducer(id, source string, generation uint64, config Config, hub *Hub) Producer {
	return &gstProducer{
		id:            id,
		source:        source,
		generation:    generation,
		config:        config,
		hub:           hub,
		ready:         make(chan struct{}),
		hlsReady:      make(chan struct{}),
		errors:        make(chan error, 1),
		busDone:       make(chan struct{}),
		routed:        make(map[string]bool),
		expectedMedia: make(map[string]bool),
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
	select {
	case <-p.ready:
	default:
		return nil, fmt.Errorf("%w: canonical media layout is not ready", ErrHLSPlaylistNotReady)
	}
	snapshot, err := ReadHLSPlaylist(p.playlist)
	if err != nil {
		return nil, err
	}
	directory := filepath.Dir(p.playlist)
	if err := snapshot.ValidateSegments(directory); err != nil {
		return nil, err
	}

	p.hlsStartupMu.Lock()
	defer p.hlsStartupMu.Unlock()
	if p.hlsBootstrapReady {
		steady := p.steadyHLSWindow(snapshot)
		if len(steady.Media) == 0 {
			return nil, fmt.Errorf("%w: no completed segment remains after startup filtering", ErrHLSPlaylistNotReady)
		}
		return steady, nil
	}

	p.pruneHLSSegmentInspections(directory, snapshot.Segments)
	status := p.hub.BootstrapStatus()
	ready, err := snapshot.decoderReadyStartupWindow(directory, hlsMediaExpectation{
		Video: status.HasVideo,
		Audio: status.HasAudio,
	}, p.inspectHLSSegment)
	if err != nil {
		return nil, err
	}

	p.hlsSuppressed = make(map[string]struct{})
	readyNames := make(map[string]struct{}, len(ready.Media))
	for _, media := range ready.Media {
		readyNames[media.Name] = struct{}{}
	}
	for _, media := range snapshot.Media {
		if _, retained := readyNames[media.Name]; !retained {
			p.hlsSuppressed[media.Name] = struct{}{}
		}
	}
	p.hlsBootstrapReady = true
	p.clearHLSSegmentInspections()
	log.Debug().Str("stream_id", p.id).Uint64("generation", p.generation).
		Int("startup_segments_skipped", len(p.hlsSuppressed)).
		Str("first_segment", ready.Media[0].Name).
		Msg("HLS startup media layout validated; steady-state segment inspection disabled")
	return ready, nil
}

// steadyHLSWindow removes only the fragments rejected before the first safe
// decoder bootstrap. Every completed fragment created after that point is
// published monotonically without a per-segment track-payload requirement.
func (p *gstProducer) steadyHLSWindow(snapshot *HLSPlaylistSnapshot) *HLSPlaylistSnapshot {
	if len(p.hlsSuppressed) == 0 {
		return snapshot
	}
	current := make(map[string]struct{}, len(snapshot.Media))
	result := &HLSPlaylistSnapshot{}
	for _, media := range snapshot.Media {
		current[media.Name] = struct{}{}
		if _, suppressed := p.hlsSuppressed[media.Name]; suppressed {
			continue
		}
		result.Media = append(result.Media, media)
		result.Segments = append(result.Segments, media.Name)
		result.Duration += media.Duration
	}
	for name := range p.hlsSuppressed {
		if _, advertised := current[name]; !advertised {
			delete(p.hlsSuppressed, name)
		}
	}
	return result
}

func (p *gstProducer) inspectHLSSegment(path string) (BootstrapStatus, error) {
	info, err := os.Stat(path)
	if err != nil {
		return BootstrapStatus{}, err
	}
	p.hlsMediaMu.Lock()
	defer p.hlsMediaMu.Unlock()
	if cached, ok := p.hlsMediaInspections[path]; ok && cached.size == info.Size() &&
		cached.modifiedNS == info.ModTime().UnixNano() {
		return cached.status, nil
	}
	status, err := inspectHLSSegment(path)
	if err != nil {
		return BootstrapStatus{}, err
	}
	if p.hlsMediaInspections == nil {
		p.hlsMediaInspections = make(map[string]cachedHLSSegmentInspection)
	}
	p.hlsMediaInspections[path] = cachedHLSSegmentInspection{
		size: info.Size(), modifiedNS: info.ModTime().UnixNano(), status: status,
	}
	return status, nil
}

func (p *gstProducer) pruneHLSSegmentInspections(directory string, segments []string) {
	retained := make(map[string]struct{}, len(segments))
	for _, segment := range segments {
		retained[filepath.Join(directory, segment)] = struct{}{}
	}
	p.hlsMediaMu.Lock()
	defer p.hlsMediaMu.Unlock()
	for path := range p.hlsMediaInspections {
		if _, advertised := retained[path]; !advertised {
			delete(p.hlsMediaInspections, path)
		}
	}
}

func (p *gstProducer) clearHLSSegmentInspections() {
	p.hlsMediaMu.Lock()
	p.hlsMediaInspections = nil
	p.hlsMediaMu.Unlock()
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
	// Repeat programme tables frequently so every decoder-safe warm replay can
	// begin close to its keyframe instead of inheriting a long delta-frame run.
	setOptional(mux, "pat-interval", uint(9000))
	setOptional(mux, "pmt-interval", uint(9000))
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
		if p.stopping.Load() {
			return
		}
		name := strings.ToLower(elementary.GetName())
		switch {
		case strings.HasPrefix(name, "video_"):
			p.expectMedia("video")
		case strings.HasPrefix(name, "audio_"):
			p.expectMedia("audio")
		}
		p.routeElementaryPadWhenNegotiated(elementary, mux)
	})
	p.graphMu.Unlock()
	locked = false
	queue.SyncStateWithParent()
	demux.SyncStateWithParent()
	return nil
}

// routeElementaryPadWhenNegotiated waits for a fixed CAPS event instead of
// choosing a parser from QueryCaps. QueryCaps describes every format a dynamic
// pad may eventually produce (for example AAC versions 2 or 4 alongside MPEG
// audio), not the format selected for this stream. Guessing from that list can
// attach an incompatible parser and tear down the shared producer.
func (p *gstProducer) routeElementaryPadWhenNegotiated(pad *gst.Pad, mux *gst.Element) {
	var routeOnce sync.Once
	route := func(caps *gst.Caps) {
		if caps == nil || !caps.IsFixed() || p.stopping.Load() {
			return
		}
		routeOnce.Do(func() {
			if err := p.routeElementaryPad(pad, caps.String(), mux); err != nil {
				p.reportError(err)
			}
		})
	}

	probeID := pad.AddProbe(gst.PadProbeTypeEventDownstream, func(self *gst.Pad, info *gst.PadProbeInfo) gst.PadProbeReturn {
		event := info.GetEvent()
		if event == nil || event.Type() != gst.EventTypeCaps {
			return gst.PadProbeOK
		}
		caps := event.ParseCaps()
		if caps == nil || !caps.IsFixed() {
			return gst.PadProbeOK
		}
		route(caps)
		return gst.PadProbeRemove
	})
	if caps := pad.GetCurrentCaps(); caps != nil && caps.IsFixed() {
		route(caps)
		pad.RemoveProbe(probeID)
	}
}

func (p *gstProducer) routeElementaryPad(pad *gst.Pad, capsString string, mux *gst.Element) error {
	p.rememberMediaTrack(capsString)
	p.graphMu.Lock()
	if p.stopping.Load() {
		p.graphMu.Unlock()
		return nil
	}
	parserName, media, ok := parserForCaps(capsString)
	if !ok {
		p.graphMu.Unlock()
		log.Warn().Str("stream_id", p.id).Str("caps", capsString).Msg("Ignoring unsupported source media track")
		return nil
	}
	p.expectMedia(media)
	p.routeMu.Lock()
	if p.routed[media] {
		p.routeMu.Unlock()
		p.graphMu.Unlock()
		return nil
	}
	p.routed[media] = true
	p.lastRouteChange = time.Now()
	p.routeMu.Unlock()
	elements, err := p.routePadToMux(pad, parserName, capsString, media, mux)
	if err != nil {
		p.routeMu.Lock()
		delete(p.routed, media)
		p.routeMu.Unlock()
		p.graphMu.Unlock()
		return err
	}
	// Syncing a dynamically-added element can execute streaming work. Do not
	// hold graphMu across it: teardown also needs that lock before moving the
	// pipeline to NULL, and waiting on each other leaves a session in stopping.
	p.graphMu.Unlock()
	for _, element := range elements {
		if p.stopping.Load() {
			return nil
		}
		element.SyncStateWithParent()
	}
	return nil
}

func (p *gstProducer) expectMedia(media string) {
	p.routeMu.Lock()
	defer p.routeMu.Unlock()
	if p.expectedMedia == nil {
		p.expectedMedia = make(map[string]bool)
	}
	if p.expectedMedia[media] {
		return
	}
	p.expectedMedia[media] = true
	p.lastRouteChange = time.Now()
}

func (p *gstProducer) decoderReady(now time.Time) bool {
	p.routeMu.Lock()
	expectsVideo := p.expectedMedia["video"] || p.routed["video"]
	expectsAudio := p.expectedMedia["audio"] || p.routed["audio"]
	routedVideo := p.routed["video"]
	routedAudio := p.routed["audio"]
	lastRouteChange := p.lastRouteChange
	p.routeMu.Unlock()
	if (!expectsVideo && !expectsAudio) || lastRouteChange.IsZero() || now.Sub(lastRouteChange) < trackDiscoverySettle {
		return false
	}
	if (expectsVideo && !routedVideo) || (expectsAudio && !routedAudio) {
		return false
	}
	status := p.hub.BootstrapStatus()
	if !status.Complete {
		return false
	}
	if expectsVideo && (!status.HasVideo || status.FirstVideoPTS90K == nil) {
		return false
	}
	if expectsAudio && (!status.HasAudio || status.FirstAudioPTS90K == nil) {
		return false
	}
	return true
}

func (p *gstProducer) routePadToMux(pad *gst.Pad, parserName, capsString, media string, mux *gst.Element) ([]*gst.Element, error) {
	parser, err := gst.NewElement(parserName)
	if err != nil {
		return nil, fmt.Errorf("create %s: %w", parserName, err)
	}
	configureStreamParser(parser, parserName)
	queue, err := gst.NewElement("queue")
	if err != nil {
		return nil, fmt.Errorf("create adaptive track queue: %w", err)
	}
	_ = queue.Set("max-size-time", uint64(3*time.Second))
	_ = queue.Set("max-size-bytes", uint(0))
	_ = queue.Set("max-size-buffers", uint(0))
	tee, err := gst.NewElement("tee")
	if err != nil {
		return nil, fmt.Errorf("create %s output tee: %w", media, err)
	}
	elements := []*gst.Element{}
	p.pipeline.Add(parser)
	p.pipeline.Add(tee)
	p.pipeline.Add(queue)
	if result := pad.Link(parser.GetStaticPad("sink")); result != gst.PadLinkOK {
		return nil, fmt.Errorf("link adaptive track to %s: %s", parserName, result.String())
	}
	if err := parser.Link(tee); err != nil {
		return nil, fmt.Errorf("link %s to shared track outputs: %w", parserName, err)
	}
	if err := tee.Link(queue); err != nil {
		return nil, fmt.Errorf("link %s to transport queue: %w", parserName, err)
	}
	muxPad := mux.GetRequestPad("sink_%d")
	if muxPad == nil {
		return nil, errors.New("request adaptive muxer input pad")
	}
	if result := queue.GetStaticPad("src").Link(muxPad); result != gst.PadLinkOK {
		return nil, fmt.Errorf("link adaptive queue to MPEG-TS muxer: %s", result.String())
	}
	if p.hlsSink != nil {
		hlsQueue, queueErr := gst.NewElement("queue")
		if queueErr != nil {
			return nil, fmt.Errorf("create %s HLS queue: %w", media, queueErr)
		}
		_ = hlsQueue.Set("max-size-time", uint64(10*time.Second))
		_ = hlsQueue.Set("max-size-bytes", uint(0))
		_ = hlsQueue.Set("max-size-buffers", uint(0))
		p.pipeline.Add(hlsQueue)
		if err := tee.Link(hlsQueue); err != nil {
			return nil, fmt.Errorf("link %s to HLS queue: %w", media, err)
		}
		hlsTail := hlsQueue
		compatibility, action, compatibilityErr := p.hlsCompatibilityChain(parserName, capsString)
		if compatibilityErr != nil {
			return nil, compatibilityErr
		}
		for _, element := range compatibility {
			p.pipeline.Add(element)
			if err := hlsTail.Link(element); err != nil {
				return nil, fmt.Errorf("link %s HLS compatibility pipeline: %w", media, err)
			}
			hlsTail = element
		}
		hlsPad := p.hlsSink.GetRequestPad(media)
		if hlsPad == nil {
			return nil, fmt.Errorf("request HLS %s input pad", media)
		}
		if result := hlsTail.GetStaticPad("src").Link(hlsPad); result != gst.PadLinkOK {
			return nil, fmt.Errorf("link %s to HLS segmenter: %s", media, result.String())
		}
		elements = append(elements, hlsQueue)
		elements = append(elements, compatibility...)
		if action != "" {
			p.rememberCompatibilityAction(action)
		}
	}
	elements = append(elements, parser, tee, queue)
	return elements, nil
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
	case parserName == "aacparse" && aacProfileNeedsNormalization(lowerCaps):
		// Chromium's Media Source implementation rejects AAC Main (mp4a.40.1)
		// and the other legacy AAC profiles even though they are valid in an
		// MPEG transport stream. Keep the original elementary stream on the
		// canonical MPEG-TS branch, and encode only the HLS branch as AAC-LC.
		names = []string{"faad", "audioconvert", "audioresample", "voaacenc", "aacparse"}
		action = "The source AAC profile is being normalized to AAC-LC for browser playback."
	case parserName == "mpegaudioparse":
		names = []string{"mpg123audiodec", "audioconvert", "audioresample", "voaacenc", "aacparse"}
		action = "MPEG audio is being normalized to AAC for browser playback."
	default:
		return nil, ""
	}
	return names, action
}

func aacProfileNeedsNormalization(lowerCaps string) bool {
	for _, profile := range []string{"main", "ssr", "ltp", "ld", "eld"} {
		if strings.Contains(lowerCaps, "profile=(string)"+profile) ||
			strings.Contains(lowerCaps, "profile="+profile) ||
			strings.Contains(lowerCaps, "base-profile=(string)"+profile) ||
			strings.Contains(lowerCaps, "base-profile="+profile) {
			return true
		}
	}
	return false
}

func (p *gstProducer) addTransportOutput(tee *gst.Element) error {
	queue, err := gst.NewElement("queue")
	if err != nil {
		return fmt.Errorf("create shared transport queue: %w", err)
	}
	_ = queue.Set("max-size-time", uint64(2*time.Second))
	_ = queue.Set("max-size-bytes", uint(16*1024*1024))
	_ = queue.Set("max-size-buffers", uint(0))
	// Never discard packets after the canonical mux. Even a single dropped TS
	// packet can corrupt the first keyframe and leave strict tuner clients black.
	// The downstream Hub is already non-blocking and independently bounds every
	// viewer, while this queue provides short scheduling backpressure only.
	_ = queue.Set("leaky", 0)

	sink, err := gstapp.NewAppSink()
	if err != nil {
		return fmt.Errorf("create shared transport sink: %w", err)
	}
	// Eight 1,316-byte samples represented only a few milliseconds at common
	// channel bitrates. A brief Go scheduling pause could therefore make the
	// appsink silently drop canonical transport packets during cold startup.
	sink.SetMaxBuffers(512)
	sink.SetDrop(false)
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
			hasTables := p.probe.Push(data)
			p.hub.Publish(data)
			if hasTables && p.decoderReady(time.Now()) {
				p.readyOnce.Do(func() {
					close(p.ready)
					go p.enableSteadyBuffering()
				})
			}
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
	case strings.Contains(lower, "audio/mpeg"):
		// GStreamer uses mpegversion 2 and 4 for AAC, and mpegversion 1
		// for MPEG audio layers I-III. Do not guess when the version is
		// absent, a range, or a list from unnegotiated QueryCaps.
		mpegVersion, fixed := capsIntegerField(lower, "mpegversion")
		if !fixed {
			return "", "", false
		}
		switch mpegVersion {
		case 2, 4:
			return "aacparse", "audio", true
		case 1:
			return "mpegaudioparse", "audio", true
		default:
			return "", "", false
		}
	default:
		return "", "", false
	}
}

func capsIntegerField(caps, field string) (int, bool) {
	marker := strings.ToLower(field) + "="
	index := strings.Index(caps, marker)
	if index < 0 {
		return 0, false
	}
	value := strings.TrimSpace(caps[index+len(marker):])
	if strings.HasPrefix(value, "(int)") {
		value = strings.TrimSpace(value[len("(int)"):])
	}
	end := 0
	for end < len(value) && value[end] >= '0' && value[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, false
	}
	parsed, err := strconv.Atoi(value[:end])
	return parsed, err == nil
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
		snapshot, err := p.HLSPlaylistSnapshot()
		// One independently decodable segment with the canonical audio/video
		// layout is enough to start. Waiting for three makes startup depend on
		// upstream keyframe cadence and can turn a healthy stream into a timeout.
		if err != nil || len(snapshot.Segments) < 1 || snapshot.Duration <= 0 {
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
		graphLocked := false
		deadline := time.Now().Add(500 * time.Millisecond)
		for !graphLocked && time.Now().Before(deadline) {
			graphLocked = p.graphMu.TryLock()
			if !graphLocked {
				time.Sleep(10 * time.Millisecond)
			}
		}
		if graphLocked {
			defer p.graphMu.Unlock()
		} else {
			// A dynamic-pad callback must never prevent shutdown indefinitely.
			// The session-level cleanup watchdog keeps the source lease reserved
			// until this producer actually returns.
			log.Warn().Str("stream_id", p.id).Msg("Stopping pipeline without graph lock after cleanup deadline")
		}
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
