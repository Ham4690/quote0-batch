# CLAUDE.md

本リポジトリで作業する LLM / 自動化エージェント(Claude Code / GitHub Action 含む)は、
規約の一次情報として **[`AGENTS.md`](AGENTS.md)** に従うこと。本ファイルは索引であり、
詳細は AGENTS.md および参照先(`README.md` / `CONTRIBUTING.md` / `docs/design-docs/`)に集約する。

## 最重要ルール(詳細は AGENTS.md)

- **Design Doc が先、実装が後**。実装は `docs/design-docs/` の対応セクションに紐づける。
- **main 直 push 禁止**。全変更は作業ブランチ → PR 経由。**CI green を merge 条件**とする。
- **1 PR = 1 関心事**。ついで修正は別 PR に分ける。
- アーキテクチャは Ports & Adapters。依存方向は常に **外側 → 内側**(`domain` は無依存)。
- 完了前に green を確認: `go build ./...` / `go test ./...` / `go vet ./...` / `gofmt -l .` / `golangci-lint run`。
