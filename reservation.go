package apibudget

import (
	"context"
	"sync"
	"time"
)

// windowKeyInfo はキャンセル時にカウンタを復元するための窓キー情報。
type windowKeyInfo struct {
	key      string
	duration time.Duration
}

// Reservation はレート枠とクレジットの予約を表す。
// Reserve/ReserveNで生成され、Confirm/Cancelで確定する。
type Reservation struct {
	manager      *BudgetManager
	reservedCost Credit
	apiName      string
	poolName     string
	windowKeys   []windowKeyInfo
	n            int64
	delay        time.Duration
	mu           sync.Mutex
	ok           bool
	confirmed    bool
	canceled     bool
}

// OK は予約が有効かどうかを返す。
func (r *Reservation) OK() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.canceled {
		return false
	}
	return r.ok
}

// Delay は予約が実行可能になるまでの待機時間を返す。
func (r *Reservation) Delay() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.delay
}

// Cancel は予約をキャンセルし、確保したレート枠とクレジットを解放する。
// 既にConfirmまたはCancel済みの場合はno-op。
func (r *Reservation) Cancel() {
	r.CancelAt(time.Now())
}

// CancelAt は指定時刻で予約をキャンセルする。
// 既にConfirmまたはCancel済みの場合はno-op。
func (r *Reservation) CancelAt(t time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.confirmed || r.canceled || !r.ok {
		return
	}
	r.canceled = true

	ctx := context.Background()

	// 窓カウンタを復元
	for _, wk := range r.windowKeys {
		if err := r.manager.store.DecrementWindow(ctx, wk.key, r.n); err != nil {
			r.manager.logger.Error("failed to decrement window on cancel",
				"api", r.apiName, "key", wk.key, "error", err)
		}
	}

	// クレジットを返還
	if !r.reservedCost.IsZero() && r.poolName != "" {
		if _, err := r.manager.store.AddCredit(ctx, r.poolName, r.reservedCost); err != nil {
			r.manager.logger.Error("failed to restore credit on cancel",
				"pool", r.poolName, "amount", r.reservedCost.String(), "error", err)
		}
	}
}

// Confirm は予約を確定し、実際の消費量で差分を調整する。
// actualCost が予約時のデフォルトコストと異なる場合、差分を調整する。
// 既にConfirmまたはCancel済みの場合は ErrReservationAlreadyFinalized を返す。
func (r *Reservation) Confirm(actualCost Credit) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.confirmed || r.canceled {
		return ErrReservationAlreadyFinalized
	}
	r.confirmed = true

	// クレジットプールが関連しない場合は何もしない
	if r.poolName == "" {
		return nil
	}

	diff := actualCost.Sub(r.reservedCost)

	if diff.IsZero() {
		return nil
	}

	ctx := context.Background()

	if diff.IsNegative() {
		// 予約量より少なかった → 差分を返還
		refund := Credit{val: diff.ensureVal().Neg(diff.ensureVal())} // 正の値に変換
		_, err := r.manager.store.AddCredit(ctx, r.poolName, refund)
		return err
	}

	// 予約量より多かった → 追加消費
	_, err := r.manager.store.DeductCredit(ctx, r.poolName, diff)
	if err != nil {
		// 残高不足でも消費自体は反映する（事後超過を許容）
		// 強制的に残高を減算
		remaining, getErr := r.manager.store.GetCredit(ctx, r.poolName)
		if getErr == nil {
			_ = r.manager.store.SetCredit(ctx, r.poolName, remaining.Sub(diff))
		}
		return ErrInsufficientCredits
	}
	return nil
}
