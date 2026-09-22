# sppr-prototype — веб-прототип СППР для интеллектуального контура внутрифирменного планирования

`sppr-prototype` — демонстрационный backend веб-прототипа интеллектуальной системы поддержки принятия решений (СППР) в контуре внутрифирменного планирования промышленного предприятия.

MVP v0.1 показывает сквозной сценарий обработки управленческого случая: регистрация события отклонения, диагностика вероятной причины, генерация сценариев, CVaR-оценка корректирующих действий, выбор действия, экспертная валидация human-in-the-loop, исполнение или архивирование и фиксация протокола `decision_trace` / П.

## Назначение

Система демонстрирует:

- регистрацию события отклонения от плана;
- причинную диагностику отклонения;
- генерацию сценариев развития ситуации;
- CVaR-оценку корректирующих действий;
- выбор рекомендуемого действия;
- экспертную валидацию human-in-the-loop;
- исполнение и архивирование управленческого случая;
- ведение протокола принятия решения `decision_trace` / П.

## Архитектура Backend

- `cmd/api` — точка входа HTTP-сервера, wiring зависимостей, router, graceful shutdown.
- `internal/handler` — REST handlers, DTO, единый формат ошибок.
- `internal/service` — orchestration layer: workflow, смена статусов, экспертная валидация.
- `internal/repository` — adapter между сервисами и `sqlc`.
- `internal/db` — сгенерированный `sqlc`-код доступа к PostgreSQL.
- `internal/fsm` — чистая статусная машина управленческого случая.
- `internal/protocol` — структуры протокола П и JSON payload для `decision_trace`.
- `internal/cvar` — чистый модуль сценарной VaR/CVaR-оценки действий.
- `internal/diagnostics` — чистый модуль MVP-диагностики причин отклонения.
- `internal/scenario` — генерация сценариев, KPIConfig и корректирующих действий.

## Технологический Стек Backend

- Go
- Gin
- PostgreSQL
- pgx
- sqlc
- goose
- Docker Compose

## Переменные Окружения

- `APP_ENV` — окружение запуска, например `development`.
- `HTTP_PORT` — порт внутри backend-контейнера, по умолчанию `8080`.
- `BACKEND_HOST_PORT` — внешний порт backend при локальном запуске, в демонстрации используется `18080`.
- `DATABASE_URL` — строка подключения к PostgreSQL.
- `FRONTEND_ORIGIN` — origin frontend-приложения для CORS, по умолчанию `http://localhost:5173`.

Пример см. в [.env.example](.env.example).

## Запуск Backend

```bash
docker compose up --build -d
```

Проверка:

```bash
curl http://localhost:18080/health
curl http://localhost:18080/ready
```

Ожидаемые ответы:

```json
{"service":"sppr-backend","status":"ok"}
```

```json
{"db":"up","status":"ok"}
```

## API Endpoints

- `GET /health` — liveness backend.
- `GET /ready` — readiness backend и ping PostgreSQL.
- `POST /api/workflow/process` — запуск workflow по событию отклонения, создание кейса до `action_selected`.
- `GET /api/cases` — список кейсов, поддерживает `?status=...`.
- `GET /api/cases/:id` — карточка кейса.
- `GET /api/cases/:id/trace` — протокол П / `decision_trace`.
- `GET /api/cases/:id/diagnostics` — диагностические гипотезы кейса.
- `GET /api/cases/:id/scenarios` — сценарии кейса.
- `GET /api/cases/:id/actions` — корректирующие действия кейса.
- `POST /api/cases/:id/submit-validation` — отправка кейса на экспертную валидацию.
- `POST /api/cases/:id/expert-decision` — фиксация решения эксперта: `approved`, `rejected`, `returned`.
- `POST /api/cases/:id/execute` — перевод approved-кейса в `executed`.
- `POST /api/cases/:id/archive` — архивирование `executed` или `expert_rejected` кейса.

## Полный Curl-Сценарий

Создать кейс:

```bash
curl -X POST http://localhost:18080/api/workflow/process \
  -H "Content-Type: application/json" \
  -d '{"event_id":"evt-demo-001","product":"A100","plan_demand":1000,"fact_demand":1350,"capacity_load":0.93,"forecast_error":0.18,"campaign_flag":true,"alpha":0.95}'
```

Получить данные кейса:

```bash
curl http://localhost:18080/api/cases
curl http://localhost:18080/api/cases/<case_id>
curl http://localhost:18080/api/cases/<case_id>/trace
curl http://localhost:18080/api/cases/<case_id>/diagnostics
curl http://localhost:18080/api/cases/<case_id>/scenarios
curl http://localhost:18080/api/cases/<case_id>/actions
```

Пройти approved HITL-сценарий:

```bash
curl -X POST http://localhost:18080/api/cases/<case_id>/submit-validation

curl -X POST http://localhost:18080/api/cases/<case_id>/expert-decision \
  -H "Content-Type: application/json" \
  -d '{"outcome":"approved","comment":"Решение подтверждено экспертом","expert_id":"demo-expert"}'

curl -X POST http://localhost:18080/api/cases/<case_id>/execute

curl -X POST http://localhost:18080/api/cases/<case_id>/archive

curl http://localhost:18080/api/cases/<case_id>/trace
```

## Статусы FSM

- `registered` — кейс зарегистрирован.
- `normalized` — данные нормализованы.
- `deviation_confirmed` — отклонение подтверждено.
- `diagnosed` — причина диагностирована.
- `risk_assessed` — риск оценён.
- `alternatives_generated` — альтернативы сформированы.
- `action_selected` — корректирующее действие выбрано.
- `expert_validation` — кейс передан эксперту.
- `expert_approved` — эксперт подтвердил решение.
- `expert_rejected` — эксперт отклонил решение.
- `expert_returned` — эксперт вернул кейс на доработку.
- `executed` — действие исполнено.
- `archived` — кейс архивирован.

## Decision Trace / Протокол П

`decision_trace` фиксирует последовательность действий интеллектуального контура и экспертной валидации.

Пример структуры:

```json
{
  "step": "cvar_calculated",
  "timestamp": "2026-05-19T12:38:09Z",
  "data_snapshot": {
    "expected_loss": 50.96,
    "selected_action_code": "keep_plan"
  },
  "model_version": "",
  "parameters": {
    "alpha": 0.95
  },
  "risk": {
    "alpha": 0.95,
    "tail_probability": 0.05,
    "var_value": 88.72,
    "cvar_value": 88.72
  },
  "expert_decision": {
    "outcome": "approved",
    "comment": "Решение подтверждено экспертом",
    "expert_id": "demo-expert"
  },
  "final_action_id": null
}
```

## MVP v0.1

Реализовано:

- PostgreSQL schema, goose migrations, sqlc queries.
- Go backend на Gin с Docker Compose.
- FSM управленческого случая.
- Протокол П / `decision_trace`.
- MVP-диагностика отклонений.
- Генерация сценариев и корректирующих действий.
- CVaR-модуль для риск-ориентированного выбора действия.
- Workflow до `action_selected`.
- Human-in-the-loop экспертная валидация.
- REST API для backend/frontend связки.
- CORS для `http://localhost:5173`.
- React frontend dashboard в отдельном проекте `d:\ProjectApp\sites\sppr-prototype`.

## Что Не Входит В MVP v0.1

- Авторизация.
- Swagger / OpenAPI.
- Experiments runner.
- Интеграция с ERP/MES.
- Промышленная эксплуатация.
- Настоящие пользователи и роли.
- Полноценная оптимизационная модель и промышленная CVaR-калибровка.

## Проверки Разработчика

Backend:

```bash
cd d:\ProjectApp\go\sppr-prototype\backend
go test ./...
```

Frontend:

```bash
cd d:\ProjectApp\sites\sppr-prototype
npm run build
```
