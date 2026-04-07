package apibudget

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestConcurrency_AllowSimultaneous launches N goroutines all calling Allow on the same API
// simultaneously and verifies total successful calls don't exceed the limit.
// **Validates: Requirements 16.1, 16.2**
func TestConcurrency_AllowSimultaneous(t *testing.T) {
	const limit int64 = 50
	const goroutines = 100

	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "conc_api", Windows: []Window{{Duration: time.Minute, Limit: limit}}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var wg sync.WaitGroup
	var successCount atomic.Int64

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			allowed, _ := m.Allow("conc_api")
			if allowed {
				successCount.Add(1)
			}
		}()
	}
	wg.Wait()

	got := successCount.Load()
	if got > limit {
		t.Errorf("total successful Allow calls (%d) exceeded limit (%d)", got, limit)
	}
	if got == 0 {
		t.Error("expected at least some successful Allow calls, got 0")
	}
}

// TestConcurrency_AllowNSimultaneous tests multiple goroutines calling AllowN with n>1.
// **Validates: Requirements 16.1, 16.2**
func TestConcurrency_AllowNSimultaneous(t *testing.T) {
	const limit int64 = 100
	const goroutines = 50
	const requestsPerGoroutine int64 = 3

	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "conc_api_n", Windows: []Window{{Duration: time.Minute, Limit: limit}}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var wg sync.WaitGroup
	var totalGranted atomic.Int64

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			allowed, _ := m.AllowN("conc_api_n", requestsPerGoroutine, time.Now())
			if allowed {
				totalGranted.Add(requestsPerGoroutine)
			}
		}()
	}
	wg.Wait()

	got := totalGranted.Load()
	if got > limit {
		t.Errorf("total granted requests (%d) exceeded limit (%d)", got, limit)
	}
}

// TestConcurrency_AllowWithCredits tests concurrent Allow calls with credit pool.
// **Validates: Requirements 16.1, 16.2**
func TestConcurrency_AllowWithCredits(t *testing.T) {
	const limit int64 = 200
	const credits int64 = 30
	const goroutines = 100

	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "conc_credit_api", Windows: []Window{{Duration: time.Minute, Limit: limit}}},
		},
		CreditPools: []CreditPoolConfig{
			{
				Name:       "conc_pool",
				MaxCredits: NewCreditFromInt(credits),
				Costs: []CreditCost{
					{APIName: "conc_credit_api", CostPerCall: MustNewCredit("1"), BatchSize: 1},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var wg sync.WaitGroup
	var successCount atomic.Int64

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			allowed, _ := m.Allow("conc_credit_api")
			if allowed {
				successCount.Add(1)
			}
		}()
	}
	wg.Wait()

	got := successCount.Load()
	if got > credits {
		t.Errorf("total successful calls (%d) exceeded credit limit (%d)", got, credits)
	}
	if got == 0 {
		t.Error("expected at least some successful calls, got 0")
	}
}

// TestConcurrency_WaitSimultaneous tests multiple goroutines calling Wait concurrently.
// **Validates: Requirements 16.1**
func TestConcurrency_WaitSimultaneous(t *testing.T) {
	const limit int64 = 20
	const goroutines = 20

	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "conc_wait_api", Windows: []Window{{Duration: time.Minute, Limit: limit}}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var wg sync.WaitGroup
	var successCount atomic.Int64
	var errCount atomic.Int64

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			err := m.Wait(ctx, "conc_wait_api")
			if err == nil {
				successCount.Add(1)
			} else {
				errCount.Add(1)
			}
		}()
	}
	wg.Wait()

	got := successCount.Load()
	if got > limit {
		t.Errorf("total successful Wait calls (%d) exceeded limit (%d)", got, limit)
	}
}

// TestConcurrency_ReserveSimultaneous tests multiple goroutines calling Reserve concurrently.
// **Validates: Requirements 16.1**
func TestConcurrency_ReserveSimultaneous(t *testing.T) {
	const limit int64 = 50
	const goroutines = 100

	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "conc_reserve_api", Windows: []Window{{Duration: time.Minute, Limit: limit}}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var wg sync.WaitGroup
	var okCount atomic.Int64

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			r := m.Reserve("conc_reserve_api")
			if r == nil {
				t.Error("Reserve returned nil")
				return
			}
			if r.OK() {
				okCount.Add(1)
			}
		}()
	}
	wg.Wait()

	got := okCount.Load()
	if got > limit {
		t.Errorf("total successful Reserve calls (%d) exceeded limit (%d)", got, limit)
	}
}

// TestConcurrency_ReservationConfirmCancel tests concurrent Confirm and Cancel on the same reservation.
// Only one of Confirm/Cancel should succeed; the other should be a no-op or return ErrReservationAlreadyFinalized.
// **Validates: Requirements 16.4**
func TestConcurrency_ReservationConfirmCancel(t *testing.T) {
	const iterations = 100

	for iter := 0; iter < iterations; iter++ {
		m, err := NewBudgetManager(ManagerConfig{
			APIs: []RateConfig{
				{Name: "conc_rc_api", Windows: []Window{{Duration: time.Minute, Limit: 1000}}},
			},
			CreditPools: []CreditPoolConfig{
				{
					Name:       "conc_rc_pool",
					MaxCredits: MustNewCredit("10000"),
					Costs: []CreditCost{
						{APIName: "conc_rc_api", CostPerCall: MustNewCredit("1"), BatchSize: 1},
					},
				},
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		r := m.Reserve("conc_rc_api")
		if !r.OK() {
			t.Fatal("expected reservation to be OK")
		}

		var wg sync.WaitGroup
		var confirmErr atomic.Value

		wg.Add(2)

		// Goroutine 1: Confirm
		go func() {
			defer wg.Done()
			cerr := r.Confirm(MustNewCredit("1"))
			if cerr != nil {
				confirmErr.Store(cerr)
			}
		}()

		// Goroutine 2: Cancel
		go func() {
			defer wg.Done()
			r.Cancel()
		}()

		wg.Wait()

		// After both complete, the reservation should be finalized
		// A second Confirm should always return ErrReservationAlreadyFinalized
		err = r.Confirm(MustNewCredit("1"))
		if err != ErrReservationAlreadyFinalized {
			t.Errorf("iteration %d: expected ErrReservationAlreadyFinalized on second Confirm, got %v", iter, err)
		}
	}
}

// TestConcurrency_ReservationDoubleConfirm tests concurrent double Confirm calls.
// **Validates: Requirements 16.4**
func TestConcurrency_ReservationDoubleConfirm(t *testing.T) {
	const iterations = 100

	for iter := 0; iter < iterations; iter++ {
		m, err := NewBudgetManager(ManagerConfig{
			APIs: []RateConfig{
				{Name: "conc_dc_api", Windows: []Window{{Duration: time.Minute, Limit: 1000}}},
			},
			CreditPools: []CreditPoolConfig{
				{
					Name:       "conc_dc_pool",
					MaxCredits: MustNewCredit("10000"),
					Costs: []CreditCost{
						{APIName: "conc_dc_api", CostPerCall: MustNewCredit("1"), BatchSize: 1},
					},
				},
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		r := m.Reserve("conc_dc_api")
		if !r.OK() {
			t.Fatal("expected reservation to be OK")
		}

		var wg sync.WaitGroup
		var errCount atomic.Int64

		wg.Add(2)

		// Two goroutines both try to Confirm
		for i := 0; i < 2; i++ {
			go func() {
				defer wg.Done()
				err := r.Confirm(MustNewCredit("1"))
				if err == ErrReservationAlreadyFinalized {
					errCount.Add(1)
				}
			}()
		}
		wg.Wait()

		// Exactly one should have gotten ErrReservationAlreadyFinalized
		if errCount.Load() != 1 {
			t.Errorf("iteration %d: expected exactly 1 ErrReservationAlreadyFinalized, got %d", iter, errCount.Load())
		}
	}
}

// TestConcurrency_MixedOperations tests a mix of Allow, Reserve, and Wait calls concurrently.
// **Validates: Requirements 16.1, 16.2, 16.3, 16.4**
func TestConcurrency_MixedOperations(t *testing.T) {
	const limit int64 = 200
	const goroutines = 60 // 20 Allow + 20 Reserve + 20 Wait

	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "conc_mixed_api", Windows: []Window{{Duration: time.Minute, Limit: limit}}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(goroutines)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 20 goroutines calling Allow
	for i := 0; i < 20; i++ {
		go func() {
			defer wg.Done()
			m.Allow("conc_mixed_api")
		}()
	}

	// 20 goroutines calling Reserve
	for i := 0; i < 20; i++ {
		go func() {
			defer wg.Done()
			r := m.Reserve("conc_mixed_api")
			if r == nil {
				t.Error("Reserve returned nil")
			}
		}()
	}

	// 20 goroutines calling Wait
	for i := 0; i < 20; i++ {
		go func() {
			defer wg.Done()
			_ = m.Wait(ctx, "conc_mixed_api")
		}()
	}

	wg.Wait()
	// If we get here without a race detector complaint or panic, the test passes
}

// TestConcurrency_BatchCounterSafety tests that batchCounters are safely accessed
// under concurrent Allow calls with BatchSize > 1.
// **Validates: Requirements 16.1, 16.2**
func TestConcurrency_BatchCounterSafety(t *testing.T) {
	const goroutines = 100

	m, err := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "conc_batch_api", Windows: []Window{{Duration: time.Minute, Limit: 1000}}},
		},
		CreditPools: []CreditPoolConfig{
			{
				Name:       "conc_batch_pool",
				MaxCredits: MustNewCredit("10000"),
				Costs: []CreditCost{
					{APIName: "conc_batch_api", CostPerCall: MustNewCredit("1"), BatchSize: 5},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var wg sync.WaitGroup
	var successCount atomic.Int64

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			allowed, _ := m.Allow("conc_batch_api")
			if allowed {
				successCount.Add(1)
			}
		}()
	}
	wg.Wait()

	got := successCount.Load()
	if got == 0 {
		t.Error("expected at least some successful calls, got 0")
	}
	// With BatchSize=5 and 10000 credits, all 100 should succeed (20 batches = 20 credits)
	if got != int64(goroutines) {
		t.Errorf("expected all %d calls to succeed, got %d", goroutines, got)
	}
}
