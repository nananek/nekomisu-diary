# nekomisu-diary

クローズドな少人数向け交換日記システム。WordPress からの移行先として、
Go + PostgreSQL + Vite (React) で実装しました。

> 主に自分の仲間内で使うために作ったものを公開しています。そのまま使えると
> 嬉しいですが、個別の事情に合わせた調整を前提にしてください。

## 機能

- **認証:** パスワード / TOTP 2FA / WebAuthn（セキュリティキー）
- **投稿:** 公開 / 自分のみ / 下書き、Markdown エディタ + プレビュー
- **コメント:** ネスト返信、自分のコメントの削除
- **メディア:** アップロード（JPEG は自動で EXIF 除去・サムネ生成）、ギャラリー
- **閲覧:** タイムライン（未読バッジ）、メンバー別、検索、個別記事
- **プロフィール:** 表示名、アバター
- **通知:** Discord Webhook（新規投稿・コメント）
- **HTML 安全化:** 投稿 HTML は bluemonday でサーバー側サニタイズ
- **セキュリティ:** ログインのレート制限、セッション Cookie
- **PWA:** Service Worker + manifest、ホーム画面追加対応
- **モバイル:** 下部タブ、ダークモード、自動/手動テーマ切替
- **多言語:** 日本語 UI
- **TUI クライアント:** bubbletea で作ったターミナル用アプリ (`cmd/diary-tui`)

## スタック

- Go 1.24+ / PostgreSQL 17
- React 19 + Vite + TypeScript
- [bubbletea](https://github.com/charmbracelet/bubbletea) / [bluemonday](https://github.com/microcosm-cc/bluemonday) / [pquerna/otp](https://github.com/pquerna/otp) / [go-webauthn](https://github.com/go-webauthn/webauthn) / [marked](https://github.com/markedjs/marked)

## 起動 (ローカル開発)

```sh
# 1. PostgreSQL を立てる (Docker でも OS ネイティブでも)
# 2. スキーマをロード
psql -U postgres -f schema.sql

# 3. サーバーをビルドして起動
go build -o bin/server ./cmd/server
./bin/server -pg 'postgres://localhost/diary?sslmode=disable' \
             -uploads ./uploads -web ./web/dist

# 4. フロントエンドを別ターミナルで (dev サーバーは /api を proxy)
cd web
npm install
npm run dev
```

ブラウザで `http://localhost:5173` 。

## 本番デプロイ

Tailscale で公開する前提の `compose.prod.yml` を同梱。Tailscale Serve で
自動 HTTPS、サーバー本体は GHCR (`ghcr.io/nananek/nekomisu-diary-server`)
から pull します。詳細は `DEPLOY.md`（バンドル時に自動生成）を参照。

```sh
cp .env.example .env  # 値を埋める
mkdir -p tailscale_state uploads   # 初回のみ
docker compose -f compose.prod.yml up -d
```

特定のバージョンに固定したい場合は `.env` 等で `SERVER_IMAGE=ghcr.io/nananek/nekomisu-diary-server:sha-abc1234` を指定。

## パスワードリセット

メール基盤は持っていないので、リセットは管理者 (サーバーに入れる人) が CLI で行います。
`passwdreset` バイナリはサーバーイメージに同梱されています。

### 3つのモード

| 用途 | コマンド |
|---|---|
| 1人・パスワード指定 | `-user <login> -password <pw>` |
| 1人・ランダム生成 (stdout に出力) | `-user <login> -random` |
| 全員・ランダム生成 (確認プロンプト) | `-all -random` |

ランダムパスワードは 16文字、紛らわしい文字 (0/O/1/l/I) を除外した英数字。

### 本番 (compose.prod.yml で動かしている場合)

```sh
# 1人、パスワード指定
docker compose -f compose.prod.yml exec server \
  /app/bin/passwdreset \
  -pg "postgres://diary:${PG_PASSWORD}@127.0.0.1:5432/diary?sslmode=disable" \
  -user <login> -password <新しいパスワード>

# 1人、ランダム (標準出力に表示された文字列を渡す)
docker compose -f compose.prod.yml exec server \
  /app/bin/passwdreset -pg "..." -user <login> -random

# 全員リセット (対話的確認あり、login<TAB>password で出力)
docker compose -f compose.prod.yml exec -T server \
  /app/bin/passwdreset -pg "..." -all -random > new-passwords.tsv
# ※ compose exec に -T (no TTY) + 対話入力できない環境では -yes が必要
```

`-all -random` 実行時の確認プロンプト例:

```
⚠  This will reset passwords for all 3 users to fresh random values.
   Existing passwords will be lost; you must hand the new ones to each user.
   2FA (TOTP / WebAuthn) is NOT affected.

Type 'yes' to proceed:
```

### ローカル開発

```sh
go run ./cmd/passwdreset -pg "postgres://localhost/diary?sslmode=disable" -user <login> -random
```

### 挙動

- 指定した login が存在しない場合はエラー終了（既存ハッシュは変更されない）
- 2FA (TOTP / WebAuthn) の設定はリセットされない — ユーザーが 2FA を紛失した場合は
  `DELETE FROM totp_secrets WHERE user_id = ...` / `DELETE FROM webauthn_credentials WHERE user_id = ...`
  を psql で直接実行してください
- `-all` で出力されたパスワード一覧は安全な方法 (Discord DM、暗号化ファイル転送 等) で配布してください。シェル履歴・ログに残さないこと。

## テスト

```sh
# Go (unit + integration, 需要 postgres)
go test ./internal/...

# Playwright E2E
cd web && npx playwright test
```

## ディレクトリ

```
.
├── schema.sql              PostgreSQL スキーマ
├── cmd/
│   ├── server/             本体 HTTP サーバー
│   ├── migrate/            WP MariaDB → Postgres 移行ツール
│   ├── sanitize-existing/  既存投稿 HTML の一括再サニタイズ
│   ├── exif-strip/         既存 JPEG の EXIF 一括除去
│   ├── passwdreset/        パスワードリセット CLI
│   └── diary-tui/          TUI クライアント
├── internal/
│   ├── handler/            HTTP ハンドラ
│   ├── session/            セッション管理
│   ├── notifier/           Discord 通知
│   ├── ratelimit/          レート制限
│   ├── sanitize/           HTML サニタイズ
│   ├── db/                 DB 接続
│   └── testutil/           テストヘルパ
├── web/                    Vite + React フロントエンド
│   └── tests/              Playwright E2E
├── ops/
│   ├── bundle.sh           デプロイバンドル作成
│   ├── backup.sh           DB + uploads バックアップ
│   ├── restore.sh          リストア
│   └── tui                 TUI を Docker で起動
└── compose.prod.yml
```

## ライセンス

MIT
