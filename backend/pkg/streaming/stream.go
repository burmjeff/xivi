package streaming

import (
	"bufio"
	"errors"
	"fmt"
	"strings"
	"time"
	"xivi/backend/app/models"

	"github.com/go-gst/go-gst/gst"
	gstapp "github.com/go-gst/go-gst/gst/app"
	"github.com/valyala/fasthttp"
)

var Streams []*Stream

type Stream struct {
	pipeline *gst.Pipeline
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
		s.count--
		close(done)
		if s.count == 0 {
			go func() {
				if !s.pipeline.SendEvent(gst.NewEOSEvent()) {
					fmt.Println("WARNING: Failed to send EOS to pipeline")
				}
				elements, _ := s.pipeline.GetElementsSorted()

				for _, element := range elements {
					fmt.Println("Disposing GST element:", element.GetName(), "state:", element.GetCurrentState())
					pads, _ := element.GetSrcPads()
					for _, pad := range pads {
						pad.PauseTask()
					}
					if err := element.SetState(gst.StateNull); err != nil {
						fmt.Println("WARNING: Failed to set", element.GetName(), "state to Null")
					}
					if err := s.pipeline.Remove(element); err != nil {
						fmt.Println("WARNING: Failed to remove element from pipeline:", element.GetName())
					}
				}
				s.pipeline.Clear()
				fmt.Println("PIPELINE DISPOSED")
			}()
			RemoveStream(s)
			return gst.FlowEOS
		}

		tee, _ := s.pipeline.GetElementByName("stream")
		tee.Unlink(sinkBin.Element)
		if err := s.pipeline.Remove(sinkBin.Element); err != nil {
			fmt.Println("WARNING: Failed to remove stream bin from pipeline:", sinkBin.GetName())
		}
		go func() {
			if !sinkBin.SendEvent(gst.NewEOSEvent()) {
				fmt.Println("WARNING: Failed to send EOS to stream branch")
			}
			elements, _ := sinkBin.GetElementsSorted()
			for _, element := range elements {
				fmt.Println("Disposing GST element:", element.GetName(), "state:", element.GetCurrentState())
				pads, _ := element.GetPads()
				for _, pad := range pads {
					pad.PauseTask()
				}
			}
			if err := sinkBin.SetState(gst.StateNull); err != nil {
				fmt.Println("WARNING: Failed to set", sinkBin.GetName(), "state to Null")
			}

			sinkBin.Clear()
			fmt.Println("STREAM CLOSED")
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
		return fmt.Errorf("Streaming started - can't set status")
	}

	ctx.SetStatusCode(statusCode)
	return nil
}

// SetHeader sets a response header. *Must* be called before Write and Flush
// value can be string or []byte
func (s *Stream) SetHeader(ctx *fasthttp.RequestCtx, writer *bufio.Writer, key string, value interface{}) error {
	if writer != nil {
		return fmt.Errorf("Streaming started - can't set header")
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

func (s *Stream) NewSink(ctx *fasthttp.RequestCtx) error {
	var writer *bufio.Writer
	var done chan bool
	var bin *gst.Bin

	if writer == nil {
		done = make(chan bool)
		ready := make(chan bool)
		ctx.Response.SetBodyStreamWriter(func(w *bufio.Writer) {
			writer = w
			ready <- true // Signal that writer is set
			<-done        // Wait for stream to be closed
		})

		<-ready // Wait until writer is set
	}

	queue, err := gst.NewElement("queue")
	if err != nil {
		return err
	}
	sink, err := gstapp.NewAppSink()
	if err != nil {
		return err
	}

	queue.Set("name", "sinkqueue")
	sink.Set("max-time", 15000000000)
	sink.Set("drop", true)
	sink.SetProperty("emit-signals", true)
	sink.SetProperty("sync", false)

	for i := 0; i <= s.count; i++ {
		if element, _ := s.pipeline.GetElementByName(fmt.Sprintf("sinkbin%d", i)); element == nil {
			bin = gst.NewBin(fmt.Sprintf("sinkbin%d", i))
			break
		}
	}

	bin.Add(queue)
	bin.Add(sink.Element)
	queue.Link(sink.Element)

	queuepad := gst.NewGhostPad("sink", queue.GetStaticPad("sink"))
	queuepad.SetActive(true)
	bin.AddPad(queuepad.Pad)

	sink.SetCallbacks(&gstapp.SinkCallbacks{
		NewSampleFunc: func(appSink *gstapp.Sink) gst.FlowReturn {

			sample := appSink.TryPullSample(gst.ClockTime(30 * time.Second))
			if sample == nil {
				return s.Close(bin, done)
			}

			buffer := sample.GetBuffer()
			if buffer == nil {
				return gst.FlowOK
			}
			defer buffer.Unmap()

			if _, err = s.Write(writer, buffer.Extract(0, buffer.GetSize())); err != nil {
				fmt.Println("STREAM WRITE ERROR:", err)
				return s.Close(bin, done)
			}
			if err = s.Flush(writer); err != nil {
				fmt.Println("STREAM FLUSH ERROR:", err)
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
	//bin.SyncStateWithParent()
	bin.SetState(gst.StatePlaying)

	s.count++

	return nil
}

func (s *Stream) CreatePipeline() (*gst.Pipeline, error) {
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
		fmt.Println("GST CAPS: %s", caps)
		if strings.HasPrefix(caps.String(), "application/x-hls") {
			demux, err := gst.NewElement("hlsdemux")
			if err != nil {
				fmt.Println(err) //TODO RETURN ERROR
			}
			pipeline.Add(demux)
			demux.Connect("pad-added", func(self *gst.Element, pad *gst.Pad) {
				pad.Link(tee.GetStaticPad("sink"))
			})
			typefind.Link(demux)

		} else if strings.HasPrefix(caps.String(), "video/mpegts") {
			self.Link(tee)
		}

	})

	return pipeline, nil
}

func (s *Stream) StartPipeline(pipeline *gst.Pipeline) error {
	// Start the pipeline
	pipeline.SetState(gst.StatePlaying)
	var err error

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
			s.Close(nil, nil)
			return false
		}

		return true
	})
	if err != nil {
		return err
	}

	fmt.Println(gst.LevelInfo, "Stream has started")
	return nil
}

func (s *Stream) StartStream(channel string) error {
	var err error
	//s.streamData = &bytes.Buffer{}
	appSettings := models.AppSettings{
		Proxy:      true,
		Buffer:     false,
		BufferTime: 0,
	}
	s.Settings.userAgent = "Xivi 1.0"

	switch appSettings.Buffer {

	case false:
		s.Settings.buffer = 0
	case true:
		s.Settings.buffer = appSettings.BufferTime
	}

	s.Settings.Src = channel

	if s.pipeline, err = s.CreatePipeline(); err != nil {
		return err
	}
	if err = s.StartPipeline(s.pipeline); err != nil {
		return err
	}
	s.count = 0

	return nil

}
