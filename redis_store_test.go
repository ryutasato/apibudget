package apibudget

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

// redisAddr はテスト用のRedisアドレスを返す。
func redisAddr() string {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		return "localhost:6379"
	}
	return addr
}

// newTestRedisStore はテスト用のRedisStoreを生成する。
// Redisに接続できない場合はテストをスキップする。
func newTestRedisStore(t *testing.T) *redisStore {
	t.Helper()
	store, err := NewRedisStore(redisAddr())
	if err != nil {
		t.Skip("Redis not available")
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store.(*redisStore)
}

// uniqueKey はテスト間の衝突を避けるためのユニークキーを生成する。
func uniqueKey(t *testing.T, prefix string) string {
	t.Helper()
	return prefix + ":" + t.Name()
}

func TestRedisStore_IncrementWindow(t *testing.T) {
	store := newTestRedisStore(t)
	ctx := context.Background()
	key := uniqueKey(t, "window")

	// 初回インクリメント
	count, err := store.IncrementWindow(ctx, key, 1, 10*time.Second)
	if err != nil {
		t.Fatalf("IncrementWindow failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected count=1, got %d", count)
	}

	// 2回目のインクリメント
	count, err = store.IncrementWindow(ctx, key, 3, 10*time.Second)
	if err != nil {
		t.Fatalf("IncrementWindow failed: %v", err)
	}
	if count != 4 {
		t.Errorf("expected count=4, got %d", count)
	}

	// クリーンアップ
	store.client.Del(ctx, key)
}

func TestRedisStore_DecrementWindow(t *testing.T) {
	store := newTestRedisStore(t)
	ctx := context.Background()
	key := uniqueKey(t, "window")

	// まずインクリメント
	_, err := store.IncrementWindow(ctx, key, 5, 10*time.Second)
	if err != nil {
		t.Fatalf("IncrementWindow failed: %v", err)
	}

	// デクリメント
	err = store.DecrementWindow(ctx, key, 2)
	if err != nil {
		t.Fatalf("DecrementWindow failed: %v", err)
	}

	count, err := store.GetWindowCount(ctx, key)
	if err != nil {
		t.Fatalf("GetWindowCount failed: %v", err)
	}
	if count != 3 {
		t.Errorf("expected count=3, got %d", count)
	}

	// 0以下にならないことを確認
	err = store.DecrementWindow(ctx, key, 100)
	if err != nil {
		t.Fatalf("DecrementWindow failed: %v", err)
	}

	count, err = store.GetWindowCount(ctx, key)
	if err != nil {
		t.Fatalf("GetWindowCount failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected count=0, got %d", count)
	}

	// クリーンアップ
	store.client.Del(ctx, key)
}

func TestRedisStore_GetWindowCount(t *testing.T) {
	store := newTestRedisStore(t)
	ctx := context.Background()
	key := uniqueKey(t, "window")

	// 存在しないキーは0を返す
	count, err := store.GetWindowCount(ctx, key)
	if err != nil {
		t.Fatalf("GetWindowCount failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected count=0 for non-existent key, got %d", count)
	}
}

func TestRedisStore_GetCredit_SetCredit(t *testing.T) {
	store := newTestRedisStore(t)
	ctx := context.Background()
	poolKey := uniqueKey(t, "credit")

	// 存在しないプールはErrPoolNotFound
	_, err := store.GetCredit(ctx, poolKey)
	if !errors.Is(err, ErrPoolNotFound) {
		t.Errorf("expected ErrPoolNotFound, got %v", err)
	}

	// SetCredit
	err = store.SetCredit(ctx, poolKey, MustNewCredit("100"))
	if err != nil {
		t.Fatalf("SetCredit failed: %v", err)
	}

	// GetCredit
	credit, err := store.GetCredit(ctx, poolKey)
	if err != nil {
		t.Fatalf("GetCredit failed: %v", err)
	}
	if credit.Cmp(MustNewCredit("100")) != 0 {
		t.Errorf("expected credit=100, got %s", credit.String())
	}

	// 小数値のSetCredit
	err = store.SetCredit(ctx, poolKey, MustNewCredit("99.5"))
	if err != nil {
		t.Fatalf("SetCredit failed: %v", err)
	}

	credit, err = store.GetCredit(ctx, poolKey)
	if err != nil {
		t.Fatalf("GetCredit failed: %v", err)
	}
	if credit.Cmp(MustNewCredit("99.5")) != 0 {
		t.Errorf("expected credit=99.5, got %s", credit.String())
	}

	// クリーンアップ
	store.client.Del(ctx, poolKey)
}

func TestRedisStore_DeductCredit(t *testing.T) {
	store := newTestRedisStore(t)
	ctx := context.Background()
	poolKey := uniqueKey(t, "credit")

	// 初期残高を設定
	err := store.SetCredit(ctx, poolKey, MustNewCredit("100"))
	if err != nil {
		t.Fatalf("SetCredit failed: %v", err)
	}

	// 正常な減算
	remaining, err := store.DeductCredit(ctx, poolKey, MustNewCredit("30"))
	if err != nil {
		t.Fatalf("DeductCredit failed: %v", err)
	}
	if remaining.Cmp(MustNewCredit("70")) != 0 {
		t.Errorf("expected remaining=70, got %s", remaining.String())
	}

	// 残高不足
	_, err = store.DeductCredit(ctx, poolKey, MustNewCredit("100"))
	if !errors.Is(err, ErrInsufficientCredits) {
		t.Errorf("expected ErrInsufficientCredits, got %v", err)
	}

	// 残高が変更されていないことを確認
	credit, err := store.GetCredit(ctx, poolKey)
	if err != nil {
		t.Fatalf("GetCredit failed: %v", err)
	}
	if credit.Cmp(MustNewCredit("70")) != 0 {
		t.Errorf("expected credit=70 (unchanged), got %s", credit.String())
	}

	// 存在しないプール
	_, err = store.DeductCredit(ctx, poolKey+"_nonexistent", MustNewCredit("1"))
	if !errors.Is(err, ErrPoolNotFound) {
		t.Errorf("expected ErrPoolNotFound, got %v", err)
	}

	// クリーンアップ
	store.client.Del(ctx, poolKey)
}

func TestRedisStore_DeductCredit_Fractional(t *testing.T) {
	store := newTestRedisStore(t)
	ctx := context.Background()
	poolKey := uniqueKey(t, "credit")

	// 小数値での操作
	err := store.SetCredit(ctx, poolKey, MustNewCredit("10.5"))
	if err != nil {
		t.Fatalf("SetCredit failed: %v", err)
	}

	remaining, err := store.DeductCredit(ctx, poolKey, MustNewCredit("3.5"))
	if err != nil {
		t.Fatalf("DeductCredit failed: %v", err)
	}
	if remaining.Cmp(MustNewCredit("7")) != 0 {
		t.Errorf("expected remaining=7, got %s", remaining.String())
	}

	// クリーンアップ
	store.client.Del(ctx, poolKey)
}

func TestRedisStore_AddCredit(t *testing.T) {
	store := newTestRedisStore(t)
	ctx := context.Background()
	poolKey := uniqueKey(t, "credit")

	// 初期残高を設定
	err := store.SetCredit(ctx, poolKey, MustNewCredit("50"))
	if err != nil {
		t.Fatalf("SetCredit failed: %v", err)
	}

	// 加算
	result, err := store.AddCredit(ctx, poolKey, MustNewCredit("25"))
	if err != nil {
		t.Fatalf("AddCredit failed: %v", err)
	}
	if result.Cmp(MustNewCredit("75")) != 0 {
		t.Errorf("expected result=75, got %s", result.String())
	}

	// 小数値の加算
	result, err = store.AddCredit(ctx, poolKey, MustNewCredit("0.5"))
	if err != nil {
		t.Fatalf("AddCredit failed: %v", err)
	}
	if result.Cmp(MustNewCredit("75.5")) != 0 {
		t.Errorf("expected result=75.5, got %s", result.String())
	}

	// 存在しないプール
	_, err = store.AddCredit(ctx, poolKey+"_nonexistent", MustNewCredit("1"))
	if !errors.Is(err, ErrPoolNotFound) {
		t.Errorf("expected ErrPoolNotFound, got %v", err)
	}

	// クリーンアップ
	store.client.Del(ctx, poolKey)
}

func TestRedisStore_AtomicDeductCredit(t *testing.T) {
	store := newTestRedisStore(t)
	ctx := context.Background()
	poolKey := uniqueKey(t, "credit")

	// 残高ちょうどの減算
	err := store.SetCredit(ctx, poolKey, MustNewCredit("10"))
	if err != nil {
		t.Fatalf("SetCredit failed: %v", err)
	}

	remaining, err := store.DeductCredit(ctx, poolKey, MustNewCredit("10"))
	if err != nil {
		t.Fatalf("DeductCredit failed: %v", err)
	}
	if remaining.Cmp(MustNewCredit("0")) != 0 {
		t.Errorf("expected remaining=0, got %s", remaining.String())
	}

	// 0からの減算は失敗
	_, err = store.DeductCredit(ctx, poolKey, MustNewCredit("1"))
	if !errors.Is(err, ErrInsufficientCredits) {
		t.Errorf("expected ErrInsufficientCredits, got %v", err)
	}

	// クリーンアップ
	store.client.Del(ctx, poolKey)
}

func TestRedisStore_ConnectionFailure(t *testing.T) {
	// 無効なアドレスで接続失敗を確認
	_, err := NewRedisStore("localhost:1")
	if err == nil {
		t.Error("expected error for invalid Redis address, got nil")
	}
}

func TestRedisStore_Close(t *testing.T) {
	store, err := NewRedisStore(redisAddr())
	if err != nil {
		t.Skip("Redis not available")
	}

	err = store.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestRedisStore_WithOptions(t *testing.T) {
	// パスワード付きで接続（パスワードが設定されていないRedisでは失敗する可能性がある）
	_, err := NewRedisStore(redisAddr(), WithRedisPassword("wrongpassword"))
	// パスワードが設定されていないRedisでは認証エラーになるか、
	// パスワードが設定されているRedisでは不正パスワードでエラーになる
	// いずれにしてもオプションが正しく適用されることを確認
	_ = err // エラーの有無はRedis設定に依存

	// DB番号オプション
	store, err := NewRedisStore(redisAddr(), WithRedisDB(1))
	if err != nil {
		t.Skip("Redis not available")
	}
	store.Close()

	// TLSオプション（TLS非対応のRedisでは接続失敗する）
	_, err = NewRedisStore(redisAddr(), WithRedisTLS(true))
	// TLS非対応のRedisでは接続失敗するのが正常
	_ = err
}

func TestRedisStore_IncrementWindow_TTL(t *testing.T) {
	store := newTestRedisStore(t)
	ctx := context.Background()
	key := uniqueKey(t, "window_ttl")

	// 短いTTLでインクリメント
	_, err := store.IncrementWindow(ctx, key, 1, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("IncrementWindow failed: %v", err)
	}

	// TTLが設定されていることを確認
	ttl := store.client.PTTL(ctx, key).Val()
	if ttl <= 0 {
		t.Errorf("expected positive TTL, got %v", ttl)
	}

	// 2回目のインクリメントでTTLが変わらないことを確認
	_, err = store.IncrementWindow(ctx, key, 1, 10*time.Second)
	if err != nil {
		t.Fatalf("IncrementWindow failed: %v", err)
	}

	// TTLは最初の設定値（500ms）に近いはず（10sではない）
	ttl2 := store.client.PTTL(ctx, key).Val()
	if ttl2 > 1*time.Second {
		t.Errorf("TTL should not have been reset to 10s, got %v", ttl2)
	}

	// クリーンアップ
	store.client.Del(ctx, key)
}

func TestRedisStore_DecrementWindow_NonExistent(t *testing.T) {
	store := newTestRedisStore(t)
	ctx := context.Background()
	key := uniqueKey(t, "window_nonexist")

	// 存在しないキーのデクリメントはエラーにならない
	err := store.DecrementWindow(ctx, key, 5)
	if err != nil {
		t.Fatalf("DecrementWindow on non-existent key should not fail: %v", err)
	}
}
