package parsing

import "fmt"

// Frame layout (DTU binary protocol):
//
//	SYNC_START(0xAA 0x55) | VER(1) | TYPE(1) | LEN(2 BE) | PAYLOAD(LEN) | CRC(2 BE) | SYNC_END(0x55 0xAA)
//
// len == 10 + LEN. CRC-16/CCITT-FALSE covers VER+TYPE+LEN+PAYLOAD (bytes 2..end-4).
const (
	minFrameSize    = 10 // SYNC(2)+VER(1)+TYPE(1)+LEN(2)+CRC(2)+SYNC(2)
	protocolVersion = 0x01
	maxPayloadLen   = 1024
	minPayloadLen   = 1
)

// validateFrame checks structure, version, type, length and CRC. On success it
// returns a ParseResult with Success=true and no envelope (the caller decodes
// the body). Ported 1:1 from C# FrameValidator.Validate, including error text.
func validateFrame(frame []byte) ParseResult {
	if len(frame) < minFrameSize {
		return failResult(StageSyncBytes, "Frame too short")
	}

	if frame[0] != 0xAA || frame[1] != 0x55 {
		return failResult(StageSyncBytes, fmt.Sprintf("Bad sync start: 0x%02X 0x%02X", frame[0], frame[1]))
	}

	end := len(frame) - 1
	if frame[end] != 0xAA || frame[end-1] != 0x55 {
		return failResult(StageSyncBytes, fmt.Sprintf("Bad sync end: 0x%02X 0x%02X", frame[end-1], frame[end]))
	}

	if frame[2] != protocolVersion {
		return failResult(StageVersion, fmt.Sprintf("Unknown version: 0x%02X", frame[2]))
	}

	typeCode := frame[3]
	if typeCode < 0x01 || typeCode > 0x05 {
		return failResult(StageSensorType, fmt.Sprintf("Unknown sensor type: 0x%02X", typeCode))
	}

	declaredLen := int(frame[4])<<8 | int(frame[5])
	if declaredLen < minPayloadLen || declaredLen > maxPayloadLen {
		return failResult(StageLength, fmt.Sprintf("Payload length %d out of range [1..1024]", declaredLen))
	}

	expectedLen := 10 + declaredLen
	if len(frame) != expectedLen {
		return failResult(StageLength, fmt.Sprintf("Frame size %d != expected %d for declared payload length %d", len(frame), expectedLen, declaredLen))
	}

	// CRC covers bytes 2..end-4 inclusive (VER+TYPE+LEN+PAYLOAD).
	crcData := frame[2 : len(frame)-4]
	computed := CRC16(crcData)
	crcOffset := len(frame) - 4
	received := uint16(frame[crcOffset])<<8 | uint16(frame[crcOffset+1])
	if computed != received {
		return failResult(StageCrc, fmt.Sprintf("CRC mismatch: expected 0x%04X got 0x%04X", computed, received))
	}

	return ParseResult{Success: true}
}
