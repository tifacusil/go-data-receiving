package tcp

import (
	"errors"
	"fmt"
	"io"
)

const (
	headerSize  = 6 // SYNC(2)+VER(1)+TYPE(1)+LEN(2)
	trailerSize = 4 // CRC(2)+SYNC_END(2)
)

// errClosed signals a clean end of stream (no frame in flight), mapping to the
// C# ReadFrameAsync returning null.
var errClosed = errors.New("connection closed")

// readFrame reads one length-prefixed frame from r. It returns errClosed on a
// clean EOF (no bytes or exactly at a frame boundary), or a wrapped error on a
// bad sync start or truncated frame. Port of the C# FrameReader.
func readFrame(r io.Reader) ([]byte, error) {
	header := make([]byte, headerSize)
	if err := readExact(r, header); err != nil {
		return nil, err // errClosed on clean EOF, else read error
	}

	if header[0] != 0xAA || header[1] != 0x55 {
		return nil, fmt.Errorf("bad sync start: 0x%02X 0x%02X", header[0], header[1])
	}

	payloadLen := int(header[4])<<8 | int(header[5])
	totalLen := headerSize + payloadLen + trailerSize
	frame := make([]byte, totalLen)
	copy(frame, header)

	if err := readExact(r, frame[headerSize:]); err != nil {
		if errors.Is(err, errClosed) {
			// EOF mid-frame is a truncated frame, not a clean close.
			return nil, io.ErrUnexpectedEOF
		}
		return nil, err
	}
	return frame, nil
}

// readExact fills buf fully. Returns errClosed if EOF occurs before any byte is
// read into buf; io.ErrUnexpectedEOF if EOF occurs partway.
func readExact(r io.Reader, buf []byte) error {
	read := 0
	for read < len(buf) {
		n, err := r.Read(buf[read:])
		read += n
		if err != nil {
			if errors.Is(err, io.EOF) {
				if read == 0 {
					return errClosed
				}
				return io.ErrUnexpectedEOF
			}
			return err
		}
		if n == 0 && read < len(buf) {
			// Defensive: a reader returning (0, nil) repeatedly would spin.
			return errClosed
		}
	}
	return nil
}
