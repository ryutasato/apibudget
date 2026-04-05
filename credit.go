package apibudget

import (
	"fmt"
	"math/big"
)

// Credit は小数点〜数兆単位のクレジット値を正確に扱うための型。
// 内部的に *big.Rat を使用し、浮動小数点の誤差を回避する。
type Credit struct {
	val *big.Rat
}

// NewCredit は文字列からCreditを生成する。"1.5", "1000000000000", "-3.14" 等。
func NewCredit(s string) (Credit, error) {
	r := new(big.Rat)
	if _, ok := r.SetString(s); !ok {
		return Credit{}, fmt.Errorf("apibudget: invalid credit value: %q", s)
	}
	return Credit{val: r}, nil
}

// MustNewCredit は文字列からCreditを生成する。無効な文字列の場合panicする。
func MustNewCredit(s string) Credit {
	c, err := NewCredit(s)
	if err != nil {
		panic(err)
	}
	return c
}

// NewCreditFromInt はint64からCreditを生成する。
func NewCreditFromInt(n int64) Credit {
	return Credit{val: new(big.Rat).SetInt64(n)}
}

// ensureVal はnilのvalをゼロとして扱うためのヘルパー。
func (c Credit) ensureVal() *big.Rat {
	if c.val == nil {
		return new(big.Rat)
	}
	return c.val
}

// Add は2つのCredit値を加算し、新しいCreditを返す。
func (c Credit) Add(other Credit) Credit {
	result := new(big.Rat).Add(c.ensureVal(), other.ensureVal())
	return Credit{val: result}
}

// Sub は2つのCredit値を減算し、新しいCreditを返す。
func (c Credit) Sub(other Credit) Credit {
	result := new(big.Rat).Sub(c.ensureVal(), other.ensureVal())
	return Credit{val: result}
}

// Mul はCredit値をint64で乗算し、新しいCreditを返す。
func (c Credit) Mul(n int64) Credit {
	multiplier := new(big.Rat).SetInt64(n)
	result := new(big.Rat).Mul(c.ensureVal(), multiplier)
	return Credit{val: result}
}

// Cmp は2つのCredit値を比較する。
// c < other なら -1、c == other なら 0、c > other なら 1 を返す。
func (c Credit) Cmp(other Credit) int {
	return c.ensureVal().Cmp(other.ensureVal())
}

// IsZero はCredit値がゼロかどうかを返す。
func (c Credit) IsZero() bool {
	return c.ensureVal().Sign() == 0
}

// IsNegative はCredit値が負かどうかを返す。
func (c Credit) IsNegative() bool {
	return c.ensureVal().Sign() < 0
}

// String はCredit値を文字列に変換する。
// 出力はNewCreditでラウンドトリップ可能な形式。
func (c Credit) String() string {
	return c.ensureVal().RatString()
}

// Float64 はCredit値をfloat64の近似値として返す（ログ用）。
func (c Credit) Float64() float64 {
	f, _ := c.ensureVal().Float64()
	return f
}
