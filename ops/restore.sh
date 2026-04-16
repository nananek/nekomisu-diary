#!/bin/sh
# リストアスクリプト (緊急用)。DB の中身は完全に上書きされるので注意。
# 使い方:
#   ./ops/restore.sh ./backups/db/diary-YYYYMMDD-HHMMSS.sql.gz

set -eu

cd "$(dirname "$0")/.."

if [ $# -lt 1 ]; then
  echo "Usage: $0 <dump.sql.gz>" >&2
  exit 1
fi

DUMP="$1"

if [ ! -f "$DUMP" ]; then
  echo "Not found: $DUMP" >&2
  exit 1
fi

read -p "本当に DB を $DUMP で上書きしますか？ [yes/NO]: " reply
[ "$reply" = "yes" ] || { echo "abort"; exit 1; }

echo "Stopping server..."
docker compose -f compose.prod.yml stop server

echo "Dropping/recreating database..."
docker compose -f compose.prod.yml exec -T postgres psql -U diary -d postgres -c "DROP DATABASE IF EXISTS diary"
docker compose -f compose.prod.yml exec -T postgres psql -U diary -d postgres -c "CREATE DATABASE diary"

echo "Loading dump..."
gunzip -c "$DUMP" | docker compose -f compose.prod.yml exec -T postgres psql -U diary -d diary

echo "Starting server..."
docker compose -f compose.prod.yml start server

echo "Done."
