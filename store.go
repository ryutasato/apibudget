package apibudget

import (
	"context"
	"time"
)

// Store はレート制限カウンタとクレジット残高の永続化を抽象化するインターフェース。
// インメモリ実装（MemoryStore）とRedis実装（RedisStore）を切り替え可能にする。
type Store interface {
	// IncrementWindow はキーのカウンタをdelta分増加し、現在値を返す。
	// windowはTTL設定に使用される。
	IncrementWindow(ctx context.Context, key string, delta int64, window time.Duration) (int64, error)

	// DecrementWindow はキーのカウンタをdelta分減少させる（予約キャンセル用）。
	DecrementWindow(ctx context.Context, key string, delta int64) error

	// GetWindowCount は現在の窓内のカウント値を返す。
	GetWindowCount(ctx context.Context, key string) (int64, error)

	// GetCredit はクレジットプールの残高を取得する。
	GetCredit(ctx context.Context, poolKey string) (Credit, error)

	// SetCredit はクレジットプールの残高を設定する。
	SetCredit(ctx context.Context, poolKey string, value Credit) error

	// DeductCredit はクレジットを原子的に減算する。残高不足ならエラーを返し残高を変更しない。
	DeductCredit(ctx context.Context, poolKey string, amount Credit) (Credit, error)

	// AddCredit はクレジットを加算する（リセット・増加用）。
	AddCredit(ctx context.Context, poolKey string, amount Credit) (Credit, error)

	// Close はストア接続を閉じる。
	Close() error
}
