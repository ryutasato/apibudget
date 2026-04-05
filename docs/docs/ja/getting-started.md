# クイックスタート

## インストール

```bash
go get github.com/ryutasato/apibudget
```

## 基本的な使い方

### 1. BudgetManagerの作成

```go
mgr, err := apibudget.NewBudgetManager(apibudget.ManagerConfig{
    APIs: []apibudget.RateConfig{
        {
            Name: "my-api",
            Windows: []apibudget.Window{
                {Duration: time.Minute, Limit: 60},
                {Duration: time.Hour, Limit: 1000},
            },
            Buffer: 500 * time.Millisecond,
        },
    },
})
```

### 2. レート制限チェック

`Allow`でノンブロッキングチェック：

```go
allowed, nextAvailable := mgr.Allow("my-api")
if !allowed {
    fmt.Printf("リトライ時刻: %v\n", nextAvailable)
}
```

`Wait`でブロッキング待機：

```go
err := mgr.Wait(ctx, "my-api")
if err != nil {
    // コンテキストキャンセルまたはデッドライン超過
}
```

### 3. 予約の使用

実際のコストが事前に不明なAPI（LLMトークン使用量など）向け：

```go
r := mgr.Reserve("my-api")
if r.OK() {
    // API呼び出し実行...
    actualCost := apibudget.MustNewCredit("3.5")
    err := r.Confirm(actualCost)
} else {
    r.Cancel()
}
```

### 4. クレジットプールの追加

```go
mgr, err := apibudget.NewBudgetManager(apibudget.ManagerConfig{
    APIs: []apibudget.RateConfig{
        {Name: "openai", Windows: []apibudget.Window{{Duration: time.Minute, Limit: 60}}},
    },
    CreditPools: []apibudget.CreditPoolConfig{
        {
            Name:       "llm-budget",
            MaxCredits: apibudget.MustNewCredit("100000"),
            Costs: []apibudget.CreditCost{
                {APIName: "openai", CostPerCall: apibudget.MustNewCredit("1.5")},
            },
        },
    },
})
```

## YAML設定

`apibudget.yaml`ファイルを作成：

```yaml
apis:
  - name: openai
    windows:
      - duration: 1m
        limit: 60
      - duration: 1h
        limit: 1000
    buffer: 500ms

credit_pools:
  - name: llm-budget
    max_credits: "100000"
    costs:
      - api: openai
        cost_per_call: "1.5"
```

読み込み：

```go
mgr, err := apibudget.NewBudgetManagerFromYAML("apibudget.yaml")
```

詳細は[設定リファレンス](../configuration.md)を参照してください。
