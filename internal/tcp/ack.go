package tcp

import "github.com/AngelloFD/go-data-receiving/internal/parsing"

// buildAck builds the 5-byte ACK frame [0xAA,0x55,status,code,0xAA] sent back to
// the DTU after each frame. Port of the C# SendAckAsync wire format.
func buildAck(status, code byte) []byte {
	return []byte{0xAA, 0x55, status, code, 0xAA}
}

// ackStatus maps a parse stage to the ACK status byte, matching the C# AckStatus
// (Crc=0x01, SensorType=0x04, SchemaValidation=0x03, everything else 0x02).
func ackStatus(stage parsing.ParseStage) byte {
	switch stage {
	case parsing.StageCrc:
		return 0x01
	case parsing.StageSensorType:
		return 0x04
	case parsing.StageSchemaValidation:
		return 0x03
	default:
		return 0x02
	}
}
