package apibudget

import (
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"testing/quick"
	"time"
)

// ================================================================
// Unit Tests for YAML Config (Task 11.5)
// Validates: Requirements 10.1, 10.2, 10.3, 10.4, 10.5
// ================================================================

func writeTestYAML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test YAML: %v", err)
	}
	return path
}

// TestLoadConfig_MinimalYAML tests parsing a minimal YAML config.
func TestLoadConfig_MinimalYAML(t *testing.T) {
	yaml := `
apis:
  - name: my_api
    windows:
      - duration: 1m
        limit: 60
`
	path := writeTestYAML(t, yaml)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.APIs) != 1 {
		t.Fatalf("expected 1 API, got %d", len(cfg.APIs))
	}
	if cfg.APIs[0].Name != "my_api" {
		t.Errorf("expected name=my_api, got %s", cfg.APIs[0].Name)
	}
	if len(cfg.APIs[0].Windows) != 1 {
		t.Fatalf("expected 1 window, got %d", len(cfg.APIs[0].Windows))
	}
	if cfg.APIs[0].Windows[0].Duration != time.Minute {
		t.Errorf("expected duration=1m, got %v", cfg.APIs[0].Windows[0].Duration)
	}
	if cfg.APIs[0].Windows[0].Limit != 60 {
		t.Errorf("expected limit=60, got %d", cfg.APIs[0].Windows[0].Limit)
	}
	if len(cfg.CreditPools) != 0 {
		t.Errorf("expected 0 credit pools, got %d", len(cfg.CreditPools))
	}
}

// TestLoadConfig_FullYAML tests parsing a full YAML config with all fields.
func TestLoadConfig_FullYAML(t *testing.T) {
	yaml := `
apis:
  - name: openai_chat
    windows:
      - duration: 1m
        limit: 60
      - duration: 24h
        limit: 10000
    buffer: 100ms
  - name: stripe_api
    windows:
      - duration: 1s
        limit: 25
credit_pools:
  - name: openai_credits
    max_credits: "1000"
    window: 720h
    initial: "500"
    costs:
      - api: openai_chat
        cost_per_call: "1.5"
        batch_size: 1
`
	path := writeTestYAML(t, yaml)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify APIs
	if len(cfg.APIs) != 2 {
		t.Fatalf("expected 2 APIs, got %d", len(cfg.APIs))
	}
	if cfg.APIs[0].Name != "openai_chat" {
		t.Errorf("expected first API name=openai_chat, got %s", cfg.APIs[0].Name)
	}
	if len(cfg.APIs[0].Windows) != 2 {
		t.Errorf("expected 2 windows for openai_chat, got %d", len(cfg.APIs[0].Windows))
	}
	if cfg.APIs[0].Buffer != 100*time.Millisecond {
		t.Errorf("expected buffer=100ms, got %v", cfg.APIs[0].Buffer)
	}
	if cfg.APIs[1].Name != "stripe_api" {
		t.Errorf("expected second API name=stripe_api, got %s", cfg.APIs[1].Name)
	}

	// Verify CreditPools
	if len(cfg.CreditPools) != 1 {
		t.Fatalf("expected 1 credit pool, got %d", len(cfg.CreditPools))
	}
	pool := cfg.CreditPools[0]
	if pool.Name != "openai_credits" {
		t.Errorf("expected pool name=openai_credits, got %s", pool.Name)
	}
	if pool.MaxCredits != "1000" {
		t.Errorf("expected max_credits=1000, got %s", pool.MaxCredits)
	}
	if pool.Window != 720*time.Hour {
		t.Errorf("expected window=720h, got %v", pool.Window)
	}
	if pool.Initial == nil || *pool.Initial != "500" {
		t.Errorf("expected initial=500, got %v", pool.Initial)
	}
	if len(pool.Costs) != 1 {
		t.Fatalf("expected 1 cost, got %d", len(pool.Costs))
	}
	if pool.Costs[0].API != "openai_chat" {
		t.Errorf("expected cost api=openai_chat, got %s", pool.Costs[0].API)
	}
	if pool.Costs[0].CostPerCall != "1.5" {
		t.Errorf("expected cost_per_call=1.5, got %s", pool.Costs[0].CostPerCall)
	}
	if pool.Costs[0].BatchSize != 1 {
		t.Errorf("expected batch_size=1, got %d", pool.Costs[0].BatchSize)
	}
}

// TestLoadConfig_InvalidYAML tests that invalid YAML returns an error.
func TestLoadConfig_InvalidYAML(t *testing.T) {
	yaml := `
apis:
  - name: [invalid yaml
`
	path := writeTestYAML(t, yaml)
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

// TestLoadConfig_FileNotFound tests that a missing file returns an error.
func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

// TestLoadConfig_MissingAPIName tests that missing API name returns a validation error.
func TestLoadConfig_MissingAPIName(t *testing.T) {
	yaml := `
apis:
  - windows:
      - duration: 1m
        limit: 60
`
	path := writeTestYAML(t, yaml)
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for missing API name")
	}
}

// TestLoadConfig_MissingWindows tests that missing windows returns a validation error.
func TestLoadConfig_MissingWindows(t *testing.T) {
	yaml := `
apis:
  - name: my_api
`
	path := writeTestYAML(t, yaml)
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for missing windows")
	}
}

// TestLoadConfig_MissingMaxCredits tests that missing max_credits returns a validation error.
func TestLoadConfig_MissingMaxCredits(t *testing.T) {
	yaml := `
apis:
  - name: my_api
    windows:
      - duration: 1m
        limit: 60
credit_pools:
  - name: pool1
    costs:
      - api: my_api
`
	path := writeTestYAML(t, yaml)
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for missing max_credits")
	}
}

// TestLoadConfig_MissingCosts tests that missing costs returns a validation error.
func TestLoadConfig_MissingCosts(t *testing.T) {
	yaml := `
apis:
  - name: my_api
    windows:
      - duration: 1m
        limit: 60
credit_pools:
  - name: pool1
    max_credits: "100"
`
	path := writeTestYAML(t, yaml)
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for missing costs")
	}
}

// TestLoadConfig_MissingPoolName tests that missing pool name returns a validation error.
func TestLoadConfig_MissingPoolName(t *testing.T) {
	yaml := `
apis:
  - name: my_api
    windows:
      - duration: 1m
        limit: 60
credit_pools:
  - max_credits: "100"
    costs:
      - api: my_api
`
	path := writeTestYAML(t, yaml)
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for missing pool name")
	}
}

// TestLoadConfig_NoAPIs tests that empty APIs returns a validation error.
func TestLoadConfig_NoAPIs(t *testing.T) {
	yaml := `
apis: []
`
	path := writeTestYAML(t, yaml)
	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for empty APIs")
	}
}

// TestLoadConfig_DefaultValues tests that default values are applied correctly.
func TestLoadConfig_DefaultValues(t *testing.T) {
	yaml := `
apis:
  - name: my_api
    windows:
      - duration: 1m
        limit: 60
credit_pools:
  - name: pool1
    max_credits: "100"
    costs:
      - api: my_api
`
	path := writeTestYAML(t, yaml)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Default CostPerCall should be "1"
	if cfg.CreditPools[0].Costs[0].CostPerCall != "1" {
		t.Errorf("expected default cost_per_call=1, got %s", cfg.CreditPools[0].Costs[0].CostPerCall)
	}
	// Default BatchSize should be 1
	if cfg.CreditPools[0].Costs[0].BatchSize != 1 {
		t.Errorf("expected default batch_size=1, got %d", cfg.CreditPools[0].Costs[0].BatchSize)
	}
	// Default Buffer should be 0
	if cfg.APIs[0].Buffer != 0 {
		t.Errorf("expected default buffer=0, got %v", cfg.APIs[0].Buffer)
	}
}

// TestToManagerConfig_Conversion tests that ToManagerConfig correctly converts Config to ManagerConfig.
func TestToManagerConfig_Conversion(t *testing.T) {
	yaml := `
apis:
  - name: api1
    windows:
      - duration: 1m
        limit: 60
    buffer: 200ms
credit_pools:
  - name: pool1
    max_credits: "500"
    window: 24h
    initial: "200"
    costs:
      - api: api1
        cost_per_call: "2.5"
        batch_size: 3
`
	path := writeTestYAML(t, yaml)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mcfg, err := cfg.ToManagerConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify APIs
	if len(mcfg.APIs) != 1 {
		t.Fatalf("expected 1 API, got %d", len(mcfg.APIs))
	}
	if mcfg.APIs[0].Name != "api1" {
		t.Errorf("expected name=api1, got %s", mcfg.APIs[0].Name)
	}
	if mcfg.APIs[0].Buffer != 200*time.Millisecond {
		t.Errorf("expected buffer=200ms, got %v", mcfg.APIs[0].Buffer)
	}
	if len(mcfg.APIs[0].Windows) != 1 {
		t.Fatalf("expected 1 window, got %d", len(mcfg.APIs[0].Windows))
	}
	if mcfg.APIs[0].Windows[0].Duration != time.Minute {
		t.Errorf("expected duration=1m, got %v", mcfg.APIs[0].Windows[0].Duration)
	}
	if mcfg.APIs[0].Windows[0].Limit != 60 {
		t.Errorf("expected limit=60, got %d", mcfg.APIs[0].Windows[0].Limit)
	}

	// Verify CreditPools
	if len(mcfg.CreditPools) != 1 {
		t.Fatalf("expected 1 pool, got %d", len(mcfg.CreditPools))
	}
	pool := mcfg.CreditPools[0]
	if pool.Name != "pool1" {
		t.Errorf("expected pool name=pool1, got %s", pool.Name)
	}
	if pool.MaxCredits.Cmp(MustNewCredit("500")) != 0 {
		t.Errorf("expected max_credits=500, got %s", pool.MaxCredits.String())
	}
	if pool.Window != 24*time.Hour {
		t.Errorf("expected window=24h, got %v", pool.Window)
	}
	if pool.Initial == nil {
		t.Fatal("expected initial to be set")
	}
	if pool.Initial.Cmp(MustNewCredit("200")) != 0 {
		t.Errorf("expected initial=200, got %s", pool.Initial.String())
	}
	if len(pool.Costs) != 1 {
		t.Fatalf("expected 1 cost, got %d", len(pool.Costs))
	}
	if pool.Costs[0].APIName != "api1" {
		t.Errorf("expected cost api=api1, got %s", pool.Costs[0].APIName)
	}
	if pool.Costs[0].CostPerCall.Cmp(MustNewCredit("2.5")) != 0 {
		t.Errorf("expected cost_per_call=2.5, got %s", pool.Costs[0].CostPerCall.String())
	}
	if pool.Costs[0].BatchSize != 3 {
		t.Errorf("expected batch_size=3, got %d", pool.Costs[0].BatchSize)
	}
}

// TestNewBudgetManagerFromYAML tests end-to-end YAML to BudgetManager creation.
func TestNewBudgetManagerFromYAML(t *testing.T) {
	yaml := `
apis:
  - name: test_api
    windows:
      - duration: 1m
        limit: 10
`
	path := writeTestYAML(t, yaml)
	m, err := NewBudgetManagerFromYAML(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil BudgetManager")
	}

	// Verify the manager works
	allowed, _ := m.Allow("test_api")
	if !allowed {
		t.Error("expected allowed=true")
	}
}

// TestNewBudgetManagerFromYAML_WithOptions tests YAML loading with ManagerOptions.
func TestNewBudgetManagerFromYAML_WithOptions(t *testing.T) {
	yaml := `
apis:
  - name: test_api
    windows:
      - duration: 1m
        limit: 10
`
	path := writeTestYAML(t, yaml)
	store := NewMemoryStore()
	m, err := NewBudgetManagerFromYAML(path, WithStore(store), WithLogLevel(LogLevelSilent))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.store != store {
		t.Error("expected custom store to be used")
	}
}

// TestToManagerConfig_InvalidMaxCredits tests that invalid max_credits returns an error.
func TestToManagerConfig_InvalidMaxCredits(t *testing.T) {
	yaml := `
apis:
  - name: api1
    windows:
      - duration: 1m
        limit: 60
credit_pools:
  - name: pool1
    max_credits: "invalid"
    costs:
      - api: api1
`
	path := writeTestYAML(t, yaml)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = cfg.ToManagerConfig()
	if err == nil {
		t.Fatal("expected error for invalid max_credits")
	}
}

// ================================================================
// Property 16: YAML設定のラウンドトリップ等価性 (Task 11.3)
// LoadConfig → ToManagerConfig produces same behavior as direct ManagerConfig construction.
// **Validates: Requirements 10.2, 10.3, 10.7**
// ================================================================

func TestProperty16_YAMLConfigRoundTripEquivalence(t *testing.T) {
	cfg := &quick.Config{MaxCount: 200}

	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		// Generate random config parameters
		limit := int64(rng.Intn(100)) + 1
		bufferMs := rng.Intn(1000)
		buffer := time.Duration(bufferMs) * time.Millisecond

		// Decide whether to include a credit pool
		hasPool := rng.Intn(2) == 1
		costVal := int64(rng.Intn(10)) + 1
		batchSize := int64(rng.Intn(5)) + 1
		maxCredits := int64(rng.Intn(1000)) + 100

		// Build YAML string
		yamlStr := "apis:\n"
		yamlStr += "  - name: round_trip_api\n"
		yamlStr += "    windows:\n"
		yamlStr += "      - duration: 1m\n"
		yamlStr += "        limit: " + itoa(limit) + "\n"
		if bufferMs > 0 {
			yamlStr += "    buffer: " + (time.Duration(bufferMs) * time.Millisecond).String() + "\n"
		}

		if hasPool {
			yamlStr += "credit_pools:\n"
			yamlStr += "  - name: round_trip_pool\n"
			yamlStr += "    max_credits: \"" + itoa(maxCredits) + "\"\n"
			yamlStr += "    costs:\n"
			yamlStr += "      - api: round_trip_api\n"
			yamlStr += "        cost_per_call: \"" + itoa(costVal) + "\"\n"
			yamlStr += "        batch_size: " + itoa(batchSize) + "\n"
		}

		// Path 1: Load from YAML
		dir := t.TempDir()
		path := filepath.Join(dir, "test.yaml")
		if err := os.WriteFile(path, []byte(yamlStr), 0644); err != nil {
			t.Logf("failed to write YAML: %v", err)
			return false
		}

		yamlCfg, err := LoadConfig(path)
		if err != nil {
			t.Logf("LoadConfig failed: %v", err)
			return false
		}
		mcfgFromYAML, err := yamlCfg.ToManagerConfig()
		if err != nil {
			t.Logf("ToManagerConfig failed: %v", err)
			return false
		}

		// Path 2: Build ManagerConfig directly
		directCfg := ManagerConfig{
			APIs: []RateConfig{
				{
					Name:    "round_trip_api",
					Windows: []Window{{Duration: time.Minute, Limit: limit}},
					Buffer:  buffer,
				},
			},
		}
		if hasPool {
			directCfg.CreditPools = []CreditPoolConfig{
				{
					Name:       "round_trip_pool",
					MaxCredits: NewCreditFromInt(maxCredits),
					Costs: []CreditCost{
						{
							APIName:     "round_trip_api",
							CostPerCall: NewCreditFromInt(costVal),
							BatchSize:   batchSize,
						},
					},
				},
			}
		}

		// Compare: both should produce equivalent BudgetManagers
		store1 := NewMemoryStore()
		store2 := NewMemoryStore()
		mcfgFromYAML.Store = store1
		directCfg.Store = store2
		mcfgFromYAML.LogLevel = LogLevelSilent
		directCfg.LogLevel = LogLevelSilent

		m1, err := NewBudgetManager(mcfgFromYAML)
		if err != nil {
			t.Logf("NewBudgetManager from YAML failed: %v", err)
			return false
		}
		m2, err := NewBudgetManager(directCfg)
		if err != nil {
			t.Logf("NewBudgetManager direct failed: %v", err)
			return false
		}

		// Both managers should behave identically for a sequence of Allow calls
		now := time.Now()
		for i := 0; i < int(limit)+5; i++ {
			a1, t1 := m1.AllowN("round_trip_api", 1, now)
			a2, t2 := m2.AllowN("round_trip_api", 1, now)
			if a1 != a2 {
				t.Logf("Allow mismatch at call %d: yaml=%v, direct=%v", i, a1, a2)
				return false
			}
			if a1 == false && !t1.Equal(t2) {
				t.Logf("nextAvailable mismatch at call %d: yaml=%v, direct=%v", i, t1, t2)
				return false
			}
		}

		// If pool exists, verify credits match
		if hasPool {
			c1, _ := m1.GetCredits("round_trip_pool")
			c2, _ := m2.GetCredits("round_trip_pool")
			if c1.Cmp(c2) != 0 {
				t.Logf("credit mismatch: yaml=%s, direct=%s", c1.String(), c2.String())
				return false
			}
		}

		return true
	}

	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("Property 16 (YAML round-trip equivalence) failed: %v", err)
	}
}

// itoa converts int64 to string without importing strconv.
func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	digits := make([]byte, 0, 20)
	for n > 0 {
		digits = append(digits, byte('0'+n%10))
		n /= 10
	}
	if neg {
		digits = append(digits, '-')
	}
	// reverse
	for i, j := 0, len(digits)-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}
	return string(digits)
}

// ================================================================
// Property 23: YAML必須フィールドバリデーション (Task 11.4)
// Missing required fields (name, windows, max_credits, costs) produce validation errors.
// **Validates: Requirements 10.5**
// ================================================================

func TestProperty23_YAMLRequiredFieldValidation(t *testing.T) {
	cfg := &quick.Config{MaxCount: 200}

	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		// Choose which required field to omit (0-4)
		omitField := rng.Intn(5)

		var yamlStr string
		switch omitField {
		case 0:
			// Omit API name
			yamlStr = "apis:\n  - windows:\n      - duration: 1m\n        limit: 60\n"
		case 1:
			// Omit windows
			yamlStr = "apis:\n  - name: test_api\n"
		case 2:
			// Omit pool name
			yamlStr = "apis:\n  - name: test_api\n    windows:\n      - duration: 1m\n        limit: 60\ncredit_pools:\n  - max_credits: \"100\"\n    costs:\n      - api: test_api\n"
		case 3:
			// Omit max_credits
			yamlStr = "apis:\n  - name: test_api\n    windows:\n      - duration: 1m\n        limit: 60\ncredit_pools:\n  - name: pool1\n    costs:\n      - api: test_api\n"
		case 4:
			// Omit costs
			yamlStr = "apis:\n  - name: test_api\n    windows:\n      - duration: 1m\n        limit: 60\ncredit_pools:\n  - name: pool1\n    max_credits: \"100\"\n"
		}

		dir := t.TempDir()
		path := filepath.Join(dir, "test.yaml")
		if err := os.WriteFile(path, []byte(yamlStr), 0644); err != nil {
			t.Logf("failed to write YAML: %v", err)
			return false
		}

		_, err := LoadConfig(path)
		if err == nil {
			t.Logf("expected validation error for omitField=%d, got nil", omitField)
			return false
		}
		return true
	}

	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("Property 23 (required field validation) failed: %v", err)
	}
}
