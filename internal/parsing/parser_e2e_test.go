package parsing_test

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/AngelloFD/go-data-receiving/internal/parsing"
	"github.com/AngelloFD/go-data-receiving/internal/schema"
)

// End-to-end: binary frame -> validated envelope, using the real schema
// validator. Port of the C# EndToEndParserTests.

func f32(v float32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, math.Float32bits(v))
	return b
}

func header(projectID, sensorID string, epochMs int64, sampling uint32) []byte {
	b := make([]byte, 0, 64)
	b = append(b, make([]byte, 16)...) // zero UUID
	b = append(b, byte(len(projectID)))
	b = append(b, projectID...)
	b = append(b, byte(len(sensorID)))
	b = append(b, sensorID...)
	ts := make([]byte, 8)
	binary.BigEndian.PutUint64(ts, uint64(epochMs))
	b = append(b, ts...)
	si := make([]byte, 4)
	binary.BigEndian.PutUint32(si, sampling)
	b = append(b, si...)
	return b
}

func frame(typeCode byte, payload []byte) []byte {
	f := []byte{0xAA, 0x55, 0x01, typeCode, byte(len(payload) >> 8), byte(len(payload))}
	f = append(f, payload...)
	crc := parsing.CRC16(f[2:])
	f = append(f, byte(crc>>8), byte(crc))
	f = append(f, 0x55, 0xAA)
	return f
}

func newParser() *parsing.Parser { return parsing.NewParser(schema.New()) }

func TestE2E_AllSensorTypes(t *testing.T) {
	p := newParser()

	gnss := append(header("shangtan", "GXZM-DB03", 1714660200000, 3600),
		concat(f32(12.34), f32(-5.67), f32(2.10), f32(13.57), f32(1247.5), []byte{0x04})...)

	rain := append(header("shangtan", "GXZM-RG01", 1714658400000, 3600),
		concat(f32(2.4), f32(1547.2), []byte{0x00, 0x3C})...)

	strain := append(header("shangtan", "GXZM-SG02", 1714658400000, 3600),
		concat([]byte{0x01}, f32(234.5), f32(24.3))...)

	soil := append(header("shangtan", "GXZM-SP01", 1714658400000, 3600),
		concat([]byte{0x01}, f32(45.2), f32(18.7))...)

	deep := append(header("lingzhan", "LZ-DB01", 1560578400000, 3600),
		concat(f32(15), f32(3.21), f32(-1.54), f32(48.7))...)

	cases := []struct {
		name       string
		typeCode   byte
		payload    []byte
		sensorType string
	}{
		{"gnss", 0x01, gnss, "gnss"},
		{"rainfall", 0x02, rain, "rainfall"},
		{"strain", 0x03, strain, "strain"},
		{"soil_pressure", 0x04, soil, "soil_pressure"},
		{"deep_displacement", 0x05, deep, "deep_displacement"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := p.Parse(frame(tc.typeCode, tc.payload))
			if !r.Success {
				t.Fatalf("parse failed: %s (%s)", r.Error.Message, r.Error.Stage)
			}
			if r.Envelope.SensorType != tc.sensorType {
				t.Fatalf("sensor_type = %s, want %s", r.Envelope.SensorType, tc.sensorType)
			}
			if r.Envelope.SchemaVersion != "1.0" {
				t.Fatalf("schema_version = %s", r.Envelope.SchemaVersion)
			}
		})
	}
}

func TestE2E_StrainNoTemp(t *testing.T) {
	p := newParser()
	payload := append(header("shangtan", "GXZM-SG02", 1714658400000, 3600),
		concat([]byte{0x00}, f32(234.5))...)
	r := p.Parse(frame(0x03, payload))
	if !r.Success {
		t.Fatalf("parse failed: %s", r.Error.Message)
	}
	if r.Envelope.Payload.(parsing.StrainPayload).TemperatureCelsius != nil {
		t.Fatal("expected nil temperature")
	}
}

func TestE2E_CorruptCRCRejected(t *testing.T) {
	p := newParser()
	payload := append(header("shangtan", "GXZM-DB03", 1714660200000, 3600),
		concat(f32(0), f32(0), f32(0), f32(0), f32(0), []byte{0x00})...)
	f := frame(0x01, payload)
	f[len(f)-3] ^= 0xFF // corrupt CRC byte
	r := p.Parse(f)
	if r.Success || r.Error.Stage != parsing.StageCrc {
		t.Fatalf("want Crc failure, got success=%v err=%+v", r.Success, r.Error)
	}
}

func TestE2E_UnknownTypeRejected(t *testing.T) {
	p := newParser()
	// Build a structurally valid frame then set an out-of-range type byte.
	payload := append(header("shangtan", "GXZM-DB03", 1714660200000, 3600),
		concat(f32(0), f32(0), f32(0), f32(0), f32(0), []byte{0x00})...)
	f := frame(0x09, payload) // 0x09 not a known type
	r := p.Parse(f)
	if r.Success || r.Error.Stage != parsing.StageSensorType {
		t.Fatalf("want SensorType failure, got success=%v err=%+v", r.Success, r.Error)
	}
}

func concat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}
