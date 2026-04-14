package apibudget

import (
	"context"
	"math/big"
	"sync"
	"time"
)

// counterEntry はウィンドウカウンタのエントリ。
type counterEntry struct {
	expires time.Time
	count   int64
}

// memoryStore はsync.Mutexベースのインメモリストア実装。
type memoryStore struct {
	counters map[string]*counterEntry
	credits  map[string]*big.Rat
	mu       sync.Mutex
	closed   bool
}

// NewMemoryStore はインメモリストアを生成する。
func NewMemoryStore() Store {
	return &memoryStore{
		counters: make(map[string]*counterEntry),
		credits:  make(map[string]*big.Rat),
	}
}

// IncrementWindow はキーのカウンタをdelta分増加し、現在値を返す。
// エントリが存在しないか期限切れの場合、新規作成（count=0, expires=now+window）してからdeltaを加算する。
func (s *memoryStore) IncrementWindow(_ context.Context, key string, delta int64, window time.Duration) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	entry, ok := s.counters[key]
	if !ok || now.After(entry.expires) {
		entry = &counterEntry{
			count:   0,
			expires: now.Add(window),
		}
		s.counters[key] = entry
	}

	entry.count += delta
	return entry.count, nil
}

// DecrementWindow はキーのカウンタをdelta分減少させる（最小0）。
func (s *memoryStore) DecrementWindow(_ context.Context, key string, delta int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.counters[key]
	if !ok {
		return nil
	}

	if time.Now().After(entry.expires) {
		return nil
	}

	entry.count -= delta
	if entry.count < 0 {
		entry.count = 0
	}
	return nil
}

// GetWindowCount は現在の窓内のカウント値を返す。期限切れまたは未登録の場合は0を返す。
func (s *memoryStore) GetWindowCount(_ context.Context, key string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.counters[key]
	if !ok {
		return 0, nil
	}

	if time.Now().After(entry.expires) {
		return 0, nil
	}

	return entry.count, nil
}

// GetCredit はクレジットプールの残高を取得する。未登録の場合はErrPoolNotFoundを返す。
func (s *memoryStore) GetCredit(_ context.Context, poolKey string) (Credit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	val, ok := s.credits[poolKey]
	if !ok {
		return Credit{}, ErrPoolNotFound
	}

	return Credit{val: new(big.Rat).Set(val)}, nil
}

// SetCredit はクレジットプールの残高を設定する。
func (s *memoryStore) SetCredit(_ context.Context, poolKey string, value Credit) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.credits[poolKey] = new(big.Rat).Set(value.ensureVal())
	return nil
}

// DeductCredit はクレジットを原子的に減算する。残高不足ならエラーを返し残高を変更しない。
func (s *memoryStore) DeductCredit(_ context.Context, poolKey string, amount Credit) (Credit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	val, ok := s.credits[poolKey]
	if !ok {
		return Credit{}, ErrPoolNotFound
	}

	amountVal := amount.ensureVal()
	remaining := new(big.Rat).Sub(val, amountVal)

	if remaining.Sign() < 0 {
		// 残高不足: 残高を変更せずエラーを返す
		return Credit{val: new(big.Rat).Set(val)}, ErrInsufficientCredits
	}

	s.credits[poolKey] = remaining
	return Credit{val: new(big.Rat).Set(remaining)}, nil
}

// AddCredit はクレジットを加算する。
func (s *memoryStore) AddCredit(_ context.Context, poolKey string, amount Credit) (Credit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	val, ok := s.credits[poolKey]
	if !ok {
		return Credit{}, ErrPoolNotFound
	}

	amountVal := amount.ensureVal()
	result := new(big.Rat).Add(val, amountVal)
	s.credits[poolKey] = result

	return Credit{val: new(big.Rat).Set(result)}, nil
}

// Close closes the store.
func (s *memoryStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}
