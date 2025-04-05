package streaming

import (
	"bufio"
	"errors"
	"fmt"
	"math"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"xivi/backend/platform/settings"

	"github.com/go-gst/go-glib/glib"
	"github.com/go-gst/go-gst/gst"
	gstapp "github.com/go-gst/go-gst/gst/app"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

var (
	Streams []*Stream
	// Global cleanup interval - 15 seconds
	cleanupInterval = 15 * time.Second
	// Maximum time a stream can be idle before cleanup - 60 seconds
	maxIdleTime = 60 * time.Second
	// Mutex for Streams slice operations
	streamsMutex sync.RWMutex
)

func init() {
	// Start global cleanup goroutine
	go globalCleanup()
}

// globalCleanup periodically checks for and removes stale streams
func globalCleanup() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()

		// Use a read lock to safely iterate through streams
		streamsMutex.RLock()
		streamsToCheck := make([]*Stream, len(Streams))
		copy(streamsToCheck, Streams) // Create a copy to avoid holding the lock during cleanup
		streamsMutex.RUnlock()

		for _, stream := range streamsToCheck {
			stream.Mu.Lock()
			idle := now.Sub(stream.LastAccess) > maxIdleTime
			stream.Mu.Unlock()

			if idle {
				log.Info().Msgf("Cleaning up idle stream: %s (idle for %s)",
					stream.Settings.Uuid, now.Sub(stream.LastAccess).Round(time.Second))
				go stream.Close(nil, nil)
			}
		}
	}
}

// Stream represents a media stream with its pipeline and settings
type Stream struct {
	pipeline       *gst.Pipeline
	HlsExists      bool
	hlsCleanup     chan struct{}
	Settings       *Settings // The settings for the element
	count          int       // The current stream count
	LastAccess     time.Time // Last endpoint hit
	Mu             sync.Mutex
	isClosing      bool  // Flag to prevent multiple close operations
	activeClients  int32 // Number of active clients (atomic)
	networkQuality int32 // Network quality indicator (0-100, atomic)
}

// Settings contains configuration for a stream
type Settings struct {
	Uuid      string // Unique identifier for the stream
	Src       string // Source URL
	Buffer    int    // Buffer size in seconds
	UserAgent string // User agent for HTTP requests
}

// Streamer interface for writing data to clients
type Streamer interface {
	Write(p []byte) (n int, err error) // Write writes bytes to streamer
	Flush() error                      // Flush flushes data to the client
}

// NewStream creates a new stream with the given ID and source URL
func NewStream(streamId string, src string) *Stream {
	return &Stream{
		HlsExists:      false,
		hlsCleanup:     make(chan struct{}),
		count:          0,
		LastAccess:     time.Now(),
		isClosing:      false,
		activeClients:  0,
		networkQuality: 100, // Start with optimal quality assumption
		Settings: &Settings{
			Uuid:      streamId,
			Src:       src,
			Buffer:    settings.APP_SETTINGS.Streaming.Buffer,
			UserAgent: settings.APP_SETTINGS.Streaming.UserAgent,
		},
	}
}

// AddStream adds a stream to the global Streams slice, replacing any existing stream with the same UUID
func AddStream(s *Stream) {
	streamsMutex.Lock()
	defer streamsMutex.Unlock()

	// Check for existing stream with same UUID
	for i, existing := range Streams {
		if existing != nil && existing.Settings != nil && existing.Settings.Uuid == s.Settings.Uuid {
			log.Info().Msgf("Replacing existing stream with UUID %s", s.Settings.Uuid)

			// Close existing stream before adding new one
			existing.Close(nil, nil)

			// Wait for cleanup to complete
			time.Sleep(500 * time.Millisecond)

			// Remove the existing stream from the slice
			copy(Streams[i:], Streams[i+1:])
			Streams = Streams[:len(Streams)-1]

			// Force garbage collection
			runtime.GC()
			break
		}
	}

	Streams = append(Streams, s)
	log.Info().Msgf("Added stream with UUID %s, total streams: %d", s.Settings.Uuid, len(Streams))
}

// RemoveStream removes a stream from the global Streams slice
func RemoveStream(s *Stream) {
	streamsMutex.Lock()
	defer streamsMutex.Unlock()

	for i, stream := range Streams {
		if stream == s {
			// Efficient slice manipulation without creating temporary slices
			copy(Streams[i:], Streams[i+1:])
			Streams = Streams[:len(Streams)-1]
			log.Info().Msgf("Removed stream %s, remaining streams: %d", s.Settings.Uuid, len(Streams))

			// Force garbage collection to release GStreamer resources
			runtime.GC()
			break
		}
	}
}

// Close the stream or a specific sink bin
func (s *Stream) Close(sinkBin *gst.Bin, done chan bool) gst.FlowReturn {
	s.Mu.Lock()

	// Prevent multiple closes
	if s.count < 0 || s.isClosing {
		s.Mu.Unlock()
		return gst.FlowEOS
	}

	// Mark as closing to prevent concurrent close operations
	s.isClosing = true

	// Decrement count and check if we need to do full cleanup
	s.count--
	if s.count < 0 {
		s.count = 0
	}

	// Determine if we need full cleanup
	needFullCleanup := s.count <= 0
	log.Info().Msgf("Closing stream, count: %d, UUID: %s", s.count, s.Settings.Uuid)

	// Handle HLS cleanup if needed
	if sinkBin == nil && s.HlsExists {
		select {
		case _, ok := <-s.hlsCleanup:
			if ok {
				close(s.hlsCleanup)
			}
		default:
		}
		s.HlsExists = false
	}

	// Handle done channel
	if done != nil {
		select {
		case <-done: // Already closed
			// Do nothing
		default:
			close(done)
		}
	}

	// Release the lock before potentially long operations
	s.isClosing = false
	s.Mu.Unlock()

	// Full pipeline cleanup if no streams are left
	if needFullCleanup {
		log.Info().Msgf("Cleaning up pipeline for: %s", s.Settings.Uuid)
		RemoveStream(s)
		go func() {
			if s.pipeline != nil && s.pipeline.GstObject() != nil {
				// Set pipeline to NULL state
				s.pipeline.SetState(gst.StateNull)
				time.Sleep(200 * time.Millisecond)

				// Send EOS event with retry
				for i := 0; i < 2; i++ {
					if s.pipeline.SendEvent(gst.NewEOSEvent()) {
						break
					}
					time.Sleep(100 * time.Millisecond)
				}

				// Set all elements to NULL state
				if elements, _ := s.pipeline.GetElementsSorted(); elements != nil {
					for _, element := range elements {
						if element == nil || element.GstObject() == nil {
							continue
						}

						// Pause all pad tasks
						if pads, err := element.GetPads(); err == nil && pads != nil {
							for _, pad := range pads {
								if pad != nil && pad.GstObject() != nil {
									pad.PauseTask()
								}
							}
						}

						// Set element to NULL state
						if element.GstObject() != nil {
							element.SetState(gst.StateNull)
						}
					}
					time.Sleep(100 * time.Millisecond)
				}

				// Clear the pipeline with panic recovery
				if s.pipeline.GstObject() != nil {
					func() {
						defer func() {
							if r := recover(); r != nil {
								log.Warn().Msgf("Recovered from panic during pipeline cleanup: %v", r)
							}
						}()
						s.pipeline.Clear()
					}()
				}
				s.pipeline = nil

				// Force garbage collection
				runtime.GC()
			}

			// Cleanup HLS directory
			hlsDir := fmt.Sprintf("%s/%s", settings.STREAM_FILEPATH, s.Settings.Uuid)
			if _, err := os.Stat(hlsDir); err == nil {
				os.RemoveAll(hlsDir)
			}

			// Final cleanup of stream object
			s.Mu.Lock()
			s.isClosing = false
			s.count = 0
			s.Mu.Unlock()
			log.Info().Msgf("Pipeline disposed for stream: %s", s.Settings.Uuid)
		}()
		return gst.FlowEOS
	}

	// If we still have active streams, just cleanup the specific sink bin
	if sinkBin != nil {
		log.Info().Msgf("Closing stream branch, %d streams remaining", s.count)
		go func() {
			if tee, err := s.pipeline.GetElementByName("stream"); err == nil {
				// Unlink the bin and set to NULL state
				tee.Unlink(sinkBin.Element)
				sinkBin.SetState(gst.StateNull)
				time.Sleep(50 * time.Millisecond)

				// Remove the bin from pipeline
				s.pipeline.Remove(sinkBin.Element)
				sinkBin.SendEvent(gst.NewEOSEvent())
				sinkBin.Clear()

				// Ensure remaining pipeline elements are in correct state
				if s.count > 0 && s.pipeline != nil && s.pipeline.GstObject() != nil {
					s.pipeline.SetState(gst.StatePlaying)
				}
			}
			log.Info().Msgf("Stream branch closed, %d streams remaining", s.count)
		}()
	}

	return gst.FlowEOS
}

// Write writes bytes to the provided writer
func (s *Stream) Write(writer *bufio.Writer, p []byte) (n int, err error) {
	if writer != nil {
		return writer.Write(p)
	}
	return 0, nil
}

// Flush flushes data to the client
func (s *Stream) Flush(writer *bufio.Writer) error {
	if writer != nil {
		return writer.Flush()
	}
	return nil
}

// NewHLSSink creates an HLS sink for the stream
func (s *Stream) NewHLSSink(ctx *fiber.Ctx) error {
	s.HlsExists = true
	ctx.Set(fiber.HeaderContentType, "application/vnd.apple.mpegurl")
	atomic.AddInt32(&s.activeClients, 1)

	// Configure HLS paths
	pRoot := fmt.Sprintf("http://%s:%d/stream/hls/%s", settings.APP_SETTINGS.Server.Host, settings.APP_SETTINGS.Server.Port, s.Settings.Uuid)
	pLocation := fmt.Sprintf("%s/%s/%s", settings.STREAM_FILEPATH, s.Settings.Uuid, "playlist.m3u8")
	location := fmt.Sprintf("%s/%s/%s", settings.STREAM_FILEPATH, s.Settings.Uuid, "segment.%05d.ts")

	// Create elements
	buffer, err := gst.NewElement("queue2")
	if err != nil {
		atomic.AddInt32(&s.activeClients, -1)
		return fmt.Errorf("failed to create queue2 element: %v", err)
	}
	demux, err := gst.NewElement("tsdemux")
	if err != nil {
		atomic.AddInt32(&s.activeClients, -1)
		return fmt.Errorf("failed to create tsdemux element: %v", err)
	}
	sink, err := gst.NewElement("hlssink2")
	if err != nil {
		atomic.AddInt32(&s.activeClients, -1)
		return fmt.Errorf("failed to create hlssink2 element: %v", err)
	}

	// Create queues for audio and video
	videoQueue, err := gst.NewElement("queue")
	if err != nil {
		atomic.AddInt32(&s.activeClients, -1)
		return fmt.Errorf("failed to create video queue element: %v", err)
	}
	audioQueue, err := gst.NewElement("queue")
	if err != nil {
		atomic.AddInt32(&s.activeClients, -1)
		return fmt.Errorf("failed to create audio queue element: %v", err)
	}

	// Configure video and audio queues for better performance
	videoQueue.Set("max-size-buffers", 0)
	videoQueue.Set("max-size-bytes", 0)
	videoQueue.Set("max-size-time", uint64(2*time.Second))
	videoQueue.Set("leaky", 2)           // Downstream leaky queue for smoother playback
	videoQueue.Set("flush-on-eos", true) // Flush on end of stream

	audioQueue.Set("max-size-buffers", 0)
	audioQueue.Set("max-size-bytes", 0)
	audioQueue.Set("max-size-time", uint64(2*time.Second))
	audioQueue.Set("leaky", 2)           // Downstream leaky queue for smoother playback
	audioQueue.Set("flush-on-eos", true) // Flush on end of stream

	// Configure buffer with optimized settings for faster startup
	bufferTimeNs := uint64(s.Settings.Buffer * 1000000000)
	buffer.Set("max-size-buffers", 0)
	buffer.Set("max-size-bytes", 0)
	buffer.Set("max-size-time", bufferTimeNs)
	buffer.Set("use-buffering", true)
	buffer.Set("low-watermark", 0.01)                  // Lower watermark for faster startup
	buffer.Set("high-watermark", 0.99)                 // Higher watermark for faster startup
	buffer.Set("min-threshold-time", bufferTimeNs/10)  // Lower threshold for faster startup
	buffer.Set("ring-buffer-max-size", bufferTimeNs*2) // Smaller ring buffer for faster startup

	// Configure segment duration - shorter segments for smoother playback
	segmentDuration := 1 // 1-second segments
	if s.Settings.Buffer > 8 {
		segmentDuration = 2
	}
	segmentDurationNs := uint64(segmentDuration * 1000000000)

	// Configure HLS sink with optimized settings for smoother playback
	sink.Set("name", "hlssink")
	sink.Set("playlist-root", pRoot)
	sink.Set("playlist-location", pLocation)
	sink.Set("location", location)
	sink.Set("max-files", uint64(s.Settings.Buffer/segmentDuration+3))
	sink.Set("playlist-length", max(uint64(s.Settings.Buffer/segmentDuration+2), uint64(3)))
	sink.Set("target-duration", segmentDuration)
	sink.Set("streaming", true)
	sink.Set("send-keyframe-requests", true)
	sink.Set("playlist-type", "live")
	// Add these parameters back for smoother playback
	sink.Set("max-size-time", segmentDurationNs)          // Maximum segment duration
	sink.Set("splitmux-max-size-time", segmentDurationNs) // Split at segment boundaries
	sink.Set("fragment-duration", uint64(50*1000000))     // Fragment duration (50ms)
	sink.Set("min-fragment-duration", uint64(25*1000000)) // Minimum fragment duration (50ms)
	sink.Set("render-delay", uint64(0))                   // No render delay
	sink.Set("playlist-location-ui-name", "HLS Playlist") // UI name for debugging
	sink.Set("generate-null-segment", true)               // Generate null segment to maintain continuity

	// Create bin with unique name
	bin := gst.NewBin(fmt.Sprintf("sinkbin%d", s.count))
	for i := 0; i <= s.count; i++ {
		if element, _ := s.pipeline.GetElementByName(fmt.Sprintf("sinkbin%d", i)); element == nil {
			bin = gst.NewBin(fmt.Sprintf("sinkbin%d", i))
			break
		}
	}

	// Add elements to bin
	bin.Add(buffer)
	bin.Add(demux)
	bin.Add(videoQueue)
	bin.Add(audioQueue)
	bin.Add(sink)

	// Create ghost pad for bin input
	bufPad := gst.NewGhostPad("ghost", buffer.GetStaticPad("sink"))
	bufPad.SetActive(true)
	bin.AddPad(bufPad.Pad)

	// Link initial elements
	buffer.Link(demux)

	// Handle dynamic pads from demuxer
	demux.Connect("pad-added", func(self *gst.Element, srcPad *gst.Pad) {
		var parseElement *gst.Element
		var queue *gst.Element
		var sinkPad *gst.Pad
		var err error

		caps := srcPad.GetCurrentCaps()
		if caps == nil {
			log.Error().Msg("No caps on pad")
			return
		}

		str := caps.GetStructureAt(0)
		if str == nil {
			log.Error().Msg("No structure in caps")
			return
		}

		name := str.Name()
		log.Info().Msgf("New pad added with caps: %s", name)

		// Handle different media types
		if strings.HasPrefix(name, "audio/mpeg") {
			parseElement, err = gst.NewElement("aacparse")
			queue = audioQueue
			sinkPad = sink.GetRequestPad("audio")
		} else if strings.HasPrefix(name, "audio/x-ac3") {
			parseElement, err = gst.NewElement("ac3parse")
			queue = audioQueue
			sinkPad = sink.GetRequestPad("audio")
		} else if strings.HasPrefix(name, "video/x-h264") {
			parseElement, err = gst.NewElement("h264parse")
			queue = videoQueue
			sinkPad = sink.GetRequestPad("video")
		} else if strings.HasPrefix(name, "video/x-h265") {
			parseElement, err = gst.NewElement("h265parse")
			queue = videoQueue
			sinkPad = sink.GetRequestPad("video")
		} else {
			log.Warn().Msgf("Unknown pad type: %s", name)
			return
		}

		if err != nil {
			log.Error().Msgf("Failed to create parser element: %v", err)
			return
		}

		bin.Add(parseElement)
		parseElement.SyncStateWithParent()

		// Link elements
		if ret := srcPad.Link(parseElement.GetStaticPad("sink")); ret != gst.PadLinkOK {
			log.Error().Msgf("Failed to link demux to parser: %v", ret)
			return
		}

		if err := parseElement.Link(queue); err != nil {
			log.Error().Msgf("Failed to link parser to queue: %v", err)
			return
		}

		queuePad := queue.GetStaticPad("src")
		if ret := queuePad.Link(sinkPad); ret != gst.PadLinkOK {
			log.Error().Msgf("Failed to link queue to sink: %v", ret)
			return
		}
	})

	// Sync states
	elements, _ := bin.GetElements()
	for _, e := range elements {
		e.SyncStateWithParent()
	}

	// Add bin to pipeline
	tee, err := s.pipeline.GetElementByName("stream")
	if err != nil {
		bin.SetState(gst.StateNull)
		bin.Clear()
		atomic.AddInt32(&s.activeClients, -1)
		return fmt.Errorf("failed to get tee element: %v", err)
	}

	s.pipeline.Add(bin.Element)

	if err := tee.Link(bin.Element); err != nil {
		s.pipeline.Remove(bin.Element)
		bin.SetState(gst.StateNull)
		bin.Clear()
		atomic.AddInt32(&s.activeClients, -1)
		return fmt.Errorf("failed to link tee to bin: %v", err)
	}

	bin.SyncStateWithParent()
	s.count++

	// Start cleanup monitor
	go func(stop chan struct{}, sinkbin *gst.Bin) {
		idleTime := 30 * time.Second
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		defer atomic.AddInt32(&s.activeClients, -1)

		for {
			select {
			case <-stop:
				s.HlsExists = false
				return
			case <-ticker.C:
				s.Mu.Lock()
				idle := time.Since(s.LastAccess) > idleTime
				s.Mu.Unlock()

				if idle {
					log.Info().Msgf("HLS stream idle for %s, cleaning up", idleTime)
					s.HlsExists = false
					s.Close(sinkbin, nil)
					return
				}
			}
		}
	}(s.hlsCleanup, bin)

	log.Info().Msgf("HLS Stream started for %s", s.Settings.Uuid)
	return nil
}

// NewMP2TSink creates an MPEG-TS sink for the stream
func (s *Stream) NewMP2TSink(ctx *fiber.Ctx) error {
	var writer *bufio.Writer
	var done chan bool
	var writerClosed = make(chan bool)
	var heartbeatTicker *time.Ticker
	var heartbeatDone = make(chan bool)

	ctx.Set(fiber.HeaderContentType, "video/MP2T")

	// Setup writer and connection monitoring
	done = make(chan bool)
	ready := make(chan bool)
	atomic.AddInt32(&s.activeClients, 1)

	// Monitor client connection
	go func() {
		defer func() {
			atomic.AddInt32(&s.activeClients, -1)
			close(writerClosed)
			s.Close(nil, done)
			if heartbeatTicker != nil {
				heartbeatTicker.Stop()
				close(heartbeatDone)
			}
		}()

		// Wait for client disconnection
		<-ctx.Context().Done()
		log.Info().Msgf("Client disconnected from MP2T stream: %s", s.Settings.Uuid)
	}()

	ctx.Context().Response.SetBodyStreamWriter(func(w *bufio.Writer) {
		writer = w
		ready <- true // Signal that writer is set

		// Start heartbeat to detect broken connections
		heartbeatTicker = time.NewTicker(5 * time.Second)
		go func() {
			for {
				select {
				case <-heartbeatDone:
					return
				case <-heartbeatTicker.C:
					if writer != nil {
						if err := writer.Flush(); err != nil {
							log.Debug().Msgf("Heartbeat detected broken connection: %v", err)
							return
						}
					}
				}
			}
		}()

		<-done // Wait for stream to be closed
	})

	<-ready // Wait until writer is set

	// Create bin and elements
	bin := gst.NewBin(fmt.Sprintf("sinkbin%d", s.count))
	buffer, err := gst.NewElement("queue2")
	if err != nil {
		return err
	}
	sink, err := gstapp.NewAppSink()
	if err != nil {
		return err
	}

	// Configure buffer
	bufferTimeNs := uint64(s.Settings.Buffer * 1000000000)
	buffer.Set("max-size-time", bufferTimeNs)
	buffer.Set("use-buffering", true)
	buffer.Set("low-watermark", 0.05)
	buffer.Set("high-watermark", 0.95)

	// Configure sink
	sink.Set("max-time", bufferTimeNs)
	sink.Set("drop", true)         // Drop late buffers
	sink.Set("emit-signals", true) // Emit signals for callbacks
	sink.Set("sync", false)        // Don't sync to clock for streaming

	// Add elements to bin and link
	bin.Add(buffer)
	bin.Add(sink.Element)
	buffer.Link(sink.Element)

	// Create ghost pad
	bufPad := gst.NewGhostPad("mp2tghost", buffer.GetStaticPad("sink"))
	bufPad.SetActive(true)
	bin.AddPad(bufPad.Pad)

	// Setup callbacks
	sink.SetCallbacks(&gstapp.SinkCallbacks{
		NewSampleFunc: func(appSink *gstapp.Sink) gst.FlowReturn {
			// Check if writer is closed
			select {
			case <-writerClosed:
				return s.Close(bin, done)
			default:
			}

			// Update LastAccess time
			s.Mu.Lock()
			s.LastAccess = time.Now()
			s.Mu.Unlock()

			// Check for EOS
			if appSink.IsEOS() {
				return s.Close(bin, done)
			}

			// Pull sample
			sample := appSink.TryPullSample(gst.ClockTime(5 * time.Second))
			if sample == nil {
				return s.Close(bin, done)
			}

			// Get buffer from sample
			buffer := sample.GetBuffer()
			if buffer == nil || buffer.GetSize() == 0 {
				return s.Close(bin, done)
			}

			// Extract data and write to client
			data := buffer.Extract(0, buffer.GetSize())
			defer buffer.Unmap()

			if _, err = s.Write(writer, data); err != nil {
				return s.Close(bin, done)
			}

			if err = s.Flush(writer); err != nil {
				return s.Close(bin, done)
			}

			return gst.FlowOK
		},
		EOSFunc: func(sink *gstapp.Sink) {
			s.Close(bin, done)
		},
	})

	// Sync element states
	elements, _ := bin.GetElements()
	for _, e := range elements {
		e.SyncStateWithParent()
	}

	// Add bin to pipeline and link to tee
	tee, err := s.pipeline.GetElementByName("stream")
	if err != nil {
		bin.SetState(gst.StateNull)
		bin.Clear()
		return err
	}

	s.pipeline.Add(bin.Element)

	if err := tee.Link(bin.Element); err != nil {
		s.pipeline.Remove(bin.Element)
		bin.SetState(gst.StateNull)
		bin.Clear()
		return err
	}

	bin.SyncStateWithParent()
	s.count++

	log.Info().Msgf("MP2T Stream started for %s", s.Settings.Uuid)
	return nil
}

// createPipeline creates a GStreamer pipeline for the stream
func (s *Stream) createPipeline() (*gst.Pipeline, error) {
	// Validate source URL
	if s.Settings.Src == "" {
		return nil, errors.New("no source URL configured for stream")
	}

	// Initialize GStreamer if needed
	gst.Init(nil)

	// Create a new pipeline with a unique name based on UUID
	pipelineName := fmt.Sprintf("pipeline-%s", s.Settings.Uuid)
	pipeline, err := gst.NewPipeline(pipelineName)
	if err != nil {
		return nil, fmt.Errorf("failed to create pipeline: %v", err)
	}

	// Create source element
	src, err := gst.NewElement("souphttpsrc")
	if err != nil {
		return nil, fmt.Errorf("failed to create souphttpsrc element: %v", err)
	}

	// Create typefind element
	typefind, err := gst.NewElement("typefind")
	if err != nil {
		return nil, fmt.Errorf("failed to create typefind element: %v", err)
	}

	// Create tee element for multiple outputs
	tee, err := gst.NewElement("tee")
	if err != nil {
		return nil, fmt.Errorf("failed to create tee element: %v", err)
	}

	// Configure source with optimized settings
	src.Set("location", s.Settings.Src)
	src.Set("user-agent", s.Settings.UserAgent)
	src.Set("is-live", true)
	src.Set("timeout", 5)          // 5-second timeout
	src.Set("retries", 3)          // Retry 3 times on failure
	src.Set("connect-timeout", 10) // 10-second connection timeout
	src.Set("ssl-strict", false)   // Allow self-signed certificates
	src.Set("keep-alive", true)    // Use HTTP keep-alive

	// Configure tee for multiple outputs
	tee.Set("name", "stream")
	tee.Set("allow-not-linked", true) // Allow tee to work without connected sink

	// Add elements to pipeline
	pipeline.Add(src)
	pipeline.Add(typefind)
	pipeline.Add(tee)

	// Link elements
	if err := src.Link(typefind); err != nil {
		pipeline.SetState(gst.StateNull) // Clean up
		return nil, fmt.Errorf("failed to link src to typefind: %v", err)
	}

	typefind.Connect("have-type", func(self *gst.Element, guint gst.TypeFindProbability, caps *gst.Caps) {
		// Update network quality based on typefind probability
		quality := int32(guint)
		atomic.StoreInt32(&s.networkQuality, quality)

		capsStr := caps.String()
		log.Info().Msgf("Stream type detected: %s for %s", capsStr, s.Settings.Uuid)

		// Handle HLS streams
		if strings.HasPrefix(capsStr, "application/x-hls") {
			// Create HLS demuxer
			demux, err := gst.NewElement("hlsdemux")
			if err != nil {
				log.Error().Msgf("Failed to create hlsdemux: %v", err)
				msg := gst.NewErrorMessage(self, gst.NewGError(1, err), "Failed to create hlsdemux", nil)
				pipeline.GetPipelineBus().Post(msg)
				return
			}

			// Configure HLS demuxer
			demux.Set("is-live", true)
			demux.Set("max-bitrate", 0) // No limit on bitrate

			// Add demuxer to pipeline and link
			pipeline.Add(demux)
			if err := typefind.Link(demux); err != nil {
				log.Error().Msgf("Failed to link typefind to hlsdemux: %v", err)
				return
			}

			// Handle dynamic pads from HLS demuxer
			demux.Connect("pad-added", func(self *gst.Element, pad *gst.Pad) {
				sinkpad := tee.GetStaticPad("sink")
				if sinkpad == nil {
					log.Error().Msg("Failed to get tee sink pad")
					return
				}

				if ret := pad.Link(sinkpad); ret != gst.PadLinkOK {
					log.Error().Msgf("Failed to link demux to tee: %v", ret)
					return
				}

				tee.SyncStateWithParent()
			})

			demux.SyncStateWithParent()

			// Handle MPEG-TS streams
		} else if strings.HasPrefix(capsStr, "video/mpegts") {
			// Link typefind directly to tee for MPEG-TS
			sinkpad := tee.GetStaticPad("sink")
			srcpad := self.GetStaticPad("src")
			if sinkpad == nil || srcpad == nil {
				log.Error().Msg("Failed to get pads")
				return
			}

			if ret := srcpad.Link(sinkpad); ret != gst.PadLinkOK {
				log.Error().Msgf("Failed to link typefind to tee: %v", ret)
				return
			}

			// Handle DASH streams
		} else if strings.HasPrefix(capsStr, "application/dash+xml") {
			// Create DASH demuxer
			demux, err := gst.NewElement("dashdemux")
			if err != nil {
				log.Error().Msgf("Failed to create dashdemux: %v", err)
				msg := gst.NewErrorMessage(self, gst.NewGError(1, err), "Failed to create dashdemux", nil)
				pipeline.GetPipelineBus().Post(msg)
				return
			}

			// Add demuxer to pipeline and link
			pipeline.Add(demux)
			if err := typefind.Link(demux); err != nil {
				log.Error().Msgf("Failed to link typefind to dashdemux: %v", err)
				return
			}

			// Handle dynamic pads from DASH demuxer
			demux.Connect("pad-added", func(self *gst.Element, pad *gst.Pad) {
				sinkpad := tee.GetStaticPad("sink")
				if sinkpad == nil {
					log.Error().Msg("Failed to get tee sink pad")
					return
				}

				if ret := pad.Link(sinkpad); ret != gst.PadLinkOK {
					log.Error().Msgf("Failed to link dashdemux to tee: %v", ret)
					return
				}
				tee.SyncStateWithParent()
			})

			demux.SyncStateWithParent()

			// Handle unsupported stream types
		} else {
			log.Error().Msgf("Unsupported stream type: %s", capsStr)
			msg := gst.NewErrorMessage(self, gst.NewGError(1, fmt.Errorf("unsupported stream type")),
				fmt.Sprintf("Unsupported caps: %s", capsStr), nil)
			pipeline.GetPipelineBus().Post(msg)
		}
	})

	// Sync all element states with pipeline
	elements, _ := pipeline.GetElements()
	for _, e := range elements {
		e.SyncStateWithParent()
	}

	// Set up a watch on the pipeline bus for error messages
	pipeline.GetPipelineBus().AddSignalWatch()

	return pipeline, nil
}

// mainLoop runs the GStreamer main loop for the stream
func (s *Stream) mainLoop(loop *glib.MainLoop) error {
	defer close(s.hlsCleanup)

	// Start the pipeline with retry logic
	startTimeout := time.After(5 * time.Second)
	startSuccess := false

	for i := 0; i < 3; i++ {
		// Try to set pipeline state to PLAYING
		if err := s.pipeline.SetState(gst.StatePlaying); err == nil {
			log.Info().Msgf("Pipeline started for %s", s.Settings.Uuid)
			startSuccess = true
			break
		}

		log.Warn().Msgf("Failed to start pipeline, attempt %d/3", i+1)

		// Check if we've exceeded the timeout
		select {
		case <-startTimeout:
			return fmt.Errorf("timeout starting pipeline")
		default:
			// Exponential backoff with cap
			backoff := time.Duration(math.Min(1000, 100*math.Pow(2, float64(i)))) * time.Millisecond
			time.Sleep(backoff)
		}
	}

	if !startSuccess {
		return fmt.Errorf("failed to start pipeline after multiple attempts")
	}

	// Set up bus watch for pipeline messages
	s.pipeline.GetPipelineBus().AddWatch(func(msg *gst.Message) bool {
		var err error
		retryEOS := settings.APP_SETTINGS.Streaming.RetryEOS
		var retry int32 = 0 // Use atomic for thread safety

		switch msg.Type() {
		case gst.MessageError:
			gerr := msg.ParseError()
			err = gerr
			log.Error().Msgf("Pipeline error: %v, debug: %v", gerr, gerr.DebugString())

			// Try to recover from common errors
			errStr := gerr.Error()
			if strings.Contains(errStr, "Could not open resource") ||
				strings.Contains(errStr, "404") ||
				strings.Contains(errStr, "connection") {
				// Source connection error - try to restart
				currentRetry := atomic.AddInt32(&retry, 1)
				if currentRetry <= int32(retryEOS) {
					log.Warn().Msgf("Connection error, restarting (retry %d/%d)", currentRetry, retryEOS)
					s.pipeline.SetState(gst.StateNull)
					time.Sleep(time.Second)
					s.pipeline.SetState(gst.StatePlaying)
					return true // Keep watching
				}
			}

		case gst.MessageStateChanged:
			// Only log significant state changes
			_, newState := msg.ParseStateChanged()
			if newState == gst.StatePlaying {
				atomic.StoreInt32(&retry, 0) // Reset retry counter when playing
			}

		case gst.MessageEOS:
			// Handle End-of-Stream with retry logic
			currentRetry := atomic.AddInt32(&retry, 1)
			if currentRetry <= int32(retryEOS) {
				log.Info().Msgf("Received EOS, restarting (retry %d/%d)", currentRetry, retryEOS)
				s.pipeline.SetState(gst.StatePaused)
				time.Sleep(500 * time.Millisecond)
				s.pipeline.SetState(gst.StatePlaying)
			} else {
				err = fmt.Errorf("max retries reached after EOS")
			}

		case gst.MessageBuffering:
			// Handle buffering messages
			percent := msg.ParseBuffering()
			atomic.StoreInt32(&s.networkQuality, int32(percent))

			// Pause/play pipeline based on buffering state
			if percent < 100 {
				s.pipeline.SetState(gst.StatePaused)
			} else {
				s.pipeline.SetState(gst.StatePlaying)
			}

		case gst.MessageClockLost:
			// Handle clock lost by resetting pipeline clock
			log.Warn().Msg("Clock lost, resetting pipeline")
			s.pipeline.SetState(gst.StatePaused)
			time.Sleep(100 * time.Millisecond)
			s.pipeline.SetState(gst.StatePlaying)
		}

		// Handle errors by cleaning up and stopping the pipeline
		if err != nil {
			log.Error().Msgf("Fatal pipeline error: %v", err)
			loop.Quit()
			go s.Close(nil, nil)
			return false // Stop watching
		}

		return true // Continue watching
	})

	// Run the main loop
	return loop.RunError()
}

// CreateHlsDir creates the HLS directory for the stream, removing any existing directory
func (s *Stream) CreateHlsDir() error {
	// Define the HLS directory path
	hlsDir := fmt.Sprintf("%s/%s", settings.STREAM_FILEPATH, s.Settings.Uuid)

	// Check if directory already exists
	if _, err := os.Stat(hlsDir); err == nil {
		// Directory exists, try to remove it
		log.Info().Msgf("Removing existing HLS directory for %s", s.Settings.Uuid)

		// Try to remove with retry
		for i := 0; i < 3; i++ {
			if err := os.RemoveAll(hlsDir); err != nil {
				log.Warn().Msgf("Failed to remove HLS directory (attempt %d/3): %v", i+1, err)
				time.Sleep(100 * time.Millisecond) // Wait before retry
			} else {
				break // Successfully removed
			}

			// If this was the last attempt and it failed
			if i == 2 {
				return fmt.Errorf("failed to remove existing HLS directory: %s", hlsDir)
			}
		}
	}

	// Create the directory
	if err := os.MkdirAll(hlsDir, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create HLS directory: %v", err)
	}

	log.Info().Msgf("HLS directory created for %s", s.Settings.Uuid)
	return nil
}

// StartStream initializes and starts a stream for the given context
func (s *Stream) StartStream(c *fiber.Ctx) error {
	var err error
	log.Info().Msgf("Starting stream for UUID: %s, Source: %s", s.Settings.Uuid, s.Settings.Src)

	// Ensure we're starting with a clean state
	s.Mu.Lock()
	s.isClosing = false
	s.count = 0
	s.Mu.Unlock()

	// Create a main loop for GStreamer
	mainLoop := glib.NewMainLoop(glib.MainContextDefault(), false)

	// Create the pipeline
	if s.pipeline, err = s.createPipeline(); err != nil {
		return fmt.Errorf("failed to create pipeline: %v", err)
	}

	// Set up appropriate sink based on configuration
	if settings.APP_SETTINGS.Streaming.Type == "hls" || s.HlsExists {
		// Create HLS directory
		if err := s.CreateHlsDir(); err != nil {
			return err
		}

		// Create HLS sink
		if err := s.NewHLSSink(c); err != nil {
			s.Close(nil, nil) // Clean up resources
			return fmt.Errorf("failed to create HLS sink: %v", err)
		}
	} else {
		// Create MP2T sink
		if err := s.NewMP2TSink(c); err != nil {
			s.Close(nil, nil) // Clean up resources
			return fmt.Errorf("failed to create MP2T sink: %v", err)
		}
	}

	// Start the pipeline main loop in a goroutine
	go func() {
		if err := s.mainLoop(mainLoop); err != nil {
			log.Error().Msgf("Pipeline main loop error: %v", err)
		}
	}()

	// Wait briefly to catch immediate startup errors
	time.Sleep(100 * time.Millisecond)

	// Check if pipeline is still alive
	if s.pipeline == nil {
		return fmt.Errorf("pipeline failed to start")
	}

	// Update last access time
	s.Mu.Lock()
	s.LastAccess = time.Now()
	s.Mu.Unlock()

	log.Info().Msgf("Stream started successfully for %s", s.Settings.Uuid)
	return nil
}
