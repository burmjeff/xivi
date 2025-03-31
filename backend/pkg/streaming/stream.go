package streaming

import (
	"bufio"
	"errors"
	"fmt"
	"math"
	"os"
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
	// Global cleanup interval
	cleanupInterval = 15 * time.Second
	// Maximum time a stream can be idle before cleanup
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
				log.Info().Msgf("Cleaning up idle stream: %s (last accessed %s ago)",
					stream.Settings.Uuid, now.Sub(stream.LastAccess).Round(time.Second))
				go stream.Close(nil, nil)
			}
		}
	}
}

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
type Settings struct {
	Uuid      string
	Src       string
	Buffer    int
	UserAgent string
}

type Streamer interface {
	Write(p []byte) (n int, err error) // Write writes bytes to streamer
	Flush() error                      // Flush flushes data to the client
}

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

func AddStream(s *Stream) {
	streamsMutex.Lock()
	defer streamsMutex.Unlock()

	// Check for existing stream with same UUID
	for _, existing := range Streams {
		if existing.Settings.Uuid == s.Settings.Uuid {
			// Close existing stream before adding new one
			existing.Close(nil, nil)
			break
		}
	}
	Streams = append(Streams, s)
}

func RemoveStream(s *Stream) {
	streamsMutex.Lock()
	defer streamsMutex.Unlock()

	for i, stream := range Streams {
		if stream == s {
			// More efficient slice manipulation without creating temporary slices
			copy(Streams[i:], Streams[i+1:])
			Streams = Streams[:len(Streams)-1]
			log.Debug().Msgf("Removed stream %s, remaining streams: %d", s.Settings.Uuid, len(Streams))
			break
		}
	}
}

// Close the stream
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
	log.Debug().Msgf("Closing stream, count now: %d for UUID: %s", s.count, s.Settings.Uuid)

	// Determine if we need full cleanup
	needFullCleanup := s.count <= 0

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

	// If we're not doing full cleanup, reset the closing flag
	if !needFullCleanup {
		s.isClosing = false
	}

	s.Mu.Unlock() // Release lock before potentially long operations

	// Only do full pipeline cleanup if no streams are left
	if needFullCleanup {
		log.Debug().Msgf("Closing last stream, cleaning up pipeline for: %s", s.Settings.Uuid)
		RemoveStream(s)
		go func() {
			if s.pipeline != nil && s.pipeline.GstObject() != nil {
				// First set pipeline to NULL state
				log.Debug().Msg("Setting pipeline to NULL state")
				if err := s.pipeline.SetState(gst.StateNull); err != nil {
					log.Warn().Msgf("Failed to set pipeline to NULL state: %v", err)
				}

				// Wait a bit for state change to complete
				time.Sleep(100 * time.Millisecond)

				// Send EOS event with retry
				for i := 0; i < 3; i++ {
					if s.pipeline.SendEvent(gst.NewEOSEvent()) {
						break
					}
					log.Warn().Msg("WARNING: Failed to send EOS to pipeline, retrying...")
					time.Sleep(100 * time.Millisecond)
				}

				// Set each element to NULL state first
				if elements, _ := s.pipeline.GetElementsSorted(); elements != nil {
					for _, element := range elements {
						log.Debug().Msgf("Setting element to NULL state: %s", element.GetName())
						// Cleanup all pads first
						if pads, _ := element.GetPads(); pads != nil {
							for _, pad := range pads {
								pad.PauseTask()
							}
						}
						if err := element.SetState(gst.StateNull); err != nil {
							log.Debug().Msgf("WARNING: Failed to set %s state to Null", element.GetName())
						}
					}
				}

				// Now remove elements
				if elements, _ := s.pipeline.GetElementsSorted(); elements != nil {
					for _, element := range elements {
						log.Debug().Msgf("Removing element: %s", element.GetName())
						if err := s.pipeline.Remove(element); err != nil {
							log.Debug().Msgf("WARNING: Failed to remove element from pipeline: %s", element.GetName())
						}
					}
				}

				// Final cleanup
				s.pipeline.Clear()
				s.pipeline = nil
			}

			// Cleanup HLS directory with retry
			if _, err := os.Stat(fmt.Sprintf("%s/%s", settings.STREAM_FILEPATH, s.Settings.Uuid)); err == nil {
				for i := 0; i < 3; i++ {
					err := os.RemoveAll(fmt.Sprintf("%s/%s", settings.STREAM_FILEPATH, s.Settings.Uuid))
					if err == nil {
						break
					}
					log.Debug().Msgf("Failed to remove HLS directory, attempt %d: %v", i+1, err)
					time.Sleep(100 * time.Millisecond)
				}
			}

			// Final cleanup of stream object
			s.Mu.Lock()
			s.isClosing = false
			s.Mu.Unlock()
			log.Info().Msgf("Pipeline fully disposed for stream: %s", s.Settings.Uuid)
		}()
		return gst.FlowEOS
	}

	// If we still have active streams, just cleanup the specific sink bin
	if sinkBin != nil {
		log.Debug().Msgf("Closing stream branch, %d streams remaining", s.count)
		go func() {
			if tee, err := s.pipeline.GetElementByName("stream"); err == nil {
				// First unlink the bin to isolate it
				tee.Unlink(sinkBin.Element)

				// Set sink bin to NULL state
				if err := sinkBin.SetState(gst.StateNull); err != nil {
					log.Warn().Msgf("WARNING: Failed to set sink bin to NULL state: %v", err)
				}

				// Remove the bin from pipeline
				if err := s.pipeline.Remove(sinkBin.Element); err != nil {
					log.Warn().Msgf("WARNING: Failed to remove stream bin from pipeline: %s", sinkBin.GetName())
				}

				// Send EOS to the bin we're removing
				if !sinkBin.SendEvent(gst.NewEOSEvent()) {
					log.Warn().Msg("WARNING: Failed to send EOS to stream branch")
				}

				// Clean up the bin's elements
				if elements, _ := sinkBin.GetElementsSorted(); elements != nil {
					for _, element := range elements {
						log.Debug().Msgf("Disposing GST element: %s with state: %s", element.GetName(), element.GetCurrentState())
						if pads, _ := element.GetPads(); pads != nil {
							for _, pad := range pads {
								pad.PauseTask()
							}
						}
					}
				}

				// Clear the bin
				sinkBin.Clear()

				// Ensure remaining pipeline elements are in correct state
				if s.count > 0 {
					log.Debug().Msg("Ensuring remaining pipeline elements are playing")
					if elements, _ := s.pipeline.GetElements(); elements != nil {
						for _, element := range elements {
							name := element.GetName()
							// Skip the tee element as it's managed by GStreamer
							if name != "stream" {
								if err := element.SetState(gst.StatePlaying); err != nil {
									log.Warn().Msgf("Failed to set element %s to PLAYING: %v", name, err)
								}
							}
						}
					}
				}
			}
			log.Info().Msgf("Stream branch closed, %d streams remaining", s.count)
		}()
	}

	return gst.FlowEOS
}

// Write bytes to streamer
func (s *Stream) Write(writer *bufio.Writer, p []byte) (n int, err error) {
	if writer != nil {
		return writer.Write(p)
	}
	return 0, nil
}

// Flush data to the client
func (s *Stream) Flush(writer *bufio.Writer) error {
	if writer != nil {
		return writer.Flush()
	}
	return nil
}

func (s *Stream) NewHLSSink(ctx *fiber.Ctx) error {
	var bin *gst.Bin
	s.HlsExists = true

	ctx.Set(fiber.HeaderContentType, "application/vnd.apple.mpegurl")

	// Increment active clients counter
	atomic.AddInt32(&s.activeClients, 1)

	// Configure HLS paths
	pRoot := fmt.Sprintf("http://%s:%d/stream/hls/%s", settings.APP_SETTINGS.Server.Host, settings.APP_SETTINGS.Server.Port, s.Settings.Uuid)
	pLocation := fmt.Sprintf("%s/%s/%s", settings.STREAM_FILEPATH, s.Settings.Uuid, "playlist.m3u8")
	location := fmt.Sprintf("%s/%s/%s", settings.STREAM_FILEPATH, s.Settings.Uuid, "segment.%05d.ts")

	// Create elements with error handling
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

	// Create queues for audio and video with improved settings
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
	videoQueue.Set("leaky", 2) // Downstream leaky queue for smoother playback

	audioQueue.Set("max-size-buffers", 0)
	audioQueue.Set("max-size-bytes", 0)
	audioQueue.Set("max-size-time", uint64(2*time.Second))
	audioQueue.Set("leaky", 2) // Downstream leaky queue for smoother playback

	// Convert buffer time from seconds to nanoseconds
	bufferTimeNs := uint64(s.Settings.Buffer * 1000000000)

	// Configure main buffer with optimized settings for faster startup
	buffer.Set("max-size-buffers", 0)
	buffer.Set("max-size-bytes", 0)
	buffer.Set("max-size-time", bufferTimeNs)
	buffer.Set("use-buffering", true)
	buffer.Set("low-watermark", 0.01)                  // Lower watermark for faster startup
	buffer.Set("high-watermark", 0.99)                 // Higher watermark for faster startup
	buffer.Set("min-threshold-time", bufferTimeNs/10)  // Lower threshold for faster startup
	buffer.Set("ring-buffer-max-size", bufferTimeNs*2) // Smaller ring buffer for faster startup

	// Use shorter segment duration for faster startup
	// Shorter segments mean faster initial playlist generation
	segmentDuration := 1 // Use 1-second segments for faster startup
	// For very large buffers, we can use slightly longer segments
	if s.Settings.Buffer > 8 {
		segmentDuration = 2
	}
	segmentDurationNs := uint64(segmentDuration * 1000000000)

	// Configure HLS sink with optimized settings for faster startup
	sink.Set("name", "hlssink")
	sink.Set("playlist-root", pRoot)
	sink.Set("playlist-location", pLocation)
	sink.Set("location", location)
	// Optimize for faster startup
	sink.Set("max-files", uint64(s.Settings.Buffer/segmentDuration+3))       // Keep more segments
	sink.Set("playlist-length", uint64(s.Settings.Buffer/segmentDuration+2)) // Longer playlist
	sink.Set("target-duration", segmentDuration)                             // Shorter segments
	sink.Set("streaming", true)                                              // Enable streaming mode
	sink.Set("send-keyframe-requests", true)                                 // Request keyframes for better segment boundaries
	sink.Set("max-size-time", segmentDurationNs)                             // Maximum segment duration
	sink.Set("splitmux-max-size-time", segmentDurationNs)                    // Split at segment boundaries
	sink.Set("splitmux-max-size-bytes", uint64(2*1024*1024))                 // Smaller segments for faster generation
	sink.Set("fragment-duration", uint64(100*1000000))                       // Smaller fragments for faster startup
	sink.Set("playlist-type", "live")                                        // Live playlist type
	// Add new settings for faster startup
	sink.Set("min-fragment-duration", uint64(50*1000000)) // Minimum fragment duration (50ms)
	sink.Set("render-delay", uint64(0))                   // No render delay
	sink.Set("max-header-size", uint64(512*1024))         // Smaller headers

	// Create bin with unique name
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

	// Handle dynamic pads from demuxer with improved codec support
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
		log.Debug().Msgf("New pad added with caps: %s", name)

		// Handle different audio codecs
		if strings.HasPrefix(name, "audio/mpeg") {
			parseElement, err = gst.NewElement("aacparse")
			if err != nil {
				log.Error().Msgf("Failed to create aacparse: %v", err)
				return
			}
			queue = audioQueue
			sinkPad = sink.GetRequestPad("audio")
		} else if strings.HasPrefix(name, "audio/x-ac3") {
			parseElement, err = gst.NewElement("ac3parse")
			if err != nil {
				log.Error().Msgf("Failed to create ac3parse: %v", err)
				return
			}
			queue = audioQueue
			sinkPad = sink.GetRequestPad("audio")
			// Handle different video codecs
		} else if strings.HasPrefix(name, "video/x-h264") {
			parseElement, err = gst.NewElement("h264parse")
			if err != nil {
				log.Error().Msgf("Failed to create h264parse: %v", err)
				return
			}
			queue = videoQueue
			sinkPad = sink.GetRequestPad("video")
		} else if strings.HasPrefix(name, "video/x-h265") {
			parseElement, err = gst.NewElement("h265parse")
			if err != nil {
				log.Error().Msgf("Failed to create h265parse: %v", err)
				return
			}
			queue = videoQueue
			sinkPad = sink.GetRequestPad("video")
		} else {
			log.Warn().Msgf("Unknown pad type: %s", name)
			return
		}

		bin.Add(parseElement)
		parseElement.SyncStateWithParent()

		// Link elements with better error handling
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
			log.Error().Msgf("Failed to link queuepad to sinkpad: %v", ret)
			return
		}

		if sinkPad != nil {
			log.Debug().Msgf("Successfully linked %s stream with pad %s", name, sinkPad.GetName())
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

	// Start cleanup monitor with improved logic
	go func(stop chan struct{}, sinkbin *gst.Bin) {
		idleTime := 30 * time.Second       // Increased idle time for better user experience
		cleanupInterval := 5 * time.Second // Less frequent checks to reduce overhead
		ticker := time.NewTicker(cleanupInterval)
		defer ticker.Stop()
		defer atomic.AddInt32(&s.activeClients, -1) // Ensure client counter is decremented

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
					log.Debug().Msgf("HLS stream idle for %s, cleaning up", idleTime)
					s.HlsExists = false
					s.Close(sinkbin, nil)
					return
				}
			}
		}
	}(s.hlsCleanup, bin)

	log.Info().Msgf("New HLS Stream started for %s", s.Settings.Uuid)
	return nil
}

func (s *Stream) NewMP2TSink(ctx *fiber.Ctx) error {
	var writer *bufio.Writer
	var done chan bool
	var writerClosed = make(chan bool)
	var heartbeatTicker *time.Ticker
	var heartbeatDone = make(chan bool)

	ctx.Set(fiber.HeaderContentType, "video/MP2T")

	if writer == nil {
		done = make(chan bool)
		ready := make(chan bool)

		// Increment active clients counter
		atomic.AddInt32(&s.activeClients, 1)

		// Monitor client connection with improved detection
		go func() {
			defer func() {
				atomic.AddInt32(&s.activeClients, -1) // Decrement counter on exit
				close(writerClosed)
				s.Close(nil, done)
				if heartbeatTicker != nil {
					heartbeatTicker.Stop()
					close(heartbeatDone)
				}
			}()

			// Wait for context done (client disconnection)
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
						// Send a small heartbeat packet
						if writer != nil {
							// Try to write a minimal TS packet to keep connection alive
							// This helps detect broken connections faster
							err := writer.Flush()
							if err != nil {
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
	}

	bin := gst.NewBin(fmt.Sprintf("sinkbin%d", s.count))

	// Create a more robust buffering setup
	buffer, err := gst.NewElement("queue2")
	if err != nil {
		return err
	}
	sink, err := gstapp.NewAppSink()
	if err != nil {
		return err
	}

	// Convert buffer time from seconds to nanoseconds
	bufferTimeNs := uint64(s.Settings.Buffer * 1000000000)

	// Configure buffer with optimized settings
	buffer.Set("max-size-buffers", 0)
	buffer.Set("max-size-bytes", 0)
	buffer.Set("max-size-time", bufferTimeNs)
	buffer.Set("use-buffering", true)
	buffer.Set("low-watermark", 0.05)                  // Increased from 0.01 for better stability
	buffer.Set("high-watermark", 0.95)                 // Decreased from 0.99 for better stability
	buffer.Set("min-threshold-time", bufferTimeNs/3)   // Better threshold for smoother playback
	buffer.Set("ring-buffer-max-size", bufferTimeNs*3) // Increased for better buffering

	// Configure sink with optimized settings
	sink.Set("max-time", bufferTimeNs)
	sink.Set("drop", true)                                // Drop late buffers
	sink.Set("emit-signals", true)                        // Emit signals for callbacks
	sink.Set("sync", false)                               // Don't sync to clock for streaming
	sink.Set("max-lateness", int64(500*time.Millisecond)) // Allow some lateness
	sink.Set("qos", true)                                 // Enable quality of service

	bin.Add(buffer)
	bin.Add(sink.Element)
	buffer.Link(sink.Element)

	bufPad := gst.NewGhostPad("mp2tghost", buffer.GetStaticPad("sink"))
	bufPad.SetActive(true)
	bin.AddPad(bufPad.Pad)

	// Setup callbacks with improved error handling
	sink.SetCallbacks(&gstapp.SinkCallbacks{
		NewSampleFunc: func(appSink *gstapp.Sink) gst.FlowReturn {
			// Check if writer is closed
			select {
			case <-writerClosed:
				return s.Close(bin, done)
			default:
			}

			// Update LastAccess time when processing samples
			s.Mu.Lock()
			s.LastAccess = time.Now()
			s.Mu.Unlock()

			// Check for EOS
			if appSink.IsEOS() {
				log.Debug().Msg("EOS received from sink")
				return s.Close(bin, done)
			}

			// Try to pull a sample with timeout
			sample := appSink.TryPullSample(gst.ClockTime(10 * time.Second))
			if sample == nil {
				log.Debug().Msg("Failed to pull sample from sink")
				return s.Close(bin, done)
			}

			// Get buffer from sample
			buffer := sample.GetBuffer()
			if buffer == nil {
				log.Debug().Msg("Received sample with nil buffer")
				return s.Close(bin, done)
			}

			// Check buffer size
			if buffer.GetSize() == 0 {
				log.Debug().Msg("Received empty buffer")
				return s.Close(bin, done)
			}

			// Extract data and clean up
			data := buffer.Extract(0, buffer.GetSize())
			defer buffer.Unmap()

			// Write data to client
			if _, err = s.Write(writer, data); err != nil {
				log.Debug().Msgf("Stream write error: %v", err)
				return s.Close(bin, done)
			}

			// Flush data to client
			if err = s.Flush(writer); err != nil {
				log.Debug().Msgf("Stream flush error: %v", err)
				return s.Close(bin, done)
			}

			return gst.FlowOK
		},
		EOSFunc: func(sink *gstapp.Sink) {
			log.Debug().Msg("EOS callback received")
			s.Close(bin, done)
		},
	})

	// Sync element states
	elements, _ := bin.GetElements()
	for _, e := range elements {
		e.SyncStateWithParent()
	}

	// Get tee element
	tee, err := s.pipeline.GetElementByName("stream")
	if err != nil {
		bin.SetState(gst.StateNull)
		bin.Clear()
		return err
	}

	// Add bin to pipeline
	s.pipeline.Add(bin.Element)

	// Link bin to tee
	if err := tee.Link(bin.Element); err != nil {
		s.pipeline.Remove(bin.Element)
		bin.SetState(gst.StateNull)
		bin.Clear()
		return err
	}

	// Sync bin state with pipeline
	bin.SyncStateWithParent()
	s.count++

	log.Info().Msgf("New MP2T Stream started for %s", s.Settings.Uuid)

	return nil
}

func (s *Stream) createPipeline() (*gst.Pipeline, error) {
	var pipeline *gst.Pipeline
	var err error

	// Validate source URL
	if s.Settings.Src == "" {
		return nil, errors.New("no source URL configured for stream")
	}

	// Initialize GStreamer if needed
	gst.Init(nil)

	// Create a new pipeline with a unique name based on UUID
	pipelineName := fmt.Sprintf("pipeline-%s", s.Settings.Uuid)
	pipeline, err = gst.NewPipeline(pipelineName)
	if err != nil {
		return nil, fmt.Errorf("failed to create pipeline: %v", err)
	}

	// Create source element with improved error handling
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
	src.Set("timeout", 5)          // 5-second timeout to detect connection issues faster
	src.Set("retries", 3)          // Retry 3 times on connection failure
	src.Set("connect-timeout", 10) // 10-second connection timeout
	src.Set("ssl-strict", false)   // Allow self-signed certificates
	src.Set("keep-alive", true)    // Use HTTP keep-alive for better performance

	// Configure tee for multiple outputs
	tee.Set("name", "stream")
	tee.Set("allow-not-linked", true) // Allow tee to work without immediately connected sink

	// Add elements to pipeline
	pipeline.Add(src)
	pipeline.Add(typefind)
	pipeline.Add(tee)

	// Link elements with better error handling
	if err := src.Link(typefind); err != nil {
		pipeline.SetState(gst.StateNull) // Clean up
		return nil, fmt.Errorf("failed to link src to typefind: %v", err)
	}

	typefind.Connect("have-type", func(self *gst.Element, guint gst.TypeFindProbability, caps *gst.Caps) {
		// Log stream type with probability
		log.Debug().Msgf("Stream type detected: %s (probability: %d%%)", caps.String(), guint)

		// Update network quality based on typefind probability
		quality := int32(guint)
		atomic.StoreInt32(&s.networkQuality, quality)

		// Handle HLS streams
		if strings.HasPrefix(caps.String(), "application/x-hls") {
			log.Info().Msgf("HLS stream detected for %s", s.Settings.Uuid)

			// Create HLS demuxer with error handling
			demux, err := gst.NewElement("hlsdemux")
			if err != nil {
				log.Error().Msgf("Failed to create hlsdemux: %v", err)
				msg := gst.NewErrorMessage(self, gst.NewGError(1, err), "Failed to create hlsdemux", nil)
				pipeline.GetPipelineBus().Post(msg)
				return
			}

			// Configure HLS demuxer for better performance
			demux.Set("is-live", true)
			demux.Set("max-bitrate", 0) // No limit on bitrate

			// Add demuxer to pipeline
			pipeline.Add(demux)
			if err := typefind.Link(demux); err != nil {
				log.Error().Msgf("Failed to link typefind to hlsdemux: %v", err)
				return
			}

			// Handle dynamic pads from HLS demuxer
			demux.Connect("pad-added", func(self *gst.Element, pad *gst.Pad) {
				// Get tee sink pad
				sinkpad := tee.GetStaticPad("sink")
				if sinkpad == nil {
					log.Error().Msg("Failed to get tee sink pad")
					return
				}

				// Link demuxer to tee
				if ret := pad.Link(sinkpad); ret != gst.PadLinkOK {
					log.Error().Msgf("Failed to link demux to tee: %v", ret)
					return
				}

				// Sync state
				tee.SyncStateWithParent()
				log.Debug().Msg("Successfully linked HLS demuxer to tee")
			})

			// Sync demuxer state with pipeline
			demux.SyncStateWithParent()

			// Handle MPEG-TS streams
		} else if strings.HasPrefix(caps.String(), "video/mpegts") {
			log.Info().Msgf("MPEG-TS stream detected for %s", s.Settings.Uuid)

			// Get tee sink pad
			sinkpad := tee.GetStaticPad("sink")
			if sinkpad == nil {
				log.Error().Msg("Failed to get tee sink pad")
				return
			}

			// Get typefind src pad
			srcpad := self.GetStaticPad("src")
			if srcpad == nil {
				log.Error().Msg("Failed to get typefind src pad")
				return
			}

			// Link typefind directly to tee for MPEG-TS
			if ret := srcpad.Link(sinkpad); ret != gst.PadLinkOK {
				log.Error().Msgf("Failed to link typefind to tee: %v", ret)
				return
			}
			log.Debug().Msg("Successfully linked MPEG-TS stream to tee")

			// Handle DASH streams
		} else if strings.HasPrefix(caps.String(), "application/dash+xml") {
			log.Info().Msgf("DASH stream detected for %s", s.Settings.Uuid)

			// Create DASH demuxer
			demux, err := gst.NewElement("dashdemux")
			if err != nil {
				log.Error().Msgf("Failed to create dashdemux: %v", err)
				msg := gst.NewErrorMessage(self, gst.NewGError(1, err), "Failed to create dashdemux", nil)
				pipeline.GetPipelineBus().Post(msg)
				return
			}

			// Add demuxer to pipeline
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
				log.Debug().Msg("Successfully linked DASH demuxer to tee")
			})

			demux.SyncStateWithParent()

			// Handle unsupported stream types
		} else {
			log.Error().Msgf("Unsupported stream type: %s", caps.String())
			msg := gst.NewErrorMessage(self, gst.NewGError(1, fmt.Errorf("unsupported stream type")),
				fmt.Sprintf("Unsupported caps: %s", caps.String()), nil)
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

	// Log pipeline creation success
	log.Info().Msgf("Pipeline created successfully for stream: %s", s.Settings.Uuid)

	return pipeline, nil
}

func (s *Stream) mainLoop(loop *glib.MainLoop) error {
	defer close(s.hlsCleanup)

	// Start the pipeline with improved retry logic optimized for faster startup
	startTimeout := time.After(5 * time.Second) // Reduced overall timeout for faster startup
	startSuccess := false

	for i := 0; i < 3; i++ { // Reduced retry attempts for faster startup
		// Try to set pipeline state to PLAYING
		if err := s.pipeline.SetState(gst.StatePlaying); err == nil {
			log.Info().Msgf("Pipeline successfully set to PLAYING state for %s", s.Settings.Uuid)
			startSuccess = true
			break
		}

		log.Warn().Msgf("Failed to set pipeline state to PLAYING, attempt %d/3", i+1)

		// Check if we've exceeded the overall timeout
		select {
		case <-startTimeout:
			return fmt.Errorf("timeout exceeded while trying to start pipeline")
		default:
			// Wait with shorter backoff (100ms, 200ms, 400ms)
			backoff := time.Duration(100*math.Pow(2, float64(i))) * time.Millisecond
			if backoff > 1*time.Second {
				backoff = 1 * time.Second // Cap at 1 second
			}
			time.Sleep(backoff)
		}
	}

	if !startSuccess {
		return fmt.Errorf("failed to start pipeline after multiple attempts")
	}

	// Set up improved bus watch with better error handling
	s.pipeline.GetPipelineBus().AddWatch(func(msg *gst.Message) bool {
		var err error
		retryEOS := settings.APP_SETTINGS.Streaming.RetryEOS // Use configured retry count
		var retry int32 = 0                                  // Use atomic for thread safety

		switch msg.Type() {
		case gst.MessageError:
			gerr := msg.ParseError()
			err = gerr

			// Log detailed error information
			log.Error().Msgf("Pipeline error for %s: %v, debug: %v",
				s.Settings.Uuid, gerr, gerr.DebugString())

			// Get detailed element state information for debugging
			if elements, _ := s.pipeline.GetElements(); elements != nil {
				for _, element := range elements {
					state := element.GetCurrentState()
					log.Debug().Msgf("Element %s state: %v", element.GetName(), state)
				}
			}

			// Try to recover from some common errors
			errStr := gerr.Error()
			if strings.Contains(errStr, "Could not open resource") ||
				strings.Contains(errStr, "404") ||
				strings.Contains(errStr, "connection") {
				// Source connection error - try to restart if not too many retries
				currentRetry := atomic.AddInt32(&retry, 1)
				if currentRetry <= int32(retryEOS) {
					log.Warn().Msgf("Connection error, attempting restart (retry %d/%d)",
						currentRetry, retryEOS)

					// Reset pipeline and try again
					s.pipeline.SetState(gst.StateNull)
					time.Sleep(time.Second)
					s.pipeline.SetState(gst.StatePlaying)
					return true // Keep watching
				}
			}

		case gst.MessageStateChanged:
			// Only log state changes for the pipeline itself (not elements)
			// Skip detailed logging for element state changes to reduce noise
			oldState, newState := msg.ParseStateChanged()
			// Log state changes
			log.Debug().Msgf("Pipeline state changed from %v to %v for %s",
				oldState, newState, s.Settings.Uuid)

			// If we reached PLAYING state, reset retry counter
			if newState == gst.StatePlaying {
				atomic.StoreInt32(&retry, 0)
			}

		case gst.MessageEOS:
			// Handle End-of-Stream with retry logic
			currentRetry := atomic.AddInt32(&retry, 1)
			if currentRetry <= int32(retryEOS) {
				log.Debug().Msgf("Received EOS, attempting restart (retry %d/%d)",
					currentRetry, retryEOS)

				// Reset pipeline and try again
				s.pipeline.SetState(gst.StatePaused)
				time.Sleep(500 * time.Millisecond)
				s.pipeline.SetState(gst.StatePlaying)
			} else {
				err = fmt.Errorf("max retries reached after EOS")
			}

		case gst.MessageBuffering:
			// Handle buffering messages for adaptive streaming
			percent := msg.ParseBuffering()

			// Update network quality based on buffering percentage
			atomic.StoreInt32(&s.networkQuality, int32(percent))

			// Only log significant changes to reduce noise
			if percent%10 == 0 {
				log.Debug().Msgf("Buffering: %d%% for %s", percent, s.Settings.Uuid)
			}

			// Pause/play pipeline based on buffering state for smoother playback
			if percent < 100 {
				s.pipeline.SetState(gst.StatePaused)
			} else {
				s.pipeline.SetState(gst.StatePlaying)
			}

		case gst.MessageClockLost:
			// Handle clock lost by resetting pipeline clock
			log.Warn().Msgf("Clock lost for %s, resetting pipeline", s.Settings.Uuid)
			s.pipeline.SetState(gst.StatePaused)
			time.Sleep(100 * time.Millisecond)
			s.pipeline.SetState(gst.StatePlaying)

		default:
			// Handle other message types silently
		}

		// Handle errors by cleaning up and stopping the pipeline
		if err != nil {
			log.Error().Msgf("Fatal pipeline error for %s: %v", s.Settings.Uuid, err)

			// Clean up HLS cleanup channel if needed
			select {
			case _, ok := <-s.hlsCleanup:
				if ok {
					close(s.hlsCleanup)
				}
			default:
			}

			// Stop the main loop and clean up the stream
			loop.Quit()
			go s.Close(nil, nil)
			return false // Stop watching
		}

		return true // Continue watching
	})

	// Run the main loop and return any errors
	return loop.RunError()
}

func (s *Stream) CreateHlsDir() error {
	// Define the HLS directory path
	hlsDir := fmt.Sprintf("%s/%s", settings.STREAM_FILEPATH, s.Settings.Uuid)

	// Check if directory already exists
	if _, err := os.Stat(hlsDir); err == nil {
		// Directory exists, try to remove it
		log.Debug().Msgf("Removing existing HLS directory for %s", s.Settings.Uuid)

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
				log.Error().Msgf("Failed to remove existing HLS directory after multiple attempts: %s", hlsDir)
				return fmt.Errorf("failed to remove existing HLS directory: %s", hlsDir)
			}
		}
	}

	// Create the directory
	log.Debug().Msgf("Creating HLS directory for %s", s.Settings.Uuid)
	if err := os.MkdirAll(hlsDir, os.ModePerm); err != nil {
		log.Error().Msgf("Failed to create HLS directory: %v", err)
		return fmt.Errorf("failed to create HLS directory: %v", err)
	}

	log.Debug().Msgf("HLS directory created successfully: %s", hlsDir)
	return nil
}

func (s *Stream) StartStream(c *fiber.Ctx) error {
	var err error
	log.Info().Msgf("Starting stream for UUID: %s, Source: %s", s.Settings.Uuid, s.Settings.Src)

	// Create a main loop for GStreamer
	mainLoop := glib.NewMainLoop(glib.MainContextDefault(), false)

	// Create the pipeline with error handling
	if s.pipeline, err = s.createPipeline(); err != nil {
		log.Error().Msgf("Failed to create pipeline for %s: %v", s.Settings.Uuid, err)
		return fmt.Errorf("failed to create pipeline: %v", err)
	}
	log.Debug().Msgf("Pipeline created successfully for %s", s.Settings.Uuid)

	// Set up appropriate sink based on configuration
	if settings.APP_SETTINGS.Streaming.Type == "hls" || s.HlsExists {
		log.Debug().Msgf("Setting up HLS stream for %s", s.Settings.Uuid)

		// Create HLS directory
		if err := s.CreateHlsDir(); err != nil {
			log.Error().Msgf("Failed to create HLS directory for %s: %v", s.Settings.Uuid, err)
			return fmt.Errorf("failed to create HLS directory: %v", err)
		}

		// Create HLS sink
		if err := s.NewHLSSink(c); err != nil {
			log.Error().Msgf("Failed to create HLS sink for %s: %v", s.Settings.Uuid, err)
			s.Close(nil, nil) // Clean up resources
			return fmt.Errorf("failed to create HLS sink: %v", err)
		}
		log.Debug().Msgf("HLS sink created successfully for %s", s.Settings.Uuid)
	} else {
		log.Debug().Msgf("Setting up MP2T stream for %s", s.Settings.Uuid)

		// Create MP2T sink
		if err := s.NewMP2TSink(c); err != nil {
			log.Error().Msgf("Failed to create MP2T sink for %s: %v", s.Settings.Uuid, err)
			s.Close(nil, nil) // Clean up resources
			return fmt.Errorf("failed to create MP2T sink: %v", err)
		}
		log.Debug().Msgf("MP2T sink created successfully for %s", s.Settings.Uuid)
	}

	// Start the pipeline main loop in a goroutine
	go func() {
		log.Debug().Msgf("Starting pipeline main loop for %s", s.Settings.Uuid)
		if err := s.mainLoop(mainLoop); err != nil {
			log.Error().Msgf("Pipeline main loop error for %s: %v", s.Settings.Uuid, err)
		}
	}()

	// Wait a shorter time to catch immediate startup errors
	startupWait := 100 * time.Millisecond // Reduced for faster startup
	time.Sleep(startupWait)

	// Check if pipeline is still alive
	if s.pipeline == nil {
		return fmt.Errorf("pipeline failed to start for %s", s.Settings.Uuid)
	}

	// Log current pipeline state
	currentState := s.pipeline.GetCurrentState()
	log.Debug().Msgf("Pipeline current state for %s: %v", s.Settings.Uuid, currentState)

	// Update last access time
	s.Mu.Lock()
	s.LastAccess = time.Now()
	s.Mu.Unlock()

	log.Info().Msgf("Stream started successfully for %s", s.Settings.Uuid)
	return nil
}
