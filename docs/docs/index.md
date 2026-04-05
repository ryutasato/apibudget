# apibudget

Rate limiting and budget management for external APIs in Go.

## Overview

apibudget is a Go library that provides unified rate limiting and credit/token budget management for external APIs. It follows the `Allow/Reserve/Wait` pattern from `golang.org/x/time/rate` while adding:

- Multiple time windows per API (second/minute/hour/day/month)
- Credit pool management with shared budgets across APIs
- Batch consumption support
- Redis and in-memory backends
- HTTP API server mode
- YAML configuration

## Features

- **Multi-window rate limiting** — Apply multiple time windows simultaneously per API
- **Credit pools** — Manage shared budgets across multiple APIs with precise `big.Rat` arithmetic
- **Allow/Wait/Reserve** — Familiar API pattern from `golang.org/x/time/rate`
- **Reservation with Confirm/Cancel** — Reserve credits upfront, adjust after actual usage
- **Batch consumption** — Consume credits every N calls instead of every call
- **Redis backend** — Share rate limit state across processes with atomic Lua scripts
- **API server** — Expose rate limiting via REST API for non-Go clients
- **Docker ready** — Distroless container with docker-compose for Redis + server

## Quick Example

```go
package main

import (
    "context"
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
                },
            },
        },
    })

    allowed, next := mgr.Allow("openai")
    if allowed {
        fmt.Println("Request allowed")
    } else {
        fmt.Printf("Rate limited, retry after %v\n", next)
    }

    _ = mgr.Wait(context.Background(), "openai")
    fmt.Println("Request proceeded after wait")
}
```

## License

MIT
