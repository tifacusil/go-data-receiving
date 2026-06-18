package parsing

import (
	"strconv"
	"strings"
)

// Serialize renders an Envelope to the exact JSON the C# EnvelopeSerializer
// produces: snake_case keys, no indentation, floats rounded to 4 decimals with
// trailing zeros dropped, and temperature_celsius omitted when nil.
//
// JSON is built by hand (not encoding/json) to guarantee byte-equal output:
// field order, float formatting and null-omission must match C# precisely,
// which struct-tag marshaling cannot guarantee.
func Serialize(e *Envelope) string {
	var b strings.Builder
	b.WriteByte('{')

	writeString(&b, "schema_version", e.SchemaVersion)
	b.WriteByte(',')
	writeString(&b, "message_id", e.MessageID)
	b.WriteByte(',')
	writeString(&b, "project_id", e.ProjectID)
	b.WriteByte(',')
	writeString(&b, "sensor_id", e.SensorID)
	b.WriteByte(',')
	writeString(&b, "sensor_type", e.SensorType)
	b.WriteByte(',')
	writeString(&b, "timestamp", e.Timestamp)
	b.WriteByte(',')
	writeString(&b, "received_at", e.ReceivedAt)
	b.WriteByte(',')
	writeKey(&b, "sampling_interval_seconds")
	b.WriteString(strconv.Itoa(e.SamplingIntervalSeconds))
	b.WriteByte(',')
	writeKey(&b, "payload")
	writePayload(&b, e.Payload)

	b.WriteByte('}')
	return b.String()
}

func writePayload(b *strings.Builder, p Payload) {
	b.WriteByte('{')
	switch v := p.(type) {
	case GnssPayload:
		writeFloat(b, "displacement_x_mm", v.DisplacementXMm)
		b.WriteByte(',')
		writeFloat(b, "displacement_y_mm", v.DisplacementYMm)
		b.WriteByte(',')
		writeFloat(b, "displacement_z_mm", v.DisplacementZMm)
		b.WriteByte(',')
		writeFloat(b, "displacement_horizontal_mm", v.DisplacementHorizontalMm)
		b.WriteByte(',')
		writeFloat(b, "cumulative_displacement_mm", v.CumulativeDisplacementMm)
		b.WriteByte(',')
		writeString(b, "fix_quality", v.FixQuality)
	case RainfallPayload:
		writeFloat(b, "rainfall_mm", v.RainfallMm)
		b.WriteByte(',')
		writeFloat(b, "cumulative_rainfall_mm", v.CumulativeRainfallMm)
		b.WriteByte(',')
		writeKey(b, "interval_minutes")
		b.WriteString(strconv.Itoa(v.IntervalMinutes))
	case StrainPayload:
		writeFloat(b, "strain_microstrain", v.StrainMicrostrain)
		if v.TemperatureCelsius != nil {
			b.WriteByte(',')
			writeFloat(b, "temperature_celsius", *v.TemperatureCelsius)
		}
	case SoilPressurePayload:
		writeFloat(b, "pressure_kpa", v.PressureKpa)
		if v.TemperatureCelsius != nil {
			b.WriteByte(',')
			writeFloat(b, "temperature_celsius", *v.TemperatureCelsius)
		}
	case DeepDisplacementPayload:
		writeFloat(b, "depth_m", v.DepthM)
		b.WriteByte(',')
		writeFloat(b, "tilt_x_mm", v.TiltXMm)
		b.WriteByte(',')
		writeFloat(b, "tilt_y_mm", v.TiltYMm)
		b.WriteByte(',')
		writeFloat(b, "cumulative_tilt_mm", v.CumulativeTiltMm)
	}
	b.WriteByte('}')
}

func writeKey(b *strings.Builder, key string) {
	b.WriteByte('"')
	b.WriteString(key)
	b.WriteString(`":`)
}

func writeString(b *strings.Builder, key, val string) {
	writeKey(b, key)
	// All string fields here are regex-constrained ASCII (project_id, sensor_id,
	// uuid, enum fix_quality, ISO timestamps) so strconv.Quote's escaping matches
	// System.Text.Json output. If a free-text string field is ever added, revisit:
	// C# escapes <, >, & and non-ASCII differently from strconv.Quote.
	b.WriteString(strconv.Quote(val))
}

func writeFloat(b *strings.Builder, key string, val float32) {
	writeKey(b, key)
	b.WriteString(roundedFloatJSON(val))
}
