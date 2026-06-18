package parsing

// ParseStage identifies the pipeline stage where parsing failed. Ordinal values
// match the C# ParseStage enum exactly — the TCP ACK protocol encodes the stage
// as a byte, so these must not be reordered.
type ParseStage int

const (
	StageSyncBytes ParseStage = iota
	StageVersion
	StageSensorType
	StageLength
	StageCrc
	StageHeaderDecode
	StagePayloadDecode
	StageSchemaValidation
)

// String returns the stage name as written to the dead-letter record, matching
// the C# enum member names.
func (s ParseStage) String() string {
	switch s {
	case StageSyncBytes:
		return "SyncBytes"
	case StageVersion:
		return "Version"
	case StageSensorType:
		return "SensorType"
	case StageLength:
		return "Length"
	case StageCrc:
		return "Crc"
	case StageHeaderDecode:
		return "HeaderDecode"
	case StagePayloadDecode:
		return "PayloadDecode"
	case StageSchemaValidation:
		return "SchemaValidation"
	default:
		return "Unknown"
	}
}

// ParseError describes why a frame was rejected.
type ParseError struct {
	Stage   ParseStage
	Message string
}

// ParseResult is the outcome of Parse. Exactly one of Envelope/Error is set on
// success/failure respectively (mirrors the C# record ParseResult).
type ParseResult struct {
	Success  bool
	Envelope *Envelope
	Error    *ParseError
}

func failResult(stage ParseStage, msg string) ParseResult {
	return ParseResult{Success: false, Error: &ParseError{Stage: stage, Message: msg}}
}

// Envelope is the parsed sensor message. SchemaVersion is fixed at "1.0".
// Payload holds one of the *Payload types below.
type Envelope struct {
	SchemaVersion           string
	MessageID               string
	ProjectID               string
	SensorID                string
	SensorType              string
	Timestamp               string
	ReceivedAt              string
	SamplingIntervalSeconds int
	Payload                 Payload
}

// Payload is the marker interface for the five sensor-specific payloads.
type Payload interface{ isPayload() }

// GnssPayload — sensor_type = gnss.
type GnssPayload struct {
	DisplacementXMm          float32
	DisplacementYMm          float32
	DisplacementZMm          float32
	DisplacementHorizontalMm float32
	CumulativeDisplacementMm float32
	FixQuality               string
}

// RainfallPayload — sensor_type = rainfall.
type RainfallPayload struct {
	RainfallMm           float32
	CumulativeRainfallMm float32
	IntervalMinutes      int
}

// StrainPayload — sensor_type = strain. TemperatureCelsius is optional.
type StrainPayload struct {
	StrainMicrostrain  float32
	TemperatureCelsius *float32
}

// SoilPressurePayload — sensor_type = soil_pressure. TemperatureCelsius is optional.
type SoilPressurePayload struct {
	PressureKpa        float32
	TemperatureCelsius *float32
}

// DeepDisplacementPayload — sensor_type = deep_displacement.
type DeepDisplacementPayload struct {
	DepthM           float32
	TiltXMm          float32
	TiltYMm          float32
	CumulativeTiltMm float32
}

func (GnssPayload) isPayload()             {}
func (RainfallPayload) isPayload()         {}
func (StrainPayload) isPayload()           {}
func (SoilPressurePayload) isPayload()     {}
func (DeepDisplacementPayload) isPayload() {}
