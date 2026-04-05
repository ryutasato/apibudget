# apibudget

外部APIのレート制限と予算管理を統合するGoライブラリ。

## 概要

apibudgetは、外部APIのレート制限とクレジット/トークン予算管理をローカルで統合的に行うGoライブラリです。`golang.org/x/time/rate`のAllow/Reserve/Waitパターンを踏襲しつつ、以下の機能を提供します：

- APIごとに複数の時間窓（秒/分/時/日/月）でレート制限
- 複数APIで共有されるクレジットプール管理
- バッチ消費サポート
- Redis/インメモリバックエンド
- HTTP APIサーバーモード
- YAML設定

## 主な機能

- **マルチウィンドウレート制限** — 1つのAPIに複数の時間窓を同時適用
- **クレジットプール** — `big.Rat`精度で複数APIの共有予算を管理
- **Allow/Wait/Reserve** — `golang.org/x/time/rate`互換のAPIパターン
- **予約とConfirm/Cancel** — 事前にクレジットを仮確保し、実行後に実際の消費量で確定
- **バッチ消費** — N回ごとにクレジットを消費する設定
- **Redisバックエンド** — Luaスクリプトによるアトミック操作で分散環境対応
- **APIサーバー** — REST APIでGo以外の言語からも利用可能
- **Docker対応** — distrolessコンテナとdocker-compose構成

## クイック例

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/ryutasato/apibudget"
)

func main() {
    mgr, _ := apibudget.NewBudgetManager(apibudget.ManagerConfig{
        APIs: []apibudget.RateConfig{
            {
                Name: "openai",
                Windows: []apibudget.Window{
                    {Duration: time.Minute, Limit: 60},
                },
            },
        },
    })

    allowed, next := mgr.Allow("openai")
    if allowed {
        fmt.Println("リクエスト許可")
    } else {
        fmt.Printf("レート制限中、%v 後にリトライ\n", next)
    }

    _ = mgr.Wait(context.Background(), "openai")
    fmt.Println("待機後にリクエスト実行")
}
```

## ライセンス

MIT
