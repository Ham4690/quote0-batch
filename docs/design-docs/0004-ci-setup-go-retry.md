# Design Doc: batch.yml の actions/setup-go ダウンロード失敗対策

- **Author:** Arata Higashiguchi
- **Status:** Draft
- **Created:** 2026-09-05
- **Last Updated:** 2026-09-05
- **Reviewers:** TBD
- **関連:** [0002 天気連携 Design Doc](./0002-weather-integration.md)（現行 cron 頻度の設計根拠）

---

## Context and Scope

`batch.yml` は cron `"0 15 * * *"`（UTC15:00 = JST0:00、毎日1回）で quote/0 へ実送信している。
先日、以下のインシデントが発生した:

> GitHub Actions Runner が `actions/setup-go` を GitHub からダウンロードしようとしたところ、
> GitHub側から 503 → 429 → 503 が返り、3回リトライして失敗した。

これは GitHub 側のアクション配布インフラの一時的な障害であり、ジョブの各ステップが実行される
**前**（`uses:` で指定したアクション本体を runner が取得する段階）で発生する。そのため、
ステップ内のシェルスクリプトでリトライ処理を書いても対処できない。

当初「cron を毎時実行に変更してはどうか」という案が出たが、検討の結果却下した
（[Alternatives Considered](#alternatives-considered) A 参照）。cron 頻度は
[0002](./0002-weather-integration.md#実行頻度の変更batchyml) の設計判断のまま維持し、
`setup-go` ステップ自体にリトライ耐性を持たせることで根本原因に対処する。

## Goals

- **G1**: `actions/setup-go` のダウンロードが一時的に失敗しても、同一ジョブ内で自動的に
  再試行し、ジョブ全体の失敗を防ぐ。
- **G2**: 新規の外部依存（サードパーティ action）を追加せず、既存の pin 方針
  （commit SHA 固定）を維持する。
- **G3**: cron 頻度・毎日1回という既存の運用設計（[0002](./0002-weather-integration.md)）を
  変更しない。

## Non-Goals

- cron を毎時実行に変更すること（[Alternatives](#alternatives-considered) A で却下）。
- `ci.yml` 側の同様のリスクへの対処。`ci.yml` は PR/push 毎に実行され secrets を使わないため
  影響度・対応の緊急性が異なる。必要なら別 PR で扱う（[Open Questions](#open-questions)）。
- `actions/checkout` のダウンロード失敗対策。今回のインシデント報告は `setup-go` のみが
  対象であり、スコープを絞る。

---

## Actual Design

`batch.yml` の `setup-go` ステップを、1回目の失敗を許容しつつ2回目で再試行する2ステップ構成に
分割する。

```yaml
- uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
- name: Setup Go
  id: setup-go
  continue-on-error: true
  uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0
  with:
    go-version: "1.25"
    cache: false # 外部依存(go.sum)未生成のためキャッシュ無効
- name: Setup Go (retry)
  if: steps.setup-go.outcome == 'failure'
  uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0
  with:
    go-version: "1.25"
    cache: false
```

- 1回目のステップに `id: setup-go` と `continue-on-error: true` を付与し、失敗してもジョブを
  即座に止めない。
- 2回目のステップは `if: steps.setup-go.outcome == 'failure'` で、1回目が失敗した場合のみ
  実行される。1回目が成功していればスキップされる（コスト増は失敗時のみ）。
- 両ステップとも既存と同じ commit SHA を再利用するため、新規の pin 管理は発生しない。
- `timeout-minutes` を `5` → `10` に引き上げる。1回目の失敗検知＋2回目のダウンロードに要する
  時間の余裕を確保するため（`ci.yml` も `10` で揃っている）。

## Alternatives Considered

### A. cron を毎時実行に変更する

却下。理由:

1. 原因は GitHub 側のアクション配布インフラの一時障害であり、cron 頻度とは無関係。毎時化しても
   根本原因は解決しない。
2. 現行コードは「当日の天気予報を1日1回取得・送信する」設計（[0002](./0002-weather-integration.md)）。
   毎時化すると同一内容を1日24回送信することになり、重複送信を避けるコード変更が別途
   必要になり影響範囲が広がる。
3. [0002](./0002-weather-integration.md#実行頻度の変更batchyml) で「毎時→毎日」へ意図的に
   低頻度化した設計判断を、無関係な理由で覆すことになる。

### B. サードパーティ retry action の導入

却下。本リポジトリは `actions/checkout` / `actions/setup-go` を commit SHA 固定・最小依存に
保つ方針（`golangci-lint-action` も同様に pin 済み）。新たにサードパーティの retry action
（例: `Wandalen/wretry.action`）を導入すると、サプライチェーン面の追加検討・pin 管理コストが
発生する。まずは追加依存なしのネイティブなステップ二重化で対処し、それでも不十分な場合に
再検討する。

### C. 何もしない（`workflow_dispatch` での手動再実行に依存）

却下。深夜（JST 0 時）の実行失敗に人間が気づくのは翌朝以降になりやすく、最大24時間表示が
更新されない可能性がある。

## Cross-Cutting Concerns

### コスト

失敗時のみ2回目の `setup-go` が実行されるため、通常時のコスト増はない。パブリックリポジトリの
Actions 分は無料枠のため、失敗時の追加コストも無視できる。

### 可観測性 / 失敗時挙動

2回目も失敗した場合はジョブ全体が失敗し、既存同様 GitHub Actions の失敗通知（Actions タブ /
通知設定）で検知する。追加のアラート機構は本 Design Doc のスコープ外。

## Open Questions

- `ci.yml` の `setup-go` ステップにも同様のリトライを追加するかは、今回のインシデントが
  `batch.yml`（`schedule` 実行）でのみ観測されたことを踏まえ、別途要否を判断する。
