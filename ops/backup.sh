#!/usr/bin/env bash
# Бэкап БД stage (задача 3.0.6; NFR-6, NFR-10 — бэкапы как митигация R7 из 05-risks).
# pg_dump в custom-формате (-Fc, сжимается, пригоден для pg_restore) + ротация по дням.
# Восстановление: ops/restore.sh.
#
# Запуск (на сервере; каталог вычисляется сам, можно из крона):
#   ./ops/backup.sh
# Cron (см. docs/ops/stage.md):
#   0 3 * * *  cd /opt/clubmatch && ./ops/backup.sh >> /var/log/clubmatch-backup.log 2>&1
#
# Параметры через окружение (можно задать в .env рядом с compose):
#   BACKUP_DIR      каталог дампов (по умолчанию ./backups в корне репо)
#   RETENTION_DAYS  хранение в днях (по умолчанию 14)
#
# Дамп консистентен БЕЗ остановки сервисов (MVCC-снапшот pg_dump).
set -euo pipefail

REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_DIR"

# POSTGRES_USER / POSTGRES_DB — те же переменные, что читает docker-compose.stage.yml
if [ -f .env ]; then
  set -a
  # shellcheck disable=SC1091
  . ./.env
  set +a
fi
PG_USER="${POSTGRES_USER:-clubmatch}"
PG_DB="${POSTGRES_DB:-clubmatch}"
BACKUP_DIR="$(realpath -m "${BACKUP_DIR:-$REPO_DIR/backups}")"
RETENTION_DAYS="${RETENTION_DAYS:-14}"
COMPOSE_FILE="docker-compose.stage.yml"

STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
FILE="$BACKUP_DIR/clubmatch-$STAMP.dump"

mkdir -p "$BACKUP_DIR"

echo "[backup] pg_dump $PG_USER/$PG_DB -> $FILE"
docker compose -f "$COMPOSE_FILE" exec -T db pg_dump -U "$PG_USER" -d "$PG_DB" -Fc > "$FILE"

# Архив не пуст и читается (pg_restore --list) — тем же postgres:16-alpine
[ -s "$FILE" ] || { echo "[backup] FAIL: пустой дамп" >&2; exit 1; }
docker run --rm -v "$FILE:/dump:ro" postgres:16-alpine pg_restore --list /dump > /dev/null

echo "[backup] OK: $FILE ($(du -h "$FILE" | cut -f1))"

# Ротация: старше RETENTION_DAYS дней — удалить
DELETED="$(find "$BACKUP_DIR" -name 'clubmatch-*.dump' -mtime "+$RETENTION_DAYS" -print -delete | wc -l)"
echo "[backup] ротация: удалено $DELETED дампов старше $RETENTION_DAYS дн."

# ВАЖНО (владелец): дампы лежат НА ТОМ ЖЕ сервере, что и БД — от потери сервера не защищают.
# Скопируйте BACKUP_DIR наружу (offsite): рекомендации — docs/ops/stage.md, раздел «Бэкапы».
