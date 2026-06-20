# Multi-stage build for the data-receiving microservice.
# Schemas are embedded via go:embed, so the runtime image needs only the binary.

# ---- build stage ----
FROM golang:1.23-alpine AS build
WORKDIR /src

# Cache module downloads.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=docker
# Static, CGO-free binary so it runs on a distroless/scratch base.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/data-receiving ./cmd/data-receiving

# ---- runtime stage ----
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/data-receiving /app/data-receiving

# TCP data port (5000), health+metrics port (8080). Override via RECEIVER__*.
EXPOSE 5000 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/data-receiving"]
