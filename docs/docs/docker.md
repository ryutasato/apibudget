# Docker

## Quick Start

```bash
# Start API server with Redis
docker compose up -d

# Check health
curl http://localhost:8080/api/v1/health
```

## Configuration

1. Copy the example config:

```bash
cp apibudget.example.yaml apibudget.yaml
```

2. Edit `apibudget.yaml` with your API definitions.

3. Start the services:

```bash
docker compose up -d
```

The server reads `apibudget.yaml` (mounted read-only) for API and credit pool definitions. All other settings are controlled via environment variables in `docker-compose.yaml`.

## Building the Image

```bash
docker build -t apibudget-server .
```

The Dockerfile uses a multi-stage build:

1. **Builder stage** — `golang:1.24-alpine` compiles the binary with `CGO_ENABLED=0`
2. **Runtime stage** — `gcr.io/distroless/static-debian12:nonroot` runs as nonroot user

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `APIBUDGET_CONFIG` | `/etc/apibudget/apibudget.yaml` | Config file path |
| `APIBUDGET_ADDR` | `:8080` | Listen address |
| `APIBUDGET_STORE_TYPE` | `memory` | `memory` or `redis` |
| `APIBUDGET_REDIS_ADDR` | `localhost:6379` | Redis address |
| `APIBUDGET_REDIS_PASSWORD` | (empty) | Redis password |
| `APIBUDGET_REDIS_DB` | `0` | Redis DB number |
| `APIBUDGET_REDIS_TLS` | `false` | Enable TLS |
| `APIBUDGET_LOG_LEVEL` | `info` | Log level |

## docker-compose.yaml

The included `docker-compose.yaml` provides a Redis + API server setup:

```yaml
services:
  apibudget:
    build: .
    ports:
      - "8080:8080"
    environment:
      APIBUDGET_STORE_TYPE: redis
      APIBUDGET_REDIS_ADDR: redis:6379
      APIBUDGET_LOG_LEVEL: info
    volumes:
      - ./apibudget.yaml:/etc/apibudget/apibudget.yaml:ro
    depends_on:
      - redis

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
```

## Running Without Docker

```bash
# Build
go build -o apibudget-server ./cmd/apibudget-server

# Run with in-memory store
APIBUDGET_CONFIG=apibudget.yaml ./apibudget-server

# Run with Redis
APIBUDGET_STORE_TYPE=redis APIBUDGET_REDIS_ADDR=localhost:6379 ./apibudget-server
```
