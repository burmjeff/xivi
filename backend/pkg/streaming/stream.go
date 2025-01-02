package streaming

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
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
	cleanupInterval = 30 * time.Second
	// Maximum time a stream can be idle before cleanup
	maxIdleTime = 60 * time.Second
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
		for _, stream := range Streams {
			stream.Mu.Lock()
			if now.Sub(stream.LastAccess) > maxIdleTime {
				log.Info().Msgf("Cleaning up idle stream: %s", stream.Settings.Uuid)
				go stream.Close(nil, nil)
			}
			stream.Mu.Unlock()
		}
	}
}

type Stream struct {
	pipeline   *gst.Pipeline
	HlsExists  bool
	hlsCleanup chan struct{}
	Settings   *Settings // The settings for the element
	count      int       // The current stream count
	LastAccess time.Time // Last endpoint hit
	Mu         sync.Mutex
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
		HlsExists:  false,
		hlsCleanup: make(chan struct{}),
		count:      0,
		LastAccess: time.Now(),
		Settings: &Settings{
			Uuid:      streamId,
			Src:       src,
			Buffer:    settings.APP_SETTINGS.Streaming.Buffer,
			UserAgent: settings.APP_SETTINGS.Streaming.UserAgent,
		},
	}
}

func AddStream(s *Stream) {
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
	for i, stream := range Streams {
		if stream == s {
			ret := make([]*Stream, 0)
			ret = append(ret, Streams[:i]...)
			Streams = append(ret, Streams[i+1:]...)
			break
		}
	}
}

// Close the stream
func (s *Stream) Close(sinkBin *gst.Bin, done chan bool) gst.FlowReturn {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	// Prevent multiple closes
	if s.count < 0 {
		return gst.FlowEOS
	}

	select {
	case <-done:
		return gst.FlowEOS
	default:
		if done != nil {
			close(done)
		}
		s.count--

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

		// Only do full pipeline cleanup if no streams are left
		if s.count <= 0 {
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
			}()
			log.Info().Msgf("Pipeline disposed for stream: %s", s.Settings.Uuid)
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

	ctx.Set(fiber.HeaderContentType, "application/x-hls")

	pRoot := fmt.Sprintf("http://%s:%d/stream/hls/%s", settings.APP_SETTINGS.Server.Host, settings.APP_SETTINGS.Server.Port, s.Settings.Uuid)
	pLocation := fmt.Sprintf("%s/%s/%s", settings.STREAM_FILEPATH, s.Settings.Uuid, "playlist.m3u8")
	location := fmt.Sprintf("%s/%s/%s", settings.STREAM_FILEPATH, s.Settings.Uuid, "segment.%05d.ts")

	// Create elements
	buffer, err := gst.NewElement("queue2")
	if err != nil {
		return err
	}
	demux, err := gst.NewElement("tsdemux")
	if err != nil {
		return err
	}
	sink, err := gst.NewElement("hlssink2")
	if err != nil {
		return err
	}

	// Create queues for audio and video
	videoQueue, err := gst.NewElement("queue")
	if err != nil {
		return err
	}
	audioQueue, err := gst.NewElement("queue")
	if err != nil {
		return err
	}

	// Convert buffer time from seconds to nanoseconds
	bufferTimeNs := uint64(s.Settings.Buffer * 1000000000)

	buffer.Set("max-size-buffers", 0)
	buffer.Set("max-size-bytes", 0)
	buffer.Set("max-size-time", bufferTimeNs)
	buffer.Set("use-buffering", true)
	buffer.Set("low-watermark", 0.01)
	buffer.Set("high-watermark", 0.99)
	buffer.Set("min-threshold-time", bufferTimeNs/4)
	buffer.Set("ring-buffer-max-size", bufferTimeNs)

	// Calculate HLS segment duration based on buffer time
	segmentDuration := 1
	if s.Settings.Buffer < 1 {
		segmentDuration = s.Settings.Buffer
	}
	segmentDurationNs := uint64(segmentDuration * 1000000000)

	// Configure HLS sink
	sink.Set("name", "hlssink")
	sink.Set("playlist-root", pRoot)
	sink.Set("playlist-location", pLocation)
	sink.Set("location", location)
	sink.Set("max-files", uint64(s.Settings.Buffer/segmentDuration+2))
	sink.Set("playlist-length", uint64(s.Settings.Buffer/segmentDuration+1))
	sink.Set("target-duration", segmentDuration)
	sink.Set("streaming", true)
	sink.Set("send-keyframe-requests", true)
	sink.Set("max-size-time", segmentDurationNs)
	sink.Set("splitmux-max-size-time", segmentDurationNs)
	sink.Set("splitmux-max-size-bytes", uint64(2*1024*1024))
	sink.Set("fragment-duration", uint64(200*1000000))
	sink.Set("playlist-type", "event")

	// Create bin
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
		log.Debug().Msgf("New pad added with caps: %s", name)

		if strings.HasPrefix(name, "audio/mpeg") {
			parseElement, err = gst.NewElement("aacparse")
			if err != nil {
				log.Error().Msgf("Failed to create aacparse: %v", err)
				return
			}
			queue = audioQueue
			sinkPad = sink.GetStaticPad("audio")
		} else if strings.HasPrefix(name, "video/x-h264") {
			parseElement, err = gst.NewElement("h264parse")
			if err != nil {
				log.Error().Msgf("Failed to create h264parse: %v", err)
				return
			}
			queue = videoQueue
			sinkPad = sink.GetStaticPad("video")
		} else {
			log.Warn().Msgf("Unknown pad type: %s", name)
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

	// Start cleanup monitor
	go func(stop chan struct{}, sinkbin *gst.Bin) {
		idleTime := 15 * time.Second
		cleanupInterval := 1 * time.Second
		ticker := time.NewTicker(cleanupInterval)
		defer ticker.Stop()

		for {
			select {
			case <-stop:
				s.HlsExists = false
				return
			case <-ticker.C:
				if time.Since(s.LastAccess) > idleTime {
					s.HlsExists = false
					s.Close(sinkbin, nil)
					return
				}
			}
		}
	}(s.hlsCleanup, bin)

	log.Info().Msgf("New HLS Stream started")
	return nil
}

func (s *Stream) NewMP2TSink(ctx *fiber.Ctx) error {
	var writer *bufio.Writer
	var done chan bool
	var writerClosed = make(chan bool)

	ctx.Set(fiber.HeaderContentType, "video/MP2T")

	if writer == nil {
		done = make(chan bool)
		ready := make(chan bool)

		// Monitor client connection
		go func() {
			<-ctx.Context().Done()
			log.Info().Msgf("Client disconnected from MP2T stream: %s", s.Settings.Uuid)
			close(writerClosed)
			s.Close(nil, done)
		}()

		ctx.Context().Response.SetBodyStreamWriter(func(w *bufio.Writer) {
			writer = w
			ready <- true // Signal that writer is set
			<-done        // Wait for stream to be closed
		})

		<-ready // Wait until writer is set
	}

	bin := gst.NewBin(fmt.Sprintf("sinkbin%d", s.count))

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

	buffer.Set("max-size-buffers", 0)
	buffer.Set("max-size-bytes", 0)
	buffer.Set("max-size-time", bufferTimeNs)
	buffer.Set("use-buffering", true)
	buffer.Set("low-watermark", 0.01)
	buffer.Set("high-watermark", 0.99)
	buffer.Set("min-threshold-time", bufferTimeNs/2)
	buffer.Set("ring-buffer-max-size", bufferTimeNs*2)

	sink.Set("max-time", bufferTimeNs)
	sink.Set("drop", true)
	sink.Set("emit-signals", true)
	sink.Set("sync", false)

	bin.Add(buffer)
	bin.Add(sink.Element)
	buffer.Link(sink.Element)

	bufPad := gst.NewGhostPad("mp2tghost", buffer.GetStaticPad("sink"))
	bufPad.SetActive(true)
	bin.AddPad(bufPad.Pad)

	sink.SetCallbacks(&gstapp.SinkCallbacks{
		NewSampleFunc: func(appSink *gstapp.Sink) gst.FlowReturn {
			select {
			case <-writerClosed:
				return s.Close(bin, done)
			default:
			}

			// Update LastAccess time when processing samples
			s.Mu.Lock()
			s.LastAccess = time.Now()
			s.Mu.Unlock()

			if appSink.IsEOS() {
				return s.Close(bin, done)
			}

			sample := appSink.TryPullSample(gst.ClockTime(30 * time.Second))
			if sample == nil {
				return s.Close(bin, done)
			}

			buffer := sample.GetBuffer()
			if buffer == nil {
				return s.Close(bin, done)
			}
			if buffer.GetSize() == 0 {
				return s.Close(bin, done)
			}
			defer buffer.Unmap()

			if _, err = s.Write(writer, buffer.Extract(0, buffer.GetSize())); err != nil {
				log.Debug().Msgf("STREAM WRITE ERROR: %v", err)
				return s.Close(bin, done)
			}
			if err = s.Flush(writer); err != nil {
				log.Debug().Msgf("STREAM FLUSH ERROR: %v", err)
				return s.Close(bin, done)
			}

			return gst.FlowOK
		},
	})

	elements, _ := bin.GetElements()
	for _, e := range elements {
		e.SyncStateWithParent()
	}

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

	log.Info().Msgf("New Stream started")

	return nil
}

func (s *Stream) createPipeline() (*gst.Pipeline, error) {
	var pipeline *gst.Pipeline
	var err error

	if s.Settings.Src == "" {
		err := errors.New("no src location configured on the httpsource")
		return nil, err
	}

	gst.Init(nil)

	pipeline, err = gst.NewPipeline("")
	if err != nil {
		return nil, err
	}

	src, err := gst.NewElement("souphttpsrc")
	if err != nil {
		return nil, err
	}
	typefind, err := gst.NewElement("typefind")
	if err != nil {
		return nil, err
	}
	tee, err := gst.NewElement("tee")
	if err != nil {
		return nil, err
	}

	src.Set("location", s.Settings.Src)
	src.Set("user-agent", s.Settings.UserAgent)
	src.Set("is-live", true)
	src.Set("timeout", 5) // Add timeout to detect connection issues faster

	tee.Set("name", "stream")
	tee.Set("allow-not-linked", true) // Allow tee to work without immediately connected sink

	pipeline.Add(src)
	pipeline.Add(typefind)
	pipeline.Add(tee)

	if err := src.Link(typefind); err != nil {
		return nil, fmt.Errorf("failed to link src to typefind: %v", err)
	}

	typefind.Connect("have-type", func(self *gst.Element, guint gst.TypeFindProbability, caps *gst.Caps) {
		log.Debug().Msgf("Stream type detected: %s", caps.String())

		if strings.HasPrefix(caps.String(), "application/x-hls") {
			demux, err := gst.NewElement("hlsdemux")
			if err != nil {
				log.Error().Msgf("HlsDemux error: %v", err)
				msg := gst.NewErrorMessage(self, gst.NewGError(1, err), "Failed to create hlsdemux", nil)
				pipeline.GetPipelineBus().Post(msg)
				return
			}

			pipeline.Add(demux)
			if err := typefind.Link(demux); err != nil {
				log.Error().Msgf("Failed to link typefind to hlsdemux: %v", err)
				return
			}

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

		} else if strings.HasPrefix(caps.String(), "video/mpegts") {
			sinkpad := tee.GetStaticPad("sink")
			if sinkpad == nil {
				log.Error().Msg("Failed to get tee sink pad")
				return
			}

			srcpad := self.GetStaticPad("src")
			if srcpad == nil {
				log.Error().Msg("Failed to get typefind src pad")
				return
			}

			if ret := srcpad.Link(sinkpad); ret != gst.PadLinkOK {
				log.Error().Msgf("Failed to link typefind to tee: %v", ret)
				return
			}
		} else {
			log.Error().Msgf("Unsupported stream type: %s", caps.String())
			msg := gst.NewErrorMessage(self, gst.NewGError(1, fmt.Errorf("unsupported stream type")),
				fmt.Sprintf("Unsupported caps: %s", caps.String()), nil)
			pipeline.GetPipelineBus().Post(msg)
		}
	})

	elements, _ := pipeline.GetElements()
	for _, e := range elements {
		e.SyncStateWithParent()
	}

	return pipeline, nil
}

func (s *Stream) mainLoop(loop *glib.MainLoop) error {
	defer close(s.hlsCleanup)

	// Start the pipeline with retry logic
	for i := 0; i < 3; i++ {
		if err := s.pipeline.SetState(gst.StatePlaying); err == nil {
			log.Debug().Msg("Pipeline successfully set to PLAYING state")
			break
		}
		log.Warn().Msgf("Failed to set pipeline state to PLAYING, attempt %d", i+1)
		time.Sleep(time.Second)
		if i == 2 {
			return fmt.Errorf("failed to start pipeline after 3 attempts")
		}
	}

	s.pipeline.GetPipelineBus().AddWatch(func(msg *gst.Message) bool {
		var err error
		retryEOS := settings.APP_SETTINGS.Streaming.Buffer
		retry := 0

		switch msg.Type() {
		case gst.MessageError:
			gerr := msg.ParseError()
			err = gerr
			log.Error().Msgf("Pipeline error: %v, debug: %v", gerr, gerr.DebugString())

			// Get more detailed element state information
			if elements, _ := s.pipeline.GetElements(); elements != nil {
				for _, element := range elements {
					state := element.GetCurrentState()
					log.Debug().Msgf("Element %s state: %v", element.GetName(), state)
				}
			}

		case gst.MessageStateChanged:
			oldState, newState := msg.ParseStateChanged()
			log.Debug().Msgf("Pipeline state changed from %v to %v", oldState, newState)

		case gst.MessageEOS:
			if retry < retryEOS {
				log.Debug().Msgf("Received EOS, attempting restart (retry %d/%d)", retry, retryEOS)
				s.pipeline.SetState(gst.StatePaused)
				s.pipeline.SetState(gst.StatePlaying)
				retry++
			} else {
				err = fmt.Errorf("max retries reached after EOS: %v", msg)
			}

		case gst.MessageBuffering:
			//percent := msg.ParseBuffering()
			//log.Debug().Msgf("Buffering: %d%%", percent)

		case gst.MessageClockLost:
			log.Warn().Msg("Clock lost, resetting pipeline")
			s.pipeline.SetState(gst.StatePaused)
			s.pipeline.SetState(gst.StatePlaying)
		}

		if err != nil {
			log.Error().Msgf("Pipeline error: %v", err)
			select {
			case _, ok := <-s.hlsCleanup:
				if ok {
					close(s.hlsCleanup)
				}
			default:
			}
			loop.Quit()
			go s.Close(nil, nil)
			return false
		}
		return true
	})

	return loop.RunError()
}

func (s *Stream) CreateHlsDir() error {
	if _, err := os.Stat(fmt.Sprintf("%s/%s", settings.STREAM_FILEPATH, s.Settings.Uuid)); err == nil {
		if err := os.RemoveAll(fmt.Sprintf("%s/%s", settings.STREAM_FILEPATH, s.Settings.Uuid)); err != nil {
			log.Debug().Msgf("FAILED TO REMOVE HLS FOLDER: %v", err)
		}
	}
	if err := os.Mkdir(fmt.Sprintf("%s/%s", settings.STREAM_FILEPATH, s.Settings.Uuid), os.ModePerm); err != nil {
		log.Error().Msgf("GST_HLS_MKDIR ERROR: %v", err)
		return fmt.Errorf("FAILED TO CREATE HLS FOLDER: %v", err)
	}
	return nil
}

func (s *Stream) StartStream(c *fiber.Ctx) error {
	var err error
	log.Debug().Msgf("Starting stream for UUID: %s, Source: %s", s.Settings.Uuid, s.Settings.Src)

	mainLoop := glib.NewMainLoop(glib.MainContextDefault(), false)

	if s.pipeline, err = s.createPipeline(); err != nil {
		log.Error().Msgf("Failed to create pipeline: %v", err)
		return err
	}
	log.Debug().Msg("Pipeline created successfully")

	if settings.APP_SETTINGS.Streaming.Type == "hls" || s.HlsExists {
		log.Debug().Msg("Setting up HLS stream")
		if err := s.CreateHlsDir(); err != nil {
			log.Error().Msgf("Failed to create HLS directory: %v", err)
			return err
		}

		if err := s.NewHLSSink(c); err != nil {
			log.Error().Msgf("NEWHLSSINK ERROR: %v", err)
			s.Close(nil, nil)
			return err
		}
		log.Debug().Msg("HLS sink created successfully")
	} else {
		log.Debug().Msg("Setting up MP2T stream")
		if err := s.NewMP2TSink(c); err != nil {
			log.Error().Msgf("NEWMP2TSINK ERROR: %v", err)
			s.Close(nil, nil)
			return err
		}
		log.Debug().Msg("MP2T sink created successfully")
	}

	go func() {
		log.Debug().Msg("Starting pipeline main loop")
		if err := s.mainLoop(mainLoop); err != nil {
			log.Error().Msgf("GST MAINLOOP ERROR!: %v", err)
		}
	}()

	// Wait a short time to catch immediate startup errors
	time.Sleep(100 * time.Millisecond)

	// Check if pipeline is still alive
	if s.pipeline == nil {
		return fmt.Errorf("pipeline failed to start")
	}

	// Log current pipeline state
	currentState := s.pipeline.GetCurrentState()
	log.Debug().Msgf("Pipeline current state: %v", currentState)

	// Update last access time
	s.Mu.Lock()
	s.LastAccess = time.Now()
	s.Mu.Unlock()

	return nil
}
