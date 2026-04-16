# nekomisu-diary

クローズドな少人数交換日記システム。WordPress 製の旧サイトからの移行先として Go + PostgreSQL で実装する。

## スタック

- **バックエンド:** Go
- **DB:** PostgreSQL 17
- **フロントエンド:** Vite (フレームワーク未確定)
- **認証:** パスワード + TOTP (2FA) + WebAuthn
- **ホスティング:** Tailscale 配下（外形保護はTailnet境界に委ねる）

## ディレクトリ構成（予定）

```
.
├── schema.sql            # PostgreSQL スキーマ
├── cmd/
│   ├── server/           # Web サーバ本体
│   ├── migrate/          # WP MariaDB → Postgres 移行ツール
│   └── passwdreset/      # 管理者用パスワードリセット CLI
├── internal/             # 共通パッケージ
└── web/                  # Vite フロントエンド
```

## 設計メモ

- **アカウント運用:** Tailscale 認証が一段階かかる前提のため、サインアップは塞がない。
- **メール基盤なし:** パスワード忘れは CLI ツール `passwdreset` でハッシュを差し替える運用。
- **既存ユーザーパスワードは引き継がない:** 移行時に全員リセットして配布する。
- **本文形式:** WP の Gutenberg ブロックは移行時に `wp-cli` で `do_blocks()` を実行し HTML として取り込む。原ブロックも `posts.body_source` に保存して再変換可能にする。
- **カテゴリ/タグ:** 旧サイトで実質未使用のため移植しない。
- **コメント:** ネスト・匿名（投稿者名のみ）を許容するスキーマ。

## 開発環境

旧 WordPress スタックと新 PostgreSQL を同じ Docker compose で立てている（外向き通信遮断）。
別リポジトリ管理の compose 設定 `mnt/docker/wordpress/compose.dev.yml` 側を参照。

```bash
docker compose -f compose.dev.yml up -d
docker compose -f compose.dev.yml exec postgres psql -U diary -d diary
docker compose -f compose.dev.yml exec client wp db query "SELECT ..."
```
