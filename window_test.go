package apibudget

import (
	"fmt"
	"math/rand"
	"testing"
	"testing/quick"
	"time"
)

func TestWindowKey_SameWindowSameKey(t *testing.T) {
	dur := time.Minute
	// Two times within the same 1-minute window
	t1 := time.Date(2024, 1, 1, 12, 5, 10, 0, time.UTC)
	t2 := time.Date(2024, 1, 1, 12, 5, 50, 0, time.UTC)

	k1 := windowKey("api_a", dur, t1)
	k2 := windowKey("api_a", dur, t2)

	if k1 != k2 {
		t.Errorf("expected same key for times in same window, got %q and %q", k1, k2)
	}
}

func TestWindowKey_DifferentWindowDifferentKey(t *testing.T) {
	dur := time.Minute
	// Two times in different 1-minute windows
	t1 := time.Date(2024, 1, 1, 12, 5, 30, 0, time.UTC)
	t2 := time.Date(2024, 1, 1, 12, 6, 30, 0, time.UTC)

	k1 := windowKey("api_a", dur, t1)
	k2 := windowKey("api_a", dur, t2)

	if k1 == k2 {
		t.Errorf("expected different keys for times in different windows, got same key %q", k1)
	}
}

func TestWindowKey_Format(t *testing.T) {
	dur := time.Minute
	ts := time.Date(2024, 1, 1, 12, 5, 30, 0, time.UTC)
	windowStart := ts.Truncate(dur)

	key := windowKey("openai_chat", dur, ts)
	expected := "apibudget:openai_chat:1m0s:" + formatUnix(windowStart)

	if key != expected {
		t.Errorf("expected key %q, got %q", expected, key)
	}
}

func formatUnix(t time.Time) string {
	return fmt.Sprintf("%d", t.Unix())
}

func TestWindowKey_DifferentAPIs(t *testing.T) {
	dur := time.Minute
	ts := time.Date(2024, 1, 1, 12, 5, 30, 0, time.UTC)

	k1 := windowKey("api_a", dur, ts)
	k2 := windowKey("api_b", dur, ts)

	if k1 == k2 {
		t.Errorf("expected different keys for different API names, got same key %q", k1)
	}
}

func TestWindowKey_DifferentDurations(t *testing.T) {
	ts := time.Date(2024, 1, 1, 12, 5, 30, 0, time.UTC)

	k1 := windowKey("api_a", time.Minute, ts)
	k2 := windowKey("api_a", time.Hour, ts)

	if k1 == k2 {
		t.Errorf("expected different keys for different durations, got same key %q", k1)
	}
}

func TestWindowResetTime_AfterCurrentTime(t *testing.T) {
	dur := time.Minute
	ts := time.Date(2024, 1, 1, 12, 5, 30, 0, time.UTC)

	reset := windowResetTime(ts, dur)

	if !reset.After(ts) {
		t.Errorf("expected reset time %v to be after %v", reset, ts)
	}
}

func TestWindowResetTime_EqualsWindowStartPlusDuration(t *testing.T) {
	dur := time.Minute
	ts := time.Date(2024, 1, 1, 12, 5, 30, 0, time.UTC)

	reset := windowResetTime(ts, dur)
	expected := ts.Truncate(dur).Add(dur)

	if !reset.Equal(expected) {
		t.Errorf("expected reset time %v, got %v", expected, reset)
	}
}

func TestWindowResetTime_AtBoundary(t *testing.T) {
	dur := time.Minute
	// Exactly at a window boundary
	ts := time.Date(2024, 1, 1, 12, 5, 0, 0, time.UTC)

	reset := windowResetTime(ts, dur)
	expected := time.Date(2024, 1, 1, 12, 6, 0, 0, time.UTC)

	if !reset.Equal(expected) {
		t.Errorf("expected reset time %v, got %v", expected, reset)
	}
}

func TestWindowResetTime_HourDuration(t *testing.T) {
	dur := time.Hour
	ts := time.Date(2024, 1, 1, 12, 30, 0, 0, time.UTC)

	reset := windowResetTime(ts, dur)
	expected := time.Date(2024, 1, 1, 13, 0, 0, 0, time.UTC)

	if !reset.Equal(expected) {
		t.Errorf("expected reset time %v, got %v", expected, reset)
	}
}

// **Validates: Requirements 2.6, 2.7**
// Property 6: 窓キーの一意性
// For any API name and window duration, two times within the same window
// produce the same key, and two times in different windows produce different keys.
func TestProperty_WindowKeyUniqueness(t *testing.T) {
	// Predefined window durations to test against
	durations := []time.Duration{
		time.Second,
		time.Minute,
		time.Hour,
		24 * time.Hour,
	}

	// Sub-property 1: Two times within the same window produce the same key
	t.Run("SameWindowSameKey", func(t *testing.T) {
		f := func(apiNameSeed uint16, durIdx uint8, baseUnix int64, offsetNanos uint32) bool {
			// Generate a non-empty API name
			apiName := fmt.Sprintf("api_%d", apiNameSeed)

			// Pick a duration from the predefined set
			dur := durations[int(durIdx)%len(durations)]

			// Use a reasonable base time range (year 2000 to 2100)
			const minUnix = 946684800  // 2000-01-01
			const maxUnix = 4102444800 // 2100-01-01
			base := minUnix + (abs64(baseUnix) % (maxUnix - minUnix))
			baseTime := time.Unix(base, 0).UTC()

			// Truncate to window start, then generate two times within the same window
			windowStart := baseTime.Truncate(dur)
			windowDurNanos := dur.Nanoseconds()
			if windowDurNanos <= 0 {
				return true // skip degenerate case
			}

			// Two random offsets within [0, windowDuration)
			offset1 := int64(offsetNanos) % windowDurNanos
			offset2 := int64(rand.Int63()) % windowDurNanos

			t1 := windowStart.Add(time.Duration(offset1))
			t2 := windowStart.Add(time.Duration(offset2))

			k1 := windowKey(apiName, dur, t1)
			k2 := windowKey(apiName, dur, t2)

			return k1 == k2
		}

		if err := quick.Check(f, &quick.Config{MaxCount: 1000}); err != nil {
			t.Errorf("SameWindowSameKey property failed: %v", err)
		}
	})

	// Sub-property 2: Two times in different windows produce different keys
	t.Run("DifferentWindowDifferentKey", func(t *testing.T) {
		f := func(apiNameSeed uint16, durIdx uint8, baseUnix int64, windowOffset uint32) bool {
			// Generate a non-empty API name
			apiName := fmt.Sprintf("api_%d", apiNameSeed)

			// Pick a duration from the predefined set
			dur := durations[int(durIdx)%len(durations)]

			// Use a reasonable base time range
			const minUnix = 946684800  // 2000-01-01
			const maxUnix = 4102444800 // 2100-01-01
			base := minUnix + (abs64(baseUnix) % (maxUnix - minUnix))
			t1 := time.Unix(base, 0).UTC()

			// Ensure t2 is in a different window by adding at least 1 full window duration
			// windowOffset ensures we skip at least 1 window (1 to 100 windows ahead)
			skip := int64(windowOffset%100) + 1
			t2 := t1.Truncate(dur).Add(time.Duration(skip) * dur)

			// Verify they are indeed in different windows
			if t1.Truncate(dur).Equal(t2.Truncate(dur)) {
				return true // same window, skip (shouldn't happen with skip >= 1)
			}

			k1 := windowKey(apiName, dur, t1)
			k2 := windowKey(apiName, dur, t2)

			return k1 != k2
		}

		if err := quick.Check(f, &quick.Config{MaxCount: 1000}); err != nil {
			t.Errorf("DifferentWindowDifferentKey property failed: %v", err)
		}
	})
}

func abs64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}
