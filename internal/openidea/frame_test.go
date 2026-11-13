package openidea

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestFrameRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	w := newFrameWriter(&buf)
	for _, body := range []string{`{"a":1}`, ``, `{"中":"😀"}`} {
		if err := w.write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	r := bufio.NewReader(&buf)
	for _, want := range []string{`{"a":1}`, ``, `{"中":"😀"}`} {
		got, err := readFrame(r, nil)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Fatalf("body = %q, want %q", got, want)
		}
	}
	if _, err := readFrame(r, nil); !errors.Is(err, io.EOF) {
		t.Fatalf("end = %v, want EOF", err)
	}
}

func TestFrameHalfPacket(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()
	go func() {
		_, _ = io.WriteString(pw, "Content-Length: 7\r\n\r\n")
		time.Sleep(50 * time.Millisecond)
		_, _ = io.WriteString(pw, `{"a":`)
		time.Sleep(50 * time.Millisecond)
		_, _ = io.WriteString(pw, `1}`)
	}()
	got, err := readFrame(bufio.NewReader(pr), nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"a":1}` {
		t.Fatalf("body = %q", got)
	}
}

func TestFrameStickyAndNoise(t *testing.T) {
	stream := "launcher said hi\nContent-Length: 2\r\n\r\n{}" +
		"content-length: 3\n\n[1]" +
		"Content-Type: application/json\r\nContent-Length: 4\r\n\r\nnull"
	r := bufio.NewReader(strings.NewReader(stream))
	var noise []string
	for _, want := range []string{"{}", "[1]", "null"} {
		got, err := readFrame(r, func(s string) { noise = append(noise, s) })
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Fatalf("body = %q, want %q", got, want)
		}
	}
	if len(noise) != 1 || noise[0] != "launcher said hi" {
		t.Fatalf("noise = %q", noise)
	}
}

func TestFrameTruncated(t *testing.T) {
	_, err := readFrame(bufio.NewReader(strings.NewReader("Content-Length: 10\r\n\r\nab")), nil)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("err = %v, want unexpected EOF", err)
	}
}

func TestFrameTooLarge(t *testing.T) {
	_, err := readFrame(bufio.NewReader(strings.NewReader("Content-Length: 999999999999\r\n\r\n")), nil)
	if err == nil {
		t.Fatal("expected an error")
	}
}
