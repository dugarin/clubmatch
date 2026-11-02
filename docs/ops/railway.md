# Railway: развёртывание ClubMatch

Актуальный вариант публичного окружения для Telegram Mini App. Проект Railway: `clubmatch`, окружение `production`, регион `iad` (US East, Вирджиния, США). Трассировка: Q-1.3, NFR-10.

## Состав

- `Postgres` — `postgres:16-alpine`, постоянный том 500 МБ, доступ только через приватную сеть Railway.
- `server` — HTTP API, webhook и встроенный Mini App; публичный HTTPS-домен Railway, порт 8080.
- `worker` — отправка уведомлений; публичный домен не нужен.

Caddy и `docker-compose.stage.yml` на Railway не используются: HTTPS и маршрутизацию обеспечивает платформа. Prometheus, node-exporter и cron-бэкапы из self-hosted конфигурации здесь автоматически не разворачиваются.

## Инфраструктура как код

Конфигурация: `.railway/railway.ts`. Для CLI и Railway SDK нужен Node.js 22 или новее.

```bash
railway login
railway link --project clubmatch --environment production
npm ci
railway config plan
railway config apply
```

`plan` обязателен перед `apply`: он должен содержать только ожидаемые изменения. Удаление ресурсов требует отдельного подтверждения. Переменные окружения имеют режим `unmanaged`: IaC не перезаписывает и не удаляет значения, заданные через CLI или Railway Dashboard.

Перед запуском `server` и `worker` выполняется pre-deploy команда goose. Ошибка миграции останавливает деплой. В Dockerfile для этого включены бинарь goose и каталог `/db/migrations`.

## Переменные

`DATABASE_URL` приложений ссылается на `${{Postgres.DATABASE_URL}}`. Пароль PostgreSQL хранится только в переменных Railway. Не выводить `railway variable list --json`/`--kv` в общие журналы: эти форматы показывают значения.

Для `server`:

- обязательные настройки: `BINARY=server`, `APP_ENV=stage`, `DEV_AUTH_ENABLED=false`, `HTTP_ADDR=:8080`, `DATABASE_URL`;
- Telegram: `TELEGRAM_BOT_TOKEN`, `TELEGRAM_WEBHOOK_SECRET`, `TELEGRAM_BOT_USERNAME`, `TELEGRAM_MINIAPP_NAME`;
- доступ: `MODERATOR_TG_IDS` через запятую;
- опционально: `INITDATA_TTL_HOURS=24`, `WORKER_POLL_SECONDS=30`.

Для `worker`: `BINARY=worker`, `APP_ENV=stage`, `DEV_AUTH_ENABLED=false`, `DATABASE_URL`, `TELEGRAM_BOT_TOKEN`, `WORKER_POLL_SECONDS=30`.

Секреты задаются без передачи значения в аргументах команды:

```bash
read -rsp 'Telegram bot token: ' TELEGRAM_BOT_TOKEN; echo
printf '%s' "$TELEGRAM_BOT_TOKEN" | railway variable set --service server --stdin TELEGRAM_BOT_TOKEN
printf '%s' "$TELEGRAM_BOT_TOKEN" | railway variable set --service worker --stdin TELEGRAM_BOT_TOKEN
unset TELEGRAM_BOT_TOKEN

openssl rand -hex 32 | railway variable set --service server --stdin TELEGRAM_WEBHOOK_SECRET
```

`TELEGRAM_BOT_USERNAME`, `TELEGRAM_MINIAPP_NAME` и `MODERATOR_TG_IDS` задаются через Railway Dashboard или `railway variable set --service server KEY=value`.

## Деплой

Первоначально и при ручном обновлении код загружается из текущего рабочего каталога:

```bash
railway up --service server --ci
railway up --service worker --ci
```

Проверка:

```bash
railway service list
railway logs --service server --deployment --lines 100
railway logs --service worker --deployment --lines 100
railway domain --service server --port 8080
curl -fsS "https://<RAILWAY_DOMAIN>/healthz"
curl -fsS "https://<RAILWAY_DOMAIN>/readyz"
```

`/metrics` не должен публиковаться как отдельный публичный маршрут без проверки политики доступа. Для production-эксплуатации нужно отдельно настроить резервное копирование PostgreSQL и доставку алертов; существующие `ops/backup.sh`/`ops/restore.sh` рассчитаны на docker compose и напрямую здесь не применяются.

## Telegram

1. В @BotFather указать `https://<RAILWAY_DOMAIN>/` как URL Mini App.
2. Записать username бота и короткое имя Mini App в переменные `server`.
3. Установить webhook с тем же секретом, который сохранён в `TELEGRAM_WEBHOOK_SECRET`:

```bash
read -rsp 'Telegram bot token: ' TELEGRAM_BOT_TOKEN; echo
read -rsp 'Telegram webhook secret: ' TELEGRAM_WEBHOOK_SECRET; echo
curl -fsS -X POST "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/setWebhook" \
  --data-urlencode "url=https://<RAILWAY_DOMAIN>/telegram/webhook" \
  --data-urlencode "secret_token=${TELEGRAM_WEBHOOK_SECRET}"
unset TELEGRAM_BOT_TOKEN TELEGRAM_WEBHOOK_SECRET
```

4. Проверить UC-1…UC-7 в реальном Telegram и ручные NFR по `docs/acceptance-report.md`.

`DEV_AUTH_ENABLED=true` на Railway запрещено: обход Telegram initData предназначен только для локального предпросмотра с `APP_ENV=dev`.
