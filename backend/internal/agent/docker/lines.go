package docker

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

const (
	// maxLogLine is the longest line sent to the server. Longer ones are cut
	// and end with an ellipsis.
	maxLogLine = 16 << 10
	// maxBatchLines is the most lines in one frame.
	maxBatchLines = 200
)

// demuxFrames reads Docker's multiplexed log format and calls onFrame for
// every frame with the stream (1 stdout, 2 stderr). onIdle runs when no more
// data is buffered, which is where a caller flushes what it has collected.
func demuxFrames(r io.Reader, onFrame func(stream byte, payload []byte) error, onIdle func() error) error {
	br := bufio.NewReaderSize(r, 64<<10)
	hdr := make([]byte, 8)
	for {
		if _, err := io.ReadFull(br, hdr); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return nil
			}
			return err
		}
		payload := make([]byte, binary.BigEndian.Uint32(hdr[4:]))
		if _, err := io.ReadFull(br, payload); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return nil
			}
			return err
		}
		if err := onFrame(hdr[0], payload); err != nil {
			return err
		}
		if br.Buffered() == 0 {
			if err := onIdle(); err != nil {
				return err
			}
		}
	}
}

// lineSplitter cuts log text into lines, keeps the stream of each line and
// writes them as JSON arrays of protocol.DockerLogLine.
type lineSplitter struct {
	w     io.Writer
	part  map[string][]byte // text after the last newline, per stream
	batch []protocol.DockerLogLine
}

func newLineSplitter(w io.Writer) *lineSplitter {
	return &lineSplitter{w: w, part: map[string][]byte{}}
}

// feed adds text of a stream. Full batches are written at once.
func (s *lineSplitter) feed(stream string, text []byte) error {
	buf := append(s.part[stream], text...)
	for {
		i := bytes.IndexByte(buf, '\n')
		if i < 0 {
			break
		}
		if err := s.add(stream, buf[:i]); err != nil {
			return err
		}
		buf = buf[i+1:]
	}
	if len(buf) > 4*maxLogLine {
		// A line without an end: send what there is instead of holding it.
		if err := s.add(stream, buf); err != nil {
			return err
		}
		buf = nil
	}
	s.part[stream] = append([]byte(nil), buf...)
	return nil
}

func (s *lineSplitter) add(stream string, raw []byte) error {
	s.batch = append(s.batch, parseLogLine(stream, raw))
	if len(s.batch) >= maxBatchLines {
		return s.flush()
	}
	return nil
}

// flush writes the collected lines as one frame.
func (s *lineSplitter) flush() error {
	if len(s.batch) == 0 {
		return nil
	}
	b, err := json.Marshal(s.batch)
	s.batch = s.batch[:0]
	if err != nil {
		return err
	}
	_, err = s.w.Write(b)
	return err
}

// finish turns the unfinished last lines into lines and writes everything.
func (s *lineSplitter) finish() error {
	for _, stream := range []string{"stdout", "stderr"} {
		if rest := s.part[stream]; len(rest) > 0 {
			if err := s.add(stream, rest); err != nil {
				return err
			}
		}
		s.part[stream] = nil
	}
	return s.flush()
}

// parseLogLine splits "2026-09-29T10:00:00.123456789Z text" into the time
// and the text, and cuts a long text.
func parseLogLine(stream string, raw []byte) protocol.DockerLogLine {
	line := protocol.DockerLogLine{Stream: stream}
	text := strings.TrimSuffix(strings.ToValidUTF8(string(raw), "�"), "\r")
	if i := strings.IndexByte(text, ' '); i >= 20 && i <= 35 {
		if _, err := time.Parse(time.RFC3339Nano, text[:i]); err == nil {
			line.Time, text = text[:i], text[i+1:]
		}
	}
	if len(text) > maxLogLine {
		cut := maxLogLine
		for cut > 0 && !utf8.RuneStart(text[cut]) {
			cut--
		}
		text = text[:cut] + "…"
	}
	line.Text = text
	return line
}

// streamName maps the first byte of a frame header to a stream name.
func streamName(b byte) string {
	if b == 2 {
		return "stderr"
	}
	return "stdout"
}

// logLines reads a container log as lines with their stream. tty is true for
// a container with a TTY, whose output is not multiplexed and all counts as
// stdout.
func logLines(r io.Reader, w io.Writer, tty bool) error {
	sp := newLineSplitter(w)
	var err error
	if tty {
		buf := make([]byte, logFlushSize)
		for err == nil {
			n, rerr := r.Read(buf)
			if n > 0 {
				err = sp.feed("stdout", buf[:n])
			}
			if err == nil && (rerr != nil || n < len(buf)) {
				err = sp.flush()
			}
			if rerr != nil {
				if !errors.Is(rerr, io.EOF) && err == nil {
					err = rerr
				}
				break
			}
		}
	} else {
		err = demuxFrames(r, func(stream byte, payload []byte) error { return sp.feed(streamName(stream), payload) }, sp.flush)
	}
	if ferr := sp.finish(); err == nil {
		err = ferr
	}
	return err
}
