# Getting Started

## Installation

```bash
go get github.com/ryutasato/apibudget
```

## Basic Usage

### 1. Create a BudgetManager

```go
mgr, err := apibudget.NewBudgetManager(apibudget.ManagerConfig{
    APIs: []apibudget.RateConfig{
        {
            Name: "my-api",
            Windows: []apibudget.Window{
                {Duration: time.Minute, Limit: 60},
                {Duration: time.Hour, Limit: 1000},
            },
            Buffer: 500 * time.Millisecond,
        },
    },
})
```

### 2. Check Rate Limits

Use `Allow` for non-blocking checks:

```go
allowed, nextAvailable := mgr.Allow("my-api")
if !allowed {
    fmt.Printf("Retry after: %v\n", nextAvailable)
}
```

Use `Wait` for blocking waits:

```go
err := mgr.Wait(ctx, "my-api")
if err != nil {
    // context cancelled or deadline exceeded
}
```

### 3. Use Reservations

For APIs where actual cost is unknown upfront (e.g., LLM token usage):

```go
r := mgr.Reserve("my-api")
if r.OK() {
    // Make API call...
    actualCost := apibudget.MustNewCredit("3.5")
    err := r.Confirm(actualCost)
} else {
    r.Cancel()
}
```

### 4. Add Credit Pools

```go
mgr, err := apibudget.NewBudgetManager(apibudget.ManagerConfig{
    APIs: []apibudget.RateConfig{
        {Name: "openai", Windows: []apibudget.Window{{Duration: time.Minute, Limit: 60}}},
    },
    CreditPools: []apibudget.CreditPoolConfig{
        {
            Name:       "llm-budget",
            MaxCredits: apibudget.MustNewCredit("100000"),
            Costs: []apibudget.CreditCost{
                {APIName: "openai", CostPerCall: apibudget.MustNewCredit("1.5")},
            },
        },
    },
})
```

## YAML Configuration

Create an `apibudget.yaml` file:

```yaml
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
    costs:
      - api: openai
        cost_per_call: "1.5"
```

Load it:

```go
mgr, err := apibudget.NewBudgetManagerFromYAML("apibudget.yaml")
```

See [Configuration](configuration.md) for the full reference.
