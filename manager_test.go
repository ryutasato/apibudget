package apibudget

import (
	"context"
	"math/rand"
	"testing"
	"testing/quick"
	"time"
)

func TestNewBudgetManager_MinimalConfig(t *testing.T) {
	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{
				Name:    "test_api",
				Windows: []Window{{Duration: time.Minute, Limit: 60}},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil BudgetManager")
	}
	if m.store == nil {
		t.Fatal("expected default store to be set")
	}
	if m.logger == nil {
		t.Fatal("expected default logger to be set")
	}
	if _, ok := m.apis["test_api"]; !ok {
		t.Fatal("expected test_api in apis map")
	}
}

func TestNewBudgetManager_DefaultCostPerCallAndBatchSize(t *testing.T) {
	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "api1", Windows: []Window{{Duration: time.Second, Limit: 10}}},
		},
		CreditPools: []CreditPoolConfig{
			{
				Name:       "pool1",
				MaxCredits: MustNewCredit("100"),
				Costs: []CreditCost{
					{APIName: "api1"}, // CostPerCall and BatchSize are zero values
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info := m.apiToPool["api1"]
	if info == nil {
		t.Fatal("expected api1 in apiToPool map")
	}
	if info.cost.CostPerCall.Cmp(MustNewCredit("1")) != 0 {
		t.Errorf("expected default CostPerCall=1, got %s", info.cost.CostPerCall.String())
	}
	if info.cost.BatchSize != 1 {
		t.Errorf("expected default BatchSize=1, got %d", info.cost.BatchSize)
	}
}

func TestNewBudgetManager_InitialCreditDefault(t *testing.T) {
	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "api1", Windows: []Window{{Duration: time.Second, Limit: 10}}},
		},
		CreditPools: []CreditPoolConfig{
			{
				Name:       "pool1",
				MaxCredits: MustNewCredit("500"),
				Costs: []CreditCost{
					{APIName: "api1", CostPerCall: MustNewCredit("1"), BatchSize: 1},
				},
				// Initial is nil → should default to MaxCredits
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	credit, err := m.store.GetCredit(context.Background(), "pool1")
	if err != nil {
		t.Fatalf("unexpected error getting credit: %v", err)
	}
	if credit.Cmp(MustNewCredit("500")) != 0 {
		t.Errorf("expected initial credit=500, got %s", credit.String())
	}
}

func TestNewBudgetManager_InitialCreditExplicit(t *testing.T) {
	initial := MustNewCredit("200")
	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "api1", Windows: []Window{{Duration: time.Second, Limit: 10}}},
		},
		CreditPools: []CreditPoolConfig{
			{
				Name:       "pool1",
				MaxCredits: MustNewCredit("500"),
				Initial:    &initial,
				Costs: []CreditCost{
					{APIName: "api1", CostPerCall: MustNewCredit("1"), BatchSize: 1},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	credit, err := m.store.GetCredit(context.Background(), "pool1")
	if err != nil {
		t.Fatalf("unexpected error getting credit: %v", err)
	}
	if credit.Cmp(MustNewCredit("200")) != 0 {
		t.Errorf("expected initial credit=200, got %s", credit.String())
	}
}

func TestNewBudgetManager_EmptyAPIName(t *testing.T) {
	_, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "", Windows: []Window{{Duration: time.Second, Limit: 10}}},
		},
	})
	if err == nil {
		t.Fatal("expected error for empty API name")
	}
}

func TestNewBudgetManager_DuplicateAPIName(t *testing.T) {
	_, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "dup", Windows: []Window{{Duration: time.Second, Limit: 10}}},
			{Name: "dup", Windows: []Window{{Duration: time.Minute, Limit: 100}}},
		},
	})
	if err == nil {
		t.Fatal("expected error for duplicate API name")
	}
}

func TestNewBudgetManager_EmptyPoolName(t *testing.T) {
	_, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "api1", Windows: []Window{{Duration: time.Second, Limit: 10}}},
		},
		CreditPools: []CreditPoolConfig{
			{
				Name:       "",
				MaxCredits: MustNewCredit("100"),
				Costs:      []CreditCost{{APIName: "api1"}},
			},
		},
	})
	if err == nil {
		t.Fatal("expected error for empty pool name")
	}
}

func TestNewBudgetManager_DuplicatePoolName(t *testing.T) {
	_, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "api1", Windows: []Window{{Duration: time.Second, Limit: 10}}},
		},
		CreditPools: []CreditPoolConfig{
			{
				Name:       "dup_pool",
				MaxCredits: MustNewCredit("100"),
				Costs:      []CreditCost{{APIName: "api1"}},
			},
			{
				Name:       "dup_pool",
				MaxCredits: MustNewCredit("200"),
				Costs:      []CreditCost{{APIName: "api1"}},
			},
		},
	})
	if err == nil {
		t.Fatal("expected error for duplicate pool name")
	}
}

func TestNewBudgetManager_CustomStoreAndLogger(t *testing.T) {
	store := NewMemoryStore()
	logger := newDefaultLogger(LogLevelDebug)

	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "api1", Windows: []Window{{Duration: time.Second, Limit: 10}}},
		},
		Store:  store,
		Logger: logger,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.store != store {
		t.Error("expected custom store to be used")
	}
	if m.logger != logger {
		t.Error("expected custom logger to be used")
	}
}

func TestNewBudgetManager_MultiplePoolsAndAPIs(t *testing.T) {
	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "api1", Windows: []Window{{Duration: time.Second, Limit: 10}}},
			{Name: "api2", Windows: []Window{{Duration: time.Minute, Limit: 100}}},
			{Name: "api3", Windows: []Window{{Duration: time.Hour, Limit: 1000}}},
		},
		CreditPools: []CreditPoolConfig{
			{
				Name:       "pool_a",
				MaxCredits: MustNewCredit("1000"),
				Costs: []CreditCost{
					{APIName: "api1", CostPerCall: MustNewCredit("2"), BatchSize: 5},
					{APIName: "api2"},
				},
			},
			{
				Name:       "pool_b",
				MaxCredits: MustNewCredit("500"),
				Costs: []CreditCost{
					{APIName: "api3", CostPerCall: MustNewCredit("0.5")},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify API mappings
	if len(m.apis) != 3 {
		t.Errorf("expected 3 apis, got %d", len(m.apis))
	}
	if len(m.pools) != 2 {
		t.Errorf("expected 2 pools, got %d", len(m.pools))
	}

	// Verify api1 → pool_a mapping
	info1 := m.apiToPool["api1"]
	if info1 == nil || info1.poolName != "pool_a" {
		t.Error("expected api1 mapped to pool_a")
	}
	if info1.cost.CostPerCall.Cmp(MustNewCredit("2")) != 0 {
		t.Errorf("expected api1 CostPerCall=2, got %s", info1.cost.CostPerCall.String())
	}
	if info1.cost.BatchSize != 5 {
		t.Errorf("expected api1 BatchSize=5, got %d", info1.cost.BatchSize)
	}

	// Verify api2 → pool_a with defaults
	info2 := m.apiToPool["api2"]
	if info2 == nil || info2.poolName != "pool_a" {
		t.Error("expected api2 mapped to pool_a")
	}
	if info2.cost.CostPerCall.Cmp(MustNewCredit("1")) != 0 {
		t.Errorf("expected api2 default CostPerCall=1, got %s", info2.cost.CostPerCall.String())
	}

	// Verify api3 → pool_b
	info3 := m.apiToPool["api3"]
	if info3 == nil || info3.poolName != "pool_b" {
		t.Error("expected api3 mapped to pool_b")
	}

	// Verify initial credits
	creditA, _ := m.store.GetCredit(context.Background(), "pool_a")
	if creditA.Cmp(MustNewCredit("1000")) != 0 {
		t.Errorf("expected pool_a initial=1000, got %s", creditA.String())
	}
	creditB, _ := m.store.GetCredit(context.Background(), "pool_b")
	if creditB.Cmp(MustNewCredit("500")) != 0 {
		t.Errorf("expected pool_b initial=500, got %s", creditB.String())
	}
}

func TestNewBudgetManager_NoCreditPools(t *testing.T) {
	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "api1", Windows: []Window{{Duration: time.Second, Limit: 10}}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.apiToPool) != 0 {
		t.Errorf("expected empty apiToPool, got %d entries", len(m.apiToPool))
	}
}

// --- Allow / AllowN Tests ---

func TestAllow_BasicAllow(t *testing.T) {
	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "api1", Windows: []Window{{Duration: time.Minute, Limit: 5}}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	allowed, next := m.Allow("api1")
	if !allowed {
		t.Errorf("expected allowed=true, got false, next=%v", next)
	}
	if !next.IsZero() {
		t.Errorf("expected zero time on success, got %v", next)
	}
}

func TestAllow_UnregisteredAPI(t *testing.T) {
	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "api1", Windows: []Window{{Duration: time.Minute, Limit: 5}}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	allowed, next := m.Allow("unknown_api")
	if allowed {
		t.Error("expected allowed=false for unregistered API")
	}
	if !next.IsZero() {
		t.Errorf("expected zero time for unregistered API, got %v", next)
	}
}

func TestAllowN_ExceedsWindowLimit(t *testing.T) {
	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "api1", Windows: []Window{{Duration: time.Minute, Limit: 3}}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	now := time.Now()

	// Use up the limit
	for i := 0; i < 3; i++ {
		allowed, _ := m.AllowN("api1", 1, now)
		if !allowed {
			t.Fatalf("expected allowed=true on call %d", i+1)
		}
	}

	// 4th call should be rejected
	allowed, next := m.AllowN("api1", 1, now)
	if allowed {
		t.Error("expected allowed=false when limit exceeded")
	}
	if next.IsZero() {
		t.Error("expected non-zero nextAvailable when limit exceeded")
	}
	// nextAvailable should be after now
	if !next.After(now) {
		t.Errorf("expected nextAvailable after now, got %v", next)
	}
}

func TestAllowN_MultipleWindows(t *testing.T) {
	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{
				Name: "api1",
				Windows: []Window{
					{Duration: time.Second, Limit: 2},
					{Duration: time.Minute, Limit: 5},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	now := time.Now()

	// Use up the per-second limit
	for i := 0; i < 2; i++ {
		allowed, _ := m.AllowN("api1", 1, now)
		if !allowed {
			t.Fatalf("expected allowed=true on call %d", i+1)
		}
	}

	// 3rd call in same second should be rejected (per-second limit)
	allowed, _ := m.AllowN("api1", 1, now)
	if allowed {
		t.Error("expected allowed=false when per-second limit exceeded")
	}
}

func TestAllowN_NoStateChangeOnFailure(t *testing.T) {
	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "api1", Windows: []Window{{Duration: time.Minute, Limit: 2}}},
		},
		CreditPools: []CreditPoolConfig{
			{
				Name:       "pool1",
				MaxCredits: MustNewCredit("100"),
				Costs: []CreditCost{
					{APIName: "api1", CostPerCall: MustNewCredit("1"), BatchSize: 1},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	now := time.Now()

	// Use up the limit
	m.AllowN("api1", 1, now)
	m.AllowN("api1", 1, now)

	// Get credit before failed attempt
	creditBefore, _ := m.store.GetCredit(context.Background(), "pool1")

	// This should fail (limit exceeded)
	allowed, _ := m.AllowN("api1", 1, now)
	if allowed {
		t.Fatal("expected allowed=false")
	}

	// Credit should be unchanged
	creditAfter, _ := m.store.GetCredit(context.Background(), "pool1")
	if creditBefore.Cmp(creditAfter) != 0 {
		t.Errorf("expected credit unchanged on failure, before=%s, after=%s",
			creditBefore.String(), creditAfter.String())
	}
}

func TestAllowN_CreditDeduction(t *testing.T) {
	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "api1", Windows: []Window{{Duration: time.Minute, Limit: 100}}},
		},
		CreditPools: []CreditPoolConfig{
			{
				Name:       "pool1",
				MaxCredits: MustNewCredit("10"),
				Costs: []CreditCost{
					{APIName: "api1", CostPerCall: MustNewCredit("3"), BatchSize: 1},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	now := time.Now()

	// First call: 10 - 3 = 7
	allowed, _ := m.AllowN("api1", 1, now)
	if !allowed {
		t.Fatal("expected allowed=true")
	}
	credit, _ := m.store.GetCredit(context.Background(), "pool1")
	if credit.Cmp(MustNewCredit("7")) != 0 {
		t.Errorf("expected credit=7, got %s", credit.String())
	}

	// Second call: 7 - 3 = 4
	allowed, _ = m.AllowN("api1", 1, now)
	if !allowed {
		t.Fatal("expected allowed=true")
	}
	credit, _ = m.store.GetCredit(context.Background(), "pool1")
	if credit.Cmp(MustNewCredit("4")) != 0 {
		t.Errorf("expected credit=4, got %s", credit.String())
	}

	// Third call: 4 - 3 = 1
	allowed, _ = m.AllowN("api1", 1, now)
	if !allowed {
		t.Fatal("expected allowed=true")
	}
	credit, _ = m.store.GetCredit(context.Background(), "pool1")
	if credit.Cmp(MustNewCredit("1")) != 0 {
		t.Errorf("expected credit=1, got %s", credit.String())
	}

	// Fourth call: 1 - 3 < 0 → insufficient credits
	allowed, _ = m.AllowN("api1", 1, now)
	if allowed {
		t.Error("expected allowed=false when credits insufficient")
	}

	// Credit should remain at 1 (no change on failure)
	credit, _ = m.store.GetCredit(context.Background(), "pool1")
	if credit.Cmp(MustNewCredit("1")) != 0 {
		t.Errorf("expected credit=1 after failed deduction, got %s", credit.String())
	}
}

func TestAllowN_WithBuffer(t *testing.T) {
	buffer := 500 * time.Millisecond
	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{
				Name:    "api1",
				Windows: []Window{{Duration: time.Second, Limit: 1}},
				Buffer:  buffer,
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	now := time.Now()

	// Use up the limit
	m.AllowN("api1", 1, now)

	// Should be rejected with nextAvailable = windowReset + buffer
	allowed, next := m.AllowN("api1", 1, now)
	if allowed {
		t.Error("expected allowed=false")
	}

	expectedReset := windowResetTime(now, time.Second)
	expectedNext := expectedReset.Add(buffer)
	if !next.Equal(expectedNext) {
		t.Errorf("expected nextAvailable=%v, got %v", expectedNext, next)
	}
}

func TestAllowN_NoCreditPool(t *testing.T) {
	// API without credit pool should work based on rate limits only
	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "api1", Windows: []Window{{Duration: time.Minute, Limit: 10}}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	now := time.Now()
	for i := 0; i < 10; i++ {
		allowed, _ := m.AllowN("api1", 1, now)
		if !allowed {
			t.Fatalf("expected allowed=true on call %d", i+1)
		}
	}

	// 11th should fail
	allowed, _ := m.AllowN("api1", 1, now)
	if allowed {
		t.Error("expected allowed=false when limit exceeded")
	}
}

func TestAllowN_MultipleRequestsAtOnce(t *testing.T) {
	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "api1", Windows: []Window{{Duration: time.Minute, Limit: 10}}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	now := time.Now()

	// Request 5 at once
	allowed, _ := m.AllowN("api1", 5, now)
	if !allowed {
		t.Fatal("expected allowed=true for n=5")
	}

	// Request 5 more
	allowed, _ = m.AllowN("api1", 5, now)
	if !allowed {
		t.Fatal("expected allowed=true for second n=5")
	}

	// Request 1 more should fail
	allowed, _ = m.AllowN("api1", 1, now)
	if allowed {
		t.Error("expected allowed=false when limit exceeded")
	}
}

func TestAllowN_CreditInsufficientNoCounterChange(t *testing.T) {
	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "api1", Windows: []Window{{Duration: time.Minute, Limit: 100}}},
		},
		CreditPools: []CreditPoolConfig{
			{
				Name:       "pool1",
				MaxCredits: MustNewCredit("2"),
				Costs: []CreditCost{
					{APIName: "api1", CostPerCall: MustNewCredit("1"), BatchSize: 1},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	now := time.Now()

	// Use up credits (2 calls)
	m.AllowN("api1", 1, now)
	m.AllowN("api1", 1, now)

	// Get window count before failed attempt
	key := windowKey("api1", time.Minute, now)
	countBefore, _ := m.store.GetWindowCount(context.Background(), key)

	// This should fail (credits exhausted)
	allowed, _ := m.AllowN("api1", 1, now)
	if allowed {
		t.Fatal("expected allowed=false when credits exhausted")
	}

	// Window counter should not have changed
	countAfter, _ := m.store.GetWindowCount(context.Background(), key)
	if countBefore != countAfter {
		t.Errorf("expected counter unchanged on credit failure, before=%d, after=%d",
			countBefore, countAfter)
	}
}

// --- Property-Based Tests for Allow/AllowN ---

// TestProperty3_RateLimitConsistency verifies that for any API and window,
// the total number of successful Allow/AllowN calls within a window never exceeds window.Limit.
// **Validates: Requirements 2.1, 2.2, 3.1, 3.2**
func TestProperty3_RateLimitConsistency(t *testing.T) {
	cfg := &quick.Config{MaxCount: 200}

	f := func(limitRaw uint8) bool {
		// Constrain limit to 1-100
		limit := int64(limitRaw%100) + 1

		m, err := NewBudgetManager(ManagerConfig{
			APIs: []RateConfig{
				{Name: "prop3_api", Windows: []Window{{Duration: time.Minute, Limit: limit}}},
			},
		})
		if err != nil {
			return false
		}

		now := time.Now()
		successCount := int64(0)

		// Try limit+10 calls to ensure we go past the limit
		for i := int64(0); i < limit+10; i++ {
			allowed, _ := m.AllowN("prop3_api", 1, now)
			if allowed {
				successCount++
			}
		}

		return successCount <= limit
	}

	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("Property 3 failed: %v", err)
	}
}

// TestProperty4_AllowFailureStateInvariance verifies that when Allow returns false
// (window exceeded or credit insufficient), counters and credits are unchanged.
// **Validates: Requirements 3.3, 3.4**
func TestProperty4_AllowFailureStateInvariance(t *testing.T) {
	cfg := &quick.Config{MaxCount: 200}

	f := func(limitRaw uint8, creditRaw uint8) bool {
		limit := int64(limitRaw%20) + 1
		creditVal := int64(creditRaw%20) + 1

		m, err := NewBudgetManager(ManagerConfig{
			APIs: []RateConfig{
				{Name: "prop4_api", Windows: []Window{{Duration: time.Minute, Limit: limit}}},
			},
			CreditPools: []CreditPoolConfig{
				{
					Name:       "prop4_pool",
					MaxCredits: NewCreditFromInt(creditVal),
					Costs: []CreditCost{
						{APIName: "prop4_api", CostPerCall: MustNewCredit("1"), BatchSize: 1},
					},
				},
			},
		})
		if err != nil {
			return false
		}

		now := time.Now()

		// Exhaust whichever is smaller: limit or credits
		exhaust := limit
		if creditVal < limit {
			exhaust = creditVal
		}
		for i := int64(0); i < exhaust; i++ {
			m.AllowN("prop4_api", 1, now)
		}

		// Capture state before the failing call
		key := windowKey("prop4_api", time.Minute, now)
		countBefore, _ := m.store.GetWindowCount(context.Background(), key)
		creditBefore, _ := m.store.GetCredit(context.Background(), "prop4_pool")

		// This call should fail
		allowed, _ := m.AllowN("prop4_api", 1, now)
		if allowed {
			// If it was allowed, that's fine — skip this case
			return true
		}

		// Verify state unchanged
		countAfter, _ := m.store.GetWindowCount(context.Background(), key)
		creditAfter, _ := m.store.GetCredit(context.Background(), "prop4_pool")

		return countBefore == countAfter && creditBefore.Cmp(creditAfter) == 0
	}

	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("Property 4 failed: %v", err)
	}
}

// TestProperty5_NextAvailableAccuracyWithBuffer verifies that when a request is rejected,
// nextAvailable >= latest window reset time + Buffer.
// **Validates: Requirements 2.3, 2.4**
func TestProperty5_NextAvailableAccuracyWithBuffer(t *testing.T) {
	cfg := &quick.Config{MaxCount: 200}

	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		// Random buffer between 0 and 2000ms
		bufferMs := rng.Intn(2001)
		buffer := time.Duration(bufferMs) * time.Millisecond

		// Random limit 1-20
		limit := int64(rng.Intn(20)) + 1

		m, err := NewBudgetManager(ManagerConfig{
			APIs: []RateConfig{
				{
					Name:    "prop5_api",
					Windows: []Window{{Duration: time.Minute, Limit: limit}},
					Buffer:  buffer,
				},
			},
		})
		if err != nil {
			return false
		}

		now := time.Now()

		// Exhaust the limit
		for i := int64(0); i < limit; i++ {
			m.AllowN("prop5_api", 1, now)
		}

		// Next call should be rejected
		allowed, nextAvail := m.AllowN("prop5_api", 1, now)
		if allowed {
			return true // limit wasn't actually hit, skip
		}

		// nextAvailable should be >= windowResetTime + buffer
		expectedMin := windowResetTime(now, time.Minute).Add(buffer)
		return !nextAvail.Before(expectedMin)
	}

	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("Property 5 failed: %v", err)
	}
}

// TestProperty17_UnregisteredAPIErrorHandling verifies that unregistered API names
// return (false, zero time).
// **Validates: Requirements 3.5, 6.9, 17.2, 17.3**
func TestProperty17_UnregisteredAPIErrorHandling(t *testing.T) {
	cfg := &quick.Config{MaxCount: 200}

	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "registered_api", Windows: []Window{{Duration: time.Minute, Limit: 10}}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		// Generate a random unregistered API name
		nameLen := rng.Intn(20) + 1
		nameBytes := make([]byte, nameLen)
		for i := range nameBytes {
			nameBytes[i] = byte(rng.Intn(26) + 'a')
		}
		name := "unregistered_" + string(nameBytes)

		allowed, next := m.Allow(name)
		return !allowed && next.IsZero()
	}

	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("Property 17 failed: %v", err)
	}
}

// --- calculateConsumption Tests ---

func TestCalculateConsumption_BatchSizeOne(t *testing.T) {
	cost := CreditCost{
		APIName:     "api1",
		CostPerCall: MustNewCredit("2"),
		BatchSize:   1,
	}

	credit, _ := calculateConsumption(cost, 1, 0)
	if credit.Cmp(MustNewCredit("2")) != 0 {
		t.Errorf("expected 2, got %s", credit.String())
	}

	credit, _ = calculateConsumption(cost, 5, 0)
	if credit.Cmp(MustNewCredit("10")) != 0 {
		t.Errorf("expected 10, got %s", credit.String())
	}
}

func TestCalculateConsumption_BatchSizeGreaterThanOne_FirstCall(t *testing.T) {
	cost := CreditCost{
		APIName:     "api1",
		CostPerCall: MustNewCredit("3"),
		BatchSize:   5,
	}

	// First call (totalCallsBefore=0, n=1): ceil(1/5)=1 batch → charge 3
	credit, totalAfter := calculateConsumption(cost, 1, 0)
	if credit.Cmp(MustNewCredit("3")) != 0 {
		t.Errorf("expected 3, got %s", credit.String())
	}
	if totalAfter != 1 {
		t.Errorf("expected totalAfter=1, got %d", totalAfter)
	}
}

func TestCalculateConsumption_BatchSizeGreaterThanOne_NoCrossing(t *testing.T) {
	cost := CreditCost{
		APIName:     "api1",
		CostPerCall: MustNewCredit("3"),
		BatchSize:   5,
	}

	// totalCallsBefore=1, n=1: ceil(1/5)=1, ceil(2/5)=1, diff=0
	credit, totalAfter := calculateConsumption(cost, 1, 1)
	if !credit.IsZero() {
		t.Errorf("expected 0, got %s", credit.String())
	}
	if totalAfter != 2 {
		t.Errorf("expected totalAfter=2, got %d", totalAfter)
	}
}

func TestCalculateConsumption_BatchSizeGreaterThanOne_CrossBoundary(t *testing.T) {
	cost := CreditCost{
		APIName:     "api1",
		CostPerCall: MustNewCredit("3"),
		BatchSize:   5,
	}

	// totalCallsBefore=5, n=1: ceil(5/5)=1, ceil(6/5)=2, diff=1
	credit, totalAfter := calculateConsumption(cost, 1, 5)
	if credit.Cmp(MustNewCredit("3")) != 0 {
		t.Errorf("expected 3, got %s", credit.String())
	}
	if totalAfter != 6 {
		t.Errorf("expected totalAfter=6, got %d", totalAfter)
	}
}

func TestCalculateConsumption_BatchSizeGreaterThanOne_MultipleCrossings(t *testing.T) {
	cost := CreditCost{
		APIName:     "api1",
		CostPerCall: MustNewCredit("2"),
		BatchSize:   3,
	}

	// totalCallsBefore=0, n=7: ceil(0/3)=0, ceil(7/3)=3, diff=3
	credit, totalAfter := calculateConsumption(cost, 7, 0)
	if credit.Cmp(MustNewCredit("6")) != 0 {
		t.Errorf("expected 6 (3 batches * 2), got %s", credit.String())
	}
	if totalAfter != 7 {
		t.Errorf("expected totalAfter=7, got %d", totalAfter)
	}
}

func TestCalculateConsumption_BatchSizeGreaterThanOne_ExactBoundary(t *testing.T) {
	cost := CreditCost{
		APIName:     "api1",
		CostPerCall: MustNewCredit("1"),
		BatchSize:   3,
	}

	// totalCallsBefore=0, n=3: ceil(0/3)=0, ceil(3/3)=1, diff=1
	credit, totalAfter := calculateConsumption(cost, 3, 0)
	if credit.Cmp(MustNewCredit("1")) != 0 {
		t.Errorf("expected 1, got %s", credit.String())
	}
	if totalAfter != 3 {
		t.Errorf("expected totalAfter=3, got %d", totalAfter)
	}
}

// --- AllowN Batch Consumption Integration Tests ---

func TestAllowN_BatchConsumption_BatchSize1(t *testing.T) {
	// BatchSize=1 should consume every call (same as before)
	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "api1", Windows: []Window{{Duration: time.Minute, Limit: 100}}},
		},
		CreditPools: []CreditPoolConfig{
			{
				Name:       "pool1",
				MaxCredits: MustNewCredit("10"),
				Costs: []CreditCost{
					{APIName: "api1", CostPerCall: MustNewCredit("1"), BatchSize: 1},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	now := time.Now()
	for i := 0; i < 10; i++ {
		allowed, _ := m.AllowN("api1", 1, now)
		if !allowed {
			t.Fatalf("expected allowed=true on call %d", i+1)
		}
	}

	// 11th call should fail (credits exhausted)
	allowed, _ := m.AllowN("api1", 1, now)
	if allowed {
		t.Error("expected allowed=false when credits exhausted")
	}

	credit, _ := m.store.GetCredit(context.Background(), "pool1")
	if !credit.IsZero() {
		t.Errorf("expected credit=0, got %s", credit.String())
	}
}

func TestAllowN_BatchConsumption_BatchSize5(t *testing.T) {
	// BatchSize=5: first call consumes (batch 1), calls 2-5 are free, call 6 starts batch 2
	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "api1", Windows: []Window{{Duration: time.Minute, Limit: 100}}},
		},
		CreditPools: []CreditPoolConfig{
			{
				Name:       "pool1",
				MaxCredits: MustNewCredit("10"),
				Costs: []CreditCost{
					{APIName: "api1", CostPerCall: MustNewCredit("1"), BatchSize: 5},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	now := time.Now()

	// Call 1: ceil(1/5)=1 batch → credit = 10 - 1 = 9
	allowed, _ := m.AllowN("api1", 1, now)
	if !allowed {
		t.Fatal("expected allowed=true on call 1")
	}
	credit, _ := m.store.GetCredit(context.Background(), "pool1")
	if credit.Cmp(MustNewCredit("9")) != 0 {
		t.Errorf("after call 1: expected credit=9, got %s", credit.String())
	}

	// Calls 2-5: still in batch 1, no new charge
	for i := 2; i <= 5; i++ {
		allowed, _ = m.AllowN("api1", 1, now)
		if !allowed {
			t.Fatalf("expected allowed=true on call %d", i)
		}
	}
	credit, _ = m.store.GetCredit(context.Background(), "pool1")
	if credit.Cmp(MustNewCredit("9")) != 0 {
		t.Errorf("after calls 2-5: expected credit=9, got %s", credit.String())
	}

	// Call 6: starts batch 2 → credit = 9 - 1 = 8
	allowed, _ = m.AllowN("api1", 1, now)
	if !allowed {
		t.Fatal("expected allowed=true on call 6")
	}
	credit, _ = m.store.GetCredit(context.Background(), "pool1")
	if credit.Cmp(MustNewCredit("8")) != 0 {
		t.Errorf("after call 6: expected credit=8, got %s", credit.String())
	}

	// Calls 7-10: still in batch 2, no new charge
	for i := 7; i <= 10; i++ {
		allowed, _ = m.AllowN("api1", 1, now)
		if !allowed {
			t.Fatalf("expected allowed=true on call %d", i)
		}
	}
	credit, _ = m.store.GetCredit(context.Background(), "pool1")
	if credit.Cmp(MustNewCredit("8")) != 0 {
		t.Errorf("after calls 7-10: expected credit=8, got %s", credit.String())
	}
}

func TestAllowN_BatchConsumption_MultipleAtOnce(t *testing.T) {
	// AllowN with n=7, BatchSize=3: ceil(7/3)=3 batches
	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "api1", Windows: []Window{{Duration: time.Minute, Limit: 100}}},
		},
		CreditPools: []CreditPoolConfig{
			{
				Name:       "pool1",
				MaxCredits: MustNewCredit("100"),
				Costs: []CreditCost{
					{APIName: "api1", CostPerCall: MustNewCredit("5"), BatchSize: 3},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	now := time.Now()

	// n=7, totalCallsBefore=0: ceil(0/3)=0, ceil(7/3)=3, newBatches=3
	// consume = 5 * 3 = 15
	allowed, _ := m.AllowN("api1", 7, now)
	if !allowed {
		t.Fatal("expected allowed=true")
	}
	credit, _ := m.store.GetCredit(context.Background(), "pool1")
	if credit.Cmp(MustNewCredit("85")) != 0 {
		t.Errorf("expected credit=85, got %s", credit.String())
	}
}

func TestAllowN_BatchConsumption_NoCounterUpdateOnFailure(t *testing.T) {
	// When credit deduction fails, batch counter should NOT be updated
	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "api1", Windows: []Window{{Duration: time.Minute, Limit: 100}}},
		},
		CreditPools: []CreditPoolConfig{
			{
				Name:       "pool1",
				MaxCredits: MustNewCredit("1"),
				Costs: []CreditCost{
					{APIName: "api1", CostPerCall: MustNewCredit("2"), BatchSize: 3},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	now := time.Now()

	// First call: consumes 2 credits but pool only has 1 → should fail
	allowed, _ := m.AllowN("api1", 1, now)
	if allowed {
		t.Error("expected allowed=false when credits insufficient")
	}

	// Batch counter should still be 0
	if m.batchCounters["api1"] != 0 {
		t.Errorf("expected batchCounter=0 after failure, got %d", m.batchCounters["api1"])
	}
}

// --- Property 11: バッチ消費の正確性 ---

// TestProperty11_BatchConsumptionAccuracy verifies that for any BatchSize B and CostPerCall C,
// after N requests the total credit consumption equals C × ceil(N / B).
// **Validates: Requirements 7.1, 7.2, 7.4**
func TestProperty11_BatchConsumptionAccuracy(t *testing.T) {
	cfg := &quick.Config{MaxCount: 200}

	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		// B: 1-10, N: 1-50, C: 1-10
		batchSize := int64(rng.Intn(10)) + 1
		numRequests := int64(rng.Intn(50)) + 1
		costVal := int64(rng.Intn(10)) + 1
		costPerCall := NewCreditFromInt(costVal)

		maxCredits := NewCreditFromInt(costVal * (numRequests + 1)) // enough credits

		m, err := NewBudgetManager(ManagerConfig{
			APIs: []RateConfig{
				{Name: "prop11_api", Windows: []Window{{Duration: time.Minute, Limit: numRequests + 100}}},
			},
			CreditPools: []CreditPoolConfig{
				{
					Name:       "prop11_pool",
					MaxCredits: maxCredits,
					Costs: []CreditCost{
						{APIName: "prop11_api", CostPerCall: costPerCall, BatchSize: batchSize},
					},
				},
			},
		})
		if err != nil {
			return false
		}

		now := time.Now()
		for i := int64(0); i < numRequests; i++ {
			allowed, _ := m.AllowN("prop11_api", 1, now)
			if !allowed {
				return false // should always be allowed with enough credits
			}
		}

		remaining, err := m.store.GetCredit(context.Background(), "prop11_pool")
		if err != nil {
			return false
		}

		// expected consumption = C × ceil(N / B)
		batches := numRequests / batchSize
		if numRequests%batchSize != 0 {
			batches++
		}
		expectedConsumed := costPerCall.Mul(batches)
		expectedRemaining := maxCredits.Sub(expectedConsumed)

		return remaining.Cmp(expectedRemaining) == 0
	}

	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("Property 11 failed: %v", err)
	}
}

// --- Property 12: クレジット消費の保存則 ---

// TestProperty12_CreditConservation verifies that consumed + remaining == initial.
// **Validates: Requirements 6.1, 6.2**
func TestProperty12_CreditConservation(t *testing.T) {
	cfg := &quick.Config{MaxCount: 200}

	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		// Random initial credits 10-100, N calls 1-20
		initialVal := int64(rng.Intn(91)) + 10
		numCalls := int64(rng.Intn(20)) + 1
		costVal := int64(1)

		initial := NewCreditFromInt(initialVal)

		m, err := NewBudgetManager(ManagerConfig{
			APIs: []RateConfig{
				{Name: "prop12_api", Windows: []Window{{Duration: time.Minute, Limit: numCalls + 100}}},
			},
			CreditPools: []CreditPoolConfig{
				{
					Name:       "prop12_pool",
					MaxCredits: initial,
					Costs: []CreditCost{
						{APIName: "prop12_api", CostPerCall: NewCreditFromInt(costVal), BatchSize: 1},
					},
				},
			},
		})
		if err != nil {
			return false
		}

		now := time.Now()
		successCount := int64(0)
		for i := int64(0); i < numCalls; i++ {
			allowed, _ := m.AllowN("prop12_api", 1, now)
			if allowed {
				successCount++
			}
		}

		remaining, err := m.store.GetCredit(context.Background(), "prop12_pool")
		if err != nil {
			return false
		}

		consumed := NewCreditFromInt(costVal).Mul(successCount)
		// consumed + remaining == initial
		return consumed.Add(remaining).Cmp(initial) == 0
	}

	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("Property 12 failed: %v", err)
	}
}

// --- Property 13: ResetCreditsの正確性 ---

// TestProperty13_ResetCreditsAccuracy verifies that after ResetCredits,
// GetCredits returns MaxCredits.
// **Validates: Requirements 6.3**
func TestProperty13_ResetCreditsAccuracy(t *testing.T) {
	cfg := &quick.Config{MaxCount: 200}

	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		maxVal := int64(rng.Intn(1000)) + 1
		maxCredits := NewCreditFromInt(maxVal)

		m, err := NewBudgetManager(ManagerConfig{
			APIs: []RateConfig{
				{Name: "prop13_api", Windows: []Window{{Duration: time.Minute, Limit: 1000}}},
			},
			CreditPools: []CreditPoolConfig{
				{
					Name:       "prop13_pool",
					MaxCredits: maxCredits,
					Costs: []CreditCost{
						{APIName: "prop13_api", CostPerCall: MustNewCredit("1"), BatchSize: 1},
					},
				},
			},
		})
		if err != nil {
			return false
		}

		now := time.Now()
		// Consume some credits
		consumeCount := rng.Intn(int(maxVal)) + 1
		for i := 0; i < consumeCount; i++ {
			m.AllowN("prop13_api", 1, now)
		}

		// Reset
		if resetErr := m.ResetCredits("prop13_pool"); resetErr != nil {
			return false
		}

		// Verify
		credits, err := m.GetCredits("prop13_pool")
		if err != nil {
			return false
		}

		return credits.Cmp(maxCredits) == 0
	}

	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("Property 13 failed: %v", err)
	}
}

// --- Property 14: クレジットプール初期値の適用 ---

// TestProperty14_CreditPoolInitialValue verifies that when Initial is specified,
// starting balance = Initial; when nil, starting balance = MaxCredits.
// **Validates: Requirements 6.6, 6.7**
func TestProperty14_CreditPoolInitialValue(t *testing.T) {
	cfg := &quick.Config{MaxCount: 200}

	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		maxVal := int64(rng.Intn(1000)) + 100
		maxCredits := NewCreditFromInt(maxVal)

		useInitial := rng.Intn(2) == 0

		var poolCfg CreditPoolConfig
		var expectedBalance Credit

		if useInitial {
			initialVal := int64(rng.Intn(int(maxVal))) + 1
			initial := NewCreditFromInt(initialVal)
			poolCfg = CreditPoolConfig{
				Name:       "prop14_pool",
				MaxCredits: maxCredits,
				Initial:    &initial,
				Costs: []CreditCost{
					{APIName: "prop14_api", CostPerCall: MustNewCredit("1"), BatchSize: 1},
				},
			}
			expectedBalance = initial
		} else {
			poolCfg = CreditPoolConfig{
				Name:       "prop14_pool",
				MaxCredits: maxCredits,
				Costs: []CreditCost{
					{APIName: "prop14_api", CostPerCall: MustNewCredit("1"), BatchSize: 1},
				},
			}
			expectedBalance = maxCredits
		}

		m, err := NewBudgetManager(ManagerConfig{
			APIs: []RateConfig{
				{Name: "prop14_api", Windows: []Window{{Duration: time.Minute, Limit: 1000}}},
			},
			CreditPools: []CreditPoolConfig{poolCfg},
		})
		if err != nil {
			return false
		}

		credits, err := m.GetCredits("prop14_pool")
		if err != nil {
			return false
		}

		return credits.Cmp(expectedBalance) == 0
	}

	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("Property 14 failed: %v", err)
	}
}

// --- Property 24: 共有クレジットプールの一貫性 ---

// TestProperty24_SharedCreditPoolConsistency verifies that multiple APIs sharing
// the same pool consume from the same balance.
// **Validates: Requirements 6.1**
func TestProperty24_SharedCreditPoolConsistency(t *testing.T) {
	cfg := &quick.Config{MaxCount: 200}

	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		maxVal := int64(rng.Intn(100)) + 20
		maxCredits := NewCreditFromInt(maxVal)

		// Two APIs sharing one pool, each with CostPerCall=1
		m, err := NewBudgetManager(ManagerConfig{
			APIs: []RateConfig{
				{Name: "prop24_api1", Windows: []Window{{Duration: time.Minute, Limit: 1000}}},
				{Name: "prop24_api2", Windows: []Window{{Duration: time.Minute, Limit: 1000}}},
			},
			CreditPools: []CreditPoolConfig{
				{
					Name:       "prop24_pool",
					MaxCredits: maxCredits,
					Costs: []CreditCost{
						{APIName: "prop24_api1", CostPerCall: MustNewCredit("1"), BatchSize: 1},
						{APIName: "prop24_api2", CostPerCall: MustNewCredit("1"), BatchSize: 1},
					},
				},
			},
		})
		if err != nil {
			return false
		}

		now := time.Now()

		// Random number of calls from each API
		callsApi1 := int64(rng.Intn(int(maxVal/2))) + 1
		callsApi2 := int64(rng.Intn(int(maxVal/2))) + 1

		successCount := int64(0)
		for i := int64(0); i < callsApi1; i++ {
			allowed, _ := m.AllowN("prop24_api1", 1, now)
			if allowed {
				successCount++
			}
		}
		for i := int64(0); i < callsApi2; i++ {
			allowed, _ := m.AllowN("prop24_api2", 1, now)
			if allowed {
				successCount++
			}
		}

		remaining, err := m.store.GetCredit(context.Background(), "prop24_pool")
		if err != nil {
			return false
		}

		// consumed + remaining == maxCredits
		consumed := NewCreditFromInt(successCount)
		return consumed.Add(remaining).Cmp(maxCredits) == 0
	}

	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("Property 24 failed: %v", err)
	}
}

// --- Unit Tests for GetCredits/ResetCredits/AddCredits/SetCredits ---

func TestGetCredits_Success(t *testing.T) {
	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "api1", Windows: []Window{{Duration: time.Minute, Limit: 10}}},
		},
		CreditPools: []CreditPoolConfig{
			{
				Name:       "pool1",
				MaxCredits: MustNewCredit("100"),
				Costs:      []CreditCost{{APIName: "api1"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	credits, err := m.GetCredits("pool1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if credits.Cmp(MustNewCredit("100")) != 0 {
		t.Errorf("expected 100, got %s", credits.String())
	}
}

func TestGetCredits_PoolNotFound(t *testing.T) {
	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "api1", Windows: []Window{{Duration: time.Minute, Limit: 10}}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = m.GetCredits("nonexistent")
	if err != ErrPoolNotFound {
		t.Errorf("expected ErrPoolNotFound, got %v", err)
	}
}

func TestResetCredits_Success(t *testing.T) {
	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "api1", Windows: []Window{{Duration: time.Minute, Limit: 100}}},
		},
		CreditPools: []CreditPoolConfig{
			{
				Name:       "pool1",
				MaxCredits: MustNewCredit("50"),
				Costs:      []CreditCost{{APIName: "api1", CostPerCall: MustNewCredit("10"), BatchSize: 1}},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Consume some credits
	now := time.Now()
	m.AllowN("api1", 1, now)
	m.AllowN("api1", 1, now)

	// Reset
	if resetErr := m.ResetCredits("pool1"); resetErr != nil {
		t.Fatalf("unexpected error: %v", resetErr)
	}

	credits, err := m.GetCredits("pool1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if credits.Cmp(MustNewCredit("50")) != 0 {
		t.Errorf("expected 50 after reset, got %s", credits.String())
	}
}

func TestResetCredits_PoolNotFound(t *testing.T) {
	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "api1", Windows: []Window{{Duration: time.Minute, Limit: 10}}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = m.ResetCredits("nonexistent")
	if err != ErrPoolNotFound {
		t.Errorf("expected ErrPoolNotFound, got %v", err)
	}
}

func TestAddCredits_Success(t *testing.T) {
	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "api1", Windows: []Window{{Duration: time.Minute, Limit: 100}}},
		},
		CreditPools: []CreditPoolConfig{
			{
				Name:       "pool1",
				MaxCredits: MustNewCredit("100"),
				Costs:      []CreditCost{{APIName: "api1", CostPerCall: MustNewCredit("10"), BatchSize: 1}},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Consume some credits
	now := time.Now()
	m.AllowN("api1", 1, now) // 100 - 10 = 90

	// Add credits
	if addErr := m.AddCredits("pool1", MustNewCredit("5")); addErr != nil {
		t.Fatalf("unexpected error: %v", addErr)
	}

	credits, err := m.GetCredits("pool1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if credits.Cmp(MustNewCredit("95")) != 0 {
		t.Errorf("expected 95, got %s", credits.String())
	}
}

func TestAddCredits_PoolNotFound(t *testing.T) {
	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "api1", Windows: []Window{{Duration: time.Minute, Limit: 10}}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = m.AddCredits("nonexistent", MustNewCredit("10"))
	if err != ErrPoolNotFound {
		t.Errorf("expected ErrPoolNotFound, got %v", err)
	}
}

func TestSetCredits_Success(t *testing.T) {
	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "api1", Windows: []Window{{Duration: time.Minute, Limit: 10}}},
		},
		CreditPools: []CreditPoolConfig{
			{
				Name:       "pool1",
				MaxCredits: MustNewCredit("100"),
				Costs:      []CreditCost{{APIName: "api1"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if setErr := m.SetCredits("pool1", MustNewCredit("42")); setErr != nil {
		t.Fatalf("unexpected error: %v", setErr)
	}

	credits, err := m.GetCredits("pool1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if credits.Cmp(MustNewCredit("42")) != 0 {
		t.Errorf("expected 42, got %s", credits.String())
	}
}

func TestSetCredits_PoolNotFound(t *testing.T) {
	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "api1", Windows: []Window{{Duration: time.Minute, Limit: 10}}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = m.SetCredits("nonexistent", MustNewCredit("10"))
	if err != ErrPoolNotFound {
		t.Errorf("expected ErrPoolNotFound, got %v", err)
	}
}

// --- Property 7: Wait関数のコンテキスト尊重 ---

// TestProperty7_WaitContextRespect verifies that when a context is canceled or
// deadline-exceeded, Wait returns an error and does not change counters or credits.
// **Validates: Requirements 4.3, 4.4**
func TestProperty7_WaitContextRespect(t *testing.T) {
	cfg := &quick.Config{MaxCount: 200}

	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		limit := int64(rng.Intn(5)) + 1
		creditVal := int64(rng.Intn(50)) + 10

		m, err := NewBudgetManager(ManagerConfig{
			APIs: []RateConfig{
				{Name: "prop7_api", Windows: []Window{{Duration: time.Minute, Limit: limit}}},
			},
			CreditPools: []CreditPoolConfig{
				{
					Name:       "prop7_pool",
					MaxCredits: NewCreditFromInt(creditVal),
					Costs: []CreditCost{
						{APIName: "prop7_api", CostPerCall: MustNewCredit("1"), BatchSize: 1},
					},
				},
			},
		})
		if err != nil {
			return false
		}

		now := time.Now()

		// Exhaust the rate limit so Wait would need to block
		for i := int64(0); i < limit; i++ {
			m.AllowN("prop7_api", 1, now)
		}

		// Capture state before Wait with canceled context
		key := windowKey("prop7_api", time.Minute, now)
		countBefore, _ := m.store.GetWindowCount(context.Background(), key)
		creditBefore, _ := m.store.GetCredit(context.Background(), "prop7_pool")

		// Use already-canceled context
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err = m.Wait(ctx, "prop7_api")
		if err == nil {
			return false // should have returned error
		}

		// Verify state unchanged
		countAfter, _ := m.store.GetWindowCount(context.Background(), key)
		creditAfter, _ := m.store.GetCredit(context.Background(), "prop7_pool")

		return countBefore == countAfter && creditBefore.Cmp(creditAfter) == 0
	}

	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("Property 7 failed: %v", err)
	}
}

// TestProperty7_WaitDeadlineExceeded verifies that deadline-exceeded context returns error.
// **Validates: Requirements 4.3, 4.4**
func TestProperty7_WaitDeadlineExceeded(t *testing.T) {
	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "dl_api", Windows: []Window{{Duration: time.Minute, Limit: 1}}},
		},
		CreditPools: []CreditPoolConfig{
			{
				Name:       "dl_pool",
				MaxCredits: MustNewCredit("100"),
				Costs: []CreditCost{
					{APIName: "dl_api", CostPerCall: MustNewCredit("1"), BatchSize: 1},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	now := time.Now()
	m.AllowN("dl_api", 1, now) // exhaust limit

	key := windowKey("dl_api", time.Minute, now)
	countBefore, _ := m.store.GetWindowCount(context.Background(), key)
	creditBefore, _ := m.store.GetCredit(context.Background(), "dl_pool")

	// Use deadline that's already passed
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	err = m.Wait(ctx, "dl_api")
	if err == nil {
		t.Fatal("expected error from Wait with expired deadline")
	}

	countAfter, _ := m.store.GetWindowCount(context.Background(), key)
	creditAfter, _ := m.store.GetCredit(context.Background(), "dl_pool")

	if countBefore != countAfter {
		t.Errorf("counter changed: before=%d, after=%d", countBefore, countAfter)
	}
	if creditBefore.Cmp(creditAfter) != 0 {
		t.Errorf("credit changed: before=%s, after=%s", creditBefore.String(), creditAfter.String())
	}
}

// --- Property 8: 予約のキャンセル整合性 ---

// TestProperty8_ReservationCancelConsistency verifies that after Cancel(),
// the state (counters + credits) is restored to the pre-Reserve state.
// **Validates: Requirements 5.4**
func TestProperty8_ReservationCancelConsistency(t *testing.T) {
	cfg := &quick.Config{MaxCount: 200}

	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		limit := int64(rng.Intn(20)) + 5
		creditVal := int64(rng.Intn(100)) + 20
		costVal := int64(rng.Intn(5)) + 1

		m, err := NewBudgetManager(ManagerConfig{
			APIs: []RateConfig{
				{Name: "prop8_api", Windows: []Window{{Duration: time.Minute, Limit: limit}}},
			},
			CreditPools: []CreditPoolConfig{
				{
					Name:       "prop8_pool",
					MaxCredits: NewCreditFromInt(creditVal),
					Costs: []CreditCost{
						{APIName: "prop8_api", CostPerCall: NewCreditFromInt(costVal), BatchSize: 1},
					},
				},
			},
		})
		if err != nil {
			return false
		}

		now := time.Now()

		// Do some initial calls to set up state
		initialCalls := int64(rng.Intn(3))
		for i := int64(0); i < initialCalls; i++ {
			m.AllowN("prop8_api", 1, now)
		}

		// Capture state before Reserve
		key := windowKey("prop8_api", time.Minute, now)
		countBefore, _ := m.store.GetWindowCount(context.Background(), key)
		creditBefore, _ := m.store.GetCredit(context.Background(), "prop8_pool")

		// Reserve
		n := int64(rng.Intn(3)) + 1
		r := m.ReserveN("prop8_api", n, now)
		if !r.OK() {
			// Reservation failed, state should be unchanged already
			return true
		}

		// Cancel
		r.Cancel()

		// Verify state restored
		countAfter, _ := m.store.GetWindowCount(context.Background(), key)
		creditAfter, _ := m.store.GetCredit(context.Background(), "prop8_pool")

		return countBefore == countAfter && creditBefore.Cmp(creditAfter) == 0
	}

	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("Property 8 failed: %v", err)
	}
}

// --- Property 9: Confirm差分調整の正確性 ---

// TestProperty9_ConfirmDiffAccuracy verifies that the credit change from Confirm
// is exactly (actualCost - reservedCost), with no double consumption.
// **Validates: Requirements 5.5, 5.6, 5.7, 5.11**
func TestProperty9_ConfirmDiffAccuracy(t *testing.T) {
	cfg := &quick.Config{MaxCount: 200}

	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		creditVal := int64(rng.Intn(200)) + 50
		costVal := int64(rng.Intn(5)) + 1

		m, err := NewBudgetManager(ManagerConfig{
			APIs: []RateConfig{
				{Name: "prop9_api", Windows: []Window{{Duration: time.Minute, Limit: 100}}},
			},
			CreditPools: []CreditPoolConfig{
				{
					Name:       "prop9_pool",
					MaxCredits: NewCreditFromInt(creditVal),
					Costs: []CreditCost{
						{APIName: "prop9_api", CostPerCall: NewCreditFromInt(costVal), BatchSize: 1},
					},
				},
			},
		})
		if err != nil {
			return false
		}

		now := time.Now()

		// Reserve 1 call
		r := m.ReserveN("prop9_api", 1, now)
		if !r.OK() {
			return true // skip if reservation failed
		}

		// Capture credit after reserve (reservedCost already deducted)
		creditAfterReserve, _ := m.store.GetCredit(context.Background(), "prop9_pool")

		// Generate random actual cost (0 to 2*costVal)
		actualVal := int64(rng.Intn(int(costVal)*2 + 1))
		actualCost := NewCreditFromInt(actualVal)
		reservedCost := NewCreditFromInt(costVal)

		_ = r.Confirm(actualCost)

		creditAfterConfirm, _ := m.store.GetCredit(context.Background(), "prop9_pool")

		// Expected: creditAfterConfirm = creditAfterReserve - (actualCost - reservedCost)
		diff := actualCost.Sub(reservedCost)
		expectedCredit := creditAfterReserve.Sub(diff)

		return creditAfterConfirm.Cmp(expectedCredit) == 0
	}

	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("Property 9 failed: %v", err)
	}
}

// --- Property 10: Reservation状態遷移の排他性 ---

// TestProperty10_ReservationStateExclusivity verifies that double Confirm returns
// ErrReservationAlreadyFinalized, and Cancel after Confirm is no-op.
// **Validates: Requirements 5.9, 5.10**
func TestProperty10_ReservationStateExclusivity(t *testing.T) {
	cfg := &quick.Config{MaxCount: 200}

	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		m, err := NewBudgetManager(ManagerConfig{
			APIs: []RateConfig{
				{Name: "prop10_api", Windows: []Window{{Duration: time.Minute, Limit: 100}}},
			},
			CreditPools: []CreditPoolConfig{
				{
					Name:       "prop10_pool",
					MaxCredits: MustNewCredit("1000"),
					Costs: []CreditCost{
						{APIName: "prop10_api", CostPerCall: MustNewCredit("1"), BatchSize: 1},
					},
				},
			},
		})
		if err != nil {
			return false
		}

		now := time.Now()
		r := m.ReserveN("prop10_api", 1, now)
		if !r.OK() {
			return true
		}

		// Choose scenario: confirm-then-confirm, confirm-then-cancel, cancel-then-confirm
		scenario := rng.Intn(3)

		switch scenario {
		case 0:
			// Double Confirm
			err1 := r.Confirm(MustNewCredit("1"))
			if err1 != nil {
				return false
			}
			err2 := r.Confirm(MustNewCredit("1"))
			return err2 == ErrReservationAlreadyFinalized

		case 1:
			// Confirm then Cancel (Cancel should be no-op)
			err1 := r.Confirm(MustNewCredit("1"))
			if err1 != nil {
				return false
			}
			creditBefore, _ := m.store.GetCredit(context.Background(), "prop10_pool")
			r.Cancel() // should be no-op
			creditAfter, _ := m.store.GetCredit(context.Background(), "prop10_pool")
			return creditBefore.Cmp(creditAfter) == 0

		case 2:
			// Cancel then Confirm
			r.Cancel()
			err1 := r.Confirm(MustNewCredit("1"))
			return err1 == ErrReservationAlreadyFinalized
		}

		return true
	}

	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("Property 10 failed: %v", err)
	}
}

// --- Property 22: Reserve関数の非nil保証 ---

// TestProperty22_ReserveNonNilGuarantee verifies that Reserve always returns
// a non-nil Reservation for any API name (registered or not).
// **Validates: Requirements 5.1**
func TestProperty22_ReserveNonNilGuarantee(t *testing.T) {
	cfg := &quick.Config{MaxCount: 200}

	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "prop22_api", Windows: []Window{{Duration: time.Minute, Limit: 10}}},
		},
		CreditPools: []CreditPoolConfig{
			{
				Name:       "prop22_pool",
				MaxCredits: MustNewCredit("100"),
				Costs: []CreditCost{
					{APIName: "prop22_api", CostPerCall: MustNewCredit("1"), BatchSize: 1},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		// Generate random API name (may or may not be registered)
		names := []string{"prop22_api", "unknown_api", "", "random_" + string(rune('a'+rng.Intn(26)))}
		name := names[rng.Intn(len(names))]

		r := m.Reserve(name)
		return r != nil
	}

	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("Property 22 failed: %v", err)
	}
}

// --- Property 20: 動的リミット変更の即時反映 ---

// TestProperty20_DynamicLimitChangeImmediate verifies that after SetLimit,
// Allow/Reserve use the new limit immediately.
// **Validates: Requirements 18.1, 18.2**
func TestProperty20_DynamicLimitChangeImmediate(t *testing.T) {
	cfg := &quick.Config{MaxCount: 200}

	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		// Initial limit 1-20, new limit 1-50
		initialLimit := int64(rng.Intn(20)) + 1
		newLimit := int64(rng.Intn(50)) + 1

		m, err := NewBudgetManager(ManagerConfig{
			APIs: []RateConfig{
				{Name: "prop20_api", Windows: []Window{{Duration: time.Minute, Limit: initialLimit}}},
			},
		})
		if err != nil {
			return false
		}

		now := time.Now()

		// Exhaust the initial limit
		for i := int64(0); i < initialLimit; i++ {
			allowed, _ := m.AllowN("prop20_api", 1, now)
			if !allowed {
				return false // should be allowed within initial limit
			}
		}

		// Should be rejected at initial limit
		allowed, _ := m.AllowN("prop20_api", 1, now)
		if allowed {
			return false // should be rejected
		}

		// Change limit to newLimit
		if err := m.SetLimit("prop20_api", time.Minute, newLimit); err != nil {
			return false
		}

		// If newLimit > initialLimit, additional calls should be allowed
		if newLimit > initialLimit {
			allowed, _ = m.AllowN("prop20_api", 1, now)
			if !allowed {
				return false // should be allowed with increased limit
			}
		}

		// If newLimit <= initialLimit, calls should still be rejected
		if newLimit <= initialLimit {
			allowed, _ = m.AllowN("prop20_api", 1, now)
			if allowed {
				return false // should still be rejected
			}
		}

		return true
	}

	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("Property 20 failed: %v", err)
	}
}

// TestSetLimit_UnregisteredAPI verifies SetLimit returns ErrAPINotFound for unknown APIs.
func TestSetLimit_UnregisteredAPI(t *testing.T) {
	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "api1", Windows: []Window{{Duration: time.Minute, Limit: 10}}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = m.SetLimit("nonexistent", time.Minute, 20)
	if err != ErrAPINotFound {
		t.Errorf("expected ErrAPINotFound, got %v", err)
	}
}

// TestSetLimit_WindowNotFound verifies SetLimit returns error for unknown window duration.
func TestSetLimit_WindowNotFound(t *testing.T) {
	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "api1", Windows: []Window{{Duration: time.Minute, Limit: 10}}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = m.SetLimit("api1", time.Hour, 20)
	if err == nil {
		t.Error("expected error for unknown window duration")
	}
}

// TestTokens_UnregisteredAPI verifies Tokens returns 0 for unknown APIs.
func TestTokens_UnregisteredAPI(t *testing.T) {
	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "api1", Windows: []Window{{Duration: time.Minute, Limit: 10}}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tokens := m.Tokens("nonexistent")
	if tokens != 0 {
		t.Errorf("expected 0 tokens for unregistered API, got %f", tokens)
	}
}

// --- Property 21: Tokens関数の最小窓基準 ---

// TestProperty21_TokensMinWindow verifies that for APIs with multiple windows,
// Tokens returns the most restrictive (minimum available) value.
// **Validates: Requirements 18.3**
func TestProperty21_TokensMinWindow(t *testing.T) {
	cfg := &quick.Config{MaxCount: 200}

	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		// Two windows with different limits
		limit1 := int64(rng.Intn(50)) + 10
		limit2 := int64(rng.Intn(50)) + 10

		m, err := NewBudgetManager(ManagerConfig{
			APIs: []RateConfig{
				{
					Name: "prop21_api",
					Windows: []Window{
						{Duration: time.Second, Limit: limit1},
						{Duration: time.Minute, Limit: limit2},
					},
				},
			},
		})
		if err != nil {
			return false
		}

		now := time.Now()

		// Consume some requests (within the smaller limit)
		minLimit := limit1
		if limit2 < minLimit {
			minLimit = limit2
		}
		consumeCount := int64(0)
		if minLimit > 1 {
			consumeCount = int64(rng.Intn(int(minLimit-1))) + 1
		} else {
			consumeCount = 1
		}

		for i := int64(0); i < consumeCount; i++ {
			allowed, _ := m.AllowN("prop21_api", 1, now)
			if !allowed {
				// Hit a limit, stop consuming
				consumeCount = i
				break
			}
		}

		tokens := m.Tokens("prop21_api")

		// Tokens should be the minimum of (limit1 - consumed, limit2 - consumed)
		avail1 := float64(limit1 - consumeCount)
		avail2 := float64(limit2 - consumeCount)
		expectedMin := avail1
		if avail2 < expectedMin {
			expectedMin = avail2
		}

		return tokens == expectedMin
	}

	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("Property 21 failed: %v", err)
	}
}

// --- Property 15: デフォルト値の等価性 ---

// TestProperty15_DefaultValueEquivalence verifies that config with defaults omitted
// behaves identically to config with defaults explicitly set.
// **Validates: Requirements 11.1, 11.2, 11.3, 11.4, 11.5, 11.6, 11.7, 11.8**
func TestProperty15_DefaultValueEquivalence(t *testing.T) {
	cfg := &quick.Config{MaxCount: 200}

	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		limit := int64(rng.Intn(20)) + 5
		maxCredits := int64(rng.Intn(100)) + 20
		numCalls := int64(rng.Intn(int(limit))) + 1

		// Config with defaults omitted
		mDefaults, err := NewBudgetManager(ManagerConfig{
			APIs: []RateConfig{
				{
					Name:    "prop15_api",
					Windows: []Window{{Duration: time.Minute, Limit: limit}},
					// Buffer omitted → default 0
				},
			},
			// Store omitted → default MemoryStore
			// Logger omitted → default slog-based logger
			// LogLevel omitted → default LogLevelInfo
			CreditPools: []CreditPoolConfig{
				{
					Name:       "prop15_pool",
					MaxCredits: NewCreditFromInt(maxCredits),
					// Initial omitted → default MaxCredits
					Costs: []CreditCost{
						{
							APIName: "prop15_api",
							// CostPerCall omitted → default "1"
							// BatchSize omitted → default 1
						},
					},
				},
			},
		})
		if err != nil {
			return false
		}

		// Config with defaults explicitly set
		initialCredit := NewCreditFromInt(maxCredits)
		mExplicit, err := NewBudgetManager(ManagerConfig{
			APIs: []RateConfig{
				{
					Name:    "prop15_api",
					Windows: []Window{{Duration: time.Minute, Limit: limit}},
					Buffer:  0, // explicit default
				},
			},
			Store:    NewMemoryStore(), // explicit default
			LogLevel: LogLevelInfo,     // explicit default
			CreditPools: []CreditPoolConfig{
				{
					Name:       "prop15_pool",
					MaxCredits: NewCreditFromInt(maxCredits),
					Initial:    &initialCredit, // explicit default = MaxCredits
					Costs: []CreditCost{
						{
							APIName:     "prop15_api",
							CostPerCall: MustNewCredit("1"), // explicit default
							BatchSize:   1,                  // explicit default
						},
					},
				},
			},
		})
		if err != nil {
			return false
		}

		now := time.Now()

		// Both managers should behave identically for the same sequence of calls
		for i := int64(0); i < numCalls; i++ {
			allowedD, _ := mDefaults.AllowN("prop15_api", 1, now)
			allowedE, _ := mExplicit.AllowN("prop15_api", 1, now)
			if allowedD != allowedE {
				return false
			}
		}

		// Check credit balances are equal
		creditsD, errD := mDefaults.GetCredits("prop15_pool")
		creditsE, errE := mExplicit.GetCredits("prop15_pool")
		if errD != nil || errE != nil {
			return false
		}
		if creditsD.Cmp(creditsE) != 0 {
			return false
		}

		// Check tokens are equal
		tokensD := mDefaults.Tokens("prop15_api")
		tokensE := mExplicit.Tokens("prop15_api")
		return tokensD == tokensE
	}

	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("Property 15 failed: %v", err)
	}
}
