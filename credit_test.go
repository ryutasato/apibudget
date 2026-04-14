package apibudget

import (
	"fmt"
	"math/rand"
	"testing"
	"testing/quick"
)

// randomCreditString generates a random rational number string suitable for NewCredit.
// It produces strings like "123/456" or "-789/12" with varying magnitudes.
func randomCreditString(r *rand.Rand) string {
	num := r.Int63n(1_000_000_000_000) - 500_000_000_000 // range: [-500B, 500B)
	den := r.Int63n(999_999_999) + 1                     // range: [1, 1B)
	return fmt.Sprintf("%d/%d", num, den)
}

// TestCreditArithmeticRoundTrip verifies Property 1: Credit型の算術ラウンドトリップ
// For any two Credit values a, b: a.Add(b).Sub(b) == a (verified via Cmp returning 0).
//
// **Validates: Requirements 1.4**
func TestCreditArithmeticRoundTrip(t *testing.T) {
	f := func(seed1, seed2 int64) bool {
		r := rand.New(rand.NewSource(seed1 + seed2))

		aStr := randomCreditString(r)
		bStr := randomCreditString(r)

		a, err := NewCredit(aStr)
		if err != nil {
			t.Logf("failed to create credit a from %q: %v", aStr, err)
			return false
		}
		b, err := NewCredit(bStr)
		if err != nil {
			t.Logf("failed to create credit b from %q: %v", bStr, err)
			return false
		}

		// Property: a.Add(b).Sub(b) == a
		result := a.Add(b).Sub(b)
		if result.Cmp(a) != 0 {
			t.Logf("round-trip failed: a=%s, b=%s, a.Add(b).Sub(b)=%s", a.String(), b.String(), result.String())
			return false
		}
		return true
	}

	cfg := &quick.Config{
		MaxCount: 1000,
	}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("Property 1 (arithmetic round-trip) failed: %v", err)
	}
}

// TestCreditSerializationRoundTrip verifies Property 2: Credit型のシリアライゼーションラウンドトリップ
// For any valid Credit value c: NewCredit(c.String()) == c (verified via Cmp returning 0).
//
// **Validates: Requirements 1.5**
func TestCreditSerializationRoundTrip(t *testing.T) {
	f := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))

		cStr := randomCreditString(r)

		c, err := NewCredit(cStr)
		if err != nil {
			t.Logf("failed to create credit from %q: %v", cStr, err)
			return false
		}

		// Serialize to string and parse back
		serialized := c.String()
		roundTripped, err := NewCredit(serialized)
		if err != nil {
			t.Logf("failed to parse serialized credit %q (original: %q): %v", serialized, cStr, err)
			return false
		}

		// Property: NewCredit(c.String()) == c
		if roundTripped.Cmp(c) != 0 {
			t.Logf("round-trip failed: original=%s, serialized=%q, roundTripped=%s", c.String(), serialized, roundTripped.String())
			return false
		}
		return true
	}

	cfg := &quick.Config{
		MaxCount: 1000,
	}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("Property 2 (serialization round-trip) failed: %v", err)
	}
}

// ================================================================
// Unit Tests for Credit型
// Validates: Requirements 1.1, 1.2, 1.3
// ================================================================

// TestNewCredit_ValidInputs verifies that NewCredit correctly parses valid string inputs.
// **Validates: Requirements 1.1**
func TestNewCredit_ValidInputs(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantF64 float64
	}{
		{"decimal", "1.5", 1.5},
		{"large value", "1000000000000", 1_000_000_000_000},
		{"zero", "0", 0},
		{"negative decimal", "-3.14", -3.14},
		{"rational", "1/3", 1.0 / 3.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewCredit(tt.input)
			if err != nil {
				t.Fatalf("NewCredit(%q) returned unexpected error: %v", tt.input, err)
			}
			got := c.Float64()
			diff := got - tt.wantF64
			if diff < -1e-9 || diff > 1e-9 {
				t.Errorf("NewCredit(%q).Float64() = %v, want ≈ %v", tt.input, got, tt.wantF64)
			}
		})
	}
}

// TestNewCredit_InvalidInputs verifies that NewCredit returns an error for invalid strings.
// **Validates: Requirements 1.2**
func TestNewCredit_InvalidInputs(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty string", ""},
		{"alphabetic", "abc"},
		{"division by zero", "1/0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewCredit(tt.input)
			if err == nil {
				t.Errorf("NewCredit(%q) expected error, got nil", tt.input)
			}
		})
	}
}

// TestMustNewCredit_PanicsOnInvalid verifies that MustNewCredit panics on invalid input.
// **Validates: Requirements 1.2**
func TestMustNewCredit_PanicsOnInvalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty string", ""},
		{"alphabetic", "abc"},
		{"division by zero", "1/0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("MustNewCredit(%q) expected panic, but did not panic", tt.input)
				}
			}()
			MustNewCredit(tt.input)
		})
	}
}

// TestMustNewCredit_ValidInput verifies MustNewCredit does not panic on valid input.
// **Validates: Requirements 1.1**
func TestMustNewCredit_ValidInput(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("MustNewCredit(\"42\") panicked unexpectedly: %v", r)
		}
	}()
	c := MustNewCredit("42")
	if c.Float64() != 42.0 {
		t.Errorf("MustNewCredit(\"42\").Float64() = %v, want 42", c.Float64())
	}
}

// TestNewCreditFromInt verifies NewCreditFromInt creates correct Credit values.
// **Validates: Requirements 1.1**
func TestNewCreditFromInt(t *testing.T) {
	tests := []struct {
		name    string
		input   int64
		wantF64 float64
	}{
		{"zero", 0, 0},
		{"positive", 100, 100},
		{"negative", -50, -50},
		{"large", 1_000_000_000_000, 1_000_000_000_000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCreditFromInt(tt.input)
			if c.Float64() != tt.wantF64 {
				t.Errorf("NewCreditFromInt(%d).Float64() = %v, want %v", tt.input, c.Float64(), tt.wantF64)
			}
		})
	}
}

// TestCredit_IsZero verifies IsZero returns true only for zero values.
// **Validates: Requirements 1.3**
func TestCredit_IsZero(t *testing.T) {
	tests := []struct {
		val  Credit
		name string
		want bool
	}{
		{MustNewCredit("0"), "zero from string", true},
		{NewCreditFromInt(0), "zero from int", true},
		{Credit{}, "zero value struct", true},
		{MustNewCredit("1"), "positive", false},
		{MustNewCredit("-1"), "negative", false},
		{MustNewCredit("1/1000000"), "small fraction", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.val.IsZero(); got != tt.want {
				t.Errorf("Credit(%s).IsZero() = %v, want %v", tt.val.String(), got, tt.want)
			}
		})
	}
}

// TestCredit_IsNegative verifies IsNegative returns true only for negative values.
// **Validates: Requirements 1.3**
func TestCredit_IsNegative(t *testing.T) {
	tests := []struct {
		val  Credit
		name string
		want bool
	}{
		{MustNewCredit("-1"), "negative", true},
		{MustNewCredit("-1/3"), "negative fraction", true},
		{MustNewCredit("0"), "zero", false},
		{MustNewCredit("1"), "positive", false},
		{Credit{}, "zero value struct", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.val.IsNegative(); got != tt.want {
				t.Errorf("Credit(%s).IsNegative() = %v, want %v", tt.val.String(), got, tt.want)
			}
		})
	}
}

// TestCredit_Float64 verifies Float64 returns correct approximate values.
// **Validates: Requirements 1.3**
func TestCredit_Float64(t *testing.T) {
	tests := []struct {
		val     Credit
		name    string
		wantF64 float64
	}{
		{MustNewCredit("42"), "integer", 42.0},
		{MustNewCredit("3.14"), "decimal", 3.14},
		{MustNewCredit("0"), "zero", 0.0},
		{MustNewCredit("-2.5"), "negative", -2.5},
		{Credit{}, "zero value struct", 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.val.Float64()
			diff := got - tt.wantF64
			if diff < -1e-9 || diff > 1e-9 {
				t.Errorf("Credit.Float64() = %v, want ≈ %v", got, tt.wantF64)
			}
		})
	}
}

// TestCredit_ArithmeticBoundary verifies arithmetic operations with boundary values.
// **Validates: Requirements 1.3**
func TestCredit_ArithmeticBoundary(t *testing.T) {
	zero := MustNewCredit("0")
	positive := MustNewCredit("100")
	negative := MustNewCredit("-50")
	large := MustNewCredit("1000000000000")

	t.Run("add zero identity", func(t *testing.T) {
		result := positive.Add(zero)
		if result.Cmp(positive) != 0 {
			t.Errorf("100 + 0 = %s, want 100", result.String())
		}
	})

	t.Run("sub zero identity", func(t *testing.T) {
		result := positive.Sub(zero)
		if result.Cmp(positive) != 0 {
			t.Errorf("100 - 0 = %s, want 100", result.String())
		}
	})

	t.Run("add negative", func(t *testing.T) {
		result := positive.Add(negative)
		want := MustNewCredit("50")
		if result.Cmp(want) != 0 {
			t.Errorf("100 + (-50) = %s, want 50", result.String())
		}
	})

	t.Run("sub to negative", func(t *testing.T) {
		result := negative.Sub(positive)
		want := MustNewCredit("-150")
		if result.Cmp(want) != 0 {
			t.Errorf("-50 - 100 = %s, want -150", result.String())
		}
	})

	t.Run("mul by zero", func(t *testing.T) {
		result := positive.Mul(0)
		if !result.IsZero() {
			t.Errorf("100 * 0 = %s, want 0", result.String())
		}
	})

	t.Run("mul by negative", func(t *testing.T) {
		result := positive.Mul(-1)
		want := MustNewCredit("-100")
		if result.Cmp(want) != 0 {
			t.Errorf("100 * (-1) = %s, want -100", result.String())
		}
	})

	t.Run("large value add", func(t *testing.T) {
		result := large.Add(large)
		want := MustNewCredit("2000000000000")
		if result.Cmp(want) != 0 {
			t.Errorf("1T + 1T = %s, want 2T", result.String())
		}
	})

	t.Run("cmp ordering", func(t *testing.T) {
		if negative.Cmp(zero) >= 0 {
			t.Error("expected -50 < 0")
		}
		if zero.Cmp(positive) >= 0 {
			t.Error("expected 0 < 100")
		}
		if positive.Cmp(positive) != 0 {
			t.Error("expected 100 == 100")
		}
	})

	t.Run("zero value struct arithmetic", func(t *testing.T) {
		zeroStruct := Credit{}
		result := zeroStruct.Add(positive)
		if result.Cmp(positive) != 0 {
			t.Errorf("Credit{} + 100 = %s, want 100", result.String())
		}
	})
}

// TestCredit_Sub verifies that Sub correctly subtracts two Credit values.
func TestCredit_Sub(t *testing.T) {
	tests := []struct {
		name string
		a    Credit
		b    Credit
		want Credit
	}{
		{
			name: "positive - positive",
			a:    MustNewCredit("100"),
			b:    MustNewCredit("40"),
			want: MustNewCredit("60"),
		},
		{
			name: "positive - larger positive",
			a:    MustNewCredit("40"),
			b:    MustNewCredit("100"),
			want: MustNewCredit("-60"),
		},
		{
			name: "positive - negative",
			a:    MustNewCredit("100"),
			b:    MustNewCredit("-40"),
			want: MustNewCredit("140"),
		},
		{
			name: "negative - negative",
			a:    MustNewCredit("-100"),
			b:    MustNewCredit("-40"),
			want: MustNewCredit("-60"),
		},
		{
			name: "negative - positive",
			a:    MustNewCredit("-100"),
			b:    MustNewCredit("40"),
			want: MustNewCredit("-140"),
		},
		{
			name: "zero - positive",
			a:    MustNewCredit("0"),
			b:    MustNewCredit("100"),
			want: MustNewCredit("-100"),
		},
		{
			name: "positive - zero",
			a:    MustNewCredit("100"),
			b:    MustNewCredit("0"),
			want: MustNewCredit("100"),
		},
		{
			name: "zero value struct - positive",
			a:    Credit{},
			b:    MustNewCredit("100"),
			want: MustNewCredit("-100"),
		},
		{
			name: "positive - zero value struct",
			a:    MustNewCredit("100"),
			b:    Credit{},
			want: MustNewCredit("100"),
		},
		{
			name: "rational subtraction",
			a:    MustNewCredit("1/3"),
			b:    MustNewCredit("1/6"),
			want: MustNewCredit("1/6"),
		},
		{
			name: "large values",
			a:    MustNewCredit("2000000000000"),
			b:    MustNewCredit("500000000000"),
			want: MustNewCredit("1500000000000"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.a.Sub(tt.b)
			if got.Cmp(tt.want) != 0 {
				t.Errorf("Credit(%s).Sub(Credit(%s)) = %s, want %s", tt.a.String(), tt.b.String(), got.String(), tt.want.String())
			}
		})
	}
}
