package streaming

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/go-gst/go-gst/gst"
	"github.com/go-gst/go-gst/gst/app"
	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"
)

type AppSettings struct {
	Proxy      bool `json:"proxy,omitempty"`
	Buffer     bool `json:"buffer,omitempty"`
	BufferTime int  `json:"buffertime,omitempty"`
}

// A private struct where we hold the parameter values set on our
// element.
type Settings struct {
	src       string
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

// HTTPStreamer implement Streamer with fasthttp
type HTTPStreamer struct {
	ctx      *fasthttp.RequestCtx
	done     chan bool
	writer   *bufio.Writer
	pipew    *io.PipeWriter
	piper    *io.PipeReader
	pipeline *gst.Pipeline
	// The settings for the element
	settings *Settings
	// The current state of the element
	state bool
}

// NewHTTPStreamer returns a new HTTPStreamer
func NewHTTPStreamer(ctx *fasthttp.RequestCtx) *HTTPStreamer {
	return &HTTPStreamer{
		ctx:      ctx,
		settings: &Settings{},
	}
}

// Close closes the stream
func (s *HTTPStreamer) Close() {
	if s.done != nil {
		s.pipeline.SendEvent(gst.NewEOSEvent())
		close(s.done)
	}
}

// Write writes bytes to streamer
func (s *HTTPStreamer) Write(p []byte) (n int, err error) {
	return s.writer.Write(p)
}

// Flush flushes data to the client
func (s *HTTPStreamer) Flush() error {
	if s.writer != nil {
		return s.writer.Flush()
	}

	return nil
}

// SetStatusCode sets the status code. *Must* be called before Write and Flush
func (s *HTTPStreamer) SetStatusCode(statusCode int) error {
	if s.writer != nil {
		return fmt.Errorf("Streaming started - can't set status")
	}

	s.ctx.SetStatusCode(statusCode)
	return nil
}

// SetHeader sets a response header. *Must* be called before Write and Flush
// value can be string or []byte
func (s *HTTPStreamer) SetHeader(key string, value interface{}) error {
	if s.writer != nil {
		return fmt.Errorf("Streaming started - can't set header")
	}

	switch v := value.(type) {
	case string:
		s.ctx.Response.Header.Set(key, v)
	case []byte:
		s.ctx.Response.Header.SetBytesV(key, v)
	default:
		return fmt.Errorf("Unsupported header value type - %T", value)
	}

	return nil
}

func (s *HTTPStreamer) CreatePipeline(c *fiber.Ctx) (*gst.Pipeline, error) {
	var pipeline *gst.Pipeline
	var err error

	if s.state {
		err := errors.New("GoFileSink is already started")
		return nil, err
	}

	if s.settings.src == "" {
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
	sink, err := app.NewAppSink()
	if err != nil {
		return nil, err
	}

	src.Set("location", s.settings.src)
	src.Set("user-agent", s.settings.userAgent)
	src.Set("is-live", true)
	sink.Set("max-time", 15000000000)
	sink.Set("drop", true)
	sink.SetProperty("emit-signals", true)
	sink.SetProperty("sync", false)

	pipeline.Add(src)
	pipeline.Add(typefind)
	pipeline.Add(sink.Element)
	src.Link(typefind)

	typefind.Connect("have-type", func(self *gst.Element, guint gst.TypeFindProbability, caps *gst.Caps) {
		fmt.Println("GST CAPS: %s", caps)
		if strings.HasPrefix(caps.String(), "application/x-hls") {
			demux, err := gst.NewElement("hlsdemux")
			if err != nil {
				fmt.Println(err) //TODO RETURN ERROR
			}
			pipeline.Add(demux)
			demux.Connect("pad-added", func(self *gst.Element, pad *gst.Pad) {
				pad.Link(sink.GetStaticPad("sink"))
			})
			typefind.Link(demux)
			demux.SetState(gst.StatePlaying)

		} else if strings.HasPrefix(caps.String(), "video/mpegts") {
			self.Link(sink.Element)
		}

	})

	s.piper, s.pipew = io.Pipe()
	i := 0
	sink.SetCallbacks(&app.SinkCallbacks{
		NewSampleFunc: func(appSink *app.Sink) gst.FlowReturn {
			sample := appSink.PullSample()
			if sample == nil {
				return gst.FlowEOS
			}

			buffer := sample.GetBuffer()
			if buffer == nil {
				return gst.FlowOK
			}
			defer buffer.Unmap()
			defer buffer.Unref()

			i++
			println(pipeline.GetCurrentState().String(), i)

			//s.pipew.Write(buffer.Extract(0, buffer.GetSize()))
			if s.writer != nil {
				s.Write(buffer.Extract(0, buffer.GetSize()))
			}
			//s.CustomBuffer.bufWrite.Write(data) // Write the buffer data to the HTTP response.

			return gst.FlowOK
		},
	})
	return pipeline, nil
}

func (s *HTTPStreamer) StartPipeline(pipeline *gst.Pipeline) error {
	// Start the pipeline
	pipeline.SetState(gst.StatePlaying)
	var err error
	s.settings.userAgent = "Xivi 1.0"

	pipeline.GetPipelineBus().AddWatch(func(msg *gst.Message) bool {

		// If the stream has ended or any element posts an error to the
		// bus, populate error.
		switch msg.Type() {
		case gst.MessageEOS:
			err = errors.New("end-of-stream")
		case gst.MessageError:
			// The parsed error implements the error interface, but also
			// contains additional debug information.
			gerr := msg.ParseError()
			fmt.Println("go-gst-debug:", gerr.DebugString())
			err = gerr
		}

		// If either condition triggered an error, log and quit
		if err != nil {
			fmt.Println("ERROR:", err.Error())
			return false
		}

		return true
	})
	if err != nil {
		return err
	}

	s.state = true
	fmt.Println(gst.LevelInfo, "Stream has started")
	return nil
}

// Stop is called to stop the element. Set the internal state and close the file.
func (s *HTTPStreamer) Stop(pipeline *gst.Pipeline) (bool, error) {
	if !s.state {
		err := errors.New("Stream is not started")
		return false, err
	}

	pipeline.SendEvent(gst.NewEOSEvent())
	s.state = false

	fmt.Println("Stream has stopped")
	return true, nil
}

func (s *HTTPStreamer) StartStream(channel string, c *fiber.Ctx) error {
	var pipeline *gst.Pipeline
	var err error
	//s.streamData = &bytes.Buffer{}
	appSettings := AppSettings{
		Proxy:      true,
		Buffer:     false,
		BufferTime: 0,
	}

	go func() {
		<-s.done
		pipeline.SetState(gst.StateNull)
		pipeline.Clear()
		pipeline.Unref()
		s.Close()
	}()

	switch appSettings.Buffer {

	case false:
		s.settings.buffer = 0
	case true:
		s.settings.buffer = appSettings.BufferTime
	}

	s.settings.src = channel

	if pipeline, err = s.CreatePipeline(c); err != nil {
		return err
	}
	if err = s.StartPipeline(pipeline); err != nil {
		return err
	}

	if s.writer == nil {
		s.done = make(chan bool)
		ready := make(chan bool)
		s.ctx.Response.SetBodyStreamWriter(func(w *bufio.Writer) {
			s.writer = w
			ready <- true // Signal that writer is set
			<-s.done      // Wait for stream to be closed
		})

		<-ready // Wait until writer is set
	}

	println("STREAMING")
	return nil

}
