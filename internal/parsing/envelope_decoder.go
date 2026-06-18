package parsing

import (
	"encoding/binary"
	"fmt"
	"regexp"
	"time"
)

var projectIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{2,49}$`)

// header holds the decoded common header plus how many bytes it consumed.
type header struct {
	length           int
	messageID        string
	projectID        string
	sensorID         string
	timestamp        string
	samplingInterval int
}

// decodeHeader decodes the common header from the payload region (bytes 6..N-4
// of the full frame). On failure returns a non-nil *ParseError.
// Ported 1:1 from C# EnvelopeDecoder.Decode, including offsets, ranges and text.
func decodeHeader(payload []byte) (header, *ParseError) {
	fail := func(stage ParseStage, msg string) (header, *ParseError) {
		return header{}, &ParseError{Stage: stage, Message: msg}
	}

	offset := 0

	// MSG_ID: 16 bytes UUID.
	if len(payload) < 16 {
		return fail(StageHeaderDecode, "Payload too short for MSG_ID")
	}
	messageID := formatUUID(payload[0:16])
	offset += 16

	// PROJ_LEN: uint8.
	if len(payload) < offset+1 {
		return fail(StageHeaderDecode, "Payload too short for PROJ_LEN")
	}
	projLen := int(payload[offset])
	offset++
	if projLen < 3 || projLen > 50 {
		return fail(StageHeaderDecode, fmt.Sprintf("project_id length %d out of range [3..50]", projLen))
	}

	// PROJ_ID.
	if len(payload) < offset+projLen {
		return fail(StageHeaderDecode, "Payload too short for PROJ_ID")
	}
	projectID := string(payload[offset : offset+projLen])
	offset += projLen
	if !projectIDPattern.MatchString(projectID) {
		return fail(StageHeaderDecode, fmt.Sprintf("project_id '%s' does not match pattern ^[a-z][a-z0-9_]{2,49}$", projectID))
	}

	// SENS_LEN: uint8.
	if len(payload) < offset+1 {
		return fail(StageHeaderDecode, "Payload too short for SENS_LEN")
	}
	sensLen := int(payload[offset])
	offset++
	if sensLen < 1 || sensLen > 64 {
		return fail(StageHeaderDecode, fmt.Sprintf("sensor_id length %d out of range [1..64]", sensLen))
	}

	// SENS_ID.
	if len(payload) < offset+sensLen {
		return fail(StageHeaderDecode, "Payload too short for SENS_ID")
	}
	sensorID := string(payload[offset : offset+sensLen])
	offset += sensLen

	// TIMESTAMP: int64 BE (Unix epoch ms UTC).
	if len(payload) < offset+8 {
		return fail(StageHeaderDecode, "Payload too short for TIMESTAMP")
	}
	epochMs := int64(binary.BigEndian.Uint64(payload[offset : offset+8]))
	offset += 8
	timestamp := time.UnixMilli(epochMs).UTC().Format("2006-01-02T15:04:05Z")

	// SAMPLE_INT: uint32 BE.
	if len(payload) < offset+4 {
		return fail(StageHeaderDecode, "Payload too short for SAMPLE_INT")
	}
	samplingInterval := int(binary.BigEndian.Uint32(payload[offset : offset+4]))
	offset += 4
	if samplingInterval < 1 || samplingInterval > 86400 {
		return fail(StageHeaderDecode, fmt.Sprintf("sampling_interval_seconds %d out of range [1..86400]", samplingInterval))
	}

	return header{
		length:           offset,
		messageID:        messageID,
		projectID:        projectID,
		sensorID:         sensorID,
		timestamp:        timestamp,
		samplingInterval: samplingInterval,
	}, nil
}

// formatUUID renders 16 bytes as a canonical lowercase hyphenated UUID,
// matching the C# FormatUuid (8-4-4-4-12, lowercase hex).
func formatUUID(b []byte) string {
	return fmt.Sprintf("%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		b[0], b[1], b[2], b[3], b[4], b[5], b[6], b[7],
		b[8], b[9], b[10], b[11], b[12], b[13], b[14], b[15])
}
