package apibudget

import (
	"fmt"
	"time"
)

// windowKey は時間窓に対応するストアキーを生成する。
// 同一窓内のリクエストは同一キーにマッピングされる。
// フォーマット: "apibudget:{apiName}:{duration}:{windowStart}"
func windowKey(apiName string, windowDuration time.Duration, t time.Time) string {
	windowStart := t.Truncate(windowDuration)
	return fmt.Sprintf("apibudget:%s:%s:%d",
		apiName,
		windowDuration.String(),
		windowStart.Unix(),
	)
}

// windowResetTime は現在の窓が終了する時刻（次の窓の開始時刻）を返す。
func windowResetTime(t time.Time, windowDuration time.Duration) time.Time {
	return t.Truncate(windowDuration).Add(windowDuration)
}
