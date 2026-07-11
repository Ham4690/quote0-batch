# Design Doc: quote0-batch 初期構築

- **Author:** Arata Higashiguchi
- **Status:** Draft
- **Created:** 2026-07-11
- **Last Updated:** 2026-07-11
- **Reviewers:** TBD

---

## Context and Scope

quote/0 は e-ink スマートディスプレイ。REST API 経由でテキスト / 画像 / キャンバスを送信し、デバイス上に表示できる。

本リポジトリ `quote0-batch` は、外部コンテンツ（天気・カレンダー・RSS 等、将来拡張）を定期的に取得・整形し、quote/0 の Text API へ連携する **バッチ処理基盤** を管理する。

本 Design Doc のスコープは **初期構築のみ**:

1. 技術選定（実行ランタイム・スケジューラ・シークレット管理）
2. CI/CD 上で Text API を叩き `Hello World` を実機表示する疎通確認（PoC）

将来のコンテンツソース連携ロジックそのものは対象外（Non-Goals 参照）。ただし後続で拡張可能な構造を初期構築時に用意する。

### quote/0 Text API（前提知識）

| 項目 | 内容 |
| --- | --- |
| Method | `POST` |
| Endpoint | `https://dot.mindreset.tech/api/authV2/open/device/:deviceId/text` |
| Auth | `Authorization: Bearer {API_KEY}` |
| Body (主要) | `title`, `message`(`\n`/`\t`可), `signature`, `icon`, `link`, `refreshNow`, `taskKey`, `taskAlias`, `styles` |
| Response | `{"message": "Device {deviceId} text API content switched."}` |
| 主要エラー | 400 パラメータ不正 / 403 権限不足 / 404 デバイス未検出・Text API未登録 / 500 デバイス通信失敗 |

`API_KEY`（secret）と `deviceId`（シリアルナンバー）は取得済み。両者とも秘匿情報として扱う。

---

## Goals

- **G1**: Text API に対し `Hello World` を送信し、実機表示を確認できる。
- **G2**: API Key / deviceId を平文でコミットせず、安全に注入できる仕組みを持つ。
- **G3**: 定期実行（cron スケジュール）の土台を用意する。
- **G4**: ローカルでも CI 上でも同一コードで実行できる（環境差分は環境変数のみ）。
- **G5**: 後続でコンテンツソースを追加しやすいディレクトリ / モジュール構成にする。

## Non-Goals

- 実コンテンツ（天気・カレンダー等）の取得ロジック。別 Design Doc で扱う。
- 画像 / キャンバス API 連携。
- 複数デバイス・マルチテナント対応。
- 監視・アラートの本格構築（初期は失敗通知の最小限のみ検討）。
- Web UI / 管理画面。

---

## Actual Design

### システムコンテキスト

```
┌──────────────┐   cron trigger   ┌────────────────────┐
│  Scheduler   │ ───────────────▶ │   Batch Runtime     │
│ (GH Actions) │                  │  (Node.js script)   │
└──────────────┘                  └─────────┬──────────┘
        │ inject secrets                    │ POST /device/:id/text
        │ (API_KEY, DEVICE_ID)              ▼
        │                          ┌────────────────────┐
        └─────────────────────────▶│  quote/0 Text API  │──▶ 実機表示
                                   └────────────────────┘
```

### 選定案（結論）

- **ランタイム: Node.js (v22, TypeScript)**
- **スケジューラ / CI: GitHub Actions（`schedule` cron + `workflow_dispatch`）**
- **シークレット管理: GitHub Actions Secrets（Repository / Environment secrets）**

各選定の比較は [Alternatives Considered](#alternatives-considered) 参照。

### ディレクトリ構成

```
quote0-batch/
├── docs/design_docs/           # 本 Design Doc 群
├── src/
│   ├── client/
│   │   └── quote0.ts           # Text API クライアント（薄いラッパ）
│   ├── content/                # 将来: コンテンツソース別モジュール
│   │   └── helloWorld.ts       # PoC: 固定 "Hello World" を返す
│   ├── config.ts               # 環境変数ロード / バリデーション
│   └── main.ts                 # エントリポイント（content → client 連携）
├── .github/workflows/
│   └── batch.yml               # schedule + workflow_dispatch
├── .env.example                # 必要な env のキーのみ（値は空）
├── package.json
├── tsconfig.json
└── README.md
```

### 設定 / シークレットの流れ

コードは `deviceId` / `apiKey` を **環境変数からのみ** 取得する。ハードコード禁止。

| 変数 | 内容 | 秘匿 | ローカル | CI |
| --- | --- | --- | --- | --- |
| `QUOTE0_API_KEY` | Bearer トークン | Yes | `.env`（gitignore） | GH Secrets |
| `QUOTE0_DEVICE_ID` | シリアルナンバー | Yes（推測・列挙対策） | `.env` | GH Secrets |
| `QUOTE0_BASE_URL` | API ベース URL | No | 既定値 | 既定値 |

- `.env` は `.gitignore` 対象。リポジトリには `.env.example`（キーのみ）を置く。
- `config.ts` 起動時に必須変数の存在を検証し、欠落時は即 fail（fail-fast）。
- deviceId も秘匿扱い: 漏洩すると第三者が実機へ書き込み可能なため（API Key と組で悪用）。ログ・エラー出力にマスク。

### 疑似コード（PoC）

```typescript
// src/client/quote0.ts
export async function sendText(cfg: Config, body: TextPayload): Promise<void> {
  const res = await fetch(
    `${cfg.baseUrl}/api/authV2/open/device/${cfg.deviceId}/text`,
    {
      method: "POST",
      headers: {
        Authorization: `Bearer ${cfg.apiKey}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify(body),
    },
  );
  if (!res.ok) {
    // deviceId/apiKey はマスクしてエラー送出
    throw new Error(`Text API failed: ${res.status} ${await res.text()}`);
  }
}

// src/main.ts
const cfg = loadConfig();               // env 検証、欠落で fail-fast
await sendText(cfg, {
  title: "quote0-batch",
  message: "Hello\nWorld",
  refreshNow: true,
});
```

### GitHub Actions ワークフロー（骨子）

```yaml
name: batch
on:
  workflow_dispatch:          # 手動実行（初期疎通確認用）
  schedule:
    - cron: "0 * * * *"       # 例: 毎時。初期は無効 or 頻度控えめ
jobs:
  run:
    runs-on: ubuntu-latest
    environment: production    # Environment secrets + 承認ゲート活用
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with: { node-version: 22, cache: npm }
      - run: npm ci
      - run: npm run start
        env:
          QUOTE0_API_KEY: ${{ secrets.QUOTE0_API_KEY }}
          QUOTE0_DEVICE_ID: ${{ secrets.QUOTE0_DEVICE_ID }}
```

初期は `schedule` をコメントアウトし `workflow_dispatch` のみで疎通確認 → OK 後に cron 有効化。

---

## Alternatives Considered

### A. 実行ランタイム

| 案 | 長所 | 短所 | 判定 |
| --- | --- | --- | --- |
| **Node.js + TS** | 型安全、fetch 標準内蔵(v18+)、エコシステム広い、GH Actions と親和 | ビルド構成が必要 | ✅ 採用 |
| Python | 記述簡潔、データ処理ライブラリ豊富 | 型は任意、将来コンテンツ処理次第では有力 | 次点 |
| Bash + curl | 依存ゼロ・最速で疎通可 | 整形・エラー処理・拡張に弱い、テスト困難 | PoC のみ可、本採用は不可 |

> 将来コンテンツ処理が重い場合 Python が有利になり得るが、初期は API 連携主体 → 型安全と保守性で Node.js/TS を選択。

### B. スケジューラ / 実行基盤

| 案 | 長所 | 短所 | 判定 |
| --- | --- | --- | --- |
| **GitHub Actions** | 追加インフラ不要、コードと同居、secrets 管理内蔵、cron 対応、無料枠 | cron 起動遅延あり(数分〜)、実行環境が汎用、長時間/高頻度はコスト・制約 | ✅ 採用 |
| Cloud Run Jobs + Cloud Scheduler | 起動時刻精度高、スケール・実行時間自由、リージョン選択 | GCP セットアップ・課金・IAM 管理コスト | 将来、要件超過時に移行候補 |
| セルフホスト cron (VPS/RasPi) | 常時制御・低レイテンシ | 保守・可用性・秘密管理を自前 | ✗ 運用負荷過大 |
| AWS Lambda + EventBridge | サーバレス・安価 | GH との連携に追加設定、初期学習コスト | 次点 |

> 初期要件（低頻度・軽量・単一デバイス）では GitHub Actions が最小コスト。cron 精度が問題化 or 実行が重くなれば Cloud Run Jobs へ移行する（コードは env のみ差分なので移行容易＝G4 の効用）。

### C. シークレット管理

| 案 | 長所 | 短所 | 判定 |
| --- | --- | --- | --- |
| **GH Actions Secrets (Environment)** | 標準・無料、承認ゲート/ブランチ保護と連携、ログ自動マスク | GH 外実行では使えない、ローテーション手動 | ✅ 採用 |
| GH Repository Secrets | 設定簡単 | 環境分離不可、全 workflow から参照可 | Environment secrets を優先 |
| クラウド Secret Manager (GCP/AWS) | ローテーション・監査・IAM 細粒度 | 認証の初期設定・課金 | 将来基盤移行時に併せて採用 |
| `.env` を暗号化コミット (SOPS/age) | GH 非依存・可搬 | 復号鍵の管理問題が残る | 現状不要 |

> 初期は GitHub Actions Secrets（Environment 単位）で十分。ログマスク・PR からの secret 露出制限（fork PR では secrets 非注入）が標準で効く。基盤をクラウドへ移す際に Secret Manager へ移行。

---

## Cross-Cutting Concerns

### セキュリティ

- **秘匿情報**: `QUOTE0_API_KEY` と `QUOTE0_DEVICE_ID` の両方を secret 扱い。コミット・ログ・エラー・PR 出力に平文を残さない。エラー時は deviceId をマスク（例: 末尾4桁のみ）。
- **`.gitignore`**: `.env`, `*.local` を除外。`git-secrets` 等の push 前スキャンを将来検討。
- **fork PR**: GitHub Actions は fork からの PR に secrets を注入しない標準挙動 → 外部コントリビュータ経由の漏洩を防止。
- **最小権限**: workflow の `permissions:` を必要最小（`contents: read`）に絞る。
- **キーローテーション**: API Key 漏洩時の再発行手順を README に記載（手動運用として許容）。

### 可観測性 / 失敗時挙動

- API 4xx/5xx は非ゼロ終了で fail → GitHub Actions が失敗表示。
- 失敗通知は初期最小限（GH の workflow 失敗メール）。将来 Slack 通知等を検討。
- リトライ: 500（デバイス通信失敗）は一時的な可能性 → 指数バックオフで数回リトライを将来追加（初期 PoC は単発）。

### コスト

- GitHub Actions 無料枠（public: 無制限 / private: 月2,000分）内で十分想定。低頻度 cron 前提。

### テスト / CI 品質

- `client` はモック fetch で単体テスト可能な設計（DI で `fetch`/config 注入）。
- Lint / typecheck を PR CI に追加（本 PoC 後）。

---

## Milestones

1. **M1 – PoC 疎通**: Node/TS 雛形 + `sendText` + `workflow_dispatch` で `Hello World` 実機表示。（本 Design Doc の主対象）
2. **M2 – 定期実行**: cron 有効化、失敗通知、リトライ。
3. **M3 – コンテンツ連携**: 最初の実コンテンツソース追加（別 Design Doc）。

---

## Open Questions

- cron の実行頻度・時刻（デバイス表示更新の望ましい間隔）は？
- 表示レイアウト（`title`/`signature`/`styles`/`icon`）の初期方針は？
- 将来のコンテンツソース優先順位（天気・カレンダー・RSS 等）は？
- GitHub リポジトリは public / private いずれか？（Actions 無料枠と secret 露出方針に影響）
