# go-data-receiving

Reescritura en Go del microservicio `data-receiving` del sistema de monitoreo de
deslizamientos (Bai et al. 2020). Recibe frames binarios DTU por TCP, los valida y
parsea, y publica envelopes JSON a RabbitMQ con Redis como buffer de durabilidad y
Consul para service discovery.

Baseline de referencia: `data-receiving-module-csharp` (C#/.NET 8).
Guía de ingeniería: [`CLAUDE.md`](./CLAUDE.md). Plan de hitos: [`ROADMAP.md`](./ROADMAP.md).

## Estado

En construcción. Hito actual: **H0 — Bootstrap**. Ver [`ROADMAP.md`](./ROADMAP.md).

## Desarrollo local

Requiere Go 1.23+.

```bash
make build          # compila a ./bin/data-receiving
make run            # ejecuta en modo stdout (sin servicios externos)
make test           # unit + contract tests (sin Docker)
make ci             # vet + lint + test (lo que corre CI)
go run ./cmd/data-receiving version   # imprime la versión
```

## Configuración

Toda la configuración se externaliza vía variables de entorno con prefijo `RECEIVER__`
y `__` como separador de secciones (paridad 1:1 con el baseline C# y el deploy Wave 1).
Los secretos (password de RabbitMQ, token de Consul) **nunca** van en `.env` ni en el
repo: viven en `/etc/data-receiving/secrets` en los servidores. Detalle de claves y
defaults: H2 del [`ROADMAP.md`](./ROADMAP.md) y `deploy/.env.example` (H7).

## Contratos

`contracts/` es la fuente de verdad (JSON Schema Draft 2020-12 + ejemplos + el documento
`tarea-0-contratos.md`). El validador embebe una copia bajo `internal/schema/schemas`
vía `go:embed`; `make sync-schemas` la regenera y `make check-schemas` falla si divergen.
