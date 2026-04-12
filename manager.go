package apibudget

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// SetLimit は指定APIの指定窓のリミットを動的に変更する。
// 未登録のAPI名の場合は ErrAPINotFound を返す。
// 指定された窓期間が見つからない場合はエラーを返す。
func (m *BudgetManager) SetLimit(apiName string, windowDuration time.Duration, newLimit int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	apiCfg, ok := m.apis[apiName]
	if !ok {
		return ErrAPINotFound
	}
	for i := range apiCfg.Windows {
		if apiCfg.Windows[i].Duration == windowDuration {
			apiCfg.Windows[i].Limit = newLimit
			return nil
		}
	}
	return fmt.Errorf("apibudget: window duration %v not found for API %q", windowDuration, apiName)
}

// Tokens は指定APIの現在利用可能なトークン数（最も厳しい窓基準）を返す。
// 未登録のAPI名の場合は0を返す。
func (m *BudgetManager) Tokens(apiName string) float64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	apiCfg, ok := m.apis[apiName]
	if !ok {
		return 0
	}
	ctx := context.Background()
	minTokens := float64(-1)
	now := time.Now()
	for _, w := range apiCfg.Windows {
		key := windowKey(apiName, w.Duration, now)
		count, _ := m.store.GetWindowCount(ctx, key)
		available := float64(w.Limit - count)
		if minTokens < 0 || available < minTokens {
			minTokens = available
		}
	}
	if minTokens < 0 {
		return 0
	}
	return minTokens
}

// Window はレート制限の時間窓を表す。
type Window struct {
	Duration time.Duration // 窓の長さ
	Limit    int64         // この窓内の最大リクエスト数
}

// RateConfig は1つのAPIエンドポイントのレート制限設定。
type RateConfig struct {
	Name    string        // 必須: API名
	Windows []Window      // 必須: 1つ以上の時間窓
	Buffer  time.Duration // オプション（デフォルト: 0）。誤差バッファ
}

// CreditCost は特定APIのクレジット消費ルールを定義する。
type CreditCost struct {
	CostPerCall Credit // オプション（デフォルト: "1"）。1回あたりのクレジット消費量
	APIName     string // 必須: 対象API名
	BatchSize   int64  // オプション（デフォルト: 1）。N回ごとに消費する場合のN
}

// CreditPoolConfig は共通クレジットプールの設定。
type CreditPoolConfig struct {
	Initial    *Credit       // プログラム開始時の残りクレジット（nilならMaxCredits）
	Name       string        // 必須: プール名
	MaxCredits Credit        // プールの上限
	Costs      []CreditCost  // 各APIの消費ルール
	Window     time.Duration // リセット周期（0なら手動リセットのみ）
}

// ManagerConfig はBudgetManagerの全体設定。
type ManagerConfig struct {
	Store       Store              // nilならNewMemoryStore()
	Logger      Logger             // nilならslog.Default()ベースのLogger
	APIs        []RateConfig       // APIごとのレート制限設定
	CreditPools []CreditPoolConfig // クレジットプール設定
	LogLevel    LogLevel           // デフォルト: LogLevelInfo
}

// creditPoolInfo はAPIからクレジットプールへのマッピング情報を保持する。
type creditPoolInfo struct {
	poolName string
	cost     CreditCost
}

// BudgetManager は複数APIのレート制限とクレジット管理を統合する。
type BudgetManager struct {
	store         Store
	logger        Logger
	apis          map[string]*RateConfig       // API名 → RateConfig
	pools         map[string]*CreditPoolConfig // プール名 → CreditPoolConfig
	apiToPool     map[string]*creditPoolInfo   // API名 → クレジットプール情報
	batchCounters map[string]int64             // API名 → バッチ消費用の累計リクエスト数
	cfg           ManagerConfig
	mu            sync.Mutex
}

// NewBudgetManager は設定からBudgetManagerを生成する。
// オプションフィールドが未設定の場合、デフォルト値を適用する。
func NewBudgetManager(cfg ManagerConfig) (*BudgetManager, error) {
	// デフォルト値の適用: Store
	if cfg.Store == nil {
		cfg.Store = NewMemoryStore()
	}

	// デフォルト値の適用: Logger
	if cfg.Logger == nil {
		cfg.Logger = newDefaultLogger(cfg.LogLevel)
	}

	// API名のバリデーションとマップ構築
	apis := make(map[string]*RateConfig, len(cfg.APIs))
	for i := range cfg.APIs {
		api := &cfg.APIs[i]
		if api.Name == "" {
			return nil, fmt.Errorf("apibudget: API name must not be empty (index %d)", i)
		}
		if _, exists := apis[api.Name]; exists {
			return nil, fmt.Errorf("apibudget: duplicate API name: %q", api.Name)
		}
		apis[api.Name] = api
	}

	// クレジットプールのバリデーション、デフォルト値適用、マップ構築
	pools := make(map[string]*CreditPoolConfig, len(cfg.CreditPools))
	apiToPool := make(map[string]*creditPoolInfo)

	for i := range cfg.CreditPools {
		pool := &cfg.CreditPools[i]
		if pool.Name == "" {
			return nil, fmt.Errorf("apibudget: credit pool name must not be empty (index %d)", i)
		}
		if _, exists := pools[pool.Name]; exists {
			return nil, fmt.Errorf("apibudget: duplicate credit pool name: %q", pool.Name)
		}

		// CreditCostのデフォルト値適用
		for j := range pool.Costs {
			cost := &pool.Costs[j]
			if cost.CostPerCall.IsZero() {
				cost.CostPerCall = MustNewCredit("1")
			}
			if cost.BatchSize == 0 {
				cost.BatchSize = 1
			}
			// APIからプールへのマッピングを登録
			apiToPool[cost.APIName] = &creditPoolInfo{
				poolName: pool.Name,
				cost:     *cost,
			}
		}

		pools[pool.Name] = pool
	}

	m := &BudgetManager{
		cfg:           cfg,
		store:         cfg.Store,
		logger:        cfg.Logger,
		apis:          apis,
		pools:         pools,
		apiToPool:     apiToPool,
		batchCounters: make(map[string]int64),
	}

	// クレジットプールの初期残高をストアに設定
	ctx := context.Background()
	for i := range cfg.CreditPools {
		pool := &cfg.CreditPools[i]
		var initialCredit Credit
		if pool.Initial != nil {
			initialCredit = *pool.Initial
		} else {
			initialCredit = pool.MaxCredits
		}
		if err := cfg.Store.SetCredit(ctx, pool.Name, initialCredit); err != nil {
			return nil, fmt.Errorf("apibudget: failed to set initial credit for pool %q: %w", pool.Name, err)
		}
	}

	cfg.Logger.Info("BudgetManager initialized",
		"api_count", len(cfg.APIs),
		"pool_count", len(cfg.CreditPools),
	)

	return m, nil
}

// Allow は指定APIに対して1回のリクエストが許可されるか即時判定する。
// 許可されない場合、次に利用可能な時刻を返す。
func (m *BudgetManager) Allow(apiName string) (bool, time.Time) {
	return m.AllowN(apiName, 1, time.Now())
}

// AllowN は指定APIに対してn回のリクエストが許可されるか即時判定する。
// 全ての時間窓でリクエスト数が上限以下であり、かつクレジットが十分な場合に(true, ゼロ値time.Time)を返す。
// いずれかの条件を満たさない場合、(false, nextAvailable)を返し、カウンタとクレジットを変更しない。
// 未登録のAPI名の場合は(false, ゼロ値time.Time)を返す。
func (m *BudgetManager) AllowN(apiName string, n int64, t time.Time) (bool, time.Time) {
	ctx := context.Background()

	// Step 0: API設定を検索。未登録なら(false, zero)を返す
	apiCfg, ok := m.apis[apiName]
	if !ok {
		return false, time.Time{}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var latestAvailable time.Time

	// Step 1: 全ての時間窓をチェック（最も厳しい制限を検出）
	for _, w := range apiCfg.Windows {
		key := windowKey(apiName, w.Duration, t)
		count, err := m.store.GetWindowCount(ctx, key)
		if err != nil {
			m.logger.Error("failed to get window count", "api", apiName, "window", w.Duration, "error", err)
			return false, time.Time{}
		}

		if count+n > w.Limit {
			nextReset := windowResetTime(t, w.Duration)
			nextAvail := nextReset.Add(apiCfg.Buffer)
			if nextAvail.After(latestAvailable) {
				latestAvailable = nextAvail
			}
		}
	}

	// いずれかの窓で超過していれば、状態変更なしで拒否
	if !latestAvailable.IsZero() {
		return false, latestAvailable
	}

	// Step 2: クレジットプールをチェック・消費
	if poolInfo, hasPool := m.apiToPool[apiName]; hasPool {
		totalCallsBefore := m.batchCounters[apiName]
		consumeAmount, totalCallsAfter := calculateConsumption(poolInfo.cost, n, totalCallsBefore)
		if !consumeAmount.IsZero() {
			_, err := m.store.DeductCredit(ctx, poolInfo.poolName, consumeAmount)
			if err != nil {
				// クレジット不足
				pool := m.pools[poolInfo.poolName]
				var nextReset time.Time
				if pool.Window > 0 {
					nextReset = windowResetTime(t, pool.Window)
				}
				return false, nextReset
			}
		}
		// 成功時にバッチカウンタを更新
		if poolInfo.cost.BatchSize > 1 {
			m.batchCounters[apiName] = totalCallsAfter
		}
	}

	// Step 3: 全窓のカウンタを増加
	for _, w := range apiCfg.Windows {
		key := windowKey(apiName, w.Duration, t)
		if _, err := m.store.IncrementWindow(ctx, key, n, w.Duration); err != nil {
			m.logger.Error("failed to increment window", "api", apiName, "window", w.Duration, "error", err)
		}
	}

	return true, time.Time{}
}

// calculateConsumption はN回のリクエストに対するクレジット消費量を計算する。
// BatchSize <= 1 の場合は毎回消費（n * CostPerCall）。
// BatchSize > 1 の場合: 総リクエスト数に基づくバッチ数の増分 × CostPerCall を返す。
// totalCallsBefore は今までの累計リクエスト数。
// 戻り値: (消費クレジット量, 新しい累計リクエスト数)
func calculateConsumption(cost CreditCost, n int64, totalCallsBefore int64) (Credit, int64) {
	if cost.BatchSize <= 1 {
		return cost.CostPerCall.Mul(n), 0 // totalCalls not used for BatchSize=1
	}
	totalCallsAfter := totalCallsBefore + n

	// ceil(totalCallsBefore / B) と ceil(totalCallsAfter / B) の差分が新しいバッチ数
	batchesBefore := ceilDiv(totalCallsBefore, cost.BatchSize)
	batchesAfter := ceilDiv(totalCallsAfter, cost.BatchSize)
	newBatches := batchesAfter - batchesBefore

	return cost.CostPerCall.Mul(newBatches), totalCallsAfter
}

// ceilDiv は整数の切り上げ除算を行う。
func ceilDiv(a, b int64) int64 {
	if a == 0 {
		return 0
	}
	return (a + b - 1) / b
}

// GetCredits はプールの現在のクレジット残高を返す。
// 未登録プール名の場合は ErrPoolNotFound を返す。
func (m *BudgetManager) GetCredits(poolName string) (Credit, error) {
	if _, ok := m.pools[poolName]; !ok {
		return Credit{}, ErrPoolNotFound
	}
	return m.store.GetCredit(context.Background(), poolName)
}

// ResetCredits はプールのクレジットをMaxCreditsにリセットする。
// 未登録プール名の場合は ErrPoolNotFound を返す。
func (m *BudgetManager) ResetCredits(poolName string) error {
	pool, ok := m.pools[poolName]
	if !ok {
		return ErrPoolNotFound
	}
	return m.store.SetCredit(context.Background(), poolName, pool.MaxCredits)
}

// AddCredits はプールにクレジットを追加する。
// 未登録プール名の場合は ErrPoolNotFound を返す。
func (m *BudgetManager) AddCredits(poolName string, amount Credit) error {
	if _, ok := m.pools[poolName]; !ok {
		return ErrPoolNotFound
	}
	_, err := m.store.AddCredit(context.Background(), poolName, amount)
	return err
}

// SetCredits はプールのクレジットを指定値に設定する。
// 未登録プール名の場合は ErrPoolNotFound を返す。
func (m *BudgetManager) SetCredits(poolName string, amount Credit) error {
	if _, ok := m.pools[poolName]; !ok {
		return ErrPoolNotFound
	}
	return m.store.SetCredit(context.Background(), poolName, amount)
}

// Wait はレートが解除されるまでブロッキングし、1回分のリクエスト権を取得する。
// コンテキストがキャンセルまたはデッドライン超過した場合、エラーを返しカウンタ・クレジットを変更しない。
func (m *BudgetManager) Wait(ctx context.Context, apiName string) error {
	return m.WaitN(ctx, apiName, 1)
}

// WaitN はn回分のリクエスト権が取得できるまでブロッキングする。
// コンテキストがキャンセルまたはデッドライン超過した場合、エラーを返しカウンタ・クレジットを変更しない。
func (m *BudgetManager) WaitN(ctx context.Context, apiName string, n int64) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		allowed, nextAvailable := m.AllowN(apiName, n, time.Now())
		if allowed {
			return nil
		}
		waitDuration := time.Until(nextAvailable)
		if waitDuration <= 0 {
			continue
		}
		timer := time.NewTimer(waitDuration)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// Reserve は1回分のリクエスト権を予約する。常にnon-nilのReservationを返す。
func (m *BudgetManager) Reserve(apiName string) *Reservation {
	return m.ReserveN(apiName, 1, time.Now())
}

// ReserveN はn回分のリクエスト権を予約する。常にnon-nilのReservationを返す。
// 予約が成功した場合、レート枠とクレジットが仮確保される。
// 予約が失敗した場合（API未登録、レート超過等）、ok=falseのReservationを返す。
func (m *BudgetManager) ReserveN(apiName string, n int64, t time.Time) *Reservation {
	ctx := context.Background()

	// API設定を検索。未登録ならok=falseのReservationを返す
	apiCfg, found := m.apis[apiName]
	if !found {
		return &Reservation{
			manager: m,
			apiName: apiName,
			n:       n,
			ok:      false,
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var latestAvailable time.Time

	// 全ての時間窓をチェック
	for _, w := range apiCfg.Windows {
		key := windowKey(apiName, w.Duration, t)
		count, err := m.store.GetWindowCount(ctx, key)
		if err != nil {
			return &Reservation{manager: m, apiName: apiName, n: n, ok: false}
		}
		if count+n > w.Limit {
			nextReset := windowResetTime(t, w.Duration)
			nextAvail := nextReset.Add(apiCfg.Buffer)
			if nextAvail.After(latestAvailable) {
				latestAvailable = nextAvail
			}
		}
	}

	if !latestAvailable.IsZero() {
		delay := latestAvailable.Sub(t)
		if delay < 0 {
			delay = 0
		}
		return &Reservation{
			manager: m,
			apiName: apiName,
			n:       n,
			ok:      false,
			delay:   delay,
		}
	}

	// クレジットプールをチェック・消費
	var reservedCost Credit
	var poolName string
	if poolInfo, hasPool := m.apiToPool[apiName]; hasPool {
		poolName = poolInfo.poolName
		totalCallsBefore := m.batchCounters[apiName]
		consumeAmount, totalCallsAfter := calculateConsumption(poolInfo.cost, n, totalCallsBefore)
		if !consumeAmount.IsZero() {
			_, err := m.store.DeductCredit(ctx, poolName, consumeAmount)
			if err != nil {
				return &Reservation{manager: m, apiName: apiName, n: n, ok: false, poolName: poolName}
			}
		}
		reservedCost = consumeAmount
		// 成功時にバッチカウンタを更新
		if poolInfo.cost.BatchSize > 1 {
			m.batchCounters[apiName] = totalCallsAfter
		}
	}

	// 全窓のカウンタを増加し、キャンセル用の情報を保存
	var windowKeys []windowKeyInfo
	for _, w := range apiCfg.Windows {
		key := windowKey(apiName, w.Duration, t)
		if _, err := m.store.IncrementWindow(ctx, key, n, w.Duration); err != nil {
			m.logger.Error("failed to increment window", "api", apiName, "window", w.Duration, "error", err)
		}
		windowKeys = append(windowKeys, windowKeyInfo{key: key, duration: w.Duration})
	}

	return &Reservation{
		mu:           sync.Mutex{},
		manager:      m,
		apiName:      apiName,
		poolName:     poolName,
		n:            n,
		ok:           true,
		delay:        0,
		reservedCost: reservedCost,
		windowKeys:   windowKeys,
	}
}
