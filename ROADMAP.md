# ROADMAP — Reescritura de `data-receiving` (C# → Go)

Reescritura en Go del microservicio `data-receiving` del sistema de monitoreo de
deslizamientos (Bai et al. 2020), tomando como baseline la implementación C#/.NET 8
en `data-receiving-module-csharp` y siguiendo las prácticas de ingeniería del
[`CLAUDE.md`](./CLAUDE.md) de este repositorio.

## Objetivo y criterios de diseño

El objetivo del proyecto (CLAUDE.md §0) es **validar empíricamente** que aplicar
buenas prácticas SE mejora el rendimiento (throughput, latencia, CPU, memoria) de la
versión Go frente al baseline C#. De ahí dos criterios que gobiernan todo el roadmap:

- **Fidelidad 1:1 + capa SE medible.** Se replica el comportamiento *observable* del
  C# (protocolo de frame, ACK, serialización byte-igual, flujo Redis→RabbitMQ,
  retry a 30s fijo) para que la comparación sea justa. Las prácticas SE se añaden como
  **capa no intrusiva y medible** (health real, Prometheus, tracing, CDCT). Toda
  desviación funcional respecto al baseline se marca con `// TODO(SE-practice):`.
- **Roadmap completo.** Paridad funcional con el C# **más** los extras que CLAUDE.md
  exige y el C# no tenía (observabilidad, CDCT).

El servicio recibe frames binarios DTU por TCP, los valida (CRC-16/CCITT-FALSE, sync,
versión, tipo, longitud), decodifica un envelope común + payload por tipo de sensor
(`gnss`, `rainfall`, `strain`, `soil_pressure`, `deep_displacement`), valida contra
JSON Schema Draft 2020-12, y según `OutputMode` los envía a un sink
(`stdout`/`file`/`null`/`rabbitmq`). En modo `rabbitmq`: buffer en Redis antes de
publicar con Publisher Confirms, registro en Consul, health check HTTP. Implementa las
Cards **2A1** (parser+TCP), **2A2** (Redis+RabbitMQ), **2A3** (Consul+health).

## Layout del proyecto

```
go-data-receiving/
├── cmd/data-receiving/main.go      # único entrypoint, delgado: signal → app.Run
├── internal/                       # todo el dominio (microservicio, no librería)
│   ├── app/         composition root (errgroup + signal.NotifyContext)
│   ├── config/      Config + sub-structs (env prefix RECEIVER__, separador __)
│   ├── parsing/     crc16, frame, envelope_decoder, serializer, parser + payloads/
│   ├── schema/      validator + //go:embed schemas/*.json
│   ├── tcp/         server, framereader, ack, status
│   ├── sink/        Sink (iface) + stdout/file/null/rabbitmq
│   ├── buffer/      Buffer (iface) + redis + retry_worker
│   ├── messaging/   Publisher (iface) + rabbitmq (publisher confirms)
│   ├── deadletter/  local_dlx
│   ├── discovery/   consul
│   ├── health/      server + checker (verifica TCP + Redis + RabbitMQ)
│   └── observability/ logging (slog JSON) + metrics (Prometheus) + tracing (OTel)
├── contracts/       schemas/ + examples/ + tarea-0-contratos.md  # fuente de verdad
├── test/            integration/ (testcontainers) + contract/ + pact/
└── deploy/          Dockerfile, docker-compose.yml, systemd unit, .env.example, prometheus.yml
```

No se usa `pkg/`: nada de este servicio debe ser importable por otros. Si en el futuro
otro de los 6 servicios necesita compartir código (`crc16`, `SensorEnvelope`, schemas),
lo correcto según el principio contract-first es un módulo `go-shared`/de contratos
aparte, no exponer paquetes desde aquí.

## Hitos

El orden lo dicta la cadena de dependencias: primero lo que no necesita servicios
externos. Cada hito entrega valor verificable de forma aislada.

**Progreso:** H0 ✅ · H1 ✅ · H2 ✅ · H3 ✅ · H4 ✅ · H5 ✅ · H6–H7 pendientes.
Toolchain: Go 1.23 (directiva fijada; `go get` que la subía se revirtió).

### H0 — Bootstrap  ·  _infra_  ·  ✅ hecho (`64ead16`)
- **Entregables:** `go.mod`, layout base, `cmd/data-receiving/main.go` (con `version`),
  `internal/app/app.go` (esqueleto), `Makefile`, `.golangci.yml`, `.gitignore`, CI base
  (tidy + vet + lint + test), copia de `contracts/` desde el repo C#.
- **Verificación:** `go build ./...` y `go vet ./...` verdes; `data-receiving version`
  imprime la versión; lint sin errores.

### H1 — Parsing puro  ·  _Card 2A1 (1/2)_  ·  ✅ hecho (`2af7f60`)
- **Entregables:** `internal/parsing` completo (CRC-16/CCITT-FALSE, FrameValidator,
  EnvelopeDecoder, 5 PayloadDecoders, serializer snake_case con floats a 4 decimales) +
  `internal/schema` (`//go:embed schemas/*.json` + `santhosh-tekuri/jsonschema/v6`).
  Sin I/O de red ni servicios.
- **Verificación:** unit tests port de los `*Tests.cs` (CRC `0x29B1`, FrameValidator,
  los 5 decoders, EnvelopeDecoder con regex/rangos); **golden test de serialización
  byte-igual al C#**; contract test `contracts/examples/*` válidos contra los schemas.
  Todo verde sin Docker.

### H2 — TCP + ACK + DLX + sinks locales  ·  _Card 2A1 (2/2)_  ·  ✅ hecho (`f081ab6`)
- **Entregables:** `internal/tcp` (server, framereader length-prefixed, ACK
  `[0xAA,0x55,status,code,0xAA]`, status), `internal/deadletter` (LocalDlxWriter),
  `internal/sink` (stdout/file/null), `internal/config`, composition root real en
  `internal/app` (errgroup + signal), `cmd/main.go` operativo.
- **Verificación:** unit (framereader parcial/EOF, mapa de ACK, JSON del DLX); **e2e
  local** (port de `EndToEndParserTests.cs`): server en puerto efímero, frame válido →
  envelope en sink + ACK `AA 55 00 00 AA`; CRC malo → ACK status `0x01` + archivo DLX.
  Verde sin Docker.

### H3 — Redis + RabbitMQ + RetryWorker  ·  _Card 2A2_  ·  ✅ hecho (`e0e1067`)
- **Entregables:** `internal/buffer` (`go-redis/v9` fijado a `v9.7.3` por compat con Go
  1.23, key `sensor_buffer:{message_id}`, TTL 24h, wrapper JSON snake_case, RetryWorker
  cada 30s / minAge 2m — **1:1 con baseline**, `// TODO(SE-practice)` para el backoff),
  `internal/messaging` (`amqp091-go` con **publisher confirms**, routing
  `sensors.{project}.{type}`, exchange `sensors.raw`, persistent, timeout 5s, canal
  serializado con mutex; **solo publica, nunca declara topología** — el usuario
  `data_receiving` no tiene permiso `configure`), `internal/sink/rabbitmq.go`
  (Set→Publish→Delete; si el publish falla la entrada queda en Redis y `Write` devuelve
  nil → ACK de éxito, igual que el C#). Wiring condicional por `OutputMode` en
  `internal/app/wiring.go` con teardown ordenado.
- **Verificación:** unit con fakes (borra solo tras confirm; deja en buffer si falla;
  error si falla el buffering; retry republica/conserva; parse del connection string).
  **Integration contra la infra local viva** (tag `integration`, no testcontainers —
  se usa el stack `landslide-infra` que ya corre; env `IT_RABBITMQ_URL` /
  `IT_RABBITMQ_ADMIN_URL` / `RECEIVER__REDIS__CONNECTIONSTRING`, defaults al stack local):
  publicar → mensaje en `data_management.persist` + buffer vacío; RabbitMQ caído →
  entrada persiste → RetryWorker reenvía. **Smoke e2e completo verificado**: frame TCP →
  ACK `AA 55 00 00 AA` → envelope entregado en `data_management.persist` con JSON
  byte-exacto. `go test -tags=integration` verde.
- **Infra:** los `password_hash` de RabbitMQ en `tesis-infra/local/infra/rabbitmq/
  definitions.json` estaban vacíos; se poblaron con hashes SHA256 (commit `53b4287` en
  tesis-infra) de los plaintext de `envs/rabbitmq.env`. Creds activas: `admin` /
  `XLrCTo51s4n50qy8GQ`, `data_receiving` / `changeme_data_receiving`.

### H4 — Consul + Health REAL  ·  _Card 2A3 + mejora SE_  ·  ✅ hecho
- **Hecho:** `internal/discovery` (registro/deregistro, id `data-receiving-{InstanceId}`,
  tags `env`/`version:`/`instance:`, check HTTP `/health/` cada 10s, deregister critical
  1m), `internal/health` (checker + server `net/http`). **Mejora SE aplicada:** el checker
  verifica TCP listening **+ ping real a Redis + ping activo a RabbitMQ** (abre/cierra un
  canal); respuesta `{status, checks:{tcp,redis,rabbitmq}}`, 503 si una dependencia cae.
- **Verificado:** unit (200 sano; 503 con Redis/RabbitMQ/TCP caídos; solo-tcp si pingers
  nil; handler 200/503); integration vs Consul vivo (registro→catálogo con tags correctos
  →deregistro); smoke binario: `/health` → `200 {"status":"ok","checks":{rabbitmq,redis,tcp:true}}`,
  servicio registrado en Consul.
- **Pendiente deploy (no código):** el check HTTP se registra como `http://localhost:{port}`
  (1:1 con el C#). Con Consul **en contenedor**, `localhost` resuelve al contenedor, no al
  host → el check sale `critical`. En Wave 1 (agente Consul en el mismo host) funciona; con
  Consul dockerizado hay que registrar la IP del host o usar host-network. Se resuelve en H7.
- **Entregables:** `internal/discovery` (registro/deregistro en Consul: name
  `data-receiving`, id `data-receiving-{InstanceId}`, tags, check HTTP cada 10s),
  `internal/health`. **Mejora obligatoria (CLAUDE.md §1.6/§4):** el checker verifica TCP
  listening **+ ping real a Redis + ping real a RabbitMQ** (el C# solo mira
  `IsListening`); respuesta `{status, checks:{tcp,redis,rabbitmq}}`, 503 si una
  dependencia crítica cae.
- **Verificación:** unit (503 con Redis/RabbitMQ caídos); integration (port de
  `ConsulIntegrationTests.cs` / `HealthEndpointHttpTests.cs`): registro/deregistro vía
  API, `/health` real, servicio `passing` en Consul.

### H5 — Observabilidad  ·  _EXTRA CLAUDE.md §5_  ·  ✅ hecho
- **Hecho:** `internal/observability` con métricas Prometheus (registry privado),
  logging `slog` JSON centralizado y tracing OTel (span por frame, no-op sin exporter).
  Métricas: `data_receiving_frames_total{result,sensor_type}`, histograma de latencia
  `data_receiving_frame_processing_seconds` (buckets para p50/p90/p95/p99),
  `messages_published_total` / `messages_dropped_total`, más `process_resident_memory_bytes`
  / `process_cpu_seconds_total` (process+Go collectors). `/metrics` servido en el mismo
  puerto del health (`health.Server.WithMetrics`). Instrumentación **no intrusiva** en el
  TCP server vía `WithObservability` (mide latencia, cuenta ok/rejected, abre span);
  pingers/metrics/tracer opcionales (nil-safe), no alteran el flujo medible.
- **Verificado:** unit (counters incrementan; `/metrics` expone frames/latency/published/
  dropped + runtime CPU/mem; ruta `/metrics` servida por `Run()` en puerto real); smoke
  binario: tras 1 frame válido + 1 CRC malo → `frames_total{ok,gnss}=1`,
  `{rejected,unknown}=1`, `published=1`, `dropped=1`, histograma count=2, process CPU/mem
  presentes.
- **Nota:** `/metrics` solo se expone en modo rabbitmq (donde corre el health server). En
  modos locales (stdout/file/null) las métricas se recolectan pero no se exponen — dev.
  El exporter OTLP real (tracing a un colector) se difiere a H7/config.

#### H5 (entregables originales)
- **Entregables:** `internal/observability`: logging `slog` JSON
  (`timestamp,level,service,instance_id,message_id/trace_id,event`); Prometheus
  `/metrics` (`process_resident_memory_bytes`, `process_cpu_seconds_total`, throughput
  `frames_total{result,sensor_type}`, publicados/descartados, **histograma de latencia
  p50/p90/p95/p99**); OpenTelemetry (span por frame, `message_id` como `trace_id`,
  propagación a RabbitMQ). Capa **no intrusiva** — no altera el flujo medible.
- **Verificación:** `/metrics` expone los colectores; procesar un frame incrementa los
  counters y registra latencia; logs JSON parseables; span exportado.

### H6 — CDCT con pact-go  ·  _EXTRA CLAUDE.md §3.3_
- **Entregables:** `test/pact` con message pact AMQP (data-receiving como productor de
  `sensors.{project}.{type}`).
- **Verificación:** `pact-go verify` verde, pact file generado, CI ejecuta contract+pact.

### H7 — Deploy + CI/CD  ·  _infra_
- **Entregables:** `Dockerfile` multi-stage, `docker-compose.yml`
  (redis+rabbitmq+consul+prometheus), `.env.example` y systemd unit portados (mismas
  claves `RECEIVER__*` que el deploy Wave 1), `prometheus.yml`. CI completo:
  build+vet+lint+unit+contract+integration+`-race` + job de drift de schemas.
  **Pendiente de H3:** los integration tests apuntan hoy a la infra local viva; para
  CI hay que levantar el stack (services del runner o testcontainers) y fijar los env
  `IT_RABBITMQ_*` / `RECEIVER__REDIS__CONNECTIONSTRING` contra él.
- **Verificación:** `docker compose up` → servicio `passing` en Consul + `/metrics`;
  pipeline verde; corre con los `.env` existentes sin cambiar claves.

## Prácticas de CLAUDE.md vs baseline C#

Bajo la decisión **1:1 + capa SE medible**, se añaden las prácticas que no alteran el
flujo medible y se conserva el comportamiento del baseline donde sí lo altera
(marcado con `// TODO(SE-practice):`):

| # | Práctica (CLAUDE.md) | Estado en C# | Tratamiento en Go |
|---|---|---|---|
| 1 | Health real (§1.6/§4) | solo `IsListening` | **Añadir** (H4) — no altera flujo |
| 2 | Métricas Prometheus `/metrics` (§2.1/§5) | solo log "summary"/min | **Añadir** (H5) — crítico para el experimento |
| 3 | OpenTelemetry tracing (§5) | ausente | **Añadir** (H5) — no intrusivo |
| 4 | Logging JSON con message_id/trace_id (§1.8) | texto `ILogger` | **Añadir** (H2/H5) con `slog` |
| 5 | CDCT pact-go (§3.3) | solo unit+integration | **Añadir** (H6) |
| 6 | Retry con backoff acotado 1s/5s/30s, máx 3 (§4) | 30s fijo indefinido | **Conservar baseline** + `// TODO(SE-practice):` (afecta flujo medible) |
| 7 | Dedupe del frame crudo del productor (§1.3/§2.1) | ausente | **Conservar baseline** + `// TODO(SE-practice):` (cambia comportamiento observable) |
| 8 | Circuit breaker HTTP saliente (§1.5) | N/A | N/A — data-receiving no llama síncronamente a otros servicios |

## Invariantes a portar 1:1 (verificados en el código C#)

- **Frame DTU:** `SYNC(0xAA 0x55) | VER(1) | TYPE(1) | LEN(2 BE) | PAYLOAD(LEN) | CRC(2 BE) | SYNC_END(0x55 0xAA)`;
  `len == 10 + LEN`, `LEN ∈ [1..1024]`, `VER == 0x01`, `TYPE ∈ [0x01..0x05]`.
  CRC-16/CCITT-FALSE (poly `0x1021`, init `0xFFFF`, sin reflexión, xorout 0), cubre
  bytes `2..len-4`; vector `"123456789" → 0x29B1`. TYPE→tipo: `0x01 gnss, 0x02 rainfall,
  0x03 strain, 0x04 soil_pressure, 0x05 deep_displacement`.
- **Header:** `MSG_ID` 16 bytes → UUID canónico; `PROJ_LEN(u8)|PROJ_ID(ascii)` [3..50]
  regex `^[a-z][a-z0-9_]{2,49}$`; `SENS_LEN(u8)|SENS_ID(ascii)` [1..64]; `TIMESTAMP`
  int64 BE epoch ms → `yyyy-MM-ddTHH:mm:ssZ`; `SAMPLE_INT` uint32 BE [1..86400].
- **Serialización:** snake_case, floats a 4 decimales (incl. nullables), sin
  indentación, omitir `temperature_celsius` cuando es null, `schema_version` fijo `"1.0"`.
- **ACK:** `[0xAA,0x55,status,code,0xAA]`. OK = `0x00 0x00`. Error:
  `Crc→0x01, SensorType→0x04, SchemaValidation→0x03, resto→0x02`; `code = (byte)ParseStage`.
- **Sink rabbitmq:** `buffer.Set` → `publisher.Publish` (confirm) → `buffer.Delete`; si
  el publish falla, la entrada queda en Redis para el RetryWorker.
- **Redis:** key `sensor_buffer:{message_id}` TTL 24h, wrapper
  `{stored_at, routing_key, message_id, envelope_json}`; RetryWorker cada 30s, entradas > 2 min.
- **Publisher:** delivery 2 (persistent), `application/json`, `MessageId` en props,
  confirm con timeout 5s, canal serializado con lock.
- **Consul:** check HTTP `/health/` cada 10s, deregister critical tras 1 min.
- **Config:** prefijo `RECEIVER__`, separador `__`. Defaults: Port=5000, BufferTtl=24h,
  RetryInterval=30s, RetryMinAge=2m, ConfirmTimeout=5s, HealthPort=8080.

## Verificación end-to-end (por fase)

1. **Sin Docker (H1–H2):** `go test ./internal/...` y `go test ./test/contract/...`
   verdes. `go run ./cmd/data-receiving` en modo `stdout`, enviar un frame con el sender
   Python existente y verificar envelope JSON + ACK `AA 55 00 00 AA`. Comparar el JSON
   byte-a-byte contra la salida del binario C# (golden test de paridad).
2. **Con Docker (H3–H4):** `docker compose up` (redis+rabbitmq+consul);
   `go test -tags=integration`. Enviar 100 frames; confirmar entrega en RabbitMQ y buffer
   Redis de vuelta a 0; parar RabbitMQ a mitad y verificar persistencia + reenvío del
   RetryWorker. `curl /health/` → 200 con `checks`; parar Redis → 503. Servicio `passing`
   en Consul.
3. **Observabilidad (H5):** `curl /metrics` muestra `frames_total`, histograma de
   latencia, memoria/CPU; counters incrementan; logs JSON parseables.
4. **CDCT/CI (H6–H7):** `pact-go verify` verde; pipeline CI completo verde;
   `docker compose` levanta el stack y registra el servicio.
