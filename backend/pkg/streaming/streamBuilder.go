package streaming

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/go-gst/go-glib/glib"
	"github.com/go-gst/go-gst/gst"
	"github.com/go-gst/go-gst/gst/base"
)

// A private struct to hold our internal state.
type state struct {
	// Whether the element is started or not
	started bool
	// The file the element is writing to
	file *os.File
	// The current position in the file
	position uint64
}

// A private struct where we hold the parameter values set on our
// element.
type settings struct {
	src       string
	sink      string
	buffer    int
	userAgent string
}

// A structure that implements (at a minimum) the glib.GoObject interface.
type Stream struct {
	// The settings for the element
	settings *settings
	// The current state of the element
	state *state
	// The gst pipeline
	pipeline *gst.Pipeline
}

func (s *Stream) setSrc(path string) error {
	if s.state.started {
		return errors.New("Changing the `location` property on an already started `Stream` is not supported")
	}
	s.settings.src = path
	return nil
}

func (s *Stream) setSink(path string) error {
	if s.state.started {
		return errors.New("changing the `location` property on an already started `GoFileSink` is not supported")
	}
	s.settings.sink = strings.TrimPrefix(path, "file://") // should obviously use url.URL and do actual parsing
	return nil
}

func (s *Stream) createPipeline() error {
	var err error

	if s.state.started {
		err := errors.New("GoFileSink is already started")
		return err
	}

	if s.settings.src == "" {
		err := errors.New("No src location configured on the httpsource")
		return err
	}

	if s.settings.sink == "" {
		err := errors.New("No sink location configured on the filesink")
		return err
	}

	gst.Init(nil)

	s.pipeline, err = gst.NewPipeline("")
	if err != nil {
		return err
	}

	src, err := gst.NewElement("souphttpsrc")
	if err != nil {
		return err
	}
	typefind, err := gst.NewElement("typefind")
	if err != nil {
		return err
	}
	sink, err := gst.NewElement("filesink")
	if err != nil {
		return err
	}

	src.Set("location", s.settings.src)
	src.Set("user-agent", s.settings.userAgent)
	src.Set("is-live", true)
	sink.Set("location", s.settings.sink)

	s.pipeline.Add(src)
	s.pipeline.Add(typefind)
	s.pipeline.Add(sink)
	src.Link(typefind)

	typefind.Connect("have-type", func(self *gst.Element, guint gst.TypeFindProbability, caps *gst.Caps) {
		fmt.Println("GST CAPS: %s", caps)
		if strings.HasPrefix(caps.String(), "application/x-hls") {
			demux, err := gst.NewElement("hlsdemux")
			if err != nil {
				fmt.Println(err) //TODO RETURN ERROR
			}
			s.pipeline.Add(demux)
			demux.Connect("pad-added", func(self *gst.Element, pad *gst.Pad) {
				pad.Link(sink.GetStaticPad("sink"))
			})
			typefind.Link(demux)
			demux.SetState(gst.StatePlaying)

		} else if strings.HasPrefix(caps.String(), "video/mpegts") {
			self.Link(sink)
		}

	})
	return nil
}

func (s *Stream) StartStream(loop *glib.MainLoop) error {
	destFile := s.settings.sink
	var err error

	s.state.file, err = os.Create(destFile)
	if err != nil {
		errors.New(fmt.Sprintf("Could not open %s for writing: %s", destFile, err.Error()))
		return err
	}

	if s.pipeline == nil {
		err = s.createPipeline()
		if err != nil {
			return err
		}
	}

	// Start the pipeline
	s.pipeline.SetState(gst.StatePlaying)

	// Add a message watch to the bus to quit on any error
	s.pipeline.GetPipelineBus().AddWatch(func(msg *gst.Message) bool {
		var err error

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
			loop.Quit()
			return false
		}

		return true
	})

	s.state.started = true
	fmt.Println(gst.LevelInfo, "Stream has started")
	return loop.RunError()
}

// Stop is called to stop the element. Set the internal state and close the file.
func (s *Stream) Stop(self *base.GstBaseSink) (bool, error) {
	if !s.state.started {
		err := errors.New("Stream is not started")
		return false, err
	}

	s.pipeline.SendEvent(gst.NewEOSEvent())
	s.state.started = false

	if err := s.state.file.Close(); err != nil {
		err := errors.New(fmt.Sprintf("Failed to close the destination file: %s", err.Error()))
		return false, err
	}
	if err := os.Remove(s.settings.sink); err != nil {
		err := errors.New(fmt.Sprintf("Failed to remove temp stream file: %s", err.Error()))
		return false, err
	}

	fmt.Println("Stream has stopped")
	return true, nil
}

// GetURI returns the currently configured URI
func (s *Stream) GetURI() string { return fmt.Sprintf("file://%s", s.settings.src) }

// SetURI should set the URI that this element is working on.
func (s *Stream) SetURI(uri string) (bool, error) {
	if uri == "file://" {
		return true, nil
	}
	err := s.setSrc(uri)
	if err != nil {
		return false, err
	}
	fmt.Println(gst.LevelInfo, fmt.Sprintf("Set `location` to %s via URIHandler", s.settings.src))
	return true, nil
}
