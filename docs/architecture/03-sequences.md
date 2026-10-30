# Архитектура 03. Sequence-диаграммы

Roadmap 2.7. Статус: draft-предложение. Механика матча — по FR-7/FR-8/FR-12, ADR-002, ADR-004.

## S1. Матч: взаимное «интересно» → раскрытие контакта → недоставка уведомления

```mermaid
sequenceDiagram
    participant A as Участник A (Mini App)
    participant API as Backend API
    participant DB as PostgreSQL
    participant W as Notification Worker
    participant TG as Telegram Bot API
    participant B as Участник B

    A->>API: POST /evaluations {target: B, value: interested}
    API->>DB: TX: upsert Evaluation(A→B); SELECT Evaluation(B→A)
    alt B уже оценил A «интересно»
        DB-->>API: встречная оценка есть
        API->>DB: TX: insert Match; insert ContactDisclosureJournal ×2; insert outbox ×2 (notify_match)
        API-->>A: 201 {match: true, card: <видимость after_match>}
        Note over A: Контакт B доступен сразу в ответе и в «Матчах» (FR-8)
    else нет встречной оценки
        API-->>A: 201 {match: false} (оценка зафиксирована; B не информируется, FR-7)
    end

    loop каждые ≤30 c
        W->>DB: SELECT due из outbox
        W->>TG: sendMessage(B: «У вас матч», контакт A)
        alt доставлено
            W->>DB: state=sent
        else бот заблокирован (403)
            W->>DB: state=blocked (повтор при /start)
            Note over B: Контакт A остаётся в «Матчах» Mini App (UC-4 E2, FR-8)
        else сбой сети/API
            W->>DB: retry, backoff 1→5→15→60 мин (ADR-002)
        end
    end
```

Ключевые инварианты: матч+журнал+outbox — одна транзакция; снятие «интересно» после матча не отзывает контакт (FR-12); входящая оценка не отдаётся API до взаимности (FR-7).

## S2. Допуск заявки: подача → решение модератора → уведомление → онбординг

```mermaid
sequenceDiagram
    participant C as Кандидат (Mini App)
    participant API as Backend API
    participant DB as PostgreSQL
    participant W as Notification Worker
    participant TG as Telegram Bot API
    participant M as Модератор (Mini App)

    C->>API: POST /applications (initData)
    API->>DB: TX: insert Application(pending)
    API-->>C: 201

    M->>API: GET /moderation/applications?state=pending
    M->>API: POST /moderation/applications/{id}:approve
    API->>DB: TX: Application=approved(decided_by/at); User.access_state=approved; insert ModerationJournal; insert outbox (notify_approved)
    API-->>M: 200
    W->>DB: due →
    W->>TG: sendMessage(C: «Заявка одобрена»)
    Note over C: При отклонении — notify_rejected; повторная подача разрешена (Q21)
    C->>API: POST /profiles (онбординг, UC-1)
```

Инварианты: решение модератора идемпотентно (повторный approve — 409); уведомление — через outbox, не в запросе модератора (NFR-12, ADR-002); бот не канал подачи заявки (NFR-7).

## Открытые вопросы

1. ~~409 на повторное решение по заявке~~ — утверждено 2026-10-30.

Статус: draft (2026-10-30). См. [roadmap](../roadmap.md).
