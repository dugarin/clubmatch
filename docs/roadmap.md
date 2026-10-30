# Дорожная карта: закрытие спецификации → архитектура

Контекст: план правок `docs/fix-plan.md` выполнен полностью (T1–T16, 2026-10-28).
2026-10-29 заказчик утвердил все решения (протокол `docs/requirements/08-approval-form.md`),
решения внесены в 01–07 (1.3), повторное ревью пройдено без CRITICAL (1.4),
статус документов — **spec 1.0** (1.5). Открытый пункт фазы 1 — только спайк
`tg://user?id` (1.2, влияет на безусловную приёмку FR-8/AC-4).

## Фаза 1. Закрытие спецификации (снятие draft)

| № | Задача | Результат | Зависит от | Статус |
|---|---|---|---|---|
| 1.1 | Опросник заказчику: подтверждение Р1–Р8 + решения по Q1–Q4, Q18, Q19, Q23–Q26 | `docs/requirements/08-approval-form.md` | — | [x] |
| 1.2 | Техспайк: реализуемость `tg://user?id=...` при скрытом username (Q10, блокер приёмки FR-8) | `docs/spikes/tg-user-link.md` — чек-лист проверки | — | [x] |
| 1.3 | Внесение утверждённых решений: снять `[предположение]`, закрыть вопросы в 07, обновить статусы | коммиты `docs:` | ответы заказчика, итог спайка 1.2 | [x] (решения внесены; спайк 1.2 не блокирует текст спеки — FR-8 помечен условным) |
| 1.4 | Повторное независимое ревью спеки (reviewer) | отчёт; критерий — 0 CRITICAL | 1.3 | [x] (F1–F19 закрыты, N1–N3 закрыты, CRITICAL — 0) |
| 1.5 | Снять `draft`, зафиксировать «spec 1.0», обновить README | спека готова к разработке | 1.4 | [x] |

Критерий выхода фазы 1: все вопросы 07 закрыты или имеют явный non-goal;
нет CRITICAL-находок повторного ревью; статус документов не draft.
**Выполнен 2026-10-29** (кроме спайка 1.2 — он влияет только на условность AC-4).

## Фаза 2. Архитектура (вход: spec 1.0)

| № | Артефакт | Содержание | Статус |
|---|---|---|---|
| 2.1 | Доменная модель (ER) | Анкета, Тег + алиасы, Оценка, Матч, Заявка, Приглашение, Журнал раскрытия контактов | [~] draft: [architecture/01-domain-model.md](architecture/01-domain-model.md), ждёт решения владельца |
| 2.2 | ADR-1: стек и хранилище | Postgres как кандидат (полнотекстовый поиск рус./англ. из Р2) + ≥1 альтернатива | [~] proposed: [architecture/adr-001-storage-stack.md](architecture/adr-001-storage-stack.md), ждёт решения владельца |
| 2.3 | ADR-2: интеграция Telegram | Bot API + валидация `initData` (NFR-7), доставка/повтор уведомлений (Р3, NFR-12) | [~] proposed: [architecture/adr-002-telegram-integration.md](architecture/adr-002-telegram-integration.md), ждёт решения владельца |
| 2.4 | ADR-3: поиск и ранжирование | Реализация Р2: нормализация, алиасы тегов, приоритеты полей, пагинация | [~] proposed: [architecture/adr-003-search-ranking.md](architecture/adr-003-search-ranking.md) |
| 2.5 | ADR-4: видимость полей и приватность | Server-side фильтрация полей (Р6, FR-14, AC-7), журнал раскрытия (Р8) | [~] proposed: [architecture/adr-004-visibility-privacy.md](architecture/adr-004-visibility-privacy.md) |
| 2.6 | C4: контекст + контейнеры | Mini App, backend, бот, БД | [x] draft: [architecture/02-c4.md](architecture/02-c4.md) |
| 2.7 | Sequence-диаграммы | Матч (взаимное «интересно» → раскрытие контакта → недоставка уведомления); допуск заявки | [x] draft: [architecture/03-sequences.md](architecture/03-sequences.md) |
| 2.8 | OpenAPI-контракт | Каждый эндпоинт трассируется к FR/UC; пересчёт матрицы покрытия | [x] draft: [architecture/04-openapi.yaml](architecture/04-openapi.yaml) + [04b-coverage-matrix.md](architecture/04b-coverage-matrix.md) (пробелов нет) |
| 2.9 | Список рисков + observability | По handoff-формату architecture skill | [x] draft: [architecture/05-risks-observability.md](architecture/05-risks-observability.md) |

Свод открытых вопросов фазы 2 (решения принимает владелец): [architecture/06-open-questions.md](architecture/06-open-questions.md);
опросник для утверждения: [architecture/07-approval-form.md](architecture/07-approval-form.md) (отправлен владельцу 2026-10-30).

Каждый ADR — кандидат на решение с ≥2 альтернативами; решение принимает владелец.

## Фаза 3. Подготовка к разработке

Декомпозиция на задачи — после 2.8 (эндпоинты и модель данных определены).

## Блокеры и зависимости

- Ответы заказчика (фаза 1 целиком).
- Endpoint агента reviewer — повторить 1.4 после восстановления.
- `tg://user?id` — `[непроверено]` до спайка 1.2 (выполняется владельцем/разработчиком в реальном Telegram).

Статус: active (2026-10-29). См. [README](../README.md), [fix-plan](fix-plan.md).
