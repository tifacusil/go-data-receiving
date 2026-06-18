package schema

import "embed"

// schemaFS holds the JSON Schema contracts compiled into the binary. The files
// are a synced copy of contracts/schemas/*.schema.json (go:embed cannot reach
// ../../contracts). Keep them in sync via `make sync-schemas`; CI guards drift.
//
//go:embed schemas/*.json
var schemaFS embed.FS
