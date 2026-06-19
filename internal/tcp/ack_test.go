package tcp

import (
	"bytes"
	"testing"

	"github.com/AngelloFD/go-data-receiving/internal/parsing"
)

func TestBuildAck(t *testing.T) {
	if got := buildAck(0x00, 0x00); !bytes.Equal(got, []byte{0xAA, 0x55, 0x00, 0x00, 0xAA}) {
		t.Fatalf("success ack = % X", got)
	}
	if got := buildAck(0x01, 0x04); !bytes.Equal(got, []byte{0xAA, 0x55, 0x01, 0x04, 0xAA}) {
		t.Fatalf("error ack = % X", got)
	}
}

func TestAckStatus(t *testing.T) {
	cases := map[parsing.ParseStage]byte{
		parsing.StageCrc:              0x01,
		parsing.StageSensorType:       0x04,
		parsing.StageSchemaValidation: 0x03,
		parsing.StageSyncBytes:        0x02,
		parsing.StageVersion:          0x02,
		parsing.StageLength:           0x02,
		parsing.StageHeaderDecode:     0x02,
		parsing.StagePayloadDecode:    0x02,
	}
	for stage, want := range cases {
		if got := ackStatus(stage); got != want {
			t.Fatalf("ackStatus(%s) = 0x%02X, want 0x%02X", stage, got, want)
		}
	}
}
