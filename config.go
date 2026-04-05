package apibudget

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config はYAMLファイルの構造体マッピング。
// YAMLにはAPIとクレジットプールの定義のみを含む。
type Config struct {
	APIs        []APIConfig      `yaml:"apis"`
	CreditPools []CreditPoolYAML `yaml:"credit_pools"`
}

// APIConfig はYAML内の1つのAPIエンドポイント設定。
type APIConfig struct {
	Name    string         `yaml:"name"`
	Windows []WindowConfig `yaml:"windows"`
	Buffer  time.Duration  `yaml:"buffer"`
}

// WindowConfig はYAML内の時間窓設定。
type WindowConfig struct {
	Duration time.Duration `yaml:"duration"`
	Limit    int64         `yaml:"limit"`
}

// CreditPoolYAML はYAML内のクレジットプール設定。
type CreditPoolYAML struct {
	Name       string           `yaml:"name"`
	MaxCredits string           `yaml:"max_credits"`
	Window     time.Duration    `yaml:"window"`
	Initial    *string          `yaml:"initial"`
	Costs      []CreditCostYAML `yaml:"costs"`
}

// CreditCostYAML はYAML内のクレジット消費ルール。
type CreditCostYAML struct {
	API         string `yaml:"api"`
	CostPerCall string `yaml:"cost_per_call"`
	BatchSize   int64  `yaml:"batch_size"`
}

// LoadConfig はYAMLファイルを読み込みConfigを返す。
// オプションフィールドが省略されている場合、デフォルト値を適用する。
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("apibudget: failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("apibudget: failed to parse YAML: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	// デフォルト値の適用
	for i := range cfg.CreditPools {
		for j := range cfg.CreditPools[i].Costs {
			if cfg.CreditPools[i].Costs[j].CostPerCall == "" {
				cfg.CreditPools[i].Costs[j].CostPerCall = "1"
			}
			if cfg.CreditPools[i].Costs[j].BatchSize == 0 {
				cfg.CreditPools[i].Costs[j].BatchSize = 1
			}
		}
	}

	return &cfg, nil
}

// validate はConfigのバリデーションを行う。
func (cfg *Config) validate() error {
	// バリデーション: APIs
	if len(cfg.APIs) == 0 {
		return fmt.Errorf("apibudget: config must have at least one API")
	}
	for i, api := range cfg.APIs {
		if api.Name == "" {
			return fmt.Errorf("apibudget: api[%d].name is required", i)
		}
		if len(api.Windows) == 0 {
			return fmt.Errorf("apibudget: api[%d] (%q).windows is required", i, api.Name)
		}
	}

	// バリデーション: CreditPools
	for i, pool := range cfg.CreditPools {
		if pool.Name == "" {
			return fmt.Errorf("apibudget: credit_pools[%d].name is required", i)
		}
		if pool.MaxCredits == "" {
			return fmt.Errorf("apibudget: credit_pools[%d] (%q).max_credits is required", i, pool.Name)
		}
		if len(pool.Costs) == 0 {
			return fmt.Errorf("apibudget: credit_pools[%d] (%q).costs is required", i, pool.Name)
		}
	}

	return nil
}

// ToManagerConfig はConfigをManagerConfigに変換する。
// ストア・ログの設定はManagerOptionで別途指定する。
func (c *Config) ToManagerConfig() (ManagerConfig, error) {
	var mcfg ManagerConfig

	// APIs変換
	for _, api := range c.APIs {
		rc := RateConfig{
			Name:   api.Name,
			Buffer: api.Buffer,
		}
		for _, w := range api.Windows {
			rc.Windows = append(rc.Windows, Window{
				Duration: w.Duration,
				Limit:    w.Limit,
			})
		}
		mcfg.APIs = append(mcfg.APIs, rc)
	}

	// CreditPools変換
	for _, pool := range c.CreditPools {
		maxCredits, err := NewCredit(pool.MaxCredits)
		if err != nil {
			return ManagerConfig{}, fmt.Errorf("apibudget: invalid max_credits %q for pool %q: %w", pool.MaxCredits, pool.Name, err)
		}

		pc := CreditPoolConfig{
			Name:       pool.Name,
			MaxCredits: maxCredits,
			Window:     pool.Window,
		}

		if pool.Initial != nil {
			initial, err := NewCredit(*pool.Initial)
			if err != nil {
				return ManagerConfig{}, fmt.Errorf("apibudget: invalid initial %q for pool %q: %w", *pool.Initial, pool.Name, err)
			}
			pc.Initial = &initial
		}

		for _, cost := range pool.Costs {
			costPerCall, err := NewCredit(cost.CostPerCall)
			if err != nil {
				return ManagerConfig{}, fmt.Errorf("apibudget: invalid cost_per_call %q for api %q in pool %q: %w", cost.CostPerCall, cost.API, pool.Name, err)
			}
			pc.Costs = append(pc.Costs, CreditCost{
				APIName:     cost.API,
				CostPerCall: costPerCall,
				BatchSize:   cost.BatchSize,
			})
		}

		mcfg.CreditPools = append(mcfg.CreditPools, pc)
	}

	return mcfg, nil
}

// ManagerOption はBudgetManagerの追加設定オプション。
type ManagerOption func(*ManagerConfig)

// WithStore はカスタムStoreを設定する。
func WithStore(s Store) ManagerOption {
	return func(cfg *ManagerConfig) {
		cfg.Store = s
	}
}

// WithLogger はカスタムLoggerを設定する。
func WithLogger(l Logger) ManagerOption {
	return func(cfg *ManagerConfig) {
		cfg.Logger = l
	}
}

// WithLogLevel はログレベルを設定する。
func WithLogLevel(level LogLevel) ManagerOption {
	return func(cfg *ManagerConfig) {
		cfg.LogLevel = level
	}
}

// NewBudgetManagerFromYAML はYAMLファイルからBudgetManagerを生成する。
func NewBudgetManagerFromYAML(path string, opts ...ManagerOption) (*BudgetManager, error) {
	cfg, err := LoadConfig(path)
	if err != nil {
		return nil, err
	}

	mcfg, err := cfg.ToManagerConfig()
	if err != nil {
		return nil, err
	}

	for _, opt := range opts {
		opt(&mcfg)
	}

	return NewBudgetManager(mcfg)
}
