package openidea

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

const (
	maxMessageLen    = 256 << 20
	maxHeaderLineLen = 8 << 10
)

// readFrame reads one Content-Length framed message body. Headers are matched
// case-insensitively, both CRLF and LF line endings are accepted, and stray
// output before the first header is passed to onNoise instead of failing the
// stream (a launcher or runtime may print to stdout).
func readFrame(r *bufio.Reader, onNoise func(string)) ([]byte, error) {
	var length int64 = -1
	sawHeader := false
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				if len(line) == 0 {
					return nil, io.EOF
				}
				if !sawHeader && length < 0 {
					if onNoise != nil {
						onNoise(strings.TrimRight(line, "\r\n"))
					}
					return nil, io.EOF
				}
			}
			return nil, io.ErrUnexpectedEOF
		}
		if len(line) > maxHeaderLineLen {
			return nil, fmt.Errorf("openidea: header line too long")
		}
		text := strings.TrimRight(line, "\r\n")
		if text == "" {
			if length >= 0 {
				break
			}
			// Blank lines between messages are tolerated.
			continue
		}
		name, value, ok := strings.Cut(text, ":")
		if !ok || name == "" || strings.ContainsAny(name, " \t") {
			if !sawHeader {
				if onNoise != nil {
					onNoise(text)
				}
				continue
			}
			return nil, fmt.Errorf("openidea: malformed header %q", text)
		}
		sawHeader = true
		if strings.EqualFold(name, "content-length") {
			n, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
			if err != nil || n < 0 {
				return nil, fmt.Errorf("openidea: invalid Content-Length %q", value)
			}
			if n > maxMessageLen {
				return nil, fmt.Errorf("openidea: message too large: %d bytes", n)
			}
			length = n
		}
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, fmt.Errorf("openidea: truncated message: %w", err)
	}
	return body, nil
}

// frameWriter serializes whole messages onto the stream. Concurrent writers
// must never interleave header and body bytes.
type frameWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func newFrameWriter(w io.Writer) *frameWriter { return &frameWriter{w: w} }

func (f *frameWriter) write(body []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	if _, err := io.WriteString(f.w, header); err != nil {
		return err
	}
	_, err := f.w.Write(body)
	return err
}
