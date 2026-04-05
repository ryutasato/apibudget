# Configuration

apibudget uses YAML files for API and credit pool definitions. Server, store, and log settings are configured via environment variables.

## YAML Configuration

### APIs

```yaml
apis:
  - name: openai           # Required: unique API name
    windows:                # Required: at least one time window
      - duration: 1m       # Window duration (s, m, h)
        limit: 60           # Max requests per window
      - duration: 1h
        limit: 1000
    buffer: 500ms           # Optional: safety buffer added to nextAvailable (default: 0)
```

### Credit Pools

```yaml
credit_pools:
  - name: llm-budget        # Required: unique pool name
    max_credits: "100000"    # Required: maximum credit balance
    window: 720h             # Optional: auto-reset period (0 = no auto-reset)
    initial: "50000"         # Optional: initial balance (default: max_credits)
    costs:                   # Required: at least one cost rule
      - api: openai          # API name to associate
        cost_per_call: "1.5" # Optional: credits per call (default: "1")
        batch_size: 1        # Optional: consume every N calls (default: 1)
```

### Default Values

| Field | Default |
|---|---|
| `buffer` | `0` |
| `cost_per_call` | `"1"` |
| `batch_size` | `1` |
| `initial` | same as `max_credits` |

## Environment Variables

These control server, store, and logging settings:

| Variable | Default | Description |
|---|---|---|
| `APIBUDGET_CONFIG` | `/etc/apibudget/apibudget.yaml` | YAML config file path |
| `APIBUDGET_ADDR` | `:8080` | Server listen address |
| `APIBUDGET_STORE_TYPE` | `memory` | Store backend (`memory` / `redis`) |
| `APIBUDGET_REDIS_ADDR` | `localhost:6379` | Redis address |
| `APIBUDGET_REDIS_PASSWORD` | (empty) | Redis password |
| `APIBUDGET_REDIS_DB` | `0` | Redis database number |
| `APIBUDGET_REDIS_TLS` | `false` | Enable Redis TLS |
| `APIBUDGET_LOG_LEVEL` | `info` | Log level (`debug`/`info`/`warn`/`error`/`silent`) |

## Programmatic Configuration

```go
mgr, err := apibudget.NewBudgetManager(apibudget.ManagerConfig{
    APIs: []apibudget.RateConfig{...},
    CreditPools: []apibudget.CreditPoolConfig{...},
    Store:    store,          // nil = in-memory
    Logger:   logger,         // nil = slog.Default()
    LogLevel: apibudget.LogLevelInfo,
})
```

### Manager Options (with YAML)

```go
mgr, err := apibudget.NewBudgetManagerFromYAML("apibudget.yaml",
    apibudget.WithStore(redisStore),
    apibudget.WithLogger(myLogger),
    apibudget.WithLogLevel(apibudget.LogLevelDebug),
)
```
