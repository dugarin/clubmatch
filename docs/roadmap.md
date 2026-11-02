# Дорожная карта: закрытие спецификации → архитектура

Контекст: план правок `docs/fix-plan.md` выполнен полностью (T1–T16, 2026-10-28).
2026-10-29 заказчик утвердил все решения (протокол `docs/requirements/08-approval-form.md`),
решения внесены в 01–07 (1.3), повторное ревью пройдено без CRITICAL (1.4),
статус документов — **spec 1.3** после уточнения Q10 по итогам T-1 (2026-10-31).

## Фаза 1. Закрытие спецификации (снятие draft)

| № | Задача | Результат | Зависит от | Статус |
|---|---|---|---|---|
| 1.1 | Опросник заказчику: подтверждение Р1–Р8 + решения по Q1–Q4, Q18, Q19, Q23–Q26 | `docs/requirements/08-approval-form.md` | — | [x] |
| 1.2 | Техспайк: реализуемость `tg://user?id=...` при скрытом username (Q10) | `docs/spikes/tg-user-link.md` — исследование официальной документации | — | [x] завершён 2026-10-31; формат контакта скорректирован |
| 1.3 | Внесение утверждённых решений: снять `[предположение]`, закрыть вопросы в 07, обновить статусы | коммиты `docs:` | ответы заказчика, итог спайка 1.2 | [x] |
| 1.4 | Повторное независимое ревью спеки (reviewer) | отчёт; критерий — 0 CRITICAL | 1.3 | [x] (F1–F19 закрыты, N1–N3 закрыты, CRITICAL — 0) |
| 1.5 | Снять `draft`, зафиксировать «spec 1.0», обновить README | спека готова к разработке | 1.4 | [x] |

Критерий выхода фазы 1: все вопросы 07 закрыты или имеют явный non-goal;
нет CRITICAL-находок повторного ревью; статус документов не draft.
**Выполнен 2026-10-29; техническое уточнение Q10 внесено 2026-10-31.**

## Фаза 2. Архитектура (вход: spec 1.0)

| № | Артефакт | Содержание | Статус |
|---|---|---|---|
| 2.1 | Доменная модель (ER) | Анкета, Тег + алиасы, Оценка, Матч, Заявка, Приглашение, Журнал раскрытия контактов | [x] accepted 2026-10-30: [architecture/01-domain-model.md](architecture/01-domain-model.md) |
| 2.2 | ADR-1: стек и хранилище | Postgres как кандидат (полнотекстовый поиск рус./англ. из Р2) + ≥1 альтернатива | [x] accepted 2026-10-30: [architecture/adr-001-storage-stack.md](architecture/adr-001-storage-stack.md) (PG 16, Go; публичный хостинг Railway выбран 2026-11-02) |
| 2.3 | ADR-2: интеграция Telegram | Bot API + валидация `initData` (NFR-7), доставка/повтор уведомлений (Р3, NFR-12) | [x] accepted 2026-10-30: [architecture/adr-002-telegram-integration.md](architecture/adr-002-telegram-integration.md) |
| 2.4 | ADR-3: поиск и ранжирование | Реализация Р2: нормализация, алиасы тегов, приоритеты полей, пагинация | [x] accepted 2026-10-30: [architecture/adr-003-search-ranking.md](architecture/adr-003-search-ranking.md) |
| 2.5 | ADR-4: видимость полей и приватность | Server-side фильтрация полей (Р6, FR-14, AC-7), журнал раскрытия (Р8) | [x] accepted 2026-10-30: [architecture/adr-004-visibility-privacy.md](architecture/adr-004-visibility-privacy.md) |
| 2.6 | C4: контекст + контейнеры | Mini App, backend, бот, БД | [x] draft: [architecture/02-c4.md](architecture/02-c4.md) |
| 2.7 | Sequence-диаграммы | Матч (взаимное «интересно» → раскрытие контакта → недоставка уведомления); допуск заявки | [x] draft: [architecture/03-sequences.md](architecture/03-sequences.md) |
| 2.8 | OpenAPI-контракт | Каждый эндпоинт трассируется к FR/UC; пересчёт матрицы покрытия | [x] draft: [architecture/04-openapi.yaml](architecture/04-openapi.yaml) + [04b-coverage-matrix.md](architecture/04b-coverage-matrix.md) (пробелов нет) |
| 2.9 | Список рисков + observability | По handoff-формату architecture skill | [x] draft: [architecture/05-risks-observability.md](architecture/05-risks-observability.md) |

Свод открытых вопросов фазы 2 (решения принимает владелец): [architecture/06-open-questions.md](architecture/06-open-questions.md);
опросник для утверждения: [architecture/07-approval-form.md](architecture/07-approval-form.md) (отправлен владельцу 2026-10-30).

Каждый ADR — кандидат на решение с ≥2 альтернативами; решение принимает владелец.

Критерий выхода фазы 2: все ADR приняты владельцем; покрытие FR эндпоинтами полное.
**Выполнен 2026-10-30**: опросник [architecture/07-approval-form.md](architecture/07-approval-form.md) заполнен,
ADR-001…004 и доменная модель — accepted; UC-1 дополнен шагом запуска бота, NFR-12 ограничен связкой (спека 1.1).
Независимое ревью архитектуры (2026-10-30, reviewer): 17 находок (1 CRITICAL — F1, решён владельцем; 3 MAJOR; 13 MINOR) — все исправлены.
Открыты только техпроверки T-1…T-3 (спайки) — перенесены в фазу 3; T-3 включает кейс «кафкой» (F3).

## Фаза 3. Подготовка к разработке

Декомпозиция на задачи — после 2.8 (эндпоинты и модель данных определены).
Первые задачи фазы 3: спайки T-2 (двуязычный tsvector) и T-3 (корпус AC-5, формирует команда), спайк T-1 (`tg://user?id`) — вручную в Telegram.

| № | Артефакт | Статус |
|---|---|---|
| 3.0 | Декомпозиция: 8 эпиков, 29 задач (вкл. окружение: dev/stage, CI/CD, мониторинг), граф зависимостей | [x] accepted 2026-10-30: [phase-3-plan.md](phase-3-plan.md) (MVP-1 = эпики 0–4; EN-тексты пишет команда; окружение — сразу полностью) |

### Прогресс фазы 3 (2026-11-02)

| Задача | Статус |
|---|---|
| 3.0.1–3.0.4 каркас, dev-окружение | [x] |
| 3.0.5–3.0.8 CI, stage, CD, мониторинг | [~] Railway: PostgreSQL 16, server и worker запущены, GitHub-источник `main`, миграции 8 применены, HTTPS/healthz/readyz/SPA проверены. Prometheus и бэкапы на Railway не настроены. Деплой коммита `c6a6b38` (приватные метрики) не применён: server/worker остались на предыдущих успешных контейнерах после сбоя BuildKit |
| 3.1.1 спайк T-2 (FTS) | [x] [spikes/fts-bilingual.md](spikes/fts-bilingual.md) |
| 3.1.2 корпус AC-5 (T-3) | [x] [spikes/ac5-corpus.md](spikes/ac5-corpus.md) + search_corpus_test |
| 3.1.3 спайк T-1 (`tg://user?id`) | [x] исследование официальной документации; [spikes/tg-user-link.md](spikes/tg-user-link.md), Q10 уточнён (**spec 1.3**) |
| 3.2.1–3.2.4 данные, initData, webhook | [x] |
| 3.3.1–3.3.4 анкета, теги, заявки, модерация | [x] |
| 3.4.1–3.4.5 видимость, колода, матч, поиск, матчи | [x] |
| 3.5.1–3.5.2 worker уведомлений, удаление анкеты | [x] |
| 3.6.1–3.6.5 Mini App UI (MVP-1) | [x] |
| 3.6.6 UI модерации | [ ] MVP-2 |
| 3.7.1 метрики | [x] в коде server и worker слушают приватный `:9091`; публичный `/metrics` возвращает 404 после ожидаемого redeploy |
| 3.7.2 приёмка AC | [~] покрытие тестами: [acceptance-report.md](acceptance-report.md); ручные NFR и E2E в реальном Telegram — после настройки Railway webhook/Mini App |

Дополнительно 2026-10-31 (вечер): разбор открытых вопросов — закрыты D-1.1 (MVP-поиск только по тегам, **spec 1.2**), D-2.1, D-2.2, D-3.1 (алиасы модератором), D-3.2 (revoked → 422), D-3.3 (non-goal), D-3.4, D-3.5, D-3.6 (409), D-3.7, D-3.8 (MODERATOR_TG_IDS). Репозиторий запушен на GitHub (main).

2026-11-02 выбран Railway (регион `iad`, Вирджиния, США). Публичный адрес: `https://server-production-3725.up.railway.app`. IaC — `.railway/railway.ts`, инструкция — [ops/railway.md](ops/railway.md). Коммиты `c892dbe` (Railway) и `c6a6b38` (приватные метрики) отправлены в `main`; Go build/test и golangci-lint — успешно. `TELEGRAM_WEBHOOK_SECRET` сгенерирован и сохранён в переменных server; `TELEGRAM_BOT_TOKEN` ещё не задан.

### Правило сессий

Одна сессия = одна задача (или один критерий). Сессия завершается закрытием задачи коммитом; следующая сессия стартует со следующей незакрытой задачи из этого roadmap. Точка продолжения — блок ниже.

### Следующая сессия — начать с

1. Повторить `railway redeploy --service server --from-source --yes` и то же для worker. Предыдущие сборки `d76cb234`/`f227afe5` упали из-за инфраструктурной ошибки BuildKit (`buildkitd.sock`), не из-за тестов или кода. Убедиться, что развёрнут `c6a6b38`, затем проверить `/healthz`, `/readyz`, SPA и публичный `/metrics` = 404.
2. Задать одинаковый `TELEGRAM_BOT_TOKEN` в Railway server и worker (пользователь добавляет секрет через Dashboard, не в чат).
3. Установить webhook на `https://server-production-3725.up.railway.app/telegram/webhook` с уже сохранённым `TELEGRAM_WEBHOOK_SECRET`; указать корневой URL в @BotFather как Mini App URL.
4. Задать `TELEGRAM_BOT_USERNAME`, `TELEGRAM_MINIAPP_NAME`, `MODERATOR_TG_IDS` в server.
5. Провести E2E UC-1…UC-7 в реальном Telegram и ручные проверки NFR-1/2/3/4/11.

Заметки для следующей сессии: Railway CLI авторизован, проект `clubmatch` уже связан с репозиторием. Для `railway config plan/apply` нужен Node.js 22 (`nvm use 22`; установлен 22.23.4), затем `npm ci`. Переменные — unmanaged, секреты не хранить в IaC. Автоматический запуск деплоя по push не подтверждён; использовать `railway redeploy --from-source`. Субагенты недоступны — работать в основной сессии.

## Блокеры и зависимости

- Повторный Railway redeploy коммита `c6a6b38` после временного сбоя BuildKit.
- Telegram bot token и действия владельца в @BotFather; затем webhook и ручная приёмка.
- Для полноценной эксплуатации Railway ещё нужны бэкапы PostgreSQL и доставка алертов.

Статус: active (2026-11-02). Фазы 1–2 выполнены; MVP-1 развёрнут на Railway в базовой конфигурации. Остались применение последнего коммита, настройка Telegram и ручная приёмка. Продолжение — блок «Следующая сессия». См. [README](../README.md), [fix-plan](fix-plan.md).
