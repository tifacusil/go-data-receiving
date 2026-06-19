package tcp

import (
	"bytes"
	"errors"
	"io"
	"iter"
	"testing"
)

// sampleFrame returns a syntactically framed message (content irrelevant to the
// reader, which only uses the length prefix).
func sampleFrame(payloadLen int) []byte {
	f := []byte{0xAA, 0x55, 0x01, 0x01, byte(payloadLen >> 8), byte(payloadLen)}
	f = append(f, make([]byte, payloadLen)...) // payload
	f = append(f, 0x00, 0x00, 0x55, 0xAA)      // crc + sync end
	return f
}

func TestReadFrame_Valid(t *testing.T) {
	want := sampleFrame(21)
	got, err := readFrame(bytes.NewReader(want))
	if err != nil {
		t.Fatalf("readFrame: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("frame mismatch: got %d bytes want %d", len(got), len(want))
	}
}

func TestReadFrame_CleanEOF(t *testing.T) {
	_, err := readFrame(bytes.NewReader(nil))
	if !errors.Is(err, errClosed) {
		t.Fatalf("empty stream err = %v, want errClosed", err)
	}
}

func TestReadFrame_BadSyncStart(t *testing.T) {
	_, err := readFrame(bytes.NewReader([]byte{0x00, 0x00, 0x01, 0x01, 0x00, 0x01}))
	if err == nil || errors.Is(err, errClosed) {
		t.Fatalf("expected sync error, got %v", err)
	}
}

func TestReadFrame_TruncatedMidFrame(t *testing.T) {
	full := sampleFrame(21)
	_, err := readFrame(bytes.NewReader(full[:len(full)-3])) // drop trailing bytes
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("truncated frame err = %v, want ErrUnexpectedEOF", err)
	}
}

func TestReadFrame_TruncatedHeader(t *testing.T) {
	_, err := readFrame(bytes.NewReader([]byte{0xAA, 0x55, 0x01})) // partial header
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("partial header err = %v, want ErrUnexpectedEOF", err)
	}
}

// chunkReader feeds bytes in small chunks to exercise readExact's loop.
type chunkReader struct {
	next func() ([]byte, bool)
	stop func()
}

func newChunkReader(data []byte, chunk int) *chunkReader {
	seq := func(yield func([]byte) bool) {
		for i := 0; i < len(data); i += chunk {
			end := i + chunk
			if end > len(data) {
				end = len(data)
			}
			if !yield(data[i:end]) {
				return
			}
		}
	}
	next, stop := iter.Pull(iter.Seq[[]byte](seq))
	return &chunkReader{next: next, stop: stop}
}

func (c *chunkReader) Read(p []byte) (int, error) {
	b, ok := c.next()
	if !ok {
		return 0, io.EOF
	}
	return copy(p, b), nil
}

func TestReadFrame_ChunkedDelivery(t *testing.T) {
	want := sampleFrame(10)
	r := newChunkReader(want, 3)
	defer r.stop()
	got, err := readFrame(r)
	if err != nil {
		t.Fatalf("readFrame chunked: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("chunked frame mismatch")
	}
}
