# MVP v0.1

Дата фиксации MVP: 19 мая 2026.

## Что Реализовано В Backend

- Docker Compose для PostgreSQL и Go backend.
- PostgreSQL schema, goose migrations, sqlc queries.
- REST API для запуска workflow, просмотра кейсов и HITL-операций.
- FSM управленческого случая.
- Протокол П / `decision_trace`.
- Модули `diagnostics`, `scenario`, `cvar`.
- Workflow от события отклонения до `action_selected`.
- Экспертная валидация: submit, approved/rejected/returned, execute, archive.
- CORS для frontend origin `http://localhost:5173`.

## Что Реализовано Во Frontend

- React dashboard для демонстрации СППР.
- Реестр управленческих случаев.
- Форма создания события отклонения.
- Карточка кейса.
- Блоки diagnostics, scenarios, actions, trace.
- HITL-кнопки: submit-validation, approved, rejected, returned, execute, archive.
- Сборка `npm run build` проходит.

## Основной Demo-Flow

1. Пользователь создаёт событие отклонения спроса от плана.
2. Backend создаёт кейс и выполняет workflow до `action_selected`.
3. Frontend показывает карточку кейса, диагностику, сценарии, действия и trace.
4. Пользователь отправляет кейс на экспертную валидацию.
5. Эксперт принимает или отклоняет решение.
6. Approved-кейс исполняется и архивируется.
7. Rejected-кейс архивируется.
8. Все ключевые шаги фиксируются в `decision_trace`.

## Ограничения MVP

- Нет авторизации, пользователей и ролей.
- Нет Swagger / OpenAPI.
- Нет experiments runner.
- Нет интеграции с ERP/MES.
- CVaR-модель демонстрационная, с MVP-коэффициентами.
- Диагностика правиловая, без ML-модели.
- Frontend предназначен для демонстрации, не для промышленной эксплуатации.

## Что Планируется Дальше

- Улучшение UI и визуальной иерархии dashboard.
- Графики CVaR, VaR и ExpectedLoss.
- Experiments runner для сравнения `base` vs `intelligent`.
- Swagger/OpenAPI спецификация.
- Подготовка скриншотов для главы 4 диссертации.
