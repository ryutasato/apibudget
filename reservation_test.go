package apibudget

import (
	"context"
	"errors"
	"testing"
	"time"
)

type mockStore struct {
	IncrementWindowFunc func(ctx context.Context, key string, delta int64, window time.Duration) (int64, error)
	DecrementWindowFunc func(ctx context.Context, key string, delta int64) error
	GetWindowCountFunc  func(ctx context.Context, key string) (int64, error)
	GetCreditFunc       func(ctx context.Context, poolKey string) (Credit, error)
	SetCreditFunc       func(ctx context.Context, poolKey string, value Credit) error
	DeductCreditFunc    func(ctx context.Context, poolKey string, amount Credit) (Credit, error)
	AddCreditFunc       func(ctx context.Context, poolKey string, amount Credit) (Credit, error)
	CloseFunc           func() error
}

func (m *mockStore) IncrementWindow(ctx context.Context, key string, delta int64, window time.Duration) (int64, error) {
	if m.IncrementWindowFunc != nil {
		return m.IncrementWindowFunc(ctx, key, delta, window)
	}
	return 0, nil
}

func (m *mockStore) DecrementWindow(ctx context.Context, key string, delta int64) error {
	if m.DecrementWindowFunc != nil {
		return m.DecrementWindowFunc(ctx, key, delta)
	}
	return nil
}

func (m *mockStore) GetWindowCount(ctx context.Context, key string) (int64, error) {
	if m.GetWindowCountFunc != nil {
		return m.GetWindowCountFunc(ctx, key)
	}
	return 0, nil
}

func (m *mockStore) GetCredit(ctx context.Context, poolKey string) (Credit, error) {
	if m.GetCreditFunc != nil {
		return m.GetCreditFunc(ctx, poolKey)
	}
	return Credit{}, nil
}

func (m *mockStore) SetCredit(ctx context.Context, poolKey string, value Credit) error {
	if m.SetCreditFunc != nil {
		return m.SetCreditFunc(ctx, poolKey, value)
	}
	return nil
}

func (m *mockStore) DeductCredit(ctx context.Context, poolKey string, amount Credit) (Credit, error) {
	if m.DeductCreditFunc != nil {
		return m.DeductCreditFunc(ctx, poolKey, amount)
	}
	return Credit{}, nil
}

func (m *mockStore) AddCredit(ctx context.Context, poolKey string, amount Credit) (Credit, error) {
	if m.AddCreditFunc != nil {
		return m.AddCreditFunc(ctx, poolKey, amount)
	}
	return Credit{}, nil
}

func (m *mockStore) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

func TestReservation_Confirm_InsufficientCredits(t *testing.T) {
	store := &mockStore{}
	manager := &BudgetManager{
		store:  store,
		logger: newDefaultLogger(LogLevelDebug),
	}

	reservedCost := MustNewCredit("10")
	r := &Reservation{
		manager:      manager,
		poolName:     "test_pool",
		reservedCost: reservedCost,
		ok:           true,
	}

	actualCost := MustNewCredit("15")
	diff := actualCost.Sub(reservedCost) // 5

	remainingCredit := MustNewCredit("2")
	expectedFinalCredit := remainingCredit.Sub(diff) // 2 - 5 = -3

	deductCalled := false
	store.DeductCreditFunc = func(ctx context.Context, poolKey string, amount Credit) (Credit, error) {
		deductCalled = true
		if poolKey != "test_pool" {
			t.Errorf("expected pool test_pool, got %s", poolKey)
		}
		if amount.Cmp(diff) != 0 {
			t.Errorf("expected diff %s, got %s", diff.String(), amount.String())
		}
		return Credit{}, ErrInsufficientCredits
	}

	getCreditCalled := false
	store.GetCreditFunc = func(ctx context.Context, poolKey string) (Credit, error) {
		getCreditCalled = true
		return remainingCredit, nil
	}

	setCreditCalled := false
	store.SetCreditFunc = func(ctx context.Context, poolKey string, value Credit) error {
		setCreditCalled = true
		if value.Cmp(expectedFinalCredit) != 0 {
			t.Errorf("expected final credit %s, got %s", expectedFinalCredit.String(), value.String())
		}
		return nil
	}

	err := r.Confirm(actualCost)

	if err != ErrInsufficientCredits {
		t.Errorf("expected ErrInsufficientCredits, got %v", err)
	}

	if !deductCalled {
		t.Error("DeductCredit was not called")
	}
	if !getCreditCalled {
		t.Error("GetCredit was not called")
	}
	if !setCreditCalled {
		t.Error("SetCredit was not called")
	}
	if !r.confirmed {
		t.Error("expected reservation to be marked confirmed")
	}
}

func TestReservation_Confirm_DeductError(t *testing.T) {
	store := &mockStore{}
	manager := &BudgetManager{
		store:  store,
		logger: newDefaultLogger(LogLevelDebug),
	}

	reservedCost := MustNewCredit("10")
	r := &Reservation{
		manager:      manager,
		poolName:     "test_pool",
		reservedCost: reservedCost,
		ok:           true,
	}

	actualCost := MustNewCredit("15")
	expectedErr := errors.New("some store error")

	store.DeductCreditFunc = func(ctx context.Context, poolKey string, amount Credit) (Credit, error) {
		return Credit{}, expectedErr
	}

	err := r.Confirm(actualCost)

	if !errors.Is(err, expectedErr) {
		t.Errorf("expected %v, got %v", expectedErr, err)
	}
}

func TestReservation_Confirm_NegativeDiff(t *testing.T) {
	store := &mockStore{}
	manager := &BudgetManager{
		store:  store,
		logger: newDefaultLogger(LogLevelDebug),
	}

	reservedCost := MustNewCredit("10")
	r := &Reservation{
		manager:      manager,
		poolName:     "test_pool",
		reservedCost: reservedCost,
		ok:           true,
	}

	actualCost := MustNewCredit("4")
	// diff = 4 - 10 = -6
	// refund should be 6

	addCreditCalled := false
	store.AddCreditFunc = func(ctx context.Context, poolKey string, amount Credit) (Credit, error) {
		addCreditCalled = true
		if poolKey != "test_pool" {
			t.Errorf("expected pool test_pool, got %s", poolKey)
		}
		expectedRefund := MustNewCredit("6")
		if amount.Cmp(expectedRefund) != 0 {
			t.Errorf("expected refund %s, got %s", expectedRefund.String(), amount.String())
		}
		return Credit{}, nil
	}

	err := r.Confirm(actualCost)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !addCreditCalled {
		t.Error("AddCredit was not called")
	}
	if !r.confirmed {
		t.Error("expected reservation to be marked confirmed")
	}
}
