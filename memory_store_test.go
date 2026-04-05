package apibudget

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"testing"
	"testing/quick"
	"time"
)

// TestDeductCreditAtomicity verifies Property 18: DeductCreditの原子性
// For any deduction request that exceeds the balance, DeductCredit returns an error
// and does NOT change the balance.
//
// **Validates: Requirements 8.5**
func TestDeductCreditAtomicity(t *testing.T) {
	f := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))

		// Generate a random initial balance in range [1, 10000)
		initialNum := r.Int63n(9999) + 1
		initialDen := r.Int63n(99) + 1
		initialStr := fmt.Sprintf("%d/%d", initialNum, initialDen)

		initial, err := NewCredit(initialStr)
		if err != nil {
			t.Logf("failed to create initial credit from %q: %v", initialStr, err)
			return false
		}

		// Generate a deduction amount that strictly exceeds the balance.
		// We add a positive extra amount to the initial balance to guarantee deduction > balance.
		extraNum := r.Int63n(9999) + 1
		extraDen := r.Int63n(99) + 1
		extraStr := fmt.Sprintf("%d/%d", extraNum, extraDen)
		extra, err := NewCredit(extraStr)
		if err != nil {
			t.Logf("failed to create extra credit from %q: %v", extraStr, err)
			return false
		}
		deductAmount := initial.Add(extra) // deductAmount > initial (since extra > 0)

		ctx := context.Background()
		store := NewMemoryStore()
		poolKey := "test-pool"

		// Set initial balance
		if err := store.SetCredit(ctx, poolKey, initial); err != nil {
			t.Logf("SetCredit failed: %v", err)
			return false
		}

		// Attempt deduction that exceeds balance
		_, deductErr := store.DeductCredit(ctx, poolKey, deductAmount)

		// Must return ErrInsufficientCredits
		if !errors.Is(deductErr, ErrInsufficientCredits) {
			t.Logf("expected ErrInsufficientCredits, got: %v (initial=%s, deduct=%s)",
				deductErr, initial.String(), deductAmount.String())
			return false
		}

		// Balance must remain unchanged
		remaining, err := store.GetCredit(ctx, poolKey)
		if err != nil {
			t.Logf("GetCredit failed: %v", err)
			return false
		}

		if remaining.Cmp(initial) != 0 {
			t.Logf("balance changed after failed deduction: initial=%s, remaining=%s, deductAmount=%s",
				initial.String(), remaining.String(), deductAmount.String())
			return false
		}

		return true
	}

	cfg := &quick.Config{
		MaxCount: 1000,
	}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("Property 18 (DeductCredit atomicity) failed: %v", err)
	}
}

// ============================================================
// Unit Tests for MemoryStore
// ============================================================

// --- IncrementWindow / DecrementWindow / GetWindowCount ---

func TestIncrementWindow_Basic(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	// First increment creates the entry
	count, err := store.IncrementWindow(ctx, "key1", 1, 10*time.Second)
	if err != nil {
		t.Fatalf("IncrementWindow failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected count=1, got %d", count)
	}

	// Second increment adds to existing entry
	count, err = store.IncrementWindow(ctx, "key1", 3, 10*time.Second)
	if err != nil {
		t.Fatalf("IncrementWindow failed: %v", err)
	}
	if count != 4 {
		t.Fatalf("expected count=4, got %d", count)
	}
}

func TestGetWindowCount_Basic(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	// Non-existent key returns 0
	count, err := store.GetWindowCount(ctx, "missing")
	if err != nil {
		t.Fatalf("GetWindowCount failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected count=0 for missing key, got %d", count)
	}

	// After increment, returns the count
	store.IncrementWindow(ctx, "key1", 5, 10*time.Second)
	count, err = store.GetWindowCount(ctx, "key1")
	if err != nil {
		t.Fatalf("GetWindowCount failed: %v", err)
	}
	if count != 5 {
		t.Fatalf("expected count=5, got %d", count)
	}
}

func TestDecrementWindow_Basic(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	// Decrement on non-existent key is a no-op
	if err := store.DecrementWindow(ctx, "missing", 1); err != nil {
		t.Fatalf("DecrementWindow on missing key failed: %v", err)
	}

	// Increment then decrement
	store.IncrementWindow(ctx, "key1", 10, 10*time.Second)
	if err := store.DecrementWindow(ctx, "key1", 3); err != nil {
		t.Fatalf("DecrementWindow failed: %v", err)
	}
	count, _ := store.GetWindowCount(ctx, "key1")
	if count != 7 {
		t.Fatalf("expected count=7 after decrement, got %d", count)
	}
}

func TestDecrementWindow_FloorAtZero(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	store.IncrementWindow(ctx, "key1", 2, 10*time.Second)
	// Decrement more than current count should floor at 0
	if err := store.DecrementWindow(ctx, "key1", 5); err != nil {
		t.Fatalf("DecrementWindow failed: %v", err)
	}
	count, _ := store.GetWindowCount(ctx, "key1")
	if count != 0 {
		t.Fatalf("expected count=0 (floored), got %d", count)
	}
}

func TestWindowCounter_ResetOnExpiry(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	// Use a very short window
	store.IncrementWindow(ctx, "key1", 5, 50*time.Millisecond)
	count, _ := store.GetWindowCount(ctx, "key1")
	if count != 5 {
		t.Fatalf("expected count=5 before expiry, got %d", count)
	}

	// Wait for the window to expire
	time.Sleep(60 * time.Millisecond)

	// GetWindowCount should return 0 after expiry
	count, _ = store.GetWindowCount(ctx, "key1")
	if count != 0 {
		t.Fatalf("expected count=0 after expiry, got %d", count)
	}

	// IncrementWindow should reset and start fresh
	count, _ = store.IncrementWindow(ctx, "key1", 2, 50*time.Millisecond)
	if count != 2 {
		t.Fatalf("expected count=2 after expired re-increment, got %d", count)
	}
}

func TestDecrementWindow_NoOpAfterExpiry(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	store.IncrementWindow(ctx, "key1", 5, 50*time.Millisecond)
	time.Sleep(60 * time.Millisecond)

	// Decrement on expired entry is a no-op
	if err := store.DecrementWindow(ctx, "key1", 3); err != nil {
		t.Fatalf("DecrementWindow on expired key failed: %v", err)
	}
}

// --- GetCredit / SetCredit / DeductCredit / AddCredit ---

func TestSetCredit_AndGetCredit(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	val := MustNewCredit("100.5")
	if err := store.SetCredit(ctx, "pool1", val); err != nil {
		t.Fatalf("SetCredit failed: %v", err)
	}

	got, err := store.GetCredit(ctx, "pool1")
	if err != nil {
		t.Fatalf("GetCredit failed: %v", err)
	}
	if got.Cmp(val) != 0 {
		t.Fatalf("expected %s, got %s", val.String(), got.String())
	}
}

func TestDeductCredit_Success(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	store.SetCredit(ctx, "pool1", MustNewCredit("100"))
	remaining, err := store.DeductCredit(ctx, "pool1", MustNewCredit("30"))
	if err != nil {
		t.Fatalf("DeductCredit failed: %v", err)
	}
	expected := MustNewCredit("70")
	if remaining.Cmp(expected) != 0 {
		t.Fatalf("expected remaining=%s, got %s", expected.String(), remaining.String())
	}

	// Verify via GetCredit
	got, _ := store.GetCredit(ctx, "pool1")
	if got.Cmp(expected) != 0 {
		t.Fatalf("GetCredit after deduct: expected %s, got %s", expected.String(), got.String())
	}
}

func TestDeductCredit_InsufficientBalance(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	store.SetCredit(ctx, "pool1", MustNewCredit("10"))
	_, err := store.DeductCredit(ctx, "pool1", MustNewCredit("20"))
	if !errors.Is(err, ErrInsufficientCredits) {
		t.Fatalf("expected ErrInsufficientCredits, got %v", err)
	}

	// Balance must remain unchanged
	got, _ := store.GetCredit(ctx, "pool1")
	if got.Cmp(MustNewCredit("10")) != 0 {
		t.Fatalf("balance changed after failed deduction: got %s", got.String())
	}
}

func TestAddCredit_Success(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	store.SetCredit(ctx, "pool1", MustNewCredit("50"))
	result, err := store.AddCredit(ctx, "pool1", MustNewCredit("25"))
	if err != nil {
		t.Fatalf("AddCredit failed: %v", err)
	}
	expected := MustNewCredit("75")
	if result.Cmp(expected) != 0 {
		t.Fatalf("expected result=%s, got %s", expected.String(), result.String())
	}

	got, _ := store.GetCredit(ctx, "pool1")
	if got.Cmp(expected) != 0 {
		t.Fatalf("GetCredit after add: expected %s, got %s", expected.String(), got.String())
	}
}

// --- ErrPoolNotFound for credit operations on non-existent pools ---

func TestGetCredit_PoolNotFound(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	_, err := store.GetCredit(ctx, "nonexistent")
	if !errors.Is(err, ErrPoolNotFound) {
		t.Fatalf("expected ErrPoolNotFound, got %v", err)
	}
}

func TestDeductCredit_PoolNotFound(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	_, err := store.DeductCredit(ctx, "nonexistent", MustNewCredit("1"))
	if !errors.Is(err, ErrPoolNotFound) {
		t.Fatalf("expected ErrPoolNotFound, got %v", err)
	}
}

func TestAddCredit_PoolNotFound(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	_, err := store.AddCredit(ctx, "nonexistent", MustNewCredit("1"))
	if !errors.Is(err, ErrPoolNotFound) {
		t.Fatalf("expected ErrPoolNotFound, got %v", err)
	}
}

// --- Close ---

func TestMemoryStore_Close(t *testing.T) {
	store := NewMemoryStore()
	if err := store.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}
