# API Reference

## BudgetManager

### Allow / AllowN

```go
func (m *BudgetManager) Allow(apiName string) (bool, time.Time)
func (m *BudgetManager) AllowN(apiName string, n int64) (bool, time.Time)
```

Non-blocking rate limit check. Returns `(true, zero)` if allowed, `(false, nextAvailable)` if denied.

### Wait / WaitN

```go
func (m *BudgetManager) Wait(ctx context.Context, apiName string) error
func (m *BudgetManager) WaitN(ctx context.Context, apiName string, n int64) error
```

Blocks until rate limit allows the request. Returns `context.Canceled` or `context.DeadlineExceeded` on context cancellation.

### Reserve / ReserveN

```go
func (m *BudgetManager) Reserve(apiName string) *Reservation
func (m *BudgetManager) ReserveN(apiName string, n int64) *Reservation
```

Reserves rate slots and credits. Always returns non-nil. Check `r.OK()` for success.

### SetLimit

```go
func (m *BudgetManager) SetLimit(apiName string, duration time.Duration, newLimit int64) error
```

Dynamically changes the rate limit for a specific API window.

### Tokens

```go
func (m *BudgetManager) Tokens(apiName string) (int64, error)
```

Returns available tokens based on the most restrictive window.

### Credit Pool Operations

```go
func (m *BudgetManager) GetCredits(poolName string) (Credit, error)
func (m *BudgetManager) ResetCredits(poolName string) error
func (m *BudgetManager) AddCredits(poolName string, amount Credit) error
func (m *BudgetManager) SetCredits(poolName string, amount Credit) error
```

## Reservation

```go
func (r *Reservation) OK() bool
func (r *Reservation) Delay() time.Duration
func (r *Reservation) Cancel()
func (r *Reservation) CancelAt(t time.Time)
func (r *Reservation) Confirm(actualCost Credit) error
```

## Credit

```go
func NewCredit(s string) (Credit, error)
func MustNewCredit(s string) Credit
func NewCreditFromInt(n int64) Credit
func (c Credit) Add(other Credit) Credit
func (c Credit) Sub(other Credit) Credit
func (c Credit) Mul(other Credit) Credit
func (c Credit) Cmp(other Credit) int
func (c Credit) IsZero() bool
func (c Credit) IsNegative() bool
func (c Credit) String() string
func (c Credit) Float64() float64
```

## REST API Endpoints

### POST /api/v1/allow

Request: `{"api": "openai", "n": 1}`
Response: `{"allowed": true, "next_available": "..."}`

### POST /api/v1/reserve

Request: `{"api": "openai", "n": 1}`
Response: `{"id": "uuid", "ok": true, "delay_ms": 0}`

### POST /api/v1/reserve/{id}/confirm

Request: `{"actual_cost": "3.5"}`
Response: `{"status": "confirmed"}`

### DELETE /api/v1/reserve/{id}

Response: `{"status": "cancelled"}`

### POST /api/v1/wait

Request: `{"api": "openai", "n": 1, "timeout_ms": 5000}`
Response: `{"status": "ok"}`

### GET /api/v1/credits/{pool}

Response: `{"pool": "llm-budget", "credits": "99998.5"}`

### POST /api/v1/credits/{pool}/reset

Response: `{"pool": "llm-budget", "credits": "100000"}`

### GET /api/v1/tokens/{api}

Response: `{"api": "openai", "tokens": 58}`

### GET /api/v1/health

Response: `{"status": "ok"}`
