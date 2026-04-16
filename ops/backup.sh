#!/bin/sh
# DB + uploads のバックアップスクリプト。cron やホストから呼ぶ想定。
# 環境変数:
#   BACKUP_DIR    バックアップ出力先 (default: ./backups)
#   PG_DSN        postgresql://... (default: env の PG_PASSWORD から組み立て)
#   UPLOADS_DIR   uploads ディレクトリ (default: ./uploads)
#   RETAIN_DAILY  日次バックアップの保持日数 (default: 7)
#
# 使い方:
#   ./ops/backup.sh
#
# cron 例:
#   0 3 * * * /path/to/diary/ops/backup.sh

set -eu

cd "$(dirname "$0")/.."

BACKUP_DIR="${BACKUP_DIR:-./backups}"
UPLOADS_DIR="${UPLOADS_DIR:-./uploads}"
RETAIN_DAILY="${RETAIN_DAILY:-7}"

mkdir -p "${BACKUP_DIR}/db" "${BACKUP_DIR}/uploads"

STAMP=$(date +%Y%m%d-%H%M%S)
DB_FILE="${BACKUP_DIR}/db/diary-${STAMP}.sql.gz"
UP_FILE="${BACKUP_DIR}/uploads/uploads-${STAMP}.tar.gz"

echo "[$(date -Iseconds)] DB dump → ${DB_FILE}"
docker compose -f compose.prod.yml exec -T postgres \
  pg_dump -U diary -d diary --no-owner --no-comments \
  | gzip > "${DB_FILE}"

echo "[$(date -Iseconds)] uploads archive → ${UP_FILE}"
tar -czf "${UP_FILE}" -C "${UPLOADS_DIR}" .

echo "[$(date -Iseconds)] retention: keep last ${RETAIN_DAILY} days"
find "${BACKUP_DIR}/db" -name "diary-*.sql.gz" -mtime "+${RETAIN_DAILY}" -delete
find "${BACKUP_DIR}/uploads" -name "uploads-*.tar.gz" -mtime "+${RETAIN_DAILY}" -delete

echo "[$(date -Iseconds)] done"
du -sh "${BACKUP_DIR}"
