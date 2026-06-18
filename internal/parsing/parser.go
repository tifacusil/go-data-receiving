package parsing

import (
	"fmt"
	"time"
)

// SchemaValidator validates a serialized envelope against the contract schemas.
// The interface is declared here (where it is consumed) and satisfied by
// internal/schema.Validator — the Go-idiomatic equivalent of injecting the C#
// SchemaValidator dependency.
type SchemaValidator interface {
	Validate(envelopeJSON string, sensorType string) (bool, string)
}

var typeToSensorType = map[byte]string{
	0x01: "gnss",
	0x02: "rainfall",
	0x03: "strain",
	0x04: "soil_pressure",
	0x05: "deep_displacement",
}

// Parser turns a raw DTU frame into a validated Envelope. Ported from the C#
// DtuPacketParser. It is stateless and safe for concurrent use.
type Parser struct {
	validator SchemaValidator
	now       func() time.Time // injectable clock; defaults to time.Now
}

// NewParser builds a Parser with the given schema validator.
func NewParser(validator SchemaValidator) *Parser {
	return &Parser{validator: validator, now: time.Now}
}

// Parse runs the four-stage pipeline: frame/CRC validation, header decode,
// payload decode, schema validation. Returns a ParseResult mirroring C#.
func (p *Parser) Parse(rawFrame []byte) ParseResult {
	receivedAt := p.now().UTC().Format("2006-01-02T15:04:05Z")

	// Stage 1: frame structure + CRC.
	if v := validateFrame(rawFrame); !v.Success {
		return v
	}

	typeCode := rawFrame[3]
	sensorType := typeToSensorType[typeCode]

	// Payload region is bytes 6..(len-4).
	payload := rawFrame[6 : len(rawFrame)-4]

	// Stage 2: header decode.
	hdr, hErr := decodeHeader(payload)
	if hErr != nil {
		return ParseResult{Success: false, Error: hErr}
	}

	sensorPayloadData := payload[hdr.length:]

	// Stage 3: payload decode.
	sensorPayload, pErr := decodePayload(sensorType, sensorPayloadData)
	if pErr != nil {
		return ParseResult{Success: false, Error: pErr}
	}

	envelope := &Envelope{
		SchemaVersion:           "1.0",
		MessageID:               hdr.messageID,
		ProjectID:               hdr.projectID,
		SensorID:                hdr.sensorID,
		SensorType:              sensorType,
		Timestamp:               hdr.timestamp,
		ReceivedAt:              receivedAt,
		SamplingIntervalSeconds: hdr.samplingInterval,
		Payload:                 sensorPayload,
	}

	// Stage 4: JSON Schema validation.
	json := Serialize(envelope)
	valid, schemaErr := p.validator.Validate(json, sensorType)
	if !valid {
		if schemaErr == "" {
			schemaErr = "Schema validation failed"
		}
		return failResult(StageSchemaValidation, schemaErr)
	}

	return ParseResult{Success: true, Envelope: envelope}
}

func decodePayload(sensorType string, data []byte) (Payload, *ParseError) {
	switch sensorType {
	case "gnss":
		return decodeGnss(data)
	case "rainfall":
		return decodeRainfall(data)
	case "strain":
		return decodeStrain(data)
	case "soil_pressure":
		return decodeSoilPressure(data)
	case "deep_displacement":
		return decodeDeepDisplacement(data)
	default:
		return nil, &ParseError{Stage: StageSensorType, Message: fmt.Sprintf("Unknown sensor type: %s", sensorType)}
	}
}
