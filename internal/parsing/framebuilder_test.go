package parsing

import (
	"encoding/binary"
	"math"
)

// frameBuilder constructs valid DTU binary frames for tests, mirroring the C#
// FrameBuilder test helper: it builds the header body, appends the sensor
// payload, then wraps SYNC/VER/TYPE/LEN + CRC + SYNC.

// buildHeader encodes the common header (MSG_ID..SAMPLE_INT) into payload bytes.
func buildHeader(msgID [16]byte, projectID, sensorID string, epochMs int64, samplingInterval uint32) []byte {
	b := make([]byte, 0, 64)
	b = append(b, msgID[:]...)
	b = append(b, byte(len(projectID)))
	b = append(b, projectID...)
	b = append(b, byte(len(sensorID)))
	b = append(b, sensorID...)
	ts := make([]byte, 8)
	binary.BigEndian.PutUint64(ts, uint64(epochMs))
	b = append(b, ts...)
	si := make([]byte, 4)
	binary.BigEndian.PutUint32(si, samplingInterval)
	b = append(b, si...)
	return b
}

// buildFrame wraps a payload (header+sensor body) into a full frame with the
// given type byte, computing LEN and CRC.
func buildFrame(typeCode byte, payload []byte) []byte {
	declaredLen := len(payload)
	frame := make([]byte, 0, 10+declaredLen)
	frame = append(frame, 0xAA, 0x55, 0x01, typeCode)
	lenBytes := []byte{byte(declaredLen >> 8), byte(declaredLen & 0xFF)}
	frame = append(frame, lenBytes...)
	frame = append(frame, payload...)
	// CRC covers VER+TYPE+LEN+PAYLOAD = frame[2:].
	crc := CRC16(frame[2:])
	frame = append(frame, byte(crc>>8), byte(crc&0xFF))
	frame = append(frame, 0x55, 0xAA)
	return frame
}

// f32be encodes a float32 as 4 big-endian bytes.
func f32be(v float32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, math.Float32bits(v))
	return b
}

// gnssBody builds a 21-byte GNSS sensor payload.
func gnssBody(x, y, z, h, cum float32, fix byte) []byte {
	b := make([]byte, 0, 21)
	b = append(b, f32be(x)...)
	b = append(b, f32be(y)...)
	b = append(b, f32be(z)...)
	b = append(b, f32be(h)...)
	b = append(b, f32be(cum)...)
	b = append(b, fix)
	return b
}
