#!/bin/sh
# Build a full-snapshot deployment bundle of the current staging setup.
#
# Included:
#   - Entire diary/ directory (source + .git + .env + bin/ + web/dist + uploads)
#   - Fresh DB dump in data/diary.sql.gz
#   - Tailscale state copied in (so the target keeps the same node identity)
#   - compose.prod.yml rewritten to use bundled tailscale_state and DB dump
#
# Excluded:
#   - node_modules (regeneratable; huge)
#   - web/test-results, web/playwright-report
#   - diary-deploy-*.tar.gz (previous bundles)
#   - Playwright browser cache

set -eu

cd "$(dirname "$0")/.."
ROOT="$PWD"

STAMP=$(date +%Y%m%d-%H%M%S)
BUNDLE_NAME="diary-deploy-${STAMP}"
WORK="${ROOT}/.bundle-work"
OUT="${ROOT}/${BUNDLE_NAME}.tar.gz"

cleanup() {
  # tailscale_state inside WORK is root-owned; clean via container
  if [ -d "$WORK" ]; then
    docker run --rm -v "$WORK:/s" alpine rm -rf /s 2>/dev/null || true
    rm -rf "$WORK" 2>/dev/null || true
  fi
}
trap cleanup EXIT

rm -rf "$WORK"
mkdir -p "$WORK/${BUNDLE_NAME}"
STAGE="$WORK/${BUNDLE_NAME}"

echo "==> refreshing static binaries"
# Build in a throwaway golang container to avoid requiring Go on the host.
docker run --rm \
  -v "$ROOT":/app -w /app \
  -e GOTOOLCHAIN=auto -e CGO_ENABLED=0 \
  golang:1.24-alpine \
  sh -c "go build -ldflags='-s -w' -o bin/server ./cmd/server/ && go build -ldflags='-s -w' -o bin/passwdreset ./cmd/passwdreset/ && go build -ldflags='-s -w' -o bin/diary-tui ./cmd/diary-tui/"

echo "==> refreshing frontend build"
cd "$ROOT/web"
[ -d node_modules/vite ] || npm install --include=dev >/dev/null
npx vite build >/dev/null
cd "$ROOT"

echo "==> dumping production DB"
mkdir -p "$STAGE/data"
docker compose -f compose.prod.yml exec -T postgres \
  pg_dump -U diary -d diary --no-owner --no-comments \
  | gzip > "$STAGE/data/diary.sql.gz"
echo "    $(du -h "$STAGE/data/diary.sql.gz" | cut -f1)"

echo "==> copying repo snapshot (including .git, .env)"
# tar pipe gives us clean copy with excludes (no rsync dependency).
(cd "$ROOT" && tar -cf - \
  --exclude './node_modules' \
  --exclude './web/node_modules' \
  --exclude './web/test-results' \
  --exclude './web/playwright-report' \
  --exclude './web/.vite' \
  --exclude './diary-deploy-*.tar.gz' \
  --exclude './.bundle-work' \
  --exclude './tools/rendered.json' \
  --exclude './tools/seed-data.sql' \
  .) | (cd "$STAGE" && tar -xf -)

echo "==> copying Tailscale state"
# TS_STATE_SRC: override if your tailscale_state is elsewhere.
# Default: ./tailscale_state next to this script (matches compose.prod.yml).
TS_STATE_SRC="${TS_STATE_SRC:-$ROOT/tailscale_state}"
mkdir -p "$STAGE/tailscale_state"
if [ -d "$TS_STATE_SRC" ] && [ -n "$(ls -A "$TS_STATE_SRC" 2>/dev/null || true)" ]; then
  # Use a container so root-owned files are readable.
  docker run --rm \
    -v "$TS_STATE_SRC":/src:ro \
    -v "$STAGE/tailscale_state:/dst" \
    alpine sh -c 'cp -a /src/. /dst/ && chmod -R go-rwx /dst'
else
  echo "    (no state at $TS_STATE_SRC; bundle will require fresh TS_AUTHKEY)"
fi

echo "==> injecting DB dump into compose.prod.yml postgres initdb"
# Insert the data/diary.sql.gz mount after the pg-data volume line so the
# postgres container auto-loads the dump on first boot of a fresh volume.
python3 - "$STAGE/compose.prod.yml" <<'PY'
import sys, re
p = sys.argv[1]
t = open(p).read()
t = re.sub(
    r"(      - pg-data:/var/lib/postgresql/data)",
    r"\1\n      - ./data/diary.sql.gz:/docker-entrypoint-initdb.d/01-diary.sql.gz:ro",
    t,
    count=1,
)
open(p, "w").write(t)
PY

echo "==> writing DEPLOY.md"
cat > "$STAGE/DEPLOY.md" <<'MD'
# 本番デプロイ手順

このバンドルはステージングのフルスナップショット + 現時点の DB ダンプ +
Tailscale 識別情報です。解凍した中身はそのまま作業ディレクトリとして
使えます（.git も含まれているので `git pull` で更新も可能）。

## 1. 展開

```sh
tar -xzf diary-deploy-YYYYMMDD-HHMMSS.tar.gz
cd diary-deploy-YYYYMMDD-HHMMSS
```

## 2. .env の見直し (任意)

ステージングの .env がそのまま入っています。本番で値を変える場合は編集:

- `PG_PASSWORD`: 新しいランダム文字列推奨（DB ダンプを再ロードする場合は整合が取れる）
- `RP_ID` / `RP_ORIGIN`: Tailscale ホスト名を変更するなら
- `DISCORD_WEBHOOK`: 通知先を変えるなら

## 3. 起動

```sh
docker compose -f compose.prod.yml up -d
```

初回起動で `data/diary.sql.gz` が postgres に自動ロードされます（数秒〜）。
ログで `ready to accept connections` を確認してから次へ。

```sh
docker compose -f compose.prod.yml logs --tail=20 postgres
docker compose -f compose.prod.yml logs --tail=5 server
```

## 4. 動作確認

Tailscale admin でノードが online になっていることを確認 → ブラウザで
`.env` の `RP_ORIGIN` にアクセス。

## 5. 定期バックアップ

```sh
# crontab -e
0 3 * * * /absolute/path/to/bundle-dir/ops/backup.sh >> /var/log/diary-backup.log 2>&1
```

## その他

- **パスワードリセット:** `docker compose -f compose.prod.yml exec server /app/bin/passwdreset -user <login> -password <new>`
- **TUI クライアント:** `./ops/tui` （初回はイメージを自動ビルド）
- **開発環境:** `web/` 配下に node_modules がないので `cd web && npm install`
- **ソース改修:** `.git` があるので通常の git ワークフローで。

## 機密の取り扱い

このバンドルには:
- .env (PG_PASSWORD、Discord webhook URL)
- tailscale_state (ノード認証状態)
- data/diary.sql.gz (全投稿本文、bcrypt ハッシュ、メール、セッション)
- uploads/ (アップロード画像全て)

すべて含まれます。転送と配置時は権限に注意。
MD

echo "==> creating tar (via container with GNU tar to read root-owned files)"
docker run --rm \
  -v "$WORK:/w:ro" \
  -v "$ROOT:/out" \
  alpine sh -c "apk add --no-cache tar >/dev/null && cd /w && tar --owner=0 --group=0 -czf /out/${BUNDLE_NAME}.tar.gz ${BUNDLE_NAME}"

echo
echo "Bundle: $OUT"
echo "Size:   $(du -h "$OUT" | cut -f1)"
echo
echo "Top-level contents:"
tar -tzf "$OUT" | awk -F/ 'NF>=2 && $2!="" {print $2}' | sort -u | head -30 | sed 's/^/  /'
