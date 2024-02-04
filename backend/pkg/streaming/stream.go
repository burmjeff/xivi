package streaming

import (
	"bufio"
	"errors"
	"fmt"
	"strings"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/platform/settings"

	"github.com/go-gst/go-glib/glib"
	"github.com/go-gst/go-gst/gst"
	gstapp "github.com/go-gst/go-gst/gst/app"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
	"github.com/valyala/fasthttp"
)

var Streams []*Stream

type Stream struct {
	pipeline *gst.Pipeline
	mimeType string
	// The settings for the element
	Settings *Settings
	// The current stream count
	count int
}
type Settings struct {
	Src       string
	buffer    int
	userAgent string
}

// Streamer is streaming API
type Streamer interface {
	// Write writes bytes to streamer
	Write(p []byte) (n int, err error)
	// Flush flushes data to the client
	Flush() error
	// SetStatusCode sets the status code. *Must* be called before Write and Flush
	SetStatusCode(statusCode int) error
	// SetHeader sets a response header. *Must* be called before Write and Flush
	SetHeader(key string, value interface{}) error
}

func NewStreamer() *Stream {
	return &Stream{
		Settings: &Settings{},
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

// Close closes the stream
func (s *Stream) Close(sinkBin *gst.Bin, done chan bool) gst.FlowReturn {
	select {
	case <-done:
		return gst.FlowEOS
	default:
		if done != nil {
			s.count--
			close(done)
		}
		if s.count <= 0 || sinkBin == nil {
			go func() {
				if !s.pipeline.SendEvent(gst.NewEOSEvent()) {
					log.Warn().Msg("WARNING: Failed to send EOS to pipeline")
				}
				elements, _ := s.pipeline.GetElementsSorted()

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
				s.pipeline.Clear()
				log.Debug().Msg("PIPELINE DISPOSED")
			}()
			RemoveStream(s)
			return gst.FlowEOS
		}

		tee, _ := s.pipeline.GetElementByName("stream")
		tee.Unlink(sinkBin.Element)
		if err := s.pipeline.Remove(sinkBin.Element); err != nil {
			log.Warn().Msgf("WARNING: Failed to remove stream bin from pipeline: %s", sinkBin.GetName())
		}
		go func() {
			if !sinkBin.SendEvent(gst.NewEOSEvent()) {
				log.Warn().Msg("WARNING: Failed to send EOS to stream branch")
			}
			elements, _ := sinkBin.GetElementsSorted()
			for _, element := range elements {
				log.Debug().Msgf("Disposing GST element: %s with state: %s", element.GetName(), element.GetCurrentState())
				pads, _ := element.GetPads()
				for _, pad := range pads {
					pad.PauseTask()
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

// Write writes bytes to streamer
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

// SetStatusCode sets the status code. *Must* be called before Write and Flush
func (s *Stream) SetStatusCode(ctx *fasthttp.RequestCtx, writer *bufio.Writer, statusCode int) error {
	if writer != nil {
		return errors.New("Streaming started - can't set status")
	}

	ctx.SetStatusCode(statusCode)
	return nil
}

// SetHeader sets a response header. *Must* be called before Write and Flush
// value can be string or []byte
func (s *Stream) SetHeader(ctx *fasthttp.RequestCtx, writer *bufio.Writer, key string, value interface{}) error {
	if writer != nil {
		return errors.New("Streaming started - can't set header")
	}

	switch v := value.(type) {
	case string:
		ctx.Response.Header.Set(key, v)
	case []byte:
		ctx.Response.Header.SetBytesV(key, v)
	default:
		return fmt.Errorf("Unsupported header value type - %T", value)
	}

	return nil
}

func (s *Stream) NewSink(ctx *fiber.Ctx) error {
	var bin *gst.Bin
	var writer *bufio.Writer
	var done chan bool

	s.SetStatusCode(ctx.Context(), writer, 200)
	s.SetHeader(ctx.Context(), writer, fiber.HeaderContentType, s.mimeType)
	s.SetHeader(ctx.Context(), writer, fiber.HeaderAcceptRanges, "bytes")

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
	buffer.Set("min-threshold-time", s.Settings.buffer*1000000)

	sink.Set("max-time", 15000000000)
	sink.Set("drop", true)
	sink.Set("emit-signals", true)
	sink.Set("sync", false)

	for i := 0; i <= s.count; i++ {
		if element, _ := s.pipeline.GetElementByName(fmt.Sprintf("sinkbin%d", i)); element == nil {
			bin = gst.NewBin(fmt.Sprintf("sinkbin%d", i))
			break
		}
	}

	bin.Add(buffer)
	bin.Add(sink.Element)
	buffer.Link(sink.Element)

	queuepad := gst.NewGhostPad("sink", buffer.GetStaticPad("sink"))
	queuepad.SetActive(true)
	bin.AddPad(queuepad.Pad)

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

	tee, err := s.pipeline.GetElementByName("stream")
	if err != nil {
		bin.SetState(gst.StateNull)
		bin.Clear()
		return err
	}
	bin.SetState(gst.StatePaused)
	s.pipeline.Add(bin.Element)

	if err := tee.Link(bin.Element); err != nil {
		s.pipeline.Remove(bin.Element)
		bin.SetState(gst.StateNull)
		bin.Clear()
		return err
	}

	bin.SyncStateWithParent()
	bin.SetState(gst.StatePlaying)
	log.Info().Msgf("New Stream started")

	s.count++

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
	src.Set("user-agent", s.Settings.userAgent)
	src.Set("is-live", true)

	tee.Set("name", "stream")

	pipeline.Add(src)
	pipeline.Add(typefind)
	pipeline.Add(tee)

	src.Link(typefind)

	typefind.Connect("have-type", func(self *gst.Element, guint gst.TypeFindProbability, caps *gst.Caps) {
		//fmt.Println("GST CAPS: ", caps)
		s.mimeType = "video/mp4"
		if strings.HasPrefix(caps.String(), "application/x-hls") {
			demux, err := gst.NewElement("hlsdemux")
			if err != nil {
				log.Err(err) //TODO RETURN ERROR
			}
			pipeline.Add(demux)
			typefind.Link(demux)
			demux.Connect("pad-added", func(self *gst.Element, pad *gst.Pad) {
				self.Link(tee)
			})
			demux.SetState(gst.StatePlaying)

		} else if strings.HasPrefix(caps.String(), "video/mpegts") {
			self.Link(tee)
		}

	})

	return pipeline, nil
}

func (s *Stream) mainLoop(loop *glib.MainLoop) error {
	// Start the pipeline

	// Due to recent changes in the bindings - the finalizers might fire on the pipeline
	// prematurely when it's passed between scopes. So when you do this, it is safer to
	// take a reference that you dispose of when you are done. There is an alternative
	// to this method in other examples.
	s.pipeline.Ref()
	defer s.pipeline.Unref()

	s.pipeline.GetPipelineBus().AddWatch(func(msg *gst.Message) bool {
		log.Debug().Msgf("go-gst-debug-message: %v", msg)
		switch msg.Type() {
		case gst.MessageError:
			// The parsed error implements the error interface, but also
			// contains additional debug information.
			gerr := msg.ParseError()
			log.Debug().Msgf("go-gst-debug: %v", gerr.DebugString())
			log.Error().Msgf("go-gst-error: %v", gerr.Error())
			s.Close(nil, nil)
			loop.Quit()
			return false
		case gst.MessageEOS:
			log.Info().Msgf("go-gst-EOS: %v", msg)
			s.Close(nil, nil)
			loop.Quit()
			return false
		case gst.MessageBuffering:
			bufPercent := msg.ParseBuffering()
			log.Debug().Msgf("go-gst-debug - stream buffer percent: %v", bufPercent)
			/* Wait until buffering is complete before start/resume playing */
			if bufPercent < 100 {
				s.pipeline.SetState(gst.StatePaused)
			} else {
				s.pipeline.SetState(gst.StatePlaying)
			}
		case gst.MessageClockLost:
			/* Get a new clock */
			log.Debug().Msgf("go-gst-debug - clock lost: %v", msg)
			s.pipeline.SetState(gst.StatePaused)
			s.pipeline.SetState(gst.StatePlaying)
		}

		return true
	})

	s.pipeline.SetState(gst.StatePlaying)

	return loop.RunError()
}

func RunLoop(f func(*glib.MainLoop) error) {
	mainLoop := glib.NewMainLoop(glib.MainContextDefault(), false)

	if err := f(mainLoop); err != nil {
		fmt.Println("ERROR!", err)
	}
}

func (s *Stream) StartStream(channel models.ChannelUrl) error {
	var err error
	s.Settings.userAgent = "Xivi 1.0"
	s.Settings.buffer = settings.APP_SETTINGS.Streaming.Buffer

	s.Settings.Src = channel.Url

	if s.pipeline, err = s.createPipeline(); err != nil {
		return err
	}

	go RunLoop(func(loop *glib.MainLoop) error {
		return s.mainLoop(loop)
	})

	//TODO GO CHAN TO CHECK FOR GST ERRORS AND RESTART/CLOSE ON ERR
	s.count = 0

	return nil
}
