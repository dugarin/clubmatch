# Архитектура 04б. Матрица покрытия FR → эндпоинты

Roadmap 2.8 (пересчёт покрытия). Контракт: [04-openapi.yaml](04-openapi.yaml).

| FR | Эндпоинт(ы) | UC |
|---|---|---|
| FR-1, FR-2 | POST /profiles | UC-1 |
| FR-3 | GET, PATCH /profiles/me | UC-2 |
| FR-4 | GET /tags/autocomplete; POST /moderation/tags/{id}:revoke | UC-1, UC-2 |
| FR-5 | GET /deck | UC-3 |
| FR-6, FR-7 | POST /evaluations | UC-3 |
| FR-8 | POST /evaluations (ответ 201, match=true); GET /matches | UC-4 |
| FR-9 | GET /deck (исключение 30 дн); GET /search (`my_evaluation`) | UC-3, UC-5 |
| FR-10 | GET /search | UC-5 |
| FR-11 | GET /search (location, meeting_format) | UC-5 |
| FR-12 | POST /evaluations (`source`); GET /search (метка) | UC-5 |
| FR-13 | POST /applications; POST /moderation/invites; approve; POST /profiles (`invite_token` — погашение ссылки-приглашения, ревью F2) | UC-1, UC-6 |
| FR-14 | Схемы CardPreMatch/CardAfterMatch (поля отсутствуют, не null) | все |
| FR-15 | /moderation/* | UC-6 |
| FR-16 | POST /profiles (`consent_version`) | UC-1 |
| FR-17 | DELETE /profiles/me; GET /matches (`contact_unavailable` → `card: null`, контакт не отдаётся, ревью F4) | UC-7 |

Пробелов нет: все FR-1…FR-17 имеют эндпоинт или выражены схемой контракта. NFR, выраженные вне контракта: NFR-1…NFR-3 (производительность — observability), NFR-6 (доступность), NFR-10 (журналы/хранение), NFR-11 (клиент), NFR-13 (язык лейблов).

Статус: draft (2026-10-30).
