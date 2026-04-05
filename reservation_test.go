package apibudget

import (
	"context"
	"testing"
	"time"
)

func TestReservation_OKAndDelay(t *testing.T) {
	r := &Reservation{
		ok:    true,
		delay: time.Second,
	}

	if !r.OK() {
		t.Error("expected OK to be true")
	}

	if r.Delay() != time.Second {
		t.Errorf("expected Delay %v, got %v", time.Second, r.Delay())
	}

	r.Cancel()
	if r.OK() {
		t.Error("expected OK to be false after cancellation")
	}
}

func TestReservation_Cancel(t *testing.T) {
	mgr, _ := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "api1", Windows: []Window{{Duration: time.Minute, Limit: 10}}},
		},
		CreditPools: []CreditPoolConfig{
			{
				Name:       "pool1",
				MaxCredits: MustNewCredit("100"),
				Costs:      []CreditCost{{APIName: "api1", CostPerCall: MustNewCredit("2")}},
			},
		},
	})

	r := mgr.Reserve("api1")
	if !r.OK() {
		t.Fatal("expected reservation to be OK")
	}

	// Verify credits were deducted
	c, _ := mgr.GetCredits("pool1")
	if c.String() != "98" {
		t.Errorf("expected 98 credits, got %s", c.String())
	}

	r.Cancel()

	// Verify credits were restored
	c, _ = mgr.GetCredits("pool1")
	if c.String() != "100" {
		t.Errorf("expected 100 credits, got %s", c.String())
	}

	// Verify window count was decremented
	ctx := context.Background()
	count, _ := mgr.store.GetWindowCount(ctx, r.windowKeys[0].key)
	if count != 0 {
		t.Errorf("expected count 0, got %d", count)
	}

	// Double cancel should be no-op
	r.Cancel()
}

func TestReservation_Confirm_NoPool(t *testing.T) {
	mgr, _ := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "api1", Windows: []Window{{Duration: time.Minute, Limit: 10}}},
		},
	})

	r := mgr.Reserve("api1")
	if !r.OK() {
		t.Fatal("expected reservation to be OK")
	}

	err := r.Confirm(MustNewCredit("5"))
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}

	err = r.Confirm(MustNewCredit("5"))
	if err != ErrReservationAlreadyFinalized {
		t.Errorf("expected ErrReservationAlreadyFinalized, got %v", err)
	}
}

func TestReservation_Confirm_WithPool(t *testing.T) {
	mgr, _ := NewBudgetManager(ManagerConfig{
		APIs: []RateConfig{
			{Name: "api1", Windows: []Window{{Duration: time.Minute, Limit: 10}}},
		},
		CreditPools: []CreditPoolConfig{
			{
				Name:       "pool1",
				MaxCredits: MustNewCredit("100"),
				Costs:      []CreditCost{{APIName: "api1", CostPerCall: MustNewCredit("10")}},
			},
		},
	})

	t.Run("Refund", func(t *testing.T) {
		_ = mgr.ResetCredits("pool1")
		r := mgr.Reserve("api1")
		err := r.Confirm(MustNewCredit("5"))
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
		c, _ := mgr.GetCredits("pool1")
		if c.String() != "95" {
			t.Errorf("expected 95 credits, got %s", c.String())
		}
	})

	t.Run("ExtraCost", func(t *testing.T) {
		_ = mgr.ResetCredits("pool1")
		r := mgr.Reserve("api1")
		err := r.Confirm(MustNewCredit("15"))
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
		c, _ := mgr.GetCredits("pool1")
		if c.String() != "85" {
			t.Errorf("expected 85 credits, got %s", c.String())
		}
	})

	t.Run("ExactCost", func(t *testing.T) {
		_ = mgr.ResetCredits("pool1")
		r := mgr.Reserve("api1")
		err := r.Confirm(MustNewCredit("10"))
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
		c, _ := mgr.GetCredits("pool1")
		if c.String() != "90" {
			t.Errorf("expected 90 credits, got %s", c.String())
		}
	})

	t.Run("InsufficientFunds", func(t *testing.T) {
		_ = mgr.ResetCredits("pool1")
		// Make it almost empty
		_ = mgr.SetCredits("pool1", MustNewCredit("10"))

		r := mgr.Reserve("api1") // Consumes 10. Balance: 0

		err := r.Confirm(MustNewCredit("20")) // diff is 10. Deducting 10 from 0 fails.
		if err != ErrInsufficientCredits {
			t.Errorf("expected ErrInsufficientCredits, got %v", err)
		}
		c, _ := mgr.GetCredits("pool1")
		if c.String() != "-10" {
			t.Errorf("expected -10 credits, got %s", c.String())
		}
	})
}
