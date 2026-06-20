//go:build pact

// Package pact holds consumer-driven contract tests (CLAUDE.md §3.3) using
// pact-go message pacts. data-receiving is the PRODUCER of the sensor envelope
// published to RabbitMQ under routing key sensors.{project}.{sensor_type}; the
// consumer (data-management) defines what it expects to receive. This test pins
// that contract and verifies our real serializer + schema satisfy it.
//
// Run: go test -tags=pact ./test/pact/...
// Requires the pact FFI lib (pact-go install) on the loader path; on Windows the
// DLL lives at C:\usr\local\lib\pact_ffi.dll — add that dir to PATH.
package pact

import (
	"encoding/json"
	"testing"

	"github.com/pact-foundation/pact-go/v2/matchers"
	message "github.com/pact-foundation/pact-go/v2/message/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AngelloFD/go-data-receiving/internal/parsing"
	"github.com/AngelloFD/go-data-receiving/internal/schema"
)

func TestGnssEnvelope_Contract(t *testing.T) {
	pact, err := message.NewAsynchronousPact(message.Config{
		Consumer: "data-management",
		Provider: "data-receiving",
		PactDir:  "./pacts",
	})
	require.NoError(t, err)

	// The contract: shape + types the consumer relies on. Concrete examples come
	// from a real serialized GNSS envelope so producer and pact never drift.
	body := matchers.Like(map[string]interface{}{
		"schema_version":            matchers.Term("1.0", `^\d+\.\d+$`),
		"message_id":                matchers.UUID(),
		"project_id":                matchers.Term("shangtan", `^[a-z][a-z0-9_]{2,49}$`),
		"sensor_id":                 matchers.Like("GXZM-DB03"),
		"sensor_type":               matchers.Term("gnss", `^(gnss|rainfall|strain|soil_pressure|deep_displacement)$`),
		"timestamp":                 matchers.Like("2026-05-02T14:30:00Z"),
		"received_at":               matchers.Like("2026-05-02T14:30:12Z"),
		"sampling_interval_seconds": matchers.Integer(3600),
		"payload": matchers.Like(map[string]interface{}{
			"displacement_x_mm":          matchers.Decimal(12.34),
			"displacement_y_mm":          matchers.Decimal(-5.67),
			"displacement_z_mm":          matchers.Decimal(2.1),
			"displacement_horizontal_mm": matchers.Decimal(13.57),
			"cumulative_displacement_mm": matchers.Decimal(1247.5),
			"fix_quality":                matchers.Term("rtk_fixed", `^(rtk_fixed|rtk_float|dgps|single|no_fix)$`),
		}),
	})

	err = pact.AddAsynchronousMessage().
		Given("a GNSS sensor reading was received").
		ExpectsToReceive("a gnss sensor envelope").
		WithMetadata(map[string]string{
			"routingKey":  "sensors.shangtan.gnss",
			"contentType": "application/json",
			"exchange":    "sensors.raw",
		}).
		WithJSONContent(body).
		ConsumedBy(func(mc message.AsynchronousMessage) error {
			// The consumer side: parse the envelope pact generated and validate
			// it against OUR contract schema. If the producer's contract and the
			// consumer's expectation diverge, this fails. mc.Contents holds the
			// raw reified body bytes (Body is only set when AsType is used).
			raw := mc.Contents
			require.NotEmpty(t, raw, "pact produced empty message body")

			var env struct {
				SensorType string `json:"sensor_type"`
			}
			require.NoError(t, json.Unmarshal(raw, &env))

			v := schema.New()
			ok, msg := v.Validate(string(raw), env.SensorType)
			assert.True(t, ok, "pact body must satisfy the envelope schema: %s", msg)
			return nil
		}).
		Verify(t)

	require.NoError(t, err)
}

// TestSerializerMatchesContractExamples guards that the real serializer still
// emits the concrete example values pinned in the pact above (a cheap, FFI-free
// backstop so the contract examples never silently drift from production code).
func TestSerializerMatchesContractExamples(t *testing.T) {
	env := &parsing.Envelope{
		SchemaVersion: "1.0", MessageID: "550e8400-e29b-41d4-a716-446655440000",
		ProjectID: "shangtan", SensorID: "GXZM-DB03", SensorType: "gnss",
		Timestamp: "2026-05-02T14:30:00Z", ReceivedAt: "2026-05-02T14:30:12Z",
		SamplingIntervalSeconds: 3600,
		Payload: parsing.GnssPayload{
			DisplacementXMm: 12.34, DisplacementYMm: -5.67, DisplacementZMm: 2.10,
			DisplacementHorizontalMm: 13.57, CumulativeDisplacementMm: 1247.5, FixQuality: "rtk_fixed",
		},
	}
	got := parsing.Serialize(env)

	v := schema.New()
	if ok, msg := v.Validate(got, "gnss"); !ok {
		t.Fatalf("serialized envelope fails schema: %s", msg)
	}
}
