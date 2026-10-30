# Stage-окружение: развёртывание и эксплуатация

Задача 3.0.6 ([план фазы 3](../phase-3-plan.md)); трассировка: Q-1.3, NFR-6, NFR-10.
Стек: [docker-compose.stage.yml](../../docker-compose.stage.yml) — db (PostgreSQL 16) + migrate
(goose) + server + worker + caddy (автоматический TLS); профиль `monitoring` (3.0.8) —
prometheus + node-exporter. Размещение: self-hosted, свой сервер (ADR-001).

CD-пайплайн и откат: [deploy.md](deploy.md).

## 1. Требования к серверу

| Параметр | Значение |
|---|---|
| ОС | Linux x86_64 (проверено на ядре с Docker Engine ≥ 24) `[непроверено: конкретный дистрибутив владельца]` |
| Docker | Docker Engine + docker compose plugin (v2+) |
| Порты наружу | 80/tcp, 443/tcp+udp (Caddy/ACME); 22/tcp для администрирования |
| DNS | A-запись `STAGE_DOMAIN` → внешний IP сервера |
| Ресурсы | 2 vCPU / 2 ГБ RAM / ≥ 20 ГБ диска `[предположение: масштаб MVP ≤ 1000 участников, NFR-4]` |
| Исходящий трафик | docker hub (образы), proxy.golang.org (goose при первом прогоне), api.telegram.org, Let's Encrypt |

## 2. Первый деплой (порядок: docker → clone → .env → compose up)

Все команды — на сервере.

```bash
# 1) Docker (Ubuntu/Debian; для других дистрибутивов — docs.docker.com)
curl -fsSL https://get.docker.com | sh

# 2) Код. Репозиторий приватный → нужен deploy key (read-only) или https+token.
#    Ключ: ssh-keygen -t ed25519; публичную часть добавить в GitHub → Settings → Deploy keys.
sudo mkdir -p /opt/clubmatch && sudo chown "$USER" /opt/clubmatch
git clone git@github.com:dugarin/clubmatch.git /opt/clubmatch
cd /opt/clubmatch

# 3) Секреты — только в .env на сервере (в репо его нет)
cp .env.example .env
chmod 600 .env
$EDITOR .env   # обязательно: POSTGRES_PASSWORD, STAGE_DOMAIN; по готовности: TELEGRAM_*

# 4) Поднять стек (миграции применятся автоматически сервисом migrate)
docker compose -f docker-compose.stage.yml up -d --build

# 5) Проверка
docker compose -f docker-compose.stage.yml ps          # все healthy/exited(0)
curl -fsS "https://<STAGE_DOMAIN>/healthz"             # {"status":"ok"} — TLS от Caddy
docker compose -f docker-compose.stage.yml logs server | head
```

Что проверяет `up`: caddy ждёт healthcheck server (`/healthz`), server/worker ждут
успешного завершения migrate (goose `up`), db — `pg_isready`. Ничего не поднимется
«раньше времени»; `migrate` в состоянии `exited (0)` — норма.

TLS: Caddy сам получает и продлевает сертификат Let's Encrypt. Требования: домен из
`STAGE_DOMAIN` резолвится на сервер, порты 80/443 доступны извне. Секреты ACME хранятся
в volume `caddy_data` — не удалять его (`docker compose down` безопасен, `down -v` — нет).

Для локальной прогоночной проверки без домена: `STAGE_DOMAIN=stage.localhost` — Caddy
выдаст внутренний self-signed сертификат (`curl -k https://stage.localhost/healthz`).

## 3. Обновление вручную

```bash
cd /opt/clubmatch
git pull --ff-only
docker compose -f docker-compose.stage.yml up -d --build
```

Автоматический деплой по мерджу в main — [deploy.md](deploy.md).

## 4. Секреты

- Все секреты — в `.env` на сервере (`chmod 600`); в репозитории только `.env.example`.
- `POSTGRES_PASSWORD` обязателен: без него compose откажет подниматься (ошибка подстановки).
- `TELEGRAM_BOT_TOKEN` / `TELEGRAM_WEBHOOK_SECRET` — по готовности эпика 2 (ADR-002).
- `DATABASE_URL` из `.env` на stage **перекрывается** compose (БД = контейнер `db`).

## 5. Бэкапы (NFR-6, NFR-10; митигация риска R7 из 05-risks)

Сделать бэкап вручную: `./ops/backup.sh` → `backups/clubmatch-<UTC>.dump`
(pg_dump, custom-формат; консистентен без остановки сервисов).

Cron (ежедневно в 03:00, ротация 14 дней — настраивается в `.env`):

```cron
0 3 * * *  cd /opt/clubmatch && ./ops/backup.sh >> /var/log/clubmatch-backup.log 2>&1
```

Установка cron: `crontab -e`. Лог-файл ротировать logrotate или взять `BACKUP_DIR` с ротацией.

Восстановление (разрушительно, с подтверждением и страховочным бэкапом):

```bash
./ops/restore.sh backups/clubmatch-<UTC>.dump
```

**Открытое решение владельца**: дампы лежат на том же сервере, что и БД. Для защиты от
потери сервера скопировать `backups/` наружу (offsite): `rclone`/`restic` на объектное
хранилище или `rsync` на второй хост — добавить в cron после `backup.sh`.

## 6. Мониторинг (3.0.8)

```bash
docker compose -f docker-compose.stage.yml --profile monitoring up -d

# UI Prometheus — только по SSH-туннелю (порт опубликован на 127.0.0.1 сервера):
ssh -L 9090:127.0.0.1:9090 <user>@<server>
# затем локально: http://127.0.0.1:9090  (алерты: /alerts)
```

- Конфигурация: [ops/prometheus/prometheus.yml](../../ops/prometheus/prometheus.yml),
  правила: [ops/prometheus/alerts.yml](../../ops/prometheus/alerts.yml).
- Алерты — «недоступность > 5 мин», outbox (age > 10 мин, p95 лага уведомлений > 60 c),
  p95 поиска > 4 с, диск < 10%/5% — по таблице [05-risks](../architecture/05-risks-observability.md).
- До эпика 7 (3.7.1, эндпоинт `/metrics`) алерт `ClubmatchServerDown` будет постоянно в
  FIRING — это ожидаемое состояние, фактически подтверждает работу алертинга.
- **Открытое решение владельца**: доставка алертов (Alertmanager → Telegram/email) не
  развёрнута; пока алерты видны только в UI. Добавить при вводе в эксплуатацию.

## 7. Логи

```bash
docker compose -f docker-compose.stage.yml logs -f server worker
```

Ротация: драйвер `json-file`, `max-size 10m`, `max-file 3` (задано в compose для всех
сервисов — «свободное место» из 05-risks). Логи backend — структурный JSON в stdout.

## 8. Остановка и чистка

```bash
docker compose -f docker-compose.stage.yml down            # стоп; данные/тома остаются
docker compose -f docker-compose.stage.yml --profile monitoring down
# ВНИМАНИЕ: down -v удаляет тома: pgdata (БД!), caddy_data (сертификаты), prometheus_data
```

Откат версии: [deploy.md](deploy.md) (раздел «Откат»).
