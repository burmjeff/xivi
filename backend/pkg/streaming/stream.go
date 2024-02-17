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

var Streams []*Stream

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
		Settings: &Settings{
			Uuid:      streamId,
			Src:       src,
			Buffer:    settings.APP_SETTINGS.Streaming.Buffer,
			UserAgent: settings.APP_SETTINGS.Streaming.UserAgent,
		},
	}
}

func AddStream(s *Stream) {
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
	return
}

// Close the stream
func (s *Stream) Close(sinkBin *gst.Bin, done chan bool) gst.FlowReturn {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	select {
	case <-done:
		return gst.FlowEOS
	default:
		if done != nil {
			close(done)
		}
		s.count--

		if s.count <= 0 || sinkBin == nil {
			RemoveStream(s)
			go func() {
				if s.pipeline != nil {
					if !s.pipeline.SendEvent(gst.NewEOSEvent()) {
						log.Warn().Msg("WARNING: Failed to send EOS to pipeline")
					}
					s.pipeline.SetState(gst.StateNull)

					if elements, _ := s.pipeline.GetElementsSorted(); elements != nil {
						for _, element := range elements {
							log.Debug().Msgf("Disposing GST element: %s", element.GetName())
							pads, _ := element.GetSrcPads()
							for _, pad := range pads {
								pad.PauseTask()
							}
							if err := element.SetState(gst.StateNull); err != nil {
								log.Debug().Msgf("WARNING: Failed to set %s state to Null", element.GetName())
							}
							if err := s.pipeline.Remove(element); err != nil {
								log.Debug().Msgf("WARNING: Failed to remove element from pipeline: %s", element.GetName())
							}
						}
					}
					s.pipeline.Clear()
				}
				if _, err := os.Stat(fmt.Sprintf("%s/%s", settings.STREAM_FILEPATH, s.Settings.Uuid)); err == nil {
					err := os.RemoveAll(fmt.Sprintf("%s/%s", settings.STREAM_FILEPATH, s.Settings.Uuid))
					if err != nil {
						log.Debug().Msgf("Dispose pipeline: %v", err)
					}
				}
			}()
			log.Debug().Msg("PIPELINE DISPOSED")
			return gst.FlowEOS
		}

		go func() {
			if tee, err := s.pipeline.GetElementByName("stream"); err == nil {
				tee.Unlink(sinkBin.Element)
			}
			if err := s.pipeline.Remove(sinkBin.Element); err != nil {
				log.Warn().Msgf("WARNING: Failed to remove stream bin from pipeline: %s", sinkBin.GetName())
			}
			if !sinkBin.SendEvent(gst.NewEOSEvent()) {
				log.Warn().Msg("WARNING: Failed to send EOS to stream branch")
			}
			if elements, _ := sinkBin.GetElementsSorted(); elements != nil {
				for _, element := range elements {
					log.Debug().Msgf("Disposing GST element: %s with state: %s", element.GetName(), element.GetCurrentState())
					pads, _ := element.GetPads()
					for _, pad := range pads {
						pad.PauseTask()
					}
				}
			}
			if err := sinkBin.SetState(gst.StateNull); err != nil {
				log.Warn().Msgf("WARNING: Failed to set %s state to Null", sinkBin.GetName())
			}

			sinkBin.Clear()
			log.Info().Msgf("STREAM CLOSED")
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
	s.HlsExists = true

	ctx.Set(fiber.HeaderContentType, "application/x-hls")

	pRoot := fmt.Sprintf("http://%s:%d/stream/hls/%s", settings.APP_SETTINGS.Server.Host, settings.APP_SETTINGS.Server.Port, s.Settings.Uuid)
	pLocation := fmt.Sprintf("%s/%s/%s", settings.STREAM_FILEPATH, s.Settings.Uuid, "playlist.m3u8")
	location := fmt.Sprintf("%s/%s/%s", settings.STREAM_FILEPATH, s.Settings.Uuid, "segment.%05d.ts")

	//max-files=8 playlist-length=4 target-duration=8
	bin, _ := gst.NewBinFromString(fmt.Sprintf("queue name=hlsqueue ! tsdemux name=demux ! h264parse ! queue ! hlssink2 playlist-root=%s location=%s playlist-location=%s max-files=10 playlist-length=5 target-duration=8 name=hlssink demux. ! aacparse ! queue ! hlssink.audio", pRoot, location, pLocation), false)

	queue, err := bin.GetElementByName("hlsqueue")
	if err != nil {
		bin.SetState(gst.StateNull)
		bin.Clear()
		return err
	}
	queuepad := gst.NewGhostPad("hlsghost", queue.GetStaticPad("sink"))
	queuepad.SetActive(true)
	bin.AddPad(queuepad.Pad)

	/*
		buffer, err := gst.NewElement("queue")
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

		buffer.Set("max-size-buffers", 0)
		buffer.Set("max-size-bytes", 0)
		buffer.Set("max-size-time", 15000000000)
		buffer.Set("min-threshold-time", s.Settings.Buffer*1000000)

		demux.Set("name", "demux")

		sink.Set("name", "sink")
		sink.Set("playlist-root", pRoot)
		sink.Set("playlist-location", pLocation)
		sink.Set("location", location)
		sink.Set("max-files", 8)
		sink.Set("playlist-length", 5)
		sink.Set("target-duration", 7)

		for i := 0; i <= s.count; i++ {
			if element, _ := s.pipeline.GetElementByName(fmt.Sprintf("sinkbin%d", i)); element == nil {
				bin = gst.NewBin(fmt.Sprintf("sinkbin%d", i))
				break
			}
		}

		bin.Add(buffer)
		bin.Add(demux)
		bin.Add(sink)

		bufPad := gst.NewGhostPad("ghost", buffer.GetStaticPad("sink"))
		bufPad.SetActive(true)
		bin.AddPad(bufPad.Pad)

		buffer.Link(demux)

		demux.Connect("pad-added", func(self *gst.Element, srcPad *gst.Pad) {
			// Try to detect whether this is video or audio
			var isAudio, isVideo bool
			var cap string
			caps := srcPad.GetCurrentCaps()
			for i := 0; i < caps.GetSize(); i++ {
				st := caps.GetStructureAt(i)
				cap = st.Name()
				if strings.HasPrefix(cap, "audio/") {
					isAudio = true
				}
				if strings.HasPrefix(cap, "video/") {
					isVideo = true
				}
			}

			if !isAudio && !isVideo {
				err := errors.New("could not detect media stream type")
				// We can send errors directly to the pipeline bus if they occur.
				// These will be handled downstream.
				msg := gst.NewErrorMessage(self, gst.NewGError(1, err), fmt.Sprintf("Received caps: %s", caps.String()), nil)
				s.pipeline.GetPipelineBus().Post(msg)
				return
			}

			if isAudio {
				log.Debug().Msgf("New audio pad added, is_audio=%v", cap)

				elements, err := gst.NewElementMany("aacparse", "queue")
				if err != nil {
					// We can create custom errors (with optional structures) and send them to the pipeline bus.
					// The first argument reflects the source of the error, the second is the error itself, followed by a debug string.
					msg := gst.NewErrorMessage(self, gst.NewGError(2, err), "Could not create elements for audio pipeline", nil)
					s.pipeline.GetPipelineBus().Post(msg)
					return
				}
				bin.AddMany(elements...)
				gst.ElementLinkMany(elements...)

				elements[1].Connect("pad-added", func(self *gst.Element, queuePad *gst.Pad) {
					hlsSinkPad := sink.GetStaticPad("audio")
					queuePad.Link(hlsSinkPad)
					sink.SyncStateWithParent()
				})

				//parser := elements[0]
				//sinkPad := parser.GetStaticPad("sink")
				//srcPad.Link(sinkPad)
				self.Link(elements[0])

				for _, e := range elements {
					e.SyncStateWithParent()
				}

			} else if isVideo {
				log.Debug().Msgf("New video pad added, is_video=%v", cap)

				elements, err := gst.NewElementMany("h264parse", "queue")
				if err != nil {
					msg := gst.NewErrorMessage(self, gst.NewGError(2, err), "Could not create elements for video pipeline", nil)
					s.pipeline.GetPipelineBus().Post(msg)
					return
				}
				bin.AddMany(elements...)
				gst.ElementLinkMany(elements...)

				elements[1].Connect("pad-added", func(self *gst.Element, queuePad *gst.Pad) {
					hlsSinkPad := sink.GetStaticPad("video")
					queuePad.Link(hlsSinkPad)
					sink.SyncStateWithParent()
				})

				//parser := elements[0]
				//sinkPad := parser.GetStaticPad("sink")
				//srcPad.Link(sinkPad)
				self.Link(elements[0])

				for _, e := range elements {
					e.SyncStateWithParent()
				}
			}
		})
	*/

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

	log.Info().Msgf("New Stream started")

	return nil
}

func (s *Stream) NewMP2TSink(ctx *fiber.Ctx) error {
	var writer *bufio.Writer
	var done chan bool

	ctx.Set(fiber.HeaderContentType, "video/MP2T")

	if writer == nil {
		done = make(chan bool)
		ready := make(chan bool)
		ctx.Context().Response.SetBodyStreamWriter(func(w *bufio.Writer) {
			writer = w
			ready <- true // Signal that writer is set
			<-done        // Wait for stream to be closed
		})

		<-ready // Wait until writer is set
	}

	bin := gst.NewBin(fmt.Sprintf("sinkbin%d", s.count))

	buffer, err := gst.NewElement("queue")
	if err != nil {
		return err
	}
	sink, err := gstapp.NewAppSink()
	if err != nil {
		return err
	}

	buffer.Set("max-size-buffers", 0)
	buffer.Set("max-size-bytes", 0)
	buffer.Set("max-size-time", 15000000000)
	buffer.Set("min-threshold-time", s.Settings.Buffer*1000000)

	sink.Set("max-time", 15000000000)
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
		err := errors.New("No src location configured on the httpsource")
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

	tee.Set("name", "stream")

	pipeline.Add(src)
	pipeline.Add(typefind)
	pipeline.Add(tee)

	src.Link(typefind)

	typefind.Connect("have-type", func(self *gst.Element, guint gst.TypeFindProbability, caps *gst.Caps) {
		//fmt.Println("GST CAPS: ", caps)
		if strings.HasPrefix(caps.String(), "application/x-hls") {
			demux, err := gst.NewElement("hlsdemux")
			if err != nil {
				log.Error().Msgf("HlsDemux error: %v", err) //TODO RETURN ERROR
			}

			pipeline.Add(demux)
			demux.Connect("pad-added", func(self *gst.Element, pad *gst.Pad) {
				self.Link(tee)
				tee.SyncStateWithParent()
			})

			typefind.Link(demux)
			demux.SyncStateWithParent()

		} else if strings.HasPrefix(caps.String(), "video/mpegts") {
			self.Link(tee)
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
	// Start the pipeline

	s.pipeline.SetState(gst.StatePlaying)

	// Due to recent changes in the bindings - the finalizers might fire on the pipeline
	// prematurely when it's passed between scopes. So when you do this, it is safer to
	// take a reference that you dispose of when you are done. There is an alternative
	// to this method in other examples.
	//s.pipeline.Ref()
	//defer s.pipeline.Unref()

	s.pipeline.GetPipelineBus().AddWatch(func(msg *gst.Message) bool {
		var err error
		//log.Debug().Msgf("go-gst-debug-message: %v", msg)
		switch msg.Type() {
		case gst.MessageError:
			// The parsed error implements the error interface, but also
			// contains additional debug information.
			gerr := msg.ParseError()
			err = gerr
			log.Debug().Msgf("go-gst-debug-message: %v", gerr.DebugString())
		case gst.MessageEOS:
			err = errors.New(fmt.Sprintf("EOS: %v", msg))
		case gst.MessageBuffering:
			bufPercent := msg.ParseBuffering()
			log.Debug().Msgf("go-gst-debug - stream buffer percent: %v", bufPercent)
			// Wait until buffering is complete before start/resume playing
			if bufPercent < 75 {
				s.pipeline.SetState(gst.StatePaused)
			} else {
				s.pipeline.SetState(gst.StatePlaying)
			}
		case gst.MessageClockLost:
			// Get a new clock
			log.Debug().Msgf("go-gst-debug - clock lost: %v", msg)
			s.pipeline.SetState(gst.StatePaused)
			s.pipeline.SetState(gst.StatePlaying)
		}

		// If either condition triggered an error, log and quit
		if err != nil {
			log.Error().Msgf("go-gst-error-message: %v", err.Error())
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

func RunLoop(f func(*glib.MainLoop) error) {
	mainLoop := glib.NewMainLoop(glib.MainContextDefault(), false)

	if err := f(mainLoop); err != nil {
		log.Error().Msgf("GST MAINLOOP ERROR!: %v", err)
	}
}

func (s *Stream) CreateHlsDir() error {
	if _, err := os.Stat(fmt.Sprintf("%s/%s", settings.STREAM_FILEPATH, s.Settings.Uuid)); err == nil {
		if err := os.RemoveAll(fmt.Sprintf("%s/%s", settings.STREAM_FILEPATH, s.Settings.Uuid)); err != nil {
			log.Debug().Msgf("FAILED TO REMOVE HLS FOLDER: %v", err)
		}
	}
	if err := os.Mkdir(fmt.Sprintf("%s/%s", settings.STREAM_FILEPATH, s.Settings.Uuid), os.ModePerm); err != nil {
		log.Error().Msgf("GST_HLS_MKDIR ERROR: %v", err)
		return errors.New(fmt.Sprintf("FAILED TO CREATE HLS FOLDER: %v", err))

	}
	return nil
}

func (s *Stream) StartStream(c *fiber.Ctx) error {
	var err error

	if s.pipeline, err = s.createPipeline(); err != nil {
		return err
	}

	if settings.APP_SETTINGS.Streaming.Type == "hls" || s.HlsExists {
		if err := s.CreateHlsDir(); err != nil {
			return err
		}

		if err := s.NewHLSSink(c); err != nil {
			log.Error().Msgf("NEWHLSSINK ERROR: %v", err)
			s.Close(nil, nil)
			return err
		}
	} else {
		if err := s.NewMP2TSink(c); err != nil {
			log.Error().Msgf("NEWMP2TSINK ERROR: %v", err)
			s.Close(nil, nil)
			return err
		}
	}

	go RunLoop(func(loop *glib.MainLoop) error {
		return s.mainLoop(loop)
	})

	//TODO GO CHAN TO CHECK FOR GST ERRORS AND RESTART/CLOSE ON ERR
	s.Mu.Lock()
	s.LastAccess = time.Now()
	s.Mu.Unlock()

	return nil
}
