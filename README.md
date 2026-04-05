<p align="center">
  <h1 align="center">apibudget</h1>
  <p align="center">
    Unified rate limiting & credit budget management for external APIs in Go.
    <br />
    <a href="https://ryutasato.github.io/apibudget/">Docs</a> · <a href="https://github.com/ryutasato/apibudget/issues">Issues</a> · <a href="https://github.com/ryutasato/apibudget/releases">Releases</a>
  </p>
</p>

<p align="center">
  <a href="https://github.com/ryutasato/apibudget/actions/workflows/ci.yaml"><img src="https://github.com/ryutasato/apibudget/actions/workflows/ci.yaml/badge.svg" alt="CI"></a>
  <a href="https://codecov.io/gh/ryutasato/apibudget"><img src="https://codecov.io/gh/ryutasato/apibudget/branch/main/graph/badge.svg" alt="Coverage"></a>
  <a href="https://pkg.go.dev/github.com/ryutasato/apibudget"><img src="https://pkg.go.dev/badge/github.com/ryutasato/apibudget.svg" alt="Go Reference"></a>
  <a href="https://goreportcard.com/report/github.com/ryutasato/apibudget"><img src="https://goreportcard.com/badge/github.com/ryutasato/apibudget" alt="Go Report Card"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/ryutasato/apibudget" alt="License"></a>
  <a href="https://github.com/ryutasato/apibudget/stargazers"><img src="https://img.shields.io/github/stars/ryutasato/apibudget?style=social" alt="Stars"></a>
</p>

---


## Why apibudget?

Calling external APIs (OpenAI, Stripe, Anthropic, etc.) without proper rate limiting leads to `429 Too Many Requests`, unexpected bills, and wasted tokens. apibudget gives you a single, local control plane for all of it:

- **Multi-window rate limiting** — per-second, per-minute, per-day, all at once
- **Credit/token budgets** — track spend with `*big.Rat` precision, no floating-point drift
- **Allow / Wait / Reserve** — the same patterns you know from `golang.org/x/time/rate`
- **Reserve + Confirm** — perfect for LLM APIs where token cost is unknown until after the call
- **Redis or in-memory** — single-process default, Redis for distributed setups
- **HTTP API server** — use from any language via REST, ships as a Docker image
- **YAML config** — change limits without redeploying

```
┌─────────────┐     Allow / Wait / Reserve      ┌────────────────┐
│  Your App   │ ──────────────────────────────▶  │  BudgetManager │
└─────────────┘                                  │                │
                                                 │  ┌──────────┐  │
                                                 │  │ Windows  │  │  ◀── per-API rate limits
                                                 │  └──────────┘  │
                                                 │  ┌──────────┐  │
                                                 │  │ Credits  │  │  ◀── shared budget pools
                                                 │  └──────────┘  │
                                                 │  ┌──────────┐  │
                                                 │  │  Store   │  │  ◀── Memory / Redis
                                                 │  └──────────┘  │
                                                 └────────────────┘
```

## Quick Start

### Install

```bash
go get github.com/ryutasato/apibudget
```

### Minimal example — rate limiting only

```go
package main

import (
    "fmt"
    "time"

    "github.com/ryutasato/apibudget"
)

func main() {
    mgr, _ := apibudget.NewBudgetManager(apibudget.ManagerConfig{
        APIs: []apibudget.RateConfig{
            {
                Name: "openai",
                Windows: []apibudget.Window{
                    {Duration: time.Minute, Limit: 60},
                    {Duration: 24 * time.Hour, Limit: 10000},
                },
            },
        },
    })

    allowed, next := mgr.Allow("openai")
    if !allowed {
        fmt.Printf("Rate limited until %v\n", next)
        return
    }
    fmt.Println("Request allowed")
}
```

### With credit budgets

```go
mgr, _ := apibudget.NewBudgetManager(apibudget.ManagerConfig{
    APIs: []apibudget.RateConfig{
        {Name: "openai", Windows: []apibudget.Window{{Duration: time.Minute, Limit: 60}}},
    },
    CreditPools: []apibudget.CreditPoolConfig{
        {
            Name:       "llm-budget",
            MaxCredits: apibudget.MustNewCredit("100000"),
            Window:     30 * 24 * time.Hour, // monthly reset
            Costs: []apibudget.CreditCost{
                {APIName: "openai", CostPerCall: apibudget.MustNewCredit("1.5")},
            },
        },
    },
})
```

### Reserve + Confirm (LLM token tracking)

When you don't know the actual cost until after the API call:

```go
r := mgr.Reserve("openai")
if !r.OK() {
    log.Fatal("rate limit exceeded")
}

// Call the LLM API...
// Actual tokens used: 3847

err := r.Confirm(apibudget.MustNewCredit("3847"))
if err != nil {
    // ErrInsufficientCredits — consumed but budget overdrawn
    log.Printf("warning: %v", err)
}
```

### Blocking wait

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

if err := mgr.Wait(ctx, "openai"); err != nil {
    log.Fatal(err) // context cancelled or deadline exceeded
}
// Proceed with API call
```

### YAML config

```yaml
# apibudget.yaml
apis:
  - name: openai
    windows:
      - duration: 1m
        limit: 60
      - duration: 1h
        limit: 1000
    buffer: 500ms

credit_pools:
  - name: llm-budget
    max_credits: "100000"
    window: 720h
    costs:
      - api: openai
        cost_per_call: "1.5"
```

```go
mgr, err := apibudget.NewBudgetManagerFromYAML("apibudget.yaml")
```

## HTTP API Server

Run as a standalone service for cross-language access:

```bash
# Docker
docker compose up -d

# Or directly
go run ./cmd/apibudget-server
```

| Endpoint | Method | Description |
|---|---|---|
| `/api/v1/allow` | POST | Check if request is allowed |
| `/api/v1/wait` | POST | Block until allowed (with timeout) |
| `/api/v1/reserve` | POST | Reserve rate slot + credits |
| `/api/v1/reserve/{id}/confirm` | POST | Confirm with actual cost |
| `/api/v1/reserve/{id}` | DELETE | Cancel reservation |
| `/api/v1/credits/{pool}` | GET | Get remaining credits |
| `/api/v1/credits/{pool}/reset` | POST | Reset credits to max |
| `/api/v1/tokens/{api}` | GET | Get available tokens |
| `/api/v1/health` | GET | Health check |

### Environment variables

| Variable | Default | Description |
|---|---|---|
| `APIBUDGET_CONFIG` | `apibudget.yaml` | YAML config path |
| `APIBUDGET_ADDR` | `:8080` | Listen address |
| `APIBUDGET_STORE_TYPE` | `memory` | `memory` or `redis` |
| `APIBUDGET_REDIS_ADDR` | `localhost:6379` | Redis address |
| `APIBUDGET_REDIS_PASSWORD` | | Redis password |
| `APIBUDGET_REDIS_DB` | `0` | Redis DB number |
| `APIBUDGET_LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` / `silent` |

## Features at a Glance

| Feature | Details |
|---|---|
| Multi-window rate limiting | Per-second + per-minute + per-day on the same API |
| Credit budgets | `*big.Rat` precision, shared pools across APIs |
| Batch consumption | Charge once every N calls (`BatchSize`) |
| Reserve + Confirm | For APIs with unknown upfront cost (LLMs) |
| Dynamic limits | `SetLimit()` changes take effect immediately |
| Redis backend | Lua-script atomic ops for distributed setups |
| YAML config | Hot-reload friendly, code-free limit changes |
| HTTP API | REST server in Docker for any language |
| Concurrency safe | `sync.Mutex` + atomic store operations |
| Property tested | 24 formal correctness properties via `testing/quick` |

## Redis Backend

```go
store, _ := apibudget.NewRedisStore("localhost:6379",
    apibudget.WithRedisPassword("secret"),
    apibudget.WithRedisDB(1),
)
defer store.Close()

mgr, _ := apibudget.NewBudgetManager(apibudget.ManagerConfig{
    Store: store,
    APIs:  []apibudget.RateConfig{...},
})
```

## Development

```bash
make build       # Build binary
make test        # Run tests with -race
make lint        # Run golangci-lint
make docker      # Build Docker image
```

### Requirements

- Go 1.22+ (tested on 1.22, 1.23, 1.24)
- Redis 7+ (optional, for distributed mode)

## Documentation

- [Getting Started](https://ryutasato.github.io/apibudget/getting-started/)
- [Configuration Reference](https://ryutasato.github.io/apibudget/configuration/)
- [API Reference](https://ryutasato.github.io/apibudget/api-reference/)
- [Docker Guide](https://ryutasato.github.io/apibudget/docker/)
- [日本語ドキュメント](https://ryutasato.github.io/apibudget/ja/)
- [Go Package Docs](https://pkg.go.dev/github.com/ryutasato/apibudget)

## Contributing

Contributions are welcome. Please open an issue first to discuss what you'd like to change.

```bash
git clone https://github.com/ryutasato/apibudget.git
cd apibudget
make test
```

## License

[MIT](LICENSE)
