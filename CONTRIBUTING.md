# Contributing / 開発フロー

本リポジトリは Design Doc 駆動で開発する。実装（人間 / LLM 問わず）は
`docs/design_docs/` の設計に従い、以下のフローで PR を作成する。

## 基本原則

- **Design Doc が先、実装が後**。実装は必ず対応する Design Doc セクションに紐づく。
- **main 直 push 禁止**。全変更は PR 経由。CI green を merge 条件とする。
- **1 PR = 1 関心事**。Design Doc のマイルストーン / セクション単位でスコープを切る。

## PR 分割方針

Design Doc のマイルストーン（M1/M2/M3…）とセクションを PR スコープの単位とする。
1 マイルストーンが大きい場合は関心事で分割する。

例（M1 – PoC 疎通）:

- PR1: プロジェクト骨組み（`go.mod` / ディレクトリ / `internal/config` /
  `internal/domain` port / `internal/application` RunBatch + テスト）
- PR2: adapter 実装（`adapter/in/helloworld` / `adapter/out/quote0` + httptest テスト）
  + `cmd/batch/main.go` 配線
- PR3: CI/CD（`.github/workflows/ci.yml` / `batch.yml`）+ `README.md`

## PR 作成ルール

- **タイトル**: Conventional Commits 形式（`feat:` / `fix:` / `docs:` / `chore:` …）。
- **本文に必須**:
  - 対応 Design Doc セクションへのリンク（例: `docs/design_docs/0001-initial-setup.md#actual-design`）
  - このPRのスコープ（何を含み、何を含まないか）
  - 動作確認方法（テスト / 手動確認手順）
- **スコープ外の変更を混ぜない**。ついで修正は別 PR。

## レビュー観点

- 実装が Design Doc の設計意図と乖離していないか（最重要）。
- CI（lint + test）が green か。
- secret（`DOT_API_KEY` / `SERIAL_NUM`）がコード・ログ・エラー・PR 差分に平文で
  含まれていないか。

## CI / マージ条件

- PR / push で `ci.yml`（golangci-lint + `go test ./...`）が走る。secrets 不要。
- 実送信ワークフロー `batch.yml` は secrets を使う → `workflow_dispatch` / `schedule` のみ。
- CI green を merge 必須条件とする（ブランチ保護推奨）。

## secret / 環境変数

- 秘匿情報はコミットしない。ローカルは `.env`（gitignore 対象）、CI は GitHub Secrets。
- 必要な env キーは `.env.example`（値は空）で共有。
- 詳細は `docs/design_docs/0001-initial-setup.md` の「設定 / シークレットの流れ」節を参照。

## LLM 実装者への指示テンプレ（参考）

> 対象: `docs/design_docs/0001-initial-setup.md` の <セクション/マイルストーン>。
> スコープ: <含む範囲>。<含まない範囲> は別 PR。
> 完了条件: `go test ./...` green、対応 Design Doc セクションを PR 本文にリンク。
> secret を平文でコミット/出力しないこと。
