package parsing

import "testing"

func fptr(v float32) *float32 { return &v }

// TestSerialize_Golden pins the exact JSON bytes the serializer must produce.
// Expected strings follow the C# EnvelopeSerializer rules: snake_case, no
// indentation, floats rounded to 4 decimals via a decimal cast that DROPS
// trailing zeros (15.0 -> "15", 2.10 -> "2.1", 1247.5 -> "1247.5"), and
// temperature_celsius omitted when nil.
func TestSerialize_Golden(t *testing.T) {
	tests := []struct {
		name string
		env  *Envelope
		want string
	}{
		{
			name: "gnss",
			env: &Envelope{
				SchemaVersion: "1.0", MessageID: "550e8400-e29b-41d4-a716-446655440000",
				ProjectID: "shangtan", SensorID: "GXZM-DB03", SensorType: "gnss",
				Timestamp: "2026-05-02T14:30:00Z", ReceivedAt: "2026-05-02T14:30:12Z",
				SamplingIntervalSeconds: 3600,
				Payload: GnssPayload{
					DisplacementXMm: 12.34, DisplacementYMm: -5.67, DisplacementZMm: 2.10,
					DisplacementHorizontalMm: 13.57, CumulativeDisplacementMm: 1247.5, FixQuality: "rtk_fixed",
				},
			},
			want: `{"schema_version":"1.0","message_id":"550e8400-e29b-41d4-a716-446655440000",` +
				`"project_id":"shangtan","sensor_id":"GXZM-DB03","sensor_type":"gnss",` +
				`"timestamp":"2026-05-02T14:30:00Z","received_at":"2026-05-02T14:30:12Z",` +
				`"sampling_interval_seconds":3600,"payload":{` +
				`"displacement_x_mm":12.34,"displacement_y_mm":-5.67,"displacement_z_mm":2.1,` +
				`"displacement_horizontal_mm":13.57,"cumulative_displacement_mm":1247.5,` +
				`"fix_quality":"rtk_fixed"}}`,
		},
		{
			name: "rainfall",
			env: &Envelope{
				SchemaVersion: "1.0", MessageID: "7c9e6679-7425-40de-944b-e07fc1f90ae7",
				ProjectID: "shangtan", SensorID: "GXZM-RG01", SensorType: "rainfall",
				Timestamp: "2026-05-02T14:00:00Z", ReceivedAt: "2026-05-02T14:00:08Z",
				SamplingIntervalSeconds: 3600,
				Payload:                 RainfallPayload{RainfallMm: 2.4, CumulativeRainfallMm: 1547.2, IntervalMinutes: 60},
			},
			want: `{"schema_version":"1.0","message_id":"7c9e6679-7425-40de-944b-e07fc1f90ae7",` +
				`"project_id":"shangtan","sensor_id":"GXZM-RG01","sensor_type":"rainfall",` +
				`"timestamp":"2026-05-02T14:00:00Z","received_at":"2026-05-02T14:00:08Z",` +
				`"sampling_interval_seconds":3600,"payload":{` +
				`"rainfall_mm":2.4,"cumulative_rainfall_mm":1547.2,"interval_minutes":60}}`,
		},
		{
			name: "strain with temp",
			env: &Envelope{
				SchemaVersion: "1.0", MessageID: "f47ac10b-58cc-4372-a567-0e02b2c3d479",
				ProjectID: "shangtan", SensorID: "GXZM-SG02", SensorType: "strain",
				Timestamp: "2026-05-02T14:00:00Z", ReceivedAt: "2026-05-02T14:00:05Z",
				SamplingIntervalSeconds: 3600,
				Payload:                 StrainPayload{StrainMicrostrain: 234.5, TemperatureCelsius: fptr(24.3)},
			},
			want: `{"schema_version":"1.0","message_id":"f47ac10b-58cc-4372-a567-0e02b2c3d479",` +
				`"project_id":"shangtan","sensor_id":"GXZM-SG02","sensor_type":"strain",` +
				`"timestamp":"2026-05-02T14:00:00Z","received_at":"2026-05-02T14:00:05Z",` +
				`"sampling_interval_seconds":3600,"payload":{` +
				`"strain_microstrain":234.5,"temperature_celsius":24.3}}`,
		},
		{
			name: "strain no temp omits field",
			env: &Envelope{
				SchemaVersion: "1.0", MessageID: "f47ac10b-58cc-4372-a567-0e02b2c3d479",
				ProjectID: "shangtan", SensorID: "GXZM-SG02", SensorType: "strain",
				Timestamp: "2026-05-02T14:00:00Z", ReceivedAt: "2026-05-02T14:00:05Z",
				SamplingIntervalSeconds: 3600,
				Payload:                 StrainPayload{StrainMicrostrain: 234.5, TemperatureCelsius: nil},
			},
			want: `{"schema_version":"1.0","message_id":"f47ac10b-58cc-4372-a567-0e02b2c3d479",` +
				`"project_id":"shangtan","sensor_id":"GXZM-SG02","sensor_type":"strain",` +
				`"timestamp":"2026-05-02T14:00:00Z","received_at":"2026-05-02T14:00:05Z",` +
				`"sampling_interval_seconds":3600,"payload":{"strain_microstrain":234.5}}`,
		},
		{
			name: "deep_displacement drops trailing zero",
			env: &Envelope{
				SchemaVersion: "1.0", MessageID: "a3bb189e-8bf9-3888-9912-ace4e6543002",
				ProjectID: "lingzhan", SensorID: "LZ-DB01", SensorType: "deep_displacement",
				Timestamp: "2019-06-15T06:00:00Z", ReceivedAt: "2019-06-15T06:00:07Z",
				SamplingIntervalSeconds: 3600,
				Payload: DeepDisplacementPayload{
					DepthM: 15.0, TiltXMm: 3.21, TiltYMm: -1.54, CumulativeTiltMm: 48.7,
				},
			},
			want: `{"schema_version":"1.0","message_id":"a3bb189e-8bf9-3888-9912-ace4e6543002",` +
				`"project_id":"lingzhan","sensor_id":"LZ-DB01","sensor_type":"deep_displacement",` +
				`"timestamp":"2019-06-15T06:00:00Z","received_at":"2019-06-15T06:00:07Z",` +
				`"sampling_interval_seconds":3600,"payload":{` +
				`"depth_m":15,"tilt_x_mm":3.21,"tilt_y_mm":-1.54,"cumulative_tilt_mm":48.7}}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Serialize(tc.env)
			if got != tc.want {
				t.Fatalf("serialize mismatch\n got: %s\nwant: %s", got, tc.want)
			}
		})
	}
}

func TestRoundedFloatJSON(t *testing.T) {
	cases := map[float32]string{
		15.0:   "15",
		2.10:   "2.1",
		1247.5: "1247.5",
		-1.54:  "-1.54",
		0.0:    "0",
		234.5:  "234.5",
		13.57:  "13.57",
		2.4:    "2.4",
	}
	for in, want := range cases {
		if got := roundedFloatJSON(in); got != want {
			t.Fatalf("roundedFloatJSON(%v) = %q, want %q", in, got, want)
		}
	}
}
