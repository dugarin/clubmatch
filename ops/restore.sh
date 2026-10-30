#!/usr/bin/env bash
# Восстановление БД stage (задача 3.0.6) из дампа, сделанного ops/backup.sh.
#
#   ./ops/restore.sh backups/clubmatch-20261030T030000Z.dump
#
# Процедура: страховочный бэкап текущего состояния → стоп server/worker →
# pg_restore --clean --if-existes (объекты БД пересоздаются из дампа) → старт server/worker.
#
# РАЗРУШИТЕЛЬНО: содержимое БД заменяется данными из дампа. Скрипт требует ручного
# подтверждения (ввести RESTORE) и сам сохраняет pre-restore-бэкап.
set -euo pipefail

usage() { echo "usage: $0 <dump-file>" >&2; exit 2; }
[ $# -eq 1 ] || usage

REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_DIR"

if [ -f .env ]; then
  set -a
  # shellcheck disable=SC1091
  . ./.env
  set +a
fi
PG_USER="${POSTGRES_USER:-clubmatch}"
PG_DB="${POSTGRES_DB:-clubmatch}"
COMPOSE_FILE="docker-compose.stage.yml"

FILE="$(realpath "$1")"
[ -f "$FILE" ] || { echo "дамп не найден: $FILE" >&2; exit 1; }

echo "Восстановление: $FILE -> БД $PG_DB"
echo "ВНИМАНИЕ: объекты БД будут пересозданы из дампа, текущие данные затёрты."
read -r -p "Ввести RESTORE для подтверждения: " reply
[ "$reply" = "RESTORE" ] || { echo "отменено"; exit 1; }

# 1) страховочный бэкап текущего состояния (до разрушительных действий)
PRE_DIR="$(realpath -m "${BACKUP_DIR:-$REPO_DIR/backups}")"
mkdir -p "$PRE_DIR"
PRE="$PRE_DIR/clubmatch-pre-restore-$(date -u +%Y%m%dT%H%M%SZ).dump"
docker compose -f "$COMPOSE_FILE" exec -T db pg_dump -U "$PG_USER" -d "$PG_DB" -Fc > "$PRE"
[ -s "$PRE" ] || { echo "страховочный бэкап не получился — восстановление отменено" >&2; exit 1; }
echo "страховочный бэкап текущего состояния: $PRE"

# 2) стоп писателей (server/worker); caddy останется отвечать 502 на время рестарта
docker compose -f "$COMPOSE_FILE" stop server worker

# 3) pg_restore: дамп копируется в контейнер, восстанавливается с пересозданием объектов
docker compose -f "$COMPOSE_FILE" cp "$FILE" db:/tmp/restore.dump
docker compose -f "$COMPOSE_FILE" exec -T db pg_restore -U "$PG_USER" -d "$PG_DB" --clean --if-exists --no-owner /tmp/restore.dump
docker compose -f "$COMPOSE_FILE" exec -T db rm -f /tmp/restore.dump

# 4) поднять приложение и проверить готовность
docker compose -f "$COMPOSE_FILE" start server worker
sleep 5
docker compose -f "$COMPOSE_FILE" ps
docker compose -f "$COMPOSE_FILE" exec -T server wget -qO- http://localhost:8080/readyz || true
echo
echo "Готово. Если readyz не ответил — смотрите docker compose -f $COMPOSE_FILE logs server."
echo "Откат на страховочный бэкап: $0 '$PRE'"
