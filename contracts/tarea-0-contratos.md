# Tarea 0 — Contratos, topología y nombres

> **Propósito:** este documento es el único artefacto que las dos personas que trabajen en Wave 1 necesitan referenciar mutuamente. Define el contrato de datos que viajará entre microservicios, la topología de RabbitMQ y la convención de nombres para Consul. Léelo antes de empezar tu track.

> **Alcance:** sistema de monitoreo y alerta temprana de deslizamientos basado en arquitectura de microservicios, según Bai et al. (2020). Replica fiel del paper, sin extensiones.

> **Estado:** versión `1.0` del contrato. Mantener congelada hasta que Wave 2 esté en producción.

---

## Tabla de contenido

1. [Contexto y decisiones de diseño](#1-contexto-y-decisiones-de-diseño)
2. [Parte 1 — Esquema del paquete parseado](#parte-1--esquema-del-paquete-parseado)
3. [Parte 2 — Topología de RabbitMQ](#parte-2--topología-de-rabbitmq)
4. [Parte 3 — Convención de nombres en Consul](#parte-3--convención-de-nombres-en-consul)
5. [Versionado del contrato](#versionado-del-contrato)
6. [Anexos](#anexos)

---

## 1. Contexto y decisiones de diseño

El paper describe seis microservicios (recepción, gestión de datos, análisis, alerta temprana, gestión de usuarios, gestión de proyectos) que se comunican principalmente mediante:

- **Asíncrono:** RabbitMQ como broker pub/sub para datos de sensores en tiempo real
- **Síncrono:** REST/HTTP entre servicios cuando hay dependencia directa (ej. Early Warning llamando a Data Analysis)
- **Indirecto:** base de datos MySQL distribuida compartida entre servicios

Tres restricciones del paper guían las decisiones que siguen:

1. El servicio de recepción "parsea el paquete y lo convierte a un formato estándar" antes de publicar. Lo que viaja por la cola ya está limpio y normalizado.
2. Múltiples consumidores se suscriben al mismo flujo (Data Management para persistir, Early Warning para evaluar umbrales).
3. Los proyectos tienen umbrales de alerta distintos (Tablas 4 y 5 del paper). El `project_id` debe viajar con cada lectura.

Tipos de sensores cubiertos (de la Tabla 1 del paper):

| Tipo | Identificador | Justificación |
|---|---|---|
| GNSS / desplazamiento superficial | `gnss` | Sensor principal, mayor volumen de datos |
| Pluviómetro | `rainfall` | Factor inductor crítico de deslizamientos |
| Extensómetro / strain | `strain` | Monitoreo de pilotes anti-deslizamiento |
| Presión de suelo | `soil_pressure` | Esfuerzos en estructuras de contención |
| Inclinómetro / desplazamiento profundo | `deep_displacement` | Monitoreo en pozos profundos |

Las cámaras de video (mencionadas en el paper) se manejan fuera de este pipeline, en almacenamiento de objetos con metadatos en BD.

---

## Parte 1 — Esquema del paquete parseado

### 1.1 Estructura general

Cada mensaje publicado por Data Receiving en RabbitMQ tiene dos bloques: un **envelope** con metadatos comunes a todo tipo de sensor, y un **payload** con campos específicos por tipo.

```json
{
  "schema_version": "1.0",
  "message_id": "550e8400-e29b-41d4-a716-446655440000",
  "project_id": "shangtan",
  "sensor_id": "GXZM-DB03",
  "sensor_type": "gnss",
  "timestamp": "2026-05-02T14:30:00Z",
  "received_at": "2026-05-02T14:30:12Z",
  "sampling_interval_seconds": 3600,
  "payload": {
    "displacement_x_mm": 12.34,
    "displacement_y_mm": -5.67,
    "displacement_z_mm": 2.10,
    "displacement_horizontal_mm": 13.57,
    "cumulative_displacement_mm": 1247.5,
    "fix_quality": "rtk_fixed"
  }
}
```

### 1.2 Campos del envelope

| Campo | Tipo | Obligatorio | Descripción |
|---|---|---|---|
| `schema_version` | string (semver) | Sí | Versión del contrato. Actualmente `1.0`. |
| `message_id` | string (UUID v4) | Sí | Identificador único del mensaje. Se usa para deduplicación si Data Receiving reintenta. |
| `project_id` | string (snake_case) | Sí | Identificador del proyecto de monitoreo. Ejemplos: `shangtan`, `lingzhan`. |
| `sensor_id` | string | Sí | Identificador del sensor. Sigue la nomenclatura del paper (`GXZM-DB01`, `LZ-01`, etc.). |
| `sensor_type` | enum string | Sí | Discriminador del payload. Valores: `gnss`, `rainfall`, `strain`, `soil_pressure`, `deep_displacement`. |
| `timestamp` | string (ISO 8601, UTC) | Sí | Cuándo midió el sensor. Crítico para series de tiempo. |
| `received_at` | string (ISO 8601, UTC) | Sí | Cuándo Data Receiving recibió el paquete crudo. Permite diagnosticar latencia. |
| `sampling_interval_seconds` | integer | Sí | Intervalo de muestreo en el momento de la lectura. `3600` por defecto, menor en condiciones aceleradas. |
| `payload` | object | Sí | Datos específicos del tipo de sensor. Ver sección 1.3. |

### 1.3 Payloads por tipo de sensor

#### 1.3.1 GNSS (`sensor_type: "gnss"`)

```json
{
  "displacement_x_mm": 12.34,
  "displacement_y_mm": -5.67,
  "displacement_z_mm": 2.10,
  "displacement_horizontal_mm": 13.57,
  "cumulative_displacement_mm": 1247.5,
  "fix_quality": "rtk_fixed"
}
```

| Campo | Tipo | Obligatorio | Descripción |
|---|---|---|---|
| `displacement_x_mm` | number | Sí | Componente este-oeste del desplazamiento (mm). |
| `displacement_y_mm` | number | Sí | Componente norte-sur del desplazamiento (mm). |
| `displacement_z_mm` | number | Sí | Componente vertical del desplazamiento (mm). |
| `displacement_horizontal_mm` | number | Sí | Magnitud horizontal: √(x² + y²). Redundante pero útil aguas abajo. |
| `cumulative_displacement_mm` | number | Sí | Desplazamiento acumulado desde la línea base. Se grafica en Figura 8a del paper. |
| `fix_quality` | enum string | Sí | Calidad de la solución GNSS: `rtk_fixed`, `rtk_float`, `dgps`, `single`, `no_fix`. Solo `rtk_fixed` es confiable a nivel mm. |

#### 1.3.2 Lluvia (`sensor_type: "rainfall"`)

```json
{
  "rainfall_mm": 2.4,
  "cumulative_rainfall_mm": 1547.2,
  "interval_minutes": 60
}
```

| Campo | Tipo | Obligatorio | Descripción |
|---|---|---|---|
| `rainfall_mm` | number | Sí | Lluvia caída en el intervalo (mm). |
| `cumulative_rainfall_mm` | number | Sí | Acumulado desde inicio de la temporada. Figura 8b del paper. |
| `interval_minutes` | integer | Sí | Duración del intervalo de medición. Típicamente 60. |

#### 1.3.3 Strain (`sensor_type: "strain"`)

```json
{
  "strain_microstrain": 234.5,
  "temperature_celsius": 24.3
}
```

| Campo | Tipo | Obligatorio | Descripción |
|---|---|---|---|
| `strain_microstrain` | number | Sí | Deformación unitaria en micro-strain (µε). |
| `temperature_celsius` | number | No | Temperatura del sensor para compensación. Recomendado pero opcional. |

#### 1.3.4 Presión de suelo (`sensor_type: "soil_pressure"`)

```json
{
  "pressure_kpa": 45.2,
  "temperature_celsius": 18.7
}
```

| Campo | Tipo | Obligatorio | Descripción |
|---|---|---|---|
| `pressure_kpa` | number | Sí | Presión en kilopascales. |
| `temperature_celsius` | number | No | Temperatura del sensor. |

#### 1.3.5 Desplazamiento profundo (`sensor_type: "deep_displacement"`)

Los inclinómetros miden a múltiples profundidades a lo largo de un tubo. **Cada lectura a una profundidad puntual viaja como un mensaje independiente** con el mismo `sensor_id` y `timestamp` pero distinto `depth_m`. Esto simplifica el esquema a costa de más mensajes por ciclo de muestreo.

```json
{
  "depth_m": 8.0,
  "tilt_x_mm": 12.3,
  "tilt_y_mm": -3.4,
  "cumulative_tilt_mm": 89.1
}
```

| Campo | Tipo | Obligatorio | Descripción |
|---|---|---|---|
| `depth_m` | number | Sí | Profundidad de la lectura, en metros. |
| `tilt_x_mm` | number | Sí | Inclinación en mm en el eje X. |
| `tilt_y_mm` | number | Sí | Inclinación en mm en el eje Y. |
| `cumulative_tilt_mm` | number | Sí | Inclinación acumulada desde línea base. Figura 4 del paper grafica esto vs. profundidad. |

### 1.4 Validación

Cada payload se valida contra un JSON Schema antes de publicar a RabbitMQ. Los schemas viven en `/schemas/`:

- `envelope.schema.json`
- `payload-gnss.schema.json`
- `payload-rainfall.schema.json`
- `payload-strain.schema.json`
- `payload-soil-pressure.schema.json`
- `payload-deep-displacement.schema.json`

Si un mensaje falla validación, va al Dead Letter Exchange (sección 2.4) y **no** se publica al exchange principal.

### 1.5 Ejemplo completo de cada tipo

Ver el archivo `examples/` en este mismo paquete para un ejemplo válido por cada tipo de sensor.

---

## Parte 2 — Topología de RabbitMQ

### 2.1 Decisiones de diseño

**Tipo de exchange: `topic`.** Permite que cada consumidor filtre por patrón sin que el productor sepa de los consumidores. Esto es lo que hace que agregar un nuevo servicio (digamos, un servicio futuro de exportación a un sistema externo) no requiera tocar a Data Receiving.

**Routing key: `sensors.<project_id>.<sensor_type>`.** Tres niveles, separados por punto. Permite filtros tipo:

- `sensors.#` — todo
- `sensors.shangtan.#` — todo lo de Shangtan
- `sensors.*.rainfall` — toda la lluvia de cualquier proyecto
- `sensors.lingzhan.gnss` — GNSS solo de Lingzhan

**Una cola por consumidor, no por tipo de sensor.** Cada servicio consumidor tiene su propia cola que bindea al exchange con su patrón. Si dos servicios compartieran cola los mensajes se repartirían entre ellos (competing consumers) en vez de duplicarse.

### 2.2 Exchanges

| Nombre | Tipo | Durable | Propósito |
|---|---|---|---|
| `sensors.raw` | topic | sí | Recibe mensajes parseados de Data Receiving y los enruta a consumidores. |
| `sensors.dlx` | topic | sí | Dead Letter Exchange. Mensajes envenenados o que excedieron reintentos llegan aquí. |

### 2.3 Colas

| Cola | Binding al exchange | Bindea con patrón | Consumidor |
|---|---|---|---|
| `data_management.persist` | `sensors.raw` | `sensors.#` | Data Management (persistencia en MySQL) |
| `early_warning.evaluate` | `sensors.raw` | `sensors.#` | Early Warning (evaluación de umbrales) |
| `sensors.dead_letter` | `sensors.dlx` | `#` | Job de inspección manual / alerta a operadores |

Configuración estándar para todas las colas operativas:

```
durable: true
auto-delete: false
arguments:
  x-dead-letter-exchange: sensors.dlx
  x-dead-letter-routing-key: <misma routing key del mensaje>
  x-message-ttl: 86400000   # 24 horas en milisegundos
  x-max-length: 1000000     # protección contra acumulación descontrolada
```

Los mensajes se publican con `persistent: true` (delivery_mode 2) para sobrevivir reinicios del broker.

### 2.4 Manejo de errores y reintentos

- **Reintentos automáticos del consumidor:** 3 intentos con backoff exponencial (1s, 5s, 30s).
- **Después del 3er fallo:** el mensaje va a `sensors.dead_letter`.
- **DLQ inspection:** un servicio o job revisa periódicamente la DLQ. Si el patrón de fallos sugiere bug en código, se corrige y se re-publican los mensajes.
- **Nunca se descartan silenciosamente.** Todo mensaje termina o procesado o en DLQ.

### 2.5 Cluster y alta disponibilidad

- Mínimo 2 nodos en cluster RabbitMQ
- Política de mirror para todas las colas operativas: `ha-mode: all` (replicación a todos los nodos)
- DLQ puede ser no-mirror para reducir overhead

### 2.6 Usuarios y permisos

Un usuario por servicio, con permisos mínimos:

| Usuario | Permiso configure | Permiso write | Permiso read |
|---|---|---|---|
| `data_receiving` | — | `sensors.raw` | — |
| `data_management` | `data_management.*` | — | `data_management.*` |
| `early_warning` | `early_warning.*` | — | `early_warning.*` |
| `dlq_inspector` | — | — | `sensors.dead_letter` |

### 2.7 Integración con Redis

El paper indica que Redis se usa "para prevenir pérdida de datos por congestión de la cola o interrupción de red". Implementación propuesta:

1. Data Receiving escribe cada mensaje a Redis con TTL de 24h **antes** de publicar a RabbitMQ.
2. Espera el ACK de publicación.
3. Si el ACK llega, borra la entrada de Redis.
4. Si no llega (broker caído o timeout), el mensaje queda en Redis. Un job de reintento lo republica cuando el broker vuelva.

Convención de claves: `sensor_buffer:{message_id}` con valor el JSON del mensaje completo.

### 2.8 Diagrama de la topología

```
                    ┌─────────────────────┐
                    │  Data Receiving     │
                    │  (3 instancias)     │
                    └──────────┬──────────┘
                               │
                               │ publish (persistent)
                               │ routing key: sensors.<proj>.<type>
                               ▼
                    ┌─────────────────────┐
                    │   sensors.raw       │
                    │   (topic exchange)  │
                    └─────┬───────────┬───┘
                          │           │
                  binding │           │ binding
                  sensors.#           sensors.#
                          │           │
                          ▼           ▼
            ┌─────────────────┐  ┌────────────────────┐
            │ data_management │  │ early_warning      │
            │ .persist        │  │ .evaluate          │
            └────────┬────────┘  └──────────┬─────────┘
                     │                       │
              consume│                       │ consume
                     ▼                       ▼
            ┌─────────────────┐  ┌────────────────────┐
            │ Data Management │  │ Early Warning      │
            │ (2 instancias)  │  │ (2 instancias)     │
            └─────────────────┘  └────────────────────┘

            (al fallar 3 veces, los mensajes van a sensors.dlx → sensors.dead_letter)
```

---

## Parte 3 — Convención de nombres en Consul

### 3.1 Nombres de servicios

Snake-case con guiones, descriptivos del rol no del lenguaje:

| Service name | Instancias (paper) | Rol |
|---|---|---|
| `data-receiving` | 3 | Recepción y parseo de paquetes de sensores |
| `data-management` | 2 | CRUD sobre datos de monitoreo |
| `data-analysis` | 2 | Algoritmos: outliers, denoising, regresión, predicción |
| `early-warning` | 2 | Suscripción a cola, evaluación de umbrales, push de alertas |
| `user-management` | 1 | Registro, autenticación, autorización |
| `project-management` | 1 | Gestión de proyectos, sensores, archivos |

Cada instancia se registra con un `instance_id` único: `<service-name>-<n>` (ej. `data-receiving-01`, `data-receiving-02`, `data-receiving-03`).

### 3.2 Health checks

Cada servicio expone un endpoint HTTP `/health` que retorna:

- `200 OK` si la conexión a su BD y a RabbitMQ están saludables
- `503 Service Unavailable` en cualquier otro caso

Configuración del check en Consul:

```hcl
check {
  id       = "<instance_id>-health"
  name     = "HTTP health check"
  http     = "http://<ip>:<port>/health"
  method   = "GET"
  interval = "10s"
  timeout  = "2s"
  deregister_critical_service_after = "1m"
}
```

Significado:
- **interval 10s:** detecta caídas en máx 30s (3 fallos consecutivos antes de marcar critical)
- **timeout 2s:** servicios lentos se consideran no saludables
- **deregister_critical_service_after 1m:** instancias persistentemente caídas salen del catálogo

### 3.3 Tags

Cuando un servicio se registra, declara tags que ayudan a filtrarlo:

| Tag | Valores ejemplo | Uso |
|---|---|---|
| `version` | `version-1.0.0`, `version-1.1.0` | Deploy canary, rollback |
| `lang` | `lang-csharp`, `lang-java`, `lang-python` | Debugging, métricas por stack |
| `env` | `env-dev`, `env-staging`, `env-prod` | Aislamiento de ambientes |

Nginx via `consul-template` puede generar upstreams filtrando por tags (ej. solo `env-prod`).

### 3.4 Ejemplo de registro (HTTP API de Consul)

```json
{
  "ID": "data-receiving-01",
  "Name": "data-receiving",
  "Tags": ["version-1.0.0", "lang-csharp", "env-prod"],
  "Address": "10.0.1.23",
  "Port": 8080,
  "Check": {
    "HTTP": "http://10.0.1.23:8080/health",
    "Method": "GET",
    "Interval": "10s",
    "Timeout": "2s",
    "DeregisterCriticalServiceAfter": "1m"
  }
}
```

---

## Versionado del contrato

Regla simple: **semver en el campo `schema_version` del envelope**.

- **Cambio compatible** (agregar campo opcional, agregar valor a un enum): bump de minor (`1.0` → `1.1`). Consumidores viejos lo ignoran y siguen funcionando.
- **Cambio incompatible** (renombrar campo, cambiar tipo, eliminar campo, cambiar semántica): bump de major (`1.0` → `2.0`). Hay que migrar consumidores antes de cambiar productores.

**Mientras dure Wave 1 y Wave 2:** mantener `1.0` congelado. Cualquier cambio se acumula en una rama del repositorio de contratos para release coordinado.

**Compatibilidad mínima:** los servicios deben aceptar al menos las dos últimas versiones major. Si estamos en `2.x`, todos los servicios siguen aceptando `1.x` durante un período de gracia (mínimo 3 meses) para permitir despliegues escalonados.

---

## Anexos

### A. Repositorio del contrato

Estructura recomendada del repo:

```
contracts/
├── tarea-0-contratos.md                  # este documento
├── README.md                              # apunta a este documento
├── schemas/                               # JSON Schemas
│   ├── envelope.schema.json
│   ├── payload-gnss.schema.json
│   ├── payload-rainfall.schema.json
│   ├── payload-strain.schema.json
│   ├── payload-soil-pressure.schema.json
│   └── payload-deep-displacement.schema.json
├── examples/                              # mensajes válidos de ejemplo
│   ├── gnss-shangtan.json
│   ├── rainfall-shangtan.json
│   ├── strain-shangtan.json
│   ├── soil-pressure-shangtan.json
│   └── deep-displacement-lingzhan.json
└── CHANGELOG.md                           # historial de versiones del contrato
```

Publicar paquetes generados por lenguaje:
- C# / NuGet: `LandslideMonitoring.Contracts`
- Java / Maven: `com.landslide.monitoring:contracts`
- Python / PyPI interno: `landslide-monitoring-contracts`

### B. Identificadores de proyecto y sensor (referencia del paper)

**Proyecto Shangtan** (Hezhou, Guangxi):
- `project_id`: `shangtan`
- Sensores GNSS: `GXZM-DB01` a `GXZM-DB08`
- Pluviómetro: `GXZM-RG01`
- Cámaras: `GXZM-CAM01` a `GXZM-CAM03`

**Proyecto Lingzhan** (Lingyun, Guangxi):
- `project_id`: `lingzhan`
- Sensores GNSS: `LZ-01`, `LZ-02`, `LZ-03` (y base station)

**Convención para nuevos sitios:** `project_id` en snake_case con el nombre del lugar; `sensor_id` con prefijo del proyecto seguido de tipo y número (ej. `<PROYECTO>-<TIPO><NN>`).

### C. Referencia rápida de routing keys

| Routing key | Descripción |
|---|---|
| `sensors.shangtan.gnss` | Lectura GNSS del proyecto Shangtan |
| `sensors.shangtan.rainfall` | Lluvia en Shangtan |
| `sensors.shangtan.strain` | Strain en Shangtan |
| `sensors.shangtan.soil_pressure` | Presión de suelo en Shangtan |
| `sensors.shangtan.deep_displacement` | Inclinómetro en Shangtan |
| `sensors.lingzhan.gnss` | GNSS en Lingzhan |
| `sensors.lingzhan.rainfall` | Lluvia en Lingzhan |
| ... | ... |

### D. Checklist de validación al final de Tarea 0

Antes de pasar a tracks paralelos, verificar:

- [ ] Las dos personas leyeron y entienden este documento
- [ ] El repositorio `contracts/` existe con la estructura del Anexo A
- [ ] Los 6 JSON Schemas están en `schemas/` y validan
- [ ] Hay al menos un ejemplo válido por cada tipo de sensor en `examples/`
- [ ] CHANGELOG.md inicializado con la entrada de versión `1.0`
- [ ] El equipo acordó qué herramienta usará para publicar los paquetes por lenguaje (NuGet, Maven, PyPI)

---

**Versión del documento:** 1.0
**Fecha:** mayo 2026
**Basado en:** Bai et al. (2020), *The design and application of landslide monitoring and early warning system based on microservice architecture*, Geomatics, Natural Hazards and Risk, 11(1), 928–948.
