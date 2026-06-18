package parsing

import (
	"encoding/binary"
	"fmt"
	"math"
)

// readFloat32BE reads a big-endian IEEE-754 single, matching the C#
// BinaryPrimitives.ReadSingleBigEndian used by every payload decoder.
func readFloat32BE(b []byte) float32 {
	return math.Float32frombits(binary.BigEndian.Uint32(b))
}

func payloadFail(msg string) *ParseError {
	return &ParseError{Stage: StagePayloadDecode, Message: msg}
}

var fixQualityMap = [...]string{"no_fix", "single", "dgps", "rtk_float", "rtk_fixed"}

// decodeGnss: 4+4+4+4+4+1 = 21 bytes.
func decodeGnss(data []byte) (Payload, *ParseError) {
	if len(data) < 21 {
		return nil, payloadFail("GNSS payload too short, need 21 bytes")
	}
	dispX := readFloat32BE(data[0:4])
	dispY := readFloat32BE(data[4:8])
	dispZ := readFloat32BE(data[8:12])
	dispH := readFloat32BE(data[12:16])
	cumul := readFloat32BE(data[16:20])
	fixByte := data[20]

	if dispH < 0 {
		return nil, payloadFail(fmt.Sprintf("displacement_horizontal_mm %s must be >= 0", floatErr(dispH)))
	}
	if cumul < 0 {
		return nil, payloadFail(fmt.Sprintf("cumulative_displacement_mm %s must be >= 0", floatErr(cumul)))
	}
	if fixByte > 4 {
		return nil, payloadFail(fmt.Sprintf("Unknown fix_quality byte 0x%02X", fixByte))
	}

	return GnssPayload{
		DisplacementXMm:          dispX,
		DisplacementYMm:          dispY,
		DisplacementZMm:          dispZ,
		DisplacementHorizontalMm: dispH,
		CumulativeDisplacementMm: cumul,
		FixQuality:               fixQualityMap[fixByte],
	}, nil
}

// decodeRainfall: 4+4+2 = 10 bytes.
func decodeRainfall(data []byte) (Payload, *ParseError) {
	if len(data) < 10 {
		return nil, payloadFail("Rainfall payload too short, need 10 bytes")
	}
	rainfallMm := readFloat32BE(data[0:4])
	cumulMm := readFloat32BE(data[4:8])
	intervalMin := int(data[8])<<8 | int(data[9])

	if rainfallMm < 0 {
		return nil, payloadFail(fmt.Sprintf("rainfall_mm %s must be >= 0", floatErr(rainfallMm)))
	}
	if cumulMm < 0 {
		return nil, payloadFail(fmt.Sprintf("cumulative_rainfall_mm %s must be >= 0", floatErr(cumulMm)))
	}
	if intervalMin < 1 || intervalMin > 1440 {
		return nil, payloadFail(fmt.Sprintf("interval_minutes %d out of range [1..1440]", intervalMin))
	}

	return RainfallPayload{RainfallMm: rainfallMm, CumulativeRainfallMm: cumulMm, IntervalMinutes: intervalMin}, nil
}

// decodeStrain: HAS_TEMP(1)+strain(4) = 5 min, +temp(4) = 9 with temperature.
func decodeStrain(data []byte) (Payload, *ParseError) {
	if len(data) < 5 {
		return nil, payloadFail("Strain payload too short, need at least 5 bytes")
	}
	hasTemp := data[0]
	strain := readFloat32BE(data[1:5])

	var temperature *float32
	switch hasTemp {
	case 0x01:
		if len(data) < 9 {
			return nil, payloadFail("Strain payload too short for temperature, need 9 bytes when HAS_TEMP=0x01")
		}
		temp := readFloat32BE(data[5:9])
		if temp < -40 || temp > 80 {
			return nil, payloadFail(fmt.Sprintf("temperature_celsius %s out of range [-40..80]", floatErr(temp)))
		}
		temperature = &temp
	case 0x00:
		// no temperature
	default:
		return nil, payloadFail(fmt.Sprintf("Invalid HAS_TEMP byte 0x%02X, expected 0x00 or 0x01", hasTemp))
	}

	return StrainPayload{StrainMicrostrain: strain, TemperatureCelsius: temperature}, nil
}

// decodeSoilPressure: HAS_TEMP(1)+pressure(4) = 5 min, +temp(4) = 9 with temperature.
func decodeSoilPressure(data []byte) (Payload, *ParseError) {
	if len(data) < 5 {
		return nil, payloadFail("Soil pressure payload too short, need at least 5 bytes")
	}
	hasTemp := data[0]
	pressure := readFloat32BE(data[1:5])

	var temperature *float32
	switch hasTemp {
	case 0x01:
		if len(data) < 9 {
			return nil, payloadFail("Soil pressure payload too short for temperature, need 9 bytes when HAS_TEMP=0x01")
		}
		temp := readFloat32BE(data[5:9])
		if temp < -40 || temp > 80 {
			return nil, payloadFail(fmt.Sprintf("temperature_celsius %s out of range [-40..80]", floatErr(temp)))
		}
		temperature = &temp
	case 0x00:
		// no temperature
	default:
		return nil, payloadFail(fmt.Sprintf("Invalid HAS_TEMP byte 0x%02X, expected 0x00 or 0x01", hasTemp))
	}

	return SoilPressurePayload{PressureKpa: pressure, TemperatureCelsius: temperature}, nil
}

// decodeDeepDisplacement: depth(4)+tiltX(4)+tiltY(4)+cumul(4) = 16 bytes.
func decodeDeepDisplacement(data []byte) (Payload, *ParseError) {
	if len(data) < 16 {
		return nil, payloadFail("Deep displacement payload too short, need 16 bytes")
	}
	depth := readFloat32BE(data[0:4])
	tiltX := readFloat32BE(data[4:8])
	tiltY := readFloat32BE(data[8:12])
	cumul := readFloat32BE(data[12:16])

	if depth < 0 {
		return nil, payloadFail(fmt.Sprintf("depth_m %s must be >= 0", floatErr(depth)))
	}

	return DeepDisplacementPayload{DepthM: depth, TiltXMm: tiltX, TiltYMm: tiltY, CumulativeTiltMm: cumul}, nil
}
