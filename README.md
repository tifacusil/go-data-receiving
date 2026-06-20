# go-data-receiving

Reescritura en Go del microservicio `data-receiving` del sistema de monitoreo de
deslizamientos (Bai et al. 2020). Recibe frames binarios DTU por TCP, los valida y
parsea, y publica envelopes JSON a RabbitMQ con Redis como buffer de durabilidad y
Consul para service discovery.

Baseline de referencia: `data-receiving-module-csharp` (C#/.NET 8).
Guía de ingeniería: [`CLAUDE.md`](./CLAUDE.md). Plan de hitos: [`ROADMAP.md`](./ROADMAP.md).

## Estado

Reescritura completa (H0–H7). El servicio replica el comportamiento *observable* del
baseline C# (protocolo de frame, CRC, serialización byte-igual, ACK, flujo
Redis→RabbitMQ) para que la comparación de rendimiento Go vs C# sea justa, y añade como
capa no intrusiva las prácticas SE que el baseline no tenía (health real, métricas,
tracing, CDCT). Ver [Prácticas de ingeniería](#prácticas-de-ingeniería-aplicadas).

## Arquitectura

```
DTU sensor (TCP)
   │
   ▼  internal/tcp        framereader length-prefixed, ACK, accept loop
   ▼  internal/parsing    CRC-16 → frame → header → payload → serializer
   ▼  internal/schema     validación JSON Schema 2020-12 (embebido)
   ▼  internal/sink        ┌─ stdout / file / null      (modos locales)
                           └─ rabbitmq:
                                 internal/buffer     Set en Redis  (sensor_buffer:{id})
                                 internal/messaging  Publish con confirms → sensors.raw
                                 internal/buffer     Delete tras confirmar (+ RetryWorker)
   ▼  internal/deadletter  frames inválidos → JSON en disco (nunca bloquean)

Side workers (modo rabbitmq):
   internal/health     /health (TCP+Redis+RabbitMQ) y /metrics
   internal/discovery  registro/deregistro en Consul
```

`internal/app` es el composition root que cablea todo; `cmd/data-receiving/main.go` solo
maneja señales y delega.

## Desarrollo local

Requiere Go 1.23+.

```bash
make build          # compila a ./bin/data-receiving
make run            # ejecuta en modo stdout (sin servicios externos)
make test           # unit + contract tests (sin Docker, -race)
make lint           # golangci-lint v2
make test-integration   # integration vs infra viva (redis/rabbitmq/consul)
make test-pact          # CDCT message pact (requiere lib FFI de pact)
go run ./cmd/data-receiving version   # imprime la versión (sha en imagen)
```

## Configuración

Toda la configuración se externaliza vía variables de entorno con prefijo `RECEIVER__`
y `__` como separador de secciones (paridad 1:1 con el baseline C#). Los secretos
(password de RabbitMQ, token de Consul) **nunca** van en `.env` ni en el repo: viven en
`/etc/data-receiving/secrets` en los servidores. Claves y defaults: `internal/config/config.go`
y `.env.example`.

`OutputMode` (`stdout`|`file`|`null`|`rabbitmq`) decide qué cablea el composition root.
Los modos locales no construyen Redis/RabbitMQ/Consul/health — útiles para dev y tests
sin infra.

## Contratos

`contracts/` es la fuente de verdad (JSON Schema Draft 2020-12 + ejemplos + el documento
`tarea-0-contratos.md`). El validador embebe una copia bajo `internal/schema/schemas`
vía `go:embed`; `make sync-schemas` la regenera y `make check-schemas` falla (en CI) si
divergen.

## Despliegue

`Dockerfile` (multi-stage, binario estático en imagen distroless) en este repo. El
`docker-compose.yml` de despliegue y el `prometheus.yml` viven en el repo **tesis-infra**.
CI/CD en `.github/workflows/`: CI (lint + build + test) en cada push/PR; CD publica la
imagen a GHCR tras un CI verde.

---

## Prácticas de ingeniería aplicadas

Las prácticas siguen el catálogo del [`CLAUDE.md`](./CLAUDE.md) (sustentado en El Fellah
et al. 2026, Schwarz et al. 2025, Saleh Sedghpour et al. 2022, Taibi & Lenarduzzi 2018).
Cada una se lista con **dónde** está en el código y **por qué** se aplicó.

### Diseño de microservicios

- **Contract-first.** `contracts/schemas/*.schema.json` (JSON Schema 2020-12) definen el
  envelope y los 5 payloads *antes* que cualquier handler. El productor valida contra
  ellos en `internal/schema/validator.go` antes de publicar. *Por qué:* un mensaje que no
  cumple el contrato nunca llega a `sensors.raw`; los consumidores (Data Management, Early
  Warning) dependen de una forma estable.

- **Database/buffer-per-service.** El servicio solo toca su propio Redis con el prefijo
  `sensor_buffer:` (`internal/buffer/redis.go`). No lee tablas de otros servicios. *Por
  qué:* evita el bad smell de *Shared Persistency* (Taibi & Lenarduzzi 2018); cada
  servicio dueño de sus datos.

- **Idempotencia / clave de deduplicación.** `message_id` (UUID) viaja en el envelope, es
  la key del buffer Redis y la `MessageId` de la propiedad AMQP
  (`internal/messaging/publisher.go`). *Por qué:* permite a los consumidores deduplicar
  bajo entrega *at-least-once*; nunca se asume *exactly-once*.

- **Versionado de contrato.** `schema_version` fijo `"1.0"` en el envelope, patrón semver
  en el schema. *Por qué:* cambios futuros siguen la regla aditivo→minor / incompatible→major.

### Resiliencia y durabilidad

- **Outbox / buffer de durabilidad (Set→Publish→Delete).** `internal/sink/rabbitmq.go`
  escribe a Redis *antes* de publicar y borra solo tras la confirmación del broker; si el
  publish falla, la entrada queda en Redis y `Write` devuelve `nil` (ACK de éxito al
  sensor). *Por qué:* ningún frame parseado se pierde aunque RabbitMQ esté caído.

- **Publisher Confirms.** `internal/messaging/publisher.go` espera el ack del broker con
  timeout antes de considerar publicado un mensaje. *Por qué:* sin confirms, "publicar" no
  garantiza que el broker lo aceptó.

- **Retry acotado + Dead Letter local.** `internal/buffer/retry_worker.go` reescanea Redis
  y republica las entradas atascadas; los frames inválidos van a un dead-letter en disco
  (`internal/deadletter/local.go`) y nunca bloquean el pipeline. *Por qué:* recuperación
  automática tras caídas + inspección de frames rechazados.
  *(El intervalo de retry es fijo 30s, 1:1 con el baseline; el backoff exponencial queda
  marcado con `// TODO(SE-practice):` para no romper la comparabilidad del experimento.)*

- **Health checks reales.** `internal/health/checker.go` verifica que el listener TCP esté
  arriba **y** hace ping real a Redis y a RabbitMQ; responde `503` si una dependencia
  crítica cae. *Por qué:* un `/health` que solo devuelve `200` estático miente; Consul debe
  sacar de rotación una instancia con dependencias rotas.

- **Service discovery con auto-deregistro.** `internal/discovery/consul.go` registra el
  servicio con un health check HTTP y lo deregistra en el shutdown (`Run` bloquea en `ctx`
  y limpia en el defer). *Por qué:* el balanceador solo enruta a instancias vivas; el
  graceful shutdown evita 0 frame-loss al rotar instancias.

### 12-factor / operación

- **Configuración externalizada.** `internal/config/config.go` mapea todo desde env
  (`RECEIVER__*`); cero hosts/puertos/credenciales hardcodeados. Secretos fuera del repo.
  *Por qué:* mismo binario en dev/staging/prod cambiando solo el entorno.

- **Logging estructurado (JSON) con correlación.** `internal/observability/logging.go`
  (slog JSON) añade `service`/`instance_id`/`version` a cada línea; los logs por frame
  llevan `message_id`. *Por qué:* seguir un mensaje a través de TCP → RabbitMQ →
  consumidores.

- **Métricas Prometheus.** `internal/observability/metrics.go`: `frames_total{result,
  sensor_type}`, histograma de latencia (p50/p90/p95/p99), publicados/descartados, CPU y
  memoria residente del proceso; servidas en `/metrics`. *Por qué:* el objetivo del
  proyecto es comparar empíricamente throughput/latencia/CPU/memoria Go vs C#.

- **Distributed tracing.** `internal/observability/tracing.go` abre un span por frame
  correlado por `message_id` (no-op sin exporter configurado). *Por qué:* trazar el flujo
  end-to-end cuando haya colector.

- **Observabilidad no intrusiva.** La instrumentación se inyecta vía
  `tcp.Server.WithObservability` y es nil-safe. *Por qué:* medir sin alterar el flujo
  byte-a-byte que la comparación de rendimiento necesita intacto.

### Arquitectura del código

- **Composition root + inyección por constructor (sin framework DI).** `internal/app/`
  arma las dependencias y las corre como goroutines bajo un `errgroup` con `context`
  cancelable; `cmd/data-receiving/main.go` solo monta `signal.NotifyContext` y delega.
  *Por qué:* equivalente idiomático Go del host con `BackgroundService` de .NET, sin magia.

- **Graceful shutdown.** SIGINT/SIGTERM cancelan el `context` → el `errgroup` propaga → cada
  worker cierra en su `defer` (incl. deregistro de Consul). *Por qué:* terminar limpio sin
  perder mensajes en vuelo.

- **Puertos y adaptadores (interfaces pequeñas, definidas del lado del consumidor).**
  `sink.Sink`, `buffer.Buffer`, `messaging.Publisher`, `parsing.SchemaValidator`,
  `tcp.Observer`/`tcp.Tracer`. *Por qué:* desacopla el dominio de la infraestructura y hace
  triviales los fakes en tests.

- **`internal/` para encapsular.** Todo el dominio vive en `internal/` (el compilador
  prohíbe importarlo desde fuera del módulo); no se usa `pkg/`. *Por qué:* es un
  microservicio, no una librería; nada debe ser importado por terceros.

- **Núcleo puro sin I/O.** El parser (`internal/parsing`) opera sobre `[]byte` sin sockets
  ni servicios. *Por qué:* testeable con arrays de bytes hardcodeados, rápido, determinista.

### Calidad y CI/CD (pirámide de tests)

- **Unit tests** — lógica aislada con mocks/fakes (CRC, decoders, ACK, sink con buffer
  falso, etc.).
- **Golden test de serialización** (`internal/parsing/serializer_test.go`) — fija los bytes
  JSON exactos, byte-igual al C# (incluido el drop de ceros: `15.0`→`15`, `2.10`→`2.1`).
  *Por qué:* la paridad de salida es el contrato medible del experimento.
- **Contract tests** (`internal/schema/validator_test.go`) — los ejemplos de `contracts/`
  validan contra los schemas embebidos.
- **Integration tests** (`test/integration/`, build tag `integration`) — Redis + RabbitMQ +
  Consul reales: publish→cola + buffer vacío, broker caído→persiste→RetryWorker reenvía,
  registro/deregistro en Consul.
- **CDCT con pact-go** (`test/pact/`, build tag `pact`) — message pact AMQP: el consumidor
  `data-management` define lo que espera y el handler valida el body contra *nuestro*
  schema. *Por qué:* detecta divergencias productor-consumidor sin levantar todo el sistema
  (Schwarz et al. 2025).
- **CI/CD** (`.github/workflows/`) — CI: tidy + drift de schemas + lint + build + `go test
  -race`. CD: build de imagen → push a GHCR tras CI verde. *Por qué:* cada cambio pasa por
  el mismo gate; las imágenes salen versionadas por commit.
- **Imagen mínima** (`Dockerfile`) — binario estático `CGO_ENABLED=0` sobre distroless
  static nonroot (~21 MB). *Por qué:* menor superficie de ataque y arranque rápido; los
  schemas van embebidos, sin archivos extra en runtime.

### Resumen: qué añade esta versión sobre el baseline C#

| Práctica | Baseline C# | Esta versión |
|---|---|---|
| Health check | solo "TCP listening" | TCP + ping real Redis + RabbitMQ |
| Métricas Prometheus | ausente | `/metrics` con throughput/latencia/CPU/mem |
| Tracing OpenTelemetry | ausente | span por frame correlado por `message_id` |
| Logging | texto | JSON estructurado con `message_id`/`instance_id` |
| CDCT (Pact) | solo unit+integration | message pact AMQP productor↔consumidor |

Lo demás (protocolo, CRC, serialización, flujo buffer→publish, retry 30s) se mantiene 1:1
con el baseline a propósito, para que la comparación de rendimiento sea justa.
