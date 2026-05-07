# Subscription Service

REST-сервис для агрегации данных об онлайн-подписках пользователей.

CRUDL-операции над записями о подписках + ручка подсчёта суммарной стоимости за выбранный период с фильтрацией по `user_id` и названию сервиса.

---

## Содержание

- [Subscription Service](#subscription-service)
  - [Содержание](#содержание)
  - [Возможности](#возможности)
  - [Стек технологий](#стек-технологий)
  - [Структура проекта](#структура-проекта)
  - [Быстрый старт](#быстрый-старт)
    - [Требования](#требования)
    - [Запуск](#запуск)
    - [Проверка работоспособности](#проверка-работоспособности)
    - [Остановка](#остановка)
  - [API](#api)
    - [Модель Subscription](#модель-subscription)
    - [Ответы при ошибках](#ответы-при-ошибках)
  - [Примеры запросов](#примеры-запросов)
    - [Создание подписки](#создание-подписки)
    - [Получение по ID](#получение-по-id)
    - [Список с фильтрацией](#список-с-фильтрацией)
    - [Частичное обновление](#частичное-обновление)
    - [Удаление](#удаление)
    - [Сумма за период](#сумма-за-период)
      - [Логика расчёта](#логика-расчёта)
  - [Конфигурация](#конфигурация)
    - [Шаблон `.env`](#шаблон-env)
  - [База данных и миграции](#база-данных-и-миграции)
    - [Схема](#схема)
    - [Применение миграций](#применение-миграций)
    - [Добавление новой миграции](#добавление-новой-миграции)
    - [Просмотр содержимого БД](#просмотр-содержимого-бд)
  - [Тестирование](#тестирование)
    - [Unit-тесты](#unit-тесты)
    - [E2E-тесты через curl](#e2e-тесты-через-curl)
    - [Что покрывают тесты](#что-покрывают-тесты)
  - [Логирование](#логирование)
  - [Известные ограничения](#известные-ограничения)

---

## Возможности

- **CRUDL** для записей о подписках со следующими операциями:
    - название сервиса
    - стоимость месячной подписки в рублях (целое число)
    - UUID пользователя
    - дата начала подписки в формате `MM-YYYY`
    - опциональная дата окончания в том же формате
- **Расчёт суммарной стоимости** за выбранный период с фильтрацией по `user_id` и `service_name`
- **PostgreSQL** в качестве СУБД с автоматическими миграциями при старте
- **Swagger UI** для интерактивной работы с API
- **Docker Compose** — поднимает БД и приложение одной командой
- Конфигурация через `.env` или переменные окружения

---

## Стек технологий

| Компонент | Технология |
|---|---|
| Язык | Go 1.22 |
| HTTP-роутинг | `net/http` (стандартная библиотека) |
| СУБД | PostgreSQL 16 |
| Драйвер БД | `database/sql` + `github.com/lib/pq` |
| Миграции | `github.com/golang-migrate/migrate/v4` |
| Конфигурация | `github.com/joho/godotenv` |
| Контейнеризация | Docker + Docker Compose |
| API-документация | OpenAPI 3.0 + Swagger UI (через CDN) |

Внешних HTTP-фреймворков типа Gin/Echo нет — только стандартная библиотека.

---

## Структура проекта

```
.
├── main.go                              Точка входа: подключение к БД, миграции, роутинг
├── handlers.go                          Все HTTP-хендлеры + утилиты дат и UUID
├── models.go                            Структуры (Subscription, CreateRequest, ...)
├── handlers_test.go                     Unit-тесты parseDate / formatDate / makeNewID
├── migrations/
│   ├── 000001_create_subscriptions.up.sql      Создание таблицы и индексов
│   └── 000001_create_subscriptions.down.sql    Откат
├── docs/
│   └── swagger.json                     OpenAPI 3.0 спецификация
├── Dockerfile                           Multi-stage сборка (golang -> alpine)
├── docker-compose.yml                   PostgreSQL + приложение
├── Makefile                             build / run / test / up / down
├── test_api.sh                          E2E-тесты через curl с валидацией результатов
└── go.mod / go.sum                      Зависимости и контроль целостности
```

---

## Быстрый старт

### Требования

- Docker и Docker Compose
- Свободные порты `8080` (приложение) и `5432` (PostgreSQL)

### Запуск

```bash
git clone <repo-url>
cd subscription-service
docker compose up --build
```

При первом запуске:
1. Поднимается контейнер PostgreSQL
2. Healthcheck ждёт готовности БД (`pg_isready`)
3. Стартует приложение, прогоняет миграции, открывает HTTP-сервер

В логах появится:
```
Connected to database
Migrations applied
Server started on port 8080
Swagger UI: http://localhost:8080/swagger/
```

### Проверка работоспособности

```bash
curl http://localhost:8080/api/v1/subscriptions
# → []
```

### Остановка

```bash
docker compose down            # остановить (данные сохранятся в volume)
docker compose down -v         # остановить и удалить данные
```

---

## API

Базовый путь: `/api/v1`

| Метод | Путь | Описание |
|---|---|---|
| `POST`   | `/subscriptions`        | Создать подписку |
| `GET`    | `/subscriptions`        | Список подписок (с опциональными фильтрами) |
| `GET`    | `/subscriptions/{id}`   | Получить подписку по ID |
| `PUT`    | `/subscriptions/{id}`   | Частично обновить подписку |
| `DELETE` | `/subscriptions/{id}`   | Удалить подписку |
| `GET`    | `/subscriptions/total`  | Сумма стоимости за период с фильтрами |

Полная интерактивная документация: **http://localhost:8080/swagger/**

### Модель Subscription

```json
{
  "id":           "b8619cc6-fe21-48b3-8a45-e06fe8c999f0",
  "service_name": "Yandex Plus",
  "price":        400,
  "user_id":      "60601fee-2bf1-4721-ae6f-7636e79a0cba",
  "start_date":   "07-2025",
  "end_date":     "12-2025",
  "created_at":   "2026-05-07T10:00:00Z",
  "updated_at":   "2026-05-07T10:00:00Z"
}
```

| Поле | Тип | Обязательное | Описание |
|---|---|---|---|
| `id` | UUID v4 | — (генерируется) | Уникальный идентификатор записи |
| `service_name` | string | да | Название сервиса |
| `price` | integer | да, > 0 | Стоимость в рублях, без копеек |
| `user_id` | UUID | да | UUID пользователя (валидация существования не выполняется) |
| `start_date` | string `MM-YYYY` | да | Месяц и год начала подписки |
| `end_date` | string `MM-YYYY` | нет | Месяц и год окончания (должен быть позже `start_date`) |
| `created_at` | timestamp | — (автоматически) | Время создания записи в БД |
| `updated_at` | timestamp | — (автоматически) | Время последнего изменения |

### Ответы при ошибках

Любая ошибка возвращается с соответствующим HTTP-кодом и телом:

```json
{ "error": "Field 'price' must be greater than 0" }
```

| Код | Когда возникает |
|---|---|
| `400 Bad Request` | Невалидный JSON, нарушение валидаций, плохой формат UUID или даты, отсутствие обязательных query-параметров |
| `404 Not Found` | Запись по ID не найдена |
| `405 Method Not Allowed` | На пути использован неподдерживаемый HTTP-метод |
| `500 Internal Server Error` | Сбой БД или другая внутренняя ошибка |

---

## Примеры запросов

### Создание подписки

```bash
curl -X POST http://localhost:8080/api/v1/subscriptions \
  -H "Content-Type: application/json" \
  -d '{
    "service_name": "Yandex Plus",
    "price": 400,
    "user_id": "60601fee-2bf1-4721-ae6f-7636e79a0cba",
    "start_date": "07-2025"
  }'
```

Ответ (`201 Created`):
```json
{
  "id": "b8619cc6-fe21-48b3-8a45-e06fe8c999f0",
  "service_name": "Yandex Plus",
  "price": 400,
  "user_id": "60601fee-2bf1-4721-ae6f-7636e79a0cba",
  "start_date": "07-2025",
  "created_at": "2026-05-07T10:00:00Z",
  "updated_at": "2026-05-07T10:00:00Z"
}
```

### Получение по ID

```bash
curl http://localhost:8080/api/v1/subscriptions/b8619cc6-fe21-48b3-8a45-e06fe8c999f0
```

### Список с фильтрацией

```bash
# все
curl http://localhost:8080/api/v1/subscriptions

# по пользователю
curl "http://localhost:8080/api/v1/subscriptions?user_id=60601fee-2bf1-4721-ae6f-7636e79a0cba"

# по сервису
curl "http://localhost:8080/api/v1/subscriptions?service_name=Yandex%20Plus"

# оба фильтра
curl "http://localhost:8080/api/v1/subscriptions?user_id=60601fee-2bf1-4721-ae6f-7636e79a0cba&service_name=Yandex%20Plus"
```

### Частичное обновление

В теле передаются только те поля, которые нужно изменить. Остальные сохраняют прежние значения.

```bash
curl -X PUT http://localhost:8080/api/v1/subscriptions/b8619cc6-fe21-48b3-8a45-e06fe8c999f0 \
  -H "Content-Type: application/json" \
  -d '{ "price": 599 }'
```

### Удаление

```bash
curl -X DELETE http://localhost:8080/api/v1/subscriptions/b8619cc6-fe21-48b3-8a45-e06fe8c999f0
# → 204 No Content
```

### Сумма за период

Параметры `from` и `to` обязательны (формат `MM-YYYY`), `user_id` и `service_name` опциональны.

```bash
curl "http://localhost:8080/api/v1/subscriptions/total?from=01-2025&to=12-2025"
```

С фильтрами:
```bash
curl "http://localhost:8080/api/v1/subscriptions/total?from=01-2025&to=12-2025&user_id=60601fee-2bf1-4721-ae6f-7636e79a0cba&service_name=Yandex%20Plus"
```

Ответ:
```json
{ "total": 4800 }
```

#### Логика расчёта

Для каждой подходящей под фильтры подписки вклад в сумму равен:

```
price × число_месяцев_перекрытия_с_периодом
```

Подписка без `end_date` считается активной до конца запрошенного периода.

Пример: подписка `Yandex Plus`, 400 ₽/мес, `start_date: 07-2025`, без `end_date`. Запрос периода `01-2025..12-2025`:

- перекрытие: с 07-2025 по 12-2025 = 6 месяцев
- вклад: `400 × 6 = 2400 ₽`

---

## Конфигурация

Все настройки задаются через переменные окружения. Локально удобно использовать `.env` (загружается автоматически), в Docker — секция `environment` в `docker-compose.yml`.

| Переменная | По умолчанию | Описание |
|---|---|---|
| `SERVER_PORT` | `8080` | Порт HTTP-сервера |
| `DB_HOST` | `localhost` | Хост PostgreSQL |
| `DB_PORT` | `5432` | Порт PostgreSQL |
| `DB_USER` | `postgres` | Пользователь |
| `DB_PASSWORD` | `postgres` | Пароль |
| `DB_NAME` | `subscriptions` | Имя БД |
| `DB_SSLMODE` | `disable` | Режим SSL (`disable` / `require` / `verify-ca` / `verify-full`) |

### Шаблон `.env`

Скопируйте `.env.example` в `.env`:

```bash
cp .env.example .env
```

---

## База данных и миграции

### Схема

Таблица `subscriptions` создаётся миграцией `000001_create_subscriptions.up.sql`:

```sql
CREATE TABLE subscriptions (
    id           UUID        PRIMARY KEY,
    service_name TEXT        NOT NULL,
    price        INTEGER     NOT NULL CHECK (price > 0),
    user_id      UUID        NOT NULL,
    start_date   DATE        NOT NULL,
    end_date     DATE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT end_after_start CHECK (end_date IS NULL OR end_date > start_date)
);

CREATE INDEX idx_subscriptions_user_id      ON subscriptions (user_id);
CREATE INDEX idx_subscriptions_service_name ON subscriptions (service_name);
CREATE INDEX idx_subscriptions_start_date   ON subscriptions (start_date);
```

`start_date` и `end_date` хранятся как `DATE` (первое число соответствующего месяца). Парсинг и форматирование между `time.Time` и строкой `MM-YYYY` происходит в коде через `parseDate` / `formatDate`.

### Применение миграций

Миграции применяются **автоматически при каждом старте приложения** через `golang-migrate`. Состояние отслеживается в служебной таблице `schema_migrations`. Если миграция уже применена — `migrate.ErrNoChange` игнорируется.

При сбое миграции в `schema_migrations` ставится `dirty = true` и сервис откажется стартовать до ручного вмешательства.

### Добавление новой миграции

Создайте пару файлов с новым номером:
```
migrations/000002_<name>.up.sql
migrations/000002_<name>.down.sql
```

При следующем запуске она применится автоматически.

### Просмотр содержимого БД

```bash
# в контейнере
docker compose exec postgres psql -U postgres -d subscriptions

# с хоста (если установлен psql)
psql -h localhost -p 5432 -U postgres -d subscriptions
```

---

## Тестирование

### Unit-тесты

Не требуют запущенной БД, проверяют чистые функции (парсинг даты, форматирование, генерация и валидация UUID):

```bash
go test ./...
```

Или через `make`:

```bash
make test
```

### E2E-тесты через curl

Скрипт [`test_api.sh`](test_api.sh) выполняет полный сценарий: создаёт записи, проверяет CRUDL по обеим, проверяет фильтры, считает `total` по известным значениям, удаляет всё в конце.

```bash
docker compose up -d --build
./test_api.sh
```

Ожидаемый итог:
```
Passed: 38    Failed: 0
```

Скрипт возвращает exit code 0 при всех PASS — пригоден для CI.

### Что покрывают тесты

- Happy-path для каждой ручки
- Все варианты ошибок валидации (400)
- Несуществующие ID (404)
- Неподдерживаемые методы (405)
- Корректность арифметики `total` (числа выведены вручную из созданных записей)
- Идемпотентность Delete (повторное удаление → 404)
- Неизменность нетронутых полей при PUT

---

## Логирование

Используется стандартный `log` из стандартной библиотеки. На каждом значимом событии пишется человекочитаемая строка:

```
2026/05/07 10:00:00 Connected to database
2026/05/07 10:00:00 Migrations applied
2026/05/07 10:00:00 Server started on port 8080
2026/05/07 10:00:15 Created subscription b8619cc6-fe21-48b3-8a45-e06fe8c999f0
2026/05/07 10:00:42 Updated subscription b8619cc6-fe21-48b3-8a45-e06fe8c999f0
2026/05/07 10:01:03 Calculated total: 4800
```

Ошибки БД логируются с деталями в stderr; клиенту при этом отдаётся обобщённое сообщение без раскрытия внутренностей.

---

## Известные ограничения

- **Существование пользователя не проверяется.** ТЗ явно указывает: «Управление пользователями вне зоны ответственности вашего сервиса». Любой валидный UUID считается допустимым.
- **Цена — целое число рублей**, копейки не учитываются (требование ТЗ).
- **Аутентификация и авторизация отсутствуют** — сервис рассчитан на работу за обратным прокси / API-gateway, который их обеспечивает.
- **Без HTTPS** — терминацию TLS можно реализовать на стороне прокси.
- **Вывод без пагинации** на `GET /subscriptions`.
