# CD: деплой и откат

Задача 3.0.7 ([план фазы 3](../phase-3-plan.md)); трассировка: Q-3 (ред. 2), NFR-6.
Пайплайн: [.github/workflows/deploy.yml](../../.github/workflows/deploy.yml), окружение —
[docker-compose.stage.yml](../../docker-compose.stage.yml) (см. [stage.md](stage.md)).

## 1. Схема деплоя

| Окружение | Триггер | Job | Состояние |
|---|---|---|---|
| stage | мердж/push в `main` (и вручную) | `deploy-stage` | выключен до настройки секретов (см. §2) |
| prod | тег `v*` (и вручную) | `deploy-prod` | выключен: прод-сервер не выбран (ADR-001) |

Оба job помечены feature-флагом (`vars.DEPLOY_*_ENABLED`) и **пропускаются**, пока
флаги не включены: пайплайн зелёный без настроенного сервера. Это сознательно.

Порядок шагов деплоя (по SSH на сервер):

1. `git fetch --tags && git checkout --force <ref> && git reset --hard <sha>` — точный коммит;
2. **goose-миграции** отдельным шагом: `docker compose run --rm migrate` (идемпотентно;
   затем тот же `up` прогонит migrate и через depends_on-цепочку compose);
3. `docker compose up -d --build --remove-orphans` — пересборка и рестарт;
4. пост-проверка: `compose ps` + `/healthz` внутри контейнера.

Образы пока собираются **на сервере из git** (registry не выбран, ADR-001/Q-1.2).
Профиль `monitoring` пайплайном не трогается (управляется вручную, см. stage.md §6).

## 2. Секреты и переменные (GitHub → Settings)

Включение: сначала задать секреты/переменные, затем `Variables` → `DEPLOY_STAGE_ENABLED = true`.

### Stage (обязательно)

| Тип | Имя | Значение |
|---|---|---|
| secret | `STAGE_SSH_HOST` | хост stage-сервера (IP или домен) |
| secret | `STAGE_SSH_USER` | пользователь с правами на docker и `/opt/clubmatch` |
| secret | `STAGE_SSH_PRIVATE_KEY` | приватный ed25519-ключ деплоя (публичный — в `~/.ssh/authorized_keys` сервера) |
| secret | `STAGE_SSH_KNOWN_HOSTS` | вывод `ssh-keyscan -p <port> <host>` (рекомендуется; без него — режим accept-new/TOFU) |
| variable | `STAGE_DEPLOY_DIR` | каталог репо на сервере (по умолчанию `/opt/clubmatch`) |
| variable | `STAGE_SSH_PORT` | SSH-порт (по умолчанию 22) |
| variable | `DEPLOY_STAGE_ENABLED` | `true` — включить деплой stage |

### Prod (когда появится сервер; сейчас плейсхолдеры)

Те же с префиксом `PROD_*` (`PROD_SSH_HOST/USER/PRIVATE_KEY/KNOWN_HOSTS`,
`PROD_DEPLOY_DIR`, `PROD_SSH_PORT`) + `DEPLOY_PROD_ENABLED = true`.
На GitHub-окружении `prod` **обязательно** включить required reviewers (Settings →
Environments → prod). Для stage — по желанию.

Серверу также нужен доступ к репозиторию: read-only deploy key (см. stage.md §2).

## 3. Откат

### 3.1 Откат приложения (без миграций)

Пока деплой из git — откат на предыдущий тег/коммит + пересборка:

```bash
# на сервере
cd /opt/clubmatch
git fetch --tags origin
git checkout --force v0.3.4          # предыдущий релизный тег (git tag -l 'v*' --sort=-v:refname)
git reset --hard v0.3.4
docker compose -f docker-compose.stage.yml up -d --build
```

Когда появится registry (§4): откат = предыдущий тег образа без пересборки:

```bash
docker compose -f docker-compose.stage.yml pull
docker tag <registry>/clubmatch-server:v0.3.4 <registry>/clubmatch-server:deploy
docker compose -f docker-compose.stage.yml up -d
```

### 3.2 Откат миграций (goose down) — крайняя мера

Правило: **предпочтителен forward-fix** (новая миграция-исправление), а не `down`.
`goose down` откатывает последнюю миграцию и её Down-скрипт обязан быть написан
обратимым — проверяйте Down-часть каждой миграции на ревью. Перед откатом — свежий
бэкап (`./ops/backup.sh`).

```bash
cd /opt/clubmatch
./ops/backup.sh                       # страховка

docker compose -f docker-compose.stage.yml stop server worker   # не писать во время отката
docker compose -f docker-compose.stage.yml run --rm migrate status   # какие версии применены
docker compose -f docker-compose.stage.yml run --rm migrate down     # ОТКАТИТЬ ПОСЛЕДНЮЮ
docker compose -f docker-compose.stage.yml run --rm migrate down-to 00002_example  # до версии
docker compose -f docker-compose.stage.yml start server worker
```

(`run --rm migrate <cmd>` подставляет `<cmd>` вместо `up` — entrypoint уже содержит
`goose -dir db/migrations postgres <DSN>`; команды goose: `up | up-to | down | down-to |
redo | status | version`.)

Откат приложения + миграций при деплое тега: проверить, что откатываемый тег согласован
с применённой версией схемы (`migrate status`), иначе сначала §3.2, затем §3.1.

### 3.3 Откат данных

Восстановление БД из бэкапа: `./ops/restore.sh backups/clubmatch-<UTC>.dump` (stage.md §5).

## 4. Переезд на registry (когда владелец выберет)

1. В `.github/workflows/ci.yml` раскомментировать шаги push (задать `vars.REGISTRY_PUSH`,
   `vars.REGISTRY`, `vars.IMAGE_PREFIX`, `secrets.REGISTRY_USERNAME/PASSWORD`) — образы
   `server`/`worker` начнут публиковаться с тегами `<sha>` и `latest`.
2. В `docker-compose.stage.yml` у `server`/`worker` заменить `build:` на
   `image: ${REGISTRY}/${IMAGE_PREFIX}-server:${IMAGE_TAG}` (и для worker).
3. В `deploy.yml` шаг 2 заменить на: `docker compose pull && docker compose up -d`
   (без `--build`), миграции — без изменений (`migrate` остаётся сервисом из исходников
   либо переезжает на образ goose вместе с Dockerfile).
4. Откат — §3.1 «с registry»: пул предыдущего тега образа.

## 5. Связанное

- CI (lint/test/build без push): [.github/workflows/ci.yml](../../.github/workflows/ci.yml).
- Защиту main от деплоя сломанного кода обеспечивает branch protection (required CI checks) —
  настроить в GitHub → Settings → Branches `[решение владельца]`.
