# go-data-receiving

Go rewrite of the `data-receiving` microservice from the landslide monitoring
system (Bai et al. 2020). Receives binary DTU frames over TCP, validates and
parses them, and publishes JSON envelopes to RabbitMQ with Redis as a durability
buffer and Consul for service discovery.

Reference baseline: `data-receiving-module-csharp` (C#/.NET 8).
Engineering guide: [`CLAUDE.md`](./CLAUDE.md). Milestone plan: [`ROADMAP.md`](./ROADMAP.md).

## Status

Full rewrite (H0–H7). The service replicates the *observable* behaviour of the C#
baseline (frame protocol, CRC, byte-identical serialization, ACK, Redis→RabbitMQ
flow) so the Go vs C# performance comparison stays fair, and adds — as a
non-intrusive layer — the SE practices the baseline lacked (real health checks,
metrics, tracing, CDCT). See [Engineering practices](#engineering-practices-applied).

## Architecture

```
DTU sensor (TCP)
   │
   ▼  internal/tcp        length-prefixed framereader, ACK, accept loop
   ▼  internal/parsing    CRC-16 → frame → header → payload → serializer
   ▼  internal/schema     JSON Schema 2020-12 validation (embedded)
   ▼  internal/sink        ┌─ stdout / file / null      (local modes)
                           └─ rabbitmq:
                                 internal/buffer     Set in Redis  (sensor_buffer:{id})
                                 internal/messaging  Publish with confirms → sensors.raw
                                 internal/buffer     Delete after confirm (+ RetryWorker)
   ▼  internal/deadletter  invalid frames → JSON on disk (never block)

Side workers (rabbitmq mode):
   internal/health     /health (TCP+Redis+RabbitMQ) and /metrics
   internal/discovery  register/deregister in Consul
```

`internal/app` is the composition root that wires everything; `cmd/data-receiving/main.go`
only handles signals and delegates.

## Local development

Requires Go 1.23+.

```bash
make build          # builds ./bin/data-receiving
make run            # runs in stdout mode (no external services)
make test           # unit + contract tests (no Docker, -race)
make lint           # golangci-lint v2
make test-integration   # integration vs live infra (redis/rabbitmq/consul)
make test-pact          # CDCT message pact (requires the pact FFI lib)
go run ./cmd/data-receiving version   # prints the version (sha in the image)
```

## Configuration

All configuration is externalized via environment variables with the `RECEIVER__`
prefix and `__` as the section separator (1:1 parity with the C# baseline).
Secrets (RabbitMQ password, Consul token) **never** go in `.env` or the repo: they
live in `/etc/data-receiving/secrets` on servers. Keys and defaults:
`internal/config/config.go` and `.env.example`.

`OutputMode` (`stdout`|`file`|`null`|`rabbitmq`) decides what the composition root
wires. Local modes don't build Redis/RabbitMQ/Consul/health — handy for dev and
tests without infra.

## Contracts

`contracts/` is the source of truth (JSON Schema Draft 2020-12 + examples + the
`tarea-0-contratos.md` document). The validator embeds a copy under
`internal/schema/schemas` via `go:embed`; `make sync-schemas` regenerates it and
`make check-schemas` fails (in CI) if they drift.

## Deployment

`Dockerfile` (multi-stage, static binary on a distroless image, ~21 MB) in this
repo. CI/CD in `.github/workflows/`: CI (lint + build + test) on every push/PR; CD
builds and publishes the image to GHCR (`ghcr.io/tifacusil/go-data-receiving`,
tags `latest` + sha) after a green CI.

The real deployment (the 3 instances) lives in **tesis-infra**:
`local/services/data-receiving/docker-compose.yml`. This repo only ships the image.

### Run in a container locally

Requires the base infra (Redis, RabbitMQ, Consul) running on the Docker network
`landslide-net` — brought up by `tesis-infra/local/infra/docker-compose.yml`.

**Option A — replicate the full deployment (3 instances)** via the tesis-infra
compose. Build the image with the tag the compose expects and bring it up:

```bash
# 1. from this repo: build the image with the name the compose uses
docker build -f Dockerfile -t ghcr.io/tifacusil/go-data-receiving:latest .

# 2. from tesis-infra: bring up the 3 instances
cd <path>/tesis-infra/local/services/data-receiving
docker compose up -d
docker compose ps
```

They land on `5000/5001/5002` (TCP) and `8080/8081/8082` (HTTP). Verify:

```bash
curl http://localhost:8080/health/   # {"status":"ok","checks":{"tcp":true,"redis":true,"rabbitmq":true}}
curl http://localhost:8080/metrics   # Prometheus exposition
curl http://localhost:8500/v1/health/service/data-receiving   # 3 instances in Consul
```

**Option B — a single instance** without compose (quick check), joined to
`landslide-net` and using the same `RECEIVER__*` values as the shared tesis-infra
env:

```bash
docker build -f Dockerfile -t go-data-receiving:local .

docker run -d --name data-receiving-local \
  --restart unless-stopped \
  --network landslide-net \
  -p 5000:5000 -p 8080:8080 \
  -e RECEIVER__OUTPUTMODE=rabbitmq \
  -e RECEIVER__PORT=5000 \
  -e RECEIVER__DLXPATH=/tmp/dlx \
  -e "RECEIVER__REDIS__CONNECTIONSTRING=redis:6379,password=changeme_redis" \
  -e RECEIVER__RABBITMQ__HOST=rabbitmq-1 \
  -e RECEIVER__RABBITMQ__USERNAME=data_receiving \
  -e RECEIVER__RABBITMQ__PASSWORD=changeme_data_receiving \
  -e RECEIVER__CONSUL__ADDRESS=http://consul-1:8500 \
  -e RECEIVER__CONSUL__INSTANCEID=local-01 \
  -e RECEIVER__CONSUL__HEALTHPORT=8080 \
  -e RECEIVER__CONSUL__HEALTHCHECKHOST=data-receiving-local \
  go-data-receiving:local
```

Runs detached (`-d`) and persists (`--restart unless-stopped`); it isn't removed
on stop. Manage it:

```bash
docker logs -f data-receiving-local   # view logs
docker stop data-receiving-local      # stop (does not remove it)
docker start data-receiving-local     # start again
docker rm -f data-receiving-local     # remove when no longer needed
```

`HEALTHCHECKHOST` must be the container hostname/name (not `localhost`): Consul runs
in another container and reaches `/health` by that name within `landslide-net`.

---

## Engineering practices applied

The practices follow the catalogue in [`CLAUDE.md`](./CLAUDE.md) (backed by El Fellah
et al. 2026, Schwarz et al. 2025, Saleh Sedghpour et al. 2022, Taibi & Lenarduzzi 2018).
Each is listed with **where** it lives in the code and **why** it was applied.

### Microservice design

- **Contract-first.** `contracts/schemas/*.schema.json` (JSON Schema 2020-12) define the
  envelope and the 5 payloads *before* any handler. The producer validates against them
  in `internal/schema/validator.go` before publishing. *Why:* a message that doesn't meet
  the contract never reaches `sensors.raw`; the consumers (Data Management, Early Warning)
  depend on a stable shape.

- **Database/buffer-per-service.** The service only touches its own Redis with the
  `sensor_buffer:` prefix (`internal/buffer/redis.go`). It reads no other service's tables.
  *Why:* avoids the *Shared Persistency* bad smell (Taibi & Lenarduzzi 2018); each service
  owns its data.

- **Idempotency / deduplication key.** `message_id` (UUID) travels in the envelope, is the
  Redis buffer key, and is the `MessageId` AMQP property
  (`internal/messaging/publisher.go`). *Why:* lets consumers deduplicate under
  *at-least-once* delivery; *exactly-once* is never assumed.

- **Contract versioning.** `schema_version` fixed at `"1.0"` in the envelope, semver
  pattern in the schema. *Why:* future changes follow the additive→minor /
  incompatible→major rule.

### Resilience and durability

- **Outbox / durability buffer (Set→Publish→Delete).** `internal/sink/rabbitmq.go` writes
  to Redis *before* publishing and deletes only after the broker confirms; if the publish
  fails, the entry stays in Redis and `Write` returns `nil` (success ACK to the sensor).
  *Why:* no parsed frame is lost even when RabbitMQ is down.

- **Publisher Confirms.** `internal/messaging/publisher.go` waits for the broker ack with a
  timeout before considering a message published. *Why:* without confirms, "publishing"
  doesn't guarantee the broker accepted it.

- **Bounded retry + local Dead Letter.** `internal/buffer/retry_worker.go` rescans Redis and
  republishes stuck entries; invalid frames go to an on-disk dead-letter
  (`internal/deadletter/local.go`) and never block the pipeline. *Why:* automatic recovery
  after outages + inspection of rejected frames.
  *(The retry interval is a fixed 30s, 1:1 with the baseline; exponential backoff is marked
  with `// TODO(SE-practice):` to keep the experiment comparable.)*

- **Real health checks.** `internal/health/checker.go` verifies the TCP listener is up **and**
  actively pings Redis and RabbitMQ; returns `503` if a critical dependency is down. *Why:*
  a `/health` that just returns a static `200` lies; Consul must pull an instance with
  broken dependencies out of rotation.

- **Service discovery with auto-deregister.** `internal/discovery/consul.go` registers the
  service with an HTTP health check and deregisters it on shutdown (`Run` blocks on `ctx`
  and cleans up in the defer). *Why:* the load balancer only routes to live instances;
  graceful shutdown yields 0 frame-loss when rotating instances.

### 12-factor / operation

- **Externalized configuration.** `internal/config/config.go` maps everything from env
  (`RECEIVER__*`); zero hardcoded hosts/ports/credentials. Secrets outside the repo. *Why:*
  the same binary in dev/staging/prod by only changing the environment.

- **Structured logging (JSON) with correlation.** `internal/observability/logging.go`
  (slog JSON) adds `service`/`instance_id`/`version` to every line; per-frame logs carry
  `message_id`. *Why:* follow a message across TCP → RabbitMQ → consumers.

- **Prometheus metrics.** `internal/observability/metrics.go`: `frames_total{result,
  sensor_type}`, latency histogram (p50/p90/p95/p99), published/dropped, process CPU and
  resident memory; served on `/metrics`. *Why:* the project's goal is to empirically compare
  throughput/latency/CPU/memory Go vs C#.

- **Distributed tracing.** `internal/observability/tracing.go` opens a span per frame
  correlated by `message_id` (no-op without a configured exporter). *Why:* trace the
  end-to-end flow once a collector is available.

- **Non-intrusive observability.** Instrumentation is injected via
  `tcp.Server.WithObservability` and is nil-safe. *Why:* measure without altering the
  byte-for-byte flow the performance comparison needs intact.

### Code architecture

- **Composition root + constructor injection (no DI framework).** `internal/app/` assembles
  the dependencies and runs them as goroutines under an `errgroup` with a cancellable
  `context`; `cmd/data-receiving/main.go` only sets up `signal.NotifyContext` and delegates.
  *Why:* the idiomatic Go equivalent of .NET's host with `BackgroundService`, no magic.

- **Graceful shutdown.** SIGINT/SIGTERM cancel the `context` → the `errgroup` propagates →
  each worker closes in its `defer` (incl. Consul deregistration). *Why:* terminate cleanly
  without losing in-flight messages.

- **Ports and adapters (small interfaces, defined on the consumer side).** `sink.Sink`,
  `buffer.Buffer`, `messaging.Publisher`, `parsing.SchemaValidator`, `tcp.Observer`/
  `tcp.Tracer`. *Why:* decouples the domain from infrastructure and makes test fakes trivial.

- **`internal/` for encapsulation.** All the domain lives in `internal/` (the compiler
  forbids importing it from outside the module); `pkg/` is not used. *Why:* it's a
  microservice, not a library; nothing should be imported by third parties.

- **Pure I/O-free core.** The parser (`internal/parsing`) operates on `[]byte` with no
  sockets or services. *Why:* testable with hardcoded byte arrays, fast, deterministic.

### Quality and CI/CD (test pyramid)

- **Unit tests** — isolated logic with mocks/fakes (CRC, decoders, ACK, sink with a fake
  buffer, etc.).
- **Serialization golden test** (`internal/parsing/serializer_test.go`) — pins the exact
  JSON bytes, byte-identical to C# (including zero dropping: `15.0`→`15`, `2.10`→`2.1`).
  *Why:* output parity is the experiment's measurable contract.
- **Contract tests** (`internal/schema/validator_test.go`) — the `contracts/` examples
  validate against the embedded schemas.
- **Integration tests** (`test/integration/`, build tag `integration`) — real Redis +
  RabbitMQ + Consul: publish→queue + empty buffer, broker down→persists→RetryWorker
  republishes, register/deregister in Consul.
- **CDCT with pact-go** (`test/pact/`, build tag `pact`) — AMQP message pact: the consumer
  `data-management` defines what it expects and the handler validates the body against *our*
  schema. *Why:* detects producer-consumer divergence without standing up the whole system
  (Schwarz et al. 2025).
- **CI/CD** (`.github/workflows/`) — CI: tidy + schema drift + lint + build + `go test
  -race`. CD: build image → push to GHCR after a green CI. *Why:* every change goes through
  the same gate; images come out versioned by commit.
- **Minimal image** (`Dockerfile`) — static `CGO_ENABLED=0` binary on distroless static
  nonroot (~21 MB). *Why:* smaller attack surface and fast startup; the schemas are embedded,
  no extra files at runtime.


Everything else (protocol, CRC, serialization, buffer→publish flow, 30s retry) is kept 1:1
with the baseline on purpose, so the performance comparison stays fair.
